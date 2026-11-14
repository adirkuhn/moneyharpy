package postgres_test

import (
	"context"
	"testing"

	"github.com/adirkuhn/moneyharpy/internal/postgres"
	"github.com/adirkuhn/moneyharpy/internal/postgres/postgrestest"
	"github.com/adirkuhn/moneyharpy/internal/recognize"
	"github.com/adirkuhn/moneyharpy/internal/resource"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Supplier memories were one per supplier; they become one per kind of
// document, kept for the kind they were learned from: the kind of the
// supplier's booked documents when all of one kind, else both.
func TestSupplierMemoryMigrationKeepsWhatWasLearnedPerKind(t *testing.T) {
	ctx := context.Background()
	url, cleanup, err := postgrestest.StartEmpty(ctx)
	if err != nil {
		t.Fatalf("StartEmpty() error = %v", err)
	}
	defer cleanup()
	if err := postgres.MigrateTo(ctx, url, 31); err != nil {
		t.Fatalf("MigrateTo(31) error = %v", err)
	}
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer pool.Close()

	const admin int64 = 1 << 42
	// Suppliers of receipts only, of both kinds, and of nothing booked.
	receipts, both, unbooked := admin|1, admin|2, admin|3
	exec := func(sql string, args ...any) {
		t.Helper()
		if _, err := pool.Exec(ctx, sql, args...); err != nil {
			t.Fatalf("%s: %v", sql, err)
		}
	}
	exec(`INSERT INTO administrations (id, name, language, currency, country, time_zone, period_start_date)
		VALUES ($1, 'Example Shop B.V.', 'nl', 'EUR', 'NL', 'Europe/Amsterdam', '2026-01-01')`, admin)
	for i, c := range []int64{receipts, both, unbooked} {
		exec(`INSERT INTO contacts (id, administration_id, country, customer_id, created_at, updated_at) VALUES ($1, $2, 'NL', $3, now(), now())`,
			c, admin, string(rune('1'+i)))
		exec(`INSERT INTO supplier_memories (administration_id, contact_id, document_kind, document_kind_uses, currency, currency_uses,
			prices_are_incl_tax, prices_are_incl_tax_uses, updated_at) VALUES ($1, $2, 'receipt', 2, 'EUR', 2, true, 1, '2026-10-01')`, admin, c)
		exec(`INSERT INTO supplier_memory_lines (administration_id, contact_id, description, ledger_account_id, tax_rate_id, uses, last_used_at)
			VALUES ($1, $2, '', NULL, NULL, 2, '2026-10-01'), ($1, $2, 'coffee', NULL, NULL, 1, '2026-10-01')`, admin, c)
	}
	doc := admin | 100
	for _, d := range []struct {
		contact     int64
		kind, state string
	}{
		{receipts, "Receipt", "open"}, {receipts, "PurchaseInvoice", "new"},
		{both, "Receipt", "paid"}, {both, "PurchaseInvoice", "open"},
		{unbooked, "PurchaseInvoice", "new"},
	} {
		doc++
		exec(`INSERT INTO purchase_documents (id, administration_id, kind, contact_id, entry_number, date, state, currency, created_at, updated_at)
			VALUES ($1, $2, $3, $4, $5, '2026-10-01', $6, 'EUR', now(), now())`, doc, admin, d.kind, d.contact, doc-admin, d.state)
	}

	if err := postgres.Migrate(ctx, url); err != nil {
		t.Fatalf("Migrate() error = %v", err)
	}

	store := postgres.NewRecognitionStore(pool)
	for _, tc := range []struct {
		contact  int64
		invoices bool
		receipts bool
	}{{receipts, false, true}, {both, true, true}, {unbooked, true, true}} {
		m, err := store.Memory(ctx, resource.ID(admin), resource.ID(tc.contact))
		if err != nil {
			t.Fatal(err)
		}
		if m.Kind.V != recognize.KindReceipt || m.Kind.Uses != 2 {
			t.Errorf("contact %d: kind = %+v", tc.contact, m.Kind)
		}
		for kind, want := range map[recognize.Kind]bool{recognize.KindPurchaseInvoice: tc.invoices, recognize.KindReceipt: tc.receipts} {
			k := *m.Of(kind)
			if want && (k.Currency.V != "EUR" || k.Currency.Uses != 2 || !k.PricesInclTax.V || len(k.Lines) != 2 || k.Lines[""].Uses != 2) {
				t.Errorf("contact %d: %s = %+v, want what was learned", tc.contact, kind, k)
			}
			if !want && !k.Empty() {
				t.Errorf("contact %d: %s = %+v, want nothing", tc.contact, kind, k)
			}
		}
	}

	// Down and up again keeps a memory per supplier.
	if err := postgres.MigrateDownTo(ctx, url, 31); err != nil {
		t.Fatalf("MigrateDownTo(31) error = %v", err)
	}
	var lines int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM supplier_memory_lines WHERE contact_id = $1`, both).Scan(&lines); err != nil || lines != 2 {
		t.Errorf("lines after down = %d, %v", lines, err)
	}
	if err := postgres.Migrate(ctx, url); err != nil {
		t.Fatalf("Migrate() again error = %v", err)
	}
}
