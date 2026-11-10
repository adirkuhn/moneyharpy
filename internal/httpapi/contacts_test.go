package httpapi_test

import (
	"net/http"
	"strings"
	"testing"
)

type contactPerson struct {
	ID         string  `json:"id"`
	ContactID  string  `json:"contact_id"`
	Firstname  *string `json:"firstname"`
	Department *string `json:"department"`
}

type contact struct {
	ID                  string          `json:"id"`
	CompanyName         string          `json:"company_name"`
	Firstname           *string         `json:"firstname"`
	CustomerID          string          `json:"customer_id"`
	Country             string          `json:"country"`
	DeliveryMethod      string          `json:"delivery_method"`
	Email               *string         `json:"email"`
	SendInvoicesToEmail *string         `json:"send_invoices_to_email"`
	SepaActive          bool            `json:"sepa_active"`
	SepaMandateDate     *string         `json:"sepa_mandate_date"`
	InvoiceWorkflowID   *string         `json:"invoice_workflow_id"`
	Archived            bool            `json:"archived"`
	Version             int             `json:"version"`
	URI                 string          `json:"uri"`
	People              []contactPerson `json:"contact_people"`
	Notes               []struct {
		ID   string `json:"id"`
		Note string `json:"note"`
		Todo bool   `json:"todo"`
	} `json:"notes"`
	CustomFields []struct {
		ID    string `json:"id"`
		Name  string `json:"name"`
		Value string `json:"value"`
	} `json:"custom_fields"`
}

func TestContactsLifecycle(t *testing.T) {
	h := newHarness(t, options{})
	owner := h.client()
	owner.register()
	a := owner.createAdministration("Harpy")
	api := "/api/v2/" + a.Id

	field := decode[struct {
		ID string `json:"id"`
	}](t, owner.do(http.MethodPost, api+"/custom_fields", map[string]any{"custom_field": map[string]any{"name": "Account manager", "source": "contact"}}))

	created := owner.do(http.MethodPost, api+"/contacts", map[string]any{"contact": map[string]any{
		"company_name": "Example Shop B.V.", "send_invoices_to_email": "billing@example.com", "delivery_method": "email",
		"sepa_active": "true", "sepa_iban": "NL91ABNA0417164300", "sepa_iban_account_name": "Example Shop", "sepa_mandate_id": "M-1",
		"sepa_mandate_date": "2026-11-01", "custom_fields_attributes": map[string]any{"0": map[string]any{"id": field.ID, "value": "Ann"}},
		"contact_person": map[string]any{"firstname": "Zoë", "email": "zoe@example.com"},
	}})
	wantStatus(t, created, http.StatusCreated)
	c := decode[contact](t, created)
	if c.CustomerID != "1" || c.Country != "NL" || c.DeliveryMethod != "Email" || !c.SepaActive || deref(c.SepaMandateDate) != "2026-11-01" {
		t.Errorf("created = %+v", c)
	}
	if deref(c.Email) != "billing@example.com" || c.URI != "/"+a.Id+"/contacts/"+c.ID || c.Firstname != nil {
		t.Errorf("moneybird fields = %+v", c)
	}
	if len(c.People) != 1 || len(c.CustomFields) != 1 || c.CustomFields[0].Name != "Account manager" {
		t.Errorf("nested = %+v %+v", c.People, c.CustomFields)
	}

	person := decode[contact](t, owner.do(http.MethodPost, api+"/contacts", map[string]any{"contact": map[string]any{"firstname": "Ann", "lastname": "Jansen"}}))
	bad := owner.do(http.MethodPost, api+"/contacts", map[string]any{"contact": map[string]any{"city": "Utrecht", "customer_id": "1"}})
	wantStatus(t, bad, http.StatusUnprocessableEntity)
	if body := string(bad.body); !strings.Contains(body, "base") || !strings.Contains(body, "customer_id") {
		t.Errorf("errors = %s", bad.body)
	}

	// Lists page with Moneybird's Link header; Ann Jansen sorts first.
	page := owner.do(http.MethodGet, api+"/contacts?per_page=1&page=1", nil)
	wantStatus(t, page, http.StatusOK)
	if list := decode[[]contact](t, page); len(list) != 1 || list[0].ID != person.ID {
		t.Errorf("first page = %+v", list)
	}
	if link := page.header.Get("Link"); !strings.Contains(link, `rel="next"`) || !strings.Contains(link, "page=2") || page.header.Get("X-Total-Count") != "2" {
		t.Errorf("Link = %q, total = %q", link, page.header.Get("X-Total-Count"))
	}
	found := decode[[]contact](t, owner.do(http.MethodGet, api+"/contacts?query=zoe%40example", nil))
	if len(found) != 1 || found[0].ID != c.ID {
		t.Errorf("search = %+v", found)
	}
	filtered := decode[[]contact](t, owner.do(http.MethodGet, api+"/contacts/filter?filter=contact_type:company,mandate_type:manual", nil))
	if len(filtered) != 1 || filtered[0].ID != c.ID {
		t.Errorf("filter = %+v", filtered)
	}
	wantStatus(t, owner.do(http.MethodGet, api+"/contacts/filter?filter=colour:red", nil), http.StatusBadRequest)
	byNumber := decode[contact](t, owner.do(http.MethodGet, api+"/contacts/customer_id/2", nil))
	if byNumber.ID != person.ID {
		t.Errorf("by customer id = %s", byNumber.ID)
	}

	updated := decode[contact](t, owner.do(http.MethodPatch, api+"/contacts/"+c.ID, map[string]any{"contact": map[string]any{"city": "Utrecht", "invoice_workflow_id": ""}}))
	if updated.InvoiceWorkflowID != nil || updated.Version < c.Version {
		t.Errorf("updated = %+v", updated)
	}

	versions := decode[[]struct {
		ID      string `json:"id"`
		Version int    `json:"version"`
	}](t, owner.do(http.MethodGet, api+"/contacts/synchronization", nil))
	if len(versions) != 2 {
		t.Errorf("versions = %+v", versions)
	}
	fetched := decode[[]contact](t, owner.do(http.MethodPost, api+"/contacts/synchronization", map[string]any{"ids": []any{c.ID, person.ID}}))
	if len(fetched) != 2 {
		t.Errorf("fetched = %d", len(fetched))
	}

	// Contact people and notes.
	zoe := c.People[0]
	p := decode[contactPerson](t, owner.do(http.MethodPatch, api+"/contacts/"+c.ID+"/contact_people/"+zoe.ID, map[string]any{"contact_person": map[string]any{"department": "Finance"}}))
	if deref(p.Department) != "Finance" || deref(p.Firstname) != "Zoë" || p.ContactID != c.ID {
		t.Errorf("person = %+v", p)
	}
	added := owner.do(http.MethodPost, api+"/contacts/"+c.ID+"/contact_people", map[string]any{"contact_person": map[string]any{"lastname": "de Vries"}})
	wantStatus(t, added, http.StatusCreated)
	wantStatus(t, owner.do(http.MethodDelete, api+"/contacts/"+c.ID+"/contact_people/"+decode[contactPerson](t, added).ID, nil), http.StatusNoContent)
	wantStatus(t, owner.do(http.MethodGet, api+"/contacts/"+c.ID+"/contact_people/"+person.ID, nil), http.StatusNotFound)

	note := owner.do(http.MethodPost, api+"/contacts/"+c.ID+"/notes", map[string]any{"note": map[string]any{"note": "Call back", "todo": true}})
	wantStatus(t, note, http.StatusCreated)
	withTodo := decode[[]contact](t, owner.do(http.MethodGet, api+"/contacts?todo=open", nil))
	if len(withTodo) != 1 || len(withTodo[0].Notes) != 1 || !withTodo[0].Notes[0].Todo {
		t.Errorf("to-dos = %+v", withTodo)
	}
	noteID := decode[struct {
		ID string `json:"id"`
	}](t, note).ID
	wantStatus(t, owner.do(http.MethodDelete, api+"/contacts/"+c.ID+"/notes/"+noteID, nil), http.StatusNoContent)

	// Archived contacts leave lists, come back when asked, and can be restored.
	wantStatus(t, owner.do(http.MethodPatch, api+"/contacts/"+person.ID+"/archive", nil), http.StatusNoContent)
	if list := decode[[]contact](t, owner.do(http.MethodGet, api+"/contacts", nil)); len(list) != 1 {
		t.Errorf("active = %d", len(list))
	}
	if list := decode[[]contact](t, owner.do(http.MethodGet, api+"/contacts?include_archived=true", nil)); len(list) != 2 {
		t.Errorf("with archived = %d", len(list))
	}
	if back := decode[contact](t, owner.do(http.MethodPatch, api+"/contacts/"+person.ID, map[string]any{"contact": map[string]any{"archived": false}})); back.Archived {
		t.Error("not restored")
	}

	doubles := decode[[]struct {
		Contacts []contact `json:"contacts"`
	}](t, owner.do(http.MethodGet, api+"/contacts/doubles", nil))
	if len(doubles) != 0 {
		t.Errorf("doubles = %+v", doubles)
	}

	wantStatus(t, owner.do(http.MethodDelete, api+"/contacts/"+person.ID, nil), http.StatusNoContent)
	wantStatus(t, owner.do(http.MethodGet, api+"/contacts/"+person.ID, nil), http.StatusNotFound)
}

func TestContactsFollowPermissions(t *testing.T) {
	h := newHarness(t, options{})
	owner, clerk, banker, stranger := h.client(), h.client(), h.client(), h.client()
	owner.register()
	clerk.register()
	banker.register()
	stranger.register()
	a := owner.createAdministration("Harpy")
	api := "/api/v2/" + a.Id
	invite(t, h, owner, clerk, a.Id, "clerk@example.com", map[string]any{"user_type": "employee", "permissions": []string{"documents"}})
	invite(t, h, owner, banker, a.Id, "banker@example.com", map[string]any{"user_type": "employee", "permissions": []string{"bank"}})

	wantStatus(t, clerk.do(http.MethodPost, api+"/contacts", map[string]any{"contact": map[string]any{"company_name": "Supplier"}}), http.StatusCreated)
	wantStatus(t, banker.do(http.MethodGet, api+"/contacts", nil), http.StatusForbidden)
	wantStatus(t, stranger.do(http.MethodGet, api+"/contacts", nil), http.StatusNotFound)
}

func deref[T any](p *T) T {
	var zero T
	if p == nil {
		return zero
	}
	return *p
}
