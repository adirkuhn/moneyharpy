package recognize

import (
	"slices"
	"testing"
)

func TestValidIBAN(t *testing.T) {
	for in, want := range map[string]bool{
		"NL91ABNA0417164300":              true,
		"nl91 abna 0417 1643 00":          true,
		"NL91-ABNA-0417-1643-00":          true,
		"NL91ABNA0417164301":              false,
		"NL91ABNA041716430":               false, // too short for NL
		"DE89 3704 0044 0532 0130 00":     true,
		"PT50 0002 0123 1234 5678 9015 4": true,
		"GB29NWBK60161331926819":          true,
		"":                                false,
	} {
		if got := ValidIBAN(in); got != want {
			t.Errorf("ValidIBAN(%q) = %v, want %v", in, got, want)
		}
	}
	if got := NormalizeIBAN(" nl91 abna.0417-1643 00 "); got != "NL91ABNA0417164300" {
		t.Errorf("NormalizeIBAN = %q", got)
	}
}

func TestFindIBANs(t *testing.T) {
	text := "IBAN: NL91 ABNA 0417 1643 00 BIC ABNANL2A\nOther DE89370400440532013000.\nBad NL91ABNA0417164301\nagain NL91ABNA0417164300"
	got := FindIBANs(text)
	want := []string{"NL91ABNA0417164300", "DE89370400440532013000"}
	if !slices.Equal(got, want) {
		t.Errorf("FindIBANs = %v, want %v", got, want)
	}
}

func TestValidVAT(t *testing.T) {
	for in, want := range map[string]bool{
		"NL123456782B01":     true, // elfproef
		"NL 1234.56.782.B01": true,
		"NL123456789B01":     false, // well formed, checksum fails
		"NL000099998B57":     true,  // mod 97 (sole proprietor format)
		"NL12345678B01":      false,
		"DE123456789":        true,
		"FR12345678901":      true,
		"PT123456789":        true,
		"GB123456789":        true,
		"ATU12345678":        true,
		"BE0123456789":       true,
		"XX123456789":        false,
		"DE12345678":         false,
	} {
		if got := ValidVAT(in); got != want {
			t.Errorf("ValidVAT(%q) = %v, want %v", in, got, want)
		}
	}
	if !ValidNLVAT("NL123456782B01") || ValidNLVAT("DE123456789") {
		t.Error("ValidNLVAT")
	}
	if got := NormalizeVAT(" nl 1234.56.782 b01"); got != "NL123456782B01" {
		t.Errorf("NormalizeVAT = %q", got)
	}
}

func TestValidRF(t *testing.T) {
	for in, want := range map[string]bool{
		"RF18 5390 0754 7034": true,
		"RF18539007547034":    true,
		"RF19539007547034":    false,
		"RF18":                false,
	} {
		if got := ValidRF(in); got != want {
			t.Errorf("ValidRF(%q) = %v, want %v", in, got, want)
		}
	}
}

func TestValidBetalingskenmerk(t *testing.T) {
	for in, want := range map[string]bool{
		"3000 0000 0000 0047": true,
		"8000000000000047":    false,
		"123":                 false,
	} {
		if got := ValidBetalingskenmerk(in); got != want {
			t.Errorf("ValidBetalingskenmerk(%q) = %v, want %v", in, got, want)
		}
	}
}
