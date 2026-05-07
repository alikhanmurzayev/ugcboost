# Deferred Work

Findings, surfaced by reviews/work, that we consciously kicked down the road. Each entry: what, why deferred, where it surfaced.

---

## 2026-05-07 — chunk 11 slice 1/2 review (campaign-creators-frontend-read)

### `useCampaignCreators`: hard-cap 200 без chunking
**Что:** `listCreators({ids: creatorIds, perPage: 200, ...})` не chunkает `creatorIds`. Если кампания накопит >200 креаторов, listCreators отдаст 422.
**Почему отложено:** spec явно говорит «`perPage=200` покрывает chunk-creator-cap из chunk 10». Backend chunk 10 ограничивает добавление 200 креаторами на кампанию суммарно. Слайс 2/2 (Add drawer) добавит UI hard-cap 200. До тех пор upper bound гарантирован контрактом.
**Когда возвращаться:** если бизнес попросит снять кэп креаторов на кампанию.

### Row ordering: creator.created_at vs campaign_creator.created_at
**Что:** строки сортируются по `creator.created_at desc` (порядок listCreators), а не по `campaign_creator.created_at` (порядок добавления в кампанию). Креатор, добавленный сегодня, но created_at год назад, попадёт в конец.
**Почему отложено:** spec буквально фиксирует `sort: "created_at"` для listCreators — текущая реализация не отклоняется. Spec gap: какой порядок ожидает PM/UX. Нужно согласовать.
**Когда возвращаться:** после first user feedback на staging — Aidana подскажет, какой порядок ей удобнее. Если cc.created_at desc — добавить sort на стороне фронта (или в backend listCampaignCreators).

### Хардкод символа `№` в заголовках колонок (table)
**Что:** `header: "№"` в `CreatorsListPage.buildColumns` и в `CampaignCreatorsTable.buildColumns` — литерал в JSX без `t(...)`.
**Почему отложено:** паттерн pre-existing в `CreatorsListPage.tsx:229`. Символ `№` — language-neutral для русскоязычного MVP. Локализация сейчас single-language (ru), нет английских ассетов. Когда будет — фиксим везде вместе.
**Когда возвращаться:** при добавлении второй локали (EN/KZ).

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
