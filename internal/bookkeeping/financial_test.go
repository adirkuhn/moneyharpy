package bookkeeping

import (
	"context"
	"testing"
)

func TestFinancialLedgerAccount(t *testing.T) {
	svc, store := newTestService("NL")
	ctx := context.Background()
	id, err := svc.FinancialLedgerAccount(ctx, adminID, owner, "Cash", "BankAccount")
	if err != nil {
		t.Fatal(err)
	}
	l := store.accounts[id]
	parent := byKey(t, store, "financial_accounts")
	if l.Name != "Cash (2)" || l.Taxonomy != "BLimBanRba" || l.System != FinancialRole || l.ParentID == nil || *l.ParentID != parent.ID {
		t.Errorf("category = %+v", l)
	}
	if err := svc.CheckBookable(ctx, adminID, owner, id); err != nil {
		t.Errorf("CheckBookable() = %v", err)
	}
}

func TestCheckBookable(t *testing.T) {
	svc, store := newTestService("NL")
	ctx := context.Background()
	if _, err := svc.Settings(ctx, adminID, owner); err != nil {
		t.Fatal(err)
	}
	rent := byKey(t, store, "rent")
	if err := svc.CheckBookable(ctx, adminID, owner, rent.ID); err != nil {
		t.Errorf("rent: %v", err)
	}
	rent.Active = false
	store.accounts[rent.ID] = rent
	if fieldsOf(t, svc.CheckBookable(ctx, adminID, owner, rent.ID))["booking_id"] == nil {
		t.Error("deactivated category is bookable")
	}
	if fieldsOf(t, svc.CheckBookable(ctx, adminID, owner, 1))["booking_id"] == nil {
		t.Error("unknown category is bookable")
	}
	cash, ok, err := svc.SeedLedgerAccount(ctx, adminID, owner, "cash")
	if err != nil || !ok || cash != byKey(t, store, "cash").ID {
		t.Errorf("SeedLedgerAccount() = %d, %v, %v", cash, ok, err)
	}
	if problem, _ := svc.Bookable(ctx, adminID, owner, byKey(t, store, "accounts_receivable").ID, "general_journal_document"); problem == "" {
		t.Error("debtors bookable in a journal entry")
	}
	if problem, _ := svc.Bookable(ctx, adminID, owner, byKey(t, store, "revenue").ID, "purchase_invoice"); problem == "" {
		t.Error("revenue bookable on a purchase invoice")
	}
	if problem, err := svc.Bookable(ctx, adminID, owner, byKey(t, store, "rounding").ID, "general_journal_document"); problem != "" || err != nil {
		t.Errorf("rounding in a journal entry: %q, %v", problem, err)
	}
	receivable, err := svc.SystemLedgerAccount(ctx, adminID, owner, "accounts_receivable")
	if err != nil || receivable != byKey(t, store, "accounts_receivable").ID {
		t.Errorf("SystemLedgerAccount() = %d, %v", receivable, err)
	}
	uncollectible := byKey(t, store, "uncollectible_revenue")
	delete(store.accounts, uncollectible.ID)
	again, err := svc.SystemLedgerAccount(ctx, adminID, owner, "uncollectible_revenue")
	if err != nil || again == uncollectible.ID || store.accounts[again].System != "uncollectible_revenue" || store.accounts[again].ParentID == nil {
		t.Errorf("recreated = %+v, %v", store.accounts[again], err)
	}
	if day, _ := svc.LockedUntil(ctx, adminID, owner); day != "" {
		t.Errorf("LockedUntil() = %q", day)
	}
}
