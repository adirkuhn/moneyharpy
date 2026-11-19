-- +goose Up
-- Events of the application's own work, such as a bank connection's import,
-- have no user; this tells them apart from events whose user was removed.
ALTER TABLE events ADD COLUMN by_system boolean NOT NULL DEFAULT false;

-- +goose Down
ALTER TABLE events DROP COLUMN by_system;
