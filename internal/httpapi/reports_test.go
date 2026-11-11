package httpapi_test

import (
	"bytes"
	"net/http"
	"strconv"
	"testing"
	"time"
)

type reportItem struct {
	ContactID *string `json:"contact_id"`
	ProjectID *string `json:"project_id"`
	Amount    string  `json:"amount"`
	Buckets   []struct {
		Name   string `json:"name"`
		Amount string `json:"amount"`
	} `json:"aging_buckets"`
}

func TestReports(t *testing.T) {
	h := newHarness(t, options{})
	owner, banker := h.client(), h.client()
	owner.register()
	banker.register()
	adminID, api, supplier, purchase21 := purchaseSetup(t, owner)
	invite(t, h, owner, banker, adminID, "banker@example.com", map[string]any{"user_type": "employee", "permissions": []string{"bank"}})
	today := adminNow().Format(time.DateOnly)
	customer := decode[contact](t, owner.do(http.MethodPost, api+"/contacts", map[string]any{"contact": map[string]any{"company_name": "Example Shop B.V."}}))
	app := decode[project](t, owner.do(http.MethodPost, api+"/projects", map[string]any{"project": map[string]any{"name": "Web app"}}))
	revenue := category(t, owner, adminID, "Revenue")

	inv := decode[salesInvoice](t, owner.do(http.MethodPost, api+"/sales_invoices", map[string]any{"sales_invoice": map[string]any{
		"contact_id": customer.ID, "invoice_date": today,
		"details_attributes": []map[string]any{{"description": "Consulting", "price": "200", "amount": 1, "tax_rate_id": salesRate(t, owner, adminID, "21"), "ledger_account_id": revenue, "project_id": app.ID}},
	}}))
	wantStatus(t, owner.do(http.MethodPatch, api+"/sales_invoices/"+inv.ID+"/send_invoice", map[string]any{"sales_invoice_sending": map[string]any{"delivery_method": "Manual"}}), http.StatusOK)
	wantStatus(t, owner.do(http.MethodPost, api+"/sales_invoices/"+inv.ID+"/payments", map[string]any{"payment": map[string]any{"payment_date": today, "price": "42", "manual_payment_action": "cash_payment"}}), http.StatusCreated)
	wantStatus(t, owner.do(http.MethodPost, api+"/documents/purchase_invoices", map[string]any{"purchase_invoice": map[string]any{
		"contact_id": supplier, "reference": "2026-1", "date": today, "prices_are_incl_tax": false,
		"details_attributes": []map[string]any{{"description": "Delivery", "price": "100", "amount": 1, "tax_rate_id": purchase21}},
	}}), http.StatusCreated)

	reports := api + "/reports/"
	pl := decode[map[string]any](t, owner.do(http.MethodGet, reports+"profit_loss.json?period=this_month&granularity=month", nil))
	if pl["total_revenue"] != "200.0" || pl["gross_profit"] != "200.0" || pl["net_profit"] != "100.0" || len(pl["periods"].([]any)) != 1 {
		t.Errorf("profit and loss = %v", pl)
	}
	onApp := decode[map[string]any](t, owner.do(http.MethodGet, reports+"profit_loss?project_id="+app.ID, nil))
	if onApp["total_expenses"] != "0.0" || onApp["total_revenue"] != "200.0" {
		t.Errorf("on project = %v", onApp)
	}
	r := owner.do(http.MethodGet, reports+"profit_loss?period=202401..202506", nil)
	wantStatus(t, r, http.StatusBadRequest)
	if got := decode[map[string]any](t, r); got["error"] != "Period cannot exceed 12 months" {
		t.Errorf("long period = %v", got)
	}
	wantStatus(t, owner.do(http.MethodGet, reports+"tax?period=soon", nil), http.StatusBadRequest)

	type side struct {
		Total string `json:"total"`
	}
	bs := decode[struct {
		Debit  side `json:"debit"`
		Credit struct {
			side
			OpenYears []struct{ Year, Value string } `json:"open_years"`
		} `json:"credit"`
	}](t, owner.do(http.MethodGet, reports+"balance_sheet", nil))
	if bs.Debit.Total != bs.Credit.Total || bs.Debit.Total == "0.0" || len(bs.Credit.OpenYears) != 1 || bs.Credit.OpenYears[0].Value != "100.0" {
		t.Errorf("balance sheet = %+v", bs)
	}

	tax := decode[struct {
		TaxRates []struct{ Type, Amount, Tax string } `json:"tax_rates"`
	}](t, owner.do(http.MethodGet, reports+"tax", nil))
	if len(tax.TaxRates) != 2 || tax.TaxRates[0].Type != "sales_invoice" || tax.TaxRates[0].Tax != "42.0" || tax.TaxRates[1].Amount != "100.0" || tax.TaxRates[1].Tax != "21.0" {
		t.Errorf("tax = %+v", tax)
	}

	debtors := owner.do(http.MethodGet, reports+"debtors?period=this_month&granularity=month", nil)
	got := decode[struct {
		Debtors []reportItem `json:"debtors"`
		Periods []struct {
			Period  string       `json:"period"`
			Debtors []reportItem `json:"debtors"`
		} `json:"periods"`
	}](t, debtors)
	if len(got.Debtors) != 1 || *got.Debtors[0].ContactID != customer.ID || got.Debtors[0].Amount != "200.0" || len(got.Periods) != 1 || debtors.header.Get("X-Total-Count") != "1" {
		t.Errorf("debtors = %+v", got)
	}
	creditors := decode[map[string][]reportItem](t, owner.do(http.MethodGet, reports+"creditors", nil))
	if c := creditors["creditors"]; len(c) != 1 || c[0].Amount != "121.0" {
		t.Errorf("creditors = %+v", creditors)
	}
	aging := decode[map[string][]reportItem](t, owner.do(http.MethodGet, reports+"debtors_aging.json?period_until=this_month", nil))
	if d := aging["debtors"]; len(d) != 1 || d[0].Amount != "200.0" || len(d[0].Buckets) != 4 || d[0].Buckets[0].Amount != "200.0" || d[0].Buckets[3].Amount != "200.0" {
		t.Errorf("debtors aging = %+v", aging)
	}
	if c := decode[map[string][]reportItem](t, owner.do(http.MethodGet, reports+"creditors_aging", nil))["creditors"]; len(c) != 1 || c[0].Amount != "121.0" {
		t.Errorf("creditors aging = %+v", c)
	}
	wantStatus(t, owner.do(http.MethodGet, reports+"debtors_aging?period_until=next_month", nil), http.StatusBadRequest)

	byContact := decode[map[string][]reportItem](t, owner.do(http.MethodGet, reports+"revenue_by_contact?contact_id="+customer.ID, nil))
	if c := byContact["revenue_by_contact"]; len(c) != 1 || c[0].Amount != "200.0" {
		t.Errorf("revenue by contact = %+v", byContact)
	}
	if c := decode[map[string][]reportItem](t, owner.do(http.MethodGet, reports+"expenses_by_contact", nil))["expenses_by_contact"]; len(c) != 1 || *c[0].ContactID != supplier {
		t.Errorf("expenses by contact = %+v", c)
	}
	if p := decode[map[string][]reportItem](t, owner.do(http.MethodGet, reports+"revenue_by_project", nil))["revenue_by_project"]; len(p) != 1 || *p[0].ProjectID != app.ID {
		t.Errorf("revenue by project = %+v", p)
	}
	if p := decode[map[string][]reportItem](t, owner.do(http.MethodGet, reports+"expenses_by_project", nil))["expenses_by_project"]; len(p) != 1 || p[0].ProjectID != nil || p[0].Amount != "100.0" {
		t.Errorf("expenses by project = %+v", p)
	}

	wc := decode[map[string]string](t, owner.do(http.MethodGet, reports+"working_capital", nil))
	if wc["working_capital"] != "100.0" || wc["to_pay"] != "142.0" {
		t.Errorf("working capital = %v", wc)
	}
	cash := decode[map[string]any](t, owner.do(http.MethodGet, reports+"cash_flow", nil))
	if cash["opening_balance"] != "0.0" || cash["closing_balance"] != "0.0" {
		t.Errorf("cash flow = %v", cash)
	}
	if subs := decode[map[string]string](t, owner.do(http.MethodGet, reports+"subscriptions", nil)); subs["monthly_recurring_revenue"] != "0.0" || subs["churn_rate"] != "0" {
		t.Errorf("subscriptions = %v", subs)
	}

	entries := owner.do(http.MethodGet, reports+"ledger_accounts/"+revenue, nil)
	if list := decode[[]map[string]any](t, entries); len(list) != 1 || list[0]["amount"] != "-200.0" {
		t.Errorf("ledger account = %v", list)
	}
	missing := owner.do(http.MethodGet, reports+"ledger_accounts/1", nil)
	wantStatus(t, missing, http.StatusNotFound)
	if got := decode[map[string]any](t, missing); got["error"] != "Record not found for model name: Category" {
		t.Errorf("missing = %v", got)
	}

	// Moneybird asks for every permission an operation names.
	wantStatus(t, banker.do(http.MethodGet, reports+"balance_sheet", nil), http.StatusOK)
	wantStatus(t, banker.do(http.MethodGet, reports+"profit_loss", nil), http.StatusForbidden)
	wantStatus(t, banker.do(http.MethodPost, reports+"export/auditfile", map[string]any{"year": "2026"}), http.StatusForbidden)

	year := strconv.Itoa(adminNow().Year())
	wantStatus(t, owner.do(http.MethodPost, reports+"export/auditfile", map[string]any{"year": year, "version": "5.0"}), http.StatusBadRequest)
	none := owner.do(http.MethodPost, reports+"export/auditfile.json", map[string]any{"year": "2001"})
	wantStatus(t, none, http.StatusBadRequest)
	if got := decode[map[string]any](t, none); got["error"] != "Period contains no journal entries" || got["symbolic"].(map[string]any)["period"] != "contains_no_journal_entries" {
		t.Errorf("empty year = %v", got)
	}
	wantStatus(t, owner.do(http.MethodPost, reports+"export/auditfile", map[string]any{"year": year}), http.StatusNoContent)
	wantStatus(t, owner.do(http.MethodPost, reports+"export/brugstaat", map[string]any{"year": year}), http.StatusNoContent)
	wantStatus(t, owner.do(http.MethodPost, reports+"export/ledger_accounts", map[string]any{"year": year}), http.StatusNoContent)
	wantStatus(t, owner.do(http.MethodPost, reports+"export/ledger_accounts", map[string]any{"year": "2001"}), http.StatusBadRequest)

	files := decode[[]struct {
		ID           string `json:"id"`
		DownloadType string `json:"download_type"`
		Filename     string `json:"filename"`
	}](t, owner.do(http.MethodGet, api+"/downloads", nil))
	types := map[string]string{}
	for _, f := range files {
		types[f.DownloadType] = f.ID
	}
	if len(files) != 3 || types["auditfile"] == "" || types["brugstaat"] == "" || types["export_ledger_account_report"] == "" {
		t.Fatalf("downloads = %+v", files)
	}
	xaf := owner.follow(owner.do(http.MethodPost, api+"/downloads/"+types["auditfile"]+"/download", nil))
	if !bytes.Contains(xaf.body, []byte(`<auditfile xmlns="http://www.auditfiles.nl/XAF/3.2">`)) || !bytes.Contains(xaf.body, []byte("<companyName>Harpy</companyName>")) {
		t.Errorf("auditfile = %s", xaf.body)
	}
	xlsx := owner.follow(owner.do(http.MethodPost, api+"/downloads/"+types["export_ledger_account_report"]+"/download", nil))
	if !bytes.HasPrefix(xlsx.body, []byte("PK")) {
		t.Errorf("ledger cards are no workbook: %q", xlsx.body[:min(10, len(xlsx.body))])
	}
}
