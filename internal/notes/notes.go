// Package notes holds remarks and to-dos people write on records such as
// contacts, invoices and journal entries. The record's own service checks
// access before it calls this package.
package notes

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/adirkuhn/moneyharpy/internal/resource"
)

// ErrNotFound reports a note that is not on the record.
var ErrNotFound = errors.New("note not found")

// MaxLength caps a note's text.
const MaxLength = 10000

// Note is a remark or a to-do on a record.
type Note struct {
	ID               resource.ID
	AdministrationID resource.ID
	EntityID         resource.ID
	// EntityType names the kind of record, as Moneybird does: Contact,
	// SalesInvoice, GeneralJournalDocument.
	EntityType    string
	UserID        resource.ID
	AssigneeID    *resource.ID
	Todo          bool
	Note          string
	CompletedAt   *time.Time
	CompletedByID *resource.ID
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

// Open reports an unfinished to-do.
func (n Note) Open() bool { return n.Todo && n.CompletedAt == nil }

// Input is a new note. A to-do may be assigned to a member.
type Input struct {
	Note       string
	Todo       bool
	AssigneeID *resource.ID
}

// Store persists notes.
type Store interface {
	CreateNote(ctx context.Context, n Note) error
	DeleteNote(ctx context.Context, admin, entity, id resource.ID) error
	// NotesOf returns the notes on the records, oldest first.
	NotesOf(ctx context.Context, admin resource.ID, entityType string, entities []resource.ID) ([]Note, error)
	IsMember(ctx context.Context, admin, user resource.ID) (bool, error)
}

// Service implements the note use cases.
type Service struct {
	store Store
	ids   *resource.Generator
	now   func() time.Time
}

// NewService returns a Service.
func NewService(store Store, ids *resource.Generator, now func() time.Time) *Service {
	return &Service{store: store, ids: ids, now: now}
}

// Add writes a note by user on a record.
func (s *Service) Add(ctx context.Context, admin resource.ID, entityType string, entity, user resource.ID, in Input) (Note, error) {
	n := Note{ID: s.ids.Next(), AdministrationID: admin, EntityID: entity, EntityType: entityType, UserID: user, Todo: in.Todo, Note: strings.TrimSpace(in.Note)}
	var verr resource.ValidationError
	switch {
	case n.Note == "":
		verr.Add("note", "can't be blank")
	case len(n.Note) > MaxLength:
		verr.Add("note", fmt.Sprintf("is too long (maximum is %d characters)", MaxLength))
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

// Delete removes a note from a record.
func (s *Service) Delete(ctx context.Context, admin resource.ID, entityType string, entity, id resource.ID) error {
	list, err := s.store.NotesOf(ctx, admin, entityType, []resource.ID{entity})
	if err != nil {
		return err
	}
	for _, n := range list {
		if n.ID == id {
			return s.store.DeleteNote(ctx, admin, entity, id)
		}
	}
	return ErrNotFound
}

// Of returns the notes on each record.
func (s *Service) Of(ctx context.Context, admin resource.ID, entityType string, entities []resource.ID) (map[resource.ID][]Note, error) {
	out := map[resource.ID][]Note{}
	if len(entities) == 0 {
		return out, nil
	}
	list, err := s.store.NotesOf(ctx, admin, entityType, entities)
	if err != nil {
		return nil, fmt.Errorf("load notes: %w", err)
	}
	for _, n := range list {
		out[n.EntityID] = append(out[n.EntityID], n)
	}
	return out, nil
}
