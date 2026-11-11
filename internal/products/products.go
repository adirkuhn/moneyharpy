// Package products holds what an administration sells, after Moneybird's
// products: a title or description, a price with its VAT rate and the
// category its revenue is booked on, and optionally how often it is billed.
// Invoice lines that name a product take these as their defaults.
package products

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/adirkuhn/moneyharpy/internal/administration"
	"github.com/adirkuhn/moneyharpy/internal/bookkeeping"
	"github.com/adirkuhn/moneyharpy/internal/contacts"
	"github.com/adirkuhn/moneyharpy/internal/money"
	"github.com/adirkuhn/moneyharpy/internal/resource"
)

// Errors returned by Service.
var (
	ErrNotFound = errors.New("product not found")
	// ErrNotForOnlineSales reports a product that cannot be sold through a
	// checkout page; MoneyHarpy has no online checkout, so none can.
	ErrNotForOnlineSales = errors.New("product not valid for online sales")
)

// Paging, as at Moneybird.
const (
	DefaultPerPage = 10
	MaxPerPage     = 100
)

// Values Moneybird accepts.
var (
	FrequencyTypes = []string{"day", "week", "month", "quarter", "year"}
	ProductTypes   = []string{"digital_service", "service", "product"}
	VatRateTypes   = []string{"standard", "reduced"}
	CheckoutTypes  = []string{"product", "subscription"}
)

var (
	currencyPattern = regexp.MustCompile(`^[A-Z]{3}$`)
	presetPattern   = regexp.MustCompile(`^(\d+):(day|week|month|quarter|year|custom|nil)$`)
)

// Product is something the administration sells.
type Product struct {
	ID                  resource.ID
	AdministrationID    resource.ID
	Title               string
	Description         string
	CheckoutDescription string
	// Identifier is the administration's own code for the product, such
	// as an article number; unique when set.
	Identifier string
	Price      money.Amount
	Currency   string
	// Frequency and FrequencyType say how often the product is billed;
	// zero and empty when it is not billed periodically.
	Frequency       int
	FrequencyType   string
	TaxRateID       *resource.ID
	LedgerAccountID resource.ID
	DocumentStyleID *resource.ID
	WorkflowID      *resource.ID
	ProductType     string
	VatRateType     string
	CheckoutType    string
	// MaxAmountPerOrder applies to subscriptions sold online.
	MaxAmountPerOrder *int
	// Active is false once deactivated; deactivated products leave lists.
	Active    bool
	CreatedAt time.Time
	UpdatedAt time.Time
}

// Ref locates the product.
func (p Product) Ref() resource.Ref {
	return resource.Ref{ID: p.ID, Type: resource.TypeProduct, AdministrationID: p.AdministrationID}
}

// LineDescription is what a line that names the product says by default.
func (p Product) LineDescription() string {
	if p.Description != "" {
		return p.Description
	}
	return p.Title
}

// Changes holds the fields to set; nil fields stay as they are. Empty
// strings clear optional ids and texts.
type Changes struct {
	Title               *string
	Description         *string
	CheckoutDescription *string
	Identifier          *string
	Price               *string
	Currency            *string
	Frequency           *int
	FrequencyType       *string
	// FrequencyPreset sets both as <frequency>:<type>, such as 3:month;
	// custom is ignored and nil removes the billing interval.
	FrequencyPreset   *string
	TaxRateID         *resource.ID
	LedgerAccountID   *resource.ID
	DocumentStyleID   *resource.ID
	WorkflowID        *resource.ID
	ProductType       *string
	VatRateType       *string
	CheckoutType      *string
	MaxAmountPerOrder *int
	// Active deactivates or reactivates the product; a MoneyHarpy
	// extension.
	Active *bool
}

// Query selects products, after Moneybird's list parameters.
type Query struct {
	// Search matches the title, description and identifier.
	Search   string
	Currency string
	// IncludeInactive also lists deactivated products.
	IncludeInactive bool
	// ForCheckout, when set, keeps the products that can (true) or cannot
	// (false) be sold online; none can in MoneyHarpy.
	ForCheckout     *bool
	LedgerAccountID *resource.ID
	Page, PerPage   int
}

// Matches reports whether the query selects the product; the reference for
// stores without a query language of their own.
func (q Query) Matches(p Product) bool {
	search := strings.ToLower(strings.TrimSpace(q.Search))
	switch {
	case !q.IncludeInactive && !p.Active,
		q.Currency != "" && p.Currency != q.Currency,
		q.ForCheckout != nil && *q.ForCheckout,
		q.LedgerAccountID != nil && p.LedgerAccountID != *q.LedgerAccountID,
		search != "" && !strings.Contains(strings.ToLower(p.Title+"\n"+p.Description+"\n"+p.Identifier), search):
		return false
	}
	return true
}

// Administrations checks access to an administration.
type Administrations interface {
	Get(ctx context.Context, id, user resource.ID) (administration.Listed, error)
}

// Ledger checks the VAT rate and category a product books on.
type Ledger interface {
	TaxRate(ctx context.Context, admin, id, user resource.ID) (bookkeeping.TaxRate, error)
	Bookable(ctx context.Context, admin, user, id resource.ID, documentType string) (string, error)
}

// Store persists products.
type Store interface {
	// Products lists the products the query selects by title and
	// description, with the number selected in all.
	Products(ctx context.Context, admin resource.ID, q Query) ([]Product, int, error)
	Product(ctx context.Context, admin, id resource.ID) (Product, error)
	ProductByIdentifier(ctx context.Context, admin resource.ID, identifier string) (Product, error)
	SaveProduct(ctx context.Context, p Product, created bool) error
	DeleteProduct(ctx context.Context, admin, id resource.ID) error
}

// References tells whether documents name a product.
type References interface {
	ProductInUse(ctx context.Context, admin, product resource.ID) (bool, error)
}

// AnyReferences reports a product in use when any of its members does.
type AnyReferences []References

// ProductInUse implements References.
func (refs AnyReferences) ProductInUse(ctx context.Context, admin, product resource.ID) (bool, error) {
	for _, r := range refs {
		used, err := r.ProductInUse(ctx, admin, product)
		if err != nil || used {
			return used, err
		}
	}
	return false, nil
}

// Contacts finds the contact a sales link is made for.
type Contacts interface {
	Contact(ctx context.Context, admin, id, user resource.ID) (contacts.Contact, error)
}

// Service implements the product use cases.
type Service struct {
	admins     Administrations
	ledger     Ledger
	contacts   Contacts
	store      Store
	references References
	ids        *resource.Generator
	now        func() time.Time
}

// NewService returns a Service. references may be nil.
func NewService(admins Administrations, ledger Ledger, contacts Contacts, store Store, references References, ids *resource.Generator, now func() time.Time) *Service {
	return &Service{admins: admins, ledger: ledger, contacts: contacts, store: store, references: references, ids: ids, now: now}
}

// List returns the page of products the query selects.
func (s *Service) List(ctx context.Context, admin, user resource.ID, q Query) ([]Product, int, error) {
	if _, err := s.admins.Get(ctx, admin, user); err != nil {
		return nil, 0, err
	}
	q.Page = max(q.Page, 1)
	if q.PerPage <= 0 {
		q.PerPage = DefaultPerPage
	}
	q.PerPage = min(q.PerPage, MaxPerPage)
	list, total, err := s.store.Products(ctx, admin, q)
	if err != nil {
		return nil, 0, fmt.Errorf("list products: %w", err)
	}
	return list, total, nil
}

// Product returns one product.
func (s *Service) Product(ctx context.Context, admin, id, user resource.ID) (Product, error) {
	if _, err := s.admins.Get(ctx, admin, user); err != nil {
		return Product{}, err
	}
	return s.store.Product(ctx, admin, id)
}

// ByIdentifier returns the product with the identifier.
func (s *Service) ByIdentifier(ctx context.Context, admin, user resource.ID, identifier string) (Product, error) {
	if _, err := s.admins.Get(ctx, admin, user); err != nil {
		return Product{}, err
	}
	if strings.TrimSpace(identifier) == "" {
		return Product{}, ErrNotFound
	}
	return s.store.ProductByIdentifier(ctx, admin, identifier)
}

// Create adds an active product. It needs a price, a category and a title
// or description; the currency is the administration's unless given.
func (s *Service) Create(ctx context.Context, admin, user resource.ID, c Changes) (Product, error) {
	a, err := s.admins.Get(ctx, admin, user)
	if err != nil {
		return Product{}, err
	}
	var verr resource.ValidationError
	if c.Price == nil {
		verr.Add("price", "can't be blank")
	}
	if c.LedgerAccountID == nil {
		verr.Add("ledger_account_id", "can't be blank")
	}
	now := s.now()
	p := Product{ID: s.ids.Next(), AdministrationID: admin, Currency: a.Currency, Active: true, CreatedAt: now, UpdatedAt: now}
	if err := s.apply(ctx, user, &p, c, &verr); err != nil {
		return Product{}, err
	}
	if err := s.store.SaveProduct(ctx, p, true); err != nil {
		return Product{}, fmt.Errorf("store product: %w", err)
	}
	return p, nil
}

// Update changes a product.
func (s *Service) Update(ctx context.Context, admin, id, user resource.ID, c Changes) (Product, error) {
	if _, err := s.admins.Get(ctx, admin, user); err != nil {
		return Product{}, err
	}
	p, err := s.store.Product(ctx, admin, id)
	if err != nil {
		return Product{}, err
	}
	var verr resource.ValidationError
	if err := s.apply(ctx, user, &p, c, &verr); err != nil {
		return Product{}, err
	}
	p.UpdatedAt = s.now()
	if err := s.store.SaveProduct(ctx, p, false); err != nil {
		return Product{}, fmt.Errorf("store product: %w", err)
	}
	return p, nil
}

func text(dst *string, src *string) {
	if src != nil {
		*dst = strings.TrimSpace(*src)
	}
}

func oneOf(verr *resource.ValidationError, field string, dst *string, src *string, allowed []string) {
	if src == nil {
		return
	}
	v := strings.TrimSpace(*src)
	if v != "" && !slices.Contains(allowed, v) {
		verr.Add(field, "is not one of "+strings.Join(allowed, ", "))
		return
	}
	*dst = v
}

// optionalID sets an optional id; zero clears it.
func optionalID(dst **resource.ID, src *resource.ID) {
	if src == nil {
		return
	}
	if *src == 0 {
		*dst = nil
		return
	}
	id := *src
	*dst = &id
}

func (s *Service) apply(ctx context.Context, user resource.ID, p *Product, c Changes, verr *resource.ValidationError) error {
	admin := p.AdministrationID
	text(&p.Title, c.Title)
	text(&p.Description, c.Description)
	text(&p.CheckoutDescription, c.CheckoutDescription)
	text(&p.Identifier, c.Identifier)
	if c.Price != nil {
		price, err := money.ParseAmount(*c.Price)
		switch {
		case err != nil:
			verr.Add("price", "is not an amount")
		case price > 1_000_000_000_00 || price < -1_000_000_000_00:
			verr.Add("price", "must be from -1,000,000,000 to 1,000,000,000")
		default:
			p.Price = price
		}
	}
	if c.Currency != nil {
		p.Currency = strings.ToUpper(strings.TrimSpace(*c.Currency))
	}
	if !currencyPattern.MatchString(p.Currency) {
		verr.Add("currency", "is not a currency")
	}
	s.applyFrequency(p, c, verr)
	oneOf(verr, "product_type", &p.ProductType, c.ProductType, ProductTypes)
	oneOf(verr, "vat_rate_type", &p.VatRateType, c.VatRateType, VatRateTypes)
	oneOf(verr, "checkout_type", &p.CheckoutType, c.CheckoutType, CheckoutTypes)
	if c.MaxAmountPerOrder != nil {
		p.MaxAmountPerOrder = c.MaxAmountPerOrder
	}
	if p.MaxAmountPerOrder != nil && p.CheckoutType == "subscription" && (*p.MaxAmountPerOrder < 0 || *p.MaxAmountPerOrder > 1) {
		verr.Add("max_amount_per_order", "must be 0 or 1 for a subscription")
	}
	optionalID(&p.TaxRateID, c.TaxRateID)
	optionalID(&p.DocumentStyleID, c.DocumentStyleID)
	optionalID(&p.WorkflowID, c.WorkflowID)
	if c.LedgerAccountID != nil {
		p.LedgerAccountID = *c.LedgerAccountID
	}
	if c.Active != nil {
		p.Active = *c.Active
	}
	if p.Title == "" && p.Description == "" {
		verr.Add("title", "can't be blank")
		verr.Add("description", "can't be blank")
		verr.Add("base", "must contain a title or description")
	}
	for field, v := range map[string]string{"title": p.Title, "identifier": p.Identifier} {
		if len(v) > 255 {
			verr.Add(field, "is too long (maximum is 255 characters)")
		}
	}
	for field, v := range map[string]string{"description": p.Description, "checkout_description": p.CheckoutDescription} {
		if len(v) > 10000 {
			verr.Add(field, "is too long (maximum is 10000 characters)")
		}
	}
	if p.Identifier != "" && c.Identifier != nil {
		other, err := s.store.ProductByIdentifier(ctx, admin, p.Identifier)
		switch {
		case err == nil && other.ID != p.ID:
			verr.Add("identifier", "has already been taken")
		case err != nil && !errors.Is(err, ErrNotFound):
			return fmt.Errorf("check product identifier: %w", err)
		}
	}
	if p.TaxRateID != nil && c.TaxRateID != nil {
		if rate, err := s.ledger.TaxRate(ctx, admin, *p.TaxRateID, user); err != nil || rate.Type != "sales_invoice" {
			verr.Add("tax_rate_id", "is not a sales VAT rate")
		} else if !rate.Active {
			verr.Add("tax_rate_id", "is deactivated")
		}
	}
	if c.LedgerAccountID != nil {
		if p.LedgerAccountID == 0 {
			verr.Add("ledger_account_id", "can't be blank")
		} else if problem, err := s.ledger.Bookable(ctx, admin, user, p.LedgerAccountID, "sales_invoice"); err != nil || problem != "" {
			verr.Add("ledger_account_id", cmpString(problem, "does not exist"))
		}
	}
	return verr.OrNil()
}

func (s *Service) applyFrequency(p *Product, c Changes, verr *resource.ValidationError) {
	if c.FrequencyPreset != nil {
		m := presetPattern.FindStringSubmatch(strings.TrimSpace(*c.FrequencyPreset))
		switch {
		case m == nil:
			verr.Add("frequency_preset", "is not like 1:month")
		case m[2] == "nil":
			p.Frequency, p.FrequencyType = 0, ""
		case m[2] != "custom":
			n, _ := strconv.Atoi(m[1])
			p.Frequency, p.FrequencyType = n, m[2]
			if n < 1 {
				verr.Add("frequency_preset", "needs a frequency of 1 or more")
			}
		}
	}
	oneOf(verr, "frequency_type", &p.FrequencyType, c.FrequencyType, FrequencyTypes)
	if c.Frequency != nil {
		p.Frequency = *c.Frequency
	}
	switch {
	case p.FrequencyType == "" && c.FrequencyType != nil:
		p.Frequency = 0
	case p.FrequencyType != "" && p.Frequency == 0:
		p.Frequency = 1
	}
	if p.FrequencyType != "" && (p.Frequency < 1 || p.Frequency > 1000) {
		verr.Add("frequency", "must be from 1 to 1000")
	}
	if p.FrequencyType == "" && p.Frequency != 0 {
		verr.Add("frequency_type", "can't be blank with a frequency")
	}
}

func cmpString(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

// Delete removes a product no document names. One that documents name is
// deactivated instead, as Moneybird does, and kept reports so.
func (s *Service) Delete(ctx context.Context, admin, id, user resource.ID) (kept bool, err error) {
	if _, err := s.admins.Get(ctx, admin, user); err != nil {
		return false, err
	}
	p, err := s.store.Product(ctx, admin, id)
	if err != nil {
		return false, err
	}
	if s.references != nil {
		used, err := s.references.ProductInUse(ctx, admin, id)
		if err != nil {
			return false, fmt.Errorf("check product use: %w", err)
		}
		if used {
			p.Active, p.UpdatedAt = false, s.now()
			if err := s.store.SaveProduct(ctx, p, false); err != nil {
				return false, fmt.Errorf("deactivate product: %w", err)
			}
			return true, nil
		}
	}
	return false, s.store.DeleteProduct(ctx, admin, id)
}

// SalesLink makes a link to an online checkout page for the product, as
// Moneybird does with Moneybird Payments. MoneyHarpy has no online checkout,
// so after checking the product and the contact, it answers as Moneybird
// does for a product that is not valid for online sales.
func (s *Service) SalesLink(ctx context.Context, admin, id, user resource.ID, contact *resource.ID) (string, error) {
	if _, err := s.Product(ctx, admin, id, user); err != nil {
		return "", err
	}
	if contact != nil {
		if _, err := s.contacts.Contact(ctx, admin, *contact, user); err != nil {
			return "", err
		}
	}
	return "", ErrNotForOnlineSales
}
