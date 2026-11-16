package invoicedoc

import (
	"bytes"
	"strings"
	"time"

	"github.com/go-pdf/fpdf"
)

// signatureLabels name the page of a signed quote.
type signatureLabels struct {
	Title, Customer, Sender, Name, Email, Address, Browser, SignedAt string
}

var signatureText = map[bool]signatureLabels{
	true: {
		Title: "Handtekeningen", Customer: "Ondertekend door de klant", Sender: "Ondertekend door %s", Name: "Naam", Email: "E-mailadres",
		Address: "IP-adres", Browser: "Browser", SignedAt: "Ondertekend op",
	},
	false: {
		Title: "Signatures", Customer: "Signed by the customer", Sender: "Signed by %s", Name: "Name", Email: "Email address",
		Address: "IP address", Browser: "Browser", SignedAt: "Signed at",
	},
}

const (
	signatureW, signatureH = 70.0, 28.0
	signatureGap           = 10.0
)

// signatures adds the page of a quote signed online: the customer's
// signature with what was kept of the signing, and the sender's, as
// Moneybird shows both once the customer signed.
func (w *writer) signatures() {
	g := w.doc.Signing
	if g == nil || !g.Signed() {
		return
	}
	l := signatureText[strings.HasPrefix(w.doc.Invoice.Language, "nl")]
	pdf := w.pdf
	pdf.AddPage()
	pdf.SetY(top + 10)
	w.font("B", w.size+4)
	pdf.CellFormat(width, lineHeight*2, l.Title, "", 1, "L", false, 0, "")
	pdf.Ln(4)

	y := pdf.GetY()
	w.signatureBlock(left, y, l.Customer, "customer-signature", g.Signature, [][2]string{
		{l.Name, g.SignerName}, {l.Email, g.SignerEmail}, {l.SignedAt, stamp(g.SignedAt)}, {l.Address, g.SignerIP}, {l.Browser, g.SignerUserAgent},
	})
	end := pdf.GetY()
	if len(g.SenderSignature) > 0 && g.SenderSignedAt != nil {
		x := left + signatureW + signatureGap + 15
		w.signatureBlock(x, y, strings.Replace(l.Sender, "%s", w.doc.Identity.CompanyName, 1), "sender-signature", g.SenderSignature, [][2]string{
			{l.SignedAt, stamp(g.SenderSignedAt)},
		})
		end = max(end, pdf.GetY())
	}
	pdf.SetY(end)
}

// signatureBlock draws a heading, the signature on a line and its facts.
func (w *writer) signatureBlock(x, y float64, heading, name string, image []byte, facts [][2]string) {
	pdf := w.pdf
	pdf.SetXY(x, y)
	w.font("B", w.size)
	pdf.CellFormat(signatureW+15, lineHeight, heading, "", 2, "L", false, 0, "")
	top := pdf.GetY() + 2
	info := pdf.RegisterImageOptionsReader(name, fpdf.ImageOptions{ImageType: "PNG"}, bytes.NewReader(image))
	if pdf.Ok() && info != nil && info.Width() > 0 {
		iw, ih := signatureW, signatureW*info.Height()/info.Width()
		if ih > signatureH {
			iw, ih = signatureH*info.Width()/info.Height(), signatureH
		}
		pdf.ImageOptions(name, x, top+signatureH-ih, iw, ih, false, fpdf.ImageOptions{}, 0, "")
	} else {
		pdf.ClearError()
	}
	w.rule(top+signatureH+1, x, x+signatureW, thinRule)
	pdf.SetXY(x, top+signatureH+3)
	for _, f := range facts {
		if strings.TrimSpace(f[1]) == "" {
			continue
		}
		pdf.SetX(x)
		w.font("", w.size-1.5)
		w.color(mutedColor)
		pdf.CellFormat(25, lineHeight, f[0], "", 0, "L", false, 0, "")
		w.font("", w.size-1)
		pdf.MultiCell(signatureW+15-25, lineHeight, f[1], "", "L", false)
	}
}

// stamp is a moment as a signed page shows it, in UTC so it reads the same
// for both parties.
func stamp(t *time.Time) string {
	if t == nil {
		return ""
	}
	return t.UTC().Format("02-01-2006 15:04 UTC")
}
