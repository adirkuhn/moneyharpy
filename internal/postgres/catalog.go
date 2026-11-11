package postgres

import (
	"context"
	"errors"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/adirkuhn/moneyharpy/internal/money"
	"github.com/adirkuhn/moneyharpy/internal/postgres/sqlc"
	"github.com/adirkuhn/moneyharpy/internal/products"
	"github.com/adirkuhn/moneyharpy/internal/projects"
	"github.com/adirkuhn/moneyharpy/internal/resource"
)

// CatalogStore implements products.Store and projects.Store.
type CatalogStore struct {
	pool *pgxpool.Pool
	q    *sqlc.Queries
}

// NewCatalogStore returns a CatalogStore on pool.
func NewCatalogStore(pool *pgxpool.Pool) *CatalogStore {
	return &CatalogStore{pool: pool, q: sqlc.New(pool)}
}

func optionalInt(n *int) pgtype.Int4 {
	if n == nil {
		return pgtype.Int4{}
	}
	return pgtype.Int4{Int32: int32(*n), Valid: true}
}

func intOf(v pgtype.Int4) *int {
	if !v.Valid {
		return nil
	}
	n := int(v.Int32)
	return &n
}

func limit(page, perPage int) (pgtype.Int4, int32) {
	if perPage <= 0 {
		return pgtype.Int4{}, 0
	}
	return pgtype.Int4{Int32: int32(perPage), Valid: true}, int32((max(page, 1) - 1) * perPage)
}

func toProduct(r sqlc.Product) products.Product {
	return products.Product{
		ID: resource.ID(r.ID), AdministrationID: resource.ID(r.AdministrationID), Title: r.Title, Description: r.Description,
		CheckoutDescription: r.CheckoutDescription, Identifier: r.Identifier, Price: money.Amount(r.Price), Currency: r.Currency,
		Frequency: int(r.FrequencyCount), FrequencyType: r.FrequencyType, TaxRateID: idOf(r.TaxRateID), LedgerAccountID: resource.ID(r.LedgerAccountID),
		DocumentStyleID: idOf(r.DocumentStyleID), WorkflowID: idOf(r.WorkflowID), ProductType: r.ProductType, VatRateType: r.VatRateType,
		CheckoutType: r.CheckoutType, MaxAmountPerOrder: intOf(r.MaxAmountPerOrder), Active: r.Active, CreatedAt: r.CreatedAt.Time, UpdatedAt: r.UpdatedAt.Time,
	}
}

// Products implements products.Store.
func (s *CatalogStore) Products(ctx context.Context, admin resource.ID, q products.Query) ([]products.Product, int, error) {
	p := sqlc.ListProductsParams{
		AdministrationID: int64(admin), IncludeInactive: q.IncludeInactive, Currency: q.Currency, CheckoutOnly: q.ForCheckout != nil && *q.ForCheckout,
		LedgerAccountID: optionalID(q.LedgerAccountID), Search: likeEscaper.Replace(strings.TrimSpace(q.Search)),
	}
	p.MaxRows, p.Skip = limit(q.Page, q.PerPage)
	rows, err := s.q.ListProducts(ctx, p)
	if err != nil {
		return nil, 0, err
	}
	out := make([]products.Product, len(rows))
	total := 0
	for i, r := range rows {
		out[i], total = toProduct(r.Product), int(r.Total)
	}
	if len(rows) == 0 && p.Skip > 0 {
		// Past the last page: count without it.
		p.MaxRows, p.Skip = pgtype.Int4{Int32: 1, Valid: true}, 0
		if all, err := s.q.ListProducts(ctx, p); err == nil && len(all) > 0 {
			total = int(all[0].Total)
		}
	}
	return out, total, nil
}

func oneProduct(r sqlc.Product, err error) (products.Product, error) {
	if errors.Is(err, pgx.ErrNoRows) {
		return products.Product{}, products.ErrNotFound
	}
	if err != nil {
		return products.Product{}, err
	}
	return toProduct(r), nil
}

// Product implements products.Store.
func (s *CatalogStore) Product(ctx context.Context, admin, id resource.ID) (products.Product, error) {
	return oneProduct(s.q.ProductByID(ctx, sqlc.ProductByIDParams{AdministrationID: int64(admin), ID: int64(id)}))
}

// ProductByIdentifier implements products.Store.
func (s *CatalogStore) ProductByIdentifier(ctx context.Context, admin resource.ID, identifier string) (products.Product, error) {
	return oneProduct(s.q.ProductByIdentifier(ctx, sqlc.ProductByIdentifierParams{AdministrationID: int64(admin), Identifier: identifier}))
}

// SaveProduct implements products.Store.
func (s *CatalogStore) SaveProduct(ctx context.Context, p products.Product, created bool) error {
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		q := s.q.WithTx(tx)
		if err := q.UpsertProduct(ctx, sqlc.UpsertProductParams{
			ID: int64(p.ID), AdministrationID: int64(p.AdministrationID), Title: p.Title, Description: p.Description,
			CheckoutDescription: p.CheckoutDescription, Identifier: p.Identifier, Price: int64(p.Price), Currency: p.Currency,
			FrequencyCount: int32(p.Frequency), FrequencyType: p.FrequencyType, TaxRateID: optionalID(p.TaxRateID), LedgerAccountID: int64(p.LedgerAccountID),
			DocumentStyleID: optionalID(p.DocumentStyleID), WorkflowID: optionalID(p.WorkflowID), ProductType: p.ProductType, VatRateType: p.VatRateType,
			CheckoutType: p.CheckoutType, MaxAmountPerOrder: optionalInt(p.MaxAmountPerOrder), Active: p.Active,
			CreatedAt: timestamp(p.CreatedAt), UpdatedAt: timestamp(p.UpdatedAt),
		}); err != nil {
			return err
		}
		if !created {
			return nil
		}
		return q.InsertResource(ctx, sqlc.InsertResourceParams{ID: int64(p.ID), Type: string(resource.TypeProduct), AdministrationID: int64(p.AdministrationID)})
	})
}

// DeleteProduct implements products.Store.
func (s *CatalogStore) DeleteProduct(ctx context.Context, admin, id resource.ID) error {
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		q := s.q.WithTx(tx)
		if err := q.DeleteProduct(ctx, sqlc.DeleteProductParams{AdministrationID: int64(admin), ID: int64(id)}); err != nil {
			return err
		}
		return q.DeleteResource(ctx, int64(id))
	})
}

func toProject(r sqlc.Project) projects.Project {
	return projects.Project{
		ID: resource.ID(r.ID), AdministrationID: resource.ID(r.AdministrationID), Name: r.Name, State: r.State, Budget: intOf(r.Budget),
		CreatedAt: r.CreatedAt.Time, UpdatedAt: r.UpdatedAt.Time,
	}
}

// Projects implements projects.Store.
func (s *CatalogStore) Projects(ctx context.Context, admin resource.ID, q projects.Query) ([]projects.Project, int, error) {
	p := sqlc.ListProjectsParams{AdministrationID: int64(admin), States: nonNil(q.States)}
	p.MaxRows, p.Skip = limit(q.Page, q.PerPage)
	rows, err := s.q.ListProjects(ctx, p)
	if err != nil {
		return nil, 0, err
	}
	out := make([]projects.Project, len(rows))
	total := 0
	for i, r := range rows {
		out[i], total = toProject(r.Project), int(r.Total)
	}
	if len(rows) == 0 && p.Skip > 0 {
		p.MaxRows, p.Skip = pgtype.Int4{Int32: 1, Valid: true}, 0
		if all, err := s.q.ListProjects(ctx, p); err == nil && len(all) > 0 {
			total = int(all[0].Total)
		}
	}
	return out, total, nil
}

// Project implements projects.Store.
func (s *CatalogStore) Project(ctx context.Context, admin, id resource.ID) (projects.Project, error) {
	r, err := s.q.ProjectByID(ctx, sqlc.ProjectByIDParams{AdministrationID: int64(admin), ID: int64(id)})
	if errors.Is(err, pgx.ErrNoRows) {
		return projects.Project{}, projects.ErrNotFound
	}
	if err != nil {
		return projects.Project{}, err
	}
	return toProject(r), nil
}

// NameTaken implements projects.Store.
func (s *CatalogStore) NameTaken(ctx context.Context, admin resource.ID, name string, except resource.ID) (bool, error) {
	return s.q.ProjectNameTaken(ctx, sqlc.ProjectNameTakenParams{AdministrationID: int64(admin), Name: name, OtherThan: int64(except)})
}

// SaveProject implements projects.Store.
func (s *CatalogStore) SaveProject(ctx context.Context, p projects.Project, created bool) error {
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		q := s.q.WithTx(tx)
		if err := q.UpsertProject(ctx, sqlc.UpsertProjectParams{
			ID: int64(p.ID), AdministrationID: int64(p.AdministrationID), Name: p.Name, State: p.State, Budget: optionalInt(p.Budget),
			CreatedAt: timestamp(p.CreatedAt), UpdatedAt: timestamp(p.UpdatedAt),
		}); err != nil {
			return err
		}
		if !created {
			return nil
		}
		return q.InsertResource(ctx, sqlc.InsertResourceParams{ID: int64(p.ID), Type: string(resource.TypeProject), AdministrationID: int64(p.AdministrationID)})
	})
}

// DeleteProject implements projects.Store.
func (s *CatalogStore) DeleteProject(ctx context.Context, admin, id resource.ID) error {
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		q := s.q.WithTx(tx)
		if err := q.DeleteProject(ctx, sqlc.DeleteProjectParams{AdministrationID: int64(admin), ID: int64(id)}); err != nil {
			return err
		}
		return q.DeleteResource(ctx, int64(id))
	})
}

// ProductInUse implements products.References: invoice lines keep the
// products they name.
func (s *SalesStore) ProductInUse(ctx context.Context, admin, product resource.ID) (bool, error) {
	used, err := s.q.ProductOnInvoices(ctx, sqlc.ProductOnInvoicesParams{AdministrationID: int64(admin), ProductID: pgtype.Int8{Int64: int64(product), Valid: true}})
	if err != nil || used {
		return used, err
	}
	return s.productOnEstimates(ctx, admin, product)
}

// ProjectInUse implements projects.References for invoice lines.
func (s *SalesStore) ProjectInUse(ctx context.Context, admin, project resource.ID) (bool, error) {
	used, err := s.q.ProjectOnInvoices(ctx, sqlc.ProjectOnInvoicesParams{AdministrationID: int64(admin), ProjectID: pgtype.Int8{Int64: int64(project), Valid: true}})
	if err != nil || used {
		return used, err
	}
	return s.projectOnEstimates(ctx, admin, project)
}

// ProjectInUse implements projects.References for journal entries.
func (s *JournalStore) ProjectInUse(ctx context.Context, admin, project resource.ID) (bool, error) {
	return s.q.ProjectInJournal(ctx, sqlc.ProjectInJournalParams{AdministrationID: int64(admin), ProjectID: pgtype.Int8{Int64: int64(project), Valid: true}})
}
