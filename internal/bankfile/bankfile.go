// Package bankfile reads bank statement files: ISO 20022 CAMT (052, 053
// and 054) and SWIFT MT940.
package bankfile

import (
	"bytes"
	"errors"
	"time"
)

// Errors returned by Parse.
var (
	ErrUnknownFormat = errors.New("not a CAMT or MT940 file")
	ErrInvalid       = errors.New("the file could not be read")
)

// Format names a file format.
type Format string

// The formats Parse understands.
const (
	CAMT  Format = "camt"
	MT940 Format = "mt940"
)

// Statement is one account's statement in a file.
type Statement struct {
	// Account is the account number (IBAN) the statement belongs to.
	Account   string
	Currency  string
	Reference string
	// Date and Balance are the closing balance, if the file has one.
	Date    *time.Time
	Balance *int64
	Entries []Entry
}

// Entry is one booked transaction. Amounts are in cents; positive amounts
// came in.
type Entry struct {
	Date           time.Time
	ValueDate      *time.Time
	Amount         int64
	Currency       string
	Message        string
	Code           string
	ContraName     string
	ContraAccount  string
	BatchReference string
	// TransactionID is the bank's own reference, when it gives one.
	TransactionID string
}

// MaxSize is the largest file Parse reads.
const MaxSize = 10 << 20

// Parse reads a statement file, recognising its format from the content.
func Parse(data []byte) ([]Statement, Format, error) {
	if len(data) > MaxSize {
		return nil, "", ErrInvalid
	}
	trimmed := bytes.TrimSpace(bytes.TrimPrefix(data, []byte("\xef\xbb\xbf")))
	switch {
	case bytes.HasPrefix(trimmed, []byte("<")):
		s, err := parseCAMT(trimmed)
		return s, CAMT, err
	case bytes.Contains(trimmed, []byte(":20:")) && bytes.Contains(trimmed, []byte(":61:")):
		s, err := parseMT940(trimmed)
		return s, MT940, err
	}
	return nil, "", ErrUnknownFormat
}
