package httpapi_test

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/adirkuhn/moneyharpy/internal/httpapi"
	"github.com/adirkuhn/moneyharpy/internal/postgres/postgrestest"
	"github.com/adirkuhn/moneyharpy/internal/settings"
)

// fakeEnableBanking stands in for Enable Banking's API; tests never call
// the real one.
type fakeEnableBanking struct {
	server *httptest.Server
	mu     sync.Mutex
	// reject answers every call with this status and error code.
	rejectStatus int
	rejectCode   string
	// states holds the state of each authorisation, by the code the bank
	// sends the user back with.
	states map[string]string
	// iban is the account every session shares.
	iban   string
	closed []string
}

// codeFor returns the code the bank sends back for the authorisation that
// redirected to url.
func (f *fakeEnableBanking) codeFor(t *testing.T, url string) (state, code string) {
	t.Helper()
	f.mu.Lock()
	defer f.mu.Unlock()
	state = strings.TrimPrefix(url, "https://bank.example.com/login?state=")
	code = "code-" + state[:8]
	f.states[code] = state
	return state, code
}

func newFakeEnableBanking(t *testing.T) *fakeEnableBanking {
	t.Helper()
	f := &fakeEnableBanking{states: map[string]string{}, iban: "NL91ABNA0417164300"}
	f.server = httptest.NewServer(http.HandlerFunc(f.serve))
	t.Cleanup(f.server.Close)
	return f
}

func (f *fakeEnableBanking) serve(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	if !strings.HasPrefix(r.Header.Get("Authorization"), "Bearer ") {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"message":"No token","error":"UNAUTHORIZED_ACCESS"}`))
		return
	}
	if f.rejectStatus != 0 {
		w.WriteHeader(f.rejectStatus)
		_, _ = w.Write([]byte(`{"message":"Rejected by the fake","error":"` + f.rejectCode + `"}`))
		return
	}
	switch {
	case r.Method == http.MethodGet && r.URL.Path == "/aspsps":
		_, _ = w.Write([]byte(`{"aspsps":[{"name":"Example Bank","country":"NL","logo":"https://example.com/bank.png","psu_types":["business"],"maximum_consent_validity":15552000,"beta":false}]}`))
	case r.Method == http.MethodPost && r.URL.Path == "/auth":
		var body struct {
			State string `json:"state"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		_, _ = w.Write([]byte(`{"url":"https://bank.example.com/login?state=` + body.State + `","authorization_id":"a"}`))
	case r.Method == http.MethodPost && r.URL.Path == "/sessions":
		var body struct {
			Code string `json:"code"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		if _, ok := f.states[body.Code]; !ok {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"message":"Unknown code","error":"WRONG_AUTHORIZATION_CODE"}`))
			return
		}
		until := time.Now().Add(180 * 24 * time.Hour).UTC().Format(time.RFC3339)
		_, _ = w.Write([]byte(`{"session_id":"session-` + body.Code + `","access":{"valid_until":"` + until + `"},"accounts":[{"uid":"uid-1","account_id":{"iban":"` + f.iban + `"},"currency":"EUR","name":"Business"}]}`))
	case r.Method == http.MethodGet && r.URL.Path == "/accounts/uid-1/transactions":
		day := time.Now().AddDate(0, 0, -1).Format(time.DateOnly)
		_, _ = w.Write([]byte(`{"transactions":[{"entry_reference":"E1","transaction_amount":{"amount":"250.00","currency":"EUR"},"credit_debit_indicator":"CRDT","status":"BOOK","booking_date":"` + day + `",
			"debtor":{"name":"Example Client B.V."},"debtor_account":{"iban":"NL02ABNA0123456789"},"remittance_information":["Invoice 2026-0001"]}],"continuation_key":null}`))
	case r.Method == http.MethodDelete && strings.HasPrefix(r.URL.Path, "/sessions/"):
		f.closed = append(f.closed, strings.TrimPrefix(r.URL.Path, "/sessions/"))
		_, _ = w.Write([]byte(`{"message":"OK"}`))
	default:
		http.NotFound(w, r)
	}
}

// testRSAKey is a private key in PEM for the Enable Banking settings.
var testRSAKey = sync.OnceValue(func() string {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		panic(err)
	}
	return string(pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)}))
})

func TestBankConnectionSettingsAndCheck(t *testing.T) {
	ctx := context.Background()
	pool, cleanup, err := postgrestest.Start(ctx)
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	defer cleanup()
	h := newHarness(t, options{pool: pool})
	admin, member := h.client(), h.client()
	admin.register()
	member.register()

	got := decode[httpapi.InstanceSettings](t, admin.do(http.MethodGet, "/api/instance/settings", nil))
	if got.BankConnectionsEnabled || got.EnableBankingRedirectUrl == nil || *got.EnableBankingRedirectUrl != h.server.URL+"/bank_connection_callback" {
		t.Errorf("settings = %+v, want bank connections off and the redirect address", got)
	}
	const path = "/api/instance/settings/test_bank_connections"
	wantStatus(t, member.do(http.MethodPost, path, nil), http.StatusForbidden)
	wantStatus(t, admin.do(http.MethodPost, path, nil), http.StatusUnprocessableEntity)

	r := admin.do(http.MethodPatch, "/api/instance/settings", map[string]any{
		"bank_connections_enabled": true, "enable_banking_application_id": "00000000-0000-4000-8000-000000000001", "enable_banking_private_key": testRSAKey(),
	})
	wantStatus(t, r, http.StatusOK)
	if strings.Contains(string(r.body), "PRIVATE KEY") {
		t.Error("settings response contains the private key")
	}
	if got := decode[httpapi.InstanceSettings](t, r); !got.BankConnectionsEnabled || !got.EnableBankingPrivateKeySet {
		t.Errorf("settings = %+v", got)
	}
	wantStatus(t, admin.do(http.MethodPost, path, nil), http.StatusNoContent)

	h.bank.rejectStatus, h.bank.rejectCode = http.StatusUnauthorized, "UNAUTHORIZED_ACCESS"
	r = admin.do(http.MethodPost, path, nil)
	wantStatus(t, r, http.StatusBadGateway)
	if !strings.Contains(string(r.body), "Rejected by the fake") {
		t.Errorf("body = %s, want Enable Banking's reason", r.body)
	}
}

func TestBankConnectionFlow(t *testing.T) {
	// Its own database: the instance setting would reach other tests.
	pool, cleanup, err := postgrestest.Start(context.Background())
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	defer cleanup()
	h := newHarness(t, options{pool: pool})
	owner := h.client()
	owner.register()
	api := "/api/v2/" + owner.createAdministration("Example Shop B.V.").Id

	// Off until an instance administrator turns it on.
	r := owner.do(http.MethodGet, api+"/bank_connections/banks", nil)
	wantStatus(t, r, http.StatusUnprocessableEntity)
	h.enableBankLinks()

	banks := decode[[]httpapi.ConnectableBank](t, owner.do(http.MethodGet, api+"/bank_connections/banks?country=nl", nil))
	if len(banks) != 1 || banks[0].Name != "Example Bank" || banks[0].Logo == "" {
		t.Fatalf("banks = %+v", banks)
	}
	wantStatus(t, owner.do(http.MethodPost, api+"/bank_connections", map[string]string{"bank": "Other Bank", "country": "NL"}), http.StatusUnprocessableEntity)
	r = owner.do(http.MethodPost, api+"/bank_connections", map[string]string{"bank": "Example Bank", "country": "NL"})
	wantStatus(t, r, http.StatusCreated)
	started := decode[httpapi.BankAuthorization](t, r)
	state, code := h.bank.codeFor(t, started.AuthorizationUrl)

	// A pending connection is not listed until the bank sends the user back.
	if list := decode[[]httpapi.BankConnection](t, owner.do(http.MethodGet, api+"/bank_connections", nil)); len(list) != 0 {
		t.Errorf("listed before authorising: %+v", list)
	}
	wantStatus(t, owner.do(http.MethodPost, "/api/bank_connections/complete", map[string]string{"state": "wrong", "code": code}), http.StatusNotFound)
	r = owner.do(http.MethodPost, "/api/bank_connections/complete", map[string]string{"state": state, "code": code})
	wantStatus(t, r, http.StatusOK)
	c := decode[httpapi.BankConnection](t, r)
	if c.State != "active" || c.Bank != "Example Bank" || c.LastSyncAt == nil || c.LastError != "" || len(c.FinancialAccountIds) != 1 || c.NeedsReconnect {
		t.Fatalf("connection = %+v", c)
	}
	accounts := decode[[]struct {
		ID         string  `json:"id"`
		Identifier string  `json:"identifier"`
		Provider   *string `json:"provider"`
	}](t, owner.do(http.MethodGet, api+"/financial_accounts", nil))
	found := false
	for _, a := range accounts {
		if a.ID == c.FinancialAccountIds[0] && a.Identifier == "NL91ABNA0417164300" && a.Provider != nil && *a.Provider == "enable_banking" {
			found = true
		}
	}
	if !found {
		t.Errorf("accounts = %+v, want the connected account", accounts)
	}
	mutations := decode[[]struct {
		Message string `json:"message"`
	}](t, owner.do(http.MethodGet, api+"/financial_mutations?filter=financial_account_id:"+c.FinancialAccountIds[0], nil))
	if len(mutations) != 1 || mutations[0].Message != "Invoice 2026-0001" {
		t.Errorf("mutations = %+v", mutations)
	}

	r = owner.do(http.MethodPost, api+"/bank_connections/"+c.Id+"/sync", nil)
	wantStatus(t, r, http.StatusOK)
	if again := decode[httpapi.BankConnection](t, r); again.LastError != "" {
		t.Errorf("synced = %+v", again)
	}
	r = owner.do(http.MethodPost, api+"/bank_connections/"+c.Id+"/reconnect", nil)
	wantStatus(t, r, http.StatusOK)
	if re := decode[httpapi.BankAuthorization](t, r); re.BankConnectionId != c.Id || !strings.HasPrefix(re.AuthorizationUrl, "https://bank.example.com/") {
		t.Errorf("reconnect = %+v", re)
	}
	if list := decode[[]httpapi.BankConnection](t, owner.do(http.MethodGet, api+"/bank_connections", nil)); len(list) != 1 || list[0].Id != c.Id {
		t.Errorf("list = %+v", list)
	}

	// Another user can't finish someone's authorisation or see the connection.
	stranger := h.client()
	stranger.register()
	wantStatus(t, stranger.do(http.MethodGet, api+"/bank_connections", nil), http.StatusNotFound)

	wantStatus(t, owner.do(http.MethodDelete, api+"/bank_connections/"+c.Id, nil), http.StatusNoContent)
	if len(h.bank.closed) == 0 {
		t.Error("the session was not closed")
	}
	wantStatus(t, owner.do(http.MethodDelete, api+"/bank_connections/"+c.Id, nil), http.StatusNotFound)
}

// enableBankLinks turns bank connections on with a test application.
func (h *harness) enableBankLinks() {
	h.t.Helper()
	on, appID, key := true, "00000000-0000-4000-8000-000000000001", testRSAKey()
	if _, err := h.settle.Update(context.Background(), settings.Update{BankLinkEnabled: &on, BankLinkApplicationID: &appID, BankLinkPrivateKey: &key}); err != nil {
		h.t.Fatal(err)
	}
}
