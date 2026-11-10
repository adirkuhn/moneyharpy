package contacts

import (
	"slices"
	"strings"
	"time"

	"github.com/adirkuhn/moneyharpy/internal/resource"
)

// Query selects contacts, after Moneybird's list parameters and filter.
type Query struct {
	// Search matches names, contact people, email, phone, customer id, tax
	// and chamber of commerce numbers, IBAN and address, ignoring case.
	Search          string
	IncludeArchived bool
	// Field and Value select contacts whose field equals the value.
	Field, Value string
	// OpenTodos keeps contacts with an unfinished to-do.
	OpenTodos bool
	Page      int
	PerPage   int

	CreatedAfter       *time.Time
	UpdatedAfter       *time.Time
	FirstName          string
	LastName           string
	InvoiceWorkflowID  *resource.ID
	EstimateWorkflowID *resource.ID
	// ContactType is company or private_individual; empty means all.
	ContactType string
	// DeliveryMethod is one of DeliveryMethods; empty means all.
	DeliveryMethod string
	// Trusted is trusted or not_trusted; empty means all.
	Trusted string
	// Mandate is manual, adyen or none; empty means all.
	Mandate string
}

// QueryFields are the fields Field may name.
var QueryFields = []string{"customer_id", "company_name", "firstname", "lastname", "email", "phone", "tax_number", "chamber_of_commerce", "bank_account", "sepa_iban", "zipcode", "city"}

// FieldValue returns the contact's value of one of QueryFields.
func (c Contact) FieldValue(field string) string {
	switch field {
	case "customer_id":
		return c.CustomerID
	case "company_name":
		return c.CompanyName
	case "firstname":
		return c.Firstname
	case "lastname":
		return c.Lastname
	case "email":
		return c.SendInvoicesToEmail
	case "phone":
		return c.Phone
	case "tax_number":
		return c.TaxNumber
	case "chamber_of_commerce":
		return c.ChamberOfCommerce
	case "bank_account":
		return c.BankAccount
	case "sepa_iban":
		return c.SepaIBAN
	case "zipcode":
		return c.Zipcode
	case "city":
		return c.City
	}
	return ""
}

// ParseFilter reads Moneybird's contact filter: key:value terms separated by
// commas, such as first_name:henk,contact_type:company.
func ParseFilter(filter string) (Query, error) {
	var q Query
	var verr resource.ValidationError
	for term := range strings.SplitSeq(filter, ",") {
		term = strings.TrimSpace(term)
		if term == "" {
			continue
		}
		key, value, ok := strings.Cut(term, ":")
		if !ok {
			verr.Add("filter", "has a term without a value: "+term)
			continue
		}
		value = strings.TrimSpace(value)
		switch key {
		case "created_after", "updated_after":
			t, err := time.Parse(time.RFC3339, value)
			if err != nil {
				verr.Add("filter", key+" is not an ISO 8601 time")
				continue
			}
			if key == "created_after" {
				q.CreatedAfter = &t
			} else {
				q.UpdatedAfter = &t
			}
		case "first_name":
			q.FirstName = value
		case "last_name":
			q.LastName = value
		case "invoice_workflow_id", "estimate_workflow_id":
			id, err := resource.ParseID(value)
			if err != nil {
				verr.Add("filter", key+" is not an id")
				continue
			}
			if key == "invoice_workflow_id" {
				q.InvoiceWorkflowID = &id
			} else {
				q.EstimateWorkflowID = &id
			}
		case "contact_type":
			q.ContactType = oneOf(&verr, key, value, "company", "private_individual")
		case "delivery_method":
			v := oneOf(&verr, key, value, "email", "post", "manual", "peppol", "simplerinvoicing")
			if i := slices.IndexFunc(DeliveryMethods, func(m string) bool { return strings.EqualFold(m, v) }); i >= 0 {
				q.DeliveryMethod = DeliveryMethods[i]
			}
		case "trusted_type":
			q.Trusted = oneOf(&verr, key, value, "trusted", "not_trusted")
		case "mandate_type":
			q.Mandate = oneOf(&verr, key, value, "manual", "adyen", "none")
		default:
			verr.Add("filter", "has an unknown key: "+key)
		}
	}
	return q, verr.OrNil()
}

// oneOf returns value when it is one of allowed, "" for all, and records a
// problem otherwise.
func oneOf(verr *resource.ValidationError, key, value string, allowed ...string) string {
	if value == "all" {
		return ""
	}
	if !slices.Contains(allowed, value) {
		verr.Add("filter", key+" is not one of all, "+strings.Join(allowed, ", "))
		return ""
	}
	return value
}

// Matches reports whether the query selects the contact. Stores without a
// query language of their own use it; it is the reference for those that
// have one.
func (q Query) Matches(c Contact) bool {
	switch {
	case c.Archived && !q.IncludeArchived,
		q.CreatedAfter != nil && !c.CreatedAt.After(*q.CreatedAfter),
		q.UpdatedAfter != nil && !c.UpdatedAt.After(*q.UpdatedAfter),
		q.FirstName != "" && !strings.EqualFold(c.Firstname, q.FirstName),
		q.LastName != "" && !strings.EqualFold(c.Lastname, q.LastName),
		q.InvoiceWorkflowID != nil && (c.InvoiceWorkflowID == nil || *c.InvoiceWorkflowID != *q.InvoiceWorkflowID),
		q.EstimateWorkflowID != nil && (c.EstimateWorkflowID == nil || *c.EstimateWorkflowID != *q.EstimateWorkflowID),
		q.ContactType == "company" && c.CompanyName == "",
		q.ContactType == "private_individual" && c.CompanyName != "",
		q.DeliveryMethod != "" && c.DeliveryMethod != q.DeliveryMethod,
		// MoneyHarpy marks no contact trusted yet.
		q.Trusted == "trusted",
		q.Mandate == "manual" && !c.SepaActive,
		q.Mandate == "adyen",
		q.Mandate == "none" && c.SepaActive,
		q.Field != "" && !strings.EqualFold(c.FieldValue(q.Field), q.Value),
		q.OpenTodos && !slices.ContainsFunc(c.Notes, func(n Note) bool { return n.Todo && n.CompletedAt == nil }):
		return false
	}
	return q.Search == "" || c.matchesSearch(q.Search)
}

func (c Contact) matchesSearch(search string) bool {
	needle := strings.ToLower(strings.TrimSpace(search))
	haystack := []string{
		c.CompanyName, c.Firstname, c.Lastname, joinName(c.Firstname, c.Lastname), c.SendInvoicesToEmail, c.SendEstimatesToEmail, c.Phone,
		c.CustomerID, c.TaxNumber, c.ChamberOfCommerce, c.BankAccount, c.SepaIBAN, c.Address1, c.Address2, c.Zipcode, c.City,
	}
	for _, p := range c.People {
		haystack = append(haystack, p.Firstname, p.Lastname, joinName(p.Firstname, p.Lastname), p.Email, p.Phone)
	}
	return slices.ContainsFunc(haystack, func(s string) bool { return strings.Contains(strings.ToLower(s), needle) })
}

// Less orders contacts by name, then id, as lists show them.
func Less(a, b Contact) int {
	if c := strings.Compare(strings.ToLower(a.Name()), strings.ToLower(b.Name())); c != 0 {
		return c
	}
	switch {
	case a.ID < b.ID:
		return -1
	case a.ID > b.ID:
		return 1
	}
	return 0
}
