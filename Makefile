SQLC := go run github.com/sqlc-dev/sqlc/cmd/sqlc@v1.31.1
E2E_COMPOSE := docker compose -p moneyharpy-e2e -f e2e/docker-compose.e2e.yml

.PHONY: generate test test-web test-e2e run

generate: ## Regenerate code from the OpenAPI contract and SQL queries.
	cd internal/httpapi && go tool oapi-codegen -config oapi-codegen.yaml ../../api/openapi.yaml
	cd internal/postgres && $(SQLC) generate
	cd web && npm run gen:api

test: ## Backend unit and integration tests. Needs Docker.
	go test -race ./...

test-web: ## Frontend tests.
	cd web && npm test

test-e2e: ## End-to-end tests against a disposable stack.
	$(E2E_COMPOSE) up -d --build --wait
	cd e2e && E2E_BASE_URL=http://localhost:18080 npm test; status=$$?; cd .. && $(E2E_COMPOSE) down -v; exit $$status

run: ## Start the app with Docker Compose.
	docker compose up --build
