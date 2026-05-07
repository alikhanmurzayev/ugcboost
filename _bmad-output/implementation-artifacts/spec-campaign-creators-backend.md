---
title: 'campaign_creators backend (chunk 10) — миграция + add/remove/list'
type: feature
created: '2026-05-07'
status: done
baseline_commit: cab4ff14482532785d5efbb25c4cb7b11c309c7b
context:
  - docs/standards/
  - _bmad-output/planning-artifacts/design-campaign-creator-flow.md
  - _bmad-output/implementation-artifacts/intent-campaign-creators-backend.md
---

<frozen-after-approval reason="human-owned intent — do not modify unless human renegotiates">

## Intent

**Problem:** Админ не может управлять составом креаторов в кампании — нет таблицы M2M, нет ручек add/remove/list. Без этого невозможно стартовать рассылку приглашений (chunk 12) и приём решений через TMA (chunk 14).

**Approach:** Бэк-чанк 10 из `campaign-roadmap.md`. Миграция таблицы `campaign_creators` со state-полем (4 значения: `planned/invited/declined/agreed`) + 3 admin-ручки (POST batch add, DELETE single remove, GET list без пагинации). State-transitions кроме `→ planned` — НЕ в этом чанке.

## Boundaries & Constraints

**Always:**
- Полная загрузка `docs/standards/` перед кодом — все правила hard rules.
- Generated code (`*.gen.go`, frontend `schema.ts`, e2e clients) меняется ТОЛЬКО через `make generate-api` после правки `backend/api/openapi.yaml`.
- Audit per-creator внутри той же `WithTx` (стандарт `backend-transactions.md`).
- Pre-fetch кампании ДО `WithTx` (через pool, без лишней tx); soft-deleted кампания → 404 на все 3 ручки.
- Strict-422 на batch валидацию: любой невалидный `creator_id` или конфликт → rollback всего батча, ни одной вставки.
- Per-method coverage gate ≥80% (`make test-unit-backend-coverage`).
- Status-константы в коде ставятся сервисом — DEFAULT в БД на `status` запрещён (стандарт `backend-repository.md § Целостность данных`).

**Ask First:**
- Любые отклонения от полей/семантики design-документа.
- Решения уровня state-machine за пределами 4 статусов (`planned/invited/declined/agreed`) и переходов чанка 10 (`Add → planned`, `Remove ← any except agreed`).

**Never:**
- Бот-нотификации, A4/A5 ручки, TMA, T1/T2 ручки, secret_token-парсинг, partial-success delivery — это chunks 12/14.
- PATCH `tma_url` lock после рассылок — chunk 12.
- Race-тест на concurrent insert (по design'у — non-concurrent кейса достаточно).
- Frontend (chunk 11).
- 422-from-agreed в e2e — нет business-flow для `agreed` в этом чанке (e2e добавим в chunk 14).
- Хардкод-литералы названий колонок / таблиц / ролей / status'ов вне `*_test.go`.
- DEFAULT для `status` в миграции; CHECK с regex для format в миграции.

## I/O & Edge-Case Matrix

| Scenario | Input / State | Expected Output / Behavior | Error Handling |
|---|---|---|---|
| Add happy batch | admin, валидный `id`, `creatorIds:[c1,c2]`, кампания живая | 201 `{data:{items:[...]}}` (2 строки status=planned, counts=0); 2 audit-rows `campaign_creator_add` per creator | — |
| Add re-add | в кампании уже есть c1, batch `[c1,c2]` | 422 `CREATOR_ALREADY_IN_CAMPAIGN`, БД и audit без изменений | repo: pgErr 23505 + `ConstraintName == campaign_creators_campaign_creator_unique` → `domain.ErrCreatorAlreadyInCampaign` (*ValidationError) |
| Add invalid creator_id | один из id не существует в `creators` | 422 `CREATOR_NOT_FOUND`, rollback | repo: pgErr 23503 + creator-FK → `domain.ErrCampaignCreatorCreatorNotFound` (*ValidationError) |
| Add empty creatorIds | `creatorIds:[]` | 422 `CAMPAIGN_CREATOR_IDS_REQUIRED` | handler-валидация до сервиса |
| Add duplicate ids in batch | `creatorIds:[c1,c1]` | 422 `CAMPAIGN_CREATOR_IDS_DUPLICATES` | handler dedup до сервиса |
| Add к soft-deleted campaign | `is_deleted=true` | 404 `CAMPAIGN_NOT_FOUND` | service pre-fetch |
| Add к несуществующей campaign | id неизвестен | 404 | service pre-fetch (sql.ErrNoRows) |
| Remove happy | связь существует, status≠agreed | 204; audit-row `campaign_creator_remove` с `OldValue=*CampaignCreator`, `NewValue=nil` | — |
| Remove from agreed | status=agreed | 422 `CAMPAIGN_CREATOR_REMOVE_AFTER_AGREED` | service status-guard |
| Remove несуществующей связи | пары (campaign_id, creator_id) нет | 404 `CAMPAIGN_CREATOR_NOT_FOUND` | service: sql.ErrNoRows → `domain.ErrCampaignCreatorNotFound` |
| Remove из soft-deleted/missing campaign | | 404 `CAMPAIGN_NOT_FOUND` | service pre-fetch |
| List happy | живая кампания + N связей | 200 `{data:{items:[N]}}` ORDER BY created_at ASC, id ASC | — |
| List на пустой кампании | связей нет | 200 `{data:{items:[]}}` | — |
| List на soft-deleted/missing | | 404 | service pre-fetch |
| Авторизация | non-admin / no-auth | 403 / 401 | `authzService.Can*` через `requireRole(api.Admin)` |

</frozen-after-approval>

## Code Map

- `backend/api/openapi.yaml` -- добавить 2 пути (`/campaigns/{id}/creators`, `/campaigns/{id}/creators/{creatorId}`) + 5 schemas (`CampaignCreator`, `CampaignCreatorStatus` enum, `AddCampaignCreatorsInput`, 2 result-wrappers); под существующим `tags: [campaigns]`.
- `backend/migrations/<timestamp>_campaign_creators.sql` -- новая таблица + UNIQUE `(campaign_id, creator_id)` + status CHECK + explicit FK имена `campaign_creators_campaign_id_fk`/`_creator_id_fk` для disambiguation в repo.
- `backend/internal/domain/campaign_creator.go` -- новый файл: `CampaignCreator` struct (snake_case JSON для audit), 4 status-константы, 4 sentinel/validation ошибки.
- `backend/internal/domain/errors.go` -- добавить 6 новых кодов: `CodeCampaignCreatorIdsRequired`, `CodeCampaignCreatorIdsTooMany`, `CodeCampaignCreatorIdsDuplicates`, `CodeCampaignCreatorNotFound`, `CodeCreatorAlreadyInCampaign`, `CodeCampaignCreatorRemoveAfterAgreed` (плюс переиспользуется существующий `CodeCreatorNotFound`).
- `backend/internal/repository/campaign_creator.go` -- новый файл: row + tags, константы колонок/таблицы/constraints, интерфейс `CampaignCreatorRepo`, реализация (4 метода).
- `backend/internal/repository/factory.go` -- метод `NewCampaignCreatorRepo(db) CampaignCreatorRepo`.
- `backend/internal/service/campaign_creator.go` -- новый файл: `CampaignCreatorService`, `CampaignCreatorRepoFactory` interface, методы `Add/Remove/List` + helper `campaignCreatorRowToDomain`.
- `backend/internal/service/audit_constants.go` -- добавить `AuditActionCampaignCreatorAdd`, `AuditActionCampaignCreatorRemove`, `AuditEntityTypeCampaignCreator`.
- `backend/internal/handler/server.go` -- добавить `CampaignCreatorService` interface + поле в `Server` + параметр в `NewServer` (mirror `CampaignService`).
- `backend/internal/handler/campaign_creator.go` -- новый файл: 3 strict-server метода + mapping helper `domainCampaignCreatorToAPI`.
- `backend/internal/handler/response.go` -- добавить case `errors.Is(err, domain.ErrCampaignCreatorNotFound)` → 404 + `CodeCampaignCreatorNotFound`.
- `backend/internal/authz/campaign_creator.go` -- 3 метода `CanAddCampaignCreators/CanRemoveCampaignCreator/CanListCampaignCreators` через `requireRole(api.Admin)`.
- `backend/cmd/api/main.go` -- инстанцировать `service.NewCampaignCreatorService` и передать в `handler.NewServer`.
- `backend/internal/repository/campaign_creator_test.go` + `service/campaign_creator_test.go` + `handler/campaign_creator_test.go` + `authz/campaign_creator_test.go` -- unit-тесты (≥80% per-method).
- `backend/e2e/campaign_creator/campaign_creator_test.go` -- e2e (Russian narrative header, 3 `Test*` функции).

## Tasks & Acceptance

**Execution:**
- [x] `backend/api/openapi.yaml` -- добавить пути и schemas (см. Code Map). Описание ручек явно фиксирует: «soft-deleted campaigns are treated as not found (404) for these endpoints, unlike GET /campaigns/{id}», maxItems=200 на `creatorIds`. Формат каждого 4xx — через существующий `ErrorResponse`.
- [x] `make generate-api` -- регенерит server.gen.go, e2e clients, `frontend/{web,tma,landing}/src/api/generated/schema.ts`, `frontend/e2e/types/schema.ts`. Generated файлы коммитятся.
- [x] `backend/migrations/<timestamp>_campaign_creators.sql` -- по тексту intent-файла (header-комментарий в стиле `creators.sql`); explicit FK имена обязательны.
- [x] `backend/internal/domain/campaign_creator.go` -- struct + статус-константы + named errors (`ErrCreatorAlreadyInCampaign` = `*ValidationError`, `ErrCampaignCreatorRemoveAfterAgreed` = `*ValidationError`, `ErrCampaignCreatorNotFound` = sentinel `errors.New`, `ErrCampaignCreatorCreatorNotFound` = `*ValidationError` с `CodeCreatorNotFound`).
- [x] `backend/internal/domain/errors.go` -- добавить 6 новых `Code*` (см. Code Map).
- [x] `backend/internal/repository/campaign_creator.go` -- row+теги, константы, repo. `Add`: pgErr 23505/unique → `ErrCreatorAlreadyInCampaign`; 23503/creator-FK → `ErrCampaignCreatorCreatorNotFound`; 23503/campaign-FK → `domain.ErrCampaignNotFound`. `ListByCampaign`: ORDER BY created_at ASC, id ASC. `DeleteByID`: `sql.ErrNoRows` если 0 rows.
- [x] `backend/internal/repository/factory.go` -- `NewCampaignCreatorRepo`.
- [x] `backend/internal/service/audit_constants.go` -- 3 константы.
- [x] `backend/internal/service/campaign_creator.go` -- сервис. `Add`: pre-fetch campaign (sql.ErrNoRows / IsDeleted → `ErrCampaignNotFound`) → `WithTx` loop `Add` per creator, audit per creator (полный snapshot в NewValue), на любой error rollback всего batch'а → success log после `WithTx`. `Remove`: pre-fetch campaign → `WithTx`: `GetByCampaignAndCreator` (sql.ErrNoRows → `ErrCampaignCreatorNotFound`) → status-guard (`agreed` → `ErrCampaignCreatorRemoveAfterAgreed`) → `DeleteByID` → audit (OldValue=snapshot, NewValue=nil). `List`: pre-fetch campaign → `ListByCampaign` (без tx, через pool).
- [x] `backend/internal/handler/server.go` -- добавить `CampaignCreatorService` interface + поле + параметр.
- [x] `backend/internal/handler/response.go` -- case для `ErrCampaignCreatorNotFound` → 404.
- [x] `backend/internal/handler/campaign_creator.go` -- 3 метода + helper. `AddCampaignCreators`: authz → empty/duplicate валидация в handler → service.Add → 201 с items. `RemoveCampaignCreator`: authz → service.Remove → 204. `ListCampaignCreators`: authz → service.List → 200 + items.
- [x] `backend/internal/authz/campaign_creator.go` -- 3 admin-only метода.
- [x] `backend/cmd/api/main.go` -- проводка в `NewServer`.
- [x] Unit-тесты: repo (SQL-assert через capture, все pgErr-ветки, ORDER BY), service (все ветки в матрице + audit propagation + rollback), handler (authz forbidden, empty/duplicate валидация, happy + service-error propagation), authz (admin OK, brand_manager 403, no-role 403). Per-method coverage ≥80%.
- [x] E2E `backend/e2e/campaign_creator/campaign_creator_test.go` -- Russian narrative header, `t.Parallel()`, `testutil.FindAuditEntry` (для проверки payload OldValue/NewValue) или `AssertAuditEntry` (для проверки факта существования) — для всех mutate-ручек. 3 `Test*`: `TestAddCampaignCreators` / `TestRemoveCampaignCreator` / `TestListCampaignCreators`. Cleanup defer-stack: campaign_creators (через A2 для активных) → creators → campaign. Динамические поля сравниваются через подмену + `require.Equal` целиком.
- [x] `make generate-mocks` -- регенерит мок'и для `CampaignCreatorRepo` и `CampaignCreatorService`.

**Acceptance Criteria:**
- Given чистая БД, when `make migrate-up`, then миграция `campaign_creators` применяется без ошибок; goose-Down успешно дропает таблицу.
- Given backend поднят локально + admin token, when `POST /campaigns/{id}/creators` с валидным batch, then 201 + items в ответе совпадают с записями в `campaign_creators` + audit-rows `campaign_creator_add` появляются в `audit_logs` с `actor_id=admin user_id`, `entity_type='campaign_creator'`, `new_value` JSON содержит полный snapshot.
- Given валидный сервис, when запускается `make test-unit-backend && make test-unit-backend-coverage`, then все тесты зелёные и gate ≥80% per-method не нарушен.
- Given backend и БД готовы, when запускается `make test-e2e-backend`, then e2e в `campaign_creator/` зелёные на чистой и накопленной БД.
- Given правки в `openapi.yaml`, when `make generate-api`, then generated файлы перегенерированы и коммитятся в том же PR.
- Given `make lint-backend`, when запускается, then без ошибок (включая depguard, golangci.yml).

## Verification

**Commands:**
- `make build-backend` -- expected: успешная компиляция.
- `make lint-backend` -- expected: clean.
- `make test-unit-backend` -- expected: все тесты зелёные.
- `make test-unit-backend-coverage` -- expected: gate проходит (≥80% per-method).
- `make test-e2e-backend` -- expected: e2e в `campaign_creator/` зелёные.
- Self-check агента (между unit и e2e): `make migrate-up && make start-backend`; через curl создать кампанию + 2 creator (через approve flow / test-API) → POST add → GET list (assert items=2, status=planned, counts=0) → DELETE → GET (assert items=1); через `docker exec postgres-container psql ...` проверить `campaign_creators` и `audit_logs` (entity_type='campaign_creator', 2× add + 1× remove с правильными UUID-ами в payload). Расхождение со спекой = баг → агент сам фиксит без HALT.

**Manual checks:**
- Inspect generated `backend/internal/api/server.gen.go` после `make generate-api` — должны появиться `AddCampaignCreatorsRequestObject`, `RemoveCampaignCreator204Response`, `ListCampaignCreators200JSONResponse` и т.п.
- Inspect `frontend/web/src/api/generated/schema.ts` — должны появиться типы `CampaignCreator`, `CampaignCreatorStatus`, `AddCampaignCreatorsInput`.

## Suggested Review Order

**API contract (entry point)**

- 3 admin-only endpoint'а с явным «soft-deleted=404» и maxItems=200.
  [`openapi.yaml:1199`](../../backend/api/openapi.yaml#L1199)

- Полная row-схема + enum status — то, что фронт получит на руки.
  [`openapi.yaml:2767`](../../backend/api/openapi.yaml#L2767)

**Schema + data-integrity boundary**

- Стабильные имена constraint'ов: status CHECK, UNIQUE pair, обе FK — для `pgErr.ConstraintName` translation.
  [`20260507044135_campaign_creators.sql:27`](../../backend/migrations/20260507044135_campaign_creators.sql#L27)

- EAFP-перевод pgErr 23505/23503 (3 ветки) → domain-errors; всё остальное — raw для wrapper'а.
  [`repository/campaign_creator.go:90`](../../backend/internal/repository/campaign_creator.go#L90)

- Domain-проекция + 4 типизированные ошибки (3× `*ValidationError` + 1 sentinel).
  [`domain/campaign_creator.go:11`](../../backend/internal/domain/campaign_creator.go#L11)

**Бизнес-логика и транзакции**

- Add: pre-fetch ДО WithTx → sort (anti-deadlock) → loop INSERT+audit per creator → success-log ПОСЛЕ tx.
  [`service/campaign_creator.go:48`](../../backend/internal/service/campaign_creator.go#L48)

- Remove: pre-fetch + LBYL agreed-guard в tx + audit OldValue=snapshot, NewValue=nil.
  [`service/campaign_creator.go:95`](../../backend/internal/service/campaign_creator.go#L95)

- Soft-delete gate (через pool, ДО tx — спорно, см. deferred-work).
  [`service/campaign_creator.go:159`](../../backend/internal/service/campaign_creator.go#L159)

**HTTP-граница**

- 3 strict-server метода: authz → валидация в handler → service → strict-422 в `respondError` через типы.
  [`handler/campaign_creator.go:28`](../../backend/internal/handler/campaign_creator.go#L28)

- Маппинг `ErrCampaignCreatorNotFound` → 404, остальное — через ValidationError/BusinessError автомат.
  [`handler/response.go:76`](../../backend/internal/handler/response.go#L76)

- 3 admin-only authz-метода (mirror campaign.go).
  [`authz/campaign_creator.go:15`](../../backend/internal/authz/campaign_creator.go#L15)

**Wiring**

- Server interface для CampaignCreatorService + 3 authz-метода + поле в struct.
  [`handler/server.go:106`](../../backend/internal/handler/server.go#L106)

- `NewCampaignCreatorRepo` в factory + проводка в `main.go`.
  [`repository/factory.go:90`](../../backend/internal/repository/factory.go#L90)

- Инстанцирование сервиса и проводка в `NewServer`.
  [`cmd/api/main.go:103`](../../backend/cmd/api/main.go#L103)

**Тесты**

- Service unit: 3 метода × все ветки матрицы + audit shape с проверкой actor_id (AC).
  [`service/campaign_creator_test.go:42`](../../backend/internal/service/campaign_creator_test.go#L42)

- Repo unit: SQL-assert через capture + все pgErr-translation-ветки (23505/23503/23502-неперехвачен).
  [`repository/campaign_creator_test.go:38`](../../backend/internal/repository/campaign_creator_test.go#L38)

- Handler unit: authz/валидация/maxItems/dedup/happy + 500 на corrupted UUID.
  [`handler/campaign_creator_test.go:30`](../../backend/internal/handler/campaign_creator_test.go#L30)

- Authz unit: 3 метода × {admin OK, brand_manager 403, no-role 403}.
  [`authz/campaign_creator_test.go:14`](../../backend/internal/authz/campaign_creator_test.go#L14)

- E2e: 3 Test* через apiclient, audit-row + actor_id проверки, mixed-batch rollback.
  [`e2e/campaign_creator_test.go:84`](../../backend/e2e/campaign_creator/campaign_creator_test.go#L84)

- E2e cleanup-helper: A2 для активных пар (LIFO до родительских кампании/креатора).
  [`testutil/campaign_creator.go:21`](../../backend/e2e/testutil/campaign_creator.go#L21)

**Периферия**

- Generated server + e2e client + frontend schemas — только через `make generate-api`.
  [`api/server.gen.go`](../../backend/internal/api/server.gen.go)

- 7 хендлер-тестов обновлены под `NewServer` 11-арг (вставлен `nil` за `campaigns`).
  [`handler/campaign_test.go:30`](../../backend/internal/handler/campaign_test.go#L30)

- 3 audit-action константы + 1 entity-type для audit-rows.
  [`service/audit_constants.go:20`](../../backend/internal/service/audit_constants.go#L20)
