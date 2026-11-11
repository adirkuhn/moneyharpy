package timeentries

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/adirkuhn/moneyharpy/internal/administration"
	"github.com/adirkuhn/moneyharpy/internal/notes"
	"github.com/adirkuhn/moneyharpy/internal/resource"
)

const (
	adminID  = resource.ID(1)
	owner    = resource.ID(2)
	employee = resource.ID(3)
	customer = resource.ID(10)
	falcon   = resource.ID(11)
	invoice  = resource.ID(12)
)

type fakeAdmins struct{}

func (fakeAdmins) Get(_ context.Context, id, user resource.ID) (administration.Listed, error) {
	if id != adminID || (user != owner && user != employee) {
		return administration.Listed{}, administration.ErrNotFound
	}
	return administration.Listed{Administration: administration.Administration{ID: id, TimeZone: "Europe/Amsterdam"}}, nil
}

type memStore struct{ entries map[resource.ID]TimeEntry }

func (m *memStore) Entries(_ context.Context, _ resource.ID, q Query) ([]TimeEntry, int, error) {
	var out []TimeEntry
	for _, e := range m.entries {
		if q.Matches(e) {
			out = append(out, e)
		}
	}
	slices.SortFunc(out, func(a, b TimeEntry) int { return b.StartedAt.Compare(a.StartedAt) })
	total := len(out)
	from := min((q.Page-1)*q.PerPage, total)
	return out[from:min(from+q.PerPage, total)], total, nil
}

func (m *memStore) Entry(_ context.Context, _, id resource.ID) (TimeEntry, error) {
	e, ok := m.entries[id]
	if !ok {
		return TimeEntry{}, ErrNotFound
	}
	return e, nil
}

func (m *memStore) RunningOf(_ context.Context, _, user resource.ID) ([]TimeEntry, error) {
	var out []TimeEntry
	for _, e := range m.entries {
		if e.UserID == user && e.Running() {
			out = append(out, e)
		}
	}
	return out, nil
}

func (m *memStore) OfInvoices(_ context.Context, _ resource.ID, invoices []resource.ID) ([]TimeEntry, error) {
	var out []TimeEntry
	for _, e := range m.entries {
		if e.SalesInvoiceID != nil && slices.Contains(invoices, *e.SalesInvoiceID) {
			out = append(out, e)
		}
	}
	return out, nil
}

func (m *memStore) SaveEntry(_ context.Context, e TimeEntry, _ bool) error {
	e.Notes = nil
	m.entries[e.ID] = e
	return nil
}

func (m *memStore) DeleteEntry(_ context.Context, _, id resource.ID) error {
	delete(m.entries, id)
	return nil
}

func (m *memStore) Users(_ context.Context, _ resource.ID, ids []resource.ID) (map[resource.ID]User, error) {
	out := map[resource.ID]User{}
	for _, id := range ids {
		if id == owner || id == employee {
			out[id] = User{ID: id, Name: "Member " + id.String()}
		}
	}
	return out, nil
}

func (m *memStore) ContactExists(_ context.Context, _, id resource.ID) (bool, error) {
	return id == customer, nil
}

func (m *memStore) ProjectExists(_ context.Context, _, id resource.ID) (bool, error) {
	return id == falcon, nil
}

func (m *memStore) InvoiceExists(_ context.Context, _, id resource.ID) (bool, error) {
	return id == invoice, nil
}

type memNotes struct{ list []notes.Note }

func (m *memNotes) Add(_ context.Context, admin resource.ID, entityType string, entity, user resource.ID, in notes.Input) (notes.Note, error) {
	n := notes.Note{ID: resource.ID(100 + len(m.list)), AdministrationID: admin, EntityType: entityType, EntityID: entity, UserID: user, Note: in.Note}
	m.list = append(m.list, n)
	return n, nil
}

func (m *memNotes) Delete(_ context.Context, _ resource.ID, _ string, entity, id resource.ID) error {
	for i, n := range m.list {
		if n.ID == id && n.EntityID == entity {
			m.list = slices.Delete(m.list, i, i+1)
			return nil
		}
	}
	return notes.ErrNotFound
}

func (m *memNotes) Of(_ context.Context, _ resource.ID, _ string, entities []resource.ID) (map[resource.ID][]notes.Note, error) {
	out := map[resource.ID][]notes.Note{}
	for _, n := range m.list {
		if slices.Contains(entities, n.EntityID) {
			out[n.EntityID] = append(out[n.EntityID], n)
		}
	}
	return out, nil
}

type fixture struct {
	svc   *Service
	store *memStore
	now   *time.Time
}

// 2026-11-11 10:30:20 in Amsterdam.
var start = time.Date(2026, 11, 11, 9, 30, 20, 0, time.UTC)

func newFixture() fixture {
	now := start
	store := &memStore{entries: map[resource.ID]TimeEntry{}}
	svc := NewService(fakeAdmins{}, store, &memNotes{}, resource.NewGenerator(1), func() time.Time { return now })
	return fixture{svc: svc, store: store, now: &now}
}

var (
	boss = Actor{ID: owner, AllUsers: true}
	own  = Actor{ID: employee}
)

func at(s string) *time.Time {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		panic(err)
	}
	return &t
}

func ptr[T any](v T) *T { return &v }

func fieldsOf(t *testing.T, err error) map[string][]string {
	t.Helper()
	var verr *resource.ValidationError
	if !errors.As(err, &verr) {
		t.Fatalf("err = %v, want a validation error", err)
	}
	return verr.Fields
}

func TestCreateRoundsToMinutesAndKeepsAMinute(t *testing.T) {
	f := newFixture()
	ctx := context.Background()
	e, err := f.svc.Create(ctx, adminID, boss, Input{
		UserID: ptr(employee), StartedAt: at("2026-11-10T09:00:42Z"), EndedAt: at("2026-11-10T09:00:50Z"), Description: ptr(" Designing "),
		ContactID: ptr(customer), ProjectID: ptr(falcon), PausedDuration: ptr(0),
	})
	if err != nil {
		t.Fatal(err)
	}
	if !e.StartedAt.Equal(*at("2026-11-10T09:00:00Z")) || !e.EndedAt.Equal(*at("2026-11-10T09:01:00Z")) || e.Description != "Designing" || !e.Billable ||
		*e.ContactID != customer || *e.ProjectID != falcon || e.Notes == nil {
		t.Errorf("created = %+v", e)
	}
	if e.Ref().URI() != "/1/time_entries/"+e.ID.String() {
		t.Errorf("uri = %s", e.Ref().URI())
	}
	if d := e.Duration(start); d != time.Minute {
		t.Errorf("duration = %s", d)
	}
}

func TestCreateValidates(t *testing.T) {
	f := newFixture()
	ctx := context.Background()
	_, err := f.svc.Create(ctx, adminID, boss, Input{})
	if fields := fieldsOf(t, err); len(fields["user_id"]) == 0 || len(fields["started_at"]) == 0 || len(fields["description"]) == 0 {
		t.Errorf("fields = %v", fields)
	}
	_, err = f.svc.Create(ctx, adminID, boss, Input{
		UserID: ptr(resource.ID(99)), StartedAt: at("2026-11-10T10:00:00Z"), EndedAt: at("2026-11-10T09:00:00Z"), Description: ptr(""),
		ContactID: ptr(resource.ID(98)), ProjectID: ptr(resource.ID(97)), SalesInvoiceID: ptr(resource.ID(96)), PausedDuration: ptr(-1),
	})
	fields := fieldsOf(t, err)
	for _, k := range []string{"user_id", "contact_id", "project_id", "sales_invoice_id", "description", "ended_at", "paused_duration"} {
		if len(fields[k]) == 0 {
			t.Errorf("no problem with %s: %v", k, fields)
		}
	}
	_, err = f.svc.Create(ctx, adminID, boss, Input{UserID: ptr(owner), StartedAt: at("2026-11-10T09:00:00Z"), EndedAt: at("2026-11-10T10:00:00Z"),
		Description: ptr("x"), PausedDuration: ptr(3600)})
	if len(fieldsOf(t, err)["paused_duration"]) == 0 {
		t.Error("a pause as long as the entry is accepted")
	}
	if _, err := f.svc.Create(ctx, adminID, own, Input{UserID: ptr(owner), StartedAt: at("2026-11-10T09:00:00Z"), Description: ptr("x")}); !errors.Is(err, ErrForbidden) {
		t.Errorf("entry for another user = %v", err)
	}
	if _, err := f.svc.Create(ctx, resource.ID(5), boss, Input{}); !errors.Is(err, administration.ErrNotFound) {
		t.Errorf("other administration = %v", err)
	}
}

func TestTimersStopResumeAndOneRunsAtATime(t *testing.T) {
	f := newFixture()
	ctx := context.Background()
	first, err := f.svc.Create(ctx, adminID, own, Input{UserID: ptr(employee), StartedAt: at("2026-11-11T08:00:00Z"), Description: ptr("Morning")})
	if err != nil || !first.Running() {
		t.Fatalf("first = %+v, %v", first, err)
	}
	second, err := f.svc.Create(ctx, adminID, own, Input{UserID: ptr(employee), StartedAt: at("2026-11-11T09:00:00Z"), Description: ptr("Later")})
	if err != nil {
		t.Fatal(err)
	}
	if stopped := f.store.entries[first.ID]; stopped.Running() || !stopped.EndedAt.Equal(*at("2026-11-11T09:30:00Z")) {
		t.Errorf("first after second started = %+v", stopped)
	}
	*f.now = start.Add(10 * time.Minute)
	resumed, err := f.svc.Resume(ctx, adminID, first.ID, own)
	if err != nil || !resumed.Running() || resumed.PausedDuration != 600 {
		t.Fatalf("resumed = %+v, %v", resumed, err)
	}
	if again, err := f.svc.Resume(ctx, adminID, first.ID, own); err != nil || again.PausedDuration != 600 {
		t.Errorf("resume running = %+v, %v", again, err)
	}
	if f.store.entries[second.ID].Running() {
		t.Error("second still runs after the first resumed")
	}
	stopped, err := f.svc.Stop(ctx, adminID, first.ID, own)
	if err != nil || stopped.Running() || !stopped.EndedAt.Equal(*at("2026-11-11T09:40:00Z")) {
		t.Fatalf("stopped = %+v, %v", stopped, err)
	}
	if again, err := f.svc.Stop(ctx, adminID, first.ID, own); err != nil || !again.EndedAt.Equal(*stopped.EndedAt) {
		t.Errorf("stop stopped = %+v, %v", again, err)
	}
	old, _ := f.svc.Create(ctx, adminID, own, Input{UserID: ptr(employee), StartedAt: at("2026-11-10T08:00:00Z"), EndedAt: at("2026-11-10T09:00:00Z"), Description: ptr("Yesterday")})
	if _, err := f.svc.Resume(ctx, adminID, old.ID, own); len(fieldsOf(t, err)["base"]) == 0 {
		t.Errorf("resume yesterday = %v", err)
	}
}

func TestOtherUsersEntriesNeedInvoiceAccess(t *testing.T) {
	f := newFixture()
	ctx := context.Background()
	mine, _ := f.svc.Create(ctx, adminID, boss, Input{UserID: ptr(owner), StartedAt: at("2026-11-10T08:00:00Z"), EndedAt: at("2026-11-10T09:00:00Z"), Description: ptr("Boss")})
	theirs, _ := f.svc.Create(ctx, adminID, own, Input{UserID: ptr(employee), StartedAt: at("2026-11-10T10:00:00Z"), EndedAt: at("2026-11-10T11:00:00Z"), Description: ptr("Mine")})
	if _, err := f.svc.Entry(ctx, adminID, mine.ID, own); !errors.Is(err, ErrForbidden) {
		t.Errorf("Entry() of another = %v", err)
	}
	if _, err := f.svc.Entry(ctx, adminID, theirs.ID, boss); err != nil {
		t.Errorf("Entry() by the owner = %v", err)
	}
	list, total, err := f.svc.List(ctx, adminID, own, Query{})
	if err != nil || total != 1 || list[0].ID != theirs.ID {
		t.Errorf("own list = %+v (%d), %v", list, total, err)
	}
	if list, _, _ := f.svc.List(ctx, adminID, own, Query{UserID: ptr(owner)}); len(list) != 0 {
		t.Errorf("asking for another's = %+v", list)
	}
	if _, total, _ := f.svc.List(ctx, adminID, boss, Query{}); total != 2 {
		t.Errorf("all = %d", total)
	}
	if _, err := f.svc.Update(ctx, adminID, theirs.ID, own, Input{UserID: ptr(owner)}); !errors.Is(err, ErrForbidden) {
		t.Errorf("handing an entry to another = %v", err)
	}
	if err := f.svc.Delete(ctx, adminID, mine.ID, own); !errors.Is(err, ErrForbidden) {
		t.Errorf("Delete() another's = %v", err)
	}
}

func TestBilledEntriesAreFixed(t *testing.T) {
	f := newFixture()
	ctx := context.Background()
	e, _ := f.svc.Create(ctx, adminID, boss, Input{UserID: ptr(owner), StartedAt: at("2026-11-10T08:00:00Z"), EndedAt: at("2026-11-10T09:00:00Z"), Description: ptr("Work")})
	running, _ := f.svc.Create(ctx, adminID, boss, Input{UserID: ptr(owner), StartedAt: at("2026-11-11T08:00:00Z"), Description: ptr("Now")})
	if err := f.svc.CheckBillable(ctx, adminID, boss, []resource.ID{e.ID, running.ID, 404}, 0); len(fieldsOf(t, err)["time_entry_ids"]) != 2 {
		t.Errorf("CheckBillable() = %v", err)
	}
	billed, err := f.svc.Bill(ctx, adminID, invoice, boss, []resource.ID{e.ID})
	if err != nil || len(billed) != 1 || *billed[0].SalesInvoiceID != invoice {
		t.Fatalf("Bill() = %+v, %v", billed, err)
	}
	if err := f.svc.CheckBillable(ctx, adminID, boss, []resource.ID{e.ID}, invoice); err != nil {
		t.Errorf("same invoice again = %v", err)
	}
	if err := f.svc.CheckBillable(ctx, adminID, boss, []resource.ID{e.ID}, 77); err == nil {
		t.Error("billed twice")
	}
	byInvoice, err := f.svc.OfInvoices(ctx, adminID, []resource.ID{invoice})
	if err != nil || len(byInvoice[invoice]) != 1 {
		t.Errorf("OfInvoices() = %v, %v", byInvoice, err)
	}
	if _, err := f.svc.Update(ctx, adminID, e.ID, boss, Input{Description: ptr("Changed")}); len(fieldsOf(t, err)["id"]) == 0 {
		t.Errorf("Update() billed = %v", err)
	}
	if err := f.svc.Delete(ctx, adminID, e.ID, boss); len(fieldsOf(t, err)["id"]) == 0 {
		t.Errorf("Delete() billed = %v", err)
	}
	if err := f.svc.Delete(ctx, adminID, running.ID, boss); err != nil {
		t.Errorf("Delete() = %v", err)
	}
}

func TestUpdateKeepsWhatIsNotSent(t *testing.T) {
	f := newFixture()
	ctx := context.Background()
	e, _ := f.svc.Create(ctx, adminID, boss, Input{UserID: ptr(owner), StartedAt: at("2026-11-10T08:00:00Z"), EndedAt: at("2026-11-10T09:00:00Z"),
		Description: ptr("Work"), ContactID: ptr(customer)})
	up, err := f.svc.Update(ctx, adminID, e.ID, boss, Input{Billable: ptr(false), ContactID: ptr(resource.ID(0)), SalesInvoiceID: ptr(invoice)})
	if err != nil || up.Billable || up.ContactID != nil || up.Description != "Work" || *up.SalesInvoiceID != invoice {
		t.Errorf("updated = %+v, %v", up, err)
	}
}

func TestNotes(t *testing.T) {
	f := newFixture()
	ctx := context.Background()
	e, _ := f.svc.Create(ctx, adminID, boss, Input{UserID: ptr(owner), StartedAt: at("2026-11-10T08:00:00Z"), Description: ptr("Work")})
	n, err := f.svc.AddNote(ctx, adminID, e.ID, boss, notes.Input{Note: "Check"})
	if err != nil || n.EntityType != "TimeEntry" {
		t.Fatalf("AddNote() = %+v, %v", n, err)
	}
	if got, _ := f.svc.Entry(ctx, adminID, e.ID, boss); len(got.Notes) != 1 {
		t.Errorf("notes = %+v", got.Notes)
	}
	if err := f.svc.DeleteNote(ctx, adminID, e.ID, n.ID, boss); err != nil {
		t.Errorf("DeleteNote() = %v", err)
	}
	if err := f.svc.DeleteNote(ctx, adminID, e.ID, n.ID, boss); !IsNotFound(err) {
		t.Errorf("DeleteNote() again = %v", err)
	}
}

func TestParseFilterAndMatches(t *testing.T) {
	today := time.Date(2026, 11, 11, 12, 0, 0, 0, time.UTC)
	q, err := ParseFilter("", today)
	if err != nil || !q.From.Equal(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)) || !q.To.Equal(time.Date(2026, 12, 31, 0, 0, 0, 0, time.UTC)) {
		t.Errorf("default = %+v, %v", q, err)
	}
	q, err = ParseFilter("state:open|non_billable,contact_id:10,include_nil_contacts:true,include_active:true,project_id:11,user_id:2,day:2026-11-10", today)
	if err != nil || len(q.States) != 2 || *q.ContactID != customer || !q.IncludeNilContacts || !q.IncludeActive || *q.ProjectID != falcon || *q.UserID != owner ||
		q.From != nil || !q.Day.Equal(time.Date(2026, 11, 10, 0, 0, 0, 0, time.UTC)) {
		t.Errorf("full = %+v, %v", q, err)
	}
	if q, err := ParseFilter("state:all", today); err != nil || q.States != nil || q.From != nil {
		t.Errorf("state:all = %+v, %v", q, err)
	}
	for _, bad := range []string{"state:late", "period:2024", "contact_id:x", "include_active:yes", "day:11-11-2026", "colour:red", "nonsense"} {
		if _, err := ParseFilter(bad, today); err == nil {
			t.Errorf("ParseFilter(%q) accepted", bad)
		}
	}

	ams, _ := time.LoadLocation("Europe/Amsterdam")
	late := TimeEntry{UserID: owner, StartedAt: *at("2026-11-10T23:30:00Z"), EndedAt: at("2026-11-11T00:30:00Z"), Billable: true, Description: "Late night"}
	day := time.Date(2026, 11, 11, 0, 0, 0, 0, time.UTC)
	if !(Query{Day: &day, Location: ams}).Matches(late) || (Query{Day: &day}).Matches(late) {
		t.Error("day is not read in the administration's zone")
	}
	cases := []struct {
		q    Query
		e    TimeEntry
		want bool
	}{
		{Query{}, TimeEntry{}, false}, // running
		{Query{IncludeActive: true}, TimeEntry{}, true},
		{Query{States: []string{Open}}, late, true},
		{Query{States: []string{Open}}, TimeEntry{EndedAt: late.EndedAt, Billable: true, SalesInvoiceID: ptr(invoice)}, false},
		{Query{States: []string{NonBillable}}, late, false},
		{Query{ContactID: ptr(customer)}, late, false},
		{Query{ContactID: ptr(customer), IncludeNilContacts: true}, late, true},
		{Query{Search: "NIGHT"}, late, true},
		{Query{ProjectID: ptr(falcon)}, late, false},
		{Query{UserID: ptr(employee)}, late, false},
	}
	for i, c := range cases {
		if got := c.q.Matches(c.e); got != c.want {
			t.Errorf("case %d: Matches() = %v", i, got)
		}
	}
}
