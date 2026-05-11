# Intent: видимость участия креатора в кампаниях

> **Стандарты обязательны.** Перед реализацией исполнитель полностью загружает
> `docs/standards/` и сверяет каждое решение с релевантным стандартом
> (`backend-architecture.md`, `backend-repository.md`, `frontend-state.md`,
> `frontend-components.md` и т.д.). Артефакт фиксирует итоговое состояние, не
> историю обсуждения.

## Контекст / проблема

Креаторы попадают в кампании (`campaign_creators`), но в админ-интерфейсе нет
видимости их участия. На странице `/creators` нельзя понять, распределён ли
креатор по кампаниям. Когда креатор пишет в поддержку, оператор не видит, в
какой кампании он состоит, и приходится вручную джойнить таблицы через SQL.

## Решение

1. **Список креаторов** — добавить отображение «количество активных кампаний»
   у каждой строки (активная = `campaigns.is_deleted = false`).
2. **Детали креатора (drawer)** — добавить блок со списком активных кампаний,
   в которых состоит креатор.

## Архитектура: обогащение в сервисе, без JOIN-ов в repo

Существующий `CreatorRepo.List` не модифицируется. `CreatorService.List` /
`CreatorService.GetByID` обогащают результат тремя батч-запросами — по тому же
паттерну, что уже используется для `socials` и `categories`
(`CreatorSocialRepo.ListByCreatorIDs`, `CreatorCategoryRepo.ListByCreatorIDs`).

Поток в `CreatorService.List`:

1. `CreatorRepo.List(params)` → `[]*CreatorListRow` (без изменений).
2. **Новый** `CampaignCreatorRepo.ListByCreatorIDs(creatorIDs)` →
   `[]*CampaignCreatorRow` с парами `creator_id → campaign_id` (+ статус
   participation для деталей).
3. `CampaignRepo.ListByIDs(uniqueCampaignIDs)` (метод уже есть в
   `backend/internal/repository/campaign.go:165`) → `[]*CampaignRow`
   (включая поле `is_deleted` — фильтр в сервисе).
4. Сервис фильтрует `is_deleted = false` и собирает
   `map[creatorID]int` (для списка) / `map[creatorID][]CampaignBrief` (для
   деталей).

Фильтр `is_deleted` — бизнес-решение, остаётся в сервисе по
`backend-architecture.md` § Ответственности слоёв.

## API surface

### Список — `CreatorListItem`

Добавляется одно скалярное поле `activeCampaignsCount: integer` —
количество кампаний с `is_deleted = false`, в которых состоит креатор.
Показывается отдельной колонкой в таблице.

### Детали — `GetCreatorResult` / `CreatorAggregate`

Добавляется массив `campaigns: CreatorCampaignBrief[]`, где каждая запись
содержит:

- `id: uuid` — id кампании;
- `name: string` — название кампании (актуальное);
- `status: string` — статус participation (`campaign_creators.status`).

В UI каждая строка drawer-блока — кликабельная ссылка на `/campaigns/{id}`,
рядом с названием отображается статус participation.

### Статусы и порядок строк в drawer

Используется существующий enum `CampaignCreatorStatus` (7 значений:
`planned`, `invited`, `declined`, `agreed`, `signing`, `signed`,
`signing_declined`) и существующие локали из
`frontend/web/src/features/campaigns/creators/`.

Строки группируются по статусу в три укрупнённые группы и идут именно в
этом порядке:

1. **Активные** — `signed`, `signing`, `agreed`.
2. **В процессе** — `invited`, `planned`.
3. **Отказы** — `declined`, `signing_declined`.

Внутри группы — сортировка по `campaign_creators.created_at DESC` (самые
свежие добавления креатора в кампанию — сверху).

## Ограничения архитектуры

Поскольку счётчик и список считаются обогащением в сервисе после
получения страницы креаторов из `CreatorRepo.List`, **сортировка списка
креаторов по `activeCampaignsCount` не поддерживается** — это потребовало
бы JOIN внутри основного SQL и нарушило бы решение «не трогаем существующий
запрос». Никакого нового `CreatorListSortField` для счётчика не вводим.

## Frontend

### Список — таблица `/creators`

Новая колонка `activeCampaignsCount` вставляется **второй** (сразу после
`fullName`, перед `socials`). Колонка не сортируемая (см. «Ограничения
архитектуры»).

Формат ячейки:

- `N > 0` — число обычным цветом текста.
- `0` — число `0` приглушённым цветом (`text-gray-400`), чтобы визуально
  выделить нераспределённых креаторов в списке без агрессивного alert-стиля.

### Drawer — `CreatorDrawerBody`

Новый блок «Участие в кампаниях» добавляется в конец drawer'а, ниже
существующих секций (категории, соцсети, телеграм, sourceApplicationId).

Структура блока:

- Если активных кампаний нет — заголовок секции + текст
  «Не добавлен ни в одну кампанию».
- Иначе — три визуальные группы в порядке «Активные / В процессе /
  Отказы». Группы с нулём кампаний — не отрисовываются. Каждая строка —
  кликабельная ссылка на `/campaigns/{id}` с локализованным
  status-лейблом справа от названия.

## Тесты

### Backend unit

- Новый `CampaignCreatorRepo.ListByCreatorIDs` — repo-тест с pgxmock
  (точная SQL-строка, литерал имени колонки, маппинг row → struct,
  empty-input → empty-output, error propagation). Покрытие ≥ 80%.
- `CreatorService.List` / `CreatorService.GetByID` — расширение
  существующих тестов (`service/creator_test.go`):
  агрегация трёх запросов (creators + campaign_creators + campaigns),
  фильтрация `is_deleted = true`, корректность `activeCampaignsCount` и
  `campaigns[]`, поведение при отсутствии participations / при наличии
  только удалённых кампаний. Точные mock-аргументы на каждом
  expectation.
- `CreatorHandler` — расширение `handler/creator_test.go` под новые поля
  ответа.

### Frontend unit

- Колонка `activeCampaignsCount` в `CreatorsListPage`: рендер числа,
  приглушённый стиль для `0`, обычный — для `> 0`.
- `CreatorDrawerBody`: empty-state «Не добавлен ни в одну кампанию»,
  три визуальные группы, скрытие пустых групп, кликабельность ссылок,
  локализованный лейбл статуса.

### Backend E2E

Расширение существующих файлов (новые не создаются):

- `backend/e2e/creators/list_test.go` — добавить ассерт на
  `activeCampaignsCount` после `testutil` setup'а кампаний и
  attachment'а креатора через бизнес-ручку `POST /campaigns/{id}/creators`.
  Покрыть случаи: ноль participations, несколько активных кампаний,
  удалённая кампания (`is_deleted = true`) не учитывается.
- `backend/e2e/creators/get_test.go` — добавить ассерт на массив
  `campaigns[]`: порядок групп, статусы, исключение удалённых кампаний.

### Frontend E2E

Расширение существующих файлов (новые не создаются):

- `frontend/e2e/web/admin-creators-list.spec.ts` — после существующего
  flow «зашёл, увидел список, открыл drawer» добавить проверки:
  - в таблице счётчик активных кампаний у конкретного креатора
    отображается корректно (с pre-seeded participations через
    `helpers/api.ts`);
  - креатор без participation показывает приглушённый `0`;
  - в drawer виден новый блок, группы отображаются в нужном порядке,
    клик по строке ведёт на `/campaigns/{id}`.

## Контракты, локали, авторизация

### OpenAPI

В `backend/api/openapi.yaml`:

- Новый компонент `CreatorCampaignBrief` со схемой `{id: uuid, name: string,
  status: $ref CampaignCreatorStatus}`.
- В `CreatorListItem` — required-поле `activeCampaignsCount: integer`
  (минимум 0).
- В `GetCreatorResult` (а точнее в `CreatorAggregate`, на который он
  ссылается) — required-поле `campaigns: CreatorCampaignBrief[]`.

После правки `openapi.yaml` — `make generate-api`. Никаких ручных
типов/обёрток (`backend-codegen.md`, `frontend-types.md`).

### i18n

Расширение `frontend/web/src/locales/ru/creators.json`:

- Заголовок секции drawer'а («Участие в кампаниях»).
- Empty-state («Не добавлен ни в одну кампанию»).
- Названия трёх групп («Активные», «В процессе», «Отказы»).
- Заголовок колонки таблицы.

Status-лейблы (`planned` … `signing_declined`) — переиспользуются из
существующего `frontend/web/src/locales/ru/campaigns.json` (или из
common-namespace, если уже вынесены). Дублировать запрещено.

### Авторизация

Без изменений: `GET /creators` и `GET /creators/{id}` — admin-only (как
сейчас). Service-метод обогащения наследует те же проверки auth
middleware — отдельных правок authz не требуется.

### Производительность

Размер IN-листов ограничен `perPage` (текущий `PER_PAGE = 50` в
`CreatorsListPage`) — отдельные лимиты не требуются. Cap на `perPage`
живёт в existing `CreatorListInput` валидации, добавлять новый — не
нужно.

## Миграции и аудит

Не требуются — фича read-only.
