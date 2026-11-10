// Package postgres implements the application's stores on PostgreSQL.
package postgres

import (
	"context"
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"time"

	"github.com/adirkuhn/moneyharpy/internal/administration"
	"github.com/adirkuhn/moneyharpy/internal/identity"
	"github.com/adirkuhn/moneyharpy/internal/postgres/sqlc"
	"github.com/adirkuhn/moneyharpy/internal/resource"
	"github.com/adirkuhn/moneyharpy/internal/settings"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib" // Registers the driver goose migrates through.
	"github.com/pressly/goose/v3"
)

//go:embed migrations/*.sql
var migrations embed.FS

const (
	uniqueViolation = "23505"
	dateLayout      = "2006-01-02"
)

// Migrate brings the database at url up to the latest schema.
func Migrate(ctx context.Context, url string) error {
	return withProvider(url, func(p *goose.Provider) error {
		_, err := p.Up(ctx)
		return err
	})
}

// MigrateTo brings the database at url up to a specific schema version.
func MigrateTo(ctx context.Context, url string, version int64) error {
	return withProvider(url, func(p *goose.Provider) error {
		_, err := p.UpTo(ctx, version)
		return err
	})
}

func withProvider(url string, run func(*goose.Provider) error) error {
	db, err := sql.Open("pgx", url)
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer func() {
		// Migrations have already succeeded or failed by now.
		_ = db.Close()
	}()

	provider, err := goose.NewProvider(goose.DialectPostgres, db, subFS())
	if err != nil {
		return fmt.Errorf("create migration provider: %w", err)
	}
	if err := run(provider); err != nil {
		return fmt.Errorf("apply migrations: %w", err)
	}
	return nil
}

// IdentityStore implements identity.Store.
type IdentityStore struct {
	pool *pgxpool.Pool
	q    *sqlc.Queries
}

// NewIdentityStore returns an IdentityStore on pool.
func NewIdentityStore(pool *pgxpool.Pool) *IdentityStore {
	return &IdentityStore{pool: pool, q: sqlc.New(pool)}
}

// InsertUser implements identity.Store. A transaction-scoped advisory lock
// serializes inserts, so exactly one concurrent first user becomes admin.
func (s *IdentityStore) InsertUser(ctx context.Context, u identity.User, passwordHash string) (identity.User, error) {
	var saved sqlc.InsertUserRow
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		q := s.q.WithTx(tx)
		if err := q.LockFirstUser(ctx); err != nil {
			return err
		}
		var err error
		saved, err = q.InsertUser(ctx, sqlc.InsertUserParams{
			ID: int64(u.ID), Email: u.Email, Name: u.Name, PasswordHash: passwordHash,
		})
		return err
	})
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == uniqueViolation {
			return identity.User{}, identity.ErrEmailTaken
		}
		return identity.User{}, err
	}
	return user(saved.ID, saved.Email, saved.Name, saved.IsInstanceAdmin), nil
}

// UserByEmail implements identity.Store.
func (s *IdentityStore) UserByEmail(ctx context.Context, email string) (identity.User, string, error) {
	r, err := s.q.UserByEmail(ctx, email)
	if err != nil {
		return identity.User{}, "", userErr(err)
	}
	return user(r.ID, r.Email, r.Name, r.IsInstanceAdmin), r.PasswordHash, nil
}

// UserByIdentity implements identity.Store.
func (s *IdentityStore) UserByIdentity(ctx context.Context, provider, subject string) (identity.User, error) {
	r, err := s.q.UserByIdentity(ctx, sqlc.UserByIdentityParams{Provider: provider, Subject: subject})
	if err != nil {
		return identity.User{}, userErr(err)
	}
	return user(r.ID, r.Email, r.Name, r.IsInstanceAdmin), nil
}

// LinkIdentity implements identity.Store.
func (s *IdentityStore) LinkIdentity(ctx context.Context, u resource.ID, provider, subject string) error {
	return s.q.LinkIdentity(ctx, sqlc.LinkIdentityParams{Provider: provider, Subject: subject, UserID: int64(u)})
}

// InsertSession implements identity.Store.
func (s *IdentityStore) InsertSession(ctx context.Context, hash string, u resource.ID, expires time.Time) error {
	return s.q.InsertSession(ctx, sqlc.InsertSessionParams{TokenHash: hash, UserID: int64(u), ExpiresAt: timestamp(expires)})
}

// UserBySession implements identity.Store.
func (s *IdentityStore) UserBySession(ctx context.Context, hash string, now time.Time) (identity.User, error) {
	r, err := s.q.UserBySession(ctx, sqlc.UserBySessionParams{TokenHash: hash, ExpiresAt: timestamp(now)})
	if err != nil {
		return identity.User{}, userErr(err)
	}
	return user(r.ID, r.Email, r.Name, r.IsInstanceAdmin), nil
}

// DeleteSession implements identity.Store.
func (s *IdentityStore) DeleteSession(ctx context.Context, hash string) error {
	return s.q.DeleteSession(ctx, hash)
}

// InsertToken implements identity.Store.
func (s *IdentityStore) InsertToken(ctx context.Context, hash string, t identity.Token) error {
	return s.q.InsertToken(ctx, sqlc.InsertTokenParams{
		ID: int64(t.ID), UserID: int64(t.UserID), Name: t.Name, TokenHash: hash, CreatedAt: timestamp(t.CreatedAt),
	})
}

// UserByToken implements identity.Store.
func (s *IdentityStore) UserByToken(ctx context.Context, hash string) (identity.User, error) {
	r, err := s.q.UserByToken(ctx, hash)
	if err != nil {
		return identity.User{}, userErr(err)
	}
	return user(r.ID, r.Email, r.Name, r.IsInstanceAdmin), nil
}

// ListTokens implements identity.Store.
func (s *IdentityStore) ListTokens(ctx context.Context, u resource.ID) ([]identity.Token, error) {
	rows, err := s.q.ListTokens(ctx, int64(u))
	if err != nil {
		return nil, err
	}
	tokens := make([]identity.Token, len(rows))
	for i, r := range rows {
		tokens[i] = identity.Token{ID: resource.ID(r.ID), UserID: resource.ID(r.UserID), Name: r.Name, CreatedAt: r.CreatedAt.Time}
	}
	return tokens, nil
}

// DeleteToken implements identity.Store.
func (s *IdentityStore) DeleteToken(ctx context.Context, id, u resource.ID) error {
	n, err := s.q.DeleteToken(ctx, sqlc.DeleteTokenParams{ID: int64(id), UserID: int64(u)})
	if err != nil {
		return err
	}
	if n == 0 {
		return identity.ErrTokenNotFound
	}
	return nil
}

// ReplaceResetToken implements identity.Store.
func (s *IdentityStore) ReplaceResetToken(ctx context.Context, hash string, u resource.ID, expires time.Time) error {
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		q := s.q.WithTx(tx)
		if err := q.DeleteResetTokens(ctx, int64(u)); err != nil {
			return err
		}
		return q.InsertResetToken(ctx, sqlc.InsertResetTokenParams{TokenHash: hash, UserID: int64(u), ExpiresAt: timestamp(expires)})
	})
}

// ResetPassword implements identity.Store.
func (s *IdentityStore) ResetPassword(ctx context.Context, hash, passwordHash string, now time.Time) error {
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		q := s.q.WithTx(tx)
		userID, err := q.ConsumeResetToken(ctx, sqlc.ConsumeResetTokenParams{TokenHash: hash, ExpiresAt: timestamp(now)})
		if errors.Is(err, pgx.ErrNoRows) {
			return identity.ErrResetTokenInvalid
		}
		if err != nil {
			return err
		}
		if err := q.UpdatePassword(ctx, sqlc.UpdatePasswordParams{ID: userID, PasswordHash: passwordHash}); err != nil {
			return err
		}
		return q.DeleteUserSessions(ctx, userID)
	})
}

func user(id int64, email, name string, admin bool) identity.User {
	return identity.User{ID: resource.ID(id), Email: email, Name: name, InstanceAdmin: admin}
}

func userErr(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return identity.ErrUserNotFound
	}
	return err
}

func timestamp(t time.Time) pgtype.Timestamptz {
	return pgtype.Timestamptz{Time: t, Valid: true}
}

// SettingsStore implements settings.Store.
type SettingsStore struct {
	q *sqlc.Queries
}

// NewSettingsStore returns a SettingsStore on pool.
func NewSettingsStore(pool *pgxpool.Pool) *SettingsStore {
	return &SettingsStore{q: sqlc.New(pool)}
}

// Load implements settings.Store.
func (s *SettingsStore) Load(ctx context.Context, key string) ([]byte, error) {
	v, err := s.q.LoadSetting(ctx, key)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, settings.ErrNotSet
	}
	return v, err
}

// Save implements settings.Store.
func (s *SettingsStore) Save(ctx context.Context, key string, value []byte) error {
	return s.q.SaveSetting(ctx, sqlc.SaveSettingParams{Key: key, Value: value})
}

// AdministrationStore implements administration.Store.
type AdministrationStore struct {
	pool *pgxpool.Pool
	q    *sqlc.Queries
}

// NewAdministrationStore returns an AdministrationStore on pool.
func NewAdministrationStore(pool *pgxpool.Pool) *AdministrationStore {
	return &AdministrationStore{pool: pool, q: sqlc.New(pool)}
}

// Create implements administration.Store, registering the administration and
// its default identity as resources in the same transaction.
func (s *AdministrationStore) Create(ctx context.Context, a administration.Administration, owner resource.ID, def administration.Identity) error {
	start, err := parseDate(a.PeriodStartDate)
	if err != nil {
		return fmt.Errorf("period start date: %w", err)
	}
	var locked pgtype.Date
	if a.PeriodLockedUntil != nil {
		if locked, err = parseDate(*a.PeriodLockedUntil); err != nil {
			return fmt.Errorf("period locked until: %w", err)
		}
	}
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		q := s.q.WithTx(tx)
		if err := q.InsertAdministration(ctx, sqlc.InsertAdministrationParams{
			ID: int64(a.ID), Name: a.Name, Language: a.Language, Currency: a.Currency, Country: a.Country,
			TimeZone: a.TimeZone, Suspended: a.Suspended, PeriodLockedUntil: locked, PeriodStartDate: start,
		}); err != nil {
			return err
		}
		if err := q.InsertMember(ctx, sqlc.InsertMemberParams{
			AdministrationID: int64(a.ID), UserID: int64(owner), Access: string(administration.AccessUser),
		}); err != nil {
			return err
		}
		if err := q.InsertResource(ctx, sqlc.InsertResourceParams{
			ID: int64(a.ID), Type: string(resource.TypeAdministration), AdministrationID: int64(a.ID),
		}); err != nil {
			return err
		}
		return insertIdentity(ctx, q, def)
	})
}

// Update implements administration.Store.
func (s *AdministrationStore) Update(ctx context.Context, a administration.Administration) error {
	return s.q.UpdateAdministration(ctx, sqlc.UpdateAdministrationParams{
		ID: int64(a.ID), Name: a.Name, Language: a.Language, Currency: a.Currency, TimeZone: a.TimeZone,
		SmallBusinessScheme: a.SmallBusinessScheme, CompanyActivitiesDescription: a.CompanyActivitiesDescription,
		CompanyWebsiteUrl: a.CompanyWebsiteURL, CompanyWebsiteAbsent: a.CompanyWebsiteAbsent, Country: a.Country,
	})
}

func insertIdentity(ctx context.Context, q *sqlc.Queries, i administration.Identity) error {
	if err := q.InsertIdentity(ctx, sqlc.InsertIdentityParams{
		ID: int64(i.ID), AdministrationID: int64(i.AdministrationID), IsDefault: i.Default, CompanyName: i.CompanyName,
		Country: i.Country, City: i.City, Zipcode: i.Zipcode, Address1: i.Address1, Address2: i.Address2, Email: i.Email,
		Phone: i.Phone, LegalEntityType: i.LegalEntityType, ChamberOfCommerce: i.ChamberOfCommerce, TaxNumber: i.TaxNumber,
		BankAccountName: i.BankAccountName, BankAccountNumber: i.BankAccountNumber, BankAccountBic: i.BankAccountBIC,
		TaxNumberAbsenceReason: string(i.TaxNumberAbsenceReason),
		CreatedAt:              timestamp(i.CreatedAt), UpdatedAt: timestamp(i.UpdatedAt),
	}); err != nil {
		return err
	}
	if err := q.InsertResource(ctx, sqlc.InsertResourceParams{
		ID: int64(i.ID), Type: string(resource.TypeIdentity), AdministrationID: int64(i.AdministrationID),
	}); err != nil {
		return err
	}
	return insertCustomFieldValues(ctx, q, i)
}

func insertCustomFieldValues(ctx context.Context, q *sqlc.Queries, i administration.Identity) error {
	for _, v := range i.CustomFields {
		if err := q.InsertIdentityCustomFieldValue(ctx, sqlc.InsertIdentityCustomFieldValueParams{
			IdentityID: int64(i.ID), CustomFieldID: int64(v.ID), Value: v.Value,
		}); err != nil {
			return err
		}
	}
	return nil
}

// withCustomFields attaches the stored custom field values to identities of
// one administration.
func (s *AdministrationStore) withCustomFields(ctx context.Context, admin resource.ID, identities []administration.Identity) ([]administration.Identity, error) {
	rows, err := s.q.ListIdentityCustomFieldValues(ctx, int64(admin))
	if err != nil {
		return nil, err
	}
	values := map[int64][]administration.CustomFieldValue{}
	for _, r := range rows {
		values[r.IdentityID] = append(values[r.IdentityID], administration.CustomFieldValue{ID: resource.ID(r.ID), Name: r.Name, Value: r.Value})
	}
	for i := range identities {
		identities[i].CustomFields = values[int64(identities[i].ID)]
	}
	return identities, nil
}

func (s *AdministrationStore) oneWithCustomFields(ctx context.Context, i administration.Identity) (administration.Identity, error) {
	out, err := s.withCustomFields(ctx, i.AdministrationID, []administration.Identity{i})
	if err != nil {
		return administration.Identity{}, err
	}
	return out[0], nil
}

func toIdentity(r sqlc.Identity) administration.Identity {
	return administration.Identity{
		ID: resource.ID(r.ID), AdministrationID: resource.ID(r.AdministrationID), Default: r.IsDefault, CompanyName: r.CompanyName,
		Country: r.Country, City: r.City, Zipcode: r.Zipcode, Address1: r.Address1, Address2: r.Address2, Email: r.Email,
		Phone: r.Phone, LegalEntityType: r.LegalEntityType, ChamberOfCommerce: r.ChamberOfCommerce, TaxNumber: r.TaxNumber,
		BankAccountName: r.BankAccountName, BankAccountNumber: r.BankAccountNumber, BankAccountBIC: r.BankAccountBic,
		TaxNumberAbsenceReason: administration.TaxNumberAbsenceReason(r.TaxNumberAbsenceReason),
		CreatedAt:              r.CreatedAt.Time, UpdatedAt: r.UpdatedAt.Time,
	}
}

func identityErr(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return administration.ErrIdentityNotFound
	}
	return err
}

// ListIdentities implements administration.Store.
func (s *AdministrationStore) ListIdentities(ctx context.Context, admin resource.ID) ([]administration.Identity, error) {
	rows, err := s.q.ListIdentities(ctx, int64(admin))
	if err != nil {
		return nil, err
	}
	identities := make([]administration.Identity, len(rows))
	for i, r := range rows {
		identities[i] = toIdentity(r)
	}
	return s.withCustomFields(ctx, admin, identities)
}

// DefaultIdentity implements administration.Store.
func (s *AdministrationStore) DefaultIdentity(ctx context.Context, admin resource.ID) (administration.Identity, error) {
	r, err := s.q.DefaultIdentity(ctx, int64(admin))
	if err != nil {
		return administration.Identity{}, identityErr(err)
	}
	return s.oneWithCustomFields(ctx, toIdentity(r))
}

// Identity implements administration.Store.
func (s *AdministrationStore) Identity(ctx context.Context, admin, id resource.ID) (administration.Identity, error) {
	r, err := s.q.IdentityByID(ctx, sqlc.IdentityByIDParams{AdministrationID: int64(admin), ID: int64(id)})
	if err != nil {
		return administration.Identity{}, identityErr(err)
	}
	return s.oneWithCustomFields(ctx, toIdentity(r))
}

// CreateIdentity implements administration.Store.
func (s *AdministrationStore) CreateIdentity(ctx context.Context, i administration.Identity) error {
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		return insertIdentity(ctx, s.q.WithTx(tx), i)
	})
}

// UpdateIdentity implements administration.Store.
func (s *AdministrationStore) UpdateIdentity(ctx context.Context, i administration.Identity) error {
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		q := s.q.WithTx(tx)
		if err := q.UpdateIdentity(ctx, sqlc.UpdateIdentityParams{
			AdministrationID: int64(i.AdministrationID), ID: int64(i.ID), CompanyName: i.CompanyName, Country: i.Country, City: i.City,
			Zipcode: i.Zipcode, Address1: i.Address1, Address2: i.Address2, Email: i.Email, Phone: i.Phone,
			LegalEntityType: i.LegalEntityType, ChamberOfCommerce: i.ChamberOfCommerce, TaxNumber: i.TaxNumber,
			BankAccountName: i.BankAccountName, BankAccountNumber: i.BankAccountNumber, BankAccountBic: i.BankAccountBIC,
			TaxNumberAbsenceReason: string(i.TaxNumberAbsenceReason), UpdatedAt: timestamp(i.UpdatedAt),
		}); err != nil {
			return err
		}
		if err := q.DeleteIdentityCustomFieldValues(ctx, int64(i.ID)); err != nil {
			return err
		}
		return insertCustomFieldValues(ctx, q, i)
	})
}

// CustomFields implements administration.Store.
func (s *AdministrationStore) CustomFields(ctx context.Context, admin resource.ID) ([]administration.CustomField, error) {
	rows, err := s.q.ListCustomFields(ctx, int64(admin))
	if err != nil {
		return nil, err
	}
	fields := make([]administration.CustomField, len(rows))
	for i, r := range rows {
		fields[i] = toCustomField(r)
	}
	return fields, nil
}

func toCustomField(r sqlc.CustomField) administration.CustomField {
	return administration.CustomField{
		ID: resource.ID(r.ID), AdministrationID: resource.ID(r.AdministrationID), Name: r.Name,
		Source: administration.Source(r.Source), Position: int(r.Position), CreatedAt: r.CreatedAt.Time, UpdatedAt: r.UpdatedAt.Time,
	}
}

// CustomField implements administration.Store.
func (s *AdministrationStore) CustomField(ctx context.Context, admin, id resource.ID) (administration.CustomField, error) {
	r, err := s.q.CustomFieldByID(ctx, sqlc.CustomFieldByIDParams{AdministrationID: int64(admin), ID: int64(id)})
	if errors.Is(err, pgx.ErrNoRows) {
		return administration.CustomField{}, administration.ErrCustomFieldNotFound
	}
	if err != nil {
		return administration.CustomField{}, err
	}
	return toCustomField(r), nil
}

// UpdateCustomField implements administration.Store.
func (s *AdministrationStore) UpdateCustomField(ctx context.Context, f administration.CustomField) error {
	return s.q.UpdateCustomField(ctx, sqlc.UpdateCustomFieldParams{
		AdministrationID: int64(f.AdministrationID), ID: int64(f.ID), Name: f.Name, Position: int32(f.Position),
		UpdatedAt: timestamp(f.UpdatedAt),
	})
}

// DeleteCustomField implements administration.Store. Values records hold for
// the field go with it.
func (s *AdministrationStore) DeleteCustomField(ctx context.Context, admin, id resource.ID) error {
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		q := s.q.WithTx(tx)
		n, err := q.DeleteCustomField(ctx, sqlc.DeleteCustomFieldParams{AdministrationID: int64(admin), ID: int64(id)})
		if err != nil {
			return err
		}
		if n == 0 {
			return administration.ErrCustomFieldNotFound
		}
		return q.DeleteResource(ctx, int64(id))
	})
}

// CreateCustomField implements administration.Store.
func (s *AdministrationStore) CreateCustomField(ctx context.Context, f administration.CustomField) error {
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		q := s.q.WithTx(tx)
		if err := q.InsertCustomField(ctx, sqlc.InsertCustomFieldParams{
			ID: int64(f.ID), AdministrationID: int64(f.AdministrationID), Name: f.Name, Source: string(f.Source),
			Position: int32(f.Position), CreatedAt: timestamp(f.CreatedAt), UpdatedAt: timestamp(f.UpdatedAt),
		}); err != nil {
			return err
		}
		return q.InsertResource(ctx, sqlc.InsertResourceParams{
			ID: int64(f.ID), Type: string(resource.TypeCustomField), AdministrationID: int64(f.AdministrationID),
		})
	})
}

// DeleteIdentity implements administration.Store. Document styles that send
// from the identity fall back to the company details.
func (s *AdministrationStore) DeleteIdentity(ctx context.Context, admin, id resource.ID) error {
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		q := s.q.WithTx(tx)
		if err := q.MoveDocumentStylesToDefaultIdentity(ctx, int64(id)); err != nil {
			return err
		}
		n, err := q.DeleteIdentity(ctx, sqlc.DeleteIdentityParams{AdministrationID: int64(admin), ID: int64(id)})
		if err != nil {
			return err
		}
		if n == 0 {
			return administration.ErrIdentityNotFound
		}
		return q.DeleteResource(ctx, int64(id))
	})
}

// ListForUser implements administration.Store.
func (s *AdministrationStore) ListForUser(ctx context.Context, u resource.ID) ([]administration.Listed, error) {
	rows, err := s.q.ListAdministrations(ctx, int64(u))
	if err != nil {
		return nil, err
	}
	listed := make([]administration.Listed, len(rows))
	for i, r := range rows {
		listed[i] = toListed(sqlc.AdministrationForUserRow(r))
	}
	return listed, nil
}

// ForUser implements administration.Store.
func (s *AdministrationStore) ForUser(ctx context.Context, id, u resource.ID) (administration.Listed, error) {
	r, err := s.q.AdministrationForUser(ctx, sqlc.AdministrationForUserParams{ID: int64(id), UserID: int64(u)})
	if errors.Is(err, pgx.ErrNoRows) {
		return s.forSupport(ctx, id, u)
	}
	if err != nil {
		return administration.Listed{}, err
	}
	return toListed(r), nil
}

// forSupport lets an instance admin in while the administration grants
// support access, with every permission but ownership.
func (s *AdministrationStore) forSupport(ctx context.Context, id, u resource.ID) (administration.Listed, error) {
	r, err := s.q.AdministrationForSupport(ctx, sqlc.AdministrationForSupportParams{ID: int64(id), ID_2: int64(u)})
	if errors.Is(err, pgx.ErrNoRows) {
		return administration.Listed{}, administration.ErrNotFound
	}
	if err != nil {
		return administration.Listed{}, err
	}
	l := toListed(sqlc.AdministrationForUserRow{
		ID: r.ID, Name: r.Name, Language: r.Language, Currency: r.Currency, Country: r.Country, TimeZone: r.TimeZone, Suspended: r.Suspended,
		PeriodLockedUntil: r.PeriodLockedUntil, PeriodStartDate: r.PeriodStartDate, SmallBusinessScheme: r.SmallBusinessScheme,
		CompanyActivitiesDescription: r.CompanyActivitiesDescription, CompanyWebsiteUrl: r.CompanyWebsiteUrl, CompanyWebsiteAbsent: r.CompanyWebsiteAbsent,
		RequireTwoFactor: r.RequireTwoFactor, SupportAccessUntil: r.SupportAccessUntil, Access: string(administration.AccessSupport),
		UserType: string(administration.Support),
	})
	for _, p := range administration.Permissions {
		if p != administration.Ownership {
			l.Permissions = append(l.Permissions, p)
		}
	}
	return l, nil
}

// SetFavorite implements administration.Store.
func (s *AdministrationStore) SetFavorite(ctx context.Context, id, u resource.ID, favorite bool) error {
	n, err := s.q.SetFavorite(ctx, sqlc.SetFavoriteParams{AdministrationID: int64(id), UserID: int64(u), Favorite: favorite})
	if err != nil {
		return err
	}
	if n == 0 {
		return administration.ErrNotFound
	}
	return nil
}

func toListed(r sqlc.AdministrationForUserRow) administration.Listed {
	l := administration.Listed{
		Administration: administration.Administration{
			ID: resource.ID(r.ID), Name: r.Name, Language: r.Language, Currency: r.Currency, Country: r.Country,
			TimeZone: r.TimeZone, Suspended: r.Suspended, PeriodStartDate: r.PeriodStartDate.Time.Format(dateLayout),
			Profile: administration.Profile{
				SmallBusinessScheme: r.SmallBusinessScheme, CompanyActivitiesDescription: r.CompanyActivitiesDescription,
				CompanyWebsiteURL: r.CompanyWebsiteUrl, CompanyWebsiteAbsent: r.CompanyWebsiteAbsent,
			},
		},
		Membership: administration.Membership{
			Access: administration.Access(r.Access), Favorite: r.Favorite, UserType: administration.UserType(r.UserType),
			Permissions: permissionsOf(r.Permissions),
		},
	}
	l.RequireTwoFactor = r.RequireTwoFactor
	l.SupportAccessUntil = timeOf(r.SupportAccessUntil)
	if r.PeriodLockedUntil.Valid {
		locked := r.PeriodLockedUntil.Time.Format(dateLayout)
		l.PeriodLockedUntil = &locked
	}
	return l
}

func permissionsOf(raw []string) []administration.Permission {
	out := make([]administration.Permission, len(raw))
	for i, p := range raw {
		out[i] = administration.Permission(p)
	}
	return out
}

func parseDate(s string) (pgtype.Date, error) {
	t, err := time.Parse(dateLayout, s)
	if err != nil {
		return pgtype.Date{}, err
	}
	return pgtype.Date{Time: t, Valid: true}, nil
}

// ResourceLocator finds resources by ID.
type ResourceLocator struct {
	q *sqlc.Queries
}

// NewResourceLocator returns a ResourceLocator on pool.
func NewResourceLocator(pool *pgxpool.Pool) *ResourceLocator {
	return &ResourceLocator{q: sqlc.New(pool)}
}

// Locate returns where a resource lives, or resource.ErrNotFound if it does
// not exist or user has no access to its administration.
func (l *ResourceLocator) Locate(ctx context.Context, id, u resource.ID) (resource.Ref, error) {
	r, err := l.q.LocateResource(ctx, sqlc.LocateResourceParams{ID: int64(id), UserID: int64(u)})
	if errors.Is(err, pgx.ErrNoRows) {
		return resource.Ref{}, resource.ErrNotFound
	}
	if err != nil {
		return resource.Ref{}, err
	}
	return resource.Ref{ID: resource.ID(r.ID), Type: resource.Type(r.Type), AdministrationID: resource.ID(r.AdministrationID)}, nil
}

func subFS() fs.FS {
	sub, err := fs.Sub(migrations, "migrations")
	if err != nil {
		panic(fmt.Sprintf("migrations directory: %v", err)) // The embed path is fixed at compile time.
	}
	return sub
}
