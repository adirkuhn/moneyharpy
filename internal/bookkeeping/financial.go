package bookkeeping

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/adirkuhn/moneyharpy/internal/bookkeeping/countries"
	"github.com/adirkuhn/moneyharpy/internal/resource"
)

// FinancialRole marks a category that holds a financial account's money. It
// stays as long as the account does.
const FinancialRole = "financial_account"

// FinancialLedgerAccount adds the category a financial account's money is
// kept in, under the financial accounts category. A taken name gets a
// number.
func (s *Service) FinancialLedgerAccount(ctx context.Context, admin, user resource.ID, name, accountType string) (resource.ID, error) {
	a, err := s.ensure(ctx, admin, user)
	if err != nil {
		return 0, err
	}
	all, err := s.store.LedgerAccounts(ctx, admin)
	if err != nil {
		return 0, fmt.Errorf("list ledger accounts: %w", err)
	}
	st, err := s.store.Settings(ctx, admin)
	if err != nil {
		return 0, fmt.Errorf("load settings: %w", err)
	}
	now := s.now()
	l := LedgerAccount{
		ID: s.ids.Next(), AdministrationID: admin, Name: name, Type: "current_assets", Active: true,
		AllowedDocumentTypes: []string{"general_journal_document", "financial_mutation"}, System: FinancialRole, CreatedAt: now, UpdatedAt: now,
	}
	if i := slices.IndexFunc(all, func(o LedgerAccount) bool { return o.System == "financial_accounts" }); i >= 0 {
		l.ParentID = &all[i].ID
	}
	seed := countries.For(a.Country)
	if st.Scheme != "" && st.Scheme == seed.Scheme {
		l.Taxonomy = seed.FinancialAccounts[accountType]
	}
	taken := func(n string) bool {
		return slices.ContainsFunc(all, func(o LedgerAccount) bool { return o.Type == l.Type && strings.EqualFold(o.Name, n) })
	}
	for i := 2; taken(l.Name); i++ {
		l.Name = name + " (" + strconv.Itoa(i) + ")"
	}
	if err := s.store.CreateLedgerAccounts(ctx, []LedgerAccount{l}); err != nil {
		return 0, fmt.Errorf("store ledger account: %w", err)
	}
	return l.ID, nil
}

// SeedLedgerAccount returns the category created from a seed key, if the
// administration has it.
func (s *Service) SeedLedgerAccount(ctx context.Context, admin, user resource.ID, key string) (resource.ID, bool, error) {
	if _, err := s.ensure(ctx, admin, user); err != nil {
		return 0, false, err
	}
	all, err := s.store.LedgerAccounts(ctx, admin)
	if err != nil {
		return 0, false, fmt.Errorf("list ledger accounts: %w", err)
	}
	if i := slices.IndexFunc(all, func(l LedgerAccount) bool { return l.Key == key }); i >= 0 {
		return all[i].ID, true, nil
	}
	return 0, false, nil
}

// SystemLedgerAccount returns the category that plays a role the books rely
// on, such as accounts_receivable or payable_vat. An administration seeded
// before the role existed gets it from its country's seed now.
func (s *Service) SystemLedgerAccount(ctx context.Context, admin, user resource.ID, role string) (resource.ID, error) {
	a, err := s.ensure(ctx, admin, user)
	if err != nil {
		return 0, err
	}
	all, err := s.store.LedgerAccounts(ctx, admin)
	if err != nil {
		return 0, fmt.Errorf("list ledger accounts: %w", err)
	}
	if i := slices.IndexFunc(all, func(l LedgerAccount) bool { return l.System == role }); i >= 0 {
		return all[i].ID, nil
	}
	seed := countries.For(a.Country)
	i := slices.IndexFunc(seed.Accounts, func(sa countries.Account) bool { return sa.System == role })
	if i < 0 {
		return 0, fmt.Errorf("no category for role %s", role)
	}
	added := s.accountsFrom(a.Administration, seed.Accounts[i:i+1], all, "")
	st, err := s.store.Settings(ctx, admin)
	if err != nil {
		return 0, fmt.Errorf("load settings: %w", err)
	}
	if st.Scheme != seed.Scheme {
		added[0].Taxonomy = ""
	}
	if err := s.store.CreateLedgerAccounts(ctx, added); err != nil {
		return 0, fmt.Errorf("store ledger account: %w", err)
	}
	return added[0].ID, nil
}

// CheckBookable reports, as a validation error, why transactions cannot be
// booked on a category.
func (s *Service) CheckBookable(ctx context.Context, admin, user, id resource.ID) error {
	problem, err := s.Bookable(ctx, admin, user, id, "financial_mutation")
	if err != nil || problem == "" {
		return err
	}
	var verr resource.ValidationError
	verr.Add("booking_id", problem)
	return &verr
}

// Bookable returns why the category cannot be used on a document of the
// type, or "" when it can. Journal entries cannot book on the debtors and
// creditors, which only documents settle.
func (s *Service) Bookable(ctx context.Context, admin, user, id resource.ID, documentType string) (string, error) {
	if _, err := s.ensure(ctx, admin, user); err != nil {
		return "", err
	}
	l, err := s.store.LedgerAccount(ctx, admin, id)
	switch {
	case errors.Is(err, ErrLedgerAccountNotFound):
		return "is not a category of this administration", nil
	case err != nil:
		return "", err
	case !l.Active:
		return "is a deactivated category", nil
	case documentType == "general_journal_document" && (l.System == "accounts_receivable" || l.System == "accounts_payable"):
		return "is settled by invoices and cannot be booked in a journal entry", nil
	case !slices.Contains(l.AllowedDocumentTypes, documentType):
		return documentUse[documentType], nil
	}
	return "", nil
}

var documentUse = map[string]string{
	"financial_mutation":       "is a category that cannot be used on bank transactions",
	"general_journal_document": "is a category that cannot be used in journal entries",
	"sales_invoice":            "is a category that cannot be used on sales invoices",
	"purchase_invoice":         "is a category that cannot be used on purchase invoices",
	"payment":                  "is a category that cannot be used on payments",
}

// LockedUntil returns the last day of the locked period, or "".
func (s *Service) LockedUntil(ctx context.Context, admin, user resource.ID) (string, error) {
	st, err := s.Settings(ctx, admin, user)
	if err != nil || st.LockedUntil == nil {
		return "", err
	}
	return *st.LockedUntil, nil
}
