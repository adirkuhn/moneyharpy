package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/adirkuhn/moneyharpy/internal/money"
	"github.com/adirkuhn/moneyharpy/internal/postgres/sqlc"
	"github.com/adirkuhn/moneyharpy/internal/resource"
	"github.com/adirkuhn/moneyharpy/internal/sales"
)

// Estimates implements sales.EstimateStore.
func (s *SalesStore) Estimates(ctx context.Context, admin resource.ID, q sales.EstimateQuery) ([]sales.Estimate, int, error) {
	p := sqlc.ListEstimatesParams{
		AdministrationID: int64(admin), States: q.States, Today: day(q.Today), Refs: q.References, ContactID: optionalID(q.ContactID),
		WorkflowID: optionalID(q.WorkflowID), CreatedAfter: optionalTime(q.CreatedAfter), UpdatedAfter: optionalTime(q.UpdatedAfter),
	}
	if p.States == nil {
		p.States = []string{}
	}
	if p.Refs == nil {
		p.Refs = []string{}
	}
	if q.Period != nil {
		p.FromDate, p.ToDate = day(q.Period.From), day(q.Period.To)
	}
	if q.PerPage > 0 {
		p.MaxRows = pgtype.Int4{Int32: int32(q.PerPage), Valid: true}
		p.Skip = int32((max(q.Page, 1) - 1) * q.PerPage)
	}
	rows, err := s.q.ListEstimates(ctx, p)
	if err != nil {
		return nil, 0, err
	}
	list := make([]sqlc.Estimate, len(rows))
	total := 0
	for i, r := range rows {
		list[i], total = r.Estimate, int(r.Total)
	}
	out, err := s.completeEstimates(ctx, admin, list)
	return out, total, err
}

func (s *SalesStore) oneEstimate(ctx context.Context, admin resource.ID, load func() (sqlc.Estimate, error)) (sales.Estimate, error) {
	r, err := load()
	if errors.Is(err, pgx.ErrNoRows) {
		return sales.Estimate{}, sales.ErrEstimateNotFound
	}
	if err != nil {
		return sales.Estimate{}, err
	}
	out, err := s.completeEstimates(ctx, admin, []sqlc.Estimate{r})
	if err != nil {
		return sales.Estimate{}, err
	}
	return out[0], nil
}

// Estimate implements sales.EstimateStore.
func (s *SalesStore) Estimate(ctx context.Context, admin, id resource.ID) (sales.Estimate, error) {
	return s.oneEstimate(ctx, admin, func() (sqlc.Estimate, error) {
		return s.q.EstimateByID(ctx, sqlc.EstimateByIDParams{AdministrationID: int64(admin), ID: int64(id)})
	})
}

// EstimateByNumber implements sales.EstimateStore.
func (s *SalesStore) EstimateByNumber(ctx context.Context, admin resource.ID, number string) (sales.Estimate, error) {
	return s.oneEstimate(ctx, admin, func() (sqlc.Estimate, error) {
		return s.q.EstimateByNumber(ctx, sqlc.EstimateByNumberParams{AdministrationID: int64(admin), EstimateID: number})
	})
}

// EstimatesByID implements sales.EstimateStore.
func (s *SalesStore) EstimatesByID(ctx context.Context, admin resource.ID, ids []resource.ID) ([]sales.Estimate, error) {
	rows, err := s.q.EstimatesByIDs(ctx, sqlc.EstimatesByIDsParams{AdministrationID: int64(admin), Ids: ids64(ids)})
	if err != nil {
		return nil, err
	}
	return s.completeEstimates(ctx, admin, rows)
}

// NextEstimateDraftID implements sales.EstimateStore.
func (s *SalesStore) NextEstimateDraftID(ctx context.Context, admin resource.ID) (int, error) {
	n, err := s.q.NextEstimateDraftID(ctx, int64(admin))
	return int(n), err
}

// ReserveEstimateNumber implements sales.EstimateStore: it takes the
// estimate sequence number and moves the counter on under a row lock,
// keeping its leading zeros.
func (s *SalesStore) ReserveEstimateNumber(ctx context.Context, admin resource.ID) (string, error) {
	var seq string
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		q := s.q.WithTx(tx)
		current, err := q.LockEstimateSequence(ctx, int64(admin))
		if err != nil {
			return err
		}
		n, err := strconv.ParseInt(current, 10, 64)
		if err != nil {
			return fmt.Errorf("estimate sequence %q: %w", current, err)
		}
		next := strconv.FormatInt(n+1, 10)
		if len(next) < len(current) {
			next = strings.Repeat("0", len(current)-len(next)) + next
		}
		seq = current
		return q.SetEstimateSequence(ctx, sqlc.SetEstimateSequenceParams{ID: int64(admin), EstimateSequenceID: next})
	})
	return seq, err
}

// EstimateNumberTaken implements sales.EstimateStore.
func (s *SalesStore) EstimateNumberTaken(ctx context.Context, admin resource.ID, number string, except resource.ID) (bool, error) {
	return s.q.EstimateNumberTaken(ctx, sqlc.EstimateNumberTakenParams{AdministrationID: int64(admin), EstimateID: number, ID: int64(except)})
}

// SaveEstimate implements sales.EstimateStore.
func (s *SalesStore) SaveEstimate(ctx context.Context, e sales.Estimate, created bool) error {
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		return saveEstimate(ctx, s.q.WithTx(tx), e, created)
	})
}

// SaveBilled implements sales.EstimateStore: the new invoice and the billed
// estimate are stored in one transaction.
func (s *SalesStore) SaveBilled(ctx context.Context, inv sales.Invoice, e sales.Estimate) error {
	delivery, taxTotals, err := encodeInvoice(inv)
	if err != nil {
		return err
	}
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		q := s.q.WithTx(tx)
		if err := saveInvoice(ctx, q, inv, delivery, taxTotals, nil, true); err != nil {
			return err
		}
		return saveEstimate(ctx, q, e, false)
	})
}

func saveEstimate(ctx context.Context, q *sqlc.Queries, e sales.Estimate, created bool) error {
	taxTotals, err := encodeTaxTotals(e.Totals)
	if err != nil {
		return err
	}
	var delivery []byte
	if e.Delivery != nil {
		if delivery, err = json.Marshal(e.Delivery); err != nil {
			return err
		}
	}
	if err := q.UpsertEstimate(ctx, sqlc.UpsertEstimateParams{
		ID: int64(e.ID), AdministrationID: int64(e.AdministrationID), ContactID: int64(e.ContactID), ContactPersonID: optionalID(e.ContactPersonID),
		IdentityID: int64(e.IdentityID), DocumentStyleID: int64(e.DocumentStyleID), WorkflowID: int64(e.WorkflowID), DraftID: int32(e.DraftID),
		EstimateID: e.EstimateID, SequenceID: e.SequenceID, Reference: e.Reference, Language: e.Language, Currency: e.Currency,
		PricesAreInclTax: e.PricesAreInclTax, ShowTax: e.ShowTax, SignOnline: e.SignOnline, Discount: int32(e.Discount),
		EstimateDate: optionalDate(e.EstimateDate), DueDate: optionalDate(e.DueDate), FirstDueInterval: int32(e.FirstDueInterval),
		PreText: e.PreText, PostText: e.PostText, State: e.State, SentAt: optionalTime(e.SentAt), AcceptedAt: optionalDate(e.AcceptedAt),
		RejectedAt: optionalDate(e.RejectedAt), ArchivedAt: optionalDate(e.ArchivedAt), OriginalEstimateID: optionalID(e.OriginalEstimateID),
		OriginalSalesInvoiceID: optionalID(e.OriginalSalesInvoiceID), Delivery: delivery, TotalExclTax: int64(e.Totals.ExclTax),
		TotalTax: int64(e.Totals.Tax), TotalInclTax: int64(e.Totals.InclTax), TotalDiscount: int64(e.Totals.Discount), TaxTotals: taxTotals,
		CreatedAt: timestamp(e.CreatedAt), UpdatedAt: timestamp(e.UpdatedAt),
	}); err != nil {
		return err
	}
	if created {
		if err := q.InsertResource(ctx, sqlc.InsertResourceParams{ID: int64(e.ID), Type: string(resource.TypeEstimate), AdministrationID: int64(e.AdministrationID)}); err != nil {
			return err
		}
	}
	if err := q.DeleteEstimateDetails(ctx, int64(e.ID)); err != nil {
		return err
	}
	for i, d := range e.Lines {
		var base int64
		if i < len(e.Totals.LineBases) {
			base = int64(e.Totals.LineBases[i])
		}
		if err := q.InsertEstimateDetail(ctx, sqlc.InsertEstimateDetailParams{
			ID: int64(d.ID), AdministrationID: int64(e.AdministrationID), EstimateID: int64(e.ID), Description: d.Description, Period: d.Period,
			Price: int64(d.Price), Amount: d.Amount, Quantity: d.Quantity, TaxRateID: int64(d.TaxRateID), LedgerAccountID: optionalID(d.LedgerAccountID),
			ProjectID: optionalID(d.ProjectID), ProductID: optionalID(d.ProductID), RowOrder: int32(d.RowOrder), IsOptional: d.Optional,
			IsSelected: d.Selected, LineBase: base, CreatedAt: timestamp(d.CreatedAt), UpdatedAt: timestamp(d.UpdatedAt),
		}); err != nil {
			return err
		}
	}
	if err := q.DeleteEstimateCustomFieldValues(ctx, int64(e.ID)); err != nil {
		return err
	}
	for _, v := range e.CustomFields {
		if err := q.InsertEstimateCustomFieldValue(ctx, sqlc.InsertEstimateCustomFieldValueParams{EstimateID: int64(e.ID), CustomFieldID: int64(v.FieldID), Value: v.Value}); err != nil {
			return err
		}
	}
	return nil
}

// DeleteEstimate implements sales.EstimateStore; invoices billed from it
// stay and lose the link.
func (s *SalesStore) DeleteEstimate(ctx context.Context, admin, id resource.ID) error {
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		q := s.q.WithTx(tx)
		if err := q.DeleteNotesOf(ctx, sqlc.DeleteNotesOfParams{AdministrationID: int64(admin), EntityID: int64(id)}); err != nil {
			return err
		}
		if err := q.DeleteEstimate(ctx, sqlc.DeleteEstimateParams{AdministrationID: int64(admin), ID: int64(id)}); err != nil {
			return err
		}
		return q.DeleteResource(ctx, int64(id))
	})
}

// TouchEstimate implements sales.EstimateStore.
func (s *SalesStore) TouchEstimate(ctx context.Context, admin, id resource.ID, at time.Time) error {
	return s.q.TouchEstimate(ctx, sqlc.TouchEstimateParams{AdministrationID: int64(admin), ID: int64(id), UpdatedAt: timestamp(at)})
}

func (s *SalesStore) contactOnEstimate(ctx context.Context, admin, contact resource.ID) (bool, error) {
	return s.q.ContactOnEstimate(ctx, sqlc.ContactOnEstimateParams{AdministrationID: int64(admin), ContactID: int64(contact)})
}

func (s *SalesStore) productOnEstimates(ctx context.Context, admin, product resource.ID) (bool, error) {
	return s.q.ProductOnEstimates(ctx, sqlc.ProductOnEstimatesParams{AdministrationID: int64(admin), ProductID: pgtype.Int8{Int64: int64(product), Valid: true}})
}

func (s *SalesStore) projectOnEstimates(ctx context.Context, admin, project resource.ID) (bool, error) {
	return s.q.ProjectOnEstimates(ctx, sqlc.ProjectOnEstimatesParams{AdministrationID: int64(admin), ProjectID: pgtype.Int8{Int64: int64(project), Valid: true}})
}

// completeEstimates converts rows and loads their lines and custom fields.
func (s *SalesStore) completeEstimates(ctx context.Context, admin resource.ID, rows []sqlc.Estimate) ([]sales.Estimate, error) {
	out := make([]sales.Estimate, len(rows))
	if len(rows) == 0 {
		return out, nil
	}
	ids := make([]int64, len(rows))
	index := map[int64]int{}
	for i, r := range rows {
		ids[i], index[r.ID] = r.ID, i
		e, err := toEstimate(r)
		if err != nil {
			return nil, err
		}
		out[i] = e
	}
	details, err := s.q.EstimateDetailsOf(ctx, sqlc.EstimateDetailsOfParams{AdministrationID: int64(admin), Ids: ids})
	if err != nil {
		return nil, err
	}
	for _, d := range details {
		e := &out[index[d.EstimateID]]
		e.Lines = append(e.Lines, sales.Detail{
			ID: resource.ID(d.ID), Description: d.Description, Period: d.Period, Price: money.Amount(d.Price), Amount: d.Amount, Quantity: d.Quantity,
			TaxRateID: resource.ID(d.TaxRateID), LedgerAccountID: idOf(d.LedgerAccountID), ProjectID: idOf(d.ProjectID), ProductID: idOf(d.ProductID),
			RowOrder: int(d.RowOrder), Optional: d.IsOptional, Selected: d.IsSelected, CreatedAt: d.CreatedAt.Time, UpdatedAt: d.UpdatedAt.Time,
		})
		e.Totals.LineBases = append(e.Totals.LineBases, money.Amount(d.LineBase))
	}
	values, err := s.q.EstimateCustomFieldValuesOf(ctx, sqlc.EstimateCustomFieldValuesOfParams{AdministrationID: int64(admin), Ids: ids})
	if err != nil {
		return nil, err
	}
	for _, v := range values {
		e := &out[index[v.EstimateID]]
		e.CustomFields = append(e.CustomFields, sales.FieldValue{FieldID: resource.ID(v.ID), Name: v.Name, Value: v.Value})
	}
	return out, nil
}

func toEstimate(r sqlc.Estimate) (sales.Estimate, error) {
	e := sales.Estimate{
		ID: resource.ID(r.ID), AdministrationID: resource.ID(r.AdministrationID), ContactID: resource.ID(r.ContactID), ContactPersonID: idOf(r.ContactPersonID),
		IdentityID: resource.ID(r.IdentityID), DocumentStyleID: resource.ID(r.DocumentStyleID), WorkflowID: resource.ID(r.WorkflowID), DraftID: int(r.DraftID),
		EstimateID: r.EstimateID, SequenceID: r.SequenceID, Reference: r.Reference, Language: r.Language, Currency: r.Currency,
		PricesAreInclTax: r.PricesAreInclTax, ShowTax: r.ShowTax, SignOnline: r.SignOnline, Discount: int64(r.Discount),
		EstimateDate: dateOf(r.EstimateDate), DueDate: dateOf(r.DueDate), FirstDueInterval: int(r.FirstDueInterval), PreText: r.PreText,
		PostText: r.PostText, State: r.State, SentAt: timeOf(r.SentAt), AcceptedAt: dateOf(r.AcceptedAt), RejectedAt: dateOf(r.RejectedAt),
		ArchivedAt: dateOf(r.ArchivedAt), OriginalEstimateID: idOf(r.OriginalEstimateID), OriginalSalesInvoiceID: idOf(r.OriginalSalesInvoiceID),
		Totals: sales.Totals{
			ExclTax: money.Amount(r.TotalExclTax), Tax: money.Amount(r.TotalTax), InclTax: money.Amount(r.TotalInclTax), Discount: money.Amount(r.TotalDiscount),
		},
		CreatedAt: r.CreatedAt.Time, UpdatedAt: r.UpdatedAt.Time,
	}
	if len(r.Delivery) > 0 {
		var d sales.Delivery
		if err := json.Unmarshal(r.Delivery, &d); err != nil {
			return e, fmt.Errorf("estimate %d delivery: %w", r.ID, err)
		}
		e.Delivery = &d
	}
	rates, err := decodeTaxTotals(r.TaxTotals)
	if err != nil {
		return e, fmt.Errorf("estimate %d: %w", r.ID, err)
	}
	if len(rates) > 0 {
		e.Totals.ByRate = rates
	}
	return e, nil
}
