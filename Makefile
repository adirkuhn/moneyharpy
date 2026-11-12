SQLC := go run github.com/sqlc-dev/sqlc/cmd/sqlc@v1.31.1
GOLANGCI := go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.15.0
E2E_COMPOSE := docker compose -p moneyharpy-e2e -f e2e/docker-compose.e2e.yml

.PHONY: generate moneybird-schemas lint test test-web test-e2e check run docs

generate: ## Regenerate code from the OpenAPI contract and SQL queries.
	cd internal/httpapi && go tool oapi-codegen -config oapi-codegen.yaml ../../api/openapi.yaml
	cd internal/postgres && $(SQLC) generate
	cd web && npm run gen:api

moneybird-schemas: ## Copy the structure of Moneybird's current API schemas into the contract.
	go run ./cmd/mbschemas
	$(MAKE) generate

lint: ## Formatting, vet and golangci-lint, as CI runs them.
	test -z "$$(gofmt -l cmd internal api web/embed.go)"
	go vet ./...
	$(GOLANGCI) run ./...

test: ## Backend unit and integration tests. Needs Docker.
	go test -race ./...

test-web: ## Frontend tests.
	cd web && npm test

test-e2e: ## End-to-end tests against a disposable stack.
	$(E2E_COMPOSE) up -d --build --wait
	cd e2e && E2E_BASE_URL=http://localhost:18080 npm test; status=$$?; cd .. && $(E2E_COMPOSE) down -v; exit $$status

check: ## Everything CI checks, run locally: lint, generated code, backend, frontend and e2e.
	$(MAKE) lint
	$(MAKE) generate
	git diff --exit-code
	$(MAKE) test
	cd web && npm test && npm run build
	$(MAKE) test-e2e

run: ## Start the app with Docker Compose.
	docker compose up --build

docs: ## Documentation site into build/site. Needs mkdocs-material.
	cd web && npm ci --silent
	./docs/build.sh
