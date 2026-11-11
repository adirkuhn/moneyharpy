package httpapi_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/adirkuhn/moneyharpy/internal/httpapi"
	"github.com/adirkuhn/moneyharpy/internal/postgres/postgrestest"
)

type purchaseDocument struct {
	ID          string    `json:"id"`
	ContactID   *string   `json:"contact_id"`
	State       string    `json:"state"`
	Reference   *string   `json:"reference"`
	EntryNumber int       `json:"entry_number"`
	Origin      *string   `json:"origin"`
	TotalIncl   string    `json:"total_price_incl_tax"`
	TotalUnpaid string    `json:"total_unpaid"`
	PaidAt      *string   `json:"paid_at"`
	Payments    []payment `json:"payments"`
	URI         string    `json:"uri"`
	Details     []struct {
		ID              string `json:"id"`
		LedgerAccountID string `json:"ledger_account_id"`
	} `json:"details"`
	FiscalAllocations []struct {
		Price string `json:"price"`
	} `json:"fiscal_allocations"`
	Notes []struct {
		ID string `json:"id"`
	} `json:"notes"`
	Attachments []struct {
		ID             string `json:"id"`
		AttachableType string `json:"attachable_type"`
	} `json:"attachments"`
}

func purchaseRate(t *testing.T, c *client, admin, percentage string) string {
	t.Helper()
	for _, r := range decode[[]salesTaxRate](t, c.do(http.MethodGet, "/api/v2/"+admin+"/tax_rates", nil)) {
		if r.Type == "purchase_invoice" && strings.TrimSuffix(r.Percentage, ".0") == percentage {
			return r.ID
		}
	}
	t.Fatalf("no purchase rate of %s%%", percentage)
	return ""
}

// purchaseSetup makes an administration with a supplier and returns its API
// path, the supplier and the 21% purchase rate.
func purchaseSetup(t *testing.T, owner *client) (string, string, string, string) {
	t.Helper()
	a := owner.createAdministration("Harpy")
	api := "/api/v2/" + a.Id
	supplier := decode[contact](t, owner.do(http.MethodPost, api+"/contacts", map[string]any{"contact": map[string]any{
		"company_name": "Supplier B.V.", "sepa_iban": "NL44RABO0123456789", "sepa_iban_account_name": "Supplier B.V.",
	}}))
	return a.Id, api, supplier.ID, purchaseRate(t, owner, a.Id, "21")
}

func TestPurchaseInvoiceLifecycle(t *testing.T) {
	h := newHarness(t, options{})
	owner := h.client()
	owner.register()
	adminID, api, supplier, vat21 := purchaseSetup(t, owner)
	invoices := api + "/documents/purchase_invoices"
	day := adminNow().AddDate(0, 0, -20).Format(time.DateOnly)
	phone := category(t, owner, adminID, "Uncategorized expenses")

	r := owner.do(http.MethodPost, invoices+".json", map[string]any{"purchase_invoice": map[string]any{
		"contact_id": supplier, "reference": "2026-01234", "date": day, "due_date": day, "prices_are_incl_tax": "false",
		"details_attributes":            map[string]any{"0": map[string]any{"description": "Delivery", "price": "300", "amount": 1, "tax_rate_id": vat21}},
		"fiscal_allocations_attributes": []map[string]any{{"ledger_account_id": phone, "private_ledger_account_id": category(t, owner, adminID, "Private withdrawals"), "percentage": "25"}},
	}})
	wantStatus(t, r, http.StatusCreated)
	inv := decode[purchaseDocument](t, r)
	if inv.State != "late" || inv.TotalIncl != "363.0" || inv.EntryNumber != 1 || deref(inv.Origin) != "api" || len(inv.Details) != 1 ||
		inv.Details[0].LedgerAccountID != phone || len(inv.FiscalAllocations) != 1 || inv.FiscalAllocations[0].Price != "90.75" {
		t.Fatalf("created = %+v", inv)
	}
	if inv.URI != "/"+adminID+"/documents/purchase_invoices/"+inv.ID {
		t.Errorf("uri = %s", inv.URI)
	}
	wantStatus(t, owner.do(http.MethodPost, invoices, map[string]any{"purchase_invoice": map[string]any{"colour": "red"}}), http.StatusBadRequest)
	wantStatus(t, owner.do(http.MethodPost, invoices, map[string]any{"purchase_invoice": map[string]any{
		"contact_id": supplier, "details_attributes": []map[string]any{{"price": "1", "tax_rate_id": salesRate(t, owner, adminID, "21")}},
	}}), http.StatusUnprocessableEntity)

	// A receipt is not a purchase invoice.
	wantStatus(t, owner.do(http.MethodGet, api+"/documents/receipts/"+inv.ID, nil), http.StatusNotFound)

	r = owner.do(http.MethodPatch, invoices+"/"+inv.ID, map[string]any{"purchase_invoice": map[string]any{
		"reference": "2026-01235", "details_attributes": []map[string]any{{"id": inv.Details[0].ID, "price": "100"}},
	}})
	wantStatus(t, r, http.StatusOK)
	if got := decode[purchaseDocument](t, r); deref(got.Reference) != "2026-01235" || got.TotalIncl != "121.0" {
		t.Errorf("updated = %+v", got)
	}

	r = owner.do(http.MethodPost, invoices+"/"+inv.ID+"/notes", map[string]any{"note": map[string]any{"note": "Check the amount"}})
	wantStatus(t, r, http.StatusCreated)
	note := decode[struct {
		ID string `json:"id"`
	}](t, r)
	r = owner.attach(invoices+"/"+inv.ID+"/attachments", "bill.pdf", "application/pdf", []byte("%PDF-1.4 bill"))
	wantStatus(t, r, http.StatusOK)
	got := decode[purchaseDocument](t, owner.do(http.MethodGet, invoices+"/"+inv.ID, nil))
	if len(got.Notes) != 1 || len(got.Attachments) != 1 || got.Attachments[0].AttachableType != "Document" {
		t.Fatalf("notes and attachments = %+v", got)
	}
	download := owner.follow(owner.do(http.MethodGet, invoices+"/"+inv.ID+"/attachments/"+got.Attachments[0].ID+"/download", nil))
	if !bytes.Equal(download.body, []byte("%PDF-1.4 bill")) {
		t.Errorf("download = %q", download.body)
	}
	wantStatus(t, owner.do(http.MethodDelete, invoices+"/"+inv.ID+"/attachments/"+got.Attachments[0].ID, nil), http.StatusNoContent)
	wantStatus(t, owner.do(http.MethodDelete, invoices+"/"+inv.ID+"/notes/"+note.ID, nil), http.StatusNoContent)

	r = owner.do(http.MethodGet, invoices+"?filter=state:late|open,contact_id:"+supplier+"&per_page=1", nil)
	wantStatus(t, r, http.StatusOK)
	if list := decode[[]purchaseDocument](t, r); len(list) != 1 || r.header.Get("X-Total-Count") != "1" {
		t.Errorf("list = %d, total %s", len(list), r.header.Get("X-Total-Count"))
	}
	if list := decode[[]purchaseDocument](t, owner.do(http.MethodGet, invoices+"?new_filter=state:paid", nil)); len(list) != 0 {
		t.Errorf("paid = %d", len(list))
	}
	wantStatus(t, owner.do(http.MethodGet, invoices+"?filter=state:draft", nil), http.StatusBadRequest)
	versions := decode[[]httpapi.SynchronizationItem](t, owner.do(http.MethodGet, invoices+"/synchronization", nil))
	if len(versions) != 1 || versions[0].Id != inv.ID {
		t.Errorf("versions = %+v", versions)
	}
	r = owner.do(http.MethodPost, invoices+"/synchronization", map[string]any{"ids": []any{inv.ID, 12345}})
	if fetched := decode[[]purchaseDocument](t, r); len(fetched) != 1 {
		t.Errorf("fetched = %d", len(fetched))
	}

	// Payments: part by hand, the rest the deprecated way.
	r = owner.do(http.MethodPost, invoices+"/"+inv.ID+"/payments", map[string]any{"payment": map[string]any{"payment_date": day, "price": 21}})
	wantStatus(t, r, http.StatusCreated)
	part := decode[payment](t, r)
	if part.InvoiceType != "Document" || part.InvoiceID != inv.ID || part.Price != "21.0" {
		t.Errorf("payment = %+v", part)
	}
	wantStatus(t, owner.do(http.MethodGet, api+"/payments/"+part.ID, nil), http.StatusOK)
	r = owner.do(http.MethodPatch, invoices+"/"+inv.ID+"/register_payment", map[string]any{"payment": map[string]any{"payment_date": day, "price": "100", "manual_payment_action": "private_payment"}})
	wantStatus(t, r, http.StatusOK)
	got = decode[purchaseDocument](t, owner.do(http.MethodGet, invoices+"/"+inv.ID, nil))
	if got.State != "paid" || got.TotalUnpaid != "0.0" || deref(got.PaidAt) != day || len(got.Payments) != 2 {
		t.Errorf("paid = %+v", got)
	}
	wantStatus(t, owner.do(http.MethodPatch, invoices+"/"+inv.ID, map[string]any{"purchase_invoice": map[string]any{"reference": "x"}}), http.StatusUnprocessableEntity)
	wantStatus(t, owner.do(http.MethodDelete, invoices+"/"+inv.ID+"/payments/"+part.ID, nil), http.StatusNoContent)
	if got := decode[purchaseDocument](t, owner.do(http.MethodGet, invoices+"/"+inv.ID, nil)); got.State != "late" || got.TotalUnpaid != "21.0" {
		t.Errorf("after payment removed = %+v", got)
	}

	// An invoices settlement pays it with a credit purchase invoice.
	credit := decode[purchaseDocument](t, owner.do(http.MethodPost, invoices, map[string]any{"purchase_invoice": map[string]any{
		"contact_id": supplier, "reference": "CR-1", "date": day, "details_attributes": []map[string]any{{"price": "-10", "tax_rate_id": vat21}},
	}}))
	r = owner.do(http.MethodPost, invoices+"/"+inv.ID+"/payments", map[string]any{"payment": map[string]any{
		"payment_date": day, "price": "12.10", "manual_payment_action": "invoices_settlement", "invoice_id": credit.ID,
	}})
	wantStatus(t, r, http.StatusCreated)
	if s := decode[payment](t, r); deref(s.ManualPaymentAction) != "invoices_settlement" || deref(s.CreditInvoiceID) != credit.ID {
		t.Errorf("settlement = %+v", s)
	}
	if got := decode[purchaseDocument](t, owner.do(http.MethodGet, invoices+"/"+credit.ID, nil)); got.State != "paid" {
		t.Errorf("settled credit = %+v", got)
	}
	wantStatus(t, owner.do(http.MethodPost, invoices+"/"+inv.ID+"/payments", map[string]any{"payment": map[string]any{
		"payment_date": day, "price": "1", "manual_payment_action": "invoices_settlement",
	}}), http.StatusUnprocessableEntity)

	r = owner.do(http.MethodDelete, invoices+"/"+inv.ID, nil)
	wantStatus(t, r, http.StatusOK)
	if strings.TrimSpace(string(r.body)) != "200" {
		t.Errorf("delete body = %s", r.body)
	}
	wantStatus(t, owner.do(http.MethodGet, invoices+"/"+inv.ID, nil), http.StatusNotFound)
}

func TestReceiptsAndBankLinks(t *testing.T) {
	h := newHarness(t, options{})
	owner := h.client()
	owner.register()
	adminID, api, supplier, vat21 := purchaseSetup(t, owner)
	receipts := api + "/documents/receipts"
	accounts := decode[[]financialAccount](t, owner.do(http.MethodGet, api+"/financial_accounts", nil))
	cash := accounts[0].ID

	r := owner.do(http.MethodPost, receipts, map[string]any{
		"receipt": map[string]any{"date": "2026-11-01", "prices_are_incl_tax": true, "details_attributes": []map[string]any{{"description": "Lunch", "price": "12.10", "tax_rate_id": vat21}}},
		"payment": map[string]any{"financial_account_id": cash},
	})
	wantStatus(t, r, http.StatusCreated)
	paid := decode[purchaseDocument](t, r)
	if paid.State != "paid" || paid.ContactID != nil || len(paid.Payments) != 1 || deref(paid.PaidAt) != "2026-11-01" {
		t.Fatalf("receipt paid at once = %+v", paid)
	}

	// A bank transaction pays a receipt.
	acc := decode[financialAccount](t, owner.do(http.MethodPost, api+"/financial_accounts", map[string]any{"financial_account": map[string]any{"name": "Main bank", "identifier": "NL91ABNA0417164300"}}))
	data, err := os.ReadFile("../bankfile/testdata/camt053.xml")
	if err != nil {
		t.Fatal(err)
	}
	wantStatus(t, owner.upload(api+"/financial_accounts/"+acc.ID+"/import", "application/octet-stream", data), http.StatusCreated)
	list := decode[[]mutation](t, owner.do(http.MethodGet, api+"/financial_mutations", nil))
	i := slices.IndexFunc(list, func(m mutation) bool { return strings.HasPrefix(m.Amount, "-") })
	outgoing := list[i]
	r = owner.do(http.MethodPost, receipts, map[string]any{"receipt": map[string]any{
		"contact_id": supplier, "reference": "R-1", "date": "2026-10-30", "prices_are_incl_tax": true,
		"details_attributes": []map[string]any{{"description": "Paper", "price": "25.50", "tax_rate_id": vat21}},
	}})
	wantStatus(t, r, http.StatusCreated)
	rec := decode[purchaseDocument](t, r)
	r = owner.do(http.MethodPatch, api+"/financial_mutations/"+outgoing.ID+"/link_booking", map[string]any{"booking_type": "Document", "booking_id": rec.ID})
	wantStatus(t, r, http.StatusOK)
	if m := decode[mutation](t, r); m.State != "processed" || m.AmountOpen != "0.0" {
		t.Errorf("linked transaction = %+v", m)
	}
	got := decode[purchaseDocument](t, owner.do(http.MethodGet, receipts+"/"+rec.ID, nil))
	if got.State != "paid" || len(got.Payments) != 1 || deref(got.Payments[0].FinancialMutationID) != outgoing.ID {
		t.Fatalf("paid by the bank = %+v", got)
	}
	r = owner.do(http.MethodDelete, api+"/financial_mutations/"+outgoing.ID+"/unlink_booking", map[string]any{"booking_type": "Payment", "booking_id": got.Payments[0].ID})
	wantStatus(t, r, http.StatusOK)
	if got := decode[purchaseDocument](t, owner.do(http.MethodGet, receipts+"/"+rec.ID, nil)); got.State != "open" {
		t.Errorf("after unlink = %s", got.State)
	}

	// The overview shows what is to pay.
	home := decode[httpapi.Overview](t, owner.do(http.MethodGet, api+"/overview", nil))
	if home.Incoming == nil || home.Incoming.ToPay.Count != 1 || home.Incoming.ToPay.Amount != "25.5" || home.Incoming.ToProcess != 0 {
		t.Errorf("overview incoming = %+v", home.Incoming)
	}
	_ = adminID
}

func TestGeneralAndTypelessDocuments(t *testing.T) {
	h := newHarness(t, options{})
	owner := h.client()
	owner.register()
	_, api, supplier, _ := purchaseSetup(t, owner)
	general := api + "/documents/general_documents"
	r := owner.do(http.MethodPost, general, map[string]any{"general_document": map[string]any{
		"reference": "Lease", "date": "2026-11-01", "contact_id": supplier, "reminder": map[string]any{"date": "2026-12-01", "message": "Renew"},
	}})
	wantStatus(t, r, http.StatusCreated)
	g := decode[map[string]any](t, r)
	if g["state"] != "saved" || g["reminder_date"] != "2026-12-01" || g["contact_id"] != supplier {
		t.Fatalf("general document = %v", g)
	}
	id := g["id"].(string)
	r = owner.do(http.MethodPatch, general+"/"+id, map[string]any{"general_document": map[string]any{"due_date": "2026-12-31"}, "remove_contact": true})
	wantStatus(t, r, http.StatusOK)
	if g := decode[map[string]any](t, r); g["contact_id"] != nil || g["due_date"] != "2026-12-31" {
		t.Errorf("updated = %v", g)
	}
	wantStatus(t, owner.do(http.MethodPost, general+"/"+id+"/notes", map[string]any{"note": map[string]any{"note": "Signed"}}), http.StatusCreated)
	wantStatus(t, owner.attach(general+"/"+id+"/attachments", "lease.pdf", "application/pdf", []byte("%PDF")), http.StatusOK)
	if list := decode[[]map[string]any](t, owner.do(http.MethodGet, general+"?filter=reference:Lease", nil)); len(list) != 1 {
		t.Errorf("general list = %d", len(list))
	}
	if v := decode[[]httpapi.SynchronizationItem](t, owner.do(http.MethodGet, general+"/synchronization", nil)); len(v) != 1 {
		t.Errorf("versions = %v", v)
	}
	if list := decode[[]map[string]any](t, owner.do(http.MethodPost, general+"/synchronization", map[string]any{"ids": []string{id}})); len(list) != 1 {
		t.Errorf("fetched = %d", len(list))
	}
	wantStatus(t, owner.do(http.MethodDelete, general+"/"+id, nil), http.StatusOK)

	typeless := api + "/documents/typeless_documents"
	r = owner.do(http.MethodPost, typeless, map[string]any{"typeless_document": map[string]any{"reference": "Scan 1"}})
	wantStatus(t, r, http.StatusCreated)
	tl := decode[map[string]any](t, r)
	tid := tl["id"].(string)
	if tl["state"] != "new" || tl["origin"] != "api" {
		t.Errorf("typeless = %v", tl)
	}
	r = owner.attach(typeless+"/"+tid+"/attachments", "scan.png", "image/png", []byte("\x89PNG\r\n\x1a\nimage"))
	wantStatus(t, r, http.StatusOK)
	att := decode[map[string]any](t, r)
	owner.follow(owner.do(http.MethodGet, typeless+"/"+tid+"/attachments/"+att["id"].(string)+"/download", nil))
	if list := decode[[]map[string]any](t, owner.do(http.MethodGet, typeless+"?filter=attachment:with", nil)); len(list) != 1 {
		t.Errorf("typeless with attachments = %d", len(list))
	}
	home := decode[httpapi.Overview](t, owner.do(http.MethodGet, api+"/overview", nil))
	if home.Incoming == nil || home.Incoming.ToProcess != 1 {
		t.Errorf("overview incoming = %+v", home.Incoming)
	}
	wantStatus(t, owner.do(http.MethodDelete, typeless+"/"+tid+"/attachments/"+att["id"].(string), nil), http.StatusNoContent)
	wantStatus(t, owner.do(http.MethodDelete, typeless+"/"+tid, nil), http.StatusOK)
	wantStatus(t, owner.do(http.MethodGet, typeless+"/"+tid, nil), http.StatusNotFound)
}

// sepaUpload sends a SEPA credit transfer file with form fields.
func (c *client) sepaUpload(path string, file []byte, fields map[string]string) response {
	c.t.Helper()
	var body bytes.Buffer
	form := multipart.NewWriter(&body)
	if file != nil {
		part, _ := form.CreateFormFile("file", "transfers.xml")
		_, _ = part.Write(file)
	}
	for k, v := range fields {
		_ = form.WriteField(k, v)
	}
	_ = form.Close()
	req, err := http.NewRequest(http.MethodPost, c.base+path, &body)
	if err != nil {
		c.t.Fatal(err)
	}
	req.Header.Set("Content-Type", form.FormDataContentType())
	res, err := c.http.Do(req)
	if err != nil {
		c.t.Fatal(err)
	}
	raw, _ := io.ReadAll(res.Body)
	_ = res.Body.Close()
	c.checkContract(req, res, raw)
	return response{status: res.StatusCode, header: res.Header, body: raw}
}

func TestPurchaseTransactionsAndSEPA(t *testing.T) {
	h := newHarness(t, options{})
	owner := h.client()
	owner.register()
	_, api, supplier, vat21 := purchaseSetup(t, owner)
	acc := decode[financialAccount](t, owner.do(http.MethodPost, api+"/financial_accounts", map[string]any{"financial_account": map[string]any{"name": "Main bank", "identifier": "NL91ABNA0417164300"}}))
	inv := decode[purchaseDocument](t, owner.do(http.MethodPost, api+"/documents/purchase_invoices", map[string]any{"purchase_invoice": map[string]any{
		"contact_id": supplier, "reference": "INV-9", "details_attributes": []map[string]any{{"description": "Paper", "price": "100", "tax_rate_id": vat21}},
	}}))
	r := owner.do(http.MethodPost, api+"/purchase_transactions", map[string]any{"purchase_transaction": map[string]any{"payable_id": inv.ID, "financial_account_id": acc.ID}})
	wantStatus(t, r, http.StatusCreated)
	tx := decode[map[string]any](t, r)
	if tx["state"] != "open" || tx["amount"] != "121.0" || tx["sepa_iban"] != "NL44RABO0123456789" || tx["payable_type"] != "Document" {
		t.Fatalf("transaction = %v", tx)
	}
	r = owner.do(http.MethodGet, api+"/purchase_transactions", nil)
	if list := decode[[]map[string]any](t, r); len(list) != 1 || r.header.Get("X-Total-Count") != "1" {
		t.Errorf("unbatched = %v", list)
	}
	wantStatus(t, owner.do(http.MethodGet, api+"/purchase_transactions?filter=state:nope", nil), http.StatusBadRequest)
	r = owner.do(http.MethodPost, api+"/purchase_transaction_batches", map[string]any{"purchase_transaction_batch": map[string]any{"financial_account_id": acc.ID}})
	wantStatus(t, r, http.StatusCreated)
	batch := decode[map[string]any](t, r)
	bid := batch["id"].(string)
	wantStatus(t, owner.do(http.MethodGet, api+"/purchase_transaction_batches/"+bid, nil), http.StatusOK)
	if got := decode[purchaseDocument](t, owner.do(http.MethodGet, api+"/documents/purchase_invoices/"+inv.ID, nil)); got.State != "pending_payment" {
		t.Errorf("document state = %s", got.State)
	}
	wantStatus(t, owner.do(http.MethodDelete, api+"/purchase_transactions/"+tx["id"].(string), nil), http.StatusUnprocessableEntity)
	r = owner.do(http.MethodPost, api+"/purchase_transaction_batches/"+bid+"/download", nil)
	wantStatus(t, r, http.StatusOK)
	if !strings.Contains(string(r.body), "pain.001.001.09") || r.header.Get("Content-Type") != "text/xml" {
		t.Errorf("download = %s", r.body)
	}
	if b := decode[map[string]any](t, owner.do(http.MethodGet, api+"/purchase_transaction_batches/"+bid, nil)); b["state"] != "exported" {
		t.Errorf("batch = %v", b)
	}
	if list := decode[[]map[string]any](t, owner.do(http.MethodGet, api+"/purchase_transactions?filter=purchase_transaction_batch_id:"+bid, nil)); len(list) != 1 || list[0]["state"] != "pending_payment" {
		t.Errorf("batch transactions = %v", list)
	}

	// The exported file goes back in: it makes a batch again.
	r = owner.sepaUpload(api+"/sepa_credit_transfer", r.body, map[string]string{"financial_account_id": acc.ID})
	wantStatus(t, r, http.StatusCreated)
	if batches := decode[[]map[string]any](t, r); len(batches) != 1 || batches[0]["financial_account_id"] != acc.ID {
		t.Errorf("uploaded batches = %v", batches)
	}
	wantStatus(t, owner.sepaUpload(api+"/sepa_credit_transfer", nil, nil), http.StatusBadRequest)
	wantStatus(t, owner.sepaUpload(api+"/sepa_credit_transfer", []byte("<nope/>"), nil), http.StatusUnprocessableEntity)

	// An open transaction outside a batch can go.
	other := decode[purchaseDocument](t, owner.do(http.MethodPost, api+"/documents/purchase_invoices", map[string]any{"purchase_invoice": map[string]any{
		"contact_id": supplier, "details_attributes": []map[string]any{{"price": "5", "tax_rate_id": vat21}},
	}}))
	tx2 := decode[map[string]any](t, owner.do(http.MethodPost, api+"/purchase_transactions", map[string]any{"purchase_transaction": map[string]any{"payable_id": other.ID, "financial_account_id": acc.ID}}))
	wantStatus(t, owner.do(http.MethodDelete, api+"/purchase_transactions/"+tx2["id"].(string)+"/delete", nil), http.StatusNoContent)
	wantStatus(t, owner.do(http.MethodGet, api+"/purchase_transactions/"+tx2["id"].(string), nil), http.StatusNotFound)
}

func TestIncomingDocumentPermissionsAndWebhooks(t *testing.T) {
	ctx := context.Background()
	pool, cleanup, err := postgrestest.Start(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()
	h := newHarness(t, options{pool: pool})
	owner, banker := h.client(), h.client()
	owner.register()
	banker.register()
	adminID, api, supplier, vat21 := purchaseSetup(t, owner)
	invite(t, h, owner, banker, adminID, "banker@example.com", map[string]any{"user_type": "employee", "permissions": []string{"bank"}})
	wantStatus(t, banker.do(http.MethodGet, api+"/documents/purchase_invoices", nil), http.StatusForbidden)
	wantStatus(t, banker.do(http.MethodGet, api+"/purchase_transactions", nil), http.StatusOK)

	rc := newReceiver(t)
	wantStatus(t, owner.do(http.MethodPatch, "/api/instance/settings", map[string]bool{"webhooks_allow_local": true}), http.StatusOK)
	wantStatus(t, owner.do(http.MethodPost, api+"/webhooks", map[string]any{"url": rc.srv.URL, "enabled_events": []string{"document_saved", "document_updated", "document_destroyed", "payment"}}), http.StatusCreated)
	inv := decode[purchaseDocument](t, owner.do(http.MethodPost, api+"/documents/purchase_invoices", map[string]any{"purchase_invoice": map[string]any{
		"contact_id": supplier, "details_attributes": []map[string]any{{"price": "10", "tax_rate_id": vat21}},
	}}))
	wantStatus(t, owner.do(http.MethodPatch, api+"/documents/purchase_invoices/"+inv.ID, map[string]any{"purchase_invoice": map[string]any{"reference": "X-1"}}), http.StatusOK)
	r := owner.do(http.MethodPost, api+"/documents/purchase_invoices/"+inv.ID+"/payments", map[string]any{"payment": map[string]any{"payment_date": adminNow().Format(time.DateOnly), "price": "12.10"}})
	wantStatus(t, r, http.StatusCreated)
	p := decode[payment](t, r)
	wantStatus(t, owner.do(http.MethodDelete, api+"/documents/purchase_invoices/"+inv.ID+"/payments/"+p.ID, nil), http.StatusNoContent)
	wantStatus(t, owner.do(http.MethodDelete, api+"/documents/purchase_invoices/"+inv.ID, nil), http.StatusOK)
	if _, err := h.hooks.Deliver(ctx, 20); err != nil {
		t.Fatal(err)
	}
	rc.mu.Lock()
	defer rc.mu.Unlock()
	var actions []string
	for _, b := range rc.body[1:] {
		var payload map[string]any
		_ = json.Unmarshal(b, &payload)
		actions = append(actions, payload["action"].(string))
	}
	slices.Sort(actions)
	want := []string{"document_destroyed", "document_saved", "document_updated", "payment_destroyed", "payment_registered"}
	if !slices.Equal(actions, want) {
		t.Errorf("actions = %v, want %v", actions, want)
	}
}

func TestBankTransactionsMakeAndPayPurchaseDocuments(t *testing.T) {
	h := newHarness(t, options{})
	owner := h.client()
	owner.register()
	adminID, api, supplier, vat21 := purchaseSetup(t, owner)
	acc := decode[financialAccount](t, owner.do(http.MethodPost, api+"/financial_accounts", map[string]any{"financial_account": map[string]any{"name": "Main bank", "identifier": "NL91ABNA0417164300"}}))
	data, err := os.ReadFile("../bankfile/testdata/camt053.xml")
	if err != nil {
		t.Fatal(err)
	}
	wantStatus(t, owner.upload(api+"/financial_accounts/"+acc.ID+"/import", "application/octet-stream", data), http.StatusCreated)
	list := decode[[]mutation](t, owner.do(http.MethodGet, api+"/financial_mutations", nil))
	byAmount := func(amount string) mutation {
		i := slices.IndexFunc(list, func(m mutation) bool { return m.Amount == amount })
		if i < 0 {
			t.Fatalf("no transaction of %s in %+v", amount, list)
		}
		return list[i]
	}
	statement := decode[statement](t, owner.do(http.MethodPost, api+"/financial_statements", map[string]any{"financial_statement": map[string]any{
		"financial_account_id": acc.ID, "reference": "S2",
		"financial_mutations_attributes": []map[string]any{{"date": "2026-11-02", "amount": "-9.99", "message": "Domain"}, {"date": "2026-11-02", "amount": "-40", "message": "INV-40"}},
	}}))
	hosting, small, out := byAmount("-25.5"), statement.Mutations[0], statement.Mutations[1]
	link := func(m mutation, body map[string]any) response {
		return owner.do(http.MethodPatch, api+"/financial_mutations/"+m.ID+"/link_booking.json", body)
	}

	// A new purchase invoice from the supplier with the transaction's IBAN,
	// paid with it.
	wantStatus(t, link(hosting, map[string]any{"booking_type": "NewPurchaseInvoice", "booking_id": supplier}), http.StatusUnprocessableEntity)
	r := link(hosting, map[string]any{"booking_type": "NewPurchaseInvoice", "price_base": "25.50"})
	wantStatus(t, r, http.StatusOK)
	m := decode[mutation](t, r)
	if m.State != "processed" || len(m.Payments) != 1 || m.Payments[0].InvoiceType != "Document" {
		t.Fatalf("after a new purchase invoice = %+v", m)
	}
	inv := decode[purchaseDocument](t, owner.do(http.MethodGet, api+"/documents/purchase_invoices/"+m.Payments[0].InvoiceID, nil))
	if inv.State != "paid" || deref(inv.ContactID) != supplier || inv.TotalIncl != "25.5" {
		t.Errorf("new purchase invoice = %+v", inv)
	}

	// A category booking and a new receipt, both on a project, show in
	// expenses by project.
	falcon := decode[project](t, owner.do(http.MethodPost, api+"/projects", map[string]any{"project": map[string]any{"name": "Falcon"}}))
	software := category(t, owner, adminID, "Software")
	wantStatus(t, link(small, map[string]any{"booking_type": "LedgerAccount", "booking_id": software, "price_base": "4", "project_id": "123"}), http.StatusUnprocessableEntity)
	r = link(small, map[string]any{"booking_type": "LedgerAccount", "booking_id": software, "price_base": "4", "project_id": falcon.ID})
	wantStatus(t, r, http.StatusOK)
	if m := decode[mutation](t, r); len(m.Bookings) != 1 || deref(m.Bookings[0].ProjectID) != falcon.ID {
		t.Errorf("booking on a project = %+v", m.Bookings)
	}
	r = link(small, map[string]any{"booking_type": "NewReceipt", "project_id": falcon.ID})
	wantStatus(t, r, http.StatusOK)
	if m := decode[mutation](t, r); m.State != "processed" || len(m.Payments) != 1 {
		t.Errorf("after a new receipt = %+v", m)
	}
	byProject := decode[map[string][]reportItem](t, owner.do(http.MethodGet, api+"/reports/expenses_by_project?period=202611", nil))["expenses_by_project"]
	if i := slices.IndexFunc(byProject, func(p reportItem) bool { return deref(p.ProjectID) == falcon.ID }); i < 0 || byProject[i].Amount != "9.99" {
		t.Errorf("expenses by project = %+v", byProject)
	}

	// A purchase transaction is paid when its bank transaction is linked.
	other := decode[purchaseDocument](t, owner.do(http.MethodPost, api+"/documents/purchase_invoices", map[string]any{"purchase_invoice": map[string]any{
		"contact_id": supplier, "date": "2026-10-01", "prices_are_incl_tax": true, "details_attributes": []map[string]any{{"price": "40", "tax_rate_id": vat21}},
	}}))
	tx := decode[map[string]any](t, owner.do(http.MethodPost, api+"/purchase_transactions", map[string]any{"purchase_transaction": map[string]any{"payable_id": other.ID, "financial_account_id": acc.ID}}))
	wantStatus(t, link(out, map[string]any{"booking_type": "PurchaseTransaction", "booking_id": "1"}), http.StatusUnprocessableEntity)
	r = link(out, map[string]any{"booking_type": "PurchaseTransaction", "booking_id": tx["id"]})
	wantStatus(t, r, http.StatusOK)
	if m := decode[mutation](t, r); m.State != "processed" {
		t.Errorf("after the purchase transaction = %+v", m)
	}
	if got := decode[map[string]any](t, owner.do(http.MethodGet, api+"/purchase_transactions/"+tx["id"].(string), nil)); got["state"] != "paid" {
		t.Errorf("purchase transaction = %v", got)
	}
}
