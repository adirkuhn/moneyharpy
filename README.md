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
