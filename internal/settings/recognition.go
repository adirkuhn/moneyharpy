package settings

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"slices"
	"strings"

	"github.com/adirkuhn/moneyharpy/internal/resource"
)

// Recognition configures the optional AI models Scan and Recognise asks
// about documents its rules could not read. Off by default; with an API
// provider documents are sent to that provider.
type Recognition struct {
	AIEnabled bool
	// Connections are tried in this order: the first that answers wins.
	Connections []AIConnection
	// VIESEnabled allows sending the VAT number read from a document to the
	// EU's VIES service, to add a new supplier under its registered name.
	// Off by default.
	VIESEnabled bool
}

// AIConnection is one AI model Scan and Recognise can ask.
type AIConnection struct {
	// ID stays the same while the connection exists, so its key is kept.
	ID   string
	Name string
	// Provider is one of RecognitionProviders.
	Provider string
	// URL is the address of the Ollama server, or overrides an API
	// provider's own address; empty for the provider's own.
	URL string
	// Model is empty for the provider's default (not for Ollama).
	Model   string
	APIKey  string
	Enabled bool
}

// AIConnectionUpdate is one connection in an update. A nil APIKey keeps the
// stored key of the connection with that ID; an empty one clears it.
type AIConnectionUpdate struct {
	ID       string
	Name     string
	Provider string
	URL      string
	Model    string
	APIKey   *string
	Enabled  bool
}

// RecognitionProviders are the AI providers Scan and Recognise can ask.
var RecognitionProviders = []string{"ollama", "anthropic", "nvidia"}

// MaxAIConnections caps the connections an administrator can add.
const MaxAIConnections = 10

// providerNames name connections the administrator left unnamed.
var providerNames = map[string]string{"ollama": "Ollama", "anthropic": "Anthropic", "nvidia": "NVIDIA"}

type storedConnection struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Provider string `json:"provider"`
	URL      string `json:"url"`
	Model    string `json:"model"`
	APIKey   string `json:"api_key_sealed,omitempty"`
	Enabled  bool   `json:"enabled"`
}

// legacyRecognition is the single AI connection stored before there could
// be more; it is read as the first connection until the settings are saved.
type legacyRecognition struct {
	Provider string `json:"recognition_ai_provider,omitempty"`
	URL      string `json:"recognition_ai_url,omitempty"`
	Model    string `json:"recognition_ai_model,omitempty"`
	APIKey   string `json:"recognition_ai_api_key_sealed,omitempty"`
}

func (l legacyRecognition) legacyConnections() []storedConnection {
	// Only settings an administrator filled in; a bare provider was the default.
	if l.URL == "" && l.Model == "" && l.APIKey == "" {
		return nil
	}
	provider := l.Provider
	if provider == "" {
		provider = RecognitionProviders[0]
	}
	return []storedConnection{{
		ID: "default", Name: providerNames[provider], Provider: provider, URL: l.URL, Model: l.Model,
		APIKey: l.APIKey, Enabled: true,
	}}
}

func (s *Service) openConnections(stored []storedConnection) ([]AIConnection, error) {
	if stored == nil {
		return nil, nil
	}
	out := make([]AIConnection, len(stored))
	for i, c := range stored {
		out[i] = AIConnection{ID: c.ID, Name: c.Name, Provider: c.Provider, URL: c.URL, Model: c.Model, Enabled: c.Enabled}
		if c.APIKey != "" {
			key, err := s.cipher.Open(c.APIKey)
			if err != nil {
				return nil, fmt.Errorf("decrypt api key of AI connection %q: %w", c.ID, err)
			}
			out[i].APIKey = key
		}
	}
	return out, nil
}

func (s *Service) sealConnections(cs []AIConnection) ([]storedConnection, error) {
	if cs == nil {
		return nil, nil
	}
	out := make([]storedConnection, len(cs))
	for i, c := range cs {
		out[i] = storedConnection{ID: c.ID, Name: c.Name, Provider: c.Provider, URL: c.URL, Model: c.Model, Enabled: c.Enabled}
		if c.APIKey != "" {
			sealed, err := s.cipher.Seal(c.APIKey)
			if err != nil {
				return nil, fmt.Errorf("encrypt api key of AI connection %q: %w", c.ID, err)
			}
			out[i].APIKey = sealed
		}
	}
	return out, nil
}

func applyRecognition(cfg *Recognition, u Update) error {
	if u.RecognitionAIEnabled != nil {
		cfg.AIEnabled = *u.RecognitionAIEnabled
	}
	if u.RecognitionVIESEnabled != nil {
		cfg.VIESEnabled = *u.RecognitionVIESEnabled
	}
	if u.RecognitionAIConnections == nil {
		return nil
	}
	current := map[string]AIConnection{}
	for _, c := range cfg.Connections {
		current[c.ID] = c
	}
	next := make([]AIConnection, 0, len(*u.RecognitionAIConnections))
	for _, cu := range *u.RecognitionAIConnections {
		c := AIConnection{
			ID:       cu.ID,
			Name:     strings.TrimSpace(cu.Name),
			Provider: strings.TrimSpace(cu.Provider),
			URL:      strings.TrimRight(strings.TrimSpace(cu.URL), "/"),
			Model:    strings.TrimSpace(cu.Model),
			Enabled:  cu.Enabled,
		}
		old, known := current[c.ID]
		if !known {
			id, err := newConnectionID()
			if err != nil {
				return err
			}
			c.ID = id
		}
		delete(current, c.ID) // an id given twice is new the second time
		switch {
		case cu.APIKey != nil:
			c.APIKey = *cu.APIKey
		case known:
			c.APIKey = old.APIKey
		}
		if c.Name == "" {
			c.Name = providerNames[c.Provider]
		}
		next = append(next, c)
	}
	cfg.Connections = next
	return nil
}

func newConnectionID() (string, error) {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("new AI connection id: %w", err)
	}
	return hex.EncodeToString(b), nil
}

func validateRecognition(cfg Recognition, verr *resource.ValidationError) {
	const list = "recognition_ai_connections"
	if len(cfg.Connections) > MaxAIConnections {
		verr.Add(list, fmt.Sprintf("is too long (maximum is %d connections)", MaxAIConnections))
	}
	enabled := false
	names := map[string]bool{}
	for i, c := range cfg.Connections {
		field := func(name string) string { return fmt.Sprintf("%s[%d].%s", list, i, name) }
		enabled = enabled || c.Enabled
		if name := strings.ToLower(c.Name); name == "" {
			verr.Add(field("name"), "can't be blank")
		} else if names[name] {
			verr.Add(field("name"), "has already been taken")
		} else {
			names[name] = true
		}
		if !slices.Contains(RecognitionProviders, c.Provider) {
			verr.Add(field("provider"), "is not included in the list")
		}
		if c.URL != "" && !validHTTPAddress(c.URL) {
			verr.Add(field("url"), "must be an http or https address without a query")
		}
		if !c.Enabled {
			continue
		}
		switch c.Provider {
		case "ollama":
			if c.URL == "" {
				verr.Add(field("url"), "can't be blank for Ollama")
			}
			if c.Model == "" {
				verr.Add(field("model"), "can't be blank for Ollama")
			}
		case "anthropic":
			if c.APIKey == "" {
				verr.Add(field("api_key"), "can't be blank while the connection is enabled")
			}
		case "nvidia":
			// A self-hosted NIM at its own address may run without a key.
			if c.APIKey == "" && c.URL == "" {
				verr.Add(field("api_key"), "can't be blank for NVIDIA's API catalog")
			}
		}
	}
	if cfg.AIEnabled && !enabled {
		verr.Add(list, "needs an enabled connection while the AI model is enabled")
	}
}
