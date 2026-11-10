// Package taxonomy holds the standard charts of accounts that ledger accounts
// can be mapped to, such as the Dutch Referentie Grootboekschema (RGS). Each
// country can have its own; see docs/countries.md for adding one.
package taxonomy

import (
	"embed"
	"encoding/csv"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"sync"
)

//go:embed data/*.csv
var files embed.FS

// Side tells which statement a code belongs to.
type Side int

// The sides of the books.
const (
	AnySide Side = iota
	BalanceSheet
	ProfitAndLoss
)

// Item is one code of a scheme.
type Item struct {
	Code        string
	Reference   string
	Name        string
	Description string
	// DebitCredit is "D", "C" or empty for headings.
	DebitCredit string
	Level       int
	Parent      string
	// Basic marks the codes the scheme recommends for most small businesses.
	Basic bool
}

// Side reports the statement of the code. RGS codes start with B for the
// balance sheet and W for profit and loss.
func (it Item) Side() Side {
	switch {
	case strings.HasPrefix(it.Code, "B"):
		return BalanceSheet
	case strings.HasPrefix(it.Code, "W"):
		return ProfitAndLoss
	}
	return AnySide
}

// Scheme is a standard chart of accounts.
type Scheme struct {
	ID      string
	Country string
	Name    string
	Version string
	Source  string
	file    string

	once  sync.Once
	items []Item
	byKey map[string]int
	err   error
}

var schemes = []*Scheme{
	{
		ID: "nl-rgs-3.8", Country: "NL", Name: "Referentie Grootboekschema (RGS)", Version: "3.8",
		Source: "https://www.referentiegrootboekschema.nl/definitieve-versie-rgs-38", file: "data/nl-rgs-3.8.csv",
	},
}

// All returns every built-in scheme.
func All() []*Scheme {
	return slices.Clone(schemes)
}

// Lookup returns the scheme with the given ID.
func Lookup(id string) (*Scheme, bool) {
	for _, s := range schemes {
		if s.ID == id {
			return s, true
		}
	}
	return nil, false
}

// ForCountry returns the schemes meant for a country, newest first.
func ForCountry(country string) []*Scheme {
	var out []*Scheme
	for _, s := range schemes {
		if s.Country == country {
			out = append(out, s)
		}
	}
	return out
}

// Len returns the number of codes.
func (s *Scheme) Len() int {
	s.load()
	return len(s.items)
}

// Item returns the code's item.
func (s *Scheme) Item(code string) (Item, bool) {
	s.load()
	i, ok := s.byKey[code]
	if !ok {
		return Item{}, false
	}
	return s.items[i], true
}

// Search finds codes whose code, reference or name contains query, ignoring
// case, on the given side, in scheme order. Codes whose name starts with the
// query come first.
func (s *Scheme) Search(query string, side Side, limit int) []Item {
	s.load()
	q := strings.ToLower(strings.TrimSpace(query))
	var first, rest []Item
	for _, it := range s.items {
		if side != AnySide && it.Side() != side {
			continue
		}
		name := strings.ToLower(it.Name)
		switch {
		case q == "":
			rest = append(rest, it)
		case strings.HasPrefix(name, q) || strings.EqualFold(it.Code, q) || it.Reference == q:
			first = append(first, it)
		case strings.Contains(name, q) || strings.Contains(strings.ToLower(it.Code), q) || strings.HasPrefix(it.Reference, q):
			rest = append(rest, it)
		}
	}
	out := append(first, rest...)
	if len(out) > limit {
		out = out[:limit]
	}
	return out
}

// load reads the scheme's embedded file once. The files are checked by the
// tests, so a failure here is a build defect and panics.
func (s *Scheme) load() {
	s.once.Do(func() {
		s.items, s.err = read(s.file)
		if s.err != nil {
			panic(fmt.Sprintf("taxonomy %s: %v", s.ID, s.err))
		}
		s.byKey = make(map[string]int, len(s.items))
		for i, it := range s.items {
			s.byKey[it.Code] = i
		}
	})
}

func read(name string) ([]Item, error) {
	f, err := files.Open(name)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	rows, err := csv.NewReader(f).ReadAll()
	if err != nil {
		return nil, err
	}
	items := make([]Item, 0, len(rows))
	for _, r := range rows[1:] {
		level, err := strconv.Atoi(r[5])
		if err != nil {
			return nil, fmt.Errorf("%s: level %q", r[0], r[5])
		}
		items = append(items, Item{
			Code: r[0], Reference: r[1], Name: r[2], Description: r[3], DebitCredit: r[4], Level: level, Parent: r[6], Basic: r[7] == "true",
		})
	}
	return items, nil
}
