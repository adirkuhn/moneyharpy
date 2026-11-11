-- +goose Up
-- estimates are quotes: lines like a sales invoice, numbered from the
-- administration's estimate sequence when sent, then accepted, rejected,
-- billed or archived. state late is only stored when set by hand; an open
-- estimate past its due_date shows as late.
CREATE TABLE estimates (
    id                        bigint PRIMARY KEY,
    administration_id         bigint  NOT NULL REFERENCES administrations (id) ON DELETE CASCADE,
    contact_id                bigint  NOT NULL REFERENCES contacts (id),
    contact_person_id         bigint  REFERENCES contact_people (id) ON DELETE SET NULL,
    identity_id               bigint  NOT NULL,
    document_style_id         bigint  NOT NULL DEFAULT 0,
    workflow_id               bigint  NOT NULL DEFAULT 0,
    draft_id                  integer NOT NULL,
    estimate_id               text    NOT NULL DEFAULT '',
    sequence_id               text    NOT NULL DEFAULT '',
    reference                 text    NOT NULL DEFAULT '',
    language                  text    NOT NULL,
    currency                  text    NOT NULL,
    prices_are_incl_tax       boolean NOT NULL DEFAULT false,
    show_tax                  boolean NOT NULL DEFAULT true,
    sign_online               boolean NOT NULL DEFAULT true,
    discount                  integer NOT NULL DEFAULT 0,
    estimate_date             date,
    due_date                  date,
    first_due_interval        integer NOT NULL DEFAULT 30,
    pre_text                  text    NOT NULL DEFAULT '',
    post_text                 text    NOT NULL DEFAULT '',
    state                     text    NOT NULL DEFAULT 'draft',
    sent_at                   timestamptz,
    accepted_at               date,
    rejected_at               date,
    archived_at               date,
    original_estimate_id      bigint  REFERENCES estimates (id) ON DELETE SET NULL,
    original_sales_invoice_id bigint  REFERENCES sales_invoices (id) ON DELETE SET NULL,
    delivery                  jsonb,
    total_excl_tax            bigint  NOT NULL DEFAULT 0,
    total_tax                 bigint  NOT NULL DEFAULT 0,
    total_incl_tax            bigint  NOT NULL DEFAULT 0,
    total_discount            bigint  NOT NULL DEFAULT 0,
    tax_totals                jsonb   NOT NULL DEFAULT '[]',
    created_at                timestamptz NOT NULL,
    updated_at                timestamptz NOT NULL
);
CREATE UNIQUE INDEX estimates_number_idx ON estimates (administration_id, estimate_id) WHERE estimate_id <> '';
CREATE INDEX estimates_admin_date_idx ON estimates (administration_id, estimate_date);
CREATE INDEX estimates_contact_idx ON estimates (contact_id);

CREATE TABLE estimate_details (
    id                bigint PRIMARY KEY,
    administration_id bigint  NOT NULL REFERENCES administrations (id) ON DELETE CASCADE,
    estimate_id       bigint  NOT NULL REFERENCES estimates (id) ON DELETE CASCADE,
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
CREATE INDEX estimate_details_estimate_idx ON estimate_details (estimate_id);

CREATE TABLE estimate_custom_field_values (
    estimate_id     bigint NOT NULL REFERENCES estimates (id) ON DELETE CASCADE,
    custom_field_id bigint NOT NULL REFERENCES custom_fields (id) ON DELETE CASCADE,
    value           text   NOT NULL,
    PRIMARY KEY (estimate_id, custom_field_id)
);

-- A sales invoice billed from an estimate names it, as at Moneybird.
ALTER TABLE sales_invoices ADD COLUMN original_estimate_id bigint REFERENCES estimates (id) ON DELETE SET NULL;

-- +goose Down
ALTER TABLE sales_invoices DROP COLUMN original_estimate_id;
DELETE FROM notes WHERE entity_type = 'Estimate';
DELETE FROM attachments WHERE entity_type = 'Estimate';
DELETE FROM resources WHERE type = 'estimates';
DROP TABLE estimate_custom_field_values;
DROP TABLE estimate_details;
DROP TABLE estimates;
