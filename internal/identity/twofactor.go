package identity

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/adirkuhn/moneyharpy/internal/resource"
)

// Errors of two-step verification.
var (
	ErrInvalidCode       = errors.New("the code is not valid")
	ErrChallengeInvalid  = errors.New("the sign-in expired; sign in again")
	ErrTwoFactorOn       = errors.New("two-step verification is on already")
	ErrTwoFactorOff      = errors.New("two-step verification is off")
	ErrTwoFactorRequired = errors.New("an administration you work in requires two-step verification")
)

const (
	challengeLifetime = 10 * time.Minute
	maxAttempts       = 5
	recoveryCodes     = 10
	totpIssuer        = "MoneyHarpy"
)

// Sealer encrypts secrets at rest.
type Sealer interface {
	Seal(plaintext string) (string, error)
	Open(sealed string) (string, error)
}

// TwoFactorState is a user's two-step verification as stored.
type TwoFactorState struct {
	// Secret is sealed; set while setting up and once enabled.
	Secret    string
	EnabledAt *time.Time
	// RecoveryCodesLeft counts unused recovery codes.
	RecoveryCodesLeft int
}

// TwoFactorStore persists two-step verification. It is part of Store.
type TwoFactorStore interface {
	TwoFactor(ctx context.Context, user resource.ID) (TwoFactorState, error)
	SetTwoFactorSecret(ctx context.Context, user resource.ID, sealed string) error
	// EnableTwoFactor turns it on with the step just used and replaces the
	// recovery codes.
	EnableTwoFactor(ctx context.Context, user resource.ID, at time.Time, step int64, codeHashes []string) error
	DisableTwoFactor(ctx context.Context, user resource.ID) error
	ReplaceRecoveryCodes(ctx context.Context, user resource.ID, codeHashes []string) error
	// UseTOTPStep records a used step, returning false for a step not later
	// than the last one, so a code works only once.
	UseTOTPStep(ctx context.Context, user resource.ID, step int64) (bool, error)
	// UseRecoveryCode spends a code, returning false for an unknown or spent
	// one.
	UseRecoveryCode(ctx context.Context, user resource.ID, codeHash string, at time.Time) (bool, error)
	// RequiresTwoFactor reports membership of an administration that
	// requires two-step verification.
	RequiresTwoFactor(ctx context.Context, user resource.ID) (bool, error)

	InsertChallenge(ctx context.Context, hash string, user resource.ID, expires time.Time) error
	// ChallengeUser counts an attempt and returns the challenge's user and
	// attempts so far, or ErrChallengeInvalid when unknown or expired.
	ChallengeUser(ctx context.Context, hash string, now time.Time) (User, int, error)
	DeleteChallenge(ctx context.Context, hash string) error
}

// TwoFactorSetup is a new authenticator secret, to show as a QR code.
type TwoFactorSetup struct {
	Secret string
	URI    string
}

// TwoFactorEnabled reports whether the user signs in with two-step
// verification.
func (s *Service) TwoFactorEnabled(ctx context.Context, user resource.ID) (bool, error) {
	st, err := s.store.TwoFactor(ctx, user)
	return st.EnabledAt != nil, err
}

// TwoFactorStatus returns the user's two-step verification and whether an
// administration requires it.
func (s *Service) TwoFactorStatus(ctx context.Context, user resource.ID) (TwoFactorState, bool, error) {
	st, err := s.store.TwoFactor(ctx, user)
	if err != nil {
		return TwoFactorState{}, false, err
	}
	required, err := s.store.RequiresTwoFactor(ctx, user)
	st.Secret = ""
	return st, required, err
}

// StartTwoFactor makes a new secret for an authenticator app. It is used
// once confirmed with a code.
func (s *Service) StartTwoFactor(ctx context.Context, user User) (TwoFactorSetup, error) {
	st, err := s.store.TwoFactor(ctx, user.ID)
	if err != nil {
		return TwoFactorSetup{}, err
	}
	if st.EnabledAt != nil {
		return TwoFactorSetup{}, ErrTwoFactorOn
	}
	secret := make([]byte, 20)
	if _, err := rand.Read(secret); err != nil {
		return TwoFactorSetup{}, fmt.Errorf("read random: %w", err)
	}
	sealed, err := s.sealer.Seal(base32NoPad.EncodeToString(secret))
	if err != nil {
		return TwoFactorSetup{}, fmt.Errorf("seal secret: %w", err)
	}
	if err := s.store.SetTwoFactorSecret(ctx, user.ID, sealed); err != nil {
		return TwoFactorSetup{}, err
	}
	return TwoFactorSetup{Secret: base32NoPad.EncodeToString(secret), URI: otpauthURI(totpIssuer, user.Email, secret)}, nil
}

// ConfirmTwoFactor turns two-step verification on with a code from the app,
// returning recovery codes to keep somewhere safe.
func (s *Service) ConfirmTwoFactor(ctx context.Context, user resource.ID, code string) ([]string, error) {
	st, err := s.store.TwoFactor(ctx, user)
	if err != nil {
		return nil, err
	}
	if st.EnabledAt != nil {
		return nil, ErrTwoFactorOn
	}
	if st.Secret == "" {
		return nil, ErrTwoFactorOff
	}
	secret, err := s.openSecret(st.Secret)
	if err != nil {
		return nil, err
	}
	step, ok := matchTOTP(secret, normalizeCode(code), s.now())
	if !ok {
		return nil, ErrInvalidCode
	}
	codes, hashes := newRecoveryCodes()
	if err := s.store.EnableTwoFactor(ctx, user, s.now(), step, hashes); err != nil {
		return nil, err
	}
	return codes, nil
}

// DisableTwoFactor turns two-step verification off, unless an
// administration requires it.
func (s *Service) DisableTwoFactor(ctx context.Context, user resource.ID, code string) error {
	if err := s.verify(ctx, user, code); err != nil {
		return err
	}
	required, err := s.store.RequiresTwoFactor(ctx, user)
	if err != nil {
		return err
	}
	if required {
		return ErrTwoFactorRequired
	}
	return s.store.DisableTwoFactor(ctx, user)
}

// NewRecoveryCodes replaces the recovery codes, after a valid code.
func (s *Service) NewRecoveryCodes(ctx context.Context, user resource.ID, code string) ([]string, error) {
	if err := s.verify(ctx, user, code); err != nil {
		return nil, err
	}
	codes, hashes := newRecoveryCodes()
	if err := s.store.ReplaceRecoveryCodes(ctx, user, hashes); err != nil {
		return nil, err
	}
	return codes, nil
}

// CompleteSignIn finishes a sign-in that waits for its second step, with a
// code from the app or a recovery code.
func (s *Service) CompleteSignIn(ctx context.Context, challenge, code string) (LoginResult, error) {
	hash := hashSecret(challenge)
	user, attempts, err := s.store.ChallengeUser(ctx, hash, s.now())
	if err != nil {
		return LoginResult{}, err
	}
	if attempts > maxAttempts {
		_ = s.store.DeleteChallenge(ctx, hash) // It is spent either way.
		return LoginResult{}, ErrChallengeInvalid
	}
	if err := s.verify(ctx, user.ID, code); err != nil {
		return LoginResult{}, err
	}
	if err := s.store.DeleteChallenge(ctx, hash); err != nil {
		return LoginResult{}, err
	}
	return s.startSession(ctx, user)
}

// verify checks an authenticator or recovery code of a user who has
// two-step verification on.
func (s *Service) verify(ctx context.Context, user resource.ID, code string) error {
	st, err := s.store.TwoFactor(ctx, user)
	if err != nil {
		return err
	}
	if st.EnabledAt == nil {
		return ErrTwoFactorOff
	}
	code = normalizeCode(code)
	if len(code) == totpDigits {
		secret, err := s.openSecret(st.Secret)
		if err != nil {
			return err
		}
		step, ok := matchTOTP(secret, code, s.now())
		if !ok {
			return ErrInvalidCode
		}
		fresh, err := s.store.UseTOTPStep(ctx, user, step)
		if err != nil {
			return err
		}
		if !fresh {
			return ErrInvalidCode
		}
		return nil
	}
	used, err := s.store.UseRecoveryCode(ctx, user, hashSecret(code), s.now())
	if err != nil {
		return err
	}
	if !used {
		return ErrInvalidCode
	}
	return nil
}

// signIn starts a session, or a challenge when the user has two-step
// verification on.
func (s *Service) signIn(ctx context.Context, user User) (LoginResult, error) {
	on, err := s.TwoFactorEnabled(ctx, user.ID)
	if err != nil {
		return LoginResult{}, fmt.Errorf("two-step verification: %w", err)
	}
	if !on {
		return s.startSession(ctx, user)
	}
	secret := randomSecret()
	if err := s.store.InsertChallenge(ctx, hashSecret(secret), user.ID, s.now().Add(challengeLifetime)); err != nil {
		return LoginResult{}, fmt.Errorf("insert challenge: %w", err)
	}
	return LoginResult{User: user, Challenge: secret}, nil
}

func (s *Service) openSecret(sealed string) ([]byte, error) {
	encoded, err := s.sealer.Open(sealed)
	if err != nil {
		return nil, fmt.Errorf("open secret: %w", err)
	}
	return base32NoPad.DecodeString(encoded)
}

// normalizeCode drops spaces and dashes people type in codes.
func normalizeCode(code string) string {
	return strings.ToLower(strings.NewReplacer(" ", "", "-", "").Replace(strings.TrimSpace(code)))
}

// newRecoveryCodes makes codes like 4f7k-2m9q-x8c3 and their hashes.
func newRecoveryCodes() (codes, hashes []string) {
	const alphabet = "abcdefghjkmnpqrstuvwxyz23456789"
	for range recoveryCodes {
		b := make([]byte, 12)
		if _, err := rand.Read(b); err != nil {
			panic(fmt.Sprintf("read random: %v", err))
		}
		var sb strings.Builder
		for i, v := range b {
			if i > 0 && i%4 == 0 {
				sb.WriteByte('-')
			}
			sb.WriteByte(alphabet[int(v)%len(alphabet)])
		}
		codes = append(codes, sb.String())
		hashes = append(hashes, hashSecret(normalizeCode(sb.String())))
	}
	return codes, hashes
}
