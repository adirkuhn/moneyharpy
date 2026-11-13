package sales

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/adirkuhn/moneyharpy/internal/administration"
	"github.com/adirkuhn/moneyharpy/internal/bookkeeping"
	"github.com/adirkuhn/moneyharpy/internal/journal"
	"github.com/adirkuhn/moneyharpy/internal/money"
	"github.com/adirkuhn/moneyharpy/internal/resource"
)

// Manual payment actions, with Moneybird's names. They choose the account
// a payment made by hand is booked against.
const (
	BankTransfer        = "bank_transfer"
	PaymentWithoutProof = "payment_without_proof"
	CashPayment         = "cash_payment"
	PrivatePayment      = "private_payment"
	RoundingError       = "rounding_error"
	BalanceSettlement   = "balance_settlement"
	InvoicesSettlement  = "invoices_settlement"
)

// PaymentActions are the manual payment actions.
var PaymentActions = []string{BankTransfer, PaymentWithoutProof, CashPayment, PrivatePayment, RoundingError, BalanceSettlement, InvoicesSettlement}

// PaymentInput registers a payment.
type PaymentInput struct {
	PaymentDate string
	// Price is signed like the invoice; nil pays what is unpaid.
	Price     *string
	PriceBase *string
	// FinancialAccountID is the cash account of a cash payment.
	FinancialAccountID *resource.ID
	// FinancialMutationID links the payment to a bank transaction instead.
	FinancialMutationID   *resource.ID
	TransactionIdentifier string
	ManualPaymentAction   string
	// LedgerAccountID is the balance sheet account of a balance settlement.
	LedgerAccountID *resource.ID
	// InvoiceID is the invoice an invoices settlement settles against.
	InvoiceID *resource.ID
}

// payable reports an invoice that can take a payment.
func payable(inv Invoice) bool {
	switch inv.State {
	case Open, Late, Reminded, PendingPayment:
		return true
	}
	return false
}

// waiting reports a payment registered by hand that waits for its bank
// transaction; linking one takes it over.
func waiting(p Payment) bool {
	return p.FinancialMutationID == nil && (p.ManualPaymentAction == "" || p.ManualPaymentAction == BankTransfer || p.ManualPaymentAction == PaymentWithoutProof)
}

func abs(a money.Amount) money.Amount {
	if a < 0 {
		return -a
	}
	return a
}

// fits reports whether part has the sign of whole and is not larger.
func fits(part, whole money.Amount) bool {
	return part != 0 && (part > 0) == (whole > 0) && abs(part) <= abs(whole)
}

func checkPrice(verr *resource.ValidationError, field string, price, unpaid money.Amount) {
	switch {
	case price == 0:
		verr.Add(field, "must not be zero")
	case unpaid == 0:
		verr.Add(field, "is more than what is unpaid, which is nothing")
	case !fits(price, unpaid):
		verr.Add(field, "must not be more than what is unpaid, "+unpaid.String())
	}
}

func (s *Service) newPayment(inv Invoice, user resource.ID, price money.Amount, day time.Time) Payment {
	now := s.now()
	return Payment{
		ID: s.ids.Next(), AdministrationID: inv.AdministrationID, InvoiceType: inv.kind(), InvoiceID: inv.ID, Price: price, PriceBase: price,
		PaymentDate: day, UserID: user, CreatedAt: now, UpdatedAt: now,
	}
}

// RegisterPayment registers a payment on a sent invoice. With a bank
// transaction it links the transaction instead; an invoices settlement
// settles against another invoice.
func (s *Service) RegisterPayment(ctx context.Context, admin, id, user resource.ID, in PaymentInput) (Payment, error) {
	return s.registerPayment(ctx, admin, user, EntityType, id, in)
}

// RegisterExternalPayment registers a payment on an external invoice, as
// RegisterPayment does on a sales invoice.
func (s *Service) RegisterExternalPayment(ctx context.Context, admin, id, user resource.ID, in PaymentInput) (Payment, error) {
	return s.registerPayment(ctx, admin, user, ExternalEntityType, id, in)
}

// loadInvoice returns an invoice of the kind for the payment code.
func (s *Service) loadInvoice(ctx context.Context, admin resource.ID, kind string, id resource.ID) (Invoice, error) {
	if kind == ExternalEntityType {
		e, err := s.store.ExternalInvoice(ctx, admin, id)
		return e.invoice(), err
	}
	return s.store.Invoice(ctx, admin, id)
}

func (s *Service) registerPayment(ctx context.Context, admin, user resource.ID, kind string, id resource.ID, in PaymentInput) (Payment, error) {
	a, err := s.admins.Get(ctx, admin, user)
	if err != nil {
		return Payment{}, err
	}
	inv, err := s.loadInvoice(ctx, admin, kind, id)
	if err != nil {
		return Payment{}, err
	}
	if in.FinancialMutationID != nil {
		return s.registerBankPayment(ctx, admin, user, inv, *in.FinancialMutationID, in.Price)
	}
	// Moneybird offers Register payment on a paid invoice too; what may be
	// paid is then left to the price check.
	if !payable(inv) && inv.State != Paid {
		return Payment{}, ErrNotEditable
	}
	var verr resource.ValidationError
	price := inv.Unpaid()
	if in.Price != nil {
		if price, err = money.ParseAmount(*in.Price); err != nil {
			verr.Add("price", "is not an amount")
		}
	}
	base := price
	if in.PriceBase != nil && strings.TrimSpace(*in.PriceBase) != "" {
		if base, err = money.ParseAmount(*in.PriceBase); err != nil {
			verr.Add("price_base", "is not an amount")
		}
	}
	if verr.OrNil() == nil {
		checkPrice(&verr, "price", price, inv.Unpaid())
	}
	day, err := time.Parse(time.DateOnly, strings.TrimSpace(firstPart(in.PaymentDate)))
	switch {
	case err != nil:
		verr.Add("payment_date", "is not a date")
	case day.After(s.today(a)):
		verr.Add("payment_date", "can't be in the future")
	}
	action := strings.TrimSpace(in.ManualPaymentAction)
	if action != "" && !slices.Contains(PaymentActions, action) {
		verr.Add("manual_payment_action", "is not included in the list")
	}
	if len(in.TransactionIdentifier) > 255 {
		verr.Add("transaction_identifier", "is too long (maximum is 255 characters)")
	}
	if err := verr.OrNil(); err != nil {
		return Payment{}, err
	}
	if err := s.checkDay(ctx, admin, user, day, "payment_date"); err != nil {
		return Payment{}, err
	}
	if action == InvoicesSettlement {
		if in.InvoiceID == nil {
			return Payment{}, invalid("invoice_id", "can't be blank for an invoices settlement")
		}
		other, err := s.store.Invoice(ctx, admin, *in.InvoiceID)
		if err != nil {
			return Payment{}, invalid("invoice_id", "is not a sales invoice of this administration")
		}
		return s.settle(ctx, a, user, inv, other, price, day)
	}
	contra, account, err := s.contra(ctx, admin, user, action, in)
	if err != nil {
		return Payment{}, err
	}
	p := s.newPayment(inv, user, price, day)
	p.PriceBase, p.LedgerAccountID, p.FinancialAccountID, p.ManualPaymentAction = base, &contra, account, action
	p.TransactionIdentifier = strings.TrimSpace(in.TransactionIdentifier)
	inv.Payments = append(inv.Payments, p)
	if err := s.commit(ctx, a, user, []*Invoice{&inv}, []Payment{p}, nil); err != nil {
		return Payment{}, err
	}
	return p, nil
}

// firstPart drops a time from a date such as 2026-11-10T15:08:53Z, which
// Moneybird accepts as a payment date.
func firstPart(day string) string {
	d, _, _ := strings.Cut(strings.TrimSpace(day), "T")
	return d
}

// contra is the account a payment made by hand is booked against, with the
// financial account it names.
func (s *Service) contra(ctx context.Context, admin, user resource.ID, action string, in PaymentInput) (resource.ID, *resource.ID, error) {
	system := func(role string) (resource.ID, *resource.ID, error) {
		id, err := s.ledger.SystemLedgerAccount(ctx, admin, user, role)
		return id, nil, wrap("find category "+role, err)
	}
	seed := func(key string) (resource.ID, *resource.ID, error) {
		id, ok, err := s.ledger.SeedLedgerAccount(ctx, admin, user, key)
		if err != nil {
			return 0, nil, wrap("find category "+key, err)
		}
		if !ok {
			return 0, nil, invalid("manual_payment_action", "needs the category "+key+", which this administration does not have")
		}
		return id, nil, nil
	}
	switch action {
	case CashPayment:
		if in.FinancialAccountID == nil {
			return seed("cash")
		}
		if s.bank == nil {
			return 0, nil, invalid("financial_account_id", "does not exist")
		}
		id, err := s.bank.AccountLedger(ctx, admin, *in.FinancialAccountID, user)
		if err != nil {
			return 0, nil, invalid("financial_account_id", "does not exist")
		}
		return id, in.FinancialAccountID, nil
	case PrivatePayment:
		return seed("private_deposits")
	case RoundingError:
		return system("rounding")
	case BalanceSettlement:
		if in.LedgerAccountID == nil {
			return 0, nil, invalid("ledger_account_id", "can't be blank for a balance settlement")
		}
		l, err := s.ledger.LedgerAccount(ctx, admin, *in.LedgerAccountID, user)
		switch {
		case err != nil:
			return 0, nil, invalid("ledger_account_id", "is not a category of this administration")
		case !l.Active:
			return 0, nil, invalid("ledger_account_id", "is a deactivated category")
		case !slices.Contains(bookkeeping.BalanceSheetTypes, l.Type) || l.System == "accounts_receivable" || l.System == "accounts_payable":
			return 0, nil, invalid("ledger_account_id", "must be a balance sheet category other than the debtors and creditors")
		}
		return l.ID, nil, nil
	default:
		return system("received_payments_to_classify")
	}
}

// settle registers a payment on each invoice against the settlements
// account, so they settle against each other.
func (s *Service) settle(ctx context.Context, a administration.Listed, user resource.ID, inv, other Invoice, price money.Amount, day time.Time) (Payment, error) {
	if other.ID == inv.ID {
		return Payment{}, invalid("invoice_id", "is this invoice itself")
	}
	if !payable(other) {
		return Payment{}, invalid("invoice_id", "is not an invoice waiting for payment")
	}
	var verr resource.ValidationError
	checkPrice(&verr, "price", -price, other.Unpaid())
	if err := verr.OrNil(); err != nil {
		return Payment{}, err
	}
	settlements, err := s.ledger.SystemLedgerAccount(ctx, a.ID, user, "settlements")
	if err != nil {
		return Payment{}, wrap("find category settlements", err)
	}
	p, q := s.newPayment(inv, user, price, day), s.newPayment(other, user, -price, day)
	p.ManualPaymentAction, q.ManualPaymentAction = InvoicesSettlement, InvoicesSettlement
	p.LedgerAccountID, q.LedgerAccountID = &settlements, &settlements
	p.LinkedPaymentID, q.LinkedPaymentID = &q.ID, &p.ID
	switch {
	case inv.Totals.InclTax < 0:
		q.CreditInvoiceID = &inv.ID
	case other.Totals.InclTax < 0:
		p.CreditInvoiceID = &other.ID
	}
	inv.Payments, other.Payments = append(inv.Payments, p), append(other.Payments, q)
	if err := s.commit(ctx, a, user, []*Invoice{&inv, &other}, []Payment{p, q}, nil); err != nil {
		return Payment{}, err
	}
	return p, nil
}

// SettleCredit settles a credit invoice with the invoice it credits: a
// payment of the credit's unpaid amount on each, dated today.
func (s *Service) SettleCredit(ctx context.Context, admin, id, user resource.ID) (Invoice, error) {
	a, err := s.admins.Get(ctx, admin, user)
	if err != nil {
		return Invoice{}, err
	}
	inv, err := s.store.Invoice(ctx, admin, id)
	if err != nil {
		return Invoice{}, err
	}
	if inv.OriginalID == nil || inv.Totals.InclTax >= 0 {
		return Invoice{}, invalid("base", "Only a credit invoice can be settled with the invoice it credits")
	}
	if !payable(inv) {
		return Invoice{}, ErrNotEditable
	}
	orig, err := s.store.Invoice(ctx, admin, *inv.OriginalID)
	if err != nil {
		return Invoice{}, invalid("base", "The credited invoice no longer exists")
	}
	today := s.today(a)
	if err := s.checkDay(ctx, admin, user, today, "payment_date"); err != nil {
		return Invoice{}, err
	}
	if _, err := s.settle(ctx, a, user, inv, orig, inv.Unpaid(), today); err != nil {
		return Invoice{}, err
	}
	return s.Invoice(ctx, admin, id, user)
}

// Payment returns one payment of a sales or external sales invoice.
func (s *Service) Payment(ctx context.Context, admin, id, user resource.ID) (Payment, error) {
	if _, err := s.admins.Get(ctx, admin, user); err != nil {
		return Payment{}, err
	}
	return s.store.Payment(ctx, admin, id)
}

// DeletePayment removes a payment. One linked to a bank transaction is
// unlinked from it; one half of a settlement takes the other half along.
func (s *Service) DeletePayment(ctx context.Context, admin, id, paymentID, user resource.ID) error {
	return s.deletePayment(ctx, admin, user, EntityType, id, paymentID)
}

// DeleteExternalPayment removes a payment of an external invoice, as
// DeletePayment does.
func (s *Service) DeleteExternalPayment(ctx context.Context, admin, id, paymentID, user resource.ID) error {
	return s.deletePayment(ctx, admin, user, ExternalEntityType, id, paymentID)
}

func (s *Service) deletePayment(ctx context.Context, admin, user resource.ID, kind string, id, paymentID resource.ID) error {
	a, err := s.admins.Get(ctx, admin, user)
	if err != nil {
		return err
	}
	inv, err := s.loadInvoice(ctx, admin, kind, id)
	if err != nil {
		return err
	}
	i := slices.IndexFunc(inv.Payments, func(p Payment) bool { return p.ID == paymentID })
	if i < 0 {
		return ErrPaymentNotFound
	}
	if inv.State == Uncollectible {
		return ErrNotEditable
	}
	p := inv.Payments[i]
	if p.FinancialMutationID != nil {
		if s.bank == nil {
			return ErrNotEditable
		}
		return s.bank.UnlinkPayment(ctx, admin, *p.FinancialMutationID, user, p.ID)
	}
	return s.removePayment(ctx, a, user, inv, p)
}

func (s *Service) removePayment(ctx context.Context, a administration.Listed, user resource.ID, inv Invoice, p Payment) error {
	if err := s.checkDay(ctx, a.ID, user, p.PaymentDate, "payment_date"); err != nil {
		return err
	}
	drop := func(inv *Invoice, id resource.ID) {
		inv.Payments = slices.DeleteFunc(slices.Clone(inv.Payments), func(p Payment) bool { return p.ID == id })
	}
	drop(&inv, p.ID)
	invoices, removed := []*Invoice{&inv}, []Payment{p}
	if p.LinkedPaymentID != nil {
		linked, err := s.store.Payment(ctx, a.ID, *p.LinkedPaymentID)
		if err != nil && !errors.Is(err, ErrPaymentNotFound) {
			return err
		}
		if err == nil {
			other, err := s.loadInvoice(ctx, a.ID, linked.InvoiceType, linked.InvoiceID)
			if err != nil {
				return err
			}
			if other.State == Uncollectible {
				return ErrNotEditable
			}
			drop(&other, linked.ID)
			invoices, removed = append(invoices, &other), append(removed, linked)
		}
	}
	return s.commit(ctx, a, user, invoices, nil, removed)
}

// RegisterBankPayment pays an invoice of the type (SalesInvoice or
// ExternalSalesInvoice) with (part of) a bank transaction, for banking.
// price is signed like the invoice; nil pays what both still have open. A
// payment registered by hand that waits for its transaction becomes this
// transaction's when the invoice has nothing else unpaid.
func (s *Service) RegisterBankPayment(ctx context.Context, admin, user resource.ID, invoiceType string, invoiceID, mutation, account resource.ID, day time.Time, open money.Amount, price *money.Amount) (resource.ID, error) {
	a, err := s.admins.Get(ctx, admin, user)
	if err != nil {
		return 0, err
	}
	if invoiceType != EntityType && invoiceType != ExternalEntityType {
		return 0, invalid("booking_type", "is not an invoice type")
	}
	inv, err := s.loadInvoice(ctx, admin, invoiceType, invoiceID)
	if errors.Is(err, ErrNotFound) {
		return 0, invalid("booking_id", "is not an invoice of this administration")
	}
	if err != nil {
		return 0, err
	}
	if !payable(inv) && inv.State != Paid {
		return 0, invalid("booking_id", "is not an invoice waiting for payment")
	}
	if inv.Unpaid() == 0 {
		i := slices.IndexFunc(inv.Payments, func(p Payment) bool {
			return waiting(p) && fits(p.Price, open) && (price == nil || p.Price == *price)
		})
		if i < 0 {
			return 0, invalid("booking_id", "is paid already")
		}
		p := &inv.Payments[i]
		p.FinancialMutationID, p.FinancialAccountID, p.LedgerAccountID, p.PaymentDate, p.UpdatedAt = &mutation, &account, nil, day, s.now()
		return p.ID, s.commit(ctx, a, user, []*Invoice{&inv}, []Payment{*p}, nil)
	}
	amount := inv.Unpaid()
	if price != nil {
		amount = *price
	} else if fits(open, amount) {
		amount = open
	}
	var verr resource.ValidationError
	checkPrice(&verr, "price_base", amount, inv.Unpaid())
	if verr.OrNil() == nil && !fits(amount, open) {
		verr.Add("price_base", "must not be more than the transaction has open, "+open.String())
	}
	if err := verr.OrNil(); err != nil {
		return 0, err
	}
	p := s.newPayment(inv, user, amount, day)
	p.FinancialMutationID, p.FinancialAccountID = &mutation, &account
	inv.Payments = append(inv.Payments, p)
	return p.ID, s.commit(ctx, a, user, []*Invoice{&inv}, []Payment{p}, nil)
}

// registerBankPayment links a transaction through the bank, which calls
// back into RegisterBankPayment.
func (s *Service) registerBankPayment(ctx context.Context, admin, user resource.ID, inv Invoice, mutation resource.ID, rawPrice *string) (Payment, error) {
	if s.bank == nil {
		return Payment{}, invalid("financial_mutation_id", "cannot be linked")
	}
	var price *money.Amount
	if rawPrice != nil {
		p, err := money.ParseAmount(*rawPrice)
		if err != nil {
			return Payment{}, invalid("price", "is not an amount")
		}
		price = &p
	}
	id, err := s.bank.LinkInvoicePayment(ctx, admin, mutation, user, inv.kind(), inv.ID, price)
	if err != nil {
		return Payment{}, err
	}
	return s.store.Payment(ctx, admin, id)
}

// RemoveBankPayment removes a payment linked to a bank transaction, for
// banking.
func (s *Service) RemoveBankPayment(ctx context.Context, admin, user, paymentID resource.ID) error {
	a, err := s.admins.Get(ctx, admin, user)
	if err != nil {
		return err
	}
	p, err := s.store.Payment(ctx, admin, paymentID)
	if err != nil {
		return err
	}
	inv, err := s.loadInvoice(ctx, admin, p.InvoiceType, p.InvoiceID)
	if err != nil {
		return err
	}
	if inv.State == Uncollectible {
		return ErrNotEditable
	}
	return s.removePayment(ctx, a, user, inv, p)
}

// commit stores changed payments: what added payments post, the invoices'
// new states, and their write-offs again where a payment changes them. It
// records on each invoice what happened: payments registered (and linked,
// when a bank transaction made them), payments removed, states reached.
func (s *Service) commit(ctx context.Context, a administration.Listed, user resource.ID, invoices []*Invoice, added, removed []Payment) error {
	today := s.today(a)
	byID := map[resource.ID]Invoice{}
	for _, inv := range invoices {
		byID[inv.ID] = *inv
	}
	c := PaymentChange{}
	for _, p := range removed {
		c.Removed = append(c.Removed, p.ID)
	}
	for _, p := range added {
		entries, err := s.paymentEntries(ctx, a.ID, user, p, byID[p.InvoiceID])
		if err != nil {
			return err
		}
		c.Payments = append(c.Payments, PaymentPosting{Payment: p, Entries: entries})
	}
	var changed []Invoice
	var events []event
	for _, inv := range invoices {
		was := effective(*inv, today)
		settleState(inv, today)
		inv.UpdatedAt = s.now()
		var entries []journal.Entry
		if inv.MarkedDubiousOn != nil {
			var err error
			if entries, err = s.post(ctx, a.ID, user, *inv); err != nil {
				return err
			}
		}
		c.Invoices = append(c.Invoices, Posting{Invoice: *inv, Entries: entries})
		events = append(events, paymentEvents(*inv, added, removed)...)
		if state := effective(*inv, today); state != was {
			changed = append(changed, *inv)
			events = append(events, stateChanged(*inv, state))
		}
	}
	if err := s.store.SavePayments(ctx, c); err != nil {
		return fmt.Errorf("store payments: %w", err)
	}
	for _, inv := range changed {
		s.notify(ctx, a.ID, inv.events()+"_state_changed_to_"+effective(inv, today), inv)
	}
	return s.record(ctx, user, events...)
}

// paymentEvents tells what payments added to and removed from an invoice
// did. A payment registered by hand that a bank transaction takes over was
// there before: it is only linked.
func paymentEvents(inv Invoice, added, removed []Payment) []event {
	var out []event
	for _, p := range added {
		if p.InvoiceID != inv.ID {
			continue
		}
		if p.FinancialMutationID == nil || p.CreatedAt.Equal(p.UpdatedAt) {
			out = append(out, on(inv, "payment_registered").linked("Payment", p.ID))
		}
		if p.FinancialMutationID != nil {
			out = append(out, on(inv, "payment_linked_to_financial_mutation").linked("Payment", p.ID))
		}
	}
	for _, p := range removed {
		if p.InvoiceID == inv.ID {
			out = append(out, on(inv, "payment_destroyed").linked("Payment", p.ID))
		}
	}
	return out
}

// paymentEntries is what a payment made by hand posts: the contra account
// against the debtors. A bank transaction posts a linked payment.
func (s *Service) paymentEntries(ctx context.Context, admin, user resource.ID, p Payment, inv Invoice) ([]journal.Entry, error) {
	if p.FinancialMutationID != nil || p.LedgerAccountID == nil {
		return nil, nil
	}
	debtors, err := s.ledger.SystemLedgerAccount(ctx, admin, user, "accounts_receivable")
	if err != nil {
		return nil, wrap("find category accounts_receivable", err)
	}
	contact := inv.ContactID
	description := inv.label()
	entries := []journal.Entry{
		{ID: s.ids.Next(), AdministrationID: admin, DocumentType: journal.Payment, DocumentID: p.ID, Date: p.PaymentDate, LedgerAccountID: *p.LedgerAccountID,
			FinancialAccountID: p.FinancialAccountID, Amount: p.Price, Description: description},
		{ID: s.ids.Next(), AdministrationID: admin, DocumentType: journal.Payment, DocumentID: p.ID, Date: p.PaymentDate, LedgerAccountID: debtors,
			ContactID: &contact, Amount: -p.Price, Description: description, RowOrder: 1},
	}
	return entries, journal.Balance(entries)
}

// settleState makes an invoice paid when its payments cover the total, and
// opens a paid invoice again when they no longer do.
func settleState(inv *Invoice, today time.Time) {
	total, paid := inv.Totals.InclTax, inv.Paid()
	if len(inv.Payments) > 0 && (total >= 0 && paid >= total || total < 0 && paid <= total) {
		latest := inv.Payments[0].PaymentDate
		for _, p := range inv.Payments {
			if p.PaymentDate.After(latest) {
				latest = p.PaymentDate
			}
		}
		inv.State, inv.PaidAt = Paid, &latest
		return
	}
	if inv.State == Paid {
		inv.State, inv.PaidAt = reopened(*inv, today), nil
	}
}

// reopened is the state of a sent invoice that is not paid.
func reopened(inv Invoice, today time.Time) string {
	switch {
	case inv.ReminderCount > 0:
		return Reminded
	case inv.DueDate != nil && inv.DueDate.Before(today):
		return Late
	}
	return Open
}

// checkDay refuses a date in the locked period.
func (s *Service) checkDay(ctx context.Context, admin, user resource.ID, day time.Time, field string) error {
	err := s.checkUnlocked(ctx, admin, user, &day)
	var verr *resource.ValidationError
	if errors.As(err, &verr) && field != "invoice_date" {
		return invalid(field, strings.Join(verr.Fields["invoice_date"], "; "))
	}
	return err
}
