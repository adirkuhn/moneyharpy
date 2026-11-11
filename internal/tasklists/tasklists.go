// Package tasklists holds checklists for recurring work, after Moneybird's
// task lists: a list has groups of tasks, tasks have subtasks, members
// complete and are assigned tasks. Templates describe lists made again each
// period, such as closing a month. A group or task can follow a report: its
// tasks are then generated from the report's lines and cannot be deleted.
package tasklists

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/adirkuhn/moneyharpy/internal/administration"
	"github.com/adirkuhn/moneyharpy/internal/notes"
	"github.com/adirkuhn/moneyharpy/internal/resource"
)

// Entity types, as Moneybird names them in notes and events.
const (
	NoteEntityType     = "TaskLists::Task"
	ListEntityType     = "TaskLists::List"
	TemplateEntityType = "TaskLists::ListTemplate"
)

// ReportTypes are the reports groups and tasks can follow, with Moneybird's
// names.
var ReportTypes = []string{"balance_sheet", "profit_loss", "debtor", "creditor", "assets"}

// Paging of lists, as at Moneybird.
const (
	DefaultPerPage = 30
	MaxPerPage     = 100
)

// Not found errors, with Moneybird's model names in the message.
var (
	ErrListNotFound     = errors.New("record not found for model name: List")
	ErrTemplateNotFound = errors.New("record not found for model name: List template")
	ErrGroupNotFound    = errors.New("record not found for model name: Group")
	ErrTaskNotFound     = errors.New("record not found for model name: Task")
	ErrAssigneeNotFound = errors.New("assignee not found")
)

// IsNotFound reports errors that mean a record is missing.
func IsNotFound(err error) bool {
	return errors.Is(err, ErrListNotFound) || errors.Is(err, ErrTemplateNotFound) || errors.Is(err, ErrGroupNotFound) ||
		errors.Is(err, ErrTaskNotFound) || errors.Is(err, ErrAssigneeNotFound) || errors.Is(err, notes.ErrNotFound)
}

// rule is a refusal Moneybird words as one sentence.
func rule(message string) error {
	var verr resource.ValidationError
	verr.Add("base", message)
	return &verr
}

// Template describes a task list made each period.
type Template struct {
	ID               resource.ID
	AdministrationID resource.ID
	Name             string
	Description      string
	// Period is an ISO 8601 duration such as P1M; empty for none.
	Period string
	// DeadlineAfter is an ISO 8601 duration after the period's end.
	DeadlineAfter string
	CreatedAt     time.Time
	UpdatedAt     time.Time
	Groups        []Group
}

// Ref locates the template.
func (t Template) Ref() resource.Ref {
	return resource.Ref{ID: t.ID, Type: resource.TypeTaskListTemplate, AdministrationID: t.AdministrationID}
}

// List is a checklist for one period.
type List struct {
	ID               resource.ID
	AdministrationID resource.ID
	TemplateID       *resource.ID
	Name             string
	Description      string
	DeadlineOn       *time.Time
	PeriodBegin      *time.Time
	PeriodEnd        *time.Time
	CreatedAt        time.Time
	UpdatedAt        time.Time
	Groups           []Group
}

// Ref locates the list.
func (l List) Ref() resource.Ref {
	return resource.Ref{ID: l.ID, Type: resource.TypeTaskList, AdministrationID: l.AdministrationID}
}

// Counts returns the number of tasks, subtasks included, and of those
// completed.
func (l List) Counts() (total, completed int) {
	var walk func([]Task)
	walk = func(tasks []Task) {
		for _, t := range tasks {
			total++
			if t.Completed() {
				completed++
			}
			walk(t.Subtasks)
		}
	}
	for _, g := range l.Groups {
		walk(g.Tasks)
	}
	return total, completed
}

// Completed reports a list without open tasks.
func (l List) Completed() bool {
	total, completed := l.Counts()
	return total == completed
}

// Group is a heading tasks are listed under, in a list or a template. The
// first group has no name and holds the tasks outside any heading.
type Group struct {
	ID               resource.ID
	AdministrationID resource.ID
	ListID           *resource.ID
	TemplateID       *resource.ID
	Name             *string
	First            bool
	RowOrder         int
	// ReportType is the report the group's tasks are generated from.
	ReportType string
	CreatedAt  time.Time
	UpdatedAt  time.Time
	// Tasks are the top-level tasks, with their subtasks.
	Tasks []Task
}

// owner is the list or template the group is in.
func (g Group) owner() resource.ID {
	if g.ListID != nil {
		return *g.ListID
	}
	return deref(g.TemplateID)
}

// Task is something to do.
type Task struct {
	ID               resource.ID
	AdministrationID resource.ID
	GroupID          resource.ID
	ParentID         *resource.ID
	Name             string
	Description      string
	RowOrder         int
	CompletedAt      *time.Time
	ReportType       string
	AssigneeID       *resource.ID
	// RelatedEntityID is the category (ledger account) the task is about.
	RelatedEntityID *resource.ID
	// Generated tasks come from a report or a category and stay.
	Generated bool
	CreatedAt time.Time
	UpdatedAt time.Time
	Subtasks  []Task
	Notes     []notes.Note
}

// Completed reports a done task.
func (t Task) Completed() bool { return t.CompletedAt != nil }

// Line is one line of a report a task is generated from.
type Line struct {
	Name     string
	EntityID *resource.ID
}

// Reports supplies the lines tasks are generated from.
type Reports interface {
	// ReportLines returns the lines of a report type.
	ReportLines(ctx context.Context, admin, user resource.ID, reportType string) ([]Line, error)
	// Children returns the sub-categories of a category; found is false
	// for an unknown category.
	Children(ctx context.Context, admin, user, category resource.ID) (lines []Line, found bool, err error)
}

// Administrations checks access to an administration.
type Administrations interface {
	Get(ctx context.Context, id, user resource.ID) (administration.Listed, error)
}

// Store persists templates, lists, groups and tasks. Creating a list,
// template or group stores its groups and tasks with it.
type Store interface {
	Templates(ctx context.Context, admin resource.ID) ([]Template, error)
	Template(ctx context.Context, admin, id resource.ID) (Template, error)
	SaveTemplate(ctx context.Context, t Template, created bool) error
	DeleteTemplate(ctx context.Context, admin, id resource.ID) error
	// Lists returns a page of lists, newest first, and the number in all.
	Lists(ctx context.Context, admin resource.ID, page, perPage int) ([]List, int, error)
	List(ctx context.Context, admin, id resource.ID) (List, error)
	SaveList(ctx context.Context, l List, created bool) error
	DeleteList(ctx context.Context, admin, id resource.ID) error
	Group(ctx context.Context, admin, id resource.ID) (Group, error)
	// GroupsOf returns the groups of the lists and templates, without tasks.
	GroupsOf(ctx context.Context, admin resource.ID, owners []resource.ID) ([]Group, error)
	SaveGroup(ctx context.Context, g Group, created bool) error
	DeleteGroup(ctx context.Context, admin, id resource.ID) error
	Task(ctx context.Context, admin, id resource.ID) (Task, error)
	// TasksOf returns every task in the groups, flat.
	TasksOf(ctx context.Context, admin resource.ID, groups []resource.ID) ([]Task, error)
	// SaveTasks stores tasks, flat; created ones with their subtasks.
	SaveTasks(ctx context.Context, tasks []Task, created bool) error
	// ReplaceTasks deletes the group's tasks, or the parent's generated
	// subtasks, and stores tasks in their place.
	ReplaceTasks(ctx context.Context, admin, group resource.ID, parent *resource.ID, tasks []Task) error
	DeleteTask(ctx context.Context, admin, id resource.ID) error
	IsMember(ctx context.Context, admin, user resource.ID) (bool, error)
}

// Notes keeps notes on tasks.
type Notes interface {
	Add(ctx context.Context, admin resource.ID, entityType string, entity, user resource.ID, in notes.Input) (notes.Note, error)
	Of(ctx context.Context, admin resource.ID, entityType string, entities []resource.ID) (map[resource.ID][]notes.Note, error)
}

// Service implements the task list use cases.
type Service struct {
	admins  Administrations
	store   Store
	notes   Notes
	reports Reports
	ids     *resource.Generator
	now     func() time.Time
}

// NewService returns a Service.
func NewService(admins Administrations, store Store, notes Notes, reports Reports, ids *resource.Generator, now func() time.Time) *Service {
	return &Service{admins: admins, store: store, notes: notes, reports: reports, ids: ids, now: now}
}

func deref[T any](p *T) T {
	var zero T
	if p == nil {
		return zero
	}
	return *p
}

// today is the date in the administration's time zone.
func (s *Service) today(ctx context.Context, admin, user resource.ID) (time.Time, error) {
	a, err := s.admins.Get(ctx, admin, user)
	if err != nil {
		return time.Time{}, err
	}
	now := s.now()
	if loc, err := time.LoadLocation(a.TimeZone); err == nil {
		now = now.In(loc)
	}
	y, m, d := now.Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC), nil
}

// fill loads the groups, tasks and notes of lists and templates.
func (s *Service) fill(ctx context.Context, admin resource.ID, owners []resource.ID) (map[resource.ID][]Group, error) {
	out := map[resource.ID][]Group{}
	if len(owners) == 0 {
		return out, nil
	}
	groups, err := s.store.GroupsOf(ctx, admin, owners)
	if err != nil {
		return nil, fmt.Errorf("load groups: %w", err)
	}
	if err := s.withTasks(ctx, admin, groups); err != nil {
		return nil, err
	}
	for _, g := range groups {
		out[g.owner()] = append(out[g.owner()], g)
	}
	return out, nil
}

// withTasks loads the groups' tasks as trees, with their notes.
func (s *Service) withTasks(ctx context.Context, admin resource.ID, groups []Group) error {
	ids := make([]resource.ID, len(groups))
	for i, g := range groups {
		ids[i] = g.ID
	}
	tasks, err := s.store.TasksOf(ctx, admin, ids)
	if err != nil {
		return fmt.Errorf("load tasks: %w", err)
	}
	if err := s.withNotes(ctx, admin, tasks); err != nil {
		return err
	}
	for i := range groups {
		groups[i].Tasks = tree(tasks, groups[i].ID, nil)
	}
	return nil
}

func (s *Service) withNotes(ctx context.Context, admin resource.ID, tasks []Task) error {
	ids := make([]resource.ID, len(tasks))
	for i, t := range tasks {
		ids[i] = t.ID
	}
	byID := map[resource.ID][]notes.Note{}
	if len(ids) > 0 {
		var err error
		if byID, err = s.notes.Of(ctx, admin, NoteEntityType, ids); err != nil {
			return fmt.Errorf("load notes: %w", err)
		}
	}
	for i := range tasks {
		tasks[i].Notes = byID[tasks[i].ID]
	}
	return nil
}

// tree nests the group's tasks below parent, by row order.
func tree(tasks []Task, group resource.ID, parent *resource.ID) []Task {
	out := []Task{}
	for _, t := range tasks {
		if t.GroupID == group && (parent == nil && t.ParentID == nil || parent != nil && t.ParentID != nil && *t.ParentID == *parent) {
			t.Subtasks = tree(tasks, group, &t.ID)
			out = append(out, t)
		}
	}
	slices.SortStableFunc(out, func(a, b Task) int {
		return cmp.Or(cmp.Compare(a.RowOrder, b.RowOrder), a.CreatedAt.Compare(b.CreatedAt))
	})
	return out
}

// TemplateInput holds a template's fields to set; nil ones stay.
type TemplateInput struct {
	Name, Description, Period, DeadlineAfter *string
}

// Templates returns the administration's templates by name.
func (s *Service) Templates(ctx context.Context, admin, user resource.ID) ([]Template, error) {
	if _, err := s.admins.Get(ctx, admin, user); err != nil {
		return nil, err
	}
	list, err := s.store.Templates(ctx, admin)
	if err != nil {
		return nil, fmt.Errorf("list templates: %w", err)
	}
	ids := make([]resource.ID, len(list))
	for i, t := range list {
		ids[i] = t.ID
	}
	groups, err := s.fill(ctx, admin, ids)
	if err != nil {
		return nil, err
	}
	for i := range list {
		list[i].Groups = groups[list[i].ID]
	}
	return list, nil
}

// Template returns one template with its groups and tasks.
func (s *Service) Template(ctx context.Context, admin, id, user resource.ID) (Template, error) {
	if _, err := s.admins.Get(ctx, admin, user); err != nil {
		return Template{}, err
	}
	return s.template(ctx, admin, id)
}

func (s *Service) template(ctx context.Context, admin, id resource.ID) (Template, error) {
	t, err := s.store.Template(ctx, admin, id)
	if err != nil {
		return Template{}, err
	}
	groups, err := s.fill(ctx, admin, []resource.ID{id})
	t.Groups = groups[id]
	return t, err
}

// CreateTemplate adds a template with its first group.
func (s *Service) CreateTemplate(ctx context.Context, admin, user resource.ID, in TemplateInput) (Template, error) {
	if _, err := s.admins.Get(ctx, admin, user); err != nil {
		return Template{}, err
	}
	now := s.now()
	t := Template{ID: s.ids.Next(), AdministrationID: admin, CreatedAt: now, UpdatedAt: now}
	if err := applyTemplate(&t, in); err != nil {
		return Template{}, err
	}
	t.Groups = []Group{s.firstGroup(admin, nil, &t.ID, now)}
	if err := s.store.SaveTemplate(ctx, t, true); err != nil {
		return Template{}, fmt.Errorf("store template: %w", err)
	}
	return t, nil
}

func (s *Service) firstGroup(admin resource.ID, list, template *resource.ID, now time.Time) Group {
	return Group{ID: s.ids.Next(), AdministrationID: admin, ListID: list, TemplateID: template, First: true, CreatedAt: now, UpdatedAt: now, Tasks: []Task{}}
}

func applyTemplate(t *Template, in TemplateInput) error {
	var verr resource.ValidationError
	if in.Name != nil {
		t.Name = strings.TrimSpace(*in.Name)
	}
	checkName(&verr, t.Name)
	if in.Description != nil {
		t.Description = strings.TrimSpace(*in.Description)
	}
	if in.Period != nil {
		p, err := NormalizePeriod(*in.Period)
		if err != nil {
			verr.Add("period", err.Error())
		}
		t.Period = p
	}
	if in.DeadlineAfter != nil {
		d := strings.ToUpper(strings.TrimSpace(*in.DeadlineAfter))
		if _, err := ParseDuration(d); d != "" && err != nil {
			verr.Add("deadline_after", err.Error())
		}
		t.DeadlineAfter = d
	}
	return verr.OrNil()
}

func checkName(verr *resource.ValidationError, name string) {
	switch {
	case name == "":
		verr.Add("name", "can't be blank")
	case len(name) > 255:
		verr.Add("name", "is too long (maximum is 255 characters)")
	}
}

// UpdateTemplate changes a template.
func (s *Service) UpdateTemplate(ctx context.Context, admin, id, user resource.ID, in TemplateInput) (Template, error) {
	t, err := s.Template(ctx, admin, id, user)
	if err != nil {
		return Template{}, err
	}
	if err := applyTemplate(&t, in); err != nil {
		return Template{}, err
	}
	t.UpdatedAt = s.now()
	if err := s.store.SaveTemplate(ctx, t, false); err != nil {
		return Template{}, fmt.Errorf("store template: %w", err)
	}
	return t, nil
}

// DeleteTemplate removes a template with its groups and tasks; lists made
// from it stay.
func (s *Service) DeleteTemplate(ctx context.Context, admin, id, user resource.ID) error {
	if _, err := s.Template(ctx, admin, id, user); err != nil {
		return err
	}
	return s.store.DeleteTemplate(ctx, admin, id)
}

// ListInput holds a list's fields to set; nil ones stay.
type ListInput struct {
	Name, Description *string
	DeadlineOn        *time.Time
	// ClearDeadline removes the deadline.
	ClearDeadline          bool
	PeriodBegin, PeriodEnd *time.Time
	TemplateID             *resource.ID
}

// Lists returns a page of lists, newest first, with their tasks.
func (s *Service) Lists(ctx context.Context, admin, user resource.ID, page, perPage int) ([]List, int, error) {
	if _, err := s.admins.Get(ctx, admin, user); err != nil {
		return nil, 0, err
	}
	page = max(page, 1)
	if perPage <= 0 {
		perPage = DefaultPerPage
	}
	perPage = min(perPage, MaxPerPage)
	list, total, err := s.store.Lists(ctx, admin, page, perPage)
	if err != nil {
		return nil, 0, fmt.Errorf("list task lists: %w", err)
	}
	ids := make([]resource.ID, len(list))
	for i, l := range list {
		ids[i] = l.ID
	}
	groups, err := s.fill(ctx, admin, ids)
	if err != nil {
		return nil, 0, err
	}
	for i := range list {
		list[i].Groups = groups[list[i].ID]
	}
	return list, total, nil
}

// List returns one list with its groups and tasks.
func (s *Service) List(ctx context.Context, admin, id, user resource.ID) (List, error) {
	if _, err := s.admins.Get(ctx, admin, user); err != nil {
		return List{}, err
	}
	return s.list(ctx, admin, id)
}

func (s *Service) list(ctx context.Context, admin, id resource.ID) (List, error) {
	l, err := s.store.List(ctx, admin, id)
	if err != nil {
		return List{}, err
	}
	groups, err := s.fill(ctx, admin, []resource.ID{id})
	l.Groups = groups[id]
	return l, err
}

// CreateList adds a list. With a template it is made from the template;
// otherwise it gets a first group and covers last month unless a period
// is given.
func (s *Service) CreateList(ctx context.Context, admin, user resource.ID, in ListInput) (List, error) {
	if in.TemplateID != nil {
		return s.FromTemplate(ctx, admin, *in.TemplateID, user, in)
	}
	today, err := s.today(ctx, admin, user)
	if err != nil {
		return List{}, err
	}
	now := s.now()
	l := List{ID: s.ids.Next(), AdministrationID: admin, CreatedAt: now, UpdatedAt: now}
	if in.PeriodBegin == nil && in.PeriodEnd == nil {
		begin, end := previous(today, "P1M")
		l.PeriodBegin, l.PeriodEnd = begin, end
	}
	if err := applyList(&l, in, true); err != nil {
		return List{}, err
	}
	l.Groups = []Group{s.firstGroup(admin, &l.ID, nil, now)}
	if err := s.store.SaveList(ctx, l, true); err != nil {
		return List{}, fmt.Errorf("store task list: %w", err)
	}
	return l, nil
}

// FromTemplate makes a list from a template: its name and description
// unless given, its groups and tasks, the period before the current one,
// and a deadline that long after the period's end.
func (s *Service) FromTemplate(ctx context.Context, admin, template, user resource.ID, in ListInput) (List, error) {
	today, err := s.today(ctx, admin, user)
	if err != nil {
		return List{}, err
	}
	t, err := s.template(ctx, admin, template)
	if err != nil {
		return List{}, err
	}
	now := s.now()
	l := List{ID: s.ids.Next(), AdministrationID: admin, TemplateID: &t.ID, Name: t.Name, Description: t.Description, CreatedAt: now, UpdatedAt: now}
	if in.PeriodBegin == nil && in.PeriodEnd == nil && t.Period != "" {
		l.PeriodBegin, l.PeriodEnd = previous(today, t.Period)
	}
	in.TemplateID = nil
	if err := applyList(&l, in, true); err != nil {
		return List{}, err
	}
	if in.DeadlineOn == nil && l.PeriodEnd != nil && t.DeadlineAfter != "" {
		if d, err := ParseDuration(t.DeadlineAfter); err == nil {
			deadline := d.After(*l.PeriodEnd)
			l.DeadlineOn = &deadline
		}
	}
	for _, g := range t.Groups {
		copied := g
		copied.ID, copied.ListID, copied.TemplateID, copied.CreatedAt, copied.UpdatedAt = s.ids.Next(), &l.ID, nil, now, now
		copied.Tasks = s.copyTasks(g.Tasks, copied.ID, nil, now)
		l.Groups = append(l.Groups, copied)
	}
	if err := s.store.SaveList(ctx, l, true); err != nil {
		return List{}, fmt.Errorf("store task list: %w", err)
	}
	return l, nil
}

func (s *Service) copyTasks(tasks []Task, group resource.ID, parent *resource.ID, now time.Time) []Task {
	out := make([]Task, len(tasks))
	for i, t := range tasks {
		t.ID, t.GroupID, t.ParentID, t.CompletedAt, t.CreatedAt, t.UpdatedAt, t.Notes = s.ids.Next(), group, parent, nil, now, now, nil
		t.Subtasks = s.copyTasks(t.Subtasks, group, &t.ID, now)
		out[i] = t
	}
	return out
}

func applyList(l *List, in ListInput, created bool) error {
	var verr resource.ValidationError
	if in.Name != nil {
		l.Name = strings.TrimSpace(*in.Name)
	}
	checkName(&verr, l.Name)
	if in.Description != nil {
		l.Description = strings.TrimSpace(*in.Description)
	}
	if in.DeadlineOn != nil {
		l.DeadlineOn = in.DeadlineOn
	} else if in.ClearDeadline {
		l.DeadlineOn = nil
	}
	if created && (in.PeriodBegin != nil || in.PeriodEnd != nil) {
		l.PeriodBegin, l.PeriodEnd = in.PeriodBegin, in.PeriodEnd
		switch {
		case l.PeriodBegin == nil:
			verr.Add("period_begin", "can't be blank when period_end is given")
		case l.PeriodEnd == nil:
			verr.Add("period_end", "can't be blank when period_begin is given")
		case l.PeriodEnd.Before(*l.PeriodBegin):
			verr.Add("period_end", "must be on or after period_begin")
		}
	}
	return verr.OrNil()
}

// UpdateList changes a list's name, description or deadline.
func (s *Service) UpdateList(ctx context.Context, admin, id, user resource.ID, in ListInput) (List, error) {
	l, err := s.List(ctx, admin, id, user)
	if err != nil {
		return List{}, err
	}
	if err := applyList(&l, in, false); err != nil {
		return List{}, err
	}
	l.UpdatedAt = s.now()
	if err := s.store.SaveList(ctx, l, false); err != nil {
		return List{}, fmt.Errorf("store task list: %w", err)
	}
	return l, nil
}

// DeleteList removes a list with its groups and tasks.
func (s *Service) DeleteList(ctx context.Context, admin, id, user resource.ID) error {
	if _, err := s.List(ctx, admin, id, user); err != nil {
		return err
	}
	return s.store.DeleteList(ctx, admin, id)
}

// GroupInput holds a group's fields to set; nil ones stay.
type GroupInput struct {
	Name       *string
	ReportType *string
}

// Group returns one group with its tasks.
func (s *Service) Group(ctx context.Context, admin, id, user resource.ID) (Group, error) {
	if _, err := s.admins.Get(ctx, admin, user); err != nil {
		return Group{}, err
	}
	return s.group(ctx, admin, id)
}

func (s *Service) group(ctx context.Context, admin, id resource.ID) (Group, error) {
	g, err := s.store.Group(ctx, admin, id)
	if err != nil {
		return Group{}, err
	}
	groups := []Group{g}
	err = s.withTasks(ctx, admin, groups)
	return groups[0], err
}

// AddListGroup adds a group at the end of a list.
func (s *Service) AddListGroup(ctx context.Context, admin, list, user resource.ID, in GroupInput) (Group, error) {
	if _, err := s.List(ctx, admin, list, user); err != nil {
		return Group{}, err
	}
	return s.addGroup(ctx, admin, user, Group{ListID: &list}, in)
}

// AddTemplateGroup adds a group at the end of a template.
func (s *Service) AddTemplateGroup(ctx context.Context, admin, template, user resource.ID, in GroupInput) (Group, error) {
	if _, err := s.Template(ctx, admin, template, user); err != nil {
		return Group{}, err
	}
	return s.addGroup(ctx, admin, user, Group{TemplateID: &template}, in)
}

func (s *Service) addGroup(ctx context.Context, admin, user resource.ID, g Group, in GroupInput) (Group, error) {
	siblings, err := s.store.GroupsOf(ctx, admin, []resource.ID{g.owner()})
	if err != nil {
		return Group{}, fmt.Errorf("load groups: %w", err)
	}
	now := s.now()
	g.ID, g.AdministrationID, g.CreatedAt, g.UpdatedAt, g.Tasks = s.ids.Next(), admin, now, now, []Task{}
	for _, sib := range siblings {
		g.RowOrder = max(g.RowOrder, sib.RowOrder+1)
	}
	if in.Name == nil {
		in.Name = new(string)
	}
	if err := applyGroup(&g, in); err != nil {
		return Group{}, err
	}
	if g.ReportType != "" {
		if g.Tasks, err = s.generate(ctx, admin, user, g, now); err != nil {
			return Group{}, err
		}
	}
	if err := s.store.SaveGroup(ctx, g, true); err != nil {
		return Group{}, fmt.Errorf("store group: %w", err)
	}
	return g, nil
}

func applyGroup(g *Group, in GroupInput) error {
	var verr resource.ValidationError
	if in.Name != nil {
		name := strings.TrimSpace(*in.Name)
		checkName(&verr, name)
		g.Name = &name
	}
	if in.ReportType != nil {
		if *in.ReportType != "" && !slices.Contains(ReportTypes, *in.ReportType) {
			verr.Add("report_type", "is not one of "+strings.Join(ReportTypes, ", "))
		}
		g.ReportType = *in.ReportType
	}
	return verr.OrNil()
}

// generate makes the tasks of a group that follows a report.
func (s *Service) generate(ctx context.Context, admin, user resource.ID, g Group, now time.Time) ([]Task, error) {
	lines, err := s.reports.ReportLines(ctx, admin, user, g.ReportType)
	if err != nil {
		return nil, fmt.Errorf("read report: %w", err)
	}
	out := make([]Task, len(lines))
	for i, l := range lines {
		out[i] = Task{ID: s.ids.Next(), AdministrationID: admin, GroupID: g.ID, Name: l.Name, RowOrder: i, ReportType: g.ReportType,
			RelatedEntityID: l.EntityID, Generated: true, CreatedAt: now, UpdatedAt: now, Subtasks: []Task{}}
	}
	return out, nil
}

// UpdateGroup renames a group or changes its report; a new report replaces
// all of its tasks by ones generated from it. The first group stays.
func (s *Service) UpdateGroup(ctx context.Context, admin, id, user resource.ID, in GroupInput) (Group, error) {
	g, err := s.Group(ctx, admin, id, user)
	if err != nil {
		return Group{}, err
	}
	if g.First {
		return Group{}, rule("The first group cannot be modified")
	}
	before := g.ReportType
	if err := applyGroup(&g, in); err != nil {
		return Group{}, err
	}
	now := s.now()
	g.UpdatedAt = now
	if err := s.store.SaveGroup(ctx, g, false); err != nil {
		return Group{}, fmt.Errorf("store group: %w", err)
	}
	if g.ReportType != before {
		tasks := []Task{}
		if g.ReportType != "" {
			if tasks, err = s.generate(ctx, admin, user, g, now); err != nil {
				return Group{}, err
			}
		}
		if err := s.store.ReplaceTasks(ctx, admin, g.ID, nil, tasks); err != nil {
			return Group{}, fmt.Errorf("replace tasks: %w", err)
		}
		g.Tasks = tasks
	}
	return g, nil
}

// DeleteGroup removes a group with its tasks; the first group stays.
func (s *Service) DeleteGroup(ctx context.Context, admin, id, user resource.ID) error {
	g, err := s.Group(ctx, admin, id, user)
	if err != nil {
		return err
	}
	if g.First {
		return rule("The first group cannot be deleted")
	}
	return s.store.DeleteGroup(ctx, admin, id)
}

// TaskInput holds a task's fields to set; nil ones stay. A
// RelatedEntityID of 0 unlinks the category.
type TaskInput struct {
	Name, Description, ReportType *string
	AssigneeID                    *resource.ID
	RelatedEntityID               *resource.ID
	// GenerateSubtasks makes a subtask per sub-category of the related
	// category.
	GenerateSubtasks *bool
}

// Task returns one task with its subtasks and notes.
func (s *Service) Task(ctx context.Context, admin, id, user resource.ID) (Task, error) {
	if _, err := s.admins.Get(ctx, admin, user); err != nil {
		return Task{}, err
	}
	return s.task(ctx, admin, id)
}

func (s *Service) task(ctx context.Context, admin, id resource.ID) (Task, error) {
	t, err := s.store.Task(ctx, admin, id)
	if err != nil {
		return Task{}, err
	}
	all, err := s.store.TasksOf(ctx, admin, []resource.ID{t.GroupID})
	if err != nil {
		return Task{}, fmt.Errorf("load tasks: %w", err)
	}
	if err := s.withNotes(ctx, admin, all); err != nil {
		return Task{}, err
	}
	for _, o := range all {
		if o.ID == id {
			t = o
		}
	}
	t.Subtasks = tree(all, t.GroupID, &t.ID)
	return t, nil
}

// AddTask adds a task at the end of a group; a group that follows a report
// takes no others.
func (s *Service) AddTask(ctx context.Context, admin, group, user resource.ID, in TaskInput) (Task, error) {
	g, err := s.Group(ctx, admin, group, user)
	if err != nil {
		return Task{}, err
	}
	if g.ReportType != "" {
		return Task{}, rule("Tasks cannot be added to a group with generated tasks")
	}
	now := s.now()
	t := Task{ID: s.ids.Next(), AdministrationID: admin, GroupID: group, CreatedAt: now, UpdatedAt: now, Subtasks: []Task{}, Notes: []notes.Note{}}
	for _, sib := range g.Tasks {
		t.RowOrder = max(t.RowOrder, sib.RowOrder+1)
	}
	if in.Name == nil {
		in.Name = new(string)
	}
	if err := s.applyTask(ctx, admin, user, &t, in); err != nil {
		return Task{}, err
	}
	if err := s.store.SaveTasks(ctx, []Task{t}, true); err != nil {
		return Task{}, fmt.Errorf("store task: %w", err)
	}
	return t, nil
}

func (s *Service) applyTask(ctx context.Context, admin, user resource.ID, t *Task, in TaskInput) error {
	var verr resource.ValidationError
	if in.Name != nil {
		t.Name = strings.TrimSpace(*in.Name)
		checkName(&verr, t.Name)
	}
	if in.Description != nil {
		t.Description = strings.TrimSpace(*in.Description)
	}
	if in.ReportType != nil {
		if *in.ReportType != "" && !slices.Contains(ReportTypes, *in.ReportType) {
			verr.Add("report_type", "is not one of "+strings.Join(ReportTypes, ", "))
		}
		t.ReportType = *in.ReportType
	}
	if in.AssigneeID != nil {
		member, err := s.store.IsMember(ctx, admin, *in.AssigneeID)
		if err != nil {
			return fmt.Errorf("check assignee: %w", err)
		}
		if member {
			t.AssigneeID = in.AssigneeID
		} else {
			verr.Add("assignee_id", "is not a user of this administration")
		}
	}
	var children []Line
	if in.RelatedEntityID != nil {
		if *in.RelatedEntityID == 0 {
			t.RelatedEntityID = nil
		} else {
			lines, found, err := s.reports.Children(ctx, admin, user, *in.RelatedEntityID)
			if err != nil {
				return fmt.Errorf("check category: %w", err)
			}
			if found {
				t.RelatedEntityID, children = in.RelatedEntityID, lines
			} else {
				verr.Add("related_entity_id", "is not a category of this administration")
			}
		}
	}
	if err := verr.OrNil(); err != nil {
		return err
	}
	if deref(in.GenerateSubtasks) {
		if t.RelatedEntityID == nil {
			verr.Add("generate_subtasks", "needs a related_entity_id")
			return &verr
		}
		if in.RelatedEntityID == nil {
			lines, _, err := s.reports.Children(ctx, admin, user, *t.RelatedEntityID)
			if err != nil {
				return fmt.Errorf("read sub-categories: %w", err)
			}
			children = lines
		}
		kept := slices.DeleteFunc(slices.Clone(t.Subtasks), func(sub Task) bool { return sub.Generated })
		for i, l := range children {
			kept = append(kept, Task{ID: s.ids.Next(), AdministrationID: admin, GroupID: t.GroupID, ParentID: &t.ID, Name: l.Name, RowOrder: len(kept) + i,
				RelatedEntityID: l.EntityID, Generated: true, CreatedAt: t.UpdatedAt, UpdatedAt: t.UpdatedAt, Subtasks: []Task{}, Notes: []notes.Note{}})
		}
		t.Subtasks = kept
	}
	return nil
}

// UpdateTask changes a task.
func (s *Service) UpdateTask(ctx context.Context, admin, id, user resource.ID, in TaskInput) (Task, error) {
	t, err := s.Task(ctx, admin, id, user)
	if err != nil {
		return Task{}, err
	}
	t.UpdatedAt = s.now()
	if err := s.applyTask(ctx, admin, user, &t, in); err != nil {
		return Task{}, err
	}
	if err := s.store.SaveTasks(ctx, []Task{t}, false); err != nil {
		return Task{}, fmt.Errorf("store task: %w", err)
	}
	if deref(in.GenerateSubtasks) {
		generated := slices.DeleteFunc(slices.Clone(t.Subtasks), func(sub Task) bool { return !sub.Generated })
		if err := s.store.ReplaceTasks(ctx, admin, t.GroupID, &t.ID, generated); err != nil {
			return Task{}, fmt.Errorf("replace subtasks: %w", err)
		}
	}
	return t, nil
}

// DeleteTask removes a task with its subtasks; generated tasks stay.
func (s *Service) DeleteTask(ctx context.Context, admin, id, user resource.ID) error {
	t, err := s.Task(ctx, admin, id, user)
	if err != nil {
		return err
	}
	if t.Generated {
		return rule("Generated tasks cannot be deleted")
	}
	return s.store.DeleteTask(ctx, admin, id)
}

// Assign gives a task to a member of the administration.
func (s *Service) Assign(ctx context.Context, admin, id, user, assignee resource.ID) (Task, error) {
	t, err := s.Task(ctx, admin, id, user)
	if err != nil {
		return Task{}, err
	}
	member, err := s.store.IsMember(ctx, admin, assignee)
	if err != nil {
		return Task{}, fmt.Errorf("check assignee: %w", err)
	}
	if !member {
		return Task{}, ErrAssigneeNotFound
	}
	t.AssigneeID, t.UpdatedAt = &assignee, s.now()
	return t, s.saveOne(ctx, t)
}

// Unassign takes a task from whoever it was given to.
func (s *Service) Unassign(ctx context.Context, admin, id, user resource.ID) (Task, error) {
	t, err := s.Task(ctx, admin, id, user)
	if err != nil {
		return Task{}, err
	}
	t.AssigneeID, t.UpdatedAt = nil, s.now()
	return t, s.saveOne(ctx, t)
}

func (s *Service) saveOne(ctx context.Context, t Task) error {
	if err := s.store.SaveTasks(ctx, []Task{t}, false); err != nil {
		return fmt.Errorf("store task: %w", err)
	}
	return nil
}

// Outcome is a task after completing or reopening it, with what that did
// to its list.
type Outcome struct {
	Task Task
	// List is the list the task is on, if it is on one.
	List *List
	// ListCompleted and ListReopened tell a change of the list's state.
	ListCompleted, ListReopened bool
}

// Complete marks a task and its subtasks done.
func (s *Service) Complete(ctx context.Context, admin, id, user resource.ID) (Outcome, error) {
	return s.mark(ctx, admin, id, user, true)
}

// Reopen marks a task and the tasks above it not done.
func (s *Service) Reopen(ctx context.Context, admin, id, user resource.ID) (Outcome, error) {
	return s.mark(ctx, admin, id, user, false)
}

func (s *Service) mark(ctx context.Context, admin, id, user resource.ID, done bool) (Outcome, error) {
	t, err := s.Task(ctx, admin, id, user)
	if err != nil {
		return Outcome{}, err
	}
	g, err := s.store.Group(ctx, admin, t.GroupID)
	if err != nil {
		return Outcome{}, err
	}
	var before *List
	if g.ListID != nil {
		l, err := s.list(ctx, admin, *g.ListID)
		if err != nil {
			return Outcome{}, err
		}
		before = &l
	}
	now := s.now()
	var changed []Task
	if done {
		var walk func(*Task)
		walk = func(x *Task) {
			if x.CompletedAt == nil {
				x.CompletedAt, x.UpdatedAt = &now, now
				changed = append(changed, *x)
			}
			for i := range x.Subtasks {
				walk(&x.Subtasks[i])
			}
		}
		walk(&t)
	} else {
		if t.CompletedAt != nil {
			t.CompletedAt, t.UpdatedAt = nil, now
			changed = append(changed, t)
		}
		for parent := t.ParentID; parent != nil; {
			p, err := s.store.Task(ctx, admin, *parent)
			if err != nil {
				return Outcome{}, err
			}
			if p.CompletedAt != nil {
				p.CompletedAt, p.UpdatedAt = nil, now
				changed = append(changed, p)
			}
			parent = p.ParentID
		}
	}
	if len(changed) > 0 {
		if err := s.store.SaveTasks(ctx, changed, false); err != nil {
			return Outcome{}, fmt.Errorf("store tasks: %w", err)
		}
	}
	out := Outcome{Task: t}
	if before != nil {
		after, err := s.list(ctx, admin, before.ID)
		if err != nil {
			return Outcome{}, err
		}
		out.List = &after
		out.ListCompleted = !before.Completed() && after.Completed()
		out.ListReopened = before.Completed() && !after.Completed()
	}
	return out, nil
}

// AddNote adds a note to a task.
func (s *Service) AddNote(ctx context.Context, admin, id, user resource.ID, in notes.Input) (notes.Note, error) {
	if _, err := s.Task(ctx, admin, id, user); err != nil {
		return notes.Note{}, err
	}
	return s.notes.Add(ctx, admin, NoteEntityType, id, user, in)
}
