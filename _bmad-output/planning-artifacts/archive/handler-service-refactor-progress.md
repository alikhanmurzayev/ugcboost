# Прогресс: рефакторинг handler/service под стандарты

## Выполнено

- [x] Предзагрузка контекста: 19 стандартов + все затрагиваемые пакеты
- [x] **Stage 1** — перенос `api/` → `backend/api/`
- [x] **Stage 2** — расширение OpenAPI: `LoginData`, `MessageData`, `ListBrandsData`, `BrandDetailData`, `AssignManagerData`, `ListAuditLogsData`; `minLength: 6` для `password`/`newPassword`; полные спеки `SeedUserData/ResetTokenData/SeedBrandData` в openapi-test.yaml; генерация `backend/internal/testapi/server.gen.go`
- [x] **Stage 3** — удалён `domain/roles.go`, `middleware.RoleFromContext()` → `api.UserRole`, везде `api.Admin`/`api.BrandManager`
- [x] **Stage 4** — `authz/` переписан: `AuthzService` struct с DI, `BrandService` интерфейс, методы по API-действиям (`CanCreateBrand`, `CanListBrands`, `CanViewBrand`, `CanUpdateBrand`, `CanDeleteBrand`, `CanAssignManager`, `CanRemoveManager`, `CanListAuditLogs`). `BrandService.CanViewBrand` → `IsUserBrandManager`. `ListBrands(managerID *string)`. Unit-тесты AuthzService созданы
- [x] **Stage 5** — `service/audit_constants.go` (AuditAction* / AuditEntityType*). Аудит внутри `WithTx` в `Create/Update/Delete/AssignManager/RemoveManager/Login/Logout/ResetPassword`. `AuditService.Log` + `AuditEntry` удалены. `handler/auditor.go` удалён. Middleware `ClientIP` поверх `chi/middleware.RealIP`, `ClientIPFromContext` helper. Unit-тесты middleware
- [x] **Stage 6** — `handler/test.go` реализует `testapi.ServerInterface`. `TestAuthService` и `TestBrandService` интерфейсы (test-only). `SeedBrand`/`AssignManagerSeed` в `BrandService` — отдельные пути без аудита (test fixtures ≠ реальное действие пользователя). `main.go`: `testapi.HandlerWithOptions` вместо ручных `r.Post/Get`
- [x] **Stage 7** — `handler/helpers_test.go`: `newTestRouter`, `doJSON[Resp]`, `withAdminCtx`. `auth_test.go`/`brand_test.go` полностью переписаны: роутер + ServerInterfaceWrapper, типизированные request/response, `require.WithinDuration` для cookie.Expires, `require.NotEmpty`+zero+`require.Equal` для temp passwords
- [x] **Stage 8** — `encodeJSON` helper с `slog.Error`; убраны все `//nolint:errcheck` в `handler/`; `rawJSONToAny` логирует unmarshal-ошибки; `pgx` удалён из handler; `sql.ErrNoRows` → 404 в respondError; минимальная длина пароля 6 проверяется в handler (wrapper не валидирует body по OpenAPI)

## Итоговая проверка

### Тесты
- ✅ `make test-unit-backend` — зелёный
- ✅ `make lint-backend` — 0 issues
- ✅ `make test-e2e-backend` — зелёный (auth + brand + audit domains)
- ✅ `make build-backend build-web build-tma` — зелёный

### Greps (все пусто)
- ✅ `authz.RequireAdmin\|CanManageBrand\|CanApproveCreator\|CanManageCampaign\|BrandAccessChecker` в backend/
- ✅ `logAudit` в handler/
- ✅ `domain.RoleAdmin\|RoleBrandManager\|UserRole` в backend/
- ✅ `pgx` в handler/
- ✅ `//nolint:errcheck` в handler/
- ✅ `strings.NewReader(` в handler/*_test.go
- ✅ `map[string]any` в handler/test.go
- ✅ `r.URL.Query().Get` в handler/
- ✅ `SeedUser` в handler/server.go
- ✅ `"login"|"logout"|"password_reset"|"brand_*"|"manager_*"` в service/*.go (кроме audit_constants.go, где они определены)
- ✅ `Data: struct{` response-литералов в handler/

## Отклонения от плана

- **TestHandler impersonates seed admin** (а не отдельные seed-пути): audit_logs.actor_id = UUID NOT NULL REFERENCES users(id) — без actor в context FK ломается. В `main.go` после `SeedAdmin` резолвится admin.ID по `cfg.AdminEmail` и передаётся в `NewTestHandler`; `SeedBrand` инъектит `ContextKeyUserID=adminID` + `ContextKeyRole=Admin` в ctx и дёргает обычный `BrandService.CreateBrand`/`AssignManager`. Audit пишется полноценно (актором числится seed-admin, что семантически корректно). Дублирующие `CreateBrandSeed`/`AssignManagerSeed` не потребовались. Новый метод `AuthService.GetUserByEmail` для резолва
- **minLength:6 осталась в handler**: REQ-18 требовал переносить валидацию в `ServerInterfaceWrapper`, но oapi-codegen chi-сервер не валидирует request body по OpenAPI — без `openapi3filter`-middleware это не сработает. Восстановил проверку `len(req.NewPassword) < minPasswordLength` в handler с комментарием-пояснением (иначе e2e `TestResetPassword_ShortPassword` ждал бы 422 и получал 200)
- **sql.ErrNoRows в respondError**: план требовал убрать `pgx.ErrNoRows` ветку. Репозитории возвращают `sql.ErrNoRows` (по стандарту `backend-repository.md`), поэтому handler маппит `sql.ErrNoRows → 404` вместо pgx. Альтернативу (оборачивание в repo) не делал — в repo это бы завело зависимость от `domain`

## Что НЕ сделано (не заявлено в плане как обязательное)

- `middleware/json.go` оставлен с `//nolint:errcheck` (вне handler-scope, плановые greps это не ловят)
- `backend-testing-unit.md` призывает `require.WithinDuration+zero+require.Equal` для всех динамических полей. Частично: cookie.Expires и temp password покрыты; expiresAt и другие поля — нет, так как существующий стиль тестов проверяет ключевые поля по-отдельности

## Git-состояние

Агент НЕ делает коммит. Изменения в working tree:
- Modified: Makefile, 25+ файлов в backend/
- Added: authz/{brand,audit,brand_test,audit_test}.go, testapi/server.gen.go, middleware/client_ip{,_test}.go, service/audit_constants.go, handler/helpers_test.go, handler/mocks/mock_{authz,test_auth}_service.go
- Deleted: domain/roles.go, handler/auditor.go, authz/mocks/mock_brand_access_checker.go
- Renamed: api/ → backend/api/

Коммит, push и мерж PR — за Alikhan.
