package httpapi_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/adirkuhn/moneyharpy/internal/httpapi"
	"github.com/adirkuhn/moneyharpy/internal/postgres/postgrestest"
)

type externalInvoice struct {
	ID                    string    `json:"id"`
	ContactID             *string   `json:"contact_id"`
	Date                  string    `json:"date"`
	DueDate               *string   `json:"due_date"`
	State                 string    `json:"state"`
	Reference             *string   `json:"reference"`
	EntryNumber           int       `json:"entry_number"`
	Origin                *string   `json:"origin"`
	SourceURL             *string   `json:"source_url"`
	TotalIncl             string    `json:"total_price_incl_tax"`
	TotalUnpaid           string    `json:"total_unpaid"`
	TotalPaid             string    `json:"total_paid"`
	MarkedDubiousOn       *string   `json:"marked_dubious_on"`
	MarkedUncollectibleOn *string   `json:"marked_uncollectible_on"`
	Payments              []payment `json:"payments"`
	URI                   string    `json:"uri"`
	Events                []struct {
		Action string `json:"action"`
	} `json:"events"`
	Contact *struct {
		ID string `json:"id"`
	} `json:"contact"`
	Details []struct {
		ID string `json:"id"`
	} `json:"details"`
	Notes []struct {
		ID string `json:"id"`
	} `json:"notes"`
	Attachments []struct {
		ID string `json:"id"`
	} `json:"attachments"`
}

// externalSetup makes an administration with a customer and returns the
// external invoices path and a function making a booked invoice of price
// excluding 21% VAT dated days ago.
func externalSetup(t *testing.T, owner *client) (string, string, func(price string, daysAgo int) externalInvoice) {
	t.Helper()
	a := owner.createAdministration("Harpy")
	api := "/api/v2/" + a.Id
	externals := api + "/external_sales_invoices"
	customer := decode[contact](t, owner.do(http.MethodPost, api+"/contacts", map[string]any{"contact": map[string]any{"company_name": "Example Shop B.V."}}))
	vat21 := salesRate(t, owner, a.Id, "21")
	create := func(price string, daysAgo int) externalInvoice {
		t.Helper()
		day := adminNow().AddDate(0, 0, -daysAgo).Format(time.DateOnly)
		r := owner.do(http.MethodPost, externals, map[string]any{"external_sales_invoice": map[string]any{
			"contact_id": customer.ID, "reference": "WEB-1001", "date": day, "due_date": day, "source": "Example webshop",
			"source_url":         "https://shop.example.com/orders/1001",
			"details_attributes": []map[string]any{{"description": "Sofa", "price": price, "tax_rate_id": vat21}},
		}})
		wantStatus(t, r, http.StatusCreated)
		return decode[externalInvoice](t, r)
	}
	return api, externals, create
}

func TestExternalSalesInvoiceLifecycle(t *testing.T) {
	h := newHarness(t, options{})
	owner := h.client()
	owner.register()
	api, externals, create := externalSetup(t, owner)

	e := create("100", 2)
	if e.State != "late" || e.TotalIncl != "121.0" || e.EntryNumber != 1 || deref(e.Reference) != "WEB-1001" || e.Contact == nil || deref(e.Origin) != "api" {
		t.Fatalf("created = %+v", e)
	}
	if e.URI != strings.TrimPrefix(externals, "/api/v2")+"/"+e.ID {
		t.Errorf("uri = %s", e.URI)
	}
	wantStatus(t, owner.do(http.MethodPost, externals, map[string]any{"external_sales_invoice": map[string]any{"source_url": "javascript:alert(1)"}}), http.StatusUnprocessableEntity)
	wantStatus(t, owner.do(http.MethodPost, externals, map[string]any{"external_sales_invoice": map[string]any{"colour": "red"}}), http.StatusBadRequest)

	// Uploading a file makes a new invoice to fill in.
	r := owner.attach(externals+"/attachment", "order.pdf", "application/pdf", []byte("%PDF-1.4 order 1002"))
	wantStatus(t, r, http.StatusOK)
	fresh := decode[externalInvoice](t, r)
	if fresh.State != "new" || fresh.ContactID != nil || len(fresh.Attachments) != 1 || deref(fresh.Origin) != "upload" || fresh.EntryNumber != 2 {
		t.Fatalf("from file = %+v", fresh)
	}
	download := owner.follow(owner.do(http.MethodGet, externals+"/"+fresh.ID+"/attachments/"+fresh.Attachments[0].ID+"/download", nil))
	if !bytes.Equal(download.body, []byte("%PDF-1.4 order 1002")) {
		t.Errorf("download = %q", download.body)
	}
	wantStatus(t, owner.attach(externals+"/attachment", "tool.exe", "application/octet-stream", []byte{0x4d, 0x5a, 0x90, 0x00}), http.StatusUnprocessableEntity)
	r = owner.do(http.MethodPatch, externals+"/"+fresh.ID, map[string]any{"external_sales_invoice": map[string]any{
		"contact_id": deref(e.ContactID), "date": adminNow().Format(time.DateOnly), "prices_are_incl_tax": true,
		"details_attributes": map[string]any{"0": map[string]any{"description": "Chair", "price": "60.50", "tax_rate_id": salesRate(t, owner, strings.TrimPrefix(api, "/api/v2/"), "21")}},
	}})
	wantStatus(t, r, http.StatusOK)
	if filled := decode[externalInvoice](t, r); filled.State != "open" || filled.TotalIncl != "60.5" || len(filled.Details) != 1 {
		t.Errorf("filled = %+v", filled)
	}

	r = owner.attach(externals+"/"+fresh.ID+"/attachment", "receipt.png", "image/png", []byte("\x89PNG\r\n\x1a\nimage"))
	wantStatus(t, r, http.StatusOK)
	att := decode[struct {
		ID string `json:"id"`
	}](t, r)
	wantStatus(t, owner.do(http.MethodDelete, externals+"/"+fresh.ID+"/attachments/"+att.ID, nil), http.StatusNoContent)
	r = owner.do(http.MethodPost, externals+"/"+fresh.ID+"/notes", map[string]any{"note": map[string]any{"note": "Paid at the counter"}})
	wantStatus(t, r, http.StatusCreated)
	note := decode[struct {
		ID string `json:"id"`
	}](t, r)
	if got := decode[externalInvoice](t, owner.do(http.MethodGet, externals+"/"+fresh.ID, nil)); len(got.Notes) != 1 || len(got.Attachments) != 1 {
		t.Errorf("notes and attachments = %+v", got)
	}
	wantStatus(t, owner.do(http.MethodDelete, externals+"/"+fresh.ID+"/notes/"+note.ID, nil), http.StatusNoContent)

	r = owner.do(http.MethodGet, externals+"?filter=state:open|late&per_page=1", nil)
	wantStatus(t, r, http.StatusOK)
	if list := decode[[]externalInvoice](t, r); len(list) != 1 || r.header.Get("X-Total-Count") != "2" || !strings.Contains(r.header.Get("Link"), `rel="next"`) {
		t.Errorf("list = %d, total %s", len(list), r.header.Get("X-Total-Count"))
	}
	wantStatus(t, owner.do(http.MethodGet, externals+"?filter=state:draft", nil), http.StatusBadRequest)
	versions := decode[[]httpapi.SynchronizationItem](t, owner.do(http.MethodGet, externals+"/synchronization?filter=state:new", nil))
	if len(versions) != 0 {
		t.Errorf("new versions = %+v", versions)
	}
	r = owner.do(http.MethodPost, externals+"/synchronization", map[string]any{"ids": []string{e.ID, fresh.ID}})
	wantStatus(t, r, http.StatusOK)
	if fetched := decode[[]externalInvoice](t, r); len(fetched) != 2 {
		t.Errorf("fetched = %d", len(fetched))
	}
	wantStatus(t, owner.do(http.MethodGet, api+"/sales_invoices/"+e.ID, nil), http.StatusNotFound)

	// Payments, write-offs and the overview.
	today := adminNow().Format(time.DateOnly)
	r = owner.do(http.MethodPost, externals+"/"+e.ID+"/payments", map[string]any{"payment": map[string]any{"payment_date": today, "price": "21", "manual_payment_action": "cash_payment"}})
	wantStatus(t, r, http.StatusCreated)
	part := decode[payment](t, r)
	if part.InvoiceType != "ExternalSalesInvoice" || part.InvoiceID != e.ID {
		t.Errorf("payment = %+v", part)
	}
	// As at Moneybird an invoice with payments still changes, but keeps its currency.
	r = owner.do(http.MethodPatch, externals+"/"+e.ID, map[string]any{"external_sales_invoice": map[string]any{"reference": "WEB-1"}})
	wantStatus(t, r, http.StatusOK)
	if changed := decode[externalInvoice](t, r); deref(changed.Reference) != "WEB-1" || len(changed.Payments) != 1 || len(changed.Events) == 0 || changed.Events[0].Action != "external_sales_invoice_created" {
		t.Errorf("changed with payments = %+v", changed)
	}
	wantStatus(t, owner.do(http.MethodPatch, externals+"/"+e.ID, map[string]any{"external_sales_invoice": map[string]any{"currency": "USD"}}), http.StatusUnprocessableEntity)
	ov := decode[httpapi.Overview](t, owner.do(http.MethodGet, api+"/overview", nil))
	if ov.Sales == nil || ov.Sales.ToReceive.Count != 2 || ov.Sales.ToReceive.Amount != "160.5" {
		t.Errorf("overview sales = %+v", ov.Sales)
	}
	r = owner.do(http.MethodPatch, externals+"/"+e.ID+"/mark_as_dubious", map[string]any{"dubious_date": adminNow().AddDate(0, 0, -1).Format(time.DateOnly)})
	wantStatus(t, r, http.StatusOK)
	if d := decode[externalInvoice](t, r); d.MarkedDubiousOn == nil {
		t.Errorf("dubious = %+v", d)
	}
	r = owner.do(http.MethodPatch, externals+"/"+e.ID+"/mark_as_uncollectible", nil)
	wantStatus(t, r, http.StatusOK)
	if u := decode[externalInvoice](t, r); u.State != "uncollectible" || u.MarkedUncollectibleOn == nil {
		t.Errorf("uncollectible = %+v", u)
	}
	wantStatus(t, owner.do(http.MethodDelete, externals+"/"+e.ID+"/payments/"+part.ID, nil), http.StatusUnprocessableEntity)
	wantKept(t, owner.do(http.MethodDelete, api+"/contacts/"+deref(e.ContactID), nil), "archived")

	wantStatus(t, owner.do(http.MethodDelete, externals+"/"+fresh.ID, nil), http.StatusNoContent)
	wantStatus(t, owner.do(http.MethodGet, externals+"/"+fresh.ID, nil), http.StatusNotFound)
}

func TestBankTransactionsPayExternalInvoices(t *testing.T) {
	h := newHarness(t, options{})
	owner := h.client()
	owner.register()
	api, externals, create := externalSetup(t, owner)
	acc := decode[financialAccount](t, owner.do(http.MethodPost, api+"/financial_accounts", map[string]any{"financial_account": map[string]any{"name": "Main bank", "identifier": "NL91ABNA0417164300"}}))
	data, err := os.ReadFile("../bankfile/testdata/camt053.xml")
	if err != nil {
		t.Fatal(err)
	}
	wantStatus(t, owner.upload(api+"/financial_accounts/"+acc.ID+"/import", "application/octet-stream", data), http.StatusCreated)
	list := decode[[]mutation](t, owner.do(http.MethodGet, api+"/financial_mutations", nil))
	i := slices.IndexFunc(list, func(m mutation) bool { return m.Amount == "1000.0" })
	if i < 0 {
		t.Fatalf("no incoming transaction in %+v", list)
	}
	incoming := list[i]
	e := create("500", 20)
	r := owner.do(http.MethodPatch, api+"/financial_mutations/"+incoming.ID+"/link_booking", map[string]any{"booking_type": "ExternalSalesInvoice", "booking_id": e.ID})
	wantStatus(t, r, http.StatusOK)
	var linked struct {
		mutation
		Payments []payment `json:"payments"`
	}
	if err := json.Unmarshal(r.body, &linked); err != nil {
		t.Fatal(err)
	}
	if len(linked.Payments) != 1 || linked.Payments[0].InvoiceType != "ExternalSalesInvoice" || linked.Payments[0].Price != "605.0" || linked.AmountOpen != "395.0" {
		t.Fatalf("linked = %+v", linked)
	}
	if got := decode[externalInvoice](t, owner.do(http.MethodGet, externals+"/"+e.ID, nil)); got.State != "paid" || got.TotalPaid != "605.0" {
		t.Errorf("invoice = %+v", got)
	}
	wantStatus(t, owner.do(http.MethodDelete, externals+"/"+e.ID+"/payments/"+linked.Payments[0].ID, nil), http.StatusNoContent)
	if m := decode[mutation](t, owner.do(http.MethodGet, api+"/financial_mutations/"+incoming.ID, nil)); m.AmountOpen != "1000.0" {
		t.Errorf("after unlink = %+v", m)
	}

	// The invoice's payments endpoint links a transaction for members who
	// may also work in the bank.
	seller := h.client()
	seller.register()
	invite(t, h, owner, seller, strings.TrimPrefix(api, "/api/v2/"), "seller@example.com", map[string]any{"user_type": "employee", "permissions": []string{"sales_invoices"}})
	link := map[string]any{"payment": map[string]any{"payment_date": "2026-11-01", "price": "605", "financial_mutation_id": incoming.ID}}
	wantStatus(t, seller.do(http.MethodPost, externals+"/"+e.ID+"/payments", link), http.StatusForbidden)
	wantStatus(t, seller.do(http.MethodGet, externals+"/"+e.ID, nil), http.StatusOK)
	r = owner.do(http.MethodPost, externals+"/"+e.ID+"/payments", link)
	wantStatus(t, r, http.StatusCreated)
	if p := decode[payment](t, r); deref(p.FinancialMutationID) != incoming.ID {
		t.Errorf("payment = %+v", p)
	}
	// Deleting the invoice unlinks the transaction.
	wantStatus(t, owner.do(http.MethodDelete, externals+"/"+e.ID, nil), http.StatusNoContent)
	if m := decode[mutation](t, owner.do(http.MethodGet, api+"/financial_mutations/"+incoming.ID, nil)); m.AmountOpen != "1000.0" || m.State != "unprocessed" {
		t.Errorf("after delete = %+v", m)
	}
}

func TestExternalSalesInvoicesFollowPermissions(t *testing.T) {
	h := newHarness(t, options{})
	owner, banker := h.client(), h.client()
	owner.register()
	banker.register()
	a := owner.createAdministration("Harpy")
	invite(t, h, owner, banker, a.Id, "banker@example.com", map[string]any{"user_type": "employee", "permissions": []string{"bank"}})
	wantStatus(t, banker.do(http.MethodGet, "/api/v2/"+a.Id+"/external_sales_invoices", nil), http.StatusForbidden)
	wantStatus(t, owner.do(http.MethodGet, "/api/v2/"+a.Id+"/external_sales_invoices", nil), http.StatusOK)
}

func TestExternalSalesInvoiceWebhooks(t *testing.T) {
	ctx := context.Background()
	pool, cleanup, err := postgrestest.Start(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()
	h := newHarness(t, options{pool: pool})
	owner := h.client()
	owner.register()
	api, externals, create := externalSetup(t, owner)
	rc := newReceiver(t)
	wantStatus(t, owner.do(http.MethodPatch, "/api/instance/settings", map[string]bool{"webhooks_allow_local": true}), http.StatusOK)
	wantStatus(t, owner.do(http.MethodPost, api+"/webhooks", map[string]any{"url": rc.srv.URL, "enabled_events": []string{"external_sales_invoice", "payment_registered"}}), http.StatusCreated)

	e := create("100", 0)
	wantStatus(t, owner.do(http.MethodPatch, externals+"/"+e.ID, map[string]any{"external_sales_invoice": map[string]any{"reference": "WEB-1002"}}), http.StatusOK)
	wantStatus(t, owner.do(http.MethodPost, externals+"/"+e.ID+"/payments", map[string]any{"payment": map[string]any{"payment_date": adminNow().Format(time.DateOnly), "price": "121"}}), http.StatusCreated)
	other := create("10", 0)
	wantStatus(t, owner.do(http.MethodPatch, externals+"/"+other.ID+"/mark_as_dubious", nil), http.StatusUnprocessableEntity) // Not after the invoice date.
	wantStatus(t, owner.do(http.MethodDelete, externals+"/"+other.ID, nil), http.StatusNoContent)
	if _, err := h.hooks.Deliver(ctx, 20); err != nil {
		t.Fatal(err)
	}
	rc.mu.Lock()
	defer rc.mu.Unlock()
	var actions []string
	for _, b := range rc.body[1:] {
		var p map[string]any
		_ = json.Unmarshal(b, &p)
		actions = append(actions, p["action"].(string))
	}
	slices.Sort(actions)
	want := []string{"external_sales_invoice_created", "external_sales_invoice_created", "external_sales_invoice_destroyed",
		"external_sales_invoice_state_changed_to_paid", "external_sales_invoice_updated", "payment_registered"}
	if !slices.Equal(actions, want) {
		t.Errorf("actions = %v, want %v", actions, want)
	}
}
