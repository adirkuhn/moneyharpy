# MoneyHarpy

Open source bookkeeping with an API compatible with the Moneybird v2 API.

Every entity has its own URI, so anything in the app can be linked to and opened directly.

Documentation: <https://adirkuhn.github.io/moneyharpy/> · API reference: <https://adirkuhn.github.io/moneyharpy/api/>

## Run it

Requires Docker.

```sh
cp .env.example .env   # set POSTGRES_PASSWORD and SECRET_KEY
docker compose pull app && docker compose up -d
```

This runs the published image `ghcr.io/adirkuhn/moneyharpy` for amd64 and arm64. Tags:

| Tag | What |
|---|---|
| `latest` | The newest release |
| `1.2.3`, `1.2`, `1` | A release, or the newest one in that line |
| `main` | The newest commit on main |
| `sha-<commit>` | One commit on main |

Set `MONEYHARPY_IMAGE` in `.env` to use another tag, such as `ghcr.io/adirkuhn/moneyharpy:1.2`. To build from source instead, run `docker compose up --build`.

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

Downloads (`download_pdf`, `download_packing_slip_pdf` and attachments)
answer `302 Found`, as at Moneybird, with `Location` set to
`/api/files/{token}`: a link that works for 30 seconds without signing in.
The token names the file and the user who asked for it, and is signed with a
key derived from `SECRET_KEY`, so every instance accepts the others' links;
the file is read as that user when the link is followed. `media=stationery`
leaves the sender's address and logo off the PDF. Moneybird answers `402`
when a plan's limit is reached and `503` when finding duplicate contacts
takes too long; the contract declares both, but MoneyHarpy has no plans and
groups contacts in one pass, so it answers neither.

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
make docs       # documentation site into build/site, needs pip install -r docs/requirements.txt
```

Changes are test-driven. Go code follows the [Google Go style guide](https://google.github.io/styleguide/go/guide).

To change the API, edit the contract first. Requests are validated against it at runtime, and backend tests fail on any response it does not describe.

### Releases

Every push to main and every `v*` tag, once CI passes, publishes the image to `ghcr.io/adirkuhn/moneyharpy` and the documentation to GitHub Pages. To release, tag main:

```sh
git tag v1.2.3 && git push origin v1.2.3
```

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
sales invoices, documents or estimates permission work with contacts.
Deleting a contact that documents refer to archives it instead, as Moneybird
does, answering `X-Kept-As: archived`.

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
kept per record, so invoices reuse them. Journal entry rows can be booked on
a project (`project_id`); `new_filter` is accepted on the list and used
instead of `filter`. Deleting one answers `200` with the status as its body,
as at Moneybird.

## Reports

Moneybird's reports under `/api/v2/{administration_id}/reports` are worked
out from the journal: `profit_loss` (optionally per `project_id`),
`balance_sheet`, `cash_flow` (optionally per `financial_account_id`), `tax`,
`working_capital`, `debtors`, `creditors`, `revenue_by_contact`,
`expenses_by_contact`, `revenue_by_project`, `expenses_by_project` and the
deprecated `ledger_accounts/{id}`. They take a `period` of whole months (at
most 12; `Period is invalid` and `Period cannot exceed 12 months` answer
`400`) and a `granularity` that adds `periods` per month, quarter or year.
`debtors_aging` and `creditors_aging` age what open invoices and receipts
had open at the end of `period_until` (today for a month still running) by
their document date; `subscriptions` gives the monthly recurring revenue,
annual run rate, churn and lifetime value of the period's last month.
Operations that Moneybird gives several scopes need all of them. MoneyHarpy
closes no years yet, so the balance sheet shows the result of every year as
an open year.

`POST /reports/export/auditfile` (XAF 3.2, also when 4.0 is asked),
`/reports/export/brugstaat` (RGS brugstaat as XML) and
`/reports/export/ledger_accounts` (ledger cards as an Excel workbook) make
the year's file at once and add it to the downloads, where
`POST /downloads/{id}/download` serves it; they answer `204`. The web app
shows the profit and loss statement, balance sheet, VAT summary, cash flow,
receivable and payable overviews, their aging and revenue and costs per
contact under Reports.

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

## Recurring sales invoices

Recurring invoices live at `/{id}/recurring_sales_invoices`, with Moneybird's
recurring sales invoices API under
`/api/v2/{administration_id}/recurring_sales_invoices`. A recurring invoice is
a template: a contact, invoice settings and lines like a sales invoice's, and
a schedule. It makes its first invoice on `invoice_date` (today unless given,
never in the past) and one every `frequency` days, weeks, months, quarters or
years (`frequency_type`) after it. Months, quarters and years keep the first
day, or the month's last day when the month is shorter: 31 January, 28
February, 31 March. It ends after `last_date` (MoneyHarpy extension) or, as
at Moneybird, after `desired_count` invoices with `has_desired_count`; the
response shows the end as `last_date`. A new `invoice_date`, `frequency` or
`frequency_type` restarts the schedule from the next invoice date. "Make
recurring" on a draft invoice copies it into a new recurring invoice.

Every minute the background job, under the same per-administration lock as
the other sales work, makes the invoices that are due: a draft sales invoice
with `recurring_sales_invoice_id` set, dated on the schedule, with `{period}`
in line descriptions replaced by the period it covers (lines with a `period`
get that period). Each invoice is stored in one transaction with the
recurring invoice moved on to its next date, so a round never makes an
invoice twice, and a job that was down catches up on every date it missed.
With `auto_send` the invoice is then sent the way the contact receives
invoices, acting as the administration's owner; when that fails it stays a
draft and the error is logged. `deactivate` stops a recurring invoice for
good; `DELETE` removes one that made no invoices and deactivates the others
(answering `X-Kept-As: deactivated`). Deleting a contact with recurring invoices
archives it instead. Webhooks tell `recurring_sales_invoice_created`, `_updated`,
`_deactivated`, `_destroyed` and `_invoice_created`. Subscriptions (below)
bill through recurring invoices; merging invoices (`mergeable`, accepted and
kept) is not available yet.

## Subscriptions

Moneybird's subscriptions API lives under
`/api/v2/{administration_id}/subscriptions` (there is no page for it in the
app yet). A subscription bills a contact for a product with a frequency and a
VAT rate: creating one makes a recurring sales invoice
(`recurring_sales_invoice_id`) with the product as its line, the `amount`
and the `discount`, which bills each period in advance and sends its invoices
itself. The first invoice goes out on `start_date`, but no sooner than
tomorrow. A `frequency` and `frequency_type` other than the product's must fit
it: a monthly product can be billed every quarter or year, at the price times
the months. `PATCH` with a new `product_id`, `amount` or `discount` starts a
new entry in `subscription_products` from `start_date` (today unless given);
when that falls within a period already billed, the price difference for the
rest of the period goes on a one-off invoice that is merged into the next
subscription invoice. `DELETE` cancels on `end_date` (today unless given):
the recurring invoice makes no invoice from then on. A cancelled subscription
answers `400` to further changes.

Additional charges (`POST .../subscriptions/{id}/additional_charges`) are added
as lines to the subscription's next invoice when the background job makes it.
A contact's charges (`/contacts/{id}/additional_charges`) are billed once the
month they were added in is over: one invoice per contact, dated the first of
the month and sent at once, by a background job with its own lock. Periods
such as `20250101..20250131` or `this_month` are kept as whole months
(`202501..202501`); `include_billed=true` lists billed charges too, with the
line that billed them (`detail_id`). `create_and_schedule_one_off_sales_invoice`
schedules an invoice for the next subscription invoice date (or
`invoice_date`); when the subscription's invoice is made on or after it, its
lines are merged in and the scheduled invoice is removed. Webhooks tell
`subscription_created`, `_edited` (reference, contact person and the like),
`_updated` (product, amount or discount), `_cancelled` and
`sales_invoice_created_based_on_subscription`.

What needs Moneybird Payments answers as Moneybird does when it is not
enabled: `subscription_templates` is an empty list and its `sales_link` and
`checkout_identifier` answer `404` for the template; the contact's
`moneybird_payments_mandate` answers `404` (no mandate) to `GET` and `403` to
the rest, with the status as body. The customer contact portal links open the
contact, or its sales invoices, in MoneyHarpy for its users, since there is no
portal for customers; a subscription link answers `404`, as Moneybird does
for subscriptions without a template. `verifications` answers `{}`: nothing
is verified yet.

`/api/v2/{administration_id}/downloads` lists the files exports made (25 a
page, filters `download_type`, `downloaded` and `failed`), only of the types
the user may open; `POST .../downloads/{id}/download` redirects to a link
valid for 30 seconds and marks it downloaded, or answers `401` without the
type's permission. Exports register their file with `downloads.Service.Add`
(and new types with `downloads.RegisterType`).

## Estimates

Quotes live at `/{id}/estimates`, with Moneybird's estimates API under
`/api/v2/{administration_id}/estimates`: list (filter `state`, all unless
given, several separated by `|`; `period` on the estimate date, this year when
no filter is given; `contact_id`; `workflow_id`), CRUD, synchronization,
`find_by_estimate_id`, notes and attachments. An estimate has lines, VAT
totals, products and projects like a sales invoice, and follows the contact's
estimate workflow or the default one, whose first step is its validity.

`send_estimate` gives a draft its number from the estimate numbering in the
invoice settings, its date (today unless set) and due date, and makes it
`open`; it goes out by `Email` with its PDF (the workflow's email text or
MoneyHarpy's, copied to the estimate BCC address) or is marked sent with
`Manual`. Email needs a sender address on the identity. A sent estimate goes
out again in its state. An open estimate past its due date shows as `late`.
`change_state` only changes the state: a draft leaves draft only by being
sent; open and late go to accepted, rejected, billed or archived; accepted to
open, rejected, billed or archived; rejected to open, accepted or archived;
billed to open, accepted or archived; archived back to open, accepted,
rejected or billed. Other changes answer 400 with Moneybird's symbolic error,
such as `can_not_change_draft_to_open`. Billed and archived estimates do not
change.

`bill_estimate` (which needs the sales invoices permission too) makes a draft
sales invoice of an open, late or accepted estimate, with the estimate's
lines, language, currency and prices, the default invoice workflow and
`original_estimate_id` naming the estimate, and marks the estimate `billed`.
`download_pdf` renders it as a quote, with its texts above and below the
lines. MoneyHarpy has no online signing yet: `download_signed_pdf` answers 422
for every estimate, and `sign_online` is kept but not used. Deleting an
estimate keeps invoices billed from it; a contact with estimates is archived
instead of deleted. Webhooks tell `estimate_created`,
`estimate_created_from_original_invoice`, `_updated`, `_destroyed`,
`_send_email`, `_send_manually`, `_mark_<state>`, `_billed` and
`sales_invoice_created_based_on_estimate`.

## Products

Products live at `/{id}/products`, with Moneybird's products API under
`/api/v2/{administration_id}/products`: create, update, delete, paged lists
(10 a page; `query` searches the title, description and identifier, with
`currency`, `active`, `for_checkout` and `ledger_account_id`) and lookup by
`identifier` (unique per administration). A product needs a price, a
category and a title or description; it can carry a VAT rate and how often it
is billed (`frequency` and `frequency_type`, or `frequency_preset` such as
`3:month`). A sales, recurring or external invoice line with a `product_id`
takes the product's description, price, VAT rate and category unless it
sends its own. Deleting a product that lines name deactivates it instead
(`X-Kept-As: deactivated`); `active` (MoneyHarpy extension) deactivates or
reactivates one. Writes need the settings permission; members who work on
invoices or estimates may read products to pick them. Moneybird's online
sales links need Moneybird Payments: `POST .../sales_link` checks the product
and contact and then answers `404` "Product not valid for online sales", as
Moneybird does for a product that cannot be sold online. Webhooks tell
`product_created`, `_updated`, `_activated`, `_deactivated` and
`_destroyed`.

## Projects

Projects live at `/{id}/projects`, with Moneybird's projects API under
`/api/v2/{administration_id}/projects`: a unique name and an optional budget
in hours, listed by name, 25 a page, active ones unless `filter` says
`state:archived`, `state:all` or `state:active|archived`. Invoice lines
(sales, recurring and external) and journal entry rows can be booked on an
active project with `project_id`; the posted journal entries keep it.
Deleting a project something is booked on archives it instead
(`X-Kept-As: archived`); `state` (MoneyHarpy extension) archives or
reactivates one. Webhooks tell `project_created`, `_updated`, `_activated`,
`_archived` and `_destroyed`.

## Time entries and task lists

Time lives at `/{id}/time_entries`, with Moneybird's time entries API under
`/api/v2/{administration_id}/time_entries`: a description, a start and an
end (rounded down to whole minutes, at least a minute apart), a pause in
seconds, billable or not, for a member (`user_id`) and optionally a contact
and a project. Without `ended_at` the entry is a running timer; a member has
one at a time, so starting or resuming one stops the other. `PATCH .../stop`
ends a timer and `PATCH .../resume` restarts one that started today, counting
the stop as a pause. Lists are newest first, 25 a page, filtered on `state`
(`all`, `open` to invoice, `non_billable`), `period` (this year unless any
filter is sent), `contact_id` with `include_nil_contacts`, `project_id`,
`user_id`, `day` and `include_active` (running timers are left out unless
asked), and `query` searches the description. Members without the sales
invoices permission see and change only their own time, as Moneybird asks
that permission for other users' entries. A sales invoice created or updated
with `time_entry_ids` bills those entries: they must have stopped and be on
no other invoice, they show under the invoice's `time_entries`, and they can
no longer be changed or deleted; deleting the draft frees them. Contacts and
projects with time are archived instead of deleted. Notes work as on
invoices. Webhooks tell `time_entry_created`, `_updated` (also on stop and
resume), `_destroyed` and `time_entry_sales_invoice_created`.

Task lists live at `/{id}/task_lists/lists`, with Moneybird's task list API
(`task_lists`, `task_list_groups`, `task_list_tasks`, `task_list_templates`).
A list has groups of tasks, tasks have subtasks; the first group has no name,
holds the tasks outside any group and cannot be changed or deleted. A new
list covers last month unless a period is given. Templates have a `period`
(ISO 8601 such as `P1M`, or `monthly`, `quarterly`, `yearly`, `none`) and a
`deadline_after`; a list made from one copies its groups and tasks, covers
the whole period before the current one and gets the deadline that long after
it. A group with a `report_type` gets its tasks generated from the report: a
task per main category of the balance sheet or of profit and loss (debtor,
creditor and asset reports have no lines yet). A task with a category
(`related_entity_id`) and `generate_subtasks` gets a subtask per
sub-category. Generated tasks cannot be deleted and generated groups take no
other tasks. Tasks are assigned to members, completed with their subtasks
and reopened with the tasks above them, and take notes. Moneybird asks for
every permission for task lists, and so does MoneyHarpy. Webhooks tell
`task_lists_list_created`, `_updated`, `_destroyed`, `_completed` and
`_reopened`, `task_lists_list_template_created`, `_updated` and
`_destroyed`, and `task_lists_task_created`, `_updated`, `_name`,
`_report_type_linked`, `_category_linked`, `_unlinked`, `_assigned`,
`_unassigned`, `_completed`, `_reopened` and `_destroyed`.

## Incoming documents

Purchase invoices and receipts live at `/{id}/documents/purchase_invoices`
and `/{id}/documents/receipts` ("Incoming" in the menu), with Moneybird's
documents API under `/api/v2/{administration_id}/documents/purchase_invoices`
and `.../documents/receipts`: lists (filter terms `period`, `state` with
`new`, `open`, `late`, `pending_payment` and `paid`, `reference`,
`contact_id`, `ledger_account_id`, `updated_after`, `attachment`; `new_filter`
too), synchronization, create, update, delete, notes, attachments, payments
and `register_payment`. A purchase invoice with a supplier and at least one
line, or a receipt with a line, is booked when saved, with its kind as
document type: the total on the creditors for the supplier, each line
excluding VAT on its category (expenses still to categorize when left out),
the VAT per rate on receivable VAT. Reverse-charged rates (purchase rates
that do not show VAT, such as "Within the EU 21%") leave the VAT out of the
total and book it on both receivable and payable VAT. `revenue_invoice`
books the other way. Fiscal allocations move their percentage of what the
lines book on a category, VAT included, to a private category. A document
without what booking needs is new and books nothing. A receipt created with
`payment.financial_account_id` is paid at once from that account. Payments
work as for sales invoices, against payments to classify unless the manual
payment action says otherwise, with `invoice_type` `Document`. An invoices
settlement (`invoice_id`) settles a document against another purchase
invoice or receipt, such as a credit purchase invoice: a payment on each,
of opposite signs, against the settlements category; removing one half
removes the other. A paid document, or one with payments, cannot change;
nothing changes in the locked period.

Bank transactions pay purchase invoices and receipts with `link_booking`
and `booking_type` `Document` (or a payment with `financial_mutation_id`):
the bank posts the payment on the creditors, and `unlink_booking` with the
payment undoes it. `NewPurchaseInvoice` and `NewReceipt` make a document for
(part of) a transaction and pay it with it: dated the transaction's day,
from the contact with the transaction's IBAN (a purchase invoice needs one),
one line of the amount including VAT at the 0% purchase rate on expenses
still to categorize, on the link's `project_id`. `PurchaseTransaction` and
`PurchaseTransactionBatch` pay the documents the purchase transaction, or
each transaction of the batch, pays, which makes them paid. `project_id` on
a `LedgerAccount` booking puts it on the project in the journal, so revenue
and expenses by project count it. General documents (`/documents/general_documents`) are
kept for the record and book nothing; typeless documents
(`/documents/typeless_documents`, "Uploads" in the menu) are uploads still
to process. The overview shows what open purchase invoices and receipts
have unpaid and how many uploads and new documents wait.

Purchase transactions (`/api/v2/{administration_id}/purchase_transactions`)
are the credit transfers that pay documents. `POST .../purchase_transactions`
and `POST .../purchase_transaction_batches` (MoneyHarpy extensions) make one
for what a document has unpaid, to the supplier's SEPA IBAN, and batch the
open ones of a bank account; the documents are pending payment from then on.
`POST .../purchase_transaction_batches/{id}/download` renders the batch as a
SEPA credit transfer file (pain.001.001.09, as Moneybird makes it), and
`POST .../sepa_credit_transfer` reads such a file (version .09 or older, such
as .03) into batches, paying the open documents whose reference or id the
transfers name. A transaction is paid once its document is. Webhooks tell
`document_saved`, `document_updated`, `document_destroyed`,
`purchase_transaction_created`, `purchase_transaction_deleted` and
`purchase_transaction_batch_created`, with the payment events. Documents
saved from email or Peppol, recurring documents, settling a document against
a sales invoice and paying through Moneybird Banking are not available yet.

## Assets

Fixed assets live at `/{id}/assets` ("Assets" in the menu), with Moneybird's
assets API under `/api/v2/{administration_id}/assets`: a paged list (active
ones, without a disposal, unless `active=false`; `ledger_account_id`),
create, update, delete, sources, value changes, disposals and the
reinvestment reserve, plus the assets report at `/reports/assets`. An asset
sits on a fixed asset category (non-current assets) that is not a
sub-category. It has a linear depreciation plan (lifespan in years, rounded
to months, and a residual value), except on land and buildings (chart scheme
codes `BMvaTer` and `BMvaBeg`), which never depreciate. The monthly
depreciation spreads what is left above the residual value over the months
left, from the purchase or the last arbitrary value change; the plan's last
month takes the rest. Creating an asset books nothing: sources link the
purchase invoice or receipt lines (`detail_id`) or journal entry rows
(`general_journal_document_entry_id`) on its category that bought it. Once it
has value changes, a disposal or a reinvestment reserve purchase, only its
name can change.

What an asset books follows from its state, as one journal document of type
`Asset`: each value change not booked externally on the category's
accumulated depreciation sub-category (added when missing, with the scheme's
`Cae` code) against depreciation costs, or against the book result for a
divestment; a disposal clears the accumulated depreciation against the
asset's category; a reinvestment reserve ("herinvesteringsreserve") purchase
books the reserve against the asset's category on the purchase date,
lowering the calculated purchase value the plan depreciates; a reinvestment
reserve sale books the book result against the reserve on the disposal date.
The Dutch seed has the system categories depreciation costs (`WAfsAmv`),
book result on fixed assets (`WAfsRvm`), the reinvestment reserve
(`BEivFirHer`) and the equipment's accumulated depreciation (`BMvaBeiCae`).
Nothing booked in the locked period changes, and an asset with bookings
there is not deleted.

Value changes: `manual` and `arbitrary` (an arbitrary one restarts the plan
and removes the linear ones after it), `divestment` and `full_depreciation`
(the value on the day to zero, disposing of the asset), and
`retroactive_linear_value_changes`, which books the plan's missing months up
to today (at once; Moneybird does it in the background). A background job
books each month's linear value change at its end, from the month the asset
was added in; it runs per administration under a lock and skips months
already booked. `POST .../disposals` needs the value on the day to be zero. A
reinvestment reserve purchase is at most the purchase value less the residual
value and what the reserve holds (sales less purchases). The assets report
gives per asset its value at the start of the period, investment,
depreciation, divestment and value at the end, for at most 12 months.
Webhooks tell Moneybird's `company_assets_*` events: asset created, updated
and destroyed, sources, disposals, the value changes by type (linear ones
booked or removed by MoneyHarpy too) and the plan's created, updated and
deactivated.

## Payments

`GET /api/v2/{administration_id}/payments/{id}` returns any payment of a
sales or external sales invoice, purchase invoice or receipt, as Moneybird's payments endpoint does for
the invoice types MoneyHarpy has.

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
Moneybird's financial accounts, statements and mutations; synchronization
takes the list's `filter` (this year unless given), and the bank balance
report gives the balances at the end of a `period` of up to 12 whole months
(this month unless given) and, with `granularity`, at the end of every
month, quarter or year in it (`periods`). Direct bank
connections are not built yet; `internal/bankfile` is where another source
would hand over its transactions.

## Bookkeeping per country

A new administration starts from its country's VAT rates, categories and
chart scheme; the Netherlands comes with the Referentie Grootboekschema (RGS)
3.8. See [docs/countries.md](docs/countries.md) to add a country.

## To do

Everything still open, in one place. The API matches Moneybird's published
description operation by operation (all 303 operations; request, response,
status codes and parameters), so most of what remains is behaviour behind
those operations, and screens.

### Decisions to make

- [ ] **Purchase invoice made from a bank transaction** (`link_booking`
  `NewPurchaseInvoice` / `NewReceipt`): Moneybird does not document how it is
  filled in. Today: the contact whose IBAN matches, one line at the 0%
  purchase rate on uncategorized expenses, for what is open on the
  transaction; without a matching contact a purchase invoice is refused.
- [ ] **Permissions wider than Moneybird's:** products and projects can be read
  with the sales invoices, estimates or documents permission (Moneybird:
  settings only); members log their own time without the sales invoices
  permission; task lists declare `time_entries` and check the other five.
- [ ] **Settled design differences:** inputs stay 36px high (Moneybird 28px);
  document pages keep a narrower column than Moneybird's; MoneyHarpy's brand
  colours stay. Keep, or follow Moneybird?
- [ ] **External sales invoices:** "prices include VAT" defaults to off rather
  than the administration's setting; the entry number is the highest plus one,
  so deleting the latest frees its number; they never get reminders.
- [ ] **Dubious invoices:** only a payment of the whole rest releases the
  provision; partial payments leave it.
- [ ] **Signed estimate PDF** answers 422 (Moneybird's error for an unsigned
  estimate) rather than 404.

### API behaviour still missing

- [ ] Settling a purchase invoice or receipt against a sales invoice.
- [ ] `link_booking` types `Payment`, `PaymentTransaction`,
  `PaymentTransactionBatch` and `VatDocument`.
- [ ] Closing a year; until then the balance sheet shows every year's result
  as an open year.
- [ ] Exports run while the request waits; Moneybird queues them.
- [ ] Auditfile is always XAF 3.2, also when 4.0 is asked for.
- [ ] Brugstaat uses MoneyHarpy's own XML layout; Moneybird's format is not
  published.
- [ ] Retroactive linear value changes of assets run while the request waits;
  asset sources only look at purchase document lines.
- [ ] Online signing of estimates (`sign_online`, signed PDF).
- [ ] Merging recurring invoices (`mergeable` is stored, unused).
- [ ] Subscriptions: merging charges into the next invoice is approximate (a
  failure halfway can bill a charge twice); contact charges are invoiced when
  the worker sees the month is over instead of being scheduled for the 1st; a
  failed send leaves a draft with the charges marked billed.
- [ ] Webhook events never sent: `estimate_state_changed_to_late` (late is
  worked out on reading), template published/hidden; `events` arrays on
  records are always empty.
- [ ] Moneybird Payments features answer as Moneybird does with Payments off:
  payment mandates, checkout, sales links, subscription templates.
- [ ] Check that deleting a payment linked to a bank transaction needs the bank
  permission on every invoice type (it does on external invoices).
- [ ] Watch Moneybird's API for changes: the *Moneybird compatibility* workflow
  covers schemas; rerun the operation-by-operation comparison when it reports
  a change.

### Features not built

- [ ] Peppol sending and receiving (sending answers "not available yet").
- [ ] Email-in addresses for external invoices and incoming documents;
  documents from email or Peppol; recurring purchase documents.
- [ ] Payment services: Mollie, PayPal, SEPA direct debit, Online incasso,
  paying through Moneybird-style banking.
- [ ] Direct bank connections (`internal/bankfile` is where a source would hand
  over transactions).
- [ ] VAT returns.
- [ ] Statistics and export on lists (shown greyed out, "Not available yet"),
  including the compose export.
- [ ] Side menu pages without a screen yet: reports overview, audit report,
  hours report, hours per project, events, online sales, task list
  templates, purchase transaction batches.
- [ ] Online checkout; customer portal (the API returns links into the app).
- [ ] Passkeys, "keep me signed in", KVK company search while onboarding,
  house number field, UBO and verification blocks, todos and downloads in the
  notifications panel, Moneybird's "explainer video" and "show subscriptions".

### Screens

- [ ] Pages without a Moneybird reference only follow its patterns: invoice,
  external and recurring invoice pages and editors, contacts, bank
  transactions, journal, reports, and the new pages (incoming documents,
  quotes, timesheets, task lists, assets, products, projects). Match each one
  against a saved Moneybird page.
- [ ] Info and statistics buttons lack Moneybird's dropdown caret; the period
  band can only be dragged by its ends.
- [ ] New API areas mostly have minimal screens; many operations (asset value
  changes, subscriptions, purchase transactions, SEPA batches) have no screen.

### Technical

- [ ] Shared API rate limit per address (150 requests per 5 minutes) is easily
  hit by several test runs on one machine.

## License

[MIT](LICENSE)
