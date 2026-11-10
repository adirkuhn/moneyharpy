package banking

import (
	"cmp"
	"context"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/adirkuhn/moneyharpy/internal/iban"

	"github.com/adirkuhn/moneyharpy/internal/resource"
)

// Account types, with Moneybird's names.
const (
	BankAccount           = "BankAccount"
	CashAccount           = "CashAccount"
	CreditCardAccount     = "CreditCardAccount"
	PaymentServiceAccount = "PaymentServiceAccount"
	CustomAccount         = "CustomAccount"
)

// AccountTypes are the kinds of account people can add.
var AccountTypes = []string{BankAccount, CashAccount, CreditCardAccount, PaymentServiceAccount, CustomAccount}

var (
	currencyPattern = regexp.MustCompile(`^[A-Z]{3}$`)
	// ibanShape matches what can only be meant as an IBAN: a country code,
	// check digits and at least eleven more characters.
	ibanShape = regexp.MustCompile(`^[A-Z]{2}[0-9]{2}[A-Z0-9]{11,30}$`)
)

// Account is a financial account: where money is kept.
type Account struct {
	ID               resource.ID
	AdministrationID resource.ID
	Type             string
	Name             string
	// Identifier is the account number, such as an IBAN, or a card's last
	// digits.
	Identifier string
	Currency   string
	// Provider names the connection transactions come through; empty for
	// files and manual entry.
	Provider string
	Active   bool
	Position int
	// LedgerAccountID is the category the account's money is kept in.
	LedgerAccountID resource.ID
	// LastImportAt is when transactions last came in.
	LastImportAt *time.Time
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// Ref locates the account.
func (a Account) Ref() resource.Ref {
	return resource.Ref{ID: a.ID, Type: resource.TypeFinancialAccount, AdministrationID: a.AdministrationID}
}

// AccountChanges holds the fields to set; nil fields stay as they are.
type AccountChanges struct {
	Type       *string
	Name       *string
	Identifier *string
	Currency   *string
	Active     *bool
}

// Accounts lists the financial accounts in their set order.
func (s *Service) Accounts(ctx context.Context, admin, user resource.ID) ([]Account, error) {
	if _, err := s.ensure(ctx, admin, user); err != nil {
		return nil, err
	}
	return s.accounts(ctx, admin)
}

func (s *Service) accounts(ctx context.Context, admin resource.ID) ([]Account, error) {
	list, err := s.store.Accounts(ctx, admin)
	if err != nil {
		return nil, fmt.Errorf("list accounts: %w", err)
	}
	slices.SortStableFunc(list, func(a, b Account) int { return cmp.Or(cmp.Compare(a.Position, b.Position), cmp.Compare(a.ID, b.ID)) })
	return list, nil
}

// Account returns one financial account.
func (s *Service) Account(ctx context.Context, admin, id, user resource.ID) (Account, error) {
	if _, err := s.ensure(ctx, admin, user); err != nil {
		return Account{}, err
	}
	return s.store.Account(ctx, admin, id)
}

// CreateAccount adds a financial account, last in the order, with its own
// category.
func (s *Service) CreateAccount(ctx context.Context, admin, user resource.ID, c AccountChanges) (Account, error) {
	a, err := s.ensure(ctx, admin, user)
	if err != nil {
		return Account{}, err
	}
	now := s.now()
	acc := Account{ID: s.ids.Next(), AdministrationID: admin, Type: BankAccount, Currency: a.Currency, Active: true, CreatedAt: now, UpdatedAt: now}
	existing, err := s.accounts(ctx, admin)
	if err != nil {
		return Account{}, err
	}
	if err := applyAccount(&acc, c, existing); err != nil {
		return Account{}, err
	}
	for _, e := range existing {
		acc.Position = max(acc.Position, e.Position)
	}
	acc.Position++
	if acc.LedgerAccountID, err = s.ledger.FinancialLedgerAccount(ctx, admin, user, acc.Name, acc.Type); err != nil {
		return Account{}, fmt.Errorf("add category: %w", err)
	}
	if err := s.store.CreateAccount(ctx, acc); err != nil {
		return Account{}, fmt.Errorf("store account: %w", err)
	}
	return acc, nil
}

// UpdateAccount changes a financial account.
func (s *Service) UpdateAccount(ctx context.Context, admin, id, user resource.ID, c AccountChanges) (Account, error) {
	if _, err := s.ensure(ctx, admin, user); err != nil {
		return Account{}, err
	}
	acc, err := s.store.Account(ctx, admin, id)
	if err != nil {
		return Account{}, err
	}
	existing, err := s.accounts(ctx, admin)
	if err != nil {
		return Account{}, err
	}
	if err := applyAccount(&acc, c, existing); err != nil {
		return Account{}, err
	}
	acc.UpdatedAt = s.now()
	if err := s.store.UpdateAccounts(ctx, []Account{acc}); err != nil {
		return Account{}, fmt.Errorf("store account: %w", err)
	}
	return acc, nil
}

// ReorderAccounts puts the accounts in the order of ids, which must name
// each of them once.
func (s *Service) ReorderAccounts(ctx context.Context, admin, user resource.ID, ids []resource.ID) error {
	list, err := s.Accounts(ctx, admin, user)
	if err != nil {
		return err
	}
	current := make([]resource.ID, len(list))
	byID := map[resource.ID]Account{}
	for i, a := range list {
		current[i], byID[a.ID] = a.ID, a
	}
	if !slices.Equal(slices.Sorted(slices.Values(current)), slices.Sorted(slices.Values(ids))) {
		var verr resource.ValidationError
		verr.Add("ids", "must list every financial account once")
		return &verr
	}
	var changed []Account
	for i, id := range ids {
		if a := byID[id]; a.Position != i+1 {
			a.Position, a.UpdatedAt = i+1, s.now()
			changed = append(changed, a)
		}
	}
	if len(changed) == 0 {
		return nil
	}
	return s.store.UpdateAccounts(ctx, changed)
}

func applyAccount(a *Account, c AccountChanges, existing []Account) error {
	if c.Type != nil {
		a.Type = *c.Type
	}
	if c.Name != nil {
		a.Name = strings.TrimSpace(*c.Name)
	}
	if c.Identifier != nil {
		a.Identifier = strings.ToUpper(strings.Join(strings.Fields(*c.Identifier), ""))
	}
	if c.Currency != nil {
		a.Currency = strings.ToUpper(strings.TrimSpace(*c.Currency))
	}
	if c.Active != nil {
		a.Active = *c.Active
	}
	var verr resource.ValidationError
	if !slices.Contains(AccountTypes, a.Type) {
		verr.Add("type", "is not included in the list")
	}
	if a.Name == "" || len(a.Name) > 255 {
		verr.Add("name", "can't be blank")
	}
	if len(a.Identifier) > 64 {
		verr.Add("identifier", "is too long (maximum is 64 characters)")
	}
	if ibanShape.MatchString(a.Identifier) && !iban.Valid(a.Identifier) {
		verr.Add("identifier", "is not a valid IBAN")
	}
	if !currencyPattern.MatchString(a.Currency) {
		verr.Add("currency", "is not a currency code")
	}
	for _, e := range existing {
		if e.ID != a.ID && strings.EqualFold(e.Name, a.Name) {
			verr.Add("name", "has already been taken")
		}
		if e.ID != a.ID && a.Identifier != "" && e.Identifier == a.Identifier {
			verr.Add("identifier", "has already been taken")
		}
	}
	return verr.OrNil()
}
