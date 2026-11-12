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

// checkTemplate verifies what a template's page shows: references that
// resolve, parents that are on the page too, choices and options that exist.
func checkTemplate(t *testing.T, s Seed, tpl Template) {
	t.Helper()
	if tpl.Key == "" || tpl.Name.EN == "" || tpl.Name.NL == "" || tpl.Description.NL == "" || tpl.Intro.NL == "" {
		t.Errorf("template %q lacks a key, name, description or intro", tpl.Key)
	}
	accounts := s.TemplateAccounts(tpl)
	if len(accounts) != len(tpl.Accounts) {
		t.Errorf("%s: %d of %d references resolve", tpl.Key, len(accounts), len(tpl.Accounts))
	}
	if (tpl.Single == nil) == (len(accounts) == 0) {
		t.Errorf("%s: needs either accounts or a single category", tpl.Key)
	}
	here := map[string]bool{}
	for _, a := range accounts {
		if here[a.Key] {
			t.Errorf("%s: %s twice", tpl.Key, a.Key)
		}
		here[a.Key] = true
	}
	for _, a := range accounts {
		if a.Parent != "" && !here[a.Parent] {
			t.Errorf("%s: %s is under %s, which the template does not show", tpl.Key, a.Key, a.Parent)
		}
		for _, c := range a.Choices {
			if tpl.Choice == nil || !slices.ContainsFunc(tpl.Choice.Options, func(o Option) bool { return o.Key == c }) {
				t.Errorf("%s: %s is shown for unknown choice %q", tpl.Key, a.Key, c)
			}
		}
		named := strings.Contains(a.Name.EN, Placeholder) && strings.Contains(a.Name.NL, Placeholder)
		if named != (tpl.NameLabel.EN != "") {
			t.Errorf("%s: %s has a {name} placeholder %v, the template asks for a name %v", tpl.Key, a.Key, named, tpl.NameLabel.EN != "")
		}
		if named && a.Code != "" {
			t.Errorf("%s: %s is added once per name, so it can't have a fixed code", tpl.Key, a.Key)
		}
	}
	if tpl.Single == nil {
		return
	}
	scheme, hasScheme := taxonomy.Lookup(s.Scheme)
	if !slices.Contains(types, tpl.Single.Type) || len(tpl.Single.Options) == 0 || tpl.Single.NameLabel.NL == "" {
		t.Errorf("%s: single category of type %q with %d options", tpl.Key, tpl.Single.Type, len(tpl.Single.Options))
	}
	for _, o := range tpl.Single.Options {
		if o.Key == "" || o.Name.NL == "" {
			t.Errorf("%s: option %q lacks a key or name", tpl.Key, o.Key)
		}
		if !hasScheme {
			if o.Taxonomy != "" {
				t.Errorf("%s: option %s has a code without a scheme", tpl.Key, o.Key)
			}
			continue
		}
		it, ok := scheme.Item(o.Taxonomy)
		if !ok || balance(tpl.Single.Type) != (it.Side() == taxonomy.BalanceSheet) {
			t.Errorf("%s: option %s has code %q, not one for %s", tpl.Key, o.Key, o.Taxonomy, tpl.Single.Type)
		}
	}
}

func TestDutchSeedHasMoneybirdsTenThemes(t *testing.T) {
	var keys []string
	for _, tpl := range For("NL").Templates {
		keys = append(keys, tpl.Key)
	}
	want := []string{"revenue", "direct_costs", "other_costs_set", "other_relevant", "sales_set", "car_set", "housing", "staff_set", "entrepreneur", "fixed_assets"}
	if !slices.Equal(keys, want) {
		t.Errorf("themes = %v, want %v", keys, want)
	}
}

func TestTemplateAccountsResolveReferences(t *testing.T) {
	s := For("NL")
	car := s.Templates[slices.IndexFunc(s.Templates, func(t Template) bool { return t.Key == "car_set" })]
	accounts := s.TemplateAccounts(car)
	i := slices.IndexFunc(accounts, func(a Account) bool { return a.Key == "depreciation" })
	if i < 0 {
		t.Fatal("the car theme does not show depreciation")
	}
	if d := accounts[i]; d.Type != "expenses" || d.Name.EN != "Depreciation costs" || !d.Selected || !slices.Contains(d.Choices, financialLease) {
		t.Errorf("depreciation = %+v", d)
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
				var defs []Account
				for _, a := range tpl.Accounts {
					if !a.IsRef() {
						defs = append(defs, a)
					}
				}
				// Templates may refer to each other's accounts, so keys are unique across all of them.
				checkAccounts(t, s, defs, known)
			}
			for _, tpl := range s.Templates {
				checkTemplate(t, s, tpl)
			}
			for kind, code := range s.FinancialAccounts {
				if scheme, ok := taxonomy.Lookup(s.Scheme); !ok {
					t.Errorf("%s: code %s without a scheme", kind, code)
				} else if it, ok := scheme.Item(code); !ok || it.Side() != taxonomy.BalanceSheet {
					t.Errorf("%s: %s is not a balance sheet code of %s", kind, code, s.Scheme)
				}
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
				for _, a := range tpl.Accounts {
					if !a.IsRef() {
						all = append(all, a)
					}
				}
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
