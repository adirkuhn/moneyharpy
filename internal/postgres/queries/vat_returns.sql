-- name: ListVatReturns :many
SELECT * FROM vat_returns WHERE administration_id = @administration_id
ORDER BY period_from DESC, created_at DESC, id DESC;

-- name: GetVatReturn :one
SELECT * FROM vat_returns WHERE administration_id = @administration_id AND id = @id;

-- name: InsertVatReturn :exec
INSERT INTO vat_returns (id, administration_id, kind, period_from, period_to, granularity, sent_on, boxes, corrections, icp_lines,
    created_by, created_at, updated_at)
VALUES (@id, @administration_id, @kind, @period_from, @period_to, @granularity, @sent_on, @boxes, @corrections, @icp_lines,
    @created_by, @created_at, @updated_at);

-- name: DeleteVatReturn :exec
DELETE FROM vat_returns WHERE administration_id = @administration_id AND id = @id;

-- name: TouchVatReturn :exec
UPDATE vat_returns SET updated_at = @updated_at WHERE administration_id = @administration_id AND id = @id;

-- name: VatICPSums :many
-- What was sold per contact between the dates on sales rates that count in
-- box 3b: supplies to businesses in other EU countries.
SELECT c.id AS contact_id,
       coalesce(nullif(c.company_name, ''), trim(c.firstname || ' ' || c.lastname))::text AS name,
       c.country, c.tax_number, (-sum(e.amount))::bigint AS amount
FROM ledger_entries e
JOIN ledger_accounts l ON l.id = e.ledger_account_id
JOIN tax_rates t ON t.id = e.tax_rate_id
JOIN contacts c ON c.id = e.contact_id
WHERE e.administration_id = @administration_id AND e.date >= @from_date AND e.date <= @to_date
  AND t.tax_rate_type = 'sales_invoice' AND EXISTS (SELECT 1 FROM unnest(t.report_boxes) b WHERE b LIKE '%/3b')
  AND l.system_role NOT IN ('payable_vat', 'receivable_vat')
GROUP BY c.id
HAVING sum(e.amount) <> 0
ORDER BY 2, c.id;

-- name: VatChecks :one
-- What may be missing from the books of a period: bank transactions up to
-- its end that are not fully booked, transactions in it booked straight on
-- revenue or costs, and incoming documents still to be processed.
SELECT (SELECT count(*) FROM bank_journal_entries b
        JOIN ledger_accounts l ON l.id = b.ledger_account_id
        WHERE b.administration_id = @administration_id AND b.date <= @to_date AND b.row_order = 2
          AND l.seed_key = 'transactions_to_classify')::int AS open_transactions,
       (SELECT count(DISTINCT m.id) FROM ledger_account_bookings k
        JOIN financial_mutations m ON m.id = k.financial_mutation_id
        JOIN ledger_accounts l ON l.id = k.ledger_account_id
        WHERE k.administration_id = @administration_id AND m.date >= @from_date AND m.date <= @to_date
          AND l.account_type IN ('revenue', 'direct_costs', 'expenses'))::int AS bookings_without_vat,
       ((SELECT count(*) FROM purchase_documents d WHERE d.administration_id = @administration_id AND d.state = 'new')
        + (SELECT count(*) FROM typeless_documents t WHERE t.administration_id = @administration_id))::int AS unprocessed_documents;

-- name: FirstLedgerEntry :one
SELECT min(date)::date AS first FROM ledger_entries WHERE administration_id = @administration_id;
