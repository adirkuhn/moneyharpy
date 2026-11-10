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
}

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

// Template adds a group of categories in one go, such as for a company car.
type Template struct {
	Key         string
	Name        Text
	Description Text
	Accounts    []Account
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
		templates[i] = t
	}
	return Seed{Country: country, Accounts: strip(netherlands.Accounts), Templates: templates}
}
