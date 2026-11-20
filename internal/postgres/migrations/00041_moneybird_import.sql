-- +goose Up
-- moneybird_imports is the import of a Moneybird administration, one per
-- administration here. It runs in the background: a worker leases it by
-- moving next_attempt_at ahead and saves its progress per step.
CREATE TABLE moneybird_imports (
    administration_id        bigint PRIMARY KEY REFERENCES administrations (id) ON DELETE CASCADE,
    -- user_id is the owner who started it; the import acts with their access.
    user_id                  bigint      NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    source_administration_id text        NOT NULL,
    source_name              text        NOT NULL,
    -- state is running, done or failed.
    state                    text        NOT NULL,
    -- steps are the progress per kind: [{"kind": ..., "total": ..., ...}].
    steps                    jsonb       NOT NULL DEFAULT '[]',
    -- waiting_until is when Moneybird's rate limit lets the import go on.
    waiting_until            timestamptz,
    error                    text        NOT NULL DEFAULT '',
    -- token is the sealed Moneybird API token, cleared once the import ends.
    token                    text        NOT NULL DEFAULT '',
    attempts                 integer     NOT NULL DEFAULT 0,
    next_attempt_at          timestamptz,
    started_at               timestamptz NOT NULL,
    finished_at              timestamptz,
    updated_at               timestamptz NOT NULL
);
CREATE INDEX moneybird_imports_due_idx ON moneybird_imports (next_attempt_at) WHERE state = 'running';

-- moneybird_import_ids remember what an import brought in: the Moneybird id
-- of each record and its id here, which differs only when a default
-- category or VAT rate took its place. A second import skips what is here.
CREATE TABLE moneybird_import_ids (
    administration_id bigint NOT NULL REFERENCES administrations (id) ON DELETE CASCADE,
    kind              text   NOT NULL,
    source_id         bigint NOT NULL,
    target_id         bigint NOT NULL,
    PRIMARY KEY (administration_id, kind, source_id)
);

-- +goose Down
DROP TABLE moneybird_import_ids;
DROP TABLE moneybird_imports;
