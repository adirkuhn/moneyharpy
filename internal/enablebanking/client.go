// Package enablebanking reads bank accounts and their transactions through
// Enable Banking's API, which reaches the banks under its PSD2 licence. An
// application registered in Enable Banking's control panel signs each
// request with its private key; in restricted mode it reads only the
// accounts linked to it there.
package enablebanking

import (
	"bytes"
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/adirkuhn/moneyharpy/internal/bankfile"
)

// DefaultBaseURL is Enable Banking's API.
const DefaultBaseURL = "https://api.enablebanking.com"

// Errors returned by Client.
var (
	// ErrExpired reports a consent the bank no longer honours: the user has
	// to authorise access again.
	ErrExpired = errors.New("the bank connection has expired")
	// ErrRateLimited reports the bank's limit on fetches without the user
	// present, usually four a day; fetch again hours later.
	ErrRateLimited = errors.New("the bank allows no more fetches for now")
)

// Client calls Enable Banking's API as one application.
type Client struct {
	BaseURL       string
	ApplicationID string
	Key           *rsa.PrivateKey
	HTTP          *http.Client
	Now           func() time.Time
}

// Bank is a bank, an ASPSP in PSD2's terms, that can be connected.
type Bank struct {
	Name    string
	Country string
	Logo    string
	// MaxConsent is the longest the bank honours a consent.
	MaxConsent time.Duration
}

// Authorization asks the user's bank for access to their accounts.
type Authorization struct {
	Bank, Country string
	// State comes back on the redirect, to find the connection again.
	State       string
	RedirectURL string
	ValidUntil  time.Time
}

// Session is an authorised access to the user's accounts.
type Session struct {
	ID         string
	ValidUntil time.Time
	Accounts   []Account
}

// Account is a bank account in a session.
type Account struct {
	// UID addresses the account in this session only.
	UID      string
	IBAN     string
	Currency string
	Name     string
}

// Banks lists the banks of the country that business accounts can connect to.
func (c *Client) Banks(ctx context.Context, country string) ([]Bank, error) {
	var out struct {
		ASPSPs []struct {
			Name       string   `json:"name"`
			Country    string   `json:"country"`
			Logo       string   `json:"logo"`
			PSUTypes   []string `json:"psu_types"`
			MaxConsent int64    `json:"maximum_consent_validity"`
		} `json:"aspsps"`
	}
	q := url.Values{"country": {country}, "psu_type": {"business"}, "service": {"AIS"}}
	if err := c.do(ctx, http.MethodGet, "/aspsps?"+q.Encode(), nil, &out); err != nil {
		return nil, err
	}
	banks := make([]Bank, 0, len(out.ASPSPs))
	for _, a := range out.ASPSPs {
		banks = append(banks, Bank{Name: a.Name, Country: a.Country, Logo: a.Logo, MaxConsent: time.Duration(a.MaxConsent) * time.Second})
	}
	return banks, nil
}

// Authorize starts the user's authorisation at their bank and returns the
// address to send them to.
func (c *Client) Authorize(ctx context.Context, a Authorization) (string, error) {
	body := map[string]any{
		"access":       map[string]any{"valid_until": a.ValidUntil.UTC().Format(time.RFC3339)},
		"aspsp":        map[string]string{"name": a.Bank, "country": a.Country},
		"state":        a.State,
		"redirect_url": a.RedirectURL,
		"psu_type":     "business",
	}
	var out struct {
		URL string `json:"url"`
	}
	if err := c.do(ctx, http.MethodPost, "/auth", body, &out); err != nil {
		return "", err
	}
	if out.URL == "" {
		return "", errors.New("enable banking: no authorisation address")
	}
	return out.URL, nil
}

// Session completes the authorisation with the code the bank redirected
// back with.
func (c *Client) Session(ctx context.Context, code string) (Session, error) {
	var out struct {
		SessionID string `json:"session_id"`
		Access    struct {
			ValidUntil time.Time `json:"valid_until"`
		} `json:"access"`
		Accounts []struct {
			UID       string `json:"uid"`
			AccountID struct {
				IBAN *string `json:"iban"`
			} `json:"account_id"`
			Name     string `json:"name"`
			Currency string `json:"currency"`
		} `json:"accounts"`
	}
	if err := c.do(ctx, http.MethodPost, "/sessions", map[string]string{"code": code}, &out); err != nil {
		return Session{}, err
	}
	s := Session{ID: out.SessionID, ValidUntil: out.Access.ValidUntil.UTC()}
	for _, a := range out.Accounts {
		iban := ""
		if a.AccountID.IBAN != nil {
			iban = strings.ToUpper(strings.ReplaceAll(*a.AccountID.IBAN, " ", ""))
		}
		s.Accounts = append(s.Accounts, Account{UID: a.UID, IBAN: iban, Currency: a.Currency, Name: a.Name})
	}
	return s, nil
}

// Close ends a session, closing the consent at the bank where it can.
func (c *Client) Close(ctx context.Context, sessionID string) error {
	return c.do(ctx, http.MethodDelete, "/sessions/"+url.PathEscape(sessionID), nil, nil)
}

type amount struct {
	Amount   string `json:"amount"`
	Currency string `json:"currency"`
}

type party struct {
	Name string `json:"name"`
}

type accountID struct {
	IBAN string `json:"iban"`
}

type transaction struct {
	EntryReference string    `json:"entry_reference"`
	Amount         amount    `json:"transaction_amount"`
	Indicator      string    `json:"credit_debit_indicator"`
	Status         string    `json:"status"`
	BookingDate    string    `json:"booking_date"`
	ValueDate      string    `json:"value_date"`
	TxDate         string    `json:"transaction_date"`
	Creditor       party     `json:"creditor"`
	CreditorAcc    accountID `json:"creditor_account"`
	Debtor         party     `json:"debtor"`
	DebtorAcc      accountID `json:"debtor_account"`
	Remittance     []string  `json:"remittance_information"`
	Code           struct {
		Code    string `json:"code"`
		SubCode string `json:"sub_code"`
	} `json:"bank_transaction_code"`
}

// Transactions returns the account's booked transactions from the day on,
// oldest first as the bank gives them, read as statement entries.
func (c *Client) Transactions(ctx context.Context, uid string, from time.Time) ([]bankfile.Entry, error) {
	var entries []bankfile.Entry
	key := ""
	for page := 0; ; page++ {
		if page > 1000 {
			return nil, errors.New("enable banking: too many pages of transactions")
		}
		q := url.Values{"date_from": {from.Format(time.DateOnly)}, "transaction_status": {"BOOK"}}
		if key != "" {
			q.Set("continuation_key", key)
		}
		var out struct {
			Transactions    []transaction `json:"transactions"`
			ContinuationKey *string       `json:"continuation_key"`
		}
		if err := c.do(ctx, http.MethodGet, "/accounts/"+url.PathEscape(uid)+"/transactions?"+q.Encode(), nil, &out); err != nil {
			return nil, err
		}
		for _, t := range out.Transactions {
			if t.Status != "" && t.Status != "BOOK" {
				continue
			}
			e, err := entry(t)
			if err != nil {
				return nil, err
			}
			entries = append(entries, e)
		}
		if out.ContinuationKey == nil || *out.ContinuationKey == "" {
			return entries, nil
		}
		key = *out.ContinuationKey
	}
}

func day(s string) (time.Time, bool) {
	t, err := time.Parse(time.DateOnly, s)
	return t, err == nil && s != ""
}

func entry(t transaction) (bankfile.Entry, error) {
	cents, err := bankfile.Cents(t.Amount.Amount)
	if err != nil {
		return bankfile.Entry{}, err
	}
	credit := t.Indicator != "DBIT"
	if !credit {
		cents = -cents
	}
	date, ok := day(t.BookingDate)
	if !ok {
		if date, ok = day(t.ValueDate); !ok {
			if date, ok = day(t.TxDate); !ok {
				return bankfile.Entry{}, fmt.Errorf("enable banking: transaction %q without a date", t.EntryReference)
			}
		}
	}
	e := bankfile.Entry{
		Date: date, Amount: cents, Currency: t.Amount.Currency, TransactionID: t.EntryReference,
		Code:    strings.TrimSpace(t.Code.Code + " " + t.Code.SubCode),
		Message: strings.Join(strings.Fields(strings.Join(t.Remittance, " ")), " "),
	}
	if v, ok := day(t.ValueDate); ok {
		e.ValueDate = &v
	}
	if credit {
		e.ContraName, e.ContraAccount = t.Debtor.Name, t.DebtorAcc.IBAN
	} else {
		e.ContraName, e.ContraAccount = t.Creditor.Name, t.CreditorAcc.IBAN
	}
	e.ContraName = strings.TrimSpace(e.ContraName)
	e.ContraAccount = strings.ToUpper(strings.ReplaceAll(e.ContraAccount, " ", ""))
	return e, nil
}

// token is the application's signed JWT, valid an hour.
func (c *Client) token() (string, error) {
	now := c.Now()
	enc := func(v any) string {
		raw, _ := json.Marshal(v)
		return base64.RawURLEncoding.EncodeToString(raw)
	}
	signing := enc(map[string]string{"typ": "JWT", "alg": "RS256", "kid": c.ApplicationID}) + "." +
		enc(map[string]any{"iss": "enablebanking.com", "aud": "api.enablebanking.com", "iat": now.Unix(), "exp": now.Add(time.Hour).Unix()})
	sum := sha256.Sum256([]byte(signing))
	sig, err := rsa.SignPKCS1v15(rand.Reader, c.Key, crypto.SHA256, sum[:])
	if err != nil {
		return "", fmt.Errorf("enable banking: sign token: %w", err)
	}
	return signing + "." + base64.RawURLEncoding.EncodeToString(sig), nil
}

func (c *Client) do(ctx context.Context, method, path string, body, out any) error {
	token, err := c.token()
	if err != nil {
		return err
	}
	var reader io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(raw)
	}
	req, err := http.NewRequestWithContext(ctx, method, strings.TrimRight(c.BaseURL, "/")+path, reader)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return fmt.Errorf("enable banking: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 20<<20))
	if err != nil {
		return fmt.Errorf("enable banking: %w", err)
	}
	if resp.StatusCode >= 300 {
		var problem struct {
			Message string `json:"message"`
			Error   string `json:"error"`
		}
		_ = json.Unmarshal(raw, &problem)
		switch {
		case strings.HasSuffix(problem.Error, "_SESSION"):
			// EXPIRED_SESSION, CLOSED_SESSION and the like: the consent is over.
			return fmt.Errorf("%w: %s", ErrExpired, problem.Message)
		case resp.StatusCode == http.StatusTooManyRequests || problem.Error == "ASPSP_RATE_LIMIT_EXCEEDED":
			return fmt.Errorf("%w: %s", ErrRateLimited, problem.Message)
		}
		msg := problem.Message
		if msg == "" {
			msg = http.StatusText(resp.StatusCode)
		}
		return fmt.Errorf("enable banking: %s (%d %s)", msg, resp.StatusCode, problem.Error)
	}
	if out == nil {
		return nil
	}
	if err := json.Unmarshal(raw, out); err != nil {
		return fmt.Errorf("enable banking: read answer: %w", err)
	}
	return nil
}
