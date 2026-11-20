package httpapi_test

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/adirkuhn/moneyharpy/internal/httpapi"
)

func TestImportFromMoneybird(t *testing.T) {
	h := newHarness(t, options{})
	owner, clerk := h.client(), h.client()
	owner.register()
	clerk.register()
	a := owner.createAdministration("Harpy")
	api := "/api/v2/" + a.Id
	invite(t, h, owner, clerk, a.Id, "clerk@example.com", map[string]any{"user_type": "employee", "permissions": []string{"settings"}})

	// Only owners import: it replaces the categories and VAT rates.
	wantStatus(t, clerk.do(http.MethodPost, api+"/moneybird_import/administrations", map[string]any{"token": moneybirdToken}), http.StatusForbidden)
	wantStatus(t, owner.do(http.MethodPost, api+"/moneybird_import/administrations", map[string]any{"token": "wrong"}), http.StatusUnprocessableEntity)
	list := decode[[]httpapi.MoneybirdAdministration](t, owner.do(http.MethodPost, api+"/moneybird_import/administrations", map[string]any{"token": moneybirdToken}))
	if len(list) != 1 || list[0].Id != "123" || list[0].Name != "Example Shop B.V." {
		t.Fatalf("administrations = %+v", list)
	}
	wantStatus(t, owner.do(http.MethodGet, api+"/moneybird_import", nil), http.StatusNotFound)

	start := map[string]any{"moneybird_import": map[string]any{"token": moneybirdToken, "administration_id": "123"}}
	r := owner.do(http.MethodPost, api+"/moneybird_import", start)
	wantStatus(t, r, http.StatusCreated)
	if strings.Contains(string(r.body), moneybirdToken) {
		t.Error("the token is shown")
	}
	started := decode[httpapi.MoneybirdImport](t, r)
	if started.State != "running" || len(started.Steps) != 8 {
		t.Errorf("started = %+v", started)
	}
	if err := h.mbimport.Work(context.Background()); err != nil {
		t.Fatal(err)
	}
	done := decode[httpapi.MoneybirdImport](t, owner.do(http.MethodGet, api+"/moneybird_import", nil))
	if done.State != "done" || done.FinishedAt == nil {
		t.Fatalf("import = %+v", done)
	}
	for _, st := range done.Steps {
		if st.Kind == "contacts" && (st.Imported != 1 || st.Failed != 1 || !strings.Contains(st.Problems[0], "504000000000000502")) {
			t.Errorf("contacts step = %+v", st)
		}
	}

	// Records keep their Moneybird ids.
	c := decode[map[string]any](t, owner.do(http.MethodGet, api+"/contacts/504000000000000501", nil))
	people, _ := c["contact_people"].([]any)
	if c["customer_id"] != "7" || len(people) != 1 || people[0].(map[string]any)["id"] != "504000000000000601" {
		t.Errorf("contact = %+v", c)
	}
	p := decode[map[string]any](t, owner.do(http.MethodGet, api+"/products/504000000000000701", nil))
	if p["ledger_account_id"] != "504000000000000902" || p["tax_rate_id"] == nil {
		t.Errorf("product = %+v", p)
	}
	wantStatus(t, owner.do(http.MethodGet, api+"/projects/504000000000000801", nil), http.StatusOK)
	// Moneybird's receivables took the default's place: one category, with Moneybird's name.
	accounts := decode[[]map[string]any](t, owner.do(http.MethodGet, api+"/ledger_accounts", nil))
	receivables := 0
	for _, l := range accounts {
		if l["name"] == "Debiteuren" {
			receivables++
		}
	}
	if receivables != 1 {
		t.Errorf("%d categories named Debiteuren", receivables)
	}

	// The bank account keeps its id and its imported category; the transaction is its statement's.
	account := decode[map[string]any](t, owner.do(http.MethodGet, api+"/financial_accounts/504000000000000201", nil))
	if account["name"] != "Zakelijke rekening" || account["identifier"] != "NL91ABNA0417164300" {
		t.Errorf("account = %+v", account)
	}
	// The paid invoice keeps its number and dates, and its payment is made with the transaction.
	paid := decode[map[string]any](t, owner.do(http.MethodGet, api+"/sales_invoices/504000000000000111", nil))
	if paid["invoice_id"] != "2026-0001" || paid["state"] != "paid" || paid["invoice_date"] != "2026-01-10" || paid["due_date"] != "2026-01-24" {
		t.Errorf("paid invoice = %+v", paid)
	}
	mutation := decode[map[string]any](t, owner.do(http.MethodGet, api+"/financial_mutations/504000000000000301", nil))
	if mutation["state"] != "processed" || mutation["financial_statement_id"] != "504000000000000401" {
		t.Errorf("transaction = %+v", mutation)
	}
	draft := decode[map[string]any](t, owner.do(http.MethodGet, api+"/sales_invoices/504000000000000121", nil))
	if draft["state"] != "draft" || draft["invoice_id"] != nil || draft["total_price_excl_tax"] != "100.0" {
		t.Errorf("draft = %+v", draft)
	}
	external := decode[map[string]any](t, owner.do(http.MethodGet, api+"/external_sales_invoices/504000000000000131", nil))
	if external["reference"] != "WEB-1001" || external["entry_number"] != float64(4) || external["state"] == "new" {
		t.Errorf("external = %+v", external)
	}
	// Nothing was booked twice: the debtors are what the open external invoice owes.
	debtors := decode[map[string]any](t, owner.do(http.MethodGet, api+"/reports/debtors", nil))
	if !strings.Contains(fmt.Sprint(debtors), "60.5") {
		t.Errorf("debtors = %+v", debtors)
	}

	// Again, only what is new would come in; another Moneybird administration cannot mix in.
	wantStatus(t, owner.do(http.MethodPost, api+"/moneybird_import", start), http.StatusCreated)
	other := map[string]any{"moneybird_import": map[string]any{"token": moneybirdToken, "administration_id": "456"}}
	_ = h.mbimport.Work(context.Background())
	wantStatus(t, owner.do(http.MethodPost, api+"/moneybird_import", other), http.StatusUnprocessableEntity)
}
