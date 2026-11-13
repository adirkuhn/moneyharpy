package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/adirkuhn/moneyharpy/internal/money"
	"github.com/adirkuhn/moneyharpy/internal/postgres/sqlc"
	"github.com/adirkuhn/moneyharpy/internal/purchases"
	"github.com/adirkuhn/moneyharpy/internal/resource"
)

// IncomingDocuments implements purchases.Store.
func (s *PurchaseStore) IncomingDocuments(ctx context.Context, admin resource.ID, q purchases.IncomingQuery) ([]purchases.Incoming, int, error) {
	p := incomingParams(admin, q)
	p.MaxRows, p.Skip = page(q.Page, q.PerPage)
	return s.incoming(ctx, p)
}

// IncomingDocument implements purchases.Store.
func (s *PurchaseStore) IncomingDocument(ctx context.Context, admin, id resource.ID, today time.Time) (purchases.Incoming, error) {
	p := incomingParams(admin, purchases.IncomingQuery{Query: purchases.Query{Today: today}})
	p.ID = pgtype.Int8{Int64: int64(id), Valid: true}
	list, _, err := s.incoming(ctx, p)
	if err != nil {
		return purchases.Incoming{}, err
	}
	if len(list) == 0 {
		return purchases.Incoming{}, purchases.ErrNotFound
	}
	return list[0], nil
}

func incomingParams(admin resource.ID, q purchases.IncomingQuery) sqlc.ListIncomingDocumentsParams {
	p := sqlc.ListIncomingDocumentsParams{
		AdministrationID: int64(admin), Kinds: nonNil(q.Kinds), Nothing: q.None, States: nonNil(q.States), Today: day(q.Today),
		ContactID: optionalID(q.ContactID), Attachment: q.Attachment, Ascending: q.Ascending,
	}
	p.FromDate, p.ToDate = period(q.Period)
	return p
}

func (s *PurchaseStore) incoming(ctx context.Context, p sqlc.ListIncomingDocumentsParams) ([]purchases.Incoming, int, error) {
	rows, err := s.q.ListIncomingDocuments(ctx, p)
	if err != nil {
		return nil, 0, err
	}
	out := make([]purchases.Incoming, len(rows))
	total := 0
	for i, r := range rows {
		out[i] = purchases.Incoming{
			ID: resource.ID(r.ID), AdministrationID: resource.ID(p.AdministrationID), Kind: r.Kind, Reference: r.Reference,
			ContactID: resource.ID(r.ContactID), ContactName: r.ContactName, Date: dayOf(r.Date), State: r.State, Currency: r.Currency,
			TotalInclTax: money.Amount(r.TotalInclTax), AttachmentsCount: int(r.AttachmentsCount), CreatedAt: r.CreatedAt.Time,
		}
		total = int(r.Total)
	}
	return out, total, nil
}

// IncomingStatistics implements purchases.Store.
func (s *PurchaseStore) IncomingStatistics(ctx context.Context, admin resource.ID, q purchases.IncomingQuery) (purchases.IncomingStatistics, error) {
	p := sqlc.IncomingDocumentStatisticsParams{
		AdministrationID: int64(admin), Kinds: nonNil(q.Kinds), Nothing: q.None, States: nonNil(q.States), Today: day(q.Today),
		ContactID: optionalID(q.ContactID), Attachment: q.Attachment,
	}
	p.FromDate, p.ToDate = period(q.Period)
	r, err := s.q.IncomingDocumentStatistics(ctx, p)
	if err != nil {
		return purchases.IncomingStatistics{}, err
	}
	var categories []struct {
		LedgerAccountID *int64 `json:"ledger_account_id"`
		Name            string `json:"name"`
		Amount          int64  `json:"amount"`
	}
	if err := json.Unmarshal(r.Categories, &categories); err != nil {
		return purchases.IncomingStatistics{}, fmt.Errorf("read categories: %w", err)
	}
	st := purchases.IncomingStatistics{CostsExclTax: money.Amount(r.Costs), AmountPayable: money.Amount(r.Payable), Categories: make([]purchases.CostCategory, len(categories))}
	for i, c := range categories {
		st.Categories[i] = purchases.CostCategory{Name: c.Name, Amount: money.Amount(c.Amount)}
		if c.LedgerAccountID != nil {
			st.Categories[i].LedgerAccountID = resource.ID(*c.LedgerAccountID)
		}
	}
	return st, nil
}
