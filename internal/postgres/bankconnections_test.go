package postgres_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/adirkuhn/moneyharpy/internal/banking"
)

func TestBankConnectionsRoundTrip(t *testing.T) {
	ctx := context.Background()
	svc, _, store, admin, owner := bankingService(t)
	acc, err := svc.CreateAccount(ctx, admin, owner, banking.AccountChanges{Name: ptr("Main bank"), Identifier: ptr("NL91ABNA0417164300")})
	if err != nil {
		t.Fatal(err)
	}
	other, err := svc.CreateAccount(ctx, admin, owner, banking.AccountChanges{Name: ptr("Savings"), Identifier: ptr("NL20INGB0001234567")})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Truncate(time.Microsecond)
	until := now.Add(180 * 24 * time.Hour)
	c := banking.Connection{
		ID: ids.Next(), AdministrationID: admin, Provider: banking.ProviderEnableBanking, Bank: "Example Bank", Country: "NL",
		State: banking.ConnectionPending, AuthState: "state-1", CreatedBy: owner, CreatedAt: now, UpdatedAt: now,
	}
	if err := store.SaveConnection(ctx, c); err != nil {
		t.Fatal(err)
	}
	got, err := store.ConnectionByAuthState(ctx, "state-1")
	if err != nil || got.ID != c.ID || got.State != banking.ConnectionPending || got.SessionID != "" || got.ValidUntil != nil || len(got.Accounts) != 0 {
		t.Fatalf("by state = %+v, %v", got, err)
	}

	c.AuthState, c.State, c.SessionID, c.ValidUntil, c.Secret = "", banking.ConnectionActive, "session-1", &until, "sealed-key"
	c.Accounts = []banking.LinkedAccount{{AccountID: acc.ID, UID: "uid-1"}, {AccountID: other.ID, UID: "uid-2"}}
	if err := store.SaveConnection(ctx, c); err != nil {
		t.Fatal(err)
	}
	if _, err := store.ConnectionByAuthState(ctx, "state-1"); !errors.Is(err, banking.ErrConnectionNotFound) {
		t.Errorf("used state = %v", err)
	}
	got, err = store.Connection(ctx, admin, c.ID)
	if err != nil || got.SessionID != "session-1" || got.Secret != "sealed-key" || !got.ValidUntil.Equal(until) || len(got.Accounts) != 2 || got.Accounts[0] != c.Accounts[0] || got.Accounts[1] != c.Accounts[1] {
		t.Fatalf("Connection() = %+v, %v", got, err)
	}
	due, err := store.DueConnections(ctx, now.Add(time.Minute))
	if err != nil || !containsConnection(due, c.ID) {
		t.Errorf("due = %+v, %v", due, err)
	}
	c.LastSyncAt = &now
	c.Accounts = c.Accounts[1:]
	if err := store.SaveConnection(ctx, c); err != nil {
		t.Fatal(err)
	}
	if due, _ := store.DueConnections(ctx, now.Add(-time.Minute)); containsConnection(due, c.ID) {
		t.Error("a connection fetched later is due")
	}
	list, err := store.Connections(ctx, admin)
	if err != nil || len(list) != 1 || len(list[0].Accounts) != 1 || list[0].Accounts[0].AccountID != other.ID {
		t.Errorf("Connections() = %+v, %v", list, err)
	}
	if err := store.DeleteConnection(ctx, admin, c.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Connection(ctx, admin, c.ID); !errors.Is(err, banking.ErrConnectionNotFound) {
		t.Errorf("deleted = %v", err)
	}
}

func containsConnection(list []banking.Connection, id interface{ String() string }) bool {
	for _, c := range list {
		if c.ID.String() == id.String() {
			return true
		}
	}
	return false
}
