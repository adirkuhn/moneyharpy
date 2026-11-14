package recognize

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
)

// The till receipt fixtures are one synthetic receipt photographed with a
// phone, as Tesseract reads it with its default page mode (psm 3) and with
// the page read as one block of text (psm 6).
func TestParseTextTillReceiptPhoto(t *testing.T) {
	clean := textWant{
		kind: KindReceipt, name: "Example Shop B.V.", vat: "NL123456782B01", website: "www.example.com",
		address: "Example Street 1", zip: "1234 AB", city: "Amsterdam", country: "NL", date: "2026-10-20",
		currency: "EUR", incl: "36.30", excl: "30.00", vat21: []string{"21:30.00:6.30"}, prices: "incl",
		lines: []string{"Bureaulamp LED|1|12.95|12.95|", "Notitieboek A5|2|4.50|9.00|", "Opbergdoos|1|8.35|8.35|", "Batterijen AA|1|6.00|6.00|"},
		good:  true,
	}
	for file, w := range map[string]textWant{
		"nl_till_receipt.txt":       clean,
		"ocr_till_receipt_psm6.txt": clean,
		"ocr_till_receipt_psm3.txt": {kind: KindReceipt, vat: "NL123456782B01", date: "2026-10-20", good: false},
	} {
		t.Run(file, func(t *testing.T) {
			data, err := os.ReadFile(filepath.Join("testdata", file))
			if err != nil {
				t.Fatal(err)
			}
			got := ParseText(string(data), today)
			checkText(t, got, w)
			if w.good && (got.TotalInclTax.Confidence != Certain || got.Date.Confidence != Certain) {
				t.Errorf("total %+v, date %+v, want both certain", got.TotalInclTax, got.Date)
			}
			if !w.good && got.Date.Confidence != Uncertain {
				t.Errorf("date %+v, want uncertain: another reading disagrees", got.Date)
			}
			if strings.ContainsAny(got.Supplier.Name, "“\"") {
				t.Errorf("supplier name %q is OCR garbage", got.Supplier.Name)
			}
		})
	}
}

// The tesseract_receipt fixtures are what Tesseract 5 read of a synthetic
// phone photo of that receipt (a tall photo, slightly rotated, with noise) with
// page modes 3, 4 and 6: OCR mistakes and all ("BIW" for BTW, "S35" for
// 8,35, "36, 30").
func TestParseTextTesseractReadingsOfAReceiptPhoto(t *testing.T) {
	for _, mode := range []string{"3", "4", "6"} {
		t.Run("psm "+mode, func(t *testing.T) {
			data, err := os.ReadFile(filepath.Join("testdata", "tesseract_receipt_psm"+mode+".txt"))
			if err != nil {
				t.Fatal(err)
			}
			got := ParseText(string(data), today)
			checkText(t, got, textWant{
				kind: KindReceipt, name: "Example Shop B.V.", vat: "NL123456782B01", zip: "1234 AB", city: "Amsterdam",
				date: "2026-10-20", incl: "36.30", excl: "30.00", vat21: []string{"21:30.00:6.30"}, good: true,
			})
		})
	}
}

func TestJoinSplitDecimals(t *testing.T) {
	for in, want := range map[string]string{
		"TOTAAL [4] 36 . 30":         "TOTAAL [4] 36.30",
		"TOTAAL 36 ,30":              "TOTAAL 36,30",
		"TOTAAL 36, 30":              "TOTAAL 36,30",
		"Bedrag EUR 36 , 30  ":       "Bedrag EUR 36,30  ",
		"21% 30 ,00   6, 30   36,30": "21% 30,00   6,30   36,30",
		"Totaal €36 .30":             "Totaal €36.30",
		"TOTAAL 36,30":               "TOTAAL 36,30",
		// Not amounts: times, dates, counts in a sentence, card numbers.
		"Datum 20-10-2026 14:32:24":  "Datum 20-10-2026 14:32:24",
		"Tijd 11 . 32 . 24":          "Tijd 11 . 32 . 24",
		"Datum 10 . 11 . 2026":       "Datum 10 . 11 . 2026",
		"Kassa 3, 12 artikelen":      "Kassa 3, 12 artikelen",
		"Kaart ************1234":     "Kaart ************1234",
		"Kaart 5555 , 12345678":      "Kaart 5555 , 12345678",
		"Pas 1234 5678 , 90":         "Pas 1234 5678 , 90",
		"Artikel 12 . 34 56":         "Artikel 12 . 34 56",
		"Aantal 2 , 50 stuks à 3,00": "Aantal 2 , 50 stuks à 3,00",
	} {
		if got := joinSplitDecimals(in); got != want {
			t.Errorf("joinSplitDecimals(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestFindAmountsAfterPrepare(t *testing.T) {
	for in, want := range map[string][]string{
		"TOTAAL [4] 36 . 30":        {"36.30"},
		"TOTAAL [43 36,30":          {"36.30"},
		"Datum 20-10-2026 14:32:24": nil,
		"Kaart ************1234":    nil,
	} {
		var got []string
		for _, h := range findAmounts(prepare(in)[0]) {
			got = append(got, amountString(true, h.v))
		}
		if diff := cmp.Diff(want, got); diff != "" {
			t.Errorf("findAmounts(prepare(%q)) (-want +got)\n%s", in, diff)
		}
	}
}

func TestVATSummaryTableGivesTheTotal(t *testing.T) {
	for name, tc := range map[string]struct {
		text    string
		incl    string
		certain bool
	}{
		"total agrees with the table": {
			text: "Artikel 1,00\nTOTAAL 36,30\nBTW% Netto BTW Bruto\n21% 30,00 6,30 36,30\n", incl: "36.30", certain: true,
		},
		"total unreadable, the table adds up": {
			text: "TOTAAL 36,3%\nBTW% Netto BTW Bruto\n21% 30,00 6,30 36,30\n", incl: "36.30", certain: true,
		},
		"two rates in the table": {
			text: "BTW% Netto BTW Bruto\n9% 10,00 0,90 10,90\n21% 20,00 4,20 24,20\nTOTAAL 35,10\n", incl: "35.10", certain: true,
		},
		"total disagrees with the table": {
			text: "TOTAAL 29,13\nBTW% Netto BTW Bruto\n21% 30,00 6,30 36,30\n", incl: "36.30", certain: false,
		},
		"table does not add up": {
			text: "BTW% Netto BTW Bruto\n21% 30,00 6,30 36,39\n", certain: false,
		},
	} {
		t.Run(name, func(t *testing.T) {
			got := ParseText(tc.text, today)
			if tc.incl != "" {
				if have := amountString(got.TotalInclTax.Found, got.TotalInclTax.V); have != tc.incl {
					t.Errorf("total = %q, want %q", have, tc.incl)
				}
			}
			if (got.TotalInclTax.Confidence == Certain) != tc.certain {
				t.Errorf("total %+v, want certain %v", got.TotalInclTax, tc.certain)
			}
		})
	}
}

func TestReadDatesWithReadingsThatDisagree(t *testing.T) {
	for name, tc := range map[string]struct {
		text    string
		date    string
		certain bool
	}{
		"printed twice":                   {"20-10-2026 14:32\nKassa 3\n20/10/2026 14:33\n", "2026-10-20", true},
		"labelled and unlabelled agree":   {"20-10-2026 14:32\nDatum: 20/10/2026\n", "2026-10-20", true},
		"labelled wins over a misread":    {"26-10-2026 14:32\nDatum: 20/10/2026\n", "2026-10-20", false},
		"misread labelled, printed twice": {"20-10-2026 14:32\nDatum: 26/10/2026\n20-10-2026\n", "2026-10-20", false},
		"other dates are no misreading":   {"Datum: 20/10/2026\nGeldig tot 31-12-2027\n", "2026-10-20", true},
		"the most frequent unlabelled":    {"20-10-2026\n26-10-2026\n20-10-2026\n", "2026-10-20", true},
	} {
		t.Run(name, func(t *testing.T) {
			date, _ := readDates(prepare(tc.text), today)
			if have := dateString(date); have != tc.date {
				t.Errorf("date = %q, want %q", have, tc.date)
			}
			if (date.Confidence == Certain) != tc.certain {
				t.Errorf("date %+v, want certain %v", date, tc.certain)
			}
		})
	}
}

func TestReadSupplierVATNumber(t *testing.T) {
	for name, tc := range map[string]struct {
		text string
		vat  string
		sure bool
	}{
		"labelled":                 {"BTW nummer NL123456782B01\n", "NL123456782B01", true},
		"OCR spacing":              {"BTW nr: NL 123456782B01\n", "NL123456782B01", true},
		"grouped":                  {"VAT NL 1234.56.782.B01\n", "NL123456782B01", true},
		"B read as 8":              {"BTW nummer NL123456782801\n", "NL123456782B01", true},
		"valid labelled beats bad": {"Ref NL999999999B99\nBTW nummer NL123456782B01\n", "NL123456782B01", true},
		"other EU format":          {"USt-IdNr. DE123456789\n", "DE123456789", true},
		"checksum fails":           {"BTW nummer NL123456789B01\n", "NL123456789B01", false},
	} {
		t.Run(name, func(t *testing.T) {
			lines := prepare(tc.text)
			s, conf := readSupplier(lines, buyerLines(lines))
			if s.VATNumber != tc.vat {
				t.Errorf("VAT number = %q, want %q", s.VATNumber, tc.vat)
			}
			if (conf == Certain) != tc.sure {
				t.Errorf("confidence = %q, want certain %v", conf, tc.sure)
			}
		})
	}
}

func TestSupplierNameSkipsOCRGarbage(t *testing.T) {
	for name, tc := range map[string]struct {
		text string
		want string
	}{
		"legal form near the VAT number": {"Example Shop\nExample Street 1\nEXAMPLE SHOP B.V.\nBTW nummer NL123456782B01\n", "EXAMPLE SHOP B.V."},
		"garbage with a leading quote":   {"EXAMPLE SHOP\n“Xampl Shop BV\nBTW nummer NL123456782B01\n", "EXAMPLE SHOP"},
		"low letter ratio":               {"EXAMPLE SHOP\n|~ 3#; ^% BV\nBTW nummer NL123456782B01\n", "EXAMPLE SHOP"},
		"leading punctuation cleaned":    {"- Example Shop B.V.\nBTW nummer NL123456782B01\n", "Example Shop B.V."},
		"nearest the VAT number":         {"Example Holding B.V.\nVoorbeeldstraat 1\n1234 AB Amsterdam\nKassa 3\nBon 4\nTerminal 7\nExample Shop B.V.\nBTW nummer NL123456782B01\n", "Example Shop B.V."},
	} {
		t.Run(name, func(t *testing.T) {
			lines := prepare(tc.text)
			s, _ := readSupplier(lines, buyerLines(lines))
			if s.Name != tc.want {
				t.Errorf("name = %q, want %q", s.Name, tc.want)
			}
		})
	}
}

func TestGarbledName(t *testing.T) {
	for in, want := range map[string]bool{
		"Example Shop B.V.":  false,
		"EXAMPLE SHOP":       false,
		"Café de Voorbeeld":  false,
		"A&B Example N.V.":   false,
		"“Xampl Shop BV":     true,
		"\"Exarnple Shap BV": true,
		"|~ 3#; ^% BV":       true,
		"":                   true,
		"ii':, BV":           true,
	} {
		if got := garbledName(in); got != want {
			t.Errorf("garbledName(%q) = %v, want %v", in, got, want)
		}
	}
}
