package postgres

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/adirkuhn/moneyharpy/internal/audits"
	"github.com/adirkuhn/moneyharpy/internal/money"
	"github.com/adirkuhn/moneyharpy/internal/postgres/sqlc"
	"github.com/adirkuhn/moneyharpy/internal/resource"
)

// AuditStore implements audits.Store.
type AuditStore struct{ q *sqlc.Queries }

// NewAuditStore returns an AuditStore on pool.
func NewAuditStore(pool *pgxpool.Pool) *AuditStore { return &AuditStore{q: sqlc.New(pool)} }

func (s *AuditStore) Sums(ctx context.Context, admin resource.ID, p money.Period) ([]audits.Sums, error) {
	rows, err := s.q.AuditSums(ctx, sqlc.AuditSumsParams{AdministrationID: int64(admin), FromDate: dateValue(p.From), ToDate: dateValue(p.To)})
	if err != nil {
		return nil, err
	}
	out := make([]audits.Sums, len(rows))
	for i, r := range rows {
		ids := make([]resource.ID, len(r.TaxRateIds))
		for j, id := range r.TaxRateIds {
			ids[j] = resource.ID(id)
		}
		out[i] = audits.Sums{LedgerAccountID: resource.ID(r.LedgerAccountID), Debit: money.Amount(r.Debit), Credit: money.Amount(r.Credit), TaxRateIDs: ids}
	}
	return out, nil
}

func (s *AuditStore) Audits(ctx context.Context, admin resource.ID, p money.Period) ([]audits.Audit, error) {
	rows, err := s.q.ListAudits(ctx, sqlc.ListAuditsParams{AdministrationID: int64(admin), PeriodFrom: dateValue(p.From), PeriodTo: dateValue(p.To)})
	if err != nil {
		return nil, err
	}
	out := make([]audits.Audit, len(rows))
	for i, r := range rows {
		out[i] = audits.Audit{
			ID: resource.ID(r.ID), LedgerAccountID: resource.ID(r.LedgerAccountID), Period: money.Period{From: r.PeriodFrom.Time, To: r.PeriodTo.Time},
			Debit: money.Amount(r.Debit), Credit: money.Amount(r.Credit), UserID: resource.ID(r.UserID), AuditedAt: r.AuditedAt.Time,
		}
	}
	return out, nil
}

func (s *AuditStore) Save(ctx context.Context, admin resource.ID, a audits.Audit) error {
	return s.q.SaveAudit(ctx, sqlc.SaveAuditParams{
		ID: int64(a.ID), AdministrationID: int64(admin), LedgerAccountID: int64(a.LedgerAccountID), PeriodFrom: dateValue(a.Period.From), PeriodTo: dateValue(a.Period.To),
		Debit: int64(a.Debit), Credit: int64(a.Credit), UserID: int64(a.UserID), AuditedAt: timestamp(a.AuditedAt),
	})
}

func (s *AuditStore) Delete(ctx context.Context, admin, ledger resource.ID, p money.Period) error {
	return s.q.DeleteAudit(ctx, sqlc.DeleteAuditParams{AdministrationID: int64(admin), LedgerAccountID: int64(ledger), PeriodFrom: dateValue(p.From), PeriodTo: dateValue(p.To)})
}

func (s *AuditStore) HasAccount(ctx context.Context, admin, ledger resource.ID) (bool, error) {
	return s.q.LedgerAccountInAdministration(ctx, sqlc.LedgerAccountInAdministrationParams{AdministrationID: int64(admin), ID: int64(ledger)})
}
