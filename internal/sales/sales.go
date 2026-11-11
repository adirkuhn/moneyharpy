// Package sales holds an administration's sales invoices: drafts with their
// lines, sending them (numbering, the PDF, email and e-invoice), the
// bookings they post, credit invoices, and the notes and files on them,
// after Moneybird's sales invoices.
package sales

import (
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/adirkuhn/moneyharpy/internal/administration"
	"github.com/adirkuhn/moneyharpy/internal/attachments"
	"github.com/adirkuhn/moneyharpy/internal/bookkeeping"
	"github.com/adirkuhn/moneyharpy/internal/contacts"
	"github.com/adirkuhn/moneyharpy/internal/invoicing"
	"github.com/adirkuhn/moneyharpy/internal/journal"
	"github.com/adirkuhn/moneyharpy/internal/money"
	"github.com/adirkuhn/moneyharpy/internal/notes"
	"github.com/adirkuhn/moneyharpy/internal/resource"
)

// Errors returned by Service.
var (
	ErrNotFound = errors.New("sales invoice not found")
	// ErrNotEditable reports an invoice whose state no longer allows the
	// change, such as editing a paid invoice or deleting a sent one.
	ErrNotEditable = errors.New("sales invoice cannot be changed in this state")
	// ErrDeliveryUnavailable reports a way of sending MoneyHarpy cannot use
	// yet, such as the Peppol network.
	ErrDeliveryUnavailable = errors.New("delivery method not available yet")
	// ErrDelivery reports an invoice that was sent but could not be
	// delivered; it stays open and can be sent again.
	ErrDelivery = errors.New("sales invoice could not be delivered")
)

// IsNotFound reports whether err means a sales invoice resource is missing.
func IsNotFound(err error) bool {
	return errors.Is(err, ErrNotFound) || errors.Is(err, notes.ErrNotFound) || errors.Is(err, attachments.ErrNotFound)
}

// States, with Moneybird's names.
const (
	Draft          = "draft"
	Scheduled      = "scheduled"
	Open           = "open"
	PendingPayment = "pending_payment"
	Late           = "late"
	Reminded       = "reminded"
	Paid           = "paid"
	Uncollectible  = "uncollectible"
)

// States are every state, for filters.
var States = []string{Draft, Scheduled, Open, PendingPayment, Late, Reminded, Paid, Uncollectible}

// EntityType names sales invoices in notes, attachments and the journal.
const EntityType = journal.SalesInvoice

// Invoice is a sales invoice.
type Invoice struct {
	ID               resource.ID
	AdministrationID resource.ID
	ContactID        resource.ID
	ContactPersonID  *resource.ID
	IdentityID       resource.ID
	DocumentStyleID  resource.ID
	WorkflowID       resource.ID
	// DraftID numbers drafts; InvoiceID is the number given when sent.
	DraftID   int
	InvoiceID string
	// SequenceID overrides the {id} part of the number when sending.
	SequenceID        string
	Reference         string
	Language          string
	Currency          string
	PricesAreInclTax  bool
	PaymentConditions string
	// Discount is a percentage in hundredths.
	Discount              int64
	InvoiceDate           *time.Time
	DueDate               *time.Time
	FirstDueInterval      int
	State                 string
	Paused                bool
	PaymentReference      string
	SentAt                *time.Time
	PaidAt                *time.Time
	MarkedDubiousOn       *time.Time
	MarkedUncollectibleOn *time.Time
	ReminderCount         int
	// OriginalID is the invoice a credit invoice credits.
	OriginalID              *resource.ID
	RecurringSalesInvoiceID *resource.ID
	// Delivery remembers how a scheduled invoice will be sent.
	Delivery     *Delivery
	Lines        []Detail
	CustomFields []FieldValue
	Notes        []notes.Note
	Attachments  []attachments.Attachment
	Payments     []Payment
	// Totals are the amounts worked out when the invoice was last saved.
	Totals    Totals
	CreatedAt time.Time
	UpdatedAt time.Time
}

// Ref locates the invoice.
func (i Invoice) Ref() resource.Ref {
	return resource.Ref{ID: i.ID, Type: resource.TypeSalesInvoice, AdministrationID: i.AdministrationID}
}

// Version changes whenever the invoice does.
func (i Invoice) Version() int64 { return i.UpdatedAt.Unix() }

// Sent reports an invoice that left draft, and is booked.
func (i Invoice) Sent() bool { return i.State != Draft && i.State != Scheduled }

// Paid is what payments settled.
func (i Invoice) Paid() money.Amount {
	var sum money.Amount
	for _, p := range i.Payments {
		sum += p.Price
	}
	return sum
}

// Detail is one line of an invoice.
type Detail struct {
	ID          resource.ID
	Description string
	// Period is the range of days the line is about, as yyyymmdd..yyyymmdd.
	Period string
	Price  money.Amount
	// Amount is the quantity as people wrote it, such as "2 hours";
	// Quantity is its number in thousandths.
	Amount          string
	Quantity        int64
	TaxRateID       resource.ID
	LedgerAccountID *resource.ID
	ProjectID       *resource.ID
	ProductID       *resource.ID
	RowOrder        int
	Optional        bool
	Selected        bool
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

// FieldValue is the invoice's value of a custom field.
type FieldValue struct {
	FieldID resource.ID
	Name    string
	Value   string
}

// Payment settles (part of) an invoice; registering them comes with the
// payments of phase two.
type Payment struct {
	ID          resource.ID
	PaymentDate time.Time
	Price       money.Amount
}

// Delivery is how an invoice is sent.
type Delivery struct {
	// Method is Email, Manual, Post, Peppol or Simplerinvoicing.
	Method       string
	EmailAddress string
	EmailMessage string
	DeliverUBL   bool
	Mergeable    bool
}

// Administrations gives access to administrations and their identities.
type Administrations interface {
	Get(ctx context.Context, id, user resource.ID) (administration.Listed, error)
	DefaultIdentity(ctx context.Context, admin, user resource.ID) (administration.Identity, error)
	Identity(ctx context.Context, admin, id, user resource.ID) (administration.Identity, error)
	CustomFields(ctx context.Context, admin, user resource.ID) ([]administration.CustomField, error)
}

// Contacts gives the invoice's customer.
type Contacts interface {
	Contact(ctx context.Context, admin, id, user resource.ID) (contacts.Contact, error)
}

// Invoicing gives the settings invoices follow.
type Invoicing interface {
	Settings(ctx context.Context, admin, user resource.ID) (invoicing.SettingsView, error)
	Workflow(ctx context.Context, admin, id, user resource.ID) (invoicing.Workflow, error)
	Workflows(ctx context.Context, admin, user resource.ID) ([]invoicing.Workflow, error)
	DocumentStyle(ctx context.Context, admin, id, user resource.ID) (invoicing.DocumentStyle, error)
	DocumentStyles(ctx context.Context, admin, user resource.ID) ([]invoicing.DocumentStyle, error)
	OpenFile(ctx context.Context, admin, id, user resource.ID, kind invoicing.FileKind) (io.ReadCloser, string, error)
}

// Ledger is the part of bookkeeping invoices rely on.
type Ledger interface {
	Bookable(ctx context.Context, admin, user, id resource.ID, documentType string) (string, error)
	SystemLedgerAccount(ctx context.Context, admin, user resource.ID, role string) (resource.ID, error)
	TaxRate(ctx context.Context, admin, id, user resource.ID) (bookkeeping.TaxRate, error)
	LockedUntil(ctx context.Context, admin, user resource.ID) (string, error)
}

// Notes keeps notes on invoices.
type Notes interface {
	Add(ctx context.Context, admin resource.ID, entityType string, entity, user resource.ID, in notes.Input) (notes.Note, error)
	Delete(ctx context.Context, admin resource.ID, entityType string, entity, id resource.ID) error
	Of(ctx context.Context, admin resource.ID, entityType string, entities []resource.ID) (map[resource.ID][]notes.Note, error)
}

// Attachments keeps files on invoices.
type Attachments interface {
	Add(ctx context.Context, admin resource.ID, entityType string, entity resource.ID, filename, contentType string, data []byte) (attachments.Attachment, error)
	Open(ctx context.Context, admin resource.ID, entityType string, entity, id resource.ID) (attachments.Attachment, io.ReadCloser, error)
	Delete(ctx context.Context, admin resource.ID, entityType string, entity, id resource.ID) error
	DeleteAll(ctx context.Context, admin resource.ID, entityType string, entity resource.ID) error
	Of(ctx context.Context, admin resource.ID, entityType string, entities []resource.ID) (map[resource.ID][]attachments.Attachment, error)
}

// Store persists invoices with what they post.
type Store interface {
	Invoices(ctx context.Context, admin resource.ID, q Query) ([]Invoice, int, error)
	Invoice(ctx context.Context, admin, id resource.ID) (Invoice, error)
	InvoicesByID(ctx context.Context, admin resource.ID, ids []resource.ID) ([]Invoice, error)
	InvoiceByNumber(ctx context.Context, admin resource.ID, number string) (Invoice, error)
	InvoiceByReference(ctx context.Context, admin resource.ID, reference string) (Invoice, error)
	NextDraftID(ctx context.Context, admin resource.ID) (int, error)
	// ReserveNumber takes the next invoice sequence number and moves the
	// counter on, at once, so two invoices never share a number.
	ReserveNumber(ctx context.Context, admin resource.ID) (string, error)
	NumberTaken(ctx context.Context, admin resource.ID, number string, except resource.ID) (bool, error)
	// SaveInvoice stores the invoice and replaces what it posts.
	SaveInvoice(ctx context.Context, inv Invoice, entries []journal.Entry, created bool) error
	DeleteInvoice(ctx context.Context, admin, id resource.ID) error
	TouchInvoice(ctx context.Context, admin, id resource.ID, at time.Time) error
	ContactInUse(ctx context.Context, admin, contact resource.ID) (bool, error)
}

// Renderer turns an invoice into its documents.
type Renderer interface {
	PDF(doc Document) ([]byte, error)
	PackingSlip(doc Document) ([]byte, error)
	UBL(doc Document) ([]byte, error)
}

// Mailer sends invoices by email.
type Mailer interface {
	SendDocument(ctx context.Context, m Mail) error
}

// Mail is an invoice email with its files.
type Mail struct {
	To, BCC       []string
	ReplyTo       string
	FromName      string
	Subject, Body string
	Files         []File
}

// File is a document attached to an email.
type File struct {
	Name, ContentType string
	Data              []byte
}

// Service implements the sales invoice use cases.
type Service struct {
	admins      Administrations
	contacts    Contacts
	invoicing   Invoicing
	ledger      Ledger
	notes       Notes
	attachments Attachments
	store       Store
	renderer    Renderer
	mailer      Mailer
	ids         *resource.Generator
	now         func() time.Time
}

// Deps are what a Service needs.
type Deps struct {
	Administrations Administrations
	Contacts        Contacts
	Invoicing       Invoicing
	Ledger          Ledger
	Notes           Notes
	Attachments     Attachments
	Store           Store
	Renderer        Renderer
	Mailer          Mailer
	IDs             *resource.Generator
	Now             func() time.Time
}

// NewService returns a Service.
func NewService(d Deps) *Service {
	return &Service{
		admins: d.Administrations, contacts: d.Contacts, invoicing: d.Invoicing, ledger: d.Ledger, notes: d.Notes,
		attachments: d.Attachments, store: d.Store, renderer: d.Renderer, mailer: d.Mailer, ids: d.IDs, now: d.Now,
	}
}

// today is the date in the administration's time zone.
func (s *Service) today(a administration.Listed) time.Time {
	now := s.now()
	if loc, err := time.LoadLocation(a.TimeZone); err == nil {
		now = now.In(loc)
	}
	return time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
}

// effective is the state to show: an open invoice past its due date is late.
func effective(inv Invoice, today time.Time) string {
	if inv.State == Open && inv.DueDate != nil && inv.DueDate.Before(today) {
		return Late
	}
	return inv.State
}

// save works out the totals and stores the invoice with what it posts.
func (s *Service) save(ctx context.Context, admin, user resource.ID, inv *Invoice, entries []journal.Entry, created bool) error {
	tot, err := s.Totals(ctx, admin, user, *inv)
	if err != nil {
		return err
	}
	inv.Totals = tot
	return wrap("store invoice", s.store.SaveInvoice(ctx, *inv, entries, created))
}

func invalid(field, problem string) error {
	var verr resource.ValidationError
	verr.Add(field, problem)
	return &verr
}

func wrap(what string, err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%s: %w", what, err)
}
