// Package invoicedoc renders sales invoices: the PDF people receive, a
// packing slip without prices, and the UBL 2.1 e-invoice; and estimates,
// which look like invoices with their own title and texts.
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
	Invoice, Credit, DraftInvoice, DraftEstimate, PackingSlip, Number, Date, Due, Reference, Description, Quantity, Price, VAT, Amount,
	Subtotal, Discount, VATOver, Total, Page, CoC, VATNumber, IBAN string
	// Estimate, EstimateNumber and EstimateDate label estimates; their due
	// date is labelled Due, "Vervaldatum", as on Moneybird's quotes.
	Estimate, EstimateNumber, EstimateDate string
}

var dutch = labels{
	Invoice: "Factuur", Credit: "Creditfactuur", DraftInvoice: "Conceptfactuur", DraftEstimate: "Conceptofferte", PackingSlip: "Pakbon", Number: "Factuurnummer", Date: "Factuurdatum",
	Due: "Vervaldatum", Reference: "Kenmerk", Description: "Omschrijving", Quantity: "Aantal", Price: "Bedrag",
	VAT: "Btw", Amount: "Bedrag", Subtotal: "Subtotaal excl. btw", Discount: "Korting", VATOver: "%s btw", Total: "Totaal",
	Page: "Pagina %d van {nb}", CoC: "KVK", VATNumber: "Btw-nummer", IBAN: "IBAN",
	Estimate: "Offerte", EstimateNumber: "Offertenummer", EstimateDate: "Offertedatum",
}

var english = labels{
	Invoice: "Invoice", Credit: "Credit invoice", DraftInvoice: "Draft invoice", DraftEstimate: "Draft quote", PackingSlip: "Packing slip", Number: "Invoice number", Date: "Invoice date",
	Due: "Due date", Reference: "Reference", Description: "Description", Quantity: "Qty", Price: "Price",
	VAT: "VAT", Amount: "Amount", Subtotal: "Subtotal excl. VAT", Discount: "Discount", VATOver: "%s VAT", Total: "Total",
	Page: "Page %d of {nb}", CoC: "CoC", VATNumber: "VAT number", IBAN: "IBAN",
	Estimate: "Quote", EstimateNumber: "Quote number", EstimateDate: "Quote date",
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

// The page as Moneybird lays out its default invoice, in millimetres: the
// sender block on the right near the top, the customer's address in an
// envelope window, the title well below it, and a footer with the page
// number on every page.
const (
	left, right = 20.0, 190.0
	width       = right - left
	top         = 15.0
	// senderMargin is the space right of the sender block.
	senderMargin = 10.0
	recipientY   = 47.0
	titleY       = 95.0
	// footerY is where the page number goes, from the bottom of the page.
	footerY = 12.0
	// bodyMargin is the space under the body on a page without the notice.
	bodyMargin = 20.0
	lineHeight = 5.0
	maxLogoH   = 25.0
	// thinRule separates the lines of the table, in the line colour;
	// heavyRule closes the table's header, its last line and the totals,
	// in the text colour.
	thinRule  = 0.2
	heavyRule = 0.35
	// rowPadding is the space above and below a line of the table.
	rowPadding = 1.5
	// columnGap is the least space between the quantity and description.
	columnGap = 3.0
)

// The web app's colours (web/src/styles.css): text in --text, labels and
// the page number in --muted, which reads at 6:1 on white, and the rules
// between lines in --line.
var (
	textColor  = rgb{0x09, 0x26, 0x53}
	mutedColor = rgb{0x4e, 0x64, 0x85}
	lineColor  = rgb{0xd9, 0xe1, 0xed}
)

type rgb struct{ r, g, b int }

func render(doc sales.Document, prices bool) ([]byte, error) {
	var out bytes.Buffer
	if err := build(doc, prices).Output(&out); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

// writer lays a document out page by page.
type writer struct {
	pdf    *fpdf.Fpdf
	face   face
	doc    sales.Document
	l      labels
	prices bool
	size   float64
	cols   []column
	// inTable repeats the table's header on the pages the lines continue on.
	inTable bool
	// last marks the last page, whose footer carries the notice.
	last   bool
	notice *notice
	// scratch measures text before it is laid out.
	scratch *fpdf.Fpdf
}

type column struct {
	head  string
	w     float64
	align string
}

// notice is the payment notice in the footer of the last page.
type notice struct {
	spans  []sales.Span
	qrCode bool
	height float64
}

// build lays the document out.
func build(doc sales.Document, prices bool) *fpdf.Fpdf {
	l := labelsFor(doc.Invoice.Language)
	size := "A4"
	if doc.Style.PaperSize == "letter" {
		size = "Letter"
	}
	pdf := fpdf.New("P", "mm", size, "")
	pdf.SetTitle(title(doc, l, prices), true)
	pdf.SetCreator("MoneyHarpy", true)
	pdf.AliasNbPages("")
	pdf.SetMargins(left, 20, 210-right)
	pdf.SetAutoPageBreak(true, bodyMargin)
	fontSize := float64(doc.Style.FontSize)
	if fontSize < 8 || fontSize > 14 {
		fontSize = 10
	}
	w := &writer{pdf: pdf, face: faceFor(doc.Style.FontFamily), doc: doc, l: l, prices: prices, size: fontSize}
	w.cols = w.columns()
	if prices && doc.Estimate == nil {
		w.notice = w.paymentNotice()
	}
	pdf.SetHeaderFunc(w.header)
	pdf.SetFooterFunc(w.footer)
	pdf.AddPage()
	w.letterhead()
	w.recipient()
	y := w.heading()
	pdf.SetY(y + 8)
	if doc.Estimate != nil && doc.Estimate.PreText != "" {
		w.estimateText(doc.Estimate.PreText)
		pdf.Ln(4)
	}
	w.table()
	if prices {
		w.totals()
	}
	if doc.Estimate != nil && doc.Estimate.PostText != "" {
		pdf.Ln(8)
		w.estimateText(doc.Estimate.PostText)
	}
	if pdf.GetY() > w.lastBottom() {
		pdf.AddPage()
	}
	w.last = true
	return pdf
}

// estimateText writes a text of a quote, above or under its lines, styled
// by its textile and with its tags filled in, from the left margin.
func (w *writer) estimateText(text string) {
	w.font("", w.size-1)
	w.pdf.SetX(left)
	writeSpans(w.pdf, w.face, sales.EstimateTextSpans(w.doc, text, time.Now().UTC()), w.size-1)
	w.pdf.Ln(lineHeight)
}

// font selects the face in a style and size, in the text colour.
func (w *writer) font(style string, size float64) {
	w.face.set(w.pdf, style, size)
	w.color(textColor)
}

func (w *writer) color(c rgb) { w.pdf.SetTextColor(c.r, c.g, c.b) }

func (w *writer) pageHeight() float64 {
	_, h := w.pdf.GetPageSize()
	return h
}

// bottom is where the body ends on pages before the last.
func (w *writer) bottom() float64 { return w.pageHeight() - bodyMargin }

// lastBottom is where the body ends on the last page, above the notice.
func (w *writer) lastBottom() float64 {
	if w.notice == nil {
		return w.bottom()
	}
	return w.noticeTop() - 6
}

func (w *writer) noticeTop() float64 {
	return w.pageHeight() - footerY - 4 - w.notice.height
}

func (w *writer) header() {
	if w.inTable {
		w.pdf.SetY(20)
		w.tableHead()
	}
}

// footer prints the page number on every page and, on the last, the
// payment notice above it under a full-width rule.
func (w *writer) footer() {
	pdf := w.pdf
	if w.last && w.notice != nil {
		y := w.noticeTop()
		w.rule(y-3, left, right, thinRule)
		marginLeft, marginTop, marginRight, _ := pdf.GetMargins()
		if w.notice.qrCode {
			pdf.SetRightMargin(marginRight + 34)
			epcCode(pdf, w.doc, right, y)
		}
		pdf.SetXY(left, y)
		w.font("", w.size-1)
		writeSpans(pdf, w.face, w.notice.spans, w.size-1)
		pdf.SetMargins(marginLeft, marginTop, marginRight)
	}
	pdf.SetY(-footerY)
	w.font("", w.size-1.5)
	w.color(mutedColor)
	pdf.CellFormat(0, lineHeight, fmt.Sprintf(w.l.Page, pdf.PageNo()), "", 0, "C", false, 0, "")
}

// paymentNotice is the invoice's payment conditions, styled by their
// textile and with their tags filled in, as Moneybird prints them; and
// the payment QR code beside them. Nil when there is neither.
func (w *writer) paymentNotice() *notice {
	_, qrCode := epcPayload(w.doc)
	n := &notice{spans: sales.PaymentConditionsSpans(w.doc, time.Now().UTC()), qrCode: qrCode}
	if len(n.spans) == 0 && !qrCode {
		return nil
	}
	if len(n.spans) > 0 {
		n.height = w.measure(n.spans, qrCode)
	}
	if qrCode {
		n.height = max(n.height, 28)
	}
	return n
}

// measure is how high the spans are as the footer writes them.
func (w *writer) measure(spans []sales.Span, qrCode bool) float64 {
	pageW, _ := w.pdf.GetPageSize()
	marginRight := pageW - right
	if qrCode {
		marginRight += 34
	}
	scratch := w.scratchPage()
	scratch.SetMargins(left, 0, marginRight)
	scratch.SetXY(left, 10)
	writeSpans(scratch, w.face, spans, w.size-1)
	return scratch.GetY() - 10 + lineHeight
}

// wrappedHeight is how high text is as MultiCell writes it in a column of
// width cw.
func (w *writer) wrappedHeight(text string, cw, size float64) float64 {
	scratch := w.scratchPage()
	w.face.set(scratch, "", size)
	scratch.SetXY(left, 10)
	scratch.MultiCell(cw, lineHeight, text, "", "L", false)
	return max(scratch.GetY()-10, lineHeight)
}

// scratchPage is a page of the document's size to measure text on.
func (w *writer) scratchPage() *fpdf.Fpdf {
	if w.scratch == nil {
		pageW, pageH := w.pdf.GetPageSize()
		w.scratch = fpdf.NewCustom(&fpdf.InitType{UnitStr: "mm", Size: fpdf.SizeType{Wd: pageW, Ht: pageH}})
		w.scratch.SetAutoPageBreak(false, 0)
		w.scratch.AddPage()
	}
	return w.scratch
}

// letterhead prints the logo and the sender block, which the logo's top
// lines up with; neither on stationery.
func (w *writer) letterhead() {
	if w.doc.Stationery {
		return
	}
	senderY := top
	size := w.size - 1
	// Where the capitals of the sender's first line start in its cell.
	capOffset := 4.5/2 + (0.3-w.face.capHeight(w.pdf))*size*25.4/72
	if lw, lh, ok := w.logoSize(); ok {
		x := left
		y := senderY + capOffset
		switch w.doc.Style.LogoPosition {
		case "right":
			x, y = right-lw, top
			senderY = top + lh + 4
		case "center":
			x = left + (width-lw)/2
		}
		w.pdf.ImageOptions("logo", x, y, lw, lh, false, fpdf.ImageOptions{}, 0, "")
	}
	if layout(w.doc).HideSenderAddress {
		return
	}
	lines := senderLines(w.doc, w.l)
	// The block starts where the style puts it, or further left when its
	// widest line would cross the right margin in this face.
	pageW, _ := w.pdf.GetPageSize()
	x := min(pageW-senderMargin-senderWidth(w.doc), right-w.widest(lines, size)-w.pdf.GetCellMargin())
	w.pdf.SetXY(x, senderY)
	w.color(textColor)
	drawStyled(w.pdf, w.face, lines, x, right-x, size)
}

// widest is the width of the widest of the lines as drawStyled prints them.
func (w *writer) widest(lines []styledLine, size float64) float64 {
	scratch := w.scratchPage()
	widest := 0.0
	for _, line := range lines {
		style := ""
		if line.Bold {
			style = "B"
		}
		w.face.set(scratch, style, size)
		for i, text := range line.Text {
			if i == 0 && line.Label != "" {
				text = line.Label + ": " + text
			}
			widest = max(widest, scratch.GetStringWidth(text))
		}
	}
	return widest
}

// logoSize registers the logo and is the size it prints at: the style's
// width, at most 25 mm high, keeping its proportions.
func (w *writer) logoSize() (float64, float64, bool) {
	doc := w.doc
	if len(doc.LogoData) == 0 {
		return 0, 0, false
	}
	kind := map[string]string{"image/png": "PNG", "image/jpeg": "JPG", "image/gif": "GIF"}[doc.LogoType]
	if kind == "" {
		return 0, 0, false
	}
	info := w.pdf.RegisterImageOptionsReader("logo", fpdf.ImageOptions{ImageType: kind, ReadDpi: true}, bytes.NewReader(doc.LogoData))
	if w.pdf.Err() || info == nil || info.Width() <= 0 || info.Height() <= 0 {
		w.pdf.ClearError()
		return 0, 0, false
	}
	lw := 50.0
	if doc.Style.LogoDisplayWidth > 0 {
		lw = min(float64(doc.Style.LogoDisplayWidth), 90)
	}
	lh := lw * info.Height() / info.Width()
	if lh > maxLogoH {
		lh, lw = maxLogoH, maxLogoH*info.Width()/info.Height()
	}
	return lw, lh, true
}

// recipient prints the customer's address in the envelope window, and
// what the style prints under it.
func (w *writer) recipient() {
	pdf, doc := w.pdf, w.doc
	pdf.SetXY(left, recipientY)
	w.font("", w.size)
	for _, line := range addressLines(doc) {
		pdf.CellFormat(95, lineHeight, line, "", 2, "L", false, 0, "")
	}
	if info := recipientInfo(doc, w.l); len(info) > 0 {
		pdf.SetX(left)
		pdf.CellFormat(95, 2, "", "", 2, "L", false, 0, "")
		drawStyled(pdf, w.face, info, left, 95, w.size-1)
	}
}

// addressLines is the customer's address as Moneybird prints it: the city
// in capitals in the Netherlands, the country only when it is not the
// sender's.
func addressLines(doc sales.Document) []string {
	c := doc.Contact
	attention := c.SendInvoicesToAttention
	if doc.Estimate != nil {
		attention = c.SendEstimatesToAttention
	}
	if doc.ContactPerson != nil {
		attention = strings.TrimSpace(doc.ContactPerson.Firstname + " " + doc.ContactPerson.Lastname)
	}
	country := c.Country
	if country == "" {
		country = doc.Administration.Country
	}
	city := c.City
	if country == "NL" {
		city = strings.ToUpper(city)
	}
	name := c.Name()
	if attention == name {
		attention = ""
	}
	lines := nonEmpty(name, attention, c.Address1, c.Address2, strings.TrimSpace(c.Zipcode+" "+city))
	if c.Country != doc.Administration.Country {
		lines = append(lines, nonEmpty(countryName(c.Country, doc.Invoice.Language))...)
	}
	return lines
}

// heading prints the title with the style's left metadata under it and
// its right metadata beside it, and is where it ends.
func (w *writer) heading() float64 {
	pdf := w.pdf
	titleSize := w.size * 2.3
	w.font("", titleSize)
	pdf.SetXY(left, titleY)
	pdf.CellFormat(0, 10, title(w.doc, w.l, w.prices), "", 1, "L", false, 0, "")
	leftRows, rightRows := metaColumns(w.doc, w.l, w.prices)
	size := w.size - 1
	mm := func(pt float64) float64 { return pt * 25.4 / 72 }
	// The right column's first line sits on the title's baseline.
	baseline := titleY + 5 + 0.3*mm(titleSize)
	rightY := baseline - lineHeight/2 - 0.3*mm(size)
	leftEnd := w.metaBlock(leftRows, left, titleY+12, size, false)
	rightEnd := w.metaBlock(rightRows, right, rightY, size, true)
	return max(leftEnd, rightEnd, titleY+11)
}

// metaBlock prints label and value columns from x, or ending at x when
// alignRight, and is where they end.
func (w *writer) metaBlock(rows [][2]string, x, y, size float64, alignRight bool) float64 {
	if len(rows) == 0 {
		return y
	}
	pdf := w.pdf
	w.font("", size)
	labelW, valueW := 0.0, 0.0
	for _, r := range rows {
		labelW = max(labelW, pdf.GetStringWidth(r[0]+":"))
		valueW = max(valueW, pdf.GetStringWidth(r[1]))
	}
	labelW += 4
	valueW += 2 * pdf.GetCellMargin()
	if alignRight {
		x -= labelW + valueW
	}
	pdf.SetY(y)
	for _, r := range rows {
		pdf.SetX(x)
		w.color(mutedColor)
		pdf.CellFormat(labelW, lineHeight, r[0]+":", "", 0, "L", false, 0, "")
		w.color(textColor)
		pdf.CellFormat(valueW, lineHeight, r[1], "", 1, "L", false, 0, "")
	}
	return pdf.GetY()
}

// columns are the lines table's: the quantity, the description and, with
// prices, the price, line total and VAT rate. The quantity column is as wide
// as its header or widest quantity in this face, with columnGap to the
// description, which takes the rest.
func (w *writer) columns() []column {
	l := w.l
	quantity := 15.0
	if !w.prices {
		quantity = 20
	}
	scratch := w.scratchPage()
	w.face.set(scratch, "B", w.size-1.5)
	widest := scratch.GetStringWidth(l.Quantity)
	w.face.set(scratch, "", w.size-0.5)
	for _, d := range w.lines() {
		widest = max(widest, scratch.GetStringWidth(d.Amount))
	}
	quantity = max(quantity, widest+scratch.GetCellMargin()+columnGap)
	if !w.prices {
		return []column{{l.Quantity, quantity, "L"}, {l.Description, width - quantity, "L"}}
	}
	return []column{{l.Quantity, quantity, "L"}, {l.Description, width - quantity - 60, "L"}, {l.Price, 25, "R"}, {l.Total, 25, "R"}, {l.VAT, 10, "R"}}
}

// lines are the invoice's lines the document prints: not the optional ones
// the customer left out.
func (w *writer) lines() []sales.Detail {
	var lines []sales.Detail
	for _, d := range w.doc.Invoice.Lines {
		if !d.Optional || d.Selected {
			lines = append(lines, d)
		}
	}
	return lines
}

func (w *writer) tableHead() {
	pdf := w.pdf
	w.font("B", w.size-1.5)
	pdf.SetX(left)
	for _, c := range w.cols {
		pdf.CellFormat(c.w, 7, c.head, "", 0, c.align, false, 0, "")
	}
	pdf.Ln(7)
	w.rule(pdf.GetY(), left, right, heavyRule)
}

// rule draws a horizontal line: a heavy one in the text colour, a thin one
// in the line colour.
func (w *writer) rule(y, from, to, lineWidth float64) {
	c := lineColor
	if lineWidth >= heavyRule {
		c = textColor
	}
	w.pdf.SetDrawColor(c.r, c.g, c.b)
	w.pdf.SetLineWidth(lineWidth)
	w.pdf.Line(from, y, to, y)
}

// table prints the lines, a thin rule between them and a heavier one under
// the last; lines that do not fit go to the next page under the header.
func (w *writer) table() {
	pdf, inv := w.pdf, w.doc.Invoice
	lines := w.lines()
	w.tableHead()
	w.inTable = true
	for i, d := range lines {
		cells := []string{d.Amount, d.Description}
		if w.prices {
			rate := w.doc.Rates[d.TaxRateID]
			total := money.Amount(divRound(int64(d.Price)*d.Quantity, 1000))
			cells = append(cells, sales.FormatMoney(d.Price, inv.Currency, inv.Language), sales.FormatMoney(total, inv.Currency, inv.Language), percent(rate.Percentage))
		}
		height := w.wrappedHeight(d.Description, w.cols[1].w, w.size-0.5) + 2*rowPadding
		if pdf.GetY()+height > w.bottom() {
			pdf.AddPage()
		}
		w.font("", w.size-0.5)
		y := pdf.GetY() + rowPadding
		x := left
		for j, c := range w.cols {
			pdf.SetXY(x, y)
			if j == 1 {
				pdf.MultiCell(c.w, lineHeight, cells[j], "", "L", false)
			} else {
				pdf.CellFormat(c.w, lineHeight, cells[j], "", 0, c.align, false, 0, "")
			}
			x += c.w
		}
		pdf.SetXY(left, y+height-rowPadding)
		if i < len(lines)-1 {
			w.rule(pdf.GetY(), left, right, thinRule)
		}
	}
	w.rule(pdf.GetY(), left, right, heavyRule)
	w.inTable = false
}

func percent(p string) string {
	return strings.TrimSuffix(strings.TrimSuffix(p, ".0"), ".00") + "%"
}

// totals prints the subtotal, the VAT per rate and the total under the
// table, the amounts under the line totals; on the last page.
func (w *writer) totals() {
	pdf, doc := w.pdf, w.doc
	inv, tot := doc.Invoice, doc.Totals
	m := func(a money.Amount) string { return sales.FormatMoney(a, inv.Currency, inv.Language) }
	rows := [][2]string{{w.l.Subtotal, m(tot.ExclTax + tot.Discount*boolInt(!inv.PricesAreInclTax))}}
	if tot.Discount != 0 {
		rows = append(rows, [2]string{w.l.Discount, "-" + m(tot.Discount)})
	}
	if doc.Estimate == nil || doc.Estimate.ShowTax {
		for _, r := range tot.ByRate {
			rows = append(rows, [2]string{fmt.Sprintf(w.l.VATOver, percent(doc.Rates[r.TaxRateID].Percentage)), m(r.Tax)})
		}
	}
	const rowH = 6.0
	amountX, amountW := left+w.cols[0].w+w.cols[1].w+w.cols[2].w, w.cols[3].w
	labelX, labelW := 100.0, amountX-3-100
	if pdf.GetY()+2+float64(len(rows)+1)*rowH+2 > w.lastBottom() {
		pdf.AddPage()
	}
	pdf.Ln(2)
	row := func(r [2]string, style string) {
		w.font(style, w.size)
		pdf.SetX(labelX)
		pdf.CellFormat(labelW, rowH, r[0], "", 0, "R", false, 0, "")
		pdf.SetX(amountX)
		pdf.CellFormat(amountW, rowH, r[1], "", 1, "R", false, 0, "")
	}
	for _, r := range rows {
		row(r, "")
	}
	pdf.Ln(1)
	w.rule(pdf.GetY(), labelX, amountX+amountW, heavyRule)
	pdf.Ln(1)
	row([2]string{w.l.Total, m(tot.InclTax)}, "B")
}

func boolInt(b bool) money.Amount {
	if b {
		return 1
	}
	return 0
}

// title names the document with its number, as Moneybird's heading
// "Invoice 2026-0001" does; a draft has no number yet.
func title(doc sales.Document, l labels, prices bool) string {
	number := doc.Invoice.InvoiceID
	name := l.Invoice
	switch {
	case doc.Estimate != nil && number == "":
		return l.DraftEstimate
	case doc.Estimate != nil:
		name = l.Estimate
	case !prices:
		name = l.PackingSlip
	case doc.Totals.InclTax < 0 || doc.Invoice.OriginalID != nil:
		name = l.Credit
	case number == "":
		return l.DraftInvoice
	}
	return strings.TrimSpace(name + " " + number)
}

// writeSpans writes textile spans as flowing text, each in its style.
func writeSpans(pdf *fpdf.Fpdf, f face, spans []sales.Span, size float64) {
	for _, sp := range spans {
		style := ""
		if sp.Bold {
			style += "B"
		}
		if sp.Italic {
			style += "I"
		}
		if sp.Underline {
			style += "U"
		}
		f.set(pdf, style, size)
		pdf.Write(5, sp.Text)
	}
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
