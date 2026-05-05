# Backlog: рефакторинг логирования backend через DI

**Статус:** запланировано, отложено (не входит в текущую задачу по unit-тестам).
**Инициатор:** Alikhan.
**Дата фиксации:** 2026-04-19.

Артефакт-заметка с полной постановкой задачи. Когда придёт время — стартовать через `/scout` и `/plan`, использовать этот документ как вход.

## Мотивация

Сейчас в backend логирование через глобальный пакет `log/slog` (прямые вызовы `slog.Info/Error/...` по всему коду). Это:

1. **Не позволяет unit-тестировать логи.** Нельзя замокать, нельзя проверить что конкретный лог был вызван с корректным уровнем и payload'ом. Alikhan рассматривает логирование как бизнес-логику, а не побочный эффект — каждый лог должен тестироваться, потому что:
   - Уровень логирования (Info/Warn/Error/Debug) выбирается осознанно, регрессия меняет операционную картину
   - Контент лога — критичная операционная информация, особенно для observability в продакшене
   - Security: негативные тесты (пароль/токен/секрет не попал в лог) должны быть автоматизированы
   - Хрупкость текстов при рефакторинге — приемлема, тесты поправятся вместе с кодом
2. **Нарушает `backend-design.md`.** Все зависимости в проекте инжектятся через конструктор, кроме логгера — это непоследовательно.
3. **Не даёт переопределять поведение per-request.** В будущем хочется, например, включать `DEBUG` для конкретного запроса через заголовок (для дебага прод-инцидента без релиза) — с глобальным `slog` это нереализуемо.

## Целевая архитектура

### Пакет `internal/logger` (новый)

**Структура:** экспортируемая структура `Logger`, обёртка над `*slog.Logger`. Методы принимают `context.Context` первым параметром — **даже если внутри контекст не используется сейчас** (placeholder `_`). Это инвестиция в будущее расширение без повторного breaking change API.

**Интерфейс:** `Logger interface` лежит **рядом** в том же пакете (исключение из правила «interfaces in consumer package» — здесь интерфейс семантически принадлежит пакету-логгеру, используется всеми слоями без конфликта имён).

Набросок (не окончательный, подлежит уточнению на `/scout`/`/plan`):

```go
package logger

import (
    "context"
    "log/slog"
)

// Logger is the single logging abstraction used across all backend layers.
// All implementations must accept context to support future per-request
// overrides (trace IDs, dynamic log level, user context, etc.).
type Logger interface {
    Debug(ctx context.Context, msg string, args ...any)
    Info(ctx context.Context, msg string, args ...any)
    Warn(ctx context.Context, msg string, args ...any)
    Error(ctx context.Context, msg string, args ...any)
    With(args ...any) Logger // для логгеров с preset-атрибутами
}

type SlogLogger struct {
    inner *slog.Logger
}

func New(inner *slog.Logger) *SlogLogger { return &SlogLogger{inner: inner} }

func (l *SlogLogger) Info(_ context.Context, msg string, args ...any) {
    // Пока ctx игнорируется. В будущем:
    //  - достать trace ID / request ID из ctx и добавить в attrs
    //  - прочитать per-request log-level override из ctx
    //  - обогатить user/role если есть в ctx
    l.inner.Info(msg, args...)
}
// ...аналогично Debug/Warn/Error/With
```

### DI — во все слои

Логгер создаётся в `main.go` один раз и передаётся во все конструкторы:

- `handler.NewServer(auth, brands, authz, audit, cookieSecure, logger)`
- `service.NewAuthService(pool, factory, tokens, notifier, bcryptCost, logger)`
- `service.NewBrandService(pool, factory, bcryptCost, logger)`
- `service.NewAuditService(pool, factory, logger)`
- `middleware.Auth(validator, logger)`, `middleware.Logging(logger)`, `middleware.Recovery(logger)`, ...
- `repository/*` — по мере необходимости (обычно repo мало логирует, но если есть — тоже через DI)
- `handler/TestHandler` — тоже через DI, чтобы тест-ручки тестировались

### Моки

- `mockery` с `all: true` уже генерирует моки — интерфейс `logger.Logger` попадёт автоматически
- В тестах использовать `logger.NewMockLogger(t)` с `EXPECT().Info(mock.Anything, "ожидаемое сообщение", "key", "value", ...)`
- Точные аргументы — по стандарту `backend-testing-unit.md`

## Политика тестирования логов

**Логи — часть бизнес-контракта, проверяются наравне с return-значениями.**

Для каждого публичного метода, который логирует:
- В сценариях, где лог ожидается — мок **должен** ожидать точный вызов (`.EXPECT().Info(...)`). Если лог не произошёл — тест падает (mockery strict mode).
- В сценариях, где лог **не должен** происходить (например, security: пароль/токен не утёк в Info) — добавить негативную проверку через `.Times(0)` или `.NotCalled()`.
- Уровень лога проверяется: `Info` vs `Warn` vs `Error` — отдельные методы интерфейса, опечатка = ошибка компиляции.
- Attrs (key/value пары) проверяются полностью: `mock.MatchedBy` с частичкой запрещён, полные args в `EXPECT()`.

**Хрупкость текстов.** Alikhan принимает осознанно: если формулировка меняется, тест чинится вместе с кодом. Это цена за уверенность, что логи идут корректные.

## Зоны, которые рефакторинг затронет

Точная инвентаризация — на этапе `/scout`, но ориентировочно:

1. **`main.go`** — создание `logger.Logger`, передача во все конструкторы
2. **`internal/service/*.go`** — убрать `slog.Info/Error/...`, заменить на `s.logger.Info(ctx, ...)`; добавить `logger` в структуру и конструктор
3. **`internal/handler/*.go`** — аналогично; `rawJSONToAny` сейчас логирует через глобальный `slog`, перевести на `h.logger.Error(ctx, ...)` (где `h` — `*Server` с полем `logger`)
4. **`internal/middleware/*.go`** — `logging.go` особенно критичен; `recovery.go`; все middleware, которые логируют
5. **`internal/repository/*.go`** — только если где-то используется логирование (сейчас вроде нет)
6. **`internal/config/*`** — стартовые логи конфигурации
7. **Тесты** — все существующие `*_test.go` обновить: инжектить `MockLogger`, добавить `.EXPECT()` для каждого лога. Новые тесты сразу писать с mock-логгером

Грубая оценка: ~30-40 файлов прод-кода + все `*_test.go`. Работа значительная, делать отдельной PR после стабилизации текущего рефакторинга.

## Что ПОКА не делаем

- Не извлекать `logger` в отдельный shared-package (остаётся внутри backend)
- Не добавлять сложные фичи (sampling, rate limiting, multi-handler fan-out) — это при первой необходимости
- Не менять формат вывода (JSON / text) — оставить как есть в slog handler
- Не интегрировать OpenTelemetry — отдельная задача, но интерфейс с `ctx` позволит добавить trace-корреляцию без изменения API

## Будущие расширения (явно предусмотрены в API)

- **Trace ID / Request ID:** middleware добавляет в `ctx`, `SlogLogger.Info(ctx, ...)` читает и добавляет как attr
- **Per-request log level override:** заголовок `X-Log-Level: debug` → middleware кладёт override в `ctx` → `SlogLogger` учитывает при фильтрации
- **User context enrichment:** если `middleware.UserIDFromContext(ctx) != ""` — автоматически добавлять `user_id` attr
- **Dynamic sampling:** на горячих путях (healthz) — dropping с учётом ctx

## Критерии готовности рефакторинга (для будущей задачи)

1. Глобальный `slog.*` вызов остался **только** в `main.go` (для startup-логов до инициализации DI)
2. Во всех остальных пакетах `grep -r "slog\.Info\|slog\.Error\|slog\.Warn\|slog\.Debug" backend/internal/` — пусто
3. Каждый конструктор сервиса/хендлера/мидлвара принимает `logger.Logger`
4. mockery генерит `internal/logger/mocks/mock_logger.go`
5. Все существующие unit-тесты обновлены и проходят с `MockLogger.EXPECT()` для каждого лога
6. `make test-unit-backend`, `lint-backend`, `test-e2e-backend` зелёные
7. Coverage ≥ 80% на новом пакете `internal/logger`

## Связанные стандарты

- `backend-design.md` — «Зависимости через конструктор» (обоснование DI)
- `backend-libraries.md` — «Инфраструктурный код — библиотеки» (под капотом slog остаётся, своя только тонкая обёртка ради тестируемости и ctx)
- `backend-testing-unit.md` — «Точные аргументы моков», «Порядок вызовов проверяется» (применяется к mock-логгеру)
- `security.md` — «Логирование без чувствительных данных» (негативные тесты — пароли/токены не утекают)
- `naming.md` — именование пакета (`logger`), структуры (`SlogLogger` или `Logger`), интерфейса (`Logger`)

## Процесс запуска рефакторинга

Когда будем готовы стартовать:

1. `/scout` с аргументом: «рефакторинг логирования через DI по `@_bmad-output/planning-artifacts/logger-refactor-brief.md`»
2. После scout — `/plan`
3. После одобрения плана — `/build`
4. Отдельной PR, мержить только после полной стабилизации unit-тестового рефакторинга (чтобы не конфликтовать в `*_test.go` массово)
