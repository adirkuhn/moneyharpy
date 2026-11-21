package journal

import (
	"context"
	"fmt"

	"github.com/adirkuhn/moneyharpy/internal/resource"
)

// Import adds a journal entry brought in from another system, such as
// Moneybird, keeping its id and the ids of its rows, and posts it. It
// checks the entry as CreateDocument does. Bringing in an entry that is
// here already changes nothing.
func (s *Service) Import(ctx context.Context, admin, user resource.ID, o resource.Origin, c DocumentChanges, rows []resource.Origin) (Document, error) {
	if _, err := s.admins.Get(ctx, admin, user); err != nil {
		return Document{}, err
	}
	if d, err := s.documents.Document(ctx, admin, o.ID); err == nil {
		return d, nil
	}
	created, updated := o.Or(s.now())
	d := Document{ID: o.ID, AdministrationID: admin, CreatedAt: created, UpdatedAt: updated}
	if err := s.apply(ctx, admin, user, &d, c, nil, true); err != nil {
		return Document{}, err
	}
	for i := range d.Rows {
		if i < len(rows) && rows[i].ID != 0 {
			d.Rows[i].ID = rows[i].ID
		}
	}
	if err := s.documents.CreateDocument(ctx, d); err != nil {
		return Document{}, fmt.Errorf("store journal entry: %w", err)
	}
	return d, nil
}

// ImportCorrection books an entry an import makes to have the books as the
// source has them, such as for what the source booked that cannot come in.
// Like the rest of an import it takes the locked period as the source has
// it.
func (s *Service) ImportCorrection(ctx context.Context, admin, user resource.ID, c DocumentChanges) (Document, error) {
	if _, err := s.admins.Get(ctx, admin, user); err != nil {
		return Document{}, err
	}
	now := s.now()
	d := Document{ID: s.ids.Next(), AdministrationID: admin, CreatedAt: now, UpdatedAt: now}
	if err := s.apply(ctx, admin, user, &d, c, nil, true); err != nil {
		return Document{}, err
	}
	if err := s.documents.CreateDocument(ctx, d); err != nil {
		return Document{}, fmt.Errorf("store journal entry: %w", err)
	}
	return d, nil
}
