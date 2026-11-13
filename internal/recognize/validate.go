package recognize

import (
	"regexp"
	"strings"

	"github.com/adirkuhn/moneyharpy/internal/iban"
)

// NormalizeIBAN upper-cases an IBAN and removes the spaces, dots and dashes
// documents print between its groups.
func NormalizeIBAN(s string) string {
	return iban.Normalize(stripSeparators(s))
}

// ValidIBAN tells whether s (in any common print form) is an IBAN with a
// correct ISO 13616 mod-97 checksum and, for known countries, length.
func ValidIBAN(s string) bool {
	n := NormalizeIBAN(s)
	if l, ok := ibanLengths[n[:min(2, len(n))]]; ok && len(n) != l {
		return false
	}
	return iban.Valid(n)
}

// ibanLengths are the IBAN lengths of the countries suppliers usually bank in.
var ibanLengths = map[string]int{
	"AT": 20, "BE": 16, "BG": 22, "CH": 21, "CY": 28, "CZ": 24, "DE": 22,
	"DK": 18, "EE": 20, "ES": 24, "FI": 18, "FR": 27, "GB": 22, "GR": 27,
	"HR": 21, "HU": 28, "IE": 22, "IT": 27, "LT": 20, "LU": 20, "LV": 21,
	"MT": 31, "NL": 18, "NO": 15, "PL": 28, "PT": 25, "RO": 24, "SE": 24,
	"SI": 19, "SK": 24,
}

var ibanStart = regexp.MustCompile(`\b[A-Z]{2}\d{2}`)

// FindIBANs returns every valid IBAN in text, normalised, in order of first
// appearance and without duplicates. IBANs may be printed in groups separated
// by single spaces.
func FindIBANs(text string) []string {
	var out []string
	for _, loc := range ibanStart.FindAllStringIndex(text, -1) {
		if s := ibanAt(text, loc[0]); s != "" && !contains(out, s) {
			out = append(out, s)
		}
	}
	return out
}

// ibanAt reads a valid IBAN starting at i, or "".
func ibanAt(text string, i int) string {
	country := text[i : i+2]
	want, known := ibanLengths[country]
	var b strings.Builder
	for j := i; j < len(text) && b.Len() < 34; j++ {
		c := text[j]
		switch {
		case c >= '0' && c <= '9' || c >= 'A' && c <= 'Z':
			b.WriteByte(c)
		case c == ' ' && j+1 < len(text) && isAlnum(text[j+1]) && b.Len() >= 4:
			continue
		default:
			j = len(text)
		}
		if known && b.Len() == want {
			break
		}
	}
	s := b.String()
	if known {
		if len(s) == want && iban.Valid(s) {
			return s
		}
		return ""
	}
	for l := len(s); l >= 15; l-- {
		if iban.Valid(s[:l]) {
			return s[:l]
		}
	}
	return ""
}

// NormalizeVAT upper-cases a VAT identification number and removes spaces,
// dots and dashes.
func NormalizeVAT(s string) string { return strings.ToUpper(stripSeparators(s)) }

// vatFormats are the EU (and UK, Swiss) VAT number shapes after the prefix.
var vatFormats = map[string]*regexp.Regexp{}

func init() {
	for c, f := range map[string]string{
		"AT": `U\d{8}`, "BE": `[01]\d{9}`, "BG": `\d{9,10}`, "CY": `\d{8}[A-Z]`,
		"CZ": `\d{8,10}`, "DE": `\d{9}`, "DK": `\d{8}`, "EE": `\d{9}`,
		"EL": `\d{9}`, "ES": `[0-9A-Z]\d{7}[0-9A-Z]`, "FI": `\d{8}`,
		"FR": `[0-9A-Z]{2}\d{9}`, "HR": `\d{11}`, "HU": `\d{8}`,
		"IE": `\d{7}[A-Z]{1,2}|\d[A-Z+*]\d{5}[A-Z]`, "IT": `\d{11}`,
		"LT": `\d{9}|\d{12}`, "LU": `\d{8}`, "LV": `\d{11}`, "MT": `\d{8}`,
		"NL": `\d{9}B\d{2}`, "PL": `\d{10}`, "PT": `\d{9}`, "RO": `\d{2,10}`,
		"SE": `\d{12}`, "SI": `\d{8}`, "SK": `\d{10}`, "GB": `\d{9}|\d{12}`,
		"XI": `\d{9}|\d{12}`, "CHE": `\d{9}`,
	} {
		vatFormats[c] = regexp.MustCompile(`^(?:` + f + `)$`)
	}
}

// ValidVAT tells whether s is shaped like a VAT number of its country prefix;
// Dutch numbers must also pass their checksum (ValidNLVAT).
func ValidVAT(s string) bool {
	n := NormalizeVAT(s)
	if strings.HasPrefix(n, "NL") {
		return ValidNLVAT(n)
	}
	return vatShaped(n)
}

func vatShaped(n string) bool {
	for _, prefix := range []string{n[:min(3, len(n))], n[:min(2, len(n))]} {
		if f, ok := vatFormats[prefix]; ok {
			return f.MatchString(n[len(prefix):])
		}
	}
	return false
}

// ValidNLVAT checks a Dutch VAT number (NL + 9 digits + B + 2 digits): the
// company format passes the elfproef on the nine digits, the sole proprietor
// format (since 2020) passes mod 97 over the whole number.
func ValidNLVAT(s string) bool {
	n := NormalizeVAT(s)
	if !vatFormats["NL"].MatchString(strings.TrimPrefix(n, "NL")) || !strings.HasPrefix(n, "NL") {
		return false
	}
	sum := 0
	for i := range 8 {
		sum += int(n[2+i]-'0') * (9 - i)
	}
	if sum%11 != 10 && sum%11 == int(n[10]-'0') {
		return true
	}
	return mod97(n) == 1
}

// ValidRF checks an ISO 11649 structured creditor reference (RF + 2 check
// digits + up to 21 characters).
func ValidRF(s string) bool {
	n := strings.ToUpper(stripSeparators(s))
	if len(n) < 5 || len(n) > 25 || !strings.HasPrefix(n, "RF") {
		return false
	}
	for i := 2; i < len(n); i++ {
		if !isAlnum(n[i]) || i < 4 && (n[i] < '0' || n[i] > '9') {
			return false
		}
	}
	return mod97(n[4:]+n[:4]) == 1
}

// ValidBetalingskenmerk checks a 16-digit Dutch payment reference: its first
// digit is the check digit over the other 15 (weights 2,4,8,5,10,9,7,3,6,1
// from the right, 11 minus the sum mod 11; 10 becomes 1, 11 becomes 0).
func ValidBetalingskenmerk(s string) bool {
	n := stripSeparators(s)
	if len(n) != 16 {
		return false
	}
	weights := []int{2, 4, 8, 5, 10, 9, 7, 3, 6, 1}
	sum := 0
	for i := range 15 {
		c := n[15-i]
		if c < '0' || c > '9' {
			return false
		}
		sum += int(c-'0') * weights[i%len(weights)]
	}
	check := 11 - sum%11
	switch check {
	case 10:
		check = 1
	case 11:
		check = 0
	}
	return n[0] >= '0' && n[0] <= '9' && int(n[0]-'0') == check
}

// mod97 is the ISO 7064 remainder of s with letters as 10..35.
func mod97(s string) int {
	mod := 0
	for _, r := range s {
		switch {
		case r >= 'A' && r <= 'Z':
			mod = (mod*100 + int(r-'A'+10)) % 97
		case r >= '0' && r <= '9':
			mod = (mod*10 + int(r-'0')) % 97
		}
	}
	return mod
}

func stripSeparators(s string) string {
	return strings.Map(func(r rune) rune {
		if r == ' ' || r == '.' || r == '-' || r == ' ' || r == '\t' {
			return -1
		}
		return r
	}, strings.TrimSpace(s))
}

func isAlnum(c byte) bool { return c >= '0' && c <= '9' || c >= 'A' && c <= 'Z' }

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

// ValidPTNIF checks a Portuguese tax number (NIF/NIPC): nine digits whose
// last is 11 minus the weighted sum (weights 9 to 2) modulo 11, or 0 when
// that is 10 or 11.
func ValidPTNIF(n string) bool {
	if len(n) != 9 || !isDigits(n) {
		return false
	}
	sum := 0
	for i := range 8 {
		sum += int(n[i]-'0') * (9 - i)
	}
	check := 11 - sum%11
	if check >= 10 {
		check = 0
	}
	return int(n[8]-'0') == check
}
