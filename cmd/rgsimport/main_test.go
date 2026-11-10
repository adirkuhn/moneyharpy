package main

import (
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestConvertNestsCodesByLevel(t *testing.T) {
	rows := [][]string{
		{"RGS3.8"},
		{"Referentiecode", "ReferentieOmslagcode", "Sortering", "Referentienummer", "Omschrijving (verkort)", "Omschrijving", "D/C", "Nivo", "Basis"},
		{"B", "", "", "", "BALANS", "BALANS", "", "1", "1"},
		{"BLim", "", "I", "10", "Liquide middelen", "Liquide middelen", "D", "2", "1"},
		{"BLimKas", "", "I.A", "1001000", "Kasmiddelen ", "Kasmiddelen", "D", "3", "1"},
		{"W", "", "", "", "WINST- EN VERLIESREKENING", "W", "", "1", ""},
		{"WBed", "", "", "42", "Bedrijfslasten", "Bedrijfslasten", "D", "2", "1"},
	}
	got, err := convert(rows)
	if err != nil {
		t.Fatal(err)
	}
	want := []item{
		{code: "B", name: "BALANS", description: "BALANS", level: 1, basic: true},
		{code: "BLim", reference: "10", name: "Liquide middelen", description: "Liquide middelen", debitCredit: "D", level: 2, parent: "B", basic: true},
		{code: "BLimKas", reference: "1001000", name: "Kasmiddelen", description: "Kasmiddelen", debitCredit: "D", level: 3, parent: "BLim", basic: true},
		{code: "W", name: "WINST- EN VERLIESREKENING", description: "W", level: 1},
		{code: "WBed", reference: "42", name: "Bedrijfslasten", description: "Bedrijfslasten", debitCredit: "D", level: 2, parent: "W", basic: true},
	}
	if diff := cmp.Diff(want, got, cmp.AllowUnexported(item{})); diff != "" {
		t.Errorf("convert() mismatch (-want +got):\n%s", diff)
	}
}

func TestConvertNeedsTheHeader(t *testing.T) {
	if _, err := convert([][]string{{"nothing"}}); err == nil {
		t.Error("convert() without a header error = nil")
	}
}

func TestColumn(t *testing.T) {
	for ref, want := range map[string]int{"A1": 0, "H12": 7, "Z3": 25, "AA7": 26} {
		if got := column(ref); got != want {
			t.Errorf("column(%s) = %d, want %d", ref, got, want)
		}
	}
}
