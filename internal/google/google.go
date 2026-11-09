// Package google signs users in through Google's OpenID Connect service, using
// credentials read from instance settings on every request so that changes
// take effect without a restart.
package google

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/adirkuhn/moneyharpy/internal/identity"
	"github.com/adirkuhn/moneyharpy/internal/settings"
	"github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"
)

// Issuer is Google's OpenID Connect issuer URL.
const Issuer = "https://accounts.google.com"

// CallbackPath is where Google sends users back to.
const CallbackPath = "/api/auth/google/callback"

// ErrDisabled reports that Google login is not enabled.
var ErrDisabled = errors.New("google login is disabled")

// Settings provides the configured credentials.
type Settings interface {
	Instance(ctx context.Context) (settings.Instance, error)
}

// Client talks to Google.
type Client struct {
	settings    Settings
	redirectURL string
	issuer      string

	mu       sync.Mutex
	provider *oidc.Provider
}

// New returns a Client for an installation served at baseURL.
func New(s Settings, baseURL, issuer string) *Client {
	return &Client{settings: s, redirectURL: baseURL + CallbackPath, issuer: issuer}
}

// AuthURL returns the address that starts a Google sign-in.
func (c *Client) AuthURL(ctx context.Context, state, nonce string) (string, error) {
	cfg, _, err := c.config(ctx)
	if err != nil {
		return "", err
	}
	return cfg.AuthCodeURL(state, oidc.Nonce(nonce)), nil
}

// Exchange trades an authorization code for the verified user it belongs to.
func (c *Client) Exchange(ctx context.Context, code, nonce string) (identity.External, error) {
	cfg, provider, err := c.config(ctx)
	if err != nil {
		return identity.External{}, err
	}
	token, err := cfg.Exchange(ctx, code)
	if err != nil {
		return identity.External{}, fmt.Errorf("exchange code: %w", err)
	}
	raw, ok := token.Extra("id_token").(string)
	if !ok {
		return identity.External{}, errors.New("token response has no id_token")
	}
	verified, err := provider.Verifier(&oidc.Config{ClientID: cfg.ClientID}).Verify(ctx, raw)
	if err != nil {
		return identity.External{}, fmt.Errorf("verify id token: %w", err)
	}
	if verified.Nonce != nonce {
		return identity.External{}, errors.New("id token nonce mismatch")
	}
	var claims struct {
		Email         string `json:"email"`
		EmailVerified bool   `json:"email_verified"`
		Name          string `json:"name"`
	}
	if err := verified.Claims(&claims); err != nil {
		return identity.External{}, fmt.Errorf("decode claims: %w", err)
	}
	return identity.External{
		Provider:      "google",
		Subject:       verified.Subject,
		Email:         claims.Email,
		EmailVerified: claims.EmailVerified,
		Name:          claims.Name,
	}, nil
}

func (c *Client) config(ctx context.Context) (oauth2.Config, *oidc.Provider, error) {
	inst, err := c.settings.Instance(ctx)
	if err != nil {
		return oauth2.Config{}, nil, fmt.Errorf("read settings: %w", err)
	}
	if !inst.Google.Enabled {
		return oauth2.Config{}, nil, ErrDisabled
	}
	provider, err := c.discover(ctx)
	if err != nil {
		return oauth2.Config{}, nil, err
	}
	return oauth2.Config{
		ClientID:     inst.Google.ClientID,
		ClientSecret: inst.Google.ClientSecret,
		RedirectURL:  c.redirectURL,
		Endpoint:     provider.Endpoint(),
		Scopes:       []string{oidc.ScopeOpenID, "email", "profile"},
	}, provider, nil
}

// discover caches the provider. It outlives the request that created it, and
// its key set keeps using this context, so cancellation must not reach it.
func (c *Client) discover(ctx context.Context) (*oidc.Provider, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.provider != nil {
		return c.provider, nil
	}
	provider, err := oidc.NewProvider(context.WithoutCancel(ctx), c.issuer)
	if err != nil {
		return nil, fmt.Errorf("discover provider: %w", err)
	}
	c.provider = provider
	return provider, nil
}
