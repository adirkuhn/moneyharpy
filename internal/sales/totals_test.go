package sales

import (
	"testing"

	"github.com/adirkuhn/moneyharpy/internal/money"
	"github.com/adirkuhn/moneyharpy/internal/resource"
)

func TestParseQuantity(t *testing.T) {
	cases := map[string]int64{"1": 1000, "2,5": 2500, "1.25 uur": 1250, " 3 x": 3000, "-1": -1000, "0.333": 333}
	for in, want := range cases {
		got, err := ParseQuantity(in)
		if err != nil || got != want {
			t.Errorf("ParseQuantity(%q) = %d, %v; want %d", in, got, err, want)
		}
	}
	for _, bad := range []string{"", "x", "1.2345", "1e3"} {
		if _, err := ParseQuantity(bad); err == nil {
			t.Errorf("ParseQuantity(%q) accepted", bad)
		}
	}
}

func TestParsePercentage(t *testing.T) {
	cases := map[string]int64{"21": 2100, "9.5": 950, "0": 0, "21.0": 2100, "10,25": 1025}
	for in, want := range cases {
		if got, err := ParsePercentage(in); err != nil || got != want {
			t.Errorf("ParsePercentage(%q) = %d, %v", in, got, err)
		}
	}
	if _, err := ParsePercentage("101"); err == nil {
		t.Error("101% accepted")
	}
}

const high, low resource.ID = 1, 2

func TestTotalsExclusiveRoundPerRate(t *testing.T) {
	// Three lines of 0.10 at 21%: per line VAT would be 0.02 each (0.06);
	// per rate it is 21% of 0.30 = 0.063, so 0.06 too, but 0.15 x 3 shows
	// the difference: per line 0.03 x 3 = 0.09, per rate 21% of 0.45 = 0.09.
	lines := []Line{
		{Price: 15, Quantity: 1000, TaxRateID: high, Rate: 2100},
		{Price: 15, Quantity: 1000, TaxRateID: high, Rate: 2100},
		{Price: 15, Quantity: 1000, TaxRateID: high, Rate: 2100},
		{Price: 1000, Quantity: 2500, TaxRateID: low, Rate: 900},
	}
	tot := Compute(lines, 0, false)
	if tot.ExclTax != 45+2500 || tot.Tax != 9+225 || tot.InclTax != 45+2500+9+225 {
		t.Errorf("totals = %+v", tot)
	}
	if len(tot.ByRate) != 2 || tot.ByRate[0].TaxRateID != high || tot.ByRate[0].Taxable != 45 || tot.ByRate[0].Tax != 9 {
		t.Errorf("by rate = %+v", tot.ByRate)
	}
}

func TestTotalsInclusive(t *testing.T) {
	// 121.00 incl 21% is 100.00 + 21.00; two lines of 60.50 split it.
	lines := []Line{{Price: 6050, Quantity: 1000, TaxRateID: high, Rate: 2100}, {Price: 6050, Quantity: 1000, TaxRateID: high, Rate: 2100}}
	tot := Compute(lines, 0, true)
	if tot.InclTax != 12100 || tot.ExclTax != 10000 || tot.Tax != 2100 {
		t.Errorf("totals = %+v", tot)
	}
	var sum money.Amount
	for _, b := range tot.LineBases {
		sum += b
	}
	if sum != 10000 {
		t.Errorf("line bases %v add up to %s", tot.LineBases, sum)
	}
}

func TestTotalsDiscountAndQuantity(t *testing.T) {
	// 3 x 33.33 = 99.99, 10% discount = 89.99 (rounded), 21% VAT = 18.90.
	lines := []Line{{Price: 3333, Quantity: 3000, TaxRateID: high, Rate: 2100}}
	tot := Compute(lines, 1000, false)
	if tot.ExclTax != 8999 || tot.Discount != 1000 || tot.Tax != 1890 || tot.InclTax != 10889 {
		t.Errorf("totals = %+v", tot)
	}
}

func TestTotalsSkipUnselectedOptionalLines(t *testing.T) {
	lines := []Line{{Price: 1000, Quantity: 1000, TaxRateID: high, Rate: 2100}, {Price: 500, Quantity: 1000, TaxRateID: high, Rate: 2100, Optional: true}}
	if tot := Compute(lines, 0, false); tot.ExclTax != 1000 || tot.LineBases[1] != 0 {
		t.Errorf("totals = %+v", tot)
	}
}

func TestTotalsCredit(t *testing.T) {
	lines := []Line{{Price: 1000, Quantity: -1000, TaxRateID: high, Rate: 2100}}
	if tot := Compute(lines, 0, false); tot.InclTax != -1210 || tot.Tax != -210 {
		t.Errorf("credit totals = %+v", tot)
	}
}
