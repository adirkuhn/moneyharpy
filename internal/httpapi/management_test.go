package httpapi_test

import (
	"context"
	"crypto/hmac"
	"crypto/sha1"
	"encoding/base32"
	"encoding/binary"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/adirkuhn/moneyharpy/internal/httpapi"
	"github.com/adirkuhn/moneyharpy/internal/postgres/postgrestest"
)

// totp computes an authenticator code the way apps do (RFC 6238).
func totp(t *testing.T, secret string, at time.Time) string {
	t.Helper()
	key, err := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(secret)
	if err != nil {
		t.Fatal(err)
	}
	var msg [8]byte
	binary.BigEndian.PutUint64(msg[:], uint64(at.Unix()/30))
	mac := hmac.New(sha1.New, key)
	mac.Write(msg[:])
	sum := mac.Sum(nil)
	o := sum[len(sum)-1] & 0x0f
	return fmt.Sprintf("%06d", (binary.BigEndian.Uint32(sum[o:o+4])&0x7fffffff)%1_000_000)
}

// invite has owner invite email and joins as invitee, returning the user's id.
func invite(t *testing.T, h *harness, owner, invitee *client, admin, email string, body map[string]any) string {
	t.Helper()
	body["email"] = email
	r := owner.do(http.MethodPost, "/api/v2/"+admin+"/user_invitations", map[string]any{"user_invitation": body})
	wantStatus(t, r, http.StatusCreated)
	inv := decode[httpapi.CreatedUserInvitation](t, r)
	token := inv.AcceptUrl[strings.LastIndex(inv.AcceptUrl, "/")+1:]
	wantStatus(t, invitee.do(http.MethodPost, "/api/invitations/"+token+"/accept", nil), http.StatusOK)
	_ = h
	return token
}

func TestPermissionsGuardEveryResource(t *testing.T) {
	h := newHarness(t, options{})
	owner, clerk := h.client(), h.client()
	owner.register()
	me := clerk.register()
	a := owner.createAdministration("Harpy")
	api := "/api/v2/" + a.Id
	invite(t, h, owner, clerk, a.Id, "clerk@example.com", map[string]any{"user_type": "employee", "permissions": []string{"bank"}})

	got := decode[httpapi.Administration](t, clerk.do(http.MethodGet, "/api/v2/administrations/"+a.Id, nil))
	if got.UserType != "employee" || len(got.Permissions) != 1 || got.Permissions[0] != "bank" {
		t.Fatalf("clerk sees %+v", got)
	}
	for _, path := range []string{"/bookkeeping_settings", "/invoice_settings", "/identities", "/users", "/tax_rates", "/user_invitations"} {
		wantStatus(t, clerk.do(http.MethodGet, api+path, nil), http.StatusForbidden)
	}
	for _, path := range []string{"/financial_accounts", "/financial_mutations", "/ledger_accounts", "/reports/bank_balance", "/bank_settings"} {
		wantStatus(t, clerk.do(http.MethodGet, api+path, nil), http.StatusOK)
	}
	wantStatus(t, clerk.do(http.MethodPatch, "/api/v2/administrations/"+a.Id, map[string]any{"administration": map[string]any{"name": "Mine"}}), http.StatusForbidden)
	wantStatus(t, clerk.do(http.MethodPost, api+"/financial_accounts", map[string]any{"financial_account": map[string]any{"name": "x"}}), http.StatusForbidden)

	users := decode[[]httpapi.User](t, owner.do(http.MethodGet, api+"/users", nil))
	if len(users) != 2 {
		t.Fatalf("users = %+v", users)
	}
	r := owner.do(http.MethodPatch, api+"/users/"+me.Id, map[string]any{"user": map[string]any{"permissions": []string{"settings", "bank"}}})
	wantStatus(t, r, http.StatusOK)
	wantStatus(t, clerk.do(http.MethodGet, api+"/bookkeeping_settings", nil), http.StatusOK)

	wantStatus(t, owner.do(http.MethodPatch, api+"/users/"+me.Id, map[string]any{"user": map[string]any{"active": false}}), http.StatusOK)
	wantStatus(t, clerk.do(http.MethodGet, "/api/v2/administrations/"+a.Id, nil), http.StatusNotFound)
	wantStatus(t, clerk.do(http.MethodGet, api+"/financial_accounts", nil), http.StatusNotFound)
}

func TestTurningTwoFactorOff(t *testing.T) {
	h := newHarness(t, options{})
	owner := h.client()
	owner.register()
	setup := decode[httpapi.TwoFactorSetup](t, owner.do(http.MethodPost, "/api/user/two_factor", nil))
	r := owner.do(http.MethodPost, "/api/user/two_factor/confirm", map[string]string{"code": totp(t, setup.Secret, time.Now())})
	wantStatus(t, r, http.StatusOK)
	codes := decode[httpapi.RecoveryCodes](t, r).RecoveryCodes

	wantStatus(t, owner.do(http.MethodDelete, "/api/user/two_factor", map[string]string{"code": "zzzz-zzzz-zzzz"}), http.StatusUnprocessableEntity)
	wantStatus(t, owner.do(http.MethodDelete, "/api/user/two_factor", map[string]string{"code": codes[0]}), http.StatusNoContent)
	if st := decode[httpapi.TwoFactorStatus](t, owner.do(http.MethodGet, "/api/user/two_factor", nil)); st.Enabled {
		t.Errorf("status = %+v", st)
	}
}

func TestUsersAndLeaving(t *testing.T) {
	h := newHarness(t, options{})
	owner, partner := h.client(), h.client()
	ownerUser := owner.register()
	partnerUser := partner.register()
	a := owner.createAdministration("Harpy")
	api := "/api/v2/" + a.Id

	wantStatus(t, owner.do(http.MethodDelete, api+"/membership", nil), http.StatusUnprocessableEntity)
	wantStatus(t, owner.do(http.MethodPatch, api+"/users/"+ownerUser.Id, map[string]any{"user": map[string]any{"user_type": "employee"}}), http.StatusUnprocessableEntity)

	invite(t, h, owner, partner, a.Id, "partner@example.com", map[string]any{"user_type": "owner"})
	u := decode[httpapi.User](t, owner.do(http.MethodGet, api+"/users/"+partnerUser.Id, nil))
	if *u.UserType != "owner" || len(*u.Permissions) != 7 {
		t.Errorf("partner = %+v", u)
	}
	accountants := decode[[]httpapi.User](t, owner.do(http.MethodGet, api+"/users?include_accountants=true&include_inactive=true", nil))
	if len(accountants) != 2 {
		t.Errorf("users with flags = %d", len(accountants))
	}
	clerk := h.client()
	clerkUser := clerk.register()
	invite(t, h, owner, clerk, a.Id, "clerk@example.com", map[string]any{"user_type": "employee"})
	wantStatus(t, clerk.do(http.MethodGet, "/api/v2/administrations/"+a.Id, nil), http.StatusOK)
	wantStatus(t, owner.do(http.MethodDelete, api+"/users/"+clerkUser.Id, nil), http.StatusNoContent)
	wantStatus(t, clerk.do(http.MethodGet, "/api/v2/administrations/"+a.Id, nil), http.StatusNotFound)
	wantStatus(t, owner.do(http.MethodDelete, api+"/membership", nil), http.StatusNoContent)
	wantStatus(t, owner.do(http.MethodGet, "/api/v2/administrations/"+a.Id, nil), http.StatusNotFound)
	wantStatus(t, partner.do(http.MethodDelete, api+"/users/"+partnerUser.Id, nil), http.StatusUnprocessableEntity)
}

func TestInvitations(t *testing.T) {
	h := newHarness(t, options{})
	owner, other := h.client(), h.client()
	owner.register()
	other.register()
	a := owner.createAdministration("Harpy")
	api := "/api/v2/" + a.Id

	r := owner.do(http.MethodPost, api+"/user_invitations", map[string]any{"user_invitation": map[string]any{"email": "New@Example.com", "user_type": "accountant"}})
	wantStatus(t, r, http.StatusCreated)
	inv := decode[httpapi.CreatedUserInvitation](t, r)
	if inv.Email != "new@example.com" || !inv.EmailSent || len(h.mailer.invitations) != 1 {
		t.Errorf("invitation = %+v", inv)
	}
	list := decode[[]httpapi.UserInvitation](t, owner.do(http.MethodGet, api+"/user_invitations", nil))
	if len(list) != 1 || list[0].Expired {
		t.Errorf("open = %+v", list)
	}
	token := h.mailer.invitations[0].token
	preview := decode[httpapi.InvitationPreview](t, h.client().do(http.MethodGet, "/api/invitations/"+token, nil))
	if preview.AdministrationName != "Harpy" || !preview.Open || preview.UserType != "accountant" {
		t.Errorf("preview = %+v", preview)
	}
	wantStatus(t, h.client().do(http.MethodPost, "/api/invitations/"+token+"/accept", nil), http.StatusUnauthorized)
	wantStatus(t, owner.do(http.MethodDelete, api+"/user_invitations/"+list[0].Id, nil), http.StatusNoContent)
	wantStatus(t, other.do(http.MethodPost, "/api/invitations/"+token+"/accept", nil), http.StatusNotFound)
	wantStatus(t, owner.do(http.MethodPost, api+"/user_invitations", map[string]any{"user_invitation": map[string]any{"email": "bad", "user_type": "employee"}}), http.StatusUnprocessableEntity)
	wantStatus(t, h.client().do(http.MethodGet, "/api/invitations/"+strings.Repeat("x", 43), nil), http.StatusNotFound)
}

func TestInvitationOpensClosedRegistration(t *testing.T) {
	ctx := context.Background()
	pool, cleanup, err := postgrestest.Start(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()
	h := newHarness(t, options{pool: pool})
	owner := h.client()
	owner.register()
	wantStatus(t, owner.do(http.MethodPatch, "/api/instance/settings", map[string]bool{"registration_open": false}), http.StatusOK)
	a := owner.createAdministration("Harpy")
	wantStatus(t, owner.do(http.MethodPost, "/api/v2/"+a.Id+"/user_invitations", map[string]any{"user_invitation": map[string]any{"email": "join@example.com", "user_type": "employee"}}), http.StatusCreated)
	token := h.mailer.invitations[0].token

	newcomer := h.client()
	wantStatus(t, newcomer.do(http.MethodPost, "/api/auth/register", map[string]any{"email": "join@example.com", "name": "Jo", "password": password}), http.StatusForbidden)
	wantStatus(t, newcomer.do(http.MethodPost, "/api/auth/register", map[string]any{"email": "someone@example.com", "name": "Jo", "password": password, "invitation_token": token}), http.StatusForbidden)
	wantStatus(t, newcomer.do(http.MethodPost, "/api/auth/register", map[string]any{"email": "join@example.com", "name": "Jo", "password": password, "invitation_token": token}), http.StatusCreated)
	wantStatus(t, newcomer.do(http.MethodPost, "/api/invitations/"+token+"/accept", nil), http.StatusOK)
}

func TestTwoFactorSignInAndRequirement(t *testing.T) {
	h := newHarness(t, options{})
	owner, colleague := h.client(), h.client()
	email := uniqueEmail()
	wantStatus(t, owner.do(http.MethodPost, "/api/auth/register", map[string]string{"email": email, "name": "Ann", "password": password}), http.StatusCreated)
	colleagueUser := colleague.register()
	a := owner.createAdministration("Harpy")
	api := "/api/v2/" + a.Id
	invite(t, h, owner, colleague, a.Id, "colleague@example.com", map[string]any{"user_type": "employee"})
	_ = colleagueUser

	wantStatus(t, owner.do(http.MethodPut, api+"/require_two_factor", nil), http.StatusUnprocessableEntity)

	r := owner.do(http.MethodPost, "/api/user/two_factor", nil)
	wantStatus(t, r, http.StatusCreated)
	setup := decode[httpapi.TwoFactorSetup](t, r)
	if !strings.HasPrefix(setup.QrSvg, "<svg") || !strings.HasPrefix(setup.OtpauthUri, "otpauth://totp/") {
		t.Errorf("setup = %+v", setup)
	}
	wantStatus(t, owner.do(http.MethodPost, "/api/user/two_factor/confirm", map[string]string{"code": "000000"}), http.StatusUnprocessableEntity)
	now := time.Now()
	r = owner.do(http.MethodPost, "/api/user/two_factor/confirm", map[string]string{"code": totp(t, setup.Secret, now)})
	wantStatus(t, r, http.StatusOK)
	codes := decode[httpapi.RecoveryCodes](t, r).RecoveryCodes
	if len(codes) != 10 {
		t.Fatalf("codes = %v", codes)
	}
	if st := decode[httpapi.TwoFactorStatus](t, owner.do(http.MethodGet, "/api/user/two_factor", nil)); !st.Enabled || st.RecoveryCodesLeft != 10 {
		t.Errorf("status = %+v", st)
	}

	// Signing in now takes a second step.
	fresh := h.client()
	r = fresh.do(http.MethodPost, "/api/auth/login", map[string]string{"email": email, "password": password})
	wantStatus(t, r, http.StatusAccepted)
	wantStatus(t, fresh.do(http.MethodGet, "/api/auth/me", nil), http.StatusUnauthorized)
	wantStatus(t, fresh.do(http.MethodPost, "/api/auth/two_factor", map[string]string{"code": "zzzz-zzzz-zzzz"}), http.StatusUnprocessableEntity)
	r = fresh.do(http.MethodPost, "/api/auth/two_factor", map[string]string{"code": codes[0]})
	wantStatus(t, r, http.StatusOK)
	if me := decode[httpapi.CurrentUser](t, r); !me.TwoFactorEnabled {
		t.Errorf("me = %+v", me)
	}
	wantStatus(t, h.client().do(http.MethodPost, "/api/auth/two_factor", map[string]string{"code": codes[1]}), http.StatusUnauthorized)

	// Requiring it locks out the colleague until they turn it on.
	wantStatus(t, owner.do(http.MethodPut, api+"/require_two_factor", nil), http.StatusNoContent)
	wantStatus(t, colleague.do(http.MethodGet, api+"/financial_mutations", nil), http.StatusForbidden)
	got := decode[httpapi.Administration](t, colleague.do(http.MethodGet, "/api/v2/administrations/"+a.Id, nil))
	if !got.RequireTwoFactor {
		t.Error("requirement not shown")
	}
	wantStatus(t, owner.do(http.MethodGet, api+"/financial_mutations", nil), http.StatusOK)
	wantStatus(t, owner.do(http.MethodDelete, "/api/user/two_factor", map[string]string{"code": codes[2]}), http.StatusUnprocessableEntity)
	r = owner.do(http.MethodPost, "/api/user/two_factor/recovery_codes", map[string]string{"code": codes[3]})
	wantStatus(t, r, http.StatusOK)
}

func TestSupportAccessAndDelete(t *testing.T) {
	ctx := context.Background()
	pool, cleanup, err := postgrestest.Start(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()
	h := newHarness(t, options{pool: pool})
	helper, owner := h.client(), h.client()
	if !helper.register().InstanceAdmin {
		t.Fatal("first user is not the instance admin")
	}
	owner.register()
	a := owner.createAdministration("Harpy")
	api := "/api/v2/" + a.Id

	wantStatus(t, helper.do(http.MethodGet, "/api/v2/administrations/"+a.Id, nil), http.StatusNotFound)
	until := time.Now().Add(72 * time.Hour).UTC().Truncate(time.Second)
	r := owner.do(http.MethodPut, api+"/support_access", map[string]any{"until": until})
	wantStatus(t, r, http.StatusOK)
	got := decode[httpapi.Administration](t, helper.do(http.MethodGet, "/api/v2/administrations/"+a.Id, nil))
	if got.Access != "support" || got.UserType != "support" {
		t.Errorf("helper sees %+v", got)
	}
	wantStatus(t, helper.do(http.MethodGet, api+"/bookkeeping_settings", nil), http.StatusOK)
	wantStatus(t, helper.do(http.MethodGet, api+"/user_invitations", nil), http.StatusForbidden)
	grants := decode[[]httpapi.SupportGrant](t, helper.do(http.MethodGet, "/api/instance/support_access", nil))
	if len(grants) != 1 || grants[0].Name != "Harpy" {
		t.Errorf("grants = %+v", grants)
	}
	wantStatus(t, owner.do(http.MethodGet, "/api/instance/support_access", nil), http.StatusForbidden)
	wantStatus(t, owner.do(http.MethodPut, api+"/support_access", map[string]any{"until": time.Now().AddDate(1, 0, 0)}), http.StatusUnprocessableEntity)
	wantStatus(t, owner.do(http.MethodPut, api+"/support_access", map[string]any{"until": nil}), http.StatusOK)
	wantStatus(t, helper.do(http.MethodGet, api+"/bookkeeping_settings", nil), http.StatusNotFound)

	wantStatus(t, owner.do(http.MethodPost, "/api/v2/administrations/"+a.Id+"/delete", map[string]string{"name": "harpy"}), http.StatusUnprocessableEntity)
	wantStatus(t, owner.do(http.MethodPost, "/api/v2/administrations/"+a.Id+"/delete", map[string]string{"name": "Harpy"}), http.StatusNoContent)
	wantStatus(t, owner.do(http.MethodGet, "/api/v2/administrations/"+a.Id, nil), http.StatusNotFound)
}

func TestCountryChangeMovesTheVATReturn(t *testing.T) {
	h := newHarness(t, options{})
	owner := h.client()
	owner.register()
	a := owner.createAdministration("Harpy")
	r := owner.do(http.MethodPatch, "/api/v2/administrations/"+a.Id, map[string]any{"administration": map[string]any{"country": "BE"}})
	wantStatus(t, r, http.StatusOK)
	if got := decode[httpapi.Administration](t, r); got.Country != "BE" {
		t.Errorf("country = %s", got.Country)
	}
	st := decode[httpapi.BookkeepingSettings](t, owner.do(http.MethodGet, "/api/v2/"+a.Id+"/bookkeeping_settings", nil))
	if len(st.TaxReportBoxes) != 0 {
		t.Errorf("boxes = %+v", st.TaxReportBoxes)
	}
	wantStatus(t, owner.do(http.MethodPatch, "/api/v2/administrations/"+a.Id, map[string]any{"administration": map[string]any{"country": "XX"}}), http.StatusUnprocessableEntity)
}
