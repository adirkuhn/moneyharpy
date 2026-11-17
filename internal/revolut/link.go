package revolut

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/adirkuhn/moneyharpy/internal/bankfile"
	"github.com/adirkuhn/moneyharpy/internal/banking"
)

// Link implements banking.KeyLink for Revolut Business.
type Link struct {
	// BaseURL is the Business API; empty is DefaultBaseURL.
	BaseURL string
	HTTP    *http.Client
}

func (l Link) client(provider, key string) (*Client, error) {
	if provider != banking.ProviderRevolut {
		return nil, fmt.Errorf("revolut: provider %q", provider)
	}
	return &Client{BaseURL: l.BaseURL, Key: key, HTTP: l.HTTP}, nil
}

// domainError tells the banking service what an error means for the connection.
func domainError(err error) error {
	switch {
	case err == nil, errors.Is(err, context.Canceled):
		return err
	case errors.Is(err, ErrKeyRejected):
		return fmt.Errorf("%w (%v)", banking.ErrLinkExpired, err)
	case errors.Is(err, ErrRateLimited):
		return fmt.Errorf("%w (%v)", banking.ErrLinkRateLimited, err)
	}
	return fmt.Errorf("%w: %v", banking.ErrLinkFailed, err)
}

// Accounts implements banking.KeyLink.
func (l Link) Accounts(ctx context.Context, provider, key string) ([]banking.LinkAccount, error) {
	c, err := l.client(provider, key)
	if err != nil {
		return nil, err
	}
	accounts, err := c.Accounts(ctx)
	if err != nil {
		return nil, domainError(err)
	}
	out := make([]banking.LinkAccount, 0, len(accounts))
	for _, a := range accounts {
		out = append(out, banking.LinkAccount{UID: a.ID, IBAN: a.IBAN, Currency: a.Currency, Name: a.Name})
	}
	return out, nil
}

// Transactions implements banking.KeyLink.
func (l Link) Transactions(ctx context.Context, provider, key, uid string, from time.Time) ([]bankfile.Entry, error) {
	c, err := l.client(provider, key)
	if err != nil {
		return nil, err
	}
	entries, err := c.Transactions(ctx, uid, from)
	return entries, domainError(err)
}
