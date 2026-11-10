// Package bookkeeping holds an administration's bookkeeping setup: VAT
// declaration periods, VAT rates, categories (ledger accounts) and the
// locked period. A new administration starts from its country's seed.
package bookkeeping

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/adirkuhn/moneyharpy/internal/administration"
	"github.com/adirkuhn/moneyharpy/internal/bookkeeping/countries"
	"github.com/adirkuhn/moneyharpy/internal/resource"
	"github.com/adirkuhn/moneyharpy/internal/taxonomy"
)

// Errors returned by Service.
var (
	ErrTaxRateNotFound       = errors.New("tax rate not found")
	ErrLedgerAccountNotFound = errors.New("ledger account not found")
	ErrTemplateNotFound      = errors.New("template not found")
	// ErrSystemAccount reports a change the books need to stay as they are.
	ErrSystemAccount = errors.New("system ledger accounts cannot be deleted or change type")
	// ErrHasChildren reports a category with active sub-categories.
	ErrHasChildren = errors.New("ledger account has active child ledger accounts")
	// ErrInUse reports a category that bookings, rules or financial accounts
	// still use.
	ErrInUse = errors.New("ledger account is in use")
)

// Periods a declaration can cover.
var Periods = []string{"month", "quarter", "year"}

// Administrations gives access to administrations; Get returns
// administration.ErrNotFound when the user has no access.
type Administrations interface {
	Get(ctx context.Context, id, user resource.ID) (administration.Listed, error)
}

// Settings are an administration's bookkeeping preferences.
type Settings struct {
	VATPeriod string
	ICPPeriod string
	// Scheme is the chart scheme categories map to, or empty for none.
	Scheme string
	// LockedUntil closes the books up to and including that date (YYYY-MM-DD).
	LockedUntil *string
	// Seeded reports that the country's seed has been applied.
	Seeded bool
}

// SettingsChanges holds the settings to set. Nil fields stay as they are;
// an empty LockedUntil removes the lock.
type SettingsChanges struct {
	VATPeriod   *string
	ICPPeriod   *string
	Scheme      *string
	LockedUntil *string
}

// Store persists bookkeeping data. Methods taking an administration trust
// the caller to have checked access.
type Store interface {
	Settings(ctx context.Context, admin resource.ID) (Settings, error)
	SaveSettings(ctx context.Context, admin resource.ID, s Settings) error
	// ClearTaxonomy removes every category's chart scheme code.
	ClearTaxonomy(ctx context.Context, admin resource.ID) error
	// Seed marks the administration seeded and stores rates and accounts, in
	// one transaction. It does nothing and returns false when the
	// administration was seeded already.
	Seed(ctx context.Context, admin resource.ID, scheme string, rates []TaxRate, accounts []LedgerAccount) (bool, error)

	TaxRates(ctx context.Context, admin resource.ID) ([]TaxRate, error)
	TaxRate(ctx context.Context, admin, id resource.ID) (TaxRate, error)
	CreateTaxRate(ctx context.Context, r TaxRate) error
	UpdateTaxRates(ctx context.Context, rates []TaxRate) error

	LedgerAccounts(ctx context.Context, admin resource.ID) ([]LedgerAccount, error)
	LedgerAccount(ctx context.Context, admin, id resource.ID) (LedgerAccount, error)
	CreateLedgerAccounts(ctx context.Context, accounts []LedgerAccount) error
	UpdateLedgerAccounts(ctx context.Context, accounts []LedgerAccount) error
	DeleteLedgerAccount(ctx context.Context, admin, id resource.ID) error
}

// Service implements the bookkeeping use cases.
type Service struct {
	admins Administrations
	store  Store
	ids    *resource.Generator
	now    func() time.Time
}

// NewService returns a Service.
func NewService(admins Administrations, store Store, ids *resource.Generator, now func() time.Time) *Service {
	return &Service{admins: admins, store: store, ids: ids, now: now}
}

// Settings returns the administration's bookkeeping settings.
func (s *Service) Settings(ctx context.Context, admin, user resource.ID) (Settings, error) {
	if _, err := s.ensure(ctx, admin, user); err != nil {
		return Settings{}, err
	}
	return s.store.Settings(ctx, admin)
}

// UpdateSettings changes the bookkeeping settings. Choosing another chart
// scheme clears the codes categories had in the previous one.
func (s *Service) UpdateSettings(ctx context.Context, admin, user resource.ID, c SettingsChanges) (Settings, error) {
	a, err := s.ensure(ctx, admin, user)
	if err != nil {
		return Settings{}, err
	}
	st, err := s.store.Settings(ctx, admin)
	if err != nil {
		return Settings{}, fmt.Errorf("load settings: %w", err)
	}
	var verr resource.ValidationError
	if c.VATPeriod != nil {
		st.VATPeriod = *c.VATPeriod
	}
	if c.ICPPeriod != nil {
		st.ICPPeriod = *c.ICPPeriod
	}
	if !slices.Contains(Periods, st.VATPeriod) {
		verr.Add("vat_documents_period", "is not included in the list")
	}
	if !slices.Contains(Periods, st.ICPPeriod) {
		verr.Add("intra_community_transactions_declaration_period", "is not included in the list")
	}
	schemeChanged := false
	if c.Scheme != nil && *c.Scheme != st.Scheme {
		if _, ok := taxonomy.Lookup(*c.Scheme); *c.Scheme != "" && !ok {
			verr.Add("chart_scheme", "is not included in the list")
		}
		st.Scheme, schemeChanged = *c.Scheme, true
	}
	if c.LockedUntil != nil {
		if *c.LockedUntil == "" {
			st.LockedUntil = nil
		} else if day, err := time.Parse(time.DateOnly, *c.LockedUntil); err != nil {
			verr.Add("period_locked_until", "must be a date")
		} else if day.After(s.today(a.Administration)) {
			verr.Add("period_locked_until", "can't be in the future")
		} else {
			st.LockedUntil = c.LockedUntil
		}
	}
	if err := verr.OrNil(); err != nil {
		return Settings{}, err
	}
	if schemeChanged {
		if err := s.store.ClearTaxonomy(ctx, admin); err != nil {
			return Settings{}, fmt.Errorf("clear chart codes: %w", err)
		}
	}
	if err := s.store.SaveSettings(ctx, admin, st); err != nil {
		return Settings{}, fmt.Errorf("save settings: %w", err)
	}
	return st, nil
}

func (s *Service) today(a administration.Administration) time.Time {
	now := s.now()
	if loc, err := time.LoadLocation(a.TimeZone); err == nil {
		now = now.In(loc)
	}
	y, m, d := now.Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

// Box is a box of the VAT return, named in the administration's language.
type Box struct{ Code, Name string }

// Boxes returns the boxes of the administration's VAT return.
func (s *Service) Boxes(ctx context.Context, admin, user resource.ID) ([]Box, error) {
	a, err := s.admins.Get(ctx, admin, user)
	if err != nil {
		return nil, err
	}
	boxes := countries.For(a.Country).Boxes
	out := make([]Box, len(boxes))
	for i, b := range boxes {
		out[i] = Box{Code: b.Code, Name: b.Name.In(a.Language)}
	}
	return out, nil
}

// Scheme returns the administration's chart scheme, if it has one.
func (s *Service) Scheme(ctx context.Context, admin, user resource.ID) (*taxonomy.Scheme, error) {
	st, err := s.Settings(ctx, admin, user)
	if err != nil {
		return nil, err
	}
	scheme, ok := taxonomy.Lookup(st.Scheme)
	if !ok {
		return nil, nil
	}
	return scheme, nil
}

// ensure checks access and applies the country's seed on first use.
func (s *Service) ensure(ctx context.Context, admin, user resource.ID) (administration.Listed, error) {
	a, err := s.admins.Get(ctx, admin, user)
	if err != nil {
		return administration.Listed{}, err
	}
	st, err := s.store.Settings(ctx, admin)
	if err != nil {
		return administration.Listed{}, fmt.Errorf("load settings: %w", err)
	}
	if st.Seeded {
		return a, nil
	}
	seed := countries.For(a.Country)
	rates, accounts := s.fromSeed(a.Administration, seed)
	if _, err := s.store.Seed(ctx, admin, seed.Scheme, rates, accounts); err != nil {
		return administration.Listed{}, fmt.Errorf("seed bookkeeping: %w", err)
	}
	return a, nil
}

func (s *Service) fromSeed(a administration.Administration, seed countries.Seed) ([]TaxRate, []LedgerAccount) {
	now := s.now()
	rates := make([]TaxRate, len(seed.TaxRates))
	position := map[string]int{}
	for i, r := range seed.TaxRates {
		position[r.Type]++
		rates[i] = TaxRate{
			ID: s.ids.Next(), AdministrationID: a.ID, Name: r.Name.In(a.Language), Percentage: r.Percentage, Type: r.Type,
			ShowTax: r.ShowTax, Active: true, ReportBoxes: slices.Clone(r.Boxes), Position: position[r.Type],
			CreatedAt: now, UpdatedAt: now,
		}
	}
	return rates, s.accountsFrom(a, seed.Accounts, nil, "")
}

// accountsFrom turns seed accounts into categories. Parents are found among
// the new accounts first and then among existing ones, by key.
func (s *Service) accountsFrom(a administration.Administration, in []countries.Account, existing []LedgerAccount, template string) []LedgerAccount {
	now := s.now()
	byKey := map[string]resource.ID{}
	for _, e := range existing {
		if e.Key != "" {
			byKey[e.Key] = e.ID
		}
	}
	out := make([]LedgerAccount, 0, len(in))
	for _, sa := range in {
		id := s.ids.Next()
		byKey[sa.Key] = id
		la := LedgerAccount{
			ID: id, AdministrationID: a.ID, Name: sa.Name.In(a.Language), Type: sa.Type, Code: sa.Code, Active: true,
			AllowedDocumentTypes: DefaultDocumentTypes(sa.Type), Taxonomy: sa.Taxonomy, Description: sa.Description.In(a.Language),
			System: sa.System, Key: sa.Key, Template: template, CreatedAt: now, UpdatedAt: now,
		}
		if sa.Parent != "" {
			if p, ok := byKey[sa.Parent]; ok {
				la.ParentID = &p
			}
		}
		out = append(out, la)
	}
	return out
}
