package httpapi_test

import (
	"net/http"
	"testing"
)

func TestEmptyAnAdministration(t *testing.T) {
	h := newHarness(t, options{})
	owner, clerk := h.client(), h.client()
	owner.register()
	clerk.register()
	a := owner.createAdministration("Harpy")
	api := "/api/v2/" + a.Id
	invite(t, h, owner, clerk, a.Id, "clerk@example.com", map[string]any{"user_type": "employee", "permissions": []string{"settings"}})

	defaults := len(decode[[]map[string]any](t, owner.do(http.MethodGet, api+"/ledger_accounts", nil)))
	contact := decode[map[string]any](t, owner.do(http.MethodPost, api+"/contacts", map[string]any{"contact": map[string]any{"company_name": "Example Shop B.V."}}))
	wantStatus(t, owner.do(http.MethodPost, api+"/ledger_accounts", map[string]any{"ledger_account": map[string]any{"name": "Hosting", "account_type": "expenses"}, "rgs_code": "WBedKanSof"}), http.StatusCreated)
	wantStatus(t, owner.do(http.MethodPost, api+"/projects", map[string]any{"project": map[string]any{"name": "Website"}}), http.StatusCreated)
	account := decode[map[string]any](t, owner.do(http.MethodPost, api+"/financial_accounts", map[string]any{"financial_account": map[string]any{"name": "Main bank", "identifier": "NL91ABNA0417164300"}}))
	wantStatus(t, owner.do(http.MethodPost, api+"/financial_statements", map[string]any{"financial_statement": map[string]any{
		"financial_account_id": account["id"], "reference": "S1", "financial_mutations_attributes": []any{map[string]any{"date": "2026-01-15", "message": "Rent", "amount": "-50"}},
	}}), http.StatusCreated)
	field := decode[map[string]any](t, owner.do(http.MethodPost, api+"/custom_fields", map[string]any{"custom_field": map[string]any{"name": "Account manager", "source": "contact"}}))

	// Only owners, and only with the name.
	wantStatus(t, clerk.do(http.MethodPost, "/api/v2/administrations/"+a.Id+"/empty", map[string]any{"name": "Harpy"}), http.StatusForbidden)
	wantStatus(t, owner.do(http.MethodPost, "/api/v2/administrations/"+a.Id+"/empty", map[string]any{"name": "harpy"}), http.StatusUnprocessableEntity)
	wantStatus(t, owner.do(http.MethodGet, api+"/contacts/"+contact["id"].(string), nil), http.StatusOK)
	wantStatus(t, owner.do(http.MethodPost, "/api/v2/administrations/"+a.Id+"/empty", map[string]any{"name": "Harpy"}), http.StatusNoContent)

	// The data is gone.
	wantStatus(t, owner.do(http.MethodGet, api+"/contacts/"+contact["id"].(string), nil), http.StatusNotFound)
	for _, path := range []string{"/contacts", "/projects", "/financial_mutations"} {
		if list := decode[[]map[string]any](t, owner.do(http.MethodGet, api+path, nil)); len(list) != 0 {
			t.Errorf("%s still has %d", path, len(list))
		}
	}
	// The books start again from the defaults, with the cash account a new administration has.
	if got := len(decode[[]map[string]any](t, owner.do(http.MethodGet, api+"/ledger_accounts", nil))); got != defaults {
		t.Errorf("%d categories, a new administration has %d", got, defaults)
	}
	if accounts := decode[[]map[string]any](t, owner.do(http.MethodGet, api+"/financial_accounts", nil)); len(accounts) != 1 || accounts[0]["type"] != "CashAccount" {
		t.Errorf("financial accounts = %+v", accounts)
	}
	// The setup stays: users, the custom field, the company details.
	wantStatus(t, owner.do(http.MethodGet, api+"/custom_fields/"+field["id"].(string), nil), http.StatusOK)
	wantStatus(t, clerk.do(http.MethodGet, "/api/v2/administrations/"+a.Id, nil), http.StatusOK)
	wantStatus(t, owner.do(http.MethodGet, api+"/identities/default", nil), http.StatusOK)
	// And it can be filled again.
	wantStatus(t, owner.do(http.MethodPost, api+"/contacts", map[string]any{"contact": map[string]any{"company_name": "Example Shop B.V."}}), http.StatusCreated)
}
