// Package membership manages who works in an administration: its users and
// their permissions, invitations, leaving, support access for the instance
// admins, requiring two-step verification and deleting the administration.
package membership

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"net/mail"
	"slices"
	"strings"
	"time"

	"github.com/adirkuhn/moneyharpy/internal/administration"
	"github.com/adirkuhn/moneyharpy/internal/resource"
)

// Errors returned by Service.
var (
	ErrUserNotFound       = errors.New("user not found")
	ErrInvitationNotFound = errors.New("invitation not found")
	// ErrInvitationUsed reports an invitation that was accepted or expired.
	ErrInvitationUsed = errors.New("invitation was used or expired")
	// ErrForbidden reports a change the user's permissions do not allow.
	ErrForbidden = errors.New("not allowed")
	// ErrLastOwner reports a change that would leave nobody to own the
	// administration.
	ErrLastOwner = errors.New("the administration needs an owner")
	// ErrAlreadyMember reports inviting or accepting for someone who is a
	// user already.
	ErrAlreadyMember = errors.New("already a user of the administration")
	// ErrTwoFactorNeeded reports requiring two-step verification without
	// having it set up yourself.
	ErrTwoFactorNeeded = errors.New("set up two-step verification for your own account first")
)

// invitationLifetime is how long an invitation can be accepted.
const invitationLifetime = 14 * 24 * time.Hour

// maxSupportAccess is the longest support access lasts.
const maxSupportAccess = 90 * 24 * time.Hour

// Member is a user of an administration.
type Member struct {
	UserID         resource.ID
	Name           string
	Email          string
	EmailValidated bool
	UserType       administration.UserType
	Permissions    []administration.Permission
	Active         bool
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

func (m Member) owner() bool { return m.Active && m.UserType == administration.Owner }

// Invitation asks someone to join an administration.
type Invitation struct {
	ID               resource.ID
	AdministrationID resource.ID
	Email            string
	UserType         administration.UserType
	Permissions      []administration.Permission
	InvitedBy        resource.ID
	CreatedAt        time.Time
	ExpiresAt        time.Time
	AcceptedAt       *time.Time
}

// Open reports whether the invitation can still be accepted.
func (i Invitation) Open(now time.Time) bool { return i.AcceptedAt == nil && now.Before(i.ExpiresAt) }

// InvitationView is what someone holding an invitation link may see.
type InvitationView struct {
	Invitation
	AdministrationName string
	InviterName        string
}

// Grant is an administration that lets the instance admins in.
type Grant struct {
	AdministrationID resource.ID
	Name             string
	Until            time.Time
}

// Store persists memberships and invitations.
type Store interface {
	Members(ctx context.Context, admin resource.ID) ([]Member, error)
	// Member returns ErrUserNotFound for someone who is no user of admin.
	Member(ctx context.Context, admin, user resource.ID) (Member, error)
	UpdateMember(ctx context.Context, admin resource.ID, m Member) error
	RemoveMember(ctx context.Context, admin, user resource.ID) error

	Invitations(ctx context.Context, admin resource.ID) ([]Invitation, error)
	CreateInvitation(ctx context.Context, inv Invitation, tokenHash string) error
	// InvitationByToken returns ErrInvitationNotFound for an unknown token.
	InvitationByToken(ctx context.Context, tokenHash string) (InvitationView, error)
	RevokeInvitation(ctx context.Context, admin, id resource.ID) error
	// AcceptInvitation marks the invitation accepted and adds the user, or
	// returns ErrAlreadyMember.
	AcceptInvitation(ctx context.Context, inv Invitation, user resource.ID, now time.Time) error

	SetRequireTwoFactor(ctx context.Context, admin resource.ID) error
	SetSupportAccess(ctx context.Context, admin resource.ID, until *time.Time) error
	SupportGrants(ctx context.Context, now time.Time) ([]Grant, error)
	DeleteAdministration(ctx context.Context, admin resource.ID) error
}

// Administrations gives access to administrations.
type Administrations interface {
	Get(ctx context.Context, id, user resource.ID) (administration.Listed, error)
}

// Mailer sends invitations. It returns an error wrapping ErrMailDisabled
// when email is off, so the link can be shared another way.
type Mailer interface {
	SendInvitation(ctx context.Context, to, token, administration, inviter, language string) error
	InvitationURL(token string) string
}

// TwoFactor tells whether a user signs in with two-step verification.
type TwoFactor interface {
	TwoFactorEnabled(ctx context.Context, user resource.ID) (bool, error)
}

// FilePurger removes an administration's stored files.
type FilePurger interface {
	PurgeFiles(ctx context.Context, admin resource.ID) error
}

// Service implements the membership use cases.
type Service struct {
	admins Administrations
	store  Store
	mailer Mailer
	tfa    TwoFactor
	files  FilePurger
	ids    *resource.Generator
	now    func() time.Time
}

// NewService returns a Service.
func NewService(admins Administrations, store Store, mailer Mailer, tfa TwoFactor, files FilePurger, ids *resource.Generator, now func() time.Time) *Service {
	return &Service{admins: admins, store: store, mailer: mailer, tfa: tfa, files: files, ids: ids, now: now}
}

// access returns the administration as seen by user, refusing with
// ErrForbidden unless they hold one of the permissions.
func (s *Service) access(ctx context.Context, admin, user resource.ID, perms ...administration.Permission) (administration.Listed, error) {
	a, err := s.admins.Get(ctx, admin, user)
	if err != nil {
		return administration.Listed{}, err
	}
	if !a.Can(perms...) {
		return administration.Listed{}, ErrForbidden
	}
	return a, nil
}

// UserFilter widens the user list, after Moneybird's include flags.
type UserFilter struct{ IncludeInactive, IncludeAccountants bool }

// Users lists the administration's users by name.
func (s *Service) Users(ctx context.Context, admin, user resource.ID, f UserFilter) ([]Member, error) {
	if _, err := s.access(ctx, admin, user, administration.Settings); err != nil {
		return nil, err
	}
	list, err := s.store.Members(ctx, admin)
	if err != nil {
		return nil, fmt.Errorf("list users: %w", err)
	}
	list = slices.DeleteFunc(list, func(m Member) bool {
		return (!m.Active && !f.IncludeInactive) || (m.UserType == administration.Accountant && !f.IncludeAccountants)
	})
	slices.SortStableFunc(list, func(a, b Member) int { return strings.Compare(strings.ToLower(a.Name), strings.ToLower(b.Name)) })
	return list, nil
}

// User returns one user of the administration.
func (s *Service) User(ctx context.Context, admin, id, user resource.ID) (Member, error) {
	if _, err := s.access(ctx, admin, user, administration.Settings); err != nil {
		return Member{}, err
	}
	return s.store.Member(ctx, admin, id)
}

// UserChanges holds the fields to set; nil fields stay as they are.
type UserChanges struct {
	UserType    *administration.UserType
	Permissions *[]administration.Permission
	Active      *bool
}

// UpdateUser changes a user's type, permissions or whether they are active.
// Owners hold every permission; others never hold ownership.
func (s *Service) UpdateUser(ctx context.Context, admin, id, user resource.ID, c UserChanges) (Member, error) {
	if _, err := s.access(ctx, admin, user, administration.Ownership); err != nil {
		return Member{}, err
	}
	m, err := s.store.Member(ctx, admin, id)
	if err != nil {
		return Member{}, err
	}
	wasOwner := m.owner()
	if c.UserType != nil {
		m.UserType = *c.UserType
	}
	if c.Permissions != nil {
		m.Permissions = *c.Permissions
	}
	if c.Active != nil {
		m.Active = *c.Active
	}
	if m.UserType, m.Permissions, err = normalize(m.UserType, m.Permissions); err != nil {
		return Member{}, err
	}
	if wasOwner && !m.owner() {
		if err := s.keepOwner(ctx, admin, id); err != nil {
			return Member{}, err
		}
	}
	m.UpdatedAt = s.now()
	if err := s.store.UpdateMember(ctx, admin, m); err != nil {
		return Member{}, fmt.Errorf("store user: %w", err)
	}
	return m, nil
}

// RemoveUser takes a user out of the administration.
func (s *Service) RemoveUser(ctx context.Context, admin, id, user resource.ID) error {
	if _, err := s.access(ctx, admin, user, administration.Ownership); err != nil {
		return err
	}
	return s.remove(ctx, admin, id)
}

// Leave takes the user out of the administration themselves.
func (s *Service) Leave(ctx context.Context, admin, user resource.ID) error {
	a, err := s.access(ctx, admin, user)
	if err != nil {
		return err
	}
	if a.Access == administration.AccessSupport {
		return ErrForbidden
	}
	return s.remove(ctx, admin, user)
}

func (s *Service) remove(ctx context.Context, admin, id resource.ID) error {
	m, err := s.store.Member(ctx, admin, id)
	if err != nil {
		return err
	}
	if m.owner() {
		if err := s.keepOwner(ctx, admin, id); err != nil {
			return err
		}
	}
	return s.store.RemoveMember(ctx, admin, id)
}

// keepOwner refuses when id is the only active owner.
func (s *Service) keepOwner(ctx context.Context, admin, id resource.ID) error {
	list, err := s.store.Members(ctx, admin)
	if err != nil {
		return fmt.Errorf("list users: %w", err)
	}
	if !slices.ContainsFunc(list, func(m Member) bool { return m.UserID != id && m.owner() }) {
		return ErrLastOwner
	}
	return nil
}

// UserTypes are the types people can be invited as.
var UserTypes = []administration.UserType{administration.Owner, administration.Employee, administration.Accountant}

// DefaultPermissions are what a new user of the type starts with.
func DefaultPermissions(t administration.UserType) []administration.Permission {
	switch t {
	case administration.Owner:
		return slices.Clone(administration.Permissions)
	case administration.Accountant:
		return []administration.Permission{administration.SalesInvoices, administration.Documents, administration.Estimates, administration.Bank, administration.Settings, administration.TimeEntries}
	default:
		return []administration.Permission{administration.SalesInvoices, administration.Documents, administration.Estimates, administration.TimeEntries}
	}
}

// normalize checks a type and permissions and puts them in Moneybird's
// order: owners hold every permission, others never ownership.
func normalize(t administration.UserType, perms []administration.Permission) (administration.UserType, []administration.Permission, error) {
	var verr resource.ValidationError
	if !slices.Contains(UserTypes, t) {
		verr.Add("user_type", "is not included in the list")
	}
	for _, p := range perms {
		if !slices.Contains(administration.Permissions, p) {
			verr.Add("permissions", string(p)+" is not included in the list")
		}
	}
	if err := verr.OrNil(); err != nil {
		return t, nil, err
	}
	if t == administration.Owner {
		return t, slices.Clone(administration.Permissions), nil
	}
	out := []administration.Permission{}
	for _, p := range administration.Permissions {
		if p != administration.Ownership && slices.Contains(perms, p) {
			out = append(out, p)
		}
	}
	return t, out, nil
}

// InviteInput is who to invite and with what rights. Nil permissions take
// the type's defaults.
type InviteInput struct {
	Email       string
	UserType    administration.UserType
	Permissions *[]administration.Permission
}

// Invited is a new invitation with its link, which works only through the
// token in it.
type Invited struct {
	Invitation
	URL string
	// Sent reports whether the email went out; without email the inviter
	// shares the link themselves.
	Sent bool
}

// Invite asks someone to join by email.
func (s *Service) Invite(ctx context.Context, admin, user resource.ID, in InviteInput, inviterName string) (Invited, error) {
	a, err := s.access(ctx, admin, user, administration.Ownership)
	if err != nil {
		return Invited{}, err
	}
	var verr resource.ValidationError
	email := strings.ToLower(strings.TrimSpace(in.Email))
	if addr, err := mail.ParseAddress(email); err != nil || addr.Address != email || len(email) > 254 {
		verr.Add("email", "is not a valid email address")
	}
	perms := DefaultPermissions(in.UserType)
	if in.Permissions != nil {
		perms = *in.Permissions
	}
	t, perms, err := normalize(in.UserType, perms)
	if err != nil {
		return Invited{}, err
	}
	members, err := s.store.Members(ctx, admin)
	if err != nil {
		return Invited{}, fmt.Errorf("list users: %w", err)
	}
	if slices.ContainsFunc(members, func(m Member) bool { return m.Active && strings.EqualFold(m.Email, email) }) {
		verr.Add("email", "is already a user of this administration")
	}
	if err := verr.OrNil(); err != nil {
		return Invited{}, err
	}
	now := s.now()
	inv := Invitation{ID: s.ids.Next(), AdministrationID: admin, Email: email, UserType: t, Permissions: perms, InvitedBy: user, CreatedAt: now, ExpiresAt: now.Add(invitationLifetime)}
	token := randomToken()
	if err := s.store.CreateInvitation(ctx, inv, hashToken(token)); err != nil {
		return Invited{}, err
	}
	out := Invited{Invitation: inv, URL: s.mailer.InvitationURL(token)}
	if err := s.mailer.SendInvitation(ctx, email, token, a.Name, inviterName, a.Language); err == nil {
		out.Sent = true
	}
	return out, nil
}

// Invitations lists the invitations not accepted yet, expired ones too.
func (s *Service) Invitations(ctx context.Context, admin, user resource.ID) ([]Invitation, error) {
	if _, err := s.access(ctx, admin, user, administration.Ownership); err != nil {
		return nil, err
	}
	return s.store.Invitations(ctx, admin)
}

// RevokeInvitation withdraws an invitation.
func (s *Service) RevokeInvitation(ctx context.Context, admin, id, user resource.ID) error {
	if _, err := s.access(ctx, admin, user, administration.Ownership); err != nil {
		return err
	}
	return s.store.RevokeInvitation(ctx, admin, id)
}

// InvitationByToken shows an invitation to whoever holds its link.
func (s *Service) InvitationByToken(ctx context.Context, token string) (InvitationView, error) {
	return s.store.InvitationByToken(ctx, hashToken(token))
}

// Accept adds the signed-in user to the invitation's administration.
func (s *Service) Accept(ctx context.Context, token string, user resource.ID) (resource.ID, error) {
	v, err := s.store.InvitationByToken(ctx, hashToken(token))
	if err != nil {
		return 0, err
	}
	now := s.now()
	if !v.Open(now) {
		return 0, ErrInvitationUsed
	}
	if err := s.store.AcceptInvitation(ctx, v.Invitation, user, now); err != nil {
		return 0, err
	}
	return v.AdministrationID, nil
}

// RequireTwoFactor makes every user sign in with two-step verification. As
// at Moneybird, it cannot be turned off again.
func (s *Service) RequireTwoFactor(ctx context.Context, admin, user resource.ID) error {
	a, err := s.access(ctx, admin, user, administration.Ownership)
	if err != nil {
		return err
	}
	if a.RequireTwoFactor {
		return nil
	}
	on, err := s.tfa.TwoFactorEnabled(ctx, user)
	if err != nil {
		return err
	}
	if !on {
		return ErrTwoFactorNeeded
	}
	return s.store.SetRequireTwoFactor(ctx, admin)
}

// SetSupportAccess lets the instance admins in until a moment at most 90
// days ahead, or ends their access with nil.
func (s *Service) SetSupportAccess(ctx context.Context, admin, user resource.ID, until *time.Time) error {
	if _, err := s.access(ctx, admin, user, administration.Ownership); err != nil {
		return err
	}
	if until != nil {
		now := s.now()
		if !until.After(now) || until.After(now.Add(maxSupportAccess)) {
			var verr resource.ValidationError
			verr.Add("until", "must be in the next 90 days")
			return &verr
		}
	}
	return s.store.SetSupportAccess(ctx, admin, until)
}

// SupportGrants lists the administrations that let the instance admins in
// now. Callers check the user is an instance admin.
func (s *Service) SupportGrants(ctx context.Context) ([]Grant, error) {
	return s.store.SupportGrants(ctx, s.now())
}

// Delete removes the administration with all its data and files, for good.
// confirmName must be the administration's name.
func (s *Service) Delete(ctx context.Context, admin, user resource.ID, confirmName string) error {
	a, err := s.access(ctx, admin, user, administration.Ownership)
	if err != nil {
		return err
	}
	if strings.TrimSpace(confirmName) != a.Name {
		var verr resource.ValidationError
		verr.Add("name", "does not match the administration's name")
		return &verr
	}
	if err := s.files.PurgeFiles(ctx, admin); err != nil {
		return fmt.Errorf("remove files: %w", err)
	}
	return s.store.DeleteAdministration(ctx, admin)
}

func randomToken() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		panic(fmt.Sprintf("read random: %v", err))
	}
	return base64.RawURLEncoding.EncodeToString(b)
}

func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}
