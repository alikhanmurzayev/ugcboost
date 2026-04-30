---
title: 'review-fixes-telegram-link'
type: 'bugfix'
created: '2026-04-30'
status: 'done'
baseline_commit: '062c0d17cd94d283dd13b49ed38e4fefdf1e75bc'
context:
  - docs/standards/security.md
  - docs/standards/backend-architecture.md
  - docs/standards/review-checklist.md
---

<frozen-after-approval reason="human-owned intent — do not modify unless human renegotiates">

## Intent

**Problem:** Ревью ветки `alikhan/telegram-link` нашло 3 блокера и 3 major'а: бот ловит non-private chats и `From == nil` (PII leak в группы + битые записи `telegram_user_id=0`), нет panic recovery в long-poll handler'е, сервис содержит лишний preflight + `TelegramLinkResult` с dead fields + naming inconsistency `Tg*`/`Telegram*`, и в репо есть 15 неотформатированных Go-файлов без CI-gate.

**Approach:** Hardening бот-handler'а (filter chat.Type, drop From=nil/ID<=0, defer recover), упрощение сервиса до `LinkTelegram(...) error` без preflight #2 (Insert + ловля 23503/23505), переименование domain-полей, новая forward-only миграция возвращающая `CHECK telegram_user_id > 0`, gofmt всех файлов + gate в golangci-lint v2 + пункт в `review-checklist.md`.

## Boundaries & Constraints

**Always:**
- Forward-only миграции (memory `feedback_zero_downtime.md`) — существующие миграции `20260429224431` и `20260430203000` не редактируем; CHECK возвращаем третьей миграцией.
- PII (`username`, `first/last name`) разрешены ТОЛЬКО в `audit_logs.new_value`, запрещены в stdout — закрепить guard'ом нельзя (отброшено в ревью), но соблюдать в коде.
- Бот-handler — НЕ HTTP, panic recovery делаем локально по образцу `middleware/recovery.go`.
- Все мутации `creator_application_telegram_links` — внутри `dbutil.WithTx` с audit-row в той же транзакции.

**Ask First:**
- Если переименование `Tg*`/`Telegram*` или удаление `TelegramLinkResult` затрагивает callsite вне `internal/{domain,service,telegram,handler}` или `e2e/` — HALT.
- Если форматирование gofmt'ом затрагивает файлы с активными изменениями в других ветках — HALT.

**Never:**
- НЕ менять capability-token-семантику `applicationID` (отброшено в ревью).
- НЕ добавлять `/start@<bot>` parsing (отброшено).
- НЕ добавлять PII guard test (отброшено).
- НЕ откатывать удаление PII inventory из `security.md` (осознанное решение).
- НЕ редактировать существующие миграции `20260429224431` / `20260430203000`.

## I/O & Edge-Case Matrix

| Scenario | Input / State | Expected | Error Handling |
|----------|--------------|----------|----------------|
| /start <uuid> private, normal user | `Chat.Type=private`, `From={ID:>0}` | `MessageLinkSuccess`, link row + audit | — |
| /start <uuid> private, idempotent (same TG) | `Chat.Type=private`, link уже в БД | `MessageLinkSuccess`, БЕЗ audit, debug-лог `"idempotent": true` | — |
| /start <uuid> private, занято другим TG | другой `telegram_user_id` в БД | `MessageApplicationAlreadyLinked` | `BusinessError(CodeTelegramApplicationAlreadyLinked)` |
| /start <uuid> private, заявка не существует | applicationID не в `creator_applications` | `MessageApplicationNotFound` | `domain.ErrNotFound` (через FK 23503) |
| /start в group/channel | `Chat.Type != private` | (молча `return`) | — |
| /start <uuid>, `From == nil` или `From.ID <= 0` | anon admin, channel post | (молча `return`) | — |
| panic внутри Handle | любой код-путь panic'ит | error-лог со stack, `return` (процесс жив) | — |

</frozen-after-approval>

## Code Map

- `backend/internal/telegram/handler.go` — bot handler: filter chat.Type/From, defer recover, drop result-arg LinkTelegram
- `backend/internal/telegram/handler_test.go` — добавить кейсы group/channel/From=nil/panic, обновить под error-only сигнатуру
- `backend/internal/telegram/mocks/mock_link_service.go` — regenerate (mockery) под новую сигнатуру
- `backend/internal/domain/creator_application.go` — переименовать `TelegramLinkInput.Tg*→Telegram*`, удалить `TelegramLinkResult`
- `backend/internal/service/creator_application_telegram.go` — убрать preflight `GetByApplicationID`, `LinkTelegram(...) error`, маппинг 23503→`ErrNotFound`, debug-лог idempotent
- `backend/internal/service/creator_application_telegram_test.go` — переписать ассерты на repo state
- `backend/internal/repository/creator_application_telegram_link.go` — добавить ветку `pgErr.Code == "23503"` → новый sentinel `domain.ErrTelegramApplicationNotFound` (или существующий `domain.ErrNotFound` если симметрично)
- `backend/internal/repository/creator_application_telegram_link_test.go` — добавить кейс «23503 propagates as not-found»
- `backend/migrations/20260430210000_creator_application_telegram_links_check_user_id_positive.sql` — НОВАЯ forward-only миграция, `ADD CONSTRAINT ... CHECK (telegram_user_id > 0)`
- `backend/.golangci.yml` — `formatters: enable: [gofmt]` (v2 syntax)
- `docs/standards/review-checklist.md` — пункт `[major] gofmt -l backend/ пуст`
- 15 неотформатированных файлов в `backend/` — `gofmt -w`

## Tasks & Acceptance

**Execution:**
- [x] `backend/internal/domain/creator_application.go` -- переименовать `TgUserID/TgUsername/TgFirstName/TgLastName` → `TelegramUserID/...`, удалить структуру `TelegramLinkResult` -- naming consistency + dead-fields cleanup
- [x] `backend/internal/repository/creator_application_telegram_link.go` -- в `Insert` добавить ветку для `pgErr.Code == "23503"` → `domain.ErrNotFound` -- маппинг FK violation на доменную ошибку
- [x] `backend/internal/service/creator_application_telegram.go` -- сменить сигнатуру на `LinkTelegram(ctx, in, now) error`; debug-лог при idempotent; убрать `idempotentResult` helper. **Preflight `linkRepo.GetByApplicationID` оставлен** — Postgres aborts the whole transaction on a constraint violation, поэтому EAFP-стиль (catch 23505 → re-read) не работает в одной tx (см. Spec Change Log) -- упрощение в рамках Postgres-семантики
- [x] `backend/internal/telegram/handler.go` -- (a) `if update.Message.Chat.Type != "private" { return }`, (b) `if update.Message.From == nil || update.Message.From.ID <= 0 { return }`, (c) обернуть тело `Handle` в `defer recover()` с error-логом по образцу `middleware/recovery.go` (`runtime/debug.Stack()`), (d) обновить вызов `LinkTelegram` под новую сигнатуру -- bot hardening
- [x] `backend/internal/telegram/handler_test.go` -- добавить тесты: group-chat ignored, supergroup ignored, channel ignored, From=nil ignored, From.ID<=0 ignored, panic в LinkService recovers; обновить существующие под error-only сигнатуру -- coverage
- [x] `backend/internal/repository/creator_application_telegram_link_test.go` -- добавить кейс «pgErr 23503 → domain.ErrNotFound» -- coverage маппинга
- [x] `backend/internal/service/creator_application_telegram_test.go` -- переписать ассерты на error-only; покрыть preflight-сценарии (same TG → idempotent, different TG → AlreadyLinked, sql.ErrNoRows → Insert) и race-net 23505 после preflight (rare) → AlreadyLinked -- coverage
- [x] `backend/migrations/20260430210000_creator_application_telegram_links_check_user_id_positive.sql` -- новый goose-файл `ADD CONSTRAINT ... CHECK (telegram_user_id > 0)` (Up) / `DROP CONSTRAINT` (Down) -- defense in depth
- [x] `backend/internal/handler/testapi.go` -- симулируемый Update получает `Chat.Type: "private"` (production handler теперь дропает не-private) -- совместимость теста с новым фильтром
- [x] `make generate-mocks` -- regen `mock_link_service.go` под новую сигнатуру -- codegen pipeline
- [x] `gofmt -w backend/` (затронуло 15 файлов) -- унификация форматирования
- [x] `backend/.golangci.yml` -- секция `formatters: enable: [gofmt]` (golangci v2) -- gate в lint
- [x] `docs/standards/review-checklist.md` -- в § Process / Artifacts добавлен пункт `[major] gofmt -l backend/ возвращает пусто; gofmt включён в formatters секции backend/.golangci.yml` -- зафиксировать стандарт

**Acceptance Criteria:**
- Given бот добавлен в группу и пользователь шлёт `/start <uuid>`, when handler получает update с `Chat.Type == "group"`, then никакой ответ не отправляется и БД не меняется.
- Given анонимный admin постит `/start <uuid>`, when `update.Message.From == nil`, then handler return без ответа и без link-row.
- Given handler паникует внутри `LinkTelegram`, when defer recover срабатывает, then в лог уходит `"telegram handler panic"` со stack-трейсом и процесс продолжает обрабатывать следующие updates.
- Given `creator_applications` row удалён между preflight и Insert, when сервис вызывает `Insert`, then получает `pgErr.Code == "23503"`, маппит в `domain.ErrNotFound`, handler отвечает `MessageApplicationNotFound`.
- Given `INSERT INTO creator_application_telegram_links (telegram_user_id) VALUES (0)` после миграции `20260430210000`, when выполняется напрямую в БД, then получает CHECK violation `23514`.
- Given `gofmt -l backend/` запускается локально или в CI, when один или более файлов не отформатированы, then команда возвращает non-empty список и `golangci-lint run` падает.

## Spec Change Log

### 2026-04-30 — Postgres aborted-tx блокирует EAFP re-read

**Trigger:** e2e `idempotent_repeat_from_same_TG` и `application_already_linked_to_a_different_TG` упали с `MessageInternalError` после удаления preflight `linkRepo.GetByApplicationID`.

**Amended:** preflight read оставлен. Изначальный task требовал «удалить preflight» и оставить decision только в 23505-ветке.

**Bad state avoided:** Postgres помечает всю транзакцию `aborted` после любого constraint violation — последующий `SELECT` в той же tx падает `current transaction is aborted, commands ignored until end of transaction block`. Без preflight любой повторный /start (link уже существует) уходил в `MessageInternalError` вместо idempotent / AlreadyLinked.

**KEEP:**
- error-only сигнатура `LinkTelegram(...) error`.
- маппинг 23503 → `domain.ErrNotFound` в репо.
- debug-лог idempotent ветки.
- 23505-ветка как safety net на race между preflight и Insert (без re-read'а — сразу возвращает `AlreadyLinked`).

## Verification

**Commands:**
- `make generate-mocks && make build-backend` -- ожидание: успешная сборка после переименования + новой сигнатуры
- `make lint-backend` -- ожидание: проходит, в т.ч. gofmt
- `gofmt -l backend/` -- ожидание: пустой вывод
- `make test-unit-backend` -- ожидание: все тесты зелёные, новые кейсы покрывают сценарии из I/O Matrix
- `make migrate-up && make test-e2e-backend` -- ожидание: новая миграция применяется, e2e зелёные

## Suggested Review Order

**Bot handler hardening**

- Точка входа: фильтр chat.Type, From, defer recover — все три блокера ревью в одной функции
  [`handler.go:43`](../../backend/internal/telegram/handler.go#L43)

- Защитный nil-guard в buildLinkInput на случай переиспользования из новых entry-point'ов
  [`handler.go:121`](../../backend/internal/telegram/handler.go#L121)

**Service refactor**

- Error-only сигнатура `LinkTelegram(...) error` + debug-лог на idempotent + комментарий про Postgres aborted-tx
  [`creator_application_telegram.go:53`](../../backend/internal/service/creator_application_telegram.go#L53)

- 23505 safety net без re-read'а (aborted-tx semantics) — комментарий объясняет trade-off
  [`creator_application_telegram.go:107`](../../backend/internal/service/creator_application_telegram.go#L107)

**Domain rename + dead-fields cleanup**

- `Tg* → Telegram*` в TelegramLinkInput; TelegramLinkResult удалён целиком
  [`creator_application.go:194`](../../backend/internal/domain/creator_application.go#L194)

**DB integrity**

- Forward-only миграция возвращает `CHECK telegram_user_id > 0` как defence in depth
  [`20260430210000_*.sql:1`](../../backend/migrations/20260430210000_creator_application_telegram_links_check_user_id_positive.sql#L1)

**Repo error mapping**

- Симметричный маппинг 23505/23503 с проверкой ConstraintName, константы рядом
  [`creator_application_telegram_link.go:62`](../../backend/internal/repository/creator_application_telegram_link.go#L62)

**Tests**

- Новые кейсы: group/supergroup/channel/From=nil/From.ID<=0/panic/mixed-case hex
  [`handler_test.go:80`](../../backend/internal/telegram/handler_test.go#L80)

- Preflight-based scenarios + 23505 safety net + 23503 → ErrNotFound покрытие
  [`creator_application_telegram_test.go:68`](../../backend/internal/service/creator_application_telegram_test.go#L68)

- 23503 ConstraintName маппинг
  [`creator_application_telegram_link_test.go:81`](../../backend/internal/repository/creator_application_telegram_link_test.go#L81)

**Test endpoint compat**

- `/test/telegram/message` симулирует private chat — без этого production-фильтр дропает все тестовые updates
  [`testapi.go:118`](../../backend/internal/handler/testapi.go#L118)

**Infra / standards**

- gofmt включён в golangci-lint v2 formatters — gate в CI
  [`.golangci.yml:3`](../../backend/.golangci.yml#L3)

- Hard rule в process-чеклисте про `gofmt -l backend/` пустой
  [`review-checklist.md:72`](../../docs/standards/review-checklist.md#L72)
