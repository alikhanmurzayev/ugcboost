---
title: 'TrustMe webhook — обработка status=4 (Отозван компанией)'
type: bugfix
created: '2026-05-11'
status: done
baseline_commit: 236bcea80e962fcbae4fe15a186f229360b7dd33
context:
  - docs/standards/backend-transactions.md
  - docs/standards/backend-errors.md
  - docs/standards/backend-testing-unit.md
---

> **Преамбула — стандарты обязательны.** Перед любой строкой production-кода агент полностью загружает `docs/standards/`. Все правила — hard rules.

<frozen-after-approval>

## Intent

**Problem:** Менеджер отзывает чужой договор через UI Trust.me — TrustMe шлёт webhook `{status:4}` («Отозван компанией»). `webhook_service.applyCampaignCreatorTransition` сейчас обрабатывает как terminal только `3`/`9`, четвёрка падает в default-ветку `unexpected_status`. `cc.status` остаётся `'signing'`, креатор «висит» в списке подписания, бот молчит.

**Approach:** Расширить terminal-обработку на `status=4` симметрично `status=9`: `cc.status='signing_declined'`, audit-action `campaign_creator.contract_signing_declined`, `NotifyCampaignContractDeclined` (тот же текст). Terminal-guard в SQL UPDATE расширяется до `NOT IN (3, 4, 9)`. Audit-action и текст бота — общие для 4 и 9 (различие видно в payload через `trustme_status_code_new`). Миграций нет — `signing_declined` уже разрешён CHECK от chunk 16.

## Boundaries & Constraints

**Always:**
- Audit-action и текст бота едины для status=4 и status=9.
- Идемпотентность: повтор `{status:4}` → 0 affected, no-op (no audit, no cc.status mutation, no notify).
- Terminal-guard `NOT IN (3, 4, 9)`: stale `{status:3}`/`{status:9}` после `4` → 0 affected, info-log `stale_webhook_after_terminal`.
- Soft-deleted campaign + `{status:4}` → state-transition + audit пишутся (factual record); notify пропускаем; warn-log `webhook_for_deleted_campaign`.
- PII в stdout-логах и `error.Message` запрещена. В audit-payload — только `contract_id`, `trustme_status_code_old/new`.

**Never:**
- НЕ вводить новый `cc.status` (например, `signing_revoked`) — статусы не различаем.
- НЕ заводить второй текст для бота.
- НЕ делать миграцию — CHECK от chunk 16 уже покрывает.
- НЕ создавать UI-ручку «отозвать» (scope A — webhook-only fix).
- НЕ трогать обработку статусов 1/5/6/7/8 — остаются `unexpected_status`.

## I/O & Edge-Case Matrix

| Scenario | Input / State | Expected Output / Behavior |
|----------|--------------|----------------------------|
| Happy revoked | `{status:4}`, `cc.status='signing'`, `c.is_deleted=false` | `contracts.trustme_status_code=4`, `declined_at=now()`, `webhook_received_at=now()`; `cc.status='signing_declined'`; audit `contract_signing_declined` с `trustme_status_code_new:4`; `NotifyCampaignContractDeclined` после Tx. 200. |
| Idempotent revoked | `{status:4}`, в БД уже `trustme_status_code=4` | 0 affected → no audit / cc.status / notify. 200. |
| Terminal-guard after revoked | В БД `trustme_status_code=4`, прилетает `{status:3}` или `{status:9}` | 0 affected, info-log `stale_webhook_after_terminal`. 200. |
| Soft-deleted + revoked | `c.is_deleted=true`, `{status:4}` | state+audit пишутся; notify НЕ шлём; warn-log. 200. |
| Other unexpected (1,5,6,7,8) | `{status:N}` | `trustme_status_code=N`, `cc.status` не тронут, audit `contract_unexpected_status`. 200. |

Сценарии signed/declined/idempotent-signed/intermediate-0-2/unknown-doc/missing-token/wrong-token/invalid-status — без изменений (см. `spec-17-trustme-webhook.md`).

</frozen-after-approval>

## Code Map

- `backend/internal/repository/contracts.go` -- константы статусов; `UpdateAfterWebhook` (terminal-guard slice, switch newStatus).
- `backend/internal/repository/contracts_test.go` -- `TestContractRepository_UpdateAfterWebhook`, SQL-литералы.
- `backend/internal/contract/webhook_service.go` -- `applyCampaignCreatorTransition.switch ev.Status`, godoc.
- `backend/internal/contract/webhook_service_test.go` -- declined-ветка, `IntermediateStatuses`, terminal-guard кейсы.
- `backend/e2e/contract/webhook_test.go` -- `TestTrustMeWebhook` (заменить `intermediate status=4` на `revoked`), нарратив-header.

## Tasks & Acceptance

**Execution:**
- [x] `backend/internal/repository/contracts.go` -- добавить `TrustMeStatusRevoked = 4`; в `UpdateAfterWebhook` slice terminal-guard → `[TrustMeStatusSigned, TrustMeStatusRevoked, TrustMeStatusSigningDeclined]`; в switch newStatus добавить `case TrustMeStatusRevoked` set `declined_at = now()`.
- [x] `backend/internal/repository/contracts_test.go` -- `contractWebhookGuardSQL` под `NOT IN ($4,$5,$6)`; во всех `WithArgs(..., 3, 9)` → `WithArgs(..., 3, 4, 9)`; новый `t.Run("status=4 stamps declined_at")` симметричный status=9.
- [x] `backend/internal/contract/webhook_service.go` -- в `applyCampaignCreatorTransition.switch ev.Status` добавить `case repository.TrustMeStatusRevoked:` идентично `case repository.TrustMeStatusSigningDeclined` (тот же `UpdateStatus(... SigningDeclined)`, тот же `auditActionWebhookSigningDeclinedSuffix`, `NotifyKindDeclined`). Godoc «terminal status (3/4/9)».
- [x] `backend/internal/contract/webhook_service_test.go` -- (а) `TestWebhookService_HandleEvent_Revoked` симметричный existing `_Declined`; (б) убрать `4` из `statuses` в `_IntermediateStatuses`; (в) добавить кейс «после revoked прилетает status=9 → no-op» в terminal-guard тест.
- [x] `backend/e2e/contract/webhook_test.go` -- переписать `t.Run("intermediate status=4 ...")` в `t.Run("revoked (status=4) flips cc to signing_declined + audit + decline bot message")` симметрично declined; обновить нарратив-header.
- [x] `make build-backend && make lint-backend && make test-unit-backend && make test-unit-backend-coverage && make test-e2e-backend` — все зелёные.

**Acceptance Criteria:**
- Given `cc.status='signing'`, валидный токен, when `{status:4}`, then `contracts.trustme_status_code=4`, `declined_at` непустой, `cc.status='signing_declined'`, audit `contract_signing_declined` с `trustme_status_code_new:4`, telegram-spy получил declined-message. 200.
- Given повтор `{status:4}`, when обработан, then 0 affected — audit-count и spy-count не выросли. 200.
- Given `trustme_status_code=4` в БД, when stale `{status:9}` или `{status:3}`, then 0 affected, info-log `stale_webhook_after_terminal`. 200.
- Given `c.is_deleted=true`, when `{status:4}`, then state+audit пишутся, notify пропущен, warn-log `webhook_for_deleted_campaign`. 200.
- Given `{status:N}` для N ∈ {1,5,6,7,8}, then `cc.status` не тронут, audit `contract_unexpected_status`. 200.
- `make test-unit-backend-coverage` — gate ≥80% per identifier; `-race` зелёный.

## Verification

**Commands:**
- `make build-backend` -- compiles clean.
- `make lint-backend` -- gofmt + golangci-lint clean.
- `make test-unit-backend && make test-unit-backend-coverage` -- зелёные, coverage gate сохранён.
- `make test-e2e-backend` -- `TestTrustMeWebhook` зелёный с новым `t.Run("revoked")`.

**Manual check (staging):**
- `curl -X POST https://api.staging.ugcboost.kz/trustme/webhook -H "Authorization: Bearer $TRUSTME_WEBHOOK_TOKEN" -d '{"contract_id":"<spy-doc-id>","status":4,"client":"77071234567","contract_url":"..."}'` → `cc.status='signing_declined'`, audit с `trustme_status_code_new:4`, креатор пропал из «подписывает договор» в админке, declined-message в telegram.

## Suggested Review Order

**Service-уровневое склеивание 4 и 9**

- Входная точка фикса: `status=4` идёт по той же ветке, что и `status=9`.
  [`webhook_service.go:203`](../../backend/internal/contract/webhook_service.go#L203)

- Stale-after-terminal info-log расширен на новый terminal-статус.
  [`webhook_service.go:168`](../../backend/internal/contract/webhook_service.go#L168)

**Repository: терминальный guard и declined_at**

- Константа нового terminal-кода для repo + сервиса.
  [`contracts.go:69`](../../backend/internal/repository/contracts.go#L69)

- Terminal-guard расширяется до `NOT IN (3, 4, 9)`; switch newStatus стампит `declined_at` для 4 и 9.
  [`contracts.go:431`](../../backend/internal/repository/contracts.go#L431)

**Unit-тесты симметричных кейсов**

- Happy revoked: status=4 → cc.status=signing_declined + declined-message.
  [`webhook_service_test.go:139`](../../backend/internal/contract/webhook_service_test.go#L139)

- Idempotent repeat покрыт для signed и revoked.
  [`webhook_service_test.go:212`](../../backend/internal/contract/webhook_service_test.go#L212)

- Terminal-guard: cross-terminal stale (revoked→9 даёт no-op).
  [`webhook_service_test.go:253`](../../backend/internal/contract/webhook_service_test.go#L253)

- Soft-deleted campaign покрыт для signed и revoked (state+audit без notify).
  [`webhook_service_test.go:304`](../../backend/internal/contract/webhook_service_test.go#L304)

- IntermediateStatuses — `4` убран из списка warn-веток.
  [`webhook_service_test.go:167`](../../backend/internal/contract/webhook_service_test.go#L167)

- Repo-уровневый SQL: `declined_at` стампится при newStatus=4, `NOT IN ($4,$5,$6)`.
  [`contracts_test.go:462`](../../backend/internal/repository/contracts_test.go#L462)

**E2E**

- `intermediate status=4` переписан в `revoked (status=4)` с проверкой cc.status + audit + telegram declined-message.
  [`webhook_test.go:173`](../../backend/e2e/contract/webhook_test.go#L173)

- Header-нарратив: status=4 теперь terminal, intermediate/unexpected-список без `4`.
  [`webhook_test.go:11`](../../backend/e2e/contract/webhook_test.go#L11)
