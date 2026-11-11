package httpapi_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/adirkuhn/moneyharpy/internal/postgres/postgrestest"
)

type estimateResponse struct {
	ID           string  `json:"id"`
	EstimateID   *string `json:"estimate_id"`
	DraftID      *int    `json:"draft_id"`
	State        string  `json:"state"`
	EstimateDate *string `json:"estimate_date"`
	DueDate      string  `json:"due_date"`
	TotalIncl    string  `json:"total_price_incl_tax"`
	PreText      *string `json:"pre_text"`
	AcceptedAt   *string `json:"accepted_at"`
	URI          string  `json:"uri"`
	Contact      struct {
		ID string `json:"id"`
	} `json:"contact"`
	Details []struct {
		ID string `json:"id"`
	} `json:"details"`
	Notes []struct {
		ID string `json:"id"`
	} `json:"notes"`
}

type symbolicError struct {
	Error    string         `json:"error"`
	Symbolic map[string]any `json:"symbolic"`
}

// estimateSetup makes an administration with a customer, and a function
// that creates estimates for it.
func estimateSetup(t *testing.T, owner *client) (api, estimates string, create func(body map[string]any) estimateResponse) {
	t.Helper()
	a := owner.createAdministration("Harpy")
	api = "/api/v2/" + a.Id
	estimates = api + "/estimates"
	customer := decode[contact](t, owner.do(http.MethodPost, api+"/contacts", map[string]any{"contact": map[string]any{
		"company_name": "Example Shop B.V.", "send_estimates_to_email": "quotes@example.com",
	}}))
	vat21 := salesRate(t, owner, a.Id, "21")
	revenue := category(t, owner, a.Id, "Revenue")
	create = func(body map[string]any) estimateResponse {
		t.Helper()
		body["contact_id"] = customer.ID
		if _, ok := body["details_attributes"]; !ok {
			body["details_attributes"] = map[string]any{"0": map[string]any{"description": "Project X", "amount": "1 x", "price": "300", "tax_rate_id": vat21, "ledger_account_id": revenue}}
		}
		r := owner.do(http.MethodPost, estimates, map[string]any{"estimate": body})
		wantStatus(t, r, http.StatusCreated)
		return decode[estimateResponse](t, r)
	}
	return api, estimates, create
}

func TestEstimateLifecycle(t *testing.T) {
	h := newHarness(t, options{})
	owner := h.client()
	owner.register()
	api, estimates, create := estimateSetup(t, owner)

	e := create(map[string]any{"reference": "Project X", "pre_text": "Hello", "language": "en"})
	if e.State != "draft" || e.EstimateID != nil || e.DraftID == nil || e.TotalIncl != "363.0" || e.Contact.ID == "" || *e.PreText != "Hello" || e.URI != "/"+strings.Split(api, "/")[3]+"/estimates/"+e.ID {
		t.Fatalf("draft = %+v", e)
	}
	invalid := owner.do(http.MethodPost, estimates, map[string]any{"estimate": map[string]any{"language": "abracadabra"}})
	wantStatus(t, invalid, http.StatusUnprocessableEntity)
	if !strings.Contains(string(invalid.body), "contact_id") || !strings.Contains(string(invalid.body), "language") {
		t.Errorf("invalid = %s", invalid.body)
	}

	// A draft cannot be opened by hand; it is sent.
	r := owner.do(http.MethodPatch, estimates+"/"+e.ID+"/change_state", map[string]any{"state": "open"})
	wantStatus(t, r, http.StatusBadRequest)
	if s := decode[symbolicError](t, r); s.Symbolic["state"] != "can_not_change_draft_to_open" {
		t.Errorf("draft to open = %+v", s)
	}
	r = owner.do(http.MethodPatch, estimates+"/"+e.ID+"/change_state.json", map[string]any{"state": "lost"})
	wantStatus(t, r, http.StatusBadRequest)
	if s := decode[symbolicError](t, r); s.Symbolic["state"] != "in" {
		t.Errorf("unknown state = %+v", s)
	}

	// Email needs a sender address.
	sending := map[string]any{"estimate_sending": map[string]any{"delivery_method": "Email"}}
	noSender := owner.do(http.MethodPatch, estimates+"/"+e.ID+"/send_estimate", sending)
	wantStatus(t, noSender, http.StatusUnprocessableEntity)
	if !strings.Contains(string(noSender.body), "sender address") {
		t.Errorf("no sender = %s", noSender.body)
	}
	wantStatus(t, owner.do(http.MethodPatch, api+"/identities/default", map[string]any{"identity": map[string]any{"email": "books@example.com"}}), http.StatusOK)
	sent := owner.do(http.MethodPatch, estimates+"/"+e.ID+"/send_estimate", sending)
	wantStatus(t, sent, http.StatusOK)
	e = decode[estimateResponse](t, sent)
	if e.State != "open" || e.EstimateID == nil || !strings.HasSuffix(*e.EstimateID, "0001") || e.EstimateDate == nil || e.DraftID != nil {
		t.Fatalf("sent = %+v", e)
	}
	h.mailer.mu.Lock()
	mails := h.mailer.documents
	h.mailer.mu.Unlock()
	if len(mails) != 1 || mails[0].To[0] != "quotes@example.com" || len(mails[0].Files) != 1 || !bytes.HasPrefix(mails[0].Files[0].Data, []byte("%PDF-")) ||
		!strings.HasPrefix(mails[0].Files[0].Name, "estimate-") {
		t.Fatalf("mails = %+v", mails)
	}
	wantStatus(t, owner.do(http.MethodPatch, estimates+"/"+e.ID+"/send_estimate", map[string]any{"estimate_sending": map[string]any{"delivery_method": "Post"}}), http.StatusBadRequest)

	byNumber := decode[estimateResponse](t, owner.do(http.MethodGet, estimates+"/find_by_estimate_id/"+*e.EstimateID, nil))
	if byNumber.ID != e.ID {
		t.Errorf("by number = %s", byNumber.ID)
	}
	missing := owner.do(http.MethodGet, estimates+"/find_by_estimate_id/nothing", nil)
	wantStatus(t, missing, http.StatusNotFound)

	pdf := owner.follow(owner.do(http.MethodGet, estimates+"/"+e.ID+"/download_pdf", nil))
	if !bytes.HasPrefix(pdf.body, []byte("%PDF-")) || !strings.Contains(pdf.header.Get("Content-Disposition"), "estimate-") {
		t.Errorf("pdf = %d bytes, %s", len(pdf.body), pdf.header.Get("Content-Disposition"))
	}
	owner.follow(owner.do(http.MethodGet, estimates+"/"+e.ID+"/download_pdf.json?media=stationery", nil))
	wantStatus(t, owner.do(http.MethodGet, estimates+"/"+e.ID+"/download_signed_pdf", nil), http.StatusUnprocessableEntity)
	wantStatus(t, owner.do(http.MethodGet, estimates+"/1/download_signed_pdf", nil), http.StatusNotFound)

	wantStatus(t, owner.do(http.MethodPost, estimates+"/"+e.ID+"/notes", map[string]any{"note": map[string]any{"note": "Called"}}), http.StatusCreated)
	att := owner.attach(estimates+"/"+e.ID+"/attachments", "brief.pdf", "application/pdf", []byte("%PDF-1.4 brief"))
	wantStatus(t, att, http.StatusOK)
	attID := decode[struct {
		ID string `json:"id"`
	}](t, att).ID
	file := owner.follow(owner.do(http.MethodGet, estimates+"/"+e.ID+"/attachments/"+attID+"/download", nil))
	if string(file.body) != "%PDF-1.4 brief" {
		t.Errorf("attachment = %q", file.body)
	}
	wantStatus(t, owner.do(http.MethodDelete, estimates+"/"+e.ID+"/attachments/"+attID, nil), http.StatusNoContent)
	withNote := decode[estimateResponse](t, owner.do(http.MethodGet, estimates+"/"+e.ID, nil))
	if len(withNote.Notes) != 1 {
		t.Fatalf("notes = %+v", withNote.Notes)
	}
	wantStatus(t, owner.do(http.MethodDelete, estimates+"/"+e.ID+"/notes/"+withNote.Notes[0].ID, nil), http.StatusNoContent)

	accepted := decode[estimateResponse](t, owner.do(http.MethodPatch, estimates+"/"+e.ID+"/change_state", map[string]any{"state": "accepted"}))
	if accepted.State != "accepted" || accepted.AcceptedAt == nil {
		t.Errorf("accepted = %+v", accepted)
	}
	billed := owner.do(http.MethodPatch, estimates+"/"+e.ID+"/bill_estimate", map[string]any{})
	wantStatus(t, billed, http.StatusOK)
	inv := decode[struct {
		ID                 string  `json:"id"`
		State              string  `json:"state"`
		OriginalEstimateID *string `json:"original_estimate_id"`
		TotalIncl          string  `json:"total_price_incl_tax"`
	}](t, billed)
	if inv.State != "draft" || inv.OriginalEstimateID == nil || *inv.OriginalEstimateID != e.ID || inv.TotalIncl != "363.0" {
		t.Errorf("invoice = %+v", inv)
	}
	again := owner.do(http.MethodPatch, estimates+"/"+e.ID+"/bill_estimate", map[string]any{})
	wantStatus(t, again, http.StatusBadRequest)
	if s := decode[symbolicError](t, again); s.Symbolic["id"] != "state" {
		t.Errorf("bill again = %+v", s)
	}
	// A billed estimate does not change, but can be archived.
	wantStatus(t, owner.do(http.MethodPatch, estimates+"/"+e.ID, map[string]any{"estimate": map[string]any{"reference": "Other"}}), http.StatusUnprocessableEntity)
	wantStatus(t, owner.do(http.MethodPatch, estimates+"/"+e.ID+"/change_state", map[string]any{"state": "archived"}), http.StatusOK)

	// Lists, filters and synchronization.
	draft := create(map[string]any{"details_attributes": []any{}})
	empty := owner.do(http.MethodPatch, estimates+"/"+draft.ID+"/send_estimate", map[string]any{})
	wantStatus(t, empty, http.StatusBadRequest)
	if s := decode[symbolicError](t, empty); s.Symbolic["estimate"] != "details_cannot_be_empty" {
		t.Errorf("empty = %+v", s)
	}
	updated := owner.do(http.MethodPatch, estimates+"/"+draft.ID+".json", map[string]any{"estimate": map[string]any{
		"details_attributes": map[string]any{"0": map[string]any{"description": "More", "price": 99, "amount": 1, "tax_rate_id": salesRate(t, owner, strings.Split(api, "/")[3], "21")}},
	}})
	wantStatus(t, updated, http.StatusOK)
	if got := decode[estimateResponse](t, updated); len(got.Details) != 1 {
		t.Errorf("updated = %+v", got)
	}
	drafts := owner.do(http.MethodGet, estimates+"?filter=state:draft&per_page=1", nil)
	wantStatus(t, drafts, http.StatusOK)
	if drafts.header.Get("X-Total-Count") != "1" || len(decode[[]estimateResponse](t, drafts)) != 1 {
		t.Errorf("drafts = %s", drafts.header.Get("X-Total-Count"))
	}
	if all := decode[[]estimateResponse](t, owner.do(http.MethodGet, estimates, nil)); len(all) != 2 {
		t.Errorf("all = %d", len(all))
	}
	wantStatus(t, owner.do(http.MethodGet, estimates+"?filter=state:paid", nil), http.StatusBadRequest)
	versions := decode[[]struct {
		ID string `json:"id"`
	}](t, owner.do(http.MethodGet, estimates+"/synchronization?filter=state:archived", nil))
	if len(versions) != 1 || versions[0].ID != e.ID {
		t.Errorf("versions = %+v", versions)
	}
	fetched := decode[[]estimateResponse](t, owner.do(http.MethodPost, estimates+"/synchronization", map[string]any{"ids": []any{e.ID, draft.ID}}))
	if len(fetched) != 2 {
		t.Errorf("fetched = %d", len(fetched))
	}

	// The contact is in use; deleting estimates leaves the invoice.
	wantKept(t, owner.do(http.MethodDelete, api+"/contacts/"+e.Contact.ID, nil), "archived")
	wantStatus(t, owner.do(http.MethodDelete, estimates+"/"+e.ID, nil), http.StatusNoContent)
	wantStatus(t, owner.do(http.MethodGet, estimates+"/"+e.ID, nil), http.StatusNotFound)
	wantStatus(t, owner.do(http.MethodGet, api+"/sales_invoices/"+inv.ID, nil), http.StatusOK)
}

func TestEstimatesFollowPermissions(t *testing.T) {
	h := newHarness(t, options{})
	owner, banker, quoter := h.client(), h.client(), h.client()
	owner.register()
	banker.register()
	quoter.register()
	_, estimates, create := estimateSetup(t, owner)
	admin := strings.Split(estimates, "/")[3]
	invite(t, h, owner, banker, admin, "banker@example.com", map[string]any{"user_type": "employee", "permissions": []string{"bank"}})
	invite(t, h, owner, quoter, admin, "quoter@example.com", map[string]any{"user_type": "employee", "permissions": []string{"estimates"}})
	wantStatus(t, banker.do(http.MethodGet, estimates, nil), http.StatusForbidden)
	e := create(map[string]any{})
	wantStatus(t, quoter.do(http.MethodPatch, estimates+"/"+e.ID+"/send_estimate", map[string]any{"estimate_sending": map[string]any{"delivery_method": "Manual"}}), http.StatusOK)
	// Billing makes an invoice, which needs that permission too.
	wantStatus(t, quoter.do(http.MethodPatch, estimates+"/"+e.ID+"/bill_estimate", map[string]any{}), http.StatusForbidden)
	wantStatus(t, owner.do(http.MethodPatch, estimates+"/"+e.ID+"/bill_estimate", map[string]any{}), http.StatusOK)
}

func TestEstimateWebhooks(t *testing.T) {
	ctx := context.Background()
	pool, cleanup, err := postgrestest.Start(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()
	h := newHarness(t, options{pool: pool})
	owner := h.client()
	owner.register()
	api, estimates, create := estimateSetup(t, owner)
	rc := newReceiver(t)
	wantStatus(t, owner.do(http.MethodPatch, "/api/instance/settings", map[string]bool{"webhooks_allow_local": true}), http.StatusOK)
	wantStatus(t, owner.do(http.MethodPost, api+"/webhooks", map[string]any{"url": rc.srv.URL, "enabled_events": []string{"estimate", "sales_invoice_created_based_on_estimate"}}), http.StatusCreated)

	e := create(map[string]any{})
	wantStatus(t, owner.do(http.MethodPatch, estimates+"/"+e.ID, map[string]any{"estimate": map[string]any{"reference": "Project Y"}}), http.StatusOK)
	wantStatus(t, owner.do(http.MethodPatch, estimates+"/"+e.ID+"/send_estimate", map[string]any{"estimate_sending": map[string]any{"delivery_method": "Manual"}}), http.StatusOK)
	wantStatus(t, owner.do(http.MethodPatch, estimates+"/"+e.ID+"/change_state", map[string]any{"state": "accepted"}), http.StatusOK)
	wantStatus(t, owner.do(http.MethodPatch, estimates+"/"+e.ID+"/bill_estimate", map[string]any{}), http.StatusOK)
	wantStatus(t, owner.do(http.MethodDelete, estimates+"/"+e.ID, nil), http.StatusNoContent)
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
	want := []string{"estimate_billed", "estimate_created", "estimate_destroyed", "estimate_mark_accepted", "estimate_send_manually",
		"estimate_updated", "sales_invoice_created_based_on_estimate"}
	if !slices.Equal(actions, want) {
		t.Errorf("actions = %v, want %v", actions, want)
	}
}
