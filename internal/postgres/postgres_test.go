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
	"github.com/google/go-cmp/cmp/cmpopts"
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
	}
}

func newDefaultIdentity(a administration.Administration) administration.Identity {
	now := time.Now().UTC().Truncate(time.Microsecond)
	return administration.Identity{
		ID: ids.Next(), AdministrationID: a.ID, Default: true, CompanyName: a.Name, Country: "NL",
		City: "Amsterdam", Zipcode: "1012JS", Address1: "Dam 1", Address2: "2", Email: "a@example.com", Phone: "06",
		LegalEntityType: "BV", ChamberOfCommerce: "123", TaxNumber: "NL1B01",
		BankAccountName: "Harpy", BankAccountNumber: "NL91ABNA0417164300", BankAccountBIC: "ABNANL2A",
		CreatedAt: now, UpdatedAt: now,
	}
}

func TestIdentityRoundTripAndLocation(t *testing.T) {
	ctx := context.Background()
	s := postgres.NewAdministrationStore(testPool)
	locator := postgres.NewResourceLocator(testPool)
	owner := newUser(t)
	a := newAdministration()
	def := newDefaultIdentity(a)
	if err := s.Create(ctx, a, owner.ID, def); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	got, err := s.DefaultIdentity(ctx, a.ID)
	if err != nil {
		t.Fatalf("DefaultIdentity() error = %v", err)
	}
	if diff := cmp.Diff(def, got, cmpopts.EquateApproxTime(time.Millisecond)); diff != "" {
		t.Errorf("DefaultIdentity() diff (-want +got):\n%s", diff)
	}

	extra := def
	extra.ID, extra.Default, extra.CompanyName = ids.Next(), false, "Alpha trade name"
	if err := s.CreateIdentity(ctx, extra); err != nil {
		t.Fatalf("CreateIdentity() error = %v", err)
	}
	listed, err := s.ListIdentities(ctx, a.ID)
	if err != nil {
		t.Fatalf("ListIdentities() error = %v", err)
	}
	if len(listed) != 2 || listed[0].ID != def.ID {
		t.Errorf("ListIdentities() = %+v, want default first", listed)
	}

	ref, err := locator.Locate(ctx, extra.ID, owner.ID)
	if err != nil {
		t.Fatalf("Locate() error = %v", err)
	}
	if diff := cmp.Diff(extra.Ref(), ref); diff != "" {
		t.Errorf("Locate() diff (-want +got):\n%s", diff)
	}

	extra.Phone = "07"
	if err := s.UpdateIdentity(ctx, extra); err != nil {
		t.Fatalf("UpdateIdentity() error = %v", err)
	}
	if got, err := s.Identity(ctx, a.ID, extra.ID); err != nil || got.Phone != "07" {
		t.Errorf("Identity() = %+v, %v; want updated phone", got, err)
	}

	if err := s.DeleteIdentity(ctx, a.ID, extra.ID); err != nil {
		t.Fatalf("DeleteIdentity() error = %v", err)
	}
	if _, err := s.Identity(ctx, a.ID, extra.ID); !errors.Is(err, administration.ErrIdentityNotFound) {
		t.Errorf("Identity() after delete error = %v, want ErrIdentityNotFound", err)
	}
	if _, err := locator.Locate(ctx, extra.ID, owner.ID); !errors.Is(err, resource.ErrNotFound) {
		t.Errorf("Locate() after delete error = %v, want resource.ErrNotFound", err)
	}
	if err := s.DeleteIdentity(ctx, a.ID, extra.ID); !errors.Is(err, administration.ErrIdentityNotFound) {
		t.Errorf("DeleteIdentity() twice error = %v, want ErrIdentityNotFound", err)
	}
}

func TestCustomFieldsAndIdentityValues(t *testing.T) {
	ctx := context.Background()
	s := postgres.NewAdministrationStore(testPool)
	locator := postgres.NewResourceLocator(testPool)
	owner := newUser(t)
	a, other := newAdministration(), newAdministration()
	def := newDefaultIdentity(a)
	for _, x := range []struct {
		a administration.Administration
		i administration.Identity
	}{{a, def}, {other, newDefaultIdentity(other)}} {
		if err := s.Create(ctx, x.a, owner.ID, x.i); err != nil {
			t.Fatalf("Create() error = %v", err)
		}
	}
	now := time.Now().UTC()
	slogan := administration.CustomField{ID: ids.Next(), AdministrationID: a.ID, Name: "Slogan", Source: administration.SourceIdentity, CreatedAt: now, UpdatedAt: now}
	motto := slogan
	motto.ID, motto.Name = ids.Next(), "Motto"
	foreign := slogan
	foreign.ID, foreign.AdministrationID = ids.Next(), other.ID
	for _, f := range []administration.CustomField{slogan, motto, foreign} {
		if err := s.CreateCustomField(ctx, f); err != nil {
			t.Fatalf("CreateCustomField() error = %v", err)
		}
	}

	fields, err := s.CustomFields(ctx, a.ID)
	if err != nil {
		t.Fatalf("CustomFields() error = %v", err)
	}
	if diff := cmp.Diff([]administration.CustomField{slogan, motto}, fields, cmpopts.EquateApproxTime(time.Millisecond)); diff != "" {
		t.Errorf("CustomFields() diff (-want +got):\n%s", diff)
	}
	if ref, err := locator.Locate(ctx, slogan.ID, owner.ID); err != nil || ref != slogan.Ref() {
		t.Errorf("Locate(custom field) = %+v, %v; want %+v", ref, err, slogan.Ref())
	}

	def.TaxNumberAbsenceReason = administration.BelowTaxThreshold
	def.CustomFields = []administration.CustomFieldValue{{ID: slogan.ID, Name: "Slogan", Value: "Fresh"}, {ID: motto.ID, Name: "Motto", Value: "Fast"}}
	if err := s.UpdateIdentity(ctx, def); err != nil {
		t.Fatalf("UpdateIdentity() error = %v", err)
	}
	got, err := s.DefaultIdentity(ctx, a.ID)
	if err != nil {
		t.Fatalf("DefaultIdentity() error = %v", err)
	}
	if diff := cmp.Diff(def, got, cmpopts.EquateApproxTime(time.Millisecond)); diff != "" {
		t.Errorf("DefaultIdentity() diff (-want +got):\n%s", diff)
	}

	def.CustomFields = def.CustomFields[1:]
	if err := s.UpdateIdentity(ctx, def); err != nil {
		t.Fatalf("UpdateIdentity() error = %v", err)
	}
	listed, err := s.ListIdentities(ctx, a.ID)
	if err != nil {
		t.Fatalf("ListIdentities() error = %v", err)
	}
	if diff := cmp.Diff(def.CustomFields, listed[0].CustomFields); diff != "" {
		t.Errorf("ListIdentities() custom fields diff (-want +got):\n%s", diff)
	}
}

func TestOnlyOneDefaultIdentityPerAdministration(t *testing.T) {
	ctx := context.Background()
	s := postgres.NewAdministrationStore(testPool)
	owner := newUser(t)
	a := newAdministration()
	def := newDefaultIdentity(a)
	if err := s.Create(ctx, a, owner.ID, def); err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	second := def
	second.ID = ids.Next()
	if err := s.CreateIdentity(ctx, second); err == nil {
		t.Error("CreateIdentity() of second default error = nil, want constraint failure")
	}
}

func TestUpdateAdministrationPersists(t *testing.T) {
	ctx := context.Background()
	s := postgres.NewAdministrationStore(testPool)
	owner := newUser(t)
	a := newAdministration()
	if err := s.Create(ctx, a, owner.ID, newDefaultIdentity(a)); err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	a.Name, a.Language, a.Currency, a.TimeZone = "New", "nl", "USD", "Europe/Brussels"
	a.Profile = administration.Profile{
		SmallBusinessScheme: true, CompanyActivitiesDescription: "Bakery", CompanyWebsiteURL: "https://example.com",
	}
	if err := s.Update(ctx, a); err != nil {
		t.Fatalf("Update() error = %v", err)
	}
	got, err := s.ForUser(ctx, a.ID, owner.ID)
	if err != nil {
		t.Fatalf("ForUser() error = %v", err)
	}
	if diff := cmp.Diff(a, got.Administration); diff != "" {
		t.Errorf("ForUser() diff (-want +got):\n%s", diff)
	}
}

func TestAdministrationRoundTripAndAccess(t *testing.T) {
	ctx := context.Background()
	s := postgres.NewAdministrationStore(testPool)
	owner, stranger := newUser(t), newUser(t)
	a := newAdministration()

	if err := s.Create(ctx, a, owner.ID, newDefaultIdentity(a)); err != nil {
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
	if err := admins.Create(ctx, a, owner.ID, newDefaultIdentity(a)); err != nil {
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

func TestIdentityMigrationKeepsExistingCompanyDetails(t *testing.T) {
	ctx := context.Background()
	url, cleanup, err := postgrestest.StartEmpty(ctx)
	if err != nil {
		t.Fatalf("StartEmpty() error = %v", err)
	}
	defer cleanup()
	if err := postgres.MigrateTo(ctx, url, 3); err != nil {
		t.Fatalf("MigrateTo(3) error = %v", err)
	}
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer pool.Close()

	// Generator-shaped IDs with consecutive sequence numbers, as two
	// administrations created in the same millisecond would have.
	const first, second int64 = 1<<40 | 1<<12 | 5, 1<<40 | 1<<12 | 6
	for _, id := range []int64{first, second} {
		if _, err := pool.Exec(ctx, `INSERT INTO administrations
			(id, name, language, currency, country, time_zone, period_start_date, address1, address2, zipcode, city, chamber_of_commerce)
			VALUES ($1, 'Harpy', 'en', 'EUR', 'BE', 'Europe/Brussels', '2026-01-01', 'Dam 1', 'Floor 2', '1012JS', 'Amsterdam', '123')`, id); err != nil {
			t.Fatalf("insert administration: %v", err)
		}
		if _, err := pool.Exec(ctx, `INSERT INTO resources (id, type, administration_id) VALUES ($1, 'administrations', $1)`, id); err != nil {
			t.Fatalf("insert resource: %v", err)
		}
	}

	if err := postgres.Migrate(ctx, url); err != nil {
		t.Fatalf("Migrate() error = %v", err)
	}

	s := postgres.NewAdministrationStore(pool)
	seen := map[resource.ID]bool{}
	for _, id := range []int64{first, second} {
		got, err := s.DefaultIdentity(ctx, resource.ID(id))
		if err != nil {
			t.Fatalf("DefaultIdentity(%d) error = %v", id, err)
		}
		if got.CompanyName != "Harpy" || got.Country != "BE" || got.Address1 != "Dam 1" || got.Address2 != "Floor 2" ||
			got.Zipcode != "1012JS" || got.City != "Amsterdam" || got.ChamberOfCommerce != "123" || !got.Default {
			t.Errorf("DefaultIdentity(%d) = %+v, want migrated details", id, got)
		}
		if got.ID == resource.ID(first) || got.ID == resource.ID(second) || seen[got.ID] {
			t.Errorf("identity ID %d collides", got.ID)
		}
		seen[got.ID] = true
	}
}
