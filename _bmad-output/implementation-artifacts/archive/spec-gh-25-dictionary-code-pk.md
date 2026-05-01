---
title: 'Перевод словарей categories/cities на code как PK (gh-25)'
type: 'refactor'
created: '2026-04-30'
status: 'done'
baseline_commit: '4abf0230929c156f4c113631d91cf09dc41dbc1e'
context:
  - 'docs/standards/backend-repository.md (§ Идентификатор словаря, § Миграции)'
  - 'docs/standards/backend-codegen.md'
  - 'docs/standards/backend-testing-unit.md'
---

<frozen-after-approval reason="human-owned intent — do not modify unless human renegotiates">

## Intent

**Problem:** Словари `categories` и `cities` хранят и `id (UUID)`, и `code (TEXT UNIQUE)`. Потребители держат UUID-FK (`creator_application_categories.category_id`), что форсит JOIN при чтении и indirection в repo/service. На стороне `creator_applications.city` — TEXT без FK, имя колонки не отражает суть (это код).

**Approach:** Single PR + single migration переключает обе таблицы на `code` как PK; FK на потребителях — `*_code TEXT REFERENCES <dict>(code)`. JOIN в репо и `resolveCategoryIDs`-indirection удаляются. API-контракт (`/dictionaries/*`, `/creators/applications`) и фронт не меняются — они уже работают через коды. Кратковременная несовместимость во время раскатки приемлема (tech-debt, low-traffic пути).

## Boundaries & Constraints

**Always:**
- Миграция — одна forward-миграция goose, одной транзакцией. Backfill `category_code` через JOIN ДО drop UUID id.
- Все стандарты `docs/standards/` — hard rules. Особенно: `backend-repository.md` (`{Entity}Column{Field}` константы, `selectColumns` через stom, no string literals в коде), `backend-codegen.md` (мокsы — mockery, не ручные).
- Coverage gate ≥ 80% на каждом методе в handler/service/repository/middleware/authz сохраняется.
- `docs/standards/backend-repository.md` § «Идентификатор словаря» — переписать в однострочник про целевую модель.

**Ask First:**
- Если миграция падает на orphan-кодах (`creator_applications.city_code NOT IN cities.code` или battle-test на staging) — HALT и спросить (cleanup данных vs ослабление FK).
- Если backfill `category_code` через JOIN даёт NULL'ы (orphan `category_id`) — HALT.

**Never:**
- Нельзя редактировать существующие миграции in-place — только новая forward.
- Не менять API-контракт (`api/openapi.yaml`) — поля `city`, `categories[]` остаются как есть в request/response.
- Не трогать frontend (web/tma/landing) — они уже отдают/принимают коды.
- Не делать multi-stage миграцию (решение human'а — single PR, см. Spec Change Log при изменении).

## I/O & Edge-Case Matrix

| Scenario | Input / State | Expected Output / Behavior | Error Handling |
|----------|---------------|----------------------------|-----------------|
| Submit creator application с валидными codes | `categories: ["beauty","fashion"]`, `city: "almaty"` | INSERT в `creator_application_categories` с `category_code` напрямую; INSERT в `creator_applications.city_code` | N/A |
| Submit с unknown category | `categories: ["nonexistent"]` | 422 `UNKNOWN_CATEGORY` (как сейчас, но проверяет по code напрямую) | domain.NewValidationError |
| GET application с активной категорией | application с `category_code='beauty'` | Detail-response: `categories: [{code:"beauty",name:"Бьюти…",sortOrder:20}]` без JOIN в repo | N/A |
| GET application с деактивированной категорией | `categories.active=false` для кода | Fallback в handler `resolveCategory`: `{code:"x",name:"x",sortOrder:0}` (как сейчас) | N/A |
| Concurrent INSERT same `(application_id, category_code)` | Дубль кода в одном submit | UNIQUE violation 23505 → existing race-handling в `creator_application_consents` style | repo транслирует в domain-error если потребуется |
| Orphan `city` value в существующих данных | в `creator_applications.city = "deleted_city"` | Миграция падает на ADD FK, требуется ручной cleanup | fail-loud, fix-forward |

</frozen-after-approval>

## Code Map

- `backend/migrations/<NEW>_dictionaries_code_pk.sql` -- forward-only goose-миграция всей реструктуризации
- `backend/internal/repository/dictionary.go` -- drop `ID` из row + `DictionaryColumnID`; `selectColumns` пересчитывается автоматически
- `backend/internal/repository/creator_application_category.go` -- field `CategoryID`→`CategoryCode`, drop JOIN в `ListByApplicationID`, новая константа `CreatorApplicationCategoryColumnCategoryCode`
- `backend/internal/repository/creator_application.go` -- field `City`→`CityCode`, константа `CreatorApplicationColumnCity`→`CreatorApplicationColumnCityCode`, тег `db:"city_code" insert:"city_code"`
- `backend/internal/service/creator_application.go` -- `resolveCategoryIDs`→`resolveCategoryCodes` (возвращает `[]string` codes напрямую), удаление маппинга `byCode → row.ID`, `in.City`→`in.CityCode`
- `backend/internal/domain/creator_application.go` -- `City` field в `CreatorApplicationInput` и `CreatorApplicationDetail` → `CityCode`
- `backend/internal/handler/creator_application.go` -- маппинг: `req.City` (API не меняется) → `domain.CityCode`; `d.City` → `d.CityCode` в `mapDetail`; `resolveCity(d.CityCode, …)`
- `backend/internal/repository/dictionary_test.go`, `creator_application_category_test.go`, `creator_application_test.go` -- обновить SQL-литералы и mock data под новую схему
- `backend/internal/service/creator_application_test.go` -- mocks для `resolveCategoryCodes`, fixtures `CategoryCode`, `CityCode`
- `backend/internal/handler/creator_application_test.go` -- fixtures `City: …` остаётся в API request; в domain-input ожидаем `CityCode`
- `backend/e2e/creator_application/creator_application_test.go` -- проверить race-test на `(application_id, category_code)`
- `docs/standards/backend-repository.md` -- секцию «Идентификатор словаря» заменить на однострочник
- `make generate-mocks` -- regen после переименований интерфейсов

## Tasks & Acceptance

**Execution:**
- [x] `backend/migrations/20260430233030_dictionaries_code_pk.sql` -- создана через `make migrate-create NAME=dictionaries_code_pk`. Up: backfill category_code через JOIN на categories(id); drop FK + drop column category_id; drop UNIQUE(application_id, category_id); добавить UNIQUE(application_id, category_code); DROP UUID id у categories+cities, ADD PRIMARY KEY(code); RENAME column creator_applications.city → city_code; ADD FK creator_applications.city_code → cities(code); ADD FK creator_application_categories.category_code → categories(code). Down: явный RAISE EXCEPTION (UUID невозможно восстановить).
- [x] `backend/internal/repository/dictionary.go` -- удалены `DictionaryColumnID` и поле `ID` из `DictionaryEntryRow`.
- [x] `backend/internal/repository/creator_application_category.go` -- константа/поле/тег переименованы на `category_code`; `ListByApplicationID` теперь делает single-table SELECT без JOIN.
- [x] `backend/internal/repository/creator_application.go` -- `CreatorApplicationColumnCity`→`...ColumnCityCode`, поле `City`→`CityCode` с тегами `db:"city_code" insert:"city_code"`.
- [x] `backend/internal/domain/creator_application.go` -- `City` field в `CreatorApplicationInput` и `CreatorApplicationDetail` → `CityCode`.
- [x] `backend/internal/service/creator_application.go` -- `resolveCategoryIDs`→`resolveCategoryCodes`, возвращает codes без UUID-маппинга; `in.CityCode` везде; `CreatorApplicationCategoryRow{CategoryCode: code}`.
- [x] `backend/internal/handler/creator_application.go` -- `domain.CreatorApplicationInput{… CityCode: req.City}`; `resolveCity(d.CityCode, cityByCode)`. API-контракт не изменён.
- [x] `make generate-mocks` -- mocks перегенерированы.
- [x] Unit-тесты затронутых пакетов обновлены (SQL-литералы, fixtures `CategoryCode`/`CityCode`, mock expectations). Дополнены тесты для blank-only/duplicate/repo-error путей `resolveCategoryCodes` чтобы удержать coverage gate ≥ 80% per-method.
- [x] `backend/e2e/creator_application/creator_application_test.go` -- `validRequestMap` обновлён, чтобы посылать city code (`almaty`) вместо имени. Race-тест на регулярный UNIQUE не требуется по standards (только для partial unique).
- [x] `docs/standards/backend-repository.md` -- секция «Идентификатор словаря» заменена на однострочник про целевую модель.

**Acceptance Criteria:**
- Given чистая БД и `make migrate-up`, when goose применяет новую миграцию, then `categories` и `cities` имеют PK на `code` (без `id`), `creator_application_categories.category_code` — TEXT FK на `categories(code)`, `creator_applications.city_code` — TEXT FK на `cities(code)`.
- Given данные в staging с валидными `category_id` и `city`, when миграция применяется, then `category_code` корректно backfilled из JOIN, `city` переименован в `city_code` без потери данных.
- Given submit на `/creators/applications` с валидным `city: "almaty"` и `categories: ["beauty"]`, when запрос проходит, then `creator_applications.city_code='almaty'`, `creator_application_categories.category_code='beauty'`, ответ 201.
- Given submit с unknown category code, when запрос проходит, then 422 `UNKNOWN_CATEGORY` без обращения к UUID-маппингу.
- Given application с деактивированной категорией, when GET `/creators/applications/{id}`, then response категории fallback'ит через handler-функцию `resolveCategory` (поведение не регрессирует).
- Given `make test-unit-backend && make test-unit-backend-coverage && make test-e2e-backend && make lint-backend`, then все таргеты зелёные.
- Given `git grep -E '\bcategory_id\b|\bDictionaryColumnID\b|\.ID\b.*Dictionary'` по `backend/internal/`, then нет совпадений (вне сгенерированных файлов).
- Given `docs/standards/backend-repository.md` после правки, then секция «Идентификатор словаря» — один абзац ≤ 2 предложений про целевую модель.

## Spec Change Log

<!-- empty until first bad_spec loopback -->

## Verification

**Commands:**
- `cd backend && go build ./...` -- expected: clean compile (0 errors)
- `make lint-backend` -- expected: pass (golangci-lint clean)
- `make test-unit-backend` -- expected: all green с `-race`
- `make test-unit-backend-coverage` -- expected: 80% gate проходит на всех слоях
- `make migrate-reset && make migrate-up` -- expected: все миграции от нуля до новой проходят
- `make test-e2e-backend` -- expected: e2e creator_application/dictionary suite зелёные
- `git grep -nE 'DictionaryColumnID|category_id|\\.ID.*Dictionary' backend/internal/ ':!*.gen.go' ':!**/mocks/**'` -- expected: empty
- `git grep -nE '"city"' backend/internal/ ':!*.gen.go' ':!**/mocks/**' ':!*_test.go'` -- expected: только в handler/openapi-маппинге (API field), не в repo/service

## Suggested Review Order

> Ссылки в формате `path:line` — Ctrl+B / ⌘+B в GoLand, либо Goto File (Ctrl+Shift+N) с вставкой пути.

**Schema migration (the foundation)**

- Single forward-миграция: backfill `category_code` через JOIN, drop UUID id, rename city → city_code, добавить FK.
  `backend/migrations/20260430233030_dictionaries_code_pk.sql:1`

**Service layer — symmetric dictionary validation**

- Новая `resolveCityCode` симметрично `resolveCategoryCodes` — закрывает unknown city → 422 (вместо 500 от FK).
  `backend/internal/service/creator_application.go:360`

- `resolveCategoryCodes` теперь возвращает codes напрямую без UUID-маппинга и принимает общий `dictRepo`.
  `backend/internal/service/creator_application.go:318`

- Один `NewDictionaryRepo(tx)` обслуживает оба lookup'а внутри транзакции.
  `backend/internal/service/creator_application.go:97`

**Repository layer — single-table reads и code-FK**

- `ListByApplicationID` теперь single-table SELECT без JOIN.
  `backend/internal/repository/creator_application_category.go:65`

- `DictionaryEntryRow` без поля ID — словарь читается code/name/sort_order/active.
  `backend/internal/repository/dictionary.go:33`

- `CreatorApplicationRow.CityCode` с тегами `db:"city_code" insert:"city_code"`.
  `backend/internal/repository/creator_application.go:54`

**Domain — code carries through the type**

- Новый код ошибки `CodeUnknownCity = "UNKNOWN_CITY"` симметрично `CodeUnknownCategory`.
  `backend/internal/domain/creator_application.go:77`

- `CityCode` в `CreatorApplicationInput` и `CreatorApplicationDetail` — domain хранит код.
  `backend/internal/domain/creator_application.go:113`

**Handler — API contract intact**

- API-поле `city` маппится на `domain.CityCode`; `resolveCity(d.CityCode, …)` — handler-fallback не регрессирует.
  `backend/internal/handler/creator_application.go:48`

**Standards — целевая модель в одну строку**

- Секция «Идентификатор словаря» переписана: code как PK, FK на потребителях — `<entity>_code`.
  `docs/standards/backend-repository.md:128`

**Тесты**

- Новые сценарии `resolveCityCode`: unknown city → 422, repo error → 500-wrap; helper `expectCityLookupSuccess`.
  `backend/internal/service/creator_application_test.go:411`

- E2E raw-map шлёт код города `almaty` (а не имя «Алматы»).
  `backend/e2e/creator_application/creator_application_test.go:555`

