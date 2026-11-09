-- +goose Up
CREATE TABLE users (
    id                bigint PRIMARY KEY,
    email             text NOT NULL UNIQUE,
    name              text NOT NULL,
    password_hash     text NOT NULL DEFAULT '',
    is_instance_admin boolean NOT NULL DEFAULT false,
    created_at        timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE user_identities (
    provider text NOT NULL,
    subject  text NOT NULL,
    user_id  bigint NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    PRIMARY KEY (provider, subject)
);

CREATE TABLE sessions (
    token_hash text PRIMARY KEY,
    user_id    bigint NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    expires_at timestamptz NOT NULL
);
CREATE INDEX sessions_expires_at_idx ON sessions (expires_at);

CREATE TABLE api_tokens (
    id         bigint PRIMARY KEY,
    user_id    bigint NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    name       text NOT NULL,
    token_hash text NOT NULL UNIQUE,
    created_at timestamptz NOT NULL
);
CREATE INDEX api_tokens_user_id_idx ON api_tokens (user_id);

CREATE TABLE settings (
    key        text PRIMARY KEY,
    value      jsonb NOT NULL,
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE administrations (
    id                  bigint PRIMARY KEY,
    name                text NOT NULL,
    language            text NOT NULL,
    currency            text NOT NULL,
    country             text NOT NULL,
    time_zone           text NOT NULL,
    suspended           boolean NOT NULL DEFAULT false,
    period_locked_until date,
    period_start_date   date NOT NULL,
    created_at          timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE administration_members (
    administration_id bigint NOT NULL REFERENCES administrations (id) ON DELETE CASCADE,
    user_id           bigint NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    access            text NOT NULL,
    favorite          boolean NOT NULL DEFAULT false,
    PRIMARY KEY (administration_id, user_id)
);
CREATE INDEX administration_members_user_id_idx ON administration_members (user_id);

-- resources maps every administration-scoped entity to its location, so that
-- any ID can be resolved to a URI without knowing its type.
CREATE TABLE resources (
    id                bigint PRIMARY KEY,
    type              text NOT NULL,
    administration_id bigint NOT NULL REFERENCES administrations (id) ON DELETE CASCADE
);
CREATE INDEX resources_administration_id_idx ON resources (administration_id);

-- +goose Down
DROP TABLE resources;
DROP TABLE administration_members;
DROP TABLE administrations;
DROP TABLE settings;
DROP TABLE api_tokens;
DROP TABLE sessions;
DROP TABLE user_identities;
DROP TABLE users;
