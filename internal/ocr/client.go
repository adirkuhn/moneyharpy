package ocr

import (
	"bytes"
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// ErrNoPage means the page asked for does not exist.
var ErrNoPage = errors.New("no such page")

// Timeouts of calls to the service. Reading a scan of many pages with OCR
// takes a while; rendering one page does not.
const (
	readTimeout   = 3 * time.Minute
	renderTimeout = 30 * time.Second
	pingTimeout   = 3 * time.Second
)

// maxResponse caps what the service may answer.
const maxResponse = 64 << 20

// Client calls the OCR service (cmd/ocrd) over HTTP.
type Client struct {
	base string
	http *http.Client
}

// New returns a client for the service at baseURL, such as
// http://ocr:8090. Without a URL, every call fails with ErrUnavailable.
func New(baseURL string) Service {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if baseURL == "" {
		return Unavailable{}
	}
	return &Client{base: baseURL, http: &http.Client{}}
}

// Read implements Service.
func (c *Client) Read(ctx context.Context, f File) (Text, error) {
	ctx, cancel := context.WithTimeout(ctx, readTimeout)
	defer cancel()
	res, err := c.post(ctx, "/read", f)
	if err != nil {
		return Text{}, err
	}
	defer func() { _ = res.Body.Close() }()
	if err := statusError(res); err != nil {
		return Text{}, err
	}
	var body struct {
		Text     string `json:"text"`
		Method   string `json:"method"`
		Pages    int    `json:"pages"`
		Embedded []struct {
			Name        string `json:"name"`
			ContentType string `json:"content_type"`
			Data        []byte `json:"data"`
		} `json:"embedded"`
		Alternatives []struct {
			Text string `json:"text"`
		} `json:"alternatives"`
	}
	if err := json.NewDecoder(io.LimitReader(res.Body, maxResponse)).Decode(&body); err != nil {
		return Text{}, fmt.Errorf("%w: read response: %w", ErrUnavailable, err)
	}
	t := Text{Text: body.Text, Method: body.Method, Pages: body.Pages}
	for _, e := range body.Embedded {
		t.Embedded = append(t.Embedded, File{Name: e.Name, ContentType: e.ContentType, Data: e.Data})
	}
	for _, a := range body.Alternatives {
		t.Alternatives = append(t.Alternatives, a.Text)
	}
	return t, nil
}

// Render implements Service.
func (c *Client) Render(ctx context.Context, f File, page int) (Page, error) {
	return c.render(ctx, f, page, 0)
}

// RenderAt implements Service.
func (c *Client) RenderAt(ctx context.Context, f File, page, maxSide int) (Page, error) {
	if maxSide < 1 {
		return Page{}, fmt.Errorf("ocr: render with a side of %d pixels", maxSide)
	}
	return c.render(ctx, f, page, maxSide)
}

// render renders a page as a preview, or with its longest side maxSide.
func (c *Client) render(ctx context.Context, f File, page, maxSide int) (Page, error) {
	if page < 1 {
		return Page{}, ErrNoPage
	}
	ctx, cancel := context.WithTimeout(ctx, renderTimeout)
	defer cancel()
	q := url.Values{"page": {strconv.Itoa(page)}}
	if maxSide > 0 {
		q.Set("max_side", strconv.Itoa(maxSide))
	}
	res, err := c.post(ctx, "/render?"+q.Encode(), f)
	if err != nil {
		return Page{}, err
	}
	defer func() { _ = res.Body.Close() }()
	count, _ := strconv.Atoi(res.Header.Get("X-Page-Count"))
	if res.StatusCode == http.StatusNotFound {
		return Page{Count: count}, ErrNoPage
	}
	if err := statusError(res); err != nil {
		return Page{}, err
	}
	data, err := io.ReadAll(io.LimitReader(res.Body, maxResponse))
	if err != nil {
		return Page{}, fmt.Errorf("%w: %w", ErrUnavailable, err)
	}
	return Page{PNG: data, Count: max(count, 1)}, nil
}

// Ping implements Service.
func (c *Client) Ping(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, pingTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.base+"/healthz", nil)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrUnavailable, err)
	}
	res, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrUnavailable, err)
	}
	_ = res.Body.Close()
	return statusError(res)
}

func (c *Client) post(ctx context.Context, path string, f File) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.base+path, bytes.NewReader(f.Data))
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrUnavailable, err)
	}
	req.Header.Set("Content-Type", cmp.Or(f.ContentType, "application/octet-stream"))
	if f.Name != "" {
		req.Header.Set("X-Filename", url.PathEscape(f.Name))
	}
	res, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrUnavailable, err)
	}
	return res, nil
}

// statusError maps the service's answer to ErrUnsupported for files it
// cannot handle and ErrUnavailable for everything else that went wrong.
func statusError(res *http.Response) error {
	switch {
	case res.StatusCode >= 200 && res.StatusCode < 300:
		return nil
	case res.StatusCode == http.StatusUnsupportedMediaType, res.StatusCode == http.StatusUnprocessableEntity,
		res.StatusCode == http.StatusRequestEntityTooLarge:
		return fmt.Errorf("%w (%s)", ErrUnsupported, res.Status)
	default:
		return fmt.Errorf("%w: the service answered %s", ErrUnavailable, res.Status)
	}
}

// Unavailable is the Service of an installation without the OCR service.
type Unavailable struct{}

// Read implements Service.
func (Unavailable) Read(context.Context, File) (Text, error) { return Text{}, ErrUnavailable }

// Render implements Service.
func (Unavailable) Render(context.Context, File, int) (Page, error) { return Page{}, ErrUnavailable }

// RenderAt implements Service.
func (Unavailable) RenderAt(context.Context, File, int, int) (Page, error) {
	return Page{}, ErrUnavailable
}

// Ping implements Service.
func (Unavailable) Ping(context.Context) error { return ErrUnavailable }
