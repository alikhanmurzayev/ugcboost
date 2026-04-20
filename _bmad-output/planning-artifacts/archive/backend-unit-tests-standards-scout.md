# Разведка: приведение unit-тестов бэкенда к стандартам

## ⚠️ Обязательная преамбула для следующей команды (plan/build)

**Перед любой работой по этому артефакту — полностью загрузи в контекст ВСЕ 19 файлов стандартов из `docs/standards/`, НЕ только backend-testing-unit.md и не только «релевантные»:**

```
docs/standards/backend-architecture.md
docs/standards/backend-codegen.md
docs/standards/backend-constants.md
docs/standards/backend-design.md
docs/standards/backend-errors.md
docs/standards/backend-libraries.md
docs/standards/backend-repository.md
docs/standards/backend-testing-e2e.md
docs/standards/backend-testing-unit.md
docs/standards/backend-transactions.md
docs/standards/frontend-api.md
docs/standards/frontend-components.md
docs/standards/frontend-quality.md
docs/standards/frontend-state.md
docs/standards/frontend-testing-e2e.md
docs/standards/frontend-testing-unit.md
docs/standards/frontend-types.md
docs/standards/naming.md
docs/standards/security.md
```

Стандарты переплетены: `backend-testing-unit.md` оперирует терминами из `backend-repository.md` (Row-structs, dbutil.DB, константы колонок), `backend-transactions.md` (WithTx, RepoFactory), `backend-codegen.md` (`api/`-типы в handler-тестах, mockery), `naming.md` (`Test{Struct}_{Method}`), `backend-constants.md` (SQL-литералы в тестах — намеренно). Без чтения всех стандартов решения будут локально правильные, но системно неверные.

---

## Задача 1: Понимание запроса

**Что просит пользователь.** Привести весь unit-тест backend в соответствие `docs/standards/backend-testing-unit.md` и связанным стандартам. Контекст: только что завершён большой рефакторинг `handler/service` (коммит `962edf5`, см. `_bmad-output/planning-artifacts/handler-service-refactor-progress.md`). В прогресс-артефакте явно признано: «`backend-testing-unit.md` призывает `require.WithinDuration+zero+require.Equal` для всех динамических полей. Частично ... expiresAt и другие поля — нет». То есть рефакторинг handler/service завершён, а тесты подтянуты не до конца.

**Какие области затронуты.**

- `backend/internal/service/**/*_test.go` (4 файла)
- `backend/internal/handler/**/*_test.go` (3 файла + 3 пропущенных)
- `backend/internal/repository/**/*_test.go` (4 файла)
- `backend/internal/middleware/**/*_test.go` (6 файлов)
- `backend/internal/authz/**/*_test.go` (2 файла)
- `backend/internal/closer/*_test.go` (1 файл)
- Сам прод-код (`handler/service/repository`) **не меняем** — он уже соответствует стандартам после последнего рефакторинга. Задача — подтянуть только тесты.

**Зависимости.** Моки `mockery` автосгенерированы (`make generate-mocks`). После изменения интерфейсов обязательно регенерировать. Утилиты тестов: `helpers_test.go` в каждом пакете (handler, repository, service), `testutil/`.

---

## Задача 2: Исследование кодовой базы

### Инвентаризация существующих unit-тестов

**service/ (4 файла):**
- `auth_test.go` (299 строк) — TestAuthService_{Login,Refresh,Logout,ResetPassword,SeedAdmin}
- `brand_test.go` (367 строк) — TestBrandService_{CreateBrand,ListBrands,UpdateBrand,DeleteBrand,AssignManager,RemoveManager,IsUserBrandManager}
- `token_test.go` (137 строк) — TestTokenService_{GenerateAccessToken,ValidateAccessToken,GenerateRefreshToken,GenerateResetToken,HashToken}
- `helpers_test.go` — stub `testTx` для `pgx.Tx`

**handler/ (3 файла):**
- `auth_test.go` (295 строк) — TestServer_{Login,RefreshToken,RequestPasswordReset,ResetPassword,Logout,GetMe}
- `brand_test.go` (291 строк) — TestServer_{CreateBrand,ListBrands,GetBrand,UpdateBrand,DeleteBrand,AssignManager,RemoveManager}
- `helpers_test.go` — `newTestRouter`, `doJSON[Resp]`, `withAdminCtx`

**repository/ (4 файла):**
- `user_test.go`, `brand_test.go`, `audit_test.go`, `helpers_test.go` (captureQuery/captureExec + scalarRows)

**middleware/ (6 файлов):**
- `auth_test.go`, `cors_test.go`, `recovery_test.go`, `secure_headers_test.go`, `bodylimit_test.go`, `client_ip_test.go`

**authz/ (2 файла):**
- `brand_test.go`, `audit_test.go`

**closer/** (1 файл): `closer_test.go`

### Отсутствующие тест-файлы

| Файл | Прод-код | Нужен? |
|------|----------|--------|
| `handler/audit_test.go` | `handler/audit.go` (ListAuditLogs + rawJSONToAny) | **Да** — endpoint не покрыт unit-тестами |
| `handler/health_test.go` | `handler/health.go` | Нет (тривиальный статический ответ) |
| `handler/test_test.go` | `handler/test.go` (TestHandler) | **Да** — публичные ручки, зависят от моков AuthService/BrandService/TokenStore |
| `service/audit_test.go` | `service/audit.go` (AuditService.List + `writeAudit`/`contextWithActor`) | **Да** — `writeAudit` и `List` без покрытия |
| `service/reset_token_store_test.go` | `service/reset_token_store.go` (InMemoryResetTokenStore) | Да (простой, но есть concurrency — `sync.RWMutex`) |
| `middleware/logging_test.go` | `middleware/logging.go` | Нет (обёртка над slog) |
| `middleware/json_test.go` | `middleware/json.go` | Нет (внутренний хелпер, покрыт транзитивно) |

---

## Задача 3: Отчёт — затронутые области, паттерны, риски

### Затронутые файлы

**Требуют доработки (не переписывания):**
- `backend/internal/service/auth_test.go`
- `backend/internal/service/brand_test.go`
- `backend/internal/service/token_test.go`
- `backend/internal/handler/auth_test.go`
- `backend/internal/handler/brand_test.go`
- `backend/internal/repository/user_test.go`
- `backend/internal/repository/brand_test.go`
- `backend/internal/repository/audit_test.go`
- `backend/internal/middleware/bodylimit_test.go`
- `backend/internal/middleware/auth_test.go`

**Создать с нуля:**
- `backend/internal/handler/audit_test.go`
- `backend/internal/handler/test_test.go`
- `backend/internal/service/audit_test.go`
- `backend/internal/service/reset_token_store_test.go`

### Паттерны реализации (как сейчас vs как по стандарту)

#### Repository layer — **самое большое отставание**

**Стандарт `backend-testing-unit.md`:**
> Repository. SQL assertion: точная SQL-строка + аргументы. **Проверяем маппинг row → struct**: мокаем `rows.Scan()`. **Проверяем error propagation**: ошибки БД оборачиваются с контекстом.

**Сейчас.** Каждый тест содержит **только один `t.Run("SQL")`** с SQL assertion, возвращая фиктивную ошибку из `captureQuery/captureExec`. Success path (маппинг) и error wrapping **не покрыты**. Пример: `TestBrandRepository_GetByID` проверяет SQL, но не проверяет, что `GetByID` корректно возвращает `*BrandRow` c заполненными полями или что `sql.ErrNoRows` корректно пробрасывается.

**Что нужно.** Для каждого repo-метода — минимум 3 `t.Run`:
1. `"SQL"` — существующий (SQL-assertion через captureQuery/captureExec)
2. `"maps row to struct"` — подсунуть `pgx.Rows` (расширить `scalarRows`/сделать отдельный `rowsMock`), вернуть валидный ряд, проверить что все поля структуры заполнены корректно
3. `"propagates error"` / `"wraps error"` — для методов, которые оборачивают ошибку (например, `ExistsByEmail` — проверить что generic ошибка прокидывается, `sql.ErrNoRows` — что выдаётся именно `sql.ErrNoRows` а не pgx)

#### Service layer — **умеренное отставание**

**Стандарт:**
> Моки на все зависимости. Точные аргументы. **Порядок вызовов моков проверяется**. Динамические поля — через `require.WithinDuration` + подмена + `require.Equal` целиком.

**Сейчас.**
- ✅ Моки и изоляция (`t.Parallel` в `t.Run`, новый мок на каждый сценарий) — на месте
- ⚠️ `mock.Anything` для ctx везде, хотя стандарт приводит пример `mock.AnythingOfType("*context.valueCtx")`. Это soft-отклонение
- ⚠️ `mock.MatchedBy(func(row repository.AuditLogRow) bool { return row.Action == AuditActionLogin })` — неполная проверка. Стандарт: «точные аргументы каждого вызова» и для сложных — `.Run(...)` с приведением и `require`. Сейчас check по 2 полям из 8, остальные (`EntityType`, `EntityID`, `IPAddress`, `NewValue`, `OldValue`) не проверяются
- ❌ Порядок вызовов моков не контролируется (mockery + testify по умолчанию не ordered). Стандарт явно: *«если сервис сохранит токен до проверки пароля — тест должен упасть»*. Сейчас такая регрессия пройдёт незамеченной
- ⚠️ Сценарии не следуют «порядку исполнения» строго — в `TestAuthService_Login` есть success, wrong password, user not found (3 сценария), но нет token generation error, save refresh token error, audit error. Стандарт требует «стремимся к высокому покрытию — каждая ветка кода покрыта»
- ❌ `TestAuthService_Refresh` — отсутствует «wrong user not found», «token generation error», «save refresh token error»
- ❌ `TestAuthService_Logout` — только один success; нет error от `DeleteUserRefreshTokens` и от audit
- ❌ `TestAuthService_ResetPassword` — только success + invalid token; нет `UpdatePassword error`, `hash password error`, `DeleteUserRefreshTokens error`
- ❌ Нет теста `TestAuthService_RequestPasswordReset` (публичный метод без покрытия!)
- ❌ Нет теста `TestAuthService_GetUser`, `TestAuthService_GetUserByEmail`, `TestAuthService_SeedUser` (новый метод из Stage 6)
- ⚠️ `TestBrandService_ListBrands` — нет error case
- ⚠️ `TestBrandService_AssignManager "new user"` — не проверено, что temp password реально bcrypt-совместим; не проверена форма аргумента `Create(ctx, email, hashString, "brand_manager")` — `mock.AnythingOfType("string")` не заменяет проверку того, что пароль bcrypt-хэширован
- Audit payload (`NewValue`) нигде не проверяется — хотя стандарт говорит «точные аргументы»

#### Handler layer — **лёгкое отставание**

**Стандарт:**
> Response — `json.Unmarshal` в сгенерированную структуру → динамика отдельно → подмена → `require.Equal` целиком.

**Сейчас.**
- ✅ `newTestRouter` + `doJSON[Resp]` + ServerInterfaceWrapper — образцовые хелперы
- ✅ Request/response только через `api/*`-типы — соответствует `backend-codegen.md`
- ⚠️ `require.Equal` целиком есть в:
  - `TestServer_Login success` (response), `TestServer_AssignManager new user` (resp после обнуления `TempPassword`), `TestServer_GetMe success` — ✅
  - Остальные тесты — только `require.Equal(t, "b-1", resp.Data.Id)` по 1-2 полям
- ⚠️ `TestServer_Login success` не подменяет access token (он не динамический — статически "jwt-token"), но не проверяет весь envelope (`refresh_token` не в body, а в cookie — cookie проверяется корректно, ✅)
- ❌ `TestServer_ListBrands` / `TestServer_UpdateBrand` / `TestServer_DeleteBrand` / `TestServer_GetBrand success` — не `require.Equal` целиком
- ❌ Нет `TestServer_ListAuditLogs` (нет файла вообще)
- ❌ Нет тестов на `TestHandler` (`SeedUser`, `SeedBrand`, `GetResetToken`) — бесконтрольный код, пусть и test-only
- ⚠️ `withAdminCtx` использует `api.Admin` — ок, но лучше параметризовать (`withRole(role, userID)`) для повторного использования

#### Middleware layer — **минимальные правки**

- ✅ `auth_test.go`, `cors_test.go`, `recovery_test.go`, `secure_headers_test.go`, `client_ip_test.go` — хорошее покрытие (context, status, headers, ошибки)
- ❌ `bodylimit_test.go "over limit"` — пустой assertion на response (status не проверяется, response body не проверяется). Нужно проверить что http.MaxBytesReader → 413/корректная ошибка в handler
- ⚠️ `auth_test.go "invalid token"` — ошибку мока `v.EXPECT().ValidateAccessToken("bad-token").Return("", "", fmt.Errorf("expired"))` — но `fmt.Errorf` лучше заменить на `errors.New`, и `ErrorContains` на вывод в response (сейчас только статус проверяется)

#### authz/closer/token — **в порядке**

- `authz/brand_test.go` и `audit_test.go` — соответствуют стандарту, включая удобный table-driven `TestAuthzService_AdminOnlyBrandActions`
- `closer_test.go` — покрыты LIFO, все вызываются на ошибке, первая ошибка возвращается, пустой case, context пробрасывается
- `token_test.go` — покрытие хорошее, но можно добавить проверку claims для GenerateAccessToken (subject, roles) отдельно от ValidateAccessToken round-trip

### Конвенции из смежных стандартов

1. **`backend-constants.md`:** SQL-строки в repo-тестах пишутся **литералами намеренно** (двойная проверка). Не заменять на константы. ✅ сейчас соблюдается
2. **`backend-codegen.md`:** моки — mockery (с `all: true`), ручных моков нет — ✅; handler-тесты используют `api/*`-типы — ✅
3. **`backend-repository.md`:** репозитории возвращают `sql.ErrNoRows`, не `pgx.ErrNoRows` — тесты должны проверять именно `sql.ErrNoRows`
4. **`backend-transactions.md`:** в service-тестах `pool.EXPECT().Begin(mock.Anything).Return(testTx{}, nil)` — `testTx` корректный заглушечный `pgx.Tx`. Паттерн повторяется — оставить
5. **`backend-errors.md`:** невалидный ввод → ошибка, не fallback. В handler-тестах валидации (empty email, short password) → 422 — соблюдается
6. **`naming.md`:** `Test{Struct}_{Method}` — соблюдается. Для middleware — `Test{Func}_{Variant}` (напр. `TestAuth_ValidateAccessToken`) — допустимо
7. **`backend-design.md`:** зависимости через конструктор, иммутабельны. В тестах всегда `NewXxxService(...)` — ✅
8. **`security.md`:** логирование не должно утекать sensitive data. В `TestAuthService_ResetPassword` убедиться что в аудит-payload не попадает пароль/хэш (нигде не проверяется!)

### Паттерны тестирования, которые уже устоялись и должны сохраниться

- **`newTestRouter` + `doJSON[Resp]`** (handler/helpers_test.go) — образец, повторно использовать для audit/test handler тестов
- **`captureQuery` / `captureExec` + `scalarRows`** (repository/helpers_test.go) — образец; расширить для success-маппинга (например, `queryRows(rows pgx.Rows)` хелпер, возвращающий заранее подготовленные `pgx.Rows`)
- **`testTx`** (service/helpers_test.go) — минимальная stub `pgx.Tx`, оставить как есть
- **`ctxWithRole`** (authz/brand_test.go) — образец для middleware context
- **Table-driven** в authz — применяется там, где структура сценариев однородная. Можно расширить на admin-only handler-тесты в brand_test.go (`TestServer_*` для forbidden-ветки)

---

## Риски и соображения

### Функциональные риски

1. **Регенерация моков.** Если план потребует добавить методы в интерфейсы (напр. TokenStore, TestAuthService) — обязательно `make generate-mocks`. Ручные правки mocks/ запрещены по `backend-codegen.md`
2. **`require.Equal` целиком может сломать существующие passing-тесты**, если в prod-коде есть поля, которые тест не заполняет. Порядок работ: сначала фиксируем поведение через точечные assertions, затем переводим на `Equal`-целиком (по стандарту)
3. **Порядок моков (`InOrder`).** testify/mock поддерживает `.NotBefore(...)` и ordered calls. При добавлении проверки порядка не сломать существующие success-тесты
4. **`sql.ErrNoRows` vs generic error.** Сейчас `captureQuery` возвращает `errors.New("mock: query intercepted")` — для новых тестов success path нужно возвращать валидный `pgx.Rows`. Для тестов, где код проверяет `errors.Is(err, sql.ErrNoRows)` — хелпер должен возвращать именно `sql.ErrNoRows` (иначе ветка останется непокрытой)

### Edge cases, которые сейчас отсутствуют

- `AuthService.ResetPassword`: bcrypt cost слишком высок (`MinCost=4` используется — ok для тестов, но нужно verify no `bcrypt.ErrPasswordTooLong` path)
- `AuthService.Refresh`: rotation race — сейчас test не проверяет, что старый refresh token **удаляется** (это ответственность `ClaimRefreshToken`), но логика rotation не покрыта end-to-end на unit-уровне
- `BrandService.AssignManager`: что если email валидный, но юзер есть в другой роли (не `brand_manager`)? Прод-код просто читает его — тест на этот кейс отсутствует
- `AuditService.List`: нет unit-теста на пагинацию/мэппинг `AuditLogRow → domain.AuditLog`
- `TestHandler.SeedBrand`: impersonation adminID — если adminID пустой, `AssignManager` создаст запись с пустым actor — FK-нарушение. Тест отсутствует

### Вопросы безопасности

- Все тесты используют `bcrypt.MinCost` через `testBcryptCost` — ок (стандарт прямо это не покрывает, но в production config идёт из env var)
- `TestAuthService_ResetPassword` — не проверяется, что после успеха все прошлые refresh_tokens для user **удалены** (стандарт `security.md` о sessions)
- `TestHandler` тесты должны убедиться, что **prod-ручки не регистрируют `SeedUser`/`SeedBrand`** — это e2e-ответственность, но в unit можно проверить, что `/test/*` маршруты не добавляются в prod-router (эта логика в `main.go`, unit-тест тут не сработает)

---

## Рекомендуемый подход

### Порядок работ (вертикальными слайсами, по одному слою за раз)

Делать ровно **в этом порядке**, потому что каждый слой опирается на чистоту моков из нижнего:

**Слайс 0 — подготовка (≈30 мин):**
1. Прочесть все 19 стандартов в контекст (обязательно!)
2. `make test-unit-backend` зелёный — baseline
3. `go test -cover ./internal/... > coverage-before.txt` — зафиксировать стартовое покрытие

**Слайс 1 — repository (самый большой дефицит):**
1. Расширить `repository/helpers_test.go`: добавить `queryRows(t, db, rows)` → возвращает настоящий `pgx.Rows` стаб с заданными данными
2. Для каждого `*Row` собрать тест: `"SQL"` + `"maps row to struct"` + `"propagates sql.ErrNoRows as-is"` (для методов с `GetBy*`) + `"wraps other errors with context"` (где `fmt.Errorf("...: %w", err)` в коде)
3. `user_test.go`, `brand_test.go`, `audit_test.go` — прогнать все методы

**Слайс 2 — service:**
1. Для каждого `*Service` метода обеспечить:
   - Все ветки ошибок (happy + 2-5 error cases по ветвлениям в коде)
   - `mock.MatchedBy` заменить на `.Run(func(args mock.Arguments) { row := args.Get(1).(repository.AuditLogRow); require.Equal(t, repository.AuditLogRow{...}, row) })` с подменой динамики (`CreatedAt=time.Time{}`)
   - Порядок вызовов через `.NotBefore(prev)` или ordered mock
2. Добавить полностью новый `service/audit_test.go` (List + writeAudit + contextWithActor)
3. Добавить `service/reset_token_store_test.go` (включая parallel writes через `t.Parallel()` внутри goroutine — проверка на race)
4. Дописать в `auth_test.go` и `brand_test.go` недостающие сценарии (Refresh errors, Logout errors, ResetPassword errors, RequestPasswordReset, GetUser, GetUserByEmail, SeedUser, AssignManager edge cases)

**Слайс 3 — handler:**
1. `require.Equal` целиком для всех `TestServer_*` success-тестов (подмена динамики по паттерну Login/AssignManager)
2. Создать `handler/audit_test.go` (ListAuditLogs — admin allowed, manager forbidden, фильтры, пагинация, empty, rawJSONToAny success/unmarshal failure)
3. Создать `handler/test_test.go` (SeedUser, SeedBrand с/без managerEmail, GetResetToken success/not found, impersonation context)
4. Параметризовать `withAdminCtx` → `withRole(userID, role)`

**Слайс 4 — middleware docpolish:**
1. `bodylimit_test.go` — полные assertions (status, body)
2. `auth_test.go` — `errors.New` вместо `fmt.Errorf`; проверить error response body целиком

**Слайс 5 — финал:**
1. `go test -cover ./internal/... > coverage-after.txt` — target 80%+, сравнить с before
2. `make test-unit-backend` — зелёный
3. `make lint-backend` — 0 issues
4. `make test-e2e-backend` — зелёный (подстраховка, что unit-правки не сломали контракт)
5. Оформить коммит: `test(backend): align unit tests with standards` с кратким списком слайсов

### Ключевые решения

1. **Не трогать прод-код.** Задача «привести тесты к стандартам». Если в процессе обнаружится, что тест не написать без изменения прод-кода (например, нужен новый публичный метод) — **остановиться и поднять вопрос**, не делать самовольно
2. **Success-path для repo.** Не тащить `pgxmock` как библиотеку — stay with in-package `scalarRows`-style стабами. Это дешевле, явнее, и стандарт прямо говорит «Mock: MockDB (pgx interface)»
3. **`require.Equal` целиком** — выполнять строго по алгоритму стандарта: проверили динамику → обнулили → `require.Equal` на целую структуру. Не обнулять через reflection, только явным присваиванием `resp.Data.Field = nil` — как в `TestServer_AssignManager`
4. **Порядок моков через `.NotBefore(...)`** в testify/mock — поддерживается нативно. Избегать самописного трекинга
5. **Для repo success-mapping не плодить unit-тесты до паранойи.** Маппинг всех полей тестировать один раз на `GetByID` (full row), для List — только что столбцы считываются в правильном порядке. Иначе переполнение кода дубликатами

### Альтернативы (отвергнуты)

- **pgxmock вместо scalarRows.** Добавляет dep, нарушает «Libraries vs велосипеды только для infra», а тесты — это не infra. Отказ
- **Testcontainers для repo-тестов (integration вместо unit).** `backend-testing-e2e.md` и так покрывает это на E2E уровне. В unit-слое — моки. Отказ
- **Ручная проверка порядка через счётчик.** testify/mock уже умеет `.NotBefore(...)`. Не изобретать. Отказ

---

## Итог

- **Масштаб работы:** не «переписать всё», а системно поднять существующие тесты до стандарта + создать 4 недостающих файла. Ориентировочно ~600-900 строк новых/изменённых тестов.
- **Риск для прод-кода:** низкий — трогаем только `*_test.go` + `mocks/` (через `make generate-mocks`)
- **Гарантия качества:** после слайса 5 все три цели (`test-unit`, `lint`, `test-e2e`) зелёные + покрытие ≥ 80% + явная проверка всех пунктов из `backend-testing-unit.md`

**Готов приступить к реализации, или нужно исследовать что-то ещё?**
