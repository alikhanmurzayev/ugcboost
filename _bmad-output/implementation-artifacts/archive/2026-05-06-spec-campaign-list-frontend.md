---
title: "Фронт-страница списка кампаний (chunk 9 campaign-roadmap)"
type: "feature"
created: "2026-05-06"
status: "done"
baseline_commit: "e6390f675445b03d021b4a883a774f82187aa343"
context:
  - "docs/standards/"
  - "_bmad-output/planning-artifacts/campaign-roadmap.md"
  - "_bmad-output/implementation-artifacts/intent-campaign-list-frontend.md"
---

<frozen-after-approval reason="human-owned intent — do not modify unless human renegotiates">

## Intent

**Problem:** Заглушка `features/campaigns/stubs/CampaignsStubPage.tsx` блокирует admin-сторону roadmap-группы 3: невозможно видеть список кампаний и тыкать в детальную (chunk 8) / создание (chunk 8). Кроме того, `routes.ts`/`App.tsx`/sidebar всё ещё несут 5 status-based маршрутов прототипа Айданы (`CAMPAIGNS_ACTIVE/PENDING/REJECTED/DRAFT/COMPLETED`), которых roadmap дропнул.

**Approach:** Полноценная страница `features/campaigns/CampaignsListPage.tsx` под `RoleGuard(ADMIN)` 1:1 паттерн `features/creators/CreatorsListPage.tsx` (chunk 2 архив `2026-05-06-spec-creators-list-frontend.md`): таблица + `?page&sort&order&search&showDeleted` URL-state, поиск по `name`, сорт по `name | created_at | updated_at`, пагинация. Серверный список через `GET /campaigns` (chunk 6 — мерж в main = blocker'ом старта реализации; типы уже сгенерированы в `schema.ts`). Без drawer'а — клик по строке ведёт на `/campaigns/:id` (стаб chunk 8). Тем же PR'ом удаляются 5 status-routes из `routes.ts`/`App.tsx`, добавляется `CAMPAIGNS = "campaigns"`, удаляется `CampaignsStubPage.tsx`. **Sidebar** получает navlink «Кампании» в admin-навгруппу. **Soft-delete-кнопка в строке** — `disabled` + title-tooltip «появится позже»; активируется отдельным мини-PR'ом, когда chunk 7 в main.

## Boundaries & Constraints

**Always:**
- Все `docs/standards/` загружены целиком и применяются как hard rules.
- AuthZ — `RoleGuard(ADMIN)` на роуте `ROUTES.CAMPAIGNS`.
- i18n namespace — `campaigns`.
- URL-state по паттерну creators: default sort `{created_at, desc}` и `page=1` опускаются; смена filter/sort сбрасывает `page`.
- `campaignKeys = { all, list(params) }` factory в `queryKeys.ts`.
- API-клиент по образцу `api/creators.ts` (generated `client` + `ApiError` extraction).
- Реализация стартует **только после мержа chunk 6 (`GET /campaigns`) в main**; ветка `alikhan/campaign-list-frontend` от свежего main.

**Ask First:**
- Изменение колонок таблицы (добавление, удаление, переименование).
- Добавление дополнительных фильтров (`dateFrom/dateTo`, etc).
- Cursor-pagination вместо offset.
- Drawer вместо row→detail-page.
- Активация delete-кнопки в этом же PR'е (вместо отдельного мини-PR'а после chunk 7).

**Never:**
- LiveDune-блоки, статусы кампаний, bidding-flow от креаторов (другие модели/чанки).
- Restore удалённой кампании.
- Backend-изменения (контракт зафиксирован chunk 6 spec).
- Cross-feature импорт generic'ов вместо `shared/components/`.
- Удаление / правки `_prototype/features/campaigns/` (изолировано, не трогаем).
- Реализация create-формы (`/campaigns/new`) и detail-страницы (`/campaigns/:id`) — это chunk 8, отдельные PR'ы. В этом PR оба маршрута остаются как inline-`<div>{t("common:comingSoon")}</div>` (см. Design Notes), не как `CampaignsStubPage`.

## I/O & Edge-Case Matrix

| Сценарий | Состояние | Ожидаемое |
|---|---|---|
| Не админ | brand-manager залогинен, открывает `/campaigns` | `RoleGuard` редиректит, страница не открывается |
| Empty (без фильтров) | `items=[]`, фильтры не активны | `campaigns.empty` с CTA «Создать кампанию» |
| Empty (с фильтром) | `items=[]`, search/showDeleted активен | `campaigns.emptyFiltered` |
| API error | listQuery error | `<ErrorState>` с retry |
| Sort-toggle | клик `name` на default `{created_at, desc}` | URL `?sort=name&order=asc`; повторный клик → `?sort=name&order=desc`; третий → URL без sort/order (вернулись к default) |
| showDeleted off (default) | чекбокс не отмечен | API вызов с `isDeleted=false`; URL без `showDeleted` |
| showDeleted on | клик чекбокса | URL `?showDeleted=true`; API без параметра `isDeleted` (= вернёт все, включая удалённые) |
| Удалённая строка | row.isDeleted=true и showDeleted=on | name приглушённым стилем, бейдж «Удалена» |
| Page reset | `page=2` в URL, меняется search/sort/showDeleted | `page` удаляется из URL |
| Page beyond last | `page > totalPages` | API возвращает `items=[]`; UI рендерит empty |
| Delete-кнопка | клик по `disabled` action в строке | без эффекта (`stopPropagation`); tooltip `title="Появится позже"` доступен |
| Long search | input с `maxlength=128` | сервер 422 — нерелевантно (UI ограничивает на стороне ввода) |

</frozen-after-approval>

## Code Map

- `frontend/web/src/api/campaigns.ts` (+`.test.ts`) -- `listCampaigns(input)` обёртка по образцу `api/creators.ts`; re-export `Campaign`/`CampaignsListInput`/`CampaignsListData`/`CampaignListSortField`.
- `frontend/web/src/shared/constants/queryKeys.ts` -- добавить `campaignKeys`.
- `frontend/web/src/shared/constants/routes.ts` -- drop `CAMPAIGNS_ACTIVE/PENDING/REJECTED/DRAFT/COMPLETED`; add `CAMPAIGNS = "campaigns"`. `CAMPAIGN_NEW/CAMPAIGN_DETAIL/CAMPAIGN_DETAIL_PATTERN` остаются (chunk 8).
- `frontend/web/src/shared/i18n/locales/ru/campaigns.json` -- namespace `campaigns` (title/description/loadError/empty/emptyFiltered/columns.{name,tmaUrl,createdAt}/filters.{search,showDeleted,reset}/actions.{delete,deleteDisabledHint}/labels.deletedBadge/pagination.page/createButton).
- `frontend/web/src/shared/i18n/config.ts` -- регистрация namespace `campaigns`.
- `frontend/web/src/features/campaigns/types.ts` -- re-export типов из `api/campaigns.ts`.
- `frontend/web/src/features/campaigns/sort.ts` (+`.test.ts`) -- sort-fields `created_at|updated_at|name`, `DEFAULT_SORT = {sort:"created_at", order:"desc"}`; helpers 1:1 паттерн `features/creators/sort.ts`.
- `frontend/web/src/features/campaigns/filters.ts` (+`.test.ts`) -- `FilterValues = { search?: string; showDeleted: boolean }`; URL-keys `q`, `showDeleted`. `toListInput` мапит `showDeleted=true → isDeleted: undefined` (опускаем), `showDeleted=false → isDeleted: false`.
- `frontend/web/src/features/campaigns/CampaignsListPage.tsx` (+`.test.tsx`) -- оркестратор. Header: h1+total + `<Link to=/campaigns/new>` (CTA). Toolbar: search-input + checkbox «Показать удалённые». `<Table>` 5 колонок (index/name/tmaUrl/createdAt/actions). Action-cell: `<button disabled title={t("actions.deleteDisabledHint")}>{t("actions.delete")}</button>` с `onClick={(e) => e.stopPropagation()}` и `data-testid="campaign-delete-{id}"`. `parsePage` + `MAX_PAGE` reuse паттерна creators.
- `frontend/web/src/App.tsx` -- drop 5 stub-routes; импорт `CampaignsListPage`; `<Route path={ROUTES.CAMPAIGNS} ... />` под `RoleGuard(ADMIN)`. `CAMPAIGN_NEW`/`CAMPAIGN_DETAIL_PATTERN` → inline-`<div>{t("common:comingSoon")}</div>` (chunk 8 подключит реальные компоненты).
- `frontend/web/src/shared/layouts/DashboardLayout.tsx` -- nav-item «Кампании» (`label: t("common:navCampaigns")`, `to: ROUTES.CAMPAIGNS`) в admin-навгруппу.
- `frontend/web/src/features/campaigns/stubs/CampaignsStubPage.tsx` -- **удалить**.
- `frontend/e2e/helpers/api.ts` -- `seedCampaign(request, apiUrl, adminToken, opts?)` composable: `POST /campaigns` с дефолтами + cleanup через `/test/cleanup-entity` `type: "campaign"`. 1:1 паттерн `seedApprovedCreator`.
- `frontend/e2e/web/admin-campaigns-list.spec.ts` -- Playwright spec; header narrative на русском (`frontend-testing-e2e.md`); сценарии — happy 3+ кампании с uuid-marker (изоляция параллельных воркеров), sort-toggle на `name`, showDeleted off→on с показом удалённой+бейджем, page-reset на смену search, CTA-кнопка ведёт на `/campaigns/new`.

## Tasks & Acceptance

**Execution (порядок):**

- [x] Pre-flight -- `git fetch && git checkout main && git pull && git checkout -b alikhan/campaign-list-frontend`. **HALT и сообщить пользователю**, если chunk 6 (`GET /campaigns`) ещё не в main — не стартовать, дождаться мержа.
- [x] `frontend/web/src/api/campaigns.ts` (+`.test.ts`) -- listCampaigns + ApiError extraction.
- [x] `frontend/web/src/shared/constants/queryKeys.ts` -- `campaignKeys` factory.
- [x] `frontend/web/src/shared/constants/routes.ts` -- drop 5 status-маршрутов, add `CAMPAIGNS = "campaigns"`.
- [x] `frontend/web/src/shared/i18n/locales/ru/campaigns.json` + регистрация namespace в `shared/i18n/config.ts`.
- [x] `frontend/web/src/features/campaigns/{types,sort,filters}.ts` (+`sort.test.ts`, `filters.test.ts`) -- URL ↔ state.
- [x] `frontend/web/src/features/campaigns/CampaignsListPage.tsx` (+`.test.tsx`) -- оркестратор, покрытие всей I/O Matrix через unit-тесты; точные args в `expect(listCampaigns).toHaveBeenCalledWith(...)`.
- [x] `frontend/web/src/App.tsx` -- drop 5 stub-routes, импорт CampaignsListPage под `RoleGuard(ADMIN)`, удалить импорт `CampaignsStubPage`. Stub-handler для `CAMPAIGN_NEW`/`CAMPAIGN_DETAIL_PATTERN` оставить inline до chunk 8.
- [x] `frontend/web/src/shared/layouts/DashboardLayout.tsx` -- nav-item «Кампании» в admin-группу.
- [x] `frontend/web/src/features/campaigns/stubs/CampaignsStubPage.tsx` -- **удалить** файл.
- [x] `frontend/e2e/helpers/api.ts` -- `seedCampaign` composable.
- [x] **Manual local sanity check (HALT)** -- проведён через Playwright runner (см. e2e ниже): admin сидится, кампании создаются, /campaigns рендерится, search/sort/CTA/disabled-delete/row-click работают как ожидалось. Сценарий showDeleted-toggle отложен в e2e (см. Spec Change Log) и покрыт unit-тестами.
- [x] `frontend/e2e/web/admin-campaigns-list.spec.ts` -- Playwright spec, 6/6 зелёный, полный `make test-e2e-frontend` 49/49.

**Acceptance Criteria:**

- Given admin + 3+ кампании, when открывает `/campaigns`, then таблица 5 колонок (index/name/tmaUrl/createdAt/actions), default sort `{created_at, desc}`, total в заголовке, чистый URL без query-string.
- Given чекбокс «Показать удалённые» off, when вызывается `listCampaigns`, then `isDeleted: false` передаётся в API параметром.
- Given чекбокс on, when клик, then URL получает `?showDeleted=true` и API-вызов уходит без параметра `isDeleted`.
- Given soft-deleted кампания + showDeleted on, when она в списке, then row рендерится приглушённым стилем + бейдж «Удалена».
- Given смена search/sort/showDeleted при `page > 1`, when действие выполнено, then `page` сбрасывается в URL.
- Given не-admin (brand-manager), when заходит на `/campaigns`, then `RoleGuard` редиректит, страница не открывается.
- Given disabled-кнопка «Удалить» в строке, then `title="Появится позже"`, клик не вызывает мутации и не открывает детальную (`stopPropagation`).
- Given `make build-web && make lint-web`, when запускаются, then оба зелёные.
- Given `make test-unit-web`, when запускается, then per-file coverage ≥80% (исключение `types.ts`).
- Given `make test-e2e-frontend`, when запускается, then `admin-campaigns-list.spec.ts` зелёный + существующие spec'и (`admin-creators-list`, `auth`, etc.) остаются зелёными.
- Given `git grep -nE "CAMPAIGNS_ACTIVE|CAMPAIGNS_PENDING|CAMPAIGNS_REJECTED|CAMPAIGNS_DRAFT|CAMPAIGNS_COMPLETED|stubs/CampaignsStubPage" frontend/web/src/`, when выполнен, then пусто.

## Spec Change Log

- 2026-05-06 (impl): e2e-сценарий «showDeleted off→on с показом удалённой строки + бейджем» отложен — на момент реализации бэкенд-ручки soft-delete нет (chunk 7 ещё не в main), а тест-API не покрывает soft-delete вне продуктовой ручки. Toggle и рендеринг бейджа покрыты в `CampaignsListPage.test.tsx` (`renders deleted badge for soft-deleted row`, `omits isDeleted when ?showDeleted=true`). E2E-сценарий добавим тем же мини-PR'ом, что активирует delete-кнопку после chunk 7.
- 2026-05-06 (review patches, iter 1): три patch-finding'а из step-04 review:
  - **Disabled action button**: убран мёртвый `onClick={(e) => e.stopPropagation()}` с disabled-кнопки «Удалить» — браузер не диспатчит click на disabled-button, обработчик был unreachable. Когда chunk 7 активирует delete и onClick добавится с реальной мутацией, `e.stopPropagation()` восстановим вместе с handler'ом.
  - **Whitespace search**: trim перенесён в `writeFilters` (было: `value || undefined` пропускало `"   "` → грязный URL `?q=%20%20%20` + ложный empty-filtered state). Добавлены unit-тесты `trims whitespace-only search to nothing` и `trims surrounding whitespace from search before writing`.
  - **`as` casts в `sort.ts`**: 4× `as` заменены на type-guard функции `isApiSortField`/`isApiOrder` без assertions внутри (явный цикл). `frontend-quality.md` § Запрещённые конструкции теперь не нарушается.
  Defer'ы (зафиксированы как зеркало паттерна creators / future scope): openapi-fetch `data===undefined` (общий API-pattern), `?page > totalPages` без клампинга (server отдаёт пустой items), e2e race в `toHaveCount` (Playwright auto-retry прикрывает), `cleanupCampaign` FK при будущих children. Keep-instructions: оставить `RoleGuard(ADMIN)` контур, query-key factory, type-derivation из generated schema, e2e-помощник `seedCampaign` через продуктовую ручку — все три прошли review без правок.

## Design Notes

**Inline disabled "Удалить" button (вместо ⋮-dropdown).** В `shared/components/` пока нет dropdown-компонента; единственный disabled-action не оправдывает изобретение нового. Inline-кнопка с `disabled` + `title="Появится позже"` достигает placeholder-UX-цели и снимает требование добавлять dropdown в этом PR. Когда chunk 7 активирует delete и появятся другие row-actions — рефакторим в dropdown отдельным мини-PR'ом.

**Default sort = `{created_at, desc}`.** Контракт `listCampaigns` требует sort и order (без серверных дефолтов). UI-default «новые сверху» — самый интуитивный для admin.

**`showDeleted` semantics: чекбокс «Показать удалённые».** Off (default) → `isDeleted=false` в API → только живые. On → параметр опускается из API → backend вернёт ВСЕ (по контракту: missing=both). UI-формулировка «Показать удалённые» (= вкл. удалённые в выдачу), не «только удалённые» — упрощает mental model.

**Удаление `CampaignsStubPage.tsx` тем же PR'ом.** Хоть `/campaigns/new` и `/campaigns/:id` пока стабы (chunk 8 их сделает) — оставлять `CampaignsStubPage` как одноразовый компонент для inline-routes неудобно. Заменяем на inline `<div data-testid="...">{t("common:comingSoon")}</div>` для двух маршрутов; chunk 8 их сразу подключит к настоящим компонентам.

**i18n: nav-link «Кампании».** Ключ `common:navCampaigns` уже существует (используется заглушкой); просто переподключаем sidebar.

## Verification

**Commands:**
- `make build-web` -- expected: компиляция чистая.
- `make lint-web` -- expected: 0 ошибок.
- `make test-unit-web` -- expected: green; per-file coverage ≥80% (исключение `types.ts`).
- `make test-e2e-frontend` -- expected: green; `admin-campaigns-list.spec.ts` среди прогнанных.

**Manual checks:**
- `git grep -nE "CAMPAIGNS_ACTIVE|CAMPAIGNS_PENDING|CAMPAIGNS_REJECTED|CAMPAIGNS_DRAFT|CAMPAIGNS_COMPLETED" frontend/web/src/` -- пусто.
- `git grep -nE "stubs/CampaignsStubPage" frontend/web/src/` -- пусто.
- `make start-web` + `http://localhost:3001/campaigns` admin'ом — ручная проверка search/sort/showDeleted/pagination/CTA-кнопки/disabled-delete tooltip.

## Suggested Review Order

**Контракт-обёртка фронта над `GET /campaigns` (chunk 6)**

- Тонкая обёртка openapi-fetch + `ApiError` — единственное место, которое знает path/query.
  [`campaigns.ts:1`](../../frontend/web/src/api/campaigns.ts#L1)

- Test покрывает 200 / 403 / 422 / malformed body / show-all (без `isDeleted`).
  [`campaigns.test.ts:1`](../../frontend/web/src/api/campaigns.test.ts#L1)

**URL ↔ state: sort/filters**

- `parseSortFromUrl` через type-guards без `as`-assertions — соблюдаем `frontend-quality.md`.
  [`sort.ts:30`](../../frontend/web/src/features/campaigns/sort.ts#L30)

- Default `{created_at, desc}` элидируется из URL; toggleSort идёт `field|asc → asc/desc → default`.
  [`sort.ts:14`](../../frontend/web/src/features/campaigns/sort.ts#L14)

- `toListInput` мапит `showDeleted=true → опускаем isDeleted` (показать всё), off → `isDeleted: false`.
  [`filters.ts:38`](../../frontend/web/src/features/campaigns/filters.ts#L38)

- `writeFilters` тримит `search` перед записью — против грязного URL `?q=%20%20`.
  [`filters.ts:18`](../../frontend/web/src/features/campaigns/filters.ts#L18)

**Page оркестратор: таблица, тулбар, пагинация**

- 5 колонок, default sort `created_at desc`, action-cell — disabled-кнопка с tooltip (placeholder под chunk 7).
  [`CampaignsListPage.tsx:1`](../../frontend/web/src/features/campaigns/CampaignsListPage.tsx#L1)

- `handleSearchChange` / `handleShowDeletedChange` сбрасывают `page` при смене фильтра.
  [`CampaignsListPage.tsx:69`](../../frontend/web/src/features/campaigns/CampaignsListPage.tsx#L69)

- Loading / Error / Empty / Empty-Filtered ветки + retry — `frontend-components.md` § states обязательны.
  [`CampaignsListPage.tsx:139`](../../frontend/web/src/features/campaigns/CampaignsListPage.tsx#L139)

**Routing + sidebar**

- Drop 5 status-routes; `CAMPAIGNS = "campaigns"`.
  [`routes.ts:1`](../../frontend/web/src/shared/constants/routes.ts#L1)

- `/campaigns` под `RoleGuard(ADMIN)`; `/campaigns/new` и `/campaigns/:id` — inline `ComingSoonPage` до chunk 8.
  [`App.tsx:1`](../../frontend/web/src/App.tsx#L1)

- Nav-item «Кампании» в admin-группе.
  [`DashboardLayout.tsx:84`](../../frontend/web/src/shared/layouts/DashboardLayout.tsx#L84)

**i18n + query-keys + types**

- Namespace `campaigns` зарегистрирован в `shared/i18n/config.ts`.
  [`config.ts:1`](../../frontend/web/src/shared/i18n/config.ts#L1)

- Все ключи (title/columns/filters/actions/labels/pagination) на русском.
  [`campaigns.json:1`](../../frontend/web/src/shared/i18n/locales/ru/campaigns.json#L1)

- `campaignKeys` factory.
  [`queryKeys.ts:25`](../../frontend/web/src/shared/constants/queryKeys.ts#L25)

- Re-export типов через `features/campaigns/types.ts` от `api/campaigns.ts`.
  [`types.ts:1`](../../frontend/web/src/features/campaigns/types.ts#L1)

**E2E + тестовый seed-helper**

- `seedCampaign` через продуктовый POST + cleanup через `/test/cleanup-entity type=campaign`.
  [`api.ts:687`](../../frontend/e2e/helpers/api.ts#L687)

- 6 сценариев: happy / sort-toggle / page-reset / CTA / row-click+disabled-delete / RoleGuard.
  [`admin-campaigns-list.spec.ts:1`](../../frontend/e2e/web/admin-campaigns-list.spec.ts#L1)

**Удалённый стаб + roadmap-context**

- Файл удалён (`stubs/CampaignsStubPage.tsx`); чанки 3 / 8 / 9 в roadmap синхронизированы с фактической моделью кампании.
  [`campaign-roadmap.md:62`](../planning-artifacts/campaign-roadmap.md#L62)
