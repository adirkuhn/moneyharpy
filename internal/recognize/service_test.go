package recognize

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/adirkuhn/moneyharpy/internal/contacts"
	"github.com/adirkuhn/moneyharpy/internal/money"
	"github.com/adirkuhn/moneyharpy/internal/ocr"
	"github.com/adirkuhn/moneyharpy/internal/purchases"
	"github.com/adirkuhn/moneyharpy/internal/resource"
	"github.com/adirkuhn/moneyharpy/internal/sales"
	"github.com/adirkuhn/moneyharpy/internal/settings"
)

const (
	admin resource.ID = 1
	owner resource.ID = 2
)

type fixture struct {
	svc      *Service
	store    *fakeStore
	docs     *fakeDocuments
	contacts *fakeContacts
	ocr      *fakeOCR
	model    *fakeModel
	settings *fakeSettings
	notifier *fakeNotifier
	clock    *clock
	// parsed counts the calls of the parsers.
	eInvoices, texts int
	eInvoice         Extraction
	text             Extraction
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	f := &fixture{
		store: newFakeStore(), docs: newFakeDocuments(), contacts: &fakeContacts{}, ocr: &fakeOCR{}, model: &fakeModel{},
		settings: &fakeSettings{}, notifier: &fakeNotifier{}, clock: &clock{t: time.Date(2026, 11, 13, 9, 0, 0, 0, time.UTC)},
	}
	f.svc = NewService(Deps{
		Documents: f.docs, Contacts: f.contacts, Ledger: fakeLedger{}, Settings: f.settings, Store: f.store, OCR: f.ocr,
		Parsers: Parsers{
			EInvoice: func(xml []byte) (Extraction, error) {
				f.eInvoices++
				if !strings.HasPrefix(string(xml), "<Invoice") {
					return Extraction{}, errors.New("not an e-invoice")
				}
				return f.eInvoice, nil
			},
			Text: func(string, time.Time, Self) Extraction { f.texts++; return f.text },
		},
		Models:   func(settings.Recognition) (Model, error) { return f.model, nil },
		Notifier: f.notifier, Now: f.clock.now,
	})
	return f
}

// goodText is what the rules read of a digital PDF: enough to fill it in.
func goodText() Extraction {
	return Extraction{
		Supplier: Supplier{Name: "Example Shop B.V.", IBAN: "NL91ABNA0417164300"}, Reference: certain("2026-0042"),
		Date: certain(day("2026-11-02")), TotalInclTax: certain(money.Amount(12100)), PricesInclTax: certain(false),
		VAT: []VAT{{Rate: "21", Taxable: 10000, Tax: 2100}},
	}
}

func (f *fixture) work(t *testing.T) {
	t.Helper()
	if err := f.svc.Work(context.Background()); err != nil {
		t.Fatalf("Work() error = %v", err)
	}
}

func (f *fixture) recognition(t *testing.T, id resource.ID) Recognition {
	t.Helper()
	r, err := f.svc.Recognition(context.Background(), admin, id)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func TestRecognizeQueuesOnlyDocumentsThatCanBeRead(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t)
	f.docs.upload(10, map[string][]byte{"bill.pdf": []byte("%PDF")})
	f.docs.incoming[11] = purchases.Incoming{ID: 11, Kind: purchases.PurchaseInvoice, State: purchases.Open, AttachmentsCount: 1}
	f.docs.incoming[12] = purchases.Incoming{ID: 12, Kind: purchases.GeneralDocument, State: purchases.Saved, AttachmentsCount: 1}
	f.docs.incoming[13] = purchases.Incoming{ID: 13, Kind: purchases.Receipt, State: purchases.New}
	f.docs.incoming[14] = purchases.Incoming{ID: 14, Kind: purchases.Receipt, State: purchases.New, AttachmentsCount: 2}

	for _, id := range []resource.ID{11, 12, 13} {
		var verr *resource.ValidationError
		if _, err := f.svc.Recognize(ctx, admin, id, owner); !errors.As(err, &verr) {
			t.Errorf("Recognize(%d) error = %v, want a validation error", id, err)
		}
	}
	if _, err := f.svc.Recognize(ctx, admin, 99, owner); !purchases.IsNotFound(err) {
		t.Errorf("Recognize(missing) error = %v", err)
	}
	for _, id := range []resource.ID{10, 14} {
		got, err := f.svc.Recognize(ctx, admin, id, owner)
		if err != nil || got.ID != id {
			t.Fatalf("Recognize(%d) = %+v, %v", id, got, err)
		}
		if r := f.recognition(t, id); r.State != StatePending || r.UserID != owner || r.NextAttemptAt == nil {
			t.Errorf("recognition %d = %+v", id, r)
		}
	}
	if r := f.recognition(t, 11); r.State != StateNone {
		t.Errorf("never read = %+v", r)
	}
}

func TestWorkFillsInAnUploadFromItsText(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t)
	f.docs.upload(10, map[string][]byte{"bill.pdf": []byte("%PDF")})
	f.contacts.list = []contacts.Contact{{ID: 77, CompanyName: "Example Shop", SepaIBAN: "NL91ABNA0417164300"}}
	f.store.mems[77] = Memory{ContactID: 77, Kind: Learned[Kind]{V: KindReceipt, Uses: 2}, Receipts: KindMemory{Lines: map[string]Category{"": {LedgerAccountID: 500, TaxRateID: 1, Uses: 2}}}}
	f.ocr.text = ocr.Text{Text: "Invoice 2026-0042", Method: "text", Pages: 1}
	f.text = goodText()
	if err := f.svc.Queue(ctx, admin, 10, owner); err != nil {
		t.Fatal(err)
	}

	f.work(t)

	r := f.recognition(t, 10)
	if r.State != StateDone || r.Source != SourceText || r.Attempts != 1 || r.Error != "" || r.NextAttemptAt != nil {
		t.Fatalf("recognition = %+v", r)
	}
	if len(f.docs.proposed) != 1 {
		t.Fatalf("proposed = %+v", f.docs.proposed)
	}
	p := f.docs.proposed[0]
	if p.kind != purchases.Receipt || *p.changes.ContactID != 77 || *p.changes.Reference != "2026-0042" || len(p.changes.Details) != 1 ||
		*p.changes.Details[0].LedgerAccountID != 500 || *p.changes.Details[0].Price != "100.0" {
		t.Errorf("proposal = %+v", p)
	}
	fields := fieldNames(r.Fields)
	if fields[FieldContactID] != Certain || fields[FieldDocumentType] != Certain || fields["details.0.ledger_account_id"] != Certain {
		t.Errorf("fields = %v", fields)
	}
	if f.eInvoices != 0 || f.model.got != "" || len(f.contacts.created) != 0 {
		t.Errorf("e-invoice parser %d, model %q, created %v", f.eInvoices, f.model.got, f.contacts.created)
	}
	if len(f.notifier.documents) != 1 {
		t.Errorf("notified %v", f.notifier.documents)
	}

	// Nothing is due any more.
	f.work(t)
	if len(f.docs.proposed) != 1 {
		t.Errorf("read again: %+v", f.docs.proposed)
	}
}

func TestWorkReadsAnEInvoiceFirstAndAddsItsSupplier(t *testing.T) {
	f := newFixture(t)
	f.docs.upload(10, map[string][]byte{"a.pdf": []byte("%PDF"), "b.xml": []byte("<Invoice/>")})
	f.eInvoice = goodText()
	f.eInvoice.Supplier = Supplier{Name: "Example Shop B.V.", VATNumber: "NL123456789B01", KVK: "12345678", Country: "nl", IBAN: "NL91ABNA0417164300"}
	if err := f.svc.Queue(context.Background(), admin, 10, owner); err != nil {
		t.Fatal(err)
	}

	f.work(t)

	r := f.recognition(t, 10)
	if r.State != StateDone || r.Source != SourceEInvoice {
		t.Fatalf("recognition = %+v", r)
	}
	if f.ocr.reads != 0 || f.texts != 0 {
		t.Errorf("the e-invoice was not enough: ocr reads %d, text parses %d", f.ocr.reads, f.texts)
	}
	if len(f.contacts.created) != 1 {
		t.Fatalf("created = %+v", f.contacts.created)
	}
	c := f.contacts.created[0]
	if *c.CompanyName != "Example Shop B.V." || *c.TaxNumber != "NL123456789B01" || *c.ChamberOfCommerce != "12345678" || *c.Country != "NL" ||
		*c.SepaIBAN != "NL91ABNA0417164300" {
		t.Errorf("new contact = %+v", c)
	}
	if fieldNames(r.Fields)[FieldContactID] != Certain || len(f.notifier.contacts) != 1 {
		t.Errorf("fields = %v, contacts notified %v", r.Fields, f.notifier.contacts)
	}
}

func TestWorkAddsASupplierByNameWhenItsDetailsDoNotPass(t *testing.T) {
	f := newFixture(t)
	f.docs.upload(10, map[string][]byte{"a.pdf": []byte("%PDF")})
	f.ocr.text = ocr.Text{Text: "x", Method: "ocr"}
	f.text = goodText()
	f.text.Supplier = Supplier{Name: "Example Shop", VATNumber: "bad"}
	_ = f.svc.Queue(context.Background(), admin, 10, owner)
	f.work(t)
	if len(f.contacts.created) != 2 || f.contacts.created[1].TaxNumber != nil {
		t.Fatalf("created = %+v", f.contacts.created)
	}
	r := f.recognition(t, 10)
	if r.Source != SourceOCR || fieldNames(r.Fields)[FieldContactID] != Uncertain {
		t.Errorf("recognition = %+v", r)
	}
}

func TestWorkAddsANewSupplierUnderItsRegisteredName(t *testing.T) {
	registered := RegisteredCompany{Valid: true, Name: "EXAMPLE SHOP B.V.", Address: "EXAMPLE STREET 00001\n1234AB AMSTERDAM \n"}
	for name, tc := range map[string]struct {
		on       bool
		supplier Supplier
		existing []contacts.Contact
		err      error
		asked    []string
		created  string
		address  []string // address1, zipcode, city of the new contact
	}{
		"off": {
			supplier: Supplier{Name: "Xampl Shop BV", VATNumber: "NL123456782B01"}, created: "Xampl Shop BV",
		},
		"on": {
			on: true, supplier: Supplier{Name: "Xampl Shop BV", VATNumber: "NL 1234 56 782 B01"}, asked: []string{"NL123456782B01"},
			created: "EXAMPLE SHOP B.V.", address: []string{"EXAMPLE STREET 00001", "1234 AB", "AMSTERDAM"},
		},
		"no name read": {
			on: true, supplier: Supplier{VATNumber: "NL123456782B01"}, asked: []string{"NL123456782B01"}, created: "EXAMPLE SHOP B.V.",
		},
		"the document's address is kept": {
			on: true, supplier: Supplier{Name: "Xampl Shop BV", VATNumber: "NL123456782B01", Address1: "Example Street 1", Zipcode: "1234 AB", City: "Amsterdam"},
			asked: []string{"NL123456782B01"}, created: "EXAMPLE SHOP B.V.", address: []string{"Example Street 1", "1234 AB", "Amsterdam"},
		},
		"an existing contact with the tax number": {
			on: true, supplier: Supplier{Name: "Xampl Shop BV", VATNumber: "NL123456782B01"},
			existing: []contacts.Contact{{ID: 77, CompanyName: "Example Shop", TaxNumber: "NL 123456782 B01"}},
		},
		"VIES unavailable": {
			on: true, supplier: Supplier{Name: "Xampl Shop BV", VATNumber: "NL123456782B01"}, err: errors.New("timeout"),
			asked: []string{"NL123456782B01"}, created: "Xampl Shop BV",
		},
		"not registered": {
			on: true, supplier: Supplier{Name: "Example Shop", VATNumber: "NL000099998B57"}, asked: []string{"NL000099998B57"}, created: "Example Shop",
		},
		"checksum fails": {
			on: true, supplier: Supplier{Name: "Example Shop", VATNumber: "NL123456789B01"}, created: "Example Shop",
		},
		"outside the EU": {
			on: true, supplier: Supplier{Name: "Example Cloud Ltd", VATNumber: "GB123456789"}, created: "Example Cloud Ltd",
		},
	} {
		t.Run(name, func(t *testing.T) {
			f := newFixture(t)
			registry := &fakeRegistry{companies: map[string]RegisteredCompany{"NL123456782B01": registered}, err: tc.err}
			f.svc.Registry = registry
			f.settings.vies = tc.on
			f.contacts.list = tc.existing
			f.docs.upload(10, map[string][]byte{"receipt.jpg": []byte("\xff\xd8\xff")})
			f.ocr.text = ocr.Text{Text: "x", Method: "ocr"}
			f.text = goodText()
			f.text.Supplier = tc.supplier
			_ = f.svc.Queue(context.Background(), admin, 10, owner)
			f.work(t)
			if !slices.Equal(registry.asked, tc.asked) {
				t.Errorf("asked VIES about %q, want %q", registry.asked, tc.asked)
			}
			if tc.created == "" {
				if len(f.contacts.created) != 0 || *f.docs.proposed[0].changes.ContactID != 77 {
					t.Errorf("created %+v, want the existing contact", f.contacts.created)
				}
				return
			}
			if len(f.contacts.created) != 1 || *f.contacts.created[0].CompanyName != tc.created {
				t.Fatalf("created %+v, want %q", f.contacts.created, tc.created)
			}
			if tc.address != nil {
				c := f.contacts.created[0]
				if got := []string{deref(c.Address1), deref(c.Zipcode), deref(c.City)}; !slices.Equal(got, tc.address) {
					t.Errorf("address %q, want %q", got, tc.address)
				}
			}
		})
	}
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func TestRegisteredAddress(t *testing.T) {
	for in, want := range map[string][]string{
		"EXAMPLE STREET 00001\n1234AB AMSTERDAM \n":       {"EXAMPLE STREET 00001", "1234 AB", "AMSTERDAM"},
		"EXAMPLE STREET 1\nBUILDING 2\n1234 AB AMSTERDAM": {"EXAMPLE STREET 1, BUILDING 2", "1234 AB", "AMSTERDAM"},
		"---":                       {"", "", ""},
		"EXAMPLE STREET 1\nNOWHERE": {"", "", ""},
	} {
		a, z, c := registeredAddress(in, "NL")
		if got := []string{a, z, c}; !slices.Equal(got, want) {
			t.Errorf("registeredAddress(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestSplitRegisteredAddress(t *testing.T) {
	for name, tc := range map[string]struct {
		addr, vat string
		want      []string
	}{
		"Dutch":                    {"EXAMPLE STREET 00001\n1234AB AMSTERDAM \n", "NL123456782B01", []string{"EXAMPLE STREET 00001", "1234 AB", "AMSTERDAM"}},
		"Dutch with a second line": {"EXAMPLE STREET 1\nBUILDING 2\n1234 AB AMSTERDAM", "NL123456782B01", []string{"EXAMPLE STREET 1, BUILDING 2", "1234 AB", "AMSTERDAM"}},
		"Belgian":                  {"Example Street 1\n1000 Brussel", "BE0123456789", []string{"Example Street 1", "1000", "Brussel"}},
		"German":                   {"Example Street 1\n10115 Berlin", "DE123456789", []string{"Example Street 1", "10115", "Berlin"}},
		"French":                   {"1 RUE EXEMPLE\n75001 PARIS", "FR12345678901", []string{"1 RUE EXEMPLE", "75001", "PARIS"}},
		"Greek, prefixed EL":       {"EXAMPLE STREET 1\n10431 ATHINA", "EL123456789", []string{"EXAMPLE STREET 1", "10431", "ATHINA"}},
		"Portuguese":               {"RUA EXEMPLO 1\n1000-001 LISBOA", "PT123456789", []string{"RUA EXEMPLO 1", "1000-001", "LISBOA"}},
		"no postcode keeps it all": {"EXAMPLE STREET 1\nNOWHERE", "NL123456782B01", []string{"EXAMPLE STREET 1, NOWHERE", "", ""}},
		"one line keeps it all":    {"EXAMPLE STREET 1 1234 AB AMSTERDAM", "NL123456782B01", []string{"EXAMPLE STREET 1 1234 AB AMSTERDAM", "", ""}},
		"unknown postcode shape":   {"1 EXAMPLE ROAD\nDUBLIN 2", "IE1234567T", []string{"1 EXAMPLE ROAD, DUBLIN 2", "", ""}},
		"nothing":                  {"", "NL123456782B01", []string{"", "", ""}},
	} {
		a, z, c := SplitRegisteredAddress(tc.addr, tc.vat)
		if got := []string{a, z, c}; !slices.Equal(got, tc.want) {
			t.Errorf("%s: SplitRegisteredAddress(%q) = %q, want %q", name, tc.addr, got, tc.want)
		}
	}
}

func TestVATCountry(t *testing.T) {
	for vat, want := range map[string]string{"NL123456782B01": "NL", "el 123456789": "GR", "XI123456789": "GB", "": ""} {
		if got := VATCountry(vat); got != want {
			t.Errorf("VATCountry(%q) = %q, want %q", vat, got, want)
		}
	}
}

func TestWorkRetriesWhileTheOCRServiceIsUnavailable(t *testing.T) {
	f := newFixture(t)
	f.docs.upload(10, map[string][]byte{"a.pdf": []byte("%PDF")})
	f.ocr.err = ocr.ErrUnavailable
	_ = f.svc.Queue(context.Background(), admin, 10, owner)

	start := f.clock.t
	for attempt, wait := range []time.Duration{time.Minute, 4 * time.Minute} {
		f.work(t)
		r := f.recognition(t, 10)
		if r.State != StatePending || r.Attempts != attempt+1 || r.NextAttemptAt == nil || !r.NextAttemptAt.Equal(f.clock.t.Add(wait)) {
			t.Fatalf("after attempt %d: %+v", attempt+1, r)
		}
		f.work(t) // not due yet
		if got := f.recognition(t, 10); got.Attempts != attempt+1 {
			t.Fatalf("read before it was due: %+v", got)
		}
		f.clock.t = r.NextAttemptAt.Add(time.Second)
	}
	f.work(t)
	r := f.recognition(t, 10)
	if r.State != StateFailed || r.Error != "OCR service unavailable" || r.Attempts != MaxAttempts || r.NextAttemptAt != nil {
		t.Errorf("after the last attempt: %+v", r)
	}
	if len(f.docs.proposed) != 0 || f.clock.t.Sub(start) < 5*time.Minute {
		t.Errorf("proposed = %+v", f.docs.proposed)
	}
}

func TestWorkWithoutAnOCRServiceReadsOnlyEInvoices(t *testing.T) {
	f := newFixture(t)
	f.svc.OCR = nil
	f.docs.upload(10, map[string][]byte{"b.xml": []byte("<Invoice/>")})
	f.eInvoice = goodText()
	_ = f.svc.Queue(context.Background(), admin, 10, owner)
	f.work(t)
	if r := f.recognition(t, 10); r.State != StateDone {
		t.Errorf("recognition = %+v", r)
	}
}

func TestWorkAsksTheModelOnlyWhenTheRulesFallShortAndItIsOn(t *testing.T) {
	poor := Extraction{Supplier: Supplier{Name: "Example Shop"}, Reference: certain("A-1")}
	for _, on := range []bool{false, true} {
		f := newFixture(t)
		f.settings.rec = settings.Recognition{AIEnabled: on, Connections: []settings.AIConnection{{Name: "Ollama", Provider: "ollama", Model: "m", Enabled: true}}}
		f.docs.upload(10, map[string][]byte{"a.pdf": []byte("%PDF")})
		f.ocr.text, f.ocr.pages = ocr.Text{Text: "scanned text", Method: "ocr", Pages: 7}, 7
		f.text = poor
		f.model.out = Extraction{Date: certain(day("2026-11-01")), TotalInclTax: certain(money.Amount(5000))}
		_ = f.svc.Queue(context.Background(), admin, 10, owner)
		f.work(t)
		r := f.recognition(t, 10)
		if !on {
			if f.model.got != "" || r.Source != SourceOCR {
				t.Errorf("model off: asked %q, recognition %+v", f.model.got, r)
			}
			continue
		}
		if f.model.got != "scanned text" || f.model.pages != maxPages || r.Source != SourceAI {
			t.Fatalf("model on: asked %q with %d pages, recognition %+v", f.model.got, f.model.pages, r)
		}
		p := f.docs.proposed[0].changes
		if *p.Reference != "A-1" || *p.Date != "2026-11-01" || *p.Details[0].Price != "50.0" {
			t.Errorf("merged proposal = %+v", p)
		}
	}
}

func TestWorkTakesTheOCRReadingTheRulesReadBest(t *testing.T) {
	f := newFixture(t)
	f.docs.upload(10, map[string][]byte{"receipt.jpg": []byte("\xff\xd8\xff")})
	f.ocr.text = ocr.Text{Text: "psm 3", Method: "ocr", Pages: 1, Alternatives: []ocr.Reading{{Method: "tesseract-psm6", Text: "psm 6"}, {Method: "tesseract-psm4", Text: "psm 4"}}}
	var parsed []string
	f.svc.Parsers.Text = func(text string, _ time.Time, _ Self) Extraction {
		parsed = append(parsed, text)
		if text == "psm 6" {
			return goodText()
		}
		return Extraction{TotalInclTax: uncertain(money.Amount(2913))}
	}
	f.settings.rec = settings.Recognition{AIEnabled: true, Connections: []settings.AIConnection{{Name: "Ollama", Provider: "ollama", Model: "m", Enabled: true}}}
	_ = f.svc.Queue(context.Background(), admin, 10, owner)
	f.work(t)
	if !slices.Equal(parsed, []string{"psm 3", "psm 6", "psm 4"}) {
		t.Errorf("parsed %q, want every reading", parsed)
	}
	r := f.recognition(t, 10)
	if r.State != StateDone || r.Source != SourceOCR || f.model.got != "" {
		t.Errorf("recognition = %+v, model asked %q: the psm 6 reading is good enough", r, f.model.got)
	}
	if p := f.docs.proposed[0].changes; *p.Reference != "2026-0042" {
		t.Errorf("proposal = %+v", p)
	}
}

func TestWorkGivesTheModelTheBestReadingAndLargePages(t *testing.T) {
	f := newFixture(t)
	f.docs.upload(10, map[string][]byte{"receipt.jpg": []byte("\xff\xd8\xff")})
	f.ocr.text, f.ocr.pages = ocr.Text{Text: "psm 3", Method: "ocr", Pages: 1, Alternatives: []ocr.Reading{{Method: "tesseract-psm6", Text: "psm 6"}}}, 1
	f.svc.Parsers.Text = func(text string, _ time.Time, _ Self) Extraction {
		if text == "psm 6" {
			return Extraction{TotalInclTax: uncertain(money.Amount(3630)), Reference: uncertain("0042")}
		}
		return Extraction{}
	}
	f.settings.rec = settings.Recognition{AIEnabled: true, Connections: []settings.AIConnection{{Name: "Ollama", Provider: "ollama", Model: "m", Enabled: true}}}
	f.model.out = Extraction{Date: uncertain(day("2026-10-20")), TotalInclTax: uncertain(money.Amount(3630))}
	_ = f.svc.Queue(context.Background(), admin, 10, owner)
	f.work(t)
	if f.model.got != "psm 6" || !slices.Equal(f.ocr.sides, []int{aiPageSide}) {
		t.Errorf("model got %q, pages rendered at %v", f.model.got, f.ocr.sides)
	}
}

func TestWorkFailsWhenNothingIsReadAndLeavesTheUpload(t *testing.T) {
	f := newFixture(t)
	f.docs.upload(10, map[string][]byte{"a.pdf": []byte("%PDF")})
	f.ocr.text = ocr.Text{Text: "   ", Method: "ocr"}
	_ = f.svc.Queue(context.Background(), admin, 10, owner)
	f.work(t)
	r := f.recognition(t, 10)
	if r.State != StateFailed || r.Error != "Nothing could be read from the attachments" || len(f.docs.proposed) != 0 {
		t.Errorf("recognition = %+v, proposed %+v", r, f.docs.proposed)
	}
}

func TestWorkLeavesOutRememberedCategoriesThatNoLongerPass(t *testing.T) {
	f := newFixture(t)
	f.docs.upload(10, map[string][]byte{"a.pdf": []byte("%PDF")})
	f.contacts.list = []contacts.Contact{{ID: 77, CompanyName: "Example Shop", SepaIBAN: "NL91ABNA0417164300"}}
	f.store.mems[77] = Memory{ContactID: 77, Invoices: KindMemory{Lines: map[string]Category{"": {LedgerAccountID: 500, Uses: 3}}}}
	f.ocr.text = ocr.Text{Text: "x", Method: "text"}
	f.text = goodText()
	var verr resource.ValidationError
	verr.Add("details_attributes.0.ledger_account_id", "is deactivated")
	f.docs.refuse = []error{&verr}
	_ = f.svc.Queue(context.Background(), admin, 10, owner)
	f.work(t)
	r := f.recognition(t, 10)
	if r.State != StateDone || len(f.docs.proposed) != 2 || f.docs.proposed[1].changes.Details[0].LedgerAccountID != nil {
		t.Fatalf("recognition = %+v, proposed %+v", r, f.docs.proposed)
	}
	if _, ok := fieldNames(r.Fields)["details.0.ledger_account_id"]; ok {
		t.Errorf("fields = %v", r.Fields)
	}

	// Refused twice: failed with the reason, the upload untouched.
	f.docs.upload(11, map[string][]byte{"a.pdf": []byte("%PDF")})
	f.docs.refuse = []error{&verr, &verr}
	_ = f.svc.Queue(context.Background(), admin, 11, owner)
	f.work(t)
	if r := f.recognition(t, 11); r.State != StateFailed || !strings.Contains(r.Error, "could not be filled in") {
		t.Errorf("recognition = %+v", r)
	}
}

func TestWorkOnDocumentsThatChangedMeanwhile(t *testing.T) {
	f := newFixture(t)
	f.docs.upload(10, map[string][]byte{"a.pdf": []byte("%PDF")})
	_ = f.svc.Queue(context.Background(), admin, 10, owner)
	_ = f.svc.Queue(context.Background(), admin, 11, owner) // removed since
	f.docs.incoming[12] = purchases.Incoming{ID: 12, Kind: purchases.PurchaseInvoice, State: purchases.Open}
	f.docs.documents[12] = purchases.Document{ID: 12, Kind: purchases.PurchaseInvoice, State: purchases.Open}
	_ = f.svc.Queue(context.Background(), admin, 12, owner) // booked since
	f.docs.typeless[10] = purchases.Typeless{ID: 10}        // attachment removed since

	f.work(t)

	if r := f.recognition(t, 10); r.State != StateNone {
		t.Errorf("without attachments = %+v", r)
	}
	if _, ok := f.store.recs[11]; ok {
		t.Error("removed document still has a recognition")
	}
	if r := f.recognition(t, 12); r.State != StateNone {
		t.Errorf("booked = %+v", r)
	}
}

func TestEditedUnmarksChangedFieldsAndLearnsFromBookedDocuments(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t)
	ledger := resource.ID(500)
	before := purchases.Document{
		ID: 10, Kind: purchases.PurchaseInvoice, State: purchases.New, ContactID: 77, Reference: "A", Currency: "EUR",
		Lines: []sales.Detail{{ID: 1, Description: "Hosting", Price: 100, TaxRateID: 1}},
	}
	f.store.recs[10] = Recognition{DocumentID: 10, State: StateDone, Fields: []Field{
		{Name: FieldReference}, {Name: FieldContactID}, {Name: "details.0.price"}, {Name: "details.0.description"},
	}}
	after := before
	after.Lines = []sales.Detail{{ID: 1, Description: "Hosting", Price: 150, TaxRateID: 1, LedgerAccountID: &ledger}}
	after.Reference, after.State = "B", purchases.Open

	f.svc.Edited(ctx, admin, owner, before, after, nil)

	got := fieldNames(f.recognition(t, 10).Fields)
	if _, ok := got[FieldReference]; ok || len(got) != 2 {
		t.Errorf("fields = %v, want contact_id and details.0.description", got)
	}
	m := f.store.mems[77]
	if m.Kind.V != KindPurchaseInvoice || m.Invoices.Lines["hosting"].LedgerAccountID != ledger || m.Invoices.Currency.V != "EUR" {
		t.Errorf("memory = %+v", m)
	}

	// A new document teaches nothing.
	f.svc.Edited(ctx, admin, owner, purchases.Document{}, purchases.Document{ID: 20, ContactID: 78, State: purchases.New}, nil)
	if _, ok := f.store.mems[78]; ok {
		t.Error("learned from a new document")
	}
}

func TestConfirmedUnmarksSavedFieldsEvenUnchanged(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t)
	f.store.recs[10] = Recognition{DocumentID: 10, State: StateDone, Fields: []Field{
		{Name: FieldReference}, {Name: FieldContactID}, {Name: FieldDocumentType}, {Name: "details.0.price"},
	}}
	f.svc.Confirmed(ctx, admin, 10, FieldReference, FieldDocumentType)
	got := fieldNames(f.recognition(t, 10).Fields)
	if _, ok := got[FieldReference]; ok || len(got) != 2 {
		t.Errorf("fields = %v, want contact_id and details.0.price", got)
	}
	if _, ok := got[FieldDocumentType]; ok {
		t.Errorf("fields = %v, still the type", got)
	}
	// A document nothing was read of has nothing to unmark.
	f.svc.Confirmed(ctx, admin, 11, FieldReference)
	if _, ok := f.store.recs[11]; ok {
		t.Error("made a recognition")
	}
}

func TestChangedFieldsOfReorderedLines(t *testing.T) {
	before := purchases.Document{Lines: []sales.Detail{{ID: 1}, {ID: 2}}}
	after := purchases.Document{Lines: []sales.Detail{{ID: 2}}}
	got := ChangedFields(before, after)
	if len(got) != 10 {
		t.Errorf("ChangedFields() = %v, want every field of both lines", got)
	}
}

func TestCheck(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t)
	c, err := f.svc.Check(ctx)
	if err != nil || !c.OCRAvailable || c.AI != AIOff || c.Message != "" {
		t.Errorf("Check() = %+v, %v", c, err)
	}
	f.settings.rec = settings.Recognition{AIEnabled: true, Connections: []settings.AIConnection{{ID: "a", Name: "Ollama", Provider: "ollama", Enabled: true}}}
	f.model.err = errors.New("connection refused")
	f.ocr.err = ocr.ErrUnavailable
	c, err = f.svc.Check(ctx)
	if err != nil || c.OCRAvailable || c.AI != AIUnavailable || !strings.Contains(c.Message, "connection refused") {
		t.Errorf("Check() = %+v, %v", c, err)
	}
	f.model.err = nil
	if c, _ = f.svc.Check(ctx); c.AI != AIOK {
		t.Errorf("Check() = %+v", c)
	}
}

func TestCheckEachConnection(t *testing.T) {
	f := newFixture(t)
	f.settings.rec = settings.Recognition{AIEnabled: true, Connections: []settings.AIConnection{
		{ID: "a", Name: "NVIDIA", Provider: "nvidia", Enabled: true},
		{ID: "b", Name: "Ollama", Provider: "ollama", Enabled: true},
		{ID: "c", Name: "Claude", Provider: "anthropic"},
	}}
	f.svc.Models = func(r settings.Recognition) (Model, error) {
		if len(r.Connections) != 1 {
			t.Errorf("checked %d connections at once", len(r.Connections))
		}
		if r.Connections[0].ID == "a" {
			return &fakeModel{err: errors.New("rate limited")}, nil
		}
		return &fakeModel{}, nil
	}
	c, err := f.svc.Check(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	want := []ConnectionCheck{
		{ID: "a", Name: "NVIDIA", State: ConnectionUnavailable, Message: "rate limited"},
		{ID: "b", Name: "Ollama", State: ConnectionOK},
		{ID: "c", Name: "Claude", State: ConnectionDisabled},
	}
	if c.AI != AIOK || !slices.Equal(c.Connections, want) {
		t.Errorf("Check() = %+v", c)
	}

	f.settings.rec.AIEnabled = false
	if c, _ := f.svc.Check(context.Background()); c.AI != AIOff || len(c.Connections) != 3 {
		t.Errorf("off: Check() = %+v", c)
	}
	f.settings.rec = settings.Recognition{AIEnabled: true, Connections: f.settings.rec.Connections[:1]}
	if c, _ := f.svc.Check(context.Background()); c.AI != AIUnavailable {
		t.Errorf("none answers: Check() = %+v", c)
	}
}
