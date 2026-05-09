---
title: 'Чанк 17 — приём webhook от TrustMe'
type: feature
created: '2026-05-09'
status: in-review
baseline_commit: 080617aa8802e405138f378b2fd13bbc11615b0d
context:
  - docs/standards/backend-transactions.md
  - docs/standards/backend-errors.md
  - docs/standards/security.md
---

> **Преамбула — стандарты обязательны.** Перед любой строкой production-кода агент полностью загружает `docs/standards/`. Все правила — hard rules. `context` — критичные слои; остальные читаются по релевантности. Pre-req chunk 16 уже в main (PR #93 merged `345a6aa`).

<frozen-after-approval reason="human-owned intent — do not modify unless human renegotiates">

## Intent

**Problem:** Когда TrustMe меняет статус документа (креатор подписал/отказался), нам нужно перевести `campaign_creators.status` в терминал (`signed`/`signing_declined`) и уведомить креатора. Без приёма webhook'а статусы навсегда замораживаются в `signing`.

**Approach:** Public endpoint `POST /trustme/webhook` со статичным bearer-токеном в `Authorization` header (как требует TrustMe blueprint). Service `WebhookService.HandleEvent` идемпотентно (двойной guard: `WHERE trustme_status_code != $new AND NOT IN (3,9)`) обновляет `contracts` + при `status=3/9` мутирует `campaign_creators` и пишет audit, всё в одной Tx. Бот-уведомление — после COMMIT, fire-and-forget. Скачивание signed PDF — out of scope (отложено).

## Boundaries & Constraints

**Always:**
- Auth через статичный токен в `Authorization: <token>` (raw, без `Bearer`-префикса) — TrustMe blueprint § «Установка хуков» жёстко прописывает этот формат, кастомный header невозможен. Сравнение через `subtle.ConstantTimeCompare` со значением `cfg.TrustMeWebhookToken`. Anti-fingerprinting: одинаковый `error.Message` для missing и wrong token.
- Idempotency: UPDATE `contracts` с `WHERE trustme_status_code != $new` — повтор того же события → 0 affected → no-op (no audit, no cc.status update, no notify, no `webhook_received_at` update).
- Terminal-guard: UPDATE `contracts` с `WHERE trustme_status_code NOT IN (3, 9)` — после `signed`/`signing_declined` любой stale-webhook с другим статусом игнорируется (info-log `stale_webhook_after_terminal`).
- Audit ВНУТРИ Tx (стандарт `backend-transactions.md`); бот-уведомление service шлёт сам ПОСЛЕ Tx (Variant B, симметрия с `internal/contract/sender_service.go`).
- 2-step lookup ВНУТРИ Tx: (1) `SELECT id, subject_kind, trustme_status_code FROM contracts WHERE trustme_document_id = $1 FOR UPDATE`, (2) dispatcher по `subject_kind` с `JOIN campaign_creators + creators + campaigns` для `'campaign_creator'` (берём `cc.id`, `cr.telegram_user_id`, `c.is_deleted`).
- PII не пишем в stdout-логи и `error.Message` (security.md). Логируем UUID'ы, `trustme_status_code_old/new`, HTTP-метаданные. Webhook payload `client` (телефон) и `contract_url` — игнорируем полностью, в БД не пишем.
- Schema payload в OpenAPI повторяет wire-формат TrustMe (имена `contract_id`/`status`/`client`/`contract_url`). В `description` каждого поля — пояснение реального смысла. В Go-handler сразу мапим `trustmeDocumentID := req.ContractID` для clarity.
- Soft-deleted кампания: state-transition + audit делаем (factual record), бот-уведомление **пропускаем** + `logger.Warn("webhook_for_deleted_campaign")`.

**Ask First:**
- Если в ходе реализации обнаружится несовместимость генератора `oapi-codegen` (`security: []` + custom middleware-bearer не работает как у SendPulse) — HALT, обсудить альтернативу.

**Never:**
- НЕ скачивать signed PDF через `DownloadContractFile` (отложено на отдельный mini-PR; колонка `contracts.signed_pdf_content` остаётся `NULL` после chunk 17).
- НЕ создавать миграции (все нужные колонки/checks уже в `20260509064702_chunk16_trustme_outbox.sql` от chunk 16).
- НЕ писать PII в audit_logs payload и stdout-логи (`client`/телефон/ФИО/ИИН/PDF-байты — запрещено).
- НЕ откатывать терминальные `signed`/`signing_declined` — terminal-guard в SQL.
- НЕ изменять рантайм-логику chunk 16 (sender_service phases, outbox-крон, миграции) — только аддитивные изменения и godoc-блок над `ContractColumnTrustMe*`.
- НЕ использовать `Bearer`-префикс в auth-сравнении.
- НЕ записывать `webhook.contract_url` в `contracts.trustme_short_url` (он уже записан в Phase 3 outbox; перезапись избыточна).
- НЕ коммитить и не мержить автоматически (`feedback_no_commits` / `feedback_no_merge`).

## I/O & Edge-Case Matrix

| Scenario | Input / State | Expected Output / Behavior | Error Handling |
|----------|--------------|---------------------------|----------------|
| Happy signed | Webhook `{contract_id, status:3}`, в БД `cc.status='signing'`, `c.is_deleted=false` | Tx: `contracts.signed_at=now()`, `trustme_status_code=3`, `webhook_received_at=now()`; `cc.status='signed'`; audit `campaign_creator.contract_signed`. После Tx: `Notifier.NotifyCampaignContractSigned(ctx, telegramUserID)`. Response 200 `{}`. | N/A |
| Happy declined | `{contract_id, status:9}`, `cc.status='signing'` | Tx: `declined_at=now()`, `trustme_status_code=9`; `cc.status='signing_declined'`; audit `contract_signing_declined`. После Tx: `NotifyCampaignContractDeclined`. 200. | N/A |
| Unexpected status (1/4–8) | `{contract_id, status:N}`, N ∈ {1,4,5,6,7,8} | Tx: только `contracts.trustme_status_code=N`+`webhook_received_at=now()`; `cc.status` не тронут; audit `contract_unexpected_status` (warn-level лог). 200. | N/A |
| Intermediate (0/2) | `{status:0}` или `{status:2}`, `cc.status='signing'` | Tx: `trustme_status_code=N`; `cc.status` не тронут; audit `contract_unexpected_status` (info-level лог). 200. | N/A |
| Idempotent повтор | Тот же status, `contracts.trustme_status_code` уже = $new | Lookup ОК, FOR UPDATE заблокировал. UPDATE `WHERE != $new` → 0 affected → COMMIT с пустым результатом. NO-OP по всему конвейеру. 200. | N/A |
| Terminal-guard | В БД `trustme_status_code=3` (или 9), прилетает `{status:2}` (stale reorder) | UPDATE `WHERE NOT IN (3,9)` → 0 affected → no-op. `logger.Info("stale_webhook_after_terminal")`. 200. | N/A |
| Soft-deleted campaign | `c.is_deleted=true`, `{status:3}` | Tx: всё как Happy signed (factual record). После Tx: `notifyKind=NotifyKindNone` + `logger.Warn("webhook_for_deleted_campaign")`. 200. | N/A |
| Unknown document | `contract_id` не существует в БД | Lookup-шаг 1 → 0 рядов | 404 `CodeContractWebhookUnknownDocument`. Anti-fingerprint: одинаковая `error.Message`. |
| Unknown subject_kind | `contracts.subject_kind != 'campaign_creator'` | Default branch dispatcher | 422 `CodeContractWebhookUnknownSubject` + warning-лог. |
| Invalid status | `status` вне 0..9 (миновал OpenAPI validation, например, через прямой curl) | service возвращает domain-error | 422 `CodeContractWebhookInvalidStatus`. |
| Missing/wrong token | Отсутствует/неверный `Authorization` header | Middleware `TrustMeWebhookAuth` → 401 до handler'а | 401, body пустой `{}`, anti-fingerprint один message. |
| Race outbox | Webhook прилетел до Phase 3 finalize (нет `trustme_document_id` в БД) | Lookup → 0 → 404. TrustMe ретраит → к ретраю Phase 3 закоммичен → 200. | Нормальное поведение, не баг. |

</frozen-after-approval>

## Code Map

**Создаём:**
- `backend/internal/contract/webhook_service.go` — `WebhookService` struct + `HandleEvent(ctx, payload domain.TrustMeWebhookEvent) error` + приватный `applyCampaignCreatorTransition(ctx, tx, contractID, ccID string, currentStatus, newStatus int) (NotifyKind, error)`. Иммутабельный конструктор `NewWebhookService(pool dbutil.Pool, repoFactory WebhookRepoFactory, notifier WebhookNotifier, logger logger.Logger, now func() time.Time) *WebhookService`. Узкие интерфейсы:
  - `WebhookRepoFactory`: `NewContractsRepo`, `NewCampaignCreatorRepo`, `NewCreatorRepo`, `NewAuditRepo` (подмножество `repository.RepoFactory`).
  - `WebhookNotifier`: `NotifyCampaignContractSigned(ctx, chatID int64)`, `NotifyCampaignContractDeclined(ctx, chatID int64)`.
  - Локальный `NotifyKind` enum: `NotifyKindNone`/`NotifyKindSigned`/`NotifyKindDeclined`.
- `backend/internal/contract/webhook_service_test.go` — pgxmock + mockery моки, `t.Parallel()` на `Test*` и `t.Run`. Сценарии per status 0..9, idempotency, terminal-guard, soft-deleted (NotifyKind=None), unknown-document, unknown-subject_kind. Captured-input через `mock.Run(...)` для notify-проверок. Coverage gate ≥80% per public+private function.
- `backend/internal/handler/trustme_webhook.go` — `(s *Server) TrustMeWebhook(ctx, request) (response, error)` через strict-server. Конвертит api-type в `domain.TrustMeWebhookEvent`, зовёт `webhookService.HandleEvent`. Auth выполняется в **middleware** (см. ниже), handler сам токен не проверяет.
- `backend/internal/handler/trustme_webhook_test.go` — handler-тесты через ServerInterfaceWrapper, mock service. 200 (success / idempotent), 401 (через middleware), 404 (unknown doc), 422 (unknown subject / invalid status). Captured-input проверка маппинга `req.ContractID → trustmeDocumentID`.
- `backend/internal/middleware/trustme_webhook_auth.go` — `TrustMeWebhookAuth(token string) func(http.Handler) http.Handler`. Извлекает `Authorization` header, `subtle.ConstantTimeCompare`. Failure → 401 `application/json {}` (anti-fingerprint). Образец — `internal/middleware/sendpulse_auth.go`.
- `backend/internal/middleware/trustme_webhook_auth_test.go` — happy/missing/wrong/extra-bytes сценарии.
- `backend/internal/domain/trustme_webhook.go` — `type TrustMeWebhookEvent struct { ContractID string; Status int; Client string; ContractURL string }` + `NewTrustMeWebhookEvent(api.TrustMeWebhookRequest) (TrustMeWebhookEvent, error)` (валидирует пустой `contract_id` и `status` ∈ 0..9). Sentinel-ошибки: `ErrContractWebhookUnknownDocument`, `ErrContractWebhookUnknownSubject`, `ErrContractWebhookInvalidStatus`. Коды: `CodeContractWebhookUnknownDocument = "CONTRACT_WEBHOOK_UNKNOWN_DOCUMENT"`, `CodeContractWebhookUnknownSubject = "CONTRACT_WEBHOOK_UNKNOWN_SUBJECT"`, `CodeContractWebhookInvalidStatus = "CONTRACT_WEBHOOK_INVALID_STATUS"`.
- `backend/internal/domain/trustme_webhook_test.go` — table-driven для `NewTrustMeWebhookEvent` (валидные/невалидные).
- `backend/e2e/contract/webhook_test.go` — `TestTrustMeWebhook` с сценариями Happy signed / Happy declined / Unexpected status (один из 1/4-8) / Idempotent повтор / Terminal-guard (signed → status=2 → no-op) / Unknown document / Bad token / Soft-deleted. Русский нарратив-header. `t.Parallel()` на `Test*`. Прямой HTTP через `testutil.PostTrustMeWebhook(t, body, token)`. Audit через `testutil.AssertAuditEntry`. Telegram через существующий SpyOnly + `testutil.TelegramSpyList`. Использовать `testutil.SetupCampaignWithSigningCreator` (новый helper в `testutil/campaign_creator.go`, готовит `cc.status='signing'` + `contract_id` через прогон outbox-worker'а или прямой `/test/trustme/run-outbox-once` после `agree`).
- `backend/e2e/testutil/trustme_webhook.go` — `PostTrustMeWebhook(t, body, token)`, `TrustMeWebhookSignedPayload(contractID)`, etc. Тип-хелперы payload-конструкторов.

**Изменяем:**
- `backend/api/openapi.yaml` — добавить `POST /trustme/webhook` (`tags: [webhooks]`, `security: []`, requestBody `TrustMeWebhookRequest`, responses 200/401/404/422). Payload schema:
  ```yaml
  TrustMeWebhookRequest:
    type: object
    required: [contract_id, status]
    properties:
      contract_id: { type: string, minLength: 1, description: "TrustMe-side document identifier (в их API называется по-разному: document_id в SendToSign response, id в /search/Contracts, contract_id здесь). У нас в БД это contracts.trustme_document_id." }
      status: { type: integer, minimum: 0, maximum: 9, description: "TrustMe document status code per blueprint § Получить статус документа." }
      client: { type: string, description: "Phone of the actor that triggered the change. Игнорируем — PII не пишем." }
      contract_url: { type: string, description: "Full URL to the document. Игнорируем — детерминирован от contract_id, уже в trustme_short_url." }
  ```
  Описание endpoint'а явно говорит об anti-fingerprint 401 (как у SendPulse). Регенерация через `make generate-api`.
- `backend/internal/config/config.go` — добавить поле `TrustMeWebhookToken string` `env:"TRUSTME_WEBHOOK_TOKEN" envDefault:""` рядом с `TrustMeToken`. В `Load()` после environment-switch добавить проверку: «если `Environment != EnvLocal` и `TrustMeWebhookToken == ""` → return error» (по аналогии с `TelegramBotToken` гардом). Локально допустим пустой (test-API триггер не использует).
- `backend/internal/repository/contracts.go` — godoc-блок над константами `ContractColumnTrustMeDocumentID/ShortURL/StatusCode` и над полями `ContractRow.TrustMeDocumentID/ShortURL/StatusCode`. Текст: «trustme_document_id — TrustMe-side document identifier. TrustMe непоследовательны в нейминге: `document_id` в response отправки (`/SendToSignBase64FileExt`), `id` в `/search/Contracts`, `contract_id` в webhook payload. У нас всегда `trustme_document_id`, отличая от внутреннего `contracts.id` (UUID).» Аналогично для `trustme_short_url` (TrustMe-issued short URL `tct.kz/uploader/<id>`) и `trustme_status_code` (последний known статус документа в TrustMe, 0..9).
- `backend/internal/service/audit_constants.go` — добавить:
  ```go
  AuditActionCampaignCreatorContractSigned             = "campaign_creator.contract_signed"
  AuditActionCampaignCreatorContractSigningDeclined    = "campaign_creator.contract_signing_declined"
  AuditActionCampaignCreatorContractUnexpectedStatus   = "campaign_creator.contract_unexpected_status"
  ```
- `backend/internal/telegram/notifier.go` — добавить:
  - константа `campaignContractDeclinedText = "Поняли, в этот раз не подписываем. Если появятся другие подходящие предложения — обязательно вам напишем 💫"`.
  - функция `CampaignContractDeclinedText() string` (для тестов).
  - метод `(n *Notifier) NotifyCampaignContractSigned(ctx context.Context, chatID int64)` — `n.fire(ctx, "campaign_contract_signed", chatID, &bot.SendMessageParams{ChatID: chatID, Text: campaignContractSignedText})`.
  - метод `(n *Notifier) NotifyCampaignContractDeclined(ctx context.Context, chatID int64)` — аналогично с `campaignContractDeclinedText`.
- `backend/internal/handler/response.go` — добавить case'ы в `respondError`:
  ```go
  case errors.Is(err, domain.ErrContractWebhookUnknownDocument):
      writeError(w, r, 404, domain.CodeContractWebhookUnknownDocument, "Webhook target not found", log)
  case errors.Is(err, domain.ErrContractWebhookUnknownSubject):
      writeError(w, r, 422, domain.CodeContractWebhookUnknownSubject, "Unsupported webhook subject", log)
  case errors.Is(err, domain.ErrContractWebhookInvalidStatus):
      writeError(w, r, 422, domain.CodeContractWebhookInvalidStatus, "Invalid status code", log)
  ```
  Тексты на английском по образцу существующих generic-ответов webhook'у; PII никогда. Anti-fingerprint: 401 не идёт через `respondError` (handler никогда не вызывается на bad-token пути), middleware пишет напрямую.
- `backend/cmd/api/main.go` — после блока инициализации TrustMe (после `setupTrustMe(...)` и `contractSenderSvc`):
  - создать `webhookSvc := contract.NewWebhookService(pool, repoFactory, notifier, log, time.Now)`.
  - в роутинге после `routerCfg.Server.SendPulseInstagramWebhook` подключить `TrustMeWebhookAuth(cfg.TrustMeWebhookToken)` middleware на `/trustme/webhook` (как `middleware.SendPulseAuth` на `/webhooks/sendpulse/instagram`).
  - прокинуть `webhookSvc` в `Server` struct (рядом с `contractSenderSvc`).

**Reference (читать, не править):**
- `_bmad-output/implementation-artifacts/intent-chunk-17-trustme-webhook.md` — полный design (intent живёт до архива по `feedback_intent_file_lifecycle`).
- `_bmad-output/implementation-artifacts/intent-trustme-contract-v2.md` — design Группы 7 целиком (Decisions #7, #9, #14 — webhook).
- `backend/internal/contract/sender_service.go` — образец RepoFactory-узкого-интерфейса, audit ВНУТРИ Tx, notify ПОСЛЕ Tx, `recordAudit` с JSON-payload.
- `backend/internal/handler/webhook_sendpulse.go` + middleware — образец webhook-обработчика (`security: []`, middleware-auth).
- `backend/internal/repository/contracts.go` — `ContractRepo` интерфейс (используем `Insert` нет; нужны новые `GetByTrustMeDocumentID` и `UpdateAfterWebhook`).
- `backend/internal/repository/campaign_creator.go` — нужен `UpdateStatus(ctx, ccID, newStatus string)` для перехода `signing → signed/signing_declined`.
- `backend/internal/dbutil/db.go:149` — `WithTx` signature.
- `docs/external/trustme/blueprint.apib` (строки 1020–1117) — webhook spec и payload format.

**Миграции:** НЕТ. Все колонки и checks (`trustme_status_code BETWEEN 0 AND 9`, `cc_status_check` со статусами `signing/signed/signing_declined`, `signed_at`, `declined_at`, `webhook_received_at`) уже в `backend/migrations/20260509064702_chunk16_trustme_outbox.sql` от chunk 16.

## Tasks & Acceptance

**Execution (порядок по зависимостям):**
- [x] `backend/internal/repository/contracts.go` — godoc-блок над `ContractColumnTrustMe*` константами и `ContractRow.TrustMe*` полями (TrustMe naming inconsistency).
- [x] `backend/internal/repository/contracts.go` — добавлены `LockByTrustMeDocumentID` и `UpdateAfterWebhook` (UPDATE с двойным guard'ом, возвращает rowsAffected). Также пара констант `TrustMeStatusSigned/SigningDeclined`.
- [x] `backend/internal/repository/campaign_creator.go` — `UpdateStatus` + `GetWithCampaignAndCreatorByContractID` + новая `CampaignCreatorWebhookView`.
- [x] `backend/internal/domain/trustme_webhook.go` — `TrustMeWebhookEvent` + конструктор-валидатор + sentinel-errors + коды.
- [x] `backend/internal/domain/trustme_webhook_test.go` — table-driven тесты валидатора.
- [x] `backend/internal/service/audit_constants.go` — три новые константы.
- [x] `backend/internal/telegram/notifier.go` — `campaignContractDeclinedText` + `CampaignContractDeclinedText()` + `NotifyCampaignContractSigned` + `NotifyCampaignContractDeclined` + unit-тесты.
- [x] `backend/internal/contract/webhook_service.go` — `WebhookService.HandleEvent` + приватный `applyCampaignCreatorTransition` + интерфейсы `WebhookRepoFactory`/`WebhookNotifier` + локальный `NotifyKind` enum.
- [x] `backend/internal/contract/webhook_service_test.go` — все сценарии I/O matrix с mockery + pgxmock.
- [x] `backend/internal/middleware/trustme_webhook_auth.go` + `_test.go` — middleware (constant-time compare + anti-fingerprint 401).
- [x] `backend/api/openapi.yaml` — `POST /trustme/webhook` + `TrustMeWebhookRequest` schema + responses. `make generate-api` отработал.
- [x] `backend/internal/handler/trustme_webhook.go` + `_test.go` — handler через strict-server.
- [x] `backend/internal/handler/response.go` — три новых case в `respondError`.
- [x] `backend/internal/config/config.go` — `TrustMeWebhookToken` env var + non-local guard.
- [x] `backend/cmd/api/main.go` — wire `WebhookService` + middleware на `/trustme/webhook`.
- [x] `backend/internal/handler/server.go` — `TrustMeWebhookService` interface + расширен `NewServer` (старые callsites обновлены `nil`-аргументом).
- [x] `backend/e2e/testutil/trustme_webhook.go` + `contract_setup.go` — `PostTrustMeWebhook`, payload-конструкторы, `SetupCampaignWithSigningCreator`, `RunTrustMeOutboxOnce`, `FindTrustMeSpyByIIN`.
- [x] `backend/e2e/contract/webhook_test.go` — `TestTrustMeWebhook` со сценариями signed/declined/idempotent/terminal-guard/intermediate/unknown-doc/missing-token/wrong-token/invalid-status.
- [x] `backend/.env` — `TRUSTME_WEBHOOK_TOKEN=local-dev-trustme-webhook-token` (читается тестами через env var).
- [x] `make build-backend && make lint-backend && make test-unit-backend && make test-unit-backend-coverage && make test-e2e-backend` — все зелёные. Coverage gate ≥80% per identifier в `internal/contract/`, `internal/handler/`, `internal/middleware/`, `internal/repository/`, `internal/service/`, `internal/authz/`. E2E suite полностью passing.

**Acceptance Criteria:**
- Given `cc.status='signing'`, `contract_id` существует, валидный токен, when webhook `{contract_id, status:3}` пришёл, then в БД: `contracts.trustme_status_code=3`, `signed_at` непустой, `webhook_received_at` непустой; `cc.status='signed'`, `cc.updated_at` обновлён; audit-row `campaign_creator.contract_signed` с `actor_id=NULL`, `entity_type='campaign_creator'`, `entity_id=cc.id`, payload `{"contract_id":"<UUID>","trustme_status_code_old":0,"trustme_status_code_new":3}`. Telegram SpyOnly содержит сообщение `campaignContractSignedText` для `cr.telegram_user_id`. Response — 200 `{}`.
- Given тот же цикл, when тот же webhook пришёл повторно (status=3 уже в БД), then 0 affected — audit-row count не изменился, spy-list не вырос. Response — 200 `{}`.
- Given `contracts.trustme_status_code=3` (terminal), when прилетает `{status:2}`, then UPDATE 0 affected, info-log `stale_webhook_after_terminal`, состояние не меняется. 200.
- Given `c.is_deleted=true`, `cc.status='signing'`, when `{status:3}`, then `cc.status='signed'` + audit пишутся (factual record), warning-лог `webhook_for_deleted_campaign`, **бот-сообщение НЕ отправлено** (spy-list пуст для этого creator'а).
- Given missing `Authorization` header, when webhook вызван, then 401 `{}`, handler не вызвался (middleware вырезала), `error.Message` одинаковый для missing/wrong (anti-fingerprint).
- Given неизвестный `contract_id`, when валидный токен + payload, then 404 с кодом `CONTRACT_WEBHOOK_UNKNOWN_DOCUMENT`. Lookup-шаг 1 = 0 рядов.
- Given `status=15` (вне 0..9), when payload пришёл, then 422 `CONTRACT_WEBHOOK_INVALID_STATUS` (через OpenAPI validation либо domain-валидатор, в зависимости от того, кто первый поймает).
- `make test-unit-backend-coverage` — gate ≥80% per public+private function в `internal/contract/`, `internal/handler/`, `internal/middleware/`, `internal/domain/`.
- Race detector (`-race`) зелёный во всех тестах.

## Design Notes

**SQL UPDATE — точная форма (psevdo-Go в `UpdateAfterWebhook`):**

```go
// Build UPDATE с двойным guard'ом + opt-in signed_at/declined_at.
qb := sq.Update(TableContracts).
    Set(ContractColumnTrustMeStatusCode, newStatus).
    Set(ContractColumnWebhookReceivedAt, sq.Expr("now()")).
    Set(ContractColumnUpdatedAt, sq.Expr("now()")).
    Where(sq.Eq{ContractColumnID: contractID}).
    Where(sq.NotEq{ContractColumnTrustMeStatusCode: newStatus}).
    Where(sq.NotEq{ContractColumnTrustMeStatusCode: 3}).
    Where(sq.NotEq{ContractColumnTrustMeStatusCode: 9})
if newStatus == 3 { qb = qb.Set(ContractColumnSignedAt, sq.Expr("now()")) }
if newStatus == 9 { qb = qb.Set(ContractColumnDeclinedAt, sq.Expr("now()")) }
n, err := dbutil.Exec(ctx, db, qb)
return int(n), err  // 0 → idempotent/terminal-guard блокировал
```

**Golden flow service (упрощённо, ~10 строк):**

```go
return dbutil.WithTx(ctx, s.pool, func(tx dbutil.DB) error {
    contractsRepo := s.repoFactory.NewContractsRepo(tx)
    contractRow, err := contractsRepo.LockByTrustMeDocumentID(ctx, ev.ContractID)
    if err != nil { return mapToWebhookError(err) }  // sql.ErrNoRows → ErrContractWebhookUnknownDocument
    switch contractRow.SubjectKind {
    case repository.ContractSubjectKindCampaignCreator:
        notifyKind, err = s.applyCampaignCreatorTransition(ctx, tx, contractRow.ID, ev.Status)
        return err
    default:
        return domain.ErrContractWebhookUnknownSubject
    }
})
// после Tx:
if notifyKind == NotifyKindSigned  { s.notifier.NotifyCampaignContractSigned(ctx, telegramUserID) }
if notifyKind == NotifyKindDeclined{ s.notifier.NotifyCampaignContractDeclined(ctx, telegramUserID) }
```

**Audit payload (JSON-marshal в `applyCampaignCreatorTransition`):**

```go
payload := map[string]any{
    "contract_id":              contractRow.ID,
    "trustme_status_code_old":  contractRow.TrustMeStatusCode,
    "trustme_status_code_new":  ev.Status,
}
body, _ := json.Marshal(payload)
auditRepo.Create(ctx, repository.AuditLogRow{
    ActorID:    nil,
    ActorRole:  "system",
    Action:     auditActionForStatus(ev.Status),  // signed/signing_declined/unexpected_status
    EntityType: "campaign_creator",
    EntityID:   &ccID,
    NewValue:   body,
})
```

**Pre-req:** chunk 16 в main (PR #93 merged `345a6aa`, плюс PR #94 с правкой telegram-текста). Реализация — на новой ветке `alikhan/chunk-17-trustme-webhook` от main.

## Verification

**Commands:**
- `make build-backend` — компилируется без warnings.
- `make lint-backend` — gofmt + golangci-lint clean.
- `make generate-api` — regenerated `internal/api/server.gen.go` без ручных правок.
- `make test-unit-backend` — все unit зелёные.
- `make test-unit-backend-coverage` — gate ≥80% per-method для `internal/contract/`, `internal/handler/`, `internal/middleware/`, `internal/domain/`.
- `make test-e2e-backend` — `TestTrustMeWebhook` зелёный со всеми сценариями.

**Manual checks:**
- На staging (`TRUSTME_MOCK=true`, `TRUSTME_WEBHOOK_TOKEN` задан) — поднять, прогнать happy-path: создать кампанию с шаблоном, добавить креатора, прогнать `agree` + `runOutboxOnce`, через `curl -X POST -H "Authorization: $TRUSTME_WEBHOOK_TOKEN" -d '{"contract_id":"<spy-doc-id>","status":3,"client":"77071234567","contract_url":"..."}'` отправить webhook. Проверить: `cc.status='signed'`, audit-row с правильным payload, бот отправил congrats-сообщение.
- Прогнать запросом с битым токеном — 401, в access-логе бэкенда видно попытку, payload не принят.
