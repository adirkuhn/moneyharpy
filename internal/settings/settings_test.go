package settings

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/adirkuhn/moneyharpy/internal/resource"
	"github.com/google/go-cmp/cmp"
)

type memStore struct{ values map[string][]byte }

func (s *memStore) Load(_ context.Context, key string) ([]byte, error) {
	v, ok := s.values[key]
	if !ok {
		return nil, ErrNotSet
	}
	return v, nil
}

func (s *memStore) Save(_ context.Context, key string, value []byte) error {
	s.values[key] = value
	return nil
}

func newService(t *testing.T) (*Service, *memStore) {
	t.Helper()
	store := &memStore{values: map[string][]byte{}}
	return NewService(store, NewCipher("test-secret-key")), store
}

func ptr[T any](v T) *T { return &v }

func TestInstanceDefaults(t *testing.T) {
	s, _ := newService(t)
	got, err := s.Instance(context.Background())
	if err != nil {
		t.Fatalf("Instance() error = %v", err)
	}
	if diff := cmp.Diff(Instance{RegistrationOpen: true}, got); diff != "" {
		t.Errorf("Instance() diff (-want +got):\n%s", diff)
	}
}

func TestUpdatePersistsAndEncryptsSecret(t *testing.T) {
	s, store := newService(t)
	ctx := context.Background()
	const secret = "google-client-secret"

	_, err := s.Update(ctx, Update{
		RegistrationOpen:   ptr(false),
		GoogleEnabled:      ptr(true),
		GoogleClientID:     ptr("client-id"),
		GoogleClientSecret: ptr(secret),
	})
	if err != nil {
		t.Fatalf("Update() error = %v", err)
	}

	for key, raw := range store.values {
		if strings.Contains(string(raw), secret) {
			t.Errorf("stored value for %q contains the secret in clear", key)
		}
	}
	got, err := s.Instance(ctx)
	if err != nil {
		t.Fatalf("Instance() error = %v", err)
	}
	want := Instance{RegistrationOpen: false, Google: Google{Enabled: true, ClientID: "client-id", ClientSecret: secret}}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("Instance() diff (-want +got):\n%s", diff)
	}
}

func TestUpdateWithoutSecretKeepsExistingSecret(t *testing.T) {
	s, _ := newService(t)
	ctx := context.Background()
	if _, err := s.Update(ctx, Update{GoogleClientID: ptr("id"), GoogleClientSecret: ptr("keep-me")}); err != nil {
		t.Fatalf("Update() error = %v", err)
	}

	got, err := s.Update(ctx, Update{GoogleClientID: ptr("id-2")})
	if err != nil {
		t.Fatalf("Update() error = %v", err)
	}
	if got.Google.ClientSecret != "keep-me" {
		t.Errorf("Update() ClientSecret = %q, want %q", got.Google.ClientSecret, "keep-me")
	}
}

func TestEnablingGoogleRequiresCredentials(t *testing.T) {
	s, _ := newService(t)
	_, err := s.Update(context.Background(), Update{GoogleEnabled: ptr(true)})
	var verr *resource.ValidationError
	if !errors.As(err, &verr) {
		t.Fatalf("Update() error = %v, want *resource.ValidationError", err)
	}
	for _, field := range []string{"google_client_id", "google_client_secret"} {
		if _, ok := verr.Fields[field]; !ok {
			t.Errorf("Update() fields = %v, want key %q", verr.Fields, field)
		}
	}
}

func TestCipherRejectsWrongKey(t *testing.T) {
	sealed, err := NewCipher("key-a").Seal("value")
	if err != nil {
		t.Fatalf("Seal() error = %v", err)
	}
	if _, err := NewCipher("key-b").Open(sealed); err == nil {
		t.Error("Open() with wrong key error = nil, want error")
	}
	got, err := NewCipher("key-a").Open(sealed)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	if got != "value" {
		t.Errorf("Open() = %q, want %q", got, "value")
	}
}
