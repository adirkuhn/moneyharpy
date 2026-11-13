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
