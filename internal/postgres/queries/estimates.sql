-- name: ListEstimates :many
-- Late is worked out from the due date: open estimates past it count as
-- late, not open. Drafts without a date match on their creation.
SELECT sqlc.embed(e), count(*) OVER () AS total
FROM estimates e
WHERE e.administration_id = @administration_id
  AND (cardinality(@states::text[]) = 0 OR
       (CASE WHEN e.state = 'open' AND e.due_date < @today::date THEN 'late' ELSE e.state END) = ANY(@states::text[]))
  AND (sqlc.narg(from_date)::date IS NULL OR coalesce(e.estimate_date, e.created_at::date) >= sqlc.narg(from_date))
  AND (sqlc.narg(to_date)::date IS NULL OR coalesce(e.estimate_date, e.created_at::date) <= sqlc.narg(to_date))
  AND (cardinality(@refs::text[]) = 0 OR e.reference = ANY(@refs::text[]))
  AND (sqlc.narg(contact_id)::bigint IS NULL OR e.contact_id = sqlc.narg(contact_id))
  AND (sqlc.narg(workflow_id)::bigint IS NULL OR e.workflow_id = sqlc.narg(workflow_id))
  AND (sqlc.narg(created_after)::timestamptz IS NULL OR e.created_at > sqlc.narg(created_after))
  AND (sqlc.narg(updated_after)::timestamptz IS NULL OR e.updated_at > sqlc.narg(updated_after))
ORDER BY coalesce(e.estimate_date, e.created_at::date) DESC, e.id DESC
LIMIT sqlc.narg(max_rows) OFFSET @skip;

-- name: EstimateByID :one
SELECT * FROM estimates WHERE administration_id = $1 AND id = $2;

-- name: EstimatesByIDs :many
SELECT * FROM estimates WHERE administration_id = @administration_id AND id = ANY(@ids::bigint[]) ORDER BY id;

-- name: EstimateByNumber :one
SELECT * FROM estimates WHERE administration_id = $1 AND estimate_id = $2 AND estimate_id <> '';

-- name: NextEstimateDraftID :one
SELECT coalesce(max(draft_id), 0)::integer + 1 FROM estimates WHERE administration_id = $1;

-- name: EstimateNumberTaken :one
SELECT EXISTS (SELECT 1 FROM estimates WHERE administration_id = $1 AND estimate_id = $2 AND id <> $3);

-- name: LockEstimateSequence :one
SELECT estimate_sequence_id FROM administrations WHERE id = $1 FOR UPDATE;

-- name: SetEstimateSequence :exec
UPDATE administrations SET estimate_sequence_id = $2 WHERE id = $1;

-- name: UpsertEstimate :exec
INSERT INTO estimates (
    id, administration_id, contact_id, contact_person_id, identity_id, document_style_id, workflow_id, draft_id, estimate_id, sequence_id,
    reference, language, currency, prices_are_incl_tax, show_tax, sign_online, discount, estimate_date, due_date, first_due_interval,
    pre_text, post_text, state, sent_at, accepted_at, rejected_at, archived_at, original_estimate_id, original_sales_invoice_id, delivery,
    total_excl_tax, total_tax, total_incl_tax, total_discount, tax_totals, created_at, updated_at)
VALUES (@id, @administration_id, @contact_id, @contact_person_id, @identity_id, @document_style_id, @workflow_id, @draft_id, @estimate_id,
    @sequence_id, @reference, @language, @currency, @prices_are_incl_tax, @show_tax, @sign_online, @discount, @estimate_date, @due_date,
    @first_due_interval, @pre_text, @post_text, @state, @sent_at, @accepted_at, @rejected_at, @archived_at, @original_estimate_id,
    @original_sales_invoice_id, @delivery, @total_excl_tax, @total_tax, @total_incl_tax, @total_discount, @tax_totals, @created_at, @updated_at)
ON CONFLICT (id) DO UPDATE SET
    contact_id = EXCLUDED.contact_id, contact_person_id = EXCLUDED.contact_person_id, identity_id = EXCLUDED.identity_id,
    document_style_id = EXCLUDED.document_style_id, workflow_id = EXCLUDED.workflow_id, estimate_id = EXCLUDED.estimate_id,
    sequence_id = EXCLUDED.sequence_id, reference = EXCLUDED.reference, language = EXCLUDED.language, currency = EXCLUDED.currency,
    prices_are_incl_tax = EXCLUDED.prices_are_incl_tax, show_tax = EXCLUDED.show_tax, sign_online = EXCLUDED.sign_online,
    discount = EXCLUDED.discount, estimate_date = EXCLUDED.estimate_date, due_date = EXCLUDED.due_date,
    first_due_interval = EXCLUDED.first_due_interval, pre_text = EXCLUDED.pre_text, post_text = EXCLUDED.post_text, state = EXCLUDED.state,
    sent_at = EXCLUDED.sent_at, accepted_at = EXCLUDED.accepted_at, rejected_at = EXCLUDED.rejected_at, archived_at = EXCLUDED.archived_at,
    original_estimate_id = EXCLUDED.original_estimate_id, original_sales_invoice_id = EXCLUDED.original_sales_invoice_id,
    delivery = EXCLUDED.delivery, total_excl_tax = EXCLUDED.total_excl_tax, total_tax = EXCLUDED.total_tax,
    total_incl_tax = EXCLUDED.total_incl_tax, total_discount = EXCLUDED.total_discount, tax_totals = EXCLUDED.tax_totals,
    updated_at = EXCLUDED.updated_at;

-- name: TouchEstimate :exec
UPDATE estimates SET updated_at = $3 WHERE administration_id = $1 AND id = $2;

-- name: DeleteEstimate :exec
DELETE FROM estimates WHERE administration_id = $1 AND id = $2;

-- name: EstimateDetailsOf :many
SELECT * FROM estimate_details WHERE administration_id = @administration_id AND estimate_id = ANY(@ids::bigint[]) ORDER BY row_order, id;

-- name: DeleteEstimateDetails :exec
DELETE FROM estimate_details WHERE estimate_id = $1;

-- name: InsertEstimateDetail :exec
INSERT INTO estimate_details (id, administration_id, estimate_id, description, period, price, amount, quantity, tax_rate_id,
    ledger_account_id, project_id, product_id, row_order, is_optional, is_selected, line_base, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18);

-- name: EstimateCustomFieldValuesOf :many
SELECT v.estimate_id, f.id, f.name, v.value
FROM estimate_custom_field_values v JOIN custom_fields f ON f.id = v.custom_field_id
WHERE f.administration_id = @administration_id AND v.estimate_id = ANY(@ids::bigint[])
ORDER BY f.position, f.id;

-- name: DeleteEstimateCustomFieldValues :exec
DELETE FROM estimate_custom_field_values WHERE estimate_id = $1;

-- name: InsertEstimateCustomFieldValue :exec
INSERT INTO estimate_custom_field_values (estimate_id, custom_field_id, value) VALUES ($1, $2, $3);

-- name: SetSalesInvoiceOriginalEstimate :exec
UPDATE sales_invoices SET original_estimate_id = $3 WHERE administration_id = $1 AND id = $2;

-- name: ContactOnEstimate :one
SELECT EXISTS (SELECT 1 FROM estimates WHERE administration_id = @administration_id AND contact_id = @contact_id);

-- name: ProductOnEstimates :one
SELECT EXISTS (SELECT 1 FROM estimate_details WHERE administration_id = @administration_id AND product_id = @product_id);

-- name: ProjectOnEstimates :one
SELECT EXISTS (SELECT 1 FROM estimate_details WHERE administration_id = @administration_id AND project_id = @project_id);
