package sales

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/adirkuhn/moneyharpy/internal/administration"
	"github.com/adirkuhn/moneyharpy/internal/notes"
	"github.com/adirkuhn/moneyharpy/internal/resource"
)

// RecurringEntityType names recurring sales invoices in notes.
const RecurringEntityType = "RecurringSalesInvoice"

// Frequency types, with Moneybird's names.
const (
	Day     = "day"
	Week    = "week"
	Month   = "month"
	Quarter = "quarter"
	Year    = "year"
)

// FrequencyTypes are the units a recurring invoice repeats in.
var FrequencyTypes = []string{Day, Week, Month, Quarter, Year}

// maxFrequency bounds how many units lie between two invoices.
const maxFrequency = 1000

// Recurring is a recurring sales invoice: a template that makes a draft
// sales invoice on each of its dates, and sends it when it auto-sends.
type Recurring struct {
	ID                resource.ID
	AdministrationID  resource.ID
	ContactID         resource.ID
	ContactPersonID   *resource.ID
	IdentityID        resource.ID
	DocumentStyleID   resource.ID
	WorkflowID        resource.ID
	Reference         string
	Language          string
	Currency          string
	PricesAreInclTax  bool
	PaymentConditions string
	// Discount is a percentage in hundredths.
	Discount         int64
	FirstDueInterval int
	// StartDate is the date of the first invoice; InvoiceDate that of the
	// next one; LastDate, if set, the last day an invoice may be made on.
	StartDate   time.Time
	InvoiceDate time.Time
	LastDate    *time.Time
	// Active is false once deactivated or past the last date, for good.
	Active        bool
	FrequencyType string
	Frequency     int
	AutoSend      bool
	// AutoSendAt and AutoSendUserID are when and by whom auto-sending was
	// turned on.
	AutoSendAt     *time.Time
	AutoSendUserID *resource.ID
	Mergeable      bool
	// Anchor is the date the schedule counts from and Periods the invoices
	// made since, so month ends stay month ends: 31 January, 28 February,
	// 31 March.
	Anchor  time.Time
	Periods int
	// Generated counts every invoice made.
	Generated int
	// SubscriptionID is the subscription the recurring invoice bills; the
	// store fills it in.
	SubscriptionID *resource.ID
	Lines          []Detail
	CustomFields   []FieldValue
	Notes          []notes.Note
	// Totals are the amounts each invoice will have.
	Totals    Totals
	CreatedAt time.Time
	UpdatedAt time.Time
}

// Ref locates the recurring invoice.
func (r Recurring) Ref() resource.Ref {
	return resource.Ref{ID: r.ID, Type: resource.TypeRecurringSalesInvoice, AdministrationID: r.AdministrationID}
}

// Version changes whenever the recurring invoice does.
func (r Recurring) Version() int64 { return r.UpdatedAt.Unix() }

// invoice is the recurring invoice as the invoice code works on it.
func (r Recurring) invoice() Invoice {
	day := r.InvoiceDate
	return Invoice{
		AdministrationID: r.AdministrationID, ContactID: r.ContactID, ContactPersonID: r.ContactPersonID, IdentityID: r.IdentityID,
		DocumentStyleID: r.DocumentStyleID, WorkflowID: r.WorkflowID, Reference: r.Reference, Language: r.Language, Currency: r.Currency,
		PricesAreInclTax: r.PricesAreInclTax, PaymentConditions: r.PaymentConditions, Discount: r.Discount, FirstDueInterval: r.FirstDueInterval,
		InvoiceDate: &day, State: Draft, Lines: slices.Clone(r.Lines), CustomFields: slices.Clone(r.CustomFields), Totals: r.Totals,
	}
}

// take copies what the invoice code changed of inv, as invoice() returned it.
func (r *Recurring) take(inv Invoice) {
	r.ContactID, r.ContactPersonID, r.IdentityID, r.DocumentStyleID, r.WorkflowID = inv.ContactID, inv.ContactPersonID, inv.IdentityID, inv.DocumentStyleID, inv.WorkflowID
	r.Reference, r.Language, r.Currency, r.PricesAreInclTax = inv.Reference, inv.Language, inv.Currency, inv.PricesAreInclTax
	r.PaymentConditions, r.Discount, r.FirstDueInterval = inv.PaymentConditions, inv.Discount, inv.FirstDueInterval
	r.Lines, r.CustomFields = inv.Lines, inv.CustomFields
}

// Occurrence is the date n periods of every frequencyType after anchor.
// Months, quarters and years keep the anchor's day, or the month's last day
// when the month is shorter.
func Occurrence(anchor time.Time, frequencyType string, every, n int) time.Time {
	steps := every * n
	switch frequencyType {
	case Day:
		return anchor.AddDate(0, 0, steps)
	case Week:
		return anchor.AddDate(0, 0, 7*steps)
	case Quarter:
		return addMonths(anchor, 3*steps)
	case Year:
		return addMonths(anchor, 12*steps)
	default:
		return addMonths(anchor, steps)
	}
}

func addMonths(t time.Time, months int) time.Time {
	first := time.Date(t.Year(), t.Month()+time.Month(months), 1, 0, 0, 0, 0, t.Location())
	last := first.AddDate(0, 1, -1).Day()
	return first.AddDate(0, 0, min(t.Day(), last)-1)
}

// RecurringChanges holds the fields to set; nil fields stay as they are.
// LastDate sets the end directly; HasDesiredCount with DesiredCount sets it
// to the date of that many invoices in all, as Moneybird does.
type RecurringChanges struct {
	ContactID         *resource.ID
	ContactPersonID   *resource.ID
	DocumentStyleID   *resource.ID
	WorkflowID        *resource.ID
	IdentityID        *resource.ID
	Reference         *string
	PaymentConditions *string
	Currency          *string
	Discount          *string
	FirstDueInterval  *int
	PricesAreInclTax  *bool
	InvoiceDate       *string
	FrequencyType     *string
	Frequency         *int
	HasDesiredCount   *bool
	DesiredCount      *int
	LastDate          *string
	AutoSend          *bool
	Mergeable         *bool
	Details           []DetailInput
	CustomFields      []FieldValue
}

func (c RecurringChanges) invoiceChanges() Changes {
	return Changes{
		ContactID: c.ContactID, ContactPersonID: c.ContactPersonID, DocumentStyleID: c.DocumentStyleID, WorkflowID: c.WorkflowID,
		IdentityID: c.IdentityID, Reference: c.Reference, FirstDueInterval: c.FirstDueInterval, Currency: c.Currency,
		PricesAreInclTax: c.PricesAreInclTax, PaymentConditions: c.PaymentConditions, Discount: c.Discount, Details: c.Details,
		CustomFields: c.CustomFields,
	}
}

// CreateRecurring adds a recurring invoice, monthly and starting today
// unless the changes say otherwise, filled from the administration's
// defaults like a new invoice.
func (s *Service) CreateRecurring(ctx context.Context, admin, user resource.ID, c RecurringChanges) (Recurring, error) {
	a, err := s.admins.Get(ctx, admin, user)
	if err != nil {
		return Recurring{}, err
	}
	now, today := s.now(), s.today(a)
	r := Recurring{
		ID: s.ids.Next(), AdministrationID: admin, Active: true, Currency: a.Currency, Language: a.Language, FrequencyType: Month, Frequency: 1,
		StartDate: today, InvoiceDate: today, Anchor: today, CreatedAt: now, UpdatedAt: now,
	}
	inv := r.invoice()
	if err := s.defaults(ctx, a, user, &inv, c.invoiceChanges()); err != nil {
		return Recurring{}, err
	}
	r.take(inv)
	if err := s.applyRecurring(ctx, a, user, &r, c, true); err != nil {
		return Recurring{}, err
	}
	if err := s.saveRecurring(ctx, admin, user, &r, true); err != nil {
		return Recurring{}, err
	}
	return s.Recurring(ctx, admin, r.ID, user)
}

// UpdateRecurring changes a recurring invoice. A new invoice date restarts
// the schedule from it; it must not lie in the past.
func (s *Service) UpdateRecurring(ctx context.Context, admin, id, user resource.ID, c RecurringChanges) (Recurring, error) {
	a, err := s.admins.Get(ctx, admin, user)
	if err != nil {
		return Recurring{}, err
	}
	r, err := s.store.RecurringInvoice(ctx, admin, id)
	if err != nil {
		return Recurring{}, err
	}
	if err := s.applyRecurring(ctx, a, user, &r, c, false); err != nil {
		return Recurring{}, err
	}
	r.UpdatedAt = s.now()
	if err := s.saveRecurring(ctx, admin, user, &r, false); err != nil {
		return Recurring{}, err
	}
	return s.Recurring(ctx, admin, id, user)
}

func (s *Service) saveRecurring(ctx context.Context, admin, user resource.ID, r *Recurring, created bool) error {
	tot, err := s.Totals(ctx, admin, user, r.invoice())
	if err != nil {
		return err
	}
	r.Totals = tot
	return wrap("store recurring invoice", s.store.SaveRecurring(ctx, *r, created))
}

func (s *Service) applyRecurring(ctx context.Context, a administration.Listed, user resource.ID, r *Recurring, c RecurringChanges, created bool) error {
	inv := r.invoice()
	var verr resource.ValidationError
	if err := s.apply(ctx, a, user, &inv, c.invoiceChanges()); err != nil {
		var invalid *resource.ValidationError
		if !errors.As(err, &invalid) {
			return err
		}
		verr = *invalid
	}
	r.take(inv)
	if len(r.Lines) == 0 {
		verr.Add(detailsField, "can't be empty; add a line")
	}
	rescheduled := false
	if c.FrequencyType != nil {
		if t := strings.TrimSpace(*c.FrequencyType); !slices.Contains(FrequencyTypes, t) {
			verr.Add("frequency_type", "is not one of "+strings.Join(FrequencyTypes, ", "))
		} else if t != r.FrequencyType {
			r.FrequencyType, rescheduled = t, true
		}
	}
	if c.Frequency != nil {
		if *c.Frequency < 1 || *c.Frequency > maxFrequency {
			verr.Add("frequency", fmt.Sprintf("must be from 1 to %d", maxFrequency))
		} else if *c.Frequency != r.Frequency {
			r.Frequency, rescheduled = *c.Frequency, true
		}
	}
	if c.InvoiceDate != nil {
		d, err := time.Parse(time.DateOnly, strings.TrimSpace(*c.InvoiceDate))
		switch {
		case err != nil:
			verr.Add("invoice_date", "is not a date")
		case d.Before(s.today(a)):
			verr.Add("invoice_date", "must not lie in the past")
		default:
			r.InvoiceDate, rescheduled = d, true
			if created {
				r.StartDate = d
			}
		}
	}
	if rescheduled {
		r.Anchor, r.Periods = r.InvoiceDate, 0
	}
	if c.AutoSend != nil {
		if *c.AutoSend && !r.AutoSend {
			at := s.now()
			r.AutoSendAt, r.AutoSendUserID = &at, &user
		}
		if !*c.AutoSend {
			r.AutoSendAt, r.AutoSendUserID = nil, nil
		}
		r.AutoSend = *c.AutoSend
	}
	if c.Mergeable != nil {
		r.Mergeable = *c.Mergeable
	}
	switch {
	case c.LastDate != nil:
		r.LastDate = nil
		if v := strings.TrimSpace(*c.LastDate); v != "" {
			if d, err := time.Parse(time.DateOnly, v); err != nil {
				verr.Add("last_date", "is not a date")
			} else {
				r.LastDate = &d
			}
		}
	case c.HasDesiredCount != nil && *c.HasDesiredCount:
		remaining := deref(c.DesiredCount) - r.Generated
		if remaining < 1 {
			verr.Add("desired_count", fmt.Sprintf("must be more than the %d invoices made so far", r.Generated))
			break
		}
		last := Occurrence(r.Anchor, r.FrequencyType, r.Frequency, r.Periods+remaining-1)
		r.LastDate = &last
	case c.HasDesiredCount != nil:
		r.LastDate = nil
	}
	if r.LastDate != nil && r.LastDate.Before(r.InvoiceDate) {
		verr.Add("last_date", "must not lie before the next invoice date")
	}
	return verr.OrNil()
}

func deref[T any](p *T) T {
	var zero T
	if p == nil {
		return zero
	}
	return *p
}

// DeactivateRecurring stops a recurring invoice for good; the invoices it
// made stay.
func (s *Service) DeactivateRecurring(ctx context.Context, admin, id, user resource.ID) (Recurring, error) {
	if _, err := s.admins.Get(ctx, admin, user); err != nil {
		return Recurring{}, err
	}
	r, err := s.store.RecurringInvoice(ctx, admin, id)
	if err != nil {
		return Recurring{}, err
	}
	if !r.Active {
		return Recurring{}, ErrNotEditable
	}
	r.Active, r.UpdatedAt = false, s.now()
	if err := wrap("store recurring invoice", s.store.SaveRecurring(ctx, r, false)); err != nil {
		return Recurring{}, err
	}
	return s.Recurring(ctx, admin, id, user)
}

// DeleteRecurring removes a recurring invoice that made no invoices yet;
// one that did is deactivated instead, and kept reports so. A deactivated
// one that made invoices cannot go.
func (s *Service) DeleteRecurring(ctx context.Context, admin, id, user resource.ID) (kept bool, err error) {
	if _, err := s.admins.Get(ctx, admin, user); err != nil {
		return false, err
	}
	r, err := s.store.RecurringInvoice(ctx, admin, id)
	if err != nil {
		return false, err
	}
	if r.Generated == 0 {
		return false, wrap("delete recurring invoice", s.store.DeleteRecurring(ctx, admin, id))
	}
	_, err = s.DeactivateRecurring(ctx, admin, id, user)
	return err == nil, err
}

// RecurringQuery selects recurring invoices, after Moneybird's filter.
type RecurringQuery struct {
	// Active keeps active or inactive ones; nil keeps both.
	Active        *bool
	FrequencyType string
	AutoSend      *bool
	ContactID     *resource.ID
	WorkflowID    *resource.ID
	// IncludeSubscriptions keeps the recurring invoices that bill a
	// subscription too; stores leave them out otherwise.
	IncludeSubscriptions bool
	Page, PerPage        int
}

// Matches reports whether the query selects the recurring invoice; the
// reference for stores without a query language of their own. It does not
// know subscriptions, which stores filter themselves.
func (q RecurringQuery) Matches(r Recurring) bool {
	switch {
	case q.Active != nil && r.Active != *q.Active,
		q.FrequencyType != "" && r.FrequencyType != q.FrequencyType,
		q.AutoSend != nil && r.AutoSend != *q.AutoSend,
		q.ContactID != nil && r.ContactID != *q.ContactID,
		q.WorkflowID != nil && r.WorkflowID != *q.WorkflowID:
		return false
	}
	return true
}

// ParseRecurringFilter reads Moneybird's recurring sales invoice filter:
// state (active, inactive or, as an extension, all; active unless given),
// frequency, auto_send, contact_id, workflow_id and include_subscriptions,
// which keeps the recurring invoices of subscriptions too (false unless
// given).
func ParseRecurringFilter(filter string) (RecurringQuery, error) {
	active := true
	q := RecurringQuery{Active: &active}
	var verr resource.ValidationError
	for term := range strings.SplitSeq(filter, ",") {
		term = strings.TrimSpace(term)
		if term == "" {
			continue
		}
		key, value, ok := strings.Cut(term, ":")
		if !ok {
			verr.Add("filter", "has a term without a value: "+term)
			continue
		}
		switch key {
		case "state":
			switch value {
			case "active":
			case "inactive":
				q.Active = ptrTo(false)
			case "all":
				q.Active = nil
			default:
				verr.Add("filter", "state "+value+" is not one of active, inactive, all")
			}
		case "frequency":
			switch {
			case value == "all":
			case slices.Contains(FrequencyTypes, value):
				q.FrequencyType = value
			default:
				verr.Add("filter", "frequency "+value+" is not one of all, "+strings.Join(FrequencyTypes, ", "))
			}
		case "auto_send":
			switch value {
			case "null":
			case "true", "false":
				q.AutoSend = ptrTo(value == "true")
			default:
				verr.Add("filter", "auto_send is not true, false or null")
			}
		case "include_subscriptions":
			switch value {
			case "true", "false":
				q.IncludeSubscriptions = value == "true"
			default:
				verr.Add("filter", "include_subscriptions is not true or false")
			}
		case "contact_id", "workflow_id":
			id, err := resource.ParseID(value)
			if err != nil {
				verr.Add("filter", key+" is not an id")
				continue
			}
			if key == "contact_id" {
				q.ContactID = &id
			} else {
				q.WorkflowID = &id
			}
		default:
			verr.Add("filter", "has an unknown key: "+key)
		}
	}
	return q, verr.OrNil()
}

func ptrTo[T any](v T) *T { return &v }

// RecurringList returns the page of recurring invoices the query selects,
// the next to make first.
func (s *Service) RecurringList(ctx context.Context, admin, user resource.ID, q RecurringQuery) ([]Recurring, int, error) {
	if _, err := s.admins.Get(ctx, admin, user); err != nil {
		return nil, 0, err
	}
	q.Page = max(q.Page, 1)
	if q.PerPage <= 0 {
		q.PerPage = DefaultPerPage
	}
	q.PerPage = min(q.PerPage, MaxPerPage)
	list, total, err := s.store.RecurringInvoices(ctx, admin, q)
	if err != nil {
		return nil, 0, fmt.Errorf("list recurring invoices: %w", err)
	}
	list, err = s.completeRecurring(ctx, admin, list)
	return list, total, err
}

// RecurringVersions lists the recurring invoices the query selects, for
// synchronization.
func (s *Service) RecurringVersions(ctx context.Context, admin, user resource.ID, q RecurringQuery) ([]Recurring, error) {
	if _, err := s.admins.Get(ctx, admin, user); err != nil {
		return nil, err
	}
	q.PerPage = 0
	list, _, err := s.store.RecurringInvoices(ctx, admin, q)
	return list, err
}

// RecurringByID returns the recurring invoices with these ids.
func (s *Service) RecurringByID(ctx context.Context, admin, user resource.ID, ids []resource.ID) ([]Recurring, error) {
	if _, err := s.admins.Get(ctx, admin, user); err != nil {
		return nil, err
	}
	if len(ids) > MaxPerPage {
		return nil, invalid("ids", fmt.Sprintf("can hold at most %d ids", MaxPerPage))
	}
	list, err := s.store.RecurringInvoicesByID(ctx, admin, ids)
	if err != nil {
		return nil, err
	}
	return s.completeRecurring(ctx, admin, list)
}

// Recurring returns one recurring invoice.
func (s *Service) Recurring(ctx context.Context, admin, id, user resource.ID) (Recurring, error) {
	if _, err := s.admins.Get(ctx, admin, user); err != nil {
		return Recurring{}, err
	}
	r, err := s.store.RecurringInvoice(ctx, admin, id)
	if err != nil {
		return Recurring{}, err
	}
	list, err := s.completeRecurring(ctx, admin, []Recurring{r})
	if err != nil {
		return Recurring{}, err
	}
	return list[0], nil
}

func (s *Service) completeRecurring(ctx context.Context, admin resource.ID, list []Recurring) ([]Recurring, error) {
	ids := make([]resource.ID, len(list))
	for i, r := range list {
		ids[i] = r.ID
	}
	byID, err := s.notes.Of(ctx, admin, RecurringEntityType, ids)
	if err != nil {
		return nil, err
	}
	for i := range list {
		list[i].Notes = byID[list[i].ID]
	}
	return list, nil
}

// AddRecurringNote adds a note to a recurring invoice.
func (s *Service) AddRecurringNote(ctx context.Context, admin, id, user resource.ID, in notes.Input) (notes.Note, error) {
	if _, err := s.Recurring(ctx, admin, id, user); err != nil {
		return notes.Note{}, err
	}
	n, err := s.notes.Add(ctx, admin, RecurringEntityType, id, user, in)
	if err != nil {
		return notes.Note{}, err
	}
	return n, s.store.TouchRecurring(ctx, admin, id, n.CreatedAt)
}

// DeleteRecurringNote removes a note from a recurring invoice.
func (s *Service) DeleteRecurringNote(ctx context.Context, admin, id, noteID, user resource.ID) error {
	if _, err := s.Recurring(ctx, admin, id, user); err != nil {
		return err
	}
	if err := s.notes.Delete(ctx, admin, RecurringEntityType, id, noteID); err != nil {
		return err
	}
	return s.store.TouchRecurring(ctx, admin, id, s.now())
}

// recur makes the invoices of the administration's active recurring
// invoices that are due, catching up on every date missed, and sends those
// that auto-send. Each invoice is stored with the recurring invoice moved
// on to its next date at once, so a next round never makes it again.
func (s *Service) recur(ctx context.Context, a administration.Listed, user resource.ID, today time.Time) []error {
	list, _, err := s.store.RecurringInvoices(ctx, a.ID, RecurringQuery{Active: ptrTo(true), IncludeSubscriptions: true})
	if err != nil {
		return []error{fmt.Errorf("administration %s, list recurring invoices: %w", a.ID, err)}
	}
	var errs []error
	for _, r := range list {
		for r.Active && !r.InvoiceDate.After(today) && ctx.Err() == nil {
			inv, err := s.generate(ctx, a, user, &r)
			if err != nil {
				errs = append(errs, fmt.Errorf("administration %s, recurring invoice %s: %w", a.ID, r.ID, err))
				break
			}
			s.notify(ctx, a.ID, "sales_invoice_created", inv)
			s.notifyRecurring(ctx, a.ID, "recurring_sales_invoice_invoice_created", r)
			if s.hook != nil {
				completed, err := s.hook.Generated(ctx, a.ID, user, inv, r)
				if err != nil {
					errs = append(errs, fmt.Errorf("administration %s, complete invoice %s of recurring invoice %s: %w", a.ID, inv.ID, r.ID, err))
				} else {
					inv = completed
				}
			}
			if !r.AutoSend {
				continue
			}
			if err := s.sendInBackground(ctx, a.ID, user, inv, SendInput{InvoiceDate: inv.InvoiceDate.Format(time.DateOnly)}); err != nil {
				errs = append(errs, fmt.Errorf("administration %s, auto-send invoice %s of recurring invoice %s: %w", a.ID, inv.ID, r.ID, err))
			}
		}
	}
	return errs
}

// generate makes the draft invoice of r's next date and moves r on.
func (s *Service) generate(ctx context.Context, a administration.Listed, user resource.ID, r *Recurring) (Invoice, error) {
	day := r.InvoiceDate
	next := Occurrence(r.Anchor, r.FrequencyType, r.Frequency, r.Periods+1)
	end := next.AddDate(0, 0, -1)
	period := day.Format("20060102") + ".." + end.Format("20060102")
	text := day.Format(time.DateOnly) + " – " + end.Format(time.DateOnly)
	now := s.now()
	inv := r.invoice()
	inv.ID, inv.RecurringSalesInvoiceID, inv.CreatedAt, inv.UpdatedAt = s.ids.Next(), &r.ID, now, now
	inv.InvoiceDate = &day
	for i := range inv.Lines {
		d := &inv.Lines[i]
		d.ID, d.CreatedAt, d.UpdatedAt = s.ids.Next(), now, now
		d.Description = strings.ReplaceAll(d.Description, "{period}", text)
		if d.Period != "" {
			d.Period = period
		}
	}
	var err error
	if inv.DraftID, err = s.store.NextDraftID(ctx, a.ID); err != nil {
		return Invoice{}, wrap("number draft", err)
	}
	if inv.Totals, err = s.Totals(ctx, a.ID, user, inv); err != nil {
		return Invoice{}, err
	}
	moved := *r
	moved.Generated, moved.Periods, moved.InvoiceDate, moved.UpdatedAt = r.Generated+1, r.Periods+1, next, now
	if moved.LastDate != nil && next.After(*moved.LastDate) {
		moved.Active = false
	}
	if err := s.store.SaveGenerated(ctx, inv, moved); err != nil {
		return Invoice{}, wrap("store generated invoice", err)
	}
	*r = moved
	return inv, nil
}

// RecurringHook completes an invoice a recurring invoice made, before it is
// sent, such as with what a subscription adds to it.
type RecurringHook interface {
	// Generated returns the invoice as it is to be sent.
	Generated(ctx context.Context, admin, user resource.ID, inv Invoice, r Recurring) (Invoice, error)
}

// UseRecurringHook sets the hook invoices recurring invoices make pass
// through; it is set after construction because the hook uses the service.
func (s *Service) UseRecurringHook(h RecurringHook) { s.hook = h }

// notifyRecurring tells the notifier, if there is one.
func (s *Service) notifyRecurring(ctx context.Context, admin resource.ID, action string, r Recurring) {
	if s.notifier != nil {
		s.notifier.NotifyRecurring(ctx, admin, action, r)
	}
}
