package recognize

import (
	"fmt"
	"math"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/adirkuhn/moneyharpy/internal/bookkeeping"
	"github.com/adirkuhn/moneyharpy/internal/money"
	"github.com/adirkuhn/moneyharpy/internal/purchases"
	"github.com/adirkuhn/moneyharpy/internal/resource"
	"github.com/adirkuhn/moneyharpy/internal/sales"
)

// Target is the document to fill in, as it is now.
type Target struct {
	// Kind is purchases.TypelessDocument, PurchaseInvoice or Receipt.
	Kind             string
	ContactID        resource.ID
	Reference        string
	Date             time.Time
	DueDate          *time.Time
	Currency         string
	PricesAreInclTax bool
	Lines            []sales.Detail
}

// SupplierMatch is the contact a document is from, as matched or created.
type SupplierMatch struct {
	ContactID  resource.ID
	Name       string
	Confidence Confidence
}

// Plan is how to fill in a document: its kind, the changes and the fields
// they fill in.
type Plan struct {
	// Kind is purchases.PurchaseInvoice or purchases.Receipt.
	Kind    string
	Changes purchases.Changes
	Fields  []Field
}

// PlanInput is what a plan is made of.
type PlanInput struct {
	Extraction Extraction
	Target     Target
	// Previous is what was filled in before; fields it holds, and fields
	// still empty, are filled in again. A first reading also replaces the
	// defaults a new document starts with: its date, currency and VAT mode.
	Previous Recognition
	Supplier SupplierMatch
	Memory   Memory
	// Rates are the administration's purchase VAT rates.
	Rates []bookkeeping.TaxRate
}

// fallbackDescription names a line made of a document's totals without a
// supplier name.
const fallbackDescription = "Purchase"

var currencyPattern = regexp.MustCompile(`^[A-Z]{3}$`)

// MakePlan works out how to fill in the target with what was read and what
// is remembered of its supplier.
func MakePlan(in PlanInput) Plan {
	e, t := in.Extraction, in.Target
	firstRun := in.Previous.State != StateDone
	var p Plan
	mark := func(name string, c Confidence) {
		if c == "" {
			c = Uncertain
		}
		p.Fields = append(p.Fields, Field{Name: name, Confidence: c})
	}
	fill := func(name string, empty, defaulted bool) bool {
		return in.Previous.Has(name) || empty || (defaulted && firstRun)
	}

	// The kind: as read, else as the supplier's earlier documents were,
	// else a purchase invoice.
	p.Kind = t.Kind
	if t.Kind == purchases.TypelessDocument {
		kind, conf := KindPurchaseInvoice, Uncertain
		switch {
		case e.Kind.Found && e.Kind.V != KindUnknown && (e.Kind.Confidence == Certain || !in.Memory.Kind.Known()):
			kind, conf = e.Kind.V, e.Kind.Confidence
		case in.Memory.Kind.Known():
			kind, conf = in.Memory.Kind.V, in.Memory.Kind.Confidence()
		}
		p.Kind = purchases.PurchaseInvoice
		if kind == KindReceipt {
			p.Kind = purchases.Receipt
		}
		mark(FieldDocumentType, conf)
	}

	c := &p.Changes
	if in.Supplier.ContactID != 0 && in.Supplier.ContactID != t.ContactID && fill(FieldContactID, t.ContactID == 0, false) {
		id := in.Supplier.ContactID
		c.ContactID = &id
		mark(FieldContactID, in.Supplier.Confidence)
	} else if in.Supplier.ContactID != 0 && in.Supplier.ContactID == t.ContactID && in.Previous.Has(FieldContactID) {
		mark(FieldContactID, in.Supplier.Confidence)
	}
	if v := strings.TrimSpace(e.Reference.V); e.Reference.Found && v != "" && len(v) <= 1000 && fill(FieldReference, t.Reference == "" || isFileName(t.Reference), false) {
		c.Reference = &v
		mark(FieldReference, e.Reference.Confidence)
	}
	date := t.Date
	if e.Date.Found && !e.Date.V.IsZero() && fill(FieldDate, false, true) {
		date = e.Date.V
		v := date.Format(time.DateOnly)
		c.Date = &v
		mark(FieldDate, e.Date.Confidence)
	}
	if e.DueDate.Found && !e.DueDate.V.Before(date) && fill(FieldDueDate, t.DueDate == nil, false) {
		v := e.DueDate.V.Format(time.DateOnly)
		c.DueDate = &v
		mark(FieldDueDate, e.DueDate.Confidence)
	}
	if fill(FieldCurrency, false, true) {
		cur := strings.ToUpper(strings.TrimSpace(e.Currency.V))
		switch {
		case e.Currency.Found && currencyPattern.MatchString(cur):
			c.Currency = &cur
			mark(FieldCurrency, e.Currency.Confidence)
		case in.Memory.Currency.Known():
			v := in.Memory.Currency.V
			c.Currency = &v
			mark(FieldCurrency, in.Memory.Currency.Confidence())
		}
	}

	incl, inclConf, inclKnown := t.PricesAreInclTax, Confidence(""), false
	switch {
	case e.PricesInclTax.Found:
		incl, inclConf, inclKnown = e.PricesInclTax.V, e.PricesInclTax.Confidence, true
	case in.Memory.PricesInclTax.Known():
		incl, inclConf, inclKnown = in.Memory.PricesInclTax.V, in.Memory.PricesInclTax.Confidence(), true
	}
	lines, linesIncl := proposeLines(e, in.Supplier.Name, incl)
	if linesIncl != nil {
		incl, inclConf, inclKnown = *linesIncl, e.TotalInclTax.Confidence, true
	}
	fallback := defaultRate(in.Rates)
	rates := make([]lineRate, len(lines))
	for i, l := range lines {
		rates[i] = rateOf(l, in, fallback)
	}
	if !incl && e.TotalInclTax.Found && e.TotalInclTax.Confidence == Certain {
		if fitted, ok := fitPrintedTotal(lines, rates, in.Rates, e.TotalInclTax.V, e.VAT); ok {
			lines, incl, inclConf, inclKnown = fitted, true, Certain, true
		}
	}
	replaceLines := len(lines) > 0 && (len(t.Lines) == 0 || allMarked(in.Previous, len(t.Lines)))
	if inclKnown && (fill(FieldPricesAreInclTax, false, true) || replaceLines) {
		v := incl
		c.PricesAreInclTax = &v
		mark(FieldPricesAreInclTax, inclConf)
	}
	if !replaceLines {
		return p
	}
	for _, l := range t.Lines {
		id := l.ID
		c.Details = append(c.Details, sales.DetailInput{ID: &id, Destroy: true})
	}
	for i, l := range lines {
		field := func(name string) string { return fmt.Sprintf("details.%d.%s", i, name) }
		d := sales.DetailInput{Description: &l.description, Amount: &l.amount}
		price := l.price.String()
		d.Price = &price
		mark(field("description"), l.confidence)
		mark(field("amount"), l.confidence)
		mark(field("price"), l.confidence)

		category, remembered := in.Memory.Category(l.description)
		if rate := rates[i]; rate.id != 0 {
			d.TaxRateID = &rate.id
			mark(field("tax_rate_id"), rate.confidence)
		}
		if remembered && category.LedgerAccountID != 0 {
			ledger := category.LedgerAccountID
			d.LedgerAccountID = &ledger
			mark(field("ledger_account_id"), category.Confidence())
		}
		c.Details = append(c.Details, d)
	}
	return p
}

// lineRate is the purchase VAT rate a line gets, and how sure it is.
type lineRate struct {
	id         resource.ID
	confidence Confidence
}

// rateOf picks the VAT rate of a line: for VAT the buyer accounts for the
// reverse-charged rate, else the rate read, else the one remembered for
// such lines of the supplier, else the standard rate (fallback).
func rateOf(l line, in PlanInput, fallback resource.ID) lineRate {
	if l.reverse {
		// The document says the buyer accounts for the VAT but not
		// at which rate: the standard rate is a guess to check.
		if id, ok := MapReverseCharge(supplierCountry(in.Extraction.Supplier) == "NL", in.Rates); ok {
			return lineRate{id, Uncertain}
		}
	}
	if l.rate.Found {
		if id, ok := MapTaxRate(l.rate.V, in.Rates); ok {
			return lineRate{id, l.rate.Confidence}
		}
	}
	if category, ok := in.Memory.Category(l.description); ok && category.TaxRateID != 0 {
		return lineRate{category.TaxRateID, category.Confidence()}
	}
	return lineRate{fallback, Uncertain}
}

// allMarked reports whether every one of n lines still holds what was
// filled in, so reading again may replace them.
func allMarked(r Recognition, n int) bool {
	for i := range n {
		if !r.Has(fmt.Sprintf("details.%d.price", i)) {
			return false
		}
	}
	return true
}

// line is a proposed line of the document.
type line struct {
	description string
	amount      string
	price       money.Amount
	rate        Value[string]
	// reverse is set when the buyer accounts for the VAT of the line.
	reverse    bool
	confidence Confidence
}

// proposeLines makes the lines of the document: those read, else one per
// VAT rate of its VAT summary, else one with its total. A line of the total
// including VAT makes the prices include VAT, which inclTax then tells.
func proposeLines(e Extraction, supplierName string, incl bool) (lines []line, inclTax *bool) {
	description := cmpOr(strings.TrimSpace(supplierName), fallbackDescription)
	summaryRate := Value[string]{}
	if len(e.VAT) == 1 {
		summaryRate = Value[string]{V: e.VAT[0].Rate, Found: true, Confidence: Uncertain}
	}
	reverse := false
	for _, v := range e.VAT {
		reverse = reverse || v.ReverseCharge
	}
	for _, l := range e.Lines {
		conf := l.Price.Confidence
		amount := "1"
		if l.Quantity.Found && strings.TrimSpace(l.Quantity.V) != "" {
			amount = strings.TrimSpace(l.Quantity.V)
		}
		qty, err := sales.ParseQuantity(amount)
		if err != nil || qty == 0 {
			amount, qty = "1", 1000
		}
		var price money.Amount
		switch {
		case l.Price.Found:
			price = l.Price.V
		case l.Total.Found:
			price, conf = money.Amount(roundDiv(int64(l.Total.V)*1000, qty)), l.Total.Confidence
			if price*money.Amount(qty)/1000 != l.Total.V {
				// A discounted line: its total, with the quantity kept in
				// the description, so the document adds up to the cent.
				if amount != "1" {
					l.Description = amount + " × " + strings.TrimSpace(l.Description)
				}
				amount, price = "1", l.Total.V
			}
		default:
			continue
		}
		if e.Source == SourceEInvoice {
			conf = Certain
		}
		rate := l.TaxRate
		if !rate.Found {
			rate = summaryRate
		}
		lines = append(lines, line{description: cmpOr(strings.TrimSpace(l.Description), description), amount: amount, price: price, rate: rate,
			reverse: reverse && isZeroRate(rate), confidence: conf})
	}
	if len(lines) > 0 {
		return lines, nil
	}
	conf := e.TotalInclTax.Confidence
	if !e.TotalInclTax.Found {
		conf = Uncertain
	}
	for _, v := range e.VAT {
		if v.Taxable == 0 && v.Tax == 0 {
			continue
		}
		price := v.Taxable
		if incl {
			price += v.Tax
		}
		lines = append(lines, line{description: description, amount: "1", price: price, rate: Value[string]{V: v.Rate, Found: true, Confidence: conf},
			reverse: v.ReverseCharge, confidence: conf})
	}
	if len(lines) > 0 {
		return lines, nil
	}
	switch {
	case e.TotalInclTax.Found:
		yes := true
		return []line{{description: description, amount: "1", price: e.TotalInclTax.V, confidence: e.TotalInclTax.Confidence}}, &yes
	case e.TotalExclTax.Found:
		no := false
		return []line{{description: description, amount: "1", price: e.TotalExclTax.V, confidence: e.TotalExclTax.Confidence}}, &no
	}
	return nil, nil
}

// MapTaxRate finds the purchase VAT rate of a percentage among rates:
// active purchase invoice rates of that percentage, preferring a domestic
// rate that shows the VAT over a reverse-charged or foreign one.
func MapTaxRate(percentage string, rates []bookkeeping.TaxRate) (resource.ID, bool) {
	want, ok := hundredths(percentage)
	if !ok {
		return 0, false
	}
	var found []bookkeeping.TaxRate
	for _, r := range rates {
		if pct, ok := hundredths(r.Percentage); ok && pct == want && usable(r) {
			found = append(found, r)
		}
	}
	if len(found) == 0 {
		return 0, false
	}
	slices.SortStableFunc(found, func(a, b bookkeeping.TaxRate) int { return rank(a) - rank(b) })
	return found[0].ID, true
}

// defaultRate is the rate a line without a known VAT rate gets: the
// standard domestic rate, the highest that shows its VAT.
func defaultRate(rates []bookkeeping.TaxRate) resource.ID {
	var best *bookkeeping.TaxRate
	bestPct := int64(-1)
	for i, r := range rates {
		pct, ok := hundredths(r.Percentage)
		if !ok || !usable(r) || rank(r) != 0 {
			continue
		}
		if pct > bestPct {
			best, bestPct = &rates[i], pct
		}
	}
	if best == nil {
		return 0
	}
	return best.ID
}

func usable(r bookkeeping.TaxRate) bool { return r.Active && r.Type == "purchase_invoice" }

// rank orders rates of one percentage: domestic rates showing VAT first,
// then those of another country, then reverse-charged ones.
func rank(r bookkeeping.TaxRate) int {
	n := 0
	if r.Country != "" {
		n++
	}
	if !r.ShowTax {
		n += 2
	}
	return n
}

// hundredths reads a percentage such as "21", "21.00" or "9,5" in
// hundredths of a percent.
func hundredths(s string) (int64, bool) {
	f, err := strconv.ParseFloat(strings.ReplaceAll(strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(s), "%")), ",", "."), 64)
	if err != nil || f < 0 || f > 100 {
		return 0, false
	}
	return int64(math.Round(f * 100)), true
}

// divRound divides and rounds half away from zero.
func roundDiv(n, d int64) int64 {
	if d < 0 {
		n, d = -n, -d
	}
	if n < 0 {
		return -((-n + d/2) / d)
	}
	return (n + d/2) / d
}

func isZeroRate(r Value[string]) bool {
	pct, ok := hundredths(r.V)
	return r.Found && ok && pct == 0
}

// supplierCountry is the supplier's country as the document tells it.
func supplierCountry(s Supplier) string {
	switch {
	case s.Country != "":
		return s.Country
	case s.VATNumber != "":
		return countryOf(s.VATNumber)
	case len(s.IBAN) >= 2:
		return s.IBAN[:2]
	}
	return ""
}

// MapReverseCharge finds the purchase rate for VAT the buyer accounts for:
// a domestic reverse charge (box 2a of the Dutch VAT return) or an
// acquisition from another EU country (box 4b), the highest percentage of
// them, which is the standard rate.
func MapReverseCharge(domestic bool, rates []bookkeeping.TaxRate) (resource.ID, bool) {
	box := "/4b"
	if domestic {
		box = "/2a"
	}
	var best *bookkeeping.TaxRate
	bestPct := int64(-1)
	for i, r := range rates {
		pct, ok := hundredths(r.Percentage)
		if !ok || !usable(r) || r.ShowTax || !slices.ContainsFunc(r.ReportBoxes, func(b string) bool { return strings.HasSuffix(b, box) }) {
			continue
		}
		if pct > bestPct {
			best, bestPct = &rates[i], pct
		}
	}
	if best == nil {
		return 0, false
	}
	return best.ID, true
}

// isFileName reports whether a reference is only the name of an uploaded
// file, which an upload gets until its own reference is known.
func isFileName(reference string) bool { return purchases.IsFileName(reference) }
