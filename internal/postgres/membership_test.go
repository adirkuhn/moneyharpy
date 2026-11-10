package postgres_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/adirkuhn/moneyharpy/internal/administration"
	"github.com/adirkuhn/moneyharpy/internal/banking"
	"github.com/adirkuhn/moneyharpy/internal/bookkeeping"
	"github.com/adirkuhn/moneyharpy/internal/identity"
	"github.com/adirkuhn/moneyharpy/internal/membership"
	"github.com/adirkuhn/moneyharpy/internal/postgres"
	"github.com/adirkuhn/moneyharpy/internal/resource"
	"github.com/adirkuhn/moneyharpy/internal/settings"
)

type noMail struct{}

func (noMail) SendInvitation(context.Context, string, string, string, string, string) error {
	return errors.New("email is disabled")
}
func (noMail) InvitationURL(token string) string { return "/invitations/" + token }

type noFiles struct{}

func (noFiles) PurgeFiles(context.Context, resource.ID) error { return nil }

func membershipService(t *testing.T) (*membership.Service, *administration.Service, administration.Administration, resource.ID) {
	t.Helper()
	a, _, owner := newAdministrationWithIdentity(t)
	admins := administration.NewService(postgres.NewAdministrationStore(testPool), ids, time.Now)
	idStore := postgres.NewIdentityStore(testPool)
	tfa := identity.NewService(idStore, settings.NewService(postgres.NewSettingsStore(testPool), settings.NewCipher("x")), ids,
		identity.NewHasher(identity.Params{Time: 1, MemoryKiB: 8, Threads: 1}), nil, settings.NewCipher("test-secret"), time.Now)
	return membership.NewService(admins, postgres.NewMembershipStore(testPool), noMail{}, tfa, noFiles{}, ids, time.Now), admins, a, owner
}

func tokenOf(url string) string { return url[len("/invitations/"):] }

func TestOwnerHasEveryPermission(t *testing.T) {
	_, admins, a, owner := membershipService(t)
	l, err := admins.Get(context.Background(), a.ID, owner)
	if err != nil || l.UserType != administration.Owner || !l.Can(administration.Ownership) || !l.Can(administration.Bank) {
		t.Errorf("Get() = %+v, %v", l.Membership, err)
	}
}

func TestInvitationAcceptedAndUserManaged(t *testing.T) {
	ctx := context.Background()
	svc, admins, a, owner := membershipService(t)
	inv, err := svc.Invite(ctx, a.ID, owner, membership.InviteInput{Email: "colleague@example.com", UserType: administration.Employee,
		Permissions: &[]administration.Permission{administration.Bank}}, "Owner")
	if err != nil || inv.Sent {
		t.Fatalf("Invite() = %+v, %v", inv, err)
	}
	// Inviting again replaces the open invitation.
	inv, err = svc.Invite(ctx, a.ID, owner, membership.InviteInput{Email: "colleague@example.com", UserType: administration.Employee,
		Permissions: &[]administration.Permission{administration.Bank}}, "Owner")
	if err != nil {
		t.Fatal(err)
	}
	if open, _ := svc.Invitations(ctx, a.ID, owner); len(open) != 1 {
		t.Errorf("open invitations = %d", len(open))
	}
	view, err := svc.InvitationByToken(ctx, tokenOf(inv.URL))
	if err != nil || view.AdministrationName != a.Name || view.InviterName == "" {
		t.Errorf("view = %+v, %v", view, err)
	}

	colleague := newUser(t)
	if id, err := svc.Accept(ctx, tokenOf(inv.URL), colleague.ID); err != nil || id != a.ID {
		t.Fatalf("Accept() = %d, %v", id, err)
	}
	l, err := admins.Get(ctx, a.ID, colleague.ID)
	if err != nil || l.UserType != administration.Employee || !l.Can(administration.Bank) || l.Can(administration.Settings) {
		t.Fatalf("colleague = %+v, %v", l.Membership, err)
	}
	if _, err := svc.Users(ctx, a.ID, colleague.ID, membership.UserFilter{}); !errors.Is(err, membership.ErrForbidden) {
		t.Errorf("Users() without settings error = %v", err)
	}

	users, err := svc.Users(ctx, a.ID, owner, membership.UserFilter{})
	if err != nil || len(users) != 2 {
		t.Fatalf("Users() = %+v, %v", users, err)
	}
	if _, err := svc.UpdateUser(ctx, a.ID, colleague.ID, owner, membership.UserChanges{Active: ptr(false)}); err != nil {
		t.Fatal(err)
	}
	if _, err := admins.Get(ctx, a.ID, colleague.ID); !errors.Is(err, administration.ErrNotFound) {
		t.Errorf("deactivated user still gets in: %v", err)
	}
	if users, _ = svc.Users(ctx, a.ID, owner, membership.UserFilter{IncludeInactive: true}); len(users) != 2 {
		t.Errorf("with inactive = %d", len(users))
	}

	// A new invitation brings a deactivated user back.
	again, _ := svc.Invite(ctx, a.ID, owner, membership.InviteInput{Email: "colleague2@example.com", UserType: administration.Accountant}, "Owner")
	if _, err := svc.Accept(ctx, tokenOf(again.URL), colleague.ID); err != nil {
		t.Fatal(err)
	}
	if l, _ = admins.Get(ctx, a.ID, colleague.ID); l.UserType != administration.Accountant {
		t.Errorf("returned as %s", l.UserType)
	}
	third, _ := svc.Invite(ctx, a.ID, owner, membership.InviteInput{Email: "colleague3@example.com", UserType: administration.Employee}, "Owner")
	if _, err := svc.Accept(ctx, tokenOf(third.URL), colleague.ID); !errors.Is(err, membership.ErrAlreadyMember) {
		t.Errorf("accept as a user error = %v", err)
	}

	if err := svc.Leave(ctx, a.ID, colleague.ID); err != nil {
		t.Fatal(err)
	}
	if err := svc.Leave(ctx, a.ID, owner); !errors.Is(err, membership.ErrLastOwner) {
		t.Errorf("last owner leaving error = %v", err)
	}
}

func TestSupportAccessLetsInstanceAdminsIn(t *testing.T) {
	ctx := context.Background()
	svc, admins, a, owner := membershipService(t)
	helper := newUser(t)
	if _, err := testPool.Exec(ctx, "UPDATE users SET is_instance_admin = true WHERE id = $1", int64(helper.ID)); err != nil {
		t.Fatal(err)
	}
	if _, err := admins.Get(ctx, a.ID, helper.ID); !errors.Is(err, administration.ErrNotFound) {
		t.Fatalf("instance admin without grant error = %v", err)
	}
	until := time.Now().Add(48 * time.Hour)
	if err := svc.SetSupportAccess(ctx, a.ID, owner, &until); err != nil {
		t.Fatal(err)
	}
	l, err := admins.Get(ctx, a.ID, helper.ID)
	if err != nil || l.Access != administration.AccessSupport || l.Can(administration.Ownership) || !l.Can(administration.Settings) {
		t.Fatalf("support = %+v, %v", l.Membership, err)
	}
	grants, err := svc.SupportGrants(ctx)
	if err != nil || len(grants) == 0 {
		t.Errorf("grants = %+v, %v", grants, err)
	}
	if err := svc.SetSupportAccess(ctx, a.ID, owner, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := admins.Get(ctx, a.ID, helper.ID); !errors.Is(err, administration.ErrNotFound) {
		t.Errorf("after ending support error = %v", err)
	}
}

func TestTwoFactorStore(t *testing.T) {
	ctx := context.Background()
	s := postgres.NewIdentityStore(testPool)
	u := newUser(t)
	if err := s.SetTwoFactorSecret(ctx, u.ID, "sealed"); err != nil {
		t.Fatal(err)
	}
	if err := s.EnableTwoFactor(ctx, u.ID, time.Now(), 100, []string{"a", "b"}); err != nil {
		t.Fatal(err)
	}
	st, err := s.TwoFactor(ctx, u.ID)
	if err != nil || st.EnabledAt == nil || st.Secret != "sealed" || st.RecoveryCodesLeft != 2 {
		t.Fatalf("TwoFactor() = %+v, %v", st, err)
	}
	if ok, _ := s.UseTOTPStep(ctx, u.ID, 100); ok {
		t.Error("used a step twice")
	}
	if ok, _ := s.UseTOTPStep(ctx, u.ID, 101); !ok {
		t.Error("refused a later step")
	}
	if ok, _ := s.UseRecoveryCode(ctx, u.ID, "a", time.Now()); !ok {
		t.Error("refused a recovery code")
	}
	if ok, _ := s.UseRecoveryCode(ctx, u.ID, "a", time.Now()); ok {
		t.Error("used a recovery code twice")
	}
	if err := s.InsertChallenge(ctx, "ch", u.ID, time.Now().Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	got, attempts, err := s.ChallengeUser(ctx, "ch", time.Now())
	if err != nil || got.ID != u.ID || attempts != 1 {
		t.Errorf("ChallengeUser() = %+v, %d, %v", got, attempts, err)
	}
	if _, _, err := s.ChallengeUser(ctx, "ch", time.Now().Add(time.Hour)); !errors.Is(err, identity.ErrChallengeInvalid) {
		t.Errorf("expired challenge error = %v", err)
	}
	if err := s.DisableTwoFactor(ctx, u.ID); err != nil {
		t.Fatal(err)
	}
	if st, _ = s.TwoFactor(ctx, u.ID); st.EnabledAt != nil || st.RecoveryCodesLeft != 0 {
		t.Errorf("after disable = %+v", st)
	}
}

func TestDeleteAdministrationRemovesEverything(t *testing.T) {
	ctx := context.Background()
	svc, admins, a, owner := membershipService(t)
	// Give it bookkeeping and bank data, which reference each other.
	books := bookkeeping.NewService(admins, postgres.NewBookkeepingStore(testPool), ids, time.Now)
	banks := banking.NewService(admins, books, postgres.NewBankingStore(testPool), ids, time.Now)
	acc, err := banks.CreateAccount(ctx, a.ID, owner, banking.AccountChanges{Name: ptr("Bank")})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := banks.CreateStatement(ctx, a.ID, owner, banking.StatementInput{AccountID: &acc.ID, Reference: ptr("S"), Mutations: []banking.MutationInput{{Date: ptr("2026-11-01"), Amount: ptr("5")}}}); err != nil {
		t.Fatal(err)
	}
	if err := svc.Delete(ctx, a.ID, owner, a.Name); err != nil {
		t.Fatal(err)
	}
	if _, err := admins.Get(ctx, a.ID, owner); !errors.Is(err, administration.ErrNotFound) {
		t.Errorf("deleted administration error = %v", err)
	}
	var left int
	if err := testPool.QueryRow(ctx, "SELECT count(*) FROM resources WHERE administration_id = $1", int64(a.ID)).Scan(&left); err != nil || left != 0 {
		t.Errorf("resources left = %d, %v", left, err)
	}
}
