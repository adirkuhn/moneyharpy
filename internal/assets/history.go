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
	return nil
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
