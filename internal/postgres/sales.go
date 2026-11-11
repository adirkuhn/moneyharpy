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
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/adirkuhn/moneyharpy/internal/journal"
	"github.com/adirkuhn/moneyharpy/internal/money"
	"github.com/adirkuhn/moneyharpy/internal/postgres/sqlc"
	"github.com/adirkuhn/moneyharpy/internal/resource"
	"github.com/adirkuhn/moneyharpy/internal/sales"
)

// SalesStore implements sales.Store.
type SalesStore struct {
	pool *pgxpool.Pool
	q    *sqlc.Queries
}

// NewSalesStore returns a SalesStore on pool.
func NewSalesStore(pool *pgxpool.Pool) *SalesStore {
	return &SalesStore{pool: pool, q: sqlc.New(pool)}
}

type rateTotalJSON struct {
	TaxRateID string `json:"tax_rate_id"`
	Taxable   int64  `json:"taxable"`
	Tax       int64  `json:"tax"`
}

// Invoices implements sales.Store.
func (s *SalesStore) Invoices(ctx context.Context, admin resource.ID, q sales.Query) ([]sales.Invoice, int, error) {
	p := sqlc.ListSalesInvoicesParams{
		AdministrationID: int64(admin), States: q.States, Today: day(q.Today), Refs: q.References, ContactID: optionalID(q.ContactID),
		RecurringID: optionalID(q.RecurringSalesInvoiceID), WorkflowID: optionalID(q.WorkflowID), CreatedAfter: optionalTime(q.CreatedAfter),
		UpdatedAfter: optionalTime(q.UpdatedAfter), Ascending: q.Ascending,
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
	rows, err := s.q.ListSalesInvoices(ctx, p)
	if err != nil {
		return nil, 0, err
	}
	list := make([]sqlc.SalesInvoice, len(rows))
	total := 0
	for i, r := range rows {
		list[i], total = r.SalesInvoice, int(r.Total)
	}
	out, err := s.complete(ctx, admin, list)
	return out, total, err
}

func (s *SalesStore) one(ctx context.Context, admin resource.ID, load func() (sqlc.SalesInvoice, error)) (sales.Invoice, error) {
	r, err := load()
	if errors.Is(err, pgx.ErrNoRows) {
		return sales.Invoice{}, sales.ErrNotFound
	}
	if err != nil {
		return sales.Invoice{}, err
	}
	out, err := s.complete(ctx, admin, []sqlc.SalesInvoice{r})
	if err != nil {
		return sales.Invoice{}, err
	}
	return out[0], nil
}

// Invoice implements sales.Store.
func (s *SalesStore) Invoice(ctx context.Context, admin, id resource.ID) (sales.Invoice, error) {
	return s.one(ctx, admin, func() (sqlc.SalesInvoice, error) {
		return s.q.SalesInvoiceByID(ctx, sqlc.SalesInvoiceByIDParams{AdministrationID: int64(admin), ID: int64(id)})
	})
}

// InvoiceByNumber implements sales.Store.
func (s *SalesStore) InvoiceByNumber(ctx context.Context, admin resource.ID, number string) (sales.Invoice, error) {
	return s.one(ctx, admin, func() (sqlc.SalesInvoice, error) {
		return s.q.SalesInvoiceByNumber(ctx, sqlc.SalesInvoiceByNumberParams{AdministrationID: int64(admin), InvoiceID: number})
	})
}

// InvoiceByReference implements sales.Store.
func (s *SalesStore) InvoiceByReference(ctx context.Context, admin resource.ID, reference string) (sales.Invoice, error) {
	return s.one(ctx, admin, func() (sqlc.SalesInvoice, error) {
		return s.q.SalesInvoiceByReference(ctx, sqlc.SalesInvoiceByReferenceParams{AdministrationID: int64(admin), Reference: reference})
	})
}

// InvoicesByID implements sales.Store.
func (s *SalesStore) InvoicesByID(ctx context.Context, admin resource.ID, ids []resource.ID) ([]sales.Invoice, error) {
	rows, err := s.q.SalesInvoicesByIDs(ctx, sqlc.SalesInvoicesByIDsParams{AdministrationID: int64(admin), Ids: ids64(ids)})
	if err != nil {
		return nil, err
	}
	return s.complete(ctx, admin, rows)
}

// NextDraftID implements sales.Store.
func (s *SalesStore) NextDraftID(ctx context.Context, admin resource.ID) (int, error) {
	n, err := s.q.NextSalesInvoiceDraftID(ctx, int64(admin))
	return int(n), err
}

// ReserveNumber implements sales.Store: it takes the sequence number and
// moves the counter on under a row lock, keeping its leading zeros.
func (s *SalesStore) ReserveNumber(ctx context.Context, admin resource.ID) (string, error) {
	var seq string
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		q := s.q.WithTx(tx)
		current, err := q.LockInvoiceSequence(ctx, int64(admin))
		if err != nil {
			return err
		}
		n, err := strconv.ParseInt(current, 10, 64)
		if err != nil {
			return fmt.Errorf("invoice sequence %q: %w", current, err)
		}
		next := strconv.FormatInt(n+1, 10)
		if len(next) < len(current) {
			next = strings.Repeat("0", len(current)-len(next)) + next
		}
		seq = current
		return q.SetInvoiceSequence(ctx, sqlc.SetInvoiceSequenceParams{ID: int64(admin), InvoiceSequenceID: next})
	})
	return seq, err
}

// NumberTaken implements sales.Store.
func (s *SalesStore) NumberTaken(ctx context.Context, admin resource.ID, number string, except resource.ID) (bool, error) {
	return s.q.SalesInvoiceNumberTaken(ctx, sqlc.SalesInvoiceNumberTakenParams{AdministrationID: int64(admin), InvoiceID: number, ID: int64(except)})
}

// SaveInvoice implements sales.Store.
func (s *SalesStore) SaveInvoice(ctx context.Context, inv sales.Invoice, entries []journal.Entry, created bool) error {
	var delivery []byte
	if inv.Delivery != nil {
		var err error
		if delivery, err = json.Marshal(inv.Delivery); err != nil {
			return err
		}
	}
	rates := make([]rateTotalJSON, len(inv.Totals.ByRate))
	for i, r := range inv.Totals.ByRate {
		rates[i] = rateTotalJSON{TaxRateID: r.TaxRateID.String(), Taxable: int64(r.Taxable), Tax: int64(r.Tax)}
	}
	taxTotals, err := json.Marshal(rates)
	if err != nil {
		return err
	}
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		q := s.q.WithTx(tx)
		if err := q.UpsertSalesInvoice(ctx, sqlc.UpsertSalesInvoiceParams{
			ID: int64(inv.ID), AdministrationID: int64(inv.AdministrationID), ContactID: int64(inv.ContactID), ContactPersonID: optionalID(inv.ContactPersonID),
			IdentityID: int64(inv.IdentityID), DocumentStyleID: int64(inv.DocumentStyleID), WorkflowID: int64(inv.WorkflowID), DraftID: int32(inv.DraftID),
			InvoiceID: inv.InvoiceID, SequenceID: inv.SequenceID, Reference: inv.Reference, Language: inv.Language, Currency: inv.Currency,
			PricesAreInclTax: inv.PricesAreInclTax, PaymentConditions: inv.PaymentConditions, Discount: int32(inv.Discount),
			InvoiceDate: optionalDate(inv.InvoiceDate), DueDate: optionalDate(inv.DueDate), FirstDueInterval: int32(inv.FirstDueInterval),
			State: inv.State, Paused: inv.Paused, PaymentReference: inv.PaymentReference, SentAt: optionalTime(inv.SentAt), PaidAt: optionalTime(inv.PaidAt),
			MarkedDubiousOn: optionalDate(inv.MarkedDubiousOn), MarkedUncollectibleOn: optionalDate(inv.MarkedUncollectibleOn),
			ReminderCount: int32(inv.ReminderCount), OriginalID: optionalID(inv.OriginalID), RecurringSalesInvoiceID: optionalID(inv.RecurringSalesInvoiceID),
			Delivery: delivery, TotalExclTax: int64(inv.Totals.ExclTax), TotalTax: int64(inv.Totals.Tax), TotalInclTax: int64(inv.Totals.InclTax),
			TotalDiscount: int64(inv.Totals.Discount), TaxTotals: taxTotals, CreatedAt: timestamp(inv.CreatedAt), UpdatedAt: timestamp(inv.UpdatedAt),
		}); err != nil {
			return err
		}
		if created {
			if err := q.InsertResource(ctx, sqlc.InsertResourceParams{ID: int64(inv.ID), Type: string(resource.TypeSalesInvoice), AdministrationID: int64(inv.AdministrationID)}); err != nil {
				return err
			}
		}
		if err := q.DeleteSalesInvoiceDetails(ctx, int64(inv.ID)); err != nil {
			return err
		}
		for i, d := range inv.Lines {
			var base int64
			if i < len(inv.Totals.LineBases) {
				base = int64(inv.Totals.LineBases[i])
			}
			if err := q.InsertSalesInvoiceDetail(ctx, sqlc.InsertSalesInvoiceDetailParams{
				ID: int64(d.ID), AdministrationID: int64(inv.AdministrationID), SalesInvoiceID: int64(inv.ID), Description: d.Description, Period: d.Period,
				Price: int64(d.Price), Amount: d.Amount, Quantity: d.Quantity, TaxRateID: int64(d.TaxRateID), LedgerAccountID: optionalID(d.LedgerAccountID),
				ProjectID: optionalID(d.ProjectID), ProductID: optionalID(d.ProductID), RowOrder: int32(d.RowOrder), IsOptional: d.Optional,
				IsSelected: d.Selected, LineBase: base, CreatedAt: timestamp(d.CreatedAt), UpdatedAt: timestamp(d.UpdatedAt),
			}); err != nil {
				return err
			}
		}
		if err := q.DeleteSalesInvoiceCustomFieldValues(ctx, int64(inv.ID)); err != nil {
			return err
		}
		for _, v := range inv.CustomFields {
			if err := q.InsertSalesInvoiceCustomFieldValue(ctx, sqlc.InsertSalesInvoiceCustomFieldValueParams{SalesInvoiceID: int64(inv.ID), CustomFieldID: int64(v.FieldID), Value: v.Value}); err != nil {
				return err
			}
		}
		if entries != nil {
			return postEntries(ctx, q, inv.AdministrationID, journal.SalesInvoice, inv.ID, entries, inv.UpdatedAt)
		}
		return nil
	})
}

// DeleteInvoice implements sales.Store.
func (s *SalesStore) DeleteInvoice(ctx context.Context, admin, id resource.ID) error {
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		q := s.q.WithTx(tx)
		if err := q.DeleteJournalEntriesOf(ctx, sqlc.DeleteJournalEntriesOfParams{AdministrationID: int64(admin), DocumentType: journal.SalesInvoice, DocumentID: int64(id)}); err != nil {
			return err
		}
		if err := q.DeleteNotesOf(ctx, sqlc.DeleteNotesOfParams{AdministrationID: int64(admin), EntityID: int64(id)}); err != nil {
			return err
		}
		if err := q.DeleteSalesInvoice(ctx, sqlc.DeleteSalesInvoiceParams{AdministrationID: int64(admin), ID: int64(id)}); err != nil {
			return err
		}
		return q.DeleteResource(ctx, int64(id))
	})
}

// TouchInvoice implements sales.Store.
func (s *SalesStore) TouchInvoice(ctx context.Context, admin, id resource.ID, at time.Time) error {
	return s.q.TouchSalesInvoice(ctx, sqlc.TouchSalesInvoiceParams{AdministrationID: int64(admin), ID: int64(id), UpdatedAt: timestamp(at)})
}

// ContactInUse implements sales.Store and contacts.References.
func (s *SalesStore) ContactInUse(ctx context.Context, admin, contact resource.ID) (bool, error) {
	return s.q.ContactOnSalesInvoice(ctx, sqlc.ContactOnSalesInvoiceParams{AdministrationID: int64(admin), ContactID: int64(contact)})
}

// complete converts rows and loads their lines and custom fields.
func (s *SalesStore) complete(ctx context.Context, admin resource.ID, rows []sqlc.SalesInvoice) ([]sales.Invoice, error) {
	out := make([]sales.Invoice, len(rows))
	if len(rows) == 0 {
		return out, nil
	}
	ids := make([]int64, len(rows))
	index := map[int64]int{}
	for i, r := range rows {
		ids[i], index[r.ID] = r.ID, i
		inv, err := toInvoice(r)
		if err != nil {
			return nil, err
		}
		out[i] = inv
	}
	details, err := s.q.SalesInvoiceDetailsOf(ctx, sqlc.SalesInvoiceDetailsOfParams{AdministrationID: int64(admin), Ids: ids})
	if err != nil {
		return nil, err
	}
	for _, d := range details {
		inv := &out[index[d.SalesInvoiceID]]
		inv.Lines = append(inv.Lines, sales.Detail{
			ID: resource.ID(d.ID), Description: d.Description, Period: d.Period, Price: money.Amount(d.Price), Amount: d.Amount, Quantity: d.Quantity,
			TaxRateID: resource.ID(d.TaxRateID), LedgerAccountID: idOf(d.LedgerAccountID), ProjectID: idOf(d.ProjectID), ProductID: idOf(d.ProductID),
			RowOrder: int(d.RowOrder), Optional: d.IsOptional, Selected: d.IsSelected, CreatedAt: d.CreatedAt.Time, UpdatedAt: d.UpdatedAt.Time,
		})
		inv.Totals.LineBases = append(inv.Totals.LineBases, money.Amount(d.LineBase))
	}
	values, err := s.q.SalesInvoiceCustomFieldValuesOf(ctx, sqlc.SalesInvoiceCustomFieldValuesOfParams{AdministrationID: int64(admin), Ids: ids})
	if err != nil {
		return nil, err
	}
	for _, v := range values {
		inv := &out[index[v.SalesInvoiceID]]
		inv.CustomFields = append(inv.CustomFields, sales.FieldValue{FieldID: resource.ID(v.ID), Name: v.Name, Value: v.Value})
	}
	return out, nil
}

func toInvoice(r sqlc.SalesInvoice) (sales.Invoice, error) {
	inv := sales.Invoice{
		ID: resource.ID(r.ID), AdministrationID: resource.ID(r.AdministrationID), ContactID: resource.ID(r.ContactID), ContactPersonID: idOf(r.ContactPersonID),
		IdentityID: resource.ID(r.IdentityID), DocumentStyleID: resource.ID(r.DocumentStyleID), WorkflowID: resource.ID(r.WorkflowID), DraftID: int(r.DraftID),
		InvoiceID: r.InvoiceID, SequenceID: r.SequenceID, Reference: r.Reference, Language: r.Language, Currency: r.Currency,
		PricesAreInclTax: r.PricesAreInclTax, PaymentConditions: r.PaymentConditions, Discount: int64(r.Discount), InvoiceDate: dateOf(r.InvoiceDate),
		DueDate: dateOf(r.DueDate), FirstDueInterval: int(r.FirstDueInterval), State: r.State, Paused: r.Paused, PaymentReference: r.PaymentReference,
		SentAt: timeOf(r.SentAt), PaidAt: timeOf(r.PaidAt), MarkedDubiousOn: dateOf(r.MarkedDubiousOn), MarkedUncollectibleOn: dateOf(r.MarkedUncollectibleOn),
		ReminderCount: int(r.ReminderCount), OriginalID: idOf(r.OriginalID), RecurringSalesInvoiceID: idOf(r.RecurringSalesInvoiceID),
		Totals: sales.Totals{
			ExclTax: money.Amount(r.TotalExclTax), Tax: money.Amount(r.TotalTax), InclTax: money.Amount(r.TotalInclTax), Discount: money.Amount(r.TotalDiscount),
		},
		CreatedAt: r.CreatedAt.Time, UpdatedAt: r.UpdatedAt.Time,
	}
	if len(r.Delivery) > 0 {
		var d sales.Delivery
		if err := json.Unmarshal(r.Delivery, &d); err != nil {
			return inv, fmt.Errorf("invoice %d delivery: %w", r.ID, err)
		}
		inv.Delivery = &d
	}
	var rates []rateTotalJSON
	if err := json.Unmarshal(r.TaxTotals, &rates); err != nil {
		return inv, fmt.Errorf("invoice %d tax totals: %w", r.ID, err)
	}
	for _, t := range rates {
		id, err := resource.ParseID(t.TaxRateID)
		if err != nil {
			return inv, fmt.Errorf("invoice %d tax totals: %w", r.ID, err)
		}
		inv.Totals.ByRate = append(inv.Totals.ByRate, sales.RateTotal{TaxRateID: id, Taxable: money.Amount(t.Taxable), Tax: money.Amount(t.Tax)})
	}
	return inv, nil
}
