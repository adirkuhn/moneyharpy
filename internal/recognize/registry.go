package recognize

import (
	"context"
	"strings"
)

// Registry looks VAT numbers up in a public register of companies, such as
// the EU's VIES. It gets the VAT number only.
type Registry interface {
	LookupVAT(ctx context.Context, vat string) (RegisteredCompany, error)
}

// RegisteredCompany is what a register says of a VAT number.
type RegisteredCompany struct {
	// Valid tells the number is registered and active.
	Valid bool
	// Name is the registered name; empty when the register does not tell.
	Name string
	// Address is the registered address, its lines separated by "\n".
	Address string
}

// viesCountries are the VAT number prefixes VIES answers for: the EU
// member states and Northern Ireland.
var viesCountries = map[string]bool{
	"AT": true, "BE": true, "BG": true, "CY": true, "CZ": true, "DE": true, "DK": true, "EE": true, "EL": true,
	"ES": true, "FI": true, "FR": true, "HR": true, "HU": true, "IE": true, "IT": true, "LT": true, "LU": true,
	"LV": true, "MT": true, "NL": true, "PL": true, "PT": true, "RO": true, "SE": true, "SI": true, "SK": true,
	"XI": true,
}

// InVIES tells whether VIES knows VAT numbers like vat.
func InVIES(vat string) bool {
	n := NormalizeVAT(vat)
	return len(n) > 2 && viesCountries[n[:2]]
}

// fromRegistry completes a supplier that is to be added as a contact with
// its name, and its address when the document gave none, as registered for
// its VAT number: OCR often garbles the name printed on a receipt. It asks
// only when an administrator turned the lookup on, sends only the VAT
// number, and keeps what the document says when the register cannot
// answer.
func (s *Service) fromRegistry(ctx context.Context, sup Supplier) Supplier {
	if s.Registry == nil || s.Settings == nil || !ValidVAT(sup.VATNumber) || !InVIES(sup.VATNumber) {
		return sup
	}
	inst, err := s.Settings.Instance(ctx)
	if err != nil || !inst.VIESEnabled {
		return sup
	}
	c, err := s.Registry.LookupVAT(ctx, NormalizeVAT(sup.VATNumber))
	if err != nil {
		s.Logger.WarnContext(ctx, "scan and recognise: VAT number lookup failed", "error", err)
		return sup
	}
	if !c.Valid || strings.TrimSpace(c.Name) == "" {
		return sup
	}
	sup.Name = strings.TrimSpace(c.Name)
	if sup.Address1 == "" && sup.Zipcode == "" && sup.City == "" {
		sup.Address1, sup.Zipcode, sup.City = registeredAddress(c.Address, countryOf(NormalizeVAT(sup.VATNumber)))
	}
	return sup
}

// registeredAddress splits an address as a register prints it ("street
// 1\n1234 AB CITY") into the street and the postcode and city of its last
// line; nothing when the last line holds no postcode.
func registeredAddress(addr, country string) (address1, zipcode, city string) {
	var lines []string
	for _, l := range strings.Split(addr, "\n") {
		if l = strings.Join(strings.Fields(l), " "); l != "" {
			lines = append(lines, l)
		}
	}
	if len(lines) < 2 {
		return "", "", ""
	}
	zipcode, city, _ = postcode(lines[len(lines)-1], country)
	if zipcode == "" {
		return "", "", ""
	}
	return strings.Join(lines[:len(lines)-1], ", "), zipcode, city
}

// SplitRegisteredAddress splits an address as a register such as VIES prints
// it, one block of lines, into the street and the postcode and city of its
// last line, reading postcodes as written in the country of the VAT number.
// When it finds no postcode it keeps the whole address, its lines joined by
// commas, as the street.
func SplitRegisteredAddress(addr, vat string) (address1, zipcode, city string) {
	if address1, zipcode, city = registeredAddress(addr, VATCountry(vat)); zipcode != "" {
		return address1, zipcode, city
	}
	var lines []string
	for _, l := range strings.Split(addr, "\n") {
		if l = strings.Join(strings.Fields(l), " "); l != "" {
			lines = append(lines, l)
		}
	}
	return strings.Join(lines, ", "), "", ""
}

// VATCountry is the country code of a VAT number: its prefix, except EL for
// Greece (GR) and XI for Northern Ireland (GB).
func VATCountry(vat string) string { return countryOf(NormalizeVAT(vat)) }
