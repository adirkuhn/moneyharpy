// Package iban checks International Bank Account Numbers.
package iban

import (
	"regexp"
	"strings"
)

var shape = regexp.MustCompile(`^[A-Z]{2}[0-9]{2}[A-Z0-9]{11,30}$`)

// Normalize removes spaces and upper-cases an IBAN as people type it.
func Normalize(s string) string {
	return strings.ToUpper(strings.ReplaceAll(strings.TrimSpace(s), " ", ""))
}

// Valid checks the format and the ISO 13616 mod-97 checksum of a normalized
// IBAN.
func Valid(iban string) bool {
	if !shape.MatchString(iban) {
		return false
	}
	mod := 0
	for _, r := range iban[4:] + iban[:4] {
		if r >= 'A' {
			mod = (mod*100 + int(r-'A'+10)) % 97
		} else {
			mod = (mod*10 + int(r-'0')) % 97
		}
	}
	return mod == 1
}
