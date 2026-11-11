package purchases

import (
	"context"
	"errors"
	"strings"

	"github.com/adirkuhn/moneyharpy/internal/banking"
	"github.com/adirkuhn/moneyharpy/internal/bookkeeping"
	"github.com/adirkuhn/moneyharpy/internal/contacts"
	"github.com/adirkuhn/moneyharpy/internal/iban"
	"github.com/adirkuhn/moneyharpy/internal/resource"
	"github.com/adirkuhn/moneyharpy/internal/sales"
)

// CreateBankDocument implements banking.Purchases: it books a purchase
// invoice or receipt for a bank transaction, as Moneybird's NewPurchaseInvoice
// and NewReceipt bookings do. It is dated the transaction's day, from the
// contact with the transaction's IBAN, with one line of the amount
// including VAT on expenses still to categorize at the 0% purchase rate, to
// complete later. A purchase invoice needs that contact.
func (s *Service) CreateBankDocument(ctx context.Context, admin, user resource.ID, in banking.NewDocument) (resource.ID, error) {
	if err := checkKind(in.Kind); err != nil {
		return 0, err
	}
	contact, err := s.contactWithIBAN(ctx, admin, user, in.ContraAccount)
	if err != nil {
		return 0, err
	}
	if contact == nil && in.Kind == PurchaseInvoice {
		return 0, invalid("booking_type", "NewPurchaseInvoice needs a contact with the transaction's IBAN; book a NewReceipt or link an existing purchase invoice instead")
	}
	rate, err := s.zeroRate(ctx, admin, user)
	if err != nil {
		return 0, err
	}
	description := strings.TrimSpace(cmpString(in.Description, in.ContraName))
	day, price, incl := in.Date.Format("2006-01-02"), in.Amount.String(), true
	c := Changes{
		ContactID: contact, Date: &day, PricesAreInclTax: &incl,
		Details: []sales.DetailInput{{Description: &description, Price: &price, TaxRateID: &rate, ProjectID: in.ProjectID}},
	}
	d, err := s.Create(ctx, admin, user, in.Kind, c)
	if err != nil {
		return 0, err
	}
	return d.ID, nil
}

// contactWithIBAN finds the contact whose IBAN is the account, or nil.
func (s *Service) contactWithIBAN(ctx context.Context, admin, user resource.ID, account string) (*resource.ID, error) {
	number := iban.Normalize(account)
	if !iban.Valid(number) {
		return nil, nil
	}
	page, err := s.contacts.List(ctx, admin, user, contacts.Query{Search: number, PerPage: 100})
	if err != nil {
		return nil, wrap("find contact", err)
	}
	for _, c := range page.Contacts {
		if iban.Normalize(c.SepaIBAN) == number || iban.Normalize(c.BankAccount) == number {
			return &c.ID, nil
		}
	}
	return nil, nil
}

// zeroRate is the first active 0% purchase VAT rate that is not reverse
// charged: the VAT of a bank transaction is unknown until the document is
// completed.
func (s *Service) zeroRate(ctx context.Context, admin, user resource.ID) (resource.ID, error) {
	kind, pct, active := "purchase_invoice", "0", true
	rates, err := s.ledger.TaxRates(ctx, admin, user, bookkeeping.TaxRateFilter{Type: &kind, Percentage: &pct, Active: &active})
	if err != nil {
		return 0, wrap("find VAT rate", err)
	}
	if len(rates) == 0 {
		return 0, invalid("booking_type", "needs an active 0% purchase VAT rate")
	}
	return rates[0].ID, nil
}

// Payables implements banking.Purchases: what a purchase transaction, or
// every transaction of a batch, still pays.
func (s *Service) Payables(ctx context.Context, admin, user resource.ID, bookingType string, id resource.ID) ([]banking.Payable, error) {
	if _, err := s.admins.Get(ctx, admin, user); err != nil {
		return nil, err
	}
	var list []Transaction
	switch bookingType {
	case "PurchaseTransaction":
		t, err := s.store.Transaction(ctx, admin, id)
		if errors.Is(err, ErrTransactionNotFound) {
			return nil, invalid("booking_id", "is not a purchase transaction of this administration")
		}
		if err != nil {
			return nil, err
		}
		list = []Transaction{t}
	case "PurchaseTransactionBatch":
		b, err := s.store.Batch(ctx, admin, id)
		if errors.Is(err, ErrTransactionNotFound) {
			return nil, invalid("booking_id", "is not a purchase transaction batch of this administration")
		}
		if err != nil {
			return nil, err
		}
		if b.State == Cancelled {
			return nil, invalid("booking_id", "is a cancelled batch")
		}
		if list, _, err = s.store.Transactions(ctx, admin, TransactionQuery{BatchID: &b.ID}); err != nil {
			return nil, wrap("list purchase transactions", err)
		}
	default:
		return nil, invalid("booking_type", "is not a purchase transaction type")
	}
	var out []banking.Payable
	for _, t := range list {
		if t.State() == Cancelled || t.State() == Paid || t.PayableType != PaymentType || t.PayableID == nil {
			continue
		}
		out = append(out, banking.Payable{DocumentID: *t.PayableID, Amount: t.Amount})
	}
	if len(out) == 0 {
		return nil, invalid("booking_id", "has nothing left to pay")
	}
	return out, nil
}
