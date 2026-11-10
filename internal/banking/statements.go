package banking

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/adirkuhn/moneyharpy/internal/bankfile"
	"github.com/adirkuhn/moneyharpy/internal/resource"
)

// Where a statement's transactions came from.
const (
	ImporterAPI    = "api"
	ImporterManual = "manual"
)

// Statement groups the transactions that came in together, such as one
// bank statement or one file.
type Statement struct {
	ID               resource.ID
	AdministrationID resource.ID
	AccountID        resource.ID
	Reference        string
	OfficialDate     *time.Time
	OfficialBalance  *Amount
	// Importer names the source: a file format, api or manual.
	Importer  string
	Mutations []Mutation
	CreatedAt time.Time
	UpdatedAt time.Time
}

// Ref locates the statement.
func (s Statement) Ref() resource.Ref {
	return resource.Ref{ID: s.ID, Type: resource.TypeFinancialStatement, AdministrationID: s.AdministrationID}
}

// MutationInput is a transaction to add, change or remove. ID names an
// existing one; Destroy removes it.
type MutationInput struct {
	ID             *resource.ID
	Destroy        bool
	Date           *string
	ValueDate      *string
	Message        *string
	Amount         *string
	Code           *string
	ContraName     *string
	ContraAccount  *string
	BatchReference *string
	Offset         *int
	TransactionID  *string
}

// StatementInput creates or changes a statement. Nil fields stay as they
// are.
type StatementInput struct {
	AccountID       *resource.ID
	Reference       *string
	OfficialDate    *string
	OfficialBalance *string
	// Manual marks transactions people typed in themselves.
	Manual    bool
	Mutations []MutationInput
}

// Statement returns one statement with its transactions.
func (s *Service) Statement(ctx context.Context, admin, id, user resource.ID) (Statement, error) {
	if _, err := s.ensure(ctx, admin, user); err != nil {
		return Statement{}, err
	}
	return s.store.Statement(ctx, admin, id)
}

// CreateStatement adds a statement with its transactions. Transactions the
// account has already, by the bank's reference, are skipped.
func (s *Service) CreateStatement(ctx context.Context, admin, user resource.ID, in StatementInput) (Statement, error) {
	if _, err := s.ensure(ctx, admin, user); err != nil {
		return Statement{}, err
	}
	var verr resource.ValidationError
	if in.AccountID == nil {
		verr.Add("financial_account_id", "can't be blank")
		return Statement{}, &verr
	}
	acc, err := s.store.Account(ctx, admin, *in.AccountID)
	if errors.Is(err, ErrAccountNotFound) {
		verr.Add("financial_account_id", "does not exist")
		return Statement{}, &verr
	}
	if err != nil {
		return Statement{}, err
	}
	now := s.now()
	st := Statement{ID: s.ids.Next(), AdministrationID: admin, AccountID: acc.ID, Importer: ImporterAPI, CreatedAt: now, UpdatedAt: now}
	if in.Manual {
		st.Importer = ImporterManual
	}
	if err := applyStatement(&st, in, &verr); err != nil {
		return Statement{}, err
	}
	locked, err := s.locked(ctx, admin, user)
	if err != nil {
		return Statement{}, err
	}
	var mutations []Mutation
	for i, mi := range in.Mutations {
		if mi.ID != nil || mi.Destroy {
			verr.Add(fmt.Sprintf("financial_mutations_attributes.%d", i), "can only add transactions")
			continue
		}
		m := Mutation{ID: s.ids.Next(), AdministrationID: admin, AccountID: acc.ID, StatementID: st.ID, Currency: acc.Currency, State: Unprocessed, Offset: i, CreatedAt: now, UpdatedAt: now}
		applyMutation(&m, mi, i, locked, &verr)
		mutations = append(mutations, m)
	}
	if err := verr.OrNil(); err != nil {
		return Statement{}, err
	}
	return s.persistStatement(ctx, admin, user, acc, st, mutations)
}

// persistStatement stores a new statement, books what the rules match and marks the
// account updated.
func (s *Service) persistStatement(ctx context.Context, admin, user resource.ID, acc Account, st Statement, mutations []Mutation) (Statement, error) {
	added, err := s.store.CreateStatement(ctx, st, mutations)
	if err != nil {
		return Statement{}, fmt.Errorf("store statement: %w", err)
	}
	if len(added) > 0 {
		if added, err = s.autoBook(ctx, admin, user, added); err != nil {
			return Statement{}, err
		}
		now := s.now()
		acc.LastImportAt, acc.UpdatedAt = &now, now
		if err := s.store.UpdateAccounts(ctx, []Account{acc}); err != nil {
			return Statement{}, fmt.Errorf("store account: %w", err)
		}
	}
	st.Mutations = added
	return st, nil
}

// UpdateStatement changes a statement: its reference and balance, and
// transactions added, changed or removed. Booked transactions cannot
// change.
func (s *Service) UpdateStatement(ctx context.Context, admin, id, user resource.ID, in StatementInput) (Statement, error) {
	if _, err := s.ensure(ctx, admin, user); err != nil {
		return Statement{}, err
	}
	st, err := s.store.Statement(ctx, admin, id)
	if err != nil {
		return Statement{}, err
	}
	var verr resource.ValidationError
	if in.AccountID != nil && *in.AccountID != st.AccountID {
		verr.Add("financial_account_id", "cannot change")
	}
	if err := applyStatement(&st, in, &verr); err != nil {
		return Statement{}, err
	}
	locked, err := s.locked(ctx, admin, user)
	if err != nil {
		return Statement{}, err
	}
	byID := map[resource.ID]Mutation{}
	offset := 0
	for _, m := range st.Mutations {
		byID[m.ID] = m
		offset = max(offset, m.Offset+1)
	}
	now := s.now()
	var added, changed []Mutation
	var removed []resource.ID
	for i, mi := range in.Mutations {
		field := fmt.Sprintf("financial_mutations_attributes.%d", i)
		if mi.ID == nil {
			m := Mutation{ID: s.ids.Next(), AdministrationID: admin, AccountID: st.AccountID, StatementID: st.ID, State: Unprocessed, Offset: offset + i, CreatedAt: now, UpdatedAt: now}
			if len(st.Mutations) > 0 {
				m.Currency = st.Mutations[0].Currency
			}
			applyMutation(&m, mi, i, locked, &verr)
			added = append(added, m)
			continue
		}
		m, ok := byID[*mi.ID]
		switch {
		case !ok:
			verr.Add(field, "is not a transaction of this statement")
			continue
		case len(m.Bookings) > 0:
			verr.Add(field, "is booked; remove its bookings first")
			continue
		case locked != nil && !m.Date.After(*locked):
			verr.Add(field, "is in the locked period")
			continue
		}
		if mi.Destroy {
			removed = append(removed, m.ID)
			continue
		}
		applyMutation(&m, mi, i, locked, &verr)
		m.UpdatedAt = now
		changed = append(changed, m)
	}
	if err := verr.OrNil(); err != nil {
		return Statement{}, err
	}
	st.UpdatedAt = now
	if added, err = s.store.UpdateStatement(ctx, st, added, changed, removed); err != nil {
		return Statement{}, fmt.Errorf("store statement: %w", err)
	}
	if _, err := s.autoBook(ctx, admin, user, added); err != nil {
		return Statement{}, err
	}
	return s.store.Statement(ctx, admin, id)
}

// DeleteStatement removes a statement and its transactions, as long as none
// is booked.
func (s *Service) DeleteStatement(ctx context.Context, admin, id, user resource.ID) error {
	if _, err := s.ensure(ctx, admin, user); err != nil {
		return err
	}
	st, err := s.store.Statement(ctx, admin, id)
	if err != nil {
		return err
	}
	for _, m := range st.Mutations {
		if len(m.Bookings) > 0 {
			return ErrStatementBooked
		}
	}
	return s.store.DeleteStatement(ctx, admin, id)
}

func applyStatement(st *Statement, in StatementInput, verr *resource.ValidationError) error {
	if in.Reference != nil {
		st.Reference = strings.TrimSpace(*in.Reference)
	}
	if st.Reference == "" || len(st.Reference) > 255 {
		verr.Add("reference", "can't be blank")
	}
	if in.OfficialDate != nil {
		st.OfficialDate = nil
		if *in.OfficialDate != "" {
			day, err := time.Parse(time.DateOnly, *in.OfficialDate)
			if err != nil {
				verr.Add("official_date", "must be a date")
			}
			st.OfficialDate = &day
		}
	}
	if in.OfficialBalance != nil {
		st.OfficialBalance = nil
		if *in.OfficialBalance != "" {
			b, err := ParseAmount(*in.OfficialBalance)
			if err != nil {
				verr.Add("official_balance", err.Error())
			}
			st.OfficialBalance = &b
		}
	}
	if len(in.Mutations) > 10000 {
		verr.Add("financial_mutations_attributes", "can hold at most 10000 transactions")
	}
	return nil
}

func applyMutation(m *Mutation, in MutationInput, i int, locked *time.Time, verr *resource.ValidationError) {
	field := fmt.Sprintf("financial_mutations_attributes.%d.", i)
	text := func(dst *string, src *string, name string, limit int) {
		if src == nil {
			return
		}
		*dst = strings.TrimSpace(*src)
		if len(*dst) > limit {
			verr.Add(field+name, fmt.Sprintf("is too long (maximum is %d characters)", limit))
		}
	}
	if in.Date != nil {
		day, err := time.Parse(time.DateOnly, *in.Date)
		if err != nil {
			verr.Add(field+"date", "must be a date")
		}
		m.Date = day
	}
	if m.Date.IsZero() {
		verr.Add(field+"date", "can't be blank")
	} else if locked != nil && !m.Date.After(*locked) {
		verr.Add(field+"date", "is in the locked period, which ends on "+locked.Format(time.DateOnly))
	}
	if in.ValueDate != nil {
		m.ValueDate = nil
		if *in.ValueDate != "" {
			day, err := time.Parse(time.DateOnly, *in.ValueDate)
			if err != nil {
				verr.Add(field+"valutation_date", "must be a date")
			}
			m.ValueDate = &day
		}
	}
	if in.Amount != nil {
		a, err := ParseAmount(*in.Amount)
		if err != nil {
			verr.Add(field+"amount", err.Error())
		}
		m.Amount = a
	}
	if m.Amount == 0 {
		verr.Add(field+"amount", "must not be zero")
	}
	text(&m.Message, in.Message, "message", 5000)
	text(&m.Code, in.Code, "code", 64)
	text(&m.ContraName, in.ContraName, "contra_account_name", 255)
	text(&m.ContraAccount, in.ContraAccount, "contra_account_number", 64)
	text(&m.BatchReference, in.BatchReference, "batch_reference", 255)
	text(&m.TransactionID, in.TransactionID, "account_servicer_transaction_id", 255)
	m.ContraAccount = strings.ToUpper(strings.ReplaceAll(m.ContraAccount, " ", ""))
	if in.Offset != nil {
		m.Offset = *in.Offset
	}
}

// ImportResult tells what an import did.
type ImportResult struct {
	Format     string
	Statements []Statement
	// Imported counts new transactions, Duplicates the ones the account had
	// already, Locked the ones in the locked period, AutoBooked the ones a
	// rule booked.
	Imported, Duplicates, Locked, AutoBooked int
}

// Import reads a CAMT or MT940 file into the account. A file for another
// account number is refused.
func (s *Service) Import(ctx context.Context, admin, id, user resource.ID, data []byte) (ImportResult, error) {
	if _, err := s.ensure(ctx, admin, user); err != nil {
		return ImportResult{}, err
	}
	acc, err := s.store.Account(ctx, admin, id)
	if err != nil {
		return ImportResult{}, err
	}
	var verr resource.ValidationError
	files, format, err := bankfile.Parse(data)
	if err != nil {
		verr.Add("file", err.Error())
		return ImportResult{}, &verr
	}
	for _, f := range files {
		if f.Account != "" && acc.Identifier != "" && !strings.EqualFold(f.Account, acc.Identifier) {
			verr.Add("file", "is for account "+f.Account+", not "+acc.Identifier)
		}
		if f.Currency != "" && f.Currency != acc.Currency {
			verr.Add("file", "is in "+f.Currency+", not "+acc.Currency)
		}
	}
	if err := verr.OrNil(); err != nil {
		return ImportResult{}, err
	}
	locked, err := s.locked(ctx, admin, user)
	if err != nil {
		return ImportResult{}, err
	}
	result := ImportResult{Format: string(format)}
	for _, f := range files {
		now := s.now()
		st := Statement{ID: s.ids.Next(), AdministrationID: admin, AccountID: acc.ID, Reference: f.Reference, Importer: string(format), CreatedAt: now, UpdatedAt: now}
		if st.Reference == "" {
			st.Reference = "Import " + now.Format(time.DateOnly)
		}
		st.OfficialDate = f.Date
		if f.Balance != nil {
			b := Amount(*f.Balance)
			st.OfficialBalance = &b
		}
		seen := map[string]int{}
		var mutations []Mutation
		for i, e := range f.Entries {
			if locked != nil && !e.Date.After(*locked) {
				result.Locked++
				continue
			}
			key := strings.Join([]string{e.Date.Format(time.DateOnly), strconv.FormatInt(e.Amount, 10), e.Message, e.ContraName, e.ContraAccount}, "|")
			seen[key]++
			sum := sha256.Sum256([]byte(acc.ID.String() + "|" + key + "|" + strconv.Itoa(seen[key])))
			mutations = append(mutations, Mutation{
				ID: s.ids.Next(), AdministrationID: admin, AccountID: acc.ID, StatementID: st.ID, Date: e.Date, ValueDate: e.ValueDate,
				Message: truncate(e.Message, 5000), Amount: Amount(e.Amount), Currency: acc.Currency, Code: truncate(e.Code, 64),
				ContraName: truncate(e.ContraName, 255), ContraAccount: truncate(e.ContraAccount, 64), BatchReference: truncate(e.BatchReference, 255),
				TransactionID: truncate(e.TransactionID, 255), Fingerprint: hex.EncodeToString(sum[:]), Offset: i, State: Unprocessed,
				CreatedAt: now, UpdatedAt: now,
			})
		}
		stored, err := s.persistStatement(ctx, admin, user, acc, st, mutations)
		if err != nil {
			return ImportResult{}, err
		}
		if len(stored.Mutations) > 0 {
			acc.LastImportAt = &now
			result.Statements = append(result.Statements, stored)
		}
		result.Imported += len(stored.Mutations)
		result.Duplicates += len(mutations) - len(stored.Mutations)
		for _, m := range stored.Mutations {
			if m.AutoBooked() {
				result.AutoBooked++
			}
		}
	}
	return result, nil
}

func truncate(s string, limit int) string {
	if len(s) <= limit {
		return s
	}
	for limit > 0 && !utf8Start(s[limit]) {
		limit--
	}
	return s[:limit]
}

func utf8Start(b byte) bool { return b&0xC0 != 0x80 }
