package notes

import (
	"context"
	"fmt"
	"time"

	"github.com/adirkuhn/moneyharpy/internal/resource"
)

// Imported is a note brought in from another system, such as Moneybird,
// with the id and times it had there.
type Imported struct {
	Origin      resource.Origin
	Input       Input
	CompletedAt *time.Time
}

// Import writes a note brought in from another system on a record, keeping
// its id and times; a completed to-do counts as completed by user. It
// checks the note as Add does.
func (s *Service) Import(ctx context.Context, admin resource.ID, entityType string, entity, user resource.ID, in Imported) (Note, error) {
	n, err := s.build(ctx, admin, entityType, entity, user, in.Input)
	if err != nil {
		return Note{}, err
	}
	n.ID = in.Origin.ID
	n.CreatedAt, n.UpdatedAt = in.Origin.Or(s.now())
	if n.Todo && in.CompletedAt != nil {
		n.CompletedAt, n.CompletedByID = in.CompletedAt, &user
	}
	if err := s.store.CreateNote(ctx, n); err != nil {
		return Note{}, fmt.Errorf("store note: %w", err)
	}
	return n, nil
}
