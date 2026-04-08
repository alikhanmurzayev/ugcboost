# Архитектура тестирования UGCBoost

## Цель

Фундамент тестирования для всего проекта. Паттерны которые масштабируются без переделок.

## Три уровня

### Level 1: Unit-тесты бэкенда

Каждый слой изолированно, без БД, быстрые.

| Решение | Выбор |
|---------|-------|
| Моки | **mockery** — генерация из интерфейсов (`go generate`) |
| Ассерты | **testify** — `assert`, `require`, `mock` |
| Паттерн | Моки генерируются в `mocks/` рядом с интерфейсом |
| Запуск | `make test` (без Docker, без БД) |

#### Что проверяем по слоям

| Слой | Что мокаем | Что проверяем |
|------|-----------|---------------|
| middleware | TokenValidator (интерфейс) | HTTP статусы, context values, headers |
| handler | Auth interface (service) | HTTP статусы, JSON body, cookies |
| service | UserRepo, TokenGenerator | Бизнес-логика, error mapping |
| repository | dbutil.DB (через mockery) | **SQL-строка + аргументы** + маппинг row→struct + error propagation |
| token | Ничего (чистая логика) | Claims, expiry, hash корректность |
| closer | Ничего (чистая логика) | LIFO порядок, error handling |

**Repository — важно:** тесты MUST проверять точную SQL-строку и аргументы. Изменение SQL = красный тест = осознанное обновление. Это страховка при AI-driven разработке.

### Level 2: Backend E2E (чёрный ящик)

HTTP-клиент на хосте → backend в Docker → PostgreSQL в Docker.

| Решение | Выбор |
|---------|-------|
| HTTP-клиент | **oapi-codegen** `-generate client` из OpenAPI (автогенерация) |
| Изоляция | Отдельный Go module (`e2etest/`) — невозможно импортировать internal |
| Cookie support | Стандартный `http.Client` с `http.CookieJar` поверх сгенерированного клиента |
| Тест-эндпоинты | `ENABLE_TEST_ENDPOINTS=true` → `/test/*` роуты |
| Данные | Идемпотентные: `uniqueEmail()`, без cleanup |
| Запуск | `make test-e2e-backend` |

**Жёсткие ограничения:**
- Тесты НЕ импортируют внутренние пакеты (физически невозможно — отдельный module)
- Тесты НЕ подключаются к БД напрямую. Всё через HTTP API.
- Мокаются только внешние интеграции (email, LiveDune, TrustMe)

**Scope:** ВСЯ бизнес-логика, все edge cases, все HTTP статусы и error codes.

**Создание тестовых данных:**
- `POST /test/seed-user` — создать пользователя с email/password/role
- `GET /test/reset-tokens?email=...` — получить raw reset token
- Обычный registration/creation flow покрывается отдельными тестами

### Level 3: Browser E2E (Playwright)

Playwright на хосте → браузер → Nginx+SPA в Docker → backend в Docker → PostgreSQL в Docker.

| Решение | Выбор |
|---------|-------|
| Фреймворк | **Playwright Test Runner** (`@playwright/test`) |
| Стек | Весь в Docker: postgres + migrations + backend + web (Nginx+SPA) |
| Запуск | `make test-e2e-ui` |

**Scope:** Только критические user flows. НЕ дублирует edge cases из L2.
- L2 ловит баги бизнес-логики (быстрая обратная связь)
- L3 ловит баги сборки SPA, Nginx конфигурации, проксирования, фронтенд-интеграции

**Рост:** 2-3 smoke-теста на каждый новый модуль (кампании, креаторы, модерация и т.д.)

**Setup:** создание данных через API бэкенда напрямую (`:8081/test/*`), действия через браузер (`:3001`).

## Docker-инфраструктура для E2E

### docker-compose.test.yml

```yaml
services:
  postgres:
    image: postgres:17
    ports: ["5433:5432"]          # не конфликтует с dev (5432)
    environment:
      POSTGRES_DB: ugcboost_test
      POSTGRES_USER: ugcboost
      POSTGRES_PASSWORD: ugcboost
    healthcheck:
      test: pg_isready -U ugcboost
      interval: 2s
      timeout: 5s
      retries: 10

  migrations:
    build: .                       # init-контейнер
    command: goose -dir /migrations up
    depends_on:
      postgres:
        condition: service_healthy
    # завершается после миграций

  backend:
    build: ./backend
    ports: ["8081:8080"]           # не конфликтует с dev (8080)
    environment:
      DATABASE_URL: postgres://ugcboost:ugcboost@postgres:5432/ugcboost_test?sslmode=disable
      ENABLE_TEST_ENDPOINTS: "true"
      JWT_SECRET: test-secret
    depends_on:
      migrations:
        condition: service_completed_successfully
    healthcheck:
      test: ["CMD", "wget", "-qO-", "http://localhost:8080/healthz"]
      interval: 2s
      timeout: 5s
      retries: 10

  web:
    build: ./web
    ports: ["3001:80"]             # не конфликтует с dev (5173)
    depends_on:
      backend:
        condition: service_healthy
```

**Цепочка:** postgres healthy → migrations completed → backend healthy → web ready.

**Запуск:** `docker compose -f docker-compose.test.yml up -d --wait` — ждёт все healthcheck'и.

### Миграции

Миграции ВСЕГДА отдельно от бэкенда:
- **В тестах:** init-контейнер `migrations` в docker-compose.test.yml
- **В CI/CD:** отдельная job
- **Бэкенд никогда не запускает миграции сам**

### ENABLE_TEST_ENDPOINTS

- Сервер при `ENABLE_TEST_ENDPOINTS=true` регистрирует роуты под `/test/*`
- `POST /test/seed-user` — создать пользователя
- `GET /test/reset-tokens?email=...` — получить raw reset token (mock email service хранит в памяти)
- В production переменная НЕ выставляется → эндпоинты не существуют
- CI deploy pipeline проверяет отсутствие этой переменной

## Makefile targets

```makefile
test:                     # Level 1: unit, без Docker
    cd backend && go test ./... -count=1 -race -timeout 2m

test-e2e-backend:         # Level 2: docker up → go test → docker down
    docker compose -f docker-compose.test.yml up -d --wait
    cd e2etest && go test ./... -count=1 -race -timeout 5m; \
    status=$$?; \
    docker compose -f docker-compose.test.yml down; \
    exit $$status

test-e2e-ui:              # Level 3: docker up → playwright → docker down
    docker compose -f docker-compose.test.yml up -d --wait
    cd web && npx playwright test; \
    status=$$?; \
    docker compose -f docker-compose.test.yml down; \
    exit $$status

test-e2e-ui-headed:       # Level 3 с окном браузера
    docker compose -f docker-compose.test.yml up -d --wait
    cd web && npx playwright test --headed; \
    status=$$?; \
    docker compose -f docker-compose.test.yml down; \
    exit $$status

test-coverage:            # Coverage отчёт
    cd backend && go test ./internal/... -count=1 -coverprofile=coverage.out -covermode=atomic
    cd backend && go tool cover -html=coverage.out -o coverage.html
```

## CI (`.github/workflows/ci.yml`)

Три параллельных job-а:

1. **test-unit** — Go, без PostgreSQL
2. **test-e2e-backend** — Go + docker-compose.test.yml + E2E тесты
3. **test-e2e-ui** — Node + Playwright + docker-compose.test.yml

`build` зависит от всех трёх.

## Отчёты

| Уровень | Отчёт |
|---------|-------|
| L1 Unit | `go test -v`, coverage HTML через `make test-coverage` |
| L2 Backend E2E | `go test -v` (в CI — gotestsum для JUnit XML) |
| L3 Browser E2E | Playwright HTML report, скриншоты на failure, trace viewer |

Coverage: растёт с каждым PR, не падает. Без жёсткого порога %.

## Статус

- [x] Архитектура (3 уровня, инструменты, границы)
- [x] Docker-инфраструктура (docker-compose.test.yml)
- [x] Makefile targets
- [x] Правила в CLAUDE.md
- [ ] Реализация Level 1 (mockery + testify + тесты всех слоёв)
- [ ] Реализация Level 2 (e2etest module + oapi-codegen client + сценарии)
- [ ] Реализация Level 3 (рефакторинг Playwright + новые spec-файлы)
- [ ] CI pipeline (3 параллельных job-а)
