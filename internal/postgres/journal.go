package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/adirkuhn/moneyharpy/internal/attachments"
	"github.com/adirkuhn/moneyharpy/internal/journal"
	"github.com/adirkuhn/moneyharpy/internal/money"
	"github.com/adirkuhn/moneyharpy/internal/postgres/sqlc"
	"github.com/adirkuhn/moneyharpy/internal/resource"
)

// JournalStore implements journal.Store and journal.DocumentStore.
type JournalStore struct {
	pool *pgxpool.Pool
	q    *sqlc.Queries
}

// NewJournalStore returns a JournalStore on pool.
func NewJournalStore(pool *pgxpool.Pool) *JournalStore {
	return &JournalStore{pool: pool, q: sqlc.New(pool)}
}

func day(t time.Time) pgtype.Date { return pgtype.Date{Time: t, Valid: true} }

func dayOf(d pgtype.Date) time.Time {
	return time.Date(d.Time.Year(), d.Time.Month(), d.Time.Day(), 0, 0, 0, 0, time.UTC)
}

// postEntries replaces what a document posts, inside tx.
func postEntries(ctx context.Context, q *sqlc.Queries, admin resource.ID, documentType string, document resource.ID, entries []journal.Entry, at time.Time) error {
	if err := journal.Balance(entries); err != nil {
		return err
	}
	if err := q.DeleteJournalEntriesOf(ctx, sqlc.DeleteJournalEntriesOfParams{AdministrationID: int64(admin), DocumentType: documentType, DocumentID: int64(document)}); err != nil {
		return err
	}
	for _, e := range entries {
		if err := q.InsertJournalEntry(ctx, sqlc.InsertJournalEntryParams{
			ID: int64(e.ID), AdministrationID: int64(admin), DocumentType: documentType, DocumentID: int64(document), Date: day(e.Date),
			LedgerAccountID: int64(e.LedgerAccountID), ContactID: optionalID(e.ContactID), ProjectID: optionalID(e.ProjectID),
			TaxRateID: optionalID(e.TaxRateID), FinancialAccountID: optionalID(e.FinancialAccountID), Amount: int64(e.Amount),
			Description: e.Description, RowOrder: int32(e.RowOrder), CreatedAt: timestamp(at), UpdatedAt: timestamp(at),
		}); err != nil {
			return err
		}
	}
	return nil
}

// Entries implements journal.Store.
func (s *JournalStore) Entries(ctx context.Context, admin resource.ID, q journal.EntryQuery) ([]journal.Entry, int, error) {
	rows, err := s.q.ListLedgerEntries(ctx, sqlc.ListLedgerEntriesParams{
		AdministrationID: int64(admin), FromDate: day(q.Period.From), ToDate: day(q.Period.To), LedgerAccountID: optionalID(q.LedgerAccountID),
		ContactID: optionalID(q.ContactID), NoContact: q.NoContact, ProjectID: optionalID(q.ProjectID), NoProject: q.NoProject,
		AccountType: q.AccountType, DocumentType: q.DocumentType, DocumentID: optionalID(q.DocumentID),
		MaxRows: int32(q.PerPage), Skip: int32((max(q.Page, 1) - 1) * q.PerPage),
	})
	if err != nil {
		return nil, 0, err
	}
	out := make([]journal.Entry, len(rows))
	total := 0
	for i, r := range rows {
		total = int(r.Total)
		out[i] = journal.Entry{
			ID: resource.ID(r.ID), AdministrationID: resource.ID(r.AdministrationID), DocumentType: r.DocumentType, DocumentID: resource.ID(r.DocumentID),
			Date: dayOf(r.Date), LedgerAccountID: resource.ID(r.LedgerAccountID), ContactID: idOf(r.ContactID), ProjectID: idOf(r.ProjectID),
			TaxRateID: idOf(r.TaxRateID), FinancialAccountID: idOf(r.FinancialAccountID), Amount: money.Amount(r.Amount), Description: r.Description,
			RowOrder: int(r.RowOrder),
		}
	}
	return out, total, nil
}

// Sums implements journal.Store.
func (s *JournalStore) Sums(ctx context.Context, admin resource.ID, p money.Period) ([]journal.AccountSum, error) {
	rows, err := s.q.LedgerSums(ctx, sqlc.LedgerSumsParams{
		AdministrationID: int64(admin), FromDate: day(p.From), ToDate: day(p.To), YearStart: day(time.Date(p.From.Year(), 1, 1, 0, 0, 0, 0, time.UTC)),
	})
	if err != nil {
		return nil, err
	}
	out := make([]journal.AccountSum, len(rows))
	for i, r := range rows {
		start := money.Amount(r.StartBalance)
		out[i] = journal.AccountSum{
			LedgerAccountID: resource.ID(r.LedgerAccountID), Debit: money.Amount(r.Debit), Credit: money.Amount(r.Credit),
			Start: start, Final: start + money.Amount(r.Debit) - money.Amount(r.Credit),
		}
	}
	return out, nil
}

// ContactInUse implements contacts.References: journal entries keep the
// contacts they name.
func (s *JournalStore) ContactInUse(ctx context.Context, admin, contact resource.ID) (bool, error) {
	return s.q.ContactInJournal(ctx, sqlc.ContactInJournalParams{AdministrationID: int64(admin), ContactID: pgtype.Int8{Int64: int64(contact), Valid: true}})
}

// Documents implements journal.DocumentStore.
func (s *JournalStore) Documents(ctx context.Context, admin resource.ID, q journal.DocumentQuery) ([]journal.Document, int, error) {
	p := sqlc.ListGeneralJournalDocumentsParams{
		AdministrationID: int64(admin), Reference: q.Reference, UpdatedAfter: optionalTime(q.UpdatedAfter), ContactID: optionalID(q.ContactID),
		LedgerAccountID: optionalID(q.LedgerAccountID), Attachment: q.Attachment, Ascending: q.Ascending,
	}
	if q.Period != nil {
		p.FromDate, p.ToDate = day(q.Period.From), day(q.Period.To)
	}
	if q.PerPage > 0 {
		p.MaxRows = pgtype.Int4{Int32: int32(q.PerPage), Valid: true}
		p.Skip = int32((max(q.Page, 1) - 1) * q.PerPage)
	}
	rows, err := s.q.ListGeneralJournalDocuments(ctx, p)
	if err != nil {
		return nil, 0, err
	}
	docs := make([]sqlc.GeneralJournalDocument, len(rows))
	total := 0
	for i, r := range rows {
		docs[i], total = r.GeneralJournalDocument, int(r.Total)
	}
	out, err := s.complete(ctx, admin, docs)
	return out, total, err
}

// Document implements journal.DocumentStore.
func (s *JournalStore) Document(ctx context.Context, admin, id resource.ID) (journal.Document, error) {
	d, err := s.q.GeneralJournalDocumentByID(ctx, sqlc.GeneralJournalDocumentByIDParams{AdministrationID: int64(admin), ID: int64(id)})
	if errors.Is(err, pgx.ErrNoRows) {
		return journal.Document{}, journal.ErrDocumentNotFound
	}
	if err != nil {
		return journal.Document{}, err
	}
	out, err := s.complete(ctx, admin, []sqlc.GeneralJournalDocument{d})
	if err != nil {
		return journal.Document{}, err
	}
	return out[0], nil
}

// DocumentsByID implements journal.DocumentStore.
func (s *JournalStore) DocumentsByID(ctx context.Context, admin resource.ID, ids []resource.ID) ([]journal.Document, error) {
	rows, err := s.q.GeneralJournalDocumentsByIDs(ctx, sqlc.GeneralJournalDocumentsByIDsParams{AdministrationID: int64(admin), Ids: ids64(ids)})
	if err != nil {
		return nil, err
	}
	return s.complete(ctx, admin, rows)
}

// CreateDocument implements journal.DocumentStore.
func (s *JournalStore) CreateDocument(ctx context.Context, d journal.Document) error {
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		q := s.q.WithTx(tx)
		if err := q.InsertGeneralJournalDocument(ctx, sqlc.InsertGeneralJournalDocumentParams{
			ID: int64(d.ID), AdministrationID: int64(d.AdministrationID), Reference: d.Reference, Date: day(d.Date), JournalType: d.JournalType,
			CreatedAt: timestamp(d.CreatedAt), UpdatedAt: timestamp(d.UpdatedAt),
		}); err != nil {
			return err
		}
		if err := q.InsertResource(ctx, sqlc.InsertResourceParams{ID: int64(d.ID), Type: string(resource.TypeGeneralJournalDocument), AdministrationID: int64(d.AdministrationID)}); err != nil {
			return err
		}
		return postEntries(ctx, q, d.AdministrationID, journal.GeneralJournalDocument, d.ID, d.Entries(), d.UpdatedAt)
	})
}

// UpdateDocument implements journal.DocumentStore.
func (s *JournalStore) UpdateDocument(ctx context.Context, d journal.Document) error {
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		q := s.q.WithTx(tx)
		if err := q.UpdateGeneralJournalDocument(ctx, sqlc.UpdateGeneralJournalDocumentParams{
			AdministrationID: int64(d.AdministrationID), ID: int64(d.ID), Reference: d.Reference, Date: day(d.Date), JournalType: d.JournalType,
			UpdatedAt: timestamp(d.UpdatedAt),
		}); err != nil {
			return err
		}
		return postEntries(ctx, q, d.AdministrationID, journal.GeneralJournalDocument, d.ID, d.Entries(), d.UpdatedAt)
	})
}

// DeleteDocument implements journal.DocumentStore.
func (s *JournalStore) DeleteDocument(ctx context.Context, admin, id resource.ID) error {
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		q := s.q.WithTx(tx)
		if err := q.DeleteJournalEntriesOf(ctx, sqlc.DeleteJournalEntriesOfParams{AdministrationID: int64(admin), DocumentType: journal.GeneralJournalDocument, DocumentID: int64(id)}); err != nil {
			return err
		}
		if err := q.DeleteNotesOf(ctx, sqlc.DeleteNotesOfParams{AdministrationID: int64(admin), EntityID: int64(id)}); err != nil {
			return err
		}
		if err := q.DeleteGeneralJournalDocument(ctx, sqlc.DeleteGeneralJournalDocumentParams{AdministrationID: int64(admin), ID: int64(id)}); err != nil {
			return err
		}
		return q.DeleteResource(ctx, int64(id))
	})
}

// TouchDocument implements journal.DocumentStore.
func (s *JournalStore) TouchDocument(ctx context.Context, admin, id resource.ID, at time.Time) error {
	return s.q.TouchGeneralJournalDocument(ctx, sqlc.TouchGeneralJournalDocumentParams{AdministrationID: int64(admin), ID: int64(id), UpdatedAt: timestamp(at)})
}

// complete converts rows and loads the entries they post as their rows.
func (s *JournalStore) complete(ctx context.Context, admin resource.ID, rows []sqlc.GeneralJournalDocument) ([]journal.Document, error) {
	out := make([]journal.Document, len(rows))
	if len(rows) == 0 {
		return out, nil
	}
	ids := make([]int64, len(rows))
	index := map[int64]int{}
	for i, r := range rows {
		ids[i], index[r.ID] = r.ID, i
		out[i] = journal.Document{
			ID: resource.ID(r.ID), AdministrationID: resource.ID(r.AdministrationID), Reference: r.Reference, Date: dayOf(r.Date),
			JournalType: r.JournalType, CreatedAt: r.CreatedAt.Time, UpdatedAt: r.UpdatedAt.Time,
		}
	}
	entries, err := s.q.JournalEntriesOf(ctx, sqlc.JournalEntriesOfParams{AdministrationID: int64(admin), DocumentType: journal.GeneralJournalDocument, Ids: ids})
	if err != nil {
		return nil, err
	}
	for _, e := range entries {
		d := &out[index[e.DocumentID]]
		row := journal.Row{
			ID: resource.ID(e.ID), LedgerAccountID: resource.ID(e.LedgerAccountID), ContactID: idOf(e.ContactID), ProjectID: idOf(e.ProjectID),
			Description: e.Description, RowOrder: int(e.RowOrder), CreatedAt: e.CreatedAt.Time, UpdatedAt: e.UpdatedAt.Time,
		}
		if e.Amount > 0 {
			row.Debit = money.Amount(e.Amount)
		} else {
			row.Credit = money.Amount(-e.Amount)
		}
		d.Rows = append(d.Rows, row)
	}
	return out, nil
}

// AttachmentStore implements attachments.Store.
type AttachmentStore struct{ q *sqlc.Queries }

// NewAttachmentStore returns an AttachmentStore on pool.
func NewAttachmentStore(pool *pgxpool.Pool) *AttachmentStore {
	return &AttachmentStore{q: sqlc.New(pool)}
}

// CreateAttachment implements attachments.Store.
func (s *AttachmentStore) CreateAttachment(ctx context.Context, a attachments.Attachment) error {
	return s.q.InsertAttachment(ctx, sqlc.InsertAttachmentParams{
		ID: int64(a.ID), AdministrationID: int64(a.AdministrationID), EntityType: a.EntityType, EntityID: int64(a.EntityID), Filename: a.Filename,
		ContentType: a.ContentType, Size: a.Size, CreatedAt: timestamp(a.CreatedAt), UpdatedAt: timestamp(a.UpdatedAt),
	})
}

// DeleteAttachment implements attachments.Store.
func (s *AttachmentStore) DeleteAttachment(ctx context.Context, admin, id resource.ID) error {
	return s.q.DeleteAttachment(ctx, sqlc.DeleteAttachmentParams{AdministrationID: int64(admin), ID: int64(id)})
}

// AttachmentsOf implements attachments.Store.
func (s *AttachmentStore) AttachmentsOf(ctx context.Context, admin resource.ID, entityType string, entities []resource.ID) ([]attachments.Attachment, error) {
	rows, err := s.q.AttachmentsOf(ctx, sqlc.AttachmentsOfParams{AdministrationID: int64(admin), EntityType: entityType, Ids: ids64(entities)})
	if err != nil {
		return nil, err
	}
	out := make([]attachments.Attachment, len(rows))
	for i, r := range rows {
		out[i] = attachments.Attachment{
			ID: resource.ID(r.ID), AdministrationID: resource.ID(r.AdministrationID), EntityType: r.EntityType, EntityID: resource.ID(r.EntityID),
			Filename: r.Filename, ContentType: r.ContentType, Size: r.Size, CreatedAt: r.CreatedAt.Time, UpdatedAt: r.UpdatedAt.Time,
		}
	}
	return out, nil
}
