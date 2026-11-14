package httpapi_test

import (
	"fmt"
	"net/http"
	"net/url"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/adirkuhn/moneyharpy/internal/administration"
	"github.com/adirkuhn/moneyharpy/internal/httpapi"
)

func TestKVKLookup(t *testing.T) {
	h := newHarness(t, options{})
	c := h.client()
	c.register()
	a := c.createAdministration("Harpy")
	lookup := func(number string) response {
		return c.do(http.MethodGet, "/api/v2/"+a.Id+"/kvk_lookup?kvk_number="+number, nil)
	}
	defer func() { h.lookup.company, h.lookup.companyErr = administration.CompanyMatch{}, nil }()

	t.Run("finds the company by its number", func(t *testing.T) {
		h.lookup.company = administration.CompanyMatch{Name: "Example Shop B.V.", ChamberOfCommerce: "12345678", Address1: "Dam 1", Zipcode: "1012JS", City: "Amsterdam"}
		r := lookup("12345678")
		wantStatus(t, r, http.StatusOK)
		want := httpapi.CompanyMatch{Name: "Example Shop B.V.", ChamberOfCommerce: "12345678", Address1: "Dam 1", Zipcode: "1012JS", City: "Amsterdam"}
		if diff := cmp.Diff(want, decode[httpapi.CompanyMatch](t, r)); diff != "" {
			t.Errorf("lookup diff (-want +got):\n%s", diff)
		}
		if h.lookup.gotNumber != "12345678" {
			t.Errorf("asked %q, want 12345678", h.lookup.gotNumber)
		}
	})

	t.Run("an unknown number or a register that is off is not found", func(t *testing.T) {
		for _, err := range []error{administration.ErrCompanyNotFound, administration.ErrLookupUnavailable} {
			h.lookup.companyErr = err
			wantStatus(t, lookup("12345678"), http.StatusNotFound)
		}
	})

	t.Run("tells why the register could not be asked", func(t *testing.T) {
		h.lookup.companyErr = fmt.Errorf("lookup: %w", administration.ErrRegisterUnavailable)
		r := lookup("12345678")
		wantStatus(t, r, http.StatusServiceUnavailable)
		if got := decode[httpapi.Error](t, r); got.Code == nil || *got.Code != "kvk_unavailable" {
			t.Errorf("code = %v, want kvk_unavailable", got.Code)
		}
	})

	t.Run("only takes eight digits", func(t *testing.T) {
		h.lookup.companyErr, h.lookup.gotNumber = nil, ""
		for _, bad := range []string{"1234567", "123456789", "1234abcd"} {
			wantStatus(t, lookup(bad), http.StatusBadRequest)
		}
		if h.lookup.gotNumber != "" {
			t.Errorf("asked the register about %q", h.lookup.gotNumber)
		}
	})

	t.Run("requires access to the administration", func(t *testing.T) {
		stranger := h.client()
		stranger.register()
		if r := stranger.do(http.MethodGet, "/api/v2/"+a.Id+"/kvk_lookup?kvk_number=12345678", nil); r.status < 400 {
			t.Errorf("stranger got %d", r.status)
		}
	})
}

func TestContactRegistryDuplicates(t *testing.T) {
	h := newHarness(t, options{})
	c := h.client()
	c.register()
	a := c.createAdministration("Harpy")
	api := "/api/v2/" + a.Id
	shop := decode[contact](t, c.do(http.MethodPost, api+"/contacts", map[string]any{"contact": map[string]any{
		"company_name": "Example Shop B.V.", "chamber_of_commerce": "1234 5678", "tax_number": "NL123456782B01",
	}}))
	check := func(q url.Values) []httpapi.ContactRegistryDuplicate {
		t.Helper()
		r := c.do(http.MethodGet, api+"/contacts/registry_duplicates?"+q.Encode(), nil)
		wantStatus(t, r, http.StatusOK)
		return decode[[]httpapi.ContactRegistryDuplicate](t, r)
	}

	got := check(url.Values{"chamber_of_commerce": {"12345678"}, "tax_number": {"nl 1234.56782.b01"}})
	want := []httpapi.ContactRegistryDuplicate{
		{Field: "chamber_of_commerce", ContactId: shop.ID, ContactName: "Example Shop B.V.", Message: "A contact with this chamber of commerce number already exists: Example Shop B.V."},
		{Field: "tax_number", ContactId: shop.ID, ContactName: "Example Shop B.V.", Message: "A contact with this VAT number already exists: Example Shop B.V."},
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("duplicates diff (-want +got):\n%s", diff)
	}
	if got := check(url.Values{"chamber_of_commerce": {"12345678"}, "except_id": {shop.ID}}); len(got) != 0 {
		t.Errorf("the contact itself = %+v, want none", got)
	}
	if got := check(url.Values{"chamber_of_commerce": {"87654321"}}); len(got) != 0 {
		t.Errorf("another number = %+v, want none", got)
	}

	// The Moneybird API accepts the duplicate, as Moneybird does.
	wantStatus(t, c.do(http.MethodPost, api+"/contacts", map[string]any{"contact": map[string]any{
		"company_name": "Example Shop Amsterdam", "chamber_of_commerce": "12345678",
	}}), http.StatusCreated)

	wantStatus(t, c.do(http.MethodGet, api+"/contacts/registry_duplicates?except_id=abc", nil), http.StatusBadRequest)
}
