package reports

import (
	"cmp"
	"context"
	"fmt"
	"regexp"
	"slices"
	"time"

	"github.com/adirkuhn/moneyharpy/internal/money"
	"github.com/adirkuhn/moneyharpy/internal/resource"
)

// SubjectPeriod is the amounts per contact or project in a part of the
// report's period.
type SubjectPeriod struct {
	Period money.Period
	Items  []SubjectSum
}

// SubjectReport is a page of amounts per contact or project, the number of
// them in all and, with a granularity, the amounts of the page's contacts
// or projects in each part of the period.
type SubjectReport struct {
	Items   []SubjectSum
	Total   int
	Periods []SubjectPeriod
}

// Paging selects a page of a report.
type Paging struct{ Page, PerPage int }

// subjectReport describes a report per contact or project.
type subjectReport struct {
	project bool
	types   []string
	roles   []string
	// sign turns entries into the report's amounts.
	sign money.Amount
	// balance reports the balance at the end of each period instead of
	// the movements in it.
	balance bool
	contact *resource.ID
}

var (
	costTypes = []string{DirectCosts, Expenses}
	debtors   = subjectReport{roles: []string{"accounts_receivable"}, sign: 1, balance: true}
	creditors = subjectReport{roles: []string{"accounts_payable"}, sign: -1, balance: true}
)

// Debtors reports the balance of each debtor at the end of the period: what
// is still to receive on the debtors category.
func (s *Service) Debtors(ctx context.Context, admin, user resource.ID, period, granularity string, pg Paging) (SubjectReport, error) {
	return s.subjects(ctx, admin, user, period, granularity, pg, debtors)
}

// Creditors reports the balance of each creditor at the end of the period.
func (s *Service) Creditors(ctx context.Context, admin, user resource.ID, period, granularity string, pg Paging) (SubjectReport, error) {
	return s.subjects(ctx, admin, user, period, granularity, pg, creditors)
}

// RevenueByContact reports the revenue booked per contact in the period:
// credit minus debit on revenue categories.
func (s *Service) RevenueByContact(ctx context.Context, admin, user resource.ID, period, granularity string, contact *resource.ID, pg Paging) (SubjectReport, error) {
	return s.subjects(ctx, admin, user, period, granularity, pg, subjectReport{types: []string{Revenue}, sign: -1, contact: contact})
}

// ExpensesByContact reports the costs booked per contact in the period:
// debit minus credit on direct costs and expenses.
func (s *Service) ExpensesByContact(ctx context.Context, admin, user resource.ID, period, granularity string, contact *resource.ID, pg Paging) (SubjectReport, error) {
	return s.subjects(ctx, admin, user, period, granularity, pg, subjectReport{types: costTypes, sign: 1, contact: contact})
}

// RevenueByProject reports the revenue booked per project in the period.
func (s *Service) RevenueByProject(ctx context.Context, admin, user resource.ID, period, granularity string, pg Paging) (SubjectReport, error) {
	return s.subjects(ctx, admin, user, period, granularity, pg, subjectReport{project: true, types: []string{Revenue}, sign: -1})
}

// ExpensesByProject reports the costs booked per project in the period.
func (s *Service) ExpensesByProject(ctx context.Context, admin, user resource.ID, period, granularity string, pg Paging) (SubjectReport, error) {
	return s.subjects(ctx, admin, user, period, granularity, pg, subjectReport{project: true, types: costTypes, sign: 1})
}

func (s *Service) subjects(ctx context.Context, admin, user resource.ID, period, granularity string, pg Paging, r subjectReport) (SubjectReport, error) {
	w, err := s.open(ctx, admin, user, period, granularity)
	if err != nil {
		return SubjectReport{}, err
	}
	sums := func(p money.Period) ([]SubjectSum, error) {
		q := SubjectQuery{To: p.To, Types: nonNil(r.types), Roles: nonNil(r.roles), ContactID: r.contact}
		if !r.balance {
			q.From = &p.From
		}
		var list []SubjectSum
		var err error
		if r.project {
			list, err = s.store.ProjectSums(ctx, admin, q)
		} else {
			list, err = s.store.ContactSums(ctx, admin, q)
		}
		if err != nil {
			return nil, fmt.Errorf("sum journal: %w", err)
		}
		for i := range list {
			list[i].Amount *= r.sign
		}
		return list, nil
	}
	all, err := sums(w.period)
	if err != nil {
		return SubjectReport{}, err
	}
	all = slices.DeleteFunc(all, func(x SubjectSum) bool { return x.Amount == 0 })
	if r.balance {
		// The largest balances first.
		slices.SortFunc(all, func(a, b SubjectSum) int { return cmp.Or(cmp.Compare(b.Amount, a.Amount), compareSubject(a.ID, b.ID)) })
	} else {
		// The bookings without a contact or project first.
		slices.SortFunc(all, func(a, b SubjectSum) int {
			return cmp.Or(cmp.Compare(boolInt(a.ID != nil), boolInt(b.ID != nil)), cmp.Compare(b.Amount, a.Amount), compareSubject(a.ID, b.ID))
		})
	}
	page, perPage := pageOf(pg.Page, pg.PerPage)
	out := SubjectReport{Items: slice(all, page, perPage), Total: len(all)}
	for _, p := range w.parts {
		list, err := sums(p)
		if err != nil {
			return SubjectReport{}, err
		}
		got := map[resource.ID]money.Amount{}
		var none *money.Amount
		for _, x := range list {
			if x.ID == nil {
				none = ptr(x.Amount)
			} else {
				got[*x.ID] = x.Amount
			}
		}
		part := SubjectPeriod{Period: p, Items: []SubjectSum{}}
		for _, x := range out.Items {
			amount, ok := money.Amount(0), false
			if x.ID == nil {
				if none != nil {
					amount, ok = *none, true
				}
			} else {
				amount, ok = got[*x.ID]
			}
			// Balances show at the end of every part; movements only in
			// the parts with bookings.
			if r.balance || ok && amount != 0 {
				part.Items = append(part.Items, SubjectSum{ID: x.ID, Amount: amount})
			}
		}
		out.Periods = append(out.Periods, part)
	}
	return out, nil
}

func nonNil(v []string) []string {
	if v == nil {
		return []string{}
	}
	return v
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func compareSubject(a, b *resource.ID) int {
	switch {
	case a == nil && b == nil:
		return 0
	case a == nil:
		return -1
	case b == nil:
		return 1
	}
	return cmp.Compare(*a, *b)
}

// Bucket is part of a contact's open amount by age.
type Bucket struct {
	Name   string
	Amount money.Amount
}

// AgingItem is what a contact had open at a date, by age: 0 to 30 days, 30
// to 60 days, older, and the total.
type AgingItem struct {
	ContactID *resource.ID
	Amount    money.Amount
	Buckets   [4]Bucket
}

// AgingReport is a page of the aging report and the number of contacts in
// all.
type AgingReport struct {
	Items []AgingItem
	Total int
}

var monthPattern = regexp.MustCompile(`^\d{6}$`)

// until reads period_until: a month (202501), a day (20250115) or a preset
// such as this_month or prev_quarter; the report looks at its last day, or
// today when that is still to come.
func (s *Service) until(ctx context.Context, admin, user resource.ID, v string) (time.Time, string, error) {
	a, err := s.admins.Get(ctx, admin, user)
	if err != nil {
		return time.Time{}, "", err
	}
	today := s.today(a)
	var day time.Time
	switch {
	case v == "":
		day = endOfMonth(today)
	case len(v) == 8:
		if day, err = time.Parse("20060102", v); err != nil {
			return time.Time{}, "", refuse("Period is invalid")
		}
	case monthPattern.MatchString(v) || slices.Contains([]string{"this_month", "prev_month", "this_quarter", "prev_quarter", "this_year", "prev_year"}, v):
		p, err := money.ParseReportPeriod(v, today)
		if err != nil {
			return time.Time{}, "", refuse("Period is invalid")
		}
		day = p.To
	default:
		return time.Time{}, "", refuse("Period is invalid")
	}
	if day.After(today) {
		day = today
	}
	return day, a.Language, nil
}

// DebtorsAging reports per debtor what open sales invoices had open at the
// end of period_until, by the age of the invoice.
func (s *Service) DebtorsAging(ctx context.Context, admin, user resource.ID, periodUntil string, pg Paging) (AgingReport, error) {
	return s.aging(ctx, admin, user, periodUntil, pg, "accounts_receivable", 1)
}

// CreditorsAging reports per creditor what open purchase invoices and
// receipts had open at the end of period_until, by their age.
func (s *Service) CreditorsAging(ctx context.Context, admin, user resource.ID, periodUntil string, pg Paging) (AgingReport, error) {
	return s.aging(ctx, admin, user, periodUntil, pg, "accounts_payable", -1)
}

// bucketNames are the aging buckets' names per language.
var bucketNames = map[string][4]string{
	"nl": {"0 - 30 dagen", "30 - 60 dagen", "Ouder", "Totaal"},
	"en": {"0 - 30 days", "30 - 60 days", "Older", "Total"},
}

func (s *Service) aging(ctx context.Context, admin, user resource.ID, periodUntil string, pg Paging, role string, sign money.Amount) (AgingReport, error) {
	day, language, err := s.until(ctx, admin, user, periodUntil)
	if err != nil {
		return AgingReport{}, err
	}
	items, err := s.store.OpenItems(ctx, admin, role, day)
	if err != nil {
		return AgingReport{}, fmt.Errorf("open items: %w", err)
	}
	names := bucketNames["nl"] // also for nl-be
	if language == "en" {
		names = bucketNames["en"]
	}
	type key struct {
		set bool
		id  resource.ID
	}
	byContact := map[key]*AgingItem{}
	var order []key
	for _, it := range items {
		open := sign*it.Amount - it.Paid
		if open == 0 {
			continue
		}
		k := key{}
		if it.ContactID != nil {
			k = key{true, *it.ContactID}
		}
		row := byContact[k]
		if row == nil {
			row = &AgingItem{ContactID: it.ContactID}
			for i := range row.Buckets {
				row.Buckets[i].Name = names[i]
			}
			byContact[k] = row
			order = append(order, k)
		}
		bucket := 2
		switch age := int(day.Sub(it.Date).Hours() / 24); {
		case age <= 30:
			bucket = 0
		case age <= 60:
			bucket = 1
		}
		row.Buckets[bucket].Amount += open
		row.Buckets[3].Amount += open
		row.Amount += open
	}
	all := []AgingItem{}
	for _, k := range order {
		if r := byContact[k]; r.Amount != 0 {
			all = append(all, *r)
		}
	}
	slices.SortStableFunc(all, func(a, b AgingItem) int {
		return cmp.Or(cmp.Compare(b.Amount, a.Amount), compareSubject(a.ContactID, b.ContactID))
	})
	page, perPage := pageOf(pg.Page, pg.PerPage)
	return AgingReport{Items: slice(all, page, perPage), Total: len(all)}, nil
}
