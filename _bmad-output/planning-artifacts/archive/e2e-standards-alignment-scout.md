# Scout: приведение backend e2e тестов к стандартам проекта

**Дата:** 2026-04-19
**Ветка:** `alikhan/staging-cicd`
**Запрос:** привести e2e тесты бэкенда к стандартам проекта (`docs/standards/`).

## Контекст

Текущий backend e2e (`backend/e2e/`) имеет работоспособный фундамент: отдельный Go-модуль, сгенерированный из OpenAPI клиент (`apiclient/`, `testclient/`), composable хелперы в `testutil/`, `t.Parallel()` на всех тестах. Но структура тестов и инфраструктура отстают от того, что прописано в `docs/standards/backend-testing-e2e.md` и связанных стандартах. Задача — рефакторинг под стандарт без потери покрытия.

## Затронутые области

### Файлы, которые правим
- `backend/e2e/auth/auth_test.go` — 28 функций → 6 (`TestHealthCheck`, `TestLogin`, `TestRefresh`, `TestGetMe`, `TestLogout`, `TestPasswordReset`, `TestFullAuthFlow`)
- `backend/e2e/brand/brand_test.go` — 14 функций → 3 (`TestBrandCRUD`, `TestBrandManagerAssignment`, `TestBrandIsolation`)
- `backend/e2e/audit/audit_test.go` — 5 функций → 2 (`TestAuditLogFiltering`, `TestAuditLogAccess`)
- `backend/e2e/testutil/seed.go` — заменить `LoginAs*`/`SeedBrand*` на composable Setup-конструкторы
- `backend/e2e/testutil/client.go` — добавить retry на транспортном уровне

### Файлы, которые добавляем
- `backend/e2e/testutil/cleanup.go` — defer-based cleanup stack + env var `E2E_CLEANUP`
- `backend/e2e/testutil/raw.go` — `PostRaw(t, path, body)` для запросов в обход типобезопасной валидации сгенерированного клиента (пустой/невалидный email)

### Файлы backend-стороны test API
- `backend/api/openapi-test.yaml` — убрать `POST /test/seed-brand`; добавить `POST /test/cleanup-entity` с body `{ type: "user"|"brand", id: "..." }`
- `backend/internal/handler/testapi.go` — удалить `SeedBrand` handler; добавить `CleanupEntity` handler (диспатч по `type` → repository delete)
- После — `make generate-api` для регенерации `apiclient/`, `testclient/`, `backend/internal/testapi/server.gen.go`

### Файлы вне зоны изменений
- `backend/api/openapi.yaml` — без изменений; используем уже существующие бизнес-ручки в setup/teardown (`POST /brands`, `POST /brands/{id}/managers`, `DELETE /brands/{id}`)
- `Makefile` (`test-e2e-backend`) — без изменений: `-race -count=1 -timeout 5m` уже есть; `E2E_CLEANUP` читается тестами напрямую из env

### Зависимости и инварианты
- Сгенерированные клиенты (`apiclient/`, `testclient/`) — не трогаем руками, регенерируются через `make generate-api`
- E2E запускается через `make test-e2e-backend`, требует `make start-backend` (Docker, порт 8082)
- `ENABLE_TEST_ENDPOINTS=true` ставится только в `local`/`staging` — production защищён

## Целевые паттерны реализации

Источники: `backend-testing-e2e.md`, `naming.md`, `backend-libraries.md`, `backend-design.md`, `security.md`.

### 1. Нейминг по бизнес-сценарию
Одна функция на endpoint/сценарий, edge cases внутри через `t.Run`:

```go
func TestLogin(t *testing.T) {
    t.Parallel()
    t.Run("empty email returns 422", func(t *testing.T) { … })
    t.Run("non-existent email returns 401", func(t *testing.T) { … })
    t.Run("wrong password returns 401", func(t *testing.T) { … })
    t.Run("short password returns 401 (no info leak)", func(t *testing.T) { … })
    t.Run("email normalization (trim + lowercase)", func(t *testing.T) { … })
    t.Run("success returns access token + refresh cookie", func(t *testing.T) { … })
}
```

Порядок `t.Run` — сначала валидация/early-exits, в конце happy path.

### 2. Composable Setup-хелперы в `testutil/`
Маленькие конструкторы по образцу из стандарта:

```go
adminToken := testutil.SetupAdmin(t)
brandID := testutil.SetupBrand(t, adminToken, name)
client, mgrToken, mgrEmail := testutil.SetupManagerWithLogin(t, adminToken, brandID)
```

Состав:
- `SetupAdmin(t) (token string)` и `SetupAdminClient(t) (*Client, token)` — два варианта
- `SetupBrand(t, adminToken, name) (brandID string)` — через бизнес-ручку `POST /brands`
- `SetupManager(t, adminToken, brandID) (email, password)` — через бизнес-ручку `POST /brands/{id}/managers`
- `SetupManagerWithLogin(t, adminToken, brandID) (client, token, email)` — то же + сразу логин

Каждый Setup-хелпер автоматически регистрирует cleanup через `RegisterCleanup`.

### 3. Setup только через бизнес-ручки
`/test/*` используется только когда нет бизнес-flow:
- `POST /test/seed-user` — нет бизнес-ручки создания admin
- `GET /test/reset-tokens` — раскрытие raw token из памяти
- `POST /test/cleanup-entity` — нет публичной ручки удаления users (брендов есть, но cleanup унифицирован под одну ручку)

`POST /test/seed-brand` удаляется. Создание бренда в тестах — только через `POST /brands` (с admin-токеном).

### 4. HTTP retry на транспортном уровне
- Retry на: connection refused, timeout, DNS resolution, 502/503/504
- Не retry на: 4xx, 500, любой ответ от приложения

Реализация — через `hashicorp/go-retryablehttp` (`backend-libraries.md` запрещает велосипедить устоявшиеся утилиты). Retry-transport оборачивается в `cfAccessTransport` так, чтобы CF Access headers ставились до retry-цикла.

### 5. Cleanup stack
Defer-based LIFO через `t.Cleanup` (Go вызывает в обратном порядке регистрации, FK-safe). Управление через env var `E2E_CLEANUP` (default `true`). Удаление через бизнес-ручку приоритетно (`DELETE /brands/{id}` для брендов), иначе через `POST /test/cleanup-entity` (для users).

```go
func RegisterCleanup(t *testing.T, fn func(context.Context) error) {
    t.Cleanup(func() {
        if os.Getenv("E2E_CLEANUP") == "false" { return }
        ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
        defer cancel()
        if err := fn(ctx); err != nil {
            t.Logf("cleanup failed: %v", err) // не падаем — соседние cleanup должны выполниться
        }
    })
}
```

Setup-хелперы регистрируют cleanup сразу при создании сущности.

### 6. Strict assertions: динамические поля + целостное сравнение
Проверить ID/время отдельно (UUID не пустой, время через `require.WithinDuration`), подменить на ожидаемые значения, потом `require.Equal` на всю структуру. Field-by-field проверки маскируют регрессии в дополнительных полях.

### 7. Полные проверки ошибок
Для всех 4xx — status code + `error.code` + `error.message`. Текущий `TestLogin_WrongPassword` — образец: проверяет `JSON401.Error.Code == "UNAUTHORIZED"`. Этот паттерн распространяется на все error-сценарии.

### 8. Header-комментарий с полным описанием
По стандарту: что тестируется, каждый шаг, какие данные создаются, что ожидаем. Однострочные header-комментарии расширяются до полного описания flow.

### 9. Константы
В тестах допустимы строковые литералы (e2e — изолированный модуль). Часто повторяющиеся литералы (`"testpass123"`, `"refresh_token"`) выносим в локальные const пакета `testutil`. Роли — из сгенерированного `apiclient.Admin` / `apiclient.BrandManager`.

### 10. Raw HTTP запросы (узкое исключение)
`openapi_types.Email` валидируется на стороне клиента до отправки → сгенерированный клиент не позволяет послать пустой email. Для тестов на бэкенд-валидацию используется `testutil.PostRaw(t, path, body)` — обёртка через тот же retry-transport и CF Access headers, что и обычный клиент. В каждом месте использования — короткий комментарий «почему raw, а не сгенерированный клиент».

## Риски и соображения

### Risk 1: переход с `/test/seed-brand` на бизнес-ручку медленнее
Каждый `SetupBrand` = `POST /brands` с admin-токеном. Митигация: в одном тесте `SetupAdmin` зовётся один раз и шарится между несколькими `SetupBrand`. Если на CI обнаружится регрессия по времени — обсуждается отдельно.

### Risk 2: HTTP retry может маскировать баги
Только для transport-уровня (connection refused/timeout/DNS) и явных 5xx (502/503/504). 4xx и 500 от приложения retry не делаем — это валидные ответы.

### Risk 3: рефакторинг 47 → ~11 функций
Большой diff. Покрытие должно остаться полным — после каждого шага локально гоняем `make test-e2e-backend` (`feedback_local_build_first`).

### Risk 4: `t.Parallel()` + `t.Run`
Объединение в `t.Run` корректно, если в каждом `t.Run` тоже зовётся `t.Parallel()`. Setup делается изолированно внутри каждого `t.Run`.

### Edge cases
- Cleanup-fail в одном тесте не должен ломать соседние тесты (`t.Logf` вместо `t.Errorf`)
- При `E2E_CLEANUP=false` тесты должны проходить (идемпотентность через `UniqueEmail`)
- Глобальные счётчики только `runID + atomic counter` (уже есть)

### Безопасность
- `POST /test/cleanup-entity` регистрируется только при `ENVIRONMENT=local` (как и остальные `/test/*`)
- Расширение test API не ослабляет защиту production (`security.md`: secure by default)

## План реализации

Всё одной серией изменений в текущей ветке (`alikhan/staging-cicd`), без промежуточных коммитов. После каждого шага — локальный `make test-e2e-backend`, рабочее дерево всегда зелёное.

### Шаг 1: фундамент инфраструктуры
- `testutil/cleanup.go` — `RegisterCleanup`, чтение `E2E_CLEANUP`
- `testutil/client.go` — retry через `hashicorp/go-retryablehttp`, обёрнутый в `cfAccessTransport`
- `backend/e2e/go.mod` — добавить зависимость `hashicorp/go-retryablehttp`
- `testutil/raw.go` — `PostRaw(t, path, body)` для запросов в обход валидации клиента
- `testutil/` — локальные const (`DefaultPassword`, `RefreshCookieName`, `EnvCleanup`)
- `backend/api/openapi-test.yaml` — убрать `POST /test/seed-brand`, добавить `POST /test/cleanup-entity`
- `backend/internal/handler/testapi.go` — удалить `SeedBrand`, добавить `CleanupEntity`
- `make generate-api` — регенерация клиентов и server stubs

### Шаг 2: composable хелперы
- `testutil/seed.go` — заменить `LoginAsAdmin`/`LoginAsBrandManager`/`SeedBrand`/`SeedBrandWithManager` на:
  - `SetupAdmin`, `SetupAdminClient`
  - `SetupBrand` (через `POST /brands`)
  - `SetupManager` (через `POST /brands/{id}/managers`)
  - `SetupManagerWithLogin`
- Каждый Setup-хелпер автоматически регистрирует cleanup через `RegisterCleanup`
- Старые сигнатуры удалить полностью (вне `backend/e2e/` callers нет — модуль изолирован)

### Шаг 3: рефакторинг auth_test.go
- Объединить 28 функций в 6 по endpoint
- Полные assertions (`error.code` + `error.message` для 4xx)
- Целостное сравнение успешных ответов через подмену динамических полей
- Расширенный header-комментарий

### Шаг 4: рефакторинг brand_test.go
- 14 → 3 (`TestBrandCRUD`, `TestBrandManagerAssignment`, `TestBrandIsolation`)
- То же по assertions/комментариям

### Шаг 5: рефакторинг audit_test.go
- 5 → 2 (`TestAuditLogFiltering`, `TestAuditLogAccess`)
- То же
