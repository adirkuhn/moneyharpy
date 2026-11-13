package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"image"
	"image/color"
	"image/png"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"
)

// fakeRunner answers commands from a table, and records them.
type fakeRunner struct {
	mu    sync.Mutex
	calls []string
	tools map[string]func(args []string) ([]byte, error)
}

func (f *fakeRunner) Run(_ context.Context, name string, args ...string) ([]byte, error) {
	f.mu.Lock()
	f.calls = append(f.calls, name+" "+strings.Join(args, " "))
	f.mu.Unlock()
	tool, ok := f.tools[name]
	if !ok {
		return nil, errors.New(name + ": not faked")
	}
	return tool(args)
}

func (f *fakeRunner) ran(name string) bool {
	return slices.ContainsFunc(f.calls, func(c string) bool { return strings.HasPrefix(c, name+" ") })
}

const minimalPDF = "%PDF-1.4\n1 0 obj\n<<>>\nendobj\n"

func pdfinfo(pages string) func([]string) ([]byte, error) {
	return func([]string) ([]byte, error) {
		return []byte("Producer: Example\nPages:          " + pages + "\nEncrypted: no\n"), nil
	}
}

func newTestHandler(run Runner) http.Handler {
	return NewHandler(Config{Languages: "nld+eng", MaxBody: 1 << 20, MaxPages: 20, ReadTimeout: 5 * time.Second, Concurrency: 2}, run, slog.New(slog.DiscardHandler))
}

func post(t *testing.T, h http.Handler, path string, body []byte) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/octet-stream")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func decodeRead(t *testing.T, rec *httptest.ResponseRecorder) ReadResult {
	t.Helper()
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d %s", rec.Code, rec.Body)
	}
	var res ReadResult
	if err := json.Unmarshal(rec.Body.Bytes(), &res); err != nil {
		t.Fatal(err)
	}
	return res
}

func TestReadDigitalPDFUsesTextLayerAndEmbeddedXML(t *testing.T) {
	run := &fakeRunner{tools: map[string]func([]string) ([]byte, error){
		"pdfinfo": pdfinfo("2"),
		"pdftotext": func(args []string) ([]byte, error) {
			if !slices.Contains(args, "-layout") || args[len(args)-1] != "-" {
				t.Errorf("pdftotext args = %v", args)
			}
			return []byte("Example Shop B.V.   Invoice 2026-001\f Total EUR 121.00 incl. VAT NL123456789B01\f"), nil
		},
		"pdfdetach": func(args []string) ([]byte, error) {
			dir := args[slices.Index(args, "-o")+1]
			_ = os.WriteFile(filepath.Join(dir, "factur-x.xml"), []byte("<?xml version=\"1.0\"?><Invoice/>"), 0o600)
			_ = os.WriteFile(filepath.Join(dir, "terms.txt"), []byte("terms"), 0o600)
			_ = os.WriteFile(filepath.Join(dir, "invoice.bin"), []byte("\xef\xbb\xbf <?xml version=\"1.0\"?><Invoice/>"), 0o600)
			return nil, nil
		},
	}}
	res := decodeRead(t, post(t, newTestHandler(run), "/read", []byte(minimalPDF)))
	if res.Method != "text" || res.Pages != 2 || !strings.HasSuffix(res.Text, "VAT NL123456789B01") || strings.Count(res.Text, "\f") != 1 {
		t.Errorf("read = %+v", res)
	}
	if len(res.Embedded) != 2 || res.Embedded[0].Name != "factur-x.xml" || res.Embedded[1].Name != "invoice.bin" ||
		res.Embedded[0].ContentType != "application/xml" || !bytes.Contains(res.Embedded[0].Data, []byte("<Invoice/>")) {
		t.Errorf("embedded = %+v", res.Embedded)
	}
	if run.ran("tesseract") {
		t.Error("a digital PDF should not be read with OCR")
	}
}

func TestReadScannedPDFFallsBackToOCRAndCapsPages(t *testing.T) {
	run := &fakeRunner{tools: map[string]func([]string) ([]byte, error){
		"pdfinfo":   pdfinfo("30"),
		"pdftotext": func([]string) ([]byte, error) { return []byte(" \f \f"), nil },
		"pdftoppm": func(args []string) ([]byte, error) {
			if !slices.Contains(args, "300") || args[slices.Index(args, "-l")+1] != "20" {
				t.Errorf("pdftoppm args = %v", args)
			}
			prefix := args[len(args)-1]
			for _, n := range []string{"02", "01"} {
				_ = os.WriteFile(prefix+"-"+n+".png", []byte("png"), 0o600)
			}
			return nil, nil
		},
		"tesseract": func(args []string) ([]byte, error) {
			if args[1] != "-" || args[len(args)-1] != "nld+eng" {
				t.Errorf("tesseract args = %v", args)
			}
			return []byte("page " + strings.TrimSuffix(filepath.Base(args[0]), ".png") + "\n\f"), nil
		},
		"pdfdetach": func([]string) ([]byte, error) { return nil, errors.New("no attachments") },
	}}
	res := decodeRead(t, post(t, newTestHandler(run), "/read", []byte(minimalPDF)))
	if res.Method != "ocr" || res.Pages != 20 || res.Text != "page page-01\n\fpage page-02\n" || len(res.Embedded) != 0 {
		t.Errorf("read = %+v", res)
	}
}

func TestReadImageUsesTesseract(t *testing.T) {
	run := &fakeRunner{tools: map[string]func([]string) ([]byte, error){
		"tesseract": func(args []string) ([]byte, error) {
			if filepath.Ext(args[0]) != ".png" {
				t.Errorf("input = %s", args[0])
			}
			return []byte("Example Shop B.V.\nTotal 12.10\n\f"), nil
		},
	}}
	res := decodeRead(t, post(t, newTestHandler(run), "/read", pngBytes(t, 10, 10)))
	if res.Method != "ocr" || res.Pages != 1 || res.Text != "Example Shop B.V.\nTotal 12.10\n" || res.Embedded == nil {
		t.Errorf("read = %+v", res)
	}
}

func TestReadRefusesUnknownAndUnreadableFiles(t *testing.T) {
	failing := &fakeRunner{tools: map[string]func([]string) ([]byte, error){
		"pdfinfo": func([]string) ([]byte, error) { return nil, errors.New("syntax error") },
	}}
	h := newTestHandler(failing)
	if rec := post(t, h, "/read", []byte("MZ\x90\x00")); rec.Code != http.StatusUnsupportedMediaType {
		t.Errorf("exe = %d", rec.Code)
	}
	if rec := post(t, h, "/read", []byte(minimalPDF)); rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("broken pdf = %d", rec.Code)
	}
	if rec := post(t, h, "/read", bytes.Repeat([]byte("%PDF-"), 1<<20)); rec.Code != http.StatusRequestEntityTooLarge {
		t.Errorf("large = %d", rec.Code)
	}
}

func TestRenderPDFPage(t *testing.T) {
	var dir string
	run := &fakeRunner{tools: map[string]func([]string) ([]byte, error){
		"pdfinfo": pdfinfo("3"),
		"pdftoppm": func(args []string) ([]byte, error) {
			if args[slices.Index(args, "-f")+1] != "2" || args[slices.Index(args, "-l")+1] != "2" || !slices.Contains(args, "-singlefile") {
				t.Errorf("pdftoppm args = %v", args)
			}
			dir = filepath.Dir(args[len(args)-1])
			return nil, os.WriteFile(args[len(args)-1]+".png", []byte("\x89PNG page two"), 0o600)
		},
	}}
	h := newTestHandler(run)
	rec := post(t, h, "/render?page=2", []byte(minimalPDF))
	if rec.Code != http.StatusOK || rec.Header().Get("X-Page-Count") != "3" || rec.Header().Get("Content-Type") != "image/png" || rec.Body.String() != "\x89PNG page two" {
		t.Errorf("render = %d %v %q", rec.Code, rec.Header(), rec.Body)
	}
	if _, err := os.Stat(dir); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("temporary directory %s was not removed", dir)
	}
	if rec := post(t, h, "/render?page=4", []byte(minimalPDF)); rec.Code != http.StatusNotFound || rec.Header().Get("X-Page-Count") != "3" {
		t.Errorf("page 4 = %d %v", rec.Code, rec.Header())
	}
	if rec := post(t, h, "/render?page=zero", []byte(minimalPDF)); rec.Code != http.StatusBadRequest {
		t.Errorf("bad page = %d", rec.Code)
	}
}

func TestRenderImageScalesDownLargeImages(t *testing.T) {
	h := newTestHandler(&fakeRunner{})
	rec := post(t, h, "/render", pngBytes(t, 3200, 800))
	if rec.Code != http.StatusOK || rec.Header().Get("X-Page-Count") != "1" {
		t.Fatalf("render = %d %s", rec.Code, rec.Body)
	}
	img, err := png.Decode(rec.Body)
	if err != nil {
		t.Fatal(err)
	}
	if b := img.Bounds(); b.Dx() != 1600 || b.Dy() != 400 {
		t.Errorf("size = %v", b)
	}
	if rec := post(t, h, "/render?page=2", pngBytes(t, 10, 10)); rec.Code != http.StatusNotFound {
		t.Errorf("page 2 of an image = %d", rec.Code)
	}
	if rec := post(t, h, "/render", []byte("\x89PNG\r\n\x1a\nbroken")); rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("broken image = %d", rec.Code)
	}
}

func TestHealthz(t *testing.T) {
	rec := httptest.NewRecorder()
	newTestHandler(&fakeRunner{}).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	if rec.Code != http.StatusOK {
		t.Errorf("healthz = %d", rec.Code)
	}
}

func TestSniff(t *testing.T) {
	for data, want := range map[string]string{
		"%PDF-1.7": kindPDF, "\xff\xd8\xff\xe0": kindJPEG, "GIF89a": kindGIF, "II*\x00": kindTIFF, "MM\x00*": kindTIFF,
		"RIFF\x00\x00\x00\x00WEBPVP8 ": kindWebP, "<html>": "",
	} {
		if got := sniff([]byte(data)); got != want {
			t.Errorf("sniff(%q) = %q, want %q", data, got, want)
		}
	}
}

func pngBytes(t *testing.T, w, h int) []byte {
	t.Helper()
	img := image.NewGray(image.Rect(0, 0, w, h))
	for x := range w {
		img.SetGray(x, h/2, color.Gray{Y: 255})
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}
