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
		if discountSummary(lines[i], hits) {
			continue
		}
		prev := ""
		if i > start && len(findAmounts(lines[i-1])) == 0 {
			prev = strings.Join(strings.Fields(lines[i-1]), " ")
		}
		if l, ok := promoLine(prev, lines[i], hits); ok {
			out = append(out, l)
			continue
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

var (
	// promoNote is a price note printed under an item without an amount:
	// "Van 6.48 voor 5,00 25,00" (was 6.48, now 5.00), "2 voor 3,00 3,00",
	// "Korting 0,50 4,49".
	promoNote = regexp.MustCompile(`(?i)^\s*(?:(?:van|was|statt|de)\s+\d+[.,]\d{2}\s+(?:voor|now|nu|f[üu]r|por|pour)\s+(\d+[.,]\d{2})|\d+\s*(?:voor|for|f[üu]r|pour|por)\s+\d+[.,]\d{2}|(?:actie)?korting|actie|aanbieding|promo(?:tie|tion)?|rabatt|aktion|desconto|remise)\b`)
	// discountWords are a receipt's summary of the discounts already in
	// the line prices: "Actiekorting 7,40", "Uw voordeel 7,40".
	discountWords = regexp.MustCompile(`(?i)^\s*(?:totale?\s+|uw\s+|je\s+)?(?:actie)?(?:korting(?:en)?|voordeel|bespaard|besparing)(?:\s+totaal)?\s*:?\s*$|^\s*(?:u|je)\s+(?:heeft|hebt)\s+.*bespaard`)
)

// discountSummary tells whether a line sums up the discounts given, as a
// positive amount: they are already in the lines' prices.
func discountSummary(s string, hits []amountHit) bool {
	return len(hits) == 1 && hits[0].v > 0 && discountWords.MatchString(s[:hits[0].start])
}

// promoLine reads an item whose amount is printed on a price note under it
// (note): its description and leading quantity are the item's, its total
// the note's last amount and its price the note's new price when the
// quantity times it makes the total.
func promoLine(item, note string, hits []amountHit) (Line, bool) {
	m := promoNote.FindStringSubmatch(note)
	if item == "" || m == nil || !strings.ContainsFunc(item, isLetterRune) || itemHeader.MatchString(item) {
		return Line{}, false
	}
	qty, desc := "1", item
	if q := leadingQty.FindStringSubmatch(item); q != nil {
		qty, desc = q[1], q[2]
	}
	total := hits[len(hits)-1].v
	l := Line{Description: desc, Quantity: sure(normalizeQuantity(qty)), Total: sure(total)}
	if m[1] != "" {
		if p, err := money.ParseAmount(strings.ReplaceAll(m[1], ",", ".")); err == nil && qtyFits(qty, p, total) {
			l.Price = sure(p)
			return l, true
		}
	}
	if q, err := money.ParseAmount(strings.ReplaceAll(qty, ",", ".")); err == nil && q > 0 {
		if p := money.Amount(divRound(int64(total)*100, int64(q))); qtyFits(qty, p, total) {
			l.Price = sure(p)
		}
	}
	return l, true
}

func isLetterRune(r rune) bool {
	return r >= 'A' && r <= 'Z' || r >= 'a' && r <= 'z' || r > 127 && r != '€' && r != '£'
}
