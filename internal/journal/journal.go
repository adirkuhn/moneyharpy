// Package journal is the administration's general ledger in double entry:
// every document that changes the books posts entries whose debits and
// credits balance. Bank transactions, invoices and journal entries (general
// journal documents) post here; the reports read it.
package journal

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/adirkuhn/moneyharpy/internal/administration"
	"github.com/adirkuhn/moneyharpy/internal/money"
	"github.com/adirkuhn/moneyharpy/internal/projects"
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

// Projects gives the projects entries are booked on.
type Projects interface {
	Project(ctx context.Context, admin, id, user resource.ID) (projects.Project, error)
}

// Contacts checks the contacts entries name.
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
	// DocumentType and DocumentID select one document's entries, a
	// MoneyHarpy extension; without a period they cover every date.
	DocumentType  string
	DocumentID    *resource.ID
	Page, PerPage int
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
	projects    Projects
	store       Store
	documents   DocumentStore
	attachments Attachments
	notes       Notes
	ids         *resource.Generator
	now         func() time.Time
}

// NewService returns a Service.
func NewService(admins Administrations, ledger Ledger, contacts Contacts, projects Projects, store Store, documents DocumentStore, notes Notes, attachments Attachments, ids *resource.Generator, now func() time.Time) *Service {
	return &Service{admins: admins, ledger: ledger, contacts: contacts, projects: projects, store: store, documents: documents, notes: notes, attachments: attachments, ids: ids, now: now}
}

// today is the date in the administration's time zone.
func today(a administration.Listed, now time.Time) time.Time {
	if loc, err := time.LoadLocation(a.TimeZone); err == nil {
		now = now.In(loc)
	}
	return now
}

// ParseReportPeriod reads a report period; see money.ParseReportPeriod.
func ParseReportPeriod(s string, now time.Time) (money.Period, error) {
	return money.ParseReportPeriod(s, now)
}

// Granularities split a report into periods.
var Granularities = money.Granularities

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
	parts, err := p.Split(granularity)
	if err != nil {
		return GeneralLedgerReport{}, invalid("granularity", err.Error())
	}
	for _, part := range parts {
		partSums, err := s.store.Sums(ctx, admin, part)
		if err != nil {
			return GeneralLedgerReport{}, fmt.Errorf("sum journal: %w", err)
		}
		out.Periods = append(out.Periods, LedgerPeriod{Period: part, Sums: active(partSums)})
	}
	return out, nil
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
	hasDocument := q.DocumentID != nil
	switch {
	case hasDocument && q.DocumentType == "":
		return nil, 0, invalid("document_type", "can't be blank with a document_id")
	case !hasDocument && q.DocumentType != "":
		return nil, 0, invalid("document_id", "can't be blank with a document_type")
	}
	if hasDocument && period == "" {
		q.Period = money.Period{From: time.Date(1, 1, 1, 0, 0, 0, 0, time.UTC), To: time.Date(9999, 12, 31, 0, 0, 0, 0, time.UTC)}
	} else if q.Period, err = ParseReportPeriod(period, today(a, s.now())); err != nil {
		return nil, 0, invalid("period", err.Error())
	}
	hasContact, hasProject := q.ContactID != nil || q.NoContact, q.ProjectID != nil || q.NoProject
	switch {
	case q.LedgerAccountID == nil && !hasContact && !hasProject && !hasDocument:
		return nil, 0, invalid("base", "Give a ledger_account_id, contact_id, project_id or document_id")
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
