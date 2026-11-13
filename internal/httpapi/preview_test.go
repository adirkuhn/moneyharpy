package httpapi_test

import (
	"context"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/adirkuhn/moneyharpy/internal/ocr"
)

// fakeOCR renders every file as two pages, unless it is unavailable.
type fakeOCR struct {
	mu          sync.Mutex
	unavailable bool
	rendered    []ocr.File
}

func (f *fakeOCR) Read(context.Context, ocr.File) (ocr.Text, error) {
	return ocr.Text{}, ocr.ErrUnavailable
}

func (f *fakeOCR) Render(_ context.Context, file ocr.File, page int) (ocr.Page, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.unavailable {
		return ocr.Page{}, ocr.ErrUnavailable
	}
	f.rendered = append(f.rendered, file)
	if page > 2 {
		return ocr.Page{Count: 2}, ocr.ErrNoPage
	}
	return ocr.Page{PNG: []byte("\x89PNG page " + string(rune('0'+page))), Count: 2}, nil
}

func (f *fakeOCR) Ping(context.Context) error { return nil }

func TestPreviewIncomingDocumentAttachment(t *testing.T) {
	h := newHarness(t, options{})
	owner, stranger := h.client(), h.client()
	owner.register()
	stranger.register()
	a := owner.createAdministration("Harpy")
	api := "/api/v2/" + a.Id
	day := adminNow().Format(time.DateOnly)
	type attachment struct {
		ID string `json:"id"`
	}

	general := decode[map[string]any](t, owner.do(http.MethodPost, api+"/documents/general_documents", map[string]any{"general_document": map[string]any{
		"reference": "Lease", "date": day,
	}}))
	id := general["id"].(string)
	scan := []byte("\x89PNG\r\n\x1a\nscan of a lease")
	att := decode[attachment](t, owner.attach(api+"/documents/general_documents/"+id+"/attachments", "scan.png", "image/png", scan))
	preview := api + "/documents/" + id + "/attachments/" + att.ID + "/preview"

	r := owner.do(http.MethodGet, preview+"?page=2", nil)
	wantStatus(t, r, http.StatusOK)
	if string(r.body) != "\x89PNG page 2" || r.header.Get("X-Page-Count") != "2" || r.header.Get("Content-Type") != "image/png" ||
		!strings.Contains(r.header.Get("Cache-Control"), "private") {
		t.Errorf("preview = %q %v", r.body, r.header)
	}
	if got := h.ocr.rendered[0]; string(got.Data) != string(scan) || got.Name != "scan.png" {
		t.Errorf("rendered = %+v", got)
	}

	// Journal entries keep their attachments elsewhere; they preview too.
	rent, deposits := category(t, owner, a.Id, "Rent"), category(t, owner, a.Id, "Private deposits")
	entry := decode[journalDocument](t, owner.do(http.MethodPost, api+"/documents/general_journal_documents", map[string]any{"general_journal_document": map[string]any{
		"reference": "Rent", "date": day,
		"general_journal_document_entries_attributes": []map[string]any{{"ledger_account_id": rent, "debit": "400"}, {"ledger_account_id": deposits, "credit": "400"}},
	}}))
	pdf := decode[attachment](t, owner.attach(api+"/documents/general_journal_documents/"+entry.ID+"/attachments", "rent.pdf", "application/pdf", []byte("%PDF-1.4 rent")))
	wantStatus(t, owner.do(http.MethodGet, api+"/documents/"+entry.ID+"/attachments/"+pdf.ID+"/preview", nil), http.StatusOK)
	if got := h.ocr.rendered[1]; string(got.Data) != "%PDF-1.4 rent" {
		t.Errorf("rendered = %+v", got)
	}
	// Another document's attachment is not found.
	wantStatus(t, owner.do(http.MethodGet, api+"/documents/"+entry.ID+"/attachments/"+att.ID+"/preview", nil), http.StatusNotFound)

	wantStatus(t, owner.do(http.MethodGet, preview+"?page=3", nil), http.StatusNotFound)
	wantStatus(t, owner.do(http.MethodGet, api+"/documents/"+id+"/attachments/1/preview", nil), http.StatusNotFound)
	if r := stranger.do(http.MethodGet, preview, nil); r.status == http.StatusOK {
		t.Errorf("a stranger previewed the attachment: %d", r.status)
	}
	h.ocr.unavailable = true
	wantStatus(t, owner.do(http.MethodGet, preview, nil), http.StatusNotFound)
}
