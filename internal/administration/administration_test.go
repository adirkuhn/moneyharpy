package administration

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/adirkuhn/moneyharpy/internal/resource"
	"github.com/google/go-cmp/cmp"
)

// memStore is an in-memory Store used to exercise use cases without a database.
type memStore struct {
	admins     map[resource.ID]Administration
	access     map[resource.ID]map[resource.ID]Membership // administration -> user
	identities map[resource.ID]Identity
	fields     []CustomField
}

func newMemStore() *memStore {
	return &memStore{
		admins:     map[resource.ID]Administration{},
		access:     map[resource.ID]map[resource.ID]Membership{},
		identities: map[resource.ID]Identity{},
	}
}

func (s *memStore) Create(_ context.Context, a Administration, owner resource.ID, def Identity) error {
	s.admins[a.ID] = a
	s.identities[def.ID] = def
	s.access[a.ID] = map[resource.ID]Membership{owner: {Access: AccessUser}}
	return nil
}

func (s *memStore) ListForUser(_ context.Context, user resource.ID) ([]Listed, error) {
	var out []Listed
	for id, members := range s.access {
		if m, ok := members[user]; ok {
			out = append(out, Listed{Administration: s.admins[id], Membership: m})
		}
	}
	return out, nil
}

func (s *memStore) ForUser(_ context.Context, id, user resource.ID) (Listed, error) {
	m, ok := s.access[id][user]
	if !ok {
		return Listed{}, ErrNotFound
	}
	return Listed{Administration: s.admins[id], Membership: m}, nil
}

func (s *memStore) SetFavorite(_ context.Context, id, user resource.ID, favorite bool) error {
	m, ok := s.access[id][user]
	if !ok {
		return ErrNotFound
	}
	m.Favorite = favorite
	s.access[id][user] = m
	return nil
}

func (s *memStore) Update(_ context.Context, a Administration) error {
	s.admins[a.ID] = a
	return nil
}

func (s *memStore) ListIdentities(_ context.Context, admin resource.ID) ([]Identity, error) {
	var out []Identity
	for _, i := range s.identities {
		if i.AdministrationID == admin {
			out = append(out, i)
		}
	}
	return out, nil
}

func (s *memStore) DefaultIdentity(_ context.Context, admin resource.ID) (Identity, error) {
	for _, i := range s.identities {
		if i.AdministrationID == admin && i.Default {
			return i, nil
		}
	}
	return Identity{}, ErrIdentityNotFound
}

func (s *memStore) Identity(_ context.Context, admin, id resource.ID) (Identity, error) {
	i, ok := s.identities[id]
	if !ok || i.AdministrationID != admin {
		return Identity{}, ErrIdentityNotFound
	}
	return i, nil
}

func (s *memStore) CreateIdentity(_ context.Context, i Identity) error {
	s.identities[i.ID] = i
	return nil
}

func (s *memStore) UpdateIdentity(_ context.Context, i Identity) error {
	s.identities[i.ID] = i
	return nil
}

func (s *memStore) DeleteIdentity(_ context.Context, admin, id resource.ID) error {
	delete(s.identities, id)
	return nil
}

func (s *memStore) CustomFields(_ context.Context, admin resource.ID) ([]CustomField, error) {
	var out []CustomField
	for _, f := range s.fields {
		if f.AdministrationID == admin {
			out = append(out, f)
		}
	}
	return out, nil
}

func (s *memStore) CreateCustomField(_ context.Context, f CustomField) error {
	s.fields = append(s.fields, f)
	return nil
}

func (s *memStore) CustomField(_ context.Context, admin, id resource.ID) (CustomField, error) {
	for _, f := range s.fields {
		if f.AdministrationID == admin && f.ID == id {
			return f, nil
		}
	}
	return CustomField{}, ErrCustomFieldNotFound
}

func (s *memStore) UpdateCustomField(_ context.Context, f CustomField) error {
	for i := range s.fields {
		if s.fields[i].ID == f.ID {
			s.fields[i] = f
		}
	}
	return nil
}

func (s *memStore) DeleteCustomField(_ context.Context, admin, id resource.ID) error {
	s.fields = slices.DeleteFunc(s.fields, func(f CustomField) bool { return f.AdministrationID == admin && f.ID == id })
	return nil
}

func newService(s Store) *Service {
	return NewService(s, resource.NewGenerator(1), func() time.Time {
		return time.Date(2026, 3, 4, 5, 0, 0, 0, time.UTC)
	})
}

func TestCreateAppliesDefaults(t *testing.T) {
	const owner resource.ID = 1
	s := newService(newMemStore())

	got, err := s.Create(context.Background(), owner, NewAdministration{Name: "  Harpy B.V.  "})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	want := Listed{
		Administration: Administration{
			ID:              got.ID,
			Name:            "Harpy B.V.",
			Language:        "en",
			Currency:        "EUR",
			Country:         "NL",
			TimeZone:        "Europe/Amsterdam",
			PeriodStartDate: "2026-01-01",
		},
		Membership: Membership{Access: AccessUser},
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("Create() diff (-want +got):\n%s", diff)
	}
	if got.ID == 0 {
		t.Error("Create() ID = 0, want generated ID")
	}
}

func TestCreateRejectsInvalidInput(t *testing.T) {
	tests := []struct {
		name      string
		in        NewAdministration
		wantField string
	}{
		{name: "blank name", in: NewAdministration{Name: "   "}, wantField: "name"},
		{name: "unknown language", in: NewAdministration{Name: "A", Language: "de"}, wantField: "language"},
		{name: "lowercase currency", in: NewAdministration{Name: "A", Currency: "eur"}, wantField: "currency"},
		{name: "long country", in: NewAdministration{Name: "A", Country: "NLD"}, wantField: "country"},
		{name: "unassigned country", in: NewAdministration{Name: "A", Country: "ZZ"}, wantField: "country"},
		{name: "unknown currency", in: NewAdministration{Name: "A", Currency: "ABC"}, wantField: "currency"},
		{name: "unknown time zone", in: NewAdministration{Name: "A", TimeZone: "Mars/Olympus"}, wantField: "time_zone"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := newService(newMemStore()).Create(context.Background(), 1, tt.in)
			var verr *resource.ValidationError
			if !errors.As(err, &verr) {
				t.Fatalf("Create(%+v) error = %v, want *resource.ValidationError", tt.in, err)
			}
			if _, ok := verr.Fields[tt.wantField]; !ok {
				t.Errorf("Create(%+v) fields = %v, want key %q", tt.in, verr.Fields, tt.wantField)
			}
		})
	}
}

func TestListOrdersFavoritesFirstThenName(t *testing.T) {
	const user resource.ID = 1
	ctx := context.Background()
	svc := newService(newMemStore())

	var zeta Listed
	for _, name := range []string{"Beta", "Alpha", "Zeta"} {
		l, err := svc.Create(ctx, user, NewAdministration{Name: name})
		if err != nil {
			t.Fatalf("Create(%q) error = %v", name, err)
		}
		if name == "Zeta" {
			zeta = l
		}
	}
	if err := svc.SetFavorite(ctx, zeta.ID, user, true); err != nil {
		t.Fatalf("SetFavorite() error = %v", err)
	}

	listed, err := svc.List(ctx, user)
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	var got []string
	for _, l := range listed {
		got = append(got, l.Name)
	}
	if diff := cmp.Diff([]string{"Zeta", "Alpha", "Beta"}, got); diff != "" {
		t.Errorf("List() order diff (-want +got):\n%s", diff)
	}
}

func TestOtherUsersCannotReachAdministration(t *testing.T) {
	ctx := context.Background()
	svc := newService(newMemStore())
	a, err := svc.Create(ctx, 1, NewAdministration{Name: "Private"})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	if _, err := svc.Get(ctx, a.ID, 2); !errors.Is(err, ErrNotFound) {
		t.Errorf("Get() error = %v, want ErrNotFound", err)
	}
	if err := svc.SetFavorite(ctx, a.ID, 2, true); !errors.Is(err, ErrNotFound) {
		t.Errorf("SetFavorite() error = %v, want ErrNotFound", err)
	}
}

func TestCreateStartsDefaultIdentityFromCompanyDetails(t *testing.T) {
	svc := newService(newMemStore())
	ctx := context.Background()
	a, err := svc.Create(ctx, 1, NewAdministration{
		Name:              "Harpy",
		Country:           "BE",
		Address1:          " Dam 1 ",
		Address2:          "Floor 2",
		Zipcode:           "1012JS",
		City:              " Amsterdam",
		ChamberOfCommerce: "12345678",
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	got, err := svc.DefaultIdentity(ctx, a.ID, 1)
	if err != nil {
		t.Fatalf("DefaultIdentity() error = %v", err)
	}
	want := Identity{
		ID: got.ID, AdministrationID: a.ID, Default: true, CompanyName: "Harpy", Country: "BE",
		Address1: "Dam 1", Address2: "Floor 2", Zipcode: "1012JS", City: "Amsterdam", ChamberOfCommerce: "12345678",
		CreatedAt: got.CreatedAt, UpdatedAt: got.UpdatedAt,
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("DefaultIdentity() diff (-want +got):\n%s", diff)
	}
	if got.ID == a.ID {
		t.Error("identity reuses the administration ID")
	}
}

func ptr[T any](v T) *T { return &v }

func newAdministration(t *testing.T, svc *Service) Listed {
	t.Helper()
	a, err := svc.Create(context.Background(), 1, NewAdministration{Name: "Harpy"})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	return a
}

func TestCreateIdentityNormalizesAndKeepsItNonDefault(t *testing.T) {
	svc := newService(newMemStore())
	a := newAdministration(t, svc)

	got, err := svc.CreateIdentity(context.Background(), a.ID, 1, IdentityChanges{
		CompanyName: ptr(" Trade Name "), Country: ptr("NL"), Email: ptr(" a@example.com "),
		BankAccountNumber: ptr("nl91 abna 0417 1643 00"), BankAccountBIC: ptr("abnanl2a"),
	})
	if err != nil {
		t.Fatalf("CreateIdentity() error = %v", err)
	}
	if got.Default {
		t.Error("CreateIdentity() Default = true, want false")
	}
	if got.CompanyName != "Trade Name" || got.Email != "a@example.com" || got.BankAccountNumber != "NL91ABNA0417164300" || got.BankAccountBIC != "ABNANL2A" {
		t.Errorf("CreateIdentity() = %+v, want normalized values", got)
	}
	if got.Ref().URI() != "/"+a.ID.String()+"/identities/"+got.ID.String() {
		t.Errorf("URI() = %q", got.Ref().URI())
	}
}

func TestIdentityValidation(t *testing.T) {
	valid := func(change func(*IdentityChanges)) IdentityChanges {
		c := IdentityChanges{CompanyName: ptr("A"), Country: ptr("NL")}
		change(&c)
		return c
	}
	tests := []struct {
		name  string
		in    IdentityChanges
		field string
	}{
		{name: "no company name", in: valid(func(c *IdentityChanges) { c.CompanyName = ptr(" ") }), field: "company_name"},
		{name: "unknown country", in: valid(func(c *IdentityChanges) { c.Country = ptr("ZZ") }), field: "country"},
		{name: "bad email", in: valid(func(c *IdentityChanges) { c.Email = ptr("nope") }), field: "email"},
		{name: "iban with bad checksum", in: valid(func(c *IdentityChanges) { c.BankAccountNumber = ptr("NL91ABNA0417164301") }), field: "bank_account_number"},
		{name: "iban malformed", in: valid(func(c *IdentityChanges) { c.BankAccountNumber = ptr("1234") }), field: "bank_account_number"},
		{name: "bad bic", in: valid(func(c *IdentityChanges) { c.BankAccountBIC = ptr("ABN") }), field: "bank_account_bic"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := newService(newMemStore())
			a := newAdministration(t, svc)
			_, err := svc.CreateIdentity(context.Background(), a.ID, 1, tt.in)
			var verr *resource.ValidationError
			if !errors.As(err, &verr) {
				t.Fatalf("CreateIdentity(%+v) error = %v, want *resource.ValidationError", tt.in, err)
			}
			if _, ok := verr.Fields[tt.field]; !ok {
				t.Errorf("CreateIdentity() fields = %v, want key %q", verr.Fields, tt.field)
			}
		})
	}
}

func TestAcceptsValidBankDetails(t *testing.T) {
	for _, iban := range []string{"NL91ABNA0417164300", "GB82WEST12345698765432", ""} {
		svc := newService(newMemStore())
		a := newAdministration(t, svc)
		if _, err := svc.CreateIdentity(context.Background(), a.ID, 1, IdentityChanges{CompanyName: ptr("A"), Country: ptr("NL"), BankAccountNumber: ptr(iban)}); err != nil {
			t.Errorf("CreateIdentity(iban %q) error = %v", iban, err)
		}
	}
}

func TestUpdateDefaultIdentityChangesOnlyGivenFields(t *testing.T) {
	svc := newService(newMemStore())
	ctx := context.Background()
	a, err := svc.Create(ctx, 1, NewAdministration{Name: "Harpy", City: "Lochem"})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	got, err := svc.UpdateDefaultIdentity(ctx, a.ID, 1, IdentityChanges{Phone: ptr("0612345678"), Address2: ptr("")})
	if err != nil {
		t.Fatalf("UpdateDefaultIdentity() error = %v", err)
	}
	if got.City != "Lochem" || got.Phone != "0612345678" || !got.Default {
		t.Errorf("UpdateDefaultIdentity() = %+v", got)
	}
}

func TestDefaultIdentityCannotBeDeleted(t *testing.T) {
	svc := newService(newMemStore())
	ctx := context.Background()
	a := newAdministration(t, svc)
	def, err := svc.DefaultIdentity(ctx, a.ID, 1)
	if err != nil {
		t.Fatalf("DefaultIdentity() error = %v", err)
	}
	if err := svc.DeleteIdentity(ctx, a.ID, def.ID, 1); !errors.Is(err, ErrDefaultIdentity) {
		t.Errorf("DeleteIdentity() error = %v, want ErrDefaultIdentity", err)
	}

	extra, err := svc.CreateIdentity(ctx, a.ID, 1, IdentityChanges{CompanyName: ptr("B"), Country: ptr("NL")})
	if err != nil {
		t.Fatalf("CreateIdentity() error = %v", err)
	}
	if err := svc.DeleteIdentity(ctx, a.ID, extra.ID, 1); err != nil {
		t.Errorf("DeleteIdentity() error = %v", err)
	}
	if _, err := svc.Identity(ctx, a.ID, extra.ID, 1); !errors.Is(err, ErrIdentityNotFound) {
		t.Errorf("Identity() after delete error = %v, want ErrIdentityNotFound", err)
	}
}

func TestListsDefaultIdentityFirst(t *testing.T) {
	svc := newService(newMemStore())
	ctx := context.Background()
	a, err := svc.Create(ctx, 1, NewAdministration{Name: "Zed"})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if _, err := svc.CreateIdentity(ctx, a.ID, 1, IdentityChanges{CompanyName: ptr("Alpha"), Country: ptr("NL")}); err != nil {
		t.Fatalf("CreateIdentity() error = %v", err)
	}
	got, err := svc.Identities(ctx, a.ID, 1)
	if err != nil {
		t.Fatalf("Identities() error = %v", err)
	}
	if len(got) != 2 || got[0].CompanyName != "Zed" || !got[0].Default {
		t.Errorf("Identities() = %+v, want default first", got)
	}
}

func TestIdentitiesAreScopedToAdministrationAndMembers(t *testing.T) {
	svc := newService(newMemStore())
	ctx := context.Background()
	a, b := newAdministration(t, svc), newAdministration(t, svc)
	extra, err := svc.CreateIdentity(ctx, a.ID, 1, IdentityChanges{CompanyName: ptr("B"), Country: ptr("NL")})
	if err != nil {
		t.Fatalf("CreateIdentity() error = %v", err)
	}

	if _, err := svc.Identity(ctx, b.ID, extra.ID, 1); !errors.Is(err, ErrIdentityNotFound) {
		t.Errorf("Identity() through other administration error = %v, want ErrIdentityNotFound", err)
	}
	if err := svc.DeleteIdentity(ctx, b.ID, extra.ID, 1); !errors.Is(err, ErrIdentityNotFound) {
		t.Errorf("DeleteIdentity() through other administration error = %v, want ErrIdentityNotFound", err)
	}
	if _, err := svc.Identities(ctx, a.ID, 2); !errors.Is(err, ErrNotFound) {
		t.Errorf("Identities() by stranger error = %v, want ErrNotFound", err)
	}
	if _, err := svc.CreateIdentity(ctx, a.ID, 2, IdentityChanges{CompanyName: ptr("X"), Country: ptr("NL")}); !errors.Is(err, ErrNotFound) {
		t.Errorf("CreateIdentity() by stranger error = %v, want ErrNotFound", err)
	}
	if _, err := svc.Update(ctx, a.ID, 2, Changes{Name: ptr("X")}); !errors.Is(err, ErrNotFound) {
		t.Errorf("Update() by stranger error = %v, want ErrNotFound", err)
	}
}

func TestUpdateAdministration(t *testing.T) {
	svc := newService(newMemStore())
	ctx := context.Background()
	a := newAdministration(t, svc)

	got, err := svc.Update(ctx, a.ID, 1, Changes{Name: ptr(" New "), Language: ptr("nl"), Currency: ptr("USD"), TimeZone: ptr("Europe/Brussels")})
	if err != nil {
		t.Fatalf("Update() error = %v", err)
	}
	if got.Name != "New" || got.Language != "nl" || got.Currency != "USD" || got.TimeZone != "Europe/Brussels" {
		t.Errorf("Update() = %+v", got)
	}
	if _, err := svc.Update(ctx, a.ID, 1, Changes{Currency: ptr("ABC")}); err == nil {
		t.Error("Update() with unknown currency error = nil")
	}
}

func TestReferenceListsAreUsableChoices(t *testing.T) {
	for _, code := range []string{"NL", "BE", "XK"} {
		if !slices.Contains(Countries(), code) {
			t.Errorf("Countries() lacks %q", code)
		}
	}
	for _, code := range PopularCurrencies() {
		if !slices.Contains(Currencies(), code) {
			t.Errorf("Currencies() lacks popular currency %q", code)
		}
	}
	if !slices.IsSorted(Countries()) || !slices.IsSorted(Currencies()) {
		t.Error("reference lists are not sorted")
	}
}

func TestUpdateCompanyProfile(t *testing.T) {
	svc := newService(newMemStore())
	ctx := context.Background()
	a := newAdministration(t, svc)

	got, err := svc.Update(ctx, a.ID, 1, Changes{
		SmallBusinessScheme:          ptr(true),
		CompanyActivitiesDescription: ptr(" Bakery "),
		CompanyWebsiteURL:            ptr(" https://example.com "),
		CompanyWebsiteAbsent:         ptr(true),
	})
	if err != nil {
		t.Fatalf("Update() error = %v", err)
	}
	want := Profile{SmallBusinessScheme: true, CompanyActivitiesDescription: "Bakery", CompanyWebsiteURL: "https://example.com"}
	if diff := cmp.Diff(want, got.Profile); diff != "" {
		t.Errorf("Update() profile mismatch (-want +got):\n%s", diff)
	}

	got, err = svc.Update(ctx, a.ID, 1, Changes{CompanyWebsiteURL: ptr(""), CompanyWebsiteAbsent: ptr(true)})
	if err != nil {
		t.Fatalf("Update() error = %v", err)
	}
	if !got.CompanyWebsiteAbsent || got.CompanyWebsiteURL != "" {
		t.Errorf("Update() without website = %+v, want absent", got.Profile)
	}

	for _, url := range []string{"example", "ftp://example.com", "https://"} {
		_, err := svc.Update(ctx, a.ID, 1, Changes{CompanyWebsiteURL: ptr(url)})
		var verr *resource.ValidationError
		if !errors.As(err, &verr) || verr.Fields["company_website_url"] == nil {
			t.Errorf("Update(website %q) error = %v, want company_website_url error", url, err)
		}
	}
}

func TestTaxNumberAbsenceReason(t *testing.T) {
	svc := newService(newMemStore())
	ctx := context.Background()
	a := newAdministration(t, svc)

	got, err := svc.UpdateDefaultIdentity(ctx, a.ID, 1, IdentityChanges{TaxNumber: ptr(""), TaxNumberAbsenceReason: ptr("below_tax_threshold")})
	if err != nil {
		t.Fatalf("UpdateDefaultIdentity() error = %v", err)
	}
	if got.TaxNumberAbsenceReason != "below_tax_threshold" {
		t.Errorf("TaxNumberAbsenceReason = %q", got.TaxNumberAbsenceReason)
	}

	got, err = svc.UpdateDefaultIdentity(ctx, a.ID, 1, IdentityChanges{TaxNumber: ptr("NL000099998B57")})
	if err != nil {
		t.Fatalf("UpdateDefaultIdentity() error = %v", err)
	}
	if got.TaxNumberAbsenceReason != "" {
		t.Errorf("TaxNumberAbsenceReason with a VAT number = %q, want empty", got.TaxNumberAbsenceReason)
	}

	_, err = svc.UpdateDefaultIdentity(ctx, a.ID, 1, IdentityChanges{TaxNumber: ptr(""), TaxNumberAbsenceReason: ptr("lazy")})
	var verr *resource.ValidationError
	if !errors.As(err, &verr) || verr.Fields["tax_number_absence_reason"] == nil {
		t.Errorf("UpdateDefaultIdentity(unknown reason) error = %v, want tax_number_absence_reason error", err)
	}
}

func TestCustomFields(t *testing.T) {
	svc := newService(newMemStore())
	ctx := context.Background()
	a, b := newAdministration(t, svc), newAdministration(t, svc)

	f, err := svc.CreateCustomField(ctx, a.ID, 1, NewCustomField{Name: " Slogan ", Source: SourceIdentity})
	if err != nil {
		t.Fatalf("CreateCustomField() error = %v", err)
	}
	if f.Name != "Slogan" || f.Ref().URI() != "/"+a.ID.String()+"/custom_fields/"+f.ID.String() {
		t.Errorf("CreateCustomField() = %+v, URI %q", f, f.Ref().URI())
	}
	if _, err := svc.CreateCustomField(ctx, b.ID, 1, NewCustomField{Name: "Other", Source: SourceContact}); err != nil {
		t.Fatalf("CreateCustomField() error = %v", err)
	}

	got, err := svc.CustomFields(ctx, a.ID, 1)
	if err != nil {
		t.Fatalf("CustomFields() error = %v", err)
	}
	if len(got) != 1 || got[0].ID != f.ID {
		t.Errorf("CustomFields() = %+v, want only %v", got, f.ID)
	}

	for _, in := range []NewCustomField{{Name: " ", Source: SourceIdentity}, {Name: "X", Source: "nope"}} {
		_, err := svc.CreateCustomField(ctx, a.ID, 1, in)
		var verr *resource.ValidationError
		if !errors.As(err, &verr) {
			t.Errorf("CreateCustomField(%+v) error = %v, want validation error", in, err)
		}
	}
	if _, err := svc.CustomFields(ctx, a.ID, 2); !errors.Is(err, ErrNotFound) {
		t.Errorf("CustomFields() by stranger error = %v, want ErrNotFound", err)
	}
	if _, err := svc.CreateCustomField(ctx, a.ID, 2, NewCustomField{Name: "X", Source: SourceIdentity}); !errors.Is(err, ErrNotFound) {
		t.Errorf("CreateCustomField() by stranger error = %v, want ErrNotFound", err)
	}
}

func TestIdentityCustomFieldValues(t *testing.T) {
	svc := newService(newMemStore())
	ctx := context.Background()
	a := newAdministration(t, svc)
	slogan, err := svc.CreateCustomField(ctx, a.ID, 1, NewCustomField{Name: "Slogan", Source: SourceIdentity})
	if err != nil {
		t.Fatalf("CreateCustomField() error = %v", err)
	}
	motto, err := svc.CreateCustomField(ctx, a.ID, 1, NewCustomField{Name: "Motto", Source: SourceIdentity})
	if err != nil {
		t.Fatalf("CreateCustomField() error = %v", err)
	}
	contactField, err := svc.CreateCustomField(ctx, a.ID, 1, NewCustomField{Name: "Ref", Source: SourceContact})
	if err != nil {
		t.Fatalf("CreateCustomField() error = %v", err)
	}

	got, err := svc.UpdateDefaultIdentity(ctx, a.ID, 1, IdentityChanges{CustomFields: []CustomFieldChange{
		{ID: slogan.ID, Value: " Fresh "}, {ID: motto.ID, Value: "Fast"},
	}})
	if err != nil {
		t.Fatalf("UpdateDefaultIdentity() error = %v", err)
	}
	want := []CustomFieldValue{{ID: slogan.ID, Name: "Slogan", Value: "Fresh"}, {ID: motto.ID, Name: "Motto", Value: "Fast"}}
	if diff := cmp.Diff(want, got.CustomFields); diff != "" {
		t.Errorf("CustomFields mismatch (-want +got):\n%s", diff)
	}

	got, err = svc.UpdateDefaultIdentity(ctx, a.ID, 1, IdentityChanges{CustomFields: []CustomFieldChange{{ID: motto.ID, Value: ""}}})
	if err != nil {
		t.Fatalf("UpdateDefaultIdentity() error = %v", err)
	}
	want = []CustomFieldValue{{ID: slogan.ID, Name: "Slogan", Value: "Fresh"}}
	if diff := cmp.Diff(want, got.CustomFields); diff != "" {
		t.Errorf("CustomFields after clearing mismatch (-want +got):\n%s", diff)
	}

	for _, id := range []resource.ID{contactField.ID, 999} {
		_, err := svc.UpdateDefaultIdentity(ctx, a.ID, 1, IdentityChanges{CustomFields: []CustomFieldChange{{ID: id, Value: "x"}}})
		var verr *resource.ValidationError
		if !errors.As(err, &verr) || verr.Fields["custom_fields"] == nil {
			t.Errorf("UpdateDefaultIdentity(field %v) error = %v, want custom_fields error", id, err)
		}
	}
}

func TestCustomFieldEditingAndOrder(t *testing.T) {
	svc := newService(newMemStore())
	ctx := context.Background()
	a := newAdministration(t, svc)
	create := func(name string, source Source) CustomField {
		t.Helper()
		f, err := svc.CreateCustomField(ctx, a.ID, 1, NewCustomField{Name: name, Source: source})
		if err != nil {
			t.Fatal(err)
		}
		return f
	}
	first, second, contact := create("First", SourceIdentity), create("Second", SourceIdentity), create("Ref", SourceContact)
	if first.Position != 1 || second.Position != 2 || contact.Position != 1 {
		t.Errorf("positions = %d, %d, %d; want 1, 2 and 1 per source", first.Position, second.Position, contact.Position)
	}

	renamed, err := svc.UpdateCustomField(ctx, a.ID, first.ID, 1, CustomFieldChanges{Name: ptr(" Slogan ")})
	if err != nil || renamed.Name != "Slogan" {
		t.Errorf("UpdateCustomField() = %+v, %v", renamed, err)
	}
	if _, err := svc.UpdateCustomField(ctx, a.ID, first.ID, 1, CustomFieldChanges{Name: ptr(" ")}); err == nil {
		t.Error("UpdateCustomField(blank name) error = nil")
	}

	if err := svc.ReorderCustomFields(ctx, a.ID, 1, SourceIdentity, []resource.ID{second.ID, first.ID}); err != nil {
		t.Fatal(err)
	}
	list, _ := svc.CustomFields(ctx, a.ID, 1)
	var order []resource.ID
	for _, f := range list {
		if f.Source == SourceIdentity {
			order = append(order, f.ID)
		}
	}
	if !slices.Equal(order, []resource.ID{second.ID, first.ID}) {
		t.Errorf("identity fields order = %v, want %v", order, []resource.ID{second.ID, first.ID})
	}
	if err := svc.ReorderCustomFields(ctx, a.ID, 1, SourceIdentity, []resource.ID{second.ID}); err == nil {
		t.Error("ReorderCustomFields() leaving a field out error = nil")
	}
	if err := svc.ReorderCustomFields(ctx, a.ID, 1, SourceIdentity, []resource.ID{second.ID, contact.ID}); err == nil {
		t.Error("ReorderCustomFields() with a field of another source error = nil")
	}

	if err := svc.DeleteCustomField(ctx, a.ID, contact.ID, 1); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.CustomField(ctx, a.ID, contact.ID, 1); !errors.Is(err, ErrCustomFieldNotFound) {
		t.Errorf("CustomField() after delete error = %v", err)
	}
	if err := svc.DeleteCustomField(ctx, a.ID, first.ID, 2); !errors.Is(err, ErrNotFound) {
		t.Errorf("DeleteCustomField() by stranger error = %v", err)
	}
}
