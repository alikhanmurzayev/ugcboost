.PHONY: compose-up compose-down \
       migrate-up migrate-reset migrate-create \
       start-backend stop-backend \
       start-web stop-web start-tma stop-tma start-landing stop-landing \
       run-backend run-web run-tma run-landing \
       build-backend build-web build-tma build-landing \
       test-unit-backend test-unit-web test-unit-tma test-unit-landing \
       test-unit-backend-coverage \
       test-e2e-backend test-e2e-frontend \
       lint-backend lint-web lint-tma lint-landing \
       generate-api generate-mocks

# ── Build-time version stamping ───────────────────────────────────
# GIT_COMMIT flows through the Dockerfile ARG → ENV so the running
# container surfaces it via config.Version → /healthz. CI (GitHub
# Actions) passes the same variable through docker/build-push-action
# build-args; this Makefile value is consumed by `make run-backend`
# (read from env by config.Load) and by `make start-backend` (passed
# to docker compose as a build arg).

GIT_COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo dev)
export GIT_COMMIT


# ── Infrastructure ────────────────────────────────────────────────

compose-up:
	docker compose up -d --wait
	@echo "Postgres ready on :5433"

compose-down:
	docker compose down

# ── Migrations ────────────────────────────────────────────────────

migrate-up: compose-up
	docker compose --profile backend run --rm migrations

migrate-reset: compose-up
	docker compose --profile backend run --rm migrations goose -dir /migrations postgres "postgres://$${POSTGRES_USER:-ugcboost}:$${POSTGRES_PASSWORD:-ugcboost_dev}@postgres:5432/$${POSTGRES_DB:-ugcboost}?sslmode=disable" reset

migrate-create:
	@test -n "$(NAME)" || (echo "Usage: make migrate-create NAME=add_users_table" && exit 1)
	cd backend/migrations && goose -dir . create $(NAME) sql

# ── Run in Docker (for E2E tests) ────────────────────────────

start-backend: compose-up
	docker compose --profile backend up -d --build --force-recreate --wait backend
	@echo "Backend ready on http://localhost:8082"

stop-backend:
	docker compose --profile backend stop backend

start-web: start-backend
	docker compose --profile web up -d --build --force-recreate --wait web
	@echo "Web ready on http://localhost:3001"

stop-web:
	docker compose --profile web stop web

start-tma: start-backend
	docker compose --profile tma up -d --build --force-recreate --wait tma
	@echo "TMA ready on http://localhost:3002"

stop-tma:
	docker compose --profile tma stop tma

start-landing: start-backend
	docker compose --profile landing up -d --build --force-recreate --wait landing
	@echo "Landing ready on http://localhost:3003"

stop-landing:
	docker compose --profile landing stop landing

# ── Run locally (dev mode) ────────────────────────────────────────

run-backend:
	cd backend && go run -race ./cmd/api

run-web:
	cd frontend/web && npm run dev

run-tma:
	cd frontend/tma && npm run dev

run-landing:
	cd frontend/landing && npm run dev

# ── Build ─────────────────────────────────────────────────────────

build-backend:
	cd backend && go build ./...

build-web:
	cd frontend/web && npm run build

build-tma:
	cd frontend/tma && npm run build

build-landing:
	cd frontend/landing && npm run build

# ── Tests ────────────────────────────────────────────────────

test-unit-backend:
	cd backend && go test ./... -count=1 -race -timeout 5m

# Per-method coverage gate — fails if any public method in the enforced
# packages has < 80%. Scope per REQ-10: handler, service, repository,
# middleware, authz. Excluded: generated code (*.gen.go), mockery output
# (*/mocks/), cmd/, and out-of-scope trivial files (handler/health.go,
# middleware/logging.go, middleware/json.go).
# Runs a separate `go test` pass with -coverprofile so test-unit-backend
# isn't slowed by coverage instrumentation.
test-unit-backend-coverage:
	cd backend && go test -coverprofile=/tmp/ugc-cover.out -race -count=1 ./internal/...
	@cd backend && go tool cover -func=/tmp/ugc-cover.out | \
		awk '$$1 ~ /\/(handler|service|repository|middleware|authz)\// \
			&& $$1 !~ /\.gen\.go:/ \
			&& $$1 !~ /\/mocks\// \
			&& $$1 !~ /\/cmd\// \
			&& $$1 !~ /\/handler\/health\.go:/ \
			&& $$1 !~ /\/middleware\/(logging|json)\.go:/ \
			&& $$2 ~ /^[A-Z]/ { \
				pct = $$NF; gsub(/%/, "", pct); \
				if (pct + 0 < 80.0) { printf "FAIL %s %s %s%%\n", $$1, $$2, pct; fail=1 } \
			} END { exit fail ? 1 : 0 }'

test-unit-web:
	cd frontend/web && npm test -- --run

test-unit-tma:
	cd frontend/tma && npm test -- --run

test-unit-landing:
	cd frontend/landing && npm test -- --run

test-e2e-backend: start-backend
	cd backend/e2e && go test ./... -count=1 -v -race -timeout 5m

test-e2e-frontend: start-web
	cd frontend/web && CI=true BASE_URL=http://localhost:3001 API_URL=http://localhost:8082 npx playwright test

# ── Lint ──────────────────────────────────────────────────────────

lint-backend:
	cd backend && golangci-lint run ./...

lint-web:
	cd frontend/web && npx tsc --noEmit
	cd frontend/web && npx eslint src/

lint-tma:
	cd frontend/tma && npx tsc --noEmit
	cd frontend/tma && npx eslint src/

lint-landing:
	cd frontend/landing && npx tsc --noEmit
	cd frontend/landing && npx eslint src/

# ── Code generation ──────────────────────────────────────────────

generate-api:
	oapi-codegen -package api -generate chi-server,models \
		-o backend/internal/api/server.gen.go \
		backend/api/openapi.yaml
	oapi-codegen -package testapi -generate chi-server,models \
		-o backend/internal/testapi/server.gen.go \
		backend/api/openapi-test.yaml
	cd frontend/web && npx openapi-typescript ../../backend/api/openapi.yaml -o src/api/generated/schema.ts
	cd frontend/tma && npx openapi-typescript ../../backend/api/openapi.yaml -o src/api/generated/schema.ts
	oapi-codegen -package apiclient -generate types \
		-o backend/e2e/apiclient/types.gen.go backend/api/openapi.yaml
	oapi-codegen -package apiclient -generate client \
		-o backend/e2e/apiclient/client.gen.go backend/api/openapi.yaml
	oapi-codegen -package testclient -generate types \
		-o backend/e2e/testclient/types.gen.go backend/api/openapi-test.yaml
	oapi-codegen -package testclient -generate client \
		-o backend/e2e/testclient/client.gen.go backend/api/openapi-test.yaml

generate-mocks:
	cd backend && mockery
