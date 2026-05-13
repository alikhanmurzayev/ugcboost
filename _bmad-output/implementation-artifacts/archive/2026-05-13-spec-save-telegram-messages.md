---
title: 'Сохранение Telegram-сообщений (in + out) и ручка чтения по chat_id'
type: 'feature'
created: '2026-05-13'
status: 'done'
baseline_commit: 'dc0b655'
context:
  - '_bmad-output/implementation-artifacts/intent-save-telegram-messages.md'
  - 'docs/standards/backend-repository.md'
  - 'docs/standards/backend-transactions.md'
---

<frozen-after-approval reason="human-owned intent — do not modify unless human renegotiates">

## Intent

**Problem:** У нас нет хронологической ленты переписки бота с пользователями: входящие сообщения (всё, что не `/start`) только триггерят fallback-ответ и теряются, исходящие (Notifier / Send*Campaign*) уходят без следа в БД — нечем дебажить «мне ничего не пришло» и нечем закрыть будущую админ-страницу истории чата.

**Approach:** Новая таблица `telegram_messages` (привязка только по `chat_id`, без FK на доменные сущности) + `MessageRecorder`-сервис, который вызывается синхронно из `Handler.Handle` (inbound, после private-фильтра) и из новой обёртки `RecordingSender` поверх любого `Sender` (outbound). Плюс одна admin-only ручка `GET /telegram-messages?chatId&limit&cursor` для e2e сейчас и админ-UI в следующем чанке.

## Boundaries & Constraints

**Always:**
- Преамбула стандартов: реализатор обязан полностью прочитать `docs/standards/`. Все правила (слои, codegen, EAFP, RepoFactory, naming, security, тесты) применяются буквально.
- Recorder — sync, без бизнес-транзакции. Любая ошибка INSERT (кроме dedup-23505) — `logger.Error` с `chat_id`/`direction`/`error`, **без `text`** (PII). Caller продолжает работу.
- Inbound dedup — partial UNIQUE `(chat_id, telegram_message_id) WHERE direction='inbound' AND telegram_message_id IS NOT NULL` + EAFP catch `pgconn 23505` → доменный sentinel `ErrTelegramMessageAlreadyRecorded`; recorder логирует Debug, не Error.
- Inbound пишется **только для private chat'ов** (после фильтра `chatTypePrivate` в `Handler.Handle`).
- Outbound пишется на каждый вызов `sender.SendMessage` независимо от ParseMode / ReplyMarkup; каждый retry `Notifier.fire` = отдельный ряд.
- E2E recording тестируется **расширением существующих** `backend/e2e/*` тестов через новый testutil-хелпер; отдельных recording-only тестов не пишем (кроме самой ручки чтения).
- `make test-unit-backend-coverage` ≥ 80% на каждый новый identifier; `-race` обязателен.

**Ask First:**
- Если pgcrypto не подключен (он подключен в `00001_init.sql`, но проверь) — спросить, ставить ли его этой миграцией или отдельной.
- Если existing e2e-тест из списка под расширение требует non-trivial рефакторинга testutil (а не одного `AssertTelegramMessageRecorded` вызова) — спросить, прежде чем переписывать.
- Если миграция требует backfill (она не требует — таблица новая) — спросить.

**Never:**
- НЕТ FK на `creators`/`creator_applications`/`campaigns` в `telegram_messages` (привязка только по `chat_id`).
- НЕТ записи non-private inbound (group / supergroup / channel chats — фильтруются).
- НЕТ записи `text`-поля сообщения в stdout-логи (только `chat_id`/`direction`/`error` где применимо).
- НЕТ partial-CHECK на `status`/`error` per direction в миграции (invariant в коде).
- НЕТ race-теста на partial UNIQUE (осознанный пропуск; bot single-instance).
- НЕТ изменений UI (web/tma/landing).
- НЕТ изменений на сущностях creator/application — обогащение `telegramChatId` отложено в следующий чанк.
- НЕТ async fire-and-forget горутины в recorder'е (sync через caller goroutine).

## I/O & Edge-Case Matrix

| Scenario | Input / State | Expected Output / Behavior | Error Handling |
|----------|--------------|---------------------------|----------------|
| Inbound private text | `update.Message` от private-chat'а с непустым `Text` | Ряд: `direction='inbound'`, `text=<text>`, `telegram_message_id=<update.Message.ID>`, `telegram_username=<from.Username или NULL>`, `status=NULL`. Dispatcher `/start` срабатывает как раньше. | INSERT error (не 23505) → log.Error без `text` + продолжаем dispatch |
| Inbound private non-text | `update.Message` от private-chat'а без `Text` (фото / voice / стикер / forward) | Ряд с `text=''`, `telegram_message_id=<update.Message.ID>`, остальные поля как у text-варианта. | Аналогично |
| Inbound non-private | `update.Message.Chat.Type != 'private'` | **Ничего не пишем**, ничего не отвечаем (как сейчас). | N/A |
| Inbound from=nil | `update.Message.From == nil` или `From.ID <= 0` | **Ничего не пишем**, ничего не отвечаем (как сейчас). | N/A |
| Inbound dedup | Тот же `(chat_id, telegram_message_id, direction='inbound')` второй раз (Telegram retry) | INSERT возвращает 23505 → repo возвращает `ErrTelegramMessageAlreadyRecorded` → recorder log.Debug, не Error, не падает. Dispatcher `/start` всё равно отрабатывает (идемпотентность handler'a — не наша забота тут). | N/A |
| Outbound sent | `inner.SendMessage` вернул `(*models.Message, nil)` | Ряд: `direction='outbound'`, `text=<params.Text>`, `telegram_message_id=<msg.ID>`, `telegram_username=NULL`, `status='sent'`, `error=NULL`. Caller получает (msg, nil). | INSERT error → log.Error + caller всё равно получает (msg, nil) |
| Outbound failed | `inner.SendMessage` вернул `(nil, err)` (bot blocked, network, 4xx) | Ряд: `status='failed'`, `error=err.Error()`, `telegram_message_id=NULL`. Caller получает (nil, err) — тот же err неизменно. | INSERT error → log.Error + caller всё равно получает (nil, err) |
| Outbound retry | Notifier.fire делает 3 попытки: failed, failed, sent | 3 отдельных ряда: 2 со `status='failed'` + 1 со `status='sent'`. | N/A |
| Read happy first page | `GET /telegram-messages?chatId=X&limit=5` под admin, в БД 12 рядов для X | 200 `{items: [...5 рядов desc by created_at, id], nextCursor: <opaque>}` | N/A |
| Read happy last page | Тот же запрос с `cursor` от предыдущей страницы, осталось 2 ряда | 200 `{items: [...2], nextCursor: null}` | N/A |
| Read empty | `chatId=Y`, в БД 0 рядов для Y | 200 `{items: [], nextCursor: null}` | N/A |
| Read 422 invalid | `limit=0`, `limit=101`, `chatId` отсутствует, `cursor` не декодируется как base64-JSON с полями `createdAt`+`id` | 422 ErrorResponse `CodeValidation` | N/A |
| Read 401 / 403 | Без token / brand_manager token | 401 / 403 ErrorResponse через стандартный mapper | N/A |

</frozen-after-approval>

## Code Map

- `backend/api/openapi.yaml` -- добавить `GET /telegram-messages` + schemas `TelegramMessageList`, `TelegramMessage`.
- `backend/api/openapi-test.yaml` -- добавить `POST /test/seed-telegram-message` + `DELETE /test/telegram-messages?chatId={int64}`.
- `backend/migrations/<ts>_telegram_messages.sql` -- новая миграция: таблица + CHECK на `direction`/`status` + INDEX `(chat_id, created_at DESC)` + partial UNIQUE `(chat_id, telegram_message_id) WHERE direction='inbound' AND telegram_message_id IS NOT NULL`. Down: `DROP TABLE`.
- `backend/internal/repository/telegram_message.go` -- `TelegramMessageRow` (теги `db`/`insert`), приватная `telegramMessageRepository`, экспортируемый интерфейс `TelegramMessageRepo` с `Insert(ctx, *Row) error` и `ListByChat(ctx, chatID, cursor *Cursor, limit int) ([]*Row, error)`. Колонки через константы `TelegramMessageColumn*`, таблица `TableTelegramMessages`. EAFP catch `pgconn 23505` → `ErrTelegramMessageAlreadyRecorded`.
- `backend/internal/repository/factory.go` -- `NewTelegramMessageRepo(db)`.
- `backend/internal/domain/telegram_message.go` -- `ErrTelegramMessageAlreadyRecorded` sentinel, `TelegramMessageDirection*` константы, `TelegramMessageStatus*` константы, `Cursor` (или общий `cursor` в shared util — на усмотрение реализатора).
- `backend/internal/telegram/recorder.go` -- `MessageRecorder` interface (`RecordInbound(ctx, *models.Update)`, `RecordOutbound(ctx, params, msg, err)`); `*MessageRecorderService` (поля: `pool dbutil.Pool`, `repoFactory RecorderRepoFactory`, `log logger.Logger`); конструктор `NewMessageRecorderService(pool, repoFactory, log)`. Узкий interface `RecorderRepoFactory` с одним методом `NewTelegramMessageRepo(db)`.
- `backend/internal/telegram/recording_sender.go` -- `RecordingSender` (поля: `inner Sender`, `recorder MessageRecorder`); конструктор `NewRecordingSender(inner, recorder)`; метод `SendMessage` делегирует в `inner.SendMessage`, после — `recorder.RecordOutbound(ctx, params, msg, err)`, возвращает `(msg, err)` неизменно.
- `backend/internal/telegram/handler.go` -- `Handler` получает новый параметр `recorder MessageRecorder`. В `Handle`: после nil + non-private + from-nil фильтра, до `startPayload`, вызов `h.recorder.RecordInbound(ctx, update)`.
- `backend/internal/service/telegram_message.go` -- `*TelegramMessageService` (поля: `pool`, `repoFactory TelegramMessageRepoFactory`); метод `ListByChat(ctx, chatID int64, cursor *domain.Cursor, limit int) ([]*repository.TelegramMessageRow, *domain.Cursor, error)`. Limit+1 паттерн → hasMore → формирует nextCursor из последнего ряда.
- `backend/internal/authz/telegram_message.go` -- `CanReadTelegramMessages(ctx) error` — admin-only по precedent'у `CanRejectCreatorApplication`.
- `backend/internal/handler/telegram_message.go` -- `*Server.ListTelegramMessages(ctx, request)` — authz check → decode cursor (422 на невалид) → service call → маппинг row → generated DTO → 200.
- `backend/internal/handler/server.go` -- расширить `AuthzService` (`CanReadTelegramMessages`) + добавить `TelegramMessageService` interface с `ListByChat`.
- `backend/internal/handler/testapi.go` -- `SeedTelegramMessage(ctx, request)` (прямая запись через repo) + `CleanupTelegramMessages(ctx, request)` (hard-delete по `chat_id`); либо расширить enum `CleanupEntity` `Type` на `'telegram_message'` — на усмотрение реализатора (новые методы чище).
- `backend/cmd/api/telegram.go` -- `setupTelegram` собирает `*MessageRecorderService` (новый параметр `repoFactory`) и оборачивает финальный `sender` в `RecordingSender` ПЕРЕД `NewNotifier`. `Handler` создаётся с recorder (новый параметр `NewHandler`).
- `backend/cmd/api/main.go` -- передать `repoFactory` и `recorder` в `setupTelegram` и в `NewHandler`; передать `*TelegramMessageService` в `NewServer`.
- `backend/e2e/testutil/telegram_messages.go` (новый) -- `AssertTelegramMessageRecorded(t, chatID, direction, textMatcher)` — вызывает `GET /telegram-messages?chatId&limit=100` под admin-token, ассертит наличие/содержимое; `SeedTelegramMessage(t, ...)` — вызывает testapi; `CleanupTelegramMessagesByChat(t, chatID)` — defer-аналог.
- `backend/e2e/telegram_messages/list_test.go` (новый) -- header на русском, нарратив. `TestListTelegramMessages` с `t.Parallel()`; t.Run'ы: cursor pagination, empty, 401/403/422.
- Существующие e2e под расширение (вызов `AssertTelegramMessageRecorded`):
  - `backend/e2e/telegram/telegram_test.go` -- inbound `/start` happy + fallback (любой текст не-`/start` → inbound row).
  - `backend/e2e/creator_applications/approve_test.go` -- outbound row после `NotifyApplicationApproved`.
  - `backend/e2e/creator_applications/reject_test.go` -- outbound row после `NotifyApplicationRejected`.
  - `backend/e2e/creator_applications/manual_verify_test.go` + `backend/e2e/webhooks/sendpulse_instagram_test.go` -- outbound row после `NotifyVerificationApproved`.
  - `backend/e2e/campaign_creator/campaign_notify_test.go` -- outbound rows для invite/remind, `status='failed'` + `error` для `bot_blocked` ветки (через `RegisterTelegramSpyFailNext`).
  - `backend/e2e/contract/webhook_test.go` -- outbound rows после `NotifyContractSent` / `Signed` / `Declined`.
- Unit-тесты (рядом с production-файлами):
  - `backend/internal/repository/telegram_message_test.go`
  - `backend/internal/telegram/recorder_test.go`
  - `backend/internal/telegram/recording_sender_test.go`
  - `backend/internal/handler/telegram_message_test.go`
  - `backend/internal/authz/telegram_message_test.go`

## Tasks & Acceptance

**Execution:**
- [x] `backend/api/openapi.yaml` -- добавить `GET /telegram-messages` (operationId `listTelegramMessages`) + schemas `TelegramMessageList` / `TelegramMessage` -- contract-first основа handler/types.
- [x] `backend/api/openapi-test.yaml` -- добавить `POST /test/seed-telegram-message` + `DELETE /test/telegram-messages` -- e2e read-API нуждается в seed/cleanup.
- [x] `make generate-api` -- регенерация Go server, openapi-fetch types, e2e-клиентов -- стандарт codegen.
- [x] `backend/migrations/<ts>_telegram_messages.sql` -- таблица + CHECK + индексы + partial UNIQUE; Down: DROP TABLE -- хранилище.
- [x] `backend/internal/domain/telegram_message.go` -- sentinel + константы direction/status -- `backend-constants.md`.
- [x] `backend/internal/repository/telegram_message.go` + правка `factory.go` -- Row, Repo, EAFP 23505, `ListByChat` с keyset cursor + limit+1 -- `backend-repository.md`.
- [x] `backend/internal/repository/telegram_message_test.go` -- pgxmock с точной SQL + аргументами; happy + 23505 + DB error для Insert; happy/empty/cursor для ListByChat -- coverage gate.
- [x] `backend/internal/telegram/recorder.go` -- `MessageRecorder` interface + `*MessageRecorderService`; sync INSERT; failure-mode по матрице -- intent §«Транзакции».
- [x] `backend/internal/telegram/recorder_test.go` -- RecordInbound (happy text/non-text/dedup/error без `text` в логе), RecordOutbound (sent/failed) -- mocks per t.Run, `t.Parallel`.
- [x] `backend/internal/telegram/recording_sender.go` + `recording_sender_test.go` -- decorator + mock-проверка делегирования + вызов recorder с правильными args.
- [x] `backend/internal/telegram/handler.go` -- добавить `recorder MessageRecorder` параметр в `NewHandler` + вызов `RecordInbound` после private-фильтра, до dispatcher'а -- inbound hook.
- [x] `backend/internal/telegram/handler_test.go` -- расширить: для каждого dispatcher-сценария (private text /start, fallback, edit-error reply) ассертить через mock что `RecordInbound` был вызван с правильным `update`. Для non-private / from=nil — `AssertNotCalled`.
- [x] `backend/internal/service/telegram_message.go` + unit-тест -- `ListByChat` без транзакции; cursor encode/decode; формирование nextCursor.
- [x] `backend/internal/authz/telegram_message.go` + `telegram_message_test.go` -- `CanReadTelegramMessages` admin-only; 3 кейса по precedent'у.
- [x] `backend/internal/handler/telegram_message.go` + `_test.go` -- handler по generated wrapper'у; авторизация → cursor decode → service → маппинг; 200 happy/cursor/empty + 401/403/422.
- [x] `backend/internal/handler/server.go` -- расширить `AuthzService` + добавить `TelegramMessageService` interface; обновить godoc.
- [x] `backend/internal/handler/testapi.go` -- `SeedTelegramMessage` (прямая запись через repo) + cleanup (новый метод или новый case `CleanupEntity.Type`); guarded `EnableTestEndpoints`.
- [x] `make generate-mocks` -- mockery для новых интерфейсов (RepoFactory, Recorder, Sender wrapping) -- стандарт.
- [x] `backend/cmd/api/telegram.go` + `main.go` -- собрать `*MessageRecorderService`; обернуть финальный `sender` в `RecordingSender`; передать recorder в `NewHandler`; передать `*TelegramMessageService` в `NewServer` -- wiring.
- [x] `backend/e2e/testutil/telegram_messages.go` -- `AssertTelegramMessageRecorded`, `SeedTelegramMessage`, `CleanupTelegramMessagesByChat` -- composable хелпер для расширения existing тестов и для нового e2e.
- [x] `backend/e2e/telegram_messages/list_test.go` -- new file; header на русском, нарратив; cursor pagination + empty + 401/403/422 -- e2e read-API.
- [x] Расширение existing e2e (`telegram/telegram_test.go`, `creator_applications/approve_test.go`, `reject_test.go`, `manual_verify_test.go`, `webhooks/sendpulse_instagram_test.go`, `campaign_creator/campaign_notify_test.go`, `contract/webhook_test.go`) -- добавить `AssertTelegramMessageRecorded` после каждой существующей spy-ассерции -- intent §«Тесты».
- [x] `make build-backend lint-backend test-unit-backend test-unit-backend-coverage test-e2e-backend` -- локальный gate.

**Acceptance Criteria:**
- Given private inbound message с текстом, when `Handler.Handle` отрабатывает, then в БД появляется ряд `direction='inbound', text=<original>, telegram_message_id=<update.Message.ID>` И dispatcher `/start` всё ещё срабатывает.
- Given повторный update с тем же `(chat_id, telegram_message_id)` (Telegram retry), when recorder вызывает `Insert`, then repo возвращает `ErrTelegramMessageAlreadyRecorded`, recorder логирует `Debug` (не `Error`), caller продолжает работу.
- Given outbound `Notifier.fire` retry-loop с цепочкой failed→failed→sent, when записи закончены, then в `telegram_messages` ровно 3 ряда (`status='failed'` x2, `status='sent'` x1, в порядке created_at).
- Given INSERT в `telegram_messages` упал по любой причине кроме 23505, when recorder обрабатывает ошибку, then `logger.Error` пишет `chat_id`+`direction`+`error` БЕЗ `text` в полях, И caller получает результат от inner.SendMessage / dispatcher продолжает.
- Given non-private chat (group/supergroup/channel) или `update.Message.From == nil`, when приходит update, then в `telegram_messages` НЕТ нового ряда И recorder.RecordInbound НЕ вызван.
- Given `GET /telegram-messages?chatId=X&limit=5` под admin при 12 рядах для X, when отрабатывает запрос, then 200 `{items.length=5, nextCursor!=null}` (DESC по `created_at, id`), И повторный запрос с этим cursor возвращает следующие 5, И последний — 2 ряда + `nextCursor=null`.
- Given `GET /telegram-messages` под brand_manager или без token, when отрабатывает, then 403 / 401 ErrorResponse.
- Given невалидные параметры (`limit=0`, `limit=101`, отсутствует `chatId`, мусорный `cursor`), when отрабатывает, then 422 ErrorResponse `CodeValidation`.
- Given existing e2e сценарий из списка под расширение, when тест проходит, then ассерция `AssertTelegramMessageRecorded` валидна (outbound row с правильным text + status; для bot_blocked ветки — `status='failed'` + `error` содержит «bot was blocked by the user»).
- Given `make build-backend lint-backend test-unit-backend test-unit-backend-coverage test-e2e-backend`, when запускаются по очереди, then все четыре стадии зелёные; coverage gate ≥ 80% на каждый новый identifier.

## Verification

**Commands:**
- `make build-backend` -- expected: успешная компиляция; никаких unused/missing.
- `make lint-backend` -- expected: 0 issues; gofmt чисто.
- `make generate-api` затем `git diff --exit-code` -- expected: codegen стабилен (никаких ручных правок `*.gen.go`).
- `make test-unit-backend` -- expected: все тесты зелёные с `-race`.
- `make test-unit-backend-coverage` -- expected: per-method ≥ 80% на новых файлах; gate не падает.
- `make test-e2e-backend` -- expected: новый `list_test.go` зелёный; все расширенные тесты (`telegram`, `creator_applications/*`, `webhooks/sendpulse_instagram`, `campaign_creator/campaign_notify`, `contract/webhook`) зелёные.

## Suggested Review Order

**Контракт + миграция (фундамент)**

- OpenAPI: новая admin-only ручка + opaque cursor pagination + schemas.
  [`openapi.yaml:356`](../../backend/api/openapi.yaml#L356)

- Test API: seed + cleanup для e2e-фикстур.
  [`openapi-test.yaml:278`](../../backend/api/openapi-test.yaml#L278)

- Миграция: таблица + CHECK на enum + partial UNIQUE inbound-dedup + INDEX под keyset cursor.
  [`20260513024740_telegram_messages.sql:1`](../../backend/migrations/20260513024740_telegram_messages.sql#L1)

**Запись (recorder + sender decorator + hook)**

- Sender-обёртка: делегирует, потом recorder.RecordOutbound. Sync — каждый Notifier.fire retry создаёт свой ряд.
  [`recording_sender.go:36`](../../backend/internal/telegram/recording_sender.go#L36)

- Recorder.RecordOutbound: status sent/failed, error.Error() в Error-колонку, PII-text не в логи.
  [`recorder.go:104`](../../backend/internal/telegram/recorder.go#L104)

- Recorder.RecordInbound: dedup-23505 → Debug, остальное Error без text.
  [`recorder.go:64`](../../backend/internal/telegram/recorder.go#L64)

- Inbound hook: между private-фильтром и dispatcher'ом — audit trail переживёт downstream panic.
  [`handler.go:82`](../../backend/internal/telegram/handler.go#L82)

**Хранилище**

- EAFP catch pgconn 23505 + constraint name → domain sentinel; нет preflight SELECT.
  [`telegram_message.go:71`](../../backend/internal/repository/telegram_message.go#L71)

- Keyset pagination: tuple comparison `(created_at, id) < (?,?)` соответствует индексу DESC.
  [`telegram_message.go:100`](../../backend/internal/repository/telegram_message.go#L100)

**Чтение (service + handler + authz)**

- Service: limit+1 паттерн → hasMore → nextCursor из последнего ряда.
  [`telegram_message.go:34`](../../backend/internal/service/telegram_message.go#L34)

- Handler: authz → decode cursor (422 на невалид) → service → маппинг row→DTO.
  [`telegram_message.go:17`](../../backend/internal/handler/telegram_message.go#L17)

- Authz: admin-only по precedent'у CanRejectCreatorApplication.
  [`telegram_message.go:13`](../../backend/internal/authz/telegram_message.go#L13)

- Domain: sentinel + direction/status константы + opaque base64-JSON cursor encode/decode.
  [`telegram_message.go:32`](../../backend/internal/domain/telegram_message.go#L32)

**Wiring**

- cmd/api: setupTelegram оборачивает финальный sender в RecordingSender ПЕРЕД Notifier'ом.
  [`telegram.go:44`](../../backend/cmd/api/telegram.go#L44)

- main: TelegramMessageService прокинут в NewServer; recorder — в NewHandler.
  [`main.go:115`](../../backend/cmd/api/main.go#L115)

**E2E**

- Composable helper `AssertTelegramMessageRecorded` с polling под admin-token.
  [`telegram_messages.go:34`](../../backend/e2e/testutil/telegram_messages.go#L34)

- Новый e2e на read-ручку: 5+5+2 пагинация, empty, 401/403/422.
  [`list_test.go:33`](../../backend/e2e/telegram_messages/list_test.go#L33)

- Расширенный e2e на inbound (запись fallback-сценариев).
  [`telegram_test.go:325`](../../backend/e2e/telegram/telegram_test.go#L325)

- Расширенный e2e на outbound bot_blocked: status=failed + error содержит «bot was blocked».
  [`campaign_notify_test.go:764`](../../backend/e2e/campaign_creator/campaign_notify_test.go#L764)

**Тесты (supporting)**

- Repo unit: pgxmock exact SQL + args; 23505 + dedup constraint name.
  [`telegram_message_test.go:1`](../../backend/internal/repository/telegram_message_test.go#L1)

- Recorder unit: per-t.Run mock, PII guard, нет text в Error-полях.
  [`recorder_test.go:1`](../../backend/internal/telegram/recorder_test.go#L1)

- Handler unit: 200 happy/cursor/empty + 401/403/422 + cursor round-trip.
  [`telegram_message_test.go:1`](../../backend/internal/handler/telegram_message_test.go#L1)

- Telegram Handler unit: расширен — silentRecorder ассертит «не вызван» для non-private/from=nil.
  [`handler_test.go:46`](../../backend/internal/telegram/handler_test.go#L46)
