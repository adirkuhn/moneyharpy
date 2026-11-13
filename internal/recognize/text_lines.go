package recognize

import (
	"regexp"
	"strings"

	"github.com/adirkuhn/moneyharpy/internal/money"
)

var (
	trailingQty = regexp.MustCompile(`^(.*?\S)\s+(\d{1,5}(?:[.,]\d{1,3})?)\s*(?:x|×|st\.?|stk\.?|pcs)?$`)
	leadingQty  = regexp.MustCompile(`^(\d{1,3}(?:[.,]\d{1,3})?)\s*(?:x|×|st\.?|stk\.?)?\s+(\p{L}.*)$`)
	articleCode = regexp.MustCompile(`^\d{4,}\s+`)
	position    = regexp.MustCompile(`^\d{1,3}\.?\s+(\p{L}.*)$`)
	currencyTok = regexp.MustCompile(`(?:EUR|USD|GBP|CHF|€|\$|£)\s*$`)
	columnGap   = regexp.MustCompile(`^[\s€$£x×@*%0-9.,]*$`)
)

// readLines reads the item lines: lines with amounts after the item header
// (if any) and before the first total, VAT or payment line.
func readLines(lines []string, kinds []lineKind) []Line {
	start := 0
	for i, l := range lines {
		if kinds[i] != lineOther {
			break
		}
		if itemHeader.MatchString(l) && len(findAmounts(l)) == 0 {
			start = i + 1
			break
		}
	}
	var out []Line
	for i := start; i < len(lines); i++ {
		if kinds[i] != lineOther {
			break
		}
		hits := findAmounts(lines[i])
		if len(hits) == 0 || len(FindIBANs(lines[i])) > 0 {
			continue
		}
		prev := ""
		if i > start && len(findAmounts(lines[i-1])) == 0 {
			prev = strings.Join(strings.Fields(lines[i-1]), " ")
		}
		if l, ok := itemLine(lines[i], hits, prev); ok {
			out = append(out, l)
		}
	}
	return out
}

// itemLine reads "description [qty] [price] [rate%] total".
func itemLine(s string, hits []amountHit, prev string) (Line, bool) {
	var l Line
	if r := findRate(s); r != "" {
		l.TaxRate = sure(r)
	}
	last := hits[len(hits)-1]
	total := last.v
	pre := strings.TrimSpace(currencyTok.ReplaceAllString(s[:hits[0].start], ""))
	if !strings.ContainsFunc(pre, isLetterRune) {
		// "42,17 l x 1,999 EUR/l  84,30" under "Pomp 4 Euro 95": the
		// description is the line above.
		if prev == "" {
			return Line{}, false
		}
		pre = prev
		hits = hits[len(hits)-1:]
	}

	// Amounts inside the description ("4 uur à € 85,00) € 340,00"): only the
	// last one is the line's.
	column := true
	for k := 0; k+1 < len(hits); k++ {
		gap := currencyTok.ReplaceAllString(s[hits[k].end:hits[k+1].start], "")
		if !columnGap.MatchString(gap) {
			column = false
		}
	}
	if !column {
		pre = strings.TrimSpace(currencyTok.ReplaceAllString(s[:last.start], ""))
		hits = hits[len(hits)-1:]
	}
	qty, desc := "", pre
	switch {
	case len(hits) >= 2:
		price := hits[0].v
		if m := trailingQty.FindStringSubmatch(pre); m != nil && qtyFits(m[2], price, total) {
			qty, desc = m[2], m[1]
			if m := position.FindStringSubmatch(desc); m != nil {
				desc = m[1]
			}
		} else if m := leadingQty.FindStringSubmatch(pre); m != nil && qtyFits(m[1], price, total) {
			qty, desc = m[1], m[2]
		}
		if qty == "" {
			if price == total {
				qty = "1"
			} else if price != 0 && total%price == 0 {
				qty = money.Amount(total / price).String()
				qty = strings.TrimSuffix(qty, ".0")
				l.Quantity = unsure(qty)
			} else {
				qty, price = "1", total
				l.Price = unsure(price)
			}
		}
		if !l.Price.Found {
			l.Price = sure(price)
		}
	default:
		if m := leadingQty.FindStringSubmatch(pre); m != nil {
			qty, desc = m[1], m[2]
		} else {
			qty = "1"
		}
		q, _ := money.ParseAmount(strings.ReplaceAll(qty, ",", "."))
		if q <= 0 {
			q, qty = 100, "1"
		}
		price := money.Amount(divRound(int64(total)*100, int64(q)))
		l.Price = sure(price)
		if money.Amount(divRound(int64(price)*int64(q), 100)) != total {
			l.Price.Confidence = Uncertain
		}
	}
	desc = articleCode.ReplaceAllString(desc, "")
	l.Description = strings.Join(strings.Fields(strings.TrimRight(desc, " :-")), " ")
	if !l.Quantity.Found {
		l.Quantity = sure(normalizeQuantity(qty))
	}
	l.Total = sure(total)
	return l, l.Description != ""
}

// qtyFits tells whether qty × price = total (to the cent).
func qtyFits(qty string, price, total money.Amount) bool {
	q, err := money.ParseAmount(strings.ReplaceAll(qty, ",", "."))
	if err != nil || q <= 0 {
		return false
	}
	return near(money.Amount(divRound(int64(q)*int64(price), 100)), total, 1)
}

func isLetterRune(r rune) bool {
	return r >= 'A' && r <= 'Z' || r >= 'a' && r <= 'z' || r > 127 && r != '€' && r != '£'
}
