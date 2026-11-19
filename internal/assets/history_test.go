package assets

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/adirkuhn/moneyharpy/internal/history/historytest"
	"github.com/adirkuhn/moneyharpy/internal/notes"
	"github.com/adirkuhn/moneyharpy/internal/resource"
)

// fakeNotes keeps notes in memory.
type fakeNotes struct{ list []notes.Note }

func (f *fakeNotes) Add(_ context.Context, admin resource.ID, entityType string, entity, user resource.ID, in notes.Input) (notes.Note, error) {
	n := notes.Note{ID: resource.ID(900 + len(f.list)), AdministrationID: admin, EntityType: entityType, EntityID: entity, UserID: user, Note: in.Note, Todo: in.Todo}
	f.list = append(f.list, n)
	return n, nil
}

func (f *fakeNotes) Delete(_ context.Context, _ resource.ID, entityType string, entity, id resource.ID) error {
	for i, n := range f.list {
		if n.ID == id && n.EntityID == entity && n.EntityType == entityType {
			f.list = slices.Delete(f.list, i, i+1)
			return nil
		}
	}
	return notes.ErrNotFound
}

func (f *fakeNotes) Of(_ context.Context, _ resource.ID, entityType string, entities []resource.ID) (map[resource.ID][]notes.Note, error) {
	out := map[resource.ID][]notes.Note{}
	for _, n := range f.list {
		if n.EntityType == entityType && slices.Contains(entities, n.EntityID) {
			out[n.EntityID] = append(out[n.EntityID], n)
		}
	}
	return out, nil
}

func TestAssetsKeepTheirHistoryAndNotes(t *testing.T) {
	h := newHarness()
	log := &historytest.Log{}
	kept := &fakeNotes{}
	h.svc.UseHistory(log)
	h.svc.UseNotes(kept)

	a := h.create(t, input("Laptop", "2026-11-01", "1000", 5, "0"))
	if _, err := h.svc.Update(ctx, adminID, a.ID, owner, Input{Name: ptr("Laptop 2")}); err != nil {
		t.Fatal(err)
	}
	src, err := h.svc.AddSource(ctx, adminID, a.ID, owner, SourceInput{DetailID: ptr(purchaseLine)})
	if err != nil {
		t.Fatal(err)
	}
	if err := h.svc.DeleteSource(ctx, adminID, a.ID, src.ID, owner); err != nil {
		t.Fatal(err)
	}
	if _, err := h.svc.Dispose(ctx, adminID, a.ID, owner, Divestment, day("2026-11-10"), nil); err != nil {
		t.Fatal(err)
	}
	want := []string{"asset_created", "asset_updated", "asset_source_created", "asset_source_destroyed", "asset_value_change_created", "asset_disposal_created"}
	if got := log.Actions(); !slices.Equal(got, want) {
		t.Errorf("actions = %v, want %v", got, want)
	}
	if e := log.Events[4]; e.EntityType != "Asset" || e.EntityID != a.ID || e.UserID != owner || e.Data["type"] != Divestment {
		t.Errorf("value change event = %+v", e)
	}
	if e := log.Events[2]; e.Data["kind"] != "detail" {
		t.Errorf("source event data = %v", e.Data)
	}

	n, err := h.svc.AddNote(ctx, adminID, a.ID, owner, notes.Input{Note: "Kept in the office"})
	if err != nil || n.EntityType != "Asset" {
		t.Fatalf("note = %+v, %v", n, err)
	}
	got, err := h.svc.Asset(ctx, adminID, a.ID, owner)
	if err != nil || len(got.Notes) != 1 || len(got.Events) != len(want) {
		t.Fatalf("asset notes %d, events %d, %v", len(got.Notes), len(got.Events), err)
	}
	if err := h.svc.DeleteNote(ctx, adminID, a.ID, n.ID, owner); err != nil {
		t.Fatal(err)
	}
	if err := h.svc.DeleteNote(ctx, adminID, a.ID, n.ID, owner); !errors.Is(err, notes.ErrNotFound) {
		t.Errorf("deleted twice: %v", err)
	}
	if _, err := h.svc.AddNote(ctx, adminID, a.ID, stranger, notes.Input{Note: "x"}); err == nil {
		t.Error("a stranger added a note")
	}
	if err := h.svc.Delete(ctx, adminID, a.ID, owner); err != nil {
		t.Fatal(err)
	}
	if last := log.Actions()[len(log.Actions())-1]; last != "asset_destroyed" {
		t.Errorf("last action = %s", last)
	}
}
