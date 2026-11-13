package recognize

import (
	"regexp"
	"strings"
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
	vatSure, kvkFound := false, false
	for i, l := range lines {
		if buyer[i] {
			continue
		}
		if s.VATNumber == "" {
			if v := findVAT(l); v != "" {
				s.VATNumber = v
				vatSure = ValidVAT(v) && (vatLabel.MatchString(l) || strings.HasPrefix(v, "NL"))
			}
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
	hint := countryOf(s.VATNumber)
	if hint == "" && s.IBAN != "" {
		hint = s.IBAN[:2]
	}
	readAddress(lines, buyer, hint, &s)
	if s.Country == "" {
		s.Country = hint
	}
	s.Name = supplierName(lines, buyer)

	switch {
	case vatSure || kvkFound || s.IBAN != "" && s.Name != "":
		return s, Certain
	case s != Supplier{}:
		return s, Uncertain
	}
	return s, ""
}

// findVAT finds a VAT number on a line; groups may be separated by single
// spaces or dots ("NL 1234.56.782.B01").
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
		if v := b.String(); vatShaped(v) {
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

// supplierName takes the first company name with a legal form (B.V., GmbH,
// Ltd …) outside the buyer's block, else the first line that reads like a
// name at the top of the document.
func supplierName(lines []string, buyer []bool) string {
	for i, l := range lines {
		if buyer[i] {
			continue
		}
		for _, seg := range segments(l) {
			if strings.ContainsAny(seg, "@:") || labelLine.MatchString(seg+" ") {
				continue
			}
			if m := legalForm.FindStringSubmatch(seg); m != nil && !nlZip.MatchString(seg) {
				return seg
			}
		}
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
	case letters < 3, titleWords.MatchString(l), strings.ContainsAny(l, "@:"),
		wwwRe.MatchString(l), len(findAmounts(l)) > 0, len(findDates(l)) > 0,
		streetRe.MatchString(l) && strings.ContainsAny(l, "0123456789"):
		return false
	}
	zip, _, _ := postcode(l, "")
	return zip == ""
}
