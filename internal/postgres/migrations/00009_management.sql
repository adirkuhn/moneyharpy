-- +goose Up
-- Members get Moneybird's user types and permissions. Everyone who was a
-- member so far created or joined as an owner.
ALTER TABLE administration_members
    ADD COLUMN user_type   text    NOT NULL DEFAULT 'owner',
    ADD COLUMN permissions text[]  NOT NULL DEFAULT '{sales_invoices,documents,estimates,bank,settings,ownership,time_entries}',
    ADD COLUMN active      boolean NOT NULL DEFAULT true,
    ADD COLUMN created_at  timestamptz NOT NULL DEFAULT now(),
    ADD COLUMN updated_at  timestamptz NOT NULL DEFAULT now();

ALTER TABLE administrations
    ADD COLUMN require_two_factor   boolean NOT NULL DEFAULT false,
    ADD COLUMN support_access_until timestamptz;

CREATE TABLE user_invitations (
    id                bigint PRIMARY KEY,
    administration_id bigint  NOT NULL REFERENCES administrations (id) ON DELETE CASCADE,
    email             text    NOT NULL,
    user_type         text    NOT NULL,
    permissions       text[]  NOT NULL,
    token_hash        text    NOT NULL UNIQUE,
    invited_by        bigint  REFERENCES users (id) ON DELETE SET NULL,
    created_at        timestamptz NOT NULL,
    expires_at        timestamptz NOT NULL,
    accepted_at       timestamptz,
    accepted_by       bigint  REFERENCES users (id) ON DELETE SET NULL
);
CREATE INDEX user_invitations_administration_id_idx ON user_invitations (administration_id);
CREATE UNIQUE INDEX user_invitations_open_idx ON user_invitations (administration_id, lower(email)) WHERE accepted_at IS NULL;

-- Two-step verification: an authenticator secret, sealed with the instance
-- key, and single-use recovery codes.
ALTER TABLE users
    ADD COLUMN totp_secret     text   NOT NULL DEFAULT '',
    ADD COLUMN totp_enabled_at timestamptz,
    ADD COLUMN totp_last_step  bigint NOT NULL DEFAULT 0;

CREATE TABLE recovery_codes (
    user_id   bigint NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    code_hash text   NOT NULL,
    used_at   timestamptz,
    PRIMARY KEY (user_id, code_hash)
);

-- A password or provider sign-in waiting for its second step.
CREATE TABLE login_challenges (
    token_hash text PRIMARY KEY,
    user_id    bigint NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    attempts   integer NOT NULL DEFAULT 0,
    expires_at timestamptz NOT NULL
);
CREATE INDEX login_challenges_user_id_idx ON login_challenges (user_id);

-- +goose Down
DROP TABLE login_challenges;
DROP TABLE recovery_codes;
ALTER TABLE users DROP COLUMN totp_secret, DROP COLUMN totp_enabled_at, DROP COLUMN totp_last_step;
DROP TABLE user_invitations;
ALTER TABLE administrations DROP COLUMN require_two_factor, DROP COLUMN support_access_until;
ALTER TABLE administration_members
    DROP COLUMN user_type, DROP COLUMN permissions, DROP COLUMN active, DROP COLUMN created_at, DROP COLUMN updated_at;
