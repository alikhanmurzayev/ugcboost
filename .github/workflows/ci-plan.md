# CI Pipeline Plan

## Stage 1: Lint + Unit + Build & Push (all parallel, no dependencies)

```
  lint-backend             golangci-lint
  lint-web                 tsc --noEmit + eslint
  lint-tma                 tsc --noEmit + eslint
  lint-landing             tsc --noEmit + eslint
  test-unit-backend        go test ./... -count=1 -race
  test-unit-web            npm ci -w web     -> npm test -- --run
  test-unit-tma            npm ci -w tma     -> npm test -- --run
  test-unit-landing        npm ci -w landing -> npm test -- --run
  build-and-push-images    matrix 4x: backend, web, tma, landing
                           backend image includes goose + migrations/
```

## Stage 2: Isolated E2E (needs: all stage 1)

Runs in parallel with Stage 3.

Uses docker-compose.test.yml + docker-compose.ci.yml to spin up:
postgres -> backend (goose migrate via entrypoint) -> web

```
  test-e2e-backend         cd backend/e2e && go test ./... -v
  test-e2e-browser         cd frontend/web && npx playwright test
                           BASE_URL=http://localhost:3001
                           API_URL=http://localhost:8082
```

## Stage 3: Migrate + Deploy (needs: all stage 1)

Runs in parallel with Stage 2.

```
  migrate-staging          SSH -> docker run backend:staging goose -dir /migrations up
  deploy-staging           SSH -> Dokploy webhooks (backend, web, tma, landing)
                           + health check https://staging-api.ugcboost.kz/healthz
```

## Stage 4: Staging E2E (needs: stage 3)

Does NOT wait for Stage 2.

```
  test-staging-backend     cd backend/e2e && go test against staging-api.ugcboost.kz
  test-staging-web         cd frontend/web && npx playwright test
                           BASE_URL=https://staging-app.ugcboost.kz
                           API_URL=https://staging-api.ugcboost.kz
```

## Key changes vs old CI

- Separate migrations image removed; goose baked into backend image
- Dockerfile.migrations deleted
- All stage 1 jobs run in parallel (lint + test + build, no sequential stages)
- Frontend npm ci uses workspace root: `cd frontend && npm ci -w <name>`
- E2E test path: e2etest/ -> backend/e2e/
- lint-landing and test-unit-frontend-landing added
- push-images renamed to build-and-push-images, matrix reduced 5x -> 4x
