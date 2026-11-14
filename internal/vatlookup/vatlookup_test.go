package vatlookup_test

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/adirkuhn/moneyharpy/internal/recognize"
	"github.com/adirkuhn/moneyharpy/internal/settings"
	"github.com/adirkuhn/moneyharpy/internal/vatlookup"
)

type fakeSettings struct {
	on  bool
	err error
}

func (f fakeSettings) Instance(context.Context) (settings.Instance, error) {
	return settings.Instance{VIESEnabled: f.on}, f.err
}

type fakeRegistry struct {
	companies map[string]recognize.RegisteredCompany
	err       error
	asked     []string
}

func (f *fakeRegistry) LookupVAT(_ context.Context, vat string) (recognize.RegisteredCompany, error) {
	f.asked = append(f.asked, vat)
	return f.companies[vat], f.err
}

func TestLookup(t *testing.T) {
	registered := map[string]recognize.RegisteredCompany{
		"NL123456782B01": {Valid: true, Name: "EXAMPLE SHOP  B.V.", Address: "EXAMPLE STREET 00001\n1234AB AMSTERDAM\n"},
	}
	for name, tc := range map[string]struct {
		on    bool
		vat   string
		err   error
		want  vatlookup.Company
		fail  error
		asked []string
	}{
		"registered": {
			on: true, vat: "nl 1234.56782.b01", asked: []string{"NL123456782B01"},
			want: vatlookup.Company{Valid: true, Name: "EXAMPLE SHOP B.V.", Address1: "EXAMPLE STREET 00001", Zipcode: "1234 AB", City: "AMSTERDAM", Country: "NL"},
		},
		"not registered": {
			on: true, vat: "NL000099998B57", asked: []string{"NL000099998B57"}, want: vatlookup.Company{Country: "NL"},
		},
		"VIES unavailable": {
			on: true, vat: "NL123456782B01", err: errors.New("timeout"), asked: []string{"NL123456782B01"}, fail: vatlookup.ErrUnavailable,
		},
		"turned off":                  {vat: "NL123456782B01", fail: vatlookup.ErrDisabled},
		"failing checksum":            {on: true, vat: "NL123456789B01", fail: vatlookup.ErrInvalidNumber},
		"outside the EU":              {on: true, vat: "GB123456789", fail: vatlookup.ErrInvalidNumber},
		"not a VAT number":            {on: true, vat: "Example", fail: vatlookup.ErrInvalidNumber},
		"wrong shape for its country": {on: true, vat: "DE12345", fail: vatlookup.ErrInvalidNumber},
	} {
		t.Run(name, func(t *testing.T) {
			registry := &fakeRegistry{companies: registered, err: tc.err}
			got, err := vatlookup.New(fakeSettings{on: tc.on}, registry).Lookup(context.Background(), tc.vat)
			if !errors.Is(err, tc.fail) || (tc.fail == nil) != (err == nil) {
				t.Fatalf("Lookup() error = %v, want %v", err, tc.fail)
			}
			if diff := cmp.Diff(tc.want, got); diff != "" {
				t.Errorf("Lookup() diff (-want +got):\n%s", diff)
			}
			if !slices.Equal(registry.asked, tc.asked) {
				t.Errorf("asked VIES about %q, want %q", registry.asked, tc.asked)
			}
		})
	}
}

func TestLookupFailsWithTheSettings(t *testing.T) {
	broken := errors.New("database down")
	if _, err := vatlookup.New(fakeSettings{err: broken}, &fakeRegistry{}).Lookup(context.Background(), "NL123456782B01"); !errors.Is(err, broken) {
		t.Errorf("Lookup() error = %v, want %v", err, broken)
	}
}
