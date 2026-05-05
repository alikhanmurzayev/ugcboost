# План реализации: рефакторинг репозиториев и сервисов под стандарты

## Обзор

Привести слои repository и service в соответствие со стандартами `backend-repository`, `backend-transactions`, `backend-design`. Приватные repo-структуры, экспортируемые интерфейсы, stateless RepoFactory, транзакции для multi-write операций, auto-discovery mockery.

## Требования

- REQ-1: Repo-структуры приватные, с экспортируемыми интерфейсами рядом
- REQ-2: RepoFactory stateless, каждый метод принимает `dbutil.DB`
- REQ-3: Нет standalone конструкторов — создание только через RepoFactory
- REQ-4: Сервисы хранят `pool` + `repoFactory`, не прямые repo
- REQ-5: Multi-write методы обёрнуты в `dbutil.WithTx`
- REQ-6: Mockery на auto-discovery (`all: true` + `internal/...`)
- REQ-7: Все существующие тесты проходят после рефакторинга
- REQ-8: Бизнес-логика и API не меняются

## Файлы для изменения

| Файл | Изменения |
|------|-----------|
| `backend/.mockery.yaml` | `all: true` + `internal/...` + `exclude-subpkg-regex` |
| `backend/internal/dbutil/db.go` | Добавить интерфейс `Pool` (DB + TxStarter) |
| `backend/internal/repository/user.go` | `UserRepository` → `userRepository`, добавить `UserRepo` interface |
| `backend/internal/repository/brand.go` | `BrandRepository` → `brandRepository`, добавить `BrandRepo` interface |
| `backend/internal/repository/audit.go` | `AuditRepository` → `auditRepository`, добавить `AuditRepo` interface |
| `backend/internal/repository/factory.go` | Stateless RepoFactory с методами-конструкторами |
| `backend/internal/service/auth.go` | `pool Pool` + `AuthRepoFactory` вместо `users UserRepo` |
| `backend/internal/service/brand.go` | `pool Pool` + `BrandRepoFactory` вместо `brands`/`users` |
| `backend/internal/service/audit.go` | `pool Pool` + `AuditRepoFactory` вместо `repo` |
| `backend/cmd/api/main.go` | Wiring: RepoFactory + pool в сервисы |
| `backend/internal/repository/user_test.go` | Конструктор `&userRepository{db: db}` |
| `backend/internal/repository/brand_test.go` | То же |
| `backend/internal/repository/audit_test.go` | То же |
| `backend/internal/service/auth_test.go` | Моки Pool + AuthRepoFactory + repository.UserRepo |
| `backend/internal/service/brand_test.go` | Моки Pool + BrandRepoFactory + repository repos |

## Файлы для создания

| Файл | Назначение |
|------|------------|
| `backend/internal/service/helpers_test.go` | `testTx` — stub для `pgx.Tx` в unit-тестах сервисов (Commit/Rollback no-op, остальные panic) |

## Архитектурное решение: `dbutil.Pool`

Стандарт показывает `pool dbutil.TxStarter` в сервисе и `s.repoFactory.NewEntityRepo(s.pool)` для read-only методов. Но `TxStarter` (только `Begin`) не удовлетворяет `dbutil.DB` (Query/Exec/QueryRow), поэтому `s.pool` нельзя передать в фабрику как `DB`.

Решение — комбинированный интерфейс:

```go
// dbutil/db.go
type Pool interface {
    DB
    TxStarter
}
```

`pgxpool.Pool` реализует оба. Сервисы хранят `pool dbutil.Pool`:
- Для read-only: `s.repoFactory.NewUserRepo(s.pool)` — pool как DB
- Для транзакций: `dbutil.WithTx(ctx, s.pool, ...)` — pool как TxStarter

## Шаги реализации

### 1. [ ] Mockery config → auto-discovery

Перезаписать `backend/.mockery.yaml`:

```yaml
dir: "{{.InterfaceDir}}/mocks"
structname: "Mock{{.InterfaceName}}"
pkgname: mocks
filename: "mock_{{.InterfaceName | snakecase}}.go"
template: testify
all: true
exclude-subpkg-regex:
  - "mocks"
packages:
  github.com/alikhanmurzayev/ugcboost/backend/internal/...:
```

Проверка: удалить все `mocks/` директории, запустить `mockery`, сравнить — должны появиться моки для всех 14 текущих интерфейсов + новые (TxStarter, TokenStore, ResetTokenNotifier, BrandAccessChecker).

### 2. [ ] dbutil: добавить `Pool` interface

В `backend/internal/dbutil/db.go` добавить:

```go
type Pool interface {
    DB
    TxStarter
}
```

Без изменений в `WithTx` — он по-прежнему принимает `TxStarter`.

### 3. [ ] Repository: приватные структуры + экспортируемые интерфейсы

**user.go:**
- `UserRepository` → `userRepository` (приватная)
- Убрать `NewUserRepository()` (standalone конструктор)
- Добавить `UserRepo` интерфейс (все публичные методы)

**brand.go:**
- `BrandRepository` → `brandRepository` (приватная)
- Убрать `NewBrandRepository()`
- Добавить `BrandRepo` интерфейс

**audit.go:**
- `AuditRepository` → `auditRepository` (приватная)
- Убрать `NewAuditRepository()`
- Добавить `AuditRepo` интерфейс

### 4. [ ] Repository: stateless RepoFactory

Перезаписать `backend/internal/repository/factory.go`:

```go
type RepoFactory struct{}

func NewRepoFactory() *RepoFactory { return &RepoFactory{} }

func (f *RepoFactory) NewUserRepo(db dbutil.DB) UserRepo {
    return &userRepository{db: db}
}

func (f *RepoFactory) NewBrandRepo(db dbutil.DB) BrandRepo {
    return &brandRepository{db: db}
}

func (f *RepoFactory) NewAuditRepo(db dbutil.DB) AuditRepo {
    return &auditRepository{db: db}
}
```

### 5. [ ] Repository: обновить тесты

Тесты в `package repository` — имеют доступ к приватным типам:

```go
// Было:  repo := NewUserRepository(db)
// Стало: repo := &userRepository{db: db}
```

### 6. [ ] Service: AuthService → pool + RepoFactory + транзакции

```go
type AuthRepoFactory interface {
    NewUserRepo(db dbutil.DB) repository.UserRepo
}

type AuthService struct {
    pool           dbutil.Pool
    repoFactory    AuthRepoFactory
    tokens         TokenGenerator
    resetNotifier  ResetTokenNotifier
    bcryptCost     int
}

func NewAuthService(pool dbutil.Pool, repoFactory AuthRepoFactory, tokens TokenGenerator, resetNotifier ResetTokenNotifier, bcryptCost int) *AuthService { ... }
```

**Методы без транзакций** (read-only или single write):
- `Login`, `Refresh`, `Logout`, `RequestPasswordReset`, `GetUser`, `SeedUser`, `SeedAdmin`
- Паттерн: `userRepo := s.repoFactory.NewUserRepo(s.pool)`

**Методы с транзакцией:**
- `ResetPassword` (ClaimResetToken + UpdatePassword + DeleteUserRefreshTokens):

```go
func (s *AuthService) ResetPassword(ctx context.Context, rawToken, newPassword string) (string, error) {
    hash := HashToken(rawToken)
    passwordHash, err := bcrypt.GenerateFromPassword([]byte(newPassword), s.bcryptCost)
    if err != nil {
        return "", fmt.Errorf("hash password: %w", err)
    }

    var userID string
    err = dbutil.WithTx(ctx, s.pool, func(tx dbutil.DB) error {
        userRepo := s.repoFactory.NewUserRepo(tx)

        rt, err := userRepo.ClaimResetToken(ctx, hash)
        if err != nil {
            return domain.ErrUnauthorized
        }
        userID = rt.UserID

        if err := userRepo.UpdatePassword(ctx, userID, string(passwordHash)); err != nil {
            return fmt.Errorf("update password: %w", err)
        }
        return userRepo.DeleteUserRefreshTokens(ctx, userID)
    })
    if err != nil {
        return "", err
    }
    return userID, nil
}
```

Удалить старые интерфейсы `UserRepo`, `TokenGenerator` остаётся.

### 7. [ ] Service: BrandService → pool + RepoFactory + транзакции

```go
type BrandRepoFactory interface {
    NewBrandRepo(db dbutil.DB) repository.BrandRepo
    NewUserRepo(db dbutil.DB) repository.UserRepo
}

type BrandService struct {
    pool        dbutil.Pool
    repoFactory BrandRepoFactory
    bcryptCost  int
}

func NewBrandService(pool dbutil.Pool, repoFactory BrandRepoFactory, bcryptCost int) *BrandService { ... }
```

**Методы без транзакций:**
- `CreateBrand`, `GetBrand`, `ListBrands`, `UpdateBrand`, `DeleteBrand`, `ListManagers`, `RemoveManager`, `CanViewBrand`
- Паттерн: `brandRepo := s.repoFactory.NewBrandRepo(s.pool)`

**Метод с транзакцией:**
- `AssignManager` (Create user + AssignManager):

```go
func (s *BrandService) AssignManager(ctx context.Context, brandID, email string) (*domain.User, string, error) {
    // ... валидация email ...

    var user *domain.User
    var tempPassword string

    err := dbutil.WithTx(ctx, s.pool, func(tx dbutil.DB) error {
        brandRepo := s.repoFactory.NewBrandRepo(tx)
        userRepo := s.repoFactory.NewUserRepo(tx)

        // Check brand exists
        if _, err := brandRepo.GetByID(ctx, brandID); err != nil {
            return fmt.Errorf("get brand: %w", err)
        }

        // ... get or create user, assign manager ...
        return nil
    })
    // ...
}
```

Удалить старые интерфейсы `BrandRepo`, `BrandUserRepo`.

### 8. [ ] Service: AuditService → pool + RepoFactory

```go
type AuditRepoFactory interface {
    NewAuditRepo(db dbutil.DB) repository.AuditRepo
}

type AuditService struct {
    pool        dbutil.Pool
    repoFactory AuditRepoFactory
}

func NewAuditService(pool dbutil.Pool, repoFactory AuditRepoFactory) *AuditService { ... }
```

Без транзакций — оба метода (`Log`, `List`) single-write/read-only.

### 9. [ ] Service: тест-хелпер testTx

Создать `backend/internal/service/helpers_test.go`:

```go
// testTx satisfies pgx.Tx for unit-testing WithTx.
// Commit/Rollback are no-ops; other methods panic (should never be called
// because mock repos intercept all DB operations).
type testTx struct{}

func (testTx) Commit(context.Context) error   { return nil }
func (testTx) Rollback(context.Context) error  { return nil }
func (testTx) Begin(context.Context) (pgx.Tx, error) { panic("testTx: unexpected Begin") }
func (testTx) CopyFrom(context.Context, pgx.Identifier, []string, pgx.CopyFromSource) (int64, error) { panic("testTx: unexpected CopyFrom") }
func (testTx) SendBatch(context.Context, *pgx.Batch) pgx.BatchResults { panic("testTx: unexpected SendBatch") }
func (testTx) LargeObjects() pgx.LargeObjects { panic("testTx: unexpected LargeObjects") }
func (testTx) Prepare(context.Context, string, string) (*pgconn.StatementDescription, error) { panic("testTx: unexpected Prepare") }
func (testTx) Exec(context.Context, string, ...any) (pgconn.CommandTag, error) { panic("testTx: unexpected Exec") }
func (testTx) Query(context.Context, string, ...any) (pgx.Rows, error) { panic("testTx: unexpected Query") }
func (testTx) QueryRow(context.Context, string, ...any) pgx.Row { panic("testTx: unexpected QueryRow") }
func (testTx) Conn() *pgx.Conn { panic("testTx: unexpected Conn") }
```

### 10. [ ] Service: обновить тесты

**Паттерн для не-транзакционных методов:**
```go
pool := dbmocks.NewMockPool(t)
factory := svcmocks.NewMockAuthRepoFactory(t)
userRepo := repomocks.NewMockUserRepo(t)

factory.EXPECT().NewUserRepo(mock.Anything).Return(userRepo)
userRepo.EXPECT().GetByID(mock.Anything, "user-1").Return(&repository.UserRow{...}, nil)

svc := NewAuthService(pool, factory, tokens, nil, testBcryptCost)
```

**Паттерн для транзакционных методов:**
```go
pool := dbmocks.NewMockPool(t)
factory := svcmocks.NewMockAuthRepoFactory(t)
userRepo := repomocks.NewMockUserRepo(t)

pool.EXPECT().Begin(mock.Anything).Return(testTx{}, nil)
factory.EXPECT().NewUserRepo(mock.Anything).Return(userRepo)
// ... set up userRepo expectations ...

svc := NewAuthService(pool, factory, tokens, nil, testBcryptCost)
```

`testTx{}` — хелпер из шага 9. `WithTx` вызывает `Begin()` → получает `testTx` → вызывает callback → вызывает `testTx.Commit()` (no-op). Мок repo перехватывает все DB-операции.

Импорты с алиасами (три пакета `mocks`):
```go
import (
    dbmocks  "github.com/.../dbutil/mocks"
    repomocks "github.com/.../repository/mocks"
    svcmocks  "github.com/.../service/mocks"
)
```

### 11. [ ] Wiring: обновить main.go

```go
// Было:
userRepo := repository.NewUserRepository(pool)
brandRepo := repository.NewBrandRepository(pool)
auditRepo := repository.NewAuditRepository(pool)
authSvc := service.NewAuthService(userRepo, tokenSvc, resetTokenStore, cfg.BcryptCost)
brandSvc := service.NewBrandService(brandRepo, userRepo, cfg.BcryptCost)
auditSvc := service.NewAuditService(auditRepo)

// Стало:
repoFactory := repository.NewRepoFactory()
authSvc := service.NewAuthService(pool, repoFactory, tokenSvc, resetTokenStore, cfg.BcryptCost)
brandSvc := service.NewBrandService(pool, repoFactory, cfg.BcryptCost)
auditSvc := service.NewAuditService(pool, repoFactory)
```

### 12. [ ] Перегенерация моков

```bash
# Удалить старые моки
find backend/internal -type d -name mocks -exec rm -rf {} +

# Перегенерировать
cd backend && mockery
```

Новые моки будут включать:
- `dbutil/mocks/MockPool` (новый)
- `dbutil/mocks/MockTxStarter` (новый)
- `repository/mocks/MockUserRepo` (новый)
- `repository/mocks/MockBrandRepo` (новый)
- `repository/mocks/MockAuditRepo` (новый)
- `service/mocks/MockAuthRepoFactory` (новый)
- `service/mocks/MockBrandRepoFactory` (новый)
- `service/mocks/MockAuditRepoFactory` (новый)
- Все существующие моки (handler, middleware, dbutil.DB, service.TokenGenerator и др.)

### 13. [ ] Полная проверка

```bash
# Backend
make build-backend
make lint-backend
make test-unit-backend

# Frontend
make test-unit-web
make test-unit-tma
make test-unit-landing
make lint-web
make lint-tma
make lint-landing

# E2E (Docker)
make test-e2e-backend
make test-e2e-frontend
```

## Стратегия тестирования

- **Unit-тесты repo**: обновить конструкторы, SQL-ассерты не меняются
- **Unit-тесты service**: новый паттерн с Pool + RepoFactory + testTx. Бизнес-ассерты не меняются
- **Unit-тесты handler**: без изменений (интерфейсы сервисов стабильны)
- **E2E-тесты**: без изменений (проходят через HTTP)
- **Новое покрытие**: транзакции в ResetPassword и AssignManager (тестируются через WithTx → testTx)

## Оценка рисков

| Риск | Вероятность | Митигация |
|------|-------------|-----------|
| Mockery auto-discovery подхватывает лишние интерфейсы | Низкая | `exclude-subpkg-regex: ["mocks"]`, `include-auto-generated: false` (default). Лишние моки не мешают |
| pgx.Tx interface изменится при обновлении pgx | Низкая | testTx — единственная точка зависимости. При обновлении pgx — обновить только testTx |
| Тесты с WithTx не ловят реальные tx-ошибки | Средняя | E2E-тесты покрывают реальные транзакции с настоящей БД. Unit-тесты проверяют бизнес-логику |
| BrandService.AssignManager: race между ExistsByEmail и Create | Низкая | Транзакция + unique constraint на email в БД = двойная защита |

## План отката

Git revert — рефакторинг чисто структурный, бизнес-логика не меняется. Старый код на текущей ветке, новый в отдельном коммите.
