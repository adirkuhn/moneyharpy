package tasklists

import (
	"cmp"
	"context"
	"slices"
	"strings"

	"github.com/adirkuhn/moneyharpy/internal/administration"
	"github.com/adirkuhn/moneyharpy/internal/bookkeeping"
	"github.com/adirkuhn/moneyharpy/internal/notes"
	"github.com/adirkuhn/moneyharpy/internal/resource"
)

const (
	adminID = resource.ID(1)
	owner   = resource.ID(2)
	member  = resource.ID(3)
)

type fakeAdmins struct{}

func (fakeAdmins) Get(_ context.Context, id, user resource.ID) (administration.Listed, error) {
	if id != adminID || (user != owner && user != member) {
		return administration.Listed{}, administration.ErrNotFound
	}
	return administration.Listed{Administration: administration.Administration{ID: id, TimeZone: "Europe/Amsterdam"}}, nil
}

type memStore struct {
	templates map[resource.ID]Template
	lists     map[resource.ID]List
	groups    map[resource.ID]Group
	tasks     map[resource.ID]Task
}

func newMemStore() *memStore {
	return &memStore{templates: map[resource.ID]Template{}, lists: map[resource.ID]List{}, groups: map[resource.ID]Group{}, tasks: map[resource.ID]Task{}}
}

func (m *memStore) Templates(_ context.Context, _ resource.ID) ([]Template, error) {
	var out []Template
	for _, t := range m.templates {
		out = append(out, t)
	}
	slices.SortFunc(out, func(a, b Template) int { return strings.Compare(a.Name, b.Name) })
	return out, nil
}

func (m *memStore) Template(_ context.Context, _, id resource.ID) (Template, error) {
	t, ok := m.templates[id]
	if !ok {
		return Template{}, ErrTemplateNotFound
	}
	return t, nil
}

func (m *memStore) SaveTemplate(_ context.Context, t Template, created bool) error {
	if created {
		m.saveGroups(t.Groups)
	}
	t.Groups = nil
	m.templates[t.ID] = t
	return nil
}

func (m *memStore) saveGroups(groups []Group) {
	for _, g := range groups {
		_ = m.SaveGroup(context.Background(), g, true)
	}
}

func (m *memStore) DeleteTemplate(_ context.Context, _, id resource.ID) error {
	delete(m.templates, id)
	m.dropGroups(id)
	return nil
}

func (m *memStore) dropGroups(owner resource.ID) {
	for _, g := range m.groups {
		if g.owner() == owner {
			_ = m.DeleteGroup(context.Background(), adminID, g.ID)
		}
	}
}

func (m *memStore) Lists(_ context.Context, _ resource.ID, page, perPage int) ([]List, int, error) {
	var out []List
	for _, l := range m.lists {
		out = append(out, l)
	}
	slices.SortFunc(out, func(a, b List) int { return cmp.Compare(b.ID, a.ID) })
	total := len(out)
	from := min((page-1)*perPage, total)
	return out[from:min(from+perPage, total)], total, nil
}

func (m *memStore) List(_ context.Context, _, id resource.ID) (List, error) {
	l, ok := m.lists[id]
	if !ok {
		return List{}, ErrListNotFound
	}
	return l, nil
}

func (m *memStore) SaveList(_ context.Context, l List, created bool) error {
	if created {
		m.saveGroups(l.Groups)
	}
	l.Groups = nil
	m.lists[l.ID] = l
	return nil
}

func (m *memStore) DeleteList(_ context.Context, _, id resource.ID) error {
	delete(m.lists, id)
	m.dropGroups(id)
	return nil
}

func (m *memStore) Group(_ context.Context, _, id resource.ID) (Group, error) {
	g, ok := m.groups[id]
	if !ok {
		return Group{}, ErrGroupNotFound
	}
	return g, nil
}

func (m *memStore) GroupsOf(_ context.Context, _ resource.ID, owners []resource.ID) ([]Group, error) {
	var out []Group
	for _, g := range m.groups {
		if slices.Contains(owners, g.owner()) {
			out = append(out, g)
		}
	}
	slices.SortFunc(out, func(a, b Group) int { return cmp.Compare(a.RowOrder, b.RowOrder) })
	return out, nil
}

func (m *memStore) SaveGroup(_ context.Context, g Group, created bool) error {
	if created {
		_ = m.SaveTasks(context.Background(), g.Tasks, true)
	}
	g.Tasks = nil
	m.groups[g.ID] = g
	return nil
}

func (m *memStore) DeleteGroup(_ context.Context, _, id resource.ID) error {
	delete(m.groups, id)
	for _, t := range m.tasks {
		if t.GroupID == id {
			delete(m.tasks, t.ID)
		}
	}
	return nil
}

func (m *memStore) Task(_ context.Context, _, id resource.ID) (Task, error) {
	t, ok := m.tasks[id]
	if !ok {
		return Task{}, ErrTaskNotFound
	}
	return t, nil
}

func (m *memStore) TasksOf(_ context.Context, _ resource.ID, groups []resource.ID) ([]Task, error) {
	var out []Task
	for _, t := range m.tasks {
		if slices.Contains(groups, t.GroupID) {
			out = append(out, t)
		}
	}
	return out, nil
}

func (m *memStore) SaveTasks(_ context.Context, tasks []Task, created bool) error {
	for _, t := range tasks {
		if created {
			_ = m.SaveTasks(context.Background(), t.Subtasks, true)
		}
		t.Subtasks, t.Notes = nil, nil
		m.tasks[t.ID] = t
	}
	return nil
}

func (m *memStore) ReplaceTasks(ctx context.Context, admin, group resource.ID, parent *resource.ID, tasks []Task) error {
	for _, t := range m.tasks {
		if parent == nil && t.GroupID == group || parent != nil && t.ParentID != nil && *t.ParentID == *parent && t.Generated {
			_ = m.DeleteTask(ctx, admin, t.ID)
		}
	}
	return m.SaveTasks(ctx, tasks, true)
}

func (m *memStore) DeleteTask(_ context.Context, _, id resource.ID) error {
	delete(m.tasks, id)
	for _, t := range m.tasks {
		if t.ParentID != nil && *t.ParentID == id {
			_ = m.DeleteTask(context.Background(), adminID, t.ID)
		}
	}
	return nil
}

func (m *memStore) IsMember(_ context.Context, _, user resource.ID) (bool, error) {
	return user == owner || user == member, nil
}

type memNotes struct{ list []notes.Note }

func (m *memNotes) Add(_ context.Context, admin resource.ID, entityType string, entity, user resource.ID, in notes.Input) (notes.Note, error) {
	n := notes.Note{ID: resource.ID(900 + len(m.list)), AdministrationID: admin, EntityType: entityType, EntityID: entity, UserID: user, Note: in.Note, Todo: in.Todo}
	m.list = append(m.list, n)
	return n, nil
}

func (m *memNotes) Of(_ context.Context, _ resource.ID, _ string, entities []resource.ID) (map[resource.ID][]notes.Note, error) {
	out := map[resource.ID][]notes.Note{}
	for _, n := range m.list {
		if slices.Contains(entities, n.EntityID) {
			out[n.EntityID] = append(out[n.EntityID], n)
		}
	}
	return out, nil
}

// chart is a small chart of accounts: bank (with a sub-category), equity
// and revenue.
type chart struct{}

var (
	bank    = resource.ID(500)
	savings = resource.ID(501)
	equity  = resource.ID(502)
	revenue = resource.ID(503)
)

func (chart) LedgerAccounts(_ context.Context, admin, user resource.ID) ([]bookkeeping.LedgerAccount, error) {
	return []bookkeeping.LedgerAccount{
		{ID: bank, Name: "Bank", Code: "1100", Type: "current_assets", Active: true},
		{ID: savings, Name: "Savings", Type: "current_assets", ParentID: &bank, Active: true},
		{ID: equity, Name: "Equity", Type: "equity", Active: true},
		{ID: revenue, Name: "Revenue", Type: "revenue", Active: true},
		{ID: 504, Name: "Old", Type: "revenue"},
	}, nil
}
