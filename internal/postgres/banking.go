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

	"github.com/adirkuhn/moneyharpy/internal/banking"
	"github.com/adirkuhn/moneyharpy/internal/postgres/sqlc"
	"github.com/adirkuhn/moneyharpy/internal/resource"
)

// BankingStore implements banking.Store.
type BankingStore struct {
	pool *pgxpool.Pool
	q    *sqlc.Queries
}

// NewBankingStore returns a BankingStore on pool.
func NewBankingStore(pool *pgxpool.Pool) *BankingStore {
	return &BankingStore{pool: pool, q: sqlc.New(pool)}
}

func optionalTime(t *time.Time) pgtype.Timestamptz {
	if t == nil {
		return pgtype.Timestamptz{}
	}
	return timestamp(*t)
}

func timeOf(t pgtype.Timestamptz) *time.Time {
	if !t.Valid {
		return nil
	}
	v := t.Time
	return &v
}

func optionalDate(t *time.Time) pgtype.Date {
	if t == nil {
		return pgtype.Date{}
	}
	return pgtype.Date{Time: *t, Valid: true}
}

func dateOf(d pgtype.Date) *time.Time {
	if !d.Valid {
		return nil
	}
	v := time.Date(d.Time.Year(), d.Time.Month(), d.Time.Day(), 0, 0, 0, 0, time.UTC)
	return &v
}

func ids64(ids []resource.ID) []int64 {
	out := make([]int64, len(ids))
	for i, id := range ids {
		out[i] = int64(id)
	}
	return out
}

// Settings implements banking.Store.
func (s *BankingStore) Settings(ctx context.Context, admin resource.ID) (banking.Settings, error) {
	r, err := s.q.BankSettings(ctx, int64(admin))
	if err != nil {
		return banking.Settings{}, err
	}
	return banking.Settings{AutoLink: r.BankAutoLink, Seeded: r.BankSeeded}, nil
}

// SetAutoLink implements banking.Store.
func (s *BankingStore) SetAutoLink(ctx context.Context, admin resource.ID, on bool) error {
	return s.q.SetBankAutoLink(ctx, sqlc.SetBankAutoLinkParams{ID: int64(admin), BankAutoLink: on})
}

// Seed implements banking.Store.
func (s *BankingStore) Seed(ctx context.Context, admin resource.ID, accounts []banking.Account) (bool, error) {
	seeded := false
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		q := s.q.WithTx(tx)
		n, err := q.MarkBankSeeded(ctx, int64(admin))
		if err != nil || n == 0 {
			return err
		}
		seeded = true
		for _, a := range accounts {
			if err := insertAccount(ctx, q, a); err != nil {
				return err
			}
		}
		return nil
	})
	return seeded, err
}

// Accounts implements banking.Store.
func (s *BankingStore) Accounts(ctx context.Context, admin resource.ID) ([]banking.Account, error) {
	rows, err := s.q.ListFinancialAccounts(ctx, int64(admin))
	if err != nil {
		return nil, err
	}
	out := make([]banking.Account, len(rows))
	for i, r := range rows {
		out[i] = toAccount(r)
	}
	return out, nil
}

// Account implements banking.Store.
func (s *BankingStore) Account(ctx context.Context, admin, id resource.ID) (banking.Account, error) {
	r, err := s.q.FinancialAccountByID(ctx, sqlc.FinancialAccountByIDParams{AdministrationID: int64(admin), ID: int64(id)})
	if errors.Is(err, pgx.ErrNoRows) {
		return banking.Account{}, banking.ErrAccountNotFound
	}
	if err != nil {
		return banking.Account{}, err
	}
	return toAccount(r), nil
}

// CreateAccount implements banking.Store.
func (s *BankingStore) CreateAccount(ctx context.Context, a banking.Account) error {
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error { return insertAccount(ctx, s.q.WithTx(tx), a) })
}

// UpdateAccounts implements banking.Store.
func (s *BankingStore) UpdateAccounts(ctx context.Context, accounts []banking.Account) error {
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		q := s.q.WithTx(tx)
		for _, a := range accounts {
			if err := q.UpdateFinancialAccount(ctx, sqlc.UpdateFinancialAccountParams{
				AdministrationID: int64(a.AdministrationID), ID: int64(a.ID), Type: a.Type, Name: a.Name, Identifier: a.Identifier,
				Currency: a.Currency, Provider: a.Provider, Active: a.Active, Position: int32(a.Position),
				LastImportAt: optionalTime(a.LastImportAt), UpdatedAt: timestamp(a.UpdatedAt),
			}); err != nil {
				return err
			}
		}
		return nil
	})
}

func insertAccount(ctx context.Context, q *sqlc.Queries, a banking.Account) error {
	if err := q.InsertFinancialAccount(ctx, sqlc.InsertFinancialAccountParams{
		ID: int64(a.ID), AdministrationID: int64(a.AdministrationID), Type: a.Type, Name: a.Name, Identifier: a.Identifier, Currency: a.Currency,
		Provider: a.Provider, Active: a.Active, Position: int32(a.Position), LedgerAccountID: int64(a.LedgerAccountID),
		LastImportAt: optionalTime(a.LastImportAt), CreatedAt: timestamp(a.CreatedAt), UpdatedAt: timestamp(a.UpdatedAt),
	}); err != nil {
		return err
	}
	return q.InsertResource(ctx, sqlc.InsertResourceParams{ID: int64(a.ID), Type: string(resource.TypeFinancialAccount), AdministrationID: int64(a.AdministrationID)})
}

func toAccount(r sqlc.FinancialAccount) banking.Account {
	return banking.Account{
		ID: resource.ID(r.ID), AdministrationID: resource.ID(r.AdministrationID), Type: r.Type, Name: r.Name, Identifier: r.Identifier,
		Currency: r.Currency, Provider: r.Provider, Active: r.Active, Position: int(r.Position), LedgerAccountID: resource.ID(r.LedgerAccountID),
		LastImportAt: timeOf(r.LastImportAt), CreatedAt: r.CreatedAt.Time, UpdatedAt: r.UpdatedAt.Time,
	}
}

// Statement implements banking.Store.
func (s *BankingStore) Statement(ctx context.Context, admin, id resource.ID) (banking.Statement, error) {
	r, err := s.q.FinancialStatementByID(ctx, sqlc.FinancialStatementByIDParams{AdministrationID: int64(admin), ID: int64(id)})
	if errors.Is(err, pgx.ErrNoRows) {
		return banking.Statement{}, banking.ErrStatementNotFound
	}
	if err != nil {
		return banking.Statement{}, err
	}
	st := banking.Statement{
		ID: resource.ID(r.ID), AdministrationID: resource.ID(r.AdministrationID), AccountID: resource.ID(r.FinancialAccountID), Reference: r.Reference,
		OfficialDate: dateOf(r.OfficialDate), Importer: r.Importer, CreatedAt: r.CreatedAt.Time, UpdatedAt: r.UpdatedAt.Time,
	}
	if r.OfficialBalance.Valid {
		b := banking.Amount(r.OfficialBalance.Int64)
		st.OfficialBalance = &b
	}
	rows, err := s.q.FinancialMutationsOfStatement(ctx, sqlc.FinancialMutationsOfStatementParams{AdministrationID: int64(admin), FinancialStatementID: int64(id)})
	if err != nil {
		return banking.Statement{}, err
	}
	if st.Mutations, err = s.withBookings(ctx, admin, rows); err != nil {
		return banking.Statement{}, err
	}
	return st, nil
}

// CreateStatement implements banking.Store.
func (s *BankingStore) CreateStatement(ctx context.Context, st banking.Statement, mutations []banking.Mutation) ([]banking.Mutation, error) {
	var added []banking.Mutation
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		q := s.q.WithTx(tx)
		added = nil
		var balance pgtype.Int8
		if st.OfficialBalance != nil {
			balance = pgtype.Int8{Int64: int64(*st.OfficialBalance), Valid: true}
		}
		if err := q.InsertFinancialStatement(ctx, sqlc.InsertFinancialStatementParams{
			ID: int64(st.ID), AdministrationID: int64(st.AdministrationID), FinancialAccountID: int64(st.AccountID), Reference: st.Reference,
			OfficialDate: optionalDate(st.OfficialDate), OfficialBalance: balance, Importer: st.Importer,
			CreatedAt: timestamp(st.CreatedAt), UpdatedAt: timestamp(st.UpdatedAt),
		}); err != nil {
			return err
		}
		var err error
		if added, err = insertMutations(ctx, q, mutations); err != nil {
			return err
		}
		if len(added) == 0 && len(mutations) > 0 {
			return q.DeleteFinancialStatement(ctx, sqlc.DeleteFinancialStatementParams{AdministrationID: int64(st.AdministrationID), ID: int64(st.ID)})
		}
		return q.InsertResource(ctx, sqlc.InsertResourceParams{ID: int64(st.ID), Type: string(resource.TypeFinancialStatement), AdministrationID: int64(st.AdministrationID)})
	})
	return added, err
}

// UpdateStatement implements banking.Store.
func (s *BankingStore) UpdateStatement(ctx context.Context, st banking.Statement, added, changed []banking.Mutation, removed []resource.ID) ([]banking.Mutation, error) {
	var inserted []banking.Mutation
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		q := s.q.WithTx(tx)
		var balance pgtype.Int8
		if st.OfficialBalance != nil {
			balance = pgtype.Int8{Int64: int64(*st.OfficialBalance), Valid: true}
		}
		if err := q.UpdateFinancialStatement(ctx, sqlc.UpdateFinancialStatementParams{
			AdministrationID: int64(st.AdministrationID), ID: int64(st.ID), Reference: st.Reference,
			OfficialDate: optionalDate(st.OfficialDate), OfficialBalance: balance, UpdatedAt: timestamp(st.UpdatedAt),
		}); err != nil {
			return err
		}
		var err error
		if inserted, err = insertMutations(ctx, q, added); err != nil {
			return err
		}
		for _, m := range changed {
			if err := updateMutation(ctx, q, m); err != nil {
				return err
			}
		}
		if len(removed) > 0 {
			if err := q.DeleteFinancialMutations(ctx, sqlc.DeleteFinancialMutationsParams{AdministrationID: int64(st.AdministrationID), Ids: ids64(removed)}); err != nil {
				return err
			}
			return q.DeleteResources(ctx, ids64(removed))
		}
		return nil
	})
	return inserted, err
}

// DeleteStatement implements banking.Store.
func (s *BankingStore) DeleteStatement(ctx context.Context, admin, id resource.ID) error {
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		q := s.q.WithTx(tx)
		rows, err := q.FinancialMutationsOfStatement(ctx, sqlc.FinancialMutationsOfStatementParams{AdministrationID: int64(admin), FinancialStatementID: int64(id)})
		if err != nil {
			return err
		}
		ids := []int64{int64(id)}
		for _, r := range rows {
			ids = append(ids, r.ID)
		}
		if err := q.DeleteFinancialStatement(ctx, sqlc.DeleteFinancialStatementParams{AdministrationID: int64(admin), ID: int64(id)}); err != nil {
			return err
		}
		return q.DeleteResources(ctx, ids)
	})
}

func insertMutations(ctx context.Context, q *sqlc.Queries, mutations []banking.Mutation) ([]banking.Mutation, error) {
	var added []banking.Mutation
	for _, m := range mutations {
		n, err := q.InsertFinancialMutation(ctx, sqlc.InsertFinancialMutationParams{
			ID: int64(m.ID), AdministrationID: int64(m.AdministrationID), FinancialAccountID: int64(m.AccountID), FinancialStatementID: int64(m.StatementID),
			Date: pgtype.Date{Time: m.Date, Valid: true}, ValutationDate: optionalDate(m.ValueDate), Message: m.Message, Amount: int64(m.Amount),
			Currency: m.Currency, Code: m.Code, ContraAccountName: m.ContraName, ContraAccountNumber: m.ContraAccount, BatchReference: m.BatchReference,
			TransactionID: m.TransactionID, Fingerprint: m.Fingerprint, Position: int32(m.Offset), State: m.State,
			ProcessedAt: optionalTime(m.ProcessedAt), CreatedAt: timestamp(m.CreatedAt), UpdatedAt: timestamp(m.UpdatedAt),
		})
		if err != nil {
			return nil, err
		}
		if n == 0 {
			continue // The account has this transaction already.
		}
		if err := q.InsertResource(ctx, sqlc.InsertResourceParams{ID: int64(m.ID), Type: string(resource.TypeFinancialMutation), AdministrationID: int64(m.AdministrationID)}); err != nil {
			return nil, err
		}
		added = append(added, m)
	}
	return added, nil
}

func updateMutation(ctx context.Context, q *sqlc.Queries, m banking.Mutation) error {
	return q.UpdateFinancialMutation(ctx, sqlc.UpdateFinancialMutationParams{
		AdministrationID: int64(m.AdministrationID), ID: int64(m.ID), Date: pgtype.Date{Time: m.Date, Valid: true}, ValutationDate: optionalDate(m.ValueDate),
		Message: m.Message, Amount: int64(m.Amount), Code: m.Code, ContraAccountName: m.ContraName, ContraAccountNumber: m.ContraAccount,
		BatchReference: m.BatchReference, TransactionID: m.TransactionID, Position: int32(m.Offset), State: m.State,
		ProcessedAt: optionalTime(m.ProcessedAt), UpdatedAt: timestamp(m.UpdatedAt),
	})
}

// Mutations implements banking.Store.
func (s *BankingStore) Mutations(ctx context.Context, admin resource.ID, mq banking.MutationQuery) ([]banking.Mutation, error) {
	p := sqlc.ListFinancialMutationsParams{AdministrationID: int64(admin)}
	if mq.AccountID != nil {
		p.FinancialAccountID = pgtype.Int8{Int64: int64(*mq.AccountID), Valid: true}
	}
	if mq.Period != nil {
		p.DateFrom = pgtype.Date{Time: mq.Period.From, Valid: true}
		p.DateTo = pgtype.Date{Time: mq.Period.To, Valid: true}
	}
	rows, err := s.q.ListFinancialMutations(ctx, p)
	if err != nil {
		return nil, err
	}
	return s.withBookings(ctx, admin, rows)
}

// MutationsByID implements banking.Store.
func (s *BankingStore) MutationsByID(ctx context.Context, admin resource.ID, ids []resource.ID) ([]banking.Mutation, error) {
	rows, err := s.q.FinancialMutationsByID(ctx, sqlc.FinancialMutationsByIDParams{AdministrationID: int64(admin), Ids: ids64(ids)})
	if err != nil {
		return nil, err
	}
	return s.withBookings(ctx, admin, rows)
}

// Mutation implements banking.Store.
func (s *BankingStore) Mutation(ctx context.Context, admin, id resource.ID) (banking.Mutation, error) {
	list, err := s.MutationsByID(ctx, admin, []resource.ID{id})
	if err != nil {
		return banking.Mutation{}, err
	}
	if len(list) == 0 {
		return banking.Mutation{}, banking.ErrMutationNotFound
	}
	return list[0], nil
}

// MutationVersions implements banking.Store.
func (s *BankingStore) MutationVersions(ctx context.Context, admin resource.ID) ([]banking.Version, error) {
	rows, err := s.q.FinancialMutationVersions(ctx, int64(admin))
	if err != nil {
		return nil, err
	}
	out := make([]banking.Version, len(rows))
	for i, r := range rows {
		out[i] = banking.Version{ID: resource.ID(r.ID), Version: r.UpdatedAt.Time.Unix()}
	}
	return out, nil
}

// SaveBookings implements banking.Store.
func (s *BankingStore) SaveBookings(ctx context.Context, m banking.Mutation, added []banking.Booking, removed []resource.ID) error {
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		q := s.q.WithTx(tx)
		for _, b := range added {
			var rule pgtype.Int8
			if b.RuleID != nil {
				rule = pgtype.Int8{Int64: int64(*b.RuleID), Valid: true}
			}
			if err := q.InsertLedgerAccountBooking(ctx, sqlc.InsertLedgerAccountBookingParams{
				ID: int64(b.ID), AdministrationID: int64(b.AdministrationID), FinancialMutationID: int64(b.MutationID), LedgerAccountID: int64(b.LedgerAccountID),
				Description: b.Description, Price: int64(b.Price), BookingRuleID: rule, CreatedAt: timestamp(b.CreatedAt), UpdatedAt: timestamp(b.UpdatedAt),
			}); err != nil {
				return err
			}
		}
		for _, id := range removed {
			if err := q.DeleteLedgerAccountBooking(ctx, sqlc.DeleteLedgerAccountBookingParams{AdministrationID: int64(m.AdministrationID), FinancialMutationID: int64(m.ID), ID: int64(id)}); err != nil {
				return err
			}
		}
		return updateMutation(ctx, q, m)
	})
}

func (s *BankingStore) withBookings(ctx context.Context, admin resource.ID, rows []sqlc.FinancialMutation) ([]banking.Mutation, error) {
	if len(rows) == 0 {
		return []banking.Mutation{}, nil
	}
	ids := make([]int64, len(rows))
	for i, r := range rows {
		ids[i] = r.ID
	}
	bookings, err := s.q.BookingsOfMutations(ctx, sqlc.BookingsOfMutationsParams{AdministrationID: int64(admin), Ids: ids})
	if err != nil {
		return nil, err
	}
	byMutation := map[int64][]banking.Booking{}
	for _, b := range bookings {
		bk := banking.Booking{
			ID: resource.ID(b.ID), AdministrationID: resource.ID(b.AdministrationID), MutationID: resource.ID(b.FinancialMutationID),
			LedgerAccountID: resource.ID(b.LedgerAccountID), Description: b.Description, Price: banking.Amount(b.Price),
			CreatedAt: b.CreatedAt.Time, UpdatedAt: b.UpdatedAt.Time,
		}
		if b.BookingRuleID.Valid {
			rule := resource.ID(b.BookingRuleID.Int64)
			bk.RuleID = &rule
		}
		byMutation[b.FinancialMutationID] = append(byMutation[b.FinancialMutationID], bk)
	}
	out := make([]banking.Mutation, len(rows))
	for i, r := range rows {
		out[i] = banking.Mutation{
			ID: resource.ID(r.ID), AdministrationID: resource.ID(r.AdministrationID), AccountID: resource.ID(r.FinancialAccountID),
			StatementID: resource.ID(r.FinancialStatementID), Date: *dateOf(r.Date), ValueDate: dateOf(r.ValutationDate), Message: r.Message,
			Amount: banking.Amount(r.Amount), Currency: r.Currency, Code: r.Code, ContraName: r.ContraAccountName, ContraAccount: r.ContraAccountNumber,
			BatchReference: r.BatchReference, TransactionID: r.TransactionID, Fingerprint: r.Fingerprint, Offset: int(r.Position), State: r.State,
			ProcessedAt: timeOf(r.ProcessedAt), Bookings: byMutation[r.ID], CreatedAt: r.CreatedAt.Time, UpdatedAt: r.UpdatedAt.Time,
		}
	}
	return out, nil
}

// Balances implements banking.Store.
func (s *BankingStore) Balances(ctx context.Context, admin resource.ID) (map[resource.ID]banking.Amount, error) {
	rows, err := s.q.FinancialBalances(ctx, int64(admin))
	if err != nil {
		return nil, err
	}
	out := make(map[resource.ID]banking.Amount, len(rows))
	for _, r := range rows {
		out[resource.ID(r.FinancialAccountID)] = banking.Amount(r.Balance)
	}
	return out, nil
}

// CountUnprocessed implements banking.Store.
func (s *BankingStore) CountUnprocessed(ctx context.Context, admin resource.ID) (int, error) {
	n, err := s.q.CountUnprocessedFinancialMutations(ctx, int64(admin))
	return int(n), err
}

// RecentlyProcessed implements banking.Store.
func (s *BankingStore) RecentlyProcessed(ctx context.Context, admin resource.ID, limit int) ([]banking.Mutation, error) {
	rows, err := s.q.RecentlyProcessedFinancialMutations(ctx, sqlc.RecentlyProcessedFinancialMutationsParams{AdministrationID: int64(admin), MaxRows: int32(min(max(limit, 0), 100))})
	if err != nil {
		return nil, err
	}
	return s.withBookings(ctx, admin, rows)
}

// Rules implements banking.Store.
func (s *BankingStore) Rules(ctx context.Context, admin resource.ID) ([]banking.Rule, error) {
	rows, err := s.q.ListBookingRules(ctx, int64(admin))
	if err != nil {
		return nil, err
	}
	out := make([]banking.Rule, len(rows))
	for i, r := range rows {
		if out[i], err = toRule(r); err != nil {
			return nil, err
		}
	}
	return out, nil
}

// Rule implements banking.Store.
func (s *BankingStore) Rule(ctx context.Context, admin, id resource.ID) (banking.Rule, error) {
	r, err := s.q.BookingRuleByID(ctx, sqlc.BookingRuleByIDParams{AdministrationID: int64(admin), ID: int64(id)})
	if errors.Is(err, pgx.ErrNoRows) {
		return banking.Rule{}, banking.ErrRuleNotFound
	}
	if err != nil {
		return banking.Rule{}, err
	}
	return toRule(r)
}

type conditionJSON struct {
	Field    string `json:"field"`
	Operator string `json:"operator"`
	Value    string `json:"value"`
}

func conditionsJSON(cs []banking.Condition) ([]byte, error) {
	out := make([]conditionJSON, len(cs))
	for i, c := range cs {
		out[i] = conditionJSON(c)
	}
	return json.Marshal(out)
}

// CreateRule implements banking.Store.
func (s *BankingStore) CreateRule(ctx context.Context, r banking.Rule) error {
	conditions, err := conditionsJSON(r.Conditions)
	if err != nil {
		return err
	}
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		q := s.q.WithTx(tx)
		if err := q.InsertBookingRule(ctx, sqlc.InsertBookingRuleParams{
			ID: int64(r.ID), AdministrationID: int64(r.AdministrationID), Name: r.Name, Active: r.Active, Position: int32(r.Position),
			MatchAll: r.MatchAll, Direction: r.Direction, FinancialAccountID: optionalID(r.AccountID), Conditions: conditions,
			LedgerAccountID: int64(r.LedgerAccountID), Description: r.Description, CreatedAt: timestamp(r.CreatedAt), UpdatedAt: timestamp(r.UpdatedAt),
		}); err != nil {
			return err
		}
		return q.InsertResource(ctx, sqlc.InsertResourceParams{ID: int64(r.ID), Type: string(resource.TypeBookingRule), AdministrationID: int64(r.AdministrationID)})
	})
}

// UpdateRules implements banking.Store.
func (s *BankingStore) UpdateRules(ctx context.Context, rules []banking.Rule) error {
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		q := s.q.WithTx(tx)
		for _, r := range rules {
			conditions, err := conditionsJSON(r.Conditions)
			if err != nil {
				return err
			}
			if err := q.UpdateBookingRule(ctx, sqlc.UpdateBookingRuleParams{
				AdministrationID: int64(r.AdministrationID), ID: int64(r.ID), Name: r.Name, Active: r.Active, Position: int32(r.Position),
				MatchAll: r.MatchAll, Direction: r.Direction, FinancialAccountID: optionalID(r.AccountID), Conditions: conditions,
				LedgerAccountID: int64(r.LedgerAccountID), Description: r.Description, UpdatedAt: timestamp(r.UpdatedAt),
			}); err != nil {
				return err
			}
		}
		return nil
	})
}

// DeleteRule implements banking.Store.
func (s *BankingStore) DeleteRule(ctx context.Context, admin, id resource.ID) error {
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		q := s.q.WithTx(tx)
		if err := q.DeleteBookingRule(ctx, sqlc.DeleteBookingRuleParams{AdministrationID: int64(admin), ID: int64(id)}); err != nil {
			return err
		}
		return q.DeleteResource(ctx, int64(id))
	})
}

func toRule(r sqlc.BookingRule) (banking.Rule, error) {
	var cs []conditionJSON
	if err := json.Unmarshal(r.Conditions, &cs); err != nil {
		return banking.Rule{}, fmt.Errorf("rule %d conditions: %w", r.ID, err)
	}
	out := banking.Rule{
		ID: resource.ID(r.ID), AdministrationID: resource.ID(r.AdministrationID), Name: r.Name, Active: r.Active, Position: int(r.Position),
		MatchAll: r.MatchAll, Direction: r.Direction, LedgerAccountID: resource.ID(r.LedgerAccountID), Description: r.Description,
		CreatedAt: r.CreatedAt.Time, UpdatedAt: r.UpdatedAt.Time, Conditions: make([]banking.Condition, len(cs)),
	}
	for i, c := range cs {
		out.Conditions[i] = banking.Condition(c)
	}
	if r.FinancialAccountID.Valid {
		id := resource.ID(r.FinancialAccountID.Int64)
		out.AccountID = &id
	}
	return out, nil
}
