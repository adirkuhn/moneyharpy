package sales

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/adirkuhn/moneyharpy/internal/resource"
)

// estimateActions lists what happened to an estimate, oldest first.
func estimateActions(t *testing.T, h harness, id resource.ID) []string {
	t.Helper()
	e, err := h.svc.Estimate(context.Background(), adminID, id, owner)
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, ev := range e.Events {
		out = append(out, ev.Action)
	}
	return out
}

func TestEstimateEventsTellItsStory(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	e := sentEstimate(t, h)
	if _, err := h.svc.ChangeEstimateState(ctx, adminID, e.ID, owner, Accepted); err != nil {
		t.Fatal(err)
	}
	inv, err := h.svc.BillEstimate(ctx, adminID, e.ID, owner)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"estimate_created", "estimate_send_email", "estimate_mark_accepted", "estimate_billed"}
	if got := estimateActions(t, h, e.ID); !slices.Equal(got, want) {
		t.Fatalf("events = %v, want %v", got, want)
	}
	got, _ := h.svc.Estimate(ctx, adminID, e.ID, owner)
	billed := got.Events[3]
	if billed.LinkEntityType != EntityType || billed.LinkEntityID == nil || *billed.LinkEntityID != inv.ID || billed.UserID != owner {
		t.Errorf("billed = %+v", billed)
	}
	if got.Events[0].EntityType != EstimateEntityType {
		t.Errorf("created = %+v", got.Events[0])
	}
	manual := estimate(t, h, EstimateChanges{})
	if _, err := h.svc.SendEstimate(ctx, adminID, manual.ID, owner, EstimateSendInput{DeliveryMethod: "Manual"}); err != nil {
		t.Fatal(err)
	}
	if got := estimateActions(t, h, manual.ID); !slices.Equal(got, []string{"estimate_created", "estimate_send_manually"}) {
		t.Errorf("manual events = %v", got)
	}
}

func TestDuplicateEstimateNamesItsOriginal(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	orig := sentEstimate(t, h)
	copied := estimate(t, h, EstimateChanges{OriginalEstimateID: ptr(orig.ID)})
	if copied.OriginalEstimateID == nil || *copied.OriginalEstimateID != orig.ID {
		t.Errorf("original = %v", copied.OriginalEstimateID)
	}
	if got := estimateActions(t, h, copied.ID); !slices.Equal(got, []string{"estimate_created_from_original"}) {
		t.Fatalf("events = %v", got)
	}
	got, _ := h.svc.Estimate(ctx, adminID, copied.ID, owner)
	if ev := got.Events[0]; ev.Data["original_estimate"] != orig.EstimateID || ev.LinkEntityType != EstimateEntityType || *ev.LinkEntityID != orig.ID {
		t.Errorf("event = %+v", ev)
	}
	_, err := h.svc.CreateEstimate(ctx, adminID, owner, EstimateChanges{ContactID: ptr(customer), OriginalEstimateID: ptr(resource.ID(9))})
	var verr *resource.ValidationError
	if !errors.As(err, &verr) || len(verr.Fields["original_estimate_id"]) == 0 {
		t.Errorf("unknown original = %v", err)
	}
}

func TestNextEstimateDraftID(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	n, err := h.svc.NextEstimateDraftID(ctx, adminID, owner)
	if err != nil || n != 1 {
		t.Fatalf("next = %d, %v", n, err)
	}
	if again, _ := h.svc.NextEstimateDraftID(ctx, adminID, owner); again != 1 {
		t.Errorf("asking took the number: %d", again)
	}
	if e := estimate(t, h, EstimateChanges{}); e.DraftID != 1 {
		t.Errorf("draft id = %d", e.DraftID)
	}
	if n, _ := h.svc.NextEstimateDraftID(ctx, adminID, owner); n != 2 {
		t.Errorf("next after one = %d", n)
	}
	if _, err := h.svc.NextEstimateDraftID(ctx, adminID, stranger); err == nil {
		t.Error("a stranger asked")
	}
}
