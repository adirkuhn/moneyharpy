-- +goose Up
CREATE TABLE contacts (
    id                          bigint PRIMARY KEY,
    administration_id           bigint  NOT NULL REFERENCES administrations (id) ON DELETE CASCADE,
    company_name                text    NOT NULL DEFAULT '',
    firstname                   text    NOT NULL DEFAULT '',
    lastname                    text    NOT NULL DEFAULT '',
    address1                    text    NOT NULL DEFAULT '',
    address2                    text    NOT NULL DEFAULT '',
    zipcode                     text    NOT NULL DEFAULT '',
    city                        text    NOT NULL DEFAULT '',
    country                     text    NOT NULL,
    phone                       text    NOT NULL DEFAULT '',
    customer_id                 text    NOT NULL,
    tax_number                  text    NOT NULL DEFAULT '',
    chamber_of_commerce         text    NOT NULL DEFAULT '',
    bank_account                text    NOT NULL DEFAULT '',
    delivery_method             text    NOT NULL DEFAULT 'Email',
    send_invoices_to_attention  text    NOT NULL DEFAULT '',
    send_invoices_to_email      text    NOT NULL DEFAULT '',
    send_estimates_to_attention text    NOT NULL DEFAULT '',
    send_estimates_to_email     text    NOT NULL DEFAULT '',
    email_ubl                   boolean NOT NULL DEFAULT false,
    direct_debit                boolean NOT NULL DEFAULT false,
    sepa_active                 boolean NOT NULL DEFAULT false,
    sepa_iban                   text    NOT NULL DEFAULT '',
    sepa_iban_account_name      text    NOT NULL DEFAULT '',
    sepa_bic                    text    NOT NULL DEFAULT '',
    sepa_mandate_id             text    NOT NULL DEFAULT '',
    sepa_mandate_date           date,
    sepa_sequence_type          text    NOT NULL DEFAULT 'RCUR',
    si_identifier_type          text    NOT NULL DEFAULT '',
    si_identifier               text    NOT NULL DEFAULT '',
    invoice_workflow_id         bigint  REFERENCES workflows (id) ON DELETE SET NULL,
    estimate_workflow_id        bigint  REFERENCES workflows (id) ON DELETE SET NULL,
    archived                    boolean NOT NULL DEFAULT false,
    created_at                  timestamptz NOT NULL,
    updated_at                  timestamptz NOT NULL,
    -- sort_name orders lists: the company, else the person's name.
    sort_name text GENERATED ALWAYS AS (lower(CASE WHEN company_name <> '' THEN company_name ELSE btrim(firstname || ' ' || lastname) END)) STORED
);
CREATE UNIQUE INDEX contacts_customer_id_idx ON contacts (administration_id, customer_id);
CREATE INDEX contacts_admin_sort_idx ON contacts (administration_id, sort_name, id);
CREATE INDEX contacts_invoice_workflow_idx ON contacts (invoice_workflow_id);
CREATE INDEX contacts_estimate_workflow_idx ON contacts (estimate_workflow_id);

CREATE TABLE contact_people (
    id                bigint PRIMARY KEY,
    administration_id bigint NOT NULL REFERENCES administrations (id) ON DELETE CASCADE,
    contact_id        bigint NOT NULL REFERENCES contacts (id) ON DELETE CASCADE,
    firstname         text   NOT NULL DEFAULT '',
    lastname          text   NOT NULL DEFAULT '',
    phone             text   NOT NULL DEFAULT '',
    email             text   NOT NULL DEFAULT '',
    department        text   NOT NULL DEFAULT '',
    created_at        timestamptz NOT NULL,
    updated_at        timestamptz NOT NULL
);
CREATE INDEX contact_people_contact_idx ON contact_people (contact_id);

CREATE TABLE contact_custom_field_values (
    contact_id      bigint NOT NULL REFERENCES contacts (id) ON DELETE CASCADE,
    custom_field_id bigint NOT NULL REFERENCES custom_fields (id) ON DELETE CASCADE,
    value           text   NOT NULL,
    PRIMARY KEY (contact_id, custom_field_id)
);
CREATE INDEX contact_custom_field_values_field_idx ON contact_custom_field_values (custom_field_id);

-- notes are remarks and to-dos on any record; entity_type says which kind.
CREATE TABLE notes (
    id                bigint PRIMARY KEY,
    administration_id bigint  NOT NULL REFERENCES administrations (id) ON DELETE CASCADE,
    entity_id         bigint  NOT NULL,
    entity_type       text    NOT NULL,
    user_id           bigint  REFERENCES users (id) ON DELETE SET NULL,
    assignee_id       bigint  REFERENCES users (id) ON DELETE SET NULL,
    todo              boolean NOT NULL DEFAULT false,
    note              text    NOT NULL,
    completed_at      timestamptz,
    completed_by_id   bigint  REFERENCES users (id) ON DELETE SET NULL,
    created_at        timestamptz NOT NULL,
    updated_at        timestamptz NOT NULL
);
CREATE INDEX notes_entity_idx ON notes (entity_id);
CREATE INDEX notes_open_todos_idx ON notes (administration_id, entity_id) WHERE todo AND completed_at IS NULL;

-- +goose Down
DELETE FROM resources WHERE type = 'contacts';
DROP TABLE notes;
DROP TABLE contact_custom_field_values;
DROP TABLE contact_people;
DROP TABLE contacts;
