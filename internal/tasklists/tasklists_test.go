package tasklists

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/adirkuhn/moneyharpy/internal/administration"
	"github.com/adirkuhn/moneyharpy/internal/notes"
	"github.com/adirkuhn/moneyharpy/internal/resource"
)

// 2026-11-11 in Amsterdam.
var now = time.Date(2026, 11, 11, 9, 0, 0, 0, time.UTC)

func newService() (*Service, *memStore) {
	store := newMemStore()
	return NewService(fakeAdmins{}, store, &memNotes{}, LedgerReports{Accounts: chart{}}, resource.NewGenerator(1), func() time.Time { return now }), store
}

func ptr[T any](v T) *T { return &v }

func day(s string) *time.Time {
	d, err := time.Parse(time.DateOnly, s)
	if err != nil {
		panic(err)
	}
	return &d
}

func baseProblem(t *testing.T, err error) string {
	t.Helper()
	var verr *resource.ValidationError
	if !errors.As(err, &verr) || len(verr.Fields["base"]) == 0 {
		t.Fatalf("err = %v, want a refusal", err)
	}
	return verr.Fields["base"][0]
}

func TestNewListCoversLastMonthWithAFirstGroup(t *testing.T) {
	svc, _ := newService()
	ctx := context.Background()
	l, err := svc.CreateList(ctx, adminID, owner, ListInput{Name: ptr(" Closing ")})
	if err != nil {
		t.Fatal(err)
	}
	if l.Name != "Closing" || !l.PeriodBegin.Equal(*day("2026-10-01")) || !l.PeriodEnd.Equal(*day("2026-10-31")) || len(l.Groups) != 1 || !l.Groups[0].First ||
		l.Groups[0].Name != nil || !l.Completed() || l.Ref().URI() != "/1/task_lists/"+l.ID.String() {
		t.Errorf("list = %+v", l)
	}
	got, err := svc.List(ctx, adminID, l.ID, owner)
	if err != nil || len(got.Groups) != 1 || got.Groups[0].Tasks == nil {
		t.Errorf("List() = %+v, %v", got, err)
	}
	if _, err := svc.CreateList(ctx, adminID, owner, ListInput{}); err == nil {
		t.Error("list without a name accepted")
	}
	if _, err := svc.CreateList(ctx, adminID, owner, ListInput{Name: ptr("x"), PeriodBegin: day("2026-10-02"), PeriodEnd: day("2026-10-01")}); err == nil {
		t.Error("period ending before it begins accepted")
	}
	if _, err := svc.CreateList(ctx, adminID, owner, ListInput{Name: ptr("x"), PeriodBegin: day("2026-10-02")}); err == nil {
		t.Error("period without an end accepted")
	}
	if _, err := svc.CreateList(ctx, resource.ID(9), owner, ListInput{Name: ptr("x")}); !errors.Is(err, administration.ErrNotFound) {
		t.Errorf("other administration = %v", err)
	}
	up, err := svc.UpdateList(ctx, adminID, l.ID, owner, ListInput{Description: ptr("Month end"), DeadlineOn: day("2026-11-15")})
	if err != nil || up.Description != "Month end" || !up.DeadlineOn.Equal(*day("2026-11-15")) || up.Name != "Closing" {
		t.Errorf("updated = %+v, %v", up, err)
	}
	if up, _ := svc.UpdateList(ctx, adminID, l.ID, owner, ListInput{ClearDeadline: true}); up.DeadlineOn != nil {
		t.Errorf("deadline kept: %+v", up)
	}
	page, total, err := svc.Lists(ctx, adminID, owner, 1, 0)
	if err != nil || total != 1 || len(page[0].Groups) != 1 {
		t.Errorf("Lists() = %+v (%d), %v", page, total, err)
	}
	if err := svc.DeleteList(ctx, adminID, l.ID, owner); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.List(ctx, adminID, l.ID, owner); !errors.Is(err, ErrListNotFound) {
		t.Errorf("deleted list = %v", err)
	}
}

func TestListsFromTemplates(t *testing.T) {
	svc, _ := newService()
	ctx := context.Background()
	tpl, err := svc.CreateTemplate(ctx, adminID, owner, TemplateInput{Name: ptr("Quarter close"), Description: ptr("Each quarter"), Period: ptr("quarterly"), DeadlineAfter: ptr("p1m")})
	if err != nil {
		t.Fatal(err)
	}
	if tpl.Period != "P3M" || tpl.DeadlineAfter != "P1M" || len(tpl.Groups) != 1 || !tpl.Groups[0].First {
		t.Fatalf("template = %+v", tpl)
	}
	first := tpl.Groups[0].ID
	if _, err := svc.AddTask(ctx, adminID, first, owner, TaskInput{Name: ptr("Reconcile bank"), Description: ptr("All accounts")}); err != nil {
		t.Fatal(err)
	}
	vat, err := svc.AddTemplateGroup(ctx, adminID, tpl.ID, owner, GroupInput{Name: ptr("VAT")})
	if err != nil || vat.RowOrder != 1 || *vat.Name != "VAT" {
		t.Fatalf("group = %+v, %v", vat, err)
	}
	if _, err := svc.AddTask(ctx, adminID, vat.ID, owner, TaskInput{Name: ptr("File the return")}); err != nil {
		t.Fatal(err)
	}

	l, err := svc.FromTemplate(ctx, adminID, tpl.ID, owner, ListInput{})
	if err != nil {
		t.Fatal(err)
	}
	if l.Name != "Quarter close" || l.Description != "Each quarter" || *l.TemplateID != tpl.ID || !l.PeriodBegin.Equal(*day("2026-07-01")) ||
		!l.PeriodEnd.Equal(*day("2026-09-30")) || !l.DeadlineOn.Equal(*day("2026-10-30")) || len(l.Groups) != 2 {
		t.Fatalf("list = %+v", l)
	}
	if total, done := l.Counts(); total != 2 || done != 0 || l.Completed() {
		t.Errorf("counts = %d, %d", total, done)
	}
	stored, _ := svc.List(ctx, adminID, l.ID, owner)
	if len(stored.Groups) != 2 || stored.Groups[0].ID == first || stored.Groups[0].Tasks[0].Name != "Reconcile bank" || stored.Groups[1].Tasks[0].Name != "File the return" {
		t.Errorf("stored = %+v", stored)
	}
	named, err := svc.CreateList(ctx, adminID, owner, ListInput{Name: ptr("Q3"), TemplateID: &tpl.ID, PeriodBegin: day("2026-01-01"), PeriodEnd: day("2026-03-31")})
	if err != nil || named.Name != "Q3" || !named.DeadlineOn.Equal(*day("2026-04-30")) {
		t.Errorf("named = %+v, %v", named, err)
	}
	if _, err := svc.FromTemplate(ctx, adminID, 404, owner, ListInput{}); !errors.Is(err, ErrTemplateNotFound) {
		t.Errorf("unknown template = %v", err)
	}
	templates, err := svc.Templates(ctx, adminID, owner)
	if err != nil || len(templates) != 1 || len(templates[0].Groups) != 2 {
		t.Errorf("Templates() = %+v, %v", templates, err)
	}
	up, err := svc.UpdateTemplate(ctx, adminID, tpl.ID, owner, TemplateInput{Period: ptr("none"), DeadlineAfter: ptr("")})
	if err != nil || up.Period != "" || up.DeadlineAfter != "" {
		t.Errorf("updated template = %+v, %v", up, err)
	}
	if _, err := svc.UpdateTemplate(ctx, adminID, tpl.ID, owner, TemplateInput{Period: ptr("fortnightly"), DeadlineAfter: ptr("1M")}); err == nil {
		t.Error("bad durations accepted")
	}
	if err := svc.DeleteTemplate(ctx, adminID, tpl.ID, owner); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.List(ctx, adminID, l.ID, owner); err != nil {
		t.Errorf("list made from a deleted template = %v", err)
	}
}

func TestGroupsFollowReports(t *testing.T) {
	svc, _ := newService()
	ctx := context.Background()
	l, _ := svc.CreateList(ctx, adminID, owner, ListInput{Name: ptr("Year end")})
	if _, err := svc.UpdateGroup(ctx, adminID, l.Groups[0].ID, owner, GroupInput{Name: ptr("x")}); baseProblem(t, err) != "The first group cannot be modified" {
		t.Errorf("first group changed: %v", err)
	}
	if err := svc.DeleteGroup(ctx, adminID, l.Groups[0].ID, owner); baseProblem(t, err) != "The first group cannot be deleted" {
		t.Errorf("first group deleted: %v", err)
	}
	if _, err := svc.AddListGroup(ctx, adminID, l.ID, owner, GroupInput{}); err == nil {
		t.Error("group without a name accepted")
	}
	if _, err := svc.AddListGroup(ctx, adminID, l.ID, owner, GroupInput{Name: ptr("x"), ReportType: ptr("cash")}); err == nil {
		t.Error("unknown report accepted")
	}
	g, err := svc.AddListGroup(ctx, adminID, l.ID, owner, GroupInput{Name: ptr("Balance"), ReportType: ptr("balance_sheet")})
	if err != nil {
		t.Fatal(err)
	}
	if len(g.Tasks) != 2 || g.Tasks[0].Name != "1100 Bank" || *g.Tasks[0].RelatedEntityID != bank || !g.Tasks[0].Generated || g.Tasks[1].Name != "Equity" ||
		g.Tasks[0].ReportType != "balance_sheet" {
		t.Fatalf("generated = %+v", g.Tasks)
	}
	if _, err := svc.AddTask(ctx, adminID, g.ID, owner, TaskInput{Name: ptr("Extra")}); baseProblem(t, err) != "Tasks cannot be added to a group with generated tasks" {
		t.Errorf("task added to a generated group: %v", err)
	}
	if err := svc.DeleteTask(ctx, adminID, g.Tasks[0].ID, owner); baseProblem(t, err) != "Generated tasks cannot be deleted" {
		t.Errorf("generated task deleted: %v", err)
	}
	g, err = svc.UpdateGroup(ctx, adminID, g.ID, owner, GroupInput{ReportType: ptr("profit_loss")})
	if err != nil || len(g.Tasks) != 1 || g.Tasks[0].Name != "Revenue" {
		t.Fatalf("regenerated = %+v, %v", g, err)
	}
	if stored, _ := svc.Group(ctx, adminID, g.ID, owner); len(stored.Tasks) != 1 || stored.Tasks[0].Name != "Revenue" {
		t.Errorf("stored = %+v", stored.Tasks)
	}
	if g, _ = svc.UpdateGroup(ctx, adminID, g.ID, owner, GroupInput{Name: ptr("Results")}); *g.Name != "Results" || len(g.Tasks) != 1 {
		t.Errorf("renamed = %+v", g)
	}
	if g, _ := svc.AddListGroup(ctx, adminID, l.ID, owner, GroupInput{Name: ptr("Debtors"), ReportType: ptr("debtor")}); len(g.Tasks) != 0 {
		t.Errorf("debtor group = %+v", g.Tasks)
	}
	if err := svc.DeleteGroup(ctx, adminID, g.ID, owner); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Group(ctx, adminID, g.ID, owner); !errors.Is(err, ErrGroupNotFound) {
		t.Errorf("deleted group = %v", err)
	}
}

func TestTasks(t *testing.T) {
	svc, _ := newService()
	ctx := context.Background()
	l, _ := svc.CreateList(ctx, adminID, owner, ListInput{Name: ptr("Month end")})
	first := l.Groups[0].ID
	bankTask, err := svc.AddTask(ctx, adminID, first, owner, TaskInput{Name: ptr("Bank"), RelatedEntityID: &bank, GenerateSubtasks: ptr(true), AssigneeID: ptr(member)})
	if err != nil {
		t.Fatal(err)
	}
	if len(bankTask.Subtasks) != 1 || bankTask.Subtasks[0].Name != "Savings" || !bankTask.Subtasks[0].Generated || *bankTask.AssigneeID != member {
		t.Fatalf("task = %+v", bankTask)
	}
	other, _ := svc.AddTask(ctx, adminID, first, owner, TaskInput{Name: ptr("Invoices")})
	if other.RowOrder != 1 {
		t.Errorf("row order = %d", other.RowOrder)
	}
	for _, in := range []TaskInput{{}, {Name: ptr("x"), ReportType: ptr("cash")}, {Name: ptr("x"), AssigneeID: ptr(resource.ID(77))}, {Name: ptr("x"), RelatedEntityID: ptr(resource.ID(77))},
		{Name: ptr("x"), GenerateSubtasks: ptr(true)}} {
		if _, err := svc.AddTask(ctx, adminID, first, owner, in); err == nil {
			t.Errorf("AddTask(%+v) accepted", in)
		}
	}

	up, err := svc.UpdateTask(ctx, adminID, bankTask.ID, owner, TaskInput{Name: ptr("Bank accounts"), Description: ptr("Match all"), ReportType: ptr("balance_sheet"), GenerateSubtasks: ptr(true)})
	if err != nil || up.Name != "Bank accounts" || up.ReportType != "balance_sheet" || len(up.Subtasks) != 1 {
		t.Fatalf("updated = %+v, %v", up, err)
	}
	if stored, _ := svc.Task(ctx, adminID, bankTask.ID, owner); len(stored.Subtasks) != 1 || stored.Description != "Match all" {
		t.Errorf("stored = %+v", stored)
	}
	if up, _ = svc.UpdateTask(ctx, adminID, bankTask.ID, owner, TaskInput{RelatedEntityID: ptr(resource.ID(0))}); up.RelatedEntityID != nil {
		t.Errorf("unlinked = %+v", up)
	}

	if _, err := svc.Assign(ctx, adminID, other.ID, owner, 77); !errors.Is(err, ErrAssigneeNotFound) {
		t.Errorf("Assign() stranger = %v", err)
	}
	if got, err := svc.Assign(ctx, adminID, other.ID, owner, owner); err != nil || *got.AssigneeID != owner {
		t.Errorf("Assign() = %+v, %v", got, err)
	}
	if got, err := svc.Unassign(ctx, adminID, other.ID, owner); err != nil || got.AssigneeID != nil {
		t.Errorf("Unassign() = %+v, %v", got, err)
	}

	done, err := svc.Complete(ctx, adminID, bankTask.ID, owner)
	if err != nil || done.Task.CompletedAt == nil || done.Task.Subtasks[0].CompletedAt == nil || done.ListCompleted {
		t.Fatalf("Complete() = %+v, %v", done, err)
	}
	done, _ = svc.Complete(ctx, adminID, other.ID, owner)
	if !done.ListCompleted || done.List == nil || !done.List.Completed() {
		t.Errorf("last task done = %+v", done)
	}
	sub := done.List.Groups[0].Tasks[0].Subtasks[0]
	reopened, err := svc.Reopen(ctx, adminID, sub.ID, owner)
	if err != nil || reopened.Task.CompletedAt != nil || !reopened.ListReopened {
		t.Fatalf("Reopen() = %+v, %v", reopened, err)
	}
	if parent, _ := svc.Task(ctx, adminID, bankTask.ID, owner); parent.CompletedAt != nil {
		t.Error("parent stays completed after a subtask reopened")
	}

	n, err := svc.AddNote(ctx, adminID, other.ID, owner, notes.Input{Note: "Ask the bank"})
	if err != nil || n.EntityType != "TaskLists::Task" {
		t.Fatalf("AddNote() = %+v, %v", n, err)
	}
	if got, _ := svc.Task(ctx, adminID, other.ID, owner); len(got.Notes) != 1 {
		t.Errorf("notes = %+v", got.Notes)
	}
	if err := svc.DeleteTask(ctx, adminID, bankTask.ID, owner); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Task(ctx, adminID, sub.ID, owner); !errors.Is(err, ErrTaskNotFound) {
		t.Errorf("subtask of a deleted task = %v", err)
	}
	if !IsNotFound(ErrAssigneeNotFound) || IsNotFound(errors.New("x")) {
		t.Error("IsNotFound")
	}
}

func TestDurationsAndPeriods(t *testing.T) {
	for in, want := range map[string]string{"monthly": "P1M", "Quarterly": "P3M", "yearly": "P1Y", "none": "", "": "", "p2w": "P2W", "P1Y6M": "P1Y6M"} {
		if got, err := NormalizePeriod(in); err != nil || got != want {
			t.Errorf("NormalizePeriod(%q) = %q, %v", in, got, err)
		}
	}
	for _, bad := range []string{"P", "1M", "PT1H", "P0D", "weekly"} {
		if _, err := NormalizePeriod(bad); err == nil {
			t.Errorf("NormalizePeriod(%q) accepted", bad)
		}
	}
	today := *day("2026-11-11")
	for period, want := range map[string][2]string{
		"P1M": {"2026-10-01", "2026-10-31"}, "P3M": {"2026-07-01", "2026-09-30"}, "P1Y": {"2025-01-01", "2025-12-31"},
		"P6M": {"2026-01-01", "2026-06-30"}, "P1W": {"2026-11-04", "2026-11-10"}, "P2Y": {"2024-01-01", "2025-12-31"},
	} {
		begin, end := previous(today, period)
		if begin.Format(time.DateOnly) != want[0] || end.Format(time.DateOnly) != want[1] {
			t.Errorf("previous(%s) = %s..%s", period, begin.Format(time.DateOnly), end.Format(time.DateOnly))
		}
	}
	if begin, end := previous(today, ""); begin != nil || end != nil {
		t.Error("a period without a length")
	}
}
