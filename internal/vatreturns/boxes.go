package vatreturns

import (
	"slices"
	"strings"

	"github.com/adirkuhn/moneyharpy/internal/money"
)

// RateSum is what a VAT rate was charged on and its VAT in a period, as
// journal entries (debits positive): Base on the categories, Payable and
// Receivable on payable and receivable VAT.
type RateSum struct {
	Type                      string
	Percentage                string
	Boxes                     []string
	Base, Payable, Receivable money.Amount
}

// Box is an amount of one box of the return, in cents: what VAT is
// charged on, and the VAT.
type Box struct {
	Code      string
	Base, Tax money.Amount
}

// Input tax is the box of the VAT that is deducted; every other box adds
// to the VAT owed.
const inputTax = "5b"

// code is the box without its country, such as 1a for NL/1a.
func code(box string) string {
	_, c, ok := strings.Cut(box, "/")
	if !ok {
		return box
	}
	return c
}

// Boxes adds the rates up per box. Sales rates count what they were charged
// on and the payable VAT; purchase rates count, in the boxes of reverse
// charge and imports, what they were charged on and the VAT owed on it,
// and in input tax the VAT that is deducted.
func Boxes(sums []RateSum) []Box {
	var out []Box
	add := func(c string, base, tax money.Amount) {
		i := slices.IndexFunc(out, func(b Box) bool { return b.Code == c })
		if i < 0 {
			out = append(out, Box{Code: c})
			i = len(out) - 1
		}
		out[i].Base += base
		out[i].Tax += tax
	}
	for _, s := range sums {
		for _, b := range s.Boxes {
			switch c := code(b); {
			case c == inputTax:
				add(c, 0, s.Receivable)
			case s.Type == "purchase_invoice":
				add(c, s.Base, -s.Payable)
			default:
				add(c, -s.Base, -s.Payable)
			}
		}
	}
	return sorted(out)
}

// sorted orders boxes as the return lists them, without empty ones.
func sorted(boxes []Box) []Box {
	out := slices.DeleteFunc(slices.Clone(boxes), func(b Box) bool { return b.Base == 0 && b.Tax == 0 })
	slices.SortFunc(out, func(a, b Box) int { return strings.Compare(a.Code, b.Code) })
	return out
}

// Minus is a less b, box by box.
func Minus(a, b []Box) []Box {
	out := slices.Clone(a)
	for _, x := range b {
		out = append(out, Box{Code: x.Code, Base: -x.Base, Tax: -x.Tax})
	}
	return Plus(out)
}

// Plus adds the boxes of all lists up.
func Plus(lists ...[]Box) []Box {
	var out []Box
	for _, list := range lists {
		for _, x := range list {
			i := slices.IndexFunc(out, func(b Box) bool { return b.Code == x.Code })
			if i < 0 {
				out = append(out, x)
				continue
			}
			out[i].Base += x.Base
			out[i].Tax += x.Tax
		}
	}
	return sorted(out)
}

const euro = 100

func floorEuro(c money.Amount) money.Amount {
	if c >= 0 || c%euro == 0 {
		return c / euro * euro
	}
	return (c/euro - 1) * euro
}

func ceilEuro(c money.Amount) money.Amount { return -floorEuro(-c) }

// RoundBase is what VAT is charged on in whole euros, rounded towards zero.
func RoundBase(c money.Amount) money.Amount { return c / euro * euro }

// RoundTax is a box's VAT in whole euros, rounded in the filer's favour as
// the tax authority allows: VAT owed down, input tax up.
func RoundTax(box string, c money.Amount) money.Amount {
	if code(box) == inputTax {
		return ceilEuro(c)
	}
	return floorEuro(c)
}

// Totals are the VAT owed (5a), the input tax (5b) and what is to be paid,
// negative when it is to be received: in whole euros as declared, and in
// cents as booked.
type Totals struct {
	Owed, Input, Total                money.Amount
	ExactOwed, ExactInput, ExactTotal money.Amount
}

// Sum works the totals out from the boxes.
func Sum(boxes []Box) Totals {
	var t Totals
	for _, b := range boxes {
		if code(b.Code) == inputTax {
			t.Input += RoundTax(b.Code, b.Tax)
			t.ExactInput += b.Tax
			continue
		}
		t.Owed += RoundTax(b.Code, b.Tax)
		t.ExactOwed += b.Tax
	}
	t.Total, t.ExactTotal = t.Owed-t.Input, t.ExactOwed-t.ExactInput
	return t
}
