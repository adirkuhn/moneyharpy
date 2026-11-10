package httpapi_test

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/adirkuhn/moneyharpy/internal/httpapi"
	"github.com/adirkuhn/moneyharpy/internal/postgres/postgrestest"
)

func connect(t *testing.T, owner *client, admin string, body map[string]any) httpapi.CreatedConnection {
	t.Helper()
	r := owner.do(http.MethodPost, "/api/v2/"+admin+"/connections", map[string]any{"connection": body})
	wantStatus(t, r, http.StatusCreated)
	return decode[httpapi.CreatedConnection](t, r)
}

func TestConnectionTokensAreScoped(t *testing.T) {
	h := newHarness(t, options{})
	owner := h.client()
	owner.register()
	a := owner.createAdministration("Harpy")
	other := owner.createAdministration("Other")
	c := connect(t, owner, a.Id, map[string]any{"kind": "api", "name": "Shop", "scopes": []string{"bank"}, "read_only": true})
	if !strings.HasPrefix(c.Token, "mhc_") || c.McpUrl != nil {
		t.Fatalf("connection = %+v", c)
	}
	app := h.client()
	app.token = c.Token
	api := "/api/v2/" + a.Id

	wantStatus(t, app.do(http.MethodGet, api+"/financial_mutations", nil), http.StatusOK)
	wantStatus(t, app.do(http.MethodGet, api+"/tax_rates", nil), http.StatusForbidden)
	wantStatus(t, app.do(http.MethodPost, api+"/financial_accounts", map[string]any{"financial_account": map[string]any{"name": "x"}}), http.StatusForbidden)
	wantStatus(t, app.do(http.MethodGet, "/api/v2/"+other.Id+"/financial_mutations", nil), http.StatusNotFound)
	wantStatus(t, app.do(http.MethodGet, "/api/user/tokens", nil), http.StatusForbidden)
	wantStatus(t, app.do(http.MethodPost, "/api/v2/administrations", map[string]any{"administration": map[string]string{"name": "x"}}), http.StatusForbidden)
	if list := decode[[]httpapi.Administration](t, app.do(http.MethodGet, "/api/v2/administrations", nil)); len(list) != 1 || list[0].Id != a.Id {
		t.Errorf("administrations seen = %+v", list)
	}
	wantStatus(t, app.do(http.MethodPost, api+"/connections", map[string]any{"connection": map[string]any{"kind": "api", "name": "x", "scopes": []string{"bank"}}}), http.StatusForbidden)

	writer := connect(t, owner, a.Id, map[string]any{"kind": "api", "name": "Writer", "scopes": []string{"settings"}})
	app.token = writer.Token
	wantStatus(t, app.do(http.MethodPost, api+"/financial_accounts", map[string]any{"financial_account": map[string]any{"name": "Via API"}}), http.StatusCreated)

	list := decode[[]httpapi.Connection](t, owner.do(http.MethodGet, api+"/connections", nil))
	if len(list) != 2 || list[0].LastUsedAt == nil || !list[0].Mine {
		t.Errorf("connections = %+v", list)
	}
	wantStatus(t, owner.do(http.MethodDelete, api+"/connections/"+writer.Id, nil), http.StatusNoContent)
	wantStatus(t, app.do(http.MethodGet, api+"/financial_accounts", nil), http.StatusUnauthorized)
	wantStatus(t, owner.do(http.MethodPost, api+"/connections", map[string]any{"connection": map[string]any{"kind": "api", "name": "", "scopes": []string{}}}), http.StatusUnprocessableEntity)
}

func TestOnlyOwnersConnectUnlessAllowed(t *testing.T) {
	h := newHarness(t, options{})
	owner, clerk := h.client(), h.client()
	owner.register()
	clerk.register()
	a := owner.createAdministration("Harpy")
	invite(t, h, owner, clerk, a.Id, "clerk@example.com", map[string]any{"user_type": "employee", "permissions": []string{"bank"}})
	api := "/api/v2/" + a.Id
	body := map[string]any{"connection": map[string]any{"kind": "ai", "name": "Assistant", "scopes": []string{"bank"}}}
	wantStatus(t, clerk.do(http.MethodPost, api+"/connections", body), http.StatusForbidden)
	wantStatus(t, clerk.do(http.MethodPatch, api+"/connection_settings", map[string]bool{"users_may_connect": true}), http.StatusForbidden)
	wantStatus(t, owner.do(http.MethodPatch, api+"/connection_settings", map[string]bool{"users_may_connect": true}), http.StatusOK)
	if st := decode[httpapi.ConnectionSettings](t, clerk.do(http.MethodGet, api+"/connection_settings", nil)); !st.UsersMayConnect {
		t.Error("setting not saved")
	}
	r := clerk.do(http.MethodPost, api+"/connections", body)
	wantStatus(t, r, http.StatusCreated)
	if c := decode[httpapi.CreatedConnection](t, r); c.McpUrl == nil || !strings.HasSuffix(*c.McpUrl, "/api/mcp") {
		t.Errorf("ai connection = %+v", c)
	}
	wantStatus(t, clerk.do(http.MethodPost, api+"/connections", map[string]any{"connection": map[string]any{"kind": "api", "name": "x", "scopes": []string{"settings"}}}), http.StatusUnprocessableEntity)
}

// receiver collects webhook deliveries.
type receiver struct {
	mu   sync.Mutex
	got  []http.Header
	body [][]byte
	srv  *httptest.Server
}

func newReceiver(t *testing.T) *receiver {
	rc := &receiver{}
	rc.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		rc.mu.Lock()
		rc.got, rc.body = append(rc.got, r.Header.Clone()), append(rc.body, b)
		rc.mu.Unlock()
	}))
	t.Cleanup(rc.srv.Close)
	return rc
}

func TestWebhooksDeliverSignedEvents(t *testing.T) {
	ctx := context.Background()
	pool, cleanup, err := postgrestest.Start(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()
	h := newHarness(t, options{pool: pool})
	owner := h.client()
	owner.register()
	a := owner.createAdministration("Harpy")
	api := "/api/v2/" + a.Id
	rc := newReceiver(t)

	body := map[string]any{"url": rc.srv.URL + "/hook", "enabled_events": []string{"tax_rate", "ledger_account_created"}}
	wantStatus(t, owner.do(http.MethodPost, api+"/webhooks", body), http.StatusUnprocessableEntity) // Local addresses are refused by default.
	wantStatus(t, owner.do(http.MethodPatch, "/api/instance/settings", map[string]bool{"webhooks_allow_local": true}), http.StatusOK)
	wantStatus(t, owner.do(http.MethodPost, api+"/webhooks", map[string]any{"url": rc.srv.URL, "enabled_events": []string{"nope"}}), http.StatusUnprocessableEntity)
	r := owner.do(http.MethodPost, api+"/webhooks", body)
	wantStatus(t, r, http.StatusCreated)
	var hook struct {
		ID, Secret, Token string
	}
	raw := decode[map[string]any](t, r)
	hook.ID, hook.Secret, hook.Token = raw["id"].(string), raw["secret"].(string), raw["token"].(string)
	if hook.Secret == "" || len(rc.body) != 1 || !strings.Contains(string(rc.body[0]), "test_webhook") {
		t.Fatalf("created %v, received %d", raw, len(rc.body))
	}
	if got := decode[map[string]any](t, owner.do(http.MethodGet, api+"/webhooks/"+hook.ID, nil)); got["secret"] != nil {
		t.Errorf("secret shown again: %v", got)
	}
	events := decode[httpapi.WebhookEvents](t, owner.do(http.MethodGet, api+"/webhooks/events", nil))
	if len(events.Events) == 0 || len(events.Groups) == 0 {
		t.Errorf("events = %+v", events)
	}

	wantStatus(t, owner.do(http.MethodPost, api+"/tax_rates", map[string]any{"tax_rate": map[string]any{"name": "Hooked", "percentage": "5", "tax_rate_type": "sales_invoice"}}), http.StatusCreated)
	wantStatus(t, owner.do(http.MethodPost, api+"/booking_rules", map[string]any{"booking_rule": map[string]any{"name": "x"}}), http.StatusUnprocessableEntity)
	if n, err := h.hooks.Deliver(ctx, 10); err != nil || n != 1 {
		t.Fatalf("Deliver() = %d, %v", n, err)
	}
	rc.mu.Lock()
	payload, header := rc.body[1], rc.got[1]
	rc.mu.Unlock()
	var p map[string]any
	if err := json.Unmarshal(payload, &p); err != nil {
		t.Fatal(err)
	}
	entity := p["entity"].(map[string]any)
	if p["action"] != "tax_rate_created" || p["entity_type"] != "TaxRate" || p["webhook_token"] != hook.Token || entity["name"] != "Hooked" || p["entity_id"] != entity["id"] {
		t.Errorf("payload = %v", p)
	}
	var stamp int64
	var sig string
	if _, err := fmt.Sscanf(strings.Replace(header.Get("Moneybird-Signature"), ",v1=", " ", 1), "t=%d %s", &stamp, &sig); err != nil {
		t.Fatalf("signature header %q: %v", header.Get("Moneybird-Signature"), err)
	}
	mac := hmac.New(sha256.New, []byte(hook.Secret))
	_, _ = fmt.Fprintf(mac, "%d.%s", stamp, payload)
	if hex.EncodeToString(mac.Sum(nil)) != sig || header.Get("Idempotency-Key") == "" {
		t.Errorf("signature %s does not verify", header.Get("Moneybird-Signature"))
	}
	got := decode[map[string]any](t, owner.do(http.MethodGet, api+"/webhooks/"+hook.ID, nil))
	if got["last_http_status"].(float64) != 200 {
		t.Errorf("last status = %v", got)
	}

	wantStatus(t, owner.do(http.MethodPatch, api+"/webhooks/"+hook.ID+"/deactivate", nil), http.StatusOK)
	wantStatus(t, owner.do(http.MethodPost, api+"/tax_rates", map[string]any{"tax_rate": map[string]any{"name": "Quiet", "percentage": "6", "tax_rate_type": "sales_invoice"}}), http.StatusCreated)
	if n, _ := h.hooks.Deliver(ctx, 10); n != 0 {
		t.Errorf("deactivated webhook got %d deliveries", n)
	}
	wantStatus(t, owner.do(http.MethodPatch, api+"/webhooks/"+hook.ID+"/activate", nil), http.StatusOK)
	if list := decode[[]map[string]any](t, owner.do(http.MethodGet, api+"/webhooks", nil)); len(list) != 1 || list[0]["deactivated_at"] != nil {
		t.Errorf("webhooks = %v", list)
	}
	wantStatus(t, owner.do(http.MethodDelete, api+"/webhooks/"+hook.ID, nil), http.StatusNoContent)
	wantStatus(t, owner.do(http.MethodGet, api+"/webhooks/"+hook.ID, nil), http.StatusNotFound)
}

func mcpCall(t *testing.T, c *client, method string, params any) map[string]any {
	t.Helper()
	r := c.do(http.MethodPost, "/api/mcp", map[string]any{"jsonrpc": "2.0", "id": 1, "method": method, "params": params})
	wantStatus(t, r, http.StatusOK)
	return decode[map[string]any](t, r)
}

func TestMCPToolsFollowTheConnection(t *testing.T) {
	h := newHarness(t, options{})
	owner := h.client()
	owner.register()
	a := owner.createAdministration("Harpy")
	ai := connect(t, owner, a.Id, map[string]any{"kind": "ai", "name": "Assistant", "scopes": []string{"settings", "bank"}, "read_only": true})
	api := connect(t, owner, a.Id, map[string]any{"kind": "api", "name": "Shop", "scopes": []string{"settings"}})

	app := h.client()
	app.token = api.Token
	wantStatus(t, app.do(http.MethodPost, "/api/mcp", map[string]any{"jsonrpc": "2.0", "id": 1, "method": "ping"}), http.StatusForbidden)
	wantStatus(t, owner.do(http.MethodPost, "/api/mcp", map[string]any{"jsonrpc": "2.0", "id": 1, "method": "ping"}), http.StatusForbidden)

	assistant := h.client()
	assistant.token = ai.Token
	init := mcpCall(t, assistant, "initialize", map[string]any{"protocolVersion": "2025-06-18", "capabilities": map[string]any{}, "clientInfo": map[string]any{"name": "test", "version": "1"}})
	if init["result"].(map[string]any)["protocolVersion"] != "2025-06-18" {
		t.Errorf("initialize = %v", init)
	}
	wantStatus(t, assistant.do(http.MethodPost, "/api/mcp", map[string]any{"jsonrpc": "2.0", "method": "notifications/initialized"}), http.StatusAccepted)

	tools := mcpCall(t, assistant, "tools/list", map[string]any{})["result"].(map[string]any)["tools"].([]any)
	names := map[string]bool{}
	for _, tool := range tools {
		names[tool.(map[string]any)["name"].(string)] = true
	}
	for _, want := range []string{"listTaxRates", "listFinancialMutations", "getBookkeepingSettings"} {
		if !names[want] {
			t.Errorf("tools lack %s", want)
		}
	}
	for _, unwanted := range []string{"createTaxRate", "deleteAdministration", "createConnection", "uploadDocumentStyleFile", "listUserInvitations"} {
		if names[unwanted] {
			t.Errorf("read-only connection offers %s", unwanted)
		}
	}

	res := mcpCall(t, assistant, "tools/call", map[string]any{"name": "listTaxRates", "arguments": map[string]any{"filter": "tax_rate_type:sales_invoice"}})["result"].(map[string]any)
	text := res["content"].([]any)[0].(map[string]any)["text"].(string)
	if res["isError"] != false || !strings.Contains(text, `"tax_rate_type":"sales_invoice"`) {
		t.Errorf("listTaxRates = %v", res)
	}
	res = mcpCall(t, assistant, "tools/call", map[string]any{"name": "getTaxRate", "arguments": map[string]any{"id": "1"}})["result"].(map[string]any)
	if res["isError"] != true {
		t.Errorf("unknown tax rate = %v", res)
	}
	if e := mcpCall(t, assistant, "tools/call", map[string]any{"name": "createTaxRate", "arguments": map[string]any{}})["error"]; e == nil {
		t.Error("read-only connection created a tax rate")
	}
}
