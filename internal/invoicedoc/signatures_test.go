package invoicedoc

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"testing"
	"time"

	"github.com/adirkuhn/moneyharpy/internal/resource"
	"github.com/adirkuhn/moneyharpy/internal/sales"
)

func drawnSignature(t *testing.T) []byte {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, 60, 20))
	for x := range 60 {
		img.Set(x, 10, color.Black)
	}
	var out bytes.Buffer
	if err := png.Encode(&out, img); err != nil {
		t.Fatal(err)
	}
	return out.Bytes()
}

func TestSignedQuoteEndsWithTheSignatures(t *testing.T) {
	doc := example(t, "en")
	doc.Invoice.InvoiceID = "2026-0027"
	doc.Estimate = &sales.Estimate{ShowTax: true}
	at := time.Date(2026, 11, 16, 14, 30, 0, 0, time.UTC)
	owner := resource.ID(1)
	doc.Signing = &sales.Signing{
		SignerName: "Jan Voorbeeld", SignerEmail: "jan@example.com", SignerIP: "203.0.113.7", SignerUserAgent: "Test browser",
		Signature: drawnSignature(t), SignedAt: &at, SenderSignature: drawnSignature(t), SenderSignedAt: &at, SenderUserID: &owner,
	}
	r := layoutOf(t, doc, true)
	heading := r.one(t, "Signatures")
	for _, want := range []string{"Signed by the customer", "Signed by Example Shop B.V.", "Jan Voorbeeld", "jan@example.com", "203.0.113.7", "Test browser"} {
		if found := r.containing(want); len(found) == 0 {
			t.Errorf("signature page lacks %q: %v", want, r.texts())
		} else if found[len(found)-1].Page != heading.Page {
			t.Errorf("%q on page %d, signatures on %d", want, found[len(found)-1].Page, heading.Page)
		}
	}
	if heading.Page < 2 {
		t.Errorf("signatures on page %d, want a page of their own", heading.Page)
	}
	if len(r.Images) < 2 {
		t.Errorf("images = %d, want both signatures", len(r.Images))
	}

	// Without the sender's signature, only the customer's.
	doc.Signing.SenderSignature, doc.Signing.SenderSignedAt = nil, nil
	doc.Invoice.Language = "nl"
	nl := layoutOf(t, doc, true)
	nl.one(t, "Handtekeningen")
	if len(nl.containing("Ondertekend door Example Shop B.V.")) != 0 {
		t.Errorf("sender block without a signature: %v", nl.texts())
	}
}
