package sales

import (
	"strings"
	"time"

	"github.com/adirkuhn/moneyharpy/internal/money"
)

// Placeholders are the fields an invoice email template may use.
var Placeholders = []string{"{invoice_id}", "{invoice_date}", "{due_date}", "{total}", "{contact_name}", "{company_name}", "{payment_reference}", "{reference}"}

// DefaultEmail returns the invoice email MoneyHarpy sends when the
// workflow has none of its own.
func DefaultEmail(language string) (subject, body string) {
	if strings.HasPrefix(language, "nl") {
		return "Factuur {invoice_id} van {company_name}",
			"Beste {contact_name},\n\nIn de bijlage vind je factuur {invoice_id} van {invoice_date} voor {total}. " +
				"We ontvangen de betaling graag uiterlijk {due_date} onder vermelding van {payment_reference}.\n\nMet vriendelijke groet,\n{company_name}"
	}
	return "Invoice {invoice_id} from {company_name}",
		"Dear {contact_name},\n\nPlease find attached invoice {invoice_id} of {invoice_date} for {total}. " +
			"We would like to receive your payment by {due_date}, quoting {payment_reference}.\n\nKind regards,\n{company_name}"
}

// EmailText is the subject and body of the invoice email: the workflow's
// template, or the default for the invoice's language.
func EmailText(doc Document) (string, string) {
	subject, body := DefaultEmail(doc.Invoice.Language)
	if t := strings.TrimSpace(doc.Workflow.InvoiceEmailSubject); t != "" {
		subject = t
	}
	if t := strings.TrimSpace(doc.Workflow.InvoiceEmailBody); t != "" {
		body = t
	}
	return Expand(subject, doc), Expand(body, doc)
}

// Expand fills in an email template's placeholders, and Moneybird's tags
// such as {document.total_price} as the payment conditions do.
func Expand(template string, doc Document) string {
	inv := doc.Invoice
	date := func(t *time.Time) string {
		if t == nil {
			return ""
		}
		return t.Format("02-01-2006")
	}
	name := doc.Contact.Name()
	if doc.ContactPerson != nil {
		name = strings.TrimSpace(doc.ContactPerson.Firstname + " " + doc.ContactPerson.Lastname)
	}
	return strings.NewReplacer(
		"{invoice_id}", inv.InvoiceID,
		"{invoice_date}", date(inv.InvoiceDate),
		"{due_date}", date(inv.DueDate),
		"{total}", FormatMoney(doc.Totals.InclTax, inv.Currency, inv.Language),
		"{contact_name}", name,
		"{company_name}", doc.Identity.CompanyName,
		"{payment_reference}", ShortPaymentReference(inv.PaymentReference),
		"{reference}", inv.Reference,
	).Replace(RenderTags(template, TagValues(doc, time.Now().UTC())))
}

// FormatMoney writes an amount for people, such as "€ 1.234,50" in Dutch
// and "€1,234.50" otherwise.
func FormatMoney(a money.Amount, currency, language string) string {
	symbol := map[string]string{"EUR": "€", "USD": "$", "GBP": "£"}[currency]
	if symbol == "" {
		symbol = currency + " "
	}
	neg := a < 0
	if neg {
		a = -a
	}
	whole, cents := int64(a)/100, int64(a)%100
	digits := []byte{}
	s := []byte(itoa(whole))
	for i := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			digits = append(digits, ',')
		}
		digits = append(digits, s[i])
	}
	text := string(digits) + "." + pad2(cents)
	if strings.HasPrefix(language, "nl") {
		text = strings.NewReplacer(",", ".", ".", ",").Replace(text)
		symbol = strings.TrimSpace(symbol) + " "
	}
	if neg {
		return "-" + symbol + text
	}
	return symbol + text
}

func itoa(n int64) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for ; n > 0; n /= 10 {
		b = append([]byte{byte('0' + n%10)}, b...)
	}
	return string(b)
}

func pad2(n int64) string {
	if n < 10 {
		return "0" + itoa(n)
	}
	return itoa(n)
}
