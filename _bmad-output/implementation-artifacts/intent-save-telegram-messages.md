# Intent: сохранение всех Telegram-сообщений (in + out) с привязкой по chat_id

## Преамбула: стандарты

Перед реализацией агент обязан полностью прочитать `docs/standards/` —
все файлы целиком. Этот интент описывает только дельту над существующим
кодом; все общие правила (слои, codegen, EAFP, RepoFactory, naming,
security, тесты) живут в стандартах и применяются без исключений.

Ключевые стандарты для этой фичи:
`backend-architecture.md`, `backend-design.md`, `backend-repository.md`,
`backend-transactions.md`, `backend-errors.md`, `backend-constants.md`,
`backend-libraries.md`, `backend-testing-unit.md`,
`backend-testing-e2e.md`, `naming.md`, `security.md`.

## Тезис

Сохраняем все Telegram-сообщения между ботом и пользователем — и
входящие (любого формата от пользователя), и исходящие (всё, что шлёт
`Notifier`/`Send*Campaign*`) — в новой таблице `telegram_messages` с
привязкой только по `chat_id`, без FK на доменные сущности. Цель —
полная хронологическая лента переписки по `chat_id` на стороне БД для
дебага и поддержки. На первом шаге сохраняем только текст; схема и
hook-точки сразу заложены под расширение (raw_payload, media, captions).

## Сущность `telegram_messages`

Новая таблица. Поля:

| Колонка | Тип | Notes |
|---|---|---|
| `id` | UUID PK | стандарт; default `gen_random_uuid()` |
| `chat_id` | BIGINT NOT NULL | Telegram chat ID (точка привязки) |
| `direction` | TEXT NOT NULL | CHECK in (`'inbound'`,`'outbound'`) |
| `text` | TEXT NOT NULL DEFAULT `''` | пустая строка для не-текстовых inbound |
| `telegram_message_id` | BIGINT NULL | message_id Telegram'а (inbound — `update.Message.ID`; outbound — `response.ID`; NULL при outbound-failure до получения id) |
| `telegram_username` | TEXT NULL | только для inbound; outbound — NULL; username может меняться, фиксируем на момент сообщения |
| `status` | TEXT NULL | только outbound: `'sent'` или `'failed'`; CHECK; inbound — NULL |
| `error` | TEXT NULL | только при `status='failed'` — голый `err.Error()` |
| `created_at` | TIMESTAMPTZ NOT NULL DEFAULT `now()` | момент записи |

Индексы:
- PRIMARY KEY (`id`).
- INDEX (`chat_id`, `created_at` DESC) — для выборки ленты переписки по chat.

Constraints (рассмотреть на стадии реализации миграции; не блокирующее
решение здесь): partial-CHECK на корректность `status`/`error` per
direction (inbound: status=NULL, error=NULL; outbound: status NOT NULL,
error NOT NULL only when status='failed'). Альтернатива — оставить
инвариант на коде, чтобы не разводить SQL-логику.

PII: таблица — специализированное хранилище (по аналогии с `audit_logs`),
PII в ней допустима. В stdout-логах продолжаем писать только `chat_id`
и event-метки.

## Расширяемость

Закладываем на двух уровнях, чтобы дописать «полный payload» одной
forward-миграцией без переделки кода:

1. **БД.** Новые поля (`raw_payload jsonb`, `media_type`, `file_id`,
   `caption`, `reply_to_message_id`, …) добавляются отдельной
   forward-миграцией, все nullable. Backfill не требуется — старые
   записи остаются с NULL.
2. **Код.** Hook-точки перехватывают **весь поток**, не только
   текстовые:
   - Inbound — пишем ряд на каждый `update.Message`, который проходит
     через `Handler.Handle` (включая случаи, когда `update.Message.Text`
     пустой: фото/voice/стикер/forward — `text=''`).
   - Outbound — пишем ряд на каждый `SendMessage` (welcome / approve /
     reject / campaign invite / contract events / reminders), независимо
     от ParseMode и наличия ReplyMarkup.

   Когда добавим payload-поля, новые колонки потекут автоматически из
   тех же мест без правки handler/notifier flow.

## Точки перехвата

### Inbound

Единственная точка входа — `Handler.Handle`
(`backend/internal/telegram/handler.go:58`). Хук идёт **после** nil-check
и фильтра non-private chats (`update.Message.Chat.Type != chatTypePrivate`),
**до** dispatcher-логики `/start`-команды. То есть:

- non-private (group/supergroup/channel) — **не пишем** (любой админ
  чужой группы может добавить бота, и БД распухнет от спама).
- update.Message == nil или from == nil — **не пишем** (ни chat_id,
  ни attribution).
- private + from != nil — пишем строку (включая случаи, когда
  `update.Message.Text` пустой: фото/voice/стикер/forward/reply →
  `text=''`), затем продолжаем dispatch (`/start`-логика и fallback).

`Handler` получает новую зависимость `recorder MessageRecorder` через
конструктор. Запись синхронная (детали failure-handling — слой
«Транзакции / идемпотентность»).

### Outbound

Новая обёртка `RecordingSender` (decorator над `Sender`-интерфейсом, по
образцу `TeeSender` — `backend/internal/telegram/tee_sender.go`):

```go
type RecordingSender struct {
    inner    Sender
    recorder MessageRecorder
}

func (s *RecordingSender) SendMessage(ctx context.Context, params *bot.SendMessageParams) (*models.Message, error) {
    msg, err := s.inner.SendMessage(ctx, params)
    s.recorder.RecordOutbound(ctx, params, msg, err)
    return msg, err
}
```

Подключается в `main.go` **поверх** всего стека (`real` / `TeeSender` /
`SpyOnlySender`) — одна точка для всех трёх production-путей. Запись
происходит для каждого вызова `SendMessage` независимо от ParseMode,
ReplyMarkup, success/failure. Ошибка записи в БД не должна откатывать
доставку — fire-and-forget из перспективы caller'а (детали — слой
«Транзакции / идемпотентность»).

### Архитектура recorder'а

- **`internal/repository/telegram_message.go`** — `TelegramMessageRow`
  (с `db`/`insert` тегами по стандарту `backend-repository.md`),
  `TelegramMessageRepo` interface, приватный `telegramMessageRepository`,
  factory-метод `NewTelegramMessageRepo(db dbutil.DB) TelegramMessageRepo`
  на общем `RepoFactory`. Константы колонок
  `TelegramMessageColumn*`, `TableTelegramMessages`.
- **`internal/telegram/recorder.go`** — `MessageRecorder` interface:

  ```go
  type MessageRecorder interface {
      RecordInbound(ctx context.Context, update *models.Update)
      RecordOutbound(ctx context.Context, params *bot.SendMessageParams, msg *models.Message, err error)
  }
  ```

  Реализация — `*MessageRecorderService` с зависимостями
  `pool dbutil.Pool` + узкий `RecorderRepoFactory` interface
  (только `NewTelegramMessageRepo(db)`). Без транзакции (single
  INSERT, ничего не координируем с другими записями).

- **Wiring в `main.go`**: создаётся один `*MessageRecorderService`,
  передаётся и в `Handler` (через `NewHandler`), и в `RecordingSender`
  (новая обёртка вокруг финального sender'а).

## Ручка чтения

Одна business-API ручка для e2e + будущей админ-страницы переписки.

### API surface

OpenAPI (`backend/api/openapi.yaml`):

```yaml
GET /telegram-messages
operationId: listTelegramMessages
tags: [telegram-messages]
security: bearerAuth: []
parameters:
  - name: chatId
    in: query
    required: true
    schema:
      type: integer
      format: int64
  - name: limit
    in: query
    required: true
    schema:
      type: integer
      minimum: 1
      maximum: 100
  - name: cursor
    in: query
    required: false
    schema:
      type: string
      description: opaque base64-encoded cursor; null/missing = first page
responses:
  200: TelegramMessageList
  400/422: стандартный ErrorResponse (валидация)
  401/403: стандартные
```

Schemas:

```yaml
TelegramMessageList:
  type: object
  required: [items]
  properties:
    items:
      type: array
      items: { $ref: '#/components/schemas/TelegramMessage' }
    nextCursor:
      type: string
      nullable: true
      description: opaque cursor for next page; null = end of stream

TelegramMessage:
  type: object
  required: [id, chatId, direction, text, createdAt]
  properties:
    id: { type: string, format: uuid }
    chatId: { type: integer, format: int64 }
    direction:
      type: string
      enum: [inbound, outbound]
    text: { type: string }
    telegramMessageId:
      type: integer
      format: int64
      nullable: true
    telegramUsername:
      type: string
      nullable: true
    status:
      type: string
      enum: [sent, failed]
      nullable: true
    error:
      type: string
      nullable: true
    createdAt:
      type: string
      format: date-time
```

После правки `openapi.yaml` — `make generate-api`.

### Authz

`backend/internal/authz/telegram_message.go` (новый файл):
`CanReadTelegramMessages(ctx) error` — admin-only (по precedent'у
других admin-only ручек).

### Service / Repo

- `*TelegramMessageService.ListByChat(ctx, chatID int64, cursor *Cursor, limit int) ([]*TelegramMessageRow, *Cursor, error)` —
  read-only, без транзакции (single SELECT через `s.repoFactory.NewTelegramMessageRepo(s.pool)`).
- Repo: `ListByChat(ctx, chatID, cursor, limit) ([]*Row, error)` —
  `squirrel.Select(cols...).From(TableTelegramMessages).Where(Eq{chat_id: chatID}).OrderBy(created_at DESC, id DESC).Limit(uint64(limit + 1))`. Сервис делает hasMore = `len(rows) > limit`, обрезает до limit и формирует nextCursor из последнего ряда.
- Cursor: opaque base64-encoded JSON `{"createdAt": "...", "id": "..."}`.
  При decode — keyset WHERE (`(created_at, id) < (cursorCreatedAt, cursorID)`),
  что эквивалентно `created_at < c OR (created_at = c AND id < cID)`.

### Handler

`backend/internal/handler/telegram_message.go` (новый файл) —
`*Server.ListTelegramMessages(ctx, request) (response, error)`:
1. `s.authzService.CanReadTelegramMessages(ctx)` → 403 при ошибке.
2. Decode cursor (если задан) → 422 при невалидном.
3. `s.telegramMessageService.ListByChat(...)` → маппинг в generated DTO.
4. Response 200.

`server.go` — расширить интерфейс `AuthzService`
(`CanReadTelegramMessages`) и добавить `TelegramMessageService` interface.

### Связь с UI следующего чанка

В следующем чанке (админский интерфейс просмотра переписки) — обогатить
детали креатора/заявки полем `telegramChatId` (= `telegram_user_id` из
`creator_application_telegram_links`; для private-chat они идентичны).
Фронт берёт его и передаёт в `GET /telegram-messages?chatId=…`. Сейчас
этого обогащения **не делаем** — только бэк-ручка.

## Транзакции / идемпотентность / failure-modes

### Sync, без транзакции с бизнес-логикой

`MessageRecorderService.RecordInbound` и `RecordOutbound` выполняются
**синхронно** в вызывающей горутине (Handle / Notifier.fire callback /
SendCampaignInvite caller) — single INSERT через
`s.repoFactory.NewTelegramMessageRepo(s.pool)`, **без** транзакции с
бизнес-логикой и **без** оборачивания в `dbutil.WithTx`. Запись
теряется в crash-window между доставкой Telegram-сообщения и INSERT'ом
— это допустимая потеря по сравнению со сложностью двухфазной записи.

### Failure-mode

Любая ошибка INSERT (включая connection issues, конкурентные блокировки
и т.п.) — `logger.Error(ctx, "telegram message record failed", "chat_id",
chatID, "direction", direction, "error", err)`. **Без** `text` в логе
(может содержать PII — ИИН/ФИО/handle, см. `security.md`). Recorder
**не возвращает ошибку наружу** — caller (Handle / RecordingSender)
продолжает работу: dispatcher /start срабатывает, доставка не
откатывается. Recording — best-effort observability, не критичный
write-path.

### Идемпотентность inbound

`UNIQUE INDEX telegram_messages_inbound_dedup_idx ON telegram_messages
(chat_id, telegram_message_id) WHERE direction = 'inbound' AND
telegram_message_id IS NOT NULL`.

Repo делает INSERT, на pgconn-ошибке с `Code='23505'` (через
`errors.As` на `*pgconn.PgError`) возвращает доменный sentinel
`ErrTelegramMessageAlreadyRecorded`. Recorder ловит этот sentinel,
пишет `logger.Debug(ctx, "telegram inbound dedup hit", "chat_id",
"telegram_message_id")` и возвращает — без логирования как ошибки.
Соответствует стандарту `backend-repository.md` § Ошибки.

Outbound дедупа нет — каждый вызов `sender.SendMessage` пишет новый
ряд, что для retry-loop'ов (`Notifier.fire` с экспоненциальным
backoff) даёт честную картину «N попыток, последняя — sent/failed».

### Cursor & race-test

Partial UNIQUE покрыт unit-тестом repo (mock `pgxmock` отдаёт
`&pgconn.PgError{Code: "23505"}` → метод возвращает
`ErrTelegramMessageAlreadyRecorded`). **Race-test для concurrent
INSERT — осознанный пропуск** (стандартное `[major]`-finding из
`backend-testing-e2e.md` § Время и race-сценарии): бот long-poll'ит
одним инстансом, и реальный concurrent insert одного `update_id`
возможен только на крайне узком окне fail-over getUpdates. Защита
оставлена «defense in depth»; если в будущем включим горизонтальное
масштабирование poller'а — допишем race-test отдельным PR'ом.

## Тесты

Качество тестов — обязательно, не «галочка». Все правила из
`backend-testing-unit.md`, `backend-testing-e2e.md` применяются
буквально (`t.Parallel`, новые моки на каждый `t.Run`, captured-input
для middleware-derived полей, JSONEq для JSON-полей и т.д.).

### Backend unit — новый код

- **TelegramMessageRepo** (`backend/internal/repository/telegram_message_test.go`):
  - `Insert` — happy (mock `pgxmock` ожидает SQL с точным набором
    колонок и аргументов), 23505 → `ErrTelegramMessageAlreadyRecorded`
    (через `&pgconn.PgError{Code: "23505"}`), generic DB error →
    обёрнутая ошибка с контекстом.
  - `ListByChat` — happy с курсором (limit+1 паттерн), happy без
    курсора (первая страница), empty result, DB error.
- **MessageRecorderService** (`backend/internal/telegram/recorder_test.go`):
  - `RecordInbound` — happy (private-message c text), happy с пустым
    text (фото/voice — `text=''`), dedup-hit (repo возвращает
    `ErrTelegramMessageAlreadyRecorded` → Debug + return, не Error),
    repo error (Error без `text` в логе — captured-log проверяет).
  - `RecordOutbound` — sent (msg!=nil, err==nil → `status='sent'`,
    `telegram_message_id` из `msg.ID`), failed (msg==nil, err!=nil
    → `status='failed'`, `error=err.Error()`, `telegram_message_id=NULL`).
- **Handler `ListTelegramMessages`** (`backend/internal/handler/telegram_message_test.go`):
  - 200 happy (без курсора + с курсором), 200 empty, 401, 403,
    422 (limit=0, limit>100, отсутствует chatId, невалидный курсор).
- **Authz** (`backend/internal/authz/telegram_message_test.go`):
  - admin → no error; brand_manager → ErrForbidden; anon → ErrForbidden.
- **RecordingSender** (`backend/internal/telegram/recording_sender_test.go`):
  - Делегирует в inner.SendMessage; вызывает recorder.RecordOutbound
    с params/msg/err из inner; возвращает (msg, err) caller'у
    неизменными. Проверка через mock'и обоих интерфейсов.

`make test-unit-backend-coverage` — каждый новый identifier ≥ 80%.

### Backend e2e — расширение существующих тестов

Вместо отдельных e2e на recording — **расширяем все существующие e2e,
где идёт коммуникация с креатором через бот**, проверкой записанных
рядов в `telegram_messages`. Новый testutil-хелпер
`testutil.AssertTelegramMessageRecorded(t, chatID, direction, textMatcher)`
(или похожий) — вызывает `GET /telegram-messages?chatId=...&limit=100`
под admin-token и ассертит наличие/содержимое нужной строки.

Список тестов под расширение (по `git ls-files backend/e2e`):
- `backend/e2e/telegram/telegram_test.go` — inbound `/start` happy +
  fallback (любой текст не-`/start` → inbound row с `text=`<этот текст>).
- `backend/e2e/creator_applications/approve_test.go` —
  `NotifyApplicationApproved` → outbound row с `status='sent'` и
  text-зеркалом `applicationApprovedText`.
- `backend/e2e/creator_applications/reject_test.go` —
  `NotifyApplicationRejected` → outbound row с `status='sent'`.
- `backend/e2e/creator_applications/manual_verify_test.go` /
  `webhooks/sendpulse_instagram_test.go` —
  `NotifyVerificationApproved` → outbound row.
- `backend/e2e/campaign_creator/campaign_notify_test.go` —
  invite/remind → outbound rows для каждого приглашения, partial
  failure (`bot_blocked` через `RegisterTelegramSpyFailNext`) → outbound
  row с `status='failed'` и `error` содержит `bot was blocked by the
  user`.
- `backend/e2e/contract/webhook_test.go` —
  `NotifyCampaignContractSent` / `Signed` / `Declined` → outbound rows.

Если existing-test уже шлёт несколько сообщений — `AssertTelegramMessageRecorded`
вызывается за каждое (порядок чтения в ручке — DESC по created_at).

### Backend e2e — новый файл для READ-API

`backend/e2e/telegram_messages/list_test.go` — единственный новый
e2e-файл. Header — на русском, нарратив. Сценарии:
- 200 happy с курсором: сидим N>limit рядов через
  /test/seed-telegram-message (новая testapi-ручка для прямой
  записи) → первая страница с `nextCursor`, вторая страница с
  оставшимися, третья — empty.
- 200 happy без курсора (первая страница).
- 200 empty (chatId без переписки).
- 422 (limit=0, limit>100, отсутствует chatId, невалидный cursor
  base64).
- 401 (без token), 403 (brand_manager).

Cleanup — через бизнес-API нет (записи не удаляются нашими ручками)
→ testapi `/test/cleanup-telegram-messages?chatId=...` (по образцу
других testapi-cleanup'ов в `backend/internal/testapi/`).

`E2E_CLEANUP=true` по умолчанию.

### Тестовое API (testapi)

Две новых ручки в `backend/api/openapi-test.yaml`:
- `POST /test/seed-telegram-message` — прямая запись ряда (нужна
  для read-API e2e: сидируем разнообразные ряды без прогона через
  бот). Принимает все колонки, возвращает `id`.
- `DELETE /test/telegram-messages?chatId={int64}` — hard-delete всех
  рядов для chatId (cleanup для list-test).

Доступны только при `EnableTestEndpoints=true` (стандартная защита
testapi).

## Cohesion check

- [x] Миграция БД — новая `XXXXX_telegram_messages.sql`. `pgcrypto`
      extension уже подключен в `00001_init.sql` →
      `gen_random_uuid()` доступен без правок. CHECK на
      direction enum (`'inbound'`/`'outbound'`) — да; CHECK на
      status enum — да; partial CHECK на per-direction invariant
      (status/error) — **нет** (invariant в коде, recorder —
      единственный writer таблицы; testapi-seed-ручка под
      `EnableTestEndpoints` валидирует invariant на handler-уровне).
- [x] Партиальный UNIQUE-индекс
      `telegram_messages_inbound_dedup_idx` —
      `WHERE direction='inbound' AND telegram_message_id IS NOT NULL`.
- [x] Down-миграция: `DROP TABLE telegram_messages;` — без
      backfill-проблем (таблица новая).
- [x] Audit_logs — **не пишем** (запись в `telegram_messages` сама
      по себе не бизнес-mutate; вызывающие mutate-операции
      —`creator_application.approve`, `campaign_creator.invite`
      и др. — пишут свой audit как раньше).
- [x] PII в `telegram_messages` — допустима (специализированное
      хранилище, по аналогии с `audit_logs`, см.
      `project_audit_vs_logs` в memory). PII в stdout-логах —
      только `chat_id`, `direction`, event-метки; `text` в логах
      **запрещён**.
- [x] Anti-fingerprinting / rate-limiting — admin-only ручка чтения,
      не нужны.
- [x] OpenAPI обновлён (новый endpoint + 2 testapi-endpoint'а), типы
      реgenerируются через `make generate-api`.
- [x] Wiring в `main.go` — собрать `*MessageRecorderService` один
      раз, передать в `Handler` (через `NewHandler`) и в
      `RecordingSender` (новая обёртка над финальным sender'ом).
- [x] Race-test для partial UNIQUE — **осознанный пропуск**
      (см. секцию «Транзакции / идемпотентность»).
- [x] Frontend — вне скоупа этого чанка (UI добавится в следующем
      чанке после обогащения creator/application details полем
      `telegramChatId`).

## Открытые вопросы

Нет.

## Следующий шаг

Hand-off на `bmad-quick-dev` с этим интентом в качестве входа.
Спека (`spec-save-telegram-messages.md`) генерируется уже там.
