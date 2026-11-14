// Package kvkweb searches the Dutch company register through the public search
// of the KVK website. It is experimental and unofficial: the endpoint is not
// documented, needs no key and may change or block requests at any time. The
// official KVK API (package kvk) is the supported way.
package kvkweb

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/adirkuhn/moneyharpy/internal/administration"
	"github.com/adirkuhn/moneyharpy/internal/ratelimit"
	"github.com/adirkuhn/moneyharpy/internal/settings"
)

// ProductionURL is the search endpoint behind the KVK website.
const ProductionURL = "https://web-api.kvk.nl/zoeken/v3/search"

// Env for profileId
const ProductionEnvConfig = "https://www.kvk.nl/config.env.js"

const (
	// userAgent names the software only, never the installation or its users.
	userAgent      = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/157.0.0.0 Safari/537.36"
	defaultTimeout = 5 * time.Second
	maxBodyBytes   = 1 << 20
	resultsPerPage = "10"
	// limiterKey counts every search of this installation together.
	limiterKey = "kvk-website"
	// checkQuery is what the settings check searches for: a common word, so
	// no one's data is sent.
	checkQuery = "bakkerij"
	profileTTL = 30 * time.Minute
)

// Settings tells whether the website search is enabled.
type Settings interface {
	Instance(ctx context.Context) (settings.Instance, error)
}

// Client searches the KVK website.
type Client struct {
	settings  Settings
	url       string
	configURL string
	http      *http.Client
	limiter   *ratelimit.Limiter
	timeout   time.Duration
	now       func() time.Time

	mu            sync.RWMutex
	profileID     string
	profileExpiry time.Time
}

// New returns a Client calling the search at url and fetching configuration from
// configURL. If configURL is empty, ProductionEnvConfig is used. The limiter
// caps the searches of the whole installation, to stay a polite visitor.
func New(s Settings, url, configURL string, httpClient *http.Client, limiter *ratelimit.Limiter) *Client {
	if configURL == "" {
		configURL = ProductionEnvConfig
	}
	return &Client{
		settings:  s,
		url:       url,
		configURL: configURL,
		http:      httpClient,
		limiter:   limiter,
		timeout:   defaultTimeout,
		now:       time.Now,
	}
}

// SearchCompanies finds companies by name while the website search is
// enabled, and answers administration.ErrLookupUnavailable otherwise.
func (c *Client) SearchCompanies(ctx context.Context, query string) ([]administration.CompanyMatch, error) {
	inst, err := c.settings.Instance(ctx)
	if err != nil {
		return nil, fmt.Errorf("read settings: %w", err)
	}
	if !inst.KVK.WebsiteSearch {
		return nil, administration.ErrLookupUnavailable
	}
	return c.search(ctx, query)
}

// LookupCompany finds the company with a KVK number by searching for the
// number, while the website search is enabled. A number it does not know
// is administration.ErrCompanyNotFound.
func (c *Client) LookupCompany(ctx context.Context, number string) (administration.CompanyMatch, error) {
	found, err := c.SearchCompanies(ctx, number)
	if err != nil {
		return administration.CompanyMatch{}, err
	}
	for _, m := range found {
		if m.ChamberOfCommerce == number {
			return m, nil
		}
	}
	return administration.CompanyMatch{}, administration.ErrCompanyNotFound
}

// Check runs a sample search, enabled or not, so an administrator can see
// whether the website search works from this server before relying on it.
func (c *Client) Check(ctx context.Context) error {
	_, err := c.search(ctx, checkQuery)
	return err
}

type location struct {
	Street      string     `json:"straat"`
	HouseNumber flexString `json:"huisnummer"`
	Addition    string     `json:"huisnummerToevoeging"`
	Zipcode     string     `json:"postcode"`
	City        string     `json:"plaats"`
}

type item struct {
	Number   string    `json:"kvkNummer"`
	Name     string    `json:"naam"`
	Branch   bool      `json:"vestiging"`
	Kind     string    `json:"inschrijvingstype"`
	Visiting *location `json:"bezoeklocatie"`
}

type searchResponse struct {
	Data *struct {
		Items []item `json:"items"`
	} `json:"data"`
}

func (c *Client) search(ctx context.Context, query string) ([]administration.CompanyMatch, error) {
	if !c.limiter.Allow(limiterKey).Allowed {
		return nil, administration.ErrRegisterBusy
	}
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	profileID, err := c.getProfileID(ctx)
	if err != nil {
		return nil, err
	}

	params := url.Values{
		"q": {query}, "language": {"nl"}, "site": {"handelsregister"}, "size": {resultsPerPage}, "start": {"0"},
		"inschrijvingsstatus": {"ingeschreven"},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.url+"?"+params.Encode(), nil)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Profileid", profileID)

	res, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", administration.ErrRegisterUnavailable, err)
	}
	defer func() {
		// The body has been read or abandoned; nothing depends on closing it.
		_ = res.Body.Close()
	}()
	body, err := io.ReadAll(io.LimitReader(res.Body, maxBodyBytes))
	if err != nil {
		return nil, fmt.Errorf("%w: read response: %w", administration.ErrRegisterUnavailable, err)
	}

	switch {
	case !isJSON(res.Header.Get("Content-Type"), body) && isChallenge(body):
		// Bot protection answers with a challenge page, sometimes as a 5xx.
		return nil, fmt.Errorf("%w: kvk answered a challenge page (%d)", administration.ErrRegisterBlocked, res.StatusCode)
	case res.StatusCode >= http.StatusInternalServerError:
		return nil, fmt.Errorf("%w: kvk answered %d", administration.ErrRegisterUnavailable, res.StatusCode)
	case res.StatusCode != http.StatusOK:
		if res.StatusCode == http.StatusBadRequest {
			c.mu.Lock()
			c.profileID = ""
			c.profileExpiry = time.Time{}
			c.mu.Unlock()
		}
		// 403 and 429 are refusals; any other answer means the search no
		// longer works the way it did, which is as good as blocked.
		return nil, fmt.Errorf("%w: kvk answered %d", administration.ErrRegisterBlocked, res.StatusCode)
	case !isJSON(res.Header.Get("Content-Type"), body):
		return nil, fmt.Errorf("%w: kvk answered a page instead of results", administration.ErrRegisterBlocked)
	}

	var parsed searchResponse
	if err := json.Unmarshal(body, &parsed); err != nil || parsed.Data == nil {
		return nil, fmt.Errorf("%w: unexpected kvk response", administration.ErrRegisterBlocked)
	}
	return companies(parsed.Data.Items), nil
}

type envConfig struct {
	ProfileID string `json:"NEXT_PUBLIC_SEARCH_V2_GENERAL_SEARCH_PROFILE_ID"`
}

func extractProfileID(body []byte) (string, error) {
	start := bytes.IndexByte(body, '{')
	end := bytes.LastIndexByte(body, '}')
	if start == -1 || end == -1 || start >= end {
		return "", errors.New("no json object in env config")
	}
	var cfg envConfig
	if err := json.Unmarshal(body[start:end+1], &cfg); err != nil {
		return "", fmt.Errorf("unmarshal env config: %w", err)
	}
	if cfg.ProfileID == "" {
		return "", errors.New("profile id missing in env config")
	}
	return cfg.ProfileID, nil
}

func (c *Client) currentTime() time.Time {
	if c.now != nil {
		return c.now()
	}
	return time.Now()
}

func (c *Client) getProfileID(ctx context.Context) (string, error) {
	now := c.currentTime()
	c.mu.RLock()
	id := c.profileID
	valid := id != "" && !c.profileExpiry.IsZero() && now.Before(c.profileExpiry)
	c.mu.RUnlock()
	if valid {
		return id, nil
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	if c.profileID != "" && !c.profileExpiry.IsZero() && now.Before(c.profileExpiry) {
		return c.profileID, nil
	}

	id, err := c.fetchProfileID(ctx)
	if err != nil {
		return "", err
	}
	c.profileID = id
	c.profileExpiry = now.Add(profileTTL)
	return id, nil
}

func (c *Client) fetchProfileID(ctx context.Context) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.configURL, nil)
	if err != nil {
		return "", fmt.Errorf("build env config request: %w", err)
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "*/*")

	res, err := c.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("%w: fetch env config: %w", administration.ErrRegisterUnavailable, err)
	}
	defer func() {
		_ = res.Body.Close()
	}()

	if res.StatusCode >= http.StatusInternalServerError {
		return "", fmt.Errorf("%w: env config answered %d", administration.ErrRegisterUnavailable, res.StatusCode)
	}
	if res.StatusCode != http.StatusOK {
		return "", fmt.Errorf("%w: env config answered %d", administration.ErrRegisterBlocked, res.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(res.Body, maxBodyBytes))
	if err != nil {
		return "", fmt.Errorf("%w: read env config: %w", administration.ErrRegisterUnavailable, err)
	}

	id, err := extractProfileID(body)
	if err != nil {
		return "", fmt.Errorf("%w: %w", administration.ErrRegisterBlocked, err)
	}
	return id, nil
}

// isJSON tells results from the HTML of a block or challenge page.
func isJSON(contentType string, body []byte) bool {
	if mediaType, _, err := mime.ParseMediaType(contentType); err == nil && mediaType != "application/json" {
		return false
	}
	return bytes.HasPrefix(bytes.TrimSpace(body), []byte("{"))
}

// companies keeps one match per KVK number, in the order of the results,
// taken from its best entry: the main branch, which has the visiting address,
// before other branches and before the legal entity, which has no address.
func companies(items []item) []administration.CompanyMatch {
	var order []string
	best := map[string]item{}
	for _, it := range items {
		if it.Number == "" {
			continue
		}
		current, seen := best[it.Number]
		if !seen {
			order = append(order, it.Number)
		}
		if !seen || rank(it) > rank(current) {
			best[it.Number] = it
		}
	}
	out := make([]administration.CompanyMatch, len(order))
	for i, number := range order {
		it := best[number]
		m := administration.CompanyMatch{Name: strings.TrimSpace(it.Name), ChamberOfCommerce: number}
		if loc := it.Visiting; loc != nil {
			m.Address1 = strings.TrimSpace(loc.Street + " " + houseNumber(string(loc.HouseNumber), loc.Addition))
			m.Zipcode = strings.ReplaceAll(strings.ToUpper(strings.TrimSpace(loc.Zipcode)), " ", "")
			m.City = strings.TrimSpace(loc.City)
		}
		out[i] = m
	}
	return out
}

func rank(it item) int {
	switch {
	case it.Branch && it.Kind == "Hoofdvestiging":
		return 2
	case it.Branch:
		return 1
	default:
		return 0
	}
}

// houseNumber writes a single letter right after the number, as in "1A", and
// longer additions after a hyphen, as in "12-bis".
func houseNumber(number, addition string) string {
	number, addition = strings.TrimSpace(number), strings.TrimSpace(addition)
	switch {
	case number == "" || addition == "":
		return number
	case utf8.RuneCountInString(addition) == 1:
		return number + addition
	default:
		return number + "-" + addition
	}
}

// flexString reads a value the website writes as a string or a number.
type flexString string

func (f *flexString) UnmarshalJSON(raw []byte) error {
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		*f = flexString(s)
		return nil
	}
	var n json.Number
	if err := json.Unmarshal(raw, &n); err != nil {
		return errors.New("house number is neither a string nor a number")
	}
	*f = flexString(n.String())
	return nil
}

// challengeMarks are what bot-protection pages say, in any status.
var challengeMarks = []string{"just a moment", "attention required", "challenge-platform", "captcha", "cf-chl"}

// isChallenge tells a bot-protection challenge from an ordinary error page.
func isChallenge(body []byte) bool {
	page := strings.ToLower(string(body))
	for _, mark := range challengeMarks {
		if strings.Contains(page, mark) {
			return true
		}
	}
	return false
}
