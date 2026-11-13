package ai

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/anthropics/anthropic-sdk-go/v2/option"

	"github.com/adirkuhn/moneyharpy/internal/recognize"
)

var pngPage = []byte("\x89PNG\r\n\x1a\nfake page")

// recorder is a test server that records the last request.
type recorder struct {
	mu     sync.Mutex
	path   string
	header http.Header
	body   map[string]any
}

func (r *recorder) record(req *http.Request) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.path = req.URL.Path
	r.header = req.Header.Clone()
	r.body = nil
	if b, _ := io.ReadAll(req.Body); len(b) > 0 {
		_ = json.Unmarshal(b, &r.body)
	}
}

func serve(t *testing.T, status int, response string) (*httptest.Server, *recorder) {
	t.Helper()
	rec := &recorder{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		rec.record(req)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = io.WriteString(w, response)
	}))
	t.Cleanup(srv.Close)
	return srv, rec
}

func messageResponse(stopReason, text string) string {
	b, _ := json.Marshal(map[string]any{
		"id": "msg_test", "type": "message", "role": "assistant", "model": "claude-opus-5-5",
		"stop_reason": stopReason,
		"content":     []any{map[string]any{"type": "text", "text": text}},
		"usage":       map[string]any{"input_tokens": 1, "output_tokens": 1},
	})
	return string(b)
}

func newTestAnthropic(t *testing.T, srv *httptest.Server, model string) *Anthropic {
	t.Helper()
	m, err := NewAnthropic(Config{Provider: ProviderAnthropic, URL: srv.URL, Model: model, APIKey: "test-key"},
		option.WithMaxRetries(0))
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func TestAnthropicExtractRequest(t *testing.T) {
	srv, rec := serve(t, 200, messageResponse("end_turn", fullAnswer))
	m := newTestAnthropic(t, srv, "")

	e, err := m.Extract(context.Background(), "Example Shop B.V.\nTotal 121.00", [][]byte{pngPage, pngPage})
	if err != nil {
		t.Fatal(err)
	}
	if e.Source != recognize.SourceAI || e.TotalInclTax.V != 12100 {
		t.Errorf("extraction = %+v", e)
	}

	if rec.path != "/v1/messages" {
		t.Errorf("path = %q", rec.path)
	}
	if rec.header.Get("X-Api-Key") != "test-key" && rec.header.Get("Authorization") != "Bearer test-key" {
		t.Errorf("the API key was not sent: %v", rec.header)
	}
	b := rec.body
	if b["model"] != DefaultAnthropicModel {
		t.Errorf("model = %v", b["model"])
	}
	if b["max_tokens"] != float64(anthropicMaxTokens) {
		t.Errorf("max_tokens = %v", b["max_tokens"])
	}
	if _, ok := b["thinking"]; ok {
		t.Errorf("thinking must be left unset, got %v", b["thinking"])
	}
	if b["fallbacks"] != "default" {
		t.Errorf("fallbacks = %v", b["fallbacks"])
	}
	if got := rec.header.Get("Anthropic-Beta"); !strings.Contains(got, "server-side-fallback-2026-07-01") {
		t.Errorf("beta header = %q", got)
	}
	format := b["output_config"].(map[string]any)["format"].(map[string]any)
	if format["type"] != "json_schema" {
		t.Errorf("format = %v", format)
	}
	schema, _ := json.Marshal(format["schema"])
	var want, got any
	_ = json.Unmarshal(extractionSchema, &want)
	_ = json.Unmarshal(schema, &got)
	if !jsonEqual(want, got) {
		t.Errorf("schema = %s", schema)
	}
	system, _ := json.Marshal(b["system"])
	if !strings.Contains(string(system), "purchase invoice or receipt") {
		t.Errorf("system = %s", system)
	}

	msgs := b["messages"].([]any)
	if len(msgs) != 1 {
		t.Fatalf("messages = %v", msgs)
	}
	content := msgs[0].(map[string]any)["content"].([]any)
	if len(content) != 3 {
		t.Fatalf("want 2 images and a text block, got %v", content)
	}
	img := content[0].(map[string]any)
	src := img["source"].(map[string]any)
	if img["type"] != "image" || src["type"] != "base64" || src["media_type"] != "image/png" ||
		src["data"] != base64.StdEncoding.EncodeToString(pngPage) {
		t.Errorf("image block = %v", img)
	}
	text := content[2].(map[string]any)
	if text["type"] != "text" || !strings.Contains(text["text"].(string), "<document>\nExample Shop B.V.\nTotal 121.00\n</document>") {
		t.Errorf("text block = %v", text)
	}
}

func jsonEqual(a, b any) bool {
	x, _ := json.Marshal(a)
	y, _ := json.Marshal(b)
	return string(x) == string(y)
}

func TestAnthropicNoFallbackForOtherModels(t *testing.T) {
	srv, rec := serve(t, 200, messageResponse("end_turn", `{}`))
	m := newTestAnthropic(t, srv, "claude-haiku-5-5")
	if _, err := m.Extract(context.Background(), "text", nil); err != nil {
		t.Fatal(err)
	}
	if rec.body["model"] != "claude-haiku-5-5" {
		t.Errorf("model = %v", rec.body["model"])
	}
	if _, ok := rec.body["fallbacks"]; ok {
		t.Errorf("fallbacks sent for a model without support: %v", rec.body["fallbacks"])
	}
	if got := rec.header.Get("Anthropic-Beta"); strings.Contains(got, "fallback") {
		t.Errorf("beta header = %q", got)
	}
}

func TestAnthropicCapsPages(t *testing.T) {
	srv, rec := serve(t, 200, messageResponse("end_turn", `{}`))
	m := newTestAnthropic(t, srv, "")
	pages := make([][]byte, 25)
	for i := range pages {
		pages[i] = pngPage
	}
	if _, err := m.Extract(context.Background(), "", pages); err != nil {
		t.Fatal(err)
	}
	content := rec.body["messages"].([]any)[0].(map[string]any)["content"].([]any)
	if len(content) != maxPages+1 {
		t.Errorf("blocks = %d, want %d", len(content), maxPages+1)
	}
}

func TestAnthropicStopReasons(t *testing.T) {
	for reason, want := range map[string]error{"refusal": ErrRefused, "max_tokens": ErrTruncated} {
		srv, _ := serve(t, 200, messageResponse(reason, `{"type": "rec`))
		m := newTestAnthropic(t, srv, "")
		if _, err := m.Extract(context.Background(), "text", nil); !errors.Is(err, want) {
			t.Errorf("%s: err = %v, want %v", reason, err, want)
		}
	}
}

func TestAnthropicErrors(t *testing.T) {
	for status, want := range map[int]error{
		401: ErrUnauthorized, 404: ErrModelNotFound, 429: ErrRateLimited, 500: ErrUnavailable, 529: ErrUnavailable,
	} {
		srv, _ := serve(t, status, `{"type": "error", "error": {"type": "some_error", "message": "nope"}}`)
		m := newTestAnthropic(t, srv, "")
		if _, err := m.Extract(context.Background(), "text", nil); !errors.Is(err, want) {
			t.Errorf("extract %d: err = %v, want %v", status, err, want)
		}
		if err := m.Ping(context.Background()); !errors.Is(err, want) {
			t.Errorf("ping %d: err = %v, want %v", status, err, want)
		}
	}
}

func TestAnthropicPing(t *testing.T) {
	srv, rec := serve(t, 200, `{"id": "claude-opus-5-5", "type": "model", "display_name": "Claude Opus 5.5", "created_at": "2026-01-01T00:00:00Z"}`)
	m := newTestAnthropic(t, srv, "")
	if err := m.Ping(context.Background()); err != nil {
		t.Fatal(err)
	}
	if rec.path != "/v1/models/claude-opus-5-5" {
		t.Errorf("path = %q", rec.path)
	}
}

func TestAnthropicNothingToRead(t *testing.T) {
	srv, rec := serve(t, 200, "{}")
	m := newTestAnthropic(t, srv, "")
	if _, err := m.Extract(context.Background(), "  ", nil); !errors.Is(err, ErrNothingToRead) {
		t.Errorf("err = %v", err)
	}
	if rec.path != "" {
		t.Error("nothing should be sent")
	}
}

func TestOllamaExtractRequest(t *testing.T) {
	resp, _ := json.Marshal(map[string]any{
		"model": "llava", "done": true, "done_reason": "stop",
		"message": map[string]any{"role": "assistant", "content": fullAnswer},
	})
	srv, rec := serve(t, 200, string(resp))
	m, err := NewOllama(Config{Provider: ProviderOllama, URL: srv.URL + "/", Model: "llava"})
	if err != nil {
		t.Fatal(err)
	}
	e, err := m.Extract(context.Background(), "Total 121.00", [][]byte{pngPage})
	if err != nil {
		t.Fatal(err)
	}
	if e.Source != recognize.SourceAI || e.Supplier.IBAN != "NL91ABNA0417164300" {
		t.Errorf("extraction = %+v", e)
	}
	if rec.path != "/api/chat" {
		t.Errorf("path = %q", rec.path)
	}
	b := rec.body
	if b["model"] != "llava" || b["stream"] != false {
		t.Errorf("body = %v", b)
	}
	schema, _ := json.Marshal(b["format"])
	var want, got any
	_ = json.Unmarshal(extractionSchema, &want)
	_ = json.Unmarshal(schema, &got)
	if !jsonEqual(want, got) {
		t.Errorf("format = %s", schema)
	}
	msgs := b["messages"].([]any)
	if len(msgs) != 2 || msgs[0].(map[string]any)["role"] != "system" {
		t.Fatalf("messages = %v", msgs)
	}
	user := msgs[1].(map[string]any)
	images := user["images"].([]any)
	if len(images) != 1 || images[0] != base64.StdEncoding.EncodeToString(pngPage) {
		t.Errorf("images = %v", images)
	}
	if !strings.Contains(user["content"].(string), "<document>\nTotal 121.00\n</document>") {
		t.Errorf("content = %v", user["content"])
	}
}

func TestOllamaTruncated(t *testing.T) {
	srv, _ := serve(t, 200, `{"done": true, "done_reason": "length", "message": {"role": "assistant", "content": "{\"ty"}}`)
	m, _ := NewOllama(Config{URL: srv.URL, Model: "llava"})
	if _, err := m.Extract(context.Background(), "text", nil); !errors.Is(err, ErrTruncated) {
		t.Errorf("err = %v", err)
	}
}

func TestOllamaErrors(t *testing.T) {
	for status, want := range map[int]error{404: ErrModelNotFound, 500: ErrUnavailable} {
		srv, _ := serve(t, status, `{"error": "model \"llava\" not found, try pulling it first"}`)
		m, _ := NewOllama(Config{URL: srv.URL, Model: "llava"})
		_, err := m.Extract(context.Background(), "text", nil)
		if !errors.Is(err, want) || !strings.Contains(err.Error(), "try pulling") {
			t.Errorf("%d: err = %v", status, err)
		}
	}
	srv, _ := serve(t, 400, `{"error": "bad format"}`)
	m, _ := NewOllama(Config{URL: srv.URL, Model: "llava"})
	if _, err := m.Extract(context.Background(), "text", nil); err == nil || !strings.Contains(err.Error(), "bad format") {
		t.Errorf("400: err = %v", err)
	}
}

func TestOllamaUnreachable(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	srv.Close()
	m, _ := NewOllama(Config{URL: srv.URL, Model: "llava"})
	if err := m.Ping(context.Background()); !errors.Is(err, ErrUnavailable) {
		t.Errorf("err = %v", err)
	}
}

func TestOllamaPing(t *testing.T) {
	tags := `{"models": [{"name": "llava:latest", "model": "llava:latest"}, {"name": "qwen2.5vl:7b", "model": "qwen2.5vl:7b"}]}`
	for model, ok := range map[string]bool{"llava": true, "llava:latest": true, "qwen2.5vl:7b": true, "qwen2.5vl": false, "mistral": false} {
		srv, rec := serve(t, 200, tags)
		m, _ := NewOllama(Config{URL: srv.URL, Model: model})
		err := m.Ping(context.Background())
		if ok && err != nil || !ok && !errors.Is(err, ErrModelNotFound) {
			t.Errorf("%s: err = %v", model, err)
		}
		if rec.path != "/api/tags" {
			t.Errorf("path = %q", rec.path)
		}
	}
}

func TestNew(t *testing.T) {
	if m, err := New(Config{Provider: ProviderOllama, URL: "http://localhost:11434", Model: "llava"}); err != nil {
		t.Errorf("ollama: %v", err)
	} else if _, ok := m.(*Ollama); !ok {
		t.Errorf("ollama: got %T", m)
	}
	if m, err := New(Config{Provider: ProviderAnthropic, APIKey: "key"}); err != nil {
		t.Errorf("anthropic: %v", err)
	} else if a, ok := m.(*Anthropic); !ok || a.model != DefaultAnthropicModel {
		t.Errorf("anthropic: got %#v", m)
	}
	for _, cfg := range []Config{
		{Provider: "openai"},
		{Provider: ""},
		{Provider: ProviderAnthropic},
		{Provider: ProviderOllama, Model: "llava"},
		{Provider: ProviderOllama, URL: "localhost:11434", Model: "llava"},
		{Provider: ProviderOllama, URL: "http://localhost:11434"},
	} {
		if _, err := New(cfg); !errors.Is(err, ErrConfig) {
			t.Errorf("%+v: err = %v, want ErrConfig", cfg, err)
		}
	}
}
