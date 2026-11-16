package httpapi

import (
	"encoding/base64"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/adirkuhn/moneyharpy/internal/resource"
	"github.com/adirkuhn/moneyharpy/internal/sales"
)

type onlineDetailJSON struct {
	Description   string `json:"description"`
	Amount        string `json:"amount"`
	Price         string `json:"price"`
	Total         string `json:"total_price_excl_tax_with_discount"`
	TaxPercentage string `json:"tax_percentage"`
}

type onlineTaxJSON struct {
	TaxPercentage string `json:"tax_percentage"`
	Taxable       string `json:"taxable_amount"`
	Tax           string `json:"tax_amount"`
}

type onlineSenderJSON struct {
	CompanyName       string `json:"company_name"`
	Address1          string `json:"address1"`
	Address2          string `json:"address2"`
	Zipcode           string `json:"zipcode"`
	City              string `json:"city"`
	Country           string `json:"country"`
	Email             string `json:"email"`
	Phone             string `json:"phone"`
	ChamberOfCommerce string `json:"chamber_of_commerce"`
	TaxNumber         string `json:"tax_number"`
}

type onlineEstimateJSON struct {
	ID                string             `json:"id"`
	EstimateID        *string            `json:"estimate_id"`
	State             string             `json:"state"`
	EstimateDate      *string            `json:"estimate_date"`
	DueDate           *string            `json:"due_date"`
	Reference         *string            `json:"reference"`
	Currency          string             `json:"currency"`
	Language          string             `json:"language"`
	PricesAreInclTax  bool               `json:"prices_are_incl_tax"`
	PreText           *string            `json:"pre_text"`
	PostText          *string            `json:"post_text"`
	Details           []onlineDetailJSON `json:"details"`
	TotalPriceExclTax string             `json:"total_price_excl_tax"`
	TotalTax          string             `json:"total_tax"`
	TotalPriceInclTax string             `json:"total_price_incl_tax"`
	TaxTotals         []onlineTaxJSON    `json:"tax_totals"`
	Sender            onlineSenderJSON   `json:"sender"`
	ContactName       string             `json:"contact_name"`
	Signable          bool               `json:"signable"`
	Signed            bool               `json:"signed"`
	SignerName        *string            `json:"signer_name"`
	SignedAt          *time.Time         `json:"signed_at"`
}

// plainText is a quote text with its tags filled in, without its styling.
func plainText(doc sales.Document, text string) *string {
	if strings.TrimSpace(text) == "" {
		return nil
	}
	var b strings.Builder
	for _, sp := range sales.EstimateTextSpans(doc, text, time.Now().UTC()) {
		b.WriteString(sp.Text)
	}
	out := b.String()
	return &out
}

func toOnlineEstimate(o sales.OnlineEstimate) onlineEstimateJSON {
	e, doc := o.Estimate, o.Document
	percentage := func(id resource.ID) string { return doc.Rates[id].Percentage }
	id := doc.Identity
	out := onlineEstimateJSON{
		ID: e.ID.String(), EstimateID: nz(e.EstimateID), State: e.State, EstimateDate: dateText(e.EstimateDate), DueDate: dateText(e.DueDate),
		Reference: nz(e.Reference), Currency: e.Currency, Language: e.Language, PricesAreInclTax: e.PricesAreInclTax,
		PreText: plainText(doc, e.PreText), PostText: plainText(doc, e.PostText), Details: []onlineDetailJSON{},
		TotalPriceExclTax: e.Totals.ExclTax.String(), TotalTax: e.Totals.Tax.String(), TotalPriceInclTax: e.Totals.InclTax.String(), TaxTotals: []onlineTaxJSON{},
		Sender: onlineSenderJSON{
			CompanyName: id.CompanyName, Address1: id.Address1, Address2: id.Address2, Zipcode: id.Zipcode, City: id.City, Country: id.Country,
			Email: id.Email, Phone: id.Phone, ChamberOfCommerce: id.ChamberOfCommerce, TaxNumber: id.TaxNumber,
		},
		ContactName: o.ContactName(), Signable: o.Signable,
	}
	for i, d := range e.Lines {
		if d.Optional && !d.Selected {
			continue
		}
		total := "0.0"
		if i < len(e.Totals.LineBases) {
			total = e.Totals.LineBases[i].String()
		}
		out.Details = append(out.Details, onlineDetailJSON{Description: d.Description, Amount: d.Amount, Price: d.Price.String(), Total: total, TaxPercentage: percentage(d.TaxRateID)})
	}
	for _, r := range e.Totals.ByRate {
		out.TaxTotals = append(out.TaxTotals, onlineTaxJSON{TaxPercentage: percentage(r.TaxRateID), Taxable: r.Taxable.String(), Tax: r.Tax.String()})
	}
	if g := e.Signing; g != nil && g.Signed() {
		out.Signed, out.SignerName, out.SignedAt = true, &g.SignerName, g.SignedAt
	}
	return out
}

// failOnline answers a problem of an online estimate without telling
// whether the estimate exists.
func (s *server) failOnline(w http.ResponseWriter, r *http.Request, err error) {
	if errors.Is(err, sales.ErrEstimateNotFound) {
		writeError(w, http.StatusNotFound, "Not found")
		return
	}
	s.failEstimate(w, r, err)
}

// GetOnlineEstimate implements ServerInterface.
func (s *server) GetOnlineEstimate(w http.ResponseWriter, r *http.Request, id Id, params GetOnlineEstimateParams) {
	rid, err := resource.ParseID(id)
	if err != nil {
		writeError(w, http.StatusNotFound, "Not found")
		return
	}
	o, err := s.deps.Sales.OnlineEstimate(r.Context(), rid, params.Code)
	if err != nil {
		s.failOnline(w, r, err)
		return
	}
	w.Header().Set("Cache-Control", "private, no-store")
	writeJSON(w, http.StatusOK, toOnlineEstimate(o))
}

// DownloadOnlineEstimatePdf implements ServerInterface.
func (s *server) DownloadOnlineEstimatePdf(w http.ResponseWriter, r *http.Request, id Id, params DownloadOnlineEstimatePdfParams) {
	rid, err := resource.ParseID(id)
	if err != nil {
		writeError(w, http.StatusNotFound, "Not found")
		return
	}
	data, name, err := s.deps.Sales.OnlineEstimatePDF(r.Context(), rid, params.Code)
	if err != nil {
		s.failOnline(w, r, err)
		return
	}
	w.Header().Set("Cache-Control", "private, no-store")
	writeDocument(w, "application/pdf", name, data)
}

// SignOnlineEstimate implements ServerInterface.
func (s *server) SignOnlineEstimate(w http.ResponseWriter, r *http.Request, id Id, params SignOnlineEstimateParams) {
	rid, err := resource.ParseID(id)
	if err != nil {
		writeError(w, http.StatusNotFound, "Not found")
		return
	}
	body, ok := decodeBody[SignOnlineEstimateRequest](w, r)
	if !ok {
		return
	}
	in := sales.SignInput{
		Name: body.Name, Email: body.Email, Signature: pngDataURL(body.SignatureOutput), AuditTrail: deref(body.AuditTrail),
		IP: clientIP(r, s.deps.TrustProxyHeaders), UserAgent: r.UserAgent(),
	}
	if _, err := s.deps.Sales.SignEstimateOnline(r.Context(), rid, params.Code, in); err != nil && !errors.Is(err, sales.ErrEstimateDelivery) {
		s.failOnline(w, r, err)
		return
	}
	// Signed, even when the signed quote could not be emailed: the contact
	// sees the estimate accepted.
	o, err := s.deps.Sales.OnlineEstimate(r.Context(), rid, params.Code)
	if err != nil {
		s.failOnline(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, toOnlineEstimate(o))
}

// pngDataURL decodes a PNG data URL, as a signature pad gives; anything
// else is empty, which the signing refuses.
func pngDataURL(v string) []byte {
	data, ok := strings.CutPrefix(strings.TrimSpace(v), "data:image/png;base64,")
	if !ok {
		return nil
	}
	out, err := base64.StdEncoding.DecodeString(data)
	if err != nil {
		return nil
	}
	return out
}
