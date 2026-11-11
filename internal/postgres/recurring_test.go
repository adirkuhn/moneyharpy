package postgres_test

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/adirkuhn/moneyharpy/internal/notes"
	"github.com/adirkuhn/moneyharpy/internal/resource"
	"github.com/adirkuhn/moneyharpy/internal/sales"
)

func TestRecurringInvoicesAreStoredAndMakeInvoices(t *testing.T) {
	ctx := context.Background()
	f := newSalesFixture(t)
	today := adminNow().Format(time.DateOnly)
	r, err := f.svc.CreateRecurring(ctx, f.admin, f.owner, sales.RecurringChanges{
		ContactID: &f.customer, Reference: ptr("Contract 7"), InvoiceDate: ptr(today), FrequencyType: ptr(sales.Quarter), Frequency: ptr(2),
		AutoSend: ptr(true), LastDate: ptr("2099-12-31"), Discount: ptr("10"),
		Details: []sales.DetailInput{{Description: ptr("Hosting {period}"), Price: ptr("100"), Amount: ptr("1"), TaxRateID: &f.vat21, LedgerAccountID: &f.revenue}},
	})
	if err != nil {
		t.Fatal(err)
	}
	got, err := f.store.RecurringInvoice(ctx, f.admin, r.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.ContactID != f.customer || got.Reference != "Contract 7" || got.FrequencyType != sales.Quarter || got.Frequency != 2 || !got.AutoSend ||
		got.AutoSendUserID == nil || *got.AutoSendUserID != f.owner || got.AutoSendAt == nil || got.LastDate.Format(time.DateOnly) != "2099-12-31" ||
		got.Discount != 1000 || !got.Active || got.StartDate.Format(time.DateOnly) != today || got.Anchor.Format(time.DateOnly) != today {
		t.Fatalf("stored = %+v", got)
	}
	if len(got.Lines) != 1 || got.Lines[0].Description != "Hosting {period}" || got.Totals.InclTax != 10890 || len(got.Totals.ByRate) != 1 || len(got.Totals.LineBases) != 1 {
		t.Errorf("lines and totals = %+v, %+v", got.Lines, got.Totals)
	}
	if used, err := f.store.ContactInUse(ctx, f.admin, f.customer); err != nil || !used {
		t.Errorf("contact in use = %v, %v", used, err)
	}
	if work, err := f.store.Workload(ctx); err != nil || !slices.ContainsFunc(work, func(w sales.Workload) bool { return w.AdministrationID == f.admin && w.UserID == f.owner }) {
		t.Errorf("workload = %+v, %v", work, err)
	}
	n, err := f.svc.AddRecurringNote(ctx, f.admin, r.ID, f.owner, notes.Input{Note: "Renew in March"})
	if err != nil {
		t.Fatal(err)
	}
	if withNote, _ := f.svc.Recurring(ctx, f.admin, r.ID, f.owner); len(withNote.Notes) != 1 || withNote.Notes[0].ID != n.ID {
		t.Errorf("notes = %+v", withNote.Notes)
	}

	// As if the worker had been down since two periods ago: it catches up,
	// making each invoice with the recurring invoice moved on at once.
	start := adminNow().AddDate(0, -9, 0)
	start = time.Date(start.Year(), start.Month(), start.Day(), 0, 0, 0, 0, time.UTC)
	got.InvoiceDate, got.Anchor, got.StartDate, got.AutoSend = start, start, start, false
	if err := f.store.SaveRecurring(ctx, got, false); err != nil {
		t.Fatal(err)
	}
	if err := f.svc.Work(ctx); err != nil {
		t.Fatal(err)
	}
	made, _, err := f.store.Invoices(ctx, f.admin, sales.Query{RecurringSalesInvoiceID: &r.ID, Ascending: true})
	if err != nil || len(made) != 2 {
		t.Fatalf("made = %d, %v", len(made), err)
	}
	if made[0].State != sales.Draft || made[0].InvoiceDate.Format(time.DateOnly) != start.Format(time.DateOnly) || made[0].Reference != "Contract 7" ||
		made[0].Totals.InclTax != 10890 || made[1].InvoiceDate.Format(time.DateOnly) != sales.Occurrence(start, sales.Quarter, 2, 1).Format(time.DateOnly) {
		t.Errorf("made = %+v", made)
	}
	moved, _ := f.store.RecurringInvoice(ctx, f.admin, r.ID)
	if moved.Generated != 2 || moved.Periods != 2 || moved.InvoiceDate.Format(time.DateOnly) != sales.Occurrence(start, sales.Quarter, 2, 2).Format(time.DateOnly) {
		t.Errorf("moved = %+v", moved)
	}
	if err := f.svc.Work(ctx); err != nil {
		t.Fatal(err)
	}
	if again, _, _ := f.store.Invoices(ctx, f.admin, sales.Query{RecurringSalesInvoiceID: &r.ID}); len(again) != 2 {
		t.Errorf("a second round made %d", len(again))
	}

	other, err := f.svc.CreateRecurring(ctx, f.admin, f.owner, sales.RecurringChanges{
		ContactID: &f.customer, FrequencyType: ptr(sales.Year),
		Details: []sales.DetailInput{{Description: ptr("Licence"), Price: ptr("50"), Amount: ptr("1"), TaxRateID: &f.vat21}},
	})
	if err != nil {
		t.Fatal(err)
	}
	q, _ := sales.ParseRecurringFilter("frequency:quarter,auto_send:false")
	q.PerPage = 10
	if list, total, err := f.store.RecurringInvoices(ctx, f.admin, q); err != nil || total != 1 || list[0].ID != r.ID {
		t.Errorf("quarterly = %d, %v", total, err)
	}
	if byID, err := f.store.RecurringInvoicesByID(ctx, f.admin, []resource.ID{other.ID, 1}); err != nil || len(byID) != 1 {
		t.Errorf("by id = %+v, %v", byID, err)
	}
	if kept, err := f.svc.DeleteRecurring(ctx, f.admin, other.ID, f.owner); err != nil || kept {
		t.Errorf("delete unused = %v, %v", kept, err)
	}
	if kept, err := f.svc.DeleteRecurring(ctx, f.admin, r.ID, f.owner); err != nil || !kept {
		t.Errorf("delete used = %v, %v", kept, err)
	}
	q, _ = sales.ParseRecurringFilter("state:all")
	if list, _, err := f.store.RecurringInvoices(ctx, f.admin, q); err != nil || len(list) != 1 || list[0].Active {
		t.Errorf("left = %+v, %v", list, err)
	}
}
