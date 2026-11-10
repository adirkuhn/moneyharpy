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
INSERT INTO custom_fields (id, administration_id, name, source, position, created_at, updated_at) VALUES ($1, $2, $3, $4, $5, $6, $7);

-- name: ListCustomFields :many
SELECT * FROM custom_fields WHERE administration_id = $1 ORDER BY position, id;

-- name: CustomFieldByID :one
SELECT * FROM custom_fields WHERE administration_id = $1 AND id = $2;

-- name: UpdateCustomField :exec
UPDATE custom_fields SET name = $3, position = $4, updated_at = $5 WHERE administration_id = $1 AND id = $2;

-- name: DeleteCustomField :execrows
DELETE FROM custom_fields WHERE administration_id = $1 AND id = $2;

-- name: ListIdentityCustomFieldValues :many
SELECT v.identity_id, f.id, f.name, v.value
FROM identity_custom_field_values v JOIN custom_fields f ON f.id = v.custom_field_id
WHERE f.administration_id = $1
ORDER BY f.id;

-- name: DeleteIdentityCustomFieldValues :exec
DELETE FROM identity_custom_field_values WHERE identity_id = $1;

-- name: InsertIdentityCustomFieldValue :exec
INSERT INTO identity_custom_field_values (identity_id, custom_field_id, value) VALUES ($1, $2, $3);

-- name: InvoiceSettings :one
SELECT invoice_id_template, invoice_sequence_id, estimate_id_template, estimate_sequence_id,
       invoice_bcc_email_address, invoice_reminders_bcc_enabled, estimate_bcc_email_address, order_bcc_email_address,
       default_email_ubl, payments_without_proof
FROM administrations WHERE id = $1;

-- name: UpdateInvoiceSettings :exec
UPDATE administrations
SET invoice_id_template = $2, invoice_sequence_id = $3, estimate_id_template = $4, estimate_sequence_id = $5,
    invoice_bcc_email_address = $6, invoice_reminders_bcc_enabled = $7, estimate_bcc_email_address = $8,
    order_bcc_email_address = $9, default_email_ubl = $10, payments_without_proof = $11
WHERE id = $1;

-- name: ListWorkflows :many
SELECT * FROM workflows WHERE administration_id = $1 ORDER BY id;

-- name: WorkflowByID :one
SELECT * FROM workflows WHERE administration_id = $1 AND id = $2;

-- name: ListWorkflowSteps :many
SELECT s.* FROM workflow_steps s JOIN workflows w ON w.id = s.workflow_id
WHERE w.administration_id = $1
ORDER BY s.workflow_id, s.step_order;

-- name: InsertWorkflow :one
INSERT INTO workflows (id, administration_id, type, name, is_default, currency, language, active, prices_are_incl_tax,
                       show_qr_code, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
ON CONFLICT (administration_id, type) WHERE is_default DO NOTHING
RETURNING id;

-- name: ClearDefaultWorkflow :exec
UPDATE workflows SET is_default = false WHERE administration_id = $1 AND type = $2 AND id <> $3 AND is_default;

-- name: UpdateWorkflow :exec
UPDATE workflows
SET name = $3, is_default = $4, currency = $5, language = $6, active = $7, prices_are_incl_tax = $8,
    show_qr_code = $9, updated_at = $10
WHERE administration_id = $1 AND id = $2;

-- name: DeleteWorkflow :execrows
DELETE FROM workflows WHERE administration_id = $1 AND id = $2;

-- name: DeleteWorkflowSteps :exec
DELETE FROM workflow_steps WHERE workflow_id = $1;

-- name: InsertWorkflowStep :exec
INSERT INTO workflow_steps (id, workflow_id, step_order, due_interval, reminder_delay, reminder_text, reminder_auto_send,
                            created_at, updated_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9);

-- name: ListDocumentStyles :many
SELECT * FROM document_styles WHERE administration_id = $1 ORDER BY id;

-- name: DocumentStyleByID :one
SELECT * FROM document_styles WHERE administration_id = $1 AND id = $2;

-- name: InsertDocumentStyle :one
INSERT INTO document_styles (id, administration_id, name, identity_id, is_default, logo_hash, logo_content_type,
                             logo_container_full_width, logo_display_width, logo_position, background_hash,
                             background_content_type, print_on_stationery, paper_size, address_position, font_size,
                             font_family, custom_css, layout, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21)
ON CONFLICT (administration_id) WHERE is_default DO NOTHING
RETURNING id;

-- name: ClearDefaultDocumentStyle :exec
UPDATE document_styles SET is_default = false WHERE administration_id = $1 AND id <> $2 AND is_default;

-- name: UpdateDocumentStyle :exec
UPDATE document_styles
SET name = $3, identity_id = $4, is_default = $5, logo_hash = $6, logo_content_type = $7,
    logo_container_full_width = $8, logo_display_width = $9, logo_position = $10, background_hash = $11,
    background_content_type = $12, print_on_stationery = $13, paper_size = $14, address_position = $15,
    font_size = $16, font_family = $17, custom_css = $18, layout = $19, updated_at = $20
WHERE administration_id = $1 AND id = $2;

-- name: DeleteDocumentStyle :execrows
DELETE FROM document_styles WHERE administration_id = $1 AND id = $2;

-- name: DocumentFileInUse :one
SELECT EXISTS (SELECT 1 FROM document_styles WHERE administration_id = $1 AND (logo_hash = $2 OR background_hash = $2));

-- name: MoveDocumentStylesToDefaultIdentity :exec
UPDATE document_styles d SET identity_id = i.id
FROM identities i
WHERE d.identity_id = $1 AND i.administration_id = d.administration_id AND i.is_default;
