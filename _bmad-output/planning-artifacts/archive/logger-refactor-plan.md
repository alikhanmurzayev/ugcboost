---
title: План реализации — рефакторинг логирования backend через DI
type: implementation-plan
date: 2026-04-19
owner: Alikhan
status: approved, ready for /build
---

# План реализации: рефакторинг логирования backend через DI

> Этот документ — самодостаточный. Чтобы выполнить задачу, не нужно читать ничего, кроме явно перечисленных в Шаге 0 источников. Ссылок на бриф/скаут нет.

## 1. Обзор и мотивация

Сейчас в `backend/` логирование идёт через глобальный пакет `log/slog` — прямые вызовы `slog.Info/Error/Warn/Debug` разбросаны по сервисам, хендлерам, мидлварам, closer'у и main'у. Нужно заменить это на **инжектируемый интерфейс `Logger`**, который:

1. **Позволяет unit-тестировать каждый лог.** Логирование рассматривается как часть бизнес-контракта: уровень (Info/Warn/Error/Debug), текст и attrs — критичная operational информация, security-чувствительные сценарии (пароль/токен не утёк в Info) должны быть автоматизированы.
2. **Соответствует архитектурному стандарту** проекта «зависимости через конструктор». Сейчас `slog` — единственное исключение.
3. **Готов к будущим расширениям** без breaking change API: trace ID из контекста, per-request log level override через заголовок, user enrichment, sampling, OpenTelemetry-корреляция. Поэтому каждый метод интерфейса принимает `context.Context` первым параметром, **даже если сейчас контекст не используется** (placeholder `_`).

**Скоуп:** только backend (`backend/`). Один коммит в текущей ветке `alikhan/staging-cicd`. **Коммит делает Alikhan вручную** после ревью — Claude не коммитит.

## 2. Целевая архитектура

### 2.1 Пакет `internal/logger`

Новый пакет, единственное место в проекте где напрямую используется `log/slog` (помимо тонкой конструкции `*slog.Logger` в `cmd/api/main.go`). Содержит:

```go
package logger

import (
    "context"
    "log/slog"
)

// Logger is the single logging abstraction used across all backend layers.
// All implementations accept context to support future per-request overrides
// (trace IDs, dynamic log level, user context enrichment, etc.).
type Logger interface {
    Debug(ctx context.Context, msg string, args ...any)
    Info(ctx context.Context, msg string, args ...any)
    Warn(ctx context.Context, msg string, args ...any)
    Error(ctx context.Context, msg string, args ...any)
    With(args ...any) Logger
}

// SlogLogger is the production implementation backed by *slog.Logger.
type SlogLogger struct {
    inner *slog.Logger
}

func New(inner *slog.Logger) *SlogLogger { return &SlogLogger{inner: inner} }

func (l *SlogLogger) Info(_ context.Context, msg string, args ...any) {
    // ctx ignored for now. Future: extract trace ID, per-request log level
    // override, user_id enrichment.
    l.inner.Info(msg, args...)
}
// ... аналогично Debug/Warn/Error
func (l *SlogLogger) With(args ...any) Logger {
    return &SlogLogger{inner: l.inner.With(args...)}
}
```

**Исключение из правила «interfaces in consumer package»:** интерфейс `Logger` живёт в том же пакете, что и реализация. Иначе пришлось бы дублировать определение во всех слоях (handler/service/middleware/closer) — это семантически принадлежит пакету-логгеру.

**Нейминг `SlogLogger`:** реализация интерфейса `Logger` через бекенд stdlib `log/slog`. Префикс «Slog» обозначает технологию реализации — оставляем место для альтернатив в будущем (например, `ZapLogger` если перейдём на zap). По `naming.md` это согласуется с конвенцией интерфейсов с суффиксом слоя (`Logger`) и конкретных реализаций как отдельных типов.

### 2.2 DI — во все слои, которые логируют (напрямую или транзитивно)

Логгер создаётся **один раз** в `cmd/api/main.go` (`appLogger := logger.New(slog.New(...))`) и передаётся последним параметром в конструкторы:

- `service.NewAuthService(pool, repoFactory, tokens, resetNotifier, bcryptCost, logger)`
- `service.NewBrandService(pool, repoFactory, bcryptCost, logger)`
- `closer.New(logger)`
- `middleware.Recovery(logger)` — теперь фабрика (как `Auth(validator)`)
- `middleware.Logging(logger)` — теперь фабрика
- `handler.NewServer(auth, brands, authz, audit, version, cookieSecure, logger)`
- `handler.NewTestAPIHandler(auth, brands, tokenStore, adminID, logger)` — этот хендлер **транзитивно логирует** через общие хелперы `respondError`/`encodeJSON`, поэтому тоже получает logger. Дополнительно: переименовываем `TestHandler` → `TestAPIHandler` (см. раздел 4.2)

**НЕ получают** logger (нет ни прямых, ни транзитивных вызовов логгирования):
- `service.NewAuditService` — без изменений
- `authz.NewAuthzService` — без изменений

### 2.3 Использование в `cmd/api/main.go`

Голых вызовов `slog.Info/Warn/Error/Debug` в `main.go` НЕТ. Все собственные startup/shutdown логи идут через `appLogger.*(ctx, ...)`.

В `main.go` остаются только:
1. Конструкция `slogLogger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: cfg.LogLevel}))` — вход для `logger.New(...)`
2. Опционально `slog.SetDefault(slogLogger)` — чтобы перехватить логи сторонних библиотек, если такие появятся
3. **Один fall-back** `slog.Error("fatal", "error", err)` в `main()` (не `run()`), на случай когда logger ещё не создан (например, упал `config.Load()` до конструкции `appLogger`). Документируется комментарием в коде.

### 2.4 `internal/config/config.go`

Импорт `log/slog` остаётся — но **только** для типа поля `LogLevel slog.Level`. Это env-парсинг, не вызов. Не нарушает критерий «нет вызовов `slog.*`».

## 3. Политика тестирования логов

**Логи — часть бизнес-контракта, проверяются наравне с return-значениями.**

- В сценариях, где лог ожидается — `MockLogger.EXPECT().Info|Warn|Error|Debug(...)` с точными args. Если в коде вызвался лог, для которого нет `EXPECT()` — mockery в strict mode (`t.Cleanup` + `AssertExpectations`) уронит тест. Это даёт автоматическую негативную проверку.
- Уровень лога (Info vs Warn vs Error) — отдельные методы интерфейса, опечатка = ошибка компиляции.
- Текст сообщения — точное совпадение в `EXPECT()`. **Хрупкость текстов принята осознанно**: если меняется формулировка — тест чинится вместе с кодом.
- Attrs (key/value) — точные ключи в `EXPECT()`. Для **динамических значений** (stack trace, duration, time.Time, error) использовать `mock.Anything`. Для проверки конкретного значения — `.Run(func(args mock.Arguments) { ... })` с `require`.
- **Negative tests на security:**
  - В `service.AuthService.RequestPasswordReset`: убедиться что raw reset token НЕ передаётся в `Info` (через `.Run` + `require.NotContains` на маршалинг args в строку).
  - В `service.BrandService.AssignManager`: убедиться что `tempPassword` и `email` НЕ передаются.
  - В `middleware.Logging`: убедиться что заголовок `Authorization` и значение cookie НЕ попадают в args.

## 4. Дополнительные изменения, которые закрываем в этом же рефакторинге

### 4.1 Security-фикс в `service/brand.go`

**Файл `backend/internal/service/brand.go`, текущая строка 192:**
```go
slog.Info("temporary password generated for new manager", "email", email)
```

Это нарушение `docs/standards/security.md` — email PII в связке с фактом «временный пароль создан» = чувствительная цепочка в operational логах. **Заменить на:**
```go
s.logger.Info(ctx, "temporary password generated for new manager", "user_id", userRow.ID)
```

`userRow.ID` доступен здесь же — сразу после `userRow, err = userRepo.Create(...)`. Email и так попадает в `audit_logs` через `writeAudit` для целей аудита — это правильное место.

### 4.2 Переименование `TestHandler` → `TestAPIHandler`

Сейчас в пакете `handler` файл `test.go` (содержит `TestHandler`) парится с тестовым файлом `test_test.go` — двойное "test" путает. Заодно с logger-рефакторингом переименовываем (это бесплатно — только текстовые правки + регенерация моков):

| Было | Стало |
|---|---|
| `backend/internal/handler/test.go` | `backend/internal/handler/testapi.go` |
| `backend/internal/handler/test_test.go` | `backend/internal/handler/testapi_test.go` |
| struct `TestHandler` | `TestAPIHandler` |
| `NewTestHandler(...)` | `NewTestAPIHandler(...)` |
| interface `TestAuthService` | `TestAPIAuthService` |
| interface `TestBrandService` | `TestAPIBrandService` |
| interface `TokenStore` | `TokenStore` (без изменений — нет "test" в имени) |
| `handler/mocks/mock_test_auth_service.go` | `mock_test_api_auth_service.go` или `mock_testapi_auth_service.go` — точное имя зависит от sprig snakecase для `TestAPI`; проверить через `ls` после `make generate-mocks` (см. Шаг 6.3) |
| `handler/mocks/mock_test_brand_service.go` | аналогично — точное имя проверить после генерации |
| Тестовые функции `TestTestHandler_*` | `TestTestAPIHandler_*` (двойной "Test" остаётся, но первый — Go-префикс, не дублирование смысла) |

Все ссылки в `cmd/api/main.go` (`handler.NewTestHandler(...)` → `handler.NewTestAPIHandler(...)`) и в самом тесте (`mocks.NewMockTestAuthService` → `mocks.NewMockTestAPIAuthService` и т.д.) обновляются.

## 5. Шаг 0: загрузить контекст в память (обязательно перед началом)

**Этот шаг не пропускать.** План написан с учётом стандартов проекта и текущего состояния кода — без их полного знания шаги 1-9 нельзя выполнить корректно.

### 5.1 Прочитать стандарты `docs/standards/` (бэкенд — задача чисто бэкендовая)

- [ ] `docs/standards/backend-architecture.md`
- [ ] `docs/standards/backend-codegen.md`
- [ ] `docs/standards/backend-constants.md`
- [ ] `docs/standards/backend-design.md` — особенно «Зависимости через конструктор»
- [ ] `docs/standards/backend-errors.md`
- [ ] `docs/standards/backend-libraries.md` — особенно про «инфраструктурный код через библиотеки»
- [ ] `docs/standards/backend-repository.md`
- [ ] `docs/standards/backend-testing-e2e.md`
- [ ] `docs/standards/backend-testing-unit.md` — особенно «Точные аргументы моков», «Порядок вызовов»
- [ ] `docs/standards/backend-transactions.md`
- [ ] `docs/standards/naming.md` — нейминг пакета, структуры, интерфейса
- [ ] `docs/standards/security.md` — особенно «Логирование без чувствительных данных»

Фронтенд-стандарты — пропускаем (задача не затрагивает frontend).

**Зачем:** план опирается на конкретные правила (DI через конструктор; интерфейсы у консумера; mockery + testify; `t.Parallel`; точные args; запрет PII в логах; backend-libraries про graceful shutdown). Если что-то изменилось в стандартах с момента составления плана — приоритет у текущей версии стандарта, не у плана.

### 5.2 Прочитать ВСЕ затрагиваемые файлы текущего кода

Прод-код:
- [ ] `backend/cmd/api/main.go`
- [ ] `backend/internal/config/config.go`
- [ ] `backend/internal/closer/closer.go`
- [ ] `backend/internal/middleware/recovery.go`
- [ ] `backend/internal/middleware/logging.go`
- [ ] `backend/internal/middleware/auth.go` (как референс паттерна factory-of-handler)
- [ ] `backend/internal/service/auth.go`
- [ ] `backend/internal/service/brand.go`
- [ ] `backend/internal/service/audit.go` (для контекста, не модифицируется)
- [ ] `backend/internal/handler/server.go`
- [ ] `backend/internal/handler/response.go`
- [ ] `backend/internal/handler/auth.go`
- [ ] `backend/internal/handler/audit.go`
- [ ] `backend/internal/handler/brand.go` (не логирует напрямую, но вызывает `respondJSON`/`respondError` — нужно обновить вызовы)
- [ ] `backend/internal/handler/test.go` (`TestHandler` — переименовываем в `TestAPIHandler` + добавляем logger в DI, см. 4.2)

Тесты:
- [ ] `backend/internal/closer/closer_test.go`
- [ ] `backend/internal/middleware/recovery_test.go`
- [ ] `backend/internal/service/auth_test.go`
- [ ] `backend/internal/service/brand_test.go`
- [ ] `backend/internal/service/helpers_test.go`
- [ ] `backend/internal/handler/auth_test.go`
- [ ] `backend/internal/handler/brand_test.go`
- [ ] `backend/internal/handler/audit_test.go`
- [ ] `backend/internal/handler/test_test.go`
- [ ] `backend/internal/handler/helpers_test.go`

Конфиги:
- [ ] `backend/.mockery.yaml`
- [ ] `backend/Makefile` (через корневой `Makefile`) — таргеты `generate-mocks`, `test-unit-backend`, `test-unit-backend-coverage`, `lint-backend`, `test-e2e-backend`

### 5.3 Проверить grep'ом текущее состояние

```
grep -rE "slog\.(Info|Error|Warn|Debug)" backend/
grep -rE '"log/slog"' backend/
```

Это baseline. К концу работ первый grep должен быть пуст в `backend/internal/`, а в `backend/cmd/api/main.go` остаться **только один** fall-back `slog.Error("fatal", ...)` в `main()` (см. 2.3).

## 6. Требования

### Must-have

- **REQ-1.** Создан пакет `internal/logger` с интерфейсом `Logger` и реализацией `SlogLogger` (см. 2.1). Все методы интерфейса принимают `context.Context` первым параметром.
- **REQ-2.** Все вызовы `slog.Info/Warn/Error/Debug` в `backend/internal/` заменены на `logger.Logger.*`. В `backend/cmd/api/main.go` остаётся только один fall-back `slog.Error` в `main()`.
- **REQ-3.** `Logger` инжектится через конструкторы: `service.NewAuthService`, `service.NewBrandService`, `handler.NewServer`, `handler.NewTestAPIHandler` (транзитивно через общие хелперы), `closer.New`, `middleware.Recovery`, `middleware.Logging`. `AuditService`, `AuthzService` НЕ получают logger.
- **REQ-4.** mockery генерит `internal/logger/mocks/mock_logger.go` со strict-mode `EXPECT()`-API.
- **REQ-5.** Все существующие unit-тесты обновлены под новые сигнатуры конструкторов и под политику тестирования логов (раздел 3).
- **REQ-6.** Реализованы negative security-тесты для `RequestPasswordReset`, `AssignManager`, `Logging` middleware (см. 3).
- **REQ-7.** В `service/brand.go` лог `temporary password generated for new manager` использует `user_id`, не `email` (см. 4.1).
- **REQ-8.** Все make-цели зелёные: `build-backend`, `lint-backend`, `test-unit-backend`, `test-unit-backend-coverage`, `test-e2e-backend`.
- **REQ-9.** Coverage gate ≥ 80% per-method на изменённых пакетах из whitelist Makefile (`handler`, `service`, `repository`, `middleware`, `authz`). Пакет `internal/logger` **не входит** в whitelist coverage gate, но покрытие ≥ 80% обеспечивается тестами вручную для качества (REQ-1).
- **REQ-10.** Переименование `TestHandler` → `TestAPIHandler` (см. 4.2): файлы `test.go`/`test_test.go` → `testapi.go`/`testapi_test.go`; интерфейсы `TestAuthService`/`TestBrandService` → `TestAPIAuthService`/`TestAPIBrandService`; моки регенерены через `make generate-mocks`, старые мок-файлы удалены.

### Nice-to-have

- Создать `internal/middleware/logging_test.go` (его сейчас нет). Покрытие: success-кейс, негативный тест на отсутствие Authorization/cookie в args.

### Вне скоупа

- Реальное наполнение `ctx` (trace ID, per-request log level override, user enrichment) — отдельная задача после.
- OpenTelemetry / structured tracing.
- Изменение формата вывода (JSON/text), sampling, rate-limiting, multi-handler.
- Замена самописного `closer` на стороннюю библиотеку (отвергнуто — нет канонической Go-библиотеки под наш кейс LIFO + именованные ресурсы; код 60 строк, оставляем).
- Тестирование `cmd/api/main.go` (исключено из coverage gate).

## 7. Файлы для создания

| Файл | Назначение |
|---|---|
| `backend/internal/logger/logger.go` | Интерфейс `Logger` + `SlogLogger` обёртка + `New(*slog.Logger) *SlogLogger` + `With(args...) Logger` |
| `backend/internal/logger/logger_test.go` | Unit-тесты обёртки: proxy в `*slog.Logger` для каждого уровня (`Info/Debug/Warn/Error`), `AttrsFormat` (идентичность с прямым `slog.Info` через `JSONEq`), `With` chaining, `WithDoesNotMutate` (immutability), `IgnoresContext` (Background и nil) — детали в Шаге 1 |
| `backend/internal/logger/mocks/mock_logger.go` | Автоген mockery (после `make generate-mocks`) — НЕ редактировать руками |
| `backend/internal/middleware/logging_test.go` | Тесты `Logging` middleware: success log, нет Authorization, нет Cookie |

## 8. Файлы для изменения

### Прод-код

| Файл | Изменения |
|---|---|
| `backend/.mockery.yaml` | Добавить строку `github.com/alikhanmurzayev/ugcboost/backend/internal/logger:` |
| `backend/internal/closer/closer.go` | Конструктор: `New() *Closer` → `New(logger logger.Logger) *Closer`; `Closer` +`logger`; в `Close(ctx)`: `slog.Info("shutting down", ...)` → `c.logger.Info(ctx, ...)`, `slog.Error("shutdown error", ...)` → `c.logger.Error(ctx, ...)` |
| `backend/internal/middleware/recovery.go` | Превратить в фабрику: `func Recovery(logger logger.Logger) func(http.Handler) http.Handler`. Внутреннее `slog.Error("panic recovered", ...)` → через замыкание над `logger` с `logger.Error(r.Context(), ...)` |
| `backend/internal/middleware/logging.go` | Превратить в фабрику: `func Logging(logger logger.Logger) func(http.Handler) http.Handler`. Внутреннее `slog.Info("http request", ...)` → `logger.Info(r.Context(), ...)` |
| `backend/internal/service/auth.go` | `AuthService` +`logger logger.Logger`; `NewAuthService(pool, repoFactory, tokens, resetNotifier, bcryptCost)` → `NewAuthService(..., logger)`. Заменить 4× `slog.Info` на `s.logger.Info(ctx, ...)` в `RequestPasswordReset` (1×) и `SeedAdmin` (3×) |
| `backend/internal/service/brand.go` | `BrandService` +`logger`; `NewBrandService(pool, repoFactory, bcryptCost)` → `NewBrandService(..., logger)`. В `AssignManager`: заменить `slog.Info("temporary password generated for new manager", "email", email)` на `s.logger.Info(ctx, "temporary password generated for new manager", "user_id", userRow.ID)` |
| `backend/internal/handler/server.go` | `Server` +`logger`; `NewServer(auth, brands, authz, audit, version, cookieSecure)` → `NewServer(..., logger)` (logger последним) |
| `backend/internal/handler/response.go` | `encodeJSON`/`respondJSON`/`respondError`/`writeError` **остаются свободными функциями**, добавляется `logger logger.Logger` параметром (последним, чтобы сохранить семантику «зависимости в конце»). `slog.Error("failed to encode response", ...)` → `logger.Error(r.Context(), ...)`. `slog.Error("unexpected error", ...)` в default-branch `respondError` → `logger.Error(r.Context(), ...)`. Сигнатуры: `respondJSON(w, r, status, v, logger)`, `respondError(w, r, err, logger)`, `encodeJSON(w, r, v, logger)`, `writeError(w, r, status, code, msg, logger)` |
| `backend/internal/handler/auth.go` | Все `respondJSON`/`respondError` → передавать `s.logger` последним параметром. `slog.Error` в `Logout` (строка 81) → `s.logger.Error(r.Context(), "failed to revoke refresh tokens on logout", ...)`. `slog.Error` в `RequestPasswordReset` (строка 116) → `s.logger.Error(r.Context(), ...)` |
| `backend/internal/handler/audit.go` | `rawJSONToAny(id, raw)` (свободная функция) стать методом `(s *Server) rawJSONToAny(ctx context.Context, id string, raw []byte) interface{}`. Внутри `slog.Error("failed to unmarshal audit log value", ...)` → `s.logger.Error(ctx, ...)`. Call-site в `ListAuditLogs` обновить (×2). Все `respondJSON`/`respondError` → передавать `s.logger` |
| `backend/internal/handler/brand.go` | НЕ логирует напрямую, но вызывает `respondJSON`/`respondError` — обновить вызовы, передавать `s.logger` |
| `backend/internal/handler/test.go` → **переименовать** в `backend/internal/handler/testapi.go` | (1) Переименование файла. (2) `TestHandler` → `TestAPIHandler` (struct +`logger logger.Logger`). (3) `NewTestHandler(auth, brands, tokenStore, adminID)` → `NewTestAPIHandler(..., logger)`. (4) Интерфейсы `TestAuthService` → `TestAPIAuthService`, `TestBrandService` → `TestAPIBrandService`. (5) Все вызовы `respondJSON`/`respondError` обновить — передавать `h.logger` последним параметром |
| `backend/cmd/api/main.go` | После `cfg, err := config.Load()`: создать `slogLogger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: cfg.LogLevel}))`, опционально `slog.SetDefault(slogLogger)`, `appLogger := logger.New(slogLogger)`. Все собственные `slog.Info/Warn/Error` в `run()` → `appLogger.*(ctx, ...)`. **Один** `slog.Error("fatal", "error", err)` в `main()` оставить с комментарием — fall-back для случая когда logger ещё не создан. Прокинуть `appLogger` во все конструкторы из таблицы 2.2. Middleware регистрация: `r.Use(middleware.Recovery(appLogger))`, `r.Use(middleware.Logging(appLogger))` |

### Тесты

| Файл | Изменения |
|---|---|
| `backend/internal/closer/closer_test.go` | В каждом `t.Run` создавать `mockLogger := logmocks.NewMockLogger(t)`, передавать в `New(mockLogger)`. Ожидать `EXPECT().Info("shutting down", "resource", X)` для каждой Add в LIFO порядке. В кейсах с ошибками — `EXPECT().Error("shutdown error", "resource", X, "error", mock.Anything)`. В кейсе «empty» — никаких EXPECT (mockery strict проверит) |
| `backend/internal/middleware/recovery_test.go` | Сборка через `Recovery(mockLogger)(handler)`. В panic-кейсах: `EXPECT().Error("panic recovered", "panic", mock.Anything, "stack", mock.Anything, "path", "/")`. В success-кейсе — никаких EXPECT |
| `backend/internal/middleware/logging_test.go` (НОВЫЙ) | `TestLogging_LogsRequest`: `EXPECT().Info("http request", "method", "GET", "path", "/", "status", 200, "duration_ms", mock.Anything, "remote_addr", mock.Anything)`. `TestLogging_DoesNotLogAuthorization`: запрос с `Authorization: Bearer secret`, через `.Run` собрать args в строку и `require.NotContains(t, str, "Bearer secret")`, `require.NotContains(t, str, "secret")`. `TestLogging_DoesNotLogCookie`: аналогично с `Cookie: refresh_token=secret-cookie-value` |
| `backend/internal/service/auth_test.go` | Передавать `mockLogger` в `NewAuthService(...)` (последним аргументом). Добавить `EXPECT().Info(...)` в кейсах: `RequestPasswordReset/success` (текст `"password reset token generated"`, ключи `user_id`/`expires_at`, через `.Run` проверить отсутствие `raw` в args), `SeedAdmin/empty email/password skipped`, `SeedAdmin/admin already exists`, `SeedAdmin/admin user created`. В сценариях без логов (например `Login/wrong password`) — никаких EXPECT, mockery strict проверит |
| `backend/internal/service/brand_test.go` | Передавать `mockLogger`. Обновить `AssignManager/new user` тест: `EXPECT().Info("temporary password generated for new manager", "user_id", "<deterministic-id-from-mock-userRepo.Create>")`. Negative security: сохранить attrs через `.Run` в shared `var loggedAttrs []any`, ПОСЛЕ возврата `_, tempPass, err := svc.AssignManager(...)` (tempPass — второй return value) собрать в строку через `fmt.Sprint(loggedAttrs...)` и `require.NotContains(t, str, email)`, `require.NotContains(t, str, tempPass)`. Внутри `.Run` `tempPass` ещё не известен (генерится в `generateTempPassword()` через `crypto/rand`, не мокается) — детали в Шаге 5 |
| `backend/internal/handler/auth_test.go` | Все `NewServer(auth, nil, nil, nil, "test-version", false)` → `NewServer(..., mockLogger)`. **Рекомендуется** добавить локальный хелпер в `helpers_test.go` для уменьшения шума (см. ниже). EXPECT'ы: `Logout/service error still returns 200` → `EXPECT().Error("failed to revoke refresh tokens on logout", "error", mock.Anything, "userID", "u-admin")`. `RequestPasswordReset/always returns 200 even when service logs error` → `EXPECT().Error("password reset request failed", "error", mock.Anything)`. `ResetPassword/service error returns 500`, `GetMe/service error returns 500` → `EXPECT().Error("unexpected error", "error", mock.Anything, "path", X)` (catch-all из `respondError`) |
| `backend/internal/handler/brand_test.go` | Все `NewServer(...)` обновить. В кейсах service-error returns 500 → `EXPECT().Error("unexpected error", ...)` |
| `backend/internal/handler/audit_test.go` | Все `NewServer(...)` обновить. В кейсах с битым JSON → `EXPECT().Error("failed to unmarshal audit log value", "error", mock.Anything, "auditLogID", "<id>")` (один раз на каждое битое поле OldValue/NewValue) |
| `backend/internal/handler/test_test.go` → **переименовать** в `backend/internal/handler/testapi_test.go` | (1) Переименование файла. (2) Все `TestTestHandler_*` → `TestTestAPIHandler_*`. (3) `mocks.NewMockTestAuthService` → `mocks.NewMockTestAPIAuthService`, аналогично BrandService. (4) `NewTestHandler(auth, brands, store, seedAdminID)` → `NewTestAPIHandler(..., mockLogger)`. (5) В кейсах с `returns 500` (3 шт: SeedUser/service error, SeedBrand/brand create error, SeedBrand/assign manager error) → добавить `mockLogger.EXPECT().Error("unexpected error", "error", mock.Anything, "path", X)` (catch-all из `respondError`) |
| `backend/internal/handler/helpers_test.go` | Опционально: добавить хелпер `func newServerWithMocks(t *testing.T, auth handler.AuthService, brands handler.BrandService, authz handler.AuthzService, audit handler.AuditLogService) (*Server, *logmocks.MockLogger)` — создаёт mock и сервер за один вызов. Уменьшит шум в 18+ местах `auth_test.go` |

## 9. Шаги реализации

Каждый шаг = атомарное изменение прод-кода + обновление тестов **затронутого пакета**. После каждого шага тесты затронутого пакета (`go test ./internal/<package>/...`) зелёные.

**Важно про `make build-backend`:** между Шагом 2 и Шагом 7 полный build будет красным, потому что `cmd/api/main.go` использует старые сигнатуры (`closer.New()`, `middleware.Recovery`, `NewServer(...)` без logger), а пакеты эти сигнатуры уже сменили. Это сознательно — починить main.go раньше нельзя без полного DI-каркаса. На промежуточных шагах используем `cd backend && go test ./internal/<package>/...` для проверки конкретного пакета. Полный `make build-backend && make lint-backend && make test-unit-backend` запускается в Шаге 7 (когда main.go починен) и Шаге 8 (финальная валидация).

**Перед стартом — обязательно выполнить Шаг 0** (раздел 5).

### Шаг 1: создать пакет `internal/logger`

- [ ] Создать `backend/internal/logger/logger.go` по образцу из 2.1: интерфейс `Logger`, struct `SlogLogger`, `New(*slog.Logger) *SlogLogger`, методы `Debug/Info/Warn/Error/With`. **Все методы — на pointer receiver `(l *SlogLogger)`** (для consistency и чтобы `*SlogLogger` удовлетворял интерфейсу `Logger`). Все методы — proxy в `inner`. `ctx` помечен `_` с inline-комментарием про будущее использование.
- [ ] Добавить пакет в `backend/.mockery.yaml`: `github.com/alikhanmurzayev/ugcboost/backend/internal/logger:`
- [ ] Запустить `cd backend && make generate-mocks`. Убедиться что появился `backend/internal/logger/mocks/mock_logger.go` с `MockLogger`, `NewMockLogger(t)`, `EXPECT()`. **Сделать `git status`** — если затронуты другие моки (mockery может перегенерить всё), просмотреть diff'ы и убедиться что это benign-изменения (например, новые версии шаблона), а не регрессия.
- [ ] Создать `backend/internal/logger/logger_test.go`:
  - `TestSlogLogger_Info/Debug/Warn/Error` — для каждого уровня создать `*slog.Logger` с `slog.NewJSONHandler(buf, ...)`, обернуть в `New(...)`, вызвать метод, распарсить `buf.Bytes()` как JSON, проверить `level`, `msg`, attrs.
  - `TestSlogLogger_AttrsFormat` — убедиться что формат attrs идентичен прямому вызову `slog.Info`: вызвать `slog.Info(msg, "k1", "v1", "k2", 42)` и `wrapper.Info(ctx, msg, "k1", "v1", "k2", 42)` с одинаковым handler'ом (через два отдельных buf'а), сравнить JSON через `require.JSONEq`. Если расходится — обёртка ломает контракт.
  - `TestSlogLogger_With` — chaining: `l.With("k", "v").Info(ctx, "msg")` → JSON содержит `"k":"v"`.
  - `TestSlogLogger_WithDoesNotMutate` — `derived := l.With("k", "v"); l.Info(ctx, "x")` → output от исходного `l` НЕ содержит `"k":"v"`. Гарантия immutability.
  - `TestSlogLogger_IgnoresContext` — вызвать с `context.Background()` и с `nil` явно (`var ctx context.Context = nil; logger.Info(ctx, ...)`) — оба не должны падать. (Сейчас ctx помечен `_`, но тест фиксирует контракт.)
- [ ] `cd backend && go test ./internal/logger/...` → зелёный
- [ ] `cd backend && go test ./internal/logger/... -coverprofile=/tmp/logger-cover.out && go tool cover -func=/tmp/logger-cover.out` — каждый публичный метод ≥ 80%
- [ ] `make build-backend && make lint-backend` → зелёные (на этом шаге всё ещё зелёные, потому что мы только добавили новый пакет — старый код не тронут)

### Шаг 2: `internal/closer`

- [ ] `closer.New() *Closer` → `New(logger logger.Logger) *Closer`. Добавить поле `logger logger.Logger` в `Closer`.
- [ ] В `Close(ctx)` строки 47, 49: `slog.Info("shutting down", ...)` → `c.logger.Info(ctx, "shutting down", "resource", nf.name)`. `slog.Error("shutdown error", ...)` → `c.logger.Error(ctx, "shutdown error", "resource", nf.name, "error", err)`.
- [ ] Убрать импорт `"log/slog"` из `closer.go`. Добавить `"github.com/alikhanmurzayev/ugcboost/backend/internal/logger"`.
- [ ] Обновить `closer_test.go`:
  - В каждом `t.Run` создать `mockLogger := logmocks.NewMockLogger(t)`, передать в `New(mockLogger)`.
  - В `LIFO order` (3 Add): `EXPECT().Info("shutting down", "resource", "third").Once()`, потом `"second"`, потом `"first"` (LIFO).
  - В `all called on error` (3 Add, b возвращает ошибку): EXPECT'ы Info × 3, плюс EXPECT Error для "b".
  - В `returns first error` (2 Add, оба с ошибками): EXPECT Info × 2, EXPECT Error × 2.
  - В `empty` — никаких EXPECT (mockery strict проверит, что вызовов не было).
  - В `context passed` — EXPECT Info × 1.
- [ ] `cd backend && go test ./internal/closer/...` → зелёный. (Полный `make build-backend` будет красным до Шага 7 — это ожидаемо, см. преамбулу раздела 9.)

### Шаг 3: middleware (`recovery` + `logging`)

- [ ] `middleware.Recovery` стать фабрикой: текущая сигнатура `func Recovery(next http.Handler) http.Handler` → `func Recovery(logger logger.Logger) func(http.Handler) http.Handler`. Внутреннюю реализацию обернуть ещё одним `return func(next http.Handler) http.Handler { return http.HandlerFunc(...) }`. Заменить `slog.Error("panic recovered", ...)` на `logger.Error(r.Context(), "panic recovered", "panic", rec, "stack", string(debug.Stack()), "path", r.URL.Path)`.
- [ ] `middleware.Logging` стать фабрикой аналогично. Заменить `slog.Info("http request", ...)` на `logger.Info(r.Context(), "http request", "method", r.Method, "path", r.URL.Path, "status", rw.status, "duration_ms", time.Since(start).Milliseconds(), "remote_addr", r.RemoteAddr)`.
- [ ] Обновить `recovery_test.go`:
  - В каждом `t.Run` создать `mockLogger`, сборка `Recovery(mockLogger)(http.HandlerFunc(...))`.
  - В success-кейсе (`no panic`) — никаких EXPECT.
  - В panic-кейсах — `EXPECT().Error("panic recovered", "panic", mock.Anything, "stack", mock.Anything, "path", "/").Once()`.
- [ ] **Создать** `middleware/logging_test.go` (см. таблицу в 8 для трёх тестов).
- [ ] `cd backend && go test ./internal/middleware/...` → зелёный

### Шаг 4: `internal/service/auth`

- [ ] `AuthService` +`logger logger.Logger`. `NewAuthService(pool, repoFactory, tokens, resetNotifier, bcryptCost)` → `NewAuthService(pool, repoFactory, tokens, resetNotifier, bcryptCost, logger logger.Logger)`.
- [ ] `RequestPasswordReset` строка 209: `slog.Info("password reset token generated", "user_id", user.ID, "expires_at", expiresAt)` → `s.logger.Info(ctx, "password reset token generated", "user_id", user.ID, "expires_at", expiresAt)`. Убедиться что `raw` НЕ в args (как сейчас, но фиксировать).
- [ ] `SeedAdmin` строки 300, 311, 325: 3× `slog.Info(...)` → `s.logger.Info(ctx, ...)`.
- [ ] Убрать импорт `"log/slog"` из `service/auth.go`. Добавить импорт `logger`.
- [ ] Обновить `auth_test.go`:
  - Передавать `mockLogger` во все `NewAuthService(...)`.
  - `RequestPasswordReset/success`: `EXPECT().Info("password reset token generated", "user_id", "user-1", "expires_at", mock.Anything)` через `.Run` для проверки что в args нет `raw`-токена.
  - `SeedAdmin` тесты — добавить три кейса (если их нет, или обновить существующие): `empty` → Info с msg `"admin seed skipped: ADMIN_EMAIL or ADMIN_PASSWORD not set"`; `exists` → Info с msg `"admin already exists"`, `email`; `created` → Info с msg `"admin user created"`, `email`.
  - В сценариях без логов — никаких EXPECT (mockery strict проверит).
- [ ] `cd backend && go test ./internal/service/...` → зелёный

### Шаг 5: `internal/service/brand` (включая фикс security)

- [ ] `BrandService` +`logger`. `NewBrandService(pool, repoFactory, bcryptCost)` → `NewBrandService(..., logger)`.
- [ ] `AssignManager` строка 192: **заменить** `slog.Info("temporary password generated for new manager", "email", email)` на `s.logger.Info(ctx, "temporary password generated for new manager", "user_id", userRow.ID)`.
- [ ] Убрать импорт `"log/slog"` из `service/brand.go`. Добавить `logger`.
- [ ] Обновить `brand_test.go`:
  - Передавать `mockLogger`.
  - `AssignManager/new user` тест: `userRepo.Create` мокается с возвратом `userRow` с известным `ID`. `EXPECT().Info("temporary password generated for new manager", "user_id", "<expected-id>")`. Negative security: сохранить attrs через `.Run` в shared переменную `var loggedAttrs []any`, потому что внутри `.Run` мы ещё не знаем `tempPassword` (он генерится через `generateTempPassword()` — `crypto/rand`, не мокается). После возврата `_, tempPass, err := svc.AssignManager(...)` (`tempPass` — второй return value) проверить:
    - `args[2].([]any)` (variadic собран в slice) содержит ровно 2 элемента (`"user_id"`, `userRow.ID`) — `require.Len(t, args[2].([]any), 2)`
    - `str := fmt.Sprint(loggedAttrs...)` — `require.NotContains(t, str, email)` и `require.NotContains(t, str, tempPass)`
    - **Структура `args` в `.Run`:** для метода `Info(ctx, msg, args ...any)` всегда 3 элемента — `args[0]=ctx`, `args[1]=msg string`, `args[2]=[]any{...}`. Не путать «3 args mock-callback'а» с «3 attrs в логе».
  - В `AssignManager/existing user` — никаких EXPECT (лога нет, юзер уже существует).
- [ ] `cd backend && go test ./internal/service/...` → зелёный

### Шаг 6: `internal/handler` (NewServer, response, auth, audit, переименование TestHandler)

#### 6.1 Хелперы остаются свободными функциями + параметр `logger`

- [ ] `response.go`: добавить `logger logger.Logger` последним параметром в `respondJSON`, `respondError`, `encodeJSON`, `writeError` (все четыре). `slog.Error("failed to encode response", ...)` → `logger.Error(r.Context(), ...)`. `slog.Error("unexpected error", ...)` в default-branch → `logger.Error(r.Context(), ...)`. Убрать импорт `"log/slog"`.

#### 6.2 `Server` получает logger

- [ ] `Server` +`logger logger.Logger`. `NewServer(auth, brands, authz, audit, version, cookieSecure)` → `NewServer(..., logger)` (logger последним).
- [ ] `auth.go`: `slog.Error` в `Logout` (строка 81) и `RequestPasswordReset` (строка 116) → `s.logger.Error(r.Context(), ...)`. Все вызовы `respondJSON(w, r, ...)` → `respondJSON(w, r, ..., s.logger)`, аналогично `respondError(w, r, ..., s.logger)`. Убрать `"log/slog"`.
- [ ] `audit.go`: `rawJSONToAny(id, raw)` (свободная функция) → метод `(s *Server) rawJSONToAny(ctx context.Context, id string, raw []byte) interface{}`. Внутри `s.logger.Error(ctx, ...)`. В `ListAuditLogs` обновить call-site ×2 (для OldValue и NewValue): `s.rawJSONToAny(r.Context(), l.ID, l.OldValue)`. Все вызовы хелперов — добавить `s.logger`. Убрать `"log/slog"`.
- [ ] `brand.go`: вызовы хелперов — добавить `s.logger` последним параметром (slog в этом файле не было).

#### 6.3 Переименование TestHandler → TestAPIHandler

**Порядок важен: `git mv` ДО правок содержимого.** Так Git увидит rename, а не «удалил + создал», и blame/история сохранятся.

- [ ] `git mv backend/internal/handler/test.go backend/internal/handler/testapi.go`
- [ ] `git mv backend/internal/handler/test_test.go backend/internal/handler/testapi_test.go`
- [ ] В `testapi.go` (после `git mv`):
  - Структура `TestHandler` → `TestAPIHandler`, +`logger logger.Logger`
  - `NewTestHandler` → `NewTestAPIHandler`, +`logger` последним параметром
  - Интерфейс `TestAuthService` → `TestAPIAuthService`
  - Интерфейс `TestBrandService` → `TestAPIBrandService`
  - Интерфейс `TokenStore` остаётся (нет "test" в имени)
  - Все вызовы `respondJSON`/`respondError` — добавить `h.logger`
  - Receiver `(h *TestHandler)` → `(h *TestAPIHandler)` во всех методах
- [ ] В `testapi_test.go`:
  - Все `TestTestHandler_*` функции → `TestTestAPIHandler_*`
  - Все `mocks.NewMockTestAuthService` → `mocks.NewMockTestAPIAuthService`
  - Все `mocks.NewMockTestBrandService` → `mocks.NewMockTestAPIBrandService`
  - `NewTestHandler(auth, brands, store, seedAdminID)` → `NewTestAPIHandler(..., mockLogger)` (создавать `mockLogger` в каждом `t.Run`)
  - В `SeedUser/service error returns 500`: `mockLogger.EXPECT().Error("unexpected error", "error", mock.Anything, "path", "/test/seed-user").Once()`
  - В `SeedBrand/brand create error returns 500`: `mockLogger.EXPECT().Error("unexpected error", "error", mock.Anything, "path", "/test/seed-brand").Once()`
  - В `SeedBrand/assign manager error returns 500`: то же самое (`/test/seed-brand`)
- [ ] `cd backend && make generate-mocks` — mockery увидит интерфейсы с новыми именами и сгенерит новые мок-файлы. **Имя файла зависит от sprig snakecase** для `TestAPIAuthService` — может быть `mock_test_api_auth_service.go` (если `API` обрабатывается как 3 заглавные = граница) или `mock_testapi_auth_service.go`. Посмотреть фактическое имя через `ls backend/internal/handler/mocks/` после генерации, под него подогнать имя импорта в `testapi_test.go` (если отличается от ожиданий — в плане указан вариант `mock_test_api_*`).
- [ ] Удалить старые мок-файлы старых имён интерфейсов: `rm backend/internal/handler/mocks/mock_test_auth_service.go backend/internal/handler/mocks/mock_test_brand_service.go`. Mockery их не пересоздаст (старых интерфейсов больше нет), и удаление руками — единственный способ их убрать.
- [ ] `git status` — убедиться что в `mocks/` есть удалённый старый и созданный новый файл, без мусора.

#### 6.4 Обновить остальные handler-тесты

- [ ] `auth_test.go`: все `NewServer(auth, nil, nil, nil, "test-version", false)` → `NewServer(..., mockLogger)`. EXPECT'ы по таблице раздела 8.
- [ ] `brand_test.go`: все `NewServer(...)` → `NewServer(..., mockLogger)`; в кейсах service-error returns 500 → `EXPECT().Error("unexpected error", ...)`.
- [ ] `audit_test.go`: все `NewServer(...)` → `NewServer(..., mockLogger)`; в кейсах с битым JSON → `EXPECT().Error("failed to unmarshal audit log value", ...)`.
- [ ] Опционально: в `helpers_test.go` добавить хелпер `newServerWithMocks(t, auth, brands, authz, audit) (*Server, *logmocks.MockLogger)` чтобы уменьшить шум.

#### 6.5 Проверка

- [ ] `cd backend && go test ./internal/handler/...` → зелёный
- [ ] `make lint-backend` **не запускать** на этом шаге — он проходит по `./...` включая `cmd/api/main.go`, который ещё не починен (сигнатуры `NewServer`/`NewTestAPIHandler`/`closer.New`/middleware-фабрик изменились). Lint и полный build — после Шага 7.

### Шаг 7: финальная сборка `cmd/api/main.go`

- [ ] Добавить импорт в блок `import (...)`:
  ```go
  "github.com/alikhanmurzayev/ugcboost/backend/internal/logger"
  ```
  (`"log/slog"` остаётся — нужен для конструкции `*slog.Logger` и fall-back `slog.Error`).
- [ ] После `cfg, err := config.Load(); if err != nil { ... }`:
  ```go
  slogLogger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: cfg.LogLevel}))
  slog.SetDefault(slogLogger) // catch logs from third-party libraries
  appLogger := logger.New(slogLogger)
  ```
- [ ] Заменить все собственные `slog.Info/Warn/Error` в `run()` на `appLogger.*(ctx, ...)`. Конкретно:
  - Строка 62 `slog.Info("database connected")` → `appLogger.Info(ctx, "database connected")`
  - Строка 72 `slog.Info("cron scheduler started")` → `appLogger.Info(ctx, "cron scheduler started")`
  - Строка 132 `slog.Warn("TEST ENDPOINTS ENABLED ...")` → `appLogger.Warn(ctx, ...)`
  - Строка 150 `slog.Info("server starting", "port", cfg.Port)` → `appLogger.Info(ctx, ...)`
  - Строка 159 `slog.Info("shutting down", "signal", sig.String())` → `appLogger.Info(ctx, ...)`
- [ ] **Оставить** `slog.Error("fatal", "error", err)` в `main()` (строка 30) с комментарием:
  ```go
  // Last-resort fallback: appLogger may not be constructed yet if config.Load failed.
  slog.Error("fatal", "error", err)
  ```
- [ ] Прокинуть `appLogger` в конструкторы:
  - `service.NewAuthService(pool, repoFactory, tokenSvc, resetTokenStore, cfg.BcryptCost, appLogger)`
  - `service.NewBrandService(pool, repoFactory, cfg.BcryptCost, appLogger)`
  - `closer.New(appLogger)` (заменить `closer.New()`)
  - Middleware регистрация: `r.Use(middleware.Recovery(appLogger))`, `r.Use(middleware.Logging(appLogger))` (вместо `r.Use(middleware.Recovery)` и `r.Use(middleware.Logging)`)
  - `handler.NewServer(authSvc, brandSvc, authzSvc, auditSvc, cfg.Version, cfg.CookieSecure, appLogger)`
  - `handler.NewTestAPIHandler(authSvc, brandSvc, resetTokenStore, admin.ID, appLogger)` (заменить `handler.NewTestHandler(...)` — переименование из 4.2)
- [ ] `make build-backend && make lint-backend` → зелёные
- [ ] `make test-unit-backend` → весь backend зелёный (теперь включая main.go компилируется)

### Шаг 8: финальная валидация

- [ ] `cd backend && make generate-mocks` (на случай если что-то изменилось в интерфейсах)
- [ ] `cd backend && make build-backend`
- [ ] `cd backend && make lint-backend`
- [ ] `cd backend && make test-unit-backend`
- [ ] `cd backend && make test-unit-backend-coverage` — coverage gate ≥ 80% per-method на пакетах из whitelist Makefile (`handler`, `service`, `repository`, `middleware`, `authz`). `internal/logger` под gate не идёт — его покрытие проверяем отдельно (см. Шаг 1).
- [ ] Запустить инфру для E2E: `make compose-up` (или через `sg docker make compose-up`, если в текущей сессии группа docker не активна), `make migrate-up`
- [ ] `make test-e2e-backend` — должны пройти (бизнес-логика не менялась)
- [ ] **Grep-проверки:**
  ```
  grep -rE "slog\.(Info|Error|Warn|Debug)" backend/internal/    # должно быть пусто
  grep -rE "slog\.(Info|Error|Warn|Debug)" backend/cmd/         # допустимо ровно 1× (fall-back в main())
  grep -rE '"log/slog"' backend/internal/                        # допустимо: config/config.go, logger/logger.go
  grep -rE '"log/slog"' backend/cmd/                             # допустимо: main.go (для конструкции *slog.Logger)
  ```

### Шаг 9: оставить рабочий tree для ревью

- [ ] **НЕ коммитить.** Все изменения остаются в working tree.
- [ ] Сообщить Alikhan: «готово к ревью; коммит — за тобой».

## 10. Стратегия тестирования

### Unit-тесты (паттерны из `backend-testing-unit.md`)

- **Каждый изменённый сервис/хендлер/мидлвар:** обновить тесты под новую сигнатуру конструктора. `mockLogger := logmocks.NewMockLogger(t)` создаётся в каждом `t.Run` отдельно (новый мок на каждый сценарий, без протекания между тестами).
- **Каждый ожидаемый лог:** `MockLogger.EXPECT().Info|Warn|Error|Debug(args...).Once()` с точным сообщением и точными ключами. Динамические значения через `mock.Anything`. Конкретные значения через `.Run(func(args mock.Arguments) { ... })` + `require`.
- **Static checks через mockery strict mode:** если в реальном коде вызвался лог, для которого нет `EXPECT()` — `t.Cleanup(AssertExpectations)` уронит тест. Это автоматическая негативная проверка для всех сценариев без EXPECT.
- **Negative tests на security:**
  - `RequestPasswordReset` — нет raw token в args.
  - `AssignManager` — нет email, нет tempPassword (после фикса п.4).
  - `Logging` middleware — нет Authorization header, нет cookie value.
- **Coverage gate:** `make test-unit-backend-coverage`. Per-method ≥ 80% на `handler`, `service`, `repository`, `middleware`, `authz` (whitelist в Makefile через awk-фильтр). Новый пакет `internal/logger` **не входит** в этот whitelist — gate его не проверяет. Покрытие ≥ 80% на logger обеспечивается ручными тестами (см. Шаг 1) ради качества, не gate'а.
- **Race detector:** `-race` уже включён в make-таргетах. Обёртка над `*slog.Logger` не должна добавлять race-условий (`*slog.Logger` thread-safe).
- **`t.Parallel()`** в каждом тесте и каждом `t.Run` — стандартное требование проекта.

### E2E-тесты

- **Без новых E2E-тестов.** Бизнес-логика не меняется; существующие `backend/e2e/` должны пройти после рефакторинга — это главный регрессионный гейт.
- Если E2E падает — это ошибка в DI-проводке (один из конструкторов не получил logger) или потерянный SetDefault.

### Линтинг

- `make lint-backend` (golangci-lint) должен пройти без warnings. Обратить внимание:
  - `unused`/`staticcheck`: старые импорты `"log/slog"` нужно убрать из всех затронутых файлов.
  - `errcheck`: новые методы `s.logger.*` не возвращают ошибок (как и `slog.*`), warnings не должно быть.
  - `revive`/`stylecheck`: имена `Logger`, `SlogLogger`, `New` соответствуют `naming.md`.

## 11. Оценка рисков

| Риск | Вероятность | Митигация |
|---|---|---|
| Переименование `TestHandler` → `TestAPIHandler` затрагивает много файлов | Низкая | Все ссылки локальны: `test.go`/`test_test.go` (переименовываем), `cmd/api/main.go` (1 место), сгенерённые моки (регенерятся через `make generate-mocks`). `git mv` сохраняет историю |
| Snake_case у `TestAPIAuthService` в имени мок-файла | Низкая | Проверить фактическое имя через `ls mocks/` после первой генерации (Шаг 6.3) — может оказаться `mock_test_api_auth_service.go` или `mock_testapi_auth_service.go`; под фактическое подогнать импорты |
| Покрытие тестами `internal/logger` < 80% | Низкая | Простые методы (proxy в `*slog.Logger`) — короткие тесты покроют 100%. Gate этот пакет НЕ проверяет, но контроль ручной (Шаг 1: `go tool cover -func`) |
| Hot-path overhead `Logging` middleware | Низкая | На MVP-нагрузке несущественно. Если потом упрёмся — точечная оптимизация |
| Хрупкость текстов логов | Низкая (осознанно) | Принято: тесты чинятся вместе с кодом |
| mockery `all: true` сгенерит лишние моки если в `logger.go` появится ещё интерфейс | Низкая | Держать в `logger.go` только `Logger` + `SlogLogger` + `New` |
| `make generate-mocks` тронет неожиданные мок-файлы (изменилась версия mockery) | Низкая | После генерации (Шаги 1, 6.3, 8) — `git status`, просмотр diff'ов, отказ если затронуто что-то не относящееся к задаче |
| E2E падёт из-за ошибки в DI-проводке | Средняя | Шаг 7 проходит через `make build-backend && make lint-backend` (компилятор поймает несовпадение сигнатур); E2E в Шаге 8 — последний gate |
| Промежуточно красная компиляция `main.go` между Шагами 2 и 7 | Известно | Преамбула раздела 9 явно описывает; на промежуточных шагах используем `go test ./internal/<package>/...` |
| `slog.SetDefault` НЕ перехватывает `http.Server.ErrorLog` (stdlib uses `log.Logger`, не `slog`) | Низкая | Для перехвата нужен отдельный `slog.NewLogLogger(slogLogger.Handler(), slog.LevelError)` присвоенный в `srv.ErrorLog`. **В этой задаче не делаем** (вне скоупа), но фиксируем как known limitation — сетевые ошибки HTTP-сервера могут не попасть в JSON-формат |
| Объём изменений (~60 файлов) | Средняя | Шаги 1-7 атомарны (тесты затронутого пакета зелёные). Согласно памяти `feedback_finish_tasks.md` — выполнить за одну сессию без перерывов |
| Параллельная работа в этих файлах → merge conflict | Низкая | Текущая ветка `alikhan/staging-cicd` тыловая; коммит делает Alikhan сразу после ревью |
| Шум от `mockLogger` в 18+ местах `auth_test.go` | Низкая (косметика) | Хелпер `newServerWithMocks(t, auth, brands, authz, audit)` в `helpers_test.go` |

### Закрытые вопросы (для истории)

- **Хелперы `respondError`/`respondJSON` для `Server` и `TestAPIHandler`** — закрыто: остаются свободными функциями, принимают `logger` параметром, `TestAPIHandler` получает logger через DI.
- **Fall-back `slog.Error("fatal", ...)` в `main()`** — закрыто: оставляем 1 вызов с комментарием.
- **Замена самописного `closer` на библиотеку** — закрыто: оставляем самописным.

## 12. План отката

- **До коммита:** все изменения в working tree. Откат — `git restore <files>` или `git stash` для частичного отката.
- **Шаг сломал тесты и не чинится за 5 минут:** `git restore -- backend/internal/<package>` для отката изменений в проблемном пакете; пересмотреть подход в этом шаге.
- **Если уже закоммичено и оказалось проблемой в проде** (низкая вероятность — ревью + E2E + staging до прода):
  - Бизнес-логика не менялась → revert безопасен.
  - `git revert <commit-sha>` восстановит глобальный `slog`. Никаких миграций БД, никакого state — чистый revert.
- **Частичный откат под флагом** — не нужен. Изменения чисто структурные, без runtime-конфигурации.

---

**Все открытые вопросы закрыты. План готов к реализации (`/build`).**
