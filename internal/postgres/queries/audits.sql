-- name: AuditSums :many
SELECT e.ledger_account_id,
       coalesce(sum(e.amount) FILTER (WHERE e.amount > 0), 0)::bigint AS debit,
       coalesce(-sum(e.amount) FILTER (WHERE e.amount < 0), 0)::bigint AS credit,
       coalesce(array_agg(DISTINCT e.tax_rate_id) FILTER (WHERE e.tax_rate_id IS NOT NULL), '{}')::bigint[] AS tax_rate_ids
FROM ledger_entries e
WHERE e.administration_id = @administration_id AND e.date BETWEEN @from_date AND @to_date
GROUP BY e.ledger_account_id
ORDER BY e.ledger_account_id;

-- name: ListAudits :many
SELECT * FROM ledger_account_audits
WHERE administration_id = @administration_id AND period_from = @period_from AND period_to = @period_to
ORDER BY ledger_account_id;

-- name: SaveAudit :exec
INSERT INTO ledger_account_audits (id, administration_id, ledger_account_id, period_from, period_to, debit, credit, user_id, audited_at)
VALUES (@id, @administration_id, @ledger_account_id, @period_from, @period_to, @debit, @credit, @user_id, @audited_at)
ON CONFLICT (administration_id, ledger_account_id, period_from, period_to)
DO UPDATE SET debit = excluded.debit, credit = excluded.credit, user_id = excluded.user_id, audited_at = excluded.audited_at;

-- name: DeleteAudit :exec
DELETE FROM ledger_account_audits
WHERE administration_id = @administration_id AND ledger_account_id = @ledger_account_id AND period_from = @period_from AND period_to = @period_to;

-- name: LedgerAccountInAdministration :one
SELECT EXISTS (SELECT 1 FROM ledger_accounts WHERE administration_id = @administration_id AND id = @id);
