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
	"github.com/adirkuhn/moneyharpy/internal/notes"
	"github.com/adirkuhn/moneyharpy/internal/resource"
)

// Errors returned by Service.
var (
	ErrNotFound       = errors.New("contact not found")
	ErrPersonNotFound = errors.New("contact person not found")
	ErrNoteNotFound   = errors.New("note not found")
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

// Note is a remark or a to-do on a contact; see notes.Note.
type Note = notes.Note

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

	// TouchContact stamps a contact changed, such as when a note is added.
	TouchContact(ctx context.Context, admin, id resource.ID, at time.Time) error
}

// Service implements the contact use cases.
type Service struct {
	admins     Administrations
	workflows  Workflows
	fields     CustomFields
	references References
	notes      *notes.Service
	store      Store
	ids        *resource.Generator
	now        func() time.Time
}

// NewService returns a Service. References may be nil while no document
// can refer to contacts.
func NewService(admins Administrations, workflows Workflows, fields CustomFields, references References, notes *notes.Service, store Store, ids *resource.Generator, now func() time.Time) *Service {
	return &Service{admins: admins, workflows: workflows, fields: fields, references: references, notes: notes, store: store, ids: ids, now: now}
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
	if list, err = s.withNotes(ctx, admin, list); err != nil {
		return Page{}, err
	}
	return Page{Contacts: list, Total: total, Page: q.Page, PerPage: q.PerPage}, nil
}

// Contact returns one contact, archived or not.
func (s *Service) Contact(ctx context.Context, admin, id, user resource.ID) (Contact, error) {
	if _, err := s.admins.Get(ctx, admin, user); err != nil {
		return Contact{}, err
	}
	return s.one(ctx, admin, func() (Contact, error) { return s.store.Contact(ctx, admin, id) })
}

func (s *Service) one(ctx context.Context, admin resource.ID, load func() (Contact, error)) (Contact, error) {
	c, err := load()
	if err != nil {
		return Contact{}, err
	}
	list, err := s.withNotes(ctx, admin, []Contact{c})
	if err != nil {
		return Contact{}, err
	}
	return list[0], nil
}

// withNotes adds each contact's notes.
func (s *Service) withNotes(ctx context.Context, admin resource.ID, list []Contact) ([]Contact, error) {
	ids := make([]resource.ID, len(list))
	for i, c := range list {
		ids[i] = c.ID
	}
	byContact, err := s.notes.Of(ctx, admin, NoteEntityType, ids)
	if err != nil {
		return nil, err
	}
	for i := range list {
		list[i].Notes = byContact[list[i].ID]
	}
	return list, nil
}

// ContactExists returns an error unless the contact is the administration's,
// for records that refer to one.
func (s *Service) ContactExists(ctx context.Context, admin, id, user resource.ID) error {
	if _, err := s.admins.Get(ctx, admin, user); err != nil {
		return err
	}
	_, err := s.store.Contact(ctx, admin, id)
	return err
}

// ContactByCustomerID returns the contact with a customer id.
func (s *Service) ContactByCustomerID(ctx context.Context, admin, user resource.ID, customerID string) (Contact, error) {
	if _, err := s.admins.Get(ctx, admin, user); err != nil {
		return Contact{}, err
	}
	return s.one(ctx, admin, func() (Contact, error) { return s.store.ContactByCustomerID(ctx, admin, customerID) })
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
	list, err := s.store.ContactsByID(ctx, admin, ids)
	if err != nil {
		return nil, err
	}
	return s.withNotes(ctx, admin, list)
}

// Versions lists the ids and versions of the contacts the query selects.
func (s *Service) Versions(ctx context.Context, admin, user resource.ID, q Query) ([]Version, error) {
	if _, err := s.admins.Get(ctx, admin, user); err != nil {
		return nil, err
	}
	return s.store.Versions(ctx, admin, q)
}

// AnyReferences reports a contact in use when any of its references does.
type AnyReferences []References

// ContactInUse implements References.
func (refs AnyReferences) ContactInUse(ctx context.Context, admin, contact resource.ID) (bool, error) {
	for _, r := range refs {
		used, err := r.ContactInUse(ctx, admin, contact)
		if err != nil || used {
			return used, err
		}
	}
	return false, nil
}
