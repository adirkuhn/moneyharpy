package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/adirkuhn/moneyharpy/internal/journal"
	"github.com/adirkuhn/moneyharpy/internal/money"
	"github.com/adirkuhn/moneyharpy/internal/postgres/sqlc"
	"github.com/adirkuhn/moneyharpy/internal/resource"
	"github.com/adirkuhn/moneyharpy/internal/sales"
)

// ExternalInvoices implements sales.Store.
func (s *SalesStore) ExternalInvoices(ctx context.Context, admin resource.ID, q sales.Query) ([]sales.ExternalInvoice, int, error) {
	p := sqlc.ListExternalSalesInvoicesParams{
		AdministrationID: int64(admin), States: q.States, Today: day(q.Today), ContactID: optionalID(q.ContactID),
		CreatedAfter: optionalTime(q.CreatedAfter), UpdatedAfter: optionalTime(q.UpdatedAfter), Ascending: q.Ascending,
	}
	if p.States == nil {
		p.States = []string{}
	}
	if q.Period != nil {
		p.FromDate, p.ToDate = day(q.Period.From), day(q.Period.To)
	}
	if q.PerPage > 0 {
		p.MaxRows = pgtype.Int4{Int32: int32(q.PerPage), Valid: true}
		p.Skip = int32((max(q.Page, 1) - 1) * q.PerPage)
	}
	rows, err := s.q.ListExternalSalesInvoices(ctx, p)
	if err != nil {
		return nil, 0, err
	}
	list := make([]sqlc.ExternalSalesInvoice, len(rows))
	total := 0
	for i, r := range rows {
		list[i], total = r.ExternalSalesInvoice, int(r.Total)
	}
	out, err := s.completeExternal(ctx, admin, list)
	return out, total, err
}

// ExternalInvoice implements sales.Store.
func (s *SalesStore) ExternalInvoice(ctx context.Context, admin, id resource.ID) (sales.ExternalInvoice, error) {
	r, err := s.q.ExternalSalesInvoiceByID(ctx, sqlc.ExternalSalesInvoiceByIDParams{AdministrationID: int64(admin), ID: int64(id)})
	if errors.Is(err, pgx.ErrNoRows) {
		return sales.ExternalInvoice{}, sales.ErrNotFound
	}
	if err != nil {
		return sales.ExternalInvoice{}, err
	}
	out, err := s.completeExternal(ctx, admin, []sqlc.ExternalSalesInvoice{r})
	if err != nil {
		return sales.ExternalInvoice{}, err
	}
	return out[0], nil
}

// ExternalInvoicesByID implements sales.Store.
func (s *SalesStore) ExternalInvoicesByID(ctx context.Context, admin resource.ID, ids []resource.ID) ([]sales.ExternalInvoice, error) {
	rows, err := s.q.ExternalSalesInvoicesByIDs(ctx, sqlc.ExternalSalesInvoicesByIDsParams{AdministrationID: int64(admin), Ids: ids64(ids)})
	if err != nil {
		return nil, err
	}
	return s.completeExternal(ctx, admin, rows)
}

// NextEntryNumber implements sales.Store.
func (s *SalesStore) NextEntryNumber(ctx context.Context, admin resource.ID) (int, error) {
	n, err := s.q.NextExternalSalesInvoiceEntryNumber(ctx, int64(admin))
	return int(n), err
}

// SaveExternalInvoice implements sales.Store.
func (s *SalesStore) SaveExternalInvoice(ctx context.Context, e sales.ExternalInvoice, entries []journal.Entry, created bool) error {
	taxTotals, err := encodeTaxTotals(e.Totals)
	if err != nil {
		return err
	}
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		q := s.q.WithTx(tx)
		var contact *resource.ID
		if e.ContactID != 0 {
			contact = &e.ContactID
		}
		if err := q.UpsertExternalSalesInvoice(ctx, sqlc.UpsertExternalSalesInvoiceParams{
			ID: int64(e.ID), AdministrationID: int64(e.AdministrationID), ContactID: optionalID(contact), EntryNumber: int32(e.EntryNumber),
			Reference: e.Reference, Date: day(e.Date), DueDate: optionalDate(e.DueDate), State: e.State, Origin: e.Origin, Source: e.Source,
			SourceUrl: e.SourceURL, Currency: e.Currency, PricesAreInclTax: e.PricesAreInclTax, PaidAt: optionalTime(e.PaidAt),
			MarkedDubiousOn: optionalDate(e.MarkedDubiousOn), MarkedUncollectibleOn: optionalDate(e.MarkedUncollectibleOn),
			TotalExclTax: int64(e.Totals.ExclTax), TotalTax: int64(e.Totals.Tax), TotalInclTax: int64(e.Totals.InclTax), TaxTotals: taxTotals,
			CreatedAt: timestamp(e.CreatedAt), UpdatedAt: timestamp(e.UpdatedAt),
		}); err != nil {
			return err
		}
		if created {
			if err := q.InsertResource(ctx, sqlc.InsertResourceParams{ID: int64(e.ID), Type: string(resource.TypeExternalSalesInvoice), AdministrationID: int64(e.AdministrationID)}); err != nil {
				return err
			}
		}
		if err := q.DeleteExternalSalesInvoiceDetails(ctx, int64(e.ID)); err != nil {
			return err
		}
		for i, d := range e.Lines {
			var base int64
			if i < len(e.Totals.LineBases) {
				base = int64(e.Totals.LineBases[i])
			}
			if err := q.InsertExternalSalesInvoiceDetail(ctx, sqlc.InsertExternalSalesInvoiceDetailParams{
				ID: int64(d.ID), AdministrationID: int64(e.AdministrationID), ExternalSalesInvoiceID: int64(e.ID), Description: d.Description,
				Period: d.Period, Price: int64(d.Price), Amount: d.Amount, Quantity: d.Quantity, TaxRateID: int64(d.TaxRateID),
				LedgerAccountID: optionalID(d.LedgerAccountID), ProjectID: optionalID(d.ProjectID), ProductID: optionalID(d.ProductID),
				RowOrder: int32(d.RowOrder), LineBase: base, CreatedAt: timestamp(d.CreatedAt), UpdatedAt: timestamp(d.UpdatedAt),
			}); err != nil {
				return err
			}
		}
		if entries != nil {
			return postEntries(ctx, q, e.AdministrationID, journal.ExternalSalesInvoice, e.ID, entries, e.UpdatedAt)
		}
		return nil
	})
}

// settleExternal stores what payments and write-offs change of an external
// invoice, with what it posts unless entries is nil.
func settleExternal(ctx context.Context, q *sqlc.Queries, inv sales.Invoice, entries []journal.Entry) error {
	if err := q.SettleExternalSalesInvoice(ctx, sqlc.SettleExternalSalesInvoiceParams{
		AdministrationID: int64(inv.AdministrationID), ID: int64(inv.ID), State: inv.State, PaidAt: optionalTime(inv.PaidAt),
		MarkedDubiousOn: optionalDate(inv.MarkedDubiousOn), MarkedUncollectibleOn: optionalDate(inv.MarkedUncollectibleOn), UpdatedAt: timestamp(inv.UpdatedAt),
	}); err != nil {
		return err
	}
	if entries != nil {
		return postEntries(ctx, q, inv.AdministrationID, journal.ExternalSalesInvoice, inv.ID, entries, inv.UpdatedAt)
	}
	return nil
}

// DeleteExternalInvoice implements sales.Store: the invoice goes with its
// payments and what they all post.
func (s *SalesStore) DeleteExternalInvoice(ctx context.Context, admin, id resource.ID) error {
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		q := s.q.WithTx(tx)
		payments, err := q.PaymentsOfInvoice(ctx, sqlc.PaymentsOfInvoiceParams{AdministrationID: int64(admin), InvoiceType: journal.ExternalSalesInvoice, InvoiceID: int64(id)})
		if err != nil {
			return err
		}
		for _, p := range payments {
			if err := q.DeleteJournalEntriesOf(ctx, sqlc.DeleteJournalEntriesOfParams{AdministrationID: p.AdministrationID, DocumentType: journal.Payment, DocumentID: p.ID}); err != nil {
				return err
			}
			if err := q.DeletePayment(ctx, sqlc.DeletePaymentParams{AdministrationID: p.AdministrationID, ID: p.ID}); err != nil {
				return err
			}
		}
		if err := q.DeleteJournalEntriesOf(ctx, sqlc.DeleteJournalEntriesOfParams{AdministrationID: int64(admin), DocumentType: journal.ExternalSalesInvoice, DocumentID: int64(id)}); err != nil {
			return err
		}
		if err := q.DeleteNotesOf(ctx, sqlc.DeleteNotesOfParams{AdministrationID: int64(admin), EntityID: int64(id)}); err != nil {
			return err
		}
		if err := q.DeleteExternalSalesInvoice(ctx, sqlc.DeleteExternalSalesInvoiceParams{AdministrationID: int64(admin), ID: int64(id)}); err != nil {
			return err
		}
		return q.DeleteResource(ctx, int64(id))
	})
}

// TouchExternalInvoice implements sales.Store.
func (s *SalesStore) TouchExternalInvoice(ctx context.Context, admin, id resource.ID, at time.Time) error {
	return s.q.TouchExternalSalesInvoice(ctx, sqlc.TouchExternalSalesInvoiceParams{AdministrationID: int64(admin), ID: int64(id), UpdatedAt: timestamp(at)})
}

// completeExternal converts rows and loads their lines and payments.
func (s *SalesStore) completeExternal(ctx context.Context, admin resource.ID, rows []sqlc.ExternalSalesInvoice) ([]sales.ExternalInvoice, error) {
	out := make([]sales.ExternalInvoice, len(rows))
	if len(rows) == 0 {
		return out, nil
	}
	ids := make([]int64, len(rows))
	index := map[int64]int{}
	for i, r := range rows {
		ids[i], index[r.ID] = r.ID, i
		e, err := toExternalInvoice(r)
		if err != nil {
			return nil, err
		}
		out[i] = e
	}
	details, err := s.q.ExternalSalesInvoiceDetailsOf(ctx, sqlc.ExternalSalesInvoiceDetailsOfParams{AdministrationID: int64(admin), Ids: ids})
	if err != nil {
		return nil, err
	}
	for _, d := range details {
		e := &out[index[d.ExternalSalesInvoiceID]]
		e.Lines = append(e.Lines, sales.Detail{
			ID: resource.ID(d.ID), Description: d.Description, Period: d.Period, Price: money.Amount(d.Price), Amount: d.Amount, Quantity: d.Quantity,
			TaxRateID: resource.ID(d.TaxRateID), LedgerAccountID: idOf(d.LedgerAccountID), ProjectID: idOf(d.ProjectID), ProductID: idOf(d.ProductID),
			RowOrder: int(d.RowOrder), Selected: true, CreatedAt: d.CreatedAt.Time, UpdatedAt: d.UpdatedAt.Time,
		})
		e.Totals.LineBases = append(e.Totals.LineBases, money.Amount(d.LineBase))
	}
	payments, err := s.q.PaymentsOfInvoices(ctx, sqlc.PaymentsOfInvoicesParams{AdministrationID: int64(admin), InvoiceType: journal.ExternalSalesInvoice, Ids: ids})
	if err != nil {
		return nil, err
	}
	for _, p := range payments {
		e := &out[index[p.InvoiceID]]
		e.Payments = append(e.Payments, toPayment(p))
	}
	return out, nil
}

func toExternalInvoice(r sqlc.ExternalSalesInvoice) (sales.ExternalInvoice, error) {
	e := sales.ExternalInvoice{
		ID: resource.ID(r.ID), AdministrationID: resource.ID(r.AdministrationID), EntryNumber: int(r.EntryNumber), Reference: r.Reference,
		Date: dayOf(r.Date), DueDate: dateOf(r.DueDate), State: r.State, Origin: r.Origin, Source: r.Source, SourceURL: r.SourceUrl,
		Currency: r.Currency, PricesAreInclTax: r.PricesAreInclTax, PaidAt: timeOf(r.PaidAt), MarkedDubiousOn: dateOf(r.MarkedDubiousOn),
		MarkedUncollectibleOn: dateOf(r.MarkedUncollectibleOn),
		Totals:                sales.Totals{ExclTax: money.Amount(r.TotalExclTax), Tax: money.Amount(r.TotalTax), InclTax: money.Amount(r.TotalInclTax)},
		CreatedAt:             r.CreatedAt.Time, UpdatedAt: r.UpdatedAt.Time,
	}
	if r.ContactID.Valid {
		e.ContactID = resource.ID(r.ContactID.Int64)
	}
	rates, err := decodeTaxTotals(r.TaxTotals)
	if err != nil {
		return e, fmt.Errorf("external invoice %d: %w", r.ID, err)
	}
	e.Totals.ByRate = rates
	return e, nil
}

// encodeTaxTotals turns the VAT per rate into JSON.
func encodeTaxTotals(t sales.Totals) ([]byte, error) {
	rates := make([]rateTotalJSON, len(t.ByRate))
	for i, r := range t.ByRate {
		rates[i] = rateTotalJSON{TaxRateID: r.TaxRateID.String(), Taxable: int64(r.Taxable), Tax: int64(r.Tax)}
	}
	return json.Marshal(rates)
}

// decodeTaxTotals reads the VAT per rate from JSON.
func decodeTaxTotals(raw []byte) ([]sales.RateTotal, error) {
	var rates []rateTotalJSON
	if err := json.Unmarshal(raw, &rates); err != nil {
		return nil, fmt.Errorf("tax totals: %w", err)
	}
	out := make([]sales.RateTotal, 0, len(rates))
	for _, t := range rates {
		id, err := resource.ParseID(t.TaxRateID)
		if err != nil {
			return nil, fmt.Errorf("tax totals: %w", err)
		}
		out = append(out, sales.RateTotal{TaxRateID: id, Taxable: money.Amount(t.Taxable), Tax: money.Amount(t.Tax)})
	}
	return out, nil
}
