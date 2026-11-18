-- +goose Up
-- Moneybird's audit report: a category checked for a period, by whom and
-- when, with what was booked on it then so later changes show.
CREATE TABLE ledger_account_audits (
    id                bigint PRIMARY KEY,
    administration_id bigint NOT NULL REFERENCES administrations (id) ON DELETE CASCADE,
    ledger_account_id bigint NOT NULL REFERENCES ledger_accounts (id) ON DELETE CASCADE,
    period_from       date   NOT NULL,
    period_to         date   NOT NULL,
    debit             bigint NOT NULL,
    credit            bigint NOT NULL,
    user_id           bigint NOT NULL,
    audited_at        timestamptz NOT NULL,
    UNIQUE (administration_id, ledger_account_id, period_from, period_to)
);

-- +goose Down
DROP TABLE ledger_account_audits;
