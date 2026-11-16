package sales

import "testing"

func TestUniqueNameNumbersRepeatedFiles(t *testing.T) {
	used := map[string]bool{}
	for _, tc := range []struct{ in, want string }{
		{"order.pdf", "order.pdf"},
		{"order.pdf", "order (2).pdf"},
		{`C:\scans\order.pdf`, "order (3).pdf"},
		{"../receipt", "receipt"},
		{"", "attachment"},
	} {
		if got := uniqueName(tc.in, used); got != tc.want {
			t.Errorf("uniqueName(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestNextFillsInThePeriodWithoutMovingOn(t *testing.T) {
	r := Recurring{
		Anchor: date("2026-09-30"), InvoiceDate: date("2026-09-30"), FrequencyType: Month, Frequency: 1, Active: true,
		Lines: []Detail{{Description: "Hosting {period}", Period: "20260101..20260131"}},
	}
	inv, after := r.next()
	if inv.Lines[0].Description != "Hosting 2026-09-30 – 2026-10-29" || inv.Lines[0].Period != "20260930..20261029" {
		t.Errorf("line = %+v", inv.Lines[0])
	}
	if !after.Equal(date("2026-10-30")) || !inv.InvoiceDate.Equal(date("2026-09-30")) {
		t.Errorf("dates = %s, next %s", inv.InvoiceDate, after)
	}
	if r.Lines[0].Description != "Hosting {period}" {
		t.Error("next changed the recurring invoice's lines")
	}
}
