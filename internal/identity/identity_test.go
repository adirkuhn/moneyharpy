package identity

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/adirkuhn/moneyharpy/internal/resource"
	"github.com/google/go-cmp/cmp"
)

// memStore is an in-memory Store used to exercise use cases without a database.
type memStore struct {
	users      map[resource.ID]User
	hashes     map[resource.ID]string
	identities map[string]resource.ID
	sessions   map[string]session
	tokens     map[string]Token
	resets     map[string]session
	tfa        map[resource.ID]*tfaState
	challenges map[string]*challenge
	required   map[resource.ID]bool
}

type tfaState struct {
	secret   string
	enabled  *time.Time
	lastStep int64
	codes    map[string]bool
}

type challenge struct {
	user     resource.ID
	attempts int
	expires  time.Time
}

type session struct {
	user    resource.ID
	expires time.Time
}

func newMemStore() *memStore {
	return &memStore{
		users:      map[resource.ID]User{},
		hashes:     map[resource.ID]string{},
		identities: map[string]resource.ID{},
		sessions:   map[string]session{},
		tokens:     map[string]Token{},
		resets:     map[string]session{},
		tfa:        map[resource.ID]*tfaState{},
		challenges: map[string]*challenge{},
		required:   map[resource.ID]bool{},
	}
}

func (s *memStore) InsertUser(_ context.Context, u User, passwordHash string) (User, error) {
	for _, existing := range s.users {
		if existing.Email == u.Email {
			return User{}, ErrEmailTaken
		}
	}
	u.InstanceAdmin = len(s.users) == 0
	s.users[u.ID] = u
	s.hashes[u.ID] = passwordHash
	return u, nil
}

func (s *memStore) UserByEmail(_ context.Context, email string) (User, string, error) {
	for id, u := range s.users {
		if u.Email == email {
			return u, s.hashes[id], nil
		}
	}
	return User{}, "", ErrUserNotFound
}

func (s *memStore) UserByIdentity(_ context.Context, provider, subject string) (User, error) {
	id, ok := s.identities[provider+"/"+subject]
	if !ok {
		return User{}, ErrUserNotFound
	}
	return s.users[id], nil
}

func (s *memStore) LinkIdentity(_ context.Context, user resource.ID, provider, subject string) error {
	s.identities[provider+"/"+subject] = user
	return nil
}

func (s *memStore) InsertSession(_ context.Context, hash string, user resource.ID, expires time.Time) error {
	s.sessions[hash] = session{user: user, expires: expires}
	return nil
}

func (s *memStore) UserBySession(_ context.Context, hash string, now time.Time) (User, error) {
	sess, ok := s.sessions[hash]
	if !ok || !sess.expires.After(now) {
		return User{}, ErrUserNotFound
	}
	return s.users[sess.user], nil
}

func (s *memStore) DeleteSession(_ context.Context, hash string) error {
	delete(s.sessions, hash)
	return nil
}

func (s *memStore) InsertToken(_ context.Context, hash string, t Token) error {
	s.tokens[hash] = t
	return nil
}

func (s *memStore) UserByToken(_ context.Context, hash string) (User, error) {
	t, ok := s.tokens[hash]
	if !ok {
		return User{}, ErrUserNotFound
	}
	return s.users[t.UserID], nil
}

func (s *memStore) ListTokens(_ context.Context, user resource.ID) ([]Token, error) {
	var out []Token
	for _, t := range s.tokens {
		if t.UserID == user {
			out = append(out, t)
		}
	}
	return out, nil
}

func (s *memStore) DeleteToken(_ context.Context, id, user resource.ID) error {
	for hash, t := range s.tokens {
		if t.ID == id && t.UserID == user {
			delete(s.tokens, hash)
			return nil
		}
	}
	return ErrTokenNotFound
}

func (s *memStore) ReplaceResetToken(_ context.Context, hash string, user resource.ID, expires time.Time) error {
	for k, r := range s.resets {
		if r.user == user {
			delete(s.resets, k)
		}
	}
	s.resets[hash] = session{user: user, expires: expires}
	return nil
}

func (s *memStore) ResetPassword(_ context.Context, hash, passwordHash string, now time.Time) error {
	r, ok := s.resets[hash]
	if !ok || !r.expires.After(now) {
		return ErrResetTokenInvalid
	}
	delete(s.resets, hash)
	s.hashes[r.user] = passwordHash
	for k, sess := range s.sessions {
		if sess.user == r.user {
			delete(s.sessions, k)
		}
	}
	return nil
}

type sentReset struct{ to, token, language string }

type fakeMailer struct {
	sent []sentReset
	err  error
}

func (m *fakeMailer) SendPasswordReset(_ context.Context, to, token, language string) error {
	m.sent = append(m.sent, sentReset{to, token, language})
	return m.err
}

type policy struct{ open bool }

func (p policy) RegistrationOpen(context.Context) (bool, error) { return p.open, nil }

type harness struct {
	*Service
	store  *memStore
	mailer *fakeMailer
	now    time.Time
}

func newHarness(open bool) *harness {
	h := &harness{store: newMemStore(), mailer: &fakeMailer{}, now: time.Date(2026, 11, 10, 12, 0, 0, 0, time.UTC)}
	h.Service = NewService(h.store, policy{open: open}, resource.NewGenerator(1), NewHasher(Params{Time: 1, MemoryKiB: 8, Threads: 1}), h.mailer, plainSealer{}, func() time.Time { return h.now })
	return h
}

var alice = Registration{Email: "Alice@Example.com ", Name: "Alice", Password: "correct horse battery"}

func TestFirstUserBecomesInstanceAdmin(t *testing.T) {
	h := newHarness(true)
	ctx := context.Background()

	first, err := h.Register(ctx, alice)
	if err != nil {
		t.Fatalf("Register() error = %v", err)
	}
	if !first.User.InstanceAdmin {
		t.Error("first Register() InstanceAdmin = false, want true")
	}
	if got, want := first.User.Email, "alice@example.com"; got != want {
		t.Errorf("Register() Email = %q, want %q", got, want)
	}

	second, err := h.Register(ctx, Registration{Email: "bob@example.com", Name: "Bob", Password: alice.Password})
	if err != nil {
		t.Fatalf("Register() error = %v", err)
	}
	if second.User.InstanceAdmin {
		t.Error("second Register() InstanceAdmin = true, want false")
	}
}

func TestRegisterRejections(t *testing.T) {
	tests := []struct {
		name      string
		in        Registration
		wantField string
	}{
		{name: "bad email", in: Registration{Email: "nope", Name: "A", Password: alice.Password}, wantField: "email"},
		{name: "blank name", in: Registration{Email: "a@b.nl", Name: " ", Password: alice.Password}, wantField: "name"},
		{name: "short password", in: Registration{Email: "a@b.nl", Name: "A", Password: "short"}, wantField: "password"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := newHarness(true).Register(context.Background(), tt.in)
			var verr *resource.ValidationError
			if !errors.As(err, &verr) {
				t.Fatalf("Register(%+v) error = %v, want *resource.ValidationError", tt.in, err)
			}
			if _, ok := verr.Fields[tt.wantField]; !ok {
				t.Errorf("Register(%+v) fields = %v, want key %q", tt.in, verr.Fields, tt.wantField)
			}
		})
	}
}

func TestRegisterWhenClosed(t *testing.T) {
	_, err := newHarness(false).Register(context.Background(), alice)
	if !errors.Is(err, ErrRegistrationClosed) {
		t.Errorf("Register() error = %v, want ErrRegistrationClosed", err)
	}
}

func TestPasswordIsNeverStoredInClear(t *testing.T) {
	h := newHarness(true)
	res, err := h.Register(context.Background(), alice)
	if err != nil {
		t.Fatalf("Register() error = %v", err)
	}
	if got := h.store.hashes[res.User.ID]; got == "" || got == alice.Password {
		t.Errorf("stored password hash = %q, want a non-empty hash", got)
	}
}

func TestLogin(t *testing.T) {
	h := newHarness(true)
	ctx := context.Background()
	if _, err := h.Register(ctx, alice); err != nil {
		t.Fatalf("Register() error = %v", err)
	}

	res, err := h.Login(ctx, " ALICE@example.com", alice.Password)
	if err != nil {
		t.Fatalf("Login() error = %v", err)
	}
	got, err := h.Authenticate(ctx, res.SessionToken)
	if err != nil {
		t.Fatalf("Authenticate() error = %v", err)
	}
	if got.ID != res.User.ID {
		t.Errorf("Authenticate() ID = %d, want %d", got.ID, res.User.ID)
	}

	for name, attempt := range map[string][2]string{
		"wrong password": {"alice@example.com", "wrong password here"},
		"unknown email":  {"nobody@example.com", alice.Password},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := h.Login(ctx, attempt[0], attempt[1]); !errors.Is(err, ErrInvalidCredentials) {
				t.Errorf("Login(%q) error = %v, want ErrInvalidCredentials", attempt[0], err)
			}
		})
	}
}

func TestSessionExpiresAndLogsOut(t *testing.T) {
	h := newHarness(true)
	ctx := context.Background()
	res, err := h.Register(ctx, alice)
	if err != nil {
		t.Fatalf("Register() error = %v", err)
	}

	if err := h.Logout(ctx, res.SessionToken); err != nil {
		t.Fatalf("Logout() error = %v", err)
	}
	if _, err := h.Authenticate(ctx, res.SessionToken); !errors.Is(err, ErrUnauthenticated) {
		t.Errorf("Authenticate() after logout error = %v, want ErrUnauthenticated", err)
	}

	res, err = h.Login(ctx, alice.Email, alice.Password)
	if err != nil {
		t.Fatalf("Login() error = %v", err)
	}
	h.now = h.now.Add(sessionLifetime + time.Minute)
	if _, err := h.Authenticate(ctx, res.SessionToken); !errors.Is(err, ErrUnauthenticated) {
		t.Errorf("Authenticate() after expiry error = %v, want ErrUnauthenticated", err)
	}
}

func TestAPITokenLifecycle(t *testing.T) {
	h := newHarness(true)
	ctx := context.Background()
	res, err := h.Register(ctx, alice)
	if err != nil {
		t.Fatalf("Register() error = %v", err)
	}

	created, secret, err := h.CreateToken(ctx, res.User.ID, "ci")
	if err != nil {
		t.Fatalf("CreateToken() error = %v", err)
	}
	for hash := range h.store.tokens {
		if hash == secret {
			t.Error("token stored in clear, want hashed")
		}
	}
	got, err := h.AuthenticateToken(ctx, secret)
	if err != nil {
		t.Fatalf("AuthenticateToken() error = %v", err)
	}
	if got.ID != res.User.ID {
		t.Errorf("AuthenticateToken() ID = %d, want %d", got.ID, res.User.ID)
	}

	if err := h.DeleteToken(ctx, created.ID, 999); !errors.Is(err, ErrTokenNotFound) {
		t.Errorf("DeleteToken() by other user error = %v, want ErrTokenNotFound", err)
	}
	if err := h.DeleteToken(ctx, created.ID, res.User.ID); err != nil {
		t.Fatalf("DeleteToken() error = %v", err)
	}
	if _, err := h.AuthenticateToken(ctx, secret); !errors.Is(err, ErrUnauthenticated) {
		t.Errorf("AuthenticateToken() after delete error = %v, want ErrUnauthenticated", err)
	}
}

func TestLoginExternal(t *testing.T) {
	ctx := context.Background()
	google := External{Provider: "google", Subject: "g-1", Email: "alice@example.com", EmailVerified: true, Name: "Alice"}

	t.Run("creates a passwordless user", func(t *testing.T) {
		h := newHarness(true)
		res, err := h.LoginExternal(ctx, google)
		if err != nil {
			t.Fatalf("LoginExternal() error = %v", err)
		}
		if diff := cmp.Diff("alice@example.com", res.User.Email); diff != "" {
			t.Errorf("LoginExternal() email diff (-want +got):\n%s", diff)
		}
		if _, err := h.Login(ctx, google.Email, ""); !errors.Is(err, ErrInvalidCredentials) {
			t.Errorf("Login() with empty password error = %v, want ErrInvalidCredentials", err)
		}
	})

	t.Run("returns same user on repeat login", func(t *testing.T) {
		h := newHarness(true)
		first, err := h.LoginExternal(ctx, google)
		if err != nil {
			t.Fatalf("LoginExternal() error = %v", err)
		}
		second, err := h.LoginExternal(ctx, google)
		if err != nil {
			t.Fatalf("LoginExternal() error = %v", err)
		}
		if first.User.ID != second.User.ID {
			t.Errorf("LoginExternal() ID = %d, want %d", second.User.ID, first.User.ID)
		}
	})

	t.Run("links verified email to existing account", func(t *testing.T) {
		h := newHarness(true)
		reg, err := h.Register(ctx, alice)
		if err != nil {
			t.Fatalf("Register() error = %v", err)
		}
		got, err := h.LoginExternal(ctx, google)
		if err != nil {
			t.Fatalf("LoginExternal() error = %v", err)
		}
		if got.User.ID != reg.User.ID {
			t.Errorf("LoginExternal() ID = %d, want %d", got.User.ID, reg.User.ID)
		}
	})

	t.Run("refuses unverified email matching existing account", func(t *testing.T) {
		h := newHarness(true)
		if _, err := h.Register(ctx, alice); err != nil {
			t.Fatalf("Register() error = %v", err)
		}
		unverified := google
		unverified.EmailVerified = false
		if _, err := h.LoginExternal(ctx, unverified); !errors.Is(err, ErrEmailTaken) {
			t.Errorf("LoginExternal() error = %v, want ErrEmailTaken", err)
		}
	})

	t.Run("refuses new user while registration is closed", func(t *testing.T) {
		if _, err := newHarness(false).LoginExternal(ctx, google); !errors.Is(err, ErrRegistrationClosed) {
			t.Errorf("LoginExternal() error = %v, want ErrRegistrationClosed", err)
		}
	})
}

func TestPasswordResetChangesPasswordAndEndsSessions(t *testing.T) {
	h := newHarness(true)
	ctx := context.Background()
	reg, err := h.Register(ctx, alice)
	if err != nil {
		t.Fatalf("Register() error = %v", err)
	}

	if err := h.RequestPasswordReset(ctx, " ALICE@example.com", "nl"); err != nil {
		t.Fatalf("RequestPasswordReset() error = %v", err)
	}
	if len(h.mailer.sent) != 1 {
		t.Fatalf("mails sent = %d, want 1", len(h.mailer.sent))
	}
	mail := h.mailer.sent[0]
	if mail.to != "alice@example.com" || mail.language != "nl" {
		t.Errorf("mail = %+v, want normalized address and language", mail)
	}
	for hash := range h.store.resets {
		if hash == mail.token {
			t.Error("reset token stored in clear, want hashed")
		}
	}

	const next = "a brand new password"
	if err := h.ResetPassword(ctx, mail.token, next); err != nil {
		t.Fatalf("ResetPassword() error = %v", err)
	}
	if _, err := h.Login(ctx, alice.Email, alice.Password); !errors.Is(err, ErrInvalidCredentials) {
		t.Errorf("Login() with old password error = %v, want ErrInvalidCredentials", err)
	}
	if _, err := h.Login(ctx, alice.Email, next); err != nil {
		t.Errorf("Login() with new password error = %v", err)
	}
	if _, err := h.Authenticate(ctx, reg.SessionToken); !errors.Is(err, ErrUnauthenticated) {
		t.Errorf("Authenticate() of old session error = %v, want ErrUnauthenticated", err)
	}
	if err := h.ResetPassword(ctx, mail.token, next); !errors.Is(err, ErrResetTokenInvalid) {
		t.Errorf("ResetPassword() reused token error = %v, want ErrResetTokenInvalid", err)
	}
}

func TestPasswordResetForUnknownEmailSendsNothing(t *testing.T) {
	h := newHarness(true)
	if err := h.RequestPasswordReset(context.Background(), "nobody@example.com", "en"); err != nil {
		t.Fatalf("RequestPasswordReset() error = %v", err)
	}
	if len(h.mailer.sent) != 0 {
		t.Errorf("mails sent = %d, want 0", len(h.mailer.sent))
	}
}

func TestPasswordResetTokenExpires(t *testing.T) {
	h := newHarness(true)
	ctx := context.Background()
	if _, err := h.Register(ctx, alice); err != nil {
		t.Fatalf("Register() error = %v", err)
	}
	if err := h.RequestPasswordReset(ctx, alice.Email, "en"); err != nil {
		t.Fatalf("RequestPasswordReset() error = %v", err)
	}
	h.now = h.now.Add(resetLifetime + time.Minute)
	if err := h.ResetPassword(ctx, h.mailer.sent[0].token, "a brand new password"); !errors.Is(err, ErrResetTokenInvalid) {
		t.Errorf("ResetPassword() error = %v, want ErrResetTokenInvalid", err)
	}
}

func TestNewPasswordResetReplacesEarlierToken(t *testing.T) {
	h := newHarness(true)
	ctx := context.Background()
	if _, err := h.Register(ctx, alice); err != nil {
		t.Fatalf("Register() error = %v", err)
	}
	for range 2 {
		if err := h.RequestPasswordReset(ctx, alice.Email, "en"); err != nil {
			t.Fatalf("RequestPasswordReset() error = %v", err)
		}
	}
	if err := h.ResetPassword(ctx, h.mailer.sent[0].token, "a brand new password"); !errors.Is(err, ErrResetTokenInvalid) {
		t.Errorf("ResetPassword() with first token error = %v, want ErrResetTokenInvalid", err)
	}
	if err := h.ResetPassword(ctx, h.mailer.sent[1].token, "a brand new password"); err != nil {
		t.Errorf("ResetPassword() with latest token error = %v", err)
	}
}

func TestResetPasswordValidatesNewPassword(t *testing.T) {
	err := newHarness(true).ResetPassword(context.Background(), "any", "short")
	var verr *resource.ValidationError
	if !errors.As(err, &verr) {
		t.Fatalf("ResetPassword() error = %v, want *resource.ValidationError", err)
	}
	if _, ok := verr.Fields["password"]; !ok {
		t.Errorf("ResetPassword() fields = %v, want password", verr.Fields)
	}
}
