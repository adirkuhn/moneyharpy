package recognize

import (
	"github.com/adirkuhn/moneyharpy/internal/bookkeeping"
	"github.com/adirkuhn/moneyharpy/internal/money"
	"github.com/adirkuhn/moneyharpy/internal/resource"
	"github.com/adirkuhn/moneyharpy/internal/sales"
)

// maxRoundingGap is how far, in cents, the total MoneyHarpy books for lines
// priced excl. VAT may be from the printed total for the difference to be
// VAT rounding, rather than a line read wrong.
const maxRoundingGap = 5

// fitPrintedTotal makes lines priced excl. VAT book the printed total incl.
// VAT. MoneyHarpy takes the VAT per rate over the summed lines; a supplier
// that rounds it per line or sub-line prints a total a few cents off
// ("180,16 + 21% = 37,84", where MoneyHarpy books 37,83). Prices incl. VAT
// book their sum as the total, exactly, so the lines are priced incl. VAT
// when each rate's printed amount incl. VAT is known: the total for a
// single rate, else the VAT summary's taxable amount plus VAT. The cents of
// rounding go to the last line of a rate with quantity 1.
//
// It returns the lines priced incl. VAT and true, or false when the lines
// already book the printed total, are too far off, or cannot be fitted.
func fitPrintedTotal(lines []line, rates []lineRate, all []bookkeeping.TaxRate, total money.Amount, vat []VAT) ([]line, bool) {
	if len(lines) == 0 {
		return nil, false
	}
	pct := map[resource.ID]int64{}
	for _, r := range all {
		p, err := sales.ParsePercentage(r.Percentage)
		if err != nil {
			continue
		}
		if r.ShowTax || p == 0 {
			pct[r.ID] = p // not a reverse-charged rate, whose VAT is not in the total
		}
	}
	sl := make([]sales.Line, len(lines))
	for i, l := range lines {
		qty, err := sales.ParseQuantity(l.amount)
		bp, known := pct[rates[i].id]
		if err != nil || !known || l.reverse {
			return nil, false
		}
		sl[i] = sales.Line{Price: l.price, Quantity: qty, TaxRateID: rates[i].id, Rate: bp, Selected: true}
	}
	booked := sales.Compute(sl, 0, false).InclTax
	if booked == total || !near(booked, total, maxRoundingGap) {
		return nil, false
	}
	targets, ok := grossPerRate(sl, total, vat)
	if !ok {
		return nil, false
	}
	out := append([]line(nil), lines...)
	for id, target := range targets {
		if !priceInclVAT(out, sl, id, target) {
			return nil, false
		}
	}
	for i := range sl {
		sl[i].Price = out[i].price
	}
	if sales.Compute(sl, 0, true).InclTax != total {
		return nil, false
	}
	return out, true
}

// grossPerRate is the printed amount incl. VAT of each rate of the lines:
// the total for one rate, else each rate's taxable amount plus VAT from the
// VAT summary, which must add up to the total.
func grossPerRate(sl []sales.Line, total money.Amount, vat []VAT) (map[resource.ID]money.Amount, bool) {
	rateBPs := map[resource.ID]int64{}
	for _, l := range sl {
		rateBPs[l.TaxRateID] = l.Rate
	}
	if len(rateBPs) == 1 {
		return map[resource.ID]money.Amount{sl[0].TaxRateID: total}, true
	}
	out := map[resource.ID]money.Amount{}
	var sum money.Amount
	for id, bp := range rateBPs {
		found := false
		for _, v := range vat {
			if p, ok := hundredths(v.Rate); ok && p == bp {
				out[id], found = v.Taxable+v.Tax, true
				sum += v.Taxable + v.Tax
				break
			}
		}
		if !found {
			return nil, false
		}
	}
	return out, sum == total
}

// priceInclVAT prices the lines of one rate incl. VAT so that they add up
// to target: each price plus its VAT, and the rounding difference on the
// last line of quantity 1. It tells false when there is no such line.
func priceInclVAT(lines []line, sl []sales.Line, id resource.ID, target money.Amount) bool {
	var sum money.Amount
	last := -1
	for i, l := range sl {
		if l.TaxRateID != id {
			continue
		}
		price := money.Amount(divRound(int64(l.Price)*(10000+l.Rate), 10000))
		lines[i].price = price
		sum += money.Amount(divRound(int64(price)*l.Quantity, 1000))
		if l.Quantity == 1000 {
			last = i
		}
	}
	gap := target - sum
	switch {
	case gap == 0:
		return true
	case last < 0 || !near(gap, 0, maxRoundingGap):
		return false
	}
	lines[last].price += gap
	return true
}
