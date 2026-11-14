package recognize

import (
	"regexp"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"
)

var (
	// buyerLabel starts a block with the buyer's details ("Factuur aan:").
	buyerLabel = regexp.MustCompile(`(?i)^\s*(?:factuur aan|aan|bill(?:ed)? to|invoice to|sold to|ship to|deliver to|delivery address|afleveradres|klant|customer|cliente|client|kunde|rechnungsempf[äa]nger|lieferadresse|factur[ée] [àa]|uw gegevens|t\.?a\.?v\.?|fao|attn|for the attention of)\b\s*:?`)
	// buyerWords mark a single line about the buyer, or the buyer's account.
	buyerWords = regexp.MustCompile(`(?i)\b(?:uw|your|ihre?|cliente|client|customer|klant|kunde|buyer|koper|afnemer|verlegd naar|afgeschreven|incasso|ge[ïi]ncasseerd|debited|abgebucht|lastschrift|machtiging|mandate)\b`)
	vatLabel   = regexp.MustCompile(`(?i)\b(?:btw|b\.t\.w\.?|vat|tva|iva|ust|ust-?id(?:nr)?\.?|nif|nipc|mwst|tax|moms|p\.?\s?iva|cif|nr\.? tva)\b`)
	vatStart   = regexp.MustCompile(`\b(?:AT|BE|BG|CY|CZ|DE|DK|EE|EL|ES|FI|FR|HR|HU|IE|IT|LT|LU|LV|MT|NL|PL|PT|RO|SE|SI|SK|GB|XI|CHE)`)
	kvkPattern = regexp.MustCompile(`(?i)(?:\bkvk|\bk\.v\.k\.?|kamer van koophandel|\bcoc\b|chamber of commerce|handelsregister)[^0-9\n]{0,20}(\d{8}|\d{2}[ .]\d{2}[ .]\d{2}[ .]\d{2}|\d{4}[ .]\d{4})\b`)
	ibanLabel  = regexp.MustCompile(`(?i)iban|rekening|bank|account|konto|\bnib\b|over te maken|overmaken|t\.n\.v`)
	emailRe    = regexp.MustCompile(`[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\.[A-Za-z]{2,}`)
	wwwRe      = regexp.MustCompile(`(?i)\b(www\.[a-z0-9-]+(?:\.[a-z0-9-]+)+)`)
	webLabelRe = regexp.MustCompile(`(?i)\b(?:website|web|internet|site)\s*:\s*(?:https?://)?([a-z0-9-]+(?:\.[a-z0-9-]+)+)`)
	phoneRe    = regexp.MustCompile(`(?i)(?:\btel(?:efoon|efon|ephone|\.)?|\bphone|\bt\b\.?|\btelefone|\btél\.?|\btel[ée]fono|\bmob(?:iel|ile)?)\s*[:.]?\s*(\+?\d[\d\s()./-]{6,}\d)`)
	legalForm  = regexp.MustCompile(`(?i)^(.*\p{L}.*?)[\s,]+(b\.\s?v\.?|bv|n\.\s?v\.?|nv|v\.o\.f\.?|vof|c\.v\.|gmbh|ag|ug|kg|e\.\s?k\.|ltd\.?|limited|llc|inc\.?|plc|s\.a\.?|sa|s\.l\.?|sl|sarl|sas|lda\.?|bvba|srl|s\.r\.l\.|oy|ab|aps|a/s)$`)
	segmentSep = regexp.MustCompile(`\s*(?:\||·|•|\s{2,}|\s-\s|\t)\s*`)
	titleWords = regexp.MustCompile(`(?i)^(?:factuur|invoice|rechnung|fatura|facture|factura|creditnota|credit ?note|kassabon|kassenbon|receipt|bon|quittung|recibo|ticket|tax invoice|nota)\b`)
	labelLine  = regexp.MustCompile(`^[^:]{1,30}:\s`)
	// boilerplate is what a receipt or invoice prints about itself, never
	// the supplier's name: "KOPIE BON", "Kassabon", "Customer's receipt".
	boilerplate = regexp.MustCompile(`(?i)^\W*(?:kopie\s?bon|kopie|copy|duplica(?:at|te)|kassa\s?bon|kassenbon|bon|customer'?s?\s+(?:receipt|copy)|klant(?:en)?\s?ticket|receipt|factuur|invoice|rechnung|fatura|facture|factura|origineel|original|duplicado|triplicado|quadruplicado|\d\s?ª\s?via)\W*$`)
	streetRe    = regexp.MustCompile(`(?i)^(?:\p{L}[\p{L}\p{M}'.,\- ]*\s\d+\s?[a-z]?(?:[-/ ]?\d+[a-z]?)?(?:,.*)?|\d+[a-z]?,?\s+\p{L}.*|(?:postbus|p\.?\s?o\.?\s?box|postfach|apartado|bo[iî]te postale)\s*\d+.*)$`)
	nlZip       = regexp.MustCompile(`^(\d{4})\s?([A-Z]{2})\b\s*(.*)$`)
	ptZip       = regexp.MustCompile(`^(\d{4}-\d{3})\s+(.+)$`)
	fiveZip     = regexp.MustCompile(`^(?:[A-Z]{1,2}-)?(\d{5})\s+(\p{L}.*)$`)
	fourZip     = regexp.MustCompile(`^(?:[A-Z]{1,2}-)?(\d{4})\s+(\p{L}.*)$`)
	gbZip       = regexp.MustCompile(`^(\p{L}[\p{L} .'-]*?)\s+([A-Z]{1,2}\d[A-Z\d]?\s?\d[A-Z]{2})$`)
	cityRe      = regexp.MustCompile(`^\p{L}[\p{L}\p{M}' .-]*$`)
)

var (
	fiveDigitZipCountries = map[string]bool{"DE": true, "FR": true, "ES": true, "IT": true, "FI": true, "EE": true, "HR": true, "GR": true, "LT": true, "MY": true, "US": true}
	fourDigitZipCountries = map[string]bool{"BE": true, "AT": true, "CH": true, "DK": true, "LU": true, "NO": true, "HU": true, "SI": true, "CY": true, "BG": true}
)

// buyerLines marks lines with the buyer's details, to keep them out of the
// supplier: a block after "Factuur aan:" / "Bill to:" up to a blank line,
// and single lines that speak of "uw"/"your"/"klant".
func buyerLines(lines []string) []bool {
	out := make([]bool, len(lines))
	for i := 0; i < len(lines); i++ {
		if boilerplate.MatchString(lines[i]) {
			continue
		}
		if buyerWords.MatchString(lines[i]) {
			out[i] = true
			// A header row of columns about the buyer ("Nº de Contribuinte
			// Cliente Nº …") holds its values in the row below.
			if !strings.ContainsAny(lines[i], "0123456789") {
				if j := nextNonEmpty(lines, i); j >= 0 {
					out[j] = true
				}
			}
		}
		if !buyerLabel.MatchString(lines[i]) {
			continue
		}
		for j := i; j < len(lines) && j < i+8; j++ {
			if j > i && strings.TrimSpace(lines[j]) == "" {
				break
			}
			out[j] = true
		}
	}
	return out
}

// readSupplierFor reads who sent the document to the administration whose
// own company is self: what the lines print of self is left out first.
func readSupplierFor(lines []string, self Self) (Supplier, Confidence) {
	lines = self.scrub(lines)
	s, conf := readSupplier(lines, buyerLines(lines), self)
	if clean := self.Without(s); clean != s {
		s = clean
		if s == (Supplier{}) {
			conf = ""
		}
	}
	return s, conf
}

// readSupplier reads who sent the document, never the administration's own
// company (self).
func readSupplier(lines []string, buyer []bool, self Self) (Supplier, Confidence) {
	var s Supplier
	kvkFound := false
	vat, vatAt, vatSure := supplierVAT(lines, buyer, self)
	s.VATNumber = vat
	for i, l := range lines {
		if buyer[i] {
			continue
		}
		if s.KVK == "" {
			for _, m := range kvkPattern.FindAllStringSubmatch(l, -1) {
				if kvk := NormalizeKVK(m[1]); !self.sameKVK(kvk) {
					s.KVK, kvkFound = kvk, true
					break
				}
			}
		}
		if s.Email == "" {
			s.Email = emailRe.FindString(l)
		}
		if s.Website == "" {
			if m := wwwRe.FindStringSubmatch(l); m != nil {
				s.Website = strings.ToLower(m[1])
			} else if m := webLabelRe.FindStringSubmatch(l); m != nil {
				s.Website = strings.ToLower(m[1])
			}
		}
		if s.Phone == "" {
			if m := phoneRe.FindStringSubmatch(l); m != nil {
				s.Phone = strings.TrimSpace(m[1])
			}
		}
	}
	s.IBAN = supplierIBAN(lines, buyer, self)
	if s.VATNumber == "" {
		if v := portugueseNIF(lines, buyer, s.IBAN); v != "" {
			s.VATNumber, vatSure = v, true
		}
	}
	hint := countryOf(s.VATNumber)
	if hint == "" && s.IBAN != "" {
		hint = s.IBAN[:2]
	}
	readAddress(lines, buyer, hint, self, &s)
	if s.Country == "" {
		s.Country = hint
	}
	s.Name = supplierName(lines, buyer, vatAt, self, cmpOr(Domain(s.Email), Domain(s.Website)))

	switch {
	case vatSure || kvkFound || s.IBAN != "" && s.Name != "":
		return s, Certain
	case s != Supplier{}:
		return s, Uncertain
	}
	return s, ""
}

// supplierVAT picks the supplier's VAT number outside the buyer's lines: a
// valid one printed next to a VAT label ("BTW nummer", "VAT") first, then
// any valid one, then the first one shaped like a VAT number. It returns
// the line it is on (-1 for none), and whether it is sure: valid, and
// labelled or Dutch.
func supplierVAT(lines []string, buyer []bool, self Self) (vat string, at int, sure bool) {
	at, rank := -1, 0
	for i, l := range lines {
		if buyer[i] {
			continue
		}
		v := findVAT(l)
		if v == "" || self.sameVAT(v) {
			continue
		}
		valid, labelled := ValidVAT(v), vatLabel.MatchString(l)
		r := 1
		switch {
		case valid && labelled:
			r = 3
		case valid:
			r = 2
		}
		if r > rank {
			vat, at, rank, sure = v, i, r, valid && (labelled || strings.HasPrefix(v, "NL"))
		}
	}
	return vat, at, sure
}

func countryOf(vat string) string {
	switch {
	case vat == "":
		return ""
	case strings.HasPrefix(vat, "EL"):
		return "GR"
	case strings.HasPrefix(vat, "CHE"):
		return "CH"
	case strings.HasPrefix(vat, "XI"):
		return "GB"
	}
	return vat[:2]
}

// supplierIBAN takes the first valid IBAN not on a buyer line nor the
// administration's own, preferring one printed next to
// "IBAN"/"rekening"/"bank".
func supplierIBAN(lines []string, buyer []bool, self Self) string {
	first := ""
	for i, l := range lines {
		if buyer[i] {
			continue
		}
		ibans := slices.DeleteFunc(FindIBANs(l), self.sameIBAN)
		if len(ibans) == 0 {
			continue
		}
		if ibanLabel.MatchString(l) {
			return ibans[0]
		}
		if first == "" {
			first = ibans[0]
		}
	}
	return first
}

// segments splits a line into the parts printed with "|", "·" or wide gaps.
func segments(l string) []string {
	var out []string
	for _, p := range segmentSep.Split(strings.TrimSpace(l), -1) {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// readAddress finds the first postcode and city not in the buyer's block
// nor the administration's own, with the street before it. A postcode
// printed apart from its city ("9999 ZZ | Voorbeeldstad") is read with it.
func readAddress(lines []string, buyer []bool, hint string, self Self, s *Supplier) {
	for i, l := range lines {
		if buyer[i] {
			continue
		}
		var parts []string
		for _, seg := range segments(l) {
			for _, p := range strings.Split(seg, ",") {
				if p = strings.TrimSpace(p); p != "" {
					parts = append(parts, p)
				}
			}
		}
		for j, p := range parts {
			zip, city, country := postcode(p, hint)
			if m := nlZip.FindStringSubmatch(p); zip == "" && m != nil && m[3] == "" && j+1 < len(parts) && cityRe.MatchString(parts[j+1]) {
				zip, city, country = m[1]+" "+m[2], parts[j+1], "NL"
			}
			if zip == "" || self.sameZipcode(zip) {
				continue
			}
			s.Zipcode, s.City, s.Country = zip, city, country
			if j > 0 && streetRe.MatchString(parts[j-1]) {
				s.Address1 = parts[j-1]
				if j > 1 && isDigits(parts[j-1]) {
					s.Address1 = parts[j-2] + ", " + parts[j-1]
				}
			} else if prev := prevNonEmpty(lines, i); prev >= 0 && !buyer[prev] {
				if street := strings.TrimSpace(lines[prev]); streetRe.MatchString(street) && len(findAmounts(street)) == 0 {
					s.Address1 = street
				}
			}
			return
		}
	}
}

func postcode(p, hint string) (zip, city, country string) {
	if m := nlZip.FindStringSubmatch(p); m != nil && cityRe.MatchString(m[3]) {
		return m[1] + " " + m[2], m[3], "NL"
	}
	if m := ptZip.FindStringSubmatch(p); m != nil && cityRe.MatchString(m[2]) {
		return m[1], m[2], "PT"
	}
	if m := gbZip.FindStringSubmatch(p); m != nil && (hint == "" || hint == "GB") {
		return m[2], m[1], "GB"
	}
	if m := fiveZip.FindStringSubmatch(p); m != nil && fiveDigitZipCountries[hint] && cityRe.MatchString(m[2]) {
		return m[1], m[2], hint
	}
	if m := fourZip.FindStringSubmatch(p); m != nil && fourDigitZipCountries[hint] && cityRe.MatchString(m[2]) {
		return m[1], m[2], hint
	}
	return "", "", ""
}

func prevNonEmpty(lines []string, i int) int {
	for j := i - 1; j >= 0; j-- {
		if strings.TrimSpace(lines[j]) != "" {
			return j
		}
	}
	return -1
}

func isDigits(s string) bool {
	for i := range len(s) {
		if !isDigit(s[i]) {
			return false
		}
	}
	return s != ""
}

// vatNear is how many lines from the VAT number a company name may be
// printed to be taken for the one the VAT number belongs to.
const vatNear = 3

// maxNameWords keeps sentences that end in a company name ("… t.n.v.
// Example Shop B.V.") from being taken for one.
const maxNameWords = 8

// supplierName takes the supplier's name outside the buyer's block, never
// the administration's own (self), in this order:
//   - the legal entity a "handelsnaam van" sentence names;
//   - a company name with a legal form (B.V., GmbH, Ltd …) printed near the
//     supplier's VAT number (vatAt, -1 for none);
//   - the company a sentence says the document is from ("in favour of X",
//     "namens X"), with its legal form when a name with one is printed too;
//   - the first company name with a legal form;
//   - a name spelled as the domain of the supplier's email address or
//     website;
//   - the company that starts the line with its KvK or VAT number;
//   - the company before a "Postbus", or the first line that reads like a
//     name at the top of the document.
//
// Names OCR garbled are left out.
func supplierName(lines []string, buyer []bool, vatAt int, self Self, domain string) string {
	if name := tradeNameOwner(lines); name != "" && !self.sameName(name) {
		return name
	}
	legal, near := legalName(lines, buyer, vatAt, self)
	if near {
		return legal
	}
	if name := attributedName(lines, self); name != "" {
		if legal != "" && sameCompany(legal, name) {
			return legal
		}
		return name
	}
	for _, name := range []string{legal, domainName(lines, buyer, self, domain), idLineName(lines, buyer, self)} {
		if name != "" {
			return name
		}
	}
	return topName(lines, buyer, self)
}

// legalName is the company name with a legal form nearest the supplier's
// VAT number (vatAt), else the first; near tells whether it is printed
// near the VAT number.
func legalName(lines []string, buyer []bool, vatAt int, self Self) (name string, near bool) {
	dist := -1
	for i, l := range lines {
		if buyer[i] {
			continue
		}
		for _, seg := range segments(l) {
			seg = cleanName(seg)
			if strings.ContainsAny(seg, "@:") || labelLine.MatchString(seg+" ") || garbledName(seg) ||
				len(strings.Fields(seg)) > maxNameWords || self.sameName(seg) {
				continue
			}
			if m := legalForm.FindStringSubmatch(seg); m == nil || nlZip.MatchString(seg) {
				continue
			}
			d := absInt(i - vatAt)
			if vatAt < 0 || d > vatNear {
				d = vatNear + 1
			}
			if name == "" || d < dist {
				name, dist = seg, d
			}
		}
	}
	return name, name != "" && dist <= vatNear
}

var (
	// attribution is a sentence naming who the document is from: "in
	// favour of X", "invoiced on behalf of X", "namens X", "Rechnungssteller:
	// X".
	attribution = regexp.MustCompile(`(?i)\b(?:in favou?r of|on behalf of|ten gunste van|namens|invoiced by|issued by|billed by|gefactureerd door|rechnungssteller(?:in)?)\s*:?\s+(\S.*)$`)
	// attributionEnd ends the name a sentence gives: a comma, a full stop
	// before more text, a column gap.
	attributionEnd = regexp.MustCompile(`\s*(?:[,;|(]|\.\s|\s{2,}|\s-\s).*$`)
)

// maxAttributedWords keeps the rest of a sentence from being taken for the
// name it gives.
const maxAttributedWords = 6

// attributedName is the first company a sentence says the document is from,
// on any line: the buyer's own ("debited from your account … in favour of
// X") too.
func attributedName(lines []string, self Self) string {
	for _, l := range lines {
		m := attribution.FindStringSubmatch(l)
		if m == nil {
			continue
		}
		name := cleanName(attributionEnd.ReplaceAllString(m[1], ""))
		if !legalForm.MatchString(name) {
			name = strings.TrimRight(name, ".")
			// A Dutch or German sentence puts its verb after the name:
			// "namens Voorbeeld Levering verstuurd".
			words := strings.Fields(name)
			for len(words) > 1 && unicode.IsLower([]rune(words[len(words)-1])[0]) {
				words = words[:len(words)-1]
			}
			name = strings.Join(words, " ")
		}
		first, _ := utf8.DecodeRuneInString(name)
		if !unicode.IsUpper(first) && !unicode.IsDigit(first) || len(strings.Fields(name)) > maxAttributedWords ||
			!looksLikeName(name) || self.sameName(name) {
			continue
		}
		return name
	}
	return ""
}

// sameCompany tells whether one name holds the other: the brand
// "ExampleShip" and its legal name "ExampleShip B.V.".
func sameCompany(a, b string) bool {
	a, b = squashName(a), squashName(b)
	return a != "" && b != "" && (strings.Contains(a, b) || strings.Contains(b, a))
}

// squashName is a name to compare without legal form, case, spaces and
// punctuation: "Example Shop B.V." is "exampleshop".
func squashName(s string) string {
	return strings.ReplaceAll(NormalizeName(s), " ", "")
}

// nameParts are the parts of a line a name may be: its segments, and those
// split at commas.
func nameParts(l string) []string {
	var out []string
	for _, seg := range segments(l) {
		for _, p := range strings.Split(seg, ",") {
			if p = cleanName(p); p != "" {
				out = append(out, p)
			}
		}
	}
	return out
}

// domainName is a name printed as the domain of the supplier's email
// address or website spells it: "ExampleShip" for exampleship.example.
func domainName(lines []string, buyer []bool, self Self, domain string) string {
	labels := strings.Split(domain, ".")
	if len(labels) < 2 {
		return ""
	}
	want := labels[len(labels)-2]
	for i, l := range lines {
		if buyer[i] {
			continue
		}
		for _, p := range nameParts(l) {
			if squashName(p) == want && looksLikeName(p) && !self.sameName(p) {
				return p
			}
		}
	}
	return ""
}

// idLineName is the company that starts a line with its KvK or VAT
// number: "ExampleShip, Example Lane 31, … KvK 12345678".
func idLineName(lines []string, buyer []bool, self Self) string {
	for i, l := range lines {
		if buyer[i] || !kvkPattern.MatchString(l) && (findVAT(l) == "" || !vatLabel.MatchString(l)) {
			continue
		}
		parts := nameParts(l)
		if len(parts) < 2 || len(strings.Fields(parts[0])) > maxNameWords {
			continue
		}
		if p := parts[0]; looksLikeName(p) && !vatLabel.MatchString(p) && !kvkPattern.MatchString(p) && !self.sameName(p) {
			return p
		}
	}
	return ""
}

// topName is the company before a "Postbus", or the first line that reads
// like a name at the top of the document.
func topName(lines []string, buyer []bool, self Self) string {
	for i, l := range lines {
		if m := postbusName.FindStringSubmatch(strings.TrimSpace(l)); m != nil && !buyer[i] && looksLikeName(m[1]) && !self.sameName(m[1]) {
			return m[1]
		}
	}
	seen := 0
	for i, l := range lines {
		l = strings.TrimSpace(l)
		if l == "" || buyer[i] || boilerplate.MatchString(l) || pageMarker.MatchString(l) || attribution.MatchString(l) {
			continue
		}
		if seen++; seen > 6 {
			break
		}
		segs := segments(l)
		if j := nextNonEmpty(lines, i); j >= 0 && isPostcodeLine(lines[j]) || otherColumnSays(segs) {
			continue // the street above a postcode, a label and its value
		}
		if j := prevNonEmpty(lines, i); j >= 0 && isStreetLine(lines[j]) {
			continue // the city under a street
		}
		if looksLikeName(segs[0]) && !self.sameName(segs[0]) {
			return segs[0]
		}
	}
	return ""
}

var (
	// tradeName is a sentence naming the legal entity behind a trade name:
	// "X Zakelijk is een handelsnaam van X Retail Energie B.V.".
	tradeName = regexp.MustCompile(`(?i)\b(?:is een handelsnaam van|handelsnaam van|is a trad(?:e|ing) name of|trading name of|ist eine marke der|ist ein handelsname der|é uma marca de|est une marque de)\s+(.+?)[\s,;]*$`)
	// postbusName is the company a postal address starts with: "X, Postbus
	// 1234".
	postbusName = regexp.MustCompile(`(?i)^([^,|]{2,60}?),\s*(?:postbus|p\.?\s?o\.?\s?box|postfach|apartado|bo[iî]te postale)\s*\d`)
	// pageMarker is a page number: "Página 1 de 3", "Seite 2 von 3".
	pageMarker = regexp.MustCompile(`(?i)\b(?:p[aá]gina|p[aá]g\.?|page|seite|blad|pagina)\s*(?:n\.?\s?[º°o]\.?\s*)?\d+\s*(?:de|of|von|van|/)\s*\d+|\b\d{1,3}\s+(?:de|of|von|van)\s+\d{1,3}\s*$`)
	// deliveryWords label the buyer's addresses printed beside the
	// supplier's details.
	deliveryWords = regexp.MustCompile(`(?i)leveringsadres|factuuradres|afleveradres|bezorgadres|postadres|delivery address|billing address|invoice address|lieferadresse|rechnungsadresse|morada de entrega|adresse de livraison|direcci[oó]n de entrega`)
	longNumber    = regexp.MustCompile(`\d{6,}`)
)

// tradeNameOwner is the legal entity a "handelsnaam van" sentence names.
func tradeNameOwner(lines []string) string {
	for _, l := range lines {
		for _, seg := range segments(l) {
			m := tradeName.FindStringSubmatch(seg)
			if m == nil {
				continue
			}
			if name := cleanName(m[1]); legalForm.MatchString(name) && len(strings.Fields(name)) <= maxNameWords && !garbledName(name) {
				return name
			}
		}
	}
	return ""
}

// isStreetLine tells whether a line starts with a street and number.
func isStreetLine(l string) bool {
	segs := segments(l)
	return len(segs) > 0 && streetRe.MatchString(segs[0]) && strings.ContainsAny(segs[0], "0123456789")
}

// isPostcodeLine tells whether a line starts with a postcode and a city.
func isPostcodeLine(l string) bool {
	segs := segments(l)
	if len(segs) == 0 {
		return false
	}
	zip, _, _ := postcode(segs[0], "")
	return zip != "" || ptZip.MatchString(segs[0])
}

// otherColumnSays tells whether the columns of a line make it no name: a
// label of the buyer's address in any column, or a short label followed
// by its value ("Verbruiksperiode   oktober 2026").
func otherColumnSays(segs []string) bool {
	for _, s := range segs {
		if deliveryWords.MatchString(s) {
			return true
		}
	}
	return len(segs) >= 2 && len(strings.Fields(segs[0])) <= 2 && strings.ContainsAny(segs[1], "0123456789")
}

func looksLikeName(l string) bool {
	letters := 0
	for _, r := range l {
		if r >= 'A' && r <= 'Z' || r >= 'a' && r <= 'z' || r > 127 {
			letters++
		}
	}
	switch {
	case letters < 3, garbledName(l), titleWords.MatchString(l), strings.ContainsAny(l, "@:"),
		strings.HasSuffix(l, "!"), findVAT(l) != "", itemHeader.MatchString(l), // a slogan, the VAT number, column headings
		wwwRe.MatchString(l), len(findAmounts(l)) > 0, len(findDates(l)) > 0, longNumber.MatchString(l),
		boilerplate.MatchString(l), pageMarker.MatchString(l), deliveryWords.MatchString(l),
		streetRe.MatchString(l) && strings.ContainsAny(l, "0123456789"):
		return false
	}
	zip, _, _ := postcode(l, "")
	return zip == ""
}

// garbledName tells whether a name is OCR garbage rather than a company
// name: it starts with a quote or other mark OCR reads from specks, or too
// few of its characters are letters.
func garbledName(s string) bool {
	s = strings.TrimSpace(s)
	if s == "" || strings.ContainsAny(s, nameQuotes+nameNoise) {
		return true
	}
	letters, marks := 0, 0
	for _, r := range s {
		switch {
		case unicode.IsLetter(r):
			letters++
		case !unicode.IsSpace(r):
			marks++
		}
	}
	return letters < 2 || float64(letters) < minNameLetters*float64(letters+marks)
}

// Marks that do not belong in a company name: quotes OCR reads from specks
// at the start of a line, and other noise.
const (
	nameQuotes = "\"“”„‟«»‹›`´"
	nameNoise  = "|~#^%*;{}[]<>=_\\¦§¬"
)

// minNameLetters is the share of a name's printed characters that must be
// letters: "Example Shop B.V." has 13 of 15.
const minNameLetters = 0.7

// cleanName drops the marks OCR reads before a name and the spaces around
// it; quotes are kept, as they tell a name is garbled.
func cleanName(s string) string {
	return strings.TrimSpace(strings.TrimLeft(strings.TrimSpace(s), "-–—*•·.,:;_~ "))
}

// nifLabel is the Portuguese tax number's label, printed without the PT
// prefix: "Contribuinte: 123456789", "NIF 123456789".
var nifLabel = regexp.MustCompile(`(?i)\b(?:contribuinte|nif|nipc|n\.?º? ?de contribuinte)\b[^0-9\n]{0,40}(\d{3} ?\d{3} ?\d{3})\b`)

// portugueseNIF reads a labelled Portuguese tax number (NIF) as a VAT
// number, when its check digit is right and the document is Portuguese: its
// IBAN is Portuguese or the label is.
func portugueseNIF(lines []string, buyer []bool, iban string) string {
	for i, l := range lines {
		if buyer[i] {
			continue
		}
		m := nifLabel.FindStringSubmatch(l)
		if m == nil {
			continue
		}
		n := strings.ReplaceAll(m[1], " ", "")
		if ValidPTNIF(n) && (strings.HasPrefix(iban, "PT") || iban == "") {
			return "PT" + n
		}
	}
	return ""
}
