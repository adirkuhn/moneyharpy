package postgres

import (
	"context"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/adirkuhn/moneyharpy/internal/notes"
	"github.com/adirkuhn/moneyharpy/internal/postgres/sqlc"
	"github.com/adirkuhn/moneyharpy/internal/resource"
)

// NoteStore implements notes.Store.
type NoteStore struct{ q *sqlc.Queries }

// NewNoteStore returns a NoteStore on pool.
func NewNoteStore(pool *pgxpool.Pool) *NoteStore { return &NoteStore{q: sqlc.New(pool)} }

// CreateNote implements notes.Store.
func (s *NoteStore) CreateNote(ctx context.Context, n notes.Note) error {
	return s.q.InsertNote(ctx, sqlc.InsertNoteParams{
		ID: int64(n.ID), AdministrationID: int64(n.AdministrationID), EntityID: int64(n.EntityID), EntityType: n.EntityType,
		UserID: pgtype.Int8{Int64: int64(n.UserID), Valid: n.UserID != 0}, AssigneeID: optionalID(n.AssigneeID), Todo: n.Todo, Note: n.Note,
		CompletedAt: optionalTime(n.CompletedAt), CompletedByID: optionalID(n.CompletedByID), CreatedAt: timestamp(n.CreatedAt), UpdatedAt: timestamp(n.UpdatedAt),
	})
}

// DeleteNote implements notes.Store.
func (s *NoteStore) DeleteNote(ctx context.Context, admin, entity, id resource.ID) error {
	return s.q.DeleteNote(ctx, sqlc.DeleteNoteParams{AdministrationID: int64(admin), EntityID: int64(entity), ID: int64(id)})
}

// NotesOf implements notes.Store.
func (s *NoteStore) NotesOf(ctx context.Context, admin resource.ID, entityType string, entities []resource.ID) ([]notes.Note, error) {
	rows, err := s.q.NotesOf(ctx, sqlc.NotesOfParams{AdministrationID: int64(admin), Ids: ids64(entities)})
	if err != nil {
		return nil, err
	}
	out := make([]notes.Note, 0, len(rows))
	for _, n := range rows {
		if n.EntityType != entityType {
			continue
		}
		note := notes.Note{
			ID: resource.ID(n.ID), AdministrationID: resource.ID(n.AdministrationID), EntityID: resource.ID(n.EntityID), EntityType: n.EntityType,
			AssigneeID: idOf(n.AssigneeID), Todo: n.Todo, Note: n.Note, CompletedAt: timeOf(n.CompletedAt), CompletedByID: idOf(n.CompletedByID),
			CreatedAt: n.CreatedAt.Time, UpdatedAt: n.UpdatedAt.Time,
		}
		if n.UserID.Valid {
			note.UserID = resource.ID(n.UserID.Int64)
		}
		out = append(out, note)
	}
	return out, nil
}

// IsMember implements notes.Store.
func (s *NoteStore) IsMember(ctx context.Context, admin, user resource.ID) (bool, error) {
	return s.q.IsMember(ctx, sqlc.IsMemberParams{AdministrationID: int64(admin), UserID: int64(user)})
}
