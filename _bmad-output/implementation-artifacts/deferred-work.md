---
title: "Deferred work — отложенные находки adversarial-ревью"
type: deferred-work
status: living
created: "2026-05-04"
---

# Deferred work

Накопитель findings из step-04 ревью, классифицированных как **defer**: реальные, но не относящиеся к scope текущего chunk'а или с риском низким настолько, что фикс лучше делать отдельным focused PR.

Формат: дата → откуда пришёл finding → severity → описание → почему отложили.

---

## 2026-05-04 — Chunk 13 (Telegram-уведомление о rejected)

Из adversarial-ревью спеки `spec-creator-application-reject-notify.md` (step-04 в `/bmad-quick-dev`). Три ревьюера: blind-hunter, edge-case-hunter, acceptance-auditor.

### [minor] Race-окно lookup vs hypothetical delete-link ручки

- **Источник:** blind-hunter.
- **Где:** `backend/internal/service/creator_application.go:notifyApplicationRejected` — lookup `appTelegramLinkRepo.GetByApplicationID(ctx, applicationID)` идёт через `s.pool` после commit'а tx. Если когда-нибудь появится ручка delete-link и креатор успеет её дёрнуть между tx-commit'ом reject'а и lookup'ом — `sql.ErrNoRows` и тихий warn вместо отправки.
- **Почему defer:** ручки delete-link в проекте нет и не планируется. Read-replica lag в Postgres single-instance setup'е не даёт расхождения. Реальная угроза появится только с future feature, тогда же и решать (вариант: тащить `chat_id *int64` из tx наружу как в `notifyVerificationApproved`).

### [minor] e2e `assertRejectPushExact` намеренно НЕ ассертит `msg.Error`

- **Источник:** blind-hunter.
- **Где:** `backend/e2e/creator_applications/reject_test.go:301-308`.
- **Описание:** под TeeSender'ом (`TELEGRAM_MOCK=false` в e2e-окружении) реальный `bot.SendMessage` отвергает синтетический `chat_id` и spy фиксирует upstream-ошибку. Поле `Error` не ассертим — но это значит, что любая регрессия в send-pipeline (panic в горутине, неправильный `WithoutCancel`-обвес, изменение контракта sender'а) у этого ассерта проходит мимо радара.
- **Почему defer:** контракт зеркалит chunk-8 `assertVerificationApprovedShape` — единый паттерн через два чанка. Усиление через fingerprint-маркер (например, ассерт что `Error` содержит специфичный для invalid-chat-id текст от Telegram, а не goroutine panic) — отдельный focused PR на оба чанка сразу, чтобы не плодить inconsistency.

### [major] e2e: low-res clock на CI может вернуть welcome+reject в один срез WaitForTelegramSent

- **Источник:** edge-case-hunter.
- **Где:** `backend/e2e/creator_applications/reject_test.go:128` (`since := time.Now().UTC()` в happy_verification).
- **Описание:** если `LinkTelegramToApplication` дренирует welcome push и возвращается с `SentAt == T`, а следующая инструкция `since := time.Now().UTC()` возвращает тот же `T` (CI-контейнер с jiffy>1ms), фильтр `r.SentAt.Before(filter.Since)` пропускает welcome → spy отдаёт `[welcome, reject]` → `require.Len(t, msgs, 1)` валит.
- **Почему defer:** edge-case теоретический. На текущей инфре (`make test-e2e-backend` локально + CI) welcome корректно отфильтровывается. Профилактический фикс — гарантировать `since := time.Now().UTC().Add(time.Millisecond)` или поменять spy-фильтр на `Before(Since.Add(-time.Millisecond))`. Решать одним PR, который пройдётся по всем e2e-сценариям с тем же паттерном (chunk 8 + chunk 13).

### [minor] notifyApplicationRejected без panic-recovery

- **Источник:** edge-case-hunter.
- **Где:** `backend/internal/service/creator_application.go:notifyApplicationRejected`.
- **Описание:** Notifier.fire защищён `recover()`, helper `notifyApplicationRejected` — нет. Теоретически, если будущий refactor вставит nil в `s.pool` или `s.repoFactory`, паника убьёт процесс.
- **Почему defer:** DI без nil-able параметров на текущем DI-графе невозможен (constructor валидирует). Это refactor-protection, не реальная дыра. Симметричный фикс с `notifyVerificationApproved` (там тоже нет recovery) — отдельным defensive-PR'ом.

### [minor] WithinDuration(now, 10s) — слабое окно для CI

- **Источник:** edge-case-hunter.
- **Где:** `backend/e2e/creator_applications/reject_test.go:307`.
- **Описание:** `assertRejectPushExact` сравнивает `msg.SentAt` с `time.Now().UTC()` ассерт-времени, не `since`. На медленном CI с GC-паузами разница может превысить 10s. Сейчас прогоны зелёные, но риск flaky-теста ненулевой.
- **Почему defer:** реальные прогоны не показывают проблему. Решать поднятием окна до 30s или сменой baseline на `since` — отдельный test-hygiene PR.

### [minor] Order-invariant: моки не enforce post-commit lookup

- **Источник:** edge-case-hunter.
- **Где:** unit-тесты `creator_application_test.go:2105-2171` (happy_no_telegram_link / happy_link_lookup_error).
- **Описание:** mockery EXPECT'ы не проверяют порядок вызовов. Если будущий refactor случайно перенесёт `notifyApplicationRejected` внутрь `WithTx`, все 4 unit-теста и e2e зеленеют, но семантический инвариант "lookup post-commit" нарушен.
- **Почему defer:** усиление через `mock.InOrder` или Run-callback с auditCommitted-флагом — бойлерплейт ради защиты от refactor'а, который не планируется. Открыть chunk при появлении первого реального refactor'а.

### [minor] chatID == 0 не отбивается defensively в Notifier

- **Источник:** edge-case-hunter.
- **Где:** `backend/internal/telegram/notifier.go:NotifyApplicationRejected`.
- **Описание:** DB-constraint `telegram_user_id > 0` гарантирует валидный chat_id, но Notifier defensively не проверяет. Если будущий refactor обойдёт DB-инвариант, Telegram API ответит upstream-error'ом без явного warn в сервис-логе.
- **Почему defer:** симметрично с `NotifyVerificationApproved` (там тоже без guard'а) — единый паттерн. Защитный guard добавлять единым PR'ом на оба метода + проверка что DB-инвариант сохраняется.
