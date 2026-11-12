package invoicing

import (
	"context"
	"strings"
	"testing"
)

func TestWorkflowPaymentConditions(t *testing.T) {
	svc, _, _ := newTestService()
	ctx := context.Background()
	got, err := svc.Workflows(ctx, adminID, owner)
	if err != nil {
		t.Fatal(err)
	}
	estimate, invoice := got[0], got[1]
	if invoice.PaymentConditions != DefaultPaymentConditions("nl") || estimate.PaymentConditions != "" {
		t.Errorf("default conditions = %q / %q", invoice.PaymentConditions, estimate.PaymentConditions)
	}
	if !strings.HasPrefix(DefaultPaymentConditions("nl-be"), "We verzoeken u vriendelijk") || !strings.HasPrefix(DefaultPaymentConditions("en"), "We kindly request") ||
		!strings.Contains(DefaultPaymentConditions("en"), "{document.payment_reference}") {
		t.Errorf("DefaultPaymentConditions() = %q / %q", DefaultPaymentConditions("nl-be"), DefaultPaymentConditions("en"))
	}

	w, err := svc.CreateWorkflow(ctx, adminID, owner, InvoiceWorkflow, WorkflowChanges{Name: ptr("Export"), Language: ptr("en")})
	if err != nil {
		t.Fatal(err)
	}
	if w.PaymentConditions != DefaultPaymentConditions("en") {
		t.Errorf("English workflow conditions = %q", w.PaymentConditions)
	}
	w, err = svc.UpdateWorkflow(ctx, adminID, w.ID, owner, WorkflowChanges{PaymentConditions: ptr("  Pay *now*.  ")})
	if err != nil || w.PaymentConditions != "Pay *now*." {
		t.Errorf("UpdateWorkflow() = %q, %v", w.PaymentConditions, err)
	}
	if _, err := svc.UpdateWorkflow(ctx, adminID, w.ID, owner, WorkflowChanges{PaymentConditions: ptr(strings.Repeat("x", 1001))}); fieldsOf(t, err)["payment_conditions"] == nil {
		t.Error("too long conditions were accepted")
	}
	own, err := svc.CreateWorkflow(ctx, adminID, owner, InvoiceWorkflow, WorkflowChanges{Name: ptr("Own"), Language: ptr("en"), PaymentConditions: ptr("Within a week.")})
	if err != nil || own.PaymentConditions != "Within a week." {
		t.Errorf("CreateWorkflow(own conditions) = %q, %v", own.PaymentConditions, err)
	}
	e, err := svc.CreateWorkflow(ctx, adminID, owner, EstimateWorkflow, WorkflowChanges{Name: ptr("Quotes"), PaymentConditions: ptr("Ignored")})
	if err != nil || e.PaymentConditions != "" {
		t.Errorf("estimate workflow conditions = %q, %v", e.PaymentConditions, err)
	}
}
