package recognize

import (
	"fmt"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
)

func TestFindAmounts(t *testing.T) {
	for in, want := range map[string][]string{
		"Totaal 1.234,56":              {"1234.56"},
		"Total $1,234.56":              {"1234.56"},
		"€ 12,50 en €12.50":            {"12.50", "12.50"},
		"Korting -2,00":                {"-2.00"},
		"Discount (3.00)":              {"-3.00"},
		"Bonus 1,50-":                  {"-1.50"},
		"Prijs 12,-":                   {"12.00"},
		"Totaa112,50":                  {"112.50"},
		"Broodje6,50":                  {"6.50"},
		"Datum 13.11.26 of 13-11-2026": nil,
		"BTW 21,00% 4,20":              {"4.20"},
		"42,17 l x 1,999 EUR/l":        {"42.17"},
		"Order 2026 1234 qty 3":        nil,
		"(4 uur à € 85,00) € 340,00":   {"85.00", "340.00"},
		"Mixed 1.234.56":               nil,
		"CHF 1'250.00":                 {"1250.00"},
	} {
		var got []string
		for _, h := range findAmounts(in) {
			got = append(got, amountString(true, h.v))
		}
		if diff := cmp.Diff(want, got); diff != "" {
			t.Errorf("findAmounts(%q) (-want +got)\n%s", in, diff)
		}
	}
}

func TestFindDates(t *testing.T) {
	for in, want := range map[string][]string{
		"13-11-2026":               {"2026-11-13"},
		"13/11/2026 en 2026-11-10": {"2026-11-13", "2026-11-10"},
		"13.11.26":                 {"2026-11-13"},
		"11/13/2026":               {"2026-11-13"}, // only month first makes sense
		"05/11/2026":               {"2026-11-05"}, // ambiguous: day first
		"13 november 2026":         {"2026-11-13"},
		"13 nov. 2026":             {"2026-11-13"},
		"November 13, 2026":        {"2026-11-13"},
		"13. November 2026":        {"2026-11-13"},
		"13 de novembro de 2026":   {"2026-11-13"},
		"13 décembre 2026":         {"2026-12-13"},
		"1 mrt 2026":               {"2026-03-01"},
		"31-02-2026":               nil,
		"Bonnummer: 0042-3-4711":   nil,
		"tel 020-1234567":          nil,
	} {
		var got []string
		for _, d := range findDates(in) {
			got = append(got, d.t.Format(time.DateOnly))
		}
		if diff := cmp.Diff(want, got); diff != "" {
			t.Errorf("findDates(%q) (-want +got)\n%s", in, diff)
		}
	}
}

func TestTermDays(t *testing.T) {
	for in, want := range map[string]int{
		"Betalingstermijn: 14 dagen":                       14,
		"Graag betalen binnen 30 dagen":                    30,
		"Payment terms: within 21 days":                    21,
		"Zahlbar innerhalb von 10 Tagen":                   10,
		"Net 30":                                           30,
		"Prazo de pagamento: 60 dias":                      60,
		"Het bedrag wordt binnen 5 werkdagen teruggestort": 0,
		"Bedankt": 0,
	} {
		got, ok := termDays(in)
		if !ok {
			got = 0
		}
		if got != want {
			t.Errorf("termDays(%q) = %d, want %d", in, got, want)
		}
	}
}

func TestPrepareRepairsOCR(t *testing.T) {
	got := prepare("CAFE V00RBEELD\nTotaa1  l4,lO\nDatum l3-ll-2O26\nEC1A 1BB\nNL91ABNA0417164300\nKwast 50mm")
	want := []string{"CAFE VOORBEELD", "Totaal  14,10", "Datum 13-11-2026", "EC1A 1BB", "NL91ABNA0417164300", "Kwast 50mm"}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("prepare (-want +got)\n%s", diff)
	}
}

func TestParseTextConfidence(t *testing.T) {
	tests := []struct {
		name, text string
		incl, date Confidence
		kind       Kind
		good       bool
	}{
		{
			name: "only a total is a guess",
			text: "Bedankt!\nTotaal 12,50",
			incl: Uncertain, kind: KindReceipt,
		},
		{
			name: "total confirmed by the card payment",
			text: "Kassabon\n13-11-2026\nKoffie 3,50\nTotaal 3,50\nPIN 3,50",
			incl: Certain, date: Certain, kind: KindReceipt, good: true,
		},
		{
			name: "totals that do not add up stay uncertain",
			text: "Factuur\nFactuurdatum: 13-11-2026\nSubtotaal 100,00\nBTW 21% 30,00\nTotaal 125,00",
			incl: Uncertain, date: Certain, kind: KindPurchaseInvoice,
		},
		{
			name: "a date far ahead is uncertain",
			text: "Factuur\nFactuurdatum: 13-11-2029\nTotaal excl. btw 100,00\nBTW 21% 21,00\nTotaal 121,00",
			incl: Certain, date: Uncertain, kind: KindPurchaseInvoice,
		},
		{
			name: "several unlabelled dates are uncertain",
			text: "Invoice\n01-11-2026\n13-11-2026\nSubtotal 100.00\nVAT 21% 21.00\nTotal 121.00",
			incl: Certain, date: Uncertain, kind: KindPurchaseInvoice,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ParseText(tt.text, today)
			if got.TotalInclTax.Confidence != tt.incl {
				t.Errorf("total incl confidence = %q, want %q (%+v)", got.TotalInclTax.Confidence, tt.incl, got.TotalInclTax)
			}
			if got.Date.Confidence != tt.date {
				t.Errorf("date confidence = %q, want %q (%+v)", got.Date.Confidence, tt.date, got.Date)
			}
			if got.Kind.V != tt.kind {
				t.Errorf("kind = %q, want %q", got.Kind.V, tt.kind)
			}
			if got.Good() != tt.good {
				t.Errorf("Good() = %v, want %v", got.Good(), tt.good)
			}
		})
	}
}

func TestParseTextEmpty(t *testing.T) {
	got := ParseText("", today)
	if got.Source != SourceText || got.Kind.Found || got.TotalInclTax.Found || got.Date.Found || got.Good() {
		t.Errorf("empty text gave %+v", got)
	}
	if got.Currency.V != "EUR" || got.Currency.Confidence != Uncertain {
		t.Errorf("currency = %+v, want EUR uncertain", got.Currency)
	}
}

func TestParseTextKeepsBuyerOut(t *testing.T) {
	text := "Factuur aan:\nExample Shop B.V.\nVoorbeeldstraat 1\n1234 AB Amsterdam\nBTW NL987654329B01\n\n" +
		"Van:\nAnn de Vries Design\nDorpsweg 5\n5678 EF Eindhoven\nBTW NL123456782B01\nIBAN NL91ABNA0417164300\n" +
		"Wij incasseren van uw rekening NL20INGB0001234567"
	got := ParseText(text, today).Supplier
	want := Supplier{Name: "Ann de Vries Design", VATNumber: "NL123456782B01", IBAN: "NL91ABNA0417164300",
		Address1: "Dorpsweg 5", Zipcode: "5678 EF", City: "Eindhoven", Country: "NL"}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("supplier (-want +got)\n%s", diff)
	}
}

func TestParseTextLinesThatDoNotAddUpAreDropped(t *testing.T) {
	text := "Factuur\nFactuurdatum: 13-11-2026\nOmschrijving Bedrag\nWerk 80,00\nMateriaal 10,00\n" +
		"Subtotaal 100,00\nBTW 21% 21,00\nTotaal 121,00"
	got := ParseText(text, today)
	if len(got.Lines) != 0 {
		t.Errorf("lines = %v, want none", got.Lines)
	}
	if fmt.Sprint(got.VAT) != fmt.Sprint([]VAT{{Rate: "21", Taxable: cents("100"), Tax: cents("21")}}) {
		t.Errorf("VAT = %v", got.VAT)
	}
}

func TestParseTextPaymentReference(t *testing.T) {
	for text, want := range map[string]Value[string]{
		"Betalingskenmerk: 3000 0000 0000 0047":      sure("3000000000000047"),
		"Betalingskenmerk: 1234 5678 9012 3456":      unsure("1234567890123456"),
		"Pay with reference RF18 5390 0754 7034 now": sure("RF18539007547034"),
		"Reference RF19 5390 0754 7034":              {},
	} {
		if got := ParseText(text, today).PaymentReference; got != want {
			t.Errorf("%q: payment reference = %+v, want %+v", text, got, want)
		}
	}
}
