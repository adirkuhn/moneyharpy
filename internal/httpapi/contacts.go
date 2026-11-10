package httpapi

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/adirkuhn/moneyharpy/internal/contacts"
	"github.com/adirkuhn/moneyharpy/internal/resource"
)

// flexBool takes a JSON boolean or the strings "true" and "false", as
// Moneybird accepts both.
type flexBool bool

// UnmarshalJSON implements json.Unmarshaler.
func (f *flexBool) UnmarshalJSON(b []byte) error {
	var v bool
	if err := json.Unmarshal(b, &v); err == nil {
		*f = flexBool(v)
		return nil
	}
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return fmt.Errorf("want a boolean: %w", err)
	}
	v, err := strconv.ParseBool(s)
	if err != nil {
		return fmt.Errorf("want a boolean: %w", err)
	}
	*f = flexBool(v)
	return nil
}

func (f *flexBool) ptr() *bool {
	if f == nil {
		return nil
	}
	v := bool(*f)
	return &v
}

// flexOptionalID parses an id that may be empty to clear it.
func flexOptionalID(w http.ResponseWriter, f *flexString) (*resource.ID, bool) {
	if f == nil {
		return nil, true
	}
	if *f == "" || *f == "0" {
		zero := resource.ID(0)
		return &zero, true
	}
	return flexID(w, f)
}

// fieldValues reads custom_fields_attributes, which Moneybird takes as an
// array or as an object keyed by position.
type fieldValues []struct {
	ID    flexString `json:"id"`
	Value string     `json:"value"`
}

// UnmarshalJSON implements json.Unmarshaler.
func (f *fieldValues) UnmarshalJSON(b []byte) error {
	type plain fieldValues
	if bytes.HasPrefix(bytes.TrimSpace(b), []byte("[")) {
		return json.Unmarshal(b, (*plain)(f))
	}
	var keyed map[string]json.RawMessage
	if err := json.Unmarshal(b, &keyed); err != nil {
		return err
	}
	keys := make([]string, 0, len(keyed))
	for k := range keyed {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		a, errA := strconv.Atoi(keys[i])
		c, errC := strconv.Atoi(keys[j])
		if errA == nil && errC == nil {
			return a < c
		}
		return keys[i] < keys[j]
	})
	for _, k := range keys {
		var v struct {
			ID    flexString `json:"id"`
			Value string     `json:"value"`
		}
		if err := json.Unmarshal(keyed[k], &v); err != nil {
			return err
		}
		*f = append(*f, v)
	}
	return nil
}

type personBody struct {
	Firstname  *string `json:"firstname"`
	Lastname   *string `json:"lastname"`
	Phone      *string `json:"phone"`
	Email      *string `json:"email"`
	Department *string `json:"department"`
}

func (p personBody) changes() contacts.PersonChanges {
	return contacts.PersonChanges{Firstname: p.Firstname, Lastname: p.Lastname, Phone: p.Phone, Email: p.Email, Department: p.Department}
}

type contactBody struct {
	Contact struct {
		CompanyName              *string      `json:"company_name"`
		Firstname                *string      `json:"firstname"`
		Lastname                 *string      `json:"lastname"`
		Address1                 *string      `json:"address1"`
		Address2                 *string      `json:"address2"`
		Zipcode                  *string      `json:"zipcode"`
		City                     *string      `json:"city"`
		Country                  *string      `json:"country"`
		Phone                    *string      `json:"phone"`
		DeliveryMethod           *string      `json:"delivery_method"`
		CustomerID               *string      `json:"customer_id"`
		TaxNumber                *string      `json:"tax_number"`
		ChamberOfCommerce        *string      `json:"chamber_of_commerce"`
		BankAccount              *string      `json:"bank_account"`
		SendInvoicesToAttention  *string      `json:"send_invoices_to_attention"`
		SendInvoicesToEmail      *string      `json:"send_invoices_to_email"`
		SendEstimatesToAttention *string      `json:"send_estimates_to_attention"`
		SendEstimatesToEmail     *string      `json:"send_estimates_to_email"`
		EmailUBL                 *flexBool    `json:"email_ubl"`
		DirectDebit              *flexBool    `json:"direct_debit"`
		SepaActive               *flexBool    `json:"sepa_active"`
		SepaIBAN                 *string      `json:"sepa_iban"`
		SepaIBANAccountName      *string      `json:"sepa_iban_account_name"`
		SepaBIC                  *string      `json:"sepa_bic"`
		SepaMandateID            *string      `json:"sepa_mandate_id"`
		SepaMandateDate          *string      `json:"sepa_mandate_date"`
		SepaSequenceType         *string      `json:"sepa_sequence_type"`
		SIIdentifierType         *string      `json:"si_identifier_type"`
		SIIdentifier             *string      `json:"si_identifier"`
		InvoiceWorkflowID        *flexString  `json:"invoice_workflow_id"`
		EstimateWorkflowID       *flexString  `json:"estimate_workflow_id"`
		Archived                 *bool        `json:"archived"`
		CustomFields             *fieldValues `json:"custom_fields_attributes"`
		ContactPerson            *personBody  `json:"contact_person"`
	} `json:"contact"`
}

func toContactChanges(w http.ResponseWriter, body contactBody) (contacts.Changes, bool) {
	in := body.Contact
	c := contacts.Changes{
		CompanyName: in.CompanyName, Firstname: in.Firstname, Lastname: in.Lastname, Address1: in.Address1, Address2: in.Address2, Zipcode: in.Zipcode,
		City: in.City, Country: in.Country, Phone: in.Phone, DeliveryMethod: in.DeliveryMethod, CustomerID: in.CustomerID, TaxNumber: in.TaxNumber,
		ChamberOfCommerce: in.ChamberOfCommerce, BankAccount: in.BankAccount, SendInvoicesToAttention: in.SendInvoicesToAttention,
		SendInvoicesToEmail: in.SendInvoicesToEmail, SendEstimatesToAttention: in.SendEstimatesToAttention, SendEstimatesToEmail: in.SendEstimatesToEmail,
		EmailUBL: in.EmailUBL.ptr(), DirectDebit: in.DirectDebit.ptr(), SepaActive: in.SepaActive.ptr(), SepaIBAN: in.SepaIBAN,
		SepaIBANAccountName: in.SepaIBANAccountName, SepaBIC: in.SepaBIC, SepaMandateID: in.SepaMandateID, SepaMandateDate: in.SepaMandateDate,
		SepaSequenceType: in.SepaSequenceType, SIIdentifierType: in.SIIdentifierType, SIIdentifier: in.SIIdentifier, Archived: in.Archived,
	}
	var ok bool
	if c.InvoiceWorkflowID, ok = flexOptionalID(w, in.InvoiceWorkflowID); !ok {
		return c, false
	}
	if c.EstimateWorkflowID, ok = flexOptionalID(w, in.EstimateWorkflowID); !ok {
		return c, false
	}
	if in.CustomFields != nil {
		c.CustomFields = []contacts.FieldValue{}
		for _, v := range *in.CustomFields {
			id, ok := flexID(w, &v.ID)
			if !ok {
				return c, false
			}
			c.CustomFields = append(c.CustomFields, contacts.FieldValue{FieldID: *id, Value: v.Value})
		}
	}
	if in.ContactPerson != nil {
		p := in.ContactPerson.changes()
		c.Person = &p
	}
	return c, true
}

// nz returns nil for an empty string, as Moneybird sends null for fields
// left blank.
func nz(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func idString(id *resource.ID) *string {
	if id == nil {
		return nil
	}
	s := id.String()
	return &s
}

type contactPersonJSON struct {
	ID               string    `json:"id"`
	ContactID        string    `json:"contact_id"`
	AdministrationID string    `json:"administration_id"`
	Firstname        *string   `json:"firstname"`
	Lastname         *string   `json:"lastname"`
	Phone            *string   `json:"phone"`
	Email            *string   `json:"email"`
	Department       *string   `json:"department"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
	Version          int64     `json:"version"`
}

func toContactPerson(p contacts.Person) contactPersonJSON {
	return contactPersonJSON{
		ID: p.ID.String(), ContactID: p.ContactID.String(), AdministrationID: p.AdministrationID.String(), Firstname: nz(p.Firstname), Lastname: nz(p.Lastname),
		Phone: nz(p.Phone), Email: nz(p.Email), Department: nz(p.Department), CreatedAt: p.CreatedAt, UpdatedAt: p.UpdatedAt, Version: p.Version(),
	}
}

type noteJSON struct {
	ID               string     `json:"id"`
	AdministrationID string     `json:"administration_id"`
	EntityID         string     `json:"entity_id"`
	EntityType       string     `json:"entity_type"`
	UserID           *string    `json:"user_id,omitempty"`
	AssigneeID       *string    `json:"assignee_id"`
	Todo             bool       `json:"todo"`
	Note             string     `json:"note"`
	CompletedAt      *time.Time `json:"completed_at"`
	CompletedByID    *string    `json:"completed_by_id"`
	TodoType         *string    `json:"todo_type"`
	Data             any        `json:"data"`
	CreatedAt        time.Time  `json:"created_at"`
	UpdatedAt        time.Time  `json:"updated_at"`
}

func toNote(n contacts.Note) noteJSON {
	out := noteJSON{
		ID: n.ID.String(), AdministrationID: n.AdministrationID.String(), EntityID: n.EntityID.String(), EntityType: n.EntityType,
		AssigneeID: idString(n.AssigneeID), Todo: n.Todo, Note: n.Note, CompletedAt: n.CompletedAt, CompletedByID: idString(n.CompletedByID),
		CreatedAt: n.CreatedAt, UpdatedAt: n.UpdatedAt,
	}
	if n.UserID != 0 {
		out.UserID = ptr(n.UserID.String())
	}
	return out
}

type customFieldValueJSON struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Value string `json:"value"`
}

type contactJSON struct {
	ID                       string     `json:"id"`
	AdministrationID         string     `json:"administration_id"`
	CompanyName              string     `json:"company_name"`
	Firstname                *string    `json:"firstname"`
	Lastname                 *string    `json:"lastname"`
	Address1                 *string    `json:"address1"`
	Address2                 *string    `json:"address2"`
	Zipcode                  *string    `json:"zipcode"`
	City                     *string    `json:"city"`
	Country                  string     `json:"country"`
	Phone                    *string    `json:"phone"`
	DeliveryMethod           string     `json:"delivery_method"`
	CustomerID               string     `json:"customer_id"`
	TaxNumber                *string    `json:"tax_number"`
	ChamberOfCommerce        *string    `json:"chamber_of_commerce"`
	BankAccount              *string    `json:"bank_account"`
	IsTrusted                bool       `json:"is_trusted"`
	MaxTransferAmount        *string    `json:"max_transfer_amount"`
	Attention                *string    `json:"attention"`
	Email                    *string    `json:"email"`
	EmailUBL                 bool       `json:"email_ubl"`
	SendInvoicesToAttention  *string    `json:"send_invoices_to_attention"`
	SendInvoicesToEmail      *string    `json:"send_invoices_to_email"`
	SendEstimatesToAttention *string    `json:"send_estimates_to_attention"`
	SendEstimatesToEmail     *string    `json:"send_estimates_to_email"`
	DirectDebit              bool       `json:"direct_debit"`
	SepaActive               bool       `json:"sepa_active"`
	SepaIBAN                 *string    `json:"sepa_iban"`
	SepaIBANAccountName      *string    `json:"sepa_iban_account_name"`
	SepaBIC                  *string    `json:"sepa_bic"`
	SepaMandateID            *string    `json:"sepa_mandate_id"`
	SepaMandateDate          *string    `json:"sepa_mandate_date"`
	SepaSequenceType         string     `json:"sepa_sequence_type"`
	CreditCardNumber         *string    `json:"credit_card_number"`
	CreditCardReference      *string    `json:"credit_card_reference"`
	CreditCardType           *string    `json:"credit_card_type"`
	TaxNumberValidatedAt     *time.Time `json:"tax_number_validated_at"`
	TaxNumberValid           *bool      `json:"tax_number_valid"`
	InvoiceWorkflowID        *string    `json:"invoice_workflow_id"`
	EstimateWorkflowID       *string    `json:"estimate_workflow_id"`
	SIIdentifier             *string    `json:"si_identifier"`
	SIIdentifierType         *string    `json:"si_identifier_type"`
	MoneybirdPaymentsMandate bool       `json:"moneybird_payments_mandate"`
	CreatedAt                time.Time  `json:"created_at"`
	UpdatedAt                time.Time  `json:"updated_at"`
	Version                  int64      `json:"version"`
	// SalesInvoicesURL is Moneybird's public invoice portal; it stays out
	// until MoneyHarpy has one.
	SalesInvoicesURL string                 `json:"sales_invoices_url,omitempty"`
	Notes            []noteJSON             `json:"notes"`
	CustomFields     []customFieldValueJSON `json:"custom_fields"`
	ContactPeople    []contactPersonJSON    `json:"contact_people"`
	Archived         bool                   `json:"archived"`
	Events           []any                  `json:"events"`
	URI              string                 `json:"uri"`
}

func (s *server) toContact(c contacts.Contact) contactJSON {
	out := contactJSON{
		ID: c.ID.String(), AdministrationID: c.AdministrationID.String(), CompanyName: c.CompanyName, Firstname: nz(c.Firstname), Lastname: nz(c.Lastname),
		Address1: nz(c.Address1), Address2: nz(c.Address2), Zipcode: nz(c.Zipcode), City: nz(c.City), Country: c.Country, Phone: nz(c.Phone),
		DeliveryMethod: c.DeliveryMethod, CustomerID: c.CustomerID, TaxNumber: nz(c.TaxNumber), ChamberOfCommerce: nz(c.ChamberOfCommerce),
		BankAccount: nz(c.BankAccount), Attention: nz(c.SendInvoicesToAttention), Email: nz(c.SendInvoicesToEmail), EmailUBL: c.EmailUBL,
		SendInvoicesToAttention: nz(c.SendInvoicesToAttention), SendInvoicesToEmail: nz(c.SendInvoicesToEmail),
		SendEstimatesToAttention: nz(c.SendEstimatesToAttention), SendEstimatesToEmail: nz(c.SendEstimatesToEmail), DirectDebit: c.DirectDebit,
		SepaActive: c.SepaActive, SepaIBAN: nz(c.SepaIBAN), SepaIBANAccountName: nz(c.SepaIBANAccountName), SepaBIC: nz(c.SepaBIC),
		SepaMandateID: nz(c.SepaMandateID), SepaMandateDate: dateString(c.SepaMandateDate), SepaSequenceType: c.SepaSequenceType,
		InvoiceWorkflowID: idString(c.InvoiceWorkflowID), EstimateWorkflowID: idString(c.EstimateWorkflowID), SIIdentifier: nz(c.SIIdentifier),
		SIIdentifierType: nz(c.SIIdentifierType), CreatedAt: c.CreatedAt, UpdatedAt: c.UpdatedAt, Version: c.Version(),
		Notes: []noteJSON{}, CustomFields: []customFieldValueJSON{}, ContactPeople: []contactPersonJSON{}, Archived: c.Archived, Events: []any{},
		URI: c.Ref().URI(),
	}
	for _, n := range c.Notes {
		out.Notes = append(out.Notes, toNote(n))
	}
	for _, f := range c.CustomFields {
		out.CustomFields = append(out.CustomFields, customFieldValueJSON{ID: f.FieldID.String(), Name: f.Name, Value: f.Value})
	}
	for _, p := range c.People {
		out.ContactPeople = append(out.ContactPeople, toContactPerson(p))
	}
	return out
}

func (s *server) toContacts(list []contacts.Contact) []contactJSON {
	out := make([]contactJSON, len(list))
	for i, c := range list {
		out[i] = s.toContact(c)
	}
	return out
}

// pageNumbers reads page and per_page; the contract checked their shape.
func pageNumbers(page *Page, perPage *PerPage) (int, int) {
	p, _ := strconv.Atoi(deref((*string)(page)))
	n, _ := strconv.Atoi(deref((*string)(perPage)))
	return p, n
}

// writePage answers with one page of contacts and Moneybird's Link header.
func (s *server) writePage(w http.ResponseWriter, r *http.Request, page contacts.Page) {
	s.writeLinks(w, r, page.Page, page.PerPage, page.Total)
	writeJSON(w, http.StatusOK, s.toContacts(page.Contacts))
}

// writeLinks sets Moneybird's Link header to the first, previous, next and
// last pages, and X-Total-Count to the number of records in all.
func (s *server) writeLinks(w http.ResponseWriter, r *http.Request, page, perPage, total int) {
	last := max((total+perPage-1)/perPage, 1)
	link := func(n int, rel string) string {
		q := r.URL.Query()
		q.Set("page", strconv.Itoa(n))
		q.Set("per_page", strconv.Itoa(perPage))
		u := url.URL{Path: r.URL.Path, RawQuery: q.Encode()}
		return fmt.Sprintf("<%s%s>; rel=%q", strings.TrimSuffix(s.deps.BaseURL, "/"), u.String(), rel)
	}
	links := []string{link(1, "first")}
	if page > 1 {
		links = append(links, link(min(page-1, last), "prev"))
	}
	if page < last {
		links = append(links, link(page+1, "next"))
	}
	links = append(links, link(last, "last"))
	w.Header().Set("Link", strings.Join(links, ", "))
	w.Header().Set("X-Total-Count", strconv.Itoa(total))
}

func archivedParam(v *string) bool { return v != nil && *v == "true" }

// ListContacts implements ServerInterface.
func (s *server) ListContacts(w http.ResponseWriter, r *http.Request, administrationID AdministrationId, params ListContactsParams) {
	admin, ok := parseID(w, administrationID)
	if !ok {
		return
	}
	q := contacts.Query{Search: deref((*string)(params.Query)), IncludeArchived: archivedParam((*string)(params.IncludeArchived)), OpenTodos: deref(params.Todo) != ""}
	q.Page, q.PerPage = pageNumbers(params.Page, params.PerPage)
	if params.ContactField != nil {
		q.Field, q.Value = string(*params.ContactField), deref(params.ContactValue)
	}
	if params.ContactImportId != nil {
		// No contact imports exist, so no contact came from one.
		q.Field, q.Value = "customer_id", "\x00"
	}
	page, err := s.deps.Contacts.List(r.Context(), admin, user(r).ID, q)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.writePage(w, r, page)
}

func parseContactFilter(w http.ResponseWriter, filter *ContactFilter) (contacts.Query, bool) {
	q, err := contacts.ParseFilter(deref((*string)(filter)))
	if err != nil {
		var verr *resource.ValidationError
		if errors.As(err, &verr) {
			writeError(w, http.StatusBadRequest, strings.Join(verr.Fields["filter"], "; "))
		} else {
			writeError(w, http.StatusBadRequest, err.Error())
		}
		return q, false
	}
	return q, true
}

// FilterContacts implements ServerInterface.
func (s *server) FilterContacts(w http.ResponseWriter, r *http.Request, administrationID AdministrationId, params FilterContactsParams) {
	admin, ok := parseID(w, administrationID)
	if !ok {
		return
	}
	q, ok := parseContactFilter(w, params.Filter)
	if !ok {
		return
	}
	q.Search, q.IncludeArchived = deref((*string)(params.Query)), archivedParam((*string)(params.IncludeArchived))
	q.Page, q.PerPage = pageNumbers(params.Page, params.PerPage)
	page, err := s.deps.Contacts.List(r.Context(), admin, user(r).ID, q)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.writePage(w, r, page)
}

// ListContactVersions implements ServerInterface.
func (s *server) ListContactVersions(w http.ResponseWriter, r *http.Request, administrationID AdministrationId, params ListContactVersionsParams) {
	admin, ok := parseID(w, administrationID)
	if !ok {
		return
	}
	q, ok := parseContactFilter(w, params.Filter)
	if !ok {
		return
	}
	q.IncludeArchived = archivedParam((*string)(params.IncludeArchived))
	list, err := s.deps.Contacts.Versions(r.Context(), admin, user(r).ID, q)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	out := make([]SynchronizationItem, len(list))
	for i, v := range list {
		out[i] = SynchronizationItem{Id: v.ID.String(), Version: int(v.Version)}
	}
	writeJSON(w, http.StatusOK, out)
}

// FetchContacts implements ServerInterface.
func (s *server) FetchContacts(w http.ResponseWriter, r *http.Request, administrationID AdministrationId) {
	admin, ok := parseID(w, administrationID)
	if !ok {
		return
	}
	body, ok := decodeBody[struct {
		IDs []flexString `json:"ids"`
	}](w, r)
	if !ok {
		return
	}
	ids := make([]resource.ID, len(body.IDs))
	for i := range body.IDs {
		id, ok := flexID(w, &body.IDs[i])
		if !ok {
			return
		}
		ids[i] = *id
	}
	list, err := s.deps.Contacts.ContactsByID(r.Context(), admin, user(r).ID, ids)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, s.toContacts(list))
}

// ListContactDoubles implements ServerInterface.
func (s *server) ListContactDoubles(w http.ResponseWriter, r *http.Request, administrationID AdministrationId, params ListContactDoublesParams) {
	admin, ok := parseID(w, administrationID)
	if !ok {
		return
	}
	page, _ := pageNumbers(params.Page, nil)
	groups, err := s.deps.Contacts.Doubles(r.Context(), admin, user(r).ID, page)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	type group struct {
		Contacts []contactJSON `json:"contacts"`
	}
	out := make([]group, len(groups))
	for i, g := range groups {
		out[i] = group{Contacts: s.toContacts(g)}
	}
	writeJSON(w, http.StatusOK, out)
}

// GetContactByCustomerId implements ServerInterface.
func (s *server) GetContactByCustomerId(w http.ResponseWriter, r *http.Request, administrationID AdministrationId, customerID string) {
	admin, ok := parseID(w, administrationID)
	if !ok {
		return
	}
	c, err := s.deps.Contacts.ContactByCustomerID(r.Context(), admin, user(r).ID, customerID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, s.toContact(c))
}

// CreateContact implements ServerInterface.
func (s *server) CreateContact(w http.ResponseWriter, r *http.Request, administrationID AdministrationId) {
	admin, ok := parseID(w, administrationID)
	if !ok {
		return
	}
	body, ok := decodeBody[contactBody](w, r)
	if !ok {
		return
	}
	c, ok := toContactChanges(w, body)
	if !ok {
		return
	}
	contact, err := s.deps.Contacts.Create(r.Context(), admin, user(r).ID, c)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, s.toContact(contact))
}

// GetContact implements ServerInterface.
func (s *server) GetContact(w http.ResponseWriter, r *http.Request, administrationID AdministrationId, id Id, _ GetContactParams) {
	admin, rid, ok := parseIDs(w, administrationID, id)
	if !ok {
		return
	}
	c, err := s.deps.Contacts.Contact(r.Context(), admin, rid, user(r).ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, s.toContact(c))
}

// UpdateContact implements ServerInterface.
func (s *server) UpdateContact(w http.ResponseWriter, r *http.Request, administrationID AdministrationId, id Id) {
	admin, rid, ok := parseIDs(w, administrationID, id)
	if !ok {
		return
	}
	body, ok := decodeBody[contactBody](w, r)
	if !ok {
		return
	}
	c, ok := toContactChanges(w, body)
	if !ok {
		return
	}
	if c.Person != nil {
		writeJSON(w, http.StatusUnprocessableEntity, validationBody(map[string][]string{"contact_person": {"can only be given with a new contact; add people under contact_people"}}))
		return
	}
	contact, err := s.deps.Contacts.Update(r.Context(), admin, rid, user(r).ID, c)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, s.toContact(contact))
}

// DeleteContact implements ServerInterface.
func (s *server) DeleteContact(w http.ResponseWriter, r *http.Request, administrationID AdministrationId, id Id) {
	admin, rid, ok := parseIDs(w, administrationID, id)
	if !ok {
		return
	}
	if err := s.deps.Contacts.Delete(r.Context(), admin, rid, user(r).ID); err != nil {
		s.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ArchiveContact implements ServerInterface.
func (s *server) ArchiveContact(w http.ResponseWriter, r *http.Request, administrationID AdministrationId, id Id) {
	admin, rid, ok := parseIDs(w, administrationID, id)
	if !ok {
		return
	}
	if _, err := s.deps.Contacts.Archive(r.Context(), admin, rid, user(r).ID); err != nil {
		s.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func parseContactIDs(w http.ResponseWriter, administrationID AdministrationId, contactID ContactId, id Id) (resource.ID, resource.ID, resource.ID, bool) {
	admin, contact, ok := parseIDs(w, administrationID, contactID)
	if !ok {
		return 0, 0, 0, false
	}
	rid, ok := parseID(w, id)
	return admin, contact, rid, ok
}

// CreateContactPerson implements ServerInterface.
func (s *server) CreateContactPerson(w http.ResponseWriter, r *http.Request, administrationID AdministrationId, contactID ContactId) {
	admin, contact, ok := parseIDs(w, administrationID, contactID)
	if !ok {
		return
	}
	body, ok := decodeBody[struct {
		Person personBody `json:"contact_person"`
	}](w, r)
	if !ok {
		return
	}
	p, err := s.deps.Contacts.AddPerson(r.Context(), admin, contact, user(r).ID, body.Person.changes())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, toContactPerson(p))
}

// GetContactPerson implements ServerInterface.
func (s *server) GetContactPerson(w http.ResponseWriter, r *http.Request, administrationID AdministrationId, contactID ContactId, id Id) {
	admin, contact, rid, ok := parseContactIDs(w, administrationID, contactID, id)
	if !ok {
		return
	}
	p, err := s.deps.Contacts.Person(r.Context(), admin, contact, rid, user(r).ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, toContactPerson(p))
}

// UpdateContactPerson implements ServerInterface.
func (s *server) UpdateContactPerson(w http.ResponseWriter, r *http.Request, administrationID AdministrationId, contactID ContactId, id Id) {
	admin, contact, rid, ok := parseContactIDs(w, administrationID, contactID, id)
	if !ok {
		return
	}
	body, ok := decodeBody[struct {
		Person personBody `json:"contact_person"`
	}](w, r)
	if !ok {
		return
	}
	p, err := s.deps.Contacts.UpdatePerson(r.Context(), admin, contact, rid, user(r).ID, body.Person.changes())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, toContactPerson(p))
}

// DeleteContactPerson implements ServerInterface.
func (s *server) DeleteContactPerson(w http.ResponseWriter, r *http.Request, administrationID AdministrationId, contactID ContactId, id Id) {
	admin, contact, rid, ok := parseContactIDs(w, administrationID, contactID, id)
	if !ok {
		return
	}
	if err := s.deps.Contacts.DeletePerson(r.Context(), admin, contact, rid, user(r).ID); err != nil {
		s.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// CreateContactNote implements ServerInterface.
func (s *server) CreateContactNote(w http.ResponseWriter, r *http.Request, administrationID AdministrationId, contactID ContactId) {
	admin, contact, ok := parseIDs(w, administrationID, contactID)
	if !ok {
		return
	}
	body, ok := decodeBody[struct {
		Note struct {
			Note       string      `json:"note"`
			Todo       *flexBool   `json:"todo"`
			AssigneeID *flexString `json:"assignee_id"`
		} `json:"note"`
	}](w, r)
	if !ok {
		return
	}
	in := contacts.NoteInput{Note: body.Note.Note, Todo: deref(body.Note.Todo.ptr())}
	if body.Note.AssigneeID != nil && *body.Note.AssigneeID != "" {
		id, ok := flexID(w, body.Note.AssigneeID)
		if !ok {
			return
		}
		in.AssigneeID = id
	}
	n, err := s.deps.Contacts.AddNote(r.Context(), admin, contact, user(r).ID, in)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, toNote(n))
}

// DeleteContactNote implements ServerInterface.
func (s *server) DeleteContactNote(w http.ResponseWriter, r *http.Request, administrationID AdministrationId, contactID ContactId, id Id) {
	admin, contact, rid, ok := parseContactIDs(w, administrationID, contactID, id)
	if !ok {
		return
	}
	if err := s.deps.Contacts.DeleteNote(r.Context(), admin, contact, rid, user(r).ID); err != nil {
		s.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
