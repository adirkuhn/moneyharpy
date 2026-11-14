package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/adirkuhn/moneyharpy/internal/money"
	"github.com/adirkuhn/moneyharpy/internal/postgres/sqlc"
	"github.com/adirkuhn/moneyharpy/internal/purchases"
	"github.com/adirkuhn/moneyharpy/internal/recognize"
	"github.com/adirkuhn/moneyharpy/internal/resource"
	"github.com/adirkuhn/moneyharpy/internal/sales"
)

// RecognitionStore implements recognize.Store.
type RecognitionStore struct {
	pool *pgxpool.Pool
	q    *sqlc.Queries
}

// NewRecognitionStore returns a RecognitionStore on pool.
func NewRecognitionStore(pool *pgxpool.Pool) *RecognitionStore {
	return &RecognitionStore{pool: pool, q: sqlc.New(pool)}
}

type recognizedFieldJSON struct {
	Field            string `json:"field"`
	Confidence       string `json:"confidence"`
	Reason           string `json:"reason,omitempty"`
	SourceDocumentID int64  `json:"source_document_id,omitempty"`
	SourceReference  string `json:"source_reference,omitempty"`
}

func toRecognition(r sqlc.DocumentRecognition) (recognize.Recognition, error) {
	var fields []recognizedFieldJSON
	if err := json.Unmarshal(r.Fields, &fields); err != nil {
		return recognize.Recognition{}, fmt.Errorf("decode recognized fields: %w", err)
	}
	out := recognize.Recognition{
		DocumentID: resource.ID(r.DocumentID), AdministrationID: resource.ID(r.AdministrationID), UserID: resource.ID(r.UserID),
		State: recognize.State(r.State), Source: recognize.Source(r.Source), Error: r.Error, Attempts: int(r.Attempts),
		CreatedAt: r.CreatedAt.Time, UpdatedAt: r.UpdatedAt.Time,
	}
	if r.NextAttemptAt.Valid {
		t := r.NextAttemptAt.Time
		out.NextAttemptAt = &t
	}
	for _, f := range fields {
		out.Fields = append(out.Fields, recognize.Field{
			Name: f.Field, Confidence: recognize.Confidence(f.Confidence), Reason: recognize.Reason(f.Reason),
			SourceDocumentID: resource.ID(f.SourceDocumentID), SourceReference: f.SourceReference,
		})
	}
	return out, nil
}

// Recognition implements recognize.Store.
func (s *RecognitionStore) Recognition(ctx context.Context, admin, id resource.ID) (recognize.Recognition, error) {
	r, err := s.q.DocumentRecognition(ctx, sqlc.DocumentRecognitionParams{AdministrationID: int64(admin), DocumentID: int64(id)})
	if errors.Is(err, pgx.ErrNoRows) {
		return recognize.Recognition{}, recognize.ErrNotFound
	}
	if err != nil {
		return recognize.Recognition{}, err
	}
	return toRecognition(r)
}

// Recognitions implements recognize.Store.
func (s *RecognitionStore) Recognitions(ctx context.Context, admin resource.ID, ids []resource.ID) (map[resource.ID]recognize.Recognition, error) {
	out := map[resource.ID]recognize.Recognition{}
	if len(ids) == 0 {
		return out, nil
	}
	raw := make([]int64, len(ids))
	for i, id := range ids {
		raw[i] = int64(id)
	}
	rows, err := s.q.DocumentRecognitionsOf(ctx, sqlc.DocumentRecognitionsOfParams{AdministrationID: int64(admin), Ids: raw})
	if err != nil {
		return nil, err
	}
	for _, row := range rows {
		r, err := toRecognition(row)
		if err != nil {
			return nil, err
		}
		out[r.DocumentID] = r
	}
	return out, nil
}

// SaveRecognition implements recognize.Store.
func (s *RecognitionStore) SaveRecognition(ctx context.Context, r recognize.Recognition) error {
	fields := make([]recognizedFieldJSON, len(r.Fields))
	for i, f := range r.Fields {
		fields[i] = recognizedFieldJSON{
			Field: f.Name, Confidence: string(f.Confidence), Reason: string(f.Reason), SourceDocumentID: int64(f.SourceDocumentID),
			SourceReference: f.SourceReference,
		}
	}
	raw, err := json.Marshal(fields)
	if err != nil {
		return err
	}
	return s.q.UpsertDocumentRecognition(ctx, sqlc.UpsertDocumentRecognitionParams{
		DocumentID: int64(r.DocumentID), AdministrationID: int64(r.AdministrationID), UserID: int64(r.UserID), State: string(r.State),
		Source: string(r.Source), Fields: raw, Error: r.Error, Attempts: int32(r.Attempts), NextAttemptAt: optionalTime(r.NextAttemptAt),
		CreatedAt: timestamp(r.CreatedAt), UpdatedAt: timestamp(r.UpdatedAt),
	})
}

// DeleteRecognition implements recognize.Store.
func (s *RecognitionStore) DeleteRecognition(ctx context.Context, admin, id resource.ID) error {
	return s.q.DeleteDocumentRecognition(ctx, sqlc.DeleteDocumentRecognitionParams{AdministrationID: int64(admin), DocumentID: int64(id)})
}

// LeaseDue implements recognize.Store.
func (s *RecognitionStore) LeaseDue(ctx context.Context, now, lease time.Time, maxCount int) ([]recognize.Recognition, error) {
	rows, err := s.q.LeaseDueRecognitions(ctx, sqlc.LeaseDueRecognitionsParams{Lease: timestamp(lease), Now: timestamp(now), MaxCount: int32(maxCount)})
	if err != nil {
		return nil, err
	}
	out := make([]recognize.Recognition, 0, len(rows))
	for _, row := range rows {
		r, err := toRecognition(row)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, nil
}

// Memory implements recognize.Store.
func (s *RecognitionStore) Memory(ctx context.Context, admin, contact resource.ID) (recognize.Memory, error) {
	m := recognize.Memory{AdministrationID: admin, ContactID: contact}
	for _, k := range recognize.Kinds {
		m.Of(k).Lines = map[string]recognize.Category{}
	}
	row, err := s.q.SupplierMemory(ctx, sqlc.SupplierMemoryParams{AdministrationID: int64(admin), ContactID: int64(contact)})
	if errors.Is(err, pgx.ErrNoRows) {
		return m, nil
	}
	if err != nil {
		return recognize.Memory{}, err
	}
	m.Kind = recognize.Learned[recognize.Kind]{V: recognize.Kind(row.DocumentKind), Uses: int(row.DocumentKindUses)}
	m.UpdatedAt = row.UpdatedAt.Time
	kinds, err := s.q.SupplierMemoryKinds(ctx, sqlc.SupplierMemoryKindsParams{AdministrationID: int64(admin), ContactID: int64(contact)})
	if err != nil {
		return recognize.Memory{}, err
	}
	for _, k := range kinds {
		km := m.Of(recognize.Kind(k.DocumentKind))
		km.Currency = recognize.Learned[string]{V: k.Currency, Uses: int(k.CurrencyUses)}
		km.PricesInclTax = recognize.Learned[bool]{V: k.PricesAreInclTax, Uses: int(k.PricesAreInclTaxUses)}
		km.LastDocumentID, km.UpdatedAt = resource.ID(k.LastDocumentID.Int64), k.UpdatedAt.Time
	}
	lines, err := s.q.SupplierMemoryLines(ctx, sqlc.SupplierMemoryLinesParams{AdministrationID: int64(admin), ContactID: int64(contact)})
	if err != nil {
		return recognize.Memory{}, err
	}
	for _, l := range lines {
		m.Of(recognize.Kind(l.DocumentKind)).Lines[l.Description] = recognize.Category{
			LedgerAccountID: resource.ID(l.LedgerAccountID.Int64), TaxRateID: resource.ID(l.TaxRateID.Int64), Uses: int(l.Uses), LastUsed: l.LastUsedAt.Time,
			SourceDocumentID: resource.ID(l.SourceDocumentID.Int64),
		}
	}
	return m, nil
}

// SaveMemory implements recognize.Store. Kinds nothing was learned of are
// left out.
func (s *RecognitionStore) SaveMemory(ctx context.Context, m recognize.Memory) error {
	admin, contact := int64(m.AdministrationID), int64(m.ContactID)
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		q := s.q.WithTx(tx)
		if err := q.UpsertSupplierMemory(ctx, sqlc.UpsertSupplierMemoryParams{
			AdministrationID: admin, ContactID: contact, DocumentKind: string(m.Kind.V), DocumentKindUses: int32(m.Kind.Uses),
			UpdatedAt: timestamp(m.UpdatedAt),
		}); err != nil {
			return err
		}
		if err := q.DeleteSupplierMemoryKinds(ctx, sqlc.DeleteSupplierMemoryKindsParams{AdministrationID: admin, ContactID: contact}); err != nil {
			return err
		}
		for _, kind := range recognize.Kinds {
			k := m.Of(kind)
			if k.Empty() {
				continue
			}
			updated := k.UpdatedAt
			if updated.IsZero() {
				updated = m.UpdatedAt
			}
			if err := q.InsertSupplierMemoryKind(ctx, sqlc.InsertSupplierMemoryKindParams{
				AdministrationID: admin, ContactID: contact, DocumentKind: string(kind), Currency: k.Currency.V, CurrencyUses: int32(k.Currency.Uses),
				PricesAreInclTax: k.PricesInclTax.V, PricesAreInclTaxUses: int32(k.PricesInclTax.Uses), LastDocumentID: optionalInt8(k.LastDocumentID),
				UpdatedAt: timestamp(updated),
			}); err != nil {
				return err
			}
			for description, c := range k.Lines {
				if err := q.InsertSupplierMemoryLine(ctx, sqlc.InsertSupplierMemoryLineParams{
					AdministrationID: admin, ContactID: contact, DocumentKind: string(kind), Description: description,
					LedgerAccountID: optionalInt8(c.LedgerAccountID), TaxRateID: optionalInt8(c.TaxRateID), Uses: int32(c.Uses),
					LastUsedAt: timestamp(c.LastUsed), SourceDocumentID: optionalInt8(c.SourceDocumentID),
				}); err != nil {
					return err
				}
			}
		}
		return nil
	})
}

// History implements recognize.Store.
func (s *RecognitionStore) History(ctx context.Context, admin, contact resource.ID, kind string, limit int) ([]purchases.Document, error) {
	rows, err := s.q.SupplierHistory(ctx, sqlc.SupplierHistoryParams{
		AdministrationID: int64(admin), ContactID: optionalInt8(contact), Kind: kind, MaxCount: int32(limit),
	})
	if err != nil || len(rows) == 0 {
		return nil, err
	}
	docs := make([]purchases.Document, len(rows))
	at := make(map[int64]int, len(rows))
	ids := make([]int64, len(rows))
	for i, r := range rows {
		// The query gives the most recent first; the history is oldest
		// first.
		j := len(rows) - 1 - i
		docs[j] = purchases.Document{
			ID: resource.ID(r.ID), AdministrationID: admin, Kind: r.Kind, ContactID: contact, State: r.State, Currency: r.Currency,
			PricesAreInclTax: r.PricesAreInclTax, Date: r.Date.Time, UpdatedAt: r.UpdatedAt.Time,
		}
		at[r.ID], ids[i] = j, r.ID
	}
	lines, err := s.q.SupplierHistoryLines(ctx, sqlc.SupplierHistoryLinesParams{AdministrationID: int64(admin), Ids: ids})
	if err != nil {
		return nil, err
	}
	for _, l := range lines {
		d := &docs[at[l.PurchaseDocumentID]]
		line := sales.Detail{Description: l.Description, Price: money.Amount(l.Price), TaxRateID: resource.ID(l.TaxRateID)}
		if l.LedgerAccountID.Valid {
			id := resource.ID(l.LedgerAccountID.Int64)
			line.LedgerAccountID = &id
		}
		d.Lines = append(d.Lines, line)
	}
	return docs, nil
}

func optionalInt8(id resource.ID) pgtype.Int8 {
	return pgtype.Int8{Int64: int64(id), Valid: id != 0}
}
