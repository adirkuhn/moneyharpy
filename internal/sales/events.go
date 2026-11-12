package sales

import (
	"context"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/adirkuhn/moneyharpy/internal/history"
	"github.com/adirkuhn/moneyharpy/internal/resource"
)

// History keeps what happened to invoices, as Moneybird's events.
type History interface {
	Record(ctx context.Context, events ...history.Event) error
	Of(ctx context.Context, admin resource.ID, entityType string, entities []resource.ID) (map[resource.ID][]history.Event, error)
}

// event is something to record about an invoice.
type event struct {
	invoice    Invoice
	action     string
	data       map[string]any
	linkType   string
	linkEntity *resource.ID
}

// on makes an event about an invoice.
func on(inv Invoice, action string) event { return event{invoice: inv, action: action} }

// with adds details to the event.
func (e event) with(data map[string]any) event {
	e.data = data
	return e
}

// linked names the record the event is about, such as a payment.
func (e event) linked(entityType string, id resource.ID) event {
	e.linkType, e.linkEntity = entityType, &id
	return e
}

// record stores what happened, by user. Without a history nothing is kept.
func (s *Service) record(ctx context.Context, user resource.ID, events ...event) error {
	if s.history == nil || len(events) == 0 {
		return nil
	}
	list := make([]history.Event, len(events))
	for i, e := range events {
		list[i] = history.Event{
			AdministrationID: e.invoice.AdministrationID, EntityType: e.invoice.kind(), EntityID: e.invoice.ID, UserID: user, Action: e.action,
			LinkEntityType: e.linkType, LinkEntityID: e.linkEntity, Data: e.data,
		}
	}
	return wrap("record events", s.history.Record(ctx, list...))
}

// stateChanged is the event of an invoice reaching a state.
func stateChanged(inv Invoice, state string) event {
	return on(inv, inv.events()+"_state_changed_to_"+state)
}

// sentEvent is how the invoice went out: by email, by post or marked as
// sent by hand, with where an email went.
func sentEvent(inv Invoice) event {
	switch deliveryMethod(inv) {
	case "Email":
		return on(inv, "sales_invoice_send_email").with(map[string]any{"email_address": inv.Delivery.EmailAddress, "email_message": inv.Delivery.EmailMessage})
	case "Post":
		return on(inv, "sales_invoice_send_post")
	}
	return on(inv, "sales_invoice_send_manually")
}

// changes tells what an update changed, as Moneybird's
// sales_invoice_updated does: each field with its old and new value.
func changes(before, after Invoice) map[string]any {
	out := map[string]any{}
	add := func(field string, was, is any) {
		out[field] = []any{was, is}
	}
	day := func(t *time.Time) any {
		if t == nil {
			return nil
		}
		return t.Format(time.DateOnly)
	}
	id := func(v *resource.ID) any {
		if v == nil {
			return nil
		}
		return v.String()
	}
	if before.ContactID != after.ContactID {
		add("contact_id", before.ContactID.String(), after.ContactID.String())
	}
	if a, b := id(before.ContactPersonID), id(after.ContactPersonID); a != b {
		add("contact_person_id", a, b)
	}
	if before.Reference != after.Reference {
		add("reference", before.Reference, after.Reference)
	}
	if a, b := day(before.InvoiceDate), day(after.InvoiceDate); a != b {
		add("invoice_date", a, b)
	}
	if a, b := day(before.DueDate), day(after.DueDate); a != b {
		add("due_date", a, b)
	}
	if before.Currency != after.Currency {
		add("currency", before.Currency, after.Currency)
	}
	if before.PricesAreInclTax != after.PricesAreInclTax {
		add("prices_are_incl_tax", before.PricesAreInclTax, after.PricesAreInclTax)
	}
	if before.PaymentConditions != after.PaymentConditions {
		add("payment_conditions", before.PaymentConditions, after.PaymentConditions)
	}
	if before.Discount != after.Discount {
		add("discount", percent(before.Discount), percent(after.Discount))
	}
	if before.FirstDueInterval != after.FirstDueInterval {
		add("first_due_interval", before.FirstDueInterval, after.FirstDueInterval)
	}
	if before.WorkflowID != after.WorkflowID {
		add("workflow_id", before.WorkflowID.String(), after.WorkflowID.String())
	}
	if before.DocumentStyleID != after.DocumentStyleID {
		add("document_style_id", before.DocumentStyleID.String(), after.DocumentStyleID.String())
	}
	if before.IdentityID != after.IdentityID {
		add("identity_id", before.IdentityID.String(), after.IdentityID.String())
	}
	if before.Totals.InclTax != after.Totals.InclTax {
		add("total_price_incl_tax", before.Totals.InclTax.String(), after.Totals.InclTax.String())
	}
	if !slices.EqualFunc(before.Lines, after.Lines, sameLine) {
		add("details", len(before.Lines), len(after.Lines))
	}
	return out
}

func sameLine(a, b Detail) bool {
	return a.ID == b.ID && a.Description == b.Description && a.Period == b.Period && a.Price == b.Price && a.Quantity == b.Quantity &&
		a.TaxRateID == b.TaxRateID && ptrEqual(a.LedgerAccountID, b.LedgerAccountID) && ptrEqual(a.ProjectID, b.ProjectID) &&
		ptrEqual(a.ProductID, b.ProductID) && a.RowOrder == b.RowOrder && a.Optional == b.Optional && a.Selected == b.Selected
}

func ptrEqual(a, b *resource.ID) bool {
	return (a == nil && b == nil) || (a != nil && b != nil && *a == *b)
}

// percent writes a percentage kept in hundredths, such as "2.5".
func percent(p int64) string {
	text := strconv.FormatInt(p/100, 10) + "." + fmt.Sprintf("%02d", p%100)
	return strings.TrimSuffix(strings.TrimRight(text, "0"), ".")
}
