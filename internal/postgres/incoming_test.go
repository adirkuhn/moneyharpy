package postgres_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/adirkuhn/moneyharpy/internal/journal"
	"github.com/adirkuhn/moneyharpy/internal/purchases"
	"github.com/adirkuhn/moneyharpy/internal/resource"
	"github.com/adirkuhn/moneyharpy/internal/sales"
)

func TestIncomingDocumentsOfEveryKind(t *testing.T) {
	ctx := context.Background()
	f := newPurchaseFixture(t)
	invoice := f.purchase(t, purchases.PurchaseInvoice, "121", "2026-10-01")
	receipt := f.purchase(t, purchases.Receipt, "60.50", "2026-10-02")
	if _, err := f.docs.RegisterPayment(ctx, f.admin, f.owner, purchases.Receipt, receipt.ID, sales.PaymentInput{PaymentDate: "2026-10-02"}); err != nil {
		t.Fatal(err)
	}
	upload, err := f.docs.CreateFromFile(ctx, f.admin, f.owner, purchases.PurchaseInvoice, "bill.pdf", "application/pdf", []byte("%PDF-1.4"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := testPool.Exec(ctx, "UPDATE purchase_documents SET date = '2026-10-03' WHERE id = $1", int64(upload.ID)); err != nil {
		t.Fatal(err)
	}
	general, err := f.docs.CreateGeneral(ctx, f.admin, f.owner, purchases.GeneralChanges{ContactID: &f.supplier, Reference: ptr("Contract"), Date: ptr("2026-10-04")})
	if err != nil {
		t.Fatal(err)
	}
	entry, err := f.journal.CreateDocument(ctx, f.admin, f.owner, journal.DocumentChanges{Reference: ptr("Correction"), Date: ptr("2026-10-05"), Rows: []journal.RowInput{
		{LedgerAccountID: &f.costs, Debit: ptr("5"), ContactID: &f.supplier}, {LedgerAccountID: &f.revenue, Credit: ptr("5")},
	}})
	if err != nil {
		t.Fatal(err)
	}
	typeless, err := f.docs.CreateTypeless(ctx, f.admin, f.owner, purchases.TypelessChanges{Reference: ptr("Scan"), Date: ptr("2026-10-06")})
	if err != nil {
		t.Fatal(err)
	}
	old := f.purchase(t, purchases.Receipt, "10", "2025-05-01")

	list := func(filter string) ([]purchases.Incoming, int) {
		t.Helper()
		q, err := purchases.ParseIncomingFilter(filter, time.Now())
		if err != nil {
			t.Fatal(err)
		}
		got, total, err := f.store.IncomingDocuments(ctx, f.admin, q)
		if err != nil {
			t.Fatal(err)
		}
		return got, total
	}
	ids := func(got []purchases.Incoming) []resource.ID {
		out := make([]resource.ID, len(got))
		for i, d := range got {
			out[i] = d.ID
		}
		return out
	}

	got, total := list("period:202610..202610")
	want := []resource.ID{typeless.ID, entry.ID, general.ID, upload.ID, receipt.ID, invoice.ID}
	if total != 6 || !equalIDs(ids(got), want) {
		t.Fatalf("October = %v (%d), want %v", ids(got), total, want)
	}
	byID := map[resource.ID]purchases.Incoming{}
	for _, d := range got {
		byID[d.ID] = d
	}
	inv := byID[invoice.ID]
	if inv.Kind != purchases.PurchaseInvoice || inv.State != invoice.State || inv.ContactName != "Supplier B.V." || inv.ContactID != f.supplier ||
		inv.TotalInclTax != 12100 || inv.Currency != "EUR" || inv.Reference != "INV-121" || inv.Date.Format(time.DateOnly) != "2026-10-01" || inv.CreatedAt.IsZero() {
		t.Errorf("invoice = %+v", inv)
	}
	if d := byID[receipt.ID]; d.Kind != purchases.Receipt || d.State != purchases.Paid || d.TotalInclTax != 6050 {
		t.Errorf("receipt = %+v", d)
	}
	if d := byID[upload.ID]; d.State != purchases.New || d.AttachmentsCount != 1 || d.ContactID != 0 || d.ContactName != "" {
		t.Errorf("upload = %+v", d)
	}
	if d := byID[general.ID]; d.Kind != purchases.GeneralDocument || d.State != purchases.Saved || d.ContactName != "Supplier B.V." || d.TotalInclTax != 0 || d.Currency != "" {
		t.Errorf("general = %+v", d)
	}
	if d := byID[entry.ID]; d.Kind != journal.GeneralJournalDocument || d.State != purchases.Saved || d.ContactID != 0 || d.Reference != "Correction" {
		t.Errorf("journal entry = %+v", d)
	}
	if d := byID[typeless.ID]; d.Kind != purchases.TypelessDocument || d.State != purchases.New || d.Reference != "Scan" {
		t.Errorf("typeless = %+v", d)
	}

	cases := map[string][]resource.ID{
		"period:202610..202610,type:receipt|general_journal_document": {entry.ID, receipt.ID},
		"period:202610..202610,type:general_document":                 {general.ID},
		"period:202610..202610,state:saved":                           {entry.ID, general.ID},
		"period:202610..202610,state:new":                             {typeless.ID, upload.ID},
		"period:202610..202610,state:paid|new,type:purchase_invoice":  {upload.ID},
		"period:202610..202610,attachment:with":                       {upload.ID},
		"period:202610..202610,contact_id:" + f.supplier.String():     {entry.ID, general.ID, receipt.ID, invoice.ID},
		"period:202610..202610,recurring:enabled":                     {},
		"type:receipt": {receipt.ID, old.ID},
	}
	for filter, want := range cases {
		if got, total := list(filter); total != len(want) || !equalIDs(ids(got), want) {
			t.Errorf("%s = %v (%d), want %v", filter, ids(got), total, want)
		}
	}
	q, _ := purchases.ParseIncomingFilter("period:202610..202610", time.Now())
	q.Ascending, q.Page, q.PerPage = true, 2, 2
	if got, total, err := f.store.IncomingDocuments(ctx, f.admin, q); err != nil || total != 6 || !equalIDs(ids(got), []resource.ID{upload.ID, general.ID}) {
		t.Errorf("page 2 ascending = %v (%d), %v", ids(got), total, err)
	}

	if d, err := f.store.IncomingDocument(ctx, f.admin, entry.ID, time.Now()); err != nil || d.Kind != journal.GeneralJournalDocument || d.Ref().URI() != entry.Ref().URI() {
		t.Errorf("IncomingDocument(entry) = %+v, %v", d, err)
	}
	if d, err := f.store.IncomingDocument(ctx, f.admin, old.ID, time.Now()); err != nil || d.Kind != purchases.Receipt {
		t.Errorf("IncomingDocument(old) = %+v, %v", d, err)
	}
	if _, err := f.store.IncomingDocument(ctx, f.admin, f.supplier, time.Now()); !errors.Is(err, purchases.ErrNotFound) {
		t.Errorf("IncomingDocument(contact) error = %v", err)
	}
	if _, err := f.store.IncomingDocument(ctx, f.admin+1, invoice.ID, time.Now()); !errors.Is(err, purchases.ErrNotFound) {
		t.Errorf("IncomingDocument(other administration) error = %v", err)
	}
}

func TestIncomingDocumentStatistics(t *testing.T) {
	ctx := context.Background()
	f := newPurchaseFixture(t)
	f.purchase(t, purchases.PurchaseInvoice, "121", "2026-10-01")
	receipt := f.purchase(t, purchases.Receipt, "60.50", "2026-10-02")
	if _, err := f.docs.RegisterPayment(ctx, f.admin, f.owner, purchases.Receipt, receipt.ID, sales.PaymentInput{PaymentDate: "2026-10-02"}); err != nil {
		t.Fatal(err)
	}
	partly := f.purchase(t, purchases.PurchaseInvoice, "24.20", "2026-10-03")
	if _, err := f.docs.RegisterPayment(ctx, f.admin, f.owner, purchases.PurchaseInvoice, partly.ID, sales.PaymentInput{PaymentDate: "2026-10-03", Price: ptr("4.20")}); err != nil {
		t.Fatal(err)
	}
	if _, err := testPool.Exec(ctx, "UPDATE purchase_document_details SET ledger_account_id = NULL WHERE purchase_document_id = $1", int64(partly.ID)); err != nil {
		t.Fatal(err)
	}
	if _, err := f.docs.CreateFromFile(ctx, f.admin, f.owner, purchases.Receipt, "bill.pdf", "application/pdf", []byte("%PDF-1.4")); err != nil {
		t.Fatal(err)
	}
	stats := func(filter string) purchases.IncomingStatistics {
		t.Helper()
		q, err := purchases.ParseIncomingFilter(filter, time.Now())
		if err != nil {
			t.Fatal(err)
		}
		st, err := f.store.IncomingStatistics(ctx, f.admin, q)
		if err != nil {
			t.Fatal(err)
		}
		return st
	}
	st := stats("period:202610..202610")
	// 100 + 50 + 20 excluding VAT; 121 + 24.20 - 4.20 unpaid.
	if st.CostsExclTax != 17000 || st.AmountPayable != 14100 || len(st.Categories) != 2 {
		t.Fatalf("statistics = %+v", st)
	}
	if c := st.Categories[0]; c.LedgerAccountID != f.costs || c.Name == "" || c.Amount != 15000 {
		t.Errorf("first category = %+v", c)
	}
	if c := st.Categories[1]; c.LedgerAccountID != 0 || c.Name != "" || c.Amount != 2000 {
		t.Errorf("uncategorized = %+v", c)
	}
	if st := stats("period:202610..202610,state:paid"); st.CostsExclTax != 5000 || st.AmountPayable != 0 || len(st.Categories) != 1 {
		t.Errorf("paid = %+v", st)
	}
	for _, filter := range []string{"period:202610..202610,type:general_document", "period:202610..202610,state:saved", "period:202610..202610,recurring:enabled", "period:202001..202001"} {
		if st := stats(filter); st.CostsExclTax != 0 || st.AmountPayable != 0 || len(st.Categories) != 0 {
			t.Errorf("%s = %+v", filter, st)
		}
	}
}
