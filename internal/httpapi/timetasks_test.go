package httpapi_test

import (
	"context"
	"encoding/json"
	"net/http"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/adirkuhn/moneyharpy/internal/postgres/postgrestest"
)

type timeEntry struct {
	ID             string     `json:"id"`
	UserID         string     `json:"user_id"`
	ContactID      *string    `json:"contact_id"`
	ProjectID      *string    `json:"project_id"`
	SalesInvoiceID *string    `json:"sales_invoice_id"`
	StartedAt      time.Time  `json:"started_at"`
	EndedAt        *time.Time `json:"ended_at"`
	Description    string     `json:"description"`
	PausedDuration int        `json:"paused_duration"`
	Billable       bool       `json:"billable"`
	Contact        *struct {
		CompanyName string `json:"company_name"`
	} `json:"contact"`
	User struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	} `json:"user"`
	Project *struct {
		Name string `json:"name"`
	} `json:"project"`
	SalesInvoice *struct {
		ID string `json:"id"`
	} `json:"sales_invoice"`
	Notes []struct {
		ID   string `json:"id"`
		Note string `json:"note"`
	} `json:"notes"`
	URI string `json:"uri"`
}

func TestTimeEntriesLifecycle(t *testing.T) {
	h := newHarness(t, options{})
	owner, clerk, stranger := h.client(), h.client(), h.client()
	me := owner.register()
	clerkUser := clerk.register()
	stranger.register()
	a := owner.createAdministration("Harpy")
	api := "/api/v2/" + a.Id
	invite(t, h, owner, clerk, a.Id, "clerk@example.com", map[string]any{"user_type": "employee", "permissions": []string{"time_entries"}})
	customer := decode[contact](t, owner.do(http.MethodPost, api+"/contacts", map[string]any{"contact": map[string]any{"company_name": "Example Shop B.V."}}))
	falcon := decode[project](t, owner.do(http.MethodPost, api+"/projects", map[string]any{"project": map[string]any{"name": "Falcon"}}))
	today := time.Now().UTC().Truncate(time.Minute)
	yesterday := today.AddDate(0, 0, -1)

	r := owner.do(http.MethodPost, api+"/time_entries.json", map[string]any{"time_entry": map[string]any{
		"user_id": me.Id, "started_at": yesterday.Add(-2 * time.Hour).Add(42 * time.Second), "ended_at": yesterday, "description": "Designing the website",
		"contact_id": customer.ID, "project_id": falcon.ID, "billable": "true", "paused_duration": "600",
	}})
	wantStatus(t, r, http.StatusCreated)
	work := decode[timeEntry](t, r)
	if work.StartedAt.Second() != 0 || work.PausedDuration != 600 || deref(work.ContactID) != customer.ID || work.Contact == nil || work.Contact.CompanyName != "Example Shop B.V." ||
		work.Project == nil || work.Project.Name != "Falcon" || work.User.ID != me.Id || work.User.Name == "" || work.SalesInvoice != nil ||
		work.URI != "/"+a.Id+"/time_entries/"+work.ID || len(work.Notes) != 0 {
		t.Fatalf("created = %+v", work)
	}
	invalid := owner.do(http.MethodPost, api+"/time_entries", map[string]any{"time_entry": map[string]any{"user_id": me.Id, "started_at": today, "description": "x", "sales_invoice_id": "1"}})
	wantStatus(t, invalid, http.StatusUnprocessableEntity)
	if !strings.Contains(string(invalid.body), "sales_invoice_id") {
		t.Errorf("invalid = %s", invalid.body)
	}
	wantStatus(t, owner.do(http.MethodPost, api+"/time_entries", map[string]any{"time_entry": map[string]any{"user_id": me.Id, "started_at": today}}), http.StatusUnprocessableEntity)
	wantStatus(t, owner.do(http.MethodPost, api+"/time_entries", map[string]any{"time_entry": map[string]any{"colour": "red"}}), http.StatusBadRequest)
	wantStatus(t, stranger.do(http.MethodGet, api+"/time_entries/"+work.ID, nil), http.StatusNotFound)

	// A timer: the clerk starts one, stops it and resumes it.
	r = clerk.do(http.MethodPost, api+"/time_entries", map[string]any{"time_entry": map[string]any{"user_id": clerkUser.Id, "started_at": today.Add(-time.Hour), "description": "Timer"}})
	wantStatus(t, r, http.StatusCreated)
	timer := decode[timeEntry](t, r)
	if timer.EndedAt != nil {
		t.Errorf("timer = %+v", timer)
	}
	stopped := decode[timeEntry](t, clerk.do(http.MethodPatch, api+"/time_entries/"+timer.ID+"/stop.json", nil))
	if stopped.EndedAt == nil {
		t.Errorf("stopped = %+v", stopped)
	}
	resumed := clerk.do(http.MethodPatch, api+"/time_entries/"+timer.ID+"/resume", nil)
	wantStatus(t, resumed, http.StatusOK)
	if got := decode[timeEntry](t, resumed); got.EndedAt != nil {
		t.Errorf("resumed = %+v", got)
	}
	wantStatus(t, owner.do(http.MethodPatch, api+"/time_entries/"+work.ID+"/resume", nil), http.StatusUnprocessableEntity)

	// The clerk sees and changes only their own time.
	wantStatus(t, clerk.do(http.MethodGet, api+"/time_entries/"+work.ID, nil), http.StatusForbidden)
	wantStatus(t, clerk.do(http.MethodPost, api+"/time_entries", map[string]any{"time_entry": map[string]any{"user_id": me.Id, "started_at": today, "description": "x"}}), http.StatusForbidden)
	if got := decode[[]timeEntry](t, clerk.do(http.MethodGet, api+"/time_entries?filter=include_active:true", nil)); len(got) != 1 || got[0].ID != timer.ID {
		t.Errorf("clerk's list = %+v", got)
	}
	if got := decode[timeEntry](t, owner.do(http.MethodGet, api+"/time_entries/"+timer.ID, nil)); got.ID != timer.ID {
		t.Errorf("owner reads the clerk's = %+v", got)
	}

	list := owner.do(http.MethodGet, api+"/time_entries", nil)
	if got := decode[[]timeEntry](t, list); len(got) != 1 || got[0].ID != work.ID || list.header.Get("X-Total-Count") != "1" {
		t.Errorf("default list = %+v", got)
	}
	if got := decode[[]timeEntry](t, owner.do(http.MethodGet, api+"/time_entries?filter=state:open,include_active:true&query=website&per_page=10", nil)); len(got) != 1 {
		t.Errorf("open = %+v", got)
	}
	if got := decode[[]timeEntry](t, owner.do(http.MethodGet, api+"/time_entries?filter=project_id:"+falcon.ID+",contact_id:"+customer.ID+",user_id:"+me.Id+",day:"+yesterday.Format(time.DateOnly), nil)); len(got) != 1 {
		t.Errorf("by project = %+v", got)
	}
	wantStatus(t, owner.do(http.MethodGet, api+"/time_entries?filter=state:late", nil), http.StatusBadRequest)

	up := owner.do(http.MethodPatch, api+"/time_entries/"+work.ID, map[string]any{"time_entry": map[string]any{"billable": false, "contact_id": nil, "description": "Designing"}})
	wantStatus(t, up, http.StatusOK)
	if got := decode[timeEntry](t, up); got.Billable || got.ContactID != nil || got.Contact != nil || got.Description != "Designing" || deref(got.ProjectID) != falcon.ID {
		t.Errorf("updated = %+v", got)
	}
	wantStatus(t, owner.do(http.MethodPatch, api+"/time_entries/"+work.ID, map[string]any{"time_entry": map[string]any{"billable": true}}), http.StatusOK)

	r = owner.do(http.MethodPost, api+"/time_entries/"+work.ID+"/notes", map[string]any{"note": map[string]any{"note": "Check the hours", "todo": true}})
	wantStatus(t, r, http.StatusCreated)
	note := decode[struct {
		ID         string `json:"id"`
		EntityType string `json:"entity_type"`
	}](t, r)
	if note.EntityType != "TimeEntry" {
		t.Errorf("note = %+v", note)
	}
	if got := decode[timeEntry](t, owner.do(http.MethodGet, api+"/time_entries/"+work.ID, nil)); len(got.Notes) != 1 {
		t.Errorf("notes = %+v", got.Notes)
	}
	wantStatus(t, owner.do(http.MethodDelete, api+"/time_entries/"+work.ID+"/notes/"+note.ID, nil), http.StatusNoContent)

	// Billing: the time goes on an invoice and stays there.
	wantStatus(t, owner.do(http.MethodPost, api+"/sales_invoices", map[string]any{"sales_invoice": map[string]any{"contact_id": customer.ID, "time_entry_ids": []string{timer.ID}}}), http.StatusUnprocessableEntity)
	r = owner.do(http.MethodPost, api+"/sales_invoices", map[string]any{"sales_invoice": map[string]any{"contact_id": customer.ID, "time_entry_ids": []any{work.ID}}})
	wantStatus(t, r, http.StatusCreated)
	inv := decode[struct {
		ID          string `json:"id"`
		TimeEntries []struct {
			ID string `json:"id"`
		} `json:"time_entries"`
	}](t, r)
	if len(inv.TimeEntries) != 1 || inv.TimeEntries[0].ID != work.ID {
		t.Errorf("invoice time = %+v", inv)
	}
	billed := decode[timeEntry](t, owner.do(http.MethodGet, api+"/time_entries/"+work.ID, nil))
	if deref(billed.SalesInvoiceID) != inv.ID || billed.SalesInvoice == nil || billed.SalesInvoice.ID != inv.ID {
		t.Errorf("billed = %+v", billed)
	}
	if got := decode[[]timeEntry](t, owner.do(http.MethodGet, api+"/time_entries?filter=state:open", nil)); len(got) != 0 {
		t.Errorf("open after billing = %+v", got)
	}
	wantStatus(t, owner.do(http.MethodPatch, api+"/time_entries/"+work.ID, map[string]any{"time_entry": map[string]any{"description": "x"}}), http.StatusUnprocessableEntity)
	wantStatus(t, owner.do(http.MethodDelete, api+"/time_entries/"+work.ID, nil), http.StatusUnprocessableEntity)

	// Projects and contacts with time are kept.
	r = owner.do(http.MethodDelete, api+"/projects/"+falcon.ID, nil)
	wantStatus(t, r, http.StatusNoContent)
	if r.header.Get("X-Kept-As") != "archived" {
		t.Errorf("project with time deleted")
	}

	wantStatus(t, clerk.do(http.MethodDelete, api+"/time_entries/"+timer.ID+".json", nil), http.StatusNoContent)
	wantStatus(t, clerk.do(http.MethodGet, api+"/time_entries/"+timer.ID, nil), http.StatusNotFound)
}

type taskJSON struct {
	ID              string     `json:"id"`
	Name            string     `json:"name"`
	Description     *string    `json:"description"`
	RowOrder        int        `json:"row_order"`
	CompletedAt     *time.Time `json:"completed_at"`
	ReportType      *string    `json:"report_type"`
	GroupID         string     `json:"task_lists_group_id"`
	ParentTaskID    *string    `json:"parent_task_id"`
	AssigneeID      *string    `json:"assignee_id"`
	RelatedEntityID *string    `json:"related_entity_id"`
	Generated       bool       `json:"generated"`
	Subtasks        []taskJSON `json:"subtasks"`
	Notes           []struct {
		Note string `json:"note"`
	} `json:"notes"`
}

type taskGroup struct {
	ID         string     `json:"id"`
	Name       *string    `json:"name"`
	RowOrder   int        `json:"row_order"`
	ReportType *string    `json:"report_type"`
	Tasks      []taskJSON `json:"tasks"`
}

type taskList struct {
	ID          string      `json:"id"`
	Name        string      `json:"name"`
	Description *string     `json:"description"`
	DeadlineOn  *string     `json:"deadline_on"`
	PeriodBegin *string     `json:"period_begin"`
	PeriodEnd   *string     `json:"period_end"`
	Completed   bool        `json:"completed"`
	TemplateID  *string     `json:"template_id"`
	Total       int         `json:"total_tasks_count"`
	Done        int         `json:"completed_tasks_count"`
	Groups      []taskGroup `json:"groups"`
	URI         string      `json:"uri"`
}

type taskTemplate struct {
	ID            string      `json:"id"`
	Name          string      `json:"name"`
	Period        *string     `json:"period"`
	DeadlineAfter *string     `json:"deadline_after"`
	Groups        []taskGroup `json:"groups"`
	URI           string      `json:"uri"`
}

func TestTaskListsLifecycle(t *testing.T) {
	h := newHarness(t, options{})
	owner, clerk := h.client(), h.client()
	me := owner.register()
	clerk.register()
	a := owner.createAdministration("Harpy")
	api := "/api/v2/" + a.Id
	invite(t, h, owner, clerk, a.Id, "clerk@example.com", map[string]any{"user_type": "employee", "permissions": []string{"time_entries", "bank"}})
	wantStatus(t, clerk.do(http.MethodGet, api+"/task_lists", nil), http.StatusForbidden)

	r := owner.do(http.MethodPost, api+"/task_list_templates", map[string]any{"task_list_template": map[string]any{"name": "Month end", "description": "Closing", "period": "monthly", "deadline_after": "P10D"}})
	wantStatus(t, r, http.StatusCreated)
	tpl := decode[taskTemplate](t, r)
	if deref(tpl.Period) != "P1M" || deref(tpl.DeadlineAfter) != "P10D" || len(tpl.Groups) != 1 || tpl.Groups[0].Name != nil || tpl.URI != "/"+a.Id+"/task_list_templates/"+tpl.ID {
		t.Fatalf("template = %+v", tpl)
	}
	wantStatus(t, owner.do(http.MethodPost, api+"/task_list_templates", map[string]any{"task_list_template": map[string]any{"name": ""}}), http.StatusUnprocessableEntity)
	wantStatus(t, owner.do(http.MethodPatch, api+"/task_list_templates/"+tpl.ID, map[string]any{"task_list_template": map[string]any{"period": "P1X"}}), http.StatusUnprocessableEntity)
	r = owner.do(http.MethodPost, api+"/task_list_groups/"+tpl.Groups[0].ID+"/tasks", map[string]any{"task": map[string]any{"name": "Reconcile the bank", "description": "Every account"}})
	wantStatus(t, r, http.StatusCreated)
	r = owner.do(http.MethodPost, api+"/task_list_templates/"+tpl.ID+"/groups", map[string]any{"group": map[string]any{"name": "Balance", "report_type": "balance_sheet"}})
	wantStatus(t, r, http.StatusCreated)
	if g := decode[taskGroup](t, r); len(g.Tasks) == 0 || !g.Tasks[0].Generated || deref(g.Tasks[0].ReportType) != "balance_sheet" || g.RowOrder != 1 {
		t.Fatalf("generated group = %+v", g)
	}
	if got := decode[[]taskTemplate](t, owner.do(http.MethodGet, api+"/task_list_templates", nil)); len(got) != 1 || len(got[0].Groups) != 2 {
		t.Errorf("templates = %+v", got)
	}
	up := owner.do(http.MethodPatch, api+"/task_list_templates/"+tpl.ID+".json", map[string]any{"task_list_template": map[string]any{"name": "Month closing"}})
	wantStatus(t, up, http.StatusOK)

	r = owner.do(http.MethodPost, api+"/task_list_templates/"+tpl.ID+"/task_lists", nil)
	wantStatus(t, r, http.StatusCreated)
	l := decode[taskList](t, r)
	if l.Name != "Month closing" || deref(l.Description) != "Closing" || deref(l.TemplateID) != tpl.ID || l.PeriodBegin == nil || l.DeadlineOn == nil || len(l.Groups) != 2 ||
		l.Completed || l.Total < 2 || l.URI != "/"+a.Id+"/task_lists/"+l.ID {
		t.Fatalf("from template = %+v", l)
	}
	wantStatus(t, owner.do(http.MethodPost, api+"/task_list_templates/1/task_lists", map[string]any{"task_list": map[string]any{}}), http.StatusNotFound)

	r = owner.do(http.MethodPost, api+"/task_lists", map[string]any{"task_list": map[string]any{"name": "Year end", "period_begin": "2025-01-01", "period_end": "2025-12-31"}})
	wantStatus(t, r, http.StatusCreated)
	year := decode[taskList](t, r)
	if deref(year.PeriodEnd) != "2025-12-31" || !year.Completed || len(year.Groups) != 1 {
		t.Errorf("plain list = %+v", year)
	}
	wantStatus(t, owner.do(http.MethodPost, api+"/task_lists", map[string]any{"task_list": map[string]any{}}), http.StatusUnprocessableEntity)
	page := owner.do(http.MethodGet, api+"/task_lists?per_page=1", nil)
	if got := decode[[]taskList](t, page); len(got) != 1 || page.header.Get("X-Total-Count") != "2" {
		t.Errorf("page = %+v", got)
	}
	up = owner.do(http.MethodPatch, api+"/task_lists/"+year.ID, map[string]any{"task_list": map[string]any{"description": "All of it", "deadline_on": "2026-03-31"}})
	wantStatus(t, up, http.StatusOK)
	if got := decode[taskList](t, up); deref(got.DeadlineOn) != "2026-03-31" || deref(got.Description) != "All of it" {
		t.Errorf("updated list = %+v", got)
	}
	if got := decode[taskList](t, owner.do(http.MethodPatch, api+"/task_lists/"+year.ID, map[string]any{"task_list": map[string]any{"deadline_on": nil}})); got.DeadlineOn != nil {
		t.Errorf("deadline kept = %+v", got)
	}

	// Groups.
	first := year.Groups[0].ID
	wantStatus(t, owner.do(http.MethodPatch, api+"/task_list_groups/"+first, map[string]any{"group": map[string]any{"name": "x"}}), http.StatusUnprocessableEntity)
	wantStatus(t, owner.do(http.MethodDelete, api+"/task_list_groups/"+first, nil), http.StatusUnprocessableEntity)
	r = owner.do(http.MethodPost, api+"/task_lists/"+year.ID+"/groups", map[string]any{"group": map[string]any{"name": "Results"}})
	wantStatus(t, r, http.StatusCreated)
	results := decode[taskGroup](t, r)
	r = owner.do(http.MethodPatch, api+"/task_list_groups/"+results.ID, map[string]any{"group": map[string]any{"report_type": "profit_loss"}})
	wantStatus(t, r, http.StatusOK)
	if g := decode[taskGroup](t, r); len(g.Tasks) == 0 || !g.Tasks[0].Generated {
		t.Errorf("profit and loss group = %+v", g)
	}
	generated := decode[taskGroup](t, owner.do(http.MethodGet, api+"/task_list_groups/"+results.ID, nil)).Tasks[0]
	wantStatus(t, owner.do(http.MethodDelete, api+"/task_list_tasks/"+generated.ID, nil), http.StatusUnprocessableEntity)
	wantStatus(t, owner.do(http.MethodPost, api+"/task_list_groups/"+results.ID+"/tasks", map[string]any{"task": map[string]any{"name": "Extra"}}), http.StatusUnprocessableEntity)
	wantStatus(t, owner.do(http.MethodGet, api+"/task_list_groups/1", nil), http.StatusNotFound)

	// Tasks.
	bank := category(t, owner, a.Id, "Revenue")
	r = owner.do(http.MethodPost, api+"/task_list_groups/"+first+"/tasks", map[string]any{"task": map[string]any{"name": "Bank", "related_entity_id": bank, "generate_subtasks": true, "assignee_id": me.Id}})
	wantStatus(t, r, http.StatusCreated)
	task := decode[taskJSON](t, r)
	if task.GroupID != first || deref(task.RelatedEntityID) != bank || deref(task.AssigneeID) != me.Id {
		t.Errorf("task = %+v", task)
	}
	wantStatus(t, owner.do(http.MethodPost, api+"/task_list_groups/"+first+"/tasks", map[string]any{"task": map[string]any{}}), http.StatusUnprocessableEntity)
	r = owner.do(http.MethodPatch, api+"/task_list_tasks/"+task.ID, map[string]any{"task": map[string]any{"name": "Bank accounts", "description": "Match", "related_entity_id": nil}})
	wantStatus(t, r, http.StatusOK)
	if got := decode[taskJSON](t, r); got.Name != "Bank accounts" || got.RelatedEntityID != nil {
		t.Errorf("updated task = %+v", got)
	}
	wantStatus(t, owner.do(http.MethodPost, api+"/task_list_tasks/"+task.ID+"/assignment", map[string]any{"assignee_id": "1"}), http.StatusNotFound)
	if got := decode[taskJSON](t, owner.do(http.MethodDelete, api+"/task_list_tasks/"+task.ID+"/assignment", nil)); got.AssigneeID != nil {
		t.Errorf("unassigned = %+v", got)
	}
	if got := decode[taskJSON](t, owner.do(http.MethodPost, api+"/task_list_tasks/"+task.ID+"/assignment", map[string]any{"assignee_id": me.Id})); deref(got.AssigneeID) != me.Id {
		t.Errorf("assigned = %+v", got)
	}
	if got := decode[taskJSON](t, owner.do(http.MethodPost, api+"/task_list_tasks/"+task.ID+"/completion", nil)); got.CompletedAt == nil {
		t.Errorf("completed = %+v", got)
	}
	if got := decode[taskJSON](t, owner.do(http.MethodDelete, api+"/task_list_tasks/"+task.ID+"/completion", nil)); got.CompletedAt != nil {
		t.Errorf("reopened = %+v", got)
	}
	r = owner.do(http.MethodPost, api+"/task_list_tasks/"+task.ID+"/notes", map[string]any{"note": map[string]any{"note": "Ask the bank", "todo": false}})
	wantStatus(t, r, http.StatusCreated)
	if got := decode[taskJSON](t, owner.do(http.MethodGet, api+"/task_list_tasks/"+task.ID, nil)); len(got.Notes) != 1 {
		t.Errorf("task notes = %+v", got.Notes)
	}
	if got := decode[taskList](t, owner.do(http.MethodGet, api+"/task_lists/"+year.ID, nil)); len(got.Groups) != 2 || got.Groups[0].Tasks[0].ID != task.ID {
		t.Errorf("list = %+v", got)
	}
	wantStatus(t, owner.do(http.MethodDelete, api+"/task_list_tasks/"+task.ID, nil), http.StatusNoContent)
	wantStatus(t, owner.do(http.MethodGet, api+"/task_list_tasks/"+task.ID, nil), http.StatusNotFound)
	wantStatus(t, owner.do(http.MethodDelete, api+"/task_list_groups/"+results.ID, nil), http.StatusNoContent)
	wantStatus(t, owner.do(http.MethodDelete, api+"/task_list_templates/"+tpl.ID, nil), http.StatusNoContent)
	if got := decode[taskList](t, owner.do(http.MethodGet, api+"/task_lists/"+l.ID, nil)); got.TemplateID != nil {
		t.Errorf("list of a deleted template = %+v", got)
	}
	wantStatus(t, owner.do(http.MethodDelete, api+"/task_lists/"+l.ID, nil), http.StatusNoContent)
	wantStatus(t, owner.do(http.MethodGet, api+"/task_lists/"+l.ID, nil), http.StatusNotFound)
}

func TestTimeAndTaskWebhooks(t *testing.T) {
	ctx := context.Background()
	pool, cleanup, err := postgrestest.Start(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()
	h := newHarness(t, options{pool: pool})
	owner := h.client()
	me := owner.register()
	a := owner.createAdministration("Harpy")
	api := "/api/v2/" + a.Id
	rc := newReceiver(t)
	wantStatus(t, owner.do(http.MethodPatch, "/api/instance/settings", map[string]bool{"webhooks_allow_local": true}), http.StatusOK)
	wantStatus(t, owner.do(http.MethodPost, api+"/webhooks", map[string]any{"url": rc.srv.URL, "enabled_events": []string{"time_entry", "task_lists"}}), http.StatusCreated)

	customer := decode[contact](t, owner.do(http.MethodPost, api+"/contacts", map[string]any{"contact": map[string]any{"company_name": "Example Shop B.V."}}))
	start := time.Now().UTC().Add(-2 * time.Hour)
	entry := decode[timeEntry](t, owner.do(http.MethodPost, api+"/time_entries", map[string]any{"time_entry": map[string]any{"user_id": me.Id, "started_at": start, "ended_at": start.Add(time.Hour), "description": "Work"}}))
	wantStatus(t, owner.do(http.MethodPatch, api+"/time_entries/"+entry.ID, map[string]any{"time_entry": map[string]any{"description": "More work"}}), http.StatusOK)
	wantStatus(t, owner.do(http.MethodPost, api+"/sales_invoices", map[string]any{"sales_invoice": map[string]any{"contact_id": customer.ID, "time_entry_ids": []string{entry.ID}}}), http.StatusCreated)
	other := decode[timeEntry](t, owner.do(http.MethodPost, api+"/time_entries", map[string]any{"time_entry": map[string]any{"user_id": me.Id, "started_at": start, "ended_at": start.Add(time.Hour), "description": "Other"}}))
	wantStatus(t, owner.do(http.MethodDelete, api+"/time_entries/"+other.ID, nil), http.StatusNoContent)

	l := decode[taskList](t, owner.do(http.MethodPost, api+"/task_lists", map[string]any{"task_list": map[string]any{"name": "Close"}}))
	task := decode[taskJSON](t, owner.do(http.MethodPost, api+"/task_list_groups/"+l.Groups[0].ID+"/tasks", map[string]any{"task": map[string]any{"name": "Check"}}))
	wantStatus(t, owner.do(http.MethodPatch, api+"/task_list_tasks/"+task.ID, map[string]any{"task": map[string]any{"name": "Check all"}}), http.StatusOK)
	wantStatus(t, owner.do(http.MethodPost, api+"/task_list_tasks/"+task.ID+"/assignment", map[string]any{"assignee_id": me.Id}), http.StatusOK)
	wantStatus(t, owner.do(http.MethodPost, api+"/task_list_tasks/"+task.ID+"/completion", nil), http.StatusOK)
	wantStatus(t, owner.do(http.MethodDelete, api+"/task_list_tasks/"+task.ID+"/completion", nil), http.StatusOK)
	tpl := decode[taskTemplate](t, owner.do(http.MethodPost, api+"/task_list_templates", map[string]any{"task_list_template": map[string]any{"name": "Close"}}))
	wantStatus(t, owner.do(http.MethodDelete, api+"/task_list_templates/"+tpl.ID, nil), http.StatusNoContent)
	wantStatus(t, owner.do(http.MethodDelete, api+"/task_lists/"+l.ID, nil), http.StatusNoContent)

	if _, err := h.hooks.Deliver(ctx, 50); err != nil {
		t.Fatal(err)
	}
	rc.mu.Lock()
	defer rc.mu.Unlock()
	var actions []string
	for _, b := range rc.body[1:] {
		var p map[string]any
		_ = json.Unmarshal(b, &p)
		actions = append(actions, p["action"].(string))
	}
	slices.Sort(actions)
	want := []string{
		"task_lists_list_completed", "task_lists_list_created", "task_lists_list_destroyed", "task_lists_list_reopened",
		"task_lists_list_template_created", "task_lists_list_template_destroyed",
		"task_lists_task_assigned", "task_lists_task_completed", "task_lists_task_created", "task_lists_task_name", "task_lists_task_reopened", "task_lists_task_updated",
		"time_entry_created", "time_entry_created", "time_entry_destroyed", "time_entry_sales_invoice_created", "time_entry_updated",
	}
	if !slices.Equal(actions, want) {
		t.Errorf("actions = %v, want %v", actions, want)
	}
}
