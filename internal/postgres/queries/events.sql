-- name: EventsOf :many
SELECT * FROM events
WHERE administration_id = @administration_id AND entity_type = @entity_type AND entity_id = ANY(@ids::bigint[])
ORDER BY created_at, id;

-- name: InsertEvent :exec
INSERT INTO events (id, administration_id, entity_type, entity_id, user_id, action, link_entity_type, link_entity_id, data, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11);

-- name: DeleteEventsOf :exec
DELETE FROM events WHERE administration_id = $1 AND entity_id = $2;

-- name: ListEvents :many
SELECT * FROM events
WHERE administration_id = @administration_id
  AND (sqlc.narg(entity_types)::text[] IS NULL OR entity_type = ANY(sqlc.narg(entity_types)::text[]))
  AND (sqlc.narg(entity_id)::bigint IS NULL OR entity_id = sqlc.narg(entity_id)::bigint)
  AND (sqlc.narg(user_id)::bigint IS NULL OR user_id = sqlc.narg(user_id)::bigint)
  AND (sqlc.narg(from_at)::timestamptz IS NULL OR created_at >= sqlc.narg(from_at)::timestamptz)
  AND (sqlc.narg(to_at)::timestamptz IS NULL OR created_at < sqlc.narg(to_at)::timestamptz)
  AND (sqlc.narg(after_at)::timestamptz IS NULL OR (created_at, id) < (sqlc.narg(after_at)::timestamptz, sqlc.narg(after_id)::bigint))
ORDER BY created_at DESC, id DESC
LIMIT @max_rows;
