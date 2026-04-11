# План: приведение backend к стандартам

## Обзор

Устранение 19 нарушений стандартов (4 критичных, 13 важных, 2 мелких) за 8 атомарных шагов. Каждый шаг — один коммит с рабочим билдом и тестами. Порядок: сначала инфраструктура (конфиг), потом данные (repository), потом API-слой (handler + ServerInterface), последними — тесты и cleanup.

---

## Шаги

### Шаг 1: Config — библиотека + ENVIRONMENT + strict parsing

**Покрывает**: C2, C4, I5 (config часть), M2
**Файлы**:
- `backend/internal/config/config.go` — полная переписка
- `backend/cmd/api/main.go` — использование Environment
- `backend/go.mod` — новая зависимость

**Что делаем**:
1. `go get github.com/caarlos0/env/v11`
2. Переписать `Config` struct: убрать `getEnv`/`getBoolEnv`/`getIntEnv`/`getDurationEnv`, заменить на struct tags `env:"..." envDefault:"..."`
3. Добавить `Environment string` поле (`env:"ENVIRONMENT" envDefault:"local"`, валидные: `local`, `staging`, `production`)
4. Вычислять из Environment: `CookieSecure` (true если не local), `EnableTestEndpoints` (true только для local), log level default
5. Убрать `hasLocalhostOrigin()` — логика cookie secure теперь от Environment
6. Убрать `splitComma()` — `env` парсит слайсы из коробки (`envSeparator:","`)
7. Невалидный env var → `Load()` возвращает error → приложение не стартует
8. Определить константы log level в config пакете или использовать `slog.Level` напрямую
9. Обновить `main.go`: убрать string switch для log level, использовать `cfg.LogLevel` (уже `slog.Level`)
10. Обновить `docker-compose.yml` и `.env.example` (если есть) — добавить `ENVIRONMENT=local`

**Стандарт**: `docs/standards/security.md`, `docs/standards/backend-errors.md`, `docs/standards/backend-libraries.md`, `docs/standards/backend-constants.md`

**Проверка**:
- `go build ./...` — компилируется
- `make test` — тесты проходят
- При `ENVIRONMENT=invalid` → приложение не стартует
- При `BCRYPT_COST=abc` → приложение не стартует (не молчаливый fallback)

---

### Шаг 2: Service — bcryptCost через DI

**Покрывает**: I6
**Файлы**:
- `backend/internal/service/auth.go` — убрать глобальную var, добавить поле
- `backend/internal/service/brand.go` — добавить bcryptCost через конструктор
- `backend/internal/service/auth_test.go` — обновить TestMain
- `backend/internal/service/brand_test.go` — обновить конструкторы
- `backend/cmd/api/main.go` — передать bcryptCost в оба сервиса

**Что делаем**:
1. Удалить `var bcryptCost = 12` из `auth.go`
2. Добавить `bcryptCost int` поле в `AuthService` и `BrandService`
3. Обновить конструкторы: `NewAuthService(..., cost int)`, `NewBrandService(..., bcryptCost int)`
4. Все места использования `bcryptCost` → `s.bcryptCost`
5. Обновить `main.go`: передать `cfg.BcryptCost` обоим сервисам
6. Тесты: убрать мутацию глобальной переменной в TestMain, передавать `bcrypt.MinCost` в конструктор

**Стандарт**: `docs/standards/backend-design.md`

**Проверка**:
- `go build ./...`
- `make test` — все тесты проходят
- `go vet ./...` — нет warnings

---

### Шаг 3: Repository — экспорт констант и rename

**Покрывает**: I2, I8
**Файлы**:
- `backend/internal/repository/user.go` — переименовать константы
- `backend/internal/repository/brand.go` — переименовать константы
- `backend/internal/repository/audit.go` — переименовать константы
- `backend/internal/authz/authz.go` — заменить строковые литералы на импортированные константы

**Что делаем**:
1. В каждом файле repository переименовать:
   - `tableUsers` → `TableUsers`, `tableBrands` → `TableBrands`, и т.д.
   - `colUserID` → `UserColumnID`, `colUserEmail` → `UserColumnEmail`, и т.д.
   - `colBrandID` → `BrandColumnID`, `colBMBrandID` → `BrandManagerColumnBrandID`, и т.д.
2. Обновить все SQL-запросы в repository, где используются старые имена
3. В `authz/authz.go`: заменить `"brand_managers"` → `repository.TableBrandManagers`, `"user_id = ? AND brand_id = ?"` → использовать константы + squirrel
4. Тесты repository: оставить литералы в SQL assertions (по стандарту — двойная проверка)

**Стандарт**: `docs/standards/naming.md`, `docs/standards/backend-repository.md`, `docs/standards/backend-constants.md`

**Проверка**:
- `go build ./...`
- `make test` — тесты подтверждают что SQL не изменился (литералы в assertions ловят регрессии)
- `grep -r 'tableUsers\|colUser\|tableBrands\|colBrand\|tableAudit\|colAudit' backend/internal/repository/` — 0 результатов

---

### Шаг 4: Repository — stom + dual tags + precomputed columns

**Покрывает**: I3
**Файлы**:
- `backend/go.mod` — `go get github.com/elgris/stom`
- `backend/internal/repository/helpers.go` — новый файл: tagMap, toMap, insertEntities, sortColumns
- `backend/internal/repository/user.go` — dual tags, precomputed columns
- `backend/internal/repository/brand.go` — dual tags, precomputed columns
- `backend/internal/repository/audit.go` — dual tags, precomputed columns

**Что делаем**:
1. Добавить зависимость `github.com/elgris/stom`
2. Создать `helpers.go` с хелперами из стандарта: `tagMap`, `toMap`, `insertEntities`, `sortColumns`
3. Добавить `insert` тег к Row structs (исключая auto-generated поля: id, created_at, updated_at):
   ```go
   type UserRow struct {
       ID           string    `db:"id"`
       Email        string    `db:"email"       insert:"email"`
       PasswordHash string    `db:"password_hash" insert:"password_hash"`
       Role         string    `db:"role"         insert:"role"`
       CreatedAt    time.Time `db:"created_at"`
       UpdatedAt    time.Time `db:"updated_at"`
   }
   ```
4. Для каждой Row struct — precomputed column lists:
   ```go
   var userSelectColumns = sortColumns(stom.MustNewStom(UserRow{}).SetTag("db").TagValues())
   var userInsertMapper  = stom.MustNewStom(UserRow{}).SetTag("insert")
   var userInsertColumns = sortColumns(userInsertMapper.TagValues())
   ```
5. Рефакторинг запросов:
   - SELECT: `.Select(userSelectColumns...)`
   - INSERT: `SetMap(toMap(row, userInsertMapper))` вместо `.Columns(...).Values(...)`
6. Специфичные запросы (частичный SELECT, нестандартный INSERT) — оставить с конкретными константами

**Стандарт**: `docs/standards/backend-repository.md`

**Проверка**:
- `make test` — SQL assertions в тестах подтверждают что запросы корректны
- Порядок колонок в SELECT может измениться (сортировка), тесты нужно обновить соответственно
- `go vet ./...`

---

### Шаг 5: Repository — pointer returns + cascade

**Покрывает**: I4
**Файлы**:
- `backend/internal/repository/user.go` — `*UserRow` вместо `UserRow`
- `backend/internal/repository/brand.go` — `*BrandRow` вместо `BrandRow`
- `backend/internal/repository/audit.go` — `*AuditLogRow` вместо `AuditLogRow`, `[]*AuditLogRow`
- `backend/internal/dbutil/db.go` — `One[T]` возвращает `*T`, `Many[T]` возвращает `[]*T`
- `backend/internal/service/auth.go` — обновить интерфейс UserRepo и вызовы
- `backend/internal/service/brand.go` — обновить интерфейс BrandRepo и вызовы
- `backend/internal/service/audit.go` — обновить интерфейс AuditRepo
- `backend/internal/handler/*.go` — обновить интерфейсы и вызовы
- Все `*_test.go` — обновить mock expectations и assertions
- `backend/internal/service/mocks/` — перегенерировать

**Что делаем**:
1. Изменить `dbutil.One[T]` → возвращает `(*T, error)` (аллокация в хелпере)
2. Изменить `dbutil.Many[T]` → возвращает `([]*T, error)`
3. Обновить все методы repository: возвращать `*UserRow`, `*BrandRow` и т.д.
4. Обновить интерфейсы в service: `GetByEmail` → `(*UserRow, error)`
5. Обновить реализации service: работа с указателями
6. Обновить интерфейсы в handler: аналогично
7. Перегенерировать моки: `cd backend && mockery`
8. Обновить тесты: mock expectations возвращают указатели

**Стандарт**: `docs/standards/backend-repository.md`

**Проверка**:
- `go build ./...`
- `make test`
- `cd backend && mockery` — моки актуальны

---

### Шаг 6: Handler — ServerInterface + api types + authz + domain cleanup

**Покрывает**: C1, C3, I1, I7, M1
**Файлы**:
- `backend/internal/handler/server.go` — новый: единый Handler struct, реализует `api.ServerInterface`
- `backend/internal/handler/auth.go` — переписать как методы Server, использовать api types
- `backend/internal/handler/brand.go` — переписать как методы Server, убрать `chi.URLParam`
- `backend/internal/handler/audit.go` — переписать, принимать `api.ListAuditLogsParams`
- `backend/internal/handler/health.go` — метод Server
- `backend/internal/handler/response.go` — использовать `api.ErrorResponse` вместо `domain.APIResponse`
- `backend/internal/handler/auditor.go` — переименовать поле
- `backend/internal/handler/constants.go` — без изменений
- `backend/internal/domain/brand.go` — удалить (дубликат api types)
- `backend/internal/domain/response.go` — удалить (заменён на api.ErrorResponse)
- `backend/internal/authz/authz.go` — добавить `RequireAdmin(ctx)` метод
- `backend/internal/middleware/auth.go` — добавить BearerAuthScopes-based middleware
- `backend/cmd/api/main.go` — заменить ручные роуты на `api.HandlerWithOptions`

**Что делаем**:

1. **Единый Handler struct**:
   ```go
   type Server struct {
       authService  AuthService    // renamed from Auth
       brandService BrandService   // renamed from Brands
       auditService AuditLogService
       auditor      Auditor
       cookieSecure bool
   }
   var _ api.ServerInterface = (*Server)(nil) // compile-time check
   ```

2. **ServerInterface методы** — новые сигнатуры:
   - `Login(w, r)` — парсит `api.LoginRequest` через `json.Decode`, ответ через `api.LoginResult`
   - `GetBrand(w, r, brandID string)` — brandID приходит аргументом, не из chi.URLParam
   - `ListAuditLogs(w, r, params api.ListAuditLogsParams)` — query params уже распарсены
   - Ответы — через сгенерированные типы (`api.BrandResult`, `api.LoginResult` и т.д.)

3. **Авторизация**:
   - Добавить `authz.RequireAdmin(ctx context.Context) error` — проверяет роль из context
   - В хендлерах заменить `if role != string(domain.RoleAdmin)` → `if err := authz.RequireAdmin(r.Context()); err != nil`
   - Auth middleware: создать `AuthFromScopes(validator TokenValidator)` middleware — проверяет наличие `BearerAuthScopes` в context, если есть — валидирует токен

4. **Роутинг в main.go**:
   ```go
   server := handler.NewServer(authSvc, brandSvc, auditSvc, cfg.CookieSecure)
   api.HandlerWithOptions(server, api.ChiServerOptions{
       BaseRouter: r,
       Middlewares: []api.MiddlewareFunc{
           middleware.AuthFromScopes(tokenSvc),
       },
       ErrorHandlerFunc: handler.HandleParamError,
   })
   // Test endpoints остаются ручными (не в OpenAPI)
   ```

5. **Domain cleanup**:
   - Удалить `domain/brand.go` — типы `Brand`, `ManagerInfo` дублируют `api.Brand`, `api.ManagerInfo`
   - Удалить `domain/response.go` — `APIResponse`/`APIError` дублируют `api.ErrorResponse`/`api.APIError`
   - В handler/response.go: `respondError` формирует `api.ErrorResponse` вместо `domain.APIResponse`
   - Handler больше не импортирует `repository` — интерфейсы работают с указателями на repo types, но handler конвертирует в api types

6. **Нейминг** (M1):
   - Интерфейсы: `Auth` → `AuthService`, `Brands` → `BrandService`, `AuditLogs` → `AuditLogService`
   - Поля: `auth` → `authService`, `brands` → `brandService`

**Стандарт**: `docs/standards/backend-codegen.md`, `docs/standards/backend-architecture.md`, `docs/standards/backend-design.md`, `docs/standards/naming.md`

**Проверка**:
- `go build ./...`
- `make test` — ⚠️ старые handler тесты сломаются (вызывали методы напрямую), будут починены в Шаге 7
- E2E тесты проходят (если сервер работает — API контракт не изменился)
- Ручная проверка: `curl` на основные endpoint'ы через запущенный сервер
- `grep -r 'chi.URLParam\|r.URL.Query' backend/internal/handler/` — 0 результатов (кроме test.go)
- `grep 'repository\.' backend/internal/handler/*.go` — 0 результатов (handler не импортирует repository)

---

### Шаг 7: Tests — полная переписка по стандартам

**Покрывает**: I10, I11, I12, I13
**Файлы**:
- `backend/internal/handler/auth_test.go` — переписать
- `backend/internal/handler/brand_test.go` — переписать
- `backend/internal/handler/helpers_test.go` — новый: общие хелперы
- `backend/internal/service/auth_test.go` — assert→require, нейминг, группировка
- `backend/internal/service/brand_test.go` — assert→require, нейминг, группировка
- `backend/internal/service/token_test.go` — assert→require, нейминг
- `backend/internal/repository/*_test.go` — assert→require, нейминг
- `backend/internal/middleware/*_test.go` — assert→require, нейминг

**Что делаем**:

1. **Handler tests — через роутер с ServerInterfaceWrapper**:
   ```go
   func setupRouter(t *testing.T) (chi.Router, *mocks.MockAuthService, *mocks.MockBrandService) {
       auth := mocks.NewMockAuthService(t)
       brands := mocks.NewMockBrandService(t)
       server := NewServer(auth, brands, nil, false)
       r := chi.NewRouter()
       api.HandlerWithOptions(server, api.ChiServerOptions{
           BaseRouter: r,
           Middlewares: []api.MiddlewareFunc{
               middleware.AuthFromScopes(tokenSvc),
           },
       })
       return r, auth, brands
   }
   ```

2. **Типизированные request/response**:
   ```go
   // Вместо: doRequest(h.Login, "POST", `{"email":"test@example.com","password":"password123"}`)
   body, _ := json.Marshal(api.LoginRequest{Email: "test@example.com", Password: "password123"})
   req := httptest.NewRequest("POST", "/auth/login", bytes.NewReader(body))
   ```

3. **assert → require** во всех файлах

4. **Нейминг Test{Struct}_{Method} + t.Run**:
   ```go
   func TestServer_Login(t *testing.T) {
       t.Parallel()
       t.Run("success", func(t *testing.T) { ... })
       t.Run("invalid JSON", func(t *testing.T) { ... })
       t.Run("missing fields", func(t *testing.T) { ... })
       t.Run("wrong credentials", func(t *testing.T) { ... })
       t.Run("email normalization", func(t *testing.T) { ... })
   }
   ```

5. **Service tests — группировка**:
   ```go
   func TestAuthService_Login(t *testing.T) {
       t.Parallel()
       t.Run("user not found", func(t *testing.T) { ... })
       t.Run("wrong password", func(t *testing.T) { ... })
       t.Run("success", func(t *testing.T) { ... })
   }
   ```

6. **Repository tests** — assert→require, нейминг (SQL assertions остаются литералами)

**Стандарт**: `docs/standards/backend-testing-unit.md`

**Проверка**:
- `make test` — все тесты проходят
- `go test -race ./...` — нет race conditions
- `grep 'assert\.' backend/internal/` | grep -v vendor — 0 результатов (только require)

---

### Шаг 8: Cleanup — TODO issues, closer, infrastructure

**Покрывает**: I9, I5 (closer)
**Файлы**:
- `backend/internal/authz/authz.go` — TODO с номером issue
- `backend/internal/closer/` — оценить замену на `oklog/run` или оставить с обоснованием

**Что делаем**:
1. Создать GitHub issue для TODO в authz.go: "Implement CanManageCampaign authorization"
2. Заменить `// TODO: implement...` → `// TODO(#N): implement...`
3. Closer — оценить:
   - Если `oklog/run` или `errgroup` покрывает use case — заменить
   - Если closer достаточно прост (57 строк) и тестирован — оставить с комментарием обоснования
   - Решение: closer — пограничный случай. 57 строк, тестирован, делает одну вещь. Рекомендация: оставить, т.к. замена на `oklog/run` потребует изменения архитектуры main.go без реальной выгоды

**Стандарт**: `docs/standards/naming.md`, `docs/standards/backend-libraries.md`

**Проверка**:
- `grep 'TODO[^(]' backend/` — 0 результатов (все TODO с номером issue)
- `go build ./...`

---

## Порядок выполнения

```
Шаг 1 (config)
    ↓
Шаг 2 (bcryptCost DI)
    ↓
Шаг 3 (repo constants)  ← независим от 1-2, но логичнее после
    ↓
Шаг 4 (repo stom)       ← зависит от 3 (использует новые имена)
    ↓
Шаг 5 (repo pointers)   ← зависит от 4 (работает с теми же файлами)
    ↓
Шаг 6 (handler rewrite) ← зависит от 5 (новые сигнатуры)
    ↓
Шаг 7 (tests)           ← зависит от 6 (handler API изменён)
    ↓
Шаг 8 (cleanup)         ← независим, но после всего
```

Шаги 1 и 2 можно делать параллельно (разные файлы). Шаги 3-7 — строго последовательно.

## Проверка после всех шагов

- [ ] `go build ./...` — компилируется
- [ ] `make test` — все тесты проходят
- [ ] `go test -race ./...` — нет race conditions
- [ ] `go vet ./...` — нет warnings
- [ ] `golangci-lint run` — линтер проходит
- [ ] E2E тесты проходят (`make test-e2e`)
- [ ] Нет импорта `repository` в `handler/` (кроме test.go)
- [ ] Нет `chi.URLParam` и `r.URL.Query` в handler/ (кроме test.go)
- [ ] Нет `assert.` в тестах (только `require.`)
- [ ] Нет TODO без номера issue
- [ ] Все константы repository экспортированы
- [ ] `domain/` содержит только errors.go и roles.go
