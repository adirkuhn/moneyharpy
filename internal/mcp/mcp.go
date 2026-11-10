// Package mcp serves the Model Context Protocol over streamable HTTP, so AI
// assistants can use an administration through tools. It is stateless: each
// POST carries one JSON-RPC message and gets one JSON answer.
package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"slices"
)

// Versions are the protocol versions served, newest first.
var Versions = []string{"2025-06-18", "2025-03-26", "2024-11-05"}

// Tool is one action an assistant can take.
type Tool struct {
	Name        string         `json:"name"`
	Title       string         `json:"title,omitempty"`
	Description string         `json:"description,omitempty"`
	InputSchema map[string]any `json:"inputSchema"`
	Annotations Annotations    `json:"annotations"`
}

// Annotations hint how a tool behaves.
type Annotations struct {
	ReadOnly    bool `json:"readOnlyHint"`
	Destructive bool `json:"destructiveHint"`
}

// Result is what a tool call returned.
type Result struct {
	Text    string
	IsError bool
}

// Toolbox gives the tools of one caller and runs them.
type Toolbox interface {
	Tools(ctx context.Context) []Tool
	Call(ctx context.Context, name string, args map[string]any) (Result, error)
}

// Server answers MCP messages.
type Server struct {
	Name, Version, Instructions string
}

type message struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// JSON-RPC error codes.
const (
	codeParse          = -32700
	codeInvalidRequest = -32600
	codeNoMethod       = -32601
	codeInvalidParams  = -32602
	codeInternal       = -32603
)

// Handle answers one message. A notification gets 202 and no body.
func (s Server) Handle(ctx context.Context, body []byte, box Toolbox) (int, any) {
	var m message
	if len(body) > 0 && body[0] == '[' {
		return http.StatusOK, reply(nil, nil, &rpcError{codeInvalidRequest, "batches are not supported"})
	}
	if err := json.Unmarshal(body, &m); err != nil {
		return http.StatusOK, reply(nil, nil, &rpcError{codeParse, "the message is not JSON"})
	}
	if m.JSONRPC != "2.0" || m.Method == "" {
		return http.StatusOK, reply(m.ID, nil, &rpcError{codeInvalidRequest, "not a JSON-RPC 2.0 request"})
	}
	if len(m.ID) == 0 || string(m.ID) == "null" {
		return http.StatusAccepted, nil // Notifications need no answer.
	}
	switch m.Method {
	case "initialize":
		var p struct {
			ProtocolVersion string `json:"protocolVersion"`
		}
		_ = json.Unmarshal(m.Params, &p)
		version := Versions[0]
		if slices.Contains(Versions, p.ProtocolVersion) {
			version = p.ProtocolVersion
		}
		return http.StatusOK, reply(m.ID, map[string]any{
			"protocolVersion": version,
			"capabilities":    map[string]any{"tools": map[string]any{"listChanged": false}},
			"serverInfo":      map[string]any{"name": s.Name, "version": s.Version},
			"instructions":    s.Instructions,
		}, nil)
	case "ping":
		return http.StatusOK, reply(m.ID, map[string]any{}, nil)
	case "tools/list":
		return http.StatusOK, reply(m.ID, map[string]any{"tools": box.Tools(ctx)}, nil)
	case "tools/call":
		var p struct {
			Name      string         `json:"name"`
			Arguments map[string]any `json:"arguments"`
		}
		if err := json.Unmarshal(m.Params, &p); err != nil || p.Name == "" {
			return http.StatusOK, reply(m.ID, nil, &rpcError{codeInvalidParams, "tools/call needs a tool name"})
		}
		if !slices.ContainsFunc(box.Tools(ctx), func(t Tool) bool { return t.Name == p.Name }) {
			return http.StatusOK, reply(m.ID, nil, &rpcError{codeInvalidParams, fmt.Sprintf("unknown tool %q", p.Name)})
		}
		r, err := box.Call(ctx, p.Name, p.Arguments)
		if err != nil {
			return http.StatusOK, reply(m.ID, nil, &rpcError{codeInternal, "the tool failed"})
		}
		return http.StatusOK, reply(m.ID, map[string]any{
			"content": []map[string]any{{"type": "text", "text": r.Text}},
			"isError": r.IsError,
		}, nil)
	}
	return http.StatusOK, reply(m.ID, nil, &rpcError{codeNoMethod, fmt.Sprintf("method %q is not supported", m.Method)})
}

func reply(id json.RawMessage, result any, e *rpcError) map[string]any {
	out := map[string]any{"jsonrpc": "2.0", "id": json.RawMessage("null")}
	if len(id) > 0 {
		out["id"] = id
	}
	if e != nil {
		out["error"] = e
	} else {
		out["result"] = result
	}
	return out
}
