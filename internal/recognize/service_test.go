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
			Text: func(string, time.Time) Extraction { f.texts++; return f.text },
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
	f.store.mems[77] = Memory{ContactID: 77, Kind: Learned[Kind]{V: KindReceipt, Uses: 2}, Lines: map[string]Category{"": {LedgerAccountID: 500, TaxRateID: 1, Uses: 2}}}
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
	poor := Extraction{Supplier: Supplier{Name: "Example Shop"}, Reference: uncertain("A-1")}
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
	f.store.mems[77] = Memory{ContactID: 77, Lines: map[string]Category{"": {LedgerAccountID: 500, Uses: 3}}}
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

	f.svc.Edited(ctx, admin, owner, before, after)

	got := fieldNames(f.recognition(t, 10).Fields)
	if _, ok := got[FieldReference]; ok || len(got) != 2 {
		t.Errorf("fields = %v, want contact_id and details.0.description", got)
	}
	m := f.store.mems[77]
	if m.Kind.V != KindPurchaseInvoice || m.Lines["hosting"].LedgerAccountID != ledger || m.Currency.V != "EUR" {
		t.Errorf("memory = %+v", m)
	}

	// A new document teaches nothing.
	f.svc.Edited(ctx, admin, owner, purchases.Document{}, purchases.Document{ID: 20, ContactID: 78, State: purchases.New})
	if _, ok := f.store.mems[78]; ok {
		t.Error("learned from a new document")
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
