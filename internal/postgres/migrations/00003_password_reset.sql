-- +goose Up
CREATE TABLE password_reset_tokens (
    token_hash text PRIMARY KEY,
    user_id    bigint NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    expires_at timestamptz NOT NULL
);
CREATE INDEX password_reset_tokens_user_id_idx ON password_reset_tokens (user_id);

-- +goose Down
DROP TABLE password_reset_tokens;
