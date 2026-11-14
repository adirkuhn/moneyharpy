package recognize

import (
	"errors"
	"slices"
	"strings"
	"time"

	"github.com/adirkuhn/moneyharpy/internal/resource"
)

// ErrNotFound means a document has no recognition: nothing was read of it.
var ErrNotFound = errors.New("recognition not found")

// State is where Scan and Recognise is with a document.
type State string

const (
	// StateNone: nothing to read, or nothing read yet.
	StateNone State = "none"
	// StatePending: to be read in the background.
	StatePending State = "pending"
	// StateDone: read and filled in.
	StateDone State = "done"
	// StateFailed: the attachments could not be read.
	StateFailed State = "failed"
)

// Field is a field filled in automatically, named as in the document's
// request (contact_id, reference, details.0.price, ...).
type Field struct {
	Name       string
	Confidence Confidence
	// Reason is where the value came from; empty for fields filled in
	// before reasons were kept, which ReasonOf tells.
	Reason Reason
	// SourceDocumentID is the earlier document a remembered value came
	// from; zero when unknown or removed since.
	SourceDocumentID resource.ID
	// SourceReference is that document's reference when the value was
	// filled in.
	SourceReference string
}

// Reason is where a value filled in came from.
type Reason string

const (
	// ReasonAttachment: read from the document's attachments.
	ReasonAttachment Reason = "attachment"
	// ReasonPreviousDocument: remembered of the supplier's earlier
	// documents.
	ReasonPreviousDocument Reason = "previous_document"
	// ReasonDefault: neither read nor remembered, such as the standard VAT
	// rate.
	ReasonDefault Reason = "default"
)

// ReasonOf tells where the value of a field came from. Fields filled in
// before reasons were kept were read, but for categories, which were
// only ever remembered.
func ReasonOf(f Field) Reason {
	switch {
	case f.Reason != "":
		return f.Reason
	case strings.HasSuffix(f.Name, ".ledger_account_id"):
		return ReasonPreviousDocument
	}
	return ReasonAttachment
}

// Names of the fields Scan and Recognise fills in.
const (
	FieldDocumentType     = "document_type"
	FieldContactID        = "contact_id"
	FieldReference        = "reference"
	FieldDate             = "date"
	FieldDueDate          = "due_date"
	FieldCurrency         = "currency"
	FieldPricesAreInclTax = "prices_are_incl_tax"
)

// Recognition is what Scan and Recognise did with one incoming document.
type Recognition struct {
	DocumentID       resource.ID
	AdministrationID resource.ID
	// UserID uploaded the document or asked to read it again; the
	// background work acts with their access.
	UserID resource.ID
	State  State
	// Source is where the values came from; empty until read.
	Source Source
	Fields []Field
	// Error tells why reading failed.
	Error    string
	Attempts int
	// NextAttemptAt is when a pending document is read (again).
	NextAttemptAt *time.Time
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

// Has reports whether the field was filled in automatically.
func (r Recognition) Has(name string) bool {
	return slices.ContainsFunc(r.Fields, func(f Field) bool { return f.Name == name })
}

// Drop forgets that the fields were filled in automatically, reporting
// whether any was.
func (r *Recognition) Drop(names ...string) bool {
	n := len(r.Fields)
	r.Fields = slices.DeleteFunc(r.Fields, func(f Field) bool { return slices.Contains(names, f.Name) })
	return len(r.Fields) != n
}
