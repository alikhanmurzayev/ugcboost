# План реализации: приведение unit-тестов бэкенда к стандартам

## ⚠️ Обязательная преамбула для команды build

**Перед началом работы — полностью загрузи в контекст ВСЕ 19 файлов стандартов из `docs/standards/`, НЕ только «релевантные»:**

```
docs/standards/{backend-architecture, backend-codegen, backend-constants,
backend-design, backend-errors, backend-libraries, backend-repository,
backend-testing-e2e, backend-testing-unit, backend-transactions,
frontend-api, frontend-components, frontend-quality, frontend-state,
frontend-testing-e2e, frontend-testing-unit, frontend-types,
naming, security}.md
```

Также прочитай scout-артефакт целиком: `_bmad-output/planning-artifacts/backend-unit-tests-standards-scout.md`. Стандарты переплетены: `backend-testing-unit.md` опирается на `backend-repository.md` (Row-structs), `backend-transactions.md` (WithTx, RepoFactory), `backend-codegen.md` (mockery + `api/`-типы), `naming.md`, `backend-constants.md` (SQL-литералы в тестах намеренно). Без чтения всех 19 стандартов решения будут локально правильные, но системно неверные.

## 🔒 Критичные pin-правила на всю работу

1. **Никаких `git commit`.** Ни между слайсами, ни в конце. Все изменения остаются в working tree. Коммит и ревью — за Alikhan
2. **Прод-код не трогаем** (за одним исключением ниже). Задача — тесты, стандарты, Makefile, CI. Если где-то захочется поправить прод-код — остановиться, поднять вопрос у Alikhan
3. **Check-in после слайса 0.** После правки стандартов — STOP, показать diff Alikhan, дождаться явного «ок» перед переходом к слайсу 1. Остальные слайсы выполняются автономно до финала
4. **Обнаруженный баг в `AuthService.Refresh`** (2 операции записи без WithTx — см. ниже в разделе «Эскалации») — НЕ тестируем под текущее поведение. Эскалировать Alikhan отдельно, оставить в тестах только сценарии без допущений об атомарности
5. **Ассерты логов — пока не делаем.** Любые попытки перехватить `slog` через `slog.SetDefault` запрещены. Логи будут покрыты после рефакторинга логгера (см. `_bmad-output/planning-artifacts/logger-refactor-brief.md`). В текущей задаче тесты проверяют поведение без зависимости от логов
6. **Прогресс-файл ведётся по ходу работы** — `_bmad-output/planning-artifacts/backend-unit-tests-standards-progress.md`. После каждого слайса обновлять статус, чтобы Alikhan видел прогресс явно
7. **Работа в текущей ветке** `alikhan/staging-cicd`. Новых веток не создаём

---

## Обзор

Привести все unit-тесты backend в соответствие `docs/standards/backend-testing-unit.md` и связанным стандартам. Работа охватывает: правки стандартов (устраняют внутренние противоречия), правки тестов (10 файлов), создание 4 новых тест-файлов, новый Makefile-таргет `test-unit-backend-coverage` с per-method порогом 80%, CI-job с warning (non-blocking). Прод-код не меняется, кроме добавления `pgxmock` в `go.mod`.

## Эскалации (остановиться перед работой над этим)

### E-1: `AuthService.Refresh` делает 2 операции записи без транзакции

**Что обнаружено при scout:** `backend/internal/service/auth.go:114-149` — метод `Refresh` вызывает `ClaimRefreshToken` (это `DELETE...RETURNING`, запись) и `SaveRefreshToken` (запись) без обёртки `dbutil.WithTx`. По стандарту `backend-transactions.md` это требует атомарности: если `SaveRefreshToken` упадёт, старый refresh token уже удалён — пользователь выбит из сессии без возможности восстановления.

**Что делает план:** НЕ пишем тесты под текущее поведение. Не добавляем сценарий «успех без WithTx» как «так и должно быть». В тестах `TestAuthService_Refresh` покрываем только happy path (через реально существующие mock-вызовы) + ошибки repo — без утверждений про атомарность.

**Требуется от Alikhan:** решение — (1) добавить `WithTx` в прод-код сейчас отдельным мини-фиксом, (2) оформить как отдельный issue/backlog и покрыть позже. До решения — тесты пишутся консервативно.

---

## Требования

### Must-have (REQ-*)

- **REQ-1 (repository success-mapping):** Каждый метод `*Repo` имеет тест, проверяющий маппинг строк БД в Go-структуру — все поля `*Row` заполнены ожидаемыми значениями. Реализация — через `pgxmock` (добавляется как dependency)
- **REQ-2 (repository error propagation):** Каждый метод `*Repo` имеет тест на пробрасывание ошибок. `sql.ErrNoRows` остаётся проверяемым через `errors.Is(err, sql.ErrNoRows)`. Для методов, оборачивающих ошибку (`brandRepository.Delete`, `RemoveManager`) — проверить `ErrorContains` + `ErrorIs`
- **REQ-3 (service — все ветки):** Каждая публичная функция `*Service` имеет тест на каждую ветку кода, включая все `if err != nil { return ... }`. Минимум: happy path + все error cases. `bcrypt.GenerateFromPassword error` покрывается через реальный пароль > 72 байт (`bcrypt.ErrPasswordTooLong`)
- **REQ-4 (service — точные аргументы моков):** `mock.MatchedBy` с неполной проверкой заменён на `.Run(...)` + `require.Equal(t, expectedRow, row)` для `AuditLogRow`. JSON-поля (`OldValue`/`NewValue`) сравниваются через `require.JSONEq` перед обнулением и `require.Equal` на остальную структуру
- **REQ-5 (handler — full-response assertion):** Все success-сценарии `TestServer_*` проверяют response целиком через `require.Equal(t, expectedStruct, resp)` после подмены динамических полей. JSON-поля внутри — через `require.JSONEq`
- **REQ-6 (новые тестовые файлы):**
  - `handler/audit_test.go`
  - `handler/test_test.go`
  - `service/audit_test.go`
  - `service/reset_token_store_test.go`
- **REQ-7 (недостающие сценарии service):** Покрыты ранее отсутствующие методы: `AuthService.{RequestPasswordReset, GetUser, GetUserByEmail, SeedUser}`, `BrandService.{GetBrand, ListManagers}`
- **REQ-8 (middleware — full assertions):** `bodylimit_test.go "over limit"` проверяет что inner handler получает `*http.MaxBytesError` из `io.ReadAll`; добавлен edge-case «exact limit». `auth_test.go` использует `errors.New` вместо `fmt.Errorf`, проверяет error-response body целиком
- **REQ-9 (`withAdminCtx` → `withRole`):** `withAdminCtx` переименован в `withRole(userID string, role api.UserRole)`, все существующие вызовы переписаны, shim не оставляется
- **REQ-10 (per-method coverage ≥ 80%):** Каждый публичный метод (имя с заглавной буквы) в пакетах `handler`, `service`, `repository`, `middleware`, `authz` имеет coverage ≥ 80%. Исключения: `*.gen.go`, `*/mocks/`, `cmd/`. Проверяется через новый Makefile-таргет `test-unit-backend-coverage` (падает локально при < 80% на любой метод) + CI-job с warning (non-blocking). Обычный `make test-unit-backend` coverage не проверяет
- **REQ-11 (качество):** `make test-unit-backend` зелёный. `make lint-backend` = 0 issues. `cd backend && go vet ./...` = 0 issues. `make test-e2e-backend` зелёный. `make test-unit-backend-coverage` зелёный локально

### Nice-to-have

- Параметризация `withAdminCtx` попутно — **уже в REQ-9**
- Дополнительная проверка `TokenService` — валидация JWT claims через `jwt.ParseWithClaims` отдельно от round-trip

### Вне скоупа

- **Прод-код не меняем** (кроме `go.mod` для `pgxmock`). Рефакторинг `Refresh` под `WithTx` — эскалация E-1
- **Ассерты логов** — отложены до рефакторинга логгера (`logger-refactor-brief.md`)
- Тесты для `middleware/logging.go` и `middleware/json.go` — тривиальные wrapper'ы, транзитивно покрыты
- Тесты для `handler/health.go` — статический ответ
- Интеграционные тесты / testcontainers — это уровень e2e
- Frontend-тесты — не в скоупе
- Изменение `.github/workflows/*` за пределами добавления coverage-job — не трогаем

### Критерии успеха

1. Стандарты `backend-testing-unit.md` и `backend-libraries.md` поправлены и одобрены Alikhan (после слайса 0)
2. `make test-unit-backend` зелёный (race detector включён)
3. `make lint-backend` = 0 issues
4. `cd backend && go vet ./...` = 0 issues
5. `make test-e2e-backend` зелёный
6. `make test-unit-backend-coverage` зелёный локально (per-method ≥ 80%)
7. CI содержит non-blocking warning-job для coverage
8. Все публичные методы `*Service` / `*Handler` / `*Repo` имеют соответствующую `Test{Struct}_{Method}` функцию
9. Все repo-методы имеют 2-3 `t.Run`: SQL + маппинг + error propagation
10. Working tree готов к ревью Alikhan, коммит — за ним

---

## Файлы для изменения

| Файл | Изменения |
|------|-----------|
| `docs/standards/backend-testing-unit.md` | (1) В секции `## Моки` убрать строку «Порядок вызовов проверяется. Например: если сервис сохранит токен до проверки пароля — тест должен упасть». (2) В подсекции `### Service` убрать строку «Порядок вызовов моков проверяется». (3) В секции `## Assertions` добавить правило: «JSON-поля (`json.RawMessage`, `[]byte` с сериализованным JSON) сравниваются через `require.JSONEq` — порядок ключей в `json.Marshal(map[...]...)` не детерминирован, `require.Equal` на сырые байты создаст флаки. При сравнении содержащей структуры целиком: сначала `JSONEq` на JSON-поле, затем обнулить его перед `require.Equal` на структуру». |
| `docs/standards/backend-libraries.md` | Переформулировать как **общий** принцип «библиотеки вместо велосипедов», снять привязку к «инфраструктурным задачам». Инфраструктура — один из примеров, не единственная область. Правило применяется и к тестовым утилитам (напр. `pgxmock`), и к бизнес-хелперам, если есть устоявшаяся библиотека |
| `Makefile` | Добавить таргет `test-unit-backend-coverage`: запускает `go test -coverprofile=/tmp/ugc-cover.out -race -count=1 ./internal/...`, парсит через `go tool cover -func=...`, фильтрует публичные методы (имя начинается с заглавной буквы), исключает `*.gen.go`, `*/mocks/`, `cmd/`. Exit 1 при любом методе < 80%. Не зависит от `test-unit-backend` (отдельный прогон, иначе coverprofile overhead попадёт в основной таргет) |
| `.github/workflows/*.yml` | Добавить job/step `coverage-check`: вызывает `make test-unit-backend-coverage`, `continue-on-error: true`, выводит понятный summary в GitHub UI (`echo "::warning::..."` или step summary). Точное расположение и формат уточняются при scout'инге текущего workflow в слайсе 0 |
| `backend/go.mod` / `backend/go.sum` | Добавить dependency `github.com/pashagolub/pgxmock` (v4, совместимая с pgx/v5) через `go get` |
| `backend/internal/repository/helpers_test.go` | Переписать на `pgxmock`-based хелперы: `newPgxmock(t *testing.T) (pgxmock.PgxPoolIface, cleanup)` — возвращает мок, реализующий `dbutil.DB`. Убрать `captureQuery`/`captureExec`/`scalarRows` как устаревшие (если не используются в новых тестах) |
| `backend/internal/repository/user_test.go` | Для каждого из 10 методов `UserRepo`: `t.Run("SQL")` — точная SQL + args через `ExpectQuery`/`ExpectExec`; `t.Run("maps row")` — `AddRow(...)` в pgxmock, проверить все поля `*UserRow`/`*RefreshTokenRow`/`*PasswordResetTokenRow`; `t.Run("propagates sql.ErrNoRows")` — для `GetByEmail`/`GetByID`/`ClaimRefreshToken`/`ClaimResetToken` (через `errors.Is`); `t.Run("wraps other errors")`. Особые: `ExistsByEmail` — `no row → (false, nil)`; `UpdatePassword` — `n=0 → sql.ErrNoRows` |
| `backend/internal/repository/brand_test.go` | Аналогично user_test. Особые: `Delete`/`RemoveManager` — оборачивание `sql.ErrNoRows` в `brand not found`/`manager assignment not found` (через `ErrorContains` + `ErrorIs`); `IsManager` — `no row → (false, nil)`; `List`/`ListByUser`/`ListManagers`/`GetBrandIDsForUser` — маппинг на 2+ элементов |
| `backend/internal/repository/audit_test.go` | `Create`: `SQL`, `propagates error`. `List`: `empty (total=0) → (nil, 0, nil)` (ожидаем ровно 1 Query-вызов, data-query не вызывается), `count error propagates`, `data query error propagates`, `maps rows to structs` с 2 записями (включая `EntityID *string` и `OldValue`/`NewValue json.RawMessage` — последние через `JSONEq`) |
| `backend/internal/service/auth_test.go` | Добавить error-ветки: `Login`: token generation, save refresh, audit errors. `Refresh`: ClaimRefreshToken, GetByID, token generation, save refresh errors (БЕЗ допущений про WithTx — см. E-1). `Logout`: delete refresh, audit errors. `ResetPassword`: bcrypt error (пароль > 72 байт), UpdatePassword, DeleteUserRefreshTokens, audit errors. `SeedAdmin`: ExistsByEmail, hash (> 72 байт), Create errors. Новые функции: `TestAuthService_RequestPasswordReset` (user not found → nil, GenerateResetToken, SaveResetToken, notifier=nil/set), `TestAuthService_GetUser`, `TestAuthService_GetUserByEmail`, `TestAuthService_SeedUser`. Заменить `mock.MatchedBy` → `.Run(...)` + `require.Equal` для `AuditLogRow` (JSON-поля через `JSONEq`, `CreatedAt` ожидается zero-time — `writeAudit` не задаёт его) |
| `backend/internal/service/brand_test.go` | Новые функции: `TestBrandService_GetBrand`, `TestBrandService_ListManagers`. Покрыть error-ветки в `AssignManager`: brand not found, check user error, get user error (existing), create user error (new, + hash > 72 байт), assign manager error, audit error. `ListBrands` + error. `CreateBrand`/`UpdateBrand`/`DeleteBrand` + repo и audit errors. `mock.MatchedBy` → `.Run` + `require.Equal` с `JSONEq` |
| `backend/internal/service/token_test.go` | Добавить `"claims content"` в `TestTokenService_GenerateAccessToken`: парсить токен через `jwt.ParseWithClaims`, проверить `sub`, `role`, `exp` отдельно от round-trip |
| `backend/internal/handler/auth_test.go` | Все success-сценарии (`Login`, `RefreshToken`, `Logout`, `GetMe`, `RequestPasswordReset`, `ResetPassword`) — `require.Equal` на response целиком после подмены динамики. Добавить: `RefreshToken "service error"`, `Logout "service error"`, `ResetPassword "service error"` |
| `backend/internal/handler/brand_test.go` | `ListBrands` (admin + manager) / `GetBrand "success"` / `UpdateBrand "success"` / `DeleteBrand "success"` / `CreateBrand "success"` / `RemoveManager "success"` — `require.Equal` на response целиком |
| `backend/internal/handler/helpers_test.go` | `withAdminCtx` → `withRole(userID string, role api.UserRole)`. Все ~8 вызовов `withAdminCtx` в `auth_test.go`/`brand_test.go` переписать на `withRole("u-admin", api.Admin)` или с нужной ролью. Shim не оставляем |
| `backend/internal/middleware/bodylimit_test.go` | `"over limit"`: inner handler делает `io.ReadAll(r.Body)`, проверить что err типа `*http.MaxBytesError` (через `errors.As`). Добавить `"exact limit"`: body == N байт → `io.ReadAll` успешен, `w.Code == 200`. Не проверяем статус в `"over limit"` — это ответственность handler'а, не middleware |
| `backend/internal/middleware/auth_test.go` | `fmt.Errorf("expired")` → `errors.New("expired")`. В сценарии `"invalid token"` — `require.Equal` на полный `api.ErrorResponse` (после подмены динамических полей, если есть) |

## Файлы для создания

| Файл | Назначение |
|------|------------|
| `_bmad-output/planning-artifacts/backend-unit-tests-standards-progress.md` | Progress-лог. Заполняется по ходу: какой слайс начат/завершён, какие файлы затронуты, результаты `make test-unit-backend` / `lint` / `vet` / coverage. Обновляется после каждого слайса — Alikhan сможет следить за ходом без чтения длинных тул-выводов |
| `backend/internal/handler/audit_test.go` | `TestServer_ListAuditLogs`: manager forbidden, admin success без фильтров (+ full `require.Equal`), admin со всеми фильтрами (`actorId`/`entityType`/`entityId`/`action`/`dateFrom`/`dateTo`/`page`/`perPage`), empty result, service error → 500. `TestRawJSONToAny`: nil/empty → nil, valid json → decoded, invalid json → nil (факт возврата nil проверяем, **ассерт на лог — не делаем**, ждём рефакторинг логгера) |
| `backend/internal/handler/test_test.go` | `TestTestHandler_SeedUser`: invalid JSON, empty fields, service error, success с full `require.Equal`. `TestTestHandler_SeedBrand`: invalid JSON, empty name, brand create error, success без managerEmail (проверить impersonation ctx через `.Run(func(args) { ctx := args.Get(0).(context.Context); require.Equal(t, adminID, middleware.UserIDFromContext(ctx)); require.Equal(t, api.Admin, middleware.RoleFromContext(ctx)) })`), success с managerEmail, assign manager error. `TestTestHandler_GetResetToken`: not found → 404, success с full `require.Equal` |
| `backend/internal/service/audit_test.go` | `TestAuditService_List` на service-уровне: empty (repo возвращает `nil, 0, nil` — ровно один вызов `repo.List`), repo error, success с filtering + пагинацией + маппингом `AuditLogRow → domain.AuditLog` (включая `OldValue`/`NewValue` через `JSONEq`). Тесты на приватные `writeAudit`/`contextWithActor` — через публичный API (`Login`/`CreateBrand`/etc), прямой вызов если в helpers_test.go уместен exposer |
| `backend/internal/service/reset_token_store_test.go` | `TestInMemoryResetTokenStore`: empty store → (empty, false); set+get → (token, true); overwrite same email; isolation by email. `TestInMemoryResetTokenStore_Concurrency`: 100 goroutines параллельно пишут и читают, race detector ловит datarace — тест обязан пройти с `-race` |

---

## Шаги реализации

### Слайс 0 — правки стандартов (блокирующий check-in)

1. [ ] Прочесть в контекст все 19 файлов из `docs/standards/` + scout-артефакт + этот план целиком
2. [ ] `make test-unit-backend` — убедиться что baseline зелёный
3. [ ] `cd backend && go test -cover ./internal/... 2>&1 | tee /tmp/ugc-coverage-before.txt` — зафиксировать стартовое покрытие
4. [ ] Создать `_bmad-output/planning-artifacts/backend-unit-tests-standards-progress.md` с пустым шаблоном: слайсы со статусами `pending`/`in-progress`/`done`
5. [ ] Править `docs/standards/backend-testing-unit.md`:
   - Убрать «Порядок вызовов проверяется. Например: если сервис сохранит токен до проверки пароля — тест должен упасть» из секции `## Моки`
   - Убрать «Порядок вызовов моков проверяется» из подсекции `### Service`
   - В секции `## Assertions` добавить новый bullet про `require.JSONEq` для JSON-полей (формулировка из раздела «Файлы для изменения»)
6. [ ] Править `docs/standards/backend-libraries.md`:
   - Переформулировать вводный абзац как общий принцип «библиотеки вместо велосипедов»
   - Убрать жёсткую привязку «Инфраструктурный код ... — использовать проверенные библиотеки. Самописные решения для инфраструктурных задач запрещены»
   - Заменить на: правило применяется к любому устоявшемуся решению (инфра, тестовые утилиты, бизнес-хелперы), при наличии активно поддерживаемой библиотеки — использовать её; кастом оправдан только когда нет адекватной альтернативы
   - Инфру оставить как **пример**, а не единственную область
7. [ ] `cd backend && make lint-backend` — линт стандартов не нужен, но убедиться что документация не сломала автогенерацию
8. [ ] Обновить progress-файл: слайс 0 = done
9. [ ] **STOP.** Показать Alikhan diff двух файлов стандартов (через `git diff docs/standards/`). Дождаться явного «ок» перед переходом к слайсу 1. **Не продолжать автономно.**

### Слайс 1 — инфраструктура тестов (pgxmock, Makefile, CI)

10. [ ] `cd backend && go get github.com/pashagolub/pgxmock/v4@latest` (версия совместимая с pgx/v5) — добавить dep
11. [ ] `cd backend && go mod tidy` — зафиксировать `go.sum`
12. [ ] Добавить в `Makefile` таргет `test-unit-backend-coverage`:
    - Запускает `go test -coverprofile=/tmp/ugc-cover.out -race -count=1 ./internal/...`
    - Парсит через `go tool cover -func=/tmp/ugc-cover.out`
    - awk-фильтр: только строки с `\.go:[0-9]+:\s+[A-Z]` (публичные методы), исключить `\.(gen\.go|mocks/)`, `cmd/`
    - Для каждого метода: если coverage < 80% — печать `FAIL file:line Method pct%`, финальный `exit 1`
    - Успех: тихий выход 0
13. [ ] Проверить текущее содержимое `.github/workflows/` (glob `*.yml`), найти job с существующим `make test-unit-backend` или близким
14. [ ] Добавить в найденный workflow job `coverage-warning`:
    - Запускает `make test-unit-backend-coverage`
    - `continue-on-error: true` (не блокирует следующие джобы и деплой на stg)
    - В случае фейла — выводит `::warning::Coverage below 80% on method X` в GitHub Actions summary, чтобы PR-автор увидел в UI
    - Помещается после `test-unit-backend`, до деплоя
15. [ ] Локальная проверка: `make test-unit-backend-coverage` — прогнать. Ожидается FAIL (сейчас покрытие на repo <80%). Это baseline — значит таргет работает
16. [ ] Обновить progress-файл: слайс 1 = done

### Слайс 2 — repository

17. [ ] Переписать `backend/internal/repository/helpers_test.go` на `pgxmock`:
    - Экспортировать хелпер `newPgxmock(t *testing.T) pgxmock.PgxPoolIface` (или `pgxmock.PgxConnIface`, в зависимости от того, что реализует `dbutil.DB`)
    - Проверить что `pgxmock.PgxPoolIface` реализует все методы `dbutil.DB` (`Query`, `QueryRow`, `Exec`) — должно быть
    - Убрать `captureQuery`/`captureExec`/`scalarRows`, если они больше не используются (но сначала убедиться, что все тесты мигрированы)
18. [ ] `backend/internal/repository/user_test.go`:
    - Для каждого из 10 методов — 3 `t.Run`: SQL, maps row, propagates error (где применимо)
    - `GetByEmail`/`GetByID`/`ClaimRefreshToken`/`ClaimResetToken` — `propagates sql.ErrNoRows` через `errors.Is`
    - `ExistsByEmail` — `no row returns (false, nil)` + `other error propagates`
    - `UpdatePassword` — `zero rows affected → sql.ErrNoRows`
19. [ ] `backend/internal/repository/brand_test.go`:
    - Аналогично для 11 методов
    - `Delete` — `n=0 → wrapped sql.ErrNoRows` с `ErrorContains("brand not found")` + `ErrorIs(sql.ErrNoRows)`
    - `RemoveManager` — аналогично с `"manager assignment not found"`
    - `IsManager` — `no row returns (false, nil)`
    - `List`/`ListByUser`/`ListManagers`/`GetBrandIDsForUser` — маппинг на 2+ элементов
20. [ ] `backend/internal/repository/audit_test.go`:
    - `Create`: SQL + error propagation
    - `List`: empty (total=0 → ровно 1 Query), count error, data query error, maps rows (2 записи, JSON-поля через `JSONEq`)
21. [ ] `cd backend && go test ./internal/repository/... -race -count=1` — зелёный
22. [ ] `cd backend && go test -cover ./internal/repository/...` — per-method ≥ 80%
23. [ ] `make test-unit-backend-coverage` — прогнать, проверить что repository уже не в FAIL-списке
24. [ ] Обновить progress-файл: слайс 2 = done

### Слайс 3 — service

25. [ ] Создать `backend/internal/service/audit_test.go`:
    - `TestAuditService_List` на service-уровне: empty (repo.List возвращает `nil, 0, nil` — один вызов), repo error, success с filter + пагинацией + маппинг `AuditLogRow → domain.AuditLog` (`OldValue`/`NewValue` через `JSONEq`)
    - Тесты на `writeAudit`/`contextWithActor` через публичные методы, либо через exposer в helpers_test.go если прямой вызов удобнее
26. [ ] Создать `backend/internal/service/reset_token_store_test.go`:
    - `TestInMemoryResetTokenStore`: пустой → (empty, false); set+get; overwrite; isolation by email
    - `TestInMemoryResetTokenStore_Concurrency`: 100 goroutines с writeg/read параллельно, проверка race через `-race`
27. [ ] Дополнить `backend/internal/service/auth_test.go`:
    - `TestAuthService_Login`: + token generation error, save refresh error, audit error. В success — перевести `mock.MatchedBy` на `.Run(...) + require.Equal` для `AuditLogRow` (JSON-поля через `JSONEq`)
    - `TestAuthService_Refresh`: + ClaimRefreshToken error, GetByID error, token generation error, save refresh error. **Без допущений про WithTx — см. E-1**
    - `TestAuthService_Logout`: + delete refresh tokens error, audit error
    - `TestAuthService_ResetPassword`: + bcrypt hash error (пароль > 72 байт), UpdatePassword error, DeleteUserRefreshTokens error, audit error
    - `TestAuthService_SeedAdmin`: + ExistsByEmail error, hash error (> 72), Create error
    - Новые: `TestAuthService_RequestPasswordReset` (user not found → nil, GenerateResetToken error, SaveResetToken error, notifier=nil — OK, notifier set — `OnResetToken` вызван с raw), `TestAuthService_GetUser` (success + repo error), `TestAuthService_GetUserByEmail` (аналогично), `TestAuthService_SeedUser` (success, hash error, repo error)
28. [ ] Дополнить `backend/internal/service/brand_test.go`:
    - Новые: `TestBrandService_GetBrand` (success + маппинг, repo error), `TestBrandService_ListManagers` (success, маппинг `BrandManagerRow → domain.BrandManager`, empty, repo error)
    - `TestBrandService_CreateBrand`: + repo error, audit error; `mock.MatchedBy → .Run + require.Equal` с `JSONEq`
    - `TestBrandService_UpdateBrand`: аналогично
    - `TestBrandService_DeleteBrand`: + repo error, audit error
    - `TestBrandService_ListBrands`: + list error, listByUser error
    - `TestBrandService_AssignManager`: все ветки — brand not found, check user error, get user error (existing path), hash error (> 72, new path), create user error, assign manager error, audit error
    - `TestBrandService_RemoveManager`: + repo error, audit error
29. [ ] Дополнить `backend/internal/service/token_test.go`: `"claims content"` в `TestTokenService_GenerateAccessToken` — парсить через `jwt.ParseWithClaims`
30. [ ] `cd backend && go test ./internal/service/... -race -count=1` — зелёный
31. [ ] `make test-unit-backend-coverage` — service не в FAIL-списке
32. [ ] Обновить progress-файл: слайс 3 = done

### Слайс 4 — handler

33. [ ] `backend/internal/handler/helpers_test.go`: `withAdminCtx` → `withRole(userID string, role api.UserRole)`. Переписать все ~8 существующих вызовов в `auth_test.go`/`brand_test.go`. Shim не оставляем
34. [ ] Создать `backend/internal/handler/audit_test.go`:
    - `TestServer_ListAuditLogs`: manager forbidden (403), admin no filters (full `require.Equal` на `api.AuditLogsResult`), admin with all filters, empty result, service error → 500
    - `TestRawJSONToAny`: nil/empty → nil; valid json → decoded; invalid json → nil. **Не делаем ассерт на лог** (до рефакторинга логгера)
35. [ ] Создать `backend/internal/handler/test_test.go`:
    - `TestTestHandler_SeedUser`: invalid JSON, empty fields, service error, success full `require.Equal`
    - `TestTestHandler_SeedBrand`: invalid JSON, empty name, brand error, success без managerEmail (impersonation: мок `.Run(...)` проверяет `ctx` содержит `adminID` + `api.Admin`), success с managerEmail (оба мока вызваны в порядке Create → AssignManager через простые `.Once()` expectations без `.NotBefore` — стандарт больше не требует порядок), assign error
    - `TestTestHandler_GetResetToken`: not found → 404, success full `require.Equal`
36. [ ] Дополнить `backend/internal/handler/auth_test.go`:
    - `TestServer_Login "success"`: проверить что response + cookie через `require.Equal` целиком ✅ (уже есть)
    - `TestServer_RefreshToken "success"`: перевести на `require.Equal` целиком; добавить `"service error"` (unauthorized)
    - `TestServer_Logout "success"`: `require.Equal` на `MessageResponse`; добавить `"service error"` → 500
    - `TestServer_ResetPassword "success"`: `require.Equal`; уже покрыты error-ветки
    - `TestServer_RequestPasswordReset "always returns 200"`: `require.Equal` на `MessageResponse`
    - `TestServer_GetMe "success"`: уже `require.Equal` ✅
37. [ ] Дополнить `backend/internal/handler/brand_test.go`:
    - `TestServer_CreateBrand "success"`: `require.Equal` целиком (сейчас 2 поля)
    - `TestServer_ListBrands "admin"` + `"manager"`: `require.Equal` на `ListBrandsResult`
    - `TestServer_GetBrand "success"`: `require.Equal` на `GetBrandResult`
    - `TestServer_UpdateBrand "success"`: `require.Equal` на `BrandResult`
    - `TestServer_DeleteBrand "success"`: `require.Equal` на `MessageResponse`
    - `TestServer_RemoveManager "success"`: `require.Equal` на `MessageResponse`
    - `TestServer_AssignManager "existing user has no temp password"`: `require.Equal` на `AssignManagerResult`
38. [ ] `cd backend && go test ./internal/handler/... -race -count=1` — зелёный
39. [ ] `make test-unit-backend-coverage` — handler не в FAIL-списке
40. [ ] Обновить progress-файл: слайс 4 = done

### Слайс 5 — middleware

41. [ ] `backend/internal/middleware/bodylimit_test.go`:
    - `"over limit"`: inner handler делает `io.ReadAll(r.Body)`, через `errors.As` проверить что err — `*http.MaxBytesError`. Status не проверяем (ответственность handler'а)
    - Добавить `"exact limit"`: body == N байт → `io.ReadAll` успешен, inner handler пишет 200
42. [ ] `backend/internal/middleware/auth_test.go`:
    - `fmt.Errorf("expired")` → `errors.New("expired")`
    - `"invalid token"`: `require.Equal` на полный `api.ErrorResponse`
43. [ ] `cd backend && go test ./internal/middleware/... -race -count=1` — зелёный
44. [ ] `make test-unit-backend-coverage` — middleware не в FAIL-списке
45. [ ] Обновить progress-файл: слайс 5 = done

### Слайс 6 — финал

**Никаких `git commit` не делаем. Все изменения остаются в working tree, Alikhan коммитит сам.**

46. [ ] `cd backend && go test -cover ./internal/... 2>&1 | tee /tmp/ugc-coverage-after.txt` — сравнить с before, зафиксировать прирост
47. [ ] `make test-unit-backend` — зелёный (полный прогон)
48. [ ] `cd backend && go vet ./...` — 0 issues
49. [ ] `make lint-backend` — 0 issues
50. [ ] `make test-e2e-backend` — зелёный (страховка, что ничего не сломалось через регенерацию моков или добавление pgxmock)
51. [ ] `make build-backend` — зелёный
52. [ ] `make test-unit-backend-coverage` — зелёный локально (per-method ≥ 80% на всех пакетах)
53. [ ] Diff-review: `git status` + `git diff --stat`. Ожидаем изменения в:
    - `docs/standards/backend-testing-unit.md`, `backend-libraries.md`
    - `Makefile`
    - `.github/workflows/*.yml` (добавление одного job)
    - `backend/go.mod`, `backend/go.sum` (pgxmock)
    - `backend/internal/**/*_test.go` (модифицированные + новые)
    - `backend/internal/**/mocks/*.go` — **пусто** (интерфейсы не меняли). Если что-то появилось — разобраться почему
    - `_bmad-output/planning-artifacts/backend-unit-tests-standards-progress.md`
    Изменения в других `*.go` прод-файлах — **тревога**, агент что-то трогнул вне скоупа. Откатить точечно
54. [ ] Финальное обновление progress-файла: итоги, coverage before/after (сводка), чек-лист критериев успеха REQ-1...REQ-11
55. [ ] Отчёт Alikhan в чат: список затронутых файлов по слайсам, результаты make-команд, emerging issues (в т.ч. напомнить про эскалацию E-1 `AuthService.Refresh`), явное «коммит за тобой»

---

## Стратегия тестирования

### Unit-тесты

Сама задача — писать/править unit-тесты. Мета-уровень: каждое изменение должно оставлять `make test-unit-backend` зелёным. После каждого слайса — прогон тестов пакета через `go test ./internal/<pkg>/... -race -count=1` и `make test-unit-backend-coverage` (чтобы следить за прогрессом покрытия).

### E2E-тесты

Не меняем. Прогоняем **в финале** (шаг 50) как страховку: добавление `pgxmock` в `go.mod` и возможные обновления моков не должны сломать e2e-контракт.

### Coverage

- Baseline: зафиксирован в шаге 3 (`/tmp/ugc-coverage-before.txt`)
- Target: каждый публичный метод ≥ 80% (проверяется `make test-unit-backend-coverage`)
- В CI — warning-джоба (non-blocking), даст быстрый feedback в PR без блокировки деплоя

### Race detector

`-race` во всех `go test` командах. Особенно критично для `service/reset_token_store_test.go` (concurrency explicit).

### Логи

**Ассерты на логи не делаем.** Если в тесте возникает искушение проверить что лог был вызван с таким-то уровнем — воздержаться, пометить `// TODO: cover via mock logger after logger-refactor-brief.md`. Исключение: если ассерт тривиальный и не требует `slog.SetDefault` — всё равно пропустить, чтобы единообразно дождаться рефакторинга.

---

## Оценка рисков

| Риск | Вероятность | Митигация |
|------|-------------|-----------|
| `pgxmock` API не полностью совместим с нашим `dbutil.DB` интерфейсом | Низкая-средняя | `pgxmock.PgxPoolIface` реализует `Query`/`QueryRow`/`Exec` — все наши требования. Проверяется на первом же тесте в слайсе 2; если несовместимость — быстрая эскалация |
| Regen моков (`make generate-mocks`) ломает существующие тесты | Низкая — интерфейсы не трогаем | В финале `git diff backend/internal/**/mocks/` должен быть пустой. Если не пустой — сверить что мы не добавили методы в интерфейсы |
| `require.Equal` целиком падает из-за полей, которых нет в моке | Средняя | Сначала точечные assertions; подмена динамики (time/UUID/JSON-поля); потом перевод на Equal. JSON-поля — обязательно через `JSONEq` по новому правилу стандарта |
| `pgxmock` `ExpectQuery.WillReturnRows` требует знать `FieldDescriptions` | Низкая | У `pgxmock` `pgxmock.NewRows([]string{"id", "email", ...}).AddRow(...)` — прост и явен. Риск минимальный |
| CI-job с `continue-on-error` игнорируется и warning не виден | Средняя | Добавить step summary (`echo "::warning::..."`) + `$GITHUB_STEP_SUMMARY`; явно проверить в PR после первой сборки |
| Работа разрастается из-за большого числа методов | Средняя | Слайсы выполняются автономно (после одобрения слайса 0). Прогресс-файл обновляется после каждого слайса. Если на каком-то слайсе агент понимает что что-то идёт не так — остановиться и эскалировать, не продолжать вслепую |
| Покрытие 80% на какой-то метод недостижимо (экзотические ветки) | Средняя | Если ветка объективно не покрывается unit-тестом (например, невозможная системная ошибка) — документировать в progress-файле как accepted exception, вынести в отдельный пункт эскалации. **Не подавлять** через `//coverage:ignore` |
| `jwt.ParseWithClaims` требует тип `*jwt.RegisteredClaims` (или кастомный), отличный от того что у `TokenService.Validate` | Низкая | Смотреть прод-код `TokenService`, использовать те же claims-типы. Если нет — парсить как `jwt.MapClaims` и проверять поля |

---

## План отката

Работа ведётся в текущей ветке `alikhan/staging-cicd` **без промежуточных коммитов**. Откат одной группой команд:

```bash
git restore docs/standards/ Makefile .github/ backend/
git clean -fd backend/
rm -f _bmad-output/planning-artifacts/backend-unit-tests-standards-progress.md
```

(Оставляем scout/plan-артефакты, удаляем только progress, который создаётся по ходу работы.)

Прод-код не трогаем → откат не затрагивает runtime. Worst case — теряем работу над тестами/стандартами/инфраструктурой.

---

## Итог

- 7 слайсов. Слайс 0 — блокирующий check-in с Alikhan. Остальные — автономно
- **Git:** без коммитов. Всё в working tree до ручного ревью
- **Прод-код:** не трогаем, кроме `go.mod` (добавление `pgxmock`)
- **Эскалации:**
  - E-1: баг в `AuthService.Refresh` (2 записи без WithTx) — не тестировать под багованное поведение, эскалировать Alikhan
- **Связанные документы:**
  - `_bmad-output/planning-artifacts/backend-unit-tests-standards-scout.md` — разведка
  - `_bmad-output/planning-artifacts/backend-unit-tests-standards-progress.md` — живой прогресс (создаётся в слайсе 0)
  - `_bmad-output/planning-artifacts/logger-refactor-brief.md` — будущая задача, после которой можно будет покрыть логи
- **Ожидаемый результат:**
  - Стандарты `backend-testing-unit.md` и `backend-libraries.md` приведены к внутренней консистентности
  - Все 20 существующих тест-файлов доведены до стандарта
  - 4 новых файла созданы
  - Per-method coverage ≥ 80% + автоматизированная проверка через Makefile/CI
  - `make test-unit-backend`, `lint-backend`, `vet`, `test-e2e-backend`, `test-unit-backend-coverage` зелёные
  - Working tree готов к ревью Alikhan

---

**План выглядит нормально? Нужны правки перед тем, как начать реализацию?**
