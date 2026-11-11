package subscriptions

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/adirkuhn/moneyharpy/internal/resource"
	"github.com/adirkuhn/moneyharpy/internal/sales"
)

func subscribe(t *testing.T, h harness, in Input) Subscription {
	t.Helper()
	if in.ContactID == nil {
		in.ContactID = ptrTo(customer)
	}
	if in.ProductID == nil {
		in.ProductID = ptrTo(monthly)
	}
	if in.StartDate == nil {
		in.StartDate = ptrTo("2026-11-12")
	}
	sub, err := h.svc.Create(context.Background(), adminID, owner, in)
	if err != nil {
		t.Fatal(err)
	}
	return sub
}

func fields(err error) map[string][]string {
	var verr *resource.ValidationError
	if errors.As(err, &verr) {
		return verr.Fields
	}
	return nil
}

func TestPeriodsFit(t *testing.T) {
	for _, tc := range []struct {
		pe   int
		pt   string
		e    int
		typ  string
		want int
	}{
		{1, "month", 1, "year", 12},
		{1, "month", 1, "quarter", 3},
		{1, "month", 2, "month", 2},
		{1, "quarter", 1, "month", 0},
		{1, "week", 2, "week", 2},
		{1, "day", 1, "week", 7},
		{1, "month", 1, "week", 0},
		{0, "", 1, "month", 0},
	} {
		if got := periods(tc.pe, tc.pt, tc.e, tc.typ); got != tc.want {
			t.Errorf("periods(%d %s, %d %s) = %d, want %d", tc.pe, tc.pt, tc.e, tc.typ, got, tc.want)
		}
	}
}

func TestCreateStartsARecurringInvoice(t *testing.T) {
	h := newHarness(t)
	sub := subscribe(t, h, Input{Amount: ptrTo("5x"), Discount: ptrTo("10"), Reference: ptrTo("moneybird.nl")})
	if sub.Frequency != 1 || sub.FrequencyType != "month" || sub.Reference != "moneybird.nl" || sub.RecurringID == nil {
		t.Fatalf("subscription = %+v", sub)
	}
	if len(sub.Items) != 1 || sub.Items[0].Amount != "5x" || sub.Items[0].Quantity != 5000 || sub.Items[0].Discount != 1000 {
		t.Errorf("items = %+v", sub.Items)
	}
	r := h.sales.recurring[*sub.RecurringID]
	if !r.AutoSend || r.InvoiceDate.Format(time.DateOnly) != "2026-11-12" || r.Discount != 1000 || r.ContactID != customer {
		t.Errorf("recurring = %+v", r)
	}
	if len(r.Lines) != 1 || *r.Lines[0].ProductID != monthly || r.Lines[0].Price != 2000 || r.Lines[0].Period != "20261112..20261211" {
		t.Errorf("lines = %+v", r.Lines)
	}
	list, err := h.svc.List(context.Background(), adminID, owner, customer)
	if err != nil || len(list) != 1 {
		t.Errorf("list = %v, %v", list, err)
	}
	if _, err := h.svc.List(context.Background(), adminID, owner, stranger); err == nil {
		t.Error("listed the subscriptions of an unknown contact")
	}
}

func TestCreateBillsNoSoonerThanTomorrowAndAtTheChosenFrequency(t *testing.T) {
	h := newHarness(t)
	sub := subscribe(t, h, Input{StartDate: ptrTo("2026-01-01"), FrequencyType: ptrTo("year"), EndDate: ptrTo("2028-11-12")})
	r := h.sales.recurring[*sub.RecurringID]
	if r.InvoiceDate.Format(time.DateOnly) != "2026-11-12" || r.FrequencyType != "year" || r.Lines[0].Price != 24000 {
		t.Errorf("recurring = %+v", r)
	}
	if r.LastDate == nil || r.LastDate.Format(time.DateOnly) != "2028-11-11" || sub.StartDate.Format(time.DateOnly) != "2026-01-01" {
		t.Errorf("last date = %v, start %v", r.LastDate, sub.StartDate)
	}
}

func TestCreateValidates(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	_, err := h.svc.Create(ctx, adminID, owner, Input{})
	if f := fields(err); f["contact_id"] == nil || f["product_id"] == nil || f["start_date"] == nil {
		t.Errorf("blank = %v", err)
	}
	_, err = h.svc.Create(ctx, adminID, owner, Input{ContactID: ptrTo(stranger), ProductID: ptrTo(oneOff), StartDate: ptrTo("soon"), Amount: ptrTo("many")})
	if f := fields(err); f["contact_id"] == nil || f["product_id"] == nil || f["start_date"] == nil || f["amount"] == nil {
		t.Errorf("invalid = %v", err)
	}
	_, err = h.svc.Create(ctx, adminID, owner, Input{ContactID: ptrTo(customer), ProductID: ptrTo(monthly), StartDate: ptrTo("2026-12-01"), FrequencyType: ptrTo("week")})
	if fields(err)["frequency"] == nil {
		t.Errorf("weekly billing of a monthly product = %v", err)
	}
	if len(h.sales.recurring) != 0 {
		t.Error("a recurring invoice was made for an invalid subscription")
	}
}

func TestUpdateChangesTheProductAndBillsTheDifference(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	sub := subscribe(t, h, Input{StartDate: ptrTo("2026-11-12")})
	// The first period, 12 November to 11 December, is billed.
	*h.clock = time.Date(2026, 11, 12, 9, 0, 0, 0, time.UTC)
	first := h.sales.generate(*sub.RecurringID)
	if _, err := h.svc.Generated(ctx, adminID, owner, first, h.sales.recurring[*sub.RecurringID]); err != nil {
		t.Fatal(err)
	}
	// Halfway through, on 27 November, it moves to premium: 15 of 30 days left.
	*h.clock = time.Date(2026, 11, 27, 9, 0, 0, 0, time.UTC)
	got, err := h.svc.Update(ctx, adminID, sub.ID, owner, Changes{ProductID: ptrTo(premium), Amount: ptrTo("1")})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Items) != 2 || got.Items[0].EndDate == nil || got.Items[0].EndDate.Format(time.DateOnly) != "2026-11-26" ||
		got.Items[1].ProductID != premium || got.Items[1].StartDate.Format(time.DateOnly) != "2026-11-27" || got.Items[1].Amount != "1 x" {
		t.Errorf("items = %+v", got.Items)
	}
	r := h.sales.recurring[*sub.RecurringID]
	if *r.Lines[0].ProductID != premium || r.Lines[0].Price != 3000 || r.Lines[0].Description != "Premium hosting" {
		t.Errorf("recurring line = %+v", r.Lines[0])
	}
	oneOffs := h.store.oneOffs[sub.ID]
	if len(oneOffs) != 1 {
		t.Fatalf("one-off invoices = %v", oneOffs)
	}
	extra := h.sales.invoices[oneOffs[0]]
	if extra.State != sales.Scheduled || extra.InvoiceDate.Format(time.DateOnly) != "2026-12-12" || extra.Lines[0].Price != 500 {
		t.Errorf("difference = %+v", extra)
	}

	// The next invoice takes the difference in, and the one-off goes.
	*h.clock = time.Date(2026, 12, 12, 9, 0, 0, 0, time.UTC)
	next := h.sales.generate(*sub.RecurringID)
	done, err := h.svc.Generated(ctx, adminID, owner, next, h.sales.recurring[*sub.RecurringID])
	if err != nil {
		t.Fatal(err)
	}
	if len(done.Lines) != 2 || done.Lines[1].Price != 500 || !slices.Contains(h.sales.deleted, extra.ID) {
		t.Errorf("merged = %+v, deleted %v", done.Lines, h.sales.deleted)
	}
	if countOf(h.notifier.actions, "sales_invoice_created_based_on_subscription") != 2 {
		t.Errorf("events = %v", h.notifier.actions)
	}
}

func TestAFutureProductChangeIsTakenOnItsInvoice(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	sub := subscribe(t, h, Input{StartDate: ptrTo("2026-11-12")})
	if _, err := h.svc.Update(ctx, adminID, sub.ID, owner, Changes{Amount: ptrTo("3"), Discount: ptrTo("10,5"), StartDate: ptrTo("2026-12-12")}); err != nil {
		t.Fatal(err)
	}
	if r := h.sales.recurring[*sub.RecurringID]; r.Lines[0].Amount != "1 x" {
		t.Errorf("changed before its date: %+v", r.Lines[0])
	}
	*h.clock = time.Date(2026, 11, 12, 9, 0, 0, 0, time.UTC)
	first := h.sales.generate(*sub.RecurringID)
	if inv, _ := h.svc.Generated(ctx, adminID, owner, first, h.sales.recurring[*sub.RecurringID]); inv.Lines[0].Amount != "1 x" {
		t.Errorf("first = %+v", inv.Lines[0])
	}
	second := h.sales.generate(*sub.RecurringID)
	inv, err := h.svc.Generated(ctx, adminID, owner, second, h.sales.recurring[*sub.RecurringID])
	if err != nil {
		t.Fatal(err)
	}
	if inv.Lines[0].Amount != "3 x" || inv.Discount != 1050 || len(inv.Lines) != 1 {
		t.Errorf("second = %+v", inv)
	}
	if r := h.sales.recurring[*sub.RecurringID]; r.Lines[0].Amount != "3 x" || r.Discount != 1050 {
		t.Errorf("recurring after = %+v", r)
	}
}

func TestCancel(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	sub := subscribe(t, h, Input{StartDate: ptrTo("2026-11-12")})
	got, err := h.svc.Cancel(ctx, adminID, sub.ID, owner, ptrTo("2026-11-13"))
	if err != nil {
		t.Fatal(err)
	}
	if got.CancelledAt == nil || got.EndDate.Format(time.DateOnly) != "2026-11-13" || got.Items[0].EndDate.Format(time.DateOnly) != "2026-11-13" {
		t.Errorf("cancelled = %+v", got)
	}
	if r := h.sales.recurring[*sub.RecurringID]; !r.Active || r.LastDate.Format(time.DateOnly) != "2026-11-12" {
		t.Errorf("recurring = %+v", r)
	}
	if _, err := h.svc.Cancel(ctx, adminID, sub.ID, owner, nil); !errors.Is(err, ErrCancelled) {
		t.Errorf("cancel twice = %v", err)
	}
	if _, err := h.svc.Update(ctx, adminID, sub.ID, owner, Changes{Reference: ptrTo("x")}); !errors.Is(err, ErrCancelled) {
		t.Errorf("update cancelled = %v", err)
	}

	other := subscribe(t, h, Input{StartDate: ptrTo("2026-12-01")})
	if _, err := h.svc.Cancel(ctx, adminID, other.ID, owner, ptrTo("2026-11-01")); fields(err)["end_date"] == nil {
		t.Errorf("end in the past = %v", err)
	}
	if _, err := h.svc.Cancel(ctx, adminID, other.ID, owner, nil); err != nil {
		t.Fatal(err)
	}
	if r := h.sales.recurring[*other.RecurringID]; r.Active {
		t.Error("cancelling today left the recurring invoice running")
	}
}

func TestSubscriptionChargesGoOnTheNextInvoice(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	sub := subscribe(t, h, Input{})
	o := Owner{SubscriptionID: &sub.ID}
	c, err := h.svc.AddCharge(ctx, adminID, owner, o, ChargeInput{ProductID: monthly, Amount: "5 x", Price: ptrTo("100"), Period: "20210601..20210630", Description: "Transaction Costs"})
	if err != nil {
		t.Fatal(err)
	}
	if c.Period.Monthly() != "202106..202106" || c.Price != 10000 || c.Amount != "5 x" || c.Billed() {
		t.Errorf("charge = %+v", c)
	}
	if _, err := h.svc.AddCharge(ctx, adminID, owner, o, ChargeInput{ProductID: stranger, Amount: "1", Period: "this_month", Description: "x"}); !errors.Is(err, ErrProductNotFound) {
		t.Errorf("unknown product = %v", err)
	}
	_, err = h.svc.AddCharge(ctx, adminID, owner, o, ChargeInput{ProductID: monthly, Amount: "lots", Period: "soon", Description: " "})
	if f := fields(err); f["amount"] == nil || f["period"] == nil || f["description"] == nil {
		t.Errorf("invalid charge = %v", err)
	}
	if _, err := h.svc.AddCharge(ctx, adminID, owner, Owner{SubscriptionID: ptrTo(stranger)}, ChargeInput{}); !errors.Is(err, ErrNotFound) {
		t.Errorf("unknown subscription = %v", err)
	}

	*h.clock = time.Date(2026, 11, 12, 9, 0, 0, 0, time.UTC)
	inv := h.sales.generate(*sub.RecurringID)
	done, err := h.svc.Generated(ctx, adminID, owner, inv, h.sales.recurring[*sub.RecurringID])
	if err != nil {
		t.Fatal(err)
	}
	if len(done.Lines) != 2 || done.Lines[1].Description != "Transaction Costs" || done.Lines[1].Period != "20210601..20210630" || done.Lines[1].Quantity != 5000 {
		t.Errorf("lines = %+v", done.Lines)
	}
	open, _ := h.svc.Charges(ctx, adminID, owner, o, false)
	all, _ := h.svc.Charges(ctx, adminID, owner, o, true)
	if len(open) != 0 || len(all) != 1 || *all[0].DetailID != done.Lines[1].ID || *all[0].InvoiceID != done.ID {
		t.Errorf("open %+v, all %+v", open, all)
	}
}

func TestContactChargesAreBilledAfterTheMonth(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	o := Owner{ContactID: ptrTo(customer)}
	if _, err := h.svc.AddCharge(ctx, adminID, owner, o, ChargeInput{ProductID: oneOff, Amount: "1", Period: "this_month", Description: "Setup"}); err != nil {
		t.Fatal(err)
	}
	if _, err := h.svc.AddCharge(ctx, adminID, owner, Owner{ContactID: ptrTo(stranger)}, ChargeInput{}); err == nil {
		t.Error("charged an unknown contact")
	}
	if err := h.svc.Work(ctx); err != nil {
		t.Fatal(err)
	}
	if len(h.sales.invoices) != 0 {
		t.Fatal("billed before the month was over")
	}
	*h.clock = time.Date(2026, 12, 1, 6, 0, 0, 0, time.UTC)
	if err := h.svc.Work(ctx); err != nil {
		t.Fatal(err)
	}
	if len(h.sales.invoices) != 1 || len(h.sales.sent) != 1 {
		t.Fatalf("invoices = %+v", h.sales.invoices)
	}
	inv := h.sales.invoices[h.sales.sent[0]]
	if inv.InvoiceDate.Format(time.DateOnly) != "2026-12-01" || inv.State != sales.Open || inv.Lines[0].Price != 5000 || inv.Lines[0].Period != "20261101..20261130" {
		t.Errorf("invoice = %+v", inv)
	}
	if err := h.svc.Work(ctx); err != nil || len(h.sales.invoices) != 1 {
		t.Errorf("billed twice: %v", err)
	}
	all, _ := h.svc.Charges(ctx, adminID, owner, o, true)
	if len(all) != 1 || !all[0].Billed() {
		t.Errorf("charges = %+v", all)
	}
}

func TestScheduleOneOff(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	sub := subscribe(t, h, Input{StartDate: ptrTo("2026-12-01")})
	inv, err := h.svc.ScheduleOneOff(ctx, adminID, sub.ID, owner, []sales.DetailInput{{Description: ptrTo("Extra costs subscription"), Price: ptrTo("10.5")}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if inv.State != sales.Scheduled || inv.InvoiceDate.Format(time.DateOnly) != "2026-12-01" || len(h.store.oneOffs[sub.ID]) != 1 {
		t.Errorf("one-off = %+v", inv)
	}
	if _, err := h.svc.ScheduleOneOff(ctx, adminID, sub.ID, owner, nil, nil); fields(err)["details_attributes"] == nil {
		t.Errorf("without lines = %v", err)
	}
	if _, err := h.svc.ScheduleOneOff(ctx, adminID, stranger, owner, nil, nil); !errors.Is(err, ErrNotFound) {
		t.Errorf("unknown = %v", err)
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
