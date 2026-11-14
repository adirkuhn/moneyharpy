package recognize

import (
	"os"
	"path/filepath"
	"testing"
)

// An energy bill in two columns: the supplier on a "Postbus" line, the
// buyer's company and address under it beside the delivery address, and the
// legal entity in a "handelsnaam van" sentence by the VAT and KvK numbers.
func TestParseTextEnergyBillSupplier(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("testdata", "nl_energy_bill.txt"))
	if err != nil {
		t.Fatal(err)
	}
	got := ParseText(string(data), today)
	checkText(t, got, textWant{
		name: "Voorbeeld Retail Energie B.V.", vat: "NL123456782B01", kvk: "12345678", ref: "123456789012",
		date: "2026-11-05", incl: "218.00", excl: "180.16", vat21: []string{"21:180.16:37.84"}, good: true,
	})
	if got.TotalInclTax.Confidence != Certain {
		t.Errorf("total %+v, want certain", got.TotalInclTax)
	}
}

func TestSupplierNameIsNoAddressOrPageMarker(t *testing.T) {
	for name, tc := range map[string]struct {
		text string
		want string
	}{
		"trade name sentence": {
			"Example Shop B.V.\nVoorbeeld Stroom Zakelijk is een handelsnaam van Voorbeeld Retail Energie B.V.\nBtw NL123456782B01\n",
			"Voorbeeld Retail Energie B.V.",
		},
		"English trade name": {
			"Example Shop B.V.\nExample Power is a trading name of Example Energy Ltd.\nVAT NL123456782B01\n",
			"Example Energy Ltd.",
		},
		"the company before Postbus": {
			"Voorbeeld Stroom, Postbus 1234 | 1234 AB Voorbeeldstad          Klantnummer 0012\n", "Voorbeeld Stroom",
		},
		"a street with a column beside it": {
			"Factuur\nVoorbeeldstraat 7 B                              AMSTERDAM\nKassa 3\n", "",
		},
		"a delivery address line": {
			"Factuur\nEXAMPLE                     Leveringsadres        Voorbeeldstraat 7 B,\n", "",
		},
		"a street line without a number above the postcode": {
			"Fatura\nRUA DO EXEMPLO DA INDUSTRIA LOTE ARMAZEM\n1000-001 Lisboa\n", "",
		},
		"a page marker":          {"Fatura\nPágina 1 de 3                    Original\n", ""},
		"a numbered page marker": {"Fatura\nPágina Nº 1 de 3                 Original\n", ""},
		"a copy marker":          {"Duplicado\nTriplicado\nQuadruplicado\n2ª via\n", ""},
		"a long number":          {"Factuurnummer         123456789012\n", ""},
	} {
		t.Run(name, func(t *testing.T) {
			lines := prepare(tc.text)
			s, _ := readSupplier(lines, buyerLines(lines), Self{})
			if s.Name != tc.want {
				t.Errorf("name = %q, want %q", s.Name, tc.want)
			}
		})
	}
}

func TestOneCopyOfRepeatedLines(t *testing.T) {
	a, b := Line{Description: "A"}, Line{Description: "B"}
	for name, tc := range map[string]struct {
		in   []Line
		want int
	}{
		"four copies":    {[]Line{a, b, a, b, a, b, a, b}, 2},
		"two copies":     {[]Line{a, b, a, b}, 2},
		"no copies":      {[]Line{a, b, b}, 3},
		"the same line":  {[]Line{a, a}, 1},
		"a partial copy": {[]Line{a, b, a}, 3},
		"nothing at all": {nil, 0},
	} {
		if got := oneCopy(tc.in); len(got) != tc.want {
			t.Errorf("%s: oneCopy = %d lines, want %d", name, len(got), tc.want)
		}
	}
}
