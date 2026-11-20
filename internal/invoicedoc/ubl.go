package invoicedoc

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"math"
	"slices"
	"strings"
	"time"

	"github.com/adirkuhn/moneyharpy/internal/bookkeeping"
	"github.com/adirkuhn/moneyharpy/internal/money"
	"github.com/adirkuhn/moneyharpy/internal/sales"
)

// UBL renders the invoice as a UBL 2.1 invoice after Peppol BIS Billing 3.0,
// which Dutch and Belgian bookkeeping software imports: a credit invoice as
// a credit note. Sending it over the Peppol network is not done here.
func (Renderer) UBL(doc sales.Document) ([]byte, error) {
	inv, tot := doc.Invoice, doc.Totals
	cur := inv.Currency
	// A credit invoice is a credit note; one of less than nothing without an
	// invoice to credit is an invoice that takes off.
	credit := inv.OriginalID != nil
	// A credit note states what it credits as positive amounts.
	sign := money.Amount(1)
	if credit {
		sign = -1
	}
	amt := func(a money.Amount) amount { return amount{Currency: cur, Value: decimal(sign * a)} }
	day := func(t *time.Time) string {
		if t == nil {
			return ""
		}
		return t.Format(time.DateOnly)
	}
	id, c := doc.Identity, doc.Contact
	x := ublInvoice{
		XMLName: xml.Name{Local: "Invoice"}, XMLNS: "urn:oasis:names:specification:ubl:schema:xsd:Invoice-2", CAC: cacNS, CBC: cbcNS,
		CustomizationID: "urn:cen.eu:en16931:2017#compliant#urn:fdc:peppol.eu:2017:poacc:billing:3.0",
		ProfileID:       "urn:fdc:peppol.eu:2017:poacc:billing:01:1.0",
		ID:              inv.InvoiceID, IssueDate: day(inv.InvoiceDate), DueDate: day(inv.DueDate), TypeCode: "380", Currency: cur,
		BuyerReference: cmp(inv.Reference, c.CustomerID),
		Supplier: partyOf(id.CompanyName, id.Address1, id.City, id.Zipcode, id.Country, id.TaxNumber,
			electronicAddress("", "", id.ChamberOfCommerce, id.Country, id.TaxNumber), id.Email),
		Customer: partyOf(c.Name(), c.Address1, c.City, c.Zipcode, c.Country, c.TaxNumber,
			electronicAddress(c.SIIdentifierType, c.SIIdentifier, c.ChamberOfCommerce, c.Country, c.TaxNumber), c.SendInvoicesToEmail),
	}
	if credit {
		x.XMLName.Local, x.XMLNS = "CreditNote", "urn:oasis:names:specification:ubl:schema:xsd:CreditNote-2"
		x.TypeCode, x.CreditTypeCode, x.DueDate = "", "381", ""
		if doc.Credits != nil {
			x.Billing = &billingReference{Invoice: documentReference{ID: doc.Credits.InvoiceID, IssueDate: day(doc.Credits.InvoiceDate)}}
		}
	}
	if id.BankAccountNumber != "" {
		x.PaymentMeans = &paymentMeans{Code: "30", PaymentID: sales.ShortPaymentReference(inv.PaymentReference), Account: &account{ID: id.BankAccountNumber, Name: cmp(id.BankAccountName, id.CompanyName)}}
		if credit {
			x.PaymentMeans.DueDate = day(inv.DueDate)
		}
	}
	x.TaxTotal.TaxAmount = amt(tot.Tax)
	for _, r := range tot.ByRate {
		cat := vatCategory(doc.Rates[r.TaxRateID], inv.Language)
		if i := slices.IndexFunc(x.TaxTotal.Subtotals, func(s taxSubtotal) bool { return s.Category.same(cat) }); i >= 0 {
			// One breakdown per category and percentage.
			s := &x.TaxTotal.Subtotals[i]
			s.taxable, s.tax = s.taxable+r.Taxable, s.tax+r.Tax
			s.Taxable, s.Tax = amt(s.taxable), amt(s.tax)
			continue
		}
		x.TaxTotal.Subtotals = append(x.TaxTotal.Subtotals, taxSubtotal{Taxable: amt(r.Taxable), Tax: amt(r.Tax), Category: cat, taxable: r.Taxable, tax: r.Tax})
	}
	if slices.ContainsFunc(x.TaxTotal.Subtotals, func(s taxSubtotal) bool { return s.Category.ID == "K" }) {
		// An intra-community supply says when it was delivered, and where.
		x.Delivery = &delivery{Date: day(inv.InvoiceDate), Location: &deliveryLocation{Address: postalAddress{
			Street: c.Address1, City: c.City, Zip: c.Zipcode, Country: countryCode{Code: c.Country},
		}}}
	}
	x.Totals = monetaryTotal{LineExtension: amt(tot.ExclTax), TaxExclusive: amt(tot.ExclTax), TaxInclusive: amt(tot.InclTax), Payable: amt(tot.InclTax - inv.Paid())}
	if paid := inv.Paid(); paid != 0 {
		p := amt(paid)
		x.Totals.Prepaid = &p
	}
	n := 0
	for i, d := range inv.Lines {
		if d.Optional && !d.Selected {
			continue
		}
		n++
		base := sign * tot.LineBases[i]
		qty := float64(d.Quantity) / 1000 * float64(sign)
		price := base
		if d.Quantity != 0 {
			price = money.Amount(math.Round(float64(base) / qty))
		}
		if price < 0 {
			// A price is never negative: what is taken off is a negative quantity.
			price, qty = -price, -qty
		}
		cat := vatCategory(doc.Rates[d.TaxRateID], "")
		cat.ReasonCode, cat.Reason = "", ""
		line := invoiceLine{
			ID: fmt.Sprint(n), LineExtension: amount{Currency: cur, Value: decimal(base)},
			Item:  item{Name: firstLine(d.Description), Category: cat},
			Price: priceAmount{Amount: amount{Currency: cur, Value: decimal(price)}},
		}
		q := &quantity{Unit: "C62", Value: fmt.Sprintf("%.3f", qty)}
		if credit {
			line.Credited = q
			x.CreditLines = append(x.CreditLines, line)
		} else {
			line.Invoiced = q
			x.Lines = append(x.Lines, line)
		}
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

// vatCategory is the Peppol VAT category of a rate, with why no VAT is
// charged for those without, in the document's language.
func vatCategory(r bookkeeping.TaxRate, language string) taxCategory {
	pct := r.Percentage
	if pct == "" {
		pct = "0"
	}
	c := taxCategory{ID: "S", Percent: pct, Scheme: taxScheme{ID: "VAT"}}
	if strings.Trim(strings.ReplaceAll(pct, ".", ""), "0") != "" {
		return c
	}
	reason := func(code, en, nl string) {
		c.ReasonCode, c.Reason = code, en
		if language == "nl" {
			c.Reason = nl
		}
	}
	switch {
	case slices.Contains(r.ReportBoxes, "NL/3b"):
		c.ID = "K"
		reason("VATEX-EU-IC", "Intra-community supply, VAT reverse-charged", "Intracommunautaire levering, btw verlegd")
	case slices.Contains(r.ReportBoxes, "NL/3a"):
		c.ID = "G"
		reason("VATEX-EU-G", "Export outside the EU", "Uitvoer buiten de EU")
	case !r.ShowTax:
		c.ID = "AE"
		reason("VATEX-EU-AE", "VAT reverse-charged", "Btw verlegd")
	default:
		c.ID = "Z"
	}
	return c
}

// vatEndpoints are the Peppol schemes of VAT numbers, by country.
var vatEndpoints = map[string]string{
	"AT": "9914", "BE": "9925", "BG": "9926", "CY": "9928", "CZ": "9929", "DE": "9930", "EE": "9931", "ES": "9920", "FR": "9957",
	"GR": "9933", "HR": "9934", "HU": "9910", "IE": "9935", "IT": "0211", "LT": "9937", "LU": "9938", "LV": "9939", "MT": "9943",
	"NL": "9944", "PL": "9945", "PT": "9946", "RO": "9947", "SE": "9955", "SI": "9949", "SK": "9950",
}

// electronicAddress is a party's Peppol address: its own when it has one,
// else its Dutch chamber of commerce number, Belgian enterprise number or
// VAT number.
func electronicAddress(scheme, value, kvk, country, vat string) *endpointID {
	vat = strings.ToUpper(strings.ReplaceAll(vat, " ", ""))
	dutch := country == "NL" || country == ""
	switch {
	case value != "" && scheme != "":
		return &endpointID{Scheme: scheme, Value: value}
	case kvk != "" && dutch:
		return &endpointID{Scheme: "0106", Value: kvk}
	case country == "BE" && strings.HasPrefix(vat, "BE"):
		return &endpointID{Scheme: "0208", Value: strings.TrimPrefix(vat, "BE")}
	case vat != "" && vatEndpoints[country] != "":
		return &endpointID{Scheme: vatEndpoints[country], Value: vat}
	}
	return nil
}

func partyOf(name, street, city, zip, country, vat string, endpoint *endpointID, email string) partyWrap {
	p := party{
		Name:    partyName{Name: name},
		Address: postalAddress{Street: street, City: city, Zip: zip, Country: countryCode{Code: country}},
		Legal:   legalEntity{Name: name},
	}
	p.Endpoint = endpoint
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
	XMLName         xml.Name
	XMLNS           string            `xml:"xmlns,attr"`
	CAC             string            `xml:"xmlns:cac,attr"`
	CBC             string            `xml:"xmlns:cbc,attr"`
	CustomizationID string            `xml:"cbc:CustomizationID"`
	ProfileID       string            `xml:"cbc:ProfileID"`
	ID              string            `xml:"cbc:ID"`
	IssueDate       string            `xml:"cbc:IssueDate"`
	DueDate         string            `xml:"cbc:DueDate,omitempty"`
	TypeCode        string            `xml:"cbc:InvoiceTypeCode,omitempty"`
	CreditTypeCode  string            `xml:"cbc:CreditNoteTypeCode,omitempty"`
	Currency        string            `xml:"cbc:DocumentCurrencyCode"`
	BuyerReference  string            `xml:"cbc:BuyerReference,omitempty"`
	Billing         *billingReference `xml:"cac:BillingReference,omitempty"`
	Supplier        partyWrap         `xml:"cac:AccountingSupplierParty"`
	Customer        partyWrap         `xml:"cac:AccountingCustomerParty"`
	Delivery        *delivery         `xml:"cac:Delivery,omitempty"`
	PaymentMeans    *paymentMeans     `xml:"cac:PaymentMeans,omitempty"`
	TaxTotal        taxTotal          `xml:"cac:TaxTotal"`
	Totals          monetaryTotal     `xml:"cac:LegalMonetaryTotal"`
	Lines           []invoiceLine     `xml:"cac:InvoiceLine"`
	CreditLines     []invoiceLine     `xml:"cac:CreditNoteLine"`
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
	Code string `xml:"cbc:PaymentMeansCode"`
	// DueDate is a credit note's, which has none of its own.
	DueDate   string   `xml:"cbc:PaymentDueDate,omitempty"`
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
	Taxable      amount      `xml:"cbc:TaxableAmount"`
	Tax          amount      `xml:"cbc:TaxAmount"`
	Category     taxCategory `xml:"cac:TaxCategory"`
	taxable, tax money.Amount
}

type taxCategory struct {
	ID         string    `xml:"cbc:ID"`
	Percent    string    `xml:"cbc:Percent"`
	ReasonCode string    `xml:"cbc:TaxExemptionReasonCode,omitempty"`
	Reason     string    `xml:"cbc:TaxExemptionReason,omitempty"`
	Scheme     taxScheme `xml:"cac:TaxScheme"`
}

// same reports whether two categories are one in the VAT breakdown.
func (c taxCategory) same(o taxCategory) bool {
	a, _ := sales.ParsePercentage(c.Percent)
	b, _ := sales.ParsePercentage(o.Percent)
	return c.ID == o.ID && a == b
}

type monetaryTotal struct {
	LineExtension amount  `xml:"cbc:LineExtensionAmount"`
	TaxExclusive  amount  `xml:"cbc:TaxExclusiveAmount"`
	TaxInclusive  amount  `xml:"cbc:TaxInclusiveAmount"`
	Prepaid       *amount `xml:"cbc:PrepaidAmount,omitempty"`
	Payable       amount  `xml:"cbc:PayableAmount"`
}

type invoiceLine struct {
	ID            string      `xml:"cbc:ID"`
	Invoiced      *quantity   `xml:"cbc:InvoicedQuantity,omitempty"`
	Credited      *quantity   `xml:"cbc:CreditedQuantity,omitempty"`
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

type billingReference struct {
	Invoice documentReference `xml:"cac:InvoiceDocumentReference"`
}

type documentReference struct {
	ID        string `xml:"cbc:ID"`
	IssueDate string `xml:"cbc:IssueDate,omitempty"`
}

type delivery struct {
	Date     string            `xml:"cbc:ActualDeliveryDate,omitempty"`
	Location *deliveryLocation `xml:"cac:DeliveryLocation,omitempty"`
}

type deliveryLocation struct {
	Address postalAddress `xml:"cac:Address"`
}
