package subscriptions

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/adirkuhn/moneyharpy/internal/administration"
	"github.com/adirkuhn/moneyharpy/internal/money"
	"github.com/adirkuhn/moneyharpy/internal/products"
	"github.com/adirkuhn/moneyharpy/internal/resource"
	"github.com/adirkuhn/moneyharpy/internal/sales"
)

// Input is a new subscription, after Moneybird's fields. Start date,
// product and contact are required; the frequency defaults to the
// product's.
type Input struct {
	StartDate        *string
	EndDate          *string
	ProductID        *resource.ID
	ContactID        *resource.ID
	ContactPersonID  *resource.ID
	DocumentStyleID  *resource.ID
	Amount           *string
	Discount         *string
	Reference        *string
	Frequency        *int
	FrequencyType    *string
	Mergeable        *bool
	PricesAreInclTax *bool
}

// Changes change a subscription; nil fields stay. A product, amount or
// discount starts a new item from StartDate, today unless given.
type Changes struct {
	ContactPersonID  *resource.ID
	DocumentStyleID  *resource.ID
	Reference        *string
	Mergeable        *bool
	PricesAreInclTax *bool
	ProductID        *resource.ID
	StartDate        *string
	Amount           *string
	Discount         *string
}

// productChange reports whether the changes start a new item.
func (c Changes) productChange() bool {
	return c.ProductID != nil || c.Amount != nil || c.Discount != nil || c.StartDate != nil
}

// List returns a contact's subscriptions.
func (s *Service) List(ctx context.Context, admin, user, contact resource.ID) ([]Subscription, error) {
	if _, err := s.admins.Get(ctx, admin, user); err != nil {
		return nil, err
	}
	if _, err := s.contacts.Contact(ctx, admin, contact, user); err != nil {
		return nil, err
	}
	return s.store.Subscriptions(ctx, admin, contact)
}

// Subscription returns one subscription.
func (s *Service) Subscription(ctx context.Context, admin, id, user resource.ID) (Subscription, error) {
	if _, err := s.admins.Get(ctx, admin, user); err != nil {
		return Subscription{}, err
	}
	return s.store.Subscription(ctx, admin, id)
}

// Create starts a subscription and the recurring sales invoice that bills
// it. The first invoice goes out on the start date, but no sooner than
// tomorrow; each bills one period of the subscription in advance.
func (s *Service) Create(ctx context.Context, admin, user resource.ID, in Input) (Subscription, error) {
	a, err := s.admins.Get(ctx, admin, user)
	if err != nil {
		return Subscription{}, err
	}
	today := s.today(a)
	var verr resource.ValidationError
	now := s.now()
	sub := Subscription{ID: s.ids.Next(), AdministrationID: admin, CreatedAt: now, UpdatedAt: now}
	item := Item{ID: s.ids.Next(), Amount: "1 x", Quantity: 1000}

	if in.ContactID == nil {
		verr.Add("contact_id", "can't be blank")
	} else if _, err := s.contacts.Contact(ctx, admin, *in.ContactID, user); err != nil {
		verr.Add("contact_id", "does not exist")
	} else {
		sub.ContactID = *in.ContactID
	}
	sub.ContactPersonID = in.ContactPersonID
	if in.StartDate == nil {
		verr.Add("start_date", "can't be blank")
	} else if d := parseDate("start_date", *in.StartDate, &verr); d != nil {
		sub.StartDate, item.StartDate = *d, *d
	}
	if in.EndDate != nil && strings.TrimSpace(*in.EndDate) != "" {
		if d := parseDate("end_date", *in.EndDate, &verr); d != nil {
			if d.Before(sub.StartDate) {
				verr.Add("end_date", "must not lie before the start date")
			}
			sub.EndDate, item.EndDate = d, d
		}
	}
	var product products.Product
	if in.ProductID == nil {
		verr.Add("product_id", "can't be blank")
	} else if product, err = s.billable(ctx, admin, user, *in.ProductID); err != nil {
		verr.Add("product_id", err.Error())
	} else {
		item.ProductID = product.ID
		sub.Frequency, sub.FrequencyType = product.Frequency, product.FrequencyType
	}
	if in.FrequencyType != nil {
		sub.FrequencyType = strings.TrimSpace(*in.FrequencyType)
	}
	if in.Frequency != nil {
		sub.Frequency = *in.Frequency
	}
	count := 0
	if item.ProductID != 0 {
		if count = periods(product.Frequency, product.FrequencyType, sub.Frequency, sub.FrequencyType); count == 0 {
			verr.Add("frequency", fmt.Sprintf("does not fit the product's, every %d %s", product.Frequency, product.FrequencyType))
		}
	}
	s.applyQuantity(&item, in.Amount, in.Discount, &verr)
	if in.Reference != nil {
		sub.Reference = strings.TrimSpace(*in.Reference)
	}
	if err := verr.OrNil(); err != nil {
		return Subscription{}, err
	}
	sub.Items = []Item{item}

	first := sub.StartDate
	if tomorrow := today.AddDate(0, 0, 1); first.Before(tomorrow) {
		first = tomorrow
	}
	end := sales.Occurrence(first, sub.FrequencyType, sub.Frequency, 1).AddDate(0, 0, -1)
	rc := sales.RecurringChanges{
		ContactID: &sub.ContactID, ContactPersonID: sub.ContactPersonID, DocumentStyleID: cmpID(in.DocumentStyleID, product.DocumentStyleID),
		WorkflowID: product.WorkflowID, Reference: in.Reference, PricesAreInclTax: in.PricesAreInclTax, Mergeable: in.Mergeable,
		Discount: ptr(percentText(item.Discount)), InvoiceDate: ptr(first.Format(time.DateOnly)), FrequencyType: &sub.FrequencyType,
		Frequency: &sub.Frequency, AutoSend: ptr(true),
		Details: []sales.DetailInput{{
			ProductID: &product.ID, Amount: ptr(lineAmount(item.Amount)), Price: ptr(linePrice(product, count).String()),
			Period: ptr(first.Format("20060102") + ".." + end.Format("20060102")),
		}},
	}
	if sub.EndDate != nil {
		rc.LastDate = ptr(sub.EndDate.AddDate(0, 0, -1).Format(time.DateOnly))
		if sub.EndDate.AddDate(0, 0, -1).Before(first) {
			return Subscription{}, invalid("end_date", "must lie after the first invoice date, "+first.Format(time.DateOnly))
		}
	}
	r, err := s.sales.CreateRecurring(ctx, admin, user, rc)
	if err != nil {
		return Subscription{}, err
	}
	sub.RecurringID = &r.ID
	if err := s.store.SaveSubscription(ctx, sub, true); err != nil {
		_, _ = s.sales.DeleteRecurring(ctx, admin, r.ID, user)
		return Subscription{}, wrap("store subscription", err)
	}
	return sub, nil
}

func cmpID(a, b *resource.ID) *resource.ID {
	if a != nil {
		return a
	}
	return b
}

// billable returns a product a subscription can bill: an active one that is
// billed periodically.
func (s *Service) billable(ctx context.Context, admin, user, id resource.ID) (products.Product, error) {
	p, err := s.products.Product(ctx, admin, id, user)
	switch {
	case err != nil:
		return p, errors.New("does not exist")
	case !p.Active:
		return p, errors.New("is deactivated")
	case p.FrequencyType == "":
		return p, errors.New("is not billed periodically; give the product a frequency")
	case p.TaxRateID == nil:
		return p, errors.New("has no VAT rate; give the product one")
	}
	return p, nil
}

func (s *Service) applyQuantity(item *Item, amount, discount *string, verr *resource.ValidationError) {
	if amount != nil {
		a, q, err := quantityOf(*amount)
		if err != nil {
			verr.Add("amount", "does not start with a number")
		} else {
			item.Amount, item.Quantity = a, q
		}
	}
	if discount != nil {
		d, err := percentage(*discount)
		if err != nil {
			verr.Add("discount", "is not a percentage from 0 to 100")
		} else {
			item.Discount = d
		}
	}
}

// Update changes a subscription. A new product, amount or discount starts
// from the given start date, today unless given; when that falls within a
// period already billed, the difference for the rest of it is billed with
// the next invoice.
func (s *Service) Update(ctx context.Context, admin, id, user resource.ID, c Changes) (Subscription, error) {
	a, err := s.admins.Get(ctx, admin, user)
	if err != nil {
		return Subscription{}, err
	}
	sub, err := s.store.Subscription(ctx, admin, id)
	if err != nil {
		return Subscription{}, err
	}
	if sub.CancelledAt != nil {
		return Subscription{}, ErrCancelled
	}
	today := s.today(a)
	var verr resource.ValidationError
	if c.ContactPersonID != nil {
		sub.ContactPersonID = nil
		if *c.ContactPersonID != 0 {
			sub.ContactPersonID = c.ContactPersonID
		}
	}
	if c.Reference != nil {
		sub.Reference = strings.TrimSpace(*c.Reference)
	}
	rc := sales.RecurringChanges{
		ContactPersonID: c.ContactPersonID, DocumentStyleID: c.DocumentStyleID, Reference: c.Reference, Mergeable: c.Mergeable, PricesAreInclTax: c.PricesAreInclTax,
	}
	var change *itemChange
	if c.productChange() {
		change = s.changeItem(ctx, admin, user, &sub, c, today, &verr)
	}
	if err := verr.OrNil(); err != nil {
		return Subscription{}, err
	}
	var r *sales.Recurring
	if sub.RecurringID != nil {
		got, err := s.sales.Recurring(ctx, admin, *sub.RecurringID, user)
		if err != nil {
			return Subscription{}, err
		}
		r = &got
	}
	if change != nil && r != nil && r.Active && !change.item.StartDate.After(r.InvoiceDate) {
		if err := s.lineFor(r, change.product, sub, change.item, &rc); err != nil {
			return Subscription{}, err
		}
	}
	if r != nil && r.Active && changesRecurring(rc) {
		updated, err := s.sales.UpdateRecurring(ctx, admin, r.ID, user, rc)
		if err != nil {
			return Subscription{}, err
		}
		if change != nil {
			if err := s.billDifference(ctx, admin, user, sub, *r, updated, change); err != nil {
				return Subscription{}, err
			}
		}
	}
	sub.UpdatedAt = s.now()
	if err := s.store.SaveSubscription(ctx, sub, false); err != nil {
		return Subscription{}, wrap("store subscription", err)
	}
	return sub, nil
}

// changesRecurring reports whether rc changes anything.
func changesRecurring(rc sales.RecurringChanges) bool {
	return rc.ContactPersonID != nil || rc.DocumentStyleID != nil || rc.Reference != nil || rc.Mergeable != nil || rc.PricesAreInclTax != nil ||
		rc.Discount != nil || rc.Details != nil
}

// itemChange is a new item with its product and the item it follows.
type itemChange struct {
	item, previous Item
	product        products.Product
}

func (s *Service) changeItem(ctx context.Context, admin, user resource.ID, sub *Subscription, c Changes, today time.Time, verr *resource.ValidationError) *itemChange {
	start := today
	if len(sub.Items) > 0 && sub.Items[0].StartDate.After(start) {
		// Not started yet: the change starts with the subscription.
		start = sub.Items[0].StartDate
	}
	if c.StartDate != nil {
		if d := parseDate("start_date", *c.StartDate, verr); d != nil {
			start = *d
		}
	}
	previous := sub.ItemAt(start)
	if start.Before(previous.StartDate) {
		verr.Add("start_date", "must not lie before "+previous.StartDate.Format(time.DateOnly))
		return nil
	}
	item := previous
	item.ID, item.StartDate, item.EndDate = s.ids.Next(), start, sub.EndDate
	productID := previous.ProductID
	if c.ProductID != nil {
		productID = *c.ProductID
	}
	p, err := s.billable(ctx, admin, user, productID)
	if err != nil {
		verr.Add("product_id", err.Error())
		return nil
	}
	if periods(p.Frequency, p.FrequencyType, sub.Frequency, sub.FrequencyType) == 0 {
		verr.Add("product_id", fmt.Sprintf("is billed every %d %s, which does not fit the subscription's every %d %s", p.Frequency, p.FrequencyType, sub.Frequency, sub.FrequencyType))
		return nil
	}
	item.ProductID = productID
	s.applyQuantity(&item, c.Amount, c.Discount, verr)
	var kept []Item
	for _, it := range sub.Items {
		switch {
		case it.ID == previous.ID && it.StartDate.Equal(start):
			// Replaced from its first day.
		case it.ID == previous.ID:
			end := start.AddDate(0, 0, -1)
			it.EndDate = &end
			kept = append(kept, it)
		case it.StartDate.After(start):
			// A later change gives way to this one.
		default:
			kept = append(kept, it)
		}
	}
	sub.Items = append(kept, item)
	sortItems(sub.Items)
	return &itemChange{item: item, previous: previous, product: p}
}

// lineFor sets the recurring invoice's line and discount to bill item.
func (s *Service) lineFor(r *sales.Recurring, p products.Product, sub Subscription, item Item, rc *sales.RecurringChanges) error {
	i := slices.IndexFunc(r.Lines, func(d sales.Detail) bool { return d.ProductID != nil })
	if i < 0 {
		i = 0
	}
	if len(r.Lines) == 0 {
		return errors.New("the recurring invoice of the subscription has no lines")
	}
	in := sales.DetailInput{
		ID: &r.Lines[i].ID, ProductID: &item.ProductID, Amount: ptr(lineAmount(item.Amount)),
		Price: ptr(linePrice(p, periods(p.Frequency, p.FrequencyType, sub.Frequency, sub.FrequencyType)).String()),
	}
	if r.Lines[i].ProductID == nil || *r.Lines[i].ProductID != item.ProductID {
		in.Description = ptr(p.LineDescription())
	}
	rc.Details = []sales.DetailInput{in}
	rc.Discount = ptr(percentText(item.Discount))
	return nil
}

// periodValue is what one period of the item costs: price times quantity
// less the discount.
func periodValue(price money.Amount, quantity, discount int64) int64 {
	gross := int64(price) * quantity / 1000
	return gross * (10000 - discount) / 10000
}

// billDifference bills, with the next invoice, the extra a change costs
// for the rest of a period already billed.
func (s *Service) billDifference(ctx context.Context, admin, user resource.ID, sub Subscription, before, after sales.Recurring, change *itemChange) error {
	if before.Periods == 0 || before.Generated == 0 || !change.item.StartDate.Before(before.InvoiceDate) {
		return nil
	}
	periodStart := sales.Occurrence(before.Anchor, before.FrequencyType, before.Frequency, before.Periods-1)
	total := before.InvoiceDate.Sub(periodStart).Hours() / 24
	from := change.item.StartDate
	if from.Before(periodStart) {
		from = periodStart
	}
	left := before.InvoiceDate.Sub(from).Hours() / 24
	if total <= 0 || left <= 0 {
		return nil
	}
	i := slices.IndexFunc(before.Lines, func(d sales.Detail) bool { return d.ProductID != nil })
	j := slices.IndexFunc(after.Lines, func(d sales.Detail) bool { return d.ProductID != nil })
	if i < 0 || j < 0 {
		return nil
	}
	diff := periodValue(after.Lines[j].Price, after.Lines[j].Quantity, after.Discount) - periodValue(before.Lines[i].Price, before.Lines[i].Quantity, before.Discount)
	extra := money.Amount(int64(float64(diff)*left/total + 0.5))
	if extra <= 0 {
		return nil
	}
	last := before.InvoiceDate.AddDate(0, 0, -1)
	text := fmt.Sprintf("%s (%s – %s)", change.product.LineDescription(), from.Format(time.DateOnly), last.Format(time.DateOnly))
	_, err := s.scheduleOneOff(ctx, admin, user, sub, after, []sales.DetailInput{{
		ProductID: &change.product.ID, Description: &text, Price: ptr(extra.String()), Amount: ptr("1"),
		Period: ptr(from.Format("20060102") + ".." + last.Format("20060102")),
	}}, nil)
	return err
}

// Cancel stops a subscription on its end date, today unless given: the
// recurring sales invoice makes no invoice from then on.
func (s *Service) Cancel(ctx context.Context, admin, id, user resource.ID, endDate *string) (Subscription, error) {
	a, err := s.admins.Get(ctx, admin, user)
	if err != nil {
		return Subscription{}, err
	}
	sub, err := s.store.Subscription(ctx, admin, id)
	if err != nil {
		return Subscription{}, err
	}
	if sub.CancelledAt != nil {
		return Subscription{}, ErrCancelled
	}
	today := s.today(a)
	end := today
	if endDate != nil && strings.TrimSpace(*endDate) != "" {
		var verr resource.ValidationError
		d := parseDate("end_date", *endDate, &verr)
		if err := verr.OrNil(); err != nil {
			return Subscription{}, err
		}
		end = *d
	}
	if end.Before(today) {
		return Subscription{}, invalid("end_date", "must not lie in the past")
	}
	now := s.now()
	sub.EndDate, sub.CancelledAt, sub.UpdatedAt = &end, &now, now
	var kept []Item
	for _, it := range sub.Items {
		if it.StartDate.After(end) {
			continue
		}
		if it.EndDate == nil || it.EndDate.After(end) {
			it.EndDate = &end
		}
		kept = append(kept, it)
	}
	if len(kept) == 0 && len(sub.Items) > 0 {
		first := sub.Items[0]
		first.EndDate = &end
		kept = []Item{first}
	}
	sub.Items = kept
	if sub.RecurringID != nil {
		if err := s.stopRecurring(ctx, admin, user, *sub.RecurringID, end); err != nil {
			return Subscription{}, err
		}
	}
	if err := s.store.SaveSubscription(ctx, sub, false); err != nil {
		return Subscription{}, wrap("store subscription", err)
	}
	return sub, nil
}

// stopRecurring makes the recurring invoice make no invoice on or after
// end.
func (s *Service) stopRecurring(ctx context.Context, admin, user, id resource.ID, end time.Time) error {
	r, err := s.sales.Recurring(ctx, admin, id, user)
	if err != nil || !r.Active {
		return err
	}
	last := end.AddDate(0, 0, -1)
	if last.Before(r.InvoiceDate) {
		_, err = s.sales.DeactivateRecurring(ctx, admin, id, user)
		return err
	}
	_, err = s.sales.UpdateRecurring(ctx, admin, id, user, sales.RecurringChanges{LastDate: ptr(last.Format(time.DateOnly))})
	return err
}

// ScheduleOneOff makes a sales invoice to go with the subscription's next
// invoice: it is scheduled for that date, or invoiceDate, and merged into
// the subscription's invoice when that is made.
func (s *Service) ScheduleOneOff(ctx context.Context, admin, id, user resource.ID, details []sales.DetailInput, invoiceDate *string) (sales.Invoice, error) {
	if _, err := s.admins.Get(ctx, admin, user); err != nil {
		return sales.Invoice{}, err
	}
	sub, err := s.store.Subscription(ctx, admin, id)
	if err != nil {
		return sales.Invoice{}, err
	}
	if sub.CancelledAt != nil {
		return sales.Invoice{}, ErrCancelled
	}
	if sub.RecurringID == nil {
		return sales.Invoice{}, invalid("base", "The subscription has no next invoice")
	}
	r, err := s.sales.Recurring(ctx, admin, *sub.RecurringID, user)
	if err != nil {
		return sales.Invoice{}, err
	}
	if !r.Active {
		return sales.Invoice{}, invalid("base", "The subscription has no next invoice")
	}
	if len(details) == 0 {
		return sales.Invoice{}, invalid("details_attributes", "can't be blank")
	}
	return s.scheduleOneOff(ctx, admin, user, sub, r, details, invoiceDate)
}

func (s *Service) scheduleOneOff(ctx context.Context, admin, user resource.ID, sub Subscription, r sales.Recurring, details []sales.DetailInput, invoiceDate *string) (sales.Invoice, error) {
	day := r.InvoiceDate.Format(time.DateOnly)
	if invoiceDate != nil && strings.TrimSpace(*invoiceDate) != "" {
		day = strings.TrimSpace(*invoiceDate)
	}
	inv, err := s.sales.Create(ctx, admin, user, sales.Changes{
		ContactID: &sub.ContactID, ContactPersonID: sub.ContactPersonID, DocumentStyleID: &r.DocumentStyleID, WorkflowID: &r.WorkflowID,
		IdentityID: &r.IdentityID, Reference: &r.Reference, PricesAreInclTax: &r.PricesAreInclTax, InvoiceDate: &day, Details: details,
	})
	if err != nil {
		return sales.Invoice{}, err
	}
	scheduled, err := s.sales.Send(ctx, admin, inv.ID, user, sales.SendInput{Scheduled: true, InvoiceDate: day})
	if err != nil {
		_ = s.sales.Delete(ctx, admin, inv.ID, user)
		return sales.Invoice{}, err
	}
	if err := s.store.AddOneOffInvoice(ctx, admin, sub.ID, inv.ID); err != nil {
		return sales.Invoice{}, wrap("link one-off invoice", err)
	}
	// Read it again: the link makes it the subscription's.
	if linked, err := s.sales.Invoice(ctx, admin, inv.ID, user); err == nil {
		return linked, nil
	}
	return scheduled, nil
}

// listedOf is the administration a hook or the worker acts in.
func (s *Service) listedOf(ctx context.Context, admin, user resource.ID) (administration.Listed, error) {
	return s.admins.Get(ctx, admin, user)
}
