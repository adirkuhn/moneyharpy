package postgres

import (
	"context"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/adirkuhn/moneyharpy/internal/journal"
	"github.com/adirkuhn/moneyharpy/internal/money"
	"github.com/adirkuhn/moneyharpy/internal/postgres/sqlc"
	"github.com/adirkuhn/moneyharpy/internal/reports"
	"github.com/adirkuhn/moneyharpy/internal/resource"
)

// ReportStore implements reports.Store.
type ReportStore struct{ q *sqlc.Queries }

// NewReportStore returns a ReportStore on pool.
func NewReportStore(pool *pgxpool.Pool) *ReportStore { return &ReportStore{q: sqlc.New(pool)} }

func optionalDay(t *time.Time) pgtype.Date {
	if t == nil {
		return pgtype.Date{}
	}
	return day(*t)
}

func optionalDayOf(d pgtype.Date) *time.Time {
	if !d.Valid {
		return nil
	}
	t := dayOf(d)
	return &t
}

// Accounts implements reports.Store.
func (s *ReportStore) Accounts(ctx context.Context, admin resource.ID) ([]reports.Account, error) {
	rows, err := s.q.ReportLedgerAccounts(ctx, int64(admin))
	if err != nil {
		return nil, err
	}
	out := make([]reports.Account, len(rows))
	for i, r := range rows {
		out[i] = reports.Account{
			ID: resource.ID(r.ID), Name: r.Name, Type: r.AccountType, Code: r.AccountID, ParentID: idOf(r.ParentID), Active: r.Active,
			Taxonomy: r.TaxonomyCode, System: r.SystemRole, Financial: r.Financial,
		}
	}
	return out, nil
}

// Movements implements reports.Store.
func (s *ReportStore) Movements(ctx context.Context, admin resource.ID, from *time.Time, to time.Time, project *resource.ID) ([]reports.Sum, error) {
	rows, err := s.q.ReportMovements(ctx, sqlc.ReportMovementsParams{AdministrationID: int64(admin), FromDate: optionalDay(from), ToDate: day(to), ProjectID: optionalID(project)})
	if err != nil {
		return nil, err
	}
	out := make([]reports.Sum, len(rows))
	for i, r := range rows {
		out[i] = reports.Sum{LedgerAccountID: resource.ID(r.LedgerAccountID), Amount: money.Amount(r.Amount)}
	}
	return out, nil
}

// YearResults implements reports.Store.
func (s *ReportStore) YearResults(ctx context.Context, admin resource.ID, to time.Time) ([]reports.YearResult, error) {
	rows, err := s.q.ReportYearResults(ctx, sqlc.ReportYearResultsParams{AdministrationID: int64(admin), ToDate: day(to)})
	if err != nil {
		return nil, err
	}
	out := make([]reports.YearResult, len(rows))
	for i, r := range rows {
		out[i] = reports.YearResult{Year: int(r.Year), Amount: money.Amount(r.Amount)}
	}
	return out, nil
}

// ContactSums implements reports.Store.
func (s *ReportStore) ContactSums(ctx context.Context, admin resource.ID, q reports.SubjectQuery) ([]reports.SubjectSum, error) {
	rows, err := s.q.ReportContactSums(ctx, sqlc.ReportContactSumsParams{
		AdministrationID: int64(admin), FromDate: optionalDay(q.From), ToDate: day(q.To), AccountTypes: q.Types, SystemRoles: q.Roles, ContactID: optionalID(q.ContactID),
	})
	if err != nil {
		return nil, err
	}
	out := make([]reports.SubjectSum, len(rows))
	for i, r := range rows {
		out[i] = reports.SubjectSum{ID: idOf(r.ContactID), Amount: money.Amount(r.Amount)}
	}
	return out, nil
}

// ProjectSums implements reports.Store.
func (s *ReportStore) ProjectSums(ctx context.Context, admin resource.ID, q reports.SubjectQuery) ([]reports.SubjectSum, error) {
	rows, err := s.q.ReportProjectSums(ctx, sqlc.ReportProjectSumsParams{
		AdministrationID: int64(admin), FromDate: optionalDay(q.From), ToDate: day(q.To), AccountTypes: q.Types, SystemRoles: q.Roles,
	})
	if err != nil {
		return nil, err
	}
	out := make([]reports.SubjectSum, len(rows))
	for i, r := range rows {
		out[i] = reports.SubjectSum{ID: idOf(r.ProjectID), Amount: money.Amount(r.Amount)}
	}
	return out, nil
}

// CashFlow implements reports.Store.
func (s *ReportStore) CashFlow(ctx context.Context, admin resource.ID, p money.Period, account *resource.ID) ([]reports.CashSum, error) {
	rows, err := s.q.ReportCashFlow(ctx, sqlc.ReportCashFlowParams{AdministrationID: int64(admin), FromDate: day(p.From), ToDate: day(p.To), FinancialAccountID: optionalID(account)})
	if err != nil {
		return nil, err
	}
	out := make([]reports.CashSum, len(rows))
	for i, r := range rows {
		out[i] = reports.CashSum{LedgerAccountID: resource.ID(r.LedgerAccountID), Received: money.Amount(r.Received), Paid: money.Amount(r.Paid)}
	}
	return out, nil
}

// CashBalance implements reports.Store.
func (s *ReportStore) CashBalance(ctx context.Context, admin resource.ID, at time.Time, account *resource.ID) (money.Amount, error) {
	b, err := s.q.ReportCashBalance(ctx, sqlc.ReportCashBalanceParams{AdministrationID: int64(admin), ToDate: day(at), FinancialAccountID: optionalID(account)})
	return money.Amount(b), err
}

// TaxSums implements reports.Store.
func (s *ReportStore) TaxSums(ctx context.Context, admin resource.ID, p money.Period) ([]reports.TaxSum, error) {
	rows, err := s.q.ReportTaxSums(ctx, sqlc.ReportTaxSumsParams{AdministrationID: int64(admin), FromDate: day(p.From), ToDate: day(p.To)})
	if err != nil {
		return nil, err
	}
	out := make([]reports.TaxSum, len(rows))
	for i, r := range rows {
		out[i] = reports.TaxSum{
			TaxRateID: resource.ID(r.TaxRateID), Name: r.Name, Percentage: trimPercentage(r.Percentage), Type: r.TaxRateType, Boxes: r.ReportBoxes,
			Base: money.Amount(r.Base), Payable: money.Amount(r.Payable), Receivable: money.Amount(r.Receivable),
		}
	}
	return out, nil
}

// trimPercentage writes 21.00 as Moneybird does: 21.0.
func trimPercentage(p string) string {
	if !strings.Contains(p, ".") {
		return p + ".0"
	}
	p = strings.TrimRight(p, "0")
	if strings.HasSuffix(p, ".") {
		p += "0"
	}
	return p
}

// OpenItems implements reports.Store.
func (s *ReportStore) OpenItems(ctx context.Context, admin resource.ID, role string, until time.Time) ([]reports.OpenItem, error) {
	rows, err := s.q.ReportOpenItems(ctx, sqlc.ReportOpenItemsParams{AdministrationID: int64(admin), SystemRole: role, Until: day(until)})
	if err != nil {
		return nil, err
	}
	out := make([]reports.OpenItem, len(rows))
	for i, r := range rows {
		out[i] = reports.OpenItem{ContactID: idOf(r.ContactID), Date: dayOf(r.Date), Amount: money.Amount(r.Amount), Paid: money.Amount(r.Paid)}
	}
	return out, nil
}

// Subscriptions implements reports.Store.
func (s *ReportStore) Subscriptions(ctx context.Context, admin resource.ID) ([]reports.SubscriptionItem, error) {
	rows, err := s.q.ReportSubscriptions(ctx, int64(admin))
	if err != nil {
		return nil, err
	}
	out := make([]reports.SubscriptionItem, len(rows))
	for i, r := range rows {
		out[i] = reports.SubscriptionItem{
			SubscriptionID: resource.ID(r.ID), ContactID: resource.ID(r.ContactID), Start: dayOf(r.StartDate), End: optionalDayOf(r.EndDate),
			ItemStart: dayOf(r.ItemStart), ItemEnd: optionalDayOf(r.ItemEnd), Quantity: r.Quantity, Discount: int64(r.Discount),
			Price: money.Amount(r.Price), Every: int(r.FrequencyCount), Unit: r.FrequencyType,
		}
		if r.CancelledAt.Valid {
			out[i].CancelledAt = &r.CancelledAt.Time
		}
	}
	return out, nil
}

// LedgerEntries implements reports.Store.
func (s *ReportStore) LedgerEntries(ctx context.Context, admin, ledger resource.ID, p money.Period, page, perPage int) ([]journal.Entry, int, error) {
	rows, err := s.q.ReportLedgerAccountEntries(ctx, sqlc.ReportLedgerAccountEntriesParams{
		AdministrationID: int64(admin), LedgerAccountID: int64(ledger), FromDate: day(p.From), ToDate: day(p.To),
		MaxRows: int32(perPage), Skip: int32((max(page, 1) - 1) * perPage),
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

// Entries implements reports.Store.
func (s *ReportStore) Entries(ctx context.Context, admin resource.ID, p money.Period) ([]journal.Entry, error) {
	rows, err := s.q.ReportYearEntries(ctx, sqlc.ReportYearEntriesParams{AdministrationID: int64(admin), FromDate: day(p.From), ToDate: day(p.To)})
	if err != nil {
		return nil, err
	}
	out := make([]journal.Entry, len(rows))
	for i, r := range rows {
		out[i] = journal.Entry{
			ID: resource.ID(r.ID), AdministrationID: resource.ID(r.AdministrationID), DocumentType: r.DocumentType, DocumentID: resource.ID(r.DocumentID),
			Date: dayOf(r.Date), LedgerAccountID: resource.ID(r.LedgerAccountID), ContactID: idOf(r.ContactID), ProjectID: idOf(r.ProjectID),
			TaxRateID: idOf(r.TaxRateID), FinancialAccountID: idOf(r.FinancialAccountID), Amount: money.Amount(r.Amount), Description: r.Description,
			RowOrder: int(r.RowOrder),
		}
	}
	return out, nil
}

// Contacts implements reports.Store.
func (s *ReportStore) Contacts(ctx context.Context, admin resource.ID, ids []resource.ID) ([]reports.Contact, error) {
	rows, err := s.q.ReportContacts(ctx, sqlc.ReportContactsParams{AdministrationID: int64(admin), Ids: ids64(ids)})
	if err != nil {
		return nil, err
	}
	out := make([]reports.Contact, len(rows))
	for i, r := range rows {
		name := r.CompanyName
		if name == "" {
			name = strings.TrimSpace(r.Firstname + " " + r.Lastname)
		}
		out[i] = reports.Contact{
			ID: resource.ID(r.ID), Name: name, CustomerID: r.CustomerID, Address: r.Address1, Zipcode: r.Zipcode, City: r.City, Country: r.Country,
			TaxNumber: r.TaxNumber,
		}
	}
	return out, nil
}
