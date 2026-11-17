package enablebanking

import (
	"context"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"net/http"
	"testing"

	"github.com/adirkuhn/moneyharpy/internal/banking"
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

func TestLinkSpeaksTheBankingServicesErrors(t *testing.T) {
	c, key := fakeBank(t, map[string]http.HandlerFunc{
		"GET /accounts/gone/transactions": func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"message":"Session expired","error":"EXPIRED_SESSION"}`))
		},
		"GET /accounts/busy/transactions": func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = w.Write([]byte(`{"message":"Slow down","error":"ASPSP_RATE_LIMIT_EXCEEDED"}`))
		},
	})
	pemKey := string(pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)}))
	ctx := context.Background()
	p := &Provider{HTTP: c.HTTP, BaseURL: c.BaseURL, Now: c.Now, Settings: fixedSettings{ApplicationID: appID, PrivateKey: pemKey}}
	if _, err := p.Link(ctx); !errors.Is(err, banking.ErrLinkOff) {
		t.Errorf("off: Link() = %v", err)
	}
	p.Settings = fixedSettings{Enabled: true, ApplicationID: appID, PrivateKey: pemKey}
	l, err := p.Link(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := l.Transactions(ctx, "gone", now); !errors.Is(err, banking.ErrLinkExpired) {
		t.Errorf("expired = %v", err)
	}
	if _, err := l.Transactions(ctx, "busy", now); !errors.Is(err, banking.ErrLinkRateLimited) {
		t.Errorf("rate limited = %v", err)
	}
}
