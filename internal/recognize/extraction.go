// Package recognize is Scan and Recognise: it reads incoming documents'
// attachments and proposes their type, supplier, dates, amounts, VAT and
// lines, learning from earlier documents of the same supplier.
package recognize

import (
	"context"
	"time"

	"github.com/adirkuhn/moneyharpy/internal/money"
)

// Source tells where an extraction came from.
type Source string

const (
	SourceEInvoice Source = "e_invoice" // UBL (Peppol) or Factur-X XML, exact
	SourceText     Source = "text"      // the text of a digital PDF
	SourceOCR      Source = "ocr"       // text read from a scan or photo
	SourceAI       Source = "ai"        // the optional AI model
)

// Kind is the document type an extraction suggests.
type Kind string

const (
	KindUnknown         Kind = ""
	KindPurchaseInvoice Kind = "purchase_invoice"
	KindReceipt         Kind = "receipt"
)

// Confidence is how sure a value is; uncertain values are flagged for review.
type Confidence string

const (
	Certain   Confidence = "certain"
	Uncertain Confidence = "uncertain"
)

// Value is an extracted value with how sure it is. The zero value is not
// found.
type Value[T any] struct {
	V          T
	Found      bool
	Confidence Confidence
}

// Supplier is what identifies the sender, to match or create a contact.
type Supplier struct {
	Name      string
	VATNumber string // e.g. NL123456789B01, normalised without spaces
	KVK       string // 8 digits
	IBAN      string // normalised, without spaces
	Email     string
	Website   string
	Address1  string
	Zipcode   string
	City      string
	Country   string // ISO 3166-1 alpha-2
	Phone     string
}

// Line is one line of the document.
type Line struct {
	Description string
	Quantity    Value[string] // as written, e.g. "1" or "2,5" normalised to "2.5"
	Price       Value[money.Amount]
	// Total is the line total as printed, incl. or excl. VAT as the document
	// states (Extraction.PricesInclTax).
	Total   Value[money.Amount]
	TaxRate Value[string] // percentage such as "21", "9", "0"
}

// VAT is the VAT of one rate.
type VAT struct {
	Rate    string // percentage such as "21"
	Taxable money.Amount
	Tax     money.Amount
	// ReverseCharge is set when the buyer accounts for the VAT: a reverse
	// charge or an intra-community supply exempt in the seller's country.
	ReverseCharge bool
}

// Extraction is everything read from one document.
type Extraction struct {
	Source        Source
	Kind          Value[Kind]
	Supplier      Supplier
	SupplierFound Confidence // how sure the supplier identification is
	Reference     Value[string]
	Date          Value[time.Time]
	DueDate       Value[time.Time]
	Currency      Value[string] // ISO 4217
	PricesInclTax Value[bool]
	TotalInclTax  Value[money.Amount]
	TotalExclTax  Value[money.Amount]
	VAT           []VAT
	Lines         []Line
	// PaymentReference is a structured payment reference (e.g. RF or Dutch
	// betalingskenmerk) for paying and bank matching.
	PaymentReference Value[string]
}

// Good tells whether the rules read enough to fill in a document without
// asking the AI model: a total and a date, both certain.
func (e Extraction) Good() bool {
	return e.TotalInclTax.Found && e.TotalInclTax.Confidence == Certain &&
		e.Date.Found && e.Date.Confidence == Certain
}

// Model is the optional AI model, asked only when the rules could not read a
// document well enough (Extraction.Good is false) and an administrator turned
// it on. It gets the text read so far and the pages as PNG images.
type Model interface {
	Extract(ctx context.Context, text string, pages [][]byte) (Extraction, error)
	// Ping checks the model answers with the saved settings.
	Ping(ctx context.Context) error
}
