package sales

import (
	"context"

	"github.com/adirkuhn/moneyharpy/internal/mail"
)

// MailClient sends invoices with the instance's mail settings.
type MailClient struct{ Client *mail.Client }

// SendDocument implements Mailer.
func (m MailClient) SendDocument(ctx context.Context, msg Mail) error {
	files := make([]mail.Attachment, len(msg.Files))
	for i, f := range msg.Files {
		files[i] = mail.Attachment{Name: f.Name, ContentType: f.ContentType, Data: f.Data}
	}
	return m.Client.SendDocument(ctx, mail.Document{
		To: msg.To, BCC: msg.BCC, ReplyTo: msg.ReplyTo, FromName: msg.FromName, Subject: msg.Subject, Body: msg.Body, Files: files,
	})
}
