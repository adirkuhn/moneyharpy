-- +goose Up
-- subscriptions bill a contact for a product every period through their
-- recurring sales invoice; subscription_products are the products
-- subscribed to over time.
CREATE TABLE subscriptions (
    id                         bigint PRIMARY KEY,
    administration_id          bigint  NOT NULL REFERENCES administrations (id) ON DELETE CASCADE,
    contact_id                 bigint  NOT NULL REFERENCES contacts (id),
    contact_person_id          bigint  REFERENCES contact_people (id) ON DELETE SET NULL,
    recurring_sales_invoice_id bigint  REFERENCES recurring_sales_invoices (id) ON DELETE SET NULL,
    start_date                 date    NOT NULL,
    end_date                   date,
    frequency_count            integer NOT NULL,
    frequency_type             text    NOT NULL,
    reference                  text    NOT NULL DEFAULT '',
    cancelled_at               timestamptz,
    created_at                 timestamptz NOT NULL,
    updated_at                 timestamptz NOT NULL
);
CREATE INDEX subscriptions_contact_idx ON subscriptions (administration_id, contact_id);
CREATE UNIQUE INDEX subscriptions_recurring_idx ON subscriptions (recurring_sales_invoice_id) WHERE recurring_sales_invoice_id IS NOT NULL;

CREATE TABLE subscription_products (
    id              bigint PRIMARY KEY,
    subscription_id bigint  NOT NULL REFERENCES subscriptions (id) ON DELETE CASCADE,
    product_id      bigint  NOT NULL REFERENCES products (id),
    start_date      date    NOT NULL,
    end_date        date,
    amount          text    NOT NULL,
    quantity        bigint  NOT NULL,
    discount        integer NOT NULL DEFAULT 0
);
CREATE INDEX subscription_products_subscription_idx ON subscription_products (subscription_id);
CREATE INDEX subscription_products_product_idx ON subscription_products (product_id);

-- additional_charges are billed on a subscription's next invoice, or for a
-- contact on an invoice of their own once the month is over; detail_id is
-- the line that billed one.
CREATE TABLE additional_charges (
    id                bigint PRIMARY KEY,
    administration_id bigint  NOT NULL REFERENCES administrations (id) ON DELETE CASCADE,
    contact_id        bigint  REFERENCES contacts (id) ON DELETE CASCADE,
    subscription_id   bigint  REFERENCES subscriptions (id) ON DELETE CASCADE,
    product_id        bigint  NOT NULL REFERENCES products (id),
    amount            text    NOT NULL,
    quantity          bigint  NOT NULL,
    price             bigint  NOT NULL,
    period_from       date    NOT NULL,
    period_to         date    NOT NULL,
    description       text    NOT NULL,
    identifier        text    NOT NULL DEFAULT '',
    sales_invoice_id  bigint,
    detail_id         bigint,
    created_at        timestamptz NOT NULL,
    updated_at        timestamptz NOT NULL,
    CHECK ((contact_id IS NULL) <> (subscription_id IS NULL))
);
CREATE INDEX additional_charges_contact_idx ON additional_charges (contact_id) WHERE contact_id IS NOT NULL;
CREATE INDEX additional_charges_subscription_idx ON additional_charges (subscription_id) WHERE subscription_id IS NOT NULL;
CREATE INDEX additional_charges_unbilled_idx ON additional_charges (administration_id) WHERE detail_id IS NULL AND contact_id IS NOT NULL;
CREATE INDEX additional_charges_product_idx ON additional_charges (product_id);

-- subscription_one_off_invoices are invoices scheduled to be merged into a
-- subscription's next invoice.
CREATE TABLE subscription_one_off_invoices (
    sales_invoice_id  bigint PRIMARY KEY REFERENCES sales_invoices (id) ON DELETE CASCADE,
    subscription_id   bigint NOT NULL REFERENCES subscriptions (id) ON DELETE CASCADE,
    administration_id bigint NOT NULL REFERENCES administrations (id) ON DELETE CASCADE
);
CREATE INDEX subscription_one_off_invoices_subscription_idx ON subscription_one_off_invoices (subscription_id);

-- downloads are the files exports made, fetched later through a temporary
-- link.
CREATE TABLE downloads (
    id                bigint PRIMARY KEY,
    administration_id bigint  NOT NULL REFERENCES administrations (id) ON DELETE CASCADE,
    user_id           bigint,
    download_type     text    NOT NULL,
    filename          text    NOT NULL,
    content_type      text    NOT NULL,
    size              bigint  NOT NULL DEFAULT 0,
    failed            boolean NOT NULL DEFAULT false,
    downloaded        boolean NOT NULL DEFAULT false,
    created_at        timestamptz NOT NULL,
    updated_at        timestamptz NOT NULL
);
CREATE INDEX downloads_admin_idx ON downloads (administration_id, created_at DESC);

-- +goose Down
DROP TABLE downloads;
DROP TABLE subscription_one_off_invoices;
DROP TABLE additional_charges;
DROP TABLE subscription_products;
DELETE FROM resources WHERE type = 'subscriptions';
DROP TABLE subscriptions;
