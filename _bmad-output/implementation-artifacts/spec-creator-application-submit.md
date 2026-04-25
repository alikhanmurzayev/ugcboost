---
title: 'Публичная ручка приёма заявок креаторов с лендинга'
type: 'feature'
created: '2026-04-20'
status: 'in-progress'
baseline_commit: '2e5667add5fa308e9c303f3871cb96b77401858f'
context:
  - docs/standards/backend-architecture.md
  - docs/standards/backend-transactions.md
  - docs/standards/security.md
---

<frozen-after-approval reason="human-owned intent — do not modify unless human renegotiates">

## Intent

**Problem:** Лендинг `ugcboost.kz` не умеет принимать заявки от UGC-креаторов — нет публичного бэкенд-эндпоинта. Без него FR1/FR1a неисполнимы: нельзя собрать ПД, зафиксировать акцепт 4 согласий (требование юр. документов) и связать будущего Telegram-пользователя с заявкой.

**Approach:** Добавить публичный `POST /creators/applications`. Ручка валидирует данные, атомарно сохраняет заявку + соцсети + категории + 4 согласия + audit-лог в одной транзакции и возвращает `application_id` плюс deep-link на Telegram-бот (`https://t.me/{TELEGRAM_BOT_USERNAME}?start={application_id}`). Бот и модерация — отдельные задачи.

## Boundaries & Constraints

**Always:**
- Contract-first: эндпоинт и все типы — в `openapi.yaml`, код регистрации — через `api.HandlerFromMux()`.
- Атомарность: все `INSERT`-операции (заявка, соцсети, категории, 4 consent, audit) — внутри одной `dbutil.WithTx`. Rollback при любой ошибке.
- Публичная ручка: в OpenAPI `security: []`, никакого авторизационного `Can*` на вход в сервис.
- Константы для таблиц/колонок/enum-ов. Таблица категорий — справочник с `code`/`name`/`active`, seed в миграции.
- В `slog` / stdout-логах приложения — **никаких ПД** (ИИН, ФИО, телефон, адрес, handle). Только HTTP метод/путь/статус/duration + `application_id`. Правило не распространяется на `audit_logs` (см. Design Notes) — там ПД допустимы.
- Миграции — только через `make migrate-create NAME=...`.
- Config — через env (`TELEGRAM_BOT_USERNAME`, `LEGAL_AGREEMENT_VERSION`, `LEGAL_PRIVACY_VERSION`). `CORS_ORIGINS` дополнить `https://ugcboost.kz` в deploy-конфигах (не в коде).
- Unique `(iin)` — партициальный индекс БД WHERE `status IN ('pending','approved','blocked')`. В сервисе — explicit-check до INSERT ради чистого 409.
- Валидация: формат ИИН (12 цифр) + контрольная сумма РК + извлечение `birth_date` + 18+. `birth_date` сохраняется в `creator_applications`.

**Ask First:**
- Любое изменение начального списка категорий (seed миграции).
- Расширение enum соцсетей сверх `instagram`/`tiktok`.
- Введение новых env-переменных сверх трёх перечисленных.

**Never:**
- Прямой `r.Post(...)` или ручной парсинг body/params в обход кодогена.
- Log statements в **stdout-логах приложения**, содержащие значения ИИН, ФИО, телефона, адреса или handle соцсетей. (Audit_logs — исключение, см. Design Notes.)
- Fire-and-forget audit (вне TX) или nested transactions.
- Интеграции с LiveDune/TrustMe/Telegram Bot API в этой итерации.
- Rate-limiting в коде (делаем на reverse-proxy позже).

## I/O & Edge-Case Matrix

| Сценарий | Input / State | Expected | Error Handling |
|----------|--------------|---------------------------|----------------|
| Happy path | валидный запрос, нет активных заявок по ИИН | 201, `{application_id, telegram_bot_url}`; audit-запись; N рядов в socials/categories; 4 ряда в consents | — |
| Дубль по ИИН (service check) | есть заявка в `pending`/`approved`/`blocked`, service находит её до INSERT | 409 `CREATOR_APPLICATION_DUPLICATE` | `domain.NewBusinessError` из сервиса |
| Дубль по ИИН (race → DB) | concurrent submit, service check прошёл, partial unique index ловит при INSERT | 409 `CREATOR_APPLICATION_DUPLICATE` | repo конвертирует `pgconn.PgError` 23505 по имени индекса `creator_applications_iin_active_idx` в `domain.ErrCreatorApplicationDuplicate`, service оборачивает в `NewBusinessError` |
| Повтор после rejected | все предыдущие — `rejected` | 201 (новая заявка создаётся) | — |
| Возраст <18 | birth_date из ИИН даёт age < 18 | 422 `UNDER_AGE` "Возраст менее 18 лет" | `domain.NewValidationError(CodeUnderAge, ...)` |
| Невалидный ИИН | формат или контрольная сумма | 422 `INVALID_IIN` "Невалидный ИИН" | `domain.NewValidationError(CodeInvalidIIN, ...)` |
| Неизвестная категория | `code` не найден / `active=false` | 422 `UNKNOWN_CATEGORY` "Неизвестная категория: {code}" | `domain.NewValidationError(CodeUnknownCategory, ...)` |
| Не все 4 consent=true | любой false | 422 `MISSING_CONSENT` "Требуется согласие: {consent_type}" | `domain.NewValidationError(CodeMissingConsent, ...)` |
| Пустые обязательные поля | last_name/first_name/iin/phone/city/address — пустые или whitespace-only | 422 `VALIDATION_ERROR` | handler проверка post-trim length |
| Дубликат (platform, handle) в запросе | одна и та же пара в `socials[]` повторяется | 422 `VALIDATION_ERROR` "Дубликат соцсети: {platform}/{handle}" | `domain.NewValidationError` в `normaliseSocials` |
| Невалидная соцсеть | platform вне enum | 422 `VALIDATION_ERROR` | `normaliseSocials` (handler парсит body через `json.Decode`, не HandleParamError) |
| Любой DB-сбой посередине TX | — | 500 `INTERNAL_ERROR`, данные не остались | `WithTx` rollback |

</frozen-after-approval>

## Code Map

- `backend/api/openapi.yaml` -- добавить `/creators/applications` + schemas (`CreatorApplicationSubmitRequest`, `...Response`, `SocialAccountInput`, `ConsentsInput`, `SocialPlatform` enum)
- `backend/migrations/*_categories.sql` -- справочник + seed (beauty, fashion, food, fitness, lifestyle, tech, travel, parenting, auto, gaming)
- `backend/migrations/*_creator_applications.sql` -- таблица + partial unique index по iin
- `backend/migrations/*_creator_application_categories.sql` -- M:N join
- `backend/migrations/*_creator_application_socials.sql` -- 1:N с `(application_id, platform, handle)` unique
- `backend/migrations/*_creator_application_consents.sql` -- 1:N с `(application_id, consent_type)` unique
- `backend/internal/config/config.go` -- `TelegramBotUsername`, `LegalAgreementVersion`, `LegalPrivacyVersion`
- `backend/internal/domain/creator_application.go` -- типы, `CodeCreatorApplicationDuplicate`, константы статусов/consent types/platforms, мапперы
- `backend/internal/domain/iin.go` -- `ValidateIIN(iin) (birthDate, error)`; сначала искать Go-библиотеку (pkg.go.dev `kazakhstan iin`), если нет — своё с обоснованием
- `backend/internal/repository/category.go` -- `CategoryRepo.GetActiveByCodes(ctx, codes) ([]*CategoryRow, error)`
- `backend/internal/repository/creator_application.go` -- Create + `HasActiveByIIN`
- `backend/internal/repository/creator_application_category.go` -- batch Insert
- `backend/internal/repository/creator_application_social.go` -- batch Insert
- `backend/internal/repository/creator_application_consent.go` -- batch Insert (ровно 4 ряда)
- `backend/internal/repository/factory.go` -- 5 новых методов
- `backend/internal/service/audit_constants.go` -- `AuditActionCreatorApplicationSubmit`, `AuditEntityTypeCreatorApplication`
- `backend/internal/service/creator_application.go` -- `CreatorApplicationService.Submit(...)` с `dbutil.WithTx`
- `backend/internal/handler/server.go` -- `CreatorApplicationService` interface; расширить `NewServer` и `Server`
- `backend/internal/handler/creator_application.go` -- `SubmitCreatorApplication` handler, маппинг request→domain, формирование `telegram_bot_url`
- `backend/cmd/api/main.go` -- wiring нового сервиса + передача в `NewServer`
- `backend/e2e/creator_application/creator_application_test.go` -- E2E с narrative godoc на русском (`Package creator_application — ...`)

## Tasks & Acceptance

**Execution:**
- [x] `backend/api/openapi.yaml` -- добавить endpoint и schemas; `security: []`; документировать 201/409/422
- [x] `make generate-api` -- перегенерировать server/client stubs
- [x] `backend/internal/config/config.go` -- добавить 3 env vars
- [x] `make migrate-create NAME=categories` + 4 похожие команды; заполнить SQL-тела и seed категорий (+ миграция `audit_logs_nullable_actor` — actor_id nullable для публичных событий)
- [x] `backend/internal/domain/iin.go` -- валидация + извлечение даты рождения; unit-тесты
- [x] `backend/internal/domain/creator_application.go` -- типы, константы, мапперы
- [x] `backend/internal/repository/*.go` × 5 -- row-структуры, константы, методы; обновить `factory.go`
- [x] `backend/internal/service/creator_application.go` + `audit_constants.go` -- сервис с TX и audit
- [x] `backend/internal/handler/creator_application.go` + `server.go` -- handler + interface + конструктор
- [x] `backend/cmd/api/main.go` -- wiring
- [x] Unit-тесты handler/service/repository по правилам `backend-testing-unit.md`; `make test-unit-backend-coverage` проходит
- [x] `backend/e2e/creator_application/creator_application_test.go` -- покрыть все строки I/O-матрицы; godoc-нарратив на русском; клиент из openapi; `E2E_CLEANUP` через существующую `/test/*` cleanup-ручку (добавили тип `creator_application`)
- [ ] Добавить `https://ugcboost.kz` в `CORS_ORIGINS` документации/compose env (не в коде) — оставляю деплой-артефактом, в dev уже работает через env

**Acceptance Criteria:**
- Given валидный запрос без активной заявки по ИИН, when POST `/creators/applications`, then HTTP 201 с `application_id` (UUID) и `telegram_bot_url` = `https://t.me/{TELEGRAM_BOT_USERNAME}?start={application_id}`.
- Given в БД после успешного запроса, when читаем таблицы, then существуют: 1 ряд в `creator_applications` (status=`pending`, birth_date заполнен), N в `creator_application_socials`, M в `creator_application_categories`, ровно 4 в `creator_application_consents` (IP, UA, document_version заполнены) и 1 в `audit_logs` (action=`creator_application_submit`).
- Given второй запрос с тем же ИИН при активной заявке, when POST, then HTTP 409 `CREATOR_APPLICATION_DUPLICATE`, новые ряды не создаются.
- Given запрос, проваливающийся на N-м INSERT внутри TX, when POST, then HTTP 500 и ни одного нового ряда ни в одной из 5 таблиц.
- Given успешный запрос, when grep stdout-логов по ИИН/ФИО/телефону кандидата, then 0 совпадений.
- Given `make lint-backend` / `make test-unit-backend-coverage` / `make test-e2e-backend`, when запускаем, then все проходят.

## Design Notes

- **Partial unique index**: `CREATE UNIQUE INDEX creator_applications_iin_active_idx ON creator_applications(iin) WHERE status IN ('pending','approved','blocked')`. Это защита на уровне БД; сервис делает explicit-чек до INSERT, чтобы вернуть доменный 409 с бизнес-кодом вместо general `sql` unique-violation.
- **ИИН-библиотеки**: сперва проверить pkg.go.dev / Awesome Go (`iin`, `kazakhstan`). При отсутствии зрелого варианта — своё в `domain/iin.go`; обосновать в godoc (backend-libraries.md). Алгоритм: два прохода по 12 цифрам с весами 1..11 и 3..11,1,2; контрольная — 12-я цифра. ДР: `DDMMYY` + век из 7-й цифры (1/2→19xx, 3/4→19xx, 5/6→20xx, 7/8→21xx — точный mapping уточняется при реализации).
- **Audit для публичной ручки**: `ActorID=NULL` (миграция `audit_logs_nullable_actor`), `ActorRole=""`, `IPAddress` из `middleware.ClientIPFromContext`, `NewValue` — `{application_id, city, categories, platforms}`. В `audit_logs` **допустимы** любые поля сущности, включая ПД — это бизнес-запись действия, доступная только администраторам. Запрет ПД касается stdout-логов приложения (`slog.Info/Error`), не audit.
- **Audit actor_id в API**: поле `actor_id` в OpenAPI-типе `AuditLog` — `nullable: true`, в JSON отдаётся `null` для public-actions (submit без аутентификации). Клиент явно различает "system action" (null) и "user action" (UUID string).
- **Handle нормализация**: `strings.TrimSpace` → `strings.TrimLeft("@")` (снимаем все ведущие `@`, не один) → `strings.ToLower` (для IG/TT) → проверка platform-regex (IG/TT: `^[A-Za-z0-9._]{1,30}$`; проверка после lowercase — `A-Za-z` оставлен для валидации исходного ввода). Handle после нормализации не может быть пустым. Храним уже нормализованным — это упрощает будущий matching с LiveDune. Дубликаты `(platform, handle)` в одном запросе — 422 до INSERT.
- **Consents**: ровно 4 типа (`processing`, `third_party`, `cross_border`, `terms`). Request-схема — объект с 4 булевыми полями; любой `false`/отсутствующий → 422. В БД пишем ровно 4 ряда с `accepted_at=now()`, `document_version` из config, `ip`/`user_agent` из request.

## Verification

**Commands:**
- `make generate-api` -- expected: изменения только в сгенерированных файлах (`server.gen.go`, e2e clients, ts schemas)
- `make build-backend` -- expected: успешная сборка без warnings
- `make lint-backend` -- expected: 0 issues
- `make test-unit-backend-coverage` -- expected: gate passed (≥80% per method на handler/service/repository)
- `make test-e2e-backend` -- expected: creator_application пакет зелёный
- `curl -i -X POST http://localhost:8082/creators/applications -d @testdata/valid.json && docker logs backend 2>&1 | grep -E '\\d{12}|+7' | wc -l` -- expected: `0`

**Manual checks:**
- DB-инспекция после happy-path-запроса: 1+N+M+4+1 рядов, все в одной транзакции (timestamp совпадает).
- Смоук deep-link: `telegram_bot_url` в ответе имеет формат `https://t.me/<username>?start=<uuid>`.

## Review Findings (2026-04-20)

Adversarial code review PR #19 branch `alikhan/creator-application-submit` — 3 слоя (Blind, Edge Case, Acceptance) + локальные CI-гейты (build/lint/test/coverage — все зелёные). После триажа: 3 decision-needed, 18 patch, 21 defer, 1 dismissed.

### Decision-Needed

- [x] [Review][Decision] **Контрактная несовместимость error codes 422** — Спека замораживает `422 VALIDATION_ERROR` для всех validation-сценариев в I/O matrix, но impl использует granular `INVALID_IIN` / `UNDER_AGE` / `MISSING_CONSENT` / `UNKNOWN_CATEGORY` и E2E тесты на них же завязаны. Варианты: (a) renegotiate спеку — переписать I/O matrix под granular коды; (b) поправить impl — все 422 → `CodeValidation`, переписать E2E.
- [x] [Review][Decision] **`city` в audit_logs payload** [`service/creator_application.go:271-282`] — Спека не запрещает напрямую (Never-раздел про логи, не про audit), но city — location PII. Варианты: (a) оставить для аналитики "откуда приходят"; (b) убрать — аналитика всё равно из самой таблицы applications.
- [x] [Review][Decision] **`actor_id` *string → "" в audit API response** [`handler/audit.go:1244-1247`] — Pre-existing API возвращает `actor_id: string`. С nullable полем impl эмитит `""` для nil. Клиент не различает system-actor vs missing. Варианты: (a) change API: `actor_id → *string` (null в JSON) — breaking; (b) добавить `is_system: bool` — non-breaking; (c) оставить `""` как system-маркер, документировать.

### Patch

- [x] [Review][Patch] **Empty/whitespace-only required fields проходят валидацию** [`handler/creator_application.go:22-45`, `service/creator_application.go:87-95`] — OpenAPI `minLength: 1` пропускает `" "`, а handler/service только `TrimSpace` без post-trim length check. Нужна явная проверка.
- [x] [Review][Patch] **Handle нормализация: нет lowercase, `TrimPrefix` не multi-@, нет фильтра whitespace/control** [`service/creator_application.go:196-202`] — Design Notes требует lowercase для IG/TT. `@aidana` и `@Aidana` создадут 2 ряда. `@@aidana` сохраняет ведущий @. Нужно `TrimLeft("@")` + `ToLower` + regex-валидация handle.
- [x] [Review][Patch] **Дубликаты (platform, handle) в одном запросе → 500** [`service/creator_application.go:182-204`] — UNIQUE index в БД срабатывает mid-TX, возвращая 500 вместо 422. Дедуплицировать до INSERT.
- [x] [Review][Patch] **TOCTOU: concurrent duplicate IIN → 500 вместо 409** [`repository/creator_application.go:87-94`, `service/creator_application.go:97-99`] — При гонке `HasActiveByIIN→Create` partial unique index ловит, но pgconn unique-violation оборачивается `fmt.Errorf` → 500. Нужно catch `pgconn.PgError` SQLSTATE 23505 + имя индекса → `CodeCreatorApplicationDuplicate`.
- [x] [Review][Patch] **Сообщения не соответствуют спеке (mix RU/EN)** [`service/creator_application.go:55,155,167,172,184`] — Спека I/O matrix: `"Возраст менее 18 лет"` / `"Невалидный ИИН"` / `"Требуется согласие: {consent_type}"`. Impl: `"Applicant must be at least 18 years old"` / `"Некорректный ИИН"` / `"Отсутствует обязательное согласие: X"`. Mix RU/EN + drift от спеки.
- [x] [Review][Patch] **Missing consent error подставляет raw enum в user-facing message** [`service/creator_application.go:154-156`] — `third_party`, `cross_border` попадают в текст сообщения. Либо localise, либо machine-code в details поле. Связано с предыдущим.
- [x] [Review][Patch] **Dead branch `ErrIINUnderAge18` в `iinErrorToValidation`** [`service/creator_application.go:166-168`] — `ValidateIIN` никогда не возвращает этот sentinel (age-check делается отдельно через `EnsureAdult`). Убрать кейс.
- [x] [Review][Patch] **`iinErrorToValidation` default возвращает raw err → 500** [`service/creator_application.go:174`] — Новые sentinel-ошибки IIN деградируют в 500. Default → `CodeInvalidIIN` или хотя бы `CodeValidation`.
- [x] [Review][Patch] **`telegramBotUsername` не URL-escape'ится** [`handler/creator_application.go:72-74`] — Если env содержит URL-unsafe символ → malformed deep-link. `url.PathEscape(username)`.
- [x] [Review][Patch] **`logger.Info("submitted")` внутри WithTx до commit** [`service/creator_application.go:139-141`] — Если commit упадёт, лог говорит "submitted", а transaction откатилась. Перенести после `WithTx` return nil.
- [x] [Review][Patch] **E2E `invalid iin format` не проверяет Error.Code** [`e2e/creator_application/creator_application_test.go:123-132`] — Только StatusCode 422, без `resp.JSON422.Error.Code`. Братские сабтесты корректно проверяют код.
- [x] [Review][Patch] **E2E `unsupported platform` non-deterministic (400 \|\| 422)** [`e2e/creator_application/creator_application_test.go:185-197`] — `require.True(t, status == 400 \|\| status == 422)` — зафиксировать один код и поведение.
- [x] [Review][Patch] **E2E under-18 тест clock-dependent: `2010-05-15`** [`e2e/creator_application/creator_application_test.go:237-246`] — В 2028-05-15 кандидат станет 18, тест упадёт. Вычислять birthdate от `time.Now().Year() - 16`.
- [x] [Review][Defer] **E2E не проверяет DB state после success** [`e2e/creator_application/creator_application_test.go:76-95`] — deferred: требует нового тестового эндпоинта (`GET /test/creator-applications/{id}/summary`) или прямого SQL-доступа в testutil, что расширяет тестовую поверхность за рамки текущей итерации. Записано в deferred-work.
- [x] [Review][Patch] **Handler unit test не покрывает `IPAddress` forwarding** [`handler/creator_application_test.go:68-133`] — `middleware.ClientIP` не wired в `newTestRouter`, предикат `MatchedBy` пропускает IP. AC по consent.ip_address не верифицирован нигде в unit/e2e.
- [x] [Review][Patch] **Handler unit test не покрывает invalid-UUID ветку** [`handler/creator_application_test.go:68-133`, `handler/creator_application.go:53-58`] — Defensive-branch без покрытия.
- [x] [Review][Patch] **`r.UserAgent()` не ограничен по длине** [`handler/creator_application.go:41`] — Атакер может отправить multi-KB UA, который запишется в consent rows. Truncate до ~1KB.
- [x] [Review][Patch] **`c.AsMap()` вызывается 4 раза в цикле `requireAllConsents`** [`service/creator_application.go:152-157`] — Тривиально: вычислить map один раз до цикла.

### Deferred

- [x] [Review][Defer] Clock abstraction (`time.Now()` injection) [`handler/creator_application.go:44`] — deferred, рефакторинг вне scope MVP.
- [x] [Review][Defer] Content-Type + `DisallowUnknownFields` проверки [`handler/creator_application.go:22-27`] — deferred, не pattern в других handlers.
- [x] [Review][Defer] Century byte 7/8 → 2100s mapping [`domain/iin.go:130-131`] — deferred, edge case, не актуально до 2100.
- [x] [Review][Defer] Upper bound birth year (future/>120 лет) [`domain/iin.go:64-72`] — deferred, покрывается 18+ check в обозримом будущем.
- [x] [Review][Defer] Migration down path `audit_logs_nullable_actor` unsafe [`migrations/20260420181757_*.sql:10`] — deferred, goose down редко вызывается в prod.
- [x] [Review][Defer] `NewServer` 7 позиционных аргументов [`handler/server.go:85-93`] — deferred, рефакторинг стиля.
- [x] [Review][Defer] `UniqueIIN` counter mod 10000 overflow [`e2e/testutil/iin.go:22-32`] — deferred, не достижимо в рамках одной сессии.
- [x] [Review][Defer] Нет теста PII-guard на логи [spec AC "grep stdout = 0 matches"] — deferred, nice-to-have.
- [x] [Review][Defer] UTF-8 RTL-override / ZWJ в address/names [`api/openapi.yaml:289-316`] — deferred, hardening.
- [x] [Review][Defer] `document_version` / `ip_address` / `user_agent` NOT NULL TEXT без `CHECK length>0` [`migrations/20260420181753_*.sql`, `20260420181756_*.sql`] — deferred, defense-in-depth.
- [x] [Review][Defer] `WithArgs` mock в repo-тестах связан с порядком `CreatorApplicationActiveStatuses` [`repository/creator_application_test.go`] — deferred, cosmetic.
- [x] [Review][Defer] `CategoryRow` без `insert:` тегов [`repository/category.go`] — deferred, документированный departure (seeded-only).
- [x] [Review][Defer] TX обёртывает read-операции (`HasActiveByIIN`, `GetActiveByCodes`) [`service/creator_application.go:65-82`] — deferred, lock window допустимо короткий.
- [x] [Review][Defer] `CleanupEntity` для creator_application polagaется на неявный маппинг `sql.ErrNoRows → 404` [`handler/testapi.go`] — deferred, контракт по cleanup-stack стабилен.
- [x] [Review][Defer] `in.IPAddress == ""` не ловится в сервисе [`service/creator_application.go:124-127`] — deferred, handler всегда заполняет.
- [x] [Review][Defer] `nil` `creatorApplicationService` в Server panic [`handler/server.go:85-93`] — deferred, deployment-time concern.
- [x] [Review][Defer] `CookieSecure` default false в test-callsites [`handler/*_test.go`] — deferred, zero-value OK для текущих тестов.
- [x] [Review][Defer] Handler `uuid.Parse` double-log (logger.Error + respondError) [`handler/creator_application.go:53-57`] — deferred, low severity.
- [x] [Review][Defer] Validation order information-disclosure oracle (IIN/age/category) [`service/creator_application.go:45-85`] — deferred, rate-limit на reverse-proxy.
- [x] [Review][Defer] Handler test `Submit` использует `MatchedBy` partial вместо exact-args [`handler/creator_application_test.go:101-113`] — deferred, cosmetic; основные поля проверяются.
- [x] [Review][Defer] OpenAPI `minLength:1` пропускает `" "` [`api/openapi.yaml:289-316`] — dismissed as subsumed by patch "Empty/whitespace-only required fields".
