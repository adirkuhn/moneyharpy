-- +goose Up
ALTER TABLE workflows
    ADD COLUMN invoice_email_subject text NOT NULL DEFAULT '',
    ADD COLUMN invoice_email_body    text NOT NULL DEFAULT '';

CREATE TABLE sales_invoices (
    id                         bigint PRIMARY KEY,
    administration_id          bigint  NOT NULL REFERENCES administrations (id) ON DELETE CASCADE,
    contact_id                 bigint  NOT NULL REFERENCES contacts (id),
    contact_person_id          bigint  REFERENCES contact_people (id) ON DELETE SET NULL,
    identity_id                bigint  NOT NULL,
    document_style_id          bigint  NOT NULL DEFAULT 0,
    workflow_id                bigint  NOT NULL DEFAULT 0,
    draft_id                   integer NOT NULL,
    invoice_id                 text    NOT NULL DEFAULT '',
    sequence_id                text    NOT NULL DEFAULT '',
    reference                  text    NOT NULL DEFAULT '',
    language                   text    NOT NULL,
    currency                   text    NOT NULL,
    prices_are_incl_tax        boolean NOT NULL DEFAULT false,
    payment_conditions         text    NOT NULL DEFAULT '',
    discount                   integer NOT NULL DEFAULT 0,
    invoice_date               date,
    due_date                   date,
    first_due_interval         integer NOT NULL DEFAULT 14,
    state                      text    NOT NULL DEFAULT 'draft',
    paused                     boolean NOT NULL DEFAULT false,
    payment_reference          text    NOT NULL DEFAULT '',
    sent_at                    timestamptz,
    paid_at                    timestamptz,
    marked_dubious_on          date,
    marked_uncollectible_on    date,
    reminder_count             integer NOT NULL DEFAULT 0,
    original_id                bigint  REFERENCES sales_invoices (id) ON DELETE SET NULL,
    recurring_sales_invoice_id bigint,
    delivery                   jsonb,
    total_excl_tax             bigint  NOT NULL DEFAULT 0,
    total_tax                  bigint  NOT NULL DEFAULT 0,
    total_incl_tax             bigint  NOT NULL DEFAULT 0,
    total_discount             bigint  NOT NULL DEFAULT 0,
    tax_totals                 jsonb   NOT NULL DEFAULT '[]',
    created_at                 timestamptz NOT NULL,
    updated_at                 timestamptz NOT NULL
);
CREATE UNIQUE INDEX sales_invoices_number_idx ON sales_invoices (administration_id, invoice_id) WHERE invoice_id <> '';
CREATE INDEX sales_invoices_admin_date_idx ON sales_invoices (administration_id, invoice_date);
CREATE INDEX sales_invoices_contact_idx ON sales_invoices (contact_id);
CREATE INDEX sales_invoices_state_idx ON sales_invoices (administration_id, state);

CREATE TABLE sales_invoice_details (
    id                bigint PRIMARY KEY,
    administration_id bigint  NOT NULL REFERENCES administrations (id) ON DELETE CASCADE,
    sales_invoice_id  bigint  NOT NULL REFERENCES sales_invoices (id) ON DELETE CASCADE,
    description       text    NOT NULL DEFAULT '',
    period            text    NOT NULL DEFAULT '',
    price             bigint  NOT NULL DEFAULT 0,
    amount            text    NOT NULL DEFAULT '1',
    quantity          bigint  NOT NULL DEFAULT 1000,
    tax_rate_id       bigint  NOT NULL REFERENCES tax_rates (id),
    ledger_account_id bigint  REFERENCES ledger_accounts (id),
    project_id        bigint,
    product_id        bigint,
    row_order         integer NOT NULL DEFAULT 0,
    is_optional       boolean NOT NULL DEFAULT false,
    is_selected       boolean NOT NULL DEFAULT true,
    line_base         bigint  NOT NULL DEFAULT 0,
    created_at        timestamptz NOT NULL,
    updated_at        timestamptz NOT NULL
);
CREATE INDEX sales_invoice_details_invoice_idx ON sales_invoice_details (sales_invoice_id);

CREATE TABLE sales_invoice_custom_field_values (
    sales_invoice_id bigint NOT NULL REFERENCES sales_invoices (id) ON DELETE CASCADE,
    custom_field_id  bigint NOT NULL REFERENCES custom_fields (id) ON DELETE CASCADE,
    value            text   NOT NULL,
    PRIMARY KEY (sales_invoice_id, custom_field_id)
);

-- +goose Down
DELETE FROM resources WHERE type = 'sales_invoices';
DROP TABLE sales_invoice_custom_field_values;
DROP TABLE sales_invoice_details;
DROP TABLE sales_invoices;
ALTER TABLE workflows DROP COLUMN invoice_email_subject, DROP COLUMN invoice_email_body;
