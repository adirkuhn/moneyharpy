-- +goose Up
-- Line and product prices are kept in millionths rather than cents, as a
-- price may have up to six decimals, such as 0.0825 per kWh. What a line
-- comes to, and every other amount, stays in cents.
UPDATE sales_invoice_details SET price = price * 10000;
UPDATE external_sales_invoice_details SET price = price * 10000;
UPDATE recurring_sales_invoice_details SET price = price * 10000;
UPDATE estimate_details SET price = price * 10000;
UPDATE purchase_document_details SET price = price * 10000;
UPDATE products SET price = price * 10000;

-- +goose Down
UPDATE sales_invoice_details SET price = round(price / 10000.0);
UPDATE external_sales_invoice_details SET price = round(price / 10000.0);
UPDATE recurring_sales_invoice_details SET price = round(price / 10000.0);
UPDATE estimate_details SET price = round(price / 10000.0);
UPDATE purchase_document_details SET price = round(price / 10000.0);
UPDATE products SET price = round(price / 10000.0);
