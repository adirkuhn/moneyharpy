package bookkeeping

import (
	"context"
	"errors"
	"testing"

	"github.com/adirkuhn/moneyharpy/internal/administration"
	"github.com/adirkuhn/moneyharpy/internal/resource"
)

func TestDetailUsageKeepsTheFiveMostUsed(t *testing.T) {
	svc, store := newTestService("NL")
	ctx := context.Background()
	store.usage = Usage{
		TaxRates:       []UsageCount{{ID: 1, Count: 9}, {ID: 2, Count: 7}, {ID: 3, Count: 5}, {ID: 4, Count: 3}, {ID: 5, Count: 2}, {ID: 6, Count: 1}},
		LedgerAccounts: []UsageCount{{ID: 10, Count: 4}},
	}
	got, err := svc.DetailUsage(ctx, adminID, owner, "purchase")
	if err != nil {
		t.Fatal(err)
	}
	if len(got.TaxRates) != MostUsed || got.TaxRates[0].ID != 1 || got.TaxRates[4].ID != 5 || len(got.LedgerAccounts) != 1 {
		t.Errorf("usage = %+v", got)
	}
	// Lines of the last twelve months count, from the administration's today.
	if store.usageKind != "purchase" || store.usageSince != "2025-11-10" {
		t.Errorf("asked for %s since %s", store.usageKind, store.usageSince)
	}
}

func TestDetailUsageRefusesUnknownKindsAndStrangers(t *testing.T) {
	svc, _ := newTestService("NL")
	ctx := context.Background()
	var verr *resource.ValidationError
	if _, err := svc.DetailUsage(ctx, adminID, owner, "estimate"); !errors.As(err, &verr) {
		t.Errorf("kind estimate: %v", err)
	}
	if _, err := svc.DetailUsage(ctx, adminID, stranger, "sales"); !errors.Is(err, administration.ErrNotFound) {
		t.Errorf("stranger: %v", err)
	}
}
