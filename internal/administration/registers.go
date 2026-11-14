package administration

import (
	"context"
	"errors"
)

// Errors a company register can answer with besides ErrLookupUnavailable.
// They tell people why a search failed, so they fill in the details by hand.
var (
	// ErrRegisterUnavailable reports a register that did not answer: the
	// network failed, it timed out or it had a server error.
	ErrRegisterUnavailable = errors.New("company register unavailable")
	// ErrRegisterBlocked reports a register that refused the search or
	// answered with something other than results, such as a challenge page.
	ErrRegisterBlocked = errors.New("company register blocked the request")
	// ErrRegisterBusy reports too many searches from this installation in a
	// short time.
	ErrRegisterBusy = errors.New("too many company register searches")
)

// CompanySearcher finds companies in a company register by name.
type CompanySearcher interface {
	SearchCompanies(ctx context.Context, query string) ([]CompanyMatch, error)
}

// Registers searches with the first register this installation has enabled,
// in order of preference. Registers that are not enabled answer
// ErrLookupUnavailable; a failure of an enabled one is returned as is, so a
// search never silently moves on to a register further down the list.
type Registers []CompanySearcher

// SearchCompanies implements CompanySearcher.
func (rs Registers) SearchCompanies(ctx context.Context, query string) ([]CompanyMatch, error) {
	for _, r := range rs {
		found, err := r.SearchCompanies(ctx, query)
		if errors.Is(err, ErrLookupUnavailable) {
			continue
		}
		return found, err
	}
	return nil, ErrLookupUnavailable
}

// ErrCompanyNotFound reports a company number the register does not know.
var ErrCompanyNotFound = errors.New("company not found in the register")

// CompanyLookup finds one company in a company register by its number.
type CompanyLookup interface {
	LookupCompany(ctx context.Context, number string) (CompanyMatch, error)
}

// LookupCompany implements CompanyLookup with the first enabled register
// that can look companies up by number, like SearchCompanies.
func (rs Registers) LookupCompany(ctx context.Context, number string) (CompanyMatch, error) {
	for _, r := range rs {
		l, ok := r.(CompanyLookup)
		if !ok {
			continue
		}
		found, err := l.LookupCompany(ctx, number)
		if errors.Is(err, ErrLookupUnavailable) {
			continue
		}
		return found, err
	}
	return CompanyMatch{}, ErrLookupUnavailable
}
