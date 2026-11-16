package invoicedoc

import (
	"bytes"
	"math"
	"regexp"
	"slices"
	"sort"
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

// widthOf is how wide a piece of text prints in a face.
func widthOf(f face, p placed) float64 {
	pdf := fpdf.New("P", "mm", "A4", "")
	style := ""
	if p.Bold {
		style = "B"
	}
	f.set(pdf, style, p.Size)
	return pdf.GetStringWidth(p.Text)
}

func TestPDFTextFitsInEveryFont(t *testing.T) {
	for _, family := range invoicing.DocumentFontFamilies {
		f := faceFor(family)
		for _, prices := range []bool{true, false} {
			doc := example(t, "nl")
			doc.Style.FontFamily = family
			doc.Invoice.Lines[1].Amount = "12,5 uur"
			r := layoutOf(t, doc, prices)
			for _, p := range r.Texts {
				if end := p.X + widthOf(f, p); end > right+0.05 {
					t.Errorf("%s: %q ends at %.1f mm, past the right margin at %.0f", family, p.Text, end, right)
				}
			}
			// The table's columns, from its header to its last line, keep
			// at least 2 mm between them.
			header := r.one(t, "Omschrijving")
			bottom := r.one(t, "12,5 uur").Y
			rows := map[float64][]placed{}
			for _, p := range r.Texts {
				if p.Page == 1 && p.Y >= header.Y-0.01 && p.Y <= bottom+0.01 {
					y := math.Round(p.Y*100) / 100
					rows[y] = append(rows[y], p)
				}
			}
			if len(rows) < 3 {
				t.Errorf("%s: table rows = %v", family, rows)
			}
			for _, row := range rows {
				sort.Slice(row, func(i, j int) bool { return row[i].X < row[j].X })
				for i := 1; i < len(row); i++ {
					if gap := row[i].X - (row[i-1].X + widthOf(f, row[i-1])); gap < 2 {
						t.Errorf("%s: %q and %q %.1f mm apart, want at least 2", family, row[i-1].Text, row[i].Text, gap)
					}
				}
			}
		}
	}
}
