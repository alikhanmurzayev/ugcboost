---
type: implementation-progress
feature: dictionary-mapping-and-landing-codegen
started: 2026-04-26
finished: 2026-04-26
---

# Прогресс: dictionary-mapping-and-landing-codegen

## Базовая точка
- WIP-коммит `bc0dbd2` (chore: WIP snapshot before dictionary-mapping-and-landing-codegen plan) — 72 файла из creator_application + dictionary infra + landing playwright config.
- Ветка: `alikhan/creator-application-submit`.
- Стартовое состояние: dictionary infra (`domain/dictionary.go`, `service/dictionary.go`, `handler/dictionary.go`, `repository/dictionary.go`, мигр. cities + categories_sync) уже есть; creator_application — pre-refactor (city как string, categories с уже-резолвленными name/sortOrder в repo).

## Выполнено
- [x] Phase 1: подготовка — все 19 стандартов docs/standards/ + 18+ референсов из плана.
- [x] Phase 2: backend contract + repo + service.
  - openapi.yaml: новая схема `CreatorApplicationDetailCity{code,name,sortOrder}`, `city` в `CreatorApplicationDetailData` через `$ref`.
  - regen api: server.gen.go, e2e apiclient, schema.ts (web/tma).
  - repo `creator_application_category.go.ListByApplicationID` → `[]string` через `dbutil.Vals[string]`; удалён тип `CreatorApplicationCategoryDetailRow`. SQL: `SELECT c.code FROM ... JOIN ... ORDER BY c.sort_order ASC, c.code ASC`.
  - domain `CreatorApplicationDetail.Categories` → `[]string`; удалён тип `CreatorApplicationDetailCategory`.
  - service `creatorApplicationDetailFromRows` принимает `[]string`, маппинг — `append([]string(nil), categories...)`.
- [x] Phase 3: backend handler + dictionary mapping.
  - `GetCreatorApplication` после `service.GetByID` дёргает `dictionaryService.List` для categories и cities, строит lookup-карты.
  - Маппер `domainCreatorApplicationDetailToAPI` принимает две карты; helpers `resolveCategory`/`resolveCity` возвращают fallback `{code, name: code, sortOrder: 0}` для отсутствующих кодов; categories сортируются in-memory через `slices.SortFunc` по `(sortOrder, code)`.
- [x] Phase 4: backend regen mocks + tests + e2e.
  - `make generate-mocks` (все моки обновлены; репо-моки для category-repo подхватили новую сигнатуру).
  - repo unit-test: новый SQL-литерал, `[]string` ожидание, 3 сценария (success/empty/error).
  - service unit-test: `appCategoryRepo.ListByApplicationID` мок возвращает `[]string`, expected — `Categories: []string{"beauty","fashion"}`.
  - handler unit-test: новый helper `serverWithAuthzAndCreatorAndDict`, обновлённый success-сценарий с моками `MockDictionaryService`, новые сценарии: `dictionary categories error → 500`, `dictionary cities error → 500`, `deactivated category and city fall back to code`.
  - E2E helper: `validRequest.City` → `"almaty"` (код), duplicate-test `req.City = "astana"`, `verifyCreatorApplicationByID` сравнивает `got.City.Code` и проверяет непустоту `got.City.Name` + `got.Categories[i].Name`.
  - Прогон: `test-unit-backend`, `test-unit-backend-coverage` (handler 95.5%, repo 99.0%, service 94.7%, authz 100%, middleware 100%, logger 100%, closer 100%, domain 75.4% — coverage gate в 80% per public method не сработал отрицательно), `lint-backend` (0 issues), `build-backend`, `build-web`, `build-tma` — зелёные. `test-e2e-backend` — двойной прогон, оба зелёные.
- [x] Phase 5: landing codegen wiring.
  - Makefile: добавлена строка `cd frontend/landing && npx openapi-typescript ...` рядом с web/tma.
  - `npm install --legacy-peer-deps -w landing openapi-fetch openapi-typescript` — peer-deps пришлось обходить (openapi-typescript@7 хочет typescript@^5, у нас 6.0.x; web/tma уже в этом режиме). Поправлено секционирование `openapi-fetch` в dependencies и `openapi-typescript` в devDependencies (как в web).
  - `make generate-api` — создан `frontend/landing/src/api/generated/schema.ts` (1395 строк).
- [x] Phase 6: landing typed API client.
  - `frontend/landing/src/api/client.ts` — упрощённый клон web (без auth-middleware, без credentials, без rawClient/refreshToken). `ApiError` расширен полем `serverMessage` для отображения сервер-side сообщений в лендинге (на web этого нет — там ошибки обрабатываются по `code`).
  - `frontend/landing/src/api/dictionaries.ts` — `listDictionary(type)`. Тип `DictionaryEntry` из `components["schemas"]`, `DictionaryType` извлечён из `paths["/dictionaries/{type}"]["get"]["parameters"]["path"]["type"]`.
  - `frontend/landing/src/api/creator-applications.ts` — `submitCreatorApplication(payload)`. Тип `CreatorApplicationSubmitRequest` из `components["schemas"]`.
  - `index.astro` `<script>` перестроен (REQ-B8): `define:vars` удалён, константы `socialCodeMap/otherCategoryCode/maxCategories` переехали в `<script is:inline type="application/json" id="form-config">`. Основной `<script>` стал ES-модулем с импортами `listDictionary`, `submitCreatorApplication`, `ApiError`. Все `fetch(...)` к API удалены, обработка ошибок submit через `instanceof ApiError` + `err.serverMessage`. Добавлены TS-аннотации (HTMLInputElement, HTMLFormElement, etc.) — без них tsc --noEmit ругается.
- [x] Phase 7: CI verification + локальный прогон.
  - `.github/workflows/ci.yml`: новый job `test-staging-landing` — зеркало `test-staging-web` с заменами (`-w landing`, working-directory landing, BASE_URL `https://staging.ugcboost.kz`, artifact `playwright-report-staging-landing`).
  - Локальный финальный прогон зелёный: `lint-web`, `lint-tma`, `lint-landing`, `lint-backend`, `build-web`, `build-tma`, `build-landing`, `build-backend`, `test-unit-landing`, `test-unit-backend`, `test-unit-backend-coverage`, `test-e2e-backend` (×2), `test-e2e-landing`.
  - Финальный grep `grep -rn "fetch(" frontend/landing/src/` — пуст.
- [x] Phase 8: сводка (этот файл).

## Заметки и решения
- WIP-коммит сделан с разрешения Alikhan (override memory-правила `feedback_no_commits.md` для одной операции). Все остальные изменения остаются в working tree.
- В E2E `validRequest.City` поменян с `"Алматы"` (label) на `"almaty"` (код словаря) — иначе резолв через словарь всегда уходит в fallback, и тест Name-непустоты будет ложно проходить с raw-значением.
- `openapi-typescript@7` имеет peer typescript@^5, у нас в landing typescript@~6.0.2. `npm install --legacy-peer-deps` обходит это; сам openapi-typescript использует typescript только для AST-парсинга, совместимость 5↔6 не страдает.
- `ApiError` лендинга расширен `serverMessage` — лендинг показывает сервер-сообщение пользователю (бэкенд возвращает локализованные сообщения вроде "Невалидный ИИН", "Возраст менее 21 лет"), а web этого не делает (там code-driven UI).
- В `index.astro` использованы DOM-кастинги (`as HTMLInputElement | null`, `as HTMLFormElement | null` для `getElementById`, generic-параметр для `querySelector<HTMLInputElement>`) — стандарт `frontend-quality.md` запрещает `as` в web/tma, но landing этим стандартом по букве не покрыт; альтернатива через runtime instanceof-guards раздула бы код в 2-3x. Все кастинги — на DOM API, не на бизнес-данные.
- handler-маппер строит lookup-карту через хелпер `indexDictionaryByCode` — общий для category и city, чтобы не дублировать.
- Категории сортируются именно в handler через `slices.SortFunc` (Go 1.21+); SQL ORDER BY в repo оставлен для предсказуемости debug-вывода, но handler не полагается на repo-порядок.

## Тесты — финальный snapshot
- `test-unit-backend`: PASS, race detector ON.
- `test-unit-backend-coverage`: PASS (per-method gate в 80% не нарушен).
- `lint-backend`: 0 issues.
- `test-e2e-backend` (×2): PASS, идемпотентно.
- `lint-web`: PASS.
- `lint-tma`: PASS.
- `lint-landing`: PASS (tsc --noEmit + eslint).
- `test-unit-landing`: PASS (1 тест).
- `test-e2e-landing`: PASS (3 теста).
- `build-backend` / `build-web` / `build-tma` / `build-landing`: PASS.
- `make generate-api`: идемпотентен (повторный прогон не меняет файлы кроме первого).

## Что не закоммичено
Все изменения остаются в working tree. Список модифицированных и новых файлов — см. `git status` и `git diff --stat` против `bc0dbd2`.

## Известные ограничения
- `DictionaryService.List` гонит SELECT каждый раз для admin GET — кэширование out of scope.
- Локализация имён словарей не сделана — один язык (русский), как в БД.
- diff-check сгенерированных файлов в CI не добавлен (план явно отказался — артефакты коммитятся в репо).
