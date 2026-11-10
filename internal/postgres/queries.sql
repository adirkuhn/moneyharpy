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
       a.require_two_factor, a.support_access_until,
       m.access, m.favorite, m.user_type, m.permissions
FROM administrations a JOIN administration_members m ON m.administration_id = a.id
WHERE m.user_id = $1 AND m.active;

-- name: AdministrationForUser :one
SELECT a.id, a.name, a.language, a.currency, a.country, a.time_zone, a.suspended,
       a.period_locked_until, a.period_start_date,
       a.small_business_scheme, a.company_activities_description, a.company_website_url, a.company_website_absent,
       a.require_two_factor, a.support_access_until,
       m.access, m.favorite, m.user_type, m.permissions
FROM administrations a JOIN administration_members m ON m.administration_id = a.id
WHERE a.id = $1 AND m.user_id = $2 AND m.active;

-- name: AdministrationForSupport :one
-- An instance admin the administration granted support access to, while the
-- grant lasts. They work as a member with every permission but ownership.
SELECT a.id, a.name, a.language, a.currency, a.country, a.time_zone, a.suspended,
       a.period_locked_until, a.period_start_date,
       a.small_business_scheme, a.company_activities_description, a.company_website_url, a.company_website_absent,
       a.require_two_factor, a.support_access_until
FROM administrations a JOIN users u ON u.id = $2 AND u.is_instance_admin
WHERE a.id = $1 AND a.support_access_until > now();

-- name: SetFavorite :execrows
UPDATE administration_members SET favorite = $3 WHERE administration_id = $1 AND user_id = $2;

-- name: LocateResource :one
SELECT r.id, r.type, r.administration_id
FROM resources r JOIN administration_members m ON m.administration_id = r.administration_id
WHERE r.id = $1 AND m.user_id = $2 AND m.active;

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
    company_activities_description = $7, company_website_url = $8, company_website_absent = $9, country = $10
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

-- name: BookkeepingSettings :one
SELECT vat_documents_period, icp_declaration_period, chart_scheme, bookkeeping_seeded, period_locked_until
FROM administrations WHERE id = $1;

-- name: UpdateBookkeepingSettings :exec
UPDATE administrations
SET vat_documents_period = $2, icp_declaration_period = $3, chart_scheme = $4, period_locked_until = $5
WHERE id = $1;

-- name: MarkBookkeepingSeeded :execrows
UPDATE administrations SET bookkeeping_seeded = true, chart_scheme = $2 WHERE id = $1 AND NOT bookkeeping_seeded;

-- name: ClearLedgerAccountTaxonomy :exec
UPDATE ledger_accounts SET taxonomy_code = '' WHERE administration_id = $1;

-- name: InsertTaxRate :exec
INSERT INTO tax_rates (id, administration_id, name, percentage, tax_rate_type, show_tax, active, country, report_boxes, position, created_at, updated_at)
VALUES (@id, @administration_id, @name, (@percentage::text)::numeric, @tax_rate_type, @show_tax, @active, @country, @report_boxes::text[], @position, @created_at, @updated_at);

-- name: ListTaxRates :many
SELECT id, administration_id, name, percentage::text AS percentage, tax_rate_type, show_tax, active, country, report_boxes, position, created_at, updated_at
FROM tax_rates WHERE administration_id = $1 ORDER BY tax_rate_type, position, id;

-- name: TaxRateByID :one
SELECT id, administration_id, name, percentage::text AS percentage, tax_rate_type, show_tax, active, country, report_boxes, position, created_at, updated_at
FROM tax_rates WHERE administration_id = $1 AND id = $2;

-- name: UpdateTaxRate :exec
UPDATE tax_rates
SET name = @name, percentage = (@percentage::text)::numeric, tax_rate_type = @tax_rate_type, show_tax = @show_tax, active = @active,
    country = @country, report_boxes = @report_boxes::text[], position = @position, updated_at = @updated_at
WHERE administration_id = @administration_id AND id = @id;

-- name: InsertLedgerAccount :exec
INSERT INTO ledger_accounts (id, administration_id, name, account_type, account_id, parent_id, active, allowed_document_types,
                             taxonomy_code, description, system_role, seed_key, template_key, created_at, updated_at)
VALUES (@id, @administration_id, @name, @account_type, @account_id, @parent_id, @active, @allowed_document_types::text[],
        @taxonomy_code, @description, @system_role, @seed_key, @template_key, @created_at, @updated_at);

-- name: ListLedgerAccounts :many
SELECT * FROM ledger_accounts WHERE administration_id = $1 ORDER BY account_type, account_id, id;

-- name: LedgerAccountByID :one
SELECT * FROM ledger_accounts WHERE administration_id = $1 AND id = $2;

-- name: UpdateLedgerAccount :exec
UPDATE ledger_accounts
SET name = @name, account_type = @account_type, account_id = @account_id, parent_id = @parent_id, active = @active,
    allowed_document_types = @allowed_document_types::text[], taxonomy_code = @taxonomy_code, description = @description,
    updated_at = @updated_at
WHERE administration_id = @administration_id AND id = @id;

-- name: DeleteLedgerAccount :exec
DELETE FROM ledger_accounts WHERE administration_id = $1 AND id = $2;

-- name: BankSettings :one
SELECT bank_auto_link, bank_seeded FROM administrations WHERE id = $1;

-- name: SetBankAutoLink :exec
UPDATE administrations SET bank_auto_link = $2 WHERE id = $1;

-- name: MarkBankSeeded :execrows
UPDATE administrations SET bank_seeded = true WHERE id = $1 AND NOT bank_seeded;

-- name: DeleteResources :exec
DELETE FROM resources WHERE id = ANY(@ids::bigint[]);

-- name: InsertFinancialAccount :exec
INSERT INTO financial_accounts (id, administration_id, type, name, identifier, currency, provider, active, position, ledger_account_id, last_import_at, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13);

-- name: ListFinancialAccounts :many
SELECT * FROM financial_accounts WHERE administration_id = $1 ORDER BY position, id;

-- name: FinancialAccountByID :one
SELECT * FROM financial_accounts WHERE administration_id = $1 AND id = $2;

-- name: UpdateFinancialAccount :exec
UPDATE financial_accounts
SET type = $3, name = $4, identifier = $5, currency = $6, provider = $7, active = $8, position = $9, last_import_at = $10, updated_at = $11
WHERE administration_id = $1 AND id = $2;

-- name: InsertFinancialStatement :exec
INSERT INTO financial_statements (id, administration_id, financial_account_id, reference, official_date, official_balance, importer, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9);

-- name: FinancialStatementByID :one
SELECT * FROM financial_statements WHERE administration_id = $1 AND id = $2;

-- name: UpdateFinancialStatement :exec
UPDATE financial_statements SET reference = $3, official_date = $4, official_balance = $5, updated_at = $6
WHERE administration_id = $1 AND id = $2;

-- name: DeleteFinancialStatement :exec
DELETE FROM financial_statements WHERE administration_id = $1 AND id = $2;

-- name: InsertFinancialMutation :execrows
INSERT INTO financial_mutations (id, administration_id, financial_account_id, financial_statement_id, date, valutation_date, message, amount, currency,
                                 code, contra_account_name, contra_account_number, batch_reference, transaction_id, fingerprint, position, state,
                                 processed_at, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20)
ON CONFLICT DO NOTHING;

-- name: UpdateFinancialMutation :exec
UPDATE financial_mutations
SET date = $3, valutation_date = $4, message = $5, amount = $6, code = $7, contra_account_name = $8, contra_account_number = $9,
    batch_reference = $10, transaction_id = $11, position = $12, state = $13, processed_at = $14, updated_at = $15
WHERE administration_id = $1 AND id = $2;

-- name: DeleteFinancialMutations :exec
DELETE FROM financial_mutations WHERE administration_id = @administration_id AND id = ANY(@ids::bigint[]);

-- name: ListFinancialMutations :many
SELECT * FROM financial_mutations
WHERE administration_id = @administration_id
  AND (sqlc.narg(financial_account_id)::bigint IS NULL OR financial_account_id = sqlc.narg(financial_account_id))
  AND (sqlc.narg(date_from)::date IS NULL OR date >= sqlc.narg(date_from))
  AND (sqlc.narg(date_to)::date IS NULL OR date <= sqlc.narg(date_to))
ORDER BY date DESC, financial_statement_id DESC, position, id;

-- name: FinancialMutationsByID :many
SELECT * FROM financial_mutations WHERE administration_id = @administration_id AND id = ANY(@ids::bigint[]) ORDER BY id;

-- name: FinancialMutationsOfStatement :many
SELECT * FROM financial_mutations WHERE administration_id = $1 AND financial_statement_id = $2 ORDER BY position, id;

-- name: FinancialMutationVersions :many
SELECT id, updated_at FROM financial_mutations WHERE administration_id = $1 ORDER BY id;

-- name: BookingsOfMutations :many
SELECT * FROM ledger_account_bookings WHERE administration_id = @administration_id AND financial_mutation_id = ANY(@ids::bigint[]) ORDER BY created_at, id;

-- name: InsertLedgerAccountBooking :exec
INSERT INTO ledger_account_bookings (id, administration_id, financial_mutation_id, ledger_account_id, description, price, booking_rule_id, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9);

-- name: DeleteLedgerAccountBooking :exec
DELETE FROM ledger_account_bookings WHERE administration_id = $1 AND financial_mutation_id = $2 AND id = $3;

-- name: FinancialBalances :many
SELECT financial_account_id, sum(amount)::bigint AS balance FROM financial_mutations WHERE administration_id = $1 GROUP BY financial_account_id;

-- name: InsertBookingRule :exec
INSERT INTO booking_rules (id, administration_id, name, active, position, match_all, direction, financial_account_id, conditions, ledger_account_id, description, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13);

-- name: ListBookingRules :many
SELECT * FROM booking_rules WHERE administration_id = $1 ORDER BY position, id;

-- name: BookingRuleByID :one
SELECT * FROM booking_rules WHERE administration_id = $1 AND id = $2;

-- name: UpdateBookingRule :exec
UPDATE booking_rules
SET name = $3, active = $4, position = $5, match_all = $6, direction = $7, financial_account_id = $8, conditions = $9, ledger_account_id = $10,
    description = $11, updated_at = $12
WHERE administration_id = $1 AND id = $2;

-- name: DeleteBookingRule :exec
DELETE FROM booking_rules WHERE administration_id = $1 AND id = $2;

-- name: UserTwoFactor :one
SELECT u.totp_secret, u.totp_enabled_at,
       (SELECT count(*) FROM recovery_codes c WHERE c.user_id = u.id AND c.used_at IS NULL)::int AS codes_left
FROM users u WHERE u.id = $1;

-- name: SetTOTPSecret :exec
UPDATE users SET totp_secret = $2 WHERE id = $1 AND totp_enabled_at IS NULL;

-- name: EnableTOTP :exec
UPDATE users SET totp_enabled_at = $2, totp_last_step = $3 WHERE id = $1;

-- name: DisableTOTP :exec
UPDATE users SET totp_secret = '', totp_enabled_at = NULL, totp_last_step = 0 WHERE id = $1;

-- name: UseTOTPStep :execrows
UPDATE users SET totp_last_step = $2 WHERE id = $1 AND totp_last_step < $2;

-- name: DeleteRecoveryCodes :exec
DELETE FROM recovery_codes WHERE user_id = $1;

-- name: InsertRecoveryCode :exec
INSERT INTO recovery_codes (user_id, code_hash) VALUES ($1, $2);

-- name: UseRecoveryCode :execrows
UPDATE recovery_codes SET used_at = $3 WHERE user_id = $1 AND code_hash = $2 AND used_at IS NULL;

-- name: RequiresTwoFactor :one
SELECT EXISTS (
    SELECT 1 FROM administration_members m JOIN administrations a ON a.id = m.administration_id
    WHERE m.user_id = $1 AND m.active AND a.require_two_factor
);

-- name: InsertLoginChallenge :exec
INSERT INTO login_challenges (token_hash, user_id, expires_at) VALUES ($1, $2, $3);

-- name: SpendLoginChallengeAttempt :one
UPDATE login_challenges c SET attempts = c.attempts + 1
FROM users u
WHERE c.token_hash = $1 AND c.expires_at > $2 AND u.id = c.user_id
RETURNING u.id, u.email, u.name, u.is_instance_admin, c.attempts;

-- name: DeleteLoginChallenge :exec
DELETE FROM login_challenges WHERE token_hash = $1 OR expires_at < now();

-- name: ListMembers :many
SELECT u.id, u.name, u.email, m.user_type, m.permissions, m.active, m.created_at, m.updated_at,
       EXISTS (SELECT 1 FROM user_identities i WHERE i.user_id = u.id) AS email_validated
FROM administration_members m JOIN users u ON u.id = m.user_id
WHERE m.administration_id = $1;

-- name: MemberByUser :one
SELECT u.id, u.name, u.email, m.user_type, m.permissions, m.active, m.created_at, m.updated_at,
       EXISTS (SELECT 1 FROM user_identities i WHERE i.user_id = u.id) AS email_validated
FROM administration_members m JOIN users u ON u.id = m.user_id
WHERE m.administration_id = $1 AND m.user_id = $2;

-- name: UpdateMember :exec
UPDATE administration_members SET user_type = $3, permissions = $4, active = $5, updated_at = $6
WHERE administration_id = $1 AND user_id = $2;

-- name: DeleteMember :exec
DELETE FROM administration_members WHERE administration_id = $1 AND user_id = $2;

-- name: UpsertInvitedMember :execrows
-- Adds the user, or brings back one who was deactivated; an active user
-- stays as they are.
INSERT INTO administration_members (administration_id, user_id, access, user_type, permissions, active, created_at, updated_at)
VALUES ($1, $2, 'user', $3, $4, true, $5, $5)
ON CONFLICT (administration_id, user_id) DO UPDATE
SET user_type = EXCLUDED.user_type, permissions = EXCLUDED.permissions, active = true, updated_at = EXCLUDED.updated_at
WHERE NOT administration_members.active;

-- name: ListOpenInvitations :many
SELECT * FROM user_invitations WHERE administration_id = $1 AND accepted_at IS NULL ORDER BY created_at, id;

-- name: InsertInvitation :exec
INSERT INTO user_invitations (id, administration_id, email, user_type, permissions, token_hash, invited_by, created_at, expires_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9);

-- name: DeleteOpenInvitationsFor :exec
DELETE FROM user_invitations WHERE administration_id = $1 AND lower(email) = lower($2) AND accepted_at IS NULL;

-- name: InvitationByToken :one
SELECT i.*, a.name AS administration_name, COALESCE(u.name, '') AS inviter_name
FROM user_invitations i JOIN administrations a ON a.id = i.administration_id LEFT JOIN users u ON u.id = i.invited_by
WHERE i.token_hash = $1;

-- name: DeleteInvitation :execrows
DELETE FROM user_invitations WHERE administration_id = $1 AND id = $2 AND accepted_at IS NULL;

-- name: MarkInvitationAccepted :execrows
UPDATE user_invitations SET accepted_at = $2, accepted_by = $3 WHERE id = $1 AND accepted_at IS NULL;

-- name: SetRequireTwoFactor :exec
UPDATE administrations SET require_two_factor = true WHERE id = $1;

-- name: SetSupportAccess :exec
UPDATE administrations SET support_access_until = $2 WHERE id = $1;

-- name: SupportGrants :many
SELECT id, name, support_access_until FROM administrations WHERE support_access_until > $1 ORDER BY support_access_until, id;

-- name: DeleteAdministration :exec
DELETE FROM administrations WHERE id = $1;
