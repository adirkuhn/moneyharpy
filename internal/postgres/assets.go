package postgres

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/adirkuhn/moneyharpy/internal/assets"
	"github.com/adirkuhn/moneyharpy/internal/journal"
	"github.com/adirkuhn/moneyharpy/internal/money"
	"github.com/adirkuhn/moneyharpy/internal/postgres/sqlc"
	"github.com/adirkuhn/moneyharpy/internal/resource"
)

// AssetStore implements assets.Store.
type AssetStore struct {
	pool *pgxpool.Pool
	q    *sqlc.Queries
}

// NewAssetStore returns an AssetStore on pool.
func NewAssetStore(pool *pgxpool.Pool) *AssetStore {
	return &AssetStore{pool: pool, q: sqlc.New(pool)}
}

func optionalAmount(a *money.Amount) pgtype.Int8 {
	if a == nil {
		return pgtype.Int8{}
	}
	return pgtype.Int8{Int64: int64(*a), Valid: true}
}

func toAsset(r sqlc.Asset) assets.Asset {
	a := assets.Asset{
		ID: resource.ID(r.ID), AdministrationID: resource.ID(r.AdministrationID), LedgerAccountID: resource.ID(r.LedgerAccountID), Name: r.Name,
		PurchaseDate: dayOf(r.PurchaseDate), PurchaseValue: money.Amount(r.PurchaseValue), CreatedAt: r.CreatedAt.Time, UpdatedAt: r.UpdatedAt.Time,
	}
	if r.LifespanMonths.Valid {
		a.Plan = &assets.Plan{LifespanMonths: int(r.LifespanMonths.Int32), ResidualValue: money.Amount(r.ResidualValue)}
	}
	if r.DisposalID.Valid {
		a.Disposal = &assets.Disposal{ID: resource.ID(r.DisposalID.Int64), Date: dayOf(r.DisposalDate), Reason: r.DisposalReason.String, ExternallyBooked: r.DisposalExternallyBooked}
		if r.ReserveSaleID.Valid {
			a.Disposal.ReserveSale = &assets.ReserveBooking{ID: resource.ID(r.ReserveSaleID.Int64), Amount: money.Amount(r.ReserveSaleAmount.Int64)}
		}
	}
	if r.ReservePurchaseID.Valid {
		a.ReservePurchase = &assets.ReserveBooking{ID: resource.ID(r.ReservePurchaseID.Int64), Amount: money.Amount(r.ReservePurchaseAmount.Int64)}
	}
	return a
}

// complete adds the value changes and sources to the assets.
func (s *AssetStore) complete(ctx context.Context, admin resource.ID, list []assets.Asset) error {
	if len(list) == 0 {
		return nil
	}
	ids := make([]int64, len(list))
	at := map[int64]*assets.Asset{}
	for i := range list {
		ids[i] = int64(list[i].ID)
		at[ids[i]] = &list[i]
	}
	changes, err := s.q.AssetValueChanges(ctx, sqlc.AssetValueChangesParams{AdministrationID: int64(admin), Ids: ids})
	if err != nil {
		return err
	}
	for _, c := range changes {
		v := assets.ValueChange{
			ID: resource.ID(c.ID), Type: c.Type, Date: dayOf(c.Date), Amount: money.Amount(c.Amount), ExternallyBooked: c.ExternallyBooked, CreatedAt: c.CreatedAt.Time,
		}
		if c.Description.Valid {
			d := c.Description.String
			v.Description = &d
		}
		a := at[c.AssetID]
		a.ValueChanges = append(a.ValueChanges, v)
	}
	sources, err := s.q.AssetSources(ctx, sqlc.AssetSourcesParams{AdministrationID: int64(admin), Ids: ids})
	if err != nil {
		return err
	}
	for _, src := range sources {
		a := at[src.AssetID]
		a.Sources = append(a.Sources, assets.Source{
			ID: resource.ID(src.ID), DetailID: idOf(src.DetailID), EntryID: idOf(src.GeneralJournalDocumentEntryID), CreatedAt: src.CreatedAt.Time,
		})
	}
	return nil
}

// Assets implements assets.Store; a query without PerPage returns them all.
func (s *AssetStore) Assets(ctx context.Context, admin resource.ID, q assets.Query) ([]assets.Asset, int, error) {
	p := sqlc.ListAssetsParams{AdministrationID: int64(admin), LedgerAccountID: optionalID(q.LedgerAccountID), OnlyActive: q.OnlyActive}
	p.MaxRows, p.Skip = limit(q.Page, q.PerPage)
	rows, err := s.q.ListAssets(ctx, p)
	if err != nil {
		return nil, 0, err
	}
	out := make([]assets.Asset, len(rows))
	total := 0
	for i, r := range rows {
		out[i], total = toAsset(r.Asset), int(r.Total)
	}
	if len(rows) == 0 && p.Skip > 0 {
		p.MaxRows, p.Skip = pgtype.Int4{Int32: 1, Valid: true}, 0
		if all, err := s.q.ListAssets(ctx, p); err == nil && len(all) > 0 {
			total = int(all[0].Total)
		}
	}
	return out, total, s.complete(ctx, admin, out)
}

// Asset implements assets.Store.
func (s *AssetStore) Asset(ctx context.Context, admin, id resource.ID) (assets.Asset, error) {
	r, err := s.q.AssetByID(ctx, sqlc.AssetByIDParams{AdministrationID: int64(admin), ID: int64(id)})
	if errors.Is(err, pgx.ErrNoRows) {
		return assets.Asset{}, assets.ErrNotFound
	}
	if err != nil {
		return assets.Asset{}, err
	}
	list := []assets.Asset{toAsset(r)}
	return list[0], s.complete(ctx, admin, list)
}

// Save implements assets.Store.
func (s *AssetStore) Save(ctx context.Context, a assets.Asset, created bool, entries []journal.Entry) error {
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		q := s.q.WithTx(tx)
		admin := int64(a.AdministrationID)
		p := sqlc.UpsertAssetParams{
			ID: int64(a.ID), AdministrationID: admin, LedgerAccountID: int64(a.LedgerAccountID), Name: a.Name, PurchaseDate: day(a.PurchaseDate),
			PurchaseValue: int64(a.PurchaseValue), CreatedAt: timestamp(a.CreatedAt), UpdatedAt: timestamp(a.UpdatedAt),
		}
		if a.Plan != nil {
			p.LifespanMonths = pgtype.Int4{Int32: int32(a.Plan.LifespanMonths), Valid: true}
			p.ResidualValue = int64(a.Plan.ResidualValue)
		}
		if d := a.Disposal; d != nil {
			id := d.ID
			p.DisposalID, p.DisposalDate, p.DisposalReason, p.DisposalExternallyBooked = optionalID(&id), day(d.Date), optionalText(d.Reason), d.ExternallyBooked
			if r := d.ReserveSale; r != nil {
				p.ReserveSaleID, p.ReserveSaleAmount = optionalID(&r.ID), optionalAmount(&r.Amount)
			}
		}
		if r := a.ReservePurchase; r != nil {
			p.ReservePurchaseID, p.ReservePurchaseAmount = optionalID(&r.ID), optionalAmount(&r.Amount)
		}
		if err := q.UpsertAsset(ctx, p); err != nil {
			return err
		}
		if created {
			if err := q.InsertResource(ctx, sqlc.InsertResourceParams{ID: int64(a.ID), Type: string(resource.TypeAsset), AdministrationID: admin}); err != nil {
				return err
			}
		}
		if err := q.DeleteAssetValueChanges(ctx, sqlc.DeleteAssetValueChangesParams{AdministrationID: admin, AssetID: int64(a.ID)}); err != nil {
			return err
		}
		for _, c := range a.ValueChanges {
			description := pgtype.Text{}
			if c.Description != nil {
				description = pgtype.Text{String: *c.Description, Valid: true}
			}
			if err := q.InsertAssetValueChange(ctx, sqlc.InsertAssetValueChangeParams{
				ID: int64(c.ID), AdministrationID: admin, AssetID: int64(a.ID), Type: c.Type, Date: day(c.Date), Amount: int64(c.Amount),
				Description: description, ExternallyBooked: c.ExternallyBooked, CreatedAt: timestamp(c.CreatedAt),
			}); err != nil {
				return err
			}
		}
		if err := q.DeleteAssetSources(ctx, sqlc.DeleteAssetSourcesParams{AdministrationID: admin, AssetID: int64(a.ID)}); err != nil {
			return err
		}
		for _, src := range a.Sources {
			if err := q.InsertAssetSource(ctx, sqlc.InsertAssetSourceParams{
				ID: int64(src.ID), AdministrationID: admin, AssetID: int64(a.ID), DetailID: optionalID(src.DetailID),
				GeneralJournalDocumentEntryID: optionalID(src.EntryID), CreatedAt: timestamp(src.CreatedAt),
			}); err != nil {
				return err
			}
		}
		return postEntries(ctx, q, a.AdministrationID, assets.DocumentType, a.ID, entries, a.UpdatedAt)
	})
}

// Delete implements assets.Store.
func (s *AssetStore) Delete(ctx context.Context, admin, id resource.ID) error {
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		q := s.q.WithTx(tx)
		if err := q.DeleteJournalEntriesOf(ctx, sqlc.DeleteJournalEntriesOfParams{AdministrationID: int64(admin), DocumentType: assets.DocumentType, DocumentID: int64(id)}); err != nil {
			return err
		}
		if err := q.DeleteAsset(ctx, sqlc.DeleteAssetParams{AdministrationID: int64(admin), ID: int64(id)}); err != nil {
			return err
		}
		return q.DeleteResource(ctx, int64(id))
	})
}

// ReserveBalance implements assets.Store.
func (s *AssetStore) ReserveBalance(ctx context.Context, admin resource.ID) (money.Amount, error) {
	v, err := s.q.ReinvestmentReserveBalance(ctx, int64(admin))
	return money.Amount(v), err
}

// DetailLedgerAccount implements assets.Store.
func (s *AssetStore) DetailLedgerAccount(ctx context.Context, admin, id resource.ID) (*resource.ID, error) {
	v, err := s.q.PurchaseDetailLedgerAccount(ctx, sqlc.PurchaseDetailLedgerAccountParams{AdministrationID: int64(admin), ID: int64(id)})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, assets.ErrBookingNotFound
	}
	return idOf(v), err
}

// EntryLedgerAccount implements assets.Store.
func (s *AssetStore) EntryLedgerAccount(ctx context.Context, admin, id resource.ID) (resource.ID, error) {
	v, err := s.q.JournalDocumentEntryLedgerAccount(ctx, sqlc.JournalDocumentEntryLedgerAccountParams{AdministrationID: int64(admin), ID: int64(id)})
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, assets.ErrBookingNotFound
	}
	return resource.ID(v), err
}

// Workload implements assets.Store.
func (s *AssetStore) Workload(ctx context.Context) ([]assets.Workload, error) {
	rows, err := s.q.AssetWorkload(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]assets.Workload, len(rows))
	for i, r := range rows {
		out[i] = assets.Workload{AdministrationID: resource.ID(r.AdministrationID), UserID: resource.ID(r.UserID)}
	}
	return out, nil
}

// assetLock keeps the advisory locks of the depreciation job apart from any
// other: the administration's id is the second key.
const assetLock = 1519

// Lock implements assets.Store with a session advisory lock, held on its
// own connection until release.
func (s *AssetStore) Lock(ctx context.Context, admin resource.ID) (func(), bool, error) {
	conn, err := s.pool.Acquire(ctx)
	if err != nil {
		return nil, false, err
	}
	key := int64(assetLock)<<32 ^ int64(admin)
	var ok bool
	if err := conn.QueryRow(ctx, "SELECT pg_try_advisory_lock($1)", key).Scan(&ok); err != nil || !ok {
		conn.Release()
		return nil, false, err
	}
	release := func() {
		_, _ = conn.Exec(context.WithoutCancel(ctx), "SELECT pg_advisory_unlock($1)", key)
		conn.Release()
	}
	return release, true, nil
}
