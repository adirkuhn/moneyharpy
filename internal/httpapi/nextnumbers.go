package httpapi

import "net/http"

// GetNextCustomerId implements ServerInterface: the customer id the add
// contact screen shows prefilled.
func (s *server) GetNextCustomerId(w http.ResponseWriter, r *http.Request, administrationID AdministrationId) {
	admin, ok := parseID(w, administrationID)
	if !ok {
		return
	}
	id, err := s.deps.Contacts.NextCustomerID(r.Context(), admin, user(r).ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, NextCustomerId{CustomerId: id})
}

// GetNextSalesInvoiceDraftId implements ServerInterface: the draft number
// the add invoice screen shows as "Concept #n".
func (s *server) GetNextSalesInvoiceDraftId(w http.ResponseWriter, r *http.Request, administrationID AdministrationId) {
	admin, ok := parseID(w, administrationID)
	if !ok {
		return
	}
	n, err := s.deps.Sales.NextDraftID(r.Context(), admin, user(r).ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, NextDraftId{DraftId: n})
}
