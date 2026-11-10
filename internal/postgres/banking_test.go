package postgres_test

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/adirkuhn/moneyharpy/internal/administration"
	"github.com/adirkuhn/moneyharpy/internal/banking"
	"github.com/adirkuhn/moneyharpy/internal/bookkeeping"
	"github.com/adirkuhn/moneyharpy/internal/postgres"
	"github.com/adirkuhn/moneyharpy/internal/resource"
)

// bankingService runs the banking use cases on the database.
func bankingService(t *testing.T) (*banking.Service, *bookkeeping.Service, *postgres.BankingStore, resource.ID, resource.ID) {
	t.Helper()
	a, _, owner := newAdministrationWithIdentity(t)
	admins := administration.NewService(postgres.NewAdministrationStore(testPool), ids, time.Now)
	books := bookkeeping.NewService(admins, postgres.NewBookkeepingStore(testPool), ids, time.Now)
	store := postgres.NewBankingStore(testPool)
	return banking.NewService(admins, books, store, ids, time.Now), books, store, a.ID, owner
}

func categoryByKey(t *testing.T, books *bookkeeping.Service, admin, owner resource.ID, key string) resource.ID {
	t.Helper()
	id, ok, err := books.SeedLedgerAccount(context.Background(), admin, owner, key)
	if err != nil || !ok {
		t.Fatalf("no category %s: %v", key, err)
	}
	return id
}

func TestBankingAccountsRoundTrip(t *testing.T) {
	ctx := context.Background()
	svc, _, store, admin, owner := bankingService(t)
	list, err := svc.Accounts(ctx, admin, owner)
	if err != nil || len(list) != 1 || list[0].Type != banking.CashAccount {
		t.Fatalf("seeded accounts = %+v, %v", list, err)
	}
	acc, err := svc.CreateAccount(ctx, admin, owner, banking.AccountChanges{Name: ptr("Main bank"), Identifier: ptr("NL91ABNA0417164300")})
	if err != nil {
		t.Fatal(err)
	}
	got, err := store.Account(ctx, admin, acc.ID)
	if err != nil || got.Identifier != acc.Identifier || got.LedgerAccountID != acc.LedgerAccountID || got.Position != 2 {
		t.Errorf("Account() = %+v, %v", got, err)
	}
	if _, err := store.Account(ctx, admin, 1); !errors.Is(err, banking.ErrAccountNotFound) {
		t.Errorf("unknown error = %v", err)
	}
	if again, _ := store.Seed(ctx, admin, nil); again {
		t.Error("seeded twice")
	}
}

func TestBankingImportBookAndRules(t *testing.T) {
	ctx := context.Background()
	svc, books, store, admin, owner := bankingService(t)
	acc, err := svc.CreateAccount(ctx, admin, owner, banking.AccountChanges{Name: ptr("Main bank"), Identifier: ptr("NL91ABNA0417164300")})
	if err != nil {
		t.Fatal(err)
	}
	software := categoryByKey(t, books, admin, owner, "software")
	if _, err := svc.CreateRule(ctx, admin, owner, banking.RuleChanges{
		Name: ptr("Hosting"), LedgerAccountID: &software, Direction: ptr("debit"),
		Conditions: &[]banking.Condition{{Field: "contra_account_name", Operator: "contains", Value: "hosting"}},
	}); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile("../bankfile/testdata/camt053.xml")
	if err != nil {
		t.Fatal(err)
	}
	res, err := svc.Import(ctx, admin, acc.ID, owner, data)
	if err != nil || res.Imported != 2 || res.AutoBooked != 1 {
		t.Fatalf("Import() = %+v, %v", res, err)
	}
	again, err := svc.Import(ctx, admin, acc.ID, owner, data)
	if err != nil || again.Imported != 0 || again.Duplicates != 2 {
		t.Fatalf("second Import() = %+v, %v", again, err)
	}

	f, _ := banking.ParseMutationFilter("period:202611..202611,state:unprocessed", time.Now())
	open, err := svc.Mutations(ctx, admin, owner, f)
	if err != nil || len(open) != 1 || open[0].Amount != 100000 {
		t.Fatalf("unprocessed = %+v, %v", open, err)
	}
	revenue := categoryByKey(t, books, admin, owner, "revenue")
	m, err := svc.LinkBooking(ctx, admin, open[0].ID, owner, banking.Link{Type: "LedgerAccount", BookingID: &revenue, Price: ptr("600"), Description: "Part"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.LinkBooking(ctx, admin, m.ID, owner, banking.Link{Type: "LedgerAccount", BookingID: &revenue}); err != nil {
		t.Fatal(err)
	}
	got, err := store.Mutation(ctx, admin, m.ID)
	if err != nil || got.State != banking.Processed || len(got.Bookings) != 2 || got.Open() != 0 || got.Bookings[0].Description != "Part" {
		t.Errorf("Mutation() = %+v, %v", got, err)
	}
	if _, err := svc.UnlinkBooking(ctx, admin, m.ID, owner, "LedgerAccountBooking", got.Bookings[1].ID); err != nil {
		t.Fatal(err)
	}
	if got, _ = store.Mutation(ctx, admin, m.ID); got.State != banking.Unprocessed || len(got.Bookings) != 1 {
		t.Errorf("after unlink = %+v", got)
	}

	versions, _ := svc.MutationVersions(ctx, admin, owner)
	balances, total, _ := svc.Balances(ctx, admin, owner)
	if len(versions) != 2 || len(balances) != 1 || total != 97450 {
		t.Errorf("versions %d, balances %+v, total %s", len(versions), balances, total)
	}

	if err := svc.DeleteStatement(ctx, admin, m.StatementID, owner); !errors.Is(err, banking.ErrStatementBooked) {
		t.Errorf("delete booked statement error = %v", err)
	}
	// A category with bookings cannot be deleted, only deactivated.
	if _, _, err := books.DeleteLedgerAccount(ctx, admin, revenue, owner); err == nil {
		if _, _, err := books.DeleteLedgerAccount(ctx, admin, revenue, owner); !errors.Is(err, bookkeeping.ErrInUse) {
			t.Errorf("delete used category error = %v", err)
		}
	}
}

func TestBankingStatementRoundTrip(t *testing.T) {
	ctx := context.Background()
	svc, _, store, admin, owner := bankingService(t)
	list, _ := svc.Accounts(ctx, admin, owner)
	cash := list[0]
	st, err := svc.CreateStatement(ctx, admin, owner, banking.StatementInput{
		AccountID: &cash.ID, Reference: ptr("Till"), OfficialDate: ptr("2026-11-01"), OfficialBalance: ptr("-3.5"), Manual: true,
		Mutations: []banking.MutationInput{{Date: ptr("2026-11-01"), Amount: ptr("-3.5"), Message: ptr("Coffee"), ValueDate: ptr("2026-11-02")}},
	})
	if err != nil {
		t.Fatal(err)
	}
	got, err := store.Statement(ctx, admin, st.ID)
	if err != nil || got.Reference != "Till" || *got.OfficialBalance != -350 || got.OfficialDate.Format(time.DateOnly) != "2026-11-01" || got.Importer != banking.ImporterManual ||
		len(got.Mutations) != 1 || got.Mutations[0].ValueDate.Format(time.DateOnly) != "2026-11-02" {
		t.Fatalf("Statement() = %+v, %v", got, err)
	}
	mid := got.Mutations[0].ID
	if _, err := svc.UpdateStatement(ctx, admin, st.ID, owner, banking.StatementInput{Mutations: []banking.MutationInput{{ID: &mid, Message: ptr("Tea")}, {Date: ptr("2026-11-03"), Amount: ptr("5")}}}); err != nil {
		t.Fatal(err)
	}
	if got, _ = store.Statement(ctx, admin, st.ID); len(got.Mutations) != 2 || got.Mutations[0].Message != "Tea" {
		t.Errorf("after update = %+v", got.Mutations)
	}
	if err := svc.DeleteStatement(ctx, admin, st.ID, owner); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Mutation(ctx, admin, mid); !errors.Is(err, banking.ErrMutationNotFound) {
		t.Errorf("mutation of deleted statement error = %v", err)
	}
}

func TestBankingSummary(t *testing.T) {
	ctx := context.Background()
	svc, books, _, admin, owner := bankingService(t)
	acc, err := svc.CreateAccount(ctx, admin, owner, banking.AccountChanges{Name: ptr("Main bank"), Identifier: ptr("NL91ABNA0417164300")})
	if err != nil {
		t.Fatal(err)
	}
	software := categoryByKey(t, books, admin, owner, "software")
	if _, err := svc.CreateRule(ctx, admin, owner, banking.RuleChanges{
		Name: ptr("Hosting"), LedgerAccountID: &software, Direction: ptr("debit"),
		Conditions: &[]banking.Condition{{Field: "contra_account_name", Operator: "contains", Value: "hosting"}},
	}); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile("../bankfile/testdata/camt053.xml")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Import(ctx, admin, acc.ID, owner, data); err != nil {
		t.Fatal(err)
	}

	got, err := svc.Summary(ctx, admin, owner, 25)
	if err != nil {
		t.Fatal(err)
	}
	if got.Unprocessed != 1 || got.Balance != 97450 || got.Currency != "EUR" {
		t.Errorf("summary = %+v", got)
	}
	if len(got.RecentlyProcessed) != 1 || got.RecentlyProcessed[0].State != banking.Processed || len(got.RecentlyProcessed[0].Bookings) != 1 {
		t.Errorf("recently processed = %+v", got.RecentlyProcessed)
	}
	if none, _ := svc.Summary(ctx, admin, owner, 0); len(none.RecentlyProcessed) != 0 {
		t.Errorf("limit 0 returned %d", len(none.RecentlyProcessed))
	}

	// Inactive accounts have nothing left to link.
	if _, err := svc.UpdateAccount(ctx, admin, acc.ID, owner, banking.AccountChanges{Active: ptr(false)}); err != nil {
		t.Fatal(err)
	}
	if got, _ = svc.Summary(ctx, admin, owner, 25); got.Unprocessed != 0 {
		t.Errorf("unprocessed after deactivating = %d", got.Unprocessed)
	}
}
