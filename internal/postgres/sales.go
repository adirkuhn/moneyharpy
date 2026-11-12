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

// PaymentReferenceTaken implements sales.Store.
func (s *SalesStore) PaymentReferenceTaken(ctx context.Context, admin resource.ID, ref string) (bool, error) {
	return s.q.PaymentReferenceTaken(ctx, sqlc.PaymentReferenceTakenParams{AdministrationID: int64(admin), PaymentReference: ref})
}

// SaveInvoice implements sales.Store.
func (s *SalesStore) SaveInvoice(ctx context.Context, inv sales.Invoice, entries []journal.Entry, created bool) error {
	delivery, taxTotals, err := encodeInvoice(inv)
	if err != nil {
		return err
	}
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		return saveInvoice(ctx, s.q.WithTx(tx), inv, delivery, taxTotals, entries, created)
	})
}

func saveInvoice(ctx context.Context, q *sqlc.Queries, inv sales.Invoice, delivery, taxTotals []byte, entries []journal.Entry, created bool) error {
	if err := q.UpsertSalesInvoice(ctx, sqlc.UpsertSalesInvoiceParams{
		ID: int64(inv.ID), AdministrationID: int64(inv.AdministrationID), ContactID: int64(inv.ContactID), ContactPersonID: optionalID(inv.ContactPersonID),
		IdentityID: int64(inv.IdentityID), DocumentStyleID: int64(inv.DocumentStyleID), WorkflowID: int64(inv.WorkflowID), DraftID: int32(inv.DraftID),
		InvoiceID: inv.InvoiceID, SequenceID: inv.SequenceID, Reference: inv.Reference, Language: inv.Language, Currency: inv.Currency,
		PricesAreInclTax: inv.PricesAreInclTax, PaymentConditions: inv.PaymentConditions, Discount: int32(inv.Discount),
		InvoiceDate: optionalDate(inv.InvoiceDate), DueDate: optionalDate(inv.DueDate), FirstDueInterval: int32(inv.FirstDueInterval),
		State: inv.State, Paused: inv.Paused, PaymentReference: inv.PaymentReference, SentAt: optionalTime(inv.SentAt), PaidAt: optionalTime(inv.PaidAt),
		MarkedDubiousOn: optionalDate(inv.MarkedDubiousOn), MarkedUncollectibleOn: optionalDate(inv.MarkedUncollectibleOn),
		ReminderCount: int32(inv.ReminderCount), LastReminderOn: optionalDate(inv.LastReminderOn), OriginalID: optionalID(inv.OriginalID),
		RecurringSalesInvoiceID: optionalID(inv.RecurringSalesInvoiceID), Delivery: delivery, TotalExclTax: int64(inv.Totals.ExclTax), TotalTax: int64(inv.Totals.Tax), TotalInclTax: int64(inv.Totals.InclTax),
		TotalDiscount: int64(inv.Totals.Discount), TaxTotals: taxTotals, CreatedAt: timestamp(inv.CreatedAt), UpdatedAt: timestamp(inv.UpdatedAt),
	}); err != nil {
		return err
	}
	if created {
		if err := q.InsertResource(ctx, sqlc.InsertResourceParams{ID: int64(inv.ID), Type: string(resource.TypeSalesInvoice), AdministrationID: int64(inv.AdministrationID)}); err != nil {
			return err
		}
	}
	if inv.OriginalEstimateID != nil {
		if err := q.SetSalesInvoiceOriginalEstimate(ctx, sqlc.SetSalesInvoiceOriginalEstimateParams{
			AdministrationID: int64(inv.AdministrationID), ID: int64(inv.ID), OriginalEstimateID: optionalID(inv.OriginalEstimateID),
		}); err != nil {
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
}

// encodeInvoice turns the invoice's delivery and VAT totals into JSON.
func encodeInvoice(inv sales.Invoice) (delivery, taxTotals []byte, err error) {
	if inv.Delivery != nil {
		if delivery, err = json.Marshal(inv.Delivery); err != nil {
			return nil, nil, err
		}
	}
	taxTotals, err = encodeTaxTotals(inv.Totals)
	return delivery, taxTotals, err
}

// Payment implements sales.Store.
func (s *SalesStore) Payment(ctx context.Context, admin, id resource.ID) (sales.Payment, error) {
	r, err := s.q.PaymentByID(ctx, sqlc.PaymentByIDParams{AdministrationID: int64(admin), ID: int64(id)})
	if errors.Is(err, pgx.ErrNoRows) {
		return sales.Payment{}, sales.ErrPaymentNotFound
	}
	if err != nil {
		return sales.Payment{}, err
	}
	return toPayment(r), nil
}

// SavePayments implements sales.Store.
func (s *SalesStore) SavePayments(ctx context.Context, c sales.PaymentChange) error {
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		q := s.q.WithTx(tx)
		for _, id := range c.Removed {
			p, err := q.PaymentByID(ctx, sqlc.PaymentByIDParams{AdministrationID: adminOf(c), ID: int64(id)})
			if errors.Is(err, pgx.ErrNoRows) {
				continue
			}
			if err != nil {
				return err
			}
			if err := q.DeleteJournalEntriesOf(ctx, sqlc.DeleteJournalEntriesOfParams{AdministrationID: p.AdministrationID, DocumentType: journal.Payment, DocumentID: p.ID}); err != nil {
				return err
			}
			if err := q.DeletePayment(ctx, sqlc.DeletePaymentParams{AdministrationID: p.AdministrationID, ID: p.ID}); err != nil {
				return err
			}
		}
		for _, pp := range c.Payments {
			p := pp.Payment
			if err := q.UpsertPayment(ctx, sqlc.UpsertPaymentParams{
				ID: int64(p.ID), AdministrationID: int64(p.AdministrationID), InvoiceType: p.InvoiceType, InvoiceID: int64(p.InvoiceID),
				FinancialAccountID: optionalID(p.FinancialAccountID), FinancialMutationID: optionalID(p.FinancialMutationID),
				LedgerAccountID: optionalID(p.LedgerAccountID), CreditInvoiceID: optionalID(p.CreditInvoiceID), LinkedPaymentID: optionalID(p.LinkedPaymentID),
				ManualPaymentAction: p.ManualPaymentAction, Price: int64(p.Price), PriceBase: int64(p.PriceBase), PaymentDate: day(p.PaymentDate),
				TransactionIdentifier: p.TransactionIdentifier, UserID: optionalID(&p.UserID), CreatedAt: timestamp(p.CreatedAt), UpdatedAt: timestamp(p.UpdatedAt),
			}); err != nil {
				return err
			}
			entries := pp.Entries
			if entries == nil {
				entries = []journal.Entry{}
			}
			if err := postEntries(ctx, q, p.AdministrationID, journal.Payment, p.ID, entries, p.UpdatedAt); err != nil {
				return err
			}
		}
		for _, posting := range c.Invoices {
			if posting.Invoice.Type == sales.ExternalEntityType {
				if err := settleExternal(ctx, q, posting.Invoice, posting.Entries); err != nil {
					return err
				}
				continue
			}
			delivery, taxTotals, err := encodeInvoice(posting.Invoice)
			if err != nil {
				return err
			}
			if err := saveInvoice(ctx, q, posting.Invoice, delivery, taxTotals, posting.Entries, false); err != nil {
				return err
			}
		}
		return nil
	})
}

// adminOf is the administration a payment change is in.
func adminOf(c sales.PaymentChange) int64 {
	if len(c.Invoices) > 0 {
		return int64(c.Invoices[0].Invoice.AdministrationID)
	}
	if len(c.Payments) > 0 {
		return int64(c.Payments[0].Payment.AdministrationID)
	}
	return 0
}

// Receivables implements sales.Store.
func (s *SalesStore) Receivables(ctx context.Context, admin resource.ID, today time.Time) (sales.Receivables, error) {
	r, err := s.q.SalesReceivables(ctx, sqlc.SalesReceivablesParams{AdministrationID: int64(admin), Today: day(today)})
	if err != nil {
		return sales.Receivables{}, err
	}
	return sales.Receivables{
		ToReceive: sales.Tally{Count: int(r.ReceiveCount), Amount: money.Amount(r.ReceiveAmount)},
		ToRemind:  sales.Tally{Count: int(r.RemindCount), Amount: money.Amount(r.RemindAmount)},
	}, nil
}

// Workload implements sales.Store.
func (s *SalesStore) Workload(ctx context.Context) ([]sales.Workload, error) {
	rows, err := s.q.SalesWorkload(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]sales.Workload, len(rows))
	for i, r := range rows {
		out[i] = sales.Workload{AdministrationID: resource.ID(r.AdministrationID), UserID: resource.ID(r.UserID)}
	}
	return out, nil
}

// workLock keeps the advisory locks of the sales background job apart
// from any other: the administration's id is the second key.
const workLock = 1517

// Lock implements sales.Store with a session advisory lock, held on its
// own connection until release.
func (s *SalesStore) Lock(ctx context.Context, admin resource.ID) (func(), bool, error) {
	conn, err := s.pool.Acquire(ctx)
	if err != nil {
		return nil, false, err
	}
	key := int64(workLock)<<32 ^ int64(admin)
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

func toPayment(r sqlc.Payment) sales.Payment {
	p := sales.Payment{
		ID: resource.ID(r.ID), AdministrationID: resource.ID(r.AdministrationID), InvoiceType: r.InvoiceType, InvoiceID: resource.ID(r.InvoiceID),
		FinancialAccountID: idOf(r.FinancialAccountID), FinancialMutationID: idOf(r.FinancialMutationID), LedgerAccountID: idOf(r.LedgerAccountID),
		CreditInvoiceID: idOf(r.CreditInvoiceID), LinkedPaymentID: idOf(r.LinkedPaymentID), ManualPaymentAction: r.ManualPaymentAction,
		Price: money.Amount(r.Price), PriceBase: money.Amount(r.PriceBase), PaymentDate: *dateOf(r.PaymentDate),
		TransactionIdentifier: r.TransactionIdentifier, CreatedAt: r.CreatedAt.Time, UpdatedAt: r.UpdatedAt.Time,
	}
	if r.UserID.Valid {
		p.UserID = resource.ID(r.UserID.Int64)
	}
	return p
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
		if err := q.DeleteEventsOf(ctx, sqlc.DeleteEventsOfParams{AdministrationID: int64(admin), EntityID: int64(id)}); err != nil {
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
	used, err := s.q.ContactOnSalesInvoice(ctx, sqlc.ContactOnSalesInvoiceParams{AdministrationID: int64(admin), ContactID: int64(contact)})
	if err != nil || used {
		return used, err
	}
	return s.contactOnEstimate(ctx, admin, contact)
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
	subscribed, err := s.q.SubscriptionsOfSalesInvoices(ctx, sqlc.SubscriptionsOfSalesInvoicesParams{AdministrationID: int64(admin), Ids: ids})
	if err != nil {
		return nil, err
	}
	for _, l := range subscribed {
		id := resource.ID(l.SubscriptionID)
		out[index[l.SalesInvoiceID]].SubscriptionID = &id
	}
	values, err := s.q.SalesInvoiceCustomFieldValuesOf(ctx, sqlc.SalesInvoiceCustomFieldValuesOfParams{AdministrationID: int64(admin), Ids: ids})
	if err != nil {
		return nil, err
	}
	for _, v := range values {
		inv := &out[index[v.SalesInvoiceID]]
		inv.CustomFields = append(inv.CustomFields, sales.FieldValue{FieldID: resource.ID(v.ID), Name: v.Name, Value: v.Value})
	}
	payments, err := s.q.PaymentsOfInvoices(ctx, sqlc.PaymentsOfInvoicesParams{AdministrationID: int64(admin), InvoiceType: journal.SalesInvoice, Ids: ids})
	if err != nil {
		return nil, err
	}
	for _, p := range payments {
		inv := &out[index[p.InvoiceID]]
		inv.Payments = append(inv.Payments, toPayment(p))
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
		ReminderCount: int(r.ReminderCount), LastReminderOn: dateOf(r.LastReminderOn), OriginalID: idOf(r.OriginalID),
		RecurringSalesInvoiceID: idOf(r.RecurringSalesInvoiceID), OriginalEstimateID: idOf(r.OriginalEstimateID),
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
	rates, err := decodeTaxTotals(r.TaxTotals)
	if err != nil {
		return inv, fmt.Errorf("invoice %d: %w", r.ID, err)
	}
	if len(rates) > 0 {
		inv.Totals.ByRate = rates
	}
	return inv, nil
}
