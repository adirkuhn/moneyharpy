package banking

import (
	"context"
	"errors"
	"maps"
	"slices"
	"testing"
	"time"

	"github.com/adirkuhn/moneyharpy/internal/administration"
	"github.com/adirkuhn/moneyharpy/internal/resource"
)

const (
	adminID         resource.ID = 100
	owner, stranger resource.ID = 1, 2
	cashCategory    resource.ID = 700
	rentCategory    resource.ID = 701
	closedCategory  resource.ID = 702
)

type fakeAdmins struct{ admin administration.Administration }

func (f fakeAdmins) Get(_ context.Context, id, user resource.ID) (administration.Listed, error) {
	if id != f.admin.ID || user != owner {
		return administration.Listed{}, administration.ErrNotFound
	}
	return administration.Listed{Administration: f.admin}, nil
}

type fakeLedger struct {
	next    resource.ID
	created []string
	locked  string
}

func (f *fakeLedger) FinancialLedgerAccount(_ context.Context, _, _ resource.ID, name, _ string) (resource.ID, error) {
	f.next++
	f.created = append(f.created, name)
	return 800 + f.next, nil
}

func (f *fakeLedger) SeedLedgerAccount(_ context.Context, _, _ resource.ID, key string) (resource.ID, bool, error) {
	return cashCategory, key == "cash", nil
}

func (f *fakeLedger) CheckBookable(_ context.Context, _, _, id resource.ID) error {
	if id == rentCategory || id == cashCategory || id > 800 {
		return nil
	}
	var verr resource.ValidationError
	verr.Add("booking_id", "is a deactivated category")
	return &verr
}

func (f *fakeLedger) LockedUntil(context.Context, resource.ID, resource.ID) (string, error) {
	return f.locked, nil
}

type memStore struct {
	settings   Settings
	accounts   map[resource.ID]Account
	statements map[resource.ID]Statement
	mutations  map[resource.ID]Mutation
	rules      map[resource.ID]Rule
}

func newMemStore() *memStore {
	return &memStore{accounts: map[resource.ID]Account{}, statements: map[resource.ID]Statement{}, mutations: map[resource.ID]Mutation{}, rules: map[resource.ID]Rule{}}
}

func (m *memStore) Settings(context.Context, resource.ID) (Settings, error) { return m.settings, nil }
func (m *memStore) SetAutoLink(_ context.Context, _ resource.ID, on bool) error {
	m.settings.AutoLink = on
	return nil
}
func (m *memStore) Seed(_ context.Context, _ resource.ID, accounts []Account) (bool, error) {
	if m.settings.Seeded {
		return false, nil
	}
	m.settings.Seeded = true
	for _, a := range accounts {
		m.accounts[a.ID] = a
	}
	return true, nil
}
func (m *memStore) Accounts(context.Context, resource.ID) ([]Account, error) {
	return slices.Collect(maps.Values(m.accounts)), nil
}
func (m *memStore) Account(_ context.Context, _, id resource.ID) (Account, error) {
	a, ok := m.accounts[id]
	if !ok {
		return Account{}, ErrAccountNotFound
	}
	return a, nil
}
func (m *memStore) CreateAccount(_ context.Context, a Account) error {
	m.accounts[a.ID] = a
	return nil
}
func (m *memStore) UpdateAccounts(_ context.Context, accounts []Account) error {
	for _, a := range accounts {
		m.accounts[a.ID] = a
	}
	return nil
}
func (m *memStore) Statement(_ context.Context, _, id resource.ID) (Statement, error) {
	s, ok := m.statements[id]
	if !ok {
		return Statement{}, ErrStatementNotFound
	}
	s.Mutations = nil
	for _, mu := range m.mutations {
		if mu.StatementID == id {
			s.Mutations = append(s.Mutations, mu)
		}
	}
	return s, nil
}
func (m *memStore) known(mu Mutation) bool {
	for _, o := range m.mutations {
		if o.AccountID == mu.AccountID && ((mu.TransactionID != "" && o.TransactionID == mu.TransactionID) || (mu.Fingerprint != "" && o.Fingerprint == mu.Fingerprint)) {
			return true
		}
	}
	return false
}
func (m *memStore) CreateStatement(_ context.Context, s Statement, mutations []Mutation) ([]Mutation, error) {
	var added []Mutation
	for _, mu := range mutations {
		if !m.known(mu) {
			m.mutations[mu.ID] = mu
			added = append(added, mu)
		}
	}
	if len(added) > 0 || len(mutations) == 0 {
		m.statements[s.ID] = s
	}
	return added, nil
}
func (m *memStore) UpdateStatement(_ context.Context, s Statement, added, changed []Mutation, removed []resource.ID) ([]Mutation, error) {
	m.statements[s.ID] = s
	var out []Mutation
	for _, mu := range added {
		if !m.known(mu) {
			m.mutations[mu.ID] = mu
			out = append(out, mu)
		}
	}
	for _, mu := range changed {
		m.mutations[mu.ID] = mu
	}
	for _, id := range removed {
		delete(m.mutations, id)
	}
	return out, nil
}
func (m *memStore) DeleteStatement(_ context.Context, _, id resource.ID) error {
	delete(m.statements, id)
	for k, mu := range m.mutations {
		if mu.StatementID == id {
			delete(m.mutations, k)
		}
	}
	return nil
}
func (m *memStore) Mutations(_ context.Context, _ resource.ID, q MutationQuery) ([]Mutation, error) {
	var out []Mutation
	for _, mu := range m.mutations {
		if (q.AccountID == nil || *q.AccountID == mu.AccountID) && (q.Period == nil || q.Period.Contains(mu.Date)) {
			out = append(out, mu)
		}
	}
	return out, nil
}
func (m *memStore) MutationsByID(_ context.Context, _ resource.ID, ids []resource.ID) ([]Mutation, error) {
	var out []Mutation
	for _, id := range ids {
		if mu, ok := m.mutations[id]; ok {
			out = append(out, mu)
		}
	}
	return out, nil
}
func (m *memStore) Mutation(_ context.Context, _, id resource.ID) (Mutation, error) {
	mu, ok := m.mutations[id]
	if !ok {
		return Mutation{}, ErrMutationNotFound
	}
	mu.Bookings = slices.Clone(mu.Bookings)
	return mu, nil
}
func (m *memStore) MutationVersions(context.Context, resource.ID) ([]Version, error) {
	var out []Version
	for _, mu := range m.mutations {
		out = append(out, Version{ID: mu.ID, Version: mu.Version()})
	}
	return out, nil
}
func (m *memStore) SaveBookings(_ context.Context, mu Mutation, _ []Booking, _ []resource.ID) error {
	m.mutations[mu.ID] = mu
	return nil
}
func (m *memStore) Rules(context.Context, resource.ID) ([]Rule, error) {
	return slices.Collect(maps.Values(m.rules)), nil
}
func (m *memStore) Rule(_ context.Context, _, id resource.ID) (Rule, error) {
	r, ok := m.rules[id]
	if !ok {
		return Rule{}, ErrRuleNotFound
	}
	return r, nil
}
func (m *memStore) CreateRule(_ context.Context, r Rule) error {
	m.rules[r.ID] = r
	return nil
}
func (m *memStore) UpdateRules(_ context.Context, rules []Rule) error {
	for _, r := range rules {
		m.rules[r.ID] = r
	}
	return nil
}
func (m *memStore) DeleteRule(_ context.Context, _, id resource.ID) error {
	delete(m.rules, id)
	return nil
}
func (m *memStore) Balances(context.Context, resource.ID) (map[resource.ID]Amount, error) {
	out := map[resource.ID]Amount{}
	for _, mu := range m.mutations {
		out[mu.AccountID] += mu.Amount
	}
	return out, nil
}

func (m *memStore) CountUnprocessed(_ context.Context, _ resource.ID) (int, error) {
	n := 0
	for _, mu := range m.mutations {
		if mu.State == Unprocessed && m.accounts[mu.AccountID].Active {
			n++
		}
	}
	return n, nil
}

func (m *memStore) RecentlyProcessed(_ context.Context, _ resource.ID, limit int) ([]Mutation, error) {
	var out []Mutation
	for _, mu := range m.mutations {
		if mu.State == Processed {
			out = append(out, mu)
		}
	}
	slices.SortFunc(out, func(a, b Mutation) int { return b.ProcessedAt.Compare(*a.ProcessedAt) })
	return out[:min(limit, len(out))], nil
}

var now = time.Date(2026, 11, 10, 9, 0, 0, 0, time.UTC)

func newTestService() (*Service, *memStore, *fakeLedger) {
	store, ledger := newMemStore(), &fakeLedger{}
	admin := administration.Administration{ID: adminID, Language: "en", Currency: "EUR", Country: "NL", TimeZone: "Europe/Amsterdam"}
	return NewService(fakeAdmins{admin}, ledger, store, resource.NewGenerator(1), func() time.Time { return now }), store, ledger
}

func ptr[T any](v T) *T { return &v }

func fieldsOf(t *testing.T, err error) map[string][]string {
	t.Helper()
	var verr *resource.ValidationError
	if !errors.As(err, &verr) {
		t.Fatalf("error = %v, want *resource.ValidationError", err)
	}
	return verr.Fields
}

// bank adds a bank account with an example IBAN.
func bank(t *testing.T, svc *Service) Account {
	t.Helper()
	a, err := svc.CreateAccount(context.Background(), adminID, owner, AccountChanges{Name: ptr("Main bank"), Identifier: ptr("NL91 ABNA 0417 1643 00")})
	if err != nil {
		t.Fatal(err)
	}
	return a
}

// transactions adds a statement with the given amounts, dated 2026-11-01.
func transactions(t *testing.T, svc *Service, acc Account, amounts ...string) []Mutation {
	t.Helper()
	in := StatementInput{AccountID: &acc.ID, Reference: ptr("S1")}
	for _, a := range amounts {
		in.Mutations = append(in.Mutations, MutationInput{Date: ptr("2026-11-01"), Amount: ptr(a), Message: ptr("Payment " + a), ContraName: ptr("Hosting Company")})
	}
	st, err := svc.CreateStatement(context.Background(), adminID, owner, in)
	if err != nil {
		t.Fatal(err)
	}
	return st.Mutations
}

func resourceID(n int64) resource.ID { return resource.ID(n) }
