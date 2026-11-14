package recognize

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/google/go-cmp/cmp"
)

// The drinks receipt fixtures are a synthetic till receipt of a drinks shop
// as Tesseract and RapidOCR read it: a "KOPIE BON" header, a VAT number
// printed with dots (and O for 0), a promotion note under an item ("Van
// 6.48 voor 5,00"), a deposit without VAT and a VAT summary line "BTW 9% x
// 37,58 = 3,38".
func TestParseTextDrinksReceipt(t *testing.T) {
	want := textWant{
		kind: KindReceipt, vat: "NL001234560B01", zip: "1234 AB", city: "Amsterdam", country: "NL",
		date: "2026-10-22", incl: "48.46", excl: "45.08", vat21: []string{"9:37.58:3.38", "0:7.50:0.00"}, prices: "incl",
		lines: []string{"MINERAL WATER 1L|4|3.99|15.96|9", "SPARKLING WATER 6X|5|5.00|25.00|9", "Statiegeld|5|1.50|7.50|0"},
		good:  true,
	}
	for _, file := range []string{"nl_drinks_receipt_tesseract.txt", "nl_drinks_receipt_rapidocr.txt"} {
		t.Run(file, func(t *testing.T) {
			data, err := os.ReadFile(filepath.Join("testdata", file))
			if err != nil {
				t.Fatal(err)
			}
			got := ParseText(string(data), today)
			checkText(t, got, want)
			if got.Supplier.Name != "" {
				t.Errorf("supplier name = %q, want none: the receipt prints no name, only boilerplate", got.Supplier.Name)
			}
			for i, l := range got.Lines {
				if l.TaxRate.Confidence != Certain {
					t.Errorf("line %d rate %+v, want certain: the VAT summary confirms it", i, l.TaxRate)
				}
			}
		})
	}
}

func TestSupplierNameSkipsReceiptBoilerplate(t *testing.T) {
	for _, header := range []string{
		"KOPIE BON", "KOPIEBON", "Kopie", "Kassabon", "Kassa bon", "Bon", "Customer's receipt", "CUSTOMER RECEIPT",
		"Klantticket", "Duplicaat", "Receipt", "Copy", "Factuur", "INVOICE", "ALTIJD VOORDELIG!",
	} {
		t.Run(header, func(t *testing.T) {
			lines := prepare(header + "\nBTW Nummer NL123456782B01\nVOORBEELD DRANKEN\nExample Street 1\n")
			s, _ := readSupplier(lines, buyerLines(lines), Self{})
			if s.Name != "VOORBEELD DRANKEN" {
				t.Errorf("name = %q, want VOORBEELD DRANKEN", s.Name)
			}
		})
	}
}

func TestFindVATRepairsSeparatorsAndOCR(t *testing.T) {
	for in, want := range map[string]string{
		"BTW Nummer NL0012.34.560.B.01":    "NL001234560B01",
		"BTW Nummer NLOO12.34.56O.B.O1":    "NL001234560B01",
		"BTW NL 0012 34 560 B01":           "NL001234560B01",
		"BTW NL-0012-34-560-B-01":          "NL001234560B01",
		"BTW NL0O1234S6OBO1":               "NL001234560B01",
		"BTW NLOOI2345608O1":               "NL001234560B01",
		"BTW nr NL123456782B01 Kassa 3":    "NL123456782B01",
		"USt-IdNr. DE 123.456.789":         "DE123456789",
		"IBAN DE89 3704 0044 0532 0130 00": "",
	} {
		if got := findVAT(prepare(in)[0]); got != want {
			t.Errorf("findVAT(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestVATSummaryLineFormats(t *testing.T) {
	for _, l := range []string{
		"(BTW 9% x 37,58 = 3,38)",
		"BTW 9% × 37,58 = 3,38",
		"BTW 9% x 37,58 - 3,38",
		"(BIW 9% x 37,58 = 3,38)",
		"9% over 37,58 3,38",
		"BTW 9% 37,58 3,38",
		"BTW 9% over 40,96 3,38",
	} {
		t.Run(l, func(t *testing.T) {
			got := ParseText("4 MINERAL WATER 1L 3,99 15,96\n1 SAP 1,00 25,00\nTotaal 40,96\n"+l+"\n", today)
			if diff := cmp.Diff([]VAT{{Rate: "9", Taxable: 3758, Tax: 338}}, got.VAT); diff != "" {
				t.Errorf("VAT (-want +got)\n%s", diff)
			}
		})
	}
}

func TestRatesFromTheVATSummary(t *testing.T) {
	for name, tc := range map[string]struct {
		text  string
		rates []string
		sure  bool
	}{
		"two rates, each line's by its sum": {
			text:  "MELK 1,19\nBROOD 2,49\nWASMIDDEL 8,99\nTOTAAL 12,67\nBTW 9% over 3,68 0,30\nBTW 21% over 8,99 1,56\n",
			rates: []string{"9", "9", "21"}, sure: true,
		},
		"a deposit has no VAT": {
			text:  "COLA 1,50\nStatiegeld 0,25\nTOTAAL 1,75\nBTW 9% over 1,50 0,12\n",
			rates: []string{"9", "0"}, sure: true,
		},
		"sums that do not match leave the rates open": {
			text:  "MELK 1,19\nBROOD 2,49\nTOTAAL 3,68\nBTW 9% over 2,00 0,17\nBTW 21% over 1,68 0,29\n",
			rates: []string{"", ""},
		},
	} {
		t.Run(name, func(t *testing.T) {
			got := ParseText(tc.text, today)
			var rates []string
			for _, l := range got.Lines {
				rates = append(rates, l.TaxRate.V)
				if l.TaxRate.Found && (l.TaxRate.Confidence == Certain) != tc.sure {
					t.Errorf("line %q rate %+v, want certain %v", l.Description, l.TaxRate, tc.sure)
				}
			}
			if diff := cmp.Diff(tc.rates, rates); diff != "" {
				t.Errorf("rates (-want +got)\n%s", diff)
			}
		})
	}
}

func TestPromotionNotesBelongToTheItemAbove(t *testing.T) {
	for name, tc := range map[string]struct {
		text string
		want []string
	}{
		"was and now": {
			"5 SPARKLING WATER 6X\nVan 6.48 voor 5,00 25,00\nTotaal 25,00\n", []string{"SPARKLING WATER 6X|5|5.00|25.00|"},
		},
		"so many for": {
			"2 CHIPS PAPRIKA\n2 voor 3,00 3,00\nTotaal 3,00\n", []string{"CHIPS PAPRIKA|2|1.50|3.00|"},
		},
		"a discount note": {
			"KAAS JONG\nKorting 0,50 4,49\nTotaal 4,49\n", []string{"KAAS JONG|1|4.49|4.49|"},
		},
		"the discount summary before the total is no item": {
			"5 SPARKLING WATER 6X 5,00 25,00\nActiekorting 7,40\nTotaal 25,00\n", []string{"SPARKLING WATER 6X|5|5.00|25.00|"},
		},
		"the discount summary after the total is no item": {
			"5 SPARKLING WATER 6X 6,48 32,40\nActiekorting -7,40\nTotaal 25,00\nActiekorting 7,40\n", nil,
		},
	} {
		t.Run(name, func(t *testing.T) {
			got := ParseText(tc.text, today)
			var have []string
			for _, l := range got.Lines {
				have = append(have, l.Description+"|"+l.Quantity.V+"|"+amountString(l.Price.Found, l.Price.V)+"|"+
					amountString(l.Total.Found, l.Total.V)+"|"+l.TaxRate.V)
			}
			if tc.want == nil {
				for _, l := range got.Lines {
					if l.Description == "Actiekorting" && l.Total.V > 0 {
						t.Errorf("the discount summary is a line: %v", have)
					}
				}
				return
			}
			if diff := cmp.Diff(tc.want, have); diff != "" {
				t.Errorf("lines (-want +got)\n%s", diff)
			}
		})
	}
}
