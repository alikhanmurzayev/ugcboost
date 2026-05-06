---
title: "Intent: бэк-ручка списка креаторов (POST /creators/list)"
type: intent
status: draft
chunk: "campaign-roadmap chunk 1"
created: "2026-05-05"
updated: "2026-05-05"
---

# Intent: бэк-ручка списка креаторов (chunk 1)

> **Преамбула.** Перед стартом реализации агент обязан полностью загрузить
> `docs/standards/` (все файлы целиком) и применять их как hard rules. Этот
> документ — only intent, не PRD: фиксирует, ЧТО строим и ПОЧЕМУ так,
> а не пошаговую реализацию.

## Контекст

Первый chunk roadmap'а кампаний (`_bmad-output/planning-artifacts/campaign-roadmap.md`).
Цель — отдать админке список approved-креаторов, которых можно
выбирать в кампании. Источник данных — таблица `creators` (заполняется
в момент approve креатор-аппликейшна, миграция `20260505052212_creators.sql`).
Аналитика LiveDune и orders-метрики не подключены — эти колонки
прототипа в чанк не попадают.

**Прототип Айданы:** `frontend/web/src/_prototype/features/creatorApplications/CreatorsPage.tsx`.

## API surface

`POST /creators/list` — admin-only. POST потому что в `search` идёт PII
(ФИО, ИИН, social-handle), URL-параметры это запрещают
(`docs/standards/security.md` § PII). Authorisation evaluated **до**
любых DB-чтений, чтобы non-admin caller получал 403 одинаково и
без timing-side-channel'а на «креаторы есть/нет». Audit-row не пишем
(read-only).

Шаблон один-в-один с `POST /creators/applications/list` —
тот же подход доказал себя в моде; повторно изобретать пагинацию/
сортировку/фильтры не имеет смысла.

## Item-shape

Полностью lean его не делаем — для будущих UX-нужд (быстрый поиск в
таблице, hover/drawer без отдельного round-trip'а, копирование телефона
прямо из админки) добавляем поля, по которым идёт search. Они сейчас
не отображаются на фронте чанка 2, но возвращаются в body — admin-only,
POST, в URL не уходят, `security.md` § PII не нарушается.

| Поле | Тип | Источник |
|---|---|---|
| `id` | uuid | `creators.id` |
| `lastName` | string | `creators.last_name` |
| `firstName` | string | `creators.first_name` |
| `middleName` | string \| null | `creators.middle_name` |
| `iin` | string | `creators.iin` |
| `birthDate` | date | `creators.birth_date` (FE сам считает возраст) |
| `phone` | string | `creators.phone` |
| `city` | `DictionaryItem {code, name, sortOrder}` | `creators.city_code` + hydrate из `cities` (deactivated → fallback `(code, code, 0)`) |
| `categories[]` | `DictionaryItem[]` | `creator_categories` JOIN `categories`, sort by `sort_order, code` |
| `socials[]` | `[{platform, handle}]` | `creator_socials`, sort by `platform, handle` |
| `telegramUsername` | string \| null | `creators.telegram_username` |
| `createdAt` | date-time | `creators.created_at` |
| `updatedAt` | date-time | `creators.updated_at` |

**Что НЕ включаем:** `address, category_other_text, telegram_user_id,
telegram_first_name, telegram_last_name, source_application_id`
(остаются в `GET /creators/{id}`), `rating / completedOrders / activeOrders`
(LiveDune/orders ещё не реализованы, прототипные колонки выкидываем),
`gender` (в схеме нет — не прокидываем; решение зафиксировано на 2026-05-05).

## Filters / sort / pagination

Pagination через общий `PaginationInput` (`page>=1`, `perPage 1..200`).
Sort и order — **required**, без серверных дефолтов: клиент явно
выбирает порядок.

**Filter set** (все опциональные, комбинируются AND между полями,
OR внутри массива):

- `cities[]` — коды городов (multi-select из словаря)
- `categories[]` — коды категорий (EXISTS-subquery по `creator_categories`,
  match при пересечении хотя бы одной)
- `dateFrom` / `dateTo` — `created_at` диапазон (когда аппрувнули)
- `ageFrom` / `ageTo` — диапазон возраста в полных годах,
  переводим в `birth_date <= NOW() - interval`
- `search` — substring ILIKE по `last_name, first_name, middle_name, iin,
  phone, telegram_username` + EXISTS по `creator_socials.handle`.
  Trim+lower; пустое/whitespace игнорируем; wildcard-метасимволы
  `% _ \\` экранируем (преемственно из applications/list, чтобы
  search «100%» не возвращал всё подряд). По `phone` ищем substring —
  чтобы admin мог вставить «77012345678», «+77012345678» или последние
  4 цифры и попасть. Длину `search` ограничиваем (`maxLength: 128`,
  как в applications) — иначе free-text без bound = DoS-вектор
  (`security.md` § Length bounds)

**Sort fields** (enum `CreatorListSortField`):
`created_at | updated_at | full_name | birth_date | city_name`.
В UI чанка 1 фронт использует только `full_name | birth_date | city_name`
(default `{full_name, asc}`), но бэкенд-контракт держим полным —
расширим UI без миграции контракта.

**Резолюция sort-полей в SQL** (паттерн applications/list, всё с
хвостом `id ASC` для стабильности страниц при равных ключах):

- `created_at` / `updated_at` / `birth_date` — одна колонка `DIR`, `id ASC`
- `full_name` — `last_name DIR, first_name DIR, middle_name DIR, id ASC`
  (`NULL middle_name` ордерится Postgres-дефолтом: NULLS LAST на ASC,
  NULLS FIRST на DESC — устраивает; alphabet-locale соблюдается через
  `LC_COLLATE`, как и для applications)
- `city_name` — `cities.name DIR, id ASC` (LEFT JOIN на `cities`
  включается только в этой ветке; deactivated city → ct.name = NULL,
  ордерится тем же дефолтом)

## Слои

- **Handler** (`handler/creator.go: ListCreators`): валидация формата
  (page/perPage bounds, sort/order whitelisted) — кодгеном.
  Вызов `authzService.CanViewCreators(ctx)` ПЕРВЫМ. Trim+lower search;
  пустой → `nil`. Маппит response через сгенерированные `api.*` типы.
- **Service** (`service/creator.go: List`): принимает уже валидированные
  параметры, конструирует `repository.CreatorListParams`, делает один
  pool-вызов `repo.List(ctx, params)`, гидратирует city/categories через
  `DictionaryRepo.GetActiveByCodes` пакетно (один SELECT по cities,
  один по categories на всю страницу). Read-only — без `WithTx`,
  без audit. Deactivated codes → fallback на `(code, code, 0)`
  (паттерн `CreatorService.GetByID`).
- **Repository** (`repository/creator.go`):
    - `CreatorRepo.List(ctx, CreatorListParams) → ([]*CreatorListRow, total int64, error)`.
      `CreatorListRow` — `id, last_name, first_name, middle_name, iin,
      birth_date, phone, city_code, telegram_username, created_at,
      updated_at`. Без `address, category_other_text, telegram_user_id,
      telegram_first_name, telegram_last_name, source_application_id`
      — они нужны только в `GetByID`. Без `telegram_linked`-проекции
      (у approved-креатора TG привязан по инварианту onboarding'а). Один SELECT для page + один
      COUNT(*) для total с идентичной WHERE-цепочкой. LEFT JOIN на
      `cities` подключается лишь при `sort=city_name`.
    - **Refactor — единый batched-метод вместо single+batch.**
      Вместо «добавить второй метод» удаляем `ListByCreatorID(ctx, id)`
      и заводим только `ListByCreatorIDs(ctx, ids)` — один-в-один шаблон
      `creator_application_*.ListByApplicationIDs`:
      `CreatorSocialRepo.ListByCreatorIDs(ctx, ids) → map[creatorID][]*CreatorSocialRow`,
      `CreatorCategoryRepo.ListByCreatorIDs(ctx, ids) → map[creatorID][]string`.
      Существующий call-site `CreatorService.GetByID` переписывается на
      batched-вариант с массивом из одного id; результат достаётся из
      map[creatorID]. SQL почти тот же (`WHERE creator_id IN (?)`
      вместо `WHERE creator_id = ?`), API сужается. Никаких deprecated-
      обёрток, обратной совместимости — call-site всего один.
      Тесты `creator_social_test.go` / `creator_category_test.go`
      переписываются на batched-форму (single-id case покрывается
      одним из table-driven подсценариев).
    - City hydrate — уже существующий `DictionaryRepo.GetActiveByCodes`
      (тот же, что использует `CreatorService.GetByID`).

## AuthZ

Добавить `AuthzService.CanViewCreators(ctx) error` — отдельный метод
от существующего `CanViewCreator` (single). Сейчас оба admin-only
с одинаковой реализацией, но держим раздельно: future-proof, чтобы
ослабить любой из них (manager видит список, но не детали; или
наоборот) можно было без обратной связности через регрессии.

## Тесты

- **Unit** (≥80% per-method gate): handler / service / repository
  по стандарту `backend-testing-unit.md`. Repo — pgxmock со строгим
  ассертом SQL для page-q, count-q, children-q (включая
  ORDER BY tail-id, EXISTS-subquery по categories, ESCAPE-search).
- **E2E** (`backend/e2e/creators/list_test.go` — папка существует
  вместе с `get_test.go`): composable seed через approve-flow.
  В `testutil/creator.go` нужно **добавить новый helper**
  `SetupApprovedCreator(t, ...)` — комбинация существующих
  `SetupCreatorApplicationInModeration` + admin-approve через
  `POST /creators/applications/{id}/approve`; helper регистрирует
  `RegisterCreatorCleanup` для созданного `creator_id`.
  Полный набор сценариев: 401 (no auth), 403
  (manager), 422 (bad pagination, unknown sort, unknown order),
  happy path (3+ креатора, page=1 perPage=2, all filters off,
  default sort), search hits across name/IIN/handle (по букве —
  частичный match), filter by city, filter by categories
  (any-of OR), date range, age range, sort across each enum value
  + asc/desc, pagination boundaries (last page partial, beyond-last
  empty). Все ассерты — `require.Equal` целиком после подмены
  динамических полей (id/createdAt/updatedAt). Race: rapid
  approve+list sanity-проверка не нужна — read-only.

## Что НЕ делаем в этом чанке

- LiveDune-блоки (rating, qualityIndicator, metrics) — отдельный чанк
- Orders-метрики (completedOrders, activeOrders) — нет домена
- `signedAt` — TrustMe-флоу (group 7 в roadmap'е)
- Cursor-pagination — текущий объём (~100 креаторов на пик-event) лежит
  в offset спокойно; cursor добавим, когда таблица перейдёт в десятки тысяч
- Gender — решение «не прокидываем» (нет в схеме, в прототипе тоже
  не было; вернёмся, если бизнес явно потребует под фильтры кампаний)
- Frontend (chunk 2) — отдельный артефакт

## Связанные артефакты

- Roadmap: `_bmad-output/planning-artifacts/campaign-roadmap.md`
- Brainstorming: `_bmad-output/brainstorming/brainstorming-session-2026-05-05-1808.md`
- Прототип: `frontend/web/src/_prototype/features/creatorApplications/CreatorsPage.tsx`
- Эталон контракта: `backend/api/openapi.yaml` `/creators/applications/list`
- Стандарты: `docs/standards/`
