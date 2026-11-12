package mail

import (
	"context"
	"encoding/base64"
	"io"
	"mime"
	"mime/multipart"
	"mime/quotedprintable"
	stdmail "net/mail"
	"strings"
	"testing"
)

var gif = []byte("GIF89a\x01\x00\x01\x00")

func TestRenderHTMLStylesTheEmail(t *testing.T) {
	look := Look{
		HeaderType: "background_color_with_text", HeaderAlign: "center", HeaderBackground: "#112233", HeaderText: "Thanks <b>for</b> your order",
		HeaderTextColor: "#FFFFFF", FooterType: "contact_information", FooterTextColor: "#222222", FooterBackground: "#EEEEEE",
		PageBackground: "#F7F9FC", FontStack: "Georgia, serif", Label: "Sent with MoneyHarpy",
		ContactLines: []string{"Example Shop B.V.", "Main Street 1", "info@example.com"},
		Details:      [][2]string{{"Invoice", "2026-0001"}, {"Amount due", "€121.00"}},
	}
	html, inline := renderHTML("Dear Ann,\n\nSee <attached>.", look)
	for _, want := range []string{
		"background-color:#112233", "color:#FFFFFF", "text-align:center", "Thanks &lt;b&gt;for&lt;/b&gt; your order",
		"Dear Ann,", "See &lt;attached&gt;.", "Georgia, serif", "background-color:#F7F9FC",
		"Invoice", "2026-0001", "€121.00", "Example Shop B.V.", "Main Street 1", "background-color:#EEEEEE", "Sent with MoneyHarpy",
	} {
		if !strings.Contains(html, want) {
			t.Errorf("html lacks %q:\n%s", want, html)
		}
	}
	if len(inline) != 0 {
		t.Errorf("inline = %d, want none without images", len(inline))
	}
}

func TestRenderHTMLInlinesImages(t *testing.T) {
	logo := &Attachment{Name: "logo.gif", ContentType: "image/gif", Data: gif}
	picture := &Attachment{Name: "me.gif", ContentType: "image/gif", Data: gif}
	html, inline := renderHTML("Hello", Look{
		HeaderType: "background_image_with_logo", HeaderImage: &Attachment{Name: "h.gif", ContentType: "image/gif", Data: gif}, Logo: logo, LogoWidth: 40,
		FooterType: "signature", ProfilePicture: picture, ProfileTitle: "Ann de Vries", ProfileSubtitle: "Owner",
	})
	if len(inline) != 3 {
		t.Fatalf("inline = %d, want header, logo and profile picture", len(inline))
	}
	for _, f := range inline {
		if !strings.Contains(html, "cid:"+f.id) {
			t.Errorf("html does not use %s", f.id)
		}
	}
	for _, want := range []string{"width:40%", "Ann de Vries", "Owner"} {
		if !strings.Contains(html, want) {
			t.Errorf("html lacks %q", want)
		}
	}
	if strings.Contains(html, "Sent with") {
		t.Error("label shown although off")
	}
}

func TestRenderHTMLRefusesUnsafeColours(t *testing.T) {
	html, _ := renderHTML("Hi", Look{HeaderType: "background_color_with_text", HeaderBackground: "red;background:url(x)", PageBackground: "#FFF\"><script>"})
	if strings.Contains(html, "url(x)") || strings.Contains(html, "<script>") {
		t.Errorf("unsafe colour passed:\n%s", html)
	}
}

func TestDocumentMailWithLookIsHTML(t *testing.T) {
	cert, _ := testCert(t)
	srv := startServer(t, false, false, cert)
	err := client(srv.port(), "none", "", nil).SendDocument(context.Background(), Document{
		To: []string{"billing@example.com"}, Subject: "Invoice", Body: "Dear customer,\nSee attached.",
		Files: []Attachment{{Name: "invoice.pdf", ContentType: "application/pdf", Data: []byte("%PDF-1.4")}},
		Look:  &Look{HeaderType: "background_color_with_logo", Logo: &Attachment{Name: "logo.gif", ContentType: "image/gif", Data: gif}, PageBackground: "#F7F9FC"},
	})
	if err != nil {
		t.Fatal(err)
	}
	srv.wait(t)
	msg, err := stdmail.ReadMessage(strings.NewReader(srv.data))
	if err != nil {
		t.Fatal(err)
	}
	mediaType, params, _ := mime.ParseMediaType(msg.Header.Get("Content-Type"))
	if mediaType != "multipart/mixed" {
		t.Fatalf("Content-Type = %s", mediaType)
	}
	mixed := multipart.NewReader(msg.Body, params["boundary"])
	related, err := mixed.NextPart()
	if err != nil {
		t.Fatal(err)
	}
	mediaType, params, _ = mime.ParseMediaType(related.Header.Get("Content-Type"))
	if mediaType != "multipart/related" {
		t.Fatalf("first part = %s, want multipart/related", mediaType)
	}
	parts := multipart.NewReader(related, params["boundary"])
	alternative, _ := parts.NextPart()
	mediaType, params, _ = mime.ParseMediaType(alternative.Header.Get("Content-Type"))
	if mediaType != "multipart/alternative" {
		t.Fatalf("body = %s, want multipart/alternative", mediaType)
	}
	alt := multipart.NewReader(alternative, params["boundary"])
	text, _ := alt.NextPart()
	plain, _ := io.ReadAll(quotedprintable.NewReader(text))
	html, _ := alt.NextPart()
	rich, _ := io.ReadAll(quotedprintable.NewReader(html))
	if !strings.Contains(string(plain), "See attached.") || !strings.HasPrefix(html.Header.Get("Content-Type"), "text/html") || !strings.Contains(string(rich), "See attached.") {
		t.Errorf("text %q, html %q", plain, rich)
	}
	image, err := parts.NextPart()
	if err != nil {
		t.Fatal(err)
	}
	data, _ := io.ReadAll(base64.NewDecoder(base64.StdEncoding, image))
	cid := strings.Trim(image.Header.Get("Content-ID"), "<>")
	if string(data) != string(gif) || !strings.Contains(string(rich), "cid:"+cid) || !strings.HasPrefix(image.Header.Get("Content-Disposition"), "inline") {
		t.Errorf("image %q, cid %s", data, cid)
	}
	file, err := mixed.NextPart()
	if err != nil || file.FileName() != "invoice.pdf" {
		t.Errorf("attachment = %v, %v", file, err)
	}
}
