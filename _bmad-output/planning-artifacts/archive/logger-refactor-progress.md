---
title: Прогресс — рефакторинг логирования backend через DI
type: implementation-progress
date: 2026-04-19
status: complete, ready for review
---

# Прогресс: рефакторинг логирования backend через DI

## Выполнено
- [x] Шаг 0: загрузка стандартов + код — 2026-04-19
- [x] Шаг 1: пакет `internal/logger` (Logger, SlogLogger, mocks, 100% coverage) — 2026-04-19
- [x] Шаг 2: `internal/closer` — DI logger, тесты обновлены — 2026-04-19
- [x] Шаг 3: middleware Recovery/Logging стали фабриками + новый `logging_test.go` с проверкой на отсутствие Authorization/Cookie — 2026-04-19
- [x] Шаг 4: `service/auth` — DI logger, EXPECT'ы на Info-логи, защита raw-токена — 2026-04-19
- [x] Шаг 5: `service/brand` — DI logger + security fix (email → user_id), negative-тест на PII — 2026-04-19
- [x] Шаг 6: handler — все четыре хелпера (respondJSON/respondError/encodeJSON/writeError) принимают logger; переименование `TestHandler` → `TestAPIHandler` (git mv + регенерация моков); `rawJSONToAny` стал методом Server — 2026-04-19
- [x] Шаг 7: `cmd/api/main.go` собран заново с appLogger (1 fall-back `slog.Error` в `main()`) — 2026-04-19
- [x] Шаг 8: финальная валидация — 2026-04-19

## Результаты валидации

| Проверка | Результат |
|---|---|
| `make build-backend` | ✅ |
| `make lint-backend` | ✅ 0 issues |
| `make test-unit-backend` | ✅ все пакеты зелёные |
| `make test-unit-backend-coverage` | ✅ gate passed |
| `make test-e2e-backend` | ✅ audit + auth + brand suites зелёные |
| `grep slog\. backend/internal/` | ✅ пусто (кроме комментария в logger_test.go) |
| `grep slog\. backend/cmd/` | ✅ 1 fall-back в `main()` |
| `grep "log/slog" backend/` | ✅ `logger/logger.go`, `logger/logger_test.go`, `config/config.go` — все по плану |

## Отклонения от плана

Мелкие добавления, сохраняющие дух плана:

1. **`handler/param_error.go` → фабрика.** План не упоминал этот файл, но `HandleParamError` использует `writeError(..., logger)`. Переделал в фабрику `HandleParamError(log logger.Logger) func(...)`; в main.go — `handler.HandleParamError(appLogger)`.
2. **`handler/health.go` получил `s.logger`.** Тоже не в плане, но использовал `encodeJSON` — передал `s.logger`.
3. **`audit_test.TestHandleParamError` и `TestRawJSONToAny`** переписаны под метод `s.rawJSONToAny` и фабрику `HandleParamError`, плюс добавлен тест «invalid JSON logs error and returns nil» с EXPECT'ом на log.
4. **Mockery v3 variadic**: из-за поведения `unroll-variadic: false` EXPECT на Info/Error/Warn/Debug принимает attrs как `[]any{...}` slice, а не как развёрнутый variadic. Тесты адаптированы соответственно.

## Статистика

- Файлов изменено/создано: ~30
- `slog.*` вызовов мигрировано: 21 → 1 fall-back
- `slog.*` вызовов удалено из прод-кода в `internal/`: 100%
- Новый пакет: `internal/logger` (logger.go, logger_test.go, mocks/mock_logger.go)
- Новый тестовый файл: `middleware/logging_test.go`

## Заметки

- `internal/logger` покрыт тестами на 100% (не в whitelist coverage gate, но проверено вручную).
- Bезопасные тесты: `Logging` middleware не логирует `Authorization`/Cookie; `BrandService.AssignManager` не логирует email/tempPassword; `AuthService.RequestPasswordReset` не логирует raw-токен.
- E2E: бизнес-логика не менялась, все существующие тесты (audit/auth/brand) проходят.

## Готово к ревью

Все изменения в working tree, без коммитов. Коммит делает Alikhan вручную.
