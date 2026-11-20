package invoicedoc

import (
	"bufio"
	"bytes"
	"context"
	"encoding/xml"
	"io"
	"regexp"
	"strings"
	"testing"

	"github.com/adirkuhn/moneyharpy/internal/bookkeeping"
	"github.com/adirkuhn/moneyharpy/internal/contacts"
	"github.com/adirkuhn/moneyharpy/internal/money"
	"github.com/adirkuhn/moneyharpy/internal/resource"
	"github.com/adirkuhn/moneyharpy/internal/sales"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

// qualified are the namespaces in an SVRL location, left out to read it.
var qualified = regexp.MustCompile(`Q\{[^}]*\}`)

// failure is a Peppol or EN 16931 rule a document breaks.
type failure struct {
	Rules, ID, Flag, Location, Text string
}

// validatePeppol checks UBL documents, by name, against the official Peppol
// BIS Billing 3.0 rules and returns what each breaks, fatal or warning.
func validatePeppol(t *testing.T, docs map[string][]byte) map[string][]failure {
	t.Helper()
	if testing.Short() {
		t.Skip("the Peppol validator runs in a container")
	}
	ctx := context.Background()
	var files []testcontainers.ContainerFile
	for name, data := range docs {
		files = append(files, testcontainers.ContainerFile{Reader: bytes.NewReader(data), ContainerFilePath: "/in/" + name + ".xml", FileMode: 0o644})
	}
	c, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: testcontainers.ContainerRequest{
			FromDockerfile: testcontainers.FromDockerfile{Context: "testdata/peppol", Repo: "moneyharpy-peppol-validator", Tag: "3.0.22", KeepImage: true},
			Files:          files,
			WaitingFor:     wait.ForLog("=== end"),
		},
		Started: true,
	})
	testcontainers.CleanupContainer(t, c)
	if err != nil {
		t.Fatalf("start the Peppol validator: %v", err)
	}
	logs, err := c.Logs(ctx)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := io.ReadAll(logs)
	return parseReports(t, raw)
}

// parseReports reads the SVRL reports validate.sh prints, each after a line
// "=== <rules> <file>".
func parseReports(t *testing.T, raw []byte) map[string][]failure {
	t.Helper()
	out := map[string][]failure{}
	var rules, name string
	var report bytes.Buffer
	flush := func() {
		if name == "" {
			return
		}
		var svrl struct {
			Failed []struct {
				ID       string `xml:"id,attr"`
				Flag     string `xml:"flag,attr"`
				Location string `xml:"location,attr"`
				Text     string `xml:"text"`
			} `xml:"failed-assert"`
		}
		if err := xml.Unmarshal(report.Bytes(), &svrl); err != nil {
			t.Fatalf("%s %s: %v\n%s", rules, name, err, report.String())
		}
		for _, f := range svrl.Failed {
			out[name] = append(out[name], failure{Rules: rules, ID: f.ID, Flag: f.Flag, Location: qualified.ReplaceAllString(f.Location, ""), Text: strings.Join(strings.Fields(f.Text), " ")})
		}
		report.Reset()
	}
	sc := bufio.NewScanner(bytes.NewReader(raw))
	sc.Buffer(make([]byte, 1<<20), 1<<24)
	for sc.Scan() {
		line := sc.Text()
		if fields, ok := strings.CutPrefix(line, "=== "); ok {
			flush()
			parts := strings.Fields(fields)
			name = ""
			if len(parts) == 2 {
				rules, name = parts[0], strings.TrimSuffix(parts[1], ".xml")
			}
			continue
		}
		if name != "" {
			report.WriteString(line + "\n")
		}
	}
	return out
}

func TestUBLPassesPeppol(t *testing.T) {
	docs := map[string][]byte{}
	for name, doc := range peppolCases(t) {
		data, err := New().UBL(doc)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		docs[name] = data
	}
	for name, fails := range validatePeppol(t, docs) {
		for _, f := range fails {
			if f.Flag == "fatal" || f.Flag == "" {
				t.Errorf("%s breaks %s %s at %s: %s", name, f.Rules, f.ID, f.Location, f.Text)
			} else {
				t.Logf("%s: %s %s %s: %s", name, f.Flag, f.Rules, f.ID, f.Text)
			}
		}
	}
}

// peppolCases are the kinds of invoices a Dutch business sends.
func peppolCases(t *testing.T) map[string]sales.Document {
	t.Helper()
	nlRates := map[resource.ID]bookkeeping.TaxRate{
		1: {Percentage: "21", ShowTax: true, ReportBoxes: []string{"NL/1a"}},
		2: {Percentage: "9.0", ShowTax: true, ReportBoxes: []string{"NL/1b"}},
		3: {Percentage: "0", ShowTax: true, ReportBoxes: []string{"NL/1e"}},
		4: {Percentage: "0", ReportBoxes: []string{"NL/1e"}},
		5: {Percentage: "0", ReportBoxes: []string{"NL/3b"}},
		6: {Percentage: "0", ReportBoxes: []string{"NL/3a"}},
	}
	base := func(lines ...sales.Detail) sales.Document {
		doc := document(t, "en")
		doc.Identity.Zipcode, doc.Identity.ChamberOfCommerce = "7241AA", "12345678"
		doc.Contact = contacts.Contact{CompanyName: "Example Shop B.V.", Address1: "Dam 1", Zipcode: "1012JS", City: "Amsterdam", Country: "NL",
			CustomerID: "7", ChamberOfCommerce: "87654321", TaxNumber: "NL123456782B01"}
		doc.Rates = nlRates
		if len(lines) > 0 {
			doc.Invoice.Lines = lines
		}
		doc.Totals = sales.ComputeInvoice(doc.Invoice, doc.Rates)
		return doc
	}
	line := func(price money.Amount, qty int64, rate resource.ID) sales.Detail {
		return sales.Detail{Description: "Consultancy", Amount: "1", Quantity: qty, Price: price, TaxRateID: rate, Selected: true}
	}
	cases := map[string]sales.Document{}
	cases["domestic"] = base()
	cases["zero-rated"] = base(line(10000, 1000, 3))

	belgian := base()
	belgian.Contact.Country, belgian.Contact.TaxNumber, belgian.Contact.SIIdentifier, belgian.Contact.SIIdentifierType = "BE", "BE0123456749", "0123456749", "0208"
	cases["belgian-buyer"] = belgian

	noPeppolID := base()
	noPeppolID.Contact.ChamberOfCommerce = ""
	cases["buyer-by-vat-number"] = noPeppolID

	noKvK := base()
	noKvK.Identity.ChamberOfCommerce = ""
	cases["seller-by-vat-number"] = noKvK

	cases["reverse-charge"] = base(line(10000, 1000, 4))
	intraEU := base(line(10000, 2000, 5))
	intraEU.Contact = contacts.Contact{CompanyName: "Beispiel GmbH", Address1: "Hauptstraße 1", Zipcode: "10115", City: "Berlin", Country: "DE", CustomerID: "8", TaxNumber: "DE123456789"}
	cases["intra-eu"] = intraEU
	export := base(line(10000, 1000, 6))
	export.Contact = contacts.Contact{CompanyName: "Example Inc.", Address1: "1 Main Street", Zipcode: "10001", City: "New York", Country: "US", CustomerID: "9",
		SIIdentifier: "4000001000005", SIIdentifierType: "0088"}
	cases["export"] = export

	credit := base(line(-10000, 1000, 1), line(-5000, 3000, 2))
	original := resource.ID(41)
	credit.Invoice.OriginalID = &original
	credit.Totals = sales.ComputeInvoice(credit.Invoice, credit.Rates)
	credit.Credits = &sales.Credited{InvoiceID: "2026-0000", InvoiceDate: credit.Invoice.InvoiceDate}
	cases["credit-invoice"] = credit

	// An invoice of less than nothing without one to credit, and one that takes a deposit off.
	cases["negative-invoice"] = base(line(-10000, 1000, 1))
	cases["deposit-taken-off"] = base(line(50000, 1000, 1), line(-20000, 1000, 1))

	discount := base(line(3333, 3000, 1), line(1999, 7000, 2))
	discount.Invoice.Discount = 1000
	discount.Totals = sales.ComputeInvoice(discount.Invoice, discount.Rates)
	cases["discount"] = discount

	inclusive := base(line(1210, 3000, 1), line(999, 1000, 2))
	inclusive.Invoice.PricesAreInclTax = true
	inclusive.Totals = sales.ComputeInvoice(inclusive.Invoice, inclusive.Rates)
	cases["prices-incl-vat"] = inclusive

	paid := base()
	paid.Invoice.Payments = []sales.Payment{{Price: 10000}}
	cases["partly-paid"] = paid
	return cases
}

func TestUBLStatesTheVATCategory(t *testing.T) {
	cases := peppolCases(t)
	for name, want := range map[string]string{
		"domestic": "S", "zero-rated": "Z", "reverse-charge": "AE", "intra-eu": "K", "export": "G",
	} {
		data, err := New().UBL(cases[name])
		if err != nil {
			t.Fatal(err)
		}
		var got struct {
			Subtotals []struct {
				ID         string `xml:"TaxCategory>ID"`
				ReasonCode string `xml:"TaxCategory>TaxExemptionReasonCode"`
			} `xml:"TaxTotal>TaxSubtotal"`
		}
		if err := xml.Unmarshal(data, &got); err != nil {
			t.Fatal(err)
		}
		if len(got.Subtotals) == 0 || got.Subtotals[len(got.Subtotals)-1].ID != want {
			t.Errorf("%s: categories %+v, want %s", name, got.Subtotals, want)
		}
	}
	// Why no VAT is charged follows the document's language.
	doc := cases["reverse-charge"]
	doc.Invoice.Language = "nl"
	data, _ := New().UBL(doc)
	if !strings.Contains(string(data), "<cbc:TaxExemptionReason>Btw verlegd</cbc:TaxExemptionReason>") {
		t.Errorf("no Dutch reason:\n%s", data)
	}
}

func TestUBLCreditNoteStatesPositiveAmounts(t *testing.T) {
	data, err := New().UBL(peppolCases(t)["credit-invoice"])
	if err != nil {
		t.Fatal(err)
	}
	var got struct {
		XMLName  xml.Name
		TypeCode string `xml:"CreditNoteTypeCode"`
		Billing  string `xml:"BillingReference>InvoiceDocumentReference>ID"`
		Payable  string `xml:"LegalMonetaryTotal>PayableAmount"`
	}
	if err := xml.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	// 100.00 at 21% and 3 × 50.00 at 9%: 121.00 + 163.50.
	if got.XMLName.Local != "CreditNote" || got.TypeCode != "381" || got.Billing != "2026-0000" || got.Payable != "284.50" {
		t.Errorf("credit note = %+v", got)
	}
}
