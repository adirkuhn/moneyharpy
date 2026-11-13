package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/adirkuhn/moneyharpy/internal/journal"
	"github.com/adirkuhn/moneyharpy/internal/money"
	"github.com/adirkuhn/moneyharpy/internal/postgres/sqlc"
	"github.com/adirkuhn/moneyharpy/internal/purchases"
	"github.com/adirkuhn/moneyharpy/internal/resource"
	"github.com/adirkuhn/moneyharpy/internal/sales"
)

// PurchaseStore implements purchases.Store.
type PurchaseStore struct {
	pool *pgxpool.Pool
	q    *sqlc.Queries
}

// NewPurchaseStore returns a PurchaseStore on pool.
func NewPurchaseStore(pool *pgxpool.Pool) *PurchaseStore {
	return &PurchaseStore{pool: pool, q: sqlc.New(pool)}
}

func optionalText(s string) pgtype.Text { return pgtype.Text{String: s, Valid: s != ""} }

// page converts a page of a query to a limit and an offset.
func page(n, perPage int) (pgtype.Int4, int32) {
	if perPage <= 0 {
		return pgtype.Int4{}, 0
	}
	return pgtype.Int4{Int32: int32(perPage), Valid: true}, int32((max(n, 1) - 1) * perPage)
}

func period(p *money.Period) (from, to pgtype.Date) {
	if p == nil {
		return pgtype.Date{}, pgtype.Date{}
	}
	return day(p.From), day(p.To)
}

// Documents implements purchases.Store.
func (s *PurchaseStore) Documents(ctx context.Context, admin resource.ID, q purchases.Query) ([]purchases.Document, int, error) {
	p := sqlc.ListPurchaseDocumentsParams{
		AdministrationID: int64(admin), Kind: q.Kind, Nothing: q.None, States: nonNil(q.States), Today: day(q.Today), ContactID: optionalID(q.ContactID),
		Refs: nonNil(q.References), LedgerAccountID: optionalID(q.LedgerAccountID), UpdatedAfter: optionalTime(q.UpdatedAfter), Attachment: q.Attachment,
		Ascending: q.Ascending,
	}
	p.FromDate, p.ToDate = period(q.Period)
	p.MaxRows, p.Skip = page(q.Page, q.PerPage)
	rows, err := s.q.ListPurchaseDocuments(ctx, p)
	if err != nil {
		return nil, 0, err
	}
	list := make([]sqlc.PurchaseDocument, len(rows))
	total := 0
	for i, r := range rows {
		list[i], total = r.PurchaseDocument, int(r.Total)
	}
	out, err := s.complete(ctx, admin, list)
	return out, total, err
}

// Document implements purchases.Store.
func (s *PurchaseStore) Document(ctx context.Context, admin resource.ID, kind string, id resource.ID) (purchases.Document, error) {
	d, err := s.DocumentByID(ctx, admin, id)
	if err == nil && d.Kind != kind {
		return purchases.Document{}, purchases.ErrNotFound
	}
	return d, err
}

// DocumentByID implements purchases.Store.
func (s *PurchaseStore) DocumentByID(ctx context.Context, admin, id resource.ID) (purchases.Document, error) {
	r, err := s.q.PurchaseDocumentByID(ctx, sqlc.PurchaseDocumentByIDParams{AdministrationID: int64(admin), ID: int64(id)})
	if errors.Is(err, pgx.ErrNoRows) {
		return purchases.Document{}, purchases.ErrNotFound
	}
	if err != nil {
		return purchases.Document{}, err
	}
	out, err := s.complete(ctx, admin, []sqlc.PurchaseDocument{r})
	if err != nil {
		return purchases.Document{}, err
	}
	return out[0], nil
}

// DocumentsByID implements purchases.Store.
func (s *PurchaseStore) DocumentsByID(ctx context.Context, admin resource.ID, kind string, ids []resource.ID) ([]purchases.Document, error) {
	rows, err := s.q.PurchaseDocumentsByIDs(ctx, sqlc.PurchaseDocumentsByIDsParams{AdministrationID: int64(admin), Kind: kind, Ids: ids64(ids)})
	if err != nil {
		return nil, err
	}
	return s.complete(ctx, admin, rows)
}

// NextEntryNumber implements purchases.Store.
func (s *PurchaseStore) NextEntryNumber(ctx context.Context, admin resource.ID) (int, error) {
	n, err := s.q.NextDocumentEntryNumber(ctx, int64(admin))
	return int(n), err
}

// SaveDocument implements purchases.Store.
func (s *PurchaseStore) SaveDocument(ctx context.Context, d purchases.Document, entries []journal.Entry, created bool) error {
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		q := s.q.WithTx(tx)
		if err := storeDocument(ctx, q, d); err != nil {
			return err
		}
		if created {
			if err := q.InsertResource(ctx, sqlc.InsertResourceParams{ID: int64(d.ID), Type: string(d.Ref().Type), AdministrationID: int64(d.AdministrationID)}); err != nil {
				return err
			}
		}
		if entries == nil {
			return nil
		}
		return postEntries(ctx, q, d.AdministrationID, d.Kind, d.ID, entries, d.UpdatedAt)
	})
}

// storeDocument stores a purchase invoice or receipt with its lines and
// fiscal allocations, replacing those it had.
func storeDocument(ctx context.Context, q *sqlc.Queries, d purchases.Document) error {
	taxTotals, err := encodeTaxTotals(d.Totals)
	if err != nil {
		return err
	}
	admin := int64(d.AdministrationID)
	if err := q.UpsertPurchaseDocument(ctx, sqlc.UpsertPurchaseDocumentParams{
		ID: int64(d.ID), AdministrationID: admin, Kind: d.Kind, ContactID: optionalID(nonZero(d.ContactID)), EntryNumber: int32(d.EntryNumber),
		Reference: d.Reference, Date: day(d.Date), DueDate: optionalDate(d.DueDate), State: d.State, Origin: d.Origin, Currency: d.Currency,
		PricesAreInclTax: d.PricesAreInclTax, RevenueInvoice: d.RevenueInvoice, PaidAt: optionalTime(d.PaidAt),
		TotalExclTax: int64(d.Totals.ExclTax), TotalTax: int64(d.Totals.Tax), TotalInclTax: int64(d.Totals.InclTax), TaxTotals: taxTotals,
		CreatedAt: timestamp(d.CreatedAt), UpdatedAt: timestamp(d.UpdatedAt),
	}); err != nil {
		return err
	}
	if err := q.DeletePurchaseDocumentDetails(ctx, int64(d.ID)); err != nil {
		return err
	}
	for i, l := range d.Lines {
		var base int64
		if i < len(d.Totals.LineBases) {
			base = int64(d.Totals.LineBases[i])
		}
		if err := q.InsertPurchaseDocumentDetail(ctx, sqlc.InsertPurchaseDocumentDetailParams{
			ID: int64(l.ID), AdministrationID: admin, PurchaseDocumentID: int64(d.ID), Description: l.Description, Period: l.Period,
			Price: int64(l.Price), Amount: l.Amount, Quantity: l.Quantity, TaxRateID: int64(l.TaxRateID), LedgerAccountID: optionalID(l.LedgerAccountID),
			ProjectID: optionalID(l.ProjectID), ProductID: optionalID(l.ProductID), RowOrder: int32(l.RowOrder), LineBase: base,
			CreatedAt: timestamp(l.CreatedAt), UpdatedAt: timestamp(l.UpdatedAt),
		}); err != nil {
			return err
		}
	}
	if err := q.DeletePurchaseDocumentAllocations(ctx, int64(d.ID)); err != nil {
		return err
	}
	for _, f := range d.FiscalAllocations {
		if err := q.InsertPurchaseDocumentAllocation(ctx, sqlc.InsertPurchaseDocumentAllocationParams{
			ID: int64(f.ID), AdministrationID: admin, PurchaseDocumentID: int64(d.ID), LedgerAccountID: int64(f.LedgerAccountID),
			PrivateLedgerAccountID: int64(f.PrivateLedgerAccountID), Percentage: int32(f.Percentage), Price: int64(f.Price),
			CreatedAt: timestamp(f.CreatedAt), UpdatedAt: timestamp(f.UpdatedAt),
		}); err != nil {
			return err
		}
	}
	return nil
}

func nonZero(id resource.ID) *resource.ID {
	if id == 0 {
		return nil
	}
	return &id
}

// DeleteDocument implements purchases.Store: the document goes with its
// payments, purchase transactions and what they all post.
func (s *PurchaseStore) DeleteDocument(ctx context.Context, admin, id resource.ID) error {
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		q := s.q.WithTx(tx)
		d, err := q.PurchaseDocumentByID(ctx, sqlc.PurchaseDocumentByIDParams{AdministrationID: int64(admin), ID: int64(id)})
		if errors.Is(err, pgx.ErrNoRows) {
			return purchases.ErrNotFound
		}
		if err != nil {
			return err
		}
		payments, err := q.PaymentsOfInvoice(ctx, sqlc.PaymentsOfInvoiceParams{AdministrationID: int64(admin), InvoiceType: purchases.PaymentType, InvoiceID: int64(id)})
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
		if err := q.DeleteJournalEntriesOf(ctx, sqlc.DeleteJournalEntriesOfParams{AdministrationID: int64(admin), DocumentType: d.Kind, DocumentID: int64(id)}); err != nil {
			return err
		}
		if err := q.DeleteNotesOf(ctx, sqlc.DeleteNotesOfParams{AdministrationID: int64(admin), EntityID: int64(id)}); err != nil {
			return err
		}
		cancel := sqlc.CancelPurchaseTransactionsOfParams{AdministrationID: int64(admin), PayableID: int64(id), UpdatedAt: timestamp(time.Now())}
		if err := q.CancelPurchaseTransactionsOf(ctx, cancel); err != nil {
			return err
		}
		if err := q.DeletePurchaseDocument(ctx, sqlc.DeletePurchaseDocumentParams{AdministrationID: int64(admin), ID: int64(id)}); err != nil {
			return err
		}
		return q.DeleteResource(ctx, int64(id))
	})
}

// TouchDocument implements purchases.Store.
func (s *PurchaseStore) TouchDocument(ctx context.Context, admin, id resource.ID, at time.Time) error {
	return s.q.TouchPurchaseDocument(ctx, sqlc.TouchPurchaseDocumentParams{AdministrationID: int64(admin), ID: int64(id), UpdatedAt: timestamp(at)})
}

// Payment implements purchases.Store.
func (s *PurchaseStore) Payment(ctx context.Context, admin, id resource.ID) (sales.Payment, error) {
	r, err := s.q.PaymentByID(ctx, sqlc.PaymentByIDParams{AdministrationID: int64(admin), ID: int64(id)})
	if errors.Is(err, pgx.ErrNoRows) || err == nil && r.InvoiceType != purchases.PaymentType {
		return sales.Payment{}, purchases.ErrPaymentNotFound
	}
	if err != nil {
		return sales.Payment{}, err
	}
	return toPayment(r), nil
}

// SavePayments implements purchases.Store.
func (s *PurchaseStore) SavePayments(ctx context.Context, docs []purchases.Document, added []purchases.PaymentPosting, removed []resource.ID) error {
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		q := s.q.WithTx(tx)
		for _, id := range removed {
			for _, d := range docs {
				admin := int64(d.AdministrationID)
				if err := q.DeleteJournalEntriesOf(ctx, sqlc.DeleteJournalEntriesOfParams{AdministrationID: admin, DocumentType: journal.Payment, DocumentID: int64(id)}); err != nil {
					return err
				}
				if err := q.DeletePayment(ctx, sqlc.DeletePaymentParams{AdministrationID: admin, ID: int64(id)}); err != nil {
					return err
				}
			}
		}
		for _, pp := range added {
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
		for _, d := range docs {
			if err := q.SettlePurchaseDocument(ctx, sqlc.SettlePurchaseDocumentParams{
				AdministrationID: int64(d.AdministrationID), ID: int64(d.ID), State: d.State, PaidAt: optionalTime(d.PaidAt), UpdatedAt: timestamp(d.UpdatedAt),
			}); err != nil {
				return err
			}
		}
		return nil
	})
}

// complete converts rows and loads their lines, allocations, payments and
// whether a batch pays them.
func (s *PurchaseStore) complete(ctx context.Context, admin resource.ID, rows []sqlc.PurchaseDocument) ([]purchases.Document, error) {
	out := make([]purchases.Document, len(rows))
	if len(rows) == 0 {
		return out, nil
	}
	ids := make([]int64, len(rows))
	index := map[int64]int{}
	for i, r := range rows {
		ids[i], index[r.ID] = r.ID, i
		d, err := toPurchaseDocument(r)
		if err != nil {
			return nil, err
		}
		out[i] = d
	}
	details, err := s.q.PurchaseDocumentDetailsOf(ctx, sqlc.PurchaseDocumentDetailsOfParams{AdministrationID: int64(admin), Ids: ids})
	if err != nil {
		return nil, err
	}
	for _, l := range details {
		d := &out[index[l.PurchaseDocumentID]]
		d.Lines = append(d.Lines, sales.Detail{
			ID: resource.ID(l.ID), Description: l.Description, Period: l.Period, Price: money.Amount(l.Price), Amount: l.Amount, Quantity: l.Quantity,
			TaxRateID: resource.ID(l.TaxRateID), LedgerAccountID: idOf(l.LedgerAccountID), ProjectID: idOf(l.ProjectID), ProductID: idOf(l.ProductID),
			RowOrder: int(l.RowOrder), Selected: true, CreatedAt: l.CreatedAt.Time, UpdatedAt: l.UpdatedAt.Time,
		})
		d.Totals.LineBases = append(d.Totals.LineBases, money.Amount(l.LineBase))
	}
	allocations, err := s.q.PurchaseDocumentAllocationsOf(ctx, sqlc.PurchaseDocumentAllocationsOfParams{AdministrationID: int64(admin), Ids: ids})
	if err != nil {
		return nil, err
	}
	for _, f := range allocations {
		d := &out[index[f.PurchaseDocumentID]]
		d.FiscalAllocations = append(d.FiscalAllocations, purchases.FiscalAllocation{
			ID: resource.ID(f.ID), LedgerAccountID: resource.ID(f.LedgerAccountID), PrivateLedgerAccountID: resource.ID(f.PrivateLedgerAccountID),
			Percentage: int64(f.Percentage), Price: money.Amount(f.Price), CreatedAt: f.CreatedAt.Time, UpdatedAt: f.UpdatedAt.Time,
		})
	}
	payments, err := s.q.PaymentsOfInvoices(ctx, sqlc.PaymentsOfInvoicesParams{AdministrationID: int64(admin), InvoiceType: purchases.PaymentType, Ids: ids})
	if err != nil {
		return nil, err
	}
	for _, p := range payments {
		d := &out[index[p.InvoiceID]]
		d.Payments = append(d.Payments, toPayment(p))
	}
	pending, err := s.q.PendingPurchaseDocuments(ctx, sqlc.PendingPurchaseDocumentsParams{AdministrationID: int64(admin), Ids: ids})
	if err != nil {
		return nil, err
	}
	for _, id := range pending {
		out[index[id]].Pending = true
	}
	return out, nil
}

func toPurchaseDocument(r sqlc.PurchaseDocument) (purchases.Document, error) {
	d := purchases.Document{
		ID: resource.ID(r.ID), AdministrationID: resource.ID(r.AdministrationID), Kind: r.Kind, EntryNumber: int(r.EntryNumber), Reference: r.Reference,
		Date: dayOf(r.Date), DueDate: dateOf(r.DueDate), State: r.State, Origin: r.Origin, Currency: r.Currency, PricesAreInclTax: r.PricesAreInclTax,
		RevenueInvoice: r.RevenueInvoice, PaidAt: timeOf(r.PaidAt),
		Totals:    sales.Totals{ExclTax: money.Amount(r.TotalExclTax), Tax: money.Amount(r.TotalTax), InclTax: money.Amount(r.TotalInclTax)},
		CreatedAt: r.CreatedAt.Time, UpdatedAt: r.UpdatedAt.Time,
	}
	if r.ContactID.Valid {
		d.ContactID = resource.ID(r.ContactID.Int64)
	}
	rates, err := decodeTaxTotals(r.TaxTotals)
	if err != nil {
		return d, fmt.Errorf("document %d: %w", r.ID, err)
	}
	d.Totals.ByRate = rates
	return d, nil
}

// Totals implements purchases.Store.
func (s *PurchaseStore) Totals(ctx context.Context, admin resource.ID) (purchases.Summary, error) {
	r, err := s.q.PurchaseTotals(ctx, int64(admin))
	if err != nil {
		return purchases.Summary{}, err
	}
	return purchases.Summary{ToPay: purchases.Tally{Count: int(r.PayCount), Amount: money.Amount(r.PayAmount)}, ToProcess: int(r.ProcessCount)}, nil
}

// ContactInUse implements contacts.References.
func (s *PurchaseStore) ContactInUse(ctx context.Context, admin, contact resource.ID) (bool, error) {
	used, err := s.q.PurchaseContactInUse(ctx, sqlc.PurchaseContactInUseParams{Admin: int64(admin), Contact: int64(contact)})
	return used.Bool, err
}

// ProjectInUse implements projects.References.
func (s *PurchaseStore) ProjectInUse(ctx context.Context, admin, project resource.ID) (bool, error) {
	return s.q.PurchaseProjectInUse(ctx, sqlc.PurchaseProjectInUseParams{AdministrationID: int64(admin), ProjectID: optionalID(&project)})
}

// ProductInUse implements products.References.
func (s *PurchaseStore) ProductInUse(ctx context.Context, admin, product resource.ID) (bool, error) {
	return s.q.PurchaseProductInUse(ctx, sqlc.PurchaseProductInUseParams{AdministrationID: int64(admin), ProductID: optionalID(&product)})
}

// GeneralDocuments implements purchases.Store.
func (s *PurchaseStore) GeneralDocuments(ctx context.Context, admin resource.ID, q purchases.Query) ([]purchases.General, int, error) {
	p := sqlc.ListGeneralDocumentsParams{
		AdministrationID: int64(admin), Nothing: q.None, States: nonNil(q.States), ContactID: optionalID(q.ContactID), Refs: nonNil(q.References),
		UpdatedAfter: optionalTime(q.UpdatedAfter), Attachment: q.Attachment, Ascending: q.Ascending,
	}
	p.FromDate, p.ToDate = period(q.Period)
	p.MaxRows, p.Skip = page(q.Page, q.PerPage)
	rows, err := s.q.ListGeneralDocuments(ctx, p)
	if err != nil {
		return nil, 0, err
	}
	out := make([]purchases.General, len(rows))
	total := 0
	for i, r := range rows {
		out[i], total = toGeneral(r.GeneralDocument), int(r.Total)
	}
	return out, total, nil
}

// GeneralDocument implements purchases.Store.
func (s *PurchaseStore) GeneralDocument(ctx context.Context, admin, id resource.ID) (purchases.General, error) {
	r, err := s.q.GeneralDocumentByID(ctx, sqlc.GeneralDocumentByIDParams{AdministrationID: int64(admin), ID: int64(id)})
	if errors.Is(err, pgx.ErrNoRows) {
		return purchases.General{}, purchases.ErrNotFound
	}
	return toGeneral(r), err
}

// GeneralDocumentsByID implements purchases.Store.
func (s *PurchaseStore) GeneralDocumentsByID(ctx context.Context, admin resource.ID, ids []resource.ID) ([]purchases.General, error) {
	rows, err := s.q.GeneralDocumentsByIDs(ctx, sqlc.GeneralDocumentsByIDsParams{AdministrationID: int64(admin), Ids: ids64(ids)})
	out := make([]purchases.General, len(rows))
	for i, r := range rows {
		out[i] = toGeneral(r)
	}
	return out, err
}

// SaveGeneralDocument implements purchases.Store.
func (s *PurchaseStore) SaveGeneralDocument(ctx context.Context, g purchases.General, created bool) error {
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		q := s.q.WithTx(tx)
		if err := q.UpsertGeneralDocument(ctx, sqlc.UpsertGeneralDocumentParams{
			ID: int64(g.ID), AdministrationID: int64(g.AdministrationID), ContactID: optionalID(nonZero(g.ContactID)), EntryNumber: int32(g.EntryNumber),
			Reference: g.Reference, Date: day(g.Date), DueDate: optionalDate(g.DueDate), ReminderDate: optionalDate(g.ReminderDate),
			ReminderMessage: g.ReminderMessage, State: g.State, CreatedAt: timestamp(g.CreatedAt), UpdatedAt: timestamp(g.UpdatedAt),
		}); err != nil {
			return err
		}
		if !created {
			return nil
		}
		return q.InsertResource(ctx, sqlc.InsertResourceParams{ID: int64(g.ID), Type: string(resource.TypeGeneralDocument), AdministrationID: int64(g.AdministrationID)})
	})
}

// DeleteGeneralDocument implements purchases.Store.
func (s *PurchaseStore) DeleteGeneralDocument(ctx context.Context, admin, id resource.ID) error {
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		q := s.q.WithTx(tx)
		if err := q.DeleteNotesOf(ctx, sqlc.DeleteNotesOfParams{AdministrationID: int64(admin), EntityID: int64(id)}); err != nil {
			return err
		}
		if err := q.DeleteGeneralDocument(ctx, sqlc.DeleteGeneralDocumentParams{AdministrationID: int64(admin), ID: int64(id)}); err != nil {
			return err
		}
		return q.DeleteResource(ctx, int64(id))
	})
}

// TouchGeneralDocument implements purchases.Store.
func (s *PurchaseStore) TouchGeneralDocument(ctx context.Context, admin, id resource.ID, at time.Time) error {
	return s.q.TouchGeneralDocument(ctx, sqlc.TouchGeneralDocumentParams{AdministrationID: int64(admin), ID: int64(id), UpdatedAt: timestamp(at)})
}

func toGeneral(r sqlc.GeneralDocument) purchases.General {
	g := purchases.General{
		ID: resource.ID(r.ID), AdministrationID: resource.ID(r.AdministrationID), EntryNumber: int(r.EntryNumber), Reference: r.Reference,
		Date: dayOf(r.Date), DueDate: dateOf(r.DueDate), ReminderDate: dateOf(r.ReminderDate), ReminderMessage: r.ReminderMessage, State: r.State,
		CreatedAt: r.CreatedAt.Time, UpdatedAt: r.UpdatedAt.Time,
	}
	if r.ContactID.Valid {
		g.ContactID = resource.ID(r.ContactID.Int64)
	}
	return g
}

// TypelessDocuments implements purchases.Store.
func (s *PurchaseStore) TypelessDocuments(ctx context.Context, admin resource.ID, q purchases.Query) ([]purchases.Typeless, int, error) {
	p := sqlc.ListTypelessDocumentsParams{
		AdministrationID: int64(admin), Nothing: q.None, States: nonNil(q.States), ContactID: optionalID(q.ContactID), Refs: nonNil(q.References),
		UpdatedAfter: optionalTime(q.UpdatedAfter), Attachment: q.Attachment, Ascending: q.Ascending,
	}
	p.FromDate, p.ToDate = period(q.Period)
	p.MaxRows, p.Skip = page(q.Page, q.PerPage)
	rows, err := s.q.ListTypelessDocuments(ctx, p)
	if err != nil {
		return nil, 0, err
	}
	out := make([]purchases.Typeless, len(rows))
	total := 0
	for i, r := range rows {
		out[i], total = toTypeless(r.TypelessDocument), int(r.Total)
	}
	return out, total, nil
}

// TypelessDocument implements purchases.Store.
func (s *PurchaseStore) TypelessDocument(ctx context.Context, admin, id resource.ID) (purchases.Typeless, error) {
	r, err := s.q.TypelessDocumentByID(ctx, sqlc.TypelessDocumentByIDParams{AdministrationID: int64(admin), ID: int64(id)})
	if errors.Is(err, pgx.ErrNoRows) {
		return purchases.Typeless{}, purchases.ErrNotFound
	}
	return toTypeless(r), err
}

// TypelessDocumentsByID implements purchases.Store.
func (s *PurchaseStore) TypelessDocumentsByID(ctx context.Context, admin resource.ID, ids []resource.ID) ([]purchases.Typeless, error) {
	rows, err := s.q.TypelessDocumentsByIDs(ctx, sqlc.TypelessDocumentsByIDsParams{AdministrationID: int64(admin), Ids: ids64(ids)})
	out := make([]purchases.Typeless, len(rows))
	for i, r := range rows {
		out[i] = toTypeless(r)
	}
	return out, err
}

// SaveTypelessDocument implements purchases.Store.
func (s *PurchaseStore) SaveTypelessDocument(ctx context.Context, t purchases.Typeless, created bool) error {
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		q := s.q.WithTx(tx)
		if err := q.UpsertTypelessDocument(ctx, sqlc.UpsertTypelessDocumentParams{
			ID: int64(t.ID), AdministrationID: int64(t.AdministrationID), ContactID: optionalID(nonZero(t.ContactID)), Reference: t.Reference,
			Date: day(t.Date), Origin: t.Origin, State: t.State, CreatedAt: timestamp(t.CreatedAt), UpdatedAt: timestamp(t.UpdatedAt),
		}); err != nil {
			return err
		}
		if !created {
			return nil
		}
		return q.InsertResource(ctx, sqlc.InsertResourceParams{ID: int64(t.ID), Type: string(resource.TypeTypelessDocument), AdministrationID: int64(t.AdministrationID)})
	})
}

// DeleteTypelessDocument implements purchases.Store.
func (s *PurchaseStore) DeleteTypelessDocument(ctx context.Context, admin, id resource.ID) error {
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		q := s.q.WithTx(tx)
		if err := q.DeleteTypelessDocument(ctx, sqlc.DeleteTypelessDocumentParams{AdministrationID: int64(admin), ID: int64(id)}); err != nil {
			return err
		}
		return q.DeleteResource(ctx, int64(id))
	})
}

// TouchTypelessDocument implements purchases.Store.
func (s *PurchaseStore) TouchTypelessDocument(ctx context.Context, admin, id resource.ID, at time.Time) error {
	return s.q.TouchTypelessDocument(ctx, sqlc.TouchTypelessDocumentParams{AdministrationID: int64(admin), ID: int64(id), UpdatedAt: timestamp(at)})
}

func toTypeless(r sqlc.TypelessDocument) purchases.Typeless {
	t := purchases.Typeless{
		ID: resource.ID(r.ID), AdministrationID: resource.ID(r.AdministrationID), Reference: r.Reference, Date: dayOf(r.Date), Origin: r.Origin,
		State: r.State, CreatedAt: r.CreatedAt.Time, UpdatedAt: r.UpdatedAt.Time,
	}
	if r.ContactID.Valid {
		t.ContactID = resource.ID(r.ContactID.Int64)
	}
	return t
}

// Transactions implements purchases.Store.
func (s *PurchaseStore) Transactions(ctx context.Context, admin resource.ID, q purchases.TransactionQuery) ([]purchases.Transaction, int, error) {
	p := sqlc.ListPurchaseTransactionsParams{
		AdministrationID: int64(admin), States: nonNil(q.States), Unbatched: q.Unbatched, BatchID: optionalID(q.BatchID), PayableID: optionalID(q.PayableID),
	}
	p.FromDate, p.ToDate = period(q.Period)
	p.MaxRows, p.Skip = page(q.Page, q.PerPage)
	rows, err := s.q.ListPurchaseTransactions(ctx, p)
	if err != nil {
		return nil, 0, err
	}
	out := make([]purchases.Transaction, len(rows))
	total := 0
	for i, r := range rows {
		out[i], total = toTransaction(r.PurchaseTransaction, r.PayablePaid), int(r.Total)
	}
	return out, total, nil
}

// Transaction implements purchases.Store.
func (s *PurchaseStore) Transaction(ctx context.Context, admin, id resource.ID) (purchases.Transaction, error) {
	r, err := s.q.PurchaseTransactionByID(ctx, sqlc.PurchaseTransactionByIDParams{AdministrationID: int64(admin), ID: int64(id)})
	if errors.Is(err, pgx.ErrNoRows) {
		return purchases.Transaction{}, purchases.ErrTransactionNotFound
	}
	if err != nil {
		return purchases.Transaction{}, err
	}
	return toTransaction(r.PurchaseTransaction, r.PayablePaid), nil
}

// SaveTransactions implements purchases.Store.
func (s *PurchaseStore) SaveTransactions(ctx context.Context, batch *purchases.Batch, list []purchases.Transaction, created bool) error {
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		q := s.q.WithTx(tx)
		if batch != nil {
			if err := saveBatch(ctx, q, *batch); err != nil {
				return err
			}
			if created {
				if err := q.InsertResource(ctx, sqlc.InsertResourceParams{ID: int64(batch.ID), Type: string(resource.TypePurchaseTransactionBatch), AdministrationID: int64(batch.AdministrationID)}); err != nil {
					return err
				}
			}
		}
		for _, t := range list {
			if err := q.UpsertPurchaseTransaction(ctx, sqlc.UpsertPurchaseTransactionParams{
				ID: int64(t.ID), AdministrationID: int64(t.AdministrationID), FinancialAccountID: int64(t.FinancialAccountID),
				PurchaseTransactionBatchID: optionalID(t.BatchID), PayableType: optionalText(t.PayableType), PayableID: optionalID(t.PayableID),
				ContactID: optionalID(t.ContactID), Amount: int64(t.Amount), Date: optionalDate(t.Date), Description: t.Description,
				EndToEndID: t.EndToEndID, SepaIban: t.IBAN, SepaIbanAccountName: t.IBANName, SepaBic: t.BIC, SourceSepaIban: optionalText(t.SourceIBAN),
				SourceSepaIbanAccountName: optionalText(t.SourceName), Cancelled: t.Cancelled, CreatedAt: timestamp(t.CreatedAt), UpdatedAt: timestamp(t.UpdatedAt),
			}); err != nil {
				return err
			}
			if created {
				if err := q.InsertResource(ctx, sqlc.InsertResourceParams{ID: int64(t.ID), Type: string(resource.TypePurchaseTransaction), AdministrationID: int64(t.AdministrationID)}); err != nil {
					return err
				}
			}
		}
		return nil
	})
}

// DeleteTransaction implements purchases.Store.
func (s *PurchaseStore) DeleteTransaction(ctx context.Context, admin, id resource.ID) error {
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		q := s.q.WithTx(tx)
		if err := q.DeletePurchaseTransaction(ctx, sqlc.DeletePurchaseTransactionParams{AdministrationID: int64(admin), ID: int64(id)}); err != nil {
			return err
		}
		return q.DeleteResource(ctx, int64(id))
	})
}

// Batch implements purchases.Store.
func (s *PurchaseStore) Batch(ctx context.Context, admin, id resource.ID) (purchases.Batch, error) {
	r, err := s.q.PurchaseTransactionBatchByID(ctx, sqlc.PurchaseTransactionBatchByIDParams{AdministrationID: int64(admin), ID: int64(id)})
	if errors.Is(err, pgx.ErrNoRows) {
		return purchases.Batch{}, purchases.ErrTransactionNotFound
	}
	return purchases.Batch{
		ID: resource.ID(r.ID), AdministrationID: resource.ID(r.AdministrationID), FinancialAccountID: resource.ID(r.FinancialAccountID),
		Identifier: r.Identifier, State: r.State, ExecutionDate: dayOf(r.ExecutionDate), CreatedAt: r.CreatedAt.Time, UpdatedAt: r.UpdatedAt.Time,
	}, err
}

// SaveBatch implements purchases.Store.
func (s *PurchaseStore) SaveBatch(ctx context.Context, b purchases.Batch) error {
	return saveBatch(ctx, s.q, b)
}

func saveBatch(ctx context.Context, q *sqlc.Queries, b purchases.Batch) error {
	return q.UpsertPurchaseTransactionBatch(ctx, sqlc.UpsertPurchaseTransactionBatchParams{
		ID: int64(b.ID), AdministrationID: int64(b.AdministrationID), FinancialAccountID: int64(b.FinancialAccountID), Identifier: b.Identifier,
		State: b.State, ExecutionDate: day(b.ExecutionDate), CreatedAt: timestamp(b.CreatedAt), UpdatedAt: timestamp(b.UpdatedAt),
	})
}

func toTransaction(r sqlc.PurchaseTransaction, paid bool) purchases.Transaction {
	return purchases.Transaction{
		ID: resource.ID(r.ID), AdministrationID: resource.ID(r.AdministrationID), FinancialAccountID: resource.ID(r.FinancialAccountID),
		BatchID: idOf(r.PurchaseTransactionBatchID), PayableType: r.PayableType.String, PayableID: idOf(r.PayableID), ContactID: idOf(r.ContactID),
		Amount: money.Amount(r.Amount), Date: dateOf(r.Date), Description: r.Description, EndToEndID: r.EndToEndID, IBAN: r.SepaIban,
		IBANName: r.SepaIbanAccountName, BIC: r.SepaBic, SourceIBAN: r.SourceSepaIban.String, SourceName: r.SourceSepaIbanAccountName.String,
		Cancelled: r.Cancelled, PayablePaid: paid, CreatedAt: r.CreatedAt.Time, UpdatedAt: r.UpdatedAt.Time,
	}
}
