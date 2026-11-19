package history

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/adirkuhn/moneyharpy/internal/administration"
	"github.com/adirkuhn/moneyharpy/internal/resource"
)

func (m *memStore) ListEvents(_ context.Context, a resource.ID, q Query) ([]Event, error) {
	var out []Event
	for _, e := range m.events {
		if e.AdministrationID != a || (q.EntityTypes != nil && !slices.Contains(q.EntityTypes, e.EntityType)) ||
			(q.EntityID != nil && e.EntityID != *q.EntityID) || (q.UserID != nil && e.UserID != *q.UserID) {
			continue
		}
		if q.After != nil && !q.After.before(e) {
			continue
		}
		out = append(out, e)
	}
	slices.SortFunc(out, func(a, b Event) int { return cursorOf(b).compare(cursorOf(a)) })
	if len(out) > q.Limit {
		out = out[:q.Limit]
	}
	return out, nil
}

func TestListPagesNewestFirst(t *testing.T) {
	at := time.Date(2026, 11, 12, 9, 0, 0, 0, time.UTC)
	store := &memStore{}
	svc := NewService(store, resource.NewGenerator(1), func() time.Time { return at })
	ctx := context.Background()
	for i := range 5 {
		svc.now = func() time.Time { return at.Add(time.Duration(i) * time.Minute) }
		if err := svc.Record(ctx, Event{AdministrationID: admin, EntityType: "Contact", EntityID: record, UserID: user, Action: "contact_updated"}); err != nil {
			t.Fatal(err)
		}
	}
	if err := svc.Record(ctx, Event{AdministrationID: admin, EntityType: "Contact", EntityID: other, Action: "contact_created"}); err != nil {
		t.Fatal(err)
	}

	page, next, err := svc.List(ctx, admin, Query{EntityID: ptr(record), Limit: 2})
	if err != nil || len(page) != 2 || next == nil || !page[0].CreatedAt.Equal(at.Add(4*time.Minute)) || !page[1].CreatedAt.Equal(at.Add(3*time.Minute)) {
		t.Fatalf("first page = %+v, next %v, %v", page, next, err)
	}
	cursor, err := ParseCursor(next.String())
	if err != nil || !cursor.At.Equal(next.At) || cursor.ID != next.ID {
		t.Fatalf("cursor %q reads back as %v, %v", next.String(), cursor, err)
	}
	seen := append([]Event{}, page...)
	for next != nil {
		page, next, err = svc.List(ctx, admin, Query{EntityID: ptr(record), Limit: 2, After: next})
		if err != nil {
			t.Fatal(err)
		}
		seen = append(seen, page...)
	}
	if len(seen) != 5 || !seen[4].CreatedAt.Equal(at) {
		t.Errorf("paged through %d events, want all 5 of the record down to the oldest", len(seen))
	}
}

func TestParseCursorRefusesWhatItDidNotMake(t *testing.T) {
	for _, s := range []string{"", "abc", "MTIzOmFiYw", "LTE6Mg", "MTow"} {
		if _, err := ParseCursor(s); !errors.Is(err, ErrBadCursor) {
			t.Errorf("ParseCursor(%q) = %v, want ErrBadCursor", s, err)
		}
	}
}

func ptr[T any](v T) *T { return &v }

func can(perms ...administration.Permission) func(...administration.Permission) bool {
	return func(want ...administration.Permission) bool {
		for _, p := range want {
			if slices.Contains(perms, p) {
				return true
			}
		}
		return false
	}
}

func TestScope(t *testing.T) {
	id := resource.ID(42)
	for name, tt := range map[string]struct {
		entityType string
		entityID   *resource.ID
		perms      []administration.Permission
		want       Query
		err        error
	}{
		"neither":                  {perms: administration.Permissions, err: ErrNoFilter},
		"sales invoices":           {entityType: "SalesInvoice", perms: []administration.Permission{administration.SalesInvoices}, want: Query{EntityTypes: []string{"SalesInvoice"}}},
		"sales invoices, no right": {entityType: "SalesInvoice", perms: []administration.Permission{administration.Documents}, err: ErrForbidden},
		"contact through bank":     {entityType: "Contact", perms: []administration.Permission{administration.Bank}, want: Query{EntityTypes: []string{"Contact"}}},
		"contact, time entries":    {entityType: "Contact", perms: []administration.Permission{administration.TimeEntries}, err: ErrForbidden},
		"receipt is a Document":    {entityType: "Receipt", perms: administration.Permissions, err: ErrUseDocument},
		"unknown":                  {entityType: "Spaceship", perms: administration.Permissions, err: ErrUnknownType},
		"record of allowed types":  {entityID: &id, perms: []administration.Permission{administration.Estimates}, want: Query{EntityTypes: []string{"Contact", "Estimate"}, EntityID: &id}},
		"time entries only: own":   {entityID: &id, perms: []administration.Permission{administration.TimeEntries}, want: Query{EntityID: &id, UserID: ptr(user)}},
		"bank statements":          {entityType: "FinancialStatement", perms: []administration.Permission{administration.Bank}, want: Query{EntityTypes: []string{"FinancialStatement"}}},
		"webhooks need settings":   {entityType: "Webhook", perms: []administration.Permission{administration.Bank}, err: ErrForbidden},
		"time entries":             {entityType: "TimeEntry", perms: []administration.Permission{administration.TimeEntries, administration.Estimates}, want: Query{EntityTypes: []string{"TimeEntry"}}},
		"type and record":          {entityType: "Estimate", entityID: &id, perms: []administration.Permission{administration.Estimates}, want: Query{EntityTypes: []string{"Estimate"}, EntityID: &id}},
	} {
		t.Run(name, func(t *testing.T) {
			got, err := Scope(tt.entityType, tt.entityID, false, user, tt.perms, can(tt.perms...))
			if !errors.Is(err, tt.err) {
				t.Fatalf("Scope() error = %v, want %v", err, tt.err)
			}
			if err != nil {
				return
			}
			slices.Sort(got.EntityTypes)
			if !slices.Equal(got.EntityTypes, tt.want.EntityTypes) || (got.EntityID == nil) != (tt.want.EntityID == nil) || (got.UserID == nil) != (tt.want.UserID == nil) ||
				(got.UserID != nil && *got.UserID != *tt.want.UserID) {
				t.Errorf("Scope() = %+v, want %+v", got, tt.want)
			}
		})
	}
	var unknown *UnknownTypeError
	_, err := Scope("Spaceship", nil, false, user, []administration.Permission{administration.Estimates}, can(administration.Estimates))
	if !errors.As(err, &unknown) || !slices.Equal(unknown.Allowed, []string{"Contact", "Estimate"}) {
		t.Errorf("unknown type error = %v, want the types the user can ask for", err)
	}
}

func TestScopeWithAPeriod(t *testing.T) {
	perms := []administration.Permission{administration.SalesInvoices}
	if _, err := Scope("", nil, false, 1, perms, can(perms...)); !errors.Is(err, ErrNoFilter) {
		t.Errorf("without a filter: %v, want ErrNoFilter", err)
	}
	q, err := Scope("", nil, true, 1, perms, can(perms...))
	if err != nil || q.EntityTypes == nil {
		t.Errorf("with a period: %+v, %v; want the types the user may see", q, err)
	}
}
