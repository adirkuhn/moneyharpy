package sales

import (
	"regexp"
	"strings"
	"time"
	"unicode"

	"github.com/adirkuhn/moneyharpy/internal/contacts"
	"github.com/adirkuhn/moneyharpy/internal/money"
)

// Moneybird's texts on invoices, such as the payment conditions, hold tags
// such as {document.total_price} that are filled in where the invoice is
// shown, and textile: _italic_, *bold* and +underline+. The web app fills
// them in by the same rules for its preview.

var tagPattern = regexp.MustCompile(`\{([a-z_]+\.[a-z0-9_]+)\}`)

// RenderTags fills in the tags values knows; others stay as they are
// written.
func RenderTags(text string, values map[string]string) string {
	return tagPattern.ReplaceAllStringFunc(text, func(tag string) string {
		if v, ok := values[tag[1:len(tag)-1]]; ok {
			return v
		}
		return tag
	})
}

// TagValues are the values of the tags for the document, by tag name such
// as "document.total_price". A draft shows today as its invoice date and
// Moneybird's placeholder as its payment reference.
func TagValues(doc Document, today time.Time) map[string]string {
	inv, c, id := doc.Invoice, doc.Contact, doc.Identity
	day := today
	if inv.InvoiceDate != nil {
		day = *inv.InvoiceDate
	}
	due := day.AddDate(0, 0, inv.FirstDueInterval)
	if inv.DueDate != nil {
		due = *inv.DueDate
	}
	reference := ShortPaymentReference(inv.PaymentReference)
	if reference == "" && !inv.Sent() {
		reference = DraftPaymentReference(inv.DraftID)
	}
	unpaid := doc.Totals.InclTax - inv.Paid()
	// Without currency: the number as FormatMoney writes it.
	raw := func(a money.Amount) string {
		sign := ""
		if a < 0 {
			sign, a = "-", -a
		}
		return sign + strings.TrimSpace(FormatMoney(a, "", inv.Language))
	}
	v := map[string]string{
		"document.public_view_code":        "",
		"document.total_price":             FormatMoney(doc.Totals.InclTax, inv.Currency, inv.Language),
		"document.due_date":                due.Format("02-01-2006"),
		"document.invoice_date":            day.Format("02-01-2006"),
		"document.invoice_id":              inv.InvoiceID,
		"document.show_invoice_online_url": "",
		"document.payment_reference":       reference,
		"document.reference":               inv.Reference,
		"document.total_price_raw":         raw(doc.Totals.InclTax),
		"document.total_unpaid":            FormatMoney(unpaid, inv.Currency, inv.Language),
		"document.total_unpaid_raw":        raw(unpaid),

		"recipient.generic_account_name":          "",
		"recipient.generic_account_number":        c.BankAccount,
		"recipient.address1":                      c.Address1,
		"recipient.address2":                      c.Address2,
		"recipient.sepa_iban_account_name":        c.SepaIBANAccountName,
		"recipient.attention":                     c.SendInvoicesToAttention,
		"recipient.chamber_of_commerce":           c.ChamberOfCommerce,
		"recipient.city":                          c.City,
		"recipient.customer_id":                   c.CustomerID,
		"recipient.company_name":                  c.CompanyName,
		"recipient.country":                       c.Country,
		"recipient.credit_card_number":            "",
		"recipient.email":                         c.SendInvoicesToEmail,
		"recipient.contact_name":                  joinNames(c.Firstname, c.Lastname),
		"recipient.firstname":                     c.Firstname,
		"recipient.lastname":                      c.Lastname,
		"recipient.show_open_invoices_online_url": "",
		"recipient.name":                          c.Name(),
		"recipient.phone":                         c.Phone,
		"recipient.sepa_bic":                      c.SepaBIC,
		"recipient.sepa_iban":                     c.SepaIBAN,
		"recipient.sepa_mandate_id":               c.SepaMandateID,
		"recipient.tax_number":                    c.TaxNumber,
		"recipient.zipcode":                       c.Zipcode,

		"company.bank_account_name":   id.BankAccountName,
		"company.bank_account_number": id.BankAccountNumber,
		"company.address1":            id.Address1,
		"company.address2":            id.Address2,
		"company.bank_account_bic":    id.BankAccountBIC,
		"company.chamber_of_commerce": id.ChamberOfCommerce,
		"company.city":                id.City,
		"company.company_name":        id.CompanyName,
		"company.country":             id.Country,
		"company.email":               id.Email,
		"company.phone":               id.Phone,
		"company.tax_number":          id.TaxNumber,
		"company.zipcode":             id.Zipcode,
	}
	for _, f := range id.CustomFields {
		v["company.custom_field_"+f.ID.String()] = f.Value
	}
	p := doc.ContactPerson
	if p == nil {
		p = &contacts.Person{}
	}
	v["contact_person.department"] = p.Department
	v["contact_person.email"] = p.Email
	v["contact_person.contact_name"] = joinNames(p.Firstname, p.Lastname)
	v["contact_person.firstname"] = p.Firstname
	v["contact_person.lastname"] = p.Lastname
	v["contact_person.phone"] = p.Phone
	return v
}

func joinNames(first, last string) string { return strings.TrimSpace(first + " " + last) }

// PaymentConditionsSpans are the invoice's payment conditions as shown:
// styled by their textile, with their tags filled in. The textile is read
// first, so values with markers in them are not styled.
func PaymentConditionsSpans(doc Document, today time.Time) []Span {
	values := TagValues(doc, today)
	spans := ParseTextile(doc.Invoice.PaymentConditions)
	for i := range spans {
		spans[i].Text = RenderTags(spans[i].Text, values)
	}
	return spans
}

// PaymentConditionsText is the invoice's payment conditions with their
// tags filled in, textile left as written.
func PaymentConditionsText(doc Document, today time.Time) string {
	return RenderTags(doc.Invoice.PaymentConditions, TagValues(doc, today))
}

// Span is a piece of textile text with its style.
type Span struct {
	Text                    string
	Bold, Italic, Underline bool
}

type textileStyle struct{ bold, italic, underline bool }

func (s textileStyle) has(m rune) bool {
	switch m {
	case '*':
		return s.bold
	case '_':
		return s.italic
	}
	return s.underline
}

func (s textileStyle) with(m rune) textileStyle {
	switch m {
	case '*':
		s.bold = true
	case '_':
		s.italic = true
	default:
		s.underline = true
	}
	return s
}

// ParseTextile splits a textile text into styled spans. A marker opens at
// the start or after a character that is not a letter or digit, before one
// that is not a space; it closes at the first same marker on that line
// after one that is not a space and before the end or a character that is
// not a letter or digit. A style does not nest in itself; markers that close
// nothing stay as written.
func ParseTextile(text string) []Span {
	return mergeSpans(parseTextile([]rune(text), textileStyle{}))
}

func parseTextile(text []rune, style textileStyle) []Span {
	var spans []Span
	var plain []rune
	flush := func() {
		if len(plain) > 0 {
			spans = append(spans, Span{Text: string(plain), Bold: style.bold, Italic: style.italic, Underline: style.underline})
			plain = nil
		}
	}
	for i := 0; i < len(text); i++ {
		c := text[i]
		if isMarker(c) && !style.has(c) && i+1 < len(text) && !unicode.IsSpace(text[i+1]) && (i == 0 || !isWordChar(text[i-1])) {
			if end := closingMarker(text, i+1, c); end > i+1 {
				flush()
				spans = append(spans, parseTextile(text[i+1:end], style.with(c))...)
				i = end
				continue
			}
		}
		plain = append(plain, c)
	}
	flush()
	return spans
}

func closingMarker(text []rune, from int, marker rune) int {
	for j := from; j < len(text); j++ {
		if text[j] == '\n' {
			return -1
		}
		if text[j] == marker && !unicode.IsSpace(text[j-1]) && (j+1 == len(text) || !isWordChar(text[j+1])) {
			return j
		}
	}
	return -1
}

func isMarker(r rune) bool { return r == '*' || r == '_' || r == '+' }

// isWordChar matches the web's [0-9A-Za-z].
func isWordChar(r rune) bool {
	return (r >= '0' && r <= '9') || (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z')
}

func mergeSpans(spans []Span) []Span {
	var out []Span
	for _, s := range spans {
		if n := len(out); n > 0 && out[n-1].Bold == s.Bold && out[n-1].Italic == s.Italic && out[n-1].Underline == s.Underline {
			out[n-1].Text += s.Text
			continue
		}
		out = append(out, s)
	}
	return out
}
