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

// CheckBookable reports, as a validation error, why transactions cannot be
// booked on a category.
func (s *Service) CheckBookable(ctx context.Context, admin, user, id resource.ID) error {
	if _, err := s.ensure(ctx, admin, user); err != nil {
		return err
	}
	var verr resource.ValidationError
	l, err := s.store.LedgerAccount(ctx, admin, id)
	switch {
	case errors.Is(err, ErrLedgerAccountNotFound):
		verr.Add("booking_id", "is not a category of this administration")
	case err != nil:
		return err
	case !l.Active:
		verr.Add("booking_id", "is a deactivated category")
	case !slices.Contains(l.AllowedDocumentTypes, "financial_mutation"):
		verr.Add("booking_id", "is a category that cannot be used on bank transactions")
	}
	return verr.OrNil()
}

// LockedUntil returns the last day of the locked period, or "".
func (s *Service) LockedUntil(ctx context.Context, admin, user resource.ID) (string, error) {
	st, err := s.Settings(ctx, admin, user)
	if err != nil || st.LockedUntil == nil {
		return "", err
	}
	return *st.LockedUntil, nil
}
