-- +goose Up
-- Online acceptance of estimates, after Moneybird's: the code that opens the
-- estimate online, the sender's own signature and the contact's signature
-- with what is kept as evidence of it (name, email, address, browser and the
-- strokes drawn). One row per estimate, made when it is first sent to be
-- signed online.
CREATE TABLE estimate_signings (
    estimate_id           bigint      PRIMARY KEY REFERENCES estimates (id) ON DELETE CASCADE,
    administration_id     bigint      NOT NULL,
    view_code             text        NOT NULL DEFAULT '',
    view_code_expires_at  timestamptz,
    -- The user who sent it; the contact's online actions are done as them.
    sent_by_user_id       bigint,
    sender_signature      bytea,
    sender_signed_at      timestamptz,
    sender_user_id        bigint,
    signer_name           text        NOT NULL DEFAULT '',
    signer_email          text        NOT NULL DEFAULT '',
    signer_ip             text        NOT NULL DEFAULT '',
    signer_user_agent     text        NOT NULL DEFAULT '',
    signature             bytea,
    audit_trail           text        NOT NULL DEFAULT '',
    signed_at             timestamptz,
    updated_at            timestamptz NOT NULL
);

-- +goose Down
DROP TABLE estimate_signings;
