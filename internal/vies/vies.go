// Package vies looks VAT numbers up in VIES, the European Commission's
// public register of EU VAT numbers, so Scan and Recognise can add a new
// supplier under its registered name. Only the VAT number is sent.
package vies

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/adirkuhn/moneyharpy/internal/recognize"
)

// ProductionURL is the base address of the VIES REST API.
const ProductionURL = "https://ec.europa.eu/taxation_customs/vies/rest-api"

// Tuning of the client.
const (
	// timeout caps one lookup: recognition waits for it.
	timeout = 5 * time.Second
	// cacheFor is how long an answer is kept.
	cacheFor = 24 * time.Hour
	// maxCached caps the answers kept; the oldest go first.
	maxCached = 1000
	maxBody   = 1 << 16
)

// ErrUnavailable means VIES could not answer, such as when a member state's
// register is down.
var ErrUnavailable = errors.New("vies unavailable")

// number is the part of a VAT number after its country prefix.
var number = regexp.MustCompile(`^[0-9A-Z+*]{2,12}$`)

type entry struct {
	company recognize.RegisteredCompany
	at      time.Time
}

// Client asks VIES and keeps its answers for a day.
type Client struct {
	base    string
	http    *http.Client
	timeout time.Duration
	now     func() time.Time

	mu    sync.Mutex
	cache map[string]entry
	order []string
}

// New returns a client for the VIES REST API at baseURL.
func New(baseURL string, httpClient *http.Client) *Client {
	return &Client{
		base: strings.TrimRight(baseURL, "/"), http: httpClient, timeout: timeout, now: time.Now,
		cache: map[string]entry{},
	}
}

// LookupVAT implements recognize.Registry.
func (c *Client) LookupVAT(ctx context.Context, vat string) (recognize.RegisteredCompany, error) {
	n := recognize.NormalizeVAT(vat)
	if !recognize.InVIES(n) || !number.MatchString(n[2:]) {
		return recognize.RegisteredCompany{}, fmt.Errorf("vies: %q is not an EU VAT number", vat)
	}
	if company, ok := c.cached(n); ok {
		return company, nil
	}
	company, err := c.ask(ctx, n[:2], n[2:])
	if err != nil {
		return recognize.RegisteredCompany{}, err
	}
	c.keep(n, company)
	return company, nil
}

type answer struct {
	IsValid   bool   `json:"isValid"`
	UserError string `json:"userError"`
	Name      string `json:"name"`
	Address   string `json:"address"`
}

func (c *Client) ask(ctx context.Context, country, num string) (recognize.RegisteredCompany, error) {
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.base+"/ms/"+country+"/vat/"+num, nil)
	if err != nil {
		return recognize.RegisteredCompany{}, err
	}
	req.Header.Set("Accept", "application/json")
	res, err := c.http.Do(req)
	if err != nil {
		return recognize.RegisteredCompany{}, fmt.Errorf("%w: %w", ErrUnavailable, err)
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusOK {
		return recognize.RegisteredCompany{}, fmt.Errorf("%w: answered %s", ErrUnavailable, res.Status)
	}
	var a answer
	if err := json.NewDecoder(io.LimitReader(res.Body, maxBody)).Decode(&a); err != nil {
		return recognize.RegisteredCompany{}, fmt.Errorf("%w: %w", ErrUnavailable, err)
	}
	// VALID and INVALID are answers; anything else (MS_UNAVAILABLE,
	// TIMEOUT, MS_MAX_CONCURRENT_REQ …) is VIES failing to answer.
	switch a.UserError {
	case "VALID", "INVALID", "":
	default:
		return recognize.RegisteredCompany{}, fmt.Errorf("%w: %s", ErrUnavailable, a.UserError)
	}
	return recognize.RegisteredCompany{Valid: a.IsValid, Name: told(a.Name), Address: told(a.Address)}, nil
}

// told is what VIES tells: it answers "---" for what a member state keeps
// to itself.
func told(s string) string {
	if strings.TrimSpace(s) == "---" {
		return ""
	}
	return s
}

func (c *Client) cached(vat string) (recognize.RegisteredCompany, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.cache[vat]
	if !ok || c.now().Sub(e.at) > cacheFor {
		return recognize.RegisteredCompany{}, false
	}
	return e.company, true
}

func (c *Client) keep(vat string, company recognize.RegisteredCompany) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, ok := c.cache[vat]; !ok {
		c.order = append(c.order, vat)
	}
	c.cache[vat] = entry{company: company, at: c.now()}
	for len(c.order) > maxCached {
		delete(c.cache, c.order[0])
		c.order = c.order[1:]
	}
}
