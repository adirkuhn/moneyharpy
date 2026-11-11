package postgres_test

import (
	"context"
	"errors"
	"os"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/adirkuhn/moneyharpy/internal/administration"
	"github.com/adirkuhn/moneyharpy/internal/attachments"
	"github.com/adirkuhn/moneyharpy/internal/banking"
	"github.com/adirkuhn/moneyharpy/internal/bookkeeping"
	"github.com/adirkuhn/moneyharpy/internal/contacts"
	"github.com/adirkuhn/moneyharpy/internal/invoicing"
	"github.com/adirkuhn/moneyharpy/internal/journal"
	"github.com/adirkuhn/moneyharpy/internal/money"
	"github.com/adirkuhn/moneyharpy/internal/notes"
	"github.com/adirkuhn/moneyharpy/internal/postgres"
	"github.com/adirkuhn/moneyharpy/internal/resource"
	"github.com/adirkuhn/moneyharpy/internal/sales"
)

func TestReserveInvoiceNumbers(t *testing.T) {
	ctx := context.Background()
	a, _, _ := newAdministrationWithIdentity(t)
	store := postgres.NewSalesStore(testPool)
	first, err := store.ReserveNumber(ctx, a.ID)
	if err != nil || first != "0001" {
		t.Fatalf("first = %q, %v", first, err)
	}
	var mu sync.Mutex
	seen := map[string]bool{}
	var wg sync.WaitGroup
	for range 20 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			n, err := store.ReserveNumber(ctx, a.ID)
			if err != nil {
				t.Error(err)
				return
			}
			mu.Lock()
			defer mu.Unlock()
			if seen[n] {
				t.Errorf("number %s given twice", n)
			}
			seen[n] = true
		}()
	}
	wg.Wait()
	next, _ := store.ReserveNumber(ctx, a.ID)
	if next != "0022" || len(seen) != 20 {
		t.Errorf("next = %q after %d", next, len(seen))
	}
}

type manualOnly struct{}

func (manualOnly) PDF(sales.Document) ([]byte, error)         { return []byte("%PDF"), nil }
func (manualOnly) PackingSlip(sales.Document) ([]byte, error) { return []byte("%PDF"), nil }
func (manualOnly) UBL(sales.Document) ([]byte, error)         { return []byte("<Invoice/>"), nil }

type noDocumentMail struct{}

func (noDocumentMail) SendDocument(context.Context, sales.Mail) error { return nil }

type salesFixture struct {
	svc      *sales.Service
	bank     *banking.Service
	books    *bookkeeping.Service
	journal  *journal.Service
	store    *postgres.SalesStore
	admin    resource.ID
	owner    resource.ID
	customer resource.ID
	vat21    resource.ID
	revenue  resource.ID
}

func newSalesFixture(t *testing.T) salesFixture {
	t.Helper()
	ctx := context.Background()
	bank, books, _, admin, owner := bankingService(t)
	admins := administration.NewService(postgres.NewAdministrationStore(testPool), ids, time.Now)
	noteSvc := notes.NewService(postgres.NewNoteStore(testPool), ids, time.Now)
	files := attachments.NewService(postgres.NewAttachmentStore(testPool), diskless{}, ids, time.Now)
	invoices := invoicing.NewService(admins, postgres.NewInvoicingStore(testPool), diskless{}, ids, time.Now)
	store := postgres.NewSalesStore(testPool)
	journalStore := postgres.NewJournalStore(testPool)
	people := contacts.NewService(admins, invoices, admins, contacts.AnyReferences{journalStore, store}, noteSvc, postgres.NewContactStore(testPool), ids, time.Now)
	svc := sales.NewService(sales.Deps{
		Administrations: admins, Contacts: people, Invoicing: invoices, Ledger: books, Notes: noteSvc, Attachments: files,
		Store: store, Renderer: manualOnly{}, Mailer: noDocumentMail{}, Bank: bank, IDs: ids, Now: time.Now,
	})
	bank.UseInvoices(svc)
	customer, err := people.Create(ctx, admin, owner, contacts.Changes{CompanyName: ptr("Example Shop B.V.")})
	if err != nil {
		t.Fatal(err)
	}
	rates, err := books.TaxRates(ctx, admin, owner, bookkeeping.TaxRateFilter{Type: ptr("sales_invoice"), Percentage: ptr("21")})
	if err != nil || len(rates) == 0 {
		t.Fatalf("rates = %v, %v", rates, err)
	}
	return salesFixture{
		svc: svc, bank: bank, books: books, store: store, admin: admin, owner: owner, customer: customer.ID, vat21: rates[0].ID,
		revenue: categoryByKey(t, books, admin, owner, "revenue"),
		journal: journal.NewService(admins, books, people, journalStore, journalStore, noteSvc, files, ids, time.Now),
	}
}

// invoice sends an invoice of total including VAT on day, by hand.
func (f salesFixture) invoice(t *testing.T, total, day string) sales.Invoice {
	t.Helper()
	ctx := context.Background()
	inv, err := f.svc.Create(ctx, f.admin, f.owner, sales.Changes{ContactID: &f.customer, PricesAreInclTax: ptr(true),
		Details: []sales.DetailInput{{Description: ptr("Consulting"), Price: ptr(total), Amount: ptr("1"), TaxRateID: &f.vat21, LedgerAccountID: &f.revenue}}})
	if err != nil {
		t.Fatal(err)
	}
	if inv, err = f.svc.Send(ctx, f.admin, inv.ID, f.owner, sales.SendInput{DeliveryMethod: "Manual", InvoiceDate: day}); err != nil {
		t.Fatal(err)
	}
	return inv
}

func (f salesFixture) ledger(t *testing.T) map[resource.ID]money.Amount {
	t.Helper()
	report, err := f.journal.GeneralLedger(context.Background(), f.admin, f.owner, "202610..202611", "")
	if err != nil {
		t.Fatal(err)
	}
	out := map[resource.ID]money.Amount{}
	var total money.Amount
	for _, s := range report.Sums {
		out[s.LedgerAccountID] = s.Debit - s.Credit
		total += s.Debit - s.Credit
	}
	if total != 0 {
		t.Errorf("journal does not balance: %s", total)
	}
	return out
}

func TestPaymentsAreStoredAndPosted(t *testing.T) {
	ctx := context.Background()
	f := newSalesFixture(t)
	inv := f.invoice(t, "121", "2026-10-01")
	p, err := f.svc.RegisterPayment(ctx, f.admin, inv.ID, f.owner, sales.PaymentInput{PaymentDate: "2026-10-10", Price: ptr("21"), TransactionIdentifier: "PSP-1"})
	if err != nil {
		t.Fatal(err)
	}
	got, err := f.svc.Invoice(ctx, f.admin, inv.ID, f.owner)
	if err != nil || len(got.Payments) != 1 || got.Payments[0].TransactionIdentifier != "PSP-1" || got.Payments[0].Price != 2100 || got.Payments[0].UserID != f.owner {
		t.Fatalf("payments = %+v, %v", got.Payments, err)
	}
	if stored, err := f.store.Payment(ctx, f.admin, p.ID); err != nil || stored.PaymentDate.Format(time.DateOnly) != "2026-10-10" || *stored.LedgerAccountID != *p.LedgerAccountID {
		t.Errorf("stored = %+v, %v", stored, err)
	}
	debtors := categoryByKey(t, f.books, f.admin, f.owner, "accounts_receivable")
	toClassify := categoryByKey(t, f.books, f.admin, f.owner, "received_payments_to_classify")
	sums := f.ledger(t)
	if sums[debtors] != 10000 || sums[toClassify] != 2100 {
		t.Errorf("after part: debtors %s, to classify %s", sums[debtors], sums[toClassify])
	}
	r, err := f.store.Receivables(ctx, f.admin, time.Date(2026, 11, 11, 0, 0, 0, 0, time.UTC))
	if err != nil || r.ToReceive.Count != 1 || r.ToReceive.Amount != 10000 || r.ToRemind.Count != 1 {
		t.Errorf("receivables = %+v, %v", r, err)
	}
	if err := f.svc.DeletePayment(ctx, f.admin, inv.ID, p.ID, f.owner); err != nil {
		t.Fatal(err)
	}
	if _, err := f.store.Payment(ctx, f.admin, p.ID); !errors.Is(err, sales.ErrPaymentNotFound) {
		t.Errorf("deleted payment = %v", err)
	}
	if sums := f.ledger(t); sums[debtors] != 12100 || sums[toClassify] != 0 {
		t.Errorf("after delete: debtors %s, to classify %s", sums[debtors], sums[toClassify])
	}
	work, err := f.store.Workload(ctx)
	if err != nil || !slices.ContainsFunc(work, func(w sales.Workload) bool { return w.AdministrationID == f.admin && w.UserID == f.owner }) {
		t.Errorf("workload = %+v, %v", work, err)
	}
}

func TestBankTransactionPaysAnInvoice(t *testing.T) {
	ctx := context.Background()
	f := newSalesFixture(t)
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
	var incoming banking.Mutation
	for _, m := range result.Statements[0].Mutations {
		if m.Amount == 100000 {
			incoming = m
		}
	}
	inv := f.invoice(t, "1000", "2026-10-20")
	m, err := f.bank.LinkBooking(ctx, f.admin, incoming.ID, f.owner, banking.Link{Type: "SalesInvoice", BookingID: &inv.ID})
	if err != nil {
		t.Fatal(err)
	}
	if m.State != banking.Processed || len(m.Payments) != 1 || m.Payments[0].InvoiceID != inv.ID || m.Payments[0].Price != 100000 || m.Open() != 0 {
		t.Fatalf("mutation = %+v", m)
	}
	got, _ := f.svc.Invoice(ctx, f.admin, inv.ID, f.owner)
	if got.State != sales.Paid || got.PaidAt.Format(time.DateOnly) != "2026-11-01" || *got.Payments[0].FinancialMutationID != incoming.ID {
		t.Errorf("invoice = %s %v", got.State, got.PaidAt)
	}
	debtors := categoryByKey(t, f.books, f.admin, f.owner, "accounts_receivable")
	unprocessed := categoryByKey(t, f.books, f.admin, f.owner, "unprocessed_transactions")
	sums := f.ledger(t)
	// The debtors are settled by the bank; only the outgoing transaction
	// waits on unprocessed transactions.
	if sums[debtors] != 0 || sums[unprocessed] != 2550 {
		t.Errorf("debtors %s, unprocessed %s", sums[debtors], sums[unprocessed])
	}
	if err := f.bank.DeleteStatement(ctx, f.admin, incoming.StatementID, f.owner); !errors.Is(err, banking.ErrStatementBooked) {
		t.Errorf("deleted a statement paying an invoice: %v", err)
	}
	// Deleting the payment from the invoice unlinks the transaction.
	if err := f.svc.DeletePayment(ctx, f.admin, inv.ID, got.Payments[0].ID, f.owner); err != nil {
		t.Fatal(err)
	}
	m, _ = f.bank.Mutation(ctx, f.admin, incoming.ID, f.owner)
	got, _ = f.svc.Invoice(ctx, f.admin, inv.ID, f.owner)
	if m.State != banking.Unprocessed || len(m.Payments) != 0 || got.State == sales.Paid {
		t.Errorf("after delete: mutation %s, invoice %s", m.State, got.State)
	}
	if sums := f.ledger(t); sums[debtors] != 100000 || sums[unprocessed] != -100000+2550 {
		t.Errorf("after delete: debtors %s, unprocessed %s", sums[debtors], sums[unprocessed])
	}
}

func TestSalesWorkLock(t *testing.T) {
	ctx := context.Background()
	store := postgres.NewSalesStore(testPool)
	release, ok, err := store.Lock(ctx, 4242)
	if err != nil || !ok {
		t.Fatalf("lock = %v, %v", ok, err)
	}
	if _, again, err := store.Lock(ctx, 4242); err != nil || again {
		t.Errorf("locked twice: %v, %v", again, err)
	}
	release()
	release2, ok, err := store.Lock(ctx, 4242)
	if err != nil || !ok {
		t.Fatalf("lock after release = %v, %v", ok, err)
	}
	release2()
}

func TestExternalInvoicesAreStoredPostedAndPaid(t *testing.T) {
	ctx := context.Background()
	f := newSalesFixture(t)
	empty, err := f.svc.CreateExternalFromFile(ctx, f.admin, f.owner, "order.pdf", "application/pdf", []byte("%PDF-1.4 order"))
	if err != nil {
		t.Fatal(err)
	}
	if empty.State != sales.New || empty.EntryNumber != 1 || empty.Origin != sales.OriginUpload || len(empty.Attachments) != 1 {
		t.Fatalf("new = %+v", empty)
	}
	e, err := f.svc.UpdateExternal(ctx, f.admin, empty.ID, f.owner, sales.ExternalChanges{
		ContactID: &f.customer, Reference: ptr("WEB-1001"), Date: ptr("2026-10-20"), DueDate: ptr("2026-11-03"), Source: ptr("Example webshop"),
		SourceURL: ptr("https://shop.example.com/orders/1001"), PricesAreInclTax: ptr(true),
		Details: []sales.DetailInput{{Description: ptr("Sofa"), Price: ptr("1000"), Amount: ptr("1"), TaxRateID: &f.vat21, LedgerAccountID: &f.revenue}},
	})
	if err != nil {
		t.Fatal(err)
	}
	got, err := f.store.ExternalInvoice(ctx, f.admin, e.ID)
	if err != nil || got.ContactID != f.customer || got.Reference != "WEB-1001" || got.SourceURL != "https://shop.example.com/orders/1001" ||
		got.Totals.InclTax != 100000 || len(got.Totals.ByRate) != 1 || len(got.Lines) != 1 || got.Lines[0].Description != "Sofa" || got.DueDate.Format(time.DateOnly) != "2026-11-03" {
		t.Fatalf("stored = %+v, %v", got, err)
	}
	debtors := categoryByKey(t, f.books, f.admin, f.owner, "accounts_receivable")
	if sums := f.ledger(t); sums[debtors] != 100000 || sums[f.revenue] != -82645 {
		t.Errorf("booked: debtors %s, revenue %s", sums[debtors], sums[f.revenue])
	}
	q, _ := sales.ParseExternalFilter("state:late,period:202610..202610", time.Now())
	list, total, err := f.store.ExternalInvoices(ctx, f.admin, sales.Query{States: q.States, Period: q.Period, Today: time.Date(2026, 11, 11, 0, 0, 0, 0, time.UTC), PerPage: 10})
	if err != nil || total != 1 || list[0].ID != e.ID {
		t.Errorf("late in October = %d, %v", total, err)
	}
	if byID, err := f.store.ExternalInvoicesByID(ctx, f.admin, []resource.ID{e.ID, 1}); err != nil || len(byID) != 1 {
		t.Errorf("by id = %+v, %v", byID, err)
	}
	r, err := f.store.Receivables(ctx, f.admin, time.Date(2026, 11, 11, 0, 0, 0, 0, time.UTC))
	if err != nil || r.ToReceive.Count != 1 || r.ToReceive.Amount != 100000 {
		t.Errorf("receivables = %+v, %v", r, err)
	}
	if used, err := f.store.ContactInUse(ctx, f.admin, f.customer); err != nil || !used {
		t.Errorf("contact in use = %v, %v", used, err)
	}
	// Only open external invoices give the background job work: marking
	// them late. This one is late already.
	work, err := f.store.Workload(ctx)
	if err != nil || slices.ContainsFunc(work, func(w sales.Workload) bool { return w.AdministrationID == f.admin }) {
		t.Errorf("workload = %+v, %v", work, err)
	}

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
	var incoming banking.Mutation
	for _, m := range result.Statements[0].Mutations {
		if m.Amount == 100000 {
			incoming = m
		}
	}
	m, err := f.bank.LinkBooking(ctx, f.admin, incoming.ID, f.owner, banking.Link{Type: "ExternalSalesInvoice", BookingID: &e.ID})
	if err != nil {
		t.Fatal(err)
	}
	if len(m.Payments) != 1 || m.Payments[0].InvoiceType != "ExternalSalesInvoice" || m.State != banking.Processed {
		t.Fatalf("mutation = %+v", m)
	}
	if paid, _ := f.svc.ExternalInvoice(ctx, f.admin, e.ID, f.owner); paid.State != sales.Paid || len(paid.Payments) != 1 {
		t.Errorf("paid = %+v", paid)
	}
	var contact *int64
	var description string
	if err := testPool.QueryRow(ctx, "SELECT contact_id, description FROM bank_journal_entries WHERE id = $1", int64(m.Payments[0].ID)).Scan(&contact, &description); err != nil {
		t.Fatal(err)
	}
	if contact == nil || resource.ID(*contact) != f.customer || description != "WEB-1001" {
		t.Errorf("bank posting: contact %v, description %q", contact, description)
	}
	if sums := f.ledger(t); sums[debtors] != 0 {
		t.Errorf("paid: debtors %s", sums[debtors])
	}

	// Deleting the invoice unlinks the transaction and removes its postings.
	if err := f.svc.DeleteExternal(ctx, f.admin, e.ID, f.owner); err != nil {
		t.Fatal(err)
	}
	if _, err := f.store.ExternalInvoice(ctx, f.admin, e.ID); !errors.Is(err, sales.ErrNotFound) {
		t.Errorf("deleted = %v", err)
	}
	if m, _ := f.bank.Mutation(ctx, f.admin, incoming.ID, f.owner); m.State != banking.Unprocessed || len(m.Payments) != 0 {
		t.Errorf("after delete: mutation %+v", m)
	}
	if sums := f.ledger(t); sums[debtors] != 0 || sums[f.revenue] != 0 {
		t.Errorf("after delete: debtors %s, revenue %s", sums[debtors], sums[f.revenue])
	}
	if n, err := f.store.NextEntryNumber(ctx, f.admin); err != nil || n != 1 {
		t.Errorf("next entry number = %d, %v", n, err)
	}
}
