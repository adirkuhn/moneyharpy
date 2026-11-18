package sales

import (
	"context"
	"slices"
	"testing"

	"github.com/adirkuhn/moneyharpy/internal/resource"
)

// recurringActions lists what happened to a recurring invoice, oldest first.
func recurringActions(t *testing.T, h harness, id resource.ID) []string {
	t.Helper()
	events, err := h.svc.history.Of(context.Background(), adminID, "RecurringSalesInvoice", []resource.ID{id})
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, e := range events[id] {
		out = append(out, e.Action)
	}
	return out
}

func TestRecurringInvoiceEventsTellItsStory(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	r := recurring(t, h, "2026-11-01", RecurringChanges{})
	if _, err := h.svc.UpdateRecurring(ctx, adminID, r.ID, owner, RecurringChanges{Reference: ptr("Hosting")}); err != nil {
		t.Fatal(err)
	}
	if err := h.svc.Work(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := h.svc.DeactivateRecurring(ctx, adminID, r.ID, owner); err != nil {
		t.Fatal(err)
	}
	want := []string{"recurring_sales_invoice_created", "recurring_sales_invoice_updated", "recurring_sales_invoice_invoice_created", "recurring_sales_invoice_deactivated"}
	if got := recurringActions(t, h, r.ID); !slices.Equal(got, want) {
		t.Errorf("events = %v, want %v", got, want)
	}

	gone := recurring(t, h, "2026-12-01", RecurringChanges{})
	if _, err := h.svc.DeleteRecurring(ctx, adminID, gone.ID, owner); err != nil {
		t.Fatal(err)
	}
	if got := recurringActions(t, h, gone.ID); !slices.Equal(got, []string{"recurring_sales_invoice_created", "recurring_sales_invoice_destroyed"}) {
		t.Errorf("deleted one's events = %v", got)
	}
}
