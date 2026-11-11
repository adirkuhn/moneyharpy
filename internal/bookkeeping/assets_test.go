package bookkeeping

import (
	"context"
	"testing"
)

func TestAccumulatedDepreciation(t *testing.T) {
	svc, store := newTestService("NL")
	ctx := context.Background()
	if _, err := svc.Settings(ctx, adminID, owner); err != nil {
		t.Fatal(err)
	}
	equipment := byKey(t, store, "equipment")
	id, err := svc.AccumulatedDepreciation(ctx, adminID, owner, equipment.ID)
	if err != nil || id != byKey(t, store, "equipment_depreciation_total").ID {
		t.Fatalf("seeded = %d, %v", id, err)
	}

	name, kind, code := "Machines", "non_current_assets", "BMvaMei"
	machines, err := svc.CreateLedgerAccount(ctx, adminID, owner, LedgerAccountChanges{Name: &name, Type: &kind, Taxonomy: &code})
	if err != nil {
		t.Fatal(err)
	}
	id, err = svc.AccumulatedDepreciation(ctx, adminID, owner, machines.ID)
	if err != nil {
		t.Fatal(err)
	}
	l := store.accounts[id]
	if l.Name != "Accumulated depreciation Machines" || l.Taxonomy != "BMvaMeiCae" || l.ParentID == nil || *l.ParentID != machines.ID ||
		l.System != AccumulatedDepreciationRole || l.Type != "non_current_assets" {
		t.Errorf("added = %+v", l)
	}
	again, err := svc.AccumulatedDepreciation(ctx, adminID, owner, machines.ID)
	if err != nil || again != id {
		t.Errorf("again = %d, %v", again, err)
	}
	if _, err := svc.AccumulatedDepreciation(ctx, adminID, owner, 1); !IsNotFound(err) {
		t.Errorf("unknown parent: %v", err)
	}
}

func TestSystemLedgerAccountTakesSeededCategoryWithoutRole(t *testing.T) {
	svc, store := newTestService("NL")
	ctx := context.Background()
	if _, err := svc.Settings(ctx, adminID, owner); err != nil {
		t.Fatal(err)
	}
	depreciation := byKey(t, store, "depreciation")
	depreciation.System = ""
	store.accounts[depreciation.ID] = depreciation
	id, err := svc.SystemLedgerAccount(ctx, adminID, owner, "depreciation")
	if err != nil || id != depreciation.ID {
		t.Errorf("SystemLedgerAccount() = %d, %v", id, err)
	}
}
