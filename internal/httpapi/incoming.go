package httpapi

import (
	"net/http"
	"time"

	"github.com/adirkuhn/moneyharpy/internal/purchases"
	"github.com/adirkuhn/moneyharpy/internal/resource"
)

type incomingDocumentJSON struct {
	ID                string    `json:"id"`
	AdministrationID  string    `json:"administration_id"`
	Type              string    `json:"type"`
	Reference         string    `json:"reference"`
	ContactID         *string   `json:"contact_id"`
	ContactName       string    `json:"contact_name"`
	Date              string    `json:"date"`
	State             string    `json:"state"`
	Currency          string    `json:"currency"`
	TotalPriceInclTax string    `json:"total_price_incl_tax"`
	AttachmentsCount  int       `json:"attachments_count"`
	CreatedAt         time.Time `json:"created_at"`
	URI               string    `json:"uri"`
}

func toIncomingDocument(d purchases.Incoming) incomingDocumentJSON {
	o := incomingDocumentJSON{
		ID: d.ID.String(), AdministrationID: d.AdministrationID.String(), Type: d.Kind, Reference: d.Reference, ContactName: d.ContactName,
		Date: d.Date.Format(time.DateOnly), State: d.State, Currency: d.Currency, TotalPriceInclTax: d.TotalInclTax.String(),
		AttachmentsCount: d.AttachmentsCount, CreatedAt: d.CreatedAt, URI: "/api/v2" + d.Ref().URI(),
	}
	if d.ContactID != 0 {
		o.ContactID = ptr(d.ContactID.String())
	}
	return o
}

// incomingQuery reads the filter of the Incoming page.
func (s *server) incomingQuery(w http.ResponseWriter, r *http.Request, admin resource.ID, filter *DocumentFilter) (purchases.IncomingQuery, bool) {
	now, err := s.today(r, admin)
	if err != nil {
		s.fail(w, r, err)
		return purchases.IncomingQuery{}, false
	}
	q, err := purchases.ParseIncomingFilter(deref((*string)(filter)), now)
	if err != nil {
		writeError(w, http.StatusBadRequest, filterProblem(err))
		return q, false
	}
	return q, true
}

// ListIncomingDocuments implements ServerInterface.
func (s *server) ListIncomingDocuments(w http.ResponseWriter, r *http.Request, administrationID AdministrationId, params ListIncomingDocumentsParams) {
	admin, ok := parseID(w, administrationID)
	if !ok {
		return
	}
	q, ok := s.incomingQuery(w, r, admin, params.Filter)
	if !ok {
		return
	}
	q.Page, q.PerPage = pageNumbers(params.Page, params.PerPage)
	q.Ascending = params.Sort != nil && *params.Sort == "date_asc"
	list, total, err := s.deps.Purchases.IncomingList(r.Context(), admin, user(r).ID, q)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	out := make([]incomingDocumentJSON, len(list))
	for i, d := range list {
		out[i] = toIncomingDocument(d)
	}
	s.writeLinks(w, r, max(q.Page, 1), cmpPerPage(q.PerPage, purchases.DefaultPerPage), total)
	writeJSON(w, http.StatusOK, out)
}

// GetIncomingDocument implements ServerInterface.
func (s *server) GetIncomingDocument(w http.ResponseWriter, r *http.Request, administrationID AdministrationId, id Id) {
	admin, ok := parseID(w, administrationID)
	if !ok {
		return
	}
	docID, ok := parseID(w, id)
	if !ok {
		return
	}
	d, err := s.deps.Purchases.IncomingDocument(r.Context(), admin, docID, user(r).ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, toIncomingDocument(d))
}

type costCategoryJSON struct {
	LedgerAccountID *string `json:"ledger_account_id"`
	Name            string  `json:"name"`
	Amount          string  `json:"amount"`
}

// GetIncomingDocumentStatistics implements ServerInterface.
func (s *server) GetIncomingDocumentStatistics(w http.ResponseWriter, r *http.Request, administrationID AdministrationId, params GetIncomingDocumentStatisticsParams) {
	admin, ok := parseID(w, administrationID)
	if !ok {
		return
	}
	q, ok := s.incomingQuery(w, r, admin, params.Filter)
	if !ok {
		return
	}
	st, err := s.deps.Purchases.IncomingStatistics(r.Context(), admin, user(r).ID, q)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	categories := make([]costCategoryJSON, len(st.Categories))
	for i, c := range st.Categories {
		categories[i] = costCategoryJSON{Name: c.Name, Amount: c.Amount.String()}
		if c.LedgerAccountID != 0 {
			categories[i].LedgerAccountID = ptr(c.LedgerAccountID.String())
		}
	}
	writeJSON(w, http.StatusOK, struct {
		Currency      string             `json:"currency"`
		CostsExclTax  string             `json:"costs_excl_tax"`
		AmountPayable string             `json:"amount_payable"`
		Categories    []costCategoryJSON `json:"categories"`
	}{st.Currency, st.CostsExclTax.String(), st.AmountPayable.String(), categories})
}
