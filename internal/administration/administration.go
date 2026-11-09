// Package administration holds the company accounts that all bookkeeping data
// belongs to, and the rules for who can reach them.
package administration

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/adirkuhn/moneyharpy/internal/resource"
)

// ErrNotFound reports an administration that does not exist or that the user
// has no access to. The two are indistinguishable so IDs cannot be probed.
var ErrNotFound = errors.New("administration not found")

// Access describes how a user reaches an administration.
type Access string

// The ways a user can have access to an administration.
const (
	AccessUser              Access = "user"
	AccessAccountantCompany Access = "accountant_company"
)

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
}

// Ref locates the administration.
func (a Administration) Ref() resource.Ref {
	return resource.Ref{ID: a.ID, Type: resource.TypeAdministration, AdministrationID: a.ID}
}

// Membership is one user's relationship to an administration.
type Membership struct {
	Access   Access
	Favorite bool
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
}

// Store persists administrations. Methods reading or changing a single
// administration return ErrNotFound when the user has no access to it.
type Store interface {
	Create(ctx context.Context, a Administration, owner resource.ID) error
	ListForUser(ctx context.Context, user resource.ID) ([]Listed, error)
	ForUser(ctx context.Context, id, user resource.ID) (Listed, error)
	SetFavorite(ctx context.Context, id, user resource.ID, favorite bool) error
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

var (
	currencyPattern = regexp.MustCompile(`^[A-Z]{3}$`)
	countryPattern  = regexp.MustCompile(`^[A-Z]{2}$`)
)

// Create adds an administration owned by owner.
func (s *Service) Create(ctx context.Context, owner resource.ID, in NewAdministration) (Listed, error) {
	a, err := s.build(in)
	if err != nil {
		return Listed{}, err
	}
	if err := s.store.Create(ctx, a, owner); err != nil {
		return Listed{}, fmt.Errorf("store administration: %w", err)
	}
	return Listed{Administration: a, Membership: Membership{Access: AccessUser}}, nil
}

func (s *Service) build(in NewAdministration) (Administration, error) {
	a := Administration{
		Name:            strings.TrimSpace(in.Name),
		Language:        cmp.Or(in.Language, "en"),
		Currency:        cmp.Or(in.Currency, "EUR"),
		Country:         cmp.Or(in.Country, "NL"),
		TimeZone:        cmp.Or(in.TimeZone, "Europe/Amsterdam"),
		PeriodStartDate: fmt.Sprintf("%d-01-01", s.now().Year()),
	}

	var verr resource.ValidationError
	if a.Name == "" {
		verr.Add("name", "can't be blank")
	}
	if !slices.Contains([]string{"nl", "nl-be", "en"}, a.Language) {
		verr.Add("language", "is not included in the list")
	}
	if !currencyPattern.MatchString(a.Currency) {
		verr.Add("currency", "must be an ISO 4217 code")
	}
	if !countryPattern.MatchString(a.Country) {
		verr.Add("country", "must be an ISO 3166-1 alpha-2 code")
	}
	if _, err := time.LoadLocation(a.TimeZone); err != nil {
		verr.Add("time_zone", "is not a known time zone")
	}
	if err := verr.OrNil(); err != nil {
		return Administration{}, err
	}

	a.ID = s.ids.Next()
	return a, nil
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
