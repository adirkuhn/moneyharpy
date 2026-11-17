package enablebanking

import (
	"context"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"net/http"
	"testing"

	"github.com/adirkuhn/moneyharpy/internal/settings"
)

type fixedSettings settings.BankLink

func (f fixedSettings) Instance(context.Context) (settings.Instance, error) {
	return settings.Instance{BankLink: settings.BankLink(f)}, nil
}

func TestProviderFollowsTheSettings(t *testing.T) {
	c, key := fakeBank(t, map[string]http.HandlerFunc{
		"GET /aspsps": func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(`{"aspsps":[]}`)) },
	})
	pemKey := string(pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)}))
	ctx := context.Background()
	p := &Provider{HTTP: c.HTTP, BaseURL: c.BaseURL, Now: c.Now}

	p.Settings = fixedSettings{ApplicationID: appID, PrivateKey: pemKey}
	if _, err := p.Client(ctx); !errors.Is(err, ErrOff) {
		t.Errorf("off: Client() = %v", err)
	}
	if err := p.Check(ctx); err != nil {
		t.Errorf("Check() while off = %v, want it to try the application", err)
	}
	p.Settings = fixedSettings{Enabled: true, ApplicationID: appID}
	if _, err := p.Client(ctx); !errors.Is(err, ErrNotConfigured) {
		t.Errorf("no key: Client() = %v", err)
	}
	p.Settings = fixedSettings{Enabled: true, ApplicationID: appID, PrivateKey: pemKey}
	got, err := p.Client(ctx)
	if err != nil || got.ApplicationID != appID {
		t.Fatalf("Client() = %+v, %v", got, err)
	}
	if _, err := got.Banks(ctx, "NL"); err != nil {
		t.Errorf("Banks() = %v", err)
	}
}
