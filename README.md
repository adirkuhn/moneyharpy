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

## License

[MIT](LICENSE)
