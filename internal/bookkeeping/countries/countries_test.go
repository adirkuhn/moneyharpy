package countries

import (
	"slices"
	"strings"
	"testing"

	"github.com/adirkuhn/moneyharpy/internal/taxonomy"
)

var types = []string{"non_current_assets", "current_assets", "equity", "provisions", "non_current_liabilities", "current_liabilities", "revenue", "direct_costs", "expenses", "other_income_expenses"}

func balance(t string) bool { return slices.Index(types, t) < 6 }

// checkAccounts verifies a set of accounts against the seed's base accounts.
func checkAccounts(t *testing.T, s Seed, accounts []Account, known map[string]Account) {
	t.Helper()
	scheme, hasScheme := taxonomy.Lookup(s.Scheme)
	for _, a := range accounts {
		if _, dup := known[a.Key]; dup {
			t.Errorf("%s: key used twice", a.Key)
		}
		known[a.Key] = a
	}
	for _, a := range accounts {
		if !slices.Contains(types, a.Type) {
			t.Errorf("%s: unknown type %q", a.Key, a.Type)
		}
		if a.Name.EN == "" || a.Name.NL == "" {
			t.Errorf("%s: missing a name", a.Key)
		}
		if a.Parent != "" {
			p, ok := known[a.Parent]
			if !ok {
				t.Errorf("%s: unknown parent %q", a.Key, a.Parent)
			} else if p.Type != a.Type {
				t.Errorf("%s: parent %s has type %s, not %s", a.Key, a.Parent, p.Type, a.Type)
			}
		}
		if !hasScheme {
			continue
		}
		it, ok := scheme.Item(a.Taxonomy)
		switch {
		case !ok:
			t.Errorf("%s: %q is not in %s", a.Key, a.Taxonomy, s.Scheme)
		case balance(a.Type) != (it.Side() == taxonomy.BalanceSheet):
			t.Errorf("%s: %s is on the wrong side for %s", a.Key, a.Taxonomy, a.Type)
		}
	}
}

func TestSeedsAreConsistent(t *testing.T) {
	for _, country := range append(Countries(), "XX") {
		s := For(country)
		t.Run(country, func(t *testing.T) {
			if s.Scheme != "" {
				if _, ok := taxonomy.Lookup(s.Scheme); !ok {
					t.Fatalf("unknown scheme %q", s.Scheme)
				}
			}
			known := map[string]Account{}
			checkAccounts(t, s, s.Accounts, known)
			for _, tpl := range s.Templates {
				own := map[string]Account{}
				for k, v := range known {
					own[k] = v
				}
				checkAccounts(t, s, tpl.Accounts, own)
			}
			boxes := map[string]bool{}
			for _, b := range s.Boxes {
				boxes[b.Code] = true
			}
			for _, r := range s.TaxRates {
				for _, b := range r.Boxes {
					if !boxes[b] {
						t.Errorf("%s: unknown box %s", r.Name.EN, b)
					}
				}
			}
		})
	}
}

func TestGenericSeedHasTheSystemAccountsButNoScheme(t *testing.T) {
	g := For("DE")
	if g.Scheme != "" || len(g.TaxRates) != 0 {
		t.Errorf("generic seed scheme %q, %d tax rates", g.Scheme, len(g.TaxRates))
	}
	for _, a := range g.Accounts {
		if a.Taxonomy != "" {
			t.Errorf("%s keeps an RGS code", a.Key)
		}
	}
	if !slices.ContainsFunc(g.Accounts, func(a Account) bool { return a.System == "accounts_receivable" }) {
		t.Error("generic seed lacks accounts receivable")
	}
}

// Every seed needs the categories the books rely on, and names must be unique
// per type because the database enforces it.
func TestSeedsHaveSystemAccountsAndUniqueNames(t *testing.T) {
	var roles []string
	for _, a := range netherlands.Accounts {
		if a.System != "" {
			roles = append(roles, a.System)
		}
	}
	for _, country := range append(Countries(), "XX") {
		s := For(country)
		t.Run(country, func(t *testing.T) {
			for _, role := range roles {
				if !slices.ContainsFunc(s.Accounts, func(a Account) bool { return a.System == role }) {
					t.Errorf("no account with system role %s", role)
				}
			}
			all := slices.Clone(s.Accounts)
			for _, tpl := range s.Templates {
				all = append(all, tpl.Accounts...)
			}
			for _, lang := range []string{"en", "nl"} {
				seen := map[string]string{}
				for _, a := range all {
					name := a.Type + "/" + strings.ToLower(a.Name.In(lang))
					if other, dup := seen[name]; dup {
						t.Errorf("%s and %s share the %s name %q", other, a.Key, lang, a.Name.In(lang))
					}
					seen[name] = a.Key
				}
			}
		})
	}
}
