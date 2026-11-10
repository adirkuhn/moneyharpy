package mcp

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
)

type box struct{ called map[string]any }

func (b *box) Tools(context.Context) []Tool {
	return []Tool{{Name: "listTaxRates", InputSchema: map[string]any{"type": "object"}, Annotations: Annotations{ReadOnly: true}}}
}

func (b *box) Call(_ context.Context, name string, args map[string]any) (Result, error) {
	b.called = args
	return Result{Text: `[{"id":"1"}]`}, nil
}

func call(t *testing.T, body string) (int, map[string]any) {
	t.Helper()
	status, out := Server{Name: "MoneyHarpy", Version: "1"}.Handle(context.Background(), []byte(body), &box{})
	if out == nil {
		return status, nil
	}
	raw, _ := json.Marshal(out)
	var m map[string]any
	_ = json.Unmarshal(raw, &m)
	return status, m
}

func TestInitializeNegotiatesTheVersion(t *testing.T) {
	_, m := call(t, `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-03-26"}}`)
	r := m["result"].(map[string]any)
	if r["protocolVersion"] != "2025-03-26" || r["serverInfo"].(map[string]any)["name"] != "MoneyHarpy" || m["id"].(float64) != 1 {
		t.Errorf("initialize = %v", m)
	}
	_, m = call(t, `{"jsonrpc":"2.0","id":"a","method":"initialize","params":{"protocolVersion":"1999-01-01"}}`)
	if m["result"].(map[string]any)["protocolVersion"] != Versions[0] || m["id"] != "a" {
		t.Errorf("unknown version = %v", m)
	}
}

func TestToolsListAndCall(t *testing.T) {
	_, m := call(t, `{"jsonrpc":"2.0","id":2,"method":"tools/list"}`)
	tools := m["result"].(map[string]any)["tools"].([]any)
	if len(tools) != 1 || tools[0].(map[string]any)["annotations"].(map[string]any)["readOnlyHint"] != true {
		t.Errorf("tools = %v", tools)
	}
	b := &box{}
	_, out := Server{}.Handle(context.Background(), []byte(`{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"listTaxRates","arguments":{"filter":"active:true"}}}`), b)
	raw, _ := json.Marshal(out)
	if b.called["filter"] != "active:true" || string(raw) != `{"id":3,"jsonrpc":"2.0","result":{"content":[{"text":"[{\"id\":\"1\"}]","type":"text"}],"isError":false}}` {
		t.Errorf("call = %s, args %v", raw, b.called)
	}
	_, m = call(t, `{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"deleteEverything"}}`)
	if m["error"].(map[string]any)["code"].(float64) != codeInvalidParams {
		t.Errorf("unknown tool = %v", m)
	}
}

func TestProtocolErrorsAndNotifications(t *testing.T) {
	if status, m := call(t, `{"jsonrpc":"2.0","method":"notifications/initialized"}`); status != http.StatusAccepted || m != nil {
		t.Errorf("notification = %d %v", status, m)
	}
	for body, code := range map[string]float64{
		`not json`: codeParse, `[{"jsonrpc":"2.0","id":1,"method":"ping"}]`: codeInvalidRequest,
		`{"id":1,"method":"ping"}`: codeInvalidRequest, `{"jsonrpc":"2.0","id":1,"method":"resources/list"}`: codeNoMethod,
	} {
		if _, m := call(t, body); m["error"].(map[string]any)["code"].(float64) != code {
			t.Errorf("%s = %v", body, m)
		}
	}
	if _, m := call(t, `{"jsonrpc":"2.0","id":5,"method":"ping"}`); m["result"] == nil {
		t.Errorf("ping = %v", m)
	}
}
