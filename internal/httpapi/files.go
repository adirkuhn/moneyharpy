package httpapi

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/adirkuhn/moneyharpy/internal/attachments"
	"github.com/adirkuhn/moneyharpy/internal/resource"
)

// fileLinkTTL is how long a download link works, as Moneybird's do.
const fileLinkTTL = 30 * time.Second

// Kinds of file a download link names.
const (
	filePDF                = "pdf"
	filePackingSlip        = "packing_slip"
	fileSalesAttachment    = "sales_invoice_attachment"
	fileExternalAttachment = "external_sales_invoice_attachment"
	fileJournalAttachment  = "general_journal_document_attachment"
)

// fileLink names a file and the user who asked for it. Download operations
// redirect to it, as Moneybird redirects to a temporary storage link.
type fileLink struct {
	Kind       string      `json:"k"`
	Admin      resource.ID `json:"a"`
	ID         resource.ID `json:"i"`
	Attachment resource.ID `json:"t,omitempty"`
	User       resource.ID `json:"u"`
	Stationery bool        `json:"s,omitempty"`
	Expires    int64       `json:"e"`
}

// fileKey returns the key download links are signed with: the configured
// one, or a random one for this process.
func fileKey(configured []byte) []byte {
	if len(configured) > 0 {
		return configured
	}
	key := make([]byte, 32)
	_, _ = rand.Read(key) // Never fails; see crypto/rand.
	return key
}

func (s *server) sign(payload []byte) []byte {
	mac := hmac.New(sha256.New, s.fileKey)
	mac.Write(payload)
	return mac.Sum(nil)
}

// fileURL signs the link and returns its absolute address.
func (s *server) fileURL(l fileLink) string {
	l.Expires = time.Now().Add(fileLinkTTL).Unix()
	payload, _ := json.Marshal(l)
	enc := base64.RawURLEncoding
	token := enc.EncodeToString(payload) + "." + enc.EncodeToString(s.sign(payload))
	return strings.TrimSuffix(s.deps.BaseURL, "/") + "/api/files/" + token
}

// readFileLink checks a token's signature and age.
func (s *server) readFileLink(token string) (fileLink, bool) {
	enc := base64.RawURLEncoding
	rawPayload, rawSig, ok := strings.Cut(token, ".")
	if !ok {
		return fileLink{}, false
	}
	payload, err1 := enc.DecodeString(rawPayload)
	sig, err2 := enc.DecodeString(rawSig)
	if err1 != nil || err2 != nil || !hmac.Equal(sig, s.sign(payload)) {
		return fileLink{}, false
	}
	var l fileLink
	if err := json.Unmarshal(payload, &l); err != nil || time.Now().Unix() > l.Expires {
		return fileLink{}, false
	}
	return l, true
}

// redirectToFile answers 302 with a temporary link to the file, with
// Moneybird's text for documents and for attachments.
func (s *server) redirectToFile(w http.ResponseWriter, l fileLink) {
	to := s.fileURL(l)
	w.Header().Set("Location", to)
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusFound)
	text := `{"message":"Use the Location header to download the attachment"}`
	if l.Attachment == 0 {
		text = "This resource has been moved temporarily to " + to + "."
	}
	// The client went away if this fails; there is nobody left to tell.
	_, _ = io.WriteString(w, text)
}

// openAttachment opens the attachment a link names, as the user it names.
func (s *server) openAttachment(r *http.Request, l fileLink) (attachments.Attachment, io.ReadCloser, error) {
	switch l.Kind {
	case fileSalesAttachment:
		return s.deps.Sales.OpenAttachment(r.Context(), l.Admin, l.ID, l.Attachment, l.User)
	case fileExternalAttachment:
		return s.deps.Sales.OpenExternalAttachment(r.Context(), l.Admin, l.ID, l.Attachment, l.User)
	default:
		return s.deps.Journal.OpenDocumentAttachment(r.Context(), l.Admin, l.ID, l.Attachment, l.User)
	}
}

// redirectToAttachment checks that the user can open the attachment, and
// redirects to a link to it.
func (s *server) redirectToAttachment(w http.ResponseWriter, r *http.Request, kind string, administrationID, id, attachmentID string) {
	admin, rid, ok := parseIDs(w, administrationID, id)
	if !ok {
		return
	}
	att, ok := parseID(w, attachmentID)
	if !ok {
		return
	}
	l := fileLink{Kind: kind, Admin: admin, ID: rid, Attachment: att, User: user(r).ID}
	_, body, err := s.openAttachment(r, l)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	_ = body.Close()
	s.redirectToFile(w, l)
}

// redirectToDocument checks that the user can read the invoice, and
// redirects to a link to its PDF or packing slip.
func (s *server) redirectToDocument(w http.ResponseWriter, r *http.Request, kind string, administrationID AdministrationId, id Id, media *string) {
	admin, rid, ok := parseIDs(w, administrationID, id)
	if !ok {
		return
	}
	if _, err := s.deps.Sales.Invoice(r.Context(), admin, rid, user(r).ID); err != nil {
		s.fail(w, r, err)
		return
	}
	s.redirectToFile(w, fileLink{Kind: kind, Admin: admin, ID: rid, User: user(r).ID, Stationery: deref(media) == "stationery"})
}

// DownloadFile implements ServerInterface: it serves what a download link
// names, re-checking that its user may still read it.
func (s *server) DownloadFile(w http.ResponseWriter, r *http.Request, token string) {
	l, ok := s.readFileLink(token)
	if !ok {
		writeError(w, http.StatusNotFound, "Not found")
		return
	}
	w.Header().Set("Cache-Control", "private, no-store")
	switch l.Kind {
	case filePDF:
		data, name, err := s.deps.Sales.PDF(r.Context(), l.Admin, l.ID, l.User, l.Stationery)
		if err != nil {
			s.fail(w, r, err)
			return
		}
		writeDocument(w, "application/pdf", name, data)
	case filePackingSlip:
		data, name, err := s.deps.Sales.PackingSlip(r.Context(), l.Admin, l.ID, l.User, l.Stationery)
		if err != nil {
			s.fail(w, r, err)
			return
		}
		writeDocument(w, "application/pdf", name, data)
	default:
		a, body, err := s.openAttachment(r, l)
		if err != nil {
			s.fail(w, r, err)
			return
		}
		writeFile(w, a, body)
	}
}
