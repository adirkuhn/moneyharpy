package contacts

import (
	"context"
	"fmt"
	"net/mail"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/adirkuhn/moneyharpy/internal/administration"
	"github.com/adirkuhn/moneyharpy/internal/iban"
	"github.com/adirkuhn/moneyharpy/internal/resource"
)

const maxText = 255

var (
	bicPattern          = regexp.MustCompile(`^[A-Z]{6}[A-Z0-9]{2}([A-Z0-9]{3})?$`)
	siTypePattern       = regexp.MustCompile(`^[A-Za-z0-9:]{1,20}$`)
	customerIDPattern   = regexp.MustCompile(`^[^\s,]{1,50}$`)
	eInvoicingDelivery  = []string{"Simplerinvoicing", "Peppol"}
	contactFieldSources = administration.SourceContact
)

// Changes holds the fields to set; nil fields stay as they are. An empty
// workflow id clears it.
type Changes struct {
	CompanyName, Firstname, Lastname                      *string
	Address1, Address2, Zipcode, City, Country, Phone     *string
	CustomerID, TaxNumber, ChamberOfCommerce              *string
	BankAccount, DeliveryMethod                           *string
	SendInvoicesToAttention, SendInvoicesToEmail          *string
	SendEstimatesToAttention, SendEstimatesToEmail        *string
	EmailUBL, DirectDebit, SepaActive                     *bool
	SepaIBAN, SepaIBANAccountName, SepaBIC, SepaMandateID *string
	// SepaMandateDate is a yyyy-mm-dd date; empty clears it.
	SepaMandateDate                       *string
	SepaSequenceType                      *string
	SIIdentifierType, SIIdentifier        *string
	InvoiceWorkflowID, EstimateWorkflowID *resource.ID
	// Archived is a MoneyHarpy extension that also brings a contact back.
	Archived     *bool
	CustomFields []FieldValue
	// Person adds a contact person along with a new contact.
	Person *PersonChanges
}

// Create adds a contact. Without a customer id it gets the next number.
func (s *Service) Create(ctx context.Context, admin, user resource.ID, c Changes) (Contact, error) {
	a, err := s.admins.Get(ctx, admin, user)
	if err != nil {
		return Contact{}, err
	}
	now := s.now()
	contact := Contact{
		ID: s.ids.Next(), AdministrationID: admin, Country: a.Country, DeliveryMethod: "Email", SepaSequenceType: "RCUR",
		CreatedAt: now, UpdatedAt: now,
	}
	if err := s.apply(ctx, admin, user, &contact, c); err != nil {
		return Contact{}, err
	}
	if contact.CustomerID == "" {
		if contact.CustomerID, err = s.nextCustomerID(ctx, admin); err != nil {
			return Contact{}, err
		}
	}
	if c.Person != nil {
		p := Person{ID: s.ids.Next(), AdministrationID: admin, ContactID: contact.ID, CreatedAt: now, UpdatedAt: now}
		if err := applyPerson(&p, *c.Person, "contact_person."); err != nil {
			return Contact{}, err
		}
		contact.People = []Person{p}
	}
	if err := s.store.CreateContact(ctx, contact); err != nil {
		return Contact{}, fmt.Errorf("store contact: %w", err)
	}
	return contact, nil
}

// Update changes a contact.
func (s *Service) Update(ctx context.Context, admin, id, user resource.ID, c Changes) (Contact, error) {
	if _, err := s.admins.Get(ctx, admin, user); err != nil {
		return Contact{}, err
	}
	contact, err := s.store.Contact(ctx, admin, id)
	if err != nil {
		return Contact{}, err
	}
	if err := s.apply(ctx, admin, user, &contact, c); err != nil {
		return Contact{}, err
	}
	contact.UpdatedAt = s.now()
	if err := s.store.UpdateContact(ctx, contact); err != nil {
		return Contact{}, fmt.Errorf("store contact: %w", err)
	}
	return contact, nil
}

// Archive hides a contact from lists and pickers; its documents keep it.
func (s *Service) Archive(ctx context.Context, admin, id, user resource.ID) (Contact, error) {
	archived := true
	return s.Update(ctx, admin, id, user, Changes{Archived: &archived})
}

// Delete removes a contact that no document refers to. One that documents
// refer to is archived instead, as Moneybird does, and kept reports so.
func (s *Service) Delete(ctx context.Context, admin, id, user resource.ID) (kept bool, err error) {
	if _, err := s.admins.Get(ctx, admin, user); err != nil {
		return false, err
	}
	if _, err := s.store.Contact(ctx, admin, id); err != nil {
		return false, err
	}
	if s.references != nil {
		used, err := s.references.ContactInUse(ctx, admin, id)
		if err != nil {
			return false, fmt.Errorf("check contact use: %w", err)
		}
		if used {
			_, err := s.Archive(ctx, admin, id, user)
			return err == nil, err
		}
	}
	return false, s.store.DeleteContact(ctx, admin, id)
}

// Doubles groups the contacts that look like the same customer: they share
// a chamber of commerce number, tax number, email address or name. Groups
// come in order of their first contact; page counts from 1.
func (s *Service) Doubles(ctx context.Context, admin, user resource.ID, page int) ([][]Contact, error) {
	if _, err := s.admins.Get(ctx, admin, user); err != nil {
		return nil, err
	}
	all, _, err := s.store.Contacts(ctx, admin, Query{})
	if err != nil {
		return nil, fmt.Errorf("list contacts: %w", err)
	}
	groups := doubles(all)
	start := (max(page, 1) - 1) * DefaultPerPage
	if start >= len(groups) {
		return [][]Contact{}, nil
	}
	return groups[start:min(start+DefaultPerPage, len(groups))], nil
}

func doubles(all []Contact) [][]Contact {
	keys := func(c Contact) []string {
		var out []string
		add := func(kind, v string) {
			if v = strings.ToLower(strings.TrimSpace(v)); v != "" {
				out = append(out, kind+":"+v)
			}
		}
		add("coc", NormalizeChamberOfCommerce(c.ChamberOfCommerce))
		add("tax", NormalizeVAT(c.TaxNumber))
		add("email", c.SendInvoicesToEmail)
		add("name", c.Name())
		return out
	}
	// Union contacts that share any key.
	parent := make([]int, len(all))
	for i := range parent {
		parent[i] = i
	}
	var find func(int) int
	find = func(i int) int {
		if parent[i] != i {
			parent[i] = find(parent[i])
		}
		return parent[i]
	}
	seen := map[string]int{}
	for i, c := range all {
		for _, k := range keys(c) {
			if j, ok := seen[k]; ok {
				parent[find(i)] = find(j)
			} else {
				seen[k] = i
			}
		}
	}
	members := map[int][]Contact{}
	var order []int
	for i, c := range all {
		root := find(i)
		if _, ok := members[root]; !ok {
			order = append(order, root)
		}
		members[root] = append(members[root], c)
	}
	groups := [][]Contact{}
	for _, root := range order {
		if len(members[root]) > 1 {
			groups = append(groups, members[root])
		}
	}
	return groups
}

func (s *Service) apply(ctx context.Context, admin, user resource.ID, c *Contact, ch Changes) error {
	set := func(dst *string, src *string) {
		if src != nil {
			*dst = strings.TrimSpace(*src)
		}
	}
	set(&c.CompanyName, ch.CompanyName)
	set(&c.Firstname, ch.Firstname)
	set(&c.Lastname, ch.Lastname)
	set(&c.Address1, ch.Address1)
	set(&c.Address2, ch.Address2)
	set(&c.Zipcode, ch.Zipcode)
	set(&c.City, ch.City)
	set(&c.Phone, ch.Phone)
	set(&c.CustomerID, ch.CustomerID)
	set(&c.TaxNumber, ch.TaxNumber)
	set(&c.ChamberOfCommerce, ch.ChamberOfCommerce)
	set(&c.BankAccount, ch.BankAccount)
	set(&c.SendInvoicesToAttention, ch.SendInvoicesToAttention)
	set(&c.SendInvoicesToEmail, ch.SendInvoicesToEmail)
	set(&c.SendEstimatesToAttention, ch.SendEstimatesToAttention)
	set(&c.SendEstimatesToEmail, ch.SendEstimatesToEmail)
	set(&c.SepaIBANAccountName, ch.SepaIBANAccountName)
	set(&c.SepaMandateID, ch.SepaMandateID)
	set(&c.SIIdentifierType, ch.SIIdentifierType)
	set(&c.SIIdentifier, ch.SIIdentifier)
	if ch.Country != nil {
		c.Country = strings.ToUpper(strings.TrimSpace(*ch.Country))
	}
	if ch.DeliveryMethod != nil {
		c.DeliveryMethod = canonical(DeliveryMethods, *ch.DeliveryMethod)
	}
	if ch.SepaIBAN != nil {
		c.SepaIBAN = iban.Normalize(*ch.SepaIBAN)
	}
	if ch.SepaBIC != nil {
		c.SepaBIC = strings.ToUpper(strings.TrimSpace(*ch.SepaBIC))
	}
	if ch.SepaSequenceType != nil {
		c.SepaSequenceType = strings.ToUpper(strings.TrimSpace(*ch.SepaSequenceType))
	}
	for dst, src := range map[*bool]*bool{&c.EmailUBL: ch.EmailUBL, &c.DirectDebit: ch.DirectDebit, &c.SepaActive: ch.SepaActive, &c.Archived: ch.Archived} {
		if src != nil {
			*dst = *src
		}
	}
	var verr resource.ValidationError
	if ch.SepaMandateDate != nil {
		c.SepaMandateDate = nil
		if v := strings.TrimSpace(*ch.SepaMandateDate); v != "" {
			d, err := time.Parse(time.DateOnly, v)
			if err != nil {
				verr.Add("sepa_mandate_date", "is not a date")
			} else {
				c.SepaMandateDate = &d
			}
		}
	}
	if ch.InvoiceWorkflowID != nil {
		c.InvoiceWorkflowID = optional(*ch.InvoiceWorkflowID)
	}
	if ch.EstimateWorkflowID != nil {
		c.EstimateWorkflowID = optional(*ch.EstimateWorkflowID)
	}
	s.validate(ctx, admin, user, c, &verr)
	if ch.CustomFields != nil {
		s.applyCustomFields(ctx, admin, user, c, ch.CustomFields, &verr)
	}
	return verr.OrNil()
}

func optional(id resource.ID) *resource.ID {
	if id == 0 {
		return nil
	}
	return &id
}

// canonical returns the allowed value that equals v ignoring case, or v.
func canonical(allowed []string, v string) string {
	v = strings.TrimSpace(v)
	if i := slices.IndexFunc(allowed, func(a string) bool { return strings.EqualFold(a, v) }); i >= 0 {
		return allowed[i]
	}
	return v
}

func (s *Service) validate(ctx context.Context, admin, user resource.ID, c *Contact, verr *resource.ValidationError) {
	if c.CompanyName == "" && c.Firstname == "" && c.Lastname == "" {
		verr.Add("base", "Company name, first name or last name can't be blank")
	}
	texts := map[string]string{
		"company_name": c.CompanyName, "firstname": c.Firstname, "lastname": c.Lastname, "address1": c.Address1, "address2": c.Address2,
		"zipcode": c.Zipcode, "city": c.City, "phone": c.Phone, "tax_number": c.TaxNumber, "chamber_of_commerce": c.ChamberOfCommerce,
		"bank_account": c.BankAccount, "send_invoices_to_attention": c.SendInvoicesToAttention, "send_invoices_to_email": c.SendInvoicesToEmail,
		"send_estimates_to_attention": c.SendEstimatesToAttention, "send_estimates_to_email": c.SendEstimatesToEmail,
		"sepa_iban_account_name": c.SepaIBANAccountName, "sepa_mandate_id": c.SepaMandateID, "si_identifier": c.SIIdentifier,
	}
	for field, v := range texts {
		if len(v) > maxText {
			verr.Add(field, fmt.Sprintf("is too long (maximum is %d characters)", maxText))
		}
	}
	if !slices.Contains(administration.Countries(), c.Country) {
		verr.Add("country", "is not included in the list")
	}
	if c.CustomerID != "" && !customerIDPattern.MatchString(c.CustomerID) {
		verr.Add("customer_id", "is invalid")
	} else if c.CustomerID != "" {
		taken, err := s.store.CustomerIDTaken(ctx, admin, c.CustomerID, c.ID)
		if err != nil {
			verr.Add("customer_id", "could not be checked")
		} else if taken {
			verr.Add("customer_id", "has already been taken")
		}
	}
	if !slices.Contains(DeliveryMethods, c.DeliveryMethod) {
		verr.Add("delivery_method", "is not included in the list")
	}
	if slices.Contains(eInvoicingDelivery, c.DeliveryMethod) && (c.SIIdentifier == "" || c.SIIdentifierType == "") {
		verr.Add("si_identifier", "can't be blank when invoices are sent as e-invoices")
	}
	if c.SIIdentifierType != "" && !siTypePattern.MatchString(c.SIIdentifierType) {
		verr.Add("si_identifier_type", "is invalid")
	}
	for field, v := range map[string]string{"send_invoices_to_email": c.SendInvoicesToEmail, "send_estimates_to_email": c.SendEstimatesToEmail} {
		if !validEmails(v) {
			verr.Add(field, "is not a valid email address")
		}
	}
	if c.DeliveryMethod == "Email" && c.EmailUBL && c.SendInvoicesToEmail == "" {
		verr.Add("send_invoices_to_email", "can't be blank when invoices are sent by email")
	}
	if c.SepaIBAN != "" && !iban.Valid(c.SepaIBAN) {
		verr.Add("sepa_iban", "is not a valid IBAN")
	}
	if c.SepaBIC != "" && !bicPattern.MatchString(c.SepaBIC) {
		verr.Add("sepa_bic", "is not a valid BIC")
	}
	if !slices.Contains(SequenceTypes, c.SepaSequenceType) {
		verr.Add("sepa_sequence_type", "is not included in the list")
	}
	if c.SepaActive {
		required := map[string]bool{"sepa_iban": c.SepaIBAN == "", "sepa_iban_account_name": c.SepaIBANAccountName == "", "sepa_mandate_id": c.SepaMandateID == "", "sepa_mandate_date": c.SepaMandateDate == nil}
		for field, blank := range required {
			if blank {
				verr.Add(field, "can't be blank for a direct debit mandate")
			}
		}
	}
	s.checkWorkflow(ctx, admin, user, "invoice_workflow_id", "InvoiceWorkflow", c.InvoiceWorkflowID, verr)
	s.checkWorkflow(ctx, admin, user, "estimate_workflow_id", "EstimateWorkflow", c.EstimateWorkflowID, verr)
}

func validEmails(list string) bool {
	if list == "" {
		return true
	}
	for addr := range strings.SplitSeq(list, ",") {
		addr = strings.TrimSpace(addr)
		parsed, err := mail.ParseAddress(addr)
		if err != nil || parsed.Address != addr {
			return false
		}
	}
	return true
}

func (s *Service) checkWorkflow(ctx context.Context, admin, user resource.ID, field, kind string, id *resource.ID, verr *resource.ValidationError) {
	if id == nil {
		return
	}
	got, active, err := s.workflows.WorkflowKind(ctx, admin, *id, user)
	switch {
	case err != nil || got != kind:
		verr.Add(field, "does not exist")
	case !active:
		verr.Add(field, "is deactivated")
	}
}

func (s *Service) applyCustomFields(ctx context.Context, admin, user resource.ID, c *Contact, values []FieldValue, verr *resource.ValidationError) {
	fields, err := s.fields.CustomFields(ctx, admin, user)
	if err != nil {
		verr.Add("custom_fields_attributes", "could not be checked")
		return
	}
	names := map[resource.ID]string{}
	for _, f := range fields {
		if f.Source == contactFieldSources {
			names[f.ID] = f.Name
		}
	}
	current := slices.Clone(c.CustomFields)
	for i, v := range values {
		name, ok := names[v.FieldID]
		switch {
		case !ok:
			verr.Add(fmt.Sprintf("custom_fields_attributes.%d", i), "is not a custom field for contacts")
			continue
		case len(v.Value) > maxText:
			verr.Add(fmt.Sprintf("custom_fields_attributes.%d", i), fmt.Sprintf("value is too long (maximum is %d characters)", maxText))
			continue
		}
		value := strings.TrimSpace(v.Value)
		current = slices.DeleteFunc(current, func(e FieldValue) bool { return e.FieldID == v.FieldID })
		if value != "" {
			current = append(current, FieldValue{FieldID: v.FieldID, Name: name, Value: value})
		}
	}
	c.CustomFields = current
}

// NextCustomerID returns the customer id a new contact gets when none is
// given: one above the highest numeric one, as Moneybird prefills it.
func (s *Service) NextCustomerID(ctx context.Context, admin, user resource.ID) (string, error) {
	if _, err := s.admins.Get(ctx, admin, user); err != nil {
		return "", err
	}
	return s.nextCustomerID(ctx, admin)
}

func (s *Service) nextCustomerID(ctx context.Context, admin resource.ID) (string, error) {
	n, err := s.store.MaxCustomerNumber(ctx, admin)
	if err != nil {
		return "", fmt.Errorf("number contact: %w", err)
	}
	return strconv.FormatInt(n+1, 10), nil
}
