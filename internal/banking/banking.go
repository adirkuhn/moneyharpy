// Package banking holds an administration's money accounts and their
// transactions: bank, cash, credit card and payment service accounts,
// statements imported from files or the API, booking transactions to
// categories, and rules that book them automatically.
package banking

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
	ErrAccountNotFound   = errors.New("financial account not found")
	ErrStatementNotFound = errors.New("financial statement not found")
	ErrMutationNotFound  = errors.New("financial mutation not found")
	ErrBookingNotFound   = errors.New("booking not found")
	ErrRuleNotFound      = errors.New("booking rule not found")
	// ErrBookingUnavailable reports a booking type for records MoneyHarpy
	// does not have yet, such as invoices.
	ErrBookingUnavailable = errors.New("booking type not available yet")
	// ErrStatementBooked reports a statement whose transactions are booked.
	ErrStatementBooked = errors.New("statement has booked transactions")
)

// Administrations gives access to administrations; Get returns
// administration.ErrNotFound when the user has no access.
type Administrations interface {
	Get(ctx context.Context, id, user resource.ID) (administration.Listed, error)
}

// Ledger is the part of bookkeeping banking relies on.
type Ledger interface {
	// FinancialLedgerAccount adds the category an account's money is kept
	// in, under the financial accounts category.
	FinancialLedgerAccount(ctx context.Context, admin, user resource.ID, name, accountType string) (resource.ID, error)
	// SeedLedgerAccount returns the category created from a seed key.
	SeedLedgerAccount(ctx context.Context, admin, user resource.ID, key string) (resource.ID, bool, error)
	// CheckBookable returns a validation error unless transactions can be
	// booked on the category.
	CheckBookable(ctx context.Context, admin, user, id resource.ID) error
	// LockedUntil returns the last day of the locked period, or "".
	LockedUntil(ctx context.Context, admin, user resource.ID) (string, error)
}

// Invoices settles invoices with transactions; sales implements it for
// sales invoices, purchases for purchase invoices and receipts.
type Invoices interface {
	// RegisterBankPayment pays an invoice of the type (SalesInvoice,
	// ExternalSalesInvoice or Document) with (part of) the transaction; open
	// and price are signed like the invoice, nil pays what both have open.
	// It returns the payment.
	RegisterBankPayment(ctx context.Context, admin, user resource.ID, invoiceType string, invoice, mutation, account resource.ID, day time.Time, open Amount, price *Amount) (resource.ID, error)
	// RemoveBankPayment removes a payment made with a transaction.
	RemoveBankPayment(ctx context.Context, admin, user, payment resource.ID) error
}

// Settings are an administration's bank preferences.
type Settings struct {
	// AutoLink books new transactions that match a rule.
	AutoLink bool
	Seeded   bool
}

// Store persists banking data. Methods taking an administration trust the
// caller to have checked access.
type Store interface {
	Settings(ctx context.Context, admin resource.ID) (Settings, error)
	SetAutoLink(ctx context.Context, admin resource.ID, on bool) error
	// Seed marks the administration seeded and stores its first accounts,
	// once; it returns false when it was seeded already.
	Seed(ctx context.Context, admin resource.ID, accounts []Account) (bool, error)

	Accounts(ctx context.Context, admin resource.ID) ([]Account, error)
	Account(ctx context.Context, admin, id resource.ID) (Account, error)
	CreateAccount(ctx context.Context, a Account) error
	UpdateAccounts(ctx context.Context, accounts []Account) error

	Statement(ctx context.Context, admin, id resource.ID) (Statement, error)
	// CreateStatement stores the statement with the mutations its account
	// does not have yet, judged by transaction ID and fingerprint, and
	// returns those. A statement left without mutations is not stored.
	CreateStatement(ctx context.Context, s Statement, mutations []Mutation) ([]Mutation, error)
	UpdateStatement(ctx context.Context, s Statement, added, changed []Mutation, removed []resource.ID) ([]Mutation, error)
	DeleteStatement(ctx context.Context, admin, id resource.ID) error

	Mutations(ctx context.Context, admin resource.ID, q MutationQuery) ([]Mutation, error)
	MutationsByID(ctx context.Context, admin resource.ID, ids []resource.ID) ([]Mutation, error)
	Mutation(ctx context.Context, admin, id resource.ID) (Mutation, error)
	MutationVersions(ctx context.Context, admin resource.ID) ([]Version, error)
	// SaveBookings stores the mutation's state with added and removed
	// bookings.
	SaveBookings(ctx context.Context, m Mutation, added []Booking, removed []resource.ID) error

	Rules(ctx context.Context, admin resource.ID) ([]Rule, error)
	Rule(ctx context.Context, admin, id resource.ID) (Rule, error)
	CreateRule(ctx context.Context, r Rule) error
	UpdateRules(ctx context.Context, rules []Rule) error
	DeleteRule(ctx context.Context, admin, id resource.ID) error

	// Balances returns the sum of every account's mutations dated up to
	// until, or of all when until is nil.
	Balances(ctx context.Context, admin resource.ID, until *time.Time) (map[resource.ID]Amount, error)
	// CountUnprocessed counts the unprocessed transactions of active accounts.
	CountUnprocessed(ctx context.Context, admin resource.ID) (int, error)
	// RecentlyProcessed returns up to limit processed transactions, the
	// most recently processed first.
	RecentlyProcessed(ctx context.Context, admin resource.ID, limit int) ([]Mutation, error)
}

// Service implements the banking use cases.
type Service struct {
	admins   Administrations
	ledger   Ledger
	store    Store
	invoices Invoices
	// documents pays purchase invoices and receipts.
	documents Invoices
	purchases Purchases
	projects  Projects
	ids       *resource.Generator
	now       func() time.Time
}

// NewService returns a Service.
func NewService(admins Administrations, ledger Ledger, store Store, ids *resource.Generator, now func() time.Time) *Service {
	return &Service{admins: admins, ledger: ledger, store: store, ids: ids, now: now}
}

// UseInvoices lets transactions pay invoices. Sales needs the banking
// service too, so it joins after both exist.
func (s *Service) UseInvoices(i Invoices) { s.invoices = i }

// UseDocuments lets transactions pay purchase invoices and receipts, which
// need the banking service too.
func (s *Service) UseDocuments(d Invoices) { s.documents = d }

// payer is what pays invoices of the type, or nil.
func (s *Service) payer(invoiceType string) Invoices {
	switch invoiceType {
	case "SalesInvoice", "ExternalSalesInvoice":
		return s.invoices
	case DocumentType:
		return s.documents
	}
	return nil
}

// Settings returns the bank preferences.
func (s *Service) Settings(ctx context.Context, admin, user resource.ID) (Settings, error) {
	if _, err := s.ensure(ctx, admin, user); err != nil {
		return Settings{}, err
	}
	return s.store.Settings(ctx, admin)
}

// SetAutoLink turns automatic booking by rules on or off.
func (s *Service) SetAutoLink(ctx context.Context, admin, user resource.ID, on bool) (Settings, error) {
	if _, err := s.ensure(ctx, admin, user); err != nil {
		return Settings{}, err
	}
	if err := s.store.SetAutoLink(ctx, admin, on); err != nil {
		return Settings{}, fmt.Errorf("save settings: %w", err)
	}
	return s.store.Settings(ctx, admin)
}

// ensure checks access and gives a new administration its cash account,
// kept in the seeded cash category.
func (s *Service) ensure(ctx context.Context, admin, user resource.ID) (administration.Listed, error) {
	a, err := s.admins.Get(ctx, admin, user)
	if err != nil {
		return administration.Listed{}, err
	}
	st, err := s.store.Settings(ctx, admin)
	if err != nil {
		return administration.Listed{}, fmt.Errorf("load settings: %w", err)
	}
	if st.Seeded {
		return a, nil
	}
	var accounts []Account
	if ledger, ok, err := s.ledger.SeedLedgerAccount(ctx, admin, user, "cash"); err != nil {
		return administration.Listed{}, fmt.Errorf("find cash category: %w", err)
	} else if ok {
		now := s.now()
		accounts = append(accounts, Account{
			ID: s.ids.Next(), AdministrationID: admin, Type: CashAccount, Name: cashName(a.Language), Currency: a.Currency,
			Active: true, Position: 1, LedgerAccountID: ledger, CreatedAt: now, UpdatedAt: now,
		})
	}
	if _, err := s.store.Seed(ctx, admin, accounts); err != nil {
		return administration.Listed{}, fmt.Errorf("seed banking: %w", err)
	}
	return a, nil
}

func cashName(language string) string {
	if len(language) >= 2 && language[:2] == "nl" {
		return "Kas"
	}
	return "Cash"
}

// IsNotFound reports whether err means a banking resource is missing.
func IsNotFound(err error) bool {
	return errors.Is(err, ErrAccountNotFound) || errors.Is(err, ErrStatementNotFound) || errors.Is(err, ErrMutationNotFound) ||
		errors.Is(err, ErrBookingNotFound) || errors.Is(err, ErrRuleNotFound)
}

func invalid(field, problem string) error {
	var verr resource.ValidationError
	verr.Add(field, problem)
	return &verr
}
