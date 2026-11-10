package connections

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/adirkuhn/moneyharpy/internal/administration"
	"github.com/adirkuhn/moneyharpy/internal/resource"
)

const (
	adminID                 resource.ID = 100
	owner, employee, nobody resource.ID = 1, 2, 3
)

var now = time.Date(2026, 11, 10, 9, 0, 0, 0, time.UTC)

type fakeAdmins struct{}

func (fakeAdmins) Get(_ context.Context, id, user resource.ID) (administration.Listed, error) {
	if id != adminID {
		return administration.Listed{}, administration.ErrNotFound
	}
	switch user {
	case owner:
		return administration.Listed{Membership: administration.Membership{UserType: administration.Owner, Permissions: administration.Permissions}}, nil
	case employee:
		return administration.Listed{Membership: administration.Membership{UserType: administration.Employee, Permissions: []administration.Permission{administration.Bank}}}, nil
	}
	return administration.Listed{}, administration.ErrNotFound
}

type memStore struct {
	conns  map[resource.ID]Connection
	hashes map[string]resource.ID
	open   bool
}

func (m *memStore) Connections(context.Context, resource.ID) ([]Connection, error) {
	var out []Connection
	for _, c := range m.conns {
		out = append(out, c)
	}
	return out, nil
}
func (m *memStore) Connection(_ context.Context, _, id resource.ID) (Connection, error) {
	c, ok := m.conns[id]
	if !ok {
		return Connection{}, ErrNotFound
	}
	return c, nil
}
func (m *memStore) CreateConnection(_ context.Context, c Connection, hash string) error {
	m.conns[c.ID], m.hashes[hash] = c, c.ID
	return nil
}
func (m *memStore) DeleteConnection(_ context.Context, _, id resource.ID) error {
	delete(m.conns, id)
	return nil
}
func (m *memStore) GrantByToken(_ context.Context, hash string, at time.Time) (Grant, error) {
	id, ok := m.hashes[hash]
	c, live := m.conns[id]
	if !ok || !live {
		return Grant{}, ErrInvalidToken
	}
	c.LastUsedAt = &at
	m.conns[id] = c
	return Grant{Connection: c}, nil
}
func (m *memStore) UsersMayConnect(context.Context, resource.ID) (bool, error) { return m.open, nil }
func (m *memStore) SetUsersMayConnect(_ context.Context, _ resource.ID, on bool) error {
	m.open = on
	return nil
}

func newTestService() (*Service, *memStore) {
	store := &memStore{conns: map[resource.ID]Connection{}, hashes: map[string]resource.ID{}}
	return NewService(fakeAdmins{}, store, resource.NewGenerator(1), func() time.Time { return now }), store
}

func fieldsOf(t *testing.T, err error) map[string][]string {
	t.Helper()
	var verr *resource.ValidationError
	if !errors.As(err, &verr) {
		t.Fatalf("error = %v, want *resource.ValidationError", err)
	}
	return verr.Fields
}

func TestCreateAndAuthenticate(t *testing.T) {
	svc, store := newTestService()
	ctx := context.Background()
	c, token, err := svc.Create(ctx, adminID, owner, NewConnection{Kind: KindAPI, Name: " Shop ", Scopes: []administration.Permission{administration.Settings, administration.Bank}, ReadOnly: true})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(token, TokenPrefix) || c.Name != "Shop" || !slices.Equal(c.Scopes, []administration.Permission{administration.Bank, administration.Settings}) {
		t.Errorf("Create() = %+v, %q", c, token)
	}
	g, err := svc.Authenticate(ctx, token)
	if err != nil || g.ID != c.ID || !g.ReadOnly || !g.Allows(administration.Bank) || g.Allows(administration.SalesInvoices) || !g.Allows() {
		t.Errorf("Authenticate() = %+v, %v", g, err)
	}
	if store.conns[c.ID].LastUsedAt == nil {
		t.Error("use not recorded")
	}
	for _, bad := range []string{"mh_x", "mhc_unknown"} {
		if _, err := svc.Authenticate(ctx, bad); !errors.Is(err, ErrInvalidToken) {
			t.Errorf("Authenticate(%q) error = %v", bad, err)
		}
	}
}

func TestOnlyOwnersConnectUnlessAllowed(t *testing.T) {
	svc, _ := newTestService()
	ctx := context.Background()
	in := NewConnection{Kind: KindAI, Name: "Assistant", Scopes: []administration.Permission{administration.Bank}}
	if _, _, err := svc.Create(ctx, adminID, employee, in); !errors.Is(err, ErrForbidden) {
		t.Errorf("employee error = %v", err)
	}
	if err := svc.SetUsersMayConnect(ctx, adminID, employee, true); !errors.Is(err, ErrForbidden) {
		t.Errorf("employee opening error = %v", err)
	}
	if err := svc.SetUsersMayConnect(ctx, adminID, owner, true); err != nil {
		t.Fatal(err)
	}
	c, _, err := svc.Create(ctx, adminID, employee, in)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := svc.Create(ctx, adminID, employee, NewConnection{Kind: KindAPI, Name: "x", Scopes: []administration.Permission{administration.Settings}}); fieldsOf(t, err)["scopes"] == nil {
		t.Error("employee gave more than they may do")
	}
	if err := svc.Revoke(ctx, adminID, c.ID, employee); err != nil {
		t.Errorf("revoke own = %v", err)
	}
}

func TestValidationAndRevoke(t *testing.T) {
	svc, _ := newTestService()
	ctx := context.Background()
	_, _, err := svc.Create(ctx, adminID, owner, NewConnection{Kind: "robot", Scopes: []administration.Permission{administration.Ownership}})
	fields := fieldsOf(t, err)
	for _, f := range []string{"name", "kind", "scopes"} {
		if fields[f] == nil {
			t.Errorf("fields = %v, want %s", fields, f)
		}
	}
	c, token, _ := svc.Create(ctx, adminID, owner, NewConnection{Kind: KindAPI, Name: "Shop", Scopes: []administration.Permission{administration.Bank}})
	if err := svc.Revoke(ctx, adminID, c.ID, employee); !errors.Is(err, ErrForbidden) {
		t.Errorf("employee revoking the owner's error = %v", err)
	}
	if err := svc.Revoke(ctx, adminID, c.ID, owner); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Authenticate(ctx, token); !errors.Is(err, ErrInvalidToken) {
		t.Errorf("revoked token error = %v", err)
	}
	if _, err := svc.Connections(ctx, adminID, nobody); !errors.Is(err, administration.ErrNotFound) {
		t.Errorf("stranger error = %v", err)
	}
}
