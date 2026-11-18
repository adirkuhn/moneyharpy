// Package vatreturns makes the administration's VAT returns, after
// Moneybird's: the periodic return with the corrections of earlier
// periods, the supplementary return of a period declared before, and the
// ICP declaration of supplies to businesses in other EU countries. They are
// filed by hand at the tax authority; a return then books its VAT on what
// is to be paid or received and is paid by linking the bank transaction.
package vatreturns

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/adirkuhn/moneyharpy/internal/administration"
	"github.com/adirkuhn/moneyharpy/internal/journal"
	"github.com/adirkuhn/moneyharpy/internal/money"
	"github.com/adirkuhn/moneyharpy/internal/resource"
)

// Kinds of return, as Moneybird offers them.
const (
	VATReturn  = "vat_return"
	Supplement = "supplement"
	ICP        = "icp_declaration"
)

// Kinds are the kinds of return.
var Kinds = []string{VATReturn, Supplement, ICP}

// States of a return.
const (
	ToPay     = "to_pay"
	ToReceive = "to_receive"
	Paid      = "paid"
	Submitted = "submitted"
)

// DocumentType is how journal entries and bank links name a return, as
// Moneybird's VatDocument.
const DocumentType = "VatDocument"

// ICP supply kinds.
const (
	Goods    = "goods"
	Services = "services"
)

var (
	// ErrNotFound reports a return outside the administration.
	ErrNotFound = errors.New("vat return not found")
	// ErrNotDeletable reports a return that is paid or that later returns
	// build on.
	ErrNotDeletable = errors.New("vat return cannot be deleted")
)

// Correction is what changed in an earlier period after its return,
// declared in a later one.
type Correction struct {
	Period money.Period
	Boxes  []Box
}

// ICPLine is the supplies to one business in another EU country.
type ICPLine struct {
	ContactID resource.ID
	Name      string
	Country   string
	VATNumber string
	Kind      string
	Amount    money.Amount
}

// Payment is (part of) a bank transaction paying or refunding a return.
// Price is positive for what is paid, negative for what is received.
type Payment struct {
	ID         resource.ID
	MutationID resource.ID
	AccountID  resource.ID
	Date       time.Time
	Price      money.Amount
	CreatedAt  time.Time
}

// Return is a VAT return, supplementary return or ICP declaration that was
// filed.
type Return struct {
	ID               resource.ID
	AdministrationID resource.ID
	Kind             string
	Period           money.Period
	// Granularity is the period's length: month, quarter or year.
	Granularity string
	SentOn      time.Time
	// Boxes are the period's own amounts in cents; for a supplement, what
	// changed since the period was declared.
	Boxes       []Box
	Corrections []Correction
	ICP         []ICPLine
	Payments    []Payment
	CreatedBy   resource.ID
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// Ref locates the return.
func (r Return) Ref() resource.Ref {
	return resource.Ref{ID: r.ID, Type: resource.TypeVatDocument, AdministrationID: r.AdministrationID}
}

// AllBoxes is the period's amounts with the corrections.
func (r Return) AllBoxes() []Box {
	lists := [][]Box{r.Boxes}
	for _, c := range r.Corrections {
		lists = append(lists, c.Boxes)
	}
	return Plus(lists...)
}

// Totals are what the return declares.
func (r Return) Totals() Totals { return Sum(r.AllBoxes()) }

// Paid is what payments paid, signed like the total.
func (r Return) Paid() money.Amount {
	var sum money.Amount
	for _, p := range r.Payments {
		sum += p.Price
	}
	return sum
}

// Unpaid is what is still to be paid, or received when negative.
func (r Return) Unpaid() money.Amount {
	if r.Kind == ICP {
		return 0
	}
	return r.Totals().Total - r.Paid()
}

// State is to_pay, to_receive or paid; an ICP declaration is submitted.
func (r Return) State() string {
	switch u := r.Unpaid(); {
	case r.Kind == ICP:
		return Submitted
	case u > 0:
		return ToPay
	case u < 0:
		return ToReceive
	}
	return Paid
}

// Deadline is the last day to file and pay: the end of the month after the
// period.
func (r Return) Deadline() time.Time {
	return time.Date(r.Period.To.Year(), r.Period.To.Month()+2, 1, 0, 0, 0, 0, time.UTC).AddDate(0, 0, -1)
}

// Checks are what may be missing from the books of a period.
type Checks struct {
	// OpenTransactions are bank transactions up to the period's end that
	// are not fully booked.
	OpenTransactions int
	// UntaxedPurchases is what purchases on 0% rates other than reverse
	// charge add up to; it is not part of the return.
	UntaxedPurchases money.Amount
	// BookingsWithoutVAT are bank transactions in the period booked straight
	// on revenue or costs, without VAT.
	BookingsWithoutVAT int
	// UnprocessedDocuments are incoming documents still to be processed.
	UnprocessedDocuments int
}

// Store persists returns and reads the books. Methods trust the caller to
// have checked access.
type Store interface {
	Returns(ctx context.Context, admin resource.ID) ([]Return, error)
	Return(ctx context.Context, admin, id resource.ID) (Return, error)
	// Create stores the return with what it books.
	Create(ctx context.Context, r Return, entries []journal.Entry) error
	// Delete removes the return with what it books.
	Delete(ctx context.Context, admin, id resource.ID) error
	SavePayment(ctx context.Context, r Return, p Payment, user resource.ID) error
	DeletePayment(ctx context.Context, admin, id resource.ID) error
	// PaymentReturn finds the return a payment is of.
	PaymentReturn(ctx context.Context, admin, payment resource.ID) (resource.ID, error)

	RateSums(ctx context.Context, admin resource.ID, p money.Period) ([]RateSum, error)
	// ICPSums is what was sold per contact on rates of box 3b.
	ICPSums(ctx context.Context, admin resource.ID, p money.Period) ([]ICPLine, error)
	Checks(ctx context.Context, admin resource.ID, p money.Period) (Checks, error)
	// FirstEntry is the date of the first journal entry, if any.
	FirstEntry(ctx context.Context, admin resource.ID) (*time.Time, error)
}

// Settings are the bookkeeping settings returns follow.
type Settings struct {
	VATPeriod, ICPPeriod string
	LockedUntil          *string
}

// Books is the part of bookkeeping returns rely on.
type Books interface {
	Settings(ctx context.Context, admin, user resource.ID) (Settings, error)
	SystemLedgerAccount(ctx context.Context, admin, user resource.ID, role string) (resource.ID, error)
}

// Administrations gives access to administrations.
type Administrations interface {
	Get(ctx context.Context, id, user resource.ID) (administration.Listed, error)
}

// Service implements the VAT return use cases.
type Service struct {
	admins Administrations
	books  Books
	store  Store
	ids    *resource.Generator
	now    func() time.Time
}

// NewService returns a Service.
func NewService(admins Administrations, books Books, store Store, ids *resource.Generator, now func() time.Time) *Service {
	return &Service{admins: admins, books: books, store: store, ids: ids, now: now}
}

func invalid(field, problem string) error {
	var verr resource.ValidationError
	verr.Add(field, problem)
	return &verr
}

func (s *Service) today(a administration.Listed) time.Time {
	now := s.now()
	if loc, err := time.LoadLocation(a.TimeZone); err == nil {
		now = now.In(loc)
	}
	y, m, d := now.Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

// Returns lists the filed returns, the latest period first.
func (s *Service) Returns(ctx context.Context, admin, user resource.ID) ([]Return, error) {
	if _, err := s.admins.Get(ctx, admin, user); err != nil {
		return nil, err
	}
	return s.store.Returns(ctx, admin)
}

// Return gets a filed return.
func (s *Service) Return(ctx context.Context, admin, id, user resource.ID) (Return, error) {
	if _, err := s.admins.Get(ctx, admin, user); err != nil {
		return Return{}, err
	}
	return s.store.Return(ctx, admin, id)
}

// Candidate is a period a return of a kind can be filed for.
type Candidate struct {
	Period      money.Period
	Granularity string
}

// Periods are the periods a return of the kind can be filed for, the latest
// first: for a VAT return the ended periods not declared yet, from the
// first booking on; for an ICP declaration those with supplies to
// businesses in other EU countries; for a supplement the declared periods
// that changed since.
func (s *Service) Periods(ctx context.Context, admin, user resource.ID, kind string) ([]Candidate, error) {
	a, err := s.admins.Get(ctx, admin, user)
	if err != nil {
		return nil, err
	}
	if !slices.Contains(Kinds, kind) {
		return nil, invalid("kind", "is not one of "+fmt.Sprint(Kinds))
	}
	st, err := s.books.Settings(ctx, admin, user)
	if err != nil {
		return nil, err
	}
	returns, err := s.store.Returns(ctx, admin)
	if err != nil {
		return nil, err
	}
	if kind == Supplement {
		// As Moneybird, only a declared period that changed since can be supplemented.
		out := []Candidate{}
		for _, r := range returns {
			if r.Kind != VATReturn {
				continue
			}
			now, err := s.boxes(ctx, admin, r.Period)
			if err != nil {
				return nil, err
			}
			if len(Minus(now, declared(returns, r.Period))) > 0 {
				out = append(out, Candidate{Period: r.Period, Granularity: r.Granularity})
			}
		}
		return out, nil
	}
	granularity := st.VATPeriod
	declared := VATReturn
	if kind == ICP {
		granularity, declared = st.ICPPeriod, ICP
	}
	start, err := s.start(ctx, a, granularity)
	if err != nil {
		return nil, err
	}
	today := s.today(a)
	parts, err := money.Period{From: start, To: today}.Split(granularity)
	if err != nil {
		return nil, err
	}
	out := []Candidate{}
	for i := len(parts) - 1; i >= 0; i-- {
		p := whole(parts[i], granularity)
		if !p.To.Before(today) || slices.ContainsFunc(returns, func(r Return) bool { return r.Kind == declared && overlaps(r.Period, p) }) {
			continue
		}
		if kind == ICP {
			// As Moneybird, an ICP declaration needs supplies to businesses in other EU countries.
			lines, err := s.store.ICPSums(ctx, admin, p)
			if err != nil {
				return nil, err
			}
			if len(lines) == 0 {
				continue
			}
		}
		out = append(out, Candidate{Period: p, Granularity: granularity})
	}
	return out, nil
}

// start is where periods to declare begin: the bookkeeping start or the
// first booking, whichever is earlier; at the latest the period before
// today's, so the last ended period can always be declared, if need be as a
// zero return.
func (s *Service) start(ctx context.Context, a administration.Listed, granularity string) (time.Time, error) {
	months := map[string]int{"month": 1, "quarter": 3, "year": 12}[granularity]
	if months == 0 {
		return time.Time{}, invalid("vat_documents_period", "is not one of month, quarter, year")
	}
	start := s.today(a).AddDate(0, -months, 0)
	if day, err := time.Parse(time.DateOnly, a.PeriodStartDate); err == nil && day.Before(start) {
		start = day
	}
	first, err := s.store.FirstEntry(ctx, a.ID)
	if err != nil {
		return time.Time{}, err
	}
	if first != nil && first.Before(start) {
		start = *first
	}
	return start, nil
}

// whole stretches a part of a split to its whole calendar month, quarter
// or year.
func whole(p money.Period, granularity string) money.Period {
	months := map[string]int{"month": 1, "quarter": 3, "year": 12}[granularity]
	start := (int(p.From.Month())-1)/months*months + 1
	from := time.Date(p.From.Year(), time.Month(start), 1, 0, 0, 0, 0, time.UTC)
	return money.Period{From: from, To: from.AddDate(0, months, -1)}
}

func overlaps(a, b money.Period) bool { return !a.From.After(b.To) && !b.From.After(a.To) }

// Draft is the return the kind and period make, with what to check first.
type Draft struct {
	Return
	Checks Checks
}

// Preview works out the return of the kind for the period without filing
// it, with the checks of its books.
func (s *Service) Preview(ctx context.Context, admin, user resource.ID, kind string, period money.Period) (Draft, error) {
	a, err := s.admins.Get(ctx, admin, user)
	if err != nil {
		return Draft{}, err
	}
	r, err := s.draft(ctx, a, user, kind, period, nil)
	if err != nil {
		return Draft{}, err
	}
	checks, err := s.store.Checks(ctx, admin, r.Period)
	if err != nil {
		return Draft{}, err
	}
	sums, err := s.store.RateSums(ctx, admin, r.Period)
	if err != nil {
		return Draft{}, err
	}
	checks.UntaxedPurchases = Untaxed(sums)
	return Draft{Return: r, Checks: checks}, nil
}

// Untaxed is what purchases on 0% rates other than reverse charge add up
// to, such as Moneybird's "geen btw": they count in no box but input tax,
// and add nothing to it.
func Untaxed(sums []RateSum) money.Amount {
	var out money.Amount
	for _, s := range sums {
		zero := strings.Trim(s.Percentage, "0.") == ""
		if s.Type == "purchase_invoice" && zero && !slices.ContainsFunc(s.Boxes, func(b string) bool { return code(b) != inputTax }) {
			out += s.Base
		}
	}
	return out
}

// Overview works out what a return or ICP declaration of any period would
// hold as the books are now, as Moneybird's VAT overview shows it: the
// period's own amounts, without corrections of earlier periods.
func (s *Service) Overview(ctx context.Context, admin, user resource.ID, kind string, period money.Period) (Return, error) {
	if kind != VATReturn && kind != ICP {
		return Return{}, invalid("kind", "is not one of vat_return, icp_declaration")
	}
	a, err := s.admins.Get(ctx, admin, user)
	if err != nil {
		return Return{}, err
	}
	granularity := "month"
	switch period.Months() {
	case 3:
		granularity = "quarter"
	case 12:
		granularity = "year"
	}
	r := Return{AdministrationID: a.ID, Kind: kind, Period: period, Granularity: granularity, SentOn: s.today(a), Boxes: []Box{}, ICP: []ICPLine{}, Corrections: []Correction{}}
	if kind == ICP {
		lines, err := s.store.ICPSums(ctx, a.ID, period)
		if err != nil {
			return Return{}, err
		}
		for _, l := range lines {
			l.Kind = Services
			r.ICP = append(r.ICP, l)
		}
		return r, nil
	}
	if r.Boxes, err = s.boxes(ctx, a.ID, period); err != nil {
		return Return{}, err
	}
	return r, nil
}

// draft works the return out; kinds says per contact whether an ICP line is
// goods, services by default.
func (s *Service) draft(ctx context.Context, a administration.Listed, user resource.ID, kind string, period money.Period, kinds map[resource.ID]string) (Return, error) {
	candidates, err := s.Periods(ctx, a.ID, user, kind)
	if err != nil {
		return Return{}, err
	}
	i := slices.IndexFunc(candidates, func(c Candidate) bool { return c.Period == period })
	if i < 0 {
		return Return{}, invalid("period", "is not a period a return of this kind can be filed for")
	}
	r := Return{AdministrationID: a.ID, Kind: kind, Period: period, Granularity: candidates[i].Granularity, SentOn: s.today(a), ICP: []ICPLine{}, Corrections: []Correction{}}
	if kind == ICP {
		lines, err := s.store.ICPSums(ctx, a.ID, period)
		if err != nil {
			return Return{}, err
		}
		for _, l := range lines {
			l.Kind = Services
			if k, ok := kinds[l.ContactID]; ok {
				l.Kind = k
			}
			r.ICP = append(r.ICP, l)
		}
		r.Boxes = []Box{}
		return r, nil
	}
	returns, err := s.store.Returns(ctx, a.ID)
	if err != nil {
		return Return{}, err
	}
	now, err := s.boxes(ctx, a.ID, period)
	if err != nil {
		return Return{}, err
	}
	if kind == Supplement {
		r.Boxes = Minus(now, declared(returns, period))
		return r, nil
	}
	r.Boxes = now
	for _, earlier := range returns {
		if earlier.Kind != VATReturn || !earlier.Period.To.Before(period.From) {
			continue
		}
		again, err := s.boxes(ctx, a.ID, earlier.Period)
		if err != nil {
			return Return{}, err
		}
		if diff := Minus(again, declared(returns, earlier.Period)); len(diff) > 0 {
			r.Corrections = append(r.Corrections, Correction{Period: earlier.Period, Boxes: diff})
		}
	}
	slices.SortFunc(r.Corrections, func(a, b Correction) int { return a.Period.From.Compare(b.Period.From) })
	return r, nil
}

func (s *Service) boxes(ctx context.Context, admin resource.ID, p money.Period) ([]Box, error) {
	sums, err := s.store.RateSums(ctx, admin, p)
	if err != nil {
		return nil, fmt.Errorf("sum VAT: %w", err)
	}
	return Boxes(sums), nil
}

// declared is what returns declared for a period so far: its return, its
// supplements and the corrections of it in later returns.
func declared(returns []Return, p money.Period) []Box {
	var lists [][]Box
	for _, r := range returns {
		if (r.Kind == VATReturn || r.Kind == Supplement) && r.Period == p {
			lists = append(lists, r.Boxes)
		}
		for _, c := range r.Corrections {
			if c.Period == p {
				lists = append(lists, c.Boxes)
			}
		}
	}
	return Plus(lists...)
}

// Filing files a return made by hand at the tax authority.
type Filing struct {
	Kind   string
	Period money.Period
	// ICPKinds says per contact whether the ICP supplies were goods;
	// services unless said.
	ICPKinds map[resource.ID]string
}

// Submit records a return filed by hand: it books the VAT on what is to be
// paid or received. As on Moneybird the books stay open; later changes to
// the period come back as corrections.
func (s *Service) Submit(ctx context.Context, admin, user resource.ID, f Filing) (Return, error) {
	a, err := s.admins.Get(ctx, admin, user)
	if err != nil {
		return Return{}, err
	}
	for _, k := range f.ICPKinds {
		if k != Goods && k != Services {
			return Return{}, invalid("icp_lines", "kind is not one of goods, services")
		}
	}
	r, err := s.draft(ctx, a, user, f.Kind, f.Period, f.ICPKinds)
	if err != nil {
		return Return{}, err
	}
	now := s.now()
	r.ID, r.CreatedBy, r.CreatedAt, r.UpdatedAt = s.ids.Next(), user, now, now
	entries, err := s.entries(ctx, a, user, r)
	if err != nil {
		return Return{}, err
	}
	if err := s.store.Create(ctx, r, entries); err != nil {
		return Return{}, fmt.Errorf("store vat return: %w", err)
	}
	return r, nil
}

// entries book the return: the VAT owed off payable VAT, the input tax off
// receivable VAT, what is declared on VAT paid or received and the
// difference of rounding to whole euros on rounding differences. A return
// books on its period's last day, a supplement on the day it is filed.
func (s *Service) entries(ctx context.Context, a administration.Listed, user resource.ID, r Return) ([]journal.Entry, error) {
	if r.Kind == ICP {
		return nil, nil
	}
	t := r.Totals()
	day := r.Period.To
	if r.Kind == Supplement {
		day = r.SentOn
	}
	rows := []struct {
		role   string
		amount money.Amount
	}{
		{"payable_vat", t.ExactOwed}, {"receivable_vat", -t.ExactInput}, {"vat_paid_or_received", -t.Total}, {"rounding", t.Total - t.ExactTotal},
	}
	description := label(a.Language, r)
	var out []journal.Entry
	for i, row := range rows {
		if row.amount == 0 {
			continue
		}
		account, err := s.books.SystemLedgerAccount(ctx, a.ID, user, row.role)
		if err != nil {
			return nil, fmt.Errorf("find category %s: %w", row.role, err)
		}
		out = append(out, journal.Entry{
			ID: s.ids.Next(), AdministrationID: a.ID, DocumentType: DocumentType, DocumentID: r.ID, Date: day,
			LedgerAccountID: account, Amount: row.amount, Description: description, RowOrder: i,
		})
	}
	return out, journal.Balance(out)
}

// label names the return in the books, in the administration's language.
func label(language string, r Return) string {
	names := map[string][2]string{VATReturn: {"VAT return", "Btw-aangifte"}, Supplement: {"Supplementary VAT return", "Suppletie btw-aangifte"}}
	name := names[r.Kind][0]
	if language == "nl" {
		name = names[r.Kind][1]
	}
	return name + " " + r.Period.From.Format(time.DateOnly) + " - " + r.Period.To.Format(time.DateOnly)
}

// Delete removes a return filed by mistake, with what it booked. Only a return without payments that no later return builds
// on can go.
func (s *Service) Delete(ctx context.Context, admin, id, user resource.ID) error {
	if _, err := s.admins.Get(ctx, admin, user); err != nil {
		return err
	}
	r, err := s.store.Return(ctx, admin, id)
	if err != nil {
		return err
	}
	if len(r.Payments) > 0 {
		return ErrNotDeletable
	}
	if r.Kind != ICP {
		returns, err := s.store.Returns(ctx, admin)
		if err != nil {
			return err
		}
		if slices.ContainsFunc(returns, func(o Return) bool {
			return o.ID != r.ID && o.Kind != ICP && (o.Period.From.After(r.Period.From) || o.CreatedAt.After(r.CreatedAt))
		}) {
			return ErrNotDeletable
		}
	}
	return s.store.Delete(ctx, admin, id)
}

// RegisterBankPayment pays or refunds a return with (part of) a bank
// transaction, for banking. open and price are signed like the return;
// nil pays what both have open.
func (s *Service) RegisterBankPayment(ctx context.Context, admin, user resource.ID, invoiceType string, id, mutation, account resource.ID, day time.Time, open money.Amount, price *money.Amount) (resource.ID, error) {
	if _, err := s.admins.Get(ctx, admin, user); err != nil {
		return 0, err
	}
	if invoiceType != DocumentType {
		return 0, invalid("booking_type", "is not a VAT return")
	}
	r, err := s.store.Return(ctx, admin, id)
	if errors.Is(err, ErrNotFound) {
		return 0, invalid("booking_id", "is not a VAT return of this administration")
	}
	if err != nil {
		return 0, err
	}
	unpaid := r.Unpaid()
	if unpaid == 0 {
		return 0, invalid("booking_id", "is paid already")
	}
	amount := unpaid
	if price != nil {
		amount = *price
	} else if fits(open, amount) {
		amount = open
	}
	switch {
	case amount == 0 || !fits(amount, unpaid):
		return 0, invalid("price_base", "must be at most what the return has unpaid, "+abs(unpaid).String())
	case !fits(amount, open):
		return 0, invalid("price_base", "must not be more than the transaction has open, "+abs(open).String())
	}
	p := Payment{ID: s.ids.Next(), MutationID: mutation, AccountID: account, Date: day, Price: amount, CreatedAt: s.now()}
	if err := s.store.SavePayment(ctx, r, p, user); err != nil {
		return 0, fmt.Errorf("store payment: %w", err)
	}
	return p.ID, nil
}

// RemoveBankPayment removes a payment made with a transaction, for banking.
func (s *Service) RemoveBankPayment(ctx context.Context, admin, user, payment resource.ID) error {
	if _, err := s.admins.Get(ctx, admin, user); err != nil {
		return err
	}
	if _, err := s.store.PaymentReturn(ctx, admin, payment); err != nil {
		return err
	}
	return s.store.DeletePayment(ctx, admin, payment)
}

// fits reports an amount with the same sign as limit, not beyond it.
func fits(amount, limit money.Amount) bool {
	if limit < 0 {
		return amount < 0 && amount >= limit
	}
	return amount > 0 && amount <= limit
}

func abs(a money.Amount) money.Amount { return max(a, -a) }

// IsNotFound reports whether err means the return does not exist.
func IsNotFound(err error) bool { return errors.Is(err, ErrNotFound) }
