// Package lm2 signs users in through an Akantio LM2 installation. It never
// verifies tokens itself, which would require LM2's signing key: it asks LM2
// who a token belongs to.
package lm2

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"

	"github.com/adirkuhn/moneyharpy/internal/identity"
	"github.com/adirkuhn/moneyharpy/internal/settings"
)

const (
	provider     = "lm2"
	callbackPath = "/api/auth/lm2/callback/"
	loginPath    = "/app-login"
	userinfoPath = "/api/v1/auth/app/userinfo"
	maxBodyBytes = 1 << 16
)

// ErrDisabled reports that LM2 login is not enabled.
var ErrDisabled = errors.New("lm2 login is disabled")

// Settings provides the configured LM2 installation.
type Settings interface {
	Instance(ctx context.Context) (settings.Instance, error)
}

// Client talks to LM2.
type Client struct {
	settings Settings
	baseURL  string
	http     *http.Client
}

// New returns a Client for an installation served at baseURL.
func New(s Settings, baseURL string, httpClient *http.Client) *Client {
	return &Client{settings: s, baseURL: baseURL, http: httpClient}
}

// LoginURL returns the LM2 page that starts a sign-in. The state goes in the
// path of the return address because LM2 appends "?token=" to it verbatim, so
// a query string there would be corrupted.
func (c *Client) LoginURL(ctx context.Context, state string) (string, error) {
	cfg, err := c.config(ctx)
	if err != nil {
		return "", err
	}
	query := url.Values{
		"app":      {cfg.App},
		"redirect": {c.baseURL + callbackPath + url.PathEscape(state)},
	}
	return cfg.URL + loginPath + "?" + query.Encode(), nil
}

type userinfo struct {
	UserID string `json:"user_id"`
	Email  string `json:"email"`
	Name   string `json:"name"`
	App    string `json:"app"`
}

// Identify returns the user a token belongs to. A token issued to any other
// app is refused. The email is reported as unverified, since LM2 does not
// confirm addresses.
func (c *Client) Identify(ctx context.Context, token string) (identity.External, error) {
	cfg, err := c.config(ctx)
	if err != nil {
		return identity.External{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, cfg.URL+userinfoPath, nil)
	if err != nil {
		return identity.External{}, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)

	res, err := c.http.Do(req)
	if err != nil {
		return identity.External{}, fmt.Errorf("call userinfo: %w", err)
	}
	defer func() {
		// The body has been read or abandoned; nothing depends on closing it.
		_ = res.Body.Close()
	}()
	if res.StatusCode != http.StatusOK {
		return identity.External{}, fmt.Errorf("userinfo answered %d", res.StatusCode)
	}

	var body struct {
		Data userinfo `json:"data"`
	}
	if err := json.NewDecoder(io.LimitReader(res.Body, maxBodyBytes)).Decode(&body); err != nil {
		return identity.External{}, fmt.Errorf("decode userinfo: %w", err)
	}
	u := body.Data
	if u.App != cfg.App {
		return identity.External{}, fmt.Errorf("token was issued to app %q", u.App)
	}
	if u.UserID == "" || u.Email == "" {
		return identity.External{}, errors.New("userinfo is missing the user id or email")
	}
	return identity.External{Provider: provider, Subject: u.UserID, Email: u.Email, Name: u.Name}, nil
}

func (c *Client) config(ctx context.Context) (settings.LM2, error) {
	inst, err := c.settings.Instance(ctx)
	if err != nil {
		return settings.LM2{}, fmt.Errorf("read settings: %w", err)
	}
	if !inst.LM2.Enabled {
		return settings.LM2{}, ErrDisabled
	}
	return inst.LM2, nil
}
