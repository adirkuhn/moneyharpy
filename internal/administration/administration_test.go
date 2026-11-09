package administration

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/adirkuhn/moneyharpy/internal/resource"
	"github.com/google/go-cmp/cmp"
)

// memStore is an in-memory Store used to exercise use cases without a database.
type memStore struct {
	admins map[resource.ID]Administration
	access map[resource.ID]map[resource.ID]Membership // administration -> user
}

func newMemStore() *memStore {
	return &memStore{
		admins: map[resource.ID]Administration{},
		access: map[resource.ID]map[resource.ID]Membership{},
	}
}

func (s *memStore) Create(_ context.Context, a Administration, owner resource.ID) error {
	s.admins[a.ID] = a
	s.access[a.ID] = map[resource.ID]Membership{owner: {Access: AccessUser}}
	return nil
}

func (s *memStore) ListForUser(_ context.Context, user resource.ID) ([]Listed, error) {
	var out []Listed
	for id, members := range s.access {
		if m, ok := members[user]; ok {
			out = append(out, Listed{Administration: s.admins[id], Membership: m})
		}
	}
	return out, nil
}

func (s *memStore) ForUser(_ context.Context, id, user resource.ID) (Listed, error) {
	m, ok := s.access[id][user]
	if !ok {
		return Listed{}, ErrNotFound
	}
	return Listed{Administration: s.admins[id], Membership: m}, nil
}

func (s *memStore) SetFavorite(_ context.Context, id, user resource.ID, favorite bool) error {
	m, ok := s.access[id][user]
	if !ok {
		return ErrNotFound
	}
	m.Favorite = favorite
	s.access[id][user] = m
	return nil
}

func newService(s Store) *Service {
	return NewService(s, resource.NewGenerator(1), func() time.Time {
		return time.Date(2026, 3, 4, 5, 0, 0, 0, time.UTC)
	})
}

func TestCreateAppliesDefaults(t *testing.T) {
	const owner resource.ID = 1
	s := newService(newMemStore())

	got, err := s.Create(context.Background(), owner, NewAdministration{Name: "  Harpy B.V.  "})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	want := Listed{
		Administration: Administration{
			ID:              got.ID,
			Name:            "Harpy B.V.",
			Language:        "en",
			Currency:        "EUR",
			Country:         "NL",
			TimeZone:        "Europe/Amsterdam",
			PeriodStartDate: "2026-01-01",
		},
		Membership: Membership{Access: AccessUser},
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("Create() diff (-want +got):\n%s", diff)
	}
	if got.ID == 0 {
		t.Error("Create() ID = 0, want generated ID")
	}
}

func TestCreateRejectsInvalidInput(t *testing.T) {
	tests := []struct {
		name      string
		in        NewAdministration
		wantField string
	}{
		{name: "blank name", in: NewAdministration{Name: "   "}, wantField: "name"},
		{name: "unknown language", in: NewAdministration{Name: "A", Language: "de"}, wantField: "language"},
		{name: "lowercase currency", in: NewAdministration{Name: "A", Currency: "eur"}, wantField: "currency"},
		{name: "long country", in: NewAdministration{Name: "A", Country: "NLD"}, wantField: "country"},
		{name: "unknown time zone", in: NewAdministration{Name: "A", TimeZone: "Mars/Olympus"}, wantField: "time_zone"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := newService(newMemStore()).Create(context.Background(), 1, tt.in)
			var verr *resource.ValidationError
			if !errors.As(err, &verr) {
				t.Fatalf("Create(%+v) error = %v, want *resource.ValidationError", tt.in, err)
			}
			if _, ok := verr.Fields[tt.wantField]; !ok {
				t.Errorf("Create(%+v) fields = %v, want key %q", tt.in, verr.Fields, tt.wantField)
			}
		})
	}
}

func TestListOrdersFavoritesFirstThenName(t *testing.T) {
	const user resource.ID = 1
	ctx := context.Background()
	svc := newService(newMemStore())

	var zeta Listed
	for _, name := range []string{"Beta", "Alpha", "Zeta"} {
		l, err := svc.Create(ctx, user, NewAdministration{Name: name})
		if err != nil {
			t.Fatalf("Create(%q) error = %v", name, err)
		}
		if name == "Zeta" {
			zeta = l
		}
	}
	if err := svc.SetFavorite(ctx, zeta.ID, user, true); err != nil {
		t.Fatalf("SetFavorite() error = %v", err)
	}

	listed, err := svc.List(ctx, user)
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	var got []string
	for _, l := range listed {
		got = append(got, l.Name)
	}
	if diff := cmp.Diff([]string{"Zeta", "Alpha", "Beta"}, got); diff != "" {
		t.Errorf("List() order diff (-want +got):\n%s", diff)
	}
}

func TestOtherUsersCannotReachAdministration(t *testing.T) {
	ctx := context.Background()
	svc := newService(newMemStore())
	a, err := svc.Create(ctx, 1, NewAdministration{Name: "Private"})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	if _, err := svc.Get(ctx, a.ID, 2); !errors.Is(err, ErrNotFound) {
		t.Errorf("Get() error = %v, want ErrNotFound", err)
	}
	if err := svc.SetFavorite(ctx, a.ID, 2, true); !errors.Is(err, ErrNotFound) {
		t.Errorf("SetFavorite() error = %v, want ErrNotFound", err)
	}
}
