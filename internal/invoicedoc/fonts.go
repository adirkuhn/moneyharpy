package invoicedoc

import (
	"embed"
	"strings"
	"sync"

	"github.com/go-pdf/fpdf"

	"github.com/adirkuhn/moneyharpy/internal/invoicing"
)

// fontFiles are openly licensed fonts standing in for the document style's
// font families, metric-compatible where one exists; the README's
// third-party notices list their versions, sources and checksums. fpdf
// embeds the glyphs a document uses, so every viewer draws the same letters.
//
//go:embed fonts/*/*.ttf
var fontFiles embed.FS

// face is the typeface a document prints in: a regular and a bold font.
type face struct {
	// family names the fonts in the PDF.
	family string
	// regular and bold are the font files in fontFiles.
	regular, bold string
}

// faces are the faces of the document style's font families: Liberation
// Sans, Serif and Mono share the metrics of Arial (and Helvetica), Times New
// Roman and Courier New, and Gelasio those of Georgia; DejaVu Sans has the
// wide proportions of Verdana; and Source Sans 3 is a humanist sans like
// Lucida Grande, which has no free counterpart. Inter, the web app's
// typeface, prints its bold in semibold, as the web app's headings.
var faces = map[string]face{
	"lucida_grande":   {"SourceSans3", "fonts/sourcesans/SourceSans3-Regular.ttf", "fonts/sourcesans/SourceSans3-Bold.ttf"},
	"helvetica":       {"LiberationSans", "fonts/liberation/LiberationSans-Regular.ttf", "fonts/liberation/LiberationSans-Bold.ttf"},
	"arial":           {"LiberationSans", "fonts/liberation/LiberationSans-Regular.ttf", "fonts/liberation/LiberationSans-Bold.ttf"},
	"verdana":         {"DejaVuSans", "fonts/dejavu/DejaVuSans.ttf", "fonts/dejavu/DejaVuSans-Bold.ttf"},
	"georgia":         {"Gelasio", "fonts/gelasio/Gelasio-Regular.ttf", "fonts/gelasio/Gelasio-Bold.ttf"},
	"times_new_roman": {"LiberationSerif", "fonts/liberation/LiberationSerif-Regular.ttf", "fonts/liberation/LiberationSerif-Bold.ttf"},
	"courier":         {"LiberationMono", "fonts/liberation/LiberationMono-Regular.ttf", "fonts/liberation/LiberationMono-Bold.ttf"},
	"inter":           {"Inter", "fonts/inter/Inter-Regular.ttf", "fonts/inter/Inter-SemiBold.ttf"},
}

var (
	fontsMu sync.Mutex
	fontsBy = map[string][]byte{}
)

// fontData is a font file, read once per process.
func fontData(file string) ([]byte, error) {
	fontsMu.Lock()
	defer fontsMu.Unlock()
	if data, ok := fontsBy[file]; ok {
		return data, nil
	}
	data, err := fontFiles.ReadFile(file)
	if err != nil {
		return nil, err
	}
	fontsBy[file] = data
	return data, nil
}

// faceFor is the face of a document style's font family.
func faceFor(family string) face {
	if f, ok := faces[family]; ok {
		return f
	}
	return faces[invoicing.DefaultFontFamily]
}

// set selects the face in a style: "B" for bold and "U" to underline. The
// faces come in regular and bold only, so italic text prints upright.
func (f face) set(pdf *fpdf.Fpdf, style string, size float64) {
	style = strings.ToUpper(style)
	underline := strings.Contains(style, "U")
	key, file := "", f.regular
	if strings.Contains(style, "B") {
		key, file = "B", f.bold
	}
	// fpdf reads a font the first time a document uses it, and ignores it
	// after.
	data, err := fontData(file)
	if err != nil {
		pdf.SetError(err)
		return
	}
	pdf.AddUTF8FontFromBytes(f.family, key, data)
	if underline {
		key += "U"
	}
	pdf.SetFont(f.family, key, size)
}

// capHeight is the height of the face's capitals, as a fraction of the
// size.
func (f face) capHeight(pdf *fpdf.Fpdf) float64 {
	f.set(pdf, "B", 10)
	return float64(pdf.GetFontDesc("", "").CapHeight) / 1000
}
