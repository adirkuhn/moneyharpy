package recognize

import (
	"context"
	"testing"

	"github.com/adirkuhn/moneyharpy/internal/contacts"
	"github.com/adirkuhn/moneyharpy/internal/ocr"
	"github.com/adirkuhn/moneyharpy/internal/purchases"
	"github.com/adirkuhn/moneyharpy/internal/resource"
	"github.com/adirkuhn/moneyharpy/internal/sales"
)

func fieldsByName(fs []Field) map[string]Field {
	out := map[string]Field{}
	for _, f := range fs {
		out[f.Name] = f
	}
	return out
}

// booked is a booked document of contact 77 with one line.
func booked(id resource.ID, kind, description string, ledger, rate resource.ID) purchases.Document {
	return purchases.Document{
		ID: id, Kind: kind, ContactID: 77, State: purchases.Open, Currency: "EUR", Date: day("2026-10-01").AddDate(0, 0, int(id)),
		Lines: []sales.Detail{{ID: id * 10, Description: description, Price: 1000, TaxRateID: rate, LedgerAccountID: ledgerPtr(ledger)}},
	}
}

func TestScanLearnsFromTheSuppliersBookedDocumentsFirst(t *testing.T) {
	f := newFixture(t)
	f.docs.upload(10, map[string][]byte{"bill.pdf": []byte("%PDF")})
	f.contacts.list = []contacts.Contact{{ID: 77, CompanyName: "Example Shop", SepaIBAN: "NL91ABNA0417164300"}}
	// Booked before Scan and Recognise remembered anything, or entered by
	// hand: one purchase invoice categorised, one left uncategorised.
	f.store.history[77] = []purchases.Document{
		booked(31, purchases.PurchaseInvoice, "Example Shop B.V.", 500, 1),
		booked(32, purchases.PurchaseInvoice, "Other", uncategorized, 1),
	}
	f.docs.incoming[31] = purchases.Incoming{ID: 31, Kind: purchases.PurchaseInvoice, Reference: "EX-31"}
	f.ocr.text = ocr.Text{Text: "x", Method: "text"}
	f.text = goodText()
	if err := f.svc.Queue(context.Background(), admin, 10, owner); err != nil {
		t.Fatal(err)
	}

	f.work(t)

	if len(f.docs.proposed) != 1 {
		t.Fatalf("proposed = %+v", f.docs.proposed)
	}
	p := f.docs.proposed[0]
	if p.kind != purchases.PurchaseInvoice || len(p.changes.Details) != 1 || p.changes.Details[0].LedgerAccountID == nil || *p.changes.Details[0].LedgerAccountID != 500 {
		t.Fatalf("proposal = %+v", p.changes)
	}
	ledger := fieldsByName(f.recognition(t, 10).Fields)["details.0.ledger_account_id"]
	if ledger.Confidence != Uncertain || ledger.Reason != ReasonPreviousDocument || ledger.SourceDocumentID != 31 || ledger.SourceReference != "EX-31" {
		t.Errorf("ledger field = %+v", ledger)
	}
	if rate := fieldsByName(f.recognition(t, 10).Fields)["details.0.tax_rate_id"]; rate.Reason != ReasonAttachment {
		t.Errorf("rate field = %+v, read from the attachment", rate)
	}
	m, ok := f.store.mems[77]
	if _, other := m.Invoices.Lines["other"]; !ok || other || m.Invoices.Lines["example shop"].LedgerAccountID != 500 {
		t.Errorf("saved memory = %+v, want the categorised line only", m)
	}
	if f.store.historyCalls != 2 {
		t.Errorf("history asked %d times, want once per kind", f.store.historyCalls)
	}
}

func TestHistoryIsNotAskedOnceAKindIsRemembered(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t)
	f.store.mems[77] = Memory{ContactID: 77, Invoices: KindMemory{Lines: map[string]Category{"": {LedgerAccountID: 500, Uses: 1}}}}
	f.store.history[77] = []purchases.Document{booked(31, purchases.Receipt, "Coffee", 501, 2)}
	m, err := f.svc.memory(ctx, admin, owner, 77, 0)
	if err != nil {
		t.Fatal(err)
	}
	if c, _ := m.Recall(KindReceipt).Category("Coffee"); c.LedgerAccountID != 501 {
		t.Errorf("receipts = %+v, want learned from their history", m.Receipts)
	}
	f.store.historyCalls = 0
	if _, err := f.svc.memory(ctx, admin, owner, 77, 0); err != nil || f.store.historyCalls != 0 {
		t.Errorf("history asked %d times for a remembered supplier, %v", f.store.historyCalls, err)
	}
}

func TestEditedLearnsABookedDocumentOnce(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t)
	// The document is booked, so the history holds it already.
	doc := booked(31, purchases.PurchaseInvoice, "Hosting", 500, 1)
	f.store.history[77] = []purchases.Document{doc}
	before := doc
	before.State = purchases.New
	f.svc.Edited(ctx, admin, owner, before, doc, nil)
	if c := f.store.mems[77].Invoices.Lines["hosting"]; c.Uses != 1 || c.SourceDocumentID != 31 {
		t.Errorf("learned = %+v, want once", c)
	}
}

// changing is a new purchase invoice whose contact a user changes from 77
// to 78; Scan and Recognise filled it in with what it remembered of 77.
func changing(f *fixture) (before, after purchases.Document) {
	f.store.mems[78] = Memory{ContactID: 78, Invoices: KindMemory{
		Currency: Learned[string]{V: "USD", Uses: 2}, PricesInclTax: Learned[bool]{V: true, Uses: 2}, LastDocumentID: 41,
		Lines: map[string]Category{
			"hosting": {LedgerAccountID: 600, TaxRateID: 2, Uses: 2, SourceDocumentID: 41},
			"":        {LedgerAccountID: 601, TaxRateID: 2, Uses: 1, SourceDocumentID: 42},
		},
	}}
	f.docs.incoming[41] = purchases.Incoming{ID: 41, Kind: purchases.PurchaseInvoice, Reference: "EX-41"}
	f.docs.incoming[42] = purchases.Incoming{ID: 42, Kind: purchases.PurchaseInvoice, Reference: "EX-42"}
	f.store.recs[10] = Recognition{DocumentID: 10, AdministrationID: admin, State: StateDone, Source: SourceText, Fields: []Field{
		{Name: FieldContactID, Confidence: Uncertain, Reason: ReasonAttachment},
		{Name: FieldCurrency, Confidence: Uncertain, Reason: ReasonPreviousDocument, SourceDocumentID: 5},
		{Name: FieldPricesAreInclTax, Confidence: Certain, Reason: ReasonAttachment},
		{Name: "details.0.price", Confidence: Certain, Reason: ReasonAttachment},
		{Name: "details.0.ledger_account_id", Confidence: Uncertain, Reason: ReasonPreviousDocument, SourceDocumentID: 5},
		{Name: "details.0.tax_rate_id", Confidence: Uncertain, Reason: ReasonDefault},
		{Name: "details.1.tax_rate_id", Confidence: Certain, Reason: ReasonAttachment},
	}}
	before = purchases.Document{
		ID: 10, Kind: purchases.PurchaseInvoice, State: purchases.New, ContactID: 77, Currency: "EUR", PricesAreInclTax: false,
		Lines: []sales.Detail{
			{ID: 101, Description: "Hosting November", Price: 1000, TaxRateID: 1, LedgerAccountID: ledgerPtr(500)},
			{ID: 102, Description: "Domain", Price: 200, TaxRateID: 1, LedgerAccountID: ledgerPtr(uncategorized)},
			// Set by the user: not marked.
			{ID: 103, Description: "Support", Price: 300, TaxRateID: 1, LedgerAccountID: ledgerPtr(502)},
		},
	}
	after = before
	after.Lines = append([]sales.Detail(nil), before.Lines...)
	after.ContactID = 78
	return before, after
}

func TestRememberFillsInANewDocumentWhenItsContactChanges(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t)
	before, after := changing(f)

	c, remembered := f.svc.Remember(ctx, admin, owner, before, after)

	if c.Currency == nil || *c.Currency != "USD" {
		t.Errorf("currency = %v, want the remembered one in place of the one remembered of the other contact", c.Currency)
	}
	if c.PricesAreInclTax != nil {
		t.Errorf("prices incl. VAT = %v, want what the attachment said kept", *c.PricesAreInclTax)
	}
	byID := map[resource.ID]sales.DetailInput{}
	for _, d := range c.Details {
		byID[*d.ID] = d
	}
	if d := byID[101]; d.LedgerAccountID == nil || *d.LedgerAccountID != 600 || d.TaxRateID == nil || *d.TaxRateID != 2 {
		t.Errorf("line 0 = %+v, want the remembered category and rate", d)
	}
	if d := byID[102]; d.LedgerAccountID == nil || *d.LedgerAccountID != 601 || d.TaxRateID != nil {
		t.Errorf("line 1 = %+v, want the default category and the rate read kept", d)
	}
	if d, ok := byID[103]; ok {
		t.Errorf("line 2 = %+v, set by the user: want it left alone", d)
	}
	got := map[string]purchases.Remembered{}
	for _, r := range remembered {
		got[r.Field] = r
	}
	want := map[string]purchases.Remembered{
		FieldCurrency:                 {Field: FieldCurrency, Certain: true, SourceDocumentID: 41},
		"details.0.ledger_account_id": {Field: "details.0.ledger_account_id", Certain: true, SourceDocumentID: 41},
		"details.0.tax_rate_id":       {Field: "details.0.tax_rate_id", Certain: true, SourceDocumentID: 41},
		"details.1.ledger_account_id": {Field: "details.1.ledger_account_id", Certain: false, SourceDocumentID: 42},
	}
	if len(got) != len(want) {
		t.Errorf("remembered = %+v", remembered)
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("remembered %s = %+v, want %+v", k, got[k], v)
		}
	}

	// Saved (and booked, as it now has its supplier), the fields show
	// where they came from; the contact the user chose is no longer marked.
	saved := after
	saved.State, saved.Currency = purchases.Open, "USD"
	saved.Lines = append([]sales.Detail(nil), after.Lines...)
	saved.Lines[0].LedgerAccountID, saved.Lines[0].TaxRateID = ledgerPtr(600), 2
	saved.Lines[1].LedgerAccountID = ledgerPtr(601)
	f.svc.Edited(ctx, admin, owner, before, saved, remembered)
	fields := fieldsByName(f.recognition(t, 10).Fields)
	if _, ok := fields[FieldContactID]; ok {
		t.Error("the contact the user chose is still marked")
	}
	if l := fields["details.0.ledger_account_id"]; l.Confidence != Certain || l.Reason != ReasonPreviousDocument || l.SourceDocumentID != 41 || l.SourceReference != "EX-41" {
		t.Errorf("line 0 category = %+v", l)
	}
	if l := fields["details.1.ledger_account_id"]; l.Confidence != Uncertain || l.SourceReference != "EX-42" {
		t.Errorf("line 1 category = %+v", l)
	}
	if p := fields["details.0.price"]; p.Reason != ReasonAttachment {
		t.Errorf("price = %+v, want still marked as read", p)
	}
	if _, ok := fields["details.2.ledger_account_id"]; ok {
		t.Error("the user's category is marked")
	}
}

func TestRememberLeavesWhatTheUserSetAlone(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t)
	before, after := changing(f)
	// The user changes the category of the first line in the same request.
	after.Lines[0].LedgerAccountID = ledgerPtr(503)
	c, remembered := f.svc.Remember(ctx, admin, owner, before, after)
	for _, d := range c.Details {
		if *d.ID == 101 && d.LedgerAccountID != nil {
			t.Errorf("line 0 = %+v, want the user's category kept", d)
		}
	}
	for _, r := range remembered {
		if r.Field == "details.0.ledger_account_id" {
			t.Errorf("remembered %+v", r)
		}
	}

	// Nothing is remembered for a booked document, the same contact or no
	// contact.
	bookedBefore := before
	bookedBefore.State = purchases.Open
	bookedAfter := after
	bookedAfter.State = purchases.Open
	for name, tc := range map[string][2]purchases.Document{
		"booked":       {bookedBefore, bookedAfter},
		"same contact": {before, before},
		"no contact":   {before, func() purchases.Document { d := after; d.ContactID = 0; return d }()},
	} {
		if c, r := f.svc.Remember(ctx, admin, owner, tc[0], tc[1]); len(r) != 0 || c.Details != nil || c.Currency != nil {
			t.Errorf("%s: Remember() = %+v, %+v", name, c, r)
		}
	}
}

func TestRememberFillsADocumentEnteredByHand(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t)
	f.store.history[77] = []purchases.Document{
		booked(31, purchases.Receipt, "Coffee", 500, 2),
		booked(32, purchases.Receipt, "Coffee", 500, 2),
	}
	f.docs.incoming[32] = purchases.Incoming{ID: 32, Kind: purchases.Receipt, Reference: "EX-32"}
	// A purchase invoice, though the supplier sent receipts so far.
	d := purchases.Document{ID: 20, Kind: purchases.PurchaseInvoice, State: purchases.New, ContactID: 77, Currency: "EUR",
		Lines: []sales.Detail{{ID: 201, Description: "Coffee", Price: 500, TaxRateID: 1, LedgerAccountID: ledgerPtr(uncategorized)}}}

	c, remembered := f.svc.Remember(ctx, admin, owner, purchases.Document{}, d)

	if len(c.Details) != 1 || c.Details[0].LedgerAccountID == nil || *c.Details[0].LedgerAccountID != 500 || c.Details[0].TaxRateID != nil {
		t.Fatalf("changes = %+v, want the category; the rate is the user's", c.Details)
	}
	if len(remembered) != 1 || remembered[0].Certain {
		t.Fatalf("remembered = %+v, want uncertain from the other kind", remembered)
	}
	d.Lines[0].LedgerAccountID = ledgerPtr(500)
	f.svc.Edited(ctx, admin, owner, purchases.Document{}, d, remembered)
	r := f.recognition(t, 20)
	l := fieldsByName(r.Fields)["details.0.ledger_account_id"]
	if r.State != StateDone || l.Confidence != Uncertain || l.SourceReference != "EX-32" {
		t.Errorf("recognition = %+v", r)
	}
}
