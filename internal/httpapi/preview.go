package httpapi

import (
	"errors"
	"io"
	"net/http"
	"strconv"

	"github.com/adirkuhn/moneyharpy/internal/attachments"
	"github.com/adirkuhn/moneyharpy/internal/journal"
	"github.com/adirkuhn/moneyharpy/internal/ocr"
	"github.com/adirkuhn/moneyharpy/internal/purchases"
)

// incomingAttachmentKinds names the download link kind of the attachments of
// each type of incoming document.
var incomingAttachmentKinds = map[string]string{
	purchases.PurchaseInvoice:      filePurchaseInvoiceAttachment,
	purchases.Receipt:              fileReceiptAttachment,
	purchases.GeneralDocument:      fileGeneralDocumentAttachment,
	purchases.TypelessDocument:     fileTypelessAttachment,
	journal.GeneralJournalDocument: fileJournalAttachment,
}

// ocrService is the configured OCR service, or one that is unavailable.
func (s *server) ocrService() ocr.Service {
	if s.deps.OCR == nil {
		return ocr.Unavailable{}
	}
	return s.deps.OCR
}

// PreviewIncomingDocumentAttachment implements ServerInterface: one page of
// an attachment of an incoming document of any type, as a PNG image.
func (s *server) PreviewIncomingDocumentAttachment(w http.ResponseWriter, r *http.Request, administrationID AdministrationId, id Id, attachmentID Identifier, params PreviewIncomingDocumentAttachmentParams) {
	admin, docID, ok := parseIDs(w, administrationID, id)
	if !ok {
		return
	}
	attID, ok := parseID(w, attachmentID)
	if !ok {
		return
	}
	page := 1
	if params.Page != nil {
		page = *params.Page
	}
	u := user(r).ID
	doc, err := s.deps.Purchases.IncomingDocument(r.Context(), admin, docID, u)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	a, body, err := s.openAttachment(r, fileLink{Kind: incomingAttachmentKinds[doc.Kind], Admin: admin, ID: docID, Attachment: attID, User: u})
	if err != nil {
		s.fail(w, r, err)
		return
	}
	data, err := readAttachment(body)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	p, err := s.ocrService().Render(r.Context(), ocr.File{Name: a.Filename, ContentType: a.ContentType, Data: data}, page)
	switch {
	case err == nil:
	case errors.Is(err, ocr.ErrUnavailable), errors.Is(err, ocr.ErrUnsupported), errors.Is(err, ocr.ErrNoPage):
		// The page then shows the file itself.
		if p.Count > 0 {
			w.Header().Set("X-Page-Count", strconv.Itoa(p.Count))
		}
		writeError(w, http.StatusNotFound, "Not found")
		return
	default:
		s.fail(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("X-Page-Count", strconv.Itoa(p.Count))
	w.Header().Set("Content-Length", strconv.Itoa(len(p.PNG)))
	// Attachments never change; their previews can be kept for a day.
	w.Header().Set("Cache-Control", "private, max-age=86400")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(http.StatusOK)
	// The client went away if this fails; there is nobody left to tell.
	_, _ = w.Write(p.PNG)
}

// readAttachment reads and closes an attachment's body.
func readAttachment(body io.ReadCloser) ([]byte, error) {
	defer func() { _ = body.Close() }()
	return io.ReadAll(io.LimitReader(body, attachments.MaxSize+1))
}
