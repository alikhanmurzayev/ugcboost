---
title: 'Фронт-страница списка креаторов (chunk 2 campaign-roadmap)'
type: 'feature'
created: '2026-05-05'
status: 'done'
baseline_commit: 'a20ee01aead8e9b289d3024e08ad0e0d8c23e8e2'
context:
  - '_bmad-output/implementation-artifacts/spec-creators-list-endpoint.md'
  - '_bmad-output/planning-artifacts/campaign-roadmap.md'
  - 'docs/standards/'
---

<frozen-after-approval reason="human-owned intent — do not modify unless human renegotiates">

## Intent

**Problem:** Заглушка `creatorApplications/stubs/CreatorsPage.tsx` блокирует выбор approved-креаторов в кампанию (chunks 10/11 campaign-roadmap'а).

**Approach:** Полноценная страница `features/creators/CreatorsListPage.tsx` под `RoleGuard(ADMIN)` 1:1 паттерн `features/creatorApplications/ModerationPage.tsx`. Серверный список через `POST /creators/list` (chunk 1, типы в `schema.ts`); drawer-detail параллельным `useQuery` на `GET /creators/{id}` (`enabled: !!selectedId`) — каркас 1:1 `ApplicationDrawer.tsx` без footer-actions, verification-progress, `VerifyManualDialog`, bot-message-copy fallback (TG привязан по инварианту, `spec-creators-list-endpoint.md`). Generic-визуальные компоненты переезжают в `shared/components/` тем же PR'ом (`frontend-components.md` § «общее — в shared»).

Дополнительно: `frontend/web/playwright.config.ts` — `workers: 2 → 4` (staging-прогон долгий; новый `admin-creators-list.spec.ts` дополнительно нагружает e2e job). Тесты обязаны выдерживать 4 параллельных worker'а — изоляция через generated emails/UUID-suffix уже на месте, проверяется зелёным CI job.

### URL-state

`/creators?q=&dateFrom=&dateTo=&cities=&ageFrom=&ageTo=&categories=&sort=&order=&page=&id=` — 1:1 паттерн `features/creatorApplications/{filters,sort}.ts`, без `telegramLinked` и `statuses`. Default UI-sort `{full_name, asc}` + `page=1` опускаются. CSV для multi-select; ISO date для dateFrom/dateTo (`toListInput` достраивает `T00:00:00.000Z`/`T23:59:59.999Z`). При смене filter/sort `page` сбрасывается.

### Колонки таблицы (7)

`index | fullName (last+first) | socials (SocialLink list) | categories (CategoryChips) | age (calcAge) | city.name | createdAt (DD MMM)`. Sortable: `fullName / age / city / createdAt` (sort-fields `full_name | birth_date | city_name | created_at`). PII (`iin / phone / middleName / telegramUsername`) — не как колонки; идут в drawer + copy-to-clipboard.

### Drawer (grid 2-col, по образцу `ApplicationBody`)

Timeline `approvedAt = createdAt` → birthDate+age → iin+copy → phone (`<a href="tel:">`)+copy → city.name → middleName? → address? → categories+`categoryOtherText` italic chip → socials read-only (`SocialLink` без verify) → telegram (`@username` / `firstName lastName` / `id userId`)+copy → sourceApplicationId (мутно, аудит-след). `address, telegramFirstName/LastName/UserId, categoryOtherText, sourceApplicationId` — только из `GET /creators/{id}`; pre-fill row пока летит detail.

## Boundaries & Constraints

**Always:**
- Все `docs/standards/` грузятся целиком и применяются как hard rules (state, components, types, api, testing-unit/e2e, naming, security, quality)
- AuthZ — `RoleGuard(ADMIN)` уже на роуте `ROUTES.CREATORS`; фронт-гард — UX-слой, серверная авторизация в chunk 1
- i18n namespace — `creators`
- Default UI-sort `{full_name, asc}` + `page=1` опускаются из URL

**Ask First:**
- Изменение item-shape (полей таблицы / drawer'а)
- Расширение/сужение filter-set или sort-fields
- Отступление от паттерна `ModerationPage.tsx`

**Never:**
- Бэк-изменения (API-контракт, миграции, codegen) — chunk 1 отдельно
- Action-кнопки и mutate-логика в drawer (read-only — список approved)
- LiveDune-блоки (`rating, completedOrders, activeOrders`), `signedAt`, `gender`, cursor-pagination, `telegramLinked` фильтр/индикатор
- Cross-feature импорт generic'ов вместо переезда в shared

## I/O & Edge-Case Matrix

| Сценарий | Состояние | Ожидаемое |
|---|---|---|
| Не админ | brand-manager залогинен | `RoleGuard` редирект, страница не открывается |
| Empty | `items=[]` | `creators.empty` без фильтров; `creators.emptyFiltered` с активным фильтром |
| API error | listQuery error | `<ErrorState>` с retry |
| Sort-toggle | клик колонки на default `{full_name, asc}` | URL `?sort=full_name&order=desc`; повторный клик — URL без sort/order |
| Drawer open | клик строки | URL `?id=`, parallel useQuery `GET /creators/{id}`, pre-fill из row до resolve |
| Drawer detail error | `getCreator(id)` 4xx/5xx | drawer error-state, list работает |
| Drawer nav | Escape/Arrow | close или prev/next в текущей странице (на границах disabled) |

</frozen-after-approval>

## Code Map

**Новое в `features/creators/`:** `CreatorsListPage.tsx` (оркестратор), `CreatorFilters.tsx` (popover без `telegramLinked`), `CreatorDrawer.tsx` + `CreatorDrawerBody.tsx`, `sort.ts` (под `CreatorListSortField`, default `{full_name, asc}`), `filters.ts` (`FilterValues` + `parseFilters`/`writeFilters`/`toListInput`/`isFilterActive`/`clearFilters`/`countActive`), `types.ts` (re-export).

**API + i18n:** `frontend/web/src/api/creators.ts` — `listCreators` + `getCreator` (по образцу `api/creatorApplications.ts`). `shared/constants/queryKeys.ts` — `creatorKeys = { all, list(params), detail(id) }`. `shared/i18n/locales/ru/creators.json` + регистрация в `shared/i18n/config.ts`.

**Перенос в `shared/components/`:** `Table.tsx` (бывш. `ApplicationsTable`, default-export `Table<T>`), `SearchableMultiselect.tsx`, `DateRangePicker.tsx`, `SocialLink.tsx`, `CategoryChip.tsx` (default + named `CategoryChips`). `shared/utils/age.ts` — `calcAge(birthDate)`. Все call-sites в `features/creatorApplications/` обновить импорты.

**Подключение:** `frontend/web/src/App.tsx` — импорт стаба → `@/features/creators/CreatorsListPage`; удалить `features/creatorApplications/stubs/CreatorsPage.tsx`.

**Конфиг e2e:** `frontend/web/playwright.config.ts` — bump `workers: 2 → 4` + обновить шапочный комментарий («Four workers everywhere — staging CI parallelism»). `frontend/landing/playwright.config.ts` не трогаем (другой scope).

**Тесты:** `features/creators/{CreatorsListPage,CreatorDrawer,CreatorFilters}.test.tsx` + `{filters,sort}.test.ts`; `api/creators.test.ts`; `frontend/e2e/web/admin-creators-list.spec.ts`; `frontend/e2e/helpers/api.ts` — `seedApprovedCreator` (composable: seed application → admin approve через `POST /creators/applications/{id}/approve` → register cleanup).

## Tasks & Acceptance

**Execution:**

- [x] `frontend/web/src/shared/components/{Table,SearchableMultiselect,DateRangePicker,SocialLink,CategoryChip}.tsx` -- перенос с переименованием `ApplicationsTable → Table<T>`
- [x] `frontend/web/src/shared/utils/age.ts` -- `calcAge`
- [x] `frontend/web/src/features/creatorApplications/**` -- импорты на новые пути
- [x] `frontend/web/src/shared/constants/queryKeys.ts` -- `creatorKeys`
- [x] `frontend/web/src/api/creators.ts` (+ `creators.test.ts`)
- [x] `frontend/web/src/shared/i18n/locales/ru/creators.json` + регистрация namespace
- [x] `frontend/web/src/features/creators/{types,sort,filters}.ts` (+ `sort.test.ts`, `filters.test.ts`)
- [x] `frontend/web/src/features/creators/{CreatorFilters,CreatorDrawer,CreatorDrawerBody,CreatorsListPage}.tsx` (+ `*.test.tsx`)
- [x] `frontend/web/src/App.tsx` -- импорт + удалить стаб
- [x] `frontend/e2e/helpers/api.ts` -- `seedApprovedCreator`
- [x] `frontend/e2e/web/admin-creators-list.spec.ts` -- playwright spec, header JSDoc на русском narrative-стиль
- [x] `frontend/web/playwright.config.ts` -- `workers: 2 → 4`, обновить шапочный комментарий

**Acceptance Criteria:**

- Given admin + 3+ approved-креатора, when открывает `/creators`, then таблица 7 колонок, default sort `full_name asc`, total в заголовке, чистый URL без query-string.
- Given клик строки, when drawer открыт, then URL `?id=`, pre-fill из row, после resolve `GET /creators/{id}` добавляются `address, telegramFirstName/LastName/UserId, categoryOtherText, sourceApplicationId`.
- Given drawer открыт, when ArrowLeft/Right или Escape, then prev/next в странице (на границах disabled) или close (URL `?id=` удаляется).
- Given смена filter/sort, when `page > 1`, then `page` сбрасывается в URL.
- Given Vitest coverage gate, when `make test-unit-web`, then 0 файлов <80% (исключение `types.ts`).
- Given Playwright, when `make test-e2e-frontend`, then `admin-creators-list.spec.ts` зелёный.
- Given `frontend/web/playwright.config.ts` обновлён до `workers: 4`, when CI запускает `test-e2e-frontend` job, then job использует 4 параллельных worker'а и весь suite зелёный (изоляция через generated emails/UUID не ломается).

## Spec Change Log

- **2026-05-06 — human-owner renegotiation.** Добавлено: bump `frontend/web/playwright.config.ts` workers `2 → 4` (staging e2e job долгий; новый spec в этом chunk'е дополнительно нагрузит). Триггер: запрос Alikhan'а после approve. Изменены: Intent (Approach +1 абзац), Code Map (новый файл), Tasks +1, AC +1. KEEP: фичевая часть (страница, drawer, перенос generic'ов, остальные тесты) без изменений. Ландинг-конфиг (`frontend/landing/playwright.config.ts`) не трогаем — другой scope.

## Verification

**Commands:**
- `make build-web` — успешно
- `make lint-web` — 0 ошибок
- `make test-unit-web` — green, coverage ≥80% per-file
- `make test-e2e-frontend` — green с 4 worker'ами; `admin-creators-list.spec.ts` среди прогнанных
- `grep -n "workers:" frontend/web/playwright.config.ts` — `workers: 4`

**Manual checks:**
- `git grep -nE "ApplicationsTable|features/creatorApplications/components/(SearchableMultiselect|DateRangePicker|SocialLink|CategoryChip)" frontend/web/src/` — пусто
- `git grep -nE "stubs/CreatorsPage" frontend/web/src/` — пусто
- `make start-web` + ручной admin тест `http://localhost:3001/creators` — фильтр/sort/pagination/drawer работают; copy-to-clipboard на `iin/phone/@telegramUsername`

## Suggested Review Order

**API contract**

- Точка входа: тонкие обёртки над POST `/creators/list` и GET `/creators/{id}`, типы из generated schema.
  [`api/creators.ts:15`](../../frontend/web/src/api/creators.ts#L15)

- Query keys factory с иерархией all/list/detail для целевой инвалидации.
  [`queryKeys.ts:18`](../../frontend/web/src/shared/constants/queryKeys.ts#L18)

**URL ↔ state roundtrip**

- FilterValues + parse/write/toListInput — чистый URL без telegramLinked.
  [`filters.ts:25`](../../frontend/web/src/features/creators/filters.ts#L25)

- SortState под `CreatorListSortField`, default `{full_name, asc}` опускается из URL.
  [`sort.ts:11`](../../frontend/web/src/features/creators/sort.ts#L11)

- parsePage clamp до MAX_PAGE — review-patch против unbounded `?page=1e10`.
  [`CreatorsListPage.tsx:294`](../../frontend/web/src/features/creators/CreatorsListPage.tsx#L294)

**Оркестрация страницы**

- Page собирает list + drawer через два параллельных useQuery, default-sort и URL-sync.
  [`CreatorsListPage.tsx:28`](../../frontend/web/src/features/creators/CreatorsListPage.tsx#L28)

- CreatorDrawer: focus on mount split от keydown listener, ArrowLeft/Right/Escape без побочных deps.
  [`CreatorDrawer.tsx:39`](../../frontend/web/src/features/creators/CreatorDrawer.tsx#L39)

- DrawerBody: pre-fill из row + detail-only поля поверх; nullable-fields берутся из detail когда оно резолвится.
  [`CreatorDrawerBody.tsx:21`](../../frontend/web/src/features/creators/CreatorDrawerBody.tsx#L21)

**Filters popover**

- CreatorFilters без telegramLinked, page=1 сбрасывается на любую смену.
  [`CreatorFilters.tsx:21`](../../frontend/web/src/features/creators/CreatorFilters.tsx#L21)

**Перенос generic-компонентов**

- `Table<T>` с дефолтным `testid="data-table"`, opt-in `applications-table` для legacy call-sites.
  [`Table.tsx:25`](../../frontend/web/src/shared/components/Table.tsx#L25)

- SocialLink без констант — split ради react-refresh; PLATFORM_LABELS / SocialPlatform отдельным модулем.
  [`socials.ts:1`](../../frontend/web/src/shared/constants/socials.ts#L1)

**i18n + utils**

- Namespace `creators` зарегистрирован.
  [`config.ts:9`](../../frontend/web/src/shared/i18n/config.ts#L9)

- `calcAge` вынесен в shared/utils для переиспользования между creators и creatorApplications.
  [`age.ts:1`](../../frontend/web/src/shared/utils/age.ts#L1)

**Тесты и e2e**

- e2e composable helper с try/catch на partial-failure (review-patch) и cleanup try/finally.
  [`api.ts:566`](../../frontend/e2e/helpers/api.ts#L566)

- Browser e2e — 5 сценариев под 4 workers; header JSDoc на русском в нарративе.
  [`admin-creators-list.spec.ts:1`](../../frontend/e2e/web/admin-creators-list.spec.ts#L1)

- Workers 2 → 4 (CI parallelism alongside нового spec).
  [`playwright.config.ts:9`](../../frontend/web/playwright.config.ts#L9)

**App wiring**

- Маршрут `/creators` подключён, stub удалён.
  [`App.tsx:14`](../../frontend/web/src/App.tsx#L14)

