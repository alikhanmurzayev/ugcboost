---
title: 'Deferred work surfaced during chunk-9 review'
type: backlog
created: '2026-05-03'
status: living
---

# Deferred work

Items surfaced by step-04 review of chunk 9 (`spec-creator-bot-notify-foundation.md`) that are real but out of scope for this PR. Each item is sized for a future independent slice.

## From chunk 9 review (2026-05-03)

### LinkTelegram welcome не учитывает status заявки
- Сейчас welcome зависит только от `hasInstagram`. Заявка в `moderation`/`rejected`/`approved` (post-verification) тоже получит «отправьте код в IG» — confusing.
- Когда: при добавлении state-machine v2 (chunk 17) или раньше, если в проде попадётся реальный кейс «креатор линкуется после ручной верификации».
- Как: вынести status-aware ветвление в `LinkTelegram` (или в notifier), 3 варианта: verification (как сейчас), moderation, terminal.

### Closer-ordering race: telegram-runner vs notifier.Wait
- Pre-existing chunk-8 issue, не регрессия chunk 9. `cl.Add("telegram-runner", ...)` не блокирует до возврата `b.Start(ctx)`, между шагами LIFO остаётся окно когда bot-handler goroutine может вызвать `notifier.NotifyApplicationLinked` уже после того как `notifier.Wait()` отработал.
- Когда: при следующей касающейся closer'а задаче, либо когда нужна гарантированная durability welcome'ов.
- Как: пробросить `runnerDone chan struct{}` из `telegram.Run`, дождаться его в closer-step `telegram-runner` ДО возврата.

### Link-hijack: код раскрывается любому Telegram-юзеру с UUID
- Если UUID заявки утечёт (скриншот лендинга, переписка), посторонний может занять link и получить welcome с `UGC-NNNNNN`. Pre-chunk-9 поведение: link был возможен любому, но код в чат не ходил. Теперь — ходит.
- Когда: до публичного релиза креаторам, либо при первом zenде в support'е.
- Как: либо сделать idempotent re-link молчаливым (не слать code на повторных /start), либо ограничить welcome на «первый успех» через `welcome_sent_at` колонку.

### Лишний SELECT social-rows при каждом /start
- `socialRepo.ListByApplicationID` дёргается на каждом `/start`, включая идемпотентные re-link'и. Под нагрузкой 100+ креаторов спамящих `/start` это N+M roundtrips.
- Когда: при оптимизации под live load, либо при добавлении welcome-tracking колонки.
- Как: либо `EXISTS(SELECT 1 ... WHERE platform='instagram')`, либо persist `has_instagram` флаг при первом link.

### `SentSpyStore` FIFO eviction делает O(n) shift при заполнении
- `s.records = s.records[1:]` — header advance, underlying array хранит ссылки до релокации (мелкий memory pin, slow shift). Не блокер, но тестовая инфра под спам.
- Когда: если staging memory начнёт расти на длинных runs.
- Как: ring buffer с фиксированными head/tail или `copy(records, records[1:])` + truncate.

### `telegramNotifyTimeout` hardcoded 10s
- Не настраивается через config. На staging синтетические chat_id всегда падают, 10s × N тестов добавляет к shutdown drain.
- Когда: при тюнинге prod-задержек или во время очередного PR-ревью telegram-инфры.
- Как: `cfg.TelegramNotifyTimeout` с default 10s, прокинуть через `NewNotifier`.

### Hardcoded `https://ig.me/m/ugc_boost` в notifier.go
- Маркетинговый URL в исходнике. Ребрендинг → diff везде + tests.
- Когда: при первом ребрендинге IG-аккаунта или при добавлении конфига для мульти-окружений (staging-бот может слать в другой DM).
- Как: вытащить в `cfg.IGDirectURL` или `domain` константу.
