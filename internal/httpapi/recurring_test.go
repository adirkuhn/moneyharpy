package httpapi_test

import (
	"context"
	"encoding/json"
	"net/http"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/adirkuhn/moneyharpy/internal/httpapi"
	"github.com/adirkuhn/moneyharpy/internal/postgres/postgrestest"
)

type recurringInvoice struct {
	ID              string  `json:"id"`
	ContactID       string  `json:"contact_id"`
	WorkflowID      string  `json:"workflow_id"`
	StartDate       string  `json:"start_date"`
	InvoiceDate     string  `json:"invoice_date"`
	LastDate        *string `json:"last_date"`
	Active          bool    `json:"active"`
	AutoSend        bool    `json:"auto_send"`
	FrequencyType   string  `json:"frequency_type"`
	Frequency       int     `json:"frequency"`
	Reference       *string `json:"reference"`
	TotalIncl       string  `json:"total_price_incl_tax"`
	InvoicesCreated int     `json:"invoices_created"`
	ScheduledUserID *string `json:"sending_scheduled_user_id"`
	Subscription    *struct {
		ID string `json:"id"`
	} `json:"subscription"`
	URI     string `json:"uri"`
	Contact *struct {
		ID string `json:"id"`
	} `json:"contact"`
	Details []struct {
		ID          string  `json:"id"`
		Description string  `json:"description"`
		Period      *string `json:"period"`
	} `json:"details"`
	Notes []struct {
		ID string `json:"id"`
	} `json:"notes"`
}

// recurringSetup makes an administration with a customer and returns the
// API path, the recurring invoices path and a function making a recurring
// invoice of price excluding 21% VAT with the given extra fields.
func recurringSetup(t *testing.T, owner *client) (string, string, func(price string, extra map[string]any) recurringInvoice) {
	t.Helper()
	a := owner.createAdministration("Harpy")
	api := "/api/v2/" + a.Id
	customer := decode[contact](t, owner.do(http.MethodPost, api+"/contacts", map[string]any{"contact": map[string]any{
		"company_name": "Example Shop B.V.", "send_invoices_to_email": "billing@example.com",
	}}))
	vat21 := salesRate(t, owner, a.Id, "21")
	create := func(price string, extra map[string]any) recurringInvoice {
		t.Helper()
		body := map[string]any{
			"contact_id": customer.ID, "reference": "Contract 7", "frequency_type": "month",
			"details_attributes": map[string]any{"0": map[string]any{"description": "Hosting {period}", "price": price, "tax_rate_id": vat21}},
		}
		for k, v := range extra {
			body[k] = v
		}
		r := owner.do(http.MethodPost, api+"/recurring_sales_invoices", map[string]any{"recurring_sales_invoice": body})
		wantStatus(t, r, http.StatusCreated)
		return decode[recurringInvoice](t, r)
	}
	return api, api + "/recurring_sales_invoices", create
}

func TestRecurringSalesInvoiceLifecycle(t *testing.T) {
	h := newHarness(t, options{})
	owner := h.client()
	owner.register()
	api, recurrings, create := recurringSetup(t, owner)
	today := adminNow().Format(time.DateOnly)

	r := create("100", map[string]any{"frequency": "3", "has_desired_count": true, "desired_count": 4, "auto_send": "true"})
	if !r.Active || r.FrequencyType != "month" || r.Frequency != 3 || r.InvoiceDate != today || r.StartDate != today || r.TotalIncl != "121.0" ||
		r.LastDate == nil || !r.AutoSend || r.ScheduledUserID == nil || r.Contact == nil || r.WorkflowID == "" {
		t.Fatalf("created = %+v", r)
	}
	if r.URI != strings.TrimPrefix(recurrings, "/api/v2")+"/"+r.ID {
		t.Errorf("uri = %s", r.URI)
	}
	wantStatus(t, owner.do(http.MethodPost, recurrings, map[string]any{"recurring_sales_invoice": map[string]any{"frequency_type": "fortnight"}}), http.StatusBadRequest)
	wantStatus(t, owner.do(http.MethodPost, recurrings, map[string]any{"recurring_sales_invoice": map[string]any{"invoice_date": "2020-01-01"}}), http.StatusUnprocessableEntity)
	wantStatus(t, owner.do(http.MethodPost, recurrings, map[string]any{"recurring_sales_invoice": map[string]any{"colour": "red"}}), http.StatusBadRequest)

	next := adminNow().AddDate(0, 1, 0).Format(time.DateOnly)
	r2 := owner.do(http.MethodPatch, recurrings+"/"+r.ID, map[string]any{"recurring_sales_invoice": map[string]any{
		"invoice_date": next, "frequency_type": "year", "frequency": 1, "auto_send": false, "last_date": "",
		"details_attributes": []map[string]any{{"id": r.Details[0].ID, "price": "200"}},
	}})
	wantStatus(t, r2, http.StatusOK)
	if up := decode[recurringInvoice](t, r2); up.InvoiceDate != next || up.FrequencyType != "year" || up.AutoSend || up.LastDate != nil || up.TotalIncl != "242.0" || up.StartDate != today {
		t.Errorf("updated = %+v", up)
	}

	n := owner.do(http.MethodPost, recurrings+"/"+r.ID+"/notes", map[string]any{"note": map[string]any{"note": "Renew in March"}})
	wantStatus(t, n, http.StatusCreated)
	note := decode[struct {
		ID         string `json:"id"`
		EntityType string `json:"entity_type"`
	}](t, n)
	if note.EntityType != "RecurringSalesInvoice" {
		t.Errorf("note = %+v", note)
	}
	if got := decode[recurringInvoice](t, owner.do(http.MethodGet, recurrings+"/"+r.ID, nil)); len(got.Notes) != 1 {
		t.Errorf("notes = %+v", got.Notes)
	}
	wantStatus(t, owner.do(http.MethodDelete, recurrings+"/"+r.ID+"/notes/"+note.ID, nil), http.StatusNoContent)

	// Due today: the worker makes its first invoice.
	due := create("50", nil)
	// The shared database holds other tests' administrations too, whose
	// work may fail; only errors about this administration count.
	if err := h.sales.Work(context.Background()); err != nil {
		if strings.Contains(err.Error(), strings.TrimPrefix(api, "/api/v2/")) {
			t.Fatal(err)
		}
		t.Log(err)
	}
	made := decode[[]salesInvoice](t, owner.do(http.MethodGet, api+"/sales_invoices?filter=recurring_sales_invoice_id:"+due.ID, nil))
	if len(made) != 1 || made[0].State != "draft" {
		t.Fatalf("made = %+v", made)
	}
	if got := decode[recurringInvoice](t, owner.do(http.MethodGet, recurrings+"/"+due.ID, nil)); got.InvoicesCreated != 1 || got.InvoiceDate == today {
		t.Errorf("after the worker = %+v", got)
	}

	r3 := owner.do(http.MethodGet, recurrings+"?per_page=1", nil)
	wantStatus(t, r3, http.StatusOK)
	if list := decode[[]recurringInvoice](t, r3); len(list) != 1 || r3.header.Get("X-Total-Count") != "2" || !strings.Contains(r3.header.Get("Link"), `rel="next"`) {
		t.Errorf("list = %d, total %s", len(list), r3.header.Get("X-Total-Count"))
	}
	if list := decode[[]recurringInvoice](t, owner.do(http.MethodGet, recurrings+"?filter=frequency:year", nil)); len(list) != 1 || list[0].ID != r.ID {
		t.Errorf("yearly = %+v", list)
	}
	wantStatus(t, owner.do(http.MethodGet, recurrings+"?filter=state:draft", nil), http.StatusBadRequest)
	versions := decode[[]httpapi.SynchronizationItem](t, owner.do(http.MethodGet, recurrings+"/synchronization?filter=state:all", nil))
	if len(versions) != 2 {
		t.Errorf("versions = %+v", versions)
	}
	f := owner.do(http.MethodPost, recurrings+"/synchronization", map[string]any{"ids": []string{r.ID, due.ID}})
	wantStatus(t, f, http.StatusOK)
	if fetched := decode[[]recurringInvoice](t, f); len(fetched) != 2 {
		t.Errorf("fetched = %d", len(fetched))
	}

	// Contacts with recurring invoices stay.
	wantKept(t, owner.do(http.MethodDelete, api+"/contacts/"+r.ContactID, nil), "archived")

	// Deleting one that made invoices keeps it, deactivated.
	d := owner.do(http.MethodDelete, recurrings+"/"+due.ID, nil)
	wantStatus(t, d, http.StatusNoContent)
	if d.header.Get("X-Kept-As") != "deactivated" {
		t.Errorf("kept as = %q", d.header.Get("X-Kept-As"))
	}
	if got := decode[recurringInvoice](t, owner.do(http.MethodGet, recurrings+"/"+due.ID, nil)); got.Active {
		t.Error("still active")
	}
	wantStatus(t, owner.do(http.MethodDelete, recurrings+"/"+due.ID, nil), http.StatusUnprocessableEntity)
	wantStatus(t, owner.do(http.MethodPatch, recurrings+"/"+due.ID+"/deactivate", nil), http.StatusUnprocessableEntity)

	off := owner.do(http.MethodPatch, recurrings+"/"+r.ID+"/deactivate", nil)
	wantStatus(t, off, http.StatusOK)
	if got := decode[recurringInvoice](t, off); got.Active {
		t.Errorf("deactivated = %+v", got)
	}
	fresh := create("10", nil)
	d = owner.do(http.MethodDelete, recurrings+"/"+fresh.ID, nil)
	wantStatus(t, d, http.StatusNoContent)
	if d.header.Get("X-Kept-As") != "" {
		t.Errorf("kept as = %q", d.header.Get("X-Kept-As"))
	}
	wantStatus(t, owner.do(http.MethodGet, recurrings+"/"+fresh.ID, nil), http.StatusNotFound)
}

func TestRecurringSalesInvoicesFollowPermissions(t *testing.T) {
	h := newHarness(t, options{})
	owner, banker := h.client(), h.client()
	owner.register()
	banker.register()
	a := owner.createAdministration("Harpy")
	invite(t, h, owner, banker, a.Id, "banker@example.com", map[string]any{"user_type": "employee", "permissions": []string{"bank"}})
	wantStatus(t, banker.do(http.MethodGet, "/api/v2/"+a.Id+"/recurring_sales_invoices", nil), http.StatusForbidden)
	wantStatus(t, owner.do(http.MethodGet, "/api/v2/"+a.Id+"/recurring_sales_invoices", nil), http.StatusOK)
}

func TestRecurringSalesInvoiceWebhooks(t *testing.T) {
	ctx := context.Background()
	pool, cleanup, err := postgrestest.Start(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()
	h := newHarness(t, options{pool: pool})
	owner := h.client()
	owner.register()
	api, recurrings, create := recurringSetup(t, owner)
	rc := newReceiver(t)
	wantStatus(t, owner.do(http.MethodPatch, "/api/instance/settings", map[string]bool{"webhooks_allow_local": true}), http.StatusOK)
	wantStatus(t, owner.do(http.MethodPost, api+"/webhooks", map[string]any{"url": rc.srv.URL, "enabled_events": []string{"recurring_sales_invoice", "sales_invoice_created_based_on_recurring"}}), http.StatusCreated)

	r := create("100", nil)
	wantStatus(t, owner.do(http.MethodPatch, recurrings+"/"+r.ID, map[string]any{"recurring_sales_invoice": map[string]any{"reference": "Contract 8"}}), http.StatusOK)
	if err := h.sales.Work(ctx); err != nil {
		t.Fatal(err)
	}
	wantStatus(t, owner.do(http.MethodDelete, recurrings+"/"+r.ID, nil), http.StatusNoContent)
	other := create("10", map[string]any{"invoice_date": adminNow().AddDate(0, 0, 7).Format(time.DateOnly)})
	wantStatus(t, owner.do(http.MethodDelete, recurrings+"/"+other.ID, nil), http.StatusNoContent)
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
	want := []string{"recurring_sales_invoice_created", "recurring_sales_invoice_created", "recurring_sales_invoice_deactivated",
		"recurring_sales_invoice_destroyed", "recurring_sales_invoice_invoice_created", "recurring_sales_invoice_updated", "sales_invoice_created_based_on_recurring"}
	if !slices.Equal(actions, want) {
		t.Errorf("actions = %v, want %v", actions, want)
	}
}
