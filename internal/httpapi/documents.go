package httpapi

import (
	"net/http"
	"time"

	"github.com/adirkuhn/moneyharpy/internal/purchases"
	"github.com/adirkuhn/moneyharpy/internal/resource"
)

type generalDocumentJSON struct {
	ID               string           `json:"id"`
	AdministrationID string           `json:"administration_id"`
	ContactID        *string          `json:"contact_id"`
	Contact          *contactJSON     `json:"contact"`
	Reference        *string          `json:"reference"`
	Date             string           `json:"date"`
	DueDate          *string          `json:"due_date"`
	ReminderDate     *string          `json:"reminder_date"`
	ReminderMessage  *string          `json:"reminder_message"`
	EntryNumber      int              `json:"entry_number"`
	State            string           `json:"state"`
	ExchangeRate     string           `json:"exchange_rate"`
	CreatedAt        time.Time        `json:"created_at"`
	UpdatedAt        time.Time        `json:"updated_at"`
	Version          int64            `json:"version"`
	Notes            []noteJSON       `json:"notes"`
	Attachments      []attachmentJSON `json:"attachments"`
	Events           []any            `json:"events"`
	URI              string           `json:"uri"`
}

func (s *server) toGeneralDocuments(r *http.Request, admin resource.ID, list []purchases.General) ([]generalDocumentJSON, error) {
	ids := make([]resource.ID, len(list))
	for i, g := range list {
		ids[i] = g.ContactID
	}
	byID, err := s.contactsOf(r, admin, ids)
	if err != nil {
		return nil, err
	}
	out := make([]generalDocumentJSON, len(list))
	for i, g := range list {
		o := generalDocumentJSON{
			ID: g.ID.String(), AdministrationID: g.AdministrationID.String(), Reference: nz(g.Reference), Date: g.Date.Format(time.DateOnly),
			DueDate: dateText(g.DueDate), ReminderDate: dateText(g.ReminderDate), ReminderMessage: nz(g.ReminderMessage), EntryNumber: g.EntryNumber,
			State: g.State, ExchangeRate: "1.0", CreatedAt: g.CreatedAt, UpdatedAt: g.UpdatedAt, Version: g.Version(), Notes: []noteJSON{},
			Attachments: []attachmentJSON{}, Events: []any{}, URI: g.Ref().URI(),
		}
		if g.ContactID != 0 {
			o.ContactID = ptr(g.ContactID.String())
		}
		if c := byID[g.ContactID]; c != nil {
			cj := s.toContact(*c)
			o.Contact = &cj
		}
		for _, n := range g.Notes {
			o.Notes = append(o.Notes, toNote(n))
		}
		for _, a := range g.Attachments {
			o.Attachments = append(o.Attachments, toAttachment(a))
		}
		out[i] = o
	}
	return out, nil
}

func (s *server) writeGeneralDocument(w http.ResponseWriter, r *http.Request, admin resource.ID, status int, g purchases.General, err error) {
	if err != nil {
		s.fail(w, r, err)
		return
	}
	out, err := s.toGeneralDocuments(r, admin, []purchases.General{g})
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, status, out[0])
}

func readGeneralChanges(w http.ResponseWriter, r *http.Request) (purchases.GeneralChanges, bool) {
	body, ok := decodeBody[struct {
		Document struct {
			Reference    *string     `json:"reference"`
			Date         *string     `json:"date"`
			DueDate      *string     `json:"due_date"`
			ReminderDate *string     `json:"reminder_date"`
			ContactID    *flexString `json:"contact_id"`
			Reminder     *struct {
				Date    *string `json:"date"`
				Message *string `json:"message"`
			} `json:"reminder"`
		} `json:"general_document"`
		RemoveContact *flexBool `json:"remove_contact"`
	}](w, r)
	if !ok {
		return purchases.GeneralChanges{}, false
	}
	in := body.Document
	c := purchases.GeneralChanges{Reference: in.Reference, Date: in.Date, DueDate: in.DueDate, ReminderDate: in.ReminderDate, RemoveContact: deref(body.RemoveContact.ptr())}
	if in.Reminder != nil {
		c.ReminderDate, c.ReminderMessage = in.Reminder.Date, in.Reminder.Message
	}
	c.ContactID, ok = flexOptionalID(w, in.ContactID)
	return c, ok
}

// ListGeneralDocuments implements ServerInterface.
func (s *server) ListGeneralDocuments(w http.ResponseWriter, r *http.Request, administrationID AdministrationId, params ListGeneralDocumentsParams) {
	admin, ok := parseID(w, administrationID)
	if !ok {
		return
	}
	q, ok := s.purchaseQuery(w, r, admin, params.Filter, params.NewFilter)
	if !ok {
		return
	}
	q.Page, q.PerPage = pageNumbers(params.Page, params.PerPage)
	q.Ascending = params.Sort != nil && *params.Sort == "date_asc"
	list, total, err := s.deps.Purchases.GeneralList(r.Context(), admin, user(r).ID, q)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	out, err := s.toGeneralDocuments(r, admin, list)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.writeLinks(w, r, max(q.Page, 1), cmpPerPage(q.PerPage, purchases.DefaultPerPage), total)
	writeJSON(w, http.StatusOK, out)
}

// CreateGeneralDocument implements ServerInterface.
func (s *server) CreateGeneralDocument(w http.ResponseWriter, r *http.Request, administrationID AdministrationId) {
	admin, ok := parseID(w, administrationID)
	if !ok {
		return
	}
	c, ok := readGeneralChanges(w, r)
	if !ok {
		return
	}
	g, err := s.deps.Purchases.CreateGeneral(r.Context(), admin, user(r).ID, c)
	s.writeGeneralDocument(w, r, admin, http.StatusCreated, g, err)
}

// ListGeneralDocumentVersions implements ServerInterface.
func (s *server) ListGeneralDocumentVersions(w http.ResponseWriter, r *http.Request, administrationID AdministrationId, params ListGeneralDocumentVersionsParams) {
	admin, ok := parseID(w, administrationID)
	if !ok {
		return
	}
	q, ok := s.purchaseQuery(w, r, admin, params.Filter, nil)
	if !ok {
		return
	}
	list, err := s.deps.Purchases.GeneralVersions(r.Context(), admin, user(r).ID, q)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	out := make([]SynchronizationItem, len(list))
	for i, g := range list {
		out[i] = SynchronizationItem{Id: g.ID.String(), Version: int(g.Version())}
	}
	writeJSON(w, http.StatusOK, out)
}

// FetchGeneralDocuments implements ServerInterface.
func (s *server) FetchGeneralDocuments(w http.ResponseWriter, r *http.Request, administrationID AdministrationId) {
	admin, ok := parseID(w, administrationID)
	if !ok {
		return
	}
	ids, ok := readSyncIDs(w, r)
	if !ok {
		return
	}
	list, err := s.deps.Purchases.GeneralDocumentsByID(r.Context(), admin, user(r).ID, ids)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	out, err := s.toGeneralDocuments(r, admin, list)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

// GetGeneralDocument implements ServerInterface.
func (s *server) GetGeneralDocument(w http.ResponseWriter, r *http.Request, administrationID AdministrationId, id Id) {
	admin, rid, ok := parseIDs(w, administrationID, id)
	if !ok {
		return
	}
	g, err := s.deps.Purchases.GeneralDocument(r.Context(), admin, rid, user(r).ID)
	s.writeGeneralDocument(w, r, admin, http.StatusOK, g, err)
}

// UpdateGeneralDocument implements ServerInterface.
func (s *server) UpdateGeneralDocument(w http.ResponseWriter, r *http.Request, administrationID AdministrationId, id Id) {
	admin, rid, ok := parseIDs(w, administrationID, id)
	if !ok {
		return
	}
	c, ok := readGeneralChanges(w, r)
	if !ok {
		return
	}
	g, err := s.deps.Purchases.UpdateGeneral(r.Context(), admin, rid, user(r).ID, c)
	s.writeGeneralDocument(w, r, admin, http.StatusOK, g, err)
}

// DeleteGeneralDocument implements ServerInterface.
func (s *server) DeleteGeneralDocument(w http.ResponseWriter, r *http.Request, administrationID AdministrationId, id Id) {
	admin, rid, ok := parseIDs(w, administrationID, id)
	if !ok {
		return
	}
	if err := s.deps.Purchases.DeleteGeneral(r.Context(), admin, rid, user(r).ID); err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, http.StatusOK)
}

// CreateGeneralDocumentNote implements ServerInterface.
func (s *server) CreateGeneralDocumentNote(w http.ResponseWriter, r *http.Request, administrationID AdministrationId, id Id) {
	admin, rid, ok := parseIDs(w, administrationID, id)
	if !ok {
		return
	}
	in, ok := decodeNote(w, r)
	if !ok {
		return
	}
	n, err := s.deps.Purchases.AddGeneralNote(r.Context(), admin, rid, user(r).ID, in)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, toNote(n))
}

// DeleteGeneralDocumentNote implements ServerInterface.
func (s *server) DeleteGeneralDocumentNote(w http.ResponseWriter, r *http.Request, administrationID AdministrationId, id Id, noteID Identifier) {
	admin, rid, ok := parseIDs(w, administrationID, id)
	if !ok {
		return
	}
	note, ok := parseID(w, noteID)
	if !ok {
		return
	}
	if err := s.deps.Purchases.DeleteGeneralNote(r.Context(), admin, rid, note, user(r).ID); err != nil {
		s.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// CreateGeneralDocumentAttachment implements ServerInterface.
func (s *server) CreateGeneralDocumentAttachment(w http.ResponseWriter, r *http.Request, administrationID AdministrationId, id Id) {
	admin, rid, ok := parseIDs(w, administrationID, id)
	if !ok {
		return
	}
	name, contentType, data, ok := readUpload(w, r)
	if !ok {
		return
	}
	a, err := s.deps.Purchases.AddGeneralAttachment(r.Context(), admin, rid, user(r).ID, name, contentType, data)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, toAttachment(a))
}

// DeleteGeneralDocumentAttachment implements ServerInterface.
func (s *server) DeleteGeneralDocumentAttachment(w http.ResponseWriter, r *http.Request, administrationID AdministrationId, id Id, attachmentID Identifier) {
	admin, rid, ok := parseIDs(w, administrationID, id)
	if !ok {
		return
	}
	att, ok := parseID(w, attachmentID)
	if !ok {
		return
	}
	if err := s.deps.Purchases.DeleteGeneralAttachment(r.Context(), admin, rid, att, user(r).ID); err != nil {
		s.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// DownloadGeneralDocumentAttachment implements ServerInterface.
func (s *server) DownloadGeneralDocumentAttachment(w http.ResponseWriter, r *http.Request, administrationID AdministrationId, id Id, attachmentID Identifier) {
	s.redirectToAttachment(w, r, fileGeneralDocumentAttachment, administrationID, id, attachmentID)
}

type typelessDocumentJSON struct {
	ID               string           `json:"id"`
	AdministrationID string           `json:"administration_id"`
	ContactID        *string          `json:"contact_id"`
	Contact          *contactJSON     `json:"contact"`
	Reference        *string          `json:"reference"`
	Date             string           `json:"date"`
	State            string           `json:"state"`
	Origin           *string          `json:"origin"`
	CreatedAt        time.Time        `json:"created_at"`
	UpdatedAt        time.Time        `json:"updated_at"`
	Version          int64            `json:"version"`
	Attachments      []attachmentJSON `json:"attachments"`
	Events           []any            `json:"events"`
	URI              string           `json:"uri"`
	Recognition      Recognition      `json:"recognition"`
}

func (s *server) toTypelessDocuments(r *http.Request, admin resource.ID, list []purchases.Typeless) ([]typelessDocumentJSON, error) {
	ids := make([]resource.ID, len(list))
	for i, t := range list {
		ids[i] = t.ContactID
	}
	byID, err := s.contactsOf(r, admin, ids)
	if err != nil {
		return nil, err
	}
	docIDs := make([]resource.ID, len(list))
	for i, t := range list {
		docIDs[i] = t.ID
	}
	recs, err := s.recognitionsOf(r, admin, docIDs)
	if err != nil {
		return nil, err
	}
	out := make([]typelessDocumentJSON, len(list))
	for i, t := range list {
		o := typelessDocumentJSON{
			ID: t.ID.String(), AdministrationID: t.AdministrationID.String(), Reference: nz(t.Reference), Date: t.Date.Format(time.DateOnly),
			State: t.State, Origin: nz(t.Origin), CreatedAt: t.CreatedAt, UpdatedAt: t.UpdatedAt, Version: t.Version(),
			Attachments: []attachmentJSON{}, Events: []any{}, URI: t.Ref().URI(), Recognition: recs[t.ID],
		}
		if t.ContactID != 0 {
			o.ContactID = ptr(t.ContactID.String())
		}
		if c := byID[t.ContactID]; c != nil {
			cj := s.toContact(*c)
			o.Contact = &cj
		}
		for _, a := range t.Attachments {
			o.Attachments = append(o.Attachments, toAttachment(a))
		}
		out[i] = o
	}
	return out, nil
}

func (s *server) writeTypelessDocument(w http.ResponseWriter, r *http.Request, admin resource.ID, status int, t purchases.Typeless, err error) {
	if err != nil {
		s.fail(w, r, err)
		return
	}
	out, err := s.toTypelessDocuments(r, admin, []purchases.Typeless{t})
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, status, out[0])
}

// ListTypelessDocuments implements ServerInterface.
func (s *server) ListTypelessDocuments(w http.ResponseWriter, r *http.Request, administrationID AdministrationId, params ListTypelessDocumentsParams) {
	admin, ok := parseID(w, administrationID)
	if !ok {
		return
	}
	q, ok := s.purchaseQuery(w, r, admin, params.Filter, params.NewFilter)
	if !ok {
		return
	}
	q.Page, q.PerPage = pageNumbers(params.Page, params.PerPage)
	q.Ascending = params.Sort != nil && *params.Sort == "date_asc"
	list, total, err := s.deps.Purchases.TypelessList(r.Context(), admin, user(r).ID, q)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	out, err := s.toTypelessDocuments(r, admin, list)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.writeLinks(w, r, max(q.Page, 1), cmpPerPage(q.PerPage, purchases.DefaultPerPage), total)
	writeJSON(w, http.StatusOK, out)
}

// CreateTypelessDocument implements ServerInterface.
func (s *server) CreateTypelessDocument(w http.ResponseWriter, r *http.Request, administrationID AdministrationId) {
	admin, ok := parseID(w, administrationID)
	if !ok {
		return
	}
	body, ok := decodeBody[struct {
		Document struct {
			Reference *string     `json:"reference"`
			Date      *string     `json:"date"`
			ContactID *flexString `json:"contact_id"`
		} `json:"typeless_document"`
	}](w, r)
	if !ok {
		return
	}
	c := purchases.TypelessChanges{Reference: body.Document.Reference, Date: body.Document.Date}
	if c.ContactID, ok = flexOptionalID(w, body.Document.ContactID); !ok {
		return
	}
	t, err := s.deps.Purchases.CreateTypeless(r.Context(), admin, user(r).ID, c)
	s.writeTypelessDocument(w, r, admin, http.StatusCreated, t, err)
}

// ListTypelessDocumentVersions implements ServerInterface.
func (s *server) ListTypelessDocumentVersions(w http.ResponseWriter, r *http.Request, administrationID AdministrationId, params ListTypelessDocumentVersionsParams) {
	admin, ok := parseID(w, administrationID)
	if !ok {
		return
	}
	q, ok := s.purchaseQuery(w, r, admin, params.Filter, nil)
	if !ok {
		return
	}
	list, err := s.deps.Purchases.TypelessVersions(r.Context(), admin, user(r).ID, q)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	out := make([]SynchronizationItem, len(list))
	for i, t := range list {
		out[i] = SynchronizationItem{Id: t.ID.String(), Version: int(t.Version())}
	}
	writeJSON(w, http.StatusOK, out)
}

// FetchTypelessDocuments implements ServerInterface.
func (s *server) FetchTypelessDocuments(w http.ResponseWriter, r *http.Request, administrationID AdministrationId) {
	admin, ok := parseID(w, administrationID)
	if !ok {
		return
	}
	ids, ok := readSyncIDs(w, r)
	if !ok {
		return
	}
	list, err := s.deps.Purchases.TypelessDocumentsByID(r.Context(), admin, user(r).ID, ids)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	out, err := s.toTypelessDocuments(r, admin, list)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

// GetTypelessDocument implements ServerInterface.
func (s *server) GetTypelessDocument(w http.ResponseWriter, r *http.Request, administrationID AdministrationId, id Id) {
	admin, rid, ok := parseIDs(w, administrationID, id)
	if !ok {
		return
	}
	t, err := s.deps.Purchases.TypelessDocument(r.Context(), admin, rid, user(r).ID)
	s.writeTypelessDocument(w, r, admin, http.StatusOK, t, err)
}

// DeleteTypelessDocument implements ServerInterface.
func (s *server) DeleteTypelessDocument(w http.ResponseWriter, r *http.Request, administrationID AdministrationId, id Id) {
	admin, rid, ok := parseIDs(w, administrationID, id)
	if !ok {
		return
	}
	if err := s.deps.Purchases.DeleteTypeless(r.Context(), admin, rid, user(r).ID); err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, http.StatusOK)
}

// CreateTypelessDocumentAttachment implements ServerInterface.
func (s *server) CreateTypelessDocumentAttachment(w http.ResponseWriter, r *http.Request, administrationID AdministrationId, id Id) {
	admin, rid, ok := parseIDs(w, administrationID, id)
	if !ok {
		return
	}
	name, contentType, data, ok := readUpload(w, r)
	if !ok {
		return
	}
	a, err := s.deps.Purchases.AddTypelessAttachment(r.Context(), admin, rid, user(r).ID, name, contentType, data)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, toAttachment(a))
}

// DeleteTypelessDocumentAttachment implements ServerInterface.
func (s *server) DeleteTypelessDocumentAttachment(w http.ResponseWriter, r *http.Request, administrationID AdministrationId, id Id, attachmentID Identifier) {
	admin, rid, ok := parseIDs(w, administrationID, id)
	if !ok {
		return
	}
	att, ok := parseID(w, attachmentID)
	if !ok {
		return
	}
	if err := s.deps.Purchases.DeleteTypelessAttachment(r.Context(), admin, rid, att, user(r).ID); err != nil {
		s.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// DownloadTypelessDocumentAttachment implements ServerInterface.
func (s *server) DownloadTypelessDocumentAttachment(w http.ResponseWriter, r *http.Request, administrationID AdministrationId, id Id, attachmentID Identifier) {
	s.redirectToAttachment(w, r, fileTypelessAttachment, administrationID, id, attachmentID)
}
