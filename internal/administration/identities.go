package administration

import (
	"context"
	"errors"
	"fmt"
	"net/mail"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/adirkuhn/moneyharpy/internal/iban"

	"github.com/adirkuhn/moneyharpy/internal/resource"
)

// Errors returned for identities.
var (
	ErrIdentityNotFound = errors.New("identity not found")
	ErrDefaultIdentity  = errors.New("default identity cannot be deleted")
)

var bicPattern = regexp.MustCompile(`^[A-Z]{6}[A-Z0-9]{2}([A-Z0-9]{3})?$`)

// Identity is a name and address an administration trades and invoices under.
// The default one holds the company's own details.
type Identity struct {
	ID                resource.ID
	AdministrationID  resource.ID
	Default           bool
	CompanyName       string
	Country           string
	City              string
	Zipcode           string
	Address1          string
	Address2          string
	Email             string
	Phone             string
	LegalEntityType   string
	ChamberOfCommerce string
	TaxNumber         string
	BankAccountName   string
	BankAccountNumber string
	BankAccountBIC    string
	// TaxNumberAbsenceReason explains a missing VAT number. It is empty
	// whenever TaxNumber is set.
	TaxNumberAbsenceReason TaxNumberAbsenceReason
	// CustomFields holds the identity's non-empty custom field values.
	CustomFields []CustomFieldValue
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// TaxNumberAbsenceReason is why a company has no VAT number.
type TaxNumberAbsenceReason string

// The reasons a company may lack a VAT number.
const (
	BelowTaxThreshold TaxNumberAbsenceReason = "below_tax_threshold"
	IndustryExemption TaxNumberAbsenceReason = "industry_exemption"
)

// CustomFieldValue is the value an identity holds for a custom field.
type CustomFieldValue struct {
	ID    resource.ID
	Name  string
	Value string
}

// CustomFieldChange sets one custom field value. An empty value removes it.
type CustomFieldChange struct {
	ID    resource.ID
	Value string
}

// Ref locates the identity.
func (i Identity) Ref() resource.Ref {
	return resource.Ref{ID: i.ID, Type: resource.TypeIdentity, AdministrationID: i.AdministrationID}
}

// IdentityChanges holds the fields to set on an identity. Nil fields stay as
// they are.
type IdentityChanges struct {
	CompanyName       *string
	Country           *string
	City              *string
	Zipcode           *string
	Address1          *string
	Address2          *string
	Email             *string
	Phone             *string
	LegalEntityType   *string
	ChamberOfCommerce *string
	TaxNumber         *string
	BankAccountName   *string
	BankAccountNumber *string
	BankAccountBIC    *string

	TaxNumberAbsenceReason *string
	// CustomFields lists the custom field values to set; others stay as they
	// are.
	CustomFields []CustomFieldChange
}

func (c IdentityChanges) applyTo(i *Identity) {
	for _, field := range []struct {
		dst *string
		src *string
	}{
		{&i.CompanyName, c.CompanyName},
		{&i.Country, c.Country},
		{&i.City, c.City},
		{&i.Zipcode, c.Zipcode},
		{&i.Address1, c.Address1},
		{&i.Address2, c.Address2},
		{&i.Email, c.Email},
		{&i.Phone, c.Phone},
		{&i.LegalEntityType, c.LegalEntityType},
		{&i.ChamberOfCommerce, c.ChamberOfCommerce},
		{&i.TaxNumber, c.TaxNumber},
		{&i.BankAccountName, c.BankAccountName},
	} {
		if field.src != nil {
			*field.dst = strings.TrimSpace(*field.src)
		}
	}
	if c.BankAccountNumber != nil {
		i.BankAccountNumber = compactUpper(*c.BankAccountNumber)
	}
	if c.BankAccountBIC != nil {
		i.BankAccountBIC = compactUpper(*c.BankAccountBIC)
	}
	if c.TaxNumberAbsenceReason != nil {
		i.TaxNumberAbsenceReason = TaxNumberAbsenceReason(strings.TrimSpace(*c.TaxNumberAbsenceReason))
	}
	if i.TaxNumber != "" {
		i.TaxNumberAbsenceReason = ""
	}
}

func compactUpper(s string) string {
	return strings.ToUpper(strings.Join(strings.Fields(s), ""))
}

// identityProblems collects what is wrong with i's own fields.
func identityProblems(i Identity) *resource.ValidationError {
	verr := &resource.ValidationError{}
	if i.CompanyName == "" {
		verr.Add("company_name", "can't be blank")
	}
	if !slices.Contains(countries, i.Country) {
		verr.Add("country", "is not included in the list")
	}
	if i.Email != "" {
		if addr, err := mail.ParseAddress(i.Email); err != nil || addr.Address != i.Email {
			verr.Add("email", "is not a valid email address")
		}
	}
	if i.BankAccountNumber != "" && !iban.Valid(i.BankAccountNumber) {
		verr.Add("bank_account_number", "is not a valid IBAN")
	}
	if i.BankAccountBIC != "" && !bicPattern.MatchString(i.BankAccountBIC) {
		verr.Add("bank_account_bic", "is not a valid BIC")
	}
	if !slices.Contains([]TaxNumberAbsenceReason{"", BelowTaxThreshold, IndustryExemption}, i.TaxNumberAbsenceReason) {
		verr.Add("tax_number_absence_reason", "is not included in the list")
	}
	return verr
}

// Identities lists an administration's identities, the default one first.
func (s *Service) Identities(ctx context.Context, admin, user resource.ID) ([]Identity, error) {
	if _, err := s.store.ForUser(ctx, admin, user); err != nil {
		return nil, err
	}
	identities, err := s.store.ListIdentities(ctx, admin)
	if err != nil {
		return nil, fmt.Errorf("list identities: %w", err)
	}
	slices.SortStableFunc(identities, func(a, b Identity) int {
		switch {
		case a.Default && !b.Default:
			return -1
		case b.Default && !a.Default:
			return 1
		}
		return strings.Compare(strings.ToLower(a.CompanyName), strings.ToLower(b.CompanyName))
	})
	return identities, nil
}

// DefaultIdentity returns the identity holding the company's own details.
func (s *Service) DefaultIdentity(ctx context.Context, admin, user resource.ID) (Identity, error) {
	if _, err := s.store.ForUser(ctx, admin, user); err != nil {
		return Identity{}, err
	}
	return s.store.DefaultIdentity(ctx, admin)
}

// Identity returns one identity of an administration.
func (s *Service) Identity(ctx context.Context, admin, id, user resource.ID) (Identity, error) {
	if _, err := s.store.ForUser(ctx, admin, user); err != nil {
		return Identity{}, err
	}
	return s.store.Identity(ctx, admin, id)
}

// CreateIdentity adds a sender address to an administration.
func (s *Service) CreateIdentity(ctx context.Context, admin, user resource.ID, c IdentityChanges) (Identity, error) {
	if _, err := s.store.ForUser(ctx, admin, user); err != nil {
		return Identity{}, err
	}
	now := s.now()
	i := Identity{ID: s.ids.Next(), AdministrationID: admin, CreatedAt: now, UpdatedAt: now}
	if err := s.apply(ctx, &i, c); err != nil {
		return Identity{}, err
	}
	if err := s.store.CreateIdentity(ctx, i); err != nil {
		return Identity{}, fmt.Errorf("store identity: %w", err)
	}
	return i, nil
}

// UpdateIdentity changes one identity.
func (s *Service) UpdateIdentity(ctx context.Context, admin, id, user resource.ID, c IdentityChanges) (Identity, error) {
	i, err := s.Identity(ctx, admin, id, user)
	if err != nil {
		return Identity{}, err
	}
	return s.save(ctx, i, c)
}

// UpdateDefaultIdentity changes the company's own details.
func (s *Service) UpdateDefaultIdentity(ctx context.Context, admin, user resource.ID, c IdentityChanges) (Identity, error) {
	i, err := s.DefaultIdentity(ctx, admin, user)
	if err != nil {
		return Identity{}, err
	}
	return s.save(ctx, i, c)
}

func (s *Service) save(ctx context.Context, i Identity, c IdentityChanges) (Identity, error) {
	if err := s.apply(ctx, &i, c); err != nil {
		return Identity{}, err
	}
	i.UpdatedAt = s.now()
	if err := s.store.UpdateIdentity(ctx, i); err != nil {
		return Identity{}, fmt.Errorf("store identity: %w", err)
	}
	return i, nil
}

// apply sets c on i and validates the result.
func (s *Service) apply(ctx context.Context, i *Identity, c IdentityChanges) error {
	c.applyTo(i)
	verr := identityProblems(*i)
	if len(c.CustomFields) > 0 {
		values, ok, err := s.customFieldValues(ctx, i, c.CustomFields)
		if err != nil {
			return err
		}
		if !ok {
			verr.Add("custom_fields", "must belong to identities of this administration")
		}
		i.CustomFields = values
	}
	return verr.OrNil()
}

// customFieldValues merges changes into i's custom field values, keeping the
// order fields were created in. ok is false when a change names a field that
// is not an identity field of i's administration.
func (s *Service) customFieldValues(ctx context.Context, i *Identity, changes []CustomFieldChange) (values []CustomFieldValue, ok bool, err error) {
	fields, err := s.store.CustomFields(ctx, i.AdministrationID)
	if err != nil {
		return nil, false, fmt.Errorf("list custom fields: %w", err)
	}
	current := map[resource.ID]string{}
	for _, v := range i.CustomFields {
		current[v.ID] = v.Value
	}
	for _, c := range changes {
		if !slices.ContainsFunc(fields, func(f CustomField) bool { return f.ID == c.ID && f.Source == SourceIdentity }) {
			return nil, false, nil
		}
		current[c.ID] = strings.TrimSpace(c.Value)
	}
	for _, f := range fields {
		if v := current[f.ID]; v != "" {
			values = append(values, CustomFieldValue{ID: f.ID, Name: f.Name, Value: v})
		}
	}
	return values, true, nil
}

// DeleteIdentity removes a sender address. The default identity must remain.
func (s *Service) DeleteIdentity(ctx context.Context, admin, id, user resource.ID) error {
	i, err := s.Identity(ctx, admin, id, user)
	if err != nil {
		return err
	}
	if i.Default {
		return ErrDefaultIdentity
	}
	return s.store.DeleteIdentity(ctx, admin, id)
}
