package postgres_test

import (
	"context"
	"errors"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/adirkuhn/moneyharpy/internal/administration"
	"github.com/adirkuhn/moneyharpy/internal/identity"
	"github.com/adirkuhn/moneyharpy/internal/postgres"
	"github.com/adirkuhn/moneyharpy/internal/postgres/postgrestest"
	"github.com/adirkuhn/moneyharpy/internal/resource"
	"github.com/adirkuhn/moneyharpy/internal/settings"
	"github.com/google/go-cmp/cmp"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ids = resource.NewGenerator(9)

// testPool is shared by all tests. Each test uses fresh IDs and emails, so
// tests stay independent without truncating tables.
var testPool *pgxpool.Pool

func TestMain(m *testing.M) {
	ctx := context.Background()
	pool, cleanup, err := postgrestest.Start(ctx)
	if err != nil {
		panic(err)
	}
	testPool = pool
	code := m.Run()
	cleanup()
	os.Exit(code)
}

func newUser(t *testing.T) identity.User {
	t.Helper()
	id := ids.Next()
	u, err := postgres.NewIdentityStore(testPool).InsertUser(context.Background(), identity.User{
		ID: id, Email: id.String() + "@example.com", Name: "U",
	}, "hash")
	if err != nil {
		t.Fatalf("InsertUser() error = %v", err)
	}
	return u
}

func TestInsertUserRejectsDuplicateEmail(t *testing.T) {
	s := postgres.NewIdentityStore(testPool)
	u := newUser(t)
	_, err := s.InsertUser(context.Background(), identity.User{ID: ids.Next(), Email: u.Email, Name: "Dup"}, "")
	if !errors.Is(err, identity.ErrEmailTaken) {
		t.Errorf("InsertUser() error = %v, want ErrEmailTaken", err)
	}
}

func TestOnlyOneConcurrentFirstUserBecomesAdmin(t *testing.T) {
	ctx := context.Background()
	pool, cleanup, err := postgrestest.Start(ctx)
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	defer cleanup()
	s := postgres.NewIdentityStore(pool)

	const n = 16
	admins := make(chan bool, n)
	var wg sync.WaitGroup
	for range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			id := ids.Next()
			u, err := s.InsertUser(ctx, identity.User{ID: id, Email: id.String() + "@x.nl", Name: "U"}, "")
			if err != nil {
				t.Errorf("InsertUser() error = %v", err)
				return
			}
			admins <- u.InstanceAdmin
		}()
	}
	wg.Wait()
	close(admins)

	count := 0
	for admin := range admins {
		if admin {
			count++
		}
	}
	if count != 1 {
		t.Errorf("instance admins = %d, want exactly 1", count)
	}
}

func TestSessionRespectsExpiry(t *testing.T) {
	ctx := context.Background()
	s := postgres.NewIdentityStore(testPool)
	u := newUser(t)
	now := time.Now()
	hash := u.ID.String() + "-session"

	if err := s.InsertSession(ctx, hash, u.ID, now.Add(time.Hour)); err != nil {
		t.Fatalf("InsertSession() error = %v", err)
	}
	if _, err := s.UserBySession(ctx, hash, now); err != nil {
		t.Errorf("UserBySession() before expiry error = %v", err)
	}
	if _, err := s.UserBySession(ctx, hash, now.Add(2*time.Hour)); !errors.Is(err, identity.ErrUserNotFound) {
		t.Errorf("UserBySession() after expiry error = %v, want ErrUserNotFound", err)
	}
}

func TestDeleteTokenIsScopedToOwner(t *testing.T) {
	ctx := context.Background()
	s := postgres.NewIdentityStore(testPool)
	owner, other := newUser(t), newUser(t)
	tok := identity.Token{ID: ids.Next(), UserID: owner.ID, Name: "ci", CreatedAt: time.Now()}
	if err := s.InsertToken(ctx, tok.ID.String(), tok); err != nil {
		t.Fatalf("InsertToken() error = %v", err)
	}

	if err := s.DeleteToken(ctx, tok.ID, other.ID); !errors.Is(err, identity.ErrTokenNotFound) {
		t.Errorf("DeleteToken() by other user error = %v, want ErrTokenNotFound", err)
	}
	if err := s.DeleteToken(ctx, tok.ID, owner.ID); err != nil {
		t.Errorf("DeleteToken() by owner error = %v", err)
	}
}

func TestSettingsRoundTrip(t *testing.T) {
	ctx := context.Background()
	s := postgres.NewSettingsStore(testPool)
	key := ids.Next().String()

	if _, err := s.Load(ctx, key); !errors.Is(err, settings.ErrNotSet) {
		t.Errorf("Load() error = %v, want ErrNotSet", err)
	}
	for _, value := range []string{`{"a":1}`, `{"a":2}`} {
		if err := s.Save(ctx, key, []byte(value)); err != nil {
			t.Fatalf("Save() error = %v", err)
		}
	}
	got, err := s.Load(ctx, key)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if diff := cmp.Diff(`{"a": 2}`, string(got)); diff != "" {
		t.Errorf("Load() diff (-want +got):\n%s", diff)
	}
}

func newAdministration() administration.Administration {
	return administration.Administration{
		ID: ids.Next(), Name: "Harpy", Language: "en", Currency: "EUR", Country: "NL",
		TimeZone: "Europe/Amsterdam", PeriodStartDate: "2026-01-01",
		Company: administration.Company{Address1: "Dam 1", Address2: "2", Zipcode: "1012JS", City: "Amsterdam", ChamberOfCommerce: "123"},
	}
}

func TestAdministrationRoundTripAndAccess(t *testing.T) {
	ctx := context.Background()
	s := postgres.NewAdministrationStore(testPool)
	owner, stranger := newUser(t), newUser(t)
	a := newAdministration()

	if err := s.Create(ctx, a, owner.ID); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	got, err := s.ForUser(ctx, a.ID, owner.ID)
	if err != nil {
		t.Fatalf("ForUser() error = %v", err)
	}
	want := administration.Listed{Administration: a, Membership: administration.Membership{Access: administration.AccessUser}}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("ForUser() diff (-want +got):\n%s", diff)
	}

	listed, err := s.ListForUser(ctx, stranger.ID)
	if err != nil {
		t.Fatalf("ListForUser() error = %v", err)
	}
	if len(listed) != 0 {
		t.Errorf("ListForUser(stranger) = %v, want empty", listed)
	}
	if _, err := s.ForUser(ctx, a.ID, stranger.ID); !errors.Is(err, administration.ErrNotFound) {
		t.Errorf("ForUser(stranger) error = %v, want ErrNotFound", err)
	}
	if err := s.SetFavorite(ctx, a.ID, stranger.ID, true); !errors.Is(err, administration.ErrNotFound) {
		t.Errorf("SetFavorite(stranger) error = %v, want ErrNotFound", err)
	}
}

func TestLocateResource(t *testing.T) {
	ctx := context.Background()
	admins := postgres.NewAdministrationStore(testPool)
	locator := postgres.NewResourceLocator(testPool)
	owner, stranger := newUser(t), newUser(t)
	a := newAdministration()
	if err := admins.Create(ctx, a, owner.ID); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	got, err := locator.Locate(ctx, a.ID, owner.ID)
	if err != nil {
		t.Fatalf("Locate() error = %v", err)
	}
	if diff := cmp.Diff(a.Ref(), got); diff != "" {
		t.Errorf("Locate() diff (-want +got):\n%s", diff)
	}

	if _, err := locator.Locate(ctx, a.ID, stranger.ID); !errors.Is(err, resource.ErrNotFound) {
		t.Errorf("Locate(stranger) error = %v, want resource.ErrNotFound", err)
	}
}

func TestResetPasswordIsSingleUseAndEndsSessions(t *testing.T) {
	ctx := context.Background()
	s := postgres.NewIdentityStore(testPool)
	u := newUser(t)
	now := time.Now()
	session := u.ID.String() + "-sess"
	if err := s.InsertSession(ctx, session, u.ID, now.Add(time.Hour)); err != nil {
		t.Fatalf("InsertSession() error = %v", err)
	}
	first, second := u.ID.String()+"-reset-1", u.ID.String()+"-reset-2"
	for _, hash := range []string{first, second} {
		if err := s.ReplaceResetToken(ctx, hash, u.ID, now.Add(time.Hour)); err != nil {
			t.Fatalf("ReplaceResetToken() error = %v", err)
		}
	}

	if err := s.ResetPassword(ctx, first, "new", now); !errors.Is(err, identity.ErrResetTokenInvalid) {
		t.Errorf("ResetPassword() with replaced token error = %v, want ErrResetTokenInvalid", err)
	}
	if err := s.ResetPassword(ctx, second, "new", now.Add(2*time.Hour)); !errors.Is(err, identity.ErrResetTokenInvalid) {
		t.Errorf("ResetPassword() with expired token error = %v, want ErrResetTokenInvalid", err)
	}
	if err := s.ResetPassword(ctx, second, "new", now); err != nil {
		t.Fatalf("ResetPassword() error = %v", err)
	}
	if err := s.ResetPassword(ctx, second, "newer", now); !errors.Is(err, identity.ErrResetTokenInvalid) {
		t.Errorf("ResetPassword() reused token error = %v, want ErrResetTokenInvalid", err)
	}

	_, hash, err := s.UserByEmail(ctx, u.Email)
	if err != nil {
		t.Fatalf("UserByEmail() error = %v", err)
	}
	if hash != "new" {
		t.Errorf("password hash = %q, want new", hash)
	}
	if _, err := s.UserBySession(ctx, session, now); !errors.Is(err, identity.ErrUserNotFound) {
		t.Errorf("UserBySession() error = %v, want ErrUserNotFound", err)
	}
}
