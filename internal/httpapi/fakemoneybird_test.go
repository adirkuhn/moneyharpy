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
			"504000000000000501": `{"id":"504000000000000501","company_name":"Example Shop B.V.","customer_id":"7","contact_people":[{"id":"504000000000000601","firstname":"Ann","lastname":"de Vries"}],"created_at":"2024-03-01T10:00:00.000Z","updated_at":"2024-03-02T10:00:00.000Z"}`,
			"504000000000000502": `{"id":"504000000000000502","firstname":"Jan","lastname":"Voorbeeld","send_invoices_to_email":"not an address"}`,
		},
		"financial_mutations": {
			"504000000000000301": `{"id":"504000000000000301","financial_account_id":"504000000000000201","financial_statement_id":"504000000000000401","date":"2026-01-15","message":"Invoice 2026-0001","amount":"12.1","contra_account_name":"Example Shop B.V."}`,
		},
		"sales_invoices": {
			"504000000000000111": `{"id":"504000000000000111","contact_id":"504000000000000501","invoice_id":"2026-0001","state":"paid","invoice_date":"2026-01-10","due_date":"2026-01-24","currency":"EUR","prices_are_incl_tax":false,"discount":"0.0","sent_at":"2026-01-10T09:00:00.000Z",
				"details":[{"id":"504000000000000112","description":"Hosting","amount":"1","price":"10.0","tax_rate_id":"504000000000000911","ledger_account_id":"504000000000000902"}],
				"payments":[{"id":"504000000000000113","payment_date":"2026-01-15","price":"12.1","price_base":"12.1","financial_account_id":"504000000000000201","financial_mutation_id":"504000000000000301"}]}`,
			"504000000000000121": `{"id":"504000000000000121","contact_id":"504000000000000501","state":"draft","draft_id":3,"details":[{"id":"504000000000000122","description":"Support","amount":"2","price":"50","tax_rate_id":"504000000000000911"}],"payments":[]}`,
		},
		"external_sales_invoices": {
			"504000000000000131": `{"id":"504000000000000131","contact_id":"504000000000000501","date":"2026-02-01","due_date":"2026-02-15","reference":"WEB-1001","entry_number":4,"currency":"EUR","prices_are_incl_tax":false,
				"details":[{"id":"504000000000000132","description":"Webshop order","amount":"1","price":"50","tax_rate_id":"504000000000000911","ledger_account_id":"504000000000000902"}],"payments":[]}`,
		},
	}
	sync := func(w http.ResponseWriter, r *http.Request, kind string) {
		records := synced[kind]
		if r.Method == http.MethodGet {
			if kind != "contacts" && r.URL.Query().Get("filter") != "state:all" {
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
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("RateLimit-RequestsRemaining", "140")
		if r.Header.Get("Authorization") != "Bearer "+moneybirdToken {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"error":"Token is invalid"}`))
			return
		}
		switch strings.TrimPrefix(r.URL.Path, "/api/v2") {
		case "/administrations.json":
			_, _ = w.Write([]byte(`[{"id":123,"name":"Example Shop B.V.","country":"NL","currency":"EUR"}]`))
		case "/123/ledger_accounts.json":
			_, _ = w.Write([]byte(`[
				{"id":"504000000000000901","name":"Debiteuren","account_type":"current_assets","account_id":"13011","active":true,"taxonomy_item":{"code":"BVorDebHad"}},
				{"id":"504000000000000902","name":"Omzet webshop","account_type":"revenue","account_id":"8010","active":true},
				{"id":"504000000000000903","name":"Zakelijke rekening","account_type":"current_assets","account_id":"1010","active":true,"financial_account_id":"504000000000000201"}]`))
		case "/123/tax_rates.json":
			_, _ = w.Write([]byte(`[{"id":"504000000000000911","name":"21% btw","percentage":"21.0","tax_rate_type":"sales_invoice","show_tax":true,"active":true,"country":null}]`))
		case "/123/contacts/synchronization.json", "/123/financial_mutations/synchronization.json", "/123/sales_invoices/synchronization.json", "/123/external_sales_invoices/synchronization.json":
			sync(w, r, strings.TrimSuffix(strings.TrimPrefix(strings.TrimPrefix(r.URL.Path, "/api/v2"), "/123/"), "/synchronization.json"))
		case "/123/financial_accounts.json":
			_, _ = w.Write([]byte(`[{"id":"504000000000000201","type":"BankAccount","name":"Zakelijke rekening","identifier":"NL91ABNA0417164300","currency":"EUR","active":true}]`))
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
