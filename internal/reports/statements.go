package reports

import (
	"cmp"
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/adirkuhn/moneyharpy/internal/journal"
	"github.com/adirkuhn/moneyharpy/internal/money"
	"github.com/adirkuhn/moneyharpy/internal/resource"
)

// accountIndex finds categories by id.
type accountIndex map[resource.ID]Account

func (s *Service) accounts(ctx context.Context, admin resource.ID) ([]Account, accountIndex, error) {
	list, err := s.store.Accounts(ctx, admin)
	if err != nil {
		return nil, nil, fmt.Errorf("list ledger accounts: %w", err)
	}
	index := accountIndex{}
	for _, a := range list {
		index[a.ID] = a
	}
	return list, index, nil
}

// byCode orders sums by their category's code and name.
func (ix accountIndex) byCode(sums []Sum) {
	slices.SortStableFunc(sums, func(a, b Sum) int {
		x, y := ix[a.LedgerAccountID], ix[b.LedgerAccountID]
		return cmp.Or(cmp.Compare(x.Code, y.Code), cmp.Compare(x.Name, y.Name), cmp.Compare(x.ID, y.ID))
	})
}

func total(sums []Sum) money.Amount {
	var t money.Amount
	for _, s := range sums {
		t += s.Amount
	}
	return t
}

// ProfitLoss is the profit and loss account over a period. Revenue and
// Other are credit minus debit, the costs debit minus credit.
type ProfitLoss struct {
	Period                                 money.Period
	Revenue, DirectCosts, Expenses, Other  []Sum
	GrossProfit, OperatingProfit           money.Amount
	NetProfit, TotalExpenses, TotalRevenue money.Amount
}

// ProfitLossReport is the profit and loss account over the period and,
// with a granularity, over each part of it.
type ProfitLossReport struct {
	ProfitLoss
	Periods []ProfitLoss
}

// ProfitLoss reports the profit and loss categories with bookings in the
// period, on the project when one is given. Nested categories are flat.
func (s *Service) ProfitLoss(ctx context.Context, admin, user resource.ID, period, granularity string, project *resource.ID) (ProfitLossReport, error) {
	w, err := s.open(ctx, admin, user, period, granularity)
	if err != nil {
		return ProfitLossReport{}, err
	}
	_, index, err := s.accounts(ctx, admin)
	if err != nil {
		return ProfitLossReport{}, err
	}
	at := func(p money.Period) (ProfitLoss, error) {
		sums, err := s.store.Movements(ctx, admin, &p.From, p.To, project)
		if err != nil {
			return ProfitLoss{}, fmt.Errorf("sum journal: %w", err)
		}
		out := ProfitLoss{Period: p, Revenue: []Sum{}, DirectCosts: []Sum{}, Expenses: []Sum{}, Other: []Sum{}}
		for _, sum := range sums {
			switch index[sum.LedgerAccountID].Type {
			case Revenue:
				out.Revenue = append(out.Revenue, Sum{sum.LedgerAccountID, -sum.Amount})
			case DirectCosts:
				out.DirectCosts = append(out.DirectCosts, sum)
			case Expenses:
				out.Expenses = append(out.Expenses, sum)
			case OtherIncomeExpenses:
				out.Other = append(out.Other, Sum{sum.LedgerAccountID, -sum.Amount})
			}
		}
		for _, list := range [][]Sum{out.Revenue, out.DirectCosts, out.Expenses, out.Other} {
			index.byCode(list)
		}
		out.TotalRevenue = total(out.Revenue)
		out.GrossProfit = out.TotalRevenue - total(out.DirectCosts)
		out.OperatingProfit = out.GrossProfit - total(out.Expenses)
		out.TotalExpenses = total(out.DirectCosts) + total(out.Expenses) - total(out.Other)
		out.NetProfit = out.TotalRevenue - out.TotalExpenses
		return out, nil
	}
	report := ProfitLossReport{}
	if report.ProfitLoss, err = at(w.period); err != nil {
		return ProfitLossReport{}, err
	}
	for _, p := range w.parts {
		part, err := at(p)
		if err != nil {
			return ProfitLossReport{}, err
		}
		report.Periods = append(report.Periods, part)
	}
	return report, nil
}

// Line is a category on the balance sheet with its value and, for a top
// category, the values of the categories below it.
type Line struct {
	LedgerAccountID resource.ID
	Value           money.Amount
	Children        []Line
}

// BalanceSheet is the financial position at the end of a period. Sections
// holds the lines per account type; values are debit balances on the
// asset side and credit balances on the other.
type BalanceSheet struct {
	Period      money.Period
	Sections    map[string][]Line
	DebitTotal  money.Amount
	CreditTotal money.Amount
	// OpenYears are the results of the years not closed, as credit
	// balances.
	OpenYears []YearResult
}

// BalanceSheetReport is the balance sheet at the end of the period and,
// with a granularity, at the end of each part of it.
type BalanceSheetReport struct {
	BalanceSheet
	Periods []BalanceSheet
}

var debitSide = []string{NonCurrentAssets, CurrentAssets}

// BalanceSheet reports the balance sheet categories with a balance at the
// end of the period. A current asset with a credit balance shows as a
// current liability, and the other way round. The profit and loss results
// of every year count as open years: MoneyHarpy closes no years yet.
func (s *Service) BalanceSheet(ctx context.Context, admin, user resource.ID, period, granularity string) (BalanceSheetReport, error) {
	w, err := s.open(ctx, admin, user, period, granularity)
	if err != nil {
		return BalanceSheetReport{}, err
	}
	accounts, index, err := s.accounts(ctx, admin)
	if err != nil {
		return BalanceSheetReport{}, err
	}
	at := func(p money.Period) (BalanceSheet, error) {
		sums, err := s.store.Movements(ctx, admin, nil, p.To, nil)
		if err != nil {
			return BalanceSheet{}, fmt.Errorf("sum journal: %w", err)
		}
		years, err := s.store.YearResults(ctx, admin, p.To)
		if err != nil {
			return BalanceSheet{}, fmt.Errorf("sum results: %w", err)
		}
		out := BalanceSheet{Period: p, Sections: map[string][]Line{}, OpenYears: []YearResult{}}
		for _, t := range balanceSheetTypes {
			out.Sections[t] = []Line{}
		}
		balances := map[resource.ID]money.Amount{}
		section := map[resource.ID]string{}
		for _, sum := range sums {
			a, ok := index[sum.LedgerAccountID]
			if !ok || !OnBalanceSheet(a.Type) || sum.Amount == 0 {
				continue
			}
			t := a.Type
			switch {
			case t == CurrentAssets && sum.Amount < 0:
				t = CurrentLiabilities
			case t == CurrentLiabilities && sum.Amount > 0:
				t = CurrentAssets
			}
			balances[a.ID], section[a.ID] = sum.Amount, t
		}
		// A category hangs below its top category in the same section.
		top := func(a Account) resource.ID {
			id := a.ID
			for seen := 0; a.ParentID != nil && seen < 32; seen++ {
				parent, ok := index[*a.ParentID]
				if !ok || parent.Type != index[id].Type {
					break
				}
				a, id = parent, parent.ID
			}
			return id
		}
		lines := map[string]map[resource.ID]*Line{}
		for _, a := range accounts {
			b, ok := balances[a.ID]
			if !ok {
				continue
			}
			t := section[a.ID]
			value := b
			if !slices.Contains(debitSide, t) {
				value = -b
			}
			if lines[t] == nil {
				lines[t] = map[resource.ID]*Line{}
			}
			root := a.ID
			if t == a.Type {
				root = top(a)
			}
			l := lines[t][root]
			if l == nil {
				l = &Line{LedgerAccountID: root, Children: []Line{}}
				lines[t][root] = l
			}
			if root == a.ID {
				l.Value += value
			} else {
				l.Children = append(l.Children, Line{LedgerAccountID: a.ID, Value: value})
			}
			if slices.Contains(debitSide, t) {
				out.DebitTotal += value
			} else {
				out.CreditTotal += value
			}
		}
		for _, a := range accounts { // in the order of the chart
			for t, byRoot := range lines {
				if l, ok := byRoot[a.ID]; ok {
					out.Sections[t] = append(out.Sections[t], *l)
				}
			}
		}
		for _, y := range years {
			if y.Amount != 0 {
				out.OpenYears = append(out.OpenYears, YearResult{Year: y.Year, Amount: -y.Amount})
				out.CreditTotal -= y.Amount
			}
		}
		return out, nil
	}
	report := BalanceSheetReport{}
	if report.BalanceSheet, err = at(w.period); err != nil {
		return BalanceSheetReport{}, err
	}
	for _, p := range w.parts {
		part, err := at(p)
		if err != nil {
			return BalanceSheetReport{}, err
		}
		report.Periods = append(report.Periods, part)
	}
	return report, nil
}

// CashFlow is what came in and went out of the financial accounts in a
// period, per category booked against them: received positive, paid
// negative.
type CashFlow struct {
	Period           money.Period
	Received, Paid   []Sum
	Opening, Closing money.Amount
}

// CashFlowReport is the cash flow over the period and, with a granularity,
// over each part of it.
type CashFlowReport struct {
	CashFlow
	Periods []CashFlow
}

// CashFlow reports the cash received and paid in the period, of one
// financial account when one is given.
func (s *Service) CashFlow(ctx context.Context, admin, user resource.ID, period, granularity string, account *resource.ID) (CashFlowReport, error) {
	w, err := s.open(ctx, admin, user, period, granularity)
	if err != nil {
		return CashFlowReport{}, err
	}
	_, index, err := s.accounts(ctx, admin)
	if err != nil {
		return CashFlowReport{}, err
	}
	at := func(p money.Period) (CashFlow, error) {
		sums, err := s.store.CashFlow(ctx, admin, p, account)
		if err != nil {
			return CashFlow{}, fmt.Errorf("sum cash flow: %w", err)
		}
		out := CashFlow{Period: p, Received: []Sum{}, Paid: []Sum{}}
		for _, c := range sums {
			if c.Received != 0 {
				out.Received = append(out.Received, Sum{c.LedgerAccountID, -c.Received})
			}
			if c.Paid != 0 {
				out.Paid = append(out.Paid, Sum{c.LedgerAccountID, -c.Paid})
			}
		}
		index.byCode(out.Received)
		index.byCode(out.Paid)
		if out.Opening, err = s.store.CashBalance(ctx, admin, dayBefore(p.From), account); err != nil {
			return CashFlow{}, fmt.Errorf("cash balance: %w", err)
		}
		if out.Closing, err = s.store.CashBalance(ctx, admin, p.To, account); err != nil {
			return CashFlow{}, fmt.Errorf("cash balance: %w", err)
		}
		return out, nil
	}
	report := CashFlowReport{}
	if report.CashFlow, err = at(w.period); err != nil {
		return CashFlowReport{}, err
	}
	for _, p := range w.parts {
		part, err := at(p)
		if err != nil {
			return CashFlowReport{}, err
		}
		report.Periods = append(report.Periods, part)
	}
	return report, nil
}

// WorkingCapital is current assets less current liabilities at the end of
// a period.
type WorkingCapital struct {
	Period money.Period
	// Amount is the working capital itself.
	Amount money.Amount
	// Bank is the financial accounts' balance, or zero when negative.
	Bank money.Amount
	// ToReceive is the other current assets.
	ToReceive money.Amount
	// ToPay is the current liabilities and a negative bank balance.
	ToPay money.Amount
}

// WorkingCapitalReport is the working capital at the end of the period and,
// with a granularity, at the end of each part of it.
type WorkingCapitalReport struct {
	WorkingCapital
	Periods []WorkingCapital
}

// WorkingCapital reports the working capital at the end of the period.
func (s *Service) WorkingCapital(ctx context.Context, admin, user resource.ID, period, granularity string) (WorkingCapitalReport, error) {
	w, err := s.open(ctx, admin, user, period, granularity)
	if err != nil {
		return WorkingCapitalReport{}, err
	}
	_, index, err := s.accounts(ctx, admin)
	if err != nil {
		return WorkingCapitalReport{}, err
	}
	at := func(p money.Period) (WorkingCapital, error) {
		sums, err := s.store.Movements(ctx, admin, nil, p.To, nil)
		if err != nil {
			return WorkingCapital{}, fmt.Errorf("sum journal: %w", err)
		}
		out := WorkingCapital{Period: p}
		var bank money.Amount
		for _, sum := range sums {
			a := index[sum.LedgerAccountID]
			switch {
			case a.Financial:
				bank += sum.Amount
			case a.Type == CurrentAssets:
				out.ToReceive += sum.Amount
			case a.Type == CurrentLiabilities:
				out.ToPay -= sum.Amount
			}
		}
		if bank >= 0 {
			out.Bank = bank
		} else {
			out.ToPay -= bank
		}
		out.Amount = out.Bank + out.ToReceive - out.ToPay
		return out, nil
	}
	report := WorkingCapitalReport{}
	if report.WorkingCapital, err = at(w.period); err != nil {
		return WorkingCapitalReport{}, err
	}
	for _, p := range w.parts {
		part, err := at(p)
		if err != nil {
			return WorkingCapitalReport{}, err
		}
		report.Periods = append(report.Periods, part)
	}
	return report, nil
}

// TaxLine is a VAT rate's turnover and VAT in a period, as positive amounts
// for what was charged on sales and on purchases.
type TaxLine struct {
	TaxRateID  resource.ID
	Name       string
	Percentage *string
	Reference  string
	Type       string
	Amount     money.Amount
	Tax        money.Amount
}

// TaxPeriod is the VAT rates with an amount in a period.
type TaxPeriod struct {
	Period money.Period
	Rates  []TaxLine
}

// TaxReport is the VAT over the period and, with a granularity, over each
// part of it.
type TaxReport struct {
	TaxPeriod
	Periods []TaxPeriod
}

// Tax reports per VAT rate the amount it was charged on and the VAT, from
// what invoices, receipts and other documents booked: the VAT of sales
// rates on payable VAT, of purchase rates on receivable VAT.
func (s *Service) Tax(ctx context.Context, admin, user resource.ID, period, granularity string) (TaxReport, error) {
	w, err := s.open(ctx, admin, user, period, granularity)
	if err != nil {
		return TaxReport{}, err
	}
	at := func(p money.Period) (TaxPeriod, error) {
		sums, err := s.store.TaxSums(ctx, admin, p)
		if err != nil {
			return TaxPeriod{}, fmt.Errorf("sum VAT: %w", err)
		}
		out := TaxPeriod{Period: p, Rates: []TaxLine{}}
		for _, t := range sums {
			line := TaxLine{TaxRateID: t.TaxRateID, Name: t.Name, Reference: strings.Join(t.Boxes, ", "), Type: t.Type}
			if t.Percentage != "" {
				line.Percentage = ptr(t.Percentage)
			}
			if t.Type == "sales_invoice" {
				line.Amount, line.Tax = -t.Base, -t.Payable
			} else {
				line.Amount, line.Tax = t.Base, t.Receivable
			}
			if line.Amount != 0 || line.Tax != 0 {
				out.Rates = append(out.Rates, line)
			}
		}
		return out, nil
	}
	report := TaxReport{}
	if report.TaxPeriod, err = at(w.period); err != nil {
		return TaxReport{}, err
	}
	for _, p := range w.parts {
		part, err := at(p)
		if err != nil {
			return TaxReport{}, err
		}
		report.Periods = append(report.Periods, part)
	}
	return report, nil
}

// LedgerAccount reports a page of a category's entries in the period, the
// newest first: Moneybird's deprecated ledger account report.
func (s *Service) LedgerAccount(ctx context.Context, admin, user, ledger resource.ID, period string, page, perPage int) ([]journal.Entry, int, error) {
	w, err := s.open(ctx, admin, user, period, "")
	if err != nil {
		return nil, 0, err
	}
	_, index, err := s.accounts(ctx, admin)
	if err != nil {
		return nil, 0, err
	}
	if _, ok := index[ledger]; !ok {
		return nil, 0, ErrLedgerAccountNotFound
	}
	page, perPage = pageOf(page, perPage)
	return s.store.LedgerEntries(ctx, admin, ledger, w.period, page, perPage)
}

// endOfMonth is the last day of t's month.
func endOfMonth(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month()+1, 1, 0, 0, 0, 0, time.UTC).AddDate(0, 0, -1)
}
