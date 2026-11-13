package recognize

import (
	"regexp"
	"strings"
	"unicode"

	"github.com/adirkuhn/moneyharpy/internal/money"
)

// column is a heading of an item table, with where it is printed.
type column struct {
	role       columnRole
	start, end int // runes
}

type columnRole int

const (
	colOther columnRole = iota
	colDescription
	colQuantity
	colPrice
	colTotal
	colRate
)

// columnNames name the columns of item tables, by what they hold. Earlier
// entries win, so "V. Líquido" is the total and "%Impostos" the rate.
var columnNames = []struct {
	role columnRole
	re   *regexp.Regexp
}{
	{colRate, regexp.MustCompile(`(?i)^%\s?(?:impostos?|iva|btw|vat|tax|mwst|tva)$|^(?:btw|vat|iva|tva|mwst|ust|taxa|tarief)(?:\s?%)?$`)},
	{colTotal, regexp.MustCompile(`(?i)^(?:v\.?\s?l[ií]quido|valor(?: l[ií]quido)?|total|totaal|bedrag|amount|line total|betrag|gesamt|montant|importe|subtotal)\b`)},
	{colPrice, regexp.MustCompile(`(?i)^(?:pr\.?\s?unit\.?|pre[cç]o|unit price|price|rate|prijs|stuksprijs|eenheidsprijs|einzelpreis|preis|prix|p\.?\s?u\.?|precio)(?:\b|$)`)},
	{colQuantity, regexp.MustCompile(`(?i)^(?:qtd\.?|qty|quantity|quantidade|aantal|menge|anzahl|qt[ée]\.?|cant\.?|cantidad|aant\.?)$`)},
	{colDescription, regexp.MustCompile(`(?i)^(?:descri[cç][aã]o|omschrijving|description|bezeichnung|beschreibung|d[ée]signation|descripci[oó]n|artikel|product|produkt|item)$`)},
}

// cell is text on a line between runs of two or more spaces.
type cell struct {
	text       string
	start, end int // runes
}

func cells(line string) []cell {
	r := []rune(line)
	var out []cell
	start := -1
	for i := 0; i <= len(r); i++ {
		gap := i == len(r) || r[i] == ' ' && (i+1 == len(r) || r[i+1] == ' ')
		switch {
		case !gap && start < 0:
			start = i
		case gap && start >= 0:
			for start < i && r[start] == ' ' {
				start++
			}
			out = append(out, cell{text: string(r[start:i]), start: start, end: i})
			start = -1
		}
	}
	return out
}

// tableColumns reads a heading row of an item table: it needs a description,
// a total and a quantity or price column.
func tableColumns(line string) ([]column, bool) {
	var cols []column
	has := map[columnRole]bool{}
	for _, c := range cells(line) {
		role := colOther
		name := strings.TrimSpace(c.text)
		for _, n := range columnNames {
			if n.re.MatchString(name) {
				role = n.role
				break
			}
		}
		has[role] = true
		cols = append(cols, column{role: role, start: c.start, end: c.end})
	}
	return cols, has[colDescription] && has[colTotal] && (has[colQuantity] || has[colPrice])
}

// columnOf tells under which column a cell is printed: the one it overlaps
// most, else the nearest. Text starting between the first column and the
// end of the description heading is the description: descriptions are
// printed left-aligned while their heading is often centred.
func columnOf(c cell, cols []column) columnRole {
	for _, col := range cols {
		if col.role == colDescription && len(cols) > 0 && c.start >= cols[0].end && c.start < col.end &&
			strings.ContainsFunc(c.text, unicode.IsLetter) {
			return colDescription
		}
	}
	best, bestScore := colOther, -1<<30
	for _, col := range cols {
		overlap := min(c.end, col.end) - max(c.start, col.start)
		if overlap <= 0 {
			// distance as a negative score
			if c.end <= col.start {
				overlap = c.end - col.start
			} else {
				overlap = col.end - c.start
			}
		}
		if overlap > bestScore {
			best, bestScore = col.role, overlap
		}
	}
	return best
}

// decimalPattern finds a number as printed in a column: "20,00", "5,100",
// "1.234,56"; the last separator is read as the decimal one.
var decimalPattern = regexp.MustCompile(`-?\d[\d .,]*`)

// columnNumber reads the leading number of a cell, normalised with a dot
// ("5,100" is "5.100"), and whether there was one.
func columnNumber(s string) (string, bool) {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "(1)") // a footnote mark before a VAT rate
	m := decimalPattern.FindString(s)
	m = strings.TrimRight(strings.TrimSpace(m), ".,")
	if m == "" {
		return "", false
	}
	m = strings.ReplaceAll(m, " ", "")
	last := strings.LastIndexAny(m, ".,")
	if last < 0 {
		return m, true
	}
	whole := strings.NewReplacer(".", "", ",", "").Replace(m[:last])
	return whole + "." + m[last+1:], true
}

// centsOf turns a normalised decimal into an amount when it has no more
// than two significant decimals ("5.100" is 5.10, "2.125" is not exact).
func centsOf(n string) (money.Amount, bool) {
	whole, frac, _ := strings.Cut(n, ".")
	frac = strings.TrimRight(frac, "0")
	if len(frac) > 2 {
		return 0, false
	}
	a, err := money.ParseAmount(whole + "." + (frac + "00")[:2])
	return a, err == nil
}

// readTable reads the item lines of a table with a heading row of columns,
// such as "Descrição … Qtd. … Pr. Unit. … V. Líquido … %Impostos": each row
// with a total is a line, and a row of only text right below one continues
// its description. The quantity and unit price are kept when they make the
// total; otherwise (a discount) the line keeps its quantity and total and
// has no price.
func readTable(lines []string, kinds []lineKind) []Line {
	head, cols := -1, []column(nil)
	for i, l := range lines {
		if kinds[i] != lineOther || len(findAmounts(l)) > 0 {
			continue
		}
		if c, ok := tableColumns(l); ok {
			head, cols = i, c
			break
		}
	}
	if head < 0 {
		return nil
	}
	var out []Line
	blank, continues := 0, false
	for i := head + 1; i < len(lines); i++ {
		if kinds[i] != lineOther {
			break
		}
		if strings.TrimSpace(lines[i]) == "" {
			blank++
			continues = false
			if blank >= 3 && len(out) > 0 {
				break
			}
			continue
		}
		blank = 0
		var desc []string
		var qty, price, total, rate string
		for _, c := range cells(lines[i]) {
			switch columnOf(c, cols) {
			case colDescription:
				desc = append(desc, strings.TrimSpace(c.text))
			case colQuantity:
				qty = c.text
			case colPrice:
				price = c.text
			case colTotal:
				total = c.text
			case colRate:
				rate = c.text
			}
		}
		hits := findAmounts(total)
		if len(hits) == 0 {
			// Only text under the description: the row above goes on.
			if continues && len(desc) > 0 && qty == "" && price == "" {
				last := &out[len(out)-1]
				last.Description = strings.Join(append([]string{last.Description}, desc...), " ")
			}
			continues = false
			continue
		}
		l := Line{Description: strings.Join(strings.Fields(strings.Join(desc, " ")), " "), Total: sure(hits[len(hits)-1].v)}
		if q, ok := columnNumber(qty); ok {
			l.Quantity = sure(normalizeQuantity(q))
		} else {
			l.Quantity = sure("1")
		}
		if p, ok := columnNumber(price); ok {
			if a, ok := centsOf(p); ok && qtyFits(l.Quantity.V, a, l.Total.V) {
				l.Price = sure(a)
			}
		}
		if r, ok := columnNumber(rate); ok {
			l.TaxRate = sure(rateString(r))
		}
		if l.Description == "" {
			continues = false
			continue
		}
		out = append(out, l)
		continues = true
	}
	return out
}
