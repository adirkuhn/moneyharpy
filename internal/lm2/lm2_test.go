package lm2

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/adirkuhn/moneyharpy/internal/identity"
	"github.com/adirkuhn/moneyharpy/internal/settings"
	"github.com/google/go-cmp/cmp"
)

type staticSettings struct{ inst settings.Instance }

func (s staticSettings) Instance(context.Context) (settings.Instance, error) { return s.inst, nil }

func newClient(lmURL string, enabled bool) *Client {
	s := staticSettings{inst: settings.Instance{LM2: settings.LM2{Enabled: enabled, URL: lmURL, App: "moneyharpy"}}}
	return New(s, "https://harpy.test", http.DefaultClient)
}

func TestLoginURL(t *testing.T) {
	got, err := newClient("https://lm.example/base", true).LoginURL(context.Background(), "state-1")
	if err != nil {
		t.Fatalf("LoginURL() error = %v", err)
	}
	u, err := url.Parse(got)
	if err != nil {
		t.Fatalf("Parse(%q) error = %v", got, err)
	}
	if want := "/base/app-login"; u.Path != want {
		t.Errorf("LoginURL() path = %q, want %q", u.Path, want)
	}
	if got, want := u.Query().Get("app"), "moneyharpy"; got != want {
		t.Errorf("LoginURL() app = %q, want %q", got, want)
	}
	if got, want := u.Query().Get("redirect"), "https://harpy.test/api/auth/lm2/callback/state-1"; got != want {
		t.Errorf("LoginURL() redirect = %q, want %q", got, want)
	}
}

func TestDisabled(t *testing.T) {
	c := newClient("https://lm.example", false)
	if _, err := c.LoginURL(context.Background(), "s"); !errors.Is(err, ErrDisabled) {
		t.Errorf("LoginURL() error = %v, want ErrDisabled", err)
	}
	if _, err := c.Identify(context.Background(), "t"); !errors.Is(err, ErrDisabled) {
		t.Errorf("Identify() error = %v, want ErrDisabled", err)
	}
}

// userinfoServer answers like GET /api/v1/auth/app/userinfo in LM2.
func userinfoServer(t *testing.T, status int, data map[string]string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/auth/app/userinfo" {
			t.Errorf("path = %q, want userinfo", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer tok" {
			t.Errorf("Authorization = %q, want Bearer tok", got)
		}
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(map[string]any{"data": data})
	}))
	t.Cleanup(srv.Close)
	return srv
}

func validUser() map[string]string {
	return map[string]string{"user_id": "u1", "email": "a@example.com", "name": "Ann", "company_id": "c1", "app": "moneyharpy"}
}

func TestIdentify(t *testing.T) {
	srv := userinfoServer(t, http.StatusOK, validUser())
	got, err := newClient(srv.URL, true).Identify(context.Background(), "tok")
	if err != nil {
		t.Fatalf("Identify() error = %v", err)
	}
	// EmailVerified stays false: LM2 does not vouch that an address was confirmed.
	want := identity.External{Provider: "lm2", Subject: "u1", Email: "a@example.com", Name: "Ann"}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("Identify() diff (-want +got):\n%s", diff)
	}
}

func TestIdentifyRejects(t *testing.T) {
	with := func(key, value string) map[string]string {
		d := validUser()
		d[key] = value
		return d
	}
	tests := []struct {
		name   string
		status int
		data   map[string]string
	}{
		{name: "token of another app", status: http.StatusOK, data: with("app", "commerceos")},
		{name: "no user id", status: http.StatusOK, data: with("user_id", "")},
		{name: "no email", status: http.StatusOK, data: with("email", "")},
		{name: "refused", status: http.StatusUnauthorized, data: validUser()},
		{name: "suspended", status: http.StatusForbidden, data: validUser()},
		{name: "broken", status: http.StatusInternalServerError, data: validUser()},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := userinfoServer(t, tt.status, tt.data)
			if _, err := newClient(srv.URL, true).Identify(context.Background(), "tok"); err == nil {
				t.Error("Identify() error = nil, want error")
			}
		})
	}
}
