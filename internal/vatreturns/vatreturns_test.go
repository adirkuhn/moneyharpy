package vatreturns

import (
	"context"
	"errors"
	"reflect"
	"slices"
	"testing"
	"time"

	"github.com/adirkuhn/moneyharpy/internal/administration"
	"github.com/adirkuhn/moneyharpy/internal/journal"
	"github.com/adirkuhn/moneyharpy/internal/money"
	"github.com/adirkuhn/moneyharpy/internal/resource"
)

type admins struct{}

func (admins) Get(_ context.Context, id, _ resource.ID) (administration.Listed, error) {
	return administration.Listed{Administration: administration.Administration{ID: id, TimeZone: "Europe/Amsterdam", Language: "en"}}, nil
}

type books struct {
	settings Settings
	locked   []time.Time
}

func (b *books) Settings(context.Context, resource.ID, resource.ID) (Settings, error) {
	return b.settings, nil
}

func (b *books) LockUntil(_ context.Context, _, _ resource.ID, day time.Time) error {
	b.locked = append(b.locked, day)
	s := day.Format(time.DateOnly)
	b.settings.LockedUntil = &s
	return nil
}

var roles = map[string]resource.ID{"payable_vat": 1510, "receivable_vat": 1520, "vat_paid_or_received": 1530, "rounding": 9000}

func (b *books) SystemLedgerAccount(_ context.Context, _, _ resource.ID, role string) (resource.ID, error) {
	return roles[role], nil
}

type store struct {
	returns []Return
	entries map[resource.ID][]journal.Entry
	sums    map[money.Period][]RateSum
	icp     []ICPLine
	// icpBy, when set, is the supplies per period; icp otherwise.
	icpBy    map[money.Period][]ICPLine
	first    *time.Time
	payments map[resource.ID]resource.ID
}

func (s *store) Returns(context.Context, resource.ID) ([]Return, error) {
	out := slices.Clone(s.returns)
	slices.SortFunc(out, func(a, b Return) int { return b.Period.From.Compare(a.Period.From) })
	return out, nil
}

func (s *store) Return(_ context.Context, _, id resource.ID) (Return, error) {
	for _, r := range s.returns {
		if r.ID == id {
			return r, nil
		}
	}
	return Return{}, ErrNotFound
}

func (s *store) Create(_ context.Context, r Return, entries []journal.Entry) error {
	s.returns = append(s.returns, r)
	s.entries[r.ID] = entries
	return nil
}

func (s *store) Delete(_ context.Context, _, id resource.ID) error {
	s.returns = slices.DeleteFunc(s.returns, func(r Return) bool { return r.ID == id })
	delete(s.entries, id)
	return nil
}

func (s *store) SavePayment(_ context.Context, r Return, p Payment, _ resource.ID) error {
	for i := range s.returns {
		if s.returns[i].ID == r.ID {
			s.returns[i].Payments = append(s.returns[i].Payments, p)
		}
	}
	s.payments[p.ID] = r.ID
	return nil
}

func (s *store) DeletePayment(_ context.Context, _, id resource.ID) error {
	for i := range s.returns {
		s.returns[i].Payments = slices.DeleteFunc(s.returns[i].Payments, func(p Payment) bool { return p.ID == id })
	}
	delete(s.payments, id)
	return nil
}

func (s *store) PaymentReturn(_ context.Context, _, id resource.ID) (resource.ID, error) {
	r, ok := s.payments[id]
	if !ok {
		return 0, ErrNotFound
	}
	return r, nil
}

func (s *store) RateSums(_ context.Context, _ resource.ID, p money.Period) ([]RateSum, error) {
	return s.sums[p], nil
}

func (s *store) ICPSums(_ context.Context, _ resource.ID, p money.Period) ([]ICPLine, error) {
	if s.icpBy != nil {
		return s.icpBy[p], nil
	}
	return s.icp, nil
}

func (s *store) Checks(context.Context, resource.ID, money.Period) (Checks, error) {
	return Checks{OpenTransactions: 3}, nil
}

func (s *store) FirstEntry(context.Context, resource.ID) (*time.Time, error) { return s.first, nil }

func date(s string) time.Time {
	t, err := time.Parse(time.DateOnly, s)
	if err != nil {
		panic(err)
	}
	return t
}

func period(from, to string) money.Period { return money.Period{From: date(from), To: date(to)} }

var (
	q2 = period("2026-04-01", "2026-06-30")
	q3 = period("2026-07-01", "2026-09-30")
)

// sales is a quarter's 21% sales and 21% purchases.
func sales(base, purchases money.Amount) []RateSum {
	return []RateSum{
		{Type: "sales_invoice", Boxes: []string{"NL/1a"}, Base: -base, Payable: -base * 21 / 100},
		{Type: "purchase_invoice", Boxes: []string{"NL/5b"}, Base: purchases, Receivable: purchases * 21 / 100},
	}
}

func setup(t *testing.T) (*Service, *store, *books) {
	t.Helper()
	first := date("2026-04-10")
	st := &store{entries: map[resource.ID][]journal.Entry{}, sums: map[money.Period][]RateSum{}, first: &first, payments: map[resource.ID]resource.ID{}}
	b := &books{settings: Settings{VATPeriod: "quarter", ICPPeriod: "quarter"}}
	now := func() time.Time { return time.Date(2026, 11, 18, 10, 0, 0, 0, time.UTC) }
	return NewService(admins{}, b, st, resource.NewGenerator(1), now), st, b
}

const admin, user = resource.ID(11), resource.ID(1)

func TestPeriods(t *testing.T) {
	svc, st, _ := setup(t)
	got, err := svc.Periods(context.Background(), admin, user, VATReturn)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].Period != q3 || got[1].Period != q2 || got[0].Granularity != "quarter" {
		t.Fatalf("periods = %+v, want Q3 and Q2 (Q4 has not ended)", got)
	}
	st.sums[q2] = sales(100_000, 0)
	if _, err := svc.Submit(context.Background(), admin, user, Filing{Kind: VATReturn, Period: q2}); err != nil {
		t.Fatal(err)
	}
	if got, _ := svc.Periods(context.Background(), admin, user, VATReturn); len(got) != 1 || got[0].Period != q3 {
		t.Errorf("after Q2 periods = %+v, want Q3", got)
	}
	// A supplement needs something changed in a declared period.
	if got, _ := svc.Periods(context.Background(), admin, user, Supplement); len(got) != 0 {
		t.Errorf("supplement periods without changes = %+v, want none", got)
	}
	st.sums[q2] = sales(120_000, 0)
	if got, _ := svc.Periods(context.Background(), admin, user, Supplement); len(got) != 1 || got[0].Period != q2 {
		t.Errorf("supplement periods = %+v, want Q2", got)
	}
	// An ICP declaration needs supplies to the EU in a period not declared yet.
	if got, _ := svc.Periods(context.Background(), admin, user, ICP); len(got) != 0 {
		t.Errorf("icp periods without supplies = %+v, want none", got)
	}
	st.icpBy = map[money.Period][]ICPLine{q3: {{ContactID: 5, Amount: 1_000}}}
	if got, _ := svc.Periods(context.Background(), admin, user, ICP); len(got) != 1 || got[0].Period != q3 {
		t.Errorf("icp periods = %+v, want Q3", got)
	}
	if _, err := svc.Periods(context.Background(), admin, user, "other"); err == nil {
		t.Error("an unknown kind is accepted")
	}
}

func TestSubmitBooksAndLocks(t *testing.T) {
	svc, st, b := setup(t)
	st.sums[q3] = sales(1_000_050, 200_000)
	r, err := svc.Submit(context.Background(), admin, user, Filing{Kind: VATReturn, Period: q3})
	if err != nil {
		t.Fatal(err)
	}
	// 210010.5 owed rounds to 2100, 42000 input: 1680 to pay.
	if tot := r.Totals(); tot.Total != 168_000 || tot.ExactTotal != 168_010 || r.State() != ToPay {
		t.Errorf("totals = %+v, state %s", tot, r.State())
	}
	if !r.Deadline().Equal(date("2026-10-31")) {
		t.Errorf("deadline = %v", r.Deadline())
	}
	got := map[resource.ID]money.Amount{}
	for _, e := range st.entries[r.ID] {
		got[e.LedgerAccountID] += e.Amount
		if !e.Date.Equal(q3.To) || e.DocumentType != DocumentType || e.Description != "VAT return 2026-07-01 - 2026-09-30" {
			t.Errorf("entry = %+v", e)
		}
	}
	want := map[resource.ID]money.Amount{1510: 210_010, 1520: -42_000, 1530: -168_000, 9000: -10}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("booked %v, want %v", got, want)
	}
	if len(b.locked) != 1 || !b.locked[0].Equal(q3.To) {
		t.Errorf("locked %v, want up to %v", b.locked, q3.To)
	}
	if _, err := svc.Submit(context.Background(), admin, user, Filing{Kind: VATReturn, Period: q3}); err == nil {
		t.Error("a period is declared twice")
	}
}

func TestCorrectionsAndSupplements(t *testing.T) {
	svc, st, _ := setup(t)
	ctx := context.Background()
	st.sums[q2] = sales(100_000, 0)
	first, err := svc.Submit(ctx, admin, user, Filing{Kind: VATReturn, Period: q2})
	if err != nil {
		t.Fatal(err)
	}
	// Q2 turns out 500 more after it was filed.
	st.sums[q2] = sales(150_000, 0)
	st.sums[q3] = sales(200_000, 0)
	draft, err := svc.Preview(ctx, admin, user, VATReturn, q3)
	if err != nil {
		t.Fatal(err)
	}
	if draft.Checks.OpenTransactions != 3 {
		t.Errorf("checks = %+v", draft.Checks)
	}
	if want := []Correction{{Period: q2, Boxes: []Box{{"1a", 50_000, 10_500}}}}; !reflect.DeepEqual(draft.Corrections, want) {
		t.Fatalf("corrections = %+v, want %+v", draft.Corrections, want)
	}
	if draft.Totals().Total != 52_500 {
		t.Errorf("total = %v, want 525", draft.Totals().Total)
	}

	// A supplement declares the change instead; Q3 then has nothing to correct.
	sup, err := svc.Submit(ctx, admin, user, Filing{Kind: Supplement, Period: q2})
	if err != nil {
		t.Fatal(err)
	}
	if want := []Box{{"1a", 50_000, 10_500}}; !reflect.DeepEqual(sup.Boxes, want) || !st.entries[sup.ID][0].Date.Equal(date("2026-11-18")) {
		t.Errorf("supplement = %+v, entries %+v", sup.Boxes, st.entries[sup.ID])
	}
	q3Return, err := svc.Submit(ctx, admin, user, Filing{Kind: VATReturn, Period: q3})
	if err != nil {
		t.Fatal(err)
	}
	if len(q3Return.Corrections) != 0 {
		t.Errorf("corrections after the supplement = %+v", q3Return.Corrections)
	}
	if err := svc.Delete(ctx, admin, first.ID, user); !errors.Is(err, ErrNotDeletable) {
		t.Errorf("deleting a return later ones build on: %v", err)
	}
	if err := svc.Delete(ctx, admin, q3Return.ID, user); err != nil {
		t.Errorf("deleting the latest return: %v", err)
	}
}

func TestICPDeclaration(t *testing.T) {
	svc, st, b := setup(t)
	st.icp = []ICPLine{{ContactID: 5, Name: "Example GmbH", Country: "DE", VATNumber: "DE123456789", Amount: 120_000}, {ContactID: 6, Amount: 5_000}}
	r, err := svc.Submit(context.Background(), admin, user, Filing{Kind: ICP, Period: q3, ICPKinds: map[resource.ID]string{5: Goods}})
	if err != nil {
		t.Fatal(err)
	}
	if r.ICP[0].Kind != Goods || r.ICP[1].Kind != Services || r.State() != Submitted || len(st.entries[r.ID]) != 0 || len(b.locked) != 0 {
		t.Errorf("icp = %+v state %s entries %v locked %v", r.ICP, r.State(), st.entries[r.ID], b.locked)
	}
	if _, err := svc.Submit(context.Background(), admin, user, Filing{Kind: ICP, Period: q2, ICPKinds: map[resource.ID]string{5: "food"}}); err == nil {
		t.Error("an unknown supply kind is accepted")
	}
}

func TestBankPayments(t *testing.T) {
	svc, st, _ := setup(t)
	ctx := context.Background()
	st.sums[q3] = sales(1_000_000, 0)
	r, err := svc.Submit(ctx, admin, user, Filing{Kind: VATReturn, Period: q3})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.RegisterBankPayment(ctx, admin, user, "Document", r.ID, 50, 60, date("2026-10-20"), 210_000, nil); err == nil {
		t.Error("another booking type is accepted")
	}
	if _, err := svc.RegisterBankPayment(ctx, admin, user, DocumentType, r.ID, 50, 60, date("2026-10-20"), -500, nil); err == nil {
		t.Error("money received pays a return to pay")
	}
	id, err := svc.RegisterBankPayment(ctx, admin, user, DocumentType, r.ID, 50, 60, date("2026-10-20"), 300_000, nil)
	if err != nil {
		t.Fatal(err)
	}
	got, _ := svc.Return(ctx, admin, r.ID, user)
	if got.State() != Paid || got.Payments[0].Price != 210_000 {
		t.Errorf("after payment state %s, payments %+v", got.State(), got.Payments)
	}
	if err := svc.Delete(ctx, admin, r.ID, user); !errors.Is(err, ErrNotDeletable) {
		t.Errorf("deleting a paid return: %v", err)
	}
	if err := svc.RemoveBankPayment(ctx, admin, user, id); err != nil {
		t.Fatal(err)
	}
	if got, _ := svc.Return(ctx, admin, r.ID, user); got.State() != ToPay {
		t.Errorf("after removing the payment: %s", got.State())
	}
}
