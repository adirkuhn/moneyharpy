package sales

import (
	"context"
	"errors"
	"fmt"
	"io"
	"slices"
	"strconv"
	"testing"
	"time"

	"github.com/adirkuhn/moneyharpy/internal/administration"
	"github.com/adirkuhn/moneyharpy/internal/attachments"
	"github.com/adirkuhn/moneyharpy/internal/bookkeeping"
	"github.com/adirkuhn/moneyharpy/internal/contacts"
	"github.com/adirkuhn/moneyharpy/internal/invoicing"
	"github.com/adirkuhn/moneyharpy/internal/journal"
	"github.com/adirkuhn/moneyharpy/internal/money"
	"github.com/adirkuhn/moneyharpy/internal/notes"
	"github.com/adirkuhn/moneyharpy/internal/resource"
)

const (
	adminID         resource.ID = 100
	owner, stranger resource.ID = 1, 2
	customer        resource.ID = 300
	noEmail         resource.ID = 301
	flow            resource.ID = 400
	estimateFlow    resource.ID = 401
	style           resource.ID = 450
	identity        resource.ID = 460
	vat21, vat9     resource.ID = 500, 501
	purchaseVAT     resource.ID = 502
	revenue         resource.ID = 600
	consulting      resource.ID = 601
	debtors         resource.ID = 700
	payableVAT      resource.ID = 701
	uncategorized   resource.ID = 702
	toClassify      resource.ID = 703
	cashLedger      resource.ID = 704
	privateDeposits resource.ID = 705
	rounding        resource.ID = 706
	settlements     resource.ID = 707
	allocation      resource.ID = 708
	provision       resource.ID = 709
	uncollectible   resource.ID = 710
	loan            resource.ID = 711
	bankLedger      resource.ID = 712
	bankAccount     resource.ID = 800
)

var now = time.Date(2026, 11, 11, 9, 0, 0, 0, time.UTC)

type fakeAdmins struct{}

func (fakeAdmins) Get(_ context.Context, id, user resource.ID) (administration.Listed, error) {
	if id != adminID || user != owner {
		return administration.Listed{}, administration.ErrNotFound
	}
	return administration.Listed{Administration: administration.Administration{ID: adminID, Currency: "EUR", Language: "en", TimeZone: "UTC", Country: "NL"}}, nil
}

func (fakeAdmins) DefaultIdentity(context.Context, resource.ID, resource.ID) (administration.Identity, error) {
	return administration.Identity{ID: identity, CompanyName: "Harpy B.V.", Email: "books@example.com"}, nil
}

func (f fakeAdmins) Identity(ctx context.Context, admin, id, user resource.ID) (administration.Identity, error) {
	if id != identity {
		return administration.Identity{}, administration.ErrIdentityNotFound
	}
	return f.DefaultIdentity(ctx, admin, user)
}

func (fakeAdmins) CustomFields(context.Context, resource.ID, resource.ID) ([]administration.CustomField, error) {
	return nil, nil
}

type fakeContacts struct{}

func (fakeContacts) Contact(_ context.Context, _, id, _ resource.ID) (contacts.Contact, error) {
	switch id {
	case customer:
		return contacts.Contact{ID: customer, CompanyName: "Example Shop B.V.", CustomerID: "7", DeliveryMethod: "Email", SendInvoicesToEmail: "billing@example.com"}, nil
	case noEmail:
		return contacts.Contact{ID: noEmail, Firstname: "Ann", CustomerID: "8", DeliveryMethod: "Email"}, nil
	}
	return contacts.Contact{}, contacts.ErrNotFound
}

type fakeInvoicing struct{}

func (f *fakeInvoicing) Settings(context.Context, resource.ID, resource.ID) (invoicing.SettingsView, error) {
	return invoicing.SettingsView{Settings: invoicing.Settings{InvoiceIDTemplate: "{year}-{id}", InvoiceBCC: "copy@example.com"}}, nil
}

func (f *fakeInvoicing) workflows() []invoicing.Workflow {
	return []invoicing.Workflow{
		{ID: flow, Type: invoicing.InvoiceWorkflow, Default: true, Active: true, Currency: "EUR", Language: "nl", Steps: []invoicing.WorkflowStep{
			{StepOrder: 1, DueInterval: 14},
			{StepOrder: 2, ReminderDelay: 7, ReminderText: "Please pay {invoice_id}.\n\n{invoices}", ReminderAutoSend: true},
			{StepOrder: 3, ReminderDelay: 14},
		}},
		{ID: estimateFlow, Type: invoicing.EstimateWorkflow, Default: true, Active: true},
	}
}

func (f *fakeInvoicing) Workflow(_ context.Context, _, id, _ resource.ID) (invoicing.Workflow, error) {
	for _, w := range f.workflows() {
		if w.ID == id {
			return w, nil
		}
	}
	return invoicing.Workflow{}, invoicing.ErrWorkflowNotFound
}

func (f *fakeInvoicing) Workflows(context.Context, resource.ID, resource.ID) ([]invoicing.Workflow, error) {
	return f.workflows(), nil
}

func (f *fakeInvoicing) DocumentStyle(_ context.Context, _, id, _ resource.ID) (invoicing.DocumentStyle, error) {
	if id != style {
		return invoicing.DocumentStyle{}, invoicing.ErrDocumentStyleNotFound
	}
	return invoicing.DocumentStyle{ID: style, Default: true}, nil
}

func (f *fakeInvoicing) DocumentStyles(context.Context, resource.ID, resource.ID) ([]invoicing.DocumentStyle, error) {
	return []invoicing.DocumentStyle{{ID: style, Default: true}}, nil
}

func (f *fakeInvoicing) OpenFile(context.Context, resource.ID, resource.ID, resource.ID, invoicing.FileKind) (io.ReadCloser, string, error) {
	return nil, "", invoicing.ErrFileNotFound
}

type fakeLedger struct{ locked string }

func (f *fakeLedger) Bookable(_ context.Context, _, _, id resource.ID, _ string) (string, error) {
	if id == revenue || id == consulting {
		return "", nil
	}
	return "is not a category of this administration", nil
}

func (f *fakeLedger) SystemLedgerAccount(_ context.Context, _, _ resource.ID, role string) (resource.ID, error) {
	switch role {
	case "accounts_receivable":
		return debtors, nil
	case "payable_vat":
		return payableVAT, nil
	case "uncategorized_income":
		return uncategorized, nil
	case "received_payments_to_classify":
		return toClassify, nil
	case "rounding":
		return rounding, nil
	case "settlements":
		return settlements, nil
	case "doubtful_debtors_allocation":
		return allocation, nil
	case "doubtful_debtors_provision":
		return provision, nil
	case "uncollectible_revenue":
		return uncollectible, nil
	}
	return 0, errors.New("no such role")
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
	case loan:
		return bookkeeping.LedgerAccount{ID: loan, Type: "non_current_liabilities", Active: true}, nil
	case revenue:
		return bookkeeping.LedgerAccount{ID: revenue, Type: "revenue", Active: true}, nil
	case debtors:
		return bookkeeping.LedgerAccount{ID: debtors, Type: "current_assets", Active: true, System: "accounts_receivable"}, nil
	}
	return bookkeeping.LedgerAccount{}, bookkeeping.ErrLedgerAccountNotFound
}

func (f *fakeLedger) TaxRate(_ context.Context, _, id, _ resource.ID) (bookkeeping.TaxRate, error) {
	switch id {
	case vat21:
		return bookkeeping.TaxRate{ID: vat21, Name: "21%", Percentage: "21", Type: "sales_invoice", Active: true}, nil
	case vat9:
		return bookkeeping.TaxRate{ID: vat9, Name: "9%", Percentage: "9", Type: "sales_invoice", Active: true}, nil
	case purchaseVAT:
		return bookkeeping.TaxRate{ID: purchaseVAT, Percentage: "21", Type: "purchase_invoice", Active: true}, nil
	}
	return bookkeeping.TaxRate{}, bookkeeping.ErrTaxRateNotFound
}

func (f *fakeLedger) LockedUntil(context.Context, resource.ID, resource.ID) (string, error) {
	return f.locked, nil
}

type memStore struct {
	invoices map[resource.ID]Invoice
	entries  map[resource.ID][]journal.Entry
	// externals and externalEntries hold external invoices and what they post.
	externals       map[resource.ID]ExternalInvoice
	externalEntries map[resource.ID][]journal.Entry
	// paid holds what payments post, by payment.
	paid map[resource.ID][]journal.Entry
	// recurring holds recurring invoices.
	recurring map[resource.ID]Recurring
	sequence  int
	locked    map[resource.ID]bool
}

func (m *memStore) RecurringInvoices(_ context.Context, _ resource.ID, q RecurringQuery) ([]Recurring, int, error) {
	var out []Recurring
	for _, r := range m.recurring {
		if q.Matches(r) {
			out = append(out, r)
		}
	}
	slices.SortFunc(out, func(a, b Recurring) int {
		if c := a.InvoiceDate.Compare(b.InvoiceDate); c != 0 {
			return c
		}
		return int(a.ID - b.ID)
	})
	return out, len(out), nil
}

func (m *memStore) RecurringInvoice(_ context.Context, _, id resource.ID) (Recurring, error) {
	r, ok := m.recurring[id]
	if !ok {
		return Recurring{}, ErrNotFound
	}
	return r, nil
}

func (m *memStore) RecurringInvoicesByID(ctx context.Context, admin resource.ID, ids []resource.ID) ([]Recurring, error) {
	var out []Recurring
	for _, id := range ids {
		if r, err := m.RecurringInvoice(ctx, admin, id); err == nil {
			out = append(out, r)
		}
	}
	return out, nil
}

func (m *memStore) SaveRecurring(_ context.Context, r Recurring, _ bool) error {
	m.recurring[r.ID] = r
	return nil
}

func (m *memStore) DeleteRecurring(_ context.Context, _, id resource.ID) error {
	delete(m.recurring, id)
	return nil
}

func (m *memStore) TouchRecurring(_ context.Context, _, id resource.ID, at time.Time) error {
	r := m.recurring[id]
	r.UpdatedAt = at
	m.recurring[id] = r
	return nil
}

func (m *memStore) SaveGenerated(_ context.Context, inv Invoice, r Recurring) error {
	m.invoices[inv.ID] = inv
	m.recurring[r.ID] = r
	return nil
}

// fromRecurring lists the invoices a recurring invoice made, by date.
func (m *memStore) fromRecurring(id resource.ID) []Invoice {
	var out []Invoice
	for _, inv := range m.invoices {
		if inv.RecurringSalesInvoiceID != nil && *inv.RecurringSalesInvoiceID == id {
			out = append(out, inv)
		}
	}
	slices.SortFunc(out, func(a, b Invoice) int { return a.InvoiceDate.Compare(*b.InvoiceDate) })
	return out
}

func (m *memStore) Payment(_ context.Context, _, id resource.ID) (Payment, error) {
	var all []Payment
	for _, inv := range m.invoices {
		all = append(all, inv.Payments...)
	}
	for _, e := range m.externals {
		all = append(all, e.Payments...)
	}
	for _, p := range all {
		if p.ID == id {
			return p, nil
		}
	}
	return Payment{}, ErrPaymentNotFound
}

func (m *memStore) SavePayments(_ context.Context, c PaymentChange) error {
	for _, p := range c.Invoices {
		if p.Invoice.Type == ExternalEntityType {
			e := m.externals[p.Invoice.ID]
			e.Settled(p.Invoice)
			m.externals[e.ID] = e
			if p.Entries != nil {
				m.externalEntries[e.ID] = p.Entries
			}
			continue
		}
		m.invoices[p.Invoice.ID] = p.Invoice
		if p.Entries != nil {
			m.entries[p.Invoice.ID] = p.Entries
		}
	}
	for _, p := range c.Payments {
		m.paid[p.Payment.ID] = p.Entries
	}
	for _, id := range c.Removed {
		delete(m.paid, id)
	}
	return nil
}

func (m *memStore) Receivables(_ context.Context, _ resource.ID, today time.Time) (Receivables, error) {
	var r Receivables
	for _, e := range m.externals {
		if payable(e.invoice()) {
			r.ToReceive.Count++
			r.ToReceive.Amount += e.Unpaid()
		}
	}
	for _, inv := range m.invoices {
		if !payable(inv) {
			continue
		}
		r.ToReceive.Count++
		r.ToReceive.Amount += inv.Unpaid()
		if st := effective(inv, today); (st == Late || st == Reminded) && !inv.Paused {
			r.ToRemind.Count++
			r.ToRemind.Amount += inv.Unpaid()
		}
	}
	return r, nil
}

func (m *memStore) Workload(context.Context) ([]Workload, error) {
	return []Workload{{AdministrationID: adminID, UserID: owner}}, nil
}

func (m *memStore) Lock(_ context.Context, admin resource.ID) (func(), bool, error) {
	if m.locked[admin] {
		return nil, false, nil
	}
	m.locked[admin] = true
	return func() { m.locked[admin] = false }, true, nil
}

func (m *memStore) Invoices(_ context.Context, _ resource.ID, q Query) ([]Invoice, int, error) {
	var out []Invoice
	for _, inv := range m.invoices {
		if q.Matches(inv) {
			out = append(out, inv)
		}
	}
	slices.SortFunc(out, func(a, b Invoice) int { return int(a.ID - b.ID) })
	return out, len(out), nil
}

func (m *memStore) Invoice(_ context.Context, _, id resource.ID) (Invoice, error) {
	inv, ok := m.invoices[id]
	if !ok {
		return Invoice{}, ErrNotFound
	}
	return inv, nil
}

func (m *memStore) InvoicesByID(ctx context.Context, admin resource.ID, ids []resource.ID) ([]Invoice, error) {
	var out []Invoice
	for _, id := range ids {
		if inv, err := m.Invoice(ctx, admin, id); err == nil {
			out = append(out, inv)
		}
	}
	return out, nil
}

func (m *memStore) find(match func(Invoice) bool) (Invoice, error) {
	for _, inv := range m.invoices {
		if match(inv) {
			return inv, nil
		}
	}
	return Invoice{}, ErrNotFound
}

func (m *memStore) InvoiceByNumber(_ context.Context, _ resource.ID, number string) (Invoice, error) {
	return m.find(func(i Invoice) bool { return i.InvoiceID == number })
}

func (m *memStore) InvoiceByReference(_ context.Context, _ resource.ID, ref string) (Invoice, error) {
	return m.find(func(i Invoice) bool { return i.Reference == ref })
}

func (m *memStore) NextDraftID(context.Context, resource.ID) (int, error) {
	highest := 0
	for _, inv := range m.invoices {
		highest = max(highest, inv.DraftID)
	}
	return highest + 1, nil
}

func (m *memStore) ReserveNumber(context.Context, resource.ID) (string, error) {
	m.sequence++
	return fmt.Sprintf("%04d", m.sequence), nil
}

func (m *memStore) NumberTaken(_ context.Context, _ resource.ID, number string, except resource.ID) (bool, error) {
	_, err := m.find(func(i Invoice) bool { return i.InvoiceID == number && i.ID != except })
	return err == nil, nil
}

func (m *memStore) SaveInvoice(_ context.Context, inv Invoice, entries []journal.Entry, _ bool) error {
	m.invoices[inv.ID] = inv
	if entries != nil {
		m.entries[inv.ID] = entries
	}
	return nil
}

func (m *memStore) DeleteInvoice(_ context.Context, _, id resource.ID) error {
	delete(m.invoices, id)
	delete(m.entries, id)
	return nil
}

func (m *memStore) TouchInvoice(_ context.Context, _, id resource.ID, at time.Time) error {
	inv := m.invoices[id]
	inv.UpdatedAt = at
	m.invoices[id] = inv
	return nil
}

func (m *memStore) ContactInUse(_ context.Context, _, contact resource.ID) (bool, error) {
	_, err := m.find(func(i Invoice) bool { return i.ContactID == contact })
	for _, r := range m.recurring {
		if r.ContactID == contact {
			return true, nil
		}
	}
	return err == nil, nil
}

func (m *memStore) ExternalInvoices(_ context.Context, _ resource.ID, q Query) ([]ExternalInvoice, int, error) {
	var out []ExternalInvoice
	for _, e := range m.externals {
		if q.MatchesExternal(e) {
			out = append(out, e)
		}
	}
	slices.SortFunc(out, func(a, b ExternalInvoice) int { return int(a.ID - b.ID) })
	return out, len(out), nil
}

func (m *memStore) ExternalInvoice(_ context.Context, _, id resource.ID) (ExternalInvoice, error) {
	e, ok := m.externals[id]
	if !ok {
		return ExternalInvoice{}, ErrNotFound
	}
	return e, nil
}

func (m *memStore) ExternalInvoicesByID(ctx context.Context, admin resource.ID, ids []resource.ID) ([]ExternalInvoice, error) {
	var out []ExternalInvoice
	for _, id := range ids {
		if e, err := m.ExternalInvoice(ctx, admin, id); err == nil {
			out = append(out, e)
		}
	}
	return out, nil
}

func (m *memStore) NextEntryNumber(context.Context, resource.ID) (int, error) {
	highest := 0
	for _, e := range m.externals {
		highest = max(highest, e.EntryNumber)
	}
	return highest + 1, nil
}

func (m *memStore) SaveExternalInvoice(_ context.Context, e ExternalInvoice, entries []journal.Entry, _ bool) error {
	m.externals[e.ID] = e
	if entries != nil {
		m.externalEntries[e.ID] = entries
	}
	return nil
}

func (m *memStore) DeleteExternalInvoice(_ context.Context, _, id resource.ID) error {
	for _, p := range m.externals[id].Payments {
		delete(m.paid, p.ID)
	}
	delete(m.externals, id)
	delete(m.externalEntries, id)
	return nil
}

func (m *memStore) TouchExternalInvoice(_ context.Context, _, id resource.ID, at time.Time) error {
	e := m.externals[id]
	e.UpdatedAt = at
	m.externals[id] = e
	return nil
}

// setExternalState stores a state as if the invoice had it before.
func (m *memStore) setExternalState(id resource.ID, state string) {
	e := m.externals[id]
	e.State = state
	m.externals[id] = e
}

// fakeBank keeps what transactions have open and links them through the
// service, as banking does.
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
	id, err := f.svc.RegisterBankPayment(ctx, admin, user, invoiceType, invoice, mutation, bankAccount, f.day, open, price)
	if err != nil {
		return 0, err
	}
	p, _ := f.svc.store.Payment(ctx, admin, id)
	f.open[mutation] -= p.Price
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
	f.open[mutation] += p.Price
	return nil
}

func (f *fakeBank) AccountLedger(_ context.Context, _, account, _ resource.ID) (resource.ID, error) {
	if account != bankAccount {
		return 0, errors.New("no such account")
	}
	return bankLedger, nil
}

type fakeNotifier struct{ actions []string }

func (f *fakeNotifier) Notify(_ context.Context, _ resource.ID, action string, _ Invoice) {
	f.actions = append(f.actions, action)
}

func (f *fakeNotifier) NotifyRecurring(_ context.Context, _ resource.ID, action string, _ Recurring) {
	f.actions = append(f.actions, action)
}

type fakeRenderer struct{}

func (fakeRenderer) PDF(doc Document) ([]byte, error) {
	return []byte("%PDF " + doc.Invoice.InvoiceID + " " + strconv.FormatInt(int64(doc.Totals.InclTax), 10)), nil
}
func (fakeRenderer) PackingSlip(doc Document) ([]byte, error) { return []byte("%PDF slip"), nil }
func (fakeRenderer) UBL(doc Document) ([]byte, error) {
	return []byte("<Invoice>" + doc.Invoice.InvoiceID + "</Invoice>"), nil
}

type fakeMailer struct {
	sent []Mail
	fail bool
}

func (f *fakeMailer) SendDocument(_ context.Context, m Mail) error {
	if f.fail {
		return errors.New("smtp down")
	}
	f.sent = append(f.sent, m)
	return nil
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
	svc      *Service
	store    *memStore
	ledger   *fakeLedger
	mailer   *fakeMailer
	bank     *fakeBank
	notifier *fakeNotifier
	clock    *time.Time
}

func newHarness(t *testing.T) harness {
	t.Helper()
	store := &memStore{invoices: map[resource.ID]Invoice{}, entries: map[resource.ID][]journal.Entry{}, paid: map[resource.ID][]journal.Entry{}, locked: map[resource.ID]bool{},
		externals: map[resource.ID]ExternalInvoice{}, externalEntries: map[resource.ID][]journal.Entry{}, recurring: map[resource.ID]Recurring{}}
	ledger := &fakeLedger{}
	mailer := &fakeMailer{}
	ids := resource.NewGenerator(1)
	at := now
	clock := func() time.Time { return at }
	bank := &fakeBank{open: map[resource.ID]money.Amount{}, day: now}
	notifier := &fakeNotifier{}
	svc := NewService(Deps{
		Administrations: fakeAdmins{}, Contacts: fakeContacts{}, Invoicing: &fakeInvoicing{}, Ledger: ledger,
		Notes: notes.NewService(&memNotes{}, ids, clock), Attachments: attachments.NewService(&memAttachments{}, noFiles{}, ids, clock),
		Store: store, Renderer: fakeRenderer{}, Mailer: mailer, Bank: bank, Notifier: notifier, IDs: ids, Now: clock,
	})
	bank.svc = svc
	return harness{svc: svc, store: store, ledger: ledger, mailer: mailer, bank: bank, notifier: notifier, clock: &at}
}
