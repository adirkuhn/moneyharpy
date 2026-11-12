package postgres_test

import (
	"context"
	"testing"
	"time"

	"github.com/adirkuhn/moneyharpy/internal/postgres"
	"github.com/adirkuhn/moneyharpy/internal/postgres/postgrestest"
	"github.com/google/go-cmp/cmp"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Workflow steps used to hold, for a reminder, the days after the step
// before it; they take Moneybird's meaning, where each step expires after
// its own due interval and the next reminder is due then.
func TestReminderTimingMigrationTakesMoneybirdsMeaning(t *testing.T) {
	ctx := context.Background()
	url, cleanup, err := postgrestest.StartEmpty(ctx)
	if err != nil {
		t.Fatalf("StartEmpty() error = %v", err)
	}
	defer cleanup()
	if err := postgres.MigrateTo(ctx, url, 28); err != nil {
		t.Fatalf("MigrateTo(28) error = %v", err)
	}
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer pool.Close()

	const admin, contact, flow int64 = 1 << 41, 1<<41 | 1, 1<<41 | 2
	exec := func(sql string, args ...any) {
		t.Helper()
		if _, err := pool.Exec(ctx, sql, args...); err != nil {
			t.Fatalf("%s: %v", sql, err)
		}
	}
	exec(`INSERT INTO administrations (id, name, language, currency, country, time_zone, period_start_date)
		VALUES ($1, 'Harpy', 'nl', 'EUR', 'NL', 'Europe/Amsterdam', '2026-01-01')`, admin)
	exec(`INSERT INTO contacts (id, administration_id, country, customer_id, created_at, updated_at) VALUES ($1, $2, 'NL', '1', now(), now())`, contact, admin)
	exec(`INSERT INTO workflows (id, administration_id, type, name, currency, language, created_at, updated_at)
		VALUES ($1, $2, 'InvoiceWorkflow', 'Standaard', 'EUR', 'nl', now(), now())`, flow, admin)
	// Pay in 14 days; reminders 3, 10 and 5 days after the step before.
	for i, st := range [][2]int{{14, 0}, {0, 3}, {0, 10}, {0, 5}} {
		exec(`INSERT INTO workflow_steps (id, workflow_id, step_order, due_interval, reminder_delay, created_at, updated_at)
			VALUES ($1, $2, $3, $4, $5, now(), now())`, flow+10+int64(i), flow, i+1, st[0], st[1])
	}
	// One invoice reminded once (its reminder event late in the evening,
	// Amsterdam time), one reminded twice without events, one never.
	for i, count := range []int{1, 2, 0} {
		id := flow + 100 + int64(i)
		exec(`INSERT INTO sales_invoices (id, administration_id, contact_id, identity_id, workflow_id, draft_id, language, currency,
			invoice_date, due_date, state, reminder_count, created_at, updated_at)
			VALUES ($1, $2, $3, 0, $4, $5, 'nl', 'EUR', '2026-10-01', '2026-10-15', $6, $7, now(), now())`,
			id, admin, contact, flow, i+1, map[bool]string{true: "reminded", false: "open"}[count > 0], count)
	}
	exec(`INSERT INTO events (id, administration_id, entity_type, entity_id, action, created_at, updated_at)
		VALUES ($1, $2, 'SalesInvoice', $3, 'sales_invoice_send_reminder_email', '2026-10-17T22:30:00Z', now())`, flow+200, admin, flow+100)

	if err := postgres.Migrate(ctx, url); err != nil {
		t.Fatalf("Migrate() error = %v", err)
	}

	rows, err := pool.Query(ctx, `SELECT step_order, due_interval, reminder_delay FROM workflow_steps WHERE workflow_id = $1 ORDER BY step_order`, flow)
	if err != nil {
		t.Fatal(err)
	}
	var steps [][3]int
	for rows.Next() {
		var s [3]int
		if err := rows.Scan(&s[0], &s[1], &s[2]); err != nil {
			t.Fatal(err)
		}
		steps = append(steps, s)
	}
	// Each reminder now expires after the gap before the next one; the last
	// keeps its own gap. Moneybird's page shows Day 1, 15, 25 and 30.
	if diff := cmp.Diff([][3]int{{0, 14, 0}, {1, 10, 0}, {2, 5, 0}, {3, 5, 0}}, steps); diff != "" {
		t.Errorf("steps diff (-want +got):\n%s", diff)
	}

	for i, want := range []string{"2026-10-18", "2026-10-28", ""} {
		var day *time.Time
		if err := pool.QueryRow(ctx, `SELECT last_reminder_on FROM sales_invoices WHERE id = $1`, flow+100+int64(i)).Scan(&day); err != nil {
			t.Fatal(err)
		}
		got := ""
		if day != nil {
			got = day.Format(time.DateOnly)
		}
		if got != want {
			t.Errorf("invoice %d last reminder on %q, want %q", i, got, want)
		}
	}
}
