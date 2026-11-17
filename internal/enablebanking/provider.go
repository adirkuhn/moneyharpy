package enablebanking

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/adirkuhn/moneyharpy/internal/settings"
)

// Errors returned by Provider.
var (
	// ErrOff reports that bank connections are turned off on this installation.
	ErrOff = errors.New("bank connections are turned off")
	// ErrNotConfigured reports a missing application id or key.
	ErrNotConfigured = errors.New("the Enable Banking application id and private key are not set")
)

// Settings reads the instance settings, which an administrator can change
// at any time.
type Settings interface {
	Instance(ctx context.Context) (settings.Instance, error)
}

// Provider makes clients from the current instance settings.
type Provider struct {
	Settings Settings
	HTTP     *http.Client
	// BaseURL is Enable Banking's API; empty is DefaultBaseURL.
	BaseURL string
	Now     func() time.Time
}

// Client returns a client for the configured application, or ErrOff while
// bank connections are turned off.
func (p *Provider) Client(ctx context.Context) (*Client, error) {
	inst, err := p.Settings.Instance(ctx)
	if err != nil {
		return nil, err
	}
	if !inst.BankLink.Enabled {
		return nil, ErrOff
	}
	return p.client(inst.BankLink)
}

func (p *Provider) client(cfg settings.BankLink) (*Client, error) {
	if cfg.ApplicationID == "" || cfg.PrivateKey == "" {
		return nil, ErrNotConfigured
	}
	key, err := settings.ParseRSAKey(cfg.PrivateKey)
	if err != nil {
		return nil, ErrNotConfigured
	}
	base := p.BaseURL
	if base == "" {
		base = DefaultBaseURL
	}
	httpClient := p.HTTP
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 60 * time.Second}
	}
	now := p.Now
	if now == nil {
		now = time.Now
	}
	return &Client{BaseURL: base, ApplicationID: cfg.ApplicationID, Key: key, HTTP: httpClient, Now: now}, nil
}

// Check lists the Dutch banks with the stored application, also while bank
// connections are off, so an administrator can see the setup works.
func (p *Provider) Check(ctx context.Context) error {
	inst, err := p.Settings.Instance(ctx)
	if err != nil {
		return err
	}
	c, err := p.client(inst.BankLink)
	if err != nil {
		return err
	}
	_, err = c.Banks(ctx, "NL")
	return err
}
