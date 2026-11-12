package sales

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/adirkuhn/moneyharpy/internal/notes"
	"github.com/adirkuhn/moneyharpy/internal/resource"
)

func date(s string) time.Time {
	d, err := time.Parse(time.DateOnly, s)
	if err != nil {
		panic(err)
	}
	return d
}

func TestOccurrenceKeepsTheStartDayClampedToTheMonthEnd(t *testing.T) {
	for _, tc := range []struct {
		start, typ string
		every, n   int
		want       string
	}{
		{"2026-01-31", Month, 1, 1, "2026-02-28"},
		{"2026-01-31", Month, 1, 2, "2026-03-31"},
		{"2026-01-31", Month, 1, 3, "2026-04-30"},
		{"2028-01-31", Month, 1, 1, "2028-02-29"},
		{"2026-11-30", Quarter, 1, 1, "2027-02-28"},
		{"2026-11-30", Quarter, 1, 2, "2027-05-30"},
		{"2028-02-29", Year, 1, 1, "2029-02-28"},
		{"2028-02-29", Year, 1, 4, "2032-02-29"},
		{"2026-11-11", Day, 10, 3, "2026-12-11"},
		{"2026-11-11", Week, 2, 2, "2026-12-09"},
		{"2026-11-15", Month, 2, 1, "2027-01-15"},
		{"2026-11-15", Month, 1, 0, "2026-11-15"},
	} {
		if got := Occurrence(date(tc.start), tc.typ, tc.every, tc.n).Format(time.DateOnly); got != tc.want {
			t.Errorf("Occurrence(%s, %d %s, %d) = %s, want %s", tc.start, tc.every, tc.typ, tc.n, got, tc.want)
		}
	}
}

// recurring makes a monthly recurring invoice of 100 + 21% VAT starting on
// start. A start in the past is made back then, as if the worker had not run
// since.
func recurring(t *testing.T, h harness, start string, c RecurringChanges) Recurring {
	t.Helper()
	if at := *h.clock; date(start).Before(at) {
		*h.clock = date(start)
		defer func() { *h.clock = at }()
	}
	c.ContactID, c.InvoiceDate = ptr(customer), ptr(start)
	if c.FrequencyType == nil {
		c.FrequencyType = ptr(Month)
	}
	if c.Details == nil {
		c.Details = []DetailInput{{Description: ptr("Hosting {period}"), Price: ptr("100"), Amount: ptr("1"), TaxRateID: ptr(vat21), LedgerAccountID: ptr(revenue), Period: ptr("20261101..20261130")}}
	}
	r, err := h.svc.CreateRecurring(context.Background(), adminID, owner, c)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func TestCreateRecurring(t *testing.T) {
	h := newHarness(t)
	r := recurring(t, h, "2026-11-30", RecurringChanges{Reference: ptr("Contract 7")})
	if !r.Active || r.Frequency != 1 || r.FrequencyType != Month || r.WorkflowID != flow || r.DocumentStyleID != style || r.IdentityID != identity {
		t.Errorf("recurring = %+v", r)
	}
	if r.StartDate.Format(time.DateOnly) != "2026-11-30" || r.InvoiceDate.Format(time.DateOnly) != "2026-11-30" || r.LastDate != nil {
		t.Errorf("dates = %v %v %v", r.StartDate, r.InvoiceDate, r.LastDate)
	}
	if r.Totals.InclTax != 12100 || r.Language != "nl" || r.FirstDueInterval != 14 || r.Currency != "EUR" {
		t.Errorf("totals and defaults = %+v", r)
	}
	if r.Ref().URI() != "/100/recurring_sales_invoices/"+r.ID.String() {
		t.Errorf("uri = %s", r.Ref().URI())
	}

	counted := recurring(t, h, "2026-11-30", RecurringChanges{HasDesiredCount: ptr(true), DesiredCount: ptr(3)})
	if counted.LastDate == nil || counted.LastDate.Format(time.DateOnly) != "2027-01-30" {
		t.Errorf("last date of 3 = %v", counted.LastDate)
	}
	ended := recurring(t, h, "2026-11-30", RecurringChanges{LastDate: ptr("2027-06-01")})
	if ended.LastDate.Format(time.DateOnly) != "2027-06-01" {
		t.Errorf("last date = %v", ended.LastDate)
	}
}

func TestCreateRecurringValidates(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	good := func() RecurringChanges {
		return RecurringChanges{ContactID: ptr(customer), InvoiceDate: ptr("2026-12-01"), FrequencyType: ptr(Month), Details: []DetailInput{line("10", "1", vat21, 0)}}
	}
	for field, change := range map[string]func(c *RecurringChanges){
		"contact_id":         func(c *RecurringChanges) { c.ContactID = ptr(resource.ID(999)) },
		"frequency_type":     func(c *RecurringChanges) { c.FrequencyType = ptr("fortnight") },
		"frequency":          func(c *RecurringChanges) { c.Frequency = ptr(0) },
		"invoice_date":       func(c *RecurringChanges) { c.InvoiceDate = ptr("2026-11-01") },
		"desired_count":      func(c *RecurringChanges) { c.HasDesiredCount, c.DesiredCount = ptr(true), ptr(0) },
		"last_date":          func(c *RecurringChanges) { c.LastDate = ptr("2026-11-20") },
		"details_attributes": func(c *RecurringChanges) { c.Details = []DetailInput{} },
		"discount":           func(c *RecurringChanges) { c.Discount = ptr("200") },
	} {
		c := good()
		change(&c)
		if _, err := h.svc.CreateRecurring(ctx, adminID, owner, c); fieldsOf(t, err)[field] == nil {
			t.Errorf("no error for %s: %v", field, err)
		}
	}
	if _, err := h.svc.CreateRecurring(ctx, adminID, owner, RecurringChanges{InvoiceDate: ptr("2026-12-01")}); fieldsOf(t, err)["frequency_type"] != nil {
		t.Errorf("frequency type has no default: %v", err)
	}
	if _, err := h.svc.CreateRecurring(ctx, adminID, stranger, good()); err == nil {
		t.Error("a stranger made a recurring invoice")
	}
}

func TestUpdateRecurringMovesTheSchedule(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	r := recurring(t, h, "2026-11-30", RecurringChanges{})
	got, err := h.svc.UpdateRecurring(ctx, adminID, r.ID, owner, RecurringChanges{
		InvoiceDate: ptr("2026-12-15"), FrequencyType: ptr(Quarter), AutoSend: ptr(true), Reference: ptr("Contract 8"),
		Details: []DetailInput{{ID: &r.Lines[0].ID, Price: ptr("200")}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.InvoiceDate.Format(time.DateOnly) != "2026-12-15" || got.StartDate.Format(time.DateOnly) != "2026-11-30" || got.FrequencyType != Quarter {
		t.Errorf("schedule = %+v", got)
	}
	if !got.AutoSend || got.AutoSendUserID == nil || *got.AutoSendUserID != owner || got.Totals.InclTax != 24200 || got.Reference != "Contract 8" {
		t.Errorf("updated = %+v", got)
	}
	if _, err := h.svc.UpdateRecurring(ctx, adminID, r.ID, owner, RecurringChanges{InvoiceDate: ptr("2026-10-01")}); fieldsOf(t, err)["invoice_date"] == nil {
		t.Errorf("moved into the past: %v", err)
	}
}

func TestDeactivateAndDeleteRecurring(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	unused := recurring(t, h, "2026-12-01", RecurringChanges{})
	kept, err := h.svc.DeleteRecurring(ctx, adminID, unused.ID, owner)
	if err != nil || kept {
		t.Fatalf("delete unused = %v, %v", kept, err)
	}
	if _, err := h.svc.Recurring(ctx, adminID, unused.ID, owner); !errors.Is(err, ErrNotFound) {
		t.Errorf("deleted one = %v", err)
	}

	used := recurring(t, h, "2026-11-01", RecurringChanges{})
	if err := h.svc.Work(ctx); err != nil {
		t.Fatal(err)
	}
	kept, err = h.svc.DeleteRecurring(ctx, adminID, used.ID, owner)
	if err != nil || !kept {
		t.Fatalf("delete used = %v, %v", kept, err)
	}
	got, _ := h.svc.Recurring(ctx, adminID, used.ID, owner)
	if got.Active {
		t.Error("a used recurring invoice was not kept as deactivated")
	}
	if _, err := h.svc.DeleteRecurring(ctx, adminID, used.ID, owner); !errors.Is(err, ErrNotEditable) {
		t.Errorf("deleting a deactivated used one = %v", err)
	}

	fresh := recurring(t, h, "2026-12-01", RecurringChanges{})
	off, err := h.svc.DeactivateRecurring(ctx, adminID, fresh.ID, owner)
	if err != nil || off.Active {
		t.Fatalf("deactivate = %+v, %v", off, err)
	}
	if _, err := h.svc.DeactivateRecurring(ctx, adminID, fresh.ID, owner); !errors.Is(err, ErrNotEditable) {
		t.Errorf("deactivating twice = %v", err)
	}
}

// An invoice a subscription's recurring invoice made tells so, with
// Moneybird's sales_invoice_created_based_on_subscription; the subscriptions
// service sends its webhook once it has completed the invoice.
func TestInvoicesOfASubscriptionTellTheyComeFromIt(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	r := recurring(t, h, "2026-11-01", RecurringChanges{})
	sub := resource.ID(77)
	stored := h.store.recurring[r.ID]
	stored.SubscriptionID = &sub
	h.store.recurring[r.ID] = stored
	if err := h.svc.Work(ctx); err != nil {
		t.Fatal(err)
	}
	made := h.store.fromRecurring(r.ID)
	if len(made) != 1 {
		t.Fatalf("made %d invoices", len(made))
	}
	if got := actions(t, h, made[0].ID); !slices.Equal(got, []string{"sales_invoice_created_based_on_subscription"}) {
		t.Errorf("history = %v", got)
	}
	if slices.Contains(h.notifier.actions, "sales_invoice_created_based_on_recurring") || slices.Contains(h.notifier.actions, "sales_invoice_created") {
		t.Errorf("webhook events = %v", h.notifier.actions)
	}
}

func TestWorkerMakesTheInvoicesThatAreDueAndCatchesUp(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	// Today is 11 November: the invoices of 30 September and 30 October are due.
	r := recurring(t, h, "2026-09-30", RecurringChanges{Reference: ptr("Contract 7")})
	if err := h.svc.Work(ctx); err != nil {
		t.Fatal(err)
	}
	made := h.store.fromRecurring(r.ID)
	if len(made) != 2 {
		t.Fatalf("made %d invoices", len(made))
	}
	first, second := made[0], made[1]
	if first.InvoiceDate.Format(time.DateOnly) != "2026-09-30" || second.InvoiceDate.Format(time.DateOnly) != "2026-10-30" {
		t.Errorf("dates = %v, %v", first.InvoiceDate, second.InvoiceDate)
	}
	if first.State != Draft || first.ContactID != customer || first.Reference != "Contract 7" || first.WorkflowID != flow || first.Totals.InclTax != 12100 {
		t.Errorf("first = %+v", first)
	}
	if first.Lines[0].Description != "Hosting 2026-09-30 – 2026-10-29" || first.Lines[0].Period != "20260930..20261029" {
		t.Errorf("first line = %+v", first.Lines[0])
	}
	if second.Lines[0].Period != "20261030..20261129" || first.Lines[0].ID == second.Lines[0].ID || first.DraftID == second.DraftID {
		t.Errorf("second line = %+v", second.Lines[0])
	}
	got, _ := h.svc.Recurring(ctx, adminID, r.ID, owner)
	if got.InvoiceDate.Format(time.DateOnly) != "2026-11-30" || !got.Active {
		t.Errorf("next = %+v", got)
	}

	// A second round makes nothing new.
	if err := h.svc.Work(ctx); err != nil {
		t.Fatal(err)
	}
	if len(h.store.fromRecurring(r.ID)) != 2 {
		t.Error("the worker made an invoice twice")
	}
	if n := countOf(h.notifier.actions, "recurring_sales_invoice_invoice_created"); n != 2 {
		t.Errorf("invoice created events = %d in %v", n, h.notifier.actions)
	}
	// Moneybird's event of an invoice a recurring invoice made.
	if n := countOf(h.notifier.actions, "sales_invoice_created_based_on_recurring"); n != 2 || slices.Contains(h.notifier.actions, "sales_invoice_created") {
		t.Errorf("sales invoice events = %v", h.notifier.actions)
	}
	if got := actions(t, h, first.ID); !slices.Equal(got, []string{"sales_invoice_created_based_on_recurring"}) {
		t.Errorf("history = %v", got)
	}
	list, _, err := h.svc.List(ctx, adminID, owner, Query{RecurringSalesInvoiceID: &r.ID})
	if err != nil || len(list) != 2 {
		t.Errorf("invoices of the recurring invoice = %d, %v", len(list), err)
	}

	// A month later the next is due.
	*h.clock = time.Date(2026, 12, 1, 9, 0, 0, 0, time.UTC)
	if err := h.svc.Work(ctx); err != nil {
		t.Fatal(err)
	}
	if made := h.store.fromRecurring(r.ID); len(made) != 3 || made[2].InvoiceDate.Format(time.DateOnly) != "2026-11-30" {
		t.Errorf("after a month = %d", len(made))
	}
}

func TestWorkerStopsAfterTheLastDate(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	r := recurring(t, h, "2026-08-15", RecurringChanges{HasDesiredCount: ptr(true), DesiredCount: ptr(2)})
	if err := h.svc.Work(ctx); err != nil {
		t.Fatal(err)
	}
	if made := h.store.fromRecurring(r.ID); len(made) != 2 {
		t.Errorf("made %d, want 2", len(made))
	}
	got, _ := h.svc.Recurring(ctx, adminID, r.ID, owner)
	if got.Active {
		t.Errorf("still active after the last date: %+v", got)
	}
}

func TestWorkerLeavesInactiveOnesAlone(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	r := recurring(t, h, "2026-12-01", RecurringChanges{})
	if _, err := h.svc.DeactivateRecurring(ctx, adminID, r.ID, owner); err != nil {
		t.Fatal(err)
	}
	*h.clock = time.Date(2027, 3, 1, 9, 0, 0, 0, time.UTC)
	if err := h.svc.Work(ctx); err != nil {
		t.Fatal(err)
	}
	if made := h.store.fromRecurring(r.ID); len(made) != 0 {
		t.Errorf("a deactivated recurring invoice made %d", len(made))
	}
}

func TestWorkerAutoSends(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	r := recurring(t, h, "2026-11-11", RecurringChanges{AutoSend: ptr(true)})
	if err := h.svc.Work(ctx); err != nil {
		t.Fatal(err)
	}
	made := h.store.fromRecurring(r.ID)
	if len(made) != 1 || made[0].State != Open || made[0].InvoiceID == "" {
		t.Fatalf("made = %+v", made)
	}
	if len(h.mailer.sent) != 1 || h.mailer.sent[0].To[0] != "billing@example.com" {
		t.Errorf("mail = %+v", h.mailer.sent)
	}
	if !slices.Contains(h.notifier.actions, "sales_invoice_send_email") {
		t.Errorf("events = %v", h.notifier.actions)
	}

	// Without an email address the invoice stays a draft and the error is told.
	other, err := h.svc.CreateRecurring(ctx, adminID, owner, RecurringChanges{ContactID: ptr(noEmail), InvoiceDate: ptr("2026-11-11"), FrequencyType: ptr(Year),
		AutoSend: ptr(true), Details: []DetailInput{line("10", "1", vat21, 0)}})
	if err != nil {
		t.Fatal(err)
	}
	if err := h.svc.Work(ctx); err == nil {
		t.Error("a failed automatic sending was not told")
	}
	if made := h.store.fromRecurring(other.ID); len(made) != 1 || made[0].State != Draft {
		t.Errorf("unsendable = %+v", made)
	}
	if got, _ := h.svc.Recurring(ctx, adminID, other.ID, owner); got.InvoiceDate.Format(time.DateOnly) != "2027-11-11" {
		t.Errorf("next after a failed sending = %v", got.InvoiceDate)
	}
}

func TestRecurringListFilterAndNotes(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	monthly := recurring(t, h, "2026-12-01", RecurringChanges{})
	yearly := recurring(t, h, "2026-12-05", RecurringChanges{FrequencyType: ptr(Year), AutoSend: ptr(true)})
	off := recurring(t, h, "2026-12-03", RecurringChanges{})
	if _, err := h.svc.DeactivateRecurring(ctx, adminID, off.ID, owner); err != nil {
		t.Fatal(err)
	}
	for filter, want := range map[string][]resource.ID{
		"":                         {monthly.ID, yearly.ID},
		"state:inactive":           {off.ID},
		"state:all":                {monthly.ID, off.ID, yearly.ID},
		"frequency:year":           {yearly.ID},
		"auto_send:false":          {monthly.ID},
		"contact_id:300,state:all": {monthly.ID, off.ID, yearly.ID},
		"frequency:all,auto_send:null,include_subscriptions:true": {monthly.ID, yearly.ID},
	} {
		q, err := ParseRecurringFilter(filter)
		if err != nil {
			t.Fatalf("%q: %v", filter, err)
		}
		list, total, err := h.svc.RecurringList(ctx, adminID, owner, q)
		if err != nil || total != len(want) {
			t.Fatalf("%q: total %d, %v", filter, total, err)
		}
		var ids []resource.ID
		for _, r := range list {
			ids = append(ids, r.ID)
		}
		if !slices.Equal(ids, want) {
			t.Errorf("%q = %v, want %v", filter, ids, want)
		}
	}
	for _, bad := range []string{"state:draft", "frequency:hour", "auto_send:maybe", "period:this_year", "contact_id:x"} {
		if _, err := ParseRecurringFilter(bad); err == nil {
			t.Errorf("filter %q accepted", bad)
		}
	}

	n, err := h.svc.AddRecurringNote(ctx, adminID, monthly.ID, owner, notes.Input{Note: "Renew in March"})
	if err != nil {
		t.Fatal(err)
	}
	if got, _ := h.svc.Recurring(ctx, adminID, monthly.ID, owner); len(got.Notes) != 1 || got.Notes[0].EntityType != RecurringEntityType {
		t.Errorf("notes = %+v", got.Notes)
	}
	if err := h.svc.DeleteRecurringNote(ctx, adminID, monthly.ID, n.ID, owner); err != nil {
		t.Fatal(err)
	}
	byID, err := h.svc.RecurringByID(ctx, adminID, owner, []resource.ID{yearly.ID, 42})
	if err != nil || len(byID) != 1 {
		t.Errorf("by id = %d, %v", len(byID), err)
	}
}

func countOf(list []string, s string) int {
	n := 0
	for _, v := range list {
		if v == s {
			n++
		}
	}
	return n
}

// addingHook adds a line to every invoice a recurring invoice makes.
type addingHook struct {
	svc   *Service
	calls int
}

func (a *addingHook) Generated(ctx context.Context, admin, user resource.ID, inv Invoice, _ Recurring) (Invoice, error) {
	a.calls++
	return a.svc.Update(ctx, admin, inv.ID, user, Changes{Details: []DetailInput{line("5", "1", vat21, 0)}})
}

func TestRecurringHookCompletesTheInvoiceBeforeItIsSent(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	hook := &addingHook{svc: h.svc}
	h.svc.UseRecurringHook(hook)
	r := recurring(t, h, "2026-11-11", RecurringChanges{AutoSend: ptr(true)})
	if err := h.svc.Work(ctx); err != nil {
		t.Fatal(err)
	}
	made := h.store.fromRecurring(r.ID)
	if hook.calls != 1 || len(made) != 1 || made[0].State != Open || len(made[0].Lines) != 2 || made[0].Totals.InclTax != 12705 {
		t.Fatalf("calls %d, made = %+v", hook.calls, made)
	}
}
