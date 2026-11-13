-- +goose Up
-- Scan and Recognise reads the attachments of incoming documents in the
-- background. document_recognitions holds what it did per document: one row
-- per typeless document, purchase invoice or receipt it read or is to read.
-- It is keyed by the document's id, which a document keeps when it changes
-- type, so the row follows an upload as it becomes a purchase invoice.
CREATE TABLE document_recognitions (
    document_id       bigint PRIMARY KEY,
    administration_id bigint  NOT NULL REFERENCES administrations (id) ON DELETE CASCADE,
    -- user_id is who uploaded the document or asked to read it again; the
    -- background work acts with their access.
    user_id           bigint  NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    -- state is none, pending, done or failed.
    state             text    NOT NULL,
    -- source is e_invoice, text, ocr, ai, or empty before reading.
    source            text    NOT NULL DEFAULT '',
    -- fields are the fields filled in: [{"field": ..., "confidence": ...}].
    fields            jsonb   NOT NULL DEFAULT '[]',
    error             text    NOT NULL DEFAULT '',
    attempts          integer NOT NULL DEFAULT 0,
    -- next_attempt_at is when a pending document is (next) read; the worker
    -- moves it ahead while it reads, as a lease.
    next_attempt_at   timestamptz,
    created_at        timestamptz NOT NULL,
    updated_at        timestamptz NOT NULL
);
CREATE INDEX document_recognitions_due_idx ON document_recognitions (next_attempt_at) WHERE state = 'pending';
CREATE INDEX document_recognitions_admin_idx ON document_recognitions (administration_id);

-- supplier_memories remember per supplier (contact) what users saved on its
-- documents, to fill in the next one. Each value counts the documents in a
-- row that agreed on it; two or more make it certain.
CREATE TABLE supplier_memories (
    administration_id        bigint  NOT NULL REFERENCES administrations (id) ON DELETE CASCADE,
    contact_id               bigint  NOT NULL REFERENCES contacts (id) ON DELETE CASCADE,
    document_kind            text    NOT NULL DEFAULT '',
    document_kind_uses       integer NOT NULL DEFAULT 0,
    currency                 text    NOT NULL DEFAULT '',
    currency_uses            integer NOT NULL DEFAULT 0,
    prices_are_incl_tax      boolean NOT NULL DEFAULT false,
    prices_are_incl_tax_uses integer NOT NULL DEFAULT 0,
    updated_at               timestamptz NOT NULL,
    PRIMARY KEY (administration_id, contact_id)
);
CREATE INDEX supplier_memories_contact_idx ON supplier_memories (contact_id);

-- supplier_memory_lines remember the category and VAT rate per normalised
-- line description; the empty description is the supplier's default.
CREATE TABLE supplier_memory_lines (
    administration_id bigint  NOT NULL,
    contact_id        bigint  NOT NULL,
    description       text    NOT NULL,
    ledger_account_id bigint,
    tax_rate_id       bigint,
    uses              integer NOT NULL DEFAULT 0,
    last_used_at      timestamptz NOT NULL,
    PRIMARY KEY (administration_id, contact_id, description),
    FOREIGN KEY (administration_id, contact_id) REFERENCES supplier_memories (administration_id, contact_id) ON DELETE CASCADE
);

-- +goose Down
DROP TABLE supplier_memory_lines;
DROP TABLE supplier_memories;
DROP TABLE document_recognitions;
