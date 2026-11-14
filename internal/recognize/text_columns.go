package recognize

import (
	"math"
	"regexp"
	"strconv"
	"strings"
	"unicode"

	"github.com/adirkuhn/moneyharpy/internal/money"
)

// A column table is an item table whose rows the layout keeps in columns,
// as pdftotext -layout prints them, but whose headings are not where its
// values are: wide tables, headings on one line with single spaces, rows
// over several pages. Its rows are found by their numbers: a quantity
// times a unit price makes the line total.

// tableHead is the heading row of a column table: where its VAT and
// discount columns are printed (rune offsets, -1 for none).
type tableHead struct {
	vat, discount int
}

var (
	qtyHeadings      = words("menge", "quant", "quant.", "quantidade", "quantity", "qty", "qty.", "qtd", "qtd.", "aantal", "aant", "aant.", "cantidad", "cant", "cant.", "qté", "qte", "quantité", "stück", "stk", "stk.")
	priceHeadings    = words("preis", "einzelpreis", "e-preis", "p.einheit", "preço", "preco", "prijs", "stuksprijs", "eenheidsprijs", "price", "prix", "precio", "p.unit.", "p.unit", "pr.unit.")
	totalHeadings    = words("gesamtpreis", "gesamt", "gesamtbetrag", "betrag", "valor", "total", "totaal", "bedrag", "amount", "montant", "importe", "líquido", "liquido", "subtotal")
	vatHeadings      = words("iva", "%iva", "iva%", "mwst", "mwst.", "ust", "btw", "btw%", "vat", "vat%", "tva", "tva%", "steuer")
	discountHeadings = words("desc", "desc.", "%desc.", "desc.%", "rabatt", "korting", "discount", "remise", "dto", "dto.", "dto%")
)

func words(ws ...string) map[string]bool {
	m := map[string]bool{}
	for _, w := range ws {
		m[w] = true
	}
	return m
}

// field is a word of a line with the rune offset it starts at.
type field struct {
	text  string
	start int
}

// fields splits a line into its words.
func fields(line string) []field {
	var out []field
	rs := []rune(line)
	start := -1
	for i := 0; i <= len(rs); i++ {
		space := i == len(rs) || unicode.IsSpace(rs[i])
		switch {
		case !space && start < 0:
			start = i
		case space && start >= 0:
			out = append(out, field{text: string(rs[start:i]), start: start})
			start = -1
		}
	}
	return out
}

// columnHeader reads the heading row of a column table: it names a
// quantity, a unit price and a line total column, in any of the languages
// suppliers write, and prints no amounts.
func columnHeader(line string) (tableHead, bool) {
	h := tableHead{vat: -1, discount: -1}
	if len(findAmounts(line)) > 0 {
		return h, false
	}
	var qty, price, total bool
	for _, f := range fields(line) {
		w := strings.TrimSuffix(strings.ToLower(f.text), ":")
		switch {
		case qtyHeadings[w] || strings.HasPrefix(w, "anzahl"):
			qty = true
		case priceHeadings[w]:
			price = true
		case totalHeadings[w]:
			total = true
		case vatHeadings[w]:
			h.vat = f.start
		case discountHeadings[w]:
			h.discount = f.start
		}
	}
	return h, qty && price && total
}

// metadataLine is a line under an item with its batch, best-before date,
// barcode or weight, which is no part of its description.
var metadataLine = regexp.MustCompile(`(?i)^\s*(?:mhd|ean|gtin|lote?|lot|validade|gewicht|peso|weight|charge|batch|tht|houdbaar|best before|n\.?º? ?s[ée]rie)\b`)

// number is a number printed in a column, with its readings in
// ten-thousandths: "1,450" may be 1.45 or 1450.
type number struct {
	vals     []int64
	decimals []int
	start    int
}

// numberToken reads a number as a column prints it: "44,40", "1.234,56",
// "1,234.56", "1,450 €", "24". A single separator before three digits may
// group thousands or mark three decimals, so such a number has both
// readings. Codes ("512-1", "350g"), dates, barcodes and percentages are
// no numbers.
func numberToken(s string) (number, bool) {
	s = strings.TrimSuffix(strings.TrimPrefix(s, "€"), "€")
	neg := strings.HasPrefix(s, "-")
	s = strings.TrimPrefix(s, "-")
	if s == "" || !isDigit(s[0]) || !isDigit(s[len(s)-1]) {
		return number{}, false
	}
	var seps []int
	for i := range len(s) {
		switch {
		case s[i] == '.' || s[i] == ',':
			seps = append(seps, i)
		case !isDigit(s[i]):
			return number{}, false
		}
	}
	var n number
	add := func(decimals int) {
		digits := strings.NewReplacer(".", "", ",", "").Replace(s)
		v, err := strconv.ParseInt(digits, 10, 64)
		if err != nil || len(digits) > 12 {
			return
		}
		for range 4 - decimals {
			v *= 10
		}
		if neg {
			v = -v
		}
		n.vals, n.decimals = append(n.vals, v), append(n.decimals, decimals)
	}
	if len(seps) == 0 {
		if len(s) <= 7 { // longer is a barcode or code
			add(0)
		}
		return n, len(n.vals) > 0
	}
	last := seps[len(seps)-1]
	head, frac := s[:last], s[last+1:]
	switch groups := seps[:len(seps)-1]; {
	case len(groups) > 0 && !thousandGroups(head, groups):
	case len(groups) > 0 && s[groups[0]] == s[last]:
		if len(frac) == 3 { // "1.234.567": all thousands
			add(0)
		}
	case len(groups) > 0 && len(frac) <= 4: // "1.234,56"
		add(len(frac))
	case len(frac) == 3: // "1,450": three decimals, or thousands
		add(3)
		if len(head) <= 3 {
			add(0)
		}
	case len(frac) <= 4:
		add(len(frac))
	}
	return n, len(n.vals) > 0
}

// thousandGroups tells whether the separators seps of head, all alike, each
// start a group of three digits after a first group of one to three, the
// last group ending head.
func thousandGroups(head string, seps []int) bool {
	prev := -1
	for k, i := range seps {
		if k == 0 && (i < 1 || i > 3) || k > 0 && i-prev != 4 || head[i] != head[seps[0]] {
			return false
		}
		prev = i
	}
	return len(head)-prev == 4
}

// decimalString writes ten-thousandths as a decimal without trailing
// zeros: 125000 is "12.5".
func decimalString(v int64) string {
	s := strconv.FormatInt(v/10000, 10)
	if frac := strings.TrimRight(strconv.FormatInt(abs64(v%10000)+10000, 10)[1:], "0"); frac != "" {
		s += "." + frac
	}
	if v < 0 && v > -10000 {
		s = "-" + s
	}
	return s
}

func abs64(v int64) int64 {
	if v < 0 {
		return -v
	}
	return v
}

// percent is a percentage printed in a row, in hundredths.
type percent struct {
	bp    int64
	text  string
	start int
}

// match is a quantity, unit price and line total among a row's numbers.
type match struct {
	qty, price, total int64 // ten-thousandths
	discounted        bool
	score             int
}

// columnRow reads a row of a column table: its description is the first
// cell with words after the codes before it (position, article number,
// lot, dates); after it, a quantity times a unit price makes the line
// total, the right-most one that does. It also returns the rune offset
// where the description is printed.
func columnRow(line string, h tableHead) (Line, int, bool) {
	if metadataLine.MatchString(line) {
		return Line{}, 0, false
	}
	cs := cells(line)
	desc, at, next := description(cs)
	if desc == "" {
		return Line{}, 0, false
	}
	var nums []number
	var pcts []percent
	for _, c := range cs[next:] {
		for _, f := range fields(c.text) {
			f.start += c.start
			if p, ok := strings.CutSuffix(f.text, "%"); ok {
				if bp, ok := hundredths(p); ok {
					pcts = append(pcts, percent{bp: bp, text: p, start: f.start})
				}
				continue
			}
			if n, ok := numberToken(f.text); ok {
				n.start = f.start
				nums = append(nums, n)
			}
		}
	}
	rate, discounts := splitPercents(pcts, h)
	m, ok := findMatch(nums, discounts)
	if !ok {
		return Line{}, 0, false
	}
	qty := decimalString(m.qty)
	l := Line{Description: desc, Quantity: sure(qty), Total: sure(money.Amount(m.total / 100))}
	if price := money.Amount(m.price / 100); !m.discounted && m.price%100 == 0 && qtyFits(qty, price, l.Total.V) {
		l.Price = sure(price)
	}
	if rate != "" {
		l.TaxRate = sure(rateString(rate))
	}
	return l, at, true
}

// description finds a row's description: the first cell that has words
// once the codes printed before them are left out. It returns the
// description, where it is printed and the index of the next cell.
func description(cs []cell) (string, int, int) {
	for i, c := range cs {
		fs := fields(c.text)
		for len(fs) > 0 && strings.ContainsFunc(fs[0].text, unicode.IsDigit) {
			fs = fs[1:]
		}
		if len(fs) == 0 || !strings.ContainsFunc(fs[0].text, unicode.IsLetter) {
			continue
		}
		var ws []string
		letters := 0
		for _, f := range fs {
			ws = append(ws, f.text)
			for _, r := range f.text {
				if unicode.IsLetter(r) {
					letters++
				}
			}
		}
		if letters < 2 {
			continue
		}
		return strings.Join(ws, " "), c.start + fs[0].start, i + 1
	}
	return "", 0, 0
}

// splitPercents tells the VAT rate of a row, the percentage printed nearest
// the VAT column, from its discounts, the others.
func splitPercents(pcts []percent, h tableHead) (rate string, discounts []int64) {
	best := -1
	if h.vat >= 0 {
		for i, p := range pcts {
			d := absInt(p.start - h.vat)
			if (h.discount < 0 || d < absInt(p.start-h.discount)) && (best < 0 || d < absInt(pcts[best].start-h.vat)) {
				best = i
			}
		}
	}
	for i, p := range pcts {
		switch {
		case i == best:
			rate = p.text
		case p.bp > 0 && p.bp < 10000:
			discounts = append(discounts, p.bp)
		}
	}
	return rate, discounts
}

// roundingTolerance is how far quantity × price may be from the line
// total, in ten-thousandths: a cent, for prices with more decimals.
const roundingTolerance = 100

// findMatch finds the quantity, unit price and total among a row's numbers
// (left to right): the total is the right-most number with two decimals
// that a quantity times a unit price (with decimals) make, less a discount
// if need be. Of several pairs, the one printed nearest the total wins.
func findMatch(nums []number, discounts []int64) (match, bool) {
	for k := len(nums) - 1; k >= 0; k-- {
		for r, t := range nums[k].vals {
			if nums[k].decimals[r] != 2 {
				continue
			}
			if m, ok := matchTotal(nums[:k], t, discounts); ok {
				return m, true
			}
		}
	}
	return match{}, false
}

func matchTotal(nums []number, total int64, discounts []int64) (match, bool) {
	var best match
	found := false
	for i := range nums {
		for j := range nums {
			if i == j {
				continue
			}
			for qi, q := range nums[i].vals {
				for pi, p := range nums[j].vals {
					if q <= 0 || nums[j].decimals[pi] < 2 || nums[i].decimals[qi] > 3 {
						continue
					}
					m := match{qty: q, price: p, total: total, score: 2 * (i + j)}
					if i < j {
						m.score++ // a quantity before its price
					}
					product := float64(q) * float64(p) / 10000
					switch {
					case math.Abs(product-float64(total)) <= roundingTolerance:
					case discountFits(product, float64(total), discounts):
						m.discounted = true
						m.score -= 2 * len(nums) // a match without a discount first
					default:
						continue
					}
					if !found || m.score > best.score {
						best, found = m, true
					}
				}
			}
		}
	}
	return best, found
}

func discountFits(product, total float64, discounts []int64) bool {
	for _, d := range discounts {
		if math.Abs(product*float64(10000-d)/10000-total) <= roundingTolerance {
			return true
		}
	}
	return false
}

// readColumnRows reads the item lines of column tables: after a heading
// row, every row with a quantity, unit price and total, on all pages.
// Headings repeated on each page start the table again; carry-over rows
// ("Übertrag", "A transportar") and lines of totals have no such numbers
// and are left out. A line of words right under a row, printed where its
// description is, continues the description; lines with a batch,
// best-before date, barcode or weight do not.
//
// It also tells which lines are the table's: its rows and the lines under
// them. They hold no details of the document as a whole.
func readColumnRows(lines []string) ([]Line, []bool) {
	var out []Line
	table := make([]bool, len(lines))
	head, inTable := tableHead{}, false
	last, descAt := -1, 0
	for i, l := range lines {
		if h, ok := columnHeader(l); ok {
			head, inTable, last = h, true, -1
			continue
		}
		if !inTable {
			continue
		}
		if strings.TrimSpace(l) == "" {
			last = -1
			continue
		}
		if row, at, ok := columnRow(l, head); ok {
			out = append(out, row)
			last, descAt, table[i] = i, at, true
			continue
		}
		switch {
		case last < 0 || i-last > 2:
		case continuesDescription(l, descAt):
			d := &out[len(out)-1].Description
			*d += " " + strings.Join(strings.Fields(l), " ")
			last, table[i] = i, true
		case metadataLine.MatchString(l):
			table[i] = true
		}
	}
	return out, table
}

// withoutTable blanks the lines of a column table, so that the dates,
// codes and words of its rows are not read as the document's.
func withoutTable(lines []string, table []bool) []string {
	out := make([]string, len(lines))
	for i, l := range lines {
		if !table[i] {
			out[i] = l
		}
	}
	return out
}

// maxContinuationWords keeps footers and sentences from being taken for a
// description going on.
const maxContinuationWords = 8

// continuesDescription tells whether a line continues the description
// printed at descAt on the row above: words only, starting under it.
func continuesDescription(l string, descAt int) bool {
	fs := fields(l)
	return len(fs) > 0 && len(fs) <= maxContinuationWords && absInt(fs[0].start-descAt) <= 2 &&
		!metadataLine.MatchString(l) && len(findAmounts(l)) == 0 && strings.ContainsFunc(l, unicode.IsLetter)
}
