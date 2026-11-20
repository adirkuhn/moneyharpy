-- +goose Up
-- moneybird_import_problems are the records an import could not bring in,
-- or brought in without part of them, each with why: all of them, so they
-- can be gone through. One that comes in later is removed.
CREATE TABLE moneybird_import_problems (
    administration_id bigint      NOT NULL REFERENCES administrations (id) ON DELETE CASCADE,
    step              text        NOT NULL,
    -- kind and source_id name the Moneybird record; source_id is 0 when it
    -- had none that could be read.
    kind              text        NOT NULL,
    source_id         bigint      NOT NULL,
    message           text        NOT NULL,
    -- partial is a record that came in without part of it, such as a file.
    partial           boolean     NOT NULL DEFAULT false,
    created_at        timestamptz NOT NULL,
    PRIMARY KEY (administration_id, kind, source_id, message)
);
CREATE INDEX moneybird_import_problems_record_idx ON moneybird_import_problems (administration_id, kind, source_id);

-- +goose Down
DROP TABLE moneybird_import_problems;
