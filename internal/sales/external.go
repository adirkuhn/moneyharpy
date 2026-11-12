package sales

import (
	"context"
	"fmt"
	"io"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/adirkuhn/moneyharpy/internal/administration"
	"github.com/adirkuhn/moneyharpy/internal/attachments"
	"github.com/adirkuhn/moneyharpy/internal/history"
	"github.com/adirkuhn/moneyharpy/internal/journal"
	"github.com/adirkuhn/moneyharpy/internal/money"
	"github.com/adirkuhn/moneyharpy/internal/notes"
	"github.com/adirkuhn/moneyharpy/internal/resource"
)

// ExternalEntityType names external sales invoices in notes, attachments,
// payments and the journal.
const ExternalEntityType = journal.ExternalSalesInvoice

// New is the state of an external invoice uploaded as a file whose contact
// or lines are not filled in yet; it is not booked.
const New = "new"

// ExternalStates are the states of external invoices, for filters.
var ExternalStates = []string{New, Open, Late, Paid, Uncollectible}

// Origins of external invoices, with Moneybird's names.
const (
	OriginUpload = "upload"
	OriginAPI    = "api"
)

// ExternalInvoice is a sales invoice made in other software, such as a
// webshop or a till: MoneyHarpy books it and follows its payment, but does
// not number, render or send it.
type ExternalInvoice struct {
	ID               resource.ID
	AdministrationID resource.ID
	// ContactID is zero while a new invoice has no contact yet.
	ContactID resource.ID
	// Reference is the invoice's number in the other software.
	Reference string
	// EntryNumber numbers the administration's external invoices.
	EntryNumber      int
	Date             time.Time
	DueDate          *time.Time
	State            string
	Origin           string
	Source           string
	SourceURL        string
	Currency         string
	PricesAreInclTax bool
	PaidAt           *time.Time
	MarkedDubiousOn  *time.Time
	// MarkedUncollectibleOn is when what was open was written off.
	MarkedUncollectibleOn *time.Time
	Lines                 []Detail
	Payments              []Payment
	Notes                 []notes.Note
	Attachments           []attachments.Attachment
	// Events are what happened to the invoice, oldest first.
	Events []history.Event
	// Totals are the amounts worked out when the invoice was last saved.
	Totals    Totals
	CreatedAt time.Time
	UpdatedAt time.Time
}

// Ref locates the invoice.
func (e ExternalInvoice) Ref() resource.Ref {
	return resource.Ref{ID: e.ID, Type: resource.TypeExternalSalesInvoice, AdministrationID: e.AdministrationID}
}

// Version changes whenever the invoice does.
func (e ExternalInvoice) Version() int64 { return e.UpdatedAt.Unix() }

// Paid is what payments settled.
func (e ExternalInvoice) Paid() money.Amount { return e.invoice().Paid() }

// Unpaid is what is still to be paid.
func (e ExternalInvoice) Unpaid() money.Amount { return e.Totals.InclTax - e.Paid() }

// Booked reports an invoice that is in the books: anything but new.
func (e ExternalInvoice) Booked() bool { return e.State != New }

// complete reports an invoice with what booking needs.
func (e ExternalInvoice) complete() bool { return e.ContactID != 0 && len(e.Lines) > 0 }

// invoice is the external invoice as the payment and write-off code works
// on it.
func (e ExternalInvoice) invoice() Invoice {
	date := e.Date
	return Invoice{
		ID: e.ID, AdministrationID: e.AdministrationID, Type: ExternalEntityType, ContactID: e.ContactID, Reference: e.Reference,
		Currency: e.Currency, PricesAreInclTax: e.PricesAreInclTax, InvoiceDate: &date, DueDate: e.DueDate, State: e.State, PaidAt: e.PaidAt,
		MarkedDubiousOn: e.MarkedDubiousOn, MarkedUncollectibleOn: e.MarkedUncollectibleOn, Lines: e.Lines, Payments: e.Payments,
		Totals: e.Totals, CreatedAt: e.CreatedAt, UpdatedAt: e.UpdatedAt,
	}
}

// Settled copies what the payment and write-off code changed of inv, the
// external invoice as invoice() returned it, for stores.
func (e *ExternalInvoice) Settled(inv Invoice) {
	e.State, e.PaidAt, e.Payments, e.UpdatedAt = inv.State, inv.PaidAt, inv.Payments, inv.UpdatedAt
	e.MarkedDubiousOn, e.MarkedUncollectibleOn = inv.MarkedDubiousOn, inv.MarkedUncollectibleOn
}

// ExternalChanges holds the fields to set; nil fields stay as they are. A
// zero contact id clears the contact of a new invoice.
type ExternalChanges struct {
	ContactID        *resource.ID
	Reference        *string
	Date             *string
	DueDate          *string
	Currency         *string
	PricesAreInclTax *bool
	Source           *string
	SourceURL        *string
	Details          []DetailInput
}

// CreateExternal adds an external invoice. With a contact and lines it is
// booked at once; without, it is new.
func (s *Service) CreateExternal(ctx context.Context, admin, user resource.ID, c ExternalChanges) (ExternalInvoice, error) {
	a, err := s.admins.Get(ctx, admin, user)
	if err != nil {
		return ExternalInvoice{}, err
	}
	e, err := s.newExternal(ctx, a, OriginAPI)
	if err != nil {
		return ExternalInvoice{}, err
	}
	if err := s.applyExternal(ctx, a, user, &e, c); err != nil {
		return ExternalInvoice{}, err
	}
	if err := s.saveExternal(ctx, a, user, &e, true); err != nil {
		return ExternalInvoice{}, err
	}
	if err := s.record(ctx, user, on(e.invoice(), "external_sales_invoice_created")); err != nil {
		return ExternalInvoice{}, err
	}
	return s.ExternalInvoice(ctx, admin, e.ID, user)
}

// CreateExternalFromFile adds a new external invoice holding an uploaded
// file, such as a PDF from a webshop, to fill in later.
func (s *Service) CreateExternalFromFile(ctx context.Context, admin, user resource.ID, filename, contentType string, data []byte) (ExternalInvoice, error) {
	a, err := s.admins.Get(ctx, admin, user)
	if err != nil {
		return ExternalInvoice{}, err
	}
	e, err := s.newExternal(ctx, a, OriginUpload)
	if err != nil {
		return ExternalInvoice{}, err
	}
	if err := s.saveExternal(ctx, a, user, &e, true); err != nil {
		return ExternalInvoice{}, err
	}
	if _, err := s.attachments.Add(ctx, admin, ExternalEntityType, e.ID, filename, contentType, data); err != nil {
		if derr := s.store.DeleteExternalInvoice(ctx, admin, e.ID); derr != nil {
			return ExternalInvoice{}, fmt.Errorf("%w; remove the invoice again: %w", err, derr)
		}
		return ExternalInvoice{}, err
	}
	if err := s.record(ctx, user, on(e.invoice(), "external_sales_invoice_created")); err != nil {
		return ExternalInvoice{}, err
	}
	return s.ExternalInvoice(ctx, admin, e.ID, user)
}

func (s *Service) newExternal(ctx context.Context, a administration.Listed, origin string) (ExternalInvoice, error) {
	number, err := s.store.NextEntryNumber(ctx, a.ID)
	if err != nil {
		return ExternalInvoice{}, wrap("number external invoice", err)
	}
	now := s.now()
	return ExternalInvoice{
		ID: s.ids.Next(), AdministrationID: a.ID, EntryNumber: number, Date: s.today(a), State: New, Origin: origin, Currency: a.Currency,
		CreatedAt: now, UpdatedAt: now,
	}, nil
}

// UpdateExternal changes an external invoice and books it again. As at
// Moneybird one with payments changes too, keeping its currency, and what is
// left to pay sets its state; one written off as uncollectible, or in the
// locked period, stays as it is.
func (s *Service) UpdateExternal(ctx context.Context, admin, id, user resource.ID, c ExternalChanges) (ExternalInvoice, error) {
	a, err := s.admins.Get(ctx, admin, user)
	if err != nil {
		return ExternalInvoice{}, err
	}
	e, err := s.store.ExternalInvoice(ctx, admin, id)
	if err != nil {
		return ExternalInvoice{}, err
	}
	if e.State == Uncollectible {
		return ExternalInvoice{}, ErrNotEditable
	}
	if c.Currency != nil && len(e.Payments) > 0 && !strings.EqualFold(strings.TrimSpace(*c.Currency), e.Currency) {
		return ExternalInvoice{}, invalid("currency", "can't change on an invoice with payments")
	}
	booked, was := e.Booked(), effective(e.invoice(), s.today(a))
	if e.Booked() {
		if err := s.checkDay(ctx, admin, user, e.Date, "date"); err != nil {
			return ExternalInvoice{}, err
		}
	}
	if err := s.applyExternal(ctx, a, user, &e, c); err != nil {
		return ExternalInvoice{}, err
	}
	e.UpdatedAt = s.now()
	if err := s.saveExternal(ctx, a, user, &e, false); err != nil {
		return ExternalInvoice{}, err
	}
	// A new invoice is still being filled in; only changes to a booked one are history.
	var events []event
	if booked {
		events = append(events, on(e.invoice(), "external_sales_invoice_updated"))
	}
	if state := effective(e.invoice(), s.today(a)); booked && state != was {
		events = append(events, stateChanged(e.invoice(), state))
		s.notify(ctx, admin, "external_sales_invoice_state_changed_to_"+state, e.invoice())
	}
	if err := s.record(ctx, user, events...); err != nil {
		return ExternalInvoice{}, err
	}
	return s.ExternalInvoice(ctx, admin, id, user)
}

// saveExternal works out the totals and the state and stores the invoice
// with what it posts: nothing while it is new.
func (s *Service) saveExternal(ctx context.Context, a administration.Listed, user resource.ID, e *ExternalInvoice, created bool) error {
	tot, err := s.Totals(ctx, a.ID, user, e.invoice())
	if err != nil {
		return err
	}
	e.Totals = tot
	entries := []journal.Entry{}
	if e.complete() {
		if err := s.checkDay(ctx, a.ID, user, e.Date, "date"); err != nil {
			return err
		}
		inv := e.invoice()
		// A paid one stays paid while its payments cover it.
		if inv.State != Paid {
			inv.State = reopened(inv, s.today(a))
		}
		settleState(&inv, s.today(a))
		e.State, e.PaidAt = inv.State, inv.PaidAt
		if entries, err = s.post(ctx, a.ID, user, e.invoice()); err != nil {
			return err
		}
	}
	return wrap("store external invoice", s.store.SaveExternalInvoice(ctx, *e, entries, created))
}

func (s *Service) applyExternal(ctx context.Context, a administration.Listed, user resource.ID, e *ExternalInvoice, c ExternalChanges) error {
	var verr resource.ValidationError
	if c.ContactID != nil {
		e.ContactID = *c.ContactID
	}
	if e.ContactID == 0 {
		if e.Booked() {
			verr.Add("contact_id", "can't be blank")
		}
	} else if _, err := s.contacts.Contact(ctx, a.ID, e.ContactID, user); err != nil {
		verr.Add("contact_id", "does not exist")
	}
	for _, f := range []struct {
		src   *string
		dst   *string
		field string
	}{{c.Reference, &e.Reference, "reference"}, {c.Source, &e.Source, "source"}, {c.SourceURL, &e.SourceURL, "source_url"}} {
		if f.src == nil {
			continue
		}
		if *f.dst = strings.TrimSpace(*f.src); len(*f.dst) > 1000 {
			verr.Add(f.field, "is too long (maximum is 1000 characters)")
		}
	}
	if e.SourceURL != "" && !webURL(e.SourceURL) {
		verr.Add("source_url", "is not an http or https address")
	}
	if c.Currency != nil {
		e.Currency = strings.ToUpper(strings.TrimSpace(*c.Currency))
	}
	if !currencyPattern.MatchString(e.Currency) {
		verr.Add("currency", "is not a currency")
	}
	if c.PricesAreInclTax != nil {
		e.PricesAreInclTax = *c.PricesAreInclTax
	}
	if c.Date != nil {
		if d, err := time.Parse(time.DateOnly, strings.TrimSpace(*c.Date)); err != nil {
			verr.Add("date", "is not a date")
		} else {
			e.Date = d
		}
	}
	if c.DueDate != nil {
		e.DueDate = nil
		if v := strings.TrimSpace(*c.DueDate); v != "" {
			if d, err := time.Parse(time.DateOnly, v); err != nil {
				verr.Add("due_date", "is not a date")
			} else {
				e.DueDate = &d
			}
		}
	}
	if e.DueDate != nil && e.DueDate.Before(e.Date) {
		verr.Add("due_date", "must not be before the invoice date")
	}
	if c.Details != nil {
		inv := e.invoice()
		s.applyDetails(ctx, a.ID, user, &inv, c.Details, &verr)
		e.Lines = inv.Lines
		if e.Booked() && len(e.Lines) == 0 {
			verr.Add(detailsField, "can't be empty on a booked invoice")
		}
	}
	return verr.OrNil()
}

func webURL(raw string) bool {
	u, err := url.Parse(raw)
	return err == nil && (u.Scheme == "http" || u.Scheme == "https") && u.Host != ""
}

// DeleteExternal removes an external invoice with what it posts. Payments
// made with bank transactions are unlinked first, and settlements with
// other invoices undone, so the transactions and invoices are open again.
func (s *Service) DeleteExternal(ctx context.Context, admin, id, user resource.ID) error {
	a, err := s.admins.Get(ctx, admin, user)
	if err != nil {
		return err
	}
	e, err := s.store.ExternalInvoice(ctx, admin, id)
	if err != nil {
		return err
	}
	if e.Booked() {
		if err := s.checkDay(ctx, admin, user, e.Date, "date"); err != nil {
			return err
		}
	}
	for _, p := range e.Payments {
		if err := s.checkDay(ctx, admin, user, p.PaymentDate, "payment_date"); err != nil {
			return err
		}
	}
	for _, p := range e.Payments {
		switch {
		case p.FinancialMutationID != nil:
			if s.bank == nil {
				return ErrNotEditable
			}
			if err := s.bank.UnlinkPayment(ctx, admin, *p.FinancialMutationID, user, p.ID); err != nil {
				return err
			}
		case p.LinkedPaymentID != nil:
			current, err := s.loadInvoice(ctx, admin, ExternalEntityType, id)
			if err != nil {
				return err
			}
			if err := s.removePayment(ctx, a, user, current, p); err != nil {
				return err
			}
		}
	}
	if err := s.attachments.DeleteAll(ctx, admin, ExternalEntityType, id); err != nil {
		return wrap("delete attachments", err)
	}
	if err := s.store.DeleteExternalInvoice(ctx, admin, id); err != nil {
		return wrap("delete external invoice", err)
	}
	return nil
}

// ExternalList returns the page of external invoices the query selects.
func (s *Service) ExternalList(ctx context.Context, admin, user resource.ID, q Query) ([]ExternalInvoice, int, error) {
	a, err := s.admins.Get(ctx, admin, user)
	if err != nil {
		return nil, 0, err
	}
	q.Today = s.today(a)
	q.Page = max(q.Page, 1)
	if q.PerPage <= 0 {
		q.PerPage = DefaultPerPage
	}
	q.PerPage = min(q.PerPage, MaxPerPage)
	list, total, err := s.store.ExternalInvoices(ctx, admin, q)
	if err != nil {
		return nil, 0, fmt.Errorf("list external invoices: %w", err)
	}
	list, err = s.completeExternal(ctx, admin, list, q.Today)
	return list, total, err
}

// ExternalVersions lists the external invoices the query selects, for
// synchronization.
func (s *Service) ExternalVersions(ctx context.Context, admin, user resource.ID, q Query) ([]ExternalInvoice, error) {
	a, err := s.admins.Get(ctx, admin, user)
	if err != nil {
		return nil, err
	}
	q.Today, q.PerPage = s.today(a), 0
	list, _, err := s.store.ExternalInvoices(ctx, admin, q)
	return list, err
}

// ExternalInvoicesByID returns the external invoices with these ids.
func (s *Service) ExternalInvoicesByID(ctx context.Context, admin, user resource.ID, ids []resource.ID) ([]ExternalInvoice, error) {
	a, err := s.admins.Get(ctx, admin, user)
	if err != nil {
		return nil, err
	}
	if len(ids) > MaxPerPage {
		return nil, invalid("ids", fmt.Sprintf("can hold at most %d ids", MaxPerPage))
	}
	list, err := s.store.ExternalInvoicesByID(ctx, admin, ids)
	if err != nil {
		return nil, err
	}
	return s.completeExternal(ctx, admin, list, s.today(a))
}

// ExternalInvoice returns one external invoice.
func (s *Service) ExternalInvoice(ctx context.Context, admin, id, user resource.ID) (ExternalInvoice, error) {
	a, err := s.admins.Get(ctx, admin, user)
	if err != nil {
		return ExternalInvoice{}, err
	}
	e, err := s.store.ExternalInvoice(ctx, admin, id)
	if err != nil {
		return ExternalInvoice{}, err
	}
	list, err := s.completeExternal(ctx, admin, []ExternalInvoice{e}, s.today(a))
	if err != nil {
		return ExternalInvoice{}, err
	}
	return list[0], nil
}

// completeExternal adds notes and attachments and shows late invoices as
// late.
func (s *Service) completeExternal(ctx context.Context, admin resource.ID, list []ExternalInvoice, today time.Time) ([]ExternalInvoice, error) {
	ids := make([]resource.ID, len(list))
	for i, e := range list {
		ids[i] = e.ID
	}
	byInvoice, err := s.notes.Of(ctx, admin, ExternalEntityType, ids)
	if err != nil {
		return nil, err
	}
	files, err := s.attachments.Of(ctx, admin, ExternalEntityType, ids)
	if err != nil {
		return nil, err
	}
	events := map[resource.ID][]history.Event{}
	if s.history != nil {
		if events, err = s.history.Of(ctx, admin, ExternalEntityType, ids); err != nil {
			return nil, err
		}
	}
	for i := range list {
		list[i].Notes, list[i].Attachments, list[i].Events = byInvoice[list[i].ID], files[list[i].ID], events[list[i].ID]
		list[i].State = effective(list[i].invoice(), today)
	}
	return list, nil
}

// ParseExternalFilter reads Moneybird's external sales invoice filter:
// state, period and contact_id. Without a filter the list holds this
// year's invoices.
func ParseExternalFilter(filter string, today time.Time) (Query, error) {
	if strings.TrimSpace(filter) == "" {
		filter = "period:this_year"
	}
	for term := range strings.SplitSeq(filter, ",") {
		if key, _, _ := strings.Cut(strings.TrimSpace(term), ":"); key != "" && !slices.Contains([]string{"state", "period", "contact_id"}, key) {
			return Query{Today: today}, invalid("filter", "has an unknown key: "+key)
		}
	}
	return parseTerms(filter, today, ExternalStates)
}

// AddExternalNote adds a note to an external invoice.
func (s *Service) AddExternalNote(ctx context.Context, admin, id, user resource.ID, in notes.Input) (notes.Note, error) {
	if _, err := s.ExternalInvoice(ctx, admin, id, user); err != nil {
		return notes.Note{}, err
	}
	n, err := s.notes.Add(ctx, admin, ExternalEntityType, id, user, in)
	if err != nil {
		return notes.Note{}, err
	}
	return n, s.store.TouchExternalInvoice(ctx, admin, id, n.CreatedAt)
}

// DeleteExternalNote removes a note from an external invoice.
func (s *Service) DeleteExternalNote(ctx context.Context, admin, id, noteID, user resource.ID) error {
	if _, err := s.ExternalInvoice(ctx, admin, id, user); err != nil {
		return err
	}
	if err := s.notes.Delete(ctx, admin, ExternalEntityType, id, noteID); err != nil {
		return err
	}
	return s.store.TouchExternalInvoice(ctx, admin, id, s.now())
}

// AddExternalAttachment attaches a file to an external invoice.
func (s *Service) AddExternalAttachment(ctx context.Context, admin, id, user resource.ID, filename, contentType string, data []byte) (attachments.Attachment, error) {
	if _, err := s.ExternalInvoice(ctx, admin, id, user); err != nil {
		return attachments.Attachment{}, err
	}
	a, err := s.attachments.Add(ctx, admin, ExternalEntityType, id, filename, contentType, data)
	if err != nil {
		return attachments.Attachment{}, err
	}
	return a, s.store.TouchExternalInvoice(ctx, admin, id, a.CreatedAt)
}

// OpenExternalAttachment returns an attachment of an external invoice; the
// caller closes it.
func (s *Service) OpenExternalAttachment(ctx context.Context, admin, id, attachment, user resource.ID) (attachments.Attachment, io.ReadCloser, error) {
	if _, err := s.ExternalInvoice(ctx, admin, id, user); err != nil {
		return attachments.Attachment{}, nil, err
	}
	return s.attachments.Open(ctx, admin, ExternalEntityType, id, attachment)
}

// DeleteExternalAttachment removes an attachment from an external invoice.
func (s *Service) DeleteExternalAttachment(ctx context.Context, admin, id, attachment, user resource.ID) error {
	if _, err := s.ExternalInvoice(ctx, admin, id, user); err != nil {
		return err
	}
	if err := s.attachments.Delete(ctx, admin, ExternalEntityType, id, attachment); err != nil {
		return err
	}
	return s.store.TouchExternalInvoice(ctx, admin, id, s.now())
}
