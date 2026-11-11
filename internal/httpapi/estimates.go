package httpapi

import (
	"errors"
	"maps"
	"net/http"
	"strings"
	"time"

	"github.com/adirkuhn/moneyharpy/internal/administration"
	"github.com/adirkuhn/moneyharpy/internal/contacts"
	"github.com/adirkuhn/moneyharpy/internal/resource"
	"github.com/adirkuhn/moneyharpy/internal/sales"
	"github.com/adirkuhn/moneyharpy/internal/webhooks"
)

// Kinds of estimate file a download link names.
const (
	fileEstimatePDF        = "estimate_pdf"
	fileEstimateAttachment = "estimate_attachment"
)

type estimateJSON struct {
	ID                      string                 `json:"id"`
	AdministrationID        string                 `json:"administration_id"`
	ContactID               string                 `json:"contact_id"`
	Contact                 *contactJSON           `json:"contact"`
	ContactPersonID         *string                `json:"contact_person_id"`
	ContactPerson           *contactPersonJSON     `json:"contact_person"`
	EstimateID              *string                `json:"estimate_id"`
	EstimateSequenceID      *string                `json:"estimate_sequence_id"`
	WorkflowID              string                 `json:"workflow_id"`
	DocumentStyleID         string                 `json:"document_style_id"`
	IdentityID              string                 `json:"identity_id"`
	DraftID                 *int                   `json:"draft_id"`
	State                   string                 `json:"state"`
	EstimateDate            *string                `json:"estimate_date"`
	DueDate                 string                 `json:"due_date"`
	Reference               *string                `json:"reference"`
	Language                string                 `json:"language"`
	Currency                string                 `json:"currency"`
	ExchangeRate            string                 `json:"exchange_rate"`
	Discount                string                 `json:"discount"`
	OriginalEstimateID      *string                `json:"original_estimate_id"`
	OriginalSalesInvoiceID  *string                `json:"original_sales_invoice_id"`
	ShowTax                 bool                   `json:"show_tax"`
	SignOnline              bool                   `json:"sign_online"`
	SentAt                  *string                `json:"sent_at"`
	AcceptedAt              *string                `json:"accepted_at"`
	RejectedAt              *string                `json:"rejected_at"`
	ArchivedAt              *string                `json:"archived_at"`
	CreatedAt               time.Time              `json:"created_at"`
	UpdatedAt               time.Time              `json:"updated_at"`
	PublicViewCode          *string                `json:"public_view_code"`
	PublicViewCodeExpiresAt *time.Time             `json:"public_view_code_expires_at"`
	Version                 int64                  `json:"version"`
	PreText                 *string                `json:"pre_text"`
	PostText                *string                `json:"post_text"`
	Details                 []salesDetailJSON      `json:"details"`
	PricesAreInclTax        bool                   `json:"prices_are_incl_tax"`
	TotalPriceExclTax       string                 `json:"total_price_excl_tax"`
	TotalPriceExclTaxBase   string                 `json:"total_price_excl_tax_base"`
	TotalPriceInclTax       string                 `json:"total_price_incl_tax"`
	TotalPriceInclTaxBase   string                 `json:"total_price_incl_tax_base"`
	TotalDiscount           string                 `json:"total_discount"`
	TotalTax                string                 `json:"total_tax"`
	URL                     string                 `json:"url"`
	CustomFields            []customFieldValueJSON `json:"custom_fields"`
	Notes                   []noteJSON             `json:"notes"`
	Attachments             []attachmentJSON       `json:"attachments"`
	Events                  []any                  `json:"events"`
	TaxTotals               []taxTotalJSON         `json:"tax_totals"`
	FirstDueInterval        int                    `json:"first_due_interval"`
	DeliveryMethod          *string                `json:"delivery_method"`
	URI                     string                 `json:"uri"`
}

func (s *server) toEstimate(e sales.Estimate, contact *contacts.Contact, today time.Time) estimateJSON {
	date := func(t *time.Time) *string {
		if t == nil {
			return nil
		}
		v := t.Format(time.DateOnly)
		return &v
	}
	due := e.DueDate
	if due == nil {
		from := today
		if e.EstimateDate != nil {
			from = *e.EstimateDate
		}
		d := from.AddDate(0, 0, e.FirstDueInterval)
		due = &d
	}
	// The details and totals are an invoice's; reuse their JSON.
	inv := s.toSalesInvoice(sales.Invoice{AdministrationID: e.AdministrationID, Lines: e.Lines, Totals: e.Totals, CustomFields: e.CustomFields,
		Notes: e.Notes, Attachments: e.Attachments}, nil, today)
	out := estimateJSON{
		ID: e.ID.String(), AdministrationID: e.AdministrationID.String(), ContactID: e.ContactID.String(), ContactPersonID: idString(e.ContactPersonID),
		EstimateID: nz(e.EstimateID), EstimateSequenceID: nz(e.SequenceID), WorkflowID: e.WorkflowID.String(), DocumentStyleID: e.DocumentStyleID.String(),
		IdentityID: e.IdentityID.String(), State: e.State, EstimateDate: date(e.EstimateDate), DueDate: due.Format(time.DateOnly),
		Reference: nz(e.Reference), Language: e.Language, Currency: e.Currency, ExchangeRate: "1.0", Discount: percentText(e.Discount),
		OriginalEstimateID: idString(e.OriginalEstimateID), OriginalSalesInvoiceID: idString(e.OriginalSalesInvoiceID), ShowTax: e.ShowTax,
		SignOnline: e.SignOnline, SentAt: date(e.SentAt), AcceptedAt: date(e.AcceptedAt), RejectedAt: date(e.RejectedAt), ArchivedAt: date(e.ArchivedAt),
		CreatedAt: e.CreatedAt, UpdatedAt: e.UpdatedAt, Version: e.Version(), PreText: nz(e.PreText), PostText: nz(e.PostText),
		Details: inv.Details, PricesAreInclTax: e.PricesAreInclTax, TotalPriceExclTax: e.Totals.ExclTax.String(),
		TotalPriceExclTaxBase: e.Totals.ExclTax.String(), TotalPriceInclTax: e.Totals.InclTax.String(), TotalPriceInclTaxBase: e.Totals.InclTax.String(),
		TotalDiscount: e.Totals.Discount.String(), TotalTax: e.Totals.Tax.String(), URL: strings.TrimSuffix(s.deps.BaseURL, "/") + e.Ref().URI(),
		CustomFields: inv.CustomFields, Notes: inv.Notes, Attachments: inv.Attachments, Events: []any{}, TaxTotals: inv.TaxTotals,
		FirstDueInterval: e.FirstDueInterval, URI: e.Ref().URI(),
	}
	if e.DraftID > 0 && e.EstimateID == "" {
		out.DraftID = &e.DraftID
	}
	if e.Delivery != nil {
		out.DeliveryMethod = &e.Delivery.Method
	}
	if contact != nil {
		c := s.toContact(*contact)
		out.Contact = &c
		if e.ContactPersonID != nil {
			for _, p := range contact.People {
				if p.ID == *e.ContactPersonID {
					cp := toContactPerson(p)
					out.ContactPerson = &cp
				}
			}
		}
	}
	return out
}

// toEstimates answers with estimates and their contacts, loaded at once.
func (s *server) toEstimates(r *http.Request, admin resource.ID, list []sales.Estimate) ([]estimateJSON, error) {
	ids := make([]resource.ID, 0, len(list))
	for _, e := range list {
		ids = append(ids, e.ContactID)
	}
	byID := map[resource.ID]*contacts.Contact{}
	for start := 0; start < len(ids); start += contacts.MaxFetch {
		found, err := s.deps.Contacts.ContactsByID(r.Context(), admin, user(r).ID, ids[start:min(start+contacts.MaxFetch, len(ids))])
		if err != nil {
			return nil, err
		}
		for i := range found {
			byID[found[i].ID] = &found[i]
		}
	}
	today, err := s.today(r, admin)
	if err != nil {
		return nil, err
	}
	out := make([]estimateJSON, len(list))
	for i, e := range list {
		out[i] = s.toEstimate(e, byID[e.ContactID], today)
	}
	return out, nil
}

func (s *server) writeEstimate(w http.ResponseWriter, r *http.Request, admin resource.ID, status int, e sales.Estimate) {
	out, err := s.toEstimates(r, admin, []sales.Estimate{e})
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, status, out[0])
}

// failEstimate answers estimate errors as Moneybird does, and others as
// any request.
func (s *server) failEstimate(w http.ResponseWriter, r *http.Request, err error) {
	symbolic := func(status int, message string, symbols map[string]any) {
		writeJSON(w, status, Error{Error: message, Symbolic: &symbols})
	}
	base := func(message string) {
		writeJSON(w, http.StatusUnprocessableEntity, validationBody(map[string][]string{"base": {message}}))
	}
	var serr *sales.StateError
	switch {
	case errors.As(err, &serr) && serr.From == "":
		symbolic(http.StatusBadRequest, "State isn't an option", map[string]any{"state": serr.Symbol()})
	case errors.As(err, &serr):
		symbolic(http.StatusBadRequest, "State is invalid", map[string]any{"state": serr.Symbol()})
	case errors.Is(err, sales.ErrEstimateNoDetails):
		symbolic(http.StatusBadRequest, "details cannot be empty", map[string]any{"estimate": "details_cannot_be_empty"})
	case errors.Is(err, sales.ErrEstimateNotBillable):
		symbolic(http.StatusBadRequest, "You cannot invoice this quote. Check the state of the quote and whether the contact still exists.", map[string]any{"id": "state"})
	case errors.Is(err, sales.ErrEstimateNotEditable):
		base("This estimate cannot be changed in its state; open it again first")
	case errors.Is(err, sales.ErrEstimateNotSigned):
		base("The estimate has not been signed online")
	case errors.Is(err, sales.ErrEstimateDelivery):
		base("The estimate is sent but could not be emailed; check the mail settings and send it again")
	default:
		s.fail(w, r, err)
	}
}

type estimateBody struct {
	Estimate struct {
		ContactID              *flexString   `json:"contact_id"`
		ContactPersonID        *flexString   `json:"contact_person_id"`
		UpdateContact          *flexBool     `json:"update_contact"`
		DocumentStyleID        *flexString   `json:"document_style_id"`
		WorkflowID             *flexString   `json:"workflow_id"`
		IdentityID             *flexString   `json:"identity_id"`
		Reference              *string       `json:"reference"`
		EstimateDate           *string       `json:"estimate_date"`
		SequenceID             *flexString   `json:"estimate_sequence_id"`
		RemoveSequence         *flexBool     `json:"remove_estimate_sequence_id"`
		Language               *string       `json:"language"`
		Currency               *string       `json:"currency"`
		PricesAreInclTax       *flexBool     `json:"prices_are_incl_tax"`
		ShowTax                *flexBool     `json:"show_tax"`
		FirstDueInterval       *flexString   `json:"first_due_interval"`
		PreText                *string       `json:"pre_text"`
		PostText               *string       `json:"post_text"`
		Discount               *flexString   `json:"discount"`
		OriginalSalesInvoiceID *flexString   `json:"original_sales_invoice_id"`
		Details                *salesDetails `json:"details_attributes"`
		CustomFields           *fieldValues  `json:"custom_fields_attributes"`
	} `json:"estimate"`
}

func toEstimateChanges(w http.ResponseWriter, body estimateBody) (sales.EstimateChanges, bool) {
	in := body.Estimate
	c := sales.EstimateChanges{
		Reference: in.Reference, EstimateDate: in.EstimateDate, SequenceID: in.SequenceID.ptr(), Language: in.Language, Currency: in.Currency,
		PricesAreInclTax: in.PricesAreInclTax.ptr(), ShowTax: in.ShowTax.ptr(), PreText: in.PreText, PostText: in.PostText, Discount: in.Discount.ptr(),
	}
	if deref(in.RemoveSequence.ptr()) {
		c.SequenceID = ptr("")
	}
	var ok bool
	for _, f := range []struct {
		src *flexString
		dst **resource.ID
	}{
		{in.ContactID, &c.ContactID}, {in.ContactPersonID, &c.ContactPersonID}, {in.DocumentStyleID, &c.DocumentStyleID}, {in.WorkflowID, &c.WorkflowID},
		{in.IdentityID, &c.IdentityID}, {in.OriginalSalesInvoiceID, &c.OriginalSalesInvoiceID},
	} {
		if *f.dst, ok = flexOptionalID(w, f.src); !ok {
			return c, false
		}
	}
	if c.FirstDueInterval, ok = flexInt(w, in.FirstDueInterval, "first_due_interval"); !ok {
		return c, false
	}
	if c.Details, ok = toDetailInputs(w, in.Details); !ok {
		return c, false
	}
	if in.CustomFields != nil {
		c.CustomFields = []sales.FieldValue{}
		for _, v := range *in.CustomFields {
			id, ok := flexID(w, &v.ID)
			if !ok {
				return c, false
			}
			c.CustomFields = append(c.CustomFields, sales.FieldValue{FieldID: *id, Value: v.Value})
		}
	}
	return c, true
}

func (s *server) estimateQuery(w http.ResponseWriter, r *http.Request, admin resource.ID, filter *DocumentFilter) (sales.EstimateQuery, bool) {
	now, err := s.today(r, admin)
	if err != nil {
		s.fail(w, r, err)
		return sales.EstimateQuery{}, false
	}
	q, err := sales.ParseEstimateFilter(deref((*string)(filter)), now)
	if err != nil {
		writeError(w, http.StatusBadRequest, filterProblem(err))
		return q, false
	}
	return q, true
}

// ListEstimates implements ServerInterface.
func (s *server) ListEstimates(w http.ResponseWriter, r *http.Request, administrationID AdministrationId, params ListEstimatesParams) {
	admin, ok := parseID(w, administrationID)
	if !ok {
		return
	}
	q, ok := s.estimateQuery(w, r, admin, params.Filter)
	if !ok {
		return
	}
	q.Page, q.PerPage = pageNumbers(params.Page, params.PerPage)
	list, total, err := s.deps.Sales.Estimates(r.Context(), admin, user(r).ID, q)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	out, err := s.toEstimates(r, admin, list)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.writeLinks(w, r, max(q.Page, 1), cmpPerPage(q.PerPage, sales.DefaultPerPage), total)
	writeJSON(w, http.StatusOK, out)
}

// CreateEstimate implements ServerInterface.
func (s *server) CreateEstimate(w http.ResponseWriter, r *http.Request, administrationID AdministrationId) {
	admin, ok := parseID(w, administrationID)
	if !ok {
		return
	}
	body, ok := decodeBody[estimateBody](w, r)
	if !ok {
		return
	}
	c, ok := toEstimateChanges(w, body)
	if !ok {
		return
	}
	e, err := s.deps.Sales.CreateEstimate(r.Context(), admin, user(r).ID, c)
	if err != nil {
		s.failEstimate(w, r, err)
		return
	}
	s.writeEstimate(w, r, admin, http.StatusCreated, e)
}

// ListEstimateVersions implements ServerInterface.
func (s *server) ListEstimateVersions(w http.ResponseWriter, r *http.Request, administrationID AdministrationId, params ListEstimateVersionsParams) {
	admin, ok := parseID(w, administrationID)
	if !ok {
		return
	}
	q, ok := s.estimateQuery(w, r, admin, params.Filter)
	if !ok {
		return
	}
	list, err := s.deps.Sales.EstimateVersions(r.Context(), admin, user(r).ID, q)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	out := make([]SynchronizationItem, len(list))
	for i, e := range list {
		out[i] = SynchronizationItem{Id: e.ID.String(), Version: int(e.Version())}
	}
	writeJSON(w, http.StatusOK, out)
}

// FetchEstimates implements ServerInterface.
func (s *server) FetchEstimates(w http.ResponseWriter, r *http.Request, administrationID AdministrationId) {
	admin, ok := parseID(w, administrationID)
	if !ok {
		return
	}
	ids, ok := readSyncIDs(w, r)
	if !ok {
		return
	}
	list, err := s.deps.Sales.EstimatesByID(r.Context(), admin, user(r).ID, ids)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	out, err := s.toEstimates(r, admin, list)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

// GetEstimateByEstimateId implements ServerInterface.
func (s *server) GetEstimateByEstimateId(w http.ResponseWriter, r *http.Request, administrationID AdministrationId, number string) {
	admin, ok := parseID(w, administrationID)
	if !ok {
		return
	}
	e, err := s.deps.Sales.EstimateByNumber(r.Context(), admin, user(r).ID, number)
	if sales.IsNotFound(err) {
		writeJSON(w, http.StatusNotFound, Error{Error: "Estimate with estimate_id: " + number + " not found", Symbolic: &map[string]any{"estimate_id": "not_found"}})
		return
	}
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.writeEstimate(w, r, admin, http.StatusOK, e)
}

// GetEstimate implements ServerInterface.
func (s *server) GetEstimate(w http.ResponseWriter, r *http.Request, administrationID AdministrationId, id Id, _ GetEstimateParams) {
	admin, rid, ok := parseIDs(w, administrationID, id)
	if !ok {
		return
	}
	e, err := s.deps.Sales.Estimate(r.Context(), admin, rid, user(r).ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.writeEstimate(w, r, admin, http.StatusOK, e)
}

// UpdateEstimate implements ServerInterface.
func (s *server) UpdateEstimate(w http.ResponseWriter, r *http.Request, administrationID AdministrationId, id Id) {
	admin, rid, ok := parseIDs(w, administrationID, id)
	if !ok {
		return
	}
	body, ok := decodeBody[estimateBody](w, r)
	if !ok {
		return
	}
	c, ok := toEstimateChanges(w, body)
	if !ok {
		return
	}
	e, err := s.deps.Sales.UpdateEstimate(r.Context(), admin, rid, user(r).ID, c)
	if err != nil {
		s.failEstimate(w, r, err)
		return
	}
	s.writeEstimate(w, r, admin, http.StatusOK, e)
}

// DeleteEstimate implements ServerInterface.
func (s *server) DeleteEstimate(w http.ResponseWriter, r *http.Request, administrationID AdministrationId, id Id) {
	admin, rid, ok := parseIDs(w, administrationID, id)
	if !ok {
		return
	}
	if err := s.deps.Sales.DeleteEstimate(r.Context(), admin, rid, user(r).ID); err != nil {
		s.failEstimate(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// SendEstimate implements ServerInterface.
func (s *server) SendEstimate(w http.ResponseWriter, r *http.Request, administrationID AdministrationId, id Id) {
	admin, rid, ok := parseIDs(w, administrationID, id)
	if !ok {
		return
	}
	body, ok := decodeBody[struct {
		Sending struct {
			DeliveryMethod string    `json:"delivery_method"`
			EmailAddress   string    `json:"email_address"`
			EmailMessage   string    `json:"email_message"`
			SignOnline     *flexBool `json:"sign_online"`
		} `json:"estimate_sending"`
	}](w, r)
	if !ok {
		return
	}
	in := body.Sending
	e, err := s.deps.Sales.SendEstimate(r.Context(), admin, rid, user(r).ID, sales.EstimateSendInput{
		DeliveryMethod: in.DeliveryMethod, EmailAddress: in.EmailAddress, EmailMessage: in.EmailMessage, SignOnline: in.SignOnline.ptr(),
	})
	if err != nil {
		s.failEstimate(w, r, err)
		return
	}
	s.writeEstimate(w, r, admin, http.StatusOK, e)
}

// ChangeEstimateState implements ServerInterface.
func (s *server) ChangeEstimateState(w http.ResponseWriter, r *http.Request, administrationID AdministrationId, id Id) {
	admin, rid, ok := parseIDs(w, administrationID, id)
	if !ok {
		return
	}
	body, ok := decodeBody[ChangeEstimateStateRequest](w, r)
	if !ok {
		return
	}
	e, err := s.deps.Sales.ChangeEstimateState(r.Context(), admin, rid, user(r).ID, body.State)
	if err != nil {
		s.failEstimate(w, r, err)
		return
	}
	s.writeEstimate(w, r, admin, http.StatusOK, e)
}

// BillEstimate implements ServerInterface: billing makes a sales invoice,
// so it needs that permission too, as at Moneybird.
func (s *server) BillEstimate(w http.ResponseWriter, r *http.Request, administrationID AdministrationId, id Id) {
	admin, rid, ok := parseIDs(w, administrationID, id)
	if !ok || !s.requireAlso(w, r, admin, administration.SalesInvoices) {
		return
	}
	inv, err := s.deps.Sales.BillEstimate(r.Context(), admin, rid, user(r).ID)
	if err != nil {
		s.failEstimate(w, r, err)
		return
	}
	s.writeSalesInvoice(w, r, admin, http.StatusOK, inv)
}

// DownloadEstimatePdf implements ServerInterface: it redirects to a
// temporary link to the PDF, as Moneybird does.
func (s *server) DownloadEstimatePdf(w http.ResponseWriter, r *http.Request, administrationID AdministrationId, id Id, params DownloadEstimatePdfParams) {
	admin, rid, ok := parseIDs(w, administrationID, id)
	if !ok {
		return
	}
	if _, err := s.deps.Sales.Estimate(r.Context(), admin, rid, user(r).ID); err != nil {
		s.fail(w, r, err)
		return
	}
	s.redirectToFile(w, fileLink{Kind: fileEstimatePDF, Admin: admin, ID: rid, User: user(r).ID, Stationery: deref(params.Media) == "stationery"})
}

// DownloadSignedEstimatePdf implements ServerInterface. No estimate is
// signed online yet, so it answers Moneybird's error for that.
func (s *server) DownloadSignedEstimatePdf(w http.ResponseWriter, r *http.Request, administrationID AdministrationId, id Id) {
	admin, rid, ok := parseIDs(w, administrationID, id)
	if !ok {
		return
	}
	_, _, err := s.deps.Sales.SignedEstimatePDF(r.Context(), admin, rid, user(r).ID)
	if err == nil {
		err = sales.ErrEstimateNotSigned
	}
	s.failEstimate(w, r, err)
}

// CreateEstimateNote implements ServerInterface.
func (s *server) CreateEstimateNote(w http.ResponseWriter, r *http.Request, administrationID AdministrationId, id Id) {
	admin, rid, ok := parseIDs(w, administrationID, id)
	if !ok {
		return
	}
	in, ok := decodeNote(w, r)
	if !ok {
		return
	}
	n, err := s.deps.Sales.AddEstimateNote(r.Context(), admin, rid, user(r).ID, in)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, toNote(n))
}

// DeleteEstimateNote implements ServerInterface.
func (s *server) DeleteEstimateNote(w http.ResponseWriter, r *http.Request, administrationID AdministrationId, id Id, noteID Identifier) {
	admin, rid, ok := parseIDs(w, administrationID, id)
	if !ok {
		return
	}
	note, ok := parseID(w, noteID)
	if !ok {
		return
	}
	if err := s.deps.Sales.DeleteEstimateNote(r.Context(), admin, rid, note, user(r).ID); err != nil {
		s.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// CreateEstimateAttachment implements ServerInterface.
func (s *server) CreateEstimateAttachment(w http.ResponseWriter, r *http.Request, administrationID AdministrationId, id Id) {
	admin, rid, ok := parseIDs(w, administrationID, id)
	if !ok {
		return
	}
	name, contentType, data, ok := readUpload(w, r)
	if !ok {
		return
	}
	a, err := s.deps.Sales.AddEstimateAttachment(r.Context(), admin, rid, user(r).ID, name, contentType, data)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, toAttachment(a))
}

// DeleteEstimateAttachment implements ServerInterface.
func (s *server) DeleteEstimateAttachment(w http.ResponseWriter, r *http.Request, administrationID AdministrationId, id Id, attachmentID Identifier) {
	admin, rid, ok := parseIDs(w, administrationID, id)
	if !ok {
		return
	}
	att, ok := parseID(w, attachmentID)
	if !ok {
		return
	}
	if err := s.deps.Sales.DeleteEstimateAttachment(r.Context(), admin, rid, att, user(r).ID); err != nil {
		s.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// DownloadEstimateAttachment implements ServerInterface.
func (s *server) DownloadEstimateAttachment(w http.ResponseWriter, r *http.Request, administrationID AdministrationId, id Id, attachmentID Identifier) {
	s.redirectToAttachment(w, r, fileEstimateAttachment, administrationID, id, attachmentID)
}

// estimateEvents are Moneybird's webhook events of estimate operations.
var estimateEvents = map[string]func(c change) []webhooks.Event{
	"createEstimate": func(c change) []webhooks.Event {
		events := c.one(sales.EstimateEntityType, "estimate_created")
		if id, _ := c.responseMap["original_sales_invoice_id"].(string); id != "" {
			events = append(events, c.one(sales.EstimateEntityType, "estimate_created_from_original_invoice")...)
		}
		return events
	},
	"updateEstimate": func(c change) []webhooks.Event { return c.one(sales.EstimateEntityType, "estimate_updated") },
	"deleteEstimate": func(c change) []webhooks.Event { return c.one(sales.EstimateEntityType, "estimate_destroyed") },
	"sendEstimate": func(c change) []webhooks.Event {
		if c.responseMap["delivery_method"] == "Manual" {
			return c.one(sales.EstimateEntityType, "estimate_send_manually")
		}
		return c.one(sales.EstimateEntityType, "estimate_send_email")
	},
	"changeEstimateState": func(c change) []webhooks.Event {
		state, _ := c.request["state"].(string)
		return c.one(sales.EstimateEntityType, "estimate_mark_"+state)
	},
	"billEstimate": func(c change) []webhooks.Event {
		id := c.params["id"]
		billed := webhooks.Event{EntityType: sales.EstimateEntityType, EntityID: id, Action: "estimate_billed", State: sales.Billed,
			Entity: []byte(`{"id":"` + id + `","state":"billed"}`)}
		return append([]webhooks.Event{billed}, c.one("SalesInvoice", "sales_invoice_created_based_on_estimate")...)
	},
}

func init() { maps.Copy(webhookEvents, estimateEvents) }
