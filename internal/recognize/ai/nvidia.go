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

// DefaultNVIDIAURL is NVIDIA's hosted API catalog (build.nvidia.com), an
// OpenAI-compatible chat completions API.
const DefaultNVIDIAURL = "https://integrate.api.nvidia.com/v1"

// DefaultNVIDIAModel is used when the settings name no model: a vision model
// listed among the catalog's multimodal APIs that reads page images.
const DefaultNVIDIAModel = "meta/llama-3.2-90b-vision-instruct"

const (
	// nvidiaMaxTokens is the most the catalog's Llama 3.2 Vision APIs accept.
	nvidiaMaxTokens = 8192
	// nvidiaMaxImages: Llama 3.2 Vision reads one image per request, so the
	// first page goes as an image and the rest of the document as text.
	nvidiaMaxImages = 1
	nvidiaTimeout   = 5 * time.Minute
)

// NVIDIA reads documents with a model from NVIDIA's API catalog, or from a
// self-hosted NIM at another URL. With the hosted catalog the page image and
// the text are sent to NVIDIA.
type NVIDIA struct {
	base   string
	model  string
	apiKey string
	client *http.Client
}

// NewNVIDIA builds the NVIDIA model. The hosted catalog needs an API key; a
// self-hosted NIM at another URL may run without one.
func NewNVIDIA(cfg Config) (*NVIDIA, error) {
	base := strings.TrimRight(strings.TrimSpace(cfg.URL), "/")
	if base == "" {
		base = DefaultNVIDIAURL
	}
	if u, err := url.Parse(base); err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return nil, fmt.Errorf("%w: the NVIDIA address must be an http(s) URL", ErrConfig)
	}
	key := strings.TrimSpace(cfg.APIKey)
	if key == "" && base == DefaultNVIDIAURL {
		return nil, fmt.Errorf("%w: an API key is required", ErrConfig)
	}
	model := strings.TrimSpace(cfg.Model)
	if model == "" {
		model = DefaultNVIDIAModel
	}
	return &NVIDIA{base: base, model: model, apiKey: key, client: &http.Client{Timeout: nvidiaTimeout}}, nil
}

type chatPart struct {
	Type     string        `json:"type"`
	Text     string        `json:"text,omitempty"`
	ImageURL *chatImageURL `json:"image_url,omitempty"`
}

type chatImageURL struct {
	URL string `json:"url"`
}

type chatMessage struct {
	Role    string     `json:"role"`
	Content []chatPart `json:"content"`
}

type chatRequest struct {
	Model       string        `json:"model"`
	Messages    []chatMessage `json:"messages"`
	Temperature float64       `json:"temperature"`
	MaxTokens   int           `json:"max_tokens"`
	Stream      bool          `json:"stream"`
}

type chatResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
}

// nvidiaPrompt is the whole instruction, in the user message: the catalog's
// request schema has no structured output option, and Llama 3.2 Vision does
// not take a system message together with an image. So the schema is in the
// prompt and the answer is validated when it is mapped.
func nvidiaPrompt(text string) string {
	return systemPrompt + "\nAnswer with one JSON object and nothing else, matching this JSON schema:\n" +
		string(extractionSchema) + "\n\n" + userPrompt(text)
}

// Extract asks the model for the document's data as JSON.
func (m *NVIDIA) Extract(ctx context.Context, text string, pages [][]byte) (recognize.Extraction, error) {
	if strings.TrimSpace(text) == "" && len(pages) == 0 {
		return recognize.Extraction{}, ErrNothingToRead
	}
	var content []chatPart
	for i, p := range pagesToSend(pages) {
		if i == nvidiaMaxImages {
			break
		}
		content = append(content, chatPart{Type: "image_url", ImageURL: &chatImageURL{
			URL: "data:image/png;base64," + base64.StdEncoding.EncodeToString(p),
		}})
	}
	content = append(content, chatPart{Type: "text", Text: nvidiaPrompt(text)})
	req := chatRequest{
		Model:     m.model,
		Messages:  []chatMessage{{Role: "user", Content: content}},
		MaxTokens: nvidiaMaxTokens,
	}
	var res chatResponse
	if err := m.do(ctx, http.MethodPost, "/chat/completions", req, &res); err != nil {
		return recognize.Extraction{}, err
	}
	if len(res.Choices) == 0 {
		return recognize.Extraction{}, errors.New("the AI model answered without text")
	}
	choice := res.Choices[0]
	if choice.FinishReason == "length" {
		return recognize.Extraction{}, ErrTruncated
	}
	return toExtraction(jsonObject(choice.Message.Content))
}

// jsonObject takes the JSON object out of an answer: models without
// structured output sometimes wrap it in ``` fences or a sentence.
func jsonObject(answer string) []byte {
	start, end := strings.Index(answer, "{"), strings.LastIndex(answer, "}")
	if start < 0 || end < start {
		return []byte(answer)
	}
	return []byte(answer[start : end+1])
}

// Ping checks the key and that the model is listed, which costs nothing.
func (m *NVIDIA) Ping(ctx context.Context) error {
	var res struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := m.do(ctx, http.MethodGet, "/models", nil, &res); err != nil {
		return err
	}
	for _, x := range res.Data {
		if x.ID == m.model {
			return nil
		}
	}
	return fmt.Errorf("%w: %q is not offered at %s", ErrModelNotFound, m.model, m.base)
}

func (m *NVIDIA) do(ctx context.Context, method, path string, body, out any) error {
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
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if m.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+m.apiKey)
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
		return nvidiaError(res)
	}
	if err := json.NewDecoder(res.Body).Decode(out); err != nil {
		return fmt.Errorf("%w: unexpected answer: %w", ErrUnavailable, err)
	}
	return nil
}

// nvidiaError turns an error response into one of the package's errors,
// keeping NVIDIA's own explanation.
func nvidiaError(res *http.Response) error {
	b, _ := io.ReadAll(io.LimitReader(res.Body, 4096))
	detail := errorText(b)
	var kind error
	switch code := res.StatusCode; {
	case code == http.StatusUnauthorized || code == http.StatusForbidden:
		kind = ErrUnauthorized
	case code == http.StatusNotFound:
		kind = ErrModelNotFound
	case code == http.StatusTooManyRequests:
		kind = ErrRateLimited
	case code >= 500 || code == http.StatusAccepted: // 202: still queued at NVIDIA
		kind = ErrUnavailable
	default:
		return fmt.Errorf("NVIDIA refused the request (HTTP %d): %s", code, detail)
	}
	return fmt.Errorf("%w (HTTP %d): %s", kind, res.StatusCode, detail)
}

// errorText reads the explanation from the error shapes NVIDIA uses: problem
// details ({title, detail}), OpenAI-style ({error: {message}}) or plain text.
func errorText(b []byte) string {
	var e struct {
		Title   string          `json:"title"`
		Detail  string          `json:"detail"`
		Message string          `json:"message"`
		Error   json.RawMessage `json:"error"`
	}
	if json.Unmarshal(b, &e) == nil {
		var nested struct {
			Message string `json:"message"`
		}
		var plain string
		switch {
		case e.Detail != "":
			return e.Detail
		case json.Unmarshal(e.Error, &nested) == nil && nested.Message != "":
			return nested.Message
		case json.Unmarshal(e.Error, &plain) == nil && plain != "":
			return plain
		case e.Message != "":
			return e.Message
		case e.Title != "":
			return e.Title
		}
	}
	return strings.TrimSpace(string(b))
}
