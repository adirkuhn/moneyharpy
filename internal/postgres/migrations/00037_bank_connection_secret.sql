-- +goose Up
-- The sealed API key of banks connected with one, such as Revolut Business.
ALTER TABLE bank_connections ADD COLUMN secret text NOT NULL DEFAULT '';

-- +goose Down
ALTER TABLE bank_connections DROP COLUMN secret;
