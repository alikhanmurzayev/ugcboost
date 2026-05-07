---
title: 'POST /creators/list — фильтр ids[] (chunk 11 prereq)'
type: feature
created: '2026-05-07'
status: done
baseline_commit: 15e3c7d04d5ebb8faaace68b1d691d16960508f3
context:
  - docs/standards/
  - _bmad-output/implementation-artifacts/intent-campaign-creators-frontend.md
  - _bmad-output/implementation-artifacts/spec-campaign-creators-frontend-reference.md
---

<frozen-after-approval reason="human-owned intent — do not modify unless human renegotiates">

## Intent

**Problem:** A3 (`GET /campaigns/{id}/creators` из chunk 10) возвращает только `creator_id`. Чтобы фронт chunk 11 показал ФИО/соцсети/категории/возраст/город уже добавленных в кампанию креаторов — нужен lookup по UUID-ам. Существующий `POST /creators/list` поддерживает фильтры `cities/categories/dateRange/ageRange/search/sort/page`, но **не `ids[]`**. Без него фронт упрётся в N+1 (`GET /creators/{id}` per row) или вообще не сможет показать profile.

**Approach:** Минимальное расширение существующего `CreatorsListRequest` — опциональное поле `ids: array<string format=uuid> maxItems=200`. SQL: `Where(squirrel.Eq{CreatorColumnID: ids})` если непустой. Backward-compatible (поле optional, при отсутствии или пустом массиве поведение endpoint'а не меняется). Реализуется как отдельный мини-PR перед фронтом chunk 11; сам фронт-чанк (read-only + mutations) ждёт мержа этого PR.

## Boundaries & Constraints

**Always:**
- Полная загрузка `docs/standards/` перед кодом — все правила hard rules. Особенно `backend-architecture.md`, `backend-codegen.md`, `backend-constants.md`, `backend-design.md`, `backend-errors.md`, `backend-libraries.md`, `backend-repository.md`, `backend-testing-e2e.md`, `backend-testing-unit.md`, `naming.md`, `security.md`, `review-checklist.md`.
- Generated файлы (`*.gen.go`, frontend `schema.ts`, e2e clients) — только через `make generate-api` после правки `backend/api/openapi.yaml`. Generated файлы коммитятся в этом же PR.
- Per-method coverage gate ≥80% (`make test-unit-backend-coverage`).
- `-race` детектор включён.
- Backward compat: поле опционально; nil/empty слайс = NO-OP (поведение endpoint'а не меняется).
- Squirrel-конструктор: `squirrel.Eq{CreatorColumnID: ids}` (через константу колонки, не string-литерал; см. `backend-constants.md` § Колонки БД).
- `[]string` через `openapi_types.UUID` → `id.String()` в handler (через цикл, не `pointer.Get`-tricks).
- maxItems=200 декларирован в схеме И **enforced runtime-валидацией** в handler (oapi-codegen без явных валидаторов не enforce'ит maxItems — см. кандидат в стандарты в `deferred-work.md` от chunk 10 review).

**Ask First:**
- Любые правки в смежных эндпоинтах (`POST /creators/list` filters/sort/pagination — кроме нового `ids`).
- Любые изменения схемы `CreatorListItem`.
- Любая попытка добавить миграцию БД, новые колонки, индексы.

**Never:**
- Новые endpoint'ы (типа `POST /creators/by-ids`) — расширяем существующий.
- Миграции БД, новые колонки в `creators`, новые индексы.
- Изменения семантики уже существующих фильтров.
- Любые правки вне creator-list path (`api/openapi.yaml` `CreatorsListRequest`, `repository/creator.go`, `service/creator.go`, `handler/creator.go`, их тесты + e2e).
- DEFAULT/CHECK правки в БД (этот PR — только in-memory filtering).
- Любые правки в frontend (это для chunk 11 frontend, отдельный PR).

## I/O & Edge-Case Matrix

| Scenario | Input | Expected | Error Handling |
|---|---|---|---|
| ids отсутствует | `{ page,perPage,sort,order }` без `ids` | Filter не применяется; полный список как раньше | — |
| ids: [] (пустой) | `{ ids: [], page,... }` | Filter не применяется (эквивалент nil) | — |
| ids: [a, b] (валидные UUID, существуют) | 3 креатора (a, b, c) в БД | Возвращаются ровно 2 матча (a, b); `total: 2` | — |
| ids: [unknown_uuid] | UUID не в БД | `items: []`, `total: 0` | Без error |
| ids: [a] AND cities: [city_of_b] | Креатор a в city_of_a, b в city_of_b | `[]` (AND фильтров) | — |
| ids: 201 элемент | 201 UUID в массиве | 422 schema-level (maxItems) ИЛИ runtime-check в handler | `domain.NewValidationError` |
| ids: malformed string | строка не UUID-формата | 422 (схема format=uuid) | format violation |
| auth: non-admin | Любой ids | 403 (как сейчас, до бизнес-логики) | существующий guard |
| ids: [a, a] (дубликат) | Один и тот же UUID 2x | Возвращается 1 row (БД unique). Не падаем | — |

</frozen-after-approval>

## Code Map

- `backend/api/openapi.yaml` -- `CreatorsListRequest`: добавить optional `ids: array<string format=uuid> maxItems=200`. Description: «Match only creators whose id is in this list (admin-curated lookup для campaign-creator hydration). Combined с другими фильтрами через AND. Empty array — filter disabled (NO-OP).»
- `backend/internal/repository/creator.go` -- `List` пробрасывает `input.IDs []string` в squirrel: `if len(ids) > 0 { qb = qb.Where(squirrel.Eq{CreatorColumnID: ids}) }`. Без аргумента — поведение неизменно.
- `backend/internal/service/creator.go` -- `ListCreatorsInput` (или существующая структура — посмотреть имя) расширить `IDs []string`; service пробрасывает в repo без обработки.
- `backend/internal/handler/creator.go` -- `ListCreators`: парсит `req.Body.Ids` (`*[]openapi_types.UUID` если pointer, `[]openapi_types.UUID` если non-pointer — посмотреть в `server.gen.go` после `make generate-api`) → `[]string` через цикл `id.String()`. Если nil/empty — пустой слайс в input. **Runtime-check** `len(ids) > 200` → `domain.NewValidationError(domain.CodeValidation, "Список ids не может содержать больше 200 элементов")` ДО обращения к service (oapi-codegen не enforce'ит maxItems — defence in depth).
- `backend/internal/repository/creator_test.go` -- table-driven сценарии: `ids:nil`, `ids:[]`, `ids:[a,b]`, `ids:[unknown]`, `ids:[a]+cities:[city_of_b]` (AND даёт пусто). SQL-assert: `WHERE id = ANY($N)` присутствует в query при non-empty ids; отсутствует при empty/nil.
- `backend/internal/service/creator_test.go` -- кейс «проброс IDs в repo»: `mock.Run` захватывает arg, ассерт что service передал `IDs` как есть.
- `backend/internal/handler/creator_test.go` -- кейсы: `Ids: nil` → empty slice в service input (через captured argument); `Ids: [u1, u2]` → конвертация в `[]string{"u1", "u2"}`; `Ids: <201 элементов>` → 422 без обращения к service.
- `backend/e2e/creator/list_test.go` -- `t.Run("filter by ids", ...)`: создаём 3 approved креаторов через approve flow / test-helper, шлём `POST /creators/list { ids: [c1, c3], page:1, perPage:50, sort:"created_at", order:"desc" }` → `total=2`, `items` содержат c1, c3 (не c2). Дополнительный subtest `t.Run("filter by ids: unknown returns empty", ...)`: `ids:[<random uuid>]` → `total=0`.

## Tasks & Acceptance

**Execution:**
- [x] `backend/api/openapi.yaml` -- `CreatorsListRequest.ids` optional `array<string format=uuid> maxItems=200`. Description явно фиксирует: admin-curated lookup, AND с другими фильтрами, empty/nil = NO-OP.
- [x] `make generate-api` -- регенерит server.gen.go, e2e clients (`backend/e2e/apiclient/`), frontend schemas (`frontend/{web,tma,landing}/src/api/generated/schema.ts`, `frontend/e2e/types/schema.ts`). Generated файлы коммитятся в этом PR.
- [x] `backend/internal/repository/creator.go` -- ветка `if len(ids) > 0 { qb = qb.Where(squirrel.Eq{CreatorColumnID: ids}) }` в `List`.
- [x] `backend/internal/service/creator.go` -- расширить input `IDs []string`; проброс в repo без обработки.
- [x] `backend/internal/handler/creator.go` -- handler парсит `req.Body.Ids` в `[]string`. Runtime-check `len > 200 → domain.NewValidationError(domain.CodeValidation, "Список ids не может содержать больше 200 элементов. Сократите запрос.")`.
- [x] Unit handler/service/repo -- сценарии из I/O Matrix. Per-method coverage ≥80% (`make test-unit-backend-coverage`).
- [x] E2E `backend/e2e/creators/list_test.go` -- `t.Run("happy: ids filter narrows to subset of approved creators", ...)` с 3 креаторами (assertion на 2 матча) + `t.Run("happy: ids filter with unknown UUID returns empty page", ...)` + `t.Run("validation: ids[] over max returns 422", ...)`.
- [x] (бэк-агент chunk 10 параллельно работает в `-backend` ветке — этот PR пишется на отдельной ветке `alikhan/creators-list-ids-filter` от main, после мержа chunk 10).

**Acceptance Criteria:**
- Given чистая БД и 3 approved креатора (c1, c2, c3), when `POST /creators/list { ids: [c1, c3], page:1, perPage:50, sort:"created_at", order:"desc" }`, then ответ содержит ровно 2 row'а (c1 и c3), `total: 2`.
- Given existing call без `ids`, when `POST /creators/list { page:1, perPage:50, sort, order }`, then поведение неизменно (backward compat — полный список approved'ов).
- Given `{ ids: [], ... }` или `{ ... }` без `ids`, when шлём, then filter не применяется (empty == nil, полный список).
- Given `{ ids: [<201 элемент>] }`, then 422 (через схему OpenAPI или manual handler check); service не вызывался.
- Given `{ ids: [c1], cities: [city_of_c2] }`, then `total: 0` (AND, нет матча).
- Given non-admin auth, when шлём `{ ids: [...] }`, then 403 (до бизнес-логики, как сейчас).
- Given `make build-backend && make lint-backend && make test-unit-backend && make test-unit-backend-coverage && make test-e2e-backend`, then всё зелёное.

## Verification

**Commands:**
- `make generate-api` -- expected: regenerated файлы коммитятся, нет manual edits в `*.gen.go` / `schema.ts`.
- `make build-backend && make lint-backend` -- expected: clean.
- `make test-unit-backend && make test-unit-backend-coverage` -- expected: новые сценарии зелёные, gate ≥80% per-method.
- `make test-e2e-backend` -- expected: `creator/list_test.go` зелёный, в т.ч. `t.Run("filter by ids", ...)`.

**Manual checks:**
- После `make generate-api` — diff сгенерированных файлов должен показать только новое поле `ids` (и связанное в server.gen.go); никаких посторонних изменений.
- В `frontend/web/src/api/generated/schema.ts` — поле `ids?: string[]` появилось в `CreatorsListRequest`.
- `curl` smoke: `POST /creators/list -H "Authorization: Bearer <admin>" -d '{"ids":["<uuid_c1>","<uuid_c3>"],"page":1,"perPage":50,"sort":"created_at","order":"desc"}'` — отвечает ровно 2 креаторами.

**Self-check агента (без HALT, между unit и e2e):**
1. `make migrate-up && make start-backend`.
2. Через test-helper или approve-flow создать 3 approved креаторов, запомнить UUID-ы (c1, c2, c3).
3. `curl POST /creators/list { ids:[c1,c3], page:1, perPage:50, sort:"created_at", order:"desc" }` → `total:2`, items.length=2, ids в ответе ⊂ {c1, c3}.
4. `curl POST /creators/list { page:1, perPage:50, sort, order }` (без ids) → полный список (3+).
5. `curl POST /creators/list { ids:[<random uuid>], ... }` → `total:0`, `items:[]`.
6. Расхождение со спекой = баг → агент сам фиксит, перезапускает self-check, переходит к e2e в той же сессии.

## Suggested Review Order

**Контракт API**

- Поле `ids` в `CreatorsListRequest` — optional UUID-массив, maxItems=200.
  [`openapi.yaml:2519`](../../backend/api/openapi.yaml#L2519)

**Валидация на handler-уровне (точка входа)**

- Cap maxItems + dedup + reject zero-UUID — единый chokepoint перед service.
  [`creator.go:271`](../../backend/internal/handler/creator.go#L271)

- Где валидатор вписан в pipeline `ListCreators` — между cities и `domain.CreatorListInput`.
  [`creator.go:167`](../../backend/internal/handler/creator.go#L167)

**Domain — константа и input**

- Лимит 200 рядом с другими (page/perPage/array max).
  [`creator.go:231`](../../backend/internal/domain/creator.go#L231)

- Поле `IDs []string` в `CreatorListInput`.
  [`creator.go:238`](../../backend/internal/domain/creator.go#L238)

**SQL — фильтр в repository**

- `Where(sq.Eq{crID: p.IDs})` — единственная новая строка в `applyCreatorListFilters`, идёт первой по AND.
  [`creator.go:275`](../../backend/internal/repository/creator.go#L275)

- `IDs []string` в `CreatorListParams`.
  [`creator.go:104`](../../backend/internal/repository/creator.go#L104)

**Service — проброс без обработки**

- Mapping в `creatorListInputToRepo`.
  [`creator.go:238`](../../backend/internal/service/creator.go#L238)

**Тесты**

- TestValidateCreatorIDs — 6 кейсов: nil/empty/canonical-lowercase/dedup/nil-UUID/oversize/exact-max.
  [`creator_test.go:720`](../../backend/internal/handler/creator_test.go#L720)

- Validation table-кейс «ids[] over max returns 422 without service call».
  [`creator_test.go:355`](../../backend/internal/handler/creator_test.go#L355)

- Capture-input: ids доходит до service как `[]string`.
  [`creator_test.go:590`](../../backend/internal/handler/creator_test.go#L590)

- Repo: пустой ids не добавляет WHERE; `ids` IN-clause; AND с cities.
  [`creator_test.go:131`](../../backend/internal/repository/creator_test.go#L131)

- Service: capture-input на проброс ids в repo.
  [`creator_test.go:599`](../../backend/internal/service/creator_test.go#L599)

- E2E: 422 на 201, ids=[c1,c3] из 3 креаторов → ровно 2, unknown UUID + seed → 0, AC5 ids AND cities mismatch → 0.
  [`list_test.go:171`](../../backend/e2e/creators/list_test.go#L171)

**Generated (для справки — порождены `make generate-api`)**

- Server type.
  [`server.gen.go:942`](../../backend/internal/api/server.gen.go#L942)

- E2E client type.
  [`types.gen.go:933`](../../backend/e2e/apiclient/types.gen.go#L933)

- Frontend `schema.ts` (`web`/`tma`/`landing`/`e2e`) — добавлено поле `ids?: string[]` в `CreatorsListRequest`.

