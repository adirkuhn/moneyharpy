package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/adirkuhn/moneyharpy/internal/money"
	"github.com/adirkuhn/moneyharpy/internal/postgres/sqlc"
	"github.com/adirkuhn/moneyharpy/internal/resource"
	"github.com/adirkuhn/moneyharpy/internal/sales"
)

func optionalBool(b *bool) pgtype.Bool {
	if b == nil {
		return pgtype.Bool{}
	}
	return pgtype.Bool{Bool: *b, Valid: true}
}

// RecurringInvoices implements sales.Store.
func (s *SalesStore) RecurringInvoices(ctx context.Context, admin resource.ID, q sales.RecurringQuery) ([]sales.Recurring, int, error) {
	p := sqlc.ListRecurringSalesInvoicesParams{
		AdministrationID: int64(admin), Active: optionalBool(q.Active), FrequencyType: q.FrequencyType, AutoSend: optionalBool(q.AutoSend),
		ContactID: optionalID(q.ContactID), WorkflowID: optionalID(q.WorkflowID), IncludeSubscriptions: q.IncludeSubscriptions,
	}
	if q.PerPage > 0 {
		p.MaxRows = pgtype.Int4{Int32: int32(q.PerPage), Valid: true}
		p.Skip = int32((max(q.Page, 1) - 1) * q.PerPage)
	}
	rows, err := s.q.ListRecurringSalesInvoices(ctx, p)
	if err != nil {
		return nil, 0, err
	}
	list := make([]sqlc.RecurringSalesInvoice, len(rows))
	total := 0
	for i, r := range rows {
		list[i], total = r.RecurringSalesInvoice, int(r.Total)
	}
	out, err := s.completeRecurring(ctx, admin, list)
	return out, total, err
}

// RecurringInvoice implements sales.Store.
func (s *SalesStore) RecurringInvoice(ctx context.Context, admin, id resource.ID) (sales.Recurring, error) {
	r, err := s.q.RecurringSalesInvoiceByID(ctx, sqlc.RecurringSalesInvoiceByIDParams{AdministrationID: int64(admin), ID: int64(id)})
	if errors.Is(err, pgx.ErrNoRows) {
		return sales.Recurring{}, sales.ErrNotFound
	}
	if err != nil {
		return sales.Recurring{}, err
	}
	out, err := s.completeRecurring(ctx, admin, []sqlc.RecurringSalesInvoice{r})
	if err != nil {
		return sales.Recurring{}, err
	}
	return out[0], nil
}

// RecurringInvoicesByID implements sales.Store.
func (s *SalesStore) RecurringInvoicesByID(ctx context.Context, admin resource.ID, ids []resource.ID) ([]sales.Recurring, error) {
	rows, err := s.q.RecurringSalesInvoicesByIDs(ctx, sqlc.RecurringSalesInvoicesByIDsParams{AdministrationID: int64(admin), Ids: ids64(ids)})
	if err != nil {
		return nil, err
	}
	return s.completeRecurring(ctx, admin, rows)
}

// SaveRecurring implements sales.Store.
func (s *SalesStore) SaveRecurring(ctx context.Context, r sales.Recurring, created bool) error {
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		return saveRecurring(ctx, s.q.WithTx(tx), r, created)
	})
}

// SaveGenerated implements sales.Store: the new invoice and the recurring
// invoice moved on are stored in one transaction.
func (s *SalesStore) SaveGenerated(ctx context.Context, inv sales.Invoice, r sales.Recurring) error {
	delivery, taxTotals, err := encodeInvoice(inv)
	if err != nil {
		return err
	}
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		q := s.q.WithTx(tx)
		if err := saveInvoice(ctx, q, inv, delivery, taxTotals, nil, true); err != nil {
			return err
		}
		return saveRecurring(ctx, q, r, false)
	})
}

func saveRecurring(ctx context.Context, q *sqlc.Queries, r sales.Recurring, created bool) error {
	taxTotals, err := encodeTaxTotals(r.Totals)
	if err != nil {
		return err
	}
	if err := q.UpsertRecurringSalesInvoice(ctx, sqlc.UpsertRecurringSalesInvoiceParams{
		ID: int64(r.ID), AdministrationID: int64(r.AdministrationID), ContactID: int64(r.ContactID), ContactPersonID: optionalID(r.ContactPersonID),
		IdentityID: int64(r.IdentityID), DocumentStyleID: int64(r.DocumentStyleID), WorkflowID: int64(r.WorkflowID), Reference: r.Reference,
		Language: r.Language, Currency: r.Currency, PricesAreInclTax: r.PricesAreInclTax, PaymentConditions: r.PaymentConditions,
		Discount: int32(r.Discount), FirstDueInterval: int32(r.FirstDueInterval), StartDate: day(r.StartDate), InvoiceDate: day(r.InvoiceDate),
		LastDate: optionalDate(r.LastDate), Active: r.Active, FrequencyType: r.FrequencyType, FrequencyCount: int32(r.Frequency),
		AutoSend: r.AutoSend, AutoSendAt: optionalTime(r.AutoSendAt), AutoSendUserID: optionalID(r.AutoSendUserID), Mergeable: r.Mergeable,
		AnchorDate: day(r.Anchor), PeriodCount: int32(r.Periods), GeneratedCount: int32(r.Generated), TotalExclTax: int64(r.Totals.ExclTax),
		TotalTax: int64(r.Totals.Tax), TotalInclTax: int64(r.Totals.InclTax), TotalDiscount: int64(r.Totals.Discount), TaxTotals: taxTotals,
		CreatedAt: timestamp(r.CreatedAt), UpdatedAt: timestamp(r.UpdatedAt),
	}); err != nil {
		return err
	}
	if created {
		if err := q.InsertResource(ctx, sqlc.InsertResourceParams{ID: int64(r.ID), Type: string(resource.TypeRecurringSalesInvoice), AdministrationID: int64(r.AdministrationID)}); err != nil {
			return err
		}
	}
	if err := q.DeleteRecurringSalesInvoiceDetails(ctx, int64(r.ID)); err != nil {
		return err
	}
	for i, d := range r.Lines {
		var base int64
		if i < len(r.Totals.LineBases) {
			base = int64(r.Totals.LineBases[i])
		}
		if err := q.InsertRecurringSalesInvoiceDetail(ctx, sqlc.InsertRecurringSalesInvoiceDetailParams{
			ID: int64(d.ID), AdministrationID: int64(r.AdministrationID), RecurringSalesInvoiceID: int64(r.ID), Description: d.Description,
			Period: d.Period, Price: int64(d.Price), Amount: d.Amount, Quantity: d.Quantity, TaxRateID: int64(d.TaxRateID),
			LedgerAccountID: optionalID(d.LedgerAccountID), ProjectID: optionalID(d.ProjectID), ProductID: optionalID(d.ProductID),
			RowOrder: int32(d.RowOrder), IsOptional: d.Optional, IsSelected: d.Selected, LineBase: base, CreatedAt: timestamp(d.CreatedAt),
			UpdatedAt: timestamp(d.UpdatedAt),
		}); err != nil {
			return err
		}
	}
	if err := q.DeleteRecurringSalesInvoiceCustomFieldValues(ctx, int64(r.ID)); err != nil {
		return err
	}
	for _, v := range r.CustomFields {
		if err := q.InsertRecurringSalesInvoiceCustomFieldValue(ctx, sqlc.InsertRecurringSalesInvoiceCustomFieldValueParams{
			RecurringSalesInvoiceID: int64(r.ID), CustomFieldID: int64(v.FieldID), Value: v.Value,
		}); err != nil {
			return err
		}
	}
	return nil
}

// DeleteRecurring implements sales.Store; the invoices it made stay.
func (s *SalesStore) DeleteRecurring(ctx context.Context, admin, id resource.ID) error {
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		q := s.q.WithTx(tx)
		if err := q.DeleteNotesOf(ctx, sqlc.DeleteNotesOfParams{AdministrationID: int64(admin), EntityID: int64(id)}); err != nil {
			return err
		}
		if err := q.DeleteRecurringSalesInvoice(ctx, sqlc.DeleteRecurringSalesInvoiceParams{AdministrationID: int64(admin), ID: int64(id)}); err != nil {
			return err
		}
		return q.DeleteResource(ctx, int64(id))
	})
}

// TouchRecurring implements sales.Store.
func (s *SalesStore) TouchRecurring(ctx context.Context, admin, id resource.ID, at time.Time) error {
	return s.q.TouchRecurringSalesInvoice(ctx, sqlc.TouchRecurringSalesInvoiceParams{AdministrationID: int64(admin), ID: int64(id), UpdatedAt: timestamp(at)})
}

// completeRecurring converts rows and loads their lines and custom fields.
func (s *SalesStore) completeRecurring(ctx context.Context, admin resource.ID, rows []sqlc.RecurringSalesInvoice) ([]sales.Recurring, error) {
	out := make([]sales.Recurring, len(rows))
	if len(rows) == 0 {
		return out, nil
	}
	ids := make([]int64, len(rows))
	index := map[int64]int{}
	for i, r := range rows {
		ids[i], index[r.ID] = r.ID, i
		rec, err := toRecurring(r)
		if err != nil {
			return nil, err
		}
		out[i] = rec
	}
	details, err := s.q.RecurringSalesInvoiceDetailsOf(ctx, sqlc.RecurringSalesInvoiceDetailsOfParams{AdministrationID: int64(admin), Ids: ids})
	if err != nil {
		return nil, err
	}
	for _, d := range details {
		r := &out[index[d.RecurringSalesInvoiceID]]
		r.Lines = append(r.Lines, sales.Detail{
			ID: resource.ID(d.ID), Description: d.Description, Period: d.Period, Price: money.Amount(d.Price), Amount: d.Amount, Quantity: d.Quantity,
			TaxRateID: resource.ID(d.TaxRateID), LedgerAccountID: idOf(d.LedgerAccountID), ProjectID: idOf(d.ProjectID), ProductID: idOf(d.ProductID),
			RowOrder: int(d.RowOrder), Optional: d.IsOptional, Selected: d.IsSelected, CreatedAt: d.CreatedAt.Time, UpdatedAt: d.UpdatedAt.Time,
		})
		r.Totals.LineBases = append(r.Totals.LineBases, money.Amount(d.LineBase))
	}
	subscribed, err := s.q.SubscriptionsOfRecurringSalesInvoices(ctx, sqlc.SubscriptionsOfRecurringSalesInvoicesParams{AdministrationID: int64(admin), Ids: ids})
	if err != nil {
		return nil, err
	}
	for _, l := range subscribed {
		id := resource.ID(l.ID)
		out[index[l.RecurringSalesInvoiceID]].SubscriptionID = &id
	}
	values, err := s.q.RecurringSalesInvoiceCustomFieldValuesOf(ctx, sqlc.RecurringSalesInvoiceCustomFieldValuesOfParams{AdministrationID: int64(admin), Ids: ids})
	if err != nil {
		return nil, err
	}
	for _, v := range values {
		r := &out[index[v.RecurringSalesInvoiceID]]
		r.CustomFields = append(r.CustomFields, sales.FieldValue{FieldID: resource.ID(v.ID), Name: v.Name, Value: v.Value})
	}
	return out, nil
}

func toRecurring(r sqlc.RecurringSalesInvoice) (sales.Recurring, error) {
	rec := sales.Recurring{
		ID: resource.ID(r.ID), AdministrationID: resource.ID(r.AdministrationID), ContactID: resource.ID(r.ContactID), ContactPersonID: idOf(r.ContactPersonID),
		IdentityID: resource.ID(r.IdentityID), DocumentStyleID: resource.ID(r.DocumentStyleID), WorkflowID: resource.ID(r.WorkflowID),
		Reference: r.Reference, Language: r.Language, Currency: r.Currency, PricesAreInclTax: r.PricesAreInclTax, PaymentConditions: r.PaymentConditions,
		Discount: int64(r.Discount), FirstDueInterval: int(r.FirstDueInterval), StartDate: dayOf(r.StartDate), InvoiceDate: dayOf(r.InvoiceDate),
		LastDate: dateOf(r.LastDate), Active: r.Active, FrequencyType: r.FrequencyType, Frequency: int(r.FrequencyCount), AutoSend: r.AutoSend,
		AutoSendAt: timeOf(r.AutoSendAt), AutoSendUserID: idOf(r.AutoSendUserID), Mergeable: r.Mergeable, Anchor: dayOf(r.AnchorDate),
		Periods: int(r.PeriodCount), Generated: int(r.GeneratedCount),
		Totals: sales.Totals{
			ExclTax: money.Amount(r.TotalExclTax), Tax: money.Amount(r.TotalTax), InclTax: money.Amount(r.TotalInclTax), Discount: money.Amount(r.TotalDiscount),
		},
		CreatedAt: r.CreatedAt.Time, UpdatedAt: r.UpdatedAt.Time,
	}
	rates, err := decodeTaxTotals(r.TaxTotals)
	if err != nil {
		return rec, fmt.Errorf("recurring invoice %d: %w", r.ID, err)
	}
	if len(rates) > 0 {
		rec.Totals.ByRate = rates
	}
	return rec, nil
}
