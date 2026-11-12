package sales

import (
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/adirkuhn/moneyharpy/internal/administration"
	"github.com/adirkuhn/moneyharpy/internal/contacts"
	"github.com/adirkuhn/moneyharpy/internal/resource"
)

func TestParseTextile(t *testing.T) {
	for _, tc := range []struct {
		in   string
		want []Span
	}{
		{"plain", []Span{{Text: "plain"}}},
		{"pay *now* please", []Span{{Text: "pay "}, {Text: "now", Bold: true}, {Text: " please"}}},
		{"_soft_ and +line+", []Span{{Text: "soft", Italic: true}, {Text: " and "}, {Text: "line", Underline: true}}},
		{"*_both_*", []Span{{Text: "both", Bold: true, Italic: true}}},
		{"*bold _and italic_ text*", []Span{{Text: "bold ", Bold: true}, {Text: "and italic", Bold: true, Italic: true}, {Text: " text", Bold: true}}},
		{"{document.total_price} and snake_case_name", []Span{{Text: "{document.total_price} and snake_case_name"}}},
		{"2 * 3 * 4", []Span{{Text: "2 * 3 * 4"}}},
		{"*open only", []Span{{Text: "*open only"}}},
		{"*not\nacross* lines", []Span{{Text: "*not\nacross* lines"}}},
		{"a+b+c", []Span{{Text: "a+b+c"}}},
		{"**", []Span{{Text: "**"}}},
		{"€ *één*", []Span{{Text: "€ "}, {Text: "één", Bold: true}}},
		{"", nil},
	} {
		if got := ParseTextile(tc.in); !reflect.DeepEqual(got, tc.want) {
			t.Errorf("ParseTextile(%q) = %+v, want %+v", tc.in, got, tc.want)
		}
	}
}

func TestRenderTags(t *testing.T) {
	values := map[string]string{"document.total_price": "€ 10,00", "recipient.name": ""}
	got := RenderTags("Pay {document.total_price} {recipient.name}{document.unknown} {not a tag} {Document.total_price}", values)
	if want := "Pay € 10,00 {document.unknown} {not a tag} {Document.total_price}"; got != want {
		t.Errorf("RenderTags() = %q, want %q", got, want)
	}
}

func tagDocument() Document {
	day, due := time.Date(2026, 11, 11, 0, 0, 0, 0, time.UTC), time.Date(2026, 11, 25, 0, 0, 0, 0, time.UTC)
	return Document{
		Invoice: Invoice{
			InvoiceID: "2026-0001", DraftID: 3, Language: "nl", Currency: "EUR", InvoiceDate: &day, DueDate: &due, State: Open, FirstDueInterval: 14,
			PaymentReference: "RF130000000000000PV96X5KP", Reference: "Order 12",
			Payments: []Payment{{Price: 2000}},
		},
		Totals: Totals{InclTax: 123450},
		Contact: contacts.Contact{
			CompanyName: "Example Shop B.V.", Firstname: "Ann", Lastname: "de Vries", Address1: "Main Street 1", Zipcode: "1234 AB", City: "Amsterdam",
			Country: "NL", CustomerID: "7", SendInvoicesToEmail: "billing@example.com", SendInvoicesToAttention: "Accounts", BankAccount: "NL91ABNA0417164300",
			SepaIBAN: "NL91ABNA0417164300", SepaBIC: "ABNANL2A", SepaMandateID: "M1", SepaIBANAccountName: "Example Shop", TaxNumber: "NL000000000B01",
		},
		ContactPerson: &contacts.Person{Firstname: "Bo", Lastname: "Jansen", Email: "bo@example.com", Department: "Finance", Phone: "020 123"},
		Identity: administration.Identity{
			CompanyName: "Harpy B.V.", BankAccountName: "Harpy", BankAccountNumber: "NL91ABNA0417164300", BankAccountBIC: "ABNANL2A", Email: "hi@example.com",
			CustomFields: []administration.CustomFieldValue{{ID: resource.ID(77), Name: "Slogan", Value: "Fly high"}},
		},
	}
}

func TestTagValues(t *testing.T) {
	today := time.Date(2026, 12, 1, 0, 0, 0, 0, time.UTC)
	v := TagValues(tagDocument(), today)
	for tag, want := range map[string]string{
		"document.total_price": "€ 1.234,50", "document.total_price_raw": "1.234,50", "document.total_unpaid": "€ 1.214,50", "document.total_unpaid_raw": "1.214,50",
		"document.due_date": "25-11-2026", "document.invoice_date": "11-11-2026", "document.invoice_id": "2026-0001",
		"document.payment_reference": "RF13PV96X5KP", "document.reference": "Order 12", "document.public_view_code": "",
		"recipient.name": "Example Shop B.V.", "recipient.contact_name": "Ann de Vries", "recipient.attention": "Accounts", "recipient.email": "billing@example.com",
		"recipient.generic_account_number": "NL91ABNA0417164300", "recipient.sepa_bic": "ABNANL2A", "recipient.customer_id": "7",
		"contact_person.contact_name": "Bo Jansen", "contact_person.department": "Finance",
		"company.company_name": "Harpy B.V.", "company.bank_account_number": "NL91ABNA0417164300", "company.custom_field_77": "Fly high",
	} {
		if v[tag] != want {
			t.Errorf("%s = %q, want %q", tag, v[tag], want)
		}
	}
	for _, group := range []string{"document.", "recipient.", "contact_person.", "company."} {
		found := false
		for k := range v {
			found = found || strings.HasPrefix(k, group)
		}
		if !found {
			t.Errorf("no %s tags", group)
		}
	}

	// A draft shows today's dates and Moneybird's placeholder reference.
	d := tagDocument()
	d.Invoice.State, d.Invoice.InvoiceID, d.Invoice.InvoiceDate, d.Invoice.DueDate, d.Invoice.PaymentReference, d.ContactPerson = Draft, "", nil, nil, "", nil
	v = TagValues(d, today)
	if v["document.invoice_date"] != "01-12-2026" || v["document.due_date"] != "15-12-2026" || v["document.payment_reference"] != "RF94CONCEPT3" || v["contact_person.firstname"] != "" {
		t.Errorf("draft values = %v", v)
	}
}

func TestPaymentConditionsText(t *testing.T) {
	d := tagDocument()
	d.Invoice.PaymentConditions = "Pay *{document.total_price}* quoting {document.payment_reference}."
	if got := PaymentConditionsText(d, time.Now()); got != "Pay *€ 1.234,50* quoting RF13PV96X5KP." {
		t.Errorf("PaymentConditionsText() = %q", got)
	}
	spans := PaymentConditionsSpans(d, time.Now())
	if len(spans) != 3 || spans[1] != (Span{Text: "€ 1.234,50", Bold: true}) {
		t.Errorf("PaymentConditionsSpans() = %+v", spans)
	}
	// A value with markers in it is not styled.
	d.Invoice.Reference = "*x*"
	d.Invoice.PaymentConditions = "{document.reference}"
	if spans := PaymentConditionsSpans(d, time.Now()); len(spans) != 1 || spans[0].Bold || spans[0].Text != "*x*" {
		t.Errorf("spans of a value with markers = %+v", spans)
	}
}
