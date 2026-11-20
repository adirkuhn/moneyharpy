package mbimport

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/adirkuhn/moneyharpy/internal/administration"
	"github.com/adirkuhn/moneyharpy/internal/bookkeeping"
	"github.com/adirkuhn/moneyharpy/internal/contacts"
	"github.com/adirkuhn/moneyharpy/internal/membership"
	"github.com/adirkuhn/moneyharpy/internal/products"
	"github.com/adirkuhn/moneyharpy/internal/projects"
	"github.com/adirkuhn/moneyharpy/internal/resource"
)

const (
	adminID resource.ID = 100
	owner   resource.ID = 1
	clerk   resource.ID = 2
	source              = "123"
)

var now = time.Date(2026, 11, 20, 9, 0, 0, 0, time.UTC)

type fakeAdmins struct{}

func (fakeAdmins) Get(_ context.Context, id, user resource.ID) (administration.Listed, error) {
	if id != adminID {
		return administration.Listed{}, administration.ErrNotFound
	}
	perms := []administration.Permission{administration.Settings}
	if user == owner {
		perms = append(perms, administration.Ownership)
	}
	return administration.Listed{Administration: administration.Administration{ID: adminID}, Membership: administration.Membership{Permissions: perms}}, nil
}

// fakeMoneybird serves canned records, as Moneybird's API would.
type fakeMoneybird struct {
	down    bool
	token   string
	records map[string][]string
	waits   []time.Time
	fetched [][]string
}

func (f *fakeMoneybird) Administrations(_ context.Context, token string) ([]SourceAdministration, error) {
	if f.down {
		return nil, errors.New("dial tcp: lookup moneybird.com: no such host")
	}
	if token != f.token {
		return nil, ErrTokenRejected
	}
	return []SourceAdministration{{ID: source, Name: "Example Shop B.V."}}, nil
}

func (f *fakeMoneybird) Open(token string, onWait func(time.Time)) Reader {
	return &fakeReader{f: f, token: token, onWait: onWait}
}

type fakeReader struct {
	f      *fakeMoneybird
	token  string
	onWait func(time.Time)
}

func (r *fakeReader) raws(path string) ([]json.RawMessage, error) {
	if r.token != r.f.token {
		return nil, ErrTokenRejected
	}
	var out []json.RawMessage
	for _, s := range r.f.records[path] {
		out = append(out, json.RawMessage(s))
	}
	return out, nil
}

func (r *fakeReader) List(_ context.Context, admin, path string, _ url.Values) ([]json.RawMessage, error) {
	if admin != source {
		return nil, errors.New("wrong administration")
	}
	for _, at := range r.f.waits {
		r.onWait(at)
	}
	return r.raws(path)
}

func (r *fakeReader) SyncIDs(_ context.Context, _, kind string, _ url.Values) ([]Version, error) {
	raws, err := r.raws(kind)
	var out []Version
	for _, raw := range raws {
		var head struct {
			ID mbID `json:"id"`
		}
		_ = json.Unmarshal(raw, &head)
		out = append(out, Version{ID: head.ID.id().String(), Version: 1})
	}
	return out, err
}

func (r *fakeReader) FetchByIDs(_ context.Context, _, kind string, ids []string) ([]json.RawMessage, error) {
	r.f.fetched = append(r.f.fetched, ids)
	raws, err := r.raws(kind)
	var out []json.RawMessage
	for _, raw := range raws {
		for _, id := range ids {
			if strings.Contains(string(raw), `"id":"`+id+`"`) {
				out = append(out, raw)
			}
		}
	}
	return out, err
}

func (r *fakeReader) Get(context.Context, string, string, url.Values) (json.RawMessage, error) {
	return json.RawMessage(`{}`), nil
}

func (r *fakeReader) Download(_ context.Context, _, path string) ([]byte, string, error) {
	return []byte("file " + path), "application/pdf", nil
}

type memStore struct {
	problems []Problem
	job      *Job
	imported map[string]resource.ID
	empty    bool
	saves    []Job
}

func key(kind string, id resource.ID) string { return kind + ":" + id.String() }

func (m *memStore) Job(context.Context, resource.ID) (Job, error) {
	if m.job == nil {
		return Job{}, ErrNotFound
	}
	return *m.job, nil
}
func (m *memStore) SaveJob(_ context.Context, j Job) error {
	m.job = &j
	m.saves = append(m.saves, j)
	return nil
}
func (m *memStore) LeaseDueJobs(_ context.Context, at, until time.Time, _ int) ([]Job, error) {
	if m.job == nil || m.job.State != Running || m.job.NextAttemptAt == nil || m.job.NextAttemptAt.After(at) {
		return nil, nil
	}
	m.job.NextAttemptAt = &until
	return []Job{*m.job}, nil
}
func (m *memStore) Imported(_ context.Context, _ resource.ID, kind string, id resource.ID) (resource.ID, bool, error) {
	t, ok := m.imported[key(kind, id)]
	return t, ok, nil
}
func (m *memStore) RecordImported(_ context.Context, _ resource.ID, kind string, source, target resource.ID) error {
	m.imported[key(kind, source)] = target
	return nil
}
func (m *memStore) Empty(context.Context, resource.ID) (bool, error) { return m.empty, nil }
func (m *memStore) RecordProblem(_ context.Context, _ resource.ID, p Problem) error {
	m.problems = append(m.problems, p)
	return nil
}
func (m *memStore) ResolveProblems(_ context.Context, _ resource.ID, kind string, id resource.ID) error {
	m.problems = slices.DeleteFunc(m.problems, func(p Problem) bool { return p.Kind == kind && p.SourceID == id && !p.Partial })
	return nil
}
func (m *memStore) Problems(context.Context, resource.ID, int, int) ([]Problem, int, error) {
	return m.problems, len(m.problems), nil
}

type plainSecrets struct{}

func (plainSecrets) Seal(s string) (string, error) { return "sealed:" + s, nil }
func (plainSecrets) Open(s string) (string, error) { return strings.TrimPrefix(s, "sealed:"), nil }

// fakeTargets take in records as the services would, refusing a contact
// without a name to show a record the import passes.
type fakeTargets struct {
	chart    bookkeeping.ChartImport
	contacts map[resource.ID]contacts.Changes
	people   map[resource.ID][]contacts.ImportedPerson
	products map[resource.ID]products.Changes
	projects map[resource.ID]projects.Changes
}

func (f *fakeTargets) ImportChart(_ context.Context, _, _ resource.ID, in bookkeeping.ChartImport) (bookkeeping.ChartIDs, error) {
	f.chart = in
	ids := bookkeeping.ChartIDs{}
	for _, a := range in.Accounts {
		ids[a.Origin.ID] = a.Origin.ID
	}
	// The default receivables account takes Moneybird's place.
	ids[9001] = 42
	for _, r := range in.Rates {
		ids[r.Origin.ID] = r.Origin.ID
	}
	return ids, nil
}

func (f *fakeTargets) UpdateSettings(context.Context, resource.ID, resource.ID, bookkeeping.SettingsChanges) (bookkeeping.Settings, error) {
	return bookkeeping.Settings{}, nil
}

func (f *fakeTargets) Import(_ context.Context, _, _ resource.ID, o resource.Origin, c contacts.Changes, people []contacts.ImportedPerson) (contacts.Contact, error) {
	if c.CompanyName == nil && c.Firstname == nil {
		var verr resource.ValidationError
		verr.Add("company_name", "can't be blank")
		return contacts.Contact{}, verr.OrNil()
	}
	f.contacts[o.ID], f.people[o.ID] = c, people
	return contacts.Contact{ID: o.ID}, nil
}

type productTarget struct{ f *fakeTargets }

func (t productTarget) Import(_ context.Context, _, _ resource.ID, o resource.Origin, c products.Changes) (products.Product, error) {
	t.f.products[o.ID] = c
	return products.Product{ID: o.ID}, nil
}

type projectTarget struct{ f *fakeTargets }

func (t projectTarget) Import(_ context.Context, _, _ resource.ID, o resource.Origin, c projects.Changes) (projects.Project, error) {
	t.f.projects[o.ID] = c
	return projects.Project{ID: o.ID}, nil
}

func setup() (*Service, *memStore, *fakeMoneybird, *fakeTargets) {
	mb := &fakeMoneybird{token: "good", records: map[string][]string{
		"ledger_accounts": {
			`{"id":"9001","name":"Debiteuren","account_type":"current_assets","account_id":"13011","active":true,"taxonomy_item":{"code":"BVorDebHad"}}`,
			`{"id":"9002","name":"Omzet","account_type":"revenue","account_id":"8000","active":true}`,
		},
		"tax_rates": {`{"id":"9101","name":"21% btw","percentage":"21.0","tax_rate_type":"sales_invoice","show_tax":true,"active":true}`},
		"contacts": {
			`{"id":"501","company_name":"Example Shop B.V.","customer_id":"7","created_at":"2024-03-01T10:00:00.000Z","updated_at":"2024-03-02T10:00:00.000Z","contact_people":[{"id":"601","firstname":"Ann","lastname":"de Vries"}]}`,
			`{"id":"502","customer_id":"8"}`,
		},
		"products": {`{"id":"701","title":"Hosting","price":"10.0","tax_rate_id":"9101","ledger_account_id":"9001"}`},
		"projects": {`{"id":"801","name":"Website","state":"archived","budget":40}`},
	}}
	store := &memStore{imported: map[string]resource.ID{}, empty: true}
	t := &fakeTargets{contacts: map[resource.ID]contacts.Changes{}, people: map[resource.ID][]contacts.ImportedPerson{}, products: map[resource.ID]products.Changes{}, projects: map[resource.ID]projects.Changes{}}
	targets := Targets{Books: t, Contacts: t, Products: productTarget{t}, Projects: projectTarget{t}}
	return NewService(fakeAdmins{}, mb, store, plainSecrets{}, targets, func() time.Time { return now }), store, mb, t
}

func TestStartChecksTheOwnerTokenAndAnEmptyAdministration(t *testing.T) {
	s, store, _, _ := setup()
	ctx := context.Background()
	if _, err := s.Start(ctx, adminID, clerk, "good", source); !errors.Is(err, membership.ErrForbidden) {
		t.Errorf("a member who does not own it started an import: %v", err)
	}
	if _, err := s.Start(ctx, adminID, owner, "bad", source); !errors.Is(err, ErrTokenRejected) {
		t.Errorf("bad token: %v", err)
	}
	var verr *resource.ValidationError
	if _, err := s.Start(ctx, adminID, owner, "good", "999"); !errors.As(err, &verr) {
		t.Errorf("an administration the token does not reach: %v", err)
	}
	store.empty = false
	if _, err := s.Start(ctx, adminID, owner, "good", source); !errors.Is(err, ErrNotEmpty) {
		t.Errorf("into a used administration: %v", err)
	}
	store.empty = true
	j, err := s.Start(ctx, adminID, owner, " good ", source)
	if err != nil {
		t.Fatal(err)
	}
	if j.State != Running || j.SourceName != "Example Shop B.V." || j.Token != "sealed:good" || len(j.Steps) != len(Steps) {
		t.Errorf("job %+v", j)
	}
	if _, err := s.Start(ctx, adminID, owner, "good", source); !errors.Is(err, ErrRunning) {
		t.Errorf("a second import while one runs: %v", err)
	}
}

func TestWorkImportsEverythingAndForgetsTheToken(t *testing.T) {
	s, store, _, f := setup()
	ctx := context.Background()
	if _, err := s.Start(ctx, adminID, owner, "good", source); err != nil {
		t.Fatal(err)
	}
	if err := s.Work(ctx); err != nil {
		t.Fatal(err)
	}
	j := *store.job
	if j.State != Done || j.Token != "" || j.FinishedAt == nil {
		t.Fatalf("job %+v", j)
	}
	if len(f.chart.Accounts) != 2 || f.chart.Accounts[0].Taxonomy != "BVorDebHad" || f.chart.Rates[0].Percentage != "21.0" {
		t.Errorf("chart %+v", f.chart)
	}
	c := f.contacts[501]
	if *c.CompanyName != "Example Shop B.V." || *c.CustomerID != "7" || len(f.people[501]) != 1 || f.people[501][0].Origin.ID != 601 {
		t.Errorf("contact %+v, people %+v", c, f.people[501])
	}
	// The product points to the category that took Moneybird's place.
	p := f.products[701]
	if *p.Price != "10.0" || *p.LedgerAccountID != 42 || *p.TaxRateID != 9101 {
		t.Errorf("product %+v", p)
	}
	if pr := f.projects[801]; *pr.State != "archived" || *pr.Budget != "40" {
		t.Errorf("project %+v", pr)
	}
	contactsStep := j.Step(StepContacts)
	if contactsStep.Total != 2 || contactsStep.Imported != 1 || contactsStep.Failed != 1 || !strings.Contains(contactsStep.Problems[0], "502") {
		t.Errorf("contacts step %+v", contactsStep)
	}
	for _, st := range j.Steps {
		if !st.Done {
			t.Errorf("step %s not done", st.Kind)
		}
	}
}

func TestRunningAgainAddsOnlyWhatIsNew(t *testing.T) {
	s, store, mb, f := setup()
	ctx := context.Background()
	if _, err := s.Start(ctx, adminID, owner, "good", source); err != nil {
		t.Fatal(err)
	}
	_ = s.Work(ctx)
	store.empty = false
	mb.records["contacts"] = append(mb.records["contacts"], `{"id":"503","company_name":"Beispiel GmbH"}`)
	mb.fetched = nil
	if _, err := s.Start(ctx, adminID, owner, "good", source); err != nil {
		t.Fatalf("the same Moneybird administration again: %v", err)
	}
	_ = s.Work(ctx)
	if _, ok := f.contacts[503]; !ok {
		t.Error("the new contact was not imported")
	}
	// Only the contacts not imported yet are fetched; 502 failed before and is tried again.
	if len(mb.fetched) != 1 || strings.Join(mb.fetched[0], ",") != "502,503" {
		t.Errorf("fetched %v", mb.fetched)
	}
	if st := store.job.Step(StepContacts); st.Skipped != 1 || st.Imported != 1 {
		t.Errorf("contacts step %+v", st)
	}
}

func TestARejectedTokenFailsTheImport(t *testing.T) {
	s, store, mb, _ := setup()
	ctx := context.Background()
	if _, err := s.Start(ctx, adminID, owner, "good", source); err != nil {
		t.Fatal(err)
	}
	mb.token = "revoked"
	_ = s.Work(ctx)
	if j := store.job; j.State != Failed || j.Token != "" || !strings.Contains(j.Error, "refused") {
		t.Errorf("job %+v", j)
	}
}

func TestTheWaitForTheRateLimitShows(t *testing.T) {
	s, store, mb, _ := setup()
	ctx := context.Background()
	until := now.Add(3 * time.Minute)
	mb.waits = []time.Time{until}
	if _, err := s.Start(ctx, adminID, owner, "good", source); err != nil {
		t.Fatal(err)
	}
	_ = s.Work(ctx)
	shown := false
	for _, j := range store.saves {
		if j.WaitingUntil != nil && j.WaitingUntil.Equal(until) {
			shown = true
		}
	}
	if !shown || store.job.WaitingUntil != nil {
		t.Errorf("the wait was not shown while waiting, or stayed after: %v", store.job.WaitingUntil)
	}
}

func TestMoneybirdOutOfReachSaysSo(t *testing.T) {
	s, _, mb, _ := setup()
	mb.down = true
	if _, err := s.Administrations(context.Background(), adminID, owner, "good"); !errors.Is(err, ErrUnavailable) {
		t.Errorf("Administrations() = %v", err)
	}
	if _, err := s.Start(context.Background(), adminID, owner, "good", source); !errors.Is(err, ErrUnavailable) {
		t.Errorf("Start() = %v", err)
	}
}

// failingReader stands for Moneybird being out of reach for a while.
type failingReader struct{ Reader }

func (failingReader) List(context.Context, string, string, url.Values) ([]json.RawMessage, error) {
	return nil, errors.New("moneybird is down")
}

type downSource struct{ *fakeMoneybird }

func (d downSource) Open(string, func(time.Time)) Reader { return failingReader{} }

func TestAnImportStaysQueuedUntilItFinishes(t *testing.T) {
	s, store, mb, _ := setup()
	ctx := context.Background()
	if _, err := s.Start(ctx, adminID, owner, "good", source); err != nil {
		t.Fatal(err)
	}
	clock := now
	s.now = func() time.Time { return clock }
	s.source = downSource{mb}
	var waits []time.Duration
	for range 8 {
		if err := s.Work(ctx); err != nil {
			t.Fatal(err)
		}
		j := store.job
		if j.State != Running || j.Token == "" || j.NextAttemptAt == nil || !strings.Contains(j.Error, "moneybird is down") {
			t.Fatalf("after %d attempts: %+v", j.Attempts, j)
		}
		waits = append(waits, j.NextAttemptAt.Sub(clock))
		clock = *j.NextAttemptAt
	}
	// The pauses grow, up to half an hour.
	if waits[0] != time.Minute || waits[1] != 2*time.Minute || waits[7] != 30*time.Minute {
		t.Errorf("waits %v", waits)
	}
	// Once Moneybird is back, the import goes on and ends.
	s.source = mb
	if err := s.Work(ctx); err != nil {
		t.Fatal(err)
	}
	if store.job.State != Done || store.job.Error != "" {
		t.Errorf("job %+v", store.job)
	}
}

// pgError stands for a database error with its SQLSTATE, as pgx gives one.
type pgError struct{ code string }

func (e pgError) Error() string    { return "ERROR: duplicate key value (SQLSTATE " + e.code + ")" }
func (e pgError) SQLState() string { return e.code }

func TestADuplicateRecordIsAProblemNotAStop(t *testing.T) {
	if !isRecordProblem(fmt.Errorf("store contact: %w", pgError{"23505"})) {
		t.Error("a duplicate key stops the import")
	}
	if isRecordProblem(fmt.Errorf("store contact: %w", pgError{"57P01"})) {
		t.Error("a database shutting down counts as a record's problem")
	}
}

func TestTryNowBringsAQueuedImportForward(t *testing.T) {
	s, store, mb, _ := setup()
	ctx := context.Background()
	if _, err := s.Start(ctx, adminID, owner, "good", source); err != nil {
		t.Fatal(err)
	}
	s.source = downSource{mb}
	_ = s.Work(ctx)
	if store.job.NextAttemptAt == nil || !store.job.NextAttemptAt.After(now) {
		t.Fatalf("not queued: %+v", store.job)
	}
	if _, err := s.TryNow(ctx, adminID, clerk); !errors.Is(err, membership.ErrForbidden) {
		t.Errorf("a member who does not own it: %v", err)
	}
	j, err := s.TryNow(ctx, adminID, owner)
	if err != nil {
		t.Fatal(err)
	}
	if !j.NextAttemptAt.Equal(now) || !store.job.NextAttemptAt.Equal(now) {
		t.Errorf("next attempt %v", j.NextAttemptAt)
	}
	// Only a queued import can be brought forward.
	s.source = mb
	_ = s.Work(ctx)
	if _, err := s.TryNow(ctx, adminID, owner); !errors.Is(err, ErrNotQueued) {
		t.Errorf("a finished import: %v", err)
	}
}
