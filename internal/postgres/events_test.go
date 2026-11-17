package postgres_test

import (
	"context"
	"testing"
	"time"

	"github.com/adirkuhn/moneyharpy/internal/history"
	"github.com/adirkuhn/moneyharpy/internal/postgres"
	"github.com/adirkuhn/moneyharpy/internal/resource"
)

func TestListEventsNewestFirstWithFilters(t *testing.T) {
	ctx := context.Background()
	a, _, owner := newAdministrationWithIdentity(t)
	store := postgres.NewEventStore(testPool)
	at := time.Date(2026, 11, 17, 9, 0, 0, 0, time.UTC)
	ids := resource.NewGenerator(7)
	record := ids.Next()
	var events []history.Event
	for i, kind := range []string{"Contact", "SalesInvoice", "Contact", "Estimate"} {
		e := history.Event{ID: ids.Next(), AdministrationID: a.ID, EntityType: kind, EntityID: record, Action: "x", Data: map[string]any{}, CreatedAt: at.Add(time.Duration(i) * time.Minute), UpdatedAt: at}
		if i == 2 {
			e.UserID = owner
		}
		events = append(events, e)
	}
	if err := store.InsertEvents(ctx, events); err != nil {
		t.Fatal(err)
	}
	all, err := store.ListEvents(ctx, a.ID, history.Query{EntityID: &record, Limit: 10})
	if err != nil || len(all) != 4 || all[0].EntityType != "Estimate" || all[3].EntityType != "Contact" || !all[3].CreatedAt.Equal(at) {
		t.Fatalf("ListEvents() = %+v, %v", all, err)
	}
	contacts, _ := store.ListEvents(ctx, a.ID, history.Query{EntityTypes: []string{"Contact"}, Limit: 10})
	if len(contacts) != 2 {
		t.Errorf("contacts = %d events, want 2", len(contacts))
	}
	own, _ := store.ListEvents(ctx, a.ID, history.Query{EntityID: &record, UserID: &owner, Limit: 10})
	if len(own) != 1 || own[0].UserID != owner {
		t.Errorf("own = %+v", own)
	}
	none, _ := store.ListEvents(ctx, a.ID, history.Query{EntityTypes: []string{}, EntityID: &record, Limit: 10})
	if len(none) != 0 {
		t.Errorf("no allowed types listed %d events", len(none))
	}
	after := history.Cursor{At: all[1].CreatedAt, ID: all[1].ID}
	older, _ := store.ListEvents(ctx, a.ID, history.Query{EntityID: &record, After: &after, Limit: 1})
	if len(older) != 1 || older[0].ID != all[2].ID {
		t.Errorf("after the second = %+v, want only the third", older)
	}
}
