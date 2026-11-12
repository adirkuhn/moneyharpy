package sales

import (
	"context"
	"testing"
)

func TestNewInvoicesTakeTheWorkflowsPaymentConditions(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	inv := draft(t, h)
	if inv.PaymentConditions != "Betaal {document.total_price} voor {document.due_date}." {
		t.Errorf("conditions = %q, want the default workflow's", inv.PaymentConditions)
	}
	// Another workflow brings its own text, as long as nobody changed it.
	moved, err := h.svc.Update(ctx, adminID, inv.ID, owner, Changes{WorkflowID: ptr(exportFlow)})
	if err != nil || moved.PaymentConditions != "Pay within 30 days." {
		t.Errorf("after the workflow changed = %q, %v", moved.PaymentConditions, err)
	}
	own, err := h.svc.Update(ctx, adminID, inv.ID, owner, Changes{PaymentConditions: ptr("Cash only.")})
	if err != nil {
		t.Fatal(err)
	}
	if back, _ := h.svc.Update(ctx, adminID, own.ID, owner, Changes{WorkflowID: ptr(flow)}); back.PaymentConditions != "Cash only." {
		t.Errorf("own text was replaced by %q", back.PaymentConditions)
	}
	given, err := h.svc.Create(ctx, adminID, owner, Changes{ContactID: ptr(customer), PaymentConditions: ptr("Within a week."), WorkflowID: ptr(exportFlow)})
	if err != nil || given.PaymentConditions != "Within a week." {
		t.Errorf("explicit conditions = %q, %v", given.PaymentConditions, err)
	}
}

func TestSendGivesAnRFPaymentReference(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	inv := draft(t, h, line("100", "1", vat21, revenue))
	if inv.PaymentReference != "" {
		t.Errorf("draft reference = %q, want none", inv.PaymentReference)
	}
	sent, err := h.svc.Send(ctx, adminID, inv.ID, owner, SendInput{DeliveryMethod: "Manual"})
	if err != nil {
		t.Fatal(err)
	}
	if !ValidRF(sent.PaymentReference) || len(sent.PaymentReference) != 25 {
		t.Fatalf("reference = %q", sent.PaymentReference)
	}
	again, err := h.svc.Send(ctx, adminID, inv.ID, owner, SendInput{DeliveryMethod: "Manual"})
	if err != nil || again.PaymentReference != sent.PaymentReference {
		t.Errorf("resent reference = %q, want %q (%v)", again.PaymentReference, sent.PaymentReference, err)
	}
	other := draft(t, h, line("10", "1", vat21, revenue))
	next, err := h.svc.Send(ctx, adminID, other.ID, owner, SendInput{DeliveryMethod: "Manual"})
	if err != nil || next.PaymentReference == sent.PaymentReference || !ValidRF(next.PaymentReference) {
		t.Errorf("second reference = %q (%v)", next.PaymentReference, err)
	}
}
