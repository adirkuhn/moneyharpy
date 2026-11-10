-- +goose Up
ALTER TABLE administrations
    ADD COLUMN vat_documents_period   text    NOT NULL DEFAULT 'quarter',
    ADD COLUMN icp_declaration_period text    NOT NULL DEFAULT 'quarter',
    ADD COLUMN chart_scheme           text    NOT NULL DEFAULT '',
    ADD COLUMN bookkeeping_seeded     boolean NOT NULL DEFAULT false;

CREATE TABLE tax_rates (
    id                bigint PRIMARY KEY,
    administration_id bigint       NOT NULL REFERENCES administrations (id) ON DELETE CASCADE,
    name              text         NOT NULL,
    percentage        numeric(5,2) NOT NULL CHECK (percentage BETWEEN 0 AND 100),
    tax_rate_type     text         NOT NULL,
    show_tax          boolean      NOT NULL DEFAULT true,
    active            boolean      NOT NULL DEFAULT true,
    country           text         NOT NULL DEFAULT '',
    report_boxes      text[]       NOT NULL DEFAULT '{}',
    position          integer      NOT NULL DEFAULT 0,
    created_at        timestamptz  NOT NULL,
    updated_at        timestamptz  NOT NULL
);
CREATE INDEX tax_rates_administration_id_idx ON tax_rates (administration_id);

CREATE TABLE ledger_accounts (
    id                     bigint PRIMARY KEY,
    administration_id      bigint  NOT NULL REFERENCES administrations (id) ON DELETE CASCADE,
    name                   text    NOT NULL,
    account_type           text    NOT NULL,
    account_id             text    NOT NULL DEFAULT '',
    parent_id              bigint  REFERENCES ledger_accounts (id) DEFERRABLE INITIALLY DEFERRED,
    active                 boolean NOT NULL DEFAULT true,
    allowed_document_types text[]  NOT NULL DEFAULT '{}',
    taxonomy_code          text    NOT NULL DEFAULT '',
    description            text    NOT NULL DEFAULT '',
    system_role            text    NOT NULL DEFAULT '',
    seed_key               text    NOT NULL DEFAULT '',
    template_key           text    NOT NULL DEFAULT '',
    created_at             timestamptz NOT NULL,
    updated_at             timestamptz NOT NULL
);
CREATE INDEX ledger_accounts_administration_id_idx ON ledger_accounts (administration_id);
CREATE INDEX ledger_accounts_parent_id_idx ON ledger_accounts (parent_id);
CREATE UNIQUE INDEX ledger_accounts_name_idx ON ledger_accounts (administration_id, account_type, lower(name));

-- +goose Down
DELETE FROM resources WHERE type IN ('tax_rates', 'ledger_accounts');
DROP TABLE ledger_accounts;
DROP TABLE tax_rates;
ALTER TABLE administrations
    DROP COLUMN vat_documents_period,
    DROP COLUMN icp_declaration_period,
    DROP COLUMN chart_scheme,
    DROP COLUMN bookkeeping_seeded;
