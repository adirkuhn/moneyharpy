-- +goose Up
-- imported_at is when an import brought a record in, so an import that
-- stops and starts again counts what it brought in before as imported,
-- not as being here already. Records brought in before now count as the
-- current import's.
ALTER TABLE moneybird_import_ids ADD COLUMN imported_at timestamptz;
UPDATE moneybird_import_ids i SET imported_at = j.started_at
FROM moneybird_imports j WHERE j.administration_id = i.administration_id;
UPDATE moneybird_import_ids SET imported_at = '-infinity' WHERE imported_at IS NULL;
ALTER TABLE moneybird_import_ids ALTER COLUMN imported_at SET NOT NULL;

-- +goose Down
ALTER TABLE moneybird_import_ids DROP COLUMN imported_at;
