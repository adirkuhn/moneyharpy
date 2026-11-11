package purchases

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/adirkuhn/moneyharpy/internal/iban"
	"github.com/adirkuhn/moneyharpy/internal/money"
	"github.com/adirkuhn/moneyharpy/internal/resource"
	"github.com/adirkuhn/moneyharpy/internal/sepa"
)

// Purchase transaction and batch states, with Moneybird's names.
const (
	Cancelled = "cancelled"
	Exported  = "exported"
)

// TransactionStates are the states a filter may name.
var TransactionStates = []string{Open, PendingPayment, Paid, Cancelled}

// BankAccountType is the financial account type that has an IBAN to pay
// credit transfers from.
const BankAccountType = "BankAccount"

// Transaction is a purchase transaction: a credit transfer to make, mostly
// to pay a purchase invoice or receipt (its payable).
type Transaction struct {
	ID                 resource.ID
	AdministrationID   resource.ID
	FinancialAccountID resource.ID
	BatchID            *resource.ID
	// PayableType is Document when PayableID is a purchase invoice or
	// receipt.
	PayableType string
	PayableID   *resource.ID
	ContactID   *resource.ID
	// Amount is positive: what goes out.
	Amount      money.Amount
	Date        *time.Time
	Description string
	EndToEndID  string
	IBAN        string
	IBANName    string
	BIC         string
	SourceIBAN  string
	SourceName  string
	Cancelled   bool
	// PayablePaid reports a paid payable; the store fills it in.
	PayablePaid bool
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// Ref locates the purchase transaction.
func (t Transaction) Ref() resource.Ref {
	return resource.Ref{ID: t.ID, Type: resource.TypePurchaseTransaction, AdministrationID: t.AdministrationID}
}

// State is cancelled, paid once its payable is, pending_payment while a
// batch pays it, and open before.
func (t Transaction) State() string {
	switch {
	case t.Cancelled:
		return Cancelled
	case t.PayablePaid:
		return Paid
	case t.BatchID != nil:
		return PendingPayment
	}
	return Open
}

// Batch groups purchase transactions paid from one financial account.
type Batch struct {
	ID                 resource.ID
	AdministrationID   resource.ID
	FinancialAccountID resource.ID
	Identifier         string
	// State is open, exported once its file was downloaded, or cancelled.
	State         string
	ExecutionDate time.Time
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

// Ref locates the batch.
func (b Batch) Ref() resource.Ref {
	return resource.Ref{ID: b.ID, Type: resource.TypePurchaseTransactionBatch, AdministrationID: b.AdministrationID}
}

// TransactionQuery selects purchase transactions, after Moneybird's
// filter.
type TransactionQuery struct {
	States []string
	Period *money.Period
	// Unbatched keeps only transactions in no batch.
	Unbatched bool
	BatchID   *resource.ID
	// PayableID keeps the transactions that pay one document.
	PayableID     *resource.ID
	Page, PerPage int
}

// ParseTransactionFilter reads Moneybird's purchase transaction filter:
// state, period, unbatched (true unless a batch is asked for) and
// purchase_transaction_batch_id.
func ParseTransactionFilter(filter string, today time.Time) (TransactionQuery, error) {
	var q TransactionQuery
	var verr resource.ValidationError
	unbatched := ""
	for term := range strings.SplitSeq(filter, ",") {
		term = strings.TrimSpace(term)
		if term == "" {
			continue
		}
		key, value, ok := strings.Cut(term, ":")
		if !ok {
			verr.Add("filter", "has a term without a value: "+term)
			continue
		}
		switch key {
		case "state":
			for st := range strings.SplitSeq(value, "|") {
				switch {
				case st == "all":
				case slices.Contains(TransactionStates, st):
					q.States = append(q.States, st)
				default:
					verr.Add("filter", "state "+st+" is not one of all, "+strings.Join(TransactionStates, ", "))
				}
			}
		case "period":
			p, err := money.ParsePeriod(value, today)
			if err != nil {
				verr.Add("filter", "period "+err.Error())
				continue
			}
			q.Period = &p
		case "unbatched":
			if value != "true" && value != "false" {
				verr.Add("filter", "unbatched is not true or false")
			}
			unbatched = value
		case "purchase_transaction_batch_id":
			id, err := resource.ParseID(value)
			if err != nil {
				verr.Add("filter", key+" is not an id")
				continue
			}
			q.BatchID = &id
		default:
			verr.Add("filter", "has an unknown key: "+key)
		}
	}
	q.Unbatched = unbatched == "true" || unbatched == "" && q.BatchID == nil
	if q.Unbatched && q.BatchID != nil {
		verr.Add("filter", "purchase_transaction_batch_id needs unbatched:false")
	}
	return q, verr.OrNil()
}

// Transactions returns the page of purchase transactions the query
// selects.
func (s *Service) Transactions(ctx context.Context, admin, user resource.ID, q TransactionQuery) ([]Transaction, int, error) {
	if _, err := s.admins.Get(ctx, admin, user); err != nil {
		return nil, 0, err
	}
	q.Page = max(q.Page, 1)
	if q.PerPage <= 0 {
		q.PerPage = DefaultPerPage
	}
	q.PerPage = min(q.PerPage, MaxPerPage)
	list, total, err := s.store.Transactions(ctx, admin, q)
	return list, total, wrap("list purchase transactions", err)
}

// Transaction returns one purchase transaction.
func (s *Service) Transaction(ctx context.Context, admin, id, user resource.ID) (Transaction, error) {
	if _, err := s.admins.Get(ctx, admin, user); err != nil {
		return Transaction{}, err
	}
	return s.store.Transaction(ctx, admin, id)
}

// DeleteTransaction removes a purchase transaction that no batch holds and
// that did not pay yet.
func (s *Service) DeleteTransaction(ctx context.Context, admin, id, user resource.ID) error {
	t, err := s.Transaction(ctx, admin, id, user)
	if err != nil {
		return err
	}
	if t.BatchID != nil || t.State() != Open {
		return invalid("base", "Only an open purchase transaction outside a batch can be deleted")
	}
	return wrap("delete purchase transaction", s.store.DeleteTransaction(ctx, admin, id))
}

// Batch returns one purchase transaction batch.
func (s *Service) Batch(ctx context.Context, admin, id, user resource.ID) (Batch, error) {
	if _, err := s.admins.Get(ctx, admin, user); err != nil {
		return Batch{}, err
	}
	return s.store.Batch(ctx, admin, id)
}

// PayInput makes a purchase transaction that pays a document.
type PayInput struct {
	DocumentID         resource.ID
	FinancialAccountID resource.ID
	// Date is when to pay; empty pays as soon as possible.
	Date string
}

// Pay makes an open purchase transaction for what a purchase invoice or
// receipt has unpaid, to the supplier's IBAN, to batch and export later.
func (s *Service) Pay(ctx context.Context, admin, user resource.ID, in PayInput) (Transaction, error) {
	if _, err := s.admins.Get(ctx, admin, user); err != nil {
		return Transaction{}, err
	}
	d, err := s.store.DocumentByID(ctx, admin, in.DocumentID)
	if errors.Is(err, ErrNotFound) {
		return Transaction{}, invalid("payable_id", "is not a purchase invoice or receipt of this administration")
	}
	if err != nil {
		return Transaction{}, err
	}
	var verr resource.ValidationError
	if d.State != Open || d.Unpaid() <= 0 {
		verr.Add("payable_id", "is not a document waiting for payment")
	}
	if d.Pending {
		verr.Add("payable_id", "is being paid by a batch already")
	}
	account, err := s.bankAccount(ctx, admin, user, in.FinancialAccountID)
	if err != nil {
		verr.Add("financial_account_id", err.Error())
	}
	var when *time.Time
	if v := strings.TrimSpace(in.Date); v != "" {
		day, err := time.Parse(time.DateOnly, v)
		if err != nil {
			verr.Add("date", "is not a date")
		}
		when = &day
	}
	t := Transaction{
		ID: s.ids.Next(), AdministrationID: admin, FinancialAccountID: in.FinancialAccountID, PayableType: PaymentType, PayableID: &d.ID,
		Amount: d.Unpaid(), Date: when, Description: cmpString(d.Reference, d.label()), EndToEndID: d.ID.String(),
		SourceIBAN: account.IBAN, SourceName: account.Name, CreatedAt: s.now(), UpdatedAt: s.now(),
	}
	if d.ContactID == 0 {
		verr.Add("payable_id", "has no supplier to pay")
	} else if c, err := s.contacts.Contact(ctx, admin, d.ContactID, user); err == nil {
		t.ContactID = &d.ContactID
		t.IBAN = iban.Normalize(cmpString(c.SepaIBAN, c.BankAccount))
		t.IBANName, t.BIC = cmpString(c.SepaIBANAccountName, c.Name()), c.SepaBIC
		if !iban.Valid(t.IBAN) {
			verr.Add("payable_id", "has a supplier without a valid IBAN")
		}
	} else {
		return Transaction{}, err
	}
	if err := verr.OrNil(); err != nil {
		return Transaction{}, err
	}
	if err := s.store.SaveTransactions(ctx, nil, []Transaction{t}, true); err != nil {
		return Transaction{}, wrap("store purchase transaction", err)
	}
	return s.store.Transaction(ctx, admin, t.ID)
}

// bankAccount returns a bank account to pay from, or why it cannot be.
func (s *Service) bankAccount(ctx context.Context, admin, user, id resource.ID) (BankAccount, error) {
	if s.bank == nil || id == 0 {
		return BankAccount{}, errors.New("does not exist")
	}
	a, err := s.bank.FinancialAccount(ctx, admin, id, user)
	switch {
	case err != nil:
		return BankAccount{}, errors.New("does not exist")
	case a.Type != BankAccountType || !iban.Valid(a.IBAN):
		return BankAccount{}, errors.New("is not a bank account with an IBAN")
	}
	return a, nil
}

// BatchInput makes a batch of open purchase transactions.
type BatchInput struct {
	FinancialAccountID resource.ID
	// TransactionIDs chooses the transactions; empty takes every open one
	// of the account outside a batch.
	TransactionIDs []resource.ID
	// ExecutionDate is when the bank pays; empty is today.
	ExecutionDate string
}

// CreateBatch groups open purchase transactions of a bank account in a
// batch, to export as one SEPA credit transfer file; the documents they pay
// are pending payment until the bank transactions come in.
func (s *Service) CreateBatch(ctx context.Context, admin, user resource.ID, in BatchInput) (Batch, error) {
	a, err := s.admins.Get(ctx, admin, user)
	if err != nil {
		return Batch{}, err
	}
	var verr resource.ValidationError
	if _, err := s.bankAccount(ctx, admin, user, in.FinancialAccountID); err != nil {
		verr.Add("financial_account_id", err.Error())
	}
	day := s.today(a)
	if v := strings.TrimSpace(in.ExecutionDate); v != "" {
		if day, err = time.Parse(time.DateOnly, v); err != nil {
			verr.Add("execution_date", "is not a date")
		}
	}
	if err := verr.OrNil(); err != nil {
		return Batch{}, err
	}
	open, _, err := s.store.Transactions(ctx, admin, TransactionQuery{States: []string{Open}, Unbatched: true})
	if err != nil {
		return Batch{}, wrap("list purchase transactions", err)
	}
	var chosen []Transaction
	for _, t := range open {
		if t.FinancialAccountID == in.FinancialAccountID && (len(in.TransactionIDs) == 0 || slices.Contains(in.TransactionIDs, t.ID)) {
			chosen = append(chosen, t)
		}
	}
	if len(chosen) == 0 || len(in.TransactionIDs) > 0 && len(chosen) != len(in.TransactionIDs) {
		return Batch{}, invalid("purchase_transaction_ids", "must name open purchase transactions of the financial account outside a batch")
	}
	now := s.now()
	b := Batch{
		ID: s.ids.Next(), AdministrationID: admin, FinancialAccountID: in.FinancialAccountID, State: Open, ExecutionDate: day, CreatedAt: now, UpdatedAt: now,
	}
	b.Identifier = "MH" + day.Format("20060102") + "-" + strconv.FormatInt(int64(b.ID)%100000, 10)
	for i := range chosen {
		chosen[i].BatchID, chosen[i].UpdatedAt = &b.ID, now
		if chosen[i].Date == nil {
			chosen[i].Date = &day
		}
	}
	if err := s.store.SaveTransactions(ctx, &b, chosen, false); err != nil {
		return Batch{}, wrap("store purchase transaction batch", err)
	}
	return b, nil
}

// DownloadBatch renders a batch as a SEPA credit transfer file
// (pain.001.001.09), and marks the batch exported.
func (s *Service) DownloadBatch(ctx context.Context, admin, id, user resource.ID) ([]byte, Batch, error) {
	a, err := s.admins.Get(ctx, admin, user)
	if err != nil {
		return nil, Batch{}, err
	}
	b, err := s.store.Batch(ctx, admin, id)
	if err != nil {
		return nil, Batch{}, err
	}
	if b.State == Cancelled {
		return nil, Batch{}, ErrNoSEPAFile
	}
	account, err := s.bankAccount(ctx, admin, user, b.FinancialAccountID)
	if err != nil {
		return nil, Batch{}, invalid("financial_account_id", err.Error())
	}
	list, _, err := s.store.Transactions(ctx, admin, TransactionQuery{BatchID: &b.ID})
	if err != nil {
		return nil, Batch{}, err
	}
	out := sepa.Batch{ID: b.Identifier, ExecutionDate: b.ExecutionDate, DebtorName: cmpString(a.Name, account.Name), DebtorIBAN: account.IBAN, DebtorBIC: account.BIC}
	for _, t := range list {
		if t.Cancelled {
			continue
		}
		out.Transactions = append(out.Transactions, sepa.Transaction{
			InstructionID: t.ID.String(), EndToEndID: t.EndToEndID, Amount: t.Amount, Currency: a.Currency, CreditorName: t.IBANName,
			CreditorIBAN: t.IBAN, CreditorBIC: t.BIC, Remittance: t.Description,
		})
	}
	if len(out.Transactions) == 0 {
		return nil, Batch{}, ErrNoSEPAFile
	}
	file, err := sepa.Write(sepa.Transfer{MessageID: b.Identifier, Created: s.now(), InitiatorName: cmpString(a.Name, account.Name), Batches: []sepa.Batch{out}})
	if err != nil {
		return nil, Batch{}, fmt.Errorf("render SEPA file: %w", err)
	}
	if b.State == Open {
		b.State, b.UpdatedAt = Exported, s.now()
		if err := s.store.SaveBatch(ctx, b); err != nil {
			return nil, Batch{}, wrap("store purchase transaction batch", err)
		}
	}
	return file, b, nil
}

// ImportInput is an uploaded SEPA credit transfer file.
type ImportInput struct {
	File []byte
	// FinancialAccountID pays every batch in the file; nil finds each
	// batch's account by its IBAN.
	FinancialAccountID *resource.ID
	// LedgerAccountID is accepted for compatibility.
	LedgerAccountID *resource.ID
}

// ImportCreditTransfer reads a SEPA credit transfer file the bank was or
// will be given and makes a batch per payment information block, with a
// purchase transaction per transfer. A transfer whose message is the
// reference of an open purchase invoice or receipt with that amount
// unpaid pays it.
func (s *Service) ImportCreditTransfer(ctx context.Context, admin, user resource.ID, in ImportInput) ([]Batch, error) {
	a, err := s.admins.Get(ctx, admin, user)
	if err != nil {
		return nil, err
	}
	file, err := sepa.Parse(in.File)
	if err != nil {
		return nil, invalid("file", err.Error())
	}
	if in.LedgerAccountID != nil {
		if _, err := s.ledger.LedgerAccount(ctx, admin, *in.LedgerAccountID, user); err != nil {
			return nil, invalid("ledger_account_id", "is not a category of this administration")
		}
	}
	var accounts []BankAccount
	if in.FinancialAccountID != nil {
		acc, err := s.bankAccount(ctx, admin, user, *in.FinancialAccountID)
		if err != nil {
			return nil, invalid("financial_account_id", err.Error())
		}
		accounts = []BankAccount{acc}
	} else if s.bank != nil {
		if accounts, err = s.bank.FinancialAccounts(ctx, admin, user); err != nil {
			return nil, err
		}
	}
	now := s.now()
	type made struct {
		batch Batch
		list  []Transaction
	}
	var all []made
	for i, fb := range file.Batches {
		at := 0
		if in.FinancialAccountID == nil {
			at = slices.IndexFunc(accounts, func(acc BankAccount) bool { return acc.IBAN != "" && iban.Normalize(acc.IBAN) == fb.DebtorIBAN })
		}
		if at < 0 || len(accounts) == 0 {
			return nil, invalid("financial_account_id", fmt.Sprintf("can't be blank: no financial account has the IBAN %s of payment information %d", fb.DebtorIBAN, i+1))
		}
		acc := accounts[at]
		b := Batch{
			ID: s.ids.Next(), AdministrationID: admin, FinancialAccountID: acc.ID, Identifier: cmpString(fb.ID, fmt.Sprintf("%s-%d", file.MessageID, i+1)),
			State: Open, ExecutionDate: fb.ExecutionDate, CreatedAt: now, UpdatedAt: now,
		}
		m := made{batch: b}
		for _, ft := range fb.Transactions {
			day := fb.ExecutionDate
			t := Transaction{
				ID: s.ids.Next(), AdministrationID: admin, FinancialAccountID: acc.ID, BatchID: &b.ID, Amount: ft.Amount, Date: &day,
				Description: ft.Remittance, EndToEndID: ft.EndToEndID, IBAN: ft.CreditorIBAN, IBANName: ft.CreditorName, BIC: ft.CreditorBIC,
				SourceIBAN: fb.DebtorIBAN, SourceName: fb.DebtorName, CreatedAt: now, UpdatedAt: now,
			}
			if d, ok, err := s.match(ctx, a.ID, ft); err != nil {
				return nil, err
			} else if ok {
				t.PayableType, t.PayableID, t.ContactID = PaymentType, &d.ID, optional(d.ContactID)
			}
			m.list = append(m.list, t)
		}
		all = append(all, m)
	}
	out := make([]Batch, 0, len(all))
	for _, m := range all {
		if err := s.store.SaveTransactions(ctx, &m.batch, m.list, true); err != nil {
			return nil, wrap("store purchase transaction batch", err)
		}
		out = append(out, m.batch)
	}
	return out, nil
}

// match finds the open document a transfer pays: its message is the
// document's reference, or its end-to-end id the document's id, and the
// amount is what the document has unpaid.
func (s *Service) match(ctx context.Context, admin resource.ID, t sepa.Transaction) (Document, bool, error) {
	if id, err := resource.ParseID(t.EndToEndID); err == nil {
		d, err := s.store.DocumentByID(ctx, admin, id)
		if err == nil && d.State == Open && !d.Pending && d.Unpaid() == t.Amount {
			return d, true, nil
		}
		if err != nil && !errors.Is(err, ErrNotFound) {
			return Document{}, false, err
		}
	}
	if t.Remittance == "" {
		return Document{}, false, nil
	}
	list, _, err := s.store.Documents(ctx, admin, Query{States: []string{Open, Late}, References: []string{t.Remittance}, Today: s.now()})
	if err != nil {
		return Document{}, false, err
	}
	for _, d := range list {
		if d.Unpaid() == t.Amount && !d.Pending {
			return d, true, nil
		}
	}
	return Document{}, false, nil
}
