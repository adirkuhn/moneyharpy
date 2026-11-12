package history

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/adirkuhn/moneyharpy/internal/resource"
)

const (
	admin         resource.ID = 100
	user          resource.ID = 1
	record, other resource.ID = 10, 11
)

type memStore struct{ events []Event }

func (m *memStore) InsertEvents(_ context.Context, list []Event) error {
	m.events = append(m.events, list...)
	return nil
}

func (m *memStore) EventsOf(_ context.Context, _ resource.ID, entityType string, entities []resource.ID) ([]Event, error) {
	var out []Event
	for _, e := range m.events {
		if e.EntityType == entityType && slices.Contains(entities, e.EntityID) {
			out = append(out, e)
		}
	}
	return out, nil
}

func TestRecordStampsEachEventAndKeepsTheirOrder(t *testing.T) {
	at := time.Date(2026, 11, 12, 9, 0, 0, 0, time.UTC)
	store := &memStore{}
	svc := NewService(store, resource.NewGenerator(1), func() time.Time { return at })
	link := resource.ID(55)
	err := svc.Record(context.Background(),
		Event{AdministrationID: admin, EntityType: "SalesInvoice", EntityID: record, UserID: user, Action: "sales_invoice_created"},
		Event{AdministrationID: admin, EntityType: "SalesInvoice", EntityID: record, UserID: user, Action: "payment_registered", LinkEntityType: "Payment", LinkEntityID: &link},
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(store.events) != 2 {
		t.Fatalf("stored %d events, want 2", len(store.events))
	}
	first, second := store.events[0], store.events[1]
	if first.ID == 0 || second.ID <= first.ID {
		t.Errorf("ids %d, %d: want new increasing ids", first.ID, second.ID)
	}
	if !first.CreatedAt.Equal(at) || !first.UpdatedAt.Equal(at) {
		t.Errorf("first stamped %v / %v, want %v", first.CreatedAt, first.UpdatedAt, at)
	}
	if !second.CreatedAt.After(first.CreatedAt) {
		t.Errorf("events of one moment must keep their order: %v then %v", first.CreatedAt, second.CreatedAt)
	}
	if first.Data == nil {
		t.Error("data should be an empty object, not nil")
	}
}

func TestOfGroupsEventsByRecordOldestFirst(t *testing.T) {
	store := &memStore{}
	svc := NewService(store, resource.NewGenerator(1), time.Now)
	ctx := context.Background()
	store.events = append(store.events,
		Event{AdministrationID: admin, EntityType: "SalesInvoice", EntityID: record, Action: "b", CreatedAt: time.Unix(20, 0)},
		Event{AdministrationID: admin, EntityType: "SalesInvoice", EntityID: record, Action: "a", CreatedAt: time.Unix(10, 0)},
		Event{AdministrationID: admin, EntityType: "SalesInvoice", EntityID: other, Action: "c", CreatedAt: time.Unix(5, 0)},
		Event{AdministrationID: admin, EntityType: "Contact", EntityID: record, Action: "d", CreatedAt: time.Unix(1, 0)},
	)
	got, err := svc.Of(ctx, admin, "SalesInvoice", []resource.ID{record, other})
	if err != nil {
		t.Fatal(err)
	}
	var actions []string
	for _, e := range got[record] {
		actions = append(actions, e.Action)
	}
	if !slices.Equal(actions, []string{"a", "b"}) {
		t.Errorf("record events %v, want [a b]", actions)
	}
	if len(got[other]) != 1 {
		t.Errorf("other has %d events, want 1", len(got[other]))
	}
	if empty, _ := svc.Of(ctx, admin, "SalesInvoice", nil); len(empty) != 0 {
		t.Errorf("no records give %v", empty)
	}
}
