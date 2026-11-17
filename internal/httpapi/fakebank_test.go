package httpapi_test

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/adirkuhn/moneyharpy/internal/httpapi"
	"github.com/adirkuhn/moneyharpy/internal/postgres/postgrestest"
)

// fakeEnableBanking stands in for Enable Banking's API; tests never call
// the real one.
type fakeEnableBanking struct {
	server *httptest.Server
	mu     sync.Mutex
	// reject answers every call with this status and error code.
	rejectStatus int
	rejectCode   string
}

func newFakeEnableBanking(t *testing.T) *fakeEnableBanking {
	t.Helper()
	f := &fakeEnableBanking{}
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
