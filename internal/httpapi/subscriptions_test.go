package httpapi_test

import (
	"context"
	"encoding/json"
	"net/http"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/adirkuhn/moneyharpy/internal/downloads"
	"github.com/adirkuhn/moneyharpy/internal/postgres/postgrestest"
	"github.com/adirkuhn/moneyharpy/internal/resource"
)

type subscription struct {
	ID            string  `json:"id"`
	StartDate     string  `json:"start_date"`
	EndDate       *string `json:"end_date"`
	Frequency     int     `json:"frequency"`
	FrequencyType string  `json:"frequency_type"`
	Reference     *string `json:"reference"`
	CancelledAt   *string `json:"cancelled_at"`
	ProductID     string  `json:"product_id"`
	Product       *struct {
		ID string `json:"id"`
	} `json:"product"`
	ContactID string `json:"contact_id"`
	Contact   *struct {
		ID string `json:"id"`
	} `json:"contact"`
	Products []struct {
		StartDate string  `json:"start_date"`
		EndDate   *string `json:"end_date"`
		ProductID string  `json:"product_id"`
		Amount    string  `json:"amount"`
		Discount  string  `json:"discount"`
	} `json:"subscription_products"`
	RecurringID string `json:"recurring_sales_invoice_id"`
	URI         string `json:"uri"`
}

type additionalCharge struct {
	ID             string  `json:"id"`
	ContactID      *string `json:"contact_id"`
	SubscriptionID *string `json:"subscription_id"`
	ProductID      string  `json:"product_id"`
	DetailID       *string `json:"detail_id"`
	Amount         string  `json:"amount"`
	Price          string  `json:"price"`
	Period         string  `json:"period"`
	Description    string  `json:"description"`
}

// subscriptionSetup makes an administration with a customer and a monthly
// product of 20 + 21% VAT, and returns the API path, the customer and the
// product.
func subscriptionSetup(t *testing.T, owner *client) (string, string, string) {
	t.Helper()
	a := owner.createAdministration("Harpy")
	api := "/api/v2/" + a.Id
	customer := decode[contact](t, owner.do(http.MethodPost, api+"/contacts", map[string]any{"contact": map[string]any{
		"company_name": "Example Shop B.V.", "send_invoices_to_email": "billing@example.com",
	}}))
	p := owner.do(http.MethodPost, api+"/products", map[string]any{"product": map[string]any{
		"description": "Hosting", "price": "20", "tax_rate_id": salesRate(t, owner, a.Id, "21"), "ledger_account_id": category(t, owner, a.Id, "Revenue"),
		"frequency": 1, "frequency_type": "month",
	}})
	wantStatus(t, p, http.StatusCreated)
	return api, customer.ID, decode[product](t, p).ID
}

func TestSubscriptionLifecycle(t *testing.T) {
	h := newHarness(t, options{})
	owner, stranger := h.client(), h.client()
	owner.register()
	stranger.register()
	api, customer, hosting := subscriptionSetup(t, owner)
	tomorrow := adminNow().AddDate(0, 0, 1).Format(time.DateOnly)

	r := owner.do(http.MethodPost, api+"/subscriptions.json", map[string]any{"subscription": map[string]any{
		"start_date": tomorrow, "contact_id": customer, "product_id": hosting, "reference": "moneybird.nl", "discount": 10, "amount": "5x",
	}})
	wantStatus(t, r, http.StatusCreated)
	sub := decode[subscription](t, r)
	if sub.StartDate != tomorrow || sub.Frequency != 1 || sub.FrequencyType != "month" || deref(sub.Reference) != "moneybird.nl" || sub.ProductID != hosting ||
		sub.Product == nil || sub.Contact == nil || sub.RecurringID == "" || len(sub.Products) != 1 || sub.Products[0].Amount != "5x" ||
		sub.Products[0].Discount != "10.0" || sub.URI != strings.TrimPrefix(api, "/api/v2")+"/subscriptions/"+sub.ID {
		t.Fatalf("created = %+v", sub)
	}
	rec := decode[recurringInvoice](t, owner.do(http.MethodGet, api+"/recurring_sales_invoices/"+sub.RecurringID, nil))
	if !rec.AutoSend || rec.InvoiceDate != tomorrow || rec.TotalIncl != "108.9" || rec.Subscription == nil || rec.Subscription.ID != sub.ID {
		t.Errorf("recurring = %+v", rec)
	}
	if got := decode[[]recurringInvoice](t, owner.do(http.MethodGet, api+"/recurring_sales_invoices", nil)); len(got) != 0 {
		t.Errorf("recurring invoices of subscriptions listed by default: %+v", got)
	}
	if got := decode[[]recurringInvoice](t, owner.do(http.MethodGet, api+"/recurring_sales_invoices?filter=include_subscriptions:true", nil)); len(got) != 1 {
		t.Errorf("with include_subscriptions = %+v", got)
	}
	wantStatus(t, owner.do(http.MethodPost, api+"/subscriptions", map[string]any{"subscription": map[string]any{"contact_id": customer}}), http.StatusBadRequest)
	wantStatus(t, owner.do(http.MethodPost, api+"/subscriptions", map[string]any{"subscription": map[string]any{
		"start_date": tomorrow, "contact_id": customer, "product_id": hosting, "frequency_type": "week",
	}}), http.StatusUnprocessableEntity)

	list := decode[[]subscription](t, owner.do(http.MethodGet, api+"/subscriptions?contact_id="+customer, nil))
	if len(list) != 1 || list[0].ID != sub.ID {
		t.Errorf("list = %+v", list)
	}
	wantStatus(t, owner.do(http.MethodGet, api+"/subscriptions", nil), http.StatusBadRequest)
	wantStatus(t, owner.do(http.MethodGet, api+"/subscriptions?contact_id=1", nil), http.StatusNotFound)
	wantStatus(t, stranger.do(http.MethodGet, api+"/subscriptions/"+sub.ID, nil), http.StatusNotFound)
	missing := owner.do(http.MethodGet, api+"/subscriptions/1", nil)
	wantStatus(t, missing, http.StatusNotFound)
	if !strings.Contains(string(missing.body), "Record not found for model name: Subscription") {
		t.Errorf("missing = %s", missing.body)
	}

	up := owner.do(http.MethodPatch, api+"/subscriptions/"+sub.ID, map[string]any{"subscription": map[string]any{"amount": "3 x", "discount": "0", "start_date": tomorrow}})
	wantStatus(t, up, http.StatusOK)
	if got := decode[subscription](t, up); len(got.Products) != 1 || got.Products[0].Amount != "3 x" || got.Products[0].Discount != "0.0" {
		t.Errorf("updated = %+v", got.Products)
	}
	if got := decode[recurringInvoice](t, owner.do(http.MethodGet, api+"/recurring_sales_invoices/"+sub.RecurringID, nil)); got.TotalIncl != "72.6" {
		t.Errorf("recurring after the change = %+v", got)
	}

	// The next invoice is made today: it takes the charge in and goes out.
	charge := owner.do(http.MethodPost, api+"/subscriptions/"+sub.ID+"/additional_charges", map[string]any{
		"product_id": hosting, "amount": "5 x", "price": 100, "period": "20210601..20210630", "description": "Transaction Costs",
	})
	wantStatus(t, charge, http.StatusCreated)
	if c := decode[additionalCharge](t, charge); c.Period != "202106..202106" || c.Price != "100.0" || deref(c.SubscriptionID) != sub.ID || c.ContactID != nil || c.DetailID != nil {
		t.Errorf("charge = %+v", c)
	}
	unknown := owner.do(http.MethodPost, api+"/subscriptions/"+sub.ID+"/additional_charges", map[string]any{"product_id": "1", "amount": 1, "period": "this_month", "description": "x"})
	wantStatus(t, unknown, http.StatusNotFound)
	if !strings.Contains(string(unknown.body), `"product_id":"not_found"`) {
		t.Errorf("unknown product = %s", unknown.body)
	}
	wantStatus(t, owner.do(http.MethodPost, api+"/subscriptions/"+sub.ID+"/additional_charges", map[string]any{"product_id": hosting}), http.StatusBadRequest)

	oneOff := owner.do(http.MethodPost, api+"/subscriptions/"+sub.ID+"/create_and_schedule_one_off_sales_invoice", map[string]any{"sales_invoice": map[string]any{
		"details_attributes": map[string]any{"0": map[string]any{"description": "Extra costs subscription", "price": "10.5", "tax_rate_id": salesRate(t, owner, strings.TrimPrefix(api, "/api/v2/"), "21")}},
	}})
	wantStatus(t, oneOff, http.StatusCreated)
	scheduled := decode[salesInvoice](t, oneOff)
	if scheduled.State != "scheduled" || deref(scheduled.InvoiceDate) != tomorrow || deref(scheduled.SubscriptionID) != sub.ID {
		t.Errorf("one-off = %+v", scheduled)
	}

	today := adminNow().Format(time.DateOnly)
	wantStatus(t, owner.do(http.MethodPatch, api+"/recurring_sales_invoices/"+sub.RecurringID, map[string]any{"recurring_sales_invoice": map[string]any{"invoice_date": today}}), http.StatusOK)
	if err := h.sales.Work(context.Background()); err != nil && strings.Contains(err.Error(), strings.TrimPrefix(api, "/api/v2/")) {
		t.Fatal(err)
	}
	made := decode[[]salesInvoice](t, owner.do(http.MethodGet, api+"/sales_invoices?filter=recurring_sales_invoice_id:"+sub.RecurringID, nil))
	if len(made) != 1 || made[0].State != "open" || len(made[0].Details) != 2 || made[0].TotalIncl != "677.6" || deref(made[0].SubscriptionID) != sub.ID {
		t.Fatalf("made = %+v", made)
	}
	charges := decode[[]additionalCharge](t, owner.do(http.MethodGet, api+"/subscriptions/"+sub.ID+"/additional_charges?include_billed=true", nil))
	if len(charges) != 1 || charges[0].DetailID == nil {
		t.Errorf("billed charges = %+v", charges)
	}
	if open := decode[[]additionalCharge](t, owner.do(http.MethodGet, api+"/subscriptions/"+sub.ID+"/additional_charges", nil)); len(open) != 0 {
		t.Errorf("open charges = %+v", open)
	}
	// The one-off falls after today's invoice, so it stays for the next.
	if still := owner.do(http.MethodGet, api+"/sales_invoices/"+scheduled.ID, nil); still.status != http.StatusOK {
		t.Errorf("one-off = %d", still.status)
	}

	end := adminNow().AddDate(0, 0, 2).Format(time.DateOnly)
	c := owner.do(http.MethodDelete, api+"/subscriptions/"+sub.ID, map[string]any{"subscription": map[string]any{"end_date": end}})
	wantStatus(t, c, http.StatusOK)
	if got := decode[subscription](t, c); got.CancelledAt == nil || deref(got.EndDate) != end {
		t.Errorf("cancelled = %+v", got)
	}
	wantStatus(t, owner.do(http.MethodDelete, api+"/subscriptions/"+sub.ID, nil), http.StatusBadRequest)

	// Products a subscription bills stay.
	wantKept(t, owner.do(http.MethodDelete, api+"/products/"+hosting, nil), "deactivated")
}

func TestContactAdditionalCharges(t *testing.T) {
	h := newHarness(t, options{})
	owner := h.client()
	owner.register()
	api, customer, hosting := subscriptionSetup(t, owner)
	r := owner.do(http.MethodPost, api+"/contacts/"+customer+"/additional_charges.json", map[string]any{
		"product_id": hosting, "amount": 1, "period": "this_month", "description": "Setup", "identifier": "setup-1",
	})
	wantStatus(t, r, http.StatusCreated)
	c := decode[additionalCharge](t, r)
	if deref(c.ContactID) != customer || c.SubscriptionID != nil || c.Amount != "1 x" || c.Price != "20.0" || c.Period != adminNow().Format("200601")+".."+adminNow().Format("200601") {
		t.Errorf("charge = %+v", c)
	}
	wantStatus(t, owner.do(http.MethodPost, api+"/contacts/"+customer+"/additional_charges", map[string]any{
		"product_id": hosting, "amount": "many", "period": "someday", "description": "Setup",
	}), http.StatusUnprocessableEntity)
	if list := decode[[]additionalCharge](t, owner.do(http.MethodGet, api+"/contacts/"+customer+"/additional_charges", nil)); len(list) != 1 {
		t.Errorf("list = %+v", list)
	}
	wantStatus(t, owner.do(http.MethodGet, api+"/contacts/1/additional_charges", nil), http.StatusNotFound)
	// Billed once the month is over: nothing yet.
	if err := h.subscriptions.Work(context.Background()); err != nil && strings.Contains(err.Error(), strings.TrimPrefix(api, "/api/v2/")) {
		t.Fatal(err)
	}
	if list := decode[[]salesInvoice](t, owner.do(http.MethodGet, api+"/sales_invoices?filter=state:all", nil)); len(list) != 0 {
		t.Errorf("billed early: %+v", list)
	}
}

func TestPaymentsOnlyEndpointsAnswerAsMoneybird(t *testing.T) {
	h := newHarness(t, options{})
	owner := h.client()
	owner.register()
	api, customer, _ := subscriptionSetup(t, owner)

	templates := owner.do(http.MethodGet, api+"/subscription_templates", nil)
	wantStatus(t, templates, http.StatusOK)
	if string(templates.body) != "[]\n" && string(templates.body) != "[]" || templates.header.Get("X-Total-Count") != "0" {
		t.Errorf("templates = %s", templates.body)
	}
	for _, r := range []response{
		owner.do(http.MethodGet, api+"/subscription_templates/123/checkout_identifier?contact_id="+customer+"&start_date=2026-12-01", nil),
		owner.do(http.MethodPost, api+"/subscription_templates/123/sales_link", nil),
	} {
		wantStatus(t, r, http.StatusNotFound)
		if !strings.Contains(string(r.body), "SubscriptionTemplate") {
			t.Errorf("template = %s", r.body)
		}
	}

	mandate := api + "/contacts/" + customer + "/moneybird_payments_mandate"
	if r := owner.do(http.MethodGet, mandate, nil); r.status != http.StatusNotFound || strings.TrimSpace(string(r.body)) != "404" {
		t.Errorf("get mandate = %d %s", r.status, r.body)
	}
	for _, r := range []response{
		owner.do(http.MethodPost, mandate, map[string]any{"mandate_request": map[string]any{"email_message": "Please"}}),
		owner.do(http.MethodPost, mandate+"/url", map[string]any{}),
		owner.do(http.MethodDelete, mandate, nil),
	} {
		if r.status != http.StatusForbidden || strings.TrimSpace(string(r.body)) != "403" {
			t.Errorf("mandate = %d %s", r.status, r.body)
		}
	}

	portal := decode[string](t, owner.do(http.MethodGet, api+"/customer_contact_portal/"+customer, nil))
	if !strings.HasSuffix(portal, strings.TrimPrefix(api, "/api/v2")+"/contacts/"+customer) {
		t.Errorf("portal = %s", portal)
	}
	invoices := decode[string](t, owner.do(http.MethodGet, api+"/customer_contact_portal/"+customer+"/invoices", nil))
	if !strings.Contains(invoices, "/sales_invoices?filter=contact_id:"+customer) {
		t.Errorf("invoices = %s", invoices)
	}
	wantStatus(t, owner.do(http.MethodGet, api+"/customer_contact_portal/1", nil), http.StatusNotFound)
	wantStatus(t, owner.do(http.MethodGet, api+"/customer_contact_portal/"+customer+"/subscriptions/1", nil), http.StatusNotFound)

	v := owner.do(http.MethodGet, api+"/verifications", nil)
	wantStatus(t, v, http.StatusOK)
	if strings.TrimSpace(string(v.body)) != "{}" {
		t.Errorf("verifications = %s", v.body)
	}
}

func TestDownloads(t *testing.T) {
	h := newHarness(t, options{})
	owner, banker := h.client(), h.client()
	owner.register()
	banker.register()
	a := owner.createAdministration("Harpy")
	api := "/api/v2/" + a.Id
	invite(t, h, owner, banker, a.Id, "banker@example.com", map[string]any{"user_type": "employee", "permissions": []string{"bank"}})
	admin, err := resource.ParseID(a.Id)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	contactsFile, err := h.downloads.Add(ctx, admin, nil, downloads.ExportSalesInvoices, "invoices.csv", "text/csv", []byte("id,total\n1,10\n"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := h.downloads.AddFailed(ctx, admin, nil, downloads.ExportDocuments, ""); err != nil {
		t.Fatal(err)
	}

	list := owner.do(http.MethodGet, api+"/downloads", nil)
	wantStatus(t, list, http.StatusOK)
	if got := decode[[]map[string]any](t, list); len(got) != 2 || list.header.Get("X-Total-Count") != "2" {
		t.Errorf("list = %s", list.body)
	}
	if got := decode[[]map[string]any](t, owner.do(http.MethodGet, api+"/downloads?failed=true", nil)); len(got) != 1 || got[0]["download_type"] != "export_documents" {
		t.Errorf("failed = %+v", got)
	}
	wantStatus(t, owner.do(http.MethodGet, api+"/downloads?download_type=nonsense", nil), http.StatusBadRequest)
	wantStatus(t, owner.do(http.MethodGet, api+"/downloads?per_page=500", nil), http.StatusBadRequest)
	if got := decode[[]map[string]any](t, banker.do(http.MethodGet, api+"/downloads", nil)); len(got) != 0 {
		t.Errorf("banker sees = %+v", got)
	}

	path := api + "/downloads/" + contactsFile.ID.String() + "/download"
	wantStatus(t, banker.do(http.MethodPost, path, nil), http.StatusUnauthorized)
	got := owner.follow(owner.do(http.MethodPost, path, nil))
	if string(got.body) != "id,total\n1,10\n" {
		t.Errorf("file = %q", got.body)
	}
	if got := decode[[]map[string]any](t, owner.do(http.MethodGet, api+"/downloads?downloaded=true", nil)); len(got) != 1 {
		t.Errorf("downloaded = %+v", got)
	}
	wantStatus(t, owner.do(http.MethodPost, api+"/downloads/1/download", nil), http.StatusNotFound)
}

func TestSubscriptionWebhooks(t *testing.T) {
	ctx := context.Background()
	pool, cleanup, err := postgrestest.Start(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()
	h := newHarness(t, options{pool: pool})
	owner := h.client()
	owner.register()
	api, customer, hosting := subscriptionSetup(t, owner)
	rc := newReceiver(t)
	wantStatus(t, owner.do(http.MethodPatch, "/api/instance/settings", map[string]bool{"webhooks_allow_local": true}), http.StatusOK)
	wantStatus(t, owner.do(http.MethodPost, api+"/webhooks", map[string]any{"url": rc.srv.URL, "enabled_events": []string{"subscription", "sales_invoice_created_based_on_subscription"}}), http.StatusCreated)

	tomorrow := adminNow().AddDate(0, 0, 1).Format(time.DateOnly)
	sub := decode[subscription](t, owner.do(http.MethodPost, api+"/subscriptions", map[string]any{"subscription": map[string]any{
		"start_date": tomorrow, "contact_id": customer, "product_id": hosting,
	}}))
	wantStatus(t, owner.do(http.MethodPatch, api+"/subscriptions/"+sub.ID, map[string]any{"subscription": map[string]any{"reference": "Contract 8"}}), http.StatusOK)
	wantStatus(t, owner.do(http.MethodPatch, api+"/subscriptions/"+sub.ID, map[string]any{"subscription": map[string]any{"amount": "2"}}), http.StatusOK)
	wantStatus(t, owner.do(http.MethodPatch, api+"/recurring_sales_invoices/"+sub.RecurringID, map[string]any{"recurring_sales_invoice": map[string]any{"invoice_date": adminNow().Format(time.DateOnly)}}), http.StatusOK)
	if err := h.sales.Work(ctx); err != nil {
		t.Fatal(err)
	}
	wantStatus(t, owner.do(http.MethodDelete, api+"/subscriptions/"+sub.ID, nil), http.StatusOK)
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
	want := []string{"sales_invoice_created_based_on_subscription", "subscription_cancelled", "subscription_created", "subscription_edited", "subscription_updated"}
	if !slices.Equal(actions, want) {
		t.Errorf("actions = %v, want %v", actions, want)
	}
}
