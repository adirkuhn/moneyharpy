package httpapi_test

import (
	"context"
	"encoding/json"
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/adirkuhn/moneyharpy/internal/postgres/postgrestest"
)

type product struct {
	ID              string  `json:"id"`
	Title           *string `json:"title"`
	Description     *string `json:"description"`
	Identifier      *string `json:"identifier"`
	Price           string  `json:"price"`
	Currency        string  `json:"currency"`
	Frequency       *int    `json:"frequency"`
	FrequencyType   *string `json:"frequency_type"`
	TaxRateID       *string `json:"tax_rate_id"`
	LedgerAccountID string  `json:"ledger_account_id"`
	Active          bool    `json:"active"`
	URI             string  `json:"uri"`
}

type project struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	State  string `json:"state"`
	Budget *int   `json:"budget"`
	URI    string `json:"uri"`
}

func TestProductsLifecycle(t *testing.T) {
	h := newHarness(t, options{})
	owner, stranger := h.client(), h.client()
	owner.register()
	stranger.register()
	a := owner.createAdministration("Harpy")
	api := "/api/v2/" + a.Id
	revenue := category(t, owner, a.Id, "Revenue")
	vat21 := salesRate(t, owner, a.Id, "21")

	r := owner.do(http.MethodPost, api+"/products.json", map[string]any{"product": map[string]any{
		"description": "Geldvogel", "price": "50,50", "tax_rate_id": vat21, "ledger_account_id": revenue, "identifier": "GV.1", "frequency_preset": "1:month",
	}})
	wantStatus(t, r, http.StatusCreated)
	p := decode[product](t, r)
	if p.Price != "50.5" || p.Currency != "EUR" || deref(p.TaxRateID) != vat21 || p.LedgerAccountID != revenue || deref(p.Frequency) != 1 ||
		deref(p.FrequencyType) != "month" || p.Title != nil || !p.Active || p.URI != "/"+a.Id+"/products/"+p.ID {
		t.Fatalf("created = %+v", p)
	}
	missing := owner.do(http.MethodPost, api+"/products", map[string]any{"product": map[string]any{}})
	wantStatus(t, missing, http.StatusUnprocessableEntity)
	if !strings.Contains(string(missing.body), "must contain a title or description") {
		t.Errorf("missing = %s", missing.body)
	}
	wantStatus(t, owner.do(http.MethodPost, api+"/products", map[string]any{"product": map[string]any{"colour": "red"}}), http.StatusBadRequest)
	wantStatus(t, owner.do(http.MethodPost, api+"/products", map[string]any{"product": map[string]any{"title": "x", "price": 1, "ledger_account_id": revenue, "frequency_type": "fortnight"}}), http.StatusBadRequest)

	if got := decode[product](t, owner.do(http.MethodGet, api+"/products/identifier/GV.1", nil)); got.ID != p.ID {
		t.Errorf("by identifier = %+v", got)
	}
	wantStatus(t, owner.do(http.MethodGet, api+"/products/identifier/nope.json", nil), http.StatusNotFound)
	wantStatus(t, stranger.do(http.MethodGet, api+"/products/"+p.ID, nil), http.StatusNotFound)
	if got := decode[product](t, owner.do(http.MethodGet, api+"/products/"+p.ID, nil)); got.ID != p.ID {
		t.Errorf("fetched = %+v", got)
	}

	up := owner.do(http.MethodPatch, api+"/products/"+p.ID, map[string]any{"product": map[string]any{"title": "Harpy", "price": 60, "frequency_preset": "0:nil"}})
	wantStatus(t, up, http.StatusOK)
	if got := decode[product](t, up); deref(got.Title) != "Harpy" || got.Price != "60.0" || got.Frequency != nil || got.FrequencyType != nil {
		t.Errorf("updated = %+v", got)
	}
	other := decode[product](t, owner.do(http.MethodPost, api+"/products", map[string]any{"product": map[string]any{"title": "Apple", "price": "1", "ledger_account_id": revenue, "currency": "USD"}}))

	list := owner.do(http.MethodGet, api+"/products?per_page=1", nil)
	if got := decode[[]product](t, list); len(got) != 1 || got[0].ID != other.ID || list.header.Get("X-Total-Count") != "2" || !strings.Contains(list.header.Get("Link"), `rel="next"`) {
		t.Errorf("page = %+v, total %s", got, list.header.Get("X-Total-Count"))
	}
	if got := decode[[]product](t, owner.do(http.MethodGet, api+"/products?query=harp&currency=EUR&ledger_account_id="+revenue, nil)); len(got) != 1 || got[0].ID != p.ID {
		t.Errorf("search = %+v", got)
	}
	if got := decode[[]product](t, owner.do(http.MethodGet, api+"/products?for_checkout=true", nil)); len(got) != 0 {
		t.Errorf("for checkout = %+v", got)
	}

	link := owner.do(http.MethodPost, api+"/products/"+p.ID+"/sales_link", nil)
	wantStatus(t, link, http.StatusNotFound)
	if !strings.Contains(string(link.body), "Product not valid for online sales") {
		t.Errorf("sales link = %s", link.body)
	}
	wantStatus(t, owner.do(http.MethodPost, api+"/products/"+p.ID+"/sales_link?contact_id=1", nil), http.StatusNotFound)

	// A line that names the product takes its description, price, rate and category.
	customer := decode[contact](t, owner.do(http.MethodPost, api+"/contacts", map[string]any{"contact": map[string]any{"company_name": "Example Shop B.V."}}))
	r = owner.do(http.MethodPost, api+"/sales_invoices", map[string]any{"sales_invoice": map[string]any{
		"contact_id": customer.ID, "details_attributes": map[string]any{"0": map[string]any{"product_id": p.ID}},
	}})
	wantStatus(t, r, http.StatusCreated)
	inv := decode[struct {
		Details []struct {
			ProductID       *string `json:"product_id"`
			Description     string  `json:"description"`
			Price           string  `json:"price"`
			TaxRateID       string  `json:"tax_rate_id"`
			LedgerAccountID string  `json:"ledger_account_id"`
		} `json:"details"`
	}](t, r)
	if d := inv.Details[0]; deref(d.ProductID) != p.ID || d.Description != "Geldvogel" || d.Price != "60.0" || d.TaxRateID != vat21 || d.LedgerAccountID != revenue {
		t.Errorf("line = %+v", d)
	}
	wantStatus(t, owner.do(http.MethodPost, api+"/sales_invoices", map[string]any{"sales_invoice": map[string]any{
		"contact_id": customer.ID, "details_attributes": []map[string]any{{"product_id": "1", "tax_rate_id": vat21}},
	}}), http.StatusUnprocessableEntity)

	falcon := decode[project](t, owner.do(http.MethodPost, api+"/projects", map[string]any{"project": map[string]any{"name": "Falcon"}}))
	if got := decode[project](t, owner.do(http.MethodGet, api+"/projects/"+falcon.ID, nil)); got.ID != falcon.ID {
		t.Errorf("fetched = %+v", got)
	}
	type lines struct {
		Details []struct {
			ProductID *string `json:"product_id"`
			ProjectID *string `json:"project_id"`
			Price     string  `json:"price"`
		} `json:"details"`
	}
	for path, wrapper := range map[string]string{"/recurring_sales_invoices": "recurring_sales_invoice", "/external_sales_invoices": "external_sales_invoice"} {
		r := owner.do(http.MethodPost, api+path, map[string]any{wrapper: map[string]any{
			"contact_id": customer.ID, "details_attributes": []map[string]any{{"product_id": p.ID, "project_id": falcon.ID}},
		}})
		wantStatus(t, r, http.StatusCreated)
		if d := decode[lines](t, r).Details[0]; deref(d.ProductID) != p.ID || deref(d.ProjectID) != falcon.ID || d.Price != "60.0" {
			t.Errorf("%s line = %+v", path, d)
		}
	}

	wantKept(t, owner.do(http.MethodDelete, api+"/products/"+p.ID, nil), "deactivated")
	if got := decode[[]product](t, owner.do(http.MethodGet, api+"/products", nil)); len(got) != 1 || got[0].ID != other.ID {
		t.Errorf("active = %+v", got)
	}
	if got := decode[[]product](t, owner.do(http.MethodGet, api+"/products?active=false", nil)); len(got) != 2 {
		t.Errorf("with deactivated = %+v", got)
	}
	wantStatus(t, owner.do(http.MethodDelete, api+"/products/"+other.ID+".json", nil), http.StatusNoContent)
	wantStatus(t, owner.do(http.MethodGet, api+"/products/"+other.ID, nil), http.StatusNotFound)
}

func TestProjectsLifecycle(t *testing.T) {
	h := newHarness(t, options{})
	owner := h.client()
	owner.register()
	a := owner.createAdministration("Harpy")
	api := "/api/v2/" + a.Id

	r := owner.do(http.MethodPost, api+"/projects.json", map[string]any{"project": map[string]any{"name": "Falcon", "budget": 10}})
	wantStatus(t, r, http.StatusCreated)
	falcon := decode[project](t, r)
	if falcon.State != "active" || deref(falcon.Budget) != 10 || falcon.URI != "/"+a.Id+"/projects/"+falcon.ID {
		t.Fatalf("created = %+v", falcon)
	}
	wantStatus(t, owner.do(http.MethodPost, api+"/projects", map[string]any{"project": map[string]any{"name": "falcon"}}), http.StatusUnprocessableEntity)
	wantStatus(t, owner.do(http.MethodPost, api+"/projects", map[string]any{"project": map[string]any{"name": "Owl", "budget": "lots"}}), http.StatusBadRequest)
	up := owner.do(http.MethodPatch, api+"/projects/"+falcon.ID, map[string]any{"project": map[string]any{"name": "Eagle", "budget": nil}})
	wantStatus(t, up, http.StatusOK)
	if got := decode[project](t, up); got.Name != "Eagle" || got.Budget != nil {
		t.Errorf("updated = %+v", got)
	}
	owl := decode[project](t, owner.do(http.MethodPost, api+"/projects", map[string]any{"project": map[string]any{"name": "Owl", "budget": "2,5"}}))
	if deref(owl.Budget) != 3 {
		t.Errorf("owl budget = %v", owl.Budget)
	}

	// A journal entry books a row on the project; deleting it archives it.
	revenue := category(t, owner, a.Id, "Revenue")
	wantStatus(t, owner.do(http.MethodPost, api+"/documents/general_journal_documents", map[string]any{"general_journal_document": map[string]any{
		"reference": "Eagle", "date": "2026-11-01", "general_journal_document_entries_attributes": []map[string]any{
			{"ledger_account_id": revenue, "debit": "5", "project_id": falcon.ID}, {"ledger_account_id": revenue, "credit": "5"},
		},
	}}), http.StatusCreated)
	wantKept(t, owner.do(http.MethodDelete, api+"/projects/"+falcon.ID, nil), "archived")
	wantStatus(t, owner.do(http.MethodPost, api+"/documents/general_journal_documents", map[string]any{"general_journal_document": map[string]any{
		"reference": "Archived", "date": "2026-11-01", "general_journal_document_entries_attributes": []map[string]any{
			{"ledger_account_id": revenue, "debit": "5", "project_id": falcon.ID}, {"ledger_account_id": revenue, "credit": "5"},
		},
	}}), http.StatusUnprocessableEntity)

	list := owner.do(http.MethodGet, api+"/projects", nil)
	if got := decode[[]project](t, list); len(got) != 1 || got[0].ID != owl.ID || list.header.Get("X-Total-Count") != "1" {
		t.Errorf("active = %+v", got)
	}
	if got := decode[[]project](t, owner.do(http.MethodGet, api+"/projects?filter=state:archived", nil)); len(got) != 1 || got[0].ID != falcon.ID {
		t.Errorf("archived = %+v", got)
	}
	if got := decode[[]project](t, owner.do(http.MethodGet, api+"/projects?filter=state:active|archived", nil)); len(got) != 2 {
		t.Errorf("both = %+v", got)
	}
	wantStatus(t, owner.do(http.MethodGet, api+"/projects?filter=state:gone", nil), http.StatusBadRequest)
	back := owner.do(http.MethodPatch, api+"/projects/"+falcon.ID, map[string]any{"project": map[string]any{"state": "active"}})
	if got := decode[project](t, back); got.State != "active" {
		t.Errorf("reactivated = %+v", got)
	}
	wantStatus(t, owner.do(http.MethodDelete, api+"/projects/"+owl.ID, nil), http.StatusNoContent)
	wantStatus(t, owner.do(http.MethodGet, api+"/projects/"+owl.ID, nil), http.StatusNotFound)
}

func TestProductAndProjectWebhooks(t *testing.T) {
	ctx := context.Background()
	pool, cleanup, err := postgrestest.Start(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()
	h := newHarness(t, options{pool: pool})
	owner := h.client()
	owner.register()
	a := owner.createAdministration("Harpy")
	api := "/api/v2/" + a.Id
	revenue := category(t, owner, a.Id, "Revenue")
	rc := newReceiver(t)
	wantStatus(t, owner.do(http.MethodPatch, "/api/instance/settings", map[string]bool{"webhooks_allow_local": true}), http.StatusOK)
	wantStatus(t, owner.do(http.MethodPost, api+"/webhooks", map[string]any{"url": rc.srv.URL, "enabled_events": []string{"product", "project"}}), http.StatusCreated)

	p := decode[product](t, owner.do(http.MethodPost, api+"/products", map[string]any{"product": map[string]any{"title": "Apple", "price": "1", "ledger_account_id": revenue}}))
	wantStatus(t, owner.do(http.MethodPatch, api+"/products/"+p.ID, map[string]any{"product": map[string]any{"price": "2"}}), http.StatusOK)
	wantStatus(t, owner.do(http.MethodPatch, api+"/products/"+p.ID, map[string]any{"product": map[string]any{"active": false}}), http.StatusOK)
	wantStatus(t, owner.do(http.MethodPatch, api+"/products/"+p.ID, map[string]any{"product": map[string]any{"active": true}}), http.StatusOK)
	wantStatus(t, owner.do(http.MethodDelete, api+"/products/"+p.ID, nil), http.StatusNoContent)
	pr := decode[project](t, owner.do(http.MethodPost, api+"/projects", map[string]any{"project": map[string]any{"name": "Falcon"}}))
	wantStatus(t, owner.do(http.MethodPatch, api+"/projects/"+pr.ID, map[string]any{"project": map[string]any{"name": "Eagle"}}), http.StatusOK)
	wantStatus(t, owner.do(http.MethodPatch, api+"/projects/"+pr.ID, map[string]any{"project": map[string]any{"state": "archived"}}), http.StatusOK)
	wantStatus(t, owner.do(http.MethodPatch, api+"/projects/"+pr.ID, map[string]any{"project": map[string]any{"state": "active"}}), http.StatusOK)
	wantStatus(t, owner.do(http.MethodDelete, api+"/projects/"+pr.ID, nil), http.StatusNoContent)
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
	want := []string{"product_activated", "product_created", "product_deactivated", "product_destroyed", "product_updated",
		"project_activated", "project_archived", "project_created", "project_destroyed", "project_updated"}
	if !slices.Equal(actions, want) {
		t.Errorf("actions = %v, want %v", actions, want)
	}
}
