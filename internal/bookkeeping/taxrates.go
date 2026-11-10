package bookkeeping

import (
	"cmp"
	"context"
	"fmt"
	"math/big"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/adirkuhn/moneyharpy/internal/bookkeeping/countries"
	"github.com/adirkuhn/moneyharpy/internal/resource"
)

// TaxRateTypes are the documents a VAT rate applies to, with Moneybird's
// names.
var TaxRateTypes = []string{"sales_invoice", "purchase_invoice", "general_journal_document"}

var percentagePattern = regexp.MustCompile(`^\d{1,3}(\.\d{1,2})?$`)

// TaxRate is a VAT rate.
type TaxRate struct {
	ID               resource.ID
	AdministrationID resource.ID
	Name             string
	// Percentage is a decimal such as "21" or "9.5".
	Percentage string
	Type       string
	// ShowTax prints the VAT on the invoice. Reverse-charged and exempt
	// rates leave it off.
	ShowTax bool
	Active  bool
	// Country is set for rates of another country, such as for EU
	// distance sales.
	Country string
	// ReportBoxes are the boxes of the VAT return the rate counts in.
	ReportBoxes []string
	// Position orders the rates of one type, starting at 1.
	Position  int
	CreatedAt time.Time
	UpdatedAt time.Time
}

// Ref locates the tax rate.
func (r TaxRate) Ref() resource.Ref {
	return resource.Ref{ID: r.ID, Type: resource.TypeTaxRate, AdministrationID: r.AdministrationID}
}

// TaxRateChanges holds the fields to set. Nil fields stay as they are.
type TaxRateChanges struct {
	Name        *string
	Percentage  *string
	Type        *string
	ShowTax     *bool
	Active      *bool
	Country     *string
	ReportBoxes *[]string
}

// TaxRateFilter narrows a list of tax rates, after Moneybird's filters. Nil
// fields do not filter.
type TaxRateFilter struct {
	Name, PartialName, Type, Country *string
	Percentage                       *string
	ShowTax, Active                  *bool
	CreatedAfter, UpdatedAfter       *time.Time
}

func (f TaxRateFilter) match(r TaxRate) bool {
	eqNumber := func(a, b string) bool {
		x, okx := new(big.Rat).SetString(a)
		y, oky := new(big.Rat).SetString(b)
		return okx && oky && x.Cmp(y) == 0
	}
	return (f.Name == nil || r.Name == *f.Name) &&
		(f.PartialName == nil || strings.Contains(strings.ToLower(r.Name), strings.ToLower(*f.PartialName))) &&
		(f.Type == nil || r.Type == *f.Type) &&
		(f.Country == nil || r.Country == *f.Country) &&
		(f.Percentage == nil || eqNumber(r.Percentage, *f.Percentage)) &&
		(f.ShowTax == nil || r.ShowTax == *f.ShowTax) &&
		(f.Active == nil || r.Active == *f.Active) &&
		(f.CreatedAfter == nil || r.CreatedAt.After(*f.CreatedAfter)) &&
		(f.UpdatedAfter == nil || r.UpdatedAt.After(*f.UpdatedAfter))
}

// TaxRates lists the administration's VAT rates by type and position.
func (s *Service) TaxRates(ctx context.Context, admin, user resource.ID, f TaxRateFilter) ([]TaxRate, error) {
	if _, err := s.ensure(ctx, admin, user); err != nil {
		return nil, err
	}
	rates, err := s.store.TaxRates(ctx, admin)
	if err != nil {
		return nil, fmt.Errorf("list tax rates: %w", err)
	}
	rates = slices.DeleteFunc(rates, func(r TaxRate) bool { return !f.match(r) })
	slices.SortStableFunc(rates, func(a, b TaxRate) int {
		return cmp.Or(
			cmp.Compare(slices.Index(TaxRateTypes, a.Type), slices.Index(TaxRateTypes, b.Type)),
			cmp.Compare(a.Position, b.Position), cmp.Compare(a.ID, b.ID),
		)
	})
	return rates, nil
}

// TaxRate returns one VAT rate.
func (s *Service) TaxRate(ctx context.Context, admin, id, user resource.ID) (TaxRate, error) {
	if _, err := s.ensure(ctx, admin, user); err != nil {
		return TaxRate{}, err
	}
	return s.store.TaxRate(ctx, admin, id)
}

// CreateTaxRate adds a VAT rate, last among its type.
func (s *Service) CreateTaxRate(ctx context.Context, admin, user resource.ID, c TaxRateChanges) (TaxRate, error) {
	a, err := s.ensure(ctx, admin, user)
	if err != nil {
		return TaxRate{}, err
	}
	now := s.now()
	r := TaxRate{ID: s.ids.Next(), AdministrationID: admin, Active: true, ShowTax: true, CreatedAt: now, UpdatedAt: now}
	if err := applyTaxRate(&r, c, countries.For(a.Country).Boxes); err != nil {
		return TaxRate{}, err
	}
	existing, err := s.store.TaxRates(ctx, admin)
	if err != nil {
		return TaxRate{}, fmt.Errorf("list tax rates: %w", err)
	}
	for _, e := range existing {
		if e.Type == r.Type {
			r.Position = max(r.Position, e.Position)
		}
	}
	r.Position++
	if err := s.store.CreateTaxRate(ctx, r); err != nil {
		return TaxRate{}, fmt.Errorf("store tax rate: %w", err)
	}
	return r, nil
}

// UpdateTaxRate changes a VAT rate. A changed type moves it to the end of
// its new type.
func (s *Service) UpdateTaxRate(ctx context.Context, admin, id, user resource.ID, c TaxRateChanges) (TaxRate, error) {
	a, err := s.ensure(ctx, admin, user)
	if err != nil {
		return TaxRate{}, err
	}
	r, err := s.store.TaxRate(ctx, admin, id)
	if err != nil {
		return TaxRate{}, err
	}
	oldType := r.Type
	if err := applyTaxRate(&r, c, countries.For(a.Country).Boxes); err != nil {
		return TaxRate{}, err
	}
	if r.Type != oldType {
		existing, err := s.store.TaxRates(ctx, admin)
		if err != nil {
			return TaxRate{}, fmt.Errorf("list tax rates: %w", err)
		}
		r.Position = 0
		for _, e := range existing {
			if e.Type == r.Type {
				r.Position = max(r.Position, e.Position)
			}
		}
		r.Position++
	}
	r.UpdatedAt = s.now()
	if err := s.store.UpdateTaxRates(ctx, []TaxRate{r}); err != nil {
		return TaxRate{}, fmt.Errorf("store tax rate: %w", err)
	}
	return r, nil
}

// ReorderTaxRates puts the rates of one type in the order of ids, which must
// name each of them once.
func (s *Service) ReorderTaxRates(ctx context.Context, admin, user resource.ID, rateType string, ids []resource.ID) error {
	rates, err := s.TaxRates(ctx, admin, user, TaxRateFilter{Type: &rateType})
	if err != nil {
		return err
	}
	current := make([]resource.ID, len(rates))
	byID := map[resource.ID]TaxRate{}
	for i, r := range rates {
		current[i], byID[r.ID] = r.ID, r
	}
	if !slices.Equal(slices.Sorted(slices.Values(current)), slices.Sorted(slices.Values(ids))) {
		var verr resource.ValidationError
		verr.Add("ids", "must list every tax rate of the type once")
		return &verr
	}
	var changed []TaxRate
	for i, id := range ids {
		r := byID[id]
		if r.Position != i+1 {
			r.Position, r.UpdatedAt = i+1, s.now()
			changed = append(changed, r)
		}
	}
	if len(changed) == 0 {
		return nil
	}
	return s.store.UpdateTaxRates(ctx, changed)
}

func applyTaxRate(r *TaxRate, c TaxRateChanges, boxes []countries.Box) error {
	if c.Name != nil {
		r.Name = strings.TrimSpace(*c.Name)
	}
	if c.Percentage != nil {
		r.Percentage = normalizePercentage(*c.Percentage)
	}
	if c.Type != nil {
		r.Type = *c.Type
	}
	if c.ShowTax != nil {
		r.ShowTax = *c.ShowTax
	}
	if c.Active != nil {
		r.Active = *c.Active
	}
	if c.Country != nil {
		r.Country = strings.ToUpper(strings.TrimSpace(*c.Country))
	}
	if c.ReportBoxes != nil {
		r.ReportBoxes = slices.Compact(slices.Sorted(slices.Values(*c.ReportBoxes)))
	}
	var verr resource.ValidationError
	if r.Name == "" || len(r.Name) > 255 {
		verr.Add("name", "can't be blank")
	}
	if !percentagePattern.MatchString(r.Percentage) {
		verr.Add("percentage", "must be a number from 0 to 100 with at most 2 decimals")
	} else if p, _ := new(big.Rat).SetString(r.Percentage); p.Cmp(big.NewRat(100, 1)) > 0 {
		verr.Add("percentage", "must be a number from 0 to 100 with at most 2 decimals")
	}
	if !slices.Contains(TaxRateTypes, r.Type) {
		verr.Add("tax_rate_type", "is not included in the list")
	}
	if r.Country != "" && len(r.Country) != 2 {
		verr.Add("country", "must be an ISO 3166-1 alpha-2 code")
	}
	for _, b := range r.ReportBoxes {
		if !slices.ContainsFunc(boxes, func(x countries.Box) bool { return x.Code == b }) {
			verr.Add("tax_report_boxes", fmt.Sprintf("contains %s, which is not a box of this country's VAT return", b))
		}
	}
	return verr.OrNil()
}

// normalizePercentage accepts a comma as decimal separator and drops
// trailing zeros: "9,50" becomes "9.5".
func normalizePercentage(s string) string {
	s = strings.ReplaceAll(strings.TrimSpace(s), ",", ".")
	if strings.Contains(s, ".") {
		s = strings.TrimRight(strings.TrimRight(s, "0"), ".")
	}
	return s
}
