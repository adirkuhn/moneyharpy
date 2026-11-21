package banking

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/adirkuhn/moneyharpy/internal/resource"
)

// ImportedAccount is a financial account brought in from another system,
// such as Moneybird, with the id it had there.
type ImportedAccount struct {
	Origin  resource.Origin
	Changes AccountChanges
	// LedgerAccountID is the account's category, brought in before; nil
	// gives it a new one, as a new account gets.
	LedgerAccountID *resource.ID
	// TakeOverCash lets a cash account take the place of the administration's
	// own cash account, which every new administration has, rather than sit
	// beside it.
	TakeOverCash bool
}

// ImportAccount adds a financial account brought in from another system,
// keeping its id. It checks the fields as CreateAccount does. Bringing in
// an account that is here already changes nothing.
func (s *Service) ImportAccount(ctx context.Context, admin, user resource.ID, in ImportedAccount) (Account, error) {
	a, err := s.ensure(ctx, admin, user)
	if err != nil {
		return Account{}, err
	}
	existing, err := s.accounts(ctx, admin)
	if err != nil {
		return Account{}, err
	}
	if i := slices.IndexFunc(existing, func(e Account) bool { return e.ID == in.Origin.ID }); i >= 0 {
		return existing[i], nil
	}
	now := s.now()
	if in.TakeOverCash {
		cash := slices.DeleteFunc(slices.Clone(existing), func(e Account) bool { return e.Type != CashAccount || e.Provider != "" })
		if len(cash) == 1 {
			acc := cash[0]
			if err := applyAccount(&acc, in.Changes, existing); err != nil {
				return Account{}, err
			}
			if in.LedgerAccountID != nil {
				acc.LedgerAccountID = *in.LedgerAccountID
			}
			acc.UpdatedAt = now
			if err := s.store.UpdateAccounts(ctx, []Account{acc}); err != nil {
				return Account{}, fmt.Errorf("store account: %w", err)
			}
			return acc, nil
		}
	}
	created, updated := in.Origin.Or(now)
	acc := Account{ID: in.Origin.ID, AdministrationID: admin, Type: BankAccount, Currency: a.Currency, Active: true, CreatedAt: created, UpdatedAt: updated}
	if err := applyAccount(&acc, in.Changes, existing); err != nil {
		return Account{}, err
	}
	for _, e := range existing {
		acc.Position = max(acc.Position, e.Position)
	}
	acc.Position++
	if in.LedgerAccountID != nil {
		acc.LedgerAccountID = *in.LedgerAccountID
	} else if acc.LedgerAccountID, err = s.ledger.FinancialLedgerAccount(ctx, admin, user, acc.Name, acc.Type); err != nil {
		return Account{}, fmt.Errorf("add category: %w", err)
	}
	if err := s.store.CreateAccount(ctx, acc); err != nil {
		return Account{}, fmt.Errorf("store account: %w", err)
	}
	return acc, nil
}

// ImportedMutation is a transaction brought in with its id.
type ImportedMutation struct {
	Origin resource.Origin
	Input  MutationInput
}

// ImportedStatement is a statement brought in with its id and transactions.
type ImportedStatement struct {
	Origin    resource.Origin
	AccountID resource.ID
	Reference string
	Mutations []ImportedMutation
}

// ImportStatement adds a statement brought in from another system, keeping
// the ids of the statement and its transactions. The transactions come in
// unprocessed and no booking rule touches them: how they were booked comes
// in with what they were booked on. It returns the transactions added;
// ones here already are skipped.
func (s *Service) ImportStatement(ctx context.Context, admin, user resource.ID, in ImportedStatement) ([]Mutation, error) {
	if _, err := s.ensure(ctx, admin, user); err != nil {
		return nil, err
	}
	acc, err := s.store.Account(ctx, admin, in.AccountID)
	if errors.Is(err, ErrAccountNotFound) {
		return nil, invalidField("financial_account_id", "does not exist")
	}
	if err != nil {
		return nil, err
	}
	now := s.now()
	created, updated := in.Origin.Or(now)
	st := Statement{ID: in.Origin.ID, AdministrationID: admin, AccountID: acc.ID, Reference: in.Reference, Importer: ImporterAPI, CreatedAt: created, UpdatedAt: updated}
	var verr resource.ValidationError
	var mutations []Mutation
	for i, im := range in.Mutations {
		if _, err := s.store.Mutation(ctx, admin, im.Origin.ID); err == nil {
			continue
		}
		mc, mu := im.Origin.Or(created)
		m := Mutation{ID: im.Origin.ID, AdministrationID: admin, AccountID: acc.ID, StatementID: st.ID, Currency: acc.Currency, State: Unprocessed, Offset: i, CreatedAt: mc, UpdatedAt: mu}
		applyMutation(&m, im.Input, i, nil, &verr)
		mutations = append(mutations, m)
	}
	if err := verr.OrNil(); err != nil {
		return nil, err
	}
	if len(mutations) == 0 {
		return nil, nil
	}
	if _, err := s.store.Statement(ctx, admin, st.ID); err == nil {
		// Part of the statement came in before: the rest joins it.
		added, err := s.store.UpdateStatement(ctx, st, mutations, nil, nil)
		return added, wrapErr("store statement", err)
	}
	added, err := s.store.CreateStatement(ctx, st, mutations)
	return added, wrapErr("store statement", err)
}

// LinkImportedPayment links a payment brought in on its invoice to the
// transaction that made it in the source, when the invoice came in before
// the transaction did. Like the rest of an import it takes the books as the
// source has them, the locked period too. It reports whether it linked one;
// ErrBookingNotFound says the invoice has no such payment here.
func (s *Service) LinkImportedPayment(ctx context.Context, admin, user, mutation resource.ID, invoiceType string, invoice resource.ID, price Amount) (bool, error) {
	if _, err := s.ensure(ctx, admin, user); err != nil {
		return false, err
	}
	m, err := s.store.Mutation(ctx, admin, mutation)
	if err != nil {
		return false, err
	}
	if slices.ContainsFunc(m.Payments, func(p PaymentLink) bool {
		return p.InvoiceType == invoiceType && p.InvoiceID == invoice && p.Price == price
	}) {
		return false, s.settleImported(ctx, admin, m)
	}
	found, err := s.store.AttachPayment(ctx, admin, mutation, invoiceType, invoice, price)
	if err != nil {
		return false, fmt.Errorf("link payment: %w", err)
	}
	if !found {
		return false, ErrBookingNotFound
	}
	if _, err := s.resettle(ctx, admin, mutation); err != nil {
		return false, err
	}
	return true, nil
}

// SettleImported settles a transaction whose payments came in linked to it
// with their invoices, as the transaction settles when they are linked here.
func (s *Service) SettleImported(ctx context.Context, admin, user, mutation resource.ID) error {
	if _, err := s.ensure(ctx, admin, user); err != nil {
		return err
	}
	m, err := s.store.Mutation(ctx, admin, mutation)
	if err != nil {
		return err
	}
	return s.settleImported(ctx, admin, m)
}

func (s *Service) settleImported(ctx context.Context, admin resource.ID, m Mutation) error {
	settled := m
	settled.settle(s.now())
	if settled.State == m.State {
		return nil
	}
	_, err := s.resettle(ctx, admin, m.ID)
	return err
}

func invalidField(field, problem string) error {
	var verr resource.ValidationError
	verr.Add(field, problem)
	return &verr
}

func wrapErr(what string, err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%s: %w", what, err)
}
