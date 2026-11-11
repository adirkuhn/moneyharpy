package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/adirkuhn/moneyharpy/internal/downloads"
	"github.com/adirkuhn/moneyharpy/internal/money"
	"github.com/adirkuhn/moneyharpy/internal/postgres/sqlc"
	"github.com/adirkuhn/moneyharpy/internal/resource"
	"github.com/adirkuhn/moneyharpy/internal/subscriptions"
)

// SubscriptionStore implements subscriptions.Store and downloads.Store.
type SubscriptionStore struct {
	pool *pgxpool.Pool
	q    *sqlc.Queries
}

// NewSubscriptionStore returns a SubscriptionStore on pool.
func NewSubscriptionStore(pool *pgxpool.Pool) *SubscriptionStore {
	return &SubscriptionStore{pool: pool, q: sqlc.New(pool)}
}

func dateValue(t time.Time) pgtype.Date { return pgtype.Date{Time: t, Valid: true} }

// complete loads the subscriptions' products.
func (s *SubscriptionStore) complete(ctx context.Context, rows []sqlc.Subscription) ([]subscriptions.Subscription, error) {
	ids := make([]int64, len(rows))
	for i, r := range rows {
		ids[i] = r.ID
	}
	items, err := s.q.ListSubscriptionProducts(ctx, ids)
	if err != nil {
		return nil, err
	}
	byID := map[int64][]subscriptions.Item{}
	for _, it := range items {
		byID[it.SubscriptionID] = append(byID[it.SubscriptionID], subscriptions.Item{
			ID: resource.ID(it.ID), ProductID: resource.ID(it.ProductID), StartDate: *dateOf(it.StartDate), EndDate: dateOf(it.EndDate),
			Amount: it.Amount, Quantity: it.Quantity, Discount: int64(it.Discount),
		})
	}
	out := make([]subscriptions.Subscription, len(rows))
	for i, r := range rows {
		out[i] = subscriptions.Subscription{
			ID: resource.ID(r.ID), AdministrationID: resource.ID(r.AdministrationID), ContactID: resource.ID(r.ContactID),
			ContactPersonID: idOf(r.ContactPersonID), RecurringID: idOf(r.RecurringSalesInvoiceID), StartDate: *dateOf(r.StartDate),
			EndDate: dateOf(r.EndDate), Frequency: int(r.FrequencyCount), FrequencyType: r.FrequencyType, Reference: r.Reference,
			CancelledAt: timeOf(r.CancelledAt), Items: byID[r.ID], CreatedAt: r.CreatedAt.Time, UpdatedAt: r.UpdatedAt.Time,
		}
	}
	return out, nil
}

func (s *SubscriptionStore) one(ctx context.Context, r sqlc.Subscription, err error) (subscriptions.Subscription, error) {
	if errors.Is(err, pgx.ErrNoRows) {
		return subscriptions.Subscription{}, subscriptions.ErrNotFound
	}
	if err != nil {
		return subscriptions.Subscription{}, err
	}
	list, err := s.complete(ctx, []sqlc.Subscription{r})
	if err != nil {
		return subscriptions.Subscription{}, err
	}
	return list[0], nil
}

// Subscriptions implements subscriptions.Store.
func (s *SubscriptionStore) Subscriptions(ctx context.Context, admin, contact resource.ID) ([]subscriptions.Subscription, error) {
	rows, err := s.q.ListContactSubscriptions(ctx, sqlc.ListContactSubscriptionsParams{AdministrationID: int64(admin), ContactID: int64(contact)})
	if err != nil {
		return nil, err
	}
	return s.complete(ctx, rows)
}

// Subscription implements subscriptions.Store.
func (s *SubscriptionStore) Subscription(ctx context.Context, admin, id resource.ID) (subscriptions.Subscription, error) {
	r, err := s.q.SubscriptionByID(ctx, sqlc.SubscriptionByIDParams{AdministrationID: int64(admin), ID: int64(id)})
	return s.one(ctx, r, err)
}

// SubscriptionByRecurring implements subscriptions.Store.
func (s *SubscriptionStore) SubscriptionByRecurring(ctx context.Context, admin, recurring resource.ID) (subscriptions.Subscription, error) {
	r, err := s.q.SubscriptionByRecurring(ctx, sqlc.SubscriptionByRecurringParams{
		AdministrationID: int64(admin), RecurringSalesInvoiceID: pgtype.Int8{Int64: int64(recurring), Valid: true},
	})
	return s.one(ctx, r, err)
}

// SaveSubscription implements subscriptions.Store.
func (s *SubscriptionStore) SaveSubscription(ctx context.Context, sub subscriptions.Subscription, created bool) error {
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		q := s.q.WithTx(tx)
		if err := q.UpsertSubscription(ctx, sqlc.UpsertSubscriptionParams{
			ID: int64(sub.ID), AdministrationID: int64(sub.AdministrationID), ContactID: int64(sub.ContactID), ContactPersonID: optionalID(sub.ContactPersonID),
			RecurringSalesInvoiceID: optionalID(sub.RecurringID), StartDate: dateValue(sub.StartDate), EndDate: optionalDate(sub.EndDate),
			FrequencyCount: int32(sub.Frequency), FrequencyType: sub.FrequencyType, Reference: sub.Reference, CancelledAt: optionalTime(sub.CancelledAt),
			CreatedAt: timestamp(sub.CreatedAt), UpdatedAt: timestamp(sub.UpdatedAt),
		}); err != nil {
			return err
		}
		if err := q.DeleteSubscriptionProducts(ctx, int64(sub.ID)); err != nil {
			return err
		}
		for _, it := range sub.Items {
			if err := q.InsertSubscriptionProduct(ctx, sqlc.InsertSubscriptionProductParams{
				ID: int64(it.ID), SubscriptionID: int64(sub.ID), ProductID: int64(it.ProductID), StartDate: dateValue(it.StartDate),
				EndDate: optionalDate(it.EndDate), Amount: it.Amount, Quantity: it.Quantity, Discount: int32(it.Discount),
			}); err != nil {
				return err
			}
		}
		if !created {
			return nil
		}
		return q.InsertResource(ctx, sqlc.InsertResourceParams{ID: int64(sub.ID), Type: string(resource.TypeSubscription), AdministrationID: int64(sub.AdministrationID)})
	})
}

func toCharge(r sqlc.AdditionalCharge) subscriptions.Charge {
	return subscriptions.Charge{
		ID: resource.ID(r.ID), AdministrationID: resource.ID(r.AdministrationID), ContactID: idOf(r.ContactID), SubscriptionID: idOf(r.SubscriptionID),
		ProductID: resource.ID(r.ProductID), Amount: r.Amount, Quantity: r.Quantity, Price: money.Amount(r.Price),
		Period: money.Period{From: *dateOf(r.PeriodFrom), To: *dateOf(r.PeriodTo)}, Description: r.Description, Identifier: r.Identifier,
		InvoiceID: idOf(r.SalesInvoiceID), DetailID: idOf(r.DetailID), CreatedAt: r.CreatedAt.Time, UpdatedAt: r.UpdatedAt.Time,
	}
}

func toCharges(rows []sqlc.AdditionalCharge) []subscriptions.Charge {
	out := make([]subscriptions.Charge, len(rows))
	for i, r := range rows {
		out[i] = toCharge(r)
	}
	return out
}

// Charges implements subscriptions.Store.
func (s *SubscriptionStore) Charges(ctx context.Context, admin resource.ID, q subscriptions.ChargeQuery) ([]subscriptions.Charge, error) {
	rows, err := s.q.ListAdditionalCharges(ctx, sqlc.ListAdditionalChargesParams{
		AdministrationID: int64(admin), ContactID: optionalID(q.ContactID), SubscriptionID: optionalID(q.SubscriptionID), IncludeBilled: q.IncludeBilled,
	})
	return toCharges(rows), err
}

// SaveCharge implements subscriptions.Store.
func (s *SubscriptionStore) SaveCharge(ctx context.Context, c subscriptions.Charge) error {
	return s.q.InsertAdditionalCharge(ctx, sqlc.InsertAdditionalChargeParams{
		ID: int64(c.ID), AdministrationID: int64(c.AdministrationID), ContactID: optionalID(c.ContactID), SubscriptionID: optionalID(c.SubscriptionID),
		ProductID: int64(c.ProductID), Amount: c.Amount, Quantity: c.Quantity, Price: int64(c.Price), PeriodFrom: dateValue(c.Period.From),
		PeriodTo: dateValue(c.Period.To), Description: c.Description, Identifier: c.Identifier, CreatedAt: timestamp(c.CreatedAt), UpdatedAt: timestamp(c.UpdatedAt),
	})
}

// BillCharges implements subscriptions.Store.
func (s *SubscriptionStore) BillCharges(ctx context.Context, admin resource.ID, billed []subscriptions.Charge) error {
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		q := s.q.WithTx(tx)
		now := timestamp(time.Now())
		for _, c := range billed {
			if err := q.BillAdditionalCharge(ctx, sqlc.BillAdditionalChargeParams{
				AdministrationID: int64(admin), ID: int64(c.ID), SalesInvoiceID: optionalID(c.InvoiceID), DetailID: optionalID(c.DetailID), UpdatedAt: now,
			}); err != nil {
				return err
			}
		}
		return nil
	})
}

// UnbilledContactCharges implements subscriptions.Store.
func (s *SubscriptionStore) UnbilledContactCharges(ctx context.Context, admin resource.ID) ([]subscriptions.Charge, error) {
	rows, err := s.q.UnbilledContactCharges(ctx, int64(admin))
	return toCharges(rows), err
}

// ChargeWorkload implements subscriptions.Store.
func (s *SubscriptionStore) ChargeWorkload(ctx context.Context) ([]subscriptions.Workload, error) {
	rows, err := s.q.ChargeWorkload(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]subscriptions.Workload, len(rows))
	for i, r := range rows {
		out[i] = subscriptions.Workload{AdministrationID: resource.ID(r.AdministrationID), UserID: resource.ID(r.UserID)}
	}
	return out, nil
}

// chargeLock keeps the advisory locks of charge billing apart from any
// other: the administration's id is the second key.
const chargeLock = 1518

// Lock implements subscriptions.Store.
func (s *SubscriptionStore) Lock(ctx context.Context, admin resource.ID) (func(), bool, error) {
	conn, err := s.pool.Acquire(ctx)
	if err != nil {
		return nil, false, err
	}
	key := int64(chargeLock)<<32 ^ int64(admin)
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

// OneOffInvoices implements subscriptions.Store.
func (s *SubscriptionStore) OneOffInvoices(ctx context.Context, admin, subscription resource.ID) ([]resource.ID, error) {
	rows, err := s.q.ListOneOffInvoices(ctx, sqlc.ListOneOffInvoicesParams{AdministrationID: int64(admin), SubscriptionID: int64(subscription)})
	out := make([]resource.ID, len(rows))
	for i, id := range rows {
		out[i] = resource.ID(id)
	}
	return out, err
}

// AddOneOffInvoice implements subscriptions.Store.
func (s *SubscriptionStore) AddOneOffInvoice(ctx context.Context, admin, subscription, invoice resource.ID) error {
	return s.q.InsertOneOffInvoice(ctx, sqlc.InsertOneOffInvoiceParams{SalesInvoiceID: int64(invoice), SubscriptionID: int64(subscription), AdministrationID: int64(admin)})
}

// ProductInUse implements products.References for subscriptions and
// additional charges.
func (s *SubscriptionStore) ProductInUse(ctx context.Context, admin, product resource.ID) (bool, error) {
	return s.q.ProductInSubscriptions(ctx, sqlc.ProductInSubscriptionsParams{AdministrationID: int64(admin), ProductID: int64(product)})
}

func toDownload(r sqlc.Download) downloads.Download {
	return downloads.Download{
		ID: resource.ID(r.ID), AdministrationID: resource.ID(r.AdministrationID), UserID: idOf(r.UserID), Type: r.DownloadType, Filename: r.Filename,
		ContentType: r.ContentType, Size: r.Size, Failed: r.Failed, Downloaded: r.Downloaded, CreatedAt: r.CreatedAt.Time, UpdatedAt: r.UpdatedAt.Time,
	}
}

// Downloads implements downloads.Store.
func (s *SubscriptionStore) Downloads(ctx context.Context, admin resource.ID, q downloads.Query) ([]downloads.Download, int, error) {
	p := sqlc.ListDownloadsParams{
		AdministrationID: int64(admin), Types: nonNil(q.Types), DownloadType: q.Type, Downloaded: optionalBool(q.Downloaded), Failed: optionalBool(q.Failed),
	}
	p.MaxRows, p.Skip = limit(q.Page, q.PerPage)
	rows, err := s.q.ListDownloads(ctx, p)
	if err != nil {
		return nil, 0, err
	}
	out := make([]downloads.Download, len(rows))
	total := 0
	for i, r := range rows {
		out[i], total = toDownload(r.Download), int(r.Total)
	}
	if len(rows) == 0 && p.Skip > 0 {
		p.MaxRows, p.Skip = pgtype.Int4{Int32: 1, Valid: true}, 0
		if all, err := s.q.ListDownloads(ctx, p); err == nil && len(all) > 0 {
			total = int(all[0].Total)
		}
	}
	return out, total, nil
}

// Download implements downloads.Store.
func (s *SubscriptionStore) Download(ctx context.Context, admin, id resource.ID) (downloads.Download, error) {
	r, err := s.q.DownloadByID(ctx, sqlc.DownloadByIDParams{AdministrationID: int64(admin), ID: int64(id)})
	if errors.Is(err, pgx.ErrNoRows) {
		return downloads.Download{}, downloads.ErrNotFound
	}
	if err != nil {
		return downloads.Download{}, err
	}
	return toDownload(r), nil
}

// SaveDownload implements downloads.Store.
func (s *SubscriptionStore) SaveDownload(ctx context.Context, d downloads.Download) error {
	return s.q.UpsertDownload(ctx, sqlc.UpsertDownloadParams{
		ID: int64(d.ID), AdministrationID: int64(d.AdministrationID), UserID: optionalID(d.UserID), DownloadType: d.Type, Filename: d.Filename,
		ContentType: d.ContentType, Size: d.Size, Failed: d.Failed, Downloaded: d.Downloaded, CreatedAt: timestamp(d.CreatedAt), UpdatedAt: timestamp(d.UpdatedAt),
	})
}
