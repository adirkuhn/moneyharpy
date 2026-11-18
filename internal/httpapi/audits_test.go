package httpapi_test

import (
	"net/http"
	"testing"
)

func TestAuditReport(t *testing.T) {
	h := newHarness(t, options{})
	owner := h.client()
	owner.register()
	adminID, api, supplier, purchase21 := purchaseSetup(t, owner)
	buy := func(price string) {
		wantStatus(t, owner.do(http.MethodPost, api+"/documents/purchase_invoices", map[string]any{"purchase_invoice": map[string]any{
			"contact_id": supplier, "reference": "R-" + price, "date": adminNow().Format("2006-01-02"), "prices_are_incl_tax": false,
			"details_attributes": []map[string]any{{"description": "Paper", "price": price, "amount": 1, "tax_rate_id": purchase21}},
		}}), http.StatusCreated)
	}
	buy("100")

	type line struct {
		LedgerAccountID string   `json:"ledger_account_id"`
		Debit           string   `json:"debit"`
		Credit          string   `json:"credit"`
		TaxRateIDs      []string `json:"tax_rate_ids"`
		Audit           *struct {
			UserID string `json:"user_id"`
			Debit  string `json:"debit"`
		} `json:"audit"`
		Changed bool `json:"changed"`
	}
	type report struct {
		PeriodFrom     string `json:"period_from"`
		LedgerAccounts []line `json:"ledger_accounts"`
	}
	audit := api + "/reports/audit"
	r := decode[report](t, owner.do(http.MethodGet, audit+"?period=this_month", nil))
	var cost line
	for _, l := range r.LedgerAccounts {
		if l.Debit == "100.0" && len(l.TaxRateIDs) == 1 && l.TaxRateIDs[0] == purchase21 {
			cost = l
		}
	}
	if cost.LedgerAccountID == "" || cost.Audit != nil {
		t.Fatalf("report = %+v, want the cost category with 100 debited on the 21%% rate", r)
	}

	marked := decode[line](t, owner.do(http.MethodPost, audit+"/"+cost.LedgerAccountID+"?period=this_month", nil))
	if marked.Audit == nil || marked.Audit.Debit != "100.0" || marked.Changed {
		t.Fatalf("marked = %+v", marked)
	}
	// A booking after the audit shows as a change.
	buy("50")
	for _, l := range decode[report](t, owner.do(http.MethodGet, audit+"?period=this_month", nil)).LedgerAccounts {
		if l.LedgerAccountID == cost.LedgerAccountID && (l.Audit == nil || !l.Changed || l.Debit != "150.0") {
			t.Errorf("after a new booking = %+v, want the audit marked changed", l)
		}
	}
	wantStatus(t, owner.do(http.MethodDelete, audit+"/"+cost.LedgerAccountID+"?period=this_month", nil), http.StatusNoContent)
	for _, l := range decode[report](t, owner.do(http.MethodGet, audit+"?period=this_month", nil)).LedgerAccounts {
		if l.LedgerAccountID == cost.LedgerAccountID && l.Audit != nil {
			t.Errorf("after taking it back = %+v", l)
		}
	}
	wantStatus(t, owner.do(http.MethodPost, audit+"/1?period=this_month", nil), http.StatusNotFound)
	wantStatus(t, owner.do(http.MethodGet, audit+"?period=someday", nil), http.StatusBadRequest)
	_ = adminID
}
