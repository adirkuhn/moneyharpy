package reports

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"fmt"
	"strings"
	"time"

	"github.com/adirkuhn/moneyharpy/internal/journal"
	"github.com/adirkuhn/moneyharpy/internal/money"
	"github.com/adirkuhn/moneyharpy/internal/resource"
	"github.com/adirkuhn/moneyharpy/internal/taxonomy"
)

// decimal writes an amount as XAF and spreadsheets want it: 1234.50.
func decimal(a money.Amount) string {
	sign := ""
	if a < 0 {
		sign, a = "-", -a
	}
	return fmt.Sprintf("%s%d.%02d", sign, a/100, a%100)
}

// side splits an amount into its absolute value and D or C.
func side(a money.Amount) (string, string) {
	if a < 0 {
		return decimal(-a), "C"
	}
	return decimal(a), "D"
}

// journalOf is the XAF journal a document type books in.
func journalOf(documentType string) (id, desc, kind string) {
	switch documentType {
	case journal.SalesInvoice, journal.ExternalSalesInvoice:
		return "VRK", "Verkoopboek", "S"
	case "PurchaseInvoice", "Receipt":
		return "INK", "Inkoopboek", "P"
	case journal.FinancialMutation:
		return "BNK", "Bank", "B"
	default:
		return "MEM", "Memoriaal", "M"
	}
}

type xafAuditfile struct {
	XMLName xml.Name   `xml:"auditfile"`
	XMLNS   string     `xml:"xmlns,attr"`
	Header  xafHeader  `xml:"header"`
	Company xafCompany `xml:"company"`
}

type xafHeader struct {
	FiscalYear      int    `xml:"fiscalYear"`
	StartDate       string `xml:"startDate"`
	EndDate         string `xml:"endDate"`
	CurCode         string `xml:"curCode"`
	DateCreated     string `xml:"dateCreated"`
	SoftwareDesc    string `xml:"softwareDesc"`
	SoftwareVersion string `xml:"softwareVersion"`
}

type xafCompany struct {
	CompanyName        string           `xml:"companyName"`
	TaxRegCountry      string           `xml:"taxRegistrationCountry"`
	CustomersSuppliers *xafCustSups     `xml:"customersSuppliers,omitempty"`
	GeneralLedger      xafGeneralLedger `xml:"generalLedger"`
	Periods            xafPeriods       `xml:"periods"`
	OpeningBalance     *xafOpening      `xml:"openingBalance,omitempty"`
	Transactions       xafTransactions  `xml:"transactions"`
}

type xafCustSups struct {
	Items []xafCustSup `xml:"customerSupplier"`
}

type xafCustSup struct {
	ID          string `xml:"custSupID"`
	Name        string `xml:"custSupName"`
	TaxRegIdent string `xml:"taxRegIdent,omitempty"`
	Type        string `xml:"custSupTp"`
}

type xafGeneralLedger struct {
	Accounts []xafAccount `xml:"ledgerAccount"`
}

type xafAccount struct {
	ID   string `xml:"accID"`
	Desc string `xml:"accDesc"`
	Type string `xml:"accTp"`
}

type xafPeriods struct {
	Items []xafPeriod `xml:"period"`
}

type xafPeriod struct {
	Number int    `xml:"periodNumber"`
	Desc   string `xml:"periodDesc"`
	Start  string `xml:"startDatePeriod"`
	End    string `xml:"endDatePeriod"`
}

type xafOpening struct {
	Date        string           `xml:"opBalDate"`
	Desc        string           `xml:"opBalDesc"`
	LinesCount  int              `xml:"linesCount"`
	TotalDebit  string           `xml:"totalDebit"`
	TotalCredit string           `xml:"totalCredit"`
	Lines       []xafOpeningLine `xml:"obLine"`
}

type xafOpeningLine struct {
	Nr     int    `xml:"nr"`
	AccID  string `xml:"accID"`
	Amount string `xml:"amnt"`
	Type   string `xml:"amntTp"`
}

type xafTransactions struct {
	LinesCount  int          `xml:"linesCount"`
	TotalDebit  string       `xml:"totalDebit"`
	TotalCredit string       `xml:"totalCredit"`
	Journals    []xafJournal `xml:"journal"`
}

type xafJournal struct {
	ID           string           `xml:"jrnID"`
	Desc         string           `xml:"desc"`
	Type         string           `xml:"jrnTp"`
	Transactions []xafTransaction `xml:"transaction"`
}

type xafTransaction struct {
	Nr     string      `xml:"nr"`
	Desc   string      `xml:"desc"`
	Period int         `xml:"periodNumber"`
	Date   string      `xml:"trDt"`
	Amount string      `xml:"amnt"`
	Type   string      `xml:"amntTp"`
	Lines  []xafTrLine `xml:"trLine"`
	debit  money.Amount
}

type xafTrLine struct {
	Nr        string `xml:"nr"`
	AccID     string `xml:"accID"`
	DocRef    string `xml:"docRef"`
	EffDate   string `xml:"effDate"`
	Desc      string `xml:"desc"`
	Amount    string `xml:"amnt"`
	Type      string `xml:"amntTp"`
	CustSupID string `xml:"custSupID,omitempty"`
}

// auditfile writes the year as an XAF 3.2 auditfile, the Dutch tax
// authority's audit format: the categories, the months, the opening
// balance and every booking by journal.
func auditfile(b book, contacts []Contact, now time.Time) ([]byte, error) {
	country := b.admin.Country
	if country == "" {
		country = "NL"
	}
	out := xafAuditfile{
		XMLNS: "http://www.auditfiles.nl/XAF/3.2",
		Header: xafHeader{
			FiscalYear: b.year, StartDate: b.period.From.Format(time.DateOnly), EndDate: b.period.To.Format(time.DateOnly),
			CurCode: b.admin.Currency, DateCreated: now.Format(time.DateOnly), SoftwareDesc: "MoneyHarpy", SoftwareVersion: "1",
		},
		Company: xafCompany{CompanyName: b.admin.Name, TaxRegCountry: strings.ToUpper(country)},
	}
	if len(contacts) > 0 {
		out.Company.CustomersSuppliers = &xafCustSups{}
		for _, c := range contacts {
			out.Company.CustomersSuppliers.Items = append(out.Company.CustomersSuppliers.Items, xafCustSup{
				ID: c.ID.String(), Name: c.Name, TaxRegIdent: c.TaxNumber, Type: "B",
			})
		}
	}
	for _, a := range b.accounts {
		kind := "P"
		if OnBalanceSheet(a.Type) {
			kind = "B"
		}
		out.Company.GeneralLedger.Accounts = append(out.Company.GeneralLedger.Accounts, xafAccount{ID: a.Code, Desc: a.Name, Type: kind})
	}
	for m := 1; m <= 12; m++ {
		from := time.Date(b.year, time.Month(m), 1, 0, 0, 0, 0, time.UTC)
		out.Company.Periods.Items = append(out.Company.Periods.Items, xafPeriod{
			Number: m, Desc: from.Format("January 2006"), Start: from.Format(time.DateOnly), End: endOfMonth(from).Format(time.DateOnly),
		})
	}
	if lines := openingLines(b); len(lines) > 0 {
		ob := &xafOpening{Date: b.period.From.Format(time.DateOnly), Desc: "Beginbalans", LinesCount: len(lines)}
		var debit, credit money.Amount
		for i, l := range lines {
			amnt, tp := side(l.Amount)
			ob.Lines = append(ob.Lines, xafOpeningLine{Nr: i + 1, AccID: b.index[l.LedgerAccountID].Code, Amount: amnt, Type: tp})
			debit += max(l.Amount, 0)
			credit += max(-l.Amount, 0)
		}
		ob.TotalDebit, ob.TotalCredit = decimal(debit), decimal(credit)
		out.Company.OpeningBalance = ob
	}
	journals := map[string]*xafJournal{}
	var order []string
	var debit, credit money.Amount
	var current *xafTransaction
	var currentKey string
	for _, e := range b.entries {
		id, desc, kind := journalOf(e.DocumentType)
		j := journals[id]
		if j == nil {
			j = &xafJournal{ID: id, Desc: desc, Type: kind}
			journals[id] = j
			order = append(order, id)
		}
		key := e.DocumentType + e.DocumentID.String()
		if current == nil || key != currentKey {
			j.Transactions = append(j.Transactions, xafTransaction{
				Nr: e.DocumentID.String(), Desc: cmpOr(e.Description, e.DocumentType), Period: int(e.Date.Month()), Date: e.Date.Format(time.DateOnly),
			})
			current, currentKey = &j.Transactions[len(j.Transactions)-1], key
		}
		amnt, tp := side(e.Amount)
		line := xafTrLine{
			Nr: e.ID.String(), AccID: b.index[e.LedgerAccountID].Code, DocRef: e.DocumentID.String(), EffDate: e.Date.Format(time.DateOnly),
			Desc: cmpOr(e.Description, e.DocumentType), Amount: amnt, Type: tp,
		}
		if e.ContactID != nil {
			line.CustSupID = e.ContactID.String()
		}
		current.Lines = append(current.Lines, line)
		current.debit += e.Debit()
		debit += e.Debit()
		credit += e.Credit()
		out.Company.Transactions.LinesCount++
	}
	for _, id := range order {
		j := journals[id]
		for i := range j.Transactions {
			j.Transactions[i].Amount, j.Transactions[i].Type = decimal(j.Transactions[i].debit), "D"
		}
		out.Company.Transactions.Journals = append(out.Company.Transactions.Journals, *j)
	}
	out.Company.Transactions.TotalDebit, out.Company.Transactions.TotalCredit = decimal(debit), decimal(credit)
	var buf bytes.Buffer
	buf.WriteString(xml.Header)
	enc := xml.NewEncoder(&buf)
	enc.Indent("", "  ")
	if err := enc.Encode(out); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// openingLines are the balances at the start of the year: the balance
// sheet categories', and the results of earlier years on the first equity
// category, as no years are closed.
func openingLines(b book) []Sum {
	lines := append([]Sum{}, b.opening...)
	if b.results == 0 {
		return lines
	}
	for _, a := range b.accounts {
		if a.Type == Equity {
			for i := range lines {
				if lines[i].LedgerAccountID == a.ID {
					lines[i].Amount += b.results
					return lines
				}
			}
			return append(lines, Sum{LedgerAccountID: a.ID, Amount: b.results})
		}
	}
	return lines
}

func cmpOr(v, fallback string) string {
	if strings.TrimSpace(v) == "" {
		return fallback
	}
	return v
}

type brugstaatFile struct {
	XMLName  xml.Name        `xml:"brugstaat"`
	Name     string          `xml:"administratie"`
	Year     int             `xml:"boekjaar"`
	Scheme   string          `xml:"rgsVersie"`
	Created  string          `xml:"aangemaakt"`
	Accounts []brugstaatLine `xml:"grootboekrekening"`
}

type brugstaatLine struct {
	Code      string `xml:"nummer"`
	Name      string `xml:"omschrijving"`
	Type      string `xml:"soort"`
	RGSCode   string `xml:"rgsCode"`
	Reference string `xml:"rgsReferentienummer"`
	Opening   string `xml:"beginsaldo"`
	Debit     string `xml:"debet"`
	Credit    string `xml:"credit"`
	Closing   string `xml:"eindsaldo"`
}

// brugstaat writes the year's RGS brugstaat: per category with a balance or
// bookings its RGS code, its balance at the start, its debits and credits
// and its balance at the end (debit positive). Profit and loss categories
// start the year at zero.
func brugstaat(b book, scheme *taxonomy.Scheme, now time.Time) ([]byte, error) {
	type figures struct{ opening, debit, credit money.Amount }
	by := map[resource.ID]*figures{}
	get := func(id resource.ID) *figures {
		if by[id] == nil {
			by[id] = &figures{}
		}
		return by[id]
	}
	for _, o := range b.opening {
		get(o.LedgerAccountID).opening = o.Amount
	}
	for _, e := range b.entries {
		f := get(e.LedgerAccountID)
		f.debit += e.Debit()
		f.credit += e.Credit()
	}
	out := brugstaatFile{Name: b.admin.Name, Year: b.year, Scheme: scheme.Version, Created: now.Format(time.DateOnly)}
	for _, a := range b.accounts {
		f, ok := by[a.ID]
		if !ok {
			continue
		}
		item, _ := scheme.Item(a.Taxonomy)
		kind := "W"
		if OnBalanceSheet(a.Type) {
			kind = "B"
		}
		out.Accounts = append(out.Accounts, brugstaatLine{
			Code: a.Code, Name: a.Name, Type: kind, RGSCode: a.Taxonomy, Reference: item.Reference,
			Opening: decimal(f.opening), Debit: decimal(f.debit), Credit: decimal(f.credit), Closing: decimal(f.opening + f.debit - f.credit),
		})
	}
	var buf bytes.Buffer
	buf.WriteString(xml.Header)
	enc := xml.NewEncoder(&buf)
	enc.Indent("", "  ")
	if err := enc.Encode(out); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// cell is a spreadsheet cell: text, or a number when Number is set.
type cell struct {
	Text   string
	Number *money.Amount
}

func text(v string) cell         { return cell{Text: v} }
func amount(a money.Amount) cell { return cell{Number: &a} }

// ledgerCards writes per category with a balance or bookings its balance
// at the start of the year, every booking and its balance at the end.
func ledgerCards(b book, contacts []Contact) ([]byte, error) {
	names := map[resource.ID]string{}
	for _, c := range contacts {
		names[c.ID] = c.Name
	}
	opening := map[resource.ID]money.Amount{}
	for _, o := range b.opening {
		opening[o.LedgerAccountID] = o.Amount
	}
	byAccount := map[resource.ID][]journal.Entry{}
	for _, e := range b.entries {
		byAccount[e.LedgerAccountID] = append(byAccount[e.LedgerAccountID], e)
	}
	rows := [][]cell{{text("Grootboekrekening"), text("Omschrijving"), text("Datum"), text("Boekstuk"), text("Soort"), text("Relatie"), text("Debet"), text("Credit"), text("Saldo")}}
	for _, a := range b.accounts {
		entries, start := byAccount[a.ID], opening[a.ID]
		if len(entries) == 0 && start == 0 {
			continue
		}
		label := strings.TrimSpace(a.Code + " " + a.Name)
		balance := start
		rows = append(rows, []cell{text(label), text("Beginsaldo"), text(b.period.From.Format(time.DateOnly)), text(""), text(""), text(""), text(""), text(""), amount(balance)})
		for _, e := range entries {
			balance += e.Amount
			contact := ""
			if e.ContactID != nil {
				contact = names[*e.ContactID]
			}
			debit, credit := text(""), text("")
			if e.Amount >= 0 {
				debit = amount(e.Amount)
			} else {
				credit = amount(-e.Amount)
			}
			rows = append(rows, []cell{text(label), text(e.Description), text(e.Date.Format(time.DateOnly)), text(e.DocumentID.String()), text(e.DocumentType), text(contact), debit, credit, amount(balance)})
		}
		rows = append(rows, []cell{text(label), text("Eindsaldo"), text(b.period.To.Format(time.DateOnly)), text(""), text(""), text(""), text(""), text(""), amount(balance)})
	}
	return workbook("Grootboekkaarten", rows)
}

// column is a spreadsheet column's letters: 0 is A, 26 is AA.
func column(i int) string {
	s := ""
	for i++; i > 0; i = (i - 1) / 26 {
		s = string(rune('A'+(i-1)%26)) + s
	}
	return s
}

func escape(s string) string {
	var buf bytes.Buffer
	_ = xml.EscapeText(&buf, []byte(s))
	return buf.String()
}

// workbook writes one sheet of rows as an Office Open XML workbook.
func workbook(sheet string, rows [][]cell) ([]byte, error) {
	var data bytes.Buffer
	data.WriteString(xml.Header + `<worksheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main"><sheetData>`)
	for r, row := range rows {
		fmt.Fprintf(&data, `<row r="%d">`, r+1)
		for c, v := range row {
			ref := fmt.Sprintf("%s%d", column(c), r+1)
			if v.Number != nil {
				fmt.Fprintf(&data, `<c r="%s"><v>%s</v></c>`, ref, decimal(*v.Number))
			} else if v.Text != "" {
				fmt.Fprintf(&data, `<c r="%s" t="inlineStr"><is><t xml:space="preserve">%s</t></is></c>`, ref, escape(v.Text))
			}
		}
		data.WriteString(`</row>`)
	}
	data.WriteString(`</sheetData></worksheet>`)
	parts := []struct{ name, body string }{
		{"[Content_Types].xml", xml.Header + `<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types">` +
			`<Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/>` +
			`<Default Extension="xml" ContentType="application/xml"/>` +
			`<Override PartName="/xl/workbook.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.sheet.main+xml"/>` +
			`<Override PartName="/xl/worksheets/sheet1.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.worksheet+xml"/>` +
			`</Types>`},
		{"_rels/.rels", xml.Header + `<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">` +
			`<Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="xl/workbook.xml"/>` +
			`</Relationships>`},
		{"xl/workbook.xml", xml.Header + `<workbook xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships">` +
			`<sheets><sheet name="` + escape(sheet) + `" sheetId="1" r:id="rId1"/></sheets></workbook>`},
		{"xl/_rels/workbook.xml.rels", xml.Header + `<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">` +
			`<Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/worksheet" Target="worksheets/sheet1.xml"/>` +
			`</Relationships>`},
		{"xl/worksheets/sheet1.xml", data.String()},
	}
	var out bytes.Buffer
	z := zip.NewWriter(&out)
	for _, p := range parts {
		w, err := z.Create(p.name)
		if err != nil {
			return nil, err
		}
		if _, err := w.Write([]byte(p.body)); err != nil {
			return nil, err
		}
	}
	if err := z.Close(); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}
