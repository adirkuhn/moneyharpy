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
       a.small_business_scheme, a.company_activities_description, a.company_website_url, a.company_website_absent,
       m.access, m.favorite
FROM administrations a JOIN administration_members m ON m.administration_id = a.id
WHERE m.user_id = $1;

-- name: AdministrationForUser :one
SELECT a.id, a.name, a.language, a.currency, a.country, a.time_zone, a.suspended,
       a.period_locked_until, a.period_start_date,
       a.small_business_scheme, a.company_activities_description, a.company_website_url, a.company_website_absent,
       m.access, m.favorite
FROM administrations a JOIN administration_members m ON m.administration_id = a.id
WHERE a.id = $1 AND m.user_id = $2;

-- name: SetFavorite :execrows
UPDATE administration_members SET favorite = $3 WHERE administration_id = $1 AND user_id = $2;

-- name: LocateResource :one
SELECT r.id, r.type, r.administration_id
FROM resources r JOIN administration_members m ON m.administration_id = r.administration_id
WHERE r.id = $1 AND m.user_id = $2;

-- name: DeleteResetTokens :exec
DELETE FROM password_reset_tokens WHERE user_id = $1;

-- name: InsertResetToken :exec
INSERT INTO password_reset_tokens (token_hash, user_id, expires_at) VALUES ($1, $2, $3);

-- name: ConsumeResetToken :one
DELETE FROM password_reset_tokens WHERE token_hash = $1 AND expires_at > $2 RETURNING user_id;

-- name: UpdatePassword :exec
UPDATE users SET password_hash = $2 WHERE id = $1;

-- name: DeleteUserSessions :exec
DELETE FROM sessions WHERE user_id = $1;

-- name: UpdateAdministration :exec
UPDATE administrations
SET name = $2, language = $3, currency = $4, time_zone = $5, small_business_scheme = $6,
    company_activities_description = $7, company_website_url = $8, company_website_absent = $9
WHERE id = $1;

-- name: InsertIdentity :exec
INSERT INTO identities (id, administration_id, is_default, company_name, country, city, zipcode, address1, address2,
                        email, phone, legal_entity_type, chamber_of_commerce, tax_number,
                        bank_account_name, bank_account_number, bank_account_bic, tax_number_absence_reason,
                        created_at, updated_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20);

-- name: ListIdentities :many
SELECT * FROM identities WHERE administration_id = $1 ORDER BY is_default DESC, lower(company_name), id;

-- name: DefaultIdentity :one
SELECT * FROM identities WHERE administration_id = $1 AND is_default;

-- name: IdentityByID :one
SELECT * FROM identities WHERE administration_id = $1 AND id = $2;

-- name: UpdateIdentity :exec
UPDATE identities
SET company_name = $3, country = $4, city = $5, zipcode = $6, address1 = $7, address2 = $8, email = $9, phone = $10,
    legal_entity_type = $11, chamber_of_commerce = $12, tax_number = $13, bank_account_name = $14,
    bank_account_number = $15, bank_account_bic = $16, tax_number_absence_reason = $17, updated_at = $18
WHERE administration_id = $1 AND id = $2;

-- name: DeleteIdentity :execrows
DELETE FROM identities WHERE administration_id = $1 AND id = $2;

-- name: DeleteResource :exec
DELETE FROM resources WHERE id = $1;

-- name: InsertCustomField :exec
INSERT INTO custom_fields (id, administration_id, name, source, created_at, updated_at) VALUES ($1, $2, $3, $4, $5, $6);

-- name: ListCustomFields :many
SELECT * FROM custom_fields WHERE administration_id = $1 ORDER BY id;

-- name: ListIdentityCustomFieldValues :many
SELECT v.identity_id, f.id, f.name, v.value
FROM identity_custom_field_values v JOIN custom_fields f ON f.id = v.custom_field_id
WHERE f.administration_id = $1
ORDER BY f.id;

-- name: DeleteIdentityCustomFieldValues :exec
DELETE FROM identity_custom_field_values WHERE identity_id = $1;

-- name: InsertIdentityCustomFieldValue :exec
INSERT INTO identity_custom_field_values (identity_id, custom_field_id, value) VALUES ($1, $2, $3);
