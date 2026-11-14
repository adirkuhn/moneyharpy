package recognize

import "strings"

// maxVATBody is how many characters after the country prefix a VAT number
// may have, separators left out.
const maxVATBody = 14

// findVAT finds a VAT number on a line. Its groups may be separated by
// single spaces, dots or hyphens ("NL 0012.34.560.B.01"), and OCR may have
// read a digit as a letter ("NLOO12…": O for 0, I or L for 1, S for 5, B
// for 8) or the Dutch B as an 8: such confusions are repaired where the
// country's format needs a digit (or the B), and only when the result is
// valid. A valid number is preferred; else the first one shaped like a VAT
// number is returned, so a number whose checksum fails is still read.
func findVAT(l string) string {
	shaped := ""
	for _, loc := range vatStart.FindAllStringIndex(l, -1) {
		if loc[0] > 0 && isAlnum(l[loc[0]-1]) || ibanAt(l, loc[0]) != "" {
			continue
		}
		prefix := l[loc[0]:loc[1]]
		body, cuts := vatBody(l[loc[1]:])
		for k := len(cuts) - 1; k >= 0; k-- {
			if v := validVATOf(prefix, body[:cuts[k]]); v != "" {
				return v
			}
		}
		for k := len(cuts) - 1; k >= 0 && shaped == ""; k-- {
			if v := prefix + body[:cuts[k]]; vatShaped(v) {
				shaped = v
			}
		}
	}
	return shaped
}

// vatBody reads what follows a VAT number's country prefix: letters and
// digits, with single separators between groups. cuts are the lengths at
// which the body may end: at each separator and at its end, so a number is
// never cut inside a group.
func vatBody(rest string) (body string, cuts []int) {
	var b strings.Builder
	for j := 0; j < len(rest) && b.Len() < maxVATBody; j++ {
		c := rest[j]
		switch {
		case isAlnum(c):
			b.WriteByte(c)
		case (c == ' ' || c == '.' || c == '-') && j+1 < len(rest) && isAlnum(rest[j+1]):
			if b.Len() > 0 {
				cuts = append(cuts, b.Len())
			}
		default:
			j = len(rest)
		}
	}
	if b.Len() > 0 && (len(cuts) == 0 || cuts[len(cuts)-1] != b.Len()) {
		cuts = append(cuts, b.Len())
	}
	return b.String(), cuts
}

// validVATOf is prefix+body when that is a valid VAT number, else the
// number with OCR confusions repaired when that is valid, else "".
func validVATOf(prefix, body string) string {
	if v := prefix + body; ValidVAT(v) {
		return v
	}
	if v := prefix + repairVATBody(prefix, body); ValidVAT(v) {
		return v
	}
	return ""
}

// repairVATBody undoes OCR confusions in a VAT number's body where its
// format needs a digit: a Dutch number is nine digits, B and two digits;
// the others repaired are digits only. Most of the body must already be
// digits, so words are not turned into numbers.
func repairVATBody(prefix, body string) string {
	digits := 0
	for i := range len(body) {
		if isDigit(body[i]) {
			digits++
		}
	}
	if 2*digits < len(body) {
		return body
	}
	b := []byte(body)
	switch {
	case prefix == "NL" && len(b) == 12:
		for i := range b {
			if i == 9 {
				if b[i] == '8' {
					b[i] = 'B'
				}
				continue
			}
			b[i] = ocrDigit(b[i])
		}
	case prefix == "AT" && len(b) > 0 && b[0] == 'U':
		for i := 1; i < len(b); i++ {
			b[i] = ocrDigit(b[i])
		}
	case digitsOnlyVAT[prefix]:
		for i := range b {
			b[i] = ocrDigit(b[i])
		}
	}
	return string(b)
}

// digitsOnlyVAT are the country prefixes whose VAT numbers are digits only.
var digitsOnlyVAT = map[string]bool{
	"BE": true, "BG": true, "CZ": true, "DE": true, "DK": true, "EE": true, "EL": true, "FI": true, "HR": true,
	"HU": true, "IT": true, "LT": true, "LU": true, "LV": true, "MT": true, "PL": true, "PT": true, "RO": true,
	"SE": true, "SI": true, "SK": true, "GB": true, "XI": true, "CHE": true,
}

// ocrDigit is the digit OCR may have read as the letter c.
func ocrDigit(c byte) byte {
	switch c {
	case 'O', 'D', 'Q':
		return '0'
	case 'I', 'L':
		return '1'
	case 'S':
		return '5'
	case 'B':
		return '8'
	case 'Z':
		return '2'
	}
	return c
}
