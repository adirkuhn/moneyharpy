package purchases

import (
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

func TestSetTypeRefusesBookedDocuments(t *testing.T) {
	h := newHarness(t)
	d, err := h.svc.Create(ctx, adminID, owner, PurchaseInvoice, invoice(line("100", vat21, costs)))
	if err != nil {
		t.Fatal(err)
	}
	_, err = h.svc.SetType(ctx, adminID, d.ID, owner, "receipt")
	wantInvalid(t, err, "base")
	if h.store.docs[d.ID].Kind != PurchaseInvoice {
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
