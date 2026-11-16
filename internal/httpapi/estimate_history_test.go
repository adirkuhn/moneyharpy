package httpapi_test

import (
	"net/http"
	"slices"
	"testing"

	"github.com/adirkuhn/moneyharpy/internal/httpapi"
)

func TestEstimateHistoryDraftNumberAndDuplicate(t *testing.T) {
	h := newHarness(t, options{})
	owner := h.client()
	owner.register()
	_, estimates, create := estimateSetup(t, owner)

	if next := decode[httpapi.NextDraftId](t, owner.do(http.MethodGet, estimates+"/next_draft_id", nil)); next.DraftId != 1 {
		t.Errorf("next draft id = %d", next.DraftId)
	}
	e := create(map[string]any{})
	if next := decode[httpapi.NextDraftId](t, owner.do(http.MethodGet, estimates+"/next_draft_id", nil)); next.DraftId != 2 {
		t.Errorf("next draft id after one = %d", next.DraftId)
	}
	wantStatus(t, owner.do(http.MethodPatch, estimates+"/"+e.ID+"/send_estimate", map[string]any{"estimate_sending": map[string]any{"delivery_method": "Manual"}}), http.StatusOK)
	billed := owner.do(http.MethodPatch, estimates+"/"+e.ID+"/bill_estimate", map[string]any{})
	wantStatus(t, billed, http.StatusOK)
	invoiceID := decode[struct {
		ID string `json:"id"`
	}](t, billed).ID

	type event struct {
		Action         string         `json:"action"`
		LinkEntityID   *string        `json:"link_entity_id"`
		LinkEntityType *string        `json:"link_entity_type"`
		Data           map[string]any `json:"data"`
	}
	type withEvents struct {
		OriginalEstimateID *string `json:"original_estimate_id"`
		Events             []event `json:"events"`
	}
	got := decode[withEvents](t, owner.do(http.MethodGet, estimates+"/"+e.ID, nil))
	var actions []string
	for _, ev := range got.Events {
		actions = append(actions, ev.Action)
	}
	if !slices.Equal(actions, []string{"estimate_created", "estimate_send_manually", "estimate_billed"}) {
		t.Fatalf("actions = %v", actions)
	}
	if last := got.Events[2]; last.LinkEntityID == nil || *last.LinkEntityID != invoiceID || *last.LinkEntityType != "SalesInvoice" {
		t.Errorf("billed event = %+v", last)
	}

	copied := create(map[string]any{"original_estimate_id": e.ID})
	again := decode[withEvents](t, owner.do(http.MethodGet, estimates+"/"+copied.ID, nil))
	if again.OriginalEstimateID == nil || *again.OriginalEstimateID != e.ID || len(again.Events) != 1 || again.Events[0].Action != "estimate_created_from_original" {
		t.Errorf("copy = %+v", again)
	}
	unknown := owner.do(http.MethodPost, estimates, map[string]any{"estimate": map[string]any{"contact_id": copied.Contact.ID, "original_estimate_id": "9"}})
	wantStatus(t, unknown, http.StatusUnprocessableEntity)
}
