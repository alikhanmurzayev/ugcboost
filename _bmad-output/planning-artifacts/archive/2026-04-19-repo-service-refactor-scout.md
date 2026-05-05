# Разведка: рефакторинг репозиториев и сервисов под стандарты

## Затронутые области

### Файлы, которые нужно изменить

**Репозитории (ядро рефакторинга):**
- `backend/internal/repository/factory.go` — полная переработка RepoFactory
- `backend/internal/repository/user.go` — приватная структура + экспортируемый интерфейс
- `backend/internal/repository/brand.go` — то же самое
- `backend/internal/repository/audit.go` — то же самое

**Сервисы (паттерн зависимостей):**
- `backend/internal/service/auth.go` — TxStarter + RepoFactory вместо прямых repo
- `backend/internal/service/brand.go` — то же + транзакции для AssignManager
- `backend/internal/service/audit.go` — TxStarter + RepoFactory

**Wiring:**
- `backend/cmd/api/main.go` — новый паттерн инъекции зависимостей

**Тесты:**
- `backend/internal/repository/user_test.go` — конструктор через фабрику
- `backend/internal/repository/brand_test.go` — то же
- `backend/internal/repository/audit_test.go` — то же
- `backend/internal/service/auth_test.go` — моки RepoFactory + TxStarter
- `backend/internal/service/brand_test.go` — то же

**Mockery:**
- `backend/.mockery.yaml` — переход на auto-discovery (`all: true`)

**Handler (без изменений):** хендлеры уже соответствуют стандарту — определяют интерфейсы сервисов в своём пакете.

### Зависимости между файлами

```
repository/factory.go → repository/{user,brand,audit}.go (фабричные методы)
service/{auth,brand,audit}.go → repository (интерфейсы repo)
service/{auth,brand,audit}.go → dbutil (TxStarter, WithTx, DB)
cmd/api/main.go → repository + service (wiring)
```

## Текущие нарушения стандартов

### 1. Экспортируемые структуры repo (нарушает backend-repository)

**Сейчас:**
```go
type UserRepository struct { db dbutil.DB }  // экспортируемая
func NewUserRepository(db dbutil.DB) *UserRepository { ... }  // конструктор на уровне пакета
```

**Стандарт:**
```go
type UserRepo interface { ... }  // экспортируемый интерфейс
type userRepository struct { db dbutil.DB }  // приватная структура
// конструктор только через RepoFactory
```

Затронуто: `user.go`, `brand.go`, `audit.go`

### 2. RepoFactory хранит состояние (нарушает backend-design, backend-transactions)

**Сейчас:**
```go
type RepoFactory struct { db dbutil.DB }
func NewRepoFactory(db dbutil.DB) *RepoFactory { ... }
func (f *RepoFactory) DB() dbutil.DB { ... }
```

**Стандарт:**
```go
type RepoFactory struct{}  // stateless
func NewRepoFactory() *RepoFactory { return &RepoFactory{} }
func (f *RepoFactory) NewUserRepo(db dbutil.DB) UserRepo { ... }
```

### 3. Сервисы принимают repo напрямую (нарушает backend-transactions)

**Сейчас (auth):**
```go
type AuthService struct {
    users  UserRepo        // прямой repo
    tokens TokenGenerator
}
```

**Стандарт:**
```go
type RepoFactory interface {
    NewUserRepo(db dbutil.DB) repository.UserRepo
}
type AuthService struct {
    pool        dbutil.TxStarter
    repoFactory RepoFactory
    tokens      TokenGenerator
}
```

### 4. Нет транзакций где нужны (нарушает backend-transactions)

Методы с 2+ операциями записи, которые должны быть атомарными:

- **`AuthService.ResetPassword`**: ClaimResetToken → UpdatePassword → DeleteUserRefreshTokens (3 записи без tx)
- **`BrandService.AssignManager`**: Create user → AssignManager (2 записи без tx, + аудит потом добавится)
- **`AuthService.Login`**: аутентификация + SaveRefreshToken (можно без tx — потеря refresh token не критична)

### 5. Нет экспортируемых интерфейсов repo (нарушает backend-repository)

Интерфейсы сейчас определены в пакетах-потребителях (service), но стандарт требует **оба**:
- Экспортируемый интерфейс в `repository/` рядом с приватной структурой
- Свой узкий интерфейс в каждом сервисе (Go convention: accept interfaces)

### 6. Mockery конфиг перечисляет интерфейсы вручную

**Сейчас:**
```yaml
packages:
  github.com/.../internal/dbutil:
    interfaces:
      DB: {}
  github.com/.../internal/handler:
    interfaces:
      AuthService: {}
      BrandService: {}
      AuditLogService: {}
  # ...каждый интерфейс руками
```

**Целевое:**
```yaml
all: true
packages:
  github.com/.../internal/...:
```

Auto-discovery: mockery v3 поддерживает `all: true` + рекурсивный `...` паттерн. Файлы с `// Code generated` (например `server.gen.go`) пропускаются автоматически (`include-auto-generated: false` — default). Для исключения конкретных интерфейсов — `exclude-interface-regex`.

## Паттерны реализации

### Целевой паттерн repo (из стандарта)

```go
// repository/user.go
type UserRepo interface {
    GetByEmail(ctx context.Context, email string) (*UserRow, error)
    // ...все публичные методы
}

type userRepository struct { db dbutil.DB }
```

### Целевой паттерн factory (из стандарта)

```go
// repository/factory.go
type RepoFactory struct{}
func NewRepoFactory() *RepoFactory { return &RepoFactory{} }
func (f *RepoFactory) NewUserRepo(db dbutil.DB) UserRepo { return &userRepository{db: db} }
func (f *RepoFactory) NewBrandRepo(db dbutil.DB) BrandRepo { return &brandRepository{db: db} }
func (f *RepoFactory) NewAuditRepo(db dbutil.DB) AuditRepo { return &auditRepository{db: db} }
```

### Целевой паттерн service (из стандарта)

```go
// service/auth.go
type RepoFactory interface {
    NewUserRepo(db dbutil.DB) repository.UserRepo
}

type AuthService struct {
    pool        dbutil.TxStarter
    repoFactory RepoFactory
    tokens      TokenGenerator
    // ...
}

func (s *AuthService) ResetPassword(ctx context.Context, rawToken, newPassword string) (string, error) {
    return dbutil.WithTx(ctx, s.pool, func(tx dbutil.DB) error {
        userRepo := s.repoFactory.NewUserRepo(tx)
        // ...все операции через repos на одной tx
    })
}

func (s *AuthService) GetUser(ctx context.Context, userID string) (*domain.User, error) {
    userRepo := s.repoFactory.NewUserRepo(s.pool)  // без транзакции для read-only
    // ...
}
```

### Целевой mockery конфиг

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

- `all: true` — моки для всех интерфейсов автоматически
- `include-auto-generated: false` (default) — пропускает `server.gen.go` и другие `// Code generated` файлы
- `exclude-subpkg-regex: ["mocks"]` — не сканирует `mocks/` поддиректории
- Рекурсивный `...` — покрывает все пакеты внутри `internal/`
- Если понадобится исключить конкретный интерфейс — `exclude-interface-regex`

Текущие интерфейсы, которые будут автоматически обнаружены (14 штук):

| Пакет | Интерфейс | Нужен мок? |
|-------|-----------|-----------|
| `dbutil` | `DB` | да |
| `dbutil` | `TxStarter` | да (новый) |
| `handler` | `AuthService` | да |
| `handler` | `BrandService` | да |
| `handler` | `AuditLogService` | да |
| `handler` | `TokenStore` | да |
| `middleware` | `TokenValidator` | да |
| `service` | `UserRepo` | да (будет `RepoFactory`) |
| `service` | `TokenGenerator` | да |
| `service` | `BrandRepo` | да (будет `RepoFactory`) |
| `service` | `BrandUserRepo` | да (будет `RepoFactory`) |
| `service` | `ResetTokenNotifier` | да |
| `authz` | `BrandAccessChecker` | да |
| `repository` | `UserRepo`, `BrandRepo`, `AuditRepo` | да (новые) |

### Паттерн тестирования

Текущие тесты используют моки на интерфейсы repo в сервисе. После рефакторинга:
- Моки для `RepoFactory` — возвращают моки repo
- Моки для `TxStarter` — для транзакционных методов нужен мок `Begin()` → возвращает мок tx
- Нетранзакционные методы — `pool` передаётся как `dbutil.DB` напрямую в фабрику

## Риски и соображения

### Breaking changes
- **Внешних потребителей нет** — это внутренний код. Breaking changes ограничены рамками репозитория
- Хендлеры НЕ затрагиваются — их интерфейсы сервисов не меняются
- E2E-тесты НЕ затрагиваются — они проходят через HTTP

### Сложные моменты
- **Мокирование транзакций в unit-тестах**: нужен мок `TxStarter.Begin()` который возвращает мок `DB` (он же tx). RepoFactory принимает этот мок DB и возвращает мок repo. Цепочка длиннее, но паттерн повторяемый
- **`dbutil.WithTx` нужно мокировать через `TxStarter`**: мок `Begin()` возвращает мок tx, callback получает его. Нужен хелпер или стандартный подход в тестах
- **Auto-discovery мок файлов**: при `all: true` mockery создаст моки для ВСЕХ интерфейсов. Файлы, которые раньше не мокировались (`ResetTokenNotifier`, `BrandAccessChecker`, `TokenStore`), получат моки. Это не проблема — неиспользуемые моки не мешают

### Безопасность
- Рефакторинг не затрагивает бизнес-логику — только структуру зависимостей
- Транзакции повышают надёжность (атомарность ResetPassword, AssignManager)

## Рекомендуемый подход

### Порядок изменений

**Шаг 1: Mockery config**
1. Переключить `.mockery.yaml` на `all: true` + `internal/...`
2. Удалить старые моки
3. Перегенерировать — убедиться, что генерирует те же моки + новые

**Шаг 2: Repository layer**
1. Сделать структуры repo приватными
2. Добавить экспортируемые интерфейсы
3. Переделать RepoFactory на stateless
4. Убрать standalone конструкторы
5. Обновить тесты repo

**Шаг 3: Service layer**
1. Добавить интерфейсы RepoFactory в каждый сервис
2. Изменить структуры сервисов: `pool` + `repoFactory`
3. Обернуть multi-write методы в `dbutil.WithTx`
4. Read-only методы используют `s.pool` напрямую
5. Обновить тесты сервисов

**Шаг 4: Wiring**
1. Обновить `main.go` — новый паттерн инъекции

**Шаг 5: Перегенерация моков**
1. `cd backend && mockery` — новые интерфейсы repo + RepoFactory

**Шаг 6: Полная проверка**
1. `make build-backend` — компиляция
2. `make lint-backend` — golangci-lint
3. `make test-unit-backend` — unit-тесты бэкенда
4. `make test-unit-web` — unit-тесты web
5. `make test-unit-tma` — unit-тесты TMA
6. `make test-unit-landing` — unit-тесты landing
7. `make lint-web` — линт web (tsc + eslint)
8. `make lint-tma` — линт TMA
9. `make lint-landing` — линт landing
10. `make test-e2e-backend` — E2E-тесты бэкенда (поднимает Docker)
11. `make test-e2e-frontend` — E2E-тесты фронтенда (Playwright)

### Ключевые решения

- **RepoFactory на уровне repository** — один, stateless, экспортируемый
- **RepoFactory интерфейсы в сервисах** — свои, узкие, только нужные методы
- **Транзакции** — только для методов с 2+ записями. Login (read + SaveRefreshToken) — без tx, потеря refresh token не критична
- **Аудит** — пока не внутри транзакций (аудит вызывается из хендлеров, не из сервисов). Это отдельный рефакторинг на будущее
- **Mockery** — `all: true` с auto-discovery, без ручного перечисления интерфейсов

### Объём
- ~15 файлов (исходники + тесты + моки)
- Без изменений в бизнес-логике
- Без изменений в API/хендлерах
