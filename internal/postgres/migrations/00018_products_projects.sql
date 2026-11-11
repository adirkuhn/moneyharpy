-- +goose Up
-- products are what an administration sells; invoice lines that name one
-- take its description, price, VAT rate and category. A product documents
-- name is deactivated instead of deleted.
CREATE TABLE products (
    id                   bigint PRIMARY KEY,
    administration_id    bigint  NOT NULL REFERENCES administrations (id) ON DELETE CASCADE,
    title                text    NOT NULL DEFAULT '',
    description          text    NOT NULL DEFAULT '',
    checkout_description text    NOT NULL DEFAULT '',
    identifier           text    NOT NULL DEFAULT '',
    price                bigint  NOT NULL DEFAULT 0,
    currency             text    NOT NULL,
    frequency_count      integer NOT NULL DEFAULT 0,
    frequency_type       text    NOT NULL DEFAULT '',
    tax_rate_id          bigint  REFERENCES tax_rates (id),
    ledger_account_id    bigint  NOT NULL REFERENCES ledger_accounts (id),
    document_style_id    bigint,
    workflow_id          bigint,
    product_type         text    NOT NULL DEFAULT '',
    vat_rate_type        text    NOT NULL DEFAULT '',
    checkout_type        text    NOT NULL DEFAULT '',
    max_amount_per_order integer,
    active               boolean NOT NULL DEFAULT true,
    created_at           timestamptz NOT NULL,
    updated_at           timestamptz NOT NULL
);
CREATE INDEX products_admin_idx ON products (administration_id, active);
CREATE UNIQUE INDEX products_identifier_idx ON products (administration_id, identifier) WHERE identifier <> '';

-- projects are what lines and journal entries can be booked on; a project
-- something is booked on is archived instead of deleted.
CREATE TABLE projects (
    id                bigint PRIMARY KEY,
    administration_id bigint  NOT NULL REFERENCES administrations (id) ON DELETE CASCADE,
    name              text    NOT NULL,
    state             text    NOT NULL DEFAULT 'active',
    budget            integer,
    created_at        timestamptz NOT NULL,
    updated_at        timestamptz NOT NULL
);
CREATE UNIQUE INDEX projects_name_idx ON projects (administration_id, lower(name));

-- Lines and entries could name products and projects before these existed;
-- such names pointed nowhere.
UPDATE sales_invoice_details SET product_id = NULL, project_id = NULL WHERE product_id IS NOT NULL OR project_id IS NOT NULL;
UPDATE external_sales_invoice_details SET product_id = NULL, project_id = NULL WHERE product_id IS NOT NULL OR project_id IS NOT NULL;
UPDATE recurring_sales_invoice_details SET product_id = NULL, project_id = NULL WHERE product_id IS NOT NULL OR project_id IS NOT NULL;
UPDATE journal_entries SET project_id = NULL WHERE project_id IS NOT NULL;

ALTER TABLE sales_invoice_details
    ADD CONSTRAINT sales_invoice_details_product_fk FOREIGN KEY (product_id) REFERENCES products (id),
    ADD CONSTRAINT sales_invoice_details_project_fk FOREIGN KEY (project_id) REFERENCES projects (id);
ALTER TABLE external_sales_invoice_details
    ADD CONSTRAINT external_sales_invoice_details_product_fk FOREIGN KEY (product_id) REFERENCES products (id),
    ADD CONSTRAINT external_sales_invoice_details_project_fk FOREIGN KEY (project_id) REFERENCES projects (id);
ALTER TABLE recurring_sales_invoice_details
    ADD CONSTRAINT recurring_sales_invoice_details_product_fk FOREIGN KEY (product_id) REFERENCES products (id),
    ADD CONSTRAINT recurring_sales_invoice_details_project_fk FOREIGN KEY (project_id) REFERENCES projects (id);
ALTER TABLE journal_entries ADD CONSTRAINT journal_entries_project_fk FOREIGN KEY (project_id) REFERENCES projects (id);
CREATE INDEX sales_invoice_details_product_idx ON sales_invoice_details (product_id) WHERE product_id IS NOT NULL;
CREATE INDEX sales_invoice_details_project_idx ON sales_invoice_details (project_id) WHERE project_id IS NOT NULL;
CREATE INDEX external_sales_invoice_details_product_idx ON external_sales_invoice_details (product_id) WHERE product_id IS NOT NULL;
CREATE INDEX external_sales_invoice_details_project_idx ON external_sales_invoice_details (project_id) WHERE project_id IS NOT NULL;
CREATE INDEX recurring_sales_invoice_details_product_idx ON recurring_sales_invoice_details (product_id) WHERE product_id IS NOT NULL;
CREATE INDEX recurring_sales_invoice_details_project_idx ON recurring_sales_invoice_details (project_id) WHERE project_id IS NOT NULL;
CREATE INDEX journal_entries_project_idx ON journal_entries (project_id) WHERE project_id IS NOT NULL;

-- +goose Down
DROP INDEX journal_entries_project_idx;
DROP INDEX recurring_sales_invoice_details_project_idx;
DROP INDEX recurring_sales_invoice_details_product_idx;
DROP INDEX external_sales_invoice_details_project_idx;
DROP INDEX external_sales_invoice_details_product_idx;
DROP INDEX sales_invoice_details_project_idx;
DROP INDEX sales_invoice_details_product_idx;
ALTER TABLE journal_entries DROP CONSTRAINT journal_entries_project_fk;
ALTER TABLE recurring_sales_invoice_details DROP CONSTRAINT recurring_sales_invoice_details_product_fk, DROP CONSTRAINT recurring_sales_invoice_details_project_fk;
ALTER TABLE external_sales_invoice_details DROP CONSTRAINT external_sales_invoice_details_product_fk, DROP CONSTRAINT external_sales_invoice_details_project_fk;
ALTER TABLE sales_invoice_details DROP CONSTRAINT sales_invoice_details_product_fk, DROP CONSTRAINT sales_invoice_details_project_fk;
DELETE FROM resources WHERE type IN ('products', 'projects');
DROP TABLE projects;
DROP TABLE products;
