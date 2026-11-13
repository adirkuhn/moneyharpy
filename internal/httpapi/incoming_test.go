package httpapi_test

import (
	"net/http"
	"testing"
	"time"
)

type incomingDocument struct {
	ID                string  `json:"id"`
	Type              string  `json:"type"`
	Reference         string  `json:"reference"`
	ContactID         *string `json:"contact_id"`
	ContactName       string  `json:"contact_name"`
	State             string  `json:"state"`
	Currency          string  `json:"currency"`
	TotalPriceInclTax string  `json:"total_price_incl_tax"`
	AttachmentsCount  int     `json:"attachments_count"`
	URI               string  `json:"uri"`
}

func TestIncomingDocuments(t *testing.T) {
	h := newHarness(t, options{})
	owner, clerk := h.client(), h.client()
	owner.register()
	clerk.register()
	adminID, api, supplier, vat21 := purchaseSetup(t, owner)
	day := adminNow().Format(time.DateOnly)

	receipt := decode[purchaseDocument](t, owner.do(http.MethodPost, api+"/documents/receipts", map[string]any{"receipt": map[string]any{
		"contact_id": supplier, "reference": "Lunch", "date": day, "prices_are_incl_tax": true,
		"details_attributes": []map[string]any{{"description": "Lunch", "price": "25.92", "tax_rate_id": vat21}},
	}}))
	general := decode[map[string]any](t, owner.do(http.MethodPost, api+"/documents/general_documents", map[string]any{"general_document": map[string]any{
		"reference": "Lease", "date": day,
	}}))
	typeless := decode[map[string]any](t, owner.do(http.MethodPost, api+"/documents/typeless_documents", map[string]any{"typeless_document": map[string]any{
		"reference": "Scan", "date": day,
	}}))
	wantStatus(t, owner.attach(api+"/documents/typeless_documents/"+typeless["id"].(string)+"/attachments", "scan.png", "image/png", []byte("\x89PNG\r\n\x1a\nimage")), http.StatusOK)

	all := owner.do(http.MethodGet, api+"/documents", nil)
	wantStatus(t, all, http.StatusOK)
	list := decode[[]incomingDocument](t, all)
	if len(list) != 3 || all.header.Get("X-Total-Count") != "3" {
		t.Fatalf("incoming = %+v", list)
	}
	byType := map[string]incomingDocument{}
	for _, d := range list {
		byType[d.Type] = d
	}
	r := byType["Receipt"]
	if r.ID != receipt.ID || r.ContactName != "Supplier B.V." || deref(r.ContactID) != supplier || r.TotalPriceInclTax != "25.92" || r.Currency != "EUR" ||
		r.URI != "/api/v2/"+adminID+"/documents/receipts/"+receipt.ID {
		t.Errorf("receipt = %+v", r)
	}
	if g := byType["GeneralDocument"]; g.State != "saved" || g.TotalPriceInclTax != "0.0" || g.ContactID != nil || g.ContactName != "" || g.Currency != "EUR" {
		t.Errorf("general = %+v", g)
	}
	if tl := byType["TypelessDocument"]; tl.AttachmentsCount != 1 || tl.State != "new" || tl.URI != "/api/v2/"+adminID+"/documents/typeless_documents/"+typeless["id"].(string) {
		t.Errorf("typeless = %+v", tl)
	}

	page := owner.do(http.MethodGet, api+"/documents?filter=type:receipt|general_document&per_page=1&page=2&sort=date_asc", nil)
	if got := decode[[]incomingDocument](t, page); len(got) != 1 || page.header.Get("X-Total-Count") != "2" || page.header.Get("Link") == "" {
		t.Errorf("page = %+v, links %q", got, page.header.Get("Link"))
	}
	wantStatus(t, owner.do(http.MethodGet, api+"/documents?filter=type:invoice", nil), http.StatusBadRequest)

	got := decode[incomingDocument](t, owner.do(http.MethodGet, api+"/documents/"+general["id"].(string), nil))
	if got.Type != "GeneralDocument" || got.Reference != "Lease" {
		t.Errorf("by id = %+v", got)
	}
	wantStatus(t, owner.do(http.MethodGet, api+"/documents/"+supplier, nil), http.StatusNotFound)

	stats := decode[struct {
		Currency      string `json:"currency"`
		CostsExclTax  string `json:"costs_excl_tax"`
		AmountPayable string `json:"amount_payable"`
		Categories    []struct {
			LedgerAccountID *string `json:"ledger_account_id"`
			Name            string  `json:"name"`
			Amount          string  `json:"amount"`
		} `json:"categories"`
	}](t, owner.do(http.MethodGet, api+"/documents/statistics?filter=state:open|late|paid", nil))
	if stats.Currency != "EUR" || stats.CostsExclTax != "21.42" || stats.AmountPayable != "25.92" || len(stats.Categories) != 1 ||
		stats.Categories[0].LedgerAccountID == nil || stats.Categories[0].Amount != "21.42" {
		t.Errorf("statistics = %+v", stats)
	}
	wantStatus(t, owner.do(http.MethodGet, api+"/documents/statistics?filter=period:nonsense", nil), http.StatusBadRequest)

	invite(t, h, owner, clerk, adminID, "clerk@example.com", map[string]any{"user_type": "employee", "permissions": []string{"sales_invoices"}})
	wantStatus(t, clerk.do(http.MethodGet, api+"/documents", nil), http.StatusForbidden)
	wantStatus(t, clerk.do(http.MethodGet, api+"/documents/statistics", nil), http.StatusForbidden)
	wantStatus(t, clerk.do(http.MethodGet, api+"/documents/"+receipt.ID, nil), http.StatusForbidden)
}
