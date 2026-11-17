package banking

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/adirkuhn/moneyharpy/internal/bankfile"
	"github.com/adirkuhn/moneyharpy/internal/resource"
)

// Providers of connected accounts.
const (
	// ProviderEnableBanking connects banks through Enable Banking, which
	// asks the user to authorise access at their bank.
	ProviderEnableBanking = "enable_banking"
	// ProviderRevolut connects Revolut Business with an API key.
	ProviderRevolut = "revolut"
)

// keyProviders are the providers connected with an API key, by the name
// the connection shows.
var keyProviders = map[string]string{ProviderRevolut: "Revolut Business"}

// Connection states.
const (
	// ConnectionPending waits for the user to authorise access at the bank.
	ConnectionPending = "pending"
	// ConnectionActive fetches the transactions of its accounts.
	ConnectionActive = "active"
	// ConnectionExpired needs the user to authorise access again.
	ConnectionExpired = "expired"
)

// Errors of bank connections.
var (
	ErrConnectionNotFound = errors.New("bank connection not found")
	// ErrLinkOff reports bank connections turned off on this installation.
	ErrLinkOff = errors.New("bank connections are turned off on this installation")
	// ErrLinkExpired reports a consent the bank no longer honours.
	ErrLinkExpired = errors.New("the bank no longer gives access; reconnect the bank")
	// ErrLinkRateLimited reports the bank's limit on fetches without the
	// user, usually four a day.
	ErrLinkRateLimited = errors.New("the bank allows no more fetches for now; it is tried again later")
	// ErrLinkFailed reports a refusal or failure of the bank or the provider.
	ErrLinkFailed = errors.New("the bank connection failed")
)

// Sync timing: once a day is enough for bookkeeping and well within the
// banks' four fetches a day without the user; banks book some transactions
// days late.
const (
	syncEvery    = 24 * time.Hour
	syncOverlap  = 7 * 24 * time.Hour
	firstHistory = 90
	// defaultConsent is asked when the bank does not say how long it allows.
	defaultConsent = 90 * 24 * time.Hour
)

// Connection is the consent of a bank to fetch the transactions of the
// administration's accounts there.
type Connection struct {
	ID               resource.ID
	AdministrationID resource.ID
	Provider         string
	Bank, Country    string
	State            string
	// SessionID is the provider's session, once authorised.
	SessionID  string
	ValidUntil *time.Time
	// AuthState finds the connection when the bank sends the user back;
	// empty when no authorisation is under way.
	AuthState string
	// Secret is the sealed API key of a key provider.
	Secret string
	// CreatedBy is the user the transactions are fetched for.
	CreatedBy  resource.ID
	LastSyncAt *time.Time
	LastError  string
	Accounts   []LinkedAccount
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

// LinkedAccount is a financial account fetched through a connection.
type LinkedAccount struct {
	AccountID resource.ID
	// UID addresses the account in the provider's current session.
	UID string
}

// ConnectionRequest asks to connect a bank.
type ConnectionRequest struct {
	Bank, Country string
}

// LinkBank is a bank that can be connected.
type LinkBank struct {
	Name, Country, Logo string
	MaxConsent          time.Duration
}

// LinkAuthorization asks the bank for access.
type LinkAuthorization struct {
	Bank, Country, State, RedirectURL string
	ValidUntil                        time.Time
}

// LinkSession is an authorised access to accounts.
type LinkSession struct {
	ID         string
	ValidUntil time.Time
	Accounts   []LinkAccount
}

// LinkAccount is an account in a session.
type LinkAccount struct {
	UID, IBAN, Currency, Name string
}

// BankLink reaches the banks.
type BankLink interface {
	Banks(ctx context.Context, country string) ([]LinkBank, error)
	Authorize(ctx context.Context, a LinkAuthorization) (string, error)
	Session(ctx context.Context, code string) (LinkSession, error)
	// Transactions returns the account's booked transactions from the day on.
	Transactions(ctx context.Context, uid string, from time.Time) ([]bankfile.Entry, error)
	Close(ctx context.Context, session string) error
}

// BankLinks gives the bank link of the current settings, or ErrLinkOff.
type BankLinks interface {
	Link(ctx context.Context) (BankLink, error)
}

// ConnectionStore keeps bank connections.
type ConnectionStore interface {
	Connections(ctx context.Context, admin resource.ID) ([]Connection, error)
	Connection(ctx context.Context, admin, id resource.ID) (Connection, error)
	ConnectionByAuthState(ctx context.Context, state string) (Connection, error)
	SaveConnection(ctx context.Context, c Connection) error
	DeleteConnection(ctx context.Context, admin, id resource.ID) error
	// DueConnections returns the active connections of every administration
	// last fetched before the time.
	DueConnections(ctx context.Context, before time.Time) ([]Connection, error)
}

// KeyLink reaches banks whose account holders make an API key, as Revolut
// Business does.
type KeyLink interface {
	// Accounts lists the accounts the key reaches; UID addresses each.
	Accounts(ctx context.Context, provider, key string) ([]LinkAccount, error)
	Transactions(ctx context.Context, provider, key, uid string, from time.Time) ([]bankfile.Entry, error)
}

// Secrets seals API keys for storage and opens them again.
type Secrets interface {
	Seal(plain string) (string, error)
	Open(sealed string) (string, error)
}

// UseKeyLinks lets administrations connect banks with an API key.
func (s *Service) UseKeyLinks(k KeyLink, secrets Secrets) {
	s.keys, s.secrets = k, secrets
}

// UseBankLinks lets administrations connect their banks; redirect is where
// banks send users back to.
func (s *Service) UseBankLinks(links BankLinks, store ConnectionStore, redirect string) {
	s.links, s.connections, s.redirect = links, store, redirect
}

func (s *Service) link(ctx context.Context) (BankLink, error) {
	if s.links == nil {
		return nil, ErrLinkOff
	}
	return s.links.Link(ctx)
}

// Banks lists the banks of the country that can be connected.
func (s *Service) Banks(ctx context.Context, admin, user resource.ID, country string) ([]LinkBank, error) {
	if _, err := s.ensure(ctx, admin, user); err != nil {
		return nil, err
	}
	l, err := s.link(ctx)
	if err != nil {
		return nil, err
	}
	banks, err := l.Banks(ctx, strings.ToUpper(country))
	if err != nil {
		return nil, err
	}
	slices.SortFunc(banks, func(a, b LinkBank) int { return strings.Compare(strings.ToLower(a.Name), strings.ToLower(b.Name)) })
	return banks, nil
}

// Connections lists the administration's bank connections.
func (s *Service) Connections(ctx context.Context, admin, user resource.ID) ([]Connection, error) {
	if _, err := s.ensure(ctx, admin, user); err != nil {
		return nil, err
	}
	if s.connections == nil {
		return nil, nil
	}
	list, err := s.connections.Connections(ctx, admin)
	if err != nil {
		return nil, err
	}
	out := list[:0]
	for _, c := range list {
		// A connection never authorised is an abandoned attempt.
		if c.State != ConnectionPending {
			out = append(out, s.expire(c))
		}
	}
	slices.SortFunc(out, func(a, b Connection) int { return a.CreatedAt.Compare(b.CreatedAt) })
	return out, nil
}

// Connection returns one bank connection.
func (s *Service) Connection(ctx context.Context, admin, user, id resource.ID) (Connection, error) {
	if _, err := s.ensure(ctx, admin, user); err != nil {
		return Connection{}, err
	}
	if s.connections == nil {
		return Connection{}, ErrConnectionNotFound
	}
	c, err := s.connections.Connection(ctx, admin, id)
	return s.expire(c), err
}

// expire marks a connection past its consent expired.
func (s *Service) expire(c Connection) Connection {
	if c.State == ConnectionActive && c.ValidUntil != nil && !s.now().Before(*c.ValidUntil) {
		c.State = ConnectionExpired
	}
	return c
}

func newAuthState() (string, error) {
	b := make([]byte, 24)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// StartConnection starts connecting a bank and returns the bank's address
// to send the user to.
func (s *Service) StartConnection(ctx context.Context, admin, user resource.ID, r ConnectionRequest) (string, Connection, error) {
	if _, err := s.ensure(ctx, admin, user); err != nil {
		return "", Connection{}, err
	}
	now := s.now()
	c := Connection{
		ID: s.ids.Next(), AdministrationID: admin, Provider: ProviderEnableBanking, Bank: strings.TrimSpace(r.Bank),
		Country: strings.ToUpper(strings.TrimSpace(r.Country)), State: ConnectionPending, CreatedBy: user, CreatedAt: now, UpdatedAt: now,
	}
	url, err := s.authorize(ctx, &c)
	return url, c, err
}

// Reconnect asks the connection's bank for access again, as a consent
// lasts some months; the accounts stay connected.
func (s *Service) Reconnect(ctx context.Context, admin, user, id resource.ID) (string, error) {
	c, err := s.Connection(ctx, admin, user, id)
	if err != nil {
		return "", err
	}
	if _, ok := keyProviders[c.Provider]; ok {
		return "", invalid("api_key", "must be replaced with a new key to reconnect")
	}
	return s.authorize(ctx, &c)
}

// ConnectWithKey connects the accounts an API key reaches, matched to the
// administration's accounts by IBAN or added, and fetches their
// transactions.
func (s *Service) ConnectWithKey(ctx context.Context, admin, user resource.ID, provider, key string) (Connection, error) {
	if _, err := s.ensure(ctx, admin, user); err != nil {
		return Connection{}, err
	}
	name, ok := keyProviders[provider]
	if !ok || s.keys == nil {
		return Connection{}, invalid("provider", "is not a bank that connects with an API key")
	}
	now := s.now()
	c := Connection{ID: s.ids.Next(), AdministrationID: admin, Provider: provider, Bank: name, State: ConnectionActive, CreatedBy: user, CreatedAt: now, UpdatedAt: now}
	return s.useKey(ctx, c, user, key)
}

// ReplaceKey connects a key provider's connection with a new API key, as
// when the old one was revoked or expired; the accounts stay connected.
func (s *Service) ReplaceKey(ctx context.Context, admin, user, id resource.ID, key string) (Connection, error) {
	c, err := s.Connection(ctx, admin, user, id)
	if err != nil {
		return Connection{}, err
	}
	if _, ok := keyProviders[c.Provider]; !ok || s.keys == nil {
		return Connection{}, invalid("provider", "is not a bank that connects with an API key")
	}
	return s.useKey(ctx, c, user, key)
}

// useKey checks the key with the bank, links the accounts it reaches and
// fetches their transactions.
func (s *Service) useKey(ctx context.Context, c Connection, user resource.ID, key string) (Connection, error) {
	key = strings.TrimSpace(key)
	if key == "" {
		return Connection{}, invalid("api_key", "can't be blank")
	}
	shared, err := s.keys.Accounts(ctx, c.Provider, key)
	switch {
	case errors.Is(err, ErrLinkExpired):
		return Connection{}, invalid("api_key", "was not accepted by the bank; check that it is active and may read accounts and transactions")
	case err != nil:
		return Connection{}, err
	}
	linked, err := s.linkAccounts(ctx, c.AdministrationID, user, c.Provider, shared)
	if err != nil {
		return Connection{}, err
	}
	if c.Secret, err = s.secrets.Seal(key); err != nil {
		return Connection{}, fmt.Errorf("seal api key: %w", err)
	}
	c.Accounts, c.State, c.LastError, c.CreatedBy, c.UpdatedAt = linked, ConnectionActive, "", user, s.now()
	if err := s.connections.SaveConnection(ctx, c); err != nil {
		return Connection{}, err
	}
	return s.sync(ctx, c)
}

// authorize starts an authorisation for the connection at its bank.
func (s *Service) authorize(ctx context.Context, c *Connection) (string, error) {
	if s.connections == nil {
		return "", ErrLinkOff
	}
	l, err := s.link(ctx)
	if err != nil {
		return "", err
	}
	banks, err := l.Banks(ctx, c.Country)
	if err != nil {
		return "", err
	}
	i := slices.IndexFunc(banks, func(b LinkBank) bool { return b.Name == c.Bank })
	if i < 0 {
		var verr resource.ValidationError
		verr.Add("bank", "is not a bank that can be connected")
		return "", &verr
	}
	consent := banks[i].MaxConsent
	if consent <= 0 {
		consent = defaultConsent
	}
	if c.AuthState, err = newAuthState(); err != nil {
		return "", err
	}
	url, err := l.Authorize(ctx, LinkAuthorization{Bank: c.Bank, Country: c.Country, State: c.AuthState, RedirectURL: s.redirect, ValidUntil: s.now().Add(consent)})
	if err != nil {
		return "", err
	}
	c.UpdatedAt = s.now()
	if err := s.connections.SaveConnection(ctx, *c); err != nil {
		return "", err
	}
	return url, nil
}

// FinishConnection completes the authorisation the bank sent the user back
// from: the accounts it shares are connected, matched to the administration's
// accounts by IBAN and added when it has none yet, and their transactions
// are fetched.
func (s *Service) FinishConnection(ctx context.Context, user resource.ID, state, code string) (Connection, error) {
	if s.connections == nil || state == "" {
		return Connection{}, ErrConnectionNotFound
	}
	c, err := s.connections.ConnectionByAuthState(ctx, state)
	if err != nil {
		return Connection{}, err
	}
	if _, err := s.ensure(ctx, c.AdministrationID, user); err != nil {
		return Connection{}, err
	}
	l, err := s.link(ctx)
	if err != nil {
		return Connection{}, err
	}
	// The state is used once, whatever the bank answers.
	c.AuthState = ""
	if err := s.connections.SaveConnection(ctx, c); err != nil {
		return Connection{}, err
	}
	session, err := l.Session(ctx, code)
	if err != nil {
		return Connection{}, err
	}
	linked, err := s.linkAccounts(ctx, c.AdministrationID, user, c.Provider, session.Accounts)
	if err != nil {
		_ = l.Close(ctx, session.ID)
		return Connection{}, err
	}
	if c.SessionID != "" && c.SessionID != session.ID {
		_ = l.Close(ctx, c.SessionID) // The old consent is replaced; closing it is a courtesy.
	}
	validUntil := session.ValidUntil
	c.SessionID, c.ValidUntil, c.Accounts, c.State, c.LastError = session.ID, &validUntil, linked, ConnectionActive, ""
	c.CreatedBy, c.UpdatedAt = user, s.now()
	if err := s.connections.SaveConnection(ctx, c); err != nil {
		return Connection{}, err
	}
	return s.sync(ctx, c)
}

// linkAccounts finds or adds the financial account of each shared account.
func (s *Service) linkAccounts(ctx context.Context, admin, user resource.ID, provider string, shared []LinkAccount) ([]LinkedAccount, error) {
	accounts, err := s.accounts(ctx, admin)
	if err != nil {
		return nil, err
	}
	var out []LinkedAccount
	for _, a := range shared {
		if a.IBAN == "" || a.UID == "" {
			continue
		}
		i := slices.IndexFunc(accounts, func(acc Account) bool { return sameAccount(acc.Identifier, a.IBAN) })
		var acc Account
		if i >= 0 {
			acc = accounts[i]
		} else {
			kind, name := BankAccount, cmpOr(strings.TrimSpace(a.Name), a.IBAN)
			if slices.ContainsFunc(accounts, func(x Account) bool { return strings.EqualFold(x.Name, name) }) {
				name = a.IBAN
			}
			c := AccountChanges{Type: &kind, Name: &name, Identifier: &a.IBAN}
			if a.Currency != "" {
				c.Currency = &a.Currency
			}
			if acc, err = s.CreateAccount(ctx, admin, user, c); err != nil {
				return nil, err
			}
			accounts = append(accounts, acc)
		}
		if acc.Provider != provider {
			acc.Provider, acc.UpdatedAt = provider, s.now()
			if err := s.store.UpdateAccounts(ctx, []Account{acc}); err != nil {
				return nil, err
			}
		}
		out = append(out, LinkedAccount{AccountID: acc.ID, UID: a.UID})
	}
	if len(out) == 0 {
		var verr resource.ValidationError
		if provider == ProviderEnableBanking {
			verr.Add("accounts", "were not shared by the bank; link them in the Enable Banking control panel and connect again")
		} else {
			verr.Add("accounts", "with an IBAN were not found with this key")
		}
		return nil, &verr
	}
	return out, nil
}

func cmpOr(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

// SyncConnection fetches the connection's new transactions now.
func (s *Service) SyncConnection(ctx context.Context, admin, user, id resource.ID) (Connection, error) {
	c, err := s.Connection(ctx, admin, user, id)
	if err != nil {
		return Connection{}, err
	}
	return s.sync(ctx, c)
}

// SyncDue fetches the transactions of the connections not fetched for a
// day, and returns how many it fetched.
func (s *Service) SyncDue(ctx context.Context) int {
	if s.connections == nil {
		return 0
	}
	due, err := s.connections.DueConnections(ctx, s.now().Add(-syncEvery))
	if err != nil {
		return 0
	}
	n := 0
	for _, c := range due {
		if s.expire(c).State == ConnectionExpired {
			c.State, c.UpdatedAt = ConnectionExpired, s.now()
			_ = s.connections.SaveConnection(ctx, c)
			continue
		}
		if synced, err := s.sync(ctx, c); err == nil && synced.LastError == "" {
			n++
		}
	}
	return n
}

// fetcher fetches an account's transactions the way the connection's
// provider does.
type fetcher func(ctx context.Context, uid string, from time.Time) ([]bankfile.Entry, error)

func (s *Service) fetcher(ctx context.Context, c Connection) (fetcher, error) {
	if _, ok := keyProviders[c.Provider]; ok {
		if s.keys == nil {
			return nil, ErrLinkOff
		}
		key, err := s.secrets.Open(c.Secret)
		if err != nil {
			return nil, fmt.Errorf("open api key: %w", err)
		}
		return func(ctx context.Context, uid string, from time.Time) ([]bankfile.Entry, error) {
			return s.keys.Transactions(ctx, c.Provider, key, uid, from)
		}, nil
	}
	l, err := s.link(ctx)
	if err != nil {
		return nil, err
	}
	return l.Transactions, nil
}

// sync fetches the transactions of the connection's accounts and books the
// new ones; what goes wrong is kept on the connection for the user to see.
func (s *Service) sync(ctx context.Context, c Connection) (Connection, error) {
	c = s.expire(c)
	if c.State != ConnectionActive {
		return c, nil
	}
	fetch, err := s.fetcher(ctx, c)
	if err != nil {
		return Connection{}, err
	}
	now := s.now()
	from := now.AddDate(0, 0, -firstHistory)
	if c.LastSyncAt != nil {
		from = c.LastSyncAt.Add(-syncOverlap)
	}
	from = time.Date(from.Year(), from.Month(), from.Day(), 0, 0, 0, 0, time.UTC)
	var problem error
	for _, la := range c.Accounts {
		if err := s.syncAccount(ctx, fetch, c, la, from); err != nil {
			problem = err
			break
		}
	}
	c.UpdatedAt = now
	switch {
	case errors.Is(problem, ErrLinkExpired):
		c.State, c.LastError = ConnectionExpired, ErrLinkExpired.Error()
	case errors.Is(problem, ErrLinkRateLimited):
		c.LastError = ErrLinkRateLimited.Error()
	case problem != nil:
		c.LastError = problem.Error()
	default:
		c.LastSyncAt, c.LastError = &now, ""
	}
	if err := s.connections.SaveConnection(ctx, c); err != nil {
		return Connection{}, err
	}
	return c, nil
}

func (s *Service) syncAccount(ctx context.Context, fetch fetcher, c Connection, la LinkedAccount, from time.Time) error {
	acc, err := s.store.Account(ctx, c.AdministrationID, la.AccountID)
	if errors.Is(err, ErrAccountNotFound) {
		return nil // The account was deleted; nothing to book on.
	}
	if err != nil {
		return err
	}
	entries, err := fetch(ctx, la.UID, from)
	if err != nil {
		return err
	}
	if len(entries) == 0 {
		return nil
	}
	st := bankfile.Statement{Account: acc.Identifier, Currency: acc.Currency, Reference: fmt.Sprintf("%s %s", c.Bank, s.now().Format(time.DateOnly)), Entries: entries}
	_, err = s.importStatements(ctx, c.AdministrationID, c.CreatedBy, acc, []bankfile.Statement{st}, bankfile.Format(c.Provider))
	return err
}

// Disconnect stops fetching through the connection and closes its consent;
// the transactions fetched stay.
func (s *Service) Disconnect(ctx context.Context, admin, user, id resource.ID) error {
	c, err := s.Connection(ctx, admin, user, id)
	if err != nil {
		return err
	}
	if c.SessionID != "" && c.Provider == ProviderEnableBanking {
		if l, err := s.link(ctx); err == nil {
			_ = l.Close(ctx, c.SessionID) // The bank may have closed it already.
		}
	}
	return s.connections.DeleteConnection(ctx, admin, id)
}

// Run fetches the transactions of due bank connections at every interval
// until the context ends.
func (s *Service) Run(ctx context.Context, interval time.Duration) {
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		s.SyncDue(ctx)
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}
