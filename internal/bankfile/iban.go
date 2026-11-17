package bankfile

import (
	"fmt"
	"math/big"
	"strings"
)

// validIBAN tells whether s is an IBAN with correct check digits.
func validIBAN(s string) bool {
	if len(s) < 15 || len(s) > 34 || !isUpperLetter(s[0]) || !isUpperLetter(s[1]) || !isDigit(s[2]) || !isDigit(s[3]) {
		return false
	}
	var digits strings.Builder
	for _, r := range s[4:] + s[:4] {
		switch {
		case r >= '0' && r <= '9':
			digits.WriteRune(r)
		case r >= 'A' && r <= 'Z':
			fmt.Fprintf(&digits, "%d", r-'A'+10)
		default:
			return false
		}
	}
	n, ok := new(big.Int).SetString(digits.String(), 10)
	return ok && new(big.Int).Mod(n, big.NewInt(97)).Int64() == 1
}

// dutchIBAN makes the IBAN of a Dutch account number at the bank code.
func dutchIBAN(bank, number string) string {
	number = strings.Repeat("0", 10-len(number)) + number
	for check := 2; check <= 98; check++ {
		if s := fmt.Sprintf("NL%02d%s%s", check, bank, number); validIBAN(s) {
			return s
		}
	}
	return ""
}

// accountNumber cleans the account number a bank file names: a currency
// some banks glue to the IBAN goes, and an ABN AMRO account number becomes
// its IBAN, so the file meets the account it is for.
func accountNumber(account string, abnAmro bool) string {
	account = strings.ToUpper(strings.ReplaceAll(account, " ", ""))
	if n := len(account); n > 3 && !validIBAN(account) && validIBAN(account[:n-3]) && isUpperLetter(account[n-1]) && isUpperLetter(account[n-2]) && isUpperLetter(account[n-3]) {
		return account[:n-3]
	}
	if abnAmro && len(account) >= 9 && len(account) <= 10 && strings.Trim(account, "0123456789") == "" {
		if iban := dutchIBAN("ABNA", account); iban != "" {
			return iban
		}
	}
	return account
}

func isUpperLetter(b byte) bool { return b >= 'A' && b <= 'Z' }
func isDigit(b byte) bool       { return b >= '0' && b <= '9' }
