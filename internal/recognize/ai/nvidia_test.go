package ai

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/adirkuhn/moneyharpy/internal/recognize"
)

func completion(finish, content string) string {
	b, _ := json.Marshal(map[string]any{
		"id": "chatcmpl-test", "object": "chat.completion", "model": DefaultNVIDIAModel,
		"choices": []any{map[string]any{
			"index": 0, "finish_reason": finish,
			"message": map[string]any{"role": "assistant", "content": content},
		}},
	})
	return string(b)
}

func newTestNVIDIA(t *testing.T, srv *httptest.Server, model string) *NVIDIA {
	t.Helper()
	m, err := NewNVIDIA(Config{Provider: ProviderNVIDIA, URL: srv.URL + "/v1/", Model: model, APIKey: "nvapi-test"})
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func TestNVIDIAExtractRequest(t *testing.T) {
	srv, rec := serve(t, 200, completion("stop", fullAnswer))
	m := newTestNVIDIA(t, srv, "")

	e, err := m.Extract(context.Background(), "Example Shop B.V.\nTotal 121.00", [][]byte{pngPage, pngPage})
	if err != nil {
		t.Fatal(err)
	}
	if e.Source != recognize.SourceAI || e.TotalInclTax.V != 12100 {
		t.Errorf("extraction = %+v", e)
	}
	if rec.path != "/v1/chat/completions" {
		t.Errorf("path = %q", rec.path)
	}
	if got := rec.header.Get("Authorization"); got != "Bearer nvapi-test" {
		t.Errorf("Authorization = %q", got)
	}
	b := rec.body
	if b["model"] != DefaultNVIDIAModel || b["temperature"] != float64(0) || b["max_tokens"] != float64(nvidiaMaxTokens) || b["stream"] != false {
		t.Errorf("body = %v", b)
	}
	for _, unsupported := range []string{"response_format", "nvext"} {
		if _, ok := b[unsupported]; ok {
			t.Errorf("%s is not accepted by the hosted API, got %v", unsupported, b[unsupported])
		}
	}
	msgs := b["messages"].([]any)
	if len(msgs) != 1 || msgs[0].(map[string]any)["role"] != "user" {
		t.Fatalf("messages = %v", msgs)
	}
	content := msgs[0].(map[string]any)["content"].([]any)
	if len(content) != nvidiaMaxImages+1 {
		t.Fatalf("want %d image(s) and a text part, got %v", nvidiaMaxImages, content)
	}
	img := content[0].(map[string]any)
	if img["type"] != "image_url" || img["image_url"].(map[string]any)["url"] != "data:image/png;base64,"+base64.StdEncoding.EncodeToString(pngPage) {
		t.Errorf("image part = %v", img)
	}
	text := content[1].(map[string]any)
	prompt, _ := text["text"].(string)
	if text["type"] != "text" ||
		!strings.Contains(prompt, "purchase invoice or receipt") ||
		!strings.Contains(prompt, string(extractionSchema)) ||
		!strings.Contains(prompt, "<document>\nExample Shop B.V.\nTotal 121.00\n</document>") {
		t.Errorf("text part = %v", text)
	}
}

func TestNVIDIATextOnly(t *testing.T) {
	srv, rec := serve(t, 200, completion("stop", `{}`))
	m := newTestNVIDIA(t, srv, "meta/llama-3.2-11b-vision-instruct")
	if _, err := m.Extract(context.Background(), "text", nil); err != nil {
		t.Fatal(err)
	}
	if rec.body["model"] != "meta/llama-3.2-11b-vision-instruct" {
		t.Errorf("model = %v", rec.body["model"])
	}
	content := rec.body["messages"].([]any)[0].(map[string]any)["content"].([]any)
	if len(content) != 1 || content[0].(map[string]any)["type"] != "text" {
		t.Errorf("content = %v", content)
	}
}

func TestNVIDIAAnswerInFences(t *testing.T) {
	for name, answer := range map[string]string{
		"json fence":  "```json\n" + fullAnswer + "\n```",
		"plain fence": "```\n" + fullAnswer + "\n```",
		"with prose":  "Here is the data:\n" + fullAnswer + "\nLet me know if you need more.",
	} {
		srv, _ := serve(t, 200, completion("stop", answer))
		e, err := newTestNVIDIA(t, srv, "").Extract(context.Background(), "text", nil)
		if err != nil || e.TotalInclTax.V != 12100 {
			t.Errorf("%s: extraction = %+v, %v", name, e, err)
		}
	}
	srv, _ := serve(t, 200, completion("stop", "I cannot read this document."))
	if _, err := newTestNVIDIA(t, srv, "").Extract(context.Background(), "text", nil); err == nil {
		t.Error("an answer without JSON must fail")
	}
}

func TestNVIDIATruncated(t *testing.T) {
	srv, _ := serve(t, 200, completion("length", `{"type": "rec`))
	if _, err := newTestNVIDIA(t, srv, "").Extract(context.Background(), "text", nil); !errors.Is(err, ErrTruncated) {
		t.Errorf("err = %v", err)
	}
}

func TestNVIDIAErrors(t *testing.T) {
	bodies := map[int]string{
		401: `{"status": 401, "title": "Unauthorized", "detail": "Authentication failed"}`,
		403: `{"status": 403, "title": "Forbidden", "detail": "Authorization failed"}`,
		404: `{"status": 404, "title": "Not Found", "detail": "Function not found for account"}`,
		429: `{"status": 429, "title": "Too Many Requests", "detail": "Rate limit exceeded"}`,
		500: `{"error": {"message": "Inference failed", "type": "internal_error"}}`,
		503: `upstream down`,
	}
	want := map[int]error{401: ErrUnauthorized, 403: ErrUnauthorized, 404: ErrModelNotFound, 429: ErrRateLimited, 500: ErrUnavailable, 503: ErrUnavailable}
	text := map[int]string{401: "Authentication failed", 403: "Authorization failed", 404: "Function not found", 429: "Rate limit exceeded", 500: "Inference failed", 503: "upstream down"}
	for status, body := range bodies {
		srv, _ := serve(t, status, body)
		m := newTestNVIDIA(t, srv, "")
		_, err := m.Extract(context.Background(), "text", nil)
		if !errors.Is(err, want[status]) || !strings.Contains(err.Error(), text[status]) {
			t.Errorf("extract %d: err = %v", status, err)
		}
		if err := m.Ping(context.Background()); !errors.Is(err, want[status]) {
			t.Errorf("ping %d: err = %v", status, err)
		}
	}
	srv, _ := serve(t, 422, `{"status": 422, "title": "Unprocessable Entity", "detail": "max_tokens too large"}`)
	if _, err := newTestNVIDIA(t, srv, "").Extract(context.Background(), "text", nil); err == nil || !strings.Contains(err.Error(), "max_tokens too large") {
		t.Errorf("422: err = %v", err)
	}
}

func TestNVIDIAUnreachable(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	srv.Close()
	m, _ := NewNVIDIA(Config{URL: srv.URL, APIKey: "nvapi-test"})
	if err := m.Ping(context.Background()); !errors.Is(err, ErrUnavailable) {
		t.Errorf("err = %v", err)
	}
}

func TestNVIDIAPing(t *testing.T) {
	models := `{"object": "list", "data": [{"id": "meta/llama-3.2-90b-vision-instruct", "object": "model"}, {"id": "meta/llama-3.2-11b-vision-instruct", "object": "model"}]}`
	for model, ok := range map[string]bool{"": true, "meta/llama-3.2-11b-vision-instruct": true, "meta/llama-3.2-11b": false} {
		srv, rec := serve(t, 200, models)
		err := newTestNVIDIA(t, srv, model).Ping(context.Background())
		if ok && err != nil || !ok && !errors.Is(err, ErrModelNotFound) {
			t.Errorf("%q: err = %v", model, err)
		}
		if rec.path != "/v1/models" || rec.header.Get("Authorization") != "Bearer nvapi-test" {
			t.Errorf("%q: path = %q, header = %v", model, rec.path, rec.header)
		}
	}
}

func TestNVIDIANothingToRead(t *testing.T) {
	srv, rec := serve(t, 200, "{}")
	if _, err := newTestNVIDIA(t, srv, "").Extract(context.Background(), " ", nil); !errors.Is(err, ErrNothingToRead) {
		t.Errorf("err = %v", err)
	}
	if rec.path != "" {
		t.Error("nothing should be sent")
	}
}

func TestNewNVIDIA(t *testing.T) {
	m, err := New(Config{Provider: ProviderNVIDIA, APIKey: "nvapi-test"})
	if err != nil {
		t.Fatal(err)
	}
	if n, ok := m.(*NVIDIA); !ok || n.base != DefaultNVIDIAURL || n.model != DefaultNVIDIAModel {
		t.Errorf("got %#v", m)
	}
	// A self-hosted NIM may run without a key.
	if _, err := New(Config{Provider: ProviderNVIDIA, URL: "http://nim:8000/v1"}); err != nil {
		t.Errorf("self-hosted without key: %v", err)
	}
	for _, cfg := range []Config{
		{Provider: ProviderNVIDIA},
		{Provider: ProviderNVIDIA, URL: DefaultNVIDIAURL + "/"},
		{Provider: ProviderNVIDIA, URL: "nim:8000", APIKey: "k"},
	} {
		if _, err := New(cfg); !errors.Is(err, ErrConfig) {
			t.Errorf("%+v: err = %v, want ErrConfig", cfg, err)
		}
	}
}
