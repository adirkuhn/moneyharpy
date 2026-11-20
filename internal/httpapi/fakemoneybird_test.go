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
	contacts := map[string]string{
		"504000000000000501": `{"id":"504000000000000501","company_name":"Example Shop B.V.","customer_id":"7","contact_people":[{"id":"504000000000000601","firstname":"Ann","lastname":"de Vries"}],"created_at":"2024-03-01T10:00:00.000Z","updated_at":"2024-03-02T10:00:00.000Z"}`,
		"504000000000000502": `{"id":"504000000000000502","firstname":"Jan","lastname":"Voorbeeld","send_invoices_to_email":"not an address"}`,
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
				{"id":"504000000000000902","name":"Omzet webshop","account_type":"revenue","account_id":"8010","active":true}]`))
		case "/123/tax_rates.json":
			_, _ = w.Write([]byte(`[{"id":"504000000000000911","name":"21% btw","percentage":"21.0","tax_rate_type":"sales_invoice","show_tax":true,"active":true,"country":null}]`))
		case "/123/contacts/synchronization.json":
			if r.Method == http.MethodGet {
				_, _ = w.Write([]byte(`[{"id":"504000000000000501","version":1},{"id":"504000000000000502","version":1}]`))
				return
			}
			raw, _ := io.ReadAll(r.Body)
			var body struct {
				IDs []string `json:"ids"`
			}
			_ = json.Unmarshal(raw, &body)
			var out []string
			for _, id := range body.IDs {
				if c, ok := contacts[id]; ok {
					out = append(out, c)
				}
			}
			slices.Sort(out)
			_, _ = w.Write([]byte("[" + strings.Join(out, ",") + "]"))
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
