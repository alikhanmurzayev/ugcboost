---
title: "Admin counts endpoint + API contracts hygiene"
type: feature
created: "2026-05-02"
status: done
baseline_commit: 74e52ae7c0b26249b35e967d8564936d34ac65c6
context:
  - docs/standards/
  - _bmad-output/planning-artifacts/creator-onboarding-roadmap.md
  - _bmad-output/planning-artifacts/creator-application-state-machine.md
---

> Перед реализацией обязательно полностью загрузить все файлы `docs/standards/` — это hard rules, спека не дублирует их.

> **Multi-goal explicit override:** этот PR содержит chunk 5 (counts endpoint) + 9 side-quests по гигиене openapi.yaml. Пользователь явно одобрил совмещение.

<frozen-after-approval reason="human-owned intent — do not modify unless human renegotiates">

## Intent

**Problem:** (1) Chunk 5 roadmap'а — админка-фронт не может показывать бейдж нотификаций без эндпоинта счётчиков. (2) `openapi.yaml` накопил структурный tech-debt: задублированы enum'ы (статусы 3×, роли, consent), response-блоки (`default: Unexpected error` 19×, `403: Forbidden` 9×), object-схемы (`CreateBrandRequest`≡`UpdateBrandRequest`, `code+name+sortOrder` 3×), и pagination-параметры/тела повторяются inline во всех list-эндпоинтах. Каждое расширение системы требует править N мест.

**Approach:** В одном PR реализовать новый `GET /creators/applications/counts` (массив пар `{status, count}`, sparse — статусы без рядов отсутствуют) и привести openapi.yaml в порядок: вынести в shared `$ref` все обнаруженные дубли, унифицировать pagination через shared `components/parameters/` (query, snake_case по convention) и `components/schemas/PaginationInput` (body, camelCase). После этого добавление статуса/роли/нового error response = one-place change.

## Boundaries & Constraints

**Always:**
- *Counts endpoint:*
  - Admin-only `GET /creators/applications/counts` без тела и query-параметров. `authzService.CanGetCreatorApplicationsCounts` вызывается ПЕРВЫМ в handler. Отдельный authz-метод (не переиспользуем `CanListCreatorApplications`).
  - Response shape: `{ "data": { "items": [{"status": "<enum>", "count": <int64>}, ...] } }`. **Sparse: только статусы с рядами; статусы с 0 заявок не возвращаются.** Description в openapi явно фиксирует это поведение для фронта.
  - Service конвертирует `map[string]int64` от repo в slice пар, сортирует по `status` alphabetically для детерминизма (тесты ассертят exact порядок).
  - Один SQL: `SELECT status, COUNT(*) FROM creator_applications GROUP BY status`. Без CASE-агрегатов.
  - Read-only, без audit-log, без кеша.
- *API hygiene side-quests:*
  - Все 8 рефакторингов делаются ZERO-DIFF с точки зрения wire-контракта **за исключением** audit-logs query: `actor_id`/`entity_type`/`entity_id`/`date_from`/`date_to`/`per_page` → camelCase. Безопасно: фронт ещё не консьюмит endpoint.
  - Shared schemas/responses/parameters создаются в `components/`, inline-определения заменяются на `$ref` без изменения значений.
  - **Convention (фиксируется в этом PR): camelCase везде** — body, response, query, path. Pagination через `components/parameters/{PageQueryParam, PerPageQueryParam}` (camelCase, query) и `components/schemas/PaginationInput` (camelCase, body).
- *General:*
  - Roadmap chunk 5 правится: `POST` → `GET` + упоминание массива пар, `[ ]` → `[~]` на старте, `[x]` после merge.
  - PR содержит regenerated artifacts: `server.gen.go`, `apiclient/*.gen.go`, 3× `frontend/*/src/api/generated/schema.ts`, mockery output.
  - Все фронты должны компилироваться (`make build-web build-tma build-landing`) — после регенерации TS-типы могут чуть измениться (например, `Brand` поля стали из `BrandInput`), фронт-консьюмеры подстраиваются если ломается.

**Ask First:**
- Изменение пути `/counts` → `/stats`/`/summary`.
- Изменение counts shape (sparse → dense / массив → map).
- Drop любого из 9 side-quests или добавление новых.
- Если pagination renaming сломает что-то нетривиальное во фронте админки.

**Never:**
- POST с пустым body вместо GET для counts.
- Возврат `count: 0` для отсутствующих статусов (sparse — фронт лукапит `?? 0`).
- 500 на unknown status в map от repo (просто проходит в response — БД защищена ENUM CHECK; если расхождение — это уже сигнал фронту через TS-валидацию enum).
- Дублирование enum статусов / responses / object-схем после рефакторинга — grep verifies.
- Аудит-лог на read-only counts.
- Прямой INSERT в БД из e2e (только бизнес-эндпоинты).
- Frontend-изменения сверх адаптации под renamed types (фичи фронта — chunk 6).

## I/O & Edge-Case Matrix

| Сценарий | Input | Expected | Покрытие |
|---|---|---|---|
| No auth | без Bearer | 401 | e2e + unit handler |
| Non-admin | brand_manager Bearer | 403, до бизнес-логики | e2e + unit handler/authz |
| Пустая БД (теоретически) | admin GET | 200, `items: []` | unit service (mock repo вернул пустой map) |
| После одной заявки | admin GET | 200, `items` содержит ровно один элемент `{status: "verification", count >= 1}` | e2e |
| Repo вернул map с 3 статусами | service.Counts | slice длины 3, отсортирован по `status` alphabetically | unit service |
| Метод не GET | POST/DELETE | 405 (chi default) | не тестируем |
| Audit-logs `perPage=N` после rename (camelCase query) | GET admin | 200 + Data.PerPage эхо | regression unit на handler |

</frozen-after-approval>

## Code Map

### Counts feature

- `backend/api/openapi.yaml` — операция `getCreatorApplicationsCounts` (`GET /creators/applications/counts`), схемы `CreatorApplicationsCountsResult` (`{data}`), `CreatorApplicationsCountsData` (`{items: CreatorApplicationStatusCount[]}`), `CreatorApplicationStatusCount` (`{status: $ref CreatorApplicationStatus, count: int64 minimum 0}`). Description у operation и `CreatorApplicationsCountsData` явно говорит про sparse-семантику.
- `backend/internal/repository/creator_application.go` — `Counts(ctx) (map[string]int64, error)` в интерфейсе и реализации; squirrel `Select(status, count(*)).From(TableCreatorApplications).GroupBy(status)`.
- `backend/internal/service/creator_application.go` — `Counts(ctx) (map[string]int64, error)`: repo через `s.pool` (read-only); фильтрация неизвестных статусов через `domain.IsValidCreatorApplicationStatus` + warn-log (rolling-deploy safety), wire-shape остаётся map.
- `backend/internal/domain/creator_application.go` — `CreatorApplicationAllStatuses` slice + `IsValidCreatorApplicationStatus` хелпер. Отдельной struct'ы под `(status, count)` пары нет — wire shape (`api.CreatorApplicationStatusCount`) достаточно: handler использует generated тип напрямую.
- `backend/internal/handler/creator_application.go` — `GetCreatorApplicationsCounts(ctx, request)`: authz first, далее map→slice конверсия с `slices.SortFunc` по `string(Status)` (alphabetical) для детерминированного wire-output.
- `backend/internal/authz/creator_application.go` — `CanGetCreatorApplicationsCounts(ctx) error` (admin-only, копия pattern `CanListCreatorApplications`).
- `backend/e2e/creator_applications/counts_test.go` — `TestCreatorApplicationsCounts` (smoke).

### Side-quest 1: shared `CreatorApplicationStatus` enum

- `backend/api/openapi.yaml` — новая schema `CreatorApplicationStatus` (string enum 7 значений). Заменить inline определения в трёх местах (строки ~1306 в `CreatorApplicationDetailMain`, ~1371 в `CreatorApplicationsListRequest.statuses[]`, ~1452 в `CreatorApplicationListItem.status`) на `$ref`.

### Side-quest 2: shared `UserRole` enum

- `backend/api/openapi.yaml` — новая schema `UserRole` (string enum `[admin, brand_manager]`). Заменить inline в `User.role` (~line 825) на `$ref`. Потенциально найти другие места grep'ом `enum:.*admin, brand_manager`.

### Side-quest 3: shared `ConsentType` enum

- `backend/api/openapi.yaml` — новая schema `ConsentType` (string enum `[processing, third_party, cross_border, terms]`). Заменить inline в `CreatorApplicationDetailConsent.consentType` (~line 1218) на `$ref`. Grep на другие места.

### Side-quest 4: `components/responses/UnexpectedError` (19→1)

- `backend/api/openapi.yaml` — новый `components/responses/UnexpectedError` (description "Unexpected error" + content ErrorResponse). Заменить 19 inline-блоков `default:` на `$ref: "#/components/responses/UnexpectedError"`.

### Side-quest 5: `components/responses/Forbidden` (9→1)

- `backend/api/openapi.yaml` — новый `components/responses/Forbidden`. Заменить 9 inline-блоков `"403":` на `$ref`.

### Side-quest 6: `BrandInput` (CreateBrandRequest ≡ UpdateBrandRequest)

- `backend/api/openapi.yaml` — новая schema `BrandInput`. `CreateBrandRequest` и `UpdateBrandRequest` либо удаляются и операции ссылаются прямо на `BrandInput`, либо становятся `allOf: [BrandInput]`. Решение в реализации: смотрим что чище в openapi-typescript output.
- `backend/internal/handler/brand.go` (если есть) — адаптация под renamed types из generated.
- `frontend/web/src/features/brands/**`, `frontend/tma/...` — adapter, если используется generated `CreateBrandRequest`/`UpdateBrandRequest`.

### Side-quest 7: `DictionaryItem` для `code+name+sortOrder` (3→1)

- `backend/api/openapi.yaml` — новая schema `DictionaryItem` (`{code, name, sortOrder}` с описаниями). `DictionaryEntry`, `CreatorApplicationDetailCategory`, `CreatorApplicationDetailCity` либо `$ref: DictionaryItem` напрямую (если идентичны), либо `allOf: [{$ref: DictionaryItem}]`. Цель — одно общее определение.
- Backend hydration code, e2e, unit tests — могут потребовать импорт переименованного типа из generated.

### Side-quest 8: pagination unification + camelCase query convention

- `backend/api/openapi.yaml`:
  - Новые `components/parameters/PageQueryParam` (`name: page`) и `components/parameters/PerPageQueryParam` (`name: perPage`, camelCase) для GET-эндпоинтов.
  - Audit-logs (`/audit-logs`): inline-параметры `page` и `per_page` заменены на `$ref`; снейк-параметры `actor_id`/`entity_type`/`entity_id`/`date_from`/`date_to`/`per_page` переименованы в camelCase (`actorId`/`entityType`/`entityId`/`dateFrom`/`dateTo`/`perPage`).
  - Новая schema `PaginationInput` (`{page: int >=1, perPage: int 1..200}`) для body-pagination.
  - `CreatorApplicationsListRequest` — `allOf: [{$ref: PaginationInput}, {sort, order, filters...}]`.
- Backend handler/repo / e2e — обновить URL-строки тестов с snake → camelCase (Go-имена структур остаются прежние через oapi-codegen Title-casing).

### Roadmap

- `_bmad-output/planning-artifacts/creator-onboarding-roadmap.md` — chunk 5: `POST` → `GET` + упоминание массива пар, `[ ]` → `[~]` на старте, `[x]` после merge. Revision запись в frontmatter (две даты — старт и финиш).

### Regenerated

- `backend/internal/api/server.gen.go`
- `backend/e2e/apiclient/{types,client}.gen.go`
- `backend/e2e/testclient/{types,client}.gen.go`
- `frontend/{web,tma,landing}/src/api/generated/schema.ts`
- `backend/internal/repository/mocks/*` — после расширения `CreatorApplicationRepo` интерфейса (`Counts`).

## Tasks & Acceptance

**Execution (логический порядок зависимостей):**

- [ ] **openapi side-quests 1-7** (структурные shared schemas/responses): добавить `CreatorApplicationStatus`, `UserRole`, `ConsentType`, `BrandInput`, `DictionaryItem`, `components/responses/{UnexpectedError, Forbidden}`. Заменить все inline-дубли на `$ref`. Никакой логической правки контракта на этом шаге.
- [ ] **openapi side-quest 8** (pagination unification + camelCase query): `components/parameters/{PageQueryParam, PerPageQueryParam}` (camelCase), `components/schemas/PaginationInput` (camelCase). Audit-logs все query-параметры → camelCase, через `$ref` где применимо. Creator-applications/list — `allOf` через `PaginationInput`.
- [ ] **openapi counts**: операция `getCreatorApplicationsCounts` + схемы `CreatorApplicationsCountsResult/Data/StatusCount`. Description у `CountsData` явно указывает sparse поведение.
- [ ] `make generate-api` — re-generate (Go server + 2 e2e clients + 3 frontend schemas).
- [ ] `cd backend && mockery` — re-generate mocks (CreatorApplicationRepo расширен `Counts`).
- [ ] `backend/internal/domain/creator_application.go` — `CreatorApplicationStatusCount` struct.
- [ ] `backend/internal/repository/creator_application.go` — `Counts`; добавить в интерфейс.
- [ ] `backend/internal/authz/creator_application.go` — `CanGetCreatorApplicationsCounts`.
- [ ] `backend/internal/service/creator_application.go` — `Counts`; map→slice конверсия + `slices.SortFunc` по `Status`.
- [ ] `backend/internal/handler/creator_application.go` — `GetCreatorApplicationsCounts`; authz first.
- [ ] `backend/internal/handler/audit.go` (или эквивалент) + `backend/internal/repository/audit.go` — адаптация под renamed query param `perPage`. Тесты на handler/service/repo обновлены.
- [ ] Адаптация под renamed/unified types (`BrandInput`, `DictionaryItem`): handler/service Brand-кода, hydration кода для category/city, фронт-консьюмеры (если ломается компиляция).
- [ ] **Unit-тесты** на counts (handler/service/repo/authz) по стандарту `backend-testing-unit.md`. Coverage gate ≥80% per-method. Адаптировать существующие unit-тесты под renamed types.
- [ ] `backend/e2e/creator_applications/counts_test.go` — `TestCreatorApplicationsCounts`: t.Parallel, t.Run-сценарии (401 без Bearer, 403 для brand_manager, happy после `SetupCreatorApplicationViaLanding` → `items` содержит элемент со `status="verification"` и `count >= 1`). БЕЗ empty-DB сценария (БД накапливается). Нарративный godoc на русском с явным упоминанием sparse shape и расширения покрытия в chunks 7-12.
- [ ] `backend/e2e/...` — адаптация существующих e2e под audit-logs `perPage` (если что-то ломается).
- [ ] `_bmad-output/planning-artifacts/creator-onboarding-roadmap.md` — POST→GET + массив пар, `[ ]` → `[~]`, revision frontmatter.

**Acceptance Criteria:**

- *Counts:*
  - Given non-admin Bearer, when GET `/creators/applications/counts`, then 403 без leak'а существования.
  - Given одна заявка после реального `POST /creators/applications`, when admin GET, then `items` содержит элемент `{status: "verification", count: N>=1}` (порядок: alphabetical).
  - Given mock repo возвращает map из 3 статусов, when service.Counts, then slice длины 3, items отсортированы по `status` alphabetically (unit).
- *Hygiene:*
  - Given openapi.yaml после рефакторинга, when `grep -c "\\[verification, moderation"` — 0 (один canonical в `CreatorApplicationStatus`); `grep -c "\\[admin, brand_manager"` — 0; `grep -c "\\[processing, third_party"` — 0.
  - Given openapi.yaml, when grep `description: Unexpected error` — встречается 1 раз (в `components/responses/UnexpectedError`).
  - Given openapi.yaml, when grep `description: Forbidden` — встречается 1 раз.
  - Given openapi.yaml, when grep `name: per_page` — 0 совпадений; `name: perPage` — 1 раз (в `components/parameters/PerPageQueryParam`).
  - Given openapi.yaml, when grep `name: actor_id\|name: entity_type\|name: entity_id\|name: date_from\|name: date_to` — 0 совпадений (всё в camelCase).
  - Given regenerated `frontend/*/src/api/generated/schema.ts`, when `npx tsc --noEmit` в каждом из 3 фронтов, then без ошибок.
- *Общее:*
  - Coverage gate `make test-unit-backend-coverage` зелёный для затронутых пакетов.
  - `make build-backend lint-backend test-unit-backend test-e2e-backend build-web build-tma build-landing lint-web lint-tma lint-landing` — всё зелёное.

## Design Notes

**Почему массив пар + sparse shape.** `additionalProperties: integer` дало бы `Record<string, number>` без enum-привязки. Массив с `status: $ref CreatorApplicationStatus` — type-safe лукап на фронте. Sparse (только ненулевые статусы) экономит сериализацию и явно сообщает «не было заявок» вместо «возможно мы забыли посчитать». Фронт лукапит через `find(c => c.status === STATUS_VERIFICATION)?.count ?? 0` — после const-объекта статусов (стандарт `frontend-types.md` § Enum-ы) это коротко.

**Почему deterministic sort.** Map iteration в Go не детерминирована — тесты, ассертящие порядок, флакают. Сортировка по `status` alphabetically даёт стабильный output без необходимости специальной инфраструктуры.

**Почему `BrandInput` через `allOf` (если выберем) vs flat.** OpenAPI 3.0 `allOf` корректно генерируется openapi-typescript как intersection, но в oapi-codegen Go может дать неожиданный embedded struct. Если так — flat `$ref: BrandInput` в обоих request bodies (сейчас Create и Update идентичны, нет необходимости в композиции).

**Почему pagination renaming сейчас, а не отдельным PR.** `audit-logs` ещё не консьюмится фронтом (grep подтвердил), в Go-коде уже `perPage`. Renaming — чисто косметика на сгенерированном слое. Откладывать значит зафиксировать снейк-имя как baseline и платить дороже потом.

**Почему `DictionaryItem` — flat `$ref` (если три места идентичны) или `allOf` extension.** Идентичны сейчас, но `Category` может приобрести `categoryOtherText`-флаг. Стартуем с `$ref`, переходим на `allOf` когда появится дельта.

## Spec Change Log

- **2026-05-02 (mid-implement)**: пользователь сменил decision — service возвращает `map[string]int64`, не `[]domain.CreatorApplicationStatusCount`. Конверсия map→slice + alphabetical sort перенесены в handler. `domain.CreatorApplicationStatusCount` struct отказана (используется только generated `api.CreatorApplicationStatusCount` для wire). Wire-контракт без изменений, sparse-семантика и алфавитный порядок сохранены. Триггер: «Доменная структура для этого не нужна, обойдемся правильно типизированной мапой».
- **2026-05-02 (mid-implement)**: convention для query-параметров — `camelCase` везде (а не `snake_case` в query + `camelCase` в body). Audit-logs query переименованы (`actor_id`→`actorId`, `entity_type`→`entityType`, `entity_id`→`entityId`, `date_from`→`dateFrom`, `date_to`→`dateTo`, `per_page`→`perPage`). Триггер: «давай тогда во всем api использовать camel case уж».

## Verification

**Commands:**
- `make generate-api` — успешная регенерация (нет diff'а после повтора).
- `cd backend && mockery` — без ошибок.
- `make build-backend lint-backend test-unit-backend test-unit-backend-coverage` — зелёное.
- `make test-e2e-backend` — `TestCreatorApplicationsCounts` зелёный + регрессия по audit-logs e2e.
- `make build-web build-tma build-landing lint-web lint-tma lint-landing` — зелёное.
- `grep -E "enum: \\[(verification|admin, brand_manager|processing, third_party)" backend/api/openapi.yaml` — пусто.
- `grep "name: per_page" backend/api/openapi.yaml` — пусто.
- `grep -c "description: Unexpected error" backend/api/openapi.yaml` — 1.

## Suggested Review Order

**Counts feature: контракт → handler → service → repo → authz**

- Operation declaration со sparse-семантикой и алфавитной гарантией ordering.
  [`openapi.yaml:559`](../../backend/api/openapi.yaml#L559)

- Wire schema под `(status, count)` пары и enclosing data wrapper.
  [`openapi.yaml:1449`](../../backend/api/openapi.yaml#L1449)

- Handler: authz first, map→slice конверсия + `slices.SortFunc` (детерминизм).
  [`creator_application.go:246`](../../backend/internal/handler/creator_application.go#L246)

- Service: фильтр unknown статусов через domain helper + warn-log (rolling-deploy safety).
  [`creator_application.go:561`](../../backend/internal/service/creator_application.go#L561)

- Repository: единственный SQL `GROUP BY status`, без фильтров и пагинации.
  [`creator_application.go:292`](../../backend/internal/repository/creator_application.go#L292)

- Domain helper для whitelist-валидации статуса (используется service-фильтром).
  [`creator_application.go:332`](../../backend/internal/domain/creator_application.go#L332)

- Authz отдельным методом (не reuse `CanListCreatorApplications`) — admin-only.
  [`creator_application.go:40`](../../backend/internal/authz/creator_application.go#L40)

**API hygiene: shared schemas / responses / parameters**

- Canonical CreatorApplicationStatus (раньше дублировался 3×).
  [`openapi.yaml:730`](../../backend/api/openapi.yaml#L730)

- Shared UserRole / ConsentType.
  [`openapi.yaml:720`](../../backend/api/openapi.yaml#L720)

- Shared BrandInput (CreateBrandRequest≡UpdateBrandRequest сведены).
  [`openapi.yaml:857`](../../backend/api/openapi.yaml#L857)

- Shared DictionaryItem (3 идентичные `code+name+sortOrder` объекта сведены).
  [`openapi.yaml:752`](../../backend/api/openapi.yaml#L752)

- Shared response блоки (раньше 19 inline `default` + 9 inline `403`).
  [`openapi.yaml:660`](../../backend/api/openapi.yaml#L660)

- Pagination через query parameters + body schema, camelCase везде.
  [`openapi.yaml:643`](../../backend/api/openapi.yaml#L643)

**Type adaptations внутри backend**

- `sortDictionaryItem` helper — переиспользуется в detail и list mappers.
  [`creator_application.go:231`](../../backend/internal/handler/creator_application.go#L231)

- Dictionary handler адаптирован под renamed `api.DictionaryItem`.
  [`dictionary.go:27`](../../backend/internal/handler/dictionary.go#L27)

**Тесты**

- E2E smoke (401 / 403 / admin happy с проверкой verification ≥ 1 и алфавитной сортировки).
  [`counts_test.go:33`](../../backend/e2e/creator_applications/counts_test.go#L33)

- Handler unit: forbidden / 500 / sparse / sort-on-wire.
  [`creator_application_test.go:1065`](../../backend/internal/handler/creator_application_test.go#L1065)

- Service unit: filter unknown + warn-log assertion (mock `MatchedBy` для variadic args).
  [`creator_application_test.go:1104`](../../backend/internal/service/creator_application_test.go#L1104)

- Repo unit: SQL string + 3 ветки (rows / empty / error).
  [`creator_application_test.go:689`](../../backend/internal/repository/creator_application_test.go#L689)

- Authz unit: 3 ветки роли (admin / manager / missing).
  [`creator_application_test.go:58`](../../backend/internal/authz/creator_application_test.go#L58)

**Roadmap**

- Chunk 5 переведён в `[~]` + revision: POST→GET + sparse + объединение с гигиеной.
  [`creator-onboarding-roadmap.md:45`](../planning-artifacts/creator-onboarding-roadmap.md#L45)
