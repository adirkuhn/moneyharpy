package recognize

import (
	"bytes"
	"encoding/xml"
	"errors"
	"io"
	"math/big"
	"strings"
	"time"

	"github.com/adirkuhn/moneyharpy/internal/money"
)

// ErrNotEInvoice is returned for XML that is not a UBL Invoice or CreditNote
// or a UN/CEFACT Cross Industry Invoice.
var ErrNotEInvoice = errors.New("not a UBL or CII e-invoice")

// ParseEInvoice reads a structured e-invoice: UBL 2.1 Invoice or CreditNote
// (Peppol BIS 3, NLCIUS) or UN/CEFACT CII (Factur-X, ZUGFeRD, XRechnung). All
// values it finds are certain. Credit notes (and invoices with type code 381)
// get negative amounts. Elements are matched by local name, so any namespace
// prefixes work.
func ParseEInvoice(data []byte) (Extraction, error) {
	root, err := rootElement(data)
	if err != nil {
		return Extraction{}, err
	}
	switch root {
	case "Invoice", "CreditNote":
		var doc ublDocument
		if err := xml.Unmarshal(data, &doc); err != nil {
			return Extraction{}, err
		}
		return doc.extraction(root == "CreditNote"), nil
	case "CrossIndustryInvoice":
		var doc ciiDocument
		if err := xml.Unmarshal(data, &doc); err != nil {
			return Extraction{}, err
		}
		return doc.extraction(), nil
	}
	return Extraction{}, ErrNotEInvoice
}

func rootElement(data []byte) (string, error) {
	dec := xml.NewDecoder(bytes.NewReader(data))
	for {
		tok, err := dec.Token()
		if err == io.EOF {
			return "", ErrNotEInvoice
		}
		if err != nil {
			return "", ErrNotEInvoice
		}
		if se, ok := tok.(xml.StartElement); ok {
			return se.Name.Local, nil
		}
	}
}

// --- UBL ---

type idWithScheme struct {
	Value  string `xml:",chardata"`
	Scheme string `xml:"schemeID,attr"`
}

type ublAmount struct {
	Value    string `xml:",chardata"`
	Currency string `xml:"currencyID,attr"`
}

type ublTaxCategory struct {
	ID      string `xml:"ID"`
	Percent string `xml:"Percent"`
}

type ublParty struct {
	EndpointID          idWithScheme   `xml:"EndpointID"`
	PartyIdentification []idWithScheme `xml:"PartyIdentification>ID"`
	PartyName           string         `xml:"PartyName>Name"`
	PostalAddress       struct {
		StreetName           string `xml:"StreetName"`
		AdditionalStreetName string `xml:"AdditionalStreetName"`
		CityName             string `xml:"CityName"`
		PostalZone           string `xml:"PostalZone"`
		Country              string `xml:"Country>IdentificationCode"`
	} `xml:"PostalAddress"`
	PartyTaxScheme []struct {
		CompanyID string `xml:"CompanyID"`
		TaxScheme string `xml:"TaxScheme>ID"`
	} `xml:"PartyTaxScheme"`
	PartyLegalEntity struct {
		RegistrationName string       `xml:"RegistrationName"`
		CompanyID        idWithScheme `xml:"CompanyID"`
	} `xml:"PartyLegalEntity"`
	Contact struct {
		Telephone      string `xml:"Telephone"`
		ElectronicMail string `xml:"ElectronicMail"`
	} `xml:"Contact"`
}

type ublLine struct {
	InvoicedQuantity    string    `xml:"InvoicedQuantity"`
	CreditedQuantity    string    `xml:"CreditedQuantity"`
	LineExtensionAmount ublAmount `xml:"LineExtensionAmount"`
	Item                struct {
		Name        string         `xml:"Name"`
		Description string         `xml:"Description"`
		Tax         ublTaxCategory `xml:"ClassifiedTaxCategory"`
	} `xml:"Item"`
	Price struct {
		Amount       ublAmount `xml:"PriceAmount"`
		BaseQuantity string    `xml:"BaseQuantity"`
	} `xml:"Price"`
}

type ublDocument struct {
	ID                string   `xml:"ID"`
	IssueDate         string   `xml:"IssueDate"`
	DueDate           string   `xml:"DueDate"`
	InvoiceTypeCode   string   `xml:"InvoiceTypeCode"`
	Currency          string   `xml:"DocumentCurrencyCode"`
	PaymentTermsNotes []string `xml:"PaymentTerms>Note"`
	Supplier          ublParty `xml:"AccountingSupplierParty>Party"`
	PaymentMeans      []struct {
		PaymentID      string `xml:"PaymentID"`
		PaymentDueDate string `xml:"PaymentDueDate"`
		IBAN           string `xml:"PayeeFinancialAccount>ID"`
	} `xml:"PaymentMeans"`
	TaxTotal []struct {
		TaxAmount ublAmount `xml:"TaxAmount"`
		Subtotals []struct {
			Taxable  ublAmount      `xml:"TaxableAmount"`
			Tax      ublAmount      `xml:"TaxAmount"`
			Category ublTaxCategory `xml:"TaxCategory"`
		} `xml:"TaxSubtotal"`
	} `xml:"TaxTotal"`
	Totals struct {
		LineExtension ublAmount `xml:"LineExtensionAmount"`
		TaxExclusive  ublAmount `xml:"TaxExclusiveAmount"`
		TaxInclusive  ublAmount `xml:"TaxInclusiveAmount"`
		Payable       ublAmount `xml:"PayableAmount"`
	} `xml:"LegalMonetaryTotal"`
	InvoiceLines    []ublLine `xml:"InvoiceLine"`
	CreditNoteLines []ublLine `xml:"CreditNoteLine"`
}

func (d ublDocument) extraction(creditNote bool) Extraction {
	sign := money.Amount(1)
	if creditNote || strings.TrimSpace(d.InvoiceTypeCode) == "381" {
		sign = -1
	}
	e := eInvoiceBase()
	e.Supplier = d.Supplier.supplier()
	e.Reference = sureString(d.ID)
	e.Date = sureDate(d.IssueDate)
	e.Currency = sureString(strings.ToUpper(strings.TrimSpace(d.Currency)))
	e.DueDate = sureDate(d.DueDate)
	for _, pm := range d.PaymentMeans {
		if !e.DueDate.Found {
			e.DueDate = sureDate(pm.PaymentDueDate)
		}
		if e.Supplier.IBAN == "" && ValidIBAN(pm.IBAN) {
			e.Supplier.IBAN = NormalizeIBAN(pm.IBAN)
		}
		if !e.PaymentReference.Found {
			e.PaymentReference = sureString(pm.PaymentID)
		}
	}
	if !e.DueDate.Found && e.Date.Found {
		for _, note := range d.PaymentTermsNotes {
			if days, ok := termDays(note); ok {
				e.DueDate = sure(e.Date.V.AddDate(0, 0, days))
				break
			}
		}
	}
	if a, ok := decimalAmount(d.Totals.TaxExclusive.Value); ok {
		e.TotalExclTax = sure(sign * a)
	} else if a, ok := decimalAmount(d.Totals.LineExtension.Value); ok {
		e.TotalExclTax = sure(sign * a)
	}
	if a, ok := decimalAmount(d.Totals.TaxInclusive.Value); ok {
		e.TotalInclTax = sure(sign * a)
	} else if a, ok := decimalAmount(d.Totals.Payable.Value); ok {
		e.TotalInclTax = sure(sign * a)
	}
	for _, tt := range d.TaxTotal {
		if len(tt.Subtotals) == 0 || len(e.VAT) > 0 {
			continue
		}
		for _, st := range tt.Subtotals {
			taxable, _ := decimalAmount(st.Taxable.Value)
			tax, _ := decimalAmount(st.Tax.Value)
			e.VAT = append(e.VAT, VAT{Rate: rateString(st.Category.Percent), Taxable: sign * taxable, Tax: sign * tax})
		}
	}
	for _, l := range append(d.InvoiceLines, d.CreditNoteLines...) {
		qty := l.InvoicedQuantity
		if qty == "" {
			qty = l.CreditedQuantity
		}
		e.Lines = append(e.Lines, eLine(
			firstNonEmpty(l.Item.Name, l.Item.Description), qty,
			l.Price.Amount.Value, l.Price.BaseQuantity, l.LineExtensionAmount.Value,
			l.Item.Tax.Percent, sign))
	}
	return e
}

func (p ublParty) supplier() Supplier {
	s := Supplier{
		Name:     strings.TrimSpace(firstNonEmpty(p.PartyLegalEntity.RegistrationName, p.PartyName)),
		Email:    strings.TrimSpace(p.Contact.ElectronicMail),
		Phone:    strings.TrimSpace(p.Contact.Telephone),
		Address1: joinNonEmpty(" ", p.PostalAddress.StreetName, p.PostalAddress.AdditionalStreetName),
		Zipcode:  strings.TrimSpace(p.PostalAddress.PostalZone),
		City:     strings.TrimSpace(p.PostalAddress.CityName),
		Country:  strings.ToUpper(strings.TrimSpace(p.PostalAddress.Country)),
	}
	for _, ts := range p.PartyTaxScheme {
		if s.VATNumber == "" && (ts.TaxScheme == "" || strings.EqualFold(ts.TaxScheme, "VAT")) {
			s.VATNumber = NormalizeVAT(ts.CompanyID)
		}
	}
	ids := append([]idWithScheme{p.PartyLegalEntity.CompanyID, p.EndpointID}, p.PartyIdentification...)
	for _, id := range ids {
		v := strings.TrimSpace(id.Value)
		switch {
		case s.KVK == "" && isKVK(v) && (id.Scheme == "0106" || id.Scheme == "" && s.Country == "NL"):
			s.KVK = v
		case s.VATNumber == "" && id.Scheme == "9944":
			s.VATNumber = NormalizeVAT(v)
		case s.Email == "" && id.Scheme == "EM":
			s.Email = v
		}
	}
	return s
}

// --- CII ---

type ciiDate struct {
	Value string `xml:"DateTimeString"`
}

type ciiDocument struct {
	ID       string  `xml:"ExchangedDocument>ID"`
	TypeCode string  `xml:"ExchangedDocument>TypeCode"`
	Issue    ciiDate `xml:"ExchangedDocument>IssueDateTime"`
	Trade    struct {
		Lines []struct {
			Name        string `xml:"SpecifiedTradeProduct>Name"`
			Description string `xml:"SpecifiedTradeProduct>Description"`
			Price       string `xml:"SpecifiedLineTradeAgreement>NetPriceProductTradePrice>ChargeAmount"`
			BasisQty    string `xml:"SpecifiedLineTradeAgreement>NetPriceProductTradePrice>BasisQuantity"`
			Quantity    string `xml:"SpecifiedLineTradeDelivery>BilledQuantity"`
			Rate        string `xml:"SpecifiedLineTradeSettlement>ApplicableTradeTax>RateApplicablePercent"`
			Total       string `xml:"SpecifiedLineTradeSettlement>SpecifiedTradeSettlementLineMonetarySummation>LineTotalAmount"`
		} `xml:"IncludedSupplyChainTradeLineItem"`
		Seller struct {
			Name     string `xml:"Name"`
			LegalOrg struct {
				ID          idWithScheme `xml:"ID"`
				TradingName string       `xml:"TradingBusinessName"`
			} `xml:"SpecifiedLegalOrganization"`
			Phone   string `xml:"DefinedTradeContact>TelephoneUniversalCommunication>CompleteNumber"`
			Email   string `xml:"DefinedTradeContact>EmailURIUniversalCommunication>URIID"`
			Email2  string `xml:"URIUniversalCommunication>URIID"`
			Address struct {
				Postcode string `xml:"PostcodeCode"`
				LineOne  string `xml:"LineOne"`
				LineTwo  string `xml:"LineTwo"`
				City     string `xml:"CityName"`
				Country  string `xml:"CountryID"`
			} `xml:"PostalTradeAddress"`
			TaxRegistrations []idWithScheme `xml:"SpecifiedTaxRegistration>ID"`
		} `xml:"ApplicableHeaderTradeAgreement>SellerTradeParty"`
		Settlement struct {
			PaymentReference string   `xml:"PaymentReference"`
			Currency         string   `xml:"InvoiceCurrencyCode"`
			IBANs            []string `xml:"SpecifiedTradeSettlementPaymentMeans>PayeePartyCreditorFinancialAccount>IBANID"`
			Taxes            []struct {
				Calculated string `xml:"CalculatedAmount"`
				Basis      string `xml:"BasisAmount"`
				Rate       string `xml:"RateApplicablePercent"`
			} `xml:"ApplicableTradeTax"`
			Terms []struct {
				Description string  `xml:"Description"`
				Due         ciiDate `xml:"DueDateDateTime"`
			} `xml:"SpecifiedTradePaymentTerms"`
			Summation struct {
				LineTotal  string `xml:"LineTotalAmount"`
				TaxBasis   string `xml:"TaxBasisTotalAmount"`
				GrandTotal string `xml:"GrandTotalAmount"`
				DuePayable string `xml:"DuePayableAmount"`
			} `xml:"SpecifiedTradeSettlementHeaderMonetarySummation"`
		} `xml:"ApplicableHeaderTradeSettlement"`
	} `xml:"SupplyChainTradeTransaction"`
}

func (d ciiDocument) extraction() Extraction {
	sign := money.Amount(1)
	if strings.TrimSpace(d.TypeCode) == "381" {
		sign = -1
	}
	t := d.Trade
	e := eInvoiceBase()
	seller := t.Seller
	e.Supplier = Supplier{
		Name:     strings.TrimSpace(seller.Name),
		Email:    strings.TrimSpace(firstNonEmpty(seller.Email, seller.Email2)),
		Phone:    strings.TrimSpace(seller.Phone),
		Address1: joinNonEmpty(" ", seller.Address.LineOne, seller.Address.LineTwo),
		Zipcode:  strings.TrimSpace(seller.Address.Postcode),
		City:     strings.TrimSpace(seller.Address.City),
		Country:  strings.ToUpper(strings.TrimSpace(seller.Address.Country)),
	}
	for _, id := range seller.TaxRegistrations {
		if e.Supplier.VATNumber == "" && (id.Scheme == "VA" || id.Scheme == "") {
			e.Supplier.VATNumber = NormalizeVAT(id.Value)
		}
	}
	if v := strings.TrimSpace(seller.LegalOrg.ID.Value); isKVK(v) && (seller.LegalOrg.ID.Scheme == "0106" || e.Supplier.Country == "NL") {
		e.Supplier.KVK = v
	}
	for _, iban := range t.Settlement.IBANs {
		if e.Supplier.IBAN == "" && ValidIBAN(iban) {
			e.Supplier.IBAN = NormalizeIBAN(iban)
		}
	}
	e.Reference = sureString(d.ID)
	e.Date = sureDate(d.Issue.Value)
	e.Currency = sureString(strings.ToUpper(strings.TrimSpace(t.Settlement.Currency)))
	e.PaymentReference = sureString(t.Settlement.PaymentReference)
	for _, term := range t.Settlement.Terms {
		if !e.DueDate.Found {
			e.DueDate = sureDate(term.Due.Value)
		}
	}
	if !e.DueDate.Found && e.Date.Found {
		for _, term := range t.Settlement.Terms {
			if days, ok := termDays(term.Description); ok {
				e.DueDate = sure(e.Date.V.AddDate(0, 0, days))
				break
			}
		}
	}
	s := t.Settlement.Summation
	if a, ok := decimalAmount(firstNonEmpty(s.TaxBasis, s.LineTotal)); ok {
		e.TotalExclTax = sure(sign * a)
	}
	if a, ok := decimalAmount(firstNonEmpty(s.GrandTotal, s.DuePayable)); ok {
		e.TotalInclTax = sure(sign * a)
	}
	for _, tax := range t.Settlement.Taxes {
		taxable, _ := decimalAmount(tax.Basis)
		amount, _ := decimalAmount(tax.Calculated)
		e.VAT = append(e.VAT, VAT{Rate: rateString(tax.Rate), Taxable: sign * taxable, Tax: sign * amount})
	}
	for _, l := range t.Lines {
		e.Lines = append(e.Lines, eLine(firstNonEmpty(l.Name, l.Description), l.Quantity, l.Price, l.BasisQty, l.Total, l.Rate, sign))
	}
	return e
}

// --- shared ---

func eInvoiceBase() Extraction {
	return Extraction{
		Source:        SourceEInvoice,
		Kind:          sure(KindPurchaseInvoice),
		SupplierFound: Certain,
		PricesInclTax: sure(false),
	}
}

// eLine builds a line; the price is per base quantity in e-invoices, so it is
// divided by it.
func eLine(desc, qty, price, baseQty, total, rate string, sign money.Amount) Line {
	l := Line{Description: strings.TrimSpace(desc)}
	if q := normalizeQuantity(qty); q != "" {
		l.Quantity = sure(q)
	}
	if p, ok := decimalAmountPer(price, baseQty); ok {
		l.Price = sure(sign * p)
	}
	if a, ok := decimalAmount(total); ok {
		l.Total = sure(sign * a)
	}
	if r := strings.TrimSpace(rate); r != "" {
		l.TaxRate = sure(rateString(r))
	}
	return l
}

func sureString(s string) Value[string] {
	if s = strings.TrimSpace(s); s == "" {
		return Value[string]{}
	}
	return sure(s)
}

// sureDate reads 2026-11-13 (UBL) or 20261113 (CII format 102).
func sureDate(s string) Value[time.Time] {
	s = strings.TrimSpace(s)
	for _, layout := range []string{time.DateOnly, "20060102"} {
		if d, err := time.Parse(layout, s); err == nil {
			return sure(d)
		}
	}
	return Value[time.Time]{}
}

func sure[T any](v T) Value[T] { return Value[T]{V: v, Found: true, Confidence: Certain} }

func unsure[T any](v T) Value[T] { return Value[T]{V: v, Found: true, Confidence: Uncertain} }

// decimalAmount reads an XML decimal ("100", "50.0000", "-3.5") to cents,
// rounding half away from zero.
func decimalAmount(s string) (money.Amount, bool) { return decimalAmountPer(s, "") }

func decimalAmountPer(s, per string) (money.Amount, bool) {
	r, ok := new(big.Rat).SetString(strings.TrimSpace(s))
	if !ok {
		return 0, false
	}
	if q, ok := new(big.Rat).SetString(strings.TrimSpace(per)); ok && q.Sign() > 0 {
		r.Quo(r, q)
	}
	r.Mul(r, big.NewRat(100, 1))
	// round half away from zero: (2n + sign*d) / 2d, truncated
	n := new(big.Int).Mul(r.Num(), big.NewInt(2))
	d := new(big.Int).Mul(r.Denom(), big.NewInt(2))
	if r.Sign() < 0 {
		n.Sub(n, r.Denom())
	} else {
		n.Add(n, r.Denom())
	}
	q := new(big.Int).Quo(n, d)
	if !q.IsInt64() {
		return 0, false
	}
	return money.Amount(q.Int64()), true
}

// normalizeQuantity turns "2.000" into "2" and "2,50" into "2.5".
func normalizeQuantity(s string) string {
	s = strings.ReplaceAll(strings.TrimSpace(s), ",", ".")
	if _, ok := new(big.Rat).SetString(s); !ok || s == "" {
		return ""
	}
	if strings.Contains(s, ".") {
		s = strings.TrimRight(strings.TrimRight(s, "0"), ".")
	}
	if s == "" || s == "-" {
		return "0"
	}
	return s
}

// rateString turns "21.00" into "21" and "5,5" into "5.5".
func rateString(s string) string {
	return normalizeQuantity(strings.TrimSuffix(strings.TrimSpace(s), "%"))
}

func isKVK(s string) bool {
	if len(s) != 8 {
		return false
	}
	for i := range len(s) {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}

func firstNonEmpty(ss ...string) string {
	for _, s := range ss {
		if strings.TrimSpace(s) != "" {
			return s
		}
	}
	return ""
}

func joinNonEmpty(sep string, ss ...string) string {
	var out []string
	for _, s := range ss {
		if s = strings.TrimSpace(s); s != "" {
			out = append(out, s)
		}
	}
	return strings.Join(out, sep)
}
