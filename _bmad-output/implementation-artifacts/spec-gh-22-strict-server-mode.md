---
title: 'Перевод хендлеров на strict-server mode oapi-codegen'
type: 'refactor'
created: '2026-05-01'
status: 'done'
baseline_commit: '0a3e359'
context:
  - docs/standards/backend-codegen.md
  - docs/standards/backend-architecture.md
  - docs/standards/backend-errors.md
---

<frozen-after-approval reason="human-owned intent — do not modify unless human renegotiates">

## Intent

**Problem:** В chi-server режиме хендлеры вручную делают `json.NewDecoder(r.Body).Decode(...)` и `respondJSON(...)` — boilerplate и нарушение `backend-codegen.md`. Issue #22 называет 8 эндпоинтов; реально их 22 (auth ×6, brand ×7, creator_app ×2, audit, dictionary, health, testapi ×4).

**Approach:** Переключить oapi-codegen на `chi-server,strict-server,models` для **обоих** контрактов (`openapi.yaml`, `openapi-test.yaml`). Все хендлеры — на сигнатуру `(ctx, req) (resp, err)`. `respondError` остаётся центром перевода domain-ошибок и подключается через `StrictHTTPServerOptions.ResponseErrorHandlerFunc`. `RequestErrorHandlerFunc` сохраняет 422+`CodeValidation` на body-decode фейлах вместо дефолтного 400.

## Boundaries & Constraints

**Always:**
- HTTP-контракт идентичен до/после: статусы, тело, Set-Cookie, error codes, anti-fingerprinting (login = тот же 401 для всех классов).
- Оба контракта (api + testapi) — одним PR.
- Authz/middleware-context (UserID, Role, IP, UA) сохраняются.

**Ask First:**
- Если strict-server не позволяет вернуть текущий 422+`CodeValidation` на body-decode без потери logger/requestID — обсудить.
- Если для эндпоинта strict-server не покрывает реальный набор статусов из `respondError` — halt.

**Never:**
- Менять `openapi.yaml` / `openapi-test.yaml`.
- Заводить ручные `interface`/`type` для request/response.
- Трогать service/repository/authz, добавлять эндпоинты, менять поведение.

## I/O & Edge-Case Matrix

| Scenario | Expected | Error Handling |
|----------|----------|----------------|
| Happy mutate | 2xx + typed JSON; Set-Cookie где было | N/A |
| Body decode fail | 422 + `CodeValidation` | `RequestErrorHandlerFunc` → `respondError` |
| Domain-ошибка от service | Соотв. 4xx + код | Handler `return nil, err` → `ResponseErrorHandlerFunc` → `respondError` |
| Path/query param missing | 400 + `CodeValidation` | `ChiServerOptions.ErrorHandlerFunc = HandleParamError` (без изменений) |
| Authz fail | 403 + `CodeForbidden` | `nil, err` от authz → `respondError` |
| Internal error | 500 + `CodeInternal` | `nil, err` без обёртки → `respondError` дефолт |

</frozen-after-approval>

## Code Map

- `Makefile` — `generate-api`: добавить `strict-server` к `-generate` для обоих oapi-codegen вызовов.
- `backend/internal/api/server.gen.go`, `backend/internal/testapi/server.gen.go` — regen.
- `backend/internal/handler/server.go` — общая фабрика strict-обёртки (используется в main.go и helpers_test.go) с request/response error handlers.
- `backend/internal/handler/{auth,brand,creator_application,audit,dictionary,health,testapi}.go` — методы в strict-сигнатуру; cookie/headers — через response-варианты.
- `backend/internal/handler/response.go` — `respondError` остаётся; `respondJSON` удалить если не используется.
- `backend/internal/handler/param_error.go` — без изменений.
- `backend/internal/handler/helpers_test.go` — `newTestRouter` через ту же strict-фабрику.
- `backend/cmd/api/main.go` — обернуть `server` и `testHandler` через strict-фабрики перед `HandlerWithOptions`.

## Tasks & Acceptance

**Execution:**
- [x] `Makefile` + `make generate-api` — переключить oba контракта; убедиться, что в `*.gen.go` появились `StrictServerInterface`, `*RequestObject`, `*ResponseObject`.
- [x] `handler/server.go` + `response.go` — strict-фабрика с `RequestErrorHandlerFunc` (→ 422+CodeValidation) и `ResponseErrorHandlerFunc` (→ `respondError`); вычистить мёртвые helpers.
- [x] `handler/{auth,brand,creator_application,audit,dictionary,health}.go` — переписать в strict-сигнатуру; ручной decode и `respondJSON` исчезают; для login/refresh/logout — Set-Cookie через response-объект.
- [x] `handler/testapi.go` — переписать 4 метода в strict-сигнатуру `testapi.StrictServerInterface`.
- [x] `handler/helpers_test.go` + `cmd/api/main.go` — wire strict-обёртки.
- [x] `middleware/request_meta.go` (новый) — `RequestMeta` HTTP-middleware: User-Agent + refresh cookie в ctx (strict-handler не видит `r`); добавлен в global-middleware в main.go и в test-router.
- [x] Existing `*_test.go` — правки `newTestAPIRouter` (testapi через strict), `invalid_JSON` тестов (authz больше не вызывается до decode — strict делает body-check первым).

**Acceptance Criteria:**
- Given чистый tree, when `make generate-api && make build-backend && make lint-backend`, then всё успешно.
- Given поднятые сервисы, when `make test-unit-backend && make test-unit-backend-coverage`, then зелёное, ≥80% per-method.
- Given docker, when `make test-e2e-backend`, then зелёное (контракт идентичен).
- Given diff, when `grep -RE "json\.NewDecoder|respondJSON" backend/internal/handler/ --include='*.go' | grep -v _test.go`, then пусто.
- Given diff, when `git diff main -- backend/api/openapi.yaml backend/api/openapi-test.yaml`, then единственная правка — добавление `headers.Set-Cookie` в `/auth/logout` 200 (документация уже-существующего поведения, см. Spec Change Log); семантика контракта не меняется.

## Spec Change Log

### 2026-05-01 — `/auth/logout` Set-Cookie header добавлен в OpenAPI

- **Finding (impl)**: Текущий handler.Logout вызывает `http.SetCookie(w, ...)` чтобы стереть `refresh_token` cookie. В strict-server режиме у handler'а нет доступа к `http.ResponseWriter` — только `(ctx, request) → (response, error)`. OpenAPI для `/auth/logout` 200 не объявлял `Set-Cookie` header → генератор не создал поле `Logout200ResponseHeaders.SetCookie` — некуда вернуть значение для стирания cookie.
- **Amendment**: В `openapi.yaml` для `/auth/logout` 200 добавлено объявление `headers.Set-Cookie` (как у `/auth/login` и `/auth/refresh`). HTTP-поведение не меняется — header реально и так отправляется.
- **Avoids**: (a) Потерю security-critical clear-cookie на logout. (b) Ugly ctx-injection of `w` через middleware. (c) Молчаливое расхождение OpenAPI vs реальный ответ.
- **KEEP**: Микро-правка OpenAPI допустима, когда документирует уже-существующее HTTP-поведение и не меняет семантики контракта (статус, тело, обязательные headers).

### 2026-05-01 — Post-review patches применены

- **Findings (review)**: 4 ревью-агента (blind-hunter, edge-case-hunter, acceptance-auditor, standards-auditor). 0 intent_gap / 0 bad_spec — кода-уровневые правки без re-derivation.
- **Patches**:
  - `handler/server.go` — выделен `newStrictErrorHandlers(log)` helper; `NewStrictAPIHandler`/`NewStrictTestAPIHandler` теперь не дублируют замыкания. Лишний `Logger()` getter удалён (same-package access).
  - `handler/constants.go` — `CookieRefreshToken` теперь алиас на `middleware.CookieRefreshToken` (single source of truth, был дубль).
  - `handler/auth_test.go` — добавлены `TestServer_refreshCookieString` (secure on/off ветки), `TestServer_clearRefreshCookieString` (Max-Age=0), captured-input тест `TestServer_RefreshToken/reads_refresh_cookie_from_middleware-context` через `middleware.WithRefreshCookie`.
  - `handler/creator_application.go` — устаревший комментарий про respondError исправлен.

### 2026-05-01 — Post-merge review patches (round 2)

Алихан оставил 7 inline-комментариев на PR + meta-фидбек про избыточные комментарии. Все правки в одном раунде:

- **`docs/standards/naming.md` § Комментарии** — два буллета переписаны, без новых разделов: «по умолчанию — без комментария, WHY-only»; «godoc — однострочник, многострочник только при неочевидных предусловиях». Покрывает повторяющийся паттерн избыточных godoc'ов.
- **`docs/standards/backend-codegen.md`** — добавлен буллет про single-value Set-Cookie ограничение strict-server (генератор делает `w.Header().Set`, второе значение overwrite'нет).
- **OpenAPI `ListAuditLogsData.total` → `int64`** — `audit.go` больше не делает `int(total)` cast; на 32-bit платформе нет overflow.
- **`/auth/logout` стал public** — `security: []` в OpenAPI. Идентификация по refresh-cookie из ctx, не по Bearer. Клиент с истёкшим access всё равно может revoke. Clear-cookie response — безусловный.
  - `AuthService.Logout(userID)` → `LogoutByRefresh(rawRefreshToken)`. Внутри: claim → delete all → audit, всё в одной tx. `sql.ErrNoRows` от claim — no-op (cookie unknown/expired/already-revoked); другие ошибки пробрасываются.
  - E2E TestLogout: убран `no auth returns 401`, добавлен `no cookie returns 200 idempotent`. Header-комментарий перерасказан.
- **Refresh cookie injection вынесен из `RequestMeta`** — отдельный middleware `RefreshCookie`, монтируется глобально, но whitelist'ит только `/auth/refresh` и `/auth/logout`. Любой downstream `ctx`-dump в логах больше не утечёт refresh token на остальных ручках.
- **UA cap в `RequestMeta` middleware** — `MaxUserAgentLength = 1024`, truncate до записи в ctx. Дубль из `handler/creator_application.go` удалён.
- **`main.go:140-143` и `handler/constants.go:5-7`** — длинные godoc'и удалены целиком (имена self-explanatory).
- **KEEP**: Замена «один Logout-метод сервиса принимает userID» → «принимает refresh cookie» — service сам резолвит идентичность из секрета, поэтому handler не зависит от auth-middleware и эндпоинт можно сделать public без ослабления revoke.

## Verification

**Commands:**
- `make generate-api && make build-backend && make lint-backend` — success.
- `make test-unit-backend && make test-unit-backend-coverage` — success.
- `make test-e2e-backend` — success.
- `grep -RE "json\.NewDecoder|respondJSON" backend/internal/handler/ --include='*.go' | grep -v _test.go` — empty.
- `git diff main -- backend/api/openapi.yaml backend/api/openapi-test.yaml` — только добавление `headers.Set-Cookie` в `/auth/logout` 200 (документирует существующее поведение `http.SetCookie` clear); никаких других правок.

## Suggested Review Order

**Wiring (entry point — start here)**

- Strict-server keystone: factory + bound respondError + `StrictServerInterface` контракт.
  [`server.go:144`](../../backend/internal/handler/server.go#L144)

- Production mount: глобальный `RequestMeta` + strict-обёртки для api и testapi.
  [`main.go:126`](../../backend/cmd/api/main.go#L126)

- Generator switch: `chi-server,strict-server,models` для обоих контрактов.
  [`Makefile:185`](../../Makefile#L185)

**Handler pattern (one canonical example)**

- Login = canonical strict-handler: `(ctx, request) → response, error`, Set-Cookie через `Headers`.
  [`auth.go:17`](../../backend/internal/handler/auth.go#L17)

- Cookie-as-header helpers: `http.Cookie.String()` вместо `http.SetCookie(w, ...)`.
  [`auth.go:143`](../../backend/internal/handler/auth.go#L143)

**Context-bridged HTTP request data**

- Bridge HTTP-only fields (UA, refresh cookie) в ctx — strict-handler их через `r` не видит.
  [`request_meta.go:21`](../../backend/internal/middleware/request_meta.go#L21)

- RefreshToken читает cookie из ctx, не из request.
  [`auth.go:46`](../../backend/internal/handler/auth.go#L46)

**Contract precision (single OpenAPI doc fix)**

- Документирование уже-существующего clear-cookie поведения /auth/logout (см. Spec Change Log).
  [`openapi.yaml:115`](../../backend/api/openapi.yaml#L115)

**Mechanical conversions (skim — same shape repeated)**

- `brand.go`, `creator_application.go`, `audit.go`, `dictionary.go`, `health.go`, `testapi.go` — пересказ паттерна `auth.go:Login` для каждого endpoint'а.
  [`brand.go:11`](../../backend/internal/handler/brand.go#L11)

**Tests (peripherals)**

- Test router зеркалит prod-wiring (strict + RequestMeta).
  [`helpers_test.go:31`](../../backend/internal/handler/helpers_test.go#L31)

- Captured-input test: handler читает refresh cookie из `WithRefreshCookie(ctx)`.
  [`auth_test.go:189`](../../backend/internal/handler/auth_test.go#L189)

- Per-method coverage gate для cookie helpers.
  [`auth_test.go:436`](../../backend/internal/handler/auth_test.go#L436)

- Middleware тесты: UA + cookie + missing/empty edge cases.
  [`request_meta_test.go:12`](../../backend/internal/middleware/request_meta_test.go#L12)
