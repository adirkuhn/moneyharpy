package invoicedoc

import (
	"bytes"
	"encoding/xml"
	"image"
	"image/color"
	"image/png"
	"strings"
	"testing"
	"time"

	"github.com/adirkuhn/moneyharpy/internal/administration"
	"github.com/adirkuhn/moneyharpy/internal/bookkeeping"
	"github.com/adirkuhn/moneyharpy/internal/contacts"
	"github.com/adirkuhn/moneyharpy/internal/invoicing"
	"github.com/adirkuhn/moneyharpy/internal/resource"
	"github.com/adirkuhn/moneyharpy/internal/sales"
)

func document(t *testing.T, language string) sales.Document {
	t.Helper()
	day := time.Date(2026, 11, 11, 0, 0, 0, 0, time.UTC)
	due := day.AddDate(0, 0, 14)
	inv := sales.Invoice{
		InvoiceID: "2026-0001", Language: language, Currency: "EUR", InvoiceDate: &day, DueDate: &due, PaymentReference: "2026-0001",
		Reference: "Order 7", Lines: []sales.Detail{
			{Description: "Consultancy november\nOn site", Amount: "2 uur", Quantity: 2000, Price: 10000, TaxRateID: 1, Selected: true},
			{Description: "Café visit, € included", Amount: "1", Quantity: 1000, Price: 5000, TaxRateID: 2, Selected: true},
		},
	}
	rates := map[resource.ID]bookkeeping.TaxRate{1: {Percentage: "21"}, 2: {Percentage: "9.0"}}
	var logo bytes.Buffer
	img := image.NewRGBA(image.Rect(0, 0, 40, 20))
	img.Set(1, 1, color.RGBA{255, 0, 0, 255})
	_ = png.Encode(&logo, img)
	return sales.Document{
		Administration: administration.Administration{Country: "NL"},
		Invoice:        inv, Rates: rates, Totals: sales.ComputeInvoice(inv, rates),
		Contact:  contacts.Contact{CompanyName: "Example Shop B.V.", Address1: "Dam 1", Zipcode: "1012JS", City: "Amsterdam", Country: "BE", CustomerID: "7", SIIdentifier: "0123456789", SIIdentifierType: "0208"},
		Identity: administration.Identity{CompanyName: "Harpy B.V.", Address1: "Oosterwal 1", City: "Lochem", Country: "NL", BankAccountNumber: "NL91ABNA0417164300", TaxNumber: "NL000099998B57"},
		Style:    invoicing.DocumentStyle{LogoPosition: "right", LogoDisplayWidth: 40, FontSize: 10},
		LogoType: "image/png", LogoData: logo.Bytes(),
	}
}

func TestPDF(t *testing.T) {
	for _, lang := range []string{"nl", "en"} {
		data, err := New().PDF(document(t, lang))
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.HasPrefix(data, []byte("%PDF-")) || len(data) < 1000 {
			t.Errorf("%s: not a PDF (%d bytes)", lang, len(data))
		}
	}
	plain, _ := New().PDF(document(t, "en"))
	doc := document(t, "en")
	doc.Stationery = true
	stationery, err := New().PDF(doc)
	if err != nil || !bytes.HasPrefix(stationery, []byte("%PDF-")) || len(stationery) >= len(plain) {
		t.Errorf("stationery: %d bytes, plain %d, %v; want a smaller PDF without the logo", len(stationery), len(plain), err)
	}
	slip, err := New().PackingSlip(document(t, "en"))
	if err != nil || !bytes.HasPrefix(slip, []byte("%PDF-")) {
		t.Errorf("packing slip: %v", err)
	}
}

func TestPDFWithoutLogoOrBank(t *testing.T) {
	doc := document(t, "en")
	doc.LogoData, doc.Identity.BankAccountNumber = nil, ""
	if _, err := New().PDF(doc); err != nil {
		t.Fatal(err)
	}
	doc.LogoData, doc.LogoType = []byte("not an image"), "image/png"
	if _, err := New().PDF(doc); err != nil {
		t.Errorf("broken logo: %v", err)
	}
}

func TestUBL(t *testing.T) {
	data, err := New().UBL(document(t, "en"))
	if err != nil {
		t.Fatal(err)
	}
	var got struct {
		ID       string `xml:"ID"`
		TypeCode string `xml:"InvoiceTypeCode"`
		TaxTotal struct {
			Amount    string `xml:"TaxAmount"`
			Subtotals []struct {
				Taxable string `xml:"TaxableAmount"`
			} `xml:"TaxSubtotal"`
		} `xml:"TaxTotal"`
		Totals struct {
			Payable string `xml:"PayableAmount"`
		} `xml:"LegalMonetaryTotal"`
		Lines []struct {
			Quantity string `xml:"InvoicedQuantity"`
		} `xml:"InvoiceLine"`
		Customer struct {
			Party struct {
				Endpoint string `xml:"EndpointID"`
			} `xml:"Party"`
		} `xml:"AccountingCustomerParty"`
	}
	if err := xml.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	// 200.00 at 21% (42.00) and 50.00 at 9% (4.50).
	if got.ID != "2026-0001" || got.TypeCode != "380" || got.TaxTotal.Amount != "46.50" || got.Totals.Payable != "296.50" || len(got.Lines) != 2 || got.Lines[0].Quantity != "2.000" {
		t.Errorf("ubl = %+v", got)
	}
	if got.Customer.Party.Endpoint != "0123456789" || len(got.TaxTotal.Subtotals) != 2 {
		t.Errorf("parties = %+v", got)
	}
	if !strings.Contains(string(data), `schemeID="0208"`) {
		t.Error("no endpoint scheme")
	}
}
