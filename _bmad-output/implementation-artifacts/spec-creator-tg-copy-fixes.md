---
title: 'Обновление copy для Telegram-сообщений креатор-onboarding'
type: 'chore'
created: '2026-05-04'
status: 'done'
baseline_commit: '6505a6c'
context:
  - docs/standards/backend-testing-unit.md
  - docs/standards/backend-testing-e2e.md
---

<frozen-after-approval reason="human-owned intent — do not modify unless human renegotiates">

## Intent

**Problem:** Тексты, которые бот шлёт креатору после привязки Telegram и после авто-верификации через Instagram DM, устарели — формулировки не соответствуют текущему onboarding-нарративу (нет «модерации», переход на «результаты отбора», тон теплее).

**Approach:** Заменить значения трёх констант копирайта в `notifier.go`, синхронно обновить fixture-копии в трёх тест-файлах (один unit, два e2e) и подкорректировать narrative-докстринги e2e-пакетов, упоминающие устаревшие фразы. Поведение, HTML-режим и `%s`-подстановка кода — без изменений.

## Boundaries & Constraints

**Always:**
- Текст #1 — HTML, `<pre>%s</pre>` вокруг verification-кода сохраняется (tap-to-copy), `%s` через `html.EscapeString`.
- Тексты #2, #3 — plain, `ParseMode` не выставляется.
- Fixture-константы в тестах — литералы (намеренный дубль продакшен-источника); ассерты — `require.Equal` на полный текст, не substring.
- `make lint-backend && make test-unit-backend && make test-e2e-backend` — зелёные.

**Ask First:**
- Любое отклонение от трёх присланных текстов (эмодзи, переносы, URL `https://ig.me/m/ugc_boost`).

**Never:**
- Не трогать `Notifier.fire`, `buildWelcomeText`, `ApplicationLinkedPayload` и логику отправки.
- Не трогать `messages.go` (синхронные reply'и — отдельный контракт).
- Не вводить i18n / выноса в YAML — вне scope.

## I/O & Edge-Case Matrix

| Сценарий | Состояние | Ожидаемое поведение |
|----------|-----------|----------------------|
| /start, заявка с Instagram | `HasInstagram=true`, code `UGC-NNNNNN` | Текст #1, `ParseMode=HTML`, код в `<pre>` через `html.EscapeString` |
| /start, заявка без Instagram | `HasInstagram=false` | Текст #2, `ParseMode` пустой |
| Авто-верификация (SendPulse webhook) | verification → moderation | Текст #3, `ParseMode` пустой, без inline-кнопки |
| Verification-код с HTML-метасимволами | `UGC-<x>` | `&lt;x&gt;` в `<pre>` (контракт сохраняется) |

</frozen-after-approval>

## Code Map

- `backend/internal/telegram/notifier.go` — три константы (`welcomeWithIGTemplate`, `welcomeNoIGText`, `verificationApprovedText`).
- `backend/internal/telegram/notifier_test.go` — fixture'ы `expectedWelcomeWithIG()`, `expectedWelcomeNoIG`, `expectedVerificationApproved`.
- `backend/e2e/telegram/telegram_test.go` — fixture'ы `welcomeWithIGText()`, `welcomeNoIGText`; narrative header (строки 13–15) упоминает «Спасибо за заявку».
- `backend/e2e/webhooks/sendpulse_instagram_test.go` — константа `verificationApprovedText`; narrative header (строки 1–3, 21) упоминает «Заявка ушла на модерацию» / «модерацию».

## Tasks & Acceptance

**Execution:**
- [x] `backend/internal/telegram/notifier.go` — заменить значения трёх констант на новые тексты (Design Notes); сохранить `%s` в #1, эмодзи, индентацию-в-3-пробела перед `<pre>` и URL.
- [x] `backend/internal/telegram/notifier_test.go` — синхронизировать три fixture'а с новыми текстами байт-в-байт.
- [x] `backend/e2e/telegram/telegram_test.go` — синхронизировать `welcomeWithIGText()` / `welcomeNoIGText`; обновить narrative header (вместо «Спасибо за заявку» — фраза из нового #2).
- [x] `backend/e2e/webhooks/sendpulse_instagram_test.go` — синхронизировать `verificationApprovedText`; обновить narrative header (вместо «Заявка ушла на модерацию» / «с подстрокой «модерацию»» — фраза из нового #3).

**Acceptance Criteria:**
- Given заявка с Instagram, when `/start <appID>`, then `/test/telegram/sent` отдаёт ровно текст #1 с `UGC-NNNNNN` в `<pre>` и `ParseMode=HTML`.
- Given заявка без Instagram, when `/start <appID>`, then приходит ровно текст #2, `ParseMode` пустой.
- Given заявка в verification, when SendPulse-webhook с валидным кодом, then приходит ровно текст #3, `ParseMode` пустой, без inline-кнопки.
- `make lint-backend && make test-unit-backend && make test-e2e-backend` — зелёные.

## Spec Change Log

## Design Notes

Whitespace и переносы строк значимы — fixture-константы байт-в-байт.

**Текст #1 (`welcomeWithIGTemplate`, HTML, `%s` = код):**

```
Здравствуйте! 👋

Мы получили вашу заявку.
Подтвердите, пожалуйста, что вы действительно владеете указанным аккаунтом Instagram:

1. Скопируйте код:

   <pre>%s</pre>

2. Откройте Direct и отправьте его нам:

   https://ig.me/m/ugc_boost
```

**Текст #2 (`welcomeNoIGText`, plain):**

```
Здравствуйте! 👋

Мы получили вашу заявку. Скоро сообщим здесь результаты отбора ✅
```

**Текст #3 (`verificationApprovedText`, plain):**

```
Вы успешно подтвердили свой аккаунт ✅

Скоро сообщим здесь результаты отбора 🖤
```

## Verification

**Commands:**
- `make lint-backend` — expected: пусто, exit 0.
- `make test-unit-backend` — expected: `TestNotifier_NotifyApplicationLinked` / `TestNotifier_NotifyVerificationApproved` зелёные.
- `make test-e2e-backend` — expected: `TestTelegramLink` (with-IG / no-IG / idempotent / already-linked-elsewhere) и `TestSendPulseInstagramWebhook` (happy / self-fix / already-verified) зелёные.

## Suggested Review Order

**Источник правды (старт)**

- Текст #1, HTML с `<pre>%s</pre>` для tap-to-copy, без trailing-закрывашки
  [`notifier.go:31`](../../backend/internal/telegram/notifier.go#L31)

- Текст #2, plain, однострочный закрывающий блок с ✅
  [`notifier.go:42`](../../backend/internal/telegram/notifier.go#L42)

- Текст #3, plain, новый narrative без слова «модерация», с 🖤
  [`notifier.go:47`](../../backend/internal/telegram/notifier.go#L47)

**Fixture-дубли (assert-by-equality, должны совпадать байт-в-байт)**

- Unit-fixture для #1
  [`notifier_test.go:59`](../../backend/internal/telegram/notifier_test.go#L59)

- Unit-fixture для #2 и #3
  [`notifier_test.go:70`](../../backend/internal/telegram/notifier_test.go#L70)

- E2E-fixture для #2
  [`telegram_test.go:78`](../../backend/e2e/telegram/telegram_test.go#L78)

- E2E-fixture для #1
  [`telegram_test.go:86`](../../backend/e2e/telegram/telegram_test.go#L86)

- E2E-fixture для #3
  [`sendpulse_instagram_test.go:362`](../../backend/e2e/webhooks/sendpulse_instagram_test.go#L362)

**Narrative-комментарии (синхронизация устаревших фраз)**

- Header e2e/telegram: «Спасибо за заявку» → «Скоро сообщим здесь результаты отбора»
  [`telegram_test.go:13`](../../backend/e2e/telegram/telegram_test.go#L13)

- Header e2e/webhooks: «Заявка ушла на модерацию» → «Вы успешно подтвердили свой аккаунт»
  [`sendpulse_instagram_test.go:1`](../../backend/e2e/webhooks/sendpulse_instagram_test.go#L1)

- Сценарий happy path: «с подстрокой «модерацию»» → «с фразой «успешно подтвердили»»
  [`sendpulse_instagram_test.go:22`](../../backend/e2e/webhooks/sendpulse_instagram_test.go#L22)
