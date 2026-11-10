// Package contacts holds the customers and suppliers an administration
// invoices and pays: companies and private individuals with their
// addresses, delivery preferences, direct debit mandates, contact people
// and notes, after Moneybird's contacts.
package contacts

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/adirkuhn/moneyharpy/internal/administration"
	"github.com/adirkuhn/moneyharpy/internal/resource"
)

// Errors returned by Service.
var (
	ErrNotFound       = errors.New("contact not found")
	ErrPersonNotFound = errors.New("contact person not found")
	ErrNoteNotFound   = errors.New("note not found")
	// ErrInUse reports a contact that documents refer to; archive it instead.
	ErrInUse = errors.New("contact is in use")
)

// IsNotFound reports whether err means a contact resource is missing.
func IsNotFound(err error) bool {
	return errors.Is(err, ErrNotFound) || errors.Is(err, ErrPersonNotFound) || errors.Is(err, ErrNoteNotFound)
}

// Ways invoices reach a contact, with Moneybird's names. Simplerinvoicing
// and Peppol send e-invoices and need the contact's e-invoicing identifier.
var DeliveryMethods = []string{"Email", "Post", "Manual", "Simplerinvoicing", "Peppol"}

// SequenceTypes are the SEPA direct debit sequence types.
var SequenceTypes = []string{"RCUR", "FRST", "OOFF", "FNAL"}

// Contact is a customer or supplier.
type Contact struct {
	ID               resource.ID
	AdministrationID resource.ID
	CompanyName      string
	Firstname        string
	Lastname         string
	Address1         string
	Address2         string
	Zipcode          string
	City             string
	Country          string
	Phone            string
	// CustomerID is the contact's number, unique in the administration.
	CustomerID        string
	TaxNumber         string
	ChamberOfCommerce string
	BankAccount       string
	DeliveryMethod    string

	SendInvoicesToAttention  string
	SendInvoicesToEmail      string
	SendEstimatesToAttention string
	SendEstimatesToEmail     string
	EmailUBL                 bool

	DirectDebit         bool
	SepaActive          bool
	SepaIBAN            string
	SepaIBANAccountName string
	SepaBIC             string
	SepaMandateID       string
	SepaMandateDate     *time.Time
	SepaSequenceType    string

	// SIIdentifier is the contact's e-invoicing (Peppol) identifier.
	SIIdentifierType string
	SIIdentifier     string

	InvoiceWorkflowID  *resource.ID
	EstimateWorkflowID *resource.ID

	Archived     bool
	CustomFields []FieldValue
	People       []Person
	Notes        []Note
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// Ref locates the contact.
func (c Contact) Ref() resource.Ref {
	return resource.Ref{ID: c.ID, Type: resource.TypeContact, AdministrationID: c.AdministrationID}
}

// Name is what lists show: the company, else the person's name.
func (c Contact) Name() string {
	if c.CompanyName != "" {
		return c.CompanyName
	}
	return joinName(c.Firstname, c.Lastname)
}

// Version changes whenever the contact, its people or notes change.
func (c Contact) Version() int64 { return c.UpdatedAt.Unix() }

func joinName(first, last string) string {
	switch {
	case first == "":
		return last
	case last == "":
		return first
	}
	return first + " " + last
}

// FieldValue is a contact's value for one of the administration's custom
// fields.
type FieldValue struct {
	FieldID resource.ID
	Name    string
	Value   string
}

// Person is someone to talk to at a contact.
type Person struct {
	ID               resource.ID
	AdministrationID resource.ID
	ContactID        resource.ID
	Firstname        string
	Lastname         string
	Phone            string
	Email            string
	Department       string
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

// Version changes whenever the person changes.
func (p Person) Version() int64 { return p.UpdatedAt.Unix() }

// Note is a remark or a to-do on a record. Notes are kept per entity, so
// invoices and other records can carry them too.
type Note struct {
	ID               resource.ID
	AdministrationID resource.ID
	EntityID         resource.ID
	EntityType       string
	UserID           resource.ID
	AssigneeID       *resource.ID
	Todo             bool
	Note             string
	CompletedAt      *time.Time
	CompletedByID    *resource.ID
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

// Version is a contact's id with its version, for synchronization.
type Version struct {
	ID      resource.ID
	Version int64
}

// Administrations gives access to administrations; Get returns
// administration.ErrNotFound when the user has no access.
type Administrations interface {
	Get(ctx context.Context, id, user resource.ID) (administration.Listed, error)
}

// Workflows tells what kind of workflow an id is.
type Workflows interface {
	// WorkflowKind returns the workflow's type (InvoiceWorkflow or
	// EstimateWorkflow) and whether it is active, or an error when it does
	// not exist.
	WorkflowKind(ctx context.Context, admin, id, user resource.ID) (kind string, active bool, err error)
}

// CustomFields lists the administration's custom fields.
type CustomFields interface {
	CustomFields(ctx context.Context, admin, user resource.ID) ([]administration.CustomField, error)
}

// References reports whether documents refer to a contact. Invoices and
// other documents register one as they arrive.
type References interface {
	ContactInUse(ctx context.Context, admin, contact resource.ID) (bool, error)
}

// Store persists contacts. Methods taking an administration trust the
// caller to have checked access.
type Store interface {
	// Contacts returns one page of the contacts the query selects, ordered
	// by name, with the number of contacts it selects in all. A PerPage of
	// zero returns every contact.
	Contacts(ctx context.Context, admin resource.ID, q Query) ([]Contact, int, error)
	Contact(ctx context.Context, admin, id resource.ID) (Contact, error)
	ContactByCustomerID(ctx context.Context, admin resource.ID, customerID string) (Contact, error)
	ContactsByID(ctx context.Context, admin resource.ID, ids []resource.ID) ([]Contact, error)
	Versions(ctx context.Context, admin resource.ID, q Query) ([]Version, error)
	// MaxCustomerNumber returns the highest customer id that is a number.
	MaxCustomerNumber(ctx context.Context, admin resource.ID) (int64, error)
	CustomerIDTaken(ctx context.Context, admin resource.ID, customerID string, except resource.ID) (bool, error)

	CreateContact(ctx context.Context, c Contact) error
	UpdateContact(ctx context.Context, c Contact) error
	DeleteContact(ctx context.Context, admin, id resource.ID) error

	// SavePerson stores a new or changed person and stamps the contact.
	SavePerson(ctx context.Context, p Person) error
	DeletePerson(ctx context.Context, admin, contact, id resource.ID, at time.Time) error

	// CreateNote stores a note and stamps the contact.
	CreateNote(ctx context.Context, n Note) error
	DeleteNote(ctx context.Context, admin, entity, id resource.ID, at time.Time) error
	IsMember(ctx context.Context, admin, user resource.ID) (bool, error)
}

// Service implements the contact use cases.
type Service struct {
	admins     Administrations
	workflows  Workflows
	fields     CustomFields
	references References
	store      Store
	ids        *resource.Generator
	now        func() time.Time
}

// NewService returns a Service. References may be nil while no document
// can refer to contacts.
func NewService(admins Administrations, workflows Workflows, fields CustomFields, references References, store Store, ids *resource.Generator, now func() time.Time) *Service {
	return &Service{admins: admins, workflows: workflows, fields: fields, references: references, store: store, ids: ids, now: now}
}

// MaxPerPage caps a page, as at Moneybird.
const (
	MaxPerPage     = 100
	DefaultPerPage = 50
	// MaxFetch caps the contacts fetched by id at once.
	MaxFetch = 100
)

// Page is one page of contacts with the number selected in all.
type Page struct {
	Contacts []Contact
	Total    int
	Page     int
	PerPage  int
}

// List returns the page of contacts the query selects.
func (s *Service) List(ctx context.Context, admin, user resource.ID, q Query) (Page, error) {
	if _, err := s.admins.Get(ctx, admin, user); err != nil {
		return Page{}, err
	}
	q.Page = max(q.Page, 1)
	if q.PerPage <= 0 {
		q.PerPage = DefaultPerPage
	}
	q.PerPage = min(q.PerPage, MaxPerPage)
	list, total, err := s.store.Contacts(ctx, admin, q)
	if err != nil {
		return Page{}, fmt.Errorf("list contacts: %w", err)
	}
	return Page{Contacts: list, Total: total, Page: q.Page, PerPage: q.PerPage}, nil
}

// Contact returns one contact, archived or not.
func (s *Service) Contact(ctx context.Context, admin, id, user resource.ID) (Contact, error) {
	if _, err := s.admins.Get(ctx, admin, user); err != nil {
		return Contact{}, err
	}
	return s.store.Contact(ctx, admin, id)
}

// ContactByCustomerID returns the contact with a customer id.
func (s *Service) ContactByCustomerID(ctx context.Context, admin, user resource.ID, customerID string) (Contact, error) {
	if _, err := s.admins.Get(ctx, admin, user); err != nil {
		return Contact{}, err
	}
	return s.store.ContactByCustomerID(ctx, admin, customerID)
}

// ContactsByID returns the contacts with these ids, for synchronization.
func (s *Service) ContactsByID(ctx context.Context, admin, user resource.ID, ids []resource.ID) ([]Contact, error) {
	if _, err := s.admins.Get(ctx, admin, user); err != nil {
		return nil, err
	}
	if len(ids) > MaxFetch {
		var verr resource.ValidationError
		verr.Add("ids", fmt.Sprintf("can hold at most %d ids", MaxFetch))
		return nil, &verr
	}
	return s.store.ContactsByID(ctx, admin, ids)
}

// Versions lists the ids and versions of the contacts the query selects.
func (s *Service) Versions(ctx context.Context, admin, user resource.ID, q Query) ([]Version, error) {
	if _, err := s.admins.Get(ctx, admin, user); err != nil {
		return nil, err
	}
	return s.store.Versions(ctx, admin, q)
}
