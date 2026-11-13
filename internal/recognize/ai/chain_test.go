package ai

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"

	"github.com/adirkuhn/moneyharpy/internal/recognize"
	"github.com/adirkuhn/moneyharpy/internal/settings"
)

// fakeModel answers with err, or with an extraction naming itself.
type fakeModel struct {
	name    string
	err     error
	pingErr error
	asked   int
}

func (f *fakeModel) Extract(context.Context, string, [][]byte) (recognize.Extraction, error) {
	f.asked++
	if f.err != nil {
		return recognize.Extraction{}, f.err
	}
	return recognize.Extraction{Source: recognize.SourceAI, Reference: recognize.Value[string]{V: f.name}}, nil
}

func (f *fakeModel) Ping(context.Context) error { return f.pingErr }

func testLogger() (*slog.Logger, *bytes.Buffer) {
	var buf bytes.Buffer
	return slog.New(slog.NewTextHandler(&buf, nil)), &buf
}

func TestChainUsesTheFirstThatAnswers(t *testing.T) {
	first := &fakeModel{name: "first", err: ErrRateLimited}
	second := &fakeModel{name: "second", err: errors.New("model answer is not the expected JSON")}
	third := &fakeModel{name: "third"}
	fourth := &fakeModel{name: "fourth"}
	logger, logs := testLogger()
	c := NewChain(logger, Link{"NVIDIA free", first}, Link{"Ollama", second}, Link{"Claude", third}, Link{"Spare", fourth})

	e, err := c.Extract(context.Background(), "secret document text", nil)
	if err != nil || e.Reference.V != "third" {
		t.Fatalf("Extract() = %+v, %v", e, err)
	}
	if first.asked != 1 || second.asked != 1 || third.asked != 1 || fourth.asked != 0 {
		t.Errorf("asked = %d %d %d %d", first.asked, second.asked, third.asked, fourth.asked)
	}
	if !strings.Contains(logs.String(), `connection=Claude`) {
		t.Errorf("logs do not tell which connection answered: %s", logs)
	}
	if strings.Contains(logs.String(), "secret document text") {
		t.Errorf("logs contain the document: %s", logs)
	}
}

func TestChainFallsThroughOnEveryFailure(t *testing.T) {
	for _, fail := range []error{
		ErrUnavailable, ErrRateLimited, ErrRefused, ErrTruncated, ErrUnauthorized, ErrModelNotFound,
		context.DeadlineExceeded, errors.New("unparseable"),
	} {
		logger, _ := testLogger()
		c := NewChain(logger, Link{"a", &fakeModel{err: fail}}, Link{"b", &fakeModel{name: "b"}})
		if e, err := c.Extract(context.Background(), "text", nil); err != nil || e.Reference.V != "b" {
			t.Errorf("%v: Extract() = %+v, %v", fail, e, err)
		}
	}
}

func TestChainAllFail(t *testing.T) {
	logger, _ := testLogger()
	c := NewChain(logger, Link{"NVIDIA", &fakeModel{err: ErrRateLimited}}, Link{"Ollama", &fakeModel{err: ErrUnavailable}})
	_, err := c.Extract(context.Background(), "text", nil)
	if !errors.Is(err, ErrRateLimited) || !errors.Is(err, ErrUnavailable) ||
		!strings.Contains(err.Error(), "NVIDIA") || !strings.Contains(err.Error(), "Ollama") {
		t.Errorf("err = %v", err)
	}
	if _, err := NewChain(logger).Extract(context.Background(), "text", nil); !errors.Is(err, ErrConfig) {
		t.Errorf("empty chain: err = %v", err)
	}
}

func TestChainStops(t *testing.T) {
	logger, _ := testLogger()
	second := &fakeModel{name: "b"}
	c := NewChain(logger, Link{"a", &fakeModel{err: ErrNothingToRead}}, Link{"b", second})
	if _, err := c.Extract(context.Background(), "", nil); !errors.Is(err, ErrNothingToRead) || second.asked != 0 {
		t.Errorf("nothing to read: err = %v, asked = %d", err, second.asked)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	c = NewChain(logger, Link{"a", &fakeModel{err: context.Canceled}}, Link{"b", second})
	if _, err := c.Extract(ctx, "text", nil); !errors.Is(err, context.Canceled) || second.asked != 0 {
		t.Errorf("canceled: err = %v, asked = %d", err, second.asked)
	}
}

func TestChainPing(t *testing.T) {
	logger, _ := testLogger()
	ok := NewChain(logger, Link{"a", &fakeModel{pingErr: ErrUnauthorized}}, Link{"b", &fakeModel{}})
	if err := ok.Ping(context.Background()); err != nil {
		t.Errorf("Ping() = %v, want nil when one answers", err)
	}
	bad := NewChain(logger, Link{"a", &fakeModel{pingErr: ErrUnauthorized}})
	if err := bad.Ping(context.Background()); !errors.Is(err, ErrUnauthorized) {
		t.Errorf("Ping() = %v", err)
	}
}

func TestFromSettingsSkipsDisabledConnections(t *testing.T) {
	logger, _ := testLogger()
	m, err := FromSettings(settings.Recognition{AIEnabled: true, Connections: []settings.AIConnection{
		{Name: "Off", Provider: ProviderAnthropic, APIKey: "k"},
		{Name: "NVIDIA", Provider: ProviderNVIDIA, APIKey: "nvapi-placeholder", Enabled: true},
		{Name: "Broken", Provider: ProviderOllama, Enabled: true},
		{Name: "Ollama", Provider: ProviderOllama, URL: "http://ollama:11434", Model: "qwen2.5vl", Enabled: true},
	}}, logger)
	if err != nil {
		t.Fatal(err)
	}
	c := m.(*Chain)
	var names []string
	for _, l := range c.links {
		names = append(names, l.Name)
	}
	if strings.Join(names, ",") != "NVIDIA,Broken,Ollama" {
		t.Errorf("links = %v", names)
	}
	// A connection that cannot be built fails when asked, so the next is used.
	if _, ok := c.links[1].Model.(broken); !ok {
		t.Errorf("Broken = %T", c.links[1].Model)
	}
	if _, err := FromSettings(settings.Recognition{Connections: []settings.AIConnection{{Name: "Off", Provider: ProviderOllama}}}, logger); !errors.Is(err, ErrConfig) {
		t.Errorf("no enabled connections: err = %v", err)
	}
}
