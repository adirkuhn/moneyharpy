// Package countries holds what a new administration starts with in each
// country: VAT rates, the boxes of the VAT return they report to, a standard
// set of categories (ledger accounts) and templates that add more. See
// docs/countries.md for adding a country.
package countries

// Text is a label in English and Dutch, the languages MoneyHarpy speaks.
type Text struct{ EN, NL string }

// In returns the text in language, English when there is no translation.
func (t Text) In(language string) string {
	if len(language) >= 2 && language[:2] == "nl" && t.NL != "" {
		return t.NL
	}
	return t.EN
}

// Account seeds one category.
type Account struct {
	// Key names the account within the seed, so children can name a parent
	// and templates can tell what an administration already has.
	Key    string
	Parent string
	Type   string
	Code   string
	// Taxonomy is the code in the country's chart scheme, if it has one.
	Taxonomy    string
	Name        Text
	Description Text
	// System marks accounts the books rely on, which cannot be removed or
	// change type. The value names the role.
	System string
	// Selected ticks the account when its template's page opens.
	Selected bool
	// Choices are the options of the template's Choice the account is shown
	// for; empty for all of them.
	Choices []string
}

// IsRef reports whether a template account only refers to an account
// defined elsewhere in the seed, by its Key.
func (a Account) IsRef() bool { return a.Type == "" }

// TaxRate seeds one VAT rate.
type TaxRate struct {
	Name       Text
	Percentage string
	Type       string
	ShowTax    bool
	Boxes      []string
}

// Box is one box of the country's VAT return, such as NL/1a.
type Box struct {
	Code string
	Name Text
}

// Template is a theme of Moneybird's Add category: a group of categories to
// pick from, such as for a company car, or one category the user names, such
// as a kind of revenue.
type Template struct {
	Key  string
	Name Text
	// Description says what the theme is for, in the list of themes.
	Description Text
	// Intro and Warning head the theme's page.
	Intro, Warning Text
	// Choice shows some accounts only for one of its options, such as how the
	// car is financed.
	Choice *Choice
	// NameLabel asks for a name that fills {name} in the accounts' names, such
	// as the entrepreneur's. Empty when the names are fixed.
	NameLabel Text
	// Single makes the theme add one category the user names, instead of
	// picking from Accounts.
	Single *Single
	// Accounts are the categories to pick from. An account with only a Key
	// refers to one of the seed's accounts or of another template's.
	Accounts []Account
}

// Choice is a question on a template's page whose answer decides which of
// its accounts show.
type Choice struct {
	Label, Prompt Text
	Options       []Option
}

// Option is an answer of a Choice, or a kind of category a Single adds, with
// its chart scheme code.
type Option struct {
	Key      string
	Name     Text
	Taxonomy string
}

// Single describes the one category a template adds.
type Single struct {
	Type                    string
	NameLabel, OptionsLabel Text
	OptionsHelp             Text
	// Parent offers a parent category of the same type.
	Parent  bool
	Options []Option
}

// Placeholder is replaced by the name a template asks for (NameLabel).
const Placeholder = "{name}"

// TemplateAccounts returns the template's accounts with references replaced
// by the accounts they refer to, ticked and shown as the template says.
func (s Seed) TemplateAccounts(t Template) []Account {
	defs := map[string]Account{}
	for _, a := range s.Accounts {
		defs[a.Key] = a
	}
	for _, other := range s.Templates {
		for _, a := range other.Accounts {
			if !a.IsRef() {
				defs[a.Key] = a
			}
		}
	}
	out := make([]Account, 0, len(t.Accounts))
	for _, a := range t.Accounts {
		if a.IsRef() {
			def, ok := defs[a.Key]
			if !ok {
				continue
			}
			def.Selected, def.Choices = a.Selected, a.Choices
			a = def
		}
		out = append(out, a)
	}
	return out
}

// Seed is what an administration in a country starts with.
type Seed struct {
	Country string
	// Scheme is the chart scheme accounts map to, or empty.
	Scheme    string
	Boxes     []Box
	TaxRates  []TaxRate
	Accounts  []Account
	Templates []Template
	// FinancialAccounts maps a financial account type, such as BankAccount,
	// to the chart scheme code of the category its money is kept in.
	FinancialAccounts map[string]string
}

var seeds = map[string]Seed{"NL": netherlands}

// For returns the seed of a country. Countries without their own seed get
// the generic one: the categories the books need, without a chart scheme or
// VAT rates, which people then add themselves.
func For(country string) Seed {
	if s, ok := seeds[country]; ok {
		return s
	}
	return generic(country)
}

// Countries returns the countries with their own seed.
func Countries() []string {
	out := make([]string, 0, len(seeds))
	for c := range seeds {
		out = append(out, c)
	}
	return out
}

func generic(country string) Seed {
	strip := func(in []Account) []Account {
		out := make([]Account, len(in))
		for i, a := range in {
			a.Taxonomy = ""
			out[i] = a
		}
		return out
	}
	templates := make([]Template, len(netherlands.Templates))
	for i, t := range netherlands.Templates {
		t.Accounts = strip(t.Accounts)
		if t.Single != nil {
			single := *t.Single
			single.Options = make([]Option, len(t.Single.Options))
			for j, o := range t.Single.Options {
				o.Taxonomy = ""
				single.Options[j] = o
			}
			t.Single = &single
		}
		templates[i] = t
	}
	return Seed{Country: country, Accounts: strip(netherlands.Accounts), Templates: templates}
}
