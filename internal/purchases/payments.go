package purchases

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
	"github.com/adirkuhn/moneyharpy/internal/sales"
)

// payable reports a document that can take a payment.
func payable(d Document) bool { return d.State == Open }

// waiting reports a payment registered by hand that waits for its bank
// transaction; linking one takes it over.
func waiting(p sales.Payment) bool {
	return p.FinancialMutationID == nil && (p.ManualPaymentAction == "" || p.ManualPaymentAction == sales.BankTransfer || p.ManualPaymentAction == sales.PaymentWithoutProof)
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

func (s *Service) newPayment(d Document, user resource.ID, price money.Amount, day time.Time) sales.Payment {
	now := s.now()
	return sales.Payment{
		ID: s.ids.Next(), AdministrationID: d.AdministrationID, InvoiceType: PaymentType, InvoiceID: d.ID, Price: price, PriceBase: price,
		PaymentDate: day, UserID: user, CreatedAt: now, UpdatedAt: now,
	}
}

// RegisterPayment registers a payment on a booked document of the kind,
// at most what is unpaid. The manual payment action chooses the account it
// is booked against; with a bank transaction it links the transaction
// instead.
func (s *Service) RegisterPayment(ctx context.Context, admin, user resource.ID, kind string, id resource.ID, in sales.PaymentInput) (sales.Payment, error) {
	return s.registerPayment(ctx, admin, user, kind, id, in)
}

func (s *Service) registerPayment(ctx context.Context, admin, user resource.ID, kind string, id resource.ID, in sales.PaymentInput) (sales.Payment, error) {
	a, err := s.admins.Get(ctx, admin, user)
	if err != nil {
		return sales.Payment{}, err
	}
	d, err := s.store.Document(ctx, admin, kind, id)
	if err != nil {
		return sales.Payment{}, err
	}
	if in.FinancialMutationID != nil {
		return s.registerBankPayment(ctx, admin, user, d, *in.FinancialMutationID, in.Price)
	}
	if !payable(d) {
		return sales.Payment{}, ErrNotEditable
	}
	var verr resource.ValidationError
	price := d.Unpaid()
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
		checkPrice(&verr, "price", price, d.Unpaid())
	}
	dayText, _, _ := strings.Cut(strings.TrimSpace(in.PaymentDate), "T")
	day, err := time.Parse(time.DateOnly, dayText)
	switch {
	case err != nil:
		verr.Add("payment_date", "is not a date")
	case day.After(s.today(a)):
		verr.Add("payment_date", "can't be in the future")
	}
	action := strings.TrimSpace(in.ManualPaymentAction)
	switch {
	case action == sales.InvoicesSettlement:
		verr.Add("manual_payment_action", "invoices_settlement is not available for purchase documents yet")
	case action != "" && !slices.Contains(sales.PaymentActions, action):
		verr.Add("manual_payment_action", "is not included in the list")
	}
	if len(in.TransactionIdentifier) > 255 {
		verr.Add("transaction_identifier", "is too long (maximum is 255 characters)")
	}
	if err := verr.OrNil(); err != nil {
		return sales.Payment{}, err
	}
	if err := s.checkDay(ctx, admin, user, day, "payment_date"); err != nil {
		return sales.Payment{}, err
	}
	contra, account, err := s.contra(ctx, admin, user, action, in)
	if err != nil {
		return sales.Payment{}, err
	}
	p := s.newPayment(d, user, price, day)
	p.PriceBase, p.LedgerAccountID, p.FinancialAccountID, p.ManualPaymentAction = base, &contra, account, action
	p.TransactionIdentifier = strings.TrimSpace(in.TransactionIdentifier)
	d.Payments = append(d.Payments, p)
	if err := s.commit(ctx, a, user, &d, []sales.Payment{p}, nil); err != nil {
		return sales.Payment{}, err
	}
	return p, nil
}

// contra is the account a payment made by hand is booked against, with the
// financial account it names.
func (s *Service) contra(ctx context.Context, admin, user resource.ID, action string, in sales.PaymentInput) (resource.ID, *resource.ID, error) {
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
	case sales.CashPayment:
		if in.FinancialAccountID == nil {
			return seed("cash")
		}
		if s.bank == nil {
			return 0, nil, invalid("financial_account_id", "does not exist")
		}
		acc, err := s.bank.FinancialAccount(ctx, admin, *in.FinancialAccountID, user)
		if err != nil {
			return 0, nil, invalid("financial_account_id", "does not exist")
		}
		return acc.LedgerAccountID, in.FinancialAccountID, nil
	case sales.PrivatePayment:
		return seed("private_deposits")
	case sales.RoundingError:
		return system("rounding")
	case sales.BalanceSettlement:
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
		return system("payments_to_classify")
	}
}

// Payment returns one payment of a purchase invoice or receipt.
func (s *Service) Payment(ctx context.Context, admin, id, user resource.ID) (sales.Payment, error) {
	if _, err := s.admins.Get(ctx, admin, user); err != nil {
		return sales.Payment{}, err
	}
	return s.store.Payment(ctx, admin, id)
}

// DeletePayment removes a payment of a document of the kind. One linked to
// a bank transaction is unlinked from it.
func (s *Service) DeletePayment(ctx context.Context, admin, user resource.ID, kind string, id, paymentID resource.ID) error {
	a, err := s.admins.Get(ctx, admin, user)
	if err != nil {
		return err
	}
	d, err := s.store.Document(ctx, admin, kind, id)
	if err != nil {
		return err
	}
	i := slices.IndexFunc(d.Payments, func(p sales.Payment) bool { return p.ID == paymentID })
	if i < 0 {
		return ErrPaymentNotFound
	}
	p := d.Payments[i]
	if p.FinancialMutationID != nil {
		if s.bank == nil {
			return ErrNotEditable
		}
		return s.bank.UnlinkPayment(ctx, admin, *p.FinancialMutationID, user, p.ID)
	}
	return s.removePayment(ctx, a, user, d, p)
}

func (s *Service) removePayment(ctx context.Context, a administration.Listed, user resource.ID, d Document, p sales.Payment) error {
	if err := s.checkDay(ctx, a.ID, user, p.PaymentDate, "payment_date"); err != nil {
		return err
	}
	d.Payments = slices.DeleteFunc(slices.Clone(d.Payments), func(q sales.Payment) bool { return q.ID == p.ID })
	return s.commit(ctx, a, user, &d, nil, []resource.ID{p.ID})
}

// RegisterBankPayment pays a purchase invoice or receipt with (part of) a
// bank transaction, for banking. open and price are signed like the
// document; nil pays what both still have open. A payment registered by
// hand that waits for its transaction becomes this transaction's when the
// document has nothing else unpaid.
func (s *Service) RegisterBankPayment(ctx context.Context, admin, user resource.ID, invoiceType string, invoiceID, mutation, account resource.ID, day time.Time, open money.Amount, price *money.Amount) (resource.ID, error) {
	a, err := s.admins.Get(ctx, admin, user)
	if err != nil {
		return 0, err
	}
	if invoiceType != PaymentType {
		return 0, invalid("booking_type", "is not a document type")
	}
	d, err := s.store.DocumentByID(ctx, admin, invoiceID)
	if errors.Is(err, ErrNotFound) {
		return 0, invalid("booking_id", "is not a purchase invoice or receipt of this administration")
	}
	if err != nil {
		return 0, err
	}
	if !payable(d) && d.State != Paid {
		return 0, invalid("booking_id", "is not a document waiting for payment")
	}
	if d.Unpaid() == 0 {
		i := slices.IndexFunc(d.Payments, func(p sales.Payment) bool {
			return waiting(p) && fits(p.Price, open) && (price == nil || p.Price == *price)
		})
		if i < 0 {
			return 0, invalid("booking_id", "is paid already")
		}
		p := &d.Payments[i]
		p.FinancialMutationID, p.FinancialAccountID, p.LedgerAccountID, p.PaymentDate, p.UpdatedAt = &mutation, &account, nil, day, s.now()
		return p.ID, s.commit(ctx, a, user, &d, []sales.Payment{*p}, nil)
	}
	amount := d.Unpaid()
	if price != nil {
		amount = *price
	} else if fits(open, amount) {
		amount = open
	}
	var verr resource.ValidationError
	checkPrice(&verr, "price_base", amount, d.Unpaid())
	if verr.OrNil() == nil && !fits(amount, open) {
		verr.Add("price_base", "must not be more than the transaction has open, "+abs(open).String())
	}
	if err := verr.OrNil(); err != nil {
		return 0, err
	}
	p := s.newPayment(d, user, amount, day)
	p.FinancialMutationID, p.FinancialAccountID = &mutation, &account
	d.Payments = append(d.Payments, p)
	return p.ID, s.commit(ctx, a, user, &d, []sales.Payment{p}, nil)
}

// registerBankPayment links a transaction through the bank, which calls
// back into RegisterBankPayment.
func (s *Service) registerBankPayment(ctx context.Context, admin, user resource.ID, d Document, mutation resource.ID, rawPrice *string) (sales.Payment, error) {
	if s.bank == nil {
		return sales.Payment{}, invalid("financial_mutation_id", "cannot be linked")
	}
	var price *money.Amount
	if rawPrice != nil {
		p, err := money.ParseAmount(*rawPrice)
		if err != nil {
			return sales.Payment{}, invalid("price", "is not an amount")
		}
		price = &p
	}
	id, err := s.bank.LinkInvoicePayment(ctx, admin, mutation, user, PaymentType, d.ID, price)
	if err != nil {
		return sales.Payment{}, err
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
	d, err := s.store.DocumentByID(ctx, admin, p.InvoiceID)
	if err != nil {
		return err
	}
	return s.removePayment(ctx, a, user, d, p)
}

// commit stores changed payments: what added payments post and the
// document's new state.
func (s *Service) commit(ctx context.Context, a administration.Listed, user resource.ID, d *Document, added []sales.Payment, removed []resource.ID) error {
	var postings []PaymentPosting
	for _, p := range added {
		entries, err := s.paymentEntries(ctx, a.ID, user, p, *d)
		if err != nil {
			return err
		}
		postings = append(postings, PaymentPosting{Payment: p, Entries: entries})
	}
	settleState(d)
	d.UpdatedAt = s.now()
	if err := s.store.SavePayments(ctx, []Document{*d}, postings, removed); err != nil {
		return fmt.Errorf("store payments: %w", err)
	}
	return nil
}

// paymentEntries is what a payment made by hand posts: the creditors
// against the contra account. A bank transaction posts a linked payment.
func (s *Service) paymentEntries(ctx context.Context, admin, user resource.ID, p sales.Payment, d Document) ([]journal.Entry, error) {
	if p.FinancialMutationID != nil || p.LedgerAccountID == nil {
		return nil, nil
	}
	creditors, err := s.ledger.SystemLedgerAccount(ctx, admin, user, "accounts_payable")
	if err != nil {
		return nil, wrap("find category accounts_payable", err)
	}
	contact := optional(d.ContactID)
	label := d.label()
	entries := []journal.Entry{
		{ID: s.ids.Next(), AdministrationID: admin, DocumentType: journal.Payment, DocumentID: p.ID, Date: p.PaymentDate, LedgerAccountID: creditors,
			ContactID: contact, Amount: p.Price, Description: label},
		{ID: s.ids.Next(), AdministrationID: admin, DocumentType: journal.Payment, DocumentID: p.ID, Date: p.PaymentDate, LedgerAccountID: *p.LedgerAccountID,
			FinancialAccountID: p.FinancialAccountID, Amount: -p.Price, Description: label, RowOrder: 1},
	}
	return entries, journal.Balance(entries)
}

// settleState makes a document paid when its payments cover the total, and
// opens a paid document again when they no longer do.
func settleState(d *Document) {
	total, paid := d.Total(), d.Paid()
	if len(d.Payments) > 0 && (total >= 0 && paid >= total || total < 0 && paid <= total) {
		latest := d.Payments[0].PaymentDate
		for _, p := range d.Payments {
			if p.PaymentDate.After(latest) {
				latest = p.PaymentDate
			}
		}
		d.State, d.PaidAt = Paid, &latest
		return
	}
	if d.State == Paid {
		d.State, d.PaidAt = Open, nil
	}
}
