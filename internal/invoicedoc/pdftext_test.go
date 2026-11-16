package invoicedoc

import (
	"bytes"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"unicode/utf16"

	"github.com/adirkuhn/moneyharpy/internal/sales"
)

// placed is a piece of text or an image as a page shows it, in millimetres
// from the page's top left corner. For text, Y is the baseline.
type placed struct {
	Page       int
	X, Y, W, H float64
	Size       float64 // the font size in points
	Bold       bool
	Text       string
	// Color is the text's, or the rule's, as fpdf writes it ("r g b").
	Color string
}

// rendered is what a rendered PDF shows on its pages.
type rendered struct {
	Pages  int
	Texts  []placed
	Images []placed
	// Rules are the lines drawn, with their width in H.
	Rules []placed
}

// find is the pieces of text that read exactly s.
func (r rendered) find(s string) []placed {
	var out []placed
	for _, t := range r.Texts {
		if t.Text == s {
			out = append(out, t)
		}
	}
	return out
}

// one is the only piece of text that reads s.
func (r rendered) one(t *testing.T, s string) placed {
	t.Helper()
	found := r.find(s)
	if len(found) != 1 {
		t.Fatalf("%q printed %d times, want once; texts: %v", s, len(found), r.texts())
	}
	return found[0]
}

// containing is the pieces of text that contain s.
func (r rendered) containing(s string) []placed {
	var out []placed
	for _, t := range r.Texts {
		if strings.Contains(t.Text, s) {
			out = append(out, t)
		}
	}
	return out
}

func (r rendered) texts() []string {
	var out []string
	for _, t := range r.Texts {
		out = append(out, fmt.Sprintf("p%d@%.0f,%.0f:%s", t.Page, t.X, t.Y, t.Text))
	}
	return out
}

var (
	contentsPattern = regexp.MustCompile(`/Contents (\d+) 0 R>>`)
	fontsPattern    = regexp.MustCompile(`/F(\w+) (\d+) 0 R`)
	baseFontPattern = regexp.MustCompile(`(\d+) 0 obj\n<</Type /Font\n/Subtype /Type0\n/BaseFont /([\w-]+)`)
	opPattern       = regexp.MustCompile(`BT /F(\w+) ([\d.]+) Tf ET|BT ([\d.-]+) ([\d.-]+) Td \(((?:\\.|[^\\)])*)\) ?Tj ET|q ([\d.]+) 0 0 ([\d.]+) ([\d.-]+) ([\d.-]+) cm /I|([\d.]+) w|([\d.-]+) ([\d.-]+) m ([\d.-]+) ([\d.-]+) l S|([\d.]+ [\d.]+ [\d.]+) (rg|RG)`)
)

// layoutOf renders the document uncompressed and reads back where its text,
// images and rules are.
func layoutOf(t *testing.T, doc sales.Document, prices bool) rendered {
	t.Helper()
	pdf := build(doc, prices)
	pdf.SetCompression(false)
	var buf bytes.Buffer
	if err := pdf.Output(&buf); err != nil {
		t.Fatal(err)
	}
	data := buf.Bytes()
	_, pageH := pdf.GetPageSize()
	const k = 72 / 25.4
	bold := map[string]bool{}
	baseFonts := map[string]string{}
	for _, m := range baseFontPattern.FindAllSubmatch(data, -1) {
		baseFonts[string(m[1])] = string(m[2])
	}
	for _, m := range fontsPattern.FindAllSubmatch(data, -1) {
		// fpdf names a font after its family and style: utf8<family>B is bold.
		bold[string(m[1])] = strings.HasSuffix(baseFonts[string(m[2])], "B")
	}
	var r rendered
	for page, m := range contentsPattern.FindAllSubmatch(data, -1) {
		r.Pages++
		stream := streamOf(t, data, string(m[1]))
		size, isBold, lineWidth, textColor, drawColor := 0.0, false, 0.0, "", ""
		for _, op := range opPattern.FindAllSubmatch(stream, -1) {
			f := func(i int) float64 { v, _ := strconv.ParseFloat(string(op[i]), 64); return v }
			switch {
			case op[1] != nil:
				size, isBold = f(2), bold[string(op[1])]
			case op[3] != nil:
				r.Texts = append(r.Texts, placed{Page: page + 1, X: f(3) / k, Y: pageH - f(4)/k, Size: size, Bold: isBold, Text: pdfString(op[5]), Color: textColor})
			case op[6] != nil:
				w, h := f(6)/k, f(7)/k
				r.Images = append(r.Images, placed{Page: page + 1, X: f(8) / k, Y: pageH - f(9)/k - h, W: w, H: h})
			case op[10] != nil:
				lineWidth = f(10) / k
			case op[11] != nil:
				x1, y1, x2 := f(11)/k, pageH-f(12)/k, f(13)/k
				r.Rules = append(r.Rules, placed{Page: page + 1, X: x1, Y: y1, W: x2 - x1, H: lineWidth, Color: drawColor})
			case string(op[16]) == "rg":
				textColor = string(op[15])
			case op[16] != nil:
				drawColor = string(op[15])
			}
		}
	}
	return r
}

// streamOf is the content of object n's stream.
func streamOf(t *testing.T, data []byte, n string) []byte {
	t.Helper()
	start := bytes.Index(data, []byte("\n"+n+" 0 obj\n"))
	if start < 0 {
		t.Fatalf("no object %s", n)
	}
	rest := data[start:]
	open := bytes.Index(rest, []byte("stream\n"))
	end := bytes.Index(rest, []byte("\nendstream"))
	return rest[open+len("stream\n") : end]
}

// pdfString decodes a PDF string fpdf writes for an embedded TrueType font:
// UTF-16BE, as its fonts map character codes to Unicode.
func pdfString(b []byte) string {
	var raw []byte
	for i := 0; i < len(b); i++ {
		c := b[i]
		if c == '\\' && i+1 < len(b) {
			i++
			c = map[byte]byte{'n': '\n', 'r': '\r', 't': '\t', 'b': '\b', 'f': '\f'}[b[i]]
			if c == 0 {
				c = b[i]
			}
		}
		raw = append(raw, c)
	}
	units := make([]uint16, len(raw)/2)
	for i := range units {
		units[i] = uint16(raw[2*i])<<8 | uint16(raw[2*i+1])
	}
	return string(utf16.Decode(units))
}
