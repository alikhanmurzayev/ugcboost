---
title: 'Backend chunk #6: GET /campaigns (admin-only list)'
type: 'feature'
created: '2026-05-06'
status: 'done'
baseline_commit: 'f52e514'
context:
  - 'docs/standards/'
  - '_bmad-output/planning-artifacts/campaign-roadmap.md'
---

<frozen-after-approval reason="human-owned intent — do not modify unless human renegotiates">

## Intent

**Problem:** Чанк #6 кампаний-роудмапа: после POST/GET/PATCH админ не может получить список кампаний с пагинацией, фильтром по soft-deleted и поиском. Без list-ручки фронт-страница списка (#9) не имеет source-of-truth.

**Approach:** Admin-only `GET /campaigns` с query-параметрами (`page`, `perPage`, `sort`, `order`, `search`, `isDeleted`). Item-shape — переиспользуемая `Campaign` schema из #4 без изменений. Read-only: без транзакций, без audit, без миграции. Архитектура 1:1 с `POST /creators/applications/list` (`handler/creator_application.go:414`, `service/creator_application.go:612`, `repository/creator_application.go:309`) минус children-hydrate (нет children) и минус POST → GET (нет PII в search/response → симметрия с `GET /campaigns/{id}`).

## Boundaries & Constraints

**Always:**
- AuthZ admin-only через новый `AuthzService.CanListCampaigns(ctx)` — первой, до любого DB-чтения.
- Repository `List(ctx, CampaignListParams) ([]*CampaignRow, int64, error)`: page-q + count-q с **идентичной** WHERE-цепочкой. Helpers `applyCampaignListFilters`/`applyCampaignListOrder` по образцу creator_application. `escapeLikeWildcards` переиспользуется (`repository/creator_application.go:469`).
- Search: trim + lowercase, пустое → фильтр игнорируется; ILIKE substring по `CampaignColumnName` с `ESCAPE '\\'`; `maxLength: 128`.
- IsDeleted nullable bool: отсутствует/null = все, `true` = только soft-deleted, `false` = только живые.
- Sort enum `CampaignListSortField`: `created_at | updated_at | name`. Tie-breaker `id ASC` хвостом ко всем веткам.
- Service `List(ctx, in) (*domain.CampaignListPage, error)`: pool напрямую (без `WithTx`/audit/`logger.Info` на success); empty-page short-circuit.
- Handler — паттерн `ListCreatorApplications`: AuthZ, `Sort.Valid`/`Order.Valid`, page/perPage bounds, search trim+nil-если-пустое; маппинг `[]*domain.Campaign → []api.Campaign` через существующий `domainCampaignToAPI`.
- Code-gen: `make generate-api` после правки `openapi.yaml`; `make generate-mocks` после правки interfaces.
- **E2E-изоляция через search-marker.** Каждый list-сценарий в e2e создаёт уникальный `marker := newMarker()` (lowercase token, образец `creators/list_test.go:73`), встраивает его в `Name` сидингованных кампаний (например, `marker+"-"+suffix`), и ограничивает list-запрос параметром `search=marker`. Все ассерты `total` и состава `items` идут только по marker-scoped набору; голые числа без marker'а запрещены — иначе flake при параллельном прогоне (другие тесты могут одновременно сидить кампании).
- **Production-код — минимум комментариев.** В `handler/service/repository/authz/domain/middleware` пакетах не воспроизводить пухлый godoc-стиль соседних эталонов (`handler/creator_application.go`, `service/creator_application.go` и т.п.). Default = без комментария: понятное имя метода и типа сами по себе документация. Однострочный godoc на экспортированном идентификаторе оправдан только когда WHY реально неочевиден (скрытое ограничение, нелокальный инвариант, workaround под конкретный баг — `naming.md` § Комментарии). Многострочные блоки «что делает функция» — запрещены. **Тесты исключены** из этого правила: в e2e обязателен нарративный header (`backend-testing-e2e.md` § Комментарий), в unit допустимы точечные пояснения для неочевидной логики.

**Ask First:**
- Изменение `Campaign` schema (новые поля).
- Дополнительные фильтры (`dateFrom/dateTo` и т.п.).
- Cursor-пагинация вместо offset.

**Never:**
- POST вместо GET (нет PII).
- Reuse `PaginationInput`-allOf в GET-операции (это body-only schema).
- Hardcoded дефолты sort/order на сервере.
- Audit / `WithTx` / `logger.Info` на success-пути read-операции.
- Hydrate-словарей или children-загрузка.
- Новая миграция (таблица `campaigns` уже на месте с #3).

## I/O & Edge-Case Matrix

| Сценарий | Вход | Ответ | Обработка |
|---|---|---|---|
| Happy path | admin, валидные params, 3+ кампании | 200 + items по `sort/order`+id-tie, total корректный | — |
| Пустой список | фильтр без совпадений | 200 + `items:[], total:0` | — |
| Page beyond last | `page > total/perPage` | 200 + `items:[], total>0` | — |
| Не аутентифицирован | без токена | 401 | middleware |
| Non-admin | manager-токен | 403 (до DB) | `CanListCampaigns` |
| Bad page/perPage | `page=0` или `perPage=201` | 422 `CodeValidation` | handler-валидатор |
| Unknown sort/order | `sort=bogus` | 422 `CodeValidation` | `Sort.Valid` |
| Long search | `len > 128` | 422 (openapi `maxLength`) | wrapper |
| Whitespace search | `search="   "` | фильтр игнорируется | trim → nil |
| Wildcard в search | `search="100%"` | литеральный match | `escapeLikeWildcards` + `ESCAPE` |
| `isDeleted=true/false/missing` | admin | только soft-deleted / только живые / все | repo-WHERE |
| БД-ошибка | pool падает | 500 default | service: `fmt.Errorf` wrap |

</frozen-after-approval>

## Code Map

- `backend/api/openapi.yaml` -- (1) `GET /campaigns` (`operationId: listCampaigns`, query: `page` int≥1 required, `perPage` int 1..200 required, `sort` `CampaignListSortField` required, `order` `SortOrder` required, `search` string maxLength 128 optional, `isDeleted` bool optional); (2) schemas `CampaignListSortField` (enum `created_at|updated_at|name`), `CampaignsListData {items,total,page,perPage}`, `CampaignsListResult {data}`. `Campaign`/`SortOrder` переиспользуются.
- `backend/internal/domain/campaign.go` -- `CampaignListInput` / `CampaignListPage` / sort consts + `CampaignListSortFieldValues` / bounds (по образцу `creator_application.go:365`).
- `backend/internal/repository/campaign.go` -- расширить `CampaignRepo` методом `List`; `CampaignListParams`; helpers `applyCampaignListFilters`/`applyCampaignListOrder`. Page-q `dbutil.Many`, count-q `dbutil.Val[int64]`.
- `backend/internal/repository/campaign_test.go` -- `Test_campaignRepository_List`: success, page+count match, soft-deleted×3 ветки, wildcard ESCAPE, sort×order по каждому enum, db error в page и count.
- `backend/internal/authz/campaign.go` (+`_test.go`) -- `CanListCampaigns(ctx) error` admin-only + 3 unit-теста.
- `backend/internal/service/campaign.go` (+`_test.go`) -- `List(ctx, in)`: read-only, маппер input→params, empty short-circuit, rows→domain. Тесты: empty, repo error wrapped, success.
- `backend/internal/handler/server.go` -- расширить `CampaignService` (`List`) и `AuthzService` (`CanListCampaigns`).
- `backend/internal/handler/campaign.go` (+`_test.go`) -- `ListCampaigns` handler по паттерну `ListCreatorApplications`. Тесты: всё из I/O Matrix + точные args в mock-expectations.
- `backend/e2e/campaign/campaign_test.go` -- расширить header-нарратив; добавить отдельный `TestCampaignList` (изоляция seed). Все `t.Parallel()`. Cleanup через `RegisterCampaignCleanup`.

## Tasks & Acceptance

**Execution (порядок строго по слоям):**

- [x] `backend/api/openapi.yaml` -- добавить operation `GET /campaigns` + schemas `CampaignListSortField`, `CampaignsListData`, `CampaignsListResult`.
- [x] **`make generate-api`** -- регенерация generated кода.
- [x] `backend/internal/domain/campaign.go` -- `CampaignListInput`/`CampaignListPage`/sort consts/bounds.
- [x] `backend/internal/repository/campaign.go` (+`_test.go`) -- `List` + helpers; pgxmock-тесты со всеми ветками.
- [x] `backend/internal/authz/campaign.go` (+`_test.go`) -- `CanListCampaigns` + 3 unit-теста.
- [x] `backend/internal/service/campaign.go` (+`_test.go`) -- `List` (read-only) + 3 unit-сценария.
- [x] **`make generate-mocks`** -- mockery regen для CampaignService/AuthzService.
- [x] `backend/internal/handler/server.go` -- расширить interfaces.
- [x] `backend/internal/handler/campaign.go` (+`_test.go`) -- `ListCampaigns` + tests по I/O Matrix.
- [x] **Manual local sanity check (HALT)** -- `make migrate-up && make start-backend`. Получить admin/brand-manager-токены. Curl-сценарии: happy (seed 3, GET с sort/order, проверить total и порядок), search (substring + wildcard literal), isDeleted (true/false/missing), pagination (last partial + beyond-last empty), 422-grid (page/perPage/sort/order/long-search), 403 brand-manager, 401 без токена. **HALT и сообщить пользователю фактические HTTP-коды + JSON-тела до старта e2e.**
- [x] `backend/e2e/campaign/campaign_test.go` -- `TestCampaignList` со сценариями из I/O Matrix; в каждом сценарии создаётся `marker := newMarker()`, marker встраивается в `Name` сидингованных кампаний, list-запрос идёт с `search=marker` (паттерн `e2e/creators/list_test.go:73`); assert'ы `total`/`items` — только по marker-scoped набору, без голых чисел. `require.Equal` целиком после подмены dynamic полей.

**Acceptance Criteria:**
- Given `make build-backend && make lint-backend`, when запускаются, then оба зелёные.
- Given `make test-unit-backend && make test-unit-backend-coverage`, when запускаются, then оба зелёные; per-method gate ≥80% сохраняется на изменённых файлах.
- Given `make test-e2e-backend`, when запускается, then `TestCampaignList` зелёный, существующие `TestCampaignCRUD` + `TestCreateCampaign_RaceUniqueName` остаются зелёными.
- Given поднятый бэк с 3+ seeded кампаниями, when admin GET `/campaigns?page=1&perPage=10&sort=created_at&order=desc`, then 200 + items в правильном порядке + total соответствует count в БД с тем же фильтром.
- Given seeded soft-deleted кампания, when admin GET `/campaigns?...&isDeleted=true`, then 200 + только эта кампания; `isDeleted=false` → только живые; параметр опущен → все.

## Spec Change Log

<!-- Append-only. Populated by step-04 during review loops. Empty until the first bad_spec loopback. -->

## Design Notes

**GET vs POST.** В `creators/applications/list` POST выбран из-за PII в search; у `Campaign` PII нет (`name` — публичное название). Запрет URL-params из `security.md` § PII не применим. GET выбран для симметрии с `GET /campaigns/{id}` — фронт работает с одной операционной моделью на чтение.

**Item-shape — full Campaign.** Lean-варианта вроде `CampaignListItem` не делаем: объём мал (6 примитивных полей), а единый shape с детальной ручкой упрощает фронту маппинг и инвалидацию кэшей. Добавление поля в `Campaign` автоматически прокидывается в обе ручки.

**Параллельная работа над #5.** Параллельный агент держит ветку `alikhan/campaign-update-backend` (chunk #5, `PATCH /campaigns/{id}`). Обе спеки правят `openapi.yaml`, `handler/service/repository/campaign*.go`, `e2e/campaign/campaign_test.go`. Реализация #6 стартует **после** мержа #5 в main; `baseline_commit` фиксируется на этом моменте, ветка для #6 — отдельная (`alikhan/campaign-list-backend`) от обновлённого main.

## Verification

**Commands:**
- `make generate-api` -- expected: успех; `git diff --stat` показывает обновлённые `*.gen.go` + frontend schemas + apiclient/testclient
- `make generate-mocks` -- expected: успех
- `make build-backend` -- expected: компиляция чистая
- `make lint-backend` -- expected: zero issues
- `make test-unit-backend && make test-unit-backend-coverage` -- expected: зелёные; per-method gate ≥80%
- `make test-e2e-backend` -- expected: `TestCampaignList` + `TestCampaignCRUD` + `TestCreateCampaign_RaceUniqueName` зелёные

## Suggested Review Order

**Контракт API**

- Operation `GET /campaigns` с query params (page/perPage/sort/order/search/isDeleted)
  [`openapi.yaml:1009`](../../backend/api/openapi.yaml#L1009)

- Schemas `CampaignListSortField`, `CampaignsListData`, `CampaignsListResult`
  [`openapi.yaml:2576`](../../backend/api/openapi.yaml#L2576)

**Domain → Repository → Service**

- Доменные bounds, `CampaignListInput`, `CampaignListPage`, sort consts
  [`domain/campaign.go:91`](../../backend/internal/domain/campaign.go#L91)

- Repo `List` — count+page одинаковая WHERE, defensive bounds, OFFSET/LIMIT
  [`repository/campaign.go:137`](../../backend/internal/repository/campaign.go#L137)

- Repo helpers — `applyCampaignListFilters` (search ILIKE+ESCAPE, isDeleted) и `applyCampaignListOrder` (sort whitelist + tie-breaker id ASC)
  [`repository/campaign.go:170`](../../backend/internal/repository/campaign.go#L170)

- Service `List` — pool напрямую, empty short-circuit, маппер `campaignListInputToRepo` с trim+lowercase
  [`service/campaign.go:131`](../../backend/internal/service/campaign.go#L131)

**Handler + AuthZ**

- Handler `ListCampaigns` — AuthZ first, валидаторы (sort/order/page/perPage/search), маппинг items → API
  [`handler/campaign.go:104`](../../backend/internal/handler/campaign.go#L104)

- AuthZ admin-only gate
  [`authz/campaign.go:39`](../../backend/internal/authz/campaign.go#L39)

- Расширение `CampaignService` и `AuthzService` интерфейсов
  [`handler/server.go:58`](../../backend/internal/handler/server.go#L58)

**Tests**

- Repo unit-тесты: success, sort×order, soft-deleted×3, wildcard ESCAPE, db error в page и count
  [`repository/campaign_test.go:260`](../../backend/internal/repository/campaign_test.go#L260)

- Service unit-тесты: empty short-circuit, repo error wrap, captured-input маппинг (включая lowercase)
  [`service/campaign_test.go:476`](../../backend/internal/service/campaign_test.go#L476)

- Handler unit-тесты: 401/403/400/422-grid, beyond-last total>0, isDeleted=missing captured, corrupted ID → 500
  [`handler/campaign_test.go:410`](../../backend/internal/handler/campaign_test.go#L410)

- AuthZ unit-тесты: admin/manager/missing-role
  [`authz/campaign_test.go:80`](../../backend/internal/authz/campaign_test.go#L80)

- E2E `TestCampaignList` — marker-isolation через `newCampaignMarker()`+search=marker; покрывает всю I/O Matrix
  [`e2e/campaign_test.go:602`](../../backend/e2e/campaign/campaign_test.go#L602)
