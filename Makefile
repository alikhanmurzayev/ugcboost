.PHONY: local local-down test test-unit test-e2e lint migrate codegen build dev

# Dev: run all services with Docker Compose
local:
	docker compose up --build -d

local-down:
	docker compose down

# Dev: run backend + frontend locally (no Docker for app code)
dev:
	@echo "Starting PostgreSQL..."
	docker compose up -d postgres
	@echo "Waiting for PostgreSQL..."
	@until docker compose exec postgres pg_isready -U ugcboost > /dev/null 2>&1; do sleep 1; done
	@echo "Running migrations..."
	$(MAKE) migrate
	@echo "Ready! Run these in separate terminals:"
	@echo "  cd backend && go run ./cmd/api"
	@echo "  cd web && npm run dev"
	@echo "  cd tma && npm run dev"

# Database migrations
migrate:
	cd migrations && goose -dir . postgres "$(DATABASE_URL)" up

migrate-down:
	cd migrations && goose -dir . postgres "$(DATABASE_URL)" down

migrate-create:
	cd migrations && goose -dir . create $(NAME) sql

# OpenAPI codegen
codegen:
	oapi-codegen -package api -generate chi-server,models \
		-o backend/internal/api/server.gen.go \
		api/openapi.yaml
	@echo "Go types + Chi server generated"
	cd web && npx openapi-typescript ../api/openapi.yaml -o src/api/generated/schema.ts
	@echo "Web TS types generated"
	cd tma && npx openapi-typescript ../api/openapi.yaml -o src/api/generated/schema.ts
	@echo "TMA TS types generated"

# Tests
test: test-unit test-e2e

test-unit:
	cd backend && go test ./... -count=1 -race
	-cd web && npm test -- --run
	-cd tma && npm test -- --run

test-e2e:
	-cd e2e && npx playwright test

# Lint
lint:
	cd backend && golangci-lint run ./...
	-cd web && npx eslint src/
	-cd tma && npx eslint src/

# Build Docker images
build:
	docker build -t ugcboost/backend ./backend
	docker build -t ugcboost/web ./web
	docker build -t ugcboost/tma ./tma
