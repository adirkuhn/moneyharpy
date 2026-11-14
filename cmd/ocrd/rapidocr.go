package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

// rapidMethod labels a reading of RapidOCR.
const rapidMethod = "rapidocr"

// maxRapidResponse caps what RapidOCR may answer for one page.
const maxRapidResponse = 16 << 20

// rapidOCR asks the RapidOCR service (ocr-rapid/), a second OCR engine
// next to Tesseract: PaddleOCR's models, which read phone photos of till
// receipts well and keep their columns on one line.
type rapidOCR struct {
	url     string
	timeout time.Duration
	http    *http.Client
}

func newRapidOCR(url string, timeout time.Duration) *rapidOCR {
	url = strings.TrimRight(strings.TrimSpace(url), "/")
	if url == "" {
		return nil
	}
	return &rapidOCR{url: url, timeout: timeout, http: &http.Client{}}
}

// read reads the pages (image files) one after the other, within the
// timeout, into one reading with the pages separated by form feeds.
func (r *rapidOCR) read(ctx context.Context, pages []string) (Reading, error) {
	ctx, cancel := context.WithTimeout(ctx, r.timeout)
	defer cancel()
	texts := make([]string, len(pages))
	for i, page := range pages {
		data, err := os.ReadFile(page)
		if err != nil {
			return Reading{}, err
		}
		if texts[i], err = r.readPage(ctx, data); err != nil {
			return Reading{}, fmt.Errorf("page %d: %w", i+1, err)
		}
	}
	return Reading{Method: rapidMethod, Text: strings.Join(texts, "\f")}, nil
}

func (r *rapidOCR) readPage(ctx context.Context, data []byte) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, r.url+"/read", bytes.NewReader(data))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/octet-stream")
	res, err := r.http.Do(req)
	if err != nil {
		return "", err
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusOK {
		return "", fmt.Errorf("answered %s", res.Status)
	}
	var body struct {
		Text string `json:"text"`
	}
	if err := json.NewDecoder(io.LimitReader(res.Body, maxRapidResponse)).Decode(&body); err != nil {
		return "", fmt.Errorf("read the answer: %w", err)
	}
	return body.Text, nil
}
