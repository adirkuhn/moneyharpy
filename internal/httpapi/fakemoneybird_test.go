package httpapi_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
)

const moneybirdToken = "moneybird-test-token"

// newFakeMoneybird stands in for Moneybird's API, accepting moneybirdToken,
// with one administration of a contact, a product and a project.
func newFakeMoneybird(t *testing.T) *httptest.Server {
	t.Helper()
	synced := map[string]map[string]string{
		"contacts": {
			"504000000000000501": `{"id":"504000000000000501","company_name":"Example Shop B.V.","customer_id":"7","invoice_workflow_id":"504000000000000581","custom_fields":[{"id":"504000000000000571","name":"Account manager","value":"Ann"}],
				"notes":[{"id":"504000000000000711","note":"Prefers email","todo":false,"created_at":"2024-03-05T10:00:00.000Z"}],"contact_people":[{"id":"504000000000000601","firstname":"Ann","lastname":"de Vries"}],"created_at":"2024-03-01T10:00:00.000Z","updated_at":"2024-03-02T10:00:00.000Z"}`,
			"504000000000000502": `{"id":"504000000000000502","firstname":"Jan","lastname":"Voorbeeld","send_invoices_to_email":"not an address"}`,
			"504000000000000503": `{"id":"504000000000000503","company_name":"Beispiel GmbH","send_invoices_to_email":"Ann de Vries <ann@example.com>; billing@example.com","sepa_bic":"NOT A BIC"}`,
			"504000000000000504": `{"id":"504000000000000504"}`,
		},
		"financial_mutations": {
			"504000000000000301": `{"id":"504000000000000301","financial_account_id":"504000000000000201","financial_statement_id":"504000000000000401","date":"2026-01-15","message":"Invoice 2026-0001","amount":"12.1","contra_account_name":"Example Shop B.V."}`,
			"504000000000000302": `{"id":"504000000000000302","financial_account_id":"504000000000000201","financial_statement_id":"504000000000000401","date":"2026-01-20","message":"Rent","amount":"-50.0",
				"ledger_account_bookings":[{"id":"504000000000000312","ledger_account_id":"504000000000000904","description":"Rent January","price":"-50.0"}]}`,
			"504000000000000303": `{"id":"504000000000000303","financial_account_id":"504000000000000201","financial_statement_id":"504000000000000402","date":"2026-02-10","message":"PUR-77","amount":"-121.0"}`,
		},
		"sales_invoices": {
			"504000000000000111": `{"id":"504000000000000111","contact_id":"504000000000000501","invoice_id":"2026-0001","state":"paid","original_estimate_id":"504000000000000141","workflow_id":"504000000000000581",
				"attachments":[{"id":"504000000000000171","filename":"terms.pdf","content_type":"application/pdf"}],"invoice_date":"2026-01-10","due_date":"2026-01-24","currency":"EUR","prices_are_incl_tax":false,"discount":"0.0","sent_at":"2026-01-10T09:00:00.000Z",
				"details":[{"id":"504000000000000112","description":"Hosting","amount":"1","price":"10.0","tax_rate_id":"504000000000000911","ledger_account_id":"504000000000000902"}],
				"payments":[{"id":"504000000000000113","payment_date":"2026-01-15","price":"12.1","price_base":"12.1","financial_account_id":"504000000000000201","financial_mutation_id":"504000000000000301"}]}`,
			"504000000000000121": `{"id":"504000000000000121","contact_id":"504000000000000501","state":"draft","draft_id":3,"original_estimate_id":"504000000000000149","recurring_sales_invoice_id":"504000000000000159","details":[{"id":"504000000000000122","description":"Support","amount":"2","price":"50","tax_rate_id":"504000000000000911"}],"payments":[]}`,
		},
		"estimates": {
			"504000000000000141": `{"id":"504000000000000141","contact_id":"504000000000000501","estimate_id":"2026-0001","state":"accepted","estimate_date":"2026-01-05","due_date":"2026-02-04","currency":"EUR","discount":"0.0","prices_are_incl_tax":false,"sent_at":"2026-01-05T09:00:00.000Z","accepted_at":"2026-01-08",
				"details":[{"id":"504000000000000142","description":"Hosting","amount":"1","price":"10.0","tax_rate_id":"504000000000000911"}],"attachments":[]}`,
		},
		"recurring_sales_invoices": {
			"504000000000000151": `{"id":"504000000000000151","contact_id":"504000000000000501","start_date":"2026-01-01","invoice_date":"2026-12-01","frequency_type":"month","frequency":1,"active":true,"auto_send":true,"currency":"EUR","prices_are_incl_tax":false,
				"details":[{"id":"504000000000000152","description":"Monthly hosting","amount":"1","price":"10.0","tax_rate_id":"504000000000000911"}]}`,
		},
		"documents/purchase_invoices": {
			"504000000000000161": `{"id":"504000000000000161","contact_id":"504000000000000501","reference":"PUR-77","date":"2026-02-01","due_date":"2026-02-15","entry_number":5,"currency":"EUR","prices_are_incl_tax":false,
				"details":[{"id":"504000000000000162","description":"Paper","amount":"1","price":"100","tax_rate_id":"504000000000000912","ledger_account_id":"504000000000000904"}],
				"payments":[{"id":"504000000000000163","payment_date":"2026-02-10","price":"121.0","price_base":"121.0","financial_account_id":"504000000000000201","financial_mutation_id":"504000000000000303"}],"attachments":[]}`,
		},
		"documents/receipts": {},
		"documents/general_documents": {
			"504000000000000181": `{"id":"504000000000000181","contact_id":"504000000000000501","reference":"Contract","date":"2026-01-02","entry_number":6,
				"attachments":[{"id":"504000000000000182","filename":"contract.pdf","content_type":"application/pdf"}]}`,
		},
		"documents/typeless_documents": {
			"504000000000000191": `{"id":"504000000000000191","reference":"Scan","date":"2026-03-03","attachments":[{"id":"504000000000000192","filename":"scan.png","content_type":"image/png"}]}`,
		},
		"documents/general_journal_documents": {
			"504000000000000195": `{"id":"504000000000000195","reference":"Correction","date":"2026-03-01",
				"general_journal_document_entries":[
					{"id":"504000000000000196","ledger_account_id":"504000000000000904","debit":"25.0","credit":"0.0","row_order":1,"description":"Rent correction"},
					{"id":"504000000000000197","ledger_account_id":"504000000000000902","debit":"0.0","credit":"25.0","row_order":2,"description":"Rent correction"}],"attachments":[]}`,
		},
		"external_sales_invoices": {
			"504000000000000131": `{"id":"504000000000000131","contact_id":"504000000000000501","date":"2026-02-01","due_date":"2026-02-15","reference":"WEB-1001","entry_number":4,"currency":"EUR","prices_are_incl_tax":false,
				"details":[{"id":"504000000000000132","description":"Webshop order","amount":"1","price":"50","tax_rate_id":"504000000000000911","ledger_account_id":"504000000000000902"}],"payments":[]}`,
		},
	}
	sync := func(w http.ResponseWriter, r *http.Request, kind string) {
		records := synced[kind]
		if r.Method == http.MethodGet {
			filter := r.URL.Query().Get("filter")
			switch {
			case kind == "recurring_sales_invoices":
				if filter == "state:inactive" {
					records = nil
				} else if filter != "state:active" {
					t.Errorf("recurring invoices synchronized with %q", filter)
				}
			case kind != "contacts" && filter != "state:all":
				t.Errorf("%s synchronized without lifting the default period", kind)
			}
			var ids []string
			for id := range records {
				ids = append(ids, `{"id":"`+id+`","version":1}`)
			}
			slices.Sort(ids)
			_, _ = w.Write([]byte("[" + strings.Join(ids, ",") + "]"))
			return
		}
		raw, _ := io.ReadAll(r.Body)
		var body struct {
			IDs []string `json:"ids"`
		}
		_ = json.Unmarshal(raw, &body)
		var out []string
		for _, id := range body.IDs {
			if c, ok := records[id]; ok {
				out = append(out, c)
			}
		}
		_, _ = w.Write([]byte("[" + strings.Join(out, ",") + "]"))
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("RateLimit-RequestsRemaining", "140")
		if strings.HasPrefix(r.URL.Path, "/signed/") {
			// Moneybird's PDFs come from a signed link elsewhere, which needs no token.
			if r.Header.Get("Authorization") != "" {
				t.Errorf("the token went along to %s", r.URL.Path)
			}
			w.Header().Set("Content-Type", "application/pdf")
			_, _ = w.Write([]byte("%PDF-1.4 Moneybird"))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if r.Header.Get("Authorization") != "Bearer "+moneybirdToken {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"error":"Token is invalid"}`))
			return
		}
		path := strings.TrimPrefix(r.URL.Path, "/api/v2")
		switch {
		case strings.HasSuffix(path, "/download_pdf"):
			http.Redirect(w, r, "/signed/document.pdf", http.StatusFound)
			return
		case strings.HasSuffix(path, "/download") && strings.Contains(path, "/attachments/"):
			if strings.Contains(path, "typeless") {
				w.Header().Set("Content-Type", "image/png")
				_, _ = w.Write([]byte("\x89PNG\r\n\x1a\nfake"))
				return
			}
			w.Header().Set("Content-Type", "application/pdf")
			_, _ = w.Write([]byte("%PDF-1.4 attachment"))
			return
		}
		switch path {
		case "/administrations.json":
			_, _ = w.Write([]byte(`[{"id":123,"name":"Example Shop B.V.","country":"NL","currency":"EUR","period_locked_until":"2026-01-31"}]`))
		case "/123/ledger_accounts.json":
			_, _ = w.Write([]byte(`[
				{"id":"504000000000000901","name":"Debiteuren","account_type":"current_assets","account_id":"13011","active":true,"taxonomy_item":{"code":"BVorDebHad"}},
				{"id":"504000000000000902","name":"Omzet webshop","account_type":"revenue","account_id":"8010","active":true},
				{"id":"504000000000000903","name":"Zakelijke rekening","account_type":"current_assets","account_id":"1010","active":true,"financial_account_id":"504000000000000201"},
				{"id":"504000000000000904","name":"Huur kantoor","account_type":"expenses","account_id":"4111","active":true,"taxonomy_item":{"code":"WBedHuiBeh"}},
				{"id":"504000000000000905","name":"Rent","account_type":"expenses","account_id":"4110","active":true},
				{"id":"504000000000000906","name":"Diversen","account_type":"expenses","account_id":"4900","active":true},
				{"id":"504000000000000907","name":"Diversen","account_type":"expenses","account_id":"4901","active":true}]`))
		case "/123/tax_rates.json":
			_, _ = w.Write([]byte(`[{"id":"504000000000000911","name":"21% btw","percentage":"21.0","tax_rate_type":"sales_invoice","show_tax":true,"active":true,"country":null},
				{"id":"504000000000000912","name":"21% btw","percentage":"21.0","tax_rate_type":"purchase_invoice","show_tax":true,"active":true,"country":null}]`))
		case "/123/contacts/synchronization.json", "/123/financial_mutations/synchronization.json", "/123/sales_invoices/synchronization.json",
			"/123/external_sales_invoices/synchronization.json", "/123/estimates/synchronization.json", "/123/recurring_sales_invoices/synchronization.json",
			"/123/documents/purchase_invoices/synchronization.json", "/123/documents/receipts/synchronization.json", "/123/documents/general_documents/synchronization.json",
			"/123/documents/typeless_documents/synchronization.json", "/123/documents/general_journal_documents/synchronization.json":
			sync(w, r, strings.TrimSuffix(strings.TrimPrefix(strings.TrimPrefix(r.URL.Path, "/api/v2"), "/123/"), "/synchronization.json"))
		case "/123/custom_fields.json":
			_, _ = w.Write([]byte(`[{"id":"504000000000000571","name":"Account manager","source":"contact"}]`))
		case "/123/identities/default.json":
			_, _ = w.Write([]byte(`{"id":"504000000000000721","company_name":"Example Shop B.V.","address1":"Hoofdstraat 1","zipcode":"1234 AB","city":"Utrecht","country":"NL","email":"info@example.com"}`))
		case "/123/identities.json":
			_, _ = w.Write([]byte(`[{"id":"504000000000000722","company_name":"Example Shop Webwinkel","city":"Utrecht","country":"NL","custom_fields":[{"id":"504000000000000579","name":"Not imported","value":"x"}]}]`))
		case "/123/workflows.json":
			_, _ = w.Write([]byte(`[{"id":"504000000000000581","type":"InvoiceWorkflow","name":"Strict reminders","default":false,"currency":"EUR","language":"nl","active":true,"prices_are_incl_tax":false,
				"steps":[{"due_interval":14,"reminder_delay":7,"reminder_text":"Please pay","reminder_auto_send":true}]}]`))
		case "/123/users.json":
			_, _ = w.Write([]byte(`[{"id":"504000000000000731","name":"Someone","email":"someone@example.org"}]`))
		case "/123/time_entries.json":
			if f := r.URL.Query().Get("filter"); !strings.Contains(f, "state:all") {
				t.Errorf("time entries listed with %q", f)
			}
			_, _ = w.Write([]byte(`[{"id":"504000000000000741","user_id":"504000000000000731","contact_id":"504000000000000501","project_id":"504000000000000801","started_at":"2026-02-02T09:00:00.000Z","ended_at":"2026-02-02T11:00:00.000Z","description":"Design work","billable":true,"notes":[]}]`))
		case "/123/task_list_templates.json":
			_, _ = w.Write([]byte(`[{"id":"504000000000000751","name":"Month close","period":"P1M","groups":[{"id":"504000000000000752","name":"Bank","row_order":0,"tasks":[{"id":"504000000000000753","name":"Check the bank","row_order":0,"subtasks":[]}]}]}]`))
		case "/123/task_lists.json":
			_, _ = w.Write([]byte(`[{"id":"504000000000000761","name":"January close","period_begin":"2026-01-01","period_end":"2026-01-31","groups":[{"id":"504000000000000762","name":"Bank","row_order":0,
				"tasks":[{"id":"504000000000000763","name":"Check the bank","row_order":0,"completed_at":"2026-02-03T10:00:00.000Z","subtasks":[]}]}]}]`))
		case "/123/financial_accounts.json":
			_, _ = w.Write([]byte(`[{"id":"504000000000000201","type":"BankAccount","name":"Zakelijke rekening","identifier":"NL91ABNA0417164300","currency":"EUR","active":true},
				{"id":"504000000000000202","type":"MoneybirdPaymentsAccount","name":null,"identifier":"MB-PAY","currency":"EUR","active":true},
				{"id":"504000000000000203","type":"BankAccount","name":"zakelijke rekening","identifier":"NL20INGB0001234567","currency":"EUR","active":true}]`))
		case "/123/products.json":
			_, _ = w.Write([]byte(`[{"id":"504000000000000701","title":"Hosting","description":"Hosting","price":"10.0","currency":"EUR","tax_rate_id":"504000000000000911","ledger_account_id":"504000000000000902"}]`))
		case "/123/projects.json":
			_, _ = w.Write([]byte(`[{"id":"504000000000000801","name":"Website","state":"active","budget":40}]`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}
