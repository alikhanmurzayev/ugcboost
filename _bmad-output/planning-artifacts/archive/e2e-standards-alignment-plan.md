# План реализации: приведение backend E2E тестов к стандартам

**Дата:** 2026-04-19
**Ветка:** `alikhan/staging-cicd`
**Источник:** `_bmad-output/planning-artifacts/e2e-standards-alignment-scout.md`

## Обзор

Рефакторинг `backend/e2e/` под `docs/standards/backend-testing-e2e.md`: нейминг по бизнес-сценарию, composable Setup-хелперы, cleanup stack, HTTP retry, строгие assertions. Покрытие сохраняется полностью. Бэкенд-сторона: удаляем `/test/seed-brand`, вводим `/test/cleanup-entity`.

## Обязательное предварительное действие [BLOCKING]

**Прежде чем писать хотя бы одну строку кода, исполнитель ОБЯЗАН прочитать ВСЕ файлы в `docs/standards/` — полностью, один в один, без фильтрации и без поиска по ним.** Это не просто «ознакомиться» — это полная загрузка содержимого каждого файла в контекст через `Read`. Никаких grep/glob/выборочного чтения — только последовательное чтение всего файла целиком.

Это требование применяется и к стандартам, которые кажутся нерелевантными задаче (frontend-*, backend-codegen, backend-constants, backend-transactions и т.д.). В нашем проекте стандарты часто пересекаются и неявно ссылаются друг на друга — частичное знание приводит к ошибкам в деталях, которые всплывают на ревью.

Список файлов для обязательной загрузки (`docs/standards/`):

- `backend-architecture.md`
- `backend-codegen.md`
- `backend-constants.md`
- `backend-design.md`
- `backend-errors.md`
- `backend-libraries.md`
- `backend-repository.md`
- `backend-testing-e2e.md`
- `backend-testing-unit.md`
- `backend-transactions.md`
- `frontend-api.md`
- `frontend-components.md`
- `frontend-quality.md`
- `frontend-state.md`
- `frontend-testing-e2e.md`
- `frontend-testing-unit.md`
- `frontend-types.md`
- `naming.md`
- `security.md`

**Запрещено:** стартовать Шаг 1 до полной загрузки всех перечисленных файлов. Никаких «прочитаю только backend-testing-e2e.md и naming.md, остальные по месту». Никаких Grep вместо Read. Полное чтение — необходимое условие, не опциональное.

## Требования

### Must-have
- **REQ-1:** Нейминг тестов по бизнес-сценарию — одна функция на endpoint, edge cases через `t.Run`. Итого 47 функций → 12 (`TestHealthCheck` + 6 auth + 3 brand + 2 audit).
- **REQ-2:** Composable Setup-хелперы в `testutil/` (`SetupAdmin`, `SetupBrand`, `SetupManager`, `SetupManagerWithLogin`) вместо `LoginAs*`/`SeedBrand*`.
- **REQ-3:** Создание данных через бизнес-ручки приоритетно. `/test/*` — только для admin seed, raw reset token, cleanup user.
- **REQ-4:** HTTP retry на транспортном уровне через `hashicorp/go-retryablehttp` (connection refused / timeout / DNS / 502/503/504). 4xx и 500 — не retry.
- **REQ-5:** Cleanup stack через `t.Cleanup` (LIFO, FK-safe), управляется `E2E_CLEANUP` env var (default `true`).
- **REQ-6:** Setup-хелперы автоматически регистрируют cleanup.
- **REQ-7:** Удалить `POST /test/seed-brand` из `openapi-test.yaml` и handler. Добавить `POST /test/cleanup-entity` с `{ type: "user"|"brand", id: ... }`.
- **REQ-8:** Strict assertions: проверять динамические поля отдельно, подменять, затем `require.Equal` целиком. Для 4xx — status + `error.code` + `error.message`.
- **REQ-9:** Расширенный header-комментарий в каждом тест-файле.
- **REQ-10:** `testutil.PostRaw(t, path, body)` для запросов в обход типобезопасной валидации сгенерированного клиента (пустой/невалидный email), через тот же retry + CF Access transport.

### Nice-to-have
- Локальные const в `testutil/` (`DefaultPassword`, `RefreshCookieName`, `EnvCleanup`).

### Вне скоупа
- `openapi.yaml` (бизнес-ручки) — без изменений.
- `Makefile` — без изменений (`-race -count=1 -timeout 5m` уже есть, `E2E_CLEANUP` читается напрямую из env).
- Контракт `/test/seed-user` и `/test/reset-tokens` — без изменений.

### Критерии успеха
- `make test-e2e-backend` зелёный после каждого шага.
- `make lint-backend`, `make test-unit-backend` зелёные.
- В `testutil/` нет `LoginAs*`/`SeedBrand*` (кроме низкоуровневых `SeedUser`, `LoginAs`, `GetResetToken`).
- В `auth/`, `brand/`, `audit/` нет вызовов `t.Run` без `t.Parallel()` внутри.

## Файлы для изменения

| Файл | Изменения |
|------|-----------|
| `backend/e2e/testutil/client.go` | `retryablehttp` поверх `cfAccessTransport`; экспорт `NewRawRequest`/`PostRaw` через тот же клиент |
| `backend/e2e/testutil/seed.go` | Удалить `SeedBrand`, `SeedBrandWithManager`, `LoginAsAdmin`, `LoginAsBrandManager`. Добавить `SetupAdmin`, `SetupAdminClient`, `SetupBrand`, `SetupManager`, `SetupManagerWithLogin`. Каждый Setup вызывает `RegisterCleanup` |
| `backend/e2e/go.mod` / `go.sum` | Добавить `github.com/hashicorp/go-retryablehttp` |
| `backend/e2e/auth/auth_test.go` | 28 функций → 6 + `TestHealthCheck`. Все edge cases через `t.Run` с `t.Parallel()`. Расширенный header-комментарий |
| `backend/e2e/brand/brand_test.go` | 14 → 3 (`TestBrandCRUD`, `TestBrandManagerAssignment`, `TestBrandIsolation`) |
| `backend/e2e/audit/audit_test.go` | 5 → 2 (`TestAuditLogFiltering`, `TestAuditLogAccess`) |
| `backend/api/openapi-test.yaml` | Удалить `/test/seed-brand` и связанные схемы. Добавить `/test/cleanup-entity` (body `{ type: enum, id: string }`) + 404 |
| `backend/internal/handler/testapi.go` | Удалить `SeedBrand` handler и `TestAPIBrandService`-зависимость `CreateBrand`/`AssignManager` на сеед-уровне. Добавить `CleanupEntity` handler (диспатч по type → repo delete). Зависимости перекалибровать |
| `backend/internal/handler/testapi_test.go` | Удалить `TestTestAPIHandler_SeedBrand`. Добавить `TestTestAPIHandler_CleanupEntity` (диспатч, 404, валидация) |
| `backend/internal/repository/user.go` | Добавить `UserRepo.DeleteForTests(ctx, id) error` — в одной транзакции чистит `audit_logs WHERE actor_id = $1`, `brand_managers WHERE user_id = $1`, затем `DELETE FROM users WHERE id = $1`. `refresh_tokens` и `password_reset_tokens` уходят каскадом (ON DELETE CASCADE уже есть в `00002_users.sql`). Godoc явно помечает **test-only**, запрет использования из production-кода. Нужна транзакция → метод принимает `dbutil.DB` (как остальные repo-методы), handler оборачивает в `dbutil.WithTx` |
| `backend/internal/repository/user_test.go` | Добавить тест `TestUserRepository_DeleteForTests`: создаём user + brand_manager row + audit_log row + refresh_token → вызываем Delete → все связанные строки исчезли, включая каскадные |
| `backend/cmd/api/main.go` | Перекалибровать `NewTestAPIHandler` вызов: вместо `brandSvc` — cleanup-зависимости (см. Шаг 1) |
| `backend/e2e/apiclient/*.gen.go` | Регенерируется через `make generate-api` (не править руками) |
| `backend/e2e/testclient/*.gen.go` | Аналогично |
| `backend/internal/testapi/server.gen.go` | Аналогично |

## Файлы для создания

| Файл | Назначение |
|------|------------|
| `backend/e2e/testutil/cleanup.go` | `RegisterCleanup(t, fn func(context.Context) error)` — обёртка над `t.Cleanup` с чтением `E2E_CLEANUP`, timeout 10s, cleanup-fail через `t.Logf` (не валит соседние cleanup) |
| `backend/e2e/testutil/raw.go` | `PostRaw(t, path, body)` — HTTP POST на `BaseURL+path` через тот же retry + CF Access transport, без валидации клиента. Узкое исключение для тестов на бэкенд-валидацию |
| `backend/e2e/testutil/const.go` | Локальные const: `DefaultPassword = "testpass123"`, `RefreshCookieName = "refresh_token"`, `EnvCleanup = "E2E_CLEANUP"` |

## Целевая структура тестов

### `auth_test.go` (28 → 7)
| Функция | Edge cases (через `t.Run`) |
|---------|----------------------------|
| `TestHealthCheck` | (без подсценариев) |
| `TestLogin` | empty email 422 (raw), non-existent 401, wrong password 401, short password 401 (no info leak), email normalization (trim+lowercase), success (access token + refresh cookie + HttpOnly + user fields) |
| `TestRefresh` | no cookie 401, rotation chain (login → refresh → refresh), invalidated after logout |
| `TestGetMe` | no token 401, invalid token 401, success (full body) |
| `TestLogout` | no auth 401, success, refresh invalidated after |
| `TestPasswordReset` | request: existing email 200, non-existent 200 (no enumeration), empty email 422 (raw). Execute: success (login new works + old fails), invalid token 401, used token 401 (single-use), short password 422, invalidates refresh tokens |
| `TestFullAuthFlow` | LoginAs → Refresh → GetMe → Logout → Refresh 401, password reset full cycle |

Порядок `t.Run` — сначала валидация/early-exits, в конце happy path.

### `brand_test.go` (14 → 3)
| Функция | Edge cases |
|---------|-----------|
| `TestBrandCRUD` | create: empty name 422, forbidden for manager 403, success (+ in admin list). get: 404, success (full body + managers). update: success. delete: success + subsequent get 404 |
| `TestBrandManagerAssignment` | assign: new user (temp password set), existing user (temp password empty), forbidden for manager 403. remove: success |
| `TestBrandIsolation` | manager list sees own only, admin list sees all, manager get own OK, manager get other 403 |

### `audit_test.go` (5 → 2)
| Функция | Edge cases |
|---------|-----------|
| `TestAuditLogFiltering` | filter by entity: `brand_create` after create brand. filter by action: `manager_assign` after assign. pagination: 3 brands + perPage=2 → 2 rows + total ≥ 3 |
| `TestAuditLogAccess` | manager forbidden 403, admin OK |

## Шаги реализации

Всё одной серией изменений в `alikhan/staging-cicd` без промежуточных коммитов. После каждого шага — локальный `make test-e2e-backend`, рабочее дерево всегда зелёное.

### Шаг 0: загрузка стандартов [BLOCKING, до любого кода]

1. [ ] `Read` каждого файла в `docs/standards/` полностью, один в один:
   - `backend-architecture.md`, `backend-codegen.md`, `backend-constants.md`, `backend-design.md`, `backend-errors.md`, `backend-libraries.md`, `backend-repository.md`, `backend-testing-e2e.md`, `backend-testing-unit.md`, `backend-transactions.md`
   - `frontend-api.md`, `frontend-components.md`, `frontend-quality.md`, `frontend-state.md`, `frontend-testing-e2e.md`, `frontend-testing-unit.md`, `frontend-types.md`
   - `naming.md`, `security.md`
2. [ ] Без `Grep`/`Glob` по этим файлам. Без выборочного чтения. Без пропуска «нерелевантных».
3. [ ] Только после того как все 19 файлов загружены в контекст — переход к Шагу 1.

### Шаг 1: инфраструктура (backend-сторона + testutil без тестов)
1. [ ] `backend/api/openapi-test.yaml`: убрать `/test/seed-brand`, `SeedBrandRequest`, `SeedBrandData`, `SeedBrandResult`. Добавить `/test/cleanup-entity` (POST, body `{ type: "user"|"brand", id: string }`, 204 No Content, 404 если не найдено, 422 валидация).
2. [ ] `backend/internal/repository/user.go`: добавить `DeleteForTests(ctx, id) error`. Схема (подтверждено чтением миграций):
   - `refresh_tokens.user_id` и `password_reset_tokens.user_id` → `ON DELETE CASCADE` (срабатывают сами).
   - `brand_managers.user_id` → без CASCADE (намеренно, защита аудита) → удалить вручную.
   - `audit_logs.actor_id` → без CASCADE (намеренно, целостность аудита) → удалить вручную.
   Реализация — один метод на `UserRepo`, выполняет три SQL в транзакции (принимает `dbutil.DB` как остальные repo-методы; handler оборачивает в `dbutil.WithTx`):
   1. `DELETE FROM audit_logs WHERE actor_id = $1`
   2. `DELETE FROM brand_managers WHERE user_id = $1`
   3. `DELETE FROM users WHERE id = $1`
   Godoc на экспортированном методе (русская суть — английский текст в коде; это обязательная пометка в самом `user.go`, не где-нибудь ещё):
   ```go
   // DeleteForTests hard-deletes the user AND wipes every row that references
   // them: audit_logs (actor_id), brand_managers (user_id), refresh_tokens and
   // password_reset_tokens (both via ON DELETE CASCADE). The whole sequence
   // runs in a single transaction.
   //
   // DANGER: TEST-ONLY. This destroys audit history. NEVER call from business
   // code, from a service, from a handler other than the /test/* cleanup
   // endpoint, or from a migration. Production deletion of users must be a
   // soft delete that preserves audit integrity; use a different method for
   // that. If you are tempted to call this in a real flow — stop and ask.
   ```
   Пометка должна жить в самом `user.go` рядом с сигнатурой — это первая и главная линия защиты от misuse.
3. [ ] `backend/internal/repository/user_test.go`: `TestUserRepository_DeleteForTests` — seed user + brand_manager + audit_log + refresh_token + password_reset_token → вызов → все связанные строки отсутствуют.
4. [ ] `make generate-mocks` (обновить `MockUserRepo`).
5. [ ] `backend/internal/handler/testapi.go`: удалить `SeedBrand`, убрать зависимость `TestAPIBrandService`. Добавить `CleanupEntity` handler: диспатч по `type` → `UserRepo.Delete` / `BrandRepo.Delete`. Новая зависимость — `TestAPICleanupRepo` (узкий интерфейс с обоими Delete).
6. [ ] `backend/cmd/api/main.go`: перекалибровать `NewTestAPIHandler` — вместо `brandSvc` передать cleanup-адаптер над `repoFactory`.
7. [ ] `backend/internal/handler/testapi_test.go`: удалить `TestTestAPIHandler_SeedBrand`. Добавить `TestTestAPIHandler_CleanupEntity` (success user, success brand, invalid type 422, not found 404).
8. [ ] `make generate-api` (регенерация `apiclient/`, `testclient/`, `backend/internal/testapi/server.gen.go`).
9. [ ] `backend/e2e/go.mod`: добавить `github.com/hashicorp/go-retryablehttp` (`go get`, проверить go.sum).
10. [ ] `backend/e2e/testutil/client.go`: retry-transport через `retryablehttp.NewClient()` → `StandardClient()`, обернуть в `cfAccessTransport`. Политика: 3 попытки, экспоненциальный backoff (min 500ms, max 5s), retry на connection refused/timeout/DNS/502/503/504.
11. [ ] `backend/e2e/testutil/cleanup.go`: `RegisterCleanup(t, fn)` — см. scout §5.
12. [ ] `backend/e2e/testutil/raw.go`: `PostRaw(t, path string, body any) *http.Response` — marshal JSON, POST через общий retry-client, возвращает response для проверки тестом. Декоратор `defer resp.Body.Close()` на caller.
13. [ ] `backend/e2e/testutil/const.go`: `DefaultPassword`, `RefreshCookieName`, `EnvCleanup`.
14. [ ] `make test-unit-backend` — проверить что handler/repo unit-тесты зелёные.
15. [ ] `make test-e2e-backend` — e2e пока на старых хелперах, должен быть зелёный (старые `SeedBrand*` ещё на месте в `testutil/seed.go`, т.к. мы их не тронули).

**Важно:** в шаге 1 мы ещё НЕ удалили старые `SeedBrand*` из `testutil/seed.go`, чтобы текущие тесты продолжали работать. После `make generate-api` сгенерированный `testclient/` потеряет `SeedBrandWithResponse` → старые `testutil.SeedBrand` перестанут компилироваться. Значит шаг 1 обязан завершиться переездом `testutil.SeedBrand`/`SeedBrandWithManager` на `POST /brands` (новые внутренние реализации под старыми именами) — минимально инвазивная мера, чтобы E2E оставалось зелёным до шага 2.

### Шаг 2: composable хелперы
1. [ ] `backend/e2e/testutil/seed.go`: добавить:
   - `SetupAdmin(t) token` — `SeedUser` + `LoginAs` + `RegisterCleanup` (через `POST /test/cleanup-entity`).
   - `SetupAdminClient(t) (*Client, token)` — то же + вернуть клиент.
   - `SetupBrand(t, adminToken, name) brandID` — `POST /brands` с admin-токеном + cleanup через `DELETE /brands/{id}`.
   - `SetupManager(t, adminToken, brandID) (email, password)` — `POST /brands/{id}/managers` + cleanup user.
   - `SetupManagerWithLogin(t, adminToken, brandID) (*Client, token, email)`.
2. [ ] Удалить `LoginAsAdmin`, `LoginAsBrandManager`, `SeedBrand`, `SeedBrandWithManager` из `seed.go`.
3. [ ] `make test-e2e-backend` — падает на старых тестах, это ожидаемо → идём в шаг 3.

### Шаг 3: рефакторинг auth_test.go
1. [ ] Объединить 28 функций в 7. Порядок `t.Run` — валидация → happy path.
2. [ ] Для 4xx проверять `error.code` + `error.message` (паттерн из `TestLogin_WrongPassword`).
3. [ ] Динамические поля (UUID, время) проверять отдельно и подменять перед `require.Equal` целостного body.
4. [ ] Использовать `testutil.PostRaw` вместо текущих `http.NewRequest` (в `TestLogin_EmptyEmail`, `TestPasswordResetRequest_EmptyEmail`). Короткий комментарий «почему raw».
5. [ ] Header-комментарий — полное описание flow: что тестируется, какие данные создаются, что ожидаем.
6. [ ] `make test-e2e-backend` — зелёный.

### Шаг 4: рефакторинг brand_test.go
1. [ ] 14 → 3 функций.
2. [ ] Все setup через `SetupAdmin`/`SetupBrand`/`SetupManager` → cleanup автоматический.
3. [ ] Расширить assertions: полный body, managers array, audit-relevant поля.
4. [ ] `make test-e2e-backend` — зелёный.

### Шаг 5: рефакторинг audit_test.go
1. [ ] 5 → 2 функций.
2. [ ] Все setup через новые хелперы.
3. [ ] `make test-e2e-backend` — зелёный.

### Шаг 6: финальная валидация
1. [ ] `make lint-backend` — зелёный.
2. [ ] `make test-unit-backend` — зелёный.
3. [ ] `make test-unit-backend-coverage` — зелёный (`test-unit-backend-coverage` включает `internal/handler` с новым `CleanupEntity`; порог 80%).
4. [ ] `make test-e2e-backend` — зелёный, с `E2E_CLEANUP=true` и с `E2E_CLEANUP=false` (идемпотентность).

## Стратегия тестирования

- **Unit-тесты (handler):** `TestTestAPIHandler_CleanupEntity` — диспатч по type, 404, 422. Обновить `TestTestAPIHandler_SeedUser` если сигнатура `NewTestAPIHandler` меняется.
- **Unit-тесты (repository):** `TestUserRepository_Delete` — success + not found (если метод возвращает ошибку при 0 rows affected).
- **E2E:** рефактор сохраняет покрытие 1:1. Прохождение каждого под-кейса проверяется явным `t.Run`. На CI `E2E_CLEANUP=true`, перед merge локально гоняем `E2E_CLEANUP=false` — проверить идемпотентность при накопленных данных.
- **Линт:** `make lint-backend` (golangci-lint) после каждого шага.

## Оценка рисков

| Риск | Вероятность | Митигация |
|------|-------------|-----------|
| Сгенерированный клиент ломается после `make generate-api` → e2e не компилируется до шага 2 | Высокая | В шаге 1 мы сохраняем имена `SeedBrand`/`SeedBrandWithManager` в `testutil/seed.go`, но внутри уже через `POST /brands`. Старые тесты компилируются до шага 2 |
| Кто-то вызовет `UserRepo.DeleteForTests` из бизнес-кода → аудит потерян | Средняя | Явный DANGER-godoc в `user.go` (см. шаг 1 пункт 2). Нейминг `...ForTests` плюс регистрация зависимости — только в `TestAPIHandler` (который сам поднимается только при `ENVIRONMENT != production`). Если потребуется дополнительный барьер — опциональный runtime-guard: паника если `os.Getenv("ENVIRONMENT") == "production"` |
| `retryablehttp` ретраит 4xx/500 по умолчанию | Средняя | Кастомный `CheckRetry`: retry только на transport errors + 502/503/504. Unit-тест политики не пишем (проверяется интеграционно) |
| Рефакторинг 47 → 12 функций → большой diff → регрессия скрытого edge case | Средняя | После каждого шага `make test-e2e-backend`. Не объединять функции бездумно — сохранить все assertions, в т.ч. `HttpOnly` flag на refresh cookie, `temp password empty for existing user`, no-enumeration на password reset |
| `t.Parallel()` в под-`t.Run` → deadlock если forgot в вложенном | Низкая | Каждый `t.Run` явно начинается с `t.Parallel()`. Проверим через `grep -B1 't.Run(' backend/e2e/**/*_test.go` |
| Cleanup-gap: если тест падает до регистрации cleanup → мусор в БД | Низкая | `RegisterCleanup` вызывается сразу после успешного create в Setup-хелперах. На CI `E2E_CLEANUP=true`. Для мусора в dev БД — `make migrate-reset` |
| `PostRaw` не получает CF Access headers → e2e падает в staging | Средняя | `PostRaw` использует тот же общий retry-client, что и сгенерированный apiclient → headers ставятся автоматически |
| Переход на бизнес-ручку `POST /brands` в setup → E2E заметно медленнее | Низкая | Внутри одного теста `SetupAdmin` шарится между `SetupBrand`. Если на CI деградация > 30% — обсуждается отдельно |

## План отката

- Вся работа в ветке `alikhan/staging-cicd` — никаких изменений в `main`.
- Если после любого шага e2e красный и не чинится быстро — `git reset --hard HEAD` на предыдущий зелёный коммит (промежуточные коммиты не делаем, но `git stash` можем использовать для инкрементальной проверки).
- `openapi-test.yaml` и сгенерированные `*.gen.go` восстанавливаются через `git checkout`.
- `UserRepo.Delete` — аддитивное изменение, миграций БД не требует (только Go-код).

## Стандарты, под которые выравниваем

- `docs/standards/backend-testing-e2e.md` — структура e2e тестов, cleanup, retry
- `docs/standards/naming.md` — нейминг функций и файлов
- `docs/standards/backend-libraries.md` — `retryablehttp` вместо самописного ретрая
- `docs/standards/backend-design.md` — DI через конструктор (новая зависимость handler)
- `docs/standards/security.md` — `/test/*` только при `ENVIRONMENT != production` (уже на месте)
- `docs/standards/backend-errors.md` — все ошибки возвращаются/логируются, cleanup-fail — `t.Logf` (не валит соседние cleanup)
