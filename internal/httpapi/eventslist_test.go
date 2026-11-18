package httpapi_test

import (
	"net/http"
	"net/url"
	"strings"
	"testing"
)

func TestListEvents(t *testing.T) {
	h := newHarness(t, options{})
	owner, banker := h.client(), h.client()
	owner.register()
	banker.register()
	api, estimates, create := estimateSetup(t, owner)
	e := create(map[string]any{})
	wantStatus(t, owner.do(http.MethodPatch, estimates+"/"+e.ID+"/send_estimate", map[string]any{"estimate_sending": map[string]any{"delivery_method": "Manual"}}), http.StatusOK)
	adminID := strings.TrimPrefix(api, "/api/v2/")
	invite(t, h, owner, banker, adminID, "banker@example.com", map[string]any{"user_type": "employee", "permissions": []string{"bank"}})

	type event struct {
		ID         string `json:"id"`
		EntityType string `json:"entity_type"`
		EntityID   string `json:"entity_id"`
		Action     string `json:"action"`
		CreatedAt  string `json:"created_at"`
	}
	events := api + "/events"
	all := decode[[]event](t, owner.do(http.MethodGet, events+"?entity_id="+e.ID, nil))
	if len(all) < 2 {
		t.Fatalf("events of the estimate = %+v, want it created and sent", all)
	}
	for i, ev := range all {
		if ev.ID == "" || ev.EntityType != "Estimate" || ev.EntityID != e.ID {
			t.Errorf("event %d = %+v", i, ev)
		}
		if i > 0 && ev.CreatedAt > all[i-1].CreatedAt {
			t.Errorf("events not newest first: %+v", all)
		}
	}

	// With a period every record type the user may see is listed: the estimate's events this month, none last year.
	month := decode[[]event](t, owner.do(http.MethodGet, events+"?period=this_month", nil))
	if len(month) < len(all) {
		t.Errorf("events this month = %+v, want at least the estimate's", month)
	}
	// Contacts, documents and time entries record their events too.
	doc := decode[struct{ ID string }](t, owner.do(http.MethodPost, api+"/documents/typeless_documents", map[string]any{"typeless_document": map[string]any{"reference": "Scan"}}))
	actions := map[string]bool{}
	for _, ev := range decode[[]event](t, owner.do(http.MethodGet, events+"?period=this_month", nil)) {
		actions[ev.EntityType+" "+ev.Action] = true
	}
	if !actions["Contact contact_created"] || !actions["Document document_saved"] {
		t.Errorf("events this month = %v, want the contact created and the document saved", actions)
	}
	if got := decode[[]event](t, owner.do(http.MethodGet, events+"?entity_id="+doc.ID, nil)); len(got) != 1 || got[0].EntityType != "Document" {
		t.Errorf("events of the document = %+v", got)
	}
	if old := decode[[]event](t, owner.do(http.MethodGet, events+"?period=200001..200012", nil)); len(old) != 0 {
		t.Errorf("events in 2000 = %+v", old)
	}
	for _, ev := range decode[[]event](t, banker.do(http.MethodGet, api+"/events?period=this_month", nil)) {
		if ev.EntityType == "Estimate" {
			t.Errorf("the banker sees %+v", ev)
		}
	}
	wantStatus(t, owner.do(http.MethodGet, events+"?period=someday", nil), http.StatusBadRequest)

	// One per page: the Link header leads through all of them, the last page has none.
	r := owner.do(http.MethodGet, events+"?entity_type=Estimate&per_page=1", nil)
	var paged []event
	for {
		wantStatus(t, r, http.StatusOK)
		page := decode[[]event](t, r)
		if len(page) != 1 {
			t.Fatalf("page = %+v, want one event", page)
		}
		paged = append(paged, page...)
		link := r.header.Get("Link")
		if link == "" {
			break
		}
		next, ok := strings.CutPrefix(strings.TrimSuffix(link, `>; rel="next"`), "<"+h.server.URL)
		if !ok || len(paged) > len(all) {
			t.Fatalf("link = %q", link)
		}
		r = owner.do(http.MethodGet, next, nil)
	}
	if len(paged) != len(all) || paged[0].ID != all[0].ID || paged[len(paged)-1].ID != all[len(all)-1].ID {
		t.Errorf("paged = %+v, want %+v", paged, all)
	}

	for query, want := range map[string]string{
		"":                       "entity_type, entity_id",
		"?entity_type=Receipt":   "Document",
		"?entity_type=Spaceship": "SalesInvoice",
		"?entity_type=Estimate&cursor=" + url.QueryEscape("not-ours"): "cursor",
	} {
		r := owner.do(http.MethodGet, events+query, nil)
		wantStatus(t, r, http.StatusBadRequest)
		if !strings.Contains(string(r.body), want) {
			t.Errorf("%q: body = %s, want it to name %s", query, r.body, want)
		}
	}

	// The bank user may not see estimates: asked by type it is refused, by record they are left out.
	wantStatus(t, banker.do(http.MethodGet, events+"?entity_type=Estimate", nil), http.StatusForbidden)
	if got := decode[[]event](t, banker.do(http.MethodGet, events+"?entity_id="+e.ID, nil)); len(got) != 0 {
		t.Errorf("bank user sees %+v", got)
	}
}
