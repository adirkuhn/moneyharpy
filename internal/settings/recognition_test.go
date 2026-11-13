package settings

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/adirkuhn/moneyharpy/internal/resource"
)

func TestRecognitionAIIsOffByDefault(t *testing.T) {
	s, _ := newService(t)
	got, err := s.Instance(context.Background())
	if err != nil {
		t.Fatalf("Instance() error = %v", err)
	}
	if got.Recognition.AIEnabled || got.Recognition.Provider != "ollama" {
		t.Errorf("Instance().Recognition = %+v, want off with the ollama provider", got.Recognition)
	}
}

func TestRecognitionAPIKeyIsEncryptedAndKept(t *testing.T) {
	s, store := newService(t)
	ctx := context.Background()
	const key = "ai-api-key-value"
	if _, err := s.Update(ctx, Update{
		RecognitionAIEnabled: ptr(true), RecognitionAIProvider: ptr("anthropic"), RecognitionAIModel: ptr(" claude-opus-5-5 "),
		RecognitionAIAPIKey: ptr(key),
	}); err != nil {
		t.Fatalf("Update() error = %v", err)
	}
	for k, raw := range store.values {
		if strings.Contains(string(raw), key) {
			t.Errorf("stored value for %q contains the key in clear", k)
		}
	}
	got, err := s.Update(ctx, Update{RecognitionAIAPIKey: ptr("")})
	if err != nil {
		t.Fatalf("Update() error = %v", err)
	}
	want := Recognition{AIEnabled: true, Provider: "anthropic", Model: "claude-opus-5-5", APIKey: key}
	if got.Recognition != want {
		t.Errorf("Update().Recognition = %+v, want %+v", got.Recognition, want)
	}
	again, err := s.Instance(ctx)
	if err != nil || again.Recognition != want {
		t.Errorf("Instance().Recognition = %+v, %v, want %+v", again.Recognition, err, want)
	}
}

func TestRecognitionValidation(t *testing.T) {
	tests := []struct {
		name  string
		u     Update
		field string
	}{
		{"unknown provider", Update{RecognitionAIProvider: ptr("other")}, "recognition_ai_provider"},
		{"model needed", Update{RecognitionAIEnabled: ptr(true), RecognitionAIURL: ptr("http://ollama:11434")}, "recognition_ai_model"},
		{"api key needed", Update{RecognitionAIEnabled: ptr(true), RecognitionAIProvider: ptr("anthropic"), RecognitionAIModel: ptr("m")}, "recognition_ai_api_key"},
		{"bad url", Update{RecognitionAIURL: ptr("ftp://x")}, "recognition_ai_url"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s, _ := newService(t)
			_, err := s.Update(context.Background(), tt.u)
			var verr *resource.ValidationError
			if !errors.As(err, &verr) {
				t.Fatalf("Update() error = %v, want *resource.ValidationError", err)
			}
			if _, ok := verr.Fields[tt.field]; !ok {
				t.Errorf("Update() fields = %v, want key %q", verr.Fields, tt.field)
			}
		})
	}
}
