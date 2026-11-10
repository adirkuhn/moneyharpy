// Package identity registers users and authenticates them through passwords,
// sessions, API tokens and external login providers.
package identity

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"net/mail"
	"strings"
	"time"

	"github.com/adirkuhn/moneyharpy/internal/resource"
)

// Errors returned by Service and Store.
var (
	ErrInvalidCredentials = errors.New("invalid email or password")
	ErrUnauthenticated    = errors.New("not authenticated")
	ErrEmailTaken         = errors.New("email already registered")
	ErrRegistrationClosed = errors.New("registration is closed")
	ErrUserNotFound       = errors.New("user not found")
	ErrTokenNotFound      = errors.New("token not found")
	ErrResetTokenInvalid  = errors.New("reset token is invalid or expired")
)

const (
	sessionLifetime   = 30 * 24 * time.Hour
	resetLifetime     = time.Hour
	minPasswordLength = 12
	maxPasswordLength = 1024
	tokenPrefix       = "mh_"
)

// User is a person who can sign in.
type User struct {
	ID            resource.ID
	Email         string
	Name          string
	InstanceAdmin bool
}

// Token describes a personal API token. The secret itself is only available
// when the token is created.
type Token struct {
	ID        resource.ID
	UserID    resource.ID
	Name      string
	CreatedAt time.Time
}

// Registration is the input for creating a password account.
type Registration struct {
	Email    string
	Name     string
	Password string
}

// External is a user vouched for by an external login provider.
type External struct {
	Provider      string
	Subject       string
	Email         string
	EmailVerified bool
	Name          string
}

// LoginResult is a signed-in user and the secret of their new session.
type LoginResult struct {
	User         User
	SessionToken string
}

// Store persists users and their credentials. Token and session secrets reach
// it only as hashes.
type Store interface {
	// InsertUser stores u and returns it as saved. The first user ever
	// inserted becomes the instance admin, which must hold under concurrent
	// inserts. It returns ErrEmailTaken for a duplicate email.
	InsertUser(ctx context.Context, u User, passwordHash string) (User, error)
	UserByEmail(ctx context.Context, email string) (User, string, error)
	UserByIdentity(ctx context.Context, provider, subject string) (User, error)
	LinkIdentity(ctx context.Context, user resource.ID, provider, subject string) error

	InsertSession(ctx context.Context, hash string, user resource.ID, expires time.Time) error
	UserBySession(ctx context.Context, hash string, now time.Time) (User, error)
	DeleteSession(ctx context.Context, hash string) error

	InsertToken(ctx context.Context, hash string, t Token) error
	UserByToken(ctx context.Context, hash string) (User, error)
	ListTokens(ctx context.Context, user resource.ID) ([]Token, error)
	DeleteToken(ctx context.Context, id, user resource.ID) error

	// ReplaceResetToken stores a password reset token, invalidating the
	// user's earlier ones.
	ReplaceResetToken(ctx context.Context, hash string, user resource.ID, expires time.Time) error
	// ResetPassword spends a token, sets the password and ends the user's
	// sessions, all or nothing. It returns ErrResetTokenInvalid for a token
	// that is unknown, spent or expired.
	ResetPassword(ctx context.Context, hash, passwordHash string, now time.Time) error
}

// Mailer delivers account email.
type Mailer interface {
	SendPasswordReset(ctx context.Context, to, token, language string) error
}

// Policy answers instance-wide questions about who may join.
type Policy interface {
	RegistrationOpen(ctx context.Context) (bool, error)
}

// Service implements the identity use cases.
type Service struct {
	store  Store
	policy Policy
	ids    *resource.Generator
	hasher *Hasher
	mailer Mailer
	now    func() time.Time
	// decoy is verified for unknown emails so response time does not reveal
	// which addresses are registered.
	decoy string
}

// NewService returns a Service. It panics only if the system random source
// fails, which leaves no way to create safe credentials.
func NewService(store Store, policy Policy, ids *resource.Generator, hasher *Hasher, mailer Mailer, now func() time.Time) *Service {
	decoy, err := hasher.Hash(randomSecret())
	if err != nil {
		panic(fmt.Sprintf("hash decoy password: %v", err))
	}
	return &Service{store: store, policy: policy, ids: ids, hasher: hasher, mailer: mailer, now: now, decoy: decoy}
}

// Register creates a password account and signs it in.
func (s *Service) Register(ctx context.Context, in Registration) (LoginResult, error) {
	email := normalizeEmail(in.Email)
	name := strings.TrimSpace(in.Name)

	var verr resource.ValidationError
	if !validEmail(email) {
		verr.Add("email", "is not a valid email address")
	}
	if name == "" {
		verr.Add("name", "can't be blank")
	}
	checkPassword(in.Password, &verr)
	if err := verr.OrNil(); err != nil {
		return LoginResult{}, err
	}
	if err := s.requireOpen(ctx); err != nil {
		return LoginResult{}, err
	}

	hash, err := s.hasher.Hash(in.Password)
	if err != nil {
		return LoginResult{}, fmt.Errorf("hash password: %w", err)
	}
	user, err := s.store.InsertUser(ctx, User{ID: s.ids.Next(), Email: email, Name: name}, hash)
	if err != nil {
		return LoginResult{}, fmt.Errorf("insert user: %w", err)
	}
	return s.startSession(ctx, user)
}

// Login signs in with an email and password.
func (s *Service) Login(ctx context.Context, email, password string) (LoginResult, error) {
	user, hash, err := s.store.UserByEmail(ctx, normalizeEmail(email))
	switch {
	case errors.Is(err, ErrUserNotFound):
		s.hasher.Verify(s.decoy, password)
		return LoginResult{}, ErrInvalidCredentials
	case err != nil:
		return LoginResult{}, fmt.Errorf("find user: %w", err)
	}
	if !s.hasher.Verify(hash, password) {
		return LoginResult{}, ErrInvalidCredentials
	}
	return s.startSession(ctx, user)
}

// LoginExternal signs in a user vouched for by an external provider, creating
// or linking an account when needed. An existing account is only linked through
// a verified email, otherwise anyone could claim it by asserting its address.
func (s *Service) LoginExternal(ctx context.Context, ext External) (LoginResult, error) {
	user, err := s.store.UserByIdentity(ctx, ext.Provider, ext.Subject)
	if err == nil {
		return s.startSession(ctx, user)
	}
	if !errors.Is(err, ErrUserNotFound) {
		return LoginResult{}, fmt.Errorf("find identity: %w", err)
	}

	email := normalizeEmail(ext.Email)
	user, _, err = s.store.UserByEmail(ctx, email)
	switch {
	case err == nil:
		if !ext.EmailVerified {
			return LoginResult{}, ErrEmailTaken
		}
	case errors.Is(err, ErrUserNotFound):
		if err := s.requireOpen(ctx); err != nil {
			return LoginResult{}, err
		}
		user, err = s.store.InsertUser(ctx, User{ID: s.ids.Next(), Email: email, Name: ext.Name}, "")
		if err != nil {
			return LoginResult{}, fmt.Errorf("insert user: %w", err)
		}
	default:
		return LoginResult{}, fmt.Errorf("find user: %w", err)
	}

	if err := s.store.LinkIdentity(ctx, user.ID, ext.Provider, ext.Subject); err != nil {
		return LoginResult{}, fmt.Errorf("link identity: %w", err)
	}
	return s.startSession(ctx, user)
}

// RequestPasswordReset emails a reset link if the address has an account. An
// unknown address is not an error, so callers cannot discover accounts.
func (s *Service) RequestPasswordReset(ctx context.Context, email, language string) error {
	user, _, err := s.store.UserByEmail(ctx, normalizeEmail(email))
	if errors.Is(err, ErrUserNotFound) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("find user: %w", err)
	}
	token := randomSecret()
	if err := s.store.ReplaceResetToken(ctx, hashSecret(token), user.ID, s.now().Add(resetLifetime)); err != nil {
		return fmt.Errorf("store reset token: %w", err)
	}
	if err := s.mailer.SendPasswordReset(ctx, user.Email, token, language); err != nil {
		return fmt.Errorf("send reset mail: %w", err)
	}
	return nil
}

// ResetPassword sets a new password using an emailed token.
func (s *Service) ResetPassword(ctx context.Context, token, password string) error {
	var verr resource.ValidationError
	checkPassword(password, &verr)
	if err := verr.OrNil(); err != nil {
		return err
	}
	hash, err := s.hasher.Hash(password)
	if err != nil {
		return fmt.Errorf("hash password: %w", err)
	}
	return s.store.ResetPassword(ctx, hashSecret(token), hash, s.now())
}

// Authenticate returns the user owning a session secret.
func (s *Service) Authenticate(ctx context.Context, sessionToken string) (User, error) {
	user, err := s.store.UserBySession(ctx, hashSecret(sessionToken), s.now())
	return authenticated(user, err)
}

// Logout ends a session.
func (s *Service) Logout(ctx context.Context, sessionToken string) error {
	return s.store.DeleteSession(ctx, hashSecret(sessionToken))
}

// CreateToken issues a personal API token and returns its secret, which cannot
// be retrieved again.
func (s *Service) CreateToken(ctx context.Context, user resource.ID, name string) (Token, string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		var verr resource.ValidationError
		verr.Add("name", "can't be blank")
		return Token{}, "", &verr
	}
	secret := tokenPrefix + randomSecret()
	t := Token{ID: s.ids.Next(), UserID: user, Name: name, CreatedAt: s.now()}
	if err := s.store.InsertToken(ctx, hashSecret(secret), t); err != nil {
		return Token{}, "", fmt.Errorf("insert token: %w", err)
	}
	return t, secret, nil
}

// AuthenticateToken returns the user owning an API token secret.
func (s *Service) AuthenticateToken(ctx context.Context, secret string) (User, error) {
	user, err := s.store.UserByToken(ctx, hashSecret(secret))
	return authenticated(user, err)
}

// Tokens lists a user's API tokens.
func (s *Service) Tokens(ctx context.Context, user resource.ID) ([]Token, error) {
	return s.store.ListTokens(ctx, user)
}

// DeleteToken revokes one of a user's API tokens.
func (s *Service) DeleteToken(ctx context.Context, id, user resource.ID) error {
	return s.store.DeleteToken(ctx, id, user)
}

func (s *Service) requireOpen(ctx context.Context) error {
	open, err := s.policy.RegistrationOpen(ctx)
	if err != nil {
		return fmt.Errorf("read registration policy: %w", err)
	}
	if !open {
		return ErrRegistrationClosed
	}
	return nil
}

func (s *Service) startSession(ctx context.Context, user User) (LoginResult, error) {
	secret := randomSecret()
	if err := s.store.InsertSession(ctx, hashSecret(secret), user.ID, s.now().Add(sessionLifetime)); err != nil {
		return LoginResult{}, fmt.Errorf("insert session: %w", err)
	}
	return LoginResult{User: user, SessionToken: secret}, nil
}

func authenticated(user User, err error) (User, error) {
	if errors.Is(err, ErrUserNotFound) {
		return User{}, ErrUnauthenticated
	}
	if err != nil {
		return User{}, fmt.Errorf("find user: %w", err)
	}
	return user, nil
}

func checkPassword(password string, verr *resource.ValidationError) {
	if n := len(password); n < minPasswordLength || n > maxPasswordLength {
		verr.Add("password", fmt.Sprintf("must be between %d and %d characters", minPasswordLength, maxPasswordLength))
	}
}

func normalizeEmail(email string) string { return strings.ToLower(strings.TrimSpace(email)) }

func validEmail(email string) bool {
	addr, err := mail.ParseAddress(email)
	return err == nil && addr.Address == email
}

func randomSecret() string {
	b := make([]byte, 32)
	rand.Read(b) // Never fails: crypto/rand crashes the program instead.
	return base64.RawURLEncoding.EncodeToString(b)
}

func hashSecret(secret string) string {
	sum := sha256.Sum256([]byte(secret))
	return hex.EncodeToString(sum[:])
}
