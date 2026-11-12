package httpapi_test

import (
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

type payment struct {
	ID                  string  `json:"id"`
	InvoiceType         string  `json:"invoice_type"`
	InvoiceID           string  `json:"invoice_id"`
	Price               string  `json:"price"`
	PaymentDate         string  `json:"payment_date"`
	FinancialMutationID *string `json:"financial_mutation_id"`
	LedgerAccountID     *string `json:"ledger_account_id"`
	ManualPaymentAction *string `json:"manual_payment_action"`
	LinkedPaymentID     *string `json:"linked_payment_id"`
	CreditInvoiceID     *string `json:"credit_invoice_id"`
}

type paidInvoice struct {
	salesInvoice
	PaidAt                *string   `json:"paid_at"`
	TotalPaid             string    `json:"total_paid"`
	Payments              []payment `json:"payments"`
	MarkedDubiousOn       *string   `json:"marked_dubious_on"`
	MarkedUncollectibleOn *string   `json:"marked_uncollectible_on"`
	ReminderCount         int       `json:"reminder_count"`
	NextReminder          *string   `json:"next_reminder"`
	ContactID             string    `json:"contact_id"`
	WorkflowID            string    `json:"workflow_id"`
	DocumentStyleID       string    `json:"document_style_id"`
	IdentityID            string    `json:"identity_id"`
}

// salesSetup makes an administration with a customer who gets invoices by
// email, and returns the invoices path and a function sending an invoice
// of 100 + 21% VAT (or the price including VAT) dated days ago.
func salesSetup(t *testing.T, owner *client) (string, string, func(price string, inclTax bool, daysAgo int) paidInvoice) {
	t.Helper()
	a := owner.createAdministration("Harpy")
	api := "/api/v2/" + a.Id
	invoices := api + "/sales_invoices"
	customer := decode[contact](t, owner.do(http.MethodPost, api+"/contacts", map[string]any{"contact": map[string]any{
		"company_name": "Example Shop B.V.", "send_invoices_to_email": "billing@example.com",
	}}))
	vat21 := salesRate(t, owner, a.Id, "21")
	revenue := category(t, owner, a.Id, "Revenue")
	send := func(price string, inclTax bool, daysAgo int) paidInvoice {
		t.Helper()
		inv := decode[salesInvoice](t, owner.do(http.MethodPost, invoices, map[string]any{"sales_invoice": map[string]any{
			"contact_id": customer.ID, "prices_are_incl_tax": inclTax,
			"details_attributes": []map[string]any{{"description": "Consulting", "amount": "1", "price": price, "tax_rate_id": vat21, "ledger_account_id": revenue}},
		}}))
		r := owner.do(http.MethodPatch, invoices+"/"+inv.ID+"/send_invoice", map[string]any{"sales_invoice_sending": map[string]any{
			"delivery_method": "Manual", "invoice_date": adminNow().AddDate(0, 0, -daysAgo).Format(time.DateOnly),
		}})
		wantStatus(t, r, http.StatusOK)
		return decode[paidInvoice](t, r)
	}
	return api, invoices, send
}

func TestSalesInvoicePayments(t *testing.T) {
	h := newHarness(t, options{})
	owner := h.client()
	owner.register()
	api, invoices, send := salesSetup(t, owner)
	today := adminNow().Format(time.DateOnly)

	inv := send("100", false, 2)
	r := owner.do(http.MethodPost, invoices+"/"+inv.ID+"/payments", map[string]any{"payment": map[string]any{"payment_date": today, "price": "21", "manual_payment_action": "cash_payment"}})
	wantStatus(t, r, http.StatusCreated)
	part := decode[payment](t, r)
	if part.Price != "21.0" || part.InvoiceType != "SalesInvoice" || part.InvoiceID != inv.ID || part.LedgerAccountID == nil || deref(part.ManualPaymentAction) != "cash_payment" {
		t.Errorf("payment = %+v", part)
	}
	wantStatus(t, owner.do(http.MethodPost, invoices+"/"+inv.ID+"/payments", map[string]any{"payment": map[string]any{"payment_date": today, "price": "500"}}), http.StatusUnprocessableEntity)
	if got := decode[payment](t, owner.do(http.MethodGet, api+"/payments/"+part.ID+".json", nil)); got.ID != part.ID || got.InvoiceID != inv.ID || got.Price != "21.0" {
		t.Errorf("GET payment = %+v", got)
	}
	wantStatus(t, owner.do(http.MethodGet, api+"/payments/1", nil), http.StatusNotFound)
	r = owner.do(http.MethodPatch, invoices+"/"+inv.ID+"/register_payment", map[string]any{"payment": map[string]any{"payment_date": today + "T10:00:00Z", "price": 100}})
	wantStatus(t, r, http.StatusOK)
	rest := decode[payment](t, r)
	got := decode[paidInvoice](t, owner.do(http.MethodGet, invoices+"/"+inv.ID, nil))
	if got.State != "paid" || deref(got.PaidAt) != today || got.TotalPaid != "121.0" || got.TotalUnpaid != "0.0" || len(got.Payments) != 2 {
		t.Fatalf("paid = %+v", got)
	}
	wantStatus(t, owner.do(http.MethodDelete, invoices+"/"+inv.ID+"/payments/"+rest.ID, nil), http.StatusNoContent)
	wantStatus(t, owner.do(http.MethodDelete, invoices+"/"+inv.ID+"/payments/"+rest.ID, nil), http.StatusNotFound)
	if got := decode[paidInvoice](t, owner.do(http.MethodGet, invoices+"/"+inv.ID, nil)); got.State != "open" || got.TotalUnpaid != "100.0" {
		t.Errorf("after delete = %s, unpaid %s", got.State, got.TotalUnpaid)
	}

	// A credit invoice settles with the invoice it credits.
	credit := decode[salesInvoice](t, owner.do(http.MethodPatch, invoices+"/"+inv.ID+"/duplicate_creditinvoice", map[string]any{}))
	wantStatus(t, owner.do(http.MethodPatch, invoices+"/"+inv.ID+"/register_payment_creditinvoice", map[string]any{}), http.StatusUnprocessableEntity)
	wantStatus(t, owner.do(http.MethodPatch, invoices+"/"+credit.ID+"/send_invoice", map[string]any{"sales_invoice_sending": map[string]any{"delivery_method": "Manual"}}), http.StatusOK)
	wantStatus(t, owner.do(http.MethodPost, invoices+"/"+inv.ID+"/payments", map[string]any{"payment": map[string]any{"payment_date": today, "price": "-21", "manual_payment_action": "invoices_settlement", "invoice_id": credit.ID}}), http.StatusUnprocessableEntity)
	r = owner.do(http.MethodPatch, invoices+"/"+credit.ID+"/register_payment_creditinvoice", nil)
	wantStatus(t, r, http.StatusUnprocessableEntity) // The credit (-121) is more than the 100 the original has unpaid.
	wantStatus(t, owner.do(http.MethodDelete, invoices+"/"+inv.ID+"/payments/"+part.ID, nil), http.StatusNoContent)
	r = owner.do(http.MethodPatch, invoices+"/"+credit.ID+"/register_payment_creditinvoice", map[string]any{})
	wantStatus(t, r, http.StatusOK)
	settled := decode[paidInvoice](t, r)
	orig := decode[paidInvoice](t, owner.do(http.MethodGet, invoices+"/"+inv.ID, nil))
	if settled.State != "paid" || orig.State != "paid" || settled.Payments[0].Price != "-121.0" || deref(orig.Payments[0].LinkedPaymentID) != settled.Payments[0].ID {
		t.Errorf("settled credit %s, original %s %+v", settled.State, orig.State, orig.Payments)
	}

	// As at Moneybird, deleting a paid invoice takes its payments along.
	paidOff := send("10", false, 0)
	r = owner.do(http.MethodPatch, invoices+"/"+paidOff.ID+"/register_payment", map[string]any{"payment": map[string]any{"payment_date": today, "price": "12.10"}})
	wantStatus(t, r, http.StatusOK)
	gone := decode[payment](t, r)
	wantStatus(t, owner.do(http.MethodDelete, invoices+"/"+paidOff.ID, nil), http.StatusNoContent)
	wantStatus(t, owner.do(http.MethodGet, api+"/payments/"+gone.ID, nil), http.StatusNotFound)

	// Dubious, then uncollectible.
	late := send("100", false, 40)
	wantStatus(t, owner.do(http.MethodPatch, invoices+"/"+late.ID+"/mark_as_dubious", map[string]any{"dubious_date": adminNow().AddDate(0, 0, -41).Format(time.DateOnly)}), http.StatusUnprocessableEntity)
	r = owner.do(http.MethodPatch, invoices+"/"+late.ID+"/mark_as_dubious", map[string]any{"dubious_date": adminNow().AddDate(0, 0, -5).Format(time.DateOnly)})
	wantStatus(t, r, http.StatusOK)
	if d := decode[paidInvoice](t, r); d.MarkedDubiousOn == nil || d.State != "late" {
		t.Errorf("dubious = %+v", d)
	}
	r = owner.do(http.MethodPatch, invoices+"/"+late.ID+"/mark_as_uncollectible", map[string]any{"book_method": "revenue"})
	wantStatus(t, r, http.StatusOK)
	if u := decode[paidInvoice](t, r); u.State != "uncollectible" || u.MarkedUncollectibleOn == nil || deref(u.MarkedUncollectibleOn) != today {
		t.Errorf("uncollectible = %+v", u)
	}
	wantStatus(t, owner.do(http.MethodPatch, invoices+"/"+late.ID+"/mark_as_uncollectible", nil), http.StatusUnprocessableEntity)
	wantStatus(t, owner.do(http.MethodPost, invoices+"/"+late.ID+"/payments", map[string]any{"payment": map[string]any{"payment_date": today, "price": "1"}}), http.StatusUnprocessableEntity)

	// Reminders.
	overdue := send("50", false, 30)
	r = owner.do(http.MethodPost, invoices+"/send_reminders", map[string]any{"sales_invoice_reminders": []map[string]any{{
		"contact_id": overdue.ContactID, "workflow_id": overdue.WorkflowID, "document_style_id": overdue.DocumentStyleID,
		"identity_id": overdue.IdentityID, "sales_invoice_ids": []string{overdue.ID}, "reminder_text": "Please pay:\n{invoices}",
	}}})
	wantStatus(t, r, http.StatusOK)
	reminded := decode[[]paidInvoice](t, r)
	if len(reminded) != 1 || reminded[0].State != "reminded" || reminded[0].ReminderCount != 1 {
		t.Fatalf("reminded = %+v", reminded)
	}
	h.mailer.mu.Lock()
	last := h.mailer.documents[len(h.mailer.documents)-1]
	h.mailer.mu.Unlock()
	if last.To[0] != "billing@example.com" || len(last.Files) != 1 || !slices.ContainsFunc([]string{last.Body}, func(b string) bool { return len(b) > 0 && b[:10] == "Please pay" }) {
		t.Errorf("reminder mail = %+v", last.Body)
	}
	wantStatus(t, owner.do(http.MethodPost, invoices+"/send_reminders", map[string]any{"sales_invoice_reminders": []map[string]any{{
		"contact_id": overdue.ContactID, "workflow_id": overdue.WorkflowID, "document_style_id": overdue.DocumentStyleID,
		"identity_id": overdue.IdentityID, "sales_invoice_ids": []string{inv.ID},
	}}}), http.StatusUnprocessableEntity)

	ov := decode[httpapi.Overview](t, owner.do(http.MethodGet, api+"/overview", nil))
	if ov.Sales == nil || ov.Sales.ToReceive.Count != 1 || ov.Sales.ToReceive.Amount != "60.5" || ov.Sales.ToRemind.Count != 1 || ov.Sales.Currency != "EUR" {
		t.Errorf("overview sales = %+v", ov.Sales)
	}
}

func TestBankTransactionsPayInvoices(t *testing.T) {
	h := newHarness(t, options{})
	owner := h.client()
	owner.register()
	api, invoices, send := salesSetup(t, owner)
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

	inv := send("1000", true, 20)
	r := owner.do(http.MethodPatch, api+"/financial_mutations/"+incoming.ID+"/link_booking", map[string]any{"booking_type": "SalesInvoice", "booking_id": inv.ID})
	wantStatus(t, r, http.StatusOK)
	var linked struct {
		mutation
		Payments []payment `json:"payments"`
	}
	if err := json.Unmarshal(r.body, &linked); err != nil {
		t.Fatal(err)
	}
	if linked.State != "processed" || linked.AmountOpen != "0.0" || len(linked.Payments) != 1 || linked.Payments[0].InvoiceID != inv.ID || linked.Payments[0].Price != "1000.0" {
		t.Fatalf("linked = %+v", linked)
	}
	got := decode[paidInvoice](t, owner.do(http.MethodGet, invoices+"/"+inv.ID, nil))
	if got.State != "paid" || deref(got.Payments[0].FinancialMutationID) != incoming.ID {
		t.Errorf("invoice = %+v", got)
	}
	r = owner.do(http.MethodDelete, api+"/financial_mutations/"+incoming.ID+"/unlink_booking", map[string]any{"booking_type": "Payment", "booking_id": linked.Payments[0].ID})
	wantStatus(t, r, http.StatusOK)
	if m := decode[mutation](t, r); m.State != "unprocessed" || m.AmountOpen != "1000.0" {
		t.Errorf("unlinked = %+v", m)
	}
	if got := decode[paidInvoice](t, owner.do(http.MethodGet, invoices+"/"+inv.ID, nil)); got.State == "paid" || len(got.Payments) != 0 {
		t.Errorf("after unlink = %+v", got)
	}

	// The invoice's payments endpoint links a transaction too, for members
	// who may also work in the bank.
	seller := h.client()
	seller.register()
	invite(t, h, owner, seller, strings.TrimPrefix(api, "/api/v2/"), "seller@example.com", map[string]any{"user_type": "employee", "permissions": []string{"sales_invoices"}})
	wantStatus(t, seller.do(http.MethodPost, invoices+"/"+inv.ID+"/payments", map[string]any{"payment": map[string]any{"payment_date": "2026-11-01", "price": "1000", "financial_mutation_id": incoming.ID}}), http.StatusForbidden)
	r = owner.do(http.MethodPost, invoices+"/"+inv.ID+"/payments", map[string]any{"payment": map[string]any{"payment_date": "2026-11-01", "price": "1000", "financial_mutation_id": incoming.ID}})
	wantStatus(t, r, http.StatusCreated)
	if p := decode[payment](t, r); deref(p.FinancialMutationID) != incoming.ID {
		t.Errorf("payment = %+v", p)
	}
	if m := decode[mutation](t, owner.do(http.MethodGet, api+"/financial_mutations/"+incoming.ID, nil)); m.State != "processed" {
		t.Errorf("mutation = %+v", m)
	}
	wantStatus(t, owner.do(http.MethodPatch, api+"/financial_mutations/"+incoming.ID+"/link_booking", map[string]any{"booking_type": "SalesInvoice", "booking_id": inv.ID}), http.StatusUnprocessableEntity)
}

func TestPaymentWebhooks(t *testing.T) {
	ctx := context.Background()
	pool, cleanup, err := postgrestest.Start(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()
	h := newHarness(t, options{pool: pool})
	owner := h.client()
	owner.register()
	api, invoices, send := salesSetup(t, owner)
	rc := newReceiver(t)
	wantStatus(t, owner.do(http.MethodPatch, "/api/instance/settings", map[string]bool{"webhooks_allow_local": true}), http.StatusOK)
	wantStatus(t, owner.do(http.MethodPost, api+"/webhooks", map[string]any{"url": rc.srv.URL, "enabled_events": []string{"payment", "sales_invoice_state_changed_to_paid", "sales_invoice_marked_as_dubious", "credit_invoice_created_from_original"}}), http.StatusCreated)

	inv := send("100", false, 3)
	r := owner.do(http.MethodPost, invoices+"/"+inv.ID+"/payments", map[string]any{"payment": map[string]any{"payment_date": adminNow().Format(time.DateOnly), "price": "121"}})
	wantStatus(t, r, http.StatusCreated)
	wantStatus(t, owner.do(http.MethodDelete, invoices+"/"+inv.ID+"/payments/"+decode[payment](t, r).ID, nil), http.StatusNoContent)
	wantStatus(t, owner.do(http.MethodPatch, invoices+"/"+inv.ID+"/mark_as_dubious", map[string]any{}), http.StatusOK)
	wantStatus(t, owner.do(http.MethodPatch, invoices+"/"+inv.ID+"/duplicate_creditinvoice", map[string]any{}), http.StatusOK)
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
	want := []string{"credit_invoice_created_from_original", "payment_destroyed", "payment_registered", "sales_invoice_marked_as_dubious", "sales_invoice_state_changed_to_paid"}
	if !slices.Equal(actions, want) {
		t.Errorf("actions = %v, want %v", actions, want)
	}
}
