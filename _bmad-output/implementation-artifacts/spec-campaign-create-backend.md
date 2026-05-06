---
title: 'Backend chunk #3: campaigns table + POST /campaigns (admin-only)'
type: 'feature'
created: '2026-05-06'
status: 'in-review'
baseline_commit: 'd0dd15c653591343167b1c53a7ae665188de8fbf'
context:
  - 'docs/standards/'
  - '_bmad-output/planning-artifacts/campaign-roadmap.md'
---

<frozen-after-approval reason="human-owned intent — do not modify unless human renegotiates">

## Intent

**Problem:** Чанк #3 кампаний-роудмапа: в БД нет таблицы `campaigns`, в API нет ручки создания. Без них нельзя двигаться к выбору креаторов и рассылке (чанки #10+).

**Approach:** Минимальная модель MVP — кампания = `{name, tma_url}` + технические поля (`id`, `is_deleted`, `*At`). TMA-страница ТЗ — сверстанный лендинг по секретному URL внутри TMA-приложения (вне этого чанка). Admin-only `POST /campaigns` создаёт строку и audit-row в одной транзакции; уникальность `name` среди живых кампаний обеспечивает partial unique index, race на нём → `409 CodeCampaignNameTaken`.

## Boundaries & Constraints

**Always:**
- Полная загрузка `docs/standards/` и применение как hard rules (architecture, codegen, repository, transactions, errors, libraries, testing-unit/e2e, security, naming).
- Слои handler → service → repository через `RepoFactory`; транзакции через `dbutil.WithTx`; audit-row пишется внутри той же tx.
- AuthZ — admin-only через новый `AuthzService.CanCreateCampaign(ctx)`; проверка раньше любой бизнес-логики.
- EAFP в repo: INSERT сразу, `errors.As(*pgconn.PgError)` + `Code == "23505"` → `domain.ErrCampaignNameTaken`. Без preflight SELECT.
- Код-ген: `make generate-api` после правки `openapi.yaml` / `openapi-test.yaml` — единственный путь обновления `*.gen.go`.
- Audit `new_value` = JSON всей `*domain.Campaign` (snake_case по json-тегам); при добавлении поля audit обновляется автоматически.

**Ask First:**
- Любое изменение полей `campaigns` или их валидаций.
- Изменение публичной модели API (`CampaignInput`, `Campaign`, `CampaignResult`).
- Отказ от partial unique index или его трансформация.
- Правка файла роудмапа в этом же PR'е (по плану — отдельным PR'ом).

**Never:**
- Поля `public_brief / private_brief / event_date` (выпали с переосмыслением).
- CHECK-constraints для format-валидаций в миграции; business defaults в БД.
- Хардкод секрета TMA в БД (живёт в TMA-приложении).
- GET / LIST / PATCH / DELETE кампаний (чанки #4–#7).
- Ручной JSON-decode в handler / ручная регистрация роута / ручные моки.
- `pgx.ErrNoRows` (только `sql.ErrNoRows`).
- `math/rand`; `crypto/rand` для тестовых имён.
- PII в `logger.Info/Debug/Warn` (имя кампании — не PII, ок).

## I/O & Edge-Case Matrix

| Сценарий | Вход / Состояние | Ответ | Обработка ошибок |
|---|---|---|---|
| Happy path | admin, `name="Promo X"`, `tmaUrl="https://tma.../tz/abc"` | 201 + `CampaignCreatedResult{Data: CampaignCreatedData{id}}` + audit-row `campaign_create` | — |
| Не аутентифицирован | без токена | 401 `UNAUTHORIZED` | middleware |
| brand-manager | manager-токен | 403 `FORBIDDEN` (до любого DB-чтения) | `CanCreateCampaign` |
| Пустое имя | `name=""` или whitespace | 422 `CodeCampaignNameRequired` + actionable message | `domain.ValidateCampaignName` |
| Длинное имя | `len(name) > 255` | 422 `CodeCampaignNameTooLong` | то же |
| Пустой URL | `tmaUrl=""` или whitespace | 422 `CodeCampaignTmaURLRequired` | `domain.ValidateCampaignTmaURL` |
| Длинный URL | `len(tmaUrl) > 2048` | 422 `CodeCampaignTmaURLTooLong` | то же |
| Имя занято (race) | concurrent INSERT с одним `name` среди `is_deleted=false` | один → 201, второй → 409 `CodeCampaignNameTaken` | repo: pgErr 23505 → `ErrCampaignNameTaken` |
| Нижняя БД-ошибка | pool падает | 500 `INTERNAL_ERROR` (default-ветка `respondError`) | проброс с обёрткой |

</frozen-after-approval>

## Code Map

- `backend/migrations/<timestamp>_campaigns.sql` -- новая Up/Down миграция: таблица + partial unique index
- `backend/api/openapi.yaml` -- POST /campaigns, schemas `CampaignInput / Campaign / CampaignResult`, тег `campaigns`
- `backend/api/openapi-test.yaml` -- расширить enum `CleanupEntityRequest.type` (+`campaign`)
- `backend/internal/domain/campaign.go` -- `Campaign` struct + `ErrCampaignNameTaken` + `ValidateCampaignName/TmaURL` + новые `CodeCampaign*`
- `backend/internal/domain/errors.go` -- добавить 5 новых `CodeCampaign*` констант
- `backend/internal/repository/campaign.go` -- repo (Create + DeleteForTests), `TableCampaigns`, `CampaignColumn*`, `CampaignsNameActiveUnique`, stom-mappers
- `backend/internal/repository/factory.go` -- `NewCampaignRepo`
- `backend/internal/repository/campaign_test.go` -- pgxmock unit
- `backend/internal/authz/campaign.go` -- `CanCreateCampaign`
- `backend/internal/authz/campaign_test.go` -- admin/non-admin/no-role
- `backend/internal/service/campaign.go` -- `CampaignService` + `CampaignRepoFactory` + конструктор
- `backend/internal/service/campaign_test.go` -- mockery unit
- `backend/internal/service/audit_constants.go` -- `AuditActionCampaignCreate`, `AuditEntityTypeCampaign`
- `backend/internal/handler/campaign.go` -- `CreateCampaign(ctx, req) (resp, error)` strict-handler + mapper `domainCampaignToAPI`
- `backend/internal/handler/campaign_test.go` -- handler unit
- `backend/internal/handler/server.go` -- добавить `CampaignService` interface, поле в `Server`, аргумент `NewServer`, `AuthzService.CanCreateCampaign`
- `backend/internal/handler/testapi.go` -- `case testapi.Campaign` в `CleanupEntity`
- `backend/cmd/api/main.go` -- инстанцировать `CampaignService`, прокинуть в `NewServer`
- `backend/e2e/testutil/campaign.go` -- `SetupCampaign(t, adminToken, name, tmaURL)` + `DeleteCampaign(t, id)` (cleanup-stack-friendly)
- `backend/e2e/testutil/audit.go` -- если нужно, расширить `AssertAuditEntry` ассистентом для campaign (если уже generic — не трогать)
- `backend/e2e/campaign/campaign_test.go` -- `TestCreateCampaign` + `TestCreateCampaign_RaceUniqueName`

## Tasks & Acceptance

**Execution (порядок строго по слоям; каждая зависимость зафиксирована):**

- [x] `backend/migrations/<ts>_campaigns.sql` -- Up: `CREATE TABLE campaigns (id UUID PK DEFAULT gen_random_uuid(), name TEXT NOT NULL, tma_url TEXT NOT NULL, is_deleted BOOLEAN NOT NULL DEFAULT false, created_at TIMESTAMPTZ NOT NULL DEFAULT now(), updated_at TIMESTAMPTZ NOT NULL DEFAULT now())` + `CREATE UNIQUE INDEX campaigns_name_active_unique ON campaigns(name) WHERE is_deleted = false`. Down: `DROP INDEX campaigns_name_active_unique; DROP TABLE campaigns`. -- Создать таблицу через `make migrate-create NAME=campaigns`.
- [x] `backend/api/openapi.yaml` -- Добавить `POST /campaigns` (201/401/403/409/422/default), схемы `CampaignInput`, `Campaign`, `CampaignResult`. -- Контракт-first перед codegen.
- [x] `backend/api/openapi-test.yaml` -- Расширить enum `CleanupEntityRequest.type` значением `campaign`. -- Test cleanup для e2e.
- [x] **Запустить `make generate-api`** -- регенерация всех `*.gen.go` (backend + apiclient + testclient + frontend schemas) -- единственный путь обновления generated.
- [x] `backend/internal/domain/errors.go` -- Добавить `CodeCampaignNameRequired`, `CodeCampaignNameTooLong`, `CodeCampaignTmaURLRequired`, `CodeCampaignTmaURLTooLong`, `CodeCampaignNameTaken`. -- Granular коды для actionable UX.
- [x] `backend/internal/domain/campaign.go` -- `Campaign` struct (snake_case json), `var ErrCampaignNameTaken = NewBusinessError(CodeCampaignNameTaken, "Кампания с таким названием уже есть. Выберите другое название или удалите старую кампанию.")`, `ValidateCampaignName(string) error` (trim + non-empty + ≤255), `ValidateCampaignTmaURL(string) error` (trim + non-empty + ≤2048). -- Domain-валидаторы.
- [x] `backend/internal/repository/campaign.go` -- `TableCampaigns`, `CampaignColumn*`, `CampaignsNameActiveUnique = "campaigns_name_active_unique"`, `CampaignRow` с тегами `db` + `insert` (только Name, TmaURL), `campaignSelectColumns / campaignInsertColumns / campaignInsertMapper` через stom, `CampaignRepo` interface, `Create(ctx, name, tmaURL)` с EAFP 23505→`domain.ErrCampaignNameTaken`, `DeleteForTests(ctx, id)`. -- Слой данных.
- [x] `backend/internal/repository/factory.go` -- `NewCampaignRepo(db dbutil.DB) CampaignRepo`. -- Конструктор-зависимость через factory.
- [x] `backend/internal/repository/campaign_test.go` -- `Test_campaignRepository_Create`: `success` (точный SQL литералом + args), `name taken` (pgxmock возвращает `*pgconn.PgError{Code:"23505", ConstraintName: CampaignsNameActiveUnique}` → `ErrorIs ErrCampaignNameTaken`), `db error` (проброс с обёрткой). -- Coverage gate ≥80%.
- [x] `backend/internal/service/audit_constants.go` -- `AuditActionCampaignCreate = "campaign_create"`, `AuditEntityTypeCampaign = "campaign"`. -- Имена audit-событий.
- [x] `backend/internal/service/campaign.go` -- `CampaignRepoFactory` interface (`NewCampaignRepo`, `NewAuditRepo`), `CampaignService` struct (`pool`, `repoFactory`, `logger`), `NewCampaignService`, `CreateCampaign(ctx, name, tmaURL)`: `dbutil.WithTx` → `cRepo.Create` + `writeAudit(ctx, aRepo, AuditActionCampaignCreate, AuditEntityTypeCampaign, c.ID, nil, c)` ; success-`logger.Info("campaign created")` **после** `WithTx`. -- Бизнес-логика + audit в одной tx.
- [x] `backend/internal/service/campaign_test.go` -- `TestCampaignService_CreateCampaign`: `repo error` (audit не вызывался), `audit error → rollback`, `success` (`mock.Run(...)` ловит args `aRepo.Create`; `Action / EntityType / EntityID / OldValue=nil`; `NewValue` через `require.JSONEq` против `json.Marshal(*expected)`); затем `require.Equal` с подменой dynamic полей. -- Service unit.
- [x] `backend/internal/authz/campaign.go` -- `func (a *AuthzService) CanCreateCampaign(ctx context.Context) error { if middleware.RoleFromContext(ctx) != api.Admin { return domain.ErrForbidden } ; return nil }` + GoDoc-однострочник о том что admin-only. -- Authz.
- [x] `backend/internal/authz/campaign_test.go` -- 3 кейса: admin → nil, brand_manager → ErrForbidden, no-role → ErrForbidden. -- Authz unit.
- [x] `backend/internal/handler/campaign.go` -- `func (s *Server) CreateCampaign(ctx, request) (response, error)`: `s.authzService.CanCreateCampaign(ctx)` → проброс; `ValidateCampaignName(req.Body.Name)` / `ValidateCampaignTmaURL(req.Body.TmaUrl)` → проброс; `s.campaignService.CreateCampaign(ctx, req.Body.Name, req.Body.TmaUrl)`; success → `api.CreateCampaign201JSONResponse{Data: domainCampaignToAPI(c)}`. Errors уходят в `respondError` через strict-server. -- Handler.
- [x] `backend/internal/handler/campaign_test.go` -- `TestServer_CreateCampaign`: 6 валидационных кейсов (whitespace name, name>255, empty tmaUrl, tmaUrl>2048, forbidden non-admin, name taken→409, generic→500), happy 201 (динамические поля подменяем + `require.Equal` на полный response). -- Handler unit, точные args в expectations.
- [x] `backend/internal/handler/server.go` -- `CampaignService` interface (`CreateCampaign(ctx, name, tmaURL) (*domain.Campaign, error)`), `AuthzService.CanCreateCampaign(ctx) error`, `Server.campaignService`, аргумент `campaigns CampaignService` в `NewServer`. -- Server-wiring.
- [x] `backend/internal/handler/testapi.go` -- `case testapi.Campaign: deleteErr = h.repos.NewCampaignRepo(h.pool).DeleteForTests(ctx, req.Id)`. -- Cleanup-dispatch.
- [x] `backend/cmd/api/main.go` -- инстанцировать `service.NewCampaignService(pool, repoFactory, log)` и прокинуть в `handler.NewServer(...)`. -- DI.
- [x] **Manual local sanity check (HALT)** -- `make migrate-up && make start-backend`. Получить admin-токен через test-API (`POST /test/seed-user` → `POST /test/reset-token` → логин по выданному сценарию **или** `POST /auth/login` если admin сидится automatically). Выполнить `curl`-сценарии и закрепить `docker compose exec postgres psql -U ugcboost -d ugcboost` запросы как фактический результат: (a) **happy** `POST /campaigns` admin-токеном → 201 + полный `Campaign{...}`; `SELECT * FROM campaigns` → одна строка, `is_deleted=false`; `SELECT action, entity_type, entity_id, new_value FROM audit_logs ORDER BY created_at DESC LIMIT 1` → `('campaign_create','campaign',<id>, '{"id":"<uuid>","name":"...","tma_url":"...","is_deleted":false,"created_at":"...","updated_at":"..."}')`. (b) **race** второй POST тем же `name` → 409 `CAMPAIGN_NAME_TAKEN` + actionable RU-message. (c) **422-grid** `name=""` / `name=<256 chars>` / `tmaUrl=""` / `tmaUrl=<2049 chars>` → каждый возвращает свой granular код. (d) **403** brand-manager-токеном → 403 `FORBIDDEN`. (e) **401** без токена → 401. **HALT и сообщить пользователю результаты (вкл. фактические HTTP-коды + JSON-тела) до старта e2e.** -- Подтверждение, что код на самом деле работает прежде чем фиксировать контракт e2e-тестами; это явное требование `campaign-roadmap.md` § Тестирование.
- [x] `backend/e2e/testutil/campaign.go` -- `SetupCampaign(t *testing.T, ctx context.Context, client *apiclient.ClientWithResponses, adminToken, name, tmaURL string) (campaignID string)` + `DeleteCampaign(t, ctx, testClient, id)` через `/test/cleanup-entity`; добавляется в cleanup-stack. -- Composable e2e helpers.
- [x] `backend/e2e/campaign/campaign_test.go` -- header-комментарий на русском нарративом по `backend-testing-e2e.md`; `TestCreateCampaign` (8 t.Run: unauthenticated 401, brand-manager 403, empty name → 422 raw, name>255 → 422, empty tmaUrl → 422 raw, tmaUrl>2048 → 422, success 201 + `testutil.AssertAuditEntry(campaign_create, entityID, JSON-payload)`); `TestCreateCampaign_RaceUniqueName` (errgroup на 2 concurrent POST с одним name → один 201, второй 409 `CodeCampaignNameTaken`). Все `t.Parallel()`. -- E2E (только после успешной ручной проверки выше).

**Acceptance Criteria:**

- Given `make build-backend && make lint-backend`, when запускаются после всех правок, then оба зелёные.
- Given `make test-unit-backend && make test-unit-backend-coverage`, when запускаются, then оба зелёные; per-method coverage gate ≥80% на новых файлах в `handler/service/repository/authz`.
- Given `make test-e2e-backend`, when запускается, then `TestCreateCampaign` + `TestCreateCampaign_RaceUniqueName` зелёные; cleanup-stack удаляет созданные кампании через `/test/cleanup-entity`.
- Given поднятый бэк локально, when admin POST /campaigns с валидным телом, then в БД ровно одна строка `campaigns` (`is_deleted=false`) и одна строка `audit_logs` (`action='campaign_create'`, `new_value` = JSON c полями `id/name/tma_url/is_deleted/created_at/updated_at`).
- Given две идентичные кампании создаются параллельно, when второй INSERT попадает в гонку, then 409 + `code: "CAMPAIGN_NAME_TAKEN"` + actionable RU-message; в БД одна строка.
- Given создан и soft-deleted (этот чанк soft-delete не реализует — критерий относится к будущему чанку #7, **не проверяем здесь**).

## Spec Change Log

- 2026-05-06: реализация чанка завершена — 22 таска [x], все 5 acceptance criteria зелёные локально (build / lint / unit + per-method coverage gate / e2e со всеми 10 пакетами / ручная sanity-проверка). Имплементационных дельт от спеки нет; единственная попутная правка — `testapi_test.go` "unknown type returns 422" переключён с литерала `"campaign"` на `"totally_unknown"` (campaign теперь часть enum, нужно отдельное несуществующее значение для покрытия default-ветки).
- 2026-05-06 (renegotiation, post-implementation): по решению Alikhan'а контракт ответа `POST /campaigns` сужен до id-only — `CampaignResult{Data: Campaign{...}}` заменён на `CampaignCreatedResult{Data: CampaignCreatedData{id}}`. Полная read-проекция кампании уйдёт в чанк #4 (`GET /campaigns/{id}`), там же дополнится e2e-ассерт shape'а. Удалены: схема `Campaign` из openapi, `domainCampaignToAPI` мэппер. `*domain.Campaign` остаётся как payload audit-row'а (требование Boundaries «Always» сохранено). Изменены: openapi.yaml (новые `CampaignCreatedData/Result`), `handler/campaign.go` (response — id-only), `handler/campaign_test.go` + `e2e/campaign/campaign_test.go` (success-ассерт сужен до `require.NotEqual(uuid.Nil, id)` + audit-row).

## Design Notes

`*domain.Campaign` сериализуется в `audit_logs.new_value` целиком. Конвенция json-тегов в domain — snake_case (см. `domain.Brand.LogoURL → "logo_url"`); поле `TmaURL` → `tma_url`. При расширении модели (например, добавим `event_date` в будущем) audit обновляется без правки `service/campaign.go`.

`ErrCampaignNameTaken` — sentinel `*BusinessError`. `respondError` имеет общий `errors.As(err, &be)` ветку, которая отдаёт **409 + ve.Code + ve.Message** автоматически — отдельный `case errors.Is(...)` в `response.go` **не нужен**. Достаточно вернуть sentinel из repo и пробросить через service.

Partial unique index `... WHERE is_deleted = false` — почему партиальный: после soft-delete (чанк #7) старое имя должно быть переиспользуемо. Race на этом индексе обязывает race-тест в e2e (`backend-testing-e2e.md` § Время и race-сценарии).

Audit-payload-пример (для сверки в тестах):
```
{"id":"<uuid>","name":"Promo X","tma_url":"https://tma.ugcboost.kz/tz/abc",
 "is_deleted":false,"created_at":"2026-05-06T12:00:00Z","updated_at":"2026-05-06T12:00:00Z"}
```

## Verification

**Commands:**
- `make generate-api` -- expected: успех; `git diff --stat` показывает обновлённые `*.gen.go` + frontend schemas
- `make build-backend` -- expected: компиляция чистая
- `make lint-backend` -- expected: zero issues (включая depguard на `math/rand`)
- `make test-unit-backend` -- expected: зелёные с `-race`
- `make test-unit-backend-coverage` -- expected: зелёный gate
- `make test-e2e-backend` -- expected: `TestCreateCampaign` + `TestCreateCampaign_RaceUniqueName` зелёные
