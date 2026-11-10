package postgres_test

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/adirkuhn/moneyharpy/internal/administration"
	"github.com/adirkuhn/moneyharpy/internal/bookkeeping"
	"github.com/adirkuhn/moneyharpy/internal/postgres"
	"github.com/adirkuhn/moneyharpy/internal/resource"
)

// bookkeepingService runs the bookkeeping use cases on the database, for an
// administration owned by a new user.
func bookkeepingService(t *testing.T) (*bookkeeping.Service, *postgres.BookkeepingStore, resource.ID, resource.ID) {
	t.Helper()
	a, _, owner := newAdministrationWithIdentity(t)
	store := postgres.NewBookkeepingStore(testPool)
	admins := administration.NewService(postgres.NewAdministrationStore(testPool), ids, time.Now)
	return bookkeeping.NewService(admins, store, ids, time.Now), store, a.ID, owner
}

func TestBookkeepingSeedsOnce(t *testing.T) {
	ctx := context.Background()
	svc, store, admin, owner := bookkeepingService(t)
	for range 2 {
		if _, err := svc.Settings(ctx, admin, owner); err != nil {
			t.Fatal(err)
		}
	}
	accounts, err := store.LedgerAccounts(ctx, admin)
	if err != nil {
		t.Fatal(err)
	}
	again, err := store.Seed(ctx, admin, "", nil, nil)
	if err != nil || again {
		t.Errorf("second Seed() = %v, %v, want false", again, err)
	}
	st, _ := store.Settings(ctx, admin)
	if !st.Seeded || st.VATPeriod != "quarter" || len(accounts) == 0 {
		t.Errorf("settings %+v, %d accounts", st, len(accounts))
	}
}

func TestBookkeepingSettingsRoundTrip(t *testing.T) {
	ctx := context.Background()
	svc, store, admin, owner := bookkeepingService(t)
	day := time.Now().AddDate(0, -1, 0).Format(time.DateOnly)
	if _, err := svc.UpdateSettings(ctx, admin, owner, bookkeeping.SettingsChanges{VATPeriod: ptr("month"), ICPPeriod: ptr("year"), LockedUntil: &day}); err != nil {
		t.Fatal(err)
	}
	st, err := store.Settings(ctx, admin)
	if err != nil {
		t.Fatal(err)
	}
	if st.VATPeriod != "month" || st.ICPPeriod != "year" || st.LockedUntil == nil || *st.LockedUntil != day {
		t.Errorf("Settings() = %+v", st)
	}
	a, _ := administration.NewService(postgres.NewAdministrationStore(testPool), ids, time.Now).Get(ctx, admin, owner)
	if a.PeriodLockedUntil == nil || *a.PeriodLockedUntil != day {
		t.Errorf("administration locked until %v, want %s", a.PeriodLockedUntil, day)
	}
}

func TestTaxRatesRoundTrip(t *testing.T) {
	ctx := context.Background()
	svc, store, admin, owner := bookkeepingService(t)
	r, err := svc.CreateTaxRate(ctx, admin, owner, bookkeeping.TaxRateChanges{
		Name: ptr("Reduced"), Percentage: ptr("9.5"), Type: ptr("sales_invoice"), ReportBoxes: ptr([]string{"NL/1b"}),
	})
	if err != nil {
		t.Fatal(err)
	}
	got, err := store.TaxRate(ctx, admin, r.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Percentage != "9.5" || !slices.Equal(got.ReportBoxes, []string{"NL/1b"}) || got.Position != r.Position {
		t.Errorf("TaxRate() = %+v, want %+v", got, r)
	}
	if _, err := svc.UpdateTaxRate(ctx, admin, r.ID, owner, bookkeeping.TaxRateChanges{Percentage: ptr("21"), ReportBoxes: ptr([]string{})}); err != nil {
		t.Fatal(err)
	}
	if got, _ = store.TaxRate(ctx, admin, r.ID); got.Percentage != "21" || len(got.ReportBoxes) != 0 {
		t.Errorf("after update = %+v", got)
	}
	if _, err := store.TaxRate(ctx, admin, 1); !errors.Is(err, bookkeeping.ErrTaxRateNotFound) {
		t.Errorf("TaxRate() unknown error = %v", err)
	}
}

func TestLedgerAccountsRoundTrip(t *testing.T) {
	ctx := context.Background()
	svc, store, admin, owner := bookkeepingService(t)
	added, err := svc.ApplyTemplate(ctx, admin, owner, "company_car")
	if err != nil || len(added) == 0 {
		t.Fatalf("ApplyTemplate() = %d, %v", len(added), err)
	}
	fuel := added[slices.IndexFunc(added, func(l bookkeeping.LedgerAccount) bool { return l.Key == "company_car_fuel" })]
	got, err := store.LedgerAccount(ctx, admin, fuel.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.ParentID == nil || *got.ParentID != *fuel.ParentID || got.Template != "company_car" || got.Taxonomy != fuel.Taxonomy ||
		!slices.Equal(got.AllowedDocumentTypes, fuel.AllowedDocumentTypes) {
		t.Errorf("LedgerAccount() = %+v, want %+v", got, fuel)
	}

	if _, _, err := svc.DeleteLedgerAccount(ctx, admin, fuel.ID, owner); err != nil {
		t.Fatal(err)
	}
	if _, _, err := svc.DeleteLedgerAccount(ctx, admin, fuel.ID, owner); err != nil {
		t.Fatal(err)
	}
	if _, err := store.LedgerAccount(ctx, admin, fuel.ID); !errors.Is(err, bookkeeping.ErrLedgerAccountNotFound) {
		t.Errorf("deleted LedgerAccount() error = %v", err)
	}

	if _, err := svc.UpdateSettings(ctx, admin, owner, bookkeeping.SettingsChanges{Scheme: ptr("")}); err != nil {
		t.Fatal(err)
	}
	all, _ := store.LedgerAccounts(ctx, admin)
	for _, l := range all {
		if l.Taxonomy != "" {
			t.Fatalf("%s kept code %s", l.Name, l.Taxonomy)
		}
	}
}

func ptr[T any](v T) *T { return &v }
