# Прогресс: рефакторинг репозиториев и сервисов

## Выполнено
- [x] Шаг 1: Mockery config → auto-discovery (all:true + пакеты)
- [x] Шаг 2: dbutil: добавить Pool interface (DB + TxStarter)
- [x] Шаг 3-5: Repository: приватные структуры + интерфейсы + factory + тесты
- [x] Шаг 6: AuthService → pool + RepoFactory + транзакция для ResetPassword
- [x] Шаг 7: BrandService → pool + RepoFactory + транзакция для AssignManager
- [x] Шаг 8: AuditService → pool + RepoFactory
- [x] Шаг 9-10: testTx хелпер + обновление тестов сервисов
- [x] Шаг 11: main.go wiring
- [x] Шаг 12: Перегенерация моков (17 интерфейсов)
- [x] Шаг 13: Полная проверка — всё зелёное

## Результаты проверки
- `make build-backend` — OK
- `make lint-backend` — 0 issues
- `make test-unit-backend` — all passed (with -race)
- `make test-unit-web` — 12 tests passed
- `make test-unit-tma` — 1 test passed
- `make test-unit-landing` — 1 test passed
- `make lint-web` — OK (tsc + eslint)
- `make lint-tma` — OK
- `make lint-landing` — OK
- `make test-e2e-backend` — 42 tests passed (auth, brand, audit)

## Отклонения от плана
- Mockery: `internal/...` рекурсивный паттерн не работал с `api` пакетом (only .gen.go). Заменён на явный список 6 пакетов с `all: true`. Итог тот же — не нужно перечислять каждый интерфейс
- brand.go: пропущен import `errors` — поймано на mockery, исправлено
- user.go: `userInsertColumns` unused без nolint — добавлен nolint комментарий

## Заметки
- Frontend E2E тесты (`test-e2e-frontend`) не запускались — требуют Playwright, не затронуты этим рефакторингом
