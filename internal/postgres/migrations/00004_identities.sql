-- +goose Up
CREATE TABLE identities (
    id                  bigint PRIMARY KEY,
    administration_id   bigint NOT NULL REFERENCES administrations (id) ON DELETE CASCADE,
    is_default          boolean NOT NULL DEFAULT false,
    company_name        text NOT NULL,
    country             text NOT NULL,
    city                text NOT NULL DEFAULT '',
    zipcode             text NOT NULL DEFAULT '',
    address1            text NOT NULL DEFAULT '',
    address2            text NOT NULL DEFAULT '',
    email               text NOT NULL DEFAULT '',
    phone               text NOT NULL DEFAULT '',
    legal_entity_type   text NOT NULL DEFAULT '',
    chamber_of_commerce text NOT NULL DEFAULT '',
    tax_number          text NOT NULL DEFAULT '',
    bank_account_name   text NOT NULL DEFAULT '',
    bank_account_number text NOT NULL DEFAULT '',
    bank_account_bic    text NOT NULL DEFAULT '',
    created_at          timestamptz NOT NULL,
    updated_at          timestamptz NOT NULL
);
CREATE INDEX identities_administration_id_idx ON identities (administration_id);
CREATE UNIQUE INDEX identities_one_default_idx ON identities (administration_id) WHERE is_default;

-- Existing administrations get their default identity from the company details
-- they already hold. New IDs keep the original's timestamp and sequence under
-- generator node 1023, which the application does not use, so they are unique.
INSERT INTO identities (id, administration_id, is_default, company_name, country, city, zipcode, address1, address2,
                        chamber_of_commerce, created_at, updated_at)
SELECT (id & ~(1023::bigint << 12)) | (1023::bigint << 12), id, true, name, country, city, zipcode, address1, address2,
       chamber_of_commerce, created_at, created_at
FROM administrations;

INSERT INTO resources (id, type, administration_id)
SELECT id, 'identities', administration_id FROM identities;

ALTER TABLE administrations
    DROP COLUMN address1,
    DROP COLUMN address2,
    DROP COLUMN zipcode,
    DROP COLUMN city,
    DROP COLUMN chamber_of_commerce;

-- +goose Down
ALTER TABLE administrations
    ADD COLUMN address1 text NOT NULL DEFAULT '',
    ADD COLUMN address2 text NOT NULL DEFAULT '',
    ADD COLUMN zipcode text NOT NULL DEFAULT '',
    ADD COLUMN city text NOT NULL DEFAULT '',
    ADD COLUMN chamber_of_commerce text NOT NULL DEFAULT '';

UPDATE administrations a
SET address1 = i.address1, address2 = i.address2, zipcode = i.zipcode, city = i.city, chamber_of_commerce = i.chamber_of_commerce
FROM identities i
WHERE i.administration_id = a.id AND i.is_default;

DELETE FROM resources WHERE type = 'identities';
DROP TABLE identities;
