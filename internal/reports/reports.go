// Package reports works out Moneybird's reports from the journal: the
// profit and loss account, the balance sheet, cash flow, VAT, debtors and
// creditors, revenue and expenses by contact and project, working capital
// and subscriptions; and makes the exports (auditfile, brugstaat, ledger
// accounts) that land in the downloads.
package reports

import (
	"context"
	"errors"
	"slices"
	"time"

	"github.com/adirkuhn/moneyharpy/internal/administration"
	"github.com/adirkuhn/moneyharpy/internal/downloads"
	"github.com/adirkuhn/moneyharpy/internal/journal"
	"github.com/adirkuhn/moneyharpy/internal/money"
	"github.com/adirkuhn/moneyharpy/internal/resource"
	"github.com/adirkuhn/moneyharpy/internal/taxonomy"
)

// ErrLedgerAccountNotFound reports a category the administration does not
// have.
var ErrLedgerAccountNotFound = errors.New("ledger account not found")

// Refusal is a request Moneybird answers with 400: its message and, for
// some, symbolic codes.
type Refusal struct {
	Message  string
	Symbolic map[string]string
}

func (r *Refusal) Error() string { return r.Message }

func refuse(message string) error { return &Refusal{Message: message} }

// Account types, as the books group them.
const (
	NonCurrentAssets      = "non_current_assets"
	CurrentAssets         = "current_assets"
	Equity                = "equity"
	Provisions            = "provisions"
	NonCurrentLiabilities = "non_current_liabilities"
	CurrentLiabilities    = "current_liabilities"
	Revenue               = "revenue"
	DirectCosts           = "direct_costs"
	Expenses              = "expenses"
	OtherIncomeExpenses   = "other_income_expenses"
)

var balanceSheetTypes = []string{NonCurrentAssets, CurrentAssets, Equity, Provisions, NonCurrentLiabilities, CurrentLiabilities}

// OnBalanceSheet reports whether the account type is on the balance sheet.
func OnBalanceSheet(accountType string) bool { return slices.Contains(balanceSheetTypes, accountType) }

// Account is a category as the reports use it.
type Account struct {
	ID       resource.ID
	Name     string
	Type     string
	Code     string
	ParentID *resource.ID
	Active   bool
	Taxonomy string
	// System is the role of a category the books rely on, such as
	// accounts_receivable.
	System string
	// Financial marks the category of a financial account.
	Financial bool
}

// Sum is an amount per category: debits positive, credits negative unless
// a report says otherwise.
type Sum struct {
	LedgerAccountID resource.ID
	Amount          money.Amount
}

// YearResult is the result of the profit and loss categories in a year.
type YearResult struct {
	Year   int
	Amount money.Amount
}

// SubjectQuery selects the entries summed per contact or project: those on
// categories of the types or system roles, from From (nil: since the start)
// up to To.
type SubjectQuery struct {
	From      *time.Time
	To        time.Time
	Types     []string
	Roles     []string
	ContactID *resource.ID
}

// SubjectSum is an amount per contact or project; a nil ID is the entries
// without one.
type SubjectSum struct {
	ID     *resource.ID
	Amount money.Amount
}

// CashSum is what bank transactions booked against the bank on a category.
type CashSum struct {
	LedgerAccountID resource.ID
	// Received and Paid are the entries' sums on the category for money in
	// and money out.
	Received, Paid money.Amount
}

// TaxSum is what a VAT rate was charged on and the VAT on payable and
// receivable VAT, as entries (debits positive).
type TaxSum struct {
	TaxRateID  resource.ID
	Name       string
	Percentage string
	// Type is the rate's type: sales_invoice or purchase_invoice.
	Type                      string
	Boxes                     []string
	Base, Payable, Receivable money.Amount
}

// OpenItem is a booked document on the debtors or creditors: what it posted
// there and what was paid of it by a date.
type OpenItem struct {
	ContactID *resource.ID
	Date      time.Time
	Amount    money.Amount
	Paid      money.Amount
}

// SubscriptionItem is a product a subscription ran.
type SubscriptionItem struct {
	SubscriptionID resource.ID
	ContactID      resource.ID
	Start          time.Time
	End            *time.Time
	CancelledAt    *time.Time
	ItemStart      time.Time
	ItemEnd        *time.Time
	// Quantity in thousandths, discount in hundredths of a percent.
	Quantity int64
	Discount int64
	// Price is the product's price per Every Unit.
	Price money.Amount
	Every int
	Unit  string
}

// Contact is a contact the exports name.
type Contact struct {
	ID         resource.ID
	Name       string
	CustomerID string
	Address    string
	Zipcode    string
	City       string
	Country    string
	TaxNumber  string
}

// Store reads what the reports need.
type Store interface {
	Accounts(ctx context.Context, admin resource.ID) ([]Account, error)
	// Movements sums the entries per category from from (nil: since the
	// start) up to to, on the project when one is given.
	Movements(ctx context.Context, admin resource.ID, from *time.Time, to time.Time, project *resource.ID) ([]Sum, error)
	// YearResults sums the profit and loss entries per calendar year up to
	// to.
	YearResults(ctx context.Context, admin resource.ID, to time.Time) ([]YearResult, error)
	ContactSums(ctx context.Context, admin resource.ID, q SubjectQuery) ([]SubjectSum, error)
	ProjectSums(ctx context.Context, admin resource.ID, q SubjectQuery) ([]SubjectSum, error)
	CashFlow(ctx context.Context, admin resource.ID, p money.Period, account *resource.ID) ([]CashSum, error)
	// CashBalance is the balance of the financial accounts' categories at
	// the end of the day.
	CashBalance(ctx context.Context, admin resource.ID, at time.Time, account *resource.ID) (money.Amount, error)
	TaxSums(ctx context.Context, admin resource.ID, p money.Period) ([]TaxSum, error)
	// OpenItems lists the booked documents on the category of the system
	// role up to the day.
	OpenItems(ctx context.Context, admin resource.ID, role string, until time.Time) ([]OpenItem, error)
	Subscriptions(ctx context.Context, admin resource.ID) ([]SubscriptionItem, error)
	// LedgerEntries returns a page of a category's entries in the period,
	// the newest first, and their number in all.
	LedgerEntries(ctx context.Context, admin, ledger resource.ID, p money.Period, page, perPage int) ([]journal.Entry, int, error)
	// Entries returns every entry in the period by date and document.
	Entries(ctx context.Context, admin resource.ID, p money.Period) ([]journal.Entry, error)
	Contacts(ctx context.Context, admin resource.ID, ids []resource.ID) ([]Contact, error)
}

// Administrations checks access to an administration.
type Administrations interface {
	Get(ctx context.Context, id, user resource.ID) (administration.Listed, error)
}

// Downloads keeps the files exports make.
type Downloads interface {
	Add(ctx context.Context, admin resource.ID, user *resource.ID, downloadType, filename, contentType string, data []byte) (downloads.Download, error)
	AddFailed(ctx context.Context, admin resource.ID, user *resource.ID, downloadType, filename string) (downloads.Download, error)
}

// Schemes gives the chart scheme categories map to, nil for none.
type Schemes interface {
	Scheme(ctx context.Context, admin, user resource.ID) (*taxonomy.Scheme, error)
}

// Service implements the reports.
type Service struct {
	admins    Administrations
	store     Store
	downloads Downloads
	schemes   Schemes
	now       func() time.Time
}

// NewService returns a Service.
func NewService(admins Administrations, store Store, files Downloads, schemes Schemes, now func() time.Time) *Service {
	return &Service{admins: admins, store: store, downloads: files, schemes: schemes, now: now}
}

// MaxMonths bounds a report's period.
const MaxMonths = 12

// Paging of the reports that page, as at Moneybird.
const (
	DefaultPerPage = 100
	MaxPerPage     = 100
)

// today is the date in the administration's time zone.
func (s *Service) today(a administration.Listed) time.Time {
	now := s.now()
	if loc, err := time.LoadLocation(a.TimeZone); err == nil {
		now = now.In(loc)
	}
	y, m, d := now.Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

// window is a report's period and its parts by granularity.
type window struct {
	admin  administration.Listed
	period money.Period
	parts  []money.Period
}

// open checks access and reads the period (this month unless given, whole
// months, at most 12) and the granularity (none, month, quarter or year).
func (s *Service) open(ctx context.Context, admin, user resource.ID, period, granularity string) (window, error) {
	a, err := s.admins.Get(ctx, admin, user)
	if err != nil {
		return window{}, err
	}
	p, err := money.ParseReportPeriod(period, s.today(a))
	if err != nil {
		return window{}, refuse("Period is invalid")
	}
	if p.Months() > MaxMonths {
		return window{}, refuse("Period cannot exceed 12 months")
	}
	w := window{admin: a, period: p}
	if granularity != "" {
		if w.parts, err = p.Split(granularity); err != nil {
			return window{}, refuse("granularity does not have a valid value")
		}
	}
	return w, nil
}

// pageOf normalises paging.
func pageOf(page, perPage int) (int, int) {
	page = max(page, 1)
	if perPage <= 0 {
		perPage = DefaultPerPage
	}
	return page, min(perPage, MaxPerPage)
}

// slice returns the page of items.
func slice[T any](items []T, page, perPage int) []T {
	from := min((page-1)*perPage, len(items))
	return items[from:min(from+perPage, len(items))]
}

func ptr[T any](v T) *T { return &v }

// dayBefore is the last day before t.
func dayBefore(t time.Time) time.Time { return t.AddDate(0, 0, -1) }
