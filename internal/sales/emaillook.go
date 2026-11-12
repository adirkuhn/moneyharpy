package sales

import (
	"context"
	"io"
	"strings"
	"time"

	"github.com/adirkuhn/moneyharpy/internal/administration"
	"github.com/adirkuhn/moneyharpy/internal/invoicing"
	"github.com/adirkuhn/moneyharpy/internal/resource"
)

// EmailLook is how a document email looks: the email settings of the
// document's style, the images they show, the sender for a contact footer
// and the document's details listed under the text.
type EmailLook struct {
	Style    invoicing.EmailStyle
	Images   map[invoicing.FileKind]File
	Sender   administration.Identity
	Language string
	Details  [][2]string
}

// emailImages are the images an email can show, besides the style's logo
// which stands in for a missing email logo.
var emailImages = []invoicing.FileKind{invoicing.EmailLogo, invoicing.EmailHeaderBackground, invoicing.EmailFooterProfilePicture, invoicing.EmailFooterBackground}

// emailLook gathers the look of an email carrying doc. A document without a
// style emails in the default look.
func (s *Service) emailLook(ctx context.Context, admin, user resource.ID, doc Document, details [][2]string) *EmailLook {
	look := &EmailLook{Style: invoicing.DefaultEmailStyle(), Images: map[invoicing.FileKind]File{}, Sender: doc.Identity, Language: doc.Invoice.Language, Details: details}
	if doc.Style.ID == 0 {
		return look
	}
	look.Style = doc.Style.Email
	for _, kind := range emailImages {
		if doc.Style.File(kind) == nil {
			continue
		}
		if f, ok := s.styleImage(ctx, admin, doc.Style.ID, user, kind); ok {
			look.Images[kind] = f
		}
	}
	if _, ok := look.Images[invoicing.EmailLogo]; !ok && doc.Style.Logo != nil {
		if len(doc.LogoData) > 0 {
			look.Images[invoicing.EmailLogo] = File{Name: "logo", ContentType: doc.LogoType, Data: doc.LogoData}
		} else if f, ok := s.styleImage(ctx, admin, doc.Style.ID, user, invoicing.Logo); ok {
			look.Images[invoicing.EmailLogo] = f
		}
	}
	return look
}

// styleImage reads one of a style's images; a missing one is left out so
// the email still goes.
func (s *Service) styleImage(ctx context.Context, admin, id, user resource.ID, kind invoicing.FileKind) (File, bool) {
	body, contentType, err := s.invoicing.OpenFile(ctx, admin, id, user, kind)
	if err != nil {
		return File{}, false
	}
	defer func() { _ = body.Close() }()
	data, err := io.ReadAll(io.LimitReader(body, 5<<20))
	if err != nil || len(data) == 0 {
		return File{}, false
	}
	return File{Name: string(kind), ContentType: contentType, Data: data}, true
}

type detailLabels struct{ Invoice, Quote, Sender, Reference, InvoiceDate, QuoteDate, DueDate, ValidUntil, AmountDue, Total string }

var (
	dutchDetails   = detailLabels{"Factuur", "Offerte", "Afzender", "Referentie", "Factuurdatum", "Offertedatum", "Vervaldatum", "Geldig tot", "Openstaand bedrag", "Totaal"}
	englishDetails = detailLabels{"Invoice", "Quote", "Sender", "Reference", "Invoice date", "Quote date", "Due date", "Valid until", "Amount due", "Total"}
)

func detailsFor(language string) detailLabels {
	if strings.HasPrefix(language, "nl") {
		return dutchDetails
	}
	return englishDetails
}

func sender(id administration.Identity) string {
	if id.Email == "" {
		return id.CompanyName
	}
	return id.CompanyName + " (" + id.Email + ")"
}

func day(t *time.Time) string {
	if t == nil {
		return ""
	}
	return t.Format("02-01-2006")
}

func nonEmptyRows(rows [][2]string) [][2]string {
	out := rows[:0]
	for _, r := range rows {
		if r[1] != "" {
			out = append(out, r)
		}
	}
	return out
}

// InvoiceDetails are the facts an invoice email lists under its text, as
// Moneybird's invoice email does.
func InvoiceDetails(doc Document) [][2]string {
	l, inv := detailsFor(doc.Invoice.Language), doc.Invoice
	unpaid := doc.Totals.InclTax - inv.Paid()
	return nonEmptyRows([][2]string{
		{l.Invoice, inv.InvoiceID}, {l.Sender, sender(doc.Identity)}, {l.Reference, inv.Reference},
		{l.InvoiceDate, day(inv.InvoiceDate)}, {l.DueDate, day(inv.DueDate)},
		{l.AmountDue, FormatMoney(unpaid, inv.Currency, inv.Language)},
	})
}

// EstimateDetails are the facts an estimate email lists under its text.
func EstimateDetails(doc Document) [][2]string {
	l, inv := detailsFor(doc.Invoice.Language), doc.Invoice
	return nonEmptyRows([][2]string{
		{l.Quote, inv.InvoiceID}, {l.Sender, sender(doc.Identity)}, {l.Reference, inv.Reference},
		{l.QuoteDate, day(inv.InvoiceDate)}, {l.ValidUntil, day(inv.DueDate)},
		{l.Total, FormatMoney(doc.Totals.InclTax, inv.Currency, inv.Language)},
	})
}
