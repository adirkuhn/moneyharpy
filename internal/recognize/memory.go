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

// maxMemoryLines caps the line descriptions remembered per supplier and
// kind of document; the least recently used go first.
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

// Remembered is a learned value with the last document that taught it.
type Remembered[T comparable] struct {
	Learned[T]
	SourceDocumentID resource.ID
}

// Category is the category and VAT rate remembered for a line description.
type Category struct {
	LedgerAccountID resource.ID
	TaxRateID       resource.ID
	Uses            int
	LastUsed        time.Time
	// SourceDocumentID is the last document that taught it.
	SourceDocumentID resource.ID
}

// Confidence is certain once enough documents agreed.
func (c Category) Confidence() Confidence {
	if c.Uses >= certainUses {
		return Certain
	}
	return Uncertain
}

// KindMemory is what users saved on earlier documents of one kind, purchase
// invoices or receipts, of one supplier.
type KindMemory struct {
	Currency      Learned[string]
	PricesInclTax Learned[bool]
	// Lines map normalised line descriptions to their category; the empty
	// description is the default of the supplier's documents of the kind.
	Lines map[string]Category
	// LastDocumentID is the last document learned from.
	LastDocumentID resource.ID
	UpdatedAt      time.Time
}

// Empty reports a kind nothing was learned of.
func (k KindMemory) Empty() bool {
	return !k.Currency.Known() && !k.PricesInclTax.Known() && len(k.Lines) == 0
}

// Memory is what users saved on earlier documents of one supplier, apart
// per kind of document.
type Memory struct {
	AdministrationID resource.ID
	ContactID        resource.ID
	// Kind is the kind of the supplier's documents.
	Kind     Learned[Kind]
	Invoices KindMemory
	Receipts KindMemory
	// UpdatedAt is when anything was last learned.
	UpdatedAt time.Time
}

// Kinds are the kinds of documents remembered apart.
var Kinds = []Kind{KindPurchaseInvoice, KindReceipt}

// Empty reports a supplier nothing was learned of.
func (m Memory) Empty() bool {
	return !m.Kind.Known() && m.Invoices.Empty() && m.Receipts.Empty()
}

// Of returns the memory of a kind of documents.
func (m *Memory) Of(kind Kind) *KindMemory {
	if kind == KindReceipt {
		return &m.Receipts
	}
	return &m.Invoices
}

// KindOf is the kind of a purchase invoice or receipt (purchases.Receipt
// or PurchaseInvoice).
func KindOf(documentKind string) Kind {
	if documentKind == purchases.Receipt {
		return KindReceipt
	}
	return KindPurchaseInvoice
}

// DocumentKind is the purchases kind of a kind of document.
func DocumentKind(k Kind) string {
	if k == KindReceipt {
		return purchases.Receipt
	}
	return purchases.PurchaseInvoice
}

// otherKind is the kind a supplier's documents are when not of kind k.
func otherKind(k Kind) Kind {
	if k == KindReceipt {
		return KindPurchaseInvoice
	}
	return KindReceipt
}

// Learn remembers what the saved document d says of its supplier.
func (m *Memory) Learn(d purchases.Document, at time.Time) {
	kind := KindOf(d.Kind)
	m.Kind.learn(kind)
	m.Of(kind).learn(d, at)
	m.UpdatedAt = at
}

func (k *KindMemory) learn(d purchases.Document, at time.Time) {
	if d.Currency != "" {
		k.Currency.learn(d.Currency)
	}
	k.PricesInclTax.learn(d.PricesAreInclTax)
	if k.Lines == nil {
		k.Lines = map[string]Category{}
	}
	main := -1
	for i, l := range d.Lines {
		if l.LedgerAccountID == nil {
			continue
		}
		if main < 0 || absInt64(int64(l.Price)) > absInt64(int64(d.Lines[main].Price)) {
			main = i
		}
		if key := NormalizeDescription(l.Description); key != "" {
			k.learnLine(key, *l.LedgerAccountID, l.TaxRateID, d.ID, at)
		}
	}
	if main >= 0 {
		k.learnLine("", *d.Lines[main].LedgerAccountID, d.Lines[main].TaxRateID, d.ID, at)
	}
	k.trim()
	k.LastDocumentID, k.UpdatedAt = d.ID, at
}

func (k *KindMemory) learnLine(key string, ledger, rate, source resource.ID, at time.Time) {
	c := k.Lines[key]
	if c.Uses > 0 && c.LedgerAccountID == ledger && c.TaxRateID == rate {
		c.Uses++
	} else {
		c = Category{LedgerAccountID: ledger, TaxRateID: rate, Uses: 1}
	}
	c.LastUsed, c.SourceDocumentID = at, source
	k.Lines[key] = c
}

// trim forgets the least recently used descriptions beyond the cap; the
// default stays.
func (k *KindMemory) trim() {
	if len(k.Lines) <= maxMemoryLines+1 {
		return
	}
	keys := make([]string, 0, len(k.Lines))
	for key := range k.Lines {
		if key != "" {
			keys = append(keys, key)
		}
	}
	slices.SortFunc(keys, func(a, b string) int { return k.Lines[b].LastUsed.Compare(k.Lines[a].LastUsed) })
	for _, key := range keys[maxMemoryLines:] {
		delete(k.Lines, key)
	}
}

// Recalled is what is remembered of a supplier for one kind of document:
// what its documents of that kind taught, else what its documents of the
// other kind did, which is never certain.
type Recalled struct {
	same, other KindMemory
}

// Recall returns what is remembered for documents of kind.
func (m Memory) Recall(kind Kind) Recalled {
	return Recalled{same: *m.Of(kind), other: *m.Of(otherKind(kind))}
}

// guess caps the uses of a value learned of the other kind, so it stays
// uncertain.
func guess(uses int) int { return min(uses, 1) }

// Currency returns the remembered currency.
func (r Recalled) Currency() Remembered[string] {
	if r.same.Currency.Known() {
		return Remembered[string]{r.same.Currency, r.same.LastDocumentID}
	}
	l := r.other.Currency
	l.Uses = guess(l.Uses)
	return Remembered[string]{l, r.other.LastDocumentID}
}

// PricesInclTax returns whether prices were remembered to include VAT.
func (r Recalled) PricesInclTax() Remembered[bool] {
	if r.same.PricesInclTax.Known() {
		return Remembered[bool]{r.same.PricesInclTax, r.same.LastDocumentID}
	}
	l := r.other.PricesInclTax
	l.Uses = guess(l.Uses)
	return Remembered[bool]{l, r.other.LastDocumentID}
}

// Category returns the category remembered for a line description: of the
// same kind of document, else of the other kind, else the default of the
// same kind, else that of the other kind.
func (r Recalled) Category(description string) (Category, bool) {
	other := func(c Category, ok bool) (Category, bool) {
		c.Uses = guess(c.Uses)
		return c, ok
	}
	if key := NormalizeDescription(description); key != "" {
		if c, ok := r.same.Lines[key]; ok {
			return c, true
		}
		if c, ok := r.other.Lines[key]; ok {
			return other(c, true)
		}
	}
	if c, ok := r.same.Lines[""]; ok {
		return c, true
	}
	c, ok := r.other.Lines[""]
	return other(c, ok)
}

// monthWords are left out of descriptions, so "Hosting March" and "Hosting
// April" are the same line.
var monthWords = map[string]bool{}

func init() {
	for _, m := range strings.Fields(`january february march april may june july august september october november december
		jan feb mar apr jun jul aug sep sept oct nov dec
		januari februari maart mei juni juli augustus oktober mrt okt`) {
		monthWords[m] = true
	}
}

// NormalizeDescription reduces a line description to what stays the same
// from one document to the next: lower case words without numbers, dates
// and monthWords.
func NormalizeDescription(s string) string {
	words := strings.FieldsFunc(strings.ToLower(s), func(r rune) bool { return !unicode.IsLetter(r) })
	out := words[:0]
	for _, w := range words {
		if len([]rune(w)) > 1 && !monthWords[w] {
			out = append(out, w)
		}
	}
	n := strings.Join(out, " ")
	if r := []rune(n); len(r) > 200 {
		n = string(r[:200])
	}
	return n
}

func absInt64(n int64) int64 {
	if n < 0 {
		return -n
	}
	return n
}
