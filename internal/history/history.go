// Package history keeps what happened to records such as sales invoices,
// after Moneybird's events: who did what and when, with the record it links
// to (a payment, a credit invoice) and details such as what changed. The
// record's own service decides what to record and checks access.
package history

import (
	"context"
	"fmt"
	"slices"
	"time"

	"github.com/adirkuhn/moneyharpy/internal/resource"
)

// Event is something that happened to a record.
type Event struct {
	ID               resource.ID
	AdministrationID resource.ID
	// EntityType names the kind of record, as Moneybird does: SalesInvoice.
	EntityType string
	EntityID   resource.ID
	// UserID is who did it; the background job acts as the
	// administration's owner.
	UserID resource.ID
	// Action is Moneybird's name of the event, such as sales_invoice_created.
	Action string
	// LinkEntityType and LinkEntityID name a record the event is about,
	// such as the Payment registered.
	LinkEntityType string
	LinkEntityID   *resource.ID
	// Data tells more, such as the email address an invoice went to.
	Data      map[string]any
	CreatedAt time.Time
	UpdatedAt time.Time
}

// Store persists events.
type Store interface {
	InsertEvents(ctx context.Context, events []Event) error
	// EventsOf returns the events of the records, oldest first.
	EventsOf(ctx context.Context, admin resource.ID, entityType string, entities []resource.ID) ([]Event, error)
}

// Service implements recording and reading events.
type Service struct {
	store Store
	ids   *resource.Generator
	now   func() time.Time
}

// NewService returns a Service.
func NewService(store Store, ids *resource.Generator, now func() time.Time) *Service {
	return &Service{store: store, ids: ids, now: now}
}

// Record stores events as happening now, in the order given: each is a
// microsecond after the one before, so they read back in that order.
func (s *Service) Record(ctx context.Context, events ...Event) error {
	if len(events) == 0 {
		return nil
	}
	at := s.now()
	list := make([]Event, len(events))
	for i, e := range events {
		e.ID = s.ids.Next()
		e.CreatedAt = at.Add(time.Duration(i) * time.Microsecond)
		e.UpdatedAt = e.CreatedAt
		if e.Data == nil {
			e.Data = map[string]any{}
		}
		list[i] = e
	}
	if err := s.store.InsertEvents(ctx, list); err != nil {
		return fmt.Errorf("store events: %w", err)
	}
	return nil
}

// Of returns the events of each record, oldest first.
func (s *Service) Of(ctx context.Context, admin resource.ID, entityType string, entities []resource.ID) (map[resource.ID][]Event, error) {
	out := map[resource.ID][]Event{}
	if len(entities) == 0 {
		return out, nil
	}
	list, err := s.store.EventsOf(ctx, admin, entityType, entities)
	if err != nil {
		return nil, fmt.Errorf("load events: %w", err)
	}
	slices.SortStableFunc(list, func(a, b Event) int { return a.CreatedAt.Compare(b.CreatedAt) })
	for _, e := range list {
		out[e.EntityID] = append(out[e.EntityID], e)
	}
	return out, nil
}
