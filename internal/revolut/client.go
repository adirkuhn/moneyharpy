// Package revolut reads the accounts and transactions of a Revolut Business
// account through its Business API, with a read-only API key the account
// holder creates in Revolut Business's API Console.
package revolut

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/adirkuhn/moneyharpy/internal/bankfile"
)

// DefaultBaseURL is the Business API in production.
const DefaultBaseURL = "https://b2b.revolut.com/api/1.0"

// Errors returned by Client.
var (
	// ErrKeyRejected reports an API key Revolut does not accept: revoked,
	// expired, without the permissions or needing request signing.
	ErrKeyRejected = errors.New("the API key was not accepted by Revolut")
	// ErrRateLimited reports Revolut's limit on requests.
	ErrRateLimited = errors.New("the Revolut API allows no more requests for now")
)

// Client calls the Business API with an API key.
type Client struct {
	BaseURL string
	Key     string
	HTTP    *http.Client
	// PageSize is the transactions asked per request; 1000, the most, when 0.
	PageSize int
}

// Account is an active account with its IBAN, if it has one.
type Account struct {
	ID, Name, Currency, IBAN string
}

// Accounts lists the active accounts with the IBAN they receive SEPA
// transfers on, or else any IBAN they have.
func (c *Client) Accounts(ctx context.Context) ([]Account, error) {
	var list []struct {
		ID       string `json:"id"`
		Name     string `json:"name"`
		Currency string `json:"currency"`
		State    string `json:"state"`
	}
	if err := c.get(ctx, "/accounts", &list); err != nil {
		return nil, err
	}
	var out []Account
	for _, a := range list {
		if a.State != "active" {
			continue
		}
		var details []struct {
			IBAN    string   `json:"iban"`
			Schemes []string `json:"schemes"`
		}
		if err := c.get(ctx, "/accounts/"+url.PathEscape(a.ID)+"/bank-details", &details); err != nil {
			return nil, err
		}
		iban := ""
		for _, d := range details {
			if d.IBAN == "" {
				continue
			}
			if iban == "" || slices.Contains(d.Schemes, "sepa") {
				iban = strings.ToUpper(strings.ReplaceAll(d.IBAN, " ", ""))
			}
			if slices.Contains(d.Schemes, "sepa") {
				break
			}
		}
		out = append(out, Account{ID: a.ID, Name: strings.TrimSpace(a.Name), Currency: a.Currency, IBAN: iban})
	}
	return out, nil
}

type leg struct {
	AccountID    string      `json:"account_id"`
	Amount       json.Number `json:"amount"`
	Fee          json.Number `json:"fee"`
	Currency     string      `json:"currency"`
	BillAmount   json.Number `json:"bill_amount"`
	BillCurrency string      `json:"bill_currency"`
	Description  string      `json:"description"`
}

type transaction struct {
	ID          string `json:"id"`
	Type        string `json:"type"`
	State       string `json:"state"`
	CreatedAt   string `json:"created_at"`
	CompletedAt string `json:"completed_at"`
	Reference   string `json:"reference"`
	Merchant    *struct {
		Name string `json:"name"`
	} `json:"merchant"`
	Legs []leg `json:"legs"`
}

// Transactions returns the account's completed transactions created from
// the time on, oldest first, read as statement entries.
func (c *Client) Transactions(ctx context.Context, accountID string, from time.Time) ([]bankfile.Entry, error) {
	size := c.PageSize
	if size <= 0 {
		size = 1000
	}
	var entries []bankfile.Entry
	to := ""
	for page := 0; page < 10000; page++ {
		q := url.Values{"account": {accountID}, "from": {from.UTC().Format(time.RFC3339)}, "count": {strconv.Itoa(size)}}
		if to != "" {
			q.Set("to", to)
		}
		var list []transaction
		if err := c.get(ctx, "/transactions?"+q.Encode(), &list); err != nil {
			return nil, err
		}
		for _, t := range list {
			if t.State != "completed" {
				continue
			}
			e, ok, err := entry(t, accountID)
			if err != nil {
				return nil, err
			}
			if ok {
				entries = append(entries, e)
			}
		}
		// Newest first: the next page ends where this one did.
		if len(list) < size || list[len(list)-1].CreatedAt == to {
			break
		}
		to = list[len(list)-1].CreatedAt
	}
	slices.Reverse(entries)
	slices.SortStableFunc(entries, func(a, b bankfile.Entry) int { return a.Date.Compare(b.Date) })
	return entries, nil
}

func day(s string) (time.Time, bool) {
	t, err := time.Parse(time.RFC3339Nano, s)
	if err != nil {
		return time.Time{}, false
	}
	t = t.UTC()
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC), true
}

// cents reads a signed decimal amount in cents.
func cents(n json.Number) (int64, error) {
	if n == "" {
		return 0, nil
	}
	f, err := n.Float64()
	if err != nil || math.Abs(f) > 1e12 {
		return 0, fmt.Errorf("revolut: amount %q", n)
	}
	return int64(math.Round(f * 100)), nil
}

// counterparty takes the name out of the descriptions Revolut gives
// transfers, such as "To Hosting Company" and "Payment from Example B.V.".
func counterparty(description string) string {
	for _, prefix := range []string{"Payment from ", "Money added from ", "Transfer from ", "Transfer to ", "From ", "To "} {
		if rest, ok := strings.CutPrefix(description, prefix); ok {
			return strings.TrimSpace(rest)
		}
	}
	return ""
}

// entry reads the account's leg of a transaction; an exchange has a leg on
// each of its accounts.
func entry(t transaction, accountID string) (bankfile.Entry, bool, error) {
	i := slices.IndexFunc(t.Legs, func(l leg) bool { return l.AccountID == accountID })
	if i < 0 {
		return bankfile.Entry{}, false, nil
	}
	l := t.Legs[i]
	amount, err := cents(l.Amount)
	if err != nil {
		return bankfile.Entry{}, false, err
	}
	fee, err := cents(l.Fee)
	if err != nil {
		return bankfile.Entry{}, false, err
	}
	// What the transaction did to the balance: the fee is paid on top.
	amount -= int64(math.Abs(float64(fee)))
	date, ok := day(t.CompletedAt)
	if !ok {
		if date, ok = day(t.CreatedAt); !ok {
			return bankfile.Entry{}, false, fmt.Errorf("revolut: transaction %s without a date", t.ID)
		}
	}
	e := bankfile.Entry{Date: date, Amount: amount, Currency: l.Currency, TransactionID: t.ID, Code: strings.ToUpper(t.Type)}
	if created, ok := day(t.CreatedAt); ok {
		e.ValueDate = &created
	}
	e.Message = cmpOr(strings.TrimSpace(t.Reference), strings.TrimSpace(l.Description))
	if t.Merchant != nil && t.Merchant.Name != "" {
		e.ContraName = strings.TrimSpace(t.Merchant.Name)
	} else {
		e.ContraName = counterparty(strings.TrimSpace(l.Description))
	}
	// A payment in another currency says what it was in that currency.
	if l.BillCurrency != "" && l.BillCurrency != l.Currency && l.BillAmount != "" {
		if f, err := l.BillAmount.Float64(); err == nil {
			e.Message += fmt.Sprintf(" %s %.2f", l.BillCurrency, math.Abs(f))
		}
	}
	e.Message = strings.Join(strings.Fields(e.Message), " ")
	return e, true, nil
}

func cmpOr(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

func (c *Client) get(ctx context.Context, path string, out any) error {
	base := c.BaseURL
	if base == "" {
		base = DefaultBaseURL
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(base, "/")+path, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.Key)
	req.Header.Set("Accept", "application/json")
	httpClient := c.HTTP
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 60 * time.Second}
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("revolut: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 50<<20))
	if err != nil {
		return fmt.Errorf("revolut: %w", err)
	}
	if resp.StatusCode >= 300 {
		var problem struct {
			Message string `json:"message"`
		}
		_ = json.Unmarshal(raw, &problem)
		msg := cmpOr(problem.Message, http.StatusText(resp.StatusCode))
		switch resp.StatusCode {
		case http.StatusUnauthorized, http.StatusForbidden:
			return fmt.Errorf("%w: %s", ErrKeyRejected, msg)
		case http.StatusTooManyRequests:
			return fmt.Errorf("%w: %s", ErrRateLimited, msg)
		}
		return fmt.Errorf("revolut: %s (%d)", msg, resp.StatusCode)
	}
	dec := json.NewDecoder(strings.NewReader(string(raw)))
	dec.UseNumber()
	if err := dec.Decode(out); err != nil {
		return fmt.Errorf("revolut: read answer: %w", err)
	}
	return nil
}
