# Отчёт: разведка по стандартам — Backend

**Дата**: 2026-04-11
**Область**: backend/
**Стандарты**: backend-architecture, backend-design, backend-errors, backend-codegen, backend-constants, backend-repository, backend-testing-unit, backend-libraries, naming, security

---

## Критичные — ломают архитектуру, безопасность, тестируемость

### C1. Кодогенерация не используется для роутинга и типов

- **Файлы**: `backend/cmd/api/main.go:120-156`, все файлы `backend/internal/handler/*.go`
- **Стандарт**: `docs/standards/backend-codegen.md` [REQUIRED]
- **Что не так**: Сгенерированный `server.gen.go` содержит `ServerInterface` и `HandlerFromMux()`, но они не используются. Вместо этого:
  - Роуты зарегистрированы вручную (`r.Post("/auth/login", ...)`, `r.Route("/brands", ...)`)
  - Request body парсится через анонимные `struct{}` в хендлерах (auth.go:41-44, brand.go:48-51, audit.go:38-57)
  - Response body формируется через `map[string]any{...}` вместо сгенерированных типов
  - Query/path params парсятся вручную (`chi.URLParam()`, `r.URL.Query().Get()`)
- **Как исправить**: Реализовать `api.ServerInterface` в хендлерах, подключить через `api.HandlerFromMux(impl, r)`. Использовать сгенерированные типы для request/response.

### C2. Нет явного ENVIRONMENT env var

- **Файл**: `backend/internal/config/config.go`
- **Стандарт**: `docs/standards/security.md` [CRITICAL], `docs/standards/backend-design.md` [REQUIRED]
- **Что не так**: Нет поля `Environment` в Config. Поведение (CookieSecure) вычисляется косвенно из CORS origins (строка 79: `!hasLocalhostOrigin(corsOrigins)`). Стандарт требует явный `ENVIRONMENT` env var (`local`/`staging`/`production`) и всё поведение от него.
- **Как исправить**: Добавить `Environment string` в Config, парсить из `ENVIRONMENT` env var, использовать для CookieSecure, EnableTestEndpoints, debug logging.

### C3. Прямые проверки ролей в хендлерах

- **Файлы**: `backend/internal/handler/brand.go:43,141,175,202,241`, `backend/internal/handler/audit.go:32`
- **Стандарт**: `docs/standards/backend-architecture.md` [REQUIRED]
- **Что не так**: Хендлеры сравнивают роли напрямую (`role != string(domain.RoleAdmin)`). Стандарт: "Вся авторизационная логика — в отдельном сервисе (AuthzService). Прямые сравнения ролей в хендлерах запрещены."
- **Как исправить**: Вынести проверки в middleware `RequireRole()` (уже существует, но не используется) или AuthzService. Хендлер вызывает один метод авторизации.

### C4. Молчаливый fallback при невалидном конфиге

- **Файл**: `backend/internal/config/config.go:118-152`
- **Стандарт**: `docs/standards/backend-errors.md` [CRITICAL], `docs/standards/backend-libraries.md` [REQUIRED]
- **Что не так**: `getBoolEnv()`, `getIntEnv()`, `getDurationEnv()` при ошибке парсинга молча возвращают fallback. Стандарт: "Невалидный ввод = ошибка, не тихий fallback". Невалидный конфиг = приложение не стартует.
- **Как исправить**: При ошибке парсинга возвращать error из `Load()`. Заменить кастомные хелперы на библиотеку (caarlos0/env или kelseyhightower/envconfig).

---

## Важные — нарушают конвенции, усложняют поддержку

### I1. Handler зависит от repository типов напрямую

- **Файлы**: `backend/internal/handler/auth.go:14,24`, `backend/internal/handler/brand.go:13,18-27`, `backend/internal/handler/audit.go:16`
- **Стандарт**: `docs/standards/backend-architecture.md`
- **Что не так**: Handler импортирует `repository` и использует `repository.UserRow`, `repository.BrandRow` и т.д. в сигнатурах интерфейсов. Стандарт: "handler зависит от service interfaces — никогда от repository напрямую".
- **Как исправить**: Service должен возвращать domain-типы или сгенерированные API-типы. Handler не знает про repository.

### I2. Repository: константы приватные и не по стандарту

- **Файлы**: `backend/internal/repository/user.go:15-40`, `brand.go:17-31`, `audit.go:14-26`
- **Стандарт**: `docs/standards/backend-repository.md`, `docs/standards/naming.md`
- **Что не так**: Константы приватные (`tableUsers`, `colUserID`). Стандарт требует экспортированные: `TableUsers`, `UserColumnID`, `UserColumnEmail`.
- **Как исправить**: Переименовать в `Table{Entity}` и `{Entity}Column{Field}` формат, сделать экспортированными.

### I3. Repository: нет stom-based предвычисленных колонок

- **Файлы**: все файлы `backend/internal/repository/*.go`
- **Стандарт**: `docs/standards/backend-repository.md`
- **Что не так**: Стандарт описывает паттерн с `stom` для предвычисленных `selectColumns` и `insertMapper`. Текущий код перечисляет колонки вручную в каждом запросе.
- **Как исправить**: Добавить dual tags (`db` + `insert`) на Row struct, создать precomputed column lists через stom.

### I4. Repository: возвращает значения, не указатели

- **Файлы**: все методы во всех файлах `backend/internal/repository/*.go`
- **Стандарт**: `docs/standards/backend-repository.md`
- **Что не так**: Методы возвращают `(UserRow, error)`, `(BrandRow, error)`. Стандарт: "Методы репозитория возвращают указатели на структуры, не структуры по значению."
- **Как исправить**: Изменить сигнатуры на `(*UserRow, error)`, `([]*BrandRow, error)`.

### I5. Кастомные инфраструктурные утилиты

- **Файлы**: `backend/internal/config/config.go:111-182`, `backend/internal/closer/closer.go`
- **Стандарт**: `docs/standards/backend-libraries.md` [REQUIRED]
- **Что не так**: Кастомный парсинг env vars (getEnv, getBoolEnv и т.д.) и кастомный graceful shutdown (closer пакет). Стандарт: "Кастомные хелперы для стандартных инфраструктурных задач — не писать."
- **Как исправить**: Конфиг — `caarlos0/env` или `kelseyhightower/envconfig`. Closer — `oklog/run` или стандартный паттерн с errgroup.

### I6. bcryptCost — глобальная мутируемая переменная

- **Файл**: `backend/internal/service/auth.go:16-17,48-49`
- **Стандарт**: `docs/standards/backend-design.md`
- **Что не так**: `var bcryptCost = 12` — package-level mutable var, меняется в `NewAuthService()`. BrandService использует её неявно. Стандарт: "Структура иммутабельна после создания."
- **Как исправить**: Передавать bcryptCost как поле в AuthService и BrandService через конструктор.

### I7. domain дублирует сгенерированные API типы

- **Файлы**: `backend/internal/domain/brand.go`, `backend/internal/domain/response.go`
- **Стандарт**: `docs/standards/backend-codegen.md`, `docs/standards/backend-design.md`
- **Что не так**: `domain.Brand`, `domain.ManagerInfo`, `domain.APIResponse`, `domain.APIError` — ручные дубликаты типов из `api/server.gen.go`. Стандарт: "Ручные дубликаты API request/response запрещены."
- **Как исправить**: Использовать `api.APIError`, `api.Brand` и т.д. из сгенерированного кода. В domain — только бизнес-ошибки и бизнес-константы.

### I8. authz.go: строковые литералы в SQL

- **Файл**: `backend/internal/authz/authz.go:33-35`
- **Стандарт**: `docs/standards/backend-constants.md` [CRITICAL]
- **Что не так**: SQL-запрос использует строковые литералы `"brand_managers"`, `"user_id = ? AND brand_id = ?"` вместо констант из repository.
- **Как исправить**: Использовать экспортированные константы из repository пакета.

### I9. TODO без номера issue

- **Файл**: `backend/internal/authz/authz.go:22`
- **Стандарт**: `docs/standards/naming.md`
- **Что не так**: `// TODO: implement when campaigns table exists` — нет ссылки на GitHub issue.
- **Как исправить**: Создать issue, формат: `// TODO(#N): implement when campaigns table exists`.

### I10. Тесты: assert вместо require

- **Файлы**: `backend/internal/handler/auth_test.go`, `brand_test.go`, `service/*_test.go`
- **Стандарт**: `docs/standards/backend-testing-unit.md`
- **Что не так**: Повсеместное использование `assert.Equal` вместо `require.Equal`. Стандарт: "`require` везде — первый провал останавливает тест."
- **Как исправить**: Заменить `assert` → `require` во всех assertions.

### I11. Тесты: сырой JSON вместо типизированных структур

- **Файлы**: `backend/internal/handler/auth_test.go`, `brand_test.go`
- **Стандарт**: `docs/standards/backend-testing-unit.md`
- **Что не так**: Request body — строковые литералы JSON. Стандарт: "Сырой JSON в тестах запрещён — request body и response body через типизированные структуры кодогенерации."
- **Как исправить**: Формировать request через `api.LoginRequest{Email: "...", Password: "..."}` → `json.Marshal`.

### I12. Тесты: нейминг не по стандарту, нет t.Run группировки

- **Файлы**: все `*_test.go`
- **Стандарт**: `docs/standards/backend-testing-unit.md`
- **Что не так**: Flat naming (`TestLoginHandler_Success`, `TestCreateBrand_Success`). Стандарт: `Test{Struct}_{Method}` с сценариями внутри через `t.Run`.
- **Как исправить**: Группировать: `TestAuthHandler_Login` → `t.Run("success", ...)`, `t.Run("invalid JSON", ...)` и т.д.

### I13. Тесты хендлеров: не через роутер с ServerInterfaceWrapper

- **Файлы**: `backend/internal/handler/auth_test.go`, `brand_test.go`
- **Стандарт**: `docs/standards/backend-testing-unit.md`
- **Что не так**: Тесты вызывают методы хендлера напрямую. Стандарт: "Запрос прогоняется через роутер с ServerInterfaceWrapper."
- **Как исправить**: После C1 (переход на ServerInterface) — тесты через `httptest.NewServer` с `api.HandlerFromMux`.

---

## Мелкие — нейминг, форматирование, стиль

### M1. Нейминг интерфейсов и полей хендлеров

- **Файлы**: `backend/internal/handler/auth.go:18,29`, `brand.go:17,31`, `audit.go:15,21`
- **Стандарт**: `docs/standards/naming.md`
- **Что не так**: Интерфейсы `Auth`, `Brands`, `AuditLogs` — без суффикса слоя. Поля `auth`, `brands`, `audit` — тоже. Стандарт: интерфейсы `AuthService`, `BrandService`; поля `authService`, `brandService`.
- **Как исправить**: Переименовать при рефакторинге хендлеров.

### M2. Log level: строковые литералы вместо констант

- **Файл**: `backend/cmd/api/main.go:43-49`
- **Стандарт**: `docs/standards/backend-constants.md`
- **Что не так**: `"debug"`, `"warn"`, `"error"` — строковые литералы для конечного набора значений.
- **Как исправить**: Определить константы или использовать библиотечный enum.

---

## Статистика

- **Всего файлов проверено**: 42 (30 исходных + 12 тестовых)
- **Нарушений найдено**: 19 (4 критичных / 13 важных / 2 мелких)

---

## Рекомендация

Приоритетный порядок исправлений:

1. **C1+C2+C3 вместе** — переход на кодогенерацию (ServerInterface), ENVIRONMENT, авторизация через middleware/AuthzService. Это единый блок: пока роуты ручные, нет смысла фиксить авторизацию и тесты отдельно.

2. **C4 + I5** — конфиг: заменить кастомные хелперы на библиотеку, убрать молчаливые fallback'и, добавить ENVIRONMENT.

3. **I2 + I3 + I4 + I8** — repository: экспортированные константы, stom-based columns, указатели в возвратах. Атомарный блок.

4. **I1 + I7 + I6** — domain cleanup: убрать дубликаты API типов, убрать зависимость handler→repository, bcryptCost через DI.

5. **I10-I13** — тесты: после перехода на ServerInterface (блок 1) переписать тесты по стандарту.

6. **Остальное** — мелочи по мере попутного рефакторинга.

Блок 1 — самый большой и архитектурно значимый. Рекомендую начать с `/plan-standards backend` для детального плана.
