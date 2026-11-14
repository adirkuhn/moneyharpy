package kvkweb

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/adirkuhn/moneyharpy/internal/administration"
	"github.com/adirkuhn/moneyharpy/internal/ratelimit"
	"github.com/adirkuhn/moneyharpy/internal/settings"
	"github.com/google/go-cmp/cmp"
)

type staticSettings struct{ inst settings.Instance }

const envConfigBody = `window.__ENV = {"NEXT_PUBLIC_SEARCH_V2_GENERAL_SEARCH_PROFILE_ID":"test-profile-id"};`

func (s staticSettings) Instance(context.Context) (settings.Instance, error) { return s.inst, nil }

func client(t *testing.T, url string, enabled bool) *Client {
	t.Helper()
	s := staticSettings{inst: settings.Instance{KVK: settings.KVK{WebsiteSearch: enabled}}}
	return New(s, url, url+"/config.env.js", http.DefaultClient, ratelimit.New(1000, time.Minute))
}

// serve answers like the KVK website search, checking what is asked.
func serve(t *testing.T, handler func(w http.ResponseWriter)) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/config.env.js" {
			w.Header().Set("Content-Type", "application/javascript")
			_, _ = w.Write([]byte(envConfigBody))
			return
		}
		if got := r.Header.Get("Profileid"); got != "test-profile-id" {
			t.Errorf("Profileid = %q, want %q", got, "test-profile-id")
		}
		q := r.URL.Query()
		if got := q.Get("q"); got != "example & co" {
			t.Errorf("q = %q, want the query", got)
		}
		for key, want := range map[string]string{"site": "handelsregister", "inschrijvingsstatus": "ingeschreven", "size": "10", "start": "0", "language": "nl"} {
			if got := q.Get(key); got != want {
				t.Errorf("%s = %q, want %q", key, got, want)
			}
		}
		if ua := r.Header.Get("User-Agent"); ua != userAgent {
			t.Errorf("User-Agent = %q, want %q", ua, userAgent)
		}
		handler(w)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func reply(status int, contentType, body string) func(http.ResponseWriter) {
	return func(w http.ResponseWriter) {
		w.Header().Set("Content-Type", contentType)
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}
}

const results = `{"data":{"startPage":0,"numberOfHits":5,"itemsPerPage":10,"items":[
	{"set":"hr-handelsregister","kvkNummer":"12345678","rechtsvormCode":"BV","actief":true,"vestiging":false,"naam":"Example Shop B.V.","postlocatie":{}},
	{"set":"hr-handelsregister","kvkNummer":"12345678","vestigingsnummer":"000000000002","actief":true,"vestiging":true,"naam":"Example Shop B.V.","inschrijvingstype":"Nevenvestiging",
		"bezoeklocatie":{"straat":"Kalverstraat","huisnummer":"10","postcode":"1012AB","plaats":"Amsterdam"}},
	{"set":"hr-handelsregister","kvkNummer":"12345678","vestigingsnummer":"000000000001","actief":true,"vestiging":true,"naam":"Example Shop B.V.","inschrijvingstype":"Hoofdvestiging",
		"bezoeklocatie":{"straat":"Dam","huisnummer":"1","huisnummerToevoeging":"A","postcode":"1012AB","plaats":"Amsterdam"}},
	{"set":"hr-handelsregister","kvkNummer":"87654321","vestigingsnummer":"000000000003","actief":true,"vestiging":true,"naam":"Example Bakery","inschrijvingstype":"Hoofdvestiging",
		"bezoeklocatie":{"straat":"Voorbeeldweg","huisnummer":12,"huisnummerToevoeging":"bis","postcode":"9999 ZZ","plaats":"Voorbeeldstad"}},
	{"set":"hr-handelsregister","kvkNummer":"11223344","actief":true,"vestiging":false,"naam":"Example Foundation"}
],"keyMatches":[]}}`

func TestSearchMapsBranchesOnePerCompany(t *testing.T) {
	srv := serve(t, reply(http.StatusOK, "application/json; charset=utf-8", results))
	got, err := client(t, srv.URL, true).SearchCompanies(context.Background(), "example & co")
	if err != nil {
		t.Fatalf("SearchCompanies() error = %v", err)
	}
	want := []administration.CompanyMatch{
		{Name: "Example Shop B.V.", ChamberOfCommerce: "12345678", Address1: "Dam 1A", Zipcode: "1012AB", City: "Amsterdam"},
		{Name: "Example Bakery", ChamberOfCommerce: "87654321", Address1: "Voorbeeldweg 12-bis", Zipcode: "9999ZZ", City: "Voorbeeldstad"},
		{Name: "Example Foundation", ChamberOfCommerce: "11223344"},
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("SearchCompanies() diff (-want +got):\n%s", diff)
	}
}

func TestSearchWithoutHitsIsEmpty(t *testing.T) {
	srv := serve(t, reply(http.StatusOK, "application/json", `{"data":{"startPage":0,"numberOfHits":0,"itemsPerPage":10,"items":[],"keyMatches":[]}}`))
	got, err := client(t, srv.URL, true).SearchCompanies(context.Background(), "example & co")
	if err != nil || len(got) != 0 {
		t.Errorf("SearchCompanies() = %v, %v, want none", got, err)
	}
}

func TestSearchIsUnavailableUntilEnabled(t *testing.T) {
	if _, err := client(t, "http://unused.invalid", false).SearchCompanies(context.Background(), "x"); !errors.Is(err, administration.ErrLookupUnavailable) {
		t.Errorf("SearchCompanies() error = %v, want ErrLookupUnavailable", err)
	}
}

func TestSearchFailures(t *testing.T) {
	tests := []struct {
		name    string
		handler func(http.ResponseWriter)
		want    error
	}{
		{"server error", reply(http.StatusInternalServerError, "text/plain", "oops"), administration.ErrRegisterUnavailable},
		{"bad gateway", reply(http.StatusBadGateway, "text/html", "<html>bad gateway</html>"), administration.ErrRegisterUnavailable},
		{"challenge behind a 503", reply(http.StatusServiceUnavailable, "text/html", "<html><title>Just a moment...</title><div id=\"challenge-platform\"></div></html>"), administration.ErrRegisterBlocked},
		{"a company named like a challenge is a result", reply(http.StatusOK, "application/json", `{"data":{"items":[{"kvkNummer":"12345678","naam":"Captcha B.V.","vestiging":false}]}}`), nil},
		{"attention required behind a 503", reply(http.StatusServiceUnavailable, "text/html", "<html><title>Attention Required!</title></html>"), administration.ErrRegisterBlocked},
		{"forbidden", reply(http.StatusForbidden, "text/html", "<html>Access denied</html>"), administration.ErrRegisterBlocked},
		{"too many requests", reply(http.StatusTooManyRequests, "application/json", `{}`), administration.ErrRegisterBlocked},
		{"challenge page", reply(http.StatusOK, "text/html", "<!DOCTYPE html><html><title>Just a moment...</title></html>"), administration.ErrRegisterBlocked},
		{"html without content type", reply(http.StatusOK, "", "<html><body>captcha</body></html>"), administration.ErrRegisterBlocked},
		{"malformed json", reply(http.StatusOK, "application/json", `{"data":`), administration.ErrRegisterBlocked},
		{"unexpected json", reply(http.StatusOK, "application/json", `{"message":"moved"}`), administration.ErrRegisterBlocked},
		{"not found", reply(http.StatusNotFound, "application/json", `{}`), administration.ErrRegisterBlocked},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := serve(t, tt.handler)
			_, err := client(t, srv.URL, true).SearchCompanies(context.Background(), "example & co")
			if !errors.Is(err, tt.want) {
				t.Errorf("SearchCompanies() error = %v, want %v", err, tt.want)
			}
		})
	}
}

func TestSearchTimesOut(t *testing.T) {
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { <-release }))
	t.Cleanup(srv.Close)
	t.Cleanup(func() { close(release) })
	c := client(t, srv.URL, true)
	c.timeout = 50 * time.Millisecond
	if _, err := c.SearchCompanies(context.Background(), "example"); !errors.Is(err, administration.ErrRegisterUnavailable) {
		t.Errorf("SearchCompanies() error = %v, want ErrRegisterUnavailable", err)
	}
}

func TestSearchServerDown(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	url := srv.URL
	srv.Close()
	if _, err := client(t, url, true).SearchCompanies(context.Background(), "example"); !errors.Is(err, administration.ErrRegisterUnavailable) {
		t.Errorf("SearchCompanies() error = %v, want ErrRegisterUnavailable", err)
	}
}

func TestSearchIsRateLimitedPerInstance(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/config.env.js" {
			_, _ = w.Write([]byte(envConfigBody))
			return
		}
		calls++
		reply(http.StatusOK, "application/json", `{"data":{"items":[]}}`)(w)
	}))
	t.Cleanup(srv.Close)
	s := staticSettings{inst: settings.Instance{KVK: settings.KVK{WebsiteSearch: true}}}
	c := New(s, srv.URL, srv.URL+"/config.env.js", http.DefaultClient, ratelimit.New(2, time.Minute))
	for range 2 {
		if _, err := c.SearchCompanies(context.Background(), "example"); err != nil {
			t.Fatalf("SearchCompanies() error = %v", err)
		}
	}
	if _, err := c.SearchCompanies(context.Background(), "example"); !errors.Is(err, administration.ErrRegisterBusy) {
		t.Errorf("third SearchCompanies() error = %v, want ErrRegisterBusy", err)
	}
	if calls != 2 {
		t.Errorf("KVK was called %d times, want 2", calls)
	}
}

func TestCheckWorksWhileDisabled(t *testing.T) {
	var asked string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/config.env.js" {
			_, _ = w.Write([]byte(envConfigBody))
			return
		}
		if got := r.Header.Get("Profileid"); got != "test-profile-id" {
			t.Errorf("Profileid = %q, want %q", got, "test-profile-id")
		}
		asked = r.URL.Query().Get("q")
		reply(http.StatusOK, "application/json", results)(w)
	}))
	t.Cleanup(srv.Close)
	if err := client(t, srv.URL, false).Check(context.Background()); err != nil {
		t.Errorf("Check() error = %v", err)
	}
	if asked == "" || strings.ContainsAny(asked, "@") {
		t.Errorf("Check() searched %q, want a fixed neutral term", asked)
	}
}

func TestCheckReportsBlocked(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/config.env.js" {
			_, _ = w.Write([]byte(envConfigBody))
			return
		}
		reply(http.StatusForbidden, "text/html", "<html>blocked</html>")(w)
	}))
	t.Cleanup(srv.Close)
	if err := client(t, srv.URL, false).Check(context.Background()); !errors.Is(err, administration.ErrRegisterBlocked) {
		t.Errorf("Check() error = %v, want ErrRegisterBlocked", err)
	}
}

func TestExtractProfileID(t *testing.T) {
	tests := []struct {
		name    string
		body    string
		want    string
		wantErr bool
	}{
		{
			name: "valid single line",
			body: `window.__ENV = {"NEXT_PUBLIC_SEARCH_V2_GENERAL_SEARCH_PROFILE_ID":"5C10A89D-635E-49CC-94B8-042DD533B64A"};`,
			want: "5C10A89D-635E-49CC-94B8-042DD533B64A",
		},
		{
			name: "valid multiline with other fields",
			body: `window.__ENV = {
				"FOO": "bar",
				"NEXT_PUBLIC_SEARCH_V2_GENERAL_SEARCH_PROFILE_ID": "abc-123",
				"NESTED": {"key": 123}
			};`,
			want: "abc-123",
		},
		{
			name:    "missing json braces",
			body:    `window.__ENV = "invalid";`,
			wantErr: true,
		},
		{
			name:    "empty json object",
			body:    `window.__ENV = {};`,
			wantErr: true,
		},
		{
			name:    "missing target key",
			body:    `window.__ENV = {"OTHER_KEY": "123"};`,
			wantErr: true,
		},
		{
			name:    "malformed json",
			body:    `window.__ENV = {invalid json};`,
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := extractProfileID([]byte(tt.body))
			if (err != nil) != tt.wantErr {
				t.Fatalf("extractProfileID() error = %v, wantErr = %v", err, tt.wantErr)
			}
			if got != tt.want {
				t.Errorf("extractProfileID() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestProfileIDCached(t *testing.T) {
	configCalls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/config.env.js" {
			configCalls++
			w.Header().Set("Content-Type", "application/javascript")
			_, _ = w.Write([]byte(envConfigBody))
			return
		}
		if got := r.Header.Get("Profileid"); got != "test-profile-id" {
			t.Errorf("Profileid = %q, want %q", got, "test-profile-id")
		}
		reply(http.StatusOK, "application/json", `{"data":{"items":[]}}`)(w)
	}))
	t.Cleanup(srv.Close)

	c := client(t, srv.URL, true)
	currentTime := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	c.now = func() time.Time { return currentTime }

	for i := 0; i < 3; i++ {
		if _, err := c.SearchCompanies(context.Background(), "example & co"); err != nil {
			t.Fatalf("SearchCompanies() [%d] error = %v", i, err)
		}
	}
	if configCalls != 1 {
		t.Errorf("config.env.js fetched %d times, want 1 within 30m", configCalls)
	}

	// Advance time by 31 minutes: profile id should expire and be refetched
	currentTime = currentTime.Add(31 * time.Minute)
	if _, err := c.SearchCompanies(context.Background(), "example & co"); err != nil {
		t.Fatalf("SearchCompanies() after 31m error = %v", err)
	}
	if configCalls != 2 {
		t.Errorf("config.env.js fetched %d times, want 2 after 30m TTL", configCalls)
	}
}

func TestSearchConfigUnavailable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/config.env.js" {
			reply(http.StatusInternalServerError, "text/plain", "error")(w)
			return
		}
		reply(http.StatusOK, "application/json", `{"data":{"items":[]}}`)(w)
	}))
	t.Cleanup(srv.Close)

	c := client(t, srv.URL, true)
	if _, err := c.SearchCompanies(context.Background(), "example & co"); !errors.Is(err, administration.ErrRegisterUnavailable) {
		t.Errorf("SearchCompanies() error = %v, want ErrRegisterUnavailable", err)
	}
}

func TestSearchConfigMalformed(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/config.env.js" {
			reply(http.StatusOK, "text/html", "<html>blocked</html>")(w)
			return
		}
		reply(http.StatusOK, "application/json", `{"data":{"items":[]}}`)(w)
	}))
	t.Cleanup(srv.Close)

	c := client(t, srv.URL, true)
	if _, err := c.SearchCompanies(context.Background(), "example & co"); !errors.Is(err, administration.ErrRegisterBlocked) {
		t.Errorf("SearchCompanies() error = %v, want ErrRegisterBlocked", err)
	}
}

func TestSearchInvalidatesProfileIDOnBadRequest(t *testing.T) {
	configCalls := 0
	searchCalls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/config.env.js" {
			configCalls++
			w.Header().Set("Content-Type", "application/javascript")
			_, _ = w.Write([]byte(envConfigBody))
			return
		}
		searchCalls++
		if searchCalls == 1 {
			// First search fails with 400 Bad Request (expired profile id)
			reply(http.StatusBadRequest, "application/json", `{"errors":{"profileId":["required"]}}`)(w)
			return
		}
		reply(http.StatusOK, "application/json", `{"data":{"items":[]}}`)(w)
	}))
	t.Cleanup(srv.Close)

	c := client(t, srv.URL, true)
	// First search fails with 400
	if _, err := c.SearchCompanies(context.Background(), "example & co"); !errors.Is(err, administration.ErrRegisterBlocked) {
		t.Errorf("first search error = %v, want ErrRegisterBlocked", err)
	}
	// Second search should re-fetch config because profile ID was invalidated
	if _, err := c.SearchCompanies(context.Background(), "example & co"); err != nil {
		t.Errorf("second search error = %v, want nil", err)
	}
	if configCalls != 2 {
		t.Errorf("config.env.js fetched %d times, want 2 (refetched after 400)", configCalls)
	}
}
