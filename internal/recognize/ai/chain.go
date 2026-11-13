package ai

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/adirkuhn/moneyharpy/internal/recognize"
	"github.com/adirkuhn/moneyharpy/internal/settings"
)

// Link is one AI connection of a Chain.
type Link struct {
	Name  string
	Model recognize.Model
}

// Chain asks its connections in order and returns the first answer: when a
// connection fails for any reason (unavailable, rate limited, refused, a
// wrong key, an unreadable answer) the next one is asked.
type Chain struct {
	links  []Link
	logger *slog.Logger
}

// NewChain builds a chain of the links, in order.
func NewChain(logger *slog.Logger, links ...Link) *Chain {
	return &Chain{links: links, logger: logger}
}

// FromSettings builds the chain of the enabled connections, in their order.
func FromSettings(cfg settings.Recognition, logger *slog.Logger) (recognize.Model, error) {
	var links []Link
	for _, c := range cfg.Connections {
		if !c.Enabled {
			continue
		}
		m, err := New(Config{Provider: c.Provider, URL: c.URL, Model: c.Model, APIKey: c.APIKey})
		if err != nil {
			m = broken{err}
		}
		links = append(links, Link{Name: c.Name, Model: m})
	}
	if len(links) == 0 {
		return nil, fmt.Errorf("%w: no AI connection is enabled", ErrConfig)
	}
	return NewChain(logger, links...), nil
}

// Extract returns the answer of the first connection that reads the document.
func (c *Chain) Extract(ctx context.Context, text string, pages [][]byte) (recognize.Extraction, error) {
	if len(c.links) == 0 {
		return recognize.Extraction{}, fmt.Errorf("%w: no AI connection is enabled", ErrConfig)
	}
	var errs []error
	for _, l := range c.links {
		e, err := l.Model.Extract(ctx, text, pages)
		if err == nil {
			c.logger.InfoContext(ctx, "scan and recognise: AI connection answered", "connection", l.Name)
			return e, nil
		}
		if errors.Is(err, ErrNothingToRead) || ctx.Err() != nil {
			return recognize.Extraction{}, err
		}
		// Errors carry the provider's explanation, never the document.
		c.logger.WarnContext(ctx, "scan and recognise: AI connection failed", "connection", l.Name, "error", err)
		errs = append(errs, fmt.Errorf("%s: %w", l.Name, err))
	}
	return recognize.Extraction{}, errors.Join(errs...)
}

// Ping succeeds when at least one connection answers.
func (c *Chain) Ping(ctx context.Context) error {
	if len(c.links) == 0 {
		return fmt.Errorf("%w: no AI connection is enabled", ErrConfig)
	}
	var errs []error
	for _, l := range c.links {
		err := l.Model.Ping(ctx)
		if err == nil {
			return nil
		}
		errs = append(errs, fmt.Errorf("%s: %w", l.Name, err))
	}
	return errors.Join(errs...)
}

// broken stands for a connection whose settings cannot build a model, so the
// chain skips past it with the reason.
type broken struct{ err error }

func (b broken) Extract(context.Context, string, [][]byte) (recognize.Extraction, error) {
	return recognize.Extraction{}, b.err
}

func (b broken) Ping(context.Context) error { return b.err }
