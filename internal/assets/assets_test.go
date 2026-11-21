package assets

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/adirkuhn/moneyharpy/internal/administration"
	"github.com/adirkuhn/moneyharpy/internal/journal"
	"github.com/adirkuhn/moneyharpy/internal/money"
	"github.com/adirkuhn/moneyharpy/internal/resource"
)

var ctx = context.Background()

func ptr[T any](v T) *T { return &v }

func amount(s string) money.Amount {
	a, err := money.ParseAmount(s)
	if err != nil {
		panic(err)
	}
	return a
}

func input(name, date, value string, years float64, residual string) Input {
	in := Input{Name: ptr(name), LedgerAccountID: ptr(equipment), PurchaseDate: ptr(day(date)), PurchaseValue: ptr(amount(value))}
	if years > 0 {
		in.Plan = &PlanInput{LifespanYears: &years, ResidualValue: ptr(amount(residual))}
	}
	return in
}

func fields(t *testing.T, err error) map[string][]string {
	t.Helper()
	var verr *resource.ValidationError
	if !errors.As(err, &verr) {
		t.Fatalf("error = %v, want a validation error", err)
	}
	return verr.Fields
}

func (h *harness) create(t *testing.T, in Input) Asset {
	t.Helper()
	a, err := h.svc.Create(ctx, adminID, owner, in)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	return a
}

func sumOn(entries []journal.Entry, ledger resource.ID) money.Amount {
	var sum money.Amount
	for _, e := range entries {
		if e.LedgerAccountID == ledger {
			sum += e.Amount
		}
	}
	return sum
}

func TestCreateValidatesAndPlans(t *testing.T) {
	h := newHarness()
	a := h.create(t, input("Laptop", "2026-11-11", "1000", 5, "100"))
	if a.MonthlyDepreciation() != amount("15") || a.CurrentValue() != amount("1000") || !a.PlanActive() || a.Plan.LifespanYears() != 5 {
		t.Errorf("created = %+v, monthly %s", a, a.MonthlyDepreciation())
	}
	if len(h.store.entries[a.ID]) != 0 {
		t.Error("creating an asset booked something")
	}
	if _, err := h.svc.Create(ctx, adminID, stranger, input("x", "2026-01-01", "1", 1, "0")); !errors.Is(err, administration.ErrNotFound) {
		t.Errorf("stranger: %v", err)
	}

	terrain := input("Plot", "2026-01-01", "50000", 0, "")
	terrain.LedgerAccountID = ptr(land)
	if got := h.create(t, terrain); got.Plan != nil || got.MonthlyDepreciation() != 0 {
		t.Errorf("land = %+v", got)
	}
	cases := map[string]struct {
		in    Input
		field string
	}{
		"plan on land": {func() Input {
			in := input("Plot", "2026-01-01", "1", 5, "0")
			in.LedgerAccountID = ptr(land)
			return in
		}(), "value_change_plan.base"},
		"no plan":        {input("Laptop", "2026-01-01", "1000", 0, ""), "value_change_plan"},
		"sub-category":   {func() Input { in := input("x", "2026-01-01", "1", 5, "0"); in.LedgerAccountID = ptr(child); return in }(), "ledger_account"},
		"costs category": {func() Input { in := input("x", "2026-01-01", "1", 5, "0"); in.LedgerAccountID = ptr(costs); return in }(), "ledger_account"},
		"unknown category": {func() Input {
			in := input("x", "2026-01-01", "1", 5, "0")
			in.LedgerAccountID = ptr(resource.ID(9))
			return in
		}(), "ledger_account"},
		"blank name":    {input(" ", "2026-01-01", "1", 5, "0"), "name"},
		"residual high": {input("x", "2026-01-01", "10", 5, "20"), "value_change_plan.residual_value"},
		"no lifespan":   {input("x", "2026-01-01", "10", 0.01, "0"), "value_change_plan.lifespan_in_years"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := h.svc.Create(ctx, adminID, owner, c.in)
			if fields(t, err)[c.field] == nil {
				t.Errorf("fields = %v, want %s", fields(t, err), c.field)
			}
		})
	}
}

func TestImportKeepsTheValueChangesAsTheSourceBookedThem(t *testing.T) {
	// Moneybird booked the first month pro rata and every month since; they
	// come in as they are, booked, and the asset goes on from there.
	h := newHarness()
	in := ImportedAsset{
		Origin: resource.Origin{ID: 4801},
		Input:  input("Car", "2025-01-14", "20003.60", 6, "5000"),
		Sources: []resource.ID{purchaseLine, 999},
		ValueChanges: []ValueChange{
			{Type: Linear, Date: day("2025-01-31"), Amount: amount("-120.99")},
			{Type: Linear, Date: day("2025-02-28"), Amount: amount("-208.38")},
		},
	}
	a, err := h.svc.Import(ctx, adminID, owner, in)
	if err != nil {
		t.Fatal(err)
	}
	if a.ID != 4801 || len(a.ValueChanges) != 2 || a.CurrentValue() != amount("19674.23") || len(a.Sources) != 1 || *a.Sources[0].DetailID != purchaseLine {
		t.Fatalf("asset = %+v", a)
	}
	var booked money.Amount
	for _, e := range h.store.entries[a.ID] {
		if e.LedgerAccountID == depreciation {
			booked += e.Amount
		}
	}
	if booked != amount("329.37") {
		t.Errorf("depreciation booked %s, entries %+v", booked, h.store.entries[a.ID])
	}
	// Again changes nothing.
	if again, err := h.svc.Import(ctx, adminID, owner, in); err != nil || len(again.ValueChanges) != 2 {
		t.Errorf("again: %+v, %v", again, err)
	}
}

func TestUpdateOnlyNameOnceItHasValueChanges(t *testing.T) {
	h := newHarness()
	a := h.create(t, input("Laptop", "2026-11-11", "1000", 5, "100"))
	got, err := h.svc.Update(ctx, adminID, a.ID, owner, Input{PurchaseValue: ptr(amount("10000")), Plan: &PlanInput{LifespanYears: ptr(10.0), ResidualValue: ptr(amount("500"))}})
	if err != nil || got.MonthlyDepreciation() != amount("79.17") {
		t.Fatalf("planned update = %+v, %v", got, err)
	}
	if _, err := h.svc.AddValueChange(ctx, adminID, a.ID, owner, Manual, ChangeInput{Date: day("2026-11-11"), Amount: -1000, Description: "Scratch"}); err != nil {
		t.Fatal(err)
	}
	if _, err := h.svc.Update(ctx, adminID, a.ID, owner, Input{PurchaseValue: ptr(amount("9"))}); fields(t, err)["base"] == nil {
		t.Errorf("value change after changes: %v", err)
	}
	if got, err := h.svc.Update(ctx, adminID, a.ID, owner, Input{Name: ptr("New Name"), PurchaseValue: ptr(amount("10000"))}); err != nil || got.Name != "New Name" {
		t.Errorf("rename = %+v, %v", got, err)
	}
	if _, err := h.svc.Update(ctx, adminID, 1, owner, Input{Name: ptr("x")}); !errors.Is(err, ErrNotFound) {
		t.Errorf("unknown asset: %v", err)
	}
}

func TestArbitraryChangesRestartThePlan(t *testing.T) {
	h := newHarness()
	a := h.create(t, input("Asset 1", "2026-06-01", "12345.67", 5, "10"))
	if a.MonthlyDepreciation() != amount("205.59") {
		t.Fatalf("monthly = %s", a.MonthlyDepreciation())
	}
	if err := h.svc.AddRetroactiveLinear(ctx, adminID, a.ID, owner); err != nil {
		t.Fatal(err)
	}
	a, _ = h.store.Asset(ctx, adminID, a.ID)
	if len(a.ValueChanges) != 5 || a.ValueChanges[0].Date != day("2026-06-30") || a.ValueChanges[4].Date != day("2026-10-31") ||
		a.CurrentValue() != amount("12345.67")-5*amount("205.59") {
		t.Fatalf("linear = %+v", a.ValueChanges)
	}
	entries := h.store.entries[a.ID]
	if sumOn(entries, depreciation) != 5*amount("205.59") || sumOn(entries, accumulated) != -5*amount("205.59") {
		t.Errorf("entries = %+v", entries)
	}
	if err := h.svc.AddRetroactiveLinear(ctx, adminID, a.ID, owner); fields(t, err)["base"] == nil {
		t.Errorf("nothing missing: %v", err)
	}

	for i, d := range []string{"2026-07-01", "2026-08-01", "2026-09-01", "2026-10-01"} {
		c, err := h.svc.AddValueChange(ctx, adminID, a.ID, owner, Arbitrary, ChangeInput{Date: day(d), Amount: -5000, Description: "Value change"})
		if err != nil || c.Type != Arbitrary || c.Amount != -5000 {
			t.Fatalf("arbitrary %d = %+v, %v", i, c, err)
		}
	}
	a, _ = h.store.Asset(ctx, adminID, a.ID)
	// The linear changes after each arbitrary one are gone: June's stays.
	if len(a.ValueChanges) != 5 || a.ValueChanges[0].Type != Linear {
		t.Fatalf("changes = %+v", a.ValueChanges)
	}
	destroyed := slices.DeleteFunc(slices.Clone(h.notifier.got), func(n notice) bool { return n.action != "company_assets_value_changes_linear_destroyed" })
	if len(destroyed) != 4 {
		t.Errorf("destroyed notices = %+v", destroyed)
	}
	// What is left over the residual value is spread over the 56 months
	// from October.
	left := amount("12345.67") - amount("205.59") - 4*amount("50") - amount("10")
	if want := money.Amount((int64(left)*2 + 56) / 112); a.MonthlyDepreciation() != want {
		t.Errorf("monthly = %s, want %s", a.MonthlyDepreciation(), want)
	}
	if err := h.svc.AddRetroactiveLinear(ctx, adminID, a.ID, owner); err != nil {
		t.Fatal(err)
	}
	a, _ = h.store.Asset(ctx, adminID, a.ID)
	if last := a.ValueChanges[len(a.ValueChanges)-1]; last.Type != Linear || last.Date != day("2026-10-31") {
		t.Errorf("after restart = %+v", a.ValueChanges)
	}
}

func TestPlanEndsAtResidualValue(t *testing.T) {
	h := newHarness()
	a := h.create(t, input("Phone", "2026-01-15", "100", 0.25, "0"))
	if err := h.svc.AddRetroactiveLinear(ctx, adminID, a.ID, owner); err != nil {
		t.Fatal(err)
	}
	a, _ = h.store.Asset(ctx, adminID, a.ID)
	var amounts []string
	for _, c := range a.ValueChanges {
		amounts = append(amounts, c.Date.Format(time.DateOnly)+" "+c.Amount.String())
	}
	// As Moneybird's: the first month pro rata by its days in use, the rest in the month after the lifespan.
	want := []string{"2026-01-31 -18.28", "2026-02-28 -33.33", "2026-03-31 -33.33", "2026-04-30 -15.06"}
	if !slices.Equal(amounts, want) || a.PlanActive() {
		t.Errorf("changes = %v, active %v", amounts, a.PlanActive())
	}
	if err := h.svc.AddRetroactiveLinear(ctx, adminID, a.ID, owner); fields(t, err)["base"] == nil {
		t.Errorf("inactive plan: %v", err)
	}
	terrain := input("Plot", "2026-01-01", "50000", 0, "")
	terrain.LedgerAccountID = ptr(land)
	plot := h.create(t, terrain)
	if err := h.svc.AddRetroactiveLinear(ctx, adminID, plot.ID, owner); fields(t, err)["base"][0] != "Asset has no value change plan" {
		t.Errorf("no plan: %v", err)
	}
}

// runPlan books the plan's months one by one, as the worker does, and returns them.
func runPlan(a Asset) []string {
	var out []string
	for _, d := range a.Schedule(day("2100-01-01")) {
		amount, ok := a.linearOn(d)
		if !ok {
			continue
		}
		a.addChange(ValueChange{Type: Linear, Date: d, Amount: amount})
		out = append(out, d.Format(time.DateOnly)+" "+amount.String())
	}
	return out
}

func TestFirstMonthIsProRata(t *testing.T) {
	// Moneybird's example: commissioned on 14 January 2025, 15,003.60 over 6 years.
	car := Asset{PurchaseDate: day("2025-01-14"), PurchaseValue: amount("20003.60"), Plan: &Plan{LifespanMonths: 72, ResidualValue: amount("5000")}}
	got := runPlan(car)
	if car.MonthlyDepreciation() != amount("208.38") || len(got) != 73 {
		t.Fatalf("monthly %s, %d months", car.MonthlyDepreciation(), len(got))
	}
	if got[0] != "2025-01-31 -120.99" || got[1] != "2025-02-28 -208.38" || got[72] != "2031-01-31 -87.63" {
		t.Errorf("first %s, second %s, last %s", got[0], got[1], got[72])
	}
	// Bought on the first of a month, the plan takes exactly its lifespan.
	desk := Asset{PurchaseDate: day("2026-06-01"), PurchaseValue: amount("1200"), Plan: &Plan{LifespanMonths: 12}}
	if got := runPlan(desk); len(got) != 12 || got[0] != "2026-06-30 -100.0" || got[11] != "2027-05-31 -100.0" {
		t.Errorf("desk = %v", got)
	}
}

func TestValueChangeChecks(t *testing.T) {
	h := newHarness()
	a := h.create(t, input("Laptop", "2026-06-01", "1000", 5, "100"))
	cases := map[string]struct {
		in    ChangeInput
		field string
	}{
		"before purchase": {ChangeInput{Date: day("2026-05-31"), Amount: -1, Description: "x"}, "date"},
		"future":          {ChangeInput{Date: day("2026-11-12"), Amount: -1, Description: "x"}, "date"},
		"too large":       {ChangeInput{Date: day("2026-07-01"), Amount: -100001, Description: "x"}, "amount"},
		"no description":  {ChangeInput{Date: day("2026-07-01"), Amount: -1}, "description"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := h.svc.AddValueChange(ctx, adminID, a.ID, owner, Manual, c.in)
			if fields(t, err)[c.field] == nil {
				t.Errorf("fields = %v, want %s", fields(t, err), c.field)
			}
		})
	}
	if c, err := h.svc.AddValueChange(ctx, adminID, a.ID, owner, Manual, ChangeInput{Date: day("2026-07-01"), Amount: 500, Description: "Upgrade", ExternallyBooked: true}); err != nil || !c.ExternallyBooked {
		t.Fatalf("increase = %+v, %v", c, err)
	}
	if len(h.store.entries[a.ID]) != 0 {
		t.Error("externally booked change was booked")
	}
	h.ledger.locked = "2026-07-31"
	if _, err := h.svc.AddValueChange(ctx, adminID, a.ID, owner, Manual, ChangeInput{Date: day("2026-07-15"), Amount: -1, Description: "x"}); fields(t, err)["date"] == nil {
		t.Errorf("locked: %v", err)
	}
}

func TestDivestmentDisposesAndBooksTheBookResult(t *testing.T) {
	h := newHarness()
	a := h.create(t, input("Car", "2026-01-01", "12000", 5, "0"))
	if _, err := h.svc.AddValueChange(ctx, adminID, a.ID, owner, Arbitrary, ChangeInput{Date: day("2026-03-01"), Amount: -amount("2000"), Description: "Damage"}); err != nil {
		t.Fatal(err)
	}
	if _, err := h.svc.Dispose(ctx, adminID, a.ID, owner, Divestment, day("2026-02-01"), nil); fields(t, err)["date"] == nil {
		t.Errorf("before the arbitrary change: %v", err)
	}
	// The first day of November is depreciated pro rata before the rest is written off.
	c, err := h.svc.Dispose(ctx, adminID, a.ID, owner, Divestment, day("2026-11-01"), nil)
	if err != nil || c.Type != Divestment || c.Amount != amount("-9994.25") || c.Description != nil {
		t.Fatalf("divestment = %+v, %v", c, err)
	}
	a, _ = h.store.Asset(ctx, adminID, a.ID)
	if a.Disposal == nil || a.Disposal.Reason != "divested" || a.CurrentValue() != 0 || a.PlanActive() {
		t.Fatalf("disposed = %+v", a)
	}
	entries := h.store.entries[a.ID]
	if sumOn(entries, bookResult) != amount("9994.25") || sumOn(entries, depreciation) != amount("2005.75") || sumOn(entries, accumulated) != 0 ||
		sumOn(entries, equipment) != amount("-12000") {
		t.Errorf("entries = %+v", entries)
	}
	if _, err := h.svc.AddValueChange(ctx, adminID, a.ID, owner, Manual, ChangeInput{Date: day("2026-11-02"), Amount: -1, Description: "x"}); fields(t, err)["base"] == nil {
		t.Errorf("change after disposal: %v", err)
	}
	if _, err := h.svc.AddDisposal(ctx, adminID, a.ID, owner, day("2026-11-02"), "sold"); fields(t, err)["base"] == nil {
		t.Errorf("second disposal: %v", err)
	}
}

func TestDisposalDepreciatesItsMonthProRata(t *testing.T) {
	// Moneybird's example: the car is depreciated up to October; divested on 18 November.
	h := newHarness()
	*h.clock = time.Date(2026, 11, 18, 9, 0, 0, 0, time.UTC)
	a := h.create(t, input("Car", "2025-01-14", "20003.60", 6, "5000"))
	if err := h.svc.AddRetroactiveLinear(ctx, adminID, a.ID, owner); err != nil {
		t.Fatal(err)
	}
	a, _ = h.store.Asset(ctx, adminID, a.ID)
	if a.CurrentValue() != amount("15506.63") {
		t.Fatalf("value on 31 October = %s", a.CurrentValue())
	}
	if got := a.ValueOn(day("2026-11-18")); got != amount("15381.60") {
		t.Errorf("value on 18 November = %s, want 15381.60", got)
	}
	c, err := h.svc.Dispose(ctx, adminID, a.ID, owner, Divestment, day("2026-11-18"), nil)
	if err != nil || c.Amount != amount("-15381.60") {
		t.Fatalf("divestment = %+v, %v", c, err)
	}
	a, _ = h.store.Asset(ctx, adminID, a.ID)
	n := len(a.ValueChanges)
	if p := a.ValueChanges[n-2]; p.Type != Linear || p.Date != day("2026-11-18") || p.Amount != amount("-125.03") {
		t.Errorf("partial month = %+v", p)
	}

	// On a month end, the month is whole and nothing extra is booked.
	h2 := newHarness()
	*h2.clock = time.Date(2026, 11, 18, 9, 0, 0, 0, time.UTC)
	b := h2.create(t, input("Desk", "2026-06-01", "1200", 1, "0"))
	if got := b.ValueOn(day("2026-06-30")); got != amount("1100") {
		t.Errorf("desk on 30 June = %s", got)
	}
	if _, err := h2.svc.Dispose(ctx, adminID, b.ID, owner, FullDepreciation, day("2026-06-30"), ptr("Broken")); err != nil {
		t.Fatal(err)
	}
	b, _ = h2.store.Asset(ctx, adminID, b.ID)
	if len(b.ValueChanges) != 2 || b.ValueChanges[0].Amount != amount("-100") || b.ValueChanges[1].Amount != amount("-1100") {
		t.Errorf("desk changes = %+v", b.ValueChanges)
	}
}

func TestFullDepreciationAndDisposal(t *testing.T) {
	h := newHarness()
	a := h.create(t, input("Desk", "2020-11-11", "12345.67", 5, "10"))
	if _, err := h.svc.AddDisposal(ctx, adminID, a.ID, owner, day("2026-11-11"), "out_of_use"); fields(t, err)["date"][0] != "is invalid, asset is not fully depreciated on this date" {
		t.Errorf("not depreciated: %v", err)
	}
	if _, err := h.svc.Dispose(ctx, adminID, a.ID, owner, FullDepreciation, day("2026-11-11"), nil); fields(t, err)["description"] == nil {
		t.Errorf("no description: %v", err)
	}
	if _, err := h.svc.AddValueChange(ctx, adminID, a.ID, owner, Manual, ChangeInput{Date: day("2026-11-11"), Amount: amount("-12345.67"), Description: "Depreciation"}); err != nil {
		t.Fatal(err)
	}
	d, err := h.svc.AddDisposal(ctx, adminID, a.ID, owner, day("2026-11-11"), "out_of_use")
	if err != nil || d.Reason != "out_of_use" {
		t.Fatalf("disposal = %+v, %v", d, err)
	}
	entries := h.store.entries[a.ID]
	if sumOn(entries, accumulated) != 0 || sumOn(entries, equipment) != amount("-12345.67") || sumOn(entries, depreciation) != amount("12345.67") {
		t.Errorf("entries = %+v", entries)
	}

	b := h.create(t, input("Chair", "2026-01-01", "500", 5, "50"))
	c, err := h.svc.Dispose(ctx, adminID, b.ID, owner, FullDepreciation, day("2026-11-11"), ptr("Broken"))
	// 11 days of November are depreciated first, 7.50 a month pro rata.
	if err != nil || c.Amount != amount("-497.25") {
		t.Fatalf("full depreciation = %+v, %v", c, err)
	}
	b, _ = h.store.Asset(ctx, adminID, b.ID)
	if b.Disposal == nil || b.Disposal.Reason != "out_of_use" || sumOn(h.store.entries[b.ID], depreciation) != amount("500") {
		t.Errorf("fully depreciated = %+v", b)
	}
}

func TestReinvestmentReserve(t *testing.T) {
	h := newHarness()
	old := h.create(t, input("Old van", "2026-01-01", "1000", 5, "0"))
	if _, err := h.svc.AddReserveSale(ctx, adminID, old.ID, owner, amount("300")); fields(t, err)["base"][0] != "Asset has no disposal" {
		t.Errorf("no disposal: %v", err)
	}
	if _, err := h.svc.Dispose(ctx, adminID, old.ID, owner, Divestment, day("2026-10-01"), nil); err != nil {
		t.Fatal(err)
	}
	sale, err := h.svc.AddReserveSale(ctx, adminID, old.ID, owner, amount("300"))
	if err != nil || sale.Amount != amount("300") {
		t.Fatalf("sale = %+v, %v", sale, err)
	}
	// The divestment's book result is the value after the first day of October's depreciation.
	if e := h.store.entries[old.ID]; sumOn(e, reserve) != amount("-300") || sumOn(e, bookResult) != amount("1299.46") {
		t.Errorf("sale entries = %+v", e)
	}

	van := h.create(t, input("New van", "2026-11-01", "1000", 5, "100"))
	if _, err := h.svc.AddReservePurchase(ctx, adminID, van.ID, owner, amount("950")); len(fields(t, err)["amount"]) != 2 {
		t.Errorf("too much: %v", err)
	}
	r, err := h.svc.AddReservePurchase(ctx, adminID, van.ID, owner, amount("300"))
	if err != nil {
		t.Fatal(err)
	}
	van, _ = h.store.Asset(ctx, adminID, van.ID)
	if van.CalculatedPurchaseValue() != amount("700") || van.ReservePurchase.ID != r.ID || van.MonthlyDepreciation() != amount("10") {
		t.Errorf("van = %+v, monthly %s", van, van.MonthlyDepreciation())
	}
	if e := h.store.entries[van.ID]; sumOn(e, reserve) != amount("300") || sumOn(e, equipment) != amount("-300") {
		t.Errorf("purchase entries = %+v", e)
	}
	if _, err := h.svc.AddReservePurchase(ctx, adminID, van.ID, owner, amount("1")); fields(t, err)["base"] == nil {
		t.Errorf("second purchase: %v", err)
	}
	if err := h.svc.DeleteReservePurchase(ctx, adminID, van.ID, owner); err != nil {
		t.Fatal(err)
	}
	if err := h.svc.DeleteReservePurchase(ctx, adminID, van.ID, owner); !errors.Is(err, ErrReserveNotFound) {
		t.Errorf("deleted twice: %v", err)
	}
	if err := h.svc.DeleteReserveSale(ctx, adminID, old.ID, owner); err != nil {
		t.Fatal(err)
	}
	if err := h.svc.DeleteReserveSale(ctx, adminID, van.ID, owner); !IsNotFound(err) {
		t.Errorf("no disposal: %v", err)
	}
}

func TestSources(t *testing.T) {
	h := newHarness()
	a := h.create(t, input("Laptop", "2026-11-01", "1000", 5, "0"))
	src, err := h.svc.AddSource(ctx, adminID, a.ID, owner, SourceInput{DetailID: ptr(purchaseLine)})
	if err != nil || *src.DetailID != purchaseLine {
		t.Fatalf("detail source = %+v, %v", src, err)
	}
	if _, err := h.svc.AddSource(ctx, adminID, a.ID, owner, SourceInput{EntryID: ptr(journalRow)}); err != nil {
		t.Fatal(err)
	}
	for name, in := range map[string]SourceInput{
		"other category": {DetailID: ptr(otherLine)}, "unknown": {DetailID: ptr(resource.ID(1))}, "twice": {DetailID: ptr(purchaseLine)}, "none": {},
	} {
		if _, err := h.svc.AddSource(ctx, adminID, a.ID, owner, in); fields(t, err)["booking"] == nil {
			t.Errorf("%s: %v", name, err)
		}
	}
	if err := h.svc.DeleteSource(ctx, adminID, a.ID, src.ID, owner); err != nil {
		t.Fatal(err)
	}
	if err := h.svc.DeleteSource(ctx, adminID, a.ID, src.ID, owner); !errors.Is(err, ErrSourceNotFound) {
		t.Errorf("deleted twice: %v", err)
	}
	if a, _ = h.store.Asset(ctx, adminID, a.ID); len(a.Sources) != 1 || a.Sources[0].EntryID == nil {
		t.Errorf("sources = %+v", a.Sources)
	}
}

func TestLockedPeriodKeepsBookings(t *testing.T) {
	h := newHarness()
	a := h.create(t, input("Laptop", "2026-01-01", "1200", 1, "0"))
	if err := h.svc.AddRetroactiveLinear(ctx, adminID, a.ID, owner); err != nil {
		t.Fatal(err)
	}
	h.ledger.locked = "2026-03-31"
	if err := h.svc.Delete(ctx, adminID, a.ID, owner); fields(t, err)["base"] == nil {
		t.Errorf("delete in locked period: %v", err)
	}
	if _, err := h.svc.AddValueChange(ctx, adminID, a.ID, owner, Arbitrary, ChangeInput{Date: day("2026-04-01"), Amount: -10, Description: "x"}); err != nil {
		t.Errorf("after the locked period: %v", err)
	}
	if _, err := h.svc.Update(ctx, adminID, a.ID, owner, Input{Name: ptr("Laptop 2")}); err != nil {
		t.Errorf("rename: %v", err)
	}
	h.ledger.locked = ""
	if err := h.svc.Delete(ctx, adminID, a.ID, owner); err != nil {
		t.Fatal(err)
	}
	if _, ok := h.store.entries[a.ID]; ok {
		t.Error("deleted asset keeps entries")
	}
}

func TestWorkBooksDueMonthsOnce(t *testing.T) {
	h := newHarness()
	old := h.create(t, input("Desk", "2026-06-01", "1200", 1, "0"))
	fresh := h.create(t, input("Chair", "2026-11-01", "120", 1, "0"))
	if err := h.svc.Work(ctx); err != nil {
		t.Fatal(err)
	}
	if a, _ := h.store.Asset(ctx, adminID, old.ID); len(a.ValueChanges) != 0 {
		t.Errorf("months before the asset was added were booked: %+v", a.ValueChanges)
	}
	*h.clock = time.Date(2026, 12, 2, 9, 0, 0, 0, time.UTC)
	for range 2 {
		if err := h.svc.Work(ctx); err != nil {
			t.Fatal(err)
		}
	}
	old, _ = h.store.Asset(ctx, adminID, old.ID)
	fresh, _ = h.store.Asset(ctx, adminID, fresh.ID)
	if len(old.ValueChanges) != 1 || old.ValueChanges[0].Date != day("2026-11-30") || old.ValueChanges[0].Amount != amount("-100") {
		t.Errorf("old = %+v", old.ValueChanges)
	}
	if len(fresh.ValueChanges) != 1 || fresh.ValueChanges[0].Amount != amount("-10") {
		t.Errorf("fresh = %+v", fresh.ValueChanges)
	}
	created := slices.DeleteFunc(slices.Clone(h.notifier.got), func(n notice) bool { return n.action != "company_assets_value_changes_linear_created" })
	if len(created) != 2 || h.store.locks != 3 {
		t.Errorf("notices = %+v, locks %d", created, h.store.locks)
	}
}

func TestReport(t *testing.T) {
	h := newHarness()
	a := h.create(t, input("Test Asset", "2025-01-01", "12345.67", 5, "10"))
	if _, err := h.svc.AddValueChange(ctx, adminID, a.ID, owner, Arbitrary, ChangeInput{Date: day("2025-06-01"), Amount: -20000, Description: "x"}); err != nil {
		t.Fatal(err)
	}
	later := h.create(t, input("Later", "2026-11-01", "500", 5, "0"))
	lines, err := h.svc.Report(ctx, adminID, owner, "202501..202512")
	if err != nil || len(lines) != 2 {
		t.Fatalf("report = %+v, %v", lines, err)
	}
	l := lines[0]
	if l.Asset.ID != a.ID || l.ValueAtBegin != 0 || l.Investment != amount("12345.67") || l.Depreciation != -20000 || l.ValueChanges != -20000 ||
		l.ValueAtEnd != amount("12145.67") || l.TotalSincePurchase != -20000 || *l.LifespanSeconds != 157784760 || l.ResidualValue != 1000 {
		t.Errorf("full year = %+v", l)
	}
	if l := lines[1]; l.Asset.ID != later.ID || l.NotInUse != amount("500") || l.ValueAtEnd != 0 {
		t.Errorf("not in use = %+v", l)
	}
	lines, err = h.svc.Report(ctx, adminID, owner, "202601..202612")
	if err != nil || lines[0].ValueAtBegin != amount("12145.67") || lines[0].Investment != 0 || lines[0].ValueAtEnd != amount("12145.67") {
		t.Errorf("next year = %+v, %v", lines, err)
	}
	if _, err := h.svc.Report(ctx, adminID, owner, "202501..202612"); !errors.Is(err, ErrLongPeriod) {
		t.Errorf("long period: %v", err)
	}
	if _, err := h.svc.Report(ctx, adminID, owner, "soon"); !errors.Is(err, ErrPeriod) {
		t.Errorf("bad period: %v", err)
	}
}

func TestLinearDepreciationIsNamedInTheAdministrationsLanguage(t *testing.T) {
	for language, want := range map[string]string{"nl": "Lineaire afschrijving", "nl-NL": "Lineaire afschrijving", "en": "Linear depreciation", "": "Linear depreciation"} {
		if got := linearDescription(language); got != want {
			t.Errorf("linearDescription(%q) = %q, want %q", language, got, want)
		}
	}
}
