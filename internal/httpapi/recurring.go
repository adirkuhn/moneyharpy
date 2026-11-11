package httpapi

import (
	"net/http"
	"time"

	"github.com/adirkuhn/moneyharpy/internal/contacts"
	"github.com/adirkuhn/moneyharpy/internal/resource"
	"github.com/adirkuhn/moneyharpy/internal/sales"
)

type recurringSalesInvoiceJSON struct {
	ID                     string                 `json:"id"`
	AdministrationID       string                 `json:"administration_id"`
	ContactID              string                 `json:"contact_id"`
	Contact                *contactJSON           `json:"contact,omitempty"`
	ContactPersonID        *string                `json:"contact_person_id"`
	ContactPerson          *contactPersonJSON     `json:"contact_person"`
	WorkflowID             string                 `json:"workflow_id"`
	DocumentStyleID        string                 `json:"document_style_id"`
	IdentityID             string                 `json:"identity_id"`
	StartDate              string                 `json:"start_date"`
	InvoiceDate            string                 `json:"invoice_date"`
	LastDate               *string                `json:"last_date"`
	Active                 bool                   `json:"active"`
	PaymentConditions      *string                `json:"payment_conditions"`
	Reference              *string                `json:"reference"`
	Language               string                 `json:"language"`
	Currency               string                 `json:"currency"`
	Discount               string                 `json:"discount"`
	FirstDueInterval       int                    `json:"first_due_interval"`
	AutoSend               bool                   `json:"auto_send"`
	SendingScheduledAt     *time.Time             `json:"sending_scheduled_at"`
	SendingScheduledUserID *string                `json:"sending_scheduled_user_id"`
	Mergeable              bool                   `json:"mergeable"`
	FrequencyType          string                 `json:"frequency_type"`
	Frequency              int                    `json:"frequency"`
	InvoicesCreated        int                    `json:"invoices_created"`
	CreatedAt              time.Time              `json:"created_at"`
	UpdatedAt              time.Time              `json:"updated_at"`
	Version                int64                  `json:"version"`
	PricesAreInclTax       bool                   `json:"prices_are_incl_tax"`
	TotalPriceExclTax      string                 `json:"total_price_excl_tax"`
	TotalPriceExclTaxBase  string                 `json:"total_price_excl_tax_base"`
	TotalPriceInclTax      string                 `json:"total_price_incl_tax"`
	TotalPriceInclTaxBase  string                 `json:"total_price_incl_tax_base"`
	TotalTax               string                 `json:"total_tax"`
	TotalDiscount          string                 `json:"total_discount"`
	Details                []salesDetailJSON      `json:"details"`
	CustomFields           []customFieldValueJSON `json:"custom_fields"`
	Notes                  []noteJSON             `json:"notes"`
	Attachments            []attachmentJSON       `json:"attachments"`
	Events                 []any                  `json:"events"`
	Subscription           any                    `json:"subscription"`
	URI                    string                 `json:"uri"`
}

func (s *server) toRecurringSalesInvoice(rec sales.Recurring, contact *contacts.Contact) recurringSalesInvoiceJSON {
	out := recurringSalesInvoiceJSON{
		ID: rec.ID.String(), AdministrationID: rec.AdministrationID.String(), ContactID: rec.ContactID.String(), ContactPersonID: idString(rec.ContactPersonID),
		WorkflowID: rec.WorkflowID.String(), DocumentStyleID: rec.DocumentStyleID.String(), IdentityID: rec.IdentityID.String(),
		StartDate: rec.StartDate.Format(time.DateOnly), InvoiceDate: rec.InvoiceDate.Format(time.DateOnly), LastDate: dateText(rec.LastDate),
		Active: rec.Active, PaymentConditions: nz(rec.PaymentConditions), Reference: nz(rec.Reference), Language: rec.Language, Currency: rec.Currency,
		Discount: percentText(rec.Discount), FirstDueInterval: rec.FirstDueInterval, AutoSend: rec.AutoSend, SendingScheduledAt: rec.AutoSendAt,
		SendingScheduledUserID: idString(rec.AutoSendUserID), Mergeable: rec.Mergeable, FrequencyType: rec.FrequencyType, Frequency: rec.Frequency,
		InvoicesCreated: rec.Generated, CreatedAt: rec.CreatedAt, UpdatedAt: rec.UpdatedAt, Version: rec.Version(), PricesAreInclTax: rec.PricesAreInclTax,
		TotalPriceExclTax: rec.Totals.ExclTax.String(), TotalPriceExclTaxBase: rec.Totals.ExclTax.String(), TotalPriceInclTax: rec.Totals.InclTax.String(),
		TotalPriceInclTaxBase: rec.Totals.InclTax.String(), TotalTax: rec.Totals.Tax.String(), TotalDiscount: rec.Totals.Discount.String(),
		Details: []salesDetailJSON{}, CustomFields: []customFieldValueJSON{}, Notes: []noteJSON{}, Attachments: []attachmentJSON{}, Events: []any{},
		URI: rec.Ref().URI(),
	}
	if contact != nil {
		c := s.toContact(*contact)
		out.Contact = &c
		if rec.ContactPersonID != nil {
			for _, p := range contact.People {
				if p.ID == *rec.ContactPersonID {
					cp := toContactPerson(p)
					out.ContactPerson = &cp
				}
			}
		}
	}
	for i, d := range rec.Lines {
		base := "0.0"
		if i < len(rec.Totals.LineBases) {
			base = rec.Totals.LineBases[i].String()
		}
		out.Details = append(out.Details, salesDetailJSON{
			ID: d.ID.String(), AdministrationID: rec.AdministrationID.String(), TaxRateID: d.TaxRateID.String(), LedgerAccountID: deref(idString(d.LedgerAccountID)),
			ProjectID: idString(d.ProjectID), ProductID: idString(d.ProductID), Amount: d.Amount, AmountDecimal: quantityText(d.Quantity),
			Description: d.Description, Price: d.Price.String(), Period: nz(d.Period), RowOrder: d.RowOrder, TotalPriceExclTaxWithDisc: base,
			TotalPriceExclTaxWithDiscBs: base, TaxReportReference: []string{}, IsOptional: d.Optional, IsSelected: d.Selected,
			CreatedAt: d.CreatedAt, UpdatedAt: d.UpdatedAt,
		})
	}
	for _, f := range rec.CustomFields {
		out.CustomFields = append(out.CustomFields, customFieldValueJSON{ID: f.FieldID.String(), Name: f.Name, Value: f.Value})
	}
	for _, n := range rec.Notes {
		out.Notes = append(out.Notes, toNote(n))
	}
	return out
}

// toRecurringSalesInvoices answers with recurring invoices and their
// contacts, loaded at once.
func (s *server) toRecurringSalesInvoices(r *http.Request, admin resource.ID, list []sales.Recurring) ([]recurringSalesInvoiceJSON, error) {
	ids := make([]resource.ID, 0, len(list))
	for _, rec := range list {
		ids = append(ids, rec.ContactID)
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
	out := make([]recurringSalesInvoiceJSON, len(list))
	for i, rec := range list {
		out[i] = s.toRecurringSalesInvoice(rec, byID[rec.ContactID])
	}
	return out, nil
}

func (s *server) writeRecurringSalesInvoice(w http.ResponseWriter, r *http.Request, admin resource.ID, status int, rec sales.Recurring, err error) {
	if err != nil {
		s.fail(w, r, err)
		return
	}
	out, err := s.toRecurringSalesInvoices(r, admin, []sales.Recurring{rec})
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, status, out[0])
}

type recurringSalesInvoiceBody struct {
	Invoice struct {
		ContactID         *flexString   `json:"contact_id"`
		ContactPersonID   *flexString   `json:"contact_person_id"`
		UpdateContact     *flexBool     `json:"update_contact"`
		DocumentStyleID   *flexString   `json:"document_style_id"`
		WorkflowID        *flexString   `json:"workflow_id"`
		IdentityID        *flexString   `json:"identity_id"`
		Reference         *string       `json:"reference"`
		InvoiceDate       *string       `json:"invoice_date"`
		Currency          *string       `json:"currency"`
		FirstDueInterval  *flexString   `json:"first_due_interval"`
		PricesAreInclTax  *flexBool     `json:"prices_are_incl_tax"`
		PaymentConditions *string       `json:"payment_conditions"`
		Discount          *flexString   `json:"discount"`
		FrequencyType     *string       `json:"frequency_type"`
		Frequency         *flexString   `json:"frequency"`
		HasDesiredCount   *flexBool     `json:"has_desired_count"`
		DesiredCount      *flexString   `json:"desired_count"`
		LastDate          *string       `json:"last_date"`
		AutoSend          *flexBool     `json:"auto_send"`
		Mergeable         *flexBool     `json:"mergeable"`
		Details           *salesDetails `json:"details_attributes"`
		CustomFields      *fieldValues  `json:"custom_fields_attributes"`
	} `json:"recurring_sales_invoice"`
}

// readRecurringChanges reads the body of a create or update.
func readRecurringChanges(w http.ResponseWriter, r *http.Request) (sales.RecurringChanges, bool) {
	body, ok := decodeBody[recurringSalesInvoiceBody](w, r)
	if !ok {
		return sales.RecurringChanges{}, false
	}
	in := body.Invoice
	c := sales.RecurringChanges{
		Reference: in.Reference, InvoiceDate: in.InvoiceDate, Currency: in.Currency, PricesAreInclTax: in.PricesAreInclTax.ptr(),
		PaymentConditions: in.PaymentConditions, Discount: in.Discount.ptr(), FrequencyType: in.FrequencyType, HasDesiredCount: in.HasDesiredCount.ptr(),
		LastDate: in.LastDate, AutoSend: in.AutoSend.ptr(), Mergeable: in.Mergeable.ptr(),
	}
	for _, f := range []struct {
		src *flexString
		dst **resource.ID
	}{{in.ContactID, &c.ContactID}, {in.ContactPersonID, &c.ContactPersonID}, {in.DocumentStyleID, &c.DocumentStyleID}, {in.WorkflowID, &c.WorkflowID}, {in.IdentityID, &c.IdentityID}} {
		if *f.dst, ok = flexOptionalID(w, f.src); !ok {
			return c, false
		}
	}
	for _, f := range []struct {
		src   *flexString
		dst   **int
		field string
	}{{in.FirstDueInterval, &c.FirstDueInterval, "first_due_interval"}, {in.Frequency, &c.Frequency, "frequency"}, {in.DesiredCount, &c.DesiredCount, "desired_count"}} {
		if *f.dst, ok = flexInt(w, f.src, f.field); !ok {
			return c, false
		}
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

func recurringQuery(w http.ResponseWriter, filter *DocumentFilter) (sales.RecurringQuery, bool) {
	q, err := sales.ParseRecurringFilter(deref((*string)(filter)))
	if err != nil {
		writeError(w, http.StatusBadRequest, filterProblem(err))
		return q, false
	}
	return q, true
}

// ListRecurringSalesInvoices implements ServerInterface.
func (s *server) ListRecurringSalesInvoices(w http.ResponseWriter, r *http.Request, administrationID AdministrationId, params ListRecurringSalesInvoicesParams) {
	admin, ok := parseID(w, administrationID)
	if !ok {
		return
	}
	q, ok := recurringQuery(w, params.Filter)
	if !ok {
		return
	}
	q.Page, q.PerPage = pageNumbers(params.Page, params.PerPage)
	list, total, err := s.deps.Sales.RecurringList(r.Context(), admin, user(r).ID, q)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	out, err := s.toRecurringSalesInvoices(r, admin, list)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.writeLinks(w, r, max(q.Page, 1), cmpPerPage(q.PerPage, sales.DefaultPerPage), total)
	writeJSON(w, http.StatusOK, out)
}

// CreateRecurringSalesInvoice implements ServerInterface.
func (s *server) CreateRecurringSalesInvoice(w http.ResponseWriter, r *http.Request, administrationID AdministrationId) {
	admin, ok := parseID(w, administrationID)
	if !ok {
		return
	}
	c, ok := readRecurringChanges(w, r)
	if !ok {
		return
	}
	rec, err := s.deps.Sales.CreateRecurring(r.Context(), admin, user(r).ID, c)
	s.writeRecurringSalesInvoice(w, r, admin, http.StatusCreated, rec, err)
}

// ListRecurringSalesInvoiceVersions implements ServerInterface.
func (s *server) ListRecurringSalesInvoiceVersions(w http.ResponseWriter, r *http.Request, administrationID AdministrationId, params ListRecurringSalesInvoiceVersionsParams) {
	admin, ok := parseID(w, administrationID)
	if !ok {
		return
	}
	q, ok := recurringQuery(w, params.Filter)
	if !ok {
		return
	}
	list, err := s.deps.Sales.RecurringVersions(r.Context(), admin, user(r).ID, q)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	out := make([]SynchronizationItem, len(list))
	for i, rec := range list {
		out[i] = SynchronizationItem{Id: rec.ID.String(), Version: int(rec.Version())}
	}
	writeJSON(w, http.StatusOK, out)
}

// FetchRecurringSalesInvoices implements ServerInterface.
func (s *server) FetchRecurringSalesInvoices(w http.ResponseWriter, r *http.Request, administrationID AdministrationId) {
	admin, ok := parseID(w, administrationID)
	if !ok {
		return
	}
	ids, ok := readSyncIDs(w, r)
	if !ok {
		return
	}
	list, err := s.deps.Sales.RecurringByID(r.Context(), admin, user(r).ID, ids)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	out, err := s.toRecurringSalesInvoices(r, admin, list)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

// GetRecurringSalesInvoice implements ServerInterface.
func (s *server) GetRecurringSalesInvoice(w http.ResponseWriter, r *http.Request, administrationID AdministrationId, id Id) {
	admin, rid, ok := parseIDs(w, administrationID, id)
	if !ok {
		return
	}
	rec, err := s.deps.Sales.Recurring(r.Context(), admin, rid, user(r).ID)
	s.writeRecurringSalesInvoice(w, r, admin, http.StatusOK, rec, err)
}

// UpdateRecurringSalesInvoice implements ServerInterface.
func (s *server) UpdateRecurringSalesInvoice(w http.ResponseWriter, r *http.Request, administrationID AdministrationId, id Id) {
	admin, rid, ok := parseIDs(w, administrationID, id)
	if !ok {
		return
	}
	c, ok := readRecurringChanges(w, r)
	if !ok {
		return
	}
	rec, err := s.deps.Sales.UpdateRecurring(r.Context(), admin, rid, user(r).ID, c)
	s.writeRecurringSalesInvoice(w, r, admin, http.StatusOK, rec, err)
}

// DeleteRecurringSalesInvoice implements ServerInterface: one that made
// invoices is kept, deactivated, as the X-Kept-As header tells.
func (s *server) DeleteRecurringSalesInvoice(w http.ResponseWriter, r *http.Request, administrationID AdministrationId, id Id) {
	admin, rid, ok := parseIDs(w, administrationID, id)
	if !ok {
		return
	}
	kept, err := s.deps.Sales.DeleteRecurring(r.Context(), admin, rid, user(r).ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if kept {
		w.Header().Set("X-Kept-As", "deactivated")
	}
	w.WriteHeader(http.StatusNoContent)
}

// DeactivateRecurringSalesInvoice implements ServerInterface.
func (s *server) DeactivateRecurringSalesInvoice(w http.ResponseWriter, r *http.Request, administrationID AdministrationId, id Id) {
	admin, rid, ok := parseIDs(w, administrationID, id)
	if !ok {
		return
	}
	rec, err := s.deps.Sales.DeactivateRecurring(r.Context(), admin, rid, user(r).ID)
	s.writeRecurringSalesInvoice(w, r, admin, http.StatusOK, rec, err)
}

// CreateRecurringSalesInvoiceNote implements ServerInterface.
func (s *server) CreateRecurringSalesInvoiceNote(w http.ResponseWriter, r *http.Request, administrationID AdministrationId, id Id) {
	admin, rid, ok := parseIDs(w, administrationID, id)
	if !ok {
		return
	}
	in, ok := decodeNote(w, r)
	if !ok {
		return
	}
	n, err := s.deps.Sales.AddRecurringNote(r.Context(), admin, rid, user(r).ID, in)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, toNote(n))
}

// DeleteRecurringSalesInvoiceNote implements ServerInterface.
func (s *server) DeleteRecurringSalesInvoiceNote(w http.ResponseWriter, r *http.Request, administrationID AdministrationId, id Id, noteID Identifier) {
	admin, rid, ok := parseIDs(w, administrationID, id)
	if !ok {
		return
	}
	note, ok := parseID(w, noteID)
	if !ok {
		return
	}
	if err := s.deps.Sales.DeleteRecurringNote(r.Context(), admin, rid, note, user(r).ID); err != nil {
		s.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
