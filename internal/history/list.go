package history

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/adirkuhn/moneyharpy/internal/administration"
	"github.com/adirkuhn/moneyharpy/internal/resource"
)

// Errors of listing events, after Moneybird's events list.
var (
	// ErrNoFilter reports a list asked for without a record type or record.
	ErrNoFilter = errors.New("give entity_type, entity_id or both")
	// ErrUseDocument reports a purchase document type: its events are the
	// Document's.
	ErrUseDocument = errors.New("events of purchase invoices, receipts and other documents have entity_type Document")
	// ErrUnknownType reports a record type that has no events.
	ErrUnknownType = errors.New("unknown entity_type")
	// ErrForbidden reports a record type the user has no permission for.
	ErrForbidden = errors.New("no permission for the events of this record type")
	// ErrBadCursor reports a cursor this list did not make.
	ErrBadCursor = errors.New("invalid cursor")
)

// UnknownTypeError names the record types the user can ask for.
type UnknownTypeError struct{ Allowed []string }

func (e *UnknownTypeError) Error() string {
	return fmt.Sprintf("%v; ask for one of %s", ErrUnknownType, strings.Join(e.Allowed, ", "))
}

func (e *UnknownTypeError) Unwrap() error { return ErrUnknownType }

// typePermissions are the record types with events and the permissions, any
// one of which, that show them.
var typePermissions = map[string][]administration.Permission{
	"SalesInvoice":          {administration.SalesInvoices},
	"ExternalSalesInvoice":  {administration.SalesInvoices},
	"RecurringSalesInvoice": {administration.SalesInvoices},
	"Estimate":              {administration.Estimates},
	"Document":              {administration.Documents},
	"FinancialMutation":     {administration.Bank},
	"Contact":               {administration.Estimates, administration.SalesInvoices, administration.Documents, administration.Bank, administration.Settings},
}

// documentKinds keep their events under Document.
var documentKinds = []string{"PurchaseInvoice", "Receipt", "TypelessDocument", "GeneralJournalDocument", "GeneralDocument"}

// Query selects events of an administration, newest first.
type Query struct {
	// EntityTypes keeps events of these record types; nil keeps all.
	EntityTypes []string
	EntityID    *resource.ID
	// UserID keeps only the events this user did.
	UserID *resource.ID
	// From and To keep the events made from From up to, not including, To.
	From, To *time.Time
	// After continues below the event the cursor points at.
	After *Cursor
	Limit int
}

// Scope works out which events a user may list for Moneybird's
// entity_type and entity_id: a record type needs its permission, and a
// record alone shows the events of the types the user may see. A user with
// only the time_entries permission sees only their own events. With a
// period (a MoneyHarpy extension) no record filter is needed: the list
// holds the events of every type the user may see.
func Scope(entityType string, entityID *resource.ID, period bool, user resource.ID, perms []administration.Permission, can func(...administration.Permission) bool) (Query, error) {
	if entityType == "" && entityID == nil && !period {
		return Query{}, ErrNoFilter
	}
	var allowed []string
	for t, need := range typePermissions {
		if can(need...) {
			allowed = append(allowed, t)
		}
	}
	slices.Sort(allowed)
	q := Query{EntityID: entityID}
	if slices.Equal(perms, []administration.Permission{administration.TimeEntries}) {
		q.UserID = &user
		if entityType != "" {
			return Query{}, ErrForbidden
		}
		return q, nil
	}
	switch {
	case entityType == "":
		q.EntityTypes = allowed
	case slices.Contains(documentKinds, entityType):
		return Query{}, ErrUseDocument
	case typePermissions[entityType] == nil:
		return Query{}, &UnknownTypeError{Allowed: allowed}
	case !slices.Contains(allowed, entityType):
		return Query{}, ErrForbidden
	default:
		q.EntityTypes = []string{entityType}
	}
	return q, nil
}

// Cursor marks an event in the list, which goes down by creation time and id.
type Cursor struct {
	At time.Time
	ID resource.ID
}

func cursorOf(e Event) Cursor { return Cursor{At: e.CreatedAt, ID: e.ID} }

func (c Cursor) compare(o Cursor) int {
	if n := c.At.Compare(o.At); n != 0 {
		return n
	}
	switch {
	case c.ID < o.ID:
		return -1
	case c.ID > o.ID:
		return 1
	}
	return 0
}

// before reports whether e comes after the cursor in the list, so is older.
func (c Cursor) before(e Event) bool { return cursorOf(e).compare(c) < 0 }

// String encodes the cursor for the next page's address.
func (c Cursor) String() string {
	return base64.RawURLEncoding.EncodeToString([]byte(strconv.FormatInt(c.At.UnixMicro(), 10) + ":" + c.ID.String()))
}

// ParseCursor reads a cursor String made.
func ParseCursor(s string) (Cursor, error) {
	raw, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil {
		return Cursor{}, ErrBadCursor
	}
	at, id, ok := strings.Cut(string(raw), ":")
	micros, err := strconv.ParseInt(at, 10, 64)
	if !ok || err != nil || micros < 0 {
		return Cursor{}, ErrBadCursor
	}
	rid, err := resource.ParseID(id)
	if err != nil || rid == 0 {
		return Cursor{}, ErrBadCursor
	}
	return Cursor{At: time.UnixMicro(micros).UTC(), ID: rid}, nil
}

// List returns a page of the administration's events, newest first, and
// the cursor of the next page when there is one.
func (s *Service) List(ctx context.Context, admin resource.ID, q Query) ([]Event, *Cursor, error) {
	if q.Limit < 1 {
		q.Limit = 1
	}
	limit := q.Limit
	q.Limit++
	list, err := s.store.ListEvents(ctx, admin, q)
	if err != nil {
		return nil, nil, fmt.Errorf("list events: %w", err)
	}
	if len(list) <= limit {
		return list, nil, nil
	}
	list = list[:limit]
	next := cursorOf(list[limit-1])
	return list, &next, nil
}
