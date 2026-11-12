-- +goose Up
-- Invoice workflows carry the payment conditions new invoices start with,
-- as the Send invoice step of Moneybird's workflow does, filled with
-- Moneybird's default text in the workflow's language.
ALTER TABLE workflows ADD COLUMN payment_conditions text NOT NULL DEFAULT '';

UPDATE workflows
SET payment_conditions = CASE WHEN language LIKE 'nl%'
    THEN 'We verzoeken u vriendelijk het bovenstaande bedrag van {document.total_price} voor {document.due_date} te voldoen op onze bankrekening onder vermelding van de omschrijving {document.payment_reference}. Voor vragen kunt u contact opnemen per e-mail.'
    ELSE 'We kindly request you to pay the above amount of {document.total_price} before {document.due_date} to our bank account, stating the description {document.payment_reference}. If you have any questions, please contact us by email.'
    END
WHERE type = 'InvoiceWorkflow';

-- Sent invoices get a payment reference (an RF creditor reference) to find
-- their bank transactions by.
CREATE INDEX sales_invoices_payment_reference_idx ON sales_invoices (administration_id, payment_reference) WHERE payment_reference <> '';

-- +goose Down
DROP INDEX sales_invoices_payment_reference_idx;
ALTER TABLE workflows DROP COLUMN payment_conditions;
