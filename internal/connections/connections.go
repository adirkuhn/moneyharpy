// Package connections lets users connect other software to one
// administration: API tokens for external applications and AI connections
// over MCP. A connection acts for the user who made it, limited to its
// scopes and, if chosen, to reading.
package connections

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/adirkuhn/moneyharpy/internal/administration"
	"github.com/adirkuhn/moneyharpy/internal/resource"
)

// TokenPrefix starts every connection token, so it can be told apart from
// a personal API token.
const TokenPrefix = "mhc_"

// Errors returned by Service.
var (
	ErrNotFound = errors.New("connection not found")
	// ErrForbidden reports that the user may not create or revoke this
	// connection.
	ErrForbidden = errors.New("not allowed")
	// ErrInvalidToken reports an unknown connection token.
	ErrInvalidToken = errors.New("invalid connection token")
)

// Kinds of connection.
const (
	KindAPI = "api"
	KindAI  = "ai"
)

// Scopes are Moneybird's API scopes. Each opens the permission of the same
// name; ownership is never part of a connection.
var Scopes = []administration.Permission{
	administration.SalesInvoices, administration.Documents, administration.Estimates,
	administration.Bank, administration.TimeEntries, administration.Settings,
}

// Connection is an API token or AI connection.
type Connection struct {
	ID               resource.ID
	AdministrationID resource.ID
	UserID           resource.ID
	// GrantedBy is the name of the user who made the connection.
	GrantedBy  string
	Kind       string
	Name       string
	Scopes     []administration.Permission
	ReadOnly   bool
	CreatedAt  time.Time
	LastUsedAt *time.Time
}

// Grant is what a connection token lets its bearer do.
type Grant struct {
	Connection
	// User is who the connection acts for.
	UserEmail, UserName string
}

// Allows reports whether the grant covers one of the permissions; an empty
// list asks only for the administration.
func (g Grant) Allows(perms ...administration.Permission) bool {
	if len(perms) == 0 {
		return true
	}
	for _, p := range perms {
		if slices.Contains(g.Scopes, p) {
			return true
		}
	}
	return false
}

// Store persists connections.
type Store interface {
	Connections(ctx context.Context, admin resource.ID) ([]Connection, error)
	Connection(ctx context.Context, admin, id resource.ID) (Connection, error)
	CreateConnection(ctx context.Context, c Connection, tokenHash string) error
	DeleteConnection(ctx context.Context, admin, id resource.ID) error
	// GrantByToken returns ErrInvalidToken for an unknown token and marks
	// the connection used.
	GrantByToken(ctx context.Context, tokenHash string, now time.Time) (Grant, error)
	UsersMayConnect(ctx context.Context, admin resource.ID) (bool, error)
	SetUsersMayConnect(ctx context.Context, admin resource.ID, on bool) error
}

// Administrations gives access to administrations.
type Administrations interface {
	Get(ctx context.Context, id, user resource.ID) (administration.Listed, error)
}

// Service implements the connection use cases.
type Service struct {
	admins Administrations
	store  Store
	ids    *resource.Generator
	now    func() time.Time
}

// NewService returns a Service.
func NewService(admins Administrations, store Store, ids *resource.Generator, now func() time.Time) *Service {
	return &Service{admins: admins, store: store, ids: ids, now: now}
}

// UsersMayConnect tells whether users other than owners may connect software.
func (s *Service) UsersMayConnect(ctx context.Context, admin, user resource.ID) (bool, error) {
	if _, err := s.admins.Get(ctx, admin, user); err != nil {
		return false, err
	}
	return s.store.UsersMayConnect(ctx, admin)
}

// SetUsersMayConnect lets users other than owners connect software, or not.
func (s *Service) SetUsersMayConnect(ctx context.Context, admin, user resource.ID, on bool) error {
	a, err := s.admins.Get(ctx, admin, user)
	if err != nil {
		return err
	}
	if !a.Can(administration.Ownership) {
		return ErrForbidden
	}
	return s.store.SetUsersMayConnect(ctx, admin, on)
}

// Connections lists the administration's connections, newest first.
func (s *Service) Connections(ctx context.Context, admin, user resource.ID) ([]Connection, error) {
	if _, err := s.admins.Get(ctx, admin, user); err != nil {
		return nil, err
	}
	list, err := s.store.Connections(ctx, admin)
	if err != nil {
		return nil, fmt.Errorf("list connections: %w", err)
	}
	slices.SortStableFunc(list, func(a, b Connection) int { return b.CreatedAt.Compare(a.CreatedAt) })
	return list, nil
}

// NewConnection is what to connect.
type NewConnection struct {
	Kind     string
	Name     string
	Scopes   []administration.Permission
	ReadOnly bool
}

// Create makes a connection and returns its token, shown only now. Owners
// may always connect; others when the administration lets users connect.
// Nobody can give a connection more than they may do themselves.
func (s *Service) Create(ctx context.Context, admin, user resource.ID, in NewConnection) (Connection, string, error) {
	a, err := s.admins.Get(ctx, admin, user)
	if err != nil {
		return Connection{}, "", err
	}
	if a.Access == administration.AccessSupport {
		return Connection{}, "", ErrForbidden
	}
	if !a.Can(administration.Ownership) {
		open, err := s.store.UsersMayConnect(ctx, admin)
		if err != nil {
			return Connection{}, "", err
		}
		if !open {
			return Connection{}, "", ErrForbidden
		}
	}
	var verr resource.ValidationError
	name := strings.TrimSpace(in.Name)
	if name == "" || len(name) > 255 {
		verr.Add("name", "can't be blank")
	}
	if in.Kind != KindAPI && in.Kind != KindAI {
		verr.Add("kind", "is not included in the list")
	}
	scopes := []administration.Permission{}
	for _, p := range Scopes {
		if slices.Contains(in.Scopes, p) {
			scopes = append(scopes, p)
		}
	}
	for _, p := range in.Scopes {
		switch {
		case !slices.Contains(Scopes, p):
			verr.Add("scopes", string(p)+" is not included in the list")
		case !a.Can(p):
			verr.Add("scopes", string(p)+" is more than you may do yourself")
		}
	}
	if len(scopes) == 0 {
		verr.Add("scopes", "can't be blank")
	}
	if err := verr.OrNil(); err != nil {
		return Connection{}, "", err
	}
	c := Connection{ID: s.ids.Next(), AdministrationID: admin, UserID: user, Kind: in.Kind, Name: name, Scopes: scopes, ReadOnly: in.ReadOnly, CreatedAt: s.now()}
	token := TokenPrefix + randomSecret()
	if err := s.store.CreateConnection(ctx, c, HashToken(token)); err != nil {
		return Connection{}, "", fmt.Errorf("store connection: %w", err)
	}
	return c, token, nil
}

// Revoke ends a connection. Owners revoke any; others their own. Webhooks
// the connection made go with it.
func (s *Service) Revoke(ctx context.Context, admin, id, user resource.ID) error {
	a, err := s.admins.Get(ctx, admin, user)
	if err != nil {
		return err
	}
	c, err := s.store.Connection(ctx, admin, id)
	if err != nil {
		return err
	}
	if c.UserID != user && !a.Can(administration.Ownership) {
		return ErrForbidden
	}
	return s.store.DeleteConnection(ctx, admin, id)
}

// Authenticate returns what a connection token may do. The user it acts for
// must still be able to open the administration.
func (s *Service) Authenticate(ctx context.Context, token string) (Grant, error) {
	if !strings.HasPrefix(token, TokenPrefix) {
		return Grant{}, ErrInvalidToken
	}
	return s.store.GrantByToken(ctx, HashToken(token), s.now())
}

// HashToken is how tokens are stored.
func HashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func randomSecret() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		panic(fmt.Sprintf("read random: %v", err))
	}
	return base64.RawURLEncoding.EncodeToString(b)
}
