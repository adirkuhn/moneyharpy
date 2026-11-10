package notes

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/adirkuhn/moneyharpy/internal/resource"
)

const (
	admin           resource.ID = 100
	owner, stranger resource.ID = 1, 2
	record, other   resource.ID = 10, 11
)

type memStore struct{ notes []Note }

func (m *memStore) CreateNote(_ context.Context, n Note) error {
	m.notes = append(m.notes, n)
	return nil
}
func (m *memStore) DeleteNote(_ context.Context, _, entity, id resource.ID) error {
	m.notes = slices.DeleteFunc(m.notes, func(n Note) bool { return n.EntityID == entity && n.ID == id })
	return nil
}
func (m *memStore) NotesOf(_ context.Context, _ resource.ID, entityType string, entities []resource.ID) ([]Note, error) {
	var out []Note
	for _, n := range m.notes {
		if n.EntityType == entityType && slices.Contains(entities, n.EntityID) {
			out = append(out, n)
		}
	}
	return out, nil
}
func (m *memStore) IsMember(_ context.Context, _, user resource.ID) (bool, error) {
	return user == owner, nil
}

func newService() (*Service, *memStore) {
	store := &memStore{}
	return NewService(store, resource.NewGenerator(1), func() time.Time { return time.Date(2026, 11, 11, 9, 0, 0, 0, time.UTC) }), store
}

func TestAddValidates(t *testing.T) {
	s, _ := newService()
	ctx := context.Background()
	if _, err := s.Add(ctx, admin, "Contact", record, owner, Input{Note: "  "}); err == nil {
		t.Error("blank note accepted")
	}
	if _, err := s.Add(ctx, admin, "Contact", record, owner, Input{Note: "x", AssigneeID: ptr(owner)}); err == nil {
		t.Error("assignee on a plain note accepted")
	}
	if _, err := s.Add(ctx, admin, "Contact", record, owner, Input{Note: "x", Todo: true, AssigneeID: ptr(stranger)}); err == nil {
		t.Error("stranger assigned")
	}
	n, err := s.Add(ctx, admin, "Contact", record, owner, Input{Note: " Call back ", Todo: true, AssigneeID: ptr(owner)})
	if err != nil || n.Note != "Call back" || !n.Open() || n.UserID != owner {
		t.Errorf("note = %+v, %v", n, err)
	}
}

func TestOfAndDeleteKeepRecordsApart(t *testing.T) {
	s, _ := newService()
	ctx := context.Background()
	a, _ := s.Add(ctx, admin, "Contact", record, owner, Input{Note: "a"})
	_, _ = s.Add(ctx, admin, "Contact", other, owner, Input{Note: "b"})
	_, _ = s.Add(ctx, admin, "SalesInvoice", record, owner, Input{Note: "c"})
	got, _ := s.Of(ctx, admin, "Contact", []resource.ID{record, other})
	if len(got[record]) != 1 || len(got[other]) != 1 {
		t.Errorf("notes = %+v", got)
	}
	if err := s.Delete(ctx, admin, "Contact", other, a.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("delete through another record = %v", err)
	}
	if err := s.Delete(ctx, admin, "Contact", record, a.ID); err != nil {
		t.Fatal(err)
	}
	got, _ = s.Of(ctx, admin, "Contact", []resource.ID{record})
	if len(got[record]) != 0 {
		t.Errorf("left = %+v", got[record])
	}
}

func ptr[T any](v T) *T { return &v }
