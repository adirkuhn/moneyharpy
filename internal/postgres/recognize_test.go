package postgres_test

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/adirkuhn/moneyharpy/internal/administration"
	"github.com/adirkuhn/moneyharpy/internal/attachments"
	"github.com/adirkuhn/moneyharpy/internal/contacts"
	"github.com/adirkuhn/moneyharpy/internal/money"
	"github.com/adirkuhn/moneyharpy/internal/notes"
	"github.com/adirkuhn/moneyharpy/internal/ocr"
	"github.com/adirkuhn/moneyharpy/internal/postgres"
	"github.com/adirkuhn/moneyharpy/internal/purchases"
	"github.com/adirkuhn/moneyharpy/internal/recognize"
	"github.com/adirkuhn/moneyharpy/internal/resource"
	"github.com/adirkuhn/moneyharpy/internal/sales"
)

// memFiles keeps files in memory.
type memFiles struct {
	mu   sync.Mutex
	data map[string][]byte
}

func (m *memFiles) Put(_ context.Context, key, _ string, data []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.data[key] = data
	return nil
}

func (m *memFiles) Open(_ context.Context, key string) (io.ReadCloser, string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	data, ok := m.data[key]
	if !ok {
		return nil, "", os.ErrNotExist
	}
	return io.NopCloser(bytes.NewReader(data)), "", nil
}

func (m *memFiles) Delete(_ context.Context, key string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.data, key)
	return nil
}

func TestRecognitionStoreRoundTripAndLease(t *testing.T) {
	ctx := context.Background()
	f := newPurchaseFixture(t)
	store := postgres.NewRecognitionStore(testPool)
	now := time.Now().UTC().Truncate(time.Second)
	due, later := now.Add(-time.Minute), now.Add(time.Hour)
	a := recognize.Recognition{
		DocumentID: ids.Next(), AdministrationID: f.admin, UserID: f.owner, State: recognize.StatePending, NextAttemptAt: &due,
		Fields: []recognize.Field{{Name: "reference", Confidence: recognize.Certain}}, CreatedAt: now, UpdatedAt: now,
	}
	b := a
	b.DocumentID, b.NextAttemptAt = ids.Next(), &later
	c := a
	c.DocumentID, c.State, c.Source, c.Error, c.Attempts, c.NextAttemptAt = ids.Next(), recognize.StateFailed, recognize.SourceOCR, "OCR service unavailable", 3, nil
	for _, r := range []recognize.Recognition{a, b, c} {
		if err := store.SaveRecognition(ctx, r); err != nil {
			t.Fatal(err)
		}
	}
	got, err := store.Recognition(ctx, f.admin, c.DocumentID)
	if err != nil || got.State != recognize.StateFailed || got.Source != recognize.SourceOCR || got.Error != c.Error || got.Attempts != 3 ||
		got.NextAttemptAt != nil || len(got.Fields) != 1 || got.Fields[0].Confidence != recognize.Certain {
		t.Errorf("Recognition() = %+v, %v", got, err)
	}
	if _, err := store.Recognition(ctx, f.admin+1, c.DocumentID); err != recognize.ErrNotFound {
		t.Errorf("Recognition() of another administration error = %v", err)
	}
	all, err := store.Recognitions(ctx, f.admin, []resource.ID{a.DocumentID, c.DocumentID, ids.Next()})
	if err != nil || len(all) != 2 {
		t.Errorf("Recognitions() = %v, %v", all, err)
	}

	leased, err := store.LeaseDue(ctx, now, now.Add(10*time.Minute), 100)
	if err != nil {
		t.Fatal(err)
	}
	var mine []recognize.Recognition
	for _, r := range leased {
		if r.AdministrationID == f.admin {
			mine = append(mine, r)
		}
	}
	if len(mine) != 1 || mine[0].DocumentID != a.DocumentID || !mine[0].NextAttemptAt.Equal(now.Add(10*time.Minute)) {
		t.Fatalf("LeaseDue() = %+v", mine)
	}
	again, err := store.LeaseDue(ctx, now, now.Add(10*time.Minute), 100)
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range again {
		if r.DocumentID == a.DocumentID {
			t.Error("a leased recognition was leased again")
		}
	}
	if err := store.DeleteRecognition(ctx, f.admin, a.DocumentID); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Recognition(ctx, f.admin, a.DocumentID); err != recognize.ErrNotFound {
		t.Errorf("deleted recognition error = %v", err)
	}
}

func TestSupplierMemoryRoundTrip(t *testing.T) {
	ctx := context.Background()
	f := newPurchaseFixture(t)
	store := postgres.NewRecognitionStore(testPool)
	empty, err := store.Memory(ctx, f.admin, f.supplier)
	if err != nil || !empty.Empty() {
		t.Fatalf("Memory() = %+v, %v", empty, err)
	}
	at := time.Now().UTC().Truncate(time.Second)
	m := recognize.Memory{
		AdministrationID: f.admin, ContactID: f.supplier, Kind: recognize.Learned[recognize.Kind]{V: recognize.KindReceipt, Uses: 2},
		Receipts: recognize.KindMemory{
			Currency: recognize.Learned[string]{V: "EUR", Uses: 1}, PricesInclTax: recognize.Learned[bool]{V: true, Uses: 3}, LastDocumentID: 41,
			Lines: map[string]recognize.Category{
				"":        {LedgerAccountID: f.costs, TaxRateID: f.vat21, Uses: 2, LastUsed: at, SourceDocumentID: 41},
				"hosting": {LedgerAccountID: f.costs, Uses: 1, LastUsed: at},
			},
			UpdatedAt: at,
		},
		Invoices: recognize.KindMemory{
			Currency: recognize.Learned[string]{V: "USD", Uses: 1}, LastDocumentID: 42,
			Lines:     map[string]recognize.Category{"hosting": {LedgerAccountID: f.costs, TaxRateID: f.vat21, Uses: 1, LastUsed: at, SourceDocumentID: 42}},
			UpdatedAt: at,
		},
		UpdatedAt: at,
	}
	if err := store.SaveMemory(ctx, m); err != nil {
		t.Fatal(err)
	}
	delete(m.Receipts.Lines, "hosting")
	if err := store.SaveMemory(ctx, m); err != nil {
		t.Fatal(err)
	}
	got, err := store.Memory(ctx, f.admin, f.supplier)
	if err != nil {
		t.Fatal(err)
	}
	r, i := got.Receipts, got.Invoices
	if got.Kind != m.Kind || r.Currency != m.Receipts.Currency || r.PricesInclTax != m.Receipts.PricesInclTax || r.LastDocumentID != 41 ||
		len(r.Lines) != 1 || r.Lines[""].LedgerAccountID != f.costs || r.Lines[""].TaxRateID != f.vat21 || !r.Lines[""].LastUsed.Equal(at) ||
		r.Lines[""].SourceDocumentID != 41 || !r.UpdatedAt.Equal(at) {
		t.Errorf("receipts = %+v", r)
	}
	if i.Currency.V != "USD" || i.LastDocumentID != 42 || len(i.Lines) != 1 || i.Lines["hosting"].SourceDocumentID != 42 || i.PricesInclTax.Known() {
		t.Errorf("purchase invoices = %+v", i)
	}
}

func TestSupplierHistoryHoldsTheMostRecentBookedDocuments(t *testing.T) {
	ctx := context.Background()
	f := newPurchaseFixture(t)
	store := postgres.NewRecognitionStore(testPool)
	var want []resource.ID
	for i, day := range []string{"2026-10-03", "2026-10-01", "2026-10-02"} {
		d := f.purchase(t, purchases.PurchaseInvoice, fmt.Sprintf("1%d", i), day)
		want = append(want, d.ID)
	}
	f.purchase(t, purchases.Receipt, "50", "2026-10-04")
	// A new document, without lines, is not booked.
	if _, err := f.docs.Create(ctx, f.admin, f.owner, purchases.PurchaseInvoice, purchases.Changes{ContactID: &f.supplier}); err != nil {
		t.Fatal(err)
	}

	got, err := store.History(ctx, f.admin, f.supplier, purchases.PurchaseInvoice, 2)
	if err != nil {
		t.Fatal(err)
	}
	// The two most recent, oldest first.
	if len(got) != 2 || got[0].ID != want[2] || got[1].ID != want[0] {
		t.Fatalf("History() = %+v, want documents %d and %d", got, want[2], want[0])
	}
	d := got[1]
	if d.Kind != purchases.PurchaseInvoice || d.ContactID != f.supplier || d.State == purchases.New || d.Currency != "EUR" || !d.PricesAreInclTax ||
		d.Date.Format(time.DateOnly) != "2026-10-03" || d.UpdatedAt.IsZero() || len(d.Lines) != 1 {
		t.Fatalf("document = %+v", d)
	}
	if l := d.Lines[0]; l.Description != "Paper" || l.Price != 1000 || l.TaxRateID != f.vat21 || l.LedgerAccountID == nil || *l.LedgerAccountID != f.costs {
		t.Errorf("line = %+v", l)
	}
	if receipts, err := store.History(ctx, f.admin, f.supplier, purchases.Receipt, 20); err != nil || len(receipts) != 1 {
		t.Errorf("receipts = %+v, %v", receipts, err)
	}
	if none, err := store.History(ctx, f.admin+1, f.supplier, purchases.Receipt, 20); err != nil || len(none) != 0 {
		t.Errorf("of another administration = %+v, %v", none, err)
	}
}

// scanOCR reads every file as the same text.
type scanOCR struct{ text string }

func (o scanOCR) Read(context.Context, ocr.File) (ocr.Text, error) {
	return ocr.Text{Text: o.text, Method: "text", Pages: 1}, nil
}
func (scanOCR) Render(context.Context, ocr.File, int) (ocr.Page, error) {
	return ocr.Page{}, ocr.ErrUnsupported
}
func (scanOCR) RenderAt(context.Context, ocr.File, int, int) (ocr.Page, error) {
	return ocr.Page{}, ocr.ErrUnsupported
}
func (scanOCR) Ping(context.Context) error { return nil }

func TestScanAndRecogniseFillsInUploadsAndLearns(t *testing.T) {
	ctx := context.Background()
	f := newSalesFixture(t)
	admins := administration.NewService(postgres.NewAdministrationStore(testPool), ids, time.Now)
	store := postgres.NewPurchaseStore(testPool)
	noteSvc := notes.NewService(postgres.NewNoteStore(testPool), ids, time.Now)
	files := attachments.NewService(postgres.NewAttachmentStore(testPool), &memFiles{data: map[string][]byte{}}, ids, time.Now)
	people := contacts.NewService(admins, nil, admins, contacts.AnyReferences{store}, noteSvc, postgres.NewContactStore(testPool), ids, time.Now)
	docs := purchases.NewService(purchases.Deps{
		Administrations: admins, Contacts: people, Products: f.products, Projects: f.projects, Ledger: f.books, Notes: noteSvc, Attachments: files,
		Store: store, IDs: ids, Now: time.Now,
	})
	// The parser stands in for recognize.ParseText: it reads the supplier
	// and amounts of the text "<IBAN> <total>".
	parse := func(text string, _ time.Time, _ recognize.Self) recognize.Extraction {
		var iban string
		var total money.Amount
		for i, part := range bytes.Fields([]byte(text)) {
			if i == 0 {
				iban = string(part)
			} else {
				total, _ = money.ParseAmount(string(part))
			}
		}
		return recognize.Extraction{
			Supplier:     recognize.Supplier{Name: "Example Supplies B.V.", IBAN: iban, Email: "billing@example.com"},
			Reference:    recognize.Value[string]{V: "INV-" + total.String(), Found: true, Confidence: recognize.Certain},
			Date:         recognize.Value[time.Time]{V: time.Date(2026, 11, 2, 0, 0, 0, 0, time.UTC), Found: true, Confidence: recognize.Certain},
			TotalInclTax: recognize.Value[money.Amount]{V: total, Found: true, Confidence: recognize.Certain},
			Lines: []recognize.Line{{
				Description: "Hosting November 2026", Price: recognize.Value[money.Amount]{V: total, Found: true, Confidence: recognize.Certain},
				TaxRate: recognize.Value[string]{V: "21", Found: true, Confidence: recognize.Certain},
			}},
			PricesInclTax: recognize.Value[bool]{V: true, Found: true, Confidence: recognize.Certain},
		}
	}
	svc := recognize.NewService(recognize.Deps{
		Documents: docs, Contacts: people, Ledger: f.books, Store: postgres.NewRecognitionStore(testPool),
		OCR: scanOCR{text: "NL91ABNA0417164300 121.00"}, Parsers: recognize.Parsers{Text: parse}, Now: time.Now,
	})
	docs.UseRecognition(svc)

	first, err := docs.Upload(ctx, f.admin, f.owner, "bill.pdf", "application/pdf", []byte("%PDF-1.4"))
	if err != nil {
		t.Fatal(err)
	}
	if r, _ := svc.Recognition(ctx, f.admin, first.ID); r.State != recognize.StatePending {
		t.Fatalf("after upload = %+v", r)
	}
	if err := svc.Work(ctx); err != nil {
		t.Fatalf("Work() error = %v", err)
	}
	r, err := svc.Recognition(ctx, f.admin, first.ID)
	if err != nil || r.State != recognize.StateDone || r.Source != recognize.SourceText {
		t.Fatalf("recognition = %+v, %v", r, err)
	}
	inv, err := docs.Document(ctx, f.admin, purchases.PurchaseInvoice, first.ID, f.owner)
	if err != nil {
		t.Fatalf("the upload is no purchase invoice: %v", err)
	}
	if inv.State != purchases.New || inv.ContactID == 0 || inv.Reference != "INV-121.0" || inv.Date.Format(time.DateOnly) != "2026-11-02" ||
		!inv.PricesAreInclTax || len(inv.Lines) != 1 || inv.Totals.InclTax != 12100 || len(inv.Attachments) != 1 {
		t.Fatalf("purchase invoice = %+v", inv)
	}
	supplier, err := people.Contact(ctx, f.admin, inv.ContactID, f.owner)
	if err != nil || supplier.CompanyName != "Example Supplies B.V." || supplier.SepaIBAN != "NL91ABNA0417164300" {
		t.Errorf("new supplier = %+v, %v", supplier, err)
	}
	marks := map[string]recognize.Confidence{}
	for _, field := range r.Fields {
		marks[field.Name] = field.Confidence
	}
	if marks["contact_id"] != recognize.Uncertain || marks["reference"] != recognize.Certain || marks["details.0.price"] != recognize.Certain {
		t.Errorf("fields = %v", marks)
	}

	// The user picks a category and books it; the bolt leaves the line's
	// category alone, and the supplier is remembered.
	software := categoryByKey(t, f.books, f.admin, f.owner, "software")
	line := inv.Lines[0].ID
	booked, err := docs.Update(ctx, f.admin, f.owner, purchases.PurchaseInvoice, inv.ID, purchases.Changes{
		Reference: ptr("INV-1"), Details: []sales.DetailInput{{ID: &line, LedgerAccountID: &software}},
	})
	if err != nil || booked.State != purchases.Open {
		t.Fatalf("Update() = %+v, %v", booked, err)
	}
	r, _ = svc.Recognition(ctx, f.admin, first.ID)
	for _, field := range r.Fields {
		if field.Name == "reference" || field.Name == "details.0.ledger_account_id" {
			t.Errorf("changed field %s is still marked", field.Name)
		}
	}

	// The supplier's next upload is matched by IBAN and gets the category.
	second, err := docs.Upload(ctx, f.admin, f.owner, "bill2.pdf", "application/pdf", []byte("%PDF-1.4"))
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.Work(ctx); err != nil {
		t.Fatal(err)
	}
	next, err := docs.Document(ctx, f.admin, purchases.PurchaseInvoice, second.ID, f.owner)
	if err != nil || next.ContactID != inv.ContactID || len(next.Lines) != 1 || next.Lines[0].LedgerAccountID == nil ||
		*next.Lines[0].LedgerAccountID != software || next.State != purchases.New {
		t.Fatalf("second document = %+v, %v", next, err)
	}
	r, _ = svc.Recognition(ctx, f.admin, second.ID)
	marks = map[string]recognize.Confidence{}
	for _, field := range r.Fields {
		marks[field.Name] = field.Confidence
	}
	if marks["contact_id"] != recognize.Certain || marks["details.0.ledger_account_id"] != recognize.Uncertain {
		t.Errorf("second fields = %v", marks)
	}

	// Removing the document forgets what was read.
	if err := docs.Delete(ctx, f.admin, f.owner, purchases.PurchaseInvoice, second.ID); err != nil {
		t.Fatal(err)
	}
	if r, _ := svc.Recognition(ctx, f.admin, second.ID); r.State != recognize.StateNone {
		t.Errorf("after removal = %+v", r)
	}
}
