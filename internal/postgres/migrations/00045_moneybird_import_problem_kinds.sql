-- +goose Up
-- What a record came in without is noted with the record, by its kind,
-- so importing again can bring it in. A file left out was noted under the
-- path Moneybird serves it from, and a PDF apart from its record, as kind
-- pdf and id 0.
UPDATE moneybird_import_problems
SET kind = CASE WHEN message LIKE 'sales_invoices/%' THEN 'sales_invoice' ELSE 'estimate' END,
    source_id = substring(message from '^[a-z_]+/([0-9]+)/download_pdf')::bigint
WHERE kind = 'pdf' AND message ~ '^(sales_invoices|estimates)/[0-9]+/download_pdf';

UPDATE moneybird_import_problems p
SET kind = k.kind
FROM (VALUES
    ('estimates', 'estimate'),
    ('sales_invoices', 'sales_invoice'),
    ('external_sales_invoices', 'external_sales_invoice'),
    ('documents/purchase_invoices', 'purchase_invoice'),
    ('documents/receipts', 'receipt'),
    ('documents/general_documents', 'general_document'),
    ('documents/typeless_documents', 'typeless_document'),
    ('documents/general_journal_documents', 'general_journal_document')
) AS k (path, kind)
WHERE p.kind = k.path
  AND NOT EXISTS (
    SELECT 1 FROM moneybird_import_problems q
    WHERE q.administration_id = p.administration_id AND q.kind = k.kind AND q.source_id = p.source_id AND q.message = p.message
  );

-- +goose Down
-- The notes stay with their records.
SELECT 1;
