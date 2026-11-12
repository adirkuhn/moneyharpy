package invoicedoc

import (
	"fmt"
	"strings"
	"time"

	"github.com/go-pdf/fpdf"
	"rsc.io/qr"

	"github.com/adirkuhn/moneyharpy/internal/invoicing"
	"github.com/adirkuhn/moneyharpy/internal/resource"
	"github.com/adirkuhn/moneyharpy/internal/sales"
)

// styledLine is one entry of a block the document style lays out: a blank
// line when Text is empty, else a field's text with its formatting.
type styledLine struct {
	Label        string
	Text         []string
	Bold, Italic bool
}

// fieldNames are the labels fields print with when a layout asks for them.
var fieldNames = map[bool]map[string]string{
	false: {
		"company_name": "Company name", "address": "Address", "email": "Email", "phone": "Phone", "website": "Website",
		"chamber_of_commerce": "CoC", "tax_number": "VAT number", "bank_account_number": "IBAN", "bank_account_bic": "BIC",
		"bank_account_name": "Account holder", "customer_tax_number": "VAT number", "customer_chamber_of_commerce": "CoC number",
		"customer_id": "Customer number", "customer_email": "Email address", "customer_phone": "Phone number", "reference": "Reference",
		"invoice_id": "Invoice number", "invoice_date": "Invoice date", "due_date": "Due date", "estimate_id": "Quote number",
		"estimate_date": "Quote date",
	},
	true: {
		"company_name": "Bedrijfsnaam", "address": "Adres", "email": "E-mail", "phone": "Telefoon", "website": "Website",
		"chamber_of_commerce": "KVK", "tax_number": "Btw-nummer", "bank_account_number": "IBAN", "bank_account_bic": "BIC",
		"bank_account_name": "Rekeninghouder", "customer_tax_number": "Btw-nummer", "customer_chamber_of_commerce": "KVK-nummer",
		"customer_id": "Klantnummer", "customer_email": "E-mailadres", "customer_phone": "Telefoonnummer", "reference": "Referentie",
		"invoice_id": "Factuurnummer", "invoice_date": "Factuurdatum", "due_date": "Vervaldatum", "estimate_id": "Offertenummer",
		"estimate_date": "Offertedatum",
	},
}

// layout is the style's layout, or the default one for a document without
// a style.
func layout(doc sales.Document) invoicing.Layout {
	if doc.Style.ID == 0 {
		return invoicing.DefaultLayout()
	}
	return doc.Style.Layout
}

func isEstimate(doc sales.Document) bool { return doc.Estimate != nil }

func pdfDay(t *time.Time) string {
	if t == nil {
		return ""
	}
	return t.Format("02-01-2006")
}

// fieldValue is what a layout field prints for the document, with its name.
func fieldValue(doc sales.Document, field string, l labels) (string, []string) {
	nl := l == dutch
	id, c, inv := doc.Identity, doc.Contact, doc.Invoice
	name := fieldNames[nl][field]
	if rest, ok := strings.CutPrefix(field, invoicing.CustomFieldPrefix); ok {
		fid, err := resource.ParseID(rest)
		if err != nil {
			return "", nil
		}
		for _, v := range id.CustomFields {
			if v.ID == fid {
				return v.Name, nonEmpty(v.Value)
			}
		}
		for _, v := range c.CustomFields {
			if v.FieldID == fid {
				return v.Name, nonEmpty(v.Value)
			}
		}
		for _, v := range inv.CustomFields {
			if v.FieldID == fid {
				return v.Name, nonEmpty(v.Value)
			}
		}
		return "", nil
	}
	customerEmail := c.SendInvoicesToEmail
	if isEstimate(doc) {
		customerEmail = c.SendEstimatesToEmail
	}
	due := inv.DueDate
	if isEstimate(doc) && field == "due_date" {
		name = l.ValidUntil
	}
	switch field {
	case "company_name":
		return name, nonEmpty(id.CompanyName)
	case "address":
		lines := nonEmpty(id.Address1, id.Address2, strings.TrimSpace(id.Zipcode+" "+id.City))
		if c.Country != "" && id.Country != "" && c.Country != id.Country {
			lines = append(lines, countryName(id.Country, inv.Language))
		}
		return name, lines
	case "email":
		return name, nonEmpty(id.Email)
	case "phone":
		return name, nonEmpty(id.Phone)
	case "chamber_of_commerce":
		return name, nonEmpty(id.ChamberOfCommerce)
	case "tax_number":
		return name, nonEmpty(id.TaxNumber)
	case "bank_account_number":
		return name, nonEmpty(id.BankAccountNumber)
	case "bank_account_bic":
		return name, nonEmpty(id.BankAccountBIC)
	case "bank_account_name":
		return name, nonEmpty(id.BankAccountName)
	case "customer_tax_number":
		return name, nonEmpty(c.TaxNumber)
	case "customer_chamber_of_commerce":
		return name, nonEmpty(c.ChamberOfCommerce)
	case "customer_id":
		return name, nonEmpty(c.CustomerID)
	case "customer_email":
		return name, nonEmpty(customerEmail)
	case "customer_phone":
		return name, nonEmpty(c.Phone)
	case "reference":
		return name, nonEmpty(inv.Reference)
	case "invoice_id", "estimate_id":
		return name, nonEmpty(inv.InvoiceID)
	case "invoice_date", "estimate_date":
		return name, nonEmpty(pdfDay(inv.InvoiceDate))
	case "due_date":
		return name, nonEmpty(pdfDay(due))
	}
	// Fields MoneyHarpy has no value for, such as the website, print nothing.
	return name, nil
}

func nonEmpty(values ...string) []string {
	var out []string
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			out = append(out, v)
		}
	}
	return out
}

func styledLines(doc sales.Document, lines []invoicing.Line, l labels) []styledLine {
	var out []styledLine
	for _, line := range lines {
		if line.Field == "newline" {
			out = append(out, styledLine{})
			continue
		}
		name, text := fieldValue(doc, line.Field, l)
		if len(text) == 0 {
			continue
		}
		s := styledLine{Text: text, Bold: line.Bold, Italic: line.Italic}
		if line.Label {
			s.Label = name
		}
		out = append(out, s)
	}
	return out
}

// senderLines is the sender block as the style lays it out.
func senderLines(doc sales.Document, l labels) []styledLine {
	ly := layout(doc)
	if isEstimate(doc) {
		return styledLines(doc, ly.EstimateSenderAddress, l)
	}
	return styledLines(doc, ly.InvoiceSenderAddress, l)
}

// recipientInfo is what the style prints under the customer's address.
func recipientInfo(doc sales.Document, l labels) []styledLine {
	ly := layout(doc)
	if isEstimate(doc) {
		return styledLines(doc, ly.EstimateRecipientAddress, l)
	}
	return styledLines(doc, ly.InvoiceRecipientAddress, l)
}

// metaRows are the document's details beside its title: its number, then
// the style's left and right columns. A packing slip has no due date.
func metaRows(doc sales.Document, l labels, prices bool) [][2]string {
	ly := layout(doc)
	number, columns := "invoice_id", append(append([]invoicing.Line{}, ly.InvoiceMetadataLeft...), ly.InvoiceMetadataRight...)
	if isEstimate(doc) {
		number, columns = "estimate_id", append(append([]invoicing.Line{}, ly.EstimateMetadataLeft...), ly.EstimateMetadataRight...)
	}
	var rows [][2]string
	add := func(field string) {
		name, text := fieldValue(doc, field, l)
		if len(text) > 0 {
			rows = append(rows, [2]string{name, strings.Join(text, ", ")})
		}
	}
	add(number)
	for _, line := range columns {
		if line.Field == number || line.Field == "newline" || (!prices && !isEstimate(doc) && line.Field == "due_date") {
			continue
		}
		add(line.Field)
	}
	return rows
}

// drawStyled prints lines from the current position in a column of width w.
func drawStyled(pdf *fpdf.Fpdf, tr func(string) string, lines []styledLine, x, w, size float64) {
	for _, line := range lines {
		if len(line.Text) == 0 {
			pdf.SetX(x)
			pdf.CellFormat(w, 3, "", "", 2, "L", false, 0, "")
			continue
		}
		style := ""
		if line.Bold {
			style += "B"
		}
		if line.Italic {
			style += "I"
		}
		pdf.SetFont("Helvetica", style, size)
		for i, text := range line.Text {
			if i == 0 && line.Label != "" {
				text = line.Label + ": " + text
			}
			pdf.SetX(x)
			pdf.CellFormat(w, 4.5, tr(text), "", 2, "L", false, 0, "")
		}
	}
	pdf.SetFont("Helvetica", "", size)
}

// senderWidth turns the style's sender width in pixels into millimetres
// that fit beside the customer's address.
func senderWidth(doc sales.Document) float64 {
	px := layout(doc).SenderAddressWidth
	if px <= 0 {
		px = 200
	}
	return min(max(float64(px)*0.3, 30), 90)
}

// epcPayload is the EPC QR code text (European Payments Council, version 2)
// that lets a banking app pay the invoice, if the invoice can carry one.
func epcPayload(doc sales.Document) (string, bool) {
	inv, id := doc.Invoice, doc.Identity
	iban := strings.ReplaceAll(id.BankAccountNumber, " ", "")
	if !layout(doc).ShowEPCCode || isEstimate(doc) || iban == "" || inv.Currency != "EUR" || doc.Totals.InclTax <= 0 || doc.Contact.DirectDebit ||
		(doc.Workflow.ID != 0 && !doc.Workflow.ShowQRCode) {
		return "", false
	}
	name := id.BankAccountName
	if name == "" {
		name = id.CompanyName
	}
	if len([]rune(name)) > 70 {
		name = string([]rune(name)[:70])
	}
	reference := inv.PaymentReference
	if reference == "" {
		reference = inv.InvoiceID
	}
	return strings.Join([]string{"BCD", "002", "1", "SCT", id.BankAccountBIC, name, iban, fmt.Sprintf("EUR%d.%02d", doc.Totals.InclTax/100, doc.Totals.InclTax%100), "", "", reference}, "\n"), true
}

// epcCode draws the payment QR code with its top right corner at x, y.
func epcCode(pdf *fpdf.Fpdf, doc sales.Document, x, y float64) {
	text, ok := epcPayload(doc)
	if !ok {
		return
	}
	code, err := qr.Encode(text, qr.M)
	if err != nil {
		return
	}
	const size = 28.0
	module := size / float64(code.Size)
	pdf.SetFillColor(0, 0, 0)
	for row := range code.Size {
		for col := range code.Size {
			if code.Black(col, row) {
				pdf.Rect(x-size+float64(col)*module, y+float64(row)*module, module, module, "F")
			}
		}
	}
	pdf.SetFillColor(255, 255, 255)
}
