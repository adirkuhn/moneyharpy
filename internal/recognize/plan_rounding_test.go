package recognize

import (
	"testing"

	"github.com/adirkuhn/moneyharpy/internal/money"
	"github.com/adirkuhn/moneyharpy/internal/purchases"
	"github.com/adirkuhn/moneyharpy/internal/sales"
)

// bookedTotal is the total incl. VAT MoneyHarpy books for the details of a
// plan, as purchases computes it.
func bookedTotal(t *testing.T, p Plan) money.Amount {
	t.Helper()
	var lines []sales.Line
	for _, d := range p.Changes.Details {
		if d.Destroy {
			continue
		}
		price, err := money.ParseAmount(*d.Price)
		if err != nil {
			t.Fatal(err)
		}
		qty, err := sales.ParseQuantity(*d.Amount)
		if err != nil {
			t.Fatal(err)
		}
		var bp int64
		for _, r := range testRates {
			if r.ID == *d.TaxRateID {
				bp, _ = sales.ParsePercentage(r.Percentage)
			}
		}
		lines = append(lines, sales.Line{Price: price, Quantity: qty, TaxRateID: *d.TaxRateID, Rate: bp, Selected: true})
	}
	return sales.Compute(lines, 0, p.Changes.PricesAreInclTax != nil && *p.Changes.PricesAreInclTax).InclTax
}

// An energy bill prints 180,16 excl. VAT, 37,84 VAT at 21% and 218,00 in
// total: its supplier rounded the VAT per sub-line. Booked excl. VAT,
// MoneyHarpy's 21% of 180,16 is 37,83 and the total 217,99. Prices incl.
// VAT make the printed total.
func TestPlanBooksPricesInclVATWhenThatMakesThePrintedTotal(t *testing.T) {
	for name, tc := range map[string]struct {
		lines  []Line
		prices []string
	}{
		"one line": {
			lines:  []Line{{Description: "Energie", Quantity: certain("1"), Price: certain(money.Amount(18016)), Total: certain(money.Amount(18016)), TaxRate: certain("21")}},
			prices: []string{"218.0"},
		},
		"two lines, the rounding on the last": {
			lines: []Line{
				{Description: "Stroom", Quantity: certain("1"), Price: certain(money.Amount(9005)), Total: certain(money.Amount(9005)), TaxRate: certain("21")},
				{Description: "Gas", Quantity: certain("1"), Price: certain(money.Amount(9011)), Total: certain(money.Amount(9011)), TaxRate: certain("21")},
			},
			prices: []string{"108.96", "109.04"},
		},
		"no rate on the lines, the summary's": {
			lines:  []Line{{Description: "Energie", Quantity: certain("1"), Price: certain(money.Amount(18016)), Total: certain(money.Amount(18016))}},
			prices: []string{"218.0"},
		},
	} {
		t.Run(name, func(t *testing.T) {
			e := Extraction{
				Source: SourceText, PricesInclTax: certain(false),
				TotalInclTax: certain(money.Amount(21800)), TotalExclTax: certain(money.Amount(18016)),
				VAT: []VAT{{Rate: "21", Taxable: 18016, Tax: 3784}}, Lines: tc.lines,
			}
			p := MakePlan(PlanInput{Extraction: e, Target: Target{Kind: purchases.TypelessDocument}, Rates: testRates})
			if p.Changes.PricesAreInclTax == nil || !*p.Changes.PricesAreInclTax {
				t.Fatalf("prices incl. VAT = %v, want true", p.Changes.PricesAreInclTax)
			}
			for i, want := range tc.prices {
				if got := *p.Changes.Details[i].Price; got != want {
					t.Errorf("line %d price = %s, want %s", i, got, want)
				}
			}
			if got := bookedTotal(t, p); got != 21800 {
				t.Errorf("booked total = %s, want 218.00", got)
			}
		})
	}
}

func TestPlanOfAnEnergyBillBooksItsPrintedTotal(t *testing.T) {
	text := "Voorbeeld Energie B.V.\nFactuurdatum: 13-11-2026\n\nOmschrijving                    Bedrag\n" +
		"Levering stroom                  120,10\nLevering gas                      60,06\n\n" +
		"Totaal excl. btw                 180,16\nBtw 21%                           37,84\nTotaal incl. btw                 218,00\n"
	e := ParseText(text, today)
	if !e.TotalInclTax.Found || e.TotalInclTax.V != 21800 || e.TotalInclTax.Confidence != Certain || len(e.Lines) != 2 {
		t.Fatalf("read total %+v, lines %+v", e.TotalInclTax, e.Lines)
	}
	p := MakePlan(PlanInput{Extraction: e, Target: Target{Kind: purchases.TypelessDocument}, Rates: testRates})
	if got := bookedTotal(t, p); got != 21800 {
		t.Errorf("booked total = %s, want the printed 218.00 (details %+v)", got, p.Changes.Details)
	}
}

func TestPlanKeepsPricesExclVATWhenTheyMakeThePrintedTotal(t *testing.T) {
	line := Line{Description: "Energie", Quantity: certain("1"), Price: certain(money.Amount(18000)), Total: certain(money.Amount(18000)), TaxRate: certain("21")}
	for name, incl := range map[string]money.Amount{
		"exact":                   21780,
		"too far off to be cents": 21900,
	} {
		t.Run(name, func(t *testing.T) {
			e := Extraction{
				Source: SourceText, PricesInclTax: certain(false), TotalInclTax: certain(incl), TotalExclTax: certain(money.Amount(18000)),
				VAT: []VAT{{Rate: "21", Taxable: 18000, Tax: incl - 18000}}, Lines: []Line{line},
			}
			p := MakePlan(PlanInput{Extraction: e, Target: Target{Kind: purchases.TypelessDocument}, Rates: testRates})
			if p.Changes.PricesAreInclTax == nil || *p.Changes.PricesAreInclTax || *p.Changes.Details[0].Price != "180.0" {
				t.Errorf("prices incl. VAT %v, details %+v; want the lines as read", p.Changes.PricesAreInclTax, p.Changes.Details)
			}
		})
	}
}
