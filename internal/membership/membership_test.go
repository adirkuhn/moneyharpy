package membership

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/adirkuhn/moneyharpy/internal/administration"
	"github.com/adirkuhn/moneyharpy/internal/resource"
)

const (
	adminID                        resource.ID = 100
	owner, employee, other, admin2 resource.ID = 1, 2, 3, 4
)

var now = time.Date(2026, 11, 10, 9, 0, 0, 0, time.UTC)

type memStore struct {
	members     map[resource.ID]Member
	invitations map[string]Invitation
	require2FA  bool
	support     *time.Time
	deleted     bool
}

func (m *memStore) Members(context.Context, resource.ID) ([]Member, error) {
	out := []Member{}
	for _, v := range m.members {
		out = append(out, v)
	}
	return out, nil
}
func (m *memStore) Member(_ context.Context, _, user resource.ID) (Member, error) {
	v, ok := m.members[user]
	if !ok {
		return Member{}, ErrUserNotFound
	}
	return v, nil
}
func (m *memStore) UpdateMember(_ context.Context, _ resource.ID, v Member) error {
	m.members[v.UserID] = v
	return nil
}
func (m *memStore) RemoveMember(_ context.Context, _, user resource.ID) error {
	delete(m.members, user)
	return nil
}
func (m *memStore) Invitations(context.Context, resource.ID) ([]Invitation, error) {
	out := []Invitation{}
	for _, v := range m.invitations {
		if v.AcceptedAt == nil {
			out = append(out, v)
		}
	}
	return out, nil
}
func (m *memStore) CreateInvitation(_ context.Context, inv Invitation, hash string) error {
	m.invitations[hash] = inv
	return nil
}
func (m *memStore) InvitationByToken(_ context.Context, hash string) (InvitationView, error) {
	v, ok := m.invitations[hash]
	if !ok {
		return InvitationView{}, ErrInvitationNotFound
	}
	return InvitationView{Invitation: v, AdministrationName: "Harpy"}, nil
}
func (m *memStore) RevokeInvitation(_ context.Context, _, id resource.ID) error {
	for k, v := range m.invitations {
		if v.ID == id {
			delete(m.invitations, k)
			return nil
		}
	}
	return ErrInvitationNotFound
}
func (m *memStore) AcceptInvitation(_ context.Context, inv Invitation, user resource.ID, at time.Time) error {
	if mem, ok := m.members[user]; ok && mem.Active {
		return ErrAlreadyMember
	}
	for k, v := range m.invitations {
		if v.ID == inv.ID {
			v.AcceptedAt = &at
			m.invitations[k] = v
		}
	}
	m.members[user] = Member{UserID: user, UserType: inv.UserType, Permissions: inv.Permissions, Active: true}
	return nil
}
func (m *memStore) SetRequireTwoFactor(context.Context, resource.ID) error {
	m.require2FA = true
	return nil
}
func (m *memStore) SetSupportAccess(_ context.Context, _ resource.ID, until *time.Time) error {
	m.support = until
	return nil
}
func (m *memStore) SupportGrants(context.Context, time.Time) ([]Grant, error) { return nil, nil }
func (m *memStore) DeleteAdministration(context.Context, resource.ID) error {
	m.deleted = true
	return nil
}

// fakeAdmins answers from the store's members, as the real store does.
type fakeAdmins struct{ store *memStore }

func (f fakeAdmins) Get(_ context.Context, id, user resource.ID) (administration.Listed, error) {
	m, ok := f.store.members[user]
	if id != adminID || !ok || !m.Active {
		return administration.Listed{}, administration.ErrNotFound
	}
	return administration.Listed{
		Administration: administration.Administration{ID: adminID, Name: "Harpy", Language: "en", RequireTwoFactor: f.store.require2FA},
		Membership:     administration.Membership{Access: administration.AccessUser, UserType: m.UserType, Permissions: m.Permissions},
	}, nil
}

type fakeMailer struct{ sent []string }

func (f *fakeMailer) SendInvitation(_ context.Context, to, token, _, _, _ string) error {
	f.sent = append(f.sent, to+" "+token)
	return nil
}
func (f *fakeMailer) InvitationURL(token string) string {
	return "https://example.com/invitations/" + token
}

type fakeTFA map[resource.ID]bool

func (f fakeTFA) TwoFactorEnabled(_ context.Context, user resource.ID) (bool, error) {
	return f[user], nil
}

type fakeFiles struct{ purged bool }

func (f *fakeFiles) PurgeFiles(context.Context, resource.ID) error {
	f.purged = true
	return nil
}

func newTestService() (*Service, *memStore, *fakeMailer, *fakeFiles) {
	store := &memStore{members: map[resource.ID]Member{
		owner:    {UserID: owner, Name: "Owner", Email: "owner@example.com", UserType: administration.Owner, Permissions: slices.Clone(administration.Permissions), Active: true},
		employee: {UserID: employee, Name: "Employee", Email: "employee@example.com", UserType: administration.Employee, Permissions: []administration.Permission{administration.Bank}, Active: true},
	}, invitations: map[string]Invitation{}}
	mailer, files := &fakeMailer{}, &fakeFiles{}
	svc := NewService(fakeAdmins{store}, store, mailer, fakeTFA{owner: true}, files, resource.NewGenerator(1), func() time.Time { return now })
	return svc, store, mailer, files
}

func ptr[T any](v T) *T { return &v }

func fieldsOf(t *testing.T, err error) map[string][]string {
	t.Helper()
	var verr *resource.ValidationError
	if !errors.As(err, &verr) {
		t.Fatalf("error = %v, want *resource.ValidationError", err)
	}
	return verr.Fields
}

func TestUsersNeedSettings(t *testing.T) {
	svc, store, _, _ := newTestService()
	ctx := context.Background()
	store.members[other] = Member{UserID: other, Name: "Accountant", UserType: administration.Accountant, Active: true}
	list, err := svc.Users(ctx, adminID, owner, UserFilter{})
	if err != nil || len(list) != 2 {
		t.Fatalf("Users() = %+v, %v", list, err)
	}
	if list, _ = svc.Users(ctx, adminID, owner, UserFilter{IncludeAccountants: true}); len(list) != 3 {
		t.Errorf("with accountants = %d", len(list))
	}
	if _, err := svc.Users(ctx, adminID, employee, UserFilter{}); !errors.Is(err, ErrForbidden) {
		t.Errorf("employee without settings error = %v", err)
	}
}

func TestUpdateUserNormalizesAndKeepsAnOwner(t *testing.T) {
	svc, _, _, _ := newTestService()
	ctx := context.Background()
	got, err := svc.UpdateUser(ctx, adminID, employee, owner, UserChanges{Permissions: ptr([]administration.Permission{administration.Settings, administration.Ownership, administration.Bank})})
	if err != nil || !slices.Equal(got.Permissions, []administration.Permission{administration.Bank, administration.Settings}) {
		t.Errorf("employee permissions = %v, %v", got.Permissions, err)
	}
	if got, _ = svc.UpdateUser(ctx, adminID, employee, owner, UserChanges{UserType: ptr(administration.Owner)}); len(got.Permissions) != len(administration.Permissions) {
		t.Errorf("owner permissions = %v", got.Permissions)
	}
	if got, err = svc.UpdateUser(ctx, adminID, owner, owner, UserChanges{UserType: ptr(administration.Employee)}); err != nil || got.UserType != administration.Employee {
		t.Errorf("demote with another owner = %+v, %v", got, err)
	}
	if _, err := svc.UpdateUser(ctx, adminID, employee, employee, UserChanges{UserType: ptr(administration.Employee)}); !errors.Is(err, ErrLastOwner) {
		t.Errorf("demote the last owner error = %v", err)
	}
	if _, err := svc.UpdateUser(ctx, adminID, employee, owner, UserChanges{}); !errors.Is(err, ErrForbidden) {
		t.Errorf("employee changing users error = %v", err)
	}
	if _, err := svc.UpdateUser(ctx, adminID, employee, employee, UserChanges{UserType: ptr(administration.UserType("boss"))}); fieldsOf(t, err)["user_type"] == nil {
		t.Error("accepted an unknown user type")
	}
}

func TestLeaveAndRemove(t *testing.T) {
	svc, store, _, _ := newTestService()
	ctx := context.Background()
	if err := svc.Leave(ctx, adminID, owner); !errors.Is(err, ErrLastOwner) {
		t.Errorf("last owner leaving error = %v", err)
	}
	if err := svc.RemoveUser(ctx, adminID, owner, employee); !errors.Is(err, ErrForbidden) {
		t.Errorf("employee removing the owner error = %v", err)
	}
	if err := svc.Leave(ctx, adminID, employee); err != nil {
		t.Fatal(err)
	}
	if _, ok := store.members[employee]; ok {
		t.Error("employee still a user")
	}
	if err := svc.RemoveUser(ctx, adminID, 9, owner); !errors.Is(err, ErrUserNotFound) {
		t.Errorf("unknown user error = %v", err)
	}
}

func TestInviteAndAccept(t *testing.T) {
	svc, store, mailer, _ := newTestService()
	ctx := context.Background()
	inv, err := svc.Invite(ctx, adminID, owner, InviteInput{Email: " New@Example.com ", UserType: administration.Accountant}, "Owner")
	if err != nil {
		t.Fatal(err)
	}
	if inv.Email != "new@example.com" || !inv.Sent || len(mailer.sent) != 1 || slices.Contains(inv.Permissions, administration.Ownership) || !inv.ExpiresAt.Equal(now.Add(invitationLifetime)) {
		t.Errorf("invited = %+v", inv)
	}
	token := inv.URL[len("https://example.com/invitations/"):]
	view, err := svc.InvitationByToken(ctx, token)
	if err != nil || view.AdministrationName != "Harpy" {
		t.Errorf("view = %+v, %v", view, err)
	}
	id, err := svc.Accept(ctx, token, other)
	if err != nil || id != adminID || store.members[other].UserType != administration.Accountant {
		t.Fatalf("Accept() = %d, %v; member %+v", id, err, store.members[other])
	}
	if _, err := svc.Accept(ctx, token, admin2); !errors.Is(err, ErrInvitationUsed) {
		t.Errorf("second accept error = %v", err)
	}
	if _, err := svc.Accept(ctx, "nope", other); !errors.Is(err, ErrInvitationNotFound) {
		t.Errorf("unknown token error = %v", err)
	}

	_, err = svc.Invite(ctx, adminID, owner, InviteInput{Email: "employee@example.com", UserType: "boss"}, "Owner")
	if fieldsOf(t, err)["user_type"] == nil {
		t.Error("accepted an unknown user type")
	}
	if _, err := svc.Invite(ctx, adminID, owner, InviteInput{Email: "employee@example.com", UserType: administration.Employee}, "Owner"); fieldsOf(t, err)["email"] == nil {
		t.Error("invited a user who is one already")
	}
	if _, err := svc.Invite(ctx, adminID, owner, InviteInput{Email: "not an address", UserType: administration.Employee}, "Owner"); fieldsOf(t, err)["email"] == nil {
		t.Error("accepted a bad address")
	}
	if _, err := svc.Invite(ctx, adminID, employee, InviteInput{Email: "x@example.com", UserType: administration.Employee}, "E"); !errors.Is(err, ErrForbidden) {
		t.Errorf("employee inviting error = %v", err)
	}
}

func TestExpiredInvitation(t *testing.T) {
	svc, store, _, _ := newTestService()
	ctx := context.Background()
	inv, _ := svc.Invite(ctx, adminID, owner, InviteInput{Email: "late@example.com", UserType: administration.Employee}, "Owner")
	for k, v := range store.invitations {
		v.ExpiresAt = now.Add(-time.Minute)
		store.invitations[k] = v
	}
	if _, err := svc.Accept(ctx, inv.URL[len("https://example.com/invitations/"):], other); !errors.Is(err, ErrInvitationUsed) {
		t.Errorf("expired accept error = %v", err)
	}
}

func TestRequireTwoFactorAndSupportAccess(t *testing.T) {
	svc, store, _, _ := newTestService()
	ctx := context.Background()
	if err := svc.RequireTwoFactor(ctx, adminID, owner); err != nil || !store.require2FA {
		t.Errorf("require = %v, %v", store.require2FA, err)
	}
	store.members[other] = Member{UserID: other, UserType: administration.Owner, Permissions: administration.Permissions, Active: true}
	store.require2FA = false
	if err := svc.RequireTwoFactor(ctx, adminID, other); !errors.Is(err, ErrTwoFactorNeeded) {
		t.Errorf("owner without 2FA error = %v", err)
	}

	until := now.Add(7 * 24 * time.Hour)
	if err := svc.SetSupportAccess(ctx, adminID, owner, &until); err != nil || !store.support.Equal(until) {
		t.Errorf("support = %v, %v", store.support, err)
	}
	tooLong := now.Add(100 * 24 * time.Hour)
	if fieldsOf(t, svc.SetSupportAccess(ctx, adminID, owner, &tooLong))["until"] == nil {
		t.Error("accepted support access beyond 90 days")
	}
	if err := svc.SetSupportAccess(ctx, adminID, owner, nil); err != nil || store.support != nil {
		t.Errorf("end support = %v, %v", store.support, err)
	}
}

func TestDelete(t *testing.T) {
	svc, store, _, files := newTestService()
	ctx := context.Background()
	if fieldsOf(t, svc.Delete(ctx, adminID, owner, "harpy"))["name"] == nil {
		t.Error("deleted without the exact name")
	}
	if err := svc.Delete(ctx, adminID, employee, "Harpy"); !errors.Is(err, ErrForbidden) {
		t.Errorf("employee delete error = %v", err)
	}
	if err := svc.Delete(ctx, adminID, owner, "Harpy"); err != nil || !store.deleted || !files.purged {
		t.Errorf("delete = %v, deleted %v, purged %v", err, store.deleted, files.purged)
	}
}
