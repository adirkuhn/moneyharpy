package administration

import (
	"context"
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
	CreatedAt        time.Time
	UpdatedAt        time.Time
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

// CustomFields lists an administration's custom fields in creation order.
func (s *Service) CustomFields(ctx context.Context, admin, user resource.ID) ([]CustomField, error) {
	if _, err := s.store.ForUser(ctx, admin, user); err != nil {
		return nil, err
	}
	fields, err := s.store.CustomFields(ctx, admin)
	if err != nil {
		return nil, fmt.Errorf("list custom fields: %w", err)
	}
	return fields, nil
}

// CreateCustomField adds a custom field to an administration.
func (s *Service) CreateCustomField(ctx context.Context, admin, user resource.ID, in NewCustomField) (CustomField, error) {
	if _, err := s.store.ForUser(ctx, admin, user); err != nil {
		return CustomField{}, err
	}
	now := s.now()
	f := CustomField{
		ID: s.ids.Next(), AdministrationID: admin, Name: strings.TrimSpace(in.Name), Source: in.Source,
		CreatedAt: now, UpdatedAt: now,
	}
	var verr resource.ValidationError
	if f.Name == "" {
		verr.Add("name", "can't be blank")
	}
	if !slices.Contains([]Source{SourceSalesInvoice, SourceContact, SourceIdentity, SourceEstimate}, f.Source) {
		verr.Add("source", "is not included in the list")
	}
	if err := verr.OrNil(); err != nil {
		return CustomField{}, err
	}
	if err := s.store.CreateCustomField(ctx, f); err != nil {
		return CustomField{}, fmt.Errorf("store custom field: %w", err)
	}
	return f, nil
}
