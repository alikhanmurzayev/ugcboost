---
title: "План: привязка Telegram-аккаунта к заявке (chunk 1 онбординга креатора)"
type: plan
status: ready-for-build
created: "2026-04-29"
scout: "_bmad-output/implementation-artifacts/29_04_2026_creator-telegram-link-scout.md"
roadmap: "_bmad-output/planning-artifacts/creator-onboarding-roadmap.md"
chunk: 1
---

# План реализации: привязка Telegram-аккаунта к заявке

> **Преамбула для следующего агента (build).**
> Перед любым действием по плану — **полностью** загрузить в контекст все
> файлы из `docs/standards/` (читать каждый файл целиком, без выборочного
> поиска). Это hard rule проекта (`feedback_artifacts_standards_preamble`).
> Любое расхождение с любым стандартом — finding. Без полной загрузки
> стандартов — не приступать. Также прочитать scout-артефакт `29_04_2026_creator-telegram-link-scout.md`
> как контекст домена.

## Обзор

Креатор подал заявку на лендинге, нажал кнопку «Открыть Telegram» — открывается
deep-link `https://t.me/<bot>?start=<application_id>`, Telegram отправляет
команду `/start <application_id>` в наш бот. Бот валидирует UUID, проверяет
статус и текущую привязку заявки, и при отсутствии конфликтов сохраняет связь
«Telegram user → application». В этом chunk бот умеет только команду `/start`
с UUID-payload; всё остальное — fallback-инструкция на русском, тон официальный.

## Зафиксированные решения (§5 scout)

- **Транспорт:** long polling. Вне scope — webhook.
- **Размещение:** в процессе API, отдельная goroutine, регистрация в closer.
  **Защита от двойного polling при Dokploy rolling deploy — простой
  retry на 409.** Telegram сам отбрасывает «лишний» concurrent `getUpdates`
  с HTTP 409 _«terminated by other getUpdates request»_; выживает последний.
  Старый контейнер получит 409 и будет retry'ить, пока Dokploy не пришлёт
  ему SIGTERM (~10-60 сек), новый сразу принимает все updates. Окно конкуренции
  локализовано во времени деплоя; теоретический дубль одного update'а
  (старый получил, не подтвердил, новый получил тот же) безопасен — наша
  бизнес-логика идемпотентна: повторный `/start` от того же TG к той же
  заявке возвращает success без дубликата audit-row. Никакого advisory
  lock'а, никаких dedicated connections. Future horizontal scaling — тот же
  механизм работает (Telegram ограничивает один активный polling); если
  понадобится bullet-proof — отдельный Dokploy-сервис `backend-bot`
  (replicas=1) — отдельный chunk.
- **Хранение:** отдельная таблица `creator_application_telegram_links`.
  Заявка и identity-метод — разные сущности. Завтра появится мобильное
  приложение / web-app — у него будет свой `creator_application_<channel>_links`,
  заявка не меняется. PK — `application_id` (1 заявка → 1 Telegram-link).
  `UNIQUE(telegram_user_id)` (1 Telegram-аккаунт → 1 link). При rejection
  flow (chunk 5) link удаляется — это разблокирует Telegram-аккаунт для
  новой заявки. До chunk 5 rejection-flow не существует, edge case
  «отклонили и пробуем снова с тем же TG» не возникает.
  `ON DELETE CASCADE` на FK к `creator_applications` — cleanup-test
  удаления заявки автоматически чистит link. **Сохраняем максимум
  Telegram-данных, доступных через update:** `telegram_username TEXT NULL`,
  `telegram_first_name TEXT NULL`, `telegram_last_name TEXT NULL`.
  Это PII (handle, ФИО) — допустимо в БД при наличии PII inventory и
  retention-политики. В audit `new_value` копии этих полей попадают
  (audit разрешено иметь PII per security.md). В stdout `logger.Info` —
  только идентификаторы (`tg_user_id`, `application_id`), имена/username
  никогда. Cap длины в service-слое: username 64, first/last name 256
  символов после trim — DoS защита (Telegram сам не ограничивает строго).
  PII inventory (`legal-documents/PII_INVENTORY.md`) сейчас отсутствует —
  pre-existing техдолг (`creator_applications` уже хранит ИИН/ФИО/phone/
  handle без inventory). Этот chunk создаёт inventory с записями по всем
  существующим и новым PII-полям.
- **Библиотека:** `github.com/go-telegram/bot`. Активная поддержка,
  идиоматичный типизированный API, минимальная зависимость, поддерживает
  long polling и webhook, удобно мокается. Альтернативы (telego, telebot,
  go-telegram-bot-api) рассматривались — go-telegram/bot чище для нашего
  кейса. Реестр `docs/standards/backend-libraries.md` пополняется тем же PR.
- **E2E:** через test-endpoint `POST /test/telegram/send-update`, которая
  принимает упрощённый update (chat_id / user_id / text), пушит его в
  диспатчер, возвращает ответы spy-клиента. Без живого Telegram. Test-режим
  включается при `EnableTestEndpoints`.
- **Reply-тоны:** официальный, на русском, без эмодзи и жаргона.

## Требования

- **REQ-1.** Команда `/start <uuid>`: при валидном UUID и активной заявке
  без существующей привязки — INSERT в `creator_application_telegram_links`,
  креатор получает reply-подтверждение, в `audit_logs` пишется
  `creator_application_link_telegram` action в той же транзакции.
- **REQ-2.** Идемпотентность: повторная команда `/start <uuid>` от того же
  Telegram-пользователя для уже привязанной к нему заявки — возвращает то же
  reply-подтверждение, новую audit-запись **не** создаёт.
- **REQ-3.** Конфликт «другой TG»: команда `/start <uuid>` от другого
  Telegram-пользователя для уже привязанной заявки — возвращает отказное
  reply, привязка не меняется.
- **REQ-4.** Конфликт «другая заявка»: команда `/start <uuid>` от
  Telegram-пользователя, у которого уже есть активная заявка с другим UUID,
  — возвращает отказное reply, привязка не меняется. `Rejected`-заявки в
  истории — допустимо, не блокирует новую привязку.
- **REQ-5.** Заявка не найдена / UUID невалиден — fallback reply с
  инструкцией. Сервер ничего не пишет, `audit_logs` не растёт.
- **REQ-6.** Заявка `rejected` / `blocked` — отдельное reply с объяснением
  статуса. Привязка не делается, audit не пишется.
- **REQ-7.** Команда без payload (`/start`) или любое другое сообщение —
  fallback reply.
- **REQ-8.** Race-сценарии для INSERT в `creator_application_telegram_links`:
  - PK conflict на `application_id` (тот же `app_id` уже занят) — сервис
    проверяет: если existing.tg_user_id == new tg_user_id → idempotent
    success; иначе → `ApplicationAlreadyLinked`. Race ловится через preflight
    SELECT внутри tx + страховочный mapping SQLSTATE 23505 на PK constraint.
  - UNIQUE conflict на `telegram_user_id` (тот же TG уже привязан к другой
    заявке) — SQLSTATE 23505 на UNIQUE constraint → `AccountAlreadyLinked`.
  Не 500.
- **REQ-9.** PII в stdout-логах приложения — 0. Telegram username, first_name,
  last_name в `audit_logs` допустимы; в `logger.Info` —
  только `application_id` и `tg_user_id` (UUID и BIGINT — идентификаторы,
  не PII per security.md). PII-guard e2e тест grep'ает stdout по handle/
  first_name/last_name/iin/phone и ожидает 0 совпадений.
- **REQ-10.** Бот не падает при отсутствии `TELEGRAM_BOT_TOKEN` или при
  `TELEGRAM_MOCK=true` — runner не стартует, диспатчер живёт только через
  test-endpoint (для CI). Логируется явное сообщение «telegram bot disabled».
- **REQ-11.** Long polling runner устойчив к ошибкам сети и 409 от Telegram:
  фиксированный retry каждые 10 секунд на любую ошибку `getUpdates`.
  Graceful shutdown — через `ctx.Done()` между retry-итерациями.
- **REQ-12.** Admin GET `/creators/applications/{id}` отдаёт `telegramLink`
  (nullable объект `{telegramUserId int64, telegramUsername *string,
  telegramFirstName *string, telegramLastName *string, linkedAt RFC3339}`)
  — чтобы модератор видел статус привязки и кому писать. Read-side делает
  второй SELECT в `service.GetByID` через `linkRepo.GetByApplicationID`.
- **REQ-13.** Coverage gate — ≥80% per-method на handler / service /
  repository (включая новый `creator_application_telegram_link.go`) /
  middleware / authz / `integration/telegram` (см. ниже про обновление
  Makefile-фильтра). `make test-unit-backend-coverage` зелёный.
- **REQ-14.** Все стандарты `docs/standards/` соблюдены. Реестр библиотек
  пополнен записью `Telegram Bot API → github.com/go-telegram/bot`.

## Вне scope

- Меню бота, `/help`, `/status`, `/cancel`. Все nicht-`/start` сообщения →
  fallback.
- TMA mini-app — chunk 2.
- Webhook-режим — отдельный chunk.
- Уведомления креатора (отклонение / запрос смены категории / договор).
- Лидер-выбор для multi-instance деплоя (на стартовом этапе один инстанс).
- Локализация (на 1-я итерация — только русский).

## Файлы для изменения

| Файл | Изменения |
|---|---|
| `backend/api/openapi.yaml` | Добавить в `CreatorApplicationDetailData` поле `telegramLink` (nullable объект `{telegramUserId: int64, telegramUsername: string nullable, telegramFirstName: string nullable, telegramLastName: string nullable, linkedAt: date-time}`). Schema-объект — отдельный, чтобы можно было его переиспользовать в будущих read-эндпоинтах. |
| `backend/api/openapi-test.yaml` | Добавить `POST /test/telegram/send-update`. Request: `{updateId, chatId, userId, text, username (optional), firstName (optional), lastName (optional)}` — позволяет e2e тестам инжектить полный From-объект Telegram. Response: `{data: {replies: [{chatId, text}]}}`. |
| `backend/internal/config/config.go` | `TelegramBotToken string` (envOptional, default ""), `TelegramPollingTimeout time.Duration` (default 30s). Условие запуска runner'а — токен непустой И `TelegramMock == false`; иначе диспатчер живёт только для test-endpoint. |
| `backend/.env.example` | Добавить `TELEGRAM_BOT_TOKEN=` (пусто, комментарий «получить у tech lead») и `TELEGRAM_POLLING_TIMEOUT=30s`. |
| `backend/internal/domain/creator_application.go` | Добавить тип `CreatorApplicationTelegramLink{ApplicationID string; TelegramUserID int64; TelegramUsername *string; TelegramFirstName *string; TelegramLastName *string; LinkedAt time.Time}`. Расширить `CreatorApplicationDetail` полем `TelegramLink *CreatorApplicationTelegramLink` (nil если не привязана). Тип `TelegramLinkInput{ApplicationID, TgUserID, TgUsername *string, TgFirstName *string, TgLastName *string}`. Тип `TelegramLinkResult{ApplicationID, Status, TelegramUserID, LinkedAt, Idempotent bool}`. Новые ошибочные коды: `CodeTelegramApplicationAlreadyLinked`, `CodeTelegramAccountAlreadyLinked`, `CodeTelegramApplicationNotActive`. Sentinel: `ErrTelegramApplicationLinkConflict` (PK conflict), `ErrTelegramAccountLinkConflict` (UNIQUE conflict). Константы лимитов: `TelegramUsernameMaxLen = 64`, `TelegramNameMaxLen = 256`. |
| `backend/internal/repository/creator_application.go` | Не трогаем — никаких новых колонок в `creator_applications`. Schema заявки остаётся прежней. |
| `backend/internal/repository/factory.go` | Добавить конструктор `NewCreatorApplicationTelegramLinkRepo(db dbutil.DB) CreatorApplicationTelegramLinkRepo`. |
| `backend/internal/service/audit_constants.go` | `AuditActionCreatorApplicationLinkTelegram = "creator_application_link_telegram"`. |
| `backend/internal/service/creator_application.go` | В `GetByID` добавить чтение `creator_application_telegram_links` через `repoFactory.NewCreatorApplicationTelegramLinkRepo(pool).GetByApplicationID(ctx, id)`. Если link отсутствует (`sql.ErrNoRows`) — `Detail.TelegramLink = nil`; если присутствует — заполнить. Расширить `CreatorApplicationRepoFactory` interface новым конструктором. |
| `backend/internal/handler/creator_application.go` | Обновить `domainCreatorApplicationDetailToAPI` — мапить `domain.CreatorApplicationTelegramLink → api.TelegramLink` (или nil). |
| `backend/internal/handler/server.go` | Не трогаем. Бот не идёт через HTTP-сервер — он вызывает `CreatorApplicationTelegramService` напрямую через диспатчер. Это держит ответственность чистой и не раздувает `Server` interface. |
| `backend/internal/handler/testapi.go` | Новый метод `SendTelegramUpdate(w, r)`: парсит `testapi.SendTelegramUpdateRequest`, формирует `telegram.IncomingUpdate`, вызывает `dispatcher.Dispatch(ctx, update)`, возвращает `spy.Drain(chatID)` в response. Расширить `TestAPIHandler` зависимостями `dispatcher TelegramDispatcher`, `spy TelegramSpy`. Расширить `NewTestAPIHandler` — два новых аргумента. |
| `backend/cmd/api/main.go` | Wiring (см. §«Шаги»). |
| `backend/.mockery.yaml` | Добавить `github.com/alikhanmurzayev/ugcboost/backend/internal/integration/telegram:` в список packages — mockery сгенерирует моки для интерфейсов клиента, диспатчера, сервиса. |
| `backend/Makefile` | Расширить awk-фильтр в `test-unit-backend-coverage`, чтобы включить `internal/integration/telegram/` в gate. |
| `docs/standards/backend-libraries.md` | Добавить строку «Telegram Bot API → `github.com/go-telegram/bot`» в реестр. |
| `_bmad-output/planning-artifacts/creator-onboarding-roadmap.md` | Перевести chunk 1 в `[~]` при старте build, в `[x]` после merge. Добавить ссылки на scout/plan-артефакты. |

## Файлы для создания

| Файл | Назначение |
|---|---|
| `backend/migrations/<ts>_creator_application_telegram_links.sql` | CREATE TABLE `creator_application_telegram_links` (PK `application_id` UUID FK ON DELETE CASCADE; `telegram_user_id BIGINT NOT NULL UNIQUE`; `telegram_username TEXT`; `telegram_first_name TEXT`; `telegram_last_name TEXT`; `linked_at TIMESTAMPTZ NOT NULL DEFAULT now()`). Down: DROP TABLE. |
| `legal-documents/PII_INVENTORY.md` | Новый документ. Закрывает pre-existing техдолг security.md § PII inventory. Записи на все существующие PII-поля в `creator_applications` (ИИН, ФИО, middle_name, phone, address, social handles, IP, UA в consents, document_version) + новые в `creator_application_telegram_links` (telegram_user_id, telegram_username, telegram_first_name, telegram_last_name). Поля: что хранится / зачем / legal basis / retention. |
| `backend/internal/integration/telegram/client.go` | Интерфейс `Client` + три реализации: `realClient` (обёртка над `go-telegram/bot`), `noopClient` (для prod без token / `TELEGRAM_MOCK=true`), `spyClient` (для test-endpoint, накапливает sent messages в memory, потокобезопасно через `sync.Mutex`). Конструктор `NewClient(cfg, logger) (Client, *spyClient, error)` — second return non-nil только в test-режиме. Кроме `Client` экспортируется `Spy` (alias для `*spyClient`) для test-handler. |
| `backend/internal/integration/telegram/dispatcher.go` | Интерфейс `Dispatcher` (`Dispatch(ctx, IncomingUpdate) error`). Тип `IncomingUpdate{UpdateID int64; ChatID int64; UserID int64; Text string; Username *string; FirstName *string; LastName *string}` — все user-метаданные несутся в update, чтобы start-handler мог их пробросить в service без дополнительных API-запросов. Реализация маршрутизирует по `Text`: `/start <payload>` → StartHandler, иначе → fallback reply через client. |
| `backend/internal/integration/telegram/start_handler.go` | Тип `StartHandler` с зависимостью на `CreatorApplicationTelegramService` и `Client`. Метод `Handle(ctx, IncomingUpdate, payload string)`: парсит UUID, конструирует `domain.TelegramLinkInput{ApplicationID, TgUserID, TgUsername, TgFirstName, TgLastName}` из update'а, вызывает `service.LinkTelegram(ctx, input, time.Now().UTC())`, маппит ошибки/успех на reply-текст из `messages.go`, отправляет reply через client. |
| `backend/internal/integration/telegram/runner.go` | Тип `PollingRunner` с зависимостями `Client`, `Dispatcher`, `Logger`. Метод `Run(ctx) error`: цикл `bot.GetUpdates`, на любую ошибку (включая HTTP 409 от Telegram при concurrent polling) — log + `select { case <-time.After(10*time.Second): case <-ctx.Done(): return nil }`, continue. На success — `for each update: dispatcher.Dispatch(ctx, ...)`. Метод `Wait()` — ждёт завершения goroutine (для closer). |
| `backend/internal/integration/telegram/messages.go` | Константы reply-текстов на русском (см. §«Тексты reply»). |
| `backend/internal/integration/telegram/dispatcher_test.go` | Unit: routing /start vs не-/start, идемпотентность к мусору, fallback reply. |
| `backend/internal/integration/telegram/start_handler_test.go` | Unit: happy / payload-не-UUID / app-not-found / status-not-active / conflict-другой-TG / conflict-эта-же-привязка / TG-привязан-к-другой-активной. |
| `backend/internal/integration/telegram/runner_test.go` | Unit: успешное чтение update'ов через mock-клиент, retry на ошибках (включая имитацию 409), выход по `ctx.Done()` во время sleep. |
| `backend/internal/integration/telegram/client_test.go` | Unit: spy сохраняет/Drain'ит сообщения корректно. real/noop без сетевых вызовов — не покрываются (тривиальные обёртки). |
| `backend/internal/service/creator_application_telegram.go` | Тип `CreatorApplicationTelegramService` с зависимостями `pool`, `repoFactory` (`CreatorApplicationTelegramRepoFactory` interface — узкий: `NewCreatorApplicationRepo`, `NewCreatorApplicationTelegramLinkRepo`, `NewAuditRepo`), `logger`. Метод `LinkTelegram(ctx, in domain.TelegramLinkInput, now time.Time) (*domain.TelegramLinkResult, error)`. Перед tx: trim+cap username/first_name/last_name (`TelegramUsernameMaxLen`/`TelegramNameMaxLen`), пустая строка после trim → nil. Внутри `dbutil.WithTx`: appRepo.GetByID (404 → ErrNotFound; status not active → BusinessError CodeTelegramApplicationNotActive) → linkRepo.GetByApplicationID: если есть и совпадает tg_user_id → idempotent return; есть и другой → BusinessError CodeTelegramApplicationAlreadyLinked → linkRepo.Insert (с username/first/last) (ловит UNIQUE на tg_user_id 23505 → BusinessError CodeTelegramAccountAlreadyLinked, ловит PK 23505 → re-SELECT → idempotent vs ApplicationAlreadyLinked) → auditRepo.Create с new_value включающим все 4 telegram-поля. После tx — log только идентификаторы (`application_id`, `tg_user_id`); username/first_name/last_name **никогда** не попадают в logger. |
| `backend/internal/service/creator_application_telegram_test.go` | Unit: все ветки (REQ-1..6, race). |
| `backend/internal/service/mocks/mock_creator_application_telegram_repo_factory.go` | Сгенерированный mock (через `make generate-mocks`). |
| `backend/internal/repository/creator_application_telegram_link.go` | Repo для `creator_application_telegram_links`. Row-struct `CreatorApplicationTelegramLinkRow{ApplicationID, TelegramUserID, TelegramUsername *string, TelegramFirstName *string, TelegramLastName *string, LinkedAt}` с тегами `db`/`insert`. Константы `TableCreatorApplicationTelegramLinks`, `CreatorApplicationTelegramLinkColumn{ApplicationID,TelegramUserID,TelegramUsername,TelegramFirstName,TelegramLastName,LinkedAt}`, имена UNIQUE и PK constraint'ов (`creator_application_telegram_links_pkey`, `creator_application_telegram_links_telegram_user_id_key`). Интерфейс `CreatorApplicationTelegramLinkRepo`: `Insert(ctx, row) (*Row, error)` (transl 23505 на UNIQUE → `ErrTelegramAccountLinkConflict`, на PK → `ErrTelegramApplicationLinkConflict`); `GetByApplicationID(ctx, appID) (*Row, error)` (sql.ErrNoRows passthrough). |
| `backend/internal/repository/creator_application_telegram_link_test.go` | pgxmock: Insert (happy + 23505/UNIQUE + 23505/PK + generic err) + GetByApplicationID (found + sql.ErrNoRows). |
| `backend/internal/repository/mocks/mock_creator_application_telegram_link_repo.go` | Сгенерированный mockery. |
| `backend/internal/integration/telegram/mocks/mock_*.go` | Сгенерированные моки (через `make generate-mocks`) для `Client`, `Dispatcher`, `CreatorApplicationTelegramService` (интерфейс на стороне dispatcher/start_handler — узкий: `LinkTelegram`). |
| `backend/e2e/telegram/telegram_link_test.go` | E2E (см. §«E2E сценарии»). Godoc-комментарий — нарратив на русском. |
| `backend/e2e/testutil/telegram.go` | Хелпер `SendTelegramUpdate(t, c, updateID, chatID, userID, text) []TelegramReply` поверх сгенерированного test-клиента. |

## Шаги реализации

1. [ ] **Стандарты.** Обновить `docs/standards/backend-libraries.md` — добавить строку про `github.com/go-telegram/bot`. Сделать первым шагом, чтобы любые последующие импорты были «в реестре».
2. [ ] **OpenAPI (prod + test).**
   - В `openapi.yaml` добавить новый schema-объект `TelegramLink{telegramUserId int64, telegramUsername string nullable, telegramFirstName string nullable, telegramLastName string nullable, linkedAt date-time}` и поле `telegramLink` (nullable, ссылка на этот объект) в `CreatorApplicationDetailData`. Не required.
   - В `openapi-test.yaml` добавить `POST /test/telegram/send-update` с request `{updateId int64, chatId int64, userId int64, text string, username string nullable, firstName string nullable, lastName string nullable}` и response `{data: {replies: [{chatId int64, text string}]}}`.
   - Запустить `make generate-api`. Убедиться, что diff содержит только сгенерированные файлы.
3. [ ] **Config + .env.**
   - `TelegramBotToken` (env optional, default ""), `TelegramPollingTimeout` (default 30s).
   - `.env.example`: пустой токен с пометкой «получить у tech lead», timeout.
4. [ ] **Миграция.** `make migrate-create NAME=creator_application_telegram_links`. Заполнить SQL:
   ```sql
   -- +goose Up
   CREATE TABLE creator_application_telegram_links (
       application_id      UUID PRIMARY KEY
                           REFERENCES creator_applications(id) ON DELETE CASCADE,
       telegram_user_id    BIGINT NOT NULL UNIQUE,
       telegram_username   TEXT,
       telegram_first_name TEXT,
       telegram_last_name  TEXT,
       linked_at           TIMESTAMPTZ NOT NULL DEFAULT now()
   );

   -- +goose Down
   DROP TABLE IF EXISTS creator_application_telegram_links;
   ```
   PK на `application_id` гарантирует 1:1 заявка↔link. UNIQUE на
   `telegram_user_id` гарантирует 1:1 Telegram↔link и автоматически
   создаёт нужный индекс. `ON DELETE CASCADE` чистит link при cleanup
   тестов и при будущем hard-delete заявки. Поля username/first_name/
   last_name nullable — не у каждого Telegram-пользователя они есть.
5. [ ] **Domain.** В `domain/creator_application.go`:
   - Тип `CreatorApplicationTelegramLink{ApplicationID string; TelegramUserID int64; TelegramUsername *string; TelegramFirstName *string; TelegramLastName *string; LinkedAt time.Time}`.
   - Расширить `CreatorApplicationDetail` полем `TelegramLink *CreatorApplicationTelegramLink` (nil = не привязана).
   - Тип `TelegramLinkInput{ApplicationID string; TgUserID int64; TgUsername *string; TgFirstName *string; TgLastName *string}` — domain-input для service.
   - Тип `TelegramLinkResult{ApplicationID string; Status string; TelegramUserID int64; LinkedAt time.Time; Idempotent bool}` — что service возвращает в start-handler.
   - Константы: `TelegramUsernameMaxLen = 64`, `TelegramNameMaxLen = 256`.
   - Коды ошибок: `CodeTelegramApplicationAlreadyLinked = "TELEGRAM_APPLICATION_ALREADY_LINKED"`, `CodeTelegramAccountAlreadyLinked = "TELEGRAM_ACCOUNT_ALREADY_LINKED"`, `CodeTelegramApplicationNotActive = "TELEGRAM_APPLICATION_NOT_ACTIVE"`.
   - Sentinel: `ErrTelegramApplicationLinkConflict` (PK conflict), `ErrTelegramAccountLinkConflict` (UNIQUE conflict). Godoc описывает контекст.
6. [ ] **Repository (новый файл `creator_application_telegram_link.go`).**
   - Row: `CreatorApplicationTelegramLinkRow{ApplicationID string `db:"application_id" insert:"application_id"`; TelegramUserID int64 `db:"telegram_user_id" insert:"telegram_user_id"`; TelegramUsername *string `db:"telegram_username" insert:"telegram_username"`; TelegramFirstName *string `db:"telegram_first_name" insert:"telegram_first_name"`; TelegramLastName *string `db:"telegram_last_name" insert:"telegram_last_name"`; LinkedAt time.Time `db:"linked_at" insert:"linked_at"`}`.
   - Константы:
     ```go
     const (
         TableCreatorApplicationTelegramLinks                       = "creator_application_telegram_links"
         CreatorApplicationTelegramLinkColumnApplicationID          = "application_id"
         CreatorApplicationTelegramLinkColumnTelegramUserID         = "telegram_user_id"
         CreatorApplicationTelegramLinkColumnLinkedAt               = "linked_at"
         CreatorApplicationTelegramLinksPK                          = "creator_application_telegram_links_pkey"
         CreatorApplicationTelegramLinksTelegramUserIDKey           = "creator_application_telegram_links_telegram_user_id_key"
     )
     ```
   - `selectColumns` / `insertMapper` — через stom (pattern из `creator_application.go`).
   - Метод `Insert(ctx, row) (*CreatorApplicationTelegramLinkRow, error)`:
     ```go
     q := sq.Insert(TableCreatorApplicationTelegramLinks).
         SetMap(toMap(row, creatorApplicationTelegramLinkInsertMapper)).
         Suffix(returningClause(creatorApplicationTelegramLinkSelectColumns))
     ```
     Translate `pgconn.PgError{Code:"23505"}`:
     - `ConstraintName == ...TelegramUserIDKey` → `domain.ErrTelegramAccountLinkConflict`
     - `ConstraintName == ...PK` → `domain.ErrTelegramApplicationLinkConflict`
   - Метод `GetByApplicationID(ctx, appID string) (*CreatorApplicationTelegramLinkRow, error)`:
     ```go
     q := sq.Select(creatorApplicationTelegramLinkSelectColumns...).
         From(TableCreatorApplicationTelegramLinks).
         Where(sq.Eq{CreatorApplicationTelegramLinkColumnApplicationID: appID})
     return dbutil.One[CreatorApplicationTelegramLinkRow](ctx, r.db, q)
     ```
     Возвращает `sql.ErrNoRows` (wrapped) если нет записи — service маппит на `Detail.TelegramLink = nil`.
   - Pgxmock-тесты: Insert (happy + 23505 на UNIQUE + 23505 на PK + generic), GetByApplicationID (found + sql.ErrNoRows). Capture SQL + аргументы.
7. [ ] **Service (creator_application).** В `GetByID` добавить чтение link через новый repo:
   ```go
   linkRow, err := s.repoFactory.NewCreatorApplicationTelegramLinkRepo(s.pool).GetByApplicationID(ctx, id)
   if err != nil && !errors.Is(err, sql.ErrNoRows) {
       return nil, fmt.Errorf("get telegram link: %w", err)
   }
   ```
   Если `linkRow != nil` → заполнить `detail.TelegramLink`. Расширить `CreatorApplicationRepoFactory` interface новым `NewCreatorApplicationTelegramLinkRepo`. Регенерировать mock через `make generate-mocks`.
8. [ ] **Service (creator_application_telegram).** Реализовать `CreatorApplicationTelegramService.LinkTelegram(ctx, in domain.TelegramLinkInput, now time.Time) (*domain.TelegramLinkResult, error)`:
   - Перед tx: `trimAndCap` username (64), first_name/last_name (256). Пустая строка после trim → nil. Безопасная санитизация: handle DoS через мегабайтный username.
   - Open `dbutil.WithTx(ctx, pool, ...)`. Внутри callback'а:
     - `appRepo.GetByID(ctx, appID)`:
       - `sql.ErrNoRows` (wrapped) → `domain.ErrNotFound` (start-handler маппит на reply «не нашли»).
       - не active статус → `domain.NewBusinessError(CodeTelegramApplicationNotActive, "Эта заявка не активна. ...")`.
     - `linkRepo.GetByApplicationID(ctx, appID)`:
       - found + same `telegram_user_id` → запоминаем `result = TelegramLinkResult{...Idempotent: true}`, **rollback не нужен** — return nil из callback (commit no-op tx, чисто). Audit **не** пишется.
       - found + другой `telegram_user_id` → `BusinessError(CodeTelegramApplicationAlreadyLinked, "Эта заявка уже связана с другим Telegram-аккаунтом. ...")`.
       - `sql.ErrNoRows` → продолжаем к INSERT.
     - `linkRepo.Insert(...)` с `linked_at = now`:
       - success → fall through, audit, result.
       - `domain.ErrTelegramAccountLinkConflict` (UNIQUE на tg_id) → `BusinessError(CodeTelegramAccountAlreadyLinked, "У вас уже есть активная заявка ...")`.
       - `domain.ErrTelegramApplicationLinkConflict` (PK conflict — concurrent INSERT для того же appID) → повторяем `linkRepo.GetByApplicationID(ctx, appID)`: либо same tg → idempotent, либо другой tg → `CodeTelegramApplicationAlreadyLinked`. Это закрывает гонку preflight-SELECT vs INSERT.
       - другая ошибка → `fmt.Errorf("insert telegram link: %w", err)`.
     - `auditRepo.Create(...)`:
       - action: `AuditActionCreatorApplicationLinkTelegram`,
       - entity_type: `AuditEntityTypeCreatorApplication`,
       - entity_id: appID,
       - actor_id: NULL (system),
       - actor_role: пусто,
       - new_value: `{telegram_user_id, telegram_username, telegram_first_name, telegram_last_name}` (PII — допустимо в audit_logs per security.md).
     - return nil из callback → commit.
   - **После WithTx:** если `result.Idempotent == false` — `logger.Info(ctx, "telegram linked to creator application", "application_id", appID, "telegram_user_id", tgUserID)`. **Только идентификаторы**; username/first_name/last_name **никогда** не попадают в logger (security.md § PII в stdout). Idempotent-случай можно логировать на debug-уровне («repeated /start for same link») — без ПД.
   - Возврат: `*domain.TelegramLinkResult`. start-handler использует только `Status` для проверки, что заявка не ушла в неактивный статус между транзакциями (хотя tx-snapshot уже это закрывает).
9. [ ] **Audit constants.** `AuditActionCreatorApplicationLinkTelegram`.
10. [ ] **Telegram package — client.** В `internal/integration/telegram/`:
    - Интерфейс `Client { SendMessage(ctx, chatID int64, text string) error; GetUpdates(ctx, offset int, timeout time.Duration) ([]Update, error) }`. Update — внутренний struct (см. dispatcher).
    - `realClient` — обёртка над `*bot.Bot`. Сетевые вызовы внутри.
    - `noopClient` — `SendMessage` логирует на DEBUG и возвращает nil; `GetUpdates` возвращает пустой слайс. Для prod без token и для CI с `TELEGRAM_MOCK=true`.
    - `spyClient` — `SendMessage` сохраняет `(chatID, text)` в `sync.Mutex`-защищённый slice. Метод `Drain(chatID int64) []SentMessage` — атомарно забирает и очищает свой slice по chatID. `GetUpdates` — пустой (тесты инжектят update'ы прямо в диспатчер через test-endpoint, polling-loop в test-режиме off).
    - Конструктор `NewClient(cfg config.Config, logger Logger) (Client, *spyClient, error)`:
      - `cfg.EnableTestEndpoints` → spy (return spy as both Client and *spyClient).
      - `cfg.TelegramMock || cfg.TelegramBotToken == ""` → noop.
      - Иначе real.
11. [ ] **Telegram package — dispatcher.** Структура `Dispatcher` с зависимостями `client Client`, `startHandler StartHandler`, `messages Messages`, `logger Logger`. Метод `Dispatch(ctx, IncomingUpdate)`:
    - Если `Text` начинается с `/start ` → `startHandler.Handle(ctx, update, strings.TrimPrefix(...))`.
    - Если `Text == "/start"` → reply `messages.StartNoPayload()`.
    - Иначе → reply `messages.Fallback()`.
12. [ ] **Telegram package — start handler.** Зависимости: `service CreatorApplicationTelegramService`, `client Client`, `messages Messages`, `logger Logger`. Метод `Handle(ctx, update, rawPayload string)`:
    - `payload := strings.TrimSpace(rawPayload)`.
    - `appID, err := uuid.Parse(payload)` → at err: reply `messages.InvalidPayload()`.
    - `input := domain.TelegramLinkInput{ApplicationID: appID.String(), TgUserID: update.UserID, TgUsername: update.Username, TgFirstName: update.FirstName, TgLastName: update.LastName}`.
    - `_, err := service.LinkTelegram(ctx, input, time.Now().UTC())` → maps:
      - nil → `messages.LinkSuccess()`
      - `domain.ErrNotFound` → `messages.ApplicationNotFound()`
      - `BusinessError{Code: CodeTelegramApplicationNotActive}` → `messages.ApplicationNotActive()`
      - `BusinessError{Code: CodeTelegramApplicationAlreadyLinked}` → `messages.ApplicationAlreadyLinked()`
      - `BusinessError{Code: CodeTelegramAccountAlreadyLinked}` → `messages.AccountAlreadyLinked()`
      - default → `messages.InternalError()`, log error
    - `client.SendMessage(ctx, update.ChatID, replyText)` — failure логировать, не пробрасывать (в polling — лог достаточно, Telegram update останется неподтверждённым и придёт снова).
13. [ ] **Telegram package — messages.go.** См. §«Тексты reply».
14. [ ] **Telegram package — runner.** `PollingRunner.Run(ctx)`:
    ```go
    offset := 0
    for {
        if ctx.Err() != nil { return nil }
        updates, err := client.GetUpdates(ctx, offset, cfg.TelegramPollingTimeout)
        if err != nil {
            log.Warn(ctx, "telegram getUpdates failed, retrying", "error", err)
            select {
            case <-time.After(10 * time.Second):
            case <-ctx.Done():
                return nil
            }
            continue
        }
        for _, u := range updates {
            dispatcher.Dispatch(ctx, u)
            if u.UpdateID >= offset { offset = u.UpdateID + 1 }
        }
    }
    ```
    На concurrent polling (rolling deploy окно) Telegram возвращает HTTP 409 _«terminated by other getUpdates request»_ — попадает в общую ветку «getUpdates failed, retrying». В логах увидим несколько таких записей в течение деплоя — это ожидаемое поведение, не ошибка кода. После SIGTERM старого контейнера новый сразу получает updates без 409. Метод `Wait()` — ждёт `done chan struct{}`, сигналимый при выходе из Run.
15. [ ] **Test-endpoint.** В `handler/testapi.go`:
    - Расширить `TestAPIHandler` зависимостями `dispatcher TelegramDispatcher`, `spy TelegramSpy`. Интерфейсы определить здесь же (узкие, accept interfaces).
    - Расширить `NewTestAPIHandler` сигнатурой.
    - Добавить метод `SendTelegramUpdate(w, r)`: декод req → `telegram.IncomingUpdate{...}` → `dispatcher.Dispatch(ctx, ...)` → `replies := spy.Drain(chatID)` → response.
    - В `openapi-test.yaml` это уже отражено (шаг 2).
16. [ ] **Cmd wiring (`main.go`).**
    ```go
    // After: cron registered, repoFactory built.
    tgClient, tgSpy, err := telegram.NewClient(cfg, appLogger)
    if err != nil {
        return fmt.Errorf("init telegram client: %w", err)
    }
    tgLinkSvc := service.NewCreatorApplicationTelegramService(pool, repoFactory, appLogger)
    tgMessages := telegram.DefaultMessages()
    tgStart := telegram.NewStartHandler(tgLinkSvc, tgClient, tgMessages, appLogger)
    tgDispatcher := telegram.NewDispatcher(tgClient, tgStart, tgMessages, appLogger)

    if !cfg.TelegramMock && cfg.TelegramBotToken != "" {
        tgRunner := telegram.NewPollingRunner(tgClient, tgDispatcher, cfg.TelegramPollingTimeout, appLogger)
        runnerCtx, runnerCancel := context.WithCancel(context.Background())
        go func() {
            if err := tgRunner.Run(runnerCtx); err != nil {
                appLogger.Error(ctx, "telegram runner stopped with error", "error", err)
            }
        }()
        cl.Add("telegram-runner", func(_ context.Context) error {
            runnerCancel()
            tgRunner.Wait()
            return nil
        })
        appLogger.Info(ctx, "telegram bot started (long polling)")
    } else {
        appLogger.Info(ctx, "telegram bot disabled (no token / mock mode)")
    }
    ```
    Test-handler передачу dispatcher/spy — добавить в `NewTestAPIHandler` вызов:
    ```go
    testHandler := handler.NewTestAPIHandler(authSvc, pool, repoFactory, resetTokenStore, tgDispatcher, tgSpy, appLogger)
    ```
    `tgSpy` может быть `nil` в prod (test-handler не регистрируется при `EnableTestEndpoints == false`).
17. [ ] **Mockery.** Расширить `.mockery.yaml` пакетом `internal/integration/telegram`. Запустить `make generate-mocks`. Mocks: `MockClient`, `MockDispatcher`, `MockCreatorApplicationTelegramService` (telegram-side interface), `MockTelegramDispatcher` (testapi-side), `MockTelegramSpy` (testapi-side), `MockCreatorApplicationTelegramRepoFactory` (service-side), `MockCreatorApplicationTelegramLinkRepo` (repository-side). Tests используют их.
18. [ ] **Coverage gate.** Расширить awk-фильтр в `Makefile` (`test-unit-backend-coverage`):
    Текущий: `$$1 ~ /\/(handler|service|repository|middleware|authz)\//`.
    Новый: `$$1 ~ /\/(handler|service|repository|middleware|authz|integration\/telegram)\//`.
    Mocks внутри `internal/integration/telegram/mocks/` уже исключаются паттерном `*/mocks/`.
19. [ ] **E2E testutil.** В `backend/e2e/testutil/telegram.go` — `SendTelegramUpdate(t, c, params)` поверх сгенерированного `testclient`, где `params` — struct `TelegramUpdateParams{UpdateID, ChatID, UserID int64; Text string; Username, FirstName, LastName *string}`. Возвращает массив reply (`[]testclient.TelegramReply`). Хелпер для билдера дефолтных значений (`DefaultTelegramUpdateParams(t)`) — генерация уникального tgUserID/updateID, дефолтные тексты `username/first_name/last_name`.
20. [ ] **E2E тесты.** `backend/e2e/telegram/telegram_link_test.go` — package-godoc на русском, нарратив. Сценарии — см. §«E2E сценарии». Каждый Test — `t.Parallel()`. Cleanup — через существующий `RegisterCreatorApplicationCleanup`. Уникальность по IIN — через `UniqueIIN`. Уникальность по `tgUserID` — через `time.Now().UnixNano()` с локальным сидом для воспроизводимости.
20a. [ ] **PII inventory.** Создать `legal-documents/PII_INVENTORY.md`. Покрыть все существующие PII-поля проекта: `creator_applications` (last_name, first_name, middle_name, iin, birth_date, phone, city, address); `creator_application_socials` (handle); `creator_application_consents` (ip_address, user_agent, document_version); + новые `creator_application_telegram_links` (telegram_user_id, telegram_username, telegram_first_name, telegram_last_name, linked_at). Для каждого поля: что хранится / зачем / legal basis (consent через privacy-policy.md / legitimate interest для аудита) / retention. Документ — living, дополняется при добавлении новых PII-полей в БД.

21. [ ] **Roadmap.** В `_bmad-output/planning-artifacts/creator-onboarding-roadmap.md` — chunk 1 в `[~]`, добавить блок «Артефакты chunk 1: scout / plan» со ссылками на оба md.
22. [ ] **Финальные проверки.**
    - `make generate-api` (изменения только в gen-файлах).
    - `make generate-mocks` (изменения в `mocks/`).
    - `make build-backend` — без warnings.
    - `make lint-backend` — 0 issues.
    - `make test-unit-backend` — green.
    - `make test-unit-backend-coverage` — gate passed.
    - `make test-e2e-backend` — green (включая новый telegram-пакет).
    - PII-grep: `make compose-up && make start-backend && curl ... && docker logs ugcboost-backend 2>&1 | grep -E '<iin>|<phone>|<handle>' | wc -l` → 0.
23. [ ] **Roadmap финал.** Chunk 1 → `[x]` после merge (ручной шаг при ревью).

## Тексты reply (русский, официальный)

```go
// backend/internal/integration/telegram/messages.go

const (
    LinkSuccess = "Здравствуйте! Заявка успешно связана с вашим Telegram-аккаунтом. " +
        "В ближайшее время в этом чате откроется мини-приложение со статусом обработки заявки."

    StartNoPayload = "Здравствуйте! Чтобы связать ваш Telegram-аккаунт с заявкой, " +
        "перейдите по ссылке со страницы успешной подачи заявки на ugcboost.kz."

    InvalidPayload = "Не удалось распознать ссылку. " +
        "Перейдите по ссылке со страницы успешной подачи заявки на ugcboost.kz."

    ApplicationNotFound = "Не удалось найти заявку по этой ссылке. " +
        "Возможно, заявка ещё не подана. Подайте заявку на ugcboost.kz."

    ApplicationNotActive = "Эта заявка не активна. " +
        "Если вам нужно подать новую заявку, перейдите на ugcboost.kz."

    ApplicationAlreadyLinked = "Эта заявка уже связана с другим Telegram-аккаунтом. " +
        "Если это ошибка, обратитесь в поддержку."

    AccountAlreadyLinked = "У вас уже есть активная заявка, связанная с этим Telegram-аккаунтом. " +
        "Дождитесь решения по ней или обратитесь в поддержку."

    Fallback = "Я понимаю только команду /start со специальной ссылкой. " +
        "Перейдите по ссылке со страницы успешной подачи заявки на ugcboost.kz."

    InternalError = "Произошла внутренняя ошибка. Попробуйте ещё раз через минуту. " +
        "Если ошибка повторится — обратитесь в поддержку."
)
```

`messages.go` экспортирует не строки, а методы интерфейса `Messages`, чтобы в
тестах подменить тривиально (или использовать как value-struct с теми же
полями для чистоты). Финальная форма — на усмотрение build-агента, но текст
**неизменен** без эскалации.

## E2E сценарии

| Тест | Сценарий |
|---|---|
| `TestTelegramLinkHappyPath` | Submit application → /start с appID + username/firstName/lastName → reply == LinkSuccess → admin GET показывает `telegramLink {telegramUserId, telegramUsername, telegramFirstName, telegramLastName, linkedAt}` со всеми переданными значениями → audit имеет `creator_application_link_telegram` action с new_value, содержащим все 4 telegram-поля. |
| `TestTelegramLinkIdempotent` | Happy → второй /start с тем же tgUserID/appID → reply == LinkSuccess (или конкретный idempotent-text — фиксируется в build) → admin GET тот же → audit-rows count = 1 (только initial link). |
| `TestTelegramLinkConflictAnotherTelegram` | Happy → /start с тем же appID от ДРУГОГО tgUserID → reply == ApplicationAlreadyLinked → admin GET — `telegramLink.telegramUserId` неизменён. |
| `TestTelegramLinkConflictAnotherApplication` | Submit app1 → /start app1 (TG-A). Submit app2 (другой IIN, тот же socials? нет — social handles unique) → /start app2 от того же TG-A → reply == AccountAlreadyLinked. |
| `TestTelegramLinkApplicationNotFound` | /start с UUID, не существующим в БД → reply == ApplicationNotFound. БД не меняется. |
| `TestTelegramLinkInvalidPayload` | /start с не-UUID payload → reply == InvalidPayload. БД не меняется. |
| `TestTelegramLinkNoPayload` | /start без payload → reply == StartNoPayload. |
| `TestTelegramLinkUnknownCommand` | /help → reply == Fallback. |
| `TestTelegramLinkPIIGuard` | После happy-path: `docker logs backend` grep по IIN/phone/socials handle + telegram username/firstName/lastName (использованным в setup) → 0. |

Все Test* — `t.Parallel()`. Сетап admin-клиента — `testutil.SetupAdminClient`.
Уникальность tgUserID — `int64(time.Now().UnixNano() & math.MaxInt32)` плюс
inc-counter, чтобы не пересекаться между параллельными тестами.

## Стратегия тестирования

**Unit:**
- Repository (новый `creator_application_telegram_link.go`): `Insert` (happy с username/first/last + happy с nil-полями, 23505/UNIQUE → ErrTelegramAccountLinkConflict, 23505/PK → ErrTelegramApplicationLinkConflict, generic err); `GetByApplicationID` (found, sql.ErrNoRows). `pgxmock` + capture SQL.
- Service `creator_application_telegram`: все ветки REQ-1..6 + race на PK + after-tx log + idempotent vs new audit разделение + cap длины username/first/last (input с >max → trimmed в Insert) + nil-passthrough.
- Service `creator_application` (existing): `GetByID` теперь читает link → extend test (link present с full metadata, link absent).
- Repository (existing) `creator_application.go` — без изменений; existing tests unchanged.
- Dispatcher: routing, fallback.
- StartHandler: все маппинги ошибок.
- Runner: успешный цикл, retry на ошибке (включая имитацию 409 от Telegram), выход по `ctx.Done()` во время sleep. Mock client.
- Client (spy): Drain атомарность.

**E2E:** см. §«E2E сценарии».

**Coverage:** ≥80% per-method на handler / service / repository / middleware /
authz / integration/telegram. Gate проверяет — фильтр awk расширяется в шаге 18.

## Оценка рисков

| Риск | Вероятность | Митигация |
|---|---|---|
| `go-telegram/bot` API не поддерживает нужный сценарий | Низкая | Изолирован за `Client`-интерфейсом; смена на `telego` — замена `realClient`. |
| Long polling блокирует goroutine при сетевой ошибке | Средняя | Fixed 10s retry в runner, выход по `ctx.Done()` через `select`. Unit-тест на retry-цикл. |
| Race на UNIQUE/PK → 500 вместо domain-error | Низкая | SQLSTATE 23505 + ConstraintName → `ErrTelegramAccountLinkConflict` (UNIQUE) или `ErrTelegramApplicationLinkConflict` (PK). Service ловит PK-conflict → re-SELECT → idempotent vs ApplicationAlreadyLinked. Тесты на оба варианта. |
| Test-endpoint случайно остаётся в prod | Низкая | `EnableTestEndpoints = (Environment != production)` — существующая защита. |
| PII утечка в stdout (username, first_name, last_name) | Средняя | Service-слой принимает username/first/last в `TelegramLinkInput`, пишет в DB и в `audit_logs.new_value`, но **никогда** в `logger.Info` — там только `application_id` и `tg_user_id`. PII-guard e2e тест grep'ает stdout по username/first_name/last_name + ИИН/phone/handle = 0. |
| `TELEGRAM_BOT_TOKEN` пустой ломает старт | Низкая | `noopClient` fallback при пустом token. Лог про disabled. |
| Заявка меняет статус на rejected между GetByID и LinkTelegram внутри tx | Низкая | Tx serializable не нужна — partial unique index покрывает race; статусная проверка происходит на uncomitted tx-snapshot, не критично для бизнес-логики (хуже — креатор увидит «not active», админ всё равно перепроверит). |
| Двойной polling во время Dokploy rolling deploy — Telegram 409, шум в логах | Принимаем как норму. Telegram сам гарантирует один активный polling: концепция «terminated by other getUpdates request» — старый контейнер начнёт получать 409, retry'ит каждые 10с до SIGTERM от Dokploy. Окно — 10-60 сек. Теоретический дубль одного update'а закрыт идемпотентностью бизнес-логики (повторный `/start` = same result, без дубля audit). Future horizontal scaling — отдельный chunk: `backend-bot` сервис в Dokploy с replicas=1 (сейчас не нужно). |
| Mockery не сгенерирует моки для нового пакета | Низкая | Расширение `.mockery.yaml` тем же PR. CI проверит. |

## План отката

- Migration down (`make migrate-reset`) → таблица `creator_application_telegram_links` удалена. `creator_applications` не затронута.
- Revert PR → код, openapi, env, реестр библиотек откатываются.
- Token бота можно отозвать через @BotFather (вне репозитория) — это операция infra, не код.
- Существующая заявка-функциональность не зависит от привязки (link — отдельная таблица, GET-handler толерантен к отсутствию link) → откат не задевает creator_application submit.

---

**Артефакт готов для build.** Следующий шаг — `/clear` сессии и `/build` с
этим планом. Build-агент обязан полностью загрузить `docs/standards/`
(преамбула выше) перед первой правкой кода.

---

## Addendum (2026-04-29, build phase)

При реализации обнаружено, что `github.com/go-telegram/bot` v1.20.0 не
публикует `GetUpdates(offset, timeout)` напрямую — библиотека инкапсулирует
свой long-polling цикл внутри `Bot.Start(ctx)` через приватный `getUpdates`.
Архитектура нашего `Client` interface (`SendMessage` + `GetUpdates`) требует
прямого контроля над polling-циклом, поэтому realClient реализован через
stdlib `net/http`: два POST-эндпойнта (`getUpdates`, `sendMessage`), JSON
envelope, sanitised error handling. Реестр библиотек (`docs/standards/backend-libraries.md`)
обновлён — строка про `go-telegram/bot` заменена на `net/http (stdlib)`.

Решение оправдано: библиотека добавляла транзитивные зависимости и закрытый
polling-цикл, который пришлось бы либо обходить, либо переписывать
realClient на колбэк-стиль. Свой HTTP-клиент в пределах одного файла
(`backend/internal/integration/telegram/client.go`) проще и тестируется
через `httptest.NewServer` без внешних точек.
