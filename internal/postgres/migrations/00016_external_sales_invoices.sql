-- +goose Up
-- external_sales_invoices are invoices made in other software, such as a
-- webshop or a till. A new one (an uploaded file not filled in yet) has no
-- contact and posts nothing; the others post like sales invoices, with
-- document type ExternalSalesInvoice.
CREATE TABLE external_sales_invoices (
    id                      bigint PRIMARY KEY,
    administration_id       bigint  NOT NULL REFERENCES administrations (id) ON DELETE CASCADE,
    contact_id              bigint  REFERENCES contacts (id),
    entry_number            integer NOT NULL,
    reference               text    NOT NULL DEFAULT '',
    date                    date    NOT NULL,
    due_date                date,
    state                   text    NOT NULL DEFAULT 'new',
    origin                  text    NOT NULL DEFAULT '',
    source                  text    NOT NULL DEFAULT '',
    source_url              text    NOT NULL DEFAULT '',
    currency                text    NOT NULL,
    prices_are_incl_tax     boolean NOT NULL DEFAULT false,
    paid_at                 timestamptz,
    marked_dubious_on       date,
    marked_uncollectible_on date,
    total_excl_tax          bigint  NOT NULL DEFAULT 0,
    total_tax               bigint  NOT NULL DEFAULT 0,
    total_incl_tax          bigint  NOT NULL DEFAULT 0,
    tax_totals              jsonb   NOT NULL DEFAULT '[]',
    created_at              timestamptz NOT NULL,
    updated_at              timestamptz NOT NULL
);
CREATE INDEX external_sales_invoices_admin_date_idx ON external_sales_invoices (administration_id, date);
CREATE INDEX external_sales_invoices_contact_idx ON external_sales_invoices (contact_id);
CREATE INDEX external_sales_invoices_state_idx ON external_sales_invoices (administration_id, state);

CREATE TABLE external_sales_invoice_details (
    id                        bigint PRIMARY KEY,
    administration_id         bigint  NOT NULL REFERENCES administrations (id) ON DELETE CASCADE,
    external_sales_invoice_id bigint  NOT NULL REFERENCES external_sales_invoices (id) ON DELETE CASCADE,
    description               text    NOT NULL DEFAULT '',
    period                    text    NOT NULL DEFAULT '',
    price                     bigint  NOT NULL DEFAULT 0,
    amount                    text    NOT NULL DEFAULT '1',
    quantity                  bigint  NOT NULL DEFAULT 1000,
    tax_rate_id               bigint  NOT NULL REFERENCES tax_rates (id),
    ledger_account_id         bigint  REFERENCES ledger_accounts (id),
    project_id                bigint,
    product_id                bigint,
    row_order                 integer NOT NULL DEFAULT 0,
    line_base                 bigint  NOT NULL DEFAULT 0,
    created_at                timestamptz NOT NULL,
    updated_at                timestamptz NOT NULL
);
CREATE INDEX external_sales_invoice_details_invoice_idx ON external_sales_invoice_details (external_sales_invoice_id);

-- The bank posts payments of external invoices like those of sales
-- invoices: on the debtors, for the invoice's contact.
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

DELETE FROM journal_entries WHERE document_type = 'ExternalSalesInvoice';
DELETE FROM journal_entries WHERE document_type = 'Payment'
    AND document_id IN (SELECT id FROM payments WHERE invoice_type = 'ExternalSalesInvoice');
DELETE FROM payments WHERE invoice_type = 'ExternalSalesInvoice';
DELETE FROM resources WHERE type = 'external_sales_invoices';
DROP TABLE external_sales_invoice_details;
DROP TABLE external_sales_invoices;
