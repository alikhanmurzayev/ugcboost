---
type: implementation-plan
feature: creator-application-get
created: 2026-04-25
---

# План реализации: GET /creators/applications/{id} (admin-only)

## Перед началом — обязательно

Этот план — самодостаточная инструкция. До первой строчки кода:

1. **Прочитай ВСЕ файлы из `docs/standards/` целиком, в сыром виде, без выборки.** Это обязательные стандарты проекта (архитектура, кодген, репозитории, транзакции, тесты, безопасность, неминг). Источник истины. Запускай `ls docs/standards/`, потом читай каждый файл — это критично, иначе нарушишь конвенции, которые здесь не повторяются.
2. **Прочитай файлы-референсы из секции "Полезные референсы в кодбазе" ниже** — это образцы, по которым ты будешь писать новый код. Не пересказ — открывай и читай каждый указанный файл с указанными line-numbers.

Без этого подготовительного этапа не начинай реализацию. План ниже описывает направление, последовательность и зафиксированные решения, но не дублирует ни стандарты, ни сигнатуры — их источник в стандартах и в существующем коде.

## Обзор

Новый admin-only HTTP-эндпоинт `GET /creators/applications/{id}` возвращает полный aggregate заявки (главная заявка + категории + соцсети + согласия). Закрывает пробел в backend E2E: после POST `/creators/applications` дёргаем GET и сверяем все данные, реально записанные в 5 таблиц (`creator_applications` + `_categories` + `_socials` + `_consents` + `audit_logs`). Слайс из Epic 2 (модерация креаторов), но поверхность минимальная — без листинга, без approve/reject, только чтение по ID.

## Требования

### Must-have

- **REQ-1.** Новый OpenAPI-эндпоинт `GET /creators/applications/{id}` с `security: bearerAuth`. Ответы 200 / 401 / 403 / 404 / default.
- **REQ-2.** RBAC strict — только роль `admin` через новый authz-метод `CanViewCreatorApplication`. Все остальные → 403. Без токена → 401 (автоматически от middleware при наличии `bearerAuth` в OpenAPI, см. "Зафиксированные решения" ниже).
- **REQ-3.** Response — полный aggregate: ФИО (с опциональным `middleName`), `iin`, `birthDate`, `phone`, `city`, `address`, `categoryOtherText?`, `status`, `createdAt`, `updatedAt`, плюс развёрнутые `categories[]` (`code/name/sortOrder`), `socials[]` (`platform/handle`), `consents[]` (`consentType/acceptedAt/documentVersion/ipAddress/userAgent`).
- **REQ-4.** Сортировка response детерминированная: `categories` — `sort_order ASC, code ASC`; `socials` — `platform ASC, handle ASC`; `consents` — in-memory по фиксированному порядку `domain.ConsentTypeValues` (`processing → third_party → cross_border → terms`), независимо от порядка из БД.
- **REQ-5.** Несуществующий валидный UUID → 404 `NOT_FOUND`. ID-параметр в path берётся напрямую из ServerInterfaceWrapper (никакого ручного `chi.URLParam`).
- **REQ-6.** Все слои покрыты unit-тестами: handler / service / repository / authz. Целевой порог coverage ≥80% per-method (стандарт `backend-testing-unit.md`).
- **REQ-7.** Backend E2E расширены: после POST в трёх happy-path тестах (`Submit`, `Other`, `Threads`) дёргаем GET через admin-токен и сверяем aggregate целиком. Плюс два новых теста: `Forbidden` (brand_manager-токен), `NotFound` (admin + несуществующий валидный UUID).
- **REQ-8.** ID берётся из POST-response поля `applicationId` (UUID-формат) напрямую — никакого парсинга `telegramBotUrl`. Поле уже есть в текущей схеме (`backend/api/openapi.yaml:1035-1046`, `CreatorApplicationSubmitData`).
- **REQ-9.** ПД (ИИН/телефон/адрес/имя) **не попадают в stdout-логи** ни на одном слое (см. `security.md`). Audit на read-операцию **не пишется**.

### Out of scope

- GET-листинг заявок (`/creators/applications` без ID).
- Approve / reject / status-машина и связанные Telegram-уведомления.
- Frontend admin UI.
- Pagination, фильтры, поиск.
- Маскирование ПД для не-admin ролей (роли просто получают 403).
- Cache-Control headers (отложено).

### Критерии успеха

В финале должны быть зелёными:
- `make test-unit-backend`
- `make test-unit-backend-coverage`
- `make test-e2e-backend` (прогнать дважды подряд для проверки идемпотентности)
- `make lint-backend`
- `make build-backend`

E2E должны явно покрывать: 401 без токена, 403 с brand_manager-токеном, 404 на несуществующий UUID, 200 с полным aggregate'ом для admin'а.

## Зафиксированные решения

- **Path:** `/creators/applications/{id}` (зеркалит POST `/creators/applications`). Без `/admin/*` префикса — текущий проект (`/brands`, `/audit-logs`) тоже admin без префикса.
- **Auth:** `security: - bearerAuth: []` в OpenAPI + новый authz-метод `CanViewCreatorApplication` (admin-only). В проекте middleware `AuthFromScopes` (`backend/internal/middleware/auth.go:85-124`) проверяет токен **только если в OpenAPI есть `bearerAuth`** (через `api.BearerAuthScopes` context value, который выставляет сгенерированный wrapper). То есть объявление в OpenAPI автоматически даёт 401 при отсутствии/невалидном токене; authz даёт 403 при wrong role. Не нужно ничего руками регистрировать в роутере.
- **Response shape:** полный aggregate, развёрнутые consents (с `document_version`/`ip_address`/`user_agent`/`accepted_at`/`consent_type`). `birthDate` тоже включаем — admin-полезное поле, в БД оно хранится отдельно (вычисляется из ИИН на POST).
- **Read pattern:** read-only через `s.pool` без `WithTx`. 4 последовательных SELECT (главная заявка → категории → соцсети → согласия). Параллельность через goroutines не нужна — холодный путь.
- **Categories JOIN:** `INNER JOIN categories` без фильтра по `active` — категории не удаляются жёстко (только soft-deactivation), а по дизайну деактивированная категория всё равно должна вернуться в исторической заявке.
- **404-маппинг:** в проекте нет `domain.NewNotFoundError`. `respondError` (`backend/internal/handler/response.go:50-51`) уже мапит и `domain.ErrNotFound`, и `sql.ErrNoRows` → 404. Service просто пробрасывает `sql.ErrNoRows` от main repo дальше — без обёртки или с обёрткой через `%w` (но не `%v` — иначе `errors.Is` сломается).
- **Audit:** на read-операцию **не пишется** — это не изменение состояния.
- **Sort consents — implementation:** при маппинге service строит `map[consentType]row` и итерирует по `domain.ConsentTypeValues`, складывая в результат. Отсутствующие типы пропускать без ошибки (insert-batch на POST атомарно создаёт все 4, но read-сторона не должна падать на edge case).
- **Миграции — не нужны.** Все таблицы и колонки уже на месте (см. миграции `backend/migrations/20260420181753_*` … `20260420181756_*` + `20260425205627_creator_application_category_other.sql`).

## Файлы для изменения

| Файл | Изменения |
|------|-----------|
| `backend/api/openapi.yaml` | Новый path `/creators/applications/{id}` с GET-операцией + 5 новых схем под aggregate. |
| `backend/internal/api/server.gen.go` | Регенерация (`make generate-api`). |
| `backend/e2e/apiclient/{client,types}.gen.go` | Регенерация. |
| `frontend/{web,tma}/src/api/generated/schema.ts` | Регенерация (тип-only, в runtime не используется в этом слайсе). |
| `backend/internal/domain/creator_application.go` | Новые domain-типы под aggregate. |
| `backend/internal/repository/creator_application.go` | Новый метод `GetByID`. |
| `backend/internal/repository/creator_application_category.go` | Новая структура для join-результата + метод `ListByApplicationID` с `INNER JOIN categories`. |
| `backend/internal/repository/creator_application_social.go` | Добавить `selectColumns` (через stom) + метод `ListByApplicationID`. |
| `backend/internal/repository/creator_application_consent.go` | Аналогично socials. |
| `backend/internal/service/creator_application.go` | Новый метод `GetByID`, собирает aggregate read-only через `s.pool`. |
| `backend/internal/handler/server.go` | Расширить интерфейсы `CreatorApplicationService` (метод `GetByID`) и `AuthzService` (`CanViewCreatorApplication`). |
| `backend/internal/handler/creator_application.go` | Новый handler `GetCreatorApplication`. |
| `backend/internal/handler/mocks/*` | Регенерация (`make generate-mocks`). |
| `backend/internal/repository/mocks/*` | Регенерация. |
| `backend/internal/handler/creator_application_test.go` | Новый блок `TestServer_GetCreatorApplication` (forbidden / not found / 500 / success). |
| `backend/internal/service/creator_application_test.go` | Новый блок `TestCreatorApplicationService_GetByID` (not found без обёртки + ошибки на каждом из 4 шагов + success с проверкой порядка consents). |
| `backend/internal/repository/creator_application_test.go` | Новый блок `TestCreatorApplicationRepository_GetByID`. |
| `backend/internal/repository/creator_application_category_test.go` | Новый блок `TestCreatorApplicationCategoryRepository_ListByApplicationID` с проверкой JOIN. |
| `backend/internal/repository/creator_application_social_test.go` | Аналогично. |
| `backend/internal/repository/creator_application_consent_test.go` | Аналогично. |
| `backend/e2e/creator_application/creator_application_test.go` | Расширить заголовочный godoc. Расширить три happy-path теста (после POST дёргать GET и проверять aggregate). Добавить `TestGetCreatorApplicationForbidden` и `TestGetCreatorApplicationNotFound`. |

## Файлы для создания

| Файл | Назначение |
|------|------------|
| `backend/internal/authz/creator_application.go` | Метод `CanViewCreatorApplication(ctx) error` (admin-only). Паттерн один-в-один из `backend/internal/authz/audit.go`. |
| `backend/internal/authz/creator_application_test.go` | Unit-тесты authz (admin → nil, brand_manager → ErrForbidden, пустая роль → ErrForbidden). |

## Полезные референсы в кодбазе (паттерны для копирования)

- **Admin GET по ID** — `backend/internal/handler/brand.go:72-109` (`Server.GetBrand`).
- **Authz admin-only метод** — `backend/internal/authz/audit.go:12-17` (`CanListAuditLogs`).
- **Read-only service метод** — `backend/internal/service/brand.go:67-75` (`BrandService.GetBrand`).
- **Repo `GetByID`** — `backend/internal/repository/brand.go:92-97` (`brandRepository.GetByID`).
- **Handler unit-тест admin-операции** — `backend/internal/handler/brand_test.go` (`TestServer_CreateBrand` / `TestServer_GetBrand`).
- **Handler test helpers** — `backend/internal/handler/helpers_test.go` (`newTestRouter`, `doJSON[Resp]`, `expectHandlerUnexpectedErrorLog`).
- **Repo unit-тест с pgxmock** — `backend/internal/repository/brand_test.go` + хелпер `backend/internal/repository/helpers_test.go:13` (`newPgxmock`, использует `pgxmock` v4 с `QueryMatcherEqual` — SQL-литералы должны точно совпадать).
- **E2E setup admin/manager** — `backend/e2e/testutil/seed.go` (`SetupAdminClient`, `SetupBrand`, `SetupManagerWithLogin`, `SetupManager`).
- **E2E auth helper** — `backend/e2e/testutil/client.go:102-106` (`testutil.WithAuth(token)` — `apiclient.RequestEditorFn`).
- **Existing POST E2E** — `backend/e2e/creator_application/creator_application_test.go` (заголовочный godoc + cleanup-стек на образец).
- **Dictionary константы для JOIN** — `backend/internal/repository/dictionary.go` (`TableCategories`, `DictionaryColumnID/Code/Name/SortOrder`).
- **`AuthFromScopes` middleware** — `backend/internal/middleware/auth.go:85-124` (объяснение, как `security: bearerAuth` в OpenAPI запускает 401-проверку).
- **Существующая POST-схема response** — `backend/api/openapi.yaml:1035-1046` (`CreatorApplicationSubmitData`, поле `applicationId` UUID — оттуда берём ID для GET).
- **Wrapper-обёртка `{ data: { ... } }`** — `backend/api/openapi.yaml` секции `GetBrandResult` / `BrandDetailData` (паттерн обёртки response).

## Шаги реализации

### Phase 1 — Контракт

1. **OpenAPI.** Добавить в `backend/api/openapi.yaml`: новый path `/creators/applications/{id}` с GET-операцией (security `bearerAuth`, ответы 200/401/403/404/default) и 5 новых схем под aggregate. Структура схем — производная от полей таблиц (см. миграции `backend/migrations/20260420181753_*` … `20260420181756_*`) и от соглашений в существующих ответах (паттерн обёртки `{ data: { ... } }`).
2. **Регенерация контракта.** `make generate-api`. После — `git status` + `git diff` на сгенерированных файлах. Должны измениться ровно: `backend/internal/api/server.gen.go`, `backend/e2e/apiclient/*.gen.go`, `frontend/{web,tma}/src/api/generated/schema.ts`. Если затронуто что-то ещё — стоп, разобраться.

### Phase 2 — Domain + Repository

3. **Domain types.** В `backend/internal/domain/creator_application.go` добавить `CreatorApplicationDetail` и три nested-типа (Category/Social/Consent). Поля — те, что выводим в response.
4. **Main repo `GetByID`.** В `backend/internal/repository/creator_application.go` добавить метод в интерфейс и реализацию. Паттерн копировать из `BrandRepo.GetByID` (`backend/internal/repository/brand.go:92-97`). `dbutil.One` сам возвращает `sql.ErrNoRows` — пробрасывать как есть.
5. **Categories repo `ListByApplicationID`.** В `backend/internal/repository/creator_application_category.go`:
   - Новая структура для join-результата (поля `Code/Name/SortOrder`).
   - Новый метод с `INNER JOIN categories` (без фильтра по `active`). Сортировка `ORDER BY c.sort_order ASC, c.code ASC`. Использовать константы из `repository/dictionary.go`.
6. **Socials repo `ListByApplicationID`.** В `backend/internal/repository/creator_application_social.go`:
   - Добавить `selectColumns` через `stom.MustNewStom(...).SetTag(string(tagSelect)).TagValues()` (по образцу `creatorApplicationSelectColumns` в main-repo).
   - Новый метод — прямой SELECT по `application_id`, сортировка `platform ASC, handle ASC`.
7. **Consents repo `ListByApplicationID`.** Аналогично socials. БД-сортировка не нужна — порядок задаётся in-memory в сервисе.

### Phase 3 — Service + Authz

8. **Service `GetByID`.** В `backend/internal/service/creator_application.go` добавить read-only метод. 4 последовательных запроса через `s.pool` (без `WithTx`). Порядок:
   - Главная заявка (`creatorApplicationRepo.GetByID`) — если `sql.ErrNoRows`, **возвращать как есть, БЕЗ ОБЁРТКИ через `fmt.Errorf` с `%v`** (handler распознаёт через `errors.Is`). Если оборачиваешь — обязательно через `%w`.
   - Категории / соцсети / согласия — wrap-ошибки через `fmt.Errorf("...: %w", err)` для контекста.
   - Consents отсортировать in-memory по `domain.ConsentTypeValues`: построить `map[consentType]row`, итерировать по слайсу значений, складывать в результат. Отсутствующий тип пропускать без ошибки.
   - Маппинг row → domain через приватные функции — паттерн `brandRowToDomain` / `brandListRowsToDomain` в `service/brand.go`.
9. **Authz метод.** Создать `backend/internal/authz/creator_application.go` с `CanViewCreatorApplication`. Реализация и стиль godoc-комментария — копия `authz/audit.go` (`CanListAuditLogs`).

### Phase 4 — Handler

10. **Расширить интерфейсы в `handler/server.go`** — новый метод в `CreatorApplicationService` и в `AuthzService`. После этого код перестанет компилироваться, пока не появится handler-метод (Phase 4) и моки (Phase 5).
11. **Handler `GetCreatorApplication`** в `backend/internal/handler/creator_application.go`:
    - Сначала `s.authzService.CanViewCreatorApplication(r.Context())` → если err → `respondError`.
    - Потом `s.creatorApplicationService.GetByID(r.Context(), id)` → если err → `respondError` (`sql.ErrNoRows` сам мапнётся в 404 через `response.go:50-51`).
    - Маппинг domain → API через приватный helper.
    - `respondJSON(w, r, http.StatusOK, ...)`.
    - Никаких `logger.Info`/`Error` с полями ПД (ИИН, телефон, адрес, имя).

### Phase 5 — Mocks + Unit-тесты

12. **Регенерация моков.** `make generate-mocks`. Проверить `git status` — должны обновиться моки сервисов / репо / authz, ничего лишнего.
13. **Unit handler.** В `backend/internal/handler/creator_application_test.go` добавить `TestServer_GetCreatorApplication`. Сценарии:
    - `forbidden for manager` — `authz.EXPECT().CanViewCreatorApplication(...).Return(domain.ErrForbidden)`, ожидаем 403.
    - `not found` — service возвращает `sql.ErrNoRows`, ожидаем 404.
    - `service error → 500` — generic error, ожидаем 500 + `expectHandlerUnexpectedErrorLog`.
    - `success` — service возвращает aggregate, проверяем 200 + полный response целиком через `require.Equal`.
    Паттерн — `TestServer_CreateBrand` / `TestServer_GetBrand` в `handler/brand_test.go`.
14. **Unit service.** В `backend/internal/service/creator_application_test.go` добавить `TestCreatorApplicationService_GetByID`. Сценарии:
    - `not found` — main repo → `sql.ErrNoRows`. Проверить через `require.ErrorIs(err, sql.ErrNoRows)` (важно — без обёртки или с `%w`).
    - `categories list error → wrapped`.
    - `socials list error → wrapped`.
    - `consents list error → wrapped`.
    - `success` — все 4 ok, проверяем aggregate целиком + порядок consents (вернуть из мока в обратном порядке, проверить, что в результате `processing → third_party → cross_border → terms`).
15. **Unit repo.** Один блок-тест на каждый из 4 файлов:
    - `creator_application_test.go` — `TestCreatorApplicationRepository_GetByID` (success-маппинг + `sql.ErrNoRows`).
    - `creator_application_category_test.go` — `TestCreatorApplicationCategoryRepository_ListByApplicationID`. SQL-литерал должен содержать JOIN на `categories` и ORDER BY.
    - Аналогично для socials и consents.
    Паттерн — `repository/brand_test.go`. Хелпер `newPgxmock` — в `repository/helpers_test.go:13`.
16. **Unit authz.** В `backend/internal/authz/creator_application_test.go` добавить `TestAuthzService_CanViewCreatorApplication`: admin → nil, brand_manager → `ErrForbidden`, пустая роль → `ErrForbidden`. Паттерн — `authz/brand_test.go` или `authz/audit_test.go`.
17. **Прогон unit-тестов и coverage.** `make test-unit-backend && make test-unit-backend-coverage`. Оба зелёные — иначе фиксить, не идти дальше.

### Phase 6 — E2E

18. **Расширение E2E.** В `backend/e2e/creator_application/creator_application_test.go`:
    - Обновить заголовочный godoc — добавить параграфы про новые/расширенные тесты в нарративном стиле (правила — `backend-testing-e2e.md`).
    - Завести helper-функцию (например, `verifyCreatorApplicationByID`) — поднимает admin-клиента, дёргает GET, нормализует динамические поля (id, createdAt, updatedAt, accepted_at[]) и сверяет aggregate целиком.
    - Расширить `TestSubmitCreatorApplication`: после 201 → берём `submitResp.JSON201.Data.ApplicationId.String()` → `testutil.SetupAdminClient` → GET → сверка с ожидаемым aggregate.
    - Расширить `TestSubmitCreatorApplicationOther`: то же + проверка `categoryOtherText`.
    - Расширить `TestSubmitCreatorApplicationThreads`: то же + проверка `platform: threads`.
    - Новый `TestGetCreatorApplicationForbidden`: создать заявку POST'ом → поднять brand_manager (через `SetupAdminClient` + `SetupBrand` + `SetupManagerWithLogin` из `testutil/seed.go`) → дёрнуть GET с manager-токеном → 403. Auth-helper для запроса — `testutil.WithAuth(token)` (`testutil/client.go:102-106`).
    - Новый `TestGetCreatorApplicationNotFound`: admin + `uuid.New().String()` (валидный UUID, которого нет в БД) → 404. Не использовать невалидный формат — pgx вернёт другую ошибку.
    - `t.Parallel()` на все новые тесты. Cleanup — через существующий `RegisterCreatorApplicationCleanup`; `SetupAdminClient` сам зарегистрирует cleanup для admin'а.
19. **Прогон E2E.** `make test-e2e-backend` — зелёный. Если красный — фиксить.

### Phase 7 — Верификация

20. **Lint.** `make lint-backend` — зелёный.
21. **Build.** `make build-backend` — зелёный.
22. **Идемпотентность.** Прогнать `make test-e2e-backend` ещё раз — оба прогона зелёные. Это подтверждает, что cleanup работает и тесты не зависят от состояния БД.
23. **Проверка ПД-гигиены.** Грепнуть новые/изменённые `.go`-файлы на упоминания полей ПД в `logger.Info`/`logger.Error`/`fmt.Sprintf` — не должно быть `iin`/`phone`/`address`/`first_name`/`last_name` в форматирующих строках.
24. **Финальная сводка** — что сделано, что прошло, что осталось в working tree до ручного ревью (по политике проекта — никаких автокоммитов).

## Стратегия тестирования

### Unit-тесты — что и как

- **Handler** — изолированный slice через `httptest` + сгенерированный chi-роутер. Моки только на `AuthzService` и `CreatorApplicationService`. Покрываем все ветки `respondError`: 403 (forbidden), 404 (через `sql.ErrNoRows` от мока сервиса), 500 (generic error + проверка лога), 200 (success). Финальная проверка — `require.Equal` на всю структуру response.
- **Service** — моки на 4 repo. Сценарии повторяют порядок исполнения метода. Главный фокус — `sql.ErrNoRows` пробрасывается так, чтобы handler смог его распознать через `errors.Is`. Отдельный сценарий проверяет сортировку consents (мок возвращает в обратном порядке).
- **Repository** — `pgxmock` (через `newPgxmock`-хелпер). Строгое матчирование SQL-строки. Литералы намеренно — двойная проверка, что константы в коде корректны (стандарт `backend-constants.md`).
- **Authz** — голый unit с `context.WithValue` для роли. 3 сценария.

### E2E — что покрывает

Расширенные happy-path тесты (`Submit`/`Other`/`Threads`) — это и есть подтверждение, что данные реально записались в 4 таблицы, потому что после POST мы их читаем через GET и сверяем целиком. Два новых теста закрывают security-границы (forbidden) и корректность ошибок (not found).

Unit-тесты не дублируем — для них достаточно мок-уровня. E2E фокусируется на интеграции реальных слоёв, реальной БД, реального middleware-стека.

### Что не должно сломаться (регрессия)

- Существующие `TestSubmitCreatorApplicationDuplicate`, `TestSubmitCreatorApplicationValidation`, все остальные backend E2E.
- Frontend landing E2E (`frontend/e2e/landing/submit.spec.ts`) — схема POST не меняется, не должны сломаться.

## Edge cases (для тестов и для реализации)

- **Заявка существует, но связанных строк нет** (categories/socials/consents). Не должно случиться (POST атомарно создаёт всё), но на read-стороне репо вернёт пустой slice — допустимо, не падать.
- **`category_other_text != nil`, но `other` нет в категориях.** Не должно случиться (service-валидация на POST), но возвращаем как есть — это историческая запись.
- **Невалидный UUID в path-параметре** → pgx может вернуть ошибку "invalid input syntax for type uuid" вместо `sql.ErrNoRows`. Это разная ветка маппинга. Тест-кейс на 404 должен использовать **валидный** UUID, которого нет в БД (`uuid.New().String()`). Невалидный формат — отдельная ветка, она не покрывается этим слайсом.
- **Деактивированная (`active = false`) категория**, на которую ссылается заявка → JOIN всё равно вернёт её (мы не фильтруем по `active`). Это by design — историческая заявка должна показывать категории, которые были выбраны на момент подачи.

## Оценка рисков

| Риск | Вероятность | Митигация |
|------|-------------|-----------|
| Регенерация `*.gen.go` затрагивает что-то лишнее | Низкая | После каждого `make generate-api` — `git diff`, проверить scope. Если затронуто что-то за пределами трёх ожидаемых файлов — стоп. |
| Mockery не подхватывает новый интерфейс или калечит существующие моки | Низкая | После `make generate-mocks` — `git status`, проверить ровно ожидаемые файлы. |
| `INNER JOIN categories` теряет soft-disabled категорию | Низкая | Не фильтруем по `c.active` — деактивированная категория всё равно вернётся. Если позже введём hard-delete — переключиться на `LEFT JOIN`. |
| ПД попадает в логи через middleware/handler/service | Низкая | Шаг 23 явно проверяет grep'ом. `respondJSON`/`respondError` уже не логируют body. Все новые слои не должны логировать значения ПД. |
| Schema-обновление сломает фронт-билд | Низкая | `schema.ts` тип-only, в runtime не используется. На всякий случай можно прогнать `cd frontend/web && npx tsc --noEmit`. |
| E2E flake из-за параллельности | Очень низкая | `SetupAdminClient` создаёт уникального admin'а на каждый `t.Parallel()` тест с уникальным email. Cleanup через стек. |
| Coverage <80% per-method | Низкая | На каждый новый public-метод ≥2 t.Run сценария (success + error). `make test-unit-backend-coverage` фейлится явно. |
| `sql.ErrNoRows` обёрнут через `%v` в сервисе и не распознаётся handler'ом | Средняя | Явно прописано в Phase 3 шаге 8. Юнит-тест на сервис проверяет `require.ErrorIs(err, sql.ErrNoRows)`. |
| Невалидный UUID в path-параметре в E2E-тесте → не та ошибка | Низкая | Для `NotFound`-теста использовать `uuid.New().String()` (валидный, но несуществующий). |

## План отката

Слайс — добавление нового эндпоинта без изменения существующих контрактов. Откат тривиален.

- **На этапе разработки** — `git restore` затронутых файлов. Working tree вернётся в исходное состояние.
- **После коммита** — `git revert` коммита (атомарный коммит для всего слайса либо серия логически связанных).
- **После деплоя** — публичные клиенты эндпоинт не используют (только E2E-тесты). Откат коммитом + редеплой. Downtime нулевой.
- **БД-откат** не нужен — миграций нет.

### Сигналы early-stop в процессе

- `make generate-api` ругается на дубли operationId или невалидный YAML → стоп, разобраться.
- `make test-unit-backend` стал красным после регенерации моков, причём не из-за наших новых тестов → стоп, проверить, что mockery не покалечил существующие моки.
- `make test-e2e-backend` стал красным на тестах, которые не трогали (`TestLogin`, `TestBrandCRUD`) → стоп, скорее всего что-то сломалось в `server.go` или роутинге.
