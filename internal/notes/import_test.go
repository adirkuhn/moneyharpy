package notes

import (
	"context"
	"testing"
	"time"

	"github.com/adirkuhn/moneyharpy/internal/resource"
)

func TestImportKeepsTheIDTimesAndCompletion(t *testing.T) {
	s, store := newService()
	created := time.Date(2024, 3, 1, 10, 0, 0, 0, time.UTC)
	done := created.Add(48 * time.Hour)
	n, err := s.Import(context.Background(), 1, "Contact", 7, owner, Imported{
		Origin: resource.Origin{ID: 9001, CreatedAt: created}, Input: Input{Note: "Call back", Todo: true}, CompletedAt: &done,
	})
	if err != nil {
		t.Fatal(err)
	}
	got := store.notes[0]
	if n.ID != 9001 || !got.CreatedAt.Equal(created) || got.CompletedAt == nil || !got.CompletedAt.Equal(done) || *got.CompletedByID != owner {
		t.Errorf("stored %+v", got)
	}
	if _, err := s.Import(context.Background(), 1, "Contact", 7, owner, Imported{Origin: resource.Origin{ID: 9002}}); err == nil {
		t.Error("an empty note came in")
	}
}
