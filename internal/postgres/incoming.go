package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
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

// RetypeDocument implements purchases.Store: in one transaction the
// document leaves its table and enters the new type's under the same id,
// and its resource, notes, attachments and events take the new type.
func (s *PurchaseStore) RetypeDocument(ctx context.Context, r purchases.Retype) error {
	admin, id := int64(r.AdministrationID), int64(r.ID)
	to := r.To()
	var typ resource.Type
	switch {
	case r.Document != nil:
		typ = r.Document.Ref().Type
	case r.General != nil:
		typ = resource.TypeGeneralDocument
	case r.Journal != nil:
		typ = resource.TypeGeneralJournalDocument
	default:
		return fmt.Errorf("retype %d: no new document", r.ID)
	}
	purchase := func(kind string) bool { return kind == purchases.PurchaseInvoice || kind == purchases.Receipt }
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		q := s.q.WithTx(tx)
		if purchase(r.From) && r.Document != nil {
			// Between purchase invoice and receipt only the kind changes;
			// the lines stay.
			if err := q.SetPurchaseDocumentKind(ctx, sqlc.SetPurchaseDocumentKindParams{
				Kind: r.Document.Kind, RevenueInvoice: r.Document.RevenueInvoice, UpdatedAt: timestamp(r.Document.UpdatedAt), AdministrationID: admin, ID: id,
			}); err != nil {
				return err
			}
		} else {
			if err := deleteIncoming(ctx, q, r.From, admin, id); err != nil {
				return err
			}
			if err := insertIncoming(ctx, q, r); err != nil {
				return err
			}
		}
		if err := q.RetypeResource(ctx, sqlc.RetypeResourceParams{Type: string(typ), AdministrationID: admin, ID: id}); err != nil {
			return err
		}
		if err := q.RetypeNotes(ctx, sqlc.RetypeNotesParams{NewType: to, AdministrationID: admin, EntityID: id, OldType: r.From}); err != nil {
			return err
		}
		if err := q.RetypeAttachments(ctx, sqlc.RetypeAttachmentsParams{NewType: to, AdministrationID: admin, EntityID: id, OldType: r.From}); err != nil {
			return err
		}
		return q.RetypeEvents(ctx, sqlc.RetypeEventsParams{NewType: to, AdministrationID: admin, EntityID: id, OldType: r.From})
	})
}

// deleteIncoming removes the row of a document that books nothing; the
// lines of a purchase invoice or receipt go with it.
func deleteIncoming(ctx context.Context, q *sqlc.Queries, kind string, admin, id int64) error {
	switch kind {
	case purchases.PurchaseInvoice, purchases.Receipt:
		return q.DeletePurchaseDocument(ctx, sqlc.DeletePurchaseDocumentParams{AdministrationID: admin, ID: id})
	case purchases.GeneralDocument:
		return q.DeleteGeneralDocument(ctx, sqlc.DeleteGeneralDocumentParams{AdministrationID: admin, ID: id})
	case purchases.TypelessDocument:
		return q.DeleteTypelessDocument(ctx, sqlc.DeleteTypelessDocumentParams{AdministrationID: admin, ID: id})
	}
	return fmt.Errorf("cannot retype a %s", kind)
}

// insertIncoming stores the document as its new type.
func insertIncoming(ctx context.Context, q *sqlc.Queries, r purchases.Retype) error {
	switch {
	case r.Document != nil:
		d := r.Document
		taxTotals, err := encodeTaxTotals(d.Totals)
		if err != nil {
			return err
		}
		return q.UpsertPurchaseDocument(ctx, sqlc.UpsertPurchaseDocumentParams{
			ID: int64(d.ID), AdministrationID: int64(d.AdministrationID), Kind: d.Kind, ContactID: optionalID(nonZero(d.ContactID)),
			EntryNumber: int32(d.EntryNumber), Reference: d.Reference, Date: day(d.Date), DueDate: optionalDate(d.DueDate), State: d.State,
			Origin: d.Origin, Currency: d.Currency, PricesAreInclTax: d.PricesAreInclTax, RevenueInvoice: d.RevenueInvoice, PaidAt: optionalTime(d.PaidAt),
			TotalExclTax: int64(d.Totals.ExclTax), TotalTax: int64(d.Totals.Tax), TotalInclTax: int64(d.Totals.InclTax), TaxTotals: taxTotals,
			CreatedAt: timestamp(d.CreatedAt), UpdatedAt: timestamp(d.UpdatedAt),
		})
	case r.General != nil:
		g := r.General
		return q.UpsertGeneralDocument(ctx, sqlc.UpsertGeneralDocumentParams{
			ID: int64(g.ID), AdministrationID: int64(g.AdministrationID), ContactID: optionalID(nonZero(g.ContactID)), EntryNumber: int32(g.EntryNumber),
			Reference: g.Reference, Date: day(g.Date), DueDate: optionalDate(g.DueDate), ReminderDate: optionalDate(g.ReminderDate),
			ReminderMessage: g.ReminderMessage, State: g.State, CreatedAt: timestamp(g.CreatedAt), UpdatedAt: timestamp(g.UpdatedAt),
		})
	}
	j := r.Journal
	return q.InsertGeneralJournalDocument(ctx, sqlc.InsertGeneralJournalDocumentParams{
		ID: int64(j.ID), AdministrationID: int64(j.AdministrationID), Reference: j.Reference, Date: day(j.Date), JournalType: j.JournalType,
		CreatedAt: timestamp(j.CreatedAt), UpdatedAt: timestamp(j.UpdatedAt),
	})
}
