package invoicedoc

import (
	"bytes"
	"fmt"
	"testing"

	"github.com/adirkuhn/moneyharpy/internal/administration"
	"github.com/adirkuhn/moneyharpy/internal/contacts"
	"github.com/adirkuhn/moneyharpy/internal/invoicing"
	"github.com/adirkuhn/moneyharpy/internal/sales"
	gocmp "github.com/google/go-cmp/cmp"
)

func styled(t *testing.T) sales.Document {
	t.Helper()
	doc := document(t, "en")
	doc.Style.ID = 9
	doc.Style.Layout = invoicing.DefaultLayout()
	doc.Identity.Email, doc.Identity.ChamberOfCommerce = "info@example.com", "12345678"
	doc.Identity.CustomFields = []administration.CustomFieldValue{{ID: 700, Name: "Brand", Value: "Harpy Shop"}}
	doc.Contact.TaxNumber, doc.Contact.SendInvoicesToEmail = "BE0123456789", "billing@example.com"
	doc.Contact.CustomFields = []contacts.FieldValue{{FieldID: 702, Name: "Site", Value: "Antwerp"}}
	doc.Invoice.CustomFields = []sales.FieldValue{{FieldID: 701, Name: "Project", Value: "Shop"}}
	return doc
}

func TestSenderFollowsTheLayout(t *testing.T) {
	doc := styled(t)
	doc.Style.InvoiceSenderAddress = []invoicing.Line{
		{Field: "custom_field_700", Bold: true, Italic: true}, {Field: "address"}, {Field: "newline"}, {Field: "website"},
		{Field: "email", Label: true}, {Field: "chamber_of_commerce", Label: true},
	}
	got := senderLines(doc, english)
	want := []styledLine{
		{Text: []string{"Harpy Shop"}, Bold: true, Italic: true},
		{Text: []string{"Oosterwal 1", "Lochem", "Netherlands"}},
		{},
		{Label: "Email", Text: []string{"info@example.com"}},
		{Label: "CoC", Text: []string{"12345678"}},
	}
	if diff := gocmp.Diff(want, got); diff != "" {
		t.Errorf("senderLines() diff (-want +got):\n%s", diff)
	}
	doc.Style = invoicing.DocumentStyle{}
	if got := senderLines(doc, english); len(got) == 0 || got[0].Text[0] != "Harpy B.V." || !got[0].Bold {
		t.Errorf("without a style = %+v, want the default layout", got)
	}
}

func TestRecipientInfoAndMetadata(t *testing.T) {
	doc := styled(t)
	doc.Style.InvoiceRecipientAddress = []invoicing.Line{{Field: "customer_tax_number", Label: true}, {Field: "custom_field_702"}, {Field: "customer_phone"}}
	if diff := gocmp.Diff([]styledLine{{Label: "VAT number", Text: []string{"BE0123456789"}}, {Text: []string{"Antwerp"}}}, recipientInfo(doc, english)); diff != "" {
		t.Errorf("recipientInfo() diff (-want +got):\n%s", diff)
	}
	doc.Style.InvoiceMetadataLeft = []invoicing.Line{{Field: "reference"}, {Field: "custom_field_701"}, {Field: "customer_email"}}
	doc.Style.InvoiceMetadataRight = []invoicing.Line{{Field: "invoice_date"}, {Field: "due_date"}}
	want := [][2]string{
		{"Invoice number", "2026-0001"}, {"Reference", "Order 7"}, {"Project", "Shop"}, {"Email address", "billing@example.com"},
		{"Invoice date", "11-11-2026"}, {"Due date", "25-11-2026"},
	}
	if diff := gocmp.Diff(want, metaRows(doc, english, true)); diff != "" {
		t.Errorf("metaRows() diff (-want +got):\n%s", diff)
	}
	if rows := metaRows(doc, english, false); len(rows) != 5 {
		t.Errorf("packing slip rows = %v, want no due date", rows)
	}
}

func TestEPCPayload(t *testing.T) {
	doc := styled(t)
	doc.Identity.BankAccountBIC, doc.Identity.BankAccountNumber = "ABNANL2A", "NL91 ABNA 0417 1643 00"
	want := "BCD\n002\n1\nSCT\nABNANL2A\nHarpy B.V.\nNL91ABNA0417164300\nEUR" + fmt.Sprintf("%d.%02d", doc.Totals.InclTax/100, doc.Totals.InclTax%100) + "\n\n\n2026-0001"
	if got, ok := epcPayload(doc); !ok || got != want {
		t.Errorf("epcPayload() = %q, %v; want %q", got, ok, want)
	}
	doc.Style.ShowEPCCode = false
	if _, ok := epcPayload(doc); ok {
		t.Error("QR code with show_epc_code off")
	}
	doc.Style.ShowEPCCode, doc.Invoice.Currency = true, "USD"
	if _, ok := epcPayload(doc); ok {
		t.Error("QR code for a dollar invoice")
	}
}

func TestPDFHidesTheSender(t *testing.T) {
	doc := styled(t)
	shown, err := New().PDF(doc)
	if err != nil {
		t.Fatal(err)
	}
	doc.Style.HideSenderAddress = true
	hidden, err := New().PDF(doc)
	if err != nil || !bytes.HasPrefix(hidden, []byte("%PDF-")) || len(hidden) >= len(shown) {
		t.Errorf("hidden sender: %d bytes, shown %d, %v", len(hidden), len(shown), err)
	}
}
