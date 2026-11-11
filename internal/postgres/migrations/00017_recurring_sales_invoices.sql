-- +goose Up
-- recurring_sales_invoices are templates that make a draft sales invoice on
-- each date of their schedule. The schedule counts frequency_count units of
-- frequency_type from anchor_date; period_count is how many invoices were made
-- since, generated_count how many in all.
CREATE TABLE recurring_sales_invoices (
    id                     bigint PRIMARY KEY,
    administration_id      bigint  NOT NULL REFERENCES administrations (id) ON DELETE CASCADE,
    contact_id             bigint  NOT NULL REFERENCES contacts (id),
    contact_person_id      bigint  REFERENCES contact_people (id) ON DELETE SET NULL,
    identity_id            bigint  NOT NULL,
    document_style_id      bigint  NOT NULL DEFAULT 0,
    workflow_id            bigint  NOT NULL DEFAULT 0,
    reference              text    NOT NULL DEFAULT '',
    language               text    NOT NULL,
    currency               text    NOT NULL,
    prices_are_incl_tax    boolean NOT NULL DEFAULT false,
    payment_conditions     text    NOT NULL DEFAULT '',
    discount               integer NOT NULL DEFAULT 0,
    first_due_interval     integer NOT NULL DEFAULT 14,
    start_date             date    NOT NULL,
    invoice_date           date    NOT NULL,
    last_date              date,
    active                 boolean NOT NULL DEFAULT true,
    frequency_type         text    NOT NULL,
    frequency_count        integer NOT NULL DEFAULT 1,
    auto_send              boolean NOT NULL DEFAULT false,
    auto_send_at           timestamptz,
    auto_send_user_id      bigint,
    mergeable              boolean NOT NULL DEFAULT false,
    anchor_date            date    NOT NULL,
    period_count           integer NOT NULL DEFAULT 0,
    generated_count        integer NOT NULL DEFAULT 0,
    total_excl_tax         bigint  NOT NULL DEFAULT 0,
    total_tax              bigint  NOT NULL DEFAULT 0,
    total_incl_tax         bigint  NOT NULL DEFAULT 0,
    total_discount         bigint  NOT NULL DEFAULT 0,
    tax_totals             jsonb   NOT NULL DEFAULT '[]',
    created_at             timestamptz NOT NULL,
    updated_at             timestamptz NOT NULL
);
CREATE INDEX recurring_sales_invoices_admin_idx ON recurring_sales_invoices (administration_id, active, invoice_date);
CREATE INDEX recurring_sales_invoices_contact_idx ON recurring_sales_invoices (contact_id);

CREATE TABLE recurring_sales_invoice_details (
    id                         bigint PRIMARY KEY,
    administration_id          bigint  NOT NULL REFERENCES administrations (id) ON DELETE CASCADE,
    recurring_sales_invoice_id bigint  NOT NULL REFERENCES recurring_sales_invoices (id) ON DELETE CASCADE,
    description                text    NOT NULL DEFAULT '',
    period                     text    NOT NULL DEFAULT '',
    price                      bigint  NOT NULL DEFAULT 0,
    amount                     text    NOT NULL DEFAULT '1',
    quantity                   bigint  NOT NULL DEFAULT 1000,
    tax_rate_id                bigint  NOT NULL REFERENCES tax_rates (id),
    ledger_account_id          bigint  REFERENCES ledger_accounts (id),
    project_id                 bigint,
    product_id                 bigint,
    row_order                  integer NOT NULL DEFAULT 0,
    is_optional                boolean NOT NULL DEFAULT false,
    is_selected                boolean NOT NULL DEFAULT true,
    line_base                  bigint  NOT NULL DEFAULT 0,
    created_at                 timestamptz NOT NULL,
    updated_at                 timestamptz NOT NULL
);
CREATE INDEX recurring_sales_invoice_details_invoice_idx ON recurring_sales_invoice_details (recurring_sales_invoice_id);

CREATE TABLE recurring_sales_invoice_custom_field_values (
    recurring_sales_invoice_id bigint NOT NULL REFERENCES recurring_sales_invoices (id) ON DELETE CASCADE,
    custom_field_id            bigint NOT NULL REFERENCES custom_fields (id) ON DELETE CASCADE,
    value                      text   NOT NULL,
    PRIMARY KEY (recurring_sales_invoice_id, custom_field_id)
);

CREATE INDEX sales_invoices_recurring_idx ON sales_invoices (recurring_sales_invoice_id) WHERE recurring_sales_invoice_id IS NOT NULL;

-- +goose Down
DROP INDEX sales_invoices_recurring_idx;
DELETE FROM notes WHERE entity_type = 'RecurringSalesInvoice';
DELETE FROM resources WHERE type = 'recurring_sales_invoices';
UPDATE sales_invoices SET recurring_sales_invoice_id = NULL WHERE recurring_sales_invoice_id IS NOT NULL;
DROP TABLE recurring_sales_invoice_custom_field_values;
DROP TABLE recurring_sales_invoice_details;
DROP TABLE recurring_sales_invoices;
