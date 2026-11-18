package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/adirkuhn/moneyharpy/internal/journal"
	"github.com/adirkuhn/moneyharpy/internal/money"
	"github.com/adirkuhn/moneyharpy/internal/postgres/sqlc"
	"github.com/adirkuhn/moneyharpy/internal/resource"
	"github.com/adirkuhn/moneyharpy/internal/vatreturns"
)

// VatReturnStore implements vatreturns.Store.
type VatReturnStore struct {
	pool *pgxpool.Pool
	q    *sqlc.Queries
}

// NewVatReturnStore returns a VatReturnStore on pool.
func NewVatReturnStore(pool *pgxpool.Pool) *VatReturnStore {
	return &VatReturnStore{pool: pool, q: sqlc.New(pool)}
}

type boxJSON struct {
	Code string `json:"code"`
	Base int64  `json:"base"`
	Tax  int64  `json:"tax"`
}

type correctionJSON struct {
	From  string    `json:"from"`
	To    string    `json:"to"`
	Boxes []boxJSON `json:"boxes"`
}

type icpJSON struct {
	ContactID int64  `json:"contact_id"`
	Name      string `json:"name"`
	Country   string `json:"country"`
	VATNumber string `json:"vat_number"`
	Kind      string `json:"kind"`
	Amount    int64  `json:"amount"`
}

func boxesJSON(boxes []vatreturns.Box) []boxJSON {
	out := make([]boxJSON, len(boxes))
	for i, b := range boxes {
		out[i] = boxJSON{Code: b.Code, Base: int64(b.Base), Tax: int64(b.Tax)}
	}
	return out
}

func boxesOf(rows []boxJSON) []vatreturns.Box {
	out := make([]vatreturns.Box, len(rows))
	for i, b := range rows {
		out[i] = vatreturns.Box{Code: b.Code, Base: money.Amount(b.Base), Tax: money.Amount(b.Tax)}
	}
	return out
}

func encodeReturn(r vatreturns.Return) (boxes, corrections, icp []byte, err error) {
	cs := make([]correctionJSON, len(r.Corrections))
	for i, c := range r.Corrections {
		cs[i] = correctionJSON{From: c.Period.From.Format(time.DateOnly), To: c.Period.To.Format(time.DateOnly), Boxes: boxesJSON(c.Boxes)}
	}
	lines := make([]icpJSON, len(r.ICP))
	for i, l := range r.ICP {
		lines[i] = icpJSON{ContactID: int64(l.ContactID), Name: l.Name, Country: l.Country, VATNumber: l.VATNumber, Kind: l.Kind, Amount: int64(l.Amount)}
	}
	if boxes, err = json.Marshal(boxesJSON(r.Boxes)); err != nil {
		return nil, nil, nil, err
	}
	if corrections, err = json.Marshal(cs); err != nil {
		return nil, nil, nil, err
	}
	icp, err = json.Marshal(lines)
	return boxes, corrections, icp, err
}

func decodeReturn(row sqlc.VatReturn) (vatreturns.Return, error) {
	r := vatreturns.Return{
		ID: resource.ID(row.ID), AdministrationID: resource.ID(row.AdministrationID), Kind: row.Kind,
		Period: money.Period{From: dayOf(row.PeriodFrom), To: dayOf(row.PeriodTo)}, Granularity: row.Granularity, SentOn: dayOf(row.SentOn),
		CreatedAt: row.CreatedAt.Time, UpdatedAt: row.UpdatedAt.Time, Corrections: []vatreturns.Correction{}, ICP: []vatreturns.ICPLine{},
		Payments: []vatreturns.Payment{},
	}
	if row.CreatedBy.Valid {
		r.CreatedBy = resource.ID(row.CreatedBy.Int64)
	}
	var boxes []boxJSON
	var cs []correctionJSON
	var lines []icpJSON
	if err := json.Unmarshal(row.Boxes, &boxes); err != nil {
		return r, err
	}
	if err := json.Unmarshal(row.Corrections, &cs); err != nil {
		return r, err
	}
	if err := json.Unmarshal(row.IcpLines, &lines); err != nil {
		return r, err
	}
	r.Boxes = boxesOf(boxes)
	for _, c := range cs {
		from, err1 := time.Parse(time.DateOnly, c.From)
		to, err2 := time.Parse(time.DateOnly, c.To)
		if err := errors.Join(err1, err2); err != nil {
			return r, err
		}
		r.Corrections = append(r.Corrections, vatreturns.Correction{Period: money.Period{From: from, To: to}, Boxes: boxesOf(c.Boxes)})
	}
	for _, l := range lines {
		r.ICP = append(r.ICP, vatreturns.ICPLine{
			ContactID: resource.ID(l.ContactID), Name: l.Name, Country: l.Country, VATNumber: l.VATNumber, Kind: l.Kind, Amount: money.Amount(l.Amount),
		})
	}
	return r, nil
}

// withPayments adds the payments of the returns.
func (s *VatReturnStore) withPayments(ctx context.Context, admin resource.ID, list []vatreturns.Return) error {
	ids := make([]int64, len(list))
	for i, r := range list {
		ids[i] = int64(r.ID)
	}
	rows, err := s.q.PaymentsOfInvoices(ctx, sqlc.PaymentsOfInvoicesParams{AdministrationID: int64(admin), InvoiceType: vatreturns.DocumentType, Ids: ids})
	if err != nil {
		return err
	}
	for _, p := range rows {
		for i := range list {
			if int64(list[i].ID) != p.InvoiceID {
				continue
			}
			pay := vatreturns.Payment{ID: resource.ID(p.ID), Date: dayOf(p.PaymentDate), Price: money.Amount(p.Price), CreatedAt: p.CreatedAt.Time}
			if p.FinancialMutationID.Valid {
				pay.MutationID = resource.ID(p.FinancialMutationID.Int64)
			}
			if p.FinancialAccountID.Valid {
				pay.AccountID = resource.ID(p.FinancialAccountID.Int64)
			}
			list[i].Payments = append(list[i].Payments, pay)
		}
	}
	return nil
}

// Returns implements vatreturns.Store.
func (s *VatReturnStore) Returns(ctx context.Context, admin resource.ID) ([]vatreturns.Return, error) {
	rows, err := s.q.ListVatReturns(ctx, int64(admin))
	if err != nil {
		return nil, err
	}
	out := make([]vatreturns.Return, len(rows))
	for i, row := range rows {
		if out[i], err = decodeReturn(row); err != nil {
			return nil, err
		}
	}
	return out, s.withPayments(ctx, admin, out)
}

// Return implements vatreturns.Store.
func (s *VatReturnStore) Return(ctx context.Context, admin, id resource.ID) (vatreturns.Return, error) {
	row, err := s.q.GetVatReturn(ctx, sqlc.GetVatReturnParams{AdministrationID: int64(admin), ID: int64(id)})
	if errors.Is(err, pgx.ErrNoRows) {
		return vatreturns.Return{}, vatreturns.ErrNotFound
	}
	if err != nil {
		return vatreturns.Return{}, err
	}
	r, err := decodeReturn(row)
	if err != nil {
		return r, err
	}
	list := []vatreturns.Return{r}
	if err := s.withPayments(ctx, admin, list); err != nil {
		return r, err
	}
	return list[0], nil
}

// Create implements vatreturns.Store.
func (s *VatReturnStore) Create(ctx context.Context, r vatreturns.Return, entries []journal.Entry) error {
	boxes, corrections, icp, err := encodeReturn(r)
	if err != nil {
		return err
	}
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		q := s.q.WithTx(tx)
		if err := q.InsertVatReturn(ctx, sqlc.InsertVatReturnParams{
			ID: int64(r.ID), AdministrationID: int64(r.AdministrationID), Kind: r.Kind, PeriodFrom: day(r.Period.From), PeriodTo: day(r.Period.To),
			Granularity: r.Granularity, SentOn: day(r.SentOn), Boxes: boxes, Corrections: corrections, IcpLines: icp,
			CreatedBy: optionalID(&r.CreatedBy), CreatedAt: timestamp(r.CreatedAt), UpdatedAt: timestamp(r.UpdatedAt),
		}); err != nil {
			return err
		}
		if err := q.InsertResource(ctx, sqlc.InsertResourceParams{ID: int64(r.ID), Type: string(resource.TypeVatDocument), AdministrationID: int64(r.AdministrationID)}); err != nil {
			return err
		}
		return postEntries(ctx, q, r.AdministrationID, vatreturns.DocumentType, r.ID, entries, r.CreatedAt)
	})
}

// Delete implements vatreturns.Store.
func (s *VatReturnStore) Delete(ctx context.Context, admin, id resource.ID) error {
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		q := s.q.WithTx(tx)
		if err := q.DeleteJournalEntriesOf(ctx, sqlc.DeleteJournalEntriesOfParams{AdministrationID: int64(admin), DocumentType: vatreturns.DocumentType, DocumentID: int64(id)}); err != nil {
			return err
		}
		if err := q.DeleteVatReturn(ctx, sqlc.DeleteVatReturnParams{AdministrationID: int64(admin), ID: int64(id)}); err != nil {
			return err
		}
		return q.DeleteResource(ctx, int64(id))
	})
}

// SavePayment implements vatreturns.Store.
func (s *VatReturnStore) SavePayment(ctx context.Context, r vatreturns.Return, p vatreturns.Payment, user resource.ID) error {
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		q := s.q.WithTx(tx)
		if err := q.UpsertPayment(ctx, sqlc.UpsertPaymentParams{
			ID: int64(p.ID), AdministrationID: int64(r.AdministrationID), InvoiceType: vatreturns.DocumentType, InvoiceID: int64(r.ID),
			FinancialAccountID: optionalID(&p.AccountID), FinancialMutationID: optionalID(&p.MutationID), Price: int64(p.Price), PriceBase: int64(p.Price),
			PaymentDate: day(p.Date), UserID: optionalID(&user), CreatedAt: timestamp(p.CreatedAt), UpdatedAt: timestamp(p.CreatedAt),
		}); err != nil {
			return err
		}
		return q.TouchVatReturn(ctx, sqlc.TouchVatReturnParams{AdministrationID: int64(r.AdministrationID), ID: int64(r.ID), UpdatedAt: timestamp(p.CreatedAt)})
	})
}

// DeletePayment implements vatreturns.Store.
func (s *VatReturnStore) DeletePayment(ctx context.Context, admin, id resource.ID) error {
	return s.q.DeletePayment(ctx, sqlc.DeletePaymentParams{AdministrationID: int64(admin), ID: int64(id)})
}

// PaymentReturn implements vatreturns.Store.
func (s *VatReturnStore) PaymentReturn(ctx context.Context, admin, id resource.ID) (resource.ID, error) {
	p, err := s.q.PaymentByID(ctx, sqlc.PaymentByIDParams{AdministrationID: int64(admin), ID: int64(id)})
	if errors.Is(err, pgx.ErrNoRows) || err == nil && p.InvoiceType != vatreturns.DocumentType {
		return 0, vatreturns.ErrNotFound
	}
	if err != nil {
		return 0, err
	}
	return resource.ID(p.InvoiceID), nil
}

// RateSums implements vatreturns.Store.
func (s *VatReturnStore) RateSums(ctx context.Context, admin resource.ID, p money.Period) ([]vatreturns.RateSum, error) {
	rows, err := s.q.ReportTaxSums(ctx, sqlc.ReportTaxSumsParams{AdministrationID: int64(admin), FromDate: day(p.From), ToDate: day(p.To)})
	if err != nil {
		return nil, err
	}
	out := make([]vatreturns.RateSum, len(rows))
	for i, r := range rows {
		out[i] = vatreturns.RateSum{
			Type: r.TaxRateType, Percentage: trimPercentage(r.Percentage), Boxes: r.ReportBoxes,
			Base: money.Amount(r.Base), Payable: money.Amount(r.Payable), Receivable: money.Amount(r.Receivable),
		}
	}
	return out, nil
}

// ICPSums implements vatreturns.Store.
func (s *VatReturnStore) ICPSums(ctx context.Context, admin resource.ID, p money.Period) ([]vatreturns.ICPLine, error) {
	rows, err := s.q.VatICPSums(ctx, sqlc.VatICPSumsParams{AdministrationID: int64(admin), FromDate: day(p.From), ToDate: day(p.To)})
	if err != nil {
		return nil, err
	}
	out := make([]vatreturns.ICPLine, len(rows))
	for i, r := range rows {
		out[i] = vatreturns.ICPLine{ContactID: resource.ID(r.ContactID), Name: r.Name, Country: r.Country, VATNumber: r.TaxNumber, Amount: money.Amount(r.Amount)}
	}
	return out, nil
}

// Checks implements vatreturns.Store.
func (s *VatReturnStore) Checks(ctx context.Context, admin resource.ID, p money.Period) (vatreturns.Checks, error) {
	r, err := s.q.VatChecks(ctx, sqlc.VatChecksParams{AdministrationID: int64(admin), FromDate: day(p.From), ToDate: day(p.To)})
	if err != nil {
		return vatreturns.Checks{}, err
	}
	return vatreturns.Checks{
		OpenTransactions: int(r.OpenTransactions), BookingsWithoutVAT: int(r.BookingsWithoutVat), UnprocessedDocuments: int(r.UnprocessedDocuments),
	}, nil
}

// FirstEntry implements vatreturns.Store.
func (s *VatReturnStore) FirstEntry(ctx context.Context, admin resource.ID) (*time.Time, error) {
	d, err := s.q.FirstLedgerEntry(ctx, int64(admin))
	if err != nil || !d.Valid {
		return nil, err
	}
	t := dayOf(d)
	return &t, nil
}
