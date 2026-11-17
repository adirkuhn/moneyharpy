-- +goose Up
CREATE TABLE bank_connections (
    id                bigint PRIMARY KEY,
    administration_id bigint NOT NULL REFERENCES administrations (id) ON DELETE CASCADE,
    provider          text   NOT NULL,
    bank              text   NOT NULL,
    country           text   NOT NULL,
    state             text   NOT NULL,
    session_id        text   NOT NULL DEFAULT '',
    valid_until       timestamptz,
    auth_state        text,
    created_by        bigint NOT NULL,
    last_sync_at      timestamptz,
    last_error        text   NOT NULL DEFAULT '',
    created_at        timestamptz NOT NULL,
    updated_at        timestamptz NOT NULL
);
CREATE INDEX bank_connections_administration_id_idx ON bank_connections (administration_id);
CREATE UNIQUE INDEX bank_connections_auth_state_idx ON bank_connections (auth_state) WHERE auth_state IS NOT NULL;
CREATE INDEX bank_connections_due_idx ON bank_connections (last_sync_at) WHERE state = 'active';

-- An account is fetched through one connection at a time.
CREATE TABLE bank_connection_accounts (
    financial_account_id bigint PRIMARY KEY REFERENCES financial_accounts (id) ON DELETE CASCADE,
    connection_id        bigint NOT NULL REFERENCES bank_connections (id) ON DELETE CASCADE,
    account_uid          text   NOT NULL,
    position             integer NOT NULL
);
CREATE INDEX bank_connection_accounts_connection_idx ON bank_connection_accounts (connection_id);

-- +goose Down
DROP TABLE bank_connection_accounts;
DROP TABLE bank_connections;
