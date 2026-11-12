package sales

import (
	"crypto/rand"
	"math/big"
	"regexp"
	"strconv"
	"strings"
	"unicode"
)

// Payment references are ISO 11649 RF creditor references, as Moneybird
// gives sent invoices: "RF", two check digits and a body. The stored form
// pads the body with zeros to 21 characters (RF130000000000000PV96X5KP), as
// Moneybird's payment_reference; the short form leaves the zeros out
// (RF13PV96X5KP), as its short_payment_reference and printed invoices.
const (
	referenceBody     = 8
	referenceLongBody = 21
	referenceAlphabet = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZ"
	// draftReference is the body of the reference a draft shows, after
	// Moneybird's placeholder, such as "RF95CONCEPT42" for draft 42.
	draftReference = "CONCEPT"
)

// NewPaymentReference returns a new random payment reference in its stored
// form.
func NewPaymentReference() string {
	body := make([]byte, referenceBody)
	for i := range body {
		// The body starts with a letter, as Moneybird's do, so the short
		// form keeps all of it.
		from := 0
		if i == 0 {
			from = 10
		}
		n, err := rand.Int(rand.Reader, big.NewInt(int64(len(referenceAlphabet)-from)))
		if err != nil {
			panic("payment reference: " + err.Error())
		}
		body[i] = referenceAlphabet[from+int(n.Int64())]
	}
	padded := strings.Repeat("0", referenceLongBody-referenceBody) + string(body)
	return "RF" + rfCheck(padded) + padded
}

// ShortPaymentReference is the reference without the zeros padding its
// body. A reference that is not an RF reference, such as an invoice number
// from before, stays as it is.
func ShortPaymentReference(ref string) string {
	if len(ref) <= 4 || !strings.HasPrefix(ref, "RF") {
		return ref
	}
	body := strings.TrimLeft(ref[4:], "0")
	if body == "" {
		return ref
	}
	return ref[:4] + body
}

// DraftPaymentReference is the reference a draft shows until it is sent and
// gets its own.
func DraftPaymentReference(draftID int) string {
	body := draftReference
	if draftID > 0 {
		body += strconv.Itoa(draftID)
	}
	return "RF" + rfCheck(body) + body
}

// ValidRF reports whether ref, in any case or spacing, is an RF reference
// with correct check digits.
func ValidRF(ref string) bool {
	ref = NormalizeReference(ref)
	if len(ref) < 5 || len(ref) > 25 || !strings.HasPrefix(ref, "RF") {
		return false
	}
	for _, r := range ref[4:] {
		if !strings.ContainsRune(referenceAlphabet, r) {
			return false
		}
	}
	return rfCheck(ref[4:]) == ref[2:4]
}

// NormalizeReference removes the spaces from a reference and writes it in
// capitals.
func NormalizeReference(s string) string {
	return strings.ToUpper(strings.Map(func(r rune) rune {
		if unicode.IsSpace(r) {
			return -1
		}
		return r
	}, s))
}

// ContainsPaymentReference reports whether a bank transaction's text names
// the payment reference, in its stored or short form, in any case or
// spacing, and not as part of a longer code.
func ContainsPaymentReference(text, ref string) bool {
	ref = NormalizeReference(ref)
	if ref == "" || text == "" {
		return false
	}
	forms := []string{ref}
	if short := ShortPaymentReference(ref); short != ref {
		forms = append(forms, short)
	}
	for _, form := range forms {
		if referencePattern(form).MatchString(text) {
			return true
		}
	}
	return false
}

// referencePattern matches the reference with spaces anywhere in it, on its
// own.
func referencePattern(ref string) *regexp.Regexp {
	parts := make([]string, 0, len(ref))
	for _, r := range ref {
		parts = append(parts, regexp.QuoteMeta(string(r)))
	}
	return regexp.MustCompile(`(?i)(?:^|[^\p{L}\p{N}])` + strings.Join(parts, `\s*`) + `(?:$|[^\p{L}\p{N}])`)
}

// rfCheck works out the two check digits of an RF reference with body b:
// 98 minus the remainder of body + "RF00", letters as numbers, modulo 97.
func rfCheck(body string) string {
	rem := 0
	for _, r := range body + "RF00" {
		v := strings.IndexRune(referenceAlphabet, r)
		if v >= 10 {
			rem = (rem*100 + v) % 97
		} else {
			rem = (rem*10 + v) % 97
		}
	}
	check := 98 - rem
	if check < 10 {
		return "0" + strconv.Itoa(check)
	}
	return strconv.Itoa(check)
}
