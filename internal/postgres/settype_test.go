package postgres_test

import (
	"context"
	"testing"
	"time"

	"github.com/adirkuhn/moneyharpy/internal/history"
	"github.com/adirkuhn/moneyharpy/internal/journal"
	"github.com/adirkuhn/moneyharpy/internal/notes"
	"github.com/adirkuhn/moneyharpy/internal/postgres"
	"github.com/adirkuhn/moneyharpy/internal/purchases"
	"github.com/adirkuhn/moneyharpy/internal/resource"
	"github.com/adirkuhn/moneyharpy/internal/sales"
)

func TestSetTypeMovesADocumentWithItsFilesNotesAndEvents(t *testing.T) {
	ctx := context.Background()
	f := newPurchaseFixture(t)
	upload, err := f.docs.Upload(ctx, f.admin, f.owner, "bill.pdf", "application/pdf", []byte("%PDF-1.4"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := testPool.Exec(ctx, "UPDATE typeless_documents SET contact_id = $2, reference = 'Bill 7', date = '2026-10-02' WHERE id = $1",
		int64(upload.ID), int64(f.supplier)); err != nil {
		t.Fatal(err)
	}
	noteSvc := notes.NewService(postgres.NewNoteStore(testPool), ids, time.Now)
	if _, err := noteSvc.Add(ctx, f.admin, purchases.TypelessDocument, upload.ID, f.owner, notes.Input{Note: "Check the amount"}); err != nil {
		t.Fatal(err)
	}
	events := postgres.NewEventStore(testPool)
	if err := events.InsertEvents(ctx, []history.Event{{ID: ids.Next(), AdministrationID: f.admin, EntityType: purchases.TypelessDocument, EntityID: upload.ID,
		UserID: f.owner, Action: "document_saved", Data: map[string]any{}, CreatedAt: time.Now(), UpdatedAt: time.Now()}}); err != nil {
		t.Fatal(err)
	}

	got, err := f.docs.SetType(ctx, f.admin, upload.ID, f.owner, "purchase_invoice")
	if err != nil {
		t.Fatalf("SetType() error = %v", err)
	}
	if got.ID != upload.ID || got.Kind != purchases.PurchaseInvoice || got.State != purchases.New || got.AttachmentsCount != 1 || got.ContactID != f.supplier {
		t.Errorf("incoming = %+v", got)
	}
	inv, err := f.docs.Document(ctx, f.admin, purchases.PurchaseInvoice, upload.ID, f.owner)
	if err != nil {
		t.Fatal(err)
	}
	if inv.Reference != "Bill 7" || inv.Date.Format(time.DateOnly) != "2026-10-02" || inv.EntryNumber == 0 || inv.Currency != "EUR" ||
		len(inv.Attachments) != 1 || len(inv.Notes) != 1 || inv.Origin != purchases.OriginUpload {
		t.Errorf("purchase invoice = %+v", inv)
	}
	if _, err := f.docs.TypelessDocument(ctx, f.admin, upload.ID, f.owner); !purchases.IsNotFound(err) {
		t.Errorf("typeless document error = %v", err)
	}
	moved, err := events.EventsOf(ctx, f.admin, purchases.PurchaseInvoice, []resource.ID{upload.ID})
	if err != nil || len(moved) != 1 {
		t.Errorf("events = %v, %v", moved, err)
	}
	var typ string
	if err := testPool.QueryRow(ctx, "SELECT type FROM resources WHERE id = $1", int64(upload.ID)).Scan(&typ); err != nil || typ != "documents/purchase_invoices" {
		t.Errorf("resource type = %q, %v", typ, err)
	}

	// Lines stay between purchase invoice and receipt.
	if _, err := f.docs.Update(ctx, f.admin, f.owner, purchases.PurchaseInvoice, upload.ID, purchases.Changes{ContactID: ptr(resource.ID(0)),
		Details: []sales.DetailInput{{Description: ptr("Paper"), Price: ptr("10"), TaxRateID: &f.vat21, LedgerAccountID: &f.costs}}}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.docs.SetType(ctx, f.admin, upload.ID, f.owner, "receipt"); err != nil {
		t.Fatalf("SetType(receipt) error = %v", err)
	}
	rec, err := f.docs.Document(ctx, f.admin, purchases.Receipt, upload.ID, f.owner)
	if err != nil {
		t.Fatal(err)
	}
	if len(rec.Lines) != 1 || rec.EntryNumber != inv.EntryNumber || len(rec.Attachments) != 1 || len(rec.Notes) != 1 {
		t.Errorf("receipt = %+v", rec)
	}

	if _, err := f.docs.SetType(ctx, f.admin, upload.ID, f.owner, "general_document"); err != nil {
		t.Fatalf("SetType(general_document) error = %v", err)
	}
	g, err := f.docs.GeneralDocument(ctx, f.admin, upload.ID, f.owner)
	if err != nil {
		t.Fatal(err)
	}
	if g.EntryNumber != inv.EntryNumber || g.Reference != "Bill 7" || len(g.Attachments) != 1 || len(g.Notes) != 1 {
		t.Errorf("general document = %+v", g)
	}
	var lines int
	if err := testPool.QueryRow(ctx, "SELECT count(*) FROM purchase_document_details WHERE purchase_document_id = $1", int64(upload.ID)).Scan(&lines); err != nil || lines != 0 {
		t.Errorf("lines left = %d, %v", lines, err)
	}

	if _, err := f.docs.SetType(ctx, f.admin, upload.ID, f.owner, "general_journal_document"); err != nil {
		t.Fatalf("SetType(general_journal_document) error = %v", err)
	}
	entry, err := f.journal.Document(ctx, f.admin, upload.ID, f.owner)
	if err != nil {
		t.Fatal(err)
	}
	if entry.Reference != "Bill 7" || len(entry.Rows) != 0 || len(entry.Attachments) != 1 || len(entry.Notes) != 1 {
		t.Errorf("journal entry = %+v", entry)
	}
	// The empty journal entry is filled in as any other.
	if _, err := f.journal.UpdateDocument(ctx, f.admin, upload.ID, f.owner, journal.DocumentChanges{Rows: []journal.RowInput{
		{LedgerAccountID: &f.costs, Debit: ptr("5")}, {LedgerAccountID: &f.revenue, Credit: ptr("5")},
	}}); err != nil {
		t.Fatalf("fill in journal entry: %v", err)
	}
	if _, err := f.docs.SetType(ctx, f.admin, upload.ID, f.owner, "receipt"); err == nil {
		t.Error("a journal entry changed type")
	}
}

func TestSetTypeBooksABookedPurchaseInvoiceAgainAsReceipt(t *testing.T) {
	ctx := context.Background()
	f := newPurchaseFixture(t)
	d, err := f.docs.Create(ctx, f.admin, f.owner, purchases.PurchaseInvoice, purchases.Changes{
		ContactID: &f.supplier, Reference: ptr("EX-1"), Date: ptr(time.Now().Format(time.DateOnly)),
		Details: []sales.DetailInput{{Description: ptr("Paper"), Price: ptr("100"), TaxRateID: &f.vat21, LedgerAccountID: &f.costs}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if d.State != purchases.Open {
		t.Fatalf("state = %s, want open", d.State)
	}
	booked := func(kind string) (n int) {
		t.Helper()
		if err := testPool.QueryRow(ctx, "SELECT count(*) FROM journal_entries WHERE document_id = $1 AND document_type = $2", int64(d.ID), kind).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	before := booked(purchases.PurchaseInvoice)
	if before == 0 {
		t.Fatal("the purchase invoice books nothing")
	}
	if _, err := f.docs.SetType(ctx, f.admin, d.ID, f.owner, "receipt"); err != nil {
		t.Fatalf("SetType(receipt) error = %v", err)
	}
	rec, err := f.docs.Document(ctx, f.admin, purchases.Receipt, d.ID, f.owner)
	if err != nil {
		t.Fatal(err)
	}
	if rec.State != purchases.Open || len(rec.Lines) != 1 || rec.EntryNumber != d.EntryNumber {
		t.Errorf("receipt = %+v", rec)
	}
	if got := booked(purchases.PurchaseInvoice); got != 0 {
		t.Errorf("%d entries still on the purchase invoice", got)
	}
	if got := booked(purchases.Receipt); got != before {
		t.Errorf("receipt books %d entries, want %d", got, before)
	}
	if _, err := f.docs.SetType(ctx, f.admin, d.ID, f.owner, "general_document"); err == nil {
		t.Error("a booked receipt became a general document")
	}
}
