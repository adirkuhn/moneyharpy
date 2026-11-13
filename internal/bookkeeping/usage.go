package bookkeeping

import (
	"cmp"
	"context"
	"fmt"
	"slices"

	"github.com/adirkuhn/moneyharpy/internal/resource"
)

// MostUsed is how many VAT rates and categories DetailUsage returns of each.
const MostUsed = 5

// UsageKinds are the documents whose lines DetailUsage counts: purchase
// invoices and receipts, or sales invoices.
var UsageKinds = []string{"purchase", "sales"}

// UsageCount is how many lines used a VAT rate or category.
type UsageCount struct {
	ID    resource.ID
	Count int
}

// Usage holds the VAT rates and categories lines used, most used first.
type Usage struct {
	TaxRates       []UsageCount
	LedgerAccounts []UsageCount
}

// DetailUsage returns the VAT rates and categories the lines of one kind of
// document used most in the last twelve months, so the line editor can offer
// them first.
func (s *Service) DetailUsage(ctx context.Context, admin, user resource.ID, kind string) (Usage, error) {
	a, err := s.ensure(ctx, admin, user)
	if err != nil {
		return Usage{}, err
	}
	if !slices.Contains(UsageKinds, kind) {
		var verr resource.ValidationError
		verr.Add("kind", "is not included in the list")
		return Usage{}, verr.OrNil()
	}
	u, err := s.store.DetailUsage(ctx, admin, kind, s.today(a.Administration).AddDate(-1, 0, 0))
	if err != nil {
		return Usage{}, fmt.Errorf("count detail usage: %w", err)
	}
	return Usage{TaxRates: mostUsed(u.TaxRates), LedgerAccounts: mostUsed(u.LedgerAccounts)}, nil
}

// mostUsed orders counts most used first, then by id, and keeps the first
// MostUsed.
func mostUsed(counts []UsageCount) []UsageCount {
	out := slices.Clone(counts)
	slices.SortStableFunc(out, func(a, b UsageCount) int {
		return cmp.Or(cmp.Compare(b.Count, a.Count), cmp.Compare(a.ID, b.ID))
	})
	return out[:min(len(out), MostUsed)]
}
