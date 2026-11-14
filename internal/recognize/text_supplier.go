package recognize

import (
	"regexp"
	"strings"
	"unicode"
)

var (
	// buyerLabel starts a block with the buyer's details ("Factuur aan:").
	buyerLabel = regexp.MustCompile(`(?i)^\s*(?:factuur aan|aan|bill(?:ed)? to|invoice to|sold to|ship to|deliver to|delivery address|afleveradres|klant|customer|cliente|client|kunde|rechnungsempf[äa]nger|lieferadresse|factur[ée] [àa]|uw gegevens|t\.?a\.?v\.?)\b\s*:?`)
	// buyerWords mark a single line about the buyer, or the buyer's account.
	buyerWords = regexp.MustCompile(`(?i)\b(?:uw|your|ihre?|cliente|client|customer|klant|kunde|buyer|koper|afnemer|verlegd naar|afgeschreven|incasso|ge[ïi]ncasseerd|debited|abgebucht|lastschrift|machtiging|mandate)\b`)
	vatLabel   = regexp.MustCompile(`(?i)\b(?:btw|b\.t\.w\.?|vat|tva|iva|ust|ust-?id(?:nr)?\.?|nif|nipc|mwst|tax|moms|p\.?\s?iva|cif|nr\.? tva)\b`)
	vatStart   = regexp.MustCompile(`\b(?:AT|BE|BG|CY|CZ|DE|DK|EE|EL|ES|FI|FR|HR|HU|IE|IT|LT|LU|LV|MT|NL|PL|PT|RO|SE|SI|SK|GB|XI|CHE)`)
	kvkPattern = regexp.MustCompile(`(?i)(?:\bkvk|\bk\.v\.k\.?|kamer van koophandel|\bcoc\b|chamber of commerce|handelsregister)[^0-9\n]{0,20}(\d{8})\b`)
	ibanLabel  = regexp.MustCompile(`(?i)iban|rekening|bank|account|konto|\bnib\b|over te maken|overmaken|t\.n\.v`)
	emailRe    = regexp.MustCompile(`[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\.[A-Za-z]{2,}`)
	wwwRe      = regexp.MustCompile(`(?i)\b(www\.[a-z0-9-]+(?:\.[a-z0-9-]+)+)`)
	webLabelRe = regexp.MustCompile(`(?i)\b(?:website|web|internet|site)\s*:\s*(?:https?://)?([a-z0-9-]+(?:\.[a-z0-9-]+)+)`)
	phoneRe    = regexp.MustCompile(`(?i)(?:\btel(?:efoon|efon|ephone|\.)?|\bphone|\bt\b\.?|\btelefone|\btél\.?|\btel[ée]fono|\bmob(?:iel|ile)?)\s*[:.]?\s*(\+?\d[\d\s()./-]{6,}\d)`)
	legalForm  = regexp.MustCompile(`(?i)^(.*\p{L}.*?)[\s,]+(b\.\s?v\.?|bv|n\.\s?v\.?|nv|v\.o\.f\.?|vof|c\.v\.|gmbh|ag|ug|kg|e\.\s?k\.|ltd\.?|limited|llc|inc\.?|plc|s\.a\.?|sa|s\.l\.?|sl|sarl|sas|lda\.?|bvba|srl|s\.r\.l\.|oy|ab|aps|a/s)$`)
	segmentSep = regexp.MustCompile(`\s*(?:\||·|•|\s{2,}|\s-\s|\t)\s*`)
	titleWords = regexp.MustCompile(`(?i)^(?:factuur|invoice|rechnung|fatura|facture|factura|creditnota|credit ?note|kassabon|kassenbon|receipt|bon|quittung|recibo|ticket|tax invoice|nota)\b`)
	labelLine  = regexp.MustCompile(`^[^:]{1,30}:\s`)
	streetRe   = regexp.MustCompile(`(?i)^(?:\p{L}[\p{L}\p{M}'.,\- ]*\s\d+\s?[a-z]?(?:[-/ ]?\d+[a-z]?)?(?:,.*)?|\d+[a-z]?,?\s+\p{L}.*|(?:postbus|p\.?\s?o\.?\s?box|postfach|apartado|bo[iî]te postale)\s*\d+.*)$`)
	nlZip      = regexp.MustCompile(`^(\d{4})\s?([A-Z]{2})\b\s*(.*)$`)
	ptZip      = regexp.MustCompile(`^(\d{4}-\d{3})\s+(.+)$`)
	fiveZip    = regexp.MustCompile(`^(?:[A-Z]{1,2}-)?(\d{5})\s+(\p{L}.*)$`)
	fourZip    = regexp.MustCompile(`^(?:[A-Z]{1,2}-)?(\d{4})\s+(\p{L}.*)$`)
	gbZip      = regexp.MustCompile(`^(\p{L}[\p{L} .'-]*?)\s+([A-Z]{1,2}\d[A-Z\d]?\s?\d[A-Z]{2})$`)
	cityRe     = regexp.MustCompile(`^\p{L}[\p{L}\p{M}' .-]*$`)
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

// readSupplier reads who sent the document.
func readSupplier(lines []string, buyer []bool) (Supplier, Confidence) {
	var s Supplier
	kvkFound := false
	vat, vatAt, vatSure := supplierVAT(lines, buyer)
	s.VATNumber = vat
	for i, l := range lines {
		if buyer[i] {
			continue
		}
		if s.KVK == "" {
			if m := kvkPattern.FindStringSubmatch(l); m != nil {
				s.KVK, kvkFound = m[1], true
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
	s.IBAN = supplierIBAN(lines, buyer)
	if s.VATNumber == "" {
		if v := portugueseNIF(lines, buyer, s.IBAN); v != "" {
			s.VATNumber, vatSure = v, true
		}
	}
	hint := countryOf(s.VATNumber)
	if hint == "" && s.IBAN != "" {
		hint = s.IBAN[:2]
	}
	readAddress(lines, buyer, hint, &s)
	if s.Country == "" {
		s.Country = hint
	}
	s.Name = supplierName(lines, buyer, vatAt)

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
func supplierVAT(lines []string, buyer []bool) (vat string, at int, sure bool) {
	at, rank := -1, 0
	for i, l := range lines {
		if buyer[i] {
			continue
		}
		v := findVAT(l)
		if v == "" {
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

// findVAT finds a VAT number on a line; groups may be separated by single
// spaces or dots ("NL 1234.56.782.B01"). A Dutch number whose B was read as
// an 8 is repaired when that makes it valid.
func findVAT(l string) string {
	for _, loc := range vatStart.FindAllStringIndex(l, -1) {
		if loc[0] > 0 && isAlnum(l[loc[0]-1]) {
			continue
		}
		var b strings.Builder
		b.WriteString(l[loc[0]:loc[1]])
		for j := loc[1]; j < len(l) && b.Len() < 20; j++ {
			c := l[j]
			switch {
			case isAlnum(c):
				b.WriteByte(c)
			case (c == ' ' || c == '.' || c == '-') && j+1 < len(l) &&
				(isDigit(l[j+1]) || l[j+1] == 'B' && j+2 < len(l) && isDigit(l[j+2]) || l[j+1] == 'U' && b.Len() == 2):
				continue
			default:
				j = len(l)
			}
		}
		v := b.String()
		if len(v) == 14 && strings.HasPrefix(v, "NL") && v[11] == '8' && isDigits(v[2:]) {
			if fixed := v[:11] + "B" + v[12:]; ValidNLVAT(fixed) {
				return fixed
			}
		}
		if vatShaped(v) {
			return v
		}
	}
	return ""
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

// supplierIBAN takes the first valid IBAN not on a buyer line, preferring one
// printed next to "IBAN"/"rekening"/"bank".
func supplierIBAN(lines []string, buyer []bool) string {
	first := ""
	for i, l := range lines {
		if buyer[i] {
			continue
		}
		ibans := FindIBANs(l)
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

// readAddress finds the first postcode and city not in the buyer's block,
// with the street before it.
func readAddress(lines []string, buyer []bool, hint string, s *Supplier) {
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
			if zip == "" {
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

// supplierName takes a company name with a legal form (B.V., GmbH, Ltd …)
// outside the buyer's block: the one nearest the supplier's VAT number
// (vatAt, -1 for none) when one is printed near it, else the first. Without
// one it takes the first line that reads like a name at the top of the
// document. Names OCR garbled are left out.
func supplierName(lines []string, buyer []bool, vatAt int) string {
	name, dist := "", -1
	for i, l := range lines {
		if buyer[i] {
			continue
		}
		for _, seg := range segments(l) {
			seg = cleanName(seg)
			if strings.ContainsAny(seg, "@:") || labelLine.MatchString(seg+" ") || garbledName(seg) ||
				len(strings.Fields(seg)) > maxNameWords {
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
	if name != "" {
		return name
	}
	seen := 0
	for i, l := range lines {
		l = strings.TrimSpace(l)
		if l == "" || buyer[i] {
			continue
		}
		if seen++; seen > 6 {
			break
		}
		if looksLikeName(l) {
			return l
		}
	}
	return ""
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
		wwwRe.MatchString(l), len(findAmounts(l)) > 0, len(findDates(l)) > 0,
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
