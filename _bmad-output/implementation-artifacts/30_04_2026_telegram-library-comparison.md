---
title: "Сравнение Go-либ для Telegram Bot API"
type: research
status: in-review
created: "2026-04-30"
context: "пересмотр выбора либы для chunk 1 онбординга (creator-telegram-link). Текущая реализация — собственный http-клиент через stdlib, нужно заменить на готовую библиотеку."
related:
  - "_bmad-output/implementation-artifacts/29_04_2026_creator-telegram-link-plan.md"
  - "_bmad-output/implementation-artifacts/29_04_2026_creator-telegram-link-progress.md"
  - "docs/standards/backend-libraries.md"
---

# Сравнение Go-либ для Telegram Bot API

## Критерии (финальные, после обсуждения)

### Hard rules (не проходит → REJECTED)

1. Лицензия — MIT/Apache-2.0/BSD/ISC. GPL/LGPL/AGPL — отсев.
2. Bot API через HTTPS, не MTProto.
3. `context.Context` first-class — все network-методы принимают `ctx`.
4. **Update — plain struct.** Можно сконструировать руками в тесте без сетевых вызовов и скрытого state.
5. **Updates приходят как канал / коллбэк / итератор + публичный entry point** (типа `Dispatcher.ProcessUpdate(ctx, update)`), куда можно подать synthetic update в e2e-тестах.
6. **Send-методы можно обернуть в 1-method интерфейс** (`SendMessager interface { SendMessage(ctx, params) error }`) для spy в тестах.
7. **Long polling управляемый** — offset, retry-after-aware, graceful stop через `ctx.Done()`.
8. Активная поддержка — last release ≤ 6 мес назад (на 2026-04-30 → не старее 2025-10).

### Дополнительные критерии для ранжирования

- **Звёзды на GitHub** (популярность как прокси доверия).
- Транзитивные зависимости (минимум — лучше).
- `allowed_updates` фильтрация.
- Built-in panic recover в handler.
- Webhook-режим как опция (для будущего).
- API coverage features: inline_keyboard, callback_query, sendPhoto, setMyCommands, editMessageText.
- SemVer (v1+, без частых breaking changes).
- Тесты в самой либе (CI зелёный).

### Конфликт критериев — обнаружен

**Изначальный hard rule "≥3000 ★"** исключает **все** либы с современным API (ctx-aware, plain Update, retry-after). Топ-2 по звёздам — `go-telegram-bot-api` (мёртв с 2021) и `tucnak/telebot` (нет `context.Context` в `Send`/`Stop`). Все живые ctx-aware либы — в диапазоне 130–1697 ★.

**Решение:** порог снижен до ~700 ★. Это нормальная ситуация для Go-экосистемы — современные либы ещё не накопили легаси-славу.

## Сводная таблица

| # | Либа | ★ | Last release | Update type | Update delivery | retry_after | Graceful stop | allowed_updates | 1-method spy | Webhook | API coverage | Verdict |
|---|---|---|---|---|---|---|---|---|---|---|---|---|
| 1 | **github.com/go-telegram/bot** | 1697 | v1.20.0 (2026-03-19) | plain struct | channel + public `ProcessUpdate(ctx, *Update)` | partial (typed err `*TooManyRequestsError`, без auto-retry) | yes (ctx) | yes | yes (через wrapper) | yes | full (Bot API ~9.x) | **Лучший по чистоте API**: zero deps, ctx везде, public ProcessUpdate, plain Update без скрытого state |
| 2 | **github.com/mymmrac/telego** | 1009 | v1.8.0 (2026-04-03) | plain struct (есть приватный `ctx` field) | channel + public `HandleUpdate(ctx, bot, Update)` | **YES, full** (`RetryCaller`: WaitOrAbort/Skip/Abort) | yes (ctx) | yes | yes (через wrapper) | yes | full | **Единственная с реально built-in retry**, но heavy deps (fasthttp, sonic, fastjson) |
| 3 | github.com/PaulSonOfLars/gotgbot/v2 | 699 | v2.0.0-rc.34 (2026-02-17) | plain struct | channel + public `Dispatcher.ProcessUpdate` | partial (typed err) | yes | yes | yes (через wrapper) | yes | full | Built-in panic recovery, zero deps. Минус: висит в `rc.34` уже годами, нет stable v2 |
| — | github.com/tucnak/telebot v4 | 4590 | v3.3.6 (2024-06-10) | plain struct | callback only (`bot.Handle`) | partial | `Stop()` без ctx | yes | yes | yes | full | **REJECTED**: нет `context.Context` в `Send`/`Stop` (нарушает hard rule #3) |
| — | github.com/go-telegram-bot-api/telegram-bot-api/v5 | 6401 | v5.5.1 (2021-12-13) | plain struct | `GetUpdatesChan` | errors only | manual | yes | yes | yes | partial (старый Bot API) | **REJECTED**: abandoned (4+ года, нарушает hard rule #8), нет ctx |
| — | github.com/NicoNex/echotron/v3 | 436 | v3.45.0 (2026-03-07) | plain struct | callback/channel | не нашёл | yes | yes | yes | yes | full | **REJECTED**: LGPL-3.0 license (нарушает hard rule #1) |
| — | github.com/mr-linch/go-tg | 130 | v0.18.0 | plain struct | channel via `Poller` | yes (built-in flood retry через `NewInterceptorRetryFloodError`) | yes | yes | yes | yes | full | **REJECTED**: тонкое community, ниже порога 700 ★ |
| — | github.com/OvyFlash/telegram-bot-api | 140 | v9.4.0 | plain struct | `GetUpdatesChan` + `WithContext` | partial | yes (через WithContext) | yes | yes | yes | full | **REJECTED**: fork с тонким сообществом |
| — | github.com/onrik/micha | 33 | no releases | ? | ? | ? | ? | ? | ? | ? | ? | **REJECTED**: <100 ★ |
| — | github.com/enetx/tg | 50 | ? | ? | ? | ? | ? | ? | ? | ? | ? | **REJECTED**: <100 ★ |
| — | github.com/SakoDroid/telego | 61 | last push 2024-06 | ? | ? | ? | ? | ? | ? | ? | ? | **REJECTED**: stale, name confusion с `mymmrac/telego` |

## Top-3 — детали

### 1. `github.com/go-telegram/bot` (1697 ★)

**Хорошо:**
- Zero dependencies (нет `go.sum`!) — чистый stdlib.
- Plain struct `Update` без скрытых полей (`models/update.go`).
- Public `ProcessUpdate(ctx, *Update)` — идеально для e2e с synthetic update.
- `WithHTTPClient(client HttpClient)` — `HttpClient interface { Do(*http.Request) (*http.Response, error) }` — точка mock-а на уровне HTTP, если понадобится.
- Все методы — `func (b *Bot) SendMessage(ctx, *SendMessageParams) (*Message, error)` — единая сигнатура, легко обернуть в spy.

**Плохо:**
- `RetryAfter` возвращается через `*TooManyRequestsError` (`errors.go:17`), но **либа не делает auto-retry/backoff** — нужно реализовать в нашем коде (как сейчас и сделано).
- Нет встроенного recover в handlers.

### 2. `github.com/mymmrac/telego` (1009 ★)

**Хорошо:**
- **Единственная либа с реально built-in retry_after** — `telegoapi.RetryCaller` с режимами `RetryRateLimitWait/Skip/Abort/WaitOrAbort` (`telegoapi/caller.go:216-251`).
- Public `HandlerGroup.HandleUpdate(ctx, bot, telego.Update{...})` (`telegohandler/handler_group.go:57`) для синтетических updates.
- `telegohandler/middleware.go:21` — встроенный `recover()` через `PanicRecovery`.
- Полная поддержка `allowed_updates`, webhook, ctx-aware.

**Плохо:**
- Тяжёлые зависимости: `bytedance/sonic`, `valyala/fasthttp`, `valyala/fastjson`, `grbit/go-json` — потенциальные конфликты в monorepo и больший бинарь.
- `Update` имеет приватное поле `ctx context.Context` (`types.go:119`) — для тестов нужно использовать `Update.WithContext(ctx)`.

### 3. `github.com/PaulSonOfLars/gotgbot/v2` (699 ★)

**Хорошо:**
- Zero deps (`go.sum` пустой).
- **Built-in panic recovery** в `Dispatcher.ProcessUpdate` (`ext/dispatcher.go:259-272`).
- Public `Dispatcher.ProcessUpdate(bot, *Update, data)` — synthetic update подаётся напрямую.
- Generated bindings — code consistent.
- Есть и `Bot.SendMessage`, и `Bot.SendMessageWithContext` — гибко.

**Плохо:**
- **Висит в статусе `v2.0.0-rc.34`** — 34 RC за несколько лет, нет stable v2.0.0.
- Не реализован auto-retry на retry_after — `*TelegramError` с `ResponseParams.RetryAfter`, дальше сами.
- Раздельные методы `Send`/`SendWithContext` — двойной API.

## Ссылки на ключевые файлы (для перепроверки)

### `go-telegram/bot`
- https://github.com/go-telegram/bot/blob/main/go.mod (zero deps)
- https://github.com/go-telegram/bot/blob/main/bot.go (Bot struct, HttpClient interface)
- https://github.com/go-telegram/bot/blob/main/process_update.go (public ProcessUpdate)
- https://github.com/go-telegram/bot/blob/main/get_updates.go (polling loop)
- https://github.com/go-telegram/bot/blob/main/raw_request.go (retry_after parsing)
- https://github.com/go-telegram/bot/blob/main/errors.go (TooManyRequestsError)
- https://github.com/go-telegram/bot/blob/main/models/update.go (Update plain struct)
- https://github.com/go-telegram/bot/blob/main/methods.go (SendMessage signature)
- https://github.com/go-telegram/bot/blob/main/options.go (WithHTTPClient, WithAllowedUpdates)

### `mymmrac/telego`
- https://github.com/mymmrac/telego/blob/main/go.mod (deps)
- https://github.com/mymmrac/telego/blob/main/types.go (Update with private ctx field, line ~119)
- https://github.com/mymmrac/telego/blob/main/long_polling.go (UpdatesViaLongPolling channel)
- https://github.com/mymmrac/telego/blob/main/methods.go (SendMessage signature)
- https://github.com/mymmrac/telego/blob/main/telegoapi/caller.go (RetryCaller logic line ~216-251)
- https://github.com/mymmrac/telego/blob/main/telegohandler/handler_group.go (HandleUpdate line ~57)
- https://github.com/mymmrac/telego/blob/main/telegohandler/middleware.go (PanicRecovery line ~21)

### `PaulSonOfLars/gotgbot/v2`
- https://github.com/PaulSonOfLars/gotgbot/blob/master/go.mod (zero deps)
- https://github.com/PaulSonOfLars/gotgbot/blob/master/gen_types.go (Update struct)
- https://github.com/PaulSonOfLars/gotgbot/blob/master/gen_methods.go (Bot.SendMessage, SendMessageWithContext)
- https://github.com/PaulSonOfLars/gotgbot/blob/master/request.go (BotClient interface, TelegramError)
- https://github.com/PaulSonOfLars/gotgbot/blob/master/ext/dispatcher.go (ProcessUpdate + recover, lines ~259-272)
- https://github.com/PaulSonOfLars/gotgbot/blob/master/ext/updater.go (Stop logic)

### REJECTED — для протокола
- https://github.com/tucnak/telebot/blob/v4/bot.go (Send без ctx)
- https://github.com/NicoNex/echotron/blob/master/COPYING.LESSER (LGPL — REJECT)
- https://github.com/go-telegram-bot-api/telegram-bot-api/blob/master/bot.go (нет ctx)

## Решение

_TBD — на ревью у Алихана._
