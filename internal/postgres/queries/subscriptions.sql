-- name: UpsertSubscription :exec
INSERT INTO subscriptions (id, administration_id, contact_id, contact_person_id, recurring_sales_invoice_id, start_date, end_date,
                           frequency_count, frequency_type, reference, cancelled_at, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
ON CONFLICT (id) DO UPDATE SET contact_person_id = EXCLUDED.contact_person_id, recurring_sales_invoice_id = EXCLUDED.recurring_sales_invoice_id,
    end_date = EXCLUDED.end_date, reference = EXCLUDED.reference, cancelled_at = EXCLUDED.cancelled_at, updated_at = EXCLUDED.updated_at
WHERE subscriptions.administration_id = EXCLUDED.administration_id;

-- name: DeleteSubscriptionProducts :exec
DELETE FROM subscription_products WHERE subscription_id = $1;

-- name: InsertSubscriptionProduct :exec
INSERT INTO subscription_products (id, subscription_id, product_id, start_date, end_date, amount, quantity, discount)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8);

-- name: SubscriptionByID :one
SELECT * FROM subscriptions WHERE administration_id = $1 AND id = $2;

-- name: SubscriptionByRecurring :one
SELECT * FROM subscriptions WHERE administration_id = $1 AND recurring_sales_invoice_id = $2;

-- name: ListContactSubscriptions :many
SELECT * FROM subscriptions WHERE administration_id = $1 AND contact_id = $2 ORDER BY created_at, id;

-- name: ListSubscriptionProducts :many
SELECT * FROM subscription_products WHERE subscription_id = ANY(@subscription_ids::bigint[]) ORDER BY start_date, id;

-- name: InsertAdditionalCharge :exec
INSERT INTO additional_charges (id, administration_id, contact_id, subscription_id, product_id, amount, quantity, price, period_from, period_to,
                                description, identifier, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14);

-- name: ListAdditionalCharges :many
SELECT * FROM additional_charges
WHERE administration_id = @administration_id
  AND (sqlc.narg(contact_id)::bigint IS NULL OR contact_id = sqlc.narg(contact_id))
  AND (sqlc.narg(subscription_id)::bigint IS NULL OR subscription_id = sqlc.narg(subscription_id))
  AND (@include_billed::boolean OR detail_id IS NULL)
ORDER BY created_at, id;

-- name: UnbilledContactCharges :many
SELECT * FROM additional_charges
WHERE administration_id = $1 AND contact_id IS NOT NULL AND detail_id IS NULL
ORDER BY created_at, id;

-- name: BillAdditionalCharge :exec
UPDATE additional_charges SET sales_invoice_id = $3, detail_id = $4, updated_at = $5 WHERE administration_id = $1 AND id = $2;

-- name: ChargeWorkload :many
-- Administrations with unbilled contact charges, each with its
-- longest-standing active owner.
SELECT w.administration_id, o.user_id
FROM (SELECT DISTINCT administration_id FROM additional_charges WHERE detail_id IS NULL AND contact_id IS NOT NULL) w
JOIN LATERAL (
    SELECT m.user_id FROM administration_members m
    WHERE m.administration_id = w.administration_id AND m.active AND 'ownership' = ANY(m.permissions)
    ORDER BY m.created_at, m.user_id LIMIT 1
) o ON true
ORDER BY w.administration_id;

-- name: ListOneOffInvoices :many
SELECT sales_invoice_id FROM subscription_one_off_invoices WHERE administration_id = $1 AND subscription_id = $2 ORDER BY sales_invoice_id;

-- name: InsertOneOffInvoice :exec
INSERT INTO subscription_one_off_invoices (sales_invoice_id, subscription_id, administration_id) VALUES ($1, $2, $3);

-- name: ProductInSubscriptions :one
SELECT (EXISTS (SELECT 1 FROM subscription_products sp JOIN subscriptions s ON s.id = sp.subscription_id
                WHERE s.administration_id = @administration_id AND sp.product_id = @product_id)
     OR EXISTS (SELECT 1 FROM additional_charges WHERE administration_id = @administration_id AND product_id = @product_id))::boolean AS in_use;

-- name: UpsertDownload :exec
INSERT INTO downloads (id, administration_id, user_id, download_type, filename, content_type, size, failed, downloaded, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
ON CONFLICT (id) DO UPDATE SET downloaded = EXCLUDED.downloaded, failed = EXCLUDED.failed, updated_at = EXCLUDED.updated_at
WHERE downloads.administration_id = EXCLUDED.administration_id;

-- name: DownloadByID :one
SELECT * FROM downloads WHERE administration_id = $1 AND id = $2;

-- name: ListDownloads :many
SELECT sqlc.embed(d), count(*) OVER () AS total
FROM downloads d
WHERE d.administration_id = @administration_id
  AND d.download_type = ANY(@types::text[])
  AND (@download_type::text = '' OR d.download_type = @download_type)
  AND (sqlc.narg(downloaded)::boolean IS NULL OR d.downloaded = sqlc.narg(downloaded))
  AND (sqlc.narg(failed)::boolean IS NULL OR d.failed = sqlc.narg(failed))
ORDER BY d.created_at DESC, d.id DESC
LIMIT sqlc.narg(max_rows) OFFSET @skip;
