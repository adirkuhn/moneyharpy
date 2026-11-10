// Package journal is the administration's general ledger in double entry:
// every document that changes the books posts entries whose debits and
// credits balance. Bank transactions, invoices and journal entries (general
// journal documents) post here; the reports read it.
package journal

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/adirkuhn/moneyharpy/internal/administration"
	"github.com/adirkuhn/moneyharpy/internal/money"
	"github.com/adirkuhn/moneyharpy/internal/resource"
)

// Document types, as Moneybird names them in reports.
const (
	FinancialMutation      = "FinancialMutation"
	GeneralJournalDocument = "GeneralJournalDocument"
	SalesInvoice           = "SalesInvoice"
	ExternalSalesInvoice   = "ExternalSalesInvoice"
	Payment                = "Payment"
)

// ErrUnbalanced reports entries whose debits and credits differ.
var ErrUnbalanced = errors.New("journal entries do not balance")

// Entry is one line of the journal: an amount on a category. Debits are
// positive, credits negative.
type Entry struct {
	ID                 resource.ID
	AdministrationID   resource.ID
	DocumentType       string
	DocumentID         resource.ID
	Date               time.Time
	LedgerAccountID    resource.ID
	ContactID          *resource.ID
	ProjectID          *resource.ID
	TaxRateID          *resource.ID
	FinancialAccountID *resource.ID
	Amount             money.Amount
	Description        string
	RowOrder           int
}

// Debit is the entry's debit side, or zero.
func (e Entry) Debit() money.Amount { return max(e.Amount, 0) }

// Credit is the entry's credit side as a positive amount, or zero.
func (e Entry) Credit() money.Amount { return max(-e.Amount, 0) }

// Balance checks that entries add up to zero.
func Balance(entries []Entry) error {
	var sum money.Amount
	for _, e := range entries {
		sum += e.Amount
	}
	if sum != 0 {
		return fmt.Errorf("%w: off by %s", ErrUnbalanced, sum)
	}
	return nil
}

// Administrations gives access to administrations; Get returns
// administration.ErrNotFound when the user has no access.
type Administrations interface {
	Get(ctx context.Context, id, user resource.ID) (administration.Listed, error)
}

// Ledger is the part of bookkeeping the journal relies on.
type Ledger interface {
	// Bookable returns why a category cannot be used on the document
	// type, or "".
	Bookable(ctx context.Context, admin, user, id resource.ID, documentType string) (string, error)
	// LockedUntil returns the last day of the locked period, or "".
	LockedUntil(ctx context.Context, admin, user resource.ID) (string, error)
}

// Contacts checks the contacts entries refer to.
type Contacts interface {
	// ContactExists returns an error unless the contact is the
	// administration's.
	ContactExists(ctx context.Context, admin, id, user resource.ID) error
}

// AccountSum is one category's movements in a period, and its balances at
// the period's ends.
type AccountSum struct {
	LedgerAccountID resource.ID
	Debit, Credit   money.Amount
	Start, Final    money.Amount
}

// EntryQuery selects entries for the journal entries report.
type EntryQuery struct {
	Period money.Period
	// One of these is set; a zero id with NoContact or NoProject selects
	// entries without one.
	LedgerAccountID *resource.ID
	ContactID       *resource.ID
	NoContact       bool
	ProjectID       *resource.ID
	NoProject       bool
	AccountType     string
	Page, PerPage   int
}

// Store reads the journal.
type Store interface {
	// Entries returns one page of the entries the query selects, oldest
	// first, with their number in all.
	Entries(ctx context.Context, admin resource.ID, q EntryQuery) ([]Entry, int, error)
	// Sums returns every category's debits and credits in the period, and
	// its balance at the start. Profit and loss categories start each
	// calendar year at zero.
	Sums(ctx context.Context, admin resource.ID, p money.Period) ([]AccountSum, error)
}

// Service implements the journal use cases.
type Service struct {
	admins      Administrations
	ledger      Ledger
	contacts    Contacts
	store       Store
	documents   DocumentStore
	attachments Attachments
	notes       Notes
	ids         *resource.Generator
	now         func() time.Time
}

// NewService returns a Service.
func NewService(admins Administrations, ledger Ledger, contacts Contacts, store Store, documents DocumentStore, notes Notes, attachments Attachments, ids *resource.Generator, now func() time.Time) *Service {
	return &Service{admins: admins, ledger: ledger, contacts: contacts, store: store, documents: documents, notes: notes, attachments: attachments, ids: ids, now: now}
}

// today is the date in the administration's time zone.
func today(a administration.Listed, now time.Time) time.Time {
	if loc, err := time.LoadLocation(a.TimeZone); err == nil {
		now = now.In(loc)
	}
	return now
}

var monthPattern = regexp.MustCompile(`^\d{6}$`)

// ParseReportPeriod reads a report period: a name such as this_month, a
// month such as 202501 or a range of whole months. Empty means this month.
func ParseReportPeriod(s string, now time.Time) (money.Period, error) {
	s = strings.TrimSpace(s)
	switch {
	case s == "":
		s = "this_month"
	case monthPattern.MatchString(s):
		s += ".." + s
	}
	p, err := money.ParsePeriod(s, now)
	if err != nil {
		return p, err
	}
	if p.From.Day() != 1 || p.To.AddDate(0, 0, 1).Day() != 1 {
		return p, errors.New("must cover whole months")
	}
	return p, nil
}

// Granularities split a report into periods.
var Granularities = []string{"month", "quarter", "year"}

// LedgerPeriod is the general ledger over part of the report's period.
type LedgerPeriod struct {
	Period money.Period
	Sums   []AccountSum
}

// GeneralLedgerReport is every category's movements and balances.
type GeneralLedgerReport struct {
	Sums    []AccountSum
	Periods []LedgerPeriod
}

// GeneralLedger reports the movements and balances of every category in
// the period, split by granularity when one is given.
func (s *Service) GeneralLedger(ctx context.Context, admin, user resource.ID, period, granularity string) (GeneralLedgerReport, error) {
	a, err := s.admins.Get(ctx, admin, user)
	if err != nil {
		return GeneralLedgerReport{}, err
	}
	p, err := ParseReportPeriod(period, today(a, s.now()))
	if err != nil {
		return GeneralLedgerReport{}, invalid("period", err.Error())
	}
	sums, err := s.store.Sums(ctx, admin, p)
	if err != nil {
		return GeneralLedgerReport{}, fmt.Errorf("sum journal: %w", err)
	}
	out := GeneralLedgerReport{Sums: active(sums)}
	if granularity == "" {
		return out, nil
	}
	months := map[string]int{"month": 1, "quarter": 3, "year": 12}[granularity]
	if months == 0 {
		return GeneralLedgerReport{}, invalid("granularity", "is not one of month, quarter, year")
	}
	for from := p.From; !from.After(p.To); {
		next := alignedNext(from, months)
		part := money.Period{From: from, To: minTime(next.AddDate(0, 0, -1), p.To)}
		partSums, err := s.store.Sums(ctx, admin, part)
		if err != nil {
			return GeneralLedgerReport{}, fmt.Errorf("sum journal: %w", err)
		}
		out.Periods = append(out.Periods, LedgerPeriod{Period: part, Sums: active(partSums)})
		from = next
	}
	return out, nil
}

// alignedNext is the start of the month, quarter or year after the one
// holding day.
func alignedNext(day time.Time, months int) time.Time {
	m := (int(day.Month())-1)/months*months + months
	return time.Date(day.Year(), time.Month(m+1), 1, 0, 0, 0, 0, time.UTC)
}

func minTime(a, b time.Time) time.Time {
	if a.Before(b) {
		return a
	}
	return b
}

// active keeps the categories with a balance or bookings.
func active(sums []AccountSum) []AccountSum {
	out := []AccountSum{}
	for _, s := range sums {
		if s.Debit != 0 || s.Credit != 0 || s.Start != 0 || s.Final != 0 {
			out = append(out, s)
		}
	}
	return out
}

// Report page limits.
const (
	DefaultPerPage = 100
	MaxPerPage     = 100
)

// Entries reports the journal entries the query selects.
func (s *Service) Entries(ctx context.Context, admin, user resource.ID, period string, q EntryQuery) ([]Entry, int, error) {
	a, err := s.admins.Get(ctx, admin, user)
	if err != nil {
		return nil, 0, err
	}
	if q.Period, err = ParseReportPeriod(period, today(a, s.now())); err != nil {
		return nil, 0, invalid("period", err.Error())
	}
	hasContact, hasProject := q.ContactID != nil || q.NoContact, q.ProjectID != nil || q.NoProject
	switch {
	case q.LedgerAccountID == nil && !hasContact && !hasProject:
		return nil, 0, invalid("base", "Give a ledger_account_id, contact_id or project_id")
	case q.AccountType != "" && !hasContact && !hasProject:
		return nil, 0, invalid("account_type", "needs a contact_id or project_id")
	}
	q.Page = max(q.Page, 1)
	if q.PerPage <= 0 {
		q.PerPage = DefaultPerPage
	}
	q.PerPage = min(q.PerPage, MaxPerPage)
	return s.store.Entries(ctx, admin, q)
}

func invalid(field, problem string) error {
	var verr resource.ValidationError
	verr.Add(field, problem)
	return &verr
}
