-- name: EventsOf :many
SELECT * FROM events
WHERE administration_id = @administration_id AND entity_type = @entity_type AND entity_id = ANY(@ids::bigint[])
ORDER BY created_at, id;

-- name: InsertEvent :exec
INSERT INTO events (id, administration_id, entity_type, entity_id, user_id, action, link_entity_type, link_entity_id, data, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11);

-- name: DeleteEventsOf :exec
DELETE FROM events WHERE administration_id = $1 AND entity_id = $2;
