package purge

import (
	"context"
	"errors"
	"testing"

	"github.com/adirkuhn/moneyharpy/internal/administration"
	"github.com/adirkuhn/moneyharpy/internal/membership"
	"github.com/adirkuhn/moneyharpy/internal/resource"
)

const (
	adminID resource.ID = 100
	owner   resource.ID = 1
	clerk   resource.ID = 2
)

type fakeAdmins struct{}

func (fakeAdmins) Get(_ context.Context, id, user resource.ID) (administration.Listed, error) {
	perms := []administration.Permission{administration.Settings}
	if user == owner {
		perms = append(perms, administration.Ownership)
	}
	return administration.Listed{Administration: administration.Administration{ID: id, Name: "Example Shop B.V."}, Membership: administration.Membership{Permissions: perms}}, nil
}

type fakeStore struct {
	running bool
	emptied bool
}

func (f *fakeStore) Empty(context.Context, resource.ID) ([]string, error) {
	f.emptied = true
	return []string{"administrations/100/attachments/7", "administrations/100/downloads/8"}, nil
}
func (f *fakeStore) ImportRunning(context.Context, resource.ID) (bool, error) { return f.running, nil }

type fakeFiles struct{ deleted []string }

func (f *fakeFiles) Delete(_ context.Context, key string) error {
	f.deleted = append(f.deleted, key)
	return nil
}

func TestEmptyNeedsTheOwnerAndTheName(t *testing.T) {
	store, files := &fakeStore{}, &fakeFiles{}
	s := NewService(fakeAdmins{}, store, files)
	ctx := context.Background()
	if err := s.Empty(ctx, adminID, clerk, "Example Shop B.V."); !errors.Is(err, membership.ErrForbidden) {
		t.Errorf("a member who does not own it: %v", err)
	}
	var verr *resource.ValidationError
	if err := s.Empty(ctx, adminID, owner, "Example Shop"); !errors.As(err, &verr) {
		t.Errorf("a wrong name: %v", err)
	}
	if store.emptied {
		t.Fatal("emptied without confirmation")
	}
	if err := s.Empty(ctx, adminID, owner, " Example Shop B.V. "); err != nil {
		t.Fatal(err)
	}
	if !store.emptied || len(files.deleted) != 2 {
		t.Errorf("emptied %v, files deleted %v", store.emptied, files.deleted)
	}
}

func TestEmptyWaitsForARunningImport(t *testing.T) {
	store := &fakeStore{running: true}
	s := NewService(fakeAdmins{}, store, &fakeFiles{})
	if err := s.Empty(context.Background(), adminID, owner, "Example Shop B.V."); !errors.Is(err, ErrImportRunning) || store.emptied {
		t.Errorf("Empty() = %v, emptied %v", err, store.emptied)
	}
}
