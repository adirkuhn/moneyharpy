-- +goose Up
-- Scan and Recognise remembers a supplier's purchase invoices and receipts
-- apart (document_kind purchase_invoice or receipt), with the document each
-- remembered value came from. supplier_memories keeps what the supplier's
-- documents are: their kind.
CREATE TABLE supplier_memory_kinds (
    administration_id        bigint  NOT NULL,
    contact_id               bigint  NOT NULL,
    document_kind            text    NOT NULL,
    currency                 text    NOT NULL DEFAULT '',
    currency_uses            integer NOT NULL DEFAULT 0,
    prices_are_incl_tax      boolean NOT NULL DEFAULT false,
    prices_are_incl_tax_uses integer NOT NULL DEFAULT 0,
    -- The last document learned from; no reference, as it may be removed.
    last_document_id         bigint,
    updated_at               timestamptz NOT NULL,
    PRIMARY KEY (administration_id, contact_id, document_kind),
    FOREIGN KEY (administration_id, contact_id) REFERENCES supplier_memories (administration_id, contact_id) ON DELETE CASCADE
);

-- What was remembered is kept for the kind it was learned from: the kind
-- of the supplier's booked documents when they are all of one kind, else
-- both kinds.
INSERT INTO supplier_memory_kinds (administration_id, contact_id, document_kind, currency, currency_uses, prices_are_incl_tax,
                                   prices_are_incl_tax_uses, updated_at)
SELECT m.administration_id, m.contact_id, k.document_kind, m.currency, m.currency_uses, m.prices_are_incl_tax,
       m.prices_are_incl_tax_uses, m.updated_at
FROM supplier_memories m
CROSS JOIN (VALUES ('purchase_invoice', 'PurchaseInvoice'), ('receipt', 'Receipt')) AS k (document_kind, kind)
WHERE EXISTS (
        SELECT 1 FROM purchase_documents d
        WHERE d.administration_id = m.administration_id AND d.contact_id = m.contact_id AND d.state <> 'new' AND d.kind = k.kind)
   OR NOT EXISTS (
        SELECT 1 FROM purchase_documents d
        WHERE d.administration_id = m.administration_id AND d.contact_id = m.contact_id AND d.state <> 'new'
          AND d.kind IN ('PurchaseInvoice', 'Receipt'));

ALTER TABLE supplier_memory_lines
    DROP CONSTRAINT supplier_memory_lines_pkey,
    ADD COLUMN document_kind text NOT NULL DEFAULT '',
    -- The last document that taught the line; no reference, as it may be
    -- removed.
    ADD COLUMN source_document_id bigint;
INSERT INTO supplier_memory_lines (administration_id, contact_id, document_kind, description, ledger_account_id, tax_rate_id, uses, last_used_at)
SELECT l.administration_id, l.contact_id, k.document_kind, l.description, l.ledger_account_id, l.tax_rate_id, l.uses, l.last_used_at
FROM supplier_memory_lines l
JOIN supplier_memory_kinds k ON k.administration_id = l.administration_id AND k.contact_id = l.contact_id
WHERE l.document_kind = '';
DELETE FROM supplier_memory_lines WHERE document_kind = '';
ALTER TABLE supplier_memory_lines
    ALTER COLUMN document_kind DROP DEFAULT,
    ADD PRIMARY KEY (administration_id, contact_id, document_kind, description),
    ADD CONSTRAINT supplier_memory_lines_kind_fkey FOREIGN KEY (administration_id, contact_id, document_kind)
        REFERENCES supplier_memory_kinds (administration_id, contact_id, document_kind) ON DELETE CASCADE;

ALTER TABLE supplier_memories
    DROP COLUMN currency,
    DROP COLUMN currency_uses,
    DROP COLUMN prices_are_incl_tax,
    DROP COLUMN prices_are_incl_tax_uses;

-- Scan and Recognise learns a supplier from its most recent booked
-- documents of a kind.
CREATE INDEX purchase_documents_admin_contact_kind_date_idx ON purchase_documents (administration_id, contact_id, kind, date DESC, id DESC)
    WHERE state <> 'new';

-- +goose Down
DROP INDEX purchase_documents_admin_contact_kind_date_idx;

ALTER TABLE supplier_memories
    ADD COLUMN currency                 text    NOT NULL DEFAULT '',
    ADD COLUMN currency_uses            integer NOT NULL DEFAULT 0,
    ADD COLUMN prices_are_incl_tax      boolean NOT NULL DEFAULT false,
    ADD COLUMN prices_are_incl_tax_uses integer NOT NULL DEFAULT 0;
-- Each supplier keeps what its most recently learned kind taught.
UPDATE supplier_memories m SET
    currency = k.currency, currency_uses = k.currency_uses, prices_are_incl_tax = k.prices_are_incl_tax,
    prices_are_incl_tax_uses = k.prices_are_incl_tax_uses
FROM (
    SELECT DISTINCT ON (administration_id, contact_id) *
    FROM supplier_memory_kinds
    ORDER BY administration_id, contact_id, updated_at DESC, document_kind
) k
WHERE k.administration_id = m.administration_id AND k.contact_id = m.contact_id;

ALTER TABLE supplier_memory_lines DROP CONSTRAINT supplier_memory_lines_kind_fkey;
ALTER TABLE supplier_memory_lines DROP CONSTRAINT supplier_memory_lines_pkey;
DELETE FROM supplier_memory_lines l
WHERE EXISTS (
    SELECT 1 FROM supplier_memory_lines o
    WHERE o.administration_id = l.administration_id AND o.contact_id = l.contact_id AND o.description = l.description
      AND (o.last_used_at, o.document_kind) > (l.last_used_at, l.document_kind));
ALTER TABLE supplier_memory_lines
    DROP COLUMN document_kind,
    DROP COLUMN source_document_id,
    ADD PRIMARY KEY (administration_id, contact_id, description);

DROP TABLE supplier_memory_kinds;
