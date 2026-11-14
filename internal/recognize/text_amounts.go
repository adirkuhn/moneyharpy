package recognize

import (
	"regexp"
	"strconv"
	"strings"

	"github.com/adirkuhn/moneyharpy/internal/money"
)

// amountHit is an amount printed on a line, with its byte span.
type amountHit struct {
	v          money.Amount
	start, end int
}

// amountPattern matches "1.234,56", "1,234.56", "4 321,09", "€ 12,50",
// "-2,00", "(3.00)", "12,50-" and Dutch whole amounts "12,-". Amounts need
// decimals so that quantities, years and numbers are not taken for money. A
// space groups thousands only as one space before exactly three digits, so
// amounts in neighbouring columns stay apart.
var amountPattern = regexp.MustCompile(`(?:([-−])\s?)?(\()?(?:(?:EUR|USD|GBP|CHF|€|\$|£)\s?)?([-−])?(\d{1,3}(?:[.,'’]\d{3})+|\d{1,3}(?: \d{3})+|\d+)(?:([.,])(\d{2})|(,-))(\))?(-)?`)

// findAmounts returns the amounts printed on a line, left to right.
func findAmounts(s string) []amountHit {
	if !mayHoldAmount(s) {
		return nil
	}
	var out []amountHit
	for _, m := range amountPattern.FindAllStringSubmatchIndex(s, -1) {
		start, end := m[0], m[1]
		if start > 0 && (isDigit(s[start-1]) || s[start-1] == '.' || s[start-1] == ',') {
			continue
		}
		if end < len(s) && isDigit(s[end]) {
			continue
		}
		if end+1 < len(s) && (s[end] == '.' || s[end] == ',') && isDigit(s[end+1]) {
			continue
		}
		if rest := strings.TrimLeft(s[end:], " "); strings.HasPrefix(rest, "%") {
			continue
		}
		whole := group(s, m, 4)
		dec := group(s, m, 5)
		if dec != "" && strings.Contains(whole, dec) {
			continue // "1.234.56"
		}
		digits := strings.Map(func(r rune) rune {
			if r >= '0' && r <= '9' {
				return r
			}
			return -1
		}, whole)
		frac := group(s, m, 6)
		if frac == "" {
			frac = "00"
		}
		cents, err := strconv.ParseInt(digits+frac, 10, 64)
		if err != nil || cents > int64(1_000_000_000_00) {
			continue
		}
		neg := group(s, m, 1) != "" || group(s, m, 3) != "" ||
			group(s, m, 2) != "" && group(s, m, 8) != "" ||
			group(s, m, 9) != "" && (end >= len(s) || !isDigit(s[end]))
		if group(s, m, 2) != "" && group(s, m, 8) == "" {
			start = m[2*2+1] // an unmatched "(" is not part of the amount
		}
		if group(s, m, 8) != "" && group(s, m, 2) == "" {
			end = m[2*8] // an unmatched ")" is not part of the amount
		}
		v := money.Amount(cents)
		if neg {
			v = -v
		}
		out = append(out, amountHit{v: v, start: start, end: end})
	}
	return out
}

// mayHoldAmount tells quickly whether s may print an amount: a digit, a
// decimal separator and two digits ("12,50"), or a Dutch "12,-".
func mayHoldAmount(s string) bool {
	for i := 1; i+1 < len(s); i++ {
		if s[i] != '.' && s[i] != ',' || !isDigit(s[i-1]) {
			continue
		}
		if s[i+1] == '-' || i+2 < len(s) && isDigit(s[i+1]) && isDigit(s[i+2]) {
			return true
		}
	}
	return false
}

func group(s string, m []int, i int) string {
	if m[2*i] < 0 {
		return ""
	}
	return s[m[2*i]:m[2*i+1]]
}

func isDigit(c byte) bool { return c >= '0' && c <= '9' }

var percentPattern = regexp.MustCompile(`(\d{1,2}(?:[.,]\d{1,2})?)\s?%`)

// findRate returns the first VAT-like percentage on a line, normalised
// ("21", "5.5"), or "".
func findRate(s string) string {
	m := percentPattern.FindStringSubmatch(s)
	if m == nil {
		return ""
	}
	return rateString(m[1])
}

// rateBP is a percentage in basis points: "21" is 2100, "5.5" is 550.
func rateBP(rate string) (int64, bool) {
	whole, frac, _ := strings.Cut(rate, ".")
	w, err := strconv.ParseInt(whole, 10, 64)
	if err != nil {
		return 0, false
	}
	f := int64(0)
	if frac != "" {
		frac = (frac + "00")[:2]
		if f, err = strconv.ParseInt(frac, 10, 64); err != nil {
			return 0, false
		}
	}
	return w*100 + f, true
}

// divRound divides rounding half away from zero.
func divRound(a, b int64) int64 {
	if (a < 0) != (b < 0) {
		return (a - b/2) / b
	}
	return (a + b/2) / b
}

func abs(a money.Amount) money.Amount {
	if a < 0 {
		return -a
	}
	return a
}

// near tells whether two amounts are equal within tol cents.
func near(a, b, tol money.Amount) bool { return abs(a-b) <= tol }

// vatTolerance allows for VAT rounded per line instead of over the total.
func vatTolerance(taxable money.Amount) money.Amount { return 2 + abs(taxable)/2000 }

// taxFits tells whether tax is rate percent of taxable.
func taxFits(taxable, tax money.Amount, rate string) bool {
	bp, ok := rateBP(rate)
	if !ok {
		return false
	}
	return near(money.Amount(divRound(int64(taxable)*bp, 10000)), tax, vatTolerance(taxable))
}

// taxPair finds, among amounts printed with a VAT rate, the taxable amount and
// the tax: either "taxable … tax" or "incl … tax".
func taxPair(rate string, amounts []money.Amount) (taxable, tax money.Amount, ok bool) {
	bp, okRate := rateBP(rate)
	if !okRate {
		return 0, 0, false
	}
	for i, a := range amounts {
		for j, b := range amounts {
			if i == j || a == 0 && bp != 0 {
				continue
			}
			if taxFits(a, b, rate) && (bp == 0 || abs(a) > abs(b)) {
				return a, b, true
			}
			inclTax := money.Amount(divRound(int64(a)*bp, 10000+bp))
			if bp != 0 && near(inclTax, b, vatTolerance(a)) && abs(a) > abs(b) {
				return a - b, b, true
			}
		}
	}
	return 0, 0, false
}

// commonRates are the VAT rates used to guess a rate from totals.
var commonRates = []string{"21", "9", "19", "7", "20", "23", "6", "5.5", "10", "22", "25", "24", "8.1", "17", "13", "12", "5", "0"}

// splitDecimal is an amount OCR printed with a stray space around its
// decimal separator: "36 . 30", "36 ,30", "36, 30".
var splitDecimal = regexp.MustCompile(`(\d{1,6})( ?)([.,])( ?)(\d{2})`)

// amountAhead is an amount, as the next column after a split one.
var amountAhead = regexp.MustCompile(`^ ?(?:\d{1,6} ?[.,] ?\d{2}|\d{1,6},-)(?:\s|$)`)

// joinSplitDecimals removes the stray spaces OCR reads around the decimal
// separator of an amount. Only a whole number standing alone and followed
// by the end of the line, a wide gap, a currency or another amount is
// joined, so times ("11 . 32 . 24"), dates, counts in a sentence ("3, 12
// artikelen") and card numbers stay as they are.
func joinSplitDecimals(l string) string {
	locs := splitDecimal.FindAllStringSubmatchIndex(l, -1)
	if len(locs) == 0 {
		return l
	}
	var b strings.Builder
	last := 0
	for _, m := range locs {
		if m[2*2] == m[2*2+1] && m[2*4] == m[2*4+1] {
			continue // no stray space
		}
		if !standsAlone(l, m[0]) || !endsAmount(l[m[1]:]) {
			continue
		}
		b.WriteString(l[last:m[0]])
		b.WriteString(group(l, m, 1) + group(l, m, 3) + group(l, m, 5))
		last = m[1]
	}
	b.WriteString(l[last:])
	return b.String()
}

// standsAlone tells whether the number at i starts a word that is not part
// of a longer run of numbers, such as a card number printed in groups, a
// date or a time.
func standsAlone(l string, i int) bool {
	if i == 0 {
		return true
	}
	switch {
	case l[i-1] == '(' || strings.HasSuffix(l[:i], "€"):
		return true
	case l[i-1] != ' ':
		return false
	}
	before := strings.TrimRight(l[:i], " ")
	if before == "" || amountBehind.MatchString(before) {
		return true // the next column after an amount
	}
	c := before[len(before)-1]
	return !isDigit(c) && !strings.ContainsRune(".,:-/*", rune(c))
}

// amountBehind is an amount ending a text, as the column before another.
var amountBehind = regexp.MustCompile(`(?:^|\s)\d{1,6} ?[.,] ?\d{2}$`)

// endsAmount tells whether rest, the text after a split amount, lets it be
// one: the line ends, a wide gap or a currency follows, or another amount.
func endsAmount(rest string) bool {
	switch {
	case strings.TrimSpace(rest) == "", strings.HasPrefix(rest, "  "):
		return true
	case rest[0] != ' ':
		return false
	}
	return amountAhead.MatchString(rest) || strings.HasPrefix(rest, " EUR") || strings.HasPrefix(rest, " €")
}
