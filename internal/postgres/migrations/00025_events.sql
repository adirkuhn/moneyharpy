-- +goose Up
-- events are what happened to a record, after Moneybird's events: who did
-- what, with the record it links to and details in data.
CREATE TABLE events (
    id                bigint PRIMARY KEY,
    administration_id bigint NOT NULL REFERENCES administrations (id) ON DELETE CASCADE,
    entity_type       text   NOT NULL,
    entity_id         bigint NOT NULL,
    user_id           bigint REFERENCES users (id) ON DELETE SET NULL,
    action            text   NOT NULL,
    link_entity_type  text,
    link_entity_id    bigint,
    data              jsonb  NOT NULL DEFAULT '{}',
    created_at        timestamptz NOT NULL,
    updated_at        timestamptz NOT NULL
);
CREATE INDEX events_entity_idx ON events (administration_id, entity_id);

-- +goose Down
DROP TABLE events;
