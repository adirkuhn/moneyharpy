package mbimport

import (
	"bytes"
	"encoding/json"
	"strconv"
	"strings"
	"time"

	"github.com/adirkuhn/moneyharpy/internal/bookkeeping"
	"github.com/adirkuhn/moneyharpy/internal/contacts"
	"github.com/adirkuhn/moneyharpy/internal/products"
	"github.com/adirkuhn/moneyharpy/internal/projects"
	"github.com/adirkuhn/moneyharpy/internal/resource"
)

// mbID reads a Moneybird id, which its API writes as a string or a number.
type mbID resource.ID

// UnmarshalJSON implements json.Unmarshaler; null and "" read as no id.
func (m *mbID) UnmarshalJSON(b []byte) error {
	s := strings.Trim(string(bytes.TrimSpace(b)), `"`)
	if s == "" || s == "null" {
		*m = 0
		return nil
	}
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return &json.UnmarshalTypeError{Value: s, Type: nil}
	}
	*m = mbID(n)
	return nil
}

func (m mbID) id() resource.ID { return resource.ID(m) }

// ref turns an optional Moneybird reference into an id, nil when absent.
func ref(m *mbID) *resource.ID {
	if m == nil || *m == 0 {
		return nil
	}
	id := m.id()
	return &id
}

// mbDecimal reads an amount Moneybird writes as a string or a number.
type mbDecimal string

// UnmarshalJSON implements json.Unmarshaler.
func (d *mbDecimal) UnmarshalJSON(b []byte) error {
	s := strings.Trim(string(bytes.TrimSpace(b)), `"`)
	if s == "null" {
		s = ""
	}
	*d = mbDecimal(s)
	return nil
}

// stamps are the times Moneybird keeps on every record.
type stamps struct {
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func origin(id mbID, s stamps) resource.Origin {
	return resource.Origin{ID: id.id(), CreatedAt: s.CreatedAt, UpdatedAt: s.UpdatedAt}
}

// opt keeps a text only when Moneybird has one, so empty fields stay unset.
func opt(s *string) *string {
	if s == nil || strings.TrimSpace(*s) == "" {
		return nil
	}
	return s
}

type mbLedgerAccount struct {
	ID                   mbID     `json:"id"`
	Name                 string   `json:"name"`
	AccountType          string   `json:"account_type"`
	AccountID            *string  `json:"account_id"`
	ParentID             *mbID    `json:"parent_id"`
	Active               bool     `json:"active"`
	AllowedDocumentTypes []string `json:"allowed_document_types"`
	Description          *string  `json:"description"`
	TaxonomyItem         *struct {
		Code string `json:"code"`
	} `json:"taxonomy_item"`
	stamps
}

func (a mbLedgerAccount) imported() bookkeeping.ImportedAccount {
	out := bookkeeping.ImportedAccount{
		Origin: origin(a.ID, a.stamps), Name: a.Name, Type: a.AccountType, ParentID: ref(a.ParentID), Active: a.Active,
		AllowedDocumentTypes: a.AllowedDocumentTypes,
	}
	if a.AccountID != nil {
		out.Code = *a.AccountID
	}
	if a.Description != nil {
		out.Description = *a.Description
	}
	if a.TaxonomyItem != nil {
		out.Taxonomy = a.TaxonomyItem.Code
	}
	return out
}

type mbTaxRate struct {
	ID          mbID      `json:"id"`
	Name        string    `json:"name"`
	Percentage  mbDecimal `json:"percentage"`
	TaxRateType string    `json:"tax_rate_type"`
	ShowTax     bool      `json:"show_tax"`
	Active      bool      `json:"active"`
	Country     *string   `json:"country"`
	stamps
}

func (t mbTaxRate) imported() bookkeeping.ImportedRate {
	out := bookkeeping.ImportedRate{
		Origin: origin(t.ID, t.stamps), Name: t.Name, Percentage: string(t.Percentage), Type: t.TaxRateType,
		ShowTax: t.ShowTax, Active: t.Active,
	}
	if t.Country != nil {
		out.Country = *t.Country
	}
	return out
}

type mbContactPerson struct {
	ID         mbID    `json:"id"`
	Firstname  *string `json:"firstname"`
	Lastname   *string `json:"lastname"`
	Phone      *string `json:"phone"`
	Email      *string `json:"email"`
	Department *string `json:"department"`
	stamps
}

type mbContact struct {
	ID                       mbID              `json:"id"`
	CompanyName              *string           `json:"company_name"`
	Firstname                *string           `json:"firstname"`
	Lastname                 *string           `json:"lastname"`
	Address1                 *string           `json:"address1"`
	Address2                 *string           `json:"address2"`
	Zipcode                  *string           `json:"zipcode"`
	City                     *string           `json:"city"`
	Country                  *string           `json:"country"`
	Phone                    *string           `json:"phone"`
	CustomerID               *string           `json:"customer_id"`
	TaxNumber                *string           `json:"tax_number"`
	ChamberOfCommerce        *string           `json:"chamber_of_commerce"`
	BankAccount              *string           `json:"bank_account"`
	DeliveryMethod           *string           `json:"delivery_method"`
	EmailUBL                 *bool             `json:"email_ubl"`
	SendInvoicesToAttention  *string           `json:"send_invoices_to_attention"`
	SendInvoicesToEmail      *string           `json:"send_invoices_to_email"`
	SendEstimatesToAttention *string           `json:"send_estimates_to_attention"`
	SendEstimatesToEmail     *string           `json:"send_estimates_to_email"`
	DirectDebit              *bool             `json:"direct_debit"`
	SepaActive               *bool             `json:"sepa_active"`
	SepaIBAN                 *string           `json:"sepa_iban"`
	SepaIBANAccountName      *string           `json:"sepa_iban_account_name"`
	SepaBIC                  *string           `json:"sepa_bic"`
	SepaMandateID            *string           `json:"sepa_mandate_id"`
	SepaMandateDate          *string           `json:"sepa_mandate_date"`
	SepaSequenceType         *string           `json:"sepa_sequence_type"`
	SIIdentifier             *string           `json:"si_identifier"`
	SIIdentifierType         *string           `json:"si_identifier_type"`
	Archived                 *bool             `json:"archived"`
	ContactPeople            []mbContactPerson `json:"contact_people"`
	stamps
}

func (c mbContact) origin() resource.Origin { return origin(c.ID, c.stamps) }

// changes are the contact's fields. Its workflows and custom fields come
// with the invoice settings, later.
func (c mbContact) changes() contacts.Changes {
	return contacts.Changes{
		CompanyName: opt(c.CompanyName), Firstname: opt(c.Firstname), Lastname: opt(c.Lastname),
		Address1: opt(c.Address1), Address2: opt(c.Address2), Zipcode: opt(c.Zipcode), City: opt(c.City), Country: opt(c.Country),
		Phone: opt(c.Phone), CustomerID: opt(c.CustomerID), TaxNumber: opt(c.TaxNumber), ChamberOfCommerce: opt(c.ChamberOfCommerce),
		BankAccount: opt(c.BankAccount), DeliveryMethod: opt(c.DeliveryMethod), EmailUBL: c.EmailUBL,
		SendInvoicesToAttention: opt(c.SendInvoicesToAttention), SendInvoicesToEmail: opt(c.SendInvoicesToEmail),
		SendEstimatesToAttention: opt(c.SendEstimatesToAttention), SendEstimatesToEmail: opt(c.SendEstimatesToEmail),
		DirectDebit: c.DirectDebit, SepaActive: c.SepaActive, SepaIBAN: opt(c.SepaIBAN), SepaIBANAccountName: opt(c.SepaIBANAccountName),
		SepaBIC: opt(c.SepaBIC), SepaMandateID: opt(c.SepaMandateID), SepaMandateDate: opt(c.SepaMandateDate),
		SepaSequenceType: opt(c.SepaSequenceType), SIIdentifier: opt(c.SIIdentifier), SIIdentifierType: opt(c.SIIdentifierType),
		Archived: c.Archived,
	}
}

func (c mbContact) people() []contacts.ImportedPerson {
	out := make([]contacts.ImportedPerson, 0, len(c.ContactPeople))
	for _, p := range c.ContactPeople {
		out = append(out, contacts.ImportedPerson{
			Origin: origin(p.ID, p.stamps),
			Changes: contacts.PersonChanges{
				Firstname: opt(p.Firstname), Lastname: opt(p.Lastname), Phone: opt(p.Phone), Email: opt(p.Email), Department: opt(p.Department),
			},
		})
	}
	return out
}

type mbProduct struct {
	ID              mbID      `json:"id"`
	Title           *string   `json:"title"`
	Description     *string   `json:"description"`
	Identifier      *string   `json:"identifier"`
	Price           mbDecimal `json:"price"`
	Currency        *string   `json:"currency"`
	Frequency       *int      `json:"frequency"`
	FrequencyType   *string   `json:"frequency_type"`
	TaxRateID       *mbID     `json:"tax_rate_id"`
	LedgerAccountID *mbID     `json:"ledger_account_id"`
	stamps
}

func (p mbProduct) origin() resource.Origin { return origin(p.ID, p.stamps) }

// changes are the product's fields; its VAT rate and category are mapped
// to the ids here by the step.
func (p mbProduct) changes() products.Changes {
	price := string(p.Price)
	c := products.Changes{
		Title: opt(p.Title), Description: opt(p.Description), Identifier: opt(p.Identifier), Currency: opt(p.Currency),
		Frequency: p.Frequency, FrequencyType: opt(p.FrequencyType),
	}
	if price != "" {
		c.Price = &price
	}
	return c
}

type mbProject struct {
	ID     mbID      `json:"id"`
	Name   *string   `json:"name"`
	State  *string   `json:"state"`
	Budget mbDecimal `json:"budget"`
	stamps
}

func (p mbProject) origin() resource.Origin { return origin(p.ID, p.stamps) }

func (p mbProject) changes() projects.Changes {
	c := projects.Changes{Name: p.Name, State: opt(p.State)}
	if b := string(p.Budget); b != "" {
		c.Budget = &b
	}
	return c
}
