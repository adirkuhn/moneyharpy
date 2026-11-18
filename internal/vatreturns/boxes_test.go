package vatreturns

import (
	"reflect"
	"testing"

	"github.com/adirkuhn/moneyharpy/internal/money"
)

func TestBoxes(t *testing.T) {
	sums := []RateSum{
		{Type: "sales_invoice", Percentage: "21", Boxes: []string{"NL/1a"}, Base: -120_250, Payable: -25_253},
		{Type: "sales_invoice", Percentage: "9", Boxes: []string{"NL/1b"}, Base: -5_130_100, Payable: -461_570},
		{Type: "sales_invoice", Percentage: "0", Boxes: []string{"NL/1e"}, Base: -177_100},
		{Type: "purchase_invoice", Percentage: "21", Boxes: []string{"NL/5b"}, Base: 1_000_000, Receivable: 210_000},
		{Type: "purchase_invoice", Percentage: "9", Boxes: []string{"NL/4b", "NL/5b"}, Base: 1_672_900, Payable: -150_561, Receivable: 150_561},
		{Type: "purchase_invoice", Percentage: "21", Boxes: []string{"NL/2a", "NL/5b"}, Base: 341_800, Payable: -71_778, Receivable: 71_778},
	}
	got := Boxes(sums)
	want := []Box{
		{"1a", 120_250, 25_253}, {"1b", 5_130_100, 461_570}, {"1e", 177_100, 0}, {"2a", 341_800, 71_778},
		{"4b", 1_672_900, 150_561}, {"5b", 0, 432_339},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("boxes = %v, want %v", got, want)
	}
	tot := Sum(got)
	// 252 + 4615 + 717 + 1505 owed, 4324 input (rounded up).
	if tot.Owed != 708_900 || tot.Input != 432_400 || tot.Total != 276_500 {
		t.Errorf("totals = %+v", tot)
	}
	if tot.ExactOwed != 709_162 || tot.ExactInput != 432_339 || tot.ExactTotal != 276_823 {
		t.Errorf("exact totals = %+v", tot)
	}
}

func TestRounding(t *testing.T) {
	cases := []struct {
		box  string
		in   money.Amount
		base money.Amount
		tax  money.Amount
	}{
		{"1a", 25_299, 25_200, 25_200},
		{"5b", 25_201, 25_200, 25_300},
		{"1a", -25_201, -25_200, -25_300},
		{"5b", -25_299, -25_200, -25_200},
		{"1a", 25_200, 25_200, 25_200},
	}
	for _, c := range cases {
		if got := RoundBase(c.in); got != c.base {
			t.Errorf("RoundBase(%d) = %d, want %d", c.in, got, c.base)
		}
		if got := RoundTax(c.box, c.in); got != c.tax {
			t.Errorf("RoundTax(%s, %d) = %d, want %d", c.box, c.in, got, c.tax)
		}
	}
}

func TestPlusMinus(t *testing.T) {
	a := []Box{{"1a", 1000, 210}, {"5b", 0, 50}}
	b := []Box{{"1a", 400, 84}, {"1b", 100, 9}}
	if got, want := Minus(a, b), []Box{{"1a", 600, 126}, {"1b", -100, -9}, {"5b", 0, 50}}; !reflect.DeepEqual(got, want) {
		t.Errorf("Minus = %v, want %v", got, want)
	}
	if got := Minus(a, a); len(got) != 0 {
		t.Errorf("Minus(a, a) = %v, want none", got)
	}
}

func TestUntaxed(t *testing.T) {
	sums := []RateSum{
		{Type: "purchase_invoice", Percentage: "0.0", Boxes: []string{"NL/5b"}, Base: 12_471},
		{Type: "purchase_invoice", Percentage: "21.0", Boxes: []string{"NL/5b"}, Base: 50_000},
		{Type: "sales_invoice", Percentage: "0.0", Boxes: []string{"NL/1e"}, Base: -9_000},
		{Type: "purchase_invoice", Percentage: "0", Boxes: []string{"NL/4b", "NL/5b"}, Base: 1_000},
	}
	if got := Untaxed(sums); got != 12_471 {
		t.Errorf("Untaxed = %v, want 124.71", got)
	}
}
