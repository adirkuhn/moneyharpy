package enablebanking

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/adirkuhn/moneyharpy/internal/bankfile"
	"github.com/adirkuhn/moneyharpy/internal/banking"
)

// Link implements banking.BankLinks with the client of the current settings.
func (p *Provider) Link(ctx context.Context) (banking.BankLink, error) {
	c, err := p.Client(ctx)
	switch {
	case errors.Is(err, ErrOff), errors.Is(err, ErrNotConfigured):
		return nil, banking.ErrLinkOff
	case err != nil:
		return nil, err
	}
	return link{c}, nil
}

// link adapts a Client to banking.BankLink.
type link struct{ c *Client }

// domainError tells the banking service what an error means for the connection.
func domainError(err error) error {
	switch {
	case errors.Is(err, ErrExpired):
		return fmt.Errorf("%w (%v)", banking.ErrLinkExpired, err)
	case errors.Is(err, ErrRateLimited):
		return fmt.Errorf("%w (%v)", banking.ErrLinkRateLimited, err)
	case err == nil, errors.Is(err, context.Canceled):
		return err
	}
	return fmt.Errorf("%w: %v", banking.ErrLinkFailed, err)
}

func (l link) Banks(ctx context.Context, country string) ([]banking.LinkBank, error) {
	banks, err := l.c.Banks(ctx, country)
	if err != nil {
		return nil, domainError(err)
	}
	out := make([]banking.LinkBank, 0, len(banks))
	for _, b := range banks {
		out = append(out, banking.LinkBank{Name: b.Name, Country: b.Country, Logo: b.Logo, MaxConsent: b.MaxConsent})
	}
	return out, nil
}

func (l link) Authorize(ctx context.Context, a banking.LinkAuthorization) (string, error) {
	url, err := l.c.Authorize(ctx, Authorization{Bank: a.Bank, Country: a.Country, State: a.State, RedirectURL: a.RedirectURL, ValidUntil: a.ValidUntil})
	return url, domainError(err)
}

func (l link) Session(ctx context.Context, code string) (banking.LinkSession, error) {
	s, err := l.c.Session(ctx, code)
	if err != nil {
		return banking.LinkSession{}, domainError(err)
	}
	out := banking.LinkSession{ID: s.ID, ValidUntil: s.ValidUntil}
	for _, a := range s.Accounts {
		out.Accounts = append(out.Accounts, banking.LinkAccount{UID: a.UID, IBAN: a.IBAN, Currency: a.Currency, Name: a.Name})
	}
	return out, nil
}

func (l link) Transactions(ctx context.Context, uid string, from time.Time) ([]bankfile.Entry, error) {
	entries, err := l.c.Transactions(ctx, uid, from)
	return entries, domainError(err)
}

func (l link) Close(ctx context.Context, session string) error {
	return domainError(l.c.Close(ctx, session))
}
