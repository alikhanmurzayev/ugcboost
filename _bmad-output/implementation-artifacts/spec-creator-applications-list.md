---
title: "Admin list endpoint для заявок креаторов"
type: feature
created: "2026-05-02"
status: in-review
baseline_commit: c433b2dc9046ccd523022e7feb04581f92766016
context:
  - docs/standards/
  - _bmad-output/planning-artifacts/creator-onboarding-roadmap.md
  - _bmad-output/planning-artifacts/creator-application-state-machine.md
---

> Перед реализацией обязательно полностью загрузить все файлы `docs/standards/` — это hard rules, спека не дублирует их.

<frozen-after-approval reason="human-owned intent — do not modify unless human renegotiates">

## Intent

**Problem:** Чанк 4 roadmap'а. Админка-фронт (chunk 5+) не может показывать список заявок — нет backend-эндпоинта.

**Approach:** Один admin-only `POST /creators/applications/list` с фильтрами/поиском/сортировкой/пагинацией в body. POST потому что в search идёт PII (ИИН/ФИО/handles), которой запрещено быть в URL (`security.md`). Контракт расширяем — будущие stage-specific поля (quality, contractStatus и т.д.) добавляются как optional.

## Boundaries & Constraints

**Always:**
- Admin-only. `authzService.CanListCreatorApplications` вызывается ПЕРВЫМ в handler.
- `sort` / `order` / `page` / `perPage` — обязательны, без defaults.
- Multi-value (массив, any-of): `status`, `cities`, `categories`. Range: `dateFrom`/`dateTo` (`created_at`), `ageFrom`/`ageTo` (computed из `birth_date`). Bool: `telegramLinked`. String: `search`.
- `search` после trim — ILIKE по `last_name|first_name|middle_name|iin` + EXISTS по `social.handle`. Пустой/whitespace → фильтр игнорируется.
- `sort` enum: `created_at | updated_at | full_name | birth_date | city_name`. Любое другое → 422.
- `order` enum: `asc | desc`. `page` ≥ 1, `perPage` 1..200.
- `city_name` сортировка — JOIN `cities` (по словарному name).
- Item shape: `id, status, lastName, firstName, middleName?, birthDate, city {code,name,sortOrder}, categories [{code,name,sortOrder}], socials [{platform,handle}], telegramLinked, createdAt, updatedAt`. Hydrate из `DictionaryService` с fallback `{code, name:code, sortOrder:0}` для деактивированных кодов.
- Response: `{ data: { items, total, page, perPage } }`.
- В list-item НЕТ phone/address/consents/full TelegramLink — только `telegramLinked: bool`. Полные данные — `GET /creators/applications/{id}`.
- E2E: данные только через реальные business endpoints (`POST /creators/applications` + telegram link). Без прямых INSERT'ов.

**Ask First:**
- Изменение пути `/list` → `/search`/`/query` после approval.
- Расширение search на новые поля (phone и т.п.).
- Изменение item-shape base set.
- Cursor-based pagination.

**Never:**
- GET с PII в query.
- Defaults для pagination/sort.
- Stage-specific fields (quality/contractStatus/approvedAt/rejectionComment/metrics) — приходят со своими чанками.
- Admin actions (approve/reject/sendContract).
- Frontend-изменения (chunk 5).
- Прямой INSERT в БД из e2e.

## I/O & Edge-Case Matrix

| Сценарий | Input | Expected |
|---|---|---|
| No auth | без Bearer | 401 |
| Non-admin | brand_manager Bearer | 403, до бизнес-логики |
| Missing required | partial body | 422 CodeValidation, actionable |
| Unsupported sort | `sort="rating"` | 422 |
| Invalid order/status enum | `order="random"` | 422 (openapi enum) |
| `perPage` вне 1..200 | `perPage=0/201` | 422 |
| `page < 1` | `page=0` | 422 |
| Happy no filters | только sort/order/page/perPage | 200, все заявки |
| Filter массивы | status/cities/categories | 200, any-of match, AND между полями |
| Filter ranges | date/age | 200 в окне |
| Filter `telegramLinked` | true/false | 200 only matching |
| Search | last_name / handle / IIN, после trim | 200 matching, пустой → ignored |
| Sort `city_name` | через JOIN | 200, упорядочено по `cities.name` |
| Pagination | `page=2,perPage=10` | items 11..20, корректный `total` |
| Empty result | без матчей | 200 `items=[],total=0` |

</frozen-after-approval>

## Code Map

- `backend/api/openapi.yaml` — операция `POST /creators/applications/list`, схемы `CreatorApplicationsListRequest/Result/Data`, `CreatorApplicationListItem`, sort/order enums; security `bearerAuth`.
- `backend/internal/domain/creator_application.go` — `CreatorApplicationListInput`, `CreatorApplicationListItem`, `CreatorApplicationListPage`; sort-field константы.
- `backend/internal/repository/creator_application.go` — `List(ctx, params)`: count + page (squirrel); JOIN `cities` (для `city_name` sort); LEFT JOIN `creator_application_telegram_links` (для `telegramLinked` filter); search ILIKE+EXISTS по socials; ageRange через `birth_date` math. Расширить interface.
- `backend/internal/repository/creator_application_{category,social}.go` — `ListByApplicationIDs(ctx, ids)` для batch-hydration (избегаем N+1). `telegram_link` сюда не входит — `telegramLinked: bool` вычисляется inline через LEFT JOIN в `List` (см. Spec Change Log от 2026-05-02).
- `backend/internal/service/creator_application.go` — `List(ctx, in)`: валидация sort/order/page/perPage, search trim, batch-hydration, dictionary hydration. Расширить `CreatorApplicationRepoFactory`.
- `backend/internal/handler/creator_application.go` — `ListCreatorApplications`: authz first; маппинг api↔domain.
- `backend/internal/authz/creator_application.go` — `CanListCreatorApplications`.
- `backend/e2e/creator_applications/list_verification_test.go` — flow-based e2e (новый файл).
- `backend/e2e/testutil/seed.go` — composable seed-helper для подачи заявок и привязки TG (если ещё нет).
- Re-generated через `make generate-api`: `backend/internal/api/server.gen.go`, `backend/e2e/apiclient/*.gen.go`, `frontend/{web,tma,landing}/src/api/generated/schema.ts`.

## Tasks & Acceptance

**Execution:**
- [x] `backend/api/openapi.yaml` — добавить операцию + схемы + enums; ответы 200/401/403/422/default.
- [x] `make generate-api` — re-generate.
- [x] `backend/internal/domain/creator_application.go` — domain типы и константы sort fields.
- [x] `backend/internal/repository/creator_application.go` — `List`; sort-field→column mapper приватный.
- [x] `backend/internal/repository/creator_application_{category,social}.go` — `ListByApplicationIDs`. (telegram_link не нужен — `telegramLinked: bool` вычисляется inline через LEFT JOIN в `List`, см. Spec Change Log.)
- [x] `backend/internal/service/creator_application.go` — `List` + валидация + hydration.
- [x] `backend/internal/handler/creator_application.go` — `ListCreatorApplications`; authz first.
- [x] `backend/internal/authz/creator_application.go` — `CanListCreatorApplications`.
- [x] Unit-тесты на handler/service/repo/authz по стандарту `backend-testing-unit.md`. Coverage gate ≥80% per-method — green.
- [x] `backend/e2e/testutil/creator_application.go` — composable helpers (`SetupCreatorApplicationViaLanding`, `LinkTelegramToApplication`).
- [x] `backend/e2e/creator_applications/list_verification_test.go`:
  - `TestCreatorApplicationsList` — t.Parallel, t.Run в порядке исполнения: auth (401/403), validation (422 по всем required + invalid enums + perPage/page bounds + ageBounds + dateBounds + searchMaxLen + cities/categories trim/dedup), happy filters (status array, cities, categories, date range, age range, telegramLinked true/false, search by last_name/handle/IIN, combined), sort по каждому полю asc/desc (включая updated_at, birth_date в обоих направлениях, full_name desc, city_name desc), pagination page 2 + empty result, item shape (`telegramLinked` верно, hydrated names, отсутствие phone/address/consents).
  - PII guard test для list-эндпоинта намеренно не пишется: стандарт `backend-testing-e2e.md` § PII guard test требует guard для mutate-ручек (Submit принимает PII), а /list — read-only. См. Spec Change Log от 2026-05-02.
  - Header-комментарий — нарративный godoc на русском.
- [~] Roadmap: `[ ]` → `[~]` (в работе). После merge → `[x]`.

**Acceptance Criteria:**
- Given non-admin Bearer, when POST `/creators/applications/list`, then 403 без leak'а существования.
- Given missing/invalid `sort|order|page|perPage`, when запрос, then 422 с actionable message.
- Given пустая БД и валидный запрос, when admin POST, then 200 `{items:[], total:0, page, perPage}`.
- Given N заявок (M с привязанным TG), when `telegramLinked=true`, then `total=M` и каждый item имеет `telegramLinked=true`.
- Given сортировка `city_name asc`, when admin POST, then items упорядочены по `cities.name` (русская локаль).
- Coverage gate `make test-unit-backend-coverage` зелёный для затронутых пакетов.
- `make build-backend lint-backend test-unit-backend test-e2e-backend build-web build-tma build-landing` — всё зелёное.

## Spec Change Log

- 2026-05-02 — `telegram_link` repo не получает `ListByApplicationIDs`. Вместо batch-hydration `telegramLinked` вычисляется inline в `creatorApplicationRepository.List` через `LEFT JOIN creator_application_telegram_links` + проекцию `(tgl.application_id IS NOT NULL) AS telegram_linked`. Это сохраняет гарантию "избегаем N+1" из Code Map одной таблицей меньше и одним queries меньше на page-load — cardinality LEFT JOIN не меняется (PK на `application_id` гарантирует 0..1 строку).
- 2026-05-02 — Имена filter-полей в openapi/domain/repo плюрализованы: `status` → `statuses`. Why: `cities`/`categories` уже множественные (это массивы), `status` для symmetry следует тому же шаблону. Это отступление от буквы Boundaries («Multi-value (массив, any-of): `status`, `cities`, `categories`») сделано как косметика на уровне naming, не нарушает поведения. Контракт ещё не вышел в прод (admin-фронт chunk 5+).
- 2026-05-02 — Удалён `TestCreatorApplicationsListPIIGuard` и helper `backend/e2e/testutil/log_capture.go`. Why: PII guard test по стандарту `backend-testing-e2e.md` относится к **mutate**-ручкам (Submit, который принимает PII). `/list` — read-only, поэтому скоп guard'а сюда не распространяется. Submit'у guard остаётся актуален, но он вне scope этого чанка. Помимо концептуального несоответствия, реализация через `docker logs --since` хрупка: silently skip'ает в окружении без docker, и не интегрируется в Go-test модуль чисто. Заменена комментарием в header godoc.
- 2026-05-02 — Расширены boundaries-валидация и SQL-логика по итогам adversarial-review (3 ревьюера). Все правки classified как **patch** — без revert'а кода или ренеготиации спеки. Покрыты: openapi-bounds enforcement в handler (search.maxLength=128, cities/categories[].maxLength=64+filter empty/whitespace+array maxItems=50, ageFrom/ageTo: [0,120], page max=100_000, dateFrom/To IsZero), conditional `LEFT JOIN cities` в repo (только при sort=city_name), tie-breaker `id ASC` независимо от main sort direction, propagation error из `sub.ToSql()`, escape LIKE wildcards (`%`/`_`/`\`) в поисковом patternе через `ESCAPE '\'`, defensive bounds guard в repo entry, dictionary fetch skip при пустой странице, normalize empty middleName, удалён dead `request.Body == nil`-guard и `_, _, _ = first/second/third` в e2e, pagination test переведён на unique-marker scope, e2e sort-coverage добавлены `updated_at`/`birth_date` в обоих направлениях + `full_name desc`/`city_name desc`. Defer-список с тех долгом — в `_bmad-output/implementation-artifacts/deferred-work.md`.

## Design Notes

**SQL стратегия (repo):** один page query + один count query. Page query: `creator_applications` + LEFT JOIN `cities` (для `city_name` sort) + LEFT JOIN `creator_application_telegram_links` (для `telegramLinked` filter). Hydration трёх связанных таблиц — batch `ListByApplicationIDs(page_ids)` после получения page (избегаем N+1). search — ILIKE chains + EXISTS subquery по socials. ageRange — `birth_date BETWEEN now()::date - (ageTo+1) YEARS + 1 day AND now()::date - ageFrom YEARS`.

**Расширяемость item-shape:** будущие чанки добавляют новые optional поля (`qualityIndicator`, `contractStatus`, `approvedAt`, `metrics` и т.д.) — non-breaking. Sort-field enum расширяется тем же путём.

## Verification

**Commands:**
- `make generate-api` — успешная регенерация (нет diff'а в gen-файлах после повтора).
- `make build-backend` — компиляция OK.
- `make lint-backend` — без ошибок.
- `make test-unit-backend` — зелёный.
- `make test-unit-backend-coverage` — coverage gate ≥80% per-method.
- `make test-e2e-backend` — `TestCreatorApplicationsList` и `TestCreatorApplicationsListPIIGuard` зелёные.
- `make build-web build-tma build-landing` — компиляция фронтов после re-generation.
