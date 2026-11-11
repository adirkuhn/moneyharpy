-- Links between areas that read across them.

-- name: SubscriptionsOfSalesInvoices :many
-- The subscription each sales invoice bills: made by its recurring sales
-- invoice, or scheduled as a one-off to merge into its next invoice.
SELECT si.id AS sales_invoice_id, s.id AS subscription_id
FROM sales_invoices si JOIN subscriptions s ON s.recurring_sales_invoice_id = si.recurring_sales_invoice_id
WHERE si.administration_id = @administration_id AND si.id = ANY(@ids::bigint[])
UNION ALL
SELECT o.sales_invoice_id, o.subscription_id
FROM subscription_one_off_invoices o
WHERE o.administration_id = @administration_id AND o.sales_invoice_id = ANY(@ids::bigint[]);

-- name: SubscriptionsOfRecurringSalesInvoices :many
SELECT id, recurring_sales_invoice_id::bigint AS recurring_sales_invoice_id FROM subscriptions
WHERE administration_id = @administration_id AND recurring_sales_invoice_id = ANY(@ids::bigint[]);
