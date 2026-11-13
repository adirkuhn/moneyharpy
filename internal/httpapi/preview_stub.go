package httpapi

import "net/http"

// PreviewIncomingDocumentAttachment implements ServerInterface.
//
// TEMPORARY placeholder so this branch builds on its own: the preview
// endpoint is built on another branch. Delete this file when merging it.
func (s *server) PreviewIncomingDocumentAttachment(w http.ResponseWriter, _ *http.Request, _ AdministrationId, _ Id, _ Identifier, _ PreviewIncomingDocumentAttachmentParams) {
	writeError(w, http.StatusNotImplemented, "Not implemented")
}
