package postgres_test

import (
	"context"
	"sync"
	"testing"

	"github.com/adirkuhn/moneyharpy/internal/postgres"
)

func TestReserveInvoiceNumbers(t *testing.T) {
	ctx := context.Background()
	a, _, _ := newAdministrationWithIdentity(t)
	store := postgres.NewSalesStore(testPool)
	first, err := store.ReserveNumber(ctx, a.ID)
	if err != nil || first != "0001" {
		t.Fatalf("first = %q, %v", first, err)
	}
	var mu sync.Mutex
	seen := map[string]bool{}
	var wg sync.WaitGroup
	for range 20 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			n, err := store.ReserveNumber(ctx, a.ID)
			if err != nil {
				t.Error(err)
				return
			}
			mu.Lock()
			defer mu.Unlock()
			if seen[n] {
				t.Errorf("number %s given twice", n)
			}
			seen[n] = true
		}()
	}
	wg.Wait()
	next, _ := store.ReserveNumber(ctx, a.ID)
	if next != "0022" || len(seen) != 20 {
		t.Errorf("next = %q after %d", next, len(seen))
	}
}
