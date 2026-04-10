# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project

UGCBoost — full-stack product (backend, frontend, landing pages, external integrations).

## Communication

All communication in this project is in **Russian**. Product documentation (briefs, PRDs, specs) is written in **Russian**. Code, variable names, and commits stay in English.

## Team

- **Alikhan Murzayev** (alikhan) — tech lead, architecture, backend
- **Aidana** (aidana) — product, business requirements, documentation, landing pages

## Development Methodology

BMad framework (modules: Core, BMM, CIS, TEA). Use BMad skills and agents for planning, design, and implementation workflows.

UGCBoost — стартап-MVP с командой из 2 человек и жёстким дедлайном. Это UGC-маркетплейс, не медицинский софт и не финтех. Калибруй все процессы под эту реальность:

- Все артефакты (PRD, архитектура, спеки) — living documents. Написал достаточно чтобы начать кодить — начинай кодить. Детали уточняются по ходу реализации
- Ревью и анализ — один раунд, без многофазных процессов. Нашёл проблему — пофиксил — пошёл дальше
- Документы — средство, не цель. Рабочий код важнее идеального документа
- Реализация вертикальными слайсами: каждая итерация заканчивается рабочим user flow
- Внешние интеграции (LiveDune, TrustMe) мокаются до момента когда реально нужны

## BMad Setup

BMad config files (`_bmad/*/config.yaml`) are per-user and gitignored. After cloning, run `/bmad-init` with:
- communication_language: Russian
- document_output_language: Russian
- output_folder: _bmad-output
- project_name: ugcboost

## Environments

| # | Окружение | Назначение | Хост |
|---|---|---|---|
| 1 | **local** | Локальная разработка. `make local` поднимает всё через Docker Compose (PostgreSQL, backend, web, tma) | Текущая машина (home server Alikhan, где запущен Claude Code) |
| 2 | **staging** | Тестовая среда. Автодеплой из main после CI. БД не чистится, данные накапливаются. Доступ ограничен через Cloudflare Access | Отдельный VPS (будет настроен) |
| 3 | **production** | Продакшн. Деплой только через manual approve в GitHub Actions. Реальные пользователи | Отдельный VPS (будет настроен) |
| 4 | **backup** | Бэкапы из production: БД (pg_basebackup + WAL archiving) + файлы/uploads (rsync). RTO ≤ 4 часа | Отдельный VPS (слабый CPU, большой диск) |

### Docker-образы (отдельный на каждый сервис)

- `ugcboost/backend` — Go binary + миграции
- `ugcboost/web` — Nginx + React SPA (веб-кабинет брендов/админов)
- `ugcboost/tma` — Nginx + React SPA (Telegram Mini App креаторов)
- `ugcboost/landing` — Nginx + Astro static

Каждый образ собирается **один раз** в CI. Один и тот же образ деплоится на staging и production. Различия — только env vars.

### CI/CD Pipeline

```
feature branch → PR → merge to main → CI (lint → test-unit → build images)
  → auto-deploy staging → E2E tests on staging
  → manual approve → deploy production
```

### Makefile targets

```
make test               # Unit-тесты бэкенда (без БД, быстро)
make test-all           # ВСЕ тесты (unit + backend E2E + browser E2E)
make test-e2e-backend   # Backend E2E (docker-compose.test.yml)
make test-coverage      # Unit-тесты с отчётом покрытия
make test-e2e           # Browser E2E headless (поднимает PostgreSQL + миграции автоматически)
make test-e2e-ui        # Browser E2E с Playwright UI (порт 3333, для визуального просмотра)
make test-e2e-report    # Открыть HTML-отчёт последнего прогона
make local              # Docker Compose up (PostgreSQL + backend + web + tma)
make local-down         # Docker Compose down
make dev                # PostgreSQL + миграции, app-код запускаешь сам
make lint               # golangci-lint + eslint
make migrate            # Применить миграции
make generate           # mockery + OpenAPI codegen
make codegen            # OpenAPI → Go + TS
make build              # Собрать Docker-образы
```

### Запуск тестов агентами

Makefile автоматически определяет доступность Docker и при необходимости оборачивает команды через `sg docker`. Никаких ручных обёрток не нужно:

```
make test           # только unit (быстро)
make test-all       # unit + backend E2E + browser E2E
make test-e2e-backend  # только backend E2E
```

**Визуальный просмотр E2E** (Playwright UI):
```
make test-e2e-ui
# Потом пользователь подключается через SSH-туннель:
# ssh -L 3333:localhost:3333 alikhan@<server-ip>
# Открывает http://localhost:3333
```

**Перед повторным запуском E2E** — убить зависшие процессы:
```
pkill -f playwright 2>/dev/null; sleep 1
```

## Testing

**MANDATORY: Before writing, modifying, or reviewing ANY test code, read `_bmad-output/planning-artifacts/testing-architecture.md` in full. This applies to: `*_test.go`, `*.spec.ts`, `e2etest/`, `docker-compose.test.yml`, Makefile test targets, CI test jobs.**

Three levels:

### Level 1: Unit tests (`make test`)
- mockery + testify. No hand-rolled mocks.
- Repository tests MUST assert exact SQL string + arguments.
- Every layer: middleware, handler, service, repository, token, closer.

### Level 2: Backend E2E (`make test-e2e-backend`)
- Black box. Separate Go module `e2etest/`. NEVER import internal packages.
- HTTP client auto-generated from OpenAPI (oapi-codegen).
- All infra in Docker (`docker-compose.test.yml`). Tests run on host.
- Test data via `/test/*` endpoints only. No direct DB access from tests.
- Covers ALL business scenarios and edge cases through API.

### Level 3: Browser E2E (`make test-e2e-ui`)
- Playwright. Full stack in Docker (postgres + migrations + backend + web).
- Critical user flows only. Don't duplicate L2 edge cases.
- Setup via backend API (`:8081/test/*`), actions via browser (`:3001`).

### Hard rules
- `ENABLE_TEST_ENDPOINTS=true` — test-only `/test/*` routes. NEVER in production.
- Migrations always separate from backend (init container in tests, separate job in CI/CD).
- Tests idempotent, work on non-empty DB, `uniqueEmail()`.
- No manual testing via Playwright MCP. All tests via `make` targets only.
- E2E tests target Docker containers, not `go run` on host.

## Coding Standards

**MANDATORY: Before writing ANY code, read ALL files in `docs/standards/`. Every rule applies to all new code and refactoring.**

Полный список правил — в `docs/standards/` (62 правила, 15 категорий). Ключевые:

**Backend:**
1. **Строковые литералы запрещены для enum-значений** — роли, коды ошибок, HTTP-заголовки, названия таблиц/колонок → только константы (CS-01—CS-05)
2. **Кодогенерация обязательна** — типы запросов/ответов, роутинг, query/path params → из generated code, не вручную (CS-06—CS-10)
3. **Библиотеки > велосипеды** — для инфраструктурного кода (config, lifecycle, etc.) использовать проверенные библиотеки (CS-12—CS-14)
4. **Слои не нарушаются** — handler → service → repository. Авторизация — отдельный сервис. Никакого прямого SQL вне repository (CS-15—CS-18)
5. **Ошибки не игнорируются** — каждая ошибка возвращается или логируется. Невалидный ввод = 422, не тихий fallback (CS-19—CS-23)
6. **Зависимости через конструктор** — никаких Set*, все зависимости в New*() (CS-24)
7. **Тесты репозиториев — строковые литералы в SQL** — двойная проверка, что константы в коде корректны (CS-33)

**Frontend:**
8. **API-типы только из generated/schema.ts** — ручные interface/type для API запрещены (CS-41—CS-42)
9. **API paths, query keys, route paths — константы** — строковые литералы запрещены (CS-46—CS-47, CS-49—CS-50)
10. **Каждая мутация — onError обязателен** — молчаливые мутации запрещены (CS-48)
11. **Loading/Error/Empty states обязательны** — голый "Загрузка..." запрещён (CS-53)
12. **strict: true в tsconfig** — non-null assertions запрещены (CS-43—CS-44)
13. **i18n для всех пользовательских строк** — хардкод русского текста в JSX запрещён (CS-54)
14. **Role-based route guards** — фронтенд проверяет доступ по роли, не только наличие auth (CS-55)

### Безопасность

- Секреты — через Dokploy env vars, не в коде и не в git
- Staging — доступ ограничен через Cloudflare Access (policy по email)
- Production — Cloudflare (DDoS/WAF, скрытие IP), SSH hardening (ключи, кастомный порт, fail2ban)

## Git Workflow

- **Branch naming**: `<username>/<description>` (e.g. `alikhan/backend-auth`)
- **No direct pushes to main** — only via Pull Request with 1 approval
- **Commit messages**: concise, in English, describe the "why"
