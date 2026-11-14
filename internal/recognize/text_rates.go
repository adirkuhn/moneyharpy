package recognize

import (
	"regexp"

	"github.com/adirkuhn/moneyharpy/internal/money"
)

// depositWords name a deposit on bottles or crates, which has no VAT.
var depositWords = regexp.MustCompile(`(?i)statiegeld|emballage|leeggoed|\bpfand\b|\bdeposit\b|\bvasilhame|\bconsigne\b`)

// maxRateSearch is the most lines whose rates are worked out by trying
// every split over two rates (2^n splits).
const maxRateSearch = 18

// rateGroup is one rate of a VAT summary and the amount its lines add up
// to.
type rateGroup struct {
	rate   string
	target money.Amount
}

// assignRates gives lines without a VAT rate the rate of the VAT summary
// whose amount (incl. or excl. VAT) their totals add up to. Deposits get no
// VAT. Rates are only given, and then certain, when every line gets one
// and each rate's lines add up to its amount, with only one way to split
// them; a 0% entry is added to the VAT for the deposits. Other lines at 0%
// stay uncertain. Lines that already have a rate are left as they are.
func assignRates(lines []Line, vat []VAT) ([]Line, []VAT) {
	if len(lines) == 0 || len(vat) == 0 {
		return lines, vat
	}
	var deposit money.Amount
	var others []int
	for i, l := range lines {
		if l.TaxRate.Found {
			return lines, vat
		}
		if depositWords.MatchString(l.Description) {
			deposit += l.Total.V
		} else {
			others = append(others, i)
		}
	}
	for _, incl := range []bool{true, false} {
		groups, ok := rateGroups(vat, deposit, incl)
		if !ok {
			continue
		}
		split, ok := splitOverRates(lines, others, groups)
		if !ok {
			continue
		}
		out := append([]Line(nil), lines...)
		for i := range out {
			out[i].TaxRate = sure("0")
		}
		for k, i := range others {
			out[i].TaxRate = sure(groups[split[k]].rate)
			if isZero(out[i].TaxRate.V) {
				// No VAT on goods or services may be an exemption or VAT
				// the buyer accounts for: the summary does not tell.
				out[i].TaxRate.Confidence = Uncertain
			}
		}
		if deposit != 0 && !hasRate(vat, "0") {
			vat = append(append([]VAT(nil), vat...), VAT{Rate: "0", Taxable: deposit})
		}
		return out, vat
	}
	return lines, vat
}

// rateGroups are the rates of a VAT summary with the amount their lines
// add up to, incl. or excl. VAT; a 0% rate's amount is less the deposits.
func rateGroups(vat []VAT, deposit money.Amount, incl bool) ([]rateGroup, bool) {
	var out []rateGroup
	for _, v := range vat {
		target := v.Taxable
		if incl {
			target += v.Tax
		}
		if isZero(v.Rate) {
			target -= deposit
		}
		if target != 0 {
			out = append(out, rateGroup{rate: v.Rate, target: target})
		}
	}
	return out, len(out) > 0 && len(out) <= 2
}

// splitOverRates finds the one way to give each of the lines idx a group
// so that each group's lines add up to its target: split[k] is the group
// of lines[idx[k]].
func splitOverRates(lines []Line, idx []int, groups []rateGroup) ([]int, bool) {
	split := make([]int, len(idx))
	if len(groups) == 1 {
		var sum money.Amount
		for _, i := range idx {
			sum += lines[i].Total.V
		}
		return split, near(sum, groups[0].target, 1)
	}
	if len(idx) > maxRateSearch {
		return nil, false
	}
	found := -1
	for mask := 0; mask < 1<<len(idx); mask++ {
		var first, second money.Amount
		for k, i := range idx {
			if mask&(1<<k) != 0 {
				second += lines[i].Total.V
			} else {
				first += lines[i].Total.V
			}
		}
		if first != groups[0].target || second != groups[1].target {
			continue
		}
		if found >= 0 {
			return nil, false // more than one way: not sure which
		}
		found = mask
	}
	if found < 0 {
		return nil, false
	}
	for k := range idx {
		if found&(1<<k) != 0 {
			split[k] = 1
		}
	}
	return split, true
}

func hasRate(vat []VAT, rate string) bool {
	for _, v := range vat {
		if v.Rate == rate || isZero(v.Rate) && isZero(rate) {
			return true
		}
	}
	return false
}

func isZero(rate string) bool {
	bp, ok := rateBP(rate)
	return ok && bp == 0
}
