---
type: scout
feature: creator-application-get
created: 2026-04-25
---

# Scout: GET /creators/applications/{id} (admin-only)

## Перед работой с этим документом

Этот документ — итог разведки кодовой базы. Используется как контекст для следующего шага (планирование или реализация). Прежде чем что-либо делать на его основе:

1. **Прочитай ВСЕ файлы из `docs/standards/` целиком, в сыром виде, без выборки и пересказа.** Это обязательные стандарты проекта (архитектура, кодген, репозитории, тесты, безопасность и т.д.). Они — источник истины.
2. **Прочитай файлы, которые этот scout явно упоминает.** Не полагайся на пересказ — открывай и читай сам.

Без этого решения, описанные ниже, могут конфликтовать со стандартами или существующими паттернами.

## Контекст задачи

E2E-тесты `backend/e2e/creator_application/creator_application_test.go` сейчас проверяют только HTTP-response от POST `/creators/applications`. Нет способа убедиться, что данные реально записались в БД (5 таблиц: `creator_applications` + `_categories` + `_socials` + `_consents` + `audit_logs`). Косвенное доказательство есть только для `iin` (через duplicate-тест).

Решение — вертикальный слайс будущей админки модерации (Epic 2): admin-only ручка `GET /creators/applications/{id}` возвращает полный aggregate. ID берётся из POST-response **напрямую** — там уже есть отдельное поле `applicationId` (формат `uuid`, см. `CreatorApplicationSubmitData` в `backend/api/openapi.yaml:1035-1046`). Никакого парсинга URL Telegram-коллбэка для извлечения ID — `telegramBotUrl` остаётся только для своей роли (deep-link), а ID читаем как `resp.JSON201.Data.ApplicationId.String()`. После POST в E2E-тесте дёргаем GET и сверяем все поля.

Скоуп: только GET одной заявки по ID, admin-only, response содержит всё включая развёрнутые consents.

## Зафиксированные решения

- **Path:** `/creators/applications/{id}` (зеркалит POST `/creators/applications`). Без `/admin/*` префикса — текущий проект (`/brands`, `/audit-logs`) тоже admin без префикса.
- **Auth:** `security: - bearerAuth: []` в OpenAPI + новый authz-метод `CanViewCreatorApplication` (admin-only). Из-за `AuthFromScopes` middleware (`backend/internal/middleware/auth.go:85`) объявление в OpenAPI автоматически даёт 401 при отсутствии/невалидном токене; authz даёт 403 при wrong role.
- **Response shape:** полный aggregate, развёрнутые consents (с `document_version`/`ip_address`/`user_agent`/`accepted_at`/`consent_type`). `birthDate` тоже включаем — admin-полезное.
- **Read pattern:** read-only через `s.pool` без `WithTx`. 4 последовательных SELECT (главная заявка → категории → соцсети → согласия). Параллельность через goroutines не нужна — холодный путь.
- **Categories JOIN:** `INNER JOIN categories` без фильтра по `active` — категории не удаляются жёстко (только soft-deactivation), а по дизайну деактивированная категория всё равно должна вернуться в исторической заявке.
- **Сортировка:** categories — by `sort_order ASC, code ASC`; socials — by `platform ASC, handle ASC`; consents — in-memory по фиксированному порядку `domain.ConsentTypeValues` (`processing → third_party → cross_border → terms`), независимо от того, как пришли из БД.
- **404-маппинг:** в проекте нет `domain.NewNotFoundError`. `respondError` (`backend/internal/handler/response.go:50`) уже мапит и `domain.ErrNotFound`, и `sql.ErrNoRows` → 404. Service просто пробрасывает `sql.ErrNoRows` от main repo дальше (через `%w` если оборачивает с контекстом, иначе как есть).
- **Audit:** на read-операцию **не пишется** — это не изменение состояния.
- **Sort consents — implementation:** при маппинге service строит `map[consentType]row` и итерирует по `domain.ConsentTypeValues` — отсутствующие consents не должны случиться (insert-batch на POST), но если случатся — пропускать без ошибки.

## Затронутые области

### Новые файлы

- `backend/internal/authz/creator_application.go` — метод `CanViewCreatorApplication`. Паттерн — `backend/internal/authz/audit.go` (один метод-файл, такой же стиль).
- `backend/internal/authz/creator_application_test.go` — unit-тесты authz.

### Изменяемые файлы

**OpenAPI:**
- `backend/api/openapi.yaml` — добавить `paths: /creators/applications/{id}: get:` (security bearerAuth, ответы 200/401/403/404/default), новые схемы под aggregate (`CreatorApplicationDetail`, `CreatorApplicationCategoryItem`, `CreatorApplicationSocialItem`, `CreatorApplicationConsentItem`, `GetCreatorApplicationResult`).

**Сгенерированный код (после `make generate-api`):**
- `backend/internal/api/server.gen.go` — новый метод `GetCreatorApplication(w, r, id)` в `ServerInterface`, новые структуры response.
- `backend/e2e/apiclient/{client,types}.gen.go` — клиентский метод `GetCreatorApplicationWithResponse` для E2E.
- `frontend/{web,tma}/src/api/generated/schema.ts` — обновлённая схема (тип-only, не используется в текущем слайсе, но закоммитится в этом же изменении).

**Domain** (`backend/internal/domain/creator_application.go`):
- `CreatorApplicationDetail` (id, status, last_name, first_name, middle_name?, iin, birth_date, phone, city, address, category_other_text?, created_at, updated_at + slices)
- `CreatorApplicationCategoryDetail` (code, name, sort_order)
- `CreatorApplicationSocialDetail` (platform, handle)
- `CreatorApplicationConsentDetail` (consent_type, accepted_at, document_version, ip_address, user_agent)

**Repository:**
- `backend/internal/repository/creator_application.go` — новый метод `GetByID(ctx, id) (*CreatorApplicationRow, error)`. `dbutil.One` сам возвращает `sql.ErrNoRows`.
- `backend/internal/repository/creator_application_category.go` — новая структура `CreatorApplicationCategoryDetailRow` (поля `Code/Name/SortOrder`) для join-результата. Новый метод `ListByApplicationID(ctx, appID)` с `INNER JOIN categories`.
- `backend/internal/repository/creator_application_social.go` — добавить `selectColumns` через `stom` (сейчас есть только insert-теги). Новый метод `ListByApplicationID(ctx, appID)` — прямой SELECT по `application_id`.
- `backend/internal/repository/creator_application_consent.go` — добавить `selectColumns`. Новый метод `ListByApplicationID(ctx, appID)`.

**Service** (`backend/internal/service/creator_application.go`):
- Метод `GetByID(ctx, id) (*domain.CreatorApplicationDetail, error)` — read-only через `s.pool`, без `WithTx`, 4 последовательных запроса. Маппинг row → domain. Сортировка consents in-memory по `domain.ConsentTypeValues`.
- В `CreatorApplicationRepoFactory` добавлять ничего не нужно — все 4 метода уже есть.

**Handler** (`backend/internal/handler/`):
- `server.go` — расширить интерфейс `CreatorApplicationService` методом `GetByID`, расширить `AuthzService` методом `CanViewCreatorApplication`.
- `creator_application.go` — новый метод `GetCreatorApplication(w, r, id string)`: authz → service → маппинг domain → API → `respondJSON`.

**Mocks** (`make generate-mocks`):
- `backend/internal/handler/mocks/MockCreatorApplicationService.go` — новый метод `GetByID`.
- `backend/internal/handler/mocks/MockAuthzService.go` — новый метод `CanViewCreatorApplication`.
- `backend/internal/repository/mocks/MockCreatorApplication{,Category,Social,Consent}Repo.go` — новые методы.

**Unit-тесты:**
- `backend/internal/handler/creator_application_test.go` — `TestServer_GetCreatorApplication` (forbidden / not found / 500 / success).
- `backend/internal/service/creator_application_test.go` — `TestCreatorApplicationService_GetByID` (not found / repo errors / success aggregate, проверить порядок consents).
- `backend/internal/repository/creator_application_test.go` — `TestCreatorApplicationRepository_GetByID`.
- `backend/internal/repository/creator_application_category_test.go` — `TestCreatorApplicationCategoryRepository_ListByApplicationID` (проверить SQL c JOIN).
- `backend/internal/repository/creator_application_social_test.go` — аналогично.
- `backend/internal/repository/creator_application_consent_test.go` — аналогично.
- `backend/internal/authz/creator_application_test.go` — `TestAuthzService_CanViewCreatorApplication`.

**E2E-тесты** (`backend/e2e/creator_application/creator_application_test.go`):
- Расширить заголовочный godoc (русский нарратив про новые тесты).
- Расширить `TestSubmitCreatorApplication`: после 201 → admin-клиент через `testutil.SetupAdminClient` → GET → сверка aggregate целиком.
- Расширить `TestSubmitCreatorApplicationOther`: то же + `categoryOtherText`.
- Расширить `TestSubmitCreatorApplicationThreads`: то же + `platform: threads`.
- Новый `TestGetCreatorApplicationForbidden` — brand_manager-токен → 403.
- Новый `TestGetCreatorApplicationNotFound` — admin + несуществующий валидный UUID → 404.

**Миграции:** не нужны. Все таблицы и колонки уже на месте.

## Полезные референсы в кодбазе (паттерны для копирования)

- **Admin GET по ID** — `backend/internal/handler/brand.go:72-109` (`Server.GetBrand`).
- **Authz admin-only метод** — `backend/internal/authz/audit.go:12-17` (`CanListAuditLogs`).
- **Read-only service метод** — `backend/internal/service/brand.go:67-75` (`BrandService.GetBrand`).
- **Repo `GetByID`** — `backend/internal/repository/brand.go:92-97` (`brandRepository.GetByID`).
- **Handler unit-тест admin-операции** — `backend/internal/handler/brand_test.go` (`TestServer_CreateBrand` / `TestServer_GetBrand`).
- **Repo unit-тест с pgxmock** — `backend/internal/repository/brand_test.go` + helpers `backend/internal/repository/helpers_test.go` (`newPgxmock`).
- **Handler test helpers** — `backend/internal/handler/helpers_test.go` (`newTestRouter`, `doJSON[Resp]`, `expectHandlerUnexpectedErrorLog`).
- **E2E setup admin/manager** — `backend/e2e/testutil/seed.go` (`SetupAdminClient`, `SetupBrand`, `SetupManagerWithLogin`).
- **E2E auth helper** — `backend/e2e/testutil/client.go:102-106` (`testutil.WithAuth(token)` — `apiclient.RequestEditorFn`).
- **Existing POST E2E** — `backend/e2e/creator_application/creator_application_test.go` (заголовочный godoc + cleanup-стек на образец).
- **Dictionary константы для JOIN** — `backend/internal/repository/dictionary.go` (`TableCategories`, `DictionaryColumnID/Code/Name/SortOrder`).
- **`AuthFromScopes` middleware** — `backend/internal/middleware/auth.go:85-124` (объяснение, как `security: bearerAuth` в OpenAPI запускает 401-проверку).

## Риски и соображения

### Безопасность

- **ИИН / телефон / адрес — ПД.** Возвращаются только admin'у. RBAC через `CanViewCreatorApplication` строгий. 401 от middleware (нет/битый токен) ≠ 403 от authz (не admin).
- **Логирование.** Service вернёт ПД, handler сериализует в JSON. Никаких `logger.Info("got app", "iin", ...)` или похожего — `security.md` запрещает ПД в stdout-логах. Audit для GET не пишется.
- **Утечка существования.** Если admin запросит несуществующий UUID → 404 (это ок, он admin). Если manager или unauthenticated — 403/401 раньше, чем GetByID коснётся БД.

### Edge cases

- Заявка существует, но связанных строк нет (categories/socials/consents). Не должно случиться (POST атомарно создаёт всё), но на read-стороне репо вернёт пустой slice — допустимо. Не падать.
- Заявка `category_other_text != nil`, но `other` нет в категориях. Тоже не должно случиться (service-валидация на POST), но возвращаем как есть.
- Невалидный UUID в path-параметре → pgx может вернуть ошибку "invalid input syntax for type uuid" вместо `sql.ErrNoRows`. Тест-кейс на 404 должен использовать **валидный** UUID, которого нет в БД (например `uuid.New().String()`).

### Breaking changes

Никаких. Только новый endpoint и новые типы. Существующие POST/E2E не меняются.

### Стандарты, особенно релевантные этой задаче

(Все стандарты `docs/standards/` обязательны — это напоминание про самое важное для слайса.)

- `backend-architecture.md` — слои handler→service→repo, никаких прямых вызовов repo из handler.
- `backend-codegen.md` — никакого ручного `chi.URLParam`, ручных типов в response, ручных моков.
- `backend-constants.md` — все таблицы/колонки через константы.
- `backend-repository.md` — Row + два тега, `selectColumns` через stom, `[]*Row` для списков, `sql.ErrNoRows` (не `pgx.ErrNoRows`).
- `backend-design.md` — RepoFactory, accept interfaces.
- `backend-transactions.md` — read-only через `s.pool`, без `WithTx`.
- `backend-testing-unit.md` — handler/service/repo/authz изолированно, ≥80% per-method coverage, `t.Parallel`.
- `backend-testing-e2e.md` — нарративный godoc-комментарий на русском (исключение из общего правила), английские inline комментарии, `t.Parallel`, через сгенерированный клиент.
- `naming.md` — `Code{Error}`, `Table{Entity}`, `{Entity}Column{Field}`.
- `security.md` — ПД не логируем, secure by default, RBAC строгий.
