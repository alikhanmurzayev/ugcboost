# Deferred Work

Findings, surfaced by reviews/work, that we consciously kicked down the road. Each entry: what, why deferred, where it surfaced.

---

## creator approve with campaigns — review round 1 (2026-05-07)

### `CampaignRepo.ListByIDs` без internal cap на размер ids
**Что:** хендлер уже cap'ает к 20 (`approveCampaignsMax`), но публичный `repository.CampaignRepo.ListByIDs` не валидирует `len(ids)`. Будущий caller с десятками тысяч UUID сгенерирует `WHERE id IN (...)` за пределами Postgres limits (`max_prepared_params ≈ 65k`), pgx упадёт.
**Почему отложено:** для текущей feature защита есть на handler-уровне; для других callsites (если появятся) фикс — внутренний cap или chunking. Не блокер.
**Когда возвращаться:** при появлении нового потребителя `ListByIDs` вне approve-flow.

### `listCampaigns({ perPage: 100 })` в ApproveApplicationDialog без server-search
**Что:** multiselect загружает первую сотню кампаний. Пока admin'у их меньше 100 — это полностью покрывает UX. При росте каталога multiselect молча обрежет хвост (search фильтрует только подгруженный массив).
**Почему отложено:** для MVP-сцены 100 кампаний — заведомо запас. Server-search потребует поднять `SearchableMultiselect` до debounce-инициированного query — отдельный PR, общий для всех будущих больших списков.
**Когда возвращаться:** при появлении кампаний 100+ или при миграции SearchableMultiselect на server-side filtering.

### Дублирующийся `extractErrorCode` / `extractErrorMessage` в `frontend/web/src/api/*.ts`
**Что:** `extractErrorMessage` добавлен только в `creatorApplications.ts`. Те же helpers (с `code` и без `message`) повторяются в 8+ модулях. После общего рефакторинга `extractError` должен стать shared в `api/client.ts`.
**Почему отложено:** ortho-задача, ранее уже зафиксирована в deferred (chunk 11 slice 1/2 review round 2).
**Когда возвращаться:** объединить с тем deferred-entry в один PR.

### Approve add-loop без явного deadline на `approveCtx`
**Что:** `service/creator_application.go` оборачивает post-tx1+notify add-loop в `context.WithoutCancel(ctx)`, но без `WithTimeout`. Если 20 кампаний по очереди висят на FK lock'ах, цикл может бежать минуты — клиент уже отвалился по HTTP read-timeout, а сервер всё ещё пишет.
**Почему отложено:** в проде каждая `Add` транзакция короткая (один INSERT + audit), реальная вероятность многоминутного зависания мала; добавление таймаута требует Config-канала (`ENV ApproveAddLoopTimeout`) и обновления ~50 NewCreatorApplicationService call-site'ов в тестах. По стандарту backend-design.md hardcoded таймауты запрещены, поэтому inline-захардкодить тоже нельзя.
**Когда возвращаться:** при первой жалобе на медленный approve, или при добавлении любого другого Config-параметра в service (тогда +1 поле в той же миграции).

---

## chunk 11 slice 2/2 — campaign_creators frontend mutations (PR #?)

### Search input в drawer'е без debounce — запрос на каждое нажатие клавиши
**Что:** `DrawerCreatorFilters` пишет в `filters.search` на каждое onChange → `creatorKeys.list(listInput)` → новый key → fetch. Идентично существующему `CreatorsListPage` без debounce.
**Почему отложено:** общий паттерн с `CreatorsListPage`; единый рефактор должен быть отдельным PR (debounce 300ms через хук + замена в обоих компонентах).
**Когда возвращаться:** когда заметим заметную нагрузку на /creators/list или жалобы UX.

### `loginAs` / `withTimeout` дублируются в 4-х e2e specs — нет `helpers/ui-web.ts`
**Что:** `frontend/e2e/helpers/` имеет только `api.ts` и `telegram.ts`. Спека `frontend-testing-e2e.md` § Хелперы предписывает `helpers/ui-web.ts`. Сейчас `loginAs(page, email, password)` и `withTimeout` — приватные копии в `admin-campaign-creators-mutations.spec.ts`, `admin-campaign-creators-read.spec.ts`, `admin-campaigns-list.spec.ts`, `admin-campaign-detail.spec.ts`.
**Почему отложено:** существующий паттерн в 4-х файлах; рефактор в `helpers/ui-web.ts` требует обновления всех specs одним PR.
**Когда возвращаться:** при следующем добавлении 5-го spec'а с тем же `loginAs` либо при изменении login-flow.

### Partial-success при add игнорируется (если backend когда-нибудь начнёт возвращать `data.items.length < creatorIds.length` без 422)
**Что:** Сейчас спека backend'а строгая — strict-422 на любой conflict. Frontend не сравнивает `added.length` с `selected.size`. Если контракт изменится на partial-success — silent UX-баг (drawer закроется, юзер думает что добавлены все).
**Почему отложено:** контракт чёткий (strict-422), вероятность изменения — низкая.
**Когда возвращаться:** при изменении API-контракта на batch-add (см. `backend/api/openapi.yaml#addCampaignCreators`).

### Reopen drawer после close сохраняет stale `creatorKeys.list` cache
**Что:** Drawer закрылся, не unmount'ится parent'ом — `listQuery` остаётся в cache. При reopen с теми же filters/sort/page стейл-данные используются (default staleTime). Если другой admin успел изменить creators-table в этот промежуток, юзер видит старые данные.
**Почему отложено:** UX-минор; staleTime tuning или `refetchOnMount` — отдельная задача.
**Когда возвращаться:** при появлении симптомов (репорт «выбрал креатора, который уже soft-deleted»).

### Page > totalPages при concurrent shrink — нет clamp
**Что:** Если в drawer'е admin на page=N, а параллельный admin удалил много креаторов — totalPages мог уменьшиться, page=N покажет пустую страницу (хотя на page=1 данные есть).
**Почему отложено:** edge редкий; happy path после submit reset'ит page=1; для остальных случаев общий patch с `useEffect`-clamp нужен на всех paginated UI (creators-list, audit и т.п.).
**Когда возвращаться:** одним общим PR clamp'а во всех paginated-компонентах.

### Focus restore после закрытия RemoveCreatorConfirm
**Что:** После Esc / Cancel / Submit фокус идёт в `<body>`, не возвращается на trash-кнопку, которая открыла confirm. Стандартный a11y.
**Почему отложено:** требует ref-pattern на `document.activeElement` при open=true→false; такой же gap у других modals в проекте.
**Когда возвращаться:** общим PR a11y-улучшений с focus-trap'ом и focus-restore (после Drawer/Modal лиц-стандартизации).

### `formatShortDate` дублируется в 3 файлах
**Что:** `formatShortDate` копи-пастится в `CampaignCreatorsTable.tsx`, `CreatorsListPage.tsx`, `AddCreatorsDrawerTable.tsx`. Кандидат на `shared/utils/formatDate.ts`.
**Почему отложено:** rule of three exactly; вынос — отдельный refactor PR без функциональных изменений.
**Когда возвращаться:** при следующем (4-м) появлении.

### `mapColumnToSortField` / `mapSortFieldToColumn` в drawer'е дублируют логику из `features/creators/sort.ts`
**Что:** В `AddCreatorsDrawer.tsx` есть две локальные функции маппинга column↔sort field. В `features/creators/sort.ts` уже есть `fieldForColumn` / `activeColumnForSort` через `ColumnFieldMap`. Можно переиспользовать.
**Почему отложено:** существующий map в `sort.ts` — sub-set нужных полей; миграция требует расширения `DEFAULT_COLUMN_TO_FIELD` и аккуратной проверки CreatorsListPage. Не в scope round 2.
**Когда возвращаться:** при следующем feature-добавлении column'а с sort.

### Number(NaN) валидация в age input в DrawerCreatorFilters / CreatorFilters
**Что:** `onChange={(e) => onChange({ ...filters, ageFrom: e.target.value ? Number(e.target.value) : undefined })}` — для `"abc"` даст `NaN`, для `"1e10"` пройдёт; нет cross-validation `ageFrom > ageTo`. Backend ловит это 422-валидацией.
**Почему отложено:** существующая проблема в `features/creators/CreatorFilters.tsx` (Spec B); фикс должен быть единым в обоих компонентах. Не блокер для slice 2/2 mutations.
**Когда возвращаться:** одним PR на оба компонента с `Number.isFinite`, clamp 14..100, cross-проверкой.

### Двойной Escape handler popover + Drawer в AddCreatorsDrawer
**Что:** `Drawer` вешает window-level keydown на `Escape`; `DrawerCreatorFilters` тоже. Когда popover открыт внутри drawer'а — Escape закроет ОБА сразу.
**Почему отложено:** UX-минор; `e.stopImmediatePropagation()` через capture-phase решит, но требует продумывания приоритетов keydown.
**Когда возвращаться:** при первой жалобе пользователя.

### AddCreatorsDrawer.tsx 326 строк > 150-строкового сигнала
**Что:** Spec'овское требование `frontend-components.md`: компонент >150 строк — сигнал к декомпозиции.
**Почему отложено:** компонент cohesive; rule of three для shared hook ещё не выполнен.
**Когда возвращаться:** при появлении следующего mutate-drawer'а (chunks 12/13).

### Outside-click handler в DrawerCreatorFilters при portal-popovers
**Что:** `useEffect(handlePointer)` закроет popover на клик «вне containerRef». Если SearchableMultiselect/DateRangePicker отрендерит popover через portal — клик внутри их popover'а будет outside.
**Почему отложено:** существующий `features/creators/CreatorFilters.tsx` имеет ту же логику; popovers рендерятся inline сейчас.
**Когда возвращаться:** если SearchableMultiselect/DateRangePicker мигрирует на portal-rendering.

### CapCounter без pluralization (deviation от Code Map)
**Что:** Spec Code Map обещал `capCounter_*` с pluralization для 0/1/N; реально использован один ключ `capCounter`. Pluralization rules для русского в i18next не настроены в проекте.
**Почему отложено:** текст работает (счётчик числовой); добавление i18next plurals — отдельная задача (config + тесты на all forms).
**Когда возвращаться:** при глобальной настройке i18next plural rules.

### Pagination overflow в AddCreatorsDrawer (page > totalPages)
**Что:** Если в drawer'е admin на page=N, а потом backend total уменьшился (concurrent admin удалил много креаторов) — drawer покажет пустую страницу, хотя данные есть на page=1.
**Почему отложено:** успешный submit закрывает drawer (fresh state на reopen). Concurrent-shrinking-without-close — узкий race, для chunk 11 не критичен.
**Когда возвращаться:** одним общим патчем со всеми pages в проекте (creators list, audit и т.п.) через `useEffect`-clamp `setPage(min(page, totalPages))`.

---

## 2026-05-07 — chunk 11 slice 1/2 review round 2 (6 субагентов)

### Дублирующийся `extractErrorCode` в каждом `frontend/web/src/api/*.ts`
**Что:** одна и та же функция `extractErrorCode` живёт в 8 модулях (auth, audit, brands, campaigns, creators, creatorApplications, dictionaries, campaignCreators).
**Почему отложено:** out-of-scope для frontend-read PR. Разрешается одним отдельным рефакторингом (вынос в `api/client.ts`).
**Когда возвращаться:** следующий PR, трогающий несколько `api/*.ts`.

### `formatShortDate` дубликат — 7 копий по фичам
**Что:** идентичная утилита `formatShortDate(iso)` живёт в `CreatorsListPage.tsx`, `CampaignsListPage.tsx`, `Verification/Moderation*Page.tsx`, теперь и в `CampaignCreatorsTable.tsx`. Rule of three превышено более чем вдвое.
**Почему отложено:** wholesale refactor через `shared/utils/formatDate.ts` касается 7 файлов — отдельный PR.
**Когда возвращаться:** при следующей правке формата короткой даты или новой таблицы со short date.

### `<tr role="button" onClick>` в shared `<Table>` вместо `<Link>` per cell
**Что:** Accessibility-правило `frontend-quality.md`: «Кликабельные строки таблиц — через `<Link>` внутри ячейки, не `onClick` на `<tr>`». Shared Table использует onClick на tr для всех страниц.
**Почему отложено:** трогать `Table.tsx` с `<Link>` API меняет 5+ потребителей одновременно. Самостоятельный PR.
**Когда возвращаться:** при WCAG-аудите либо при первой жалобе на keyboard UX.

### `data-selected` атрибут теперь на ВСЕХ строках всех таблиц проекта
**Что:** изменение `Table.tsx` ставит `data-selected="true|false"` на каждый `<tr>` — расширяет публичный «контракт» компонента, без записи в `docs/standards/frontend-*`.
**Почему отложено:** атрибут не содержит CSS-стилей и не ломает существующих потребителей (e2e/unit тесты на /creators, /campaigns, /brands не ассертят `data-selected`). Отдельный документационный PR — слишком мелко.
**Когда возвращаться:** если кто-то добавит глобальный CSS-селектор `[data-selected]` или появится PR обновляющий `docs/standards/frontend-components.md`.

### e2e cleanup-стек обрывается на первом упавшем cleanup
**Что:** в `admin-campaign-creators-read.spec.ts` (и других spec'ах) `while(cleanupStack.length>0) { await withTimeout(fn(), 5000) }` без try-catch. Один сбой → остаток стека не отрабатывает → утечка строк.
**Почему отложено:** pre-existing pattern. Решение — обернуть `withTimeout` в try-catch + warn — изменение паттерна 5+ файлов.
**Когда возвращаться:** при flaky cleanup в CI.

---

## 2026-05-07 — chunk 11 slice 1/2 review round 1

### `useCampaignCreators`: hard-cap 200 без chunking
**Что:** `listCreators({ids: creatorIds, perPage: 200, ...})` не chunkает `creatorIds`. Если кампания накопит >200 креаторов, listCreators отдаст 422.
**Почему отложено:** spec явно говорит «`perPage=200` покрывает chunk-creator-cap из chunk 10». Backend chunk 10 ограничивает добавление 200 креаторами на кампанию суммарно. Слайс 2/2 (Add drawer) добавит UI hard-cap 200. До тех пор upper bound гарантирован контрактом.
**Когда возвращаться:** если бизнес попросит снять кэп креаторов на кампанию.

### Row ordering: creator.created_at vs campaign_creator.created_at
**Что:** строки сортируются по `creator.created_at desc` (порядок listCreators), а не по `campaign_creator.created_at` (порядок добавления в кампанию). Креатор, добавленный сегодня, но created_at год назад, попадёт в конец.
**Почему отложено:** spec буквально фиксирует `sort: "created_at"` для listCreators — текущая реализация не отклоняется. Spec gap: какой порядок ожидает PM/UX. Нужно согласовать.
**Когда возвращаться:** после first user feedback на staging — Aidana подскажет, какой порядок ей удобнее. Если cc.created_at desc — добавить sort на стороне фронта (или в backend listCampaignCreators).

### Хардкод символа `№` в `CreatorsListPage.buildColumns`
**Что:** `header: "№"` в `CreatorsListPage.tsx:229` — литерал в JSX без `t(...)`. В `CampaignCreatorsTable.tsx` round 2 уже фикснут на `t('creators:columns.index')`, ключ добавлен в `creators.json`.
**Почему отложено:** правка `CreatorsListPage.tsx` out-of-scope для slice 1/2 PR. Ключ `creators:columns.index` уже доступен — заменить за один лайн в следующий касательный PR.
**Когда возвращаться:** ближайший PR трогающий `CreatorsListPage`.

### `formatShortDate` дублируется в каждом feature-таблице
**Что:** одинаковая функция `formatShortDate` живёт в `CreatorsListPage.tsx` и `CampaignCreatorsTable.tsx`.
**Почему отложено:** rule of three — пока 2 копии, выносить в `shared/utils/formatDate.ts` рано. Появится третья — выносим.
**Когда возвращаться:** при появлении третьей таблицы со short date.

### `<SocialLink>`-cell блокирует пропагацию click — keyboard nav через cell не открывает drawer
**Что:** `<div onClick={(e) => e.stopPropagation()} role="presentation">` вокруг SocialLink стопит bubble. Keyboard-юзер на cell не откроет строку через Enter.
**Почему отложено:** pre-existing pattern в `CreatorsListPage`. E2E тесты обходят, кликая `td:first-child`. Не критично для MVP админ-инструмента.
**Когда возвращаться:** при первом WCAG-аудите или жалобе админа на keyboard UX.

### `getCreator` detailQuery без `retry: false` (старые потребители)
**Что:** в `CreatorsListPage.tsx` detailQuery не имеет `retry: false` — на transient 5xx делает дефолтные 3 ретрая (~30 сек spinner).
**Почему отложено:** в `CampaignDetailPage` я уже добавил `retry: false` в detailQuery; в `CreatorsListPage` патч из-вне scope (другой файл, другой PR).
**Когда возвращаться:** следующий PR, трогающий CreatorsListPage.

### `formatShortDate` зависит от browser timezone
**Что:** `new Date(iso).toLocaleDateString("ru", {day:"numeric", month:"short"})` рендерит дату в локальном TZ. Backend отдаёт UTC. В Алматы (UTC+5) дата `2026-05-07T20:00:00Z` отрендерится как «8 мая».
**Почему отложено:** pre-existing pattern. E2E пины `timezoneId: "UTC"`. Реальные admins в Казахстане видят свою TZ — в большинстве случаев это правильное поведение для UI «когда событие произошло локально».
**Когда возвращаться:** если бизнес попросит UTC display, или будет инцидент с расхождением в audit.

### E2E cleanup-стек обрывается на первом failed cleanup
**Что:** в `frontend/e2e/web/admin-campaign-creators-read.spec.ts` (и существующих spec'ах) `while(cleanupStack.length>0) { await withTimeout(fn(), 5000) }` — если один cleanup кинет, цикл оборвётся, остальные элементы останутся в БД.
**Почему отложено:** pre-existing pattern (`admin-campaign-detail.spec.ts` etc). Существующие тесты не страдают, потому что cleanup правильно упорядочен через FK. Изменение паттерна затрагивает 5+ файлов.
**Когда возвращаться:** если flaky cleanup начнёт оставлять рассыпанные данные между прогонами. Решение: try-catch вокруг `withTimeout` + `console.warn`.

### `rowKey={creator.creatorId}` collision risk при дубликатах в campaign_creators
**Что:** Table key — creator_id. Если backend race-condition обойдёт UNIQUE-constraint и вернёт 2 строки на (campaign, creator), React выкинет warning.
**Почему отложено:** UNIQUE-constraint `campaign_creators_campaign_creator_unique` (см. миграцию `20260507044135_campaign_creators.sql`) гарантирует, что 2 строки невозможны. Repo race-handling трансформирует 23505 в `ErrCreatorAlreadyInCampaign`.
**Когда возвращаться:** если такая race всё-таки появится в проде (мониторинг pgErr 23505 в audit).

### `if (error || !data)` в API client throws ApiError со status=200 при пустом 200 OK
**Что:** в `campaignCreators.ts`: `if (error || !data) { throw new ApiError(response.status, extractErrorCode(error)) }` — если backend вернёт 200 без data, будет `ApiError(200, "INTERNAL_ERROR")` — нелогичный status.
**Почему отложено:** паттерн идентичен `campaigns.ts`, `creators.ts`. Backend никогда не возвращает 200 без data (контракт OpenAPI). Защита от phantom-edge case.
**Когда возвращаться:** если когда-то введётся 204 No Content на read-эндпоинте — нужна осмысленная обработка.

---

## (older entries below; trimmed)
