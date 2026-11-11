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
	"github.com/adirkuhn/moneyharpy/internal/resource"
	"github.com/adirkuhn/moneyharpy/internal/sales"
)

// Owner is what a charge is for: a contact or a subscription.
type Owner struct {
	ContactID      *resource.ID
	SubscriptionID *resource.ID
}

// ChargeInput is a new additional charge, after Moneybird's fields.
type ChargeInput struct {
	ProductID resource.ID
	// Amount is the quantity, such as "1" or "5 x".
	Amount string
	// Price per unit; the product's when nil.
	Price *string
	// Period is a period such as 20250101..20250131, 202501..202503 or
	// this_month; it is kept as whole months.
	Period      string
	Description string
	Identifier  *string
}

// checkOwner checks that the contact or subscription exists.
func (s *Service) checkOwner(ctx context.Context, admin, user resource.ID, o Owner) error {
	if o.ContactID != nil {
		_, err := s.contacts.Contact(ctx, admin, *o.ContactID, user)
		return err
	}
	if o.SubscriptionID != nil {
		_, err := s.store.Subscription(ctx, admin, *o.SubscriptionID)
		return err
	}
	return ErrNotFound
}

// Charges returns the charges of a contact or a subscription; billed ones
// only with includeBilled.
func (s *Service) Charges(ctx context.Context, admin, user resource.ID, o Owner, includeBilled bool) ([]Charge, error) {
	if _, err := s.admins.Get(ctx, admin, user); err != nil {
		return nil, err
	}
	if err := s.checkOwner(ctx, admin, user, o); err != nil {
		return nil, err
	}
	return s.store.Charges(ctx, admin, ChargeQuery{ContactID: o.ContactID, SubscriptionID: o.SubscriptionID, IncludeBilled: includeBilled})
}

// AddCharge adds a charge to bill on the subscription's next invoice, or,
// for a contact, on an invoice made at the start of next month.
func (s *Service) AddCharge(ctx context.Context, admin, user resource.ID, o Owner, in ChargeInput) (Charge, error) {
	a, err := s.admins.Get(ctx, admin, user)
	if err != nil {
		return Charge{}, err
	}
	if err := s.checkOwner(ctx, admin, user, o); err != nil {
		return Charge{}, err
	}
	p, err := s.products.Product(ctx, admin, in.ProductID, user)
	if err != nil {
		return Charge{}, ErrProductNotFound
	}
	now := s.now()
	c := Charge{
		ID: s.ids.Next(), AdministrationID: admin, ContactID: o.ContactID, SubscriptionID: o.SubscriptionID, ProductID: p.ID, Price: p.Price,
		Description: strings.TrimSpace(in.Description), CreatedAt: now, UpdatedAt: now,
	}
	var verr resource.ValidationError
	if p.TaxRateID == nil {
		verr.Add("product_id", "has no VAT rate; give the product one")
	}
	if c.Amount, c.Quantity, err = quantityOf(in.Amount); err != nil {
		verr.Add("amount", "does not start with a number")
	}
	if in.Price != nil {
		if c.Price, err = money.ParseAmount(*in.Price); err != nil {
			verr.Add("price", "is not an amount")
		}
	}
	period, err := money.ParsePeriod(strings.TrimSpace(in.Period), s.today(a))
	if err != nil {
		verr.Add("period", "is not a period like 20250101..20250131, 202501..202503 or this_month")
	}
	c.Period = wholeMonths(period)
	if c.Description == "" {
		verr.Add("description", "can't be blank")
	}
	if in.Identifier != nil {
		c.Identifier = strings.TrimSpace(*in.Identifier)
	}
	for field, v := range map[string]string{"description": c.Description, "identifier": c.Identifier} {
		if len(v) > 10000 {
			verr.Add(field, "is too long (maximum is 10000 characters)")
		}
	}
	if err := verr.OrNil(); err != nil {
		return Charge{}, err
	}
	if err := s.store.SaveCharge(ctx, c); err != nil {
		return Charge{}, wrap("store charge", err)
	}
	return c, nil
}

// wholeMonths widens a period to the months it touches.
func wholeMonths(p money.Period) money.Period {
	if p.From.IsZero() {
		return p
	}
	from := time.Date(p.From.Year(), p.From.Month(), 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(p.To.Year(), p.To.Month()+1, 1, 0, 0, 0, 0, time.UTC).AddDate(0, 0, -1)
	return money.Period{From: from, To: to}
}

// line is the invoice line that bills the charge.
func (c Charge) line(row int) sales.DetailInput {
	return sales.DetailInput{
		ProductID: &c.ProductID, Description: &c.Description, Amount: ptr(lineAmount(c.Amount)), Price: ptr(c.Price.String()), RowOrder: &row,
		Period: ptr(c.Period.From.Format("20060102") + ".." + c.Period.To.Format("20060102")),
	}
}

// Generated implements sales.RecurringHook: it completes an invoice a
// subscription's recurring invoice made with the product running on the
// invoice date, the subscription's unbilled charges and the one-off
// invoices scheduled to go with it, which are merged into it and removed.
func (s *Service) Generated(ctx context.Context, admin, user resource.ID, inv sales.Invoice, r sales.Recurring) (sales.Invoice, error) {
	sub, err := s.store.SubscriptionByRecurring(ctx, admin, r.ID)
	if errors.Is(err, ErrNotFound) {
		return inv, nil
	}
	if err != nil {
		return inv, err
	}
	var changes sales.Changes
	if err := s.followItem(ctx, admin, user, sub, inv, r, &changes); err != nil {
		return inv, err
	}
	row := 0
	for _, d := range inv.Lines {
		row = max(row, d.RowOrder)
	}
	charges, err := s.store.Charges(ctx, admin, ChargeQuery{SubscriptionID: &sub.ID})
	if err != nil {
		return inv, err
	}
	for _, c := range charges {
		row++
		changes.Details = append(changes.Details, c.line(row))
	}
	oneOffs, err := s.mergeable(ctx, admin, user, sub, inv)
	if err != nil {
		return inv, err
	}
	for _, o := range oneOffs {
		for _, d := range o.Lines {
			row++
			changes.Details = append(changes.Details, copyLine(d, row))
		}
	}
	if changes.Details == nil && changes.Discount == nil {
		s.notify(ctx, admin, inv)
		return inv, nil
	}
	updated, err := s.sales.Update(ctx, admin, inv.ID, user, changes)
	if err != nil {
		return inv, err
	}
	if len(charges) > 0 {
		known := map[resource.ID]bool{}
		for _, d := range inv.Lines {
			known[d.ID] = true
		}
		var added []sales.Detail
		for _, d := range updated.Lines {
			if !known[d.ID] {
				added = append(added, d)
			}
		}
		slices.SortFunc(added, func(a, b sales.Detail) int { return a.RowOrder - b.RowOrder })
		for i := range charges {
			if i < len(added) {
				charges[i].DetailID, charges[i].InvoiceID = &added[i].ID, &updated.ID
			}
		}
		if err := s.store.BillCharges(ctx, admin, charges); err != nil {
			return updated, wrap("mark charges billed", err)
		}
	}
	var errs []error
	for _, o := range oneOffs {
		if err := s.sales.Delete(ctx, admin, o.ID, user); err != nil {
			errs = append(errs, fmt.Errorf("remove merged invoice %s: %w", o.ID, err))
		}
	}
	s.notify(ctx, admin, updated)
	return updated, errors.Join(errs...)
}

func (s *Service) notify(ctx context.Context, admin resource.ID, inv sales.Invoice) {
	if s.notifier != nil {
		s.notifier.Notify(ctx, admin, "sales_invoice_created_based_on_subscription", inv)
	}
}

// followItem makes the invoice, and the recurring invoice from then on,
// bill the item running on the invoice date, when a change started since.
func (s *Service) followItem(ctx context.Context, admin, user resource.ID, sub Subscription, inv sales.Invoice, r sales.Recurring, changes *sales.Changes) error {
	if inv.InvoiceDate == nil {
		return nil
	}
	item := sub.ItemAt(*inv.InvoiceDate)
	i := slices.IndexFunc(inv.Lines, func(d sales.Detail) bool { return d.ProductID != nil })
	if item.ProductID == 0 || i < 0 {
		return nil
	}
	line := inv.Lines[i]
	if *line.ProductID == item.ProductID && line.Amount == lineAmount(item.Amount) && inv.Discount == item.Discount {
		return nil
	}
	p, err := s.products.Product(ctx, admin, item.ProductID, user)
	if err != nil {
		return wrap("load subscribed product", err)
	}
	var rc sales.RecurringChanges
	if err := s.lineFor(&r, p, sub, item, &rc); err != nil {
		return err
	}
	if r.Active {
		if _, err := s.sales.UpdateRecurring(ctx, admin, r.ID, user, rc); err != nil {
			return wrap("follow subscription change", err)
		}
	}
	in := rc.Details[0]
	in.ID = &line.ID
	changes.Details = append(changes.Details, in)
	changes.Discount = rc.Discount
	return nil
}

// mergeable returns the one-off invoices of the subscription still waiting
// to be sent on or before the invoice's date.
func (s *Service) mergeable(ctx context.Context, admin, user resource.ID, sub Subscription, inv sales.Invoice) ([]sales.Invoice, error) {
	ids, err := s.store.OneOffInvoices(ctx, admin, sub.ID)
	if err != nil {
		return nil, err
	}
	var out []sales.Invoice
	for _, id := range ids {
		o, err := s.sales.Invoice(ctx, admin, id, user)
		if sales.IsNotFound(err) {
			continue
		}
		if err != nil {
			return nil, err
		}
		if o.Sent() || o.InvoiceDate == nil || inv.InvoiceDate == nil || o.InvoiceDate.After(*inv.InvoiceDate) {
			continue
		}
		out = append(out, o)
	}
	return out, nil
}

// copyLine is a new line like d.
func copyLine(d sales.Detail, row int) sales.DetailInput {
	in := sales.DetailInput{
		Description: &d.Description, Period: &d.Period, Price: ptr(d.Price.String()), Amount: &d.Amount, TaxRateID: &d.TaxRateID,
		LedgerAccountID: d.LedgerAccountID, ProjectID: d.ProjectID, RowOrder: &row, Optional: &d.Optional, Selected: &d.Selected,
	}
	if d.ProductID != nil {
		// Name the product without taking its defaults again: the line's
		// own description, price, VAT rate and category stay.
		in.ProductID = d.ProductID
	}
	return in
}

// Run bills contact charges every interval until ctx ends.
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

// Work bills the contact charges added before this month, once a month
// is over: per contact, one invoice dated the first of the month, sent
// at once. Each administration is worked on by one instance at a time.
func (s *Service) Work(ctx context.Context) error {
	list, err := s.store.ChargeWorkload(ctx)
	if err != nil {
		return fmt.Errorf("list charge workload: %w", err)
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
		errs = append(errs, s.billContacts(ctx, w.AdministrationID, w.UserID)...)
		release()
	}
	return errors.Join(errs...)
}

func (s *Service) billContacts(ctx context.Context, admin, user resource.ID) []error {
	a, err := s.listedOf(ctx, admin, user)
	if err != nil {
		return []error{err}
	}
	charges, err := s.store.UnbilledContactCharges(ctx, admin)
	if err != nil {
		return []error{err}
	}
	today := s.today(a)
	month := time.Date(today.Year(), today.Month(), 1, 0, 0, 0, 0, time.UTC)
	byContact := map[resource.ID][]Charge{}
	var order []resource.ID
	for _, c := range charges {
		if c.ContactID == nil || !s.addedBefore(a, c, month) {
			continue
		}
		if byContact[*c.ContactID] == nil {
			order = append(order, *c.ContactID)
		}
		byContact[*c.ContactID] = append(byContact[*c.ContactID], c)
	}
	var errs []error
	for _, contact := range order {
		if err := s.billContact(ctx, admin, user, contact, month, byContact[contact]); err != nil {
			errs = append(errs, fmt.Errorf("administration %s, bill charges of contact %s: %w", admin, contact, err))
		}
	}
	return errs
}

// addedBefore reports whether the charge was added before the day, in the
// administration's time zone.
func (s *Service) addedBefore(a administration.Listed, c Charge, day time.Time) bool {
	at := c.CreatedAt
	if loc, err := time.LoadLocation(a.TimeZone); err == nil {
		at = at.In(loc)
	}
	return time.Date(at.Year(), at.Month(), at.Day(), 0, 0, 0, 0, time.UTC).Before(day)
}

func (s *Service) billContact(ctx context.Context, admin, user, contact resource.ID, day time.Time, charges []Charge) error {
	details := make([]sales.DetailInput, len(charges))
	for i, c := range charges {
		details[i] = c.line(i + 1)
	}
	date := day.Format(time.DateOnly)
	inv, err := s.sales.Create(ctx, admin, user, sales.Changes{ContactID: &contact, InvoiceDate: &date, Details: details})
	if err != nil {
		return err
	}
	slices.SortFunc(inv.Lines, func(a, b sales.Detail) int { return a.RowOrder - b.RowOrder })
	for i := range charges {
		if i < len(inv.Lines) {
			charges[i].DetailID, charges[i].InvoiceID = &inv.Lines[i].ID, &inv.ID
		}
	}
	if err := s.store.BillCharges(ctx, admin, charges); err != nil {
		_ = s.sales.Delete(ctx, admin, inv.ID, user)
		return wrap("mark charges billed", err)
	}
	if _, err := s.sales.Send(ctx, admin, inv.ID, user, sales.SendInput{InvoiceDate: date}); err != nil {
		return wrap("send invoice "+inv.ID.String(), err)
	}
	return nil
}
