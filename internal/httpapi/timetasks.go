package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/adirkuhn/moneyharpy/internal/administration"
	"github.com/adirkuhn/moneyharpy/internal/contacts"
	"github.com/adirkuhn/moneyharpy/internal/notes"
	"github.com/adirkuhn/moneyharpy/internal/projects"
	"github.com/adirkuhn/moneyharpy/internal/resource"
	"github.com/adirkuhn/moneyharpy/internal/sales"
	"github.com/adirkuhn/moneyharpy/internal/tasklists"
	"github.com/adirkuhn/moneyharpy/internal/timeentries"
	"github.com/adirkuhn/moneyharpy/internal/webhooks"
)

// failTimeTasks answers the errors of time entries and task lists with
// Moneybird's messages, and others as usual.
func (s *server) failTimeTasks(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case timeentries.IsNotFound(err) && !errors.Is(err, notes.ErrNotFound):
		writeError(w, http.StatusNotFound, "record not found")
	case errors.Is(err, timeentries.ErrForbidden):
		writeError(w, http.StatusForbidden, "Your user has no permission for this: the time entry is another user's")
	case tasklists.IsNotFound(err) && !errors.Is(err, notes.ErrNotFound):
		// Moneybird's sentence, such as "Record not found for model name: List".
		msg := err.Error()
		writeError(w, http.StatusNotFound, strings.ToUpper(msg[:1])+msg[1:])
	case errors.Is(err, notes.ErrNotFound):
		writeError(w, http.StatusNotFound, "Not found")
	default:
		s.fail(w, r, err)
	}
}

// timeActor is the user working on time entries: with the sales_invoices
// permission, on everyone's.
func (s *server) timeActor(r *http.Request, admin resource.ID) (timeentries.Actor, error) {
	a := timeentries.Actor{ID: user(r).ID}
	if g, ok := grantFrom(r.Context()); ok && !g.Allows(administration.SalesInvoices) {
		return a, nil
	}
	l, err := s.deps.Administrations.Get(r.Context(), admin, a.ID)
	if err != nil {
		return a, err
	}
	a.AllUsers = l.Can(administration.SalesInvoices)
	return a, nil
}

type minimalContactJSON struct {
	ID          string  `json:"id"`
	CompanyName *string `json:"company_name"`
	Firstname   *string `json:"firstname"`
	Lastname    *string `json:"lastname"`
}

type baseUserJSON struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type timeEntryJSON struct {
	ID               string              `json:"id"`
	AdministrationID string              `json:"administration_id"`
	ContactID        *string             `json:"contact_id"`
	ProjectID        *string             `json:"project_id"`
	SalesInvoiceID   *string             `json:"sales_invoice_id"`
	UserID           string              `json:"user_id"`
	StartedAt        time.Time           `json:"started_at"`
	EndedAt          *time.Time          `json:"ended_at"`
	Description      string              `json:"description"`
	PausedDuration   int                 `json:"paused_duration"`
	Billable         bool                `json:"billable"`
	CreatedAt        time.Time           `json:"created_at"`
	UpdatedAt        time.Time           `json:"updated_at"`
	Contact          *minimalContactJSON `json:"contact"`
	User             baseUserJSON        `json:"user"`
	Project          *projectJSON        `json:"project"`
	SalesInvoice     *salesInvoiceJSON   `json:"sales_invoice"`
	Events           []any               `json:"events"`
	Notes            []noteJSON          `json:"notes"`
	URI              string              `json:"uri"`
}

// minimalTimeEntryJSON is a time entry as sales invoices show it.
type minimalTimeEntryJSON struct {
	ID               string     `json:"id"`
	AdministrationID string     `json:"administration_id"`
	ContactID        *string    `json:"contact_id"`
	ProjectID        *string    `json:"project_id"`
	UserID           string     `json:"user_id"`
	StartedAt        time.Time  `json:"started_at"`
	EndedAt          *time.Time `json:"ended_at"`
	Description      *string    `json:"description"`
	PausedDuration   int        `json:"paused_duration"`
	Billable         bool       `json:"billable"`
	CreatedAt        time.Time  `json:"created_at"`
	UpdatedAt        time.Time  `json:"updated_at"`
}

func utc(t *time.Time) *time.Time {
	if t == nil {
		return nil
	}
	v := t.UTC()
	return &v
}

func toMinimalTimeEntry(e timeentries.TimeEntry) minimalTimeEntryJSON {
	return minimalTimeEntryJSON{
		ID: e.ID.String(), AdministrationID: e.AdministrationID.String(), ContactID: idString(e.ContactID), ProjectID: idString(e.ProjectID),
		UserID: e.UserID.String(), StartedAt: e.StartedAt.UTC(), EndedAt: utc(e.EndedAt), Description: nz(e.Description),
		PausedDuration: e.PausedDuration, Billable: e.Billable, CreatedAt: e.CreatedAt.UTC(), UpdatedAt: e.UpdatedAt.UTC(),
	}
}

// billedTimeEntries returns the time entries on each invoice, as invoices
// show them.
func (s *server) billedTimeEntries(ctx context.Context, admin resource.ID, invoices []resource.ID) (map[resource.ID][]any, error) {
	out := map[resource.ID][]any{}
	if s.deps.TimeEntries == nil {
		return out, nil
	}
	byInvoice, err := s.deps.TimeEntries.OfInvoices(ctx, admin, invoices)
	if err != nil {
		return nil, err
	}
	for id, list := range byInvoice {
		for _, e := range list {
			out[id] = append(out[id], toMinimalTimeEntry(e))
		}
	}
	return out, nil
}

// toTimeEntries answers with entries and what they point at, loaded once
// per record.
func (s *server) toTimeEntries(r *http.Request, admin resource.ID, list []timeentries.TimeEntry) ([]timeEntryJSON, error) {
	ctx, me := r.Context(), user(r).ID
	var userIDs, contactIDs []resource.ID
	projectsByID := map[resource.ID]*projectJSON{}
	invoicesByID := map[resource.ID]*salesInvoiceJSON{}
	for _, e := range list {
		userIDs = append(userIDs, e.UserID)
		if e.ContactID != nil {
			contactIDs = append(contactIDs, *e.ContactID)
		}
		if e.ProjectID != nil {
			projectsByID[*e.ProjectID] = nil
		}
		if e.SalesInvoiceID != nil {
			invoicesByID[*e.SalesInvoiceID] = nil
		}
	}
	users, err := s.deps.TimeEntries.Users(ctx, admin, userIDs)
	if err != nil {
		return nil, err
	}
	contactsByID := map[resource.ID]contacts.Contact{}
	for start := 0; start < len(contactIDs); start += contacts.MaxFetch {
		found, err := s.deps.Contacts.ContactsByID(ctx, admin, me, contactIDs[start:min(start+contacts.MaxFetch, len(contactIDs))])
		if err != nil {
			return nil, err
		}
		for _, c := range found {
			contactsByID[c.ID] = c
		}
	}
	for id := range projectsByID {
		p, err := s.deps.Projects.Project(ctx, admin, id, me)
		if errors.Is(err, projects.ErrNotFound) {
			continue
		}
		if err != nil {
			return nil, err
		}
		pj := toProject(p)
		projectsByID[id] = &pj
	}
	// Only those who may work on invoices see the invoice the time is on.
	actor, err := s.timeActor(r, admin)
	if err != nil {
		return nil, err
	}
	if !actor.AllUsers {
		clear(invoicesByID)
	}
	var invoices []sales.Invoice
	for id := range invoicesByID {
		inv, err := s.deps.Sales.Invoice(ctx, admin, id, me)
		if sales.IsNotFound(err) {
			continue
		}
		if err != nil {
			return nil, err
		}
		invoices = append(invoices, inv)
	}
	if len(invoices) > 0 {
		shown, err := s.toSalesInvoices(r, admin, invoices)
		if err != nil {
			return nil, err
		}
		for i := range shown {
			inv := shown[i]
			invoicesByID[invoices[i].ID] = &inv
		}
	}
	out := make([]timeEntryJSON, len(list))
	for i, e := range list {
		u := users[e.UserID]
		out[i] = timeEntryJSON{
			ID: e.ID.String(), AdministrationID: e.AdministrationID.String(), ContactID: idString(e.ContactID), ProjectID: idString(e.ProjectID),
			SalesInvoiceID: idString(e.SalesInvoiceID), UserID: e.UserID.String(), StartedAt: e.StartedAt.UTC(), EndedAt: utc(e.EndedAt),
			Description: e.Description, PausedDuration: e.PausedDuration, Billable: e.Billable, CreatedAt: e.CreatedAt.UTC(), UpdatedAt: e.UpdatedAt.UTC(),
			User:   baseUserJSON{ID: e.UserID.String(), Name: u.Name, CreatedAt: u.CreatedAt.UTC(), UpdatedAt: u.UpdatedAt.UTC()},
			Events: []any{}, Notes: []noteJSON{}, URI: e.Ref().URI(),
		}
		if e.ContactID != nil {
			if c, ok := contactsByID[*e.ContactID]; ok {
				out[i].Contact = &minimalContactJSON{ID: c.ID.String(), CompanyName: nz(c.CompanyName), Firstname: nz(c.Firstname), Lastname: nz(c.Lastname)}
			}
		}
		if e.ProjectID != nil {
			out[i].Project = projectsByID[*e.ProjectID]
		}
		if e.SalesInvoiceID != nil {
			out[i].SalesInvoice = invoicesByID[*e.SalesInvoiceID]
		}
		for _, n := range e.Notes {
			out[i].Notes = append(out[i].Notes, toNote(n))
		}
	}
	return out, nil
}

func (s *server) writeTimeEntry(w http.ResponseWriter, r *http.Request, admin resource.ID, status int, e timeentries.TimeEntry, err error) {
	if err != nil {
		s.failTimeTasks(w, r, err)
		return
	}
	out, err := s.toTimeEntries(r, admin, []timeentries.TimeEntry{e})
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, status, out[0])
}

// optionalRef reads an id that null or an empty value clears, as 0.
func optionalRef(w http.ResponseWriter, p presence) (*resource.ID, bool) {
	if !p.sent {
		return nil, true
	}
	raw := bytes.TrimSpace(p.raw)
	if bytes.Equal(raw, []byte("null")) {
		return ptr(resource.ID(0)), true
	}
	var f flexString
	if err := json.Unmarshal(raw, &f); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return nil, false
	}
	return flexOptionalID(w, &f)
}

func readTimeEntryInput(w http.ResponseWriter, r *http.Request) (timeentries.Input, bool) {
	body, ok := decodeBody[struct {
		TimeEntry struct {
			UserID         *flexString `json:"user_id"`
			StartedAt      *time.Time  `json:"started_at"`
			EndedAt        *time.Time  `json:"ended_at"`
			Description    *string     `json:"description"`
			ContactID      presence    `json:"contact_id"`
			ProjectID      presence    `json:"project_id"`
			SalesInvoiceID presence    `json:"sales_invoice_id"`
			Billable       *flexBool   `json:"billable"`
			PausedDuration *flexString `json:"paused_duration"`
		} `json:"time_entry"`
	}](w, r)
	if !ok {
		return timeentries.Input{}, false
	}
	b := body.TimeEntry
	in := timeentries.Input{StartedAt: b.StartedAt, EndedAt: b.EndedAt, Description: b.Description, Billable: b.Billable.ptr()}
	if in.UserID, ok = flexID(w, b.UserID); !ok {
		return in, false
	}
	for _, f := range []struct {
		src presence
		dst **resource.ID
	}{{b.ContactID, &in.ContactID}, {b.ProjectID, &in.ProjectID}, {b.SalesInvoiceID, &in.SalesInvoiceID}} {
		if *f.dst, ok = optionalRef(w, f.src); !ok {
			return in, false
		}
	}
	if in.PausedDuration, ok = flexInt(w, b.PausedDuration, "paused_duration"); !ok {
		return in, false
	}
	return in, true
}

// timeEntryCall reads the ids and the actor of a call on one entry.
func (s *server) timeEntryCall(w http.ResponseWriter, r *http.Request, administrationID AdministrationId, id Id) (resource.ID, resource.ID, timeentries.Actor, bool) {
	admin, rid, ok := parseIDs(w, administrationID, id)
	if !ok {
		return 0, 0, timeentries.Actor{}, false
	}
	actor, err := s.timeActor(r, admin)
	if err != nil {
		s.fail(w, r, err)
		return 0, 0, actor, false
	}
	return admin, rid, actor, true
}

// ListTimeEntries implements ServerInterface.
func (s *server) ListTimeEntries(w http.ResponseWriter, r *http.Request, administrationID AdministrationId, params ListTimeEntriesParams) {
	admin, ok := parseID(w, administrationID)
	if !ok {
		return
	}
	actor, err := s.timeActor(r, admin)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	today, err := s.today(r, admin)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	q, err := timeentries.ParseFilter(deref(params.Filter), today)
	if err != nil {
		writeError(w, http.StatusBadRequest, filterProblem(err))
		return
	}
	q.Search = deref(params.Query)
	q.Page, q.PerPage = pageNumbers(params.Page, params.PerPage)
	list, total, err := s.deps.TimeEntries.List(r.Context(), admin, actor, q)
	if err != nil {
		s.failTimeTasks(w, r, err)
		return
	}
	out, err := s.toTimeEntries(r, admin, list)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.writeLinks(w, r, max(q.Page, 1), pageSize(q.PerPage, timeentries.DefaultPerPage, timeentries.MaxPerPage), total)
	writeJSON(w, http.StatusOK, out)
}

// CreateTimeEntry implements ServerInterface.
func (s *server) CreateTimeEntry(w http.ResponseWriter, r *http.Request, administrationID AdministrationId) {
	admin, ok := parseID(w, administrationID)
	if !ok {
		return
	}
	actor, err := s.timeActor(r, admin)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	in, ok := readTimeEntryInput(w, r)
	if !ok {
		return
	}
	e, err := s.deps.TimeEntries.Create(r.Context(), admin, actor, in)
	s.writeTimeEntry(w, r, admin, http.StatusCreated, e, err)
}

// GetTimeEntry implements ServerInterface.
func (s *server) GetTimeEntry(w http.ResponseWriter, r *http.Request, administrationID AdministrationId, id Id) {
	admin, rid, actor, ok := s.timeEntryCall(w, r, administrationID, id)
	if !ok {
		return
	}
	e, err := s.deps.TimeEntries.Entry(r.Context(), admin, rid, actor)
	s.writeTimeEntry(w, r, admin, http.StatusOK, e, err)
}

// UpdateTimeEntry implements ServerInterface.
func (s *server) UpdateTimeEntry(w http.ResponseWriter, r *http.Request, administrationID AdministrationId, id Id) {
	admin, rid, actor, ok := s.timeEntryCall(w, r, administrationID, id)
	if !ok {
		return
	}
	in, ok := readTimeEntryInput(w, r)
	if !ok {
		return
	}
	e, err := s.deps.TimeEntries.Update(r.Context(), admin, rid, actor, in)
	s.writeTimeEntry(w, r, admin, http.StatusOK, e, err)
}

// DeleteTimeEntry implements ServerInterface.
func (s *server) DeleteTimeEntry(w http.ResponseWriter, r *http.Request, administrationID AdministrationId, id Id) {
	admin, rid, actor, ok := s.timeEntryCall(w, r, administrationID, id)
	if !ok {
		return
	}
	if err := s.deps.TimeEntries.Delete(r.Context(), admin, rid, actor); err != nil {
		s.failTimeTasks(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// StopTimeEntry implements ServerInterface.
func (s *server) StopTimeEntry(w http.ResponseWriter, r *http.Request, administrationID AdministrationId, id Id) {
	admin, rid, actor, ok := s.timeEntryCall(w, r, administrationID, id)
	if !ok {
		return
	}
	e, err := s.deps.TimeEntries.Stop(r.Context(), admin, rid, actor)
	s.writeTimeEntry(w, r, admin, http.StatusOK, e, err)
}

// ResumeTimeEntry implements ServerInterface.
func (s *server) ResumeTimeEntry(w http.ResponseWriter, r *http.Request, administrationID AdministrationId, id Id) {
	admin, rid, actor, ok := s.timeEntryCall(w, r, administrationID, id)
	if !ok {
		return
	}
	e, err := s.deps.TimeEntries.Resume(r.Context(), admin, rid, actor)
	s.writeTimeEntry(w, r, admin, http.StatusOK, e, err)
}

// CreateTimeEntryNote implements ServerInterface.
func (s *server) CreateTimeEntryNote(w http.ResponseWriter, r *http.Request, administrationID AdministrationId, id Id) {
	admin, rid, actor, ok := s.timeEntryCall(w, r, administrationID, id)
	if !ok {
		return
	}
	in, ok := decodeNote(w, r)
	if !ok {
		return
	}
	n, err := s.deps.TimeEntries.AddNote(r.Context(), admin, rid, actor, in)
	if err != nil {
		s.failTimeTasks(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, toNote(n))
}

// DeleteTimeEntryNote implements ServerInterface.
func (s *server) DeleteTimeEntryNote(w http.ResponseWriter, r *http.Request, administrationID AdministrationId, id Id, noteID Identifier) {
	admin, rid, actor, ok := s.timeEntryCall(w, r, administrationID, id)
	if !ok {
		return
	}
	note, ok := parseID(w, noteID)
	if !ok {
		return
	}
	if err := s.deps.TimeEntries.DeleteNote(r.Context(), admin, rid, note, actor); err != nil {
		s.failTimeTasks(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// readTimeEntryIDs reads time_entry_ids of a sales invoice.
func readTimeEntryIDs(w http.ResponseWriter, raw []flexString) ([]resource.ID, bool) {
	out := make([]resource.ID, 0, len(raw))
	for _, f := range raw {
		id, ok := flexID(w, &f)
		if !ok {
			return nil, false
		}
		out = append(out, *id)
	}
	return out, true
}

// checkBillable verifies time entries can go on the invoice, 0 for a new
// one, before it is saved; it answers and reports false when not.
func (s *server) checkBillable(w http.ResponseWriter, r *http.Request, admin, invoice resource.ID, ids []resource.ID) bool {
	if len(ids) == 0 || s.deps.TimeEntries == nil {
		return true
	}
	actor, err := s.timeActor(r, admin)
	if err == nil {
		err = s.deps.TimeEntries.CheckBillable(r.Context(), admin, actor, ids, invoice)
	}
	if err != nil {
		s.failTimeTasks(w, r, err)
		return false
	}
	return true
}

// bill puts time entries on a saved invoice.
func (s *server) bill(w http.ResponseWriter, r *http.Request, admin, invoice resource.ID, ids []resource.ID) bool {
	if len(ids) == 0 || s.deps.TimeEntries == nil {
		return true
	}
	actor, err := s.timeActor(r, admin)
	if err == nil {
		_, err = s.deps.TimeEntries.Bill(r.Context(), admin, invoice, actor, ids)
	}
	if err != nil {
		s.failTimeTasks(w, r, err)
		return false
	}
	return true
}

// taskScopes are the permissions Moneybird asks for task lists, beyond
// time_entries, which the contract checks.
var taskScopes = []administration.Permission{administration.Bank, administration.Documents, administration.Estimates, administration.SalesInvoices, administration.Settings}

// taskCall checks a task list call: Moneybird asks for every permission.
func (s *server) taskCall(w http.ResponseWriter, r *http.Request, administrationID AdministrationId) (resource.ID, bool) {
	admin, ok := parseID(w, administrationID)
	if !ok {
		return 0, false
	}
	if g, ok := grantFrom(r.Context()); ok && !g.Allows(taskScopes...) {
		writeError(w, http.StatusForbidden, "This connection has no scope for this")
		return 0, false
	}
	l, err := s.deps.Administrations.Get(r.Context(), admin, user(r).ID)
	if err != nil {
		s.fail(w, r, err)
		return 0, false
	}
	for _, p := range taskScopes {
		if !l.Can(p) {
			writeError(w, http.StatusForbidden, "Your user has no permission for this: task lists need every permission")
			return 0, false
		}
	}
	return admin, true
}

func (s *server) taskCallID(w http.ResponseWriter, r *http.Request, administrationID AdministrationId, id Id) (resource.ID, resource.ID, bool) {
	admin, ok := s.taskCall(w, r, administrationID)
	if !ok {
		return 0, 0, false
	}
	rid, ok := parseID(w, id)
	return admin, rid, ok
}

type taskJSON struct {
	ID               string     `json:"id"`
	AdministrationID string     `json:"administration_id"`
	Name             string     `json:"name"`
	Description      *string    `json:"description"`
	RowOrder         int        `json:"row_order"`
	CompletedAt      *time.Time `json:"completed_at"`
	ReportType       *string    `json:"report_type"`
	GroupID          string     `json:"task_lists_group_id"`
	ParentTaskID     *string    `json:"parent_task_id"`
	AssigneeID       *string    `json:"assignee_id"`
	RelatedEntityID  *string    `json:"related_entity_id"`
	Generated        bool       `json:"generated"`
	CreatedAt        time.Time  `json:"created_at"`
	UpdatedAt        time.Time  `json:"updated_at"`
	Subtasks         []taskJSON `json:"subtasks"`
	Notes            []noteJSON `json:"notes"`
	Events           []any      `json:"events"`
}

func toTask(t tasklists.Task) taskJSON {
	out := taskJSON{
		ID: t.ID.String(), AdministrationID: t.AdministrationID.String(), Name: t.Name, Description: nz(t.Description), RowOrder: t.RowOrder,
		CompletedAt: utc(t.CompletedAt), ReportType: nz(t.ReportType), GroupID: t.GroupID.String(), ParentTaskID: idString(t.ParentID),
		AssigneeID: idString(t.AssigneeID), RelatedEntityID: idString(t.RelatedEntityID), Generated: t.Generated, CreatedAt: t.CreatedAt.UTC(),
		UpdatedAt: t.UpdatedAt.UTC(), Subtasks: []taskJSON{}, Notes: []noteJSON{}, Events: []any{},
	}
	for _, sub := range t.Subtasks {
		out.Subtasks = append(out.Subtasks, toTask(sub))
	}
	for _, n := range t.Notes {
		out.Notes = append(out.Notes, toNote(n))
	}
	return out
}

type taskGroupJSON struct {
	ID         string     `json:"id"`
	Name       *string    `json:"name"`
	RowOrder   int        `json:"row_order"`
	ReportType *string    `json:"report_type"`
	CreatedAt  time.Time  `json:"created_at"`
	UpdatedAt  time.Time  `json:"updated_at"`
	Tasks      []taskJSON `json:"tasks"`
}

func toTaskGroup(g tasklists.Group) taskGroupJSON {
	out := taskGroupJSON{ID: g.ID.String(), Name: g.Name, RowOrder: g.RowOrder, ReportType: nz(g.ReportType), CreatedAt: g.CreatedAt.UTC(),
		UpdatedAt: g.UpdatedAt.UTC(), Tasks: []taskJSON{}}
	for _, t := range g.Tasks {
		out.Tasks = append(out.Tasks, toTask(t))
	}
	return out
}

func toTaskGroups(groups []tasklists.Group) []taskGroupJSON {
	out := make([]taskGroupJSON, len(groups))
	for i, g := range groups {
		out[i] = toTaskGroup(g)
	}
	return out
}

type taskListJSON struct {
	ID                  string          `json:"id"`
	AdministrationID    string          `json:"administration_id"`
	Name                string          `json:"name"`
	Description         *string         `json:"description"`
	DeadlineOn          *string         `json:"deadline_on"`
	PeriodBegin         *string         `json:"period_begin"`
	PeriodEnd           *string         `json:"period_end"`
	Completed           bool            `json:"completed"`
	TemplateID          *string         `json:"template_id"`
	TotalTasksCount     int             `json:"total_tasks_count"`
	CompletedTasksCount int             `json:"completed_tasks_count"`
	CreatedAt           time.Time       `json:"created_at"`
	UpdatedAt           time.Time       `json:"updated_at"`
	Groups              []taskGroupJSON `json:"groups"`
	URI                 string          `json:"uri"`
}

func toTaskList(l tasklists.List) taskListJSON {
	total, done := l.Counts()
	return taskListJSON{
		ID: l.ID.String(), AdministrationID: l.AdministrationID.String(), Name: l.Name, Description: nz(l.Description), DeadlineOn: dateString(l.DeadlineOn),
		PeriodBegin: dateString(l.PeriodBegin), PeriodEnd: dateString(l.PeriodEnd), Completed: total == done, TemplateID: idString(l.TemplateID),
		TotalTasksCount: total, CompletedTasksCount: done, CreatedAt: l.CreatedAt.UTC(), UpdatedAt: l.UpdatedAt.UTC(), Groups: toTaskGroups(l.Groups), URI: l.Ref().URI(),
	}
}

type taskTemplateJSON struct {
	ID               string          `json:"id"`
	AdministrationID string          `json:"administration_id"`
	Name             string          `json:"name"`
	Description      *string         `json:"description"`
	Period           *string         `json:"period"`
	DeadlineAfter    *string         `json:"deadline_after"`
	CreatedAt        time.Time       `json:"created_at"`
	UpdatedAt        time.Time       `json:"updated_at"`
	Groups           []taskGroupJSON `json:"groups"`
	URI              string          `json:"uri"`
}

func toTaskTemplate(t tasklists.Template) taskTemplateJSON {
	return taskTemplateJSON{
		ID: t.ID.String(), AdministrationID: t.AdministrationID.String(), Name: t.Name, Description: nz(t.Description), Period: nz(t.Period),
		DeadlineAfter: nz(t.DeadlineAfter), CreatedAt: t.CreatedAt.UTC(), UpdatedAt: t.UpdatedAt.UTC(), Groups: toTaskGroups(t.Groups), URI: t.Ref().URI(),
	}
}

// answer writes a value made from a successful result.
func answer[T, J any](s *server, w http.ResponseWriter, r *http.Request, status int, v T, err error, show func(T) J) {
	if err != nil {
		s.failTimeTasks(w, r, err)
		return
	}
	writeJSON(w, status, show(v))
}

func parseDay(w http.ResponseWriter, raw *string, field string) (*time.Time, bool) {
	if raw == nil || *raw == "" {
		return nil, true
	}
	d, err := time.Parse(time.DateOnly, *raw)
	if err != nil {
		writeError(w, http.StatusBadRequest, field+" is not a date (YYYY-MM-DD)")
		return nil, false
	}
	return &d, true
}

func readTaskListInput(w http.ResponseWriter, r *http.Request, optional bool) (tasklists.ListInput, bool) {
	type fields struct {
		Name        *string     `json:"name"`
		Description *string     `json:"description"`
		DeadlineOn  presence    `json:"deadline_on"`
		PeriodBegin *string     `json:"period_begin"`
		PeriodEnd   *string     `json:"period_end"`
		TemplateID  *flexString `json:"template_id"`
	}
	var body struct {
		TaskList fields `json:"task_list"`
	}
	raw, err := io.ReadAll(r.Body)
	if err != nil || (len(bytes.TrimSpace(raw)) > 0 || !optional) && json.Unmarshal(raw, &body) != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return tasklists.ListInput{}, false
	}
	b := body.TaskList
	in := tasklists.ListInput{Name: b.Name, Description: b.Description}
	var ok bool
	if b.DeadlineOn.sent {
		var raw *string
		if err := json.Unmarshal(b.DeadlineOn.raw, &raw); err != nil {
			writeError(w, http.StatusBadRequest, "deadline_on is not a date (YYYY-MM-DD)")
			return in, false
		}
		in.ClearDeadline = raw == nil || *raw == ""
		if in.DeadlineOn, ok = parseDay(w, raw, "deadline_on"); !ok {
			return in, false
		}
	}
	if in.PeriodBegin, ok = parseDay(w, b.PeriodBegin, "period_begin"); !ok {
		return in, false
	}
	if in.PeriodEnd, ok = parseDay(w, b.PeriodEnd, "period_end"); !ok {
		return in, false
	}
	if in.TemplateID, ok = flexID(w, b.TemplateID); !ok {
		return in, false
	}
	return in, true
}

// ListTaskLists implements ServerInterface.
func (s *server) ListTaskLists(w http.ResponseWriter, r *http.Request, administrationID AdministrationId, params ListTaskListsParams) {
	admin, ok := s.taskCall(w, r, administrationID)
	if !ok {
		return
	}
	page, perPage := pageNumbers(params.Page, params.PerPage)
	list, total, err := s.deps.TaskLists.Lists(r.Context(), admin, user(r).ID, page, perPage)
	if err != nil {
		s.failTimeTasks(w, r, err)
		return
	}
	out := make([]taskListJSON, len(list))
	for i, l := range list {
		out[i] = toTaskList(l)
	}
	s.writeLinks(w, r, max(page, 1), pageSize(perPage, tasklists.DefaultPerPage, tasklists.MaxPerPage), total)
	writeJSON(w, http.StatusOK, out)
}

// CreateTaskList implements ServerInterface.
func (s *server) CreateTaskList(w http.ResponseWriter, r *http.Request, administrationID AdministrationId) {
	admin, ok := s.taskCall(w, r, administrationID)
	if !ok {
		return
	}
	in, ok := readTaskListInput(w, r, false)
	if !ok {
		return
	}
	l, err := s.deps.TaskLists.CreateList(r.Context(), admin, user(r).ID, in)
	answer(s, w, r, http.StatusCreated, l, err, toTaskList)
}

// GetTaskList implements ServerInterface.
func (s *server) GetTaskList(w http.ResponseWriter, r *http.Request, administrationID AdministrationId, id Id) {
	admin, rid, ok := s.taskCallID(w, r, administrationID, id)
	if !ok {
		return
	}
	l, err := s.deps.TaskLists.List(r.Context(), admin, rid, user(r).ID)
	answer(s, w, r, http.StatusOK, l, err, toTaskList)
}

// UpdateTaskList implements ServerInterface.
func (s *server) UpdateTaskList(w http.ResponseWriter, r *http.Request, administrationID AdministrationId, id Id) {
	admin, rid, ok := s.taskCallID(w, r, administrationID, id)
	if !ok {
		return
	}
	in, ok := readTaskListInput(w, r, false)
	if !ok {
		return
	}
	in.PeriodBegin, in.PeriodEnd, in.TemplateID = nil, nil, nil
	l, err := s.deps.TaskLists.UpdateList(r.Context(), admin, rid, user(r).ID, in)
	answer(s, w, r, http.StatusOK, l, err, toTaskList)
}

// DeleteTaskList implements ServerInterface.
func (s *server) DeleteTaskList(w http.ResponseWriter, r *http.Request, administrationID AdministrationId, id Id) {
	admin, rid, ok := s.taskCallID(w, r, administrationID, id)
	if !ok {
		return
	}
	if err := s.deps.TaskLists.DeleteList(r.Context(), admin, rid, user(r).ID); err != nil {
		s.failTimeTasks(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func readGroupInput(w http.ResponseWriter, r *http.Request) (tasklists.GroupInput, bool) {
	body, ok := decodeBody[struct {
		Group struct {
			Name       *string `json:"name"`
			ReportType *string `json:"report_type"`
		} `json:"group"`
	}](w, r)
	return tasklists.GroupInput{Name: body.Group.Name, ReportType: body.Group.ReportType}, ok
}

// CreateTaskListGroup implements ServerInterface.
func (s *server) CreateTaskListGroup(w http.ResponseWriter, r *http.Request, administrationID AdministrationId, id Id) {
	admin, rid, ok := s.taskCallID(w, r, administrationID, id)
	if !ok {
		return
	}
	in, ok := readGroupInput(w, r)
	if !ok {
		return
	}
	g, err := s.deps.TaskLists.AddListGroup(r.Context(), admin, rid, user(r).ID, in)
	answer(s, w, r, http.StatusCreated, g, err, toTaskGroup)
}

// CreateTaskListTemplateGroup implements ServerInterface.
func (s *server) CreateTaskListTemplateGroup(w http.ResponseWriter, r *http.Request, administrationID AdministrationId, id Id) {
	admin, rid, ok := s.taskCallID(w, r, administrationID, id)
	if !ok {
		return
	}
	in, ok := readGroupInput(w, r)
	if !ok {
		return
	}
	g, err := s.deps.TaskLists.AddTemplateGroup(r.Context(), admin, rid, user(r).ID, in)
	answer(s, w, r, http.StatusCreated, g, err, toTaskGroup)
}

// GetTaskListGroup implements ServerInterface.
func (s *server) GetTaskListGroup(w http.ResponseWriter, r *http.Request, administrationID AdministrationId, id Id) {
	admin, rid, ok := s.taskCallID(w, r, administrationID, id)
	if !ok {
		return
	}
	g, err := s.deps.TaskLists.Group(r.Context(), admin, rid, user(r).ID)
	answer(s, w, r, http.StatusOK, g, err, toTaskGroup)
}

// UpdateTaskListGroup implements ServerInterface.
func (s *server) UpdateTaskListGroup(w http.ResponseWriter, r *http.Request, administrationID AdministrationId, id Id) {
	admin, rid, ok := s.taskCallID(w, r, administrationID, id)
	if !ok {
		return
	}
	in, ok := readGroupInput(w, r)
	if !ok {
		return
	}
	g, err := s.deps.TaskLists.UpdateGroup(r.Context(), admin, rid, user(r).ID, in)
	answer(s, w, r, http.StatusOK, g, err, toTaskGroup)
}

// DeleteTaskListGroup implements ServerInterface.
func (s *server) DeleteTaskListGroup(w http.ResponseWriter, r *http.Request, administrationID AdministrationId, id Id) {
	admin, rid, ok := s.taskCallID(w, r, administrationID, id)
	if !ok {
		return
	}
	if err := s.deps.TaskLists.DeleteGroup(r.Context(), admin, rid, user(r).ID); err != nil {
		s.failTimeTasks(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func readTaskInput(w http.ResponseWriter, r *http.Request) (tasklists.TaskInput, bool) {
	body, ok := decodeBody[struct {
		Task struct {
			Name             *string     `json:"name"`
			Description      *string     `json:"description"`
			ReportType       *string     `json:"report_type"`
			AssigneeID       *flexString `json:"assignee_id"`
			RelatedEntityID  presence    `json:"related_entity_id"`
			GenerateSubtasks *flexBool   `json:"generate_subtasks"`
		} `json:"task"`
	}](w, r)
	if !ok {
		return tasklists.TaskInput{}, false
	}
	b := body.Task
	in := tasklists.TaskInput{Name: b.Name, Description: b.Description, ReportType: b.ReportType, GenerateSubtasks: b.GenerateSubtasks.ptr()}
	if in.AssigneeID, ok = flexID(w, b.AssigneeID); !ok {
		return in, false
	}
	in.RelatedEntityID, ok = optionalRef(w, b.RelatedEntityID)
	return in, ok
}

// CreateTaskListTask implements ServerInterface.
func (s *server) CreateTaskListTask(w http.ResponseWriter, r *http.Request, administrationID AdministrationId, id Id) {
	admin, rid, ok := s.taskCallID(w, r, administrationID, id)
	if !ok {
		return
	}
	in, ok := readTaskInput(w, r)
	if !ok {
		return
	}
	t, err := s.deps.TaskLists.AddTask(r.Context(), admin, rid, user(r).ID, in)
	answer(s, w, r, http.StatusCreated, t, err, toTask)
}

// GetTaskListTask implements ServerInterface.
func (s *server) GetTaskListTask(w http.ResponseWriter, r *http.Request, administrationID AdministrationId, id Id) {
	admin, rid, ok := s.taskCallID(w, r, administrationID, id)
	if !ok {
		return
	}
	t, err := s.deps.TaskLists.Task(r.Context(), admin, rid, user(r).ID)
	answer(s, w, r, http.StatusOK, t, err, toTask)
}

// UpdateTaskListTask implements ServerInterface.
func (s *server) UpdateTaskListTask(w http.ResponseWriter, r *http.Request, administrationID AdministrationId, id Id) {
	admin, rid, ok := s.taskCallID(w, r, administrationID, id)
	if !ok {
		return
	}
	in, ok := readTaskInput(w, r)
	if !ok {
		return
	}
	t, err := s.deps.TaskLists.UpdateTask(r.Context(), admin, rid, user(r).ID, in)
	answer(s, w, r, http.StatusOK, t, err, toTask)
}

// DeleteTaskListTask implements ServerInterface.
func (s *server) DeleteTaskListTask(w http.ResponseWriter, r *http.Request, administrationID AdministrationId, id Id) {
	admin, rid, ok := s.taskCallID(w, r, administrationID, id)
	if !ok {
		return
	}
	if err := s.deps.TaskLists.DeleteTask(r.Context(), admin, rid, user(r).ID); err != nil {
		s.failTimeTasks(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// AssignTaskListTask implements ServerInterface.
func (s *server) AssignTaskListTask(w http.ResponseWriter, r *http.Request, administrationID AdministrationId, id Id) {
	admin, rid, ok := s.taskCallID(w, r, administrationID, id)
	if !ok {
		return
	}
	body, ok := decodeBody[struct {
		AssigneeID flexString `json:"assignee_id"`
	}](w, r)
	if !ok {
		return
	}
	assignee, ok := flexID(w, &body.AssigneeID)
	if !ok {
		return
	}
	t, err := s.deps.TaskLists.Assign(r.Context(), admin, rid, user(r).ID, *assignee)
	answer(s, w, r, http.StatusOK, t, err, toTask)
}

// UnassignTaskListTask implements ServerInterface.
func (s *server) UnassignTaskListTask(w http.ResponseWriter, r *http.Request, administrationID AdministrationId, id Id) {
	admin, rid, ok := s.taskCallID(w, r, administrationID, id)
	if !ok {
		return
	}
	t, err := s.deps.TaskLists.Unassign(r.Context(), admin, rid, user(r).ID)
	answer(s, w, r, http.StatusOK, t, err, toTask)
}

// CompleteTaskListTask implements ServerInterface.
func (s *server) CompleteTaskListTask(w http.ResponseWriter, r *http.Request, administrationID AdministrationId, id Id) {
	s.markTask(w, r, administrationID, id, true)
}

// ReopenTaskListTask implements ServerInterface.
func (s *server) ReopenTaskListTask(w http.ResponseWriter, r *http.Request, administrationID AdministrationId, id Id) {
	s.markTask(w, r, administrationID, id, false)
}

func (s *server) markTask(w http.ResponseWriter, r *http.Request, administrationID AdministrationId, id Id, done bool) {
	admin, rid, ok := s.taskCallID(w, r, administrationID, id)
	if !ok {
		return
	}
	mark := s.deps.TaskLists.Reopen
	if done {
		mark = s.deps.TaskLists.Complete
	}
	out, err := mark(r.Context(), admin, rid, user(r).ID)
	if err != nil {
		s.failTimeTasks(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, toTask(out.Task))
	// A list that became complete, or open again, tells so too.
	if out.List == nil || s.deps.Webhooks == nil || !out.ListCompleted && !out.ListReopened {
		return
	}
	action := "task_lists_list_reopened"
	if out.ListCompleted {
		action = "task_lists_list_completed"
	}
	raw, _ := json.Marshal(toTaskList(*out.List))
	ctx := context.WithoutCancel(r.Context())
	if err := s.deps.Webhooks.Publish(ctx, admin, webhooks.Event{EntityType: tasklists.ListEntityType, EntityID: out.List.ID.String(), Action: action, Entity: raw}); err != nil {
		s.deps.Logger.ErrorContext(ctx, "publish webhook event", "action", action, "error", err)
	}
}

// CreateTaskListTaskNote implements ServerInterface.
func (s *server) CreateTaskListTaskNote(w http.ResponseWriter, r *http.Request, administrationID AdministrationId, id Id) {
	admin, rid, ok := s.taskCallID(w, r, administrationID, id)
	if !ok {
		return
	}
	in, ok := decodeNote(w, r)
	if !ok {
		return
	}
	n, err := s.deps.TaskLists.AddNote(r.Context(), admin, rid, user(r).ID, in)
	answer(s, w, r, http.StatusCreated, n, err, toNote)
}

func readTemplateInput(w http.ResponseWriter, r *http.Request) (tasklists.TemplateInput, bool) {
	body, ok := decodeBody[struct {
		Template struct {
			Name          *string `json:"name"`
			Description   *string `json:"description"`
			Period        *string `json:"period"`
			DeadlineAfter *string `json:"deadline_after"`
		} `json:"task_list_template"`
	}](w, r)
	t := body.Template
	return tasklists.TemplateInput{Name: t.Name, Description: t.Description, Period: t.Period, DeadlineAfter: t.DeadlineAfter}, ok
}

// ListTaskListTemplates implements ServerInterface.
func (s *server) ListTaskListTemplates(w http.ResponseWriter, r *http.Request, administrationID AdministrationId) {
	admin, ok := s.taskCall(w, r, administrationID)
	if !ok {
		return
	}
	list, err := s.deps.TaskLists.Templates(r.Context(), admin, user(r).ID)
	answer(s, w, r, http.StatusOK, list, err, func(list []tasklists.Template) []taskTemplateJSON {
		out := make([]taskTemplateJSON, len(list))
		for i, t := range list {
			out[i] = toTaskTemplate(t)
		}
		return out
	})
}

// CreateTaskListTemplate implements ServerInterface.
func (s *server) CreateTaskListTemplate(w http.ResponseWriter, r *http.Request, administrationID AdministrationId) {
	admin, ok := s.taskCall(w, r, administrationID)
	if !ok {
		return
	}
	in, ok := readTemplateInput(w, r)
	if !ok {
		return
	}
	t, err := s.deps.TaskLists.CreateTemplate(r.Context(), admin, user(r).ID, in)
	answer(s, w, r, http.StatusCreated, t, err, toTaskTemplate)
}

// GetTaskListTemplate implements ServerInterface.
func (s *server) GetTaskListTemplate(w http.ResponseWriter, r *http.Request, administrationID AdministrationId, id Id) {
	admin, rid, ok := s.taskCallID(w, r, administrationID, id)
	if !ok {
		return
	}
	t, err := s.deps.TaskLists.Template(r.Context(), admin, rid, user(r).ID)
	answer(s, w, r, http.StatusOK, t, err, toTaskTemplate)
}

// UpdateTaskListTemplate implements ServerInterface.
func (s *server) UpdateTaskListTemplate(w http.ResponseWriter, r *http.Request, administrationID AdministrationId, id Id) {
	admin, rid, ok := s.taskCallID(w, r, administrationID, id)
	if !ok {
		return
	}
	in, ok := readTemplateInput(w, r)
	if !ok {
		return
	}
	t, err := s.deps.TaskLists.UpdateTemplate(r.Context(), admin, rid, user(r).ID, in)
	answer(s, w, r, http.StatusOK, t, err, toTaskTemplate)
}

// DeleteTaskListTemplate implements ServerInterface.
func (s *server) DeleteTaskListTemplate(w http.ResponseWriter, r *http.Request, administrationID AdministrationId, id Id) {
	admin, rid, ok := s.taskCallID(w, r, administrationID, id)
	if !ok {
		return
	}
	if err := s.deps.TaskLists.DeleteTemplate(r.Context(), admin, rid, user(r).ID); err != nil {
		s.failTimeTasks(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// CreateTaskListFromTemplate implements ServerInterface.
func (s *server) CreateTaskListFromTemplate(w http.ResponseWriter, r *http.Request, administrationID AdministrationId, id Id) {
	admin, rid, ok := s.taskCallID(w, r, administrationID, id)
	if !ok {
		return
	}
	in, ok := readTaskListInput(w, r, true)
	if !ok {
		return
	}
	l, err := s.deps.TaskLists.FromTemplate(r.Context(), admin, rid, user(r).ID, in)
	answer(s, w, r, http.StatusCreated, l, err, toTaskList)
}
