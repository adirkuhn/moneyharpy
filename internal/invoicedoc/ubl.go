package invoicedoc

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"strings"
	"time"

	"github.com/adirkuhn/moneyharpy/internal/money"
	"github.com/adirkuhn/moneyharpy/internal/sales"
)

// UBL renders the invoice as a UBL 2.1 invoice after Peppol BIS Billing 3.0,
// which Dutch and Belgian bookkeeping software imports. Sending it over the
// Peppol network is not done here.
func (Renderer) UBL(doc sales.Document) ([]byte, error) {
	inv, tot := doc.Invoice, doc.Totals
	cur := inv.Currency
	amt := func(a money.Amount) amount { return amount{Currency: cur, Value: decimal(a)} }
	day := func(t *time.Time) string {
		if t == nil {
			return ""
		}
		return t.Format(time.DateOnly)
	}
	x := ublInvoice{
		XMLNS: "urn:oasis:names:specification:ubl:schema:xsd:Invoice-2", CAC: cacNS, CBC: cbcNS,
		CustomizationID: "urn:cen.eu:en16931:2017#compliant#urn:fdc:peppol.eu:2017:poacc:billing:3.0",
		ProfileID:       "urn:fdc:peppol.eu:2017:poacc:billing:01:1.0",
		ID:              inv.InvoiceID, IssueDate: day(inv.InvoiceDate), DueDate: day(inv.DueDate), TypeCode: "380", Currency: cur,
		BuyerReference: cmp(inv.Reference, doc.Contact.CustomerID),
		Supplier: partyOf(doc.Identity.CompanyName, doc.Identity.Address1, doc.Identity.City, doc.Identity.Zipcode, doc.Identity.Country,
			doc.Identity.TaxNumber, doc.Identity.ChamberOfCommerce, "0106", doc.Identity.Email),
		Customer: partyOf(doc.Contact.Name(), doc.Contact.Address1, doc.Contact.City, doc.Contact.Zipcode, doc.Contact.Country,
			doc.Contact.TaxNumber, doc.Contact.SIIdentifier, doc.Contact.SIIdentifierType, doc.Contact.SendInvoicesToEmail),
	}
	if tot.InclTax < 0 || inv.OriginalID != nil {
		x.TypeCode = "381"
	}
	if doc.Identity.BankAccountNumber != "" {
		x.PaymentMeans = &paymentMeans{Code: "30", PaymentID: sales.ShortPaymentReference(inv.PaymentReference), Account: &account{ID: doc.Identity.BankAccountNumber, Name: cmp(doc.Identity.BankAccountName, doc.Identity.CompanyName)}}
	}
	x.TaxTotal.TaxAmount = amt(tot.Tax)
	for _, r := range tot.ByRate {
		pct := doc.Rates[r.TaxRateID].Percentage
		x.TaxTotal.Subtotals = append(x.TaxTotal.Subtotals, taxSubtotal{Taxable: amt(r.Taxable), Tax: amt(r.Tax), Category: category(pct)})
	}
	x.Totals = monetaryTotal{LineExtension: amt(tot.ExclTax), TaxExclusive: amt(tot.ExclTax), TaxInclusive: amt(tot.InclTax), Payable: amt(tot.InclTax - doc.Invoice.Paid())}
	n := 0
	for i, d := range inv.Lines {
		if d.Optional && !d.Selected {
			continue
		}
		n++
		qty := float64(d.Quantity) / 1000
		price := tot.LineBases[i]
		if d.Quantity != 0 {
			price = money.Amount(float64(tot.LineBases[i]) / qty)
		}
		x.Lines = append(x.Lines, invoiceLine{
			ID: fmt.Sprint(n), Quantity: quantity{Unit: "C62", Value: fmt.Sprintf("%.3f", qty)}, LineExtension: amt(tot.LineBases[i]),
			Item:  item{Name: firstLine(d.Description), Category: category(doc.Rates[d.TaxRateID].Percentage)},
			Price: priceAmount{Amount: amt(price)},
		})
	}
	var out bytes.Buffer
	out.WriteString(xml.Header)
	enc := xml.NewEncoder(&out)
	enc.Indent("", "  ")
	if err := enc.Encode(x); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

const (
	cacNS = "urn:oasis:names:specification:ubl:schema:xsd:CommonAggregateComponents-2"
	cbcNS = "urn:oasis:names:specification:ubl:schema:xsd:CommonBasicComponents-2"
)

func decimal(a money.Amount) string {
	sign := ""
	if a < 0 {
		sign, a = "-", -a
	}
	return fmt.Sprintf("%s%d.%02d", sign, a/100, a%100)
}

func cmp(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

func firstLine(s string) string {
	s, _, _ = strings.Cut(strings.TrimSpace(s), "\n")
	if s == "" {
		return "-"
	}
	return s
}

// category is the VAT category for a percentage: standard, or zero rated.
func category(pct string) taxCategory {
	if pct == "" {
		pct = "0"
	}
	id := "S"
	if strings.Trim(strings.ReplaceAll(pct, ".", ""), "0") == "" {
		id = "Z"
	}
	return taxCategory{ID: id, Percent: pct, Scheme: taxScheme{ID: "VAT"}}
}

func partyOf(name, street, city, zip, country, vat, endpoint, scheme, email string) partyWrap {
	p := party{
		Name:    partyName{Name: name},
		Address: postalAddress{Street: street, City: city, Zip: zip, Country: countryCode{Code: country}},
		Legal:   legalEntity{Name: name},
	}
	if endpoint != "" && scheme != "" {
		p.Endpoint = &endpointID{Scheme: scheme, Value: endpoint}
	}
	if vat != "" {
		p.Tax = &partyTax{CompanyID: vat, Scheme: taxScheme{ID: "VAT"}}
	}
	if email != "" {
		p.Contact = &contact{Email: strings.TrimSpace(strings.Split(email, ",")[0])}
	}
	return partyWrap{Party: p}
}

type amount struct {
	Currency string `xml:"currencyID,attr"`
	Value    string `xml:",chardata"`
}

type quantity struct {
	Unit  string `xml:"unitCode,attr"`
	Value string `xml:",chardata"`
}

type ublInvoice struct {
	XMLName         xml.Name      `xml:"Invoice"`
	XMLNS           string        `xml:"xmlns,attr"`
	CAC             string        `xml:"xmlns:cac,attr"`
	CBC             string        `xml:"xmlns:cbc,attr"`
	CustomizationID string        `xml:"cbc:CustomizationID"`
	ProfileID       string        `xml:"cbc:ProfileID"`
	ID              string        `xml:"cbc:ID"`
	IssueDate       string        `xml:"cbc:IssueDate"`
	DueDate         string        `xml:"cbc:DueDate,omitempty"`
	TypeCode        string        `xml:"cbc:InvoiceTypeCode"`
	Currency        string        `xml:"cbc:DocumentCurrencyCode"`
	BuyerReference  string        `xml:"cbc:BuyerReference,omitempty"`
	Supplier        partyWrap     `xml:"cac:AccountingSupplierParty"`
	Customer        partyWrap     `xml:"cac:AccountingCustomerParty"`
	PaymentMeans    *paymentMeans `xml:"cac:PaymentMeans,omitempty"`
	TaxTotal        taxTotal      `xml:"cac:TaxTotal"`
	Totals          monetaryTotal `xml:"cac:LegalMonetaryTotal"`
	Lines           []invoiceLine `xml:"cac:InvoiceLine"`
}

type partyWrap struct {
	Party party `xml:"cac:Party"`
}

type party struct {
	Endpoint *endpointID   `xml:"cbc:EndpointID,omitempty"`
	Name     partyName     `xml:"cac:PartyName"`
	Address  postalAddress `xml:"cac:PostalAddress"`
	Tax      *partyTax     `xml:"cac:PartyTaxScheme,omitempty"`
	Legal    legalEntity   `xml:"cac:PartyLegalEntity"`
	Contact  *contact      `xml:"cac:Contact,omitempty"`
}

type endpointID struct {
	Scheme string `xml:"schemeID,attr"`
	Value  string `xml:",chardata"`
}

type partyName struct {
	Name string `xml:"cbc:Name"`
}

type postalAddress struct {
	Street  string      `xml:"cbc:StreetName,omitempty"`
	City    string      `xml:"cbc:CityName,omitempty"`
	Zip     string      `xml:"cbc:PostalZone,omitempty"`
	Country countryCode `xml:"cac:Country"`
}

type countryCode struct {
	Code string `xml:"cbc:IdentificationCode"`
}

type partyTax struct {
	CompanyID string    `xml:"cbc:CompanyID"`
	Scheme    taxScheme `xml:"cac:TaxScheme"`
}

type taxScheme struct {
	ID string `xml:"cbc:ID"`
}

type legalEntity struct {
	Name string `xml:"cbc:RegistrationName"`
}

type contact struct {
	Email string `xml:"cbc:ElectronicMail"`
}

type paymentMeans struct {
	Code      string   `xml:"cbc:PaymentMeansCode"`
	PaymentID string   `xml:"cbc:PaymentID,omitempty"`
	Account   *account `xml:"cac:PayeeFinancialAccount,omitempty"`
}

type account struct {
	ID   string `xml:"cbc:ID"`
	Name string `xml:"cbc:Name,omitempty"`
}

type taxTotal struct {
	TaxAmount amount        `xml:"cbc:TaxAmount"`
	Subtotals []taxSubtotal `xml:"cac:TaxSubtotal"`
}

type taxSubtotal struct {
	Taxable  amount      `xml:"cbc:TaxableAmount"`
	Tax      amount      `xml:"cbc:TaxAmount"`
	Category taxCategory `xml:"cac:TaxCategory"`
}

type taxCategory struct {
	ID      string    `xml:"cbc:ID"`
	Percent string    `xml:"cbc:Percent"`
	Scheme  taxScheme `xml:"cac:TaxScheme"`
}

type monetaryTotal struct {
	LineExtension amount `xml:"cbc:LineExtensionAmount"`
	TaxExclusive  amount `xml:"cbc:TaxExclusiveAmount"`
	TaxInclusive  amount `xml:"cbc:TaxInclusiveAmount"`
	Payable       amount `xml:"cbc:PayableAmount"`
}

type invoiceLine struct {
	ID            string      `xml:"cbc:ID"`
	Quantity      quantity    `xml:"cbc:InvoicedQuantity"`
	LineExtension amount      `xml:"cbc:LineExtensionAmount"`
	Item          item        `xml:"cac:Item"`
	Price         priceAmount `xml:"cac:Price"`
}

type item struct {
	Name     string      `xml:"cbc:Name"`
	Category taxCategory `xml:"cac:ClassifiedTaxCategory"`
}

type priceAmount struct {
	Amount amount `xml:"cbc:PriceAmount"`
}
