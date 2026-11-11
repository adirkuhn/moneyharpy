-- +goose Up
-- purchase_documents are purchase invoices and receipts (kind PurchaseInvoice
-- or Receipt), after Moneybird's documents. A new one (an upload not filled
-- in yet) has no contact or lines and posts nothing; the others post with
-- their kind as document type: the total on the creditors, each line on its
-- category and the VAT on receivable VAT. Their payments are in payments,
-- with invoice_type Document.
CREATE TABLE purchase_documents (
    id                           bigint PRIMARY KEY,
    administration_id            bigint  NOT NULL REFERENCES administrations (id) ON DELETE CASCADE,
    kind                         text    NOT NULL,
    contact_id                   bigint  REFERENCES contacts (id),
    entry_number                 integer NOT NULL,
    reference                    text    NOT NULL DEFAULT '',
    date                         date    NOT NULL,
    due_date                     date,
    state                        text    NOT NULL DEFAULT 'new',
    origin                       text    NOT NULL DEFAULT '',
    currency                     text    NOT NULL,
    prices_are_incl_tax          boolean NOT NULL DEFAULT false,
    revenue_invoice              boolean NOT NULL DEFAULT false,
    paid_at                      timestamptz,
    total_excl_tax               bigint  NOT NULL DEFAULT 0,
    total_tax                    bigint  NOT NULL DEFAULT 0,
    total_incl_tax               bigint  NOT NULL DEFAULT 0,
    tax_totals                   jsonb   NOT NULL DEFAULT '[]',
    created_at                   timestamptz NOT NULL,
    updated_at                   timestamptz NOT NULL
);
CREATE INDEX purchase_documents_admin_date_idx ON purchase_documents (administration_id, kind, date);
CREATE INDEX purchase_documents_contact_idx ON purchase_documents (contact_id);
CREATE INDEX purchase_documents_state_idx ON purchase_documents (administration_id, state);

CREATE TABLE purchase_document_details (
    id                   bigint PRIMARY KEY,
    administration_id    bigint  NOT NULL REFERENCES administrations (id) ON DELETE CASCADE,
    purchase_document_id bigint  NOT NULL REFERENCES purchase_documents (id) ON DELETE CASCADE,
    description          text    NOT NULL DEFAULT '',
    period               text    NOT NULL DEFAULT '',
    price                bigint  NOT NULL DEFAULT 0,
    amount               text    NOT NULL DEFAULT '1',
    quantity             bigint  NOT NULL DEFAULT 1000,
    tax_rate_id          bigint  NOT NULL REFERENCES tax_rates (id),
    ledger_account_id    bigint  REFERENCES ledger_accounts (id),
    project_id           bigint  REFERENCES projects (id),
    product_id           bigint  REFERENCES products (id) ON DELETE SET NULL,
    row_order            integer NOT NULL DEFAULT 0,
    line_base            bigint  NOT NULL DEFAULT 0,
    created_at           timestamptz NOT NULL,
    updated_at           timestamptz NOT NULL
);
CREATE INDEX purchase_document_details_document_idx ON purchase_document_details (purchase_document_id);
CREATE INDEX purchase_document_details_ledger_idx ON purchase_document_details (ledger_account_id);

-- fiscal allocations move a percentage of what lines book on a category
-- to a private category, such as the private use of a phone. percentage
-- is in hundredths.
CREATE TABLE purchase_document_fiscal_allocations (
    id                        bigint PRIMARY KEY,
    administration_id         bigint  NOT NULL REFERENCES administrations (id) ON DELETE CASCADE,
    purchase_document_id      bigint  NOT NULL REFERENCES purchase_documents (id) ON DELETE CASCADE,
    ledger_account_id         bigint  NOT NULL REFERENCES ledger_accounts (id),
    private_ledger_account_id bigint  NOT NULL REFERENCES ledger_accounts (id),
    percentage                integer NOT NULL,
    price                     bigint  NOT NULL DEFAULT 0,
    created_at                timestamptz NOT NULL,
    updated_at                timestamptz NOT NULL
);
CREATE INDEX purchase_document_fiscal_allocations_document_idx ON purchase_document_fiscal_allocations (purchase_document_id);

-- general_documents are documents kept for the record, such as a contract;
-- they book nothing.
CREATE TABLE general_documents (
    id                bigint PRIMARY KEY,
    administration_id bigint  NOT NULL REFERENCES administrations (id) ON DELETE CASCADE,
    contact_id        bigint  REFERENCES contacts (id),
    entry_number      integer NOT NULL,
    reference         text    NOT NULL DEFAULT '',
    date              date    NOT NULL,
    due_date          date,
    reminder_date     date,
    reminder_message  text    NOT NULL DEFAULT '',
    state             text    NOT NULL DEFAULT 'saved',
    created_at        timestamptz NOT NULL,
    updated_at        timestamptz NOT NULL
);
CREATE INDEX general_documents_admin_date_idx ON general_documents (administration_id, date);

-- typeless_documents are uploads still to be processed into a document.
CREATE TABLE typeless_documents (
    id                bigint PRIMARY KEY,
    administration_id bigint  NOT NULL REFERENCES administrations (id) ON DELETE CASCADE,
    contact_id        bigint  REFERENCES contacts (id),
    reference         text    NOT NULL DEFAULT '',
    date              date    NOT NULL,
    origin            text    NOT NULL DEFAULT '',
    state             text    NOT NULL DEFAULT 'new',
    created_at        timestamptz NOT NULL,
    updated_at        timestamptz NOT NULL
);
CREATE INDEX typeless_documents_admin_date_idx ON typeless_documents (administration_id, date);

-- purchase_transaction_batches group credit transfers paid from one
-- financial account, exported as a SEPA credit transfer file.
CREATE TABLE purchase_transaction_batches (
    id                   bigint PRIMARY KEY,
    administration_id    bigint NOT NULL REFERENCES administrations (id) ON DELETE CASCADE,
    financial_account_id bigint NOT NULL REFERENCES financial_accounts (id),
    identifier           text   NOT NULL,
    state                text   NOT NULL DEFAULT 'open',
    execution_date       date   NOT NULL,
    created_at           timestamptz NOT NULL,
    updated_at           timestamptz NOT NULL
);
CREATE INDEX purchase_transaction_batches_admin_idx ON purchase_transaction_batches (administration_id);

-- purchase_transactions are credit transfers to pay, mostly of a purchase
-- invoice or receipt (payable). Paid and pending are worked out from the
-- payable and the batch; only cancelled is kept.
CREATE TABLE purchase_transactions (
    id                            bigint PRIMARY KEY,
    administration_id             bigint NOT NULL REFERENCES administrations (id) ON DELETE CASCADE,
    financial_account_id          bigint NOT NULL REFERENCES financial_accounts (id),
    purchase_transaction_batch_id bigint REFERENCES purchase_transaction_batches (id) ON DELETE SET NULL,
    payable_type                  text,
    payable_id                    bigint,
    contact_id                    bigint REFERENCES contacts (id) ON DELETE SET NULL,
    amount                        bigint NOT NULL,
    date                          date,
    description                   text   NOT NULL DEFAULT '',
    end_to_end_id                 text   NOT NULL DEFAULT '',
    sepa_iban                     text   NOT NULL DEFAULT '',
    sepa_iban_account_name        text   NOT NULL DEFAULT '',
    sepa_bic                      text   NOT NULL DEFAULT '',
    source_sepa_iban              text,
    source_sepa_iban_account_name text,
    cancelled                     boolean NOT NULL DEFAULT false,
    created_at                    timestamptz NOT NULL,
    updated_at                    timestamptz NOT NULL
);
CREATE INDEX purchase_transactions_admin_idx ON purchase_transactions (administration_id);
CREATE INDEX purchase_transactions_payable_idx ON purchase_transactions (payable_id);
CREATE INDEX purchase_transactions_batch_idx ON purchase_transactions (purchase_transaction_batch_id);

-- The bank posts payments of purchase documents on the creditors (a debit:
-- money out pays what is owed), and they count against what is open of the
-- transaction with the opposite sign of sales payments.
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

DELETE FROM journal_entries WHERE document_type IN ('PurchaseInvoice', 'Receipt');
DELETE FROM journal_entries WHERE document_type = 'Payment'
    AND document_id IN (SELECT id FROM payments WHERE invoice_type = 'Document');
DELETE FROM payments WHERE invoice_type = 'Document';
DELETE FROM notes WHERE entity_type IN ('PurchaseInvoice', 'Receipt', 'GeneralDocument', 'TypelessDocument');
DELETE FROM attachments WHERE entity_type IN ('PurchaseInvoice', 'Receipt', 'GeneralDocument', 'TypelessDocument');
DELETE FROM resources WHERE type IN ('documents/purchase_invoices', 'documents/receipts', 'documents/general_documents',
                                     'documents/typeless_documents', 'purchase_transactions', 'purchase_transaction_batches');
DROP TABLE purchase_transactions;
DROP TABLE purchase_transaction_batches;
DROP TABLE typeless_documents;
DROP TABLE general_documents;
DROP TABLE purchase_document_fiscal_allocations;
DROP TABLE purchase_document_details;
DROP TABLE purchase_documents;
