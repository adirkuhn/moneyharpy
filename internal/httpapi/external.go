package httpapi

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/adirkuhn/moneyharpy/internal/administration"
	"github.com/adirkuhn/moneyharpy/internal/contacts"
	"github.com/adirkuhn/moneyharpy/internal/resource"
	"github.com/adirkuhn/moneyharpy/internal/sales"
)

type externalSalesInvoiceJSON struct {
	ID                    string            `json:"id"`
	AdministrationID      string            `json:"administration_id"`
	ContactID             *string           `json:"contact_id"`
	Contact               *contactJSON      `json:"contact"`
	Date                  string            `json:"date"`
	State                 string            `json:"state"`
	DueDate               *string           `json:"due_date"`
	Reference             *string           `json:"reference"`
	EntryNumber           int               `json:"entry_number"`
	Origin                *string           `json:"origin"`
	Source                *string           `json:"source"`
	SourceURL             *string           `json:"source_url"`
	Currency              string            `json:"currency"`
	PaidAt                *string           `json:"paid_at"`
	CreatedAt             time.Time         `json:"created_at"`
	UpdatedAt             time.Time         `json:"updated_at"`
	Version               int64             `json:"version"`
	Details               []salesDetailJSON `json:"details"`
	Payments              []paymentJSON     `json:"payments"`
	TotalPaid             string            `json:"total_paid"`
	TotalUnpaid           string            `json:"total_unpaid"`
	TotalUnpaidBase       string            `json:"total_unpaid_base"`
	PricesAreInclTax      bool              `json:"prices_are_incl_tax"`
	TotalPriceExclTax     string            `json:"total_price_excl_tax"`
	TotalPriceExclTaxBase string            `json:"total_price_excl_tax_base"`
	TotalPriceInclTax     string            `json:"total_price_incl_tax"`
	TotalPriceInclTaxBase string            `json:"total_price_incl_tax_base"`
	TotalTax              string            `json:"total_tax"`
	MarkedDubiousOn       *string           `json:"marked_dubious_on"`
	MarkedUncollectibleOn *string           `json:"marked_uncollectible_on"`
	Notes                 []noteJSON        `json:"notes"`
	Attachments           []attachmentJSON  `json:"attachments"`
	Events                []eventJSON       `json:"events"`
	TaxTotals             []taxTotalJSON    `json:"tax_totals"`
	URI                   string            `json:"uri"`
}

func dateText(t *time.Time) *string {
	if t == nil {
		return nil
	}
	v := t.Format(time.DateOnly)
	return &v
}

func (s *server) toExternalSalesInvoice(e sales.ExternalInvoice, contact *contacts.Contact) externalSalesInvoiceJSON {
	paid, unpaid := e.Paid(), e.Unpaid()
	out := externalSalesInvoiceJSON{
		ID: e.ID.String(), AdministrationID: e.AdministrationID.String(), Date: e.Date.Format(time.DateOnly), State: e.State, DueDate: dateText(e.DueDate),
		Reference: nz(e.Reference), EntryNumber: e.EntryNumber, Origin: nz(e.Origin), Source: nz(e.Source), SourceURL: nz(e.SourceURL),
		Currency: e.Currency, PaidAt: dateText(e.PaidAt), CreatedAt: e.CreatedAt, UpdatedAt: e.UpdatedAt, Version: e.Version(),
		Details: []salesDetailJSON{}, Payments: []paymentJSON{}, TotalPaid: paid.String(), TotalUnpaid: unpaid.String(), TotalUnpaidBase: unpaid.String(),
		PricesAreInclTax: e.PricesAreInclTax, TotalPriceExclTax: e.Totals.ExclTax.String(), TotalPriceExclTaxBase: e.Totals.ExclTax.String(),
		TotalPriceInclTax: e.Totals.InclTax.String(), TotalPriceInclTaxBase: e.Totals.InclTax.String(), TotalTax: e.Totals.Tax.String(),
		MarkedDubiousOn: dateText(e.MarkedDubiousOn), MarkedUncollectibleOn: dateText(e.MarkedUncollectibleOn), Notes: []noteJSON{},
		Attachments: []attachmentJSON{}, Events: toEvents(e.Events), TaxTotals: []taxTotalJSON{}, URI: e.Ref().URI(),
	}
	if e.ContactID != 0 {
		out.ContactID = ptr(e.ContactID.String())
	}
	if contact != nil {
		c := s.toContact(*contact)
		out.Contact = &c
	}
	for i, d := range e.Lines {
		base := "0.0"
		if i < len(e.Totals.LineBases) {
			base = e.Totals.LineBases[i].String()
		}
		out.Details = append(out.Details, salesDetailJSON{
			ID: d.ID.String(), AdministrationID: e.AdministrationID.String(), TaxRateID: d.TaxRateID.String(), LedgerAccountID: deref(idString(d.LedgerAccountID)),
			ProjectID: idString(d.ProjectID), ProductID: idString(d.ProductID), Amount: d.Amount, AmountDecimal: quantityText(d.Quantity),
			Description: d.Description, Price: d.Price.String(), Period: nz(d.Period), RowOrder: d.RowOrder, TotalPriceExclTaxWithDisc: base,
			TotalPriceExclTaxWithDiscBs: base, TaxReportReference: []string{}, IsSelected: true, CreatedAt: d.CreatedAt, UpdatedAt: d.UpdatedAt,
		})
	}
	for _, p := range e.Payments {
		out.Payments = append(out.Payments, toPayment(p))
	}
	for _, r := range e.Totals.ByRate {
		out.TaxTotals = append(out.TaxTotals, taxTotalJSON{TaxRateID: r.TaxRateID.String(), TaxableAmount: r.Taxable.String(), TaxableAmountBase: r.Taxable.String(), TaxAmount: r.Tax.String(), TaxAmountBase: r.Tax.String()})
	}
	for _, n := range e.Notes {
		out.Notes = append(out.Notes, toNote(n))
	}
	for _, a := range e.Attachments {
		out.Attachments = append(out.Attachments, toAttachment(a))
	}
	return out
}

// toExternalSalesInvoices answers with invoices and their contacts, loaded
// at once.
func (s *server) toExternalSalesInvoices(r *http.Request, admin resource.ID, list []sales.ExternalInvoice) ([]externalSalesInvoiceJSON, error) {
	ids := make([]resource.ID, 0, len(list))
	for _, e := range list {
		if e.ContactID != 0 {
			ids = append(ids, e.ContactID)
		}
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
	out := make([]externalSalesInvoiceJSON, len(list))
	for i, e := range list {
		out[i] = s.toExternalSalesInvoice(e, byID[e.ContactID])
	}
	return out, nil
}

func (s *server) writeExternalSalesInvoice(w http.ResponseWriter, r *http.Request, admin resource.ID, status int, e sales.ExternalInvoice, err error) {
	if err != nil {
		s.fail(w, r, err)
		return
	}
	out, err := s.toExternalSalesInvoices(r, admin, []sales.ExternalInvoice{e})
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, status, out[0])
}

type externalSalesInvoiceBody struct {
	Invoice struct {
		ContactID        *flexString   `json:"contact_id"`
		Reference        *string       `json:"reference"`
		Date             *string       `json:"date"`
		DueDate          *string       `json:"due_date"`
		Currency         *string       `json:"currency"`
		PricesAreInclTax *flexBool     `json:"prices_are_incl_tax"`
		Source           *string       `json:"source"`
		SourceURL        *string       `json:"source_url"`
		Details          *salesDetails `json:"details_attributes"`
	} `json:"external_sales_invoice"`
}

// readExternalChanges reads the body of a create or update.
func readExternalChanges(w http.ResponseWriter, r *http.Request) (sales.ExternalChanges, bool) {
	body, ok := decodeBody[externalSalesInvoiceBody](w, r)
	if !ok {
		return sales.ExternalChanges{}, false
	}
	in := body.Invoice
	c := sales.ExternalChanges{Reference: in.Reference, Date: in.Date, DueDate: in.DueDate, Currency: in.Currency, PricesAreInclTax: in.PricesAreInclTax.ptr(),
		Source: in.Source, SourceURL: in.SourceURL}
	if c.ContactID, ok = flexOptionalID(w, in.ContactID); !ok {
		return c, false
	}
	if c.Details, ok = toDetailInputs(w, in.Details); !ok {
		return c, false
	}
	// Every line of an external invoice counts.
	for i := range c.Details {
		c.Details[i].Optional, c.Details[i].Selected = nil, nil
	}
	return c, true
}

func (s *server) externalQuery(w http.ResponseWriter, r *http.Request, admin resource.ID, filter *DocumentFilter) (sales.Query, bool) {
	now, err := s.today(r, admin)
	if err != nil {
		s.fail(w, r, err)
		return sales.Query{}, false
	}
	q, err := sales.ParseExternalFilter(deref((*string)(filter)), now)
	if err != nil {
		writeError(w, http.StatusBadRequest, filterProblem(err))
		return q, false
	}
	return q, true
}

// ListExternalSalesInvoices implements ServerInterface.
func (s *server) ListExternalSalesInvoices(w http.ResponseWriter, r *http.Request, administrationID AdministrationId, params ListExternalSalesInvoicesParams) {
	admin, ok := parseID(w, administrationID)
	if !ok {
		return
	}
	q, ok := s.externalQuery(w, r, admin, params.Filter)
	if !ok {
		return
	}
	q.Page, q.PerPage = pageNumbers(params.Page, params.PerPage)
	q.Ascending = params.Sort != nil && *params.Sort == "date_asc"
	list, total, err := s.deps.Sales.ExternalList(r.Context(), admin, user(r).ID, q)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	out, err := s.toExternalSalesInvoices(r, admin, list)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.writeLinks(w, r, max(q.Page, 1), cmpPerPage(q.PerPage, sales.DefaultPerPage), total)
	writeJSON(w, http.StatusOK, out)
}

// CreateExternalSalesInvoice implements ServerInterface.
func (s *server) CreateExternalSalesInvoice(w http.ResponseWriter, r *http.Request, administrationID AdministrationId) {
	admin, ok := parseID(w, administrationID)
	if !ok {
		return
	}
	c, ok := readExternalChanges(w, r)
	if !ok {
		return
	}
	e, err := s.deps.Sales.CreateExternal(r.Context(), admin, user(r).ID, c)
	s.writeExternalSalesInvoice(w, r, admin, http.StatusCreated, e, err)
}

// CreateExternalSalesInvoiceFromAttachment implements ServerInterface.
func (s *server) CreateExternalSalesInvoiceFromAttachment(w http.ResponseWriter, r *http.Request, administrationID AdministrationId) {
	admin, ok := parseID(w, administrationID)
	if !ok {
		return
	}
	name, contentType, data, ok := readUpload(w, r)
	if !ok {
		return
	}
	e, err := s.deps.Sales.CreateExternalFromFile(r.Context(), admin, user(r).ID, name, contentType, data)
	s.writeExternalSalesInvoice(w, r, admin, http.StatusOK, e, err)
}

// ListExternalSalesInvoiceVersions implements ServerInterface.
func (s *server) ListExternalSalesInvoiceVersions(w http.ResponseWriter, r *http.Request, administrationID AdministrationId, params ListExternalSalesInvoiceVersionsParams) {
	admin, ok := parseID(w, administrationID)
	if !ok {
		return
	}
	q, ok := s.externalQuery(w, r, admin, params.Filter)
	if !ok {
		return
	}
	list, err := s.deps.Sales.ExternalVersions(r.Context(), admin, user(r).ID, q)
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

// FetchExternalSalesInvoices implements ServerInterface.
func (s *server) FetchExternalSalesInvoices(w http.ResponseWriter, r *http.Request, administrationID AdministrationId) {
	admin, ok := parseID(w, administrationID)
	if !ok {
		return
	}
	ids, ok := readSyncIDs(w, r)
	if !ok {
		return
	}
	list, err := s.deps.Sales.ExternalInvoicesByID(r.Context(), admin, user(r).ID, ids)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	out, err := s.toExternalSalesInvoices(r, admin, list)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

// GetExternalSalesInvoice implements ServerInterface.
func (s *server) GetExternalSalesInvoice(w http.ResponseWriter, r *http.Request, administrationID AdministrationId, id Id) {
	admin, rid, ok := parseIDs(w, administrationID, id)
	if !ok {
		return
	}
	e, err := s.deps.Sales.ExternalInvoice(r.Context(), admin, rid, user(r).ID)
	s.writeExternalSalesInvoice(w, r, admin, http.StatusOK, e, err)
}

// UpdateExternalSalesInvoice implements ServerInterface.
func (s *server) UpdateExternalSalesInvoice(w http.ResponseWriter, r *http.Request, administrationID AdministrationId, id Id) {
	admin, rid, ok := parseIDs(w, administrationID, id)
	if !ok {
		return
	}
	c, ok := readExternalChanges(w, r)
	if !ok {
		return
	}
	e, err := s.deps.Sales.UpdateExternal(r.Context(), admin, rid, user(r).ID, c)
	s.writeExternalSalesInvoice(w, r, admin, http.StatusOK, e, err)
}

// DeleteExternalSalesInvoice implements ServerInterface.
func (s *server) DeleteExternalSalesInvoice(w http.ResponseWriter, r *http.Request, administrationID AdministrationId, id Id) {
	admin, rid, ok := parseIDs(w, administrationID, id)
	if !ok {
		return
	}
	// Deleting unlinks the bank transactions that pay the invoice.
	e, err := s.deps.Sales.ExternalInvoice(r.Context(), admin, rid, user(r).ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	for _, p := range e.Payments {
		if p.FinancialMutationID != nil {
			if !s.requireAlso(w, r, admin, administration.Bank) {
				return
			}
			break
		}
	}
	if err := s.deps.Sales.DeleteExternal(r.Context(), admin, rid, user(r).ID); err != nil {
		s.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// CreateExternalSalesInvoiceAttachment implements ServerInterface.
func (s *server) CreateExternalSalesInvoiceAttachment(w http.ResponseWriter, r *http.Request, administrationID AdministrationId, id Id) {
	admin, rid, ok := parseIDs(w, administrationID, id)
	if !ok {
		return
	}
	name, contentType, data, ok := readUpload(w, r)
	if !ok {
		return
	}
	a, err := s.deps.Sales.AddExternalAttachment(r.Context(), admin, rid, user(r).ID, name, contentType, data)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, toAttachment(a))
}

// DeleteExternalSalesInvoiceAttachment implements ServerInterface.
func (s *server) DeleteExternalSalesInvoiceAttachment(w http.ResponseWriter, r *http.Request, administrationID AdministrationId, id Id, attachmentID Identifier) {
	admin, rid, ok := parseIDs(w, administrationID, id)
	if !ok {
		return
	}
	att, ok := parseID(w, attachmentID)
	if !ok {
		return
	}
	if err := s.deps.Sales.DeleteExternalAttachment(r.Context(), admin, rid, att, user(r).ID); err != nil {
		s.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// DownloadExternalSalesInvoiceAttachment implements ServerInterface.
func (s *server) DownloadExternalSalesInvoiceAttachment(w http.ResponseWriter, r *http.Request, administrationID AdministrationId, id Id, attachmentID Identifier) {
	s.redirectToAttachment(w, r, fileExternalAttachment, administrationID, id, attachmentID)
}

// CreateExternalSalesInvoiceNote implements ServerInterface.
func (s *server) CreateExternalSalesInvoiceNote(w http.ResponseWriter, r *http.Request, administrationID AdministrationId, id Id) {
	admin, rid, ok := parseIDs(w, administrationID, id)
	if !ok {
		return
	}
	in, ok := decodeNote(w, r)
	if !ok {
		return
	}
	n, err := s.deps.Sales.AddExternalNote(r.Context(), admin, rid, user(r).ID, in)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, toNote(n))
}

// DeleteExternalSalesInvoiceNote implements ServerInterface.
func (s *server) DeleteExternalSalesInvoiceNote(w http.ResponseWriter, r *http.Request, administrationID AdministrationId, id Id, noteID Identifier) {
	admin, rid, ok := parseIDs(w, administrationID, id)
	if !ok {
		return
	}
	note, ok := parseID(w, noteID)
	if !ok {
		return
	}
	if err := s.deps.Sales.DeleteExternalNote(r.Context(), admin, rid, note, user(r).ID); err != nil {
		s.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// CreateExternalSalesInvoicePayment implements ServerInterface.
func (s *server) CreateExternalSalesInvoicePayment(w http.ResponseWriter, r *http.Request, administrationID AdministrationId, id Id) {
	s.registerPayment(w, r, administrationID, id, http.StatusCreated, s.deps.Sales.RegisterExternalPayment)
}

// DeleteExternalSalesInvoicePayment implements ServerInterface.
func (s *server) DeleteExternalSalesInvoicePayment(w http.ResponseWriter, r *http.Request, administrationID AdministrationId, id Id, paymentID Identifier) {
	s.deletePayment(w, r, administrationID, id, paymentID, s.deps.Sales.DeleteExternalPayment)
}

// MarkExternalSalesInvoiceAsDubious implements ServerInterface.
func (s *server) MarkExternalSalesInvoiceAsDubious(w http.ResponseWriter, r *http.Request, administrationID AdministrationId, id Id) {
	admin, rid, ok := parseIDs(w, administrationID, id)
	if !ok {
		return
	}
	date, ok := dubiousDate(w, r)
	if !ok {
		return
	}
	e, err := s.deps.Sales.MarkExternalDubious(r.Context(), admin, rid, user(r).ID, date)
	s.writeExternalSalesInvoice(w, r, admin, http.StatusOK, e, err)
}

// MarkExternalSalesInvoiceAsUncollectible implements ServerInterface.
func (s *server) MarkExternalSalesInvoiceAsUncollectible(w http.ResponseWriter, r *http.Request, administrationID AdministrationId, id Id) {
	admin, rid, ok := parseIDs(w, administrationID, id)
	if !ok {
		return
	}
	body, ok := readUncollectible(w, r)
	if !ok {
		return
	}
	e, err := s.deps.Sales.MarkExternalUncollectible(r.Context(), admin, rid, user(r).ID, body.UncollectibleDate, body.BookMethod)
	s.writeExternalSalesInvoice(w, r, admin, http.StatusOK, e, err)
}

// filterProblem is what a bad filter answers.
func filterProblem(err error) string {
	var verr *resource.ValidationError
	if errors.As(err, &verr) {
		return strings.Join(verr.Fields["filter"], "; ")
	}
	return err.Error()
}

// readSyncIDs reads the ids of a synchronization fetch.
func readSyncIDs(w http.ResponseWriter, r *http.Request) ([]resource.ID, bool) {
	body, ok := decodeBody[struct {
		IDs []flexString `json:"ids"`
	}](w, r)
	if !ok {
		return nil, false
	}
	ids := make([]resource.ID, len(body.IDs))
	for i := range body.IDs {
		id, ok := flexID(w, &body.IDs[i])
		if !ok {
			return nil, false
		}
		ids[i] = *id
	}
	return ids, true
}
