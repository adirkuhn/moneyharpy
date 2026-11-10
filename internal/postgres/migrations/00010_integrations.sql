-- +goose Up
ALTER TABLE administrations ADD COLUMN users_may_connect boolean NOT NULL DEFAULT false;

-- API tokens and AI connections that act for one user in one administration,
-- limited to scopes and, optionally, to reading.
CREATE TABLE connections (
    id                bigint PRIMARY KEY,
    administration_id bigint  NOT NULL REFERENCES administrations (id) ON DELETE CASCADE,
    user_id           bigint  NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    kind              text    NOT NULL,
    name              text    NOT NULL,
    scopes            text[]  NOT NULL,
    read_only         boolean NOT NULL DEFAULT false,
    token_hash        text    NOT NULL UNIQUE,
    created_at        timestamptz NOT NULL,
    last_used_at      timestamptz
);
CREATE INDEX connections_administration_id_idx ON connections (administration_id);
CREATE INDEX connections_user_id_idx ON connections (user_id);

CREATE TABLE webhooks (
    id                bigint PRIMARY KEY,
    administration_id bigint  NOT NULL REFERENCES administrations (id) ON DELETE CASCADE,
    url               text    NOT NULL,
    enabled_events    text[]  NOT NULL DEFAULT '{}',
    token             text    NOT NULL,
    secret_sealed     text    NOT NULL,
    connection_id     bigint  REFERENCES connections (id) ON DELETE CASCADE,
    last_http_status  integer,
    last_http_body    text,
    deactivated_at    timestamptz,
    created_at        timestamptz NOT NULL,
    updated_at        timestamptz NOT NULL
);
CREATE INDEX webhooks_administration_id_idx ON webhooks (administration_id);
CREATE INDEX webhooks_connection_id_idx ON webhooks (connection_id);

-- Deliveries wait here until the webhook answers 200 or the attempts run out.
CREATE TABLE webhook_deliveries (
    id              bigint PRIMARY KEY,
    webhook_id      bigint  NOT NULL REFERENCES webhooks (id) ON DELETE CASCADE,
    idempotency_key text    NOT NULL,
    payload         jsonb   NOT NULL,
    attempts        integer NOT NULL DEFAULT 0,
    next_attempt_at timestamptz,
    delivered_at    timestamptz,
    last_status     integer,
    last_error      text    NOT NULL DEFAULT '',
    created_at      timestamptz NOT NULL
);
CREATE INDEX webhook_deliveries_due_idx ON webhook_deliveries (next_attempt_at) WHERE next_attempt_at IS NOT NULL;
CREATE INDEX webhook_deliveries_webhook_id_idx ON webhook_deliveries (webhook_id);

-- +goose Down
DELETE FROM resources WHERE type IN ('webhooks');
DROP TABLE webhook_deliveries;
DROP TABLE webhooks;
DROP TABLE connections;
ALTER TABLE administrations DROP COLUMN users_may_connect;
