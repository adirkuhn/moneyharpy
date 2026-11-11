package bookkeeping

import (
	"context"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/adirkuhn/moneyharpy/internal/resource"
	"github.com/adirkuhn/moneyharpy/internal/taxonomy"
)

// AccumulatedDepreciationRole marks the sub-category of a fixed asset
// category that holds what its assets depreciated so far.
const AccumulatedDepreciationRole = "accumulated_depreciation"

// AccumulatedDepreciation returns the sub-category of the fixed asset
// category parent that depreciation is booked on, adding it when the
// category has none yet: "Accumulated depreciation <name>", with the chart
// scheme's cumulative depreciation code when the scheme has one (such as
// BMvaBeiCae under BMvaBei).
func (s *Service) AccumulatedDepreciation(ctx context.Context, admin, user, parent resource.ID) (resource.ID, error) {
	a, err := s.ensure(ctx, admin, user)
	if err != nil {
		return 0, err
	}
	all, err := s.store.LedgerAccounts(ctx, admin)
	if err != nil {
		return 0, fmt.Errorf("list ledger accounts: %w", err)
	}
	i := slices.IndexFunc(all, func(l LedgerAccount) bool { return l.ID == parent })
	if i < 0 {
		return 0, ErrLedgerAccountNotFound
	}
	p := all[i]
	if j := slices.IndexFunc(all, func(l LedgerAccount) bool {
		return l.ParentID != nil && *l.ParentID == parent && (l.System == AccumulatedDepreciationRole || strings.HasSuffix(l.Key, "_depreciation_total"))
	}); j >= 0 {
		return all[j].ID, nil
	}
	name := "Accumulated depreciation " + p.Name
	if strings.HasPrefix(a.Language, "nl") {
		name = "Cumulatieve afschrijving " + p.Name
	}
	now := s.now()
	l := LedgerAccount{
		ID: s.ids.Next(), AdministrationID: admin, Name: name, Type: p.Type, ParentID: &p.ID, Active: true,
		AllowedDocumentTypes: []string{"general_journal_document"}, System: AccumulatedDepreciationRole, CreatedAt: now, UpdatedAt: now,
	}
	if p.Taxonomy != "" {
		st, err := s.store.Settings(ctx, admin)
		if err != nil {
			return 0, fmt.Errorf("load settings: %w", err)
		}
		if scheme, ok := taxonomy.Lookup(st.Scheme); ok {
			if _, ok := scheme.Item(p.Taxonomy + "Cae"); ok {
				l.Taxonomy = p.Taxonomy + "Cae"
			}
		}
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
