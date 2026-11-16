package invoicedoc

import (
	"bytes"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/go-pdf/fpdf"

	"github.com/adirkuhn/moneyharpy/internal/invoicing"
)

func TestEveryFontFamilyHasAnEmbeddedFace(t *testing.T) {
	for _, tc := range []struct{ family, face string }{
		{"lucida_grande", "SourceSans3"},
		{"helvetica", "LiberationSans"},
		{"arial", "LiberationSans"},
		{"verdana", "DejaVuSans"},
		{"georgia", "Gelasio"},
		{"times_new_roman", "LiberationSerif"},
		{"courier", "LiberationMono"},
		{"inter", "Inter"},
	} {
		if !slices.Contains(invoicing.DocumentFontFamilies, tc.family) {
			t.Errorf("%s is not a document style font family", tc.family)
		}
		f := faceFor(tc.family)
		if f.family != tc.face {
			t.Errorf("%s prints in %s, want %s", tc.family, f.family, tc.face)
		}
		pdf := fpdf.New("P", "mm", "A4", "")
		pdf.AddPage()
		f.set(pdf, "", 10)
		f.set(pdf, "B", 10)
		f.set(pdf, "BIU", 10)
		if err := pdf.Error(); err != nil {
			t.Errorf("%s: %v", tc.family, err)
		}
	}
	if len(invoicing.DocumentFontFamilies) != 8 {
		t.Errorf("font families = %v, want a face for each", invoicing.DocumentFontFamilies)
	}
	if faceFor("").family != faceFor(invoicing.DefaultFontFamily).family {
		t.Error("a style without a font family does not print in the default one")
	}
}

var (
	type0Pattern     = regexp.MustCompile(`<</Type /Font\n/Subtype /Type0\n/BaseFont /(\w+)\n/Encoding /Identity-H\n/DescendantFonts \[\d+ 0 R\]\n/ToUnicode (\d+) 0 R>>`)
	fontFile2Pattern = regexp.MustCompile(`/FontFile2 \d+ 0 R`)
)

func TestPDFEmbedsTheStylesFont(t *testing.T) {
	for _, family := range invoicing.DocumentFontFamilies {
		doc := example(t, "nl")
		doc.Style.FontFamily = family
		pdf := build(doc, true)
		pdf.SetCompression(false)
		var buf bytes.Buffer
		if err := pdf.Output(&buf); err != nil {
			t.Fatal(err)
		}
		data := buf.Bytes()
		want := "utf8" + strings.ToLower(faceFor(family).family)
		fonts := type0Pattern.FindAllSubmatch(data, -1)
		var names []string
		for _, m := range fonts {
			names = append(names, string(m[1]))
			if !strings.HasPrefix(string(m[1]), want) {
				t.Errorf("%s: font %s, want only %s", family, m[1], want)
			}
			if !bytes.Contains(streamOf(t, data, string(m[2])), []byte("<0000> <FFFF> <0000>")) {
				t.Errorf("%s: %s does not map its codes to Unicode", family, m[1])
			}
		}
		if !slices.Contains(names, want) || !slices.Contains(names, want+"B") {
			t.Errorf("%s: fonts %v, want %s and its bold", family, names, want)
		}
		if n := len(fontFile2Pattern.FindAll(data, -1)); n != len(fonts) {
			t.Errorf("%s: %d embedded font files for %d fonts", family, n, len(fonts))
		}
		if bytes.Contains(data, []byte("/Subtype /Type1")) {
			t.Errorf("%s: a core font, which viewers substitute", family)
		}
	}
}

func TestPDFTextRoundTrips(t *testing.T) {
	for _, family := range []string{"helvetica", "lucida_grande", "georgia", "times_new_roman", "courier", "inter"} {
		doc := example(t, "nl")
		doc.Style.FontFamily = family
		doc.Invoice.Lines[1].Description = "Café, façade, ação, Müller • € 5"
		r := layoutOf(t, doc, true)
		r.one(t, "Café, façade, ação, Müller • € 5")
		r.one(t, "€ 296,50")
		r.one(t, "Factuurdatum:")
	}
}
