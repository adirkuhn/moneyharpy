package purchases

import (
	"context"
	"errors"
	"io"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/adirkuhn/moneyharpy/internal/administration"
	"github.com/adirkuhn/moneyharpy/internal/attachments"
	"github.com/adirkuhn/moneyharpy/internal/bookkeeping"
	"github.com/adirkuhn/moneyharpy/internal/contacts"
	"github.com/adirkuhn/moneyharpy/internal/journal"
	"github.com/adirkuhn/moneyharpy/internal/money"
	"github.com/adirkuhn/moneyharpy/internal/notes"
	"github.com/adirkuhn/moneyharpy/internal/products"
	"github.com/adirkuhn/moneyharpy/internal/projects"
	"github.com/adirkuhn/moneyharpy/internal/resource"
	"github.com/adirkuhn/moneyharpy/internal/sales"
)

const (
	adminID         resource.ID = 100
	owner, stranger resource.ID = 1, 2
	supplier        resource.ID = 300
	noIBAN          resource.ID = 301
	vat21           resource.ID = 500
	reverse21       resource.ID = 501
	noVAT           resource.ID = 502
	salesVAT        resource.ID = 503
	costs           resource.ID = 600
	phone           resource.ID = 601
	privateUse      resource.ID = 602
	creditors       resource.ID = 700
	receivableVAT   resource.ID = 701
	payableVAT      resource.ID = 702
	uncategorized   resource.ID = 703
	toClassify      resource.ID = 704
	cashLedger      resource.ID = 705
	privateDeposits resource.ID = 706
	bankLedger      resource.ID = 707
	settlements     resource.ID = 708
	bankAccount     resource.ID = 800
	cashAccount     resource.ID = 801
	falcon          resource.ID = 850
)

var now = time.Date(2026, 11, 11, 9, 0, 0, 0, time.UTC)

type fakeAdmins struct{}

func (fakeAdmins) Get(_ context.Context, id, user resource.ID) (administration.Listed, error) {
	if id != adminID || user != owner {
		return administration.Listed{}, administration.ErrNotFound
	}
	return administration.Listed{Administration: administration.Administration{ID: adminID, Name: "Example Shop B.V.", Currency: "EUR", TimeZone: "UTC", Country: "NL"}}, nil
}

type fakeCatalog struct{}

func (fakeCatalog) Product(_ context.Context, _, id, _ resource.ID) (products.Product, error) {
	return products.Product{}, products.ErrNotFound
}

func (fakeCatalog) Project(_ context.Context, _, id, _ resource.ID) (projects.Project, error) {
	if id == falcon {
		return projects.Project{ID: id, Name: "Falcon", State: projects.Active}, nil
	}
	return projects.Project{}, projects.ErrNotFound
}

type fakeContacts struct{}

func (fakeContacts) Contact(_ context.Context, _, id, _ resource.ID) (contacts.Contact, error) {
	switch id {
	case supplier:
		return contacts.Contact{ID: supplier, CompanyName: "Supplier B.V.", SepaIBAN: "NL44RABO0123456789", SepaBIC: "RABONL2U"}, nil
	case noIBAN:
		return contacts.Contact{ID: noIBAN, CompanyName: "Cash Only"}, nil
	}
	return contacts.Contact{}, contacts.ErrNotFound
}

type fakeLedger struct{ locked string }

func (f *fakeLedger) Bookable(_ context.Context, _, _, id resource.ID, documentType string) (string, error) {
	if documentType == "purchase_invoice" && (id == costs || id == phone || id == uncategorized) {
		return "", nil
	}
	return "is not a category of this administration", nil
}

func (f *fakeLedger) SystemLedgerAccount(_ context.Context, _, _ resource.ID, role string) (resource.ID, error) {
	id, ok := map[string]resource.ID{
		"accounts_payable": creditors, "receivable_vat": receivableVAT, "payable_vat": payableVAT, "uncategorized_expenses": uncategorized,
		"payments_to_classify": toClassify, "settlements": settlements,
	}[role]
	if !ok {
		return 0, errors.New("no such role")
	}
	return id, nil
}

func (f *fakeLedger) SeedLedgerAccount(_ context.Context, _, _ resource.ID, key string) (resource.ID, bool, error) {
	switch key {
	case "cash":
		return cashLedger, true, nil
	case "private_deposits":
		return privateDeposits, true, nil
	}
	return 0, false, nil
}

func (f *fakeLedger) LedgerAccount(_ context.Context, _, id, _ resource.ID) (bookkeeping.LedgerAccount, error) {
	switch id {
	case costs, phone:
		return bookkeeping.LedgerAccount{ID: id, Type: "expenses", Active: true}, nil
	case privateUse:
		return bookkeeping.LedgerAccount{ID: id, Type: "equity", Active: true}, nil
	}
	return bookkeeping.LedgerAccount{}, bookkeeping.ErrLedgerAccountNotFound
}

func (f *fakeLedger) TaxRate(_ context.Context, _, id, _ resource.ID) (bookkeeping.TaxRate, error) {
	switch id {
	case vat21:
		return bookkeeping.TaxRate{ID: id, Percentage: "21", Type: "purchase_invoice", ShowTax: true, Active: true}, nil
	case reverse21:
		return bookkeeping.TaxRate{ID: id, Percentage: "21", Type: "purchase_invoice", Active: true}, nil
	case noVAT:
		return bookkeeping.TaxRate{ID: id, Percentage: "0", Type: "purchase_invoice", Active: true}, nil
	case salesVAT:
		return bookkeeping.TaxRate{ID: id, Percentage: "21", Type: "sales_invoice", ShowTax: true, Active: true}, nil
	}
	return bookkeeping.TaxRate{}, bookkeeping.ErrTaxRateNotFound
}

func (f *fakeLedger) LockedUntil(context.Context, resource.ID, resource.ID) (string, error) {
	return f.locked, nil
}

// memStore keeps documents in memory, as the postgres store does.
type memStore struct {
	docs         map[resource.ID]Document
	entries      map[resource.ID][]journal.Entry
	paid         map[resource.ID][]journal.Entry
	general      map[resource.ID]General
	typeless     map[resource.ID]Typeless
	transactions map[resource.ID]Transaction
	batches      map[resource.ID]Batch
	number       int
}

func newMemStore() *memStore {
	return &memStore{
		docs: map[resource.ID]Document{}, entries: map[resource.ID][]journal.Entry{}, paid: map[resource.ID][]journal.Entry{},
		general: map[resource.ID]General{}, typeless: map[resource.ID]Typeless{}, transactions: map[resource.ID]Transaction{}, batches: map[resource.ID]Batch{},
	}
}

// fill works out what the postgres store reads with a document.
func (m *memStore) fill(d Document) Document {
	d.Pending = false
	for _, t := range m.transactions {
		if t.PayableID != nil && *t.PayableID == d.ID && t.BatchID != nil && !t.Cancelled {
			d.Pending = true
		}
	}
	d.Payments = slices.Clone(d.Payments)
	return d
}

func (m *memStore) matches(q Query, d Document) bool {
	state := d.EffectiveState(q.Today)
	switch {
	case q.None,
		q.Kind != "" && d.Kind != q.Kind,
		len(q.States) > 0 && !slices.Contains(q.States, state),
		q.Period != nil && !q.Period.Contains(d.Date),
		len(q.References) > 0 && !slices.Contains(q.References, d.Reference),
		q.ContactID != nil && d.ContactID != *q.ContactID:
		return false
	}
	return true
}

func (m *memStore) Documents(_ context.Context, _ resource.ID, q Query) ([]Document, int, error) {
	var out []Document
	for _, d := range m.docs {
		if d = m.fill(d); m.matches(q, d) {
			out = append(out, d)
		}
	}
	slices.SortFunc(out, func(a, b Document) int { return int(b.ID - a.ID) })
	return out, len(out), nil
}

func (m *memStore) Document(_ context.Context, _ resource.ID, kind string, id resource.ID) (Document, error) {
	d, ok := m.docs[id]
	if !ok || d.Kind != kind {
		return Document{}, ErrNotFound
	}
	return m.fill(d), nil
}

func (m *memStore) DocumentByID(_ context.Context, _, id resource.ID) (Document, error) {
	d, ok := m.docs[id]
	if !ok {
		return Document{}, ErrNotFound
	}
	return m.fill(d), nil
}

func (m *memStore) DocumentsByID(ctx context.Context, admin resource.ID, kind string, ids []resource.ID) ([]Document, error) {
	var out []Document
	for _, id := range ids {
		if d, err := m.Document(ctx, admin, kind, id); err == nil {
			out = append(out, d)
		}
	}
	return out, nil
}

func (m *memStore) NextEntryNumber(context.Context, resource.ID) (int, error) {
	m.number++
	return m.number, nil
}

func (m *memStore) SaveDocument(_ context.Context, d Document, entries []journal.Entry, _ bool) error {
	d.Notes, d.Attachments = nil, nil
	m.docs[d.ID] = d
	m.entries[d.ID] = entries
	return nil
}

func (m *memStore) DeleteDocument(_ context.Context, _, id resource.ID) error {
	for _, p := range m.docs[id].Payments {
		delete(m.paid, p.ID)
	}
	delete(m.docs, id)
	delete(m.entries, id)
	return nil
}

func (m *memStore) TouchDocument(_ context.Context, _, id resource.ID, at time.Time) error {
	d := m.docs[id]
	d.UpdatedAt = at
	m.docs[id] = d
	return nil
}

func (m *memStore) Payment(_ context.Context, _, id resource.ID) (sales.Payment, error) {
	for _, d := range m.docs {
		for _, p := range d.Payments {
			if p.ID == id {
				return p, nil
			}
		}
	}
	return sales.Payment{}, ErrPaymentNotFound
}

func (m *memStore) SavePayments(_ context.Context, docs []Document, added []PaymentPosting, removed []resource.ID) error {
	for _, d := range docs {
		stored := m.docs[d.ID]
		stored.State, stored.PaidAt, stored.Payments, stored.UpdatedAt = d.State, d.PaidAt, d.Payments, d.UpdatedAt
		m.docs[d.ID] = stored
	}
	for _, p := range added {
		m.paid[p.Payment.ID] = p.Entries
	}
	for _, id := range removed {
		delete(m.paid, id)
	}
	return nil
}

func (m *memStore) GeneralDocuments(_ context.Context, _ resource.ID, q Query) ([]General, int, error) {
	var out []General
	for _, g := range m.general {
		if q.Period == nil || q.Period.Contains(g.Date) {
			out = append(out, g)
		}
	}
	return out, len(out), nil
}

func (m *memStore) GeneralDocument(_ context.Context, _, id resource.ID) (General, error) {
	g, ok := m.general[id]
	if !ok {
		return General{}, ErrNotFound
	}
	return g, nil
}

func (m *memStore) GeneralDocumentsByID(ctx context.Context, admin resource.ID, ids []resource.ID) ([]General, error) {
	var out []General
	for _, id := range ids {
		if g, err := m.GeneralDocument(ctx, admin, id); err == nil {
			out = append(out, g)
		}
	}
	return out, nil
}

func (m *memStore) SaveGeneralDocument(_ context.Context, g General, _ bool) error {
	m.general[g.ID] = g
	return nil
}

func (m *memStore) DeleteGeneralDocument(_ context.Context, _, id resource.ID) error {
	delete(m.general, id)
	return nil
}

func (m *memStore) TouchGeneralDocument(_ context.Context, _, id resource.ID, at time.Time) error {
	g := m.general[id]
	g.UpdatedAt = at
	m.general[id] = g
	return nil
}

func (m *memStore) TypelessDocuments(_ context.Context, _ resource.ID, q Query) ([]Typeless, int, error) {
	var out []Typeless
	for _, t := range m.typeless {
		if q.Period == nil || q.Period.Contains(t.Date) {
			out = append(out, t)
		}
	}
	return out, len(out), nil
}

func (m *memStore) TypelessDocument(_ context.Context, _, id resource.ID) (Typeless, error) {
	t, ok := m.typeless[id]
	if !ok {
		return Typeless{}, ErrNotFound
	}
	return t, nil
}

func (m *memStore) TypelessDocumentsByID(ctx context.Context, admin resource.ID, ids []resource.ID) ([]Typeless, error) {
	var out []Typeless
	for _, id := range ids {
		if t, err := m.TypelessDocument(ctx, admin, id); err == nil {
			out = append(out, t)
		}
	}
	return out, nil
}

func (m *memStore) SaveTypelessDocument(_ context.Context, t Typeless, _ bool) error {
	m.typeless[t.ID] = t
	return nil
}

func (m *memStore) DeleteTypelessDocument(_ context.Context, _, id resource.ID) error {
	delete(m.typeless, id)
	return nil
}

func (m *memStore) TouchTypelessDocument(_ context.Context, _, id resource.ID, at time.Time) error {
	t := m.typeless[id]
	t.UpdatedAt = at
	m.typeless[id] = t
	return nil
}

func (m *memStore) fillTransaction(t Transaction) Transaction {
	if t.PayableID != nil {
		t.PayablePaid = m.docs[*t.PayableID].State == Paid
	}
	return t
}

func (m *memStore) Transactions(_ context.Context, _ resource.ID, q TransactionQuery) ([]Transaction, int, error) {
	var out []Transaction
	for _, t := range m.transactions {
		t = m.fillTransaction(t)
		switch {
		case len(q.States) > 0 && !slices.Contains(q.States, t.State()),
			q.Unbatched && t.BatchID != nil,
			q.BatchID != nil && (t.BatchID == nil || *t.BatchID != *q.BatchID):
			continue
		}
		out = append(out, t)
	}
	slices.SortFunc(out, func(a, b Transaction) int { return int(a.ID - b.ID) })
	return out, len(out), nil
}

func (m *memStore) Transaction(_ context.Context, _, id resource.ID) (Transaction, error) {
	t, ok := m.transactions[id]
	if !ok {
		return Transaction{}, ErrTransactionNotFound
	}
	return m.fillTransaction(t), nil
}

func (m *memStore) SaveTransactions(_ context.Context, b *Batch, list []Transaction, _ bool) error {
	if b != nil {
		m.batches[b.ID] = *b
	}
	for _, t := range list {
		m.transactions[t.ID] = t
	}
	return nil
}

func (m *memStore) DeleteTransaction(_ context.Context, _, id resource.ID) error {
	delete(m.transactions, id)
	return nil
}

func (m *memStore) Batch(_ context.Context, _, id resource.ID) (Batch, error) {
	b, ok := m.batches[id]
	if !ok {
		return Batch{}, ErrTransactionNotFound
	}
	return b, nil
}

func (m *memStore) SaveBatch(_ context.Context, b Batch) error {
	m.batches[b.ID] = b
	return nil
}

func (m *memStore) Totals(_ context.Context, _ resource.ID) (Summary, error) {
	var s Summary
	for _, d := range m.docs {
		switch d.State {
		case Open:
			s.ToPay.Count++
			s.ToPay.Amount += d.Unpaid()
		case New:
			s.ToProcess++
		}
	}
	s.ToProcess += len(m.typeless)
	return s, nil
}

// fakeBank links payments to transactions and calls back as banking does.
type fakeBank struct {
	svc  *Service
	open map[resource.ID]money.Amount
	day  time.Time
}

func (f *fakeBank) LinkInvoicePayment(ctx context.Context, admin, mutation, user resource.ID, invoiceType string, invoice resource.ID, price *money.Amount) (resource.ID, error) {
	open, ok := f.open[mutation]
	if !ok {
		return 0, errors.New("no such transaction")
	}
	// Money out is negative on the bank; the document's sign is the other.
	id, err := f.svc.RegisterBankPayment(ctx, admin, user, invoiceType, invoice, mutation, bankAccount, f.day, -open, price)
	if err != nil {
		return 0, err
	}
	p, _ := f.svc.store.Payment(ctx, admin, id)
	f.open[mutation] += p.Price
	return id, nil
}

func (f *fakeBank) UnlinkPayment(ctx context.Context, admin, mutation, user, payment resource.ID) error {
	p, err := f.svc.store.Payment(ctx, admin, payment)
	if err != nil {
		return err
	}
	if err := f.svc.RemoveBankPayment(ctx, admin, user, payment); err != nil {
		return err
	}
	f.open[mutation] -= p.Price
	return nil
}

func (f *fakeBank) accounts() []BankAccount {
	return []BankAccount{
		{ID: bankAccount, Name: "Current account", Type: BankAccountType, IBAN: "NL91ABNA0417164300", BIC: "ABNANL2A", LedgerAccountID: bankLedger},
		{ID: cashAccount, Name: "Cash", Type: "CashAccount", LedgerAccountID: cashLedger},
	}
}

func (f *fakeBank) FinancialAccount(_ context.Context, _, account, _ resource.ID) (BankAccount, error) {
	for _, a := range f.accounts() {
		if a.ID == account {
			return a, nil
		}
	}
	return BankAccount{}, errors.New("no such account")
}

func (f *fakeBank) FinancialAccounts(context.Context, resource.ID, resource.ID) ([]BankAccount, error) {
	return f.accounts(), nil
}

type memNotes struct{ list []notes.Note }

func (m *memNotes) CreateNote(_ context.Context, n notes.Note) error {
	m.list = append(m.list, n)
	return nil
}
func (m *memNotes) DeleteNote(_ context.Context, _, _, id resource.ID) error {
	m.list = slices.DeleteFunc(m.list, func(n notes.Note) bool { return n.ID == id })
	return nil
}
func (m *memNotes) NotesOf(_ context.Context, _ resource.ID, t string, ids []resource.ID) ([]notes.Note, error) {
	var out []notes.Note
	for _, n := range m.list {
		if n.EntityType == t && slices.Contains(ids, n.EntityID) {
			out = append(out, n)
		}
	}
	return out, nil
}
func (m *memNotes) IsMember(context.Context, resource.ID, resource.ID) (bool, error) {
	return true, nil
}

type memAttachments struct{ list []attachments.Attachment }

func (m *memAttachments) CreateAttachment(_ context.Context, a attachments.Attachment) error {
	m.list = append(m.list, a)
	return nil
}
func (m *memAttachments) DeleteAttachment(_ context.Context, _, id resource.ID) error {
	m.list = slices.DeleteFunc(m.list, func(a attachments.Attachment) bool { return a.ID == id })
	return nil
}
func (m *memAttachments) AttachmentsOf(_ context.Context, _ resource.ID, t string, ids []resource.ID) ([]attachments.Attachment, error) {
	var out []attachments.Attachment
	for _, a := range m.list {
		if a.EntityType == t && slices.Contains(ids, a.EntityID) {
			out = append(out, a)
		}
	}
	return out, nil
}

type noFiles struct{}

func (noFiles) Put(context.Context, string, string, []byte) error { return nil }
func (noFiles) Open(context.Context, string) (io.ReadCloser, string, error) {
	return nil, "", errors.New("none")
}
func (noFiles) Delete(context.Context, string) error { return nil }

type harness struct {
	svc    *Service
	store  *memStore
	ledger *fakeLedger
	bank   *fakeBank
}

func newHarness(t *testing.T) harness {
	t.Helper()
	store := newMemStore()
	ledger := &fakeLedger{}
	ids := resource.NewGenerator(1)
	clock := func() time.Time { return now }
	bank := &fakeBank{open: map[resource.ID]money.Amount{}, day: now}
	svc := NewService(Deps{
		Administrations: fakeAdmins{}, Contacts: fakeContacts{}, Products: fakeCatalog{}, Projects: fakeCatalog{}, Ledger: ledger,
		Notes: notes.NewService(&memNotes{}, ids, clock), Attachments: attachments.NewService(&memAttachments{}, noFiles{}, ids, clock),
		Store: store, Bank: bank, IDs: ids, Now: clock,
	})
	bank.svc = svc
	return harness{svc: svc, store: store, ledger: ledger, bank: bank}
}

func ptr[T any](v T) *T { return &v }

func (f fakeContacts) List(ctx context.Context, admin, user resource.ID, q contacts.Query) (contacts.Page, error) {
	var page contacts.Page
	for _, id := range []resource.ID{supplier, noIBAN} {
		c, _ := f.Contact(ctx, admin, id, user)
		if q.Search != "" && strings.Contains(c.SepaIBAN, q.Search) {
			page.Contacts = append(page.Contacts, c)
		}
	}
	return page, nil
}

func (f *fakeLedger) TaxRates(ctx context.Context, admin, user resource.ID, filter bookkeeping.TaxRateFilter) ([]bookkeeping.TaxRate, error) {
	var out []bookkeeping.TaxRate
	for _, id := range []resource.ID{vat21, reverse21, noVAT, salesVAT} {
		r, _ := f.TaxRate(ctx, admin, id, user)
		if (filter.Type == nil || *filter.Type == r.Type) && (filter.Percentage == nil || *filter.Percentage == r.Percentage) && (filter.Active == nil || *filter.Active == r.Active) {
			out = append(out, r)
		}
	}
	return out, nil
}
