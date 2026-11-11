package httpapi_test

import (
	"bytes"
	"net/http"
	"strings"
	"testing"
)

type salesInvoice struct {
	ID          string  `json:"id"`
	InvoiceID   *string `json:"invoice_id"`
	DraftID     *int    `json:"draft_id"`
	State       string  `json:"state"`
	InvoiceDate *string `json:"invoice_date"`
	DueDate     string  `json:"due_date"`
	TotalIncl   string  `json:"total_price_incl_tax"`
	TotalExcl   string  `json:"total_price_excl_tax"`
	TotalUnpaid string  `json:"total_unpaid"`
	OriginalID  *string `json:"original_sales_invoice_id"`
	Paused      bool    `json:"paused"`
	Contact     struct {
		ID string `json:"id"`
	} `json:"contact"`
	Details []struct {
		ID              string `json:"id"`
		Amount          string `json:"amount"`
		LedgerAccountID string `json:"ledger_account_id"`
	} `json:"details"`
	TaxTotals []struct {
		TaxAmount string `json:"tax_amount"`
	} `json:"tax_totals"`
}

type salesTaxRate struct {
	ID         string `json:"id"`
	Percentage string `json:"percentage"`
	Type       string `json:"tax_rate_type"`
}

func salesRate(t *testing.T, c *client, admin, percentage string) string {
	t.Helper()
	for _, r := range decode[[]salesTaxRate](t, c.do(http.MethodGet, "/api/v2/"+admin+"/tax_rates", nil)) {
		if r.Type == "sales_invoice" && strings.TrimSuffix(r.Percentage, ".0") == percentage {
			return r.ID
		}
	}
	t.Fatalf("no sales rate of %s%%", percentage)
	return ""
}

func TestSalesInvoiceLifecycle(t *testing.T) {
	h := newHarness(t, options{})
	owner := h.client()
	owner.register()
	a := owner.createAdministration("Harpy")
	api := "/api/v2/" + a.Id
	invoices := api + "/sales_invoices"
	customer := decode[contact](t, owner.do(http.MethodPost, api+"/contacts", map[string]any{"contact": map[string]any{
		"company_name": "Example Shop B.V.", "send_invoices_to_email": "billing@example.com",
	}}))
	vat21 := salesRate(t, owner, a.Id, "21")
	revenue := category(t, owner, a.Id, "Revenue")

	created := owner.do(http.MethodPost, invoices, map[string]any{"sales_invoice": map[string]any{
		"contact_id": customer.ID, "reference": "Order 7",
		"details_attributes": map[string]any{
			"0": map[string]any{"description": "Consulting", "amount": "2 hours", "price": "100", "tax_rate_id": vat21, "ledger_account_id": revenue},
			"1": map[string]any{"description": "Travel", "amount": 1, "price": 25.5, "tax_rate_id": vat21},
		},
	}})
	wantStatus(t, created, http.StatusCreated)
	inv := decode[salesInvoice](t, created)
	// 200.00 + 25.50 = 225.50, 21% VAT 47.36, total 272.86.
	if inv.State != "draft" || inv.InvoiceID != nil || inv.DraftID == nil || inv.TotalExcl != "225.5" || inv.TotalIncl != "272.86" || inv.Contact.ID != customer.ID {
		t.Fatalf("draft = %+v", inv)
	}
	if inv.Details[1].LedgerAccountID == "" || inv.Details[1].LedgerAccountID == revenue {
		t.Errorf("uncategorized line = %+v", inv.Details[1])
	}

	sent := owner.do(http.MethodPatch, invoices+"/"+inv.ID+"/send_invoice", map[string]any{"sales_invoice_sending": map[string]any{"delivery_method": "Email"}})
	wantStatus(t, sent, http.StatusOK)
	inv = decode[salesInvoice](t, sent)
	if inv.State != "open" || inv.InvoiceID == nil || !strings.HasSuffix(*inv.InvoiceID, "-0001") || inv.TotalUnpaid != "272.86" {
		t.Fatalf("sent = %+v", inv)
	}
	h.mailer.mu.Lock()
	mails := h.mailer.documents
	h.mailer.mu.Unlock()
	if len(mails) != 1 || mails[0].To[0] != "billing@example.com" || len(mails[0].Files) != 2 || !bytes.HasPrefix(mails[0].Files[0].Data, []byte("%PDF-")) || mails[0].Files[1].ContentType != "application/xml" {
		t.Fatalf("mails = %d, files %d, first bytes %q", len(mails), len(mails[0].Files), mails[0].Files[0].Data[:min(20, len(mails[0].Files[0].Data))])
	}

	byNumber := decode[salesInvoice](t, owner.do(http.MethodGet, invoices+"/find_by_invoice_id/"+*inv.InvoiceID, nil))
	byRef := decode[salesInvoice](t, owner.do(http.MethodGet, invoices+"/find_by_reference/Order%207", nil))
	if byNumber.ID != inv.ID || byRef.ID != inv.ID {
		t.Errorf("find = %s / %s", byNumber.ID, byRef.ID)
	}

	pdf := owner.do(http.MethodGet, invoices+"/"+inv.ID+"/download_pdf", nil)
	wantStatus(t, pdf, http.StatusOK)
	if !bytes.HasPrefix(pdf.body, []byte("%PDF-")) || pdf.header.Get("Content-Type") != "application/pdf" {
		t.Errorf("pdf = %d bytes, %s", len(pdf.body), pdf.header.Get("Content-Type"))
	}
	ubl := owner.do(http.MethodGet, invoices+"/"+inv.ID+"/download_ubl", nil)
	if !strings.Contains(string(ubl.body), "<cbc:PayableAmount currencyID=\"EUR\">272.86</cbc:PayableAmount>") {
		t.Errorf("ubl = %s", ubl.body)
	}
	wantStatus(t, owner.do(http.MethodGet, invoices+"/"+inv.ID+"/download_packing_slip_pdf", nil), http.StatusOK)

	// The invoice posted debtors, revenue and VAT.
	entries := decode[[]struct {
		LedgerAccountID string `json:"ledger_account_id"`
		Amount          string `json:"amount"`
		DocumentType    string `json:"document_type"`
	}](t, owner.do(http.MethodGet, api+"/reports/journal_entries?period=this_year&contact_id="+customer.ID, nil))
	if len(entries) != 4 {
		t.Fatalf("entries = %+v", entries)
	}
	if entries[0].DocumentType != "SalesInvoice" || entries[0].Amount != "272.86" {
		t.Errorf("debtors = %+v", entries[0])
	}

	// Sent invoices cannot be deleted but can be credited.
	wantStatus(t, owner.do(http.MethodDelete, invoices+"/"+inv.ID, nil), http.StatusUnprocessableEntity)
	credit := decode[salesInvoice](t, owner.do(http.MethodPatch, invoices+"/"+inv.ID+"/duplicate_creditinvoice", map[string]any{"detail_ids": []string{inv.Details[0].ID}}))
	if credit.State != "draft" || credit.OriginalID == nil || *credit.OriginalID != inv.ID || credit.TotalIncl != "-242.0" || credit.Details[0].Amount != "-2 hours" {
		t.Errorf("credit = %+v", credit)
	}

	paused := owner.do(http.MethodPost, invoices+"/"+inv.ID+"/pause", nil)
	wantStatus(t, paused, http.StatusCreated)
	if !decode[salesInvoice](t, paused).Paused {
		t.Error("not paused")
	}
	wantStatus(t, owner.do(http.MethodPost, invoices+"/"+inv.ID+"/resume", nil), http.StatusCreated)

	open := decode[[]salesInvoice](t, owner.do(http.MethodGet, invoices+"?filter=state:open", nil))
	drafts := owner.do(http.MethodGet, invoices+"?filter=state:draft&per_page=1", nil)
	if len(open) != 1 || drafts.header.Get("X-Total-Count") != "1" {
		t.Errorf("open %d, drafts %s", len(open), drafts.header.Get("X-Total-Count"))
	}
	wantStatus(t, owner.do(http.MethodGet, invoices+"?filter=state:lost", nil), http.StatusBadRequest)

	wantStatus(t, owner.do(http.MethodPost, invoices+"/"+inv.ID+"/notes", map[string]any{"note": map[string]any{"note": "Called"}}), http.StatusCreated)
	wantStatus(t, owner.attach(invoices+"/"+inv.ID+"/attachments", "order.pdf", "application/pdf", []byte("%PDF-1.4 order")), http.StatusOK)

	// Peppol is not there yet, and the contact now has invoices.
	wantStatus(t, owner.do(http.MethodPatch, invoices+"/"+credit.ID+"/send_invoice", map[string]any{"sales_invoice_sending": map[string]any{"delivery_method": "Peppol"}}), http.StatusUnprocessableEntity)
	wantStatus(t, owner.do(http.MethodDelete, api+"/contacts/"+customer.ID, nil), http.StatusUnprocessableEntity)
	wantStatus(t, owner.do(http.MethodDelete, invoices+"/"+credit.ID, nil), http.StatusNoContent)
}

func TestSalesInvoicesFollowPermissions(t *testing.T) {
	h := newHarness(t, options{})
	owner, banker := h.client(), h.client()
	owner.register()
	banker.register()
	a := owner.createAdministration("Harpy")
	invite(t, h, owner, banker, a.Id, "banker@example.com", map[string]any{"user_type": "employee", "permissions": []string{"bank"}})
	wantStatus(t, banker.do(http.MethodGet, "/api/v2/"+a.Id+"/sales_invoices", nil), http.StatusForbidden)
	wantStatus(t, owner.do(http.MethodPost, "/api/v2/"+a.Id+"/sales_invoices", map[string]any{"sales_invoice": map[string]any{}}), http.StatusUnprocessableEntity)
}
