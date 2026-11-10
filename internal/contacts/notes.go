package contacts

import (
	"context"
	"errors"
	"fmt"

	"github.com/adirkuhn/moneyharpy/internal/notes"
	"github.com/adirkuhn/moneyharpy/internal/resource"
)

// NoteEntityType is the entity type of notes on contacts, as Moneybird names it.
const NoteEntityType = "Contact"

// NoteInput is a new note; see notes.Input.
type NoteInput = notes.Input

// AddNote adds a note or a to-do to a contact, written by user.
func (s *Service) AddNote(ctx context.Context, admin, contact, user resource.ID, in NoteInput) (Note, error) {
	if _, err := s.Contact(ctx, admin, contact, user); err != nil {
		return Note{}, err
	}
	n, err := s.notes.Add(ctx, admin, NoteEntityType, contact, user, in)
	if err != nil {
		return Note{}, err
	}
	if err := s.store.TouchContact(ctx, admin, contact, n.CreatedAt); err != nil {
		return Note{}, fmt.Errorf("stamp contact: %w", err)
	}
	return n, nil
}

// DeleteNote removes a note from a contact.
func (s *Service) DeleteNote(ctx context.Context, admin, contact, id, user resource.ID) error {
	if _, err := s.Contact(ctx, admin, contact, user); err != nil {
		return err
	}
	if err := s.notes.Delete(ctx, admin, NoteEntityType, contact, id); err != nil {
		if errors.Is(err, notes.ErrNotFound) {
			return ErrNoteNotFound
		}
		return err
	}
	return s.store.TouchContact(ctx, admin, contact, s.now())
}
