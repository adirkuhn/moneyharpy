package settings

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/adirkuhn/moneyharpy/internal/resource"
)

func TestRecognitionAIIsOffByDefault(t *testing.T) {
	s, _ := newService(t)
	got, err := s.Instance(context.Background())
	if err != nil {
		t.Fatalf("Instance() error = %v", err)
	}
	if got.Recognition.AIEnabled || len(got.Recognition.Connections) != 0 {
		t.Errorf("Instance().Recognition = %+v, want off without connections", got.Recognition)
	}
}

func connections(cs ...AIConnectionUpdate) Update {
	return Update{RecognitionAIConnections: &cs}
}

func TestRecognitionConnectionsKeepOrderAndGetIDs(t *testing.T) {
	s, _ := newService(t)
	ctx := context.Background()
	got, err := s.Update(ctx, Update{
		RecognitionAIEnabled: ptr(true),
		RecognitionAIConnections: &[]AIConnectionUpdate{
			{Name: " NVIDIA free ", Provider: "nvidia", APIKey: ptr("nvapi-placeholder"), Enabled: true},
			{Provider: "ollama", URL: " http://ollama:11434/ ", Model: " qwen2.5vl ", Enabled: true},
			{Name: "Claude", Provider: "anthropic", APIKey: ptr("sk-placeholder")},
		},
	})
	if err != nil {
		t.Fatalf("Update() error = %v", err)
	}
	cs := got.Recognition.Connections
	if len(cs) != 3 {
		t.Fatalf("connections = %+v", cs)
	}
	ids := map[string]bool{}
	for _, c := range cs {
		if c.ID == "" || ids[c.ID] {
			t.Errorf("connection %q has id %q, want a new unique id", c.Name, c.ID)
		}
		ids[c.ID] = true
	}
	want := []AIConnection{
		{ID: cs[0].ID, Name: "NVIDIA free", Provider: "nvidia", APIKey: "nvapi-placeholder", Enabled: true},
		{ID: cs[1].ID, Name: "Ollama", Provider: "ollama", URL: "http://ollama:11434", Model: "qwen2.5vl", Enabled: true},
		{ID: cs[2].ID, Name: "Claude", Provider: "anthropic", APIKey: "sk-placeholder"},
	}
	if diff := cmp.Diff(want, cs); diff != "" {
		t.Errorf("connections (-want +got):\n%s", diff)
	}
	again, err := s.Instance(ctx)
	if err != nil || !again.Recognition.AIEnabled {
		t.Fatalf("Instance() = %+v, %v", again.Recognition, err)
	}
	if diff := cmp.Diff(want, again.Recognition.Connections); diff != "" {
		t.Errorf("stored connections (-want +got):\n%s", diff)
	}
}

func TestRecognitionAPIKeysAreEncryptedAndKeptPerConnection(t *testing.T) {
	s, store := newService(t)
	ctx := context.Background()
	const nvidiaKey, anthropicKey = "nvapi-placeholder-value", "sk-placeholder-value"
	got, err := s.Update(ctx, connections(
		AIConnectionUpdate{Name: "NVIDIA", Provider: "nvidia", APIKey: ptr(nvidiaKey), Enabled: true},
		AIConnectionUpdate{Name: "Claude", Provider: "anthropic", APIKey: ptr(anthropicKey), Enabled: true},
	))
	if err != nil {
		t.Fatalf("Update() error = %v", err)
	}
	for k, raw := range store.values {
		if strings.Contains(string(raw), nvidiaKey) || strings.Contains(string(raw), anthropicKey) {
			t.Errorf("stored value for %q contains a key in clear", k)
		}
	}
	nv, cl := got.Recognition.Connections[0], got.Recognition.Connections[1]

	// Reordered, the keys follow their connections; a key left out is kept,
	// an empty one is cleared (and then needed again while enabled).
	got, err = s.Update(ctx, connections(
		AIConnectionUpdate{ID: cl.ID, Name: "Claude", Provider: "anthropic", APIKey: ptr("")},
		AIConnectionUpdate{ID: nv.ID, Name: "NVIDIA", Provider: "nvidia", Enabled: true},
	))
	if err != nil {
		t.Fatalf("Update() error = %v", err)
	}
	want := []AIConnection{
		{ID: cl.ID, Name: "Claude", Provider: "anthropic"},
		{ID: nv.ID, Name: "NVIDIA", Provider: "nvidia", APIKey: nvidiaKey, Enabled: true},
	}
	if diff := cmp.Diff(want, got.Recognition.Connections); diff != "" {
		t.Errorf("connections (-want +got):\n%s", diff)
	}

	// An unknown id is a new connection and never takes another's key.
	got, err = s.Update(ctx, connections(AIConnectionUpdate{ID: "made-up", Name: "NVIDIA", Provider: "nvidia", URL: "http://nim:8000/v1", Enabled: true}))
	if err != nil {
		t.Fatalf("Update() error = %v", err)
	}
	if c := got.Recognition.Connections[0]; c.ID == "made-up" || c.ID == nv.ID || c.APIKey != "" {
		t.Errorf("connection = %+v", c)
	}
}

func TestRecognitionMigratesTheSingleConnection(t *testing.T) {
	s, store := newService(t)
	sealed, err := s.cipher.Seal("sk-placeholder")
	if err != nil {
		t.Fatal(err)
	}
	store.values[instanceKey] = fmt.Appendf(nil, `{"registration_open": true, "recognition_ai_enabled": true,
		"recognition_ai_provider": "anthropic", "recognition_ai_model": "claude-opus-5-5",
		"recognition_ai_api_key_sealed": %q}`, sealed)
	got, err := s.Instance(context.Background())
	if err != nil {
		t.Fatalf("Instance() error = %v", err)
	}
	want := Recognition{AIEnabled: true, Connections: []AIConnection{
		{ID: "default", Name: "Anthropic", Provider: "anthropic", Model: "claude-opus-5-5", APIKey: "sk-placeholder", Enabled: true},
	}}
	if diff := cmp.Diff(want, got.Recognition); diff != "" {
		t.Errorf("Recognition (-want +got):\n%s", diff)
	}

	// Saved once, the old fields are gone and the list stays as saved.
	none := connections()
	none.RecognitionAIEnabled = ptr(false)
	if _, err := s.Update(context.Background(), none); err != nil {
		t.Fatalf("Update() error = %v", err)
	}
	if raw := string(store.values[instanceKey]); strings.Contains(raw, "recognition_ai_provider") {
		t.Errorf("stored = %s", raw)
	}
	if again, _ := s.Instance(context.Background()); len(again.Recognition.Connections) != 0 {
		t.Errorf("connections = %+v, want none", again.Recognition.Connections)
	}
}

func TestRecognitionUntouchedSingleSettingsAreNotMigrated(t *testing.T) {
	s, store := newService(t)
	store.values[instanceKey] = []byte(`{"registration_open": true, "recognition_ai_provider": "ollama"}`)
	got, err := s.Instance(context.Background())
	if err != nil || len(got.Recognition.Connections) != 0 {
		t.Errorf("Instance() = %+v, %v, want no connections", got.Recognition, err)
	}
}

func TestRecognitionValidation(t *testing.T) {
	tooMany := make([]AIConnectionUpdate, MaxAIConnections+1)
	for i := range tooMany {
		tooMany[i] = AIConnectionUpdate{Name: fmt.Sprint("Ollama ", i), Provider: "ollama", URL: "http://ollama:11434", Model: "m"}
	}
	tests := []struct {
		name  string
		u     Update
		field string
	}{
		{"unknown provider", connections(AIConnectionUpdate{Provider: "other"}), "recognition_ai_connections[0].provider"},
		{"bad url", connections(AIConnectionUpdate{Provider: "ollama", URL: "ftp://x", Model: "m"}), "recognition_ai_connections[0].url"},
		{"ollama url needed", connections(AIConnectionUpdate{Provider: "ollama", Model: "m", Enabled: true}), "recognition_ai_connections[0].url"},
		{"ollama model needed", connections(AIConnectionUpdate{Provider: "ollama", URL: "http://ollama:11434", Enabled: true}), "recognition_ai_connections[0].model"},
		{"anthropic key needed", connections(AIConnectionUpdate{Provider: "anthropic", Enabled: true}), "recognition_ai_connections[0].api_key"},
		{"nvidia key needed", connections(
			AIConnectionUpdate{Name: "Local", Provider: "nvidia", URL: "http://nim:8000/v1", Enabled: true},
			AIConnectionUpdate{Name: "Hosted", Provider: "nvidia", Enabled: true},
		), "recognition_ai_connections[1].api_key"},
		{"duplicate names", connections(
			AIConnectionUpdate{Name: "Mine", Provider: "nvidia", APIKey: ptr("k")},
			AIConnectionUpdate{Name: " mine", Provider: "anthropic", APIKey: ptr("k")},
		), "recognition_ai_connections[1].name"},
		{"too many", Update{RecognitionAIConnections: &tooMany}, "recognition_ai_connections"},
		{"enabled without connections", Update{RecognitionAIEnabled: ptr(true)}, "recognition_ai_connections"},
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

func TestRecognitionValidConnections(t *testing.T) {
	s, _ := newService(t)
	// NVIDIA needs no model (it has a default), nor a key for a self-hosted
	// NIM; disabled connections may be incomplete.
	if _, err := s.Update(context.Background(), Update{RecognitionAIEnabled: ptr(true), RecognitionAIConnections: &[]AIConnectionUpdate{
		{Name: "NIM", Provider: "nvidia", URL: "http://nim:8000/v1", Enabled: true},
		{Name: "Later", Provider: "anthropic"},
	}}); err != nil {
		t.Errorf("Update() error = %v", err)
	}
}
