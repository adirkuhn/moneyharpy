-- +goose Up
ALTER TABLE administrations
    ADD COLUMN address1 text NOT NULL DEFAULT '',
    ADD COLUMN address2 text NOT NULL DEFAULT '',
    ADD COLUMN zipcode text NOT NULL DEFAULT '',
    ADD COLUMN city text NOT NULL DEFAULT '',
    ADD COLUMN chamber_of_commerce text NOT NULL DEFAULT '';

-- +goose Down
ALTER TABLE administrations
    DROP COLUMN address1,
    DROP COLUMN address2,
    DROP COLUMN zipcode,
    DROP COLUMN city,
    DROP COLUMN chamber_of_commerce;
