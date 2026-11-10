package iban

import "testing"

func TestValid(t *testing.T) {
	cases := map[string]bool{
		"NL91ABNA0417164300":     true,
		"DE89370400440532013000": true,
		"NL91ABNA0417164301":     false, // check digits off
		"nl91abna0417164300":     false, // lower case is not normalized here
		"NL91 ABNA 0417 1643 00": false,
		"NL91ABNA":               false,
		"":                       false,
	}
	for in, want := range cases {
		if got := Valid(in); got != want {
			t.Errorf("Valid(%q) = %v, want %v", in, got, want)
		}
	}
}

func TestNormalize(t *testing.T) {
	if got := Normalize(" nl91 abna 0417 1643 00 "); got != "NL91ABNA0417164300" {
		t.Errorf("Normalize = %q", got)
	}
}
