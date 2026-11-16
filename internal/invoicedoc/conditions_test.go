package invoicedoc

import (
	"strings"
	"testing"
)

func TestPDFPrintsStyledPaymentConditions(t *testing.T) {
	doc := document(t, "en")
	doc.Invoice.PaymentConditions = "Pay {document.total_price} _before_ {document.due_date},\nquoting *{document.payment_reference}*."
	r := layoutOf(t, doc, true)
	// The faces have no italic, so emphasis prints upright; strong text is
	// bold.
	before := r.one(t, "before")
	quoting := r.containing("quoting")
	reference := r.find("2026-0001")
	if before.Bold || len(quoting) != 1 || quoting[0].Bold {
		t.Errorf("conditions %+v %+v, want regular text", before, quoting)
	}
	if len(reference) == 0 || !reference[len(reference)-1].Bold || reference[len(reference)-1].Y <= before.Y {
		t.Errorf("reference %+v, want bold on the second line", reference)
	}
	if _, err := New().PDF(doc); err != nil {
		t.Fatal(err)
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
