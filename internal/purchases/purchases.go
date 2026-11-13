// Package purchases holds an administration's incoming documents, after
// Moneybird's documents: purchase invoices and receipts, booked on saving
// and paid by hand or with bank transactions; general documents kept for the
// record; typeless documents, uploads still to be processed; and purchase
// transactions, the credit transfers that pay purchase invoices, grouped in
// batches exported as SEPA credit transfer files.
package purchases

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
	"github.com/adirkuhn/moneyharpy/internal/journal"
	"github.com/adirkuhn/moneyharpy/internal/money"
	"github.com/adirkuhn/moneyharpy/internal/notes"
	"github.com/adirkuhn/moneyharpy/internal/products"
	"github.com/adirkuhn/moneyharpy/internal/projects"
	"github.com/adirkuhn/moneyharpy/internal/resource"
	"github.com/adirkuhn/moneyharpy/internal/sales"
)

// Errors returned by Service.
var (
	ErrNotFound = errors.New("document not found")
	// ErrNotEditable reports a document whose state no longer allows the
	// change, such as editing a paid purchase invoice.
	ErrNotEditable = errors.New("document cannot be changed in this state")
	// ErrPaymentNotFound reports a payment the document does not have.
	ErrPaymentNotFound = errors.New("payment not found")
	// ErrTransactionNotFound reports a purchase transaction or batch outside
	// the administration.
	ErrTransactionNotFound = errors.New("purchase transaction not found")
	// ErrNoSEPAFile reports a batch without a credit transfer file, such as
	// one of a financial account that is not a bank account.
	ErrNoSEPAFile = errors.New("purchase transaction batch has no SEPA file")
)

// IsNotFound reports whether err means a document resource is missing.
func IsNotFound(err error) bool {
	return errors.Is(err, ErrNotFound) || errors.Is(err, ErrPaymentNotFound) || errors.Is(err, ErrTransactionNotFound) ||
		errors.Is(err, notes.ErrNotFound) || errors.Is(err, attachments.ErrNotFound)
}

// Kinds of booked documents, with Moneybird's names. They name the documents
// in the journal, notes and attachments too.
const (
	PurchaseInvoice = "PurchaseInvoice"
	Receipt         = "Receipt"
	// GeneralDocument and TypelessDocument name the other documents in notes
	// and attachments.
	GeneralDocument  = "GeneralDocument"
	TypelessDocument = "TypelessDocument"
)

// PaymentType is the invoice type of payments of purchase invoices and
// receipts, and the booking type that links a bank transaction to one, as
// Moneybird names them.
const PaymentType = "Document"

// States, with Moneybird's names.
const (
	New            = "new"
	Open           = "open"
	Late           = "late"
	Paid           = "paid"
	PendingPayment = "pending_payment"
	Saved          = "saved"
)

// States are the states a filter may name.
var States = []string{New, Saved, Open, Paid, Late, PendingPayment}

// Origins of documents, with Moneybird's names.
const (
	OriginUpload = "upload"
	OriginAPI    = "api"
)

// Administrations gives access to administrations.
type Administrations interface {
	Get(ctx context.Context, id, user resource.ID) (administration.Listed, error)
}

// Contacts gives the suppliers documents are from.
type Contacts interface {
	Contact(ctx context.Context, admin, id, user resource.ID) (contacts.Contact, error)
	// List finds contacts, such as the one with an IBAN.
	List(ctx context.Context, admin, user resource.ID, q contacts.Query) (contacts.Page, error)
}

// Products gives the products lines name.
type Products interface {
	Product(ctx context.Context, admin, id, user resource.ID) (products.Product, error)
}

// Projects gives the projects lines are booked on.
type Projects interface {
	Project(ctx context.Context, admin, id, user resource.ID) (projects.Project, error)
}

// Ledger is the part of bookkeeping documents rely on.
type Ledger interface {
	Bookable(ctx context.Context, admin, user, id resource.ID, documentType string) (string, error)
	SystemLedgerAccount(ctx context.Context, admin, user resource.ID, role string) (resource.ID, error)
	TaxRate(ctx context.Context, admin, id, user resource.ID) (bookkeeping.TaxRate, error)
	TaxRates(ctx context.Context, admin, user resource.ID, f bookkeeping.TaxRateFilter) ([]bookkeeping.TaxRate, error)
	LockedUntil(ctx context.Context, admin, user resource.ID) (string, error)
	SeedLedgerAccount(ctx context.Context, admin, user resource.ID, key string) (resource.ID, bool, error)
	LedgerAccount(ctx context.Context, admin, id, user resource.ID) (bookkeeping.LedgerAccount, error)
}

// BankAccount is what purchase transactions need of a financial account.
type BankAccount struct {
	ID              resource.ID
	Name            string
	Type            string
	IBAN            string
	BIC             string
	LedgerAccountID resource.ID
}

// Bank links documents to bank transactions. It calls back into the
// service's RegisterBankPayment and RemoveBankPayment.
type Bank interface {
	// LinkInvoicePayment pays the document with (part of) the transaction;
	// price is signed like the document, nil pays what both have open.
	LinkInvoicePayment(ctx context.Context, admin, mutation, user resource.ID, invoiceType string, invoice resource.ID, price *money.Amount) (resource.ID, error)
	// UnlinkPayment removes a payment from its transaction.
	UnlinkPayment(ctx context.Context, admin, mutation, user, payment resource.ID) error
	// FinancialAccount returns a financial account of the administration.
	FinancialAccount(ctx context.Context, admin, account, user resource.ID) (BankAccount, error)
	// FinancialAccounts returns the administration's financial accounts.
	FinancialAccounts(ctx context.Context, admin, user resource.ID) ([]BankAccount, error)
}

// Notes keeps notes on documents.
type Notes interface {
	Add(ctx context.Context, admin resource.ID, entityType string, entity, user resource.ID, in notes.Input) (notes.Note, error)
	Delete(ctx context.Context, admin resource.ID, entityType string, entity, id resource.ID) error
	Of(ctx context.Context, admin resource.ID, entityType string, entities []resource.ID) (map[resource.ID][]notes.Note, error)
}

// Attachments keeps files on documents.
type Attachments interface {
	Add(ctx context.Context, admin resource.ID, entityType string, entity resource.ID, filename, contentType string, data []byte) (attachments.Attachment, error)
	Open(ctx context.Context, admin resource.ID, entityType string, entity, id resource.ID) (attachments.Attachment, io.ReadCloser, error)
	Delete(ctx context.Context, admin resource.ID, entityType string, entity, id resource.ID) error
	DeleteAll(ctx context.Context, admin resource.ID, entityType string, entity resource.ID) error
	Of(ctx context.Context, admin resource.ID, entityType string, entities []resource.ID) (map[resource.ID][]attachments.Attachment, error)
}

// Store persists documents with what they post, and purchase transactions.
type Store interface {
	// Documents lists purchase invoices or receipts (q.Kind).
	Documents(ctx context.Context, admin resource.ID, q Query) ([]Document, int, error)
	Document(ctx context.Context, admin resource.ID, kind string, id resource.ID) (Document, error)
	// DocumentByID returns a purchase invoice or a receipt.
	DocumentByID(ctx context.Context, admin, id resource.ID) (Document, error)
	DocumentsByID(ctx context.Context, admin resource.ID, kind string, ids []resource.ID) ([]Document, error)
	// NextEntryNumber is the number the next document gets.
	NextEntryNumber(ctx context.Context, admin resource.ID) (int, error)
	// SaveDocument stores the document and replaces what it posts.
	SaveDocument(ctx context.Context, d Document, entries []journal.Entry, created bool) error
	// DeleteDocument removes the document with its payments and what they
	// all post.
	DeleteDocument(ctx context.Context, admin, id resource.ID) error
	TouchDocument(ctx context.Context, admin, id resource.ID, at time.Time) error

	// Payment returns one payment of a document.
	Payment(ctx context.Context, admin, id resource.ID) (sales.Payment, error)
	// SavePayments stores, at once, the documents' new states, payments
	// with what they post (replacing it) and the removal of payments with
	// their entries.
	SavePayments(ctx context.Context, docs []Document, added []PaymentPosting, removed []resource.ID) error

	GeneralDocuments(ctx context.Context, admin resource.ID, q Query) ([]General, int, error)
	GeneralDocument(ctx context.Context, admin, id resource.ID) (General, error)
	GeneralDocumentsByID(ctx context.Context, admin resource.ID, ids []resource.ID) ([]General, error)
	SaveGeneralDocument(ctx context.Context, g General, created bool) error
	DeleteGeneralDocument(ctx context.Context, admin, id resource.ID) error
	TouchGeneralDocument(ctx context.Context, admin, id resource.ID, at time.Time) error

	TypelessDocuments(ctx context.Context, admin resource.ID, q Query) ([]Typeless, int, error)
	TypelessDocument(ctx context.Context, admin, id resource.ID) (Typeless, error)
	TypelessDocumentsByID(ctx context.Context, admin resource.ID, ids []resource.ID) ([]Typeless, error)
	SaveTypelessDocument(ctx context.Context, t Typeless, created bool) error
	DeleteTypelessDocument(ctx context.Context, admin, id resource.ID) error
	TouchTypelessDocument(ctx context.Context, admin, id resource.ID, at time.Time) error

	Transactions(ctx context.Context, admin resource.ID, q TransactionQuery) ([]Transaction, int, error)
	Transaction(ctx context.Context, admin, id resource.ID) (Transaction, error)
	// SaveTransactions stores transactions, and the batch when not nil, at
	// once.
	SaveTransactions(ctx context.Context, batch *Batch, list []Transaction, created bool) error
	DeleteTransaction(ctx context.Context, admin, id resource.ID) error
	Batch(ctx context.Context, admin, id resource.ID) (Batch, error)
	SaveBatch(ctx context.Context, b Batch) error

	// Totals sums up the documents still to pay and those to process.
	Totals(ctx context.Context, admin resource.ID) (Summary, error)

	// IncomingDocuments lists incoming documents of every kind, journal
	// entries too; documents without a currency leave it empty.
	IncomingDocuments(ctx context.Context, admin resource.ID, q IncomingQuery) ([]Incoming, int, error)
	// IncomingDocument returns the incoming document of any kind with the
	// id; today decides whether an open one is late.
	IncomingDocument(ctx context.Context, admin, id resource.ID, today time.Time) (Incoming, error)
	// DuplicateCandidates returns the incoming documents with the ids and
	// every other one of the administration that shares a contact or the
	// contents of an attachment with one of them, as duplicate detection
	// compares them. Journal entries are left out.
	DuplicateCandidates(ctx context.Context, admin resource.ID, ids []resource.ID) ([]DuplicateCandidate, error)
	// IncomingStatistics sums up the documents the query selects; the
	// currency is left to the caller.
	IncomingStatistics(ctx context.Context, admin resource.ID, q IncomingQuery) (IncomingStatistics, error)
	// RetypeDocument removes the document as r.From and stores it as its
	// new type, with its notes, attachments and events, at once.
	RetypeDocument(ctx context.Context, r Retype) error
}

// PaymentPosting is a payment with what it posts.
type PaymentPosting struct {
	Payment sales.Payment
	Entries []journal.Entry
}

// Tally is a number of documents and what they amount to.
type Tally struct {
	Count  int
	Amount money.Amount
}

// Summary is what the overview shows of incoming documents.
type Summary struct {
	// ToPay are the purchase invoices and receipts still to pay.
	ToPay Tally
	// ToProcess counts the uploads still to process: typeless documents
	// and new purchase invoices and receipts.
	ToProcess int
	Currency  string
}

// Service implements the incoming document use cases.
type Service struct {
	admins      Administrations
	contacts    Contacts
	products    Products
	projects    Projects
	ledger      Ledger
	notes       Notes
	attachments Attachments
	store       Store
	bank        Bank
	recognition Recognition
	ids         *resource.Generator
	now         func() time.Time
}

// Deps are what a Service needs.
type Deps struct {
	Administrations Administrations
	Contacts        Contacts
	Products        Products
	Projects        Projects
	Ledger          Ledger
	Notes           Notes
	Attachments     Attachments
	Store           Store
	// Bank may be nil; payments can then not be linked to transactions.
	Bank Bank
	IDs  *resource.Generator
	Now  func() time.Time
}

// NewService returns a Service.
func NewService(d Deps) *Service {
	return &Service{
		admins: d.Administrations, contacts: d.Contacts, products: d.Products, projects: d.Projects, ledger: d.Ledger, notes: d.Notes,
		attachments: d.Attachments, store: d.Store, bank: d.Bank, ids: d.IDs, now: d.Now,
	}
}

// Summary sums up what is to pay and to process, for the overview.
func (s *Service) Summary(ctx context.Context, admin, user resource.ID) (Summary, error) {
	a, err := s.admins.Get(ctx, admin, user)
	if err != nil {
		return Summary{}, err
	}
	sum, err := s.store.Totals(ctx, admin)
	if err != nil {
		return Summary{}, wrap("sum up documents", err)
	}
	sum.Currency = a.Currency
	return sum, nil
}

// today is the date in the administration's time zone.
func (s *Service) today(a administration.Listed) time.Time {
	now := s.now()
	if loc, err := time.LoadLocation(a.TimeZone); err == nil {
		now = now.In(loc)
	}
	return time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
}

// checkDay refuses a date in the locked period.
func (s *Service) checkDay(ctx context.Context, admin, user resource.ID, day time.Time, field string) error {
	until, err := s.ledger.LockedUntil(ctx, admin, user)
	if err != nil || until == "" {
		return err
	}
	lock, err := time.Parse(time.DateOnly, until)
	if err != nil {
		return wrap("read locked period", err)
	}
	if !day.After(lock) {
		return invalid(field, "is in the locked period, which ends on "+until)
	}
	return nil
}

// Limits of lists, as at Moneybird.
const (
	DefaultPerPage = 50
	MaxPerPage     = 100
)

func pageOf(q *Query) {
	q.Page = max(q.Page, 1)
	if q.PerPage <= 0 {
		q.PerPage = DefaultPerPage
	}
	q.PerPage = min(q.PerPage, MaxPerPage)
}

func checkIDs(ids []resource.ID) error {
	if len(ids) > MaxPerPage {
		return invalid("ids", fmt.Sprintf("can hold at most %d ids", MaxPerPage))
	}
	return nil
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

func optional(id resource.ID) *resource.ID {
	if id == 0 {
		return nil
	}
	return &id
}

func cmpString(a, b string) string {
	if a != "" {
		return a
	}
	return b
}
