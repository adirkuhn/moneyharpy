package money

import (
	"testing"
	"time"
)

func day(s string) time.Time {
	t, _ := time.Parse(time.DateOnly, s)
	return t
}

func TestParseReportPeriod(t *testing.T) {
	today := day("2026-11-11")
	for in, want := range map[string]string{"": "202611..202611", "202501": "202501..202501", "this_quarter": "202610..202612", "20250101..20250331": "202501..202503"} {
		p, err := ParseReportPeriod(in, today)
		if err != nil || p.Monthly() != want {
			t.Errorf("ParseReportPeriod(%q) = %s, %v; want %s", in, p.Monthly(), err, want)
		}
	}
	for _, in := range []string{"20250102..20250131", "2025", "this_week"} {
		if _, err := ParseReportPeriod(in, today); err == nil {
			t.Errorf("ParseReportPeriod(%q) accepted", in)
		}
	}
}

func TestSplit(t *testing.T) {
	p := Period{From: day("2025-02-01"), To: day("2025-07-31")}
	if p.Months() != 6 {
		t.Errorf("Months() = %d", p.Months())
	}
	parts, err := p.Split("quarter")
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, q := range parts {
		got = append(got, q.Monthly())
	}
	if want := []string{"202502..202503", "202504..202506", "202507..202507"}; len(got) != 3 || got[0] != want[0] || got[1] != want[1] || got[2] != want[2] {
		t.Errorf("Split(quarter) = %v", got)
	}
	if _, err := p.Split("week"); err == nil {
		t.Error("Split(week) accepted")
	}
}
