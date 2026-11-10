-- +goose Up
-- The overview counts what is left to link and lists what was processed last.
CREATE INDEX financial_mutations_admin_state_idx ON financial_mutations (administration_id, state, processed_at DESC);

-- +goose Down
DROP INDEX financial_mutations_admin_state_idx;
