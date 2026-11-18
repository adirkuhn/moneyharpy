package httpapi_test

import (
	"net/http"
	"slices"
	"testing"
	"time"
)

type vatReturnBox struct {
	Box  string `json:"box"`
	Base string `json:"base"`
	Tax  string `json:"tax"`
}

type vatReturn struct {
	ID         string         `json:"id"`
	Kind       string         `json:"kind"`
	PeriodFrom string         `json:"period_from"`
	PeriodTo   string         `json:"period_to"`
	Deadline   string         `json:"deadline"`
	State      string         `json:"state"`
	Total      string         `json:"total"`
	Owed       string         `json:"owed"`
	InputTax   string         `json:"input_tax"`
	Unpaid     string         `json:"unpaid"`
	URI        string         `json:"uri"`
	Boxes      []vatReturnBox `json:"boxes"`
	ICPLines   []struct {
		ContactID string `json:"contact_id"`
		Kind      string `json:"kind"`
		Amount    string `json:"amount"`
		VATNumber string `json:"vat_number"`
	} `json:"icp_lines"`
	Checks *struct {
		OpenTransactions int    `json:"open_transactions"`
		UntaxedPurchases string `json:"untaxed_purchases"`
	} `json:"checks"`
	Payments []struct {
		Price string `json:"price"`
	} `json:"payments"`
}

// lastQuarter is the quarter before today's.
func lastQuarter() (time.Time, time.Time) {
	now := adminNow()
	start := time.Date(now.Year(), time.Month((int(now.Month())-1)/3*3+1), 1, 0, 0, 0, 0, time.UTC).AddDate(0, -3, 0)
	return start, start.AddDate(0, 3, -1)
}

func TestVatReturns(t *testing.T) {
	h := newHarness(t, options{})
	owner, banker := h.client(), h.client()
	owner.register()
	banker.register()
	adminID, api, supplier, purchase21 := purchaseSetup(t, owner)
	invite(t, h, owner, banker, adminID, "banker@example.com", map[string]any{"user_type": "employee", "permissions": []string{"documents"}})
	from, to := lastQuarter()
	day := from.AddDate(0, 0, 1).Format(time.DateOnly)
	period := from.Format("20060102") + ".." + to.Format("20060102")
	declarations := api + "/declarations"

	var euRate string
	for _, r := range decode[[]struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	}](t, owner.do(http.MethodGet, api+"/tax_rates", nil)) {
		if r.Name == "Within the EU (reverse-charged)" {
			euRate = r.ID
		}
	}
	shop := decode[contact](t, owner.do(http.MethodPost, api+"/contacts", map[string]any{"contact": map[string]any{"company_name": "Example Shop B.V."}}))
	german := decode[contact](t, owner.do(http.MethodPost, api+"/contacts", map[string]any{"contact": map[string]any{
		"company_name": "Beispiel GmbH", "country": "DE", "tax_number": "DE123456789",
	}}))
	sell := func(customer, rate, price string) {
		inv := decode[salesInvoice](t, owner.do(http.MethodPost, api+"/sales_invoices", map[string]any{"sales_invoice": map[string]any{
			"contact_id": customer, "invoice_date": day,
			"details_attributes": []map[string]any{{"description": "Consulting", "price": price, "amount": 1, "tax_rate_id": rate}},
		}}))
		wantStatus(t, owner.do(http.MethodPatch, api+"/sales_invoices/"+inv.ID+"/send_invoice", map[string]any{"sales_invoice_sending": map[string]any{"delivery_method": "Manual", "invoice_date": day}}), http.StatusOK)
	}
	sell(shop.ID, salesRate(t, owner, adminID, "21"), "200")
	sell(german.ID, euRate, "500")
	wantStatus(t, owner.do(http.MethodPost, api+"/documents/purchase_invoices", map[string]any{"purchase_invoice": map[string]any{
		"contact_id": supplier, "reference": "2026-1", "date": day, "prices_are_incl_tax": false,
		"details_attributes": []map[string]any{{"description": "Delivery", "price": "100", "amount": 1, "tax_rate_id": purchase21}},
	}}), http.StatusCreated)

	wantStatus(t, banker.do(http.MethodGet, declarations, nil), http.StatusForbidden)
	periods := decode[[]struct {
		PeriodFrom  string `json:"period_from"`
		PeriodTo    string `json:"period_to"`
		Granularity string `json:"granularity"`
	}](t, owner.do(http.MethodGet, declarations+"/periods?kind=vat_return", nil))
	if len(periods) == 0 || periods[0].PeriodFrom != from.Format(time.DateOnly) || periods[0].Granularity != "quarter" {
		t.Fatalf("periods = %+v, want the last quarter first", periods)
	}
	wantStatus(t, owner.do(http.MethodGet, declarations+"/periods?kind=other", nil), http.StatusBadRequest)

	preview := decode[vatReturn](t, owner.do(http.MethodGet, declarations+"/preview?kind=vat_return&period="+period, nil))
	want := []vatReturnBox{{"1a", "200.0", "42.0"}, {"3b", "500.0", "0.0"}, {"5b", "0.0", "21.0"}}
	if len(preview.Boxes) != len(want) || preview.Total != "21.0" || preview.Owed != "42.0" || preview.Checks == nil {
		t.Fatalf("preview = %+v", preview)
	}
	for i, b := range want {
		if preview.Boxes[i] != b {
			t.Errorf("box %d = %+v, want %+v", i, preview.Boxes[i], b)
		}
	}
	wantStatus(t, owner.do(http.MethodGet, declarations+"/preview?kind=vat_return&period=20000101..20000331", nil), http.StatusUnprocessableEntity)

	r := owner.do(http.MethodPost, declarations, map[string]any{"vat_return": map[string]any{"kind": "vat_return", "period": period}})
	wantStatus(t, r, http.StatusCreated)
	filed := decode[vatReturn](t, r)
	if filed.State != "to_pay" || filed.Total != "21.0" || filed.URI != "/"+adminID+"/declarations/"+filed.ID ||
		filed.Deadline != to.AddDate(0, 0, 1).AddDate(0, 1, -1).Format(time.DateOnly) {
		t.Errorf("filed = %+v", filed)
	}
	settings := decode[map[string]any](t, owner.do(http.MethodGet, api+"/bookkeeping_settings", nil))
	if settings["period_locked_until"] != to.Format(time.DateOnly) {
		t.Errorf("locked until %v, want %s", settings["period_locked_until"], to.Format(time.DateOnly))
	}
	booked := decode[[]struct {
		Debit  string `json:"debit"`
		Credit string `json:"credit"`
	}](t, owner.do(http.MethodGet, api+"/reports/journal_entries?period="+from.Format("200601")+".."+to.Format("200601")+"&document_type=VatDocument&document_id="+filed.ID, nil))
	if len(booked) != 3 {
		t.Errorf("booked = %+v, want payable, receivable and paid or received VAT", booked)
	}
	if list := decode[[]vatReturn](t, owner.do(http.MethodGet, declarations, nil)); len(list) != 1 || list[0].ID != filed.ID {
		t.Errorf("list = %+v", list)
	}

	// A bank transaction pays it.
	cash := decode[[]financialAccount](t, owner.do(http.MethodGet, api+"/financial_accounts", nil))[0]
	st := decode[statement](t, owner.do(http.MethodPost, api+"/financial_statements", map[string]any{"financial_statement": map[string]any{
		"financial_account_id": cash.ID, "reference": "Tax", "manual": true,
		"financial_mutations_attributes": []map[string]any{{"date": adminNow().Format(time.DateOnly), "amount": -21, "message": "Belastingdienst", "account_servicer_transaction_id": "VAT1"}},
	}}))
	suggested := decode[[]struct {
		BookingType string `json:"booking_type"`
		BookingID   string `json:"booking_id"`
		Price       string `json:"price"`
	}](t, owner.do(http.MethodGet, api+"/financial_mutations/"+st.Mutations[0].ID+"/link_suggestions", nil))
	if !slices.ContainsFunc(suggested, func(s struct {
		BookingType string `json:"booking_type"`
		BookingID   string `json:"booking_id"`
		Price       string `json:"price"`
	}) bool {
		return s.BookingType == "VatDocument" && s.BookingID == filed.ID && s.Price == "21.0"
	}) {
		t.Errorf("suggestions = %+v, want the return", suggested)
	}
	r = owner.do(http.MethodPatch, api+"/financial_mutations/"+st.Mutations[0].ID+"/link_booking", map[string]any{"booking_type": "VatDocument", "booking_id": filed.ID})
	wantStatus(t, r, http.StatusOK)
	if m := decode[mutation](t, r); m.State != "processed" || m.AmountOpen != "0.0" {
		t.Errorf("linked transaction = %+v", m)
	}
	linked := decode[struct {
		Links []struct {
			DocumentType string `json:"document_type"`
			Label        string `json:"label"`
			DocumentID   string `json:"document_id"`
		} `json:"links"`
	}](t, owner.do(http.MethodGet, api+"/financial_mutations/"+st.Mutations[0].ID, nil))
	if len(linked.Links) != 1 || linked.Links[0].DocumentType != "VatDocument" || linked.Links[0].Label != "VAT" || linked.Links[0].DocumentID != filed.ID {
		t.Errorf("links = %+v", linked.Links)
	}
	paid := decode[vatReturn](t, owner.do(http.MethodGet, declarations+"/"+filed.ID, nil))
	if paid.State != "paid" || len(paid.Payments) != 1 || paid.Payments[0].Price != "21.0" || paid.Unpaid != "0.0" {
		t.Errorf("paid = %+v", paid)
	}
	wantStatus(t, owner.do(http.MethodDelete, declarations+"/"+filed.ID, nil), http.StatusUnprocessableEntity)

	// A supplement of the declared quarter has nothing to add.
	sup := decode[vatReturn](t, owner.do(http.MethodGet, declarations+"/preview?kind=supplement&period="+period, nil))
	if len(sup.Boxes) != 0 || sup.Total != "0.0" {
		t.Errorf("supplement preview = %+v", sup)
	}

	// The ICP declaration lists the German customer.
	icp := decode[vatReturn](t, owner.do(http.MethodGet, declarations+"/preview?kind=icp_declaration&period="+period, nil))
	if len(icp.ICPLines) != 1 || icp.ICPLines[0].ContactID != german.ID || icp.ICPLines[0].Amount != "500.0" || icp.ICPLines[0].Kind != "services" {
		t.Fatalf("icp preview = %+v", icp)
	}
	r = owner.do(http.MethodPost, declarations, map[string]any{"vat_return": map[string]any{
		"kind": "icp_declaration", "period": period, "icp_lines": []map[string]any{{"contact_id": german.ID, "kind": "goods"}},
	}})
	wantStatus(t, r, http.StatusCreated)
	got := decode[vatReturn](t, r)
	if got.State != "submitted" || got.ICPLines[0].Kind != "goods" || got.ICPLines[0].VATNumber != "DE123456789" {
		t.Errorf("icp = %+v", got)
	}
	wantStatus(t, owner.do(http.MethodDelete, declarations+"/"+got.ID, nil), http.StatusNoContent)
	wantStatus(t, owner.do(http.MethodGet, declarations+"/"+got.ID, nil), http.StatusNotFound)
	wantStatus(t, owner.do(http.MethodDelete, declarations+"/"+filed.ID+"1", nil), http.StatusNotFound)
}
