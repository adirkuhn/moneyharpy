package postgres_test

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/adirkuhn/moneyharpy/internal/administration"
	"github.com/adirkuhn/moneyharpy/internal/attachments"
	"github.com/adirkuhn/moneyharpy/internal/banking"
	"github.com/adirkuhn/moneyharpy/internal/bookkeeping"
	"github.com/adirkuhn/moneyharpy/internal/contacts"
	"github.com/adirkuhn/moneyharpy/internal/notes"
	"github.com/adirkuhn/moneyharpy/internal/postgres"
	"github.com/adirkuhn/moneyharpy/internal/purchases"
	"github.com/adirkuhn/moneyharpy/internal/resource"
	"github.com/adirkuhn/moneyharpy/internal/sales"
)

type purchaseFixture struct {
	salesFixture
	docs     *purchases.Service
	store    *postgres.PurchaseStore
	supplier resource.ID
	vat21    resource.ID
	costs    resource.ID
}

func newPurchaseFixture(t *testing.T) purchaseFixture {
	t.Helper()
	ctx := context.Background()
	f := newSalesFixture(t)
	admins := administration.NewService(postgres.NewAdministrationStore(testPool), ids, time.Now)
	store := postgres.NewPurchaseStore(testPool)
	noteSvc := notes.NewService(postgres.NewNoteStore(testPool), ids, time.Now)
	files := attachments.NewService(postgres.NewAttachmentStore(testPool), diskless{}, ids, time.Now)
	people := contacts.NewService(admins, nil, admins, contacts.AnyReferences{store}, noteSvc, postgres.NewContactStore(testPool), ids, time.Now)
	docs := purchases.NewService(purchases.Deps{
		Administrations: admins, Contacts: people, Products: f.products, Projects: f.projects, Ledger: f.books, Notes: noteSvc, Attachments: files,
		Store: store, Bank: purchases.Banking{Service: f.bank}, IDs: ids, Now: time.Now,
	})
	f.bank.UseDocuments(docs)
	supplier, err := people.Create(ctx, f.admin, f.owner, contacts.Changes{CompanyName: ptr("Supplier B.V."), SepaIBAN: ptr("NL44RABO0123456789")})
	if err != nil {
		t.Fatal(err)
	}
	rates, err := f.books.TaxRates(ctx, f.admin, f.owner, bookkeeping.TaxRateFilter{Type: ptr("purchase_invoice"), Percentage: ptr("21")})
	if err != nil || len(rates) == 0 {
		t.Fatalf("rates = %v, %v", rates, err)
	}
	costs := categoryByKey(t, f.books, f.admin, f.owner, "uncategorized_expenses")
	return purchaseFixture{salesFixture: f, docs: docs, store: store, supplier: supplier.ID, vat21: rates[0].ID, costs: costs}
}

func (f purchaseFixture) purchase(t *testing.T, kind, total, day string) purchases.Document {
	t.Helper()
	d, err := f.docs.Create(context.Background(), f.admin, f.owner, kind, purchases.Changes{
		ContactID: &f.supplier, Reference: ptr("INV-" + total), Date: ptr(day), PricesAreInclTax: ptr(true),
		Details: []sales.DetailInput{{Description: ptr("Paper"), Price: ptr(total), TaxRateID: &f.vat21, LedgerAccountID: &f.costs}},
	})
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func TestPurchaseDocumentsAreStoredAndPosted(t *testing.T) {
	ctx := context.Background()
	f := newPurchaseFixture(t)
	d := f.purchase(t, purchases.PurchaseInvoice, "121", "2026-10-01")
	got, err := f.store.Document(ctx, f.admin, purchases.PurchaseInvoice, d.ID)
	if err != nil || got.State != purchases.Open || len(got.Lines) != 1 || got.Totals.InclTax != 12100 || got.Totals.LineBases[0] != 10000 || len(got.Totals.ByRate) != 1 {
		t.Fatalf("stored = %+v, %v", got, err)
	}
	if _, err := f.store.Document(ctx, f.admin, purchases.Receipt, d.ID); !errors.Is(err, purchases.ErrNotFound) {
		t.Errorf("as receipt error = %v", err)
	}
	creditors := categoryByKey(t, f.books, f.admin, f.owner, "accounts_payable")
	receivable := categoryByKey(t, f.books, f.admin, f.owner, "receivable_vat")
	sums := f.ledger(t)
	if sums[creditors] != -12100 || sums[f.costs] != 10000 || sums[receivable] != 2100 {
		t.Errorf("ledger = creditors %s, costs %s, vat %s", sums[creditors], sums[f.costs], sums[receivable])
	}
	if _, err := f.docs.RegisterPayment(ctx, f.admin, f.owner, purchases.PurchaseInvoice, d.ID, sales.PaymentInput{PaymentDate: "2026-10-05"}); err != nil {
		t.Fatal(err)
	}
	if got, _ := f.store.Document(ctx, f.admin, purchases.PurchaseInvoice, d.ID); got.State != purchases.Paid || got.PaidAt == nil || len(got.Payments) != 1 {
		t.Errorf("paid = %+v", got)
	}
	if sums := f.ledger(t); sums[creditors] != 0 {
		t.Errorf("creditors after payment = %s", sums[creditors])
	}
	sum, err := f.store.Totals(ctx, f.admin)
	if err != nil || sum.ToPay.Count != 0 {
		t.Errorf("totals = %+v, %v", sum, err)
	}
	q, _ := purchases.ParseFilter("state:paid,reference:INV-121,period:202610..202610,attachment:without,ledger_account_id:"+f.costs.String(), time.Now())
	list, total, err := f.store.Documents(ctx, f.admin, q)
	if err != nil || total != 1 || len(list) != 1 {
		t.Errorf("filtered = %d, %v", total, err)
	}
	if used, err := f.store.ContactInUse(ctx, f.admin, f.supplier); err != nil || !used {
		t.Errorf("supplier in use = %v, %v", used, err)
	}
	if err := f.docs.Delete(ctx, f.admin, f.owner, purchases.PurchaseInvoice, d.ID); err != nil {
		t.Fatal(err)
	}
	if sums := f.ledger(t); sums[creditors] != 0 || sums[f.costs] != 0 {
		t.Errorf("ledger after delete = %v", sums)
	}
}

func TestBankTransactionPaysAPurchaseInvoice(t *testing.T) {
	ctx := context.Background()
	f := newPurchaseFixture(t)
	acc, err := f.bank.CreateAccount(ctx, f.admin, f.owner, banking.AccountChanges{Name: ptr("Main bank"), Identifier: ptr("NL91ABNA0417164300")})
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile("../bankfile/testdata/camt053.xml")
	if err != nil {
		t.Fatal(err)
	}
	result, err := f.bank.Import(ctx, f.admin, acc.ID, f.owner, data)
	if err != nil {
		t.Fatal(err)
	}
	var outgoing banking.Mutation
	for _, m := range result.Statements[0].Mutations {
		if m.Amount < 0 {
			outgoing = m
		}
	}
	d := f.purchase(t, purchases.PurchaseInvoice, "25.50", "2026-10-20")
	m, err := f.bank.LinkBooking(ctx, f.admin, outgoing.ID, f.owner, banking.Link{Type: banking.DocumentType, BookingID: &d.ID})
	if err != nil {
		t.Fatal(err)
	}
	if m.State != banking.Processed || len(m.Payments) != 1 || m.Payments[0].Price != 2550 || m.Open() != 0 {
		t.Fatalf("mutation = %+v", m)
	}
	creditors := categoryByKey(t, f.books, f.admin, f.owner, "accounts_payable")
	unprocessed := categoryByKey(t, f.books, f.admin, f.owner, "unprocessed_transactions")
	// The bank settles the creditors; only the incoming transaction waits.
	if sums := f.ledger(t); sums[creditors] != 0 || sums[unprocessed] != -100000 {
		t.Errorf("creditors %s, unprocessed %s", sums[creditors], sums[unprocessed])
	}
	got, _ := f.docs.Document(ctx, f.admin, purchases.PurchaseInvoice, d.ID, f.owner)
	if got.State != purchases.Paid {
		t.Errorf("state = %s", got.State)
	}
	if _, err := f.bank.UnlinkBooking(ctx, f.admin, outgoing.ID, f.owner, "Payment", got.Payments[0].ID); err != nil {
		t.Fatal(err)
	}
	got, _ = f.docs.Document(ctx, f.admin, purchases.PurchaseInvoice, d.ID, f.owner)
	if sums := f.ledger(t); got.State != purchases.Open || sums[creditors] != -2550 || sums[unprocessed] != -100000+2550 {
		t.Errorf("after unlink: %s, creditors %s, unprocessed %s", got.State, sums[creditors], sums[unprocessed])
	}
}

func TestPurchaseTransactionsGeneralAndTypelessDocuments(t *testing.T) {
	ctx := context.Background()
	f := newPurchaseFixture(t)
	acc, err := f.bank.CreateAccount(ctx, f.admin, f.owner, banking.AccountChanges{Name: ptr("Main bank"), Identifier: ptr("NL91ABNA0417164300")})
	if err != nil {
		t.Fatal(err)
	}
	d := f.purchase(t, purchases.Receipt, "50", "2026-10-02")
	tx, err := f.docs.Pay(ctx, f.admin, f.owner, purchases.PayInput{DocumentID: d.ID, FinancialAccountID: acc.ID})
	if err != nil {
		t.Fatal(err)
	}
	b, err := f.docs.CreateBatch(ctx, f.admin, f.owner, purchases.BatchInput{FinancialAccountID: acc.ID})
	if err != nil {
		t.Fatal(err)
	}
	file, b, err := f.docs.DownloadBatch(ctx, f.admin, b.ID, f.owner)
	if err != nil || b.State != purchases.Exported || !strings.Contains(string(file), "NL44RABO0123456789") {
		t.Fatalf("download = %s, %+v, %v", file, b, err)
	}
	got, err := f.store.Transaction(ctx, f.admin, tx.ID)
	if err != nil || got.State() != purchases.PendingPayment || *got.BatchID != b.ID {
		t.Errorf("transaction = %+v, %v", got, err)
	}
	if doc, _ := f.store.DocumentByID(ctx, f.admin, d.ID); !doc.Pending {
		t.Error("document is not pending payment")
	}
	q, _ := purchases.ParseFilter("state:pending_payment", time.Now())
	if _, total, _ := f.store.Documents(ctx, f.admin, q); total != 1 {
		t.Errorf("pending documents = %d", total)
	}
	list, total, err := f.store.Transactions(ctx, f.admin, purchases.TransactionQuery{States: []string{purchases.PendingPayment}, BatchID: &b.ID, PerPage: 10, Page: 1})
	if err != nil || total != 1 || list[0].ID != tx.ID {
		t.Errorf("transactions = %v, %d, %v", list, total, err)
	}

	g, err := f.docs.CreateGeneral(ctx, f.admin, f.owner, purchases.GeneralChanges{Reference: ptr("Lease"), ContactID: &f.supplier, ReminderDate: ptr("2026-12-01")})
	if err != nil {
		t.Fatal(err)
	}
	if g.EntryNumber != 2 || g.ReminderDate == nil {
		t.Errorf("general document = %+v", g)
	}
	if list, total, err := f.store.GeneralDocuments(ctx, f.admin, purchases.Query{References: []string{"Lease"}, PerPage: 5}); err != nil || total != 1 || list[0].ID != g.ID {
		t.Errorf("general documents = %v, %d, %v", list, total, err)
	}
	tl, err := f.docs.Upload(ctx, f.admin, f.owner, "scan.pdf", "application/pdf", []byte("%PDF"))
	if err != nil {
		t.Fatal(err)
	}
	if list, total, err := f.store.TypelessDocuments(ctx, f.admin, purchases.Query{Attachment: "with"}); err != nil || total != 1 || list[0].ID != tl.ID {
		t.Errorf("typeless documents = %v, %d, %v", list, total, err)
	}
	sum, err := f.store.Totals(ctx, f.admin)
	if err != nil || sum.ToPay.Count != 1 || sum.ToPay.Amount != 5000 || sum.ToProcess != 1 {
		t.Errorf("totals = %+v, %v", sum, err)
	}
	if err := f.docs.DeleteGeneral(ctx, f.admin, g.ID, f.owner); err != nil {
		t.Fatal(err)
	}
	if err := f.docs.DeleteTypeless(ctx, f.admin, tl.ID, f.owner); err != nil {
		t.Fatal(err)
	}
}
