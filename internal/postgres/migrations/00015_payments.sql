-- +goose Up
-- payments settle invoices. One made by hand posts its own journal entries
-- (document type Payment); one linked to a bank transaction is posted by
-- the transaction, in bank_journal_entries below. price is signed like the
-- invoice: a credit invoice's payments are negative.
CREATE TABLE payments (
    id                     bigint PRIMARY KEY,
    administration_id      bigint NOT NULL REFERENCES administrations (id) ON DELETE CASCADE,
    invoice_type           text   NOT NULL,
    invoice_id             bigint NOT NULL,
    financial_account_id   bigint REFERENCES financial_accounts (id) ON DELETE SET NULL,
    financial_mutation_id  bigint REFERENCES financial_mutations (id),
    ledger_account_id      bigint REFERENCES ledger_accounts (id),
    credit_invoice_id      bigint,
    linked_payment_id      bigint,
    manual_payment_action  text   NOT NULL DEFAULT '',
    price                  bigint NOT NULL,
    price_base             bigint NOT NULL,
    payment_date           date   NOT NULL,
    transaction_identifier text   NOT NULL DEFAULT '',
    user_id                bigint,
    created_at             timestamptz NOT NULL,
    updated_at             timestamptz NOT NULL
);
CREATE INDEX payments_invoice_idx ON payments (invoice_id);
CREATE INDEX payments_mutation_idx ON payments (financial_mutation_id);

-- The bank posts the payments linked to its transactions: the debtors are
-- credited, and only what is neither booked nor paying an invoice waits
-- on unprocessed transactions.
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
       l.id, si.contact_id, NULL, NULL,
       NULL, -p.price, coalesce(nullif(si.invoice_id, ''), m.message), 1
FROM payments p
JOIN financial_mutations m ON m.id = p.financial_mutation_id
JOIN ledger_accounts l ON l.administration_id = p.administration_id AND l.system_role = 'accounts_receivable'
LEFT JOIN sales_invoices si ON si.id = p.invoice_id AND p.invoice_type = 'SalesInvoice'
UNION ALL
SELECT m.id | (1::bigint << 62), m.administration_id, 'FinancialMutation', m.id, m.date,
       l.id, NULL, NULL, NULL,
       NULL, -(m.amount - coalesce(bk.booked, 0) * CASE WHEN m.amount < 0 THEN -1 ELSE 1 END - coalesce(pm.paid, 0)), m.message, 2
FROM financial_mutations m
JOIN ledger_accounts l ON l.administration_id = m.administration_id AND l.seed_key = 'unprocessed_transactions'
LEFT JOIN (SELECT financial_mutation_id, sum(price) AS booked FROM ledger_account_bookings GROUP BY financial_mutation_id) bk ON bk.financial_mutation_id = m.id
LEFT JOIN (SELECT financial_mutation_id, sum(price) AS paid FROM payments WHERE financial_mutation_id IS NOT NULL GROUP BY financial_mutation_id) pm ON pm.financial_mutation_id = m.id
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
SELECT m.id | (1::bigint << 62), m.administration_id, 'FinancialMutation', m.id, m.date,
       l.id, NULL, NULL, NULL,
       NULL, -(m.amount - coalesce(bk.booked, 0) * CASE WHEN m.amount < 0 THEN -1 ELSE 1 END), m.message, 2
FROM financial_mutations m
JOIN ledger_accounts l ON l.administration_id = m.administration_id AND l.seed_key = 'unprocessed_transactions'
LEFT JOIN (SELECT financial_mutation_id, sum(price) AS booked FROM ledger_account_bookings GROUP BY financial_mutation_id) bk ON bk.financial_mutation_id = m.id
WHERE m.amount - coalesce(bk.booked, 0) * CASE WHEN m.amount < 0 THEN -1 ELSE 1 END <> 0;

CREATE VIEW ledger_entries AS
SELECT id, administration_id, document_type, document_id, date, ledger_account_id, contact_id, project_id, tax_rate_id,
       financial_account_id, amount, description, row_order
FROM journal_entries
UNION ALL
SELECT * FROM bank_journal_entries;

DELETE FROM journal_entries WHERE document_type = 'Payment';
DROP TABLE payments;
