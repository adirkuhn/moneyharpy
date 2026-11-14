package httpapi

import (
	"net/http"

	"github.com/adirkuhn/moneyharpy/internal/vatlookup"
)

// LookupVatNumber implements ServerInterface: what VIES registered for a VAT
// number, to fill in a contact.
func (s *server) LookupVatNumber(w http.ResponseWriter, r *http.Request, administrationID AdministrationId, params LookupVatNumberParams) {
	if _, ok := parseID(w, administrationID); !ok {
		return
	}
	if s.deps.VATNumbers == nil {
		s.fail(w, r, vatlookup.ErrDisabled)
		return
	}
	found, err := s.deps.VATNumbers.Lookup(r.Context(), params.VatNumber)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, VatLookup{
		Valid: found.Valid, Name: found.Name, Address1: found.Address1, Zipcode: found.Zipcode, City: found.City, Country: found.Country,
	})
}
