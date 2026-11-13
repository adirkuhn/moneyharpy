package postgres_test

import (
	"context"
	"testing"
	"time"

	"github.com/adirkuhn/moneyharpy/internal/bookkeeping"
	"github.com/adirkuhn/moneyharpy/internal/resource"
)

func TestDetailUsageCountsLinesOfOneKindSinceADay(t *testing.T) {
	ctx := context.Background()
	svc, store, admin, owner := bookkeepingService(t)
	rates, err := svc.TaxRates(ctx, admin, owner, bookkeeping.TaxRateFilter{})
	if err != nil {
		t.Fatal(err)
	}
	accounts, err := store.LedgerAccounts(ctx, admin)
	if err != nil {
		t.Fatal(err)
	}
	r1, r2, cat := rates[0].ID, rates[1].ID, accounts[0].ID
	exec := func(sql string, args ...any) {
		t.Helper()
		if _, err := testPool.Exec(ctx, sql, args...); err != nil {
			t.Fatal(err)
		}
	}
	now := time.Now()
	purchase := func(day string, lines ...[2]any) {
		doc := ids.Next()
		exec(`INSERT INTO purchase_documents (id, administration_id, kind, entry_number, date, currency, state, created_at, updated_at)
		      VALUES ($1, $2, 'PurchaseInvoice', 1, $3, 'EUR', 'open', $4, $4)`, int64(doc), int64(admin), day, now)
		for _, l := range lines {
			exec(`INSERT INTO purchase_document_details (id, administration_id, purchase_document_id, tax_rate_id, ledger_account_id, created_at, updated_at)
			      VALUES ($1, $2, $3, $4, $5, $6, $6)`, int64(ids.Next()), int64(admin), int64(doc), int64(l[0].(resource.ID)), l[1], now)
		}
	}
	purchase("2026-03-01", [2]any{r1, int64(cat)}, [2]any{r2, int64(cat)}, [2]any{r2, nil})
	// Too long ago to count.
	purchase("2025-01-01", [2]any{r1, int64(cat)}, [2]any{r1, int64(cat)}, [2]any{r1, int64(cat)})

	contact := ids.Next()
	exec(`INSERT INTO contacts (id, administration_id, country, customer_id, created_at, updated_at) VALUES ($1, $2, 'NL', '1', $3, $3)`, int64(contact), int64(admin), now)
	invoice := ids.Next()
	exec(`INSERT INTO sales_invoices (id, administration_id, contact_id, identity_id, draft_id, language, currency, created_at, updated_at)
	      VALUES ($1, $2, $3, 0, 1, 'en', 'EUR', $4, $4)`, int64(invoice), int64(admin), int64(contact), time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC))
	exec(`INSERT INTO sales_invoice_details (id, administration_id, sales_invoice_id, tax_rate_id, created_at, updated_at)
	      VALUES ($1, $2, $3, $4, $5, $5)`, int64(ids.Next()), int64(admin), int64(invoice), int64(r1), now)

	since := time.Date(2025, 11, 1, 0, 0, 0, 0, time.UTC)
	got, err := store.DetailUsage(ctx, admin, "purchase", since)
	if err != nil {
		t.Fatal(err)
	}
	want := bookkeeping.Usage{
		TaxRates:       []bookkeeping.UsageCount{{ID: r2, Count: 2}, {ID: r1, Count: 1}},
		LedgerAccounts: []bookkeeping.UsageCount{{ID: cat, Count: 2}},
	}
	if !equalUsage(got, want) {
		t.Errorf("purchase usage = %+v, want %+v", got, want)
	}
	// A draft invoice without a date counts by the day it was made.
	got, err = store.DetailUsage(ctx, admin, "sales", since)
	if err != nil {
		t.Fatal(err)
	}
	if !equalUsage(got, bookkeeping.Usage{TaxRates: []bookkeeping.UsageCount{{ID: r1, Count: 1}}}) {
		t.Errorf("sales usage = %+v", got)
	}
}

func equalUsage(a, b bookkeeping.Usage) bool {
	eq := func(x, y []bookkeeping.UsageCount) bool {
		if len(x) != len(y) {
			return false
		}
		for i := range x {
			if x[i] != y[i] {
				return false
			}
		}
		return true
	}
	return eq(a.TaxRates, b.TaxRates) && eq(a.LedgerAccounts, b.LedgerAccounts)
}
