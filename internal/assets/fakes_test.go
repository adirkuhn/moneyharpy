package assets

import (
	"context"
	"slices"
	"time"

	"github.com/adirkuhn/moneyharpy/internal/administration"
	"github.com/adirkuhn/moneyharpy/internal/bookkeeping"
	"github.com/adirkuhn/moneyharpy/internal/journal"
	"github.com/adirkuhn/moneyharpy/internal/money"
	"github.com/adirkuhn/moneyharpy/internal/resource"
)

const (
	adminID         resource.ID = 100
	owner, stranger resource.ID = 1, 2
	equipment       resource.ID = 600
	land            resource.ID = 601
	child           resource.ID = 602
	costs           resource.ID = 603
	depreciation    resource.ID = 700
	accumulated     resource.ID = 701
	bookResult      resource.ID = 702
	reserve         resource.ID = 703
	purchaseLine    resource.ID = 900
	otherLine       resource.ID = 901
	journalRow      resource.ID = 902
)

var now = time.Date(2026, 11, 11, 9, 0, 0, 0, time.UTC)

func day(s string) time.Time {
	t, err := time.Parse(time.DateOnly, s)
	if err != nil {
		panic(err)
	}
	return t
}

type fakeAdmins struct{}

func (fakeAdmins) Get(_ context.Context, id, user resource.ID) (administration.Listed, error) {
	if id != adminID || user != owner {
		return administration.Listed{}, administration.ErrNotFound
	}
	return administration.Listed{Administration: administration.Administration{ID: adminID, Currency: "EUR", TimeZone: "UTC", Country: "NL"}}, nil
}

type fakeLedger struct{ locked string }

func (l *fakeLedger) LedgerAccount(_ context.Context, _, id, _ resource.ID) (bookkeeping.LedgerAccount, error) {
	parent := equipment
	switch id {
	case equipment:
		return bookkeeping.LedgerAccount{ID: id, Type: "non_current_assets", Active: true, Taxonomy: "BMvaBei"}, nil
	case land:
		return bookkeeping.LedgerAccount{ID: id, Type: "non_current_assets", Active: true, Taxonomy: "BMvaTer"}, nil
	case child:
		return bookkeeping.LedgerAccount{ID: id, Type: "non_current_assets", Active: true, ParentID: &parent}, nil
	case costs:
		return bookkeeping.LedgerAccount{ID: id, Type: "expenses", Active: true}, nil
	}
	return bookkeeping.LedgerAccount{}, bookkeeping.ErrLedgerAccountNotFound
}

func (l *fakeLedger) SystemLedgerAccount(_ context.Context, _, _ resource.ID, role string) (resource.ID, error) {
	return map[string]resource.ID{DepreciationRole: depreciation, BookResultRole: bookResult, ReserveRole: reserve}[role], nil
}

func (l *fakeLedger) AccumulatedDepreciation(_ context.Context, _, _, _ resource.ID) (resource.ID, error) {
	return accumulated, nil
}

func (l *fakeLedger) LockedUntil(context.Context, resource.ID, resource.ID) (string, error) {
	return l.locked, nil
}

type fakeStore struct {
	assets  map[resource.ID]Asset
	entries map[resource.ID][]journal.Entry
	locks   int
}

func newFakeStore() *fakeStore {
	return &fakeStore{assets: map[resource.ID]Asset{}, entries: map[resource.ID][]journal.Entry{}}
}

func (s *fakeStore) Assets(_ context.Context, _ resource.ID, q Query) ([]Asset, int, error) {
	var out []Asset
	for _, a := range s.assets {
		if q.LedgerAccountID != nil && a.LedgerAccountID != *q.LedgerAccountID || q.OnlyActive && a.Disposal != nil {
			continue
		}
		out = append(out, a)
	}
	slices.SortFunc(out, func(a, b Asset) int { return a.PurchaseDate.Compare(b.PurchaseDate) })
	return out, len(out), nil
}

func (s *fakeStore) Asset(_ context.Context, _, id resource.ID) (Asset, error) {
	a, ok := s.assets[id]
	if !ok {
		return Asset{}, ErrNotFound
	}
	return a, nil
}

func (s *fakeStore) Save(_ context.Context, a Asset, _ bool, entries []journal.Entry) error {
	if err := journal.Balance(entries); err != nil {
		return err
	}
	s.assets[a.ID] = a
	s.entries[a.ID] = entries
	return nil
}

func (s *fakeStore) Delete(_ context.Context, _, id resource.ID) error {
	delete(s.assets, id)
	delete(s.entries, id)
	return nil
}

func (s *fakeStore) ReserveBalance(context.Context, resource.ID) (money.Amount, error) {
	var sum money.Amount
	for _, a := range s.assets {
		if a.Disposal != nil && a.Disposal.ReserveSale != nil {
			sum += a.Disposal.ReserveSale.Amount
		}
		if a.ReservePurchase != nil {
			sum -= a.ReservePurchase.Amount
		}
	}
	return sum, nil
}

func (s *fakeStore) DetailLedgerAccount(_ context.Context, _, id resource.ID) (*resource.ID, error) {
	l := map[resource.ID]resource.ID{purchaseLine: equipment, otherLine: costs}
	if v, ok := l[id]; ok {
		return &v, nil
	}
	return nil, ErrBookingNotFound
}

func (s *fakeStore) EntryLedgerAccount(_ context.Context, _, id resource.ID) (resource.ID, error) {
	if id == journalRow {
		return equipment, nil
	}
	return 0, ErrBookingNotFound
}

func (s *fakeStore) Workload(context.Context) ([]Workload, error) {
	return []Workload{{AdministrationID: adminID, UserID: owner}}, nil
}

func (s *fakeStore) Lock(context.Context, resource.ID) (func(), bool, error) {
	s.locks++
	return func() {}, true, nil
}

type notice struct {
	action string
	date   time.Time
}

type fakeNotifier struct{ got []notice }

func (n *fakeNotifier) Notify(_ context.Context, _ resource.ID, action string, _ Asset, c ValueChange) {
	n.got = append(n.got, notice{action, c.Date})
}

type harness struct {
	svc      *Service
	store    *fakeStore
	ledger   *fakeLedger
	notifier *fakeNotifier
	clock    *time.Time
}

func newHarness() *harness {
	h := &harness{store: newFakeStore(), ledger: &fakeLedger{}, notifier: &fakeNotifier{}}
	t := now
	h.clock = &t
	h.svc = NewService(fakeAdmins{}, h.ledger, h.store, h.notifier, resource.NewGenerator(1), func() time.Time { return *h.clock })
	return h
}
