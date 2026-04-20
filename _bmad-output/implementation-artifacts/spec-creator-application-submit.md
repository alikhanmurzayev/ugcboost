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
- В `slog` / stdout — **никаких ПД** (ИИН, ФИО, телефон, адрес, handle). Только HTTP метод/путь/статус/duration + `application_id`.
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
- Log statements, содержащие значения ИИН, ФИО, телефона, адреса или handle соцсетей.
- Fire-and-forget audit (вне TX) или nested transactions.
- Интеграции с LiveDune/TrustMe/Telegram Bot API в этой итерации.
- Rate-limiting в коде (делаем на reverse-proxy позже).

## I/O & Edge-Case Matrix

| Сценарий | Input / State | Expected | Error Handling |
|----------|--------------|---------------------------|----------------|
| Happy path | валидный запрос, нет активных заявок по ИИН | 201, `{application_id, telegram_bot_url}`; audit-запись; N рядов в socials/categories; 4 ряда в consents | — |
| Дубль по ИИН | есть заявка в `pending`/`approved`/`blocked` | 409 `CREATOR_APPLICATION_DUPLICATE` | `domain.NewBusinessError` до INSERT |
| Повтор после rejected | все предыдущие — `rejected` | 201 (новая заявка создаётся) | — |
| Возраст <18 | birth_date из ИИН даёт age < 18 | 422 `VALIDATION_ERROR` "Возраст менее 18 лет" | `domain.NewValidationError` |
| Невалидный ИИН | формат или контрольная сумма | 422 `VALIDATION_ERROR` "Невалидный ИИН" | `domain.NewValidationError` |
| Неизвестная категория | `code` не найден / `active=false` | 422 `VALIDATION_ERROR` "Неизвестная категория: {code}" | `domain.NewValidationError` |
| Не все 4 consent=true | любой false | 422 `VALIDATION_ERROR` "Требуется согласие: {consent_type}" | `domain.NewValidationError` |
| Пустые обязательные поля | last_name/first_name/iin/phone/... | 422 `VALIDATION_ERROR` | handler валидация |
| Невалидная соцсеть | platform вне enum | 422 (кодогенным `ParamError`) | `HandleParamError` |
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
- **Audit для публичной ручки**: `ActorID=""`, `ActorRole=""`, `IPAddress` из `middleware.ClientIPFromContext`, `NewValue` — только неперсональные поля (`first_name`, `last_name`, `city`, `category_codes`, `platforms`) или просто `{"application_id":"..."}`. ПД не должны попасть в `audit_logs.new_value`, т.к. audit читают администраторы (пока ИИН всё же нужен — держим в `creator_applications`, не дублируем в audit).
- **Handle нормализация**: `strings.TrimSpace` + отбрасывать ведущий `@`. Регистр приводим к lowercase для IG/TT. Храним уже нормализованным — это упрощает будущий matching с LiveDune.
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
