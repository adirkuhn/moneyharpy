package recognize

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"

	"github.com/adirkuhn/moneyharpy/internal/money"
)

func day(s string) time.Time {
	d, err := time.Parse(time.DateOnly, s)
	if err != nil {
		panic(err)
	}
	return d
}

func cents(s string) money.Amount {
	a, err := money.ParseAmount(s)
	if err != nil {
		panic(s)
	}
	return a
}

func line(desc, qty, price, total, rate string) Line {
	return Line{Description: desc, Quantity: sure(qty), Price: sure(cents(price)),
		Total: sure(cents(total)), TaxRate: sure(rate)}
}

func TestParseEInvoice(t *testing.T) {
	tests := []struct {
		file string
		want Extraction
	}{
		{"ubl_invoice.xml", Extraction{
			Source: SourceEInvoice,
			Kind:   sure(KindPurchaseInvoice),
			Supplier: Supplier{Name: "Example Shop B.V.", VATNumber: "NL123456782B01", KVK: "12345678",
				IBAN: "NL91ABNA0417164300", Email: "invoices@example.com", Address1: "Voorbeeldstraat 1",
				Zipcode: "1234 AB", City: "Amsterdam", Country: "NL", Phone: "+31 20 123 4567"},
			SupplierFound:    Certain,
			Reference:        sure("INV-2026-0042"),
			Date:             sure(day("2026-11-13")),
			DueDate:          sure(day("2026-11-27")),
			Currency:         sure("EUR"),
			PricesInclTax:    sure(false),
			TotalInclTax:     sure(cents("142.80")),
			TotalExclTax:     sure(cents("120.00")),
			VAT:              []VAT{{Rate: "21", Taxable: cents("100"), Tax: cents("21")}, {Rate: "9", Taxable: cents("20"), Tax: cents("1.80")}},
			Lines:            []Line{line("Consultancy", "2", "50", "100", "21"), line("Manual", "1", "20", "20", "9")},
			PaymentReference: sure("RF18539007547034"),
		}},
		{"ubl_creditnote.xml", Extraction{
			Source: SourceEInvoice,
			Kind:   sure(KindPurchaseInvoice),
			Supplier: Supplier{Name: "Example Shop B.V.", VATNumber: "NL123456782B01", KVK: "12345678",
				IBAN: "NL91ABNA0417164300", Address1: "Voorbeeldstraat 1", Zipcode: "1234AB", City: "Amsterdam", Country: "NL"},
			SupplierFound: Certain,
			Reference:     sure("CN-2026-007"),
			Date:          sure(day("2026-11-13")),
			DueDate:       sure(day("2026-11-27")),
			Currency:      sure("EUR"),
			PricesInclTax: sure(false),
			TotalInclTax:  sure(cents("-24.95")),
			TotalExclTax:  sure(cents("-20.62")),
			VAT:           []VAT{{Rate: "21", Taxable: cents("-20.62"), Tax: cents("-4.33")}},
			Lines:         []Line{line("Retour draadloze muis", "1", "-20.62", "-20.62", "21")},
		}},
		{"cii_invoice.xml", Extraction{
			Source: SourceEInvoice,
			Kind:   sure(KindPurchaseInvoice),
			Supplier: Supplier{Name: "Beispiel GmbH", VATNumber: "DE123456789", IBAN: "DE89370400440532013000",
				Email: "info@beispiel.example.com", Address1: "Musterstraße 12", Zipcode: "10115", City: "Berlin",
				Country: "DE", Phone: "+49 30 1234567"},
			SupplierFound:    Certain,
			Reference:        sure("RE-2026-1187"),
			Date:             sure(day("2026-11-13")),
			DueDate:          sure(day("2026-11-27")),
			Currency:         sure("EUR"),
			PricesInclTax:    sure(false),
			TotalInclTax:     sure(cents("506.35")),
			TotalExclTax:     sure(cents("425.50")),
			VAT:              []VAT{{Rate: "19", Taxable: cents("425.50"), Tax: cents("80.85")}},
			Lines:            []Line{line("Beratung (Stunden)", "4", "95", "380", "19"), line("Reisekosten", "1", "45.50", "45.50", "19")},
			PaymentReference: sure("RE-2026-1187"),
		}},
	}
	for _, tt := range tests {
		t.Run(tt.file, func(t *testing.T) {
			data, err := os.ReadFile(filepath.Join("testdata", tt.file))
			if err != nil {
				t.Fatal(err)
			}
			got, err := ParseEInvoice(data)
			if err != nil {
				t.Fatal(err)
			}
			if diff := cmp.Diff(tt.want, got); diff != "" {
				t.Errorf("(-want +got)\n%s", diff)
			}
			if !got.Good() {
				t.Error("an e-invoice should be Good")
			}
		})
	}
}

func TestParseEInvoiceRejects(t *testing.T) {
	for _, in := range []string{"", "not xml", `<?xml version="1.0"?><Order><ID>1</ID></Order>`} {
		if _, err := ParseEInvoice([]byte(in)); err == nil {
			t.Errorf("ParseEInvoice(%q) gave no error", in)
		}
	}
}

func TestParseEInvoiceInvoiceTypeCreditNote(t *testing.T) {
	xml := `<Invoice xmlns="urn:oasis:names:specification:ubl:schema:xsd:Invoice-2" xmlns:cbc="c" xmlns:cac="a">
<cbc:ID>X1</cbc:ID><cbc:IssueDate>2026-11-13</cbc:IssueDate><cbc:InvoiceTypeCode>381</cbc:InvoiceTypeCode>
<cbc:DocumentCurrencyCode>EUR</cbc:DocumentCurrencyCode>
<cac:LegalMonetaryTotal><cbc:TaxExclusiveAmount currencyID="EUR">10</cbc:TaxExclusiveAmount>
<cbc:TaxInclusiveAmount currencyID="EUR">12.10</cbc:TaxInclusiveAmount><cbc:PayableAmount currencyID="EUR">12.10</cbc:PayableAmount></cac:LegalMonetaryTotal>
</Invoice>`
	got, err := ParseEInvoice([]byte(xml))
	if err != nil {
		t.Fatal(err)
	}
	if got.TotalInclTax.V != cents("-12.10") || got.TotalExclTax.V != cents("-10") {
		t.Errorf("totals = %v, %v", got.TotalInclTax.V, got.TotalExclTax.V)
	}
}
