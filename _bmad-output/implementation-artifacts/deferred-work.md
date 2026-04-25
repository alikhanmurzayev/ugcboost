# Deferred Work

## Deferred from: code review of spec-creator-application-submit (2026-04-20)

- Clock abstraction для тестирования timing-boundary через `time.Now()` injection — `backend/internal/handler/creator_application.go:44`. Текущий подход: handler берёт `time.Now().UTC()` и передаёт в сервис. Для timestamp-edge unit-тестов пришлось бы инжектить Clock interface.
- `Content-Type` check и `json.NewDecoder.DisallowUnknownFields()` в `SubmitCreatorApplication` — `backend/internal/handler/creator_application.go:22-27`. Non-JSON и лишние поля проходят без явной ошибки. Не pattern в других handlers проекта.
- IIN century byte 7/8 → 2100s mapping — `backend/internal/domain/iin.go:130-131`. RK-алгоритм документирует только 1–6; 7/8 — наша догадка на будущее. Реальных IIN с century 7 нет до 2100 года.
- Upper bound birth year (`birth.After(now)` / age > 120) — `backend/internal/domain/iin.go:64-72`. Покрывается 18+ check для всего обозримого будущего.
- `audit_logs_nullable_actor` down migration `SET NOT NULL` упадёт при существующих NULL-рядах — `backend/migrations/20260420181757_audit_logs_nullable_actor.sql:10`. Goose down редко вызывается в prod; ручной backfill при необходимости.
- `NewServer` 7 позиционных аргументов — `backend/internal/handler/server.go:85-93`. Вариант: `ServerDeps` struct. Рефакторинг стиля без функционального эффекта.
- `UniqueIIN` counter переполняется при mod 10000 — `backend/e2e/testutil/iin.go:22-32`. В одной сессии 10000 заявок не достигаются, не блокирует MVP.
- PII-guard test (grep stdout-логов по ИИН/ФИО/телефону — 0 совпадений) — spec AC. Требует structured logger-assertion helper. Nice-to-have.
- UTF-8 normalisation (RTL-override, ZWJ) в address / names — `backend/api/openapi.yaml:289-316`. Hardening; не критично для MVP.
- `document_version`, `ip_address`, `user_agent` в consents и required fields в creator_applications — `NOT NULL TEXT` без `CHECK (length(*) > 0)`. Defense-in-depth; handler-валидация (из patch-задачи) закрывает основной сценарий.
- `WithArgs` mock в repo-тестах связан с порядком `CreatorApplicationActiveStatuses` — `backend/internal/repository/creator_application_test.go`. Cosmetic; если порядок изменится — тесты упадут явно.
- `CategoryRow` без `insert:` тегов — `backend/internal/repository/category.go`. Документированный departure (seed-only, нет runtime writes).
- TX обёртывает read-операции `HasActiveByIIN` и `GetActiveByCodes` — `backend/internal/service/creator_application.go:65-82`. Borderline; lock window допустимо короткий, partial unique index — настоящая защита.
- `CleanupEntity` для creator_application полагается на неявный маппинг `sql.ErrNoRows → 404` в testapi handler — `backend/internal/handler/testapi.go`. Контракт по cleanup-stack стабилен, но не покрыт явным unit-тестом.
- `in.IPAddress == ""` не ловится в сервисе — `backend/internal/service/creator_application.go:124-127`. Handler всегда заполняет из middleware; empty значит middleware не подключен (deployment concern).
- `nil` `creatorApplicationService` в Server → panic при POST — `backend/internal/handler/server.go:85-93`. Deployment-time concern; тесты ловят.
- `CookieSecure` default false в test-callsites — `backend/internal/handler/*_test.go`. Zero-value ServerConfig OK для текущих тестов.
- Handler uuid.Parse double-log (logger.Error + respondError) — `backend/internal/handler/creator_application.go:53-57`. Low severity; defensive branch.
- Validation order информационный oracle (IIN format → checksum → age) — `backend/internal/service/creator_application.go:45-85`. Rate-limit на reverse-proxy уровне покрывает.
- Handler test `Submit` использует `MatchedBy` partial вместо exact-args — `backend/internal/handler/creator_application_test.go:101-113`. Cosmetic; основные поля проверены. _(Updated 2026-04-20: success-кейс теперь захватывает input через `Run` и сверяет все поля через `require.Equal`; остальные сабтесты остаются на `mock.Anything`.)_
- E2E не проверяет DB state после успешного submit — `backend/e2e/creator_application/creator_application_test.go:76-95`. Нужен новый тестовый эндпоинт `GET /test/creator-applications/{id}/summary` или прямой SQL-доступ из `testutil` для проверки 1+N+M+4+1 рядов. Расширяет тестовую поверхность.
