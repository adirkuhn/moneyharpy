package invoicedoc

import (
	"testing"

	"github.com/adirkuhn/moneyharpy/internal/sales"
)

// A quote is printed in the invoice's layout, titled as a quote, with its
// due date as Moneybird prints it ("Vervaldatum"), without the payment
// notice, and with its texts' tags filled in.
func TestQuotePDF(t *testing.T) {
	doc := example(t, "en")
	doc.Invoice.InvoiceID = "Q2026-0007"
	doc.Estimate = &sales.Estimate{PreText: "Dear {recipient.name},", PostText: "Quote *{document.estimate_id}* stays valid.", ShowTax: true}
	r := layoutOf(t, doc, true)
	r.one(t, "Quote Q2026-0007")
	r.one(t, "Due date:")
	r.one(t, "Dear Jan Voorbeeld,")
	r.one(t, "Q2026-0007")
	if len(r.find("Valid until:")) != 0 || len(r.containing("We kindly request")) != 0 || len(r.containing("{document")) != 0 {
		t.Errorf("quote texts = %v", r.texts())
	}
	doc.Invoice.Language, doc.Invoice.InvoiceID = "nl", ""
	r = layoutOf(t, doc, true)
	r.one(t, "Conceptofferte")
	r.one(t, "Vervaldatum:")
}
