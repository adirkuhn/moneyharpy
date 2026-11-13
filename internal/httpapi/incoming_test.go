package httpapi_test

import (
	"net/http"
	"strings"
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

type possibleDuplicate struct {
	ID                string   `json:"id"`
	Type              string   `json:"type"`
	Reference         string   `json:"reference"`
	Date              string   `json:"date"`
	TotalPriceInclTax string   `json:"total_price_incl_tax"`
	Reasons           []string `json:"reasons"`
}

func TestIncomingDocumentsWarnOfPossibleDuplicates(t *testing.T) {
	h := newHarness(t, options{})
	owner := h.client()
	owner.register()
	_, api, supplier, vat21 := purchaseSetup(t, owner)
	day := adminNow().Format(time.DateOnly)
	invoice := func(reference string) string {
		t.Helper()
		d := decode[purchaseDocument](t, owner.do(http.MethodPost, api+"/documents/purchase_invoices", map[string]any{"purchase_invoice": map[string]any{
			"contact_id": supplier, "reference": reference, "date": day, "prices_are_incl_tax": true,
			"details_attributes": []map[string]any{{"description": "Paper", "price": "121", "tax_rate_id": vat21}},
		}}))
		return d.ID
	}
	first, again := invoice("INV-2026-0042"), invoice("inv 2026/0042")
	upload := func(name string) string {
		t.Helper()
		d := decode[map[string]any](t, owner.do(http.MethodPost, api+"/documents/typeless_documents", map[string]any{"typeless_document": map[string]any{
			"reference": name, "date": day,
		}}))
		id := d["id"].(string)
		wantStatus(t, owner.attach(api+"/documents/typeless_documents/"+id+"/attachments", name, "application/pdf", []byte("%PDF-1.4 Example Shop B.V.")), http.StatusOK)
		return id
	}
	scan, rescan := upload("scan.pdf"), upload("scan (1).pdf")

	r := owner.do(http.MethodGet, api+"/documents", nil)
	wantStatus(t, r, http.StatusOK)
	byID := map[string][]possibleDuplicate{}
	for _, d := range decode[[]struct {
		ID                 string              `json:"id"`
		PossibleDuplicates []possibleDuplicate `json:"possible_duplicates"`
	}](t, r) {
		byID[d.ID] = d.PossibleDuplicates
	}
	if got := byID[again]; len(got) != 1 || got[0].ID != first || got[0].Type != "PurchaseInvoice" || got[0].TotalPriceInclTax != "121.0" ||
		got[0].Date != day || strings.Join(got[0].Reasons, ",") != "same_reference,same_amount_and_date,same_lines" {
		t.Errorf("duplicates of the second invoice = %+v", got)
	}
	if got := byID[rescan]; len(got) != 1 || got[0].ID != scan || got[0].Type != "TypelessDocument" || strings.Join(got[0].Reasons, ",") != "same_file" {
		t.Errorf("duplicates of the second upload = %+v", got)
	}
	if got := byID[first]; len(got) != 1 || got[0].ID != again {
		t.Errorf("duplicates of the first invoice = %+v", got)
	}

	one := decode[struct {
		PossibleDuplicates []possibleDuplicate `json:"possible_duplicates"`
	}](t, owner.do(http.MethodGet, api+"/documents/"+scan, nil))
	if len(one.PossibleDuplicates) != 1 || one.PossibleDuplicates[0].ID != rescan {
		t.Errorf("duplicates by id = %+v", one.PossibleDuplicates)
	}

	// A document unlike any other has none, as an empty list.
	other := decode[map[string]any](t, owner.do(http.MethodPost, api+"/documents/general_documents", map[string]any{"general_document": map[string]any{
		"reference": "Lease", "date": day,
	}}))
	raw := decode[map[string]any](t, owner.do(http.MethodGet, api+"/documents/"+other["id"].(string), nil))
	if list, ok := raw["possible_duplicates"].([]any); !ok || len(list) != 0 {
		t.Errorf("possible_duplicates = %#v", raw["possible_duplicates"])
	}
}

func TestSetIncomingDocumentType(t *testing.T) {
	h := newHarness(t, options{})
	owner, clerk := h.client(), h.client()
	owner.register()
	clerk.register()
	adminID, api, supplier, vat21 := purchaseSetup(t, owner)
	day := adminNow().Format(time.DateOnly)

	typeless := decode[map[string]any](t, owner.do(http.MethodPost, api+"/documents/typeless_documents", map[string]any{"typeless_document": map[string]any{
		"contact_id": supplier, "reference": "Scan", "date": day,
	}}))
	id := typeless["id"].(string)
	wantStatus(t, owner.attach(api+"/documents/typeless_documents/"+id+"/attachments", "scan.png", "image/png", []byte("\x89PNG\r\n\x1a\nimage")), http.StatusOK)

	r := owner.do(http.MethodPost, api+"/documents/"+id+"/set_type", map[string]any{"document_type": "purchase_invoice"})
	wantStatus(t, r, http.StatusOK)
	got := decode[incomingDocument](t, r)
	if got.ID != id || got.Type != "PurchaseInvoice" || got.State != "new" || got.AttachmentsCount != 1 || deref(got.ContactID) != supplier ||
		got.Reference != "Scan" || got.URI != "/api/v2/"+adminID+"/documents/purchase_invoices/"+id {
		t.Errorf("set type = %+v", got)
	}
	inv := decode[map[string]any](t, owner.do(http.MethodGet, api+"/documents/purchase_invoices/"+id, nil))
	if files, _ := inv["attachments"].([]any); inv["id"] != id || len(files) != 1 {
		t.Errorf("purchase invoice = %+v", inv)
	}
	wantStatus(t, owner.do(http.MethodGet, api+"/documents/typeless_documents/"+id, nil), http.StatusNotFound)

	same := owner.do(http.MethodPost, api+"/documents/"+id+"/set_type", map[string]any{"document_type": "purchase_invoice"})
	wantStatus(t, same, http.StatusOK)
	if d := decode[incomingDocument](t, same); d.Type != "PurchaseInvoice" {
		t.Errorf("same type = %+v", d)
	}

	r = owner.do(http.MethodPost, api+"/documents/"+id+"/set_type", map[string]any{"document_type": "general_journal_document"})
	wantStatus(t, r, http.StatusOK)
	if d := decode[incomingDocument](t, r); d.Type != "GeneralJournalDocument" || d.URI != "/api/v2/"+adminID+"/documents/general_journal_documents/"+id {
		t.Errorf("journal entry = %+v", d)
	}
	wantStatus(t, owner.do(http.MethodPost, api+"/documents/"+id+"/set_type", map[string]any{"document_type": "receipt"}), http.StatusUnprocessableEntity)

	receipt := decode[purchaseDocument](t, owner.do(http.MethodPost, api+"/documents/receipts", map[string]any{"receipt": map[string]any{
		"contact_id": supplier, "reference": "Lunch", "date": day,
		"details_attributes": []map[string]any{{"description": "Lunch", "price": "10", "tax_rate_id": vat21}},
	}}))
	wantStatus(t, owner.do(http.MethodPost, api+"/documents/"+receipt.ID+"/set_type", map[string]any{"document_type": "general_document"}), http.StatusUnprocessableEntity)
	wantStatus(t, owner.do(http.MethodPost, api+"/documents/"+receipt.ID+"/set_type", map[string]any{"document_type": "invoice"}), http.StatusBadRequest)
	wantStatus(t, owner.do(http.MethodPost, api+"/documents/"+supplier+"/set_type", map[string]any{"document_type": "receipt"}), http.StatusNotFound)

	invite(t, h, owner, clerk, adminID, "clerk@example.com", map[string]any{"user_type": "employee", "permissions": []string{"sales_invoices"}})
	wantStatus(t, clerk.do(http.MethodPost, api+"/documents/"+receipt.ID+"/set_type", map[string]any{"document_type": "general_document"}), http.StatusForbidden)
}
