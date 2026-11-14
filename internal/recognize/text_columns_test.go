package recognize

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
)

// Column tables as pdftotext -layout prints them: a German order
// confirmation whose rows span two pages, with a header repeated on each,
// carry-over rows and MHD/EAN lines under each item, and a Portuguese
// invoice with very wide rows, prices with three decimals and a € sign,
// discount and VAT percentage columns and lot dates.
func TestParseTextColumnTables(t *testing.T) {
	for file, w := range map[string]textWant{
		"de_auftrag.txt": {
			name: "Example GmbH", vat: "DE123456789", date: "2026-11-05", currency: "EUR",
			incl: "1263.80", excl: "1181.12", vat21: []string{"7:1181.12:82.68"}, prices: "excl",
			lines: []string{
				"EXAMPLE Guava dessert 350g|24|1.85|44.40|7",
				"EXAMPLE Soda zero 330ml|72|0.56|40.32|7",
				"EXAMPLE Coconut water 1L|120|1.29|154.80|7",
				"EXAMPLE Mango juice 1L Fruchtsaft aus Konzentrat|24|2.15|51.60|7",
				"EXAMPLE Rice crackers 100g|1000|0.89|890.00|7",
			},
			good: true,
		},
		"pt_fatura_wide.txt": {
			kind: KindPurchaseInvoice, name: "EXEMPLO DISTRIBUICAO, LDA.", vat: "PT123456789", iban: "PT50000201231234567890154",
			date: "2026-11-05", currency: "EUR", incl: "325.42", excl: "307.00", vat21: []string{"6:307.00:18.42"}, prices: "excl",
			lines: []string{
				"EXAMPLE SNACK MINI 32 X|32|1.45|46.40|6",
				"EXAMPLE BOLACHA MARIA 200G|24||21.48|6",
				"EXAMPLE AZEITE VIRGEM 750ML|12|6.25|75.00|6",
				"EXAMPLE QUEIJO FLAMENGO|2.5|8.99|22.48|6",
				"EXAMPLE CAFE MOIDO 250G|10||29.64|6",
				"EXAMPLE ARROZ CAROLINO 1KG|40|1.15|46.00|6",
				"EXAMPLE ATUM EM OLEO 120G|48||66.00|6",
			},
			good: true,
		},
	} {
		t.Run(file, func(t *testing.T) {
			data, err := os.ReadFile(filepath.Join("testdata", file))
			if err != nil {
				t.Fatal(err)
			}
			got := ParseText(string(data), today)
			checkText(t, got, w)
			for i, l := range got.Lines {
				if l.Total.Confidence != Certain || l.TaxRate.Confidence != Certain {
					t.Errorf("line %d total %+v rate %+v, want certain: the lines add up to the total", i, l.Total, l.TaxRate)
				}
			}
		})
	}
}

func TestColumnHeader(t *testing.T) {
	for line, want := range map[string]bool{
		"Pos.  Artikelnr.  Bezeichnung   AnzahlVPE  Inhalt  Menge Einh. P.Einheit P.VPE Gesamtpreis SC": true,
		"Lote  Validade  Referência  Descrição  Quant.  Uni  Preço S/IVA  Desc.  IVA  Valor Liquido":    true,
		"Aantal  Omschrijving  Prijs  Bedrag":                     true,
		"Qty  Description  Unit price  Amount":                    true,
		"Quantité  Désignation  Prix unitaire  Montant":           true,
		"Cantidad  Descripción  Precio  Importe":                  true,
		"Aant Omschrijving P.st/kg Bedrag":                        false, // no price column
		"Omschrijving   Bedrag":                                   false,
		"Gesamtpreis  Menge":                                      false, // "Gesamtpreis" is no price column
		"Qty 2  Price 3,00  Total 6,00":                           false, // amounts: a row, not a header
		"Termijnbedrag stroom november  Totaal excl. btw  121,90": false,
	} {
		if _, got := columnHeader(line); got != want {
			t.Errorf("columnHeader(%q) = %v, want %v", line, got, want)
		}
	}
}

func TestColumnRow(t *testing.T) {
	de := "Pos.  Artikelnr.  Bezeichnung                     AnzahlVPE   Inhalt        Menge Einh.   P.Einheit     P.VPE   Gesamtpreis  SC"
	pt := "Lote        Validade       Referência    Descrição                    Quant.  Uni   Preço S/IVA    Desc.   IVA    Valor Liquido"
	en := "Description                 Qty      Unit price        Amount"
	for name, tc := range map[string]struct {
		head, row string
		want      string // "desc|qty|price|total|rate", "" for no row
	}{
		"German, the unit price": {
			de, "  1   512-1       EXAMPLE Guava dessert 350g      1,00 VPE    à 24              24 Einh         1,85     44,40         44,40  10",
			"EXAMPLE Guava dessert 350g|24|1.85|44.40|",
		},
		"German thousands": {
			de, "  5   820-3       EXAMPLE Rice crackers 100g     50,00 VPE    à 20           1.000 Einh         0,89     17,80        890,00  10",
			"EXAMPLE Rice crackers 100g|1000|0.89|890.00|",
		},
		"German big amounts": {
			de, "  6   900-1       EXAMPLE Water 1,5L             80,00 VPE    à 6              480 Einh         2,55     15,30      1.224,00  10",
			"EXAMPLE Water 1,5L|480|2.55|1224.00|",
		},
		"Portuguese, three decimals and €": {
			pt, "L123        06-abr-2027    100138        EXAMPLE SNACK MINI 32 X        32     UNI     1,450 €      0%     6%       46,40 €",
			"EXAMPLE SNACK MINI 32 X|32|1.45|46.40|6",
		},
		"Portuguese, a price not in cents": {
			pt, "L124        12-mai-2027    100139        EXAMPLE BOLACHA MARIA 200G     24     UN      0,895 €      0%     6%       21,48 €",
			"EXAMPLE BOLACHA MARIA 200G|24||21.48|6",
		},
		"Portuguese, a discount": {
			pt, "L127        01-out-2027    100142        EXAMPLE CAFE MOIDO 250G        10     CX      3,120 €      5%     6%       29,64 €",
			"EXAMPLE CAFE MOIDO 250G|10||29.64|6",
		},
		"English, price before quantity is no matter": {
			en, "Consulting hours            12,5         85.00        1,062.50", "Consulting hours|12.5|85.00|1062.50|",
		},
		"carry-over":         {de, "                                       Übertrag                    239,52", ""},
		"a transportar":      {pt, "                                                   A transportar         195,00 €", ""},
		"metadata":           {de, "                  MHD: 31.10.2027   EAN: 2000000000015        Gewicht: 9,60", ""},
		"numbers that fail":  {en, "Consulting hours            12         85,00        1.000,00", ""},
		"no description":     {en, "                            12         85,00        1.020,00", ""},
		"a date is no total": {en, "Delivery 12-11-2026        1          12,00        12,00", "Delivery 12-11-2026|1|12.00|12.00|"},
	} {
		t.Run(name, func(t *testing.T) {
			h, ok := columnHeader(tc.head)
			if !ok {
				t.Fatalf("no header in %q", tc.head)
			}
			l, _, ok := columnRow(tc.row, h)
			got := ""
			if ok {
				got = strings.Join([]string{l.Description, l.Quantity.V, amountString(l.Price.Found, l.Price.V),
					amountString(l.Total.Found, l.Total.V), l.TaxRate.V}, "|")
			}
			if got != tc.want {
				t.Errorf("columnRow = %q, want %q", got, tc.want)
			}
		})
	}
}

// A column table whose lines do not add up to the total by a little are
// kept, uncertain; way off they are dropped.
func TestColumnLinesThatDoNotAddUp(t *testing.T) {
	head := "Description                 Qty      Unit price        Amount\n" +
		"Widget                      2        10,00             20,00\n" +
		"Gadget                      3        5,00              15,00\n"
	for name, tc := range map[string]struct {
		total string
		lines int
	}{
		"a missed cent": {"Total excl. VAT  35,10\nVAT 21%  7,37\nTotal  42,47\n", 2},
		"way off":       {"Total excl. VAT  135,00\nVAT 21%  28,35\nTotal  163,35\n", 0},
	} {
		t.Run(name, func(t *testing.T) {
			got := ParseText(head+tc.total, today)
			if len(got.Lines) != tc.lines {
				t.Fatalf("lines = %+v, want %d", got.Lines, tc.lines)
			}
			for _, l := range got.Lines {
				if l.Total.Confidence != Uncertain {
					t.Errorf("line %+v, want uncertain", l)
				}
			}
		})
	}
}

// bigInvoice is a synthetic wide invoice of about 150,000 characters: many
// pages of rows of a column table.
func bigInvoice() (string, int) {
	head := "Lote        Validade       Referência    Descrição                    Quant.  Uni   Preço S/IVA    Desc.   IVA    Valor Liquido"
	var b strings.Builder
	b.WriteString("EXEMPLO DISTRIBUICAO, LDA.\nContribuinte: 123456789\nData de Emissão: 2026-11-05\n")
	rows, cents := 0, 0
	for page := 0; b.Len() < 150_000; page++ {
		b.WriteString(head + strings.Repeat(" ", 150) + "\n")
		if page > 0 {
			fmt.Fprintf(&b, "%s Transporte %s %d,%02d €\n", strings.Repeat(" ", 150), strings.Repeat(" ", 100), cents/100, cents%100)
		}
		for range 40 {
			rows++
			q := rows%9 + 1
			fmt.Fprintf(&b, "L%04d       06-abr-2027    %06d        EXAMPLE PRODUCT %04d           %d     UN      1,250 €      0%%     6%%%s%d,%02d €\n",
				rows, 100000+rows, rows, q, strings.Repeat(" ", 150), q*125/100, q*125%100)
			cents += q * 125
		}
		fmt.Fprintf(&b, "%s A transportar %s %d,%02d €\n\fProcessado por programa certificado\n", strings.Repeat(" ", 150), strings.Repeat(" ", 100), cents/100, cents%100)
	}
	fmt.Fprintf(&b, "Total Ilíquido %d,%02d €\n", cents/100, cents%100)
	return b.String(), rows
}

func TestParseTextBigColumnTableIsFast(t *testing.T) {
	text, rows := bigInvoice()
	start := time.Now()
	got := ParseText(text, today)
	took := time.Since(start)
	if len(got.Lines) != rows {
		t.Errorf("lines = %d, want %d", len(got.Lines), rows)
	}
	if budget := 200 * time.Millisecond * raceSlowdown; took > budget {
		t.Errorf("ParseText of %d characters took %v, want under %v", len(text), took, budget)
	}
}

func BenchmarkParseTextBigColumnTable(b *testing.B) {
	text, _ := bigInvoice()
	b.SetBytes(int64(len(text)))
	for b.Loop() {
		ParseText(text, today)
	}
}

func TestNumberTokens(t *testing.T) {
	for in, want := range map[string][]string{
		"44,40":         {"44.4"},
		"1.234,56":      {"1234.56"},
		"1,234.56":      {"1234.56"},
		"1,450":         {"1.45", "1450"},
		"1.000":         {"1", "1000"},
		"24":            {"24"},
		"2,500":         {"2.5", "2500"},
		"0,895":         {"0.895", "895"},
		"46,40€":        {"46.4"},
		"512-1":         nil,
		"350g":          nil,
		"31.10.2027":    nil,
		"2000000000015": nil,
		"6%":            nil,
	} {
		var got []string
		if n, ok := numberToken(in); ok {
			for _, v := range n.vals {
				got = append(got, decimalString(v))
			}
		}
		if diff := cmp.Diff(want, got); diff != "" {
			t.Errorf("numberToken(%q) (-want +got)\n%s", in, diff)
		}
	}
}
