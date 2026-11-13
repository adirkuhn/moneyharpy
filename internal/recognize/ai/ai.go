// Package ai implements recognize.Model, the optional AI model of Scan and
// Recognise, for a local Ollama server, the Anthropic API or NVIDIA's API
// catalog (or a self-hosted NIM). Every provider gets the same prompt and JSON
// schema; answers map into a recognize.Extraction whose values are uncertain
// unless the code can verify them.
package ai

import (
	"errors"
	"fmt"

	"github.com/adirkuhn/moneyharpy/internal/recognize"
)

// Providers.
const (
	ProviderOllama    = "ollama"
	ProviderAnthropic = "anthropic"
	ProviderNVIDIA    = "nvidia"
)

// Config is the administrator's AI settings.
type Config struct {
	Provider string // ProviderOllama, ProviderAnthropic or ProviderNVIDIA
	URL      string // Ollama server; for the API providers only to override their URL
	Model    string // required for Ollama; the API providers have a default
	APIKey   string // the API providers; optional for a self-hosted NVIDIA NIM
}

// Errors the models return, wrapped with details, so callers (the settings
// test button) can explain what is wrong.
var (
	ErrConfig        = errors.New("AI model is not configured correctly")
	ErrUnauthorized  = errors.New("the API key was refused")
	ErrRateLimited   = errors.New("the AI provider is rate limiting requests")
	ErrUnavailable   = errors.New("the AI provider is unavailable")
	ErrModelNotFound = errors.New("the AI model was not found")
	ErrRefused       = errors.New("the AI model declined to read the document")
	ErrTruncated     = errors.New("the AI model's answer was cut off")
	ErrNothingToRead = errors.New("no text or pages to read")
)

// New builds the model the configuration names.
func New(cfg Config) (recognize.Model, error) {
	switch cfg.Provider {
	case ProviderOllama:
		return NewOllama(cfg)
	case ProviderAnthropic:
		return NewAnthropic(cfg)
	case ProviderNVIDIA:
		return NewNVIDIA(cfg)
	default:
		return nil, fmt.Errorf("%w: unknown provider %q", ErrConfig, cfg.Provider)
	}
}

// pagesToSend caps the pages sent to a model.
func pagesToSend(pages [][]byte) [][]byte {
	if len(pages) > maxPages {
		return pages[:maxPages]
	}
	return pages
}
