package postgres

import (
	"context"
	"errors"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/adirkuhn/moneyharpy/internal/bookkeeping"
	"github.com/adirkuhn/moneyharpy/internal/postgres/sqlc"
	"github.com/adirkuhn/moneyharpy/internal/resource"
)

// BookkeepingStore implements bookkeeping.Store.
type BookkeepingStore struct {
	pool *pgxpool.Pool
	q    *sqlc.Queries
}

// NewBookkeepingStore returns a BookkeepingStore on pool.
func NewBookkeepingStore(pool *pgxpool.Pool) *BookkeepingStore {
	return &BookkeepingStore{pool: pool, q: sqlc.New(pool)}
}

// Settings implements bookkeeping.Store.
func (s *BookkeepingStore) Settings(ctx context.Context, admin resource.ID) (bookkeeping.Settings, error) {
	r, err := s.q.BookkeepingSettings(ctx, int64(admin))
	if err != nil {
		return bookkeeping.Settings{}, err
	}
	st := bookkeeping.Settings{VATPeriod: r.VatDocumentsPeriod, ICPPeriod: r.IcpDeclarationPeriod, Scheme: r.ChartScheme, Seeded: r.BookkeepingSeeded}
	if r.PeriodLockedUntil.Valid {
		day := r.PeriodLockedUntil.Time.Format(dateLayout)
		st.LockedUntil = &day
	}
	return st, nil
}

// SaveSettings implements bookkeeping.Store.
func (s *BookkeepingStore) SaveSettings(ctx context.Context, admin resource.ID, st bookkeeping.Settings) error {
	var locked pgtype.Date
	if st.LockedUntil != nil {
		var err error
		if locked, err = parseDate(*st.LockedUntil); err != nil {
			return err
		}
	}
	return s.q.UpdateBookkeepingSettings(ctx, sqlc.UpdateBookkeepingSettingsParams{
		ID: int64(admin), VatDocumentsPeriod: st.VATPeriod, IcpDeclarationPeriod: st.ICPPeriod, ChartScheme: st.Scheme, PeriodLockedUntil: locked,
	})
}

// ClearTaxonomy implements bookkeeping.Store.
func (s *BookkeepingStore) ClearTaxonomy(ctx context.Context, admin resource.ID) error {
	return s.q.ClearLedgerAccountTaxonomy(ctx, int64(admin))
}

// Seed implements bookkeeping.Store.
func (s *BookkeepingStore) Seed(ctx context.Context, admin resource.ID, scheme string, rates []bookkeeping.TaxRate, accounts []bookkeeping.LedgerAccount) (bool, error) {
	seeded := false
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		q := s.q.WithTx(tx)
		n, err := q.MarkBookkeepingSeeded(ctx, sqlc.MarkBookkeepingSeededParams{ID: int64(admin), ChartScheme: scheme})
		if err != nil || n == 0 {
			return err
		}
		seeded = true
		for _, r := range rates {
			if err := insertTaxRate(ctx, q, r); err != nil {
				return err
			}
		}
		return insertLedgerAccounts(ctx, q, accounts)
	})
	return seeded, err
}

// TaxRates implements bookkeeping.Store.
func (s *BookkeepingStore) TaxRates(ctx context.Context, admin resource.ID) ([]bookkeeping.TaxRate, error) {
	rows, err := s.q.ListTaxRates(ctx, int64(admin))
	if err != nil {
		return nil, err
	}
	out := make([]bookkeeping.TaxRate, len(rows))
	for i, r := range rows {
		out[i] = toTaxRate(sqlc.TaxRateByIDRow(r))
	}
	return out, nil
}

// TaxRate implements bookkeeping.Store.
func (s *BookkeepingStore) TaxRate(ctx context.Context, admin, id resource.ID) (bookkeeping.TaxRate, error) {
	r, err := s.q.TaxRateByID(ctx, sqlc.TaxRateByIDParams{AdministrationID: int64(admin), ID: int64(id)})
	if errors.Is(err, pgx.ErrNoRows) {
		return bookkeeping.TaxRate{}, bookkeeping.ErrTaxRateNotFound
	}
	if err != nil {
		return bookkeeping.TaxRate{}, err
	}
	return toTaxRate(r), nil
}

// CreateTaxRate implements bookkeeping.Store.
func (s *BookkeepingStore) CreateTaxRate(ctx context.Context, r bookkeeping.TaxRate) error {
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error { return insertTaxRate(ctx, s.q.WithTx(tx), r) })
}

// UpdateTaxRates implements bookkeeping.Store.
func (s *BookkeepingStore) UpdateTaxRates(ctx context.Context, rates []bookkeeping.TaxRate) error {
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		q := s.q.WithTx(tx)
		for _, r := range rates {
			if err := q.UpdateTaxRate(ctx, sqlc.UpdateTaxRateParams{
				ID: int64(r.ID), AdministrationID: int64(r.AdministrationID), Name: r.Name, Percentage: r.Percentage,
				TaxRateType: r.Type, ShowTax: r.ShowTax, Active: r.Active, Country: r.Country, ReportBoxes: nonNil(r.ReportBoxes),
				Position: int32(r.Position), UpdatedAt: timestamp(r.UpdatedAt),
			}); err != nil {
				return err
			}
		}
		return nil
	})
}

func insertTaxRate(ctx context.Context, q *sqlc.Queries, r bookkeeping.TaxRate) error {
	if err := q.InsertTaxRate(ctx, sqlc.InsertTaxRateParams{
		ID: int64(r.ID), AdministrationID: int64(r.AdministrationID), Name: r.Name, Percentage: r.Percentage,
		TaxRateType: r.Type, ShowTax: r.ShowTax, Active: r.Active, Country: r.Country, ReportBoxes: nonNil(r.ReportBoxes),
		Position: int32(r.Position), CreatedAt: timestamp(r.CreatedAt), UpdatedAt: timestamp(r.UpdatedAt),
	}); err != nil {
		return err
	}
	return q.InsertResource(ctx, sqlc.InsertResourceParams{ID: int64(r.ID), Type: string(resource.TypeTaxRate), AdministrationID: int64(r.AdministrationID)})
}

func toTaxRate(r sqlc.TaxRateByIDRow) bookkeeping.TaxRate {
	return bookkeeping.TaxRate{
		ID: resource.ID(r.ID), AdministrationID: resource.ID(r.AdministrationID), Name: r.Name, Percentage: trimDecimal(r.Percentage),
		Type: r.TaxRateType, ShowTax: r.ShowTax, Active: r.Active, Country: r.Country, ReportBoxes: nonNil(r.ReportBoxes),
		Position: int(r.Position), CreatedAt: r.CreatedAt.Time, UpdatedAt: r.UpdatedAt.Time,
	}
}

// trimDecimal drops trailing zeros: "21.00" becomes "21", "9.50" "9.5".
func trimDecimal(s string) string {
	if strings.Contains(s, ".") {
		s = strings.TrimRight(strings.TrimRight(s, "0"), ".")
	}
	return s
}

func nonNil(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

// LedgerAccounts implements bookkeeping.Store.
func (s *BookkeepingStore) LedgerAccounts(ctx context.Context, admin resource.ID) ([]bookkeeping.LedgerAccount, error) {
	rows, err := s.q.ListLedgerAccounts(ctx, int64(admin))
	if err != nil {
		return nil, err
	}
	out := make([]bookkeeping.LedgerAccount, len(rows))
	for i, r := range rows {
		out[i] = toLedgerAccount(r)
	}
	return out, nil
}

// LedgerAccount implements bookkeeping.Store.
func (s *BookkeepingStore) LedgerAccount(ctx context.Context, admin, id resource.ID) (bookkeeping.LedgerAccount, error) {
	r, err := s.q.LedgerAccountByID(ctx, sqlc.LedgerAccountByIDParams{AdministrationID: int64(admin), ID: int64(id)})
	if errors.Is(err, pgx.ErrNoRows) {
		return bookkeeping.LedgerAccount{}, bookkeeping.ErrLedgerAccountNotFound
	}
	if err != nil {
		return bookkeeping.LedgerAccount{}, err
	}
	return toLedgerAccount(r), nil
}

// CreateLedgerAccounts implements bookkeeping.Store.
func (s *BookkeepingStore) CreateLedgerAccounts(ctx context.Context, accounts []bookkeeping.LedgerAccount) error {
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error { return insertLedgerAccounts(ctx, s.q.WithTx(tx), accounts) })
}

// UpdateLedgerAccounts implements bookkeeping.Store.
func (s *BookkeepingStore) UpdateLedgerAccounts(ctx context.Context, accounts []bookkeeping.LedgerAccount) error {
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		q := s.q.WithTx(tx)
		for _, l := range accounts {
			if err := q.UpdateLedgerAccount(ctx, sqlc.UpdateLedgerAccountParams{
				ID: int64(l.ID), AdministrationID: int64(l.AdministrationID), Name: l.Name, AccountType: l.Type, AccountID: l.Code,
				ParentID: optionalID(l.ParentID), Active: l.Active, AllowedDocumentTypes: nonNil(l.AllowedDocumentTypes),
				TaxonomyCode: l.Taxonomy, Description: l.Description, UpdatedAt: timestamp(l.UpdatedAt),
			}); err != nil {
				return err
			}
		}
		return nil
	})
}

// DeleteLedgerAccount implements bookkeeping.Store.
func (s *BookkeepingStore) DeleteLedgerAccount(ctx context.Context, admin, id resource.ID) error {
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		q := s.q.WithTx(tx)
		if err := q.DeleteLedgerAccount(ctx, sqlc.DeleteLedgerAccountParams{AdministrationID: int64(admin), ID: int64(id)}); err != nil {
			return err
		}
		return q.DeleteResource(ctx, int64(id))
	})
}

func insertLedgerAccounts(ctx context.Context, q *sqlc.Queries, accounts []bookkeeping.LedgerAccount) error {
	for _, l := range accounts {
		if err := q.InsertLedgerAccount(ctx, sqlc.InsertLedgerAccountParams{
			ID: int64(l.ID), AdministrationID: int64(l.AdministrationID), Name: l.Name, AccountType: l.Type, AccountID: l.Code,
			ParentID: optionalID(l.ParentID), Active: l.Active, AllowedDocumentTypes: nonNil(l.AllowedDocumentTypes),
			TaxonomyCode: l.Taxonomy, Description: l.Description, SystemRole: l.System, SeedKey: l.Key, TemplateKey: l.Template,
			CreatedAt: timestamp(l.CreatedAt), UpdatedAt: timestamp(l.UpdatedAt),
		}); err != nil {
			return err
		}
		if err := q.InsertResource(ctx, sqlc.InsertResourceParams{ID: int64(l.ID), Type: string(resource.TypeLedgerAccount), AdministrationID: int64(l.AdministrationID)}); err != nil {
			return err
		}
	}
	return nil
}

func toLedgerAccount(r sqlc.LedgerAccount) bookkeeping.LedgerAccount {
	l := bookkeeping.LedgerAccount{
		ID: resource.ID(r.ID), AdministrationID: resource.ID(r.AdministrationID), Name: r.Name, Type: r.AccountType, Code: r.AccountID,
		Active: r.Active, AllowedDocumentTypes: nonNil(r.AllowedDocumentTypes), Taxonomy: r.TaxonomyCode, Description: r.Description,
		System: r.SystemRole, Key: r.SeedKey, Template: r.TemplateKey, CreatedAt: r.CreatedAt.Time, UpdatedAt: r.UpdatedAt.Time,
	}
	if r.ParentID.Valid {
		p := resource.ID(r.ParentID.Int64)
		l.ParentID = &p
	}
	return l
}

func optionalID(id *resource.ID) pgtype.Int8 {
	if id == nil {
		return pgtype.Int8{}
	}
	return pgtype.Int8{Int64: int64(*id), Valid: true}
}
