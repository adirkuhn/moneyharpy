package mail

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"mime"
	"mime/multipart"
	stdmail "net/mail"
	"net/textproto"
	"sort"
	"strings"
	"time"
)

// mimePart is one part of a message: its headers and encoded body.
type mimePart struct {
	header textproto.MIMEHeader
	body   []byte
}

func quotedPart(contentType, text string) (mimePart, error) {
	var body bytes.Buffer
	if err := writeQuoted(&body, text); err != nil {
		return mimePart{}, err
	}
	return mimePart{textproto.MIMEHeader{"Content-Type": {contentType}, "Content-Transfer-Encoding": {"quoted-printable"}}, body.Bytes()}, nil
}

func base64Part(header textproto.MIMEHeader, data []byte) mimePart {
	encoded := base64.StdEncoding.EncodeToString(data)
	var lines bytes.Buffer
	for len(encoded) > 76 {
		lines.WriteString(encoded[:76] + "\r\n")
		encoded = encoded[76:]
	}
	lines.WriteString(encoded + "\r\n")
	header.Set("Content-Transfer-Encoding", "base64")
	return mimePart{header, lines.Bytes()}
}

// multipartOf joins parts into one multipart part of the given subtype,
// such as mixed, related or alternative.
func multipartOf(subtype string, parts ...mimePart) (mimePart, error) {
	var body bytes.Buffer
	w := multipart.NewWriter(&body)
	for _, p := range parts {
		dst, err := w.CreatePart(p.header)
		if err != nil {
			return mimePart{}, fmt.Errorf("start part: %w", err)
		}
		if _, err := dst.Write(p.body); err != nil {
			return mimePart{}, fmt.Errorf("write part: %w", err)
		}
	}
	if err := w.Close(); err != nil {
		return mimePart{}, fmt.Errorf("finish part: %w", err)
	}
	return mimePart{textproto.MIMEHeader{"Content-Type": {"multipart/" + subtype + "; boundary=" + w.Boundary()}}, body.Bytes()}, nil
}

// bodyPart is the message text: plain, or with a look plain and HTML with
// the images the HTML shows.
func bodyPart(m message) (mimePart, error) {
	text, err := quotedPart("text/plain; charset=utf-8", m.body)
	if err != nil || m.look == nil {
		return text, err
	}
	html, inline := renderHTML(m.body, *m.look)
	if html == "" {
		return text, nil
	}
	rich, err := quotedPart("text/html; charset=utf-8", html)
	if err != nil {
		return mimePart{}, err
	}
	alternative, err := multipartOf("alternative", text, rich)
	if err != nil || len(inline) == 0 {
		return alternative, err
	}
	parts := []mimePart{alternative}
	for _, img := range inline {
		parts = append(parts, base64Part(textproto.MIMEHeader{
			"Content-Type":        {mime.FormatMediaType(img.ContentType, map[string]string{"name": img.Name})},
			"Content-ID":          {"<" + img.id + ">"},
			"Content-Disposition": {mime.FormatMediaType("inline", map[string]string{"filename": img.Name})},
		}, img.Data))
	}
	return multipartOf("related", parts...)
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

	top, err := bodyPart(m)
	if err != nil {
		return nil, err
	}
	if len(m.files) > 0 {
		parts := []mimePart{top}
		for _, f := range m.files {
			parts = append(parts, base64Part(textproto.MIMEHeader{
				"Content-Type":        {mime.FormatMediaType(f.ContentType, map[string]string{"name": f.Name})},
				"Content-Disposition": {mime.FormatMediaType("attachment", map[string]string{"filename": f.Name})},
			}, f.Data))
		}
		if top, err = multipartOf("mixed", parts...); err != nil {
			return nil, err
		}
	}
	names := make([]string, 0, len(top.header))
	for name := range top.header {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		header(name, top.header.Get(name))
	}
	out.WriteString("\r\n")
	out.Write(top.body)
	return out.Bytes(), nil
}
