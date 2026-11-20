package tasklists

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/adirkuhn/moneyharpy/internal/notes"
	"github.com/adirkuhn/moneyharpy/internal/resource"
)

// ImportedTask is a task brought in from another system, such as
// Moneybird, with the id and times it had, and its subtasks.
type ImportedTask struct {
	Origin      resource.Origin
	Name        string
	Description string
	RowOrder    int
	CompletedAt *time.Time
	ReportType  string
	Subtasks    []ImportedTask
}

// ImportedGroup is a group of tasks brought in with its id.
type ImportedGroup struct {
	Origin     resource.Origin
	Name       string
	RowOrder   int
	ReportType string
	Tasks      []ImportedTask
}

// ImportList adds a task list brought in from another system, keeping the
// ids and times of the list, its groups and its tasks, and which tasks
// were done. It checks the list as CreateList does; one that is here
// already stays as it is.
func (s *Service) ImportList(ctx context.Context, admin, user resource.ID, o resource.Origin, in ListInput, groups []ImportedGroup) (List, error) {
	if _, err := s.admins.Get(ctx, admin, user); err != nil {
		return List{}, err
	}
	if l, err := s.store.List(ctx, admin, o.ID); err == nil {
		return l, nil
	}
	created, updated := o.Or(s.now())
	l := List{ID: o.ID, AdministrationID: admin, CreatedAt: created, UpdatedAt: updated}
	in.TemplateID = nil
	if err := applyList(&l, in, true); err != nil {
		return List{}, err
	}
	l.Groups = importedGroups(admin, &l.ID, nil, groups, created)
	if err := s.store.SaveList(ctx, l, true); err != nil {
		return List{}, fmt.Errorf("store task list: %w", err)
	}
	return l, nil
}

// ImportTemplate adds a task list template brought in from another system,
// keeping the ids and times of the template, its groups and its tasks.
func (s *Service) ImportTemplate(ctx context.Context, admin, user resource.ID, o resource.Origin, in TemplateInput, groups []ImportedGroup) (Template, error) {
	if _, err := s.admins.Get(ctx, admin, user); err != nil {
		return Template{}, err
	}
	if t, err := s.store.Template(ctx, admin, o.ID); err == nil {
		return t, nil
	}
	created, updated := o.Or(s.now())
	t := Template{ID: o.ID, AdministrationID: admin, CreatedAt: created, UpdatedAt: updated}
	if err := applyTemplate(&t, in); err != nil {
		return Template{}, err
	}
	t.Groups = importedGroups(admin, nil, &t.ID, groups, created)
	if err := s.store.SaveTemplate(ctx, t, true); err != nil {
		return Template{}, fmt.Errorf("store template: %w", err)
	}
	return t, nil
}

func importedGroups(admin resource.ID, list, template *resource.ID, in []ImportedGroup, created time.Time) []Group {
	out := make([]Group, 0, len(in))
	for i, ig := range in {
		gc, gu := ig.Origin.Or(created)
		name := strings.TrimSpace(ig.Name)
		g := Group{
			ID: ig.Origin.ID, AdministrationID: admin, ListID: list, TemplateID: template, Name: &name, First: i == 0,
			RowOrder: ig.RowOrder, ReportType: ig.ReportType, CreatedAt: gc, UpdatedAt: gu,
		}
		g.Tasks = importedTasks(admin, g.ID, nil, ig.Tasks, gc)
		out = append(out, g)
	}
	return out
}

func importedTasks(admin, group resource.ID, parent *resource.ID, in []ImportedTask, created time.Time) []Task {
	out := make([]Task, 0, len(in))
	for _, it := range in {
		tc, tu := it.Origin.Or(created)
		t := Task{
			ID: it.Origin.ID, AdministrationID: admin, GroupID: group, ParentID: parent, Name: strings.TrimSpace(it.Name),
			Description: strings.TrimSpace(it.Description), RowOrder: it.RowOrder, CompletedAt: it.CompletedAt, ReportType: it.ReportType,
			Generated: it.ReportType != "", CreatedAt: tc, UpdatedAt: tu, Notes: []notes.Note{},
		}
		id := t.ID
		t.Subtasks = importedTasks(admin, group, &id, it.Subtasks, tc)
		out = append(out, t)
	}
	return out
}
