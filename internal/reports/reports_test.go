package reports

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/adirkuhn/moneyharpy/internal/administration"
	"github.com/adirkuhn/moneyharpy/internal/downloads"
	"github.com/adirkuhn/moneyharpy/internal/money"
	"github.com/adirkuhn/moneyharpy/internal/resource"
	"github.com/google/go-cmp/cmp"
)

var ctx = context.Background()

// books posts a small year and a half: an opening, a sales invoice in 2024
// paid in June 2025, one in May 2025, purchases and interest in June.
func books() fixture {
	f := newFixture()
	a, b, app := ptr(alice), ptr(bob), ptr(webApp)
	f.post("GeneralJournalDocument", "2024-01-02", nil, nil, inventory, 50000, stock, 10000, equityID, -60000)
	f.post("SalesInvoice", "2024-11-10", a, app, debtorsID, 12100, sales, -10000, payableVAT, -2100)
	f.post("SalesInvoice", "2025-05-10", b, app, debtorsID, 24200, sales, -20000, payableVAT, -4200)
	f.post("FinancialMutation", "2025-06-05", a, nil, bank, 12100, debtorsID, -12100)
	f.post("PurchaseInvoice", "2025-06-08", b, nil, purchases, 5000, rent, 2000, creditorsID, -7000)
	f.post("FinancialMutation", "2025-06-10", nil, nil, bank, 100, interest, -100)
	return f
}

func refusal(t *testing.T, err error, want string) {
	t.Helper()
	var r *Refusal
	if !errors.As(err, &r) || r.Message != want {
		t.Fatalf("err = %v, want refusal %q", err, want)
	}
}

func TestPeriodsAreChecked(t *testing.T) {
	f := books()
	_, err := f.ProfitLoss(ctx, adminID, owner, "nonsense", "", nil)
	refusal(t, err, "Period is invalid")
	_, err = f.BalanceSheet(ctx, adminID, owner, "20250102..20250131", "")
	refusal(t, err, "Period is invalid")
	_, err = f.Tax(ctx, adminID, owner, "202401..202506", "")
	refusal(t, err, "Period cannot exceed 12 months")
	_, err = f.Debtors(ctx, adminID, owner, "this_year", "week", Paging{})
	refusal(t, err, "granularity does not have a valid value")
	if _, err := f.WorkingCapital(ctx, adminID, stranger, "", ""); !errors.Is(err, administration.ErrNotFound) {
		t.Errorf("stranger err = %v", err)
	}
}

func values(sums []Sum) map[resource.ID]money.Amount {
	out := map[resource.ID]money.Amount{}
	for _, s := range sums {
		out[s.LedgerAccountID] = s.Amount
	}
	return out
}

func TestProfitLoss(t *testing.T) {
	f := books()
	r, err := f.ProfitLoss(ctx, adminID, owner, "202505..202506", "month", nil)
	if err != nil {
		t.Fatal(err)
	}
	if diff := cmp.Diff(map[resource.ID]money.Amount{sales: 20000}, values(r.Revenue)); diff != "" {
		t.Errorf("revenue: %s", diff)
	}
	if values(r.DirectCosts)[purchases] != 5000 || values(r.Expenses)[rent] != 2000 || values(r.Other)[interest] != 100 {
		t.Errorf("sections = %+v", r.ProfitLoss)
	}
	if r.TotalRevenue != 20000 || r.GrossProfit != 15000 || r.OperatingProfit != 13000 || r.TotalExpenses != 6900 || r.NetProfit != 13100 {
		t.Errorf("totals = %+v", r.ProfitLoss)
	}
	if len(r.Periods) != 2 || r.Periods[0].TotalRevenue != 20000 || len(r.Periods[0].Expenses) != 0 || r.Periods[1].NetProfit != -6900 {
		t.Errorf("periods = %+v", r.Periods)
	}
	onProject, err := f.ProfitLoss(ctx, adminID, owner, "202505..202506", "", ptr(webApp))
	if err != nil {
		t.Fatal(err)
	}
	if onProject.TotalRevenue != 20000 || len(onProject.DirectCosts) != 0 || onProject.Periods != nil {
		t.Errorf("on project = %+v", onProject)
	}
}

func TestBalanceSheet(t *testing.T) {
	f := books()
	r, err := f.BalanceSheet(ctx, adminID, owner, "202506", "")
	if err != nil {
		t.Fatal(err)
	}
	want := map[string][]Line{
		NonCurrentAssets:      {{LedgerAccountID: inventory, Value: 50000, Children: []Line{{LedgerAccountID: stock, Value: 10000}}}},
		CurrentAssets:         {{LedgerAccountID: bank, Value: 12200, Children: []Line{}}, {LedgerAccountID: debtorsID, Value: 24200, Children: []Line{}}},
		Equity:                {{LedgerAccountID: equityID, Value: 60000, Children: []Line{}}},
		Provisions:            {},
		NonCurrentLiabilities: {},
		CurrentLiabilities:    {{LedgerAccountID: payableVAT, Value: 6300, Children: []Line{}}, {LedgerAccountID: creditorsID, Value: 7000, Children: []Line{}}},
	}
	if diff := cmp.Diff(want, r.Sections); diff != "" {
		t.Errorf("sections: %s", diff)
	}
	if diff := cmp.Diff([]YearResult{{2024, 10000}, {2025, 13100}}, r.OpenYears); diff != "" {
		t.Errorf("open years: %s", diff)
	}
	if r.DebitTotal != 96400 || r.CreditTotal != 96400 {
		t.Errorf("totals = %s, %s", r.DebitTotal, r.CreditTotal)
	}
	// A bank account in the red shows with the current liabilities.
	f.post("FinancialMutation", "2025-06-12", nil, nil, bank, -20000, rent, 20000)
	r, _ = f.BalanceSheet(ctx, adminID, owner, "202505..202506", "month")
	if got := r.Sections[CurrentLiabilities]; len(got) != 3 || got[0].LedgerAccountID != bank || got[0].Value != 7800 {
		t.Errorf("overdrawn = %+v", got)
	}
	if len(r.Periods) != 2 || len(r.Periods[0].Sections[CurrentAssets]) != 1 || r.Periods[0].DebitTotal != 96300 {
		t.Errorf("by month = %+v", r.Periods)
	}
}

func TestWorkingCapitalAndCashFlow(t *testing.T) {
	f := books()
	w, err := f.WorkingCapital(ctx, adminID, owner, "202506", "")
	if err != nil {
		t.Fatal(err)
	}
	if w.Bank != 12200 || w.ToReceive != 24200 || w.ToPay != 13300 || w.Amount != 23100 {
		t.Errorf("working capital = %+v", w)
	}
	f.store.cash = []CashSum{{LedgerAccountID: debtorsID, Received: -12100}, {LedgerAccountID: interest, Received: -100}, {LedgerAccountID: rent, Paid: 300}}
	c, err := f.CashFlow(ctx, adminID, owner, "202506", "quarter", nil)
	if err != nil {
		t.Fatal(err)
	}
	if diff := cmp.Diff([]Sum{{debtorsID, 12100}, {interest, 100}}, c.Received); diff != "" {
		t.Errorf("received: %s", diff)
	}
	if diff := cmp.Diff([]Sum{{rent, -300}}, c.Paid); diff != "" {
		t.Errorf("paid: %s", diff)
	}
	if c.Opening != 0 || c.Closing != 12200 || len(c.Periods) != 1 || c.Periods[0].Period.Monthly() != "202506..202506" {
		t.Errorf("cash flow = %+v", c)
	}
}

func TestTax(t *testing.T) {
	f := books()
	f.store.tax = []TaxSum{
		{TaxRateID: 1, Name: "21% btw", Percentage: "21.0", Type: "sales_invoice", Boxes: []string{"NL/1a"}, Base: -30000, Payable: -6300},
		{TaxRateID: 2, Name: "21% btw", Percentage: "21.0", Type: "purchase_invoice", Boxes: []string{"NL/5b"}, Base: 5000, Receivable: 1050},
		{TaxRateID: 3, Name: "0% btw", Type: "purchase_invoice"},
	}
	r, err := f.Tax(ctx, adminID, owner, "this_quarter", "")
	if err != nil {
		t.Fatal(err)
	}
	want := []TaxLine{
		{TaxRateID: 1, Name: "21% btw", Percentage: ptr("21.0"), Reference: "NL/1a", Type: "sales_invoice", Amount: 30000, Tax: 6300},
		{TaxRateID: 2, Name: "21% btw", Percentage: ptr("21.0"), Reference: "NL/5b", Type: "purchase_invoice", Amount: 5000, Tax: 1050},
	}
	if diff := cmp.Diff(want, r.Rates); diff != "" {
		t.Errorf("rates: %s", diff)
	}
}

func ids(items []SubjectSum) []string {
	out := []string{}
	for _, x := range items {
		id := "none"
		if x.ID != nil {
			id = x.ID.String()
		}
		out = append(out, id+"="+x.Amount.String())
	}
	return out
}

func TestDebtorsAndCreditors(t *testing.T) {
	f := books()
	r, err := f.Debtors(ctx, adminID, owner, "202505..202506", "month", Paging{})
	if err != nil {
		t.Fatal(err)
	}
	if diff := cmp.Diff([]string{"101=242.0"}, ids(r.Items)); diff != "" || r.Total != 1 || len(r.Periods) != 2 || ids(r.Periods[0].Items)[0] != "101=242.0" {
		t.Errorf("debtors = %+v %s", r, diff)
	}
	if old, _ := f.Debtors(ctx, adminID, owner, "202412", "", Paging{}); cmp.Diff([]string{"100=121.0"}, ids(old.Items)) != "" {
		t.Errorf("debtors 2024 = %v", ids(old.Items))
	}
	c, _ := f.Creditors(ctx, adminID, owner, "202506", "", Paging{})
	if cmp.Diff([]string{"101=70.0"}, ids(c.Items)) != "" {
		t.Errorf("creditors = %v", ids(c.Items))
	}
}

func TestRevenueAndExpensesBySubject(t *testing.T) {
	f := books()
	f.post("SalesInvoice", "2025-06-01", ptr(alice), nil, debtorsID, 500, sales, -500)
	r, err := f.RevenueByContact(ctx, adminID, owner, "202501..202506", "month", nil, Paging{PerPage: 1})
	if err != nil {
		t.Fatal(err)
	}
	if cmp.Diff([]string{"101=200.0"}, ids(r.Items)) != "" || r.Total != 2 || len(r.Periods) != 6 || len(r.Periods[0].Items) != 0 || ids(r.Periods[4].Items)[0] != "101=200.0" {
		t.Errorf("revenue by contact = %+v", r)
	}
	only, _ := f.RevenueByContact(ctx, adminID, owner, "202501..202506", "", ptr(alice), Paging{})
	if cmp.Diff([]string{"100=5.0"}, ids(only.Items)) != "" {
		t.Errorf("alice = %v", ids(only.Items))
	}
	p, _ := f.RevenueByProject(ctx, adminID, owner, "202411..202506", "", Paging{})
	if cmp.Diff([]string{"none=5.0", "200=300.0"}, ids(p.Items)) != "" {
		t.Errorf("revenue by project = %v", ids(p.Items))
	}
	e, _ := f.ExpensesByProject(ctx, adminID, owner, "202506", "", Paging{})
	if cmp.Diff([]string{"none=70.0"}, ids(e.Items)) != "" {
		t.Errorf("expenses by project = %v", ids(e.Items))
	}
	ec, _ := f.ExpensesByContact(ctx, adminID, owner, "202506", "", nil, Paging{})
	if cmp.Diff([]string{"101=70.0"}, ids(ec.Items)) != "" {
		t.Errorf("expenses by contact = %v", ids(ec.Items))
	}
}

func TestAging(t *testing.T) {
	f := books()
	f.store.open["accounts_receivable"] = []OpenItem{
		{ContactID: ptr(bob), Date: day("2025-05-10"), Amount: 24200},
		{ContactID: ptr(bob), Date: day("2025-06-01"), Amount: 1000, Paid: 400},
		{ContactID: ptr(alice), Date: day("2025-01-01"), Amount: 12100, Paid: 12100},
		{Date: day("2025-01-01"), Amount: 300},
	}
	r, err := f.DebtorsAging(ctx, adminID, owner, "", Paging{})
	if err != nil {
		t.Fatal(err)
	}
	if !f.store.gotUntil.Equal(day("2025-06-15")) {
		t.Errorf("until = %s", f.store.gotUntil)
	}
	want := []AgingItem{
		{ContactID: ptr(bob), Amount: 24800, Buckets: [4]Bucket{{"0 - 30 dagen", 600}, {"30 - 60 dagen", 24200}, {"Ouder", 0}, {"Totaal", 24800}}},
		{Amount: 300, Buckets: [4]Bucket{{"0 - 30 dagen", 0}, {"30 - 60 dagen", 0}, {"Ouder", 300}, {"Totaal", 300}}},
	}
	if diff := cmp.Diff(want, r.Items); diff != "" || r.Total != 2 {
		t.Errorf("aging: %s", diff)
	}
	if _, err := f.DebtorsAging(ctx, adminID, owner, "202504", Paging{}); err != nil || !f.store.gotUntil.Equal(day("2025-04-30")) {
		t.Errorf("april: %v %s", err, f.store.gotUntil)
	}
	_, err = f.DebtorsAging(ctx, adminID, owner, "next_month", Paging{})
	refusal(t, err, "Period is invalid")

	f.store.open["accounts_payable"] = []OpenItem{{ContactID: ptr(bob), Date: day("2025-06-08"), Amount: -7000, Paid: 2000}}
	f.admins = fakeAdmins{language: "en"}
	c, _ := f.CreditorsAging(ctx, adminID, owner, "this_month", Paging{})
	if len(c.Items) != 1 || c.Items[0].Amount != 5000 || c.Items[0].Buckets[0] != (Bucket{"0 - 30 days", 5000}) {
		t.Errorf("creditors aging = %+v", c.Items)
	}
}

func TestSubscriptions(t *testing.T) {
	f := newFixture()
	jan := day("2025-01-01")
	f.store.subscriptions = []SubscriptionItem{
		{SubscriptionID: 1, ContactID: alice, Start: jan, ItemStart: jan, Quantity: 1000, Price: 1000, Every: 1, Unit: "month"},
		{SubscriptionID: 2, ContactID: alice, Start: jan, ItemStart: jan, Quantity: 1000, Price: 12000, Every: 1, Unit: "year"},
		{SubscriptionID: 3, ContactID: bob, Start: jan, End: ptr(day("2025-06-10")), ItemStart: jan, Quantity: 2000, Discount: 5000, Price: 500, Every: 1, Unit: "month"},
	}
	r, err := f.Subscriptions(ctx, adminID, owner, "this_month")
	if err != nil {
		t.Fatal(err)
	}
	want := SubscriptionFigures{MonthlyRecurringRevenue: 2000, AnnualRunRate: 24000, ChurnRate: "33.33", LifetimeValue: "60.0"}
	if diff := cmp.Diff(want, r); diff != "" {
		t.Errorf("subscriptions: %s", diff)
	}
	may, _ := f.Subscriptions(ctx, adminID, owner, "202505")
	if may.MonthlyRecurringRevenue != 2500 || may.ChurnRate != "0" || may.LifetimeValue != "0" {
		t.Errorf("may = %+v", may)
	}
}

func TestLedgerAccountReport(t *testing.T) {
	f := books()
	if _, _, err := f.LedgerAccount(ctx, adminID, owner, 4242, "202506", 1, 10); !errors.Is(err, ErrLedgerAccountNotFound) {
		t.Errorf("unknown err = %v", err)
	}
	list, total, err := f.LedgerAccount(ctx, adminID, owner, bank, "202506", 1, 1)
	if err != nil || total != 2 || len(list) != 1 || !list[0].Date.Equal(day("2025-06-10")) {
		t.Errorf("bank = %+v %d %v", list, total, err)
	}
}

func unzip(t *testing.T, data []byte, name string) string {
	t.Helper()
	z, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	r, err := z.Open(name)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(r)
	return string(body)
}

func TestExports(t *testing.T) {
	f := books()
	f.store.contacts = []Contact{{ID: alice, Name: "Alice B.V."}, {ID: bob, Name: "Bob & Co"}}
	refusal(t, f.ExportAuditfile(ctx, adminID, owner, "2025", "5.0"), "version does not have a valid value")
	refusal(t, f.ExportAuditfile(ctx, adminID, owner, "twenty", ""), "year does not have a valid value")
	err := f.ExportAuditfile(ctx, adminID, owner, "2023", "3.2")
	refusal(t, err, "Period contains no journal entries")
	if r := (*Refusal)(nil); errors.As(err, &r) && r.Symbolic["period"] != "contains_no_journal_entries" {
		t.Errorf("symbolic = %v", r.Symbolic)
	}
	if err := f.ExportAuditfile(ctx, adminID, owner, "2025", ""); err != nil {
		t.Fatal(err)
	}
	got := f.downloads.added[0]
	xaf := string(got.data)
	for _, want := range []string{
		`<auditfile xmlns="http://www.auditfiles.nl/XAF/3.2">`, "<fiscalYear>2025</fiscalYear>", "<companyName>Example Shop B.V.</companyName>",
		"<custSupName>Bob &amp; Co</custSupName>", "<accID>8000</accID>", "<accTp>P</accTp>", "<opBalDate>2025-01-01</opBalDate>",
		"<totalDebit>434.00</totalDebit>", "<totalDebit>721.00</totalDebit>", "<jrnID>VRK</jrnID>", "<amntTp>C</amntTp>",
	} {
		if !strings.Contains(xaf, want) {
			t.Errorf("auditfile lacks %s", want)
		}
	}
	if got.downloadType != downloads.Auditfile || got.filename != "auditfile_2025.xaf" {
		t.Errorf("download = %+v", got)
	}
	// The opening balance carries 2024's result on equity: 600 + 100.
	if !strings.Contains(xaf, "<accID>0500</accID>\n        <amnt>700.00</amnt>") {
		t.Errorf("opening equity missing:\n%s", xaf)
	}

	f.store.accounts[1].Code = "0100"
	refusal(t, f.ExportAuditfile(ctx, adminID, owner, "2025", ""), "Missing or duplicate account IDs")
	f.store.accounts[1].Code = "0110"

	if err := f.ExportBrugstaat(ctx, adminID, owner, "2025"); err != nil {
		t.Fatal(err)
	}
	bs := string(f.downloads.added[1].data)
	if !strings.Contains(bs, "<rgsCode>WOmzNodOdh</rgsCode>") || !strings.Contains(bs, "<boekjaar>2025</boekjaar>") {
		t.Errorf("brugstaat:\n%s", bs)
	}
	f.store.accounts[0].Taxonomy = "Nope"
	refusal(t, f.ExportBrugstaat(ctx, adminID, owner, "2025"), "Not all categories are linked to a RGS code. This is mandatory before downloading a RGS Brugstaat")
	f.schemes = fakeSchemes{none: true}
	refusal(t, f.ExportBrugstaat(ctx, adminID, owner, "2025"), "Not all categories are linked to a RGS code. This is mandatory before downloading a RGS Brugstaat")

	if err := f.ExportLedgerAccounts(ctx, adminID, owner, "2025"); err != nil {
		t.Fatal(err)
	}
	cards := f.downloads.added[2]
	sheet := unzip(t, cards.data, "xl/worksheets/sheet1.xml")
	if cards.filename != "grootboekkaarten_2025.xlsx" || !strings.Contains(sheet, "Beginsaldo") || !strings.Contains(sheet, "Bob &amp; Co") || !strings.Contains(sheet, "<v>242.00</v>") {
		t.Errorf("ledger cards %s:\n%s", cards.filename, sheet)
	}
	if !strings.Contains(unzip(t, cards.data, "xl/workbook.xml"), `name="Grootboekkaarten"`) {
		t.Error("workbook lacks the sheet")
	}
	if column(0) != "A" || column(25) != "Z" || column(26) != "AA" {
		t.Errorf("columns = %s %s %s", column(0), column(25), column(26))
	}
}

func TestUntilDefaultsToToday(t *testing.T) {
	f := newFixture()
	f.now = func() time.Time { return time.Date(2025, 2, 3, 0, 0, 0, 0, time.UTC) }
	if _, err := f.CreditorsAging(ctx, adminID, owner, "20250131", Paging{}); err != nil || !f.store.gotUntil.Equal(day("2025-01-31")) {
		t.Errorf("day: %v %s", err, f.store.gotUntil)
	}
}
