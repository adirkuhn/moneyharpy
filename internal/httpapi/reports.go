package httpapi

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/adirkuhn/moneyharpy/internal/administration"
	"github.com/adirkuhn/moneyharpy/internal/money"
	"github.com/adirkuhn/moneyharpy/internal/reports"
	"github.com/adirkuhn/moneyharpy/internal/resource"
)

// failReport answers the errors of reports as Moneybird does: refusals are
// bad requests, with their symbolic codes.
func (s *server) failReport(w http.ResponseWriter, r *http.Request, err error) {
	var refusal *reports.Refusal
	switch {
	case errors.As(err, &refusal):
		var codes map[string]any
		if len(refusal.Symbolic) > 0 {
			codes = map[string]any{}
			for k, v := range refusal.Symbolic {
				codes[k] = v
			}
		}
		symbolic(w, http.StatusBadRequest, refusal.Message, codes)
	case errors.Is(err, reports.ErrLedgerAccountNotFound):
		writeError(w, http.StatusNotFound, "Record not found for model name: Category")
	default:
		s.fail(w, r, err)
	}
}

// reportStart reads the administration id and checks the permissions an
// operation needs on top of its x-permissions: Moneybird asks for all of
// them.
func (s *server) reportStart(w http.ResponseWriter, r *http.Request, administrationID AdministrationId, also ...administration.Permission) (resource.ID, bool) {
	admin, ok := parseID(w, administrationID)
	if !ok {
		return 0, false
	}
	for _, p := range also {
		if !s.requireAlso(w, r, admin, p) {
			return 0, false
		}
	}
	return admin, true
}

func reportPaging(page *Page, perPage *string) reports.Paging {
	p, _ := strconv.Atoi(deref((*string)(page)))
	n, _ := strconv.Atoi(deref(perPage))
	return reports.Paging{Page: p, PerPage: n}
}

func (s *server) writeReportLinks(w http.ResponseWriter, r *http.Request, pg reports.Paging, total int) {
	s.writeLinks(w, r, max(pg.Page, 1), pageSize(pg.PerPage, reports.DefaultPerPage, reports.MaxPerPage), total)
}

func optionalQueryID(w http.ResponseWriter, v *string) (*resource.ID, bool) {
	if v == nil {
		return nil, true
	}
	id, ok := parseID(w, *v)
	return &id, ok
}

func period(p *ReportPeriod) string { return deref((*string)(p)) }

type reportValue struct {
	LedgerAccountID string `json:"ledger_account_id"`
	Value           string `json:"value"`
}

type reportValues struct {
	LedgerAccounts []reportValue `json:"ledger_accounts"`
}

func toReportValues(sums []reports.Sum) reportValues {
	out := reportValues{LedgerAccounts: []reportValue{}}
	for _, v := range sums {
		out.LedgerAccounts = append(out.LedgerAccounts, reportValue{v.LedgerAccountID.String(), v.Amount.String()})
	}
	return out
}

type profitLossJSON struct {
	Period              *string          `json:"period,omitempty"`
	Revenue             reportValues     `json:"revenue_by_ledger_account"`
	DirectCosts         reportValues     `json:"direct_costs_by_ledger_account"`
	Expenses            reportValues     `json:"expenses_by_ledger_account"`
	OtherIncomeExpenses reportValues     `json:"other_income_expenses_by_ledger_account"`
	GrossProfit         string           `json:"gross_profit"`
	OperatingProfit     string           `json:"operating_profit"`
	NetProfit           string           `json:"net_profit"`
	TotalExpenses       string           `json:"total_expenses"`
	TotalRevenue        string           `json:"total_revenue"`
	Periods             []profitLossJSON `json:"periods,omitempty"`
}

func label(p money.Period) *string { return ptr(p.Monthly()) }

func toProfitLoss(p reports.ProfitLoss) profitLossJSON {
	return profitLossJSON{
		Revenue: toReportValues(p.Revenue), DirectCosts: toReportValues(p.DirectCosts), Expenses: toReportValues(p.Expenses),
		OtherIncomeExpenses: toReportValues(p.Other), GrossProfit: p.GrossProfit.String(), OperatingProfit: p.OperatingProfit.String(),
		NetProfit: p.NetProfit.String(), TotalExpenses: p.TotalExpenses.String(), TotalRevenue: p.TotalRevenue.String(),
	}
}

// GetProfitLossReport implements ServerInterface.
func (s *server) GetProfitLossReport(w http.ResponseWriter, r *http.Request, administrationID AdministrationId, params GetProfitLossReportParams) {
	admin, ok := s.reportStart(w, r, administrationID, administration.SalesInvoices)
	if !ok {
		return
	}
	project, ok := optionalQueryID(w, params.ProjectId)
	if !ok {
		return
	}
	report, err := s.deps.Reports.ProfitLoss(r.Context(), admin, user(r).ID, period(params.Period), string(deref(params.Granularity)), project)
	if err != nil {
		s.failReport(w, r, err)
		return
	}
	out := toProfitLoss(report.ProfitLoss)
	for _, p := range report.Periods {
		part := toProfitLoss(p)
		part.Period = label(p.Period)
		out.Periods = append(out.Periods, part)
	}
	writeJSON(w, http.StatusOK, out)
}

type balanceLineJSON struct {
	LedgerAccountID string             `json:"ledger_account_id"`
	Value           string             `json:"value"`
	Children        []balanceChildJSON `json:"children"`
}

type balanceChildJSON struct {
	LedgerAccountID string `json:"ledger_account_id"`
	Value           string `json:"value"`
}

type openYearJSON struct {
	Year  string `json:"year"`
	Value string `json:"value"`
}

type balanceSheetJSON struct {
	Period *string `json:"period,omitempty"`
	Debit  struct {
		Total            string            `json:"total"`
		CurrentAssets    []balanceLineJSON `json:"current_assets"`
		NonCurrentAssets []balanceLineJSON `json:"non_current_assets"`
	} `json:"debit"`
	Credit struct {
		Total                 string            `json:"total"`
		Equity                []balanceLineJSON `json:"equity"`
		Provisions            []balanceLineJSON `json:"provisions"`
		NonCurrentLiabilities []balanceLineJSON `json:"non_current_liabilities"`
		CurrentLiabilities    []balanceLineJSON `json:"current_liabilities"`
		OpenYears             []openYearJSON    `json:"open_years"`
	} `json:"credit"`
	Periods []balanceSheetJSON `json:"periods,omitempty"`
}

func toBalanceLines(lines []reports.Line) []balanceLineJSON {
	out := []balanceLineJSON{}
	for _, l := range lines {
		line := balanceLineJSON{LedgerAccountID: l.LedgerAccountID.String(), Value: l.Value.String(), Children: []balanceChildJSON{}}
		for _, c := range l.Children {
			line.Children = append(line.Children, balanceChildJSON{c.LedgerAccountID.String(), c.Value.String()})
		}
		out = append(out, line)
	}
	return out
}

func toBalanceSheet(b reports.BalanceSheet) balanceSheetJSON {
	var out balanceSheetJSON
	out.Debit.Total = b.DebitTotal.String()
	out.Debit.CurrentAssets = toBalanceLines(b.Sections[reports.CurrentAssets])
	out.Debit.NonCurrentAssets = toBalanceLines(b.Sections[reports.NonCurrentAssets])
	out.Credit.Total = b.CreditTotal.String()
	out.Credit.Equity = toBalanceLines(b.Sections[reports.Equity])
	out.Credit.Provisions = toBalanceLines(b.Sections[reports.Provisions])
	out.Credit.NonCurrentLiabilities = toBalanceLines(b.Sections[reports.NonCurrentLiabilities])
	out.Credit.CurrentLiabilities = toBalanceLines(b.Sections[reports.CurrentLiabilities])
	out.Credit.OpenYears = []openYearJSON{}
	for _, y := range b.OpenYears {
		out.Credit.OpenYears = append(out.Credit.OpenYears, openYearJSON{strconv.Itoa(y.Year), y.Amount.String()})
	}
	return out
}

// GetBalanceSheetReport implements ServerInterface.
func (s *server) GetBalanceSheetReport(w http.ResponseWriter, r *http.Request, administrationID AdministrationId, params GetBalanceSheetReportParams) {
	admin, ok := s.reportStart(w, r, administrationID)
	if !ok {
		return
	}
	report, err := s.deps.Reports.BalanceSheet(r.Context(), admin, user(r).ID, period(params.Period), string(deref(params.Granularity)))
	if err != nil {
		s.failReport(w, r, err)
		return
	}
	out := toBalanceSheet(report.BalanceSheet)
	for _, p := range report.Periods {
		part := toBalanceSheet(p)
		part.Period = label(p.Period)
		out.Periods = append(out.Periods, part)
	}
	writeJSON(w, http.StatusOK, out)
}

type cashFlowJSON struct {
	Period         *string        `json:"period,omitempty"`
	Received       reportValues   `json:"cash_received_by_ledger_account"`
	Paid           reportValues   `json:"cash_paid_by_ledger_account"`
	OpeningBalance string         `json:"opening_balance"`
	ClosingBalance string         `json:"closing_balance"`
	Periods        []cashFlowJSON `json:"periods,omitempty"`
}

func toCashFlow(c reports.CashFlow) cashFlowJSON {
	return cashFlowJSON{Received: toReportValues(c.Received), Paid: toReportValues(c.Paid), OpeningBalance: c.Opening.String(), ClosingBalance: c.Closing.String()}
}

// GetCashFlowReport implements ServerInterface.
func (s *server) GetCashFlowReport(w http.ResponseWriter, r *http.Request, administrationID AdministrationId, params GetCashFlowReportParams) {
	admin, ok := s.reportStart(w, r, administrationID)
	if !ok {
		return
	}
	account, ok := optionalQueryID(w, params.FinancialAccountId)
	if !ok {
		return
	}
	report, err := s.deps.Reports.CashFlow(r.Context(), admin, user(r).ID, period(params.Period), string(deref(params.Granularity)), account)
	if err != nil {
		s.failReport(w, r, err)
		return
	}
	out := toCashFlow(report.CashFlow)
	for _, p := range report.Periods {
		part := toCashFlow(p)
		part.Period = label(p.Period)
		out.Periods = append(out.Periods, part)
	}
	writeJSON(w, http.StatusOK, out)
}

type taxLineJSON struct {
	TaxRateID       string  `json:"tax_rate_id"`
	Name            string  `json:"name"`
	Percentage      *string `json:"percentage"`
	ReportReference string  `json:"report_reference"`
	Amount          string  `json:"amount"`
	Type            string  `json:"type"`
	Tax             string  `json:"tax"`
}

type taxJSON struct {
	Period   *string       `json:"period,omitempty"`
	TaxRates []taxLineJSON `json:"tax_rates"`
	Periods  []taxJSON     `json:"periods,omitempty"`
}

func toTax(t reports.TaxPeriod) taxJSON {
	out := taxJSON{TaxRates: []taxLineJSON{}}
	for _, l := range t.Rates {
		out.TaxRates = append(out.TaxRates, taxLineJSON{
			TaxRateID: l.TaxRateID.String(), Name: l.Name, Percentage: l.Percentage, ReportReference: l.Reference, Amount: l.Amount.String(), Type: l.Type, Tax: l.Tax.String(),
		})
	}
	return out
}

// GetTaxReport implements ServerInterface.
func (s *server) GetTaxReport(w http.ResponseWriter, r *http.Request, administrationID AdministrationId, params GetTaxReportParams) {
	admin, ok := s.reportStart(w, r, administrationID, administration.SalesInvoices)
	if !ok {
		return
	}
	report, err := s.deps.Reports.Tax(r.Context(), admin, user(r).ID, period(params.Period), string(deref(params.Granularity)))
	if err != nil {
		s.failReport(w, r, err)
		return
	}
	out := toTax(report.TaxPeriod)
	for _, p := range report.Periods {
		part := toTax(p)
		part.Period = label(p.Period)
		out.Periods = append(out.Periods, part)
	}
	writeJSON(w, http.StatusOK, out)
}

type workingCapitalJSON struct {
	Period         *string              `json:"period,omitempty"`
	WorkingCapital string               `json:"working_capital"`
	Bank           string               `json:"bank"`
	ToReceive      string               `json:"to_receive"`
	ToPay          string               `json:"to_pay"`
	Periods        []workingCapitalJSON `json:"periods,omitempty"`
}

func toWorkingCapital(c reports.WorkingCapital) workingCapitalJSON {
	return workingCapitalJSON{WorkingCapital: c.Amount.String(), Bank: c.Bank.String(), ToReceive: c.ToReceive.String(), ToPay: c.ToPay.String()}
}

// GetWorkingCapitalReport implements ServerInterface.
func (s *server) GetWorkingCapitalReport(w http.ResponseWriter, r *http.Request, administrationID AdministrationId, params GetWorkingCapitalReportParams) {
	admin, ok := s.reportStart(w, r, administrationID)
	if !ok {
		return
	}
	report, err := s.deps.Reports.WorkingCapital(r.Context(), admin, user(r).ID, period(params.Period), string(deref(params.Granularity)))
	if err != nil {
		s.failReport(w, r, err)
		return
	}
	out := toWorkingCapital(report.WorkingCapital)
	for _, p := range report.Periods {
		part := toWorkingCapital(p)
		part.Period = label(p.Period)
		out.Periods = append(out.Periods, part)
	}
	writeJSON(w, http.StatusOK, out)
}

// subjectsJSON writes a report per contact or project under key, with the
// id under idKey.
func subjectsJSON(report reports.SubjectReport, key, idKey string) map[string]any {
	items := func(list []reports.SubjectSum) []map[string]any {
		out := []map[string]any{}
		for _, x := range list {
			out = append(out, map[string]any{idKey: idString(x.ID), "amount": x.Amount.String()})
		}
		return out
	}
	out := map[string]any{key: items(report.Items)}
	if report.Periods != nil {
		parts := []map[string]any{}
		for _, p := range report.Periods {
			parts = append(parts, map[string]any{"period": p.Period.Monthly(), key: items(p.Items)})
		}
		out["periods"] = parts
	}
	return out
}

type subjectFunc func(pg reports.Paging) (reports.SubjectReport, error)

func (s *server) writeSubjects(w http.ResponseWriter, r *http.Request, pg reports.Paging, key, idKey string, run subjectFunc) {
	report, err := run(pg)
	if err != nil {
		s.failReport(w, r, err)
		return
	}
	s.writeReportLinks(w, r, pg, report.Total)
	writeJSON(w, http.StatusOK, subjectsJSON(report, key, idKey))
}

// GetDebtorsReport implements ServerInterface.
func (s *server) GetDebtorsReport(w http.ResponseWriter, r *http.Request, administrationID AdministrationId, params GetDebtorsReportParams) {
	admin, ok := s.reportStart(w, r, administrationID)
	if !ok {
		return
	}
	s.writeSubjects(w, r, reportPaging(params.Page, params.PerPage), "debtors", "contact_id", func(pg reports.Paging) (reports.SubjectReport, error) {
		return s.deps.Reports.Debtors(r.Context(), admin, user(r).ID, period(params.Period), string(deref(params.Granularity)), pg)
	})
}

// GetCreditorsReport implements ServerInterface.
func (s *server) GetCreditorsReport(w http.ResponseWriter, r *http.Request, administrationID AdministrationId, params GetCreditorsReportParams) {
	admin, ok := s.reportStart(w, r, administrationID)
	if !ok {
		return
	}
	s.writeSubjects(w, r, reportPaging(params.Page, params.PerPage), "creditors", "contact_id", func(pg reports.Paging) (reports.SubjectReport, error) {
		return s.deps.Reports.Creditors(r.Context(), admin, user(r).ID, period(params.Period), string(deref(params.Granularity)), pg)
	})
}

// GetRevenueByContactReport implements ServerInterface.
func (s *server) GetRevenueByContactReport(w http.ResponseWriter, r *http.Request, administrationID AdministrationId, params GetRevenueByContactReportParams) {
	admin, ok := s.reportStart(w, r, administrationID)
	if !ok {
		return
	}
	contact, ok := optionalQueryID(w, params.ContactId)
	if !ok {
		return
	}
	s.writeSubjects(w, r, reportPaging(params.Page, params.PerPage), "revenue_by_contact", "contact_id", func(pg reports.Paging) (reports.SubjectReport, error) {
		return s.deps.Reports.RevenueByContact(r.Context(), admin, user(r).ID, period(params.Period), string(deref(params.Granularity)), contact, pg)
	})
}

// GetExpensesByContactReport implements ServerInterface.
func (s *server) GetExpensesByContactReport(w http.ResponseWriter, r *http.Request, administrationID AdministrationId, params GetExpensesByContactReportParams) {
	admin, ok := s.reportStart(w, r, administrationID)
	if !ok {
		return
	}
	contact, ok := optionalQueryID(w, params.ContactId)
	if !ok {
		return
	}
	s.writeSubjects(w, r, reportPaging(params.Page, params.PerPage), "expenses_by_contact", "contact_id", func(pg reports.Paging) (reports.SubjectReport, error) {
		return s.deps.Reports.ExpensesByContact(r.Context(), admin, user(r).ID, period(params.Period), string(deref(params.Granularity)), contact, pg)
	})
}

// GetRevenueByProjectReport implements ServerInterface.
func (s *server) GetRevenueByProjectReport(w http.ResponseWriter, r *http.Request, administrationID AdministrationId, params GetRevenueByProjectReportParams) {
	admin, ok := s.reportStart(w, r, administrationID)
	if !ok {
		return
	}
	s.writeSubjects(w, r, reportPaging(params.Page, params.PerPage), "revenue_by_project", "project_id", func(pg reports.Paging) (reports.SubjectReport, error) {
		return s.deps.Reports.RevenueByProject(r.Context(), admin, user(r).ID, period(params.Period), string(deref(params.Granularity)), pg)
	})
}

// GetExpensesByProjectReport implements ServerInterface.
func (s *server) GetExpensesByProjectReport(w http.ResponseWriter, r *http.Request, administrationID AdministrationId, params GetExpensesByProjectReportParams) {
	admin, ok := s.reportStart(w, r, administrationID)
	if !ok {
		return
	}
	s.writeSubjects(w, r, reportPaging(params.Page, params.PerPage), "expenses_by_project", "project_id", func(pg reports.Paging) (reports.SubjectReport, error) {
		return s.deps.Reports.ExpensesByProject(r.Context(), admin, user(r).ID, period(params.Period), string(deref(params.Granularity)), pg)
	})
}

type agingBucketJSON struct {
	Name   string `json:"name"`
	Amount string `json:"amount"`
}

type agingItemJSON struct {
	ContactID *string           `json:"contact_id"`
	Amount    string            `json:"amount"`
	Buckets   []agingBucketJSON `json:"aging_buckets"`
}

func (s *server) writeAging(w http.ResponseWriter, r *http.Request, key string, pg reports.Paging, report reports.AgingReport, err error) {
	if err != nil {
		s.failReport(w, r, err)
		return
	}
	items := []agingItemJSON{}
	for _, it := range report.Items {
		row := agingItemJSON{ContactID: idString(it.ContactID), Amount: it.Amount.String()}
		for _, b := range it.Buckets {
			row.Buckets = append(row.Buckets, agingBucketJSON{b.Name, b.Amount.String()})
		}
		items = append(items, row)
	}
	s.writeReportLinks(w, r, pg, report.Total)
	writeJSON(w, http.StatusOK, map[string]any{key: items})
}

// GetDebtorsAgingReport implements ServerInterface.
func (s *server) GetDebtorsAgingReport(w http.ResponseWriter, r *http.Request, administrationID AdministrationId, params GetDebtorsAgingReportParams) {
	admin, ok := s.reportStart(w, r, administrationID)
	if !ok {
		return
	}
	pg := reportPaging(params.Page, params.PerPage)
	report, err := s.deps.Reports.DebtorsAging(r.Context(), admin, user(r).ID, deref(params.PeriodUntil), pg)
	s.writeAging(w, r, "debtors", pg, report, err)
}

// GetCreditorsAgingReport implements ServerInterface.
func (s *server) GetCreditorsAgingReport(w http.ResponseWriter, r *http.Request, administrationID AdministrationId, params GetCreditorsAgingReportParams) {
	admin, ok := s.reportStart(w, r, administrationID)
	if !ok {
		return
	}
	pg := reportPaging(params.Page, params.PerPage)
	report, err := s.deps.Reports.CreditorsAging(r.Context(), admin, user(r).ID, deref(params.PeriodUntil), pg)
	s.writeAging(w, r, "creditors", pg, report, err)
}

// GetSubscriptionsReport implements ServerInterface.
func (s *server) GetSubscriptionsReport(w http.ResponseWriter, r *http.Request, administrationID AdministrationId, params GetSubscriptionsReportParams) {
	admin, ok := s.reportStart(w, r, administrationID)
	if !ok {
		return
	}
	f, err := s.deps.Reports.Subscriptions(r.Context(), admin, user(r).ID, period(params.Period))
	if err != nil {
		s.failReport(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, SubscriptionsReport{
		MonthlyRecurringRevenue: f.MonthlyRecurringRevenue.String(), AnnualRunRate: f.AnnualRunRate.String(), ChurnRate: f.ChurnRate, LifetimeValue: f.LifetimeValue,
	})
}

// GetLedgerAccountReport implements ServerInterface.
func (s *server) GetLedgerAccountReport(w http.ResponseWriter, r *http.Request, administrationID AdministrationId, ledgerAccountID Identifier, params GetLedgerAccountReportParams) {
	admin, ok := s.reportStart(w, r, administrationID, administration.SalesInvoices)
	if !ok {
		return
	}
	ledger, ok := parseID(w, ledgerAccountID)
	if !ok {
		return
	}
	pg := reportPaging(params.Page, params.PerPage)
	list, total, err := s.deps.Reports.LedgerAccount(r.Context(), admin, user(r).ID, ledger, period(params.Period), pg.Page, pg.PerPage)
	if err != nil {
		s.failReport(w, r, err)
		return
	}
	out := make([]journalEntryJSON, len(list))
	for i, e := range list {
		out[i] = journalEntryJSON{
			ID: e.ID.String(), Date: e.Date.Format(time.DateOnly), DocumentType: e.DocumentType, DocumentID: e.DocumentID.String(),
			LedgerAccountID: e.LedgerAccountID.String(), ProjectID: idString(e.ProjectID), ContactID: idString(e.ContactID),
			TaxRateID: idString(e.TaxRateID), FinancialAccountID: idString(e.FinancialAccountID), Amount: e.Amount.String(), Description: nz(e.Description),
		}
	}
	s.writeReportLinks(w, r, pg, total)
	writeJSON(w, http.StatusOK, out)
}

type exportBody struct {
	Year    flexString `json:"year"`
	Version string     `json:"version"`
}

// ExportAuditfile implements ServerInterface.
func (s *server) ExportAuditfile(w http.ResponseWriter, r *http.Request, administrationID AdministrationId) {
	admin, ok := s.reportStart(w, r, administrationID, administration.Documents, administration.Estimates, administration.SalesInvoices, administration.Settings)
	if !ok {
		return
	}
	body, ok := decodeBody[exportBody](w, r)
	if !ok {
		return
	}
	s.exported(w, r, s.deps.Reports.ExportAuditfile(r.Context(), admin, user(r).ID, string(body.Year), body.Version))
}

// ExportBrugstaat implements ServerInterface.
func (s *server) ExportBrugstaat(w http.ResponseWriter, r *http.Request, administrationID AdministrationId) {
	admin, ok := s.reportStart(w, r, administrationID, administration.Documents, administration.SalesInvoices)
	if !ok {
		return
	}
	body, ok := decodeBody[exportBody](w, r)
	if !ok {
		return
	}
	s.exported(w, r, s.deps.Reports.ExportBrugstaat(r.Context(), admin, user(r).ID, string(body.Year)))
}

// ExportLedgerAccounts implements ServerInterface.
func (s *server) ExportLedgerAccounts(w http.ResponseWriter, r *http.Request, administrationID AdministrationId) {
	admin, ok := s.reportStart(w, r, administrationID, administration.SalesInvoices)
	if !ok {
		return
	}
	body, ok := decodeBody[exportBody](w, r)
	if !ok {
		return
	}
	s.exported(w, r, s.deps.Reports.ExportLedgerAccounts(r.Context(), admin, user(r).ID, string(body.Year)))
}

func (s *server) exported(w http.ResponseWriter, r *http.Request, err error) {
	if err != nil {
		s.failReport(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
