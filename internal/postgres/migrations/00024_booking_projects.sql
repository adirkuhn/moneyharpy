-- +goose Up
-- A bank transaction booked on a category can name a project (Moneybird's
-- link_booking project_id); the journal carries it, so revenue and expenses
-- by project include it.
ALTER TABLE ledger_account_bookings ADD COLUMN project_id bigint;

DROP VIEW ledger_entries;
DROP VIEW bank_journal_entries;

CREATE VIEW bank_journal_entries AS
SELECT m.id, m.administration_id, 'FinancialMutation'::text AS document_type, m.id AS document_id, m.date,
       a.ledger_account_id, NULL::bigint AS contact_id, NULL::bigint AS project_id, NULL::bigint AS tax_rate_id,
       m.financial_account_id, m.amount, m.message AS description, 0 AS row_order
FROM financial_mutations m JOIN financial_accounts a ON a.id = m.financial_account_id
UNION ALL
SELECT b.id, b.administration_id, 'FinancialMutation', m.id, m.date,
       b.ledger_account_id, NULL, b.project_id, NULL,
       NULL, -(b.price * CASE WHEN m.amount < 0 THEN -1 ELSE 1 END), CASE WHEN b.description <> '' THEN b.description ELSE m.message END, 1
FROM ledger_account_bookings b JOIN financial_mutations m ON m.id = b.financial_mutation_id
UNION ALL
SELECT p.id, p.administration_id, 'FinancialMutation', m.id, m.date,
       l.id, coalesce(si.contact_id, ei.contact_id), NULL, NULL,
       NULL, -p.price, coalesce(nullif(si.invoice_id, ''), nullif(ei.reference, ''), m.message), 1
FROM payments p
JOIN financial_mutations m ON m.id = p.financial_mutation_id
JOIN ledger_accounts l ON l.administration_id = p.administration_id AND l.system_role = 'accounts_receivable'
LEFT JOIN sales_invoices si ON si.id = p.invoice_id AND p.invoice_type = 'SalesInvoice'
LEFT JOIN external_sales_invoices ei ON ei.id = p.invoice_id AND p.invoice_type = 'ExternalSalesInvoice'
WHERE p.invoice_type <> 'Document'
UNION ALL
SELECT p.id, p.administration_id, 'FinancialMutation', m.id, m.date,
       l.id, d.contact_id, NULL, NULL,
       NULL, p.price, coalesce(nullif(d.reference, ''), m.message), 1
FROM payments p
JOIN financial_mutations m ON m.id = p.financial_mutation_id
JOIN ledger_accounts l ON l.administration_id = p.administration_id AND l.system_role = 'accounts_payable'
LEFT JOIN purchase_documents d ON d.id = p.invoice_id
WHERE p.invoice_type = 'Document'
UNION ALL
SELECT m.id | (1::bigint << 62), m.administration_id, 'FinancialMutation', m.id, m.date,
       l.id, NULL, NULL, NULL,
       NULL, -(m.amount - coalesce(bk.booked, 0) * CASE WHEN m.amount < 0 THEN -1 ELSE 1 END - coalesce(pm.paid, 0)), m.message, 2
FROM financial_mutations m
JOIN ledger_accounts l ON l.administration_id = m.administration_id AND l.seed_key = 'unprocessed_transactions'
LEFT JOIN (SELECT financial_mutation_id, sum(price) AS booked FROM ledger_account_bookings GROUP BY financial_mutation_id) bk ON bk.financial_mutation_id = m.id
LEFT JOIN (SELECT financial_mutation_id, sum(CASE WHEN invoice_type = 'Document' THEN -price ELSE price END) AS paid
           FROM payments WHERE financial_mutation_id IS NOT NULL GROUP BY financial_mutation_id) pm ON pm.financial_mutation_id = m.id
WHERE m.amount - coalesce(bk.booked, 0) * CASE WHEN m.amount < 0 THEN -1 ELSE 1 END - coalesce(pm.paid, 0) <> 0;

CREATE VIEW ledger_entries AS
SELECT id, administration_id, document_type, document_id, date, ledger_account_id, contact_id, project_id, tax_rate_id,
       financial_account_id, amount, description, row_order
FROM journal_entries
UNION ALL
SELECT * FROM bank_journal_entries;

-- +goose Down
DROP VIEW ledger_entries;
DROP VIEW bank_journal_entries;

CREATE VIEW bank_journal_entries AS
SELECT m.id, m.administration_id, 'FinancialMutation'::text AS document_type, m.id AS document_id, m.date,
       a.ledger_account_id, NULL::bigint AS contact_id, NULL::bigint AS project_id, NULL::bigint AS tax_rate_id,
       m.financial_account_id, m.amount, m.message AS description, 0 AS row_order
FROM financial_mutations m JOIN financial_accounts a ON a.id = m.financial_account_id
UNION ALL
SELECT b.id, b.administration_id, 'FinancialMutation', m.id, m.date,
       b.ledger_account_id, NULL, NULL, NULL,
       NULL, -(b.price * CASE WHEN m.amount < 0 THEN -1 ELSE 1 END), CASE WHEN b.description <> '' THEN b.description ELSE m.message END, 1
FROM ledger_account_bookings b JOIN financial_mutations m ON m.id = b.financial_mutation_id
UNION ALL
SELECT p.id, p.administration_id, 'FinancialMutation', m.id, m.date,
       l.id, coalesce(si.contact_id, ei.contact_id), NULL, NULL,
       NULL, -p.price, coalesce(nullif(si.invoice_id, ''), nullif(ei.reference, ''), m.message), 1
FROM payments p
JOIN financial_mutations m ON m.id = p.financial_mutation_id
JOIN ledger_accounts l ON l.administration_id = p.administration_id AND l.system_role = 'accounts_receivable'
LEFT JOIN sales_invoices si ON si.id = p.invoice_id AND p.invoice_type = 'SalesInvoice'
LEFT JOIN external_sales_invoices ei ON ei.id = p.invoice_id AND p.invoice_type = 'ExternalSalesInvoice'
WHERE p.invoice_type <> 'Document'
UNION ALL
SELECT p.id, p.administration_id, 'FinancialMutation', m.id, m.date,
       l.id, d.contact_id, NULL, NULL,
       NULL, p.price, coalesce(nullif(d.reference, ''), m.message), 1
FROM payments p
JOIN financial_mutations m ON m.id = p.financial_mutation_id
JOIN ledger_accounts l ON l.administration_id = p.administration_id AND l.system_role = 'accounts_payable'
LEFT JOIN purchase_documents d ON d.id = p.invoice_id
WHERE p.invoice_type = 'Document'
UNION ALL
SELECT m.id | (1::bigint << 62), m.administration_id, 'FinancialMutation', m.id, m.date,
       l.id, NULL, NULL, NULL,
       NULL, -(m.amount - coalesce(bk.booked, 0) * CASE WHEN m.amount < 0 THEN -1 ELSE 1 END - coalesce(pm.paid, 0)), m.message, 2
FROM financial_mutations m
JOIN ledger_accounts l ON l.administration_id = m.administration_id AND l.seed_key = 'unprocessed_transactions'
LEFT JOIN (SELECT financial_mutation_id, sum(price) AS booked FROM ledger_account_bookings GROUP BY financial_mutation_id) bk ON bk.financial_mutation_id = m.id
LEFT JOIN (SELECT financial_mutation_id, sum(CASE WHEN invoice_type = 'Document' THEN -price ELSE price END) AS paid
           FROM payments WHERE financial_mutation_id IS NOT NULL GROUP BY financial_mutation_id) pm ON pm.financial_mutation_id = m.id
WHERE m.amount - coalesce(bk.booked, 0) * CASE WHEN m.amount < 0 THEN -1 ELSE 1 END - coalesce(pm.paid, 0) <> 0;

CREATE VIEW ledger_entries AS
SELECT id, administration_id, document_type, document_id, date, ledger_account_id, contact_id, project_id, tax_rate_id,
       financial_account_id, amount, description, row_order
FROM journal_entries
UNION ALL
SELECT * FROM bank_journal_entries;

ALTER TABLE ledger_account_bookings DROP COLUMN project_id;
