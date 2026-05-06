---
title: 'Backend chunk #4: GET /campaigns/{id} (admin-only)'
type: 'feature'
created: '2026-05-06'
status: 'done'
baseline_commit: 'c14cf7234cf3d1ed241d8f141049864fdba2ad6e'
context:
  - 'docs/standards/'
  - '_bmad-output/planning-artifacts/campaign-roadmap.md'
  - '_bmad-output/implementation-artifacts/spec-campaign-create-backend.md'
---

<frozen-after-approval reason="human-owned intent — do not modify unless human renegotiates">

## Intent

**Problem:** Чанк #4 кампаний-роудмапа: после POST /campaigns админ не может получить детали по id. Без read-by-id ручки фронт-страница «карточка кампании» (#8) и интерфейсы выбора креаторов (#10/#11) не получают source-of-truth.

**Approach:** Admin-only `GET /campaigns/{id}` — read-only, без транзакций, без audit. Возвращает полный `Campaign` (тип определён в #3) **независимо от `is_deleted`**: админ видит и живые, и soft-deleted записи. List-фильтр по `is_deleted` (nullable boolean) уезжает в чанк #6. Все паттерны (AuthZ-сервис, repo через `RepoFactory`, маппинг ошибок в `response.go`) повторяют существующий `GetCreator` 1:1.

## Boundaries & Constraints

**Always:**
- Полная загрузка `docs/standards/` и применение как hard rules.
- Слои handler → service → repository через `RepoFactory`. Без транзакций (одиночный read).
- AuthZ — admin-only через новый `AuthzService.CanGetCampaign(ctx)`, проверка раньше любого DB-чтения (timing-safe).
- Repository: `GetByID(ctx, id string) (*CampaignRow, error)` — `sq.Select(campaignSelectColumns...).From(TableCampaigns).Where(sq.Eq{CampaignColumnID: id})` через `dbutil.One`. **WHERE без фильтра `is_deleted`.** Не маппит `sql.ErrNoRows` — пробрасывает как есть (повторяем `creator.go:178`).
- Service: ловит `sql.ErrNoRows` и маппит в `domain.ErrCampaignNotFound`; прочие ошибки — `fmt.Errorf("get campaign: %w", err)` (повторяем `service/creator.go:67–73`).
- Handler: `request.Id.String()` для path-param UUID. AuthZ-проверка — первой. Ошибки уходят в `respondError` через strict-server.
- Response-mapper (`handler/response.go`): добавить `case errors.Is(err, domain.ErrCampaignNotFound): writeError(..., StatusNotFound, CodeCampaignNotFound, "Кампания не найдена.", ...)`.
- Code-gen: `make generate-api` после правки `openapi.yaml` — единственный путь обновления `*.gen.go` / frontend schemas.
- Reuse из #3 (что уже на диске): `domain.Campaign` struct, `testutil.SetupCampaign / RegisterCampaignCleanup`, `TableCampaigns / CampaignColumn* / campaignSelectColumns`, `CampaignRepoFactory`, `CampaignService` skeleton, `authz/campaign.go`, `handler/campaign.go` skeleton.
- **Впервые определяем в #4** (после #3 в openapi есть только `CampaignInput / CampaignCreatedData / CampaignCreatedResult` — POST возвращает только `id`): полная schema `Campaign { id, name, tmaUrl, isDeleted, createdAt, updatedAt }`, wrapper `CampaignResult { data: Campaign }`, mapper `domainCampaignToAPI(*domain.Campaign) api.Campaign` в `handler/campaign.go`.

**Ask First:**
- Изменение публичной модели API (`Campaign`, новые поля) — нет в скоупе.
- Скрытие soft-deleted в этой ручке (фильтр живёт в #6).
- Введение транзакции для read-операции.

**Never:**
- Audit-row для read-операции.
- Прямой `chi.URLParam` / ручной uuid-parse в handler (только generated-wrapper).
- 422 за невалидный UUID — wrapper отдаёт это сам.
- `pgx.ErrNoRows` (только `sql.ErrNoRows`).
- `logger.Info` на success read-пути (стандарт запрещает шум на read).
- Новый отдельный `TestGetCampaign` — расширяем существующий тест из #3 (см. e2e-таск).

## I/O & Edge-Case Matrix

| Сценарий | Вход / Состояние | Ответ | Обработка ошибок |
|---|---|---|---|
| Happy path (живая) | admin, существующий id, `is_deleted=false` | 200 + `CampaignResult{Data: Campaign{...}}` (полный набор полей) | — |
| Happy path (soft-deleted) | admin, существующий id, `is_deleted=true` | 200 + `CampaignResult{Data: Campaign{..., isDeleted:true}}` | — |
| Не аутентифицирован | без токена | 401 | middleware |
| brand-manager | manager-токен | 403 (до DB-чтения) | `CanGetCampaign` → `ErrForbidden` |
| Не найден | admin, несуществующий UUID | 404 `CAMPAIGN_NOT_FOUND` + «Кампания не найдена.» | service: `sql.ErrNoRows` → `ErrCampaignNotFound`; response.go switch |
| Невалидный UUID в path | admin, `id=not-a-uuid` | 4xx от generated wrapper (его собственный формат) | strict-server `RequestErrorHandlerFunc` |
| БД-ошибка | pool падает | 500 `INTERNAL_ERROR` (default) | service: проброс с `fmt.Errorf` |

</frozen-after-approval>

## Code Map

- `backend/api/openapi.yaml` -- (1) добавить `GET /campaigns/{id}` под тегом `campaigns`, ответы 200/401/403/404/default; (2) определить новые schemas `Campaign { id, name, tmaUrl, isDeleted, createdAt, updatedAt }` + `CampaignResult { data: Campaign }`. POST из #3 продолжает возвращать `CampaignCreatedResult{id}` — не трогаем.
- `backend/internal/domain/errors.go` -- `CodeCampaignNotFound = "CAMPAIGN_NOT_FOUND"`.
- `backend/internal/domain/campaign.go` -- `var ErrCampaignNotFound = errors.New("campaign not found")` (рядом с `ErrCampaignNameTaken` из #3).
- `backend/internal/repository/campaign.go` -- метод `GetByID(ctx, id string) (*CampaignRow, error)`.
- `backend/internal/repository/campaign_test.go` -- `Test_campaignRepository_GetByID` (success, sql.ErrNoRows pass-through, db error).
- `backend/internal/authz/campaign.go` -- `func (a *AuthzService) CanGetCampaign(ctx context.Context) error` (admin-only по паттерну `CanCreateCampaign`).
- `backend/internal/authz/campaign_test.go` -- расширить тестами `CanGetCampaign` (admin → nil, brand_manager → ErrForbidden, no-role → ErrForbidden).
- `backend/internal/service/campaign.go` -- метод `GetByID(ctx, id string) (*domain.Campaign, error)`: pool напрямую (без WithTx), маппинг `sql.ErrNoRows → ErrCampaignNotFound`, прочие — `fmt.Errorf("get campaign: %w", err)`.
- `backend/internal/service/campaign_test.go` -- `TestCampaignService_GetByID` (not found, generic error, success).
- `backend/internal/handler/campaign.go` -- (1) добавить mapper `domainCampaignToAPI(c *domain.Campaign) api.Campaign` (новый — в #3 его нет, потому что POST не возвращает полный Campaign); (2) добавить `func (s *Server) GetCampaign(ctx, request) (response, error)`.
- `backend/internal/handler/campaign_test.go` -- расширить `TestServer_GetCampaign`: forbidden, not found, service generic error, success.
- `backend/internal/handler/server.go` -- расширить `CampaignService` interface (`GetByID`), `AuthzService` interface (`CanGetCampaign`).
- `backend/internal/handler/response.go` -- добавить case `errors.Is(err, domain.ErrCampaignNotFound)` → 404 `CodeCampaignNotFound`.
- `backend/e2e/campaign/campaign_test.go` -- переименовать `TestCreateCampaign` → `TestCampaignCRUD`; обновить header-комментарий нарративом; добавить t.Run-сценарии для GET (см. таск).

## Tasks & Acceptance

**Execution (порядок строго по слоям):**

- [x] `backend/api/openapi.yaml` -- (1) Добавить schemas `Campaign { id (uuid), name (string), tmaUrl (string), isDeleted (boolean), createdAt (date-time), updatedAt (date-time) }` + `CampaignResult { data: Campaign }` (рядом с `CampaignCreatedResult` из #3). (2) Добавить `GET /campaigns/{id}`: `operationId: getCampaign`, `tags: [campaigns]`, `security: [bearerAuth]`, path-param `id` `format: uuid`, ответы `200 CampaignResult / 401 ErrorResponse / 403 Forbidden / 404 ErrorResponse / default UnexpectedError`. -- Контракт-first; полная schema появляется впервые здесь, потому что POST из #3 возвращает только `id`.
- [x] **`make generate-api`** -- регенерация всех `*.gen.go` + frontend schemas + apiclient/testclient. -- Единственный путь обновления generated.
- [x] `backend/internal/domain/errors.go` -- Добавить `CodeCampaignNotFound = "CAMPAIGN_NOT_FOUND"` рядом с другими `CodeCampaign*`. -- Granular код.
- [x] `backend/internal/domain/campaign.go` -- Добавить `var ErrCampaignNotFound = errors.New("campaign not found")` рядом с `ErrCampaignNameTaken`. -- Sentinel.
- [x] `backend/internal/repository/campaign.go` -- Добавить `GetByID(ctx context.Context, id string) (*CampaignRow, error)`: `sq.Select(campaignSelectColumns...).From(TableCampaigns).Where(sq.Eq{CampaignColumnID: id})` → `dbutil.One[CampaignRow](ctx, r.db, q)`. **Не маппить `sql.ErrNoRows`** — пробрасываем (паттерн `creator.go:178`). -- Repo-метод.
- [x] `backend/internal/repository/campaign_test.go` -- `Test_campaignRepository_GetByID`: `success` (точный SQL-литерал + args=[id]; scan возвращает full row), `not found` (мок возвращает `sql.ErrNoRows` → `errors.Is(err, sql.ErrNoRows)` пробросился), `db error` (generic ошибка → проброс as-is). -- Coverage gate ≥80%.
- [x] `backend/internal/authz/campaign.go` -- Добавить `func (a *AuthzService) CanGetCampaign(ctx context.Context) error` — `if middleware.RoleFromContext(ctx) != api.Admin { return domain.ErrForbidden }; return nil`. GoDoc-однострочник «admin-only». -- Authz.
- [x] `backend/internal/authz/campaign_test.go` -- Расширить тестами `CanGetCampaign`: admin → nil, brand_manager → ErrForbidden, no-role → ErrForbidden. -- Authz unit.
- [x] `backend/internal/service/campaign.go` -- Добавить метод `func (s *CampaignService) GetByID(ctx context.Context, id string) (*domain.Campaign, error)`: вызов `s.repoFactory.NewCampaignRepo(s.pool).GetByID(ctx, id)`; `errors.Is(err, sql.ErrNoRows) → domain.ErrCampaignNotFound`; прочие — `fmt.Errorf("get campaign: %w", err)`; success — `campaignRowToDomain(row)`. **Без `WithTx`, без audit, без `logger.Info`.** -- Бизнес-обёртка.
- [x] `backend/internal/service/campaign_test.go` -- `TestCampaignService_GetByID`: `not found` (repo вернул `sql.ErrNoRows` → service вернул `ErrCampaignNotFound` через `errors.Is`), `repo error` (generic → проброс с обёрткой), `success` (repo вернул row → `*domain.Campaign` через `require.Equal` с подменой dynamic полей). -- Service unit.
- [x] `backend/internal/handler/server.go` -- Расширить `CampaignService` интерфейс методом `GetByID(ctx, id string) (*domain.Campaign, error)`. Расширить `AuthzService` интерфейс методом `CanGetCampaign(ctx) error`. -- Server-wiring.
- [x] `backend/internal/handler/campaign.go` -- (1) Добавить mapper `func domainCampaignToAPI(c *domain.Campaign) api.Campaign` (поля 1:1 с openapi schema, `id` через `uuid.MustParse(c.ID)` или `openapi_types.UUID` парс — повторяем паттерн из `creator.go:48`). (2) Добавить `func (s *Server) GetCampaign(ctx context.Context, request api.GetCampaignRequestObject) (api.GetCampaignResponseObject, error)`: `s.authzService.CanGetCampaign(ctx)` → проброс; `s.campaignService.GetByID(ctx, request.Id.String())` → проброс; success → `api.GetCampaign200JSONResponse{Data: domainCampaignToAPI(c)}`. GoDoc — паттерн `GetCreator`. -- Handler + mapper.
- [x] `backend/internal/handler/response.go` -- Добавить `case errors.Is(err, domain.ErrCampaignNotFound): writeError(w, r, http.StatusNotFound, domain.CodeCampaignNotFound, "Кампания не найдена.", log)` в switch (рядом с `ErrCreatorNotFound`). -- HTTP-маппинг 404.
- [x] `backend/internal/handler/campaign_test.go` -- Расширить `TestServer_GetCampaign`: forbidden non-admin (authz отказал → 403, service не вызывается), not found (service вернул `ErrCampaignNotFound` → 404 + `CAMPAIGN_NOT_FOUND` + «Кампания не найдена.»), service generic error (→ 500 default), success (200 + `CampaignResult{Data: Campaign{...}}` через `require.Equal` с подменой `id`/`*At`). -- Handler unit.
- [x] **Manual local sanity check (HALT)** -- `make migrate-up && make start-backend`. Получить admin- и brand-manager-токены через test-API (паттерн из #3). Curl-сценарии: (a) **happy** `POST /campaigns` → захватить `id`; `GET /campaigns/{id}` admin-токеном → 200 + полный `Campaign` (id совпадает, name/tmaUrl, isDeleted:false, *At). (b) **not found** `GET /campaigns/<random uuid>` admin-токеном → 404 + `CAMPAIGN_NOT_FOUND` + «Кампания не найдена.». (c) **403** brand-manager-токеном → 403 `FORBIDDEN`. (d) **401** без токена → 401. (e) **invalid uuid** `GET /campaigns/not-a-uuid` → 4xx от wrapper. **HALT и сообщить пользователю фактические HTTP-коды + JSON-тела до старта e2e.** -- Проверка работающего кода до фиксации e2e-контрактом.
- [x] `backend/e2e/campaign/campaign_test.go` -- Переименовать `TestCreateCampaign` → `TestCampaignCRUD` (имя отражает реальный скоуп create+get). Обновить header-комментарий нарративом по `backend-testing-e2e.md`: добавить абзац про GET-семантику. Добавить новые `t.Run` для GET: `get unauthenticated → 401`; `get forbidden brand-manager → 403`; `get not found (random uuid) → 404 CAMPAIGN_NOT_FOUND`; `get success` — после POST из соседнего `t.Run` (или через `testutil.SetupCampaign`) GET по id, ассерт **всех полей контракта** конкретными значениями (id совпадает, name/tmaUrl равны setup-значениям, isDeleted=false, *At через `WithinDuration`). `TestCreateCampaign_RaceUniqueName` оставить как есть. Все `t.Parallel()`. Cleanup через `testutil.RegisterCampaignCleanup`. -- E2E (только после успешной ручной проверки).

**Acceptance Criteria:**
- Given `make build-backend && make lint-backend`, when запускаются, then оба зелёные.
- Given `make test-unit-backend && make test-unit-backend-coverage`, when запускаются, then оба зелёные; per-method coverage gate ≥80% сохраняется на изменённых файлах в `handler/service/repository/authz`.
- Given `make test-e2e-backend`, when запускается, then `TestCampaignCRUD` (с GET-сценариями) + `TestCreateCampaign_RaceUniqueName` зелёные; cleanup-stack удаляет созданные кампании.
- Given поднятый бэк локально, when admin GET /campaigns/{id} с валидным id, then 200 + полный `Campaign{id,name,tmaUrl,isDeleted,createdAt,updatedAt}` совпадает с тем, что вернул POST.
- Given несуществующий uuid, when admin GET /campaigns/{id}, then 404 + `code:"CAMPAIGN_NOT_FOUND"` + «Кампания не найдена.».
- Given soft-deleted кампания (этот чанк DELETE не реализует — критерий покрытия GET soft-deleted переносится в e2e #7).

## Spec Change Log

- **2026-05-06 — mapper signature** — Boundaries Always declared `domainCampaignToAPI(*domain.Campaign) api.Campaign`, but Tasks said «(`uuid.MustParse(c.ID)` или `openapi_types.UUID` парс — повторяем паттерн из `creator.go:48`)» — and `creator.go:48` (`domainCreatorAggregateToAPI`) is error-returning. Self-contradiction caught by acceptance review. Implementation chose the error-returning shape `(*domain.Campaign) (api.Campaign, error)` because (a) it matches the `creator.go` mapper invoked by Tasks, (b) it matches the existing defensive `uuid.Parse` in `CreateCampaign` in the same file, and (c) `MustParse` would panic on a corrupted DB row instead of surfacing a clean 500 — worse for observability. The matching unit-test scenario `corrupted ID from service surfaces as 500` was added in `handler/campaign_test.go` to lock the contract.

## Design Notes

POST `/campaigns` из #3 возвращает только `CampaignCreatedResult{Data: CampaignCreatedData{id}}` — полный `Campaign` фронту не нужен на create. GET `/campaigns/{id}` — первое место, где требуется полная форма ресурса наружу: поэтому `Campaign` schema, `CampaignResult` wrapper и `domainCampaignToAPI` mapper определяются впервые в этом чанке. Будущие `PATCH`/`LIST` (#5/#6) переиспользуют их.

`ErrCampaignNotFound` — простой `errors.New` (не `BusinessError`), повторяет паттерн `ErrCreatorNotFound`. Маппинг в 404 — через явный `case errors.Is` в `respondError`, не через `errors.As(*BusinessError)` (который маппит в 409). Это уже разведено в `response.go:43–84`.

`request.Id` от generated-wrapper уже `openapi_types.UUID` (валидный UUID гарантирован). `String()` нужен потому что repo принимает `id string` (паттерн всего проекта — id хранятся как string, конверсия только на API-границе). См. `creator.go:30` (`request.Id.String()`).

Repo не маппит `sql.ErrNoRows` намеренно: это позволяет service отличать «не найдено» от других DB-ошибок единым `errors.Is`. Если бы маппинг был в repo, любая будущая ручка, которая ожидает «не найдено» как штатный исход (например, idempotent-check), получала бы wrapped error.

## Verification

**Commands:**
- `make generate-api` -- expected: успех; `git diff --stat` показывает обновлённые `*.gen.go` + frontend schemas
- `make build-backend` -- expected: компиляция чистая
- `make lint-backend` -- expected: zero issues
- `make test-unit-backend` -- expected: зелёные с `-race`
- `make test-unit-backend-coverage` -- expected: per-method gate зелёный
- `make test-e2e-backend` -- expected: `TestCampaignCRUD` + `TestCreateCampaign_RaceUniqueName` зелёные

## Suggested Review Order

**API contract (entry point — что появилось наружу)**

- New `GET /campaigns/{id}` operation with full response set (200/401/403/404/default).
  [`openapi.yaml:1018`](../../backend/api/openapi.yaml#L1018)

- New `Campaign` schema + `CampaignResult` wrapper — first time the full row is exposed (POST stays id-only).
  [`openapi.yaml:2398`](../../backend/api/openapi.yaml#L2398)

**Read pipeline (handler → service → repository)**

- Handler: authz first, then service, then mapper — timing-safe 403 before any DB read.
  [`campaign.go:63`](../../backend/internal/handler/campaign.go#L63)

- Mapper `domainCampaignToAPI` — defensive `uuid.Parse` (panics-on-corruption is worse than 500), see Spec Change Log.
  [`campaign.go:84`](../../backend/internal/handler/campaign.go#L84)

- Service: pool-direct read, no tx, no audit, no success log — sql.ErrNoRows → ErrCampaignNotFound at the boundary.
  [`campaign.go:70`](../../backend/internal/service/campaign.go#L70)

- Repository: `WHERE id = $1` — deliberately no `is_deleted` filter so admins read soft-deleted rows for audit/restore.
  [`campaign.go:88`](../../backend/internal/repository/campaign.go#L88)

**Authorization (admin-only)**

- `CanGetCampaign` mirrors `CanCreateCampaign` — same admin-or-403 contract for the read path.
  [`campaign.go:23`](../../backend/internal/authz/campaign.go#L23)

**Error mapping (4xx)**

- New `ErrCampaignNotFound` sentinel — `errors.New`, not a BusinessError, so it routes to 404 not 409.
  [`campaign.go:38`](../../backend/internal/domain/campaign.go#L38)

- 404 case in `respondError` — placed before the generic `ErrNotFound`/`sql.ErrNoRows` arm so the granular code wins.
  [`response.go:73`](../../backend/internal/handler/response.go#L73)

- Granular error code surfaced to the client.
  [`errors.go:32`](../../backend/internal/domain/errors.go#L32)

**Server wiring**

- Interfaces extended for the new method/permission — strict-server bound.
  [`server.go:56`](../../backend/internal/handler/server.go#L56)

**Tests (peripherals)**

- E2E: `TestCampaignCRUD` (renamed from `TestCreateCampaign`) — POST scenarios + 4 new GET t.Runs covering 401/403/404/200.
  [`campaign_test.go:59`](../../backend/e2e/campaign/campaign_test.go#L59)

- Handler unit: 5 t.Runs locking the strict-server response contract including the corrupted-id 500 path.
  [`campaign_test.go:174`](../../backend/internal/handler/campaign_test.go#L174)

- Service unit: not-found mapping, error wrapping, and full row→domain projection.
  [`campaign_test.go:141`](../../backend/internal/service/campaign_test.go#L141)

- Repo unit: success live + success soft-deleted (locks the no-`is_deleted`-filter contract) + sql.ErrNoRows pass-through.
  [`campaign_test.go:87`](../../backend/internal/repository/campaign_test.go#L87)

- Authz unit: admin / brand_manager / no-role parity with `CanCreateCampaign`.
  [`campaign_test.go:36`](../../backend/internal/authz/campaign_test.go#L36)
