---
title: 'Видимость участия креатора в кампаниях'
type: 'feature'
created: '2026-05-11'
status: 'done'
baseline_commit: 5feb2a2645e02326191e5bc8facbb2b0b4d8ee12
context:
  - docs/standards/backend-architecture.md
  - docs/standards/backend-repository.md
  - docs/standards/frontend-components.md
---

<frozen-after-approval reason="human-owned intent — do not modify unless human renegotiates">

## Intent

**Problem:** На странице `/creators` нельзя понять, в скольких кампаниях состоит креатор, и в каких именно. Операторы вручную джойнят `creators` и `campaign_creators` через SQL, чтобы ответить на вопрос креатора в поддержке.

**Approach:** Без модификации существующего `CreatorRepo.List` обогащаем результат на уровне `CreatorService` тремя батч-запросами (`CreatorRepo.List` → новый `CampaignCreatorRepo.ListByCreatorIDs` → существующий `CampaignRepo.ListByIDs`) и публикуем два новых поля API: `activeCampaignsCount` в `CreatorListItem` и массив `campaigns: CreatorCampaignBrief[]` в `CreatorAggregate`. UI: новая колонка во второй позиции таблицы `/creators` и блок «Участие в кампаниях» в drawer'е.

## Boundaries & Constraints

**Always:**
- Активная кампания = `campaigns.is_deleted = false`. Фильтр — в `CreatorService`, не в SQL.
- Новый repo-метод `CampaignCreatorRepo.ListByCreatorIDs` без JOIN-ов; на пустом списке IDs возвращает `nil, nil` без обращения к БД.
- Контракт меняется через `backend/api/openapi.yaml` + `make generate-api`. Ручных типов и интерфейсов в Go и TS — нет.
- Backend отдаёт `CreatorAggregate.campaigns[]` уже отсортированным по `campaign_creators.created_at DESC`. Фронт группирует по статусу, сохраняя исходный порядок внутри группы.
- В drawer-блоке три группы в порядке «Активные» (`signed`, `signing`, `agreed`) → «В процессе» (`invited`, `planned`) → «Отказы» (`declined`, `signing_declined`). Пустые группы не рендерятся.
- Status-лейблы — переиспользуются из `locales/ru/campaigns.json`. Дублирование запрещено.
- Колонка `activeCampaignsCount` не сортируемая (`sortable: false`).

**Ask First:**
- Любое изменение состава фильтра «активная кампания» (например, исключение конкретного статуса).
- Любая попытка ввести JOIN в SQL вместо service-обогащения.

**Never:**
- Модифицировать `CreatorRepo.List` или его SQL.
- Добавлять `CreatorListSortField` для счётчика — сортировка списка по `activeCampaignsCount` не поддерживается.
- Создавать новые e2e-файлы — только расширение `backend/e2e/creators/list_test.go`, `get_test.go` и `frontend/e2e/web/admin-creators-list.spec.ts`.
- Вводить миграции, audit-логи, новые notifications — фича read-only.
- Включать удалённые кампании в счётчик или drawer-список.

## I/O & Edge-Case Matrix

| Scenario | Input / State | Expected Output / Behavior | Error Handling |
|---|---|---|---|
| Creator в 3 активных кампаниях | `GET /creators` | `activeCampaignsCount = 3` | N/A |
| Creator в 1 активной + 1 удалённой | `GET /creators` | `activeCampaignsCount = 1`; удалённая исключена | N/A |
| Creator без participations | `GET /creators` | `activeCampaignsCount = 0` | N/A |
| Creator без participations | `GET /creators/{id}` | `campaigns = []` | N/A |
| Creator в 7 разных статусах | `GET /creators/{id}` | `campaigns[]` содержит все 7 строк; UI группирует в 3 секции в заданном порядке | N/A |
| Все participations указывают на удалённые кампании | `GET /creators/{id}` | `campaigns = []` | N/A |
| `ListByCreatorIDs(ctx, [])` | пустой input | возвращает `nil, nil`, не открывая запрос | N/A |
| Несколько creators на странице, у одного 0 кампаний | `GET /creators` | счётчик `0` для этого creator'а, остальные — корректны | N/A |

</frozen-after-approval>

## Pre-implementation

Перед началом реализации обязательно загрузить **все** файлы `docs/standards/` (`backend-architecture.md`, `backend-codegen.md`, `backend-constants.md`, `backend-design.md`, `backend-errors.md`, `backend-libraries.md`, `backend-repository.md`, `backend-testing-e2e.md`, `backend-testing-unit.md`, `backend-transactions.md`, `frontend-api.md`, `frontend-components.md`, `frontend-quality.md`, `frontend-state.md`, `frontend-testing-e2e.md`, `frontend-testing-unit.md`, `frontend-types.md`, `naming.md`, `review-checklist.md`, `security.md`). Frontmatter `context` называет три ключевых; остальные — равноценные hard rules.

Рабочая ветка — `alikhan/creator-campaigns-list`. На `main` коммиты запрещены. Claude НЕ коммитит и НЕ мержит самостоятельно — все изменения остаются в working tree до ручного ревью Alikhan'a.

Связанный intent-документ: `_bmad-output/implementation-artifacts/intent-creator-campaign-participation.md` — историческая мотивация решений (читать опционально).

## Code Map

- `backend/api/openapi.yaml` — добавить компонент `CreatorCampaignBrief`, required-поля `activeCampaignsCount` в `CreatorListItem` и `campaigns` в `CreatorAggregate`.
- `backend/internal/domain/creator.go` — Go-эквивалент: тип `CreatorCampaignBrief`, поле `ActiveCampaignsCount int` в `CreatorListItem`, `Campaigns []CreatorCampaignBrief` в `CreatorAggregate`.
- `backend/internal/repository/campaign_creator.go` — новый метод `ListByCreatorIDs(ctx, creatorIDs []string) ([]*CampaignCreatorRow, error)` + добавление в интерфейс `CampaignCreatorRepo`.
- `backend/internal/repository/campaign_creator_test.go` — unit-тест на новый метод (точные SQL-литералы, маппинг row→struct, empty-input shortcut, error propagation).
- `backend/internal/service/creator.go` — в `List` третий batch (`ListByCreatorIDs` + `CampaignRepo.ListByIDs`) и сборка `map[creatorID]int`; в `GetByID` — батч + сборка `[]CreatorCampaignBrief`, отсортированного по `campaign_creators.created_at DESC`. Фильтр `is_deleted = false` — в сервисе. Паттерн копировать с существующих `CreatorSocialRepo.ListByCreatorIDs` / `CreatorCategoryRepo.ListByCreatorIDs` (там же `creator.go:188-194`).
- `backend/internal/service/creator_test.go` — расширение: новые мок-expectations, точные аргументы, проверка фильтра удалённой кампании и агрегации по нескольким creator'ам.
- `backend/internal/handler/creator.go` + `handler/creator_test.go` — маппинг domain → API для новых полей; проверка ответа.
- `backend/e2e/creators/list_test.go` — ассерт `activeCampaignsCount` после `testutil.SetupCampaign` + attach креатора через `POST /campaigns/{id}/creators`; покрыть удалённую кампанию.
- `backend/e2e/creators/get_test.go` — ассерт `campaigns[]` (статусы, исключение удалённой кампании, наличие нескольких participations).
- `backend/e2e/testutil/campaign_creator.go` — добавить composable helper `AttachCreatorToCampaign(t, admin, campaignID, creatorID)` + `RegisterCampaignCreatorForceCleanup(t, campaignID, creatorID)`.
- `backend/e2e/testutil/campaign.go` — добавить `SetupCampaign(t, c, adminToken, name)` (легковесный seed без шаблона договора) и `SoftDeleteCampaign(t, campaignID)` через тест-эндпоинт.
- `backend/api/openapi-test.yaml` + `backend/internal/handler/testapi.go` + `backend/internal/repository/campaign.go` / `campaign_creator.go` — два тест-эндпоинта, чтобы покрыть soft-delete и FK-каскад в e2e cleanup:
  - `POST /test/campaigns/{id}/mark-deleted` → `CampaignRepo.MarkDeletedForTests` (flip `is_deleted = true` без бизнес-API).
  - `POST /test/campaign-creators/force-cleanup` → `CampaignCreatorRepo.DeleteByCampaignAndCreatorForTests` (hard-delete pair, обходит soft-delete-gate admin endpoint'а, чтобы LIFO cleanup не падал на FK).
- `frontend/e2e/helpers/api.ts` — клиенты для двух новых тест-эндпоинтов: `markCampaignDeleted` и `forceCleanupCampaignCreator`.
- `frontend/web/src/shared/constants/campaignCreatorStatus.ts` — добавить `CAMPAIGN_CREATOR_DRAWER_GROUPS` (массив `{groupKey, statuses}` в правильном порядке) с TS-exhaustiveness проверкой по `CampaignCreatorStatus`.
- `frontend/web/src/features/creators/CreatorsListPage.tsx` — `buildColumns` вставляет новую `Column<CreatorListItem>` с `key: "activeCampaignsCount"` второй (между `fullName` и `socials`), без `sortable: true`; `0` — стиль `text-gray-400`, `> 0` — обычный. Заголовок колонки через `t("columns.activeCampaignsCount")`.
- `frontend/web/src/features/creators/CreatorsListPage.test.tsx` — проверка новой колонки и приглушённого `0`.
- `frontend/web/src/features/creators/CreatorDrawerBody.tsx` — новый блок «Участие в кампаниях» в конец компонента, ниже sourceApplicationId. Источник данных — `detail.campaigns ?? []` (prefill его не содержит). Группировка через `CAMPAIGN_CREATOR_DRAWER_GROUPS`; внутри группы сохраняется порядок массива от бэка. Каждая строка — `<Link to={ROUTES.CAMPAIGN_DETAIL(c.id)}>` со status-лейблом справа. Empty-state — отдельным компонентом при `campaigns.length === 0`. Локаторы `data-testid` на блок и каждую строку.
- `frontend/web/src/features/creators/CreatorDrawer.test.tsx` — рендер блока, группировка, локализованные лейблы статусов, кликабельный `<Link>` на `/campaigns/{id}`.
- `frontend/web/src/locales/ru/creators.json` — ключи: заголовок колонки, заголовок секции drawer'а, empty-state, названия трёх групп.
- `frontend/e2e/web/admin-creators-list.spec.ts` — расширение flow: pre-seed кампании через `helpers/api.ts` и attach креаторов; проверка счётчика, приглушённого `0`, drawer-блока с группами, перехода на `/campaigns/{id}`.

## Tasks & Acceptance

**Execution:**
- [x] `backend/api/openapi.yaml` -- добавить компонент `CreatorCampaignBrief` и поля в `CreatorListItem` / `CreatorAggregate` -- единый источник API.
- [x] `make generate-api` -- регенерация Go и TS типов -- обязательный gen-pass.
- [x] `backend/internal/domain/creator.go` -- добавить тип `CreatorCampaignBrief` и новые поля -- domain-эквивалент API.
- [x] `backend/internal/repository/campaign_creator.go` -- метод `ListByCreatorIDs` + расширение интерфейса -- batch-чтение participations.
- [x] `backend/internal/repository/campaign_creator_test.go` -- unit-тест на новый метод -- покрытие ≥ 80%.
- [x] `backend/internal/service/creator.go` -- обогащение `List` и `GetByID` тремя batch'ами + фильтр `is_deleted` -- бизнес-логика.
- [x] `backend/internal/service/creator_test.go` -- расширение сценариев -- покрытие новых веток.
- [x] `backend/internal/handler/creator.go` + `creator_test.go` -- маппинг + проверки -- handler-черный-ящик.
- [x] `backend/e2e/creators/list_test.go` -- ассерт счётчика на seed'е активной + удалённой кампании -- расширение.
- [x] `backend/e2e/creators/get_test.go` -- ассерт массива `campaigns[]` -- расширение.
- [x] `frontend/web/src/shared/constants/campaignCreatorStatus.ts` -- `CAMPAIGN_CREATOR_DRAWER_GROUPS` с exhaustiveness -- группировка statuses.
- [x] `frontend/web/src/features/creators/CreatorsListPage.tsx` (+ test) -- колонка `activeCampaignsCount`, стиль `0` -- UI list.
- [x] `frontend/web/src/features/creators/CreatorDrawerBody.tsx` (+ test через `CreatorDrawer.test.tsx`) -- блок «Участие в кампаниях» -- UI drawer.
- [x] `frontend/web/src/locales/ru/creators.json` -- новые ключи без дублей статус-лейблов -- i18n.
- [x] `frontend/e2e/web/admin-creators-list.spec.ts` -- расширение flow со счётчиком и drawer-блоком -- e2e.

**Acceptance Criteria:**
- Given admin запрашивает `GET /creators` страницу с креатором, у которого `N` активных и `M` удалённых кампаний, when сервис обогащает ответ, then `activeCampaignsCount = N`, удалённые не учитываются.
- Given admin открывает drawer креатора с participations в 7 разных статусах, when frontend рендерит блок, then три секции отображаются в порядке «Активные → В процессе → Отказы», пустые группы скрыты, лейблы статусов локализованы.
- Given admin кликает по строке кампании в drawer-блоке, when обработан клик, then браузер переходит на `/campaigns/{id}` соответствующей кампании.
- Given у креатора нет активных кампаний, when открыт drawer, then блок показывает «Не добавлен ни в одну кампанию», а в таблице ячейка счётчика — `0` приглушённого цвета (`text-gray-400`).
- Given выполняется `make test-unit-backend-coverage`, when проверяется coverage gate, then все новые / расширенные методы покрыты ≥ 80%.

## Spec Change Log

- **2026-05-11 — добавление test-эндпоинтов и testutil-хелперов.**
  Триггер: Acceptance Auditor flagged scope drift (новые `POST /test/campaigns/{id}/mark-deleted` и `POST /test/campaign-creators/force-cleanup` не были перечислены в Code Map). Известное «плохое» состояние, которого хотели избежать: e2e сценарий «creator с soft-deleted кампанией» нельзя покрыть только бизнес-API — admin DELETE гейтит soft-deleted, FK на `campaign_creators` без `ON DELETE CASCADE` валит LIFO cleanup. KEEP: тест-эндпоинты остаются gated by `ENVIRONMENT != production`, репо-методы помечены `*ForTests`-суффиксом и недоступны production-сервисам; soft-delete делается только тестовой ручкой, бизнес-API остаётся read-only до отдельной фичи admin delete.

- **2026-05-11 — раунд 2 ревью (extra-bmad-review).**
  Триггер: 7 субагентов (3 стандартных + test-auditor, frontend-codegen-auditor, security-auditor, manual-qa). Выявленные `[major]`-патчи применены: `MarkDeletedForTests` теперь идемпотентен через `WHERE is_deleted = false` (повторный вызов → 404); `loadCreatorParticipations` сортирует `campaignIDs` для детерминистичного порядка args в `ListByIDs` (убрали `mock.MatchedBy` в тестах); `ForceCleanupCampaignCreator` валидирует `uuid.Nil`; orphan-participation triggerит `logger.Warn` вместо тихого drop; добавлен handler unit-тест на parse-error branch (`campaign id`); `Table.tsx` получил `data-testid="column-{key}"` на каждый `<th>` чтобы тесты position-by-testid не зависели от копирайта; drawer-тест покрывает все 7 статусов в одном сценарии (group order + intra-group order + i18n labels); e2e admin-creators-list получил второго creator'а без campaigns + assert `data-dimmed="true"`; `markCampaignDeleted` e2e-helper уважает `E2E_CLEANUP=false`. KEEP: 8 finding'ов записаны в `deferred-work.md` (Loading vs Empty state, SoftDeleteCampaign regression, и др.); 10+ nitpicks rejected silently.

## Verification

**Commands:**
- `make generate-api` -- expected: успешная регенерация Go/TS типов, diff содержит только сгенерированные файлы.
- `make build-backend` -- expected: успешная компиляция Go.
- `make lint-backend` -- expected: 0 нарушений.
- `make test-unit-backend-coverage` -- expected: passes, новые методы ≥ 80%.
- `make lint-web` -- expected: tsc + eslint clean.
- `make test-unit-web` -- expected: passes, новые тесты включены.
- `make test-e2e-backend` -- expected: расширенные `creators/list_test.go`, `creators/get_test.go` passes.
- `make test-e2e-frontend` -- expected: `admin-creators-list.spec.ts` passes.

## Suggested Review Order

**Контракт и доменные типы**

- Контракт-первого подхода вход: новые поля и схема `CreatorCampaignBrief` в OpenAPI.
  [`openapi.yaml:2954`](../../backend/api/openapi.yaml#L2954)

- Go-эквивалент: тип брифа + расширение `CreatorListItem` / `CreatorAggregate`.
  [`creator.go:161`](../../backend/internal/domain/creator.go#L161)

**Репозиторий (batch-чтение без JOIN)**

- Главный новый метод: `ListByCreatorIDs` с empty-input shortcut и `created_at DESC, id DESC`.
  [`campaign_creator.go:225`](../../backend/internal/repository/campaign_creator.go#L225)

- Расширение интерфейса `CampaignCreatorRepo` (включая `*ForTests`-методы).
  [`campaign_creator.go:82`](../../backend/internal/repository/campaign_creator.go#L82)

- Test-only soft-delete для покрытия фильтра `is_deleted` в e2e.
  [`campaign.go:327`](../../backend/internal/repository/campaign.go#L327)

**Сервисный слой (фильтр и сборка)**

- Сердце фичи: общий helper `loadCreatorParticipations` — три batch-запроса + фильтр `is_deleted=false` в Go.
  [`creator.go:290`](../../backend/internal/service/creator.go#L290)

- `GetByID` собирает `Campaigns` сохраняя repo-order (`created_at DESC`).
  [`creator.go:89`](../../backend/internal/service/creator.go#L89)

- `List` хранит `activeCountByCreator` map → `ActiveCampaignsCount` на каждом item'е.
  [`creator.go:218`](../../backend/internal/service/creator.go#L218)

**Handler-маппинг**

- domain → API: маппинг массива `campaigns` и `activeCampaignsCount` в strict-server response.
  [`creator.go:68`](../../backend/internal/handler/creator.go#L68)

**Тест-API (для e2e cleanup)**

- Два test-only endpoint'а: `markCampaignDeleted` + `forceCleanupCampaignCreator`.
  [`testapi.go:221`](../../backend/internal/handler/testapi.go#L221)

**Frontend — статусные группы и колонка**

- Группировка статусов: новый `CAMPAIGN_CREATOR_DRAWER_GROUPS` с compile-time exhaustiveness.
  [`campaignCreatorStatus.ts:43`](../../frontend/web/src/shared/constants/campaignCreatorStatus.ts#L43)

- UI-список: новая колонка `activeCampaignsCount` второй после ФИО, `data-dimmed` для нуля.
  [`CreatorsListPage.tsx:245`](../../frontend/web/src/features/creators/CreatorsListPage.tsx#L245)

- UI-drawer: блок «Участие в кампаниях» с группами, локализованными статусами и `Link` на `/campaigns/{id}`.
  [`CreatorDrawerBody.tsx:184`](../../frontend/web/src/features/creators/CreatorDrawerBody.tsx#L184)

**i18n + e2e**

- Новые ключи: заголовок колонки, секция drawer'а, empty-state, три группы.
  [`creators.json:11`](../../frontend/web/src/shared/i18n/locales/ru/creators.json#L11)

- Backend e2e (list): счётчик с активной + удалённой кампанией, нулевой счётчик у second creator.
  [`list_test.go:483`](../../backend/e2e/creators/list_test.go#L483)

- Backend e2e (get): порядок `created_at DESC`, исключение soft-deleted кампании.
  [`get_test.go:107`](../../backend/e2e/creators/get_test.go#L107)

- Frontend e2e: счётчик + drawer-блок + клик-навигация на `/campaigns/{id}`.
  [`admin-creators-list.spec.ts:295`](../../frontend/e2e/web/admin-creators-list.spec.ts#L295)

## Suggested Review Order — Round 2 (extra-bmad-review)

**Backend — корректность и defensive logging**

- Идемпотентность soft-delete: повторный вызов даёт 404, а не silent re-stamp `updated_at`.
  [`campaign.go:325`](../../backend/internal/repository/campaign.go#L325)

- Детерминистичный порядок аргументов в `ListByIDs` — убрал `mock.MatchedBy` из тестов.
  [`creator.go:330`](../../backend/internal/service/creator.go#L330)

- Defensive `logger.Warn` на orphan participation: FK-нарушение видно ops'ам, не теряется silent'но.
  [`creator.go:344`](../../backend/internal/service/creator.go#L344)

- `uuid.Nil` validation в force-cleanup — misuse даёт 422 вместо misleading 404.
  [`testapi.go:229`](../../backend/internal/handler/testapi.go#L229)

**Unit-тесты (closes coverage gap + новый сценарий)**

- Handler unit-тест на parse-error branch для campaign id (закрывает coverage gap).
  [`creator_test.go:287`](../../backend/internal/handler/creator_test.go#L287)

- Service unit-тест на orphan-participation warn.
  [`creator_test.go:484`](../../backend/internal/service/creator_test.go#L484)

- Repo unit-тесты: повторный soft-delete + idempotence гарантированы.
  [`campaign_test.go:802`](../../backend/internal/repository/campaign_test.go#L802)

- Handler unit-тесты на `uuid.Nil` validation force-cleanup (zero campaign / zero creator).
  [`testapi_test.go:392`](../../backend/internal/handler/testapi_test.go#L392)

**Frontend — стабильность тестов и стилевая проверка**

- Универсальный `data-testid="column-{key}"` на каждом `<th>` — тесты position-by-testid вместо текста.
  [`Table.tsx:66`](../../frontend/web/src/shared/components/Table.tsx#L66)

- Один drawer-тест покрывает все 7 статусов: groups + intra-group + i18n labels.
  [`CreatorDrawer.test.tsx:359`](../../frontend/web/src/features/creators/CreatorDrawer.test.tsx#L359)

- Position-by-testid вместо «ФИО / В кампаниях» — не ломается на копирайт-изменениях.
  [`CreatorsListPage.test.tsx:150`](../../frontend/web/src/features/creators/CreatorsListPage.test.tsx#L150)

**E2E — расширенное покрытие AC#4 + надёжность**

- Второй creator с 0 кампаниями → проверка `data-dimmed="true"` (AC#4).
  [`admin-creators-list.spec.ts:315`](../../frontend/e2e/web/admin-creators-list.spec.ts#L315)

- Anchored regex `^.+/campaigns/{id}$` против ложных positives.
  [`admin-creators-list.spec.ts:412`](../../frontend/e2e/web/admin-creators-list.spec.ts#L412)

- E2E helper: типизированный body + `E2E_CLEANUP=false` skip для `markCampaignDeleted`.
  [`api.ts:751`](../../frontend/e2e/helpers/api.ts#L751)

**Backlog**

- Восемь findings (Loading vs Empty state, SoftDeleteCampaign regression, IN-list cap, и др.) — `deferred-work.md`.
  [`deferred-work.md`](./deferred-work.md)
