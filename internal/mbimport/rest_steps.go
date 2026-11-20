package mbimport

import (
	"context"
	"encoding/json"
	"net/url"
	"strings"
	"time"

	"github.com/adirkuhn/moneyharpy/internal/administration"
	"github.com/adirkuhn/moneyharpy/internal/invoicing"
	"github.com/adirkuhn/moneyharpy/internal/membership"
	"github.com/adirkuhn/moneyharpy/internal/notes"
	"github.com/adirkuhn/moneyharpy/internal/resource"
	"github.com/adirkuhn/moneyharpy/internal/tasklists"
	"github.com/adirkuhn/moneyharpy/internal/timeentries"
)

// The kinds the store remembers imported ids by, for the rest.
const (
	kindCustomField  = "custom_field"
	kindIdentity     = "identity"
	kindWorkflow     = "workflow"
	kindTimeEntry    = "time_entry"
	kindTaskTemplate = "task_list_template"
	kindTaskList     = "task_list"
)

type mbCustomField struct {
	ID     mbID   `json:"id"`
	Name   string `json:"name"`
	Source string `json:"source"`
}

// mbFieldValue is a record's value of a custom field.
type mbFieldValue struct {
	ID    mbID   `json:"id"`
	Value string `json:"value"`
}

type mbIdentity struct {
	ID                mbID           `json:"id"`
	CompanyName       *string        `json:"company_name"`
	Address1          *string        `json:"address1"`
	Address2          *string        `json:"address2"`
	Zipcode           *string        `json:"zipcode"`
	City              *string        `json:"city"`
	Country           *string        `json:"country"`
	Email             *string        `json:"email"`
	Phone             *string        `json:"phone"`
	ChamberOfCommerce *string        `json:"chamber_of_commerce"`
	TaxNumber         *string        `json:"tax_number"`
	BankAccountName   *string        `json:"bank_account_name"`
	BankAccountNumber *string        `json:"bank_account_number"`
	BankAccountBIC    *string        `json:"bank_account_bic"`
	CustomFields      []mbFieldValue `json:"custom_fields"`
	stamps
}

func (i mbIdentity) changes() administration.IdentityChanges {
	c := administration.IdentityChanges{
		CompanyName: opt(i.CompanyName), Address1: opt(i.Address1), Address2: opt(i.Address2), Zipcode: opt(i.Zipcode), City: opt(i.City),
		Country: opt(i.Country), Email: opt(i.Email), Phone: opt(i.Phone), ChamberOfCommerce: opt(i.ChamberOfCommerce), TaxNumber: opt(i.TaxNumber),
		BankAccountName: opt(i.BankAccountName), BankAccountNumber: opt(i.BankAccountNumber), BankAccountBIC: opt(i.BankAccountBIC),
	}
	for _, f := range i.CustomFields {
		c.CustomFields = append(c.CustomFields, administration.CustomFieldChange{ID: f.ID.id(), Value: f.Value})
	}
	return c
}

type mbWorkflowStep struct {
	DueInterval      int    `json:"due_interval"`
	ReminderDelay    int    `json:"reminder_delay"`
	ReminderText     string `json:"reminder_text"`
	ReminderAutoSend bool   `json:"reminder_auto_send"`
}

type mbWorkflow struct {
	ID               mbID             `json:"id"`
	Type             string           `json:"type"`
	Name             *string          `json:"name"`
	Default          *bool            `json:"default"`
	Currency         *string          `json:"currency"`
	Language         *string          `json:"language"`
	Active           *bool            `json:"active"`
	PricesAreInclTax *bool            `json:"prices_are_incl_tax"`
	ShowQRCode       *bool            `json:"show_qr_code"`
	Steps            []mbWorkflowStep `json:"steps"`
	stamps
}

type mbNote struct {
	ID          mbID       `json:"id"`
	Note        string     `json:"note"`
	Todo        bool       `json:"todo"`
	CompletedAt *time.Time `json:"completed_at"`
	stamps
}

type mbUser struct {
	ID    mbID   `json:"id"`
	Email string `json:"email"`
}

type mbTimeEntry struct {
	ID             mbID       `json:"id"`
	ContactID      *mbID      `json:"contact_id"`
	ProjectID      *mbID      `json:"project_id"`
	SalesInvoiceID *mbID      `json:"sales_invoice_id"`
	UserID         mbID       `json:"user_id"`
	StartedAt      *time.Time `json:"started_at"`
	EndedAt        *time.Time `json:"ended_at"`
	Description    *string    `json:"description"`
	PausedDuration *int       `json:"paused_duration"`
	Billable       *bool      `json:"billable"`
	Notes          []mbNote   `json:"notes"`
	stamps
}

type mbTask struct {
	ID          mbID       `json:"id"`
	Name        string     `json:"name"`
	Description *string    `json:"description"`
	RowOrder    int        `json:"row_order"`
	CompletedAt *time.Time `json:"completed_at"`
	ReportType  *string    `json:"report_type"`
	Subtasks    []mbTask   `json:"subtasks"`
	stamps
}

type mbTaskGroup struct {
	ID         mbID     `json:"id"`
	Name       *string  `json:"name"`
	RowOrder   int      `json:"row_order"`
	ReportType *string  `json:"report_type"`
	Tasks      []mbTask `json:"tasks"`
	stamps
}

type mbTaskList struct {
	ID          mbID          `json:"id"`
	Name        *string       `json:"name"`
	Description *string       `json:"description"`
	DeadlineOn  *string       `json:"deadline_on"`
	PeriodBegin *string       `json:"period_begin"`
	PeriodEnd   *string       `json:"period_end"`
	Period      *string       `json:"period"`
	Deadline    *string       `json:"deadline_after"`
	Groups      []mbTaskGroup `json:"groups"`
	stamps
}

func groupsOf(in []mbTaskGroup) []tasklists.ImportedGroup {
	out := make([]tasklists.ImportedGroup, 0, len(in))
	for _, g := range in {
		out = append(out, tasklists.ImportedGroup{
			Origin: origin(g.ID, g.stamps), Name: deref(g.Name), RowOrder: g.RowOrder, ReportType: deref(g.ReportType), Tasks: tasksOf(g.Tasks),
		})
	}
	return out
}

func tasksOf(in []mbTask) []tasklists.ImportedTask {
	out := make([]tasklists.ImportedTask, 0, len(in))
	for _, t := range in {
		out = append(out, tasklists.ImportedTask{
			Origin: origin(t.ID, t.stamps), Name: t.Name, Description: deref(t.Description), RowOrder: t.RowOrder,
			CompletedAt: t.CompletedAt, ReportType: deref(t.ReportType), Subtasks: tasksOf(t.Subtasks),
		})
	}
	return out
}

func (r *run) customFields(ctx context.Context) error {
	st := r.job.Step(StepCustomFields)
	raws, err := r.read.List(ctx, r.job.SourceID, "custom_fields", nil)
	if err != nil {
		return err
	}
	st.Total = len(raws)
	return r.take(ctx, st, kindCustomField, raws, func(ctx context.Context, raw json.RawMessage) (resource.ID, error) {
		var f mbCustomField
		if err := json.Unmarshal(raw, &f); err != nil {
			return 0, err
		}
		_, err := r.s.targets.Admin.ImportCustomField(ctx, r.job.AdministrationID, r.job.UserID, resource.Origin{ID: f.ID.id()},
			administration.NewCustomField{Name: f.Name, Source: administration.Source(f.Source)})
		return f.ID.id(), err
	})
}

// identities brings in the sender addresses. Moneybird's default one fills
// in the administration's own, which keeps its id.
func (r *run) identities(ctx context.Context) error {
	st := r.job.Step(StepIdentities)
	def, err := r.read.Get(ctx, r.job.SourceID, "identities/default", nil)
	if err != nil {
		return err
	}
	var d mbIdentity
	if err := json.Unmarshal(def, &d); err == nil && d.ID != 0 {
		st.Total++
		if _, done, err := r.s.store.Imported(ctx, r.job.AdministrationID, kindIdentity, d.ID.id()); err != nil {
			return err
		} else if !done {
			own, err := r.s.targets.Admin.UpdateDefaultIdentity(ctx, r.job.AdministrationID, r.job.UserID, d.changes())
			if err != nil {
				if !isRecordProblem(err) {
					return err
				}
				st.problem("identity %d: %v", d.ID, err)
			} else {
				if err := r.s.store.RecordImported(ctx, r.job.AdministrationID, kindIdentity, d.ID.id(), own.ID); err != nil {
					return err
				}
				st.Imported++
			}
		} else {
			st.Skipped++
		}
	}
	raws, err := r.read.List(ctx, r.job.SourceID, "identities", nil)
	if err != nil {
		return err
	}
	st.Total += len(raws)
	return r.take(ctx, st, kindIdentity, raws, func(ctx context.Context, raw json.RawMessage) (resource.ID, error) {
		var i mbIdentity
		if err := json.Unmarshal(raw, &i); err != nil {
			return 0, err
		}
		_, err := r.s.targets.Admin.ImportIdentity(ctx, r.job.AdministrationID, r.job.UserID, origin(i.ID, i.stamps), i.changes())
		return i.ID.id(), err
	})
}

func (r *run) workflows(ctx context.Context) error {
	st := r.job.Step(StepWorkflows)
	raws, err := r.read.List(ctx, r.job.SourceID, "workflows", nil)
	if err != nil {
		return err
	}
	st.Total = len(raws)
	return r.take(ctx, st, kindWorkflow, raws, func(ctx context.Context, raw json.RawMessage) (resource.ID, error) {
		var w mbWorkflow
		if err := json.Unmarshal(raw, &w); err != nil {
			return 0, err
		}
		t := invoicing.InvoiceWorkflow
		if strings.Contains(w.Type, "Estimate") {
			t = invoicing.EstimateWorkflow
		}
		c := invoicing.WorkflowChanges{
			Name: w.Name, Currency: opt(w.Currency), Language: opt(w.Language), Active: w.Active, Default: w.Default,
			PricesAreInclTax: w.PricesAreInclTax, ShowQRCode: w.ShowQRCode,
		}
		if len(w.Steps) > 0 {
			steps := make([]invoicing.StepInput, len(w.Steps))
			for i, s := range w.Steps {
				steps[i] = invoicing.StepInput{DueInterval: s.DueInterval, ReminderDelay: s.ReminderDelay, ReminderText: s.ReminderText, ReminderAutoSend: s.ReminderAutoSend}
			}
			c.Steps = &steps
		}
		_, err := r.s.targets.Invoicing.ImportWorkflow(ctx, r.job.AdministrationID, r.job.UserID, origin(w.ID, w.stamps), t, c)
		return w.ID.id(), err
	})
}

// members maps Moneybird's users to the members here by email address;
// time of anyone else is the importing owner's.
func (r *run) members(ctx context.Context) (map[resource.ID]resource.ID, error) {
	raws, err := r.read.List(ctx, r.job.SourceID, "users", nil)
	if err != nil {
		return nil, err
	}
	list, err := r.s.targets.Members.Users(ctx, r.job.AdministrationID, r.job.UserID, membership.UserFilter{})
	if err != nil {
		return nil, err
	}
	here := map[string]resource.ID{}
	for _, m := range list {
		here[strings.ToLower(strings.TrimSpace(m.Email))] = m.UserID
	}
	out := map[resource.ID]resource.ID{}
	for _, raw := range raws {
		var u mbUser
		if json.Unmarshal(raw, &u) != nil {
			continue
		}
		if id, ok := here[strings.ToLower(strings.TrimSpace(u.Email))]; ok {
			out[u.ID.id()] = id
		}
	}
	return out, nil
}

func (r *run) timeEntries(ctx context.Context) error {
	st := r.job.Step(StepTimeEntries)
	// Any filter lifts Moneybird's default of this year.
	raws, err := r.read.List(ctx, r.job.SourceID, "time_entries", urlValues("filter", "state:all,include_nil_contacts:true,include_active:true"))
	if err != nil || len(raws) == 0 {
		return err
	}
	st.Total = len(raws)
	users, err := r.members(ctx)
	if err != nil {
		return err
	}
	return r.take(ctx, st, kindTimeEntry, raws, func(ctx context.Context, raw json.RawMessage) (resource.ID, error) {
		var e mbTimeEntry
		if err := json.Unmarshal(raw, &e); err != nil {
			return 0, err
		}
		user, ok := users[e.UserID.id()]
		if !ok {
			user = r.job.UserID
		}
		in := timeentries.Input{
			UserID: &user, ContactID: ref(e.ContactID), ProjectID: ref(e.ProjectID), SalesInvoiceID: ref(e.SalesInvoiceID),
			StartedAt: e.StartedAt, EndedAt: e.EndedAt, Description: e.Description, PausedDuration: e.PausedDuration, Billable: e.Billable,
		}
		if in.Description == nil {
			empty := ""
			in.Description = &empty
		}
		got, err := r.s.targets.TimeEntries.Import(ctx, r.job.AdministrationID, timeentries.Actor{ID: r.job.UserID, AllUsers: true}, origin(e.ID, e.stamps), in)
		if err != nil {
			return 0, err
		}
		return e.ID.id(), r.notes(ctx, st, timeentries.EntityType, got.ID, e.Notes)
	})
}

func (r *run) taskTemplates(ctx context.Context) error {
	st := r.job.Step(StepTaskTemplates)
	raws, err := r.read.List(ctx, r.job.SourceID, "task_list_templates", nil)
	if err != nil {
		return err
	}
	st.Total = len(raws)
	return r.take(ctx, st, kindTaskTemplate, raws, func(ctx context.Context, raw json.RawMessage) (resource.ID, error) {
		var t mbTaskList
		if err := json.Unmarshal(raw, &t); err != nil {
			return 0, err
		}
		_, err := r.s.targets.TaskLists.ImportTemplate(ctx, r.job.AdministrationID, r.job.UserID, origin(t.ID, t.stamps),
			tasklists.TemplateInput{Name: t.Name, Description: t.Description, Period: opt(t.Period), DeadlineAfter: opt(t.Deadline)}, groupsOf(t.Groups))
		return t.ID.id(), err
	})
}

func (r *run) taskLists(ctx context.Context) error {
	st := r.job.Step(StepTaskLists)
	raws, err := r.read.List(ctx, r.job.SourceID, "task_lists", nil)
	if err != nil {
		return err
	}
	st.Total = len(raws)
	return r.take(ctx, st, kindTaskList, raws, func(ctx context.Context, raw json.RawMessage) (resource.ID, error) {
		var l mbTaskList
		if err := json.Unmarshal(raw, &l); err != nil {
			return 0, err
		}
		in := tasklists.ListInput{Name: l.Name, Description: l.Description, DeadlineOn: day(l.DeadlineOn), PeriodBegin: day(l.PeriodBegin), PeriodEnd: day(l.PeriodEnd)}
		in.NoPeriod = in.PeriodBegin == nil && in.PeriodEnd == nil
		_, err := r.s.targets.TaskLists.ImportList(ctx, r.job.AdministrationID, r.job.UserID, origin(l.ID, l.stamps), in, groupsOf(l.Groups))
		return l.ID.id(), err
	})
}

// notes brings in a record's notes and to-dos, written by the importing
// owner, as Moneybird's users are not members here.
func (r *run) notes(ctx context.Context, st *Step, entityType string, entity resource.ID, in []mbNote) error {
	for _, n := range in {
		_, err := r.s.targets.Notes.Import(ctx, r.job.AdministrationID, entityType, entity, r.job.UserID, notes.Imported{
			Origin: origin(n.ID, n.stamps), Input: notes.Input{Note: n.Note, Todo: n.Todo}, CompletedAt: n.CompletedAt,
		})
		if err != nil {
			if isRecordProblem(err) {
				st.note("note %d: %v", n.ID, err)
				continue
			}
			return err
		}
	}
	return nil
}

func urlValues(key, value string) url.Values { return url.Values{key: {value}} }

// fieldValue is a custom field value with the field's id here.
type fieldValue struct {
	id    resource.ID
	value string
}

// fieldValues maps a record's custom field values to the fields here,
// leaving out fields that did not come in.
func (r *run) fieldValues(ctx context.Context, in []mbFieldValue) ([]fieldValue, error) {
	var out []fieldValue
	for _, f := range in {
		id, err := r.mapped(ctx, kindCustomField, ref(&f.ID))
		if err != nil {
			return nil, err
		}
		if id != nil {
			out = append(out, fieldValue{id: *id, value: f.Value})
		}
	}
	return out, nil
}

// invoiceRefs maps a document's workflow and identity to the ones here,
// nil leaving the defaults.
func (r *run) invoiceRefs(ctx context.Context, workflow, identity *mbID) (*resource.ID, *resource.ID, error) {
	w, err := r.mapped(ctx, kindWorkflow, ref(workflow))
	if err != nil {
		return nil, nil, err
	}
	i, err := r.mapped(ctx, kindIdentity, ref(identity))
	return w, i, err
}
