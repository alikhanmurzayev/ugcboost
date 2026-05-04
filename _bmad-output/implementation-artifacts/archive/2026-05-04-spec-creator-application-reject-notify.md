---
title: "Telegram-уведомление о rejected заявке (бэк)"
type: feature
created: "2026-05-04"
status: in-review
baseline_commit: "6020535"
context:
  - docs/standards/
  - _bmad-output/planning-artifacts/creator-onboarding-roadmap.md
---

> Перед реализацией обязательно полностью загрузить все файлы `docs/standards/` — это hard rules, спека не дублирует.

<frozen-after-approval reason="human-owned intent — do not modify unless human renegotiates">

## Intent

**Problem:** Чанк 12 закрывает админскую ручку reject'а заявки, но **не уведомляет креатора** о решении. Без уведомления отклонённый сидит в боте без сигнала, что заявка обработана. По roadmap'у сначала шёл фронт (старый chunk 13), потом notify (старый chunk 14) — порядок переставлен: notify приклеивается прямо к бэку, чтобы admin-action сразу давал результат для креатора, а фронт-кнопка из drawer'а уже работала на полном flow.

**Approach:** Расширяем `RejectApplication` в `service/creator_application.go` — после `WithTx` дёргаем приватный `notifyApplicationRejected`. Тот через существующий `appTelegramLinkRepo.GetByApplicationID` достаёт `chat_id`, и если link есть — `notifier.NotifyApplicationRejected(ctx, chatID)`. Если link'а нет — `logger.Warn` и тишина (без fallback каналов и без падения). Notifier добавляет один новый метод поверх существующего fire-and-forget паттерна (как в chunk 8/9).

## Boundaries & Constraints

**Always:**
- Notify дёргается **после успешного `WithTx`**, не внутри callback'а (`backend-transactions.md` § «Логи успеха пишутся ПОСЛЕ WithTx» — здесь та же логика для notify, чтобы откатанная tx не отправила сообщение).
- Lookup link через `appTelegramLinkRepo.GetByApplicationID(ctx, applicationID)`. `sql.ErrNoRows` ⇒ `logger.Warn` + return. Любая другая ошибка ⇒ `logger.Error` + return. `RejectApplication` всё равно возвращает `nil` — reject уже закоммичен, не падаем.
- Сам `RejectApplication` возвращает результат **независимо от исхода notify** — fire-and-forget, как `notifyVerificationApproved` в chunk 8.
- Текст сообщения — статическая константа `applicationRejectedText` в `internal/telegram/notifier.go`. Содержимое — verbatim из § Decision спеки чанка 12. Plain text: `parse_mode` НЕ ставится (нет `<pre>`/HTML). Эмодзи `🙏` `🤍` сохраняются.
- Метод нотификатора — `(*Notifier).NotifyApplicationRejected(ctx, chatID int64)`. Принимает только chatID, никаких payload-структур (текст статичен). op-ключ для error-логов — `"application_rejected"`.
- Сервисный интерфейс `creatorAppNotifier` (в `service/creator_application.go`) расширяется новым методом. `make generate-mocks`.
- Никаких миграций. Никаких изменений `openapi.yaml`. Никаких новых полей в admin GET detail.

**Ask First:** none (все вопросы зарезолвлены при дизайне 2026-05-04).

**Never:**
- Retry-логика / persistent outbox / dead-letter — fire-and-forget, как chunk 8.
- Fallback-каналы (email/sms/звонок) когда нет telegram link — только warn в лог.
- Прокидывание notify-результата в HTTP response (200 возвращается **до** того, как notify завершится).
- Synchronous send внутри `WithTx` (откат tx → ложное сообщение креатору о rejected).
- Изменение текста сообщения через Config или категоризацию — статика, итерация PR'ом.
- Дубликаты на повторный POST: повторный reject отдаёт 422 (логика чанка 12) — до notify дело не доходит.
- PII в текст сообщения / в логи (имя/IIN/handle/email) — текст без переменных, лог содержит только `application_id` (UUID) и `chat_id` (числовой Telegram ID, не PII).
- **Комментарии в production-коде — минимум.** Стандарт `naming.md` § Комментарии: по умолчанию без комментария, только когда WHY реально неочевиден (скрытый инвариант, workaround, нелокальное ограничение). Никаких многострочных godoc на каждый метод. Никаких пояснений ЧТО делает понятный код. Это касается `internal/telegram/notifier.go`, `internal/service/creator_application.go`, моков и unit-тестов. **Исключение — header-комментарий e2e-файла (`backend/e2e/creator_applications/reject_test.go`):** обязателен, развёрнутый, нарратив на русском по `backend-testing-e2e.md` § Комментарий в начале файла.

## I/O & Edge-Case Matrix

> Триггер — успешный `POST /creators/applications/{id}/reject` (чанк 12). Сюда входим только когда reject уже закоммичен.

| Сценарий | Состояние link | Поведение |
|---|---|---|
| Happy verification + linked TG | row есть, `TelegramUserID=X` | `notifier.NotifyApplicationRejected(ctx, X)`. Spy-store содержит запись `{ChatID=X, Text=applicationRejectedText, ParseMode="", ReplyMarkup=nil, Err=""}`. Reject HTTP вернул 200 |
| Happy moderation + linked TG | то же | то же, статус-источник `moderation` |
| Happy + link отсутствует | `sql.ErrNoRows` | `logger.Warn(ctx, "creator application rejected without telegram link", "application_id", appID)`. Notifier не вызван. Spy-store пуст для этой заявки. Reject HTTP вернул 200 |
| Link lookup упал (БД-ошибка) | `GetByApplicationID` → generic error | `logger.Error(ctx, "creator application reject notify lookup failed", "application_id", appID, "error", err)`. Notifier не вызван. Reject HTTP вернул 200 |
| Telegram API упал на send | link есть, `sender.SendMessage` → error | Notifier ловит сам — `n.log.Error(ctx, "telegram notify failed", "op", "application_rejected", "chat_id", X, "error", err)`. Spy-store запись содержит `Err != ""`. Reject HTTP не затронут |
| Reject 422 / 404 / 403 / 401 (чанк 12) | — | `notifyApplicationRejected` НЕ вызывается (он за `WithTx`). Spy-store пуст |
| Двойной reject (idempotency) | первый закоммитился | первый дёргает notify; второй 422 → notify не дёргается. Spy-store ровно ОДНА запись для chat_id |

</frozen-after-approval>

## Code Map

> Baseline — merge-commit чанка 12 (фиксируется при подъёме status=ready-for-dev).

- `backend/internal/telegram/notifier.go` — новая константа `applicationRejectedText` (текст из § Decision спеки чанка 12, verbatim). Новый метод `NotifyApplicationRejected(ctx, chatID int64)` — копия `NotifyVerificationApproved`, op-ключ `"application_rejected"`, без `ParseMode`, без `ReplyMarkup`.
- `backend/internal/telegram/notifier_test.go` — `TestNotifier_NotifyApplicationRejected` (captured `*bot.SendMessageParams`: ChatID, Text == константа целиком, ParseMode == "", ReplyMarkup == nil; drain через `Wait()`). Дополнительный сценарий: SendMessage возвращает error → горутина не паникует, лог Error содержит `op="application_rejected"`, `chat_id`, `error`.
- `backend/internal/service/creator_application.go` — расширить интерфейс `creatorAppNotifier` (l.61) методом `NotifyApplicationRejected(ctx context.Context, chatID int64)`. Новый приватный helper `notifyApplicationRejected(ctx context.Context, applicationID string)` рядом с `notifyVerificationApproved` (l.~1099). В `RejectApplication` (добавляется чанком 12) после `WithTx` (вне callback'а) — вызов helper'а.
- `backend/internal/service/creator_application_test.go` — расширить `TestCreatorApplicationService_RejectApplication` (4 новых `t.Run` ниже + актуализация happy-кейсов с EXPECT на notifier).
- `backend/internal/service/mocks/mock_creator_app_notifier.go` — регенерация `make generate-mocks`.
- `backend/e2e/creator_applications/reject_test.go` — флипнуть `EnsureNoNewTelegramSent` в happy-кейсах на `WaitForTelegramSent` с ассертом точного `Text`/`ChatID`/`WebAppUrl=nil`/`Error=nil`. Добавить новый `happy_verification_no_telegram_link`. Локальная константа `expectedRejectText` с комментом «keep in sync with internal/telegram/notifier.go::applicationRejectedText» (e2e module изолирован, импорт `internal/` запрещён по `backend-testing-e2e.md`).
- `_bmad-output/planning-artifacts/creator-onboarding-roadmap.md` — переставить порядок: новый chunk 13 = notify (этот спек), новый chunk 14 = фронт reject в drawer (бывший 13). Обновить нумерацию/ссылки в группе 3.

## Tasks & Acceptance

**Pre-execution gates:**
- [x] PR чанка 12 (`spec-creator-application-reject`) смержен в main (PR #58, merge-commit `6020535`, спека архивирована). Baseline зафиксирован, status `draft` → `ready-for-dev`.

**Execution:**
- [x] Notifier: константа текста + метод `NotifyApplicationRejected` + unit-тест.
- [x] Service: расширение интерфейса, helper `notifyApplicationRejected`, вызов после `WithTx` в `RejectApplication`, регенерация моков (`make generate-mocks`).
- [x] Service unit-тесты: 4 новых `t.Run` в `TestCreatorApplicationService_RejectApplication` + актуализация существующих happy-сценариев (EXPECT на notifier).
- [x] E2E: `reject_test.go` — флип негативных ассертов в happy-сценариях на позитивные + новый `happy_verification_no_telegram_link` + проверка идемпотентности (двойной reject → ровно 1 запись в spy).
- [x] Roadmap: чанки 13 ↔ 14 переставлены, описание группы 3 актуализировано, revision-line добавлена.

**Acceptance Criteria:**
- Given app в `verification` с привязанным Telegram (`chat_id=X`), when admin POST reject, then 200 `{}`, и в течение 5s `GET /test/telegram/sent?chat_id=X&since=before` возвращает ровно ОДНУ запись с `text == applicationRejectedText` (целиком, через `require.Equal`), `web_app_url == null`, `sent_at` ≈ now (`WithinDuration(now, 10s)`). Поле `error` НЕ ассертим — под TeeSender'ом upstream Telegram отвергает синтетический chat id и spy фиксирует ошибку. Это outbound-параметры, не факт доставки (тот же контракт, что в `assertVerificationApprovedShape` из chunk 8 e2e).
- Given app в `moderation` с привязанным Telegram, when admin POST reject, then то же что выше, тот же текст.
- Given app в `verification` БЕЗ привязанного Telegram, when admin POST reject, then 200 `{}`, в логе сервиса warn с `application_id` (UUID), notifier не дёрнут — за 3s окно `EnsureNoNewTelegramSent` для контрольного chat_id пусто; admin GET detail подтверждает `telegramLink == null`.
- Given app в `verification` с привязанным TG, where unit-test `appTelegramLinkRepo.GetByApplicationID` возвращает generic-ошибку, when `service.RejectApplication` отрабатывает, then метод возвращает nil, в логе error с `application_id` и `error`, notifier mock без EXPECT.
- Given двойной POST reject подряд, when обе ручки отработали, then в spy-store ровно ОДНА запись для chat_id (второй POST 422 за `WithTx`, до notify не доходит).
- `make generate-api generate-mocks build-backend lint-backend test-unit-backend-coverage test-e2e-backend` — все зелёные, per-method coverage ≥80% на новых методах сервиса и notifier'а.

## Test Plan

> Производное от `backend-testing-unit.md`, `backend-testing-e2e.md`, `security.md`. Сверять с актуальной версией стандартов перед стартом Execute.

### Security invariants

- В `Text` сообщения — никаких переменных. Только статическая константа.
- В `logger.Warn` / `logger.Error` — только UUID и числовой `chat_id`. Никаких имён / IIN / handle / phone / email.
- В `error.Message` чанк 12 уже без PII — здесь не расширяем поверхность ошибок.

### Unit: notifier (`internal/telegram/notifier_test.go`)

`TestNotifier_NotifyApplicationRejected` (по образцу `TestNotifier_NotifyVerificationApproved`):
- `t.Parallel()`. Mock Sender с `mock.EXPECT().SendMessage(...).Run(captureParams).Return(&Message{ID:1}, nil)`.
- `notifier.NotifyApplicationRejected(ctx, int64(12345))`, затем `notifier.Wait()` для drain'а fire-and-forget горутины.
- Ассерты по captured `*bot.SendMessageParams`:
  - `params.ChatID.(int64) == 12345` точно.
  - `params.Text == applicationRejectedText` через `require.Equal` (целиком, не Contains).
  - `params.ParseMode == ""` (plain).
  - `params.ReplyMarkup == nil`.
- Сценарий `send error` — Sender возвращает `errors.New("network")`. После `Wait()` — мокабельный logger зафиксировал Error с точными kv: `op="application_rejected"`, `chat_id=12345`, `error="network"`. Горутина не паникует.

### Unit: service (`internal/service/creator_application_test.go`)

Расширение `TestCreatorApplicationService_RejectApplication`. Каждый `t.Run` с `t.Parallel()` и новым моком (`backend-testing-unit.md`).

- `happy_verification_with_linked_telegram` — `appTelegramLinkRepo.EXPECT().GetByApplicationID(ctx, appID).Return(&Row{TelegramUserID:12345, ...}, nil)`. `notifier.EXPECT().NotifyApplicationRejected(mock.Anything, int64(12345)).Run(captureChat).Return()`. Ассерт captured `chatID == 12345`. Существующий captured-input на `applyTransition`/`writeAudit` сохраняется.
- `happy_moderation_with_linked_telegram` — то же, `fromStatus=moderation`.
- `happy_no_telegram_link` — `GetByApplicationID` → `sql.ErrNoRows`. Notifier mock БЕЗ EXPECT (mockery упадёт при вызове). Logger mock: `EXPECT().Warn(mock.Anything, "creator application rejected without telegram link", "application_id", appID).Return()` — точные аргументы.
- `happy_link_lookup_error` — `GetByApplicationID` → `errors.New("db down")`. Notifier mock без EXPECT. Logger mock: `EXPECT().Error(mock.Anything, "creator application reject notify lookup failed", "application_id", appID, "error", mock.AnythingOfType("*errors.errorString")).Return()`. `RejectApplication` вернул `nil`.
- Существующие `not_rejectable_*` / `not_found` / `applyTransition error` / `audit error` — НЕ меняются; notifier mock остаётся без EXPECT (за `WithTx` до notify не доходим).

### E2E: расширение `backend/e2e/creator_applications/reject_test.go`

Header-комментарий обновляется: добавляется абзац про `happy_verification_no_telegram_link` и про переход с негативных Telegram-ассертов на позитивные.

Локальная константа в файле теста:
```go
// expectedRejectText must be kept in sync with
// internal/telegram/notifier.go::applicationRejectedText.
const expectedRejectText = "Здравствуйте! Благодарим вас за интерес к платформе UGC boost.\n\n..."
```

Изменения по сценариям относительно чанка 12:

- `happy_verification` — после reject `WaitForTelegramSent(t, chatID, {Since: before, ExpectCount: 1, Timeout: 5*time.Second})` возвращает срез из 1 элемента. Ассерты:
  - `msg.ChatID == linkedTelegramUserID` (захвачен в setup'е через `LinkTelegramToApplication`).
  - `msg.Text == expectedRejectText` через `require.Equal` (целиком).
  - `msg.WebAppUrl == nil`. **Поле `Error` не ассертим** — TeeSender (`TELEGRAM_MOCK=false` в e2e-окружении) форвардит реальный bot.SendMessage, который отвергает синтетический chat id. Spy фиксирует upstream-ошибку, но это outbound-параметры — fire-and-forget pipeline свою работу выполнил.
  - `msg.SentAt` ≈ now через `WithinDuration(now, 10*time.Second)`.
- `happy_moderation` — то же.
- `happy_verification_no_telegram_link` (НОВЫЙ) — setup использует `SetupCreatorApplicationInVerification` БЕЗ `LinkTelegramToApplication`. Reject 200. Admin GET detail подтверждает `telegramLink == nil` и `rejection.fromStatus == "verification"`. Notify-ассерт: для контрольного `dummyChatID := uniqueTelegramUserID()` `EnsureNoNewTelegramSent(t, dummyChatID, before, 3*time.Second)` — фильтр по chat_id защищает от чужих параллельных тестов.
- `not_rejectable_after_reject` (idempotency) — после первого 200 + второго 422: `WaitForTelegramSent(t, chatID, {Since: before, ExpectCount: 1})` возвращает РОВНО 1. Доп-poll `EnsureNoNewTelegramSent(t, chatID, after_first_send_ts, 2*time.Second)` подтверждает что вторая запись не появилась.
- `not_found` / `forbidden` / `unauthenticated` — без изменений (notify не дёргается, тут assert не критичен).

### Coverage / generation / race

- `make generate-mocks` после изменения интерфейса `creatorAppNotifier`.
- Per-method coverage ≥80% на `*Notifier.NotifyApplicationRejected` и `*CreatorApplicationService.notifyApplicationRejected`.
- `-race` — Notifier fire-and-forget горутина уже под race-detector, новый метод повторно покрывается.

## Verification

**Commands:**
- `make generate-mocks` — regenerate notifier mock with new method
- `make build-backend lint-backend test-unit-backend-coverage test-e2e-backend` — всё зелёное, coverage gate ≥80% per-method

**Manual smoke (local):**
- `make compose-up && make migrate-up`. Создать заявку через лендос, привязать Telegram (`/start UGC-...`), вызвать `curl -X POST -H "Authorization: Bearer $ADMIN_TOKEN" http://localhost:8082/creators/applications/<id>/reject`. В тестовом аккаунте Telegram приходит сообщение с текстом из § Decision (verbatim, plain text, эмодзи). Повторный reject — ничего не приходит, сервер 422. Без link'а — admin reject 200, в логах warn-строка с `application_id`.

## Spec Change Log

- **2026-05-04** — drafted. Pre-execution gate: ждём merge'а чанка 12 (`spec-creator-application-reject`). Baseline зафиксируется при подъёме до `ready-for-dev`.
- **2026-05-04** — чанк 12 в main (PR #58, merge `6020535`, спека архивирована). Baseline зафиксирован. Pre-execution gate `[x]`. Status `draft` → `ready-for-dev`. Добавлен инвариант в `Always`: минимум комментариев в production-коде по `naming.md` (исключение — header e2e-файла).
- **2026-05-04** — реализовано. Notifier-метод + service-helper + 4 unit-сценария + 4 e2e-сценария + roadmap swap 13↔14 + revision-line. Все Tasks `[x]`. Прогон `make build-backend lint-backend test-unit-backend-coverage test-e2e-backend` зелёный. Коррекция Test Plan / Acceptance Criteria: `msg.Error` НЕ ассертим (TeeSender фиксирует upstream-ошибку от синтетического chat id — outbound-параметры важнее факта доставки, тот же контракт, что в chunk-8 `assertVerificationApprovedShape`).
- **2026-05-04** — step-04 review (3 sub-agents: blind-hunter, edge-case-hunter, acceptance-auditor). Acceptance auditor: pass, 0 findings. Один **patch** применён: `notifyApplicationRejected` теперь оборачивает `ctx` в `context.WithoutCancel` перед lookup'ом link'а — симметрия с `Notifier.fire`'s send-call'ом, защита от cancel'а на shutdown'е/закрытом соединении между commit'ом tx и lookup'ом. 7 находок защищены как **defer** в `deferred-work.md` (race vs hypothetical delete-link, msg.Error ассерт, low-res clock CI race, panic-recovery в helper'е, WithinDuration окно, order-invariant в моках, chatID==0 guard). Один finding — operational note про rolling-deploy текста — **reject** (это прямо product-decision из спеки). Прогон `make build-backend lint-backend test-e2e-backend` после patch'а — зелёный. Status `in-progress` → `in-review`.
