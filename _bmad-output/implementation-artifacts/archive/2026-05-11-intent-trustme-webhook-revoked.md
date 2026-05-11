---
title: 'TrustMe webhook — обработка status=4 (Отозван компанией)'
type: bugfix
created: '2026-05-11'
status: draft
context:
  - docs/standards/backend-transactions.md
  - docs/standards/backend-errors.md
  - docs/standards/backend-testing-unit.md
  - docs/standards/backend-testing-e2e.md
---

> **Преамбула — стандарты обязательны.** Перед любой строкой production-кода агент полностью загружает `docs/standards/`. Все правила — hard rules. `context` — критичные слои; остальные читаются по релевантности.

## Intent

**Problem:** Когда менеджер отзывает договор через UI TrustMe для договора с чужим телефоном, креатор остаётся «висеть» в списке `cc.status='signing'`, уведомление не приходит. TrustMe шлёт webhook со `status=4` («Отозван компанией»), но текущий `webhook_service.applyCampaignCreatorTransition` обрабатывает как terminal только `status=3` и `status=9` — четвёрка падает в default-ветку, пишется `unexpected_status`, `cc.status` не мутирует.

**Approach:** Расширить terminal-обработку в webhook на `status=4` — точно так же как `status=9`: `cc.status='signing_declined'`, audit-action `campaign_creator.contract_signing_declined`, уведомление через `NotifyCampaignContractDeclined` (тот же текст). Соответственно расширить terminal-guard в SQL UPDATE до `NOT IN (3, 4, 9)`. Миграций нет — `signing_declined` уже в CHECK constraint от chunk 16.

## Boundaries & Constraints

**Always:**
- Идемпотентность сохраняется через существующий guard `trustme_status_code != newStatus` — повтор `status=4` даёт 0 affected, no-op.
- Terminal-guard расширяется до `NOT IN (3, 4, 9)` — после revoked любой stale-webhook с другим статусом игнорируется, `logger.Info("stale_webhook_after_terminal")` пишется.
- Audit-action **один и тот же** для status=4 и status=9 — `campaign_creator.contract_signing_declined`. В payload `trustme_status_code_new` показывает реальную разницу (4 vs 9) для постфактум-анализа.
- Текст бот-уведомления **один и тот же** для обоих кейсов (по решению Алихана 2026-05-11). Notifier: `NotifyCampaignContractDeclined` без перегрузок.
- PII в стандартных stdout-логах и `error.Message` — запрещена (security.md). В audit_logs.payload — `contract_id` и `trustme_status_code_*` только.

**Never:**
- НЕ вводить новый `cc.status` (типа `signing_revoked`) — решение Алихана: статусы не различать.
- НЕ заводить второй текст для бота — единый текст по обоим терминалам.
- НЕ делать миграцию — `signing_declined` уже разрешён CHECK constraint'ом таблицы `campaign_creators`.
- НЕ создавать ручку «отозвать договор» в нашем UI — out of scope (scope A по согласованию).
- НЕ трогать статусы 5/6/7/8 (расторжение после полного подписания) — они приходят только после `3`, текущий terminal-guard и так блокирует.
- НЕ коммитить и не мержить автоматически (`feedback_no_commits` / `feedback_no_merge`).

## I/O Matrix — обновлённые строки

| Scenario | Input / State | Expected Output / Behavior |
|----------|--------------|----------------------------|
| Happy revoked (NEW) | webhook `{contract_id, status:4}`, `cc.status='signing'`, `c.is_deleted=false` | Tx: `contracts.declined_at=now()` (NEW: пишем тот же `declined_at`), `trustme_status_code=4`, `webhook_received_at=now()`; `cc.status='signing_declined'`; audit `campaign_creator.contract_signing_declined` payload `{contract_id, trustme_status_code_old:0, trustme_status_code_new:4}`. После Tx: `NotifyCampaignContractDeclined`. 200. |
| Unexpected status (UPDATED) | `{status:N}`, N ∈ {1,5,6,7,8} | Без изменений — `trustme_status_code=N`, `cc.status` не тронут, audit `contract_unexpected_status` (warn-log). 200. **Статус 4 убран из этой строки.** |
| Terminal-guard revoked (NEW) | В БД `trustme_status_code=4`, прилетает `{status:9}` (или `{status:3}`) | UPDATE `WHERE NOT IN (3,4,9)` → 0 affected → no-op. `logger.Info("stale_webhook_after_terminal")`. 200. |
| Idempotent revoked (NEW) | В БД `trustme_status_code=4`, прилетает `{status:4}` | guard `!= newStatus` → 0 affected → NO-OP по всему конвейеру. 200. |

Остальные строки матрицы (signed/declined/idempotent signed/declined/intermediate/soft-deleted/unknown-doc/unknown-subject/missing-token и т.п.) — без изменений, см. `spec-17-trustme-webhook.md`.

## Code Map

**Изменяем:**

- `backend/internal/repository/contracts.go`:
  - Добавить константу `TrustMeStatusRevoked = 4` рядом с существующими `TrustMeStatusSigned = 3` и `TrustMeStatusSigningDeclined = 9`.
  - В `UpdateAfterWebhook` расширить terminal-guard в `Where(sq.NotEq{ContractColumnTrustMeStatusCode: ...})` со списка `[3, 9]` до `[3, 4, 9]`.
  - В switch `case TrustMeStatusSigningDeclined` (или эквивалентном — set `declined_at`) добавить ветку `case TrustMeStatusRevoked` симметрично (тоже пишет `declined_at = now()`).
- `backend/internal/contract/webhook_service.go`:
  - В `applyCampaignCreatorTransition`, в `switch ev.Status`, добавить `case repository.TrustMeStatusRevoked:` идентично существующему `case repository.TrustMeStatusSigningDeclined` — `ccRepo.UpdateStatus(... CampaignCreatorStatusSigningDeclined)`, `actionSuffix = auditActionWebhookSigningDeclinedSuffix`, `notifyKind = NotifyKindDeclined`.
  - Альтернатива (DRY): склеить через fallthrough или общий хелпер. Принять решение по месту, опираясь на читаемость switch'а.
  - Godoc-блок над `applyCampaignCreatorTransition` обновить: «terminal status (3/4/9) → cc.status flips; intermediate (0/2) и unexpected (1/5/6/7/8) — без перехода».
- `backend/internal/contract/webhook_service_test.go`:
  - В тесте «happy declined» добавить или дублировать `t.Run("happy revoked")` со `status=4`, expected `cc.status='signing_declined'`, audit-action и notify идентичны declined.
  - Из intermediate-list `statuses := []int{0, 1, 2, 4, 5, 6, 7, 8}` (см. ~webhook_service_test.go:142) убрать `4` → останется `{0, 1, 2, 5, 6, 7, 8}`.
  - Terminal-guard test (если есть отдельный): добавить кейс `trustme_status_code=4` → прилетает `{status:9}` → 0 affected → no-op.
- `backend/internal/repository/contracts_test.go`:
  - Unit-тест `UpdateAfterWebhook` — обновить ожидание точной SQL-строки (литералы) под новый список `NOT IN (3, 4, 9)`, добавить кейс newStatus=4 → проверить `Set(declined_at, now())`.
- `backend/e2e/contract/webhook_test.go`:
  - Добавить сценарий «Happy revoked» в `TestTrustMeWebhook` — `t.Run("revoked")` симметрично «declined»: подготовить cc.status='signing' через `testutil.SetupCampaignWithSigningCreator`, отправить webhook со `status=4`, проверить `cc.status='signing_declined'`, audit-row с `action='campaign_creator.contract_signing_declined'` + `payload.trustme_status_code_new=4`, telegram-spy получил declined-сообщение.

**Reference (читать, не править):**

- `_bmad-output/implementation-artifacts/archive/spec-17-trustme-webhook.md` (или текущий путь spec-17) — оригинальный design webhook'а.
- `docs/external/trustme/blueprint.apib` строки 754–776 (статусы 0..9), 1103–1119 (формат хука), 845–909 (отзыв/расторжение — для понимания семантики 4 vs 5/8).
- `backend/internal/repository/contracts.go:405–435` — `UpdateAfterWebhook` golden-form с двойным guard'ом.
- `backend/internal/contract/webhook_service.go:154–237` — `applyCampaignCreatorTransition` golden-flow.

**Создаём:** ничего (миграций нет, новых файлов нет).

**Миграции:** НЕТ. `signing_declined` уже разрешён CHECK constraint в `20260509064702_chunk16_trustme_outbox.sql`.

## Tasks & Acceptance

**Execution (порядок по зависимостям):**

- [ ] `backend/internal/repository/contracts.go` — `TrustMeStatusRevoked=4`, расширить terminal-guard и `declined_at` set-ветку в `UpdateAfterWebhook`.
- [ ] `backend/internal/repository/contracts_test.go` — обновить SQL-литералы + добавить кейс newStatus=4.
- [ ] `backend/internal/contract/webhook_service.go` — `case TrustMeStatusRevoked` в `applyCampaignCreatorTransition`, обновлённый godoc.
- [ ] `backend/internal/contract/webhook_service_test.go` — `t.Run("happy revoked")`, выкинуть `4` из intermediate-list, обновить terminal-guard тест.
- [ ] `backend/e2e/contract/webhook_test.go` — `t.Run("revoked")` симметрично declined.
- [ ] `make build-backend && make lint-backend && make test-unit-backend && make test-unit-backend-coverage && make test-e2e-backend` — все зелёные. Coverage gate ≥80% per identifier сохраняется.

**Acceptance Criteria:**

- Given `cc.status='signing'`, `contract_id` существует, валидный TrustMe webhook-токен, when webhook `{contract_id, status:4}` пришёл, then в БД: `contracts.trustme_status_code=4`, `contracts.declined_at` непустой, `contracts.webhook_received_at` непустой; `cc.status='signing_declined'`, `cc.updated_at` обновлён; audit-row `campaign_creator.contract_signing_declined` с `actor_id=NULL`, `entity_type='campaign_creator'`, `entity_id=cc.id`, payload `{"contract_id":"<UUID>","trustme_status_code_old":0,"trustme_status_code_new":4}`. Telegram SpyOnly содержит сообщение `campaignContractDeclinedText` для `cr.telegram_user_id`. Response — 200 `{}`.
- Given тот же цикл, when webhook `{status:4}` прилетел повторно (`trustme_status_code` уже 4), then 0 affected — audit-row count не вырос, spy-list не вырос. 200.
- Given `trustme_status_code=4`, when прилетает `{status:9}` (или `{status:3}`), then UPDATE 0 affected, info-log `stale_webhook_after_terminal`, состояние не меняется. 200.
- Given `c.is_deleted=true`, `cc.status='signing'`, when `{status:4}`, then `cc.status='signing_declined'` + audit пишутся (factual record), warning-лог `webhook_for_deleted_campaign`, бот-сообщение **НЕ отправлено**.
- Given `cc.status='signing'`, when `{status:N}` для N ∈ {1,5,6,7,8}, then `trustme_status_code=N`, `cc.status` не тронут, audit `campaign_creator.contract_unexpected_status` (warn-лог). 200.
- `make test-unit-backend-coverage` — gate ≥80% per public+private function в `internal/contract/`, `internal/repository/`. Race detector (`-race`) зелёный.

## Verification

**Commands:**

- `make build-backend` — компилируется.
- `make lint-backend` — clean.
- `make test-unit-backend` — все зелёные, новые `t.Run("happy revoked")` и обновлённый intermediate-list проходят.
- `make test-unit-backend-coverage` — gate ≥80% сохраняется.
- `make test-e2e-backend` — `TestTrustMeWebhook` зелёный с новым сценарием revoked.

**Manual check (staging):**

- Поднять, прогнать happy revoked: создать кампанию с шаблоном, добавить креатора, прогнать `agree` + `runOutboxOnce`, через `curl -X POST -H "Authorization: Bearer $TRUSTME_WEBHOOK_TOKEN" -d '{"contract_id":"<spy-doc-id>","status":4,"client":"77071234567","contract_url":"..."}'` отправить webhook. Проверить: `cc.status='signing_declined'`, audit-row с правильным payload (`trustme_status_code_new:4`), бот отправил declined-сообщение, креатор пропал из фильтра «подписывает договор» в админке.
