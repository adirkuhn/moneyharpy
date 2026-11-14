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
