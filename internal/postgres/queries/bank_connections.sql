-- name: ListBankConnections :many
SELECT * FROM bank_connections WHERE administration_id = $1 ORDER BY created_at, id;

-- name: GetBankConnection :one
SELECT * FROM bank_connections WHERE administration_id = $1 AND id = $2;

-- name: GetBankConnectionByAuthState :one
SELECT * FROM bank_connections WHERE auth_state = $1;

-- name: DueBankConnections :many
SELECT * FROM bank_connections
WHERE state = 'active' AND (last_sync_at IS NULL OR last_sync_at < $1)
ORDER BY last_sync_at NULLS FIRST, id
LIMIT 200;

-- name: UpsertBankConnection :exec
INSERT INTO bank_connections (id, administration_id, provider, bank, country, state, session_id, valid_until, auth_state, created_by, last_sync_at, last_error, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
ON CONFLICT (id) DO UPDATE SET
    state = EXCLUDED.state, session_id = EXCLUDED.session_id, valid_until = EXCLUDED.valid_until, auth_state = EXCLUDED.auth_state,
    created_by = EXCLUDED.created_by, last_sync_at = EXCLUDED.last_sync_at, last_error = EXCLUDED.last_error, updated_at = EXCLUDED.updated_at;

-- name: DeleteBankConnection :exec
DELETE FROM bank_connections WHERE administration_id = $1 AND id = $2;

-- name: BankConnectionAccounts :many
SELECT * FROM bank_connection_accounts WHERE connection_id = ANY(@connection_ids::bigint[]) ORDER BY connection_id, position;

-- name: DeleteBankConnectionAccounts :exec
DELETE FROM bank_connection_accounts WHERE connection_id = $1;

-- name: InsertBankConnectionAccount :exec
INSERT INTO bank_connection_accounts (financial_account_id, connection_id, account_uid, position)
VALUES ($1, $2, $3, $4)
ON CONFLICT (financial_account_id) DO UPDATE SET connection_id = EXCLUDED.connection_id, account_uid = EXCLUDED.account_uid, position = EXCLUDED.position;
