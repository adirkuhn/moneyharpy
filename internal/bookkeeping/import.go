package bookkeeping

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/adirkuhn/moneyharpy/internal/bookkeeping/countries"
	"github.com/adirkuhn/moneyharpy/internal/resource"
)

// ImportedAccount is a category brought in from another system, such as
// Moneybird. ParentID refers to the parent's id in that system.
type ImportedAccount struct {
	Origin               resource.Origin
	Name, Type, Code     string
	ParentID             *resource.ID
	Active               bool
	AllowedDocumentTypes []string
	Taxonomy             string
	Description          string
}

// ImportedRate is a VAT rate brought in from another system.
type ImportedRate struct {
	Origin                          resource.Origin
	Name, Percentage, Type, Country string
	ShowTax, Active                 bool
}

// ChartImport is the chart of accounts and VAT rates of another system.
type ChartImport struct {
	Accounts []ImportedAccount
	Rates    []ImportedRate
}

// ChartIDs maps the ids of imported categories and VAT rates to the ids
// they have here: the same id, unless a default took their place.
type ChartIDs map[resource.ID]resource.ID

// ImportChart brings in another system's categories and VAT rates in place
// of the administration's defaults.
//
// The books rely on some default categories, such as accounts receivable,
// and the VAT return on the boxes of the default rates. So an imported
// category or rate that clearly is a default takes that default over: it
// keeps the default's id and role and takes the imported name and code.
// Clearly means one candidate on each side, by chart scheme code for
// categories and by type, percentage, country and printing for rates, with
// the name deciding between several. Everything else comes in with its own
// id. Default categories nothing took over are removed unless the books
// rely on them; such rates are turned off. Running it again changes nothing
// more.
func (s *Service) ImportChart(ctx context.Context, admin, user resource.ID, in ChartImport) (ChartIDs, error) {
	a, err := s.ensure(ctx, admin, user)
	if err != nil {
		return nil, err
	}
	seed := countries.For(a.Country)
	ids := ChartIDs{}
	if err := s.importAccounts(ctx, admin, in.Accounts, seed, ids); err != nil {
		return nil, err
	}
	if err := s.importRates(ctx, admin, in.Rates, seed, ids); err != nil {
		return nil, err
	}
	return ids, nil
}

func (s *Service) importAccounts(ctx context.Context, admin resource.ID, in []ImportedAccount, seed countries.Seed, ids ChartIDs) error {
	existing, err := s.store.LedgerAccounts(ctx, admin)
	if err != nil {
		return fmt.Errorf("list ledger accounts: %w", err)
	}
	byID := map[resource.ID]LedgerAccount{}
	for _, l := range existing {
		byID[l.ID] = l
	}
	names := map[string][]string{}
	for _, sa := range seed.Accounts {
		names[sa.Key] = []string{sa.Name.EN, sa.Name.NL}
	}
	// Defaults are the categories made from the seed, by their key.
	defaults := slices.DeleteFunc(slices.Clone(existing), func(l LedgerAccount) bool { return l.Key == "" })
	taken := map[resource.ID]bool{}
	match := func(ia ImportedAccount) (LedgerAccount, bool) {
		if ia.Taxonomy == "" {
			return LedgerAccount{}, false
		}
		same := func(t string) bool { return strings.EqualFold(t, ia.Taxonomy) }
		ours := slices.DeleteFunc(slices.Clone(defaults), func(l LedgerAccount) bool { return taken[l.ID] || !same(l.Taxonomy) })
		theirs := slices.DeleteFunc(slices.Clone(in), func(o ImportedAccount) bool { return !same(o.Taxonomy) })
		if len(ours) == 1 && len(theirs) == 1 {
			return ours[0], true
		}
		return byName(ours, ia.Name, func(l LedgerAccount) []string { return append(names[l.Key], l.Name) })
	}

	now := s.now()
	var created, changed []LedgerAccount
	for _, ia := range in {
		if _, ok := byID[ia.Origin.ID]; ok {
			ids[ia.Origin.ID] = ia.Origin.ID
			continue
		}
		if !slices.Contains(AccountTypes, ia.Type) {
			// Moneybird's own bookkeeping accounts, such as temporary ones, have no place here.
			continue
		}
		if l, ok := match(ia); ok {
			taken[l.ID] = true
			ids[ia.Origin.ID] = l.ID
			l.Name, l.Code, l.Description, l.UpdatedAt = ia.Name, ia.Code, ia.Description, now
			if l.System == "" {
				l.Active = ia.Active
			}
			if len(ia.AllowedDocumentTypes) > 0 {
				l.AllowedDocumentTypes = slices.Clone(ia.AllowedDocumentTypes)
			}
			changed = append(changed, l)
			continue
		}
		createdAt, updatedAt := ia.Origin.Or(now)
		types := slices.Clone(ia.AllowedDocumentTypes)
		if len(types) == 0 {
			types = DefaultDocumentTypes(ia.Type)
		}
		ids[ia.Origin.ID] = ia.Origin.ID
		created = append(created, LedgerAccount{
			ID: ia.Origin.ID, AdministrationID: admin, Name: ia.Name, Type: ia.Type, Code: ia.Code, Active: ia.Active,
			AllowedDocumentTypes: types, Taxonomy: ia.Taxonomy, Description: ia.Description, CreatedAt: createdAt, UpdatedAt: updatedAt,
		})
	}
	// Parents point to ids here once every category has one.
	parent := func(p *resource.ID) *resource.ID {
		if p == nil {
			return nil
		}
		if id, ok := ids[*p]; ok {
			return &id
		}
		return nil
	}
	sources := map[resource.ID]*resource.ID{}
	for _, ia := range in {
		sources[ids[ia.Origin.ID]] = ia.ParentID
	}
	for i := range created {
		created[i].ParentID = parent(sources[created[i].ID])
	}
	for i := range changed {
		if p := parent(sources[changed[i].ID]); p != nil {
			changed[i].ParentID = p
		}
	}
	if len(created) > 0 {
		// Parents first: a category may only point to one that exists.
		slices.SortStableFunc(created, func(a, b LedgerAccount) int { return depth(created, a) - depth(created, b) })
		if err := s.store.CreateLedgerAccounts(ctx, created); err != nil {
			return fmt.Errorf("store imported ledger accounts: %w", err)
		}
	}
	if len(changed) > 0 {
		if err := s.store.UpdateLedgerAccounts(ctx, changed); err != nil {
			return fmt.Errorf("store taken over ledger accounts: %w", err)
		}
	}
	if len(in) == 0 {
		return nil
	}
	// Defaults nothing took over go, children first, unless the books rely on them or they are in use.
	var left []LedgerAccount
	for _, l := range defaults {
		if !taken[l.ID] && l.System == "" && !slices.ContainsFunc(changed, func(c LedgerAccount) bool { return c.ID == l.ID }) && !importedAgain(l, ids) {
			left = append(left, l)
		}
	}
	slices.SortStableFunc(left, func(a, b LedgerAccount) int { return depth(existing, b) - depth(existing, a) })
	for _, l := range left {
		if hasChildren(ctx, s, admin, l.ID) {
			continue
		}
		if err := s.store.DeleteLedgerAccount(ctx, admin, l.ID); err != nil && !errors.Is(err, ErrInUse) {
			return fmt.Errorf("remove default ledger account: %w", err)
		}
	}
	return nil
}

// importedAgain reports whether a default took an imported category's place
// on an earlier run, which leaves it in ids by its own id.
func importedAgain(l LedgerAccount, ids ChartIDs) bool {
	for _, here := range ids {
		if here == l.ID {
			return true
		}
	}
	return false
}

func hasChildren(ctx context.Context, s *Service, admin, id resource.ID) bool {
	all, err := s.store.LedgerAccounts(ctx, admin)
	if err != nil {
		return true
	}
	return slices.ContainsFunc(all, func(l LedgerAccount) bool { return l.ParentID != nil && *l.ParentID == id })
}

// depth counts a category's ancestors among accounts.
func depth(accounts []LedgerAccount, l LedgerAccount) int {
	n := 0
	for p := l.ParentID; p != nil && n < len(accounts); n++ {
		i := slices.IndexFunc(accounts, func(a LedgerAccount) bool { return a.ID == *p })
		if i < 0 {
			break
		}
		p = accounts[i].ParentID
	}
	return n
}

// byName picks the one candidate known by name, in any language.
func byName[T any](candidates []T, name string, known func(T) []string) (T, bool) {
	var found []T
	for _, c := range candidates {
		if slices.ContainsFunc(known(c), func(n string) bool { return n != "" && strings.EqualFold(n, strings.TrimSpace(name)) }) {
			found = append(found, c)
		}
	}
	if len(found) == 1 {
		return found[0], true
	}
	var zero T
	return zero, false
}

func (s *Service) importRates(ctx context.Context, admin resource.ID, in []ImportedRate, seed countries.Seed, ids ChartIDs) error {
	existing, err := s.store.TaxRates(ctx, admin)
	if err != nil {
		return fmt.Errorf("list tax rates: %w", err)
	}
	have := map[resource.ID]bool{}
	for _, r := range existing {
		have[r.ID] = true
	}
	// A default rate is known by its names in every language of the seed.
	names := func(r TaxRate) []string {
		out := []string{r.Name}
		for _, sr := range seed.TaxRates {
			if sr.Type == r.Type && (sr.Name.EN == r.Name || sr.Name.NL == r.Name) {
				out = append(out, sr.Name.EN, sr.Name.NL)
			}
		}
		return out
	}
	alike := func(r TaxRate, ir ImportedRate) bool {
		return r.Type == ir.Type && samePercentage(r.Percentage, ir.Percentage) && r.ShowTax == ir.ShowTax && strings.EqualFold(r.Country, ir.Country)
	}
	taken := map[resource.ID]bool{}
	now := s.now()
	var changed []TaxRate
	for _, ir := range in {
		if have[ir.Origin.ID] {
			ids[ir.Origin.ID] = ir.Origin.ID
			taken[ir.Origin.ID] = true
			continue
		}
		ours := slices.DeleteFunc(slices.Clone(existing), func(r TaxRate) bool { return taken[r.ID] || !alike(r, ir) })
		theirs := slices.DeleteFunc(slices.Clone(in), func(o ImportedRate) bool {
			return o.Type != ir.Type || !samePercentage(o.Percentage, ir.Percentage) || o.ShowTax != ir.ShowTax || !strings.EqualFold(o.Country, ir.Country)
		})
		var r TaxRate
		ok := len(ours) == 1 && len(theirs) == 1
		if ok {
			r = ours[0]
		} else {
			r, ok = byName(ours, ir.Name, names)
		}
		if ok {
			taken[r.ID] = true
			ids[ir.Origin.ID] = r.ID
			r.Name, r.Active, r.UpdatedAt = ir.Name, ir.Active, now
			changed = append(changed, r)
			continue
		}
		created, updated := ir.Origin.Or(now)
		rate := TaxRate{
			ID: ir.Origin.ID, AdministrationID: admin, Name: ir.Name, Percentage: plainPercentage(ir.Percentage), Type: ir.Type,
			ShowTax: ir.ShowTax, Active: ir.Active, Country: strings.ToUpper(ir.Country), Position: len(existing) + 1,
			// Rates alike count in the same boxes of the VAT return.
			ReportBoxes: commonBoxes(existing, ir, alike), CreatedAt: created, UpdatedAt: updated,
		}
		if err := s.store.CreateTaxRate(ctx, rate); err != nil {
			return fmt.Errorf("store imported tax rate: %w", err)
		}
		existing = append(existing, rate)
		have[rate.ID], taken[rate.ID] = true, true
		ids[ir.Origin.ID] = rate.ID
	}
	if len(in) > 0 {
		// Defaults the other system does not have are turned off; rates cannot be removed.
		for _, r := range existing {
			if !taken[r.ID] && r.Active && !slices.ContainsFunc(changed, func(c TaxRate) bool { return c.ID == r.ID }) && slices.ContainsFunc(seed.TaxRates, func(sr countries.TaxRate) bool { return sr.Name.EN == r.Name || sr.Name.NL == r.Name }) {
				r.Active, r.UpdatedAt = false, now
				changed = append(changed, r)
			}
		}
	}
	if len(changed) > 0 {
		if err := s.store.UpdateTaxRates(ctx, changed); err != nil {
			return fmt.Errorf("store taken over tax rates: %w", err)
		}
	}
	return nil
}

// commonBoxes returns the VAT return boxes every existing rate alike shares.
func commonBoxes(existing []TaxRate, ir ImportedRate, alike func(TaxRate, ImportedRate) bool) []string {
	var boxes []string
	for i, r := range slices.DeleteFunc(slices.Clone(existing), func(r TaxRate) bool { return !alike(r, ir) }) {
		if i == 0 {
			boxes = slices.Clone(r.ReportBoxes)
		} else if !slices.Equal(boxes, r.ReportBoxes) {
			return nil
		}
	}
	return boxes
}

// plainPercentage writes "21.0" as "21", as the rates here are written.
func plainPercentage(p string) string {
	p = strings.TrimSpace(p)
	if strings.Contains(p, ".") {
		p = strings.TrimRight(strings.TrimRight(p, "0"), ".")
	}
	if p == "" {
		return "0"
	}
	return p
}

func samePercentage(a, b string) bool { return plainPercentage(a) == plainPercentage(b) }
