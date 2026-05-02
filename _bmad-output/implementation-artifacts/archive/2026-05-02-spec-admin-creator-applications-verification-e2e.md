---
title: 'Browser e2e: admin verification flow'
type: feature
created: '2026-05-02'
status: 'done'
baseline_commit: 'fa17256'
context:
  - docs/standards/frontend-testing-e2e.md
  - docs/standards/naming.md
---

<frozen-after-approval reason="human-owned intent — do not modify unless human renegotiates">

## Intent

**Problem:** Chunk 6 (admin verification page, PR #49) уже в main без browser-e2e. Регресс в RoleGuard, sidebar-навигации, фильтре telegramLinked, search/empty или drawer prev/next уйдёт в прод незамеченным — unit-тесты не страхуют URL-state, role-маршруты и кросс-компонентный flow.

**Approach:** Один Playwright spec в `frontend/e2e/web/admin-creator-applications-verification.spec.ts` с 5 сценариями. Данные сидятся через API (`/creators/applications`, `/test/seed-user`, `/test/telegram/message`); UI задействуется ровно для проверяемого flow. Полная per-test изоляция.

## Boundaries & Constraints

**Always:**
- Per-test изоляция: `beforeEach` сидит свой набор, `afterEach` дренирует cleanup-стек LIFO, уважая `E2E_CLEANUP=false`.
- Локаторы только `data-testid`. Header — JSDoc на русском, нарративом (стандарт `frontend-testing-e2e.md`). Inline-комментарии — английский.
- Уникальность: `uniqueIIN()` для applicant; `crypto.randomUUID()`-префикс в `lastName` для search; admin/brand_manager email с `Date.now()`-префиксом.
- Точные seeded-значения в ассертах (assert на корректность данных, не «непусто/видно»).

**Ask First:**
- Любые правки бизнес-логики или продакшен-кода (`frontend/web/src/`, `backend/internal/`).
- Изменения схемы тестовых ручек или `openapi-test.yaml`.

**Never:**
- Дубль edge-кейсов из backend e2e: date-range, age-from/to, мульти-сити/категория, sort, pagination — они в `backend/e2e/creator_applications/list_verification_test.go`.
- Mock'и/MSW; hardcoded даты или IIN'ы; новые бэкенд-эндпоинты или продакшен-testid'ы.
- Проверка clipboard для `drawer-copy-bot-message` (нестабильно в headless).
- Auth-redirect для анонима — общий guard уже покрыт `auth.spec.ts`.

</frozen-after-approval>

## Code Map

- `frontend/e2e/web/admin-creator-applications-verification.spec.ts` — новый spec, 5 тестов в одном `test.describe`.
- `frontend/e2e/helpers/api.ts` — расширяется тремя группами хелперов (users, application seed, telegram link).
- `frontend/web/src/features/creatorApplications/components/ApplicationDrawer.tsx` — `drawer-prev/next/close` + Escape/← /→ keyboard nav уже на месте; без изменений.
- `frontend/web/src/features/creatorApplications/components/ApplicationFilters.tsx` — testid'ы `filter-telegram-linked-{any|true|false}` (ключи именно так, не `linked`/`not_linked`); без изменений.
- `frontend/web/src/shared/layouts/DashboardLayout.tsx` — nav-ссылки `nav-link-${routePath}`. Группа `creatorApplications` рендерится только под `isAdmin`.
- `frontend/web/src/features/auth/RoleGuard.tsx` — non-admin → `Navigate(ROUTES.DASHBOARD = "/")`.
- `frontend/web/src/shared/constants/routes.ts` — `CREATOR_APP_VERIFICATION = "creator-applications/verification"` (без `/admin`).
- `backend/api/openapi-test.yaml` — `seed-user`, `cleanup-entity`, `telegram/message` уже есть.

## Tasks & Acceptance

**Execution:**
- [x] `frontend/e2e/helpers/api.ts` — `seedAdmin(request, apiUrl)` и `seedBrandManager(request, apiUrl)`. Возвращают `{ email, password, userId, cleanup }`. Email с `Date.now()`+role-префиксом, password `"testpass123"`. `cleanup` → `POST /test/cleanup-entity {type:"user", id}` под guard `E2E_CLEANUP !== "false"`.
- [x] `frontend/e2e/helpers/api.ts` — `seedCreatorApplication(request, apiUrl, opts?)`. POST на `/creators/applications`. Дефолты: `lastName = e2e-${randomUUID}-Иванов`, `firstName = "Айдана"`, `middleName = "Тестовна"`, `phone = "+77001234567"`, `city = "almaty"`, `categories = ["beauty"]`, `socials = [{platform:"instagram", handle:"aidana_test_${uuid}"}]`, `acceptedAll = true`, `iin = uniqueIIN()`. Все поля перезаписываются через `opts`. Возвращает `{ applicationId, lastName, firstName, middleName, iin, phone, city, categories, categoryOtherText?, socials, birthDate, cleanup }`. `birthDate` парсится из IIN (digits 0..5 + century byte 6).
- [x] `frontend/e2e/helpers/api.ts` — `linkTelegramToApplication(request, apiUrl, applicationId, opts?)`. POST на `/test/telegram/message` с `text="/start ${applicationId}"`, `chatId/userId` через `uniqueTelegramUserId()` (epoch `1<<30` + module-level atomic counter + `Date.now()%(1<<20)*1024`). Возвращает `{ telegramUserId, username, firstName, lastName }`.
- [x] `frontend/e2e/web/admin-creator-applications-verification.spec.ts` — новый файл. Russian narrative JSDoc-header. Один `test.describe("Admin verification flow")` с 5 тестами по AC. Локальный `cleanupStack` per-test, drain в `afterEach`. UI-логин по образцу `auth.spec.ts`. Browser TZ закреплён `test.use({ timezoneId: "UTC" })`. Локальный `fetchDictionary` резолвит city/category names в момент прогона, чтобы тест не ломался при правках dict-копирайта.

**Acceptance Criteria:**

- **AC1 (Happy path).** Given admin + одна заявка с uuid-префиксом в `lastName`, валидным IIN, городом `almaty`, категориями `["beauty","other"]`+`categoryOtherText="Тест-ниша"`, соцсетями `instagram`+`tiktok`, без TG-link; when admin логинится, открывает `filters-toggle`, вводит uuid в `filters-search`, then `row-${applicationId}` единственная видна; click → `drawer` открыт, URL содержит `?id=${applicationId}`. Ассерты в drawer на equality seeded values: header full-name (`'${last} ${first} ${middle}'`); `application-timeline` содержит локализованную дату+время; дата рождения `dd.mm.yyyy · N {год|года|лет}` со склонением; IIN; `application-phone` — text + `href="tel:${phone}"`; city; chip на каждую категорию + chip с `categoryOtherText`; `social-instagram`+`social-tiktok` с handle; `drawer-telegram-not-linked` виден, `drawer-copy-bot-message` присутствует. Click `drawer-close` → drawer исчезает, URL без `id`.

- **AC2 (Drawer prev/next).** Given admin + 3 заявки с общим uuid-префиксом и разными `lastName` (последовательность POST'ов задаёт детерминированный `created_at desc`); when admin search'ит по uuid и кликает первую строку, then `drawer-prev` disabled, `drawer-next` enabled; next → header сменился на вторую; next → header третьей, `drawer-next` disabled; prev → возврат на вторую. В одном переходе используется `page.keyboard.press('ArrowRight')`, закрытие через `Escape`.

- **AC3 (Filter telegramLinked, все 3 ветки).** Given admin + 2 заявки с общим uuid-префиксом — A без TG, B с TG через `linkTelegramToApplication(B)`; when admin search'ит по uuid, then `filter-telegram-linked-any` (default) — обе строки; `filter-telegram-linked-true` — только `row-${B.id}`, A отсутствует; `filter-telegram-linked-false` — только A, B отсутствует; возврат на `any` — обе.

- **AC4 (Empty state).** Given admin (заявок не создаём); when admin вводит `crypto.randomUUID()` в `filters-search`, then `applications-table-empty` виден с empty-сообщением, `applications-table` отсутствует в DOM.

- **AC5 (RoleGuard).** Given seeded `brand_manager`; when он логинится, then `await expect(page.getByTestId('nav-link-creator-applications/verification')).toHaveCount(0)`; when `page.goto('/creator-applications/verification')`, then `await expect(page).toHaveURL('/')` и отрисован дашборд.

## Spec Change Log

- **2026-05-02 (impl)** — На прогоне выяснилось, что: (1) `socials` column в `ApplicationsTable.tsx` останавливает propagation, поэтому `row.click()` мог попасть на `<a>` соцссылки и не открыть drawer — клик переведён на `td:first-child` (index-ячейка). (2) Локализованный datetime в `application-timeline` рендерится как `«… г. в HH:MM»` (через "в", не запятую) — regex поправлен. (3) Имена городов/категорий в production dict (`Бьюти (макияж, уход)` etc.) могут отличаться от ожидаемого — добавлен локальный `fetchDictionary` с резолвом code → name из `/dictionaries/*` (публичный endpoint), assert'ы привязаны к live dict.
- **2026-05-02 (impl)** — Cleanup для seeded user'ов делается через `POST /test/cleanup-entity {type:"user", id}` (не `DELETE /test/users/:email` — такого эндпоинта на бэке нет; auth.spec.ts работает потому, что 404 от `request.delete` молча игнорируется).
- **2026-05-02 (review patches)** — После 3-агентного ревью применены три патча: (1) AC1 — добавлен click `filters-toggle` перед `filters-search.fill` и close-toggle после, чтобы popover не перекрывал клик по строке; close именно toggle'ом (не Escape — Chromium очищает `<input type="search">` на Escape). (2) AC5 — перед `nav-link.toHaveCount(0)` добавлен `expect(sidebar).toBeVisible()`, чтобы count(0) был привязан к фактическому рендеру сайдбара. (3) AC1 + AC2 — fullName собирается через `[last, first, middle].filter(Boolean).join(" ")` (повторяет `buildFullName` в drawer), защищает тест от middleName=null/empty.
- **2026-05-02 (PR #50 review fixes)** — После inline-комментариев Алихана + дополнительный аудит всех frontend e2e-тестов на соответствие стандартам:
  - **codegen для e2e**: добавлены `frontend/e2e/types/{schema.ts, test-schema.ts}` через `openapi-typescript` в `make generate-api`. Все ручные интерфейсы для API request/response в `helpers/api.ts` удалены — типы derive'ятся из generated `components["schemas"][...]` (требование `frontend-types.md`).
  - **cleanup fail-fast** (PR-comment): `try/catch{}` в `afterEach` убран; cleanup падает на первой же ошибке. `cleanupCreatorApplication` / `cleanupUser` проверяют status (204|404 OK, иначе throw). Применено также к `auth.spec.ts` и `submit.spec.ts`.
  - **per-call cleanup timeout 5s** (PR-comment): `withTimeout` (Promise.race + clearTimeout) защищает от зависшего backend-вызова.
  - **`auth.spec.ts` cleanup endpoint** (PR-comment): сломанный `DELETE /test/users/:email` заменён на `seedAdmin` helper, который использует `POST /test/cleanup-entity {type:"user", id}`. Заодно убран module-level `TEST_EMAIL/TEST_PASSWORD` (anti-pattern для параллельных воркеров) — каждый тест сидит своего admin'а.
  - **`uniqueIIN` рандомизация** (PR-comment): год (1985..2005), месяц (1..12), день (1..28) и serial рисуются из `crypto.randomBytes` — то же поведение, что и в `backend/e2e/testutil/iin.go::UniqueIIN`. Устранён May-15 UTC-midnight age-race и birthday-paradox для параллельных воркеров.
  - **`uniqueTelegramUserId` через crypto** (PR-comment): `epoch(1<<30) + crypto.randomBytes(6)` вместо `Date.now+counter`. Параллельно-безопасно без atomic counter, который ресетится на каждый Node-процесс.
  - **AC2 timing** (PR-comment): `seedCreatorApplication` делает `sleep(10ms)` перед каждым POST — детерминированный `created_at desc` даже на медленной CI с микросекундными ties в Postgres `now()`.
  - **`middleName=""` дроп** (PR-comment): helper больше не дропает поле; `middleName: string | null` (null — не отправлять в API, "" — отправлять как есть). Backend нормализует "" → nil через `trimOptional`. Добавлен test "Creator without middleName" с `middleName=null` для покрытия two-word ФИО без trailing-space.
  - **getByText("Дашборд"/"Админ") → testid + structural** (audit): копирайт-зависимые ассерты в auth.spec заменены на `dashboard-page` testid + проверка набора admin-only nav-link'ов (`creator-applications/verification`).
  - **submit.spec console.log → console.warn** (audit): соответствует `frontend-quality.md` (eslint allow `warn`/`error`).
  - **Test names без нумерации** (audit): `1. Happy login...` → `Happy login...`. Стандарт `frontend-testing-e2e.md` требует именование "по user flow", нумерация ничего не сообщает.
  - **`placeholder.test.ts` удалён** (audit): пустой `expect(true).toBe(true)`. Makefile target `test-unit-landing` обновлён с `--passWithNoTests` (landing — Astro-лендос, юнит-тестов фактически нет).
  - **JSDoc `auth.spec.ts`** (audit): обновлён нарратив, упоминание сломанного DELETE /test/users убрано.

## Verification

**Commands:**
- `make test-e2e-frontend` — expected: 5 новых тестов проходят в chromium, exit code 0; общая длительность нового spec'а <60s.
- `cd frontend/web && npx tsc --noEmit` — expected: zero errors после правок helper'ов.
- `cd frontend/web && npx eslint src/` — expected: zero new warnings.

**Manual checks:**
- Прогон с `E2E_CLEANUP=false`: после spec'а seed-users (`test-admin-…`/`test-bm-…`) и applications с uuid-префиксом остаются в БД; повторный прогон проходит (изоляция через uuid).

## Design Notes

**Telegram linking паттерн.** `linkTelegramToApplication` шлёт `/start <applicationId>` через `POST /test/telegram/message`. Бот обрабатывает update in-process; после ответа 200 привязка синхронно записана — можно сразу запускать UI без `waitFor`.

**Filter testid keys.** Реальные ключи `TelegramLinkedSegment` — `any`/`true`/`false` (не `linked`/`not_linked`). Ошибка в testid дала бы false-fail в неочевидном месте.

**RoleGuard target.** При роли brand_manager `RoleGuard` редиректит именно на `ROUTES.DASHBOARD` (`/`). AC5 ассертим точный URL, чтобы поймать смену поведения guard'а.

## Suggested Review Order

**Codegen для e2e (новое — Phase 0)**

- Makefile target `generate-api`: добавлены две строки `openapi-typescript` для production и test схем в `frontend/e2e/types/`
  `Makefile:193`

- Production OpenAPI types для e2e (auto-generated)
  `frontend/e2e/types/schema.ts:1`

- Test OpenAPI types для e2e (auto-generated)
  `frontend/e2e/types/test-schema.ts:1`

**Test scenarios — entry point**

- Header JSDoc + 6 сценариев одним `test.describe` — основная читаемая поверхность изменений
  `frontend/e2e/web/admin-creator-applications-verification.spec.ts:1`

- Happy path — drawer-equality на всех отображаемых полях, ключевая проверка
  `frontend/e2e/web/admin-creator-applications-verification.spec.ts:86`

- Creator without middleName — новый кейс на null/empty middleName (по результатам PR-ревью)
  `frontend/e2e/web/admin-creator-applications-verification.spec.ts:218`

- Drawer prev/next — детерминированный mix кнопок и keyboard через 3 заявки
  `frontend/e2e/web/admin-creator-applications-verification.spec.ts:259`

- Filter telegramLinked — three-branch фильтр + linkTelegramToApplication side-effect
  `frontend/e2e/web/admin-creator-applications-verification.spec.ts:331`

- RoleGuard для brand_manager — проверка sidebar + redirect
  `frontend/e2e/web/admin-creator-applications-verification.spec.ts:400`

**Cleanup-инфраструктура (PR-fix)**

- `withTimeout` + fail-fast `afterEach` — cleanup падает на первой ошибке, per-call 5s timeout
  `frontend/e2e/web/admin-creator-applications-verification.spec.ts:74`

**Helpers**

- `helpers/api.ts` верхушка — JSDoc + импорты derived из generated schemas (нет ручных API-типов)
  `frontend/e2e/helpers/api.ts:1`

- `uniqueIIN` через `crypto.randomBytes` — рандомные year/month/day/serial, mirror `testutil.UniqueIIN`
  `frontend/e2e/helpers/api.ts:81`

- `cleanupCreatorApplication` / `cleanupUser` — респектят `E2E_CLEANUP=false`, проверяют status, throw на unexpected
  `frontend/e2e/helpers/api.ts:124`

- `postJson<T>` — единственное место где Playwright's untyped resp.json() кастится к типу
  `frontend/e2e/helpers/api.ts:108`

- `seedAdmin` / `seedBrandManager` — wrappers вокруг seedUser
  `frontend/e2e/helpers/api.ts:170`

- `SeedCreatorApplicationOpts` / `SeededCreatorApplication` — derive'ы из generated `CreatorApplicationSubmitRequest`
  `frontend/e2e/helpers/api.ts:218`

- `seedCreatorApplication` — 10ms sleep, middleName="" больше не дропается, derived types
  `frontend/e2e/helpers/api.ts:255`

- `uniqueTelegramUserId` — `crypto.randomBytes(6)` вместо `Date.now+counter`
  `frontend/e2e/helpers/api.ts:347`

**Auth spec (большой рефакторинг — PR-fix + audit)**

- `auth.spec.ts` JSDoc — обновлён, нет упоминания сломанного DELETE /test/users
  `frontend/e2e/web/auth.spec.ts:1`

- 5 тестов на seedAdmin helper, без module-level state, data-testid вместо getByText("Дашборд"/"Админ")
  `frontend/e2e/web/auth.spec.ts:51`

**Submit spec (мелкие правки — audit)**

- `submit.spec.ts` afterAll — fail-fast cleanup без silent swallow
  `frontend/e2e/landing/submit.spec.ts:47`

- console.log → console.warn (соответствие `no-console: ["error", { allow: ["warn", "error"] }]`)
  `frontend/e2e/landing/submit.spec.ts:107`

**Прочее**

- `frontend/landing/src/placeholder.test.ts` — удалён
- Makefile `test-unit-landing` — `--passWithNoTests` для пустой landing test-suite
  `Makefile:137`

