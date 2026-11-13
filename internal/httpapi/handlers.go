package httpapi

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"path"
	"slices"
	"strings"
	"time"

	"github.com/adirkuhn/moneyharpy/internal/administration"
	"github.com/adirkuhn/moneyharpy/internal/identity"
	"github.com/adirkuhn/moneyharpy/internal/resource"
	"github.com/adirkuhn/moneyharpy/internal/settings"
)

const (
	sessionMaxAge    = 30 * 24 * time.Hour
	resetSendTimeout = time.Minute
	oauthCookieAge   = 10 * time.Minute
	oauthCookie      = "moneyharpy_oauth"
	oauthCookiePath  = "/api/auth/google"
	lm2Cookie        = "moneyharpy_lm2"
	lm2CookiePath    = "/api/auth/lm2"
	afterLoginPath   = "/administrations"
)

func decodeBody[T any](w http.ResponseWriter, r *http.Request) (T, bool) {
	var v T
	if err := json.NewDecoder(r.Body).Decode(&v); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return v, false
	}
	return v, true
}

func user(r *http.Request) identity.User {
	u, ok := userFrom(r.Context())
	if !ok {
		// Unreachable: the contract rejects unauthenticated calls first.
		panic("handler reached without an authenticated user")
	}
	return u
}

func toAdministration(l administration.Listed) Administration {
	return Administration{
		Id:                l.ID.String(),
		Name:              l.Name,
		Language:          Language(l.Language),
		Currency:          l.Currency,
		Country:           l.Country,
		TimeZone:          l.TimeZone,
		Access:            AdministrationAccess(l.Access),
		Suspended:         l.Suspended,
		PeriodLockedUntil: l.PeriodLockedUntil,
		PeriodStartDate:   l.PeriodStartDate,
		Favorite:          l.Favorite,
		Uri:               l.Ref().URI(),

		SmallBusinessScheme:          l.SmallBusinessScheme,
		CompanyActivitiesDescription: l.CompanyActivitiesDescription,
		CompanyWebsiteUrl:            l.CompanyWebsiteURL,
		CompanyWebsiteAbsent:         l.CompanyWebsiteAbsent,
		UserType:                     AdministrationUserType(l.UserType),
		Permissions:                  permissionsJSON(l.Permissions),
		RequireTwoFactor:             l.RequireTwoFactor,
		SupportAccessUntil:           l.SupportAccessUntil,
	}
}

func permissionsJSON(ps []administration.Permission) []Permission {
	out := make([]Permission, len(ps))
	for i, p := range ps {
		out[i] = Permission(p)
	}
	return out
}

// ListAdministrations implements ServerInterface.
func (s *server) ListAdministrations(w http.ResponseWriter, r *http.Request) {
	listed, err := s.deps.Administrations.List(r.Context(), user(r).ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if g, ok := grantFrom(r.Context()); ok {
		// A connection sees only its own administration.
		listed = slices.DeleteFunc(listed, func(l administration.Listed) bool { return l.ID != g.AdministrationID })
	}
	out := make([]Administration, len(listed))
	for i, l := range listed {
		out[i] = toAdministration(l)
	}
	writeJSON(w, http.StatusOK, out)
}

// CreateAdministration implements ServerInterface.
func (s *server) CreateAdministration(w http.ResponseWriter, r *http.Request) {
	if _, ok := grantFrom(r.Context()); ok {
		writeError(w, http.StatusForbidden, "Connection tokens cannot create administrations")
		return
	}
	body, ok := decodeBody[CreateAdministrationRequest](w, r)
	if !ok {
		return
	}
	in := body.Administration
	created, err := s.deps.Administrations.Create(r.Context(), user(r).ID, administration.NewAdministration{
		Name:     in.Name,
		Language: string(deref(in.Language)),
		Currency: deref(in.Currency),
		Country:  deref(in.Country),
		TimeZone: deref(in.TimeZone),

		Address1:          deref(in.Address1),
		Address2:          deref(in.Address2),
		Zipcode:           deref(in.Zipcode),
		City:              deref(in.City),
		ChamberOfCommerce: deref(in.ChamberOfCommerce),
	})
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, toAdministration(created))
}

// GetAdministration implements ServerInterface.
func (s *server) GetAdministration(w http.ResponseWriter, r *http.Request, id Id) {
	rid, ok := parseID(w, id)
	if !ok {
		return
	}
	got, err := s.deps.Administrations.Get(r.Context(), rid, user(r).ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, toAdministration(got))
}

// UpdateAdministration implements ServerInterface.
func (s *server) UpdateAdministration(w http.ResponseWriter, r *http.Request, id Id) {
	rid, ok := parseID(w, id)
	if !ok {
		return
	}
	body, ok := decodeBody[UpdateAdministrationRequest](w, r)
	if !ok {
		return
	}
	in := body.Administration
	updated, err := s.deps.Administrations.Update(r.Context(), rid, user(r).ID, administration.Changes{
		Name: in.Name, Country: in.Country, Language: (*string)(in.Language), Currency: in.Currency, TimeZone: in.TimeZone,
		SmallBusinessScheme: in.SmallBusinessScheme, CompanyActivitiesDescription: in.CompanyActivitiesDescription,
		CompanyWebsiteURL: in.CompanyWebsiteUrl, CompanyWebsiteAbsent: in.CompanyWebsiteAbsent,
	})
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, toAdministration(updated))
}

// nullable maps an empty value to JSON null, as Moneybird does.
func nullable[S ~string](s S) *S {
	if s == "" {
		return nil
	}
	return &s
}

func toIdentity(i administration.Identity) Identity {
	return Identity{
		Id:                i.ID.String(),
		AdministrationId:  i.AdministrationID.String(),
		CompanyName:       nullable(i.CompanyName),
		Country:           nullable(i.Country),
		City:              nullable(i.City),
		Zipcode:           nullable(i.Zipcode),
		Address1:          nullable(i.Address1),
		Address2:          nullable(i.Address2),
		Email:             nullable(i.Email),
		Phone:             nullable(i.Phone),
		BankAccountName:   nullable(i.BankAccountName),
		BankAccountNumber: nullable(i.BankAccountNumber),
		BankAccountBic:    nullable(i.BankAccountBIC),
		ChamberOfCommerce: nullable(i.ChamberOfCommerce),
		TaxNumber:         nullable(i.TaxNumber),
		LegalEntityType:   nullable(i.LegalEntityType),
		CustomFields:      toCustomFieldValues(i.CustomFields),
		CreatedAt:         i.CreatedAt,
		UpdatedAt:         i.UpdatedAt,
		Default:           i.Default,
		Uri:               i.Ref().URI(),

		TaxNumberAbsenceReason: nullable(TaxNumberAbsenceReason(i.TaxNumberAbsenceReason)),
	}
}

func toCustomFieldValues(values []administration.CustomFieldValue) []CustomFieldValue {
	out := make([]CustomFieldValue, len(values))
	for i, v := range values {
		out[i] = CustomFieldValue{Id: v.ID.String(), Name: v.Name, Value: v.Value}
	}
	return out
}

func toCustomField(f administration.CustomField) CustomField {
	return CustomField{
		Id: f.ID.String(), AdministrationId: f.AdministrationID.String(), Name: f.Name,
		Source: CustomFieldSource(f.Source), Uri: f.Ref().URI(),
	}
}

func toIdentityChanges(body IdentityRequest) administration.IdentityChanges {
	in := body.Identity
	return administration.IdentityChanges{
		CompanyName:       in.CompanyName,
		Country:           in.Country,
		City:              in.City,
		Zipcode:           in.Zipcode,
		Address1:          in.Address1,
		Address2:          in.Address2,
		Email:             in.Email,
		Phone:             in.Phone,
		LegalEntityType:   in.LegalEntityType,
		ChamberOfCommerce: in.ChamberOfCommerce,
		TaxNumber:         in.TaxNumber,
		BankAccountName:   in.BankAccountName,
		BankAccountNumber: in.BankAccountNumber,
		BankAccountBIC:    in.BankAccountBic,

		TaxNumberAbsenceReason: (*string)(in.TaxNumberAbsenceReason),
		CustomFields:           toCustomFieldChanges(in.CustomFieldsAttributes),
	}
}

// toDefaultIdentityChanges converts a default identity update. Moneybird's
// administration_attributes set the administration's legal entity type, which
// MoneyHarpy keeps on the default identity.
func toDefaultIdentityChanges(body DefaultIdentityRequest) (administration.IdentityChanges, error) {
	raw, err := json.Marshal(body)
	if err != nil {
		return administration.IdentityChanges{}, fmt.Errorf("encode identity: %w", err)
	}
	var plain IdentityRequest // drops administration_attributes
	if err := json.Unmarshal(raw, &plain); err != nil {
		return administration.IdentityChanges{}, fmt.Errorf("decode identity: %w", err)
	}
	changes := toIdentityChanges(plain)
	if a := body.Identity.AdministrationAttributes; a != nil && a.LegalEntityType != nil {
		changes.LegalEntityType = (*string)(a.LegalEntityType)
	}
	return changes, nil
}

// toCustomFieldChanges converts requested values. The contract guarantees
// numeric IDs; one that overflows becomes 0, which matches no field and so
// fails validation.
func toCustomFieldChanges(in *[]struct {
	Id    Identifier `json:"id"`
	Value string     `json:"value"`
}) []administration.CustomFieldChange {
	if in == nil {
		return nil
	}
	out := make([]administration.CustomFieldChange, len(*in))
	for i, c := range *in {
		id, _ := resource.ParseID(c.Id)
		out[i] = administration.CustomFieldChange{ID: id, Value: c.Value}
	}
	return out
}

// ListCustomFields implements ServerInterface.
func (s *server) ListCustomFields(w http.ResponseWriter, r *http.Request, administrationID AdministrationId) {
	admin, ok := parseID(w, administrationID)
	if !ok {
		return
	}
	fields, err := s.deps.Administrations.CustomFields(r.Context(), admin, user(r).ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	out := make([]CustomField, len(fields))
	for i, f := range fields {
		out[i] = toCustomField(f)
	}
	writeJSON(w, http.StatusOK, out)
}

// CreateCustomField implements ServerInterface.
func (s *server) CreateCustomField(w http.ResponseWriter, r *http.Request, administrationID AdministrationId) {
	admin, ok := parseID(w, administrationID)
	if !ok {
		return
	}
	body, ok := decodeBody[CustomFieldRequest](w, r)
	if !ok {
		return
	}
	created, err := s.deps.Administrations.CreateCustomField(r.Context(), admin, user(r).ID, administration.NewCustomField{
		Name: body.CustomField.Name, Source: administration.Source(body.CustomField.Source),
	})
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, toCustomField(created))
}

// ListIdentities implements ServerInterface.
func (s *server) ListIdentities(w http.ResponseWriter, r *http.Request, administrationID AdministrationId) {
	admin, ok := parseID(w, administrationID)
	if !ok {
		return
	}
	identities, err := s.deps.Administrations.Identities(r.Context(), admin, user(r).ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	out := make([]Identity, len(identities))
	for i, identity := range identities {
		out[i] = toIdentity(identity)
	}
	writeJSON(w, http.StatusOK, out)
}

// CreateIdentity implements ServerInterface.
func (s *server) CreateIdentity(w http.ResponseWriter, r *http.Request, administrationID AdministrationId) {
	admin, ok := parseID(w, administrationID)
	if !ok {
		return
	}
	body, ok := decodeBody[IdentityRequest](w, r)
	if !ok {
		return
	}
	created, err := s.deps.Administrations.CreateIdentity(r.Context(), admin, user(r).ID, toIdentityChanges(body))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, toIdentity(created))
}

// GetDefaultIdentity implements ServerInterface.
func (s *server) GetDefaultIdentity(w http.ResponseWriter, r *http.Request, administrationID AdministrationId) {
	admin, ok := parseID(w, administrationID)
	if !ok {
		return
	}
	got, err := s.deps.Administrations.DefaultIdentity(r.Context(), admin, user(r).ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, toIdentity(got))
}

// UpdateDefaultIdentity implements ServerInterface.
func (s *server) UpdateDefaultIdentity(w http.ResponseWriter, r *http.Request, administrationID AdministrationId) {
	admin, ok := parseID(w, administrationID)
	if !ok {
		return
	}
	body, ok := decodeBody[DefaultIdentityRequest](w, r)
	if !ok {
		return
	}
	changes, err := toDefaultIdentityChanges(body)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	updated, err := s.deps.Administrations.UpdateDefaultIdentity(r.Context(), admin, user(r).ID, changes)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, toIdentity(updated))
}

// GetIdentity implements ServerInterface.
func (s *server) GetIdentity(w http.ResponseWriter, r *http.Request, administrationID AdministrationId, id Id) {
	admin, rid, ok := parseIDs(w, administrationID, id)
	if !ok {
		return
	}
	got, err := s.deps.Administrations.Identity(r.Context(), admin, rid, user(r).ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, toIdentity(got))
}

// UpdateIdentity implements ServerInterface.
func (s *server) UpdateIdentity(w http.ResponseWriter, r *http.Request, administrationID AdministrationId, id Id) {
	admin, rid, ok := parseIDs(w, administrationID, id)
	if !ok {
		return
	}
	body, ok := decodeBody[IdentityRequest](w, r)
	if !ok {
		return
	}
	updated, err := s.deps.Administrations.UpdateIdentity(r.Context(), admin, rid, user(r).ID, toIdentityChanges(body))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, toIdentity(updated))
}

// DeleteIdentity implements ServerInterface.
func (s *server) DeleteIdentity(w http.ResponseWriter, r *http.Request, administrationID AdministrationId, id Id) {
	admin, rid, ok := parseIDs(w, administrationID, id)
	if !ok {
		return
	}
	if err := s.deps.Administrations.DeleteIdentity(r.Context(), admin, rid, user(r).ID); err != nil {
		s.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func parseIDs(w http.ResponseWriter, administrationID AdministrationId, id Id) (resource.ID, resource.ID, bool) {
	admin, ok := parseID(w, administrationID)
	if !ok {
		return 0, 0, false
	}
	rid, ok := parseID(w, id)
	return admin, rid, ok
}

// lookupCountry is the only country with a register and address service wired
// in so far.
const lookupCountry = "NL"

// OnboardingOptions implements ServerInterface.
func (s *server) OnboardingOptions(w http.ResponseWriter, r *http.Request) {
	inst, err := s.deps.Settings.Instance(r.Context())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	out := OnboardingOptions{
		Countries:              administration.Countries(),
		Currencies:             administration.Currencies(),
		PopularCurrencies:      administration.PopularCurrencies(),
		CompanySearchCountries: []string{},
		AddressLookupCountries: []string{},
	}
	if inst.KVK.Enabled || inst.KVK.WebsiteSearch {
		out.CompanySearchCountries = []string{lookupCountry}
	}
	if inst.AddressLookupEnabled {
		out.AddressLookupCountries = []string{lookupCountry}
	}
	writeJSON(w, http.StatusOK, out)
}

// CompanySearch implements ServerInterface.
func (s *server) CompanySearch(w http.ResponseWriter, r *http.Request, params CompanySearchParams) {
	if params.Country != lookupCountry {
		s.fail(w, r, administration.ErrLookupUnavailable)
		return
	}
	found, err := s.deps.Companies.SearchCompanies(r.Context(), params.Query)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	out := make([]CompanyMatch, len(found))
	for i, c := range found {
		out[i] = CompanyMatch{Name: c.Name, ChamberOfCommerce: c.ChamberOfCommerce, Address1: c.Address1, Zipcode: c.Zipcode, City: c.City}
	}
	writeJSON(w, http.StatusOK, out)
}

// AddressLookup implements ServerInterface.
func (s *server) AddressLookup(w http.ResponseWriter, r *http.Request, params AddressLookupParams) {
	found, err := s.deps.Addresses.LookupAddress(r.Context(), params.Zipcode, params.HouseNumber)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, AddressMatch{Address1: found.Address1, Zipcode: found.Zipcode, City: found.City})
}

// FavoriteAdministration implements ServerInterface.
func (s *server) FavoriteAdministration(w http.ResponseWriter, r *http.Request, id Id) {
	s.setFavorite(w, r, id, true)
}

// UnfavoriteAdministration implements ServerInterface.
func (s *server) UnfavoriteAdministration(w http.ResponseWriter, r *http.Request, id Id) {
	s.setFavorite(w, r, id, false)
}

func (s *server) setFavorite(w http.ResponseWriter, r *http.Request, id Id, favorite bool) {
	rid, ok := parseID(w, id)
	if !ok {
		return
	}
	if err := s.deps.Administrations.SetFavorite(r.Context(), rid, user(r).ID, favorite); err != nil {
		s.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// LocateResource implements ServerInterface.
func (s *server) LocateResource(w http.ResponseWriter, r *http.Request, id Id) {
	rid, ok := parseID(w, id)
	if !ok {
		return
	}
	ref, err := s.deps.Resources.Locate(r.Context(), rid, user(r).ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, ResourceLocation{Id: ref.ID.String(), Type: string(ref.Type), Uri: ref.URI()})
}

// AuthProviders implements ServerInterface.
func (s *server) AuthProviders(w http.ResponseWriter, r *http.Request) {
	inst, err := s.deps.Settings.Instance(r.Context())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, AuthProviders{RegistrationOpen: inst.RegistrationOpen, Google: inst.Google.Enabled, Lm2: inst.LM2.Enabled, PasswordReset: inst.SMTP.Enabled})
}

// Register implements ServerInterface.
func (s *server) Register(w http.ResponseWriter, r *http.Request) {
	body, ok := decodeBody[RegisterRequest](w, r)
	if !ok {
		return
	}
	in := identity.Registration{Email: body.Email, Name: body.Name, Password: body.Password}
	if body.InvitationToken != nil {
		// An open invitation to this very address lets it register while
		// registration is closed.
		if v, err := s.deps.Membership.InvitationByToken(r.Context(), *body.InvitationToken); err == nil {
			in.Invited = v.Open(time.Now()) && strings.EqualFold(v.Email, strings.TrimSpace(body.Email))
		}
	}
	res, err := s.deps.Identity.Register(r.Context(), in)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.signedIn(w, r, http.StatusCreated, res)
}

// Login implements ServerInterface.
func (s *server) Login(w http.ResponseWriter, r *http.Request) {
	body, ok := decodeBody[LoginRequest](w, r)
	if !ok {
		return
	}
	res, err := s.deps.Identity.Login(r.Context(), body.Email, body.Password)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.signedIn(w, r, http.StatusOK, res)
}

// RequestPasswordReset implements ServerInterface. The work happens after the
// response, so an address with an account takes as long as one without.
func (s *server) RequestPasswordReset(w http.ResponseWriter, r *http.Request) {
	inst, err := s.deps.Settings.Instance(r.Context())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if !inst.SMTP.Enabled {
		writeError(w, http.StatusNotFound, "Not found")
		return
	}
	body, ok := decodeBody[PasswordResetRequest](w, r)
	if !ok {
		return
	}
	if s.deps.ResetLimiter.Allow(strings.ToLower(strings.TrimSpace(body.Email))).Allowed {
		ctx, cancel := context.WithTimeout(context.WithoutCancel(r.Context()), resetSendTimeout)
		go func() {
			defer cancel()
			if err := s.deps.Identity.RequestPasswordReset(ctx, body.Email, string(deref(body.Language))); err != nil {
				s.deps.Logger.ErrorContext(ctx, "password reset failed", "error", err)
			}
		}()
	}
	w.WriteHeader(http.StatusNoContent)
}

// ConfirmPasswordReset implements ServerInterface.
func (s *server) ConfirmPasswordReset(w http.ResponseWriter, r *http.Request) {
	body, ok := decodeBody[PasswordResetConfirmation](w, r)
	if !ok {
		return
	}
	if err := s.deps.Identity.ResetPassword(r.Context(), body.Token, body.Password); err != nil {
		s.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// Logout implements ServerInterface.
func (s *server) Logout(w http.ResponseWriter, r *http.Request) {
	if token, ok := r.Context().Value(sessionKey).(string); ok {
		if err := s.deps.Identity.Logout(r.Context(), token); err != nil {
			s.fail(w, r, err)
			return
		}
	}
	s.setSession(w, "", -time.Second)
	w.WriteHeader(http.StatusNoContent)
}

// CurrentUser implements ServerInterface.
func (s *server) CurrentUser(w http.ResponseWriter, r *http.Request) {
	s.currentUser(w, r, http.StatusOK, user(r))
}

// GoogleStart implements ServerInterface.
func (s *server) GoogleStart(w http.ResponseWriter, r *http.Request) {
	inst, err := s.deps.Settings.Instance(r.Context())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if !inst.Google.Enabled {
		writeError(w, http.StatusNotFound, "Not found")
		return
	}
	state, nonce := rand.Text(), rand.Text()
	target, err := s.deps.Google.AuthURL(r.Context(), state, nonce)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.setChallengeCookie(w, oauthCookie, oauthCookiePath, state+"."+nonce, oauthCookieAge)
	http.Redirect(w, r, target, http.StatusFound)
}

// GoogleCallback implements ServerInterface.
func (s *server) GoogleCallback(w http.ResponseWriter, r *http.Request, params GoogleCallbackParams) {
	s.setChallengeCookie(w, oauthCookie, oauthCookiePath, "", -time.Second)

	state, nonce, ok := s.oauthChallenge(r)
	if !ok || params.Error != nil || params.Code == nil || params.State == nil || !sameSecret(*params.State, state) {
		http.Redirect(w, r, "/login?error=google", http.StatusFound)
		return
	}

	external, err := s.deps.Google.Exchange(r.Context(), *params.Code, nonce)
	if err != nil {
		s.deps.Logger.WarnContext(r.Context(), "google exchange failed", "error", err)
		http.Redirect(w, r, "/login?error=google", http.StatusFound)
		return
	}
	s.finishExternalLogin(w, r, external, "google")
}

// Lm2Start implements ServerInterface.
func (s *server) Lm2Start(w http.ResponseWriter, r *http.Request) {
	inst, err := s.deps.Settings.Instance(r.Context())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if !inst.LM2.Enabled {
		writeError(w, http.StatusNotFound, "Not found")
		return
	}
	state := rand.Text()
	target, err := s.deps.LM2.LoginURL(r.Context(), state)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.setChallengeCookie(w, lm2Cookie, lm2CookiePath, state, oauthCookieAge)
	http.Redirect(w, r, target, http.StatusFound)
}

// Lm2Callback implements ServerInterface. The token arrives in the address
// because that is how LM2 hands it over, so it must never be logged.
func (s *server) Lm2Callback(w http.ResponseWriter, r *http.Request, state string, params Lm2CallbackParams) {
	s.setChallengeCookie(w, lm2Cookie, lm2CookiePath, "", -time.Second)

	c, err := r.Cookie(lm2Cookie)
	if err != nil || params.Token == nil || !sameSecret(state, c.Value) {
		http.Redirect(w, r, "/login?error=lm2", http.StatusFound)
		return
	}
	external, err := s.deps.LM2.Identify(r.Context(), *params.Token)
	if err != nil {
		s.deps.Logger.WarnContext(r.Context(), "lm2 identify failed", "error", err)
		http.Redirect(w, r, "/login?error=lm2", http.StatusFound)
		return
	}
	s.finishExternalLogin(w, r, external, "lm2")
}

func (s *server) finishExternalLogin(w http.ResponseWriter, r *http.Request, external identity.External, provider string) {
	res, err := s.deps.Identity.LoginExternal(r.Context(), external)
	switch {
	case errors.Is(err, identity.ErrRegistrationClosed):
		http.Redirect(w, r, "/login?error=registration_closed", http.StatusFound)
		return
	case errors.Is(err, identity.ErrEmailTaken):
		http.Redirect(w, r, "/login?error=email_taken", http.StatusFound)
		return
	case err != nil:
		s.deps.Logger.ErrorContext(r.Context(), "external login failed", "provider", provider, "error", err)
		http.Redirect(w, r, "/login?error="+provider, http.StatusFound)
		return
	}
	if res.Challenge != "" {
		s.setChallengeCookie(w, signInCookie, signInCookiePath, res.Challenge, signInCookieAge)
		http.Redirect(w, r, "/login/two-factor", http.StatusFound)
		return
	}
	s.setSession(w, res.SessionToken, sessionMaxAge)
	http.Redirect(w, r, afterLoginPath, http.StatusFound)
}

func sameSecret(a, b string) bool {
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}

func (s *server) oauthChallenge(r *http.Request) (state, nonce string, ok bool) {
	c, err := r.Cookie(oauthCookie)
	if err != nil {
		return "", "", false
	}
	return strings.Cut(c.Value, ".")
}

// setChallengeCookie stores a sign-in challenge. SameSite=Lax keeps it on the
// top-level redirect back from the provider.
func (s *server) setChallengeCookie(w http.ResponseWriter, name, cookiePath, value string, maxAge time.Duration) {
	http.SetCookie(w, &http.Cookie{
		Name:     name,
		Value:    value,
		Path:     cookiePath,
		MaxAge:   int(maxAge.Seconds()),
		HttpOnly: true,
		Secure:   s.secureCookies,
		SameSite: http.SameSiteLaxMode,
	})
}

func toAPIToken(t identity.Token) ApiToken {
	return ApiToken{Id: t.ID.String(), Name: t.Name, CreatedAt: t.CreatedAt}
}

// ListTokens implements ServerInterface.
func (s *server) ListTokens(w http.ResponseWriter, r *http.Request) {
	tokens, err := s.deps.Identity.Tokens(r.Context(), user(r).ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	out := make([]ApiToken, len(tokens))
	for i, t := range tokens {
		out[i] = toAPIToken(t)
	}
	writeJSON(w, http.StatusOK, out)
}

// CreateToken implements ServerInterface.
func (s *server) CreateToken(w http.ResponseWriter, r *http.Request) {
	body, ok := decodeBody[CreateTokenRequest](w, r)
	if !ok {
		return
	}
	t, secret, err := s.deps.Identity.CreateToken(r.Context(), user(r).ID, body.Name)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, createdToken{ApiToken: toAPIToken(t), Token: secret})
}

// createdToken is the wire form of CreatedApiToken.
type createdToken struct {
	ApiToken
	Token string `json:"token"`
}

// DeleteToken implements ServerInterface.
func (s *server) DeleteToken(w http.ResponseWriter, r *http.Request, id Id) {
	rid, ok := parseID(w, id)
	if !ok {
		return
	}
	if err := s.deps.Identity.DeleteToken(r.Context(), rid, user(r).ID); err != nil {
		s.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func toInstanceSettings(inst settings.Instance) InstanceSettings {
	return InstanceSettings{
		RegistrationOpen:         inst.RegistrationOpen,
		GoogleEnabled:            inst.Google.Enabled,
		GoogleClientId:           inst.Google.ClientID,
		GoogleClientSecretSet:    inst.Google.ClientSecret != "",
		Lm2Enabled:               inst.LM2.Enabled,
		Lm2Url:                   inst.LM2.URL,
		Lm2App:                   inst.LM2.App,
		KvkEnabled:               inst.KVK.Enabled,
		KvkApiKeySet:             inst.KVK.APIKey != "",
		KvkWebsiteSearchEnabled:  inst.KVK.WebsiteSearch,
		AddressLookupEnabled:     inst.AddressLookupEnabled,
		WebhooksAllowLocal:       inst.WebhooksAllowLocal,
		SmtpEnabled:              inst.SMTP.Enabled,
		SmtpHost:                 inst.SMTP.Host,
		SmtpPort:                 inst.SMTP.Port,
		SmtpUsername:             inst.SMTP.Username,
		SmtpPasswordSet:          inst.SMTP.Password != "",
		SmtpFrom:                 inst.SMTP.From,
		SmtpSecurity:             inst.SMTP.Security,
		StorageEnabled:           inst.Storage.Enabled,
		StorageEndpoint:          inst.Storage.Endpoint,
		StorageRegion:            inst.Storage.Region,
		StorageBucket:            inst.Storage.Bucket,
		StorageAccessKey:         inst.Storage.AccessKey,
		StorageSecretKeySet:      inst.Storage.SecretKey != "",
		StoragePathStyle:         inst.Storage.PathStyle,
		RecognitionAiEnabled:     inst.Recognition.AIEnabled,
		RecognitionAiConnections: toAIConnections(inst.Recognition.Connections),
	}
}

func toAIConnections(cs []settings.AIConnection) []RecognitionAIConnection {
	out := make([]RecognitionAIConnection, len(cs))
	for i, c := range cs {
		out[i] = RecognitionAIConnection{
			Id: c.ID, Name: c.Name, Provider: RecognitionAIProvider(c.Provider), Url: c.URL, Model: c.Model,
			ApiKeySet: c.APIKey != "", Enabled: c.Enabled,
		}
	}
	return out
}

func fromAIConnections(in *[]RecognitionAIConnectionRequest) *[]settings.AIConnectionUpdate {
	if in == nil {
		return nil
	}
	out := make([]settings.AIConnectionUpdate, len(*in))
	for i, c := range *in {
		out[i] = settings.AIConnectionUpdate{
			ID: deref(c.Id), Name: deref(c.Name), Provider: string(c.Provider), URL: deref(c.Url), Model: deref(c.Model),
			APIKey: c.ApiKey, Enabled: c.Enabled == nil || *c.Enabled,
		}
	}
	return &out
}

func requireInstanceAdmin(w http.ResponseWriter, r *http.Request) bool {
	if !user(r).InstanceAdmin {
		writeError(w, http.StatusForbidden, "Instance administrators only")
		return false
	}
	return true
}

// InstanceSettings implements ServerInterface.
func (s *server) InstanceSettings(w http.ResponseWriter, r *http.Request) {
	if !requireInstanceAdmin(w, r) {
		return
	}
	inst, err := s.deps.Settings.Instance(r.Context())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, toInstanceSettings(inst))
}

// SendTestEmail implements ServerInterface. The mail server's reason is shown
// because only instance administrators can reach this and need it to fix setup.
func (s *server) SendTestEmail(w http.ResponseWriter, r *http.Request) {
	if !requireInstanceAdmin(w, r) {
		return
	}
	if err := s.deps.Mail.SendTest(r.Context(), user(r).Email); err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// TestKvkWebsiteSearch implements ServerInterface.
func (s *server) TestKvkWebsiteSearch(w http.ResponseWriter, r *http.Request) {
	if !requireInstanceAdmin(w, r) {
		return
	}
	if err := s.deps.CompanyCheck.Check(r.Context()); err != nil {
		s.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// UpdateInstanceSettings implements ServerInterface.
func (s *server) UpdateInstanceSettings(w http.ResponseWriter, r *http.Request) {
	if !requireInstanceAdmin(w, r) {
		return
	}
	body, ok := decodeBody[UpdateInstanceSettingsRequest](w, r)
	if !ok {
		return
	}
	inst, err := s.deps.Settings.Update(r.Context(), settings.Update{
		RegistrationOpen:   body.RegistrationOpen,
		GoogleEnabled:      body.GoogleEnabled,
		GoogleClientID:     body.GoogleClientId,
		GoogleClientSecret: body.GoogleClientSecret,
		LM2Enabled:         body.Lm2Enabled,
		LM2URL:             body.Lm2Url,
		LM2App:             body.Lm2App,

		KVKEnabled:              body.KvkEnabled,
		KVKAPIKey:               body.KvkApiKey,
		KVKWebsiteSearchEnabled: body.KvkWebsiteSearchEnabled,
		AddressLookupEnabled:    body.AddressLookupEnabled,
		WebhooksAllowLocal:      body.WebhooksAllowLocal,

		SMTPEnabled:  body.SmtpEnabled,
		SMTPHost:     body.SmtpHost,
		SMTPPort:     body.SmtpPort,
		SMTPUsername: body.SmtpUsername,
		SMTPPassword: body.SmtpPassword,
		SMTPFrom:     body.SmtpFrom,
		SMTPSecurity: (*string)(body.SmtpSecurity),

		StorageEnabled:   body.StorageEnabled,
		StorageEndpoint:  body.StorageEndpoint,
		StorageRegion:    body.StorageRegion,
		StorageBucket:    body.StorageBucket,
		StorageAccessKey: body.StorageAccessKey,
		StorageSecretKey: body.StorageSecretKey,
		StoragePathStyle: body.StoragePathStyle,

		RecognitionAIEnabled:     body.RecognitionAiEnabled,
		RecognitionAIConnections: fromAIConnections(body.RecognitionAiConnections),
	})
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, toInstanceSettings(inst))
}

// serveWeb serves the web application: real files where they exist, and the
// single-page entry point for every other path, so any URI can be opened.
func (s *server) serveWeb(w http.ResponseWriter, r *http.Request) {
	if strings.HasPrefix(r.URL.Path, "/api/") || (r.Method != http.MethodGet && r.Method != http.MethodHead) {
		writeError(w, http.StatusNotFound, "Not found")
		return
	}
	name := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
	for _, candidate := range []string{name, path.Join(name, "index.html")} {
		if info, err := fs.Stat(s.deps.Web, candidate); candidate != "" && err == nil && !info.IsDir() {
			s.serveFile(w, r, candidate)
			return
		}
	}
	s.serveFile(w, r, "index.html")
}

func (s *server) serveFile(w http.ResponseWriter, r *http.Request, name string) {
	content, err := fs.ReadFile(s.deps.Web, name)
	if errors.Is(err, fs.ErrNotExist) {
		writeError(w, http.StatusNotFound, "Not found")
		return
	}
	if err != nil {
		s.fail(w, r, fmt.Errorf("read %s: %w", name, err))
		return
	}
	if strings.HasPrefix(name, "assets/") {
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	} else {
		w.Header().Set("Cache-Control", "no-cache")
	}
	http.ServeContent(w, r, name, time.Time{}, strings.NewReader(string(content)))
}

func parseID(w http.ResponseWriter, id Id) (resource.ID, bool) {
	rid, err := resource.ParseID(id)
	if err != nil {
		writeError(w, http.StatusNotFound, "Not found")
		return 0, false
	}
	return rid, true
}

func deref[T any](p *T) T {
	if p == nil {
		var zero T
		return zero
	}
	return *p
}
