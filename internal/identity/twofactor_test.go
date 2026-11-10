package identity

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/adirkuhn/moneyharpy/internal/resource"
)

// plainSealer keeps secrets readable in tests.
type plainSealer struct{}

func (plainSealer) Seal(s string) (string, error) { return "sealed:" + s, nil }
func (plainSealer) Open(s string) (string, error) {
	plain, ok := strings.CutPrefix(s, "sealed:")
	if !ok {
		return "", errors.New("not sealed")
	}
	return plain, nil
}

func (s *memStore) state(user resource.ID) *tfaState {
	if s.tfa[user] == nil {
		s.tfa[user] = &tfaState{codes: map[string]bool{}}
	}
	return s.tfa[user]
}

func (s *memStore) TwoFactor(_ context.Context, user resource.ID) (TwoFactorState, error) {
	st := s.state(user)
	left := 0
	for _, used := range st.codes {
		if !used {
			left++
		}
	}
	return TwoFactorState{Secret: st.secret, EnabledAt: st.enabled, RecoveryCodesLeft: left}, nil
}
func (s *memStore) SetTwoFactorSecret(_ context.Context, user resource.ID, sealed string) error {
	s.state(user).secret = sealed
	return nil
}
func (s *memStore) EnableTwoFactor(ctx context.Context, user resource.ID, at time.Time, step int64, hashes []string) error {
	st := s.state(user)
	st.enabled, st.lastStep = &at, step
	return s.ReplaceRecoveryCodes(ctx, user, hashes)
}
func (s *memStore) DisableTwoFactor(_ context.Context, user resource.ID) error {
	delete(s.tfa, user)
	return nil
}
func (s *memStore) ReplaceRecoveryCodes(_ context.Context, user resource.ID, hashes []string) error {
	st := s.state(user)
	st.codes = map[string]bool{}
	for _, h := range hashes {
		st.codes[h] = false
	}
	return nil
}
func (s *memStore) UseTOTPStep(_ context.Context, user resource.ID, step int64) (bool, error) {
	st := s.state(user)
	if step <= st.lastStep {
		return false, nil
	}
	st.lastStep = step
	return true, nil
}
func (s *memStore) UseRecoveryCode(_ context.Context, user resource.ID, hash string, _ time.Time) (bool, error) {
	st := s.state(user)
	used, ok := st.codes[hash]
	if !ok || used {
		return false, nil
	}
	st.codes[hash] = true
	return true, nil
}
func (s *memStore) RequiresTwoFactor(_ context.Context, user resource.ID) (bool, error) {
	return s.required[user], nil
}
func (s *memStore) InsertChallenge(_ context.Context, hash string, user resource.ID, expires time.Time) error {
	s.challenges[hash] = &challenge{user: user, expires: expires}
	return nil
}
func (s *memStore) ChallengeUser(_ context.Context, hash string, now time.Time) (User, int, error) {
	c, ok := s.challenges[hash]
	if !ok || !now.Before(c.expires) {
		return User{}, 0, ErrChallengeInvalid
	}
	c.attempts++
	return s.users[c.user], c.attempts, nil
}
func (s *memStore) DeleteChallenge(_ context.Context, hash string) error {
	delete(s.challenges, hash)
	return nil
}

// enroll turns two-step verification on for a new user and returns the
// secret and recovery codes.
func enroll(t *testing.T, h *harness) (User, []byte, []string) {
	t.Helper()
	ctx := context.Background()
	res, err := h.Register(ctx, alice)
	if err != nil {
		t.Fatal(err)
	}
	setup, err := h.StartTwoFactor(ctx, res.User)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(setup.URI, "otpauth://totp/MoneyHarpy:alice@example.com?") {
		t.Errorf("uri = %s", setup.URI)
	}
	secret, _ := base32NoPad.DecodeString(setup.Secret)
	if _, err := h.ConfirmTwoFactor(ctx, res.User.ID, "000000"); !errors.Is(err, ErrInvalidCode) && totpCode(secret, totpStep(h.now)) != "000000" {
		t.Errorf("wrong code error = %v", err)
	}
	codes, err := h.ConfirmTwoFactor(ctx, res.User.ID, totpCode(secret, totpStep(h.now)))
	if err != nil || len(codes) != recoveryCodes {
		t.Fatalf("ConfirmTwoFactor() = %v, %v", codes, err)
	}
	return res.User, secret, codes
}

func TestSignInAsksForTheSecondStep(t *testing.T) {
	h := newHarness(true)
	ctx := context.Background()
	user, secret, codes := enroll(t, h)

	res, err := h.Login(ctx, alice.Email, alice.Password)
	if err != nil || res.SessionToken != "" || res.Challenge == "" {
		t.Fatalf("Login() = %+v, %v", res, err)
	}
	// The code that confirmed the setup cannot be used again.
	if _, err := h.CompleteSignIn(ctx, res.Challenge, totpCode(secret, totpStep(h.now))); !errors.Is(err, ErrInvalidCode) {
		t.Errorf("replayed code error = %v", err)
	}
	h.now = h.now.Add(30 * time.Second)
	done, err := h.CompleteSignIn(ctx, res.Challenge, totpCode(secret, totpStep(h.now)))
	if err != nil || done.SessionToken == "" || done.User.ID != user.ID {
		t.Fatalf("CompleteSignIn() = %+v, %v", done, err)
	}
	if _, err := h.CompleteSignIn(ctx, res.Challenge, "123456"); !errors.Is(err, ErrChallengeInvalid) {
		t.Errorf("spent challenge error = %v", err)
	}

	res, _ = h.Login(ctx, alice.Email, alice.Password)
	if _, err := h.CompleteSignIn(ctx, res.Challenge, strings.ToUpper(codes[0])); err != nil {
		t.Errorf("recovery code: %v", err)
	}
	res, _ = h.Login(ctx, alice.Email, alice.Password)
	if _, err := h.CompleteSignIn(ctx, res.Challenge, codes[0]); !errors.Is(err, ErrInvalidCode) {
		t.Errorf("spent recovery code error = %v", err)
	}
	st, _, _ := h.TwoFactorStatus(ctx, user.ID)
	if st.RecoveryCodesLeft != recoveryCodes-1 || st.Secret != "" {
		t.Errorf("status = %+v", st)
	}
}

func TestChallengeLimitsAttempts(t *testing.T) {
	h := newHarness(true)
	ctx := context.Background()
	enroll(t, h)
	res, _ := h.Login(ctx, alice.Email, alice.Password)
	for range maxAttempts {
		if _, err := h.CompleteSignIn(ctx, res.Challenge, "aaaa-bbbb-cccc"); !errors.Is(err, ErrInvalidCode) {
			t.Fatalf("attempt error = %v", err)
		}
	}
	if _, err := h.CompleteSignIn(ctx, res.Challenge, "aaaa-bbbb-cccc"); !errors.Is(err, ErrChallengeInvalid) {
		t.Errorf("too many attempts error = %v", err)
	}
	res, _ = h.Login(ctx, alice.Email, alice.Password)
	h.now = h.now.Add(challengeLifetime + time.Second)
	if _, err := h.CompleteSignIn(ctx, res.Challenge, "aaaa-bbbb-cccc"); !errors.Is(err, ErrChallengeInvalid) {
		t.Errorf("expired challenge error = %v", err)
	}
}

func TestDisableTwoFactor(t *testing.T) {
	h := newHarness(true)
	ctx := context.Background()
	user, secret, codes := enroll(t, h)
	if _, err := h.StartTwoFactor(ctx, user); !errors.Is(err, ErrTwoFactorOn) {
		t.Errorf("start again error = %v", err)
	}
	h.store.required[user.ID] = true
	if err := h.DisableTwoFactor(ctx, user.ID, codes[1]); !errors.Is(err, ErrTwoFactorRequired) {
		t.Errorf("disable while required error = %v", err)
	}
	h.store.required[user.ID] = false
	fresh, err := h.NewRecoveryCodes(ctx, user.ID, codes[2])
	if err != nil || len(fresh) != recoveryCodes {
		t.Fatalf("NewRecoveryCodes() = %v, %v", fresh, err)
	}
	h.now = h.now.Add(time.Minute)
	if err := h.DisableTwoFactor(ctx, user.ID, totpCode(secret, totpStep(h.now))); err != nil {
		t.Fatal(err)
	}
	if on, _ := h.TwoFactorEnabled(ctx, user.ID); on {
		t.Error("still on")
	}
	if res, _ := h.Login(ctx, alice.Email, alice.Password); res.SessionToken == "" {
		t.Error("sign-in still asks for a code")
	}
}

func TestInvitedAddressRegistersWhileClosed(t *testing.T) {
	h := newHarness(false)
	in := alice
	if _, err := h.Register(context.Background(), in); !errors.Is(err, ErrRegistrationClosed) {
		t.Fatalf("closed error = %v", err)
	}
	in.Invited = true
	if _, err := h.Register(context.Background(), in); err != nil {
		t.Errorf("invited Register() = %v", err)
	}
}
