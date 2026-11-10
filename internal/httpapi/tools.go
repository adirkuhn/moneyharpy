package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"sort"
	"strings"

	"github.com/getkin/kin-openapi/openapi3"

	"github.com/adirkuhn/moneyharpy/internal/administration"
	"github.com/adirkuhn/moneyharpy/internal/connections"
	"github.com/adirkuhn/moneyharpy/internal/mcp"
)

// operationTool is an API operation offered to AI assistants as an MCP tool.
type operationTool struct {
	mcp.Tool
	method, path string
	pathParams   []string
	queryParams  []string
	hasBody      bool
	perms        []administration.Permission
}

// maxToolOutput caps what a tool hands back to the assistant.
const maxToolOutput = 100 << 10

// buildTools turns the contract's JSON operations on one administration into
// tools. Ownership, file uploads and connection management stay out of
// reach of assistants.
func buildTools(spec *openapi3.T, rules map[string][]administration.Permission) []operationTool {
	skip := []string{"createConnection", "revokeConnection", "updateConnectionSettings", "leaveAdministration", "favoriteAdministration", "unfavoriteAdministration"}
	var out []operationTool
	for path, item := range spec.Paths.Map() {
		for method, op := range item.Operations() {
			perms, ruled := rules[op.OperationID]
			if !ruled || slices.Contains(perms, administration.Ownership) || slices.Contains(skip, op.OperationID) {
				continue
			}
			if !strings.HasPrefix(path, "/api/v2/{administration_id}/") && op.OperationID != "getAdministration" && op.OperationID != "updateAdministration" {
				continue
			}
			if op.RequestBody != nil && op.RequestBody.Value.Content.Get("application/json") == nil {
				continue // Files go through the API, not tools.
			}
			if !jsonResponses(op) {
				continue
			}
			t := operationTool{method: method, path: path, perms: perms}
			props := map[string]any{}
			required := []string{}
			for _, p := range append(item.Parameters, op.Parameters...) {
				v := p.Value
				if v.Name == "administration_id" || (v.Name == "id" && strings.HasPrefix(path, "/api/v2/administrations/")) {
					continue
				}
				props[v.Name] = schemaJSON(v.Schema, 0)
				switch v.In {
				case "path":
					t.pathParams = append(t.pathParams, v.Name)
					required = append(required, v.Name)
				case "query":
					t.queryParams = append(t.queryParams, v.Name)
				}
				if d, ok := props[v.Name].(map[string]any); ok && v.Description != "" {
					d["description"] = v.Description
				}
			}
			if op.RequestBody != nil {
				t.hasBody = true
				props["body"] = schemaJSON(op.RequestBody.Value.Content.Get("application/json").Schema, 0)
				if op.RequestBody.Value.Required {
					required = append(required, "body")
				}
			}
			sort.Strings(required)
			schema := map[string]any{"type": "object", "properties": props, "additionalProperties": false}
			if len(required) > 0 {
				schema["required"] = required
			}
			t.Tool = mcp.Tool{
				Name: op.OperationID, Title: op.Summary, Description: strings.TrimSpace(op.Summary + ". " + op.Description), InputSchema: schema,
				Annotations: mcp.Annotations{ReadOnly: method == http.MethodGet, Destructive: method == http.MethodDelete},
			}
			out = append(out, t)
		}
	}
	slices.SortFunc(out, func(a, b operationTool) int { return strings.Compare(a.Name, b.Name) })
	return out
}

func jsonResponses(op *openapi3.Operation) bool {
	for code, r := range op.Responses.Map() {
		if strings.HasPrefix(code, "2") && r.Value != nil && len(r.Value.Content) > 0 && r.Value.Content.Get("application/json") == nil {
			return false
		}
	}
	return true
}

// schemaJSON spells a contract schema as plain JSON Schema, with references
// resolved, for assistants.
func schemaJSON(ref *openapi3.SchemaRef, depth int) any {
	if ref == nil || ref.Value == nil || depth > 8 {
		return map[string]any{}
	}
	v := ref.Value
	out := map[string]any{}
	if v.Type != nil && len(*v.Type) > 0 {
		types := []string(*v.Type)
		if v.Nullable {
			types = append(types, "null")
		}
		if len(types) == 1 {
			out["type"] = types[0]
		} else {
			out["type"] = types
		}
	}
	set := func(k string, val any, ok bool) {
		if ok {
			out[k] = val
		}
	}
	set("description", v.Description, v.Description != "")
	set("format", v.Format, v.Format != "")
	set("enum", v.Enum, len(v.Enum) > 0)
	set("pattern", v.Pattern, v.Pattern != "")
	set("maxLength", v.MaxLength, v.MaxLength != nil)
	set("minimum", v.Min, v.Min != nil)
	set("maximum", v.Max, v.Max != nil)
	if v.Items != nil {
		out["items"] = schemaJSON(v.Items, depth+1)
	}
	if len(v.Properties) > 0 {
		props := map[string]any{}
		for k, p := range v.Properties {
			if p.Value != nil && p.Value.ReadOnly {
				continue
			}
			props[k] = schemaJSON(p, depth+1)
		}
		out["properties"] = props
	}
	set("required", v.Required, len(v.Required) > 0)
	for k, list := range map[string]openapi3.SchemaRefs{"allOf": v.AllOf, "oneOf": v.OneOf, "anyOf": v.AnyOf} {
		if len(list) > 0 {
			parts := make([]any, len(list))
			for i, p := range list {
				parts[i] = schemaJSON(p, depth+1)
			}
			out[k] = parts
		}
	}
	return out
}

// toolbox offers one AI connection the tools its scopes allow, and runs them
// as API requests with the connection's own token.
type toolbox struct {
	s     *server
	grant connections.Grant
	req   *http.Request
}

func (b *toolbox) allowed(t operationTool) bool {
	return b.grant.Allows(t.perms...) && (!b.grant.ReadOnly || t.method == http.MethodGet)
}

func (b *toolbox) Tools(context.Context) []mcp.Tool {
	var out []mcp.Tool
	for _, t := range b.s.tools {
		if b.allowed(t) {
			out = append(out, t.Tool)
		}
	}
	return out
}

func (b *toolbox) Call(ctx context.Context, name string, args map[string]any) (mcp.Result, error) {
	i := slices.IndexFunc(b.s.tools, func(t operationTool) bool { return t.Name == name })
	if i < 0 || !b.allowed(b.s.tools[i]) {
		return mcp.Result{Text: "unknown tool", IsError: true}, nil
	}
	t := b.s.tools[i]
	path := strings.ReplaceAll(t.path, "{administration_id}", b.grant.AdministrationID.String())
	if strings.HasPrefix(t.path, "/api/v2/administrations/{id}") {
		path = strings.ReplaceAll(path, "{id}", b.grant.AdministrationID.String())
	}
	for _, p := range t.pathParams {
		v, ok := args[p]
		if !ok {
			return mcp.Result{Text: "missing " + p, IsError: true}, nil
		}
		path = strings.ReplaceAll(path, "{"+p+"}", url.PathEscape(fmt.Sprint(v)))
	}
	q := url.Values{}
	for _, p := range t.queryParams {
		if v, ok := args[p]; ok {
			q.Set(p, fmt.Sprint(v))
		}
	}
	if len(q) > 0 {
		path += "?" + q.Encode()
	}
	var body *bytes.Reader
	if t.hasBody {
		raw, err := json.Marshal(args["body"])
		if err != nil {
			return mcp.Result{Text: "the body is not JSON", IsError: true}, nil
		}
		body = bytes.NewReader(raw)
	} else {
		body = bytes.NewReader(nil)
	}
	// A fresh context: the outer one carries the router's state for the MCP
	// request itself. It still ends when the outer request does.
	fresh, cancel := context.WithCancel(context.Background())
	defer cancel()
	stop := context.AfterFunc(ctx, cancel)
	defer stop()
	inner := httptest.NewRequestWithContext(fresh, t.method, path, body)
	inner.RemoteAddr = b.req.RemoteAddr
	inner.Header.Set("Authorization", b.req.Header.Get("Authorization"))
	inner.Header.Set("Accept", "application/json")
	if t.hasBody {
		inner.Header.Set("Content-Type", "application/json")
	}
	rec := httptest.NewRecorder()
	b.s.root.ServeHTTP(rec, inner)
	text := rec.Body.String()
	if len(text) > maxToolOutput {
		text = text[:maxToolOutput] + "\n[cut off]"
	}
	if rec.Code >= 400 {
		return mcp.Result{Text: fmt.Sprintf("HTTP %d: %s", rec.Code, text), IsError: true}, nil
	}
	if text == "" {
		text = fmt.Sprintf("HTTP %d", rec.Code)
	}
	return mcp.Result{Text: text}, nil
}
