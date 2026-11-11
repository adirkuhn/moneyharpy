package postgres

import (
	"context"
	"errors"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/adirkuhn/moneyharpy/internal/postgres/sqlc"
	"github.com/adirkuhn/moneyharpy/internal/resource"
	"github.com/adirkuhn/moneyharpy/internal/tasklists"
	"github.com/adirkuhn/moneyharpy/internal/timeentries"
)

// TimeEntryStore implements timeentries.Store.
type TimeEntryStore struct {
	pool *pgxpool.Pool
	q    *sqlc.Queries
}

// NewTimeEntryStore returns a TimeEntryStore on pool.
func NewTimeEntryStore(pool *pgxpool.Pool) *TimeEntryStore {
	return &TimeEntryStore{pool: pool, q: sqlc.New(pool)}
}

func toTimeEntry(r sqlc.TimeEntry) timeentries.TimeEntry {
	return timeentries.TimeEntry{
		ID: resource.ID(r.ID), AdministrationID: resource.ID(r.AdministrationID), UserID: resource.ID(r.UserID), ContactID: idOf(r.ContactID),
		ProjectID: idOf(r.ProjectID), SalesInvoiceID: idOf(r.SalesInvoiceID), StartedAt: r.StartedAt.Time, EndedAt: timeOf(r.EndedAt),
		Description: r.Description, PausedDuration: int(r.PausedDuration), Billable: r.Billable, CreatedAt: r.CreatedAt.Time, UpdatedAt: r.UpdatedAt.Time,
	}
}

func toTimeEntries(rows []sqlc.TimeEntry) []timeentries.TimeEntry {
	out := make([]timeentries.TimeEntry, len(rows))
	for i, r := range rows {
		out[i] = toTimeEntry(r)
	}
	return out
}

// Entries implements timeentries.Store.
func (s *TimeEntryStore) Entries(ctx context.Context, admin resource.ID, q timeentries.Query) ([]timeentries.TimeEntry, int, error) {
	zone := "UTC"
	if q.Location != nil {
		zone = q.Location.String()
	}
	p := sqlc.ListTimeEntriesParams{
		AdministrationID: int64(admin), IncludeActive: q.IncludeActive, FromDay: optionalDate(q.From), ToDay: optionalDate(q.To), OnDay: optionalDate(q.Day),
		TimeZone: zone, ContactID: optionalID(q.ContactID), IncludeNilContacts: q.IncludeNilContacts, ProjectID: optionalID(q.ProjectID),
		UserID: optionalID(q.UserID), Search: likeEscaper.Replace(strings.TrimSpace(q.Search)), States: nonNil(q.States),
	}
	p.MaxRows, p.Skip = limit(q.Page, q.PerPage)
	rows, err := s.q.ListTimeEntries(ctx, p)
	if err != nil {
		return nil, 0, err
	}
	out := make([]timeentries.TimeEntry, len(rows))
	total := 0
	for i, r := range rows {
		out[i], total = toTimeEntry(r.TimeEntry), int(r.Total)
	}
	if len(rows) == 0 && p.Skip > 0 {
		p.MaxRows, p.Skip = pgtype.Int4{Int32: 1, Valid: true}, 0
		if all, err := s.q.ListTimeEntries(ctx, p); err == nil && len(all) > 0 {
			total = int(all[0].Total)
		}
	}
	return out, total, nil
}

// Entry implements timeentries.Store.
func (s *TimeEntryStore) Entry(ctx context.Context, admin, id resource.ID) (timeentries.TimeEntry, error) {
	r, err := s.q.TimeEntryByID(ctx, sqlc.TimeEntryByIDParams{AdministrationID: int64(admin), ID: int64(id)})
	if errors.Is(err, pgx.ErrNoRows) {
		return timeentries.TimeEntry{}, timeentries.ErrNotFound
	}
	if err != nil {
		return timeentries.TimeEntry{}, err
	}
	return toTimeEntry(r), nil
}

// RunningOf implements timeentries.Store.
func (s *TimeEntryStore) RunningOf(ctx context.Context, admin, user resource.ID) ([]timeentries.TimeEntry, error) {
	rows, err := s.q.RunningTimeEntries(ctx, sqlc.RunningTimeEntriesParams{AdministrationID: int64(admin), UserID: int64(user)})
	return toTimeEntries(rows), err
}

// OfInvoices implements timeentries.Store.
func (s *TimeEntryStore) OfInvoices(ctx context.Context, admin resource.ID, invoices []resource.ID) ([]timeentries.TimeEntry, error) {
	rows, err := s.q.TimeEntriesOfInvoices(ctx, sqlc.TimeEntriesOfInvoicesParams{AdministrationID: int64(admin), Ids: ids64(invoices)})
	return toTimeEntries(rows), err
}

// SaveEntry implements timeentries.Store.
func (s *TimeEntryStore) SaveEntry(ctx context.Context, e timeentries.TimeEntry, created bool) error {
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		q := s.q.WithTx(tx)
		if err := q.UpsertTimeEntry(ctx, sqlc.UpsertTimeEntryParams{
			ID: int64(e.ID), AdministrationID: int64(e.AdministrationID), UserID: int64(e.UserID), ContactID: optionalID(e.ContactID),
			ProjectID: optionalID(e.ProjectID), SalesInvoiceID: optionalID(e.SalesInvoiceID), StartedAt: timestamp(e.StartedAt), EndedAt: optionalTime(e.EndedAt),
			Description: e.Description, PausedDuration: int32(e.PausedDuration), Billable: e.Billable, CreatedAt: timestamp(e.CreatedAt), //nolint:gosec // Bounded by the service.
			UpdatedAt: timestamp(e.UpdatedAt),
		}); err != nil {
			return err
		}
		if !created {
			return nil
		}
		return q.InsertResource(ctx, sqlc.InsertResourceParams{ID: int64(e.ID), Type: string(resource.TypeTimeEntry), AdministrationID: int64(e.AdministrationID)})
	})
}

// DeleteEntry implements timeentries.Store.
func (s *TimeEntryStore) DeleteEntry(ctx context.Context, admin, id resource.ID) error {
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		q := s.q.WithTx(tx)
		if err := q.DeleteTimeEntry(ctx, sqlc.DeleteTimeEntryParams{AdministrationID: int64(admin), ID: int64(id)}); err != nil {
			return err
		}
		return q.DeleteResource(ctx, int64(id))
	})
}

// Users implements timeentries.Store.
func (s *TimeEntryStore) Users(ctx context.Context, admin resource.ID, ids []resource.ID) (map[resource.ID]timeentries.User, error) {
	rows, err := s.q.TimeEntryUsers(ctx, sqlc.TimeEntryUsersParams{AdministrationID: int64(admin), Ids: ids64(ids)})
	if err != nil {
		return nil, err
	}
	out := make(map[resource.ID]timeentries.User, len(rows))
	for _, r := range rows {
		out[resource.ID(r.ID)] = timeentries.User{ID: resource.ID(r.ID), Name: r.Name, CreatedAt: r.CreatedAt.Time, UpdatedAt: r.UpdatedAt.Time}
	}
	return out, nil
}

// ContactExists implements timeentries.Store.
func (s *TimeEntryStore) ContactExists(ctx context.Context, admin, id resource.ID) (bool, error) {
	return s.q.ContactExists(ctx, sqlc.ContactExistsParams{AdministrationID: int64(admin), ID: int64(id)})
}

// ProjectExists implements timeentries.Store.
func (s *TimeEntryStore) ProjectExists(ctx context.Context, admin, id resource.ID) (bool, error) {
	return s.q.ProjectExists(ctx, sqlc.ProjectExistsParams{AdministrationID: int64(admin), ID: int64(id)})
}

// InvoiceExists implements timeentries.Store.
func (s *TimeEntryStore) InvoiceExists(ctx context.Context, admin, id resource.ID) (bool, error) {
	return s.q.SalesInvoiceExists(ctx, sqlc.SalesInvoiceExistsParams{AdministrationID: int64(admin), ID: int64(id)})
}

// TaskListStore implements tasklists.Store.
type TaskListStore struct {
	pool *pgxpool.Pool
	q    *sqlc.Queries
}

// NewTaskListStore returns a TaskListStore on pool.
func NewTaskListStore(pool *pgxpool.Pool) *TaskListStore {
	return &TaskListStore{pool: pool, q: sqlc.New(pool)}
}

func toTemplate(r sqlc.TaskListTemplate) tasklists.Template {
	return tasklists.Template{
		ID: resource.ID(r.ID), AdministrationID: resource.ID(r.AdministrationID), Name: r.Name, Description: r.Description, Period: r.Period,
		DeadlineAfter: r.DeadlineAfter, CreatedAt: r.CreatedAt.Time, UpdatedAt: r.UpdatedAt.Time,
	}
}

// Templates implements tasklists.Store.
func (s *TaskListStore) Templates(ctx context.Context, admin resource.ID) ([]tasklists.Template, error) {
	rows, err := s.q.ListTaskListTemplates(ctx, int64(admin))
	if err != nil {
		return nil, err
	}
	out := make([]tasklists.Template, len(rows))
	for i, r := range rows {
		out[i] = toTemplate(r)
	}
	return out, nil
}

// Template implements tasklists.Store.
func (s *TaskListStore) Template(ctx context.Context, admin, id resource.ID) (tasklists.Template, error) {
	r, err := s.q.TaskListTemplateByID(ctx, sqlc.TaskListTemplateByIDParams{AdministrationID: int64(admin), ID: int64(id)})
	if errors.Is(err, pgx.ErrNoRows) {
		return tasklists.Template{}, tasklists.ErrTemplateNotFound
	}
	if err != nil {
		return tasklists.Template{}, err
	}
	return toTemplate(r), nil
}

// SaveTemplate implements tasklists.Store.
func (s *TaskListStore) SaveTemplate(ctx context.Context, t tasklists.Template, created bool) error {
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		q := s.q.WithTx(tx)
		if err := q.UpsertTaskListTemplate(ctx, sqlc.UpsertTaskListTemplateParams{
			ID: int64(t.ID), AdministrationID: int64(t.AdministrationID), Name: t.Name, Description: t.Description, Period: t.Period,
			DeadlineAfter: t.DeadlineAfter, CreatedAt: timestamp(t.CreatedAt), UpdatedAt: timestamp(t.UpdatedAt),
		}); err != nil || !created {
			return err
		}
		if err := insertGroups(ctx, q, t.Groups); err != nil {
			return err
		}
		return q.InsertResource(ctx, sqlc.InsertResourceParams{ID: int64(t.ID), Type: string(resource.TypeTaskListTemplate), AdministrationID: int64(t.AdministrationID)})
	})
}

// DeleteTemplate implements tasklists.Store.
func (s *TaskListStore) DeleteTemplate(ctx context.Context, admin, id resource.ID) error {
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		q := s.q.WithTx(tx)
		if err := q.DeleteTaskListTemplate(ctx, sqlc.DeleteTaskListTemplateParams{AdministrationID: int64(admin), ID: int64(id)}); err != nil {
			return err
		}
		return q.DeleteResource(ctx, int64(id))
	})
}

func toTaskList(r sqlc.TaskList) tasklists.List {
	return tasklists.List{
		ID: resource.ID(r.ID), AdministrationID: resource.ID(r.AdministrationID), TemplateID: idOf(r.TemplateID), Name: r.Name, Description: r.Description,
		DeadlineOn: dateOf(r.DeadlineOn), PeriodBegin: dateOf(r.PeriodBegin), PeriodEnd: dateOf(r.PeriodEnd), CreatedAt: r.CreatedAt.Time, UpdatedAt: r.UpdatedAt.Time,
	}
}

// Lists implements tasklists.Store.
func (s *TaskListStore) Lists(ctx context.Context, admin resource.ID, page, perPage int) ([]tasklists.List, int, error) {
	p := sqlc.ListTaskListsParams{AdministrationID: int64(admin)}
	p.MaxRows, p.Skip = limit(page, perPage)
	rows, err := s.q.ListTaskLists(ctx, p)
	if err != nil {
		return nil, 0, err
	}
	out := make([]tasklists.List, len(rows))
	total := 0
	for i, r := range rows {
		out[i], total = toTaskList(r.TaskList), int(r.Total)
	}
	if len(rows) == 0 && p.Skip > 0 {
		p.MaxRows, p.Skip = pgtype.Int4{Int32: 1, Valid: true}, 0
		if all, err := s.q.ListTaskLists(ctx, p); err == nil && len(all) > 0 {
			total = int(all[0].Total)
		}
	}
	return out, total, nil
}

// List implements tasklists.Store.
func (s *TaskListStore) List(ctx context.Context, admin, id resource.ID) (tasklists.List, error) {
	r, err := s.q.TaskListByID(ctx, sqlc.TaskListByIDParams{AdministrationID: int64(admin), ID: int64(id)})
	if errors.Is(err, pgx.ErrNoRows) {
		return tasklists.List{}, tasklists.ErrListNotFound
	}
	if err != nil {
		return tasklists.List{}, err
	}
	return toTaskList(r), nil
}

// SaveList implements tasklists.Store.
func (s *TaskListStore) SaveList(ctx context.Context, l tasklists.List, created bool) error {
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		q := s.q.WithTx(tx)
		if err := q.UpsertTaskList(ctx, sqlc.UpsertTaskListParams{
			ID: int64(l.ID), AdministrationID: int64(l.AdministrationID), TemplateID: optionalID(l.TemplateID), Name: l.Name, Description: l.Description,
			DeadlineOn: optionalDate(l.DeadlineOn), PeriodBegin: optionalDate(l.PeriodBegin), PeriodEnd: optionalDate(l.PeriodEnd),
			CreatedAt: timestamp(l.CreatedAt), UpdatedAt: timestamp(l.UpdatedAt),
		}); err != nil || !created {
			return err
		}
		if err := insertGroups(ctx, q, l.Groups); err != nil {
			return err
		}
		return q.InsertResource(ctx, sqlc.InsertResourceParams{ID: int64(l.ID), Type: string(resource.TypeTaskList), AdministrationID: int64(l.AdministrationID)})
	})
}

// DeleteList implements tasklists.Store.
func (s *TaskListStore) DeleteList(ctx context.Context, admin, id resource.ID) error {
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		q := s.q.WithTx(tx)
		if err := q.DeleteTaskList(ctx, sqlc.DeleteTaskListParams{AdministrationID: int64(admin), ID: int64(id)}); err != nil {
			return err
		}
		return q.DeleteResource(ctx, int64(id))
	})
}

func toGroup(r sqlc.TaskListGroup) tasklists.Group {
	g := tasklists.Group{
		ID: resource.ID(r.ID), AdministrationID: resource.ID(r.AdministrationID), ListID: idOf(r.TaskListID), TemplateID: idOf(r.TemplateID),
		First: r.IsFirst, RowOrder: int(r.RowOrder), ReportType: r.ReportType, CreatedAt: r.CreatedAt.Time, UpdatedAt: r.UpdatedAt.Time,
	}
	if r.Name.Valid {
		g.Name = &r.Name.String
	}
	return g
}

func upsertGroup(ctx context.Context, q *sqlc.Queries, g tasklists.Group) error {
	name := pgtype.Text{}
	if g.Name != nil {
		name = pgtype.Text{String: *g.Name, Valid: true}
	}
	return q.UpsertTaskListGroup(ctx, sqlc.UpsertTaskListGroupParams{
		ID: int64(g.ID), AdministrationID: int64(g.AdministrationID), TaskListID: optionalID(g.ListID), TemplateID: optionalID(g.TemplateID), Name: name,
		IsFirst: g.First, RowOrder: int32(g.RowOrder), ReportType: g.ReportType, CreatedAt: timestamp(g.CreatedAt), UpdatedAt: timestamp(g.UpdatedAt), //nolint:gosec // Row orders are small.
	})
}

func insertGroups(ctx context.Context, q *sqlc.Queries, groups []tasklists.Group) error {
	for _, g := range groups {
		if err := upsertGroup(ctx, q, g); err != nil {
			return err
		}
		if err := upsertTasks(ctx, q, g.Tasks, true); err != nil {
			return err
		}
	}
	return nil
}

// Group implements tasklists.Store.
func (s *TaskListStore) Group(ctx context.Context, admin, id resource.ID) (tasklists.Group, error) {
	r, err := s.q.TaskListGroupByID(ctx, sqlc.TaskListGroupByIDParams{AdministrationID: int64(admin), ID: int64(id)})
	if errors.Is(err, pgx.ErrNoRows) {
		return tasklists.Group{}, tasklists.ErrGroupNotFound
	}
	if err != nil {
		return tasklists.Group{}, err
	}
	return toGroup(r), nil
}

// GroupsOf implements tasklists.Store.
func (s *TaskListStore) GroupsOf(ctx context.Context, admin resource.ID, owners []resource.ID) ([]tasklists.Group, error) {
	rows, err := s.q.TaskListGroupsOf(ctx, sqlc.TaskListGroupsOfParams{AdministrationID: int64(admin), Owners: ids64(owners)})
	if err != nil {
		return nil, err
	}
	out := make([]tasklists.Group, len(rows))
	for i, r := range rows {
		out[i] = toGroup(r)
	}
	return out, nil
}

// SaveGroup implements tasklists.Store.
func (s *TaskListStore) SaveGroup(ctx context.Context, g tasklists.Group, created bool) error {
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		q := s.q.WithTx(tx)
		if !created {
			return upsertGroup(ctx, q, g)
		}
		return insertGroups(ctx, q, []tasklists.Group{g})
	})
}

// DeleteGroup implements tasklists.Store.
func (s *TaskListStore) DeleteGroup(ctx context.Context, admin, id resource.ID) error {
	return s.q.DeleteTaskListGroup(ctx, sqlc.DeleteTaskListGroupParams{AdministrationID: int64(admin), ID: int64(id)})
}

func toTask(r sqlc.TaskListTask) tasklists.Task {
	return tasklists.Task{
		ID: resource.ID(r.ID), AdministrationID: resource.ID(r.AdministrationID), GroupID: resource.ID(r.GroupID), ParentID: idOf(r.ParentTaskID),
		Name: r.Name, Description: r.Description, RowOrder: int(r.RowOrder), CompletedAt: timeOf(r.CompletedAt), ReportType: r.ReportType,
		AssigneeID: idOf(r.AssigneeID), RelatedEntityID: idOf(r.RelatedEntityID), Generated: r.Generated, CreatedAt: r.CreatedAt.Time, UpdatedAt: r.UpdatedAt.Time,
	}
}

// upsertTasks stores tasks; created ones with their subtasks, parents
// first.
func upsertTasks(ctx context.Context, q *sqlc.Queries, tasks []tasklists.Task, created bool) error {
	for _, t := range tasks {
		if err := q.UpsertTaskListTask(ctx, sqlc.UpsertTaskListTaskParams{
			ID: int64(t.ID), AdministrationID: int64(t.AdministrationID), GroupID: int64(t.GroupID), ParentTaskID: optionalID(t.ParentID), Name: t.Name,
			Description: t.Description, RowOrder: int32(t.RowOrder), CompletedAt: optionalTime(t.CompletedAt), ReportType: t.ReportType, //nolint:gosec // Row orders are small.
			AssigneeID: optionalID(t.AssigneeID), RelatedEntityID: optionalID(t.RelatedEntityID), Generated: t.Generated,
			CreatedAt: timestamp(t.CreatedAt), UpdatedAt: timestamp(t.UpdatedAt),
		}); err != nil {
			return err
		}
		if created {
			if err := upsertTasks(ctx, q, t.Subtasks, true); err != nil {
				return err
			}
		}
	}
	return nil
}

// Task implements tasklists.Store.
func (s *TaskListStore) Task(ctx context.Context, admin, id resource.ID) (tasklists.Task, error) {
	r, err := s.q.TaskListTaskByID(ctx, sqlc.TaskListTaskByIDParams{AdministrationID: int64(admin), ID: int64(id)})
	if errors.Is(err, pgx.ErrNoRows) {
		return tasklists.Task{}, tasklists.ErrTaskNotFound
	}
	if err != nil {
		return tasklists.Task{}, err
	}
	return toTask(r), nil
}

// TasksOf implements tasklists.Store.
func (s *TaskListStore) TasksOf(ctx context.Context, admin resource.ID, groups []resource.ID) ([]tasklists.Task, error) {
	rows, err := s.q.TaskListTasksOf(ctx, sqlc.TaskListTasksOfParams{AdministrationID: int64(admin), Groups: ids64(groups)})
	if err != nil {
		return nil, err
	}
	out := make([]tasklists.Task, len(rows))
	for i, r := range rows {
		out[i] = toTask(r)
	}
	return out, nil
}

// SaveTasks implements tasklists.Store.
func (s *TaskListStore) SaveTasks(ctx context.Context, tasks []tasklists.Task, created bool) error {
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error { return upsertTasks(ctx, s.q.WithTx(tx), tasks, created) })
}

// ReplaceTasks implements tasklists.Store.
func (s *TaskListStore) ReplaceTasks(ctx context.Context, admin, group resource.ID, parent *resource.ID, tasks []tasklists.Task) error {
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		q := s.q.WithTx(tx)
		var err error
		if parent == nil {
			err = q.DeleteTaskListGroupTasks(ctx, sqlc.DeleteTaskListGroupTasksParams{AdministrationID: int64(admin), GroupID: int64(group)})
		} else {
			err = q.DeleteGeneratedSubtasks(ctx, sqlc.DeleteGeneratedSubtasksParams{AdministrationID: int64(admin), ParentTaskID: optionalID(parent)})
		}
		if err != nil {
			return err
		}
		return upsertTasks(ctx, q, tasks, true)
	})
}

// DeleteTask implements tasklists.Store.
func (s *TaskListStore) DeleteTask(ctx context.Context, admin, id resource.ID) error {
	return s.q.DeleteTaskListTask(ctx, sqlc.DeleteTaskListTaskParams{AdministrationID: int64(admin), ID: int64(id)})
}

// IsMember implements tasklists.Store.
func (s *TaskListStore) IsMember(ctx context.Context, admin, user resource.ID) (bool, error) {
	return s.q.IsMember(ctx, sqlc.IsMemberParams{AdministrationID: int64(admin), UserID: int64(user)})
}

// ContactInUse implements contacts.References: time entries keep their
// contact.
func (s *TimeEntryStore) ContactInUse(ctx context.Context, admin, contact resource.ID) (bool, error) {
	return s.q.ContactHasTimeEntries(ctx, sqlc.ContactHasTimeEntriesParams{AdministrationID: int64(admin), ContactID: pgtype.Int8{Int64: int64(contact), Valid: true}})
}

// ProjectInUse implements projects.References: time entries keep their
// project.
func (s *TimeEntryStore) ProjectInUse(ctx context.Context, admin, project resource.ID) (bool, error) {
	return s.q.ProjectHasTimeEntries(ctx, sqlc.ProjectHasTimeEntriesParams{AdministrationID: int64(admin), ProjectID: pgtype.Int8{Int64: int64(project), Valid: true}})
}
