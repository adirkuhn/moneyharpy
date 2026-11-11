// Package mail sends MoneyHarpy's email through the SMTP server configured in
// instance settings.
package mail

import (
	"bytes"
	"cmp"
	"context"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"mime/quotedprintable"
	"net"
	stdmail "net/mail"
	"net/smtp"
	"net/textproto"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/adirkuhn/moneyharpy/internal/settings"
)

const sendTimeout = 30 * time.Second

// ErrDisabled reports that email is not enabled.
var ErrDisabled = errors.New("email is disabled")

// Settings provides the configured mail server.
type Settings interface {
	Instance(ctx context.Context) (settings.Instance, error)
}

// Client sends email.
type Client struct {
	settings Settings
	baseURL  string
	rootCAs  *x509.CertPool // nil means the system roots.
}

// New returns a Client whose links point at baseURL.
func New(s Settings, baseURL string) *Client {
	return &Client{settings: s, baseURL: baseURL}
}

type message struct {
	subject string
	body    string
	// Documents also name the sender, a reply-to address, copies and files.
	cc       []string
	bcc      []string
	replyTo  string
	fromName string
	files    []Attachment
}

// Attachment is a file sent along with a document.
type Attachment struct {
	Name, ContentType string
	Data              []byte
}

// Document is an email carrying documents, such as an invoice with its PDF.
type Document struct {
	To, BCC       []string
	ReplyTo       string
	FromName      string
	Subject, Body string
	Files         []Attachment
}

// SendDocument emails documents in the name of the sender. The first To
// address is the recipient, the others get a visible copy.
func (c *Client) SendDocument(ctx context.Context, d Document) error {
	if len(d.To) == 0 {
		return errors.New("no recipient")
	}
	body := strings.ReplaceAll(strings.ReplaceAll(d.Body, "\r\n", "\n"), "\n", "\r\n")
	return c.send(ctx, d.To[0], message{subject: d.Subject, body: body, cc: d.To[1:], bcc: d.BCC, replyTo: d.ReplyTo, fromName: d.FromName, files: d.Files})
}

// SendPasswordReset emails a link for choosing a new password.
func (c *Client) SendPasswordReset(ctx context.Context, to, token, language string) error {
	link := c.baseURL + "/passwords/edit?token=" + url.QueryEscape(token)
	if strings.HasPrefix(language, "nl") {
		return c.send(ctx, to, message{
			subject: "Stel je MoneyHarpy-wachtwoord opnieuw in",
			body: "Iemand heeft gevraagd om het wachtwoord van je MoneyHarpy-account opnieuw in te stellen.\r\n\r\n" +
				"Kies een nieuw wachtwoord via deze link. De link werkt één keer en verloopt na een uur:\r\n" + link + "\r\n\r\n" +
				"Was jij dit niet? Dan kun je deze e-mail negeren. Je wachtwoord blijft hetzelfde.\r\n",
		})
	}
	return c.send(ctx, to, message{
		subject: "Reset your MoneyHarpy password",
		body: "Someone asked to reset the password of your MoneyHarpy account.\r\n\r\n" +
			"Choose a new password with this link. It works once and expires in one hour:\r\n" + link + "\r\n\r\n" +
			"If this was not you, ignore this email. Your password stays the same.\r\n",
	})
}

// InvitationURL is where an invitation is accepted.
func (c *Client) InvitationURL(token string) string {
	return c.baseURL + "/invitations/" + url.PathEscape(token)
}

// SendInvitation emails an invitation to join an administration.
func (c *Client) SendInvitation(ctx context.Context, to, token, administration, inviter, language string) error {
	link := c.InvitationURL(token)
	if strings.HasPrefix(language, "nl") {
		return c.send(ctx, to, message{
			subject: inviter + " nodigt je uit voor " + administration + " in MoneyHarpy",
			body: inviter + " nodigt je uit om mee te werken in de administratie " + administration + " in MoneyHarpy.\r\n\r\n" +
				"Accepteer de uitnodiging via deze link. Hij verloopt na 14 dagen:\r\n" + link + "\r\n\r\n" +
				"Verwachtte je dit niet? Dan kun je deze e-mail negeren.\r\n",
		})
	}
	return c.send(ctx, to, message{
		subject: inviter + " invited you to " + administration + " on MoneyHarpy",
		body: inviter + " invited you to work in the administration " + administration + " on MoneyHarpy.\r\n\r\n" +
			"Accept the invitation with this link. It expires in 14 days:\r\n" + link + "\r\n\r\n" +
			"If you did not expect this, ignore this email.\r\n",
	})
}

// SendTest emails a message confirming that the mail settings work.
func (c *Client) SendTest(ctx context.Context, to string) error {
	return c.send(ctx, to, message{
		subject: "MoneyHarpy test email",
		body:    "Your MoneyHarpy mail settings work.\r\n",
	})
}

func (c *Client) send(ctx context.Context, to string, m message) error {
	inst, err := c.settings.Instance(ctx)
	if err != nil {
		return fmt.Errorf("read settings: %w", err)
	}
	cfg := inst.SMTP
	if !cfg.Enabled {
		return ErrDisabled
	}
	recipients := append(append([]string{to}, m.cc...), m.bcc...)
	for _, r := range append(recipients, nonEmpty(m.replyTo)...) {
		if addr, err := stdmail.ParseAddress(r); err != nil || addr.Address != r {
			return errors.New("recipient is not a plain email address")
		}
	}
	if cfg.Security == "none" && cfg.Username != "" {
		return errors.New("refusing to send credentials without encryption")
	}
	raw, err := c.compose(cfg.From, to, m)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(ctx, sendTimeout)
	defer cancel()
	client, err := c.connect(ctx, cfg)
	if err != nil {
		return err
	}
	defer func() {
		// Close after Quit reports an already closed connection, which is fine.
		_ = client.Close()
	}()

	if cfg.Username != "" {
		if err := client.Auth(smtp.PlainAuth("", cfg.Username, cfg.Password, cfg.Host)); err != nil {
			return fmt.Errorf("authenticate: %w", err)
		}
	}
	if err := client.Mail(cfg.From); err != nil {
		return fmt.Errorf("set sender: %w", err)
	}
	for _, r := range recipients {
		if err := client.Rcpt(r); err != nil {
			return fmt.Errorf("set recipient: %w", err)
		}
	}
	w, err := client.Data()
	if err != nil {
		return fmt.Errorf("start message: %w", err)
	}
	if _, err := w.Write(raw); err != nil {
		return fmt.Errorf("write message: %w", err)
	}
	if err := w.Close(); err != nil {
		return fmt.Errorf("finish message: %w", err)
	}
	return client.Quit()
}

// connect opens an SMTP session with the encryption the settings ask for. With
// starttls, a server not offering it is an error rather than a downgrade.
func (c *Client) connect(ctx context.Context, cfg settings.SMTP) (*smtp.Client, error) {
	address := net.JoinHostPort(cfg.Host, strconv.Itoa(cfg.Port))
	tlsConfig := &tls.Config{ServerName: cfg.Host, RootCAs: c.rootCAs, MinVersion: tls.VersionTLS12}

	var (
		conn net.Conn
		err  error
	)
	if cfg.Security == "tls" {
		conn, err = (&tls.Dialer{Config: tlsConfig}).DialContext(ctx, "tcp", address)
	} else {
		conn, err = new(net.Dialer).DialContext(ctx, "tcp", address)
	}
	if err != nil {
		return nil, fmt.Errorf("connect: %w", err)
	}
	if deadline, ok := ctx.Deadline(); ok {
		if err := conn.SetDeadline(deadline); err != nil {
			_ = conn.Close() // The deadline error is the one worth reporting.
			return nil, fmt.Errorf("set deadline: %w", err)
		}
	}

	client, err := smtp.NewClient(conn, cfg.Host)
	if err != nil {
		_ = conn.Close() // The greeting error is the one worth reporting.
		return nil, fmt.Errorf("start session: %w", err)
	}
	if err := client.Hello(c.hostname()); err != nil {
		_ = client.Close() // The greeting error is the one worth reporting.
		return nil, fmt.Errorf("greet server: %w", err)
	}
	if cfg.Security == "starttls" {
		if ok, _ := client.Extension("STARTTLS"); !ok {
			_ = client.Close() // The missing extension is the one worth reporting.
			return nil, errors.New("server does not offer STARTTLS")
		}
		if err := client.StartTLS(tlsConfig); err != nil {
			_ = client.Close() // The handshake error is the one worth reporting.
			return nil, fmt.Errorf("start TLS: %w", err)
		}
	}
	return client, nil
}

func (c *Client) hostname() string {
	if u, err := url.Parse(c.baseURL); err == nil {
		return cmp.Or(u.Hostname(), "localhost")
	}
	return "localhost"
}

func (c *Client) compose(from, to string, m message) ([]byte, error) {
	var out bytes.Buffer
	header := func(name, value string) { fmt.Fprintf(&out, "%s: %s\r\n", name, value) }
	if m.fromName != "" {
		from = (&stdmail.Address{Name: m.fromName, Address: from}).String()
	}
	header("From", from)
	header("To", to)
	if len(m.cc) > 0 {
		header("Cc", strings.Join(m.cc, ", "))
	}
	if m.replyTo != "" {
		header("Reply-To", m.replyTo)
	}
	header("Subject", mime.QEncoding.Encode("utf-8", m.subject))
	header("Date", time.Now().Format(time.RFC1123Z))
	header("Message-ID", "<"+rand.Text()+"@"+c.hostname()+">")
	header("MIME-Version", "1.0")
	if len(m.files) == 0 {
		header("Content-Type", "text/plain; charset=utf-8")
		header("Content-Transfer-Encoding", "quoted-printable")
		out.WriteString("\r\n")
		if err := writeQuoted(&out, m.body); err != nil {
			return nil, err
		}
		return out.Bytes(), nil
	}
	parts := multipart.NewWriter(&out)
	header("Content-Type", "multipart/mixed; boundary="+parts.Boundary())
	out.WriteString("\r\n")
	text, err := parts.CreatePart(textproto.MIMEHeader{"Content-Type": {"text/plain; charset=utf-8"}, "Content-Transfer-Encoding": {"quoted-printable"}})
	if err != nil {
		return nil, fmt.Errorf("start body: %w", err)
	}
	if err := writeQuoted(text, m.body); err != nil {
		return nil, err
	}
	for _, f := range m.files {
		part, err := parts.CreatePart(textproto.MIMEHeader{
			"Content-Type":              {mime.FormatMediaType(f.ContentType, map[string]string{"name": f.Name})},
			"Content-Disposition":       {mime.FormatMediaType("attachment", map[string]string{"filename": f.Name})},
			"Content-Transfer-Encoding": {"base64"},
		})
		if err != nil {
			return nil, fmt.Errorf("start attachment: %w", err)
		}
		encoded := base64.StdEncoding.EncodeToString(f.Data)
		var lines strings.Builder
		for len(encoded) > 76 {
			lines.WriteString(encoded[:76] + "\r\n")
			encoded = encoded[76:]
		}
		lines.WriteString(encoded + "\r\n")
		if _, err := io.WriteString(part, lines.String()); err != nil {
			return nil, fmt.Errorf("write attachment: %w", err)
		}
	}
	if err := parts.Close(); err != nil {
		return nil, fmt.Errorf("finish message: %w", err)
	}
	return out.Bytes(), nil
}

func writeQuoted(dst io.Writer, body string) error {
	w := quotedprintable.NewWriter(dst)
	if _, err := w.Write([]byte(body)); err != nil {
		return fmt.Errorf("encode body: %w", err)
	}
	if err := w.Close(); err != nil {
		return fmt.Errorf("encode body: %w", err)
	}
	return nil
}

func nonEmpty(s string) []string {
	if s == "" {
		return nil
	}
	return []string{s}
}
