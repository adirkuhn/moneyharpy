-- Incoming documents: purchase invoices and receipts, general and typeless
-- documents, and the purchase transactions that pay them.

-- name: ListPurchaseDocuments :many
-- An open document a batched purchase transaction pays is pending payment;
-- one past its due date is late.
SELECT sqlc.embed(d), count(*) OVER () AS total
FROM purchase_documents d
WHERE d.administration_id = @administration_id
  AND (@kind::text = '' OR d.kind = @kind)
  AND NOT @nothing::boolean
  AND (cardinality(@states::text[]) = 0 OR
       (CASE WHEN d.state = 'open' AND EXISTS (SELECT 1 FROM purchase_transactions t
                                               WHERE t.payable_id = d.id AND t.purchase_transaction_batch_id IS NOT NULL AND NOT t.cancelled)
                  THEN 'pending_payment'
             WHEN d.state = 'open' AND d.due_date < @today::date THEN 'late'
             ELSE d.state END) = ANY(@states::text[]))
  AND (sqlc.narg(from_date)::date IS NULL OR d.date >= sqlc.narg(from_date))
  AND (sqlc.narg(to_date)::date IS NULL OR d.date <= sqlc.narg(to_date))
  AND (sqlc.narg(contact_id)::bigint IS NULL OR d.contact_id = sqlc.narg(contact_id))
  AND (cardinality(@refs::text[]) = 0 OR d.reference = ANY(@refs::text[]))
  AND (sqlc.narg(ledger_account_id)::bigint IS NULL OR EXISTS (SELECT 1 FROM purchase_document_details l
                                                               WHERE l.purchase_document_id = d.id AND l.ledger_account_id = sqlc.narg(ledger_account_id)))
  AND (sqlc.narg(updated_after)::timestamptz IS NULL OR d.updated_at > sqlc.narg(updated_after))
  AND (@attachment::text = '' OR (@attachment::text = 'with') = EXISTS (SELECT 1 FROM attachments a WHERE a.entity_id = d.id))
ORDER BY CASE WHEN @ascending::boolean THEN d.date END ASC,
         CASE WHEN NOT @ascending::boolean THEN d.date END DESC,
         CASE WHEN @ascending::boolean THEN d.id END ASC,
         CASE WHEN NOT @ascending::boolean THEN d.id END DESC
LIMIT sqlc.narg(max_rows) OFFSET @skip;

-- name: PurchaseDocumentByID :one
SELECT * FROM purchase_documents WHERE administration_id = $1 AND id = $2;

-- name: PurchaseDocumentsByIDs :many
SELECT * FROM purchase_documents
WHERE administration_id = @administration_id AND kind = @kind AND id = ANY(@ids::bigint[]) ORDER BY id;

-- name: PendingPurchaseDocuments :many
-- The documents of these a batched purchase transaction pays.
SELECT DISTINCT payable_id::bigint FROM purchase_transactions
WHERE administration_id = @administration_id AND payable_id = ANY(@ids::bigint[])
  AND purchase_transaction_batch_id IS NOT NULL AND NOT cancelled;

-- name: NextDocumentEntryNumber :one
-- Purchase invoices, receipts and general documents share their numbers.
SELECT (coalesce(max(n), 0) + 1)::integer FROM (
    SELECT max(p.entry_number) AS n FROM purchase_documents p WHERE p.administration_id = @admin::bigint
    UNION ALL
    SELECT max(g.entry_number) FROM general_documents g WHERE g.administration_id = @admin::bigint
) numbers;

-- name: UpsertPurchaseDocument :exec
INSERT INTO purchase_documents (
    id, administration_id, kind, contact_id, entry_number, reference, date, due_date, state, origin, currency, prices_are_incl_tax,
    revenue_invoice, paid_at, total_excl_tax, total_tax, total_incl_tax, tax_totals, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20)
ON CONFLICT (id) DO UPDATE SET
    contact_id = EXCLUDED.contact_id, reference = EXCLUDED.reference, date = EXCLUDED.date, due_date = EXCLUDED.due_date,
    state = EXCLUDED.state, currency = EXCLUDED.currency, prices_are_incl_tax = EXCLUDED.prices_are_incl_tax,
    revenue_invoice = EXCLUDED.revenue_invoice, paid_at = EXCLUDED.paid_at, total_excl_tax = EXCLUDED.total_excl_tax,
    total_tax = EXCLUDED.total_tax, total_incl_tax = EXCLUDED.total_incl_tax, tax_totals = EXCLUDED.tax_totals, updated_at = EXCLUDED.updated_at;

-- name: SettlePurchaseDocument :exec
-- What payments change of a document.
UPDATE purchase_documents SET state = $3, paid_at = $4, updated_at = $5 WHERE administration_id = $1 AND id = $2;

-- name: TouchPurchaseDocument :exec
UPDATE purchase_documents SET updated_at = $3 WHERE administration_id = $1 AND id = $2;

-- name: DeletePurchaseDocument :exec
DELETE FROM purchase_documents WHERE administration_id = $1 AND id = $2;

-- name: PurchaseDocumentDetailsOf :many
SELECT * FROM purchase_document_details
WHERE administration_id = @administration_id AND purchase_document_id = ANY(@ids::bigint[]) ORDER BY row_order, id;

-- name: DeletePurchaseDocumentDetails :exec
DELETE FROM purchase_document_details WHERE purchase_document_id = $1;

-- name: InsertPurchaseDocumentDetail :exec
INSERT INTO purchase_document_details (id, administration_id, purchase_document_id, description, period, price, amount, quantity,
    tax_rate_id, ledger_account_id, project_id, product_id, row_order, line_base, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16);

-- name: PurchaseDocumentAllocationsOf :many
SELECT * FROM purchase_document_fiscal_allocations
WHERE administration_id = @administration_id AND purchase_document_id = ANY(@ids::bigint[]) ORDER BY created_at, id;

-- name: DeletePurchaseDocumentAllocations :exec
DELETE FROM purchase_document_fiscal_allocations WHERE purchase_document_id = $1;

-- name: InsertPurchaseDocumentAllocation :exec
INSERT INTO purchase_document_fiscal_allocations (id, administration_id, purchase_document_id, ledger_account_id, private_ledger_account_id,
    percentage, price, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9);

-- name: PurchaseTotals :one
-- What open purchase invoices and receipts have unpaid, and how many
-- uploads wait to be processed.
SELECT (count(*) FILTER (WHERE d.state = 'open'))::int AS pay_count,
       coalesce(sum((CASE WHEN d.revenue_invoice THEN -d.total_incl_tax ELSE d.total_incl_tax END)
                    - coalesce((SELECT sum(p.price) FROM payments p WHERE p.invoice_type = 'Document' AND p.invoice_id = d.id), 0))
                FILTER (WHERE d.state = 'open'), 0)::bigint AS pay_amount,
       ((count(*) FILTER (WHERE d.state = 'new'))
        + (SELECT count(*) FROM typeless_documents t WHERE t.administration_id = @admin::bigint))::int AS process_count
FROM purchase_documents d
WHERE d.administration_id = @admin::bigint;

-- name: PurchaseContactInUse :one
SELECT EXISTS (SELECT 1 FROM purchase_documents p WHERE p.administration_id = @admin::bigint AND p.contact_id = @contact::bigint)
    OR EXISTS (SELECT 1 FROM general_documents g WHERE g.administration_id = @admin::bigint AND g.contact_id = @contact::bigint)
    OR EXISTS (SELECT 1 FROM typeless_documents t WHERE t.administration_id = @admin::bigint AND t.contact_id = @contact::bigint);

-- name: PurchaseProjectInUse :one
SELECT EXISTS (SELECT 1 FROM purchase_document_details WHERE administration_id = @administration_id AND project_id = @project_id);

-- name: PurchaseProductInUse :one
SELECT EXISTS (SELECT 1 FROM purchase_document_details WHERE administration_id = @administration_id AND product_id = @product_id);

-- name: ListGeneralDocuments :many
SELECT sqlc.embed(g), count(*) OVER () AS total
FROM general_documents g
WHERE g.administration_id = @administration_id
  AND NOT @nothing::boolean
  AND (cardinality(@states::text[]) = 0 OR g.state = ANY(@states::text[]))
  AND (sqlc.narg(from_date)::date IS NULL OR g.date >= sqlc.narg(from_date))
  AND (sqlc.narg(to_date)::date IS NULL OR g.date <= sqlc.narg(to_date))
  AND (sqlc.narg(contact_id)::bigint IS NULL OR g.contact_id = sqlc.narg(contact_id))
  AND (cardinality(@refs::text[]) = 0 OR g.reference = ANY(@refs::text[]))
  AND (sqlc.narg(updated_after)::timestamptz IS NULL OR g.updated_at > sqlc.narg(updated_after))
  AND (@attachment::text = '' OR (@attachment::text = 'with') = EXISTS (SELECT 1 FROM attachments a WHERE a.entity_id = g.id))
ORDER BY CASE WHEN @ascending::boolean THEN g.date END ASC,
         CASE WHEN NOT @ascending::boolean THEN g.date END DESC,
         CASE WHEN @ascending::boolean THEN g.id END ASC,
         CASE WHEN NOT @ascending::boolean THEN g.id END DESC
LIMIT sqlc.narg(max_rows) OFFSET @skip;

-- name: GeneralDocumentByID :one
SELECT * FROM general_documents WHERE administration_id = $1 AND id = $2;

-- name: GeneralDocumentsByIDs :many
SELECT * FROM general_documents WHERE administration_id = @administration_id AND id = ANY(@ids::bigint[]) ORDER BY id;

-- name: UpsertGeneralDocument :exec
INSERT INTO general_documents (id, administration_id, contact_id, entry_number, reference, date, due_date, reminder_date, reminder_message,
    state, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
ON CONFLICT (id) DO UPDATE SET
    contact_id = EXCLUDED.contact_id, reference = EXCLUDED.reference, date = EXCLUDED.date, due_date = EXCLUDED.due_date,
    reminder_date = EXCLUDED.reminder_date, reminder_message = EXCLUDED.reminder_message, state = EXCLUDED.state, updated_at = EXCLUDED.updated_at;

-- name: TouchGeneralDocument :exec
UPDATE general_documents SET updated_at = $3 WHERE administration_id = $1 AND id = $2;

-- name: DeleteGeneralDocument :exec
DELETE FROM general_documents WHERE administration_id = $1 AND id = $2;

-- name: ListTypelessDocuments :many
SELECT sqlc.embed(t), count(*) OVER () AS total
FROM typeless_documents t
WHERE t.administration_id = @administration_id
  AND NOT @nothing::boolean
  AND (cardinality(@states::text[]) = 0 OR t.state = ANY(@states::text[]))
  AND (sqlc.narg(from_date)::date IS NULL OR t.date >= sqlc.narg(from_date))
  AND (sqlc.narg(to_date)::date IS NULL OR t.date <= sqlc.narg(to_date))
  AND (sqlc.narg(contact_id)::bigint IS NULL OR t.contact_id = sqlc.narg(contact_id))
  AND (cardinality(@refs::text[]) = 0 OR t.reference = ANY(@refs::text[]))
  AND (sqlc.narg(updated_after)::timestamptz IS NULL OR t.updated_at > sqlc.narg(updated_after))
  AND (@attachment::text = '' OR (@attachment::text = 'with') = EXISTS (SELECT 1 FROM attachments a WHERE a.entity_id = t.id))
ORDER BY CASE WHEN @ascending::boolean THEN t.date END ASC,
         CASE WHEN NOT @ascending::boolean THEN t.date END DESC,
         CASE WHEN @ascending::boolean THEN t.id END ASC,
         CASE WHEN NOT @ascending::boolean THEN t.id END DESC
LIMIT sqlc.narg(max_rows) OFFSET @skip;

-- name: TypelessDocumentByID :one
SELECT * FROM typeless_documents WHERE administration_id = $1 AND id = $2;

-- name: TypelessDocumentsByIDs :many
SELECT * FROM typeless_documents WHERE administration_id = @administration_id AND id = ANY(@ids::bigint[]) ORDER BY id;

-- name: UpsertTypelessDocument :exec
INSERT INTO typeless_documents (id, administration_id, contact_id, reference, date, origin, state, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
ON CONFLICT (id) DO UPDATE SET
    contact_id = EXCLUDED.contact_id, reference = EXCLUDED.reference, date = EXCLUDED.date, origin = EXCLUDED.origin,
    state = EXCLUDED.state, updated_at = EXCLUDED.updated_at;

-- name: TouchTypelessDocument :exec
UPDATE typeless_documents SET updated_at = $3 WHERE administration_id = $1 AND id = $2;

-- name: DeleteTypelessDocument :exec
DELETE FROM typeless_documents WHERE administration_id = $1 AND id = $2;

-- name: ListPurchaseTransactions :many
-- Paid follows the payable; pending payment is being in a batch.
SELECT sqlc.embed(t), coalesce(d.state = 'paid', false)::boolean AS payable_paid, count(*) OVER () AS total
FROM purchase_transactions t
LEFT JOIN purchase_documents d ON d.id = t.payable_id
WHERE t.administration_id = @administration_id
  AND (cardinality(@states::text[]) = 0 OR
       (CASE WHEN t.cancelled THEN 'cancelled'
             WHEN d.state = 'paid' THEN 'paid'
             WHEN t.purchase_transaction_batch_id IS NOT NULL THEN 'pending_payment'
             ELSE 'open' END) = ANY(@states::text[]))
  AND (sqlc.narg(from_date)::date IS NULL OR t.date >= sqlc.narg(from_date))
  AND (sqlc.narg(to_date)::date IS NULL OR t.date <= sqlc.narg(to_date))
  AND (NOT @unbatched::boolean OR t.purchase_transaction_batch_id IS NULL)
  AND (sqlc.narg(batch_id)::bigint IS NULL OR t.purchase_transaction_batch_id = sqlc.narg(batch_id))
  AND (sqlc.narg(payable_id)::bigint IS NULL OR t.payable_id = sqlc.narg(payable_id))
ORDER BY t.created_at, t.id
LIMIT sqlc.narg(max_rows) OFFSET @skip;

-- name: PurchaseTransactionByID :one
SELECT sqlc.embed(t), coalesce(d.state = 'paid', false)::boolean AS payable_paid
FROM purchase_transactions t
LEFT JOIN purchase_documents d ON d.id = t.payable_id
WHERE t.administration_id = $1 AND t.id = $2;

-- name: UpsertPurchaseTransaction :exec
INSERT INTO purchase_transactions (id, administration_id, financial_account_id, purchase_transaction_batch_id, payable_type, payable_id,
    contact_id, amount, date, description, end_to_end_id, sepa_iban, sepa_iban_account_name, sepa_bic, source_sepa_iban,
    source_sepa_iban_account_name, cancelled, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19)
ON CONFLICT (id) DO UPDATE SET
    purchase_transaction_batch_id = EXCLUDED.purchase_transaction_batch_id, date = EXCLUDED.date, cancelled = EXCLUDED.cancelled,
    updated_at = EXCLUDED.updated_at;

-- name: CancelPurchaseTransactionsOf :exec
-- A deleted document's credit transfers are no longer to make.
UPDATE purchase_transactions SET cancelled = true, updated_at = @updated_at
WHERE administration_id = @administration_id AND payable_id = @payable_id::bigint AND NOT cancelled;

-- name: DeletePurchaseTransaction :exec
DELETE FROM purchase_transactions WHERE administration_id = $1 AND id = $2;

-- name: PurchaseTransactionBatchByID :one
SELECT * FROM purchase_transaction_batches WHERE administration_id = $1 AND id = $2;

-- name: UpsertPurchaseTransactionBatch :exec
INSERT INTO purchase_transaction_batches (id, administration_id, financial_account_id, identifier, state, execution_date, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
ON CONFLICT (id) DO UPDATE SET state = EXCLUDED.state, updated_at = EXCLUDED.updated_at;

-- name: ListIncomingDocuments :many
-- Incoming documents of every kind, for the Incoming page: purchase
-- invoices and receipts with the state to show (pending payment when a
-- batched purchase transaction pays an open one, late past its due date),
-- general documents, journal entries, which are saved, and typeless
-- documents, which only an unfiltered type shows. With an id it finds that
-- one document.
WITH docs AS (
    SELECT d.id, d.kind, d.reference, d.contact_id, d.date,
           (CASE WHEN d.state = 'open' AND EXISTS (SELECT 1 FROM purchase_transactions t
                                                   WHERE t.payable_id = d.id AND t.purchase_transaction_batch_id IS NOT NULL AND NOT t.cancelled)
                      THEN 'pending_payment'
                 WHEN d.state = 'open' AND d.due_date < @today::date THEN 'late'
                 ELSE d.state END)::text AS state,
           d.currency, d.total_incl_tax, d.created_at
    FROM purchase_documents d
    WHERE d.administration_id = @administration_id
      AND (cardinality(@kinds::text[]) = 0 OR d.kind = ANY(@kinds::text[]))
      AND (sqlc.narg(id)::bigint IS NULL OR d.id = sqlc.narg(id))
      AND (sqlc.narg(from_date)::date IS NULL OR d.date >= sqlc.narg(from_date))
      AND (sqlc.narg(to_date)::date IS NULL OR d.date <= sqlc.narg(to_date))
      AND (sqlc.narg(contact_id)::bigint IS NULL OR d.contact_id = sqlc.narg(contact_id))
    UNION ALL
    SELECT g.id, 'GeneralDocument', g.reference, g.contact_id, g.date, g.state, ''::text, 0::bigint, g.created_at
    FROM general_documents g
    WHERE g.administration_id = @administration_id
      AND (cardinality(@kinds::text[]) = 0 OR 'GeneralDocument' = ANY(@kinds::text[]))
      AND (sqlc.narg(id)::bigint IS NULL OR g.id = sqlc.narg(id))
      AND (sqlc.narg(from_date)::date IS NULL OR g.date >= sqlc.narg(from_date))
      AND (sqlc.narg(to_date)::date IS NULL OR g.date <= sqlc.narg(to_date))
      AND (sqlc.narg(contact_id)::bigint IS NULL OR g.contact_id = sqlc.narg(contact_id))
    UNION ALL
    SELECT j.id, 'GeneralJournalDocument', j.reference, NULL::bigint, j.date, 'saved', ''::text, 0::bigint, j.created_at
    FROM general_journal_documents j
    WHERE j.administration_id = @administration_id
      AND (cardinality(@kinds::text[]) = 0 OR 'GeneralJournalDocument' = ANY(@kinds::text[]))
      AND (sqlc.narg(id)::bigint IS NULL OR j.id = sqlc.narg(id))
      AND (sqlc.narg(from_date)::date IS NULL OR j.date >= sqlc.narg(from_date))
      AND (sqlc.narg(to_date)::date IS NULL OR j.date <= sqlc.narg(to_date))
      AND (sqlc.narg(contact_id)::bigint IS NULL OR EXISTS (
          SELECT 1 FROM journal_entries e WHERE e.document_id = j.id AND e.contact_id = sqlc.narg(contact_id)))
    UNION ALL
    SELECT t.id, 'TypelessDocument', t.reference, t.contact_id, t.date, t.state, ''::text, 0::bigint, t.created_at
    FROM typeless_documents t
    WHERE t.administration_id = @administration_id
      AND cardinality(@kinds::text[]) = 0
      AND (sqlc.narg(id)::bigint IS NULL OR t.id = sqlc.narg(id))
      AND (sqlc.narg(from_date)::date IS NULL OR t.date >= sqlc.narg(from_date))
      AND (sqlc.narg(to_date)::date IS NULL OR t.date <= sqlc.narg(to_date))
      AND (sqlc.narg(contact_id)::bigint IS NULL OR t.contact_id = sqlc.narg(contact_id))
)
SELECT x.id::bigint AS id, x.kind::text AS kind, x.reference::text AS reference, coalesce(x.contact_id, 0)::bigint AS contact_id,
       coalesce(CASE WHEN c.company_name <> '' THEN c.company_name ELSE trim(c.firstname || ' ' || c.lastname) END, '')::text AS contact_name,
       x.date::date AS date, x.state::text AS state, x.currency::text AS currency, x.total_incl_tax::bigint AS total_incl_tax,
       (SELECT count(*) FROM attachments a WHERE a.entity_id = x.id)::int AS attachments_count,
       x.created_at::timestamptz AS created_at, count(*) OVER () AS total
FROM docs x
LEFT JOIN contacts c ON c.id = x.contact_id
WHERE NOT @nothing::boolean
  AND (cardinality(@states::text[]) = 0 OR x.state = ANY(@states::text[]))
  AND (@attachment::text = '' OR (@attachment::text = 'with') = EXISTS (SELECT 1 FROM attachments a WHERE a.entity_id = x.id))
ORDER BY CASE WHEN @ascending::boolean THEN x.date END ASC,
         CASE WHEN NOT @ascending::boolean THEN x.date END DESC,
         CASE WHEN @ascending::boolean THEN x.id END ASC,
         CASE WHEN NOT @ascending::boolean THEN x.id END DESC
LIMIT sqlc.narg(max_rows) OFFSET @skip;

-- name: IncomingDocumentStatistics :one
-- What the purchase invoices and receipts the Incoming page selects book as
-- costs excluding VAT (new documents book nothing), what is unpaid on the
-- open, late and pending ones, and the costs per category as JSON, largest
-- first, with the lines without a category under a null ledger account.
WITH selected AS (
    SELECT d.id, d.revenue_invoice, d.total_excl_tax, d.total_incl_tax,
           (CASE WHEN d.state = 'open' AND EXISTS (SELECT 1 FROM purchase_transactions t
                                                   WHERE t.payable_id = d.id AND t.purchase_transaction_batch_id IS NOT NULL AND NOT t.cancelled)
                      THEN 'pending_payment'
                 WHEN d.state = 'open' AND d.due_date < @today::date THEN 'late'
                 ELSE d.state END)::text AS state
    FROM purchase_documents d
    WHERE d.administration_id = @administration_id
      AND NOT @nothing::boolean
      AND (cardinality(@kinds::text[]) = 0 OR d.kind = ANY(@kinds::text[]))
      AND (sqlc.narg(from_date)::date IS NULL OR d.date >= sqlc.narg(from_date))
      AND (sqlc.narg(to_date)::date IS NULL OR d.date <= sqlc.narg(to_date))
      AND (sqlc.narg(contact_id)::bigint IS NULL OR d.contact_id = sqlc.narg(contact_id))
      AND (@attachment::text = '' OR (@attachment::text = 'with') = EXISTS (SELECT 1 FROM attachments a WHERE a.entity_id = d.id))
), kept AS (
    SELECT s.*, CASE WHEN s.revenue_invoice THEN -1 ELSE 1 END AS sign
    FROM selected s WHERE cardinality(@states::text[]) = 0 OR s.state = ANY(@states::text[])
), categories AS (
    SELECT l.ledger_account_id, coalesce(la.name, '') AS name, sum(k.sign * l.line_base)::bigint AS amount
    FROM kept k
    JOIN purchase_document_details l ON l.purchase_document_id = k.id
    LEFT JOIN ledger_accounts la ON la.id = l.ledger_account_id
    WHERE k.state <> 'new'
    GROUP BY l.ledger_account_id, la.name
)
SELECT (SELECT coalesce(sum(k.sign * k.total_excl_tax), 0) FROM kept k WHERE k.state <> 'new')::bigint AS costs,
       (SELECT coalesce(sum(k.sign * k.total_incl_tax
                            - coalesce((SELECT sum(p.price) FROM payments p WHERE p.invoice_type = 'Document' AND p.invoice_id = k.id), 0)), 0)
        FROM kept k WHERE k.state IN ('open', 'late', 'pending_payment'))::bigint AS payable,
       (SELECT coalesce(jsonb_agg(c ORDER BY c.amount DESC, c.name), '[]') FROM categories c)::jsonb AS categories;

-- name: SetPurchaseDocumentKind :exec
UPDATE purchase_documents SET kind = @kind, revenue_invoice = @revenue_invoice, updated_at = @updated_at
WHERE administration_id = @administration_id AND id = @id;

-- name: RetypeResource :exec
UPDATE resources SET type = @type WHERE administration_id = @administration_id AND id = @id;

-- name: RetypeNotes :exec
UPDATE notes SET entity_type = @new_type
WHERE administration_id = @administration_id AND entity_id = @entity_id AND entity_type = @old_type;

-- name: RetypeAttachments :exec
UPDATE attachments SET entity_type = @new_type
WHERE administration_id = @administration_id AND entity_id = @entity_id AND entity_type = @old_type;

-- name: RetypeEvents :exec
UPDATE events SET entity_type = @new_type
WHERE administration_id = @administration_id AND entity_id = @entity_id AND entity_type = @old_type;
