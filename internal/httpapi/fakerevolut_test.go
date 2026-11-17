package httpapi_test

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/adirkuhn/moneyharpy/internal/httpapi"
)

const revolutKey = "revolut-test-key"

// newFakeRevolut stands in for Revolut's Business API, accepting revolutKey.
func newFakeRevolut(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Header.Get("Authorization") != "Bearer "+revolutKey && r.Header.Get("Authorization") != "Bearer "+revolutKey+"-2" {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"message":"Unauthorized"}`))
			return
		}
		switch r.URL.Path {
		case "/accounts":
			_, _ = w.Write([]byte(`[{"id":"acc-eur","name":"Main","balance":250,"currency":"EUR","state":"active","public":false,"account_type":"current","created_at":"2026-01-01T00:00:00Z","updated_at":"2026-01-01T00:00:00Z"}]`))
		case "/accounts/acc-eur/bank-details":
			_, _ = w.Write([]byte(`[{"iban":"NL20INGB0001234567","schemes":["sepa"],"beneficiary":"Example Shop B.V.","beneficiary_address":{"country":"NL","postcode":"1000"},"estimated_time":{"unit":"days"}}]`))
		case "/transactions":
			at := time.Now().Add(-24 * time.Hour).UTC().Format(time.RFC3339)
			_, _ = w.Write([]byte(`[{"id":"tx-1","type":"transfer","state":"completed","created_at":"` + at + `","updated_at":"` + at + `","completed_at":"` + at + `",
				"reference":"Invoice 2026-0001","legs":[{"leg_id":"l1","account_id":"acc-eur","amount":250,"currency":"EUR","description":"Payment from Example Client B.V.","balance":250}]}]`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestRevolutConnectionWithAnAPIKey(t *testing.T) {
	h := newHarness(t, options{})
	owner := h.client()
	owner.register()
	api := "/api/v2/" + owner.createAdministration("Example Shop B.V.").Id

	wantStatus(t, owner.do(http.MethodPost, api+"/bank_connections/api_key", map[string]string{"provider": "revolut", "api_key": "wrong"}), http.StatusUnprocessableEntity)
	r := owner.do(http.MethodPost, api+"/bank_connections/api_key", map[string]string{"provider": "revolut", "api_key": revolutKey})
	wantStatus(t, r, http.StatusCreated)
	c := decode[httpapi.BankConnection](t, r)
	if c.Provider != "revolut" || c.Bank != "Revolut Business" || c.State != "active" || c.ValidUntil != nil || c.NeedsReconnect || len(c.FinancialAccountIds) != 1 || c.LastSyncAt == nil {
		t.Fatalf("connection = %+v", c)
	}
	mutations := decode[[]struct {
		Message string `json:"message"`
	}](t, owner.do(http.MethodGet, api+"/financial_mutations?filter=financial_account_id:"+c.FinancialAccountIds[0], nil))
	if len(mutations) != 1 || mutations[0].Message != "Invoice 2026-0001" {
		t.Errorf("mutations = %+v", mutations)
	}

	// Reconnecting a key connection asks for a new key; replacing it keeps the accounts.
	wantStatus(t, owner.do(http.MethodPost, api+"/bank_connections/"+c.Id+"/reconnect", nil), http.StatusUnprocessableEntity)
	wantStatus(t, owner.do(http.MethodPut, api+"/bank_connections/"+c.Id+"/api_key", map[string]string{"api_key": "wrong"}), http.StatusUnprocessableEntity)
	r = owner.do(http.MethodPut, api+"/bank_connections/"+c.Id+"/api_key", map[string]string{"api_key": revolutKey + "-2"})
	wantStatus(t, r, http.StatusOK)
	if again := decode[httpapi.BankConnection](t, r); again.Id != c.Id || again.FinancialAccountIds[0] != c.FinancialAccountIds[0] {
		t.Errorf("replaced = %+v", again)
	}
	if list := decode[[]httpapi.BankConnection](t, owner.do(http.MethodGet, api+"/bank_connections", nil)); len(list) != 1 || list[0].Provider != "revolut" {
		t.Errorf("list = %+v", list)
	}
}
