package httpapi

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/getkin/kin-openapi/routers/gorillamux"
	"rsc.io/qr"

	"github.com/adirkuhn/moneyharpy/internal/administration"
	"github.com/adirkuhn/moneyharpy/internal/identity"
	"github.com/adirkuhn/moneyharpy/internal/membership"
	"github.com/adirkuhn/moneyharpy/internal/resource"
)

const (
	// signInCookie carries a sign-in waiting for its second step.
	signInCookie     = "moneyharpy_signin"
	signInCookiePath = "/api/auth/two_factor"
	signInCookieAge  = 10 * time.Minute
)

// permissionRules reads the x-permissions of every operation in the
// contract. Every operation on an administration must declare them, so none
// can be left open by mistake.
func permissionRules(spec *openapi3.T) (map[string][]administration.Permission, error) {
	rules := map[string][]administration.Permission{}
	for path, item := range spec.Paths.Map() {
		for method, op := range item.Operations() {
			raw, ok := op.Extensions["x-permissions"]
			scoped := strings.Contains(path, "{administration_id}") || strings.HasPrefix(path, "/api/v2/administrations/{id}")
			if !ok {
				if scoped {
					return nil, fmt.Errorf("%s %s declares no x-permissions", method, path)
				}
				continue
			}
			list, ok := raw.([]any)
			if !ok {
				return nil, fmt.Errorf("%s %s: x-permissions is not a list", method, path)
			}
			perms := []administration.Permission{}
			for _, v := range list {
				p, ok := v.(string)
				if !ok || !slices.Contains(administration.Permissions, administration.Permission(p)) {
					return nil, fmt.Errorf("%s %s: unknown permission %v", method, path, v)
				}
				perms = append(perms, administration.Permission(p))
			}
			rules[op.OperationID] = perms
		}
	}
	return rules, nil
}

// authorize lets a request on an administration through only for a user
// who holds one of the operation's permissions and, where the administration
// requires it, signs in with two-step verification.
func (s *server) authorize(spec *openapi3.T, rules map[string][]administration.Permission) (func(http.Handler) http.Handler, error) {
	router, err := gorillamux.NewRouter(spec)
	if err != nil {
		return nil, fmt.Errorf("route contract: %w", err)
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !strings.HasPrefix(r.URL.Path, "/api/v2/") {
				next.ServeHTTP(w, r)
				return
			}
			route, params, err := router.FindRoute(r)
			if err != nil {
				next.ServeHTTP(w, r)
				return
			}
			r = r.WithContext(context.WithValue(r.Context(), routeKey, routeInfo{op: route.Operation.OperationID, params: params}))
			perms, ruled := rules[route.Operation.OperationID]
			u, signedIn := userFrom(r.Context())
			if !ruled || !signedIn {
				next.ServeHTTP(w, r)
				return
			}
			raw := params["administration_id"]
			if raw == "" {
				raw = params["id"]
			}
			admin, err := resource.ParseID(raw)
			if err != nil {
				next.ServeHTTP(w, r) // The contract answers malformed ids.
				return
			}
			if g, ok := grantFrom(r.Context()); ok {
				switch {
				case admin != g.AdministrationID:
					s.fail(w, r, administration.ErrNotFound)
					return
				case g.ReadOnly && r.Method != http.MethodGet:
					writeError(w, http.StatusForbidden, "This connection may only read")
					return
				case !g.Allows(perms...):
					writeError(w, http.StatusForbidden, "This connection has no scope for this")
					return
				}
			}
			if err := s.allowed(r.Context(), admin, u.ID, perms); err != nil {
				s.fail(w, r, err)
				return
			}
			next.ServeHTTP(w, r)
		})
	}, nil
}

// requireAlso checks a permission an operation needs only for some requests,
// beyond the ones its x-permissions declare, for the connection and the user.
// It answers the request and reports false when the permission is missing.
func (s *server) requireAlso(w http.ResponseWriter, r *http.Request, admin resource.ID, perm administration.Permission) bool {
	if g, ok := grantFrom(r.Context()); ok && !g.Allows(perm) {
		writeError(w, http.StatusForbidden, "This connection has no scope for this")
		return false
	}
	if err := s.allowed(r.Context(), admin, user(r).ID, []administration.Permission{perm}); err != nil {
		s.fail(w, r, err)
		return false
	}
	return true
}

var (
	errNoPermission   = errors.New("no permission")
	errNeedsTwoFactor = errors.New("administration requires two-step verification")
)

func (s *server) allowed(ctx context.Context, admin, user resource.ID, perms []administration.Permission) error {
	l, err := s.deps.Administrations.Get(ctx, admin, user)
	if err != nil {
		return err
	}
	if len(perms) > 0 && l.RequireTwoFactor {
		on, err := s.deps.Identity.TwoFactorEnabled(ctx, user)
		if err != nil {
			return err
		}
		if !on {
			return errNeedsTwoFactor
		}
	}
	if !l.Can(perms...) {
		names := make([]string, len(perms))
		for i, p := range perms {
			names[i] = string(p)
		}
		return fmt.Errorf("%w: needs %s", errNoPermission, strings.Join(names, " or "))
	}
	return nil
}

func toUser(m membership.Member, a administration.Listed) User {
	perms := make([]UserPermissions, len(m.Permissions))
	for i, p := range m.Permissions {
		perms[i] = UserPermissions(p)
	}
	return User{
		Id: ptr(m.UserID.String()), Name: ptr(m.Name), Email: ptr(m.Email), EmailValidated: ptr(m.EmailValidated),
		Language: ptr(a.Language), TimeZone: ptr(a.TimeZone), UserType: ptr(UserUserType(m.UserType)), Permissions: &perms,
		Active: m.Active, CreatedAt: ptr(m.CreatedAt), UpdatedAt: ptr(m.UpdatedAt),
	}
}

// ListUsers implements ServerInterface.
func (s *server) ListUsers(w http.ResponseWriter, r *http.Request, administrationID AdministrationId, params ListUsersParams) {
	admin, ok := parseID(w, administrationID)
	if !ok {
		return
	}
	a, err := s.deps.Administrations.Get(r.Context(), admin, user(r).ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	list, err := s.deps.Membership.Users(r.Context(), admin, user(r).ID, membership.UserFilter{IncludeInactive: deref(params.IncludeInactive), IncludeAccountants: deref(params.IncludeAccountants)})
	if err != nil {
		s.fail(w, r, err)
		return
	}
	out := make([]User, len(list))
	for i, m := range list {
		out[i] = toUser(m, a)
	}
	writeJSON(w, http.StatusOK, out)
}

// GetUser implements ServerInterface.
func (s *server) GetUser(w http.ResponseWriter, r *http.Request, administrationID AdministrationId, id Id) {
	admin, uid, ok := parseIDs(w, administrationID, id)
	if !ok {
		return
	}
	a, err := s.deps.Administrations.Get(r.Context(), admin, user(r).ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	m, err := s.deps.Membership.User(r.Context(), admin, uid, user(r).ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, toUser(m, a))
}

// UpdateUser implements ServerInterface.
func (s *server) UpdateUser(w http.ResponseWriter, r *http.Request, administrationID AdministrationId, id Id) {
	admin, uid, ok := parseIDs(w, administrationID, id)
	if !ok {
		return
	}
	body, ok := decodeBody[UserRequest](w, r)
	if !ok {
		return
	}
	in := body.User
	c := membership.UserChanges{Active: in.Active, UserType: (*administration.UserType)(in.UserType)}
	if in.Permissions != nil {
		perms := make([]administration.Permission, len(*in.Permissions))
		for i, p := range *in.Permissions {
			perms[i] = administration.Permission(p)
		}
		c.Permissions = &perms
	}
	a, err := s.deps.Administrations.Get(r.Context(), admin, user(r).ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	m, err := s.deps.Membership.UpdateUser(r.Context(), admin, uid, user(r).ID, c)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, toUser(m, a))
}

// RemoveUser implements ServerInterface.
func (s *server) RemoveUser(w http.ResponseWriter, r *http.Request, administrationID AdministrationId, id Id) {
	admin, uid, ok := parseIDs(w, administrationID, id)
	if !ok {
		return
	}
	if err := s.deps.Membership.RemoveUser(r.Context(), admin, uid, user(r).ID); err != nil {
		s.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// LeaveAdministration implements ServerInterface.
func (s *server) LeaveAdministration(w http.ResponseWriter, r *http.Request, administrationID AdministrationId) {
	admin, ok := parseID(w, administrationID)
	if !ok {
		return
	}
	if err := s.deps.Membership.Leave(r.Context(), admin, user(r).ID); err != nil {
		s.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func toInvitation(i membership.Invitation, now time.Time) UserInvitation {
	perms := make([]Permission, len(i.Permissions))
	for j, p := range i.Permissions {
		perms[j] = Permission(p)
	}
	return UserInvitation{
		Id: i.ID.String(), Email: i.Email, UserType: UserType(i.UserType), Permissions: perms, CreatedAt: i.CreatedAt,
		ExpiresAt: i.ExpiresAt, Expired: !now.Before(i.ExpiresAt),
	}
}

// ListUserInvitations implements ServerInterface.
func (s *server) ListUserInvitations(w http.ResponseWriter, r *http.Request, administrationID AdministrationId) {
	admin, ok := parseID(w, administrationID)
	if !ok {
		return
	}
	list, err := s.deps.Membership.Invitations(r.Context(), admin, user(r).ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	out := make([]UserInvitation, len(list))
	for i, inv := range list {
		out[i] = toInvitation(inv, time.Now())
	}
	writeJSON(w, http.StatusOK, out)
}

// CreateUserInvitation implements ServerInterface.
func (s *server) CreateUserInvitation(w http.ResponseWriter, r *http.Request, administrationID AdministrationId) {
	admin, ok := parseID(w, administrationID)
	if !ok {
		return
	}
	body, ok := decodeBody[UserInvitationRequest](w, r)
	if !ok {
		return
	}
	in := body.UserInvitation
	input := membership.InviteInput{Email: in.Email, UserType: administration.UserType(in.UserType)}
	if in.Permissions != nil {
		perms := make([]administration.Permission, len(*in.Permissions))
		for i, p := range *in.Permissions {
			perms[i] = administration.Permission(p)
		}
		input.Permissions = &perms
	}
	u := user(r)
	inv, err := s.deps.Membership.Invite(r.Context(), admin, u.ID, input, u.Name)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, CreatedUserInvitation{
		Id: inv.ID.String(), Email: inv.Email, UserType: UserType(inv.UserType), Permissions: toInvitation(inv.Invitation, time.Now()).Permissions,
		CreatedAt: inv.CreatedAt, ExpiresAt: inv.ExpiresAt, AcceptUrl: inv.URL, EmailSent: inv.Sent,
	})
}

// RevokeUserInvitation implements ServerInterface.
func (s *server) RevokeUserInvitation(w http.ResponseWriter, r *http.Request, administrationID AdministrationId, id Id) {
	admin, iid, ok := parseIDs(w, administrationID, id)
	if !ok {
		return
	}
	if err := s.deps.Membership.RevokeInvitation(r.Context(), admin, iid, user(r).ID); err != nil {
		s.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// GetInvitation implements ServerInterface.
func (s *server) GetInvitation(w http.ResponseWriter, r *http.Request, token string) {
	v, err := s.deps.Membership.InvitationByToken(r.Context(), token)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, InvitationPreview{
		AdministrationName: v.AdministrationName, InviterName: v.InviterName, Email: v.Email, UserType: UserType(v.UserType), Open: v.Open(time.Now()),
	})
}

// AcceptInvitation implements ServerInterface.
func (s *server) AcceptInvitation(w http.ResponseWriter, r *http.Request, token string) {
	u := user(r)
	admin, err := s.deps.Membership.Accept(r.Context(), token, u.ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	l, err := s.deps.Administrations.Get(r.Context(), admin, u.ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, toAdministration(l))
}

// RequireTwoFactor implements ServerInterface.
func (s *server) RequireTwoFactor(w http.ResponseWriter, r *http.Request, administrationID AdministrationId) {
	admin, ok := parseID(w, administrationID)
	if !ok {
		return
	}
	if err := s.deps.Membership.RequireTwoFactor(r.Context(), admin, user(r).ID); err != nil {
		s.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// UpdateSupportAccess implements ServerInterface.
func (s *server) UpdateSupportAccess(w http.ResponseWriter, r *http.Request, administrationID AdministrationId) {
	admin, ok := parseID(w, administrationID)
	if !ok {
		return
	}
	body, ok := decodeBody[SupportAccess](w, r)
	if !ok {
		return
	}
	if err := s.deps.Membership.SetSupportAccess(r.Context(), admin, user(r).ID, body.Until); err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, body)
}

// ListSupportGrants implements ServerInterface.
func (s *server) ListSupportGrants(w http.ResponseWriter, r *http.Request) {
	if !requireInstanceAdmin(w, r) {
		return
	}
	grants, err := s.deps.Membership.SupportGrants(r.Context())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	out := make([]SupportGrant, len(grants))
	for i, g := range grants {
		out[i] = SupportGrant{AdministrationId: g.AdministrationID.String(), Name: g.Name, Until: g.Until, Uri: "/" + g.AdministrationID.String()}
	}
	writeJSON(w, http.StatusOK, out)
}

// DeleteAdministration implements ServerInterface.
func (s *server) DeleteAdministration(w http.ResponseWriter, r *http.Request, id Id) {
	admin, ok := parseID(w, id)
	if !ok {
		return
	}
	body, ok := decodeBody[DeleteAdministrationRequest](w, r)
	if !ok {
		return
	}
	if err := s.deps.Membership.Delete(r.Context(), admin, user(r).ID, body.Name); err != nil {
		s.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// currentUser answers with the signed-in user and their two-step status.
func (s *server) currentUser(w http.ResponseWriter, r *http.Request, status int, u identity.User) {
	on, err := s.deps.Identity.TwoFactorEnabled(r.Context(), u.ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, status, CurrentUser{Id: u.ID.String(), Email: u.Email, Name: u.Name, InstanceAdmin: u.InstanceAdmin, TwoFactorEnabled: on})
}

// signedIn starts the session, or asks for the second step.
func (s *server) signedIn(w http.ResponseWriter, r *http.Request, status int, res identity.LoginResult) {
	if res.Challenge != "" {
		s.setChallengeCookie(w, signInCookie, signInCookiePath, res.Challenge, signInCookieAge)
		writeJSON(w, http.StatusAccepted, TwoFactorRequired{TwoFactorRequired: true})
		return
	}
	s.setSession(w, res.SessionToken, sessionMaxAge)
	s.currentUser(w, r, status, res.User)
}

// CompleteSignIn implements ServerInterface.
func (s *server) CompleteSignIn(w http.ResponseWriter, r *http.Request) {
	body, ok := decodeBody[TwoFactorCode](w, r)
	if !ok {
		return
	}
	c, err := r.Cookie(signInCookie)
	if err != nil {
		s.fail(w, r, identity.ErrChallengeInvalid)
		return
	}
	res, err := s.deps.Identity.CompleteSignIn(r.Context(), c.Value, body.Code)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.setChallengeCookie(w, signInCookie, signInCookiePath, "", -time.Second)
	s.signedIn(w, r, http.StatusOK, res)
}

// GetTwoFactor implements ServerInterface.
func (s *server) GetTwoFactor(w http.ResponseWriter, r *http.Request) {
	st, required, err := s.deps.Identity.TwoFactorStatus(r.Context(), user(r).ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, TwoFactorStatus{Enabled: st.EnabledAt != nil, Required: required, RecoveryCodesLeft: st.RecoveryCodesLeft})
}

// StartTwoFactor implements ServerInterface.
func (s *server) StartTwoFactor(w http.ResponseWriter, r *http.Request) {
	setup, err := s.deps.Identity.StartTwoFactor(r.Context(), user(r))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	svg, err := qrSVG(setup.URI)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, TwoFactorSetup{Secret: setup.Secret, OtpauthUri: setup.URI, QrSvg: svg})
}

// ConfirmTwoFactor implements ServerInterface.
func (s *server) ConfirmTwoFactor(w http.ResponseWriter, r *http.Request) {
	body, ok := decodeBody[TwoFactorCode](w, r)
	if !ok {
		return
	}
	codes, err := s.deps.Identity.ConfirmTwoFactor(r.Context(), user(r).ID, body.Code)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, RecoveryCodes{RecoveryCodes: codes})
}

// DisableTwoFactor implements ServerInterface.
func (s *server) DisableTwoFactor(w http.ResponseWriter, r *http.Request) {
	body, ok := decodeBody[TwoFactorCode](w, r)
	if !ok {
		return
	}
	if err := s.deps.Identity.DisableTwoFactor(r.Context(), user(r).ID, body.Code); err != nil {
		s.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// NewRecoveryCodes implements ServerInterface.
func (s *server) NewRecoveryCodes(w http.ResponseWriter, r *http.Request) {
	body, ok := decodeBody[TwoFactorCode](w, r)
	if !ok {
		return
	}
	codes, err := s.deps.Identity.NewRecoveryCodes(r.Context(), user(r).ID, body.Code)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, RecoveryCodes{RecoveryCodes: codes})
}

// qrSVG draws text as a QR code in SVG, with a quiet zone of four modules.
func qrSVG(text string) (string, error) {
	code, err := qr.Encode(text, qr.M)
	if err != nil {
		return "", fmt.Errorf("encode QR code: %w", err)
	}
	size := code.Size + 8
	var b strings.Builder
	fmt.Fprintf(&b, `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 %d %d" shape-rendering="crispEdges"><rect width="%d" height="%d" fill="#fff"/><path fill="#000" d="`, size, size, size, size)
	for y := range code.Size {
		for x := range code.Size {
			if code.Black(x, y) {
				fmt.Fprintf(&b, "M%d %dh1v1h-1z", x+4, y+4)
			}
		}
	}
	b.WriteString(`"/></svg>`)
	return b.String(), nil
}
