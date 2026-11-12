-- +goose Up
-- A document style also styles the emails, public views and online sales
-- checkout of its documents (MoneyHarpy extensions after Moneybird's editor).
-- channels holds those settings; files the images beyond the logo and
-- background, by kind, and the images custom CSS uses. Missing keys take
-- the defaults, so existing styles keep their look.
ALTER TABLE document_styles
    ADD COLUMN channels jsonb NOT NULL DEFAULT '{}',
    ADD COLUMN files    jsonb NOT NULL DEFAULT '{}';

-- +goose Down
ALTER TABLE document_styles DROP COLUMN files, DROP COLUMN channels;
