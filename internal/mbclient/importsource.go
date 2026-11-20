package mbclient

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"time"

	"github.com/adirkuhn/moneyharpy/internal/mbimport"
)

// ImportSource opens Moneybird for an import. Each import gets its own
// client, which keeps Moneybird's rate limit for its token.
type ImportSource struct {
	// BaseURL is Moneybird's API; empty means DefaultBaseURL.
	BaseURL string
	HTTP    *http.Client
}

func (s ImportSource) client(token string, onWait func(time.Time)) *Client {
	return &Client{BaseURL: s.BaseURL, Token: token, HTTP: s.HTTP, OnWait: onWait}
}

// Administrations implements mbimport.Source.
func (s ImportSource) Administrations(ctx context.Context, token string) ([]mbimport.SourceAdministration, error) {
	list, err := s.client(token, nil).Administrations(ctx)
	if err != nil {
		return nil, importError(err)
	}
	out := make([]mbimport.SourceAdministration, len(list))
	for i, a := range list {
		out[i] = mbimport.SourceAdministration{ID: a.ID, Name: a.Name, Country: a.Country, Currency: a.Currency}
	}
	return out, nil
}

// Open implements mbimport.Source.
func (s ImportSource) Open(token string, onWait func(until time.Time)) mbimport.Reader {
	return importReader{c: s.client(token, onWait)}
}

type importReader struct{ c *Client }

func (r importReader) List(ctx context.Context, adminID, path string, query url.Values) ([]json.RawMessage, error) {
	out, err := r.c.List(ctx, adminID, path, query)
	return out, importError(err)
}

func (r importReader) SyncIDs(ctx context.Context, adminID, kind string, query url.Values) ([]mbimport.Version, error) {
	list, err := r.c.SyncIDs(ctx, adminID, kind, query)
	if err != nil {
		return nil, importError(err)
	}
	out := make([]mbimport.Version, len(list))
	for i, v := range list {
		out[i] = mbimport.Version{ID: v.ID, Version: v.Version}
	}
	return out, nil
}

func (r importReader) FetchByIDs(ctx context.Context, adminID, kind string, ids []string) ([]json.RawMessage, error) {
	out, err := r.c.FetchByIDs(ctx, adminID, kind, ids)
	return out, importError(err)
}

// importError tells the import that Moneybird refused the token, which ends
// it; other errors let it try again later.
func importError(err error) error {
	if errors.Is(err, ErrTokenRejected) {
		return mbimport.ErrTokenRejected
	}
	return err
}
