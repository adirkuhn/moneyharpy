package httpapi_test

import (
	"context"
	"errors"
	"net/http"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"

	"github.com/adirkuhn/moneyharpy/internal/httpapi"
	"github.com/adirkuhn/moneyharpy/internal/postgres/postgrestest"
	"github.com/adirkuhn/moneyharpy/internal/ratelimit"
	"github.com/adirkuhn/moneyharpy/internal/recognize"
	"github.com/adirkuhn/moneyharpy/internal/settings"
)

// fakeVIES stands in for the EU's VIES register.
type fakeVIES struct {
	mu        sync.Mutex
	companies map[string]recognize.RegisteredCompany
	err       error
	asked     []string
}

// LookupVAT implements recognize.Registry.
func (f *fakeVIES) LookupVAT(_ context.Context, vat string) (recognize.RegisteredCompany, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.asked = append(f.asked, vat)
	return f.companies[vat], f.err
}

func (f *fakeVIES) reset(err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.asked, f.err = nil, err
}

func TestVATLookup(t *testing.T) {
	ctx := context.Background()
	pool, cleanup, err := postgrestest.Start(ctx)
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	defer cleanup()
	h := newHarness(t, options{pool: pool})
	h.vies.companies = map[string]recognize.RegisteredCompany{
		"NL123456782B01": {Valid: true, Name: "EXAMPLE SHOP B.V.", Address: "EXAMPLE STREET 00001\n1234AB AMSTERDAM\n"},
	}
	c := h.client()
	c.register()
	// The first user is the instance administrator.
	wantStatus(t, c.do(http.MethodGet, "/api/instance/settings", nil), http.StatusOK)
	a := c.createAdministration("Harpy")
	lookup := func(vat string) response {
		return c.do(http.MethodGet, "/api/v2/"+a.Id+"/vat_lookup?vat_number="+vat, nil)
	}
	options := func() httpapi.OnboardingOptions {
		return decode[httpapi.OnboardingOptions](t, c.do(http.MethodGet, "/api/onboarding/options", nil))
	}

	t.Run("is off until an administrator turns it on", func(t *testing.T) {
		h.vies.reset(nil)
		r := lookup("NL123456782B01")
		wantStatus(t, r, http.StatusForbidden)
		if got := decode[httpapi.Error](t, r); got.Code == nil || *got.Code != "vies_disabled" {
			t.Errorf("code = %v, want vies_disabled", got.Code)
		}
		if len(h.vies.asked) != 0 {
			t.Errorf("asked VIES about %q while off", h.vies.asked)
		}
		if options().VatLookupEnabled {
			t.Error("options offer the VAT number lookup while off")
		}
	})

	on := true
	if _, err := h.settle.Update(ctx, settings.Update{VIESEnabled: &on}); err != nil {
		t.Fatalf("Update() error = %v", err)
	}

	t.Run("is offered once on", func(t *testing.T) {
		if !options().VatLookupEnabled {
			t.Error("options do not offer the VAT number lookup")
		}
	})

	t.Run("fills in a registered company", func(t *testing.T) {
		h.vies.reset(nil)
		r := lookup("nl123456782b01")
		wantStatus(t, r, http.StatusOK)
		want := httpapi.VatLookup{Valid: true, Name: "EXAMPLE SHOP B.V.", Address1: "EXAMPLE STREET 00001", Zipcode: "1234 AB", City: "AMSTERDAM", Country: "NL"}
		if diff := cmp.Diff(want, decode[httpapi.VatLookup](t, r)); diff != "" {
			t.Errorf("lookup diff (-want +got):\n%s", diff)
		}
		// Only the VAT number goes out.
		if !slices.Equal(h.vies.asked, []string{"NL123456782B01"}) {
			t.Errorf("asked VIES about %q", h.vies.asked)
		}
	})

	t.Run("tells a number VIES does not know", func(t *testing.T) {
		h.vies.reset(nil)
		r := lookup("NL000099998B57")
		wantStatus(t, r, http.StatusOK)
		if diff := cmp.Diff(httpapi.VatLookup{Country: "NL"}, decode[httpapi.VatLookup](t, r)); diff != "" {
			t.Errorf("lookup diff (-want +got):\n%s", diff)
		}
	})

	t.Run("says when VIES is unavailable", func(t *testing.T) {
		h.vies.reset(errors.New("vies unavailable: MS_UNAVAILABLE"))
		defer h.vies.reset(nil)
		r := lookup("NL123456782B01")
		wantStatus(t, r, http.StatusServiceUnavailable)
		if got := decode[httpapi.Error](t, r); got.Code == nil || *got.Code != "vies_unavailable" {
			t.Errorf("code = %v, want vies_unavailable", got.Code)
		}
	})

	t.Run("refuses numbers that are not EU VAT numbers without asking", func(t *testing.T) {
		h.vies.reset(nil)
		for _, vat := range []string{"NL123456789B01", "GB123456789", "DE12345", "12345678"} {
			r := lookup(vat)
			wantStatus(t, r, http.StatusUnprocessableEntity)
			if got := decode[httpapi.ValidationError](t, r).Error["vat_number"]; len(got) == 0 {
				t.Errorf("%s: errors = %s, want one on vat_number", vat, r.body)
			}
		}
		if len(h.vies.asked) != 0 {
			t.Errorf("asked VIES about %q", h.vies.asked)
		}
	})

	t.Run("only for users of the administration", func(t *testing.T) {
		other := h.client()
		other.register()
		wantStatus(t, other.do(http.MethodGet, "/api/v2/"+a.Id+"/vat_lookup?vat_number=NL123456782B01", nil), http.StatusNotFound)
	})

	t.Run("the old setting name still turns it off", func(t *testing.T) {
		r := c.do(http.MethodPatch, "/api/instance/settings", map[string]any{"recognition_vies_enabled": false})
		wantStatus(t, r, http.StatusOK)
		if decode[httpapi.InstanceSettings](t, r).ViesEnabled {
			t.Error("vies_enabled stayed on")
		}
		wantStatus(t, lookup("NL123456782B01"), http.StatusForbidden)
	})
}

func TestVATLookupIsRateLimitedLikeTheRegisterSearch(t *testing.T) {
	h := newHarness(t, options{lookupLimiter: ratelimit.New(1, 5*time.Minute)})
	c := h.client()
	wantStatus(t, c.do(http.MethodGet, "/api/v2/1/vat_lookup?vat_number=NL123456782B01", nil), http.StatusUnauthorized)
	wantStatus(t, c.do(http.MethodGet, "/api/v2/1/vat_lookup.json?vat_number=NL123456782B01", nil), http.StatusTooManyRequests)
	// Other API calls keep their own limit.
	wantStatus(t, c.do(http.MethodGet, "/api/v2/administrations", nil), http.StatusUnauthorized)
}
