package recognize

import (
	"slices"

	"github.com/adirkuhn/moneyharpy/internal/money"
)

// rowSet is the item lines as one way of reading them found them.
type rowSet struct {
	lines []Line
	// read reads the lines when needed: only when no set before it adds
	// up to the total.
	read func() []Line
	// roughly is set when lines close to the total are worth keeping,
	// uncertain: lines read by their numbers, in columns.
	roughly bool
}

// readSets reads the sets that are read when needed, up to the first that
// comes after a set that adds up to the total; the sets after it are left
// out.
func readSets(sets []rowSet, incl, excl Value[money.Amount]) []rowSet {
	out := make([]rowSet, 0, len(sets))
	for _, set := range sets {
		if set.read != nil {
			if slices.ContainsFunc(out, func(s rowSet) bool { return addsUp(lineSum(s.lines), incl, excl) && len(s.lines) > 0 }) {
				break
			}
			set.lines = set.read()
		}
		out = append(out, set)
	}
	return out
}

// addsUp tells whether lines summing to sum make the total incl. or excl.
// VAT.
func addsUp(sum money.Amount, incl, excl Value[money.Amount]) bool {
	return incl.Found && near(sum, incl.V, 1) || excl.Found && near(sum, excl.V, 1)
}

// maxRoughGap is how far, as a share of the total, the lines of a column
// table may be from it and still be kept, uncertain: a line read wrong or
// missed. Further off, they are dropped.
const maxRoughGap = 0.02

// pickLines picks the item lines: of the sets that add up to the total
// incl. VAT (then the prices include VAT and the total is certain) or excl.
// VAT, the one that read the most prices; else a set kept roughly that
// comes close to either, uncertain; else, when no total was read, the first
// set found, whose sum is then the total.
func pickLines(sets []rowSet, incl *Value[money.Amount], excl Value[money.Amount], receipt bool) ([]Line, Value[bool]) {
	pricesIncl := Value[bool]{V: receipt, Found: true, Confidence: Uncertain}
	sets = byPrices(readSets(sets, *incl, excl))
	for _, set := range sets {
		if len(set.lines) == 0 {
			continue
		}
		sum := lineSum(set.lines)
		switch {
		case incl.Found && excl.Found && incl.V == excl.V && near(sum, incl.V, 1):
			return set.lines, pricesIncl
		case incl.Found && near(sum, incl.V, 1):
			incl.Confidence = Certain // the lines confirm it
			return set.lines, sure(true)
		case excl.Found && near(sum, excl.V, 1):
			return set.lines, sure(false)
		}
	}
	for _, set := range sets {
		if !set.roughly || len(set.lines) == 0 {
			continue
		}
		sum := lineSum(set.lines)
		switch {
		case excl.Found && roughly(sum, excl.V):
			return uncertainLines(set.lines), unsure(false)
		case incl.Found && roughly(sum, incl.V):
			return uncertainLines(set.lines), unsure(true)
		}
	}
	if incl.Found || excl.Found {
		return nil, pricesIncl
	}
	for _, set := range sets {
		if len(set.lines) > 0 {
			*incl = unsure(lineSum(set.lines))
			return set.lines, pricesIncl
		}
	}
	return nil, pricesIncl
}

// byPrices orders sets by how many of their lines have a unit price that
// makes their total, keeping the order of equals: a set that read the
// quantities and prices right first.
func byPrices(sets []rowSet) []rowSet {
	out := append([]rowSet(nil), sets...)
	slices.SortStableFunc(out, func(a, b rowSet) int { return pricedLines(b.lines) - pricedLines(a.lines) })
	return out
}

func pricedLines(lines []Line) int {
	n := 0
	for _, l := range lines {
		if l.Price.Found {
			n++
		}
	}
	return n
}

func lineSum(lines []Line) money.Amount {
	var sum money.Amount
	for _, l := range lines {
		sum += l.Total.V
	}
	return sum
}

// roughly tells whether sum is within maxRoughGap of total.
func roughly(sum, total money.Amount) bool {
	return total != 0 && float64(abs(sum-total)) <= maxRoughGap*float64(abs(total))
}

// uncertainLines marks lines as not sure: they do not add up to the total.
func uncertainLines(lines []Line) []Line {
	out := append([]Line(nil), lines...)
	for i := range out {
		for _, v := range []*Value[money.Amount]{&out[i].Price, &out[i].Total} {
			if v.Found {
				v.Confidence = Uncertain
			}
		}
		if out[i].Quantity.Found {
			out[i].Quantity.Confidence = Uncertain
		}
		if out[i].TaxRate.Found {
			out[i].TaxRate.Confidence = Uncertain
		}
	}
	return out
}
