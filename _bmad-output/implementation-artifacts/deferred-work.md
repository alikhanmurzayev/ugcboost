# Deferred Work

Накопленные `defer` findings — pre-existing или surfaced incidentally в ходе ревью. Каждая запись: что, откуда (PR/spec), почему отложено.

## 2026-05-13 — spec-save-telegram-messages review

- **`service.TelegramMessageService.ListByChat` запрашивает `limit+1` без верхней границы.** Handler валидирует `limit ∈ [1,100]`, service передаёт 101 в repo. Если будущий caller (не handler) передаст бóльший limit — repo не защитит. Cosmetic, не exploitable. Источник: blind-hunter review.
- **Миграция `telegram_messages` без CHECK на пару `status` × `error`.** Recorder enforces `error IS NULL WHEN status='sent'` в коде; CHECK на табличном уровне был бы defence-in-depth. Источник: blind-hunter review.
- **`SeedTelegramMessage` testapi при 23505-collision возвращает 500, не 409/422.** Repo возвращает `domain.ErrTelegramMessageAlreadyRecorded`, error-mapper кладёт в default 500. Тест-only endpoint; для production read/recorder path семантика корректная. Источник: blind-hunter review.
- **Recorder DB-INSERT может блокировать Notifier `fire` goroutine.** Recorder синхронен; pgx-вызов разделяет `attemptCtx` deadline (10s). Если DB подвисла > timeout — WaitGroup может не успеть в graceful shutdown. Best-effort по спеке, но `context.WithTimeout` на сам INSERT добавил бы запас. Источник: edge-case-hunter.
- **Retry storm на DB outage → 4× error log per failure × N concurrent notifies.** Recorder Error-логирует каждую неудачу INSERT; нет rate-limit / dedup. Operational alerting concern. Источник: edge-case-hunter.
- **`RecordingSender` на client-disconnect создаёт false-positive `status=failed`.** `SendCampaignInvite/Reminder` принимают request ctx; при отмене ctx Telegram может успеть отправить, но recorder запишет error="context canceled". UI «failed-rows» вьюшка может ввести в заблуждение. Источник: edge-case-hunter.
- **`uuid.MustParse` → `uuid.Parse`** (`handler/telegram_message.go`) — applied as patch. Test для error path не написан (UUID column в БД делает path practically unreachable). Источник: blind-hunter + edge-case-hunter.
