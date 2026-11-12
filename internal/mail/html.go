package mail

import (
	"bytes"
	"crypto/rand"
	"fmt"
	"html/template"
	"regexp"
	"strings"
)

// Look is how a document email looks, after the email settings of the
// document's style: a header, the text in a card, the document's details, a
// footer and an optional label. Images travel inline with the message.
type Look struct {
	// HeaderType is background_image_with_text, background_image_with_logo,
	// background_image, background_color_with_logo or
	// background_color_with_text; empty leaves the header out.
	HeaderType       string
	HeaderAlign      string
	HeaderBackground string
	HeaderText       string
	HeaderTextColor  string
	// LogoWidth is the logo's width in percent of the header.
	LogoWidth int
	// FooterType is contact_information, signature or background_image;
	// empty leaves the footer out.
	FooterType       string
	FooterTextColor  string
	FooterBackground string
	ProfileTitle     string
	ProfileSubtitle  string
	// PageBackground is the colour around the card.
	PageBackground string
	// FontStack is a CSS font-family list.
	FontStack string
	// Label is a line under the card, such as "Sent with MoneyHarpy".
	Label                                          string
	Logo, HeaderImage, ProfilePicture, FooterImage *Attachment
	// ContactLines are the sender's details for a contact information footer.
	ContactLines []string
	// Details are the document's facts listed under the text, as label and value.
	Details [][2]string
}

// inlineImage is an image the HTML shows through its Content-ID.
type inlineImage struct {
	id string
	Attachment
}

var (
	colour    = regexp.MustCompile(`^#[0-9A-Fa-f]{3,6}$`)
	fontStack = regexp.MustCompile(`^[A-Za-z0-9 ,'-]+$`)
)

func safeColour(c, fallback string) string {
	if colour.MatchString(c) {
		return c
	}
	return fallback
}

func safeAlign(a string) string {
	if a == "center" || a == "right" {
		return a
	}
	return "left"
}

var page = template.Must(template.New("email").Parse(`<!DOCTYPE html>
<html><head><meta charset="utf-8"><meta name="viewport" content="width=device-width, initial-scale=1"></head>
<body style="{{.BodyStyle}}">
<table role="presentation" width="100%" cellpadding="0" cellspacing="0" border="0" style="{{.BodyStyle}}">
<tr><td align="center" style="padding:32px 12px">
<table role="presentation" width="600" cellpadding="0" cellspacing="0" border="0" style="{{.CardStyle}}">
{{- with .Header}}
<tr><td align="{{.Align}}" style="{{.Style}}"{{if .Background}} background="cid:{{.Background}}"{{end}}>
{{- if .Image}}<img src="cid:{{.Image}}" alt="" width="600" style="display:block;width:100%;height:auto;border:0">{{end}}
{{- if .Logo}}<img src="cid:{{.Logo}}" alt="" style="{{.LogoStyle}}">{{end}}
{{- range .Text}}<p style="{{$.HeaderTextStyle}}">{{.}}</p>{{end}}
</td></tr>
{{- end}}
<tr><td style="padding:32px">
{{- range .Paragraphs}}<p style="margin:0 0 16px">{{range $i, $l := .}}{{if $i}}<br>{{end}}{{$l}}{{end}}</p>{{end}}
{{- if .Details}}
<table role="presentation" cellpadding="0" cellspacing="0" border="0" style="margin-top:16px;width:100%;border-collapse:collapse">
{{- range .Details}}<tr><th align="left" style="padding:4px 16px 4px 0;font-weight:bold;vertical-align:top">{{index . 0}}</th><td style="padding:4px 0">{{index . 1}}</td></tr>{{end}}
</table>
{{- end}}
</td></tr>
{{- with .Footer}}
<tr><td align="{{.Align}}" style="{{.Style}}">
{{- if .Image}}<img src="cid:{{.Image}}" alt="" width="600" style="display:block;width:100%;height:auto;border:0">{{end}}
{{- if .Picture}}<img src="cid:{{.Picture}}" alt="" width="80" height="80" style="display:block;margin:0 auto 12px;width:80px;height:80px;border-radius:40px">{{end}}
{{- if .Title}}<p style="margin:0;font-weight:bold">{{.Title}}</p>{{end}}
{{- if .Subtitle}}<p style="margin:0">{{.Subtitle}}</p>{{end}}
{{- range .Lines}}<p style="margin:0">{{.}}</p>{{end}}
</td></tr>
{{- end}}
</table>
{{- if .Label}}<p style="margin:16px 0 0;font-size:12px;color:#8492A6">{{.Label}}</p>{{end}}
</td></tr></table>
</body></html>
`))

type headerView struct {
	Align, Background, Image, Logo string
	Style, LogoStyle               template.CSS
	Text                           []string
}

type footerView struct {
	Align, Image, Picture, Title, Subtitle string
	Style                                  template.CSS
	Lines                                  []string
}

// renderHTML builds the HTML body of a document email from its text and
// look, with the images it shows inline.
func renderHTML(text string, l Look) (string, []inlineImage) {
	var inline []inlineImage
	cid := func(a *Attachment) string {
		if a == nil || len(a.Data) == 0 {
			return ""
		}
		id := rand.Text() + "@moneyharpy"
		inline = append(inline, inlineImage{id: id, Attachment: *a})
		return id
	}
	font := "'Lucida Grande', 'Lucida Sans Unicode', Arial, sans-serif"
	if fontStack.MatchString(l.FontStack) {
		font = l.FontStack
	}
	pageColour := safeColour(l.PageBackground, "#F7F9FC")
	view := struct {
		BodyStyle, CardStyle, HeaderTextStyle template.CSS
		Header                                *headerView
		Footer                                *footerView
		Paragraphs                            [][]string
		Details                               [][2]string
		Label                                 string
	}{
		BodyStyle:       template.CSS("margin:0;padding:0;background-color:" + pageColour),
		CardStyle:       template.CSS("width:100%;max-width:600px;background-color:#FFFFFF;border-radius:8px;overflow:hidden;color:#1F2937;font-size:14px;line-height:1.5;font-family:" + font),
		HeaderTextStyle: template.CSS("margin:0;font-size:20px;line-height:1.4;color:" + safeColour(l.HeaderTextColor, "#465569")),
		Details:         l.Details,
		Label:           l.Label,
	}
	for p := range strings.SplitSeq(strings.ReplaceAll(strings.TrimSpace(text), "\r\n", "\n"), "\n\n") {
		if p = strings.TrimSpace(p); p != "" {
			view.Paragraphs = append(view.Paragraphs, strings.Split(p, "\n"))
		}
	}

	align := safeAlign(l.HeaderAlign)
	h := &headerView{Align: align}
	style := "padding:24px 32px;text-align:" + align
	withImage := strings.HasPrefix(l.HeaderType, "background_image")
	switch l.HeaderType {
	case "background_image":
		h.Image, style = cid(l.HeaderImage), "padding:0"
	case "background_image_with_text", "background_image_with_logo", "background_color_with_logo", "background_color_with_text":
		if withImage {
			h.Background = cid(l.HeaderImage)
			style += ";height:128px;background-size:cover;background-position:center"
			if h.Background != "" {
				style += ";background-image:url(cid:" + h.Background + ")"
			}
		}
		style += ";background-color:" + safeColour(l.HeaderBackground, "#F7F9FC")
		if strings.HasSuffix(l.HeaderType, "_with_logo") {
			h.Logo = cid(l.Logo)
			width := l.LogoWidth
			if width < 10 || width > 100 {
				width = 50
			}
			h.LogoStyle = template.CSS(fmt.Sprintf("display:inline-block;width:%d%%;max-width:100%%;height:auto;border:0", width))
		} else {
			for t := range strings.SplitSeq(strings.TrimSpace(l.HeaderText), "\n") {
				h.Text = append(h.Text, t)
			}
		}
	default:
		h = nil
	}
	if h != nil {
		h.Style = template.CSS(style)
		view.Header = h
	}

	f := &footerView{Align: "center"}
	fstyle := "padding:24px 32px;font-size:13px;color:" + safeColour(l.FooterTextColor, "#465569") + ";background-color:" + safeColour(l.FooterBackground, "#F7FAFC")
	switch l.FooterType {
	case "contact_information":
		f.Lines = l.ContactLines
	case "signature":
		f.Picture, f.Title, f.Subtitle = cid(l.ProfilePicture), l.ProfileTitle, l.ProfileSubtitle
	case "background_image":
		f.Image, fstyle = cid(l.FooterImage), "padding:0"
	default:
		f = nil
	}
	if f != nil {
		f.Style = template.CSS(fstyle)
		view.Footer = f
	}

	var out bytes.Buffer
	if err := page.Execute(&out, view); err != nil {
		// The template and its data are fixed in shape, so this does not
		// happen; the email then goes as plain text.
		return "", nil
	}
	return out.String(), inline
}
