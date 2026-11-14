package main

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"
)

// fakeRapidOCR is a RapidOCR service answering every image with handle,
// counting the calls.
type fakeRapidOCR struct {
	mu     sync.Mutex
	bodies [][]byte
	handle func(w http.ResponseWriter, r *http.Request)
}

func (f *fakeRapidOCR) start(t *testing.T) string {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/read" {
			t.Errorf("rapidocr request = %s %s", r.Method, r.URL)
		}
		body, _ := io.ReadAll(r.Body)
		f.mu.Lock()
		f.bodies = append(f.bodies, body)
		f.mu.Unlock()
		f.handle(w, r)
	}))
	t.Cleanup(srv.Close)
	return srv.URL
}

func (f *fakeRapidOCR) calls() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.bodies)
}

func answer(text string) func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"text":  text,
			"lines": []map[string]any{{"text": text, "box": []int{0, 0, 10, 10}, "score": 0.9}},
		})
	}
}

// rapidHandler is the service with RapidOCR at url, logging into logs.
func rapidHandler(run Runner, url string, timeout time.Duration, logs *bytes.Buffer) http.Handler {
	logger := slog.New(slog.NewTextHandler(logs, nil))
	return NewHandler(Config{
		Languages: "nld+eng", MaxBody: 1 << 20, MaxPages: 20, ReadTimeout: 5 * time.Second, Concurrency: 2,
		RapidOCRURL: url, RapidOCRTimeout: timeout,
	}, run, logger)
}

// tesseractReads answers every page mode with the text of readings, or
// noise.
func tesseractReads(readings map[string]string) *fakeRunner {
	return &fakeRunner{tools: map[string]func([]string) ([]byte, error){
		"tesseract": func(args []string) ([]byte, error) {
			if text, ok := readings[psmArg(args)]; ok {
				return []byte(text), nil
			}
			return []byte("“Xampl ~~ |"), nil
		},
	}}
}

func methods(readings []Reading) []string {
	var out []string
	for _, r := range readings {
		out = append(out, r.Method)
	}
	slices.Sort(out)
	return out
}

const rapidReceipt = "Example Shop B.V.\nDatum 20-10-2026 14:32\nCrème brûlée 7,50\nTOTAAL 24,10\nBTW 9% over 22,11 1,99"

func TestReadPhotoJoinsRapidOCRToTheCandidates(t *testing.T) {
	rapid := &fakeRapidOCR{handle: answer(rapidReceipt)}
	var logs bytes.Buffer
	img := pngBytes(t, 10, 10)
	h := rapidHandler(tesseractReads(map[string]string{"6": "TOTAAL 24.10"}), rapid.start(t), time.Minute, &logs)
	res := decodeRead(t, post(t, h, "/read", img))
	if res.Method != "ocr" || res.Text != rapidReceipt || res.TextMethod != "rapidocr" {
		t.Errorf("read = %q by %q (%s), want RapidOCR's reading", res.Text, res.TextMethod, res.Method)
	}
	if got := methods(res.Alternatives); !slices.Equal(got, []string{"tesseract-psm3", "tesseract-psm4", "tesseract-psm6"}) {
		t.Errorf("alternatives by %v", got)
	}
	if res.Alternatives[0].Method != "tesseract-psm6" || res.Alternatives[0].PageMode != "6" {
		t.Errorf("best alternative = %+v, want Tesseract's page mode 6", res.Alternatives[0])
	}
	if rapid.calls() != 1 || !bytes.Equal(rapid.bodies[0], img) {
		t.Errorf("rapidocr got %d calls", rapid.calls())
	}
}

func TestReadPhotoKeepsTesseractWhenItReadsBetter(t *testing.T) {
	rapid := &fakeRapidOCR{handle: answer("“Xampl ~~")}
	var logs bytes.Buffer
	h := rapidHandler(tesseractReads(map[string]string{"6": "TOTAAL 24,10\nDatum 20-10-2026"}), rapid.start(t), time.Minute, &logs)
	res := decodeRead(t, post(t, h, "/read", pngBytes(t, 10, 10)))
	if res.Text != "TOTAAL 24,10\nDatum 20-10-2026" || res.TextMethod != "tesseract-psm6" {
		t.Errorf("read = %q by %q", res.Text, res.TextMethod)
	}
	if !slices.Contains(methods(res.Alternatives), "rapidocr") || len(res.Alternatives) != 3 {
		t.Errorf("alternatives = %+v", res.Alternatives)
	}
}

func TestReadFallsBackToTesseractWhenRapidOCRIsDown(t *testing.T) {
	down := httptest.NewServer(http.NotFoundHandler())
	down.Close()
	var logs bytes.Buffer
	h := rapidHandler(tesseractReads(map[string]string{"6": "TOTAAL 24,10"}), down.URL, time.Minute, &logs)
	res := decodeRead(t, post(t, h, "/read", pngBytes(t, 10, 10)))
	if res.Text != "TOTAAL 24,10" || res.TextMethod != "tesseract-psm6" || slices.Contains(methods(res.Alternatives), "rapidocr") {
		t.Errorf("read = %+v", res)
	}
	if !strings.Contains(logs.String(), "level=WARN") || !strings.Contains(logs.String(), "rapidocr") {
		t.Errorf("no warning logged: %s", logs.String())
	}
}

func TestReadGivesUpOnASlowRapidOCR(t *testing.T) {
	rapid := &fakeRapidOCR{handle: func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-time.After(5 * time.Second):
		}
		answer(rapidReceipt)(w, r)
	}}
	var logs bytes.Buffer
	h := rapidHandler(tesseractReads(map[string]string{"6": "TOTAAL 24,10"}), rapid.start(t), 50*time.Millisecond, &logs)
	start := time.Now()
	res := decodeRead(t, post(t, h, "/read", pngBytes(t, 10, 10)))
	if took := time.Since(start); took > 2*time.Second {
		t.Errorf("waited %s for RapidOCR", took)
	}
	if res.Text != "TOTAAL 24,10" || len(res.Alternatives) != 2 {
		t.Errorf("read = %+v", res)
	}
	if !strings.Contains(logs.String(), "level=WARN") {
		t.Errorf("no warning logged: %s", logs.String())
	}
}

func TestReadIgnoresRapidOCRErrors(t *testing.T) {
	for name, handle := range map[string]func(http.ResponseWriter, *http.Request){
		"status": func(w http.ResponseWriter, _ *http.Request) {
			http.Error(w, "internal error", http.StatusInternalServerError)
		},
		"json": func(w http.ResponseWriter, _ *http.Request) { _, _ = io.WriteString(w, "{not json") },
	} {
		t.Run(name, func(t *testing.T) {
			rapid := &fakeRapidOCR{handle: handle}
			var logs bytes.Buffer
			h := rapidHandler(tesseractReads(map[string]string{"6": "TOTAAL 24,10"}), rapid.start(t), time.Minute, &logs)
			res := decodeRead(t, post(t, h, "/read", pngBytes(t, 10, 10)))
			if res.Text != "TOTAAL 24,10" || slices.Contains(methods(res.Alternatives), "rapidocr") {
				t.Errorf("read = %+v", res)
			}
			if !strings.Contains(logs.String(), "level=WARN") {
				t.Errorf("no warning logged: %s", logs.String())
			}
		})
	}
}

func TestReadUsesRapidOCRWhenTesseractFails(t *testing.T) {
	rapid := &fakeRapidOCR{handle: answer(rapidReceipt)}
	failing := &fakeRunner{tools: map[string]func([]string) ([]byte, error){}}
	var logs bytes.Buffer
	res := decodeRead(t, post(t, rapidHandler(failing, rapid.start(t), time.Minute, &logs), "/read", pngBytes(t, 10, 10)))
	if res.Text != rapidReceipt || res.TextMethod != "rapidocr" || len(res.Alternatives) != 0 {
		t.Errorf("read = %+v", res)
	}
}

// scan fakes the tools for a scanned PDF of pages pages.
func scan(pages string, files ...string) map[string]func([]string) ([]byte, error) {
	return map[string]func([]string) ([]byte, error){
		"pdfinfo":   pdfinfo(pages),
		"pdftotext": func([]string) ([]byte, error) { return []byte(" \f"), nil },
		"pdftoppm": func(args []string) ([]byte, error) {
			for _, f := range files {
				_ = os.WriteFile(args[len(args)-1]+"-"+f+".png", []byte("png "+f), 0o600)
			}
			return nil, nil
		},
		"tesseract": func([]string) ([]byte, error) { return []byte("Tota@l l2,1O"), nil },
		"pdfdetach": func([]string) ([]byte, error) { return nil, nil },
	}
}

func TestReadShortScanAsksRapidOCRForEachPage(t *testing.T) {
	rapid := &fakeRapidOCR{}
	rapid.handle = func(w http.ResponseWriter, r *http.Request) {
		rapid.mu.Lock()
		page := string(rapid.bodies[len(rapid.bodies)-1])
		rapid.mu.Unlock()
		answer("Totaal 12,10 Datum 20-10-2026 "+page)(w, r)
	}
	var logs bytes.Buffer
	h := rapidHandler(&fakeRunner{tools: scan("2", "1", "2")}, rapid.start(t), time.Minute, &logs)
	res := decodeRead(t, post(t, h, "/read", []byte(minimalPDF)))
	if res.Method != "ocr" || res.TextMethod != "rapidocr" ||
		res.Text != "Totaal 12,10 Datum 20-10-2026 png 1\fTotaal 12,10 Datum 20-10-2026 png 2" {
		t.Errorf("read = %q by %q", res.Text, res.TextMethod)
	}
}

func TestReadLongScanDoesNotAskRapidOCR(t *testing.T) {
	rapid := &fakeRapidOCR{handle: answer(rapidReceipt)}
	var logs bytes.Buffer
	h := rapidHandler(&fakeRunner{tools: scan("3", "1", "2", "3")}, rapid.start(t), time.Minute, &logs)
	res := decodeRead(t, post(t, h, "/read", []byte(minimalPDF)))
	if rapid.calls() != 0 || res.TextMethod != "tesseract-psm3" {
		t.Errorf("rapidocr calls = %d, read by %q", rapid.calls(), res.TextMethod)
	}
}

func TestDigitalPDFDoesNotAskRapidOCR(t *testing.T) {
	rapid := &fakeRapidOCR{handle: answer(rapidReceipt)}
	run := &fakeRunner{tools: map[string]func([]string) ([]byte, error){
		"pdfinfo": pdfinfo("1"),
		"pdftotext": func([]string) ([]byte, error) {
			return []byte("Example Shop B.V. Invoice 2026-001 Total EUR 121.00"), nil
		},
		"pdfdetach": func([]string) ([]byte, error) { return nil, nil },
	}}
	var logs bytes.Buffer
	res := decodeRead(t, post(t, rapidHandler(run, rapid.start(t), time.Minute, &logs), "/read", []byte(minimalPDF)))
	if rapid.calls() != 0 || res.Method != "text" || res.TextMethod != "" {
		t.Errorf("rapidocr calls = %d, read = %+v", rapid.calls(), res)
	}
}
