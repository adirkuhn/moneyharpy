package recognize

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/adirkuhn/moneyharpy/internal/administration"
	"github.com/adirkuhn/moneyharpy/internal/banking"
	"github.com/adirkuhn/moneyharpy/internal/contacts"
	"github.com/adirkuhn/moneyharpy/internal/ocr"
	"github.com/adirkuhn/moneyharpy/internal/resource"
)

// buyer is the administration that receives nl_buyer_first.txt.
var buyer = Self{
	Name: "Example Shop B.V.", Address1: "Example Street 31", Zipcode: "1234AB", VATNumber: "NL987654329B01", KVK: "87654321",
	IBANs: []string{"NL20 INGB 0001 2345 67"}, Website: "https://shop.example.com",
}

// An invoice that starts with the buyer's block (the administration's own
// name, "FAO", its address) and names the supplier only in its footer.
func TestParseTextSupplierInFooter(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("testdata", "nl_buyer_first.txt"))
	if err != nil {
		t.Fatal(err)
	}
	got := ParseTextFor(string(data), today, buyer)
	s := got.Supplier
	want := Supplier{
		Name: "ExampleShip", VATNumber: "NL123456782B01", KVK: "12345678", IBAN: "NL91ABNA0417164300",
		Email: "info@exampleship.example", Address1: "Example Lane 31", Zipcode: "9999 ZZ", City: "Voorbeeldstad", Country: "NL",
		Phone: "012 345 6789",
	}
	if s != want {
		t.Errorf("supplier\n got %+v\nwant %+v", s, want)
	}
	if got.SupplierFound != Certain {
		t.Errorf("supplier found %q, want certain", got.SupplierFound)
	}
	if buyer.Is(s) {
		t.Errorf("the supplier %+v is the administration itself", s)
	}
	if got.Reference.V != "260000001" || got.TotalInclTax.V.String() != "428.68" {
		t.Errorf("reference %q, total %s", got.Reference.V, got.TotalInclTax.V)
	}
}

// Without knowing the administration, the footer still names the
// supplier rather than the block at the top.
func TestParseTextAttributionBeatsTheTopBlock(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("testdata", "nl_buyer_first.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if s := ParseText(string(data), today).Supplier; s.Name != "ExampleShip" {
		t.Errorf("name = %q, want ExampleShip", s.Name)
	}
}

func TestSupplierAttribution(t *testing.T) {
	for name, tc := range map[string]struct{ text, want string }{
		"in favour of":      {"Invoice\nJohn Example\nPaid in favour of Example Supplies.\n", "Example Supplies"},
		"on behalf of":      {"Invoice\nJohn Example\nInvoiced on behalf of Example Supplies\n", "Example Supplies"},
		"namens":            {"Factuur\nJan Voorbeeld\nDeze factuur wordt namens Voorbeeld Levering verstuurd.\n", "Voorbeeld Levering"},
		"ten gunste van":    {"Factuur\nJan Voorbeeld\nWordt afgeschreven ten gunste van Voorbeeld Levering.\n", "Voorbeeld Levering"},
		"invoiced by":       {"Invoice\nJohn Example\nInvoiced by: Example Supplies Ltd\n", "Example Supplies Ltd"},
		"Rechnungssteller":  {"Rechnung\nMax Beispiel\nRechnungssteller: Beispiel Handel GmbH\n", "Beispiel Handel GmbH"},
		"a legal name wins": {"Invoice\nJohn Example\nInvoiced on behalf of Example Supplies\nExample Supplies B.V. | KvK 12345678\n", "Example Supplies B.V."},
		"never the buyer":   {"Invoice\nInvoiced on behalf of Example Shop\nExample Supplies\n", "Example Supplies"},
	} {
		t.Run(name, func(t *testing.T) {
			lines := prepare(tc.text)
			s, _ := readSupplierFor(lines, buyer)
			if s.Name != tc.want {
				t.Errorf("name = %q, want %q", s.Name, tc.want)
			}
		})
	}
}

// The administration's own numbers are never the supplier's, wherever they
// are printed.
func TestSupplierNumbersAreNotTheBuyers(t *testing.T) {
	for name, tc := range map[string]struct {
		text                string
		iban, vat, kvk, zip string
	}{
		"the account debited": {
			text: "Example Supplies B.V.\nThis amount will be debited from your account NL20INGB0001234567.\nIBAN NL91ABNA0417164300\n",
			iban: "NL91ABNA0417164300",
		},
		"the buyer's IBAN on a line of its own": {
			text: "Example Supplies B.V.\nNL20 INGB 0001 2345 67\nBank NL91 ABNA 0417 1643 00\n",
			iban: "NL91ABNA0417164300",
		},
		"only the buyer's IBAN": {text: "Example Supplies B.V.\nIBAN NL20INGB0001234567\n"},
		"the buyer's VAT number labelled": {
			text: "Example Supplies B.V.\nBTW NL987654329B01\nNL123456782B01\n", vat: "NL123456782B01",
		},
		"the buyer's KvK number": {
			text: "Example Supplies B.V.\nKvK 87654321\nKvK 12345678\n", kvk: "12345678",
		},
		"the buyer's address": {
			text: "Example Supplies B.V.\nExample Street 31\n1234 AB Amsterdam\nExample Lane 31\n9999 ZZ Voorbeeldstad\n", zip: "9999 ZZ",
		},
	} {
		t.Run(name, func(t *testing.T) {
			lines := prepare(tc.text)
			s, _ := readSupplierFor(lines, buyer)
			if s.IBAN != tc.iban || s.VATNumber != tc.vat || s.KVK != tc.kvk || s.Zipcode != tc.zip {
				t.Errorf("got IBAN %q VAT %q KvK %q zip %q; want %q %q %q %q", s.IBAN, s.VATNumber, s.KVK, s.Zipcode, tc.iban, tc.vat, tc.kvk, tc.zip)
			}
			if s.Name != "Example Supplies B.V." {
				t.Errorf("name = %q", s.Name)
			}
		})
	}
}

func TestKVKWithSpaces(t *testing.T) {
	lines := prepare("Example Supplies B.V.\nKvK. 12 34 56 78\n")
	if s, _ := readSupplier(lines, buyerLines(lines), Self{}); s.KVK != "12345678" {
		t.Errorf("KvK = %q, want 12345678", s.KVK)
	}
	c := contacts.Contact{ID: 1, CompanyName: "Other", ChamberOfCommerce: "12 34 56 78"}
	if got, by, ok := Match(Supplier{KVK: "12345678"}, []contacts.Contact{c}, Self{}); !ok || got.ID != 1 || by != ByKVK {
		t.Errorf("Match() = %d, %q, %v; want the contact by KvK", got.ID, by, ok)
	}
}

func TestSelfIs(t *testing.T) {
	for name, tc := range map[string]struct {
		s    Supplier
		want bool
	}{
		"same name":                {Supplier{Name: "Example Shop"}, true},
		"same name, other case":    {Supplier{Name: "EXAMPLE SHOP BV"}, true},
		"an OCR slip of the name":  {Supplier{Name: "Exarnple Shop B.V."}, true},
		"one letter off, one word": {Supplier{Name: "ExampleShip"}, false},
		"another name":             {Supplier{Name: "Example Supplies"}, false},
		"the VAT number":           {Supplier{Name: "Other", VATNumber: "NL 9876.54.329.B01"}, true},
		"the KvK number":           {Supplier{KVK: "8765 4321"}, true},
		"the IBAN":                 {Supplier{IBAN: "NL20INGB0001234567"}, true},
		"nothing":                  {Supplier{}, false},
	} {
		t.Run(name, func(t *testing.T) {
			if got := buyer.Is(tc.s); got != tc.want {
				t.Errorf("Is(%+v) = %v, want %v", tc.s, got, tc.want)
			}
		})
	}
	if (Self{}).Is(Supplier{Name: "Example Shop"}) {
		t.Error("an unknown administration is everyone")
	}
}

func TestSelfWithout(t *testing.T) {
	got := buyer.Without(Supplier{
		Name: "Example Shop", VATNumber: "NL123456782B01", KVK: "87654321", IBAN: "NL20INGB0001234567",
		Address1: "Example Street 31", Zipcode: "1234 AB", City: "Amsterdam", Country: "NL", Email: "info@example.com",
	})
	want := Supplier{VATNumber: "NL123456782B01", Country: "NL", Email: "info@example.com"}
	if got != want {
		t.Errorf("Without() = %+v, want %+v", got, want)
	}
}

// The administration has a contact of itself; it is never the supplier.
func TestMatchNeverTheAdministration(t *testing.T) {
	own := contacts.Contact{ID: 1, CompanyName: "Example Shop B.V."}
	ownByVAT := contacts.Contact{ID: 2, CompanyName: "Shop", TaxNumber: "NL987654329B01"}
	supplier := contacts.Contact{ID: 3, CompanyName: "Example Productions B.V.", TaxNumber: "NL123456782B01"}
	all := []contacts.Contact{own, ownByVAT, supplier}
	if c, by, ok := Match(Supplier{Name: "Example Shop"}, all, buyer); ok {
		t.Errorf("Match() = %d by %q, want none", c.ID, by)
	}
	if c, by, ok := Match(Supplier{VATNumber: "NL987654329B01"}, all, buyer); ok {
		t.Errorf("Match() = %d by %q, want none", c.ID, by)
	}
	if c, by, ok := Match(Supplier{Name: "Example Shop", VATNumber: "NL123456782B01"}, all, buyer); !ok || c.ID != 3 || by != ByVATNumber {
		t.Errorf("Match() = %d by %q, %v; want 3 by VAT number", c.ID, by, ok)
	}
	// Without knowing the administration, a name matches as before.
	if c, _, ok := Match(Supplier{Name: "Example Shop"}, all, Self{}); !ok || c.ID != 1 {
		t.Errorf("Match() = %d, %v; want 1", c.ID, ok)
	}
}

type fakeOwn struct {
	self  Self
	err   error
	asked []resource.ID
}

func (o *fakeOwn) Self(_ context.Context, admin, _ resource.ID) (Self, error) {
	o.asked = append(o.asked, admin)
	return o.self, o.err
}

// Recognition reads for the administration: the rules get its identity,
// and a document naming the administration does not make it the supplier.
func TestRecognizeNeverTakesTheAdministration(t *testing.T) {
	f := newFixture(t)
	own := &fakeOwn{self: buyer}
	f.svc.Own = own
	var gotSelf Self
	f.svc.Parsers.Text = func(_ string, _ time.Time, self Self) Extraction {
		gotSelf = self
		e := goodText()
		e.Supplier = Supplier{Name: "Example Shop", IBAN: "NL20INGB0001234567"}
		return e
	}
	f.contacts.list = []contacts.Contact{{ID: 1, CompanyName: "Example Shop B.V."}}
	f.docs.upload(10, map[string][]byte{"bill.pdf": []byte("%PDF")})
	f.ocr.text = ocr.Text{Text: "Invoice 2026-0042", Method: "text", Pages: 1}
	if err := f.svc.Queue(context.Background(), admin, 10, owner); err != nil {
		t.Fatal(err)
	}
	f.work(t)
	if gotSelf.Name != buyer.Name || len(own.asked) != 1 || own.asked[0] != admin {
		t.Errorf("the rules got %+v (asked %v), want the administration", gotSelf, own.asked)
	}
	if len(f.docs.proposed) != 1 {
		t.Fatalf("proposed = %+v", f.docs.proposed)
	}
	if c := f.docs.proposed[0].changes.ContactID; c != nil && *c == 1 {
		t.Error("the administration's own contact was taken for the supplier")
	}
	for _, c := range f.contacts.created {
		if c.CompanyName != nil && strings.Contains(*c.CompanyName, "Example Shop") {
			t.Errorf("added the administration as a supplier: %+v", c)
		}
	}
}

type fakeAdministrations struct{}

func (fakeAdministrations) Get(context.Context, resource.ID, resource.ID) (administration.Listed, error) {
	return administration.Listed{Administration: administration.Administration{
		Name: "Example", Profile: administration.Profile{CompanyWebsiteURL: "https://shop.example.com"},
	}}, nil
}

func (fakeAdministrations) DefaultIdentity(context.Context, resource.ID, resource.ID) (administration.Identity, error) {
	return administration.Identity{
		CompanyName: "Example Shop B.V.", Address1: "Example Street 31", Zipcode: "1234 AB", TaxNumber: "NL987654329B01",
		ChamberOfCommerce: "87654321", Email: "info@example.com", BankAccountNumber: "NL20INGB0001234567",
	}, nil
}

type fakeAccounts struct{ err error }

func (a fakeAccounts) Accounts(context.Context, resource.ID, resource.ID) ([]banking.Account, error) {
	return []banking.Account{{Identifier: "NL91 ABNA 0417 1643 00"}, {Identifier: "1234"}}, a.err
}

func TestOwnCompany(t *testing.T) {
	ctx := context.Background()
	got, err := OwnCompany{Administrations: fakeAdministrations{}, Accounts: fakeAccounts{}}.Self(ctx, admin, owner)
	if err != nil {
		t.Fatal(err)
	}
	want := Self{
		Name: "Example Shop B.V.", Address1: "Example Street 31", Zipcode: "1234 AB", VATNumber: "NL987654329B01", KVK: "87654321",
		Email: "info@example.com", Website: "https://shop.example.com", IBANs: []string{"NL20INGB0001234567", "NL91ABNA0417164300"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Self() = %+v\nwant %+v", got, want)
	}
	// A user who may not see the bank accounts still gets the rest.
	got, err = OwnCompany{Administrations: fakeAdministrations{}, Accounts: fakeAccounts{err: errors.New("forbidden")}}.Self(ctx, admin, owner)
	if err != nil || got.Name != want.Name || len(got.IBANs) != 1 {
		t.Errorf("Self() = %+v, %v", got, err)
	}
}

// Without its own company, a document is read as if for anyone.
func TestRecognizeWithoutOwnCompany(t *testing.T) {
	f := newFixture(t)
	f.svc.Own = &fakeOwn{err: errors.New("unavailable")}
	f.text = goodText()
	f.contacts.list = []contacts.Contact{{ID: 77, CompanyName: "Example Shop B.V.", SepaIBAN: "NL91ABNA0417164300"}}
	f.docs.upload(10, map[string][]byte{"bill.pdf": []byte("%PDF")})
	f.ocr.text = ocr.Text{Text: "Invoice 2026-0042", Method: "text", Pages: 1}
	if err := f.svc.Queue(context.Background(), admin, 10, owner); err != nil {
		t.Fatal(err)
	}
	f.work(t)
	if len(f.docs.proposed) != 1 || f.docs.proposed[0].changes.ContactID == nil || *f.docs.proposed[0].changes.ContactID != 77 {
		t.Fatalf("proposed = %+v", f.docs.proposed)
	}
}
