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

func TestUpdateStoresLM2AndTrimsURL(t *testing.T) {
	s, _ := newService(t)
	ctx := context.Background()
	if _, err := s.Update(ctx, Update{LM2Enabled: ptr(true), LM2URL: ptr("https://lm.example.com/"), LM2App: ptr("moneyharpy")}); err != nil {
		t.Fatalf("Update() error = %v", err)
	}
	got, err := s.Instance(ctx)
	if err != nil {
		t.Fatalf("Instance() error = %v", err)
	}
	want := LM2{Enabled: true, URL: "https://lm.example.com", App: "moneyharpy"}
	if diff := cmp.Diff(want, got.LM2); diff != "" {
		t.Errorf("Instance().LM2 diff (-want +got):\n%s", diff)
	}
}

func TestLM2Validation(t *testing.T) {
	tests := []struct {
		name  string
		in    Update
		field string
	}{
		{name: "enabled without url", in: Update{LM2Enabled: ptr(true), LM2App: ptr("a")}, field: "lm2_url"},
		{name: "enabled without app", in: Update{LM2Enabled: ptr(true), LM2URL: ptr("https://x.nl")}, field: "lm2_app"},
		{name: "relative url", in: Update{LM2URL: ptr("x.nl")}, field: "lm2_url"},
		{name: "other scheme", in: Update{LM2URL: ptr("ftp://x.nl")}, field: "lm2_url"},
		{name: "url with query", in: Update{LM2URL: ptr("https://x.nl?a=1")}, field: "lm2_url"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s, _ := newService(t)
			_, err := s.Update(context.Background(), tt.in)
			var verr *resource.ValidationError
			if !errors.As(err, &verr) {
				t.Fatalf("Update() error = %v, want *resource.ValidationError", err)
			}
			if _, ok := verr.Fields[tt.field]; !ok {
				t.Errorf("Update() fields = %v, want key %q", verr.Fields, tt.field)
			}
		})
	}
}

func TestKVKKeyIsEncryptedAndKept(t *testing.T) {
	s, store := newService(t)
	ctx := context.Background()
	const key = "kvk-api-key-value"
	if _, err := s.Update(ctx, Update{KVKEnabled: ptr(true), KVKAPIKey: ptr(key), AddressLookupEnabled: ptr(true)}); err != nil {
		t.Fatalf("Update() error = %v", err)
	}
	for k, raw := range store.values {
		if strings.Contains(string(raw), key) {
			t.Errorf("stored value for %q contains the key in clear", k)
		}
	}
	got, err := s.Update(ctx, Update{KVKAPIKey: ptr("")})
	if err != nil {
		t.Fatalf("Update() error = %v", err)
	}
	if got.KVK.APIKey != key || !got.KVK.Enabled || !got.AddressLookupEnabled {
		t.Errorf("Update() = %+v, want key kept and flags on", got)
	}
}

func TestEnablingKVKRequiresKey(t *testing.T) {
	s, _ := newService(t)
	_, err := s.Update(context.Background(), Update{KVKEnabled: ptr(true)})
	var verr *resource.ValidationError
	if !errors.As(err, &verr) {
		t.Fatalf("Update() error = %v, want *resource.ValidationError", err)
	}
	if _, ok := verr.Fields["kvk_api_key"]; !ok {
		t.Errorf("Update() fields = %v, want kvk_api_key", verr.Fields)
	}
}

func validSMTP() Update {
	return Update{
		SMTPEnabled: ptr(true), SMTPHost: ptr("mail.example.com"), SMTPPort: ptr(587),
		SMTPUsername: ptr("user"), SMTPPassword: ptr("smtp-password-value"),
		SMTPFrom: ptr("books@example.com"), SMTPSecurity: ptr("starttls"),
	}
}

func TestSMTPPasswordIsEncryptedAndKept(t *testing.T) {
	s, store := newService(t)
	ctx := context.Background()
	if _, err := s.Update(ctx, validSMTP()); err != nil {
		t.Fatalf("Update() error = %v", err)
	}
	for key, raw := range store.values {
		if strings.Contains(string(raw), "smtp-password-value") {
			t.Errorf("stored value for %q contains the password in clear", key)
		}
	}
	got, err := s.Update(ctx, Update{SMTPPassword: ptr("")})
	if err != nil {
		t.Fatalf("Update() error = %v", err)
	}
	want := SMTP{Enabled: true, Host: "mail.example.com", Port: 587, Username: "user", Password: "smtp-password-value", From: "books@example.com", Security: "starttls"}
	if diff := cmp.Diff(want, got.SMTP); diff != "" {
		t.Errorf("Update() SMTP diff (-want +got):\n%s", diff)
	}
}

func TestSMTPValidation(t *testing.T) {
	with := func(change func(*Update)) Update {
		u := validSMTP()
		change(&u)
		return u
	}
	tests := []struct {
		name  string
		in    Update
		field string
	}{
		{name: "no host", in: with(func(u *Update) { u.SMTPHost = ptr("") }), field: "smtp_host"},
		{name: "host with line break", in: with(func(u *Update) { u.SMTPHost = ptr("a\r\nb") }), field: "smtp_host"},
		{name: "port too low", in: with(func(u *Update) { u.SMTPPort = ptr(0) }), field: "smtp_port"},
		{name: "port too high", in: with(func(u *Update) { u.SMTPPort = ptr(70000) }), field: "smtp_port"},
		{name: "bad from", in: with(func(u *Update) { u.SMTPFrom = ptr("nope") }), field: "smtp_from"},
		{name: "from with header injection", in: with(func(u *Update) { u.SMTPFrom = ptr("a@b.nl\r\nBcc: x@y.nl") }), field: "smtp_from"},
		{name: "unknown security", in: with(func(u *Update) { u.SMTPSecurity = ptr("ssl3") }), field: "smtp_security"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s, _ := newService(t)
			_, err := s.Update(context.Background(), tt.in)
			var verr *resource.ValidationError
			if !errors.As(err, &verr) {
				t.Fatalf("Update() error = %v, want *resource.ValidationError", err)
			}
			if _, ok := verr.Fields[tt.field]; !ok {
				t.Errorf("Update() fields = %v, want key %q", verr.Fields, tt.field)
			}
		})
	}
}

func validStorage() Update {
	return Update{
		StorageEnabled: ptr(true), StorageEndpoint: ptr(" https://s3.example.com/ "), StorageRegion: ptr("eu-west-1"),
		StorageBucket: ptr("moneyharpy"), StorageAccessKey: ptr("access"), StorageSecretKey: ptr("storage-secret-value"),
		StoragePathStyle: ptr(true),
	}
}

func TestStorageSecretIsEncryptedAndKept(t *testing.T) {
	s, store := newService(t)
	ctx := context.Background()
	if _, err := s.Update(ctx, validStorage()); err != nil {
		t.Fatalf("Update() error = %v", err)
	}
	for key, raw := range store.values {
		if strings.Contains(string(raw), "storage-secret-value") {
			t.Errorf("stored value for %q contains the secret key in clear", key)
		}
	}
	got, err := s.Update(ctx, Update{StorageSecretKey: ptr("")})
	if err != nil {
		t.Fatalf("Update() error = %v", err)
	}
	want := Storage{
		Enabled: true, Endpoint: "https://s3.example.com", Region: "eu-west-1", Bucket: "moneyharpy",
		AccessKey: "access", SecretKey: "storage-secret-value", PathStyle: true,
	}
	if diff := cmp.Diff(want, got.Storage); diff != "" {
		t.Errorf("Update() Storage diff (-want +got):\n%s", diff)
	}
}

func TestStorageValidation(t *testing.T) {
	with := func(change func(*Update)) Update {
		u := validStorage()
		change(&u)
		return u
	}
	tests := []struct {
		name  string
		in    Update
		field string
	}{
		{name: "no endpoint", in: with(func(u *Update) { u.StorageEndpoint = ptr("") }), field: "storage_endpoint"},
		{name: "endpoint with a path", in: with(func(u *Update) { u.StorageEndpoint = ptr("https://s3.example.com/bucket") }), field: "storage_endpoint"},
		{name: "endpoint not http", in: with(func(u *Update) { u.StorageEndpoint = ptr("ftp://s3.example.com") }), field: "storage_endpoint"},
		{name: "bad bucket", in: with(func(u *Update) { u.StorageBucket = ptr("Not A Bucket") }), field: "storage_bucket"},
		{name: "no access key", in: with(func(u *Update) { u.StorageAccessKey = ptr("") }), field: "storage_access_key"},
		{name: "no secret", in: with(func(u *Update) { u.StorageSecretKey = nil }), field: "storage_secret_key"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s, _ := newService(t)
			_, err := s.Update(context.Background(), tt.in)
			var verr *resource.ValidationError
			if !errors.As(err, &verr) {
				t.Fatalf("Update() error = %v, want *resource.ValidationError", err)
			}
			if _, ok := verr.Fields[tt.field]; !ok {
				t.Errorf("Update() fields = %v, want key %q", verr.Fields, tt.field)
			}
		})
	}
}
