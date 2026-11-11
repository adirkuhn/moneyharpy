// Package resource identifies and locates every entity MoneyHarpy stores.
//
// Each entity has one globally unique ID and one canonical URI, so any of them
// can be linked to and opened directly.
package resource

import (
	"errors"
	"fmt"
	"maps"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"
)

// ID globally identifies an entity. IDs exceed the safe integer range of
// JavaScript, so they always cross the API as strings.
type ID int64

// String returns the decimal form used in URIs and API payloads.
func (id ID) String() string { return strconv.FormatInt(int64(id), 10) }

// Errors returned when identifying resources.
var (
	ErrInvalidID = errors.New("invalid resource id")
	ErrNotFound  = errors.New("resource not found")
)

// ParseID parses the decimal form of an ID.
func ParseID(s string) (ID, error) {
	if s == "" || s[0] == '+' {
		return 0, fmt.Errorf("%w: %q", ErrInvalidID, s)
	}
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil || n <= 0 {
		return 0, fmt.Errorf("%w: %q", ErrInvalidID, s)
	}
	return ID(n), nil
}

const (
	epochMillis  = 1767225600000 // 2026-01-01T00:00:00Z
	sequenceBits = 12
	nodeBits     = 10
	maxSequence  = 1<<sequenceBits - 1
	maxNode      = 1<<nodeBits - 1
)

// Generator issues time-ordered IDs that are unique per node.
type Generator struct {
	node int64
	now  func() time.Time

	mu       sync.Mutex
	last     int64
	sequence int64
}

// NewGenerator returns a Generator for the given node, which must be unique
// among processes writing to the same database. Only the low 10 bits are used.
func NewGenerator(node int64) *Generator {
	return &Generator{node: node & maxNode, now: time.Now}
}

// Next returns a new ID. A clock that moves backwards never produces a
// duplicate: the generator keeps counting from the latest time it saw.
func (g *Generator) Next() ID {
	g.mu.Lock()
	defer g.mu.Unlock()

	millis := max(g.now().UnixMilli()-epochMillis, g.last)
	if millis == g.last {
		g.sequence = (g.sequence + 1) & maxSequence
		if g.sequence == 0 {
			millis++
		}
	} else {
		g.sequence = 0
	}
	g.last = millis
	return ID(millis<<(sequenceBits+nodeBits) | g.node<<sequenceBits | g.sequence)
}

// Type names a kind of resource, as it appears in URIs.
type Type string

// The types of resource.
const (
	TypeAdministration Type = "administrations"
	TypeIdentity       Type = "identities"
	TypeCustomField    Type = "custom_fields"
	TypeWorkflow       Type = "workflows"
	TypeDocumentStyle  Type = "document_styles"
	TypeTaxRate        Type = "tax_rates"
	TypeLedgerAccount  Type = "ledger_accounts"
	// Banking.
	TypeFinancialAccount   Type = "financial_accounts"
	TypeFinancialStatement Type = "financial_statements"
	TypeFinancialMutation  Type = "financial_mutations"
	TypeBookingRule        Type = "booking_rules"
	TypeWebhook            Type = "webhooks"
	// Contacts.
	TypeContact Type = "contacts"
	// Sales.
	TypeSalesInvoice          Type = "sales_invoices"
	TypeExternalSalesInvoice  Type = "external_sales_invoices"
	TypeRecurringSalesInvoice Type = "recurring_sales_invoices"
	// Journal.
	TypeGeneralJournalDocument Type = "documents/general_journal_documents"
)

// Ref locates one resource.
type Ref struct {
	ID               ID
	Type             Type
	AdministrationID ID
}

// URI returns the canonical path of the resource in the web application. The
// JSON API serves the same resource at the same path below /api/v2.
func (r Ref) URI() string {
	if r.Type == TypeAdministration {
		return "/" + r.AdministrationID.String()
	}
	return "/" + r.AdministrationID.String() + "/" + string(r.Type) + "/" + r.ID.String()
}

// Permalink returns the type-independent path that redirects to a resource.
func Permalink(id ID) string { return "/r/" + id.String() }

// ValidationError reports which fields of a resource are invalid and why.
type ValidationError struct {
	Fields map[string][]string
}

// Add records a problem with field.
func (e *ValidationError) Add(field, problem string) {
	if e.Fields == nil {
		e.Fields = map[string][]string{}
	}
	e.Fields[field] = append(e.Fields[field], problem)
}

// OrNil returns e if it recorded any problem and nil otherwise.
func (e *ValidationError) OrNil() error {
	if len(e.Fields) == 0 {
		return nil
	}
	return e
}

// Error lists the invalid fields in a stable order.
func (e *ValidationError) Error() string {
	return "invalid fields: " + strings.Join(slices.Sorted(maps.Keys(e.Fields)), ", ")
}
