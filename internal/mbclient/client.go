// Package mbclient reads a Moneybird administration through Moneybird's API
// v2, for the importer that copies a whole administration into MoneyHarpy.
//
// It keeps to Moneybird's limit of 150 requests per 5 minutes on its own,
// pauses when Moneybird says no requests are left, and waits out a 429
// before trying again, so a long import finishes instead of failing halfway.
package mbclient

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

// DefaultBaseURL is Moneybird's API v2 in production.
const DefaultBaseURL = "https://moneybird.com/api/v2"

// Defaults for Moneybird's rate limit: 150 requests in 5 minutes.
const (
	DefaultLimit  = 150
	DefaultWindow = 5 * time.Minute
)

const (
	userAgent = "MoneyHarpy importer"
	// perPage is the most Moneybird returns per page.
	perPage = 100
	// batchSize is the most records a synchronization POST returns; ids past
	// it are silently dropped by Moneybird.
	batchSize = 100
	// maxRateLimited is the number of 429s in a row before giving up.
	maxRateLimited = 5
	// maxUnavailable is the number of retries after a 502, 503 or 504.
	maxUnavailable = 3
	// maxBody bounds an API answer so a broken server cannot exhaust memory.
	maxBody = 50 << 20
	// maxPages stops a pagination loop that never ends.
	maxPages = 100000
)

// Errors returned by Client.
var (
	// ErrTokenRejected reports a token Moneybird does not accept (401), or
	// one without the scope the request needs (403).
	ErrTokenRejected = errors.New("mbclient: the token was not accepted by Moneybird")
	// ErrNotFound reports a resource Moneybird does not have (404).
	ErrNotFound = errors.New("mbclient: not found in Moneybird")
	// ErrRateLimited reports Moneybird still refusing requests after
	// waiting for its rate limit several times in a row.
	ErrRateLimited = errors.New("mbclient: Moneybird keeps refusing requests over its rate limit")
	// ErrTooLarge reports a download over the size the caller allows.
	ErrTooLarge = errors.New("mbclient: download is larger than allowed")
)

// StatusError is an answer with a status code other than the ones that
// have their own error, with the start of its body to tell what went wrong.
type StatusError struct {
	Status int
	Body   string
}

func (e *StatusError) Error() string {
	if e.Body == "" {
		return fmt.Sprintf("mbclient: Moneybird answered %d %s", e.Status, http.StatusText(e.Status))
	}
	return fmt.Sprintf("mbclient: Moneybird answered %d %s: %s", e.Status, http.StatusText(e.Status), e.Body)
}

// Client calls Moneybird's API with a personal or OAuth bearer token. The
// zero value of each field but Token works; a Client is safe for
// concurrent use and must not be copied after its first request.
type Client struct {
	// BaseURL is the API root; DefaultBaseURL when empty.
	BaseURL string
	// Token is the bearer token sent with every request to BaseURL's host.
	Token string
	// HTTP sends the requests; a client with a 60s timeout when nil.
	HTTP *http.Client
	// Clock tells the time; time.Now when nil.
	Clock func() time.Time
	// Sleep waits for d or until ctx is done; a timer when nil.
	Sleep func(ctx context.Context, d time.Duration) error
	// OnWait, when set, hears of every wait before it starts, so the
	// importer can tell the user why nothing happens until then.
	OnWait func(until time.Time)
	// Limit and Window bound the requests sent in any rolling window;
	// DefaultLimit and DefaultWindow when zero.
	Limit  int
	Window time.Duration

	mu      sync.Mutex
	limiter limiter
	// pausedUntil is when Moneybird said its window resets after it said
	// no requests are left.
	pausedUntil time.Time
}

// Administration is an administration the token can read.
type Administration struct {
	ID, Name, Country, Currency string
}

// Version is a record's id with the version Moneybird gives it, which
// changes whenever the record does.
type Version struct {
	ID      string
	Version int64
}

// Administrations lists the administrations the token can read.
func (c *Client) Administrations(ctx context.Context) ([]Administration, error) {
	resp, err := c.do(ctx, http.MethodGet, c.url("", "administrations", nil), nil, maxBody)
	if err != nil {
		return nil, err
	}
	var list []struct {
		ID       flexID `json:"id"`
		Name     string `json:"name"`
		Country  string `json:"country"`
		Currency string `json:"currency"`
	}
	if err := decode(resp.body, &list); err != nil {
		return nil, err
	}
	out := make([]Administration, len(list))
	for i, a := range list {
		out[i] = Administration{ID: string(a.ID), Name: a.Name, Country: a.Country, Currency: a.Currency}
	}
	return out, nil
}

// List reads every page of a paginated list in the administration, such as
// "contacts" or "sales_invoices.json", following the Link header Moneybird
// sends to the next page. ".json" is added to path when missing.
func (c *Client) List(ctx context.Context, adminID, path string, query url.Values) ([]json.RawMessage, error) {
	q := url.Values{}
	for k, v := range query {
		q[k] = v
	}
	q.Set("per_page", strconv.Itoa(perPage))
	next := c.url(adminID, path, q)
	var all []json.RawMessage
	for page := 0; next != ""; page++ {
		if page == maxPages {
			return nil, fmt.Errorf("mbclient: %s has more than %d pages", path, maxPages)
		}
		resp, err := c.do(ctx, http.MethodGet, next, nil, maxBody)
		if err != nil {
			return nil, err
		}
		var items []json.RawMessage
		if err := decode(resp.body, &items); err != nil {
			return nil, err
		}
		all = append(all, items...)
		if next, err = c.nextLink(next, resp.header); err != nil {
			return nil, err
		}
	}
	return all, nil
}

// Get reads a single resource, or a list Moneybird does not paginate, in
// the administration. ".json" is added to path when missing.
func (c *Client) Get(ctx context.Context, adminID, path string, query url.Values) (json.RawMessage, error) {
	resp, err := c.do(ctx, http.MethodGet, c.url(adminID, path, query), nil, maxBody)
	if err != nil {
		return nil, err
	}
	if !json.Valid(resp.body) {
		return nil, errors.New("mbclient: Moneybird answered something other than JSON")
	}
	return resp.body, nil
}

// SyncIDs lists the id and version of every record of a kind, such as
// "contacts" or "documents/purchase_invoices", from its synchronization
// endpoint; query can filter them as the kind's list does.
func (c *Client) SyncIDs(ctx context.Context, adminID, kind string, query url.Values) ([]Version, error) {
	resp, err := c.do(ctx, http.MethodGet, c.url(adminID, syncPath(kind), query), nil, maxBody)
	if err != nil {
		return nil, err
	}
	// Moneybird documents both fields as an integer or a string.
	var list []struct {
		ID      flexID `json:"id"`
		Version flexID `json:"version"`
	}
	if err := decode(resp.body, &list); err != nil {
		return nil, err
	}
	out := make([]Version, len(list))
	for i, v := range list {
		n, err := strconv.ParseInt(string(v.Version), 10, 64)
		if err != nil {
			return nil, fmt.Errorf("mbclient: version %q of %s %s", v.Version, kind, v.ID)
		}
		out[i] = Version{ID: string(v.ID), Version: n}
	}
	return out, nil
}

// FetchByIDs reads the full records of a kind with the given ids through
// its synchronization endpoint, a hundred per request because Moneybird
// returns no more than that.
func (c *Client) FetchByIDs(ctx context.Context, adminID, kind string, ids []string) ([]json.RawMessage, error) {
	var all []json.RawMessage
	for start := 0; start < len(ids); start += batchSize {
		batch := ids[start:min(start+batchSize, len(ids))]
		body, err := json.Marshal(struct {
			IDs []string `json:"ids"`
		}{batch})
		if err != nil {
			return nil, err
		}
		resp, err := c.do(ctx, http.MethodPost, c.url(adminID, syncPath(kind), nil), body, maxBody)
		if err != nil {
			return nil, err
		}
		var items []json.RawMessage
		if err := decode(resp.body, &items); err != nil {
			return nil, err
		}
		all = append(all, items...)
	}
	return all, nil
}

// Download reads an absolute URL, such as an attachment's, up to maxBytes,
// and returns it with its content type. The token goes only to BaseURL's
// scheme and host: download links can point at file storage elsewhere,
// which must never see it.
func (c *Client) Download(ctx context.Context, rawURL string, maxBytes int64) ([]byte, string, error) {
	u, err := url.Parse(rawURL)
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" {
		return nil, "", fmt.Errorf("mbclient: cannot download %q", rawURL)
	}
	resp, err := c.do(ctx, http.MethodGet, u.String(), nil, maxBytes)
	if err != nil {
		return nil, "", err
	}
	return resp.body, resp.header.Get("Content-Type"), nil
}

// response is what is kept of an answer once its body is read.
type response struct {
	header http.Header
	body   []byte
}

// do sends a request, waiting first for the rate limit, and retries it
// after a 429 or a gateway error; any other answer is final.
func (c *Client) do(ctx context.Context, method, rawURL string, body []byte, maxBytes int64) (*response, error) {
	rateLimited, unavailable := 0, 0
	for {
		if err := c.throttle(ctx); err != nil {
			return nil, err
		}
		status, resp, err := c.send(ctx, method, rawURL, body, maxBytes)
		if err != nil {
			return nil, err
		}
		c.observe(resp.header)
		switch status {
		case http.StatusTooManyRequests:
			rateLimited++
			if rateLimited == maxRateLimited {
				return nil, ErrRateLimited
			}
			c.pause(retryAt(resp.header, c.now()))
			continue
		case http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout:
			unavailable++
			if unavailable > maxUnavailable {
				return nil, statusError(status, resp.body)
			}
			// 2s, 4s, 8s: long enough for a restart, short against an import.
			if err := c.wait(ctx, c.now().Add(time.Second<<unavailable)); err != nil {
				return nil, err
			}
			continue
		}
		switch {
		case status == http.StatusUnauthorized || status == http.StatusForbidden:
			return nil, fmt.Errorf("%w (%d)", ErrTokenRejected, status)
		case status == http.StatusNotFound:
			return nil, ErrNotFound
		case status < 200 || status > 299:
			return nil, statusError(status, resp.body)
		}
		return resp, nil
	}
}

// send makes one request and reads its answer.
func (c *Client) send(ctx context.Context, method, rawURL string, body []byte, maxBytes int64) (int, *response, error) {
	var reader io.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, rawURL, reader)
	if err != nil {
		return 0, nil, fmt.Errorf("mbclient: %w", err)
	}
	if c.ownHost(req.URL) {
		req.Header.Set("Authorization", "Bearer "+c.Token)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", userAgent)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	httpClient := c.HTTP
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 60 * time.Second}
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return 0, nil, fmt.Errorf("mbclient: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxBytes+1))
	if err != nil {
		return 0, nil, fmt.Errorf("mbclient: %w", err)
	}
	if int64(len(raw)) > maxBytes {
		return 0, nil, ErrTooLarge
	}
	return resp.StatusCode, &response{header: resp.Header, body: raw}, nil
}

// throttle waits until Moneybird's pause is over and the rolling window
// has room, then counts the request that is about to go out.
func (c *Client) throttle(ctx context.Context) error {
	for {
		c.mu.Lock()
		now := c.now()
		until := c.pausedUntil
		if !until.After(now) {
			c.limiter.limit, c.limiter.window = c.limits()
			d := c.limiter.delay(now)
			if d <= 0 {
				c.limiter.record(now)
				c.mu.Unlock()
				return nil
			}
			until = now.Add(d)
		}
		c.mu.Unlock()
		// Check again after waiting: another goroutine may have taken the
		// slot, or Moneybird may have asked for a longer pause meanwhile.
		if err := c.wait(ctx, until); err != nil {
			return err
		}
	}
}

// observe reads Moneybird's rate limit headers and pauses until the window
// resets once no requests are left in it.
func (c *Client) observe(h http.Header) {
	left, err := strconv.Atoi(strings.TrimSpace(h.Get("RateLimit-RequestsRemaining")))
	if err != nil || left > 0 {
		return
	}
	now := c.now()
	if reset, ok := epoch(h.Get("RateLimit-Reset")); ok && reset.After(now) {
		c.pause(reset)
		return
	}
	// RateLimit-Remaining is, despite its name, the seconds until the reset.
	if secs, err := strconv.Atoi(strings.TrimSpace(h.Get("RateLimit-Remaining"))); err == nil && secs > 0 {
		c.pause(now.Add(time.Duration(secs) * time.Second))
		return
	}
	_, window := c.limits()
	c.pause(now.Add(window))
}

// pause holds every request back until the time, unless one is already
// held back longer.
func (c *Client) pause(until time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if until.After(c.pausedUntil) {
		c.pausedUntil = until
	}
}

// wait tells OnWait and sleeps until the time, or until ctx is done.
func (c *Client) wait(ctx context.Context, until time.Time) error {
	d := until.Sub(c.now())
	if d <= 0 {
		return ctx.Err()
	}
	if c.OnWait != nil {
		c.OnWait(until)
	}
	if c.Sleep != nil {
		return c.Sleep(ctx, d)
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

func (c *Client) now() time.Time {
	if c.Clock != nil {
		return c.Clock()
	}
	return time.Now()
}

func (c *Client) limits() (int, time.Duration) {
	limit, window := c.Limit, c.Window
	if limit <= 0 {
		limit = DefaultLimit
	}
	if window <= 0 {
		window = DefaultWindow
	}
	return limit, window
}

func (c *Client) base() *url.URL {
	base := c.BaseURL
	if base == "" {
		base = DefaultBaseURL
	}
	u, err := url.Parse(strings.TrimRight(base, "/"))
	if err != nil {
		return &url.URL{}
	}
	return u
}

// ownHost tells whether u is on the API's scheme and host, the only place
// the token may go.
func (c *Client) ownHost(u *url.URL) bool {
	b := c.base()
	return b.Host != "" && strings.EqualFold(u.Host, b.Host) && u.Scheme == b.Scheme
}

// url builds the address of a path inside the administration, or at the
// API root when adminID is empty.
func (c *Client) url(adminID, path string, query url.Values) string {
	path = strings.TrimLeft(path, "/")
	if !strings.HasSuffix(path, ".json") {
		path += ".json"
	}
	s := c.base().String() + "/"
	if adminID != "" {
		s += url.PathEscape(adminID) + "/"
	}
	s += path
	if len(query) > 0 {
		s += "?" + query.Encode()
	}
	return s
}

// nextLink finds the rel="next" address in a Link header, resolved against
// the page it came with. It refuses one on another host, which would
// otherwise receive the token.
func (c *Client) nextLink(current string, h http.Header) (string, error) {
	for _, v := range h.Values("Link") {
		for part := range strings.SplitSeq(v, ",") {
			target, params, ok := strings.Cut(part, ";")
			if !ok || !isNext(params) {
				continue
			}
			target = strings.Trim(strings.TrimSpace(target), "<>")
			base, err := url.Parse(current)
			if err != nil {
				return "", fmt.Errorf("mbclient: %w", err)
			}
			next, err := base.Parse(target)
			if err != nil {
				return "", fmt.Errorf("mbclient: next page %q: %w", target, err)
			}
			if !c.ownHost(next) {
				return "", fmt.Errorf("mbclient: next page %q is not on Moneybird's host", target)
			}
			return next.String(), nil
		}
	}
	return "", nil
}

func isNext(params string) bool {
	for p := range strings.SplitSeq(params, ";") {
		k, v, _ := strings.Cut(strings.TrimSpace(p), "=")
		if strings.EqualFold(k, "rel") && strings.Contains(" "+strings.Trim(v, `"`)+" ", " next ") {
			return true
		}
	}
	return false
}

// retryAt is when to try again after a 429. Moneybird's Retry-After is the
// reset time in epoch seconds, but a small number is read as seconds from
// now, as HTTP defines it, in case Moneybird ever sends that.
func retryAt(h http.Header, now time.Time) time.Time {
	until := now.Add(time.Minute)
	if v := strings.TrimSpace(h.Get("Retry-After")); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil {
			if n > 1_000_000_000 {
				until = time.Unix(n, 0)
			} else {
				until = now.Add(time.Duration(n) * time.Second)
			}
		} else if t, err := http.ParseTime(v); err == nil {
			until = t
		}
	} else if reset, ok := epoch(h.Get("RateLimit-Reset")); ok {
		until = reset
	}
	// A reset already past, from a skewed clock, must not turn into a burst
	// of retries.
	if earliest := now.Add(time.Second); until.Before(earliest) {
		until = earliest
	}
	return until
}

func epoch(v string) (time.Time, bool) {
	n, err := strconv.ParseInt(strings.TrimSpace(v), 10, 64)
	if err != nil || n <= 0 {
		return time.Time{}, false
	}
	return time.Unix(n, 0), true
}

func syncPath(kind string) string {
	return strings.Trim(strings.TrimSuffix(kind, ".json"), "/") + "/synchronization.json"
}

func statusError(status int, body []byte) error {
	excerpt := strings.Join(strings.Fields(string(body)), " ")
	if r := []rune(excerpt); len(r) > 200 {
		excerpt = string(r[:200]) + "…"
	}
	return &StatusError{Status: status, Body: excerpt}
}

func decode(raw []byte, out any) error {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	if err := dec.Decode(out); err != nil {
		return fmt.Errorf("mbclient: read answer: %w", err)
	}
	return nil
}

// flexID reads a JSON number or string as its text, since Moneybird sends
// its 18-digit ids either way and a float64 would round them.
type flexID string

func (f *flexID) UnmarshalJSON(b []byte) error {
	if len(b) > 0 && b[0] == '"' {
		var s string
		if err := json.Unmarshal(b, &s); err != nil {
			return err
		}
		*f = flexID(s)
		return nil
	}
	var n json.Number
	if err := json.Unmarshal(b, &n); err != nil {
		return err
	}
	*f = flexID(n)
	return nil
}
