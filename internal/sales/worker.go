package sales

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/adirkuhn/moneyharpy/internal/money"
	"github.com/adirkuhn/moneyharpy/internal/resource"
)

// Tally counts invoices and adds up what they have unpaid.
type Tally struct {
	Count  int
	Amount money.Amount
}

// Receivables sum up the sent invoices still to be paid; ToRemind are
// those past their due date and not paused.
type Receivables struct {
	ToReceive Tally
	ToRemind  Tally
}

// Summary is what the overview shows of sales invoices.
type Summary struct {
	Receivables
	Currency string
}

// Summary sums up the invoices still to be paid.
func (s *Service) Summary(ctx context.Context, admin, user resource.ID) (Summary, error) {
	a, err := s.admins.Get(ctx, admin, user)
	if err != nil {
		return Summary{}, err
	}
	r, err := s.store.Receivables(ctx, admin, s.today(a))
	if err != nil {
		return Summary{}, fmt.Errorf("sum up receivables: %w", err)
	}
	return Summary{Receivables: r, Currency: a.Currency}, nil
}

// Run does the background work every interval until ctx ends: sending
// scheduled invoices, marking invoices late and sending the reminders
// their workflows send automatically.
func (s *Service) Run(ctx context.Context, interval time.Duration, onError func(error)) {
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		if err := s.Work(ctx); err != nil {
			onError(err)
		}
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

// Work does one round of background work. Each administration is worked
// on by one instance at a time, and each step changes the invoices so a
// next round does not repeat it.
func (s *Service) Work(ctx context.Context) error {
	list, err := s.store.Workload(ctx)
	if err != nil {
		return fmt.Errorf("list workload: %w", err)
	}
	var errs []error
	for _, w := range list {
		if ctx.Err() != nil {
			break
		}
		release, ok, err := s.store.Lock(ctx, w.AdministrationID)
		if err != nil {
			errs = append(errs, fmt.Errorf("lock administration %s: %w", w.AdministrationID, err))
			continue
		}
		if !ok {
			continue
		}
		errs = append(errs, s.workOn(ctx, w.AdministrationID, w.UserID)...)
		release()
	}
	return errors.Join(errs...)
}

func (s *Service) workOn(ctx context.Context, admin, user resource.ID) []error {
	a, err := s.admins.Get(ctx, admin, user)
	if err != nil {
		return []error{err}
	}
	today := s.today(a)
	var errs []error
	fail := func(what string, id resource.ID, err error) {
		errs = append(errs, fmt.Errorf("administration %s, %s %s: %w", admin, what, id, err))
	}

	scheduled, _, err := s.store.Invoices(ctx, admin, Query{States: []string{Scheduled}, Today: today})
	if err != nil {
		return append(errs, err)
	}
	for _, inv := range scheduled {
		if inv.InvoiceDate == nil || inv.InvoiceDate.After(today) {
			continue
		}
		in := SendInput{InvoiceDate: inv.InvoiceDate.Format(time.DateOnly)}
		if inv.Delivery != nil {
			in.EmailMessage = inv.Delivery.EmailMessage
		}
		sent, err := s.Send(ctx, admin, inv.ID, user, in)
		if err != nil && !errors.Is(err, ErrDelivery) {
			fail("send scheduled invoice", inv.ID, err)
			continue
		}
		if err != nil {
			fail("deliver scheduled invoice", inv.ID, err)
			sent, _ = s.Invoice(ctx, admin, inv.ID, user)
		}
		action := map[string]string{"Email": "sales_invoice_send_email", "Post": "sales_invoice_send_post"}[deliveryMethod(sent)]
		if action == "" {
			action = "sales_invoice_send_manually"
		}
		s.notify(ctx, admin, action, sent)
		s.notify(ctx, admin, "sales_invoice_state_changed_to_open", sent)
	}

	late, _, err := s.store.Invoices(ctx, admin, Query{States: []string{Late}, Today: today})
	if err != nil {
		return append(errs, err)
	}
	for _, inv := range late {
		if inv.State != Open {
			continue
		}
		inv.State, inv.UpdatedAt = Late, s.now()
		if err := s.save(ctx, admin, user, &inv, nil, false); err != nil {
			fail("mark late", inv.ID, err)
			continue
		}
		s.notify(ctx, admin, "sales_invoice_state_changed_to_late", inv)
	}
	return append(errs, s.remind(ctx, admin, user, today)...)
}

// remind sends the reminders that are due and that the workflow sends by
// itself, grouped as Moneybird groups them. Contacts without an email
// address get none.
func (s *Service) remind(ctx context.Context, admin, user resource.ID, today time.Time) []error {
	due, _, err := s.store.Invoices(ctx, admin, Query{States: []string{Late, Reminded}, Today: today})
	if err != nil || len(due) == 0 {
		if err != nil {
			return []error{err}
		}
		return nil
	}
	flows, err := s.invoicing.Workflows(ctx, admin, user)
	if err != nil {
		return []error{wrap("load workflows", err)}
	}
	type key struct{ contact, workflow, style, identity resource.ID }
	groups := map[key][]resource.ID{}
	var order []key
	for _, inv := range due {
		for _, w := range flows {
			if day, step, ok := nextReminder(inv, w); ok && step.ReminderAutoSend && !day.After(today) {
				k := key{inv.ContactID, inv.WorkflowID, inv.DocumentStyleID, inv.IdentityID}
				if groups[k] == nil {
					order = append(order, k)
				}
				groups[k] = append(groups[k], inv.ID)
			}
		}
	}
	var errs []error
	for _, k := range order {
		contact, err := s.contacts.Contact(ctx, admin, k.contact, user)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		if contact.SendInvoicesToEmail == "" {
			continue
		}
		in := ReminderInput{ContactID: k.contact, WorkflowID: k.workflow, DocumentStyleID: k.style, IdentityID: k.identity, InvoiceIDs: groups[k], DeliveryMethod: "Email"}
		if _, err := s.SendReminders(ctx, admin, user, []ReminderInput{in}); err != nil {
			errs = append(errs, fmt.Errorf("administration %s, remind contact %s: %w", admin, k.contact, err))
		}
	}
	return errs
}

func deliveryMethod(inv Invoice) string {
	if inv.Delivery == nil {
		return ""
	}
	return inv.Delivery.Method
}
