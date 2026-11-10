package kvk

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/adirkuhn/moneyharpy/internal/administration"
	"github.com/adirkuhn/moneyharpy/internal/settings"
	"github.com/google/go-cmp/cmp"
)

type staticSettings struct{ inst settings.Instance }

func (s staticSettings) Instance(context.Context) (settings.Instance, error) { return s.inst, nil }

func client(base string, enabled bool) *Client {
	s := staticSettings{inst: settings.Instance{KVK: settings.KVK{Enabled: enabled, APIKey: "secret"}}}
	return New(s, base, http.DefaultClient)
}

func serve(t *testing.T, status int, body string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/zoeken" {
			t.Errorf("path = %q, want /zoeken", r.URL.Path)
		}
		if got := r.Header.Get("apikey"); got != "secret" {
			t.Errorf("apikey = %q, want secret", got)
		}
		if got := r.URL.Query().Get("naam"); got != "harpy & co" {
			t.Errorf("naam = %q, want the query", got)
		}
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return srv
}

// Bodies below are the shapes the KVK test environment returns.
func TestSearch(t *testing.T) {
	srv := serve(t, http.StatusOK, `{"pagina":1,"totaal":3,"resultaten":[
		{"kvkNummer":"68750110","naam":"Test BV Donald","type":"rechtspersoon"},
		{"kvkNummer":"69599068","naam":"Test Stichting","adres":{"binnenlandsAdres":{"type":"bezoekadres","straatnaam":"Oosterwal","plaats":"Lochem"}},"type":"rechtspersoon"},
		{"kvkNummer":"90003942","naam":"Full","adres":{"binnenlandsAdres":{"straatnaam":"Dam","huisnummer":1,"huisletter":"A","postcode":"1012JS","plaats":"Amsterdam"}}}
	]}`)
	got, err := client(srv.URL, true).SearchCompanies(context.Background(), "harpy & co")
	if err != nil {
		t.Fatalf("SearchCompanies() error = %v", err)
	}
	want := []administration.CompanyMatch{
		{Name: "Test BV Donald", ChamberOfCommerce: "68750110"},
		{Name: "Test Stichting", ChamberOfCommerce: "69599068", Address1: "Oosterwal", City: "Lochem"},
		{Name: "Full", ChamberOfCommerce: "90003942", Address1: "Dam 1A", Zipcode: "1012JS", City: "Amsterdam"},
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("SearchCompanies() diff (-want +got):\n%s", diff)
	}
}

func TestSearchWithoutMatchesIsEmpty(t *testing.T) {
	srv := serve(t, http.StatusNotFound, `{"fout":[{"code":"IPD5200","omschrijving":"geen gegevens"}]}`)
	got, err := client(srv.URL, true).SearchCompanies(context.Background(), "harpy & co")
	if err != nil {
		t.Fatalf("SearchCompanies() error = %v", err)
	}
	if len(got) != 0 {
		t.Errorf("SearchCompanies() = %v, want none", got)
	}
}

func TestSearchFailures(t *testing.T) {
	srv := serve(t, http.StatusUnauthorized, `{}`)
	if _, err := client(srv.URL, true).SearchCompanies(context.Background(), "harpy & co"); err == nil {
		t.Error("SearchCompanies() error = nil, want error")
	}
	if _, err := client("http://unused", false).SearchCompanies(context.Background(), "x"); !errors.Is(err, administration.ErrLookupUnavailable) {
		t.Errorf("SearchCompanies() when disabled error = %v, want ErrLookupUnavailable", err)
	}
}
