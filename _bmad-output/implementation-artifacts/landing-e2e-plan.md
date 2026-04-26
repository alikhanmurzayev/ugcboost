# Plan: E2E тесты для лендинга (Playwright)

**Контекст:** PR #19 синхронизировал лендинг с бэком (1 чекбокс согласий, dynamic dictionaries, fetch к API, success-screen с динамическим bot URL). Backend E2E зелёные, но browser-flow лендинга не проверен. Нужны автоматизированные browser E2E на Playwright (test runner, не MCP).

**Дата:** 2026-04-25
**Ветка:** `alikhan/creator-application-submit`
**Связанные документы:**
- `docs/standards/frontend-testing-e2e.md` — обязательный стандарт (структура, нейминг, локаторы, cleanup)
- `docs/standards/frontend-components.md` — требование `data-testid` на интерактивные элементы
- `_bmad-output/implementation-artifacts/sync-with-landing-plan.md` — предыдущий план (контекст синхронизации)

---

## Принципы (из стандартов, что нельзя нарушать)

1. **`data-testid`** — каждый интерактивный элемент в форме получает testid (`frontend-components.md`). Тесты используют `getByTestId`, не текстовые селекторы.
2. **Структура:** конфиг в `frontend/landing/playwright.config.ts` (есть), спеки в `frontend/e2e/landing/`, общие хелперы в `frontend/e2e/helpers/api.ts` (`frontend-testing-e2e.md`).
3. **Заголовочный комментарий spec** — нарратив на русском, не bullet-list. Inline-комментарии в `test()` — на английском.
4. **`E2E_CLEANUP`** — env var управляет cleanup (default `true`, `false` оставляет данные для дебага). Cleanup через бизнес/тест ручки в afterAll.
5. **Изоляция:** `t.Parallel`-эквивалент в Playwright workers; каждый тест создаёт уникальные данные через generator (`UniqueIIN()` стиль).
6. **Не дублируем backend E2E** — там покрыта серверная валидация всех 422-сценариев (под-21, missing consent, unknown category, too many categories, other-без-text). Browser-тест проверяет UX-flow, а не пере-проверяет каждое серверное правило.
7. **Не использовать MSW / mocks API** — это true E2E против настоящего бэкенда. Backend поднимается через `make start-backend` или playwright `webServer`.

---

## Сценарии для покрытия

### Рекомендуемый минимум (3 теста)

**1. Golden path — заполнение валидной формы → success-screen**
- Подгружаются categories и cities из API (проверка что dropdown/checkboxes наполнены)
- Заполняем все required поля валидными данными (валидный IIN > 21 года)
- Чекаем 1 социалку, чекаем 1 категорию (не "other"), чекаем consent
- Клик submit → видим success-screen, CTA href начинается с `https://t.me/`
- В afterAll: cleanup application через POST /test/cleanup-entity

**2. Категория "Другое" — input появляется и требуется**
- Чекаем категорию `other` → input `categoryOtherText` становится visible + required
- Сабмитим без заполнения other-input → браузер блокирует через HTML5 required (либо backend возвращает 422 и мы видим error)
- Заполняем other-input валидным текстом → submit success
- Cleanup в afterAll

**3. Серверная ошибка отображается на UI**
- Сабмитим с под-21 IIN (валидный checksum) → backend возвращает 422 UNDER_AGE
- На UI появляется `data-testid="form-error"` с текстом из API ("Возраст менее 21 года")
- Form НЕ переходит на success-screen
- Кнопка submit снова enabled (можно повторить попытку)

### Опционально (если решим расширить)

**4. Threads platform** — Threads чекбокс + handle → 201
**5. Без consent** — submit без чекбокса → HTML5 required блокирует (тест на frontend-валидацию)

→ **См. Q1 ниже.**

---

## Шаги реализации

### Шаг 1: `data-testid` в `index.astro`

Добавить testid на каждый интерактивный элемент формы:

| Элемент | testid |
|---|---|
| `<form>` | `application-form` |
| `<input name="lastName">` | `last-name-input` |
| `<input name="firstName">` | `first-name-input` |
| `<input name="middleName">` | `middle-name-input` |
| `<input name="phone">` | `phone-input` |
| `<input name="iin">` | `iin-input` |
| `<select name="city">` | `city-select` |
| `<input data-social=X>` (checkbox) | `social-checkbox-{X}` (lowercase) |
| `<input data-social-input=X>` | `social-input-{X}` |
| `<input class="category-checkbox" value=Y>` | `category-checkbox-{Y}` (генерируется) |
| `<input id="category-other-input">` | `category-other-input` |
| `<input id="consent-all">` | `consent-all` |
| `<button type="submit">` | `submit-button` |
| `#form-error` | `form-error` |
| `#success-screen` | `success-screen` |
| `#success-cta` | `success-cta` |

**Verification:** `make build-landing && make lint-landing` зелёные. Существующая логика на CSS-классах (`.social-checkbox`, `.category-checkbox`) сохраняется — testid добавляется параллельно.

### Шаг 2: `frontend/e2e/helpers/api.ts`

Создать общий helper-модуль (стандарт требует helpers именно тут):

```typescript
// validIIN — детерминированно строит валидный казахстанский ИИН для возраста ~30 лет.
//   Используется в тестах, где нужен корректный checksum, но возраст не важен.
// underageIIN — IIN для возраста ~ MinCreatorAge - 2, чтобы стабильно ловить UNDER_AGE
//   независимо от реального времени.
// uniqueIIN — каждый вызов даёт уникальный (атомарный счётчик в serial), чтобы
//   параллельные тесты не натыкались на partial unique index.
// cleanupCreatorApplication — POST /test/cleanup-entity для удаления заявки в afterAll.
```

Алгоритм checksum — копия двух-проходного алгоритма из backend (домен-логика стабильна, дублирование оправдано: e2e-helpers — отдельный модуль, не импортирует backend).

**Где хранить:** `frontend/e2e/helpers/api.ts` (новый файл, стандарт это явно требует).

### Шаг 3: `frontend/e2e/landing/submit.spec.ts`

Создать spec с тестами по сценариям из раздела выше. Структура:

```typescript
/**
 * Package landing — E2E тесты HTTP-поверхности заявки креатора на лендинге EFW.
 *
 * <... нарратив на русском по образцу auth.spec.ts ...>
 */
import { test, expect } from "@playwright/test";
import { uniqueIIN, underageIIN, cleanupCreatorApplication } from "../helpers/api";

const API_URL = process.env.API_URL || "http://localhost:8080";
// Created application IDs collected in tests for cleanup in afterAll.
const created: string[] = [];

test.afterAll(async ({ request }) => {
  if (process.env.E2E_CLEANUP === "false") return;
  for (const id of created.reverse()) {
    await cleanupCreatorApplication(request, API_URL, id);
  }
});

test.describe("Landing submission flow", () => {
  test("1. Golden path …", async ({ page, request }) => { ... });
  test("2. Other category …", async ({ page, request }) => { ... });
  test("3. Server validation …", async ({ page }) => { ... });
});
```

### Шаг 4: Makefile target `test-e2e-landing`

По образцу `test-e2e-frontend`:

```makefile
test-e2e-landing: start-backend
	cd frontend/landing && CI=true BASE_URL=http://localhost:3003 API_URL=http://localhost:8082 npx playwright test
```

→ **См. Q2 ниже** про webServer vs docker landing.

### Шаг 5: CORS

Backend по env `CORS_ORIGINS` whitelist'ит origins. Сейчас включает `:3003` (docker landing) и `:4321` (astro dev) — но это в локальном `.env`, который в `.gitignore`. Для CI это нужно либо:
- Зашить дефолт в `config.go` (включая landing origins безусловно)
- Или передавать через CI secrets

→ **См. Q3 ниже.**

### Шаг 6: Verification

- `cd frontend/landing && npx playwright test` локально — зелёный
- `make test-e2e-landing` (с docker landing+backend) — зелёный
- Запустить дважды подряд — оба раза зелёные (проверка идемпотентности)
- `E2E_CLEANUP=false make test-e2e-landing` → в БД остаются заявки → проверить через psql
- `E2E_CLEANUP=true make test-e2e-landing` → в БД ничего не остаётся

---

## Открытые вопросы

| ID | Вопрос | Default |
|---|---|---|
| Q1 | Сколько сценариев — 3 (минимум) или 5 (с threads + frontend-only consent validation)? | **3** (минимум, не дублируем backend E2E) |
| Q2 | Где запускаем лендинг для тестов — docker landing :3003 или Playwright webServer (Astro dev :4321)? | **docker landing** — ближе к проду, не требует Astro dev (быстрее CI) |
| Q3 | CORS_ORIGINS для CI/staging/prod — зашить landing origins в код или передавать через env? | **через env** — оставляем как сейчас, добавим в README документацию какие origins нужны на каждом env |
| Q4 | UniqueIIN counter — глобальный JS-singleton или timestamp-based? | **4-значный random в serial-секции IIN** — без shared state, безопасно для параллельных workers, коллизия 1/10000 за прогон (на 3 теста — практически 0) |

---

## Definition of Done

- [ ] `data-testid` добавлены во все 16 элементов формы согласно таблице (Шаг 1)
- [ ] `make build-landing && make lint-landing` зелёные после правок Astro
- [ ] `frontend/e2e/helpers/api.ts` создан с validIIN/uniqueIIN/underageIIN/cleanupCreatorApplication
- [ ] `frontend/e2e/landing/submit.spec.ts` создан с N тестами (см. Q1)
- [ ] Заголовочный комментарий на русском, нарратив, без bullet-list (стандарт)
- [ ] Inline комментарии в `test()` на английском (стандарт)
- [ ] Каждый тест использует свой `uniqueIIN()` — параллельный запуск зелёный
- [ ] `E2E_CLEANUP=true` (default) → БД чиста после прогона
- [ ] `E2E_CLEANUP=false` → данные остаются для дебага
- [ ] Makefile target `test-e2e-landing` добавлен и зелёный
- [ ] CORS-вопрос решён (Q3) — либо документация, либо код
- [ ] Тесты проходят дважды подряд без перезапуска БД (идемпотентность)
- [ ] Spec не использует селекторы, отсутствующие в HTML после Шага 1

---

## Risks

| Риск | Митигация |
|---|---|
| `npx playwright install chromium` не выполнен на машине | Документировать в README + добавить как build-step в Makefile (или скрытно в test-e2e-landing) |
| CORS preflight отклоняется при тестах | Заранее добавить :3003 и :4321 в CORS_ORIGINS на всех env (Q3) |
| Race на partial unique index при параллельных тестах с одинаковым IIN | uniqueIIN с atomic counter (Q4) |
| Backend в docker не успевает подняться к моменту тестов | start-backend имеет healthcheck wait — корректно блокирует |
| HTML5 required может тормозить тест-сценарий "без consent" (sub-test 5) | Использовать `form.dispatchEvent(new SubmitEvent...)` или просто исключить этот сценарий — он избыточен над unit-тестом |

---

## Execution log

*Заполняется по ходу работы.*

- _2026-04-25_ — план составлен, ожидание решений по Q1-Q4 от Alikhan.
- _2026-04-25_ — Alikhan ответил: Q1=3 (минимум), Q2=как web (webServer для local + docker для CI), Q3=env-only + README, Q4=4-значный random в serial.
- _2026-04-25_ — Шаг 1: 16 `data-testid` добавлены в `index.astro` (форма + поля + чекбоксы соцсетей/категорий + consent + submit + form-error + success-screen + success-cta). Динамические category checkbox получают testid через innerHTML template.
- _2026-04-25_ — Шаг 2: `playwright.config.ts` уточнён под Q2 — webServer стартует backend (`go run` :8080) и Astro dev (:4321) для local; `CI=true` отключает webServer (тесты идут к docker через make).
- _2026-04-25_ — Шаг 3: `frontend/e2e/helpers/api.ts` создан. validIIN/uniqueIIN/underageIIN с правильным century byte (1/3/5 по году рождения, как в backend `iinYear`). cleanupCreatorApplication через POST /test/cleanup-entity. TS strict: убрал `!` non-null assertions согласно `frontend-quality.md`.
- _2026-04-25_ — Шаг 4: `frontend/e2e/landing/submit.spec.ts` создан. 3 теста: Golden path, Other category, Server validation. Заголовочный комментарий — нарратив на русском (3 абзаца). pageerror/requestfailed listeners для дебага. Cleanup в afterAll через E2E_CLEANUP env.
- _2026-04-25_ — Шаг 5: Makefile target `test-e2e-landing` добавлен (по образцу `test-e2e-frontend`, BASE_URL=:3003, API_URL=:8082). README дополнен таблицей CORS_ORIGINS для всех окружений.
- _2026-04-25_ — Шаг 6: финальная верификация прошла, **3/3 passed**.

### Найденные/исправленные баги по ходу

1. **`pattern="\d{12}"` в Astro теряет бекслеш** → HTML5 валидация всегда фейлила. Заменил на `minlength=12` + `maxlength=12` (валидация формата всё равно делается на бэке).
2. **Category checkbox имеет `class="hidden"`** → Playwright `check()` не работает даже с `force:true`. Добавил `data-testid="category-label-${code}"` на wrapping `<label>`, тесты кликают по label (стандартный UX-flow).
3. **IIN century byte** в helper изначально был зашит `5` → backend интерпретировал 1995 как 2095 → UNDER_AGE для всех. Исправил `centuryByteFor(year)` по mapping из `domain/iin.go`.

### Verification

- Single run: `make test-e2e-landing` → `3 passed (~5s)`
- Двойной прогон подряд: оба `3 passed` (idempotency ✅)
- `E2E_CLEANUP=false` → 2 заявки в БД (golden + other; server-validation не создаёт)
- `E2E_CLEANUP=true` (default) → БД чиста (✅)

### CI integration

Чтобы новые тесты прогонялись в GitHub Actions (помимо локального `make test-e2e-landing`):

- `.github/workflows/ci.yml` — добавлен job `test-e2e-landing` в Stage 2 (параллельно с `test-e2e-backend` и `test-e2e-frontend`). По образцу `test-e2e-frontend`: поднимает `docker-compose.ci.isolated.yml`, устанавливает chromium через `npx playwright install`, запускает `CI=true npx playwright test` против landing :3003 + backend :8082, в случае падения апплоадит report.
- `backend/.env.ci` — CORS_ORIGINS дополнен `http://localhost:3003`, чтобы isolated backend пропускал запросы от landing-контейнера.
- `docker-compose.ci.isolated.yml` уже содержит landing service на :3003 — изменения не нужны.
- Staging E2E для landing (Stage 4 `test-staging-landing`) — **deferred**, по аналогии с тем что staging-tma тоже отсутствует. Локальный isolated прогон покрывает submit-flow; staging-проверка — отдельный шаг.

### Что Alikhan получит при ревью

- Working tree содержит:
  - `frontend/landing/playwright.config.ts` (новый)
  - `frontend/landing/src/pages/index.astro` (data-testid + pattern fix + category label testid)
  - `frontend/e2e/helpers/api.ts` (новый)
  - `frontend/e2e/landing/submit.spec.ts` (новый)
  - `Makefile` (test-e2e-landing target)
  - `README.md` (раздел про CORS_ORIGINS)
  - `.github/workflows/ci.yml` (новый job test-e2e-landing)
  - `backend/.env.ci` (+ http://localhost:3003 в CORS)
- Никаких новых коммитов (per memory `feedback_no_commits`).
- Все автотесты зелёные локально (backend unit/coverage/e2e + landing e2e). CI прогон проверится после push на PR.
