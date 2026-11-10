-- +goose Up
ALTER TABLE administrations
    ADD COLUMN bank_auto_link boolean NOT NULL DEFAULT true,
    ADD COLUMN bank_seeded    boolean NOT NULL DEFAULT false;

CREATE TABLE financial_accounts (
    id                bigint PRIMARY KEY,
    administration_id bigint  NOT NULL REFERENCES administrations (id) ON DELETE CASCADE,
    type              text    NOT NULL,
    name              text    NOT NULL,
    identifier        text    NOT NULL DEFAULT '',
    currency          text    NOT NULL,
    provider          text    NOT NULL DEFAULT '',
    active            boolean NOT NULL DEFAULT true,
    position          integer NOT NULL DEFAULT 0,
    ledger_account_id bigint  NOT NULL REFERENCES ledger_accounts (id),
    last_import_at    timestamptz,
    created_at        timestamptz NOT NULL,
    updated_at        timestamptz NOT NULL
);
CREATE INDEX financial_accounts_administration_id_idx ON financial_accounts (administration_id);
CREATE INDEX financial_accounts_ledger_account_id_idx ON financial_accounts (ledger_account_id);
CREATE UNIQUE INDEX financial_accounts_name_idx ON financial_accounts (administration_id, lower(name));
CREATE UNIQUE INDEX financial_accounts_identifier_idx ON financial_accounts (administration_id, identifier) WHERE identifier <> '';

CREATE TABLE financial_statements (
    id                   bigint PRIMARY KEY,
    administration_id    bigint NOT NULL REFERENCES administrations (id) ON DELETE CASCADE,
    financial_account_id bigint NOT NULL REFERENCES financial_accounts (id) ON DELETE CASCADE,
    reference            text   NOT NULL,
    official_date        date,
    official_balance     bigint,
    importer             text   NOT NULL,
    created_at           timestamptz NOT NULL,
    updated_at           timestamptz NOT NULL
);
CREATE INDEX financial_statements_account_idx ON financial_statements (financial_account_id);

CREATE TABLE financial_mutations (
    id                     bigint PRIMARY KEY,
    administration_id      bigint  NOT NULL REFERENCES administrations (id) ON DELETE CASCADE,
    financial_account_id   bigint  NOT NULL REFERENCES financial_accounts (id) ON DELETE CASCADE,
    financial_statement_id bigint  NOT NULL REFERENCES financial_statements (id) ON DELETE CASCADE,
    date                   date    NOT NULL,
    valutation_date        date,
    message                text    NOT NULL DEFAULT '',
    amount                 bigint  NOT NULL,
    currency               text    NOT NULL,
    code                   text    NOT NULL DEFAULT '',
    contra_account_name    text    NOT NULL DEFAULT '',
    contra_account_number  text    NOT NULL DEFAULT '',
    batch_reference        text    NOT NULL DEFAULT '',
    transaction_id         text    NOT NULL DEFAULT '',
    fingerprint            text    NOT NULL DEFAULT '',
    position               integer NOT NULL DEFAULT 0,
    state                  text    NOT NULL DEFAULT 'unprocessed',
    processed_at           timestamptz,
    created_at             timestamptz NOT NULL,
    updated_at             timestamptz NOT NULL
);
CREATE INDEX financial_mutations_admin_date_idx ON financial_mutations (administration_id, date);
CREATE INDEX financial_mutations_statement_idx ON financial_mutations (financial_statement_id);
CREATE UNIQUE INDEX financial_mutations_transaction_idx ON financial_mutations (financial_account_id, transaction_id) WHERE transaction_id <> '';
CREATE UNIQUE INDEX financial_mutations_fingerprint_idx ON financial_mutations (financial_account_id, fingerprint) WHERE fingerprint <> '';

CREATE TABLE booking_rules (
    id                   bigint PRIMARY KEY,
    administration_id    bigint  NOT NULL REFERENCES administrations (id) ON DELETE CASCADE,
    name                 text    NOT NULL,
    active               boolean NOT NULL DEFAULT true,
    position             integer NOT NULL DEFAULT 0,
    match_all            boolean NOT NULL DEFAULT true,
    direction            text    NOT NULL DEFAULT 'any',
    financial_account_id bigint  REFERENCES financial_accounts (id) ON DELETE CASCADE,
    conditions           jsonb   NOT NULL DEFAULT '[]',
    ledger_account_id    bigint  NOT NULL REFERENCES ledger_accounts (id),
    description          text    NOT NULL DEFAULT '',
    created_at           timestamptz NOT NULL,
    updated_at           timestamptz NOT NULL
);
CREATE INDEX booking_rules_administration_id_idx ON booking_rules (administration_id);
CREATE INDEX booking_rules_ledger_account_id_idx ON booking_rules (ledger_account_id);

CREATE TABLE ledger_account_bookings (
    id                    bigint PRIMARY KEY,
    administration_id     bigint NOT NULL REFERENCES administrations (id) ON DELETE CASCADE,
    financial_mutation_id bigint NOT NULL REFERENCES financial_mutations (id) ON DELETE CASCADE,
    ledger_account_id     bigint NOT NULL REFERENCES ledger_accounts (id),
    description           text   NOT NULL DEFAULT '',
    price                 bigint NOT NULL,
    booking_rule_id       bigint REFERENCES booking_rules (id) ON DELETE SET NULL,
    created_at            timestamptz NOT NULL,
    updated_at            timestamptz NOT NULL
);
CREATE INDEX ledger_account_bookings_mutation_idx ON ledger_account_bookings (financial_mutation_id);
CREATE INDEX ledger_account_bookings_ledger_account_idx ON ledger_account_bookings (ledger_account_id);
CREATE INDEX ledger_account_bookings_rule_idx ON ledger_account_bookings (booking_rule_id);

-- +goose Down
DELETE FROM resources WHERE type IN ('financial_accounts', 'financial_statements', 'financial_mutations', 'booking_rules');
DROP TABLE ledger_account_bookings;
DROP TABLE booking_rules;
DROP TABLE financial_mutations;
DROP TABLE financial_statements;
DROP TABLE financial_accounts;
ALTER TABLE administrations
    DROP COLUMN bank_auto_link,
    DROP COLUMN bank_seeded;
