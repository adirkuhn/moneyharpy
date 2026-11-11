package postgres_test

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/adirkuhn/moneyharpy/internal/administration"
	"github.com/adirkuhn/moneyharpy/internal/assets"
	"github.com/adirkuhn/moneyharpy/internal/money"
	"github.com/adirkuhn/moneyharpy/internal/postgres"
	"github.com/adirkuhn/moneyharpy/internal/purchases"
	"github.com/adirkuhn/moneyharpy/internal/resource"
	"github.com/adirkuhn/moneyharpy/internal/sales"
)

func assetBooked(t *testing.T, admin, ledger resource.ID) money.Amount {
	t.Helper()
	var sum int64
	if err := testPool.QueryRow(context.Background(),
		"SELECT coalesce(sum(amount), 0)::bigint FROM journal_entries WHERE administration_id = $1 AND document_type = 'Asset' AND ledger_account_id = $2",
		int64(admin), int64(ledger)).Scan(&sum); err != nil {
		t.Fatal(err)
	}
	return money.Amount(sum)
}

func TestAssetsAreStoredAndBooked(t *testing.T) {
	ctx := context.Background()
	f := newPurchaseFixture(t)
	store := postgres.NewAssetStore(testPool)
	admins := administration.NewService(postgres.NewAdministrationStore(testPool), ids, time.Now)
	svc := assets.NewService(admins, f.books, store, nil, ids, time.Now)
	equipment := categoryByKey(t, f.books, f.admin, f.owner, "equipment")

	years, residual, value := 1.0, money.Amount(0), money.Amount(120000)
	purchase := time.Date(time.Now().Year()-1, 1, 1, 0, 0, 0, 0, time.UTC)
	a, err := svc.Create(ctx, f.admin, f.owner, assets.Input{
		Name: ptr("Laptop"), LedgerAccountID: &equipment, PurchaseDate: &purchase, PurchaseValue: &value,
		Plan: &assets.PlanInput{LifespanYears: &years, ResidualValue: &residual},
	})
	if err != nil {
		t.Fatal(err)
	}
	d, err := f.docs.Create(ctx, f.admin, f.owner, purchases.PurchaseInvoice, purchases.Changes{
		ContactID: &f.supplier, Reference: ptr("LAPTOP"), Date: ptr(purchase.Format(time.DateOnly)),
		Details: []sales.DetailInput{{Description: ptr("Laptop"), Price: ptr("1200"), TaxRateID: &f.vat21, LedgerAccountID: &equipment}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.AddSource(ctx, f.admin, a.ID, f.owner, assets.SourceInput{DetailID: &d.Lines[0].ID}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.AddSource(ctx, f.admin, a.ID, f.owner, assets.SourceInput{EntryID: ptr(resource.ID(1))}); err == nil {
		t.Error("unknown journal entry row accepted")
	}
	work, err := store.Workload(ctx)
	if err != nil || !slices.Contains(work, assets.Workload{AdministrationID: f.admin, UserID: f.owner}) {
		t.Errorf("workload = %v, %v", work, err)
	}
	if err := svc.AddRetroactiveLinear(ctx, f.admin, a.ID, f.owner); err != nil {
		t.Fatal(err)
	}
	got, err := store.Asset(ctx, f.admin, a.ID)
	if err != nil || len(got.ValueChanges) != 12 || len(got.Sources) != 1 || got.Plan.LifespanMonths != 12 || got.CurrentValue() != 0 || got.Name != "Laptop" {
		t.Fatalf("stored = %+v, %v", got, err)
	}
	depreciation := categoryByKey(t, f.books, f.admin, f.owner, "depreciation")
	accumulated := categoryByKey(t, f.books, f.admin, f.owner, "equipment_depreciation_total")
	if assetBooked(t, f.admin, depreciation) != 120000 || assetBooked(t, f.admin, accumulated) != -120000 {
		t.Errorf("booked %s and %s", assetBooked(t, f.admin, depreciation), assetBooked(t, f.admin, accumulated))
	}

	if _, err := svc.AddDisposal(ctx, f.admin, a.ID, f.owner, purchase.AddDate(1, 0, 0), "sold"); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.AddReserveSale(ctx, f.admin, a.ID, f.owner, 5000); err != nil {
		t.Fatal(err)
	}
	if bal, err := store.ReserveBalance(ctx, f.admin); err != nil || bal != 5000 {
		t.Errorf("reserve = %s, %v", bal, err)
	}
	if assetBooked(t, f.admin, accumulated) != 0 || assetBooked(t, f.admin, equipment) != -120000 {
		t.Errorf("disposal booked %s on accumulated", assetBooked(t, f.admin, accumulated))
	}
	got, _ = store.Asset(ctx, f.admin, a.ID)
	if got.Disposal == nil || got.Disposal.Reason != "sold" || got.Disposal.ReserveSale == nil {
		t.Errorf("disposal = %+v", got.Disposal)
	}

	list, total, err := store.Assets(ctx, f.admin, assets.Query{OnlyActive: true, Page: 1, PerPage: 10})
	if err != nil || total != 0 || len(list) != 0 {
		t.Errorf("active = %v, %d, %v", list, total, err)
	}
	list, total, err = store.Assets(ctx, f.admin, assets.Query{LedgerAccountID: &equipment, Page: 1, PerPage: 10})
	if err != nil || total != 1 || len(list[0].ValueChanges) != 12 {
		t.Errorf("all = %v, %d, %v", list, total, err)
	}
	release, ok, err := store.Lock(ctx, f.admin)
	if err != nil || !ok {
		t.Fatalf("lock = %v, %v", ok, err)
	}
	if _, again, _ := store.Lock(ctx, f.admin); again {
		t.Error("locked twice")
	}
	release()

	if err := svc.Delete(ctx, f.admin, a.ID, f.owner); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Asset(ctx, f.admin, a.ID); !errors.Is(err, assets.ErrNotFound) {
		t.Errorf("deleted asset: %v", err)
	}
	if assetBooked(t, f.admin, depreciation) != 0 {
		t.Error("deleted asset keeps its bookings")
	}
}
