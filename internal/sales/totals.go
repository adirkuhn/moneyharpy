package sales

import (
	"errors"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/adirkuhn/moneyharpy/internal/money"
	"github.com/adirkuhn/moneyharpy/internal/resource"
)

var (
	quantityPattern   = regexp.MustCompile(`^\s*(-?\d{1,9})(?:[.,](\d{1,3}))?(?:\s.*)?$`)
	percentagePattern = regexp.MustCompile(`^(\d{1,3})(?:[.,](\d{1,2}))?$`)
	errQuantity       = errors.New("is not a quantity")
	errPercentage     = errors.New("is not a percentage")
)

// ParseQuantity reads a line's amount, such as "2", "1,5" or "3 hours", in
// thousandths; Moneybird keeps the text and reads the number in front.
func ParseQuantity(s string) (int64, error) {
	m := quantityPattern.FindStringSubmatch(s)
	if m == nil || strings.ContainsAny(s, "eE") && !strings.Contains(s, " ") {
		return 0, errQuantity
	}
	whole, _ := strconv.ParseInt(m[1], 10, 64)
	frac := m[2] + strings.Repeat("0", 3-len(m[2]))
	f, _ := strconv.ParseInt(frac, 10, 64)
	if strings.HasPrefix(m[1], "-") {
		return whole*1000 - f, nil
	}
	return whole*1000 + f, nil
}

// ParsePercentage reads a percentage such as "21" or "9.5" in hundredths of
// a percent.
func ParsePercentage(s string) (int64, error) {
	m := percentagePattern.FindStringSubmatch(strings.TrimSpace(s))
	if m == nil {
		return 0, errPercentage
	}
	whole, _ := strconv.ParseInt(m[1], 10, 64)
	frac := m[2] + strings.Repeat("0", 2-len(m[2]))
	f, _ := strconv.ParseInt(frac, 10, 64)
	p := whole*100 + f
	if p > 10000 {
		return 0, errPercentage
	}
	return p, nil
}

// divRound divides and rounds half away from zero.
func divRound(n, d int64) int64 {
	if d < 0 {
		n, d = -n, -d
	}
	if n < 0 {
		return -((-n + d/2) / d)
	}
	return (n + d/2) / d
}

// Line is what totals need of an invoice line.
type Line struct {
	Price    money.Amount
	Quantity int64 // thousandths
	// TaxRateID groups lines for VAT; Rate is its percentage in hundredths.
	TaxRateID resource.ID
	Rate      int64
	// Optional lines count only when selected.
	Optional, Selected bool
}

// RateTotal is the VAT of the lines with one tax rate.
type RateTotal struct {
	TaxRateID resource.ID
	Taxable   money.Amount
	Tax       money.Amount
}

// Totals are an invoice's amounts. LineBases holds each line's amount
// excluding VAT after discount, in line order; they add up to ExclTax.
type Totals struct {
	ExclTax, Tax, InclTax money.Amount
	// Discount is what the discount took off, before VAT when prices are
	// exclusive and including it when they are inclusive.
	Discount  money.Amount
	ByRate    []RateTotal
	LineBases []money.Amount
}

// Compute works out the totals. discount is a percentage in hundredths.
// VAT is computed per tax rate on the summed lines, as Moneybird's
// tax_totals show it, so rounding differences do not pile up per line.
func Compute(lines []Line, discount int64, inclTax bool) Totals {
	tot := Totals{LineBases: make([]money.Amount, len(lines))}
	amounts := make([]int64, len(lines))
	groups := map[resource.ID][]int{}
	var order []resource.ID
	for i, l := range lines {
		if l.Optional && !l.Selected {
			continue
		}
		gross := divRound(int64(l.Price)*l.Quantity, 1000)
		net := gross - divRound(gross*discount, 10000)
		tot.Discount += money.Amount(gross - net)
		amounts[i] = net
		if _, seen := groups[l.TaxRateID]; !seen {
			order = append(order, l.TaxRateID)
		}
		groups[l.TaxRateID] = append(groups[l.TaxRateID], i)
	}
	for _, rate := range order {
		idx := groups[rate]
		bp := lines[idx[0]].Rate
		var sum int64
		for _, i := range idx {
			sum += amounts[i]
		}
		var base, tax int64
		if inclTax {
			base = divRound(sum*10000, 10000+bp)
			tax = sum - base
			// Spread the base over the lines; the last takes the rounding.
			var spread int64
			for n, i := range idx {
				if n == len(idx)-1 {
					tot.LineBases[i] = money.Amount(base - spread)
					break
				}
				b := divRound(amounts[i]*10000, 10000+bp)
				tot.LineBases[i] = money.Amount(b)
				spread += b
			}
		} else {
			base = sum
			tax = divRound(sum*bp, 10000)
			for _, i := range idx {
				tot.LineBases[i] = money.Amount(amounts[i])
			}
		}
		tot.ByRate = append(tot.ByRate, RateTotal{TaxRateID: rate, Taxable: money.Amount(base), Tax: money.Amount(tax)})
		tot.ExclTax += money.Amount(base)
		tot.Tax += money.Amount(tax)
	}
	tot.InclTax = tot.ExclTax + tot.Tax
	slices.SortStableFunc(tot.ByRate, func(a, b RateTotal) int { return int(a.TaxRateID - b.TaxRateID) })
	return tot
}
