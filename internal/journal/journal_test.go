package journal

import (
	"context"
	"errors"
	"io"
	"slices"
	"testing"
	"time"

	"github.com/adirkuhn/moneyharpy/internal/administration"
	"github.com/adirkuhn/moneyharpy/internal/attachments"
	"github.com/adirkuhn/moneyharpy/internal/money"
	"github.com/adirkuhn/moneyharpy/internal/notes"
	"github.com/adirkuhn/moneyharpy/internal/projects"
	"github.com/adirkuhn/moneyharpy/internal/resource"
)

const (
	adminID         resource.ID = 100
	owner, stranger resource.ID = 1, 2
	bank            resource.ID = 700
	rent            resource.ID = 701
	debtors         resource.ID = 702
	closed          resource.ID = 703
	customer        resource.ID = 900
	falcon          resource.ID = 950
	shelved         resource.ID = 951
)

var now = time.Date(2026, 11, 11, 9, 0, 0, 0, time.UTC)

type fakeAdmins struct{}

func (fakeAdmins) Get(_ context.Context, id, user resource.ID) (administration.Listed, error) {
	if id != adminID || user != owner {
		return administration.Listed{}, administration.ErrNotFound
	}
	return administration.Listed{Administration: administration.Administration{ID: adminID, TimeZone: "Europe/Amsterdam"}}, nil
}

type fakeLedger struct{ locked string }

func (f *fakeLedger) Bookable(_ context.Context, _, _, id resource.ID, _ string) (string, error) {
	switch id {
	case bank, rent:
		return "", nil
	case debtors:
		return "is settled by invoices and cannot be booked in a journal entry", nil
	case closed:
		return "is a deactivated category", nil
	}
	return "is not a category of this administration", nil
}

func (f *fakeLedger) LockedUntil(context.Context, resource.ID, resource.ID) (string, error) {
	return f.locked, nil
}

type fakeContacts struct{}

func (fakeContacts) ContactExists(_ context.Context, _, id, _ resource.ID) error {
	if id != customer {
		return errors.New("contact not found")
	}
	return nil
}

// memJournal keeps documents and the entries they post.
type memJournal struct {
	docs    map[resource.ID]Document
	entries []Entry
}

func (m *memJournal) post(d Document) {
	m.entries = slices.DeleteFunc(m.entries, func(e Entry) bool { return e.DocumentID == d.ID })
	m.entries = append(m.entries, d.Entries()...)
}

func (m *memJournal) Documents(_ context.Context, _ resource.ID, q DocumentQuery) ([]Document, int, error) {
	var out []Document
	for _, d := range m.docs {
		if q.Period == nil || q.Period.Contains(d.Date) {
			out = append(out, d)
		}
	}
	slices.SortFunc(out, func(a, b Document) int { return b.Date.Compare(a.Date) })
	return out, len(out), nil
}

func (m *memJournal) Document(_ context.Context, _, id resource.ID) (Document, error) {
	d, ok := m.docs[id]
	if !ok {
		return Document{}, ErrDocumentNotFound
	}
	return d, nil
}

func (m *memJournal) DocumentsByID(_ context.Context, _ resource.ID, ids []resource.ID) ([]Document, error) {
	var out []Document
	for _, id := range ids {
		if d, ok := m.docs[id]; ok {
			out = append(out, d)
		}
	}
	return out, nil
}

func (m *memJournal) CreateDocument(_ context.Context, d Document) error {
	m.docs[d.ID] = d
	m.post(d)
	return nil
}

func (m *memJournal) UpdateDocument(ctx context.Context, d Document) error {
	return m.CreateDocument(ctx, d)
}

func (m *memJournal) DeleteDocument(_ context.Context, _, id resource.ID) error {
	delete(m.docs, id)
	m.entries = slices.DeleteFunc(m.entries, func(e Entry) bool { return e.DocumentID == id })
	return nil
}

func (m *memJournal) TouchDocument(_ context.Context, _, id resource.ID, at time.Time) error {
	d := m.docs[id]
	d.UpdatedAt = at
	m.docs[id] = d
	return nil
}

func (m *memJournal) Entries(_ context.Context, _ resource.ID, q EntryQuery) ([]Entry, int, error) {
	var out []Entry
	for _, e := range m.entries {
		if q.Period.Contains(e.Date) && (q.LedgerAccountID == nil || e.LedgerAccountID == *q.LedgerAccountID) &&
			(q.DocumentID == nil || (e.DocumentType == q.DocumentType && e.DocumentID == *q.DocumentID)) {
			out = append(out, e)
		}
	}
	return out, len(out), nil
}

func (m *memJournal) Sums(_ context.Context, _ resource.ID, p money.Period) ([]AccountSum, error) {
	byAccount := map[resource.ID]*AccountSum{}
	for _, e := range m.entries {
		s := byAccount[e.LedgerAccountID]
		if s == nil {
			s = &AccountSum{LedgerAccountID: e.LedgerAccountID}
			byAccount[e.LedgerAccountID] = s
		}
		switch {
		case e.Date.Before(p.From):
			s.Start += e.Amount
		case !e.Date.After(p.To):
			s.Debit += e.Debit()
			s.Credit += e.Credit()
		}
	}
	var out []AccountSum
	for _, s := range byAccount {
		s.Final = s.Start + s.Debit - s.Credit
		out = append(out, *s)
	}
	slices.SortFunc(out, func(a, b AccountSum) int { return int(a.LedgerAccountID - b.LedgerAccountID) })
	return out, nil
}

type memNotes struct{ list []notes.Note }

func (m *memNotes) CreateNote(_ context.Context, n notes.Note) error {
	m.list = append(m.list, n)
	return nil
}
func (m *memNotes) DeleteNote(_ context.Context, _, entity, id resource.ID) error {
	m.list = slices.DeleteFunc(m.list, func(n notes.Note) bool { return n.ID == id })
	return nil
}
func (m *memNotes) NotesOf(_ context.Context, _ resource.ID, entityType string, entities []resource.ID) ([]notes.Note, error) {
	var out []notes.Note
	for _, n := range m.list {
		if n.EntityType == entityType && slices.Contains(entities, n.EntityID) {
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
func (m *memAttachments) AttachmentsOf(_ context.Context, _ resource.ID, entityType string, entities []resource.ID) ([]attachments.Attachment, error) {
	var out []attachments.Attachment
	for _, a := range m.list {
		if a.EntityType == entityType && slices.Contains(entities, a.EntityID) {
			out = append(out, a)
		}
	}
	return out, nil
}

func (m *memAttachments) UnhashedAttachments(context.Context, []string, resource.ID, int) ([]attachments.Attachment, error) {
	return nil, nil
}

func (m *memAttachments) SetAttachmentHash(context.Context, resource.ID, resource.ID, string) error {
	return nil
}

type fakeProjects struct{}

func (fakeProjects) Project(_ context.Context, _, id, _ resource.ID) (projects.Project, error) {
	switch id {
	case falcon:
		return projects.Project{ID: id, Name: "Falcon", State: projects.Active}, nil
	case shelved:
		return projects.Project{ID: id, Name: "Shelved", State: projects.Archived}, nil
	}
	return projects.Project{}, projects.ErrNotFound
}

type noFiles struct{}

func (noFiles) Put(context.Context, string, string, []byte) error { return nil }
func (noFiles) Open(context.Context, string) (io.ReadCloser, string, error) {
	return nil, "", errors.New("no files")
}
func (noFiles) Delete(context.Context, string) error { return nil }

func newService() (*Service, *memJournal, *fakeLedger) {
	store := &memJournal{docs: map[resource.ID]Document{}}
	ledger := &fakeLedger{}
	ids := resource.NewGenerator(1)
	clock := func() time.Time { return now }
	return NewService(fakeAdmins{}, ledger, fakeContacts{}, fakeProjects{}, store, store, notes.NewService(&memNotes{}, ids, clock),
		attachments.NewService(&memAttachments{}, noFiles{}, ids, clock), ids, clock), store, ledger
}

func ptr[T any](v T) *T { return &v }

func rows(pairs ...any) []RowInput {
	var out []RowInput
	for i := 0; i < len(pairs); i += 3 {
		in := RowInput{LedgerAccountID: ptr(pairs[i].(resource.ID))}
		if d := pairs[i+1].(string); d != "" {
			in.Debit = &d
		}
		if c := pairs[i+2].(string); c != "" {
			in.Credit = &c
		}
		out = append(out, in)
	}
	return out
}

func fieldsOf(t *testing.T, err error) map[string][]string {
	t.Helper()
	var verr *resource.ValidationError
	if !errors.As(err, &verr) {
		t.Fatalf("want a validation error, got %v", err)
	}
	return verr.Fields
}

func TestJournalEntryPostsBalancedRows(t *testing.T) {
	s, store, _ := newService()
	ctx := context.Background()
	d, err := s.CreateDocument(ctx, adminID, owner, DocumentChanges{Reference: ptr("Correction"), Date: ptr("2026-11-01"), Rows: rows(rent, "100.00", "", bank, "", "100,00")})
	if err != nil {
		t.Fatal(err)
	}
	if len(store.entries) != 2 || store.entries[0].Amount != 10000 || store.entries[1].Amount != -10000 || store.entries[0].DocumentType != GeneralJournalDocument {
		t.Fatalf("entries = %+v", store.entries)
	}
	if d.Ref().URI() != "/100/documents/general_journal_documents/"+d.ID.String() {
		t.Errorf("uri = %s", d.Ref().URI())
	}
}

func TestJournalEntryRowsBookOnProjects(t *testing.T) {
	s, store, _ := newService()
	ctx := context.Background()
	in := rows(rent, "100", "", bank, "", "100")
	in[0].ProjectID = ptr(falcon)
	d, err := s.CreateDocument(ctx, adminID, owner, DocumentChanges{Reference: ptr("Falcon costs"), Date: ptr("2026-11-01"), Rows: in})
	if err != nil {
		t.Fatal(err)
	}
	if d.Rows[0].ProjectID == nil || *d.Rows[0].ProjectID != falcon || store.entries[0].ProjectID == nil || *store.entries[0].ProjectID != falcon {
		t.Errorf("rows = %+v, entries = %+v", d.Rows, store.entries)
	}
	for id, problem := range map[resource.ID]string{shelved: "is archived", 5: "does not exist"} {
		in[0].ProjectID = ptr(id)
		_, err := s.CreateDocument(ctx, adminID, owner, DocumentChanges{Reference: ptr("x"), Date: ptr("2026-11-01"), Rows: in})
		if got := fieldsOf(t, err)[rowsField+".0.project_id"]; len(got) != 1 || got[0] != problem {
			t.Errorf("project %d: %v", id, got)
		}
	}
	cleared, err := s.UpdateDocument(ctx, adminID, d.ID, owner, DocumentChanges{Rows: []RowInput{{ID: &d.Rows[0].ID, ProjectID: ptr(resource.ID(0))}}})
	if err != nil || cleared.Rows[0].ProjectID != nil {
		t.Errorf("cleared = %+v, %v", cleared.Rows, err)
	}
}

func TestJournalEntryMustBalance(t *testing.T) {
	s, _, _ := newService()
	_, err := s.CreateDocument(context.Background(), adminID, owner, DocumentChanges{Reference: ptr("x"), Date: ptr("2026-11-01"), Rows: rows(rent, "100", "", bank, "", "90")})
	if f := fieldsOf(t, err); len(f["base"]) != 1 {
		t.Errorf("errors = %v", f)
	}
}

func TestJournalEntryChecksRows(t *testing.T) {
	s, _, _ := newService()
	in := rows(debtors, "10", "", closed, "", "10", resource.ID(1), "5", "5")
	in = append(in, RowInput{LedgerAccountID: ptr(rent), Debit: ptr("-3")}, RowInput{LedgerAccountID: ptr(rent), Credit: ptr("1"), ContactID: ptr(resource.ID(5))})
	_, err := s.CreateDocument(context.Background(), adminID, owner, DocumentChanges{Rows: in})
	f := fieldsOf(t, err)
	for _, key := range []string{"reference", "date", rowsField + ".0.ledger_account_id", rowsField + ".1.ledger_account_id", rowsField + ".2", rowsField + ".3.debit", rowsField + ".4.contact_id"} {
		if len(f[key]) == 0 {
			t.Errorf("no error for %s in %v", key, f)
		}
	}
}

func TestJournalEntryRespectsTheLockedPeriod(t *testing.T) {
	s, _, ledger := newService()
	ctx := context.Background()
	d, err := s.CreateDocument(ctx, adminID, owner, DocumentChanges{Reference: ptr("x"), Date: ptr("2026-10-15"), Rows: rows(rent, "1", "", bank, "", "1")})
	if err != nil {
		t.Fatal(err)
	}
	ledger.locked = "2026-10-31"
	if _, err := s.CreateDocument(ctx, adminID, owner, DocumentChanges{Reference: ptr("y"), Date: ptr("2026-10-31"), Rows: rows(rent, "1", "", bank, "", "1")}); fieldsOf(t, err)["date"] == nil {
		t.Error("created in the locked period")
	}
	if _, err := s.UpdateDocument(ctx, adminID, d.ID, owner, DocumentChanges{Date: ptr("2026-11-05")}); fieldsOf(t, err)["date"] == nil {
		t.Error("moved out of the locked period")
	}
	if err := s.DeleteDocument(ctx, adminID, d.ID, owner); fieldsOf(t, err)["date"] == nil {
		t.Error("deleted from the locked period")
	}
}

func TestJournalEntryRowsChange(t *testing.T) {
	s, store, _ := newService()
	ctx := context.Background()
	d, _ := s.CreateDocument(ctx, adminID, owner, DocumentChanges{Reference: ptr("x"), Date: ptr("2026-11-01"), Rows: rows(rent, "50", "", bank, "", "50")})
	first, second := d.Rows[0].ID, d.Rows[1].ID
	d, err := s.UpdateDocument(ctx, adminID, d.ID, owner, DocumentChanges{Rows: []RowInput{
		{ID: &first, Debit: ptr("80")},
		{ID: &second, Destroy: true},
		{LedgerAccountID: ptr(bank), Credit: ptr("80"), ContactID: ptr(customer), Description: ptr(" Settle ")},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if len(d.Rows) != 2 || d.Rows[0].Debit != 8000 || d.Rows[1].ContactID == nil || d.Rows[1].Description != "Settle" {
		t.Errorf("rows = %+v", d.Rows)
	}
	if len(store.entries) != 2 || Balance(store.entries) != nil {
		t.Errorf("entries = %+v", store.entries)
	}
	if err := s.DeleteDocument(ctx, adminID, d.ID, owner); err != nil || len(store.entries) != 0 {
		t.Errorf("delete = %v, entries %d", err, len(store.entries))
	}
}

func TestStrangersCannotReadTheJournal(t *testing.T) {
	s, _, _ := newService()
	if _, err := s.GeneralLedger(context.Background(), adminID, stranger, "", ""); !errors.Is(err, administration.ErrNotFound) {
		t.Errorf("general ledger = %v", err)
	}
	if _, err := s.Document(context.Background(), adminID, 1, owner); !errors.Is(err, ErrDocumentNotFound) {
		t.Errorf("missing = %v", err)
	}
}

func TestGeneralLedgerSplitsPeriods(t *testing.T) {
	s, _, _ := newService()
	ctx := context.Background()
	for _, day := range []string{"2026-09-10", "2026-10-10", "2026-11-10"} {
		if _, err := s.CreateDocument(ctx, adminID, owner, DocumentChanges{Reference: ptr(day), Date: ptr(day), Rows: rows(rent, "10", "", bank, "", "10")}); err != nil {
			t.Fatal(err)
		}
	}
	report, err := s.GeneralLedger(ctx, adminID, owner, "202610..202611", "month")
	if err != nil {
		t.Fatal(err)
	}
	var rentSum AccountSum
	for _, a := range report.Sums {
		if a.LedgerAccountID == rent {
			rentSum = a
		}
	}
	if rentSum.Start != 1000 || rentSum.Debit != 2000 || rentSum.Final != 3000 {
		t.Errorf("rent = %+v", rentSum)
	}
	if len(report.Periods) != 2 || report.Periods[0].Period.To.Format(time.DateOnly) != "2026-10-31" {
		t.Errorf("periods = %+v", report.Periods)
	}
	if _, err := s.GeneralLedger(ctx, adminID, owner, "20261005..20261020", ""); fieldsOf(t, err)["period"] == nil {
		t.Error("partial months accepted")
	}
	if _, err := s.GeneralLedger(ctx, adminID, owner, "202611", "week"); fieldsOf(t, err)["granularity"] == nil {
		t.Error("unknown granularity accepted")
	}
}

func TestEntriesReportNeedsASubject(t *testing.T) {
	s, _, _ := newService()
	ctx := context.Background()
	if _, _, err := s.Entries(ctx, adminID, owner, "", EntryQuery{}); fieldsOf(t, err)["base"] == nil {
		t.Error("entries without a subject")
	}
	if _, _, err := s.Entries(ctx, adminID, owner, "", EntryQuery{LedgerAccountID: ptr(rent), AccountType: "revenue"}); fieldsOf(t, err)["account_type"] == nil {
		t.Error("account type without contact or project")
	}
	if _, _, err := s.Entries(ctx, adminID, owner, "this_year", EntryQuery{LedgerAccountID: ptr(rent)}); err != nil {
		t.Error(err)
	}
}

func TestEntriesOfADocumentCoverEveryDate(t *testing.T) {
	s, store, _ := newService()
	ctx := context.Background()
	old := time.Date(2019, 3, 1, 0, 0, 0, 0, time.UTC)
	store.entries = append(store.entries,
		Entry{ID: 1, DocumentType: SalesInvoice, DocumentID: 42, Date: old, LedgerAccountID: debtors, Amount: 1210},
		Entry{ID: 2, DocumentType: SalesInvoice, DocumentID: 42, Date: old, LedgerAccountID: rent, Amount: -1210},
		Entry{ID: 3, DocumentType: SalesInvoice, DocumentID: 43, Date: old, LedgerAccountID: debtors, Amount: 500},
	)
	// A document is a subject of its own, and without a period every date counts.
	list, total, err := s.Entries(ctx, adminID, owner, "", EntryQuery{DocumentType: SalesInvoice, DocumentID: ptr(resource.ID(42))})
	if err != nil || total != 2 || len(list) != 2 || list[0].ID != 1 {
		t.Fatalf("entries = %+v (%d), %v", list, total, err)
	}
	// A period still narrows it down.
	if list, _, err := s.Entries(ctx, adminID, owner, "this_year", EntryQuery{DocumentType: SalesInvoice, DocumentID: ptr(resource.ID(42))}); err != nil || len(list) != 0 {
		t.Errorf("this year = %+v, %v", list, err)
	}
	if _, _, err := s.Entries(ctx, adminID, owner, "", EntryQuery{DocumentID: ptr(resource.ID(42))}); fieldsOf(t, err)["document_type"] == nil {
		t.Error("document id without its type")
	}
	if _, _, err := s.Entries(ctx, adminID, owner, "", EntryQuery{DocumentType: SalesInvoice}); fieldsOf(t, err)["document_id"] == nil {
		t.Error("document type without its id")
	}
}

func TestParseDocumentFilter(t *testing.T) {
	q, err := ParseDocumentFilter("", now)
	if err != nil || q.Period == nil || q.Period.From.Format(time.DateOnly) != "2026-01-01" {
		t.Errorf("default = %+v, %v", q, err)
	}
	q, err = ParseDocumentFilter("period:202610..202610,reference:Correction,contact_id:900,attachment:with,state:all", now)
	if err != nil || q.Reference != "Correction" || *q.ContactID != customer || q.Attachment != "with" {
		t.Errorf("filter = %+v, %v", q, err)
	}
	if _, err := ParseDocumentFilter("colour:red", now); err == nil {
		t.Error("unknown key accepted")
	}
}

func TestNotesOnJournalEntries(t *testing.T) {
	s, _, _ := newService()
	ctx := context.Background()
	d, _ := s.CreateDocument(ctx, adminID, owner, DocumentChanges{Reference: ptr("x"), Date: ptr("2026-11-01"), Rows: rows(rent, "1", "", bank, "", "1")})
	n, err := s.AddDocumentNote(ctx, adminID, d.ID, owner, notes.Input{Note: "Checked"})
	if err != nil {
		t.Fatal(err)
	}
	got, _ := s.Document(ctx, adminID, d.ID, owner)
	if len(got.Notes) != 1 || got.Notes[0].EntityType != GeneralJournalDocument {
		t.Errorf("notes = %+v", got.Notes)
	}
	if err := s.DeleteDocumentNote(ctx, adminID, d.ID, n.ID, owner); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteDocumentNote(ctx, adminID, d.ID, n.ID, owner); !IsNotFound(err) {
		t.Errorf("second delete = %v", err)
	}
}
