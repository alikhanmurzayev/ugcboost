---
title: "Intent: chunk 17 — TrustMe webhook receiver"
type: intent
status: draft
created: "2026-05-09"
roadmap: _bmad-output/planning-artifacts/campaign-roadmap.md
parent_intent: _bmad-output/implementation-artifacts/intent-trustme-contract-v2.md
related_spec: _bmad-output/implementation-artifacts/spec-16-trustme-outbox-worker.md
---

# Intent: chunk 17 — TrustMe webhook receiver

## Преамбула — стандарты обязательны

Перед любой строкой production-кода агент обязан полностью загрузить все файлы `docs/standards/` (через `/standards`). Применимы все. Особенно: `backend-architecture.md`, `backend-codegen.md`, `backend-constants.md`, `backend-design.md`, `backend-errors.md`, `backend-libraries.md`, `backend-repository.md`, `backend-testing-e2e.md`, `backend-testing-unit.md`, `backend-transactions.md`, `naming.md`, `security.md`, `review-checklist.md`. Каждое правило — hard rule; отклонение = finding.

Single source of truth для design-решений Группы 7 — `intent-trustme-contract-v2.md` (Decisions #5, #7, #9, #14 + раздел «Webhook flow»). Этот intent — конкретизация уровня PR-чанка для приёмной части. Не повторяет архитектурные решения, ссылается.

## Тезис

Chunk 17 — `POST /trustme/webhook` (public, статичный bearer-auth, idempotent по `(trustme_document_id, target_status_code)`): обрабатывает все TrustMe-статусы 0–9 → `contracts.trustme_status_code` обновляется всегда; `campaign_creators.status` переводится только для `3 → signed` и `9 → signing_declined` (1/2/4–8 — no-op в state-machine + audit-warning); креатору шлётся бот-уведомление (3 — готовый `campaignContractSignedText`; 9 — новый текст «если появятся другие подходящие предложения — напишем»). Скачивание signed PDF — отложено на отдельный mini-PR.

## Скоуп

**В чанк 17 входит:**

- Новый endpoint `POST /trustme/webhook` (public, статичный bearer-auth) в `backend/api/openapi.yaml` + регенерация типов.
- `TrustMeWebhookService.HandleEvent(ctx, payload)` — приём payload'а, dispatcher по `subject_kind`, idempotent UPDATE `contracts.trustme_status_code` + state-transition `campaign_creators.status` (только для status=3 и status=9) — всё в одной Tx с audit ВНУТРИ Tx.
- Бот-уведомления креатору **после Tx** (fire-and-forget): для status=3 — существующий `campaignContractSignedText`; для status=9 — новый текст decline (`campaignContractDeclinedText`) + `Notifier.NotifyCampaignContractSigned` / `NotifyCampaignContractDeclined` обёртки.
- Audit actions: `campaign_creator.contract_signed`, `campaign_creator.contract_signing_declined`, `campaign_creator.contract_unexpected_status` (для статусов 1/2/4–8 как audit-warning без state-transition).
- Конфиг: `TrustMeWebhookToken` env var → `internal/config`.
- Пояснения в `backend/internal/repository/contracts.go` — godoc-блок над константами `ContractColumnTrustMe*` и полями `ContractRow.TrustMeDocumentID/ShortURL/StatusCode`. Документирует TrustMe-непоследовательность нейминга одного и того же ID: `document_id` в response отправки (`/SendToSignBase64FileExt`), `id` в `/search/Contracts`, `contract_id` в webhook payload. У нас всегда `trustme_document_id`. Правка кода chunk 16 — войдёт в наш PR после merge ветки 16.
- Unit-тесты на `TrustMeWebhookService` (per status 0..9 + idempotency + lookup-fail), handler-тесты (401/404/422/200), e2e на endpoint с реальным DB-state.

**Out of scope (отложено / в других чанках):**

- **Скачивание `signed_pdf_content`** через `DownloadContractFile` — отложено. Поле в БД остаётся (миграция была в chunk 16), наполнение — отдельный mini-PR позже (recovery-job или admin-кнопка). Это упрощает webhook-handler: после Tx по status=3 — только бот-уведомление.
- Frontend-индикатор статуса контракта — встраивается в chunk 15 (расширение страницы кампании со статусами и счётчиками), не отдельный chunk.
- Pre-flight `SetHook` (регистрация webhook URL + bearer-токена в TrustMe) — runbook, не код.
- Поддержка `subject_kind != 'campaign_creator'` — расширение dispatcher'а под `brand_agreement` будет, когда понадобится (per parent intent #8). Сейчас один branch + 422 на остальное.

## Принятые решения

1. **Текст бот-уведомления при отказе (status=9):**

   ```
   Поняли, в этот раз не подписываем. Если появятся другие подходящие предложения — обязательно вам напишем 💫
   ```

   Стиль одной строкой с эмодзи в конце — соответствует существующим `campaignContractSentText` и `campaignContractSignedText` в `internal/telegram/notifier.go`. Living document — итерируется после первых отправок, не frozen.

   Константа в `internal/telegram/notifier.go` рядом с уже существующими: `campaignContractDeclinedText` + getter `CampaignContractDeclinedText()` для тестов.

2. **Endpoint surface — `POST /trustme/webhook` (public).**

   - **Auth**: header `Authorization: Bearer <token>` (scheme case-insensitive). Сравнение со статичным `TrustMeWebhookToken` из конфига (constant-time comparison через `subtle.ConstantTimeCompare` per security.md). Blueprint § «Установка хуков» (строка 1041) описывает формат как `Authorization: {token}`, но реальный кабинет TrustMe шлёт Bearer-prefixed (подтверждено staging-дампом 2026-05-09).
   - **Request body** (JSON, OpenAPI schema по blueprint § «Содержимое хука»):
     ```json
     { "contract_id": "string", "status": 0..9, "client": "string", "contract_url": "string" }
     ```
     Имена полей в schema повторяют wire-format TrustMe. `description` каждого поля в OpenAPI поясняет реальный смысл (`contract_id` → «TrustMe-side document identifier; маппится на наш `contracts.trustme_document_id` колонку»; `client` → «номер инициатора события, не используем»; `contract_url` → «детерминирован от `contract_id`, не используем»). В Go-handler сразу маппим в локальный `trustmeDocumentID := req.ContractID` для clarity ниже по стеку.
   - **Response codes**:
     - `200` (пустой body) — успех (включая no-op idempotent повтор того же события).
     - `401` — bad/missing token. Anti-fingerprinting (security.md): один и тот же `error.Message` для отсутствующего и неверного токена.
     - `404` — unknown `contract_id` (в нашей БД нет ряда с таким `trustme_document_id`). Принципиально отдаём 404, чтобы узнавать о потерянных webhook'ах через мониторинг — если TrustMe начнёт ретраить в loop'е, вылезет в логах + разберём отдельно. Альтернатива (200 для retry-friendly) скрывает баги.
     - `422` — `status` вне диапазона 0..9 либо `subject_kind` найденного contract'а не `'campaign_creator'` (защита под расширение dispatcher'а на `brand_agreement`).

3. **Маппинг payload → БД.**

   | Wire (TrustMe webhook) | DB column                       | Действие                                                          |
   |------------------------|---------------------------------|-------------------------------------------------------------------|
   | `contract_id` (string) | `contracts.trustme_document_id` | lookup (`WHERE trustme_document_id = $1`) — точка входа           |
   | `status` (int 0..9)    | `contracts.trustme_status_code` | UPDATE c idempotency-guard `WHERE trustme_status_code != $new`    |
   | `client` (string/phone)| —                               | игнорируем (PII; в audit/log не пишем — security.md hard rule)    |
   | `contract_url` (string)| —                               | игнорируем (детерминирован от `contract_id`, уже записан в `trustme_short_url` после Phase 3 outbox; перезапись избыточна) |

   **Side-effects per транзакция (внутри Tx):**
   - Всегда: `contracts.webhook_received_at = now()`, `contracts.updated_at = now()`, INSERT audit_logs.
   - status=3 (полностью подписан): `contracts.signed_at = now()`, `campaign_creators.status = 'signed'`, `campaign_creators.updated_at = now()`.
   - status=9 (отказ креатора): `contracts.declined_at = now()`, `campaign_creators.status = 'signing_declined'`, `campaign_creators.updated_at = now()`.
   - status=0/1/2/4/5/6/7/8: только `trustme_status_code` обновляется. `campaign_creators.status` не трогаем (parent intent Decisions #7 + #14). Audit-action: `contract_unexpected_status` для 1/4/5/6/7/8 (warning); 0/2 — info-level audit без warning (это ожидаемые intermediate состояния).

   **Бот-уведомление (после COMMIT, fire-and-forget):**
   - status=3 → `Notifier.NotifyCampaignContractSigned(ctx, telegramUserID)` с текстом `campaignContractSignedText`.
   - status=9 → `Notifier.NotifyCampaignContractDeclined(ctx, telegramUserID)` с текстом `campaignContractDeclinedText` (Decision #1).
   - Остальные статусы — без уведомления.

4. **Service-архитектура — `internal/contract/webhook_service.go`.**

   - **Расположение / имя**: пакет `internal/contract/` (симметрично `sender_service.go` от chunk 16). Структура — `WebhookService` (без `TrustMe`-префикса; контекст задан пакетом + endpoint-path'ом). Альтернатива `internal/trustme/webhook_service.go` отвергнута: пакет `trustme` сейчас держит только outbound HTTP-клиент (`Client`/`RealClient`/`SpyOnly`), service-слой логичнее в `contract/`.
   - **Контракт**: `HandleEvent(ctx context.Context, payload domain.TrustMeWebhookEvent) error`. Принимает domain-DTO, не api-type (service-слой не зависит от `api/` per `backend-architecture.md`). Handler конвертит из сгенерированного `api.PostTrustmeWebhookJSONRequestBody` в `domain.TrustMeWebhookEvent` перед вызовом.
   - **Granular error codes** (per `backend-errors.md` actionable + granular):
     - `CodeContractWebhookUnknownDocument` → handler маппит в HTTP 404.
     - `CodeContractWebhookUnknownSubject` → 422 (consistency-defence на будущее под `brand_agreement`).
     - `CodeContractWebhookInvalidStatus` → 422 (`status` вне 0..9).
   - **Зависимости (через конструктор `NewWebhookService(...)`, struct иммутабельна per `backend-design.md`):**
     - `pool dbutil.Pool` — для `dbutil.WithTx`.
     - `repoFactory WebhookRepoFactory` — подмножество `RepoFactory`, перечисляющее только нужные конструкторы (`NewContractsRepo`, `NewCampaignCreatorsRepo`, `NewCreatorsRepo`, `NewAuditRepo`). Образец — `ContractSenderRepoFactory` в `internal/contract/sender_service.go:48`.
     - `notifier WebhookNotifier` — узкий интерфейс с двумя методами: `NotifyCampaignContractSigned(ctx, chatID int64)`, `NotifyCampaignContractDeclined(ctx, chatID int64)`. Реализация — `*telegram.Notifier` (новые методы `Notifier.NotifyCampaignContractSigned/Declined` добавляются в chunk 17 рядом с существующим `NotifyContractSent`).
     - `logger Logger` — стандарт.

   - **Бот-уведомление — Variant B**: `WebhookService` сам зовёт `notifier.NotifyCampaignContract*` **ПОСЛЕ** `dbutil.WithTx` callback'а (стандарт `backend-transactions.md`: «Логи успеха пишутся ПОСЛЕ `WithTx`»; бот-уведомление — то же самое, fire-and-forget). Симметрия с `sender_service.go` (там Phase 3 finalize → notify после Tx).

   - **2-step lookup и idempotency — обе защиты (defense in depth):**
     1. **Шаг 1 (внутри Tx)**: `SELECT id, subject_kind, trustme_status_code FROM contracts WHERE trustme_document_id = $1 FOR UPDATE` — блокирует contracts-row на длительность Tx. Защищает от concurrent webhook'ов на ту же row (TrustMe в теории не дедуплицирует ретраи). 0 рядов → `CodeContractWebhookUnknownDocument`.
     2. **Шаг 2 (внутри Tx, dispatcher)**: `switch contract.SubjectKind { case ContractSubjectKindCampaignCreator: ... default: return CodeContractWebhookUnknownSubject }`. В branch `'campaign_creator'`: `SELECT cc.id AS cc_id, cr.telegram_user_id FROM campaign_creators cc JOIN creators cr ON cr.id = cc.creator_id WHERE cc.contract_id = $1`. 0 рядов в этом запросе — это нарушение data integrity (FK contracts ↔ campaign_creators существует) → возвращаем `CodeContractWebhookUnknownSubject` + warning-лог.
     3. **Idempotency-guard**: UPDATE'ы строятся с `WHERE trustme_status_code != $newStatus` (поверх FOR UPDATE). Если `n_affected == 0` — повтор того же события, COMMIT с пустым результатом, NO state-transition, NO audit, NO notify. Сервис возвращает `nil` → handler 200.
     4. **Defense in depth обоснование**: `FOR UPDATE` гарантирует serializability concurrent-handler'ов; `WHERE != $new` гарантирует idempotency однократного применения. Двойная защита нужна, потому что TrustMe может ретраить webhook произвольно (network таймауты, перезапуск их сервиса).

   - **State-transition реализация**: приватный метод `(s *WebhookService) applyCampaignCreatorTransition(ctx, tx, contractID, ccID string, newStatus int) (notifyKind NotifyKind, err error)`. Возвращает enum `NotifyKind` (`NotifyKindNone`/`Signed`/`Declined`), который наружу решает, что слать после COMMIT.

5. **State-transition matrix per `status` 0..9.**

   Всё ниже — внутри одной `dbutil.WithTx`. UPDATE на `contracts` всегда сочетает `SET trustme_status_code = $new, webhook_received_at = now(), updated_at = now()` с двойным guard'ом:

   ```sql
   UPDATE contracts
   SET trustme_status_code = $new, webhook_received_at = now(), updated_at = now()
        [ , signed_at = now() OR declined_at = now() — для status=3/9 ]
   WHERE id = $contract_id
     AND trustme_status_code != $new           -- idempotency
     AND trustme_status_code NOT IN (3, 9)     -- terminal-guard
   ```

   - **idempotency-guard** (`!= $new`): повтор того же события → 0 affected → NO-OP (no audit, no cc.status update, no notify, no `webhook_received_at` update).
   - **terminal-guard** (`NOT IN (3, 9)`): защита от reorder'а webhook'ов и от попыток откатить терминальные состояния. Если в БД уже `signed`/`signing_declined`, любой пришедший webhook с другим status → 0 affected → NO-OP. Парный intent #14 явно требует: «finalize'нные `signed/signing_declined` не возвращаются назад». Реальный сценарий: TrustMe ретранслирует stale webhook со status=2 (промежуточный «подписал клиент»), а у нас уже status=3 — игнорируем без ошибки. Логируем info-level `stale_webhook_after_terminal`, не warning.

   | `status` | + к UPDATE `contracts`     | UPDATE `campaign_creators`                                       | Audit action                                          | Notify       |
   |----------|----------------------------|------------------------------------------------------------------|-------------------------------------------------------|--------------|
   | 0        | —                          | —                                                                | `contract_unexpected_status` (info-level лог)         | —            |
   | 1        | —                          | —                                                                | `contract_unexpected_status` (warn-level лог)         | —            |
   | 2        | —                          | —                                                                | `contract_unexpected_status` (info — intermediate ok) | —            |
   | **3**    | `signed_at = now()`        | `status = 'signed'`, `updated_at = now()`                        | `contract_signed`                                     | **Signed**   |
   | 4        | —                          | —                                                                | `contract_unexpected_status` (warn)                   | —            |
   | 5        | —                          | —                                                                | `contract_unexpected_status` (warn)                   | —            |
   | 6        | —                          | —                                                                | `contract_unexpected_status` (warn)                   | —            |
   | 7        | —                          | —                                                                | `contract_unexpected_status` (warn)                   | —            |
   | 8        | —                          | —                                                                | `contract_unexpected_status` (warn)                   | —            |
   | **9**    | `declined_at = now()`      | `status = 'signing_declined'`, `updated_at = now()`              | `contract_signing_declined`                           | **Declined** |
   | other    | —                          | —                                                                | — (handler возвращает 422 `CodeContractWebhookInvalidStatus`) | —    |

   Логика level'а лога: 0/2 — info (это нормальные intermediate состояния от TrustMe — «не подписан» сразу после загрузки и «подписан клиентом» до auto-sign); 1/4–8 — warning, не ожидаем такие сценарии в EFW pilot.

   **Audit-action constants** (добавляются в `backend/internal/service/audit_constants.go` после `AuditActionCampaignCreatorContractInitiated`, dot-стиль уже введён в chunk 16):

   ```go
   AuditActionCampaignCreatorContractSigned             = "campaign_creator.contract_signed"
   AuditActionCampaignCreatorContractSigningDeclined    = "campaign_creator.contract_signing_declined"
   AuditActionCampaignCreatorContractUnexpectedStatus   = "campaign_creator.contract_unexpected_status"
   ```

   **Audit row**:
   - `actor_id = NULL` (system actor — webhook от TrustMe).
   - `entity_type = AuditEntityTypeCampaignCreator` (существующая константа).
   - `entity_id = campaign_creators.id` (не `contracts.id` — тот через FK находим, но внешнее отображение в UI идёт по `cc.id`).
   - `payload` (UUID-only per security.md):
     ```json
     { "contract_id": "<our contracts.id UUID>", "trustme_status_code_old": 0, "trustme_status_code_new": 3 }
     ```
     Без PII (без phone, ФИО, ИИН, без `webhook.client`, без `webhook.contract_url`).

6. **Edge cases.**

   - **Soft-deleted кампания + webhook 3/9.** Lookup всё равно находит `cc.contract_id` (FK не каскадится по `is_deleted`). State-transition + audit делаем как обычно — это factual record, договор реально подписан/отказан со стороны TrustMe, наша БД должна это отражать. **Notify пропускаем** + warning-лог (`webhook_for_deleted_campaign` через `logger.Warn`, не audit-action). Реализация: JOIN с `campaigns` в шаге 2 lookup'а, флаг `c.IsDeleted` влияет только на `notifyKind = NotifyKindNone` после Tx.

   - **Race outbox vs webhook (Phase 2c done, Phase 3 not done).** TrustMe в этом случае знает наш `document_id`, но в нашей БД `contracts.trustme_document_id IS NULL` ещё не записан. Webhook прилетает → lookup `WHERE trustme_document_id = $1` → 0 рядов → `CodeContractWebhookUnknownDocument` → 404. TrustMe ретраит. К моменту retry Phase 3 finalize прошёл (или не прошёл, и тогда Phase 0 recovery подхватит orphan'а на следующем тике cron). Либо случай разрешается естественно, либо webhook будет отвечать 404 пока orphan не закроется → ОК, не баг.

   - **Race outbox 2c upload без 3 finalize (orphan).** Phase 0 recovery находит orphan через `search/Contracts` (по `additionalInfo`) → `UpdateAfterSend` → `trustme_document_id` записан. Следующий webhook-retry → lookup OK → 200. Hairy но рабочий поток.

   - **Idempotent повтор того же status.** `WHERE trustme_status_code != $new` → 0 affected → COMMIT с пустым результатом. Сервис возвращает `nil` → 200. Audit не пишется, notify не отправляется. Ровно то поведение, которое нужно для retry-friendly receiver'а.

   - **Unknown subject_kind.** Lookup-шаг 1 нашёл contract, но `subject_kind != 'campaign_creator'` (миграция расширила discriminator под `brand_agreement` без обновления handler'а). Возврат `CodeContractWebhookUnknownSubject` → 422. Эта ситуация — баг логики, не нормальный flow; warning-лог.

7. **Тесты — три уровня.**

   - **Unit `WebhookService`** (`backend/internal/contract/webhook_service_test.go`):
     - `t.Run` per status 0..9 (success, audit, notify-kind), плюс separate run-cases: idempotent повтор → 0 affected → no-op; terminal-guard (БД уже status=3, прилетает 2 → no-op); unknown-document → `CodeContractWebhookUnknownDocument`; unknown-subject_kind → `CodeContractWebhookUnknownSubject`; soft-deleted кампания + status=3 → state-transition есть, NotifyKind=None.
     - Моки: `WebhookRepoFactory` через mockery (новый интерфейс), `WebhookNotifier` через mockery, `dbutil.Pool` уже мокается через существующий паттерн.
     - Captured-input через `mock.Run(...)`: проверка что `notifier.NotifyCampaignContractSigned/Declined` зовётся с правильным `chatID` и НЕ зовётся при soft-deleted.
     - Coverage gate ≥80% per public+private function (per `backend-testing-unit.md`).

   - **Unit Handler** (`backend/internal/handler/trustme_webhook_test.go`):
     - 200 на success.
     - 200 на idempotent повтор (service вернул `nil`, n_affected внутри сервиса = 0).
     - 401 на missing `Authorization`; 401 на wrong token; anti-fingerprinting (`backend-testing-unit.md` + `security.md`) — оба сценария один и тот же `error.Message`.
     - 404 на unknown contract_id (service вернул `CodeContractWebhookUnknownDocument`).
     - 422 на status вне 0..9 (валидация в OpenAPI / handler до сервиса) и на unknown subject_kind.
     - Через ServerInterfaceWrapper из codegen, request body — типизированная структура из `api/`, не сырой JSON.

   - **Backend E2E** (`backend/e2e/contract/webhook_test.go` — отдельный файл рядом с `contract_test.go` chunk 16):
     - Header — русский нарратив (`backend-testing-e2e.md`).
     - `t.Parallel()` на каждом `Test*`.
     - Сценарии: signed(3) → `cc.status='signed'` + audit `contract_signed` через `testutil.AssertAuditEntry` + бот-сообщение (через telegram SpyOnly); declined(9) → `cc.status='signing_declined'` + audit + бот; unexpected(1/4–8) → `contracts.trustme_status_code` обновлён, `cc.status` не тронут, audit `contract_unexpected_status`; idempotent повтор → 200, audit-row count не меняется; terminal-guard (после signed прилетает status=2) → 200, состояние не меняется; unknown contract_id → 404; bad/missing token → 401; soft-deleted кампания → state-transition + audit, бот не отправлен (spy-list не растёт).
     - Триггер — **прямой HTTP** через `testutil.PostTrustMeWebhook(t, body, token)` (Variant A, см. Decision #8 ниже).

8. **E2E-триггер — Variant A (прямой HTTP).**

   `testutil.PostTrustMeWebhook(t *testing.T, payload TrustMeWebhookPayload, token string) *http.Response` — обычный HTTP-запрос с `Authorization: Bearer <token>` header'ом на `${API_URL}/trustme/webhook`. Токен берётся из конфига test-окружения (`TRUSTME_WEBHOOK_TOKEN` уставлен в docker-compose / e2e env). Не добавляем `/test/trustme/webhook-fire` test-API endpoint — webhook публичный по дизайну, прямое тестирование точнее воспроизводит реальный поток. Меньше surface для maintenance.

9. **Конфиг — `TrustMeWebhookToken`.**

   Добавляется в `backend/internal/config/config.go` рядом с `TrustMeBaseURL`/`TrustMeToken`/`TrustMeMock`:
   ```go
   TrustMeWebhookToken string `env:"TRUSTME_WEBHOOK_TOKEN,required"`
   ```
   `required:"true"` (стандарт `caarlos0/env`) — отсутствие env при `production`/`staging` → fatal-fail на startup (security.md fail-loud). Локально / в CI — задаётся в `.env.test` либо docker-compose env. Тот же токен в проде регистрируется в TrustMe через `SetHook` (runbook, не код).

10. **Code Map (для предстоящей spec'а через bmad-quick-dev).**

    **Создаём:**
    - `backend/internal/contract/webhook_service.go` (+ `_test.go`) — `WebhookService.HandleEvent`, приватный `applyCampaignCreatorTransition`, `WebhookRepoFactory`/`WebhookNotifier` интерфейсы, `NotifyKind` enum.
    - `backend/internal/handler/trustme_webhook.go` (+ `_test.go`) — handler, маппинг ошибок service → HTTP code, auth-проверка через `subtle.ConstantTimeCompare`.
    - `backend/internal/domain/trustme_webhook.go` (+ `_test.go`) — `type TrustMeWebhookEvent struct { ContractID string; Status int; Client string; ContractURL string }` + конструктор-валидатор `NewTrustMeWebhookEvent(...)` который ловит status вне 0..9 и пустой `contract_id`.
    - `backend/e2e/contract/webhook_test.go` — `TestTrustMeWebhook` со всеми сценариями.

    **Изменяем:**
    - `backend/api/openapi.yaml` — `POST /trustme/webhook` (public, request body schema, responses 200/401/404/422). Включить `description` каждого поля payload (clarity per Decision #2).
    - `backend/internal/config/config.go` — `TrustMeWebhookToken` (Decision #9).
    - `backend/internal/repository/contracts.go` — godoc-блок над `ContractColumnTrustMe*` константами и над `ContractRow.TrustMeDocumentID/ShortURL/StatusCode` (просьба Алихана; параметр chunk 17, потому что после merge ветки 16). Документирует, что один и тот же TrustMe-side ID шлётся под разными именами в их API.
    - `backend/internal/service/audit_constants.go` — добавить 3 константы (Decision #5).
    - `backend/internal/telegram/notifier.go` — методы `Notifier.NotifyCampaignContractSigned(ctx, chatID int64)` и `NotifyCampaignContractDeclined(ctx, chatID int64)` + константа `campaignContractDeclinedText` + getter `CampaignContractDeclinedText()` для тестов.
    - `backend/cmd/api/main.go` — wire `WebhookService`: после `setupTelegram` создаём `webhookSvc := contract.NewWebhookService(...)`, прокидываем в handler-bootstrap.
    - `backend/internal/handler/response.go` — error mapping для 3 новых codes (если общий dispatcher не покрывает).

    **Reference (читать, не править):**
    - `intent-trustme-contract-v2.md` — все архитектурные решения Группы 7.
    - `spec-16-trustme-outbox-worker.md` — что сделал параллельный агент.
    - `internal/contract/sender_service.go` — образец RepoFactory-узкого-интерфейса, audit-вызовов внутри Tx, notify после Tx.
    - `internal/dbutil/db.go:149` — `WithTx` signature.
    - `docs/external/trustme/blueprint.apib` (строки 1020–1117) — webhook spec и пример payload.

11. **Миграции — НЕТ.**

    Все нужные колонки и checks уже в chunk 16 миграции `20260509064702_chunk16_trustme_outbox.sql`:
    - `contracts.trustme_status_code INT NOT NULL DEFAULT 0`
    - `contracts.signed_at TIMESTAMPTZ`, `declined_at TIMESTAMPTZ`, `webhook_received_at TIMESTAMPTZ`
    - `contracts.signed_pdf_content BYTEA` (поле есть, но в chunk 17 не наполняем — отложено)
    - `CONSTRAINT contracts_trustme_status_code_range CHECK (trustme_status_code BETWEEN 0 AND 9)`
    - `campaign_creators_status_check` со статусами `signing/signed/signing_declined`

    Это снимает риск migration-conflicts при merge с веткой chunk 16.

## Открытые развилки

На текущем этапе intent крупных структурных развилок не остаётся. Финализируется в spec через bmad-quick-dev:

- Точные имена полей в OpenAPI schema (`description` каждого поля payload — формулировка для clarity).
- Точная сигнатура `testutil.PostTrustMeWebhook(...)` хелпера + где он живёт (`backend/e2e/testutil/trustme.go` логично, рядом с существующими).
- Текст precision: «`stale_webhook_after_terminal`» как структурированное log-event поле или просто message?

## Ссылки

- Roadmap: `_bmad-output/planning-artifacts/campaign-roadmap.md`, chunk 17.
- Parent intent (Группа 7): `_bmad-output/implementation-artifacts/intent-trustme-contract-v2.md`.
- Chunk 16 spec (контекст pre-req): `_bmad-output/implementation-artifacts/spec-16-trustme-outbox-worker.md`.
- TrustMe blueprint: `docs/external/trustme/blueprint.apib`.
- Существующие бот-тексты: `backend/internal/telegram/notifier.go:104-132`.
- Стандарты: `docs/standards/`.
