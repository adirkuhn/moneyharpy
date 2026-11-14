package purchases

import (
	"context"
	"testing"
	"time"

	"github.com/adirkuhn/moneyharpy/internal/journal"
	"github.com/adirkuhn/moneyharpy/internal/resource"
	"github.com/adirkuhn/moneyharpy/internal/sales"
	"github.com/google/go-cmp/cmp"
)

func TestSetTypeTurnsTypelessDocumentIntoPurchaseInvoice(t *testing.T) {
	h := newHarness(t)
	tl, err := h.svc.CreateTypeless(ctx, adminID, owner, TypelessChanges{ContactID: ptr(supplier), Reference: ptr("Bill 7"), Date: ptr("2026-10-02")})
	if err != nil {
		t.Fatal(err)
	}
	h.store.number = 4
	got, err := h.svc.SetType(ctx, adminID, tl.ID, owner, "purchase_invoice")
	if err != nil {
		t.Fatalf("SetType() error = %v", err)
	}
	if got.ID != tl.ID || got.Kind != PurchaseInvoice || got.State != New || got.Currency != "EUR" {
		t.Errorf("incoming = %+v", got)
	}
	d := h.store.docs[tl.ID]
	if d.Kind != PurchaseInvoice || d.State != New || d.ContactID != supplier || d.Reference != "Bill 7" || d.EntryNumber != 5 ||
		d.Date.Format(time.DateOnly) != "2026-10-02" || d.Currency != "EUR" || d.Origin != OriginAPI || len(h.store.entries[tl.ID]) != 0 {
		t.Errorf("document = %+v", d)
	}
	if _, ok := h.store.typeless[tl.ID]; ok {
		t.Error("typeless document is still there")
	}
}

func TestSetTypeSwitchesPurchaseInvoiceAndReceiptKeepingLines(t *testing.T) {
	h := newHarness(t)
	d, err := h.svc.Create(ctx, adminID, owner, PurchaseInvoice, Changes{Reference: ptr("INV-9"), Details: []sales.DetailInput{line("100", vat21, costs)}})
	if err != nil {
		t.Fatal(err)
	}
	if d.State != New {
		t.Fatalf("state = %s, want new (no contact)", d.State)
	}
	if _, err := h.svc.SetType(ctx, adminID, d.ID, owner, "receipt"); err != nil {
		t.Fatalf("SetType() error = %v", err)
	}
	r := h.store.docs[d.ID]
	if r.Kind != Receipt || r.EntryNumber != d.EntryNumber || len(r.Lines) != 1 || r.Reference != "INV-9" {
		t.Errorf("receipt = %+v", r)
	}
}

func TestSetTypeMovesGeneralDocumentKeepingItsNumber(t *testing.T) {
	h := newHarness(t)
	g, err := h.svc.CreateGeneral(ctx, adminID, owner, GeneralChanges{ContactID: ptr(supplier), Reference: ptr("Contract"), Date: ptr("2026-09-01")})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := h.svc.SetType(ctx, adminID, g.ID, owner, "receipt"); err != nil {
		t.Fatalf("SetType() error = %v", err)
	}
	r := h.store.docs[g.ID]
	if r.Kind != Receipt || r.EntryNumber != g.EntryNumber || r.ContactID != supplier || r.Reference != "Contract" || r.State != New {
		t.Errorf("receipt = %+v", r)
	}
	got, err := h.svc.SetType(ctx, adminID, g.ID, owner, "general_document")
	if err != nil {
		t.Fatalf("SetType() back error = %v", err)
	}
	back := h.store.general[g.ID]
	if got.Kind != GeneralDocument || back.EntryNumber != g.EntryNumber || back.ContactID != supplier || back.State != Saved {
		t.Errorf("general = %+v", back)
	}
}

func TestSetTypeMakesAnEmptyJournalEntry(t *testing.T) {
	h := newHarness(t)
	tl, err := h.svc.CreateTypeless(ctx, adminID, owner, TypelessChanges{Reference: ptr("Correction"), Date: ptr("2026-10-02")})
	if err != nil {
		t.Fatal(err)
	}
	got, err := h.svc.SetType(ctx, adminID, tl.ID, owner, "general_journal_document")
	if err != nil {
		t.Fatalf("SetType() error = %v", err)
	}
	j := h.store.journals[tl.ID]
	if got.Kind != journal.GeneralJournalDocument || j.Reference != "Correction" || len(j.Rows) != 0 || h.store.number != 0 {
		t.Errorf("journal entry = %+v, number %d", j, h.store.number)
	}
	_, err = h.svc.SetType(ctx, adminID, tl.ID, owner, "receipt")
	wantInvalid(t, err, "base")
}

func TestSetTypeSwitchesABookedPurchaseInvoiceAndReceiptAndBooksThemAgain(t *testing.T) {
	h := newHarness(t)
	d, err := h.svc.Create(ctx, adminID, owner, PurchaseInvoice, invoice(line("100", vat21, costs)))
	if err != nil {
		t.Fatal(err)
	}
	if d.State != Open {
		t.Fatalf("state = %s, want open", d.State)
	}
	got, err := h.svc.SetType(ctx, adminID, d.ID, owner, "receipt")
	if err != nil {
		t.Fatalf("SetType(receipt) error = %v", err)
	}
	r := h.store.docs[d.ID]
	if got.Kind != Receipt || r.Kind != Receipt || r.State != Open || r.EntryNumber != d.EntryNumber || len(r.Lines) != 1 || r.Totals.InclTax != 12100 {
		t.Errorf("receipt = %+v", r)
	}
	entries := h.store.entries[d.ID]
	if len(entries) == 0 {
		t.Fatal("the receipt books nothing")
	}
	for _, e := range entries {
		if e.DocumentType != Receipt {
			t.Errorf("entry %+v is still booked on the purchase invoice", e)
		}
	}
	if err := journal.Balance(entries); err != nil {
		t.Errorf("entries do not balance: %v", err)
	}
	if _, err := h.svc.SetType(ctx, adminID, d.ID, owner, "purchase_invoice"); err != nil {
		t.Fatalf("SetType(purchase_invoice) error = %v", err)
	}
	if back := h.store.docs[d.ID]; back.Kind != PurchaseInvoice || back.State != Open || h.store.entries[d.ID][0].DocumentType != PurchaseInvoice {
		t.Errorf("purchase invoice = %+v", back)
	}
}

func TestSetTypeKeepsOtherTypesForNewDocuments(t *testing.T) {
	h := newHarness(t)
	d, err := h.svc.Create(ctx, adminID, owner, PurchaseInvoice, invoice(line("100", vat21, costs)))
	if err != nil {
		t.Fatal(err)
	}
	for _, to := range []string{"general_document", "general_journal_document"} {
		_, err = h.svc.SetType(ctx, adminID, d.ID, owner, to)
		wantInvalid(t, err, "base")
	}
	if h.store.docs[d.ID].Kind != PurchaseInvoice {
		t.Error("kind changed")
	}
}

func TestSetTypeRefusesAPaidDocumentAndABookedReceiptWithoutContact(t *testing.T) {
	h := newHarness(t)
	d, err := h.svc.Create(ctx, adminID, owner, PurchaseInvoice, invoice(line("100", vat21, costs)))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := h.svc.RegisterPayment(ctx, adminID, owner, PurchaseInvoice, d.ID, sales.PaymentInput{PaymentDate: "2026-11-10", Price: ptr("21")}); err != nil {
		t.Fatal(err)
	}
	_, err = h.svc.SetType(ctx, adminID, d.ID, owner, "receipt")
	wantInvalid(t, err, "base")

	r, err := h.svc.Create(ctx, adminID, owner, Receipt, Changes{Date: ptr("2026-11-01"), Details: []sales.DetailInput{line("10", vat21, costs)}})
	if err != nil || r.State != Open {
		t.Fatalf("receipt = %+v, %v", r, err)
	}
	_, err = h.svc.SetType(ctx, adminID, r.ID, owner, "purchase_invoice")
	wantInvalid(t, err, "contact_id")
	if h.store.docs[r.ID].Kind != Receipt {
		t.Error("kind changed")
	}
}

func TestSetTypeToItsTypeChangesNothing(t *testing.T) {
	h := newHarness(t)
	g, err := h.svc.CreateGeneral(ctx, adminID, owner, GeneralChanges{Reference: ptr("Lease")})
	if err != nil {
		t.Fatal(err)
	}
	before := h.store.general[g.ID]
	got, err := h.svc.SetType(ctx, adminID, g.ID, owner, "general_document")
	if err != nil || got.Kind != GeneralDocument {
		t.Fatalf("SetType() = %+v, %v", got, err)
	}
	if !cmp.Equal(h.store.general[g.ID], before) {
		t.Error("document changed")
	}
}

func TestSetTypeRefusesLockedPeriodAndUnknownTypes(t *testing.T) {
	h := newHarness(t)
	tl, err := h.svc.CreateTypeless(ctx, adminID, owner, TypelessChanges{Date: ptr("2026-01-15")})
	if err != nil {
		t.Fatal(err)
	}
	_, err = h.svc.SetType(ctx, adminID, tl.ID, owner, "typeless_document")
	wantInvalid(t, err, "document_type")
	h.ledger.locked = "2026-03-31"
	_, err = h.svc.SetType(ctx, adminID, tl.ID, owner, "general_document")
	wantInvalid(t, err, "date")
	if _, err := h.svc.SetType(ctx, adminID, resource.ID(999999), owner, "receipt"); !IsNotFound(err) {
		t.Errorf("missing document error = %v", err)
	}
	if _, err := h.svc.SetType(ctx, adminID, tl.ID, stranger, "receipt"); err == nil {
		t.Error("stranger changed the type")
	}
}

// confirmations records what Scan and Recognise is told a user confirmed.
type confirmations struct{ fields map[resource.ID][]string }

func (c *confirmations) Queue(context.Context, resource.ID, resource.ID, resource.ID) error {
	return nil
}
func (c *confirmations) Edited(context.Context, resource.ID, resource.ID, Document, Document) {}
func (c *confirmations) Forget(context.Context, resource.ID, resource.ID) error               { return nil }
func (c *confirmations) Confirmed(_ context.Context, _, id resource.ID, fields ...string) {
	c.fields[id] = append(c.fields[id], fields...)
}

func TestSavedFieldsAndTypeAreConfirmedEvenUnchanged(t *testing.T) {
	h := newHarness(t)
	rec := &confirmations{fields: map[resource.ID][]string{}}
	h.svc.UseRecognition(rec)
	d, err := h.svc.Create(ctx, adminID, owner, PurchaseInvoice, Changes{Reference: ptr("INV-9")})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := h.svc.Update(ctx, adminID, owner, PurchaseInvoice, d.ID, Changes{Reference: ptr("INV-9"), DueDate: ptr("")}); err != nil {
		t.Fatal(err)
	}
	if got := rec.fields[d.ID]; !cmp.Equal(got, []string{"reference", "due_date"}) {
		t.Errorf("confirmed = %v, want reference and due_date", got)
	}
	// Lines confirm nothing of the head.
	if _, err := h.svc.Update(ctx, adminID, owner, PurchaseInvoice, d.ID, Changes{Details: []sales.DetailInput{line("10", vat21, costs)}}); err != nil {
		t.Fatal(err)
	}
	if got := rec.fields[d.ID]; len(got) != 2 {
		t.Errorf("confirmed = %v after saving lines", got)
	}
	if _, err := h.svc.SetType(ctx, adminID, d.ID, owner, "purchase_invoice"); err != nil {
		t.Fatal(err)
	}
	if _, err := h.svc.SetType(ctx, adminID, d.ID, owner, "receipt"); err != nil {
		t.Fatal(err)
	}
	if got := rec.fields[d.ID]; !cmp.Equal(got, []string{"reference", "due_date", "document_type", "document_type"}) {
		t.Errorf("confirmed = %v, want the type twice", got)
	}
}
