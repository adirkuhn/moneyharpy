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

func (s staticSettings) Instance(context.Context) (settings.Instance, error) { return s.inst, nil }

func client(t *testing.T, url string, enabled bool) *Client {
	t.Helper()
	s := staticSettings{inst: settings.Instance{KVK: settings.KVK{WebsiteSearch: enabled}}}
	return New(s, url, http.DefaultClient, ratelimit.New(1000, time.Minute))
}

// serve answers like the KVK website search, checking what is asked.
func serve(t *testing.T, handler func(w http.ResponseWriter)) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls++
		reply(http.StatusOK, "application/json", `{"data":{"items":[]}}`)(w)
	}))
	t.Cleanup(srv.Close)
	s := staticSettings{inst: settings.Instance{KVK: settings.KVK{WebsiteSearch: true}}}
	c := New(s, srv.URL, http.DefaultClient, ratelimit.New(2, time.Minute))
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
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		reply(http.StatusForbidden, "text/html", "<html>blocked</html>")(w)
	}))
	t.Cleanup(srv.Close)
	if err := client(t, srv.URL, false).Check(context.Background()); !errors.Is(err, administration.ErrRegisterBlocked) {
		t.Errorf("Check() error = %v, want ErrRegisterBlocked", err)
	}
}
