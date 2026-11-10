package contacts

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/adirkuhn/moneyharpy/internal/resource"
)

// NoteEntityType is the entity type of notes on contacts, as Moneybird names it.
const NoteEntityType = "Contact"

const maxNote = 10000

// NoteInput is a new note. A to-do may be assigned to a member.
type NoteInput struct {
	Note       string
	Todo       bool
	AssigneeID *resource.ID
}

// AddNote adds a note or a to-do to a contact, written by user.
func (s *Service) AddNote(ctx context.Context, admin, contact, user resource.ID, in NoteInput) (Note, error) {
	if _, err := s.Contact(ctx, admin, contact, user); err != nil {
		return Note{}, err
	}
	n := Note{ID: s.ids.Next(), AdministrationID: admin, EntityID: contact, EntityType: NoteEntityType, UserID: user, Todo: in.Todo, Note: strings.TrimSpace(in.Note)}
	var verr resource.ValidationError
	switch {
	case n.Note == "":
		verr.Add("note", "can't be blank")
	case len(n.Note) > maxNote:
		verr.Add("note", fmt.Sprintf("is too long (maximum is %d characters)", maxNote))
	}
	if in.AssigneeID != nil {
		if !in.Todo {
			verr.Add("assignee_id", "can only be set on a to-do")
		} else if member, err := s.store.IsMember(ctx, admin, *in.AssigneeID); err != nil || !member {
			verr.Add("assignee_id", "is not a user of this administration")
		}
		n.AssigneeID = in.AssigneeID
	}
	if err := verr.OrNil(); err != nil {
		return Note{}, err
	}
	n.CreatedAt = s.now()
	n.UpdatedAt = n.CreatedAt
	if err := s.store.CreateNote(ctx, n); err != nil {
		return Note{}, fmt.Errorf("store note: %w", err)
	}
	return n, nil
}

// DeleteNote removes a note from a contact.
func (s *Service) DeleteNote(ctx context.Context, admin, contact, id, user resource.ID) error {
	c, err := s.Contact(ctx, admin, contact, user)
	if err != nil {
		return err
	}
	if !slices.ContainsFunc(c.Notes, func(n Note) bool { return n.ID == id }) {
		return ErrNoteNotFound
	}
	return s.store.DeleteNote(ctx, admin, contact, id, s.now())
}
