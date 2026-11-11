// Package invoicedoc renders sales invoices: the PDF people receive, a
// packing slip without prices, and the UBL 2.1 e-invoice.
package invoicedoc

import (
	"bytes"
	"fmt"
	"strings"
	"time"

	"github.com/go-pdf/fpdf"

	"github.com/adirkuhn/moneyharpy/internal/money"
	"github.com/adirkuhn/moneyharpy/internal/sales"
)

// Renderer implements sales.Renderer.
type Renderer struct{}

// New returns a Renderer.
func New() Renderer { return Renderer{} }

type labels struct {
	Invoice, Credit, Draft, PackingSlip, Number, Date, Due, Reference, Customer, Description, Quantity, Price, VAT, Amount,
	Subtotal, Discount, VATOver, Total, PayBefore, Page, CoC, VATNumber, IBAN string
}

var dutch = labels{
	Invoice: "Factuur", Credit: "Creditfactuur", Draft: "Concept", PackingSlip: "Pakbon", Number: "Factuurnummer", Date: "Factuurdatum",
	Due: "Vervaldatum", Reference: "Referentie", Customer: "Klantnummer", Description: "Omschrijving", Quantity: "Aantal", Price: "Prijs",
	VAT: "Btw", Amount: "Bedrag", Subtotal: "Subtotaal", Discount: "Korting", VATOver: "%s btw over %s", Total: "Totaal",
	PayBefore: "Graag ontvangen we %s vóór %s op rekening %s ten name van %s, onder vermelding van %s.", Page: "Pagina %d van {nb}",
	CoC: "KVK", VATNumber: "Btw-nummer", IBAN: "IBAN",
}

var english = labels{
	Invoice: "Invoice", Credit: "Credit invoice", Draft: "Draft", PackingSlip: "Packing slip", Number: "Invoice number", Date: "Invoice date",
	Due: "Due date", Reference: "Reference", Customer: "Customer number", Description: "Description", Quantity: "Quantity", Price: "Price",
	VAT: "VAT", Amount: "Amount", Subtotal: "Subtotal", Discount: "Discount", VATOver: "%s VAT on %s", Total: "Total",
	PayBefore: "Please pay %s before %s to account %s in the name of %s, quoting %s.", Page: "Page %d of {nb}",
	CoC: "CoC", VATNumber: "VAT number", IBAN: "IBAN",
}

func labelsFor(language string) labels {
	if strings.HasPrefix(language, "nl") {
		return dutch
	}
	return english
}

// PDF implements sales.Renderer.
func (Renderer) PDF(doc sales.Document) ([]byte, error) { return render(doc, true) }

// PackingSlip implements sales.Renderer.
func (Renderer) PackingSlip(doc sales.Document) ([]byte, error) { return render(doc, false) }

const (
	left, right = 20.0, 190.0
	width       = right - left
)

func render(doc sales.Document, prices bool) ([]byte, error) {
	inv := doc.Invoice
	l := labelsFor(inv.Language)
	size := "A4"
	if doc.Style.PaperSize == "letter" {
		size = "Letter"
	}
	pdf := fpdf.New("P", "mm", size, "")
	pdf.SetTitle(title(doc, l, prices), true)
	pdf.SetCreator("MoneyHarpy", true)
	pdf.AliasNbPages("")
	pdf.SetMargins(left, 20, 210-right)
	pdf.SetAutoPageBreak(true, 25)
	tr := pdf.UnicodeTranslatorFromDescriptor("cp1252")
	fontSize := float64(doc.Style.FontSize)
	if fontSize < 8 || fontSize > 14 {
		fontSize = 10
	}
	pdf.SetFooterFunc(func() {
		pdf.SetY(-15)
		pdf.SetFont("Helvetica", "", 8)
		pdf.SetTextColor(110, 110, 110)
		pdf.CellFormat(0, 5, tr(fmt.Sprintf(l.Page, pdf.PageNo())), "", 0, "C", false, 0, "")
	})
	pdf.AddPage()
	pdf.SetFont("Helvetica", "", fontSize)

	logo(pdf, doc)
	sender(pdf, tr, doc, l, fontSize)
	recipient(pdf, tr, doc, fontSize)

	pdf.SetY(95)
	pdf.SetFont("Helvetica", "B", fontSize+8)
	pdf.CellFormat(0, 10, tr(title(doc, l, prices)), "", 1, "L", false, 0, "")
	pdf.SetFont("Helvetica", "", fontSize)
	meta(pdf, tr, doc, l, prices)
	pdf.Ln(6)
	table(pdf, tr, doc, l, prices, fontSize)
	if prices {
		totals(pdf, tr, doc, l, fontSize)
		payment(pdf, tr, doc, l, fontSize)
	}
	var out bytes.Buffer
	if err := pdf.Output(&out); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

func title(doc sales.Document, l labels, prices bool) string {
	switch {
	case !prices:
		return l.PackingSlip
	case doc.Totals.InclTax < 0 || doc.Invoice.OriginalID != nil:
		return l.Credit
	case doc.Invoice.InvoiceID == "":
		return l.Draft + " " + strings.ToLower(l.Invoice)
	}
	return l.Invoice
}

func logo(pdf *fpdf.Fpdf, doc sales.Document) {
	if len(doc.LogoData) == 0 {
		return
	}
	kind := map[string]string{"image/png": "PNG", "image/jpeg": "JPG", "image/gif": "GIF"}[doc.LogoType]
	if kind == "" {
		return
	}
	opts := fpdf.ImageOptions{ImageType: kind, ReadDpi: true}
	info := pdf.RegisterImageOptionsReader("logo", opts, bytes.NewReader(doc.LogoData))
	if pdf.Err() || info == nil {
		pdf.ClearError()
		return
	}
	w := 50.0
	if doc.Style.LogoDisplayWidth > 0 {
		w = min(float64(doc.Style.LogoDisplayWidth), 90)
	}
	h := w * info.Height() / info.Width()
	if h > 30 {
		h, w = 30, 30*info.Width()/info.Height()
	}
	x := left
	switch doc.Style.LogoPosition {
	case "right":
		x = right - w
	case "center":
		x = left + (width-w)/2
	}
	pdf.ImageOptions("logo", x, 15, w, h, false, opts, 0, "")
}

func sender(pdf *fpdf.Fpdf, tr func(string) string, doc sales.Document, l labels, size float64) {
	id := doc.Identity
	lines := []string{id.Address1, id.Address2, strings.TrimSpace(id.Zipcode + " " + id.City), countryName(id.Country, doc.Invoice.Language), id.Email, id.Phone}
	if id.ChamberOfCommerce != "" {
		lines = append(lines, l.CoC+" "+id.ChamberOfCommerce)
	}
	if id.TaxNumber != "" {
		lines = append(lines, l.VATNumber+" "+id.TaxNumber)
	}
	if id.BankAccountNumber != "" {
		lines = append(lines, l.IBAN+" "+id.BankAccountNumber)
	}
	x := 130.0
	pdf.SetXY(x, 48)
	pdf.SetFont("Helvetica", "B", size)
	pdf.CellFormat(right-x, 5, tr(id.CompanyName), "", 2, "L", false, 0, "")
	pdf.SetFont("Helvetica", "", size-1)
	for _, line := range lines {
		if line != "" {
			pdf.CellFormat(right-x, 4.5, tr(line), "", 2, "L", false, 0, "")
		}
	}
}

func recipient(pdf *fpdf.Fpdf, tr func(string) string, doc sales.Document, size float64) {
	c := doc.Contact
	attention := c.SendInvoicesToAttention
	if doc.ContactPerson != nil {
		attention = strings.TrimSpace(doc.ContactPerson.Firstname + " " + doc.ContactPerson.Lastname)
	}
	lines := []string{c.Name(), attention, c.Address1, c.Address2, strings.TrimSpace(c.Zipcode + " " + c.City)}
	if c.Country != doc.Administration.Country {
		lines = append(lines, countryName(c.Country, doc.Invoice.Language))
	}
	pdf.SetXY(left, 52)
	for i, line := range lines {
		if line == "" || (i == 1 && line == lines[0]) {
			continue
		}
		style := ""
		if i == 0 {
			style = "B"
		}
		pdf.SetFont("Helvetica", style, size)
		pdf.CellFormat(95, 5, tr(line), "", 2, "L", false, 0, "")
	}
}

func meta(pdf *fpdf.Fpdf, tr func(string) string, doc sales.Document, l labels, prices bool) {
	inv := doc.Invoice
	day := func(t *time.Time) string {
		if t == nil {
			return ""
		}
		return t.Format("02-01-2006")
	}
	rows := [][2]string{{l.Number, inv.InvoiceID}, {l.Date, day(inv.InvoiceDate)}}
	if prices {
		rows = append(rows, [2]string{l.Due, day(inv.DueDate)})
	}
	rows = append(rows, [2]string{l.Reference, inv.Reference}, [2]string{l.Customer, doc.Contact.CustomerID})
	for _, r := range rows {
		if r[1] == "" {
			continue
		}
		pdf.SetTextColor(100, 100, 100)
		pdf.CellFormat(40, 5, tr(r[0]), "", 0, "L", false, 0, "")
		pdf.SetTextColor(0, 0, 0)
		pdf.CellFormat(0, 5, tr(r[1]), "", 1, "L", false, 0, "")
	}
}

func table(pdf *fpdf.Fpdf, tr func(string) string, doc sales.Document, l labels, prices bool, size float64) {
	inv := doc.Invoice
	type col struct {
		head  string
		w     float64
		align string
	}
	cols := []col{{l.Description, 90, "L"}, {l.Quantity, 20, "R"}, {l.Price, 25, "R"}, {l.VAT, 15, "R"}, {l.Amount, 20, "R"}}
	if !prices {
		cols = []col{{l.Description, 140, "L"}, {l.Quantity, 30, "R"}}
	}
	pdf.SetFont("Helvetica", "B", size-1)
	pdf.SetDrawColor(180, 180, 180)
	for _, c := range cols {
		pdf.CellFormat(c.w, 7, tr(c.head), "B", 0, c.align, false, 0, "")
	}
	pdf.Ln(-1)
	pdf.SetFont("Helvetica", "", size-1)
	for _, d := range inv.Lines {
		if d.Optional && !d.Selected {
			continue
		}
		rate := doc.Rates[d.TaxRateID]
		total := money.Amount(divRound(int64(d.Price)*d.Quantity, 1000))
		cells := []string{d.Description, d.Amount}
		if prices {
			cells = append(cells, sales.FormatMoney(d.Price, inv.Currency, inv.Language), strings.TrimSuffix(strings.TrimSuffix(rate.Percentage, ".0"), ".00")+"%", sales.FormatMoney(total, inv.Currency, inv.Language))
		}
		// The description may wrap; the other cells line up with its first line.
		y := pdf.GetY()
		if y > 255 {
			pdf.AddPage()
			y = pdf.GetY()
		}
		pdf.MultiCell(cols[0].w, 5, tr(cells[0]), "", "L", false)
		bottom := pdf.GetY()
		x := left + cols[0].w
		for i := 1; i < len(cells); i++ {
			pdf.SetXY(x, y)
			pdf.CellFormat(cols[i].w, 5, tr(cells[i]), "", 0, cols[i].align, false, 0, "")
			x += cols[i].w
		}
		pdf.SetXY(left, max(bottom, y+5)+1)
	}
	pdf.Line(left, pdf.GetY(), right, pdf.GetY())
	pdf.Ln(2)
}

func totals(pdf *fpdf.Fpdf, tr func(string) string, doc sales.Document, l labels, size float64) {
	inv, tot := doc.Invoice, doc.Totals
	m := func(a money.Amount) string { return sales.FormatMoney(a, inv.Currency, inv.Language) }
	row := func(label, value string, bold bool) {
		style := ""
		if bold {
			style = "B"
		}
		pdf.SetFont("Helvetica", style, size)
		pdf.SetX(110)
		pdf.CellFormat(55, 6, tr(label), "", 0, "L", false, 0, "")
		pdf.CellFormat(25, 6, tr(value), "", 1, "R", false, 0, "")
	}
	row(l.Subtotal, m(tot.ExclTax+tot.Discount*boolInt(!inv.PricesAreInclTax)), false)
	if tot.Discount != 0 {
		row(l.Discount, "-"+m(tot.Discount), false)
	}
	for _, r := range tot.ByRate {
		pct := strings.TrimSuffix(strings.TrimSuffix(doc.Rates[r.TaxRateID].Percentage, ".0"), ".00") + "%"
		row(fmt.Sprintf(l.VATOver, pct, m(r.Taxable)), m(r.Tax), false)
	}
	pdf.SetX(110)
	pdf.Line(110, pdf.GetY(), right, pdf.GetY())
	row(l.Total, m(tot.InclTax), true)
}

func boolInt(b bool) money.Amount {
	if b {
		return 1
	}
	return 0
}

func payment(pdf *fpdf.Fpdf, tr func(string) string, doc sales.Document, l labels, size float64) {
	inv, id := doc.Invoice, doc.Identity
	pdf.Ln(8)
	pdf.SetFont("Helvetica", "", size-1)
	if inv.PaymentConditions != "" {
		pdf.MultiCell(0, 5, tr(inv.PaymentConditions), "", "L", false)
		pdf.Ln(2)
	}
	if id.BankAccountNumber == "" || inv.DueDate == nil || doc.Totals.InclTax <= 0 {
		return
	}
	holder := id.BankAccountName
	if holder == "" {
		holder = id.CompanyName
	}
	text := fmt.Sprintf(l.PayBefore, sales.FormatMoney(doc.Totals.InclTax, inv.Currency, inv.Language), inv.DueDate.Format("02-01-2006"),
		id.BankAccountNumber, holder, inv.PaymentReference)
	pdf.MultiCell(0, 5, tr(text), "", "L", false)
}

func divRound(n, d int64) int64 {
	if n < 0 {
		return -((-n + d/2) / d)
	}
	return (n + d/2) / d
}

func countryName(code, language string) string {
	if code == "" {
		return ""
	}
	names := englishCountries
	if strings.HasPrefix(language, "nl") {
		names = dutchCountries
	}
	if name, ok := names[code]; ok {
		return name
	}
	return code
}

var englishCountries = map[string]string{
	"NL": "Netherlands", "BE": "Belgium", "DE": "Germany", "FR": "France", "GB": "United Kingdom", "LU": "Luxembourg", "ES": "Spain",
	"IT": "Italy", "AT": "Austria", "PT": "Portugal", "IE": "Ireland", "DK": "Denmark", "SE": "Sweden", "PL": "Poland", "US": "United States",
	"BR": "Brazil", "CH": "Switzerland",
}

var dutchCountries = map[string]string{
	"NL": "Nederland", "BE": "België", "DE": "Duitsland", "FR": "Frankrijk", "GB": "Verenigd Koninkrijk", "LU": "Luxemburg", "ES": "Spanje",
	"IT": "Italië", "AT": "Oostenrijk", "PT": "Portugal", "IE": "Ierland", "DK": "Denemarken", "SE": "Zweden", "PL": "Polen", "US": "Verenigde Staten",
	"BR": "Brazilië", "CH": "Zwitserland",
}
