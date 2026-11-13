package httpapi

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/adirkuhn/moneyharpy/internal/contacts"
	"github.com/adirkuhn/moneyharpy/internal/purchases"
	"github.com/adirkuhn/moneyharpy/internal/recognize"
	"github.com/adirkuhn/moneyharpy/internal/resource"
	"github.com/adirkuhn/moneyharpy/internal/webhooks"
)

// noRecognition is what a document never read shows.
var noRecognition = Recognition{State: RecognitionStateNone, Fields: []RecognizedField{}}

func toRecognition(r recognize.Recognition) Recognition {
	out := Recognition{State: RecognitionState(r.State), Fields: make([]RecognizedField, len(r.Fields))}
	if r.State == "" {
		out.State = RecognitionStateNone
	}
	if r.Source != "" {
		src := RecognitionSource(r.Source)
		out.Source = &src
	}
	for i, f := range r.Fields {
		out.Fields[i] = RecognizedField{Field: f.Name, Confidence: RecognizedFieldConfidence(f.Confidence)}
	}
	return out
}

// recognitionsOf loads what Scan and Recognise read of the documents, each
// document never read showing none.
func (s *server) recognitionsOf(r *http.Request, admin resource.ID, ids []resource.ID) (map[resource.ID]Recognition, error) {
	out := make(map[resource.ID]Recognition, len(ids))
	for _, id := range ids {
		out[id] = noRecognition
	}
	if s.deps.Recognition == nil {
		return out, nil
	}
	found, err := s.deps.Recognition.Recognitions(r.Context(), admin, ids)
	if err != nil {
		return nil, err
	}
	for id, rec := range found {
		out[id] = toRecognition(rec)
	}
	return out, nil
}

// writeIncomingDocuments writes incoming document summaries with their
// recognition state.
func (s *server) toIncomingDocuments(r *http.Request, admin resource.ID, list []purchases.Incoming) ([]incomingDocumentJSON, error) {
	ids := make([]resource.ID, len(list))
	for i, d := range list {
		ids[i] = d.ID
	}
	recs, err := s.recognitionsOf(r, admin, ids)
	if err != nil {
		return nil, err
	}
	out := make([]incomingDocumentJSON, len(list))
	for i, d := range list {
		out[i] = toIncomingDocument(d)
		out[i].RecognitionState = string(recs[d.ID].State)
	}
	return out, nil
}

func (s *server) writeIncomingDocument(w http.ResponseWriter, r *http.Request, admin resource.ID, status int, d purchases.Incoming) {
	out, err := s.toIncomingDocuments(r, admin, []purchases.Incoming{d})
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, status, out[0])
}

// RecognizeIncomingDocument implements ServerInterface.
func (s *server) RecognizeIncomingDocument(w http.ResponseWriter, r *http.Request, administrationID AdministrationId, id Id) {
	admin, docID, ok := parseIDs(w, administrationID, id)
	if !ok {
		return
	}
	if s.deps.Recognition == nil {
		writeJSON(w, http.StatusUnprocessableEntity, validationBody(map[string][]string{"base": {"Scan and Recognise is not available"}}))
		return
	}
	d, err := s.deps.Recognition.Recognize(r.Context(), admin, docID, user(r).ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.writeIncomingDocument(w, r, admin, http.StatusAccepted, d)
}

// TestRecognition implements ServerInterface.
func (s *server) TestRecognition(w http.ResponseWriter, r *http.Request) {
	if !requireInstanceAdmin(w, r) {
		return
	}
	out := RecognitionCheck{Ai: Off}
	if s.deps.Recognition != nil {
		c, err := s.deps.Recognition.Check(r.Context())
		if err != nil {
			s.fail(w, r, err)
			return
		}
		out = RecognitionCheck{OcrAvailable: c.OCRAvailable, Ai: RecognitionCheckAi(c.AI)}
		if c.Message != "" {
			out.Message = &c.Message
		}
	}
	writeJSON(w, http.StatusOK, out)
}

// RecognitionNotifier publishes what Scan and Recognise changes in the
// background as webhook events: the documents it fills in and the
// suppliers it adds.
type RecognitionNotifier struct {
	Webhooks *webhooks.Service
	Logger   *slog.Logger
}

// DocumentUpdated implements recognize.Notifier.
func (n RecognitionNotifier) DocumentUpdated(ctx context.Context, admin resource.ID, d purchases.Document) {
	entity, _ := json.Marshal(map[string]any{
		"id": d.ID.String(), "administration_id": admin.String(), "contact_id": idString(optionalID(d.ContactID)), "reference": nz(d.Reference),
		"state": d.State, "total_price_incl_tax": d.Totals.InclTax.String(), "version": d.Version(), "uri": d.Ref().URI(),
	})
	n.publish(ctx, admin, webhooks.Event{EntityType: "Document", EntityID: d.ID.String(), Action: "document_updated", State: d.State, Entity: entity})
}

// ContactCreated implements recognize.Notifier.
func (n RecognitionNotifier) ContactCreated(ctx context.Context, admin resource.ID, c contacts.Contact) {
	entity, _ := json.Marshal(map[string]any{
		"id": c.ID.String(), "administration_id": admin.String(), "company_name": c.CompanyName, "version": c.Version(), "uri": c.Ref().URI(),
	})
	n.publish(ctx, admin, webhooks.Event{EntityType: "Contact", EntityID: c.ID.String(), Action: "contact_created", Entity: entity})
}

func (n RecognitionNotifier) publish(ctx context.Context, admin resource.ID, e webhooks.Event) {
	if n.Webhooks == nil {
		return
	}
	if err := n.Webhooks.Publish(context.WithoutCancel(ctx), admin, e); err != nil && n.Logger != nil {
		n.Logger.ErrorContext(ctx, "publish webhook event", "action", e.Action, "error", err)
	}
}

func optionalID(id resource.ID) *resource.ID {
	if id == 0 {
		return nil
	}
	return &id
}
