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
	me := owner.register()
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
	if started.State != "running" || len(started.Steps) != 22 {
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
	// Names clash neither with the defaults nor with each other.
	seen := map[string]bool{}
	for _, l := range accounts {
		key := fmt.Sprint(l["account_type"], "/", strings.ToLower(fmt.Sprint(l["name"])))
		if seen[key] {
			t.Errorf("two categories named %s", key)
		}
		seen[key] = true
	}
	if !seen["expenses/diversen (4901)"] {
		t.Errorf("the second Diversen is not told apart: %v", seen)
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

	// The accepted estimate keeps its number; the invoice billed from it points to it.
	estimate := decode[map[string]any](t, owner.do(http.MethodGet, api+"/estimates/504000000000000141", nil))
	if estimate["estimate_id"] != "2026-0001" || estimate["state"] != "accepted" {
		t.Errorf("estimate = %+v", estimate)
	}
	if paid["original_estimate_id"] != "504000000000000141" {
		t.Errorf("invoice does not point to its estimate: %v", paid["original_estimate_id"])
	}
	// The sent invoice keeps its own attachment and the PDF Moneybird sent.
	files := fmt.Sprint(paid["attachments"])
	if !strings.Contains(files, "terms.pdf") || !strings.Contains(files, "2026-0001 (Moneybird).pdf") {
		t.Errorf("invoice files = %s", files)
	}
	// The recurring invoice comes in stopped, so it bills nobody twice.
	recurring := decode[map[string]any](t, owner.do(http.MethodGet, api+"/recurring_sales_invoices/504000000000000151", nil))
	if recurring["active"] != false || recurring["auto_send"] != false {
		t.Errorf("recurring = %+v", recurring)
	}
	// The purchase invoice is paid with its transaction; the rent is booked on its category.
	purchase := decode[map[string]any](t, owner.do(http.MethodGet, api+"/documents/purchase_invoices/504000000000000161", nil))
	if purchase["state"] != "paid" || purchase["entry_number"] != float64(5) {
		t.Errorf("purchase invoice = %+v", purchase)
	}
	for _, id := range []string{"504000000000000302", "504000000000000303"} {
		if m := decode[map[string]any](t, owner.do(http.MethodGet, api+"/financial_mutations/"+id, nil)); m["state"] != "processed" {
			t.Errorf("transaction %s = %+v", id, m)
		}
	}
	general := decode[map[string]any](t, owner.do(http.MethodGet, api+"/documents/general_documents/504000000000000181", nil))
	if !strings.Contains(fmt.Sprint(general["attachments"]), "contract.pdf") {
		t.Errorf("general document = %+v", general)
	}
	typeless := decode[map[string]any](t, owner.do(http.MethodGet, api+"/documents/typeless_documents/504000000000000191", nil))
	if !strings.Contains(fmt.Sprint(typeless["attachments"]), "scan.png") {
		t.Errorf("typeless document = %+v", typeless)
	}
	entry := decode[map[string]any](t, owner.do(http.MethodGet, api+"/documents/general_journal_documents/504000000000000195", nil))
	if rows, _ := entry["general_journal_document_entries"].([]any); len(rows) != 2 || rows[0].(map[string]any)["id"] != "504000000000000196" {
		t.Errorf("journal entry = %+v", entry)
	}
	// The books are locked as they were in Moneybird, once all came in.
	settings := decode[map[string]any](t, owner.do(http.MethodGet, api+"/bookkeeping_settings", nil))
	if settings["period_locked_until"] != "2026-01-31" {
		t.Errorf("locked until %v", settings["period_locked_until"])
	}

	// The settings around the invoices came first: the custom field, the sender address and the workflow.
	contact := decode[map[string]any](t, owner.do(http.MethodGet, api+"/contacts/504000000000000501", nil))
	if contact["invoice_workflow_id"] != "504000000000000581" || !strings.Contains(fmt.Sprint(contact["custom_fields"]), "Ann") || !strings.Contains(fmt.Sprint(contact["notes"]), "Prefers email") {
		t.Errorf("contact settings = %v, %v, %v", contact["invoice_workflow_id"], contact["custom_fields"], contact["notes"])
	}
	own := decode[map[string]any](t, owner.do(http.MethodGet, api+"/identities/default", nil))
	if own["address1"] != "Hoofdstraat 1" || own["email"] != "info@example.com" {
		t.Errorf("own identity = %+v", own)
	}
	wantStatus(t, owner.do(http.MethodGet, api+"/identities/504000000000000722", nil), http.StatusOK)
	// Reminders do not send by themselves while Moneybird may still send them.
	flow := decode[map[string]any](t, owner.do(http.MethodGet, api+"/workflows/504000000000000581", nil))
	if !strings.Contains(fmt.Sprint(flow["steps"]), "reminder_auto_send:false") || paid["workflow_id"] != "504000000000000581" {
		t.Errorf("workflow = %+v, invoice workflow %v", flow, paid["workflow_id"])
	}
	// Time of a Moneybird user who is no member here is the owner's.
	timed := decode[map[string]any](t, owner.do(http.MethodGet, api+"/time_entries/504000000000000741", nil))
	if timed["description"] != "Design work" || timed["user_id"] != me.Id {
		t.Errorf("time entry = %+v", timed)
	}
	tasks := decode[map[string]any](t, owner.do(http.MethodGet, api+"/task_lists/504000000000000761", nil))
	if !strings.Contains(fmt.Sprint(tasks["groups"]), "completed_at:2026-02-03") {
		t.Errorf("task list = %+v", tasks)
	}
	wantStatus(t, owner.do(http.MethodGet, api+"/task_list_templates/504000000000000751", nil), http.StatusOK)

	// Again, only what is new would come in; another Moneybird administration cannot mix in.
	wantStatus(t, owner.do(http.MethodPost, api+"/moneybird_import", start), http.StatusCreated)
	other := map[string]any{"moneybird_import": map[string]any{"token": moneybirdToken, "administration_id": "456"}}
	_ = h.mbimport.Work(context.Background())
	wantStatus(t, owner.do(http.MethodPost, api+"/moneybird_import", other), http.StatusUnprocessableEntity)
}
