package products

import (
	"context"
	"testing"
	"time"

	"github.com/adirkuhn/moneyharpy/internal/resource"
)

func TestImportKeepsTheIDAndTimes(t *testing.T) {
	s, store := newService()
	created := time.Date(2024, 3, 1, 10, 0, 0, 0, time.UTC)
	origin := resource.Origin{ID: 504315055158854794, CreatedAt: created}
	p, err := s.Import(context.Background(), 1, 2, origin, Changes{Title: ptr("Hosting"), Price: ptr("10"), LedgerAccountID: ptr(revenue)})
	if err != nil {
		t.Fatal(err)
	}
	if p.ID != origin.ID || !store.products[origin.ID].CreatedAt.Equal(created) {
		t.Errorf("stored %+v", store.products[origin.ID])
	}
}
