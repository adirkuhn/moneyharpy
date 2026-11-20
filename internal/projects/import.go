package projects

import (
	"context"
	"fmt"

	"github.com/adirkuhn/moneyharpy/internal/resource"
)

// Import adds a project brought in from another system, such as Moneybird,
// keeping the id, times and state it had there. It checks the fields as
// Create does.
func (s *Service) Import(ctx context.Context, admin, user resource.ID, o resource.Origin, c Changes) (Project, error) {
	if _, err := s.admins.Get(ctx, admin, user); err != nil {
		return Project{}, err
	}
	created, updated := o.Or(s.now())
	p := Project{ID: o.ID, AdministrationID: admin, State: Active, CreatedAt: created, UpdatedAt: updated}
	if err := s.apply(ctx, &p, c); err != nil {
		return Project{}, err
	}
	if err := s.store.SaveProject(ctx, p, true); err != nil {
		return Project{}, fmt.Errorf("store project: %w", err)
	}
	return p, nil
}
