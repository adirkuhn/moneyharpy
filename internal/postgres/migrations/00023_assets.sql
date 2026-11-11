-- +goose Up
-- assets are fixed assets (vaste activa), after Moneybird's assets: what was
-- bought on a balance sheet category and how its value goes down. An asset
-- without lifespan_months has no depreciation plan, as on land and
-- buildings. The disposal, when there is one, and the reinvestment reserve
-- ("herinvesteringsreserve") purchase and sale are kept on the row; what
-- the asset books is in journal_entries as document type Asset.
CREATE TABLE assets (
    id                          bigint PRIMARY KEY,
    administration_id           bigint  NOT NULL REFERENCES administrations (id) ON DELETE CASCADE,
    ledger_account_id           bigint  NOT NULL REFERENCES ledger_accounts (id),
    name                        text    NOT NULL,
    purchase_date               date    NOT NULL,
    purchase_value              bigint  NOT NULL,
    lifespan_months             integer,
    residual_value              bigint  NOT NULL DEFAULT 0,
    disposal_id                 bigint,
    disposal_date               date,
    disposal_reason             text,
    disposal_externally_booked  boolean NOT NULL DEFAULT false,
    reserve_sale_id             bigint,
    reserve_sale_amount         bigint,
    reserve_purchase_id         bigint,
    reserve_purchase_amount     bigint,
    created_at                  timestamptz NOT NULL,
    updated_at                  timestamptz NOT NULL
);
CREATE INDEX assets_admin_idx ON assets (administration_id, purchase_date);
CREATE INDEX assets_ledger_account_idx ON assets (ledger_account_id);

-- asset_value_changes are depreciations and other changes of an asset's
-- value: linear (the plan's monthly depreciation), manual, arbitrary,
-- divestment and full_depreciation. Negative amounts lower the value.
CREATE TABLE asset_value_changes (
    id                bigint PRIMARY KEY,
    administration_id bigint  NOT NULL REFERENCES administrations (id) ON DELETE CASCADE,
    asset_id          bigint  NOT NULL REFERENCES assets (id) ON DELETE CASCADE,
    type              text    NOT NULL,
    date              date    NOT NULL,
    amount            bigint  NOT NULL,
    description       text,
    externally_booked boolean NOT NULL DEFAULT false,
    created_at        timestamptz NOT NULL
);
CREATE INDEX asset_value_changes_asset_idx ON asset_value_changes (asset_id, date);

-- asset_sources link the bookings an asset was bought with: a purchase
-- invoice or receipt line (detail_id) or a journal entry row
-- (general_journal_document_entry_id).
CREATE TABLE asset_sources (
    id                                bigint PRIMARY KEY,
    administration_id                 bigint NOT NULL REFERENCES administrations (id) ON DELETE CASCADE,
    asset_id                          bigint NOT NULL REFERENCES assets (id) ON DELETE CASCADE,
    detail_id                         bigint,
    general_journal_document_entry_id bigint,
    created_at                        timestamptz NOT NULL
);
CREATE INDEX asset_sources_asset_idx ON asset_sources (asset_id);

-- +goose Down
DELETE FROM journal_entries WHERE document_type = 'Asset';
DELETE FROM resources WHERE type = 'assets';
DROP TABLE asset_sources;
DROP TABLE asset_value_changes;
DROP TABLE assets;
