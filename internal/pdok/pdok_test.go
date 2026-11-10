package pdok

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/adirkuhn/moneyharpy/internal/administration"
	"github.com/adirkuhn/moneyharpy/internal/settings"
	"github.com/google/go-cmp/cmp"
)

type staticSettings struct{ inst settings.Instance }

func (s staticSettings) Instance(context.Context) (settings.Instance, error) { return s.inst, nil }

func client(base string, enabled bool) *Client {
	return New(staticSettings{inst: settings.Instance{AddressLookupEnabled: enabled}}, base, http.DefaultClient)
}

func serve(t *testing.T, body string, gotQuery *string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		*gotQuery = r.URL.Query().Get("q")
		if got := r.URL.Query().Get("fq"); got != "type:adres" {
			t.Errorf("fq = %q, want type:adres", got)
		}
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return srv
}

const found = `{"response":{"numFound":1,"docs":[{"type":"adres","woonplaatsnaam":"Amsterdam","huis_nlt":"147","postcode":"1012RJ","huisnummer":147,"straatnaam":"Nieuwezijds Voorburgwal"}]}}`

func TestLookupAddress(t *testing.T) {
	var q string
	srv := serve(t, found, &q)
	got, err := client(srv.URL, true).LookupAddress(context.Background(), " 1012 rj ", "147")
	if err != nil {
		t.Fatalf("LookupAddress() error = %v", err)
	}
	want := administration.AddressMatch{Address1: "Nieuwezijds Voorburgwal 147", Zipcode: "1012RJ", City: "Amsterdam"}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("LookupAddress() diff (-want +got):\n%s", diff)
	}
	if want := "postcode:1012RJ and huisnummer:147"; q != want {
		t.Errorf("q = %q, want %q", q, want)
	}
}

func TestLookupKeepsHouseNumberSuffixAsTyped(t *testing.T) {
	var q string
	srv := serve(t, found, &q)
	got, err := client(srv.URL, true).LookupAddress(context.Background(), "1012RJ", "147-a")
	if err != nil {
		t.Fatalf("LookupAddress() error = %v", err)
	}
	if want := "Nieuwezijds Voorburgwal 147-a"; got.Address1 != want {
		t.Errorf("Address1 = %q, want %q", got.Address1, want)
	}
	if !strings.HasSuffix(q, "huisnummer:147") {
		t.Errorf("q = %q, want numeric house number only", q)
	}
}

func TestLookupRejects(t *testing.T) {
	var q string
	none := serve(t, `{"response":{"numFound":0,"docs":[]}}`, &q)
	tests := []struct {
		name, base, zip, number string
	}{
		{name: "no such address", base: none.URL, zip: "9999ZZ", number: "1"},
		{name: "malformed zipcode cannot reach the query", base: "http://unused", zip: "1012RJ or x", number: "1"},
		{name: "malformed number cannot reach the query", base: "http://unused", zip: "1012RJ", number: "x or 1"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := client(tt.base, true).LookupAddress(context.Background(), tt.zip, tt.number)
			if !errors.Is(err, administration.ErrAddressUnknown) {
				t.Errorf("LookupAddress() error = %v, want ErrAddressUnknown", err)
			}
		})
	}
}

func TestLookupDisabled(t *testing.T) {
	_, err := client("http://unused", false).LookupAddress(context.Background(), "1012RJ", "1")
	if !errors.Is(err, administration.ErrLookupUnavailable) {
		t.Errorf("LookupAddress() error = %v, want ErrLookupUnavailable", err)
	}
}
