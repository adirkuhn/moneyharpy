package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/adirkuhn/moneyharpy/internal/banking"
	"github.com/adirkuhn/moneyharpy/internal/postgres/sqlc"
	"github.com/adirkuhn/moneyharpy/internal/resource"
)

// Connections implements banking.ConnectionStore.
func (s *BankingStore) Connections(ctx context.Context, admin resource.ID) ([]banking.Connection, error) {
	rows, err := s.q.ListBankConnections(ctx, int64(admin))
	if err != nil {
		return nil, err
	}
	return s.withAccounts(ctx, rows)
}

// Connection implements banking.ConnectionStore.
func (s *BankingStore) Connection(ctx context.Context, admin, id resource.ID) (banking.Connection, error) {
	r, err := s.q.GetBankConnection(ctx, sqlc.GetBankConnectionParams{AdministrationID: int64(admin), ID: int64(id)})
	return s.oneConnection(ctx, r, err)
}

// ConnectionByAuthState implements banking.ConnectionStore.
func (s *BankingStore) ConnectionByAuthState(ctx context.Context, state string) (banking.Connection, error) {
	r, err := s.q.GetBankConnectionByAuthState(ctx, optionalText(state))
	return s.oneConnection(ctx, r, err)
}

// DueConnections implements banking.ConnectionStore.
func (s *BankingStore) DueConnections(ctx context.Context, before time.Time) ([]banking.Connection, error) {
	rows, err := s.q.DueBankConnections(ctx, timestamp(before))
	if err != nil {
		return nil, err
	}
	return s.withAccounts(ctx, rows)
}

// SaveConnection implements banking.ConnectionStore.
func (s *BankingStore) SaveConnection(ctx context.Context, c banking.Connection) error {
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		q := s.q.WithTx(tx)
		if err := q.UpsertBankConnection(ctx, sqlc.UpsertBankConnectionParams{
			ID: int64(c.ID), AdministrationID: int64(c.AdministrationID), Provider: c.Provider, Bank: c.Bank, Country: c.Country, State: c.State,
			SessionID: c.SessionID, ValidUntil: optionalTime(c.ValidUntil), AuthState: optionalText(c.AuthState), CreatedBy: int64(c.CreatedBy),
			LastSyncAt: optionalTime(c.LastSyncAt), LastError: c.LastError, CreatedAt: timestamp(c.CreatedAt), UpdatedAt: timestamp(c.UpdatedAt),
		}); err != nil {
			return err
		}
		if err := q.DeleteBankConnectionAccounts(ctx, int64(c.ID)); err != nil {
			return err
		}
		for i, a := range c.Accounts {
			if err := q.InsertBankConnectionAccount(ctx, sqlc.InsertBankConnectionAccountParams{
				FinancialAccountID: int64(a.AccountID), ConnectionID: int64(c.ID), AccountUid: a.UID, Position: int32(i),
			}); err != nil {
				return err
			}
		}
		return nil
	})
}

// DeleteConnection implements banking.ConnectionStore.
func (s *BankingStore) DeleteConnection(ctx context.Context, admin, id resource.ID) error {
	return s.q.DeleteBankConnection(ctx, sqlc.DeleteBankConnectionParams{AdministrationID: int64(admin), ID: int64(id)})
}

func (s *BankingStore) oneConnection(ctx context.Context, r sqlc.BankConnection, err error) (banking.Connection, error) {
	if errors.Is(err, pgx.ErrNoRows) {
		return banking.Connection{}, banking.ErrConnectionNotFound
	}
	if err != nil {
		return banking.Connection{}, err
	}
	out, err := s.withAccounts(ctx, []sqlc.BankConnection{r})
	if err != nil {
		return banking.Connection{}, err
	}
	return out[0], nil
}

func (s *BankingStore) withAccounts(ctx context.Context, rows []sqlc.BankConnection) ([]banking.Connection, error) {
	if len(rows) == 0 {
		return nil, nil
	}
	ids := make([]int64, len(rows))
	for i, r := range rows {
		ids[i] = r.ID
	}
	accounts, err := s.q.BankConnectionAccounts(ctx, ids)
	if err != nil {
		return nil, err
	}
	byConnection := map[int64][]banking.LinkedAccount{}
	for _, a := range accounts {
		byConnection[a.ConnectionID] = append(byConnection[a.ConnectionID], banking.LinkedAccount{AccountID: resource.ID(a.FinancialAccountID), UID: a.AccountUid})
	}
	out := make([]banking.Connection, len(rows))
	for i, r := range rows {
		out[i] = banking.Connection{
			ID: resource.ID(r.ID), AdministrationID: resource.ID(r.AdministrationID), Provider: r.Provider, Bank: r.Bank, Country: r.Country,
			State: r.State, SessionID: r.SessionID, ValidUntil: timeOf(r.ValidUntil), AuthState: r.AuthState.String, CreatedBy: resource.ID(r.CreatedBy),
			LastSyncAt: timeOf(r.LastSyncAt), LastError: r.LastError, Accounts: byConnection[r.ID], CreatedAt: r.CreatedAt.Time, UpdatedAt: r.UpdatedAt.Time,
		}
	}
	return out, nil
}
