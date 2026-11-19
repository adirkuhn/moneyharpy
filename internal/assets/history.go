package assets

import (
	"context"
	"fmt"

	"github.com/adirkuhn/moneyharpy/internal/history"
	"github.com/adirkuhn/moneyharpy/internal/notes"
	"github.com/adirkuhn/moneyharpy/internal/resource"
)

// EntityType names assets in events and notes, as Moneybird does.
const EntityType = "Asset"

// History keeps what happens to assets, as Moneybird's events.
type History interface {
	Record(ctx context.Context, events ...history.Event) error
	Of(ctx context.Context, admin resource.ID, entityType string, entities []resource.ID) (map[resource.ID][]history.Event, error)
}

// Notes keeps the notes and to-dos people write on assets.
type Notes interface {
	Add(ctx context.Context, admin resource.ID, entityType string, entity, user resource.ID, in notes.Input) (notes.Note, error)
	Delete(ctx context.Context, admin resource.ID, entityType string, entity, id resource.ID) error
	Of(ctx context.Context, admin resource.ID, entityType string, entities []resource.ID) (map[resource.ID][]notes.Note, error)
}

// UseHistory records what happens to assets.
func (s *Service) UseHistory(h History) { s.history = h }

// UseNotes lets people write notes on assets.
func (s *Service) UseNotes(n Notes) { s.notes = n }

// record notes that the user did action to the asset.
func (s *Service) record(ctx context.Context, admin, id, user resource.ID, action string, data map[string]any) error {
	if s.history == nil {
		return nil
	}
	e := history.Event{AdministrationID: admin, EntityType: EntityType, EntityID: id, UserID: user, Action: action, Data: data}
	if err := s.history.Record(ctx, e); err != nil {
		return fmt.Errorf("record event: %w", err)
	}
	return nil
}

// withHistory adds the asset's events and notes.
func (s *Service) withHistory(ctx context.Context, admin resource.ID, a *Asset) error {
	if s.history != nil {
		events, err := s.history.Of(ctx, admin, EntityType, []resource.ID{a.ID})
		if err != nil {
			return err
		}
		a.Events = events[a.ID]
	}
	if s.notes != nil {
		list, err := s.notes.Of(ctx, admin, EntityType, []resource.ID{a.ID})
		if err != nil {
			return err
		}
		a.Notes = list[a.ID]
	}
	return s.withBookings(ctx, admin, a)
}

// withBookings adds what each of the asset's sources books.
func (s *Service) withBookings(ctx context.Context, admin resource.ID, a *Asset) error {
	var details, entries []resource.ID
	for _, src := range a.Sources {
		if src.DetailID != nil {
			details = append(details, *src.DetailID)
		}
		if src.EntryID != nil {
			entries = append(entries, *src.EntryID)
		}
	}
	if len(details)+len(entries) == 0 {
		return nil
	}
	list, err := s.store.Bookings(ctx, admin, details, entries)
	if err != nil {
		return fmt.Errorf("load bookings: %w", err)
	}
	sources := append([]Source(nil), a.Sources...)
	for i, src := range sources {
		for _, b := range list {
			if src.DetailID != nil && b.DetailID != nil && *b.DetailID == *src.DetailID || src.EntryID != nil && b.EntryID != nil && *b.EntryID == *src.EntryID {
				sources[i].Booking = &b
			}
		}
	}
	a.Sources = sources
	return nil
}

// Linkable returns the bookings on the asset's category that no asset
// links yet, for linking as its sources.
func (s *Service) Linkable(ctx context.Context, admin, id, user resource.ID) ([]Booking, error) {
	a, err := s.Asset(ctx, admin, id, user)
	if err != nil {
		return nil, err
	}
	list, err := s.store.Linkable(ctx, admin, a.LedgerAccountID)
	if err != nil {
		return nil, fmt.Errorf("load linkable bookings: %w", err)
	}
	return list, nil
}

// AddNote writes a note or to-do on an asset.
func (s *Service) AddNote(ctx context.Context, admin, id, user resource.ID, in notes.Input) (notes.Note, error) {
	if _, err := s.Asset(ctx, admin, id, user); err != nil {
		return notes.Note{}, err
	}
	if s.notes == nil {
		return notes.Note{}, invalid("base", "Notes are not kept")
	}
	return s.notes.Add(ctx, admin, EntityType, id, user, in)
}

// DeleteNote removes a note from an asset.
func (s *Service) DeleteNote(ctx context.Context, admin, id, note, user resource.ID) error {
	if _, err := s.Asset(ctx, admin, id, user); err != nil {
		return err
	}
	if s.notes == nil {
		return notes.ErrNotFound
	}
	return s.notes.Delete(ctx, admin, EntityType, id, note)
}
