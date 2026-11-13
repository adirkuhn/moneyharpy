package main

import (
	"bytes"
	"compress/zlib"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"

	"golang.org/x/image/draw"
	"golang.org/x/image/font"
	"golang.org/x/image/font/basicfont"
	"golang.org/x/image/math/fixed"
)

// TestSmoke runs against a real service with Poppler and Tesseract, such as
// the image of Dockerfile.ocr:
//
//	docker run -d --rm -p 18090:8090 --name ocr-smoke moneyharpy-ocr
//	OCRD_SMOKE_URL=http://localhost:18090 go test ./cmd/ocrd -run Smoke -v
func TestSmoke(t *testing.T) {
	base := os.Getenv("OCRD_SMOKE_URL")
	if base == "" {
		t.Skip("OCRD_SMOKE_URL is not set")
	}
	call := func(path string, body []byte) *http.Response {
		t.Helper()
		res, err := http.Post(base+path, "application/octet-stream", bytes.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = res.Body.Close() })
		return res
	}
	read := func(name string, body []byte) ReadResult {
		t.Helper()
		res := call("/read", body)
		if res.StatusCode != http.StatusOK {
			b, _ := io.ReadAll(res.Body)
			t.Fatalf("%s: read = %s %s", name, res.Status, b)
		}
		var out ReadResult
		if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
			t.Fatal(err)
		}
		t.Logf("%s: method=%s pages=%d embedded=%d text=%q", name, out.Method, out.Pages, len(out.Embedded), out.Text)
		return out
	}

	xml := `<?xml version="1.0" encoding="UTF-8"?><Invoice xmlns="urn:oasis:names:specification:ubl:schema:xsd:Invoice-2"><ID>2026-001</ID></Invoice>`
	digital := read("digital pdf", textPDF([]string{"Example Shop B.V. Invoice 2026-001", "Total EUR 121.00"}, xml))
	if digital.Method != "text" || digital.Pages != 2 || !strings.Contains(digital.Text, "Example Shop B.V.") || !strings.Contains(digital.Text, "\f") {
		t.Errorf("digital = %+v", digital)
	}
	if len(digital.Embedded) != 1 || !strings.Contains(string(digital.Embedded[0].Data), "<ID>2026-001</ID>") {
		t.Errorf("embedded = %+v", digital.Embedded)
	}

	img := textImage("INVOICE 2026-001 TOTAL 121.00")
	if got := read("png", pngEncode(t, img)); got.Method != "ocr" || !strings.Contains(got.Text, "2026-001") {
		t.Errorf("png = %+v", got)
	}
	if got := read("scanned pdf", imagePDF(img)); got.Method != "ocr" || !strings.Contains(got.Text, "2026-001") {
		t.Errorf("scan = %+v", got)
	}

	res := call("/render?page=2", textPDF([]string{"one", "two", "three"}, ""))
	page, err := png.Decode(res.Body)
	if res.StatusCode != http.StatusOK || res.Header.Get("X-Page-Count") != "3" || err != nil {
		t.Errorf("render pdf = %s %v %v", res.Status, res.Header, err)
	} else {
		t.Logf("pdf page: %v", page.Bounds())
	}
	if res := call("/render?page=4", textPDF([]string{"one"}, "")); res.StatusCode != http.StatusNotFound {
		t.Errorf("render page 4 = %s", res.Status)
	}
	res = call("/render", pngEncode(t, img))
	if _, err := png.Decode(res.Body); res.StatusCode != http.StatusOK || res.Header.Get("X-Page-Count") != "1" || err != nil {
		t.Errorf("render png = %s %v %v", res.Status, res.Header, err)
	}
}

// textImage draws text large and black on white, like a scan.
func textImage(text string) *image.Gray {
	face := basicfont.Face7x13
	small := image.NewGray(image.Rect(0, 0, 7*len(text)+20, 30))
	draw.Draw(small, small.Bounds(), image.White, image.Point{}, draw.Src)
	d := font.Drawer{Dst: small, Src: image.NewUniform(color.Black), Face: face, Dot: fixed.P(10, 20)}
	d.DrawString(text)
	big := image.NewGray(image.Rect(0, 0, small.Bounds().Dx()*4, small.Bounds().Dy()*4))
	draw.NearestNeighbor.Scale(big, big.Bounds(), small, small.Bounds(), draw.Src, nil)
	return big
}

func pngEncode(t *testing.T, img image.Image) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// pdfWriter writes numbered objects and the cross-reference table.
type pdfWriter struct {
	buf     bytes.Buffer
	offsets []int
}

func (p *pdfWriter) obj(body string) int {
	p.offsets = append(p.offsets, p.buf.Len())
	n := len(p.offsets)
	fmt.Fprintf(&p.buf, "%d 0 obj\n%s\nendobj\n", n, body)
	return n
}

func (p *pdfWriter) stream(dict string, data []byte) int {
	return p.obj(fmt.Sprintf("<<%s /Length %d>>\nstream\n%s\nendstream", dict, len(data), data))
}

func (p *pdfWriter) finish(root int) []byte {
	xref := p.buf.Len()
	fmt.Fprintf(&p.buf, "xref\n0 %d\n0000000000 65535 f \n", len(p.offsets)+1)
	for _, o := range p.offsets {
		fmt.Fprintf(&p.buf, "%010d 00000 n \n", o)
	}
	fmt.Fprintf(&p.buf, "trailer\n<</Size %d /Root %d 0 R>>\nstartxref\n%d\n%%%%EOF\n", len(p.offsets)+1, root, xref)
	return p.buf.Bytes()
}

// textPDF makes a digital PDF with a line of text per page and, optionally,
// an embedded XML invoice. Objects: 1 font, then pages; the catalog is last.
func textPDF(pages []string, xml string) []byte {
	p := &pdfWriter{}
	p.buf.WriteString("%PDF-1.7\n")
	font := p.obj("<</Type /Font /Subtype /Type1 /BaseFont /Helvetica>>")
	parent := len(pages)*2 + 2 // The pages object comes after the pages.
	var kids []string
	for _, text := range pages {
		content := p.stream("", fmt.Appendf(nil, "BT /F1 18 Tf 72 720 Td (%s) Tj ET", text))
		kids = append(kids, fmt.Sprintf("%d 0 R", p.obj(fmt.Sprintf(
			"<</Type /Page /Parent %d 0 R /MediaBox [0 0 595 842] /Resources <</Font <</F1 %d 0 R>>>> /Contents %d 0 R>>", parent, font, content))))
	}
	p.obj(fmt.Sprintf("<</Type /Pages /Kids [%s] /Count %d>>", strings.Join(kids, " "), len(pages)))
	names := ""
	if xml != "" {
		file := p.stream(" /Type /EmbeddedFile /Subtype /text#2Fxml", []byte(xml))
		spec := p.obj(fmt.Sprintf("<</Type /Filespec /F (factur-x.xml) /UF (factur-x.xml) /EF <</F %d 0 R>>>>", file))
		names = fmt.Sprintf(" /Names <</EmbeddedFiles <</Names [(factur-x.xml) %d 0 R]>>>>", spec)
	}
	return p.finish(p.obj(fmt.Sprintf("<</Type /Catalog /Pages %d 0 R%s>>", parent, names)))
}

// imagePDF makes a PDF of one page that is only an image, like a scan.
func imagePDF(img *image.Gray) []byte {
	var z bytes.Buffer
	zw := zlib.NewWriter(&z)
	b := img.Bounds()
	for y := b.Min.Y; y < b.Max.Y; y++ {
		_, _ = zw.Write(img.Pix[y*img.Stride : y*img.Stride+b.Dx()])
	}
	_ = zw.Close()
	p := &pdfWriter{}
	p.buf.WriteString("%PDF-1.7\n")
	w, h := b.Dx()*72/300, b.Dy()*72/300 // 300 dpi.
	im := p.stream(fmt.Sprintf(" /Type /XObject /Subtype /Image /Width %d /Height %d /ColorSpace /DeviceGray /BitsPerComponent 8 /Filter /FlateDecode", b.Dx(), b.Dy()), z.Bytes())
	content := p.stream("", fmt.Appendf(nil, "q %d 0 0 %d 20 700 cm /Im1 Do Q", w, h))
	page := p.obj(fmt.Sprintf("<</Type /Page /Parent 4 0 R /MediaBox [0 0 595 842] /Resources <</XObject <</Im1 %d 0 R>>>> /Contents %d 0 R>>", im, content))
	pages := p.obj(fmt.Sprintf("<</Type /Pages /Kids [%d 0 R] /Count 1>>", page))
	return p.finish(p.obj(fmt.Sprintf("<</Type /Catalog /Pages %d 0 R>>", pages)))
}
