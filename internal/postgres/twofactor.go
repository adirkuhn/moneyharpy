package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/adirkuhn/moneyharpy/internal/identity"
	"github.com/adirkuhn/moneyharpy/internal/postgres/sqlc"
	"github.com/adirkuhn/moneyharpy/internal/resource"
)

// TwoFactor implements identity.Store.
func (s *IdentityStore) TwoFactor(ctx context.Context, user resource.ID) (identity.TwoFactorState, error) {
	r, err := s.q.UserTwoFactor(ctx, int64(user))
	if err != nil {
		return identity.TwoFactorState{}, userErr(err)
	}
	return identity.TwoFactorState{Secret: r.TotpSecret, EnabledAt: timeOf(r.TotpEnabledAt), RecoveryCodesLeft: int(r.CodesLeft)}, nil
}

// SetTwoFactorSecret implements identity.Store.
func (s *IdentityStore) SetTwoFactorSecret(ctx context.Context, user resource.ID, sealed string) error {
	return s.q.SetTOTPSecret(ctx, sqlc.SetTOTPSecretParams{ID: int64(user), TotpSecret: sealed})
}

// EnableTwoFactor implements identity.Store.
func (s *IdentityStore) EnableTwoFactor(ctx context.Context, user resource.ID, at time.Time, step int64, codeHashes []string) error {
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		q := s.q.WithTx(tx)
		if err := q.EnableTOTP(ctx, sqlc.EnableTOTPParams{ID: int64(user), TotpEnabledAt: timestamp(at), TotpLastStep: step}); err != nil {
			return err
		}
		return replaceCodes(ctx, q, user, codeHashes)
	})
}

// DisableTwoFactor implements identity.Store.
func (s *IdentityStore) DisableTwoFactor(ctx context.Context, user resource.ID) error {
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		q := s.q.WithTx(tx)
		if err := q.DisableTOTP(ctx, int64(user)); err != nil {
			return err
		}
		return q.DeleteRecoveryCodes(ctx, int64(user))
	})
}

// ReplaceRecoveryCodes implements identity.Store.
func (s *IdentityStore) ReplaceRecoveryCodes(ctx context.Context, user resource.ID, codeHashes []string) error {
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error { return replaceCodes(ctx, s.q.WithTx(tx), user, codeHashes) })
}

func replaceCodes(ctx context.Context, q *sqlc.Queries, user resource.ID, hashes []string) error {
	if err := q.DeleteRecoveryCodes(ctx, int64(user)); err != nil {
		return err
	}
	for _, h := range hashes {
		if err := q.InsertRecoveryCode(ctx, sqlc.InsertRecoveryCodeParams{UserID: int64(user), CodeHash: h}); err != nil {
			return err
		}
	}
	return nil
}

// UseTOTPStep implements identity.Store.
func (s *IdentityStore) UseTOTPStep(ctx context.Context, user resource.ID, step int64) (bool, error) {
	n, err := s.q.UseTOTPStep(ctx, sqlc.UseTOTPStepParams{ID: int64(user), TotpLastStep: step})
	return n == 1, err
}

// UseRecoveryCode implements identity.Store.
func (s *IdentityStore) UseRecoveryCode(ctx context.Context, user resource.ID, codeHash string, at time.Time) (bool, error) {
	n, err := s.q.UseRecoveryCode(ctx, sqlc.UseRecoveryCodeParams{UserID: int64(user), CodeHash: codeHash, UsedAt: timestamp(at)})
	return n == 1, err
}

// RequiresTwoFactor implements identity.Store.
func (s *IdentityStore) RequiresTwoFactor(ctx context.Context, user resource.ID) (bool, error) {
	return s.q.RequiresTwoFactor(ctx, int64(user))
}

// InsertChallenge implements identity.Store.
func (s *IdentityStore) InsertChallenge(ctx context.Context, hash string, user resource.ID, expires time.Time) error {
	return s.q.InsertLoginChallenge(ctx, sqlc.InsertLoginChallengeParams{TokenHash: hash, UserID: int64(user), ExpiresAt: timestamp(expires)})
}

// ChallengeUser implements identity.Store.
func (s *IdentityStore) ChallengeUser(ctx context.Context, hash string, now time.Time) (identity.User, int, error) {
	r, err := s.q.SpendLoginChallengeAttempt(ctx, sqlc.SpendLoginChallengeAttemptParams{TokenHash: hash, ExpiresAt: timestamp(now)})
	if errors.Is(err, pgx.ErrNoRows) {
		return identity.User{}, 0, identity.ErrChallengeInvalid
	}
	if err != nil {
		return identity.User{}, 0, err
	}
	return user(r.ID, r.Email, r.Name, r.IsInstanceAdmin), int(r.Attempts), nil
}

// DeleteChallenge implements identity.Store. Expired challenges go too.
func (s *IdentityStore) DeleteChallenge(ctx context.Context, hash string) error {
	return s.q.DeleteLoginChallenge(ctx, hash)
}
