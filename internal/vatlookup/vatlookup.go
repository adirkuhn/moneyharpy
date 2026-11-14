// Package vatlookup fills in a contact from its EU VAT number, as registered
// in VIES, the European Commission's register of EU VAT numbers. It asks
// only when an administrator turned the lookup on, and sends only the VAT
// number.
package vatlookup

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/adirkuhn/moneyharpy/internal/recognize"
	"github.com/adirkuhn/moneyharpy/internal/settings"
)

var (
	// ErrDisabled reports that no administrator turned the lookup on.
	ErrDisabled = errors.New("the EU VAT number lookup is turned off")
	// ErrInvalidNumber reports a number that is not shaped like an EU VAT
	// number, so VIES is not asked.
	ErrInvalidNumber = errors.New("not an EU VAT number")
	// ErrUnavailable reports that VIES could not answer.
	ErrUnavailable = errors.New("VIES unavailable")
)

// Settings tells whether the lookup is on.
type Settings interface {
	Instance(ctx context.Context) (settings.Instance, error)
}

// Company is what VIES registered for a VAT number. When it is not valid
// only Country is filled in.
type Company struct {
	// Valid tells the number is registered and active.
	Valid bool
	Name  string
	// Address1, Zipcode and City split the registered address where its
	// postcode can be read; otherwise the whole address is in Address1.
	Address1 string
	Zipcode  string
	City     string
	// Country is the ISO country code of the VAT number.
	Country string
}

// Service looks VAT numbers up.
type Service struct {
	settings Settings
	registry recognize.Registry
}

// New returns a Service asking registry while the settings allow it.
func New(s Settings, registry recognize.Registry) *Service {
	return &Service{settings: s, registry: registry}
}

// Lookup returns what VIES registered for vat, given in any common print
// form.
func (s *Service) Lookup(ctx context.Context, vat string) (Company, error) {
	inst, err := s.settings.Instance(ctx)
	if err != nil {
		return Company{}, err
	}
	if !inst.VIESEnabled || s.registry == nil {
		return Company{}, ErrDisabled
	}
	n := recognize.NormalizeVAT(vat)
	if !recognize.InVIES(n) || !recognize.ValidVAT(n) {
		return Company{}, ErrInvalidNumber
	}
	found, err := s.registry.LookupVAT(ctx, n)
	if err != nil {
		return Company{}, fmt.Errorf("%w: %w", ErrUnavailable, err)
	}
	out := Company{Valid: found.Valid, Country: recognize.VATCountry(n)}
	if !found.Valid {
		return out, nil
	}
	out.Name = strings.Join(strings.Fields(found.Name), " ")
	out.Address1, out.Zipcode, out.City = recognize.SplitRegisteredAddress(found.Address, n)
	return out, nil
}
