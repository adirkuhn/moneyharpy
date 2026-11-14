package recognize

import (
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/adirkuhn/moneyharpy/internal/money"
)

func TestMergeFillsGapsOnlyWithCertainRuleValues(t *testing.T) {
	ai := Extraction{
		Source:       SourceAI,
		Date:         uncertain(day("2026-10-26")),
		TotalInclTax: uncertain(money.Amount(3630)),
		Supplier:     Supplier{Name: "Example Shop B.V."},
	}
	rules := Extraction{
		Source:        SourceOCR,
		Kind:          certain(KindReceipt),
		Reference:     uncertain("0042"),
		Date:          certain(day("2026-10-20")),
		DueDate:       uncertain(day("2026-11-20")),
		Currency:      certain("EUR"),
		PricesInclTax: uncertain(true),
		TotalExclTax:  certain(money.Amount(3000)),
		VAT:           []VAT{{Rate: "21", Taxable: 3000, Tax: 630}},
		Supplier:      Supplier{Name: "Other Name B.V.", VATNumber: "NL123456782B01", IBAN: "NL91ABNA0417164300"},
		SupplierFound: Certain,
	}
	got := merge(ai, rules)
	want := Extraction{
		Source:        SourceAI,
		Kind:          certain(KindReceipt),
		Date:          certain(day("2026-10-20")), // the rules are sure, the model is not
		Currency:      certain("EUR"),
		TotalInclTax:  uncertain(money.Amount(3630)),
		TotalExclTax:  certain(money.Amount(3000)),
		Supplier:      Supplier{Name: "Example Shop B.V.", VATNumber: "NL123456782B01", IBAN: "NL91ABNA0417164300"},
		SupplierFound: Certain,
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("merge (-want +got)\n%s", diff)
	}
}

func TestMergeNeverTakesGarbageOrUncertainSupplierDetails(t *testing.T) {
	ai := Extraction{Source: SourceAI, TotalInclTax: uncertain(money.Amount(3630))}
	rules := Extraction{
		Supplier: Supplier{
			Name: "“Xampl Shop BV", VATNumber: "NL123456789B01", KVK: "12345678", City: "Amsterdam", Phone: "020-1234567",
		},
		SupplierFound: Uncertain,
	}
	got := merge(ai, rules)
	if got.Supplier != (Supplier{}) || got.SupplierFound != "" {
		t.Errorf("supplier = %+v (%q), want nothing from uncertain rules", got.Supplier, got.SupplierFound)
	}

	rules.SupplierFound = Certain
	rules.Supplier.VATNumber = "NL123456782B01"
	got = merge(ai, rules)
	want := Supplier{VATNumber: "NL123456782B01", KVK: "12345678", City: "Amsterdam", Phone: "020-1234567"}
	if diff := cmp.Diff(want, got.Supplier); diff != "" {
		t.Errorf("supplier (-want +got)\n%s", diff)
	}
}

func TestMergeKeepsRuleLinesAndVATOnlyWithTheSameCertainTotal(t *testing.T) {
	lines := []Line{{Description: "Opbergdoos", Total: certain(money.Amount(3630))}}
	vat := []VAT{{Rate: "21", Taxable: 3000, Tax: 630}}
	for name, tc := range map[string]struct {
		ai, rules Value[money.Amount]
		keep      bool
	}{
		"same total":         {uncertain(money.Amount(3630)), certain(money.Amount(3630)), true},
		"model has no total": {Value[money.Amount]{}, certain(money.Amount(3630)), true},
		"other total":        {uncertain(money.Amount(2913)), certain(money.Amount(3630)), false},
		"rules unsure":       {uncertain(money.Amount(3630)), uncertain(money.Amount(3630)), false},
	} {
		t.Run(name, func(t *testing.T) {
			got := merge(Extraction{TotalInclTax: tc.ai}, Extraction{TotalInclTax: tc.rules, Lines: lines, VAT: vat})
			if (len(got.Lines) > 0) != tc.keep || (len(got.VAT) > 0) != tc.keep {
				t.Errorf("lines %v, VAT %v, want kept %v", got.Lines, got.VAT, tc.keep)
			}
		})
	}
}
