package invoicedoc

import (
	"bytes"
	"strings"
	"testing"
)

func TestPDFPrintsStyledPaymentConditions(t *testing.T) {
	doc := document(t, "en")
	plain, err := New().PDF(doc)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(plain, []byte("Helvetica-Oblique")) {
		t.Fatal("italic font without italic text")
	}
	doc.Invoice.PaymentConditions = "Pay {document.total_price} _before_ {document.due_date},\nquoting *{document.payment_reference}*."
	styled, err := New().PDF(doc)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(styled, []byte("Helvetica-Oblique")) || len(styled) <= len(plain) {
		t.Errorf("styled conditions: %d bytes, plain %d; want the italic font", len(styled), len(plain))
	}
}

func TestEPCPayloadUsesTheShortReference(t *testing.T) {
	doc := styled(t)
	doc.Identity.BankAccountBIC, doc.Identity.BankAccountNumber = "ABNANL2A", "NL91ABNA0417164300"
	doc.Invoice.PaymentReference = "RF130000000000000PV96X5KP"
	got, ok := epcPayload(doc)
	if !ok || !strings.HasSuffix(got, "\nRF13PV96X5KP") {
		t.Errorf("epcPayload() = %q, %v", got, ok)
	}
	doc.Style.ShowEPCCode = true
	if _, err := New().PDF(doc); err != nil {
		t.Errorf("PDF with QR code and conditions: %v", err)
	}
}
