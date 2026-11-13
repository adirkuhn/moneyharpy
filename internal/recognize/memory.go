package recognize

import (
	"slices"
	"strings"
	"time"
	"unicode"

	"github.com/adirkuhn/moneyharpy/internal/purchases"
	"github.com/adirkuhn/moneyharpy/internal/resource"
)

// certainUses is how many documents in a row must agree on a remembered
// value before it is filled in as certain.
const certainUses = 2

// maxMemoryLines caps the line descriptions remembered per supplier; the
// least recently used go first.
const maxMemoryLines = 200

// Learned is a remembered value with the number of documents in a row that
// agreed on it.
type Learned[T comparable] struct {
	V    T
	Uses int
}

// Known reports whether a value was learned.
func (l Learned[T]) Known() bool { return l.Uses > 0 }

// Confidence is certain once enough documents agreed.
func (l Learned[T]) Confidence() Confidence {
	if l.Uses >= certainUses {
		return Certain
	}
	return Uncertain
}

func (l *Learned[T]) learn(v T) {
	if l.Uses > 0 && l.V == v {
		l.Uses++
		return
	}
	l.V, l.Uses = v, 1
}

// Category is the category and VAT rate remembered for a line description.
type Category struct {
	LedgerAccountID resource.ID
	TaxRateID       resource.ID
	Uses            int
	LastUsed        time.Time
}

// Confidence is certain once enough documents agreed.
func (c Category) Confidence() Confidence {
	if c.Uses >= certainUses {
		return Certain
	}
	return Uncertain
}

// Memory is what users saved on earlier documents of one supplier.
type Memory struct {
	AdministrationID resource.ID
	ContactID        resource.ID
	Kind             Learned[Kind]
	Currency         Learned[string]
	PricesInclTax    Learned[bool]
	// Lines map normalised line descriptions to their category; the empty
	// description is the supplier's default.
	Lines     map[string]Category
	UpdatedAt time.Time
}

// Empty reports a supplier nothing was learned of.
func (m Memory) Empty() bool {
	return !m.Kind.Known() && !m.Currency.Known() && !m.PricesInclTax.Known() && len(m.Lines) == 0
}

// Learn remembers what the saved document d says of its supplier.
func (m *Memory) Learn(d purchases.Document, at time.Time) {
	kind := KindPurchaseInvoice
	if d.Kind == purchases.Receipt {
		kind = KindReceipt
	}
	m.Kind.learn(kind)
	if d.Currency != "" {
		m.Currency.learn(d.Currency)
	}
	m.PricesInclTax.learn(d.PricesAreInclTax)
	if m.Lines == nil {
		m.Lines = map[string]Category{}
	}
	main := -1
	for i, l := range d.Lines {
		if l.LedgerAccountID == nil {
			continue
		}
		if main < 0 || abs(int64(l.Price)) > abs(int64(d.Lines[main].Price)) {
			main = i
		}
		if key := NormalizeDescription(l.Description); key != "" {
			m.learnLine(key, *l.LedgerAccountID, l.TaxRateID, at)
		}
	}
	if main >= 0 {
		m.learnLine("", *d.Lines[main].LedgerAccountID, d.Lines[main].TaxRateID, at)
	}
	m.trim()
	m.UpdatedAt = at
}

func (m *Memory) learnLine(key string, ledger, rate resource.ID, at time.Time) {
	c := m.Lines[key]
	if c.Uses > 0 && c.LedgerAccountID == ledger && c.TaxRateID == rate {
		c.Uses++
	} else {
		c = Category{LedgerAccountID: ledger, TaxRateID: rate, Uses: 1}
	}
	c.LastUsed = at
	m.Lines[key] = c
}

// trim forgets the least recently used descriptions beyond the cap; the
// default stays.
func (m *Memory) trim() {
	if len(m.Lines) <= maxMemoryLines+1 {
		return
	}
	keys := make([]string, 0, len(m.Lines))
	for k := range m.Lines {
		if k != "" {
			keys = append(keys, k)
		}
	}
	slices.SortFunc(keys, func(a, b string) int { return m.Lines[b].LastUsed.Compare(m.Lines[a].LastUsed) })
	for _, k := range keys[maxMemoryLines:] {
		delete(m.Lines, k)
	}
}

// Category returns the category remembered for a line description, or else
// the supplier's default.
func (m Memory) Category(description string) (Category, bool) {
	if key := NormalizeDescription(description); key != "" {
		if c, ok := m.Lines[key]; ok {
			return c, true
		}
	}
	c, ok := m.Lines[""]
	return c, ok
}

// months are left out of descriptions, so "Hosting March" and "Hosting
// April" are the same line.
var months = map[string]bool{}

func init() {
	for _, m := range strings.Fields(`january february march april may june july august september october november december
		jan feb mar apr jun jul aug sep sept oct nov dec
		januari februari maart mei juni juli augustus oktober mrt okt`) {
		months[m] = true
	}
}

// NormalizeDescription reduces a line description to what stays the same
// from one document to the next: lower case words without numbers, dates
// and months.
func NormalizeDescription(s string) string {
	words := strings.FieldsFunc(strings.ToLower(s), func(r rune) bool { return !unicode.IsLetter(r) })
	out := words[:0]
	for _, w := range words {
		if len([]rune(w)) > 1 && !months[w] {
			out = append(out, w)
		}
	}
	n := strings.Join(out, " ")
	if r := []rune(n); len(r) > 200 {
		n = string(r[:200])
	}
	return n
}

func abs(n int64) int64 {
	if n < 0 {
		return -n
	}
	return n
}
