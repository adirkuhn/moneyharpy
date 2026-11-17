package settings

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"strings"
	"testing"

	"github.com/adirkuhn/moneyharpy/internal/resource"
)

func testKeyPEM(t *testing.T) string {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	der, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	return string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der}))
}

func TestBankLinkIsOffByDefault(t *testing.T) {
	s, _ := newService(t)
	got, err := s.Instance(context.Background())
	if err != nil || got.BankLink.Enabled {
		t.Errorf("Instance() = %+v, %v; want bank connections off", got.BankLink, err)
	}
}

func TestBankLinkKeyIsEncryptedAndKept(t *testing.T) {
	s, store := newService(t)
	ctx := context.Background()
	key := testKeyPEM(t)
	appID := "00000000-0000-4000-8000-000000000001"
	if _, err := s.Update(ctx, Update{BankLinkEnabled: ptr(true), BankLinkApplicationID: ptr(" " + appID + " "), BankLinkPrivateKey: ptr(key)}); err != nil {
		t.Fatalf("Update() error = %v", err)
	}
	for k, raw := range store.values {
		if strings.Contains(string(raw), "PRIVATE KEY") {
			t.Errorf("stored value for %q contains the private key in clear", k)
		}
	}
	got, err := s.Update(ctx, Update{BankLinkPrivateKey: ptr("")})
	if err != nil {
		t.Fatalf("Update() error = %v", err)
	}
	if !got.BankLink.Enabled || got.BankLink.ApplicationID != appID || got.BankLink.PrivateKey != strings.TrimSpace(key) {
		t.Errorf("Update() = %+v, want the key kept", got.BankLink)
	}
}

func TestBankLinkNeedsApplicationAndValidKey(t *testing.T) {
	s, _ := newService(t)
	for name, tt := range map[string]struct {
		u      Update
		fields []string
	}{
		"nothing":       {Update{BankLinkEnabled: ptr(true)}, []string{"enable_banking_application_id", "enable_banking_private_key"}},
		"not a key":     {Update{BankLinkEnabled: ptr(true), BankLinkApplicationID: ptr("00000000-0000-4000-8000-000000000001"), BankLinkPrivateKey: ptr("hello")}, []string{"enable_banking_private_key"}},
		"not a uuid":    {Update{BankLinkEnabled: ptr(true), BankLinkApplicationID: ptr("my app"), BankLinkPrivateKey: ptr(testKeyPEM(t))}, []string{"enable_banking_application_id"}},
		"key while off": {Update{BankLinkPrivateKey: ptr("hello")}, []string{"enable_banking_private_key"}},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := s.Update(context.Background(), tt.u)
			var verr *resource.ValidationError
			if !errors.As(err, &verr) {
				t.Fatalf("Update() error = %v, want a validation error", err)
			}
			for _, f := range tt.fields {
				if _, ok := verr.Fields[f]; !ok {
					t.Errorf("fields = %v, want %s", verr.Fields, f)
				}
			}
		})
	}
}
