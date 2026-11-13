-- Scan and Recognise: what was read of incoming documents, and what is
-- remembered of suppliers.

-- name: UpsertDocumentRecognition :exec
INSERT INTO document_recognitions (document_id, administration_id, user_id, state, source, fields, error, attempts, next_attempt_at, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
ON CONFLICT (document_id) DO UPDATE SET
    user_id = excluded.user_id, state = excluded.state, source = excluded.source, fields = excluded.fields, error = excluded.error,
    attempts = excluded.attempts, next_attempt_at = excluded.next_attempt_at, updated_at = excluded.updated_at;

-- name: DocumentRecognition :one
SELECT * FROM document_recognitions WHERE administration_id = $1 AND document_id = $2;

-- name: DocumentRecognitionsOf :many
SELECT * FROM document_recognitions WHERE administration_id = @administration_id AND document_id = ANY(@ids::bigint[]);

-- name: DeleteDocumentRecognition :exec
DELETE FROM document_recognitions WHERE administration_id = $1 AND document_id = $2;

-- name: LeaseDueRecognitions :many
-- Takes pending documents that are due, moving them to @lease so no other
-- instance reads them meanwhile.
UPDATE document_recognitions r SET next_attempt_at = @lease
WHERE r.document_id IN (
    SELECT x.document_id FROM document_recognitions x
    WHERE x.state = 'pending' AND x.next_attempt_at <= @now
    ORDER BY x.next_attempt_at LIMIT @max_count FOR UPDATE SKIP LOCKED
)
RETURNING r.*;

-- name: SupplierMemory :one
SELECT * FROM supplier_memories WHERE administration_id = $1 AND contact_id = $2;

-- name: SupplierMemoryLines :many
SELECT * FROM supplier_memory_lines WHERE administration_id = $1 AND contact_id = $2 ORDER BY description;

-- name: UpsertSupplierMemory :exec
INSERT INTO supplier_memories (administration_id, contact_id, document_kind, document_kind_uses, currency, currency_uses,
                               prices_are_incl_tax, prices_are_incl_tax_uses, updated_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
ON CONFLICT (administration_id, contact_id) DO UPDATE SET
    document_kind = excluded.document_kind, document_kind_uses = excluded.document_kind_uses, currency = excluded.currency,
    currency_uses = excluded.currency_uses, prices_are_incl_tax = excluded.prices_are_incl_tax,
    prices_are_incl_tax_uses = excluded.prices_are_incl_tax_uses, updated_at = excluded.updated_at;

-- name: DeleteSupplierMemoryLines :exec
DELETE FROM supplier_memory_lines WHERE administration_id = $1 AND contact_id = $2;

-- name: InsertSupplierMemoryLine :exec
INSERT INTO supplier_memory_lines (administration_id, contact_id, description, ledger_account_id, tax_rate_id, uses, last_used_at)
VALUES ($1, $2, $3, $4, $5, $6, $7);
