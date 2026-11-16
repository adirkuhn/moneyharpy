package invoicedoc

import (
	"fmt"
	"math"
	"strings"
	"testing"

	"github.com/go-pdf/fpdf"

	"github.com/adirkuhn/moneyharpy/internal/sales"
)

// example is an invoice with Moneybird's default layout, a logo on the
// left and synthetic sender and customer details.
func example(t *testing.T, language string) sales.Document {
	t.Helper()
	doc := styled(t)
	doc.Invoice.Language = language
	doc.Style.LogoPosition = "left"
	doc.Identity.CompanyName, doc.Identity.Address1, doc.Identity.Zipcode, doc.Identity.City = "Example Shop B.V.", "Example Street 1", "1234AB", "Amsterdam"
	doc.Identity.Email, doc.Identity.Phone = "info@example.com", "0612345678"
	doc.Identity.ChamberOfCommerce, doc.Identity.TaxNumber, doc.Identity.BankAccountNumber = "12345678", "NL123456782B01", "NL91ABNA0417164300"
	doc.Contact.CompanyName, doc.Contact.Address1, doc.Contact.Zipcode, doc.Contact.City, doc.Contact.Country = "Jan Voorbeeld", "Voorbeeldweg 2", "2804 NA", "Gouda", "NL"
	doc.Invoice.PaymentConditions = "We kindly request you to pay the above amount of {document.total_price} before {document.due_date}."
	doc.Style.ShowEPCCode = false
	return doc
}

// long gives the invoice 30 lines, some with a second description line, so
// it needs two pages.
func long(doc sales.Document) sales.Document {
	line := doc.Invoice.Lines[1]
	doc.Invoice.Lines = nil
	for i := range 30 {
		line.Description = fmt.Sprintf("Product %d", i+1)
		if i%3 == 0 {
			line.Description += "\n• Validade: 12-2027"
		}
		doc.Invoice.Lines = append(doc.Invoice.Lines, line)
	}
	doc.Totals = sales.ComputeInvoice(doc.Invoice, doc.Rates)
	return doc
}

const ptToMM = 25.4 / 72

// The web app's --text, --muted and --line colours as fpdf writes them.
const (
	textRGB  = "0.035 0.149 0.325"
	mutedRGB = "0.306 0.392 0.522"
	lineRGB  = "0.851 0.882 0.929"
)

// capTop is where a line of text's capitals start, in the default face.
func capTop(p placed) float64 {
	return p.Y - faceFor("").capHeight(fpdf.New("P", "mm", "A4", ""))*p.Size*ptToMM
}

func near(a, b, tolerance float64) bool { return math.Abs(a-b) <= tolerance }

func TestPDFHeaderIsMoneybirds(t *testing.T) {
	r := layoutOf(t, example(t, "nl"), true)
	if r.Pages != 1 {
		t.Fatalf("pages = %d, want 1", r.Pages)
	}
	company := r.one(t, "Example Shop B.V.")
	if !company.Bold || !near(company.X, 147, 3) || capTop(company) > 25 {
		t.Errorf("sender starts at %+v, want bold at about 70%% of the width near the top", company)
	}
	var lines []string
	for _, p := range r.Texts {
		if p.Page == 1 && near(p.X, company.X, 0.01) {
			lines = append(lines, p.Text)
		}
	}
	want := []string{"Example Shop B.V.", "Example Street 1", "1234AB Amsterdam", "info@example.com", "0612345678", "KVK: 12345678", "Btw: NL123456782B01", "Bank: NL91 ABNA 0417 1643 00"}
	if strings.Join(lines, "|") != strings.Join(want, "|") {
		t.Errorf("sender block = %q, want %q", lines, want)
	}
	if len(r.Images) != 1 {
		t.Fatalf("images = %d, want the logo", len(r.Images))
	}
	logo := r.Images[0]
	if !near(logo.X, left, 0.01) || !near(logo.Y, capTop(company), 0.2) || logo.H > 25.01 {
		t.Errorf("logo at %+v, want top left, its top on the sender's first line (%.2f), at most 25 mm high", logo, capTop(company))
	}
	name := r.one(t, "Jan Voorbeeld")
	if !near(name.X, left, 1.1) || name.Y < 45 || name.Y > 55 || name.Bold {
		t.Errorf("recipient at %+v, want the envelope window", name)
	}
	city := r.one(t, "2804 NA GOUDA")
	if city.Y <= name.Y {
		t.Errorf("city above the name")
	}
	if len(r.find("Nederland")) != 0 {
		t.Error("country printed for a Dutch customer of a Dutch sender")
	}
	title := r.one(t, "Factuur 2026-0001")
	if title.Bold || title.Size < 16 || !near(title.X, left, 1.1) || title.Y < 92 || title.Y > 110 {
		t.Errorf("title at %+v, want a large regular title about 95 mm down", title)
	}
	reference, value := r.one(t, "Kenmerk:"), r.one(t, "Order 7")
	if reference.Y <= title.Y || !near(reference.Y, value.Y, 0.01) || value.X <= reference.X {
		t.Errorf("reference %+v %+v, want a label and value under the title", reference, value)
	}
	date, due := r.one(t, "Factuurdatum:"), r.one(t, "Vervaldatum:")
	dateValue, dueValue := r.one(t, "11-11-2026"), r.one(t, "25-11-2026")
	if date.X < 120 || !near(date.X, due.X, 0.01) || due.Y <= date.Y || capTop(date) > title.Y || date.Y < capTop(title) {
		t.Errorf("dates at %+v %+v, want on the right beside the title %+v", date, due, title)
	}
	if !near(dateValue.Y, date.Y, 0.01) || !near(dueValue.Y, due.Y, 0.01) || dateValue.X <= date.X+20 {
		t.Errorf("date values at %+v %+v", dateValue, dueValue)
	}
}

func TestPDFLinesAndTotals(t *testing.T) {
	r := layoutOf(t, example(t, "nl"), true)
	heads := []string{"Aantal", "Omschrijving", "Bedrag", "Totaal", "Btw"}
	var xs []float64
	for _, h := range heads {
		p := r.find(h)
		if len(p) == 0 || !p[0].Bold {
			t.Fatalf("header %q = %+v, want bold", h, p)
		}
		xs = append(xs, p[0].X)
	}
	for i := 1; i < len(xs); i++ {
		if xs[i] <= xs[i-1] {
			t.Errorf("header columns %v at %v, want in that order", heads, xs)
		}
	}
	first := r.one(t, "Consultancy november")
	second := r.one(t, "On site")
	if !near(second.X, first.X, 0.01) || second.Y <= first.Y {
		t.Errorf("second description line at %+v, want under the first", second)
	}
	quantity := r.one(t, "2 uur")
	if quantity.X >= first.X || !near(quantity.Y, first.Y, 0.01) {
		t.Errorf("quantity at %+v, want left of the description on its first line", quantity)
	}
	if len(r.containing("uur x")) != 0 || len(r.containing("2 x")) != 0 {
		t.Error("quantity with an x")
	}
	subtotal, totals := r.one(t, "Subtotaal excl. btw"), r.find("Totaal")
	total := totals[len(totals)-1]
	nine := r.one(t, "9% btw")
	// Heavy rules in the text colour close the header, the last line and
	// the totals; thin ones in the light line colour separate the lines.
	var heavy []float64
	thin := 0
	for _, rule := range r.Rules {
		switch {
		case rule.H >= 0.3 && rule.H <= 0.4 && rule.Color == textRGB:
			heavy = append(heavy, rule.Y)
		case near(rule.H, 0.2, 0.01) && rule.Color == lineRGB:
			if rule.Y > first.Y && rule.Y < total.Y {
				thin++
			}
		default:
			t.Errorf("rule %+v, want a heavy one in the text colour or a thin one in the line colour", rule)
		}
	}
	header := r.one(t, "Omschrijving")
	if len(heavy) != 3 || heavy[0] < header.Y || heavy[0] > first.Y-3 || heavy[1] < second.Y || heavy[1] > subtotal.Y-3 || heavy[2] < nine.Y || heavy[2] > total.Y-3 {
		t.Errorf("heavy rules at %v, want under the header, the last line and the VAT", heavy)
	}
	if thin != 1 {
		t.Errorf("thin rules = %d, want one between the two lines", thin)
	}
	for _, p := range []placed{first, second, quantity, total} {
		if p.Color != textRGB {
			t.Errorf("%q in %q, want the text colour", p.Text, p.Color)
		}
	}
	for _, p := range []placed{r.one(t, "Factuurdatum:"), r.one(t, "Kenmerk:"), r.one(t, "Pagina 1 van 1")} {
		if p.Color != mutedRGB {
			t.Errorf("%q in %q, want the muted colour", p.Text, p.Color)
		}
	}
	if !(subtotal.Y < nine.Y && nine.Y < total.Y) || !total.Bold {
		t.Errorf("totals %+v %+v %+v, want subtotal, VAT, then a bold total", subtotal, nine, total)
	}
	if len(r.find("€ 296,50")) != 1 || len(r.find("€ 4,50")) != 1 {
		t.Errorf("amounts = %v", r.texts())
	}
}

func TestPDFFooterOnEveryPageAndNoticeOnTheLast(t *testing.T) {
	for _, tc := range []struct{ language, page, notice, header string }{
		{"nl", "Pagina %d van 2", "We kindly request", "Omschrijving"},
		{"en", "Page %d of 2", "We kindly request", "Description"},
	} {
		r := layoutOf(t, long(example(t, tc.language)), true)
		if r.Pages != 2 {
			t.Fatalf("%s: pages = %d, want 2", tc.language, r.Pages)
		}
		var numbers []placed
		for n := 1; n <= 2; n++ {
			p := r.one(t, fmt.Sprintf(tc.page, n))
			if p.Page != n || p.Y < 280 {
				t.Errorf("%s: page number %+v, want at the bottom of page %d", tc.language, p, n)
			}
			numbers = append(numbers, p)
		}
		if math.Abs(numbers[0].X-numbers[1].X) > 0.01 {
			t.Errorf("page numbers not at the same place")
		}
		notice := r.containing(tc.notice)
		if len(notice) != 1 || notice[0].Page != 2 || notice[0].Y >= numbers[1].Y || notice[0].Y < 240 {
			t.Fatalf("%s: notice = %+v, want once, in the footer of the last page", tc.language, notice)
		}
		ruled := false
		for _, rule := range r.Rules {
			if rule.Page == 2 && rule.Y < capTop(notice[0]) && rule.Y > capTop(notice[0])-6 && near(rule.X, left, 0.01) && near(rule.W, width, 0.01) {
				ruled = true
			}
		}
		if !ruled {
			t.Errorf("%s: no full-width rule above the notice", tc.language)
		}
		headers := r.find(tc.header)
		if len(headers) != 2 || headers[1].Page != 2 || headers[1].Y > 30 {
			t.Errorf("%s: table headers = %+v, want repeated at the top of page 2", tc.language, headers)
		}
		if company := r.find("Example Shop B.V."); len(company) != 1 || company[0].Page != 1 {
			t.Errorf("%s: sender on %+v, want on page 1 only", tc.language, company)
		}
		for _, p := range r.Texts {
			if p.Page == 2 && p.Text != notice[0].Text && p.Text != numbers[1].Text && !strings.HasPrefix(tc.notice, p.Text) && p.Y > capTop(notice[0])-6 && p.Y < numbers[1].Y-1 {
				if !strings.Contains(notice[0].Text, p.Text) {
					t.Errorf("%s: body text %+v overlaps the footer", tc.language, p)
				}
			}
			if p.Page == 1 && p.Y > numbers[0].Y-8 && p != numbers[0] {
				t.Errorf("%s: body text %+v in the footer of page 1", tc.language, p)
			}
		}
		total := r.find(map[string]string{"nl": "Totaal", "en": "Total"}[tc.language])
		if last := total[len(total)-1]; last.Page != 2 || !last.Bold {
			t.Errorf("%s: total at %+v, want on the last page", tc.language, last)
		}
	}
}

func TestPDFShowsTheDraftsDueDate(t *testing.T) {
	doc := example(t, "en")
	doc.Invoice.InvoiceID, doc.Invoice.DueDate, doc.Invoice.FirstDueInterval = "", nil, 14
	r := layoutOf(t, doc, true)
	r.one(t, "Draft invoice")
	r.one(t, "Due date:")
	r.one(t, "25-11-2026")
	doc.Invoice.Language = "nl"
	r = layoutOf(t, doc, true)
	r.one(t, "Conceptfactuur")
	r.one(t, "Vervaldatum:")
}

func TestPDFOtherDocumentsShareTheLayout(t *testing.T) {
	doc := long(example(t, "en"))
	doc.Estimate = &sales.Estimate{PreText: "Here is our quote.", PostText: "Please sign it.", ShowTax: true}
	r := layoutOf(t, doc, true)
	if len(r.containing("We kindly request")) != 0 {
		t.Error("payment notice on a quote")
	}
	if p := r.one(t, "Please sign it."); p.Page != r.Pages {
		t.Errorf("post text on page %d of %d", p.Page, r.Pages)
	}
	r.one(t, fmt.Sprintf("Page %d of %d", r.Pages, r.Pages))
	r = layoutOf(t, example(t, "en"), false)
	r.one(t, "Packing slip 2026-0001")
	if len(r.find("Due date:")) != 0 || len(r.find("Total")) != 0 || len(r.containing("We kindly request")) != 0 {
		t.Errorf("packing slip with prices or dates: %v", r.texts())
	}
}
