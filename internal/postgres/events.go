package postgres

import (
	"context"
	"encoding/json"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/adirkuhn/moneyharpy/internal/history"
	"github.com/adirkuhn/moneyharpy/internal/postgres/sqlc"
	"github.com/adirkuhn/moneyharpy/internal/resource"
)

// EventStore implements history.Store.
type EventStore struct {
	pool *pgxpool.Pool
	q    *sqlc.Queries
}

// NewEventStore returns an EventStore on pool.
func NewEventStore(pool *pgxpool.Pool) *EventStore { return &EventStore{pool: pool, q: sqlc.New(pool)} }

// InsertEvents implements history.Store.
func (s *EventStore) InsertEvents(ctx context.Context, events []history.Event) error {
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		q := s.q.WithTx(tx)
		for _, e := range events {
			data, err := json.Marshal(e.Data)
			if err != nil {
				return err
			}
			if err := q.InsertEvent(ctx, sqlc.InsertEventParams{
				ID: int64(e.ID), AdministrationID: int64(e.AdministrationID), EntityType: e.EntityType, EntityID: int64(e.EntityID),
				UserID: pgtype.Int8{Int64: int64(e.UserID), Valid: e.UserID != 0}, Action: e.Action,
				LinkEntityType: pgtype.Text{String: e.LinkEntityType, Valid: e.LinkEntityType != ""}, LinkEntityID: optionalID(e.LinkEntityID),
				Data: data, CreatedAt: timestamp(e.CreatedAt), UpdatedAt: timestamp(e.UpdatedAt),
			}); err != nil {
				return err
			}
		}
		return nil
	})
}

// EventsOf implements history.Store.
func (s *EventStore) EventsOf(ctx context.Context, admin resource.ID, entityType string, entities []resource.ID) ([]history.Event, error) {
	rows, err := s.q.EventsOf(ctx, sqlc.EventsOfParams{AdministrationID: int64(admin), EntityType: entityType, Ids: ids64(entities)})
	if err != nil {
		return nil, err
	}
	out := make([]history.Event, 0, len(rows))
	for _, r := range rows {
		e := history.Event{
			ID: resource.ID(r.ID), AdministrationID: resource.ID(r.AdministrationID), EntityType: r.EntityType, EntityID: resource.ID(r.EntityID),
			Action: r.Action, LinkEntityType: r.LinkEntityType.String, LinkEntityID: idOf(r.LinkEntityID), Data: map[string]any{},
			CreatedAt: r.CreatedAt.Time, UpdatedAt: r.UpdatedAt.Time,
		}
		if r.UserID.Valid {
			e.UserID = resource.ID(r.UserID.Int64)
		}
		if err := json.Unmarshal(r.Data, &e.Data); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, nil
}
