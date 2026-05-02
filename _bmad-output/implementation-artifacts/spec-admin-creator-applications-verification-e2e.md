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

**Test scenarios — entry point**

- Header JSDoc + 5 AC-сценариев одним `test.describe` — основная читаемая поверхность изменений
  [`admin-creator-applications-verification.spec.ts:1`](../../frontend/e2e/web/admin-creator-applications-verification.spec.ts#L1)

- AC1 happy path — drawer-equality на всех отображаемых полях, ключевая проверка
  [`admin-creator-applications-verification.spec.ts:74`](../../frontend/e2e/web/admin-creator-applications-verification.spec.ts#L74)

- AC2 — детерминированный prev/next через 3 заявки, mix кнопок и keyboard
  [`admin-creator-applications-verification.spec.ts:212`](../../frontend/e2e/web/admin-creator-applications-verification.spec.ts#L212)

- AC3 — three-branch фильтр telegramLinked + linkTelegramToApplication side-effect
  [`admin-creator-applications-verification.spec.ts:286`](../../frontend/e2e/web/admin-creator-applications-verification.spec.ts#L286)

- AC5 — RoleGuard для brand_manager, проверка sidebar + redirect в одном тесте
  [`admin-creator-applications-verification.spec.ts:355`](../../frontend/e2e/web/admin-creator-applications-verification.spec.ts#L355)

**Helpers и инфра спеки**

- `loginAs` — UI-логин, mirror auth.spec.ts pattern
  [`admin-creator-applications-verification.spec.ts:381`](../../frontend/e2e/web/admin-creator-applications-verification.spec.ts#L381)

- `fetchDictionary` + `lookupOrThrow` — резолвят live dict для assert'ов на city/category
  [`admin-creator-applications-verification.spec.ts:416`](../../frontend/e2e/web/admin-creator-applications-verification.spec.ts#L416)

**Расширения api.ts (helpers)**

- `seedCreatorApplication` — POST на public endpoint, дефолты + opts override
  [`api.ts:205`](../../frontend/e2e/helpers/api.ts#L205)

- `linkTelegramToApplication` — синхронный in-process bot-handler через `/test/telegram/message`
  [`api.ts:317`](../../frontend/e2e/helpers/api.ts#L317)

- `seedAdmin` / `seedBrandManager` — обёртки над seedUser с unique email + cleanup-closure
  [`api.ts:104`](../../frontend/e2e/helpers/api.ts#L104)

- `uniqueTelegramUserId` — mirrors testutil.UniqueTelegramUserID на бэке
  [`api.ts:290`](../../frontend/e2e/helpers/api.ts#L290)

- `parseBirthDateFromIin` — обратная функция к generateValidIIN, century-byte → year
  [`api.ts:272`](../../frontend/e2e/helpers/api.ts#L272)

