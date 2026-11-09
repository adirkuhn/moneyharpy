-- name: LockFirstUser :exec
SELECT pg_advisory_xact_lock(727001);

-- name: InsertUser :one
INSERT INTO users (id, email, name, password_hash, is_instance_admin)
VALUES ($1, $2, $3, $4, NOT EXISTS (SELECT 1 FROM users))
RETURNING id, email, name, is_instance_admin;

-- name: UserByEmail :one
SELECT id, email, name, is_instance_admin, password_hash FROM users WHERE email = $1;

-- name: UserByIdentity :one
SELECT u.id, u.email, u.name, u.is_instance_admin
FROM users u JOIN user_identities i ON i.user_id = u.id
WHERE i.provider = $1 AND i.subject = $2;

-- name: LinkIdentity :exec
INSERT INTO user_identities (provider, subject, user_id) VALUES ($1, $2, $3)
ON CONFLICT DO NOTHING;

-- name: InsertSession :exec
INSERT INTO sessions (token_hash, user_id, expires_at) VALUES ($1, $2, $3);

-- name: UserBySession :one
SELECT u.id, u.email, u.name, u.is_instance_admin
FROM users u JOIN sessions s ON s.user_id = u.id
WHERE s.token_hash = $1 AND s.expires_at > $2;

-- name: DeleteSession :exec
DELETE FROM sessions WHERE token_hash = $1;

-- name: InsertToken :exec
INSERT INTO api_tokens (id, user_id, name, token_hash, created_at) VALUES ($1, $2, $3, $4, $5);

-- name: UserByToken :one
SELECT u.id, u.email, u.name, u.is_instance_admin
FROM users u JOIN api_tokens t ON t.user_id = u.id
WHERE t.token_hash = $1;

-- name: ListTokens :many
SELECT id, user_id, name, created_at FROM api_tokens WHERE user_id = $1 ORDER BY created_at DESC, id DESC;

-- name: DeleteToken :execrows
DELETE FROM api_tokens WHERE id = $1 AND user_id = $2;

-- name: LoadSetting :one
SELECT value FROM settings WHERE key = $1;

-- name: SaveSetting :exec
INSERT INTO settings (key, value) VALUES ($1, $2)
ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value, updated_at = now();

-- name: InsertAdministration :exec
INSERT INTO administrations (id, name, language, currency, country, time_zone, suspended, period_locked_until, period_start_date)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9);

-- name: InsertMember :exec
INSERT INTO administration_members (administration_id, user_id, access) VALUES ($1, $2, $3);

-- name: InsertResource :exec
INSERT INTO resources (id, type, administration_id) VALUES ($1, $2, $3);

-- name: ListAdministrations :many
SELECT a.id, a.name, a.language, a.currency, a.country, a.time_zone, a.suspended,
       a.period_locked_until, a.period_start_date,
       m.access, m.favorite
FROM administrations a JOIN administration_members m ON m.administration_id = a.id
WHERE m.user_id = $1;

-- name: AdministrationForUser :one
SELECT a.id, a.name, a.language, a.currency, a.country, a.time_zone, a.suspended,
       a.period_locked_until, a.period_start_date,
       m.access, m.favorite
FROM administrations a JOIN administration_members m ON m.administration_id = a.id
WHERE a.id = $1 AND m.user_id = $2;

-- name: SetFavorite :execrows
UPDATE administration_members SET favorite = $3 WHERE administration_id = $1 AND user_id = $2;

-- name: LocateResource :one
SELECT r.id, r.type, r.administration_id
FROM resources r JOIN administration_members m ON m.administration_id = r.administration_id
WHERE r.id = $1 AND m.user_id = $2;
