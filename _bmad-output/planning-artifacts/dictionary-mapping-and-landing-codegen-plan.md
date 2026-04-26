---
type: implementation-plan
feature: dictionary-mapping-and-landing-codegen
created: 2026-04-25
---

# План реализации: маппинг словарей в handler + кодогенерация для лендинга

## Перед началом — обязательно (читать в новой сессии)

Этот план — самодостаточная инструкция. Всё, что должен сделать агент в свежей сессии, описано здесь. До первой строчки кода:

1. **Прочитай ВСЕ файлы из `docs/standards/` целиком, полностью, в сыром виде.** Не выборочно по релевантности — все файлы. Запусти `ls docs/standards/`, потом прочитай каждый файл от начала до конца. Полный список (на момент написания плана): `backend-architecture`, `backend-codegen`, `backend-constants`, `backend-design`, `backend-errors`, `backend-libraries`, `backend-repository`, `backend-testing-e2e`, `backend-testing-unit`, `backend-transactions`, `frontend-api`, `frontend-components`, `frontend-quality`, `frontend-state`, `frontend-testing-e2e`, `frontend-testing-unit`, `frontend-types`, `naming`, `security`. Стандарты — это источник истины проекта; план их не дублирует.
2. **Прочитай файлы-референсы из секции "Полезные референсы" — каждый по указанным line-numbers.** План описывает направление и решения, но не сигнатуры — сигнатуры в существующем коде.
3. **Прочитай текущее состояние файлов из таблицы "Файлы для изменения"** — Phase 2-6 рефакторят их.

После этой подготовки сразу переходи к Phase 1. Не задавай вопросов про "готов ли приступать" — план самодостаточен.

## Обзор

Два технических слайса в одном плане:

**A. Перенести маппинг словарных кодов → human-readable имён в handler-слой.**
Сейчас в `GET /creators/applications/{id}` категории резолвятся в repo через `INNER JOIN categories` (repo возвращает `{Code, Name, SortOrder}`), а `city` отдаётся сырым кодом. Цель: repo и service оперируют **только кодами**, handler перед `respondJSON` обогащает response через `DictionaryService`. Единое правило: repo/service — данные, handler — представление.

**B. Подключить кодогенерацию (`openapi-typescript`) для `frontend/landing`.**
Сейчас лендинг ходит в API через сырые `fetch` без типов — нарушение `docs/standards/backend-codegen.md`. Цель: те же `make generate-api` обновляют схему лендинга, fetch-вызовы вынесены в типизированный модуль `src/api/client.ts`.

Оба слайса связаны темой "контрактная гигиена фронт↔бэк", но технически независимы; план их не разводит по двум артефактам, чтобы агент в одной сессии прошёл оба.

## Требования

### Must-have (Slice A — handler mapping)

- **REQ-A1.** `creatorApplicationCategoryRepository.ListByApplicationID` возвращает только category codes (`[]string` через `dbutil.Vals[string]`). JOIN на `categories` для извлечения кода остаётся (без него код не достать), но репо НЕ возвращает name/sort_order.
- **REQ-A2.** Service `GetByID` возвращает domain object, в котором `Categories []string` (codes) и `City string` (code). Никакого резолва человекочитаемых имён в service.
- **REQ-A3.** Handler `GetCreatorApplication` перед маппингом дёргает `dictionaryService.List(ctx, domain.DictionaryTypeCategories)` и `dictionaryService.List(ctx, domain.DictionaryTypeCities)`, строит lookup-карты `map[string]domain.DictionaryEntry`, и обогащает response сводкой `{code, name, sortOrder}`.
- **REQ-A4.** OpenAPI-схема обновлена: `CreatorApplicationDetailData.city` теперь объект `{code, name, sortOrder}` (новая схема `CreatorApplicationDetailCity`). `CreatorApplicationDetailCategory` остаётся как есть (схема уже подходит).
- **REQ-A5.** Если код в БД не найден в активном словаре (категория/город деактивированы), handler возвращает fallback `{code, name: code, sortOrder: 0}` — НЕ 500. Инвариант: deactivation в словаре не должна ломать чтение исторической заявки.
- **REQ-A6.** Сортировка categories в response: `(sortOrder ASC, code ASC)` — handler сортирует in-memory после маппинга. Repo всё ещё может ORDER BY для стабильности codes-выдачи, но handler не полагается на repo-порядок.
- **REQ-A7.** Все слои покрыты обновлёнными unit-тестами, ≥80% per-method.
- **REQ-A8.** Backend E2E тесты creator_application продолжают зеленеть: helper `verifyCreatorApplicationByID` обновлён под новую форму city и categories.

### Must-have (Slice B — landing codegen)

**Принцип:** взаимодействие с бэкендом в лендинге должно быть устроено **максимально похоже на `frontend/web`**: тот же runtime-клиент (`openapi-fetch`), та же структура папки `src/api/` с `client.ts` + per-domain TS-модулями, та же форма функций (`{data, error, response}` от `openapi-fetch` → `ApiError` или возврат `data`). Принципиальные отличия — в лендинге **нет auth-middleware** (ручки публичные), поэтому `client.ts` — без `onRequest`/`onResponse` middleware и без интеграции с auth-стором; также не нужны `rawClient`, `refreshToken`/`refreshPromise` — лендингу нечего обновлять. Копируются только: импорты, `getApiBase()`, экспорт `apiBase`, класс `ApiError`, ОДИН `client = createClient<paths>({ baseUrl })` без `credentials: "include"` (cookies лендингу не нужны для публичных ручек), и helper `extractErrorCode` — но как в web (по копии **в каждом per-domain файле**, не в `client.ts`).

- **REQ-B1.** В `Makefile` цель `generate-api` запускает `openapi-typescript` ещё и для `frontend/landing`. Артефакт — `frontend/landing/src/api/generated/schema.ts`.
- **REQ-B2.** В `frontend/landing/package.json` есть dev-deps `openapi-typescript` и `openapi-fetch` (те же мажорные версии, что в `frontend/web/package.json` — на момент написания плана `openapi-typescript ^7.13.0` и `openapi-fetch ^0.17.0`).
- **REQ-B3.** Структура `frontend/landing/src/api/` зеркалит `frontend/web/src/api/`:
  - `client.ts` — единая точка с `createClient<paths>({...})`, экспортом `client` (default), `apiBase`, и классом `ApiError` (повторить сигнатуру из `frontend/web/src/api/client.ts:15-25`). **Auth-middleware не добавлять** (лендинг публичный). Helper `getApiBase()` — **точная копия `frontend/web/src/api/client.ts:5-10`**: `window.__RUNTIME_CONFIG__?.apiUrl` со fallback на `/api`. Лендинг уже использует тот же механизм (`<script is:inline src="/config.js">` в `index.astro:26` сетит `window.__RUNTIME_CONFIG__`); адаптация не нужна.
  - `dictionaries.ts` — экспортирует функцию `listDictionary(type)`, тип `DictionaryEntry` через `components["schemas"]`, тип `DictionaryType` либо из `components["schemas"]` (если есть), либо извлечь как `paths["/dictionaries/{type}"]["get"]["parameters"]["path"]["type"]`. Внутри — `client.GET("/dictionaries/{type}", { params: { path: { type } } })`, error → `throw new ApiError(...)`, success → `return data`. Helper `extractErrorCode` — **локальная копия в этом файле** (как в `frontend/web/src/api/audit.ts:6-9`).
  - `creator-applications.ts` — экспортирует `submitCreatorApplication(payload)`, тип `CreatorApplicationSubmitRequest` из `components["schemas"]`. Внутри — `client.POST("/creators/applications", { body: payload })`. Тот же паттерн error/success. `extractErrorCode` — **локальная копия в этом файле**.
  - `generated/schema.ts` — артефакт `openapi-typescript`.
- **REQ-B4.** В `frontend/landing/src/pages/index.astro` сырые `fetch` (`/dictionaries/${type}`, `/creators/applications`) заменены на вызовы `listDictionary` и `submitCreatorApplication` из `src/api/`. Никакого ручного `JSON.parse`/`JSON.stringify` шейпа payload — типы из generated.
- **REQ-B5.** `make lint-landing` (tsc + eslint) — зелёный после рефакторинга. Любая рассинхрон-ошибка между `paths` и фактическими вызовами должна ловиться `tsc --noEmit`.
- **REQ-B6.** Существующие тесты лендинга (`frontend/landing` unit + `frontend/e2e/landing/`) остаются зелёными.
- **REQ-B7.** Коды ошибок в обработке submit (например `CREATOR_APPLICATION_DUPLICATE`, `INVALID_IIN`, `UNDER_AGE`, `MISSING_CONSENT`, `UNKNOWN_CATEGORY`, `VALIDATION_ERROR`) ловятся через `try/catch` на `ApiError.code` — пользовательские сообщения в `index.astro` остаются те же, меняется только источник кода.
- **REQ-B8 (Astro `<script>` constraint).** В `index.astro` сейчас весь form-runtime сидит внутри `<script define:vars={{...}}>` (строка ~640). Этот тег рендерится как **inline script без module-обработки** — `import` statement в нём **не работает**. Чтобы подключить типизированный клиент, нужно перестроить структуру:
  - Удалить `define:vars` с основного `<script>`-блока, оставив его обычным module-script (Astro обработает через Vite, импорты заработают).
  - Передать прежние константы (`socialCodeMap`, `otherCategoryCode`, `maxCategories`) через `<script type="application/json" id="form-config">`/`data-*` атрибуты или через дополнительный inline-script, который сетит глобальный объект до загрузки модуля. Точная техника на усмотрение исполнителя — критерий: основной script остаётся ES-модулем и может импортировать `listDictionary`/`submitCreatorApplication`.
  - `<script is:inline src="/config.js">` (строка 26) **не трогать** — он сетит `window.__RUNTIME_CONFIG__` до module-script'ов и работает в существующем порядке.

### Must-have (CI)

- **REQ-C1.** В `.github/workflows/ci.yml` пайплайн (если в нём есть `make generate-api` + diff-check на актуальность сгенерированных файлов) автоматически покрывает новый артефакт `frontend/landing/src/api/generated/schema.ts`. Если diff-check'а нет — план НЕ добавляет его; убедиться, что текущий пайплайн не падает на новый файл.
- **REQ-C2.** Локально все make-цели зелёные: `test-unit-backend`, `test-unit-backend-coverage`, `test-e2e-backend` (двойной прогон), `lint-backend`, `build-backend`, `lint-landing`, `test-unit-landing`, `test-e2e-landing`, `build-landing`.
- **REQ-C3 (новый).** В CI Stage 4 (staging E2E) появляется новый job `test-staging-landing`, **зеркало `test-staging-web`**: `npm ci -w landing` + `npx playwright install chromium` (в `frontend/landing/`) + `CI=true npx playwright test` против staging-инфраструктуры. Зависимости — `[migrate-staging, deploy-staging]` (как у `test-staging-web`). Env-переменные: `BASE_URL: https://staging.ugcboost.kz`, `API_URL: https://staging-api.ugcboost.kz`, `CF_ACCESS_CLIENT_ID`/`CF_ACCESS_CLIENT_SECRET` из существующих secrets. Также подгружать playwright-report как artifact (`playwright-report-staging-landing`).
- **REQ-C4 (известно).** Job `test-e2e-landing` в CI Stage 2 **уже существует** (`.github/workflows/ci.yml:278-321`) — он гоняет playwright против изолированного docker compose. После Phase 5-6 он автоматически проверит новый код; никаких правок этого job в плане **не требуется**.

### Out of scope

- Кэширование словарей в сервисе/handler (DictionaryService гонит SELECT каждый раз — это OK для admin GET).
- Локализация имён словарей (один язык — русский, как сейчас в БД).
- Маппинг enum'ов домена (`status`, `consent_type`, `platform`) — остаются кодами; локализация на фронте по необходимости.
- Внедрение `openapi-fetch` в `frontend/tma` (там сейчас только сгенерированные типы — это отдельный слайс).
- Добавление `DictionaryService.GetByCodes` без фильтра по `active` (для деактивированных категорий — пока fallback).
- Diff-check сгенерированных файлов в CI (если его сейчас нет — план не добавляет).

### Критерии успеха

В финале:
- Все make-цели из REQ-C2 зелёные.
- `make generate-api` идемпотентен (повторный прогон не меняет файлы).
- В response от GET creator-app: `city` — объект, `categories[]` — объекты с резолвленным name; коды не активного словаря дают fallback вместо 500.
- В `frontend/landing/src/pages/index.astro` нет ни одного `fetch(\`${apiUrl}/...\`)` — всё через `src/api/client.ts`.

## Зафиксированные решения

- **Resolver в handler, не в middleware/декораторе.** Один эндпоинт нуждается в этом сейчас; преждевременная абстракция запрещена. Если в будущем появится список заявок — рефакторить точечно.
- **Fallback при отсутствии кода в активном словаре:** `{code, name: code, sortOrder: 0}`. Не 500, не пустой объект. Гарантирует, что deactivation словарной записи не ломает чтение исторических заявок. Альтернатива (`GetByCodes` без фильтра по active) — out of scope этого слайса.
- **Repo categories — `[]string`, не структура.** `dbutil.Vals[string]` короче и проще, чем плодить новую structure для одного поля. Если в будущем понадобится возвращать дополнительные поля — мигрировать на структуру.
- **City в response — объект `{code, name, sortOrder}`, не пара `cityCode/cityName`.** Симметрия с categories: одна форма для словарных значений.
- **Лендинг использует `openapi-fetch` + структуру `src/api/{client,<domain>}.ts`, как в `frontend/web`.** Это решение: "максимально похоже на web". `frontend/tma` пока остаётся на сырых типах — отдельный слайс. Auth-middleware на лендинге не нужен (ручки публичные), поэтому `client.ts` лендинга — это упрощённая версия web-варианта без `onRequest`/`onResponse` интерсепторов.
- **CI без diff-check'а сгенерированных файлов** (если его сейчас нет). Артефакты коммитятся в репо. Локальная регенерация перед PR — ответственность разработчика. Этот план НЕ добавляет diff-check.
- **Никаких новых env vars на бэкенде.** `DictionaryService` уже работает с БД, словари `categories` и `cities` seeded миграциями. Лендинг по-прежнему обращается на `apiUrl`.

## Файлы для изменения

| Файл | Изменения |
|------|-----------|
| `backend/api/openapi.yaml` | Добавить схему `CreatorApplicationDetailCity{code, name, sortOrder}`. В `CreatorApplicationDetailData` поле `city` сделать `$ref` на новую схему. |
| `backend/internal/api/server.gen.go` | Регенерация (`make generate-api`). |
| `backend/e2e/apiclient/{client,types}.gen.go` | Регенерация. |
| `frontend/{web,tma}/src/api/generated/schema.ts` | Регенерация. |
| `backend/internal/domain/creator_application.go` | Удалить `CreatorApplicationDetailCategory` (тип переезжает в API/handler). `CreatorApplicationDetail.Categories []string`. `City` оставить `string` (это код). `CreatorApplicationDetailSocial`/`CreatorApplicationDetailConsent` без изменений. |
| `backend/internal/repository/creator_application_category.go` | Удалить `CreatorApplicationCategoryDetailRow`. `ListByApplicationID` теперь возвращает `[]string` через `dbutil.Vals[string]`. SQL: `SELECT c.code FROM creator_application_categories cac JOIN categories c ON c.id = cac.category_id WHERE cac.application_id = $1 ORDER BY c.sort_order ASC, c.code ASC`. Обновить интерфейс `CreatorApplicationCategoryRepo`. |
| `backend/internal/repository/creator_application_category_test.go` | Новый SQL-литерал, новый ожидаемый результат (`[]string`). |
| `backend/internal/service/creator_application.go` | `creatorApplicationDetailFromRows` принимает `[]string` для категорий. Маппинг упрощён до `append([]string(nil), categories...)`. |
| `backend/internal/service/creator_application_test.go` | Ожидаемый domain — `Categories: []string{"beauty","fashion"}`. |
| `backend/internal/handler/creator_application.go` | В `GetCreatorApplication` после `service.GetByID` — два вызова `dictionaryService.List`. Lookup-карты передаются в обновлённый `domainCreatorApplicationDetailToAPI(id, detail, categoriesByCode, cityByCode)`. Маппер делает резолв с fallback и сортирует categories. |
| `backend/internal/handler/creator_application_test.go` | В success-сценарии `TestServer_GetCreatorApplication` мок `MockDictionaryService.EXPECT().List(...)` для двух типов. Новые сценарии: `success with deactivated category falls back to code` и `dictionary list error → 500`. Обновить ожидаемый response: `city` — объект, `categories` — объекты с резолвленными name. Конструкция server'а в каждом тесте теперь должна включать ненулевой `dictionaryService` (или nil — там, где он не вызывается). |
| `backend/internal/handler/mocks/*` | Регенерация (`make generate-mocks`). `MockDictionaryService` уже есть; пересоздать на всякий случай. |
| `backend/e2e/creator_application/creator_application_test.go` | `expectedCreatorApplication.City` поменять на `string` (отправляемый код, например `"almaty"`). `verifyCreatorApplicationByID`: проверка `got.City.Code == expected.City`, `got.City.Name` непустой. Для `got.Categories[]` — проверить ElementsMatch по `Code`, что `Name` непустой, что порядок `(sort_order, code)`. |
| `Makefile` | В цель `generate-api` добавить строку для landing рядом с web и tma. |
| `frontend/landing/package.json` | Добавить `openapi-typescript` и `openapi-fetch` в `devDependencies` (если ещё нет). Версии — те же, что в `frontend/web/package.json`. |
| `frontend/landing/src/pages/index.astro` | В `<script>` импорт из `../api/dictionaries.ts` и `../api/creator-applications.ts`. `loadDictionary` (~711) → `listDictionary`. Submit-handler (~823) → `submitCreatorApplication`. Обработка ошибок через `try/catch` на `ApiError.code`. Никаких inline `fetch`. **Также перестроить `<script>` блок** под REQ-B8 (убрать `define:vars`, перенести константы в `<script type="application/json" id="form-config">`, основной `<script>` стал ES-модулем). |
| `.github/workflows/ci.yml` | Добавить новый job `test-staging-landing` рядом с `test-staging-web` (Stage 4), зеркальная структура. См. REQ-C3. |

## Файлы для создания

| Файл | Назначение |
|------|------------|
| `frontend/landing/src/api/client.ts` | Default-export `client = createClient<paths>({ baseUrl: BASE })`, named-экспорт `apiBase`, `ApiError`. Без auth-middleware, без `rawClient`, без `refreshToken`. Содержит: импорты + `getApiBase()` + `apiBase` + `ApiError` + один `client`. Зеркало `frontend/web/src/api/client.ts` строк 1-25 (импорты, `getApiBase`, `apiBase`, `ApiError`) плюс упрощённый аналог строк 46-49 (один `createClient` без `credentials: "include"`). НЕ копировать строки 28-44 (`rawClient`/`refreshToken`/`refreshPromise`) и 51-82 (`client.use(...)`). |
| `frontend/landing/src/api/dictionaries.ts` | `listDictionary(type)` через `client.GET("/dictionaries/{type}", { params: { path: { type } } })` + тип `DictionaryEntry` из `components["schemas"]` + тип `DictionaryType`. Локальный `extractErrorCode` helper. Паттерн — `frontend/web/src/api/audit.ts`. |
| `frontend/landing/src/api/creator-applications.ts` | `submitCreatorApplication(payload)` через `client.POST("/creators/applications", { body })` + тип `CreatorApplicationSubmitRequest`. Локальный `extractErrorCode` helper. Паттерн — `frontend/web/src/api/brands.ts:31-39`. |
| `frontend/landing/src/api/generated/schema.ts` | Артефакт `openapi-typescript` (создаётся `make generate-api` после правки Makefile). |

## Полезные референсы (паттерны для копирования)

- **DictionaryService на handler-уровне:**
  - `backend/internal/handler/dictionary.go` — паттерн handler'а, обращающегося к `dictionaryService.List`.
  - `backend/internal/handler/server.go:59-61` — интерфейс `DictionaryService.List(ctx, type)`.
  - `backend/internal/service/dictionary.go` — реализация (читать целиком, в т.ч. как тип `domain.DictionaryType` мапится на таблицу).
  - `backend/internal/domain/dictionary.go` — `DictionaryType`, `DictionaryEntry`, константы `DictionaryTypeCategories`/`DictionaryTypeCities`.
  - `backend/internal/handler/dictionary_test.go` — паттерн моков `MockDictionaryService`.
- **Текущая реализация GET creator app (то, что рефакторим):**
  - `backend/internal/handler/creator_application.go` — handler `GetCreatorApplication` и маппер `domainCreatorApplicationDetailToAPI`.
  - `backend/internal/service/creator_application.go` — `GetByID` и `creatorApplicationDetailFromRows`.
  - `backend/internal/repository/creator_application_category.go` — текущий `ListByApplicationID` с JOIN.
  - `backend/internal/handler/creator_application_test.go` — текущие unit-тесты `TestServer_GetCreatorApplication`.
  - `backend/internal/repository/helpers_test.go` — `newPgxmock` (используется в repo-тестах).
- **Frontend codegen (эталон для лендинга — копировать максимально точно):**
  - `frontend/web/package.json` — где зарегистрированы `openapi-typescript ^7.13.0` и `openapi-fetch ^0.17.0`.
  - `frontend/web/src/api/client.ts` — эталон structuring `client.ts`. Лендинг повторяет строки 1-49 (минус строки 51-82 с `client.use({...})` middleware — лендинг публичный). `getApiBase()` адаптируется под механизм лендинга (`window.__RUNTIME_CONFIG__` или `frontend/landing/public/config.js`).
  - `frontend/web/src/api/audit.ts` — простейший пример per-domain модуля; повторить паттерн в `dictionaries.ts`.
  - `frontend/web/src/api/brands.ts` — образец для `creator-applications.ts` (POST с body).
  - `frontend/web/src/api/auth.ts` — пример обработки публичного-ish endpoint без auth.
  - `frontend/web/src/api/generated/schema.ts` — пример сгенерированного типа.
  - `Makefile:167-186` — текущая цель `generate-api`.
- **Существующая landing-конфигурация:**
  - `frontend/landing/public/config.js` — текущий механизм передачи `apiUrl` лендингу (изучить, чтобы `getApiBase()` повторил эту логику).
  - `frontend/landing/src/pages/index.astro` — где сейчас читается `apiUrl` (где-то рядом со строками 711/823).
- **Landing fetch-вызовы (что переносим):**
  - `frontend/landing/src/pages/index.astro:711-738` — `loadDictionary` + `bootDictionaries`.
  - `frontend/landing/src/pages/index.astro:780-830` — submit handler.
- **Frontend стандарты:** `docs/standards/frontend-api.md`, `docs/standards/backend-codegen.md`, `docs/standards/frontend-types.md`.
- **`dbutil.Vals[string]`:** `backend/internal/dbutil/db.go:94-108` (обобщённая функция возврата slice скаляров; см. использование в `brand.go:GetBrandIDsForUser`).

## Шаги реализации

### Phase 1 — Подготовка

1. Прочитать все файлы из `docs/standards/` целиком.
2. Прочитать референсные файлы.
3. `git status` — убедиться, что working tree в нужном состоянии. Если есть несвязанные изменения — спросить у Alikhan.

### Phase 2 — Backend: contract + repo + service

4. **OpenAPI.** В `backend/api/openapi.yaml`:
   - Добавить схему `CreatorApplicationDetailCity`: type object, required `[code, name, sortOrder]`, поля `code` (string), `name` (string), `sortOrder` (integer).
   - В `CreatorApplicationDetailData` заменить `city: type: string` на `city: $ref: "#/components/schemas/CreatorApplicationDetailCity"`. Required-список тот же.
5. **Регенерация контракта.** `make generate-api`. Проверить `git status` — обновились только `*.gen.go` и `schema.ts` (web/tma; для landing после Phase 5).
6. **Repo categories.** В `backend/internal/repository/creator_application_category.go`:
   - Удалить `CreatorApplicationCategoryDetailRow`.
   - `ListByApplicationID` возвращает `[]string` через `dbutil.Vals[string]`. SQL: `SELECT c.code FROM creator_application_categories cac JOIN categories c ON c.id = cac.category_id WHERE cac.application_id = $1 ORDER BY c.sort_order ASC, c.code ASC`.
   - Обновить интерфейс `CreatorApplicationCategoryRepo`.
7. **Domain.** В `backend/internal/domain/creator_application.go`:
   - Удалить `CreatorApplicationDetailCategory`.
   - `CreatorApplicationDetail.Categories []string`.
   - Остальное без изменений.
8. **Service.** В `backend/internal/service/creator_application.go`:
   - В `creatorApplicationDetailFromRows` сигнатура для категорий — `[]string`, маппинг — `append([]string(nil), categories...)`.
   - `GetByID` соответственно: `categoryRows, err := ...ListByApplicationID(...)` — `categoryRows` теперь `[]string`.

### Phase 3 — Backend: handler + dictionary mapping

9. **Handler.** В `backend/internal/handler/creator_application.go`, в `GetCreatorApplication`, после `service.GetByID` и до `respondJSON`:
   ```go
   categoryEntries, err := s.dictionaryService.List(r.Context(), domain.DictionaryTypeCategories)
   if err != nil { respondError(...); return }
   cityEntries, err := s.dictionaryService.List(r.Context(), domain.DictionaryTypeCities)
   if err != nil { respondError(...); return }
   ```
   - Построить `categoriesByCode := make(map[string]domain.DictionaryEntry, len(categoryEntries))` (тип элемента — фактический; смотреть `domain.DictionaryEntry`).
   - То же для `cityByCode`.
   - Передать карты в `domainCreatorApplicationDetailToAPI(id, detail, categoriesByCode, cityByCode)`.
10. **Маппер.** Обновить `domainCreatorApplicationDetailToAPI`:
    - Categories: для каждого code из `detail.Categories` ищем `entry, ok := categoriesByCode[code]`. Если `ok` — `{code, name: entry.Name, sortOrder: entry.SortOrder}`; иначе `{code, name: code, sortOrder: 0}`.
    - Сортировать получившийся слайс по `(sortOrder ASC, code ASC)` через `slices.SortFunc`.
    - City: тот же fallback, объект `api.CreatorApplicationDetailCity{Code, Name, SortOrder}`.
11. **Handler unit-тесты.** В `backend/internal/handler/creator_application_test.go`:
    - В `serverWithAuthzAndCreator` (или новом `serverWithAuthzAndCreatorAndDict`) принять и встроить `DictionaryService`.
    - В success-сценарии: настроить `MockDictionaryService.EXPECT().List(mock.Anything, domain.DictionaryTypeCategories).Return([]domain.DictionaryEntry{...}, nil)` и то же для cities.
    - Обновить ожидаемый response: `city: api.CreatorApplicationDetailCity{Code, Name, SortOrder}`, `categories[]` — резолвленные.
    - Новые сценарии:
      - `success with deactivated category falls back to code`: словарь не содержит одного из кодов; ожидаем `{code, name: code, sortOrder: 0}`.
      - `dictionary list error → 500`: первый `List` возвращает error; ожидаем 500 + `expectHandlerUnexpectedErrorLog`.
      - `dictionary cities error → 500`: первый `List` ok, второй — error; ожидаем 500.

### Phase 4 — Backend: regen mocks + unit tests + e2e

12. `make generate-mocks` — подхватит обновлённые интерфейсы repo. Проверить `git status` — затронуты только ожидаемые моки.
13. **Repo unit-тест.** Обновить `creator_application_category_test.go`: новый SQL-литерал, ожидаемый результат `[]string{"beauty","fashion"}`. Сценарии: success, empty, error.
14. **Service unit-тест.** Обновить `creator_application_test.go` (service) — `appCategoryRepo.ListByApplicationID` мок возвращает `[]string{...}`. В success-сценарии expected `Categories: []string{...}`.
15. **E2E helper.** В `backend/e2e/creator_application/creator_application_test.go`:
    - `expectedCreatorApplication.City` — `string` (отправляемый код).
    - `verifyCreatorApplicationByID`:
      - `require.Equal(t, expected.City, got.City.Code)`.
      - `require.NotEmpty(t, got.City.Name)`.
      - `require.Equal(t, expected.City, ...)` для кода в результате.
      - `got.Categories[]` — проверка ElementsMatch по `Code`, `Name` непустой, sort_order проверка.
    - В каждом тесте, который вызывает `verifyCreatorApplicationByID`, передавать ожидаемый код города (`"almaty"`).
16. Прогон: `make test-unit-backend`, `make test-unit-backend-coverage`, `make test-e2e-backend` (**двойной прогон**). Все зелёные.
17. `make lint-backend`, `make build-backend` — зелёные. `make build-web` и `make build-tma` — зелёные (фронты подхватывают новый тип city, не должны сломаться, т.к. admin GET ими ещё не используется).

### Phase 5 — Landing: codegen wiring

18. **Makefile.** В цель `generate-api` добавить:
    ```makefile
    cd frontend/landing && npx openapi-typescript ../../backend/api/openapi.yaml -o src/api/generated/schema.ts
    ```
    Поместить рядом со строками для web/tma.
19. **Зависимости — npm workspace.** Lock-файл живёт в `frontend/package-lock.json` (workspace в корне `frontend/`). Установить через workspace-команду из `frontend/`:
    ```bash
    cd frontend && npm install -w landing openapi-fetch openapi-typescript
    ```
    Это обновит `frontend/package.json` (root), `frontend/landing/package.json` и общий `frontend/package-lock.json`. Версии — те же мажорные, что в `frontend/web/package.json` (`openapi-typescript ^7.13.0`, `openapi-fetch ^0.17.0`). Установка добавляет их в `devDependencies` лендинга.
20. **Запуск `make generate-api`** — должен создаться `frontend/landing/src/api/generated/schema.ts`. Проверить, что файл не пустой и содержит ожидаемые `paths`.

### Phase 6 — Landing: typed API client (повторить структуру `frontend/web/src/api/`)

21. **Создать `frontend/landing/src/api/client.ts`** — копия `frontend/web/src/api/client.ts` строк 1-25 (импорты, `getApiBase`, `apiBase`, `ApiError`) плюс единственный `const client = createClient<paths>({ baseUrl: BASE });` (без `credentials: "include"`, без `client.use(...)`, без `rawClient`/`refreshToken`):
    - Импорты: `createClient` из `openapi-fetch`, `paths` из `./generated/schema`. **НЕ** импортировать `useAuthStore` (его нет в лендинге).
    - `getApiBase()` — буквальная копия `frontend/web/src/api/client.ts:5-10`: возвращает `window.__RUNTIME_CONFIG__?.apiUrl ?? "/api"`. Лендинг уже использует этот механизм (`<script is:inline src="/config.js">` в `index.astro:26`).
    - `export const apiBase = getApiBase()` (как в web).
    - `class ApiError extends Error` — точная копия web строк 15-25.
    - `const client = createClient<paths>({ baseUrl: BASE })` — default-export.
22. **Создать `frontend/landing/src/api/dictionaries.ts`** — паттерн `frontend/web/src/api/audit.ts`:
    - Импортировать `components, paths` из `./generated/schema`.
    - Импортировать `client, ApiError` из `./client`.
    - Локальный `extractErrorCode` (повторить web `audit.ts:6-9`).
    - Экспорт типов: `DictionaryEntry = components["schemas"]["DictionaryEntry"]` и `DictionaryType` через `paths` (если в схеме нет именованного типа — извлечь как `paths["/dictionaries/{type}"]["get"]["parameters"]["path"]["type"]`).
    - `export async function listDictionary(type: DictionaryType)` — `client.GET("/dictionaries/{type}", { params: { path: { type } } })`. На `error` — `throw new ApiError(response.status, extractErrorCode(error))`. На success — `return data`.
23. **Создать `frontend/landing/src/api/creator-applications.ts`** — паттерн `frontend/web/src/api/brands.ts:31-39`:
    - Импортировать `components` из `./generated/schema`.
    - Импортировать `client, ApiError` из `./client`.
    - Локальный `extractErrorCode`.
    - Экспорт типа `CreatorApplicationSubmitRequest = components["schemas"]["CreatorApplicationSubmitRequest"]`.
    - `export async function submitCreatorApplication(payload: CreatorApplicationSubmitRequest)` — `client.POST("/creators/applications", { body: payload })`. Та же error/success обработка.
24. **Перестроить `<script>`-блок в `index.astro`** (это самый объёмный шаг — учти REQ-B8):
    - Сейчас весь form-runtime сидит в `<script define:vars={{ socialCodeMap, otherCategoryCode, maxCategories }}>` (строка ~640). `define:vars` рендерит inline-script без module-обработки — `import` не работает.
    - План:
      1. Сериализовать константы в встроенный JSON-блок: `<script type="application/json" id="form-config">{JSON.stringify({ socialCodeMap: formBlock.socialPlatformCodes, otherCategoryCode: "other", maxCategories: 3 })}</script>` сразу перед основным `<script>`.
      2. Удалить `define:vars` с основного `<script>` тега.
      3. В начале основного `<script>` прочитать конфиг: `const config = JSON.parse(document.getElementById("form-config")!.textContent!);` и распаковать константы.
      4. Теперь Astro собирает `<script>` через Vite как ES module — добавить импорты `listDictionary`, `submitCreatorApplication`, `ApiError` из `../api/dictionaries`, `../api/creator-applications`, `../api/client`.
      5. `loadDictionary(type)` (строка ~711) → вызов `listDictionary(type)` напрямую (вернёт типизированный `data` объект с `items`); вытащить `.data.items` или эквивалент по типу.
      6. Submit handler (строка ~823) — обернуть в `try { const result = await submitCreatorApplication(payload); showSuccess(result.data.telegramBotUrl); } catch (e) { if (e instanceof ApiError) { showFormError(messageForCode(e.code)); } else { showFormError("Сеть недоступна. Попробуйте позже."); } }`. Сообщения — те же, что сейчас в коде (`body?.error?.message ?? "Не удалось отправить заявку"` → словарь `messageForCode`).
    - Альтернатива (если проще): вынести содержимое `<script>` в отдельный модуль `frontend/landing/src/scripts/form.ts` и подключить через `<script>import "../scripts/form.ts"</script>` (Astro обработает). Константы тогда передавать через data-атрибуты на `<form>` или через тот же `application/json` блок. Выбор между in-page module и external — на усмотрение исполнителя; результат тот же.
    - Никаких inline `fetch(...)` после правки.
25. `make lint-landing` — зелёный (`tsc --noEmit` поймает любые рассинхрон-ы между `paths` и фактическими вызовами).
26. `make test-e2e-landing` (через docker compose, или локально через playwright webServer — оба способа описаны в `frontend/landing/playwright.config.ts`) — зелёный.
27. `make build-landing` — зелёный.

### Phase 7 — CI verification

28. **Прочитать `.github/workflows/ci.yml` целиком** — зафиксировать факты:
    - **Уже существуют:** `lint-landing` (строки 60-72), `test-unit-landing` (строки 144-156), `test-e2e-landing` в Stage 2 (строки 278-321, изолированный docker compose), `migrate-staging`/`deploy-staging` (Stage 3).
    - **Отсутствует:** job `test-staging-landing` в Stage 4 — рядом с `test-staging-backend` и `test-staging-web` нет аналога для лендинга. **План добавляет его** (REQ-C3).
    - **Diff-check сгенерированных файлов** (`make generate-api` + `git diff --exit-code`) — отсутствует. Не добавляется этим слайсом.
29. **Добавить `test-staging-landing` job** в `.github/workflows/ci.yml` (Stage 4, рядом с `test-staging-web`). Структура — буквально зеркало `test-staging-web` (`.github/workflows/ci.yml:393-419`, проверь актуальные номера) с заменами:
    - `name: Staging E2E Landing`.
    - `npm ci -w landing` (вместо `-w web`).
    - `working-directory: frontend/landing`.
    - `BASE_URL: https://staging.ugcboost.kz` (подтверждено Alikhan'ом).
    - `API_URL: https://staging-api.ugcboost.kz`.
    - `CF_ACCESS_CLIENT_ID`/`CF_ACCESS_CLIENT_SECRET` — те же существующие secrets.
    - Artifact upload: `playwright-report-staging-landing` (отличается от `playwright-report-staging` web'а, чтобы не конфликтовали).
    - `needs: [migrate-staging, deploy-staging]`.
30. **Локальная симуляция критичных шагов CI** (для Stage 1-2; Stage 4 staging-jobs локально не симулируются):
    ```bash
    make compose-up
    make migrate-up
    make build-backend && make lint-backend
    make test-unit-backend && make test-unit-backend-coverage
    make test-e2e-backend  # дважды для идемпотентности
    make build-web && make lint-web        # схема обновилась — фронты не должны сломаться
    make build-tma && make lint-tma
    make build-landing && make lint-landing
    make test-unit-landing
    make test-e2e-landing
    ```
    Все зелёные. Если хоть одна красная — фиксить и не идти к Phase 8.
31. **Финальный grep.** В `frontend/landing/src/` не должно быть ни одного `fetch(` API-вызова. Команда: `grep -rn "fetch(" frontend/landing/src/`. Если результат пуст — ОК. Импорт `createClient` из `openapi-fetch` в `src/api/client.ts` — это `createClient`, а не `fetch(`, под grep не подпадает.

### Phase 8 — Сводка

32. Подготовить сводку для Alikhan: что сделано, какие файлы изменены, результаты тестов, отклонения от плана. **Никаких автокоммитов** — оставить всё в working tree до ручного ревью.

## Стратегия тестирования

### Backend unit

- **Repo categories:** SQL-литерал в тесте обновлён (`SELECT c.code FROM ... JOIN categories c ON ... ORDER BY c.sort_order ASC, c.code ASC`), ожидаемый результат `[]string{"beauty","fashion"}`. Сценарии: success, empty, error.
- **Service `GetByID`:** мок `appCategoryRepo.ListByApplicationID` отдаёт `[]string`, ожидаемый domain — `Categories: []string`. Все сценарии из текущего теста сохранить, обновить ожидания только в success.
- **Handler `GetCreatorApplication`:**
  - `forbidden` — без изменений.
  - `not found` — без изменений (вызов dict не происходит — service возвращает `sql.ErrNoRows` до dict-резолва).
  - `service error → 500` — без изменений.
  - `success` — добавить два мока `MockDictionaryService.EXPECT().List(...)`; expected response — с резолвленными city и categories.
  - **Новые:** `success with deactivated category falls back to code`, `dictionary categories error → 500`, `dictionary cities error → 500`.
- **Authz** — без изменений.

### Backend E2E

- Все существующие тесты creator_application + 2 новых GET-теста. Helper `verifyCreatorApplicationByID` — единая точка изменений.
- Тест `TestGetCreatorApplicationNotFound` — проверить, что dict-вызовы НЕ происходят при 404 (service возвращает `sql.ErrNoRows` раньше). Это уже соблюдается за счёт раннего `respondError`.

### Frontend landing

- **Unit (если Vitest существует в landing):** обновить тесты под новые типы.
- **E2E** (`frontend/e2e/landing/`): должны пройти без изменений, если контракт API не сломался. Запросы те же, шейп ответа dictionary тот же, шейп submit-payload тот же. Если падают — диагностировать.

### Что не должно сломаться

- `frontend/web` и `frontend/tma`: `schema.ts` обновится (city теперь объект). Если они где-то используют `creator_applications.city` — проверить (вероятно, нет, GET ещё не используется UI). `make build-web` и `make build-tma` — зелёные.
- Backend E2E (другие домены): `auth`, `brand`, `audit-logs`, `dictionary` — изменения изолированы в creator_application слой и handler/service/repo creator_application.

## Edge cases

- **Деактивированная категория/город в исторической заявке.** `DictionaryService.List` (с `WHERE active = TRUE`) их не вернёт. Handler делает fallback `{code, name: code, sortOrder: 0}`. Покрывается тестом.
- **Код в БД, которого никогда не было в словаре.** Та же ветка fallback. Не паника, не 500.
- **Пустой список категорий.** На POST минимум 1 категория. Defensively repo возвращает `[]string{}`, handler — `[]CreatorApplicationDetailCategory{}` (НЕ nil — для JSON-симметрии).
- **Параллельные `DictionaryService.List`.** Можно через goroutines, но это микро-оптимизация для двух мелких SELECT'ов. Out of scope.
- **Astro `<script define:vars={...}>` блокирует ES-imports.** Текущий `<script>` в `index.astro:640` использует `define:vars` — Astro рендерит его как inline-script без module-обработки, импорты не работают. Решение в Phase 6 шаг 24: убрать `define:vars`, передать константы через `<script type="application/json" id="form-config">`-блок, тогда основной `<script>` обрабатывается Vite как ES module и импорты заработают. Альтернатива — вынести JS в отдельный `frontend/landing/src/scripts/form.ts`. Это самый объёмный рефакторинг в слайсе B; недооценка ведёт к застреванию.
- **Дубликаты записей в словаре с одним кодом** (не должно быть, есть UNIQUE constraint в миграциях). Если случится — handler возьмёт последний по итерации; это OK для fallback-поведения.
- **`make generate-api` на чистой среде** (без `node_modules` в `frontend/landing`): `npx openapi-typescript` упадёт. Фиксится `npm install` в Phase 5 шаг 20. Документировать в сводке, если CI запускается на чистой среде.

## Оценка рисков

| Риск | Вероятность | Митигация |
|------|-------------|-----------|
| Изменение шейпа `city` в OpenAPI ломает фронт-приложения | Низкая | Эта ручка admin-only, frontend admin UI ещё не реализован. `make build-web/tma` всё равно прогоняем. |
| Регенерация задевает что-то лишнее в `*.gen.go` | Низкая | После каждого `make generate-api` — `git diff`, проверить scope. |
| `openapi-typescript`/`openapi-fetch` для лендинга упирается в монорепо-нюансы (npm workspaces) | Средняя | Lock-файл монорепо — `frontend/package-lock.json`; установка через `cd frontend && npm install -w landing ...`. CI использует `npm ci -w landing` (`working-directory: frontend`) — этот путь уже работает для web/tma. |
| Astro `<script define:vars={...}>` ломает попытку добавить `import` (БЛОКЕР) | **Высокая** (если не учесть) | Phase 6 шаг 24 явно убирает `define:vars` и переносит константы в `<script type="application/json" id="form-config">`. Основной `<script>` становится ES-модулем, который обрабатывается Vite. Без этого шага импорт API не подключить. |
| Astro `<script>` (после фикса) не находит модуль `../api/client.ts` | Низкая | Astro/Vite поддерживает относительные импорты в module-`<script>`. Если не работает — вынести в `src/scripts/form.ts` и подключить через `<script>import "../scripts/form.ts"</script>`. |
| Coverage <80% per-method после рефакторинга handler-маппера | Средняя | Покрыть fallback-ветки + dict-error явными тестами (Phase 3 шаг 11). |
| `DictionaryService.List` выдаёт большой словарь для каждого GET | Низкая | Сейчас одиночная ручка, не listing. При вводе списка — кэшировать или менять подход. Зафиксировать как known limitation в финальной сводке. |
| Staging URL лендинга `https://staging.ugcboost.kz` оказался временно недоступен (Cloudflare Access misconfigure) | Низкая | Прогон `test-staging-landing` упадёт с понятной ошибкой; secret-headers те же, что для web. Если упало — проверить `vars`/`secrets` в Settings → Actions, подтвердить что Dokploy деплой landing'а зелёный. |
| `tsc --noEmit` ловит расхождение между `paths` и реальными вызовами в `client.ts` | Высокая (по дизайну, это feature) | Это и есть смысл миграции; чинить вызовы по типам. |
| `make test-e2e-landing` падает из-за смены контракта (хотя контракт сохранён) | Очень низкая | Контракт идентичен; форма payload и dictionary не меняется. Если падает — сразу диагностировать. |
| Неактуальный submit POST в admin GET (он не меняется планом) | Очень низкая | План не трогает submit. |

## План отката

Слайс A (handler-маппинг): атомарный рефакторинг. Откат — `git revert` коммита/серии. После деплоя — admin UI ещё не использует ручку, downtime нулевой.

Слайс B (landing codegen): тоже атомарно. Откат — вернуть `index.astro` к `fetch`, удалить строку из Makefile, удалить `src/api/`.

БД-миграций нет — откат БД не нужен.

### Сигналы early-stop

- `make generate-api` ругается на YAML или дублирует операции — стоп, разбираться.
- После Phase 3 шага 9 `make build-backend` падает с unresolved `dictionaryService.List` — проверить `handler/server.go:59-61` (интерфейс `DictionaryService`).
- После Phase 5 шага 20 `npx openapi-typescript` ругается на отсутствующий пакет — `npm install -w landing` (Phase 5 шаг 19) не отработал; проверить, что в `frontend/landing/package.json` появились новые devDeps и что `frontend/package-lock.json` обновился.
- После Phase 6 шага 24 в браузере вылетает `Cannot use import statement outside a module` — `define:vars` всё ещё на тэге, либо тег `<script is:inline>`. Удалить `define:vars`/`is:inline` с основного блока (REQ-B8).
- `make lint-landing` падает в `tsc --noEmit` сразу после Phase 6 шагов 21-23 — типы из `paths` не резолвятся; проверить, что `frontend/landing/src/api/generated/schema.ts` создан и не пустой.
- `make test-unit-backend` стал красным после регенерации моков, причём не из-за наших новых тестов — стоп, разобраться.
- `make test-e2e-landing` падает на тестах, которые не трогали — скорее всего что-то сломалось в landing-side контракте; диагностировать сразу.

## ENV / Staging

**Никаких новых env vars и secrets не требуется.** Все существующие переменные и secrets уже на месте.

- Бэкенд: `DictionaryService` уже работает с БД, словари `categories` и `cities` seeded миграциями. Существующие env (`LEGAL_AGREEMENT_VERSION`, `LEGAL_PRIVACY_VERSION`, `TELEGRAM_BOT_USERNAME` и т.д.) — без изменений.
- Лендинг: использует `apiUrl` через `<script is:inline src="/config.js">` — без изменений.
- CI Stage 1-3: без изменений.
- CI Stage 4 (`test-staging-landing` — новый job): использует существующие secrets `CF_ACCESS_CLIENT_ID`/`CF_ACCESS_CLIENT_SECRET` (уже есть в репо) и захардкоженные хосты `BASE_URL: https://staging.ugcboost.kz` + `API_URL: https://staging-api.ugcboost.kz`.

**Alikhan на staging менять не нужно ничего** — лендинг уже задеплоен на `https://staging.ugcboost.kz`, secrets для Cloudflare Access уже подключены к workflow.

## Память проекта (учёт правил)

- "Audit vs stdout-логи" — план не добавляет stdout-логов с ПД.
- "Не делать коммиты автоматически" — план НЕ коммитит. Все изменения остаются в working tree.
- "Не мержить PR" — Claude не мержит PR.
- "Доводить задачи до конца" — все 8 фаз выполнить в одной сессии.
- "Локальная проверка перед пушем" — Phase 7 явно прогоняет всё локально.
- "Скорость > идеальная документация" — план не добавляет лишних абстракций; маппинг живёт в одном handler-методе, без middleware/декораторов.
- "Тесты по слоям" — unit тесты на handler/service/repo/authz; E2E через `make test-e2e-backend` и `make test-e2e-landing`.
- "Стандарты как источник истины" — Phase 1 шаг 1 явно требует прочитать ВСЕ стандарты целиком.
