// Package kvk searches the company register of the Dutch Chamber of Commerce.
package kvk

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/adirkuhn/moneyharpy/internal/administration"
	"github.com/adirkuhn/moneyharpy/internal/settings"
)

// ProductionURL is the base address of the KVK API.
const ProductionURL = "https://api.kvk.nl/api/v2"

const (
	resultsPerPage = "10"
	maxBodyBytes   = 1 << 20
)

// Settings provides the configured API key.
type Settings interface {
	Instance(ctx context.Context) (settings.Instance, error)
}

// Client talks to the KVK API.
type Client struct {
	settings Settings
	baseURL  string
	http     *http.Client
}

// New returns a Client using the API at baseURL.
func New(s Settings, baseURL string, httpClient *http.Client) *Client {
	return &Client{settings: s, baseURL: baseURL, http: httpClient}
}

type searchResponse struct {
	Results []struct {
		Number  string `json:"kvkNummer"`
		Name    string `json:"naam"`
		Address struct {
			Domestic struct {
				Street      string `json:"straatnaam"`
				HouseNumber int    `json:"huisnummer"`
				HouseLetter string `json:"huisletter"`
				Zipcode     string `json:"postcode"`
				City        string `json:"plaats"`
			} `json:"binnenlandsAdres"`
		} `json:"adres"`
	} `json:"resultaten"`
}

// SearchCompanies finds companies by name. The API answers 404 when nothing
// matches, which is an ordinary empty result here.
func (c *Client) SearchCompanies(ctx context.Context, query string) ([]administration.CompanyMatch, error) {
	return c.zoeken(ctx, url.Values{"naam": {query}, "resultatenPerPagina": {resultsPerPage}})
}

// LookupCompany finds the company with a KVK number, preferring the
// result that has an address: the API lists the legal entity and its
// branches separately. A number it does not know is ErrCompanyNotFound.
func (c *Client) LookupCompany(ctx context.Context, number string) (administration.CompanyMatch, error) {
	found, err := c.zoeken(ctx, url.Values{"kvkNummer": {number}, "resultatenPerPagina": {resultsPerPage}})
	if err != nil {
		return administration.CompanyMatch{}, err
	}
	var best *administration.CompanyMatch
	for i, m := range found {
		if m.ChamberOfCommerce != number {
			continue
		}
		if best == nil || (best.Address1 == "" && m.Address1 != "") {
			best = &found[i]
		}
	}
	if best == nil {
		return administration.CompanyMatch{}, administration.ErrCompanyNotFound
	}
	return *best, nil
}

func (c *Client) zoeken(ctx context.Context, params url.Values) ([]administration.CompanyMatch, error) {
	inst, err := c.settings.Instance(ctx)
	if err != nil {
		return nil, fmt.Errorf("read settings: %w", err)
	}
	if !inst.KVK.Enabled {
		return nil, administration.ErrLookupUnavailable
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/zoeken?"+params.Encode(), nil)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("apikey", inst.KVK.APIKey)

	res, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("call kvk: %w", err)
	}
	defer func() {
		// The body has been read or abandoned; nothing depends on closing it.
		_ = res.Body.Close()
	}()
	if res.StatusCode == http.StatusNotFound {
		return nil, nil
	}
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("kvk answered %d", res.StatusCode)
	}

	var body searchResponse
	if err := json.NewDecoder(io.LimitReader(res.Body, maxBodyBytes)).Decode(&body); err != nil {
		return nil, fmt.Errorf("decode kvk response: %w", err)
	}
	matches := make([]administration.CompanyMatch, len(body.Results))
	for i, r := range body.Results {
		a := r.Address.Domestic
		number := ""
		if a.HouseNumber != 0 {
			number = strconv.Itoa(a.HouseNumber) + a.HouseLetter
		}
		matches[i] = administration.CompanyMatch{
			Name:              r.Name,
			ChamberOfCommerce: r.Number,
			Address1:          strings.TrimSpace(a.Street + " " + number),
			Zipcode:           a.Zipcode,
			City:              a.City,
		}
	}
	return matches, nil
}
