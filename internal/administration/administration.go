// Package administration holds the company accounts that all bookkeeping data
// belongs to, and the rules for who can reach them.
package administration

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/adirkuhn/moneyharpy/internal/resource"
)

// Errors returned by Service, Store and the onboarding lookups.
var (
	// ErrNotFound reports an administration that does not exist or that the
	// user has no access to. The two are indistinguishable so IDs cannot be
	// probed.
	ErrNotFound = errors.New("administration not found")
	// ErrLookupUnavailable reports a lookup this installation has not enabled.
	ErrLookupUnavailable = errors.New("lookup unavailable")
	// ErrAddressUnknown reports a zipcode and house number that match nothing.
	ErrAddressUnknown = errors.New("address unknown")
)

// Access describes how a user reaches an administration.
type Access string

// The ways a user can have access to an administration.
const (
	AccessUser              Access = "user"
	AccessAccountantCompany Access = "accountant_company"
	// AccessSupport is an instance admin let in through support access.
	AccessSupport Access = "support"
)

// UserType is a member's role, with Moneybird's names.
type UserType string

// The user types members can have.
const (
	Owner      UserType = "owner"
	Employee   UserType = "employee"
	Accountant UserType = "accountant"
	// Support is an instance admin let in through support access.
	Support UserType = "support"
)

// Permission is something a member may do, with Moneybird's names. Each one
// opens the API resources of the scope with the same name; ownership opens
// managing the administration itself.
type Permission string

// The permissions, in Moneybird's order.
const (
	SalesInvoices Permission = "sales_invoices"
	Documents     Permission = "documents"
	Estimates     Permission = "estimates"
	Bank          Permission = "bank"
	Settings      Permission = "settings"
	Ownership     Permission = "ownership"
	TimeEntries   Permission = "time_entries"
)

// Permissions lists every permission.
var Permissions = []Permission{SalesInvoices, Documents, Estimates, Bank, Settings, Ownership, TimeEntries}

// Administration is one company account.
type Administration struct {
	ID                resource.ID
	Name              string
	Language          string
	Currency          string
	Country           string
	TimeZone          string
	Suspended         bool
	PeriodLockedUntil *string
	PeriodStartDate   string
	Profile
	// RequireTwoFactor makes every member sign in with two-step
	// verification before using the administration.
	RequireTwoFactor bool
	// SupportAccessUntil lets the instance admins in until then.
	SupportAccessUntil *time.Time
}

// Profile describes what the company does. Moneybird keeps these on the
// administration and shows them on the company details form.
type Profile struct {
	// SmallBusinessScheme reports registration for the Dutch small business
	// scheme (KOR), under which no VAT is charged.
	SmallBusinessScheme          bool
	CompanyActivitiesDescription string
	CompanyWebsiteURL            string
	// CompanyWebsiteAbsent confirms the company has no website. It is false
	// whenever a website is set.
	CompanyWebsiteAbsent bool
}

// CompanyMatch is a company found in a public register. Address parts the
// register does not return are empty.
type CompanyMatch struct {
	Name              string
	ChamberOfCommerce string
	Address1          string
	Zipcode           string
	City              string
}

// AddressMatch is an address found by zipcode and house number.
type AddressMatch struct {
	Address1 string
	Zipcode  string
	City     string
}

// Ref locates the administration.
func (a Administration) Ref() resource.Ref {
	return resource.Ref{ID: a.ID, Type: resource.TypeAdministration, AdministrationID: a.ID}
}

// Membership is one user's relationship to an administration.
type Membership struct {
	Access   Access
	Favorite bool
	UserType UserType
	// Permissions are what the user may do, after Moneybird's permissions.
	Permissions []Permission
}

// Can reports whether the member holds at least one of the permissions. No
// permissions asks only for membership.
func (m Membership) Can(perms ...Permission) bool {
	if len(perms) == 0 {
		return true
	}
	for _, p := range perms {
		if slices.Contains(m.Permissions, p) {
			return true
		}
	}
	return false
}

// Listed is an administration as seen by one user.
type Listed struct {
	Administration
	Membership
}

// NewAdministration is the input for creating an administration. Empty optional
// fields take their defaults.
type NewAdministration struct {
	Name     string
	Language string
	Currency string
	Country  string
	TimeZone string

	Address1          string
	Address2          string
	Zipcode           string
	City              string
	ChamberOfCommerce string
}

// Store persists administrations. Methods reading or changing a single
// administration return ErrNotFound when the user has no access to it.
type Store interface {
	// Create stores an administration with its owner and default identity.
	Create(ctx context.Context, a Administration, owner resource.ID, def Identity) error
	Update(ctx context.Context, a Administration) error
	ListForUser(ctx context.Context, user resource.ID) ([]Listed, error)
	ForUser(ctx context.Context, id, user resource.ID) (Listed, error)
	SetFavorite(ctx context.Context, id, user resource.ID, favorite bool) error

	// The identity methods trust the caller to have checked access. Identity
	// and DeleteIdentity return ErrIdentityNotFound for an identity outside
	// the administration.
	ListIdentities(ctx context.Context, admin resource.ID) ([]Identity, error)
	DefaultIdentity(ctx context.Context, admin resource.ID) (Identity, error)
	Identity(ctx context.Context, admin, id resource.ID) (Identity, error)
	CreateIdentity(ctx context.Context, i Identity) error
	UpdateIdentity(ctx context.Context, i Identity) error
	DeleteIdentity(ctx context.Context, admin, id resource.ID) error

	// The custom field methods trust the caller to have checked access.
	CustomFields(ctx context.Context, admin resource.ID) ([]CustomField, error)
	CreateCustomField(ctx context.Context, f CustomField) error
	// CustomField returns ErrCustomFieldNotFound for a field outside admin.
	CustomField(ctx context.Context, admin, id resource.ID) (CustomField, error)
	UpdateCustomField(ctx context.Context, f CustomField) error
	DeleteCustomField(ctx context.Context, admin, id resource.ID) error
}

// Service implements the administration use cases.
type Service struct {
	store Store
	ids   *resource.Generator
	now   func() time.Time
}

// NewService returns a Service using store for persistence.
func NewService(store Store, ids *resource.Generator, now func() time.Time) *Service {
	return &Service{store: store, ids: ids, now: now}
}

// Create adds an administration owned by owner, with a default identity made
// from the company details.
func (s *Service) Create(ctx context.Context, owner resource.ID, in NewAdministration) (Listed, error) {
	a := Administration{
		Name:            strings.TrimSpace(in.Name),
		Language:        cmp.Or(in.Language, "en"),
		Currency:        cmp.Or(in.Currency, "EUR"),
		Country:         cmp.Or(in.Country, "NL"),
		TimeZone:        cmp.Or(in.TimeZone, "Europe/Amsterdam"),
		PeriodStartDate: fmt.Sprintf("%d-01-01", s.now().Year()),
	}
	if err := validate(a); err != nil {
		return Listed{}, err
	}
	a.ID = s.ids.Next()

	now := s.now()
	def := Identity{ID: s.ids.Next(), AdministrationID: a.ID, Default: true, CreatedAt: now, UpdatedAt: now}
	IdentityChanges{
		CompanyName:       &a.Name,
		Country:           &a.Country,
		Address1:          &in.Address1,
		Address2:          &in.Address2,
		Zipcode:           &in.Zipcode,
		City:              &in.City,
		ChamberOfCommerce: &in.ChamberOfCommerce,
	}.applyTo(&def)

	if err := s.store.Create(ctx, a, owner, def); err != nil {
		return Listed{}, fmt.Errorf("store administration: %w", err)
	}
	return Listed{Administration: a, Membership: Membership{Access: AccessUser, UserType: Owner, Permissions: slices.Clone(Permissions)}}, nil
}

// Changes holds the administration fields to set. Nil fields stay as they are.
type Changes struct {
	Name     *string
	Country  *string
	Language *string
	Currency *string
	TimeZone *string

	SmallBusinessScheme          *bool
	CompanyActivitiesDescription *string
	CompanyWebsiteURL            *string
	CompanyWebsiteAbsent         *bool
}

// Update changes an administration's own settings.
func (s *Service) Update(ctx context.Context, id, user resource.ID, c Changes) (Listed, error) {
	l, err := s.store.ForUser(ctx, id, user)
	if err != nil {
		return Listed{}, err
	}
	if c.Name != nil {
		l.Name = strings.TrimSpace(*c.Name)
	}
	if c.Language != nil {
		l.Language = *c.Language
	}
	if c.Country != nil {
		l.Country = strings.ToUpper(strings.TrimSpace(*c.Country))
	}
	if c.Currency != nil {
		l.Currency = *c.Currency
	}
	if c.TimeZone != nil {
		l.TimeZone = *c.TimeZone
	}
	if c.SmallBusinessScheme != nil {
		l.SmallBusinessScheme = *c.SmallBusinessScheme
	}
	if c.CompanyActivitiesDescription != nil {
		l.CompanyActivitiesDescription = strings.TrimSpace(*c.CompanyActivitiesDescription)
	}
	if c.CompanyWebsiteURL != nil {
		l.CompanyWebsiteURL = strings.TrimSpace(*c.CompanyWebsiteURL)
	}
	if c.CompanyWebsiteAbsent != nil {
		l.CompanyWebsiteAbsent = *c.CompanyWebsiteAbsent
	}
	if l.CompanyWebsiteURL != "" {
		l.CompanyWebsiteAbsent = false
	}
	if err := validate(l.Administration); err != nil {
		return Listed{}, err
	}
	if err := s.store.Update(ctx, l.Administration); err != nil {
		return Listed{}, fmt.Errorf("store administration: %w", err)
	}
	return l, nil
}

func validate(a Administration) error {
	var verr resource.ValidationError
	if a.Name == "" {
		verr.Add("name", "can't be blank")
	}
	if !slices.Contains([]string{"nl", "nl-be", "en"}, a.Language) {
		verr.Add("language", "is not included in the list")
	}
	if !slices.Contains(currencies, a.Currency) {
		verr.Add("currency", "must be an ISO 4217 code")
	}
	if !slices.Contains(countries, a.Country) {
		verr.Add("country", "must be an ISO 3166-1 alpha-2 code")
	}
	if _, err := time.LoadLocation(a.TimeZone); err != nil {
		verr.Add("time_zone", "is not a known time zone")
	}
	if a.CompanyWebsiteURL != "" && !validWebsite(a.CompanyWebsiteURL) {
		verr.Add("company_website_url", "is not a valid website address")
	}
	return verr.OrNil()
}

// validWebsite accepts absolute http and https URLs with a host.
func validWebsite(s string) bool {
	u, err := url.Parse(s)
	return err == nil && (u.Scheme == "http" || u.Scheme == "https") && u.Host != ""
}

// List returns the administrations user can access, favorites first and then
// alphabetically.
func (s *Service) List(ctx context.Context, user resource.ID) ([]Listed, error) {
	listed, err := s.store.ListForUser(ctx, user)
	if err != nil {
		return nil, fmt.Errorf("list administrations: %w", err)
	}
	slices.SortStableFunc(listed, func(a, b Listed) int {
		if a.Favorite != b.Favorite {
			if a.Favorite {
				return -1
			}
			return 1
		}
		return cmp.Or(
			cmp.Compare(strings.ToLower(a.Name), strings.ToLower(b.Name)),
			cmp.Compare(a.ID, b.ID),
		)
	})
	return listed, nil
}

// Get returns one administration as seen by user.
func (s *Service) Get(ctx context.Context, id, user resource.ID) (Listed, error) {
	return s.store.ForUser(ctx, id, user)
}

// SetFavorite marks or unmarks an administration as one of user's favorites.
func (s *Service) SetFavorite(ctx context.Context, id, user resource.ID, favorite bool) error {
	return s.store.SetFavorite(ctx, id, user, favorite)
}
