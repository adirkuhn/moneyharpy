package httpapi_test

import (
	"net/http"
	"testing"
	"time"

	"github.com/adirkuhn/moneyharpy/internal/httpapi"
)

func TestDetailUsage(t *testing.T) {
	h := newHarness(t, options{})
	owner, stranger := h.client(), h.client()
	owner.register()
	stranger.register()
	adminID, api, supplier, vat21 := purchaseSetup(t, owner)
	phone := category(t, owner, adminID, "Uncategorized expenses")
	wantStatus(t, owner.do(http.MethodPost, api+"/documents/purchase_invoices", map[string]any{"purchase_invoice": map[string]any{
		"contact_id": supplier, "reference": "Bill 1", "date": adminNow().Format(time.DateOnly),
		"details_attributes": []map[string]any{
			{"description": "Paper", "price": "10", "tax_rate_id": vat21, "ledger_account_id": phone},
			{"description": "Ink", "price": "20", "tax_rate_id": vat21, "ledger_account_id": phone},
		},
	}}), http.StatusCreated)

	usage := decode[httpapi.DetailUsage](t, owner.do(http.MethodGet, api+"/details/usage?kind=purchase", nil))
	if len(usage.TaxRates) != 1 || usage.TaxRates[0].Id != vat21 || usage.TaxRates[0].Count != 2 ||
		len(usage.LedgerAccounts) != 1 || usage.LedgerAccounts[0].Id != phone || usage.LedgerAccounts[0].Count != 2 {
		t.Errorf("purchase usage = %+v", usage)
	}
	if sales := decode[httpapi.DetailUsage](t, owner.do(http.MethodGet, api+"/details/usage?kind=sales", nil)); len(sales.TaxRates) != 0 || len(sales.LedgerAccounts) != 0 {
		t.Errorf("sales usage = %+v", sales)
	}
	wantStatus(t, owner.do(http.MethodGet, api+"/details/usage?kind=estimate", nil), http.StatusBadRequest)
	wantStatus(t, owner.do(http.MethodGet, api+"/details/usage", nil), http.StatusBadRequest)
	wantStatus(t, stranger.do(http.MethodGet, api+"/details/usage?kind=sales", nil), http.StatusNotFound)
}
