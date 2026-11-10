-- +goose Up
-- journal_entries holds what documents post, in double entry: debits are
-- positive amounts, credits negative, and a document's entries add up to
-- zero. A journal entry made by hand (general journal document) keeps its
-- rows here too.
CREATE TABLE journal_entries (
    id                   bigint PRIMARY KEY,
    administration_id    bigint  NOT NULL REFERENCES administrations (id) ON DELETE CASCADE,
    document_type        text    NOT NULL,
    document_id          bigint  NOT NULL,
    date                 date    NOT NULL,
    ledger_account_id    bigint  NOT NULL REFERENCES ledger_accounts (id),
    contact_id           bigint  REFERENCES contacts (id) ON DELETE SET NULL,
    project_id           bigint,
    tax_rate_id          bigint  REFERENCES tax_rates (id) ON DELETE SET NULL,
    financial_account_id bigint  REFERENCES financial_accounts (id) ON DELETE SET NULL,
    amount               bigint  NOT NULL,
    description          text    NOT NULL DEFAULT '',
    row_order            integer NOT NULL DEFAULT 0,
    created_at           timestamptz NOT NULL,
    updated_at           timestamptz NOT NULL
);
CREATE INDEX journal_entries_admin_date_idx ON journal_entries (administration_id, date);
CREATE INDEX journal_entries_document_idx ON journal_entries (document_id);
CREATE INDEX journal_entries_ledger_account_idx ON journal_entries (ledger_account_id);
CREATE INDEX journal_entries_contact_idx ON journal_entries (contact_id);

CREATE TABLE general_journal_documents (
    id                bigint PRIMARY KEY,
    administration_id bigint NOT NULL REFERENCES administrations (id) ON DELETE CASCADE,
    reference         text   NOT NULL,
    date              date   NOT NULL,
    journal_type      text   NOT NULL DEFAULT '',
    created_at        timestamptz NOT NULL,
    updated_at        timestamptz NOT NULL
);
CREATE INDEX general_journal_documents_admin_date_idx ON general_journal_documents (administration_id, date);

-- attachments are files on any record; entity_type says which kind.
CREATE TABLE attachments (
    id                bigint PRIMARY KEY,
    administration_id bigint NOT NULL REFERENCES administrations (id) ON DELETE CASCADE,
    entity_type       text   NOT NULL,
    entity_id         bigint NOT NULL,
    filename          text   NOT NULL,
    content_type      text   NOT NULL,
    size              bigint NOT NULL,
    created_at        timestamptz NOT NULL,
    updated_at        timestamptz NOT NULL
);
CREATE INDEX attachments_entity_idx ON attachments (entity_id);

-- bank_journal_entries is what bank transactions post, derived from the
-- transactions and their bookings so it never runs out of step: the whole
-- amount on the account's category, each booking against its category, and
-- the part not booked yet against unprocessed transactions. The last row's
-- id sets bit 62 of the transaction's id, which no generated id reaches.
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

-- ledger_entries is the whole journal the reports read.
CREATE VIEW ledger_entries AS
SELECT id, administration_id, document_type, document_id, date, ledger_account_id, contact_id, project_id, tax_rate_id,
       financial_account_id, amount, description, row_order
FROM journal_entries
UNION ALL
SELECT * FROM bank_journal_entries;

-- +goose Down
DROP VIEW ledger_entries;
DROP VIEW bank_journal_entries;
DELETE FROM resources WHERE type = 'documents/general_journal_documents';
DROP TABLE attachments;
DROP TABLE general_journal_documents;
DROP TABLE journal_entries;
