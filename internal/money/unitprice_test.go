package money

import "testing"

func TestParseUnitPrice(t *testing.T) {
	for in, want := range map[string]UnitPrice{
		"12.5": 12_500_000, "0.0825": 82_500, "-1,995": -1_995_000, "1000": 1_000_000_000, "0.000001": 1, "12.500000": 12_500_000,
	} {
		if got, err := ParseUnitPrice(in); err != nil || got != want {
			t.Errorf("ParseUnitPrice(%q) = %d, %v; want %d", in, got, err, want)
		}
	}
	for _, bad := range []string{"", "x", "1.0000001", "1.2.3", "--1", "10000000000"} {
		if _, err := ParseUnitPrice(bad); err == nil {
			t.Errorf("ParseUnitPrice(%q) passed", bad)
		}
	}
}

func TestUnitPriceString(t *testing.T) {
	for p, want := range map[UnitPrice]string{12_500_000: "12.5", 82_500: "0.0825", -1_995_000: "-1.995", 3_000_000: "3.0", 1: "0.000001"} {
		if got := p.String(); got != want {
			t.Errorf("%d = %q, want %q", p, got, want)
		}
	}
	// On a document a price shows at least its cents.
	if got := UnitPrice(12_500_000).Decimal(); got != "12.50" {
		t.Errorf("Decimal = %q", got)
	}
	if got := UnitPrice(82_500).Decimal(); got != "0.0825" {
		t.Errorf("Decimal = %q", got)
	}
}

func TestUnitPriceTimes(t *testing.T) {
	// 3 × 0.0825 = 0.2475, rounded half away from zero to 0.25; quantities are in thousandths.
	for _, c := range []struct {
		price UnitPrice
		qty   int64
		want  Amount
	}{{82_500, 3000, 25}, {-82_500, 3000, -25}, {12_500_000, 1500, 1875}, {1_000_000_000_000_000, 1_000_000, 100_000_000_000_000}} {
		if got := c.price.Times(c.qty); got != c.want {
			t.Errorf("%d × %d = %d, want %d", c.price, c.qty, got, c.want)
		}
	}
	if got := FromAmount(1250); got != 12_500_000 {
		t.Errorf("FromAmount = %d", got)
	}
}
