package sales

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/adirkuhn/moneyharpy/internal/history"
	"github.com/adirkuhn/moneyharpy/internal/invoicing"
	"github.com/adirkuhn/moneyharpy/internal/money"
	"github.com/adirkuhn/moneyharpy/internal/resource"
)

// Query selects invoices, after Moneybird's filter.
type Query struct {
	// States keeps invoices in one of them; late is worked out from the
	// due date, so open invoices past it count as late, not open.
	States                  []string
	Period                  *money.Period
	References              []string
	ContactID               *resource.ID
	RecurringSalesInvoiceID *resource.ID
	WorkflowID              *resource.ID
	CreatedAfter            *time.Time
	UpdatedAfter            *time.Time
	Ascending               bool
	Page, PerPage           int
	// Today decides which open invoices are late.
	Today time.Time
}

// ParseFilter reads Moneybird's sales invoice filter. Without a filter the
// list holds this year's invoices.
func ParseFilter(filter string, today time.Time) (Query, error) {
	if strings.TrimSpace(filter) == "" {
		filter = "period:this_year"
	}
	return parseTerms(filter, today, States)
}

// parseTerms reads filter terms; states are those a state term may name.
func parseTerms(filter string, today time.Time, states []string) (Query, error) {
	q := Query{Today: today}
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
			for st := range strings.SplitSeq(value, "|") {
				switch {
				case st == "all":
				case slices.Contains(states, st):
					q.States = append(q.States, st)
				default:
					verr.Add("filter", "state "+st+" is not one of all, "+strings.Join(states, ", "))
				}
			}
		case "period":
			p, err := money.ParsePeriod(value, today)
			if err != nil {
				verr.Add("filter", "period "+err.Error())
				continue
			}
			q.Period = &p
		case "reference":
			q.References = strings.Split(value, "|")
		case "contact_id", "recurring_sales_invoice_id", "workflow_id":
			id, err := resource.ParseID(value)
			if err != nil {
				verr.Add("filter", key+" is not an id")
				continue
			}
			switch key {
			case "contact_id":
				q.ContactID = &id
			case "recurring_sales_invoice_id":
				q.RecurringSalesInvoiceID = &id
			default:
				q.WorkflowID = &id
			}
		case "created_after", "updated_after":
			t, err := time.Parse(time.RFC3339, value)
			if err != nil {
				verr.Add("filter", key+" is not an ISO 8601 time")
				continue
			}
			if key == "created_after" {
				q.CreatedAfter = &t
			} else {
				q.UpdatedAfter = &t
			}
		default:
			verr.Add("filter", "has an unknown key: "+key)
		}
	}
	return q, verr.OrNil()
}

// Matches reports whether the query selects the invoice; the reference for
// stores without a query language of their own.
func (q Query) Matches(inv Invoice) bool {
	state := effective(inv, q.Today)
	day := inv.InvoiceDate
	if day == nil {
		day = &inv.CreatedAt
	}
	switch {
	case len(q.States) > 0 && !slices.Contains(q.States, state),
		q.Period != nil && !q.Period.Contains(*day),
		len(q.References) > 0 && !slices.Contains(q.References, inv.Reference),
		q.ContactID != nil && inv.ContactID != *q.ContactID,
		q.RecurringSalesInvoiceID != nil && (inv.RecurringSalesInvoiceID == nil || *inv.RecurringSalesInvoiceID != *q.RecurringSalesInvoiceID),
		q.WorkflowID != nil && inv.WorkflowID != *q.WorkflowID,
		q.CreatedAfter != nil && !inv.CreatedAt.After(*q.CreatedAfter),
		q.UpdatedAfter != nil && !inv.UpdatedAt.After(*q.UpdatedAfter):
		return false
	}
	return true
}

// MatchesExternal reports whether the query selects the external invoice.
func (q Query) MatchesExternal(e ExternalInvoice) bool { return q.Matches(e.invoice()) }

// Page sizes.
const (
	DefaultPerPage = 50
	MaxPerPage     = 100
)

// List returns the page of invoices the query selects.
func (s *Service) List(ctx context.Context, admin, user resource.ID, q Query) ([]Invoice, int, error) {
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
	list, total, err := s.store.Invoices(ctx, admin, q)
	if err != nil {
		return nil, 0, fmt.Errorf("list invoices: %w", err)
	}
	list, err = s.complete(ctx, a.ID, user, list, q.Today)
	return list, total, err
}

// Versions lists the invoices the query selects, for synchronization.
func (s *Service) Versions(ctx context.Context, admin, user resource.ID, q Query) ([]Invoice, error) {
	a, err := s.admins.Get(ctx, admin, user)
	if err != nil {
		return nil, err
	}
	q.Today, q.PerPage = s.today(a), 0
	list, _, err := s.store.Invoices(ctx, admin, q)
	return list, err
}

// InvoicesByID returns the invoices with these ids.
func (s *Service) InvoicesByID(ctx context.Context, admin, user resource.ID, ids []resource.ID) ([]Invoice, error) {
	a, err := s.admins.Get(ctx, admin, user)
	if err != nil {
		return nil, err
	}
	if len(ids) > MaxPerPage {
		return nil, invalid("ids", fmt.Sprintf("can hold at most %d ids", MaxPerPage))
	}
	list, err := s.store.InvoicesByID(ctx, admin, ids)
	if err != nil {
		return nil, err
	}
	return s.complete(ctx, admin, user, list, s.today(a))
}

// Invoice returns one invoice.
func (s *Service) Invoice(ctx context.Context, admin, id, user resource.ID) (Invoice, error) {
	return s.one(ctx, admin, user, func() (Invoice, error) { return s.store.Invoice(ctx, admin, id) })
}

// InvoiceByNumber returns the invoice with an invoice number.
func (s *Service) InvoiceByNumber(ctx context.Context, admin, user resource.ID, number string) (Invoice, error) {
	return s.one(ctx, admin, user, func() (Invoice, error) { return s.store.InvoiceByNumber(ctx, admin, number) })
}

// InvoiceByReference returns the invoice with a reference.
func (s *Service) InvoiceByReference(ctx context.Context, admin, user resource.ID, reference string) (Invoice, error) {
	return s.one(ctx, admin, user, func() (Invoice, error) { return s.store.InvoiceByReference(ctx, admin, reference) })
}

func (s *Service) one(ctx context.Context, admin, user resource.ID, load func() (Invoice, error)) (Invoice, error) {
	a, err := s.admins.Get(ctx, admin, user)
	if err != nil {
		return Invoice{}, err
	}
	inv, err := load()
	if err != nil {
		return Invoice{}, err
	}
	list, err := s.complete(ctx, admin, user, []Invoice{inv}, s.today(a))
	if err != nil {
		return Invoice{}, err
	}
	return list[0], nil
}

// complete adds notes and attachments, shows late invoices as late and
// works out when the next reminder is due.
func (s *Service) complete(ctx context.Context, admin, user resource.ID, list []Invoice, today time.Time) ([]Invoice, error) {
	ids := make([]resource.ID, len(list))
	for i, inv := range list {
		ids[i] = inv.ID
	}
	byInvoice, err := s.notes.Of(ctx, admin, EntityType, ids)
	if err != nil {
		return nil, err
	}
	files, err := s.attachments.Of(ctx, admin, EntityType, ids)
	if err != nil {
		return nil, err
	}
	events := map[resource.ID][]history.Event{}
	if s.history != nil {
		if events, err = s.history.Of(ctx, admin, EntityType, ids); err != nil {
			return nil, err
		}
	}
	var flows map[resource.ID]invoicing.Workflow
	if slices.ContainsFunc(list, payable) {
		all, err := s.invoicing.Workflows(ctx, admin, user)
		if err != nil {
			return nil, wrap("load workflows", err)
		}
		flows = map[resource.ID]invoicing.Workflow{}
		for _, w := range all {
			flows[w.ID] = w
		}
	}
	for i := range list {
		list[i].Notes, list[i].Attachments, list[i].Events = byInvoice[list[i].ID], files[list[i].ID], events[list[i].ID]
		list[i].State = effective(list[i], today)
		if day, _, ok := nextReminder(list[i], flows[list[i].WorkflowID]); ok {
			list[i].NextReminder = &day
		} else if list[i].Paused && payable(list[i]) {
			day := PausedNextReminder
			list[i].NextReminder = &day
		}
	}
	return list, nil
}
