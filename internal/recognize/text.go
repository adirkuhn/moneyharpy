package recognize

import (
	"regexp"
	"strings"
	"time"

	"github.com/adirkuhn/moneyharpy/internal/money"
)

// ParseText reads a document from its text: the text layer of a digital PDF
// or what OCR read from a scan. It sets Source to SourceText; a caller that
// ran OCR sets SourceOCR. Values the rules are not sure of are Uncertain;
// today bounds plausible dates.
//
// Lines are kept only when their totals add up to the document's total incl.
// or excl. VAT; otherwise none are returned and the integrator books one line
// per VAT rate from the VAT summary.
func ParseText(text string, today time.Time) Extraction {
	lines := prepare(text)
	buyer := buyerLines(lines)
	e := Extraction{Source: SourceText}

	e.Supplier, e.SupplierFound = readSupplier(lines, buyer)
	e.Reference = readReference(lines)
	e.Date, e.DueDate = readDates(lines, today)
	e.Currency = readCurrency(lines)
	e.PaymentReference = readPaymentReference(lines)

	kinds := classify(lines)
	t := readTotals(lines, kinds)
	rows := readTable(lines, kinds)
	if len(rows) == 0 {
		rows = readLines(lines, kinds)
	}
	kind := readKind(lines, e.DueDate.Found, t.paid)
	e.Kind = kind
	reconcile(&e, t, rows, kind.V, lines)
	if isCreditNote(lines) {
		negate(&e)
	}
	return e
}

var ocrNumber = regexp.MustCompile(`[0-9OoIl|][0-9OoIl|.,:\-/]*[0-9OoIl|]`)
var ocrWord = regexp.MustCompile(`\b[A-Za-z][A-Za-z01]*[A-Za-z]\b|\b[A-Za-z]{2,}[a-z]1\b`)

// prepare splits text into lines and repairs common OCR confusions: O for 0
// and l or I for 1 inside numbers, 0 and 1 inside words.
func prepare(text string) []string {
	text = strings.NewReplacer("\r\n", "\n", "\r", "\n", "\t", "    ", " ", " ", " ", " ",
		" ", " ", " ", " ", "–", "-", "—", "-", "−", "-", "’", "'").Replace(text)
	lines := strings.Split(text, "\n")
	for i, l := range lines {
		lines[i] = strings.TrimRight(fixOCR(l), " ")
	}
	return lines
}

func fixOCR(l string) string {
	locs := ocrNumber.FindAllStringIndex(l, -1)
	if len(locs) > 0 {
		b := []byte(l)
		for _, loc := range locs {
			m := l[loc[0]:loc[1]]
			if !strings.ContainsAny(m, "0123456789") {
				continue
			}
			if !isDigit(m[0]) && loc[0] > 0 && isLetter(l[loc[0]-1]) {
				continue
			}
			if loc[1] < len(l) && isLetter(l[loc[1]]) && !isDigit(m[len(m)-1]) {
				continue
			}
			for k := loc[0]; k < loc[1]; k++ {
				switch b[k] {
				case 'O', 'o':
					b[k] = '0'
				case 'I', 'l', '|':
					b[k] = '1'
				}
			}
		}
		l = string(b)
	}
	return ocrWord.ReplaceAllStringFunc(l, func(w string) string {
		if !strings.ContainsAny(w, "01") {
			return w
		}
		upper, lower := 0, 0
		for i := range len(w) {
			switch {
			case w[i] >= 'A' && w[i] <= 'Z':
				upper++
			case w[i] >= 'a' && w[i] <= 'z':
				lower++
			}
		}
		if upper+lower < 4 {
			return w
		}
		zero, one := "o", "l"
		if upper > lower {
			zero, one = "O", "I"
		}
		return strings.NewReplacer("0", zero, "1", one).Replace(w)
	})
}

func isLetter(c byte) bool { return c >= 'A' && c <= 'Z' || c >= 'a' && c <= 'z' }

// --- line classification ---

type lineKind int

const (
	lineOther lineKind = iota
	lineIncl
	lineExcl
	lineVAT
	linePaid
)

var (
	vatWord      = regexp.MustCompile(`(?i)\b(?:btw|b\.t\.w\.?|vat|mwst|ust|umsatzsteuer|mehrwertsteuer|iva|tva|tax|moms|igic|impostos?)\b`)
	reverseWords = regexp.MustCompile(`(?i)verlegd|reverse[ -]charge|autoliquidation|steuerschuldnerschaft|inversi[oó]n del sujeto pasivo|autoliquida[cç][aã]o|reverse charged`)
	// exemptEUWords mark a supply exempt in the seller's country because the
	// buyer accounts for the VAT: an intra-community supply.
	exemptEUWords = regexp.MustCompile(`(?i)verlegd|reverse[ -]charge|autoliquida|steuerschuldnerschaft|inversi[oó]n del sujeto pasivo|intra-?communautaire|intra-?community|intracomunit[aá]ri|innergemeinschaftlich|isento artigo 14|artigo 14\.?º? do civa|art(?:\.|icle|ikel)? ?138|art(?:\.|igo)? ?25 (?:de la )?ley|exenta art|vat exempt intra`)
	exclWords     = regexp.MustCompile(`(?i)\b(?:subtotaal|subtotal|sub-total|sub total|sub\. total|incid[eê]ncia|excl\.?|exclusief|excluding|exkl\.?|ex\.? btw|netto|nettobetrag|net amount|net total|total net|total ht|montant ht|hors taxes|base imponible|zwischensumme|sous-total|il[ií]quido|sem iva|total excl)`)
	inclMarker    = regexp.MustCompile(`(?i)\b(?:incl\.?|inclusief|including|inkl\.?|ttc)\b`)
	inclStrong    = regexp.MustCompile(`(?i)totaal incl|total incl|totaalbedrag|totaal te betalen|te betalen|amount due|total due|balance due|grand total|gesamtbetrag|rechnungsbetrag|zu zahlen|zahlbetrag|endbetrag|total a pagar|total ttc|net [aà] payer|importe total|total factura|eindtotaal|te voldoen|invoice total|total amount|totale`)
	inclWeak      = regexp.MustCompile(`(?i)\b(?:totaal|total|summe|gesamt|totale)\b`)
	paidWords     = regexp.MustCompile(`(?i)\b(?:pin|pinnen|pinbetaling|betaald|contant|cash|maestro|v ?pay|visa|mastercard|amex|card|kaart|contactloos|paid|bezahlt|ec-karte|girocard)\b`)
	itemHeader    = regexp.MustCompile(`(?i)\b(?:omschrijving|description|artikel|art\.?nr|item|items|bezeichnung|beschreibung|descri[cç][aã]o|descripci[oó]n|d[ée]signation|product|produkt|aantal|qty|quantity|menge|qtd|qté)\b`)
	htWord        = regexp.MustCompile(`\bHT\b`)
	vatTableHead  = regexp.MustCompile(`(?i)\b(?:btw|vat|mwst|iva|tva|tarief|rate)\b`)
)

// classify tells what each line with amounts is: a total, a VAT line, a
// payment, or something else (an item line).
func classify(lines []string) []lineKind {
	kinds := make([]lineKind, len(lines))
	vatTableUntil := -1
	for i, l := range lines {
		amounts := findAmounts(l)
		if len(amounts) == 0 {
			if vatTableHead.MatchString(l) && !itemHeader.MatchString(l) {
				vatTableUntil = i + 4
			}
			continue
		}
		hasVAT, hasRate := vatWord.MatchString(l), findRate(l) != ""
		incl, excl := inclStrong.MatchString(l) || inclWeak.MatchString(l), exclWords.MatchString(l)
		switch {
		case hasVAT && hasRate && (incl || excl) && len(amounts) == 1 && (inclMarker.MatchString(l) || excl):
			if excl {
				kinds[i] = lineExcl
			} else {
				kinds[i] = lineIncl
			}
		case hasVAT && hasRate:
			kinds[i] = lineVAT
		case excl:
			kinds[i] = lineExcl
		case incl && hasVAT && !inclMarker.MatchString(l):
			kinds[i] = lineVAT
		case incl:
			kinds[i] = lineIncl
		case hasVAT || reverseWords.MatchString(l):
			kinds[i] = lineVAT
		case paidWords.MatchString(l):
			kinds[i] = linePaid
		case i <= vatTableUntil && hasRate && len(amounts) >= 2:
			if _, _, ok := taxPair(findRate(l), amountValues(amounts)); ok {
				kinds[i] = lineVAT
			}
		}
	}
	return kinds
}

func amountValues(hits []amountHit) []money.Amount {
	out := make([]money.Amount, len(hits))
	for i, h := range hits {
		out[i] = h.v
	}
	return out
}

// --- totals ---

type candidate struct {
	v     money.Amount
	score int
}

type vatEntry struct {
	rate       string
	taxable    money.Amount
	tax        money.Amount
	hasTaxable bool
	checked    bool
}

type totals struct {
	incl, excl []candidate
	vat        []vatEntry
	paid       []money.Amount
}

func readTotals(lines []string, kinds []lineKind) totals {
	var t totals
	for i, l := range lines {
		amounts := findAmounts(l)
		if len(amounts) == 0 {
			continue
		}
		last := amounts[len(amounts)-1].v
		switch kinds[i] {
		case lineIncl:
			score := 2
			if inclStrong.MatchString(l) || inclMarker.MatchString(l) {
				score = 3
			}
			t.incl = append(t.incl, candidate{last, score})
		case lineExcl:
			t.excl = append(t.excl, candidate{last, 2})
		case linePaid:
			t.paid = append(t.paid, last)
		case lineVAT:
			t.vat = addVAT(t.vat, vatLine(l, amountValues(amounts)))
		}
	}
	return t
}

func vatLine(l string, amounts []money.Amount) vatEntry {
	rate := findRate(l)
	if rate == "" && reverseWords.MatchString(l) {
		rate = "0"
	}
	if rate != "" && len(amounts) >= 2 {
		if taxable, tax, ok := taxPair(rate, amounts); ok {
			return vatEntry{rate: rate, taxable: taxable, tax: tax, hasTaxable: true, checked: true}
		}
		return vatEntry{rate: rate, taxable: amounts[0], tax: amounts[len(amounts)-1], hasTaxable: true}
	}
	a := amounts[len(amounts)-1]
	if rate == "0" && a != 0 {
		return vatEntry{rate: rate, taxable: a, hasTaxable: true, checked: true}
	}
	return vatEntry{rate: rate, tax: a}
}

// addVAT keeps one entry per rate, the one that says the most.
func addVAT(list []vatEntry, v vatEntry) []vatEntry {
	for i, old := range list {
		if old.rate == v.rate {
			if !old.hasTaxable && v.hasTaxable {
				list[i] = v
			}
			return list
		}
	}
	return append(list, v)
}

// best is the strongest candidate, the largest of equally strong ones.
func best(cands []candidate) (candidate, bool) {
	var b candidate
	found := false
	for _, c := range cands {
		if !found || c.score > b.score || c.score == b.score && abs(c.v) > abs(b.v) {
			b, found = c, true
		}
	}
	return b, found
}

// reconcile picks the totals that agree with each other (excl + VAT = incl)
// and marks them certain; a lone best guess stays uncertain.
func reconcile(e *Extraction, t totals, rows []Line, kind Kind, lines []string) {
	vatSum, taxableSum, allTaxable := money.Amount(0), money.Amount(0), len(t.vat) > 0
	for _, v := range t.vat {
		vatSum += v.tax
		taxableSum += v.taxable
		allTaxable = allTaxable && v.hasTaxable
	}
	single := len(t.vat) == 1 && !t.vat[0].hasTaxable
	var incl, excl Value[money.Amount]
	sorted := sortCandidates(t.incl)

	if len(t.vat) > 0 {
	search:
		for _, ic := range sorted {
			for _, ec := range t.excl {
				if near(ec.v+vatSum, ic.v, 2) && ec.v != ic.v || near(ec.v+vatSum, ic.v, 2) && vatSum == 0 {
					incl, excl = sure(ic.v), sure(ec.v)
					if single {
						t.vat[0].taxable, t.vat[0].hasTaxable = ec.v, true
					}
					break search
				}
			}
			if allTaxable && near(taxableSum+vatSum, ic.v, 2) {
				incl, excl = sure(ic.v), sure(taxableSum)
				break
			}
			if single && t.vat[0].rate != "" && taxFits(ic.v-vatSum, vatSum, t.vat[0].rate) {
				incl, excl = sure(ic.v), sure(ic.v-vatSum)
				t.vat[0].taxable, t.vat[0].hasTaxable = ic.v-vatSum, true
				break
			}
		}
		if !incl.Found && len(t.incl) == 0 {
			for _, ec := range t.excl {
				if allTaxable && near(ec.v, taxableSum, 2) || single && taxFits(ec.v, vatSum, t.vat[0].rate) {
					incl, excl = sure(ec.v+vatSum), sure(ec.v)
					if single {
						t.vat[0].taxable, t.vat[0].hasTaxable = ec.v, true
					}
					break
				}
			}
		}
	} else if len(t.excl) > 0 && len(sorted) > 0 {
	guess:
		for _, ic := range sorted {
			for _, ec := range t.excl {
				if ic.v == ec.v || abs(ic.v) < abs(ec.v) {
					continue
				}
				for _, r := range commonRates {
					if r != "0" && taxFits(ec.v, ic.v-ec.v, r) {
						incl, excl = sure(ic.v), sure(ec.v)
						t.vat = []vatEntry{{rate: r, taxable: ec.v, tax: ic.v - ec.v, hasTaxable: true, checked: true}}
						break guess
					}
				}
			}
		}
	}

	if !incl.Found {
		if c, ok := best(t.incl); ok {
			incl = unsure(c.v)
			for _, p := range t.paid {
				if p == c.v {
					incl.Confidence = Certain
				}
			}
		}
		if c, ok := best(t.excl); ok && (!incl.Found || abs(c.v) < abs(incl.V) || vatSum == 0) {
			excl = unsure(c.v)
		} else if incl.Found && len(t.vat) > 0 {
			excl = unsure(incl.V - vatSum)
		}
	}
	if !incl.Found && len(t.paid) > 0 {
		incl = unsure(t.paid[0])
	}

	// Lines: keep them only when they add up to a total.
	var sum money.Amount
	for _, r := range rows {
		sum += r.Total.V
	}
	pricesIncl := Value[bool]{V: kind == KindReceipt, Found: true, Confidence: Uncertain}
	switch {
	case len(rows) == 0:
	case incl.Found && excl.Found && incl.V == excl.V && near(sum, incl.V, 1):
		e.Lines = rows
		pricesIncl.V = kind == KindReceipt
	case incl.Found && near(sum, incl.V, 1):
		e.Lines = rows
		pricesIncl = sure(true)
		incl.Confidence = Certain // the lines confirm it
	case excl.Found && near(sum, excl.V, 1):
		e.Lines = rows
		pricesIncl = sure(false)
	case !incl.Found && !excl.Found:
		incl = unsure(sum)
		e.Lines = rows
	}
	if !e.PricesInclTax.Found {
		e.PricesInclTax = pricesIncl
	}
	if pricesIncl.Confidence != Certain {
		for _, l := range lines {
			if itemHeader.MatchString(l) && len(findAmounts(l)) == 0 {
				switch {
				case inclMarker.MatchString(l):
					e.PricesInclTax = sure(true)
				case exclWords.MatchString(l) || htWord.MatchString(l):
					e.PricesInclTax = sure(false)
				}
				break
			}
		}
	}

	// VAT entries: fill what the totals tell.
	for i := range t.vat {
		v := &t.vat[i]
		if !v.hasTaxable && len(t.vat) == 1 && excl.Found {
			v.taxable, v.hasTaxable = excl.V, true
		}
		if v.rate == "" && v.hasTaxable && v.taxable != 0 {
			for _, r := range commonRates {
				if taxFits(v.taxable, v.tax, r) {
					v.rate = r
					break
				}
			}
		}
		if v.rate == "" {
			continue
		}
		e.VAT = append(e.VAT, VAT{Rate: v.rate, Taxable: v.taxable, Tax: v.tax})
	}
	if len(e.VAT) == 0 && incl.Found && excl.Found && incl.V == excl.V && incl.Confidence == Certain {
		e.VAT = []VAT{{Rate: "0", Taxable: excl.V}}
	}
	if mentions(lines, exemptEUWords) {
		for i := range e.VAT {
			e.VAT[i].ReverseCharge = e.VAT[i].Tax == 0
		}
	}
	e.TotalInclTax, e.TotalExclTax = incl, excl
}

func sortCandidates(c []candidate) []candidate {
	out := append([]candidate(nil), c...)
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && out[j].score > out[j-1].score; j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}
	return out
}

var creditWords = regexp.MustCompile(`(?i)credit ?nota|credit note|creditfactuur|gutschrift|nota de cr[eé]dito|\bavoir\b|factura rectificativa|rechnungskorrektur|stornorechnung`)

func isCreditNote(lines []string) bool {
	for _, l := range lines {
		if creditWords.MatchString(l) {
			return true
		}
	}
	return false
}

// negate makes the amounts of a credit note negative, as Moneybird books them.
func negate(e *Extraction) {
	if !e.TotalInclTax.Found || e.TotalInclTax.V <= 0 {
		return
	}
	e.TotalInclTax.V = -e.TotalInclTax.V
	e.TotalExclTax.V = -e.TotalExclTax.V
	for i := range e.VAT {
		e.VAT[i].Taxable, e.VAT[i].Tax = -e.VAT[i].Taxable, -e.VAT[i].Tax
	}
	for i := range e.Lines {
		e.Lines[i].Price.V = -e.Lines[i].Price.V
		e.Lines[i].Total.V = -e.Lines[i].Total.V
	}
}

// --- kind ---

var (
	invoiceWords = regexp.MustCompile(`(?i)factuurnummer|factuurdatum|factuur|invoice number|invoice date|invoice|rechnungsnummer|rechnung|fatura|facture|factura|credit ?nota|credit note|gutschrift|vervaldatum|due date|betalingstermijn|payment terms|zahlbar|vencimento|[ée]ch[ée]ance|vencimiento|encomenda|condi[cç][aã]o de pagamento|nota de cr[eé]dito`)
	receiptWords = regexp.MustCompile(`(?i)kassabon|kassenbon|\bkassa\b|bonnummer|\bbonnr|\bbon\b|\bpin\b|pinnen|pinbetaling|contactloos|terminal|transactie|autorisatiecode|wisselgeld|kassier|merchant|\btafel\b|\breceipt\b|\bticket\b|\bbeleg\b|\brecibo\b|tal[aã]o|btw-overzicht|\btarief\b|bedankt|tot ziens|thank you for your (?:visit|purchase)|ticket de caisse`)
)

// readKind tells a purchase invoice from a till receipt by the words used.
func readKind(lines []string, hasDue bool, paid []money.Amount) Value[Kind] {
	inv, rec := map[string]bool{}, map[string]bool{}
	for _, l := range lines {
		for _, m := range invoiceWords.FindAllString(l, -1) {
			inv[strings.ToLower(m)] = true
		}
		for _, m := range receiptWords.FindAllString(l, -1) {
			rec[strings.ToLower(m)] = true
		}
	}
	invScore, recScore := len(inv), len(rec)
	if hasDue {
		invScore += 2
	}
	if len(paid) > 0 {
		recScore++
	}
	var k Kind
	switch {
	case invScore == 0 && recScore == 0:
		return Value[Kind]{}
	case invScore >= recScore:
		k = KindPurchaseInvoice
	default:
		k = KindReceipt
	}
	if diff := invScore - recScore; diff >= 2 || diff <= -2 {
		return sure(k)
	}
	return unsure(k)
}

// --- reference ---

var (
	refLabels  = regexp.MustCompile(`(?i)factuurnummer|factuurnr\.?|factuur ?nr\.?|factuur ?nummer|creditnotanummer|creditnota ?nr\.?|creditnota ?nummer|invoice number|invoice no\.?|invoice nr\.?|invoice #|invoice id|inv\.? ?no\.?|rechnungsnummer|rechnungs-?nr\.?|rechnung nr\.?|gutschriftsnummer|n\.?[ºo°] ?(?:da )?fatura|fatura n\.?[ºo°]|n[uú]mero de factura|factura n\.?[ºo°]|n[uú]mero da fatura|num[ée]ro de facture|facture n[°ºo]\.?|bonnummer|bonnr\.?|bon ?nr\.?|receipt number|receipt no\.?|receipt #|credit note number|credit note no\.?|documentnummer|document number|kassabon ?nr\.?|confirma[cç][aã]o de encomenda|nota de encomenda|encomenda n\.?[ºo°]|fatura-recibo(?: n\.?[ºo°])?|nota de cr[eé]dito(?: n\.?[ºo°])?`)
	refGeneric = regexp.MustCompile(`(?i)\b(?:factuur|invoice|rechnung|fatura|facture|factura|creditnota|bon|encomenda)\b`)
	refToken   = regexp.MustCompile(`^\s*[:#.]?\s*(?:(?i:nr|no|n[°º])\.?)?\s*[:#.]?\s*((?:[A-Z]{1,5} )?[A-Z0-9][A-Za-z0-9\-/._]*)`)
	tokenRe    = regexp.MustCompile(`\S+`)
)

// readReference reads the invoice or receipt number after its label, on the
// same line or in the same column on the next line.
func readReference(lines []string) Value[string] {
	var generic Value[string]
	for i, l := range lines {
		for _, m := range refLabels.FindAllStringIndex(l, -1) {
			if tok := refAfter(l[m[1]:]); tok != "" {
				return sure(tok)
			}
			if j := nextNonEmpty(lines, i); j >= 0 {
				for _, t := range tokenRe.FindAllStringIndex(lines[j], -1) {
					// Under the label: starting near it, or within it when
					// the label and value are right-aligned.
					if absInt(t[0]-m[0]) <= 3 || t[0] >= m[0] && t[0] < m[1] {
						if tok := refAfter(lines[j][t[0]:]); tok != "" {
							return sure(tok)
						}
					}
				}
			}
		}
		if !generic.Found {
			for _, m := range refGeneric.FindAllStringIndex(l, -1) {
				if tok := refAfter(l[m[1]:]); tok != "" {
					generic = unsure(tok)
					break
				}
			}
		}
	}
	return generic
}

func refAfter(s string) string {
	m := refToken.FindStringSubmatch(s)
	if m == nil {
		return ""
	}
	tok := strings.TrimRight(m[1], ".,;:/-")
	if !strings.ContainsAny(tok, "0123456789") || len(findDates(tok)) > 0 || len(tok) > 40 {
		return ""
	}
	return tok
}

// --- payment reference and currency ---

var (
	kenmerkLabel = regexp.MustCompile(`(?i)betalingskenmerk|betaalkenmerk|kenmerk|payment reference|betalingsreferentie|betaalreferentie|o\.v\.v\.|onder vermelding`)
	kenmerkRe    = regexp.MustCompile(`\b(\d{4}[ .]?\d{4}[ .]?\d{4}[ .]?\d{4})\b`)
	rfRe         = regexp.MustCompile(`\bRF\d{2}(?:\s?[A-Z0-9]{1,4}){1,6}`)
)

// readPaymentReference reads a Dutch betalingskenmerk or an RF creditor
// reference; it is certain when its check digits are right.
func readPaymentReference(lines []string) Value[string] {
	for _, l := range lines {
		if !kenmerkLabel.MatchString(l) {
			continue
		}
		if m := kenmerkRe.FindStringSubmatch(l); m != nil {
			n := stripSeparators(m[1])
			if ValidBetalingskenmerk(n) {
				return sure(n)
			}
			return unsure(n)
		}
	}
	for _, l := range lines {
		for _, m := range rfRe.FindAllString(l, -1) {
			n := strings.ToUpper(stripSeparators(m))
			for k := len(n); k >= 5; k-- {
				if ValidRF(n[:k]) {
					return sure(n[:k])
				}
			}
		}
	}
	return Value[string]{}
}

var currencies = []struct {
	code string
	re   *regexp.Regexp
}{
	{"EUR", regexp.MustCompile(`€|\bEUR\b`)},
	{"USD", regexp.MustCompile(`\$|\bUSD\b`)},
	{"GBP", regexp.MustCompile(`£|\bGBP\b`)},
	{"CHF", regexp.MustCompile(`\bCHF\b`)},
	{"SEK", regexp.MustCompile(`\bSEK\b`)},
	{"DKK", regexp.MustCompile(`\bDKK\b`)},
	{"NOK", regexp.MustCompile(`\bNOK\b`)},
	{"PLN", regexp.MustCompile(`\bPLN\b|\bzł`)},
	{"CZK", regexp.MustCompile(`\bCZK\b|\bKč`)},
	{"HUF", regexp.MustCompile(`\bHUF\b`)},
	{"JPY", regexp.MustCompile(`\bJPY\b|¥`)},
	{"CAD", regexp.MustCompile(`\bCAD\b`)},
	{"AUD", regexp.MustCompile(`\bAUD\b`)},
}

// readCurrency takes the currency mentioned most; without any it is EUR,
// uncertain.
func readCurrency(lines []string) Value[string] {
	text := strings.Join(lines, "\n")
	bestCode, bestCount := "", 0
	for _, c := range currencies {
		if n := len(c.re.FindAllStringIndex(text, -1)); n > bestCount {
			bestCode, bestCount = c.code, n
		}
	}
	if bestCode == "" {
		return unsure("EUR")
	}
	return sure(bestCode)
}

// mentions tells whether any line matches re.
func mentions(lines []string, re *regexp.Regexp) bool {
	for _, l := range lines {
		if re.MatchString(l) {
			return true
		}
	}
	return false
}
