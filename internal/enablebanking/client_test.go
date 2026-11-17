package enablebanking

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

const appID = "00000000-0000-4000-8000-000000000001"

var now = time.Date(2026, 11, 17, 9, 0, 0, 0, time.UTC)

// fakeBank checks every request's token and answers from routes.
func fakeBank(t *testing.T, routes map[string]http.HandlerFunc) (*Client, *rsa.PrivateKey) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		checkToken(t, r, &key.PublicKey)
		h, ok := routes[r.Method+" "+r.URL.Path]
		if !ok {
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
			http.NotFound(w, r)
			return
		}
		h(w, r)
	}))
	t.Cleanup(srv.Close)
	return &Client{BaseURL: srv.URL, ApplicationID: appID, Key: key, HTTP: srv.Client(), Now: func() time.Time { return now }}, key
}

func checkToken(t *testing.T, r *http.Request, pub *rsa.PublicKey) {
	t.Helper()
	token, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
	parts := strings.Split(token, ".")
	if !ok || len(parts) != 3 {
		t.Fatalf("authorization = %q", r.Header.Get("Authorization"))
	}
	var header, claims map[string]any
	for i, v := range []*map[string]any{&header, &claims} {
		raw, err := base64.RawURLEncoding.DecodeString(parts[i])
		if err != nil || json.Unmarshal(raw, v) != nil {
			t.Fatalf("token part %d = %q", i, parts[i])
		}
	}
	if header["alg"] != "RS256" || header["typ"] != "JWT" || header["kid"] != appID {
		t.Errorf("header = %v", header)
	}
	if claims["iss"] != "enablebanking.com" || claims["aud"] != "api.enablebanking.com" || claims["iat"] != float64(now.Unix()) || claims["exp"] != float64(now.Add(time.Hour).Unix()) {
		t.Errorf("claims = %v", claims)
	}
	sig, _ := base64.RawURLEncoding.DecodeString(parts[2])
	sum := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
	if err := rsa.VerifyPKCS1v15(pub, crypto.SHA256, sum[:], sig); err != nil {
		t.Errorf("signature: %v", err)
	}
}

func TestBanks(t *testing.T) {
	c, _ := fakeBank(t, map[string]http.HandlerFunc{
		"GET /aspsps": func(w http.ResponseWriter, r *http.Request) {
			if q := r.URL.Query(); q.Get("country") != "NL" || q.Get("psu_type") != "business" || q.Get("service") != "AIS" {
				t.Errorf("query = %v", q)
			}
			_, _ = w.Write([]byte(`{"aspsps":[{"name":"Example Bank","country":"NL","logo":"https://example.com/logo.png","psu_types":["business","personal"],"maximum_consent_validity":15552000,"beta":false,"bic":"EXBKNL2A"}]}`))
		},
	})
	got, err := c.Banks(context.Background(), "NL")
	if err != nil || len(got) != 1 {
		t.Fatalf("Banks() = %+v, %v", got, err)
	}
	if b := got[0]; b.Name != "Example Bank" || b.Country != "NL" || b.Logo != "https://example.com/logo.png" || b.MaxConsent != 180*24*time.Hour {
		t.Errorf("bank = %+v", b)
	}
}

func TestAuthorizeAndSession(t *testing.T) {
	c, _ := fakeBank(t, map[string]http.HandlerFunc{
		"POST /auth": func(w http.ResponseWriter, r *http.Request) {
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			access, _ := body["access"].(map[string]any)
			aspsp, _ := body["aspsp"].(map[string]any)
			if access["valid_until"] != "2027-05-16T09:00:00Z" || aspsp["name"] != "Example Bank" || aspsp["country"] != "NL" || body["state"] != "s1" ||
				body["redirect_url"] != "https://books.example.com/bank_connection_callback" || body["psu_type"] != "business" {
				t.Errorf("auth body = %v", body)
			}
			_, _ = w.Write([]byte(`{"url":"https://bank.example.com/login?x=1","authorization_id":"a1"}`))
		},
		"POST /sessions": func(w http.ResponseWriter, r *http.Request) {
			var body map[string]string
			_ = json.NewDecoder(r.Body).Decode(&body)
			if body["code"] != "c1" {
				t.Errorf("session body = %v", body)
			}
			_, _ = w.Write([]byte(`{"session_id":"s-1","access":{"valid_until":"2027-05-16T09:00:00.000000+00:00"},"aspsp":{"name":"Example Bank","country":"NL"},
				"accounts":[{"uid":"u-1","account_id":{"iban":"NL91ABNA0417164300"},"name":"Business","currency":"EUR","identification_hash":"h1"},{"uid":"u-2","account_id":{"iban":null},"currency":"EUR"}]}`))
		},
		"DELETE /sessions/s-1": func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(`{"message":"OK"}`)) },
	})
	ctx := context.Background()
	url, err := c.Authorize(ctx, Authorization{Bank: "Example Bank", Country: "NL", State: "s1", RedirectURL: "https://books.example.com/bank_connection_callback", ValidUntil: now.Add(180 * 24 * time.Hour)})
	if err != nil || url != "https://bank.example.com/login?x=1" {
		t.Fatalf("Authorize() = %q, %v", url, err)
	}
	s, err := c.Session(ctx, "c1")
	if err != nil {
		t.Fatal(err)
	}
	if s.ID != "s-1" || !s.ValidUntil.Equal(now.Add(180*24*time.Hour)) || len(s.Accounts) != 2 || s.Accounts[0].UID != "u-1" || s.Accounts[0].IBAN != "NL91ABNA0417164300" || s.Accounts[0].Currency != "EUR" || s.Accounts[0].Name != "Business" || s.Accounts[1].IBAN != "" {
		t.Errorf("Session() = %+v", s)
	}
	if err := c.Close(ctx, "s-1"); err != nil {
		t.Errorf("Close() = %v", err)
	}
}

func TestTransactionsFollowsPagesAndReadsBooked(t *testing.T) {
	calls := 0
	c, _ := fakeBank(t, map[string]http.HandlerFunc{
		"GET /accounts/u-1/transactions": func(w http.ResponseWriter, r *http.Request) {
			calls++
			q := r.URL.Query()
			if q.Get("date_from") != "2026-11-01" || q.Get("transaction_status") != "BOOK" {
				t.Errorf("query = %v", q)
			}
			if q.Get("continuation_key") == "" {
				_, _ = w.Write([]byte(`{"transactions":[{"entry_reference":"E1","transaction_amount":{"amount":"250.00","currency":"EUR"},"credit_debit_indicator":"CRDT","status":"BOOK","booking_date":"2026-11-02","value_date":"2026-11-01",
					"debtor":{"name":"Example Client B.V."},"debtor_account":{"iban":"NL02ABNA0123456789"},"remittance_information":["Invoice 2026-0001","thanks"],"bank_transaction_code":{"code":"RCDT","sub_code":"ESCT"},"balance_after_transaction":{"amount":"1250.00","currency":"EUR"}}],"continuation_key":"k2"}`))
				return
			}
			if q.Get("continuation_key") != "k2" {
				t.Errorf("continuation_key = %q", q.Get("continuation_key"))
			}
			_, _ = w.Write([]byte(`{"transactions":[{"entry_reference":"E2","transaction_amount":{"amount":"25.50","currency":"EUR"},"credit_debit_indicator":"DBIT","status":"BOOK","booking_date":"2026-11-03",
				"creditor":{"name":"Hosting Company"},"creditor_account":{"iban":"NL44 RABO 0123 4567 89"},"remittance_information":["Monthly hosting plan"]},
				{"transaction_amount":{"amount":"1.00","currency":"EUR"},"credit_debit_indicator":"DBIT","status":"PDNG","value_date":"2026-11-04"}],"continuation_key":null}`))
		},
	})
	got, err := c.Transactions(context.Background(), "u-1", time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC))
	if err != nil || calls != 2 || len(got) != 2 {
		t.Fatalf("Transactions() = %+v, %v after %d calls", got, err, calls)
	}
	in, out := got[0], got[1]
	if in.Amount != 25000 || !in.Date.Equal(time.Date(2026, 11, 2, 0, 0, 0, 0, time.UTC)) || in.ValueDate == nil || in.ContraName != "Example Client B.V." || in.ContraAccount != "NL02ABNA0123456789" ||
		in.Message != "Invoice 2026-0001 thanks" || in.TransactionID != "E1" || in.Code != "RCDT ESCT" || in.Currency != "EUR" {
		t.Errorf("incoming = %+v", in)
	}
	if out.Amount != -2550 || out.ContraName != "Hosting Company" || out.ContraAccount != "NL44RABO0123456789" || out.Message != "Monthly hosting plan" {
		t.Errorf("outgoing = %+v", out)
	}
}

func TestErrors(t *testing.T) {
	c, _ := fakeBank(t, map[string]http.HandlerFunc{
		"GET /accounts/gone/transactions": func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"code":401,"message":"Session expired","error":"EXPIRED_SESSION"}`))
		},
		"GET /accounts/busy/transactions": func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = w.Write([]byte(`{"code":429,"message":"Too many","error":"ASPSP_RATE_LIMIT_EXCEEDED"}`))
		},
		"GET /accounts/broken/transactions": func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusBadGateway)
			_, _ = w.Write([]byte(`{"code":502,"message":"Bank unavailable","error":"ASPSP_ERROR"}`))
		},
	})
	ctx := context.Background()
	if _, err := c.Transactions(ctx, "gone", now); !errors.Is(err, ErrExpired) {
		t.Errorf("expired = %v", err)
	}
	if _, err := c.Transactions(ctx, "busy", now); !errors.Is(err, ErrRateLimited) {
		t.Errorf("rate limited = %v", err)
	}
	_, err := c.Transactions(ctx, "broken", now)
	if err == nil || errors.Is(err, ErrExpired) || !strings.Contains(err.Error(), "Bank unavailable") {
		t.Errorf("bank error = %v", err)
	}
}

func TestABadKeyIsNotAnExpiredConnection(t *testing.T) {
	c, _ := fakeBank(t, map[string]http.HandlerFunc{
		"GET /accounts/u-1/transactions": func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"code":401,"message":"Invalid token","error":"UNAUTHORIZED_ACCESS"}`))
		},
	})
	if _, err := c.Transactions(context.Background(), "u-1", now); err == nil || errors.Is(err, ErrExpired) {
		t.Errorf("bad key = %v, want a setup error", err)
	}
}
