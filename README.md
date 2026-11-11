# MoneyHarpy

Open source bookkeeping with an API compatible with the Moneybird v2 API.

Every entity has its own URI, so anything in the app can be linked to and opened directly.

## Run it

Requires Docker.

```sh
cp .env.example .env   # set POSTGRES_PASSWORD and SECRET_KEY
docker compose up --build
```

Open <http://localhost:8080> and register. The first user becomes the instance administrator.

Only the values in `.env` live outside the app. Everything else, such as open registration and Google login, is set under **Instance settings** and stored in the database, with secrets encrypted by `SECRET_KEY`. Changing `SECRET_KEY` makes stored secrets unreadable.

Behind one reverse proxy that appends to `X-Forwarded-For`, also set `TRUST_PROXY_HEADERS=true`, so rate limits apply per client.

### Google login

Create an OAuth client in Google Cloud with the redirect URI `<BASE_URL>/api/auth/google/callback`, then enter its ID and secret under **Instance settings**.

### LM2 login

In LM2, add this installation to `APP_REDIRECT_ALLOWLIST`, for example `moneyharpy=https://books.example.com`. Then enter the LM2 address and that app name under **Instance settings**.

MoneyHarpy never holds LM2's signing key: it asks LM2 who a token belongs to. A LM2 user whose email matches an existing MoneyHarpy account is refused rather than linked, because LM2 does not confirm addresses.

### Email

Enter your SMTP server under **Instance settings**, then use **Send test email** to confirm it works. Password recovery appears on the login page once email is enabled.

### File storage

Uploads, such as logos and stationery on document styles, work out of the box: without further setup they are kept on the server's disk, in the `uploads-data` volume (`UPLOADS_DIR`, default `/data/uploads`). Back that volume up with the database. For larger installations, use S3-compatible storage instead: Amazon S3, Cloudflare R2, Backblaze B2, MinIO, SeaweedFS or any other S3 service. The bundled compose file runs SeaweedFS. To use it, set `S3_ACCESS_KEY` and `S3_SECRET_KEY` in `.env`. Then enter them under **Instance settings > File storage** with endpoint `http://storage:8333`, any bucket name and path-style addresses on. **Test storage** creates the bucket if it is missing. Files uploaded before S3 was set up stay readable from disk.

### Company search and address lookup

Both are optional and off by default, and the app works without them. Under **Instance settings**:

- **Company search** needs an API key from the [KVK developer portal](https://developers.kvk.nl/) and searches the Dutch company register while adding an administration.
- **Address lookup** completes Dutch addresses from zipcode and house number. It sends those two values to PDOK, the Dutch government address service.

## API

The contract is [`api/openapi.yaml`](api/openapi.yaml). A running instance renders it at `/docs/api/`.

```sh
curl -H "Authorization: Bearer $TOKEN" http://localhost:8080/api/v2/administrations.json
```

Create tokens under **User account**.

The schemas follow Moneybird's published API description. `go run ./cmd/mbschemas` copies their structure (field names, types and enums, without Moneybird's text) into the contract. The *Moneybird compatibility* workflow reports when Moneybird's API changes.

## URIs

| What | URI |
|---|---|
| Administration | `/{administration_id}` |
| Resource in an administration | `/{administration_id}/{type}/{id}` |
| Any resource, by ID alone | `/r/{id}` |

## Development

| Path | Contents |
|---|---|
| `api/` | OpenAPI contract, the source of truth for all HTTP communication |
| `cmd/moneyharpy/` | Entry point and wiring |
| `internal/administration`, `identity`, `settings`, `resource` | Business rules, free of HTTP and SQL |
| `internal/httpapi`, `postgres`, `google` | Adapters that depend on the rules, never the reverse |
| `web/` | React application |
| `e2e/` | Playwright tests |

```sh
make generate   # after changing api/openapi.yaml or SQL queries
make test       # backend, needs Docker
make test-web   # frontend, run npm ci in web/ first
make test-e2e   # whole stack, run npm ci in e2e/ first
```

Changes are test-driven. Go code follows the [Google Go style guide](https://google.github.io/styleguide/go/guide).

To change the API, edit the contract first. Requests are validated against it at runtime, and backend tests fail on any response it does not describe.

## Overview

Opening an administration lands on its overview at `/{id}/feed`, and the
logo leads back to it: to the open administration, or else the one opened
last in this browser. `GET /api/v2/{administration_id}/overview` (a
MoneyHarpy extension) returns one block per part of the books the member,
and a connection token, may see, plus the records processed last. Today the
bank block holds the balance and the transactions left to link; the boxes for
sales invoices and incoming documents are in place and fill in once those
parts exist.

## Contacts

Customers and suppliers live at `/{id}/contacts`, with Moneybird's contacts
API under `/api/v2/{administration_id}/contacts`: create, update, archive and
delete, search and paged lists (`page`, `per_page`, a `Link` header and
`X-Total-Count`), the `filter` grammar, synchronization, lookup by customer
id, likely duplicates, contact people and notes or to-dos. A new contact
without a customer id gets the next number. Delivery by Peppol or
Simplerinvoicing needs the contact's e-invoicing identifier, and a direct
debit mandate needs its IBAN, holder, reference and date. Members with the
sales invoices, documents or estimates permission work with contacts. A
contact that documents refer to cannot be deleted; archive it instead.

## Journal

Everything that changes the books posts to one journal in double entry:
debits positive, credits negative, every document's entries adding up to
zero. Bank transactions post through a view, so they never run out of step:
the whole amount on the account's category, each booking against its
category, and the part not booked yet against unprocessed transactions.
Journal entries made by hand (Moneybird's general journal documents, under
`/api/v2/{administration_id}/documents/general_journal_documents`) post their
rows, refuse the debtors and creditors that only invoices settle, and keep
out of the locked period. `GET /reports/general_ledger` and
`GET /reports/journal_entries` read the whole journal, as do the trial
balance and a category's bookings under Reports. Notes and attachments are
kept per record, so invoices reuse them.

## Sales invoices

Invoices live at `/{id}/sales_invoices`, with Moneybird's sales invoices API
under `/api/v2/{administration_id}/sales_invoices`. A draft follows the
default workflow, document style and identity; VAT is worked out per rate on
the summed lines, as Moneybird's `tax_totals` show it. Sending reserves the
next number (one database lock, so two invoices never share one), sets the
due date and books the invoice: the total on the debtors, each line on its
category (or uncategorized income), the VAT per rate on payable VAT. It then
emails the PDF (and a UBL 2.1 e-invoice when the contact or settings ask for
it) with the workflow's email text, or leaves sending to you. A sent invoice
can change until it is paid and is booked again; it is credited rather than
deleted. Peppol delivery is not available yet.

**Payments.** A payment registered by hand (`POST .../payments`, or the older
`register_payment`) posts the contra account against the debtors on its
date: received payments to be classified for a bank transfer or a payment
without proof, cash, private deposits, rounding, a balance sheet category
(`balance_settlement`) or settlements (`invoices_settlement`, which pays the
other invoice too). Once the payments cover the total the invoice is paid;
deleting one opens it again. `register_payment_creditinvoice` settles a
credit invoice with the invoice it credits.

**Bank linking.** A bank transaction pays an invoice with `link_booking`
(`SalesInvoice`) or a payment with `financial_mutation_id`; it pays what both
have open unless an amount is given, and takes over a payment registered by
hand that waits for it. The transaction posts the payment (bank against the
debtors, in the `bank_journal_entries` view), lists it in `payments` and
counts it in its open amount. `unlink_booking` (`Payment`) or deleting the
payment undoes it. The locked period applies to the transaction's date.
Linking from the invoice's side also needs the bank permission.

**Dubious and uncollectible.** `mark_as_dubious` reserves what is open,
excluding VAT, in the provision for doubtful debtors; a later payment of the
rest releases it. `mark_as_uncollectible` writes off what is open for good:
excluding VAT on uncollectible revenue (or the provision), and its VAT, split
over the invoice's rates, back off payable VAT. Both are posted with the
invoice on their own date, after the invoice date and not in the future.

**Reminders and the background job.** The workflow's steps after the payment
term are reminders, each due its delay after the previous one; invoices show
the next as `next_reminder`. `send_reminders` emails one reminder per contact,
workflow, style and identity, with the invoices listed (`{invoices}` in the
text) and their PDFs attached, or marks them reminded when sent by hand.
Every minute a background job sends scheduled invoices whose date has come,
marks overdue open invoices late, and sends the reminders whose step sends
automatically, acting as the administration's owner. An advisory lock per
administration keeps two instances from doing the same work twice. The
overview shows what is still to receive and to remind.

## External sales invoices

Invoices made in other software, such as a webshop or a till, live at
`/{id}/external_sales_invoices`, with Moneybird's external sales invoices API
under `/api/v2/{administration_id}/external_sales_invoices`. MoneyHarpy does
not number, render or send them; it books them and follows their payment.
Dropping files on the list (or `POST .../attachment`) makes a new invoice of
each, dated today and holding the file, to fill in. An invoice with a contact
and at least one line is booked when it is saved: the total on the debtors,
each line on its category (or uncategorized income), the VAT per rate on
payable VAT, with document type `ExternalSalesInvoice`. Its `reference` is
the number the other software gave it; `entry_number` counts the
administration's external invoices. It can change until it has payments or is
marked dubious, and not in the locked period.

Payments, bank linking (`link_booking` with `ExternalSalesInvoice`, or a
payment with `financial_mutation_id`, which needs the bank permission too),
`mark_as_dubious` and `mark_as_uncollectible` work as for sales invoices,
and so does the background job that marks overdue invoices late. Deleting an
invoice unlinks its bank transactions first and removes what it and its
payments post. The overview counts open external invoices as to receive, and
the transaction page offers them for linking. The email-in address that
Moneybird offers for forwarding invoices is not available yet.

## Users and permissions

Each user of an administration is an owner, employee or accountant, with
Moneybird's permissions: `sales_invoices`, `documents`, `estimates`, `bank`,
`settings`, `time_entries` and `ownership`. Every operation in
`api/openapi.yaml` declares the permissions it needs in `x-permissions`
(one of them is enough), and the server refuses to start if an operation on
an administration declares none. Owners invite people under Settings →
Management; without email, the invitation link is shown to share by hand.

Users can turn on two-step verification with an authenticator app, and
owners can require it for an administration. Owners can also give this
installation's admins time-limited support access, and delete the
administration with all its data.

## Integrations

Settings → Integrations connects other software to one administration:

- **API tokens** (`mhc_…`) act for the user who made them, in that
  administration only, limited to Moneybird's scopes and, if chosen, to
  reading. Owners can always make them; other users when "users may create
  connections" is on, and never with more than their own rights.
- **AI connections** use the same tokens over MCP at `/api/mcp` (streamable
  HTTP, stateless JSON). The tools are the API operations the connection
  may use, run through the same checks as any API call.
- **Webhooks** follow Moneybird: subscribe to events or groups, receive
  `{administration_id, webhook_id, webhook_token, entity_type, entity_id,
  state, action, entity}` signed in `Moneybird-Signature: t=…,v1=…`
  (HMAC-SHA256 of `t.body`), retried 10 times. Webhooks need public HTTPS
  addresses; an instance setting allows plain HTTP and local addresses for
  development.

Payment services (Mollie, PayPal, SEPA direct debit, Online incasso) wait
for invoices.

## Bank

Transactions come in from statement files: CAMT.052, CAMT.053, CAMT.054 or
MT940, downloaded from online banking and imported on an account under
Settings → Bank. Transactions an account has already are skipped, so a file
can be imported twice. Cash and other accounts without files take
transactions typed in by hand. Booking rules book matching transactions on a
category as they come in; the rest are booked under Bank. The API follows
Moneybird's financial accounts, statements and mutations. Direct bank
connections are not built yet; `internal/bankfile` is where another source
would hand over its transactions.

## Bookkeeping per country

A new administration starts from its country's VAT rates, categories and
chart scheme; the Netherlands comes with the Referentie Grootboekschema (RGS)
3.8. See [docs/countries.md](docs/countries.md) to add a country.

## License

[MIT](LICENSE)
