package ai

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"

	"github.com/anthropics/anthropic-sdk-go/v2"
	"github.com/anthropics/anthropic-sdk-go/v2/option"

	"github.com/adirkuhn/moneyharpy/internal/recognize"
)

// DefaultAnthropicModel is used when the settings name no model.
const DefaultAnthropicModel = "claude-opus-5-5"

// anthropicMaxTokens leaves room for long invoices while staying under the
// SDK's non-streaming timeout.
const anthropicMaxTokens = 16000

// fallbackModels support server-side refusal fallbacks: when the model
// declines, the API answers with a fallback model instead in the same call.
var fallbackModels = map[string]bool{
	"claude-opus-5-5":   true,
	"claude-opus-5":     true,
	"claude-sonnet-5-5": true,
	"claude-fable-5-2":  true,
	"claude-fable-5-1":  true,
	"claude-mythos-5-2": true,
	"claude-mythos-5-1": true,
}

// Anthropic reads documents with a Claude model through the Anthropic API.
// It sends the page images and the text to Anthropic.
type Anthropic struct {
	client anthropic.Client
	model  string
}

// NewAnthropic builds the Anthropic model. Extra options are for tests.
func NewAnthropic(cfg Config, opts ...option.RequestOption) (*Anthropic, error) {
	if strings.TrimSpace(cfg.APIKey) == "" {
		return nil, fmt.Errorf("%w: an API key is required", ErrConfig)
	}
	model := strings.TrimSpace(cfg.Model)
	if model == "" {
		model = DefaultAnthropicModel
	}
	all := []option.RequestOption{option.WithAPIKey(cfg.APIKey)}
	if cfg.URL != "" {
		all = append(all, option.WithBaseURL(cfg.URL))
	}
	all = append(all, opts...)
	return &Anthropic{client: anthropic.NewClient(all...), model: model}, nil
}

// Extract asks the model for the document's data as structured JSON.
func (m *Anthropic) Extract(ctx context.Context, text string, pages [][]byte) (recognize.Extraction, error) {
	if strings.TrimSpace(text) == "" && len(pages) == 0 {
		return recognize.Extraction{}, ErrNothingToRead
	}
	var content []anthropic.BetaContentBlockParamUnion
	images, split := splitTall(pagesToSend(pages))
	for _, p := range images {
		content = append(content, anthropic.NewBetaImageBlock(anthropic.BetaBase64ImageSourceParam{
			Data:      base64.StdEncoding.EncodeToString(p),
			MediaType: anthropic.BetaBase64ImageSourceMediaTypeImagePNG,
		}))
	}
	prompt := userPrompt(text)
	if split {
		prompt = splitNote + prompt
	}
	content = append(content, anthropic.NewBetaTextBlock(prompt))

	params := anthropic.BetaMessageNewParams{
		Model:     m.model,
		MaxTokens: anthropicMaxTokens,
		System:    []anthropic.BetaTextBlockParam{{Text: systemPrompt}},
		Messages:  []anthropic.BetaMessageParam{anthropic.NewBetaUserMessage(content...)},
		OutputConfig: anthropic.BetaOutputConfigParam{
			Format: anthropic.BetaJSONOutputFormatParam{Schema: extractionSchema},
		},
	}
	if fallbackModels[m.model] {
		params.Betas = []anthropic.AnthropicBeta{anthropic.AnthropicBetaServerSideFallback2026_07_01}
		params.Fallbacks = anthropic.BetaFallbacksParamOfDefault()
	}

	msg, err := m.client.Beta.Messages.New(ctx, params)
	if err != nil {
		return recognize.Extraction{}, classify(err)
	}
	switch msg.StopReason {
	case anthropic.BetaStopReasonRefusal:
		return recognize.Extraction{}, ErrRefused
	case anthropic.BetaStopReasonMaxTokens:
		return recognize.Extraction{}, ErrTruncated
	}
	for _, block := range msg.Content {
		if t, ok := block.AsAny().(anthropic.BetaTextBlock); ok {
			return toExtraction([]byte(t.Text))
		}
	}
	return recognize.Extraction{}, errors.New("the AI model answered without text")
}

// Ping checks the key and the model with the Models API, which costs nothing.
func (m *Anthropic) Ping(ctx context.Context) error {
	if _, err := m.client.Models.Get(ctx, m.model, anthropic.ModelGetParams{}); err != nil {
		return classify(err)
	}
	return nil
}

// classify turns an API error into one of the package's errors.
func classify(err error) error {
	var apierr *anthropic.Error
	if !errors.As(err, &apierr) {
		return fmt.Errorf("%w: %w", ErrUnavailable, err)
	}
	var kind error
	switch code := apierr.StatusCode; {
	case code == 401 || code == 403:
		kind = ErrUnauthorized
	case code == 404:
		kind = ErrModelNotFound
	case code == 429:
		kind = ErrRateLimited
	case code >= 500:
		kind = ErrUnavailable
	default:
		return fmt.Errorf("AI request failed: %w", err)
	}
	return fmt.Errorf("%w (HTTP %d): %w", kind, apierr.StatusCode, err)
}
