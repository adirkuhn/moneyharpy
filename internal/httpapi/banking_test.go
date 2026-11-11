package httpapi_test

import (
	"bytes"
	"io"
	"net/http"
	"net/url"
	"os"
	"slices"
	"testing"

	"github.com/adirkuhn/moneyharpy/internal/httpapi"
)

type financialAccount struct {
	ID         string `json:"id"`
	Type       string `json:"type"`
	Name       string `json:"name"`
	Identifier string `json:"identifier"`
	Position   int    `json:"position"`
	URI        string `json:"uri"`
	Active     bool   `json:"active"`
}

type mutation struct {
	ID          string `json:"id"`
	Amount      string `json:"amount"`
	AmountOpen  string `json:"amount_open"`
	State       string `json:"state"`
	Message     string `json:"message"`
	StatementID string `json:"financial_statement_id"`
	Bookings    []struct {
		ID            string  `json:"id"`
		Price         string  `json:"price"`
		BookingRuleID *string `json:"booking_rule_id"`
	} `json:"ledger_account_bookings"`
}

type statement struct {
	ID        string     `json:"id"`
	Reference string     `json:"reference"`
	Mutations []mutation `json:"financial_mutations"`
}

// upload sends a raw file body, checked against the contract like do.
func (c *client) upload(path, contentType string, data []byte) response {
	c.t.Helper()
	req, err := http.NewRequest(http.MethodPost, c.base+path, bytes.NewReader(data))
	if err != nil {
		c.t.Fatal(err)
	}
	req.Header.Set("Content-Type", contentType)
	res, err := c.http.Do(req)
	if err != nil {
		c.t.Fatal(err)
	}
	raw, _ := io.ReadAll(res.Body)
	_ = res.Body.Close()
	c.checkContract(req, res, raw)
	return response{status: res.StatusCode, header: res.Header, body: raw}
}

func category(t *testing.T, c *client, admin, name string) string {
	t.Helper()
	all := decode[[]ledgerAccount](t, c.do(http.MethodGet, "/api/v2/"+admin+"/ledger_accounts", nil))
	i := slices.IndexFunc(all, func(l ledgerAccount) bool { return l.Name == name })
	if i < 0 {
		t.Fatalf("no category %s", name)
	}
	return all[i].ID
}

func TestFinancialAccounts(t *testing.T) {
	h := newHarness(t, options{})
	owner, stranger := h.client(), h.client()
	owner.register()
	stranger.register()
	a := owner.createAdministration("Harpy")
	base := "/api/v2/" + a.Id + "/financial_accounts"

	list := decode[[]financialAccount](t, owner.do(http.MethodGet, base+".json", nil))
	if len(list) != 1 || list[0].Type != "CashAccount" {
		t.Fatalf("list = %+v", list)
	}
	r := owner.do(http.MethodPost, base, map[string]any{"financial_account": map[string]any{"name": "Main bank", "identifier": "NL91 ABNA 0417 1643 00"}})
	wantStatus(t, r, http.StatusCreated)
	bank := decode[financialAccount](t, r)
	if bank.Identifier != "NL91ABNA0417164300" || bank.Type != "BankAccount" || bank.URI != "/"+a.Id+"/financial_accounts/"+bank.ID {
		t.Errorf("created = %+v", bank)
	}
	wantStatus(t, owner.do(http.MethodPost, base, map[string]any{"financial_account": map[string]any{"name": "Bad", "identifier": "NL00ABNA0417164300"}}), http.StatusUnprocessableEntity)
	wantStatus(t, owner.do(http.MethodPost, base, map[string]any{"financial_account": map[string]any{"name": "Bad", "type": "Piggy"}}), http.StatusBadRequest)

	wantStatus(t, owner.do(http.MethodPut, base+"/order", map[string]any{"ids": []string{bank.ID, list[0].ID}}), http.StatusNoContent)
	if got := decode[[]financialAccount](t, owner.do(http.MethodGet, base, nil)); got[0].ID != bank.ID {
		t.Errorf("order = %+v", got)
	}
	r = owner.do(http.MethodPatch, base+"/"+bank.ID, map[string]any{"financial_account": map[string]any{"active": false}})
	wantStatus(t, r, http.StatusOK)
	if decode[financialAccount](t, r).Active {
		t.Error("not deactivated")
	}
	wantStatus(t, stranger.do(http.MethodGet, base, nil), http.StatusNotFound)
	wantStatus(t, stranger.do(http.MethodGet, base+"/"+bank.ID, nil), http.StatusNotFound)
}

func TestImportBookAndSynchronize(t *testing.T) {
	h := newHarness(t, options{})
	owner := h.client()
	owner.register()
	a := owner.createAdministration("Harpy")
	api := "/api/v2/" + a.Id
	bank := decode[financialAccount](t, owner.do(http.MethodPost, api+"/financial_accounts", map[string]any{"financial_account": map[string]any{"name": "Main bank", "identifier": "NL91ABNA0417164300"}}))

	software := category(t, owner, a.Id, "Software")
	r := owner.do(http.MethodPost, api+"/booking_rules", map[string]any{"booking_rule": map[string]any{
		"name": "Hosting", "direction": "debit", "ledger_account_id": software,
		"conditions": []map[string]string{{"field": "contra_account_name", "operator": "contains", "value": "hosting"}},
	}})
	wantStatus(t, r, http.StatusCreated)
	rule := decode[httpapi.BookingRule](t, r)

	data, err := os.ReadFile("../bankfile/testdata/camt053.xml")
	if err != nil {
		t.Fatal(err)
	}
	r = owner.upload(api+"/financial_accounts/"+bank.ID+"/import", "application/octet-stream", data)
	wantStatus(t, r, http.StatusCreated)
	res := decode[httpapi.ImportResult](t, r)
	if res.Format != "camt" || res.Imported != 2 || res.AutoBooked != 1 {
		t.Fatalf("import = %+v", res)
	}
	r = owner.upload(api+"/financial_accounts/"+bank.ID+"/import", "application/octet-stream", data)
	if again := decode[httpapi.ImportResult](t, r); again.Imported != 0 || again.Duplicates != 2 {
		t.Errorf("second import = %+v", again)
	}
	wantStatus(t, owner.upload(api+"/financial_accounts/"+bank.ID+"/import", "application/octet-stream", []byte("hello")), http.StatusUnprocessableEntity)

	open := decode[[]mutation](t, owner.do(http.MethodGet, api+"/financial_mutations?filter="+url.QueryEscape("period:202611..202611,state:unprocessed"), nil))
	if len(open) != 1 || open[0].Amount != "1000.0" || open[0].AmountOpen != "1000.0" {
		t.Fatalf("unprocessed = %+v", open)
	}
	auto := decode[[]mutation](t, owner.do(http.MethodGet, api+"/financial_mutations?filter="+url.QueryEscape("period:202611..202611,state:auto_booked"), nil))
	if len(auto) != 1 || *auto[0].Bookings[0].BookingRuleID != rule.Id {
		t.Errorf("auto booked = %+v", auto)
	}
	wantStatus(t, owner.do(http.MethodGet, api+"/financial_mutations?filter=colour:red", nil), http.StatusBadRequest)

	revenue := category(t, owner, a.Id, "Revenue")
	m := open[0]
	r = owner.do(http.MethodPatch, api+"/financial_mutations/"+m.ID+"/link_booking", map[string]any{"booking_type": "LedgerAccount", "booking_id": revenue, "price_base": 600, "description": "Part"})
	wantStatus(t, r, http.StatusOK)
	if got := decode[mutation](t, r); got.AmountOpen != "400.0" || got.State != "unprocessed" {
		t.Errorf("after part = %+v", got)
	}
	r = owner.do(http.MethodPatch, api+"/financial_mutations/"+m.ID+"/link_booking", map[string]any{"booking_type": "LedgerAccount", "booking_id": revenue})
	got := decode[mutation](t, r)
	if got.State != "processed" || len(got.Bookings) != 2 || got.Bookings[1].Price != "400.0" {
		t.Errorf("after rest = %+v", got)
	}
	wantStatus(t, owner.do(http.MethodPatch, api+"/financial_mutations/"+m.ID+"/link_booking", map[string]any{"booking_type": "Document", "booking_id": "1"}), http.StatusUnprocessableEntity)
	r = owner.do(http.MethodDelete, api+"/financial_mutations/"+m.ID+"/unlink_booking", map[string]any{"booking_type": "LedgerAccountBooking", "booking_id": got.Bookings[1].ID})
	wantStatus(t, r, http.StatusOK)
	if got := decode[mutation](t, r); got.State != "unprocessed" || len(got.Bookings) != 1 {
		t.Errorf("after unlink = %+v", got)
	}

	versions := decode[[]httpapi.SynchronizationItem](t, owner.do(http.MethodGet, api+"/financial_mutations/synchronization", nil))
	fetched := decode[[]mutation](t, owner.do(http.MethodPost, api+"/financial_mutations/synchronization", map[string]any{"ids": []any{versions[0].Id, 1}}))
	if len(versions) != 2 || len(fetched) != 1 {
		t.Errorf("versions %d, fetched %d", len(versions), len(fetched))
	}
	wantStatus(t, owner.do(http.MethodGet, api+"/financial_mutations/"+m.ID, nil), http.StatusOK)

	balance := decode[map[string]any](t, owner.do(http.MethodGet, api+"/reports/bank_balance", nil))
	if balance["total"] != "974.5" {
		t.Errorf("balance = %v", balance)
	}
	wantStatus(t, owner.do(http.MethodDelete, api+"/financial_statements/"+m.StatementID, nil), http.StatusUnprocessableEntity)
}

func TestFinancialStatements(t *testing.T) {
	h := newHarness(t, options{})
	owner := h.client()
	owner.register()
	a := owner.createAdministration("Harpy")
	api := "/api/v2/" + a.Id
	cash := decode[[]financialAccount](t, owner.do(http.MethodGet, api+"/financial_accounts", nil))[0]

	r := owner.do(http.MethodPost, api+"/financial_statements", map[string]any{"financial_statement": map[string]any{
		"financial_account_id": cash.ID, "reference": "Till", "official_balance": "-3,50", "manual": true,
		"financial_mutations_attributes": []map[string]any{{"date": "2026-11-01", "amount": -3.5, "message": "Coffee", "account_servicer_transaction_id": "T1"}},
	}})
	wantStatus(t, r, http.StatusCreated)
	st := decode[statement](t, r)
	if st.Reference != "Till" || len(st.Mutations) != 1 || st.Mutations[0].Amount != "-3.5" {
		t.Fatalf("created = %+v", st)
	}
	r = owner.do(http.MethodPatch, api+"/financial_statements/"+st.ID, map[string]any{"financial_statement": map[string]any{
		"reference":                      "Till 2",
		"financial_mutations_attributes": []map[string]any{{"id": st.Mutations[0].ID, "message": "Tea"}, {"date": "2026-11-02", "amount": "5"}},
	}})
	wantStatus(t, r, http.StatusOK)
	if got := decode[statement](t, r); got.Reference != "Till 2" || len(got.Mutations) != 2 {
		t.Errorf("updated = %+v", got)
	}
	wantStatus(t, owner.do(http.MethodGet, api+"/financial_statements/"+st.ID, nil), http.StatusOK)
	wantStatus(t, owner.do(http.MethodPost, api+"/financial_statements", map[string]any{"financial_statement": map[string]any{"financial_account_id": cash.ID}}), http.StatusUnprocessableEntity)
	wantStatus(t, owner.do(http.MethodDelete, api+"/financial_statements/"+st.ID, nil), http.StatusNoContent)
	wantStatus(t, owner.do(http.MethodGet, api+"/financial_statements/"+st.ID, nil), http.StatusNotFound)
}

func TestBankSettingsAndRules(t *testing.T) {
	h := newHarness(t, options{})
	owner := h.client()
	owner.register()
	a := owner.createAdministration("Harpy")
	api := "/api/v2/" + a.Id
	if st := decode[httpapi.BankSettings](t, owner.do(http.MethodGet, api+"/bank_settings", nil)); !st.AutomaticBooking {
		t.Error("automatic booking off by default")
	}
	r := owner.do(http.MethodPatch, api+"/bank_settings", map[string]any{"bank_settings": map[string]any{"automatic_booking": false}})
	if decode[httpapi.BankSettings](t, r).AutomaticBooking {
		t.Error("automatic booking still on")
	}
	software := category(t, owner, a.Id, "Software")
	rule := decode[httpapi.BookingRule](t, owner.do(http.MethodPost, api+"/booking_rules", map[string]any{"booking_rule": map[string]any{
		"name": "Hosting", "ledger_account_id": software, "conditions": []map[string]string{{"field": "amount", "operator": "greater_than", "value": "10"}},
	}}))
	r = owner.do(http.MethodPatch, api+"/booking_rules/"+rule.Id, map[string]any{"booking_rule": map[string]any{"match_all": false, "financial_account_id": ""}})
	wantStatus(t, r, http.StatusOK)
	wantStatus(t, owner.do(http.MethodPost, api+"/booking_rules", map[string]any{"booking_rule": map[string]any{"name": "x", "ledger_account_id": software, "conditions": []map[string]string{}}}), http.StatusUnprocessableEntity)
	if list := decode[[]httpapi.BookingRule](t, owner.do(http.MethodGet, api+"/booking_rules", nil)); len(list) != 1 || list[0].MatchAll {
		t.Errorf("rules = %+v", list)
	}
	wantStatus(t, owner.do(http.MethodDelete, api+"/booking_rules/"+rule.Id, nil), http.StatusNoContent)
	wantStatus(t, owner.do(http.MethodGet, api+"/booking_rules/"+rule.Id, nil), http.StatusNotFound)
}
