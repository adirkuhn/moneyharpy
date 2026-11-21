-- Imports from Moneybird and what they brought in.

-- name: MoneybirdImport :one
SELECT * FROM moneybird_imports WHERE administration_id = $1;

-- name: SaveMoneybirdImport :exec
INSERT INTO moneybird_imports (administration_id, user_id, source_administration_id, source_name, state, steps, waiting_until, error, token, attempts, next_attempt_at, started_at, finished_at, updated_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
ON CONFLICT (administration_id) DO UPDATE SET
    user_id = excluded.user_id, source_administration_id = excluded.source_administration_id, source_name = excluded.source_name,
    state = excluded.state, steps = excluded.steps, waiting_until = excluded.waiting_until, error = excluded.error, token = excluded.token,
    attempts = excluded.attempts, next_attempt_at = excluded.next_attempt_at, started_at = excluded.started_at,
    finished_at = excluded.finished_at, updated_at = excluded.updated_at;

-- name: LeaseDueMoneybirdImports :many
-- Takes running imports that are due, moving them to @lease so no other
-- instance works on them meanwhile.
UPDATE moneybird_imports i SET next_attempt_at = @lease
WHERE i.administration_id IN (
    SELECT x.administration_id FROM moneybird_imports x
    WHERE x.state = 'running' AND x.next_attempt_at <= @now
    ORDER BY x.next_attempt_at LIMIT @max_count FOR UPDATE SKIP LOCKED
)
RETURNING i.*;

-- name: MoneybirdImportedID :one
SELECT target_id FROM moneybird_import_ids WHERE administration_id = $1 AND kind = $2 AND source_id = $3;

-- name: MoneybirdImportedAt :one
SELECT imported_at FROM moneybird_import_ids WHERE administration_id = $1 AND kind = $2 AND source_id = $3;

-- name: RecordMoneybirdImportedID :exec
INSERT INTO moneybird_import_ids (administration_id, kind, source_id, target_id, imported_at) VALUES ($1, $2, $3, $4, $5)
ON CONFLICT (administration_id, kind, source_id) DO UPDATE SET target_id = excluded.target_id;

-- name: AdministrationHasData :one
-- Whether the administration holds any of the data an import brings.
SELECT EXISTS (SELECT 1 FROM contacts t WHERE t.administration_id = @administration_id::bigint)
    OR EXISTS (SELECT 1 FROM sales_invoices t WHERE t.administration_id = @administration_id::bigint)
    OR EXISTS (SELECT 1 FROM external_sales_invoices t WHERE t.administration_id = @administration_id::bigint)
    OR EXISTS (SELECT 1 FROM recurring_sales_invoices t WHERE t.administration_id = @administration_id::bigint)
    OR EXISTS (SELECT 1 FROM estimates t WHERE t.administration_id = @administration_id::bigint)
    OR EXISTS (SELECT 1 FROM purchase_documents t WHERE t.administration_id = @administration_id::bigint)
    OR EXISTS (SELECT 1 FROM general_documents t WHERE t.administration_id = @administration_id::bigint)
    OR EXISTS (SELECT 1 FROM general_journal_documents t WHERE t.administration_id = @administration_id::bigint)
    OR EXISTS (SELECT 1 FROM typeless_documents t WHERE t.administration_id = @administration_id::bigint)
    OR EXISTS (SELECT 1 FROM financial_mutations t WHERE t.administration_id = @administration_id::bigint)
    OR EXISTS (SELECT 1 FROM products t WHERE t.administration_id = @administration_id::bigint)
    OR EXISTS (SELECT 1 FROM projects t WHERE t.administration_id = @administration_id::bigint)
    OR EXISTS (SELECT 1 FROM time_entries t WHERE t.administration_id = @administration_id::bigint)
    OR EXISTS (SELECT 1 FROM assets t WHERE t.administration_id = @administration_id::bigint) AS has_data;

-- name: RecordMoneybirdImportProblem :exec
INSERT INTO moneybird_import_problems (administration_id, step, kind, source_id, message, partial, created_at)
VALUES ($1, $2, $3, $4, $5, $6, $7)
ON CONFLICT (administration_id, kind, source_id, message) DO UPDATE SET step = excluded.step, created_at = excluded.created_at;

-- name: ResolveMoneybirdImportProblems :exec
-- A record that came in no longer failed; what came in without it stays.
DELETE FROM moneybird_import_problems WHERE administration_id = $1 AND kind = $2 AND source_id = $3 AND NOT partial;

-- name: MoneybirdImportPartials :many
-- What the records of a kind came in without.
SELECT source_id, message FROM moneybird_import_problems WHERE administration_id = $1 AND kind = $2 AND partial;

-- name: ForgetMoneybirdImportProblem :exec
DELETE FROM moneybird_import_problems WHERE administration_id = $1 AND kind = $2 AND source_id = $3 AND message = $4;

-- name: MoneybirdImportProblems :many
SELECT * FROM moneybird_import_problems WHERE administration_id = $1 ORDER BY step, kind, source_id, message LIMIT $2 OFFSET $3;

-- name: CountMoneybirdImportProblems :one
SELECT count(*) FROM moneybird_import_problems WHERE administration_id = $1;
