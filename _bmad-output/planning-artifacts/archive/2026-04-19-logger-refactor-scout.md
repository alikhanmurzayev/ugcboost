---
title: Разведка — рефакторинг логирования через DI
type: scout-report
inputs:
  - _bmad-output/planning-artifacts/logger-refactor-brief.md
date: 2026-04-19
owner: Alikhan
status: approved, ready for /plan
---

# Разведка — рефакторинг логирования backend через DI

Документ — вход для `/plan`. Бриф (`logger-refactor-brief.md`) задаёт мотивацию и целевую архитектуру; здесь — точная карта затронутых мест в текущем коде, паттерны проекта, риски, согласованные решения и порядок работ.

## 0. Изменения относительно брифа (согласованы с Alikhan)

| # | Бриф | Уточнение / изменение | Обоснование |
|---|---|---|---|
| Δ1 | «Глобальный `slog.*` вызов остался **только** в `main.go`» | Использовать обёртку `logger.Logger` **везде, включая `main.go`**. Голый `slog.*` (Info/Error/Warn/Debug) не остаётся нигде в `internal/` и `cmd/`. В `main.go` остаются только конструкции (`slog.New(...)`, опционально `slog.SetDefault(...)`) для создания `*slog.Logger`, передаваемого в `logger.New(...)` | Единообразие, готовность к будущим расширениям (trace ID и т.д.) и в startup-логах |
| Δ2 | «Конструкторы `service.NewAuditService`, `authz.NewAuthzService`, `handler.NewTestHandler` принимают `logger`» | **НЕ принимают** — внутри сейчас нет `slog.*` вызовов | Стандарт «не делать впрок»; добавим при первой реальной необходимости |
| Δ3 | (не упоминалось) | Заменить лог в `service/brand.go:192` `"email", email` на `"user_id", userRow.ID` | Email — PII, в связке с фактом «временный пароль создан» нарушает `security.md`. UUID уже доступен после `Create`. Email и так уйдёт в `audit_logs` через `writeAudit` |
| Δ4 | `closer` модифицируется через DI | **Сохраняем самописным**, без замены на библиотеку, но через DI (`closer.New(logger)`). В `backend-libraries.md` теоретически противоречит, но канонической Go-библиотеки под наш кейс (LIFO + именованные ресурсы) нет; код 60 строк | Не размывать скоп текущей задачи |
| Δ5 | Тип `slog.Level` в `internal/config/config.go` | **Оставляем**. Это тип env-поля `LogLevel slog.Level`, не вызов | Не нарушает критерий «нет вызовов `slog.*`» |

## 1. Затронутые области

### 1.1 Места прямых вызовов `slog.*` (14 вызовов в 8 файлах внутри `internal/` + 5 вызовов в `cmd/api/main.go`)

| Файл | Метод/функция | Вызовы | Критичность |
|---|---|---|---|
| `internal/handler/response.go:19` | `encodeJSON` | `slog.Error("failed to encode response", ...)` | низкая (общий хелпер) |
| `internal/handler/response.go:59` | `respondError` (default) | `slog.Error("unexpected error", ...)` | средняя (catch-all для 500) |
| `internal/handler/audit.go:89` | `rawJSONToAny` (свободная функция!) | `slog.Error("failed to unmarshal audit log value", ...)` | средняя (free func — нужно сделать методом `*Server`) |
| `internal/handler/auth.go:81` | `(*Server).Logout` | `slog.Error("failed to revoke refresh tokens on logout", ...)` | высокая (security path) |
| `internal/handler/auth.go:116` | `(*Server).RequestPasswordReset` | `slog.Error("password reset request failed", ...)` | высокая (security path, антиэнумерация) |
| `internal/service/auth.go:209` | `(*AuthService).RequestPasswordReset` | `slog.Info("password reset token generated", "user_id", ...)` | критичная (содержит user_id — security audit) |
| `internal/service/auth.go:300,311,325` | `(*AuthService).SeedAdmin` | 3× `slog.Info(...)` (skipped/exists/created) | средняя (startup) |
| `internal/service/brand.go:192` | `(*BrandService).AssignManager` | `slog.Info("temporary password generated for new manager", "email", ...)` | **Δ3: заменить `email` → `user_id` (`userRow.ID`)** |
| `internal/middleware/recovery.go:18` | `Recovery` | `slog.Error("panic recovered", ...)` | критичная (operational visibility) |
| `internal/middleware/logging.go:27` | `Logging` | `slog.Info("http request", ...)` | высокая (per-request) |
| `internal/closer/closer.go:47,49` | `(*Closer).Close` | `slog.Info("shutting down", ...)`, `slog.Error("shutdown error", ...)` | средняя (graceful shutdown) |
| `cmd/api/main.go:30,62,72,132,150,159` | `main`/`run` | 5× `slog.Info/Warn/Error` для startup и shutdown | **Δ1: заменить на `appLogger.*`** |

### 1.2 Использование `log/slog` без вызовов (можно оставить)

- `internal/config/config.go:6,28` — только тип `slog.Level` для `LogLevel slog.Level` env-поля. **Δ5**: оставляем.
- `cmd/api/main.go` — только конструкция `slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: cfg.LogLevel}))` для входа в `logger.New(...)`. Опционально `slog.SetDefault(...)` чтобы перехватить логи сторонних библиотек, если такие появятся.

### 1.3 Конструкторы, которые получат `logger.Logger`

| Конструктор | Аргументы сейчас | Изменение |
|---|---|---|
| `handler.NewServer` | `auth, brands, authz, audit, version, cookieSecure` | +`logger` (логирует в auth.go, audit.go, response.go) |
| `service.NewAuthService` | `pool, repoFactory, tokens, resetNotifier, bcryptCost` | +`logger` |
| `service.NewBrandService` | `pool, repoFactory, bcryptCost` | +`logger` |
| `service.NewAuditService` | `pool, repoFactory` | **Δ2: НЕ изменяется** (нет `slog.*` внутри) |
| `authz.NewAuthzService` | `brandService` | **Δ2: НЕ изменяется** |
| `closer.New` | без аргументов | +`logger` (Δ4) |
| `middleware.Recovery` | сейчас прямо `func(http.Handler) http.Handler` | превратить в фабрику `func Recovery(logger logger.Logger) func(http.Handler) http.Handler` (паттерн как у `Auth(validator)`, `CORS(origins)`) |
| `middleware.Logging` | сейчас прямо `func(http.Handler) http.Handler` | то же — фабрика `func Logging(logger logger.Logger)` |
| `handler.NewTestHandler` | `auth, brands, tokenStore, adminID` | **Δ2: НЕ изменяется** (нет `slog.*` внутри) |

### 1.4 Тесты, которые нужно обновить

Каждый файл, конструирующий один из изменённых объектов:

- `internal/handler/auth_test.go` — `NewServer(...)` × 18+ вызовов; ⚠️ кейсы `Logout/service error` и `RequestPasswordReset/always returns 200 even when service logs error` по новой политике должны иметь `MockLogger.EXPECT().Error(...)`
- `internal/handler/brand_test.go` — `NewServer(...)` много раз
- `internal/handler/audit_test.go` — `NewServer(...)` + кейсы с битым JSON (`rawJSONToAny` логирует)
- `internal/handler/test_test.go` — без изменений (Δ2)
- `internal/handler/helpers_test.go` — `newTestRouter(t, *Server)` хелпер; через него передаётся `Server` уже сконструированный
- `internal/service/auth_test.go` — `NewAuthService(...)`; ⚠️ `RequestPasswordReset` success тест должен ожидать `Info("password reset token generated", "user_id", ...)`; `SeedAdmin` тесты — три сценария с `Info`
- `internal/service/brand_test.go` — `NewBrandService(...)`; ⚠️ `AssignManager` (новый user) ожидает `Info("temporary password generated...", "user_id", userRow.ID)` — обновить под Δ3
- `internal/service/audit_test.go` — без изменений (Δ2)
- `internal/middleware/recovery_test.go` — middleware теперь фабрика, ожидать `Error` при panic
- `internal/middleware/logging_test.go` — **этого файла нет!** Логирование middleware сейчас не покрыто тестами; после рефакторинга должны появиться как минимум: success log, log не содержит body/Authorization (security негативный тест)
- `internal/closer/closer_test.go` — `New(logger)`; ожидать `Info("shutting down", "resource", X)` для каждой Add в LIFO порядке + `Error("shutdown error", ...)` в кейсах с ошибками
- `cmd/api/main.go` — НЕ тестируется (исключено из coverage gate), нужно только обновить вызовы

### 1.5 Новые файлы

- `internal/logger/logger.go` — интерфейс `Logger` + структура `SlogLogger` + `New(*slog.Logger) *SlogLogger`
- `internal/logger/logger_test.go` — unit-тесты обёртки (правильный proxy на `*slog.Logger`, корректное игнорирование `ctx` пока)
- `internal/logger/mocks/mock_logger.go` — генерируется mockery (после добавления пакета в `.mockery.yaml`)

### 1.6 `.mockery.yaml`

Добавить:
```yaml
github.com/alikhanmurzayev/ugcboost/backend/internal/logger:
```

## 2. Паттерны реализации проекта

### 2.1 DI через конструктор (`docs/standards/backend-design.md`)

Все зависимости — через `New*()`. Структура иммутабельна. Никаких setter'ов. Проект уже последователен — `slog` единственное исключение.

### 2.2 Интерфейсы — у консумера

В коде интерфейсы (`AuthService`, `BrandService`, `AuthzService`, `TokenValidator`) лежат в пакете-консумере. Бриф даёт явное исключение: `Logger` живёт в `internal/logger` — оправдано, потому что иначе один интерфейс пришлось бы дублировать в каждом пакете.

### 2.3 Middleware-фабрики

`middleware.Auth(validator)`, `middleware.CORS(origins)`, `middleware.BodyLimit(n)` — все принимают зависимости/конфиг и возвращают `func(http.Handler) http.Handler`. `Recovery` и `Logging` — единственные middleware без зависимостей, поэтому реализованы прямо как `func(http.Handler) http.Handler`. После рефакторинга станут фабриками — единый паттерн.

### 2.4 Тестирование (`docs/standards/backend-testing-unit.md`)

- `mockery` (template: testify) + `all: true` — моки автогенерятся
- В каждом `t.Run`: `mock := mocks.NewMockX(t)` — strict mode, `t.Cleanup` авто-asserts
- Точные аргументы: `EXPECT().Method(mock.Anything, "exact", "value")`
- `t.Parallel()` везде

### 2.5 Тест-роутер handler-слоя

`newTestRouter(t, s *Server)` в `helpers_test.go` строит chi-роутер БЕЗ middleware Logging/Recovery — это удобно: handler-тесты не вынуждены ожидать `logger.Info("http request", ...)` на каждый вызов.

### 2.6 Уже принятый стандарт «не делать впрок»

Из CLAUDE.md и стандарта `backend-design.md`: «Don't add features beyond what the task requires». Влияет на Δ2 — `AuditService`/`AuthzService`/`TestHandler` не получают logger.

## 3. Риски и соображения

### 3.1 Hot-path overhead — `Logging` middleware

`Logging` вызывается на каждый HTTP-запрос. Через интерфейс — лишний indirect-call + аллокация при `args ...any`. На MVP-нагрузке несущественно, но фиксируем — если потом упрёмся в перформанс, точечно сможем оптимизировать.

### 3.2 «Точные args» в `EXPECT` для динамических полей

Бриф запрещает `mock.MatchedBy` для частичной проверки. Но логи реально содержат:
- `recovery.go`: `"stack", string(debug.Stack())` — недетерминированный stack trace
- `closer.go`: `"error", err` — может быть произвольная error
- `service/auth.go RequestPasswordReset`: `"expires_at", expiresAt` — `time.Time`, динамика
- `middleware/logging.go`: `"duration_ms", time.Since(start).Milliseconds()`, `"remote_addr", r.RemoteAddr`

Подходы:
1. Жёсткий: `EXPECT().Info(mock.Anything, "msg", "k1", v1, "k2", mock.Anything, ...)` — точные ключи, `mock.Anything` для динамических значений. Это **разрешено** стандартом (`mock.AnythingOfType("...")` не запрещён — запрещено `MatchedBy` с partial-логикой).
2. Через `.Run(func(args mock.Arguments) { ... })` — достать аргумент, проверить тип/диапазон через `require`. Уже принятый паттерн (`expectAudit` в `service/auth_test.go`).

**Решение:** комбинация — `.Run(...)` для значений, которые нужно проверить (например, что `user_id` именно ожидаемый), `mock.Anything` для значений, которые проверять смысла нет (stack trace, duration).

### 3.3 Negative tests — security

Бриф явно требует негативные тесты: «пароль/токен/секрет не утёк в Info». Конкретно:
- `service/auth.go RequestPasswordReset`: убедиться, что `raw` (raw reset token) **не передаётся** в `Info`. Сейчас не передаётся (только `user_id`, `expires_at`) — фиксируем тестом, чтобы регрессия не прошла.
- `service/brand.go AssignManager` (после Δ3): убедиться, что ни `tempPassword`, ни `email` не утекают в лог. После замены на `user_id` оба исчезают.
- `middleware/logging.go`: убедиться, что Authorization header / cookie не попадают в лог.

### 3.4 `rawJSONToAny` — свободная функция в `audit.go`

```go
func rawJSONToAny(id string, raw []byte) interface{} {
    ...
    slog.Error("failed to unmarshal audit log value", ...)
}
```

Чтобы убрать `slog`, делаем методом `*Server`: `func (s *Server) rawJSONToAny(...)` — естественный доступ к `s.logger`.

### 3.5 Coverage gate (≥ 80% per-method)

Новый пакет `internal/logger` подпадает под gate (он не в исключениях). Нужно покрыть `SlogLogger.{Debug,Info,Warn,Error,With}` тестами. Простые методы — короткие тесты, проблем не будет.

### 3.6 `.mockery.yaml` — `all: true` + новый пакет

Mockery генерит моки для **всех** интерфейсов в перечисленных пакетах. После добавления `internal/logger` сгенерится `MockLogger`. Если в `logger.go` лежит ещё что-то, что выглядит как интерфейс — попадёт в моки. Поэтому в пакете `logger` держим **только** `Logger` interface + конкретную реализацию + `New`.

### 3.7 Хрупкость текстов логов

Бриф принимает осознанно. Если кто-то редактирует текст лога без обновления теста — компиляция пройдёт, упадёт `go test`. Это нормально для проекта где «логи = бизнес-контракт».

### 3.8 Объём изменений

~30 файлов прод-кода + ~30 файлов тестов. Один коммит в текущей ветке `alikhan/staging-cicd`, без отдельного PR.

## 4. План реализации

### 4.1 Порядок работ (вертикальный слайс — пакет за пакетом)

1. **`internal/logger`** — пакет, mockery-конфиг, unit-тесты обёртки. ≥ 80% coverage.
2. **`internal/closer`** — самый изолированный. Обновить `New(logger)`, `closer_test.go`. Проверить `make test-unit-backend`.
3. **`internal/middleware/recovery`** + **`internal/middleware/logging`** — превратить в фабрики. Обновить `recovery_test.go`, **создать** `logging_test.go`. Обновить вызовы в `cmd/api/main.go`.
4. **`internal/service/auth`** — добавить logger в конструктор. Обновить `auth_test.go`.
5. **`internal/service/brand`** — добавить logger; **применить Δ3** (email → user_id) и обновить тест `AssignManager`.
6. **`internal/handler`** — `NewServer` + `logger`. Сделать `rawJSONToAny` методом. Перевести `response.go`, `auth.go`, `audit.go`. Обновить `auth_test.go`, `brand_test.go`, `audit_test.go`. Кейсы с ожидаемым логом получают `EXPECT()`.
7. **`cmd/api/main.go`** — собрать `appLogger := logger.New(slog.New(...))`, прокинуть во все конструкторы, **заменить все собственные `slog.*` вызовы на `appLogger.*` (Δ1)**.
8. **Финальная валидация:** `make build-backend && make lint-backend && make test-unit-backend && make test-unit-backend-coverage && make test-e2e-backend`.
9. **Grep-проверка:**
   - `grep -rE "slog\.(Info|Error|Warn|Debug)" backend/` — должно быть **пусто** (Δ1)
   - `grep -rE "log/slog" backend/internal/` — допустимо только в `config/config.go` (тип `slog.Level`) и `logger/logger.go` (импорт обёртки) (Δ5)
   - `grep -rE "log/slog" backend/cmd/` — допустимо только в `main.go` (для конструкции `*slog.Logger`)
10. **Коммит делает Alikhan вручную** после ревью.

### 4.2 Подтверждённые решения (закрыто)

| # | Решение | Принято |
|---|---|---|
| D-1 | `closer.New(logger)` — единый стиль | ✅ |
| D-2 | `NewTestHandler`, `NewAuditService`, `NewAuthzService` НЕ принимают logger (нет `slog.*` внутри) | ✅ |
| D-5 | `With(args ...any) Logger` остаётся в интерфейсе | ✅ |
| D-6 | `service/brand.go:192`: `email` → `user_id` (фиксим в этом же рефакторинге, Δ3) | ✅ |
| D-7 | Обёртка используется **везде** в проекте, кроме самой обёртки. В `main.go` тоже через `appLogger.*`, голых `slog.*` вызовов нет (Δ1) | ✅ |
| D-8 | `context.Context` первым параметром во всех методах интерфейса (placeholder сейчас, наполнение позже) | ✅ |
| extra | Closer оставляем самописным, в backlog не выносим (Δ4) | ✅ |
| extra | Один коммит в ветке `alikhan/staging-cicd`. Коммит делает Alikhan, не Claude | ✅ |

### 4.3 Альтернативный подход — отвергнут

**Использовать готовую обёртку (zerolog / zap / slog-multi).** Не подходит: проект уже в `slog`, нужна тонкая адаптация для DI и `ctx`-параметра — это 30 строк кода, библиотека добавит только зависимость.

## 5. Что выносим за скоп этой задачи

- Реальное использование `ctx` (trace ID, per-request override) — интерфейс готов, наполнение позже отдельной задачей
- OpenTelemetry / structured tracing — отдельно
- Изменение формата вывода (JSON vs text) — оставляем как есть
- Sampling, rate-limiting, multi-handler — нет необходимости
- Изменение конфигурации `LogLevel` (env var) — без изменений
- Замена самописного `closer` на библиотеку (Δ4) — отвергнуто; пересмотр откладывается до момента, когда появится настоящая канонная Go-библиотека под наш кейс

## 6. Критерии готовности (обновлены относительно брифа)

1. **`grep -rE "slog\.(Info|Error|Warn|Debug)" backend/` — пусто** (заменяет п.1 и п.2 брифа; Δ1)
2. Каждый затронутый конструктор сервиса/хендлера/мидлвара принимает `logger.Logger` (см. таблицу в 1.3 — без `AuditService`, `AuthzService`, `TestHandler`)
3. mockery генерит `internal/logger/mocks/mock_logger.go`
4. Все существующие unit-тесты обновлены и проходят с `MockLogger.EXPECT()` для каждого лога
5. `make test-unit-backend`, `lint-backend`, `test-e2e-backend` зелёные
6. `make test-unit-backend-coverage` зелёный (per-method ≥ 80% на изменённых пакетах; новый пакет `internal/logger` не в скопе coverage gate, но покрыт ≥ 80% тестами своими силами)
7. `service/brand.go:192` логирует `user_id`, не `email` (Δ3); тест проверяет
8. `cmd/api/main.go` использует `appLogger.*`, не `slog.*` (Δ1)

---

Следующий шаг — `/plan`.
