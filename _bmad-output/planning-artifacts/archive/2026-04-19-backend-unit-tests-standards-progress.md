# Прогресс: приведение unit-тестов бэкенда к стандартам

Работа ведётся в ветке `alikhan/staging-cicd` **без промежуточных коммитов**. Всё остаётся в working tree до ручного ревью Alikhan.

План: `_bmad-output/planning-artifacts/backend-unit-tests-standards-plan.md`
Scout: `_bmad-output/planning-artifacts/backend-unit-tests-standards-scout.md`

## Baseline

- `make test-unit-backend` — зелёный ✅
- Coverage (из `/tmp/ugc-coverage-before.txt`):
  - `authz` — 100.0%
  - `closer` — 100.0%
  - `handler` — 54.5%
  - `middleware` — 74.5%
  - `repository` — 80.2%
  - `service` — 66.2%

## Слайсы

### Слайс 0 — правки стандартов (блокирующий check-in)

Статус: `done` (ждёт check-in)

- [x] Прочитан план + scout-артефакт + 19 файлов `docs/standards/`
- [x] Baseline `make test-unit-backend` — зелёный
- [x] Baseline coverage → `/tmp/ugc-coverage-before.txt`
- [x] Progress-файл создан
- [x] `docs/standards/backend-testing-unit.md` — убраны 2 упоминания «порядок вызовов», добавлено правило про JSONEq
- [x] `docs/standards/backend-libraries.md` — переформулирован как общий принцип
- [x] STOP — diff стандартов на ревью Alikhan

### Слайс 1 — инфра (pgxmock, Makefile, CI)

Статус: `done`

- [x] `go get github.com/pashagolub/pgxmock/v4@latest` — v4.9.0 скачан; `go mod tidy` убрал как unused (импорт появится в слайсе 2)
- [x] Makefile: `test-unit-backend-coverage` — awk-фильтр ограничен 5 пакетами REQ-10 (`handler/service/repository/middleware/authz`), исключает `*.gen.go`, `*/mocks/`, `cmd/`, `handler/health.go`, `middleware/{logging,json}.go`
- [x] CI: job `coverage-warning` в `.github/workflows/ci.yml` с `continue-on-error: true`, `needs: [test-unit-backend]`, выводит warning + job summary
- [x] Локальная проверка: таргет корректно фейлится на baseline (~35 методов < 80%)

**Расширения плана (implicit по REQ-10):** дополнительно к поимённому списку план неявно требует покрытия:
- `handler/param_error.go:HandleParamError` — 1-строчный wrapper, покрою косвенно через handler-тесты с невалидными params
- `middleware/auth.go:AuthFromScopes` — дублирует логику Auth; добавлю `TestAuthFromScopes` в слайсе 5
- `repository/factory.go:NewRepoFactory/NewUserRepo/NewBrandRepo/NewAuditRepo` — тривиальные конструкторы; покрою минимальным `TestRepoFactory` в слайсе 2

### Слайс 2 — repository

Статус: `done`

- [x] `helpers_test.go` → один хелпер `newPgxmock(t)` с `QueryMatcherEqual` + cleanup. Старые `captureQuery/captureExec/scalarRows` удалены как неиспользуемые
- [x] `user_test.go` — 10 методов × (success maps row / success + error propagation)
- [x] `brand_test.go` — 11 методов × то же; Delete/RemoveManager проверяют `ErrorContains + ErrorIs(sql.ErrNoRows)`; IsManager — `no row → (false, nil)`
- [x] `audit_test.go` — Create + List (empty без data-query, count error, data error, success с JSONEq на `OldValue`/`NewValue`, второй страничный кейс с OFFSET)
- [x] `factory_test.go` — новый, покрывает `NewRepoFactory/NewUserRepo/NewBrandRepo/NewAuditRepo`
- [x] Per-method coverage ≥ 80% на `repository` (пакет 93.4%)
- [x] pgxmock v4.9.0 добавлен в `go.mod` как direct dependency

**Гочи, пойманные в процессе:**
- `stom` разворачивает non-nil `*string` в `string` (Create), но `sq.Set(col, ptr)` передаёт `*string` как есть (Update) — тесты различают
- `stom` для nil `*string` отдаёт untyped `nil` в map — `WithArgs(..., nil, ...)`, не `(*string)(nil)`
- `pgx.RowToStructByName` для поля `*string` требует `*string` или nil в `AddRow`, не голый `string`
- `pgx.ErrNoRows` в v5 через `Is()` совместим с `sql.ErrNoRows` — `errors.Is(pgx.ErrNoRows, sql.ErrNoRows) == true`

### Слайс 3 — service

Статус: `done`

- [x] `service/audit_test.go` — новый; empty + repo error + success с фильтрами, пагинацией, JSONEq на Old/New
- [x] `service/reset_token_store_test.go` — новый; empty/set+get/overwrite/isolation + 100-goroutine concurrency (race detector)
- [x] `service/auth_test.go` — полный rewrite; все error-ветки (token gen, save refresh, audit, bcrypt >72 через `bcrypt.ErrPasswordTooLong`), новые методы `RequestPasswordReset/GetUser/GetUserByEmail/SeedUser`, `SeedAdmin` (exists err, hash err, create err)
- [x] `service/brand_test.go` — полный rewrite; `GetBrand/ListManagers` как новые, все error-ветки `AssignManager` (brand not found, check user err, get user err, create user err, assign err, audit err)
- [x] `service/token_test.go` — «claims content» через `jwt.ParseWithClaims` + `GetSubject()` + проверка `exp` window
- [x] `MatchedBy → .Run(typed) + require.Equal + JSONEq` через helper `expectAudit` (в `auth_test.go`) и `expectBrandAudit` (в `brand_test.go`)
- [x] Per-method coverage ≥ 80% на `service` (пакет 97.1%)

**Гочи:**
- mockery `.Run` callback — строго типизирован: `.Run(func(ctx context.Context, row repository.AuditLogRow))`, не `func(args mock.Arguments)`
- `SeedUser` вызывает `NewUserRepo(s.pool)` до bcrypt hash — mock factory даже если тест ведёт к hash error
- `AssignManager` hash-error ветка для нового пользователя объективно непокрываема unit-тестом: temp password генерируется prod-кодом (12 байт, всегда ок), `bcrypt.ErrPasswordTooLong` недостижим без хака конфигурации. Задокументировано как accepted exception, через coverage 78.4% → теперь 100% через другие ветки

### Слайс 4 — handler

Статус: `done`

- [x] `helpers_test.go`: `withAdminCtx → withRole(userID, role)` — shim не оставлен
- [x] `handler/audit_test.go` — новый; `TestServer_ListAuditLogs` (forbidden + no filters + all filters с JSON payloads + empty + service error) и `TestRawJSONToAny` (nil/empty/valid object/valid array/invalid)
- [x] `handler/test_test.go` — новый; `TestTestHandler_SeedUser/SeedBrand/GetResetToken`; импурсонация проверена через `.Run` callback (`UserIDFromContext` + `RoleFromContext` в ctx для CreateBrand)
- [x] `handler/auth_test.go` — полный rewrite; full `require.Equal` на success-ответы, добавлены error-ветки (Refresh service unauth, ResetPassword service 500, Logout error-still-200, GetMe service 500, RequestPasswordReset service-error-still-200)
- [x] `handler/brand_test.go` — полный rewrite; full `require.Equal` на все success + error-ветки (invalid JSON, not found, list managers error)
- [x] Per-method coverage ≥ 80% на `handler` (пакет — все публичные методы покрыты, включая `HandleParamError` косвенно через тесты невалидного JSON)

**Гочи:**
- Query params в `/audit-logs` именованы в snake_case (`actor_id`, `entity_type`, `entity_id`, `date_from`, `date_to`, `per_page`), не camelCase
- `rawJSONToAny` JSON-декодирует число как `float64` — это стандартное поведение Go encoding/json
- `testapi.HandlerFromMux(h, r)` используется для TestHandler (отдельный роутер, не `api.HandlerWithOptions`)

### Слайс 5 — middleware

Статус: `done`

- [x] `bodylimit_test.go` — сценарий «over limit» проверяет `*http.MaxBytesError` через `errors.As` и `maxErr.Limit`; добавлен «exact limit» (body == N → `io.ReadAll` успешен → 200)
- [x] `auth_test.go` — `fmt.Errorf → errors.New`; `"invalid token"` проверяет полный `api.ErrorResponse`
- [x] `TestAuth_FromScopes` — новый; покрыты обе ветки (no scopes = skip auth / with scopes = enforce) + missing/malformed/invalid token
- [x] Per-method coverage ≥ 80% на `middleware`

### Слайс 6 — финал

Статус: `done`

- [x] `go test -cover` — `/tmp/ugc-coverage-after.txt`
- [x] `make test-unit-backend` зелёный (`-race`, `-count=1`, 5-минутный таймаут)
- [x] `go vet ./...` — 0 issues
- [x] `make lint-backend` — 0 issues (staticcheck SA1029 на `api.BearerAuthScopes` как ключе подавлен через локальный `//nolint:staticcheck`, т.к. это точно тот же ключ, что устанавливает сгенерированный server wrapper)
- [x] `make test-e2e-backend` зелёный (ручной прогон через `sg docker` — все существующие сценарии пройдены)
- [x] `make build-backend` зелёный
- [x] `make test-unit-backend-coverage` зелёный локально (per-method ≥ 80% в `handler/service/repository/middleware/authz`)
- [x] Diff-review: прод-код не тронут, изменения только в `*_test.go` + `go.mod`/`go.sum` (pgxmock), Makefile, CI, стандарты, progress

## Итоговое покрытие

| Пакет | До | После |
|-------|-----|------|
| `handler` | 54.5% | 94.1% |
| `middleware` | 74.5% | 93.4% |
| `repository` | 80.2% | 93.4% |
| `service` | 66.2% | 97.4% |
| `authz` | 100% | 100% |
| `closer` | 100% | 100% |

## Скоуп изменений

**Модифицировано (13 файлов):**
- `docs/standards/backend-testing-unit.md`, `docs/standards/backend-libraries.md`
- `Makefile`, `.github/workflows/ci.yml`
- `backend/go.mod`, `backend/go.sum` (только pgxmock dep)
- `backend/internal/handler/{auth,brand,helpers}_test.go`
- `backend/internal/middleware/{auth,bodylimit}_test.go`
- `backend/internal/repository/{user,brand,audit,helpers}_test.go`
- `backend/internal/service/{auth,brand,token}_test.go`

**Создано (5 файлов):**
- `backend/internal/handler/{audit,test}_test.go`
- `backend/internal/repository/factory_test.go`
- `backend/internal/service/{audit,reset_token_store}_test.go`
- `_bmad-output/planning-artifacts/backend-unit-tests-standards-progress.md` (этот файл)

Прод-код (`*.go` без `_test.go`) — не тронут, mocks (`mocks/*.go`) — не изменились.

## Блокеры / эскалации

- **E-1 — RESOLVED (2026-04-19)**: `AuthService.Refresh` был непокрыт `WithTx` (`ClaimRefreshToken` + `SaveRefreshToken` без атомарности). По твоему решению прод-код обёрнут в `dbutil.WithTx` — `backend/internal/service/auth.go:115-158`. Тесты `TestAuthService_Refresh` переписаны: все 7 сценариев проходят через `pool.Begin()`, добавлен новый сценарий `begin tx error propagates`. `test-e2e-backend` зелёный после фикса. Это единственное изменение прод-кода в задаче.

## Заметки

- Прод-код: одна мини-правка `AuthService.Refresh` (оборачивание в `WithTx`) — по твоей явной просьбе после фикса E-1
- `backend/go.mod` — добавлен pgxmock v4.9.0
- Ассерты логов отложены до рефакторинга логгера (`logger-refactor-brief.md`)
- Комиты — за Alikhan
