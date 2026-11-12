package sales

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/adirkuhn/moneyharpy/internal/history"
	"github.com/adirkuhn/moneyharpy/internal/resource"
)

// actions lists what happened to an invoice, oldest first.
func actions(t *testing.T, h harness, id resource.ID) []string {
	t.Helper()
	inv, err := h.svc.Invoice(context.Background(), adminID, id, owner)
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, e := range inv.Events {
		out = append(out, e.Action)
	}
	return out
}

func eventOf(t *testing.T, h harness, id resource.ID, action string) history.Event {
	t.Helper()
	inv, err := h.svc.Invoice(context.Background(), adminID, id, owner)
	if err != nil {
		t.Fatal(err)
	}
	i := slices.IndexFunc(inv.Events, func(e history.Event) bool { return e.Action == action })
	if i < 0 {
		t.Fatalf("no %s in %v", action, actions(t, h, id))
	}
	return inv.Events[i]
}

func TestEventsTellHowAnInvoiceWasMadeAndSent(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	inv := draft(t, h, line("100", "1", vat21, revenue))
	if _, err := h.svc.Send(ctx, adminID, inv.ID, owner, SendInput{EmailAddress: "ann@example.com", EmailMessage: "Hello"}); err != nil {
		t.Fatal(err)
	}
	if got, want := actions(t, h, inv.ID), []string{"sales_invoice_created", "sales_invoice_state_changed_to_open", "sales_invoice_send_email"}; !slices.Equal(got, want) {
		t.Errorf("events = %v, want %v", got, want)
	}
	created := eventOf(t, h, inv.ID, "sales_invoice_created")
	if created.UserID != owner || created.EntityType != EntityType || created.AdministrationID != adminID {
		t.Errorf("created = %+v", created)
	}
	if mail := eventOf(t, h, inv.ID, "sales_invoice_send_email"); mail.Data["email_address"] != "ann@example.com" || mail.Data["email_message"] != "Hello" {
		t.Errorf("email data = %v", mail.Data)
	}
	if _, err := h.svc.Send(ctx, adminID, inv.ID, owner, SendInput{DeliveryMethod: "Manual"}); err != nil {
		t.Fatal(err)
	}
	if got := actions(t, h, inv.ID); !slices.Equal(got[3:], []string{"sales_invoice_send_manually"}) {
		t.Errorf("sent again = %v", got)
	}

	later := draft(t, h, line("10", "1", vat21, revenue))
	if _, err := h.svc.Send(ctx, adminID, later.ID, owner, SendInput{DeliveryMethod: "Manual", Scheduled: true, InvoiceDate: "2026-12-01"}); err != nil {
		t.Fatal(err)
	}
	if got := actions(t, h, later.ID); !slices.Equal(got, []string{"sales_invoice_created", "sales_invoice_state_changed_to_scheduled"}) {
		t.Errorf("scheduled = %v", got)
	}
}

func TestEventsTellWhatAnUpdateOfASentInvoiceChanged(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	inv := draft(t, h, line("100", "1", vat21, revenue))
	// A draft is still being written: its edits are not history.
	if _, err := h.svc.Update(ctx, adminID, inv.ID, owner, Changes{Reference: ptr("Order 1"), PaymentConditions: ptr("Soon")}); err != nil {
		t.Fatal(err)
	}
	if got := actions(t, h, inv.ID); !slices.Equal(got, []string{"sales_invoice_created"}) {
		t.Errorf("draft events = %v", got)
	}
	if _, err := h.svc.Send(ctx, adminID, inv.ID, owner, SendInput{DeliveryMethod: "Manual"}); err != nil {
		t.Fatal(err)
	}
	if _, err := h.svc.Update(ctx, adminID, inv.ID, owner, Changes{Reference: ptr("Order 2")}); err != nil {
		t.Fatal(err)
	}
	updated := eventOf(t, h, inv.ID, "sales_invoice_updated")
	if change, _ := updated.Data["changes"].(map[string]any); !slices.Equal(change["reference"].([]any), []any{"Order 1", "Order 2"}) {
		t.Errorf("changes = %v", updated.Data)
	}
	// Saving without a change tells nothing.
	if _, err := h.svc.Update(ctx, adminID, inv.ID, owner, Changes{Reference: ptr("Order 2")}); err != nil {
		t.Fatal(err)
	}
	if got := actions(t, h, inv.ID); len(got) != 4 {
		t.Errorf("events = %v", got)
	}
}

func TestEventsTellAboutPaymentsAndTheirStates(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	inv := sent(t, h, "2026-11-01")
	p, err := h.svc.RegisterPayment(ctx, adminID, inv.ID, owner, PaymentInput{PaymentDate: "2026-11-05"})
	if err != nil {
		t.Fatal(err)
	}
	registered := eventOf(t, h, inv.ID, "payment_registered")
	if registered.LinkEntityType != "Payment" || registered.LinkEntityID == nil || *registered.LinkEntityID != p.ID {
		t.Errorf("registered = %+v", registered)
	}
	*h.clock = h.clock.Add(time.Minute)
	if err := h.svc.DeletePayment(ctx, adminID, inv.ID, p.ID, owner); err != nil {
		t.Fatal(err)
	}
	want := []string{
		"sales_invoice_created", "sales_invoice_state_changed_to_open", "sales_invoice_send_manually",
		"payment_registered", "sales_invoice_state_changed_to_paid", "payment_destroyed", "sales_invoice_state_changed_to_open",
	}
	if got := actions(t, h, inv.ID); !slices.Equal(got, want) {
		t.Errorf("events = %v\nwant %v", got, want)
	}
}

func TestEventsTellAboutRemindersWriteOffsAndCredit(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	inv := sent(t, h, "2026-11-01")
	if _, err := h.svc.Pause(ctx, adminID, inv.ID, owner, true); err != nil {
		t.Fatal(err)
	}
	if _, err := h.svc.Pause(ctx, adminID, inv.ID, owner, false); err != nil {
		t.Fatal(err)
	}
	if _, err := h.svc.MarkDubious(ctx, adminID, inv.ID, owner, ""); err != nil {
		t.Fatal(err)
	}
	*h.clock = now.AddDate(0, 0, 1)
	if _, err := h.svc.MarkUncollectible(ctx, adminID, inv.ID, owner, "", ""); err != nil {
		t.Fatal(err)
	}
	got := actions(t, h, inv.ID)
	for _, want := range []string{"sales_invoice_paused", "sales_invoice_unpaused", "sales_invoice_marked_as_dubious", "sales_invoice_state_changed_to_uncollectible", "sales_invoice_marked_as_uncollectible"} {
		if !slices.Contains(got, want) {
			t.Errorf("no %s in %v", want, got)
		}
	}

	other := sent(t, h, "2026-11-02")
	credit, err := h.svc.Credit(ctx, adminID, other.ID, owner, nil)
	if err != nil {
		t.Fatal(err)
	}
	if e := eventOf(t, h, credit.ID, "credit_invoice_created_from_original"); e.Data["original_invoice"] != other.InvoiceID {
		t.Errorf("credit event = %+v", e)
	}
	// As at Moneybird the credit invoice records only where it came from;
	// Moneybird has no event for the original.
	if got := actions(t, h, credit.ID); !slices.Equal(got, []string{"credit_invoice_created_from_original"}) {
		t.Errorf("credit events = %v", got)
	}
	if got := actions(t, h, other.ID); slices.ContainsFunc(got, func(a string) bool { return strings.HasPrefix(a, "credit_invoice") }) {
		t.Errorf("original's events = %v", got)
	}
}

func TestEventsOfAnInvoiceBilledFromAnEstimate(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	e := sentEstimate(t, h)
	inv, err := h.svc.BillEstimate(ctx, adminID, e.ID, owner)
	if err != nil {
		t.Fatal(err)
	}
	if ev := eventOf(t, h, inv.ID, "sales_invoice_created_based_on_estimate"); ev.Data["original_estimate"] != e.EstimateID {
		t.Errorf("estimate event = %+v", ev)
	}
}

func TestEditingAPaidInvoiceReopensItWhenItAsksMore(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	inv := sent(t, h, "2026-11-01")
	if _, err := h.svc.RegisterPayment(ctx, adminID, inv.ID, owner, PaymentInput{PaymentDate: "2026-11-05"}); err != nil {
		t.Fatal(err)
	}
	updated, err := h.svc.Update(ctx, adminID, inv.ID, owner, Changes{Details: []DetailInput{{ID: &inv.Lines[0].ID, Price: ptr("200")}}})
	if err != nil {
		t.Fatalf("edit paid invoice: %v", err)
	}
	if updated.State != Open || updated.PaidAt != nil || updated.Unpaid() != 12100 {
		t.Errorf("after edit = %s, unpaid %s", updated.State, updated.Unpaid())
	}
	if s := sums(h.store.entries[inv.ID]); s[debtors] != 24200 {
		t.Errorf("rebooked debtors = %v", s)
	}
	got := actions(t, h, inv.ID)
	if !slices.Contains(got, "sales_invoice_updated") || got[len(got)-1] != "sales_invoice_state_changed_to_open" {
		t.Errorf("events = %v", got)
	}
	// Back to what was paid, the invoice is paid again.
	back, err := h.svc.Update(ctx, adminID, inv.ID, owner, Changes{Details: []DetailInput{{ID: &inv.Lines[0].ID, Price: ptr("100")}}})
	if err != nil {
		t.Fatal(err)
	}
	if back.State != Paid || back.PaidAt == nil {
		t.Errorf("paid again = %s", back.State)
	}
}

// Moneybird's DELETE refuses an invoice with pending payments.
func TestAnInvoiceWithPendingPaymentsStays(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	inv := sent(t, h, "2026-11-01")
	stored := h.store.invoices[inv.ID]
	stored.State = PendingPayment
	h.store.invoices[inv.ID] = stored
	if err := h.svc.Delete(ctx, adminID, inv.ID, owner); fieldsOf(t, err)["base"] == nil {
		t.Errorf("deleted with pending payments: %v", err)
	}
}

func TestDeletingASentInvoiceRemovesWhatItBooked(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	inv := sent(t, h, "2026-11-01")
	paid := sent(t, h, "2026-11-01")
	payment, err := h.svc.RegisterPayment(ctx, adminID, paid.ID, owner, PaymentInput{PaymentDate: "2026-11-05"})
	if err != nil {
		t.Fatal(err)
	}
	// Moneybird deletes an invoice with its payments; only pending ones stop it.
	if err := h.svc.Delete(ctx, adminID, paid.ID, owner); err != nil {
		t.Fatalf("deleting a paid invoice: %v", err)
	}
	if _, err := h.svc.Invoice(ctx, adminID, paid.ID, owner); !errors.Is(err, ErrNotFound) {
		t.Errorf("paid invoice deleted = %v", err)
	}
	if h.store.entries[paid.ID] != nil {
		t.Error("the paid invoice's bookings stay")
	}
	if _, ok := h.store.paid[payment.ID]; ok {
		t.Error("the payment's bookings stay")
	}
	h.ledger.locked = "2026-11-15"
	if err := h.svc.Delete(ctx, adminID, inv.ID, owner); fieldsOf(t, err)["invoice_date"] == nil {
		t.Errorf("deleted in the locked period: %v", err)
	}
	h.ledger.locked = ""
	if err := h.svc.Delete(ctx, adminID, inv.ID, owner); err != nil {
		t.Fatal(err)
	}
	if _, err := h.svc.Invoice(ctx, adminID, inv.ID, owner); !errors.Is(err, ErrNotFound) {
		t.Errorf("deleted = %v", err)
	}
	if h.store.entries[inv.ID] != nil {
		t.Error("its bookings stay")
	}
}
