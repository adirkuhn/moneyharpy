package httpapi

import (
	"net/http"

	"github.com/adirkuhn/moneyharpy/internal/administration"
	"github.com/adirkuhn/moneyharpy/internal/contacts"
	"github.com/adirkuhn/moneyharpy/internal/resource"
)

// LookupKvkNumber implements ServerInterface: the company the Dutch
// company register holds under a KVK number, to fill in a contact.
func (s *server) LookupKvkNumber(w http.ResponseWriter, r *http.Request, administrationID AdministrationId, params LookupKvkNumberParams) {
	if _, ok := parseID(w, administrationID); !ok {
		return
	}
	if s.deps.CompanyNumbers == nil {
		s.fail(w, r, administration.ErrLookupUnavailable)
		return
	}
	found, err := s.deps.CompanyNumbers.LookupCompany(r.Context(), params.KvkNumber)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, CompanyMatch{
		Name: found.Name, ChamberOfCommerce: found.ChamberOfCommerce, Address1: found.Address1, Zipcode: found.Zipcode, City: found.City,
	})
}

// duplicateMessages say what a registry number duplicate is, per field.
var duplicateMessages = map[string]string{
	contacts.FieldChamberOfCommerce: "A contact with this chamber of commerce number already exists: ",
	contacts.FieldTaxNumber:         "A contact with this VAT number already exists: ",
}

// ListContactRegistryDuplicates implements ServerInterface: the contacts
// that already hold a chamber of commerce or VAT number, for a form to warn
// before saving another.
func (s *server) ListContactRegistryDuplicates(w http.ResponseWriter, r *http.Request, administrationID AdministrationId, params ListContactRegistryDuplicatesParams) {
	admin, ok := parseID(w, administrationID)
	if !ok {
		return
	}
	var except resource.ID
	if params.ExceptId != nil && *params.ExceptId != "" {
		id, ok := parseID(w, *params.ExceptId)
		if !ok {
			return
		}
		except = id
	}
	found, err := s.deps.Contacts.RegistryDuplicates(r.Context(), admin, user(r).ID, orEmpty(params.ChamberOfCommerce), orEmpty(params.TaxNumber), except)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	out := make([]ContactRegistryDuplicate, len(found))
	for i, d := range found {
		name := d.Contact.Name()
		out[i] = ContactRegistryDuplicate{
			Field: ContactRegistryDuplicateField(d.Field), ContactId: d.Contact.ID.String(), ContactName: name,
			Message: duplicateMessages[d.Field] + name,
		}
	}
	writeJSON(w, http.StatusOK, out)
}

// orEmpty returns the string p points to, or "" for nil.
func orEmpty(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}
