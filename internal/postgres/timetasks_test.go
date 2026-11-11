package postgres_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/adirkuhn/moneyharpy/internal/administration"
	"github.com/adirkuhn/moneyharpy/internal/notes"
	"github.com/adirkuhn/moneyharpy/internal/postgres"
	"github.com/adirkuhn/moneyharpy/internal/projects"
	"github.com/adirkuhn/moneyharpy/internal/resource"
	"github.com/adirkuhn/moneyharpy/internal/sales"
	"github.com/adirkuhn/moneyharpy/internal/tasklists"
	"github.com/adirkuhn/moneyharpy/internal/timeentries"
)

func TestTimeEntriesRoundTripFilterAndBill(t *testing.T) {
	ctx := context.Background()
	f := newSalesFixture(t)
	admins := administration.NewService(postgres.NewAdministrationStore(testPool), ids, time.Now)
	noteSvc := notes.NewService(postgres.NewNoteStore(testPool), ids, time.Now)
	svc := timeentries.NewService(admins, postgres.NewTimeEntryStore(testPool), noteSvc, ids, time.Now)
	boss := timeentries.Actor{ID: f.owner, AllUsers: true}
	falcon, err := f.projects.Create(ctx, f.admin, f.owner, projects.Changes{Name: ptr("Falcon")})
	if err != nil {
		t.Fatal(err)
	}
	at := func(s string) *time.Time {
		v, _ := time.Parse(time.RFC3339, s)
		return &v
	}
	work, err := svc.Create(ctx, f.admin, boss, timeentries.Input{UserID: &f.owner, StartedAt: at("2026-03-02T08:00:00Z"), EndedAt: at("2026-03-02T10:30:00Z"),
		Description: ptr("Designing 100% of it"), ContactID: &f.customer, ProjectID: &falcon.ID, PausedDuration: ptr(900)})
	if err != nil {
		t.Fatal(err)
	}
	free, _ := svc.Create(ctx, f.admin, boss, timeentries.Input{UserID: &f.owner, StartedAt: at("2026-03-03T08:00:00Z"), EndedAt: at("2026-03-03T09:00:00Z"),
		Description: ptr("Coffee"), Billable: ptr(false)})
	running, _ := svc.Create(ctx, f.admin, boss, timeentries.Input{UserID: &f.owner, StartedAt: at("2026-03-04T08:00:00Z"), Description: ptr("Now")})

	got, err := svc.Entry(ctx, f.admin, work.ID, boss)
	if err != nil || got.PausedDuration != 900 || *got.ProjectID != falcon.ID || *got.ContactID != f.customer || !got.EndedAt.Equal(*at("2026-03-02T10:30:00Z")) {
		t.Fatalf("Entry() = %+v, %v", got, err)
	}
	list := func(filter string) []resource.ID {
		t.Helper()
		q, err := timeentries.ParseFilter(filter, time.Date(2026, 3, 10, 0, 0, 0, 0, time.UTC))
		if err != nil {
			t.Fatal(err)
		}
		entries, _, err := svc.List(ctx, f.admin, boss, q)
		if err != nil {
			t.Fatal(err)
		}
		out := []resource.ID{}
		for _, e := range entries {
			out = append(out, e.ID)
		}
		return out
	}
	for filter, want := range map[string][]resource.ID{
		"":                                      {free.ID, work.ID},
		"period:this_month,include_active:true": {running.ID, free.ID, work.ID},
		"state:open":                            {work.ID},
		"state:non_billable":                    {free.ID},
		"contact_id:" + f.customer.String():     {work.ID},
		"contact_id:" + f.customer.String() + ",include_nil_contacts:true": {free.ID, work.ID},
		"project_id:" + falcon.ID.String():                                 {work.ID},
		"day:2026-03-03":                                                   {free.ID},
		"period:202602..202602":                                            {},
	} {
		if got := list(filter); !equalIDs(got, want) {
			t.Errorf("filter %q = %v, want %v", filter, got, want)
		}
	}
	q, _ := timeentries.ParseFilter("state:all", time.Now())
	q.Search = "100%"
	if found, total, err := svc.List(ctx, f.admin, boss, q); err != nil || total != 1 || found[0].ID != work.ID {
		t.Errorf("search = %+v (%d), %v", found, total, err)
	}
	q.Search, q.PerPage, q.Page = "", 1, 2
	if page, total, _ := svc.List(ctx, f.admin, boss, q); total != 2 || len(page) != 1 || page[0].ID != work.ID {
		t.Errorf("page 2 = %+v of %d", page, total)
	}
	users, err := svc.Users(ctx, f.admin, []resource.ID{f.owner, 404})
	if err != nil || len(users) != 1 || users[f.owner].Name == "" {
		t.Errorf("Users() = %+v, %v", users, err)
	}

	inv, err := f.svc.Create(ctx, f.admin, f.owner, sales.Changes{ContactID: &f.customer})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Bill(ctx, f.admin, inv.ID, boss, []resource.ID{work.ID}); err != nil {
		t.Fatal(err)
	}
	billed, err := svc.OfInvoices(ctx, f.admin, []resource.ID{inv.ID})
	if err != nil || len(billed[inv.ID]) != 1 || billed[inv.ID][0].ID != work.ID {
		t.Errorf("OfInvoices() = %+v, %v", billed, err)
	}
	if err := f.svc.Delete(ctx, f.admin, inv.ID, f.owner); err != nil {
		t.Fatal(err)
	}
	if got, _ := svc.Entry(ctx, f.admin, work.ID, boss); got.SalesInvoiceID != nil {
		t.Errorf("entry keeps a deleted invoice: %+v", got)
	}
	if err := svc.Delete(ctx, f.admin, running.ID, boss); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Entry(ctx, f.admin, running.ID, boss); !errors.Is(err, timeentries.ErrNotFound) {
		t.Errorf("deleted = %v", err)
	}
}

func equalIDs(a, b []resource.ID) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestTaskListsRoundTrip(t *testing.T) {
	ctx := context.Background()
	f := newSalesFixture(t)
	admins := administration.NewService(postgres.NewAdministrationStore(testPool), ids, time.Now)
	noteSvc := notes.NewService(postgres.NewNoteStore(testPool), ids, time.Now)
	svc := tasklists.NewService(admins, postgres.NewTaskListStore(testPool), noteSvc, tasklists.LedgerReports{Accounts: f.books}, ids, time.Now)

	tpl, err := svc.CreateTemplate(ctx, f.admin, f.owner, tasklists.TemplateInput{Name: ptr("Month end"), Period: ptr("monthly"), DeadlineAfter: ptr("P10D")})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.AddTask(ctx, f.admin, tpl.Groups[0].ID, f.owner, tasklists.TaskInput{Name: ptr("Reconcile"), RelatedEntityID: &f.revenue}); err != nil {
		t.Fatal(err)
	}
	balance, err := svc.AddTemplateGroup(ctx, f.admin, tpl.ID, f.owner, tasklists.GroupInput{Name: ptr("Balance"), ReportType: ptr("balance_sheet")})
	if err != nil || len(balance.Tasks) == 0 {
		t.Fatalf("generated group = %+v, %v", balance, err)
	}
	l, err := svc.FromTemplate(ctx, f.admin, tpl.ID, f.owner, tasklists.ListInput{})
	if err != nil {
		t.Fatal(err)
	}
	stored, err := svc.List(ctx, f.admin, l.ID, f.owner)
	if err != nil || len(stored.Groups) != 2 || !stored.Groups[0].First || stored.Groups[0].Name != nil || stored.Groups[0].Tasks[0].Name != "Reconcile" ||
		*stored.Groups[0].Tasks[0].RelatedEntityID != f.revenue || *stored.TemplateID != tpl.ID || stored.PeriodEnd == nil || stored.DeadlineOn == nil ||
		len(stored.Groups[1].Tasks) != len(balance.Tasks) || !stored.Groups[1].Tasks[0].Generated {
		t.Fatalf("List() = %+v, %v", stored, err)
	}
	task := stored.Groups[0].Tasks[0]
	if _, err := svc.Assign(ctx, f.admin, task.ID, f.owner, f.owner); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.AddNote(ctx, f.admin, task.ID, f.owner, notes.Input{Note: "Ask the accountant"}); err != nil {
		t.Fatal(err)
	}
	out, err := svc.Complete(ctx, f.admin, task.ID, f.owner)
	if err != nil || out.Task.CompletedAt == nil || out.List == nil {
		t.Fatalf("Complete() = %+v, %v", out, err)
	}
	got, _ := svc.Task(ctx, f.admin, task.ID, f.owner)
	if got.CompletedAt == nil || *got.AssigneeID != f.owner || len(got.Notes) != 1 {
		t.Errorf("Task() = %+v", got)
	}
	page, total, err := svc.Lists(ctx, f.admin, f.owner, 1, 10)
	if err != nil || total != 1 || page[0].ID != l.ID {
		t.Errorf("Lists() = %+v (%d), %v", page, total, err)
	}
	if _, err := svc.UpdateGroup(ctx, f.admin, stored.Groups[1].ID, f.owner, tasklists.GroupInput{ReportType: ptr("profit_loss")}); err != nil {
		t.Fatal(err)
	}
	if g, _ := svc.Group(ctx, f.admin, stored.Groups[1].ID, f.owner); len(g.Tasks) == 0 || g.Tasks[0].ReportType != "profit_loss" {
		t.Errorf("regenerated = %+v", g.Tasks)
	}
	if err := svc.DeleteTemplate(ctx, f.admin, tpl.ID, f.owner); err != nil {
		t.Fatal(err)
	}
	if again, err := svc.List(ctx, f.admin, l.ID, f.owner); err != nil || again.TemplateID != nil {
		t.Errorf("list after its template went = %+v, %v", again, err)
	}
	if err := svc.DeleteList(ctx, f.admin, l.ID, f.owner); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Task(ctx, f.admin, task.ID, f.owner); !errors.Is(err, tasklists.ErrTaskNotFound) {
		t.Errorf("task of a deleted list = %v", err)
	}
}
