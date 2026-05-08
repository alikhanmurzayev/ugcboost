---
title: "Intent: chunk 12 — рассылка приглашений и ремайндеров через бот (бэк)"
type: intent
status: draft
created: "2026-05-08"
chunk: 12 (campaign-roadmap.md / Группа 5)
design_source: _bmad-output/planning-artifacts/design-campaign-creator-flow.md
---

# Intent: chunk 12 — рассылка приглашений и ремайндеров (бэк)

## Преамбула

Перед началом реализации агент обязан полностью загрузить все файлы из `docs/standards/`
(без сокращений). Этот intent — implementation-level спецификация поверх дизайн-документа
`design-campaign-creator-flow.md`; противоречий с дизайн-документом и стандартами
`docs/standards/` быть не должно. Источник правды по state-машине, audit-event'ам и edge
cases — design-документ; intent добавляет только impl-уровневые решения.

## Scope

Chunk 12 из `campaign-roadmap.md` (Группа 5 в design'е). Только бэкенд. Out of scope:

- TMA-ручки T1/T2 agree/decline — chunk 14.
- TrustMe-механика и ремайндер по подписанию — Группа 7 (chunks 16–18), отдельный design.
- Фронт-страница рассылок — chunk 13.
- Любые изменения чанка 10 (миграция `campaign_creators`, A1/A2/A3) — этот intent не трогает уже зарелиженный chunk 10.

## Тезис

Две admin-ручки на бэке + расширение существующей `CampaignService.UpdateCampaign`:

- `POST /campaigns/{id}/notify` — первое/повторное приглашение. Переход
  «Запланирован» / «Отказался» → «Приглашён».
- `POST /campaigns/{id}/remind-invitation` — ремайндер для статуса «Приглашён».
  Статус не меняется, инкрементируется `reminded_count`/`reminded_at`.
- Расширение PATCH `/campaigns/{id}`: смена `tma_url` отклоняется (422), если у любого
  `campaign_creators.invited_count > 0` в этой кампании.

Принципы поведения (детали — § Edge cases в design'е):

- **Strict-422 на validation:** любой `creator_id` в батче невалиден / в несовместимом
  статусе → весь batch отклоняется до доставки. Никаких частичных вставок.
- **Partial-success на runtime delivery:** успешная validation → пытаемся доставить всем
  через `internal/telegram/notifier`. Доставленным — счётчики/timestamps инкрементируются,
  audit пишется. Не доставленным — состояние не меняется, response 200 с
  `undelivered: [{creator_id, reason}]`.
- **Re-invite после отказа:** на A4 из «Отказался» → `invited_count++`,
  `reminded_count = 0`, `reminded_at = NULL`, `decided_at = NULL`. Цикл «приглашение →
  решение» начинается заново.
- **Universal-сообщение бота:** без деталей кампании, тело = ссылка = `tma_url`. Конкретные
  формулировки текстов — на этапе реализации (легко меняется).

## Транзакционная гранулярность

Per-creator мелкий tx. Поток A4/A5:

1. **Validation pre-pass (без tx).** Read-only SELECT всех релевантных
   `campaign_creators` строк по `(campaign_id, creator_id IN ($batch))`. Проверка:
   - кампания существует и не soft-deleted (иначе 404);
   - все `creator_ids` присутствуют в этой кампании;
   - статус каждого совместим с действием (A4: «Запланирован»/«Отказался»;
     A5: «Приглашён»);
   - `creators.telegram_user_id` есть (инвариант — `NOT NULL` в схеме, но мы доверяем
     схеме, без явной проверки).
   Любое нарушение → 422, весь batch отклонён до доставки.
2. **Delivery loop (per-creator).** Для каждого `creator_id` в батче:
   - Telegram send через `internal/telegram/notifier`.
   - Успех → отдельный `dbutil.WithTx`: UPDATE counters/timestamps + INSERT audit одной
     транзакцией. Tx-hold ≈ один UPDATE + один INSERT, без сетевых I/O.
   - Ошибка отправки → не меняем БД, копим creator в `undelivered: [{creator_id, reason}]`.
3. **Response.** 200 `{ undelivered: [...] }`.

Race-window между validation и delivery (admin-remove / TMA agree-decline другим путём)
acceptable: статус мог поменяться, но post-факт UPDATE проинкрементирует counters
для уже несовместимого статуса. Считаем это noise-уровнем; явный re-check внутри
per-creator tx не делаем (усложнение без реальной защиты — Telegram уже отправлен).

## API: response schemas (кастомные для A4/A5)

Отдельные per-endpoint OpenAPI-схемы, не расширяем generic `APIError`. A4 и A5 разделяют
один и тот же набор схем (поведение симметричное).

### 422 — batch-validation failure

```json
{
  "error": {
    "code": "campaign_creator_batch_invalid",
    "message": "Некоторые креаторы не могут быть приглашены: обновите список и повторите.",
    "details": [
      {"creator_id": "...", "reason": "wrong_status", "current_status": "agreed"},
      {"creator_id": "...", "reason": "not_in_campaign"}
    ]
  }
}
```

`details[].reason` enum:
- `not_in_campaign` — creator_id отсутствует в `campaign_creators` для этой кампании.
  Включает кейс «creator_id не существует в `creators`»: для админа это семантически
  одно и то же — «нет в этой кампании, обнови список».
- `wrong_status` — статус несовместим с действием. Дополнительно прикладываем
  `current_status` для UX-сообщения («креатор уже согласился» / «приглашение не отправлено»).

Validate-pass собирает все нарушения за один проход (не выходит на first-fail) — фронт
видит полный список и подсвечивает все проблемные строки.

Format-ошибки (невалидный UUID, пустой массив, `len > 200`) — обычный 422 от
ServerInterfaceWrapper с `CodeValidation` (стандартный путь, не наш кастомный schema).

### 200 — partial-success delivery

```json
{
  "undelivered": [
    {"creator_id": "...", "reason": "bot_blocked"},
    {"creator_id": "...", "reason": "unknown"}
  ]
}
```

`undelivered[].reason` enum:
- `bot_blocked` — Telegram вернул 403 Forbidden («bot was blocked by the user» /
  «user is deactivated»). Креатор недоступен через бот, нужен альт-канал.
- `unknown` — всё остальное (network / timeout / 5xx от TG / internal). Сырая ошибка
  логируется в stdout с context (`campaign_id`, `creator_id`, error string) — без PII.

`delivered_count` / `delivered` list **не возвращаем**: длина батча есть на фронте,
доставлено = `len(creator_ids) - len(undelivered)`.

## API: request shape

Симметрично с `POST /campaigns/{id}/creators` (chunk 10). Один и тот же тип для обеих
ручек (A4 notify и A5 remind-invitation) — поведение одинаковое, отличается только
обработчик в сервисе.

```json
{ "creatorIds": ["uuid", "uuid", ...] }
```

- `creatorIds` — required, `minItems: 1`, `maxItems: 200`, `format: uuid`.
- Дубликаты в массиве → 422 `CodeValidation` (handler-уровень валидации до семантики).

## PATCH /campaigns/{id} — lock на tma_url

Расширение существующего `CampaignService.UpdateCampaign`. Проверка: при PATCH с
`tma_url`, отличающимся от текущего значения, `SELECT EXISTS(SELECT 1 FROM
campaign_creators WHERE campaign_id = $1 AND invited_count > 0)`. Если есть — атомарный
reject всего PATCH (422), даже если в том же запросе менялся только `name`. Если
`tma_url` совпадает с текущим (no-op) — лок не срабатывает, PATCH проходит.

- Domain error: `ErrCampaignTmaURLLocked` (`NewValidationError`), code
  `CampaignTmaURLLocked`, message: «Нельзя изменить ссылку на ТЗ — приглашения по
  текущей ссылке уже отправлены креаторам.»
- Реализация в сервисе: SELECT текущей кампании → если `tma_url` отличается, SELECT-EXISTS
  на `campaign_creators` → если есть, return ErrCampaignTmaURLLocked → handler возвращает
  422 через стандартный `respondError`. Транзакция остаётся прежней (внутри `WithTx`,
  как сейчас в UpdateCampaign).
- На стороне `campaign_creator` repo нужен новый метод вроде
  `ExistsInvitedInCampaign(ctx, campaignID) (bool, error)`.

## Audit события

Добавляем две константы в `backend/internal/service/audit_constants.go`:

- `AuditActionCampaignCreatorInvite = "campaign_creator_invite"` (для A4 успешной доставки).
- `AuditActionCampaignCreatorRemind = "campaign_creator_remind"` (для A5).

Payload — следуем convention из chunk 10 (`writeAudit(... oldCC, newCC)`): запись пары
«до / после» полного `domain.CampaignCreator`. `invited_count` / `reminded_count` уже
лежат в post-snapshot — отдельное поле `attempt_no` не нужно (consumer audit-логов
читает счётчик из newCC). Это упрощение относительно «attempt_no в payload» из
design'а — функционально эквивалентно, но соблюдает существующий формат.

Audit пишется в той же per-creator tx, что и UPDATE (`backend-transactions.md`
§ Аудит-лог). Не доставленным — аudit не пишется (соответствует partial-success).

## Сервисная гранулярность

Расширяем существующий `CampaignCreatorService`: добавляем методы `Notify(ctx, campaignID,
creatorIDs []string) (undelivered []NotifyFailure, err error)` и
`RemindInvitation(ctx, campaignID, creatorIDs []string) (undelivered ..., err error)`.

Зависимости сервиса дополняются:
- `notifier internal/telegram.Notifier` — для отправки сообщений (через DI в конструкторе).
- `repoFactory` уже есть, добавляется `NewCreatorRepo` в интерфейс
  `CampaignCreatorRepoFactory` (если ещё нет) — нужно читать `creators.telegram_user_id`.

Отдельный `CampaignNotificationService` не делаем: ответственность та же (lifecycle
campaign_creators), частично пересекается с add/remove (soft-delete check, status guard).
Дублирование boilerplate выйдет дороже расширения.

## Сообщение в боте: текст + inline web_app button

Каждое сообщение бота — текст + одна inline-кнопка типа `web_app` с `url = tma_url`
кампании. Это обязательно: TMA-ручки в chunk 14 идентифицируют креатора через
Telegram initData (HMAC), а initData отдаётся клиенту **только** когда TMA открыта через
inline-кнопку `web_app`. Если бы мы слали plain-текст со ссылкой (`url`-кнопка или
просто URL в теле), креатор бы попадал в обычный браузер без initData — бэк не смог бы
авторизовать agree/decline.

Текст универсальный, без деталей кампании. Финальные формулировки — на этапе
реализации (legко меняется через i18n / константы). Базовая идея:

- A4 invite: «Привет! У нас есть для тебя предложение по сотрудничеству. Открой,
  чтобы посмотреть условия:» + кнопка «Посмотреть» (web_app → tma_url).
- A5 remind-invitation: «Напоминаем — мы ждём твоего решения по приглашению.»
  + кнопка «Посмотреть» (web_app → tma_url).

`internal/telegram/notifier` нужно расширить методом `SendInviteWithWebApp(ctx,
chatID int64, text string, webAppURL string) error` (или аналогичным) — если такого
ещё нет в существующем интерфейсе. Spy_store в тестах должен фиксировать `web_app.url`
и текст сообщения для assert'ов в spy/e2e тестах.

## Тесты

### Unit (`backend-testing-unit.md`, `t.Parallel()`, race detector, mockery)

Помимо общего gate ≥80% per-method (`make test-unit-backend-coverage`):

**Service** (`*CampaignCreatorService`):
- happy A4: все доставлены → status=Приглашён, invited_count++, invited_at, audit per creator;
- happy A5 на «Приглашён»: reminded_count++, reminded_at, audit;
- batch-validation 422 — собираем **все** `details`, не first-fail (wrong_status и
  not_in_campaign в одном батче);
- partial-success A4/A5: часть доставленных, часть `bot_blocked`, часть `unknown` —
  для не-доставленных counters/audit не пишутся;
- re-invite из «Отказался»: invited_count++, reminded_count=0, reminded_at=NULL,
  decided_at=NULL;
- soft-deleted campaign → 404 (через `assertCampaignActive`);
- PATCH tma_url lock: срабатывает когда tma_url меняется и есть invited_count>0; не
  срабатывает на no-op (tma_url не меняется); не срабатывает когда invited_count=0
  во всех строках кампании.

**Repository** (`*campaignCreatorRepository.ExistsInvitedInCampaign`): pgxmock,
точный SQL (литералы колонок), параметры, true/false ветки.

**Telegram error → reason mapping**: отдельный helper `mapTelegramErrorToReason` —
table-driven тест: 403 Forbidden / "bot was blocked by the user" / "user is
deactivated" → `bot_blocked`; network / timeout / 5xx / прочее → `unknown`.

**Notifier-вызовы**: mock-notifier проверяет аргументы — `chat_id =
creator.telegram_user_id`, `web_app.url = campaign.tma_url`, текст сообщения,
ровно один вызов на каждого creator_id в батче.

### E2E (`backend-testing-e2e.md`, `t.Parallel()`, `internal/telegram/spy_store`)

Все assert'ы — строгие, конкретные значения (не «не пустое»). Audit-row проверяется
через `testutil.AssertAuditEntry` для каждой mutate-ручки. Бот-сообщения — через
spy_store: проверяем число вызовов, chat_id, web_app.url.

Сценарии:

1. **Happy A4**: campaign (с tma_url) → seed creators (через approve-flow) → A1 add →
   A4 notify → 200 без undelivered; статусы=Приглашён; invited_count=1, invited_at;
   audit `campaign_creator_invite` per creator; spy_store: N сообщений с правильными
   chat_id и web_app.url.
2. **Happy A5**: после happy A4 → A5 remind → 200; reminded_count=1, reminded_at;
   audit `campaign_creator_remind`.
3. **Strict-422 wrong_status**: A4 → A4 повторно на тот же creator → 422 с
   `details: [{creator_id, reason: "wrong_status", current_status: "invited"}]`;
   БД и spy_store без изменений.
4. **Strict-422 not_in_campaign**: A4 с creator_id, не привязанным к кампании
   → 422 с `details: [{..., reason: "not_in_campaign"}]`.
5. **Partial-success A4**: spy-notifier настроен на fail для одного `creator_id`
   (через `spy_store` API) → 200 с `undelivered: [{creator_id, reason: "bot_blocked"}]`;
   у failed-креатора: status, invited_count, invited_at без изменений, audit не
   записан; у delivered — всё инкрементировано как в #1.
6. **Soft-delete kampanii → 404 на A4/A5**: создаём кампанию, soft-delete (через A2 от
   chunk 7 если он в main к моменту, или через testapi), A4 на неё → 404.
7. **PATCH tma_url lock после A4 → 422**: campaign + A1 + A4 → PATCH с новым tma_url
   → 422 `CampaignTmaURLLocked`; в БД tma_url не поменялся; audit `campaign_update` не
   записан.
8. **PATCH name only после A4 → 200**: тот же setup, PATCH только name → 200, name
   обновлён, tma_url не тронут, audit `campaign_update` записан.

«Re-invite из Отказался» (через цикл A4 → T2 decline → A4) — **в chunk 12 e2e не
покрываем**, так как T2 — chunk 14. Сценарий уже залит в design.md и будет покрыт в
chunk 14 e2e. Логика re-invite (reset counter'ов) проверена unit-тестами сервиса.

### Self-check агента

Между unit и e2e — обязательный self-check (curl + чтение БД + spy_store). Расхождение
→ агент сам фиксит код, перезапускает self-check, сразу e2e в той же сессии. HALT
только при продуктовой развилке.

## Связанные документы

- Design: `_bmad-output/planning-artifacts/design-campaign-creator-flow.md`
- Roadmap: `_bmad-output/planning-artifacts/campaign-roadmap.md`
- Стандарты: `docs/standards/` (полностью)
