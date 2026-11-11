package projects

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/adirkuhn/moneyharpy/internal/administration"
	"github.com/adirkuhn/moneyharpy/internal/resource"
)

const (
	adminID = resource.ID(1)
	owner   = resource.ID(2)
)

var now = time.Date(2026, 11, 11, 9, 0, 0, 0, time.UTC)

type fakeAdmins struct{}

func (fakeAdmins) Get(_ context.Context, id, user resource.ID) (administration.Listed, error) {
	if id != adminID || user != owner {
		return administration.Listed{}, administration.ErrNotFound
	}
	return administration.Listed{Administration: administration.Administration{ID: id}}, nil
}

type memStore struct{ projects map[resource.ID]Project }

func (m *memStore) Projects(_ context.Context, _ resource.ID, q Query) ([]Project, int, error) {
	var out []Project
	for _, p := range m.projects {
		if q.Matches(p) {
			out = append(out, p)
		}
	}
	slices.SortFunc(out, func(a, b Project) int { return strings.Compare(a.Name, b.Name) })
	total := len(out)
	if q.PerPage > 0 {
		from := min((q.Page-1)*q.PerPage, total)
		out = out[from:min(from+q.PerPage, total)]
	}
	return out, total, nil
}

func (m *memStore) Project(_ context.Context, _, id resource.ID) (Project, error) {
	p, ok := m.projects[id]
	if !ok {
		return Project{}, ErrNotFound
	}
	return p, nil
}

func (m *memStore) NameTaken(_ context.Context, _ resource.ID, name string, except resource.ID) (bool, error) {
	for _, p := range m.projects {
		if p.ID != except && strings.EqualFold(p.Name, name) {
			return true, nil
		}
	}
	return false, nil
}

func (m *memStore) SaveProject(_ context.Context, p Project, _ bool) error {
	m.projects[p.ID] = p
	return nil
}

func (m *memStore) DeleteProject(_ context.Context, _, id resource.ID) error {
	delete(m.projects, id)
	return nil
}

type fakeReferences map[resource.ID]bool

func (f fakeReferences) ProjectInUse(_ context.Context, _, id resource.ID) (bool, error) {
	return f[id], nil
}

func newService(refs References) (*Service, *memStore) {
	store := &memStore{projects: map[resource.ID]Project{}}
	return NewService(fakeAdmins{}, store, refs, resource.NewGenerator(1), func() time.Time { return now }), store
}

func ptr[T any](v T) *T { return &v }

func fieldsOf(t *testing.T, err error) map[string][]string {
	t.Helper()
	var verr *resource.ValidationError
	if !errors.As(err, &verr) {
		t.Fatalf("want a validation error, got %v", err)
	}
	return verr.Fields
}

func TestCreateUpdateAndList(t *testing.T) {
	s, _ := newService(nil)
	ctx := context.Background()
	p, err := s.Create(ctx, adminID, owner, Changes{Name: ptr(" Falcon "), Budget: ptr("10,4")})
	if err != nil || p.Name != "Falcon" || p.State != Active || p.Budget == nil || *p.Budget != 10 {
		t.Fatalf("Create() = %+v, %v", p, err)
	}
	if p.Ref().URI() != "/1/projects/"+p.ID.String() {
		t.Errorf("uri = %s", p.Ref().URI())
	}
	if _, err := s.Create(ctx, adminID, owner, Changes{Name: ptr("falcon")}); fieldsOf(t, err)["name"] == nil {
		t.Error("duplicate name accepted")
	}
	if _, err := s.Create(ctx, adminID, owner, Changes{}); fieldsOf(t, err)["name"] == nil {
		t.Error("blank name accepted")
	}
	if _, err := s.Create(ctx, adminID, owner, Changes{Name: ptr("Eagle"), Budget: ptr("lots")}); fieldsOf(t, err)["budget"] == nil {
		t.Error("budget lots accepted")
	}
	up, err := s.Update(ctx, adminID, p.ID, owner, Changes{Name: ptr("Eagle"), Budget: ptr(""), State: ptr(Archived)})
	if err != nil || up.Name != "Eagle" || up.Budget != nil || up.State != Archived {
		t.Fatalf("Update() = %+v, %v", up, err)
	}
	if _, err := s.Update(ctx, adminID, p.ID, owner, Changes{State: ptr("gone")}); fieldsOf(t, err)["state"] == nil {
		t.Error("state gone accepted")
	}
	if _, err := s.Create(ctx, adminID, owner, Changes{Name: ptr("Falcon")}); err != nil {
		t.Fatal(err)
	}
	active, total, err := s.List(ctx, adminID, owner, Query{States: []string{Active}})
	if err != nil || total != 1 || active[0].Name != "Falcon" {
		t.Errorf("active = %+v (%d), %v", active, total, err)
	}
	if _, err := s.Project(ctx, adminID, p.ID, 99); !errors.Is(err, administration.ErrNotFound) {
		t.Errorf("other user = %v", err)
	}
	if err := s.CheckProject(ctx, adminID, owner, active[0].ID); err != nil {
		t.Errorf("CheckProject() of an active project = %v", err)
	}
	if fieldsOf(t, s.CheckProject(ctx, adminID, owner, p.ID))["project_id"] == nil {
		t.Error("CheckProject() accepted an archived project")
	}
	if fieldsOf(t, s.CheckProject(ctx, adminID, owner, 12345))["project_id"] == nil {
		t.Error("CheckProject() accepted an unknown project")
	}
}

func TestParseFilter(t *testing.T) {
	for in, want := range map[string]string{"": "active", "state:archived": "archived", "state:active|archived": "active,archived", "state:all": ""} {
		q, err := ParseFilter(in)
		if err != nil || strings.Join(q.States, ",") != want {
			t.Errorf("ParseFilter(%q) = %v, %v; want %s", in, q.States, err, want)
		}
	}
	for _, in := range []string{"state:gone", "colour:red", "state"} {
		if _, err := ParseFilter(in); err == nil {
			t.Errorf("ParseFilter(%q) accepted", in)
		}
	}
}

func TestDeleteArchivesProjectsInUse(t *testing.T) {
	s, store := newService(nil)
	ctx := context.Background()
	used, _ := s.Create(ctx, adminID, owner, Changes{Name: ptr("Used")})
	free, _ := s.Create(ctx, adminID, owner, Changes{Name: ptr("Free")})
	s.references = AnyReferences{fakeReferences{}, fakeReferences{used.ID: true}}
	if kept, err := s.Delete(ctx, adminID, used.ID, owner); err != nil || !kept || store.projects[used.ID].State != Archived {
		t.Errorf("delete used = %v, %v, %+v", kept, err, store.projects[used.ID])
	}
	if kept, err := s.Delete(ctx, adminID, free.ID, owner); err != nil || kept {
		t.Errorf("delete free = %v, %v", kept, err)
	}
	if _, ok := store.projects[free.ID]; ok {
		t.Error("free project still stored")
	}
	if _, err := s.Delete(ctx, adminID, free.ID, owner); !errors.Is(err, ErrNotFound) {
		t.Errorf("delete again = %v", err)
	}
}
