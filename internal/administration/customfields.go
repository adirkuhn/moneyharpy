package administration

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/adirkuhn/moneyharpy/internal/resource"
)

// Source is the kind of record a custom field adds information to.
type Source string

// The record kinds custom fields can belong to, as named by Moneybird.
const (
	SourceSalesInvoice Source = "sales_invoice"
	SourceContact      Source = "contact"
	SourceIdentity     Source = "identity"
	SourceEstimate     Source = "estimate"
)

// CustomField is an extra named field an administration adds to one kind of
// record.
type CustomField struct {
	ID               resource.ID
	AdministrationID resource.ID
	Name             string
	Source           Source
	// Position orders the fields of one source, starting at 1.
	Position  int
	CreatedAt time.Time
	UpdatedAt time.Time
}

// Ref locates the custom field.
func (f CustomField) Ref() resource.Ref {
	return resource.Ref{ID: f.ID, Type: resource.TypeCustomField, AdministrationID: f.AdministrationID}
}

// NewCustomField is the input for creating a custom field.
type NewCustomField struct {
	Name   string
	Source Source
}

// ErrCustomFieldNotFound reports a custom field outside the administration.
var ErrCustomFieldNotFound = errors.New("custom field not found")

// CustomFieldChanges holds the custom field fields to set. Nil fields stay as
// they are. The source cannot change, since values belong to its records.
type CustomFieldChanges struct {
	Name *string
}

// CustomFields lists an administration's custom fields, ordered by position
// within each source.
func (s *Service) CustomFields(ctx context.Context, admin, user resource.ID) ([]CustomField, error) {
	if _, err := s.store.ForUser(ctx, admin, user); err != nil {
		return nil, err
	}
	fields, err := s.store.CustomFields(ctx, admin)
	if err != nil {
		return nil, fmt.Errorf("list custom fields: %w", err)
	}
	slices.SortStableFunc(fields, func(a, b CustomField) int {
		return cmp.Or(cmp.Compare(a.Position, b.Position), cmp.Compare(a.ID, b.ID))
	})
	return fields, nil
}

// CustomField returns one custom field.
func (s *Service) CustomField(ctx context.Context, admin, id, user resource.ID) (CustomField, error) {
	if _, err := s.store.ForUser(ctx, admin, user); err != nil {
		return CustomField{}, err
	}
	return s.store.CustomField(ctx, admin, id)
}

// UpdateCustomField renames a custom field.
func (s *Service) UpdateCustomField(ctx context.Context, admin, id, user resource.ID, c CustomFieldChanges) (CustomField, error) {
	f, err := s.CustomField(ctx, admin, id, user)
	if err != nil {
		return CustomField{}, err
	}
	if c.Name != nil {
		f.Name = strings.TrimSpace(*c.Name)
	}
	if err := validateCustomField(f); err != nil {
		return CustomField{}, err
	}
	f.UpdatedAt = s.now()
	if err := s.store.UpdateCustomField(ctx, f); err != nil {
		return CustomField{}, fmt.Errorf("store custom field: %w", err)
	}
	return f, nil
}

// DeleteCustomField removes a custom field and the values records hold for it.
func (s *Service) DeleteCustomField(ctx context.Context, admin, id, user resource.ID) error {
	if _, err := s.CustomField(ctx, admin, id, user); err != nil {
		return err
	}
	return s.store.DeleteCustomField(ctx, admin, id)
}

// ReorderCustomFields puts the fields of one source in the order of ids,
// which must name each of them once.
func (s *Service) ReorderCustomFields(ctx context.Context, admin, user resource.ID, source Source, ids []resource.ID) error {
	fields, err := s.CustomFields(ctx, admin, user)
	if err != nil {
		return err
	}
	var current []resource.ID
	byID := map[resource.ID]CustomField{}
	for _, f := range fields {
		if f.Source == source {
			current = append(current, f.ID)
			byID[f.ID] = f
		}
	}
	if !slices.Equal(slices.Sorted(slices.Values(current)), slices.Sorted(slices.Values(ids))) {
		var verr resource.ValidationError
		verr.Add("ids", "must list every custom field of the source once")
		return &verr
	}
	for i, id := range ids {
		f := byID[id]
		if f.Position == i+1 {
			continue
		}
		f.Position, f.UpdatedAt = i+1, s.now()
		if err := s.store.UpdateCustomField(ctx, f); err != nil {
			return fmt.Errorf("store custom field: %w", err)
		}
	}
	return nil
}

func validateCustomField(f CustomField) error {
	var verr resource.ValidationError
	if f.Name == "" || len(f.Name) > 255 {
		verr.Add("name", "can't be blank")
	}
	if !slices.Contains([]Source{SourceSalesInvoice, SourceContact, SourceIdentity, SourceEstimate}, f.Source) {
		verr.Add("source", "is not included in the list")
	}
	return verr.OrNil()
}

// CreateCustomField adds a custom field to an administration, last among the
// fields of its source.
func (s *Service) CreateCustomField(ctx context.Context, admin, user resource.ID, in NewCustomField) (CustomField, error) {
	if _, err := s.store.ForUser(ctx, admin, user); err != nil {
		return CustomField{}, err
	}
	now := s.now()
	f := CustomField{
		ID: s.ids.Next(), AdministrationID: admin, Name: strings.TrimSpace(in.Name), Source: in.Source,
		CreatedAt: now, UpdatedAt: now,
	}
	if err := validateCustomField(f); err != nil {
		return CustomField{}, err
	}
	existing, err := s.store.CustomFields(ctx, admin)
	if err != nil {
		return CustomField{}, fmt.Errorf("list custom fields: %w", err)
	}
	for _, e := range existing {
		if e.Source == f.Source {
			f.Position = max(f.Position, e.Position)
		}
	}
	f.Position++
	if err := s.store.CreateCustomField(ctx, f); err != nil {
		return CustomField{}, fmt.Errorf("store custom field: %w", err)
	}
	return f, nil
}
