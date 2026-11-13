package httpapi

import "net/http"

// Placeholders so this branch builds on its own; the recognition and
// settings branches implement these. Delete this file when merging them.

// RecognizeIncomingDocument implements ServerInterface.
func (s *server) RecognizeIncomingDocument(w http.ResponseWriter, r *http.Request, administrationID AdministrationId, id Id) {
	Unimplemented{}.RecognizeIncomingDocument(w, r, administrationID, id)
}

// TestRecognition implements ServerInterface.
func (s *server) TestRecognition(w http.ResponseWriter, r *http.Request) {
	Unimplemented{}.TestRecognition(w, r)
}
