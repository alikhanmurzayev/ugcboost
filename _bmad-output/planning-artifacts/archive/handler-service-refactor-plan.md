# План реализации: рефакторинг handler/service под стандарты

## Предзагрузка контекста (ОБЯЗАТЕЛЬНО)

**Перед началом работы агент-исполнитель обязан полностью прочитать (через Read tool, без grep/search, в сыром виде) все файлы из `docs/standards/`** — это загрузит стандарты в контекст и не даст пропустить нарушение при правках. Порядок не важен, но должны быть прочитаны все:

- `docs/standards/backend-architecture.md`
- `docs/standards/backend-codegen.md`
- `docs/standards/backend-constants.md`
- `docs/standards/backend-design.md`
- `docs/standards/backend-errors.md`
- `docs/standards/backend-libraries.md`
- `docs/standards/backend-repository.md`
- `docs/standards/backend-testing-e2e.md`
- `docs/standards/backend-testing-unit.md`
- `docs/standards/backend-transactions.md`
- `docs/standards/frontend-api.md`
- `docs/standards/frontend-components.md`
- `docs/standards/frontend-quality.md`
- `docs/standards/frontend-state.md`
- `docs/standards/frontend-testing-e2e.md`
- `docs/standards/frontend-testing-unit.md`
- `docs/standards/frontend-types.md`
- `docs/standards/naming.md`
- `docs/standards/security.md`

Также агент обязан полностью прочитать текущее состояние затрагиваемых пакетов (`backend/internal/handler/`, `backend/internal/service/`, `backend/internal/authz/`, `backend/internal/middleware/`, `backend/internal/domain/`, `backend/cmd/api/`, Makefile) до начала правок.

---

## Обзор

Привести слои handler и service к стандартам `backend-architecture.md`, `backend-transactions.md`, `backend-codegen.md`, `backend-testing-unit.md`, `backend-errors.md`, `backend-constants.md`, `backend-design.md`: вынести авторизацию в AuthzService, перенести аудит-логи в сервисы (внутри транзакций), перенести OpenAPI-контракт в `backend/api/`, удалить дубликаты сгенерированных констант, расширить OpenAPI именованными схемами и полноценной спекой test-ручек, вывести test-only `SeedUser` из prod-интерфейса, переписать unit-тесты handler'а под стандарт (роутер + типизированные request/response), убрать молчаливое игнорирование ошибок сериализации, ввести именованные константы для audit action / entity type.

## Стратегия работы агента

**Агент НЕ делает коммитов и НЕ мержит PR.** Работа агента заканчивается на рабочем дереве: все правки лежат в working tree / staging только если агент сам это обеспечил для своих нужд (например, `git mv`), **но финального `git commit` нет**. Решение о коммите, push'е и мерже PR принимает Alikhan после ревью и локального тестирования.

Этапы ниже — это **порядок действий**, а не отдельные коммиты. Цель порядка — чтобы код компилировался и тесты проходили на каждом промежуточном шаге. Промежуточных коммитов агент не создаёт.

Запрещено:
- `git commit` (любой, включая `--amend`)
- `git push`
- `gh pr merge`, `gh pr create` (PR уже открыт)
- `git reset --hard`, `git checkout <file>` по файлам с правками — можно потерять работу пользователя
- Любые действия, влияющие на удалённый репозиторий

## Требования

### Must-have
- **REQ-1**: AuthzService как struct с DI через интерфейсы. Методы именуются по API-действиям (`CanCreateBrand`, `CanViewBrand` и т.д.). Каждый метод достаёт userID/role из context самостоятельно.
- **REQ-2**: Зависимости AuthzService именуются как сервисы (`BrandService`, не `BrandAccessChecker`).
- **REQ-3**: Авторизация — только в AuthzService. Никаких role-checks в handler и business service.
- **REQ-4**: Аудит-логи пишутся сервисами внутри транзакций (`dbutil.WithTx`). Fire-and-forget запрещён.
- **REQ-5**: Handler не знает об аудите — нет импорта `service.AuditEntry`, нет вызовов `logAudit`.
- **REQ-6**: OpenAPI YAML перенесён в `backend/api/`. `make generate-api` работает.
- **REQ-7**: `domain/roles.go` удалён. Везде используется `api.UserRole` / `api.Admin` / `api.BrandManager`.
- **REQ-8**: `middleware.RoleFromContext()` возвращает `api.UserRole`, не `string`.
- **REQ-9**: Handler не импортирует `pgx` — использует domain errors.
- **REQ-10**: Все unit-тесты проходят: `make test-unit-backend`. Линтер чист: `make lint-backend`.
- **REQ-11**: E2E тесты проходят: `make test-e2e-backend`. HTTP-контракт не изменён по семантике (структура response и коды сохраняются).
- **REQ-12**: Unit-тесты handler'а прогоняются через зарегистрированный роутер с `ServerInterfaceWrapper`. Request body формируется через `json.Marshal` типизированной структуры из `api/`, response — через `json.Unmarshal` в сгенерированный тип. Динамические поля (time, temp password) проверяются через `require.WithinDuration`/`require.NotEmpty`, затем зануляются, затем `require.Equal` целиком. Сырой JSON (`strings.NewReader("`{...}`")`) в тестах handler'а запрещён.
- **REQ-13**: Тестовые эндпоинты описаны в `openapi-test.yaml` с полной спецификацией request/response. Для handler'а сгенерирован `testapi.ServerInterface`. В `handler/test.go` нет анонимных request-struct, нет `r.URL.Query().Get(...)`, нет `map[string]any` — только сгенерированные типы.
- **REQ-14**: `SeedUser` вынесен из prod-интерфейса `AuthService` в отдельный `TestAuthService` интерфейс (объявленный в `handler/test.go`). `Server`/`AuthService` в `handler/server.go` не содержат test-only методов.
- **REQ-15**: Ошибки `json.Encode` в handler'е не игнорируются. Все вызовы перенесены на общий helper `encodeJSON`, который логирует ошибку через `slog.Error` с контекстом (path, status). `//nolint:errcheck` для `json.Encode` запрещён.
- **REQ-16**: `rawJSONToAny` логирует ошибку `json.Unmarshal` через `slog.Error` с ключом (audit entry ID), не проглатывает молча.
- **REQ-17**: Audit actions и entity types — именованные константы в `service/audit_constants.go` (`AuditActionLogin`, `AuditActionBrandCreate` и т.д., `AuditEntityTypeUser`, `AuditEntityTypeBrand`). Строковые литералы для этих значений в коде `service/` запрещены.
- **REQ-18**: `password` и `newPassword` в OpenAPI имеют `minLength: 6`. Валидация длины пароля выполняется ServerInterfaceWrapper'ом, не handler'ом. `len(req.NewPassword) < 6` в `handler/auth.go` удалён.
- **REQ-19**: Все response-bodies в OpenAPI описаны через именованные компоненты схем (`$ref`), inline `data` запрещён. В handler'е нет anonymous struct literal при формировании ответа — данные кладутся в `api.{Entity}Response{ Data: api.{Entity}Data{...} }`.

### Nice-to-have
- Использовать `chi/middleware.RealIP` для нормализации IP вместо ручного парсинга (стандарт `backend-libraries.md`).

### Вне скоупа
- Рефакторинг repository слоя (уже сделан в предыдущих PR).
- Изменение **бизнес-поведения** OpenAPI (новые эндпоинты, смена семантики, новые поля). Допустимы **структурные** правки: выделение именованных схем через `$ref`, `minLength` валидация, полноценная спека test-endpoints, переименование inline data wrappers в именованные schemas.
- Рефакторинг middleware (auth, logging, recovery) кроме добавления IP в context и типизации Role.
- Содержательные изменения на фронтенде. Регенерация `schema.ts` — автоматом через `make generate-api`. Если фронт использует устаревшие пути в типах — точечный фикс на местах (не полноценный рефакторинг фронта).
- Создание PR (уже открыт).

### Критерии успеха
- `make generate-api && make generate-mocks` работает без ошибок
- `make test-unit-backend` — зелёный
- `make test-e2e-backend` — зелёный
- `make lint-backend lint-web lint-tma` — без предупреждений
- Визуальный грепп — все пусто:
  - `authz.RequireAdmin` в `backend/`
  - `logAudit` в `backend/internal/handler/`
  - `domain.RoleAdmin\|domain.RoleBrandManager\|domain.UserRole` в `backend/`
  - `pgx` в `backend/internal/handler/`
  - `//nolint:errcheck` в `backend/internal/handler/`
  - `strings.NewReader(` в `backend/internal/handler/*_test.go`
  - `map[string]any` в `backend/internal/handler/test.go`
  - `r\.URL\.Query\(\)\.Get` в `backend/internal/handler/`
  - `SeedUser` в `backend/internal/handler/server.go`
  - Литералы `"login"\|"logout"\|"password_reset"\|"brand_create"\|"brand_update"\|"brand_delete"\|"manager_assign"\|"manager_remove"` в `backend/internal/service/*.go` (только через константы)
  - `struct \{` внутри возвращаемых `api.*Result{Data: ...}` в `backend/internal/handler/` (только named типы)

---

## Анализ текущего состояния

Релевантные стандарты:
- `backend-architecture.md` — слои и их ответственности, авторизация в отдельном сервисе, только валидированные данные до сервиса
- `backend-transactions.md` — аудит внутри транзакций, fire-and-forget запрещён
- `backend-codegen.md` — запрет ручных дубликатов, анонимных структур, ручного парсинга query/path params
- `backend-design.md` — DI через конструктор, принимать интерфейсы, возвращать struct
- `backend-errors.md` — нельзя проглатывать ошибки, `_ = fn()` запрещено
- `backend-testing-unit.md` — паттерны handler-тестов (router + wrapper + типизированные request/response)
- `backend-libraries.md` — библиотеки вместо велосипедов (для IP-парсинга)
- `backend-constants.md` — enum-like значения только через константы
- `naming.md` — именование файлов, структур, receiver'ов
- `security.md` — без sensitive данных в логах

Ключевые паттерны из текущего кода (сохранить):
- RepoFactory с методами `New{Entity}Repo(dbutil.DB) repository.{Entity}Repo`
- Каждый сервис объявляет свой `{Service}RepoFactory` интерфейс с нужными конструкторами
- `dbutil.WithTx(ctx, pool, func(tx) error {...})` для транзакций
- Mock-генерация через mockery (`.mockery.yaml` с `all: true`)

Места нарушений стандартов (до правок):
- **Роли как строки / дубликат в `domain`**: `domain.RoleAdmin`/`RoleBrandManager` в `handler/auth_test.go` (3), `handler/brand_test.go` (2), `authz/authz.go` (3), `service/brand.go` (3), `service/auth.go` (2). `ContextKeyRole` хранит `string`: `middleware/auth.go:56, 120`, `middleware/auth_test.go:18`, `handler/auth_test.go:224, 264`, `handler/brand_test.go:21, 28`.
- **Авторизация размазана**: `authz.RequireAdmin` в `handler/brand.go` (5 мест), `handler/audit.go:14`. `BrandService.CanViewBrand` в `handler/brand.go:80`.
- **Fire-and-forget audit**: `logAudit(...)` в `handler/auth.go` (3 места: login, logout, password_reset), `handler/brand.go` (5 мест).
- **Handler импортирует `pgx`**: `handler/response.go:9, 38` (`pgx.ErrNoRows`).
- **OpenAPI YAML в корне**: `api/openapi.yaml`, `api/openapi-test.yaml` — должно быть в `backend/api/`.
- **Unit-тесты handler'а нарушают `backend-testing-unit.md`**: сырой JSON в `strings.NewReader` (`auth_test.go` и `brand_test.go` — все тесты), прямой вызов `s.Login(w, r)` вместо роутера, response body не парсится в типизированную структуру, нет `require.Equal` целиком, нет `require.WithinDuration` для динамических полей.
- **`handler/test.go`**: ручные анонимные `struct` для request body (`SeedUser`, `SeedBrand`), `r.URL.Query().Get("email")` в `GetResetToken`, `map[string]any{...}` в response.
- **`AuthService` интерфейс содержит `SeedUser`** (`handler/server.go:19`) — test-only метод в prod-интерфейсе.
- **Игнорирование ошибок `json.Encode`**: `//nolint:errcheck` в `handler/response.go:20, 55`, `handler/health.go:21`, `handler/test.go:51, 89, 111`.
- **`rawJSONToAny` проглатывает ошибку unmarshal молча**: `handler/audit.go:91`.
- **Строковые литералы audit actions/entities**: `handler/auth.go` (`"login"`, `"logout"`, `"password_reset"`, `"user"`), `handler/brand.go` (`"brand_create"`, `"brand_update"`, `"brand_delete"`, `"manager_assign"`, `"manager_remove"`, `"brand"`).
- **Бизнес-валидация в handler**: `handler/auth.go:159` — `len(req.NewPassword) < 6`.
- **Inline response schemas в OpenAPI**: `LoginResult.data`, `BrandResult.data`, `GetBrandResult.data`, `AssignManagerResult.data`, `ListBrandsResult.data`, `MessageResponse.data`, `AuditLogsResult.data`, `UserResponse.data` — генерятся как anonymous structs, handler вынужден писать `struct { ... }{ ... }`.

---

## Последовательность этапов

1. Перенос `api/` → `backend/api/`
2. Расширение OpenAPI (named schemas + test endpoints + minLength) + регенерация
3. Типизация ролей (удаление `domain/roles.go`)
4. AuthzService
5. Аудит в сервисах + audit константы
6. test.go рефакторинг (TestAuthService split + codegen-based test handler)
7. Рефакторинг unit-тестов handler'а под стандарт
8. Финальные чистки: pgx, errcheck, rawJSONToAny, грепы

Порядок подобран так, чтобы: (а) типы из OpenAPI были готовы до всех правок handler'а, (б) handler пересматривался один раз, (в) тесты переписывались на уже финальной сигнатуре.

---

## Файлы для изменения

### Этап 1 — перенос api/

| Файл | Изменения |
|------|-----------|
| `Makefile` | Все пути `api/openapi*.yaml` → `backend/api/openapi*.yaml`. Frontend: `../../api/openapi.yaml` → `../../backend/api/openapi.yaml` |

### Этап 2 — расширение OpenAPI + регенерация

| Файл | Изменения |
|------|-----------|
| `backend/api/openapi.yaml` | Все inline `data` schemas вынести в `components/schemas` через `$ref`: `LoginData`, `UserData`, `BrandData`, `BrandDetailData`, `ListBrandsData`, `AssignManagerData`, `ListAuditLogsData`, `MessageData`. Добавить `minLength: 6` для `password` (`LoginRequest`) и `newPassword` (`PasswordResetBody`) |
| `backend/api/openapi-test.yaml` | Полноценная спека для `POST /test/seed-user` (`SeedUserRequest`, `SeedUserResponse`), `POST /test/seed-brand` (`SeedBrandRequest`, `SeedBrandResponse`), `GET /test/reset-tokens` (query param `email`, `ResetTokenResponse`) |
| `Makefile` | В `generate-api` добавить генерацию `chi-server,models` для `openapi-test.yaml` в пакет `testapi`: `-o backend/internal/testapi/server.gen.go` |
| `backend/internal/api/server.gen.go` | Перегенерировать |
| `backend/internal/testapi/server.gen.go` | **Новый** (генерация) |
| `backend/e2e/apiclient/*.gen.go`, `backend/e2e/testclient/*.gen.go` | Перегенерировать |
| `frontend/web/src/api/generated/schema.ts`, `frontend/tma/src/api/generated/schema.ts` | Перегенерировать |
| `backend/internal/handler/auth.go` | Заменить `struct { AccessToken string; User api.User }{...}` на `api.LoginData{...}`. Аналогично для `UserResponse`. Удалить `if len(req.NewPassword) < 6` (теперь валидация в wrapper) |
| `backend/internal/handler/brand.go` | `api.BrandResult.Data` — `api.BrandData{...}`; `api.GetBrandResult.Data` — `api.BrandDetailData{...}`; `api.ListBrandsResult.Data` — `api.ListBrandsData{...}`; `api.AssignManagerResult.Data` — `api.AssignManagerData{...}`; `api.MessageResponse.Data` — `api.MessageData{...}` |
| `backend/internal/handler/audit.go` | `api.AuditLogsResult.Data` — `api.ListAuditLogsData{...}` |

### Этап 3 — типизация ролей

| Файл | Изменения |
|------|-----------|
| `backend/internal/middleware/auth.go` | `ContextKeyRole` хранит `api.UserRole`. `RoleFromContext()` возвращает `api.UserRole`. При записи — `api.UserRole(role)` |
| `backend/internal/middleware/auth_test.go` | Setup context с `api.UserRole` |
| `backend/internal/domain/user.go` | `Role` тип — `api.UserRole`. Импорт `api` |
| `backend/internal/authz/authz.go` | `domain.RoleAdmin` → `api.Admin`. Параметр role → `api.UserRole` (временно до этапа 4) |
| `backend/internal/service/brand.go` | Сравнения ролей под `api.UserRole` (временно) |
| `backend/internal/service/auth.go` | `string(domain.RoleAdmin)` → `string(api.Admin)`; `domain.UserRole(...)` → `api.UserRole(...)` |
| `backend/internal/handler/auth_test.go` | `domain.RoleAdmin` → `api.Admin`, строки `"admin"` в context → `api.Admin` |
| `backend/internal/handler/brand_test.go` | `domain.RoleBrandManager` → `api.BrandManager`, строки в context → константы |

### Этап 4 — AuthzService

| Файл | Изменения |
|------|-----------|
| `backend/internal/authz/authz.go` | Переписать: `AuthzService` struct, конструктор, интерфейсы зависимостей (`BrandService`) |
| `backend/internal/service/brand.go` | Добавить `IsUserBrandManager(ctx, userID, brandID) (bool, error)`. Удалить `CanViewBrand`. Изменить `ListBrands(ctx, managerID *string)` — без role |
| `backend/internal/handler/server.go` | Интерфейс `AuthzService`, зависимость `authzService`. Убрать `CanViewBrand` из `BrandService`. Сигнатура `ListBrands(ctx, managerID *string)` |
| `backend/internal/handler/brand.go` | `authz.RequireAdmin(ctx)` → `s.authzService.Can*Brand(ctx, ...)`. `s.brandService.CanViewBrand` → `s.authzService.CanViewBrand`. `ListBrands`: `canViewAll, userID, err := s.authzService.CanListBrands(ctx); if !canViewAll { managerID = &userID }` |
| `backend/internal/handler/audit.go` | `authz.RequireAdmin` → `s.authzService.CanListAuditLogs` |
| `backend/internal/service/brand_test.go` | Удалить `TestBrandService_CanViewBrand`. Обновить `TestBrandService_ListBrands`. Добавить `TestBrandService_IsUserBrandManager` |
| `backend/internal/handler/brand_test.go` | Mock AuthzService вместо context-based role checks |
| `backend/cmd/api/main.go` | Создать AuthzService, передать в `handler.NewServer` |

### Этап 5 — аудит в сервисах + audit константы

| Файл | Изменения |
|------|-----------|
| `backend/internal/middleware/client_ip.go` | **Новый**: adapter-middleware над `chi/middleware.RealIP`, кладёт `r.RemoteAddr` в context. Экспортирует `ClientIPFromContext(ctx) string` |
| `backend/internal/middleware/client_ip_test.go` | **Новый**: X-Forwarded-For, X-Real-IP, RemoteAddr fallback |
| `backend/internal/service/audit_constants.go` | **Новый**: `AuditActionLogin`, `AuditActionLogout`, `AuditActionPasswordReset`, `AuditActionBrandCreate`, `AuditActionBrandUpdate`, `AuditActionBrandDelete`, `AuditActionManagerAssign`, `AuditActionManagerRemove`, `AuditEntityTypeUser`, `AuditEntityTypeBrand` |
| `backend/internal/service/audit.go` | Удалить метод `Log()` и тип `AuditEntry`. Оставить `List()` + `AuditRepoFactory` |
| `backend/internal/service/brand.go` | `BrandRepoFactory` добавляет `NewAuditRepo`. Write-методы обёрнуты в `WithTx` с `auditRepo.Create(...)`. Action/entity через константы. Actor info из context |
| `backend/internal/service/auth.go` | `AuthRepoFactory` добавляет `NewAuditRepo`. `Login`, `Logout`, `ResetPassword` — audit внутри транзакции через константы |
| `backend/internal/handler/server.go` | `AuditLogService` — только `List` |
| `backend/internal/handler/brand.go` | Удалить все `logAudit(...)`, чистить импорты |
| `backend/internal/handler/auth.go` | Удалить все `logAudit(...)`, чистить импорты |
| `backend/internal/handler/auditor.go` | **Удалить файл** |
| `backend/internal/service/brand_test.go` | Expectations: `Pool.Begin`, `factory.NewAuditRepo`, `auditRepo.Create` с константами и корректным actor. Test context с userID, role, IP |
| `backend/internal/service/auth_test.go` | Expectations: `Pool.Begin` для `Login`/`Logout`, audit с константами. Test context |
| `backend/cmd/api/main.go` | `r.Use(chiMiddleware.RealIP)` + `r.Use(middleware.ClientIP)` после recovery, перед API routes |

### Этап 6 — test.go рефакторинг

| Файл | Изменения |
|------|-----------|
| `backend/internal/handler/test.go` | `TestHandler` имплементирует `testapi.ServerInterface`. Request body — сгенерированные типы (`testapi.SeedUserRequest`), не ручные struct. `GetResetToken` принимает `params testapi.GetResetTokenParams` (email из wrapper). Response — `testapi.SeedUserResponse{Data: ...}`, не `map[string]any`. Зависимость `TestAuthService` интерфейс (объявленный здесь же) с единственным методом `SeedUser`. Использует те же `respondJSON`/`respondError`, что и prod-handler |
| `backend/internal/handler/server.go` | Удалить `SeedUser` из `AuthService` интерфейса |
| `backend/internal/handler/mocks/mock_auth_service.go` | Регенерация через `make generate-mocks` (убирает `SeedUser`) |
| `backend/cmd/api/main.go` | Для `ENVIRONMENT=local`: `testapi.HandlerFromMux(testHandler, r)` вместо ручных `r.Post/Get(...)`. `*service.AuthService` передаётся как `TestAuthService` (struct удовлетворяет обоим интерфейсам) |

### Этап 7 — рефакторинг unit-тестов handler'а

| Файл | Изменения |
|------|-----------|
| `backend/internal/handler/helpers_test.go` | **Новый**: `newTestRouter(t, server *Server) chi.Router` — оборачивает в `api.HandlerFromMuxWithBaseURL` / `api.HandlerFromMux` + middleware (ClientIP-stub для tests). `doJSON[Resp any](t, router, method, path, body any, reqMut ...func(*http.Request)) (*http.Response, Resp)` — маршалит body, делает запрос через `router.ServeHTTP`, анмаршалит в `Resp`. Хелпер для контекста: `withAdmin(r)`/`withManager(r)` через middleware stub или прямую инъекцию через фейковый auth middleware (для unit) |
| `backend/internal/handler/auth_test.go` | Полная переделка: все тесты используют `newTestRouter` + `doJSON[api.LoginResult]`. Request body — `json.Marshal(api.LoginRequest{Email: ..., Password: ...})`. Response — типизированный `api.LoginResult`, динамические поля (`refreshCookie.Expires`) — через `require.WithinDuration` и занулить, затем `require.Equal`. Добавить сценарии: service returns `domain.ErrUnauthorized`, `domain.ErrNotFound`, internal error |
| `backend/internal/handler/brand_test.go` | Полная переделка: типизированные request/response, через роутер. `AssignManager` — `tempPassword` через `require.NotEmpty` + занулить + `require.Equal`. `ListBrands` — проверка всего body. Добавить сценарии: `not found`, `conflict`, `internal error` |

### Этап 8 — финальные чистки

| Файл | Изменения |
|------|-----------|
| `backend/internal/handler/response.go` | Убрать импорт `pgx`. Убрать `errors.Is(err, pgx.ErrNoRows)` ветку. Вместо `//nolint:errcheck` ввести хелпер `encodeJSON(w, r, v)` — при ошибке `slog.Error("failed to encode response", "error", err, "path", r.URL.Path, "method", r.Method)`. Использовать в `respondJSON` и `writeError` |
| `backend/internal/handler/health.go` | Использовать `encodeJSON` (или локальный log-on-error pattern), убрать `//nolint:errcheck` |
| `backend/internal/handler/audit.go` | В `rawJSONToAny`: при ошибке `json.Unmarshal` — `slog.Error("failed to unmarshal audit log value", "error", err)` перед `return nil` |
| `backend/internal/repository/*.go` | Проверить обёртку `sql.ErrNoRows` в `domain.ErrNotFound` во всех методах возвращающих единичную запись |

---

## Файлы для создания

| Файл | Назначение |
|------|------------|
| `backend/api/openapi.yaml` | Перемещён из `api/openapi.yaml` через `git mv`, расширен named schemas + minLength |
| `backend/api/openapi-test.yaml` | Перемещён из `api/openapi-test.yaml`, расширен спекой test-endpoints |
| `backend/internal/testapi/server.gen.go` | Сгенерирован из `openapi-test.yaml` (chi-server, models) |
| `backend/internal/authz/brand.go` | `CanCreateBrand`, `CanListBrands`, `CanViewBrand`, `CanUpdateBrand`, `CanDeleteBrand`, `CanAssignManager`, `CanRemoveManager` |
| `backend/internal/authz/audit.go` | `CanListAuditLogs` |
| `backend/internal/authz/brand_test.go` | Unit-тесты brand-методов AuthzService |
| `backend/internal/authz/audit_test.go` | Unit-тесты audit-методов AuthzService |
| `backend/internal/middleware/client_ip.go` | Middleware-adapter для записи IP в context |
| `backend/internal/middleware/client_ip_test.go` | Unit-тесты middleware |
| `backend/internal/service/audit_constants.go` | Константы `AuditAction*` / `AuditEntityType*` |
| `backend/internal/handler/helpers_test.go` | `newTestRouter`, `doJSON[Resp]`, `withAdmin`/`withManager` |

---

## Файлы для удаления

| Файл | Причина |
|------|---------|
| `api/openapi.yaml` | Перемещён в `backend/api/` |
| `api/openapi-test.yaml` | Перемещён в `backend/api/` |
| `backend/internal/domain/roles.go` | Дубликат сгенерированных `api.UserRole` / `api.Admin` / `api.BrandManager` |
| `backend/internal/handler/auditor.go` | Аудит — ответственность сервисов, не handler |
| `backend/internal/authz/mocks/mock_brand_access_checker.go` | Интерфейс `BrandAccessChecker` заменён на `BrandService` |

---

## Шаги реализации

Этапы — порядок действий; коммит один в конце. После каждого этапа проверять `make test-unit-backend lint-backend`, чтобы билд оставался зелёным.

### Этап 1: Перенос api/ → backend/api/

1. [ ] `git mv api/ backend/api/`
2. [ ] Обновить `Makefile`: все пути к YAML
3. [ ] `make generate-api` — все генерации проходят
4. [ ] `make build-backend build-web build-tma` — собирается
5. [ ] `make test-unit-backend lint-backend` — зелёный

### Этап 2: Расширение OpenAPI + регенерация

6. [ ] `backend/api/openapi.yaml`: вынести все inline `data` в `components/schemas` (`LoginData`, `UserData`, `BrandData`, `BrandDetailData`, `ListBrandsData`, `AssignManagerData`, `ListAuditLogsData`, `MessageData`), в ответах — `$ref`. Добавить `minLength: 6` для `password` и `newPassword`
7. [ ] `backend/api/openapi-test.yaml`: описать `POST /test/seed-user`, `POST /test/seed-brand`, `GET /test/reset-tokens` с request/response типами
8. [ ] `Makefile`: добавить в `generate-api` генерацию `chi-server,models` из `openapi-test.yaml` в `backend/internal/testapi/`
9. [ ] `make generate-api` — все файлы перегенерированы
10. [ ] `handler/auth.go`, `handler/brand.go`, `handler/audit.go`: заменить inline `struct {...}{...}` на именованные типы (`api.LoginData`, `api.BrandData` и т.д.)
11. [ ] `handler/auth.go`: удалить `if len(req.NewPassword) < 6 {...}` — валидация в wrapper
12. [ ] `make build-backend build-web build-tma` — собирается (при сломе фронта — точечный фикс типов)
13. [ ] `make test-unit-backend lint-backend` — зелёный

### Этап 3: Удаление domain/roles.go, типизация RoleFromContext

14. [ ] `domain/user.go`: `Role api.UserRole`, импорт api
15. [ ] `middleware/auth.go`: `RoleFromContext()` → `api.UserRole`, при записи — `api.UserRole(role)`
16. [ ] `middleware/auth_test.go`: setup context с `api.UserRole`
17. [ ] `authz/authz.go`: `domain.RoleAdmin` → `api.Admin`
18. [ ] `service/brand.go`, `service/auth.go`: роли через `api.UserRole`/`api.Admin`
19. [ ] `handler/auth_test.go`, `handler/brand_test.go`: `domain.Role*` → `api.*`, строки в context → константы
20. [ ] Удалить `backend/internal/domain/roles.go`
21. [ ] Греп: `grep -r "domain.RoleAdmin\|domain.RoleBrandManager\|domain.UserRole" backend/` — пусто
22. [ ] `make test-unit-backend lint-backend` — зелёный

### Этап 4: AuthzService

23. [ ] `service/brand.go`: добавить `IsUserBrandManager(ctx, userID, brandID) (bool, error)`
24. [ ] `service/brand.go`: удалить `CanViewBrand`
25. [ ] `service/brand.go`: изменить `ListBrands(ctx, managerID *string)`
26. [ ] `service/brand_test.go`: удалить `TestBrandService_CanViewBrand`, обновить `TestBrandService_ListBrands`, добавить `TestBrandService_IsUserBrandManager`
27. [ ] Переписать `authz/authz.go`: `AuthzService` struct, `BrandService` interface, `NewAuthzService`. Удалить `RequireAdmin`, `CanApproveCreator`, `CanManageCampaign`, `CanManageBrand`, `BrandAccessChecker`
28. [ ] Создать `authz/brand.go` (7 методов), `authz/audit.go` (`CanListAuditLogs`)
29. [ ] Создать `authz/brand_test.go`, `authz/audit_test.go` (по паттерну `backend-testing-unit.md`)
30. [ ] `handler/server.go`: `AuthzService` interface, `authzService` зависимость, убрать `CanViewBrand` из `BrandService`, сигнатура `ListBrands(ctx, managerID *string)`
31. [ ] `handler/brand.go`: `authz.RequireAdmin` → `s.authzService.Can*`, `ListBrands` — через `CanListBrands`, убрать импорт `authz`
32. [ ] `handler/audit.go`: `authz.RequireAdmin` → `s.authzService.CanListAuditLogs`
33. [ ] `handler/brand_test.go`: mock AuthzService, новая сигнатура `NewServer`
34. [ ] `handler/auth_test.go`: новая сигнатура `NewServer` (authzService=nil для auth)
35. [ ] `cmd/api/main.go`: создать AuthzService, передать в handler
36. [ ] `make generate-mocks`
37. [ ] `make test-unit-backend lint-backend test-e2e-backend` — зелёный

### Этап 5: Аудит в сервисах + константы

38. [ ] Создать `middleware/client_ip.go` (`chi/middleware.RealIP` + adapter, `ClientIPFromContext`)
39. [ ] Создать `middleware/client_ip_test.go`
40. [ ] `cmd/api/main.go`: `r.Use(chiMiddleware.RealIP)` + `r.Use(middleware.ClientIP)`
41. [ ] Создать `service/audit_constants.go` (`AuditAction*`, `AuditEntityType*`)
42. [ ] `service/audit.go`: удалить `AuditEntry` тип и `Log()` метод, оставить `List`
43. [ ] `service/brand.go`: `NewAuditRepo` в factory, write-методы в `WithTx` + audit через константы
44. [ ] `service/auth.go`: `NewAuditRepo` в factory, `Login`/`Logout` в `WithTx`, `ResetPassword` — audit в существующий WithTx
45. [ ] `handler/brand.go`, `handler/auth.go`: удалить все `logAudit(...)`, чистить импорты
46. [ ] `handler/server.go`: `AuditLogService` — только `List`
47. [ ] Удалить `handler/auditor.go`
48. [ ] `service/brand_test.go`, `service/auth_test.go`: `Pool.Begin`, `NewAuditRepo`, `auditRepo.Create` с константами и actor info
49. [ ] `handler/brand_test.go`, `handler/auth_test.go`: убрать audit-мок/проверки
50. [ ] `make generate-mocks`
51. [ ] `make test-unit-backend lint-backend test-e2e-backend` — зелёный

### Этап 6: test.go рефакторинг

52. [ ] `handler/server.go`: удалить `SeedUser` из `AuthService` интерфейса
53. [ ] `handler/test.go`:
    - Объявить `TestAuthService` интерфейс (с `SeedUser`)
    - `TestHandler` имплементирует `testapi.ServerInterface`
    - Методы принимают сгенерированные request-типы + wrapper
    - `GetResetToken` — email через `params testapi.GetResetTokenParams`
    - Response через `testapi.*Response{Data: ...}`
    - Использовать `respondJSON`/`respondError`
54. [ ] `cmd/api/main.go`: `testapi.HandlerFromMux(testHandler, r)` для `ENVIRONMENT=local`, удалить ручные `r.Post/Get("/test/*", ...)`. `*service.AuthService` передаётся в `NewTestHandler` как `TestAuthService`
55. [ ] `make generate-mocks` (исчезнет `MockAuthService.SeedUser`)
56. [ ] `make test-unit-backend lint-backend test-e2e-backend` — зелёный

### Этап 7: Рефакторинг unit-тестов handler'а

57. [ ] Создать `handler/helpers_test.go`:
    - `newTestRouter(t *testing.T, s *Server) chi.Router` — регистрирует `api.HandlerFromMux(s, chi.NewRouter())`, подключает middleware-stub для ClientIP/auth (чтобы context нёс нужные userID/role без реального auth-middleware)
    - `doJSON[Resp any](t, router, method, path, body any, opts ...reqOpt) (status int, resp Resp, cookies []*http.Cookie)` — маршалит body, гоняет через роутер, анмаршалит response
    - `withAdminCtx(r) *http.Request`, `withManagerCtx(r, userID)` — инъекция userID/role в context
58. [ ] Полностью переписать `handler/auth_test.go`:
    - Все тесты через `newTestRouter` + `doJSON[api.LoginResult]` / `api.UserResponse` / `api.MessageResponse`
    - Request — типизированные структуры из `api/`
    - Response — типизированный анмаршал
    - Динамические поля (`refreshCookie.Expires`, `expiresAt`) — `require.WithinDuration` + занулить + `require.Equal` целиком
    - Добавить сценарии: service returns `ErrUnauthorized`, `ErrNotFound`, internal error; порядок `t.Run` по течению кода
59. [ ] Полностью переписать `handler/brand_test.go`:
    - Аналогично auth_test
    - `AssignManager`: `tempPassword` — `require.NotEmpty` + занулить + `require.Equal`
    - `ListBrands`: проверка всего body
    - Добавить сценарии: `not found`, `conflict`, `internal error`
60. [ ] Греп `strings.NewReader(` в `handler/*_test.go` — пусто
61. [ ] `make test-unit-backend lint-backend test-e2e-backend` — зелёный, покрытие ≥80%

### Этап 8: Финальные чистки

62. [ ] Проверить `repository/*.go`: везде ли `sql.ErrNoRows` → `domain.ErrNotFound`. Добавить обёртку где нет
63. [ ] `handler/response.go`: убрать `pgx` импорт и ветку `errors.Is(err, pgx.ErrNoRows)`. Ввести `encodeJSON(w, r, v)` с `slog.Error` вместо `//nolint:errcheck`. Использовать в `respondJSON`, `writeError`
64. [ ] `handler/health.go`: убрать `//nolint:errcheck`, логировать ошибку encode
65. [ ] `handler/audit.go`: в `rawJSONToAny` — `slog.Error` перед `return nil` при ошибке unmarshal
66. [ ] `handler/test.go`: убрать `//nolint:errcheck` (через `encodeJSON`)
67. [ ] `make test-unit-backend lint-backend test-e2e-backend` — зелёный

### Финальная проверка (агент останавливается здесь, без коммита)

68. [ ] Грепы (все пусто):
    - `grep -r "authz.RequireAdmin\|authz.CanManageBrand\|authz.CanApprove" backend/`
    - `grep -r "logAudit" backend/`
    - `grep -r "domain.RoleAdmin\|domain.RoleBrandManager\|domain.UserRole" backend/`
    - `grep -r "pgx" backend/internal/handler/`
    - `grep -r "//nolint:errcheck" backend/internal/handler/`
    - `grep -rn 'strings.NewReader(' backend/internal/handler/ | grep _test.go`
    - `grep -n 'map\[string\]any' backend/internal/handler/test.go`
    - `grep -n 'r\.URL\.Query()\.Get' backend/internal/handler/`
    - `grep -n 'SeedUser' backend/internal/handler/server.go`
    - `grep -rEn '"login"|"logout"|"password_reset"|"brand_create"|"brand_update"|"brand_delete"|"manager_assign"|"manager_remove"' backend/internal/service/`
69. [ ] Локально: `make run-backend` + smoke через e2e
70. [ ] Отчитаться пользователю: сводка `git status` + `git diff --stat`, перечень изменённых/новых/удалённых файлов, результаты `make test-unit-backend`/`lint-backend`/`test-e2e-backend`. **Коммит не делать.** Решение о коммите и мерже PR — за Alikhan

---

## Стратегия тестирования

### Unit-тесты

**authz/brand_test.go** (новый):
- Каждый `Can*` метод: admin → pass, brand_manager как manager → pass (где актуально), brand_manager не manager → `ErrForbidden`, ошибка из brandService → обёртка
- `TestAuthzService_CanListBrands`: admin → `canViewAll=true`, brand_manager → `canViewAll=false, userID=<uid>`
- Порядок `t.Run`: ранние выходы (не admin), happy path в конце

**authz/audit_test.go** (новый):
- `TestAuthzService_CanListAuditLogs`: admin → nil, brand_manager → `ErrForbidden`

**service/brand_test.go** (обновлён):
- Удалить `TestBrandService_CanViewBrand`
- `TestBrandService_ListBrands`: `managerID == nil` → List, `managerID != nil` → ListByUser
- `TestBrandService_IsUserBrandManager` (новый): true, false, error
- Write-тесты: `Pool.Begin`, `factory.NewAuditRepo`, `auditRepo.Create` с константами `AuditAction*` и корректным actor (userID, role, IP из context)

**service/auth_test.go** (обновлён):
- `TestAuthService_Login` — `Pool.Begin`, audit в WithTx с `AuditActionLogin`
- `TestAuthService_Logout` — `Pool.Begin`, audit в WithTx
- `TestAuthService_ResetPassword` — audit внутри существующего WithTx

**handler/auth_test.go** (полностью переписан):
- Через `newTestRouter` + `doJSON[Resp]`
- Все request body — типизированные структуры `api.*Request`
- Все response — типизированные `api.*Result`/`api.*Response`, полный `require.Equal` (после зануления динамических полей)
- Сценарии: `success`, `invalid JSON` (ожидается 400 от wrapper для некорректного JSON), `validation error` (422 от wrapper для коротких паролей благодаря `minLength: 6`), `unauthorized`, `not found`, `internal error`
- Cookies: `refresh_token` — `HttpOnly`, `Secure` по конфигу, `Expires` через `require.WithinDuration`
- Mock AuthzService где handler его использует

**handler/brand_test.go** (полностью переписан):
- Через `newTestRouter` + `doJSON[Resp]`
- Mock `AuthzService.Can*`-методов вместо context-based role checks
- Проверка порядка: handler вызывает authz до service
- `AssignManager`: `tempPassword` — `require.NotEmpty` → занулить → `require.Equal`
- `ListBrands`: полная проверка body, не только status
- Сценарии: success, forbidden (от authz), validation error, not found, conflict, internal error

**middleware/client_ip_test.go** (новый):
- X-Forwarded-For → первый IP
- X-Real-IP → значение
- RemoteAddr fallback (с портом и без)
- Через `chi.RealIP` + наш adapter

**handler/test.go** (test-эндпоинты):
- Отдельных unit-тестов не требуется, покрытие — через E2E (test-endpoints используются в `backend/e2e/`). Если покрытие упадёт ниже 80% — добавить точечные тесты через `testapi.HandlerFromMux`

### E2E-тесты

Существующие `backend/e2e/audit/`, `backend/e2e/auth/`, `backend/e2e/brand/` должны пройти без изменений. HTTP-контракт сохраняется (названия data-обёрток меняются только в коде, JSON-форма та же: `{"data": {...}}`).

Test-клиент (`backend/e2e/testclient/`) перегенерируется из расширенного `openapi-test.yaml` — возможны мелкие правки в вызовах (например, `SeedUserRequest` вместо собственного `map[string]any`). Обновить вызовы в `backend/e2e/testutil/` по необходимости.

### Проверки после каждого этапа
- `make generate-api` (этапы 1, 2)
- `make generate-mocks` (этапы 4, 5, 6)
- `make test-unit-backend` (каждый этап)
- `make lint-backend` (каждый этап)
- `make test-e2e-backend` (после этапов 4, 5, 6, 7)
- `make build-web build-tma` (после этапа 2 — проверка что фронт собирается)

---

## Оценка рисков

| Риск | Вероятность | Митигация |
|------|-------------|-----------|
| Поломка E2E audit-тестов из-за транзакционности | Средняя | E2E проверяет факт записи, не поведение при сбое. Прогнать e2e после этапа 5 |
| Frontend сломается из-за расширения OpenAPI (named schemas) | Средняя | openapi-typescript сохраняет `paths[...]['responses'][...]` как источник типов — структура JSON та же. Если фронт обращается напрямую к `schemas['Foo']` — точечный фикс имени. Проверка `make build-web build-tma` сразу после этапа 2 |
| Mock-генерация не подхватит новый `AuthzService` | Низкая | `.mockery.yaml` имеет `all: true`, пакет `authz` уже в списке |
| Аудит в WithTx → лишний INSERT | Низкая | Overhead ~1ms, гарантия консистентности |
| Пропустим место с `domain.RoleAdmin` при замене | Низкая | После удаления `domain/roles.go` компилятор укажет все места |
| Pool.Begin ожидается в service-тестах, где раньше не было | Средняя | Добавить в каждый write-тест после WithTx |
| `chi/middleware.RealIP` не настроен на trusted proxies | Низкая | В MVP — все запросы за Caddy/Nginx, X-Forwarded-For доверяем |
| Сбой аудита ломает login | Принято | Требование `backend-transactions.md`. Fire-and-forget запрещён |
| Рефакторинг тестов handler'а раскроет ранее незамеченные баги | Средняя | Хорошо — именно то, для чего стандарт и нужен. При обнаружении — точечный фикс в коде, не тесте |
| Regenerate `openapi-test.yaml` сломает `backend/e2e/testclient` usage | Низкая | В этапе 6 сверить с `backend/e2e/testutil/*` и поправить вызовы. Ручная работа, но маленькая |
| `ENVIRONMENT=local` guard для test-routes — регрессия | Низкая | Перенос с ручных `r.Post("/test/...")` на `testapi.HandlerFromMux` не меняет guard — остаётся проверка `if cfg.Environment == "local"` в `main.go` |
| `minLength: 6` в OpenAPI изменит код ответа для коротких паролей | Средняя | Раньше `422 Unprocessable Entity` от handler, теперь — `400 Bad Request` от wrapper (или `422` если настроить). Проверить e2e-тесты и при необходимости настроить error handler в wrapper на `422` (через `api.HandlerWithOptions{ErrorHandlerFunc}`) |

---

## План отката

Работа ведётся на ветке `alikhan/staging-cicd` (PR уже открыт в main). Агент коммит не создаёт — откат до коммита делается через `git checkout -- <file>` или `git restore <file>` (Alikhan, вручную).

Дальнейшие шаги после того, как Alikhan закоммитит и смержит:
- **Если после мержа PR найдена проблема**: `git revert <merge-commit>` на main, rebuild Docker, передеплой Dokploy. Zero-downtime (по `feedback_zero_downtime.md`)
- **Если проблема точечная**: следующим коммитом точечный фикс, не откат
