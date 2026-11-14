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
			if args[1] != "-" || args[len(args)-1] != "nld+eng" || psmArg(args) != "3" {
				t.Errorf("tesseract args = %v: a long scan is read with the first page mode only", args)
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

// psmArg returns the page mode a tesseract command was run with.
func psmArg(args []string) string {
	if i := slices.Index(args, "--psm"); i >= 0 && i+1 < len(args) {
		return args[i+1]
	}
	return ""
}

func TestReadPhotoWithEveryPageModeAndReturnsTheBestReading(t *testing.T) {
	// What the page modes make of one till receipt photo.
	readings := map[string]string{
		"3": "EXAMPLE SHOP\nOTAAL [4] 36,3%\n“Xampl Shop BV\n26-10-2026 14:32\n\f",
		"6": "EXAMPLE SHOP\nTOTAAL [4] 36 . 30\n21% 30,00 6,30 36,30\nDatum 20-10-2026 14:32\nExample Shop B.V.\n\f",
		"4": "EXAMPLE SHOP\nTOTAAL [43 36,30\n21% 30,00 6,30 36,3\n\f",
	}
	run := &fakeRunner{tools: map[string]func([]string) ([]byte, error){
		"tesseract": func(args []string) ([]byte, error) {
			if len(args) != 6 || args[1] != "-" || args[2] != "--psm" || args[4] != "-l" || args[5] != "nld+eng" {
				t.Errorf("tesseract args = %v", args)
			}
			return []byte(readings[psmArg(args)]), nil
		},
	}}
	res := decodeRead(t, post(t, newTestHandler(run), "/read", pngBytes(t, 10, 10)))
	var modes []string
	for _, c := range run.calls {
		modes = append(modes, psmArg(strings.Fields(c)[1:]))
	}
	slices.Sort(modes)
	if !slices.Equal(modes, []string{"3", "4", "6"}) {
		t.Errorf("page modes run = %v", modes)
	}
	if res.Text != strings.TrimSuffix(readings["6"], "\f") {
		t.Errorf("text = %q, want the reading of page mode 6", res.Text)
	}
	if len(res.Alternatives) != 2 || res.Alternatives[0].PageMode != "4" || res.Alternatives[1].PageMode != "3" ||
		res.Alternatives[1].Text != strings.TrimSuffix(readings["3"], "\f") {
		t.Errorf("alternatives = %+v", res.Alternatives)
	}
}

func TestReadPhotoLeavesOutPageModesThatFail(t *testing.T) {
	run := &fakeRunner{tools: map[string]func([]string) ([]byte, error){
		"tesseract": func(args []string) ([]byte, error) {
			if psmArg(args) != "4" {
				return nil, errors.New("tesseract crashed")
			}
			return []byte("Total 12,10"), nil
		},
	}}
	res := decodeRead(t, post(t, newTestHandler(run), "/read", pngBytes(t, 10, 10)))
	if res.Text != "Total 12,10" || len(res.Alternatives) != 0 {
		t.Errorf("read = %+v", res)
	}
	failing := &fakeRunner{tools: map[string]func([]string) ([]byte, error){
		"tesseract": func([]string) ([]byte, error) { return nil, errors.New("tesseract crashed") },
	}}
	if rec := post(t, newTestHandler(failing), "/read", pngBytes(t, 10, 10)); rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("all modes failing = %d", rec.Code)
	}
}

func TestReadRunsAtMostWorkersTesseractsAtOnce(t *testing.T) {
	var mu sync.Mutex
	running, most := 0, 0
	run := &fakeRunner{tools: map[string]func([]string) ([]byte, error){
		"tesseract": func([]string) ([]byte, error) {
			mu.Lock()
			running++
			most = max(most, running)
			mu.Unlock()
			time.Sleep(20 * time.Millisecond)
			mu.Lock()
			running--
			mu.Unlock()
			return []byte("Total 12,10"), nil
		},
	}}
	h := NewHandler(Config{Languages: "nld+eng", MaxBody: 1 << 20, MaxPages: 20, ReadTimeout: 5 * time.Second, Concurrency: 4, Workers: 2}, run, slog.New(slog.DiscardHandler))
	var wg sync.WaitGroup
	for range 3 {
		wg.Go(func() { decodeRead(t, post(t, h, "/read", pngBytes(t, 10, 10))) })
	}
	wg.Wait()
	if most != 2 || len(run.calls) != 9 {
		t.Errorf("at most %d tesseracts at once over %d runs, want 2 over 9", most, len(run.calls))
	}
}

func TestReadingScore(t *testing.T) {
	good := readingScore("TOTAAL 36.30\n21% 30,00 6,30 36,30\nDatum 20-10-2026")
	bad := readingScore("OTAAL [4] 36,3%\n“Xampl Shop BV\n2t% 30,0 6,3o 36,3")
	if good <= bad {
		t.Errorf("score of a good reading %d <= of a bad one %d", good, bad)
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

func TestReadShortScanWithEveryPageMode(t *testing.T) {
	run := &fakeRunner{tools: map[string]func([]string) ([]byte, error){
		"pdfinfo":   pdfinfo("1"),
		"pdftotext": func([]string) ([]byte, error) { return []byte(" \f"), nil },
		"pdftoppm": func(args []string) ([]byte, error) {
			return nil, os.WriteFile(args[len(args)-1]+"-1.png", []byte("png"), 0o600)
		},
		"tesseract": func(args []string) ([]byte, error) {
			if psmArg(args) == "6" {
				return []byte("Totaal 12,10 Datum 20-10-2026"), nil
			}
			return []byte("Tota@l l2,1O"), nil
		},
		"pdfdetach": func([]string) ([]byte, error) { return nil, errors.New("no attachments") },
	}}
	res := decodeRead(t, post(t, newTestHandler(run), "/read", []byte(minimalPDF)))
	if res.Method != "ocr" || res.Text != "Totaal 12,10 Datum 20-10-2026" || len(res.Alternatives) != 2 {
		t.Errorf("read = %+v", res)
	}
}

func TestRenderForReadingAtAnotherSize(t *testing.T) {
	h := newTestHandler(&fakeRunner{})
	rec := post(t, h, "/render?max_side=800", pngBytes(t, 450, 1000))
	if rec.Code != http.StatusOK {
		t.Fatalf("render = %d %s", rec.Code, rec.Body)
	}
	img, err := png.Decode(rec.Body)
	if err != nil {
		t.Fatal(err)
	}
	if b := img.Bounds(); b.Dx() != 360 || b.Dy() != 800 {
		t.Errorf("size = %v", b)
	}
	for _, bad := range []string{"0", "100", "5000", "big"} {
		if rec := post(t, h, "/render?max_side="+bad, pngBytes(t, 10, 10)); rec.Code != http.StatusBadRequest {
			t.Errorf("max_side=%s = %d", bad, rec.Code)
		}
	}
	run := &fakeRunner{tools: map[string]func([]string) ([]byte, error){
		"pdfinfo": pdfinfo("1"),
		"pdftoppm": func(args []string) ([]byte, error) {
			if args[slices.Index(args, "-scale-to")+1] != "2048" || slices.Contains(args, "-r") {
				t.Errorf("pdftoppm args = %v", args)
			}
			return nil, os.WriteFile(args[len(args)-1]+".png", []byte("\x89PNG page"), 0o600)
		},
	}}
	if rec := post(t, newTestHandler(run), "/render?max_side=2048", []byte(minimalPDF)); rec.Code != http.StatusOK {
		t.Errorf("pdf = %d %s", rec.Code, rec.Body)
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
