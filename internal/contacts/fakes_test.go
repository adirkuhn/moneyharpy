package contacts

import (
	"context"
	"errors"
	"slices"
	"strconv"
	"time"

	"github.com/adirkuhn/moneyharpy/internal/administration"
	"github.com/adirkuhn/moneyharpy/internal/notes"
	"github.com/adirkuhn/moneyharpy/internal/resource"
)

const (
	adminID         resource.ID = 100
	owner, stranger resource.ID = 1, 2
	colleague       resource.ID = 3
	invoiceFlow     resource.ID = 500
	estimateFlow    resource.ID = 501
	closedFlow      resource.ID = 502
	contactField    resource.ID = 600
	identityField   resource.ID = 601
)

var start = time.Date(2026, 11, 10, 9, 0, 0, 0, time.UTC)

type fakeAdmins struct{}

func (fakeAdmins) Get(_ context.Context, id, user resource.ID) (administration.Listed, error) {
	if id != adminID || user != owner {
		return administration.Listed{}, administration.ErrNotFound
	}
	return administration.Listed{Administration: administration.Administration{ID: adminID, Country: "NL", Language: "en"}}, nil
}

type fakeWorkflows struct{}

func (fakeWorkflows) WorkflowKind(_ context.Context, _, id, _ resource.ID) (string, bool, error) {
	switch id {
	case invoiceFlow:
		return "InvoiceWorkflow", true, nil
	case estimateFlow:
		return "EstimateWorkflow", true, nil
	case closedFlow:
		return "InvoiceWorkflow", false, nil
	}
	return "", false, errors.New("workflow not found")
}

type fakeFields struct{}

func (fakeFields) CustomFields(context.Context, resource.ID, resource.ID) ([]administration.CustomField, error) {
	return []administration.CustomField{
		{ID: contactField, Name: "Account manager", Source: administration.SourceContact},
		{ID: identityField, Name: "Branch", Source: administration.SourceIdentity},
	}, nil
}

type fakeReferences struct{ used map[resource.ID]bool }

func (f fakeReferences) ContactInUse(_ context.Context, _, id resource.ID) (bool, error) {
	return f.used[id], nil
}

type memStore struct {
	contacts map[resource.ID]Contact
	notes    *memNotes
}

func newMemStore() *memStore {
	return &memStore{contacts: map[resource.ID]Contact{}, notes: &memNotes{}}
}

func (m *memStore) Contacts(_ context.Context, _ resource.ID, q Query) ([]Contact, int, error) {
	var out []Contact
	for _, c := range m.contacts {
		c.Notes, _ = m.notes.NotesOf(context.Background(), 0, NoteEntityType, []resource.ID{c.ID})
		if q.Matches(c) {
			out = append(out, c)
		}
	}
	slices.SortFunc(out, Less)
	total := len(out)
	if q.PerPage > 0 {
		from := min((max(q.Page, 1)-1)*q.PerPage, total)
		out = out[from:min(from+q.PerPage, total)]
	}
	return out, total, nil
}

func (m *memStore) Contact(_ context.Context, _, id resource.ID) (Contact, error) {
	c, ok := m.contacts[id]
	if !ok {
		return Contact{}, ErrNotFound
	}
	return c, nil
}

func (m *memStore) ContactByCustomerID(_ context.Context, _ resource.ID, customerID string) (Contact, error) {
	for _, c := range m.contacts {
		if c.CustomerID == customerID {
			return c, nil
		}
	}
	return Contact{}, ErrNotFound
}

func (m *memStore) ContactsByID(_ context.Context, _ resource.ID, ids []resource.ID) ([]Contact, error) {
	var out []Contact
	for _, id := range ids {
		if c, ok := m.contacts[id]; ok {
			out = append(out, c)
		}
	}
	return out, nil
}

func (m *memStore) Versions(ctx context.Context, admin resource.ID, q Query) ([]Version, error) {
	list, _, _ := m.Contacts(ctx, admin, Query{IncludeArchived: q.IncludeArchived})
	out := []Version{}
	for _, c := range list {
		if q.Matches(c) {
			out = append(out, Version{ID: c.ID, Version: c.Version()})
		}
	}
	return out, nil
}

func (m *memStore) MaxCustomerNumber(context.Context, resource.ID) (int64, error) {
	var highest int64
	for _, c := range m.contacts {
		if n, err := strconv.ParseInt(c.CustomerID, 10, 64); err == nil {
			highest = max(highest, n)
		}
	}
	return highest, nil
}

func (m *memStore) CustomerIDTaken(_ context.Context, _ resource.ID, customerID string, except resource.ID) (bool, error) {
	for _, c := range m.contacts {
		if c.CustomerID == customerID && c.ID != except {
			return true, nil
		}
	}
	return false, nil
}

func (m *memStore) CreateContact(_ context.Context, c Contact) error {
	m.contacts[c.ID] = c
	return nil
}

func (m *memStore) UpdateContact(_ context.Context, c Contact) error {
	m.contacts[c.ID] = c
	return nil
}

func (m *memStore) DeleteContact(_ context.Context, _, id resource.ID) error {
	delete(m.contacts, id)
	return nil
}

func (m *memStore) SavePerson(_ context.Context, p Person) error {
	c := m.contacts[p.ContactID]
	c.People = slices.DeleteFunc(slices.Clone(c.People), func(e Person) bool { return e.ID == p.ID })
	c.People = append(c.People, p)
	c.UpdatedAt = p.UpdatedAt
	m.contacts[c.ID] = c
	return nil
}

func (m *memStore) DeletePerson(_ context.Context, _, contact, id resource.ID, at time.Time) error {
	c := m.contacts[contact]
	c.People = slices.DeleteFunc(slices.Clone(c.People), func(e Person) bool { return e.ID == id })
	c.UpdatedAt = at
	m.contacts[contact] = c
	return nil
}

func (m *memStore) TouchContact(_ context.Context, _, id resource.ID, at time.Time) error {
	c := m.contacts[id]
	c.UpdatedAt = at
	m.contacts[id] = c
	return nil
}

// memNotes is a notes.Store the contacts fake reads to match open to-dos.
type memNotes struct{ notes []notes.Note }

func (m *memNotes) CreateNote(_ context.Context, n notes.Note) error {
	m.notes = append(m.notes, n)
	return nil
}

func (m *memNotes) DeleteNote(_ context.Context, _, entity, id resource.ID) error {
	m.notes = slices.DeleteFunc(m.notes, func(n notes.Note) bool { return n.EntityID == entity && n.ID == id })
	return nil
}

func (m *memNotes) NotesOf(_ context.Context, _ resource.ID, entityType string, entities []resource.ID) ([]notes.Note, error) {
	var out []notes.Note
	for _, n := range m.notes {
		if n.EntityType == entityType && slices.Contains(entities, n.EntityID) {
			out = append(out, n)
		}
	}
	return out, nil
}

func (m *memNotes) IsMember(_ context.Context, _, user resource.ID) (bool, error) {
	return user == owner || user == colleague, nil
}

type clock struct{ t time.Time }

func (c *clock) now() time.Time {
	c.t = c.t.Add(time.Second)
	return c.t
}

func newService(used ...resource.ID) (*Service, *memStore) {
	store := newMemStore()
	refs := fakeReferences{used: map[resource.ID]bool{}}
	for _, id := range used {
		refs.used[id] = true
	}
	c := &clock{t: start}
	ids := resource.NewGenerator(1)
	return NewService(fakeAdmins{}, fakeWorkflows{}, fakeFields{}, refs, notes.NewService(store.notes, ids, c.now), store, ids, c.now), store
}
