package main

import (
	"bytes"
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	_ "image/gif" // Decoders for previews of images.
	_ "image/jpeg"
	"image/png"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"

	"golang.org/x/image/draw"
	_ "golang.org/x/image/tiff"
	_ "golang.org/x/image/webp"
)

// Config tunes the service.
type Config struct {
	// Languages are Tesseract's, such as "nld+eng".
	Languages string
	// MaxBody caps an uploaded file in bytes.
	MaxBody int64
	// MaxPages caps the pages of a PDF that are read.
	MaxPages int
	// ReadTimeout caps one request.
	ReadTimeout time.Duration
	// Concurrency caps the requests worked on at once; others wait.
	Concurrency int
	// PageModes are the Tesseract page segmentation modes (--psm) a photo
	// or a short scan is read with; the best reading is returned. The
	// first is the default, used alone for longer scans. Mode 3 finds the
	// layout itself, 4 reads one column of lines of different sizes, 6 one
	// block of text: a till receipt photographed with a phone reads best
	// with 6 or 4.
	PageModes []string
	// Workers caps the Tesseract runs at once over all requests.
	Workers int
	// RapidOCRURL is the RapidOCR service (ocr-rapid/), such as
	// http://ocr-rapid:8091, that reads photos and short scans next to
	// Tesseract; its reading is one more candidate. Empty is off.
	RapidOCRURL string
	// RapidOCRTimeout caps RapidOCR's reading of one file; when it takes
	// longer, fails or is down, Tesseract's readings are used.
	RapidOCRTimeout time.Duration
}

// DefaultRapidOCRTimeout caps RapidOCR's reading of one file.
const DefaultRapidOCRTimeout = time.Minute

// multiModePages is the most pages a scanned PDF may have to be read with
// every page mode: a receipt or a short invoice. Longer scans are read
// with the first mode only, to keep them fast.
const multiModePages = 2

// Runner runs a command and returns what it wrote to stdout.
type Runner interface {
	Run(ctx context.Context, name string, args ...string) ([]byte, error)
}

// ExecRunner runs commands on this machine.
type ExecRunner struct{}

// maxOutput caps what one command may write to stdout.
const maxOutput = 16 << 20

// Run implements Runner.
func (ExecRunner) Run(ctx context.Context, name string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	var stdout, stderr limitedBuffer
	stdout.limit, stderr.limit = maxOutput, 4<<10
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("%s: %w: %s", name, err, strings.TrimSpace(stderr.String()))
	}
	return stdout.Bytes(), nil
}

// limitedBuffer keeps the first limit bytes written to it.
type limitedBuffer struct {
	bytes.Buffer
	limit int
}

func (b *limitedBuffer) Write(p []byte) (int, error) {
	if room := b.limit - b.Len(); room > 0 {
		b.Buffer.Write(p[:min(len(p), room)])
	}
	return len(p), nil
}

// Kinds of file the service handles.
const (
	kindPDF  = "pdf"
	kindPNG  = "png"
	kindJPEG = "jpg"
	kindGIF  = "gif"
	kindTIFF = "tif"
	kindWebP = "webp"
)

// sniff tells the kind of file from its first bytes; the declared content
// type is not trusted.
func sniff(data []byte) string {
	switch {
	case bytes.HasPrefix(data, []byte("%PDF-")):
		return kindPDF
	case bytes.HasPrefix(data, []byte("\x89PNG\r\n\x1a\n")):
		return kindPNG
	case bytes.HasPrefix(data, []byte("\xff\xd8\xff")):
		return kindJPEG
	case bytes.HasPrefix(data, []byte("GIF87a")), bytes.HasPrefix(data, []byte("GIF89a")):
		return kindGIF
	case bytes.HasPrefix(data, []byte("II*\x00")), bytes.HasPrefix(data, []byte("MM\x00*")):
		return kindTIFF
	case len(data) >= 12 && string(data[:4]) == "RIFF" && string(data[8:12]) == "WEBP":
		return kindWebP
	}
	return ""
}

// File is an embedded file in the read response.
type File struct {
	Name        string `json:"name"`
	ContentType string `json:"content_type"`
	Data        []byte `json:"data"`
}

// ReadResult is the response of POST /read.
type ReadResult struct {
	Text   string `json:"text"`
	Method string `json:"method"`
	// TextMethod tells which OCR read Text, as Reading.Method; empty for
	// a PDF's text layer.
	TextMethod string `json:"text_method,omitempty"`
	Pages      int    `json:"pages"`
	Embedded   []File `json:"embedded"`
	// Alternatives are the other readings of OCR, by other page modes or
	// engines, best first, for a reader that can tell better which one it
	// understands.
	Alternatives []Reading `json:"alternatives"`
}

// Reading is the text one OCR engine read.
type Reading struct {
	// Method is the engine and its mode: "tesseract-psm6" for Tesseract
	// with page mode 6, "rapidocr" for RapidOCR.
	Method string `json:"method"`
	// PageMode is Tesseract's page mode; empty for RapidOCR.
	PageMode string `json:"page_mode,omitempty"`
	Text     string `json:"text"`
}

// errUnreadable means the file looked supported but the tools could not read
// it; errNoPage means the page asked for does not exist.
var (
	errUnreadable = errors.New("file cannot be read")
	errNoPage     = errors.New("no such page")
)

type service struct {
	cfg    Config
	run    Runner
	logger *slog.Logger
	slots  chan struct{}
	// workers caps the Tesseract runs at once.
	workers chan struct{}
	// rapid is the RapidOCR service, or nil.
	rapid *rapidOCR
}

// NewHandler returns the service's HTTP handler.
func NewHandler(cfg Config, run Runner, logger *slog.Logger) http.Handler {
	if len(cfg.PageModes) == 0 {
		cfg.PageModes = DefaultPageModes
	}
	s := &service{
		cfg: cfg, run: run, logger: logger, slots: make(chan struct{}, max(cfg.Concurrency, 1)),
		workers: make(chan struct{}, max(cfg.Workers, 1)),
		rapid:   newRapidOCR(cfg.RapidOCRURL, cmp.Or(cfg.RapidOCRTimeout, DefaultRapidOCRTimeout)),
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, "ok\n")
	})
	mux.HandleFunc("POST /read", s.handle(s.read))
	mux.HandleFunc("POST /render", s.handle(s.render))
	return mux
}

// job is one request's work on an uploaded file in its own temporary
// directory.
type job struct {
	dir  string
	kind string
	in   string
	data []byte
}

// handle reads the body, makes a temporary directory, waits for a free slot
// and runs work; the directory is removed afterwards.
func (s *service) handle(work func(ctx context.Context, w http.ResponseWriter, r *http.Request, j job) error) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		data, err := io.ReadAll(http.MaxBytesReader(w, r.Body, s.cfg.MaxBody))
		if err != nil {
			var tooBig *http.MaxBytesError
			if errors.As(err, &tooBig) {
				http.Error(w, "file too large", http.StatusRequestEntityTooLarge)
				return
			}
			http.Error(w, "cannot read the body", http.StatusBadRequest)
			return
		}
		kind := sniff(data)
		if kind == "" {
			http.Error(w, "unsupported file type", http.StatusUnsupportedMediaType)
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), s.cfg.ReadTimeout)
		defer cancel()
		select {
		case s.slots <- struct{}{}:
			defer func() { <-s.slots }()
		case <-ctx.Done():
			http.Error(w, "busy", http.StatusServiceUnavailable)
			return
		}
		dir, err := os.MkdirTemp("", "ocrd-")
		if err != nil {
			s.logger.Error("temporary directory", "error", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		defer func() { _ = os.RemoveAll(dir) }()
		in := filepath.Join(dir, "in."+kind)
		if err := os.WriteFile(in, data, 0o600); err != nil {
			s.logger.Error("write input", "error", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		err = work(ctx, w, r, job{dir: dir, kind: kind, in: in, data: data})
		switch {
		case err == nil:
		case errors.Is(err, errNoPage):
			http.Error(w, "no such page", http.StatusNotFound)
		case ctx.Err() != nil:
			s.logger.Warn("timed out", "path", r.URL.Path, "error", err)
			http.Error(w, "timed out", http.StatusGatewayTimeout)
		case errors.Is(err, errUnreadable):
			s.logger.Info("unreadable file", "path", r.URL.Path, "kind", kind, "error", err)
			http.Error(w, "the file cannot be read", http.StatusUnprocessableEntity)
		default:
			s.logger.Error("failed", "path", r.URL.Path, "error", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
		}
	}
}

// minCharsPerPage is the number of letters and digits a page of a PDF's
// text layer needs; with fewer, the PDF is taken to be a scan and read with
// OCR.
const minCharsPerPage = 20

func (s *service) read(ctx context.Context, w http.ResponseWriter, _ *http.Request, j job) error {
	var res ReadResult
	var err error
	if j.kind == kindPDF {
		res, err = s.readPDF(ctx, j)
	} else {
		res, err = s.readImage(ctx, j.in)
	}
	if err != nil {
		return err
	}
	if res.Embedded == nil {
		res.Embedded = []File{}
	}
	if res.Alternatives == nil {
		res.Alternatives = []Reading{}
	}
	w.Header().Set("Content-Type", "application/json")
	return json.NewEncoder(w).Encode(res)
}

func (s *service) readPDF(ctx context.Context, j job) (ReadResult, error) {
	count, err := s.pageCount(ctx, j.in)
	if err != nil {
		return ReadResult{}, err
	}
	pages := min(count, s.cfg.MaxPages)
	last := strconv.Itoa(pages)
	out, err := s.run.Run(ctx, "pdftotext", "-layout", "-enc", "UTF-8", "-l", last, j.in, "-")
	if err != nil {
		return ReadResult{}, fmt.Errorf("%w: %w", errUnreadable, err)
	}
	res := ReadResult{Text: strings.TrimRight(string(out), "\f"), Method: "text", Pages: pages}
	if meaningful(res.Text) < minCharsPerPage*pages {
		images := filepath.Join(j.dir, "pages")
		if err := os.Mkdir(images, 0o700); err != nil {
			return ReadResult{}, err
		}
		if _, err := s.run.Run(ctx, "pdftoppm", "-r", "300", "-png", "-l", last, j.in, filepath.Join(images, "page")); err != nil {
			return ReadResult{}, fmt.Errorf("%w: %w", errUnreadable, err)
		}
		files, err := sortedFiles(images)
		if err != nil {
			return ReadResult{}, err
		}
		modes, short := s.cfg.PageModes, pages <= multiModePages
		if !short {
			modes = modes[:1]
		}
		readings, err := s.ocr(ctx, files, modes, short)
		if err != nil {
			return ReadResult{}, err
		}
		res.Text, res.TextMethod, res.Alternatives, res.Method = readings[0].Text, readings[0].Method, readings[1:], "ocr"
	}
	res.Embedded = s.embedded(ctx, j)
	return res, nil
}

func (s *service) readImage(ctx context.Context, in string) (ReadResult, error) {
	readings, err := s.ocr(ctx, []string{in}, s.cfg.PageModes, true)
	if err != nil {
		return ReadResult{}, err
	}
	text := readings[0].Text
	return ReadResult{
		Text: text, TextMethod: readings[0].Method, Method: "ocr", Pages: strings.Count(text, "\f") + 1,
		Alternatives: readings[1:],
	}, nil
}

// ocr reads the pages (image files) with each page mode, the runs in
// parallel as far as the workers allow, and returns the readings best
// first. With withRapid, RapidOCR (when configured) reads them at the same
// time, and its reading is one more. A mode that fails is left out; only
// when all fail is the error returned. RapidOCR failing is only logged.
func (s *service) ocr(ctx context.Context, pages []string, modes []string, withRapid bool) ([]Reading, error) {
	texts := make([][]string, len(modes))
	errs := make([][]error, len(modes))
	var wg sync.WaitGroup
	withRapid = withRapid && s.rapid != nil
	var rapid Reading
	var rapidErr error
	if withRapid {
		wg.Go(func() {
			start := time.Now()
			rapid, rapidErr = s.rapid.read(ctx, pages)
			s.logger.Debug("rapidocr", "duration", time.Since(start))
		})
	}
	for m, mode := range modes {
		texts[m], errs[m] = make([]string, len(pages)), make([]error, len(pages))
		for p, page := range pages {
			wg.Go(func() {
				text, err := s.tesseract(ctx, page, mode)
				texts[m][p], errs[m][p] = strings.TrimRight(text, "\f"), err
			})
		}
	}
	wg.Wait()
	var readings []Reading
	var first error
	for m, mode := range modes {
		if err := errors.Join(errs[m]...); err != nil {
			first = cmp.Or(first, err)
			continue
		}
		readings = append(readings, Reading{Method: "tesseract-psm" + mode, PageMode: mode, Text: strings.Join(texts[m], "\f")})
	}
	switch {
	case !withRapid:
	case rapidErr != nil:
		s.logger.Warn("rapidocr failed; using Tesseract's readings only", "error", rapidErr)
	default:
		readings = append(readings, rapid)
	}
	if len(readings) == 0 {
		return nil, first
	}
	return bestFirst(readings), nil
}

// tesseract reads one image with a page mode, waiting for a free worker.
func (s *service) tesseract(ctx context.Context, in, mode string) (string, error) {
	select {
	case s.workers <- struct{}{}:
		defer func() { <-s.workers }()
	case <-ctx.Done():
		return "", ctx.Err()
	}
	start := time.Now()
	out, err := s.run.Run(ctx, "tesseract", in, "-", "--psm", mode, "-l", s.cfg.Languages)
	if err != nil {
		return "", fmt.Errorf("%w: %w", errUnreadable, err)
	}
	s.logger.Debug("tesseract", "page_mode", mode, "duration", time.Since(start))
	return string(out), nil
}

// maxEmbedded caps one embedded file that is returned.
const maxEmbedded = 5 << 20

// embedded returns the XML files embedded in a PDF, such as the UBL or
// Factur-X invoice of an e-invoice. A PDF whose attachments cannot be read
// simply has none.
func (s *service) embedded(ctx context.Context, j job) []File {
	dir := filepath.Join(j.dir, "embedded")
	if err := os.Mkdir(dir, 0o700); err != nil {
		return nil
	}
	if _, err := s.run.Run(ctx, "pdfdetach", "-saveall", "-o", dir, j.in); err != nil {
		s.logger.Info("embedded files", "error", err)
		return nil
	}
	files, err := sortedFiles(dir)
	if err != nil {
		return nil
	}
	var out []File
	for _, f := range files {
		info, err := os.Lstat(f)
		if err != nil || !info.Mode().IsRegular() || info.Size() > maxEmbedded {
			continue
		}
		data, err := os.ReadFile(f)
		if err != nil || !isXML(filepath.Base(f), data) {
			continue
		}
		out = append(out, File{Name: filepath.Base(f), ContentType: "application/xml", Data: data})
	}
	return out
}

func isXML(name string, data []byte) bool {
	if strings.EqualFold(filepath.Ext(name), ".xml") {
		return true
	}
	trimmed := bytes.TrimLeft(bytes.TrimPrefix(data, []byte("\xef\xbb\xbf")), " \t\r\n")
	return bytes.HasPrefix(trimmed, []byte("<?xml"))
}

var pagesLine = regexp.MustCompile(`(?m)^Pages:\s+(\d+)`)

func (s *service) pageCount(ctx context.Context, in string) (int, error) {
	out, err := s.run.Run(ctx, "pdfinfo", in)
	if err != nil {
		return 0, fmt.Errorf("%w: %w", errUnreadable, err)
	}
	m := pagesLine.FindSubmatch(out)
	if m == nil {
		return 0, fmt.Errorf("%w: pdfinfo shows no page count", errUnreadable)
	}
	n, _ := strconv.Atoi(string(m[1]))
	if n < 1 {
		return 0, fmt.Errorf("%w: no pages", errUnreadable)
	}
	return n, nil
}

// maxPreviewSide caps the longest side of an image preview in pixels.
const maxPreviewSide = 1600

// Bounds of the max_side parameter of /render, which asks for a page larger
// or smaller than a preview, such as a page an AI model is to read.
const (
	minRenderSide = 200
	maxRenderSide = 4096
)

// maxPixels caps the size of an image that is decoded, so a small file
// cannot claim gigabytes of memory.
const maxPixels = 100_000_000

func (s *service) render(ctx context.Context, w http.ResponseWriter, r *http.Request, j job) error {
	page := 1
	if p := r.URL.Query().Get("page"); p != "" {
		n, err := strconv.Atoi(p)
		if err != nil || n < 1 {
			http.Error(w, "page must be a positive number", http.StatusBadRequest)
			return nil
		}
		page = n
	}
	side := 0 // a preview
	if p := r.URL.Query().Get("max_side"); p != "" {
		n, err := strconv.Atoi(p)
		if err != nil || n < minRenderSide || n > maxRenderSide {
			http.Error(w, fmt.Sprintf("max_side must be a number from %d to %d", minRenderSide, maxRenderSide), http.StatusBadRequest)
			return nil
		}
		side = n
	}
	var data []byte
	var count int
	var err error
	if j.kind == kindPDF {
		data, count, err = s.renderPDF(ctx, j, page, side)
	} else {
		count = 1
		if page > 1 {
			w.Header().Set("X-Page-Count", "1")
			return errNoPage
		}
		data, err = renderImage(j.data, cmp.Or(side, maxPreviewSide))
	}
	if count > 0 {
		w.Header().Set("X-Page-Count", strconv.Itoa(count))
	}
	if err != nil {
		return err
	}
	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Content-Length", strconv.Itoa(len(data)))
	_, _ = w.Write(data)
	return nil
}

// renderPDF renders a page at preview resolution, or with its longest side
// side pixels.
func (s *service) renderPDF(ctx context.Context, j job, page, side int) ([]byte, int, error) {
	count, err := s.pageCount(ctx, j.in)
	if err != nil {
		return nil, 0, err
	}
	if page > count {
		return nil, count, errNoPage
	}
	n := strconv.Itoa(page)
	out := filepath.Join(j.dir, "page")
	size := []string{"-r", "110"}
	if side > 0 {
		size = []string{"-scale-to", strconv.Itoa(side)}
	}
	args := append([]string{"-f", n, "-l", n}, size...)
	if _, err := s.run.Run(ctx, "pdftoppm", append(args, "-png", "-singlefile", j.in, out)...); err != nil {
		return nil, count, fmt.Errorf("%w: %w", errUnreadable, err)
	}
	data, err := os.ReadFile(out + ".png")
	if err != nil {
		return nil, count, fmt.Errorf("%w: %w", errUnreadable, err)
	}
	return data, count, nil
}

// renderImage turns an image into a PNG whose longest side is at most side
// pixels.
func renderImage(data []byte, side int) ([]byte, error) {
	cfg, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("%w: %w", errUnreadable, err)
	}
	if cfg.Width*cfg.Height > maxPixels {
		return nil, fmt.Errorf("%w: %dx%d pixels is too large", errUnreadable, cfg.Width, cfg.Height)
	}
	src, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("%w: %w", errUnreadable, err)
	}
	b := src.Bounds()
	if long := max(b.Dx(), b.Dy()); long > side {
		dst := image.NewRGBA(image.Rect(0, 0, b.Dx()*side/long, b.Dy()*side/long))
		draw.ApproxBiLinear.Scale(dst, dst.Bounds(), src, b, draw.Src, nil)
		src = dst
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, src); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// meaningful counts the letters and digits of text.
func meaningful(text string) int {
	n := 0
	for _, r := range text {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			n++
		}
	}
	return n
}

// sortedFiles lists the files of dir by name; pdftoppm pads page numbers, so
// this is page order.
func sortedFiles(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(entries))
	for _, e := range entries {
		out = append(out, filepath.Join(dir, e.Name()))
	}
	sort.Strings(out)
	return out, nil
}
