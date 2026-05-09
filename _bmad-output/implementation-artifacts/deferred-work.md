# Deferred work

Findings из review-этапов bmad-quick-dev, классифицированные как `defer` —
не покрытые текущим scope'ом, требуют отдельного дизайн-обсуждения или
hardening-PR. Перед началом нового PR — пересмотреть и снять закрытые.

## Из ревью chunk 17 (TrustMe webhook receiver, 2026-05-09)

### Monotonic guard в `contracts.UpdateAfterWebhook`
**Источник:** Edge case hunter (E-2).
**Суть:** WHERE-clause использует `!= newStatus` (idempotency) + `NOT IN (3,9)` (terminal). Если в БД `trustme_status_code=5` и прилетит out-of-order webhook со `status=0`, ряд silently regress'ит 5→0 + audit `unexpected_status`. TrustMe webhook ordering не гарантирован.
**Risk:** observability искажается — local view несинхронен с TrustMe-side.
**Возможный fix:** добавить `WHERE trustme_status_code < $newStatus` или явные правила переходов (state machine). Требует обсуждения с TrustMe support — какие переходы действительно возможны.

### Default `status=0` accepted при отсутствующем поле
**Источник:** Edge case hunter (E-3).
**Суть:** strict-server / oapi-codegen не enforce'ят `required: [status]` runtime'ом. Payload `{"contract_id":"x"}` декодится в `Status: 0`, проходит валидацию (0 — валидное значение), и может regress'ить ряд при наличии non-zero status'а в БД.
**Risk:** малой — TrustMe blueprint не пропускает payload без status. Но defensive validation hardening полезен.
**Возможный fix:** перейти на `*int` для status в openapi schema + nil-check в `NewTrustMeWebhookEvent`.

### `cc.UpdateStatus` без source-status guard
**Источник:** Edge case hunter (E-6).
**Суть:** `UPDATE campaign_creators SET status=$1 WHERE id=$2` не фильтрует по source status. Если admin вручную перевёл cc в нестандартное состояние (или в `signed`/`signing_declined`), webhook повторно применит UPDATE без проверки текущего статуса.
**Risk:** мелкий — terminal-guard в `contracts.UpdateAfterWebhook` отсекает большинство race'ов на уровне contracts row. Но cc может расходиться.
**Возможный fix:** `WHERE id=$1 AND status='signing'`, n=0 → log + propagate как domain-error.
