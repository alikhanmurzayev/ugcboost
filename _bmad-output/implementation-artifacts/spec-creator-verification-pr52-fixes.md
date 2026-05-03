---
title: 'PR #52 review fixes + outbound Telegram spy для e2e'
type: 'refactor'
created: '2026-05-03'
status: 'in-review'
baseline_commit: '97fc21944c3985a7a528890187755e36b4c66fed'
context:
  - docs/standards/backend-libraries.md
  - docs/standards/testing.md
---

<frozen-after-approval reason="human-owned intent — do not modify unless human renegotiates">

## Intent

**Problem:** Ревью PR #52 (chunk 8 — SendPulse webhook) выкатило 12 правок: лишние тест-ручки, шумные комментарии, неверный severity логов, ручной HTTP-клиент в e2e вместо сгенерированного, отсутствие graceful shutdown для pending TG-уведомлений, тихий fallback при пустом TG-токене. Плюс реализация не имеет наблюдаемости отправок в Telegram — e2e не проверяет, что пользователю улетело сообщение «верификация пройдена».

**Approach:** Прокидываем `verificationCode` в админский `/creators/applications/{id}` и удаляем тест-ручку. Сжимаем комментарии. Делаем `TELEGRAM_BOT_TOKEN` обязательным; при `TELEGRAM_MOCK=true` подменяем sender на spy-only-обёртку, при `false` (staging/prod) оборачиваем реальный bot в TeeSender, дублирующий каждый исходящий вызов в общий in-memory `SentSpyStore` (capacity 5000). Новая тест-ручка `GET /test/telegram/sent` выдаёт записи. Вся проводка Telegram уезжает из `cmd/api/main.go` в `cmd/api/telegram.go`. Pending notify-горутины tracking'ятся через WaitGroup, ждутся в closer. E2E-тесты webhook'а переписываются на сгенерированный API-клиент и расширяются: проверяют outbound TG-сообщение, полный audit payload, `verifiedAt` / `verifiedByUserID`.

## Boundaries & Constraints

**Always:**
- Anti-fingerprint поведение webhook'а сохраняется (200 `{}` для любого исхода после bearer'а, 401 `{}` до).
- `verification_code` остаётся секретом наружу: выдаётся ТОЛЬКО в админском `/creators/applications/{id}` (всегда заполнен, не nullable — публичных ручек на деталях у нас нет).
- Только сгенерированные API-клиенты в e2e; ручной `net/http` запрещён.
- Spy-store работает в local / isolated CI / staging (везде где `EnableTestEndpoints=true`), отключается в production.
- `TELEGRAM_BOT_TOKEN` обязателен и непуст в `config.Load()` ТОЛЬКО когда `TELEGRAM_MOCK=false`. При mock=true токен может быть пустым — sender работает в spy-only режиме без сетевых вызовов.
- WaitGroup на pending TG-нотификации ждётся в closer перед закрытием pool'а.

**Ask First:**
- Если `getApplicationDetail` уже возвращает данные большим количеством запросов и добавление поля требует менять SQL — подтвердить путь (расширить существующий запрос vs. отдельный select).
- Если в e2e обнаружится что test-only `chatID` пересекается между параллельными тестами — обсудить scheme генерации (hash от `t.Name()` vs. отдельный счётчик).

**Never:**
- Не делать публичных ручек чтения деталей заявки.
- Не подменять production sender spy-обёрткой.
- Не оставлять `Sender == nil` веткой (после правки она становится мёртвым кодом — удалить).
- Не вводить shared mutable state между параллельными e2e-тестами помимо самого spy-store (он thread-safe внутри).
- Не делать spy через MITM, mock-Telegram-server или Redis — только in-memory ring.

## I/O & Edge-Case Matrix

| Scenario | Input / State | Expected Output / Behavior | Error Handling |
|---|---|---|---|
| TG mock включён, webhook happy-path с залинкованной заявкой | `TELEGRAM_MOCK=true`, app linked → chat_id=42 | spy-store содержит SentRecord{ChatID=42, Text непустой, ReplyMarkup с WebApp.URL} | N/A |
| TG real (staging), webhook happy-path | `TELEGRAM_MOCK=false`, real bot | Реальная отправка в Telegram + параллельная запись в spy-store с тем же ChatID/Text | Ошибка отправки → запись с Err, sender возвращает err, service логирует Error |
| Webhook против заявки без TG-link | linked == nil | Никакой записи в spy-store; service логирует Info «not linked» | N/A |
| Spy-store overflow | >5000 записей | Старые вытесняются ring-буфером, lookup по ChatID работает на свежих | N/A |
| `GET /test/telegram/sent?chatId=X&since=T` | EnableTestEndpoints=true | 200 {messages: [...]} отфильтровано по chatID и since | EnableTestEndpoints=false → ручка не зарегистрирована |
| Empty TG_BOT_TOKEN при mock=false | `TELEGRAM_BOT_TOKEN=""`, `TELEGRAM_MOCK=false` | `config.Load()` возвращает error, сервис не стартует | Fail-fast |
| Empty TG_BOT_TOKEN при mock=true | `TELEGRAM_BOT_TOKEN=""`, `TELEGRAM_MOCK=true` | Сервис стартует, sender = SpyOnlySender | N/A |
| Empty body POST в webhook | `request.Body == nil` | 200 `{}`, лог Warn (не Debug) | N/A |
| Закрытие сервиса с pending notify | SIGTERM во время `notifyVerificationApproved` | closer ждёт WaitGroup до telegramNotifyTimeout, потом закрывает pool | Ждать ровно telegramNotifyTimeout, не дольше |

</frozen-after-approval>

## Code Map

- `backend/api/openapi.yaml` -- `CreatorApplicationDetailData.verificationCode` (string, required)
- `backend/api/openapi-test.yaml` -- удалить `/test/creator-applications/{id}/verification-code`, добавить `/test/telegram/sent`
- `backend/internal/config/config.go` -- сделать `TELEGRAM_BOT_TOKEN` required+non-empty (с условием на `TELEGRAM_MOCK`); добавить `TelegramMock bool`
- `backend/.env.example`, `backend/.env.ci` -- `TELEGRAM_MOCK=true`, dummy `TELEGRAM_BOT_TOKEN`
- `backend/cmd/api/main.go` -- удалить inline-комменты вокруг tgSender, перевести проводку на хелперы из telegram.go
- `backend/cmd/api/telegram.go` (NEW) -- `setupTelegram(cfg, log) (Sender, *SentSpyStore, runner func, closer func)` + helpers
- `backend/internal/telegram/spy_store.go` (NEW) -- `SentSpyStore` (ring 5000, mutex, Filter)
- `backend/internal/telegram/tee_sender.go` (NEW) -- `SpyOnlySender`, `TeeSender`
- `backend/internal/service/creator_application.go` -- ужать комментарии в `VerifyInstagramByCode` + `notifyVerificationApproved`; убрать `nil sender` ветку (мёртвая после config-fix); зарегистрировать notify-горутину в общей WaitGroup, переданной в конструктор
- `backend/internal/service/creator_application.go` -- конструктор принимает `*sync.WaitGroup` для tracking pending notifications
- `backend/internal/handler/webhook_sendpulse.go` -- сжать комменты, инлайн `sendPulseEmptyOK`, empty body → `Warn`
- `backend/internal/handler/testapi.go` -- удалить `GetCreatorApplicationVerificationCode`; добавить `GetTelegramSent` принимающий spy-store
- `backend/internal/handler/creator_application.go` (или mapper) -- проброс `verificationCode` из row в DTO
- `backend/internal/repository/creator_application.go` -- убедиться что `GetByID` уже возвращает `VerificationCode` (он там есть)
- `backend/e2e/testutil/sendpulse_webhook.go` -- переписать на `apiclient.ClientWithResponses.SendPulseInstagramWebhookWithResponse` через `RequestEditorFn`
- `backend/e2e/testutil/telegram_sent.go` (NEW) -- `GetTelegramSent(t, chatID, since)`
- `backend/e2e/testutil/creator_application.go` -- удалить `GetCreatorApplicationVerificationCode` (читать из admin detail напрямую)
- `backend/e2e/webhooks/sendpulse_instagram_test.go` -- доп assert'ы (verifiedAt, verifiedByUserID, full audit payload, TG-spy в happy/self-fix); линковка TG в setup
- `backend/internal/service/creator_application_test.go` -- happy-path: capture `*bot.SendMessageParams`, deep-assert ChatID/Text/WebApp URL; полный audit payload; удалить `nil sender` тест
- `backend/cmd/api/main.go` -- передать WaitGroup в `closer.Add("telegram-notify-wait", wg.Wait)` ПЕРЕД pool close

## Tasks & Acceptance

**Execution:**
- [x] `backend/internal/telegram/spy_store.go` -- создать `SentSpyStore` (ring buffer 5000, mutex, типы `SentRecord`/`SentFilter`, методы `Record`/`List`)
- [x] `backend/internal/telegram/tee_sender.go` -- создать `SpyOnlySender` (запись только в store) и `TeeSender` (real + store, не зависит от ошибки real)
- [x] `backend/internal/config/config.go` -- добавить `TelegramMock bool` env (default `false`); требовать `TELEGRAM_BOT_TOKEN` непустым только если `TelegramMock=false`
- [x] `backend/.env.example`, `backend/.env.ci` -- выставить `TELEGRAM_MOCK=true`, `TELEGRAM_BOT_TOKEN` оставить пустым (mock-режим не требует)
- [x] `backend/cmd/api/telegram.go` -- вынести проводку: `setupTelegramSender`, `setupTelegramRunner`; вернуть sender, spy, runner-cleanup, notify-WaitGroup
- [x] `backend/cmd/api/main.go` -- заменить inline-блок на 1-2 вызова из telegram.go; зарегистрировать `telegram-notify-wait` ДО `postgres` close
- [x] `backend/internal/service/creator_application.go` -- конструктор принимает `*sync.WaitGroup`; `notifyVerificationApproved` делает `wg.Add(1)`/`Done`; удалить `nil sender` ветку; ужать комментарии
- [x] `backend/api/openapi.yaml` -- добавить `verificationCode: string (required)` в `CreatorApplicationDetailData`; ужать description webhook'а до 1-2 строк
- [x] `backend/internal/handler/creator_application.go` (mapper) -- проброс `VerificationCode` row → DTO в админский detail-handler
- [x] `backend/api/openapi-test.yaml` -- удалить `/test/creator-applications/{id}/verification-code`; добавить `/test/telegram/sent` с фильтрами `chatId` (int64, required) и `since` (date-time, optional)
- [x] `backend/internal/handler/testapi.go` -- удалить `GetCreatorApplicationVerificationCode`; добавить `GetTelegramSent` (читает spy-store)
- [x] `backend/internal/handler/webhook_sendpulse.go` -- empty body → `Warn`; убрать `sendPulseEmptyOK` (инлайн); сжать шапочный комментарий до 2-3 строк
- [x] `make generate-api` -- перегенерировать клиенты (server.gen.go, apiclient, openapi-typescript для всех 3 фронтов)
- [x] `backend/e2e/testutil/sendpulse_webhook.go` -- переписать через сгенерированный `ClientWithResponses` + `RequestEditorFn` для bearer; убрать ручной net/http
- [x] `backend/e2e/testutil/telegram_sent.go` -- хелпер `GetTelegramSent(t, chatID, since) []apiclient.TestTelegramSentMessage`
- [x] `backend/e2e/testutil/creator_application.go` -- удалить `GetCreatorApplicationVerificationCode`; добавить `GetVerificationCodeFromAdminDetail(t, adminClient, token, appID)`
- [x] `backend/e2e/webhooks/sendpulse_instagram_test.go` -- линковка TG в happy/self-fix через `/test/telegram/message` (`/start <appID>`); добавить assert'ы на spy-store, verifiedAt, verifiedByUserID, полный audit payload
- [x] `backend/internal/service/creator_application_test.go` -- happy-path: capture `*bot.SendMessageParams`, deep-assert ChatID/Text/InlineKeyboardMarkup.WebApp.URL; полный audit payload (`application_id`, `social_id`, `from_status`, `to_status`); удалить тест `nil sender`
- [x] `make lint-backend test-unit-backend test-e2e-backend` -- зелёный полный гейт перед PR-update

**Acceptance Criteria:**
- Given залинкованная заявка в `verification` и валидный SendPulse-вебхук, when приходит правильный код, then в spy-store появляется ровно одна запись с правильным ChatID и WebApp-кнопкой, заявка переходит в `moderation`, audit content включает все 5 полей.
- Given `TELEGRAM_BOT_TOKEN=""` и `TELEGRAM_MOCK=false`, when запускается сервис, then `config.Load()` возвращает error и сервис не стартует.
- Given `TELEGRAM_BOT_TOKEN=""` и `TELEGRAM_MOCK=true`, when запускается сервис, then сервис стартует с `SpyOnlySender`, никаких сетевых вызовов в Telegram не происходит.
- Given активная notify-горутина в фоне, when сервис получает SIGTERM, then closer ждёт её завершения (до `telegramNotifyTimeout=10s`) перед закрытием pool'а.
- Given `EnableTestEndpoints=false` (production), when приходит запрос на `/test/telegram/sent`, then 404 (ручка не зарегистрирована).
- Given e2e-тесты SendPulse webhook'а, when они инспектируют клиент, then используется только сгенерированный `apiclient.ClientWithResponses` (никакого `net/http.NewRequest` к webhook-пути).
- Given админ запрашивает `/creators/applications/{id}`, when возвращается detail, then `verificationCode` присутствует и совпадает с persisted значением.
- Given `make lint-backend test-unit-backend test-e2e-backend`, then все три зелёные.

## Verification

**Commands:**
- `make generate-api` -- expected: clean diff на server.gen.go / typescript schema
- `make lint-backend` -- expected: 0 issues
- `make test-unit-backend` -- expected: PASS, новые asserts покрывают TG params + полный audit payload
- `make test-unit-backend-coverage` -- expected: 80%+ для новых файлов (`telegram/spy_store.go`, `telegram/tee_sender.go`, `cmd/api/telegram.go`)
- `make test-e2e-backend` -- expected: 8 webhook-сценариев + новые TG-spy asserts проходят
- CI на ветке -- expected: все джобы зелёные, включая `Staging E2E Backend`

**Manual checks:**
- `cmd/api/main.go` глазами: блок Telegram-проводки умещается в ≤4 строки и читается без скроллинга.
- Открыть PR-diff в GitHub: убедиться, что все 12 review-comment'ов закрыты (либо resolved, либо ответ-комментарием).
