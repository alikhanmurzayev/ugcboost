# Разведка: рефакторинг handler/service под стандарты

## Резюме запроса

Рефакторинг слоёв handler и service в бэкенде по трём направлениям:
1. **Авторизация** — собрать в AuthzService, убрать из хендлеров и бизнес-сервисов
2. **Аудит-логи** — перенести из хендлеров в сервисы, писать внутри транзакций
3. **Папка api/** — перенести OpenAPI-контракт из корня в `backend/api/`

---

## 1. Авторизация: текущее состояние и нарушения

### Что есть сейчас

**authz/authz.go** — пакет со standalone-функциями (не сервис):
- `RequireAdmin(ctx)` — достаёт роль из context, сравнивает с admin
- `CanManageBrand(ctx, checker, userID, role, brandID)` — проверяет admin или IsManager
- `CanApproveCreator(role)` — сравнивает с admin
- `CanManageCampaign()` — TODO-заглушка

**handler/brand.go** — прямые вызовы `authz.RequireAdmin()`:
- `CreateBrand` (строка 17)
- `UpdateBrand` (строка 127)
- `DeleteBrand` (строка 157)
- `AssignManager` (строка 182)
- `RemoveManager` (строка 227)

**handler/audit.go** — `authz.RequireAdmin()` (строка 14)

**handler/brand.go** — хендлер вызывает `s.brandService.CanViewBrand()` (строка 80) — авторизация размазана между хендлером и сервисом

**service/brand.go** — бизнес-сервис делает role-check:
- `ListBrands()` (строка 65): `if role == string(domain.RoleAdmin)` — решает что возвращать
- `CanViewBrand()` (строки 175-188) — проверяет admin или IsManager

**domain/roles.go** — ручной дубликат сгенерированных констант:
- `RoleAdmin`, `RoleBrandManager` — дублирует `api.Admin`, `api.BrandManager` из `server.gen.go`

**middleware/auth.go** — `RoleFromContext()` возвращает `string` вместо типизированного `api.UserRole`

### Нарушения стандартов

**backend-architecture.md** (строка 29):
> "Вся авторизационная логика — в отдельном сервисе (AuthzService). Прямые сравнения ролей в хендлерах запрещены."

- `authz` — не сервис, а набор функций
- Хендлеры напрямую вызывают `authz.RequireAdmin()` — это прямое сравнение ролей
- BrandService содержит authz-логику (`CanViewBrand`, `ListBrands`)

**backend-codegen.md**:
> "Ручные дубликаты запрещены."

- `domain/roles.go` дублирует сгенерированные `api.UserRole`, `api.Admin`, `api.BrandManager`

### Целевое состояние

**Удалить `domain/roles.go`** — везде использовать `api.UserRole`, `api.Admin`, `api.BrandManager`.

**middleware/auth.go** — `RoleFromContext()` возвращает `api.UserRole`, не `string`:
```go
func RoleFromContext(ctx context.Context) api.UserRole {
    v, _ := ctx.Value(ContextKeyRole).(api.UserRole)
    return v
}
```

**authz/authz.go** — AuthzService как struct с DI. Зависимости именуются как сервисы (не "чекеры"):
```go
// BrandService defines what AuthzService needs from the brand service.
type BrandService interface {
    IsUserBrandManager(ctx context.Context, userID, brandID string) (bool, error)
}

type AuthzService struct {
    brandService BrandService
}

func NewAuthzService(brandService BrandService) *AuthzService { ... }
```

**Методы именуются по конкретным API-действиям** — отражают что пользователь пытается сделать. Каждый метод сам достаёт userID/role из context:

```go
// authz/brand.go
func (a *AuthzService) CanCreateBrand(ctx context.Context) error {
    if middleware.RoleFromContext(ctx) != api.Admin {
        return domain.ErrForbidden
    }
    return nil
}

func (a *AuthzService) CanViewBrand(ctx context.Context, brandID string) error {
    if middleware.RoleFromContext(ctx) == api.Admin {
        return nil
    }
    userID := middleware.UserIDFromContext(ctx)
    ok, err := a.brandService.IsUserBrandManager(ctx, userID, brandID)
    if err != nil { return fmt.Errorf("check brand access: %w", err) }
    if !ok { return domain.ErrForbidden }
    return nil
}

func (a *AuthzService) CanListBrands(ctx context.Context) (canViewAll bool, userID string, err error) {
    uid := middleware.UserIDFromContext(ctx)
    if middleware.RoleFromContext(ctx) == api.Admin {
        return true, uid, nil
    }
    return false, uid, nil
}
```

Полный список методов:
| Эндпоинт | Метод AuthzService |
|---|---|
| `POST /brands` | `CanCreateBrand(ctx) error` |
| `GET /brands` | `CanListBrands(ctx) (canViewAll bool, userID string, err error)` |
| `GET /brands/{id}` | `CanViewBrand(ctx, brandID) error` |
| `PUT /brands/{id}` | `CanUpdateBrand(ctx, brandID) error` |
| `DELETE /brands/{id}` | `CanDeleteBrand(ctx, brandID) error` |
| `POST /brands/{id}/managers` | `CanAssignManager(ctx, brandID) error` |
| `DELETE /brands/{id}/managers/{uid}` | `CanRemoveManager(ctx, brandID, userID) error` |
| `GET /audit-logs` | `CanListAuditLogs(ctx) error` |

**handler/brand.go** — хендлер вызывает authzService, не знает о ролях:
```go
func (s *Server) CreateBrand(w http.ResponseWriter, r *http.Request) {
    if err := s.authzService.CanCreateBrand(r.Context()); err != nil { ... }
    brand, err := s.brandService.CreateBrand(r.Context(), name, logoURL)
    ...
}

func (s *Server) ListBrands(w http.ResponseWriter, r *http.Request) {
    canViewAll, userID, err := s.authzService.CanListBrands(r.Context())
    if err != nil { ... }

    var managerID *string
    if !canViewAll {
        managerID = &userID
    }

    brands, err := s.brandService.ListBrands(r.Context(), managerID)
    ...
}
```

**service/brand.go** — без role-checks, принимает managerID для фильтрации:
```go
func (s *BrandService) ListBrands(ctx context.Context, managerID *string) ([]*domain.BrandListItem, error) {
    // managerID == nil → все бренды
    // managerID != nil → бренды этого менеджера
}
```

**Организация файлов authz** — один struct, методы по доменам:
```
authz/
  authz.go          # AuthzService struct, конструктор, интерфейсы зависимостей
  brand.go          # CanCreateBrand, CanViewBrand, CanListBrands, ...
  audit.go          # CanListAuditLogs
  brand_test.go
  audit_test.go
```

---

## 2. Аудит-логи: текущее состояние и нарушения

### Что есть сейчас

**handler/auditor.go** — хелпер `logAudit()` в пакете handler:
- Fire-and-forget: ошибка логируется, не пробрасывается
- Вызывается ПОСЛЕ service call, вне транзакции

**handler/brand.go** — аудит вызывается в каждом write-хендлере:
- `CreateBrand` (строки 34-38), `UpdateBrand` (строки 144-148), `DeleteBrand` (строки 167-171)
- `AssignManager` (строки 199-203), `RemoveManager` (строки 237-241)

**handler/auth.go** — аудит в auth-хендлерах:
- `Login` (строки 41-45), `Logout` (строки 97-101), `ResetPassword` (строки 170-173)

**handler/server.go** — Server хранит `auditService AuditLogService`

**service/audit.go** — `AuditService.Log()` пишет запись, но вне транзакции бизнес-операции

### Нарушения стандартов

**backend-transactions.md** (строки 111-126):
> "Аудит-лог обязательно пишется внутри той же транзакции, что и изменение данных. Fire-and-forget запись аудита запрещена."

**backend-architecture.md** (строки 19-21):
> "Handler — точка входа запроса. [...] До сервиса доходят только провалидированные данные"
> "Service — бизнес-логика"

Хендлер собирает `service.AuditEntry` с полями ActorID, ActorRole, Action, EntityType — это знание о бизнес-логике, не о HTTP.

### Целевое состояние

Сервис получает actor info из context (userID, role — уже там через auth middleware). Для IP-адреса: добавить middleware, который ставит `clientIP` в context.

**service/brand.go** — аудит внутри транзакции:
```go
func (s *BrandService) CreateBrand(ctx context.Context, name string, logoURL *string) (*domain.Brand, error) {
    var brand *domain.Brand
    err := dbutil.WithTx(ctx, s.pool, func(tx dbutil.DB) error {
        brandRepo := s.repoFactory.NewBrandRepo(tx)
        auditRepo := s.repoFactory.NewAuditRepo(tx)

        row, err := brandRepo.Create(ctx, name, logoURL)
        if err != nil { return err }
        brand = brandRowToDomain(row)

        return auditRepo.Create(ctx, repository.AuditLogRow{
            ActorID: middleware.UserIDFromContext(ctx),
            ActorRole: string(middleware.RoleFromContext(ctx)),
            Action: "brand_create", EntityType: "brand", EntityID: &brand.ID,
            IPAddress: middleware.ClientIPFromContext(ctx),
            ...
        })
    })
    return brand, err
}
```

**handler/brand.go** — хендлер не знает об аудите:
```go
func (s *Server) CreateBrand(w http.ResponseWriter, r *http.Request) {
    if err := s.authzService.CanCreateBrand(r.Context()); err != nil { ... }
    // parse request
    brand, err := s.brandService.CreateBrand(r.Context(), name, logoURL)
    // respond
}
```

### Операции без текущих транзакций

`CreateBrand`, `UpdateBrand`, `DeleteBrand`, `RemoveManager` — сейчас single-query, но с аудитом станут multi-write → нужна транзакция `dbutil.WithTx`.

`Login`, `Logout` — аудит после операции. Для login аудит можно писать без транзакции (login сам не пишет данные, кроме refresh token). Но по стандарту fire-and-forget запрещён.

`ResetPassword` — уже использует `WithTx`, нужно добавить аудит внутрь.

`AssignManager` — уже использует `WithTx`, нужно добавить аудит внутрь.

### Что меняется в RepoFactory интерфейсах сервисов

BrandRepoFactory добавляет `NewAuditRepo(db dbutil.DB) repository.AuditRepo`.
AuthRepoFactory добавляет `NewAuditRepo(db dbutil.DB) repository.AuditRepo`.

---

## 3. Перенос api/ в backend/api/

### Текущее состояние

```
api/
  openapi.yaml       # основной контракт
  openapi-test.yaml  # контракт test-endpoints
```

**Makefile (generate-api)**:
- Backend: `oapi-codegen ... api/openapi.yaml` (строка 133)
- Frontend web: `cd frontend/web && npx openapi-typescript ../../api/openapi.yaml` (строка 135)
- Frontend tma: `cd frontend/tma && npx openapi-typescript ../../api/openapi.yaml` (строка 136)
- E2E types: `oapi-codegen ... api/openapi.yaml` (строки 138-144)

### Что меняется

1. `git mv api/ backend/api/` — перемещение с сохранением истории
2. Makefile: обновить все пути `api/openapi*.yaml` → `backend/api/openapi*.yaml`
3. Frontend codegen: `../../api/openapi.yaml` → `../../backend/api/openapi.yaml`
4. Код не меняется: go-пакет `internal/api` останется на месте, генерация в `backend/internal/api/server.gen.go` не зависит от пути YAML

---

## 4. Другие нарушения стандартов (обнаруженные попутно)

### domain/roles.go — ручной дубликат сгенерированных констант

`domain.UserRole`, `domain.RoleAdmin`, `domain.RoleBrandManager` дублируют `api.UserRole`, `api.Admin`, `api.BrandManager` из `server.gen.go`.

**Действие**: удалить `domain/roles.go`, заменить все использования на `api.UserRole` / `api.Admin` / `api.BrandManager`.

### handler/response.go — прямой import pgx

Строка 10: `"github.com/jackc/pgx/v5"`, строка 38: `errors.Is(err, pgx.ErrNoRows)`

**Нарушение**: handler зависит от DB-слоя. По стандарту handler → service → repository → DB. Нужно заменить на `sql.ErrNoRows` (используется в repository) или добавить domain sentinel error.

### handler/brand.go — хендлер импортирует authz напрямую

Строка 9: `"github.com/alikhanmurzayev/ugcboost/backend/internal/authz"`

После рефакторинга хендлер будет зависеть от интерфейса AuthzService, не от пакета authz.

### middleware/auth.go — RoleFromContext возвращает string

Должен возвращать `api.UserRole` для типобезопасности. Middleware при записи в context тоже должен писать `api.UserRole(role)`, а не `string`.

---

## Затронутые файлы

### Файлы для изменения

| Файл | Изменение |
|------|-----------|
| `backend/internal/authz/authz.go` | Переписать: AuthzService struct, интерфейсы зависимостей |
| `backend/internal/authz/brand.go` | **Новый**: Can*Brand методы |
| `backend/internal/authz/audit.go` | **Новый**: CanListAuditLogs |
| `backend/internal/handler/server.go` | Добавить authzService, убрать auditService |
| `backend/internal/handler/brand.go` | Заменить authz вызовы на authzService, убрать audit |
| `backend/internal/handler/auth.go` | Убрать logAudit вызовы |
| `backend/internal/handler/audit.go` | Заменить authz на authzService |
| `backend/internal/handler/auditor.go` | **Удалить** (аудит — ответственность сервисов) |
| `backend/internal/handler/response.go` | Убрать import pgx, использовать domain error |
| `backend/internal/service/brand.go` | Добавить аудит в транзакции, убрать role-checks, добавить IsUserBrandManager |
| `backend/internal/service/auth.go` | Добавить аудит в login/logout/reset |
| `backend/internal/service/audit.go` | Обновить при необходимости |
| `backend/internal/middleware/auth.go` | RoleFromContext → api.UserRole, добавить clientIP в context |
| `backend/internal/domain/roles.go` | **Удалить** (дубликат api.UserRole) |
| `backend/cmd/api/main.go` | Создать AuthzService, обновить wiring |
| `Makefile` | Обновить пути к api/*.yaml |

### Файлы для перемещения

| Файл | Куда |
|------|------|
| `api/openapi.yaml` | `backend/api/openapi.yaml` |
| `api/openapi-test.yaml` | `backend/api/openapi-test.yaml` |

### Тесты для обновления

| Файл | Изменение |
|------|-----------|
| `backend/internal/handler/brand_test.go` | Mock AuthzService, убрать audit-зависимость |
| `backend/internal/handler/auth_test.go` | Убрать audit-зависимость |
| `backend/internal/service/brand_test.go` | Добавить AuditRepo expectations, убрать role params |
| `backend/internal/service/auth_test.go` | Добавить AuditRepo expectations |
| `backend/internal/authz/brand_test.go` | **Новый**: тесты brand-методов AuthzService |
| `backend/internal/authz/audit_test.go` | **Новый**: тесты audit-методов AuthzService |
| `backend/internal/middleware/auth_test.go` | Обновить: RoleFromContext возвращает api.UserRole |

### Моки для обновления

| Действие | Файл |
|----------|------|
| Добавить | `handler/mocks/mock_authz_service.go` (сгенерировать) |
| Удалить | `handler/mocks/mock_audit_log_service.go` (аудит уходит из handler) |
| Обновить | `service/mocks/mock_brand_repo_factory.go` (добавить NewAuditRepo) |
| Обновить | `service/mocks/mock_auth_repo_factory.go` (добавить NewAuditRepo) |
| Добавить | `authz/mocks/mock_brand_service.go` (сгенерировать) |
| Удалить | `authz/mocks/mock_brand_access_checker.go` (заменён на BrandService) |

---

## Риски и соображения

### Breaking changes (нет внешних)
- API-контракт не меняется — все изменения внутренние
- Поведение эндпоинтов не меняется — тот же authz, тот же аудит
- E2E тесты не затронуты (проверяют HTTP-контракт)

### Риск: аудит в транзакциях увеличит длительность транзакций
- Минимальный: аудит — один INSERT, транзакция не станет заметно длиннее
- Зато гарантия консистентности: нет записи без аудита и наоборот

### Риск: IP в context через middleware
- Middleware уже ставит userID и role в context — добавление IP — тот же паттерн
- `clientIP()` из handler/auditor.go переезжает в middleware

### Риск: перенос api/ сломает CI
- Если CI или Dockerfile ссылаются на `api/` — нужно проверить
- Docker: `api/` не копируется в Docker build (бэкенд Dockerfile копирует только `backend/`)
- CI: проверить наличие ссылок на `api/` в workflow файлах

### Риск: удаление domain/roles.go
- Все файлы, импортирующие `domain.RoleAdmin` / `domain.RoleBrandManager`, нужно обновить
- Middleware начнёт зависеть от `api` пакета — допустимо, middleware уже импортирует `api`

---

## Рекомендуемый подход

### Порядок изменений (5 этапов)

**Этап 1: Перенос api/ → backend/api/**
- Минимальный risk, чистый diff
- `git mv`, обновление Makefile, проверка `make generate-api`
- Отдельный коммит

**Этап 2: Удаление domain/roles.go, типизация RoleFromContext**
- Удалить `domain/roles.go`
- `middleware.RoleFromContext()` → возвращает `api.UserRole`
- Заменить все `domain.RoleAdmin` → `api.Admin`, `domain.RoleBrandManager` → `api.BrandManager`
- Отдельный коммит

**Этап 3: AuthzService**
- Создать AuthzService struct с DI (зависимость: `BrandService` interface)
- Методы по конкретным действиям: CanCreateBrand, CanViewBrand, CanListBrands и т.д.
- Файлы по доменам: `authz.go`, `brand.go`, `audit.go`
- Добавить `IsUserBrandManager` в BrandService
- Убрать `CanViewBrand` из BrandService, role-checks из `ListBrands`
- Обновить handler: authzService интерфейс, ListBrands принимает `managerID *string`
- Обновить тесты и моки

**Этап 4: Аудит в сервисах**
- Добавить clientIP middleware
- Добавить `NewAuditRepo` в RepoFactory интерфейсы сервисов
- Перенести аудит в каждый service method (внутри WithTx)
- Убрать logAudit из хендлеров, удалить auditor.go
- Убрать auditService из handler.Server
- Обновить тесты

**Этап 5: Мелкие фиксы**
- handler/response.go: убрать pgx import
- Регенерировать моки через `make generate-mocks`
- Прогнать `make test-unit-backend && make lint-backend`

### Ключевые решения (согласованы)

1. **Именование методов AuthzService** — по конкретным API-действиям: CanCreateBrand, CanViewBrand и т.д. Каждый метод сам достаёт user из context.

2. **DI в AuthzService** — зависимости именуются как сервисы (BrandService, не BrandAccessChecker). Интерфейсы определяются в пакете authz.

3. **ListBrands scope** — AuthzService возвращает `(canViewAll bool, userID string, err error)`. Хендлер на основе `canViewAll` передаёт в BrandService `managerID *string` (nil = все бренды).

4. **Роли** — удалить domain/roles.go, использовать сгенерированные `api.UserRole`, `api.Admin`, `api.BrandManager`. Middleware возвращает типизированный `api.UserRole`.

5. **Actor info для аудита** — из context (userID, role, IP). IP добавляется middleware.

6. **Операции без текущей транзакции** — CreateBrand, UpdateBrand, DeleteBrand, RemoveManager станут транзакционными из-за добавления аудита (2+ write ops → WithTx).
