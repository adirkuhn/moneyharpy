-- Assets.

-- name: ListAssets :many
SELECT sqlc.embed(a), count(*) OVER () AS total
FROM assets a
WHERE a.administration_id = @administration_id
  AND (sqlc.narg(ledger_account_id)::bigint IS NULL OR a.ledger_account_id = sqlc.narg(ledger_account_id)::bigint)
  AND (NOT @only_active::boolean OR a.disposal_id IS NULL)
ORDER BY a.purchase_date, a.id
LIMIT sqlc.narg(max_rows) OFFSET @skip;

-- name: AssetByID :one
SELECT * FROM assets WHERE administration_id = $1 AND id = $2;

-- name: AssetValueChanges :many
SELECT * FROM asset_value_changes WHERE administration_id = @administration_id AND asset_id = ANY(@ids::bigint[])
ORDER BY date, created_at, id;

-- name: AssetSources :many
SELECT * FROM asset_sources WHERE administration_id = @administration_id AND asset_id = ANY(@ids::bigint[])
ORDER BY created_at, id;

-- name: UpsertAsset :exec
INSERT INTO assets (id, administration_id, ledger_account_id, name, purchase_date, purchase_value, lifespan_months, residual_value,
    disposal_id, disposal_date, disposal_reason, disposal_externally_booked, reserve_sale_id, reserve_sale_amount,
    reserve_purchase_id, reserve_purchase_amount, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18)
ON CONFLICT (id) DO UPDATE SET ledger_account_id = EXCLUDED.ledger_account_id, name = EXCLUDED.name,
    purchase_date = EXCLUDED.purchase_date, purchase_value = EXCLUDED.purchase_value, lifespan_months = EXCLUDED.lifespan_months,
    residual_value = EXCLUDED.residual_value, disposal_id = EXCLUDED.disposal_id, disposal_date = EXCLUDED.disposal_date,
    disposal_reason = EXCLUDED.disposal_reason, disposal_externally_booked = EXCLUDED.disposal_externally_booked,
    reserve_sale_id = EXCLUDED.reserve_sale_id, reserve_sale_amount = EXCLUDED.reserve_sale_amount,
    reserve_purchase_id = EXCLUDED.reserve_purchase_id, reserve_purchase_amount = EXCLUDED.reserve_purchase_amount,
    updated_at = EXCLUDED.updated_at;

-- name: DeleteAssetValueChanges :exec
DELETE FROM asset_value_changes WHERE administration_id = $1 AND asset_id = $2;

-- name: InsertAssetValueChange :exec
INSERT INTO asset_value_changes (id, administration_id, asset_id, type, date, amount, description, externally_booked, created_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9);

-- name: DeleteAssetSources :exec
DELETE FROM asset_sources WHERE administration_id = $1 AND asset_id = $2;

-- name: InsertAssetSource :exec
INSERT INTO asset_sources (id, administration_id, asset_id, detail_id, general_journal_document_entry_id, created_at)
VALUES ($1, $2, $3, $4, $5, $6);

-- name: DeleteAsset :exec
DELETE FROM assets WHERE administration_id = $1 AND id = $2;

-- name: ReinvestmentReserveBalance :one
-- What sales put in the reinvestment reserve minus what purchases took out.
SELECT (coalesce(sum(reserve_sale_amount), 0) - coalesce(sum(reserve_purchase_amount), 0))::bigint
FROM assets WHERE administration_id = $1;

-- name: PurchaseDetailLedgerAccount :one
-- The category of a purchase invoice or receipt line; null while it has none.
SELECT ledger_account_id FROM purchase_document_details WHERE administration_id = $1 AND id = $2;

-- name: JournalDocumentEntryLedgerAccount :one
-- The category of a row of a journal entry (general journal document).
SELECT ledger_account_id FROM journal_entries WHERE administration_id = $1 AND id = $2 AND document_type = 'GeneralJournalDocument';

-- name: AssetWorkload :many
-- Administrations with assets that may still depreciate, each with its
-- longest-standing active owner.
SELECT w.administration_id, o.user_id
FROM (SELECT DISTINCT administration_id FROM assets WHERE lifespan_months IS NOT NULL AND disposal_id IS NULL) w
JOIN LATERAL (
    SELECT m.user_id FROM administration_members m
    WHERE m.administration_id = w.administration_id AND m.active AND 'ownership' = ANY(m.permissions)
    ORDER BY m.created_at, m.user_id LIMIT 1
) o ON true
ORDER BY w.administration_id;
