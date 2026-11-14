package contacts

import (
	"context"
	"errors"
	"testing"

	"github.com/adirkuhn/moneyharpy/internal/administration"
	"github.com/adirkuhn/moneyharpy/internal/resource"
)

func TestNormalizeRegistryNumbers(t *testing.T) {
	for in, want := range map[string]string{"12345678": "12345678", " 1234 5678 ": "12345678", "1234.56.78": "12345678", "KVK 12345678": "12345678", "": ""} {
		if got := NormalizeChamberOfCommerce(in); got != want {
			t.Errorf("NormalizeChamberOfCommerce(%q) = %q, want %q", in, got, want)
		}
	}
	for in, want := range map[string]string{"NL123456782B01": "NL123456782B01", "nl 1234.56782-b01": "NL123456782B01", "\tNL123456782B01 ": "NL123456782B01", "": ""} {
		if got := NormalizeVAT(in); got != want {
			t.Errorf("NormalizeVAT(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestRegistryDuplicates(t *testing.T) {
	s, _ := newService()
	ctx := context.Background()
	shop := create(t, s, Changes{CompanyName: ptr("Example Shop B.V."), ChamberOfCommerce: ptr("1234 5678"), TaxNumber: ptr("NL 1234.56782.B01")})
	old := create(t, s, Changes{CompanyName: ptr("Example Archive B.V."), ChamberOfCommerce: ptr("87654321"), TaxNumber: ptr("NL000099998B57")})
	if _, err := s.Archive(ctx, adminID, old.ID, owner); err != nil {
		t.Fatal(err)
	}
	check := func(coc, tax string, except resource.ID) map[string]resource.ID {
		t.Helper()
		got, err := s.RegistryDuplicates(ctx, adminID, owner, coc, tax, except)
		if err != nil {
			t.Fatalf("RegistryDuplicates: %v", err)
		}
		out := map[string]resource.ID{}
		for _, d := range got {
			out[d.Field] = d.Contact.ID
		}
		return out
	}

	if got := check("12345678", "nl123456782b01", 0); got[FieldChamberOfCommerce] != shop.ID || got[FieldTaxNumber] != shop.ID {
		t.Errorf("normalised numbers = %v, want both on %v", got, shop.ID)
	}
	if got := check("12.34.56.78", "", 0); len(got) != 1 || got[FieldChamberOfCommerce] != shop.ID {
		t.Errorf("KVK only = %v", got)
	}
	if got := check("12345678", "NL123456782B01", shop.ID); len(got) != 0 {
		t.Errorf("the contact itself = %v, want none", got)
	}
	if got := check("87654321", "NL000099998B57", 0); len(got) != 0 {
		t.Errorf("archived contact = %v, want none", got)
	}
	if got := check("", "  ", 0); len(got) != 0 {
		t.Errorf("blank numbers = %v, want none", got)
	}
	if _, err := s.RegistryDuplicates(ctx, adminID, stranger, "12345678", "", 0); !errors.Is(err, administration.ErrNotFound) {
		t.Errorf("stranger = %v", err)
	}
}

func TestDoublesNormaliseRegistryNumbers(t *testing.T) {
	s, _ := newService()
	create(t, s, Changes{CompanyName: ptr("Example Shop"), TaxNumber: ptr("NL123456782B01")})
	create(t, s, Changes{CompanyName: ptr("Example Shop Amsterdam"), TaxNumber: ptr("nl 1234.56782.b01")})
	groups, err := s.Doubles(context.Background(), adminID, owner, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(groups) != 1 || len(groups[0]) != 2 {
		t.Errorf("groups = %+v", groups)
	}
}
