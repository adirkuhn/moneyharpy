package ai

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/adirkuhn/moneyharpy/internal/recognize"
)

// ollamaTimeout bounds one request: local vision models can be slow on CPU.
const ollamaTimeout = 10 * time.Minute

// Ollama reads documents with a model on an Ollama server, usually local, so
// documents stay on the administrator's own machines.
type Ollama struct {
	base   string
	model  string
	client *http.Client
}

// NewOllama builds the Ollama model; the URL and model are required.
func NewOllama(cfg Config) (*Ollama, error) {
	base := strings.TrimRight(strings.TrimSpace(cfg.URL), "/")
	if u, err := url.Parse(base); base == "" || err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return nil, fmt.Errorf("%w: an http(s) URL of the Ollama server is required", ErrConfig)
	}
	model := strings.TrimSpace(cfg.Model)
	if model == "" {
		return nil, fmt.Errorf("%w: a model is required", ErrConfig)
	}
	return &Ollama{base: base, model: model, client: &http.Client{Timeout: ollamaTimeout}}, nil
}

type ollamaMessage struct {
	Role    string   `json:"role"`
	Content string   `json:"content"`
	Images  []string `json:"images,omitempty"`
}

type ollamaChatRequest struct {
	Model    string          `json:"model"`
	Messages []ollamaMessage `json:"messages"`
	Format   json.RawMessage `json:"format"`
	Stream   bool            `json:"stream"`
	Options  map[string]any  `json:"options,omitempty"`
}

type ollamaChatResponse struct {
	Message    ollamaMessage `json:"message"`
	DoneReason string        `json:"done_reason"`
}

// Extract asks the model for the document's data, constrained to the schema.
func (m *Ollama) Extract(ctx context.Context, text string, pages [][]byte) (recognize.Extraction, error) {
	if strings.TrimSpace(text) == "" && len(pages) == 0 {
		return recognize.Extraction{}, ErrNothingToRead
	}
	images, split := splitTall(pagesToSend(pages))
	user := ollamaMessage{Role: "user", Content: userPrompt(text)}
	if split {
		user.Content = splitNote + user.Content
	}
	for _, p := range images {
		user.Images = append(user.Images, base64.StdEncoding.EncodeToString(p))
	}
	req := ollamaChatRequest{
		Model:    m.model,
		Messages: []ollamaMessage{{Role: "system", Content: systemPrompt}, user},
		Format:   extractionSchema,
		Options:  map[string]any{"temperature": 0},
	}
	var res ollamaChatResponse
	if err := m.do(ctx, http.MethodPost, "/api/chat", req, &res); err != nil {
		return recognize.Extraction{}, err
	}
	if res.DoneReason == "length" {
		return recognize.Extraction{}, ErrTruncated
	}
	return toExtraction([]byte(res.Message.Content))
}

// Ping checks the server answers and has the model pulled.
func (m *Ollama) Ping(ctx context.Context) error {
	var res struct {
		Models []struct {
			Name  string `json:"name"`
			Model string `json:"model"`
		} `json:"models"`
	}
	if err := m.do(ctx, http.MethodGet, "/api/tags", nil, &res); err != nil {
		return err
	}
	want := m.model
	if !strings.Contains(want, ":") {
		want += ":latest"
	}
	for _, x := range res.Models {
		if x.Name == m.model || x.Name == want || x.Model == m.model || x.Model == want {
			return nil
		}
	}
	return fmt.Errorf("%w: %q is not pulled on the Ollama server (ollama pull %s)", ErrModelNotFound, m.model, m.model)
}

func (m *Ollama) do(ctx context.Context, method, path string, body, out any) error {
	var r io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		r = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, m.base+path, r)
	if err != nil {
		return err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	res, err := m.client.Do(req)
	if err != nil {
		if errors.Is(err, context.Canceled) {
			return err
		}
		return fmt.Errorf("%w: %w", ErrUnavailable, err)
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusOK {
		var e struct {
			Error string `json:"error"`
		}
		_ = json.NewDecoder(io.LimitReader(res.Body, 4096)).Decode(&e)
		kind := ErrUnavailable
		if res.StatusCode == http.StatusNotFound {
			kind = ErrModelNotFound
		} else if res.StatusCode < 500 {
			return fmt.Errorf("the Ollama server refused the request (HTTP %d): %s", res.StatusCode, e.Error)
		}
		return fmt.Errorf("%w (HTTP %d): %s", kind, res.StatusCode, e.Error)
	}
	if err := json.NewDecoder(res.Body).Decode(out); err != nil {
		return fmt.Errorf("%w: unexpected answer: %w", ErrUnavailable, err)
	}
	return nil
}
