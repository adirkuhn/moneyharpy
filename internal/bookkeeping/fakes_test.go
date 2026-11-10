package bookkeeping

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
)

type fakeAdmins struct{ admin administration.Administration }

func (f fakeAdmins) Get(_ context.Context, id, user resource.ID) (administration.Listed, error) {
	if id != f.admin.ID || user != owner {
		return administration.Listed{}, administration.ErrNotFound
	}
	return administration.Listed{Administration: f.admin}, nil
}

type memStore struct {
	settings Settings
	seeds    int
	rates    map[resource.ID]TaxRate
	accounts map[resource.ID]LedgerAccount
}

func newMemStore() *memStore {
	return &memStore{
		settings: Settings{VATPeriod: "quarter", ICPPeriod: "quarter"},
		rates:    map[resource.ID]TaxRate{},
		accounts: map[resource.ID]LedgerAccount{},
	}
}

func (m *memStore) Settings(context.Context, resource.ID) (Settings, error) { return m.settings, nil }
func (m *memStore) SaveSettings(_ context.Context, _ resource.ID, s Settings) error {
	m.settings = s
	return nil
}
func (m *memStore) ClearTaxonomy(context.Context, resource.ID) error {
	for id, l := range m.accounts {
		l.Taxonomy = ""
		m.accounts[id] = l
	}
	return nil
}
func (m *memStore) Seed(_ context.Context, _ resource.ID, scheme string, rates []TaxRate, accounts []LedgerAccount) (bool, error) {
	if m.settings.Seeded {
		return false, nil
	}
	m.seeds++
	m.settings.Seeded, m.settings.Scheme = true, scheme
	for _, r := range rates {
		m.rates[r.ID] = r
	}
	for _, l := range accounts {
		m.accounts[l.ID] = l
	}
	return true, nil
}
func (m *memStore) TaxRates(context.Context, resource.ID) ([]TaxRate, error) {
	return slices.Collect(maps.Values(m.rates)), nil
}
func (m *memStore) TaxRate(_ context.Context, _, id resource.ID) (TaxRate, error) {
	r, ok := m.rates[id]
	if !ok {
		return TaxRate{}, ErrTaxRateNotFound
	}
	return r, nil
}
func (m *memStore) CreateTaxRate(_ context.Context, r TaxRate) error {
	m.rates[r.ID] = r
	return nil
}
func (m *memStore) UpdateTaxRates(_ context.Context, rates []TaxRate) error {
	for _, r := range rates {
		m.rates[r.ID] = r
	}
	return nil
}
func (m *memStore) LedgerAccounts(context.Context, resource.ID) ([]LedgerAccount, error) {
	return slices.Collect(maps.Values(m.accounts)), nil
}
func (m *memStore) LedgerAccount(_ context.Context, _, id resource.ID) (LedgerAccount, error) {
	l, ok := m.accounts[id]
	if !ok {
		return LedgerAccount{}, ErrLedgerAccountNotFound
	}
	return l, nil
}
func (m *memStore) CreateLedgerAccounts(_ context.Context, accounts []LedgerAccount) error {
	for _, l := range accounts {
		m.accounts[l.ID] = l
	}
	return nil
}
func (m *memStore) UpdateLedgerAccounts(ctx context.Context, accounts []LedgerAccount) error {
	return m.CreateLedgerAccounts(ctx, accounts)
}
func (m *memStore) DeleteLedgerAccount(_ context.Context, _, id resource.ID) error {
	delete(m.accounts, id)
	return nil
}

var now = time.Date(2026, 11, 10, 9, 0, 0, 0, time.UTC)

func newTestService(country string) (*Service, *memStore) {
	store := newMemStore()
	admin := administration.Administration{ID: adminID, Language: "en", Country: country, TimeZone: "Europe/Amsterdam"}
	return NewService(fakeAdmins{admin}, store, resource.NewGenerator(1), func() time.Time { return now }), store
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

func byKey(t *testing.T, s *memStore, key string) LedgerAccount {
	t.Helper()
	for _, l := range s.accounts {
		if l.Key == key {
			return l
		}
	}
	t.Fatalf("no account with key %q", key)
	return LedgerAccount{}
}
