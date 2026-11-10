package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/adirkuhn/moneyharpy/internal/administration"
	"github.com/adirkuhn/moneyharpy/internal/membership"
	"github.com/adirkuhn/moneyharpy/internal/postgres/sqlc"
	"github.com/adirkuhn/moneyharpy/internal/resource"
)

// MembershipStore implements membership.Store.
type MembershipStore struct {
	pool *pgxpool.Pool
	q    *sqlc.Queries
}

// NewMembershipStore returns a MembershipStore on pool.
func NewMembershipStore(pool *pgxpool.Pool) *MembershipStore {
	return &MembershipStore{pool: pool, q: sqlc.New(pool)}
}

func toMember(r sqlc.ListMembersRow) membership.Member {
	return membership.Member{
		UserID: resource.ID(r.ID), Name: r.Name, Email: r.Email, EmailValidated: r.EmailValidated, UserType: administration.UserType(r.UserType),
		Permissions: permissionsOf(r.Permissions), Active: r.Active, CreatedAt: r.CreatedAt.Time, UpdatedAt: r.UpdatedAt.Time,
	}
}

func permissionStrings(ps []administration.Permission) []string {
	out := make([]string, len(ps))
	for i, p := range ps {
		out[i] = string(p)
	}
	return out
}

// Members implements membership.Store.
func (s *MembershipStore) Members(ctx context.Context, admin resource.ID) ([]membership.Member, error) {
	rows, err := s.q.ListMembers(ctx, int64(admin))
	if err != nil {
		return nil, err
	}
	out := make([]membership.Member, len(rows))
	for i, r := range rows {
		out[i] = toMember(r)
	}
	return out, nil
}

// Member implements membership.Store.
func (s *MembershipStore) Member(ctx context.Context, admin, user resource.ID) (membership.Member, error) {
	r, err := s.q.MemberByUser(ctx, sqlc.MemberByUserParams{AdministrationID: int64(admin), UserID: int64(user)})
	if errors.Is(err, pgx.ErrNoRows) {
		return membership.Member{}, membership.ErrUserNotFound
	}
	if err != nil {
		return membership.Member{}, err
	}
	return toMember(sqlc.ListMembersRow(r)), nil
}

// UpdateMember implements membership.Store.
func (s *MembershipStore) UpdateMember(ctx context.Context, admin resource.ID, m membership.Member) error {
	return s.q.UpdateMember(ctx, sqlc.UpdateMemberParams{
		AdministrationID: int64(admin), UserID: int64(m.UserID), UserType: string(m.UserType), Permissions: permissionStrings(m.Permissions),
		Active: m.Active, UpdatedAt: timestamp(m.UpdatedAt),
	})
}

// RemoveMember implements membership.Store.
func (s *MembershipStore) RemoveMember(ctx context.Context, admin, user resource.ID) error {
	return s.q.DeleteMember(ctx, sqlc.DeleteMemberParams{AdministrationID: int64(admin), UserID: int64(user)})
}

func toInvitation(r sqlc.UserInvitation) membership.Invitation {
	return membership.Invitation{
		ID: resource.ID(r.ID), AdministrationID: resource.ID(r.AdministrationID), Email: r.Email, UserType: administration.UserType(r.UserType),
		Permissions: permissionsOf(r.Permissions), InvitedBy: resource.ID(r.InvitedBy.Int64), CreatedAt: r.CreatedAt.Time,
		ExpiresAt: r.ExpiresAt.Time, AcceptedAt: timeOf(r.AcceptedAt),
	}
}

// Invitations implements membership.Store.
func (s *MembershipStore) Invitations(ctx context.Context, admin resource.ID) ([]membership.Invitation, error) {
	rows, err := s.q.ListOpenInvitations(ctx, int64(admin))
	if err != nil {
		return nil, err
	}
	out := make([]membership.Invitation, len(rows))
	for i, r := range rows {
		out[i] = toInvitation(r)
	}
	return out, nil
}

// CreateInvitation implements membership.Store. A new invitation replaces
// an open one for the same address.
func (s *MembershipStore) CreateInvitation(ctx context.Context, inv membership.Invitation, tokenHash string) error {
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		q := s.q.WithTx(tx)
		if err := q.DeleteOpenInvitationsFor(ctx, sqlc.DeleteOpenInvitationsForParams{AdministrationID: int64(inv.AdministrationID), Lower: inv.Email}); err != nil {
			return err
		}
		return q.InsertInvitation(ctx, sqlc.InsertInvitationParams{
			ID: int64(inv.ID), AdministrationID: int64(inv.AdministrationID), Email: inv.Email, UserType: string(inv.UserType),
			Permissions: permissionStrings(inv.Permissions), TokenHash: tokenHash, InvitedBy: pgtype.Int8{Int64: int64(inv.InvitedBy), Valid: inv.InvitedBy != 0},
			CreatedAt: timestamp(inv.CreatedAt), ExpiresAt: timestamp(inv.ExpiresAt),
		})
	})
}

// InvitationByToken implements membership.Store.
func (s *MembershipStore) InvitationByToken(ctx context.Context, tokenHash string) (membership.InvitationView, error) {
	r, err := s.q.InvitationByToken(ctx, tokenHash)
	if errors.Is(err, pgx.ErrNoRows) {
		return membership.InvitationView{}, membership.ErrInvitationNotFound
	}
	if err != nil {
		return membership.InvitationView{}, err
	}
	inv := toInvitation(sqlc.UserInvitation{
		ID: r.ID, AdministrationID: r.AdministrationID, Email: r.Email, UserType: r.UserType, Permissions: r.Permissions, TokenHash: r.TokenHash,
		InvitedBy: r.InvitedBy, CreatedAt: r.CreatedAt, ExpiresAt: r.ExpiresAt, AcceptedAt: r.AcceptedAt, AcceptedBy: r.AcceptedBy,
	})
	return membership.InvitationView{Invitation: inv, AdministrationName: r.AdministrationName, InviterName: r.InviterName}, nil
}

// RevokeInvitation implements membership.Store.
func (s *MembershipStore) RevokeInvitation(ctx context.Context, admin, id resource.ID) error {
	n, err := s.q.DeleteInvitation(ctx, sqlc.DeleteInvitationParams{AdministrationID: int64(admin), ID: int64(id)})
	if err != nil {
		return err
	}
	if n == 0 {
		return membership.ErrInvitationNotFound
	}
	return nil
}

// AcceptInvitation implements membership.Store.
func (s *MembershipStore) AcceptInvitation(ctx context.Context, inv membership.Invitation, user resource.ID, now time.Time) error {
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		q := s.q.WithTx(tx)
		n, err := q.MarkInvitationAccepted(ctx, sqlc.MarkInvitationAcceptedParams{ID: int64(inv.ID), AcceptedAt: timestamp(now), AcceptedBy: pgtype.Int8{Int64: int64(user), Valid: true}})
		if err != nil {
			return err
		}
		if n == 0 {
			return membership.ErrInvitationUsed
		}
		n, err = q.UpsertInvitedMember(ctx, sqlc.UpsertInvitedMemberParams{
			AdministrationID: int64(inv.AdministrationID), UserID: int64(user), UserType: string(inv.UserType),
			Permissions: permissionStrings(inv.Permissions), CreatedAt: timestamp(now),
		})
		if err != nil {
			return err
		}
		if n == 0 {
			return membership.ErrAlreadyMember
		}
		return nil
	})
}

// SetRequireTwoFactor implements membership.Store.
func (s *MembershipStore) SetRequireTwoFactor(ctx context.Context, admin resource.ID) error {
	return s.q.SetRequireTwoFactor(ctx, int64(admin))
}

// SetSupportAccess implements membership.Store.
func (s *MembershipStore) SetSupportAccess(ctx context.Context, admin resource.ID, until *time.Time) error {
	return s.q.SetSupportAccess(ctx, sqlc.SetSupportAccessParams{ID: int64(admin), SupportAccessUntil: optionalTime(until)})
}

// SupportGrants implements membership.Store.
func (s *MembershipStore) SupportGrants(ctx context.Context, now time.Time) ([]membership.Grant, error) {
	rows, err := s.q.SupportGrants(ctx, timestamp(now))
	if err != nil {
		return nil, err
	}
	out := make([]membership.Grant, len(rows))
	for i, r := range rows {
		out[i] = membership.Grant{AdministrationID: resource.ID(r.ID), Name: r.Name, Until: r.SupportAccessUntil.Time}
	}
	return out, nil
}

// DeleteAdministration implements membership.Store. Everything that belongs
// to the administration goes with it through the foreign keys.
func (s *MembershipStore) DeleteAdministration(ctx context.Context, admin resource.ID) error {
	return s.q.DeleteAdministration(ctx, int64(admin))
}
