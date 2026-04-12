.PHONY: deps deps-down migrate migrate-reset migrate-create \
       backend web tma landing \
       test test-unit lint generate codegen

DATABASE_URL ?= postgres://ugcboost:ugcboost_dev@localhost:5433/ugcboost?sslmode=disable

# ── Infrastructure ────────────────────────────────────────────────

deps:
	docker compose up -d postgres
	@until docker compose exec postgres pg_isready -U ugcboost > /dev/null 2>&1; do sleep 1; done
	@echo "Postgres ready on :5433"

deps-down:
	docker compose down

# ── Migrations ────────────────────────────────────────────────────

migrate:
	cd migrations && goose -dir . postgres "$(DATABASE_URL)" up

migrate-reset:
	cd migrations && goose -dir . postgres "$(DATABASE_URL)" reset

migrate-create:
	@test -n "$(NAME)" || (echo "Usage: make migrate-create NAME=add_users_table" && exit 1)
	cd migrations && goose -dir . create $(NAME) sql

# ── Run locally (dev mode) ────────────────────────────────────────

backend:
	cd backend && go run ./cmd/api

web:
	cd frontend/web && npm run dev

tma:
	cd frontend/tma && npm run dev

landing:
	cd frontend/landing && npm run dev

# ── Tests ─────────────────────────────────────────────────────────

test: test-unit

test-unit:
	cd backend && go test ./... -count=1 -race
	cd frontend/web && npm test -- --run
	cd frontend/tma && npm test -- --run

# ── Lint ──────────────────────────────────────────────────────────

lint:
	cd backend && golangci-lint run ./...
	cd frontend/web && npx tsc --noEmit
	cd frontend/web && npx eslint src/
	cd frontend/tma && npx tsc --noEmit
	cd frontend/tma && npx eslint src/

# ── Code generation ──────────────────────────────────────────────

generate:
	cd backend && mockery
	$(MAKE) codegen

codegen:
	oapi-codegen -package api -generate chi-server,models \
		-o backend/internal/api/server.gen.go \
		api/openapi.yaml
	cd frontend/web && npx openapi-typescript ../../api/openapi.yaml -o src/api/generated/schema.ts
	cd frontend/tma && npx openapi-typescript ../../api/openapi.yaml -o src/api/generated/schema.ts
	oapi-codegen -package apiclient -generate types \
		-o e2etest/apiclient/types.gen.go api/openapi.yaml
	oapi-codegen -package apiclient -generate client \
		-o e2etest/apiclient/client.gen.go api/openapi.yaml
	oapi-codegen -package testclient -generate types \
		-o e2etest/testclient/types.gen.go api/openapi-test.yaml
	oapi-codegen -package testclient -generate client \
		-o e2etest/testclient/client.gen.go api/openapi-test.yaml
