package revolut

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/adirkuhn/moneyharpy/internal/banking"
)

const key = "test-api-key"

func fake(t *testing.T, routes map[string]http.HandlerFunc) *Client {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+key {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"code":9002,"message":"Unauthorized"}`))
			return
		}
		h, ok := routes[r.Method+" "+r.URL.Path]
		if !ok {
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		h(w, r)
	}))
	t.Cleanup(srv.Close)
	return &Client{BaseURL: srv.URL, Key: key, HTTP: srv.Client()}
}

func TestAccountsWithTheirIBAN(t *testing.T) {
	c := fake(t, map[string]http.HandlerFunc{
		"GET /accounts": func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte(`[{"id":"acc-eur","name":"Main","balance":1197.08,"currency":"EUR","state":"active","public":false,"account_type":"current","created_at":"2026-01-01T00:00:00Z","updated_at":"2026-11-17T00:00:00Z"},
				{"id":"acc-usd","name":"USD","balance":0,"currency":"USD","state":"active","public":false,"account_type":"current","created_at":"2026-01-01T00:00:00Z","updated_at":"2026-11-17T00:00:00Z"},
				{"id":"acc-old","name":"Old","balance":0,"currency":"EUR","state":"inactive","public":false,"account_type":"current","created_at":"2026-01-01T00:00:00Z","updated_at":"2026-11-17T00:00:00Z"}]`))
		},
		"GET /accounts/acc-eur/bank-details": func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte(`[{"account_no":"12345678","sort_code":"040004","schemes":["faster_payments"],"beneficiary":"Example Shop B.V.","beneficiary_address":{"country":"GB","postcode":"E14"},"estimated_time":{"unit":"hours"}},
				{"iban":"NL91 ABNA 0417 1643 00","bic":"REVONL22","schemes":["sepa"],"beneficiary":"Example Shop B.V.","beneficiary_address":{"country":"NL","postcode":"1000"},"estimated_time":{"unit":"days"}}]`))
		},
		"GET /accounts/acc-usd/bank-details": func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte(`[{"iban":"LT121000011101001000","bic":"REVOLT21","schemes":["swift"],"beneficiary":"Example Shop B.V.","beneficiary_address":{"country":"LT","postcode":"1"},"estimated_time":{"unit":"days"}}]`))
		},
	})
	got, err := c.Accounts(context.Background())
	if err != nil || len(got) != 2 {
		t.Fatalf("Accounts() = %+v, %v; want the two active ones", got, err)
	}
	if got[0].ID != "acc-eur" || got[0].IBAN != "NL91ABNA0417164300" || got[0].Currency != "EUR" || got[0].Name != "Main" {
		t.Errorf("EUR account = %+v, want its SEPA IBAN", got[0])
	}
	if got[1].IBAN != "LT121000011101001000" || got[1].Currency != "USD" {
		t.Errorf("USD account = %+v", got[1])
	}
}

func TestTransactionsPageBackAndReadTheAccountsLeg(t *testing.T) {
	from := time.Date(2026, 8, 19, 0, 0, 0, 0, time.UTC)
	calls := 0
	c := fake(t, map[string]http.HandlerFunc{
		"GET /transactions": func(w http.ResponseWriter, r *http.Request) {
			calls++
			q := r.URL.Query()
			if q.Get("account") != "acc-eur" || q.Get("from") != "2026-08-19T00:00:00Z" || q.Get("count") != "2" {
				t.Errorf("query = %v", q)
			}
			switch q.Get("to") {
			case "":
				_, _ = w.Write([]byte(`[
					{"id":"tx-3","type":"card_payment","state":"completed","created_at":"2026-11-02T10:00:00Z","updated_at":"2026-11-03T10:00:00Z","completed_at":"2026-11-03T10:00:00Z",
					 "merchant":{"name":"Example Cloud","city":"Dublin"},"legs":[{"leg_id":"l3","account_id":"acc-eur","amount":-22.27,"fee":0,"currency":"EUR","bill_amount":25.0,"bill_currency":"USD","description":"Example Cloud","balance":1197.08}]},
					{"id":"tx-2","type":"transfer","state":"completed","created_at":"2026-11-01T10:00:00Z","updated_at":"2026-11-02T10:00:00Z","completed_at":"2026-11-02T10:00:00Z",
					 "reference":"INV-2026-77","legs":[{"leg_id":"l2","account_id":"acc-eur","amount":-25.5,"fee":0.2,"currency":"EUR","description":"To Hosting Company","balance":1219.35,"counterparty":{"account_type":"external","id":"cp-1"}}]}]`))
			case "2026-11-01T10:00:00Z":
				_, _ = w.Write([]byte(`[
					{"id":"tx-1","type":"transfer","state":"completed","created_at":"2026-10-31T10:00:00Z","updated_at":"2026-11-01T10:00:00Z","completed_at":"2026-11-01T10:00:00Z",
					 "reference":"Invoice 2026-0001","legs":[{"leg_id":"l1","account_id":"acc-eur","amount":250,"currency":"EUR","description":"Payment from Example Client B.V.","balance":1245.05}]},
					{"id":"tx-0","type":"exchange","state":"completed","created_at":"2026-10-30T10:00:00Z","updated_at":"2026-10-30T10:00:00Z","completed_at":"2026-10-30T10:00:00Z",
					 "legs":[{"leg_id":"a","account_id":"acc-usd","amount":-10,"currency":"USD","description":"Exchanged to EUR"},{"leg_id":"b","account_id":"acc-eur","amount":9.1,"currency":"EUR","description":"Exchanged from USD","balance":995.05}]}]`))
			case "2026-10-30T10:00:00Z":
				_, _ = w.Write([]byte(`[{"id":"tx-p","type":"card_payment","state":"pending","created_at":"2026-10-29T10:00:00Z","updated_at":"2026-10-29T10:00:00Z","legs":[{"leg_id":"p","account_id":"acc-eur","amount":-1,"currency":"EUR","description":"Pending"}]}]`))
			default:
				t.Errorf("to = %q", q.Get("to"))
				_, _ = w.Write([]byte(`[]`))
			}
		},
	})
	c.PageSize = 2
	got, err := c.Transactions(context.Background(), "acc-eur", from)
	if err != nil || calls != 3 || len(got) != 4 {
		t.Fatalf("Transactions() = %+v, %v after %d calls; want the four completed ones", got, err, calls)
	}
	// Oldest first.
	exchange, in, out, card := got[0], got[1], got[2], got[3]
	if exchange.Amount != 910 || exchange.Message != "Exchanged from USD" || exchange.TransactionID != "tx-0" {
		t.Errorf("exchange = %+v, want this account's leg", exchange)
	}
	if in.Amount != 25000 || in.ContraName != "Example Client B.V." || in.Message != "Invoice 2026-0001" || !in.Date.Equal(time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)) || in.ValueDate == nil || in.Code != "TRANSFER" {
		t.Errorf("incoming = %+v", in)
	}
	if out.Amount != -2570 || out.ContraName != "Hosting Company" || out.Message != "INV-2026-77" {
		t.Errorf("outgoing = %+v, want the fee in the amount", out)
	}
	if card.Amount != -2227 || card.ContraName != "Example Cloud" || card.Message != "Example Cloud USD 25.00" || card.Currency != "EUR" {
		t.Errorf("card = %+v", card)
	}
}

func TestErrors(t *testing.T) {
	c := fake(t, map[string]http.HandlerFunc{
		"GET /transactions": func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = w.Write([]byte(`{"message":"Too many requests"}`))
		},
	})
	if _, err := c.Transactions(context.Background(), "acc", time.Now()); !errors.Is(err, ErrRateLimited) {
		t.Errorf("rate limited = %v", err)
	}
	c.Key = "wrong"
	if _, err := c.Accounts(context.Background()); !errors.Is(err, ErrKeyRejected) {
		t.Errorf("wrong key = %v", err)
	}
}

func TestLinkSpeaksTheBankingServicesErrors(t *testing.T) {
	c := fake(t, map[string]http.HandlerFunc{
		"GET /accounts": func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(`[]`)) },
	})
	l := Link{BaseURL: c.BaseURL, HTTP: c.HTTP}
	ctx := context.Background()
	if _, err := l.Accounts(ctx, banking.ProviderRevolut, key); err != nil {
		t.Errorf("Accounts() = %v", err)
	}
	if _, err := l.Accounts(ctx, banking.ProviderRevolut, "revoked"); !errors.Is(err, banking.ErrLinkExpired) {
		t.Errorf("revoked key = %v", err)
	}
}
