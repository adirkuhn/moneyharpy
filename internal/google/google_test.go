package google

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/adirkuhn/moneyharpy/internal/identity"
	"github.com/adirkuhn/moneyharpy/internal/settings"
	"github.com/go-jose/go-jose/v4"
	"github.com/go-jose/go-jose/v4/jwt"
	"github.com/google/go-cmp/cmp"
)

type staticSettings struct{ inst settings.Instance }

func (s staticSettings) Instance(context.Context) (settings.Instance, error) { return s.inst, nil }

// issuer is a minimal OpenID Connect provider.
type issuer struct {
	*httptest.Server
	key    *rsa.PrivateKey
	claims map[string]any
}

func newIssuer(t *testing.T) *issuer {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("GenerateKey() error = %v", err)
	}
	iss := &issuer{key: key}
	mux := http.NewServeMux()
	mux.HandleFunc("/.well-known/openid-configuration", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"issuer":                                iss.URL,
			"authorization_endpoint":                iss.URL + "/auth",
			"token_endpoint":                        iss.URL + "/token",
			"jwks_uri":                              iss.URL + "/jwks",
			"id_token_signing_alg_values_supported": []string{"RS256"},
		})
	})
	mux.HandleFunc("/jwks", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(jose.JSONWebKeySet{Keys: []jose.JSONWebKey{{Key: key.Public(), KeyID: "k", Algorithm: "RS256", Use: "sig"}}})
	})
	mux.HandleFunc("/token", func(w http.ResponseWriter, _ *http.Request) {
		signer, err := jose.NewSigner(jose.SigningKey{Algorithm: jose.RS256, Key: jose.JSONWebKey{Key: key, KeyID: "k"}}, nil)
		if err != nil {
			t.Errorf("NewSigner() error = %v", err)
			return
		}
		token, err := jwt.Signed(signer).Claims(iss.claims).Serialize()
		if err != nil {
			t.Errorf("Serialize() error = %v", err)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "a", "token_type": "bearer", "id_token": token})
	})
	iss.Server = httptest.NewServer(mux)
	t.Cleanup(iss.Close)
	return iss
}

func (i *issuer) validClaims() map[string]any {
	return map[string]any{
		"iss": i.URL, "aud": "client-id", "sub": "subject-1", "nonce": "nonce-1",
		"exp": time.Now().Add(time.Hour).Unix(), "iat": time.Now().Unix(),
		"email": "a@example.com", "email_verified": true, "name": "Ann",
	}
}

func newClient(i *issuer) *Client {
	s := staticSettings{inst: settings.Instance{Google: settings.Google{Enabled: true, ClientID: "client-id", ClientSecret: "secret"}}}
	return New(s, "https://harpy.test", i.URL)
}

func TestAuthURL(t *testing.T) {
	i := newIssuer(t)
	got, err := newClient(i).AuthURL(context.Background(), "state-1", "nonce-1")
	if err != nil {
		t.Fatalf("AuthURL() error = %v", err)
	}
	u, err := url.Parse(got)
	if err != nil {
		t.Fatalf("Parse(%q) error = %v", got, err)
	}
	want := map[string]string{
		"client_id":     "client-id",
		"redirect_uri":  "https://harpy.test/api/auth/google/callback",
		"response_type": "code",
		"state":         "state-1",
		"nonce":         "nonce-1",
		"scope":         "openid email profile",
	}
	for k, v := range want {
		if q := u.Query().Get(k); q != v {
			t.Errorf("AuthURL() %s = %q, want %q", k, q, v)
		}
	}
}

func TestExchange(t *testing.T) {
	i := newIssuer(t)
	i.claims = i.validClaims()
	got, err := newClient(i).Exchange(context.Background(), "code", "nonce-1")
	if err != nil {
		t.Fatalf("Exchange() error = %v", err)
	}
	want := identity.External{Provider: "google", Subject: "subject-1", Email: "a@example.com", EmailVerified: true, Name: "Ann"}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("Exchange() diff (-want +got):\n%s", diff)
	}
}

func TestExchangeRejectsBadTokens(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(map[string]any)
		want   string
	}{
		{name: "wrong nonce", mutate: func(c map[string]any) { c["nonce"] = "other" }, want: "nonce"},
		{name: "wrong audience", mutate: func(c map[string]any) { c["aud"] = "someone-else" }, want: "audience"},
		{name: "expired", mutate: func(c map[string]any) { c["exp"] = time.Now().Add(-time.Hour).Unix() }, want: "expired"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			i := newIssuer(t)
			i.claims = i.validClaims()
			tt.mutate(i.claims)
			_, err := newClient(i).Exchange(context.Background(), "code", "nonce-1")
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Errorf("Exchange() error = %v, want containing %q", err, tt.want)
			}
		})
	}
}
