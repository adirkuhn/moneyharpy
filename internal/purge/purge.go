// Package purge empties an administration: it removes all its data while
// keeping its setup, so it can start over, such as before importing it
// again from Moneybird.
package purge

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/adirkuhn/moneyharpy/internal/administration"
	"github.com/adirkuhn/moneyharpy/internal/membership"
	"github.com/adirkuhn/moneyharpy/internal/resource"
)

// ErrImportRunning reports that an import is still bringing data in.
var ErrImportRunning = errors.New("an import is running")

// Administrations checks a user's access to an administration.
type Administrations interface {
	Get(ctx context.Context, id, user resource.ID) (administration.Listed, error)
}

// Store removes an administration's data.
type Store interface {
	// Empty removes the data at once and returns the storage keys of the
	// files it had.
	Empty(ctx context.Context, admin resource.ID) ([]string, error)
	ImportRunning(ctx context.Context, admin resource.ID) (bool, error)
}

// Files deletes stored files.
type Files interface {
	Delete(ctx context.Context, key string) error
}

// Service empties administrations.
type Service struct {
	admins Administrations
	store  Store
	files  Files
}

// NewService returns a Service.
func NewService(admins Administrations, store Store, files Files) *Service {
	return &Service{admins: admins, store: store, files: files}
}

// Empty removes everything an administration holds: contacts, documents,
// invoices, the bank, the books, time, tasks, notes, files and events.
// Its users, company details, sender addresses, workflows, document styles,
// custom fields, connections and webhooks stay; categories and VAT rates go
// back to the defaults. Only an owner may, giving the administration's name
// exactly, and not while an import is bringing data in.
func (s *Service) Empty(ctx context.Context, admin, user resource.ID, confirmName string) error {
	a, err := s.admins.Get(ctx, admin, user)
	if err != nil {
		return err
	}
	if !a.Can(administration.Ownership) {
		return membership.ErrForbidden
	}
	if strings.TrimSpace(confirmName) != a.Name {
		var verr resource.ValidationError
		verr.Add("name", "does not match the administration's name")
		return &verr
	}
	running, err := s.store.ImportRunning(ctx, admin)
	if err != nil {
		return err
	}
	if running {
		return ErrImportRunning
	}
	keys, err := s.store.Empty(ctx, admin)
	if err != nil {
		return fmt.Errorf("empty administration: %w", err)
	}
	// The records are gone; a file left behind is only space, so every one is tried.
	var errs []error
	for _, k := range keys {
		errs = append(errs, s.files.Delete(ctx, k))
	}
	if err := errors.Join(errs...); err != nil {
		return fmt.Errorf("remove files: %w", err)
	}
	return nil
}
