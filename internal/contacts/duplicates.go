package contacts

import (
	"context"
	"fmt"
	"strings"

	"github.com/adirkuhn/moneyharpy/internal/resource"
)

// Fields a registry number duplicate is found on.
const (
	FieldChamberOfCommerce = "chamber_of_commerce"
	FieldTaxNumber         = "tax_number"
)

// Duplicate is another contact that already holds a registry number.
type Duplicate struct {
	// Field is FieldChamberOfCommerce or FieldTaxNumber.
	Field   string
	Contact Contact
}

// NormalizeChamberOfCommerce keeps the digits of a chamber of commerce
// number, so "1234 5678" and "12345678" are the same. A number without
// digits, as some foreign registers write them, is kept upper-cased
// without separators instead.
func NormalizeChamberOfCommerce(s string) string {
	digits := strings.Map(func(r rune) rune {
		if r >= '0' && r <= '9' {
			return r
		}
		return -1
	}, s)
	if digits != "" {
		return digits
	}
	return NormalizeVAT(s)
}

// NormalizeVAT upper-cases a VAT identification number and removes spaces,
// dots and dashes, by the same rules as recognize.NormalizeVAT (which
// cannot be imported here, as it imports this package).
func NormalizeVAT(s string) string {
	return strings.ToUpper(strings.Map(func(r rune) rune {
		switch r {
		case ' ', '.', '-', ' ', '\t':
			return -1
		}
		return r
	}, strings.TrimSpace(s)))
}

// RegistryDuplicates returns the other contacts of the administration that
// are not archived and already hold the chamber of commerce number or the
// VAT number, compared after normalising both. The contact except (zero
// for a new one) is left out, so a contact never duplicates itself.
//
// Moneybird's API accepts such contacts and lists them as possible
// doubles afterwards, so creating and updating do not refuse them; this
// lets a client warn before saving.
func (s *Service) RegistryDuplicates(ctx context.Context, admin, user resource.ID, chamberOfCommerce, taxNumber string, except resource.ID) ([]Duplicate, error) {
	if _, err := s.admins.Get(ctx, admin, user); err != nil {
		return nil, err
	}
	coc, tax := NormalizeChamberOfCommerce(chamberOfCommerce), NormalizeVAT(taxNumber)
	if coc == "" && tax == "" {
		return []Duplicate{}, nil
	}
	all, _, err := s.store.Contacts(ctx, admin, Query{})
	if err != nil {
		return nil, fmt.Errorf("list contacts: %w", err)
	}
	out := []Duplicate{}
	for _, field := range []string{FieldChamberOfCommerce, FieldTaxNumber} {
		for _, c := range all {
			if c.ID == except || c.Archived {
				continue
			}
			if (field == FieldChamberOfCommerce && coc != "" && NormalizeChamberOfCommerce(c.ChamberOfCommerce) == coc) ||
				(field == FieldTaxNumber && tax != "" && NormalizeVAT(c.TaxNumber) == tax) {
				out = append(out, Duplicate{Field: field, Contact: c})
				break
			}
		}
	}
	return out, nil
}
