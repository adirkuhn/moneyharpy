package money

import (
	"testing"
	"time"
)

func TestParseAmount(t *testing.T) {
	for in, want := range map[string]Amount{"12.50": 1250, "-12,5": -1250, "1000": 100000, "0.01": 1, ".5": 50, "+3": 300, "1000000000": maxAmount} {
		got, err := ParseAmount(in)
		if err != nil || got != want {
			t.Errorf("ParseAmount(%q) = %d, %v; want %d", in, got, err, want)
		}
	}
	for _, in := range []string{"", "1.234", "abc", "1.2.3", "--1", "1000000000.01", "-"} {
		if _, err := ParseAmount(in); err == nil {
			t.Errorf("ParseAmount(%q) accepted", in)
		}
	}
}

func TestAmountString(t *testing.T) {
	for a, want := range map[Amount]string{1250: "12.5", -300: "-3.0", 1: "0.01", 0: "0.0", 123456: "1234.56"} {
		if got := a.String(); got != want {
			t.Errorf("%d.String() = %q, want %q", a, got, want)
		}
	}
}

func TestParsePeriod(t *testing.T) {
	today := time.Date(2026, 11, 10, 15, 0, 0, 0, time.UTC) // A Tuesday.
	day := func(s string) time.Time { d, _ := time.Parse(time.DateOnly, s); return d }
	for in, want := range map[string][2]string{
		"this_month":         {"2026-11-01", "2026-11-30"},
		"prev_month":         {"2026-10-01", "2026-10-31"},
		"next_month":         {"2026-12-01", "2026-12-31"},
		"this_quarter":       {"2026-10-01", "2026-12-31"},
		"prev_quarter":       {"2026-07-01", "2026-09-30"},
		"this_year":          {"2026-01-01", "2026-12-31"},
		"prev_year":          {"2025-01-01", "2025-12-31"},
		"this_week":          {"2026-11-09", "2026-11-15"},
		"202401..202402":     {"2024-01-01", "2024-02-29"},
		"20240105..20240110": {"2024-01-05", "2024-01-10"},
	} {
		got, err := ParsePeriod(in, today)
		if err != nil || !got.From.Equal(day(want[0])) || !got.To.Equal(day(want[1])) {
			t.Errorf("ParsePeriod(%q) = %v..%v, %v; want %v", in, got.From, got.To, err, want)
		}
	}
	for _, in := range []string{"", "202401", "202402..202401", "19990101..19990102", "200001..201101", "last_month", "2024..2025"} {
		if _, err := ParsePeriod(in, today); err == nil {
			t.Errorf("ParsePeriod(%q) accepted", in)
		}
	}
}
