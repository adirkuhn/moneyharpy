// Package pdok completes Dutch addresses through the government's public
// address service.
package pdok

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"

	"github.com/adirkuhn/moneyharpy/internal/administration"
	"github.com/adirkuhn/moneyharpy/internal/settings"
)

// ProductionURL is the base address of the PDOK location service.
const ProductionURL = "https://api.pdok.nl/bzk/locatieserver/search/v3_1"

const maxBodyBytes = 1 << 18

// Both patterns also keep user input from altering the service's search query.
var (
	zipcodePattern     = regexp.MustCompile(`^[0-9]{4}[A-Z]{2}$`)
	houseNumberPattern = regexp.MustCompile(`^([0-9]{1,6})[ -]?[A-Za-z0-9]{0,8}$`)
)

// Settings tells whether lookups are enabled.
type Settings interface {
	Instance(ctx context.Context) (settings.Instance, error)
}

// Client talks to PDOK.
type Client struct {
	settings Settings
	baseURL  string
	http     *http.Client
}

// New returns a Client using the service at baseURL.
func New(s Settings, baseURL string, httpClient *http.Client) *Client {
	return &Client{settings: s, baseURL: baseURL, http: httpClient}
}

type searchResponse struct {
	Response struct {
		Docs []struct {
			Street  string `json:"straatnaam"`
			Zipcode string `json:"postcode"`
			City    string `json:"woonplaatsnaam"`
		} `json:"docs"`
	} `json:"response"`
}

// LookupAddress finds an address. The house number is kept as typed, so a
// suffix such as "147-a" survives even though only the digits are searched.
func (c *Client) LookupAddress(ctx context.Context, zipcode, houseNumber string) (administration.AddressMatch, error) {
	inst, err := c.settings.Instance(ctx)
	if err != nil {
		return administration.AddressMatch{}, fmt.Errorf("read settings: %w", err)
	}
	if !inst.AddressLookupEnabled {
		return administration.AddressMatch{}, administration.ErrLookupUnavailable
	}

	zipcode = strings.ToUpper(strings.ReplaceAll(zipcode, " ", ""))
	houseNumber = strings.TrimSpace(houseNumber)
	number := houseNumberPattern.FindStringSubmatch(houseNumber)
	if !zipcodePattern.MatchString(zipcode) || number == nil {
		return administration.AddressMatch{}, administration.ErrAddressUnknown
	}

	params := url.Values{
		"q":    {"postcode:" + zipcode + " and huisnummer:" + number[1]},
		"fq":   {"type:adres"},
		"rows": {"1"},
		"fl":   {"straatnaam,postcode,woonplaatsnaam"},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/free?"+params.Encode(), nil)
	if err != nil {
		return administration.AddressMatch{}, fmt.Errorf("build request: %w", err)
	}
	res, err := c.http.Do(req)
	if err != nil {
		return administration.AddressMatch{}, fmt.Errorf("call pdok: %w", err)
	}
	defer func() {
		// The body has been read or abandoned; nothing depends on closing it.
		_ = res.Body.Close()
	}()
	if res.StatusCode != http.StatusOK {
		return administration.AddressMatch{}, fmt.Errorf("pdok answered %d", res.StatusCode)
	}

	var body searchResponse
	if err := json.NewDecoder(io.LimitReader(res.Body, maxBodyBytes)).Decode(&body); err != nil {
		return administration.AddressMatch{}, fmt.Errorf("decode pdok response: %w", err)
	}
	if len(body.Response.Docs) == 0 {
		return administration.AddressMatch{}, administration.ErrAddressUnknown
	}
	doc := body.Response.Docs[0]
	return administration.AddressMatch{
		Address1: doc.Street + " " + houseNumber,
		Zipcode:  doc.Zipcode,
		City:     doc.City,
	}, nil
}
