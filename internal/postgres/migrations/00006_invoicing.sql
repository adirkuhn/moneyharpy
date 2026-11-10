-- +goose Up
ALTER TABLE administrations
    ADD COLUMN invoice_id_template           text    NOT NULL DEFAULT '{year}-{id}',
    ADD COLUMN invoice_sequence_id           text    NOT NULL DEFAULT '0001',
    ADD COLUMN estimate_id_template          text    NOT NULL DEFAULT '{year}-{id}',
    ADD COLUMN estimate_sequence_id          text    NOT NULL DEFAULT '0001',
    ADD COLUMN invoice_bcc_email_address     text    NOT NULL DEFAULT '',
    ADD COLUMN invoice_reminders_bcc_enabled boolean NOT NULL DEFAULT false,
    ADD COLUMN estimate_bcc_email_address    text    NOT NULL DEFAULT '',
    ADD COLUMN order_bcc_email_address       text    NOT NULL DEFAULT '',
    ADD COLUMN default_email_ubl             boolean NOT NULL DEFAULT true,
    ADD COLUMN payments_without_proof        boolean NOT NULL DEFAULT true;

ALTER TABLE custom_fields ADD COLUMN position integer NOT NULL DEFAULT 0;
UPDATE custom_fields f
SET position = o.n
FROM (SELECT id, row_number() OVER (PARTITION BY administration_id, source ORDER BY id) AS n FROM custom_fields) o
WHERE o.id = f.id;

CREATE TABLE workflows (
    id                  bigint PRIMARY KEY,
    administration_id   bigint  NOT NULL REFERENCES administrations (id) ON DELETE CASCADE,
    type                text    NOT NULL,
    name                text    NOT NULL,
    is_default          boolean NOT NULL DEFAULT false,
    currency            text    NOT NULL,
    language            text    NOT NULL,
    active              boolean NOT NULL DEFAULT true,
    prices_are_incl_tax boolean NOT NULL DEFAULT false,
    show_qr_code        boolean NOT NULL DEFAULT false,
    created_at          timestamptz NOT NULL,
    updated_at          timestamptz NOT NULL
);
CREATE INDEX workflows_administration_id_idx ON workflows (administration_id);
CREATE UNIQUE INDEX workflows_one_default_idx ON workflows (administration_id, type) WHERE is_default;

CREATE TABLE workflow_steps (
    id                 bigint PRIMARY KEY,
    workflow_id        bigint  NOT NULL REFERENCES workflows (id) ON DELETE CASCADE,
    step_order         integer NOT NULL,
    due_interval       integer NOT NULL DEFAULT 0,
    reminder_delay     integer NOT NULL DEFAULT 0,
    reminder_text      text    NOT NULL DEFAULT '',
    reminder_auto_send boolean NOT NULL DEFAULT false,
    created_at         timestamptz NOT NULL,
    updated_at         timestamptz NOT NULL,
    UNIQUE (workflow_id, step_order)
);

CREATE TABLE document_styles (
    id                        bigint PRIMARY KEY,
    administration_id         bigint  NOT NULL REFERENCES administrations (id) ON DELETE CASCADE,
    name                      text    NOT NULL,
    identity_id               bigint  NOT NULL REFERENCES identities (id),
    is_default                boolean NOT NULL DEFAULT false,
    logo_hash                 text,
    logo_content_type         text,
    logo_container_full_width boolean NOT NULL DEFAULT false,
    logo_display_width        integer NOT NULL DEFAULT 50,
    logo_position             text    NOT NULL DEFAULT 'left',
    background_hash           text,
    background_content_type   text,
    print_on_stationery       boolean NOT NULL DEFAULT false,
    paper_size                text    NOT NULL DEFAULT 'A4',
    address_position          text    NOT NULL DEFAULT 'left',
    font_size                 integer NOT NULL DEFAULT 10,
    font_family               text    NOT NULL DEFAULT 'lucida_grande',
    custom_css                text    NOT NULL DEFAULT '',
    layout                    jsonb   NOT NULL DEFAULT '{}',
    created_at                timestamptz NOT NULL,
    updated_at                timestamptz NOT NULL
);
CREATE INDEX document_styles_administration_id_idx ON document_styles (administration_id);
CREATE INDEX document_styles_identity_id_idx ON document_styles (identity_id);
CREATE UNIQUE INDEX document_styles_one_default_idx ON document_styles (administration_id) WHERE is_default;

-- +goose Down
DELETE FROM resources WHERE type IN ('workflows', 'document_styles');
DROP TABLE document_styles;
DROP TABLE workflow_steps;
DROP TABLE workflows;
ALTER TABLE custom_fields DROP COLUMN position;
ALTER TABLE administrations
    DROP COLUMN invoice_id_template,
    DROP COLUMN invoice_sequence_id,
    DROP COLUMN estimate_id_template,
    DROP COLUMN estimate_sequence_id,
    DROP COLUMN invoice_bcc_email_address,
    DROP COLUMN invoice_reminders_bcc_enabled,
    DROP COLUMN estimate_bcc_email_address,
    DROP COLUMN order_bcc_email_address,
    DROP COLUMN default_email_ubl,
    DROP COLUMN payments_without_proof;
