// Package ocr talks to the OCR service, a separate container with Poppler and
// Tesseract that turns attachments into text and PDF pages into images. The
// app runs without it: Scan and Recognise then reads only e-invoices, and
// previews fall back to the file itself.
package ocr

import (
	"context"
	"errors"
)

// ErrUnavailable means the OCR service is not configured or does not answer.
var ErrUnavailable = errors.New("ocr service unavailable")

// ErrUnsupported means the service cannot read or render this kind of file.
var ErrUnsupported = errors.New("file type not supported by the ocr service")

// File is an attachment as stored.
type File struct {
	Name        string
	ContentType string
	Data        []byte
}

// Text is what the service read from a file.
type Text struct {
	// Text is the text, pages separated by a form feed ("\f").
	Text string
	// Method is "text" for the text of a digital PDF and "ocr" for text read
	// from a scan or photo.
	Method string
	// TextMethod tells which OCR engine read Text, as Reading.Method; empty
	// for the text of a digital PDF.
	TextMethod string
	// Pages is the number of pages read.
	Pages int
	// Embedded are files embedded in a PDF, such as the UBL or Factur-X XML of
	// an e-invoice.
	Embedded []File
	// Alternatives are other readings of OCR, with other page modes or
	// engines, that the service thought worse than Text: a reader may
	// understand one of them better.
	Alternatives []Reading
}

// Reading is the text one OCR engine read.
type Reading struct {
	// Method is the engine and its mode, such as "tesseract-psm6" or
	// "rapidocr".
	Method string
	Text   string
}

// Page is one page rendered as a PNG image.
type Page struct {
	PNG   []byte
	Count int
}

// Service reads and renders attachments.
type Service interface {
	// Read returns the text of a PDF or image; a PDF without a text layer is
	// read with OCR.
	Read(ctx context.Context, f File) (Text, error)
	// Render returns page (1-based) of a PDF or image as a PNG, with the page
	// count.
	Render(ctx context.Context, f File, page int) (Page, error)
	// RenderAt renders a page as Render does, with its longest side
	// maxSide pixels (at most, for an image): larger than a preview, for
	// an AI model to read the text.
	RenderAt(ctx context.Context, f File, page, maxSide int) (Page, error)
	// Ping tells whether the service answers.
	Ping(ctx context.Context) error
}
