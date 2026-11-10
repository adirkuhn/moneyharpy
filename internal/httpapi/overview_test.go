package httpapi_test

import (
	"net/http"
	"os"
	"testing"

	"github.com/adirkuhn/moneyharpy/internal/httpapi"
)

func TestOverview(t *testing.T) {
	h := newHarness(t, options{})
	owner, clerk, stranger := h.client(), h.client(), h.client()
	owner.register()
	clerk.register()
	stranger.register()
	a := owner.createAdministration("Harpy")
	api := "/api/v2/" + a.Id

	empty := decode[httpapi.Overview](t, owner.do(http.MethodGet, api+"/overview", nil))
	if empty.Bank == nil || empty.Bank.Balance != "0.0" || empty.Bank.UnprocessedCount != 0 || len(empty.RecentlyProcessed) != 0 {
		t.Errorf("new administration = %+v", empty)
	}

	bank := decode[financialAccount](t, owner.do(http.MethodPost, api+"/financial_accounts", map[string]any{"financial_account": map[string]any{"name": "Main bank", "identifier": "NL91ABNA0417164300"}}))
	software := category(t, owner, a.Id, "Software")
	wantStatus(t, owner.do(http.MethodPost, api+"/booking_rules", map[string]any{"booking_rule": map[string]any{
		"name": "Hosting", "direction": "debit", "ledger_account_id": software,
		"conditions": []map[string]string{{"field": "contra_account_name", "operator": "contains", "value": "hosting"}},
	}}), http.StatusCreated)
	data, err := os.ReadFile("../bankfile/testdata/camt053.xml")
	if err != nil {
		t.Fatal(err)
	}
	wantStatus(t, owner.upload(api+"/financial_accounts/"+bank.ID+"/import", "application/octet-stream", data), http.StatusCreated)

	got := decode[httpapi.Overview](t, owner.do(http.MethodGet, api+"/overview", nil))
	if got.Bank == nil || got.Bank.Balance != "974.5" || got.Bank.Currency != "EUR" || got.Bank.UnprocessedCount != 1 {
		t.Fatalf("bank = %+v", got.Bank)
	}
	if len(got.RecentlyProcessed) != 1 {
		t.Fatalf("recently processed = %+v", got.RecentlyProcessed)
	}
	item := got.RecentlyProcessed[0]
	if item.Type != "financial_mutation" || item.State != "processed" || item.Amount != "-25.5" || item.Uri == "" {
		t.Errorf("item = %+v", item)
	}

	// Without the bank permission, or the bank scope, there is no bank block.
	invite(t, h, owner, clerk, a.Id, "clerk@example.com", map[string]any{"user_type": "employee", "permissions": []string{"sales_invoices"}})
	if seen := decode[httpapi.Overview](t, clerk.do(http.MethodGet, api+"/overview", nil)); seen.Bank != nil || len(seen.RecentlyProcessed) != 0 {
		t.Errorf("clerk sees %+v", seen)
	}
	app := h.client()
	app.token = connect(t, owner, a.Id, map[string]any{"kind": "api", "name": "Shop", "scopes": []string{"documents"}, "read_only": true}).Token
	if seen := decode[httpapi.Overview](t, app.do(http.MethodGet, api+"/overview", nil)); seen.Bank != nil {
		t.Errorf("token without bank scope sees %+v", seen.Bank)
	}
	wantStatus(t, stranger.do(http.MethodGet, api+"/overview", nil), http.StatusNotFound)
}
