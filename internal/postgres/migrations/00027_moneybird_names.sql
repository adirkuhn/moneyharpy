-- +goose Up
-- Document styles take Moneybird's names: the sender line of the company's
-- website is company_website_url, and the label toggles of emails and public
-- views are email_moneybird_label_enabled and
-- public_view_moneybird_label_enabled.
UPDATE document_styles d
SET layout = d.layout || COALESCE((
    SELECT jsonb_object_agg(k, (
        SELECT COALESCE(jsonb_agg(CASE WHEN l ->> 'field' = 'website'
                                       THEN jsonb_set(l, '{field}', '"company_website_url"') ELSE l END ORDER BY n), '[]'::jsonb)
        FROM jsonb_array_elements(d.layout -> k) WITH ORDINALITY AS e (l, n)))
    FROM unnest(ARRAY['invoice_sender_address', 'estimate_sender_address']) AS k
    WHERE jsonb_typeof(d.layout -> k) = 'array'), '{}'::jsonb)
WHERE d.layout::text LIKE '%"website"%';

UPDATE document_styles
SET channels = (channels - 'email_label_enabled' - 'public_view_label_enabled')
    || CASE WHEN channels ? 'email_label_enabled'
            THEN jsonb_build_object('email_moneybird_label_enabled', channels -> 'email_label_enabled') ELSE '{}'::jsonb END
    || CASE WHEN channels ? 'public_view_label_enabled'
            THEN jsonb_build_object('public_view_moneybird_label_enabled', channels -> 'public_view_label_enabled') ELSE '{}'::jsonb END
WHERE channels ?| ARRAY['email_label_enabled', 'public_view_label_enabled'];

-- Sales invoice events take Moneybird's actions: an invoice a subscription
-- made was created based on the subscription, and the original of a credit
-- invoice records nothing (Moneybird has no credit_invoice_created).
UPDATE events e
SET action = 'sales_invoice_created_based_on_subscription'
FROM sales_invoices i
JOIN subscriptions s ON s.recurring_sales_invoice_id = i.recurring_sales_invoice_id
WHERE e.action = 'sales_invoice_created_based_on_recurring' AND e.entity_type = 'SalesInvoice'
  AND e.entity_id = i.id AND e.administration_id = i.administration_id;

DELETE FROM events WHERE action = 'credit_invoice_created';

-- +goose Down
UPDATE events SET action = 'sales_invoice_created_based_on_recurring'
WHERE action = 'sales_invoice_created_based_on_subscription' AND entity_type = 'SalesInvoice';

UPDATE document_styles d
SET layout = d.layout || COALESCE((
    SELECT jsonb_object_agg(k, (
        SELECT COALESCE(jsonb_agg(CASE WHEN l ->> 'field' = 'company_website_url'
                                       THEN jsonb_set(l, '{field}', '"website"') ELSE l END ORDER BY n), '[]'::jsonb)
        FROM jsonb_array_elements(d.layout -> k) WITH ORDINALITY AS e (l, n)))
    FROM unnest(ARRAY['invoice_sender_address', 'estimate_sender_address']) AS k
    WHERE jsonb_typeof(d.layout -> k) = 'array'), '{}'::jsonb)
WHERE d.layout::text LIKE '%"company_website_url"%';

UPDATE document_styles
SET channels = (channels - 'email_moneybird_label_enabled' - 'public_view_moneybird_label_enabled')
    || CASE WHEN channels ? 'email_moneybird_label_enabled'
            THEN jsonb_build_object('email_label_enabled', channels -> 'email_moneybird_label_enabled') ELSE '{}'::jsonb END
    || CASE WHEN channels ? 'public_view_moneybird_label_enabled'
            THEN jsonb_build_object('public_view_label_enabled', channels -> 'public_view_moneybird_label_enabled') ELSE '{}'::jsonb END
WHERE channels ?| ARRAY['email_moneybird_label_enabled', 'public_view_moneybird_label_enabled'];
