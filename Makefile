.PHONY: local local-down test test-all test-unit test-e2e test-e2e-ui test-e2e-backend lint migrate codegen generate build dev

DATABASE_URL ?= postgres://ugcboost:ugcboost_dev@localhost:5433/ugcboost?sslmode=disable
export PATH := $(HOME)/go/bin:/usr/local/go/bin:$(PATH)

# Auto-wrap: if docker not accessible (e.g. missing docker group), re-run via sg docker
define DOCKER_WRAP
@if docker info >/dev/null 2>&1; then \
	$(MAKE) _$@; \
else \
	sg docker -c "$(MAKE) _$@"; \
fi
endef

# Dev: run all services with Docker Compose
local:
	$(DOCKER_WRAP)

_local:
	docker compose up --build -d

local-down:
	$(DOCKER_WRAP)

_local-down:
	docker compose down

# Dev: run backend + frontend locally (no Docker for app code)
dev:
	$(DOCKER_WRAP)

_dev: _ensure-db
	@echo "Ready! Run these in separate terminals:"
	@echo "  cd backend && go run ./cmd/api"
	@echo "  cd frontend/web && npm run dev"
	@echo "  cd frontend/tma && npm run dev"

# Database migrations
migrate:
	cd migrations && goose -dir . postgres "$(DATABASE_URL)" up

migrate-down:
	cd migrations && goose -dir . postgres "$(DATABASE_URL)" down

migrate-create:
	cd migrations && goose -dir . create $(NAME) sql

# Code generation (mocks + OpenAPI)
generate:
	cd backend && mockery
	$(MAKE) codegen

# OpenAPI codegen
codegen:
	oapi-codegen -package api -generate chi-server,models \
		-o backend/internal/api/server.gen.go \
		api/openapi.yaml
	@echo "Go types + Chi server generated"
	cd frontend/web && npx openapi-typescript ../../api/openapi.yaml -o src/api/generated/schema.ts
	@echo "Web TS types generated"
	cd frontend/tma && npx openapi-typescript ../../api/openapi.yaml -o src/api/generated/schema.ts
	@echo "TMA TS types generated"
	oapi-codegen -package apiclient -generate types \
		-o e2etest/apiclient/types.gen.go api/openapi.yaml
	oapi-codegen -package apiclient -generate client \
		-o e2etest/apiclient/client.gen.go api/openapi.yaml
	oapi-codegen -package testclient -generate types \
		-o e2etest/testclient/types.gen.go api/openapi-test.yaml
	oapi-codegen -package testclient -generate client \
		-o e2etest/testclient/client.gen.go api/openapi-test.yaml
	@echo "E2E test clients generated"

# ── Tests ──────────────────────────────────────────────────────────
# make test-all  — запускает ВСЁ (unit + backend E2E + browser E2E)
# make test      — только unit (быстро, без Docker)

test: test-unit

test-all:
	$(DOCKER_WRAP)

_test-all: _test-cleanup test-unit _test-e2e-backend _test-e2e
	@docker compose -f docker-compose.test.yml down 2>/dev/null || true
	@echo ""
	@echo "═══ All tests passed ═══"

test-unit:
	@echo "── Unit tests ──"
	cd backend && go test ./... -count=1 -race
	-cd frontend/web && npm test -- --run
	-cd frontend/tma && npm test -- --run

test-coverage:
	cd backend && go test ./internal/closer ./internal/handler ./internal/middleware ./internal/repository ./internal/service \
		-count=1 -coverprofile=coverage.out -covermode=atomic
	@cd backend && go tool cover -func=coverage.out | tail -1
	cd backend && go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report: backend/coverage.html"

# Backend E2E: docker-compose.test.yml (postgres:5434 + backend:8082)
test-e2e-backend:
	$(DOCKER_WRAP)

_test-e2e-backend:
	@echo "── Backend E2E tests ──"
	@docker compose -f docker-compose.test.yml down 2>/dev/null || true
	docker compose -f docker-compose.test.yml up -d --build --wait
	cd e2etest && go test ./... -count=1 -v -race -timeout 5m; \
	status=$$?; \
	cd .. && docker compose -f docker-compose.test.yml down; \
	exit $$status

# Browser E2E: Playwright (postgres:5433 + go run:8080 + vite:5173)
test-e2e:
	$(DOCKER_WRAP)

_test-e2e: _ensure-db
	@echo "── Browser E2E tests ──"
	cd frontend/web && npx playwright test

test-e2e-ui:
	$(DOCKER_WRAP)

_test-e2e-ui: _ensure-db
	cd frontend/web && npx playwright test --ui --ui-host=0.0.0.0 --ui-port=3333

test-e2e-report:
	cd frontend/web && npx playwright show-report --host=0.0.0.0 --port=3333

# ── Internal helpers ───────────────────────────────────────────────

_ensure-db:
	@docker compose up -d postgres
	@until docker compose exec postgres pg_isready -U ugcboost > /dev/null 2>&1; do sleep 1; done
	@$(MAKE) migrate

_test-cleanup:
	@docker compose -f docker-compose.test.yml down 2>/dev/null || true

# ── Lint & Build ───────────────────────────────────────────────────

lint:
	cd backend && golangci-lint run ./...
	-cd frontend/web && npx eslint src/
	-cd frontend/tma && npx eslint src/

build:
	$(DOCKER_WRAP)

_build:
	docker build -t ugcboost/backend ./backend
	docker build -t ugcboost/web -f frontend/web/Dockerfile .
	docker build -t ugcboost/tma -f frontend/tma/Dockerfile .
