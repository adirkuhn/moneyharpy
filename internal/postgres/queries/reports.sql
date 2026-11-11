-- Reports and exports: every figure comes from the journal (ledger_entries),
-- apart from what is still open on documents and the subscriptions.

-- name: ReportLedgerAccounts :many
-- financial marks the categories of financial accounts: the bank.
SELECT l.id, l.name, l.account_type, l.account_id, l.parent_id, l.active, l.taxonomy_code, l.system_role,
       EXISTS (SELECT 1 FROM financial_accounts f WHERE f.ledger_account_id = l.id)::boolean AS financial
FROM ledger_accounts l
WHERE l.administration_id = @administration_id
ORDER BY l.account_id, l.name, l.id;

-- name: ReportMovements :many
-- The sum per category of the entries between the dates; without a from
-- date, of every entry up to the to date.
SELECT e.ledger_account_id, sum(e.amount)::bigint AS amount
FROM ledger_entries e
WHERE e.administration_id = @administration_id
  AND (sqlc.narg(from_date)::date IS NULL OR e.date >= sqlc.narg(from_date)::date)
  AND e.date <= @to_date
  AND (sqlc.narg(project_id)::bigint IS NULL OR e.project_id = sqlc.narg(project_id))
GROUP BY e.ledger_account_id
ORDER BY e.ledger_account_id;

-- name: ReportYearResults :many
-- The result of the profit and loss categories per calendar year.
SELECT extract(year FROM e.date)::integer AS year, sum(e.amount)::bigint AS amount
FROM ledger_entries e JOIN ledger_accounts l ON l.id = e.ledger_account_id
WHERE e.administration_id = @administration_id AND e.date <= @to_date
  AND l.account_type IN ('revenue', 'direct_costs', 'expenses', 'other_income_expenses')
GROUP BY 1
ORDER BY 1;

-- name: ReportContactSums :many
-- The sum per contact of the entries on categories of the types or system
-- roles between the dates; without a from date, the balance at the to date.
SELECT e.contact_id, sum(e.amount)::bigint AS amount
FROM ledger_entries e JOIN ledger_accounts l ON l.id = e.ledger_account_id
WHERE e.administration_id = @administration_id
  AND (sqlc.narg(from_date)::date IS NULL OR e.date >= sqlc.narg(from_date)::date)
  AND e.date <= @to_date
  AND (l.account_type = ANY(@account_types::text[]) OR l.system_role = ANY(@system_roles::text[]))
  AND (sqlc.narg(contact_id)::bigint IS NULL OR e.contact_id = sqlc.narg(contact_id))
GROUP BY e.contact_id;

-- name: ReportProjectSums :many
SELECT e.project_id, sum(e.amount)::bigint AS amount
FROM ledger_entries e JOIN ledger_accounts l ON l.id = e.ledger_account_id
WHERE e.administration_id = @administration_id
  AND (sqlc.narg(from_date)::date IS NULL OR e.date >= sqlc.narg(from_date)::date)
  AND e.date <= @to_date
  AND (l.account_type = ANY(@account_types::text[]) OR l.system_role = ANY(@system_roles::text[]))
GROUP BY e.project_id;

-- name: ReportCashFlow :many
-- What the bank transactions between the dates booked against the bank,
-- per category: received for money in, paid for money out.
SELECT e.ledger_account_id,
       coalesce(sum(e.amount) FILTER (WHERE m.amount > 0), 0)::bigint AS received,
       coalesce(sum(e.amount) FILTER (WHERE m.amount < 0), 0)::bigint AS paid
FROM ledger_entries e JOIN financial_mutations m ON m.id = e.document_id
WHERE e.administration_id = @administration_id AND e.document_type = 'FinancialMutation' AND e.financial_account_id IS NULL
  AND e.date >= @from_date AND e.date <= @to_date
  AND (sqlc.narg(financial_account_id)::bigint IS NULL OR m.financial_account_id = sqlc.narg(financial_account_id))
GROUP BY e.ledger_account_id
ORDER BY e.ledger_account_id;

-- name: ReportCashBalance :one
-- The balance of the financial accounts' categories at the to date.
SELECT coalesce(sum(e.amount), 0)::bigint AS balance
FROM ledger_entries e
WHERE e.administration_id = @administration_id AND e.date <= @to_date
  AND e.ledger_account_id IN (SELECT f.ledger_account_id FROM financial_accounts f
                              WHERE f.administration_id = @administration_id
                                AND (sqlc.narg(financial_account_id)::bigint IS NULL OR f.id = sqlc.narg(financial_account_id)));

-- name: ReportTaxSums :many
-- Per VAT rate between the dates: what the rate was charged on, and the VAT
-- on the payable or receivable VAT category.
SELECT t.id AS tax_rate_id, t.name, t.percentage::text AS percentage, t.tax_rate_type, t.report_boxes,
       coalesce(sum(e.amount) FILTER (WHERE l.system_role NOT IN ('payable_vat', 'receivable_vat')), 0)::bigint AS base,
       coalesce(sum(e.amount) FILTER (WHERE l.system_role = 'payable_vat'), 0)::bigint AS payable,
       coalesce(sum(e.amount) FILTER (WHERE l.system_role = 'receivable_vat'), 0)::bigint AS receivable
FROM ledger_entries e
JOIN ledger_accounts l ON l.id = e.ledger_account_id
JOIN tax_rates t ON t.id = e.tax_rate_id
WHERE e.administration_id = @administration_id AND e.date >= @from_date AND e.date <= @to_date
GROUP BY t.id
ORDER BY t.tax_rate_type DESC, t.position, t.id;

-- name: ReportOpenItems :many
-- What each booked document on the category of the role still had open at
-- the date: what it posted there less what was paid by then.
WITH posted AS (
    SELECT e.document_type, e.document_id, e.contact_id, min(e.date)::date AS date, sum(e.amount)::bigint AS amount
    FROM journal_entries e JOIN ledger_accounts l ON l.id = e.ledger_account_id
    WHERE e.administration_id = @administration_id AND l.system_role = @system_role AND e.date <= @until
      AND e.document_type IN ('SalesInvoice', 'ExternalSalesInvoice', 'PurchaseInvoice', 'Receipt')
    GROUP BY e.document_type, e.document_id, e.contact_id
), paid AS (
    SELECT p.invoice_id, sum(p.price)::bigint AS paid
    FROM payments p
    WHERE p.administration_id = @administration_id AND p.payment_date <= @until
    GROUP BY p.invoice_id
)
SELECT d.contact_id, d.date, d.amount, coalesce(paid.paid, 0)::bigint AS paid
FROM posted d LEFT JOIN paid ON paid.invoice_id = d.document_id
ORDER BY d.date, d.document_id;

-- name: ReportSubscriptions :many
-- Every subscription with each product it ran, for the subscriptions report.
SELECT s.id, s.contact_id, s.start_date, s.end_date, s.cancelled_at,
       i.start_date AS item_start, i.end_date AS item_end, i.quantity, i.discount,
       p.price, p.frequency_count, p.frequency_type
FROM subscriptions s
JOIN subscription_products i ON i.subscription_id = s.id
JOIN products p ON p.id = i.product_id
WHERE s.administration_id = @administration_id
ORDER BY s.id, i.start_date, i.id;

-- name: ReportLedgerAccountEntries :many
-- The deprecated ledger account report: the newest first.
SELECT e.id, e.administration_id, e.document_type, e.document_id, e.date, e.ledger_account_id, e.contact_id, e.project_id,
       e.tax_rate_id, e.financial_account_id, e.amount, e.description, e.row_order, count(*) OVER () AS total
FROM ledger_entries e
WHERE e.administration_id = @administration_id AND e.ledger_account_id = @ledger_account_id
  AND e.date >= @from_date AND e.date <= @to_date
ORDER BY e.date DESC, e.document_id DESC, e.row_order DESC, e.id DESC
LIMIT @max_rows OFFSET @skip;

-- name: ReportYearEntries :many
-- Every entry between the dates, by document, for the exports.
SELECT e.id, e.administration_id, e.document_type, e.document_id, e.date, e.ledger_account_id, e.contact_id, e.project_id,
       e.tax_rate_id, e.financial_account_id, e.amount, e.description, e.row_order
FROM ledger_entries e
WHERE e.administration_id = @administration_id AND e.date >= @from_date AND e.date <= @to_date
ORDER BY e.date, e.document_type, e.document_id, e.row_order, e.id;

-- name: ReportContacts :many
-- The contacts the exports name.
SELECT c.id, c.company_name, c.firstname, c.lastname, c.customer_id, c.address1, c.zipcode, c.city, c.country, c.tax_number
FROM contacts c
WHERE c.administration_id = @administration_id AND c.id = ANY(@ids::bigint[])
ORDER BY c.id;
