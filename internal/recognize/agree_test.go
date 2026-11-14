package recognize

import (
	"testing"

	"github.com/adirkuhn/moneyharpy/internal/money"
)

// Synthetic readings of one receipt photo with different page modes. The
// receipt was printed on 20-10-2026.
const (
	// misreadOnly reads the date once, one digit off; the slip's date is
	// garbled beyond reading.
	misreadOnly = "EXAMPLE SHOP\nTOTAAL 36,30\nBTW% Netto BTW Bruto\n21% 30,00 6,30 36,30\n26-10-2026 14:32:24 Kassa 3\nDatum Z0/1O/Z0Z6 14:32\n"
	rightOnce   = "EXAMPLE SHOP\nTOTAAL 36,30\nBTW% Netto BTW Bruto\n21% 30,00 6,30 36,30\n20-1O-2O2 14:32:24 Kassa 3\nDatum 20/10/2026 14:32\n"
	rightTwice  = "EXAMPLE SHOP\nTOTAAL 36,30\nBTW% Netto BTW Bruto\n21% 30,00 6,30 36,30\n20-10-2026 14:32:24 Kassa 3\nDatum 20/10/2026 14:32\n"
	otherTotal  = "EXAMPLE SHOP\nTOTAAL 38,30\nPIN 38,30\nDatum 20/10/2026 14:32\n20-10-2026\n"
)

func TestAgreeAcrossReadings(t *testing.T) {
	for name, tc := range map[string]struct {
		readings    []string
		pick        int // the reading taken as best
		date        string
		dateCertain bool
		total       string
		totalSure   bool
	}{
		"a misread alone is outvoted": {
			readings: []string{misreadOnly, rightOnce, rightTwice}, pick: 0,
			date: "2026-10-20", dateCertain: true, total: "36.30", totalSure: true,
		},
		"the pick agrees with the others": {
			readings: []string{rightTwice, misreadOnly, rightOnce}, pick: 0,
			date: "2026-10-20", dateCertain: true, total: "36.30", totalSure: true,
		},
		"a tie goes to the labelled date, unsure": {
			readings: []string{misreadOnly, rightOnce}, pick: 0,
			date: "2026-10-20", dateCertain: false, total: "36.30", totalSure: true,
		},
		"one reading alone is left as it is": {
			readings: []string{misreadOnly}, pick: 0, date: "2026-10-26", dateCertain: true, total: "36.30", totalSure: true,
		},
		"a total most readings disagree with is unsure": {
			readings: []string{otherTotal, rightTwice, rightOnce}, pick: 0,
			date: "2026-10-20", dateCertain: true, total: "38.30", totalSure: false,
		},
	} {
		t.Run(name, func(t *testing.T) {
			parsed := make([]Extraction, len(tc.readings))
			for i, r := range tc.readings {
				parsed[i] = ParseText(r, today)
			}
			got := agreeAcross(parsed[tc.pick], tc.readings, parsed, today)
			if have := dateString(got.Date); have != tc.date || (got.Date.Confidence == Certain) != tc.dateCertain {
				t.Errorf("date = %s %s, want %s certain %v", have, got.Date.Confidence, tc.date, tc.dateCertain)
			}
			if have := amountString(got.TotalInclTax.Found, got.TotalInclTax.V); have != tc.total || (got.TotalInclTax.Confidence == Certain) != tc.totalSure {
				t.Errorf("total = %s %s, want %s certain %v", have, got.TotalInclTax.Confidence, tc.total, tc.totalSure)
			}
		})
	}
}

func TestAgreeAcrossReadingsIgnoresOtherDates(t *testing.T) {
	// A date that is not a misreading of the document date, such as a
	// promotion's end printed in every reading, does not compete.
	r := "TOTAAL 36,30\nDatum 20/10/2026\nActie geldig t/m 31-10-2026\n31-10-2026\n"
	readings := []string{r, r, r}
	parsed := []Extraction{ParseText(r, today), ParseText(r, today), ParseText(r, today)}
	got := agreeAcross(parsed[0], readings, parsed, today)
	if dateString(got.Date) != "2026-10-20" || got.TotalInclTax.V != money.Amount(3630) {
		t.Errorf("date %s, total %v", dateString(got.Date), got.TotalInclTax)
	}
}

// Synthetic readings of one invoice whose supplier OCR reads differently
// with each page mode.
const (
	// garbledWithVAT garbles the name but reads the VAT number.
	garbledWithVAT = "Exarnple Shcp BV\nKerkstraat 1\n1234 AB Amsterdam\nBTW NL123456782B01\nTOTAAL 36,30\nDatum 20/10/2026\n"
	// cleanWithoutVAT reads the name but not the bottom line with the VAT
	// number.
	cleanWithoutVAT = "Example Shop BV\nKerkstraat 1\n1234 AB Amsterdam\nTOTAAL 36,30\nDatum 20/10/2026\n"
	// cleanOtherVAT reads another valid VAT number.
	cleanOtherVAT = "Example Shop BV\nKerkstraat 1\n1234 AB Amsterdam\nBTW NL111111110B01\nTOTAAL 36,30\nDatum 20/10/2026\n"
	// cleanWithKVK reads the name and the KVK number, not the VAT number.
	cleanWithKVK = "Example Shop BV\nKerkstraat 1\n1234 AB Amsterdam\nKvK 12345678\nTOTAAL 36,30\nDatum 20/10/2026\n"
	// otherName reads a name unlike the others.
	otherName = "Another Store BV\nTOTAAL 36,30\nDatum 20/10/2026\n"
)

func TestAgreeAcrossReadingsOnTheSupplier(t *testing.T) {
	for name, tc := range map[string]struct {
		readings []string
		pick     int // the reading taken as best
		name     string
		vat      string
		kvk      string
		found    Confidence
	}{
		"the clean name joins the VAT number of the garbled reading": {
			readings: []string{garbledWithVAT, cleanWithoutVAT}, pick: 0,
			name: "Example Shop BV", vat: "NL123456782B01", found: Certain,
		},
		"the clean name wins over a garble two readings repeat": {
			readings: []string{garbledWithVAT, garbledWithVAT, cleanWithoutVAT}, pick: 0,
			name: "Example Shop BV", vat: "NL123456782B01", found: Certain,
		},
		"a VAT number read by another reading only is taken": {
			readings: []string{cleanWithoutVAT, garbledWithVAT}, pick: 0,
			name: "Example Shop BV", vat: "NL123456782B01", found: Certain,
		},
		"conflicting valid VAT numbers keep the best reading's": {
			readings: []string{garbledWithVAT, cleanOtherVAT}, pick: 0,
			name: "Example Shop BV", vat: "NL123456782B01", found: Certain,
		},
		"a VAT number most readings agree on wins": {
			readings: []string{cleanOtherVAT, garbledWithVAT, garbledWithVAT}, pick: 0,
			name: "Example Shop BV", vat: "NL123456782B01", found: Certain,
		},
		"a KVK number from another reading backs the name": {
			readings: []string{cleanWithoutVAT, cleanWithKVK}, pick: 0,
			name: "Example Shop BV", kvk: "12345678", found: Certain,
		},
		"a name two readings agree on is certain": {
			readings: []string{otherName, cleanWithoutVAT, cleanWithoutVAT}, pick: 1,
			name: "Example Shop BV", found: Certain,
		},
		"a name one reading reads, without anything backing it, is unsure": {
			readings: []string{cleanWithoutVAT, otherName}, pick: 0,
			name: "Example Shop BV", found: Uncertain,
		},
		"one reading alone is left as it is": {
			readings: []string{garbledWithVAT}, pick: 0,
			name: "Exarnple Shcp BV", vat: "NL123456782B01", found: Certain,
		},
	} {
		t.Run(name, func(t *testing.T) {
			parsed := make([]Extraction, len(tc.readings))
			for i, r := range tc.readings {
				parsed[i] = ParseText(r, today)
			}
			got := agreeAcross(parsed[tc.pick], tc.readings, parsed, today)
			s := got.Supplier
			if s.Name != tc.name || s.VATNumber != tc.vat || s.KVK != tc.kvk || got.SupplierFound != tc.found {
				t.Errorf("supplier = %q %q %q %s, want %q %q %q %s",
					s.Name, s.VATNumber, s.KVK, got.SupplierFound, tc.name, tc.vat, tc.kvk, tc.found)
			}
		})
	}
}

func TestAgreeAcrossReadingsOnIdentifiers(t *testing.T) {
	// An IBAN or e-mail address that fails validation does not vote.
	a := "Example Shop BV\nIBAN NL91ABNA0417164300\ninfo@example.com\nTOTAAL 36,30\nDatum 20/10/2026\n"
	b := "Example Shop BV\nIBAN NL91ABNA0417164301\ninfo@example\nTOTAAL 36,30\nDatum 20/10/2026\n"
	parsed := []Extraction{ParseText(b, today), ParseText(a, today)}
	got := agreeAcross(parsed[0], []string{b, a}, parsed, today)
	if got.Supplier.IBAN != "NL91ABNA0417164300" || got.Supplier.Email != "info@example.com" {
		t.Errorf("IBAN %q, e-mail %q", got.Supplier.IBAN, got.Supplier.Email)
	}
}

func TestNameSimilarity(t *testing.T) {
	for _, tc := range []struct {
		a, b string
		same bool
	}{
		{"Exarnple Shcp BV", "Example Shop BV", true},
		{"Example Shop B.V.", "Example Shop BV", true},
		{"EXAMPLE SHOP BV", "Example Shop BV", true},
		{"Another Store BV", "Example Shop BV", false},
	} {
		if got := similarNames(tc.a, tc.b); got != tc.same {
			t.Errorf("similarNames(%q, %q) = %v", tc.a, tc.b, got)
		}
	}
}
