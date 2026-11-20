package mbclient

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

const token = "test-token"

// clock is a fake time source whose Sleep only moves the time forward, so
// tests can check every wait without really waiting.
type clock struct {
	mu     sync.Mutex
	now    time.Time
	slept  []time.Duration
	waited []time.Time
}

func newClock() *clock { return &clock{now: time.Unix(1_800_000_000, 0)} }

func (c *clock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *clock) Sleep(ctx context.Context, d time.Duration) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.slept = append(c.slept, d)
	c.now = c.now.Add(d)
	return nil
}

func (c *clock) OnWait(until time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.waited = append(c.waited, until)
}

// fake starts a Moneybird stand-in that checks the token and answers
// "METHOD /path" routes.
func fake(t *testing.T, routes map[string]http.HandlerFunc) (*Client, *clock) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+token {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"error":"Token is invalid"}`))
			return
		}
		h, ok := routes[r.Method+" "+r.URL.Path]
		if !ok {
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		h(w, r)
	}))
	t.Cleanup(srv.Close)
	clk := newClock()
	return &Client{BaseURL: srv.URL + "/api/v2", Token: token, HTTP: srv.Client(), Clock: clk.Now, Sleep: clk.Sleep, OnWait: clk.OnWait}, clk
}

func TestAdministrationsSendsTokenAndHeaders(t *testing.T) {
	c, _ := fake(t, map[string]http.HandlerFunc{
		"GET /api/v2/administrations.json": func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get("Accept") != "application/json" || r.Header.Get("User-Agent") != "MoneyHarpy importer" {
				t.Errorf("headers = %v", r.Header)
			}
			_, _ = w.Write([]byte(`[{"id":"179517248071352172","name":"Example Shop B.V.","language":"nl","currency":"EUR","country":"NL"},
				{"id":42,"name":"Example Studio","currency":"USD","country":"US"}]`))
		},
	})
	got, err := c.Administrations(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	want := []Administration{
		{ID: "179517248071352172", Name: "Example Shop B.V.", Country: "NL", Currency: "EUR"},
		{ID: "42", Name: "Example Studio", Country: "US", Currency: "USD"},
	}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Errorf("Administrations() = %+v, want %+v", got, want)
	}
}

func TestRejectedTokenIsErrTokenRejected(t *testing.T) {
	c, _ := fake(t, nil)
	c.Token = "wrong"
	if _, err := c.Administrations(context.Background()); !errors.Is(err, ErrTokenRejected) {
		t.Errorf("err = %v, want ErrTokenRejected", err)
	}
}

func TestErrorsByStatus(t *testing.T) {
	c, _ := fake(t, map[string]http.HandlerFunc{
		"GET /api/v2/1/contacts/9.json": func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"error":"Record not found"}`))
		},
		"GET /api/v2/1/forbidden.json": func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusForbidden)
		},
		"GET /api/v2/1/broken.json": func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusUnprocessableEntity)
			_, _ = w.Write([]byte(`{"error":"per_page does not have a valid value"}`))
		},
	})
	ctx := context.Background()
	if _, err := c.Get(ctx, "1", "contacts/9", nil); !errors.Is(err, ErrNotFound) {
		t.Errorf("404: err = %v, want ErrNotFound", err)
	}
	if _, err := c.Get(ctx, "1", "forbidden.json", nil); !errors.Is(err, ErrTokenRejected) {
		t.Errorf("403: err = %v, want ErrTokenRejected", err)
	}
	_, err := c.Get(ctx, "1", "broken.json", nil)
	var se *StatusError
	if !errors.As(err, &se) || se.Status != 422 || !strings.Contains(err.Error(), "per_page does not have a valid value") {
		t.Errorf("422: err = %v, want a StatusError with the body", err)
	}
}

func TestGetReturnsTheBody(t *testing.T) {
	c, _ := fake(t, map[string]http.HandlerFunc{
		"GET /api/v2/1/ledger_accounts.json": func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Query().Get("filter") != "x" {
				t.Errorf("query = %v", r.URL.Query())
			}
			_, _ = w.Write([]byte(`[{"id":"1","name":"Revenue"}]`))
		},
	})
	got, err := c.Get(context.Background(), "1", "/ledger_accounts.json", url.Values{"filter": {"x"}})
	if err != nil || string(got) != `[{"id":"1","name":"Revenue"}]` {
		t.Errorf("Get() = %s, %v", got, err)
	}
}

func TestListFollowsTheLinkHeader(t *testing.T) {
	var srvURL string
	c, _ := fake(t, map[string]http.HandlerFunc{
		"GET /api/v2/7/contacts.json": func(w http.ResponseWriter, r *http.Request) {
			q := r.URL.Query()
			if q.Get("per_page") != "100" || q.Get("query") != "shop" {
				t.Errorf("query = %v, want per_page=100 and the caller's query", q)
			}
			switch q.Get("page") {
			case "":
				w.Header().Set("Link", fmt.Sprintf(`<%s/api/v2/7/contacts.json?page=2&per_page=100&query=shop>; rel="next", <%s/api/v2/7/contacts.json?page=2&per_page=100&query=shop>; rel="last"`, srvURL, srvURL))
				_, _ = w.Write([]byte(`[{"id":"1"},{"id":"2"}]`))
			case "2":
				w.Header().Set("Link", fmt.Sprintf(`<%s/api/v2/7/contacts.json?page=1&per_page=100&query=shop>; rel="prev"`, srvURL))
				_, _ = w.Write([]byte(`[{"id":"3"}]`))
			default:
				t.Errorf("page %q", q.Get("page"))
			}
		},
	})
	srvURL = strings.TrimSuffix(c.BaseURL, "/api/v2")
	got, err := c.List(context.Background(), "7", "contacts", url.Values{"query": {"shop"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 || string(got[2]) != `{"id":"3"}` {
		t.Errorf("List() = %s", got)
	}
}

func TestListRefusesANextLinkToAnotherHost(t *testing.T) {
	c, _ := fake(t, map[string]http.HandlerFunc{
		"GET /api/v2/7/contacts.json": func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Link", `<https://elsewhere.example/steal?page=2>; rel="next"`)
			_, _ = w.Write([]byte(`[{"id":"1"}]`))
		},
	})
	if _, err := c.List(context.Background(), "7", "contacts.json", nil); err == nil {
		t.Error("List() followed a link to another host")
	}
}

func TestSyncIDsReadsIDsAndVersions(t *testing.T) {
	c, _ := fake(t, map[string]http.HandlerFunc{
		"GET /api/v2/7/documents/purchase_invoices/synchronization.json": func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Query().Get("filter") != "period:this_year" {
				t.Errorf("query = %v", r.URL.Query())
			}
			_, _ = w.Write([]byte(`[{"id":"504315184640164883","version":1795172460},{"id":12,"version":"1795172461"}]`))
		},
	})
	got, err := c.SyncIDs(context.Background(), "7", "documents/purchase_invoices", url.Values{"filter": {"period:this_year"}})
	if err != nil {
		t.Fatal(err)
	}
	want := []Version{{ID: "504315184640164883", Version: 1795172460}, {ID: "12", Version: 1795172461}}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Errorf("SyncIDs() = %+v, want %+v", got, want)
	}
}

func TestFetchByIDsInBatchesOfAHundred(t *testing.T) {
	var batches []int
	c, _ := fake(t, map[string]http.HandlerFunc{
		"POST /api/v2/7/contacts/synchronization.json": func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get("Content-Type") != "application/json" {
				t.Errorf("Content-Type = %q", r.Header.Get("Content-Type"))
			}
			var body struct {
				IDs []string `json:"ids"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatal(err)
			}
			batches = append(batches, len(body.IDs))
			out := make([]string, len(body.IDs))
			for i, id := range body.IDs {
				out[i] = `{"id":"` + id + `"}`
			}
			_, _ = w.Write([]byte("[" + strings.Join(out, ",") + "]"))
		},
	})
	ids := make([]string, 250)
	for i := range ids {
		ids[i] = strconv.Itoa(i + 1)
	}
	got, err := c.FetchByIDs(context.Background(), "7", "contacts", ids)
	if err != nil {
		t.Fatal(err)
	}
	if fmt.Sprint(batches) != "[100 100 50]" {
		t.Errorf("batches = %v, want [100 100 50]", batches)
	}
	if len(got) != 250 || string(got[249]) != `{"id":"250"}` {
		t.Errorf("FetchByIDs() returned %d records", len(got))
	}
	if got, err := c.FetchByIDs(context.Background(), "7", "contacts", nil); err != nil || got != nil {
		t.Errorf("FetchByIDs(no ids) = %v, %v; want nothing without a request", got, err)
	}
}

func TestTooManyRequestsWaitsForRetryAfterThenRetries(t *testing.T) {
	calls := 0
	var clk *clock
	c, clk := fake(t, map[string]http.HandlerFunc{
		"GET /api/v2/7/contacts.json": func(w http.ResponseWriter, _ *http.Request) {
			calls++
			if calls == 1 {
				w.Header().Set("Retry-After", strconv.FormatInt(clk.Now().Add(90*time.Second).Unix(), 10))
				w.WriteHeader(http.StatusTooManyRequests)
				_, _ = w.Write([]byte(`"Retry later"`))
				return
			}
			_, _ = w.Write([]byte(`[{"id":"1"}]`))
		},
	})
	start := clk.Now()
	got, err := c.List(context.Background(), "7", "contacts", nil)
	if err != nil || len(got) != 1 {
		t.Fatalf("List() = %s, %v", got, err)
	}
	if calls != 2 || fmt.Sprint(clk.slept) != "[1m30s]" {
		t.Errorf("calls = %d, slept %v; want a retry after 90s", calls, clk.slept)
	}
	if len(clk.waited) != 1 || !clk.waited[0].Equal(start.Add(90*time.Second)) {
		t.Errorf("OnWait got %v, want the Retry-After time", clk.waited)
	}
}

func TestRetryAfterForms(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	cases := []struct {
		name   string
		header http.Header
		want   time.Time
	}{
		{"epoch", http.Header{"Retry-After": {"1800000120"}}, now.Add(120 * time.Second)},
		{"seconds", http.Header{"Retry-After": {"30"}}, now.Add(30 * time.Second)},
		{"reset", http.Header{"Ratelimit-Reset": {"1800000200"}}, now.Add(200 * time.Second)},
		{"nothing", http.Header{}, now.Add(time.Minute)},
		{"past", http.Header{"Retry-After": {"1700000000"}}, now.Add(time.Second)},
	}
	for _, tc := range cases {
		if got := retryAt(tc.header, now); !got.Equal(tc.want) {
			t.Errorf("%s: retryAt = %v, want %v", tc.name, got, tc.want)
		}
	}
}

func TestGivesUpAfterFiveTooManyRequests(t *testing.T) {
	calls := 0
	c, clk := fake(t, map[string]http.HandlerFunc{
		"GET /api/v2/7/contacts.json": func(w http.ResponseWriter, _ *http.Request) {
			calls++
			w.Header().Set("Retry-After", "10")
			w.WriteHeader(http.StatusTooManyRequests)
		},
	})
	_, err := c.List(context.Background(), "7", "contacts", nil)
	if !errors.Is(err, ErrRateLimited) {
		t.Errorf("err = %v, want ErrRateLimited", err)
	}
	if calls != 5 || len(clk.slept) != 4 {
		t.Errorf("calls = %d, waits = %d; want 5 tries with a wait between each", calls, len(clk.slept))
	}
}

func TestNoRequestsRemainingPausesUntilReset(t *testing.T) {
	calls := 0
	var clk *clock
	c, clk := fake(t, map[string]http.HandlerFunc{
		"GET /api/v2/7/contacts/1.json": func(w http.ResponseWriter, _ *http.Request) {
			calls++
			w.Header().Set("RateLimit-Limit", "150")
			w.Header().Set("RateLimit-RequestsRemaining", "0")
			w.Header().Set("RateLimit-Remaining", "120")
			w.Header().Set("RateLimit-Reset", strconv.FormatInt(clk.Now().Add(120*time.Second).Unix(), 10))
			_, _ = w.Write([]byte(`{"id":"1"}`))
		},
	})
	start := clk.Now()
	ctx := context.Background()
	if _, err := c.Get(ctx, "7", "contacts/1", nil); err != nil {
		t.Fatal(err)
	}
	if len(clk.slept) != 0 {
		t.Fatalf("waited before the first request: %v", clk.slept)
	}
	if _, err := c.Get(ctx, "7", "contacts/1", nil); err != nil {
		t.Fatal(err)
	}
	if fmt.Sprint(clk.slept) != "[2m0s]" || len(clk.waited) != 1 || !clk.waited[0].Equal(start.Add(2*time.Minute)) {
		t.Errorf("slept %v, OnWait %v; want a pause until the reset", clk.slept, clk.waited)
	}
}

func TestRollingWindowHoldsBackTheRequestOverTheLimit(t *testing.T) {
	calls := 0
	c, clk := fake(t, map[string]http.HandlerFunc{
		"GET /api/v2/7/contacts/1.json": func(w http.ResponseWriter, _ *http.Request) {
			calls++
			_, _ = w.Write([]byte(`{}`))
		},
	})
	for i := range 151 {
		if _, err := c.Get(context.Background(), "7", "contacts/1", nil); err != nil {
			t.Fatalf("request %d: %v", i+1, err)
		}
	}
	if calls != 151 || fmt.Sprint(clk.slept) != "[5m0s]" {
		t.Errorf("calls = %d, slept %v; want only the 151st to wait a full window", calls, clk.slept)
	}
}

func TestBadGatewayIsRetried(t *testing.T) {
	calls := 0
	c, clk := fake(t, map[string]http.HandlerFunc{
		"GET /api/v2/7/contacts/1.json": func(w http.ResponseWriter, _ *http.Request) {
			calls++
			if calls < 3 {
				w.WriteHeader(http.StatusServiceUnavailable)
				return
			}
			_, _ = w.Write([]byte(`{}`))
		},
	})
	if _, err := c.Get(context.Background(), "7", "contacts/1", nil); err != nil {
		t.Fatal(err)
	}
	if calls != 3 || fmt.Sprint(clk.slept) != "[2s 4s]" {
		t.Errorf("calls = %d, slept %v", calls, clk.slept)
	}
}

func TestDownloadSendsTheTokenOnlyToMoneybird(t *testing.T) {
	foreign := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if a := r.Header.Get("Authorization"); a != "" {
			t.Errorf("token leaked to another host: %q", a)
		}
		w.Header().Set("Content-Type", "application/pdf")
		_, _ = w.Write([]byte("%PDF-1.7 elsewhere"))
	}))
	t.Cleanup(foreign.Close)
	c, _ := fake(t, map[string]http.HandlerFunc{
		"GET /api/v2/7/documents/receipts/1/attachments/2/download": func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/pdf")
			_, _ = w.Write([]byte("%PDF-1.7 own"))
		},
	})
	ctx := context.Background()
	body, ctype, err := c.Download(ctx, c.BaseURL+"/7/documents/receipts/1/attachments/2/download", 1<<20)
	if err != nil || string(body) != "%PDF-1.7 own" || ctype != "application/pdf" {
		t.Errorf("own host: %q, %q, %v", body, ctype, err)
	}
	body, _, err = c.Download(ctx, foreign.URL+"/file.pdf", 1<<20)
	if err != nil || string(body) != "%PDF-1.7 elsewhere" {
		t.Errorf("foreign host: %q, %v", body, err)
	}
	if _, _, err := c.Download(ctx, foreign.URL+"/file.pdf", 4); !errors.Is(err, ErrTooLarge) {
		t.Errorf("over the size: err = %v, want ErrTooLarge", err)
	}
	if _, _, err := c.Download(ctx, "file:///etc/passwd", 1<<20); err == nil {
		t.Error("Download accepted a non-HTTP URL")
	}
}

func TestCancelDuringAWaitStops(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Retry-After", "3600")
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = io.WriteString(w, `"Retry later"`)
	}))
	t.Cleanup(srv.Close)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	// The real Sleep: the cancel must cut an hour's wait short.
	c := &Client{BaseURL: srv.URL, Token: token, HTTP: srv.Client(), OnWait: func(time.Time) { cancel() }}
	done := make(chan error, 1)
	go func() {
		_, err := c.Get(ctx, "7", "contacts/1", nil)
		done <- err
	}()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Errorf("err = %v, want context.Canceled", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the wait ignored the cancel")
	}
}

func TestRedirectsLeaveTheTokenBehind(t *testing.T) {
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/signed/file.pdf" {
			gotAuth = r.Header.Get("Authorization")
			_, _ = w.Write([]byte("%PDF"))
			return
		}
		http.Redirect(w, r, "/signed/file.pdf", http.StatusFound)
	}))
	defer srv.Close()
	c := &Client{BaseURL: srv.URL, Token: "secret", HTTP: srv.Client()}
	data, _, err := c.Download(context.Background(), srv.URL+"/123/sales_invoices/1/download_pdf", 1<<20)
	if err != nil || string(data) != "%PDF" {
		t.Fatalf("Download() = %q, %v", data, err)
	}
	if gotAuth != "" {
		t.Errorf("the token followed the redirect: %q", gotAuth)
	}
}
