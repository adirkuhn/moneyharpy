// Package audits keeps Moneybird's audit report: per period, which
// categories have been checked, by whom and when, and whether their
// bookings changed after.
package audits

import (
	"cmp"
	"context"
	"errors"
	"slices"
	"time"

	"github.com/adirkuhn/moneyharpy/internal/administration"
	"github.com/adirkuhn/moneyharpy/internal/money"
	"github.com/adirkuhn/moneyharpy/internal/resource"
)

// ErrInvalid is a request the audit report cannot answer, such as an
// unknown period.
var ErrInvalid = errors.New("invalid")

// ErrNotFound is a category the administration does not have.
var ErrNotFound = errors.New("not found")

// Audit marks a category as checked for a period, with what was booked on it
// then.
type Audit struct {
	ID              resource.ID
	LedgerAccountID resource.ID
	Period          money.Period
	Debit, Credit   money.Amount
	UserID          resource.ID
	AuditedAt       time.Time
}

// Sums is what was booked on a category in a period, and on which VAT rates.
type Sums struct {
	LedgerAccountID resource.ID
	Debit, Credit   money.Amount
	TaxRateIDs      []resource.ID
}

// Line is a category of the report: what was booked on it, and its audit
// with whether the bookings changed since.
type Line struct {
	Sums
	Audit   *Audit
	Changed bool
}

// Report is the audit report of a period.
type Report struct {
	Period money.Period
	Lines  []Line
}

// Store keeps the audits.
type Store interface {
	Sums(ctx context.Context, admin resource.ID, p money.Period) ([]Sums, error)
	Audits(ctx context.Context, admin resource.ID, p money.Period) ([]Audit, error)
	// Save stores the audit, replacing an earlier one of the category and period.
	Save(ctx context.Context, admin resource.ID, a Audit) error
	Delete(ctx context.Context, admin, ledger resource.ID, p money.Period) error
	// HasAccount says whether the category is the administration's.
	HasAccount(ctx context.Context, admin, ledger resource.ID) (bool, error)
}

// Administrations checks access to an administration.
type Administrations interface {
	Get(ctx context.Context, id, user resource.ID) (administration.Listed, error)
}

// Service is the audit report.
type Service struct {
	admins Administrations
	store  Store
	ids    *resource.Generator
	now    func() time.Time
}

// NewService returns the audit report on the store.
func NewService(admins Administrations, store Store, ids *resource.Generator, now func() time.Time) *Service {
	return &Service{admins: admins, store: store, ids: ids, now: now}
}

// period reads a report period in the administration's time zone.
func (s *Service) period(ctx context.Context, admin, user resource.ID, raw string) (money.Period, error) {
	a, err := s.admins.Get(ctx, admin, user)
	if err != nil {
		return money.Period{}, err
	}
	today := s.now()
	if loc, err := time.LoadLocation(a.TimeZone); err == nil {
		today = today.In(loc)
	}
	p, err := money.ParseReportPeriod(raw, today)
	if err != nil {
		return money.Period{}, ErrInvalid
	}
	return p, nil
}

// Report lists the categories with bookings in the period or an audit of
// it, each with its audit and whether its bookings changed since.
func (s *Service) Report(ctx context.Context, admin, user resource.ID, period string) (Report, error) {
	p, err := s.period(ctx, admin, user, period)
	if err != nil {
		return Report{}, err
	}
	sums, err := s.store.Sums(ctx, admin, p)
	if err != nil {
		return Report{}, err
	}
	audits, err := s.store.Audits(ctx, admin, p)
	if err != nil {
		return Report{}, err
	}
	lines := map[resource.ID]*Line{}
	for _, x := range sums {
		lines[x.LedgerAccountID] = &Line{Sums: x}
	}
	for _, a := range audits {
		l := lines[a.LedgerAccountID]
		if l == nil {
			l = &Line{Sums: Sums{LedgerAccountID: a.LedgerAccountID, TaxRateIDs: []resource.ID{}}}
			lines[a.LedgerAccountID] = l
		}
		l.Audit = &a
		l.Changed = a.Debit != l.Debit || a.Credit != l.Credit
	}
	out := Report{Period: p, Lines: make([]Line, 0, len(lines))}
	for _, l := range lines {
		out.Lines = append(out.Lines, *l)
	}
	slices.SortFunc(out.Lines, func(a, b Line) int { return cmp.Compare(a.LedgerAccountID, b.LedgerAccountID) })
	return out, nil
}

// Mark marks the category as audited for the period by the user, with what
// is booked on it now.
func (s *Service) Mark(ctx context.Context, admin, user, ledger resource.ID, period string) (Line, error) {
	p, err := s.period(ctx, admin, user, period)
	if err != nil {
		return Line{}, err
	}
	if ok, err := s.store.HasAccount(ctx, admin, ledger); err != nil || !ok {
		return Line{}, errors.Join(ErrNotFound, err)
	}
	sums, err := s.store.Sums(ctx, admin, p)
	if err != nil {
		return Line{}, err
	}
	line := Line{Sums: Sums{LedgerAccountID: ledger, TaxRateIDs: []resource.ID{}}}
	if i := slices.IndexFunc(sums, func(x Sums) bool { return x.LedgerAccountID == ledger }); i >= 0 {
		line.Sums = sums[i]
	}
	a := Audit{ID: s.ids.Next(), LedgerAccountID: ledger, Period: p, Debit: line.Debit, Credit: line.Credit, UserID: user, AuditedAt: s.now()}
	if err := s.store.Save(ctx, admin, a); err != nil {
		return Line{}, err
	}
	line.Audit = &a
	return line, nil
}

// Unmark takes the audit of the category for the period back.
func (s *Service) Unmark(ctx context.Context, admin, user, ledger resource.ID, period string) error {
	p, err := s.period(ctx, admin, user, period)
	if err != nil {
		return err
	}
	return s.store.Delete(ctx, admin, ledger, p)
}
