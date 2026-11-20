package postgres

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/adirkuhn/moneyharpy/internal/resource"
)

// PurgeStore implements purge.Store.
type PurgeStore struct {
	pool *pgxpool.Pool
}

// NewPurgeStore returns a PurgeStore on pool.
func NewPurgeStore(pool *pgxpool.Pool) *PurgeStore { return &PurgeStore{pool: pool} }

// keptResources are the kinds of record emptying keeps: the setup of the
// administration, not its data.
var keptResources = []string{
	string(resource.TypeAdministration), string(resource.TypeIdentity), string(resource.TypeCustomField),
	string(resource.TypeWorkflow), string(resource.TypeDocumentStyle), string(resource.TypeWebhook),
}

// emptyStatements delete an administration's data, children before the
// records they point to; $1 is the administration. Its setup (members,
// invitations, identities, workflows, document styles, custom fields,
// connections and webhooks) stays.
var emptyStatements = []string{
	`DELETE FROM events WHERE administration_id = $1`,
	`DELETE FROM notes WHERE administration_id = $1`,
	`DELETE FROM attachments WHERE administration_id = $1`,
	`DELETE FROM downloads WHERE administration_id = $1`,
	`DELETE FROM document_recognitions WHERE administration_id = $1`,
	`DELETE FROM supplier_memory_lines WHERE administration_id = $1`,
	`DELETE FROM supplier_memory_kinds WHERE administration_id = $1`,
	`DELETE FROM supplier_memories WHERE administration_id = $1`,
	`DELETE FROM moneybird_import_ids WHERE administration_id = $1`,
	`DELETE FROM moneybird_imports WHERE administration_id = $1`,
	`DELETE FROM ledger_account_bookings WHERE administration_id = $1`,
	`DELETE FROM payments WHERE administration_id = $1`,
	`DELETE FROM journal_entries WHERE administration_id = $1`,
	`DELETE FROM time_entries WHERE administration_id = $1`,
	`DELETE FROM task_list_tasks WHERE administration_id = $1`,
	`DELETE FROM task_list_groups WHERE administration_id = $1`,
	`DELETE FROM task_lists WHERE administration_id = $1`,
	`DELETE FROM task_list_templates WHERE administration_id = $1`,
	`DELETE FROM additional_charges WHERE administration_id = $1`,
	`DELETE FROM subscription_one_off_invoices WHERE administration_id = $1`,
	`DELETE FROM subscriptions WHERE administration_id = $1`,
	`DELETE FROM estimate_signings WHERE administration_id = $1`,
	`DELETE FROM estimates WHERE administration_id = $1`,
	`DELETE FROM sales_invoices WHERE administration_id = $1`,
	`DELETE FROM recurring_sales_invoices WHERE administration_id = $1`,
	`DELETE FROM external_sales_invoices WHERE administration_id = $1`,
	`DELETE FROM purchase_transactions WHERE administration_id = $1`,
	`DELETE FROM purchase_transaction_batches WHERE administration_id = $1`,
	`DELETE FROM purchase_documents WHERE administration_id = $1`,
	`DELETE FROM general_documents WHERE administration_id = $1`,
	`DELETE FROM typeless_documents WHERE administration_id = $1`,
	`DELETE FROM general_journal_documents WHERE administration_id = $1`,
	`DELETE FROM vat_returns WHERE administration_id = $1`,
	`DELETE FROM assets WHERE administration_id = $1`,
	`DELETE FROM ledger_account_audits WHERE administration_id = $1`,
	`DELETE FROM bank_connections WHERE administration_id = $1`,
	`DELETE FROM booking_rules WHERE administration_id = $1`,
	`DELETE FROM financial_mutations WHERE administration_id = $1`,
	`DELETE FROM financial_statements WHERE administration_id = $1`,
	`DELETE FROM financial_accounts WHERE administration_id = $1`,
	`DELETE FROM products WHERE administration_id = $1`,
	`DELETE FROM projects WHERE administration_id = $1`,
	`DELETE FROM contacts WHERE administration_id = $1`,
	`UPDATE ledger_accounts SET parent_id = NULL WHERE administration_id = $1`,
	`DELETE FROM ledger_accounts WHERE administration_id = $1`,
	`DELETE FROM tax_rates WHERE administration_id = $1`,
	`DELETE FROM resources WHERE administration_id = $1 AND type <> ALL($2::text[])`,
	// The default categories, VAT rates and cash account come again on next use, as for a new administration.
	`UPDATE administrations SET bookkeeping_seeded = false, bank_seeded = false, period_locked_until = NULL WHERE id = $1`,
}

// Empty implements purge.Store. It returns the storage keys of the files
// the removed records had, to delete once the records are gone.
func (s *PurgeStore) Empty(ctx context.Context, admin resource.ID) ([]string, error) {
	var keys []string
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		keys = nil
		for _, q := range []string{
			`SELECT 'administrations/' || administration_id || '/attachments/' || id FROM attachments WHERE administration_id = $1`,
			`SELECT 'administrations/' || administration_id || '/downloads/' || id FROM downloads WHERE administration_id = $1`,
		} {
			rows, err := tx.Query(ctx, q, int64(admin))
			if err != nil {
				return err
			}
			found, err := pgx.CollectRows(rows, pgx.RowTo[string])
			if err != nil {
				return err
			}
			keys = append(keys, found...)
		}
		for _, q := range emptyStatements {
			args := []any{int64(admin)}
			if strings.Contains(q, "$2") {
				args = append(args, keptResources)
			}
			if _, err := tx.Exec(ctx, q, args...); err != nil {
				return fmt.Errorf("%s: %w", q, err)
			}
		}
		return nil
	})
	return keys, err
}

// ImportRunning implements purge.Store.
func (s *PurgeStore) ImportRunning(ctx context.Context, admin resource.ID) (bool, error) {
	var running bool
	err := s.pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM moneybird_imports WHERE administration_id = $1 AND state = 'running')`, int64(admin)).Scan(&running)
	return running, err
}
