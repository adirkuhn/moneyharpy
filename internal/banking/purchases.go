package banking

import (
	"context"
	"strings"
	"time"

	"github.com/adirkuhn/moneyharpy/internal/resource"
)

// NewDocument is a purchase invoice or receipt to make for a transaction,
// which then pays it.
type NewDocument struct {
	// Kind is PurchaseInvoice or Receipt.
	Kind string
	Date time.Time
	// Amount is the total including VAT, signed like the document: positive
	// for a purchase paid, negative for money received back.
	Amount      Amount
	Description string
	ContraName  string
	// ContraAccount is the IBAN the money went to or came from.
	ContraAccount string
	ProjectID     *resource.ID
}

// Payable is a document a purchase transaction pays, with the amount it
// pays, signed like the document.
type Payable struct {
	DocumentID resource.ID
	Amount     Amount
}

// Purchases makes purchase invoices and receipts for transactions and tells
// what purchase transactions pay; purchases implements it. The documents
// are paid through the Invoices that UseDocuments set.
type Purchases interface {
	// CreateBankDocument adds a booked purchase invoice or receipt and
	// returns its id.
	CreateBankDocument(ctx context.Context, admin, user resource.ID, d NewDocument) (resource.ID, error)
	// Payables returns what a purchase transaction (PurchaseTransaction) or
	// every transaction of a batch (PurchaseTransactionBatch) still pays;
	// a validation error on booking_id when there is nothing.
	Payables(ctx context.Context, admin, user resource.ID, bookingType string, id resource.ID) ([]Payable, error)
}

// Projects checks the projects bookings are put on.
type Projects interface {
	// CheckProject returns a validation error on project_id unless the
	// project exists and is active.
	CheckProject(ctx context.Context, admin, user, id resource.ID) error
}

// UsePurchases lets transactions make and pay purchase documents and pay
// purchase transactions.
func (s *Service) UsePurchases(p Purchases) { s.purchases = p }

// UseProjects lets bookings name a project.
func (s *Service) UseProjects(p Projects) { s.projects = p }

func (s *Service) checkProject(ctx context.Context, admin, user resource.ID, id *resource.ID) error {
	if id == nil {
		return nil
	}
	if s.projects == nil {
		return invalid("project_id", "does not exist")
	}
	return s.projects.CheckProject(ctx, admin, user, *id)
}

// fitsOpen checks that money, signed like the transaction's money, is not
// zero, goes the transaction's way and is not more than it has open.
func fitsOpen(m Mutation, money Amount) error {
	open := m.Open()
	switch {
	case open == 0:
		return invalid("price_base", "is more than the transaction has open, which is nothing")
	case money == 0:
		return invalid("price_base", "must not be zero")
	case (money > 0) != (open > 0):
		return invalid("price_base", "must go the transaction's way")
	case abs(money) > abs(open):
		return invalid("price_base", "must not be more than the transaction has open, "+abs(open).String())
	}
	return nil
}

func abs(a Amount) Amount {
	if a < 0 {
		return -a
	}
	return a
}

// linkNewDocument makes a purchase invoice or receipt for (part of) the
// transaction and pays it with it. Link.Price is signed like the document;
// nil takes what is open.
func (s *Service) linkNewDocument(ctx context.Context, admin, user resource.ID, m Mutation, l Link) (Mutation, error) {
	if l.BookingID != nil {
		return Mutation{}, invalid("booking_id", "is not allowed for "+l.Type+", which makes a new document")
	}
	amount := m.Open() * invoiceSign(DocumentType)
	if l.Price != nil {
		var err error
		if amount, err = ParseAmount(*l.Price); err != nil {
			return Mutation{}, invalid("price_base", err.Error())
		}
	}
	if err := fitsOpen(m, amount*invoiceSign(DocumentType)); err != nil {
		return Mutation{}, err
	}
	if err := s.checkUnlocked(ctx, admin, user, m.Date); err != nil {
		return Mutation{}, err
	}
	if err := s.checkProject(ctx, admin, user, l.ProjectID); err != nil {
		return Mutation{}, err
	}
	id, err := s.purchases.CreateBankDocument(ctx, admin, user, NewDocument{
		Kind: strings.TrimPrefix(l.Type, "New"), Date: m.Date, Amount: amount, Description: m.Message,
		ContraName: m.ContraName, ContraAccount: m.ContraAccount, ProjectID: l.ProjectID,
	})
	if err != nil {
		return Mutation{}, err
	}
	m, _, err = s.payInvoice(ctx, admin, user, m, DocumentType, id, &amount)
	return m, err
}

// linkPurchaseTransactions pays what a purchase transaction or every
// transaction of a batch pays with the transaction; the amounts come from
// the purchase transactions.
func (s *Service) linkPurchaseTransactions(ctx context.Context, admin, user resource.ID, m Mutation, l Link) (Mutation, error) {
	if l.BookingID == nil {
		return Mutation{}, invalid("booking_id", "can't be blank")
	}
	payables, err := s.purchases.Payables(ctx, admin, user, l.Type, *l.BookingID)
	if err != nil {
		return Mutation{}, err
	}
	var total Amount
	for _, p := range payables {
		total += p.Amount
	}
	if err := fitsOpen(m, total*invoiceSign(DocumentType)); err != nil {
		return Mutation{}, err
	}
	if err := s.checkUnlocked(ctx, admin, user, m.Date); err != nil {
		return Mutation{}, err
	}
	for _, p := range payables {
		if m, _, err = s.payInvoice(ctx, admin, user, m, DocumentType, p.DocumentID, &p.Amount); err != nil {
			return Mutation{}, err
		}
	}
	return m, nil
}
