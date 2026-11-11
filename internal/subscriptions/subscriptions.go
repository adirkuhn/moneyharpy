// Package subscriptions bills contacts periodically for products, after
// Moneybird's subscriptions: each subscription runs a recurring sales invoice
// that bills its product, and additional charges, for a subscription or for
// a contact, are billed on the next invoice.
package subscriptions

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/adirkuhn/moneyharpy/internal/administration"
	"github.com/adirkuhn/moneyharpy/internal/contacts"
	"github.com/adirkuhn/moneyharpy/internal/money"
	"github.com/adirkuhn/moneyharpy/internal/products"
	"github.com/adirkuhn/moneyharpy/internal/resource"
	"github.com/adirkuhn/moneyharpy/internal/sales"
)

// Errors returned by Service.
var (
	ErrNotFound = errors.New("subscription not found")
	// ErrCancelled reports a subscription that was cancelled already.
	ErrCancelled = errors.New("subscription is cancelled already")
	// ErrProductNotFound reports a charge for a product that does not exist.
	ErrProductNotFound = errors.New("product not found")
)

// Subscription bills a contact for a product every period through its
// recurring sales invoice.
type Subscription struct {
	ID               resource.ID
	AdministrationID resource.ID
	ContactID        resource.ID
	ContactPersonID  *resource.ID
	// RecurringID is the recurring sales invoice that bills it; nil once
	// that is deleted.
	RecurringID   *resource.ID
	StartDate     time.Time
	EndDate       *time.Time
	Frequency     int
	FrequencyType string
	Reference     string
	CancelledAt   *time.Time
	// Items are the products subscribed to over time, the first first.
	Items     []Item
	CreatedAt time.Time
	UpdatedAt time.Time
}

// Item is a product subscribed to from its start date until its end date.
type Item struct {
	ID        resource.ID
	ProductID resource.ID
	StartDate time.Time
	EndDate   *time.Time
	// Amount is the quantity as given, such as "1 x"; Quantity its number
	// in thousandths.
	Amount   string
	Quantity int64
	// Discount is a percentage in hundredths.
	Discount int64
}

// ItemAt returns the item that runs on day: the last one started by then,
// or the first.
func (s Subscription) ItemAt(day time.Time) Item {
	if len(s.Items) == 0 {
		return Item{}
	}
	at := s.Items[0]
	for _, it := range s.Items {
		if !it.StartDate.After(day) {
			at = it
		}
	}
	return at
}

// Ref locates the subscription.
func (s Subscription) Ref() resource.Ref {
	return resource.Ref{ID: s.ID, Type: resource.TypeSubscription, AdministrationID: s.AdministrationID}
}

// Charge is an additional charge billed on the next invoice of a
// subscription or, for a contact, on an invoice of its own.
type Charge struct {
	ID               resource.ID
	AdministrationID resource.ID
	// Exactly one of ContactID and SubscriptionID is set.
	ContactID      *resource.ID
	SubscriptionID *resource.ID
	ProductID      resource.ID
	Amount         string
	Quantity       int64
	Price          money.Amount
	// Period is the whole months the charge is about.
	Period      money.Period
	Description string
	Identifier  string
	// InvoiceID and DetailID are the invoice and line that billed it.
	InvoiceID *resource.ID
	DetailID  *resource.ID
	CreatedAt time.Time
	UpdatedAt time.Time
}

// Billed reports whether an invoice billed the charge.
func (c Charge) Billed() bool { return c.DetailID != nil }

// ChargeQuery selects charges: those of a contact or of a subscription.
type ChargeQuery struct {
	ContactID      *resource.ID
	SubscriptionID *resource.ID
	IncludeBilled  bool
}

// Workload is an administration with contact charges to bill, with the
// user to act as.
type Workload struct {
	AdministrationID resource.ID
	UserID           resource.ID
}

// Administrations checks access to an administration.
type Administrations interface {
	Get(ctx context.Context, id, user resource.ID) (administration.Listed, error)
}

// Contacts finds the contacts subscriptions bill.
type Contacts interface {
	Contact(ctx context.Context, admin, id, user resource.ID) (contacts.Contact, error)
}

// Products finds what subscriptions and charges bill.
type Products interface {
	Product(ctx context.Context, admin, id, user resource.ID) (products.Product, error)
}

// Sales makes the invoices subscriptions bill with.
type Sales interface {
	CreateRecurring(ctx context.Context, admin, user resource.ID, c sales.RecurringChanges) (sales.Recurring, error)
	UpdateRecurring(ctx context.Context, admin, id, user resource.ID, c sales.RecurringChanges) (sales.Recurring, error)
	Recurring(ctx context.Context, admin, id, user resource.ID) (sales.Recurring, error)
	DeactivateRecurring(ctx context.Context, admin, id, user resource.ID) (sales.Recurring, error)
	DeleteRecurring(ctx context.Context, admin, id, user resource.ID) (bool, error)
	Create(ctx context.Context, admin, user resource.ID, c sales.Changes) (sales.Invoice, error)
	Update(ctx context.Context, admin, id, user resource.ID, c sales.Changes) (sales.Invoice, error)
	Send(ctx context.Context, admin, id, user resource.ID, in sales.SendInput) (sales.Invoice, error)
	Invoice(ctx context.Context, admin, id, user resource.ID) (sales.Invoice, error)
	Delete(ctx context.Context, admin, id, user resource.ID) error
}

// Notifier tells about invoices subscriptions made, for webhooks. It may be
// nil.
type Notifier interface {
	Notify(ctx context.Context, admin resource.ID, action string, inv sales.Invoice)
}

// Store persists subscriptions and charges.
type Store interface {
	// Subscriptions lists a contact's subscriptions, the oldest first.
	Subscriptions(ctx context.Context, admin, contact resource.ID) ([]Subscription, error)
	Subscription(ctx context.Context, admin, id resource.ID) (Subscription, error)
	SubscriptionByRecurring(ctx context.Context, admin, recurring resource.ID) (Subscription, error)
	// SaveSubscription stores the subscription with its items.
	SaveSubscription(ctx context.Context, s Subscription, created bool) error

	// Charges lists charges, the oldest first.
	Charges(ctx context.Context, admin resource.ID, q ChargeQuery) ([]Charge, error)
	SaveCharge(ctx context.Context, c Charge) error
	// BillCharges stores, at once, which invoice line billed each charge.
	BillCharges(ctx context.Context, admin resource.ID, billed []Charge) error
	// UnbilledContactCharges lists the administration's unbilled charges
	// of contacts.
	UnbilledContactCharges(ctx context.Context, admin resource.ID) ([]Charge, error)
	// ChargeWorkload lists administrations with unbilled contact charges.
	ChargeWorkload(ctx context.Context) ([]Workload, error)
	// Lock keeps other instances from billing the administration's charges
	// until release is called; ok is false when another holds it.
	Lock(ctx context.Context, admin resource.ID) (release func(), ok bool, err error)

	// OneOffInvoices lists the invoices scheduled to go with the
	// subscription's next invoice.
	OneOffInvoices(ctx context.Context, admin, subscription resource.ID) ([]resource.ID, error)
	AddOneOffInvoice(ctx context.Context, admin, subscription, invoice resource.ID) error
}

// Service implements the subscription use cases.
type Service struct {
	admins   Administrations
	contacts Contacts
	products Products
	sales    Sales
	store    Store
	notifier Notifier
	ids      *resource.Generator
	now      func() time.Time
}

// Deps are what a Service needs.
type Deps struct {
	Administrations Administrations
	Contacts        Contacts
	Products        Products
	Sales           Sales
	Store           Store
	// Notifier may be nil.
	Notifier Notifier
	IDs      *resource.Generator
	Now      func() time.Time
}

// NewService returns a Service.
func NewService(d Deps) *Service {
	return &Service{admins: d.Administrations, contacts: d.Contacts, products: d.Products, sales: d.Sales, store: d.Store, notifier: d.Notifier, ids: d.IDs, now: d.Now}
}

// today is the date in the administration's time zone.
func (s *Service) today(a administration.Listed) time.Time {
	now := s.now()
	if loc, err := time.LoadLocation(a.TimeZone); err == nil {
		now = now.In(loc)
	}
	return time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
}

func invalid(field, problem string) error {
	var verr resource.ValidationError
	verr.Add(field, problem)
	return &verr
}

func parseDate(field, v string, verr *resource.ValidationError) *time.Time {
	d, err := time.Parse(time.DateOnly, strings.TrimSpace(v))
	if err != nil {
		verr.Add(field, "is not a date")
		return nil
	}
	return &d
}

var leadingNumber = regexp.MustCompile(`^-?\d+([.,]\d+)?`)

// unitDays and unitMonths measure the frequency types: days and weeks in
// days, months, quarters and years in months.
var (
	unitDays   = map[string]int{sales.Day: 1, sales.Week: 7}
	unitMonths = map[string]int{sales.Month: 1, sales.Quarter: 3, sales.Year: 12}
)

// periods is how many of the product's billing periods one of the
// subscription's holds, or 0 when they do not fit: a monthly product can be
// billed yearly (12) but not weekly.
func periods(productEvery int, productType string, every int, typ string) int {
	if productEvery <= 0 || every <= 0 {
		return 0
	}
	for _, unit := range []map[string]int{unitDays, unitMonths} {
		p, ok1 := unit[productType]
		q, ok2 := unit[typ]
		if ok1 && ok2 {
			a, b := q*every, p*productEvery
			if a%b != 0 {
				return 0
			}
			return a / b
		}
	}
	return 0
}

// quantityOf reads an amount such as "1", "5 x" or 3. A bare number gets
// " x", as Moneybird shows it.
func quantityOf(v string) (string, int64, error) {
	v = strings.TrimSpace(v)
	m := leadingNumber.FindString(v)
	if m == "" {
		return "", 0, errors.New("does not start with a number")
	}
	q, err := sales.ParseQuantity(strings.ReplaceAll(m, ",", "."))
	if err != nil {
		return "", 0, err
	}
	if !strings.ContainsFunc(v, func(r rune) bool { return r < '0' || r > '9' }) {
		v += " x"
	}
	return v, q, nil
}

// lineAmount is the amount as invoice lines take it: the number apart from
// what follows, so "5x" becomes "5 x".
func lineAmount(v string) string {
	m := leadingNumber.FindString(v)
	rest := strings.TrimSpace(strings.TrimPrefix(v, m))
	if rest == "" {
		return m
	}
	return m + " " + rest
}

// percentage reads a discount such as "10", 2.5 or "10,95".
func percentage(v string) (int64, error) {
	v = strings.ReplaceAll(strings.TrimSpace(v), ",", ".")
	if v == "" {
		return 0, nil
	}
	return sales.ParsePercentage(v)
}

// percentText writes a percentage in hundredths as sales do.
func percentText(p int64) string {
	return fmt.Sprintf("%d.%02d", p/100, p%100)
}

// linePrice is what one billing period of the product costs for a
// subscription that bills it every count periods.
func linePrice(p products.Product, count int) money.Amount {
	return p.Price * money.Amount(count)
}

// sortItems keeps items by start date.
func sortItems(items []Item) {
	slices.SortStableFunc(items, func(a, b Item) int { return a.StartDate.Compare(b.StartDate) })
}

func wrap(what string, err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%s: %w", what, err)
}

func ptr[T any](v T) *T { return &v }
