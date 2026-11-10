package banking

import (
	"cmp"
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/adirkuhn/moneyharpy/internal/resource"
)

// Mutation states, after Moneybird.
const (
	Unprocessed = "unprocessed"
	Processed   = "processed"
)

// Mutation is one transaction on a financial account.
type Mutation struct {
	ID               resource.ID
	AdministrationID resource.ID
	AccountID        resource.ID
	StatementID      resource.ID
	Date             time.Time
	ValueDate        *time.Time
	Message          string
	// Amount is positive for money in, negative for money out.
	Amount         Amount
	Currency       string
	Code           string
	ContraName     string
	ContraAccount  string
	BatchReference string
	// TransactionID is the bank's own reference, used to skip duplicates.
	TransactionID string
	// Fingerprint identifies an imported transaction without a reference.
	Fingerprint string
	Offset      int
	State       string
	ProcessedAt *time.Time
	Bookings    []Booking
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// Ref locates the mutation.
func (m Mutation) Ref() resource.Ref {
	return resource.Ref{ID: m.ID, Type: resource.TypeFinancialMutation, AdministrationID: m.AdministrationID}
}

// direction is 1 for money in and -1 for money out.
func (m Mutation) direction() Amount {
	if m.Amount < 0 {
		return -1
	}
	return 1
}

// Open is the part of the amount that is not booked yet.
func (m Mutation) Open() Amount {
	open := m.Amount
	for _, b := range m.Bookings {
		open -= b.Price * m.direction()
	}
	return open
}

// Version changes whenever the mutation does, for synchronization.
func (m Mutation) Version() int64 { return m.UpdatedAt.Unix() }

// AutoBooked reports that a rule booked the mutation.
func (m Mutation) AutoBooked() bool {
	return slices.ContainsFunc(m.Bookings, func(b Booking) bool { return b.RuleID != nil })
}

// settle sets the state from the open amount.
func (m *Mutation) settle(now time.Time) {
	if len(m.Bookings) > 0 && m.Open() == 0 {
		if m.State != Processed {
			m.State, m.ProcessedAt = Processed, &now
		}
	} else {
		m.State, m.ProcessedAt = Unprocessed, nil
	}
	m.UpdatedAt = now
}

// Booking books part of a mutation on a category.
type Booking struct {
	ID               resource.ID
	AdministrationID resource.ID
	MutationID       resource.ID
	LedgerAccountID  resource.ID
	Description      string
	// Price is positive when booked in the mutation's direction.
	Price Amount
	// RuleID is the rule that made the booking, if one did.
	RuleID    *resource.ID
	CreatedAt time.Time
	UpdatedAt time.Time
}

// Version is a mutation's ID and version, for synchronization.
type Version struct {
	ID      resource.ID
	Version int64
}

// MutationQuery narrows the mutations a store returns.
type MutationQuery struct {
	AccountID *resource.ID
	Period    *Period
}

// MutationFilter narrows a list of mutations, after Moneybird's filter.
type MutationFilter struct {
	MutationQuery
	// States holds unprocessed, processed and auto_booked; empty is all.
	States []string
	// Directions holds debit and credit; empty is both.
	Directions []string
	AmountFrom *Amount
	AmountTo   *Amount
}

func (f MutationFilter) match(m Mutation) bool {
	if len(f.States) > 0 && !slices.Contains(f.States, m.State) && (!slices.Contains(f.States, "auto_booked") || !m.AutoBooked()) {
		return false
	}
	direction := "credit"
	if m.Amount < 0 {
		direction = "debit"
	}
	abs := m.Amount
	if abs < 0 {
		abs = -abs
	}
	return (len(f.Directions) == 0 || slices.Contains(f.Directions, direction)) &&
		(f.AmountFrom == nil || abs >= *f.AmountFrom) && (f.AmountTo == nil || abs <= *f.AmountTo)
}

// ParseMutationFilter reads Moneybird's filter: key:value terms separated by
// commas, values of state and mutation_type separated by pipes. Without a
// filter, the period is this year.
func ParseMutationFilter(raw string, today time.Time) (MutationFilter, error) {
	var f MutationFilter
	var verr resource.ValidationError
	if raw == "" {
		p, _ := ParsePeriod("this_year", today)
		f.Period = &p
		return f, nil
	}
	for part := range strings.SplitSeq(raw, ",") {
		key, value, ok := strings.Cut(part, ":")
		if !ok {
			verr.Add("filter", fmt.Sprintf("%q is not key:value", part))
			continue
		}
		switch key {
		case "period":
			p, err := ParsePeriod(value, today)
			if err != nil {
				verr.Add("filter", "period "+err.Error())
			}
			f.Period = &p
		case "state":
			for v := range strings.SplitSeq(value, "|") {
				switch v {
				case "all":
				case Unprocessed, Processed, "auto_booked":
					f.States = append(f.States, v)
				default:
					verr.Add("filter", "state "+v+" is not included in the list")
				}
			}
		case "mutation_type":
			for v := range strings.SplitSeq(value, "|") {
				switch v {
				case "all":
				case "debit", "credit":
					f.Directions = append(f.Directions, v)
				default:
					verr.Add("filter", "mutation_type "+v+" is not included in the list")
				}
			}
		case "financial_account_id":
			id, err := resource.ParseID(value)
			if err != nil {
				verr.Add("filter", "financial_account_id is not an id")
			}
			f.AccountID = &id
		case "amount_from", "amount_to":
			a, err := ParseAmount(value)
			if err != nil {
				verr.Add("filter", key+" "+err.Error())
			}
			if a < 0 {
				a = -a
			}
			if key == "amount_from" {
				f.AmountFrom = &a
			} else {
				f.AmountTo = &a
			}
		default:
			verr.Add("filter", key+" is not a filter")
		}
	}
	return f, verr.OrNil()
}

// Mutations lists transactions, newest first.
func (s *Service) Mutations(ctx context.Context, admin, user resource.ID, f MutationFilter) ([]Mutation, error) {
	if _, err := s.ensure(ctx, admin, user); err != nil {
		return nil, err
	}
	list, err := s.store.Mutations(ctx, admin, f.MutationQuery)
	if err != nil {
		return nil, fmt.Errorf("list mutations: %w", err)
	}
	list = slices.DeleteFunc(list, func(m Mutation) bool { return !f.match(m) })
	slices.SortStableFunc(list, func(a, b Mutation) int {
		return cmp.Or(b.Date.Compare(a.Date), cmp.Compare(b.StatementID, a.StatementID), cmp.Compare(a.Offset, b.Offset), cmp.Compare(a.ID, b.ID))
	})
	return list, nil
}

// Mutation returns one transaction.
func (s *Service) Mutation(ctx context.Context, admin, id, user resource.ID) (Mutation, error) {
	if _, err := s.ensure(ctx, admin, user); err != nil {
		return Mutation{}, err
	}
	return s.store.Mutation(ctx, admin, id)
}

// MutationVersions lists every transaction's ID and version.
func (s *Service) MutationVersions(ctx context.Context, admin, user resource.ID) ([]Version, error) {
	if _, err := s.ensure(ctx, admin, user); err != nil {
		return nil, err
	}
	return s.store.MutationVersions(ctx, admin)
}

// MutationsByID returns up to 100 transactions by ID; unknown IDs are left
// out.
func (s *Service) MutationsByID(ctx context.Context, admin, user resource.ID, ids []resource.ID) ([]Mutation, error) {
	if _, err := s.ensure(ctx, admin, user); err != nil {
		return nil, err
	}
	if len(ids) > 100 {
		var verr resource.ValidationError
		verr.Add("ids", "can hold at most 100 ids")
		return nil, &verr
	}
	return s.store.MutationsByID(ctx, admin, ids)
}

// Booking types Moneybird links transactions to. Only ledger accounts exist
// in MoneyHarpy so far.
var bookingTypes = []string{"SalesInvoice", "Document", "LedgerAccount", "PaymentTransactionBatch", "PurchaseTransaction", "NewPurchaseInvoice", "NewReceipt", "PaymentTransaction", "PurchaseTransactionBatch", "ExternalSalesInvoice", "Payment", "VatDocument"}

// Link books a transaction.
type Link struct {
	Type      string
	BookingID *resource.ID
	// Price is the amount to book; positive books in the transaction's
	// direction. Nil books what is still open.
	Price       *string
	Description string
}

// LinkBooking books (part of) a transaction on a category.
func (s *Service) LinkBooking(ctx context.Context, admin, id, user resource.ID, l Link) (Mutation, error) {
	if _, err := s.ensure(ctx, admin, user); err != nil {
		return Mutation{}, err
	}
	m, err := s.store.Mutation(ctx, admin, id)
	if err != nil {
		return Mutation{}, err
	}
	var verr resource.ValidationError
	switch {
	case l.Type == "LedgerAccount":
	case slices.Contains(bookingTypes, l.Type):
		return Mutation{}, ErrBookingUnavailable
	default:
		verr.Add("booking_type", "is not included in the list")
		return Mutation{}, &verr
	}
	if l.BookingID == nil {
		verr.Add("booking_id", "can't be blank")
		return Mutation{}, &verr
	}
	if err := s.ledger.CheckBookable(ctx, admin, user, *l.BookingID); err != nil {
		return Mutation{}, err
	}
	if err := s.checkUnlocked(ctx, admin, user, m.Date); err != nil {
		return Mutation{}, err
	}
	price := m.Open() * m.direction()
	if l.Price != nil {
		if price, err = ParseAmount(*l.Price); err != nil {
			verr.Add("price_base", err.Error())
		}
	}
	if price == 0 && verr.OrNil() == nil {
		verr.Add("price_base", "must not be zero")
	}
	if len(l.Description) > 1000 {
		verr.Add("description", "is too long (maximum is 1000 characters)")
	}
	if err := verr.OrNil(); err != nil {
		return Mutation{}, err
	}
	now := s.now()
	b := Booking{ID: s.ids.Next(), AdministrationID: admin, MutationID: m.ID, LedgerAccountID: *l.BookingID, Description: strings.TrimSpace(l.Description), Price: price, CreatedAt: now, UpdatedAt: now}
	m.Bookings = append(m.Bookings, b)
	m.settle(now)
	if err := s.store.SaveBookings(ctx, m, []Booking{b}, nil); err != nil {
		return Mutation{}, fmt.Errorf("store booking: %w", err)
	}
	return m, nil
}

// UnlinkBooking removes a booking from a transaction.
func (s *Service) UnlinkBooking(ctx context.Context, admin, id, user resource.ID, bookingType string, bookingID resource.ID) (Mutation, error) {
	if _, err := s.ensure(ctx, admin, user); err != nil {
		return Mutation{}, err
	}
	m, err := s.store.Mutation(ctx, admin, id)
	if err != nil {
		return Mutation{}, err
	}
	switch bookingType {
	case "LedgerAccountBooking":
	case "Payment":
		return Mutation{}, ErrBookingUnavailable
	default:
		var verr resource.ValidationError
		verr.Add("booking_type", "is not included in the list")
		return Mutation{}, &verr
	}
	i := slices.IndexFunc(m.Bookings, func(b Booking) bool { return b.ID == bookingID })
	if i < 0 {
		return Mutation{}, ErrBookingNotFound
	}
	if err := s.checkUnlocked(ctx, admin, user, m.Date); err != nil {
		return Mutation{}, err
	}
	m.Bookings = slices.Delete(m.Bookings, i, i+1)
	m.settle(s.now())
	if err := s.store.SaveBookings(ctx, m, nil, []resource.ID{bookingID}); err != nil {
		return Mutation{}, fmt.Errorf("remove booking: %w", err)
	}
	return m, nil
}

// checkUnlocked refuses changes dated in the locked period.
func (s *Service) checkUnlocked(ctx context.Context, admin, user resource.ID, day time.Time) error {
	locked, err := s.locked(ctx, admin, user)
	if err != nil {
		return err
	}
	if locked != nil && !day.After(*locked) {
		var verr resource.ValidationError
		verr.Add("date", "is in the locked period, which ends on "+locked.Format(time.DateOnly))
		return &verr
	}
	return nil
}

func (s *Service) locked(ctx context.Context, admin, user resource.ID) (*time.Time, error) {
	raw, err := s.ledger.LockedUntil(ctx, admin, user)
	if err != nil || raw == "" {
		return nil, err
	}
	day, err := time.Parse(time.DateOnly, raw)
	if err != nil {
		return nil, fmt.Errorf("locked period: %w", err)
	}
	return &day, nil
}

// Balance is the balance of one financial account.
type Balance struct {
	Account Account
	Balance Amount
}

// Balances returns every account's balance, leaving out zero balances.
func (s *Service) Balances(ctx context.Context, admin, user resource.ID) ([]Balance, Amount, error) {
	accounts, err := s.Accounts(ctx, admin, user)
	if err != nil {
		return nil, 0, err
	}
	sums, err := s.store.Balances(ctx, admin)
	if err != nil {
		return nil, 0, fmt.Errorf("balances: %w", err)
	}
	var out []Balance
	var total Amount
	for _, a := range accounts {
		if b := sums[a.ID]; b != 0 {
			out = append(out, Balance{Account: a, Balance: b})
			total += b
		}
	}
	return out, total, nil
}

// Summary is what the overview shows of the bank.
type Summary struct {
	// Balance is the sum of every financial account's balance.
	Balance  Amount
	Currency string
	// Unprocessed counts the transactions of active accounts still to link.
	Unprocessed int
	// RecentlyProcessed are the latest processed transactions, newest first.
	RecentlyProcessed []Mutation
}

// Summary returns the bank's balance, what is left to link and up to recent
// recently processed transactions.
func (s *Service) Summary(ctx context.Context, admin, user resource.ID, recent int) (Summary, error) {
	a, err := s.ensure(ctx, admin, user)
	if err != nil {
		return Summary{}, err
	}
	_, total, err := s.Balances(ctx, admin, user)
	if err != nil {
		return Summary{}, err
	}
	n, err := s.store.CountUnprocessed(ctx, admin)
	if err != nil {
		return Summary{}, fmt.Errorf("count unprocessed: %w", err)
	}
	list, err := s.store.RecentlyProcessed(ctx, admin, recent)
	if err != nil {
		return Summary{}, fmt.Errorf("recently processed: %w", err)
	}
	return Summary{Balance: total, Currency: a.Currency, Unprocessed: n, RecentlyProcessed: list}, nil
}
