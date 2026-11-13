package recognize

import (
	"slices"
	"testing"

	"github.com/adirkuhn/moneyharpy/internal/contacts"
)

func TestNormalizeName(t *testing.T) {
	for _, tt := range []struct{ in, want string }{
		{"Example Shop B.V.", "example shop"},
		{"EXAMPLE SHOP BV", "example shop"},
		{"Example Shop Ltd.", "example shop"},
		{"Example-Shop GmbH", "example shop"},
		{"The Example Company", "example company"},
	} {
		if got := NormalizeName(tt.in); got != tt.want {
			t.Errorf("NormalizeName(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestDomainSkipsFreeMail(t *testing.T) {
	for _, tt := range []struct{ in, want string }{
		{"billing@example.com", "example.com"},
		{"Billing@Mail.Example.com", "mail.example.com"},
		{"https://www.example.com/contact", "example.com"},
		{"www.example.com", "example.com"},
		{"ann@gmail.com", ""},
		{"someone@outlook.com", ""},
		{"", ""},
	} {
		if got := Domain(tt.in); got != tt.want {
			t.Errorf("Domain(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestMatchPrefersStrongKeys(t *testing.T) {
	byName := contacts.Contact{ID: 1, CompanyName: "Example Shop"}
	byMail := contacts.Contact{ID: 2, CompanyName: "Other", SendInvoicesToEmail: "ann@example.com, bob@example.org"}
	byKVK := contacts.Contact{ID: 3, CompanyName: "Third", ChamberOfCommerce: "1234 5678"}
	byVAT := contacts.Contact{ID: 4, CompanyName: "Fourth", TaxNumber: "nl 1234.56789.B01"}
	byIBAN := contacts.Contact{ID: 5, CompanyName: "Fifth", BankAccount: "NL91 ABNA 0417 1643 00"}
	all := []contacts.Contact{byName, byMail, byKVK, byVAT, byIBAN}
	sup := Supplier{Name: "Example Shop B.V.", Email: "billing@example.com", KVK: "12345678", VATNumber: "NL123456789B01", IBAN: "NL91ABNA0417164300"}

	for _, tt := range []struct {
		name   string
		drop   func(*Supplier)
		wantID int
		wantBy MatchedBy
	}{
		{"iban first", func(*Supplier) {}, 5, ByIBAN},
		{"then vat", func(s *Supplier) { s.IBAN = "" }, 4, ByVATNumber},
		{"then kvk", func(s *Supplier) { s.IBAN, s.VATNumber = "", "" }, 3, ByKVK},
		{"then email domain", func(s *Supplier) { s.IBAN, s.VATNumber, s.KVK = "", "", "" }, 2, ByEmailDomain},
		{"then name", func(s *Supplier) { s.IBAN, s.VATNumber, s.KVK, s.Email = "", "", "", "" }, 1, ByName},
	} {
		t.Run(tt.name, func(t *testing.T) {
			s := sup
			tt.drop(&s)
			got, by, ok := Match(s, all)
			if !ok || int(got.ID) != tt.wantID || by != tt.wantBy {
				t.Errorf("Match() = %d, %q, %v; want %d, %q", got.ID, by, ok, tt.wantID, tt.wantBy)
			}
		})
	}
	if by := ByIBAN; by.Confidence() != Certain || ByName.Confidence() != Uncertain || ByEmailDomain.Confidence() != Uncertain {
		t.Error("confidence of matches is wrong")
	}
}

func TestMatchIgnoresFreeMailAndMisses(t *testing.T) {
	c := contacts.Contact{ID: 1, CompanyName: "Ann de Vries", SendInvoicesToEmail: "ann@gmail.com"}
	if _, _, ok := Match(Supplier{Name: "Example Shop", Email: "shop@gmail.com"}, []contacts.Contact{c}); ok {
		t.Error("Match() matched on a free mail domain")
	}
}

func TestSearchTerms(t *testing.T) {
	got := Supplier{Name: "Example Shop B.V.", Email: "billing@example.com", KVK: "1234 5678", VATNumber: "NL 123456789 B01", IBAN: "nl91 abna 0417 1643 00"}.SearchTerms()
	want := []string{"NL91ABNA0417164300", "NL123456789B01", "12345678", "@example.com", "example"}
	if !slices.Equal(got, want) {
		t.Errorf("SearchTerms() = %v, want %v", got, want)
	}
}
