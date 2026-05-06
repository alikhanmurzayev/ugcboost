---
title: 'Backend chunk #5: PATCH /campaigns/{id} (admin-only)'
type: 'feature'
created: '2026-05-06'
status: 'done'
baseline_commit: '89487da'
context:
  - 'docs/standards/'
  - '_bmad-output/planning-artifacts/campaign-roadmap.md'
  - '_bmad-output/implementation-artifacts/archive/2026-05-06-spec-4-campaign-detail-backend.md'
---

<frozen-after-approval reason="human-owned intent — do not modify unless human renegotiates">

## Intent

**Problem:** Чанк #5 кампаний-роудмапа: после POST/GET админ не может изменить ни имя, ни TMA-ссылку у кампании. Без update-ручки фронт-страница edit (#8) и сценарии исправления опечаток / смены секретного TMA-URL не закрыты.

**Approach:** Admin-only `PATCH /campaigns/{id}` — full-replace `{name, tmaUrl}` (оба поля required, переиспользуют `domain.ValidateCampaignName / TmaURL` и openapi-`CampaignInput` schema из #3). Успех = `204 No Content`, без тела (фронту id известен из path, snapshot читается через `GET /campaigns/{id}` если нужен). Service-методы принимают доменный input-DTO `domain.CampaignInput { Name, TmaURL }` (новый, по паттерну `domain.CreatorApplicationInput`); `CreateCampaign` симметрично рефакторится на ту же сигнатуру. Service в `dbutil.WithTx`: read-snapshot через `GetByID` (для audit `old_value`), `Update`, audit-row `campaign_update` с full `*domain.Campaign` до и после в одной tx. Race на partial unique index `campaigns_name_active_unique` — EAFP 23505 → `domain.ErrCampaignNameTaken` → 409. Не найдено → `domain.ErrCampaignNotFound` → 404. Soft-deleted кампании остаются доступны для PATCH (как и для GET в #4) — отдельной фильтрации нет. Дополнительно в этом же PR: рефактор #4 — schema `CampaignResult → GetCampaignResult` (wrapper привязан к операции, как `GetCreatorResult` / `CampaignCreatedResult`).

## Boundaries & Constraints

**Always:**
- Полная загрузка `docs/standards/` и применение как hard rules.
- Слои handler → service → repository через `RepoFactory`; транзакция через `dbutil.WithTx`; audit-row пишется внутри той же tx.
- AuthZ — admin-only через новый `AuthzService.CanUpdateCampaign(ctx)`, проверка раньше любого DB-чтения (timing-safe).
- Repository `Update(ctx, id, name, tmaURL string) (*CampaignRow, error)`: `sq.Update(TableCampaigns).Set(name, tmaURL, updated_at=now()).Where(id=...).Suffix(returningClause(campaignSelectColumns))` через `dbutil.One`. EAFP в repo: `errors.As(*pgconn.PgError)` + `Code=="23505"` + `ConstraintName == CampaignsNameActiveUnique` → `domain.ErrCampaignNameTaken`. WHERE без фильтра `is_deleted` (повторяет семантику GET из #4).
- Service `UpdateCampaign(ctx, id string, in domain.CampaignInput) error` в `dbutil.WithTx`: (1) `cRepo.GetByID` — снимок старого, `sql.ErrNoRows` → `ErrCampaignNotFound`; (2) `cRepo.Update(ctx, id, in.Name, in.TmaURL)` — `sql.ErrNoRows` (race-delete) → `ErrCampaignNotFound`, `ErrCampaignNameTaken` пробрасывается, прочие — `fmt.Errorf("update campaign: %w", err)`; (3) `writeAudit(...campaign_update, oldCampaign, newCampaign)`. `logger.Info("campaign updated", "campaign_id", id)` после `WithTx`, не внутри callback'а. Возвращает только `error`.
- Service `CreateCampaign` рефакторится на сигнатуру `(ctx, in domain.CampaignInput) (*domain.Campaign, error)` — внутри `in.Name`/`in.TmaURL`, остальная логика без изменений. Repo Create с раздельными аргументами (data-layer без struct).
- Handler: `request.Id.String()` для path-param UUID; AuthZ — первой; granular validators дают trimmed-значения; handler собирает `domain.CampaignInput{Name, TmaURL}` и отдаёт в service; success → `api.UpdateCampaign204Response{}`.
- Code-gen: `make generate-api` после правки `openapi.yaml` — единственный путь обновления `*.gen.go` / frontend schemas / apiclient/testclient. `make generate-mocks` после правки service-interface'ов.
- Audit `old_value` / `new_value` = JSON всей `*domain.Campaign` (snake_case json-теги); при добавлении поля audit обновляется автоматически.

**Ask First:**
- Изменение публичной модели API (`Campaign`, `CampaignInput`) — нет в скоупе.
- Partial-PATCH (optional fields) — нет в скоупе, делаем full-replace.
- Запрет PATCH для soft-deleted записей — нет в скоупе.
- Pre-fetch `oldCampaign` через отдельный SELECT — намеренное решение ради audit `old_value` (альтернатива через CTE+RETURNING сложнее без выигрыша).

**Never:**
- Партиальное обновление (только одно из полей) — оба поля required в `CampaignInput`, без `omitempty`.
- Прямой `chi.URLParam` / ручной uuid-parse в handler (только generated-wrapper).
- 422 за невалидный UUID — wrapper отдаёт это сам.
- `pgx.ErrNoRows` (только `sql.ErrNoRows`); `math/rand` (только `crypto/rand`).
- `logger.Info` ВНУТРИ `WithTx` callback'а; fire-and-forget audit.
- `update_at` вычисляется в Go — обновляется через `sq.Expr("now()")` в SQL.
- Новая миграция — таблица и partial unique index уже на месте с #3.
- E2E race-тест на UPDATE — стандарт `backend-testing-e2e.md` § Race-сценарии требует один race-тест на partial-unique index (уже есть `TestCreateCampaign_RaceUniqueName`); дублирование = flake-риск без ценности.

## I/O & Edge-Case Matrix

| Сценарий | Вход / Состояние | Ответ | Обработка ошибок |
|---|---|---|---|
| Happy path (живая) | admin, существующий id, новые `{name, tmaUrl}` | 204 No Content + audit `campaign_update` (`old`/`new` — full Campaign) + БД-строка с `updated_at = now()` | — |
| Happy path (soft-deleted) | admin, soft-deleted id | 204 + БД-строка обновлена (`is_deleted` остаётся true) + audit | как live |
| Не аутентифицирован | без токена | 401 | middleware |
| brand-manager | manager-токен | 403 (до DB-чтения) | `CanUpdateCampaign` → `ErrForbidden` |
| Не найден | admin, несуществующий UUID | 404 `CAMPAIGN_NOT_FOUND` + «Кампания не найдена.» | service: `sql.ErrNoRows` (от GetByID или Update RETURNING) → `ErrCampaignNotFound` |
| Невалидный UUID в path | admin, `id=not-a-uuid` | 4xx от generated wrapper | strict-server `RequestErrorHandlerFunc` |
| Пустое имя | `name=""` или whitespace | 422 `CAMPAIGN_NAME_REQUIRED` | `ValidateCampaignName` |
| Длинное имя | `len(name) > 255` | 422 `CAMPAIGN_NAME_TOO_LONG` | `ValidateCampaignName` |
| Пустой URL | `tmaUrl=""` или whitespace | 422 `CAMPAIGN_TMA_URL_REQUIRED` | `ValidateCampaignTmaURL` |
| Длинный URL | `len(tmaUrl) > 2048` | 422 `CAMPAIGN_TMA_URL_TOO_LONG` | `ValidateCampaignTmaURL` |
| Имя занято | UPDATE на имя другой live-кампании | 409 `CAMPAIGN_NAME_TAKEN` | repo: pgErr 23505 → `ErrCampaignNameTaken` |
| Имя = текущему | `name = oldCampaign.Name` | 204 — UPDATE проходит (PG не считает self-row), audit пишет old==new | — |
| БД-ошибка | pool падает | 500 `INTERNAL_ERROR` (default) | service: проброс с `fmt.Errorf` |

</frozen-after-approval>

## Code Map

- `backend/api/openapi.yaml` -- (1) refactor: `CampaignResult → GetCampaignResult`; (2) добавить `PATCH /campaigns/{id}` (`operationId: updateCampaign`, request `CampaignInput`, ответы 204/401/403/404/409/422/default).
- `backend/internal/domain/campaign.go` -- добавить `type CampaignInput struct { Name, TmaURL string }`.
- `backend/internal/repository/campaign.go` -- метод `Update(ctx, id, name, tmaURL)`: `sq.Update` + `now()` + `RETURNING`; EAFP 23505 → `ErrCampaignNameTaken`; `sql.ErrNoRows` пробрасывается. Расширить `CampaignRepo` interface.
- `backend/internal/repository/campaign_test.go` -- `Test_campaignRepository_Update`: success live, success soft-deleted, name taken, not found, db error.
- `backend/internal/authz/campaign.go` -- `CanUpdateCampaign(ctx)` (admin-only по паттерну `CanGetCampaign`).
- `backend/internal/authz/campaign_test.go` -- расширить тестами `CanUpdateCampaign` (admin/brand_manager/no-role).
- `backend/internal/service/audit_constants.go` -- `AuditActionCampaignUpdate = "campaign_update"`.
- `backend/internal/service/campaign.go` -- (1) refactor: `CreateCampaign` принимает `domain.CampaignInput`; (2) новый `UpdateCampaign(ctx, id, in)` с pre-fetch + audit в одной tx.
- `backend/internal/service/campaign_test.go` -- (1) refactor existing create-tests под input-struct; (2) новый `TestCampaignService_UpdateCampaign` (5 t.Run + success).
- `backend/internal/service/mocks/` -- `make generate-mocks` после правки interface'ов.
- `backend/internal/handler/server.go` -- (1) refactor `CampaignService.CreateCampaign` сигнатура; (2) расширить `CampaignService` методом `UpdateCampaign`; (3) расширить `AuthzService` методом `CanUpdateCampaign`.
- `backend/internal/handler/campaign.go` -- (1) refactor: `CreateCampaign` собирает `domain.CampaignInput`; (2) новый `UpdateCampaign` handler.
- `backend/internal/handler/campaign_test.go` -- (1) refactor: `api.CampaignResult → api.GetCampaignResult` (2 строки в `TestServer_GetCampaign`); (2) refactor existing create-tests под input-struct; (3) новый `TestServer_UpdateCampaign` (8 t.Run).
- `backend/e2e/campaign/campaign_test.go` -- расширить header-комментарий нарративом про PATCH; добавить 9 t.Run в `TestCampaignCRUD` (401/403/404/422-grid/409/success).

## Tasks & Acceptance

**Execution (порядок строго по слоям):**

- [x] `backend/api/openapi.yaml` -- (1) Refactor: переименовать schema `CampaignResult → GetCampaignResult` (definition + 200-response в `getCampaign`). (2) Добавить `PATCH /campaigns/{id}`: `operationId: updateCampaign`, `tags: [campaigns]`, `security: [bearerAuth]`, path-param `id` (`format: uuid`), `requestBody: $ref CampaignInput`, ответы `204 (без body) / 401 ErrorResponse / 403 Forbidden / 404 ErrorResponse / 409 ErrorResponse / 422 ErrorResponse / default UnexpectedError`. Никаких новых schemas.
- [x] **`make generate-api`** -- регенерация всех `*.gen.go` + frontend schemas + apiclient/testclient. После регенерации `api.CampaignResult` исчезает, появляется `api.GetCampaignResult` и `api.UpdateCampaign*` объекты.
- [x] `backend/internal/domain/campaign.go` -- Добавить `type CampaignInput struct { Name, TmaURL string }` рядом с `domain.Campaign`. Без json-тегов (struct не сериализуется).
- [x] `backend/internal/service/audit_constants.go` -- Добавить `AuditActionCampaignUpdate = "campaign_update"` рядом с `AuditActionCampaignCreate`.
- [x] `backend/internal/repository/campaign.go` -- (1) Расширить `CampaignRepo` интерфейс методом `Update(ctx, id, name, tmaURL string) (*CampaignRow, error)`. (2) Реализовать: `sq.Update(TableCampaigns).Set(CampaignColumnName, name).Set(CampaignColumnTmaURL, tmaURL).Set(CampaignColumnUpdatedAt, sq.Expr("now()")).Where(sq.Eq{CampaignColumnID: id}).Suffix(returningClause(campaignSelectColumns))` → `dbutil.One[CampaignRow]`. EAFP: `errors.As(*pgconn.PgError)` + `Code=="23505"` + `ConstraintName == CampaignsNameActiveUnique` → `domain.ErrCampaignNameTaken`. `sql.ErrNoRows` пробрасывается as-is.
- [x] `backend/internal/repository/campaign_test.go` -- `Test_campaignRepository_Update`: `success live` (точный SQL литералом + args=[name, tmaURL, id]; RETURNING с `is_deleted=false`); `success soft-deleted` (RETURNING с `is_deleted=true` — фиксирует «no is_deleted filter» контракт); `name taken` (pgxmock возвращает `*pgconn.PgError{Code:"23505", ConstraintName: CampaignsNameActiveUnique}` → `errors.Is ErrCampaignNameTaken`); `not found` (мок `pgx.ErrNoRows` для пустого RETURNING → `errors.Is sql.ErrNoRows`); `db error` (generic → проброс as-is).
- [x] `backend/internal/authz/campaign.go` -- Добавить `func (a *AuthzService) CanUpdateCampaign(ctx context.Context) error` — `if middleware.RoleFromContext(ctx) != api.Admin { return domain.ErrForbidden }; return nil`. GoDoc-однострочник «admin-only».
- [x] `backend/internal/authz/campaign_test.go` -- Расширить тестами `CanUpdateCampaign`: admin → nil, brand_manager → ErrForbidden, no-role → ErrForbidden.
- [x] `backend/internal/service/campaign.go` -- (1) **Refactor**: `CreateCampaign(ctx, name, tmaURL string)` → `CreateCampaign(ctx context.Context, in domain.CampaignInput) (*domain.Campaign, error)` (внутри `in.Name`/`in.TmaURL`, остальная логика без изменений). (2) Добавить `func (s *CampaignService) UpdateCampaign(ctx context.Context, id string, in domain.CampaignInput) error`: `dbutil.WithTx` → (a) `cRepo.GetByID(ctx, id)` → `sql.ErrNoRows → ErrCampaignNotFound`; маппим в `oldCampaign := campaignRowToDomain(...)`; (b) `cRepo.Update(ctx, id, in.Name, in.TmaURL)` → `sql.ErrNoRows → ErrCampaignNotFound`; `ErrCampaignNameTaken` пробрасывается; прочие — `fmt.Errorf("update campaign: %w", err)`; маппим в `newCampaign := campaignRowToDomain(...)`; (c) `writeAudit(ctx, aRepo, AuditActionCampaignUpdate, AuditEntityTypeCampaign, newCampaign.ID, oldCampaign, newCampaign)`. После `WithTx` — `s.logger.Info(ctx, "campaign updated", "campaign_id", id)`. Возвращает только `error`.
- [x] `backend/internal/service/campaign_test.go` -- (1) **Refactor**: `TestCampaignService_CreateCampaign` — обновить вызовы под input-struct (тестовая логика без изменений). (2) Новый `TestCampaignService_UpdateCampaign`: `not found before update` (GetByID → `sql.ErrNoRows`; Update + audit не вызывались), `not found between get and update` (GetByID success, Update → `sql.ErrNoRows`; audit не вызывался), `name taken` (Update → `ErrCampaignNameTaken`; audit не вызывался), `update generic error` (Update → generic; wrapped; audit не вызывался), `audit error rolls back` (audit error → проброс), `success` (захват args через `mock.Run`: `Action == AuditActionCampaignUpdate`, `EntityType == AuditEntityTypeCampaign`, `EntityID == newCampaign.ID`, `OldValue` JSONEq `json.Marshal(oldCampaign)`, `NewValue` JSONEq `json.Marshal(newCampaign)`; `require.NoError(err)` финальный).
- [x] **`make generate-mocks`** -- mockery regen после правки `CampaignService` interface (новый метод + правка сигнатуры Create).
- [x] `backend/internal/handler/server.go` -- (1) Refactor: сигнатура `CampaignService.CreateCampaign` → `(ctx, in domain.CampaignInput) (*domain.Campaign, error)`. (2) Расширить `CampaignService` методом `UpdateCampaign(ctx, id string, in domain.CampaignInput) error`. (3) Расширить `AuthzService` методом `CanUpdateCampaign(ctx) error`.
- [x] `backend/internal/handler/campaign.go` -- (1) Refactor: `CreateCampaign` — после `ValidateCampaignName/TmaURL` собрать `domain.CampaignInput{Name: name, TmaURL: tmaURL}` и передать в service. (2) Новый `func (s *Server) UpdateCampaign(ctx context.Context, request api.UpdateCampaignRequestObject) (api.UpdateCampaignResponseObject, error)`: `s.authzService.CanUpdateCampaign(ctx)` → проброс; `name, err := domain.ValidateCampaignName(request.Body.Name)` → проброс; `tmaURL, err := domain.ValidateCampaignTmaURL(request.Body.TmaUrl)` → проброс; `if err := s.campaignService.UpdateCampaign(ctx, request.Id.String(), domain.CampaignInput{Name: name, TmaURL: tmaURL}); err != nil { return nil, err }`; success → `api.UpdateCampaign204Response{}`. GoDoc-однострочник «admin-only PATCH; success returns 204».
- [x] `backend/internal/handler/campaign_test.go` -- (1) Refactor: 2 строки `api.CampaignResult → api.GetCampaignResult` в `TestServer_GetCampaign` success-сценарии. (2) Refactor: `TestServer_CreateCampaign` mock expectations на `CampaignService.CreateCampaign` — точные args через `domain.CampaignInput{Name: ..., TmaURL: ...}`. (3) Новый `TestServer_UpdateCampaign`: forbidden non-admin (authz отказал → 403, service не вызывается); whitespace name → 422 + `CAMPAIGN_NAME_REQUIRED` (service не вызывается); name>255 → 422 + `CAMPAIGN_NAME_TOO_LONG`; whitespace tmaUrl → 422 + `CAMPAIGN_TMA_URL_REQUIRED`; tmaUrl>2048 → 422 + `CAMPAIGN_TMA_URL_TOO_LONG`; not found → 404 + `CAMPAIGN_NOT_FOUND`; name taken → 409 + `CAMPAIGN_NAME_TAKEN`; service generic → 500; success 204 (`require.Equal(t, http.StatusNoContent, w.Code)` + `require.Zero(t, w.Body.Len())`). Mockery с точными args (id-string, `domain.CampaignInput{Name, TmaURL}`).
- [x] **Manual local sanity check (HALT)** -- `make migrate-up && make start-backend`. Получить admin-/brand-manager-токены через test-API (паттерн из #4). Curl-сценарии: (a) **happy** `POST /campaigns name=A1, tmaUrl=u1` → захватить id; `PATCH /campaigns/{id}` admin-токеном `name=A2, tmaUrl=u2` → 204, body пустое; `GET /campaigns/{id}` → 200 с обновлёнными `name=A2, tmaUrl=u2, updatedAt > createdAt, isDeleted=false`; `SELECT action, entity_type, entity_id, old_value, new_value FROM audit_logs WHERE entity_id='{id}' ORDER BY created_at DESC LIMIT 2` → две строки (`campaign_create`, `campaign_update`); у update'а `old_value` содержит `A1/u1`, `new_value` содержит `A2/u2`. (b) **404** `PATCH /campaigns/<random uuid>` → 404 + `CAMPAIGN_NOT_FOUND`. (c) **409** создать B (`name=A2`), `PATCH A name=A3` → 204; `PATCH B name=A3` → 409 `CAMPAIGN_NAME_TAKEN`. (d) **422-grid** `name=""` / `name=<256 chars>` / `tmaUrl=""` / `tmaUrl=<2049 chars>` → каждый со своим granular кодом. (e) **403** brand-manager → 403 `FORBIDDEN`. (f) **401** без токена → 401. **HALT и сообщить пользователю фактические HTTP-коды + JSON-тела (для 4xx) до старта e2e.**
- [x] `backend/e2e/campaign/campaign_test.go` -- (1) Расширить header-комментарий новым абзацем про PATCH-семантику (нарратив, не bullets). (2) Добавить в `TestCampaignCRUD` t.Run-ы: `update unauthenticated → 401` (без токена через `testutil.NewAPIClient`); `update brand_manager forbidden → 403` (manager-токен); `update not found → 404 CAMPAIGN_NOT_FOUND`; `update empty name → 422 CAMPAIGN_NAME_REQUIRED`; `update name too long → 422 CAMPAIGN_NAME_TOO_LONG`; `update empty tmaUrl → 422 CAMPAIGN_TMA_URL_REQUIRED`; `update tmaUrl too long → 422 CAMPAIGN_TMA_URL_TOO_LONG`; `update name taken → 409 CAMPAIGN_NAME_TAKEN` (создать A и B sequential, PATCH B на имя A — без барьеров и горутин); `update success` (POST → PATCH с новыми name/tmaUrl → ассерт `204 NoContent` без body; затем GET по тому же id → проверка что в БД обновлённое значение: name/tmaUrl новые, isDeleted=false, `updatedAt.After(createdAt)`; затем `FindAuditEntry` action `campaign_update`, `old_value`/`new_value` через `json.Marshal/Unmarshal` round-trip + проверка name/tma_url до и после). Все `t.Parallel()`. Cleanup через `testutil.RegisterCampaignCleanup`. Race-тест на UPDATE НЕ добавляем — `TestCreateCampaign_RaceUniqueName` уже покрывает partial-unique index.

**Acceptance Criteria:**
- Given `make build-backend && make lint-backend`, when запускаются, then оба зелёные.
- Given `make test-unit-backend && make test-unit-backend-coverage`, when запускаются, then оба зелёные; per-method coverage gate ≥80% сохраняется на изменённых файлах в `handler/service/repository/authz`.
- Given `make test-e2e-backend`, when запускается, then `TestCampaignCRUD` (с PATCH-сценариями) + `TestCreateCampaign_RaceUniqueName` зелёные; cleanup-stack удаляет созданные кампании.
- Given поднятый бэк локально, when admin PATCH /campaigns/{id} с валидными новыми name/tmaUrl, then 204 (без body); GET возвращает обновлённые значения; в БД ровно одна строка для id (UPDATE, не INSERT) с `updated_at > created_at`; в `audit_logs` ровно одна новая строка `action='campaign_update'` с `old_value` (JSON старого) и `new_value` (JSON нового).
- Given две живые кампании A и B, when sequential PATCH B на имя A, then 409 + `code:"CAMPAIGN_NAME_TAKEN"` + actionable RU-message; B сохраняет старое имя.
- Given несуществующий uuid, when admin PATCH /campaigns/{id}, then 404 + `code:"CAMPAIGN_NOT_FOUND"` + «Кампания не найдена.».

## Spec Change Log

<!-- Append-only. Populated by step-04 during review loops. Empty until the first bad_spec loopback. -->

## Design Notes

**Response 204 No Content.** PATCH не возвращает тело — фронту id известен, snapshot он берёт через `GET /campaigns/{id}`. Зеркалит решение #3 «narrow POST response to id-only»: read-контракт в одном месте, мутирующие ручки не дублируют его.

**Refactor `CampaignResult → GetCampaignResult`.** В свежих ручках проекта wrapper привязан к операции, не к сущности (`GetCreatorResult`, `CampaignCreatedResult`, `CreatorsListResult`). Делается одним рефактор-коммитом до PATCH-логики.

**`domain.CampaignInput` как единый input-DTO.** Сервисные методы принимают `domain.CampaignInput { Name, TmaURL }` — паттерн `domain.CreatorApplicationInput` (см. `service/creator_application.go:103`). При росте полей кампании (PublicBrief, PrivateBrief, EventDate из brainstorming-roadmap'а) сигнатуры service не нужно перетрясать на каждом callsite. `CreateCampaign` рефакторится симметрично — legacy с россыпью аргументов не оставляем. Repo на data-layer остаётся с раздельными `name, tmaURL` — там struct не нужен.

**Pre-fetch `oldCampaign`** нужен исключительно для audit `old_value`. Альтернатива (CTE с `OLD AS SELECT` + `RETURNING`) усложнила бы repo — отказались. Audit пишется в той же `WithTx`: если UPDATE прошёл, audit гарантированно есть; если откат — нет ни UPDATE, ни audit.

**Race-семантика UPDATE отличается от CREATE.** PG при UPDATE на partial unique не считает self-row, так что PATCH на текущее имя проходит. EAFP-обработка 23505 в repo живёт через тот же `CampaignsNameActiveUnique`-switch и однажды покрыта `TestCreateCampaign_RaceUniqueName` — отдельный e2e на UPDATE-race не повышает confidence, повышает flake-риск.

Audit-payload-пример:
```
old_value: {"id":"<uuid>","name":"Promo X","tma_url":"https://tma.../old","is_deleted":false,"created_at":"...","updated_at":"..."}
new_value: {"id":"<uuid>","name":"Promo Y","tma_url":"https://tma.../new","is_deleted":false,"created_at":"...","updated_at":"..."}
```

## Verification

**Commands:**
- `make generate-api` -- expected: успех; `git diff --stat` показывает обновлённые `*.gen.go` + frontend schemas + apiclient/testclient
- `make generate-mocks` -- expected: успех; обновлённые `*/mocks/*.go`
- `make build-backend` -- expected: компиляция чистая
- `make lint-backend` -- expected: zero issues
- `make test-unit-backend` -- expected: зелёные с `-race`
- `make test-unit-backend-coverage` -- expected: per-method gate зелёный
- `make test-e2e-backend` -- expected: `TestCampaignCRUD` + `TestCreateCampaign_RaceUniqueName` зелёные

## Suggested Review Order

**Контракт API**

- Точка входа: PATCH-операция (204/401/403/404/409/422), переиспользует `CampaignInput`.
  [`openapi.yaml:1056`](../../backend/api/openapi.yaml#L1056)

- Refactor wrapper-схемы — `CampaignResult` → `GetCampaignResult` под operation-bound naming.
  [`openapi.yaml:2483`](../../backend/api/openapi.yaml#L2483)

**Сервисный флоу UPDATE**

- Pre-fetch + Update + audit в одной `WithTx`; маппинг `sql.ErrNoRows`/`ErrCampaignNameTaken`.
  [`service/campaign.go:79`](../../backend/internal/service/campaign.go#L79)

- EAFP 23505 + `CampaignsNameActiveUnique` → `ErrCampaignNameTaken`; без `is_deleted`-фильтра в WHERE.
  [`repository/campaign.go:105`](../../backend/internal/repository/campaign.go#L105)

- Audit-action для PATCH-флоу.
  [`audit_constants.go:21`](../../backend/internal/service/audit_constants.go#L21)

**Handler**

- Authz first → granular validators → собрать `domain.CampaignInput` → 204.
  [`handler/campaign.go:88`](../../backend/internal/handler/campaign.go#L88)

- Симметричный refactor `CreateCampaign` под тот же input-DTO.
  [`handler/campaign.go:41`](../../backend/internal/handler/campaign.go#L41)

**Контракт сервиса и domain**

- `CampaignService` обзавёлся `UpdateCampaign`; `CreateCampaign` сменил сигнатуру на `domain.CampaignInput`.
  [`handler/server.go:97`](../../backend/internal/handler/server.go#L97)

- AuthzService расширен `CanUpdateCampaign`.
  [`handler/server.go:57`](../../backend/internal/handler/server.go#L57)

- Новый input-DTO для service-методов.
  [`domain/campaign.go:30`](../../backend/internal/domain/campaign.go#L30)

**AuthZ**

- Admin-only gate для PATCH (тот же паттерн, что у `CanGetCampaign`).
  [`authz/campaign.go:31`](../../backend/internal/authz/campaign.go#L31)

**Tests (peripherals)**

- Repo-тесты Update: success live / soft-deleted / 23505 / unrelated 23505 / not-found / db-error.
  [`repository/campaign_test.go:158`](../../backend/internal/repository/campaign_test.go#L158)

- Service-тесты UpdateCampaign: 6 t.Run + success с captured-input через `mock.Run` (JSONEq на old/new).
  [`service/campaign_test.go:207`](../../backend/internal/service/campaign_test.go#L207)

- Handler-тесты: forbidden / валидационная сетка / not-found / name-taken / 500 / success 204 без body.
  [`handler/campaign_test.go:273`](../../backend/internal/handler/campaign_test.go#L273)

- AuthZ-тесты: admin / brand_manager / no-role.
  [`authz/campaign_test.go:58`](../../backend/internal/authz/campaign_test.go#L58)

- E2E расширен 9 PATCH-сценариями + обновлён header-нарратив.
  [`e2e/campaign_test.go:274`](../../backend/e2e/campaign/campaign_test.go#L274)

