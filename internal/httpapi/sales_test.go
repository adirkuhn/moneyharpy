package httpapi_test

import (
	"bytes"
	"net/http"
	"strings"
	"testing"

	"github.com/adirkuhn/moneyharpy/internal/httpapi"
	"github.com/adirkuhn/moneyharpy/internal/sales"
)

type salesInvoice struct {
	ID             string  `json:"id"`
	InvoiceID      *string `json:"invoice_id"`
	DraftID        *int    `json:"draft_id"`
	State          string  `json:"state"`
	InvoiceDate    *string `json:"invoice_date"`
	SubscriptionID *string `json:"subscription_id"`
	DueDate        string  `json:"due_date"`
	TotalIncl      string  `json:"total_price_incl_tax"`
	TotalExcl      string  `json:"total_price_excl_tax"`
	TotalUnpaid    string  `json:"total_unpaid"`
	OriginalID     *string `json:"original_sales_invoice_id"`
	Paused         bool    `json:"paused"`
	// PaymentConditions start as the workflow's; PaymentReference and
	// ShortPaymentReference are given when the invoice is sent.
	PaymentConditions     *string `json:"payment_conditions"`
	PaymentReference      *string `json:"payment_reference"`
	ShortPaymentReference *string `json:"short_payment_reference"`
	Contact               struct {
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
	Events []struct {
		Action           string         `json:"action"`
		UserID           string         `json:"user_id"`
		AdministrationID string         `json:"administration_id"`
		LinkEntityType   *string        `json:"link_entity_type"`
		LinkEntityID     *string        `json:"link_entity_id"`
		Data             map[string]any `json:"data"`
		CreatedAt        string         `json:"created_at"`
	} `json:"events"`
	Notes []struct {
		UserID string `json:"user_id"`
	} `json:"notes"`
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

	if next := decode[httpapi.NextDraftId](t, owner.do(http.MethodGet, invoices+"/next_draft_id", nil)); next.DraftId != 1 {
		t.Errorf("next draft id = %d", next.DraftId)
	}
	created := owner.do(http.MethodPost, invoices, map[string]any{"sales_invoice": map[string]any{
		"contact_id": customer.ID, "reference": "Order 6",
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
	if inv.PaymentReference != nil || inv.ShortPaymentReference != nil || inv.PaymentConditions == nil || !strings.Contains(*inv.PaymentConditions, "{document.payment_reference}") {
		t.Errorf("draft conditions %v, references %v / %v", inv.PaymentConditions, inv.PaymentReference, inv.ShortPaymentReference)
	}
	if inv.Details[1].LedgerAccountID == "" || inv.Details[1].LedgerAccountID == revenue {
		t.Errorf("uncategorized line = %+v", inv.Details[1])
	}

	edited := owner.do(http.MethodPatch, invoices+"/"+inv.ID, map[string]any{"sales_invoice": map[string]any{"reference": "Order 7"}})
	wantStatus(t, edited, http.StatusOK)
	inv = decode[salesInvoice](t, edited)
	if len(inv.Details) != 2 || inv.TotalIncl != "272.86" {
		t.Fatalf("edited draft = %+v", inv)
	}
	re := owner.do(http.MethodPost, invoices, map[string]any{"sales_invoice": map[string]any{
		"contact_id": customer.ID, "details_attributes": []map[string]any{{"description": "Travel", "price": 25.5, "tax_rate_id": vat21}},
	}})
	wantStatus(t, re, http.StatusCreated)
	travel := decode[salesInvoice](t, re)
	versions := decode[[]httpapi.SynchronizationItem](t, owner.do(http.MethodGet, invoices+"/synchronization", nil))
	if len(versions) != 2 {
		t.Errorf("versions = %+v", versions)
	}
	if list := decode[[]salesInvoice](t, owner.do(http.MethodPost, invoices+"/synchronization", map[string]any{"ids": []string{travel.ID}})); len(list) != 1 || list[0].ID != travel.ID {
		t.Errorf("fetched = %+v", list)
	}
	wantStatus(t, owner.do(http.MethodDelete, invoices+"/"+travel.ID, nil), http.StatusNoContent)

	sent := owner.do(http.MethodPatch, invoices+"/"+inv.ID+"/send_invoice", map[string]any{"sales_invoice_sending": map[string]any{"delivery_method": "Email"}})
	wantStatus(t, sent, http.StatusOK)
	inv = decode[salesInvoice](t, sent)
	if inv.State != "open" || inv.InvoiceID == nil || !strings.HasSuffix(*inv.InvoiceID, "-0001") || inv.TotalUnpaid != "272.86" {
		t.Fatalf("sent = %+v", inv)
	}
	if inv.PaymentReference == nil || !sales.ValidRF(*inv.PaymentReference) || len(*inv.PaymentReference) != 25 || inv.ShortPaymentReference == nil ||
		*inv.ShortPaymentReference != sales.ShortPaymentReference(*inv.PaymentReference) {
		t.Errorf("sent references = %v / %v", inv.PaymentReference, inv.ShortPaymentReference)
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

	pdf := owner.follow(owner.do(http.MethodGet, invoices+"/"+inv.ID+"/download_pdf", nil))
	if !bytes.HasPrefix(pdf.body, []byte("%PDF-")) || pdf.header.Get("Content-Type") != "application/pdf" {
		t.Errorf("pdf = %d bytes, %s", len(pdf.body), pdf.header.Get("Content-Type"))
	}
	stationery := owner.follow(owner.do(http.MethodGet, invoices+"/"+inv.ID+"/download_pdf.json?media=stationery", nil))
	if !bytes.HasPrefix(stationery.body, []byte("%PDF-")) || len(stationery.body) >= len(pdf.body) {
		t.Errorf("stationery pdf = %d bytes, plain %d", len(stationery.body), len(pdf.body))
	}
	wantStatus(t, owner.do(http.MethodGet, invoices+"/1/download_pdf", nil), http.StatusNotFound)
	ubl := owner.do(http.MethodGet, invoices+"/"+inv.ID+"/download_ubl", nil)
	if !strings.Contains(string(ubl.body), "<cbc:PayableAmount currencyID=\"EUR\">272.86</cbc:PayableAmount>") {
		t.Errorf("ubl = %s", ubl.body)
	}
	owner.follow(owner.do(http.MethodGet, invoices+"/"+inv.ID+"/download_packing_slip_pdf", nil))

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
	// The invoice's own entries, as its Journal entries view shows them, whatever their date (a MoneyHarpy extension).
	own := decode[[]struct {
		DocumentID string `json:"document_id"`
		Amount     string `json:"amount"`
	}](t, owner.do(http.MethodGet, api+"/reports/journal_entries?document_type=SalesInvoice&document_id="+inv.ID, nil))
	if len(own) != 4 || own[0].DocumentID != inv.ID || own[0].Amount != "272.86" {
		t.Errorf("own entries = %+v", own)
	}
	if other := decode[[]struct{}](t, owner.do(http.MethodGet, api+"/reports/journal_entries?document_type=Payment&document_id="+inv.ID, nil)); len(other) != 0 {
		t.Errorf("payment entries of an invoice id = %+v", other)
	}
	wantStatus(t, owner.do(http.MethodGet, api+"/reports/journal_entries?document_id="+inv.ID, nil), http.StatusUnprocessableEntity)
	wantStatus(t, owner.do(http.MethodGet, api+"/reports/journal_entries?document_type=Invoice&document_id="+inv.ID, nil), http.StatusBadRequest)

	// What happened is told as Moneybird's events, by whom.
	var told []string
	for _, e := range inv.Events {
		told = append(told, e.Action)
		if e.UserID == "" || e.AdministrationID != a.Id || e.CreatedAt == "" || e.Data == nil {
			t.Errorf("event = %+v", e)
		}
	}
	if strings.Join(told, ",") != "sales_invoice_created,sales_invoice_state_changed_to_open,sales_invoice_send_email" {
		t.Errorf("events = %v", told)
	}
	if inv.Events[2].Data["email_address"] != "billing@example.com" {
		t.Errorf("email event data = %v", inv.Events[2].Data)
	}
	edited = owner.do(http.MethodPatch, invoices+"/"+inv.ID, map[string]any{"sales_invoice": map[string]any{"reference": "Order 8"}})
	wantStatus(t, edited, http.StatusOK)
	if last := decode[salesInvoice](t, edited).Events; last[len(last)-1].Action != "sales_invoice_updated" {
		t.Errorf("edit of a sent invoice = %+v", last)
	}

	// Crediting is the regular way, but a sent invoice can be deleted too (below).
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

	r := owner.do(http.MethodPost, invoices+"/"+inv.ID+"/notes", map[string]any{"note": map[string]any{"note": "Called"}})
	wantStatus(t, r, http.StatusCreated)
	noteID := decode[map[string]any](t, r)["id"].(string)
	r = owner.attach(invoices+"/"+inv.ID+"/attachments", "order.pdf", "application/pdf", []byte("%PDF-1.4 order"))
	wantStatus(t, r, http.StatusOK)
	attID := decode[map[string]any](t, r)["id"].(string)
	if got := owner.follow(owner.do(http.MethodGet, invoices+"/"+inv.ID+"/attachments/"+attID+"/download", nil)); string(got.body) != "%PDF-1.4 order" {
		t.Errorf("attachment = %q", got.body)
	}
	wantStatus(t, owner.do(http.MethodDelete, invoices+"/"+inv.ID+"/attachments/"+attID, nil), http.StatusNoContent)
	if withNote := decode[salesInvoice](t, owner.do(http.MethodGet, invoices+"/"+inv.ID, nil)); len(withNote.Notes) != 1 || withNote.Notes[0].UserID == "" {
		t.Errorf("note author = %+v", withNote.Notes)
	}
	wantStatus(t, owner.do(http.MethodDelete, invoices+"/"+inv.ID+"/notes/"+noteID, nil), http.StatusNoContent)

	// Peppol is not there yet, and the contact now has invoices.
	wantStatus(t, owner.do(http.MethodPatch, invoices+"/"+credit.ID+"/send_invoice", map[string]any{"sales_invoice_sending": map[string]any{"delivery_method": "Peppol"}}), http.StatusUnprocessableEntity)
	wantKept(t, owner.do(http.MethodDelete, api+"/contacts/"+customer.ID, nil), "archived")
	wantStatus(t, owner.do(http.MethodDelete, invoices+"/"+credit.ID, nil), http.StatusNoContent)

	// Deleting the sent invoice takes its bookings along.
	wantStatus(t, owner.do(http.MethodDelete, invoices+"/"+inv.ID, nil), http.StatusNoContent)
	wantStatus(t, owner.do(http.MethodGet, invoices+"/"+inv.ID, nil), http.StatusNotFound)
	if left := decode[[]struct{}](t, owner.do(http.MethodGet, api+"/reports/journal_entries?document_type=SalesInvoice&document_id="+inv.ID, nil)); len(left) != 0 {
		t.Errorf("entries left = %d", len(left))
	}
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

func TestDownloadLinksExpireAndCannotBeForged(t *testing.T) {
	h := newHarness(t, options{})
	owner := h.client()
	owner.register()
	stranger := h.client()
	stranger.register()
	a := owner.createAdministration("Harpy")
	customer := decode[contact](t, owner.do(http.MethodPost, "/api/v2/"+a.Id+"/contacts", map[string]any{"contact": map[string]any{"company_name": "Example Shop B.V."}}))
	inv := decode[salesInvoice](t, owner.do(http.MethodPost, "/api/v2/"+a.Id+"/sales_invoices", map[string]any{"sales_invoice": map[string]any{"contact_id": customer.ID}}))
	path := "/api/v2/" + a.Id + "/sales_invoices/" + inv.ID + "/download_pdf"
	wantStatus(t, stranger.do(http.MethodGet, path, nil), http.StatusNotFound)
	r := owner.do(http.MethodGet, path, nil)
	wantStatus(t, r, http.StatusFound)
	link := strings.TrimPrefix(r.header.Get("Location"), h.server.URL)
	token := strings.TrimPrefix(link, "/api/files/")
	payload, sig, _ := strings.Cut(token, ".")
	for _, forged := range []string{payload + "." + sig[:len(sig)-2] + "AA", payload, "x.y", payload[1:] + "." + sig} {
		wantStatus(t, stranger.do(http.MethodGet, "/api/files/"+forged, nil), http.StatusNotFound)
	}
	wantStatus(t, stranger.do(http.MethodGet, link, nil), http.StatusOK)
}
