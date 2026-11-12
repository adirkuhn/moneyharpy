package sales

import (
	"context"
	"strings"

	"github.com/adirkuhn/moneyharpy/internal/invoicing"
	"github.com/adirkuhn/moneyharpy/internal/mail"
)

// MailClient sends invoices with the instance's mail settings.
type MailClient struct{ Client *mail.Client }

// SendDocument implements Mailer.
func (m MailClient) SendDocument(ctx context.Context, msg Mail) error {
	return m.Client.SendDocument(ctx, mail.Document{
		To: msg.To, BCC: msg.BCC, ReplyTo: msg.ReplyTo, FromName: msg.FromName, Subject: msg.Subject, Body: msg.Body,
		Files: mailFiles(msg.Files), Look: mailLook(msg.Look),
	})
}

func mailFiles(files []File) []mail.Attachment {
	out := make([]mail.Attachment, len(files))
	for i, f := range files {
		out[i] = mail.Attachment{Name: f.Name, ContentType: f.ContentType, Data: f.Data}
	}
	return out
}

// FontStacks are the CSS font lists of a document style's fonts.
var FontStacks = map[string]string{
	"lucida_grande": "'Lucida Grande', 'Lucida Sans Unicode', Arial, sans-serif", "helvetica": "Helvetica, Arial, sans-serif",
	"arial": "Arial, Helvetica, sans-serif", "verdana": "Verdana, Geneva, sans-serif", "georgia": "Georgia, serif",
	"times_new_roman": "'Times New Roman', Times, serif", "courier": "'Courier New', Courier, monospace",
}

// mailLook turns an email's look into what the mail client draws.
func mailLook(l *EmailLook) *mail.Look {
	if l == nil {
		return nil
	}
	e, id := l.Style, l.Sender
	image := func(kind invoicing.FileKind) *mail.Attachment {
		f, ok := l.Images[kind]
		if !ok {
			return nil
		}
		return &mail.Attachment{Name: f.Name, ContentType: f.ContentType, Data: f.Data}
	}
	label := ""
	if e.LabelEnabled {
		label = "Sent with MoneyHarpy"
		if strings.HasPrefix(l.Language, "nl") {
			label = "Verzonden met MoneyHarpy"
		}
	}
	var contact []string
	for _, line := range []string{id.CompanyName, id.Address1, id.Address2, strings.TrimSpace(id.Zipcode + " " + id.City), id.Email, id.Phone} {
		if strings.TrimSpace(line) != "" {
			contact = append(contact, line)
		}
	}
	return &mail.Look{
		HeaderType: e.HeaderType, HeaderAlign: e.HeaderContentPosition, HeaderBackground: e.HeaderBackgroundColor, HeaderText: e.HeaderText,
		HeaderTextColor: e.HeaderTextColor, LogoWidth: e.LogoDisplayWidth, FooterType: e.FooterType, FooterTextColor: e.FooterTextColor,
		FooterBackground: e.FooterBackgroundColor, ProfileTitle: e.FooterProfileTitle, ProfileSubtitle: e.FooterProfileSubtitle,
		PageBackground: e.BodyBackgroundColor, FontStack: FontStacks[e.FontFamily], Label: label,
		Logo: image(invoicing.EmailLogo), HeaderImage: image(invoicing.EmailHeaderBackground),
		ProfilePicture: image(invoicing.EmailFooterProfilePicture), FooterImage: image(invoicing.EmailFooterBackground),
		ContactLines: contact, Details: l.Details,
	}
}
