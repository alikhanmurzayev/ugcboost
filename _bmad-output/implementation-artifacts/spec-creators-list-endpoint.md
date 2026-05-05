---
title: 'Бэк-ручка списка креаторов (POST /creators/list, chunk 1)'
type: 'feature'
created: '2026-05-05'
status: 'in-progress'
baseline_commit: 'a3a56fe58579aaa41693d748c93f2debc5888fbc'
context:
  - 'docs/standards/'
  - '_bmad-output/planning-artifacts/campaign-roadmap.md'
  - '_bmad-output/implementation-artifacts/intent-creators-list-endpoint.md'
---

<frozen-after-approval reason="human-owned intent — do not modify unless human renegotiates">

## Intent

**Problem:** В админке нет списка approved-креаторов; без него нельзя выбирать креаторов в кампании (chunk 1 `campaign-roadmap.md`).

**Approach:** Новая admin-only ручка `POST /creators/list` — пагинация/фильтры/сортировка/поиск 1:1 шаблона `POST /creators/applications/list` (см. `backend/api/openapi.yaml` § `/creators/applications/list`, `backend/internal/repository/creator_application.go` § `List`, `backend/internal/service/creator_application.go` § `List`, `backend/internal/handler/creator_application.go` § `ListCreatorApplications`). Read-only, без аудита, без миграции. Coupled-рефактор: `CreatorSocialRepo.ListByCreatorID` / `CreatorCategoryRepo.ListByCreatorID` заменяются на `ListByCreatorIDs(ctx, ids) → map[creatorID]…` (шаблон `creator_application_social.ListByApplicationIDs` / `creator_application_category.ListByApplicationIDs`); существующий `CreatorService.GetByID` переезжает на batched-форму с массивом из одного id.

### Item-shape (response `data.items[]`)

| Поле | Тип | Источник |
|---|---|---|
| `id` | uuid | `creators.id` |
| `lastName` | string | `creators.last_name` |
| `firstName` | string | `creators.first_name` |
| `middleName` | string \| null | `creators.middle_name` |
| `iin` | string | `creators.iin` |
| `birthDate` | date | `creators.birth_date` (FE сам считает возраст) |
| `phone` | string | `creators.phone` |
| `city` | `DictionaryItem {code, name, sortOrder}` | `creators.city_code` + hydrate; deactivated → fallback `(code, code, 0)` |
| `categories[]` | `DictionaryItem[]` | `creator_categories` ↔ `categories`, sort `sort_order, code` |
| `socials[]` | `[{platform, handle}]` | `creator_socials`, sort `platform, handle` |
| `telegramUsername` | string \| null | `creators.telegram_username` |
| `createdAt` / `updatedAt` | date-time | `creators.*` |

**НЕ включаем в response:** `address, category_other_text, telegram_user_id, telegram_first_name, telegram_last_name, source_application_id` (только в `GET /creators/{id}`). Без `telegram_linked`-проекции (у approved-креатора TG привязан по инварианту onboarding'а — всегда true).

### Search

Substring ILIKE по: `creators.{last_name, first_name, middle_name, iin, phone, telegram_username}` + EXISTS по `creator_socials.handle`. Trim+lower; пустое/whitespace → фильтр игнорируется. Wildcards `% _ \\` экранируются (паттерн escapeLikeWildcards из `creator_application.go`); каждое ILIKE с `ESCAPE '\\'`. `phone` ищется substring (admin может вставить `77012`, `+77012345678` или хвост).

### Sort enum + резолюция в SQL

Enum `CreatorListSortField`: `created_at | updated_at | full_name | birth_date | city_name`. Tie-breaker `id ASC` хвостом ко всем веткам.

- `created_at` / `updated_at` / `birth_date` — одна колонка `DIR`, `id ASC`
- `full_name` — `last_name DIR, first_name DIR, middle_name DIR, id ASC` (NULL middle_name ордерится Postgres-дефолтом: NULLS LAST на ASC, FIRST на DESC)
- `city_name` — `cities.name DIR, id ASC` (LEFT JOIN `cities` подключается **только** в этой ветке)

### Pagination + bounds

Reuse `PaginationInput` (`page≥1`, `perPage 1..200`, оба required). `search.maxLength: 128`. Filter-array sizes (cities, categories) — bounded как в applications (`CreatorApplicationListFilterArrayMax = 50`).

### CreatorListRow (repo Row-структура)

Колонки: `id, last_name, first_name, middle_name, iin, birth_date, phone, city_code, telegram_username, created_at, updated_at`. Без `address, category_other_text, telegram_user_id, telegram_first_name, telegram_last_name, source_application_id`. Без derived-колонок (нет `telegram_linked`).

## Boundaries & Constraints

**Always:**
- Все стандарты `docs/standards/` грузятся целиком и применяются как hard rules (slои, codegen, security, naming, testing).
- AuthZ-check **первым**: до любых DB-чтений.
- POST (не GET) из-за PII (`iin`, `phone`, `telegram_username`, ФИО) в search/response.
- Repo-children загружаются batched-методом по `creator_id IN (...)` (без N+1).
- Sort и order **required**, без серверных дефолтов; tie-breaker `id ASC` хвостом ко всем сортировкам.
- Wildcard-метасимволы `% _ \\` экранируются в `search` (ESCAPE `\\`); пустой/whitespace search игнорируется.
- City/categories гидрятся через `DictionaryRepo.GetActiveByCodes`; deactivated → fallback `(code, code, 0)`.
- Coverage gate ≥80% per-method; e2e `t.Parallel()`, cleanup через `RegisterCreatorCleanup`.

**Ask First:**
- Любое изменение item-shape (добавить/убрать поля).
- Расширение или сужение search-колонок.
- Любое отступление от паттерна `creator_applications/list` (валидация, sort-резолюция, query-форма).

**Never:**
- Миграция БД (схема `creators` / `creator_socials` / `creator_categories` уже на месте).
- Audit-log запись (read-only).
- Hardcoded дефолты пагинации/сортировки на сервере.
- Cursor-pagination, gender, LiveDune-поля (rating/qualityIndicator/metrics), orders-метрики (completedOrders/activeOrders), `signedAt` — отложено в roadmap.
- Deprecated-обёртки старых `ListByCreatorID` (single-id call-site всего один — рефактор полный).
- Прямой `pgx` в repo вне тестов; manual struct для request/response (только сгенерированные).

## I/O & Edge-Case Matrix

| Сценарий | Вход | Поведение | Ошибка |
|---|---|---|---|
| Happy path | admin, валидный body, есть данные | 200, `{data: {items, total, page, perPage}}` с lean-полями креатора | — |
| No auth | без bearer | 401 | bearer required |
| Manager / brand | bearer не admin | 403, `Forbidden` | до DB-чтений |
| Bad pagination | page<1 или perPage>200 | 422 | `CodeValidation` |
| Unknown sort/order | `sort="bogus"` или `order="up"` | 422 | `CodeValidation` |
| Empty search | `search=""` или whitespace | фильтр игнорируется | — |
| Wildcard в search | `search="100%"` | литеральный substring match | — |
| Search hit by phone | `search="77012"` | match на `phone` substring | — |
| Filter cities=[] / categories=[] | пустые массивы | фильтр не применяется | — |
| Filter cities/categories с deactivated кодом | dict-row active=false | возвращаем `(code, code, 0)` в hydrate | — |
| Page > total pages | beyond last | 200, `items=[], total=N` | — |
| Concurrent approve+list | сразу после approve | новый креатор виден в следующей странице | — (read-only) |

</frozen-after-approval>

## Code Map

- `backend/api/openapi.yaml` — добавить `POST /creators/list` + схемы (`CreatorsListRequest`, `CreatorListItem`, `CreatorsListData`, `CreatorsListResult`, `CreatorListSortField`); генерация через `make generate-api`.
- `backend/internal/domain/creator.go` — добавить domain-types (`CreatorListInput`, `CreatorListPage`, `CreatorListItem`, sort consts, bounds-консты, validators) по шаблону `creator_application.go` § sort fields / list bounds.
- `backend/internal/repository/creator.go` — добавить `CreatorRepo.List`, `CreatorListParams`, `CreatorListRow`; sort/filter helpers (mirror `creator_application.go`).
- `backend/internal/repository/creator_social.go` — refactor: удалить `ListByCreatorID`, добавить `ListByCreatorIDs(ctx, ids) → map[string][]*CreatorSocialRow` (шаблон `creator_application_social.go`).
- `backend/internal/repository/creator_category.go` — то же: `ListByCreatorIDs(ctx, ids) → map[string][]string` (шаблон `creator_application_category.go`).
- `backend/internal/service/creator.go` — добавить `List(ctx, in CreatorListInput) → *CreatorListPage`; рефактор `GetByID` на batched-форму с `[]string{creatorID}`.
- `backend/internal/handler/creator.go` — добавить `(s *Server) ListCreators(ctx, req)`; AuthZ-check первым; trim/lower search; маппинг через сгенерированные `api.*` типы.
- `backend/internal/handler/server.go` — расширить интерфейс `AuthzService` методом `CanViewCreators(ctx) error`.
- `backend/internal/authz/creator.go` — `CanViewCreators(ctx) error` (admin-only, шаблон `CanViewCreator`).
- `backend/e2e/testutil/creator.go` — новый helper `SetupApprovedCreator(t, opts...) ApprovedCreatorFixture` (реюз `SetupCreatorApplicationInModeration` + admin-approve через `POST /creators/applications/{id}/approve` + `RegisterCreatorCleanup`).
- `backend/e2e/creators/list_test.go` — новый e2e-файл; godoc на русском, нарратив (см. `docs/standards/backend-testing-e2e.md`).

## Tasks & Acceptance

**Execution:**
- [x] `backend/api/openapi.yaml` -- добавить путь и схемы для `/creators/list` -- contract-first source of truth
- [x] прогнать `make generate-api` -- регенерация Go-server, Go-e2e-client, TS-schemas
- [x] `backend/internal/domain/creator.go` -- добавить `CreatorListInput / CreatorListPage / CreatorListItem`, sort consts (`CreatorSortCreatedAt | UpdatedAt | FullName | BirthDate | CityName`), bounds-консты, sort/order валидаторы -- single source of truth для list-API
- [x] `backend/internal/repository/creator_social.go` (+`_test.go`) -- удалить `ListByCreatorID`, добавить `ListByCreatorIDs`; переписать тесты на batched-форму -- DRY с child repos applications
- [x] `backend/internal/repository/creator_category.go` (+`_test.go`) -- то же
- [x] `backend/internal/repository/creator.go` (+`_test.go`) -- добавить `CreatorListRow`, `CreatorListParams`, `CreatorRepo.List`; SQL-helpers (`apply…Filters`, `apply…Order`, `…TelegramJoin`, `…CityJoin`); pgxmock-тесты на page-q + count-q + tie-breaker + ESCAPE-search
- [x] `backend/internal/authz/creator.go` (+`_test.go`) -- `CanViewCreators(ctx) error` admin-only + 3 unit-теста (admin pass, manager forbidden, no-auth forbidden)
- [x] `backend/internal/handler/server.go` -- расширить `AuthzService` интерфейс `CanViewCreators`
- [x] `backend/internal/service/creator.go` (+`_test.go`) -- `List` (read-only, без `WithTx`), рефактор `GetByID` на `ListByCreatorIDs(ctx, []string{id})` + `m[id]` извлечение; пакетные `DictionaryRepo.GetActiveByCodes` для cities/categories страницы; deactivated → fallback `(code, code, 0)`
- [x] `backend/internal/handler/creator.go` (+`_test.go`) -- `ListCreators` (AuthZ первым, trim/lower search, маппинг через `api.*` типы); unit-тесты на 401/403/422/happy/empty/dict-fallback
- [x] прогнать `make generate-mocks` -- регенерация моков для новых интерфейсных методов
- [x] `backend/e2e/testutil/creator.go` -- `SetupApprovedCreator` helper (composable, реюз existing setup + approve)
- [x] `backend/e2e/creators/list_test.go` -- e2e со всеми сценариями из I/O Matrix + sort across each enum value asc+desc + pagination boundaries + filter cross-product (city × categories × age × date) хотя бы одним кейсом каждый

**Acceptance Criteria:**
- Given admin, when POST `/creators/list` с валидным body и существуют 3+ approved-креатора, then 200 + items lean-shape, total корректный, items упорядочены по запрошенному `sort/order`+id-tie-breaker.
- Given non-admin (manager / no-auth), when POST `/creators/list`, then 403/401 с timing identical для «есть креаторы / нет».
- Given валидный body с `search` ≤128 символов, содержащим wildcard `%/_`, when POST, then результат содержит только литеральный substring match.
- Given filter `cities=[deactivated_code]`, when POST, then в `city.name` возвращается code (fallback), не 500.
- Given approved-креатор seed через `SetupApprovedCreator`, when e2e дёргает `/creators/list`, then `RegisterCreatorCleanup` срабатывает по LIFO и удаляет `creator_id` после теста.
- Given существующий `GET /creators/{id}`, when рефактор `ListByCreatorIDs` мержится, then ответ ручки полностью совпадает с прежним (regression-проверка через существующий `creators/get_test.go`).
- Given coverage gate, when `make test-unit-backend-coverage`, then ни один новый identifier в `handler/service/repository/authz` не падает <80%.

## Spec Change Log

## Verification

**Commands:**
- `make generate-api` — expected: успешно, generated-файлы обновлены.
- `make generate-mocks` — expected: успешно.
- `make build-backend` — expected: успешно.
- `make lint-backend` — expected: 0 findings.
- `make test-unit-backend` — expected: green.
- `make test-unit-backend-coverage` — expected: пусто, gate ≥80% per-method.
- `make test-e2e-backend` — expected: green; `creators/list_test.go` среди прогнанных.

**Manual checks:**
- `git diff backend/api/openapi.yaml` — путь и схемы добавлены, ручных правок в `*.gen.*` нет.
- `grep -R "ListByCreatorID\b" backend/internal/` — пусто (старый single-id метод удалён).
