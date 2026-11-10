package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/adirkuhn/moneyharpy/internal/connections"
	"github.com/adirkuhn/moneyharpy/internal/postgres/sqlc"
	"github.com/adirkuhn/moneyharpy/internal/resource"
	"github.com/adirkuhn/moneyharpy/internal/webhooks"
)

// ConnectionStore implements connections.Store.
type ConnectionStore struct{ q *sqlc.Queries }

// NewConnectionStore returns a ConnectionStore on pool.
func NewConnectionStore(pool *pgxpool.Pool) *ConnectionStore {
	return &ConnectionStore{q: sqlc.New(pool)}
}

func toConnection(r sqlc.ListConnectionsRow) connections.Connection {
	return connections.Connection{
		ID: resource.ID(r.ID), AdministrationID: resource.ID(r.AdministrationID), UserID: resource.ID(r.UserID), GrantedBy: r.GrantedBy,
		Kind: r.Kind, Name: r.Name, Scopes: permissionsOf(r.Scopes), ReadOnly: r.ReadOnly, CreatedAt: r.CreatedAt.Time, LastUsedAt: timeOf(r.LastUsedAt),
	}
}

// Connections implements connections.Store.
func (s *ConnectionStore) Connections(ctx context.Context, admin resource.ID) ([]connections.Connection, error) {
	rows, err := s.q.ListConnections(ctx, int64(admin))
	if err != nil {
		return nil, err
	}
	out := make([]connections.Connection, len(rows))
	for i, r := range rows {
		out[i] = toConnection(r)
	}
	return out, nil
}

// Connection implements connections.Store.
func (s *ConnectionStore) Connection(ctx context.Context, admin, id resource.ID) (connections.Connection, error) {
	r, err := s.q.ConnectionByID(ctx, sqlc.ConnectionByIDParams{AdministrationID: int64(admin), ID: int64(id)})
	if errors.Is(err, pgx.ErrNoRows) {
		return connections.Connection{}, connections.ErrNotFound
	}
	if err != nil {
		return connections.Connection{}, err
	}
	return toConnection(sqlc.ListConnectionsRow(r)), nil
}

// CreateConnection implements connections.Store.
func (s *ConnectionStore) CreateConnection(ctx context.Context, c connections.Connection, tokenHash string) error {
	return s.q.InsertConnection(ctx, sqlc.InsertConnectionParams{
		ID: int64(c.ID), AdministrationID: int64(c.AdministrationID), UserID: int64(c.UserID), Kind: c.Kind, Name: c.Name,
		Scopes: permissionStrings(c.Scopes), ReadOnly: c.ReadOnly, TokenHash: tokenHash, CreatedAt: timestamp(c.CreatedAt),
	})
}

// DeleteConnection implements connections.Store. Its webhooks go with it.
func (s *ConnectionStore) DeleteConnection(ctx context.Context, admin, id resource.ID) error {
	return s.q.DeleteConnection(ctx, sqlc.DeleteConnectionParams{AdministrationID: int64(admin), ID: int64(id)})
}

// GrantByToken implements connections.Store.
func (s *ConnectionStore) GrantByToken(ctx context.Context, tokenHash string, now time.Time) (connections.Grant, error) {
	r, err := s.q.UseConnection(ctx, sqlc.UseConnectionParams{TokenHash: tokenHash, LastUsedAt: timestamp(now)})
	if errors.Is(err, pgx.ErrNoRows) {
		return connections.Grant{}, connections.ErrInvalidToken
	}
	if err != nil {
		return connections.Grant{}, err
	}
	c := connections.Connection{
		ID: resource.ID(r.ID), AdministrationID: resource.ID(r.AdministrationID), UserID: resource.ID(r.UserID), GrantedBy: r.GrantedBy,
		Kind: r.Kind, Name: r.Name, Scopes: permissionsOf(r.Scopes), ReadOnly: r.ReadOnly, CreatedAt: r.CreatedAt.Time, LastUsedAt: timeOf(r.LastUsedAt),
	}
	return connections.Grant{Connection: c, UserEmail: r.Email, UserName: r.GrantedBy}, nil
}

// UsersMayConnect implements connections.Store.
func (s *ConnectionStore) UsersMayConnect(ctx context.Context, admin resource.ID) (bool, error) {
	return s.q.UsersMayConnect(ctx, int64(admin))
}

// SetUsersMayConnect implements connections.Store.
func (s *ConnectionStore) SetUsersMayConnect(ctx context.Context, admin resource.ID, on bool) error {
	return s.q.SetUsersMayConnect(ctx, sqlc.SetUsersMayConnectParams{ID: int64(admin), UsersMayConnect: on})
}

// WebhookStore implements webhooks.Store. Secrets are sealed at rest.
type WebhookStore struct {
	pool   *pgxpool.Pool
	q      *sqlc.Queries
	sealer interface {
		Seal(string) (string, error)
		Open(string) (string, error)
	}
}

// NewWebhookStore returns a WebhookStore on pool sealing secrets with sealer.
func NewWebhookStore(pool *pgxpool.Pool, sealer interface {
	Seal(string) (string, error)
	Open(string) (string, error)
}) *WebhookStore {
	return &WebhookStore{pool: pool, q: sqlc.New(pool), sealer: sealer}
}

func toWebhook(r sqlc.Webhook) webhooks.Webhook {
	w := webhooks.Webhook{
		ID: resource.ID(r.ID), AdministrationID: resource.ID(r.AdministrationID), URL: r.Url, Events: nonNil(r.EnabledEvents), Token: r.Token,
		DeactivatedAt: timeOf(r.DeactivatedAt), CreatedAt: r.CreatedAt.Time, UpdatedAt: r.UpdatedAt.Time,
	}
	if r.ConnectionID.Valid {
		id := resource.ID(r.ConnectionID.Int64)
		w.ConnectionID = &id
	}
	if r.LastHttpStatus.Valid {
		st := int(r.LastHttpStatus.Int32)
		w.LastHTTPStatus = &st
	}
	if r.LastHttpBody.Valid {
		w.LastHTTPBody = &r.LastHttpBody.String
	}
	return w
}

// Webhooks implements webhooks.Store.
func (s *WebhookStore) Webhooks(ctx context.Context, admin resource.ID) ([]webhooks.Webhook, error) {
	rows, err := s.q.ListWebhooks(ctx, int64(admin))
	if err != nil {
		return nil, err
	}
	out := make([]webhooks.Webhook, len(rows))
	for i, r := range rows {
		out[i] = toWebhook(r)
	}
	return out, nil
}

// Webhook implements webhooks.Store.
func (s *WebhookStore) Webhook(ctx context.Context, admin, id resource.ID) (webhooks.Webhook, error) {
	r, err := s.q.WebhookByID(ctx, sqlc.WebhookByIDParams{AdministrationID: int64(admin), ID: int64(id)})
	if errors.Is(err, pgx.ErrNoRows) {
		return webhooks.Webhook{}, webhooks.ErrNotFound
	}
	if err != nil {
		return webhooks.Webhook{}, err
	}
	return toWebhook(r), nil
}

// CreateWebhook implements webhooks.Store.
func (s *WebhookStore) CreateWebhook(ctx context.Context, w webhooks.Webhook) error {
	sealed, err := s.sealer.Seal(w.Secret)
	if err != nil {
		return err
	}
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		q := s.q.WithTx(tx)
		if err := q.InsertWebhook(ctx, sqlc.InsertWebhookParams{
			ID: int64(w.ID), AdministrationID: int64(w.AdministrationID), Url: w.URL, EnabledEvents: nonNil(w.Events), Token: w.Token,
			SecretSealed: sealed, ConnectionID: optionalID(w.ConnectionID), CreatedAt: timestamp(w.CreatedAt), UpdatedAt: timestamp(w.UpdatedAt),
		}); err != nil {
			return err
		}
		return q.InsertResource(ctx, sqlc.InsertResourceParams{ID: int64(w.ID), Type: string(resource.TypeWebhook), AdministrationID: int64(w.AdministrationID)})
	})
}

// DeleteWebhook implements webhooks.Store.
func (s *WebhookStore) DeleteWebhook(ctx context.Context, admin, id resource.ID) error {
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		q := s.q.WithTx(tx)
		if err := q.DeleteWebhook(ctx, sqlc.DeleteWebhookParams{AdministrationID: int64(admin), ID: int64(id)}); err != nil {
			return err
		}
		return q.DeleteResource(ctx, int64(id))
	})
}

// SetDeactivated implements webhooks.Store.
func (s *WebhookStore) SetDeactivated(ctx context.Context, admin, id resource.ID, at *time.Time) error {
	return s.q.SetWebhookDeactivated(ctx, sqlc.SetWebhookDeactivatedParams{AdministrationID: int64(admin), ID: int64(id), DeactivatedAt: optionalTime(at)})
}

// Enqueue implements webhooks.Store.
func (s *WebhookStore) Enqueue(ctx context.Context, deliveries []webhooks.Delivery) error {
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		q := s.q.WithTx(tx)
		for _, d := range deliveries {
			if err := q.InsertWebhookDelivery(ctx, sqlc.InsertWebhookDeliveryParams{
				ID: int64(d.ID), WebhookID: int64(d.WebhookID), IdempotencyKey: d.IdempotencyKey, Payload: d.Payload,
				NextAttemptAt: optionalTime(d.NextAttemptAt), CreatedAt: timestamp(d.CreatedAt),
			}); err != nil {
				return err
			}
		}
		return nil
	})
}

// Due implements webhooks.Store.
func (s *WebhookStore) Due(ctx context.Context, now, lease time.Time, limit int) ([]webhooks.Delivery, error) {
	rows, err := s.q.LeaseDueDeliveries(ctx, sqlc.LeaseDueDeliveriesParams{Lease: timestamp(lease), Now: timestamp(now), MaxCount: int32(limit)})
	if err != nil {
		return nil, err
	}
	out := make([]webhooks.Delivery, 0, len(rows))
	for _, r := range rows {
		secret, err := s.sealer.Open(r.SecretSealed)
		if err != nil {
			return nil, err
		}
		out = append(out, webhooks.Delivery{
			ID: resource.ID(r.ID), WebhookID: resource.ID(r.WebhookID), IdempotencyKey: r.IdempotencyKey, Payload: r.Payload,
			Attempts: int(r.Attempts), CreatedAt: r.CreatedAt.Time, URL: r.Url, Secret: secret,
		})
	}
	return out, nil
}

// SaveAttempt implements webhooks.Store.
func (s *WebhookStore) SaveAttempt(ctx context.Context, d webhooks.Delivery, r *webhooks.Response) error {
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		q := s.q.WithTx(tx)
		var status pgtype.Int4
		if d.LastStatus != nil {
			status = pgtype.Int4{Int32: int32(*d.LastStatus), Valid: true}
		}
		if err := q.SaveDeliveryAttempt(ctx, sqlc.SaveDeliveryAttemptParams{
			ID: int64(d.ID), Attempts: int32(d.Attempts), NextAttemptAt: optionalTime(d.NextAttemptAt), DeliveredAt: optionalTime(d.DeliveredAt),
			LastStatus: status, LastError: d.LastError,
		}); err != nil {
			return err
		}
		if r == nil {
			return nil
		}
		return q.SaveWebhookResponse(ctx, sqlc.SaveWebhookResponseParams{
			ID: int64(d.WebhookID), LastHttpStatus: pgtype.Int4{Int32: int32(r.Status), Valid: true}, LastHttpBody: pgtype.Text{String: r.Body, Valid: true},
		})
	})
}
