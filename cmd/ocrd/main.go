// Command ocrd is MoneyHarpy's OCR service: it reads the text of PDFs and
// images with Poppler and Tesseract, and renders PDF pages as images for
// previews. It runs in its own container without network access; the app
// talks to it over HTTP (see internal/ocr).
package main

import (
	"cmp"
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"
)

func main() {
	healthcheck := flag.Bool("healthcheck", false, "check that the service on LISTEN_ADDR answers, for Docker's HEALTHCHECK")
	flag.Parse()
	addr := cmp.Or(os.Getenv("LISTEN_ADDR"), ":8090")
	if *healthcheck {
		if err := checkHealth(addr); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	if err := run(logger, addr); err != nil {
		logger.Error("server stopped", "error", err)
		os.Exit(1)
	}
}

func run(logger *slog.Logger, addr string) error {
	for _, tool := range []string{"pdfinfo", "pdftotext", "pdftoppm", "pdfdetach", "tesseract"} {
		if _, err := exec.LookPath(tool); err != nil {
			logger.Warn("tool not found; requests that need it fail", "tool", tool)
		}
	}
	cfg := Config{
		Languages:   cmp.Or(os.Getenv("OCR_LANGUAGES"), "nld+eng"),
		MaxBody:     int64(envInt("MAX_BODY_MB", 20)) << 20,
		MaxPages:    envInt("MAX_PAGES", 20),
		ReadTimeout: time.Duration(envInt("READ_TIMEOUT_SECONDS", 120)) * time.Second,
		Concurrency: envInt("CONCURRENCY", runtime.NumCPU()),
		PageModes:   pageModes(os.Getenv("OCR_PAGE_MODES")),
		Workers:     envInt("OCR_WORKERS", runtime.NumCPU()),
		// Optional: a second OCR engine, asked next to Tesseract.
		RapidOCRURL:     os.Getenv("RAPIDOCR_URL"),
		RapidOCRTimeout: time.Duration(envInt("RAPIDOCR_TIMEOUT_SECONDS", 60)) * time.Second,
	}
	if cfg.RapidOCRURL != "" {
		logger.Info("reading photos and short scans with RapidOCR too", "url", cfg.RapidOCRURL, "timeout", cfg.RapidOCRTimeout)
	}
	srv := &http.Server{
		Addr:              addr,
		Handler:           NewHandler(cfg, ExecRunner{}, logger),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       time.Minute,
		WriteTimeout:      cfg.ReadTimeout + 30*time.Second,
		IdleTimeout:       2 * time.Minute,
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	errc := make(chan error, 1)
	go func() {
		logger.Info("listening", "addr", addr)
		errc <- srv.ListenAndServe()
	}()
	select {
	case err := <-errc:
		return err
	case <-ctx.Done():
	}
	shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdown); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

func envInt(name string, fallback int) int {
	if n, err := strconv.Atoi(os.Getenv(name)); err == nil && n > 0 {
		return n
	}
	return fallback
}

// pageModes reads a list of Tesseract page modes such as "3,6,4"; without
// a valid one it is DefaultPageModes.
func pageModes(s string) []string {
	var out []string
	for _, f := range strings.Split(s, ",") {
		if n, err := strconv.Atoi(strings.TrimSpace(f)); err == nil && (n == 1 || n >= 3 && n <= 13) { // 0 and 2 read no text
			out = append(out, strconv.Itoa(n))
		}
	}
	if len(out) == 0 {
		return DefaultPageModes
	}
	return out
}

func checkHealth(addr string) error {
	host := addr
	if len(host) > 0 && host[0] == ':' {
		host = "127.0.0.1" + host
	}
	client := &http.Client{Timeout: 3 * time.Second}
	res, err := client.Get("http://" + host + "/healthz")
	if err != nil {
		return err
	}
	_ = res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("healthz answered %s", res.Status)
	}
	return nil
}
