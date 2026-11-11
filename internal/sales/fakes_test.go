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
		{ID: flow, Type: invoicing.InvoiceWorkflow, Default: true, Active: true, Currency: "EUR", Language: "nl", Steps: []invoicing.WorkflowStep{{DueInterval: 14}}},
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
	}
	return 0, errors.New("no such role")
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
	sequence int
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
	return err == nil, nil
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
	svc    *Service
	store  *memStore
	ledger *fakeLedger
	mailer *fakeMailer
}

func newHarness(t *testing.T) harness {
	t.Helper()
	store := &memStore{invoices: map[resource.ID]Invoice{}, entries: map[resource.ID][]journal.Entry{}}
	ledger := &fakeLedger{}
	mailer := &fakeMailer{}
	ids := resource.NewGenerator(1)
	clock := func() time.Time { return now }
	svc := NewService(Deps{
		Administrations: fakeAdmins{}, Contacts: fakeContacts{}, Invoicing: &fakeInvoicing{}, Ledger: ledger,
		Notes: notes.NewService(&memNotes{}, ids, clock), Attachments: attachments.NewService(&memAttachments{}, noFiles{}, ids, clock),
		Store: store, Renderer: fakeRenderer{}, Mailer: mailer, IDs: ids, Now: clock,
	})
	return harness{svc: svc, store: store, ledger: ledger, mailer: mailer}
}
