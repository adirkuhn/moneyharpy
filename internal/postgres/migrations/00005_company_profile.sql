-- +goose Up
ALTER TABLE administrations
    ADD COLUMN small_business_scheme          boolean NOT NULL DEFAULT false,
    ADD COLUMN company_activities_description text    NOT NULL DEFAULT '',
    ADD COLUMN company_website_url            text    NOT NULL DEFAULT '',
    ADD COLUMN company_website_absent         boolean NOT NULL DEFAULT false;

ALTER TABLE identities
    ADD COLUMN tax_number_absence_reason text NOT NULL DEFAULT '';

CREATE TABLE custom_fields (
    id                bigint PRIMARY KEY,
    administration_id bigint NOT NULL REFERENCES administrations (id) ON DELETE CASCADE,
    name              text NOT NULL,
    source            text NOT NULL,
    created_at        timestamptz NOT NULL,
    updated_at        timestamptz NOT NULL
);
CREATE INDEX custom_fields_administration_id_idx ON custom_fields (administration_id);

CREATE TABLE identity_custom_field_values (
    identity_id     bigint NOT NULL REFERENCES identities (id) ON DELETE CASCADE,
    custom_field_id bigint NOT NULL REFERENCES custom_fields (id) ON DELETE CASCADE,
    value           text NOT NULL,
    PRIMARY KEY (identity_id, custom_field_id)
);

-- +goose Down
DROP TABLE identity_custom_field_values;
DELETE FROM resources WHERE type = 'custom_fields';
DROP TABLE custom_fields;
ALTER TABLE identities DROP COLUMN tax_number_absence_reason;
ALTER TABLE administrations
    DROP COLUMN small_business_scheme,
    DROP COLUMN company_activities_description,
    DROP COLUMN company_website_url,
    DROP COLUMN company_website_absent;
