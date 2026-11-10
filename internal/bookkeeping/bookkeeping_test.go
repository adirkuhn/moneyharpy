package bookkeeping

import (
	"context"
	"errors"
	"testing"

	"github.com/adirkuhn/moneyharpy/internal/administration"
)

func TestSeedsOnceFromCountry(t *testing.T) {
	svc, store := newTestService("NL")
	ctx := context.Background()
	for range 2 {
		if _, err := svc.Settings(ctx, adminID, owner); err != nil {
			t.Fatal(err)
		}
	}
	if store.seeds != 1 {
		t.Errorf("seeded %d times, want 1", store.seeds)
	}
	if store.settings.Scheme != "nl-rgs-3.8" {
		t.Errorf("scheme = %q, want nl-rgs-3.8", store.settings.Scheme)
	}
	if len(store.rates) == 0 || len(store.accounts) == 0 {
		t.Fatalf("seeded %d rates, %d accounts", len(store.rates), len(store.accounts))
	}
	for _, l := range store.accounts {
		if l.Taxonomy == "" {
			t.Errorf("%s has no RGS code", l.Name)
		}
		if l.ParentID != nil {
			if p, ok := store.accounts[*l.ParentID]; !ok || p.Type != l.Type {
				t.Errorf("%s has parent %v of another type", l.Name, l.ParentID)
			}
		}
	}
	if _, err := svc.Settings(ctx, adminID, stranger); !errors.Is(err, administration.ErrNotFound) {
		t.Errorf("Settings() by stranger error = %v, want ErrNotFound", err)
	}
}

func TestGenericSeedHasNoSchemeOrRates(t *testing.T) {
	svc, store := newTestService("BR")
	if _, err := svc.Settings(context.Background(), adminID, owner); err != nil {
		t.Fatal(err)
	}
	if store.settings.Scheme != "" || len(store.rates) != 0 || len(store.accounts) == 0 {
		t.Errorf("scheme %q, %d rates, %d accounts", store.settings.Scheme, len(store.rates), len(store.accounts))
	}
}

func TestUpdateSettings(t *testing.T) {
	svc, store := newTestService("NL")
	ctx := context.Background()
	got, err := svc.UpdateSettings(ctx, adminID, owner, SettingsChanges{VATPeriod: ptr("month"), ICPPeriod: ptr("year"), LockedUntil: ptr("2026-11-10")})
	if err != nil {
		t.Fatal(err)
	}
	if got.VATPeriod != "month" || got.ICPPeriod != "year" || *got.LockedUntil != "2026-11-10" {
		t.Errorf("UpdateSettings() = %+v", got)
	}
	got, err = svc.UpdateSettings(ctx, adminID, owner, SettingsChanges{LockedUntil: ptr("")})
	if err != nil || got.LockedUntil != nil {
		t.Errorf("unlock = %+v, %v", got, err)
	}

	_, err = svc.UpdateSettings(ctx, adminID, owner, SettingsChanges{
		VATPeriod: ptr("week"), ICPPeriod: ptr(""), Scheme: ptr("xx"), LockedUntil: ptr("2026-11-11"),
	})
	fields := fieldsOf(t, err)
	for _, f := range []string{"vat_documents_period", "intra_community_transactions_declaration_period", "chart_scheme", "period_locked_until"} {
		if fields[f] == nil {
			t.Errorf("fields = %v, want %s", fields, f)
		}
	}
	if _, err := svc.UpdateSettings(ctx, adminID, owner, SettingsChanges{LockedUntil: ptr("10-11-2026")}); fieldsOf(t, err)["period_locked_until"] == nil {
		t.Error("accepted a date in the wrong format")
	}

	if _, err := svc.UpdateSettings(ctx, adminID, owner, SettingsChanges{Scheme: ptr("")}); err != nil {
		t.Fatal(err)
	}
	for _, l := range store.accounts {
		if l.Taxonomy != "" {
			t.Fatalf("%s kept RGS code %s after the scheme was removed", l.Name, l.Taxonomy)
		}
	}
}
