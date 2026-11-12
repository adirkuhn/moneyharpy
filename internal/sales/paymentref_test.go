package sales

import (
	"strings"
	"testing"
)

func TestNewPaymentReference(t *testing.T) {
	seen := map[string]bool{}
	for range 50 {
		ref := NewPaymentReference()
		if len(ref) != 25 || !strings.HasPrefix(ref, "RF") || !ValidRF(ref) {
			t.Fatalf("NewPaymentReference() = %q", ref)
		}
		short := ShortPaymentReference(ref)
		if len(short) != 12 || short[:4] != ref[:4] || !strings.HasSuffix(ref, short[4:]) || !ValidRF(short) {
			t.Fatalf("ShortPaymentReference(%q) = %q", ref, short)
		}
		if seen[ref] {
			t.Fatalf("NewPaymentReference() repeated %q", ref)
		}
		seen[ref] = true
	}
}

func TestPaymentReferenceForms(t *testing.T) {
	for _, tc := range []struct{ in, short string }{
		{"RF130000000000000PV96X5KP", "RF13PV96X5KP"},
		{"RF180000000000000SKE5TSYN", "RF18SKE5TSYN"},
		{"RF18SKE5TSYN", "RF18SKE5TSYN"},
		{"2026-0001", "2026-0001"},
		{"", ""},
	} {
		if got := ShortPaymentReference(tc.in); got != tc.short {
			t.Errorf("ShortPaymentReference(%q) = %q, want %q", tc.in, got, tc.short)
		}
	}
	for _, tc := range []struct {
		ref   string
		valid bool
	}{
		{"RF130000000000000PV96X5KP", true}, {"RF18SKE5TSYN", true}, {"rf18 ske5 tsyn", true}, {"RF19SKE5TSYN", false}, {"RF98", false}, {"2026-0001", false},
	} {
		if got := ValidRF(tc.ref); got != tc.valid {
			t.Errorf("ValidRF(%q) = %v", tc.ref, got)
		}
	}
	if got := DraftPaymentReference(42); got != "RF95CONCEPT42" {
		t.Errorf("DraftPaymentReference(42) = %q", got)
	}
	if got := DraftPaymentReference(0); got != "RF89CONCEPT" {
		t.Errorf("DraftPaymentReference(0) = %q", got)
	}
	if got := NormalizeReference(" rf18 ske5\ttsyn\n"); got != "RF18SKE5TSYN" {
		t.Errorf("NormalizeReference() = %q", got)
	}
}

func TestContainsPaymentReference(t *testing.T) {
	const ref = "RF130000000000000PV96X5KP"
	for _, tc := range []struct {
		text string
		ref  string
		want bool
	}{
		{"Payment RF13PV96X5KP thanks", ref, true},
		{"betaling rf13 pv96 x5kp", ref, true},
		{"RF13 0000 0000 0000 0PV9 6X5KP", ref, true},
		{"/REMI/USTD//RF13PV96X5KP/", ref, true},
		{"Invoice 2026-0001", ref, false},
		{"RF13PV96X5K", ref, false},
		{"RF13PV96X5KPX", ref, false},
		{"XRF13PV96X5KP", ref, false},
		{"", ref, false},
		{"anything", "", false},
		{"Invoice 2026-0001", "2026-0001", true},
		{"Invoice 2026-00012", "2026-0001", false},
	} {
		if got := ContainsPaymentReference(tc.text, tc.ref); got != tc.want {
			t.Errorf("ContainsPaymentReference(%q, %q) = %v, want %v", tc.text, tc.ref, got, tc.want)
		}
	}
}
