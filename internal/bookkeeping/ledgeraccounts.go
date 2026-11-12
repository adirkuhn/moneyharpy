package bookkeeping

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/adirkuhn/moneyharpy/internal/resource"
	"github.com/adirkuhn/moneyharpy/internal/taxonomy"
)

// Account types, in the order the books show them. The first six are on
// the balance sheet, the rest on the profit and loss account.
var (
	BalanceSheetTypes  = []string{"non_current_assets", "current_assets", "equity", "provisions", "non_current_liabilities", "current_liabilities"}
	ProfitAndLossTypes = []string{"revenue", "direct_costs", "expenses", "other_income_expenses"}
	AccountTypes       = slices.Concat(BalanceSheetTypes, ProfitAndLossTypes)
)

// DocumentTypes are the documents a category can be used on.
var DocumentTypes = []string{"sales_invoice", "purchase_invoice", "general_journal_document", "financial_mutation", "payment"}

// DefaultDocumentTypes returns the documents a new category of the type can
// be used on.
func DefaultDocumentTypes(accountType string) []string {
	switch accountType {
	case "revenue":
		return []string{"sales_invoice", "general_journal_document", "financial_mutation"}
	case "direct_costs", "expenses":
		return []string{"purchase_invoice", "general_journal_document", "financial_mutation"}
	case "other_income_expenses":
		return []string{"sales_invoice", "purchase_invoice", "general_journal_document", "financial_mutation"}
	default:
		return []string{"purchase_invoice", "general_journal_document", "financial_mutation"}
	}
}

// SideOf returns the side of the books an account type is on.
func SideOf(accountType string) taxonomy.Side {
	if slices.Contains(BalanceSheetTypes, accountType) {
		return taxonomy.BalanceSheet
	}
	if slices.Contains(ProfitAndLossTypes, accountType) {
		return taxonomy.ProfitAndLoss
	}
	return taxonomy.AnySide
}

// LedgerAccount is a category the books are kept in.
type LedgerAccount struct {
	ID               resource.ID
	AdministrationID resource.ID
	Name             string
	Type             string
	// Code is the account number people know the category by, such as 8000.
	Code                 string
	ParentID             *resource.ID
	Active               bool
	AllowedDocumentTypes []string
	// Taxonomy is the code in the administration's chart scheme.
	Taxonomy    string
	Description string
	// System names the role of a category the books rely on.
	System string
	// Key is the seed key the category was created from, if any.
	Key string
	// Template is the template the category was added with, if any.
	Template  string
	CreatedAt time.Time
	UpdatedAt time.Time
}

// Ref locates the ledger account.
func (l LedgerAccount) Ref() resource.Ref {
	return resource.Ref{ID: l.ID, Type: resource.TypeLedgerAccount, AdministrationID: l.AdministrationID}
}

// LedgerAccountChanges holds the fields to set. Nil fields stay as they are;
// a ParentID of 0 removes the parent.
type LedgerAccountChanges struct {
	Name                 *string
	Type                 *string
	Code                 *string
	ParentID             *resource.ID
	Active               *bool
	AllowedDocumentTypes *[]string
	Taxonomy             *string
	Description          *string
}

// LedgerAccounts lists the administration's categories by type, code and
// name.
func (s *Service) LedgerAccounts(ctx context.Context, admin, user resource.ID) ([]LedgerAccount, error) {
	if _, err := s.ensure(ctx, admin, user); err != nil {
		return nil, err
	}
	return s.ledgerAccounts(ctx, admin)
}

func (s *Service) ledgerAccounts(ctx context.Context, admin resource.ID) ([]LedgerAccount, error) {
	accounts, err := s.store.LedgerAccounts(ctx, admin)
	if err != nil {
		return nil, fmt.Errorf("list ledger accounts: %w", err)
	}
	slices.SortStableFunc(accounts, func(a, b LedgerAccount) int {
		return cmp.Or(
			cmp.Compare(slices.Index(AccountTypes, a.Type), slices.Index(AccountTypes, b.Type)),
			cmp.Compare(blankLast(a.Code), blankLast(b.Code)), cmp.Compare(a.Code, b.Code),
			cmp.Compare(strings.ToLower(a.Name), strings.ToLower(b.Name)), cmp.Compare(a.ID, b.ID),
		)
	})
	return accounts, nil
}

// LedgerAccount returns one category.
func (s *Service) LedgerAccount(ctx context.Context, admin, id, user resource.ID) (LedgerAccount, error) {
	if _, err := s.ensure(ctx, admin, user); err != nil {
		return LedgerAccount{}, err
	}
	return s.store.LedgerAccount(ctx, admin, id)
}

// CreateLedgerAccount adds a category. With a chart scheme set, it needs a
// code in that scheme.
func (s *Service) CreateLedgerAccount(ctx context.Context, admin, user resource.ID, c LedgerAccountChanges) (LedgerAccount, error) {
	if _, err := s.ensure(ctx, admin, user); err != nil {
		return LedgerAccount{}, err
	}
	now := s.now()
	l := LedgerAccount{ID: s.ids.Next(), AdministrationID: admin, Active: true, CreatedAt: now, UpdatedAt: now}
	if c.AllowedDocumentTypes == nil && c.Type != nil {
		l.AllowedDocumentTypes = DefaultDocumentTypes(*c.Type)
	}
	if err := s.applyLedgerAccount(ctx, &l, c, true); err != nil {
		return LedgerAccount{}, err
	}
	if err := s.store.CreateLedgerAccounts(ctx, []LedgerAccount{l}); err != nil {
		return LedgerAccount{}, fmt.Errorf("store ledger account: %w", err)
	}
	return l, nil
}

// UpdateLedgerAccount changes a category.
func (s *Service) UpdateLedgerAccount(ctx context.Context, admin, id, user resource.ID, c LedgerAccountChanges) (LedgerAccount, error) {
	if _, err := s.ensure(ctx, admin, user); err != nil {
		return LedgerAccount{}, err
	}
	l, err := s.store.LedgerAccount(ctx, admin, id)
	if err != nil {
		return LedgerAccount{}, err
	}
	if l.System != "" && ((c.Type != nil && *c.Type != l.Type) || (c.Active != nil && !*c.Active)) {
		return LedgerAccount{}, ErrSystemAccount
	}
	if err := s.applyLedgerAccount(ctx, &l, c, false); err != nil {
		return LedgerAccount{}, err
	}
	l.UpdatedAt = s.now()
	if err := s.store.UpdateLedgerAccounts(ctx, []LedgerAccount{l}); err != nil {
		return LedgerAccount{}, fmt.Errorf("store ledger account: %w", err)
	}
	return l, nil
}

// DeleteLedgerAccount deactivates an active category and deletes an inactive
// one, as Moneybird does. It returns the category as it is now, with ok
// false once deleted.
func (s *Service) DeleteLedgerAccount(ctx context.Context, admin, id, user resource.ID) (l LedgerAccount, ok bool, err error) {
	if _, err := s.ensure(ctx, admin, user); err != nil {
		return LedgerAccount{}, false, err
	}
	l, err = s.store.LedgerAccount(ctx, admin, id)
	if err != nil {
		return LedgerAccount{}, false, err
	}
	if l.System != "" {
		return LedgerAccount{}, false, ErrSystemAccount
	}
	all, err := s.store.LedgerAccounts(ctx, admin)
	if err != nil {
		return LedgerAccount{}, false, fmt.Errorf("list ledger accounts: %w", err)
	}
	for _, c := range all {
		if c.ParentID != nil && *c.ParentID == id && (c.Active || !l.Active) {
			return LedgerAccount{}, false, ErrHasChildren
		}
	}
	if !l.Active {
		if err := s.store.DeleteLedgerAccount(ctx, admin, id); err != nil {
			return LedgerAccount{}, false, fmt.Errorf("delete ledger account: %w", err)
		}
		return l, false, nil
	}
	l.Active, l.UpdatedAt = false, s.now()
	if err := s.store.UpdateLedgerAccounts(ctx, []LedgerAccount{l}); err != nil {
		return LedgerAccount{}, false, fmt.Errorf("store ledger account: %w", err)
	}
	return l, true, nil
}

// UpdateScheme sets the chart scheme codes of several categories at once,
// keyed by category. An empty code removes it.
func (s *Service) UpdateScheme(ctx context.Context, admin, user resource.ID, codes map[resource.ID]string) ([]LedgerAccount, error) {
	if _, err := s.ensure(ctx, admin, user); err != nil {
		return nil, err
	}
	st, err := s.store.Settings(ctx, admin)
	if err != nil {
		return nil, fmt.Errorf("load settings: %w", err)
	}
	scheme, ok := taxonomy.Lookup(st.Scheme)
	if !ok {
		var verr resource.ValidationError
		verr.Add("chart_scheme", "must be chosen before mapping categories")
		return nil, &verr
	}
	all, err := s.ledgerAccounts(ctx, admin)
	if err != nil {
		return nil, err
	}
	var verr resource.ValidationError
	var changed []LedgerAccount
	for i, l := range all {
		code, ok := codes[l.ID]
		if !ok {
			continue
		}
		delete(codes, l.ID)
		code = strings.TrimSpace(code)
		if code == l.Taxonomy {
			continue
		}
		if code != "" {
			if problem := checkTaxonomy(scheme, code, l.Type); problem != "" {
				verr.Add("ledger_accounts."+l.ID.String(), problem)
				continue
			}
		}
		all[i].Taxonomy, all[i].UpdatedAt = code, s.now()
		changed = append(changed, all[i])
	}
	for id := range codes {
		verr.Add("ledger_accounts."+id.String(), "does not exist")
	}
	if err := verr.OrNil(); err != nil {
		return nil, err
	}
	if len(changed) > 0 {
		if err := s.store.UpdateLedgerAccounts(ctx, changed); err != nil {
			return nil, fmt.Errorf("store ledger accounts: %w", err)
		}
	}
	return all, nil
}

func checkTaxonomy(scheme *taxonomy.Scheme, code, accountType string) string {
	item, ok := scheme.Item(code)
	if !ok {
		return "is not a code of " + scheme.Name + " " + scheme.Version
	}
	if side := SideOf(accountType); side != taxonomy.AnySide && item.Side() != side {
		if side == taxonomy.BalanceSheet {
			return "must be a balance sheet code"
		}
		return "must be a profit and loss code"
	}
	return ""
}

func (s *Service) applyLedgerAccount(ctx context.Context, l *LedgerAccount, c LedgerAccountChanges, creating bool) error {
	if c.Name != nil {
		l.Name = strings.TrimSpace(*c.Name)
	}
	if c.Type != nil {
		l.Type = *c.Type
	}
	if c.Code != nil {
		l.Code = strings.TrimSpace(*c.Code)
	}
	if c.ParentID != nil {
		if *c.ParentID == 0 {
			l.ParentID = nil
		} else {
			p := *c.ParentID
			l.ParentID = &p
		}
	}
	if c.Active != nil {
		l.Active = *c.Active
	}
	if c.AllowedDocumentTypes != nil {
		l.AllowedDocumentTypes = slices.Compact(slices.Sorted(slices.Values(*c.AllowedDocumentTypes)))
	}
	if c.Taxonomy != nil {
		l.Taxonomy = strings.TrimSpace(*c.Taxonomy)
	}
	if c.Description != nil {
		l.Description = strings.TrimSpace(*c.Description)
	}

	var verr resource.ValidationError
	if l.Name == "" || len(l.Name) > 255 {
		verr.Add("name", "can't be blank")
	}
	if !slices.Contains(AccountTypes, l.Type) {
		verr.Add("account_type", "is not included in the list")
	}
	if len(l.Code) > 20 {
		verr.Add("account_id", "is too long (maximum is 20 characters)")
	}
	if len(l.Description) > 1000 {
		verr.Add("description", "is too long (maximum is 1000 characters)")
	}
	for _, d := range l.AllowedDocumentTypes {
		if !slices.Contains(DocumentTypes, d) {
			verr.Add("allowed_document_types", d+" is not included in the list")
		}
	}

	all, err := s.store.LedgerAccounts(ctx, l.AdministrationID)
	if err != nil {
		return fmt.Errorf("list ledger accounts: %w", err)
	}
	byID := map[resource.ID]LedgerAccount{}
	for _, o := range all {
		byID[o.ID] = o
		if o.ID != l.ID && o.Type == l.Type && strings.EqualFold(o.Name, l.Name) {
			verr.Add("name", "has already been taken")
		}
	}
	if l.ParentID != nil {
		if p, ok := byID[*l.ParentID]; !ok {
			verr.Add("parent_id", "does not exist")
		} else if p.Type != l.Type {
			verr.Add("parent_id", "must have the same account type")
		} else {
			for at, seen := p, 0; ; seen++ {
				if at.ID == l.ID || seen > len(all) {
					verr.Add("parent_id", "can't be the category itself or one of its sub-categories")
					break
				}
				if at.ParentID == nil {
					break
				}
				at = byID[*at.ParentID]
			}
		}
	}

	st, err := s.store.Settings(ctx, l.AdministrationID)
	if err != nil {
		return fmt.Errorf("load settings: %w", err)
	}
	if scheme, ok := taxonomy.Lookup(st.Scheme); ok {
		switch {
		case l.Taxonomy == "" && (creating || c.Taxonomy != nil):
			verr.Add("rgs_code", "can't be blank")
		case l.Taxonomy != "" && (creating || c.Taxonomy != nil || c.Type != nil):
			if problem := checkTaxonomy(scheme, l.Taxonomy, l.Type); problem != "" {
				verr.Add("rgs_code", problem)
			}
		}
	} else if c.Taxonomy != nil && l.Taxonomy != "" {
		verr.Add("rgs_code", "needs a chart scheme in the bookkeeping settings")
	}
	return verr.OrNil()
}

// blankLast sorts empty codes after the others.
func blankLast(code string) int {
	if code == "" {
		return 1
	}
	return 0
}

func keysOf(accounts []LedgerAccount) map[string]bool {
	keys := map[string]bool{}
	for _, l := range accounts {
		if l.Key != "" {
			keys[l.Key] = true
		}
	}
	return keys
}

// IsNotFound reports whether err means a bookkeeping resource is missing.
func IsNotFound(err error) bool {
	return errors.Is(err, ErrTaxRateNotFound) || errors.Is(err, ErrLedgerAccountNotFound) || errors.Is(err, ErrTemplateNotFound)
}
