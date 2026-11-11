// Package sales holds an administration's sales invoices: drafts with their
// lines, sending them (numbering, the PDF, email and e-invoice), the
// bookings they post, credit invoices, and the notes and files on them,
// after Moneybird's sales invoices. External sales invoices, made in other
// software, are booked and paid the same way; recurring sales invoices make
// sales invoices on a schedule.
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
	"github.com/adirkuhn/moneyharpy/internal/products"
	"github.com/adirkuhn/moneyharpy/internal/projects"
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
	// ErrPaymentNotFound reports a payment the invoice does not have.
	ErrPaymentNotFound = errors.New("payment not found")
)

// IsNotFound reports whether err means a sales invoice resource is missing.
func IsNotFound(err error) bool {
	return errors.Is(err, ErrNotFound) || errors.Is(err, ErrPaymentNotFound) || errors.Is(err, ErrEstimateNotFound) || errors.Is(err, notes.ErrNotFound) || errors.Is(err, attachments.ErrNotFound)
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
	// Type is SalesInvoice, or ExternalSalesInvoice for an external invoice
	// the payment code works on; empty means SalesInvoice.
	Type            string
	ContactID       resource.ID
	ContactPersonID *resource.ID
	IdentityID      resource.ID
	DocumentStyleID resource.ID
	WorkflowID      resource.ID
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
	// NextReminder is when the workflow's next reminder is due; it is
	// worked out on reading and empty when no reminder follows.
	NextReminder *time.Time
	// OriginalID is the invoice a credit invoice credits.
	OriginalID              *resource.ID
	RecurringSalesInvoiceID *resource.ID
	// SubscriptionID is the subscription the invoice bills; the store fills
	// it in.
	SubscriptionID *resource.ID
	// OriginalEstimateID is the estimate the invoice was billed from.
	OriginalEstimateID *resource.ID
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
	t := resource.TypeSalesInvoice
	if i.external() {
		t = resource.TypeExternalSalesInvoice
	}
	return resource.Ref{ID: i.ID, Type: t, AdministrationID: i.AdministrationID}
}

// kind is the invoice's document type: SalesInvoice or ExternalSalesInvoice.
func (i Invoice) kind() string {
	if i.Type == "" {
		return EntityType
	}
	return i.Type
}

func (i Invoice) external() bool { return i.Type == ExternalEntityType }

// label names the invoice in what it posts: its number, an external
// invoice's reference, or a draft's number.
func (i Invoice) label() string {
	if i.external() {
		return cmpString(i.Reference, "external invoice")
	}
	return cmpString(i.InvoiceID, fmt.Sprintf("draft %d", i.DraftID))
}

// events is the prefix of the invoice's webhook events.
func (i Invoice) events() string {
	if i.external() {
		return "external_sales_invoice"
	}
	return "sales_invoice"
}

// Version changes whenever the invoice does.
func (i Invoice) Version() int64 { return i.UpdatedAt.Unix() }

// Sent reports an invoice that left draft, and is booked.
func (i Invoice) Sent() bool { return i.State != Draft && i.State != Scheduled }

// Unpaid is what is still to be paid.
func (i Invoice) Unpaid() money.Amount { return i.Totals.InclTax - i.Paid() }

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

// Payment settles (part of) an invoice. A payment linked to a bank
// transaction is posted by the transaction; any other posts its own
// entries: the contra account against the debtors.
type Payment struct {
	ID               resource.ID
	AdministrationID resource.ID
	// InvoiceType is SalesInvoice or ExternalSalesInvoice.
	InvoiceType         string
	InvoiceID           resource.ID
	FinancialAccountID  *resource.ID
	FinancialMutationID *resource.ID
	// LedgerAccountID is the contra account of a payment made by hand.
	LedgerAccountID *resource.ID
	// CreditInvoiceID is the credit invoice a settlement payment uses.
	CreditInvoiceID *resource.ID
	// LinkedPaymentID is the other half of a settlement between invoices.
	LinkedPaymentID     *resource.ID
	ManualPaymentAction string
	// Price is signed like the invoice's amounts: a credit invoice's
	// payments are negative.
	Price                 money.Amount
	PriceBase             money.Amount
	PaymentDate           time.Time
	TransactionIdentifier string
	UserID                resource.ID
	CreatedAt             time.Time
	UpdatedAt             time.Time
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

// Products gives the products lines name; a line that names one takes its
// description, price, VAT rate and category unless it says otherwise.
type Products interface {
	Product(ctx context.Context, admin, id, user resource.ID) (products.Product, error)
}

// Projects gives the projects lines are booked on.
type Projects interface {
	Project(ctx context.Context, admin, id, user resource.ID) (projects.Project, error)
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
	SeedLedgerAccount(ctx context.Context, admin, user resource.ID, key string) (resource.ID, bool, error)
	LedgerAccount(ctx context.Context, admin, id, user resource.ID) (bookkeeping.LedgerAccount, error)
}

// Bank links invoices to bank transactions. It calls back into the
// service's RegisterBankPayment and RemoveBankPayment.
type Bank interface {
	// LinkInvoicePayment pays the invoice of the type (SalesInvoice or
	// ExternalSalesInvoice) with (part of) the transaction; a nil price pays
	// what both still have open. It returns the payment.
	LinkInvoicePayment(ctx context.Context, admin, mutation, user resource.ID, invoiceType string, invoice resource.ID, price *money.Amount) (resource.ID, error)
	// UnlinkPayment removes a payment from its transaction.
	UnlinkPayment(ctx context.Context, admin, mutation, user, payment resource.ID) error
	// AccountLedger returns the category a financial account's money is in.
	AccountLedger(ctx context.Context, admin, account, user resource.ID) (resource.ID, error)
}

// Notifier tells about what happened to invoices, such as a state change
// or an invoice a recurring invoice made, for webhooks. It may be nil.
type Notifier interface {
	Notify(ctx context.Context, admin resource.ID, action string, inv Invoice)
	NotifyRecurring(ctx context.Context, admin resource.ID, action string, r Recurring)
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

// Store persists invoices with what they post, and estimates.
type Store interface {
	EstimateStore
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

	ExternalInvoices(ctx context.Context, admin resource.ID, q Query) ([]ExternalInvoice, int, error)
	ExternalInvoice(ctx context.Context, admin, id resource.ID) (ExternalInvoice, error)
	ExternalInvoicesByID(ctx context.Context, admin resource.ID, ids []resource.ID) ([]ExternalInvoice, error)
	// NextEntryNumber is the number the next external invoice gets.
	NextEntryNumber(ctx context.Context, admin resource.ID) (int, error)
	// SaveExternalInvoice stores the external invoice and replaces what it
	// posts; nil entries keep them.
	SaveExternalInvoice(ctx context.Context, e ExternalInvoice, entries []journal.Entry, created bool) error
	// DeleteExternalInvoice removes the external invoice with what it and
	// its payments post.
	DeleteExternalInvoice(ctx context.Context, admin, id resource.ID) error
	TouchExternalInvoice(ctx context.Context, admin, id resource.ID, at time.Time) error

	// RecurringInvoices lists recurring invoices, the next to make first.
	RecurringInvoices(ctx context.Context, admin resource.ID, q RecurringQuery) ([]Recurring, int, error)
	RecurringInvoice(ctx context.Context, admin, id resource.ID) (Recurring, error)
	RecurringInvoicesByID(ctx context.Context, admin resource.ID, ids []resource.ID) ([]Recurring, error)
	SaveRecurring(ctx context.Context, r Recurring, created bool) error
	DeleteRecurring(ctx context.Context, admin, id resource.ID) error
	TouchRecurring(ctx context.Context, admin, id resource.ID, at time.Time) error
	// SaveGenerated stores, at once, a new draft invoice a recurring
	// invoice made and the recurring invoice moved on to its next date.
	SaveGenerated(ctx context.Context, inv Invoice, r Recurring) error

	// Payment returns one payment.
	Payment(ctx context.Context, admin, id resource.ID) (Payment, error)
	// SavePayments stores, at once, invoices with what they post (nil
	// entries keep them), payments with theirs (replacing them) and the
	// removal of payments with their entries.
	SavePayments(ctx context.Context, c PaymentChange) error
	// Receivables sums up the invoices still to be paid.
	Receivables(ctx context.Context, admin resource.ID, today time.Time) (Receivables, error)
	// Workload lists the administrations with sent or scheduled invoices,
	// sales or external, or active recurring invoices, each with an owner to
	// act as.
	Workload(ctx context.Context) ([]Workload, error)
	// Lock keeps other instances from working on the administration until
	// release is called; ok is false when another holds it.
	Lock(ctx context.Context, admin resource.ID) (release func(), ok bool, err error)
}

// PaymentChange is what a payment action stores at once.
type PaymentChange struct {
	Invoices []Posting
	Payments []PaymentPosting
	Removed  []resource.ID
}

// Posting is an invoice with what it posts; nil entries keep them. An
// external invoice (Type ExternalSalesInvoice) changes only its state,
// payment and write-off dates.
type Posting struct {
	Invoice Invoice
	Entries []journal.Entry
}

// PaymentPosting is a payment with what it posts.
type PaymentPosting struct {
	Payment Payment
	Entries []journal.Entry
}

// Workload is an administration the background job works on, with the
// user it acts as.
type Workload struct {
	AdministrationID resource.ID
	UserID           resource.ID
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
	products    Products
	projects    Projects
	invoicing   Invoicing
	ledger      Ledger
	notes       Notes
	attachments Attachments
	store       Store
	renderer    Renderer
	mailer      Mailer
	bank        Bank
	notifier    Notifier
	hook        RecurringHook
	ids         *resource.Generator
	now         func() time.Time
}

// Deps are what a Service needs.
type Deps struct {
	Administrations Administrations
	Contacts        Contacts
	Products        Products
	Projects        Projects
	Invoicing       Invoicing
	Ledger          Ledger
	Notes           Notes
	Attachments     Attachments
	Store           Store
	Renderer        Renderer
	Mailer          Mailer
	// Bank may be nil; payments can then not be linked to transactions.
	Bank Bank
	// Notifier may be nil.
	Notifier Notifier
	IDs      *resource.Generator
	Now      func() time.Time
}

// NewService returns a Service.
func NewService(d Deps) *Service {
	return &Service{
		admins: d.Administrations, contacts: d.Contacts, products: d.Products, projects: d.Projects, invoicing: d.Invoicing, ledger: d.Ledger, notes: d.Notes,
		attachments: d.Attachments, store: d.Store, renderer: d.Renderer, mailer: d.Mailer, bank: d.Bank, notifier: d.Notifier, ids: d.IDs, now: d.Now,
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

// notify tells the notifier, if there is one.
func (s *Service) notify(ctx context.Context, admin resource.ID, action string, inv Invoice) {
	if s.notifier != nil {
		s.notifier.Notify(ctx, admin, action, inv)
	}
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
