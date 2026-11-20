package projects

import (
	"context"
	"testing"
	"time"

	"github.com/adirkuhn/moneyharpy/internal/resource"
)

func TestImportKeepsTheIDTimesAndState(t *testing.T) {
	s, store := newService(nil)
	created := time.Date(2024, 3, 1, 10, 0, 0, 0, time.UTC)
	origin := resource.Origin{ID: 504315055158854794, CreatedAt: created}
	p, err := s.Import(context.Background(), 1, 2, origin, Changes{Name: ptr("Website"), State: ptr(Archived)})
	if err != nil {
		t.Fatal(err)
	}
	got := store.projects[origin.ID]
	if p.ID != origin.ID || got.State != Archived || !got.CreatedAt.Equal(created) {
		t.Errorf("stored %+v", got)
	}
}
