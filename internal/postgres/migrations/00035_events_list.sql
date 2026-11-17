-- +goose Up
-- The events list goes through an administration's events newest first.
CREATE INDEX events_list_idx ON events (administration_id, created_at DESC, id DESC);

-- +goose Down
DROP INDEX events_list_idx;
