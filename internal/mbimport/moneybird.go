package mbimport

import (
	"bytes"
	"encoding/json"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/adirkuhn/moneyharpy/internal/banking"
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
	// FinancialAccountID is set on the category a financial account's money is in.
	FinancialAccountID *mbID `json:"financial_account_id"`
	TaxonomyItem       *struct {
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

type mbFinancialAccount struct {
	ID         mbID    `json:"id"`
	Type       string  `json:"type"`
	Name       *string `json:"name"`
	Identifier *string `json:"identifier"`
	Currency   *string `json:"currency"`
	Active     *bool   `json:"active"`
	stamps
}

func (a mbFinancialAccount) changes() banking.AccountChanges {
	kind := a.Type
	if !slices.Contains(banking.AccountTypes, kind) {
		// Moneybird's own kinds of account, such as its payment account, are custom accounts here.
		kind = banking.CustomAccount
	}
	return banking.AccountChanges{Type: &kind, Name: opt(a.Name), Identifier: opt(a.Identifier), Currency: opt(a.Currency), Active: a.Active}
}

type mbMutation struct {
	ID                  mbID              `json:"id"`
	FinancialAccountID  mbID              `json:"financial_account_id"`
	FinancialStatement  mbID              `json:"financial_statement_id"`
	Date                string            `json:"date"`
	Message             *string           `json:"message"`
	Amount              mbDecimal         `json:"amount"`
	Code                *string           `json:"code"`
	ContraAccountName   *string           `json:"contra_account_name"`
	ContraAccountNumber *string           `json:"contra_account_number"`
	BatchReference      *string           `json:"batch_reference"`
	TransactionID       *string           `json:"account_servicer_transaction_id"`
	Bookings            []mbLedgerBooking `json:"ledger_account_bookings"`
	stamps
}

func (m mbMutation) input() banking.MutationInput {
	amount := string(m.Amount)
	date := m.Date
	return banking.MutationInput{
		Date: &date, Message: m.Message, Amount: &amount, Code: opt(m.Code), ContraName: opt(m.ContraAccountName),
		ContraAccount: opt(m.ContraAccountNumber), BatchReference: opt(m.BatchReference), TransactionID: opt(m.TransactionID),
	}
}

type mbDetail struct {
	ID              mbID      `json:"id"`
	TaxRateID       *mbID     `json:"tax_rate_id"`
	LedgerAccountID *mbID     `json:"ledger_account_id"`
	ProjectID       *mbID     `json:"project_id"`
	ProductID       *mbID     `json:"product_id"`
	Amount          *string   `json:"amount"`
	Description     *string   `json:"description"`
	Price           mbDecimal `json:"price"`
	Period          *string   `json:"period"`
	RowOrder        *int      `json:"row_order"`
	IsOptional      *bool     `json:"is_optional"`
	IsSelected      *bool     `json:"is_selected"`
	stamps
}

type mbPayment struct {
	ID                    mbID      `json:"id"`
	PaymentDate           string    `json:"payment_date"`
	Price                 mbDecimal `json:"price"`
	PriceBase             mbDecimal `json:"price_base"`
	FinancialAccountID    *mbID     `json:"financial_account_id"`
	FinancialMutationID   *mbID     `json:"financial_mutation_id"`
	LedgerAccountID       *mbID     `json:"ledger_account_id"`
	CreditInvoiceID       *mbID     `json:"credit_invoice_id"`
	TransactionIdentifier *string   `json:"transaction_identifier"`
	ManualPaymentAction   *string   `json:"manual_payment_action"`
	stamps
}

type mbSalesInvoice struct {
	ID                     mbID           `json:"id"`
	ContactID              *mbID          `json:"contact_id"`
	ContactPersonID        *mbID          `json:"contact_person_id"`
	InvoiceID              *string        `json:"invoice_id"`
	DraftID                *int           `json:"draft_id"`
	State                  string         `json:"state"`
	InvoiceDate            *string        `json:"invoice_date"`
	DueDate                *string        `json:"due_date"`
	PaymentConditions      *string        `json:"payment_conditions"`
	PaymentReference       *string        `json:"payment_reference"`
	Reference              *string        `json:"reference"`
	Currency               *string        `json:"currency"`
	Discount               mbDecimal      `json:"discount"`
	OriginalSalesInvoiceID *mbID          `json:"original_sales_invoice_id"`
	SentAt                 *time.Time     `json:"sent_at"`
	PricesAreInclTax       *bool          `json:"prices_are_incl_tax"`
	MarkedDubiousOn        *string        `json:"marked_dubious_on"`
	MarkedUncollectibleOn  *string        `json:"marked_uncollectible_on"`
	OriginalEstimateID     *mbID          `json:"original_estimate_id"`
	RecurringID            *mbID          `json:"recurring_sales_invoice_id"`
	Details                []mbDetail     `json:"details"`
	Payments               []mbPayment    `json:"payments"`
	Attachments            []mbAttachment `json:"attachments"`
	stamps
}

// sent reports an invoice that was sent: a scheduled one was not yet.
func (i mbSalesInvoice) sent() bool { return i.State != "draft" && i.State != "scheduled" }

type mbExternalInvoice struct {
	ID                    mbID           `json:"id"`
	ContactID             *mbID          `json:"contact_id"`
	Date                  *string        `json:"date"`
	DueDate               *string        `json:"due_date"`
	Reference             *string        `json:"reference"`
	EntryNumber           *int           `json:"entry_number"`
	Source                *string        `json:"source"`
	SourceURL             *string        `json:"source_url"`
	Currency              *string        `json:"currency"`
	PricesAreInclTax      *bool          `json:"prices_are_incl_tax"`
	MarkedDubiousOn       *string        `json:"marked_dubious_on"`
	MarkedUncollectibleOn *string        `json:"marked_uncollectible_on"`
	Details               []mbDetail     `json:"details"`
	Payments              []mbPayment    `json:"payments"`
	Attachments           []mbAttachment `json:"attachments"`
	stamps
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

type mbAttachment struct {
	ID          mbID   `json:"id"`
	Filename    string `json:"filename"`
	ContentType string `json:"content_type"`
}

type mbLedgerBooking struct {
	ID              mbID      `json:"id"`
	LedgerAccountID *mbID     `json:"ledger_account_id"`
	ProjectID       *mbID     `json:"project_id"`
	Description     *string   `json:"description"`
	Price           mbDecimal `json:"price"`
}

type mbEstimate struct {
	ID               mbID           `json:"id"`
	ContactID        *mbID          `json:"contact_id"`
	ContactPersonID  *mbID          `json:"contact_person_id"`
	EstimateID       *string        `json:"estimate_id"`
	DraftID          *int           `json:"draft_id"`
	State            string         `json:"state"`
	EstimateDate     *string        `json:"estimate_date"`
	DueDate          *string        `json:"due_date"`
	Reference        *string        `json:"reference"`
	Language         *string        `json:"language"`
	Currency         *string        `json:"currency"`
	Discount         mbDecimal      `json:"discount"`
	ShowTax          *bool          `json:"show_tax"`
	PricesAreInclTax *bool          `json:"prices_are_incl_tax"`
	PreText          *string        `json:"pre_text"`
	PostText         *string        `json:"post_text"`
	SentAt           *time.Time     `json:"sent_at"`
	AcceptedAt       *time.Time     `json:"accepted_at"`
	RejectedAt       *time.Time     `json:"rejected_at"`
	ArchivedAt       *time.Time     `json:"archived_at"`
	Details          []mbDetail     `json:"details"`
	Attachments      []mbAttachment `json:"attachments"`
	stamps
}

type mbRecurring struct {
	ID                mbID       `json:"id"`
	ContactID         *mbID      `json:"contact_id"`
	ContactPersonID   *mbID      `json:"contact_person_id"`
	StartDate         *string    `json:"start_date"`
	InvoiceDate       *string    `json:"invoice_date"`
	LastDate          *string    `json:"last_date"`
	PaymentConditions *string    `json:"payment_conditions"`
	Reference         *string    `json:"reference"`
	Currency          *string    `json:"currency"`
	Discount          mbDecimal  `json:"discount"`
	FirstDueInterval  *int       `json:"first_due_interval"`
	FrequencyType     *string    `json:"frequency_type"`
	Frequency         *int       `json:"frequency"`
	PricesAreInclTax  *bool      `json:"prices_are_incl_tax"`
	Details           []mbDetail `json:"details"`
	stamps
}

type mbPurchase struct {
	ID               mbID           `json:"id"`
	ContactID        *mbID          `json:"contact_id"`
	Reference        *string        `json:"reference"`
	Date             *string        `json:"date"`
	DueDate          *string        `json:"due_date"`
	EntryNumber      *int           `json:"entry_number"`
	Currency         *string        `json:"currency"`
	RevenueInvoice   *bool          `json:"revenue_invoice"`
	PricesAreInclTax *bool          `json:"prices_are_incl_tax"`
	Details          []mbDetail     `json:"details"`
	Payments         []mbPayment    `json:"payments"`
	Attachments      []mbAttachment `json:"attachments"`
	stamps
}

type mbJournalEntry struct {
	ID              mbID      `json:"id"`
	LedgerAccountID *mbID     `json:"ledger_account_id"`
	ProjectID       *mbID     `json:"project_id"`
	ContactID       *mbID     `json:"contact_id"`
	Debit           mbDecimal `json:"debit"`
	Credit          mbDecimal `json:"credit"`
	RowOrder        *int      `json:"row_order"`
	Description     *string   `json:"description"`
}

type mbJournal struct {
	ID          mbID             `json:"id"`
	Reference   *string          `json:"reference"`
	Date        *string          `json:"date"`
	JournalType *string          `json:"journal_type"`
	Entries     []mbJournalEntry `json:"general_journal_document_entries"`
	Attachments []mbAttachment   `json:"attachments"`
	stamps
}

// mbDocumentBase is what general and typeless documents share.
type mbDocumentBase struct {
	ID          mbID           `json:"id"`
	ContactID   *mbID          `json:"contact_id"`
	Reference   *string        `json:"reference"`
	Date        *string        `json:"date"`
	DueDate     *string        `json:"due_date"`
	EntryNumber *int           `json:"entry_number"`
	Attachments []mbAttachment `json:"attachments"`
	stamps
}
