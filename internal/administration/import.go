package administration

import (
	"context"
	"fmt"
	"strings"

	"github.com/adirkuhn/moneyharpy/internal/resource"
)

// ImportCustomField adds a custom field brought in from another system,
// such as Moneybird, keeping its id. It checks the field as
// CreateCustomField does; one that is here already stays as it is.
func (s *Service) ImportCustomField(ctx context.Context, admin, user resource.ID, o resource.Origin, in NewCustomField) (CustomField, error) {
	if _, err := s.store.ForUser(ctx, admin, user); err != nil {
		return CustomField{}, err
	}
	existing, err := s.store.CustomFields(ctx, admin)
	if err != nil {
		return CustomField{}, fmt.Errorf("list custom fields: %w", err)
	}
	created, updated := o.Or(s.now())
	f := CustomField{ID: o.ID, AdministrationID: admin, Name: strings.TrimSpace(in.Name), Source: in.Source, CreatedAt: created, UpdatedAt: updated}
	for _, e := range existing {
		if e.ID == o.ID {
			return e, nil
		}
		if e.Source == f.Source {
			f.Position = max(f.Position, e.Position)
		}
	}
	f.Position++
	if err := validateCustomField(f); err != nil {
		return CustomField{}, err
	}
	if err := s.store.CreateCustomField(ctx, f); err != nil {
		return CustomField{}, fmt.Errorf("store custom field: %w", err)
	}
	return f, nil
}

// ImportIdentity adds a sender address brought in from another system,
// keeping its id. It checks the fields as CreateIdentity does; one that is
// here already stays as it is.
func (s *Service) ImportIdentity(ctx context.Context, admin, user resource.ID, o resource.Origin, c IdentityChanges) (Identity, error) {
	if _, err := s.store.ForUser(ctx, admin, user); err != nil {
		return Identity{}, err
	}
	if i, err := s.store.Identity(ctx, admin, o.ID); err == nil {
		return i, nil
	}
	created, updated := o.Or(s.now())
	i := Identity{ID: o.ID, AdministrationID: admin, CreatedAt: created, UpdatedAt: updated}
	if err := s.apply(ctx, &i, c); err != nil {
		return Identity{}, err
	}
	if err := s.store.CreateIdentity(ctx, i); err != nil {
		return Identity{}, fmt.Errorf("store identity: %w", err)
	}
	return i, nil
}
