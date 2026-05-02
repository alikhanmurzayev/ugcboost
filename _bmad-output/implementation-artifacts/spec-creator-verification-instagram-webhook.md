---
title: "SendPulse webhook auto-верификации Instagram + state-history таблица"
type: feature
created: "2026-05-02"
status: ready-for-dev
context:
  - docs/standards/
  - _bmad-output/planning-artifacts/creator-onboarding-roadmap.md
  - _bmad-output/planning-artifacts/creator-verification-concept.md
---

> Перед реализацией обязательно полностью загрузить все файлы `docs/standards/` — это hard rules, спека не дублирует их.

<frozen-after-approval reason="human-owned intent — do not modify unless human renegotiates">

## Intent

**Problem:** Chunk 8 roadmap'а. После chunk 7 (PR #51) у заявок есть `verification_code` и поля верификации соцсетей, но нет endpoint'а, через который SendPulse-бот в IG авто-помечал бы IG-социалку верифицированной. Заявки в проде сидят в `verification` без способа уйти в `moderation`. Параллельно нужна таблица истории переходов — иначе после первого реального движения в проде потеряем структурированный след для будущей бизнес-аналитики (audit_logs покрывает «что произошло», transitions — «как менялся статус»).

**Approach:** Публичный POST `/webhooks/sendpulse/instagram` с bearer-secret auth парсит код из IG DM, находит активную заявку, верифицирует IG-социалку (`auto`) и переводит её `verification → moderation` через первый action-метод поверх нового приватного helper'а `applyTransition` (декларативная таблица легальных переходов в `domain` — сейчас только эта пара). В этом же чанке создаётся таблица `creator_application_status_transitions` (без бэкфила, без публичной ручки чтения). В одной `WithTx`: UPDATE social + UPDATE app.status + INSERT transition + INSERT audit. Telegram-нотификация креатору — fire-and-forget после tx. TMA-shell получает нейтральный плейсхолдер.

## Boundaries & Constraints

**Always:**
- Endpoint описан в `openapi.yaml` (паттерн codegen). Body: `username` (string, required), `lastMessage` (string, required), `contactId` (string, optional). Response 200 — пустой объект `{}` на всё, что отработали (включая no-op кейсы). Response 401 — wrong/missing secret, body `{}`.
- Auth — отдельное chi-middleware на этот path вне `AuthFromScopes`: constant-time сравнение `Authorization: Bearer <SENDPULSE_WEBHOOK_SECRET>`.
- Парсинг кода — `domain.ParseVerificationCode(text) (string, bool)`: regex `(?i)UGC-[0-9]{6}`, первый match в upper-case.
- Нормализация handle — `domain.NormalizeInstagramHandle(h) string`: lowercase + strip leading `@`. Применяется (a) при сохранении IG-социалок в `Submit`, (b) при приёме webhook payload, (c) в backfill-миграции для существующих рядов с `platform='instagram'`.
- Сервис: новый action-метод `(s *CreatorApplicationService) VerifyInstagramByCode(ctx, code, igHandle string) (VerifyInstagramStatus, error)`. Внутри одной `dbutil.WithTx` строгий порядок проверок: (1) lookup заявки по коду + status=verification — нет → `not_found`; (2) IG-социалки нет — `no_ig_social`; (3) social.verified==true — `noop` (приоритет идемпотентности над self-fix); (4) social.handle != normalizedIG — self-fix (флаг `handleChanged=true`); (5) UPDATE social (verified=true, method=auto, verified_at=NOW(), handle=normalizedIG) → `applyTransition` → audit (с `handle_changed`); (6) внутри той же tx подгрузить `telegram_user_id` через `linkRepo.GetByApplicationID`. После commit'а tx (если `status_changed`): сервис сам шлёт Telegram-уведомление (см. ниже) — это часть бизнес-логики, не handler'а.
- Helper `(s) applyTransition(ctx, tx dbutil.DB, app, toStatus string, actorID *uuid.UUID, reason string)`: проверка `domain.IsCreatorApplicationTransitionAllowed(from, to)` → UPDATE app.status → INSERT в `creator_application_status_transitions`. Не пишет audit, не дёргает side effects.
- Декларативная state-machine в `domain`: `creatorApplicationAllowedTransitions = map[from][to]bool`, сейчас только `verification → moderation`. Остальные переходы добавляются в своих чанках.
- Таблица `creator_application_status_transitions`: `id UUID PK gen_random_uuid()`, `application_id UUID NOT NULL → creator_applications(id) ON DELETE CASCADE`, `from_status TEXT NULL`, `to_status TEXT NOT NULL`, `actor_id UUID NULL → users(id) ON DELETE SET NULL`, `reason TEXT NULL`, `created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()`. Index `(application_id, created_at DESC)`. Без бэкфила.
- `reason` — TEXT в БД, но в Go-коде допустимые значения объявлены как enum-константы в `domain` (`TransitionReason*`); сейчас одно значение `TransitionReasonInstagramAuto = "instagram_auto"`, остальные добавятся в будущих чанках (manual verify, reject, withdraw). CHECK на enum в БД не делаем (стандарт: format/business на бэке, не в БД).
- Audit: новая action-константа `creator_application.verification.auto`. Метаданные включают `application_id`, `social_id`, `from_status`, `to_status`, `handle_changed` (bool). Audit пишется в той же `WithTx`, что и UPDATE'ы (стандарт).
- Telegram-нотификация триггерится **сервисом** (бизнес-логика, не транспорт). После commit'а `WithTx`: если статус сменился — вызов `telegram.SendVerificationNotification` через инжектченный `Sender`; `telegram_user_id == nil` → skip + warn-лог; ошибка `SendMessage` → error-лог, не пробрасывается. Handler тупой — отдаёт 200/401 независимо от исхода нотификации. Текст — новая константа в `internal/telegram/messages.go`. Кнопка — `InlineKeyboardButton.WebApp = &WebAppInfo{URL: tmaPublicURL}`. `telegram.Sender` и `cfg.TMAPublicURL` инжектятся в **`CreatorApplicationService`** через конструктор.
- Конфиг: `SendPulseWebhookSecret string env:"SENDPULSE_WEBHOOK_SECRET,required"`, `TMAPublicURL string env:"TMA_PUBLIC_URL,required"`.
- TMA placeholder: правка `frontend/tma/src/App.tsx` — заголовок «UGCBoost», подзаголовок меняется с «Telegram Mini App» на «Спасибо за заявку! Статус и инструкции скоро появятся здесь. Обновления приходят в Telegram». Без i18n.
- E2E-проверка истории переходов: содержимое `creator_application_status_transitions` через API не ассертится (публичной/админской ручки чтения нет — отдельный будущий чанк, когда появится UI). Покрытие — unit-тесты (pgxmock на `Insert` SQL + mock на `transitionRepo.Insert` в сервисе) + локальный psql при разработке. E2E ассертит audit-row через стандартную `/audit-logs` ручку (как везде при модификации данных).

**Ask First:**
- Дубли handle после backfill-нормализации (`@User` и `user` для одной заявки) — добавлять ли UNIQUE на `(application_id, platform, handle)`? Default — не добавляем.
- SendPulse передаёт `event_type` или `bot_id` в payload — валидируем ли? Default — нет, доверяем shared secret.
- Per-route middleware в oapi-codegen для одного path — поддерживается? Если нет — fallback на `r.Group` поверх HandlerFromMux с регистрацией пути отдельно.

**Never:**
- Admin/creator endpoint для чтения transitions — отдельный чанк, когда понадобится UI.
- Manual-verify endpoint — chunk 9. Reject — chunk 13. Withdraw — chunk 15.
- Бэкфил transitions для существующих заявок (концепция: первый row при первом реальном переходе).
- Creator-detail ручка для TMA — chunk 11.
- Outbox / persistent retry для Telegram — fire-and-forget по дизайну.
- Любой ответ webhook'у кроме 200/401 (никаких 4xx с подсказками злоумышленнику).
- Отправка ответа пользователю в IG DM из нашего бэка.

## I/O & Edge-Case Matrix

| Сценарий | Input / State | Expected Behavior |
|---|---|---|
| Happy path | valid bearer + payload, заявка `verification`, IG-handle совпадает | UPDATE social verified=true method=auto, status `moderation`, transition row, audit, Telegram-notify, 200 `{}` |
| Self-fix mismatch | valid bearer + payload, IG-handle отличается | UPDATE social handle на новый + verified, далее как happy. Audit с `handle_changed=true` |
| Already verified | valid bearer + payload, IG-социалка уже `verified=true` | no-op, debug-лог, no Telegram-notify, 200 `{}` |
| Заявки нет в verification | код не находит активную заявку (по partial unique index) | no-op, warn-лог, 200 `{}` |
| Заявка без IG-социалки | заявка есть, но соцсеть только TikTok/Threads | no-op, warn-лог, 200 `{}` |
| Код не найден в тексте | payload без `UGC-NNNNNN` | no-op, debug-лог, 200 `{}` |
| Код в нижнем регистре | "ugc-123456" | normalize в "UGC-123456" → дальше как обычно |
| Несколько кодов | "UGC-111111 UGC-222222" | берём первый match |
| Wrong/missing secret | invalid `Authorization` | 401 `{}`, body не парсим |
| Empty telegram_user_id | happy path, у заявки нет TG-binding | UPDATE+transition+audit committed, Telegram skip + warn, 200 `{}` |
| Telegram API failure | happy path, `SendMessage` upstream error | error-лог, 200 `{}` (бизнес-effect уже committed) |

</frozen-after-approval>

## Code Map

- `backend/api/openapi.yaml` — новый path `/webhooks/sendpulse/instagram`, схемы request/response.
- `backend/internal/config/config.go` — `SendPulseWebhookSecret` (required), `TMAPublicURL` (required).
- `backend/internal/middleware/sendpulse_auth.go` — constant-time bearer-compare.
- `backend/internal/handler/webhook_sendpulse.go` — тупой handler: парсинг payload → `VerifyInstagramByCode` → 200 `{}`. Никакой Telegram-логики, нет зависимости от `Sender`.
- `backend/cmd/api/main.go` — регистрация webhook-route с `sendpulseAuth` middleware вне `AuthFromScopes`.
- `backend/internal/domain/creator_application.go` — `ParseVerificationCode`, `NormalizeInstagramHandle`, `creatorApplicationAllowedTransitions`, `IsCreatorApplicationTransitionAllowed`, `ErrInvalidStatusTransition`, `VerifyInstagramStatus` enum, `TransitionReason*` enum-константы, audit-action const.
- `backend/internal/repository/creator_application_status_transition.go` — Row + Repo (`Insert`) + конструктор в RepoFactory.
- `backend/internal/repository/creator_application.go` — методы `UpdateStatus`, `GetByVerificationCodeAndStatus`. Расширение интерфейса.
- `backend/internal/repository/creator_application_social.go` — метод `UpdateVerification(ctx, id, handle, verified, method, verifiedByUserID, verifiedAt)`. Расширение интерфейса.
- `backend/internal/service/creator_application.go` — `applyTransition` (приватный), `VerifyInstagramByCode` (публичный, шлёт Telegram-уведомление после commit'а tx). Нормализация handle в `Submit` перед insert IG-социалок. Расширение `CreatorApplicationRepoFactory` интерфейса методом для transition-repo. Конструктор принимает `telegram.Sender` + `tmaPublicURL string`.
- `backend/internal/telegram/messages.go` — `MessageVerificationApproved` (RU).
- `backend/internal/telegram/notify.go` — helper `SendVerificationNotification(ctx, sender, chatID, tmaURL)` собирает SendMessageParams с inline-кнопкой `WebApp`.
- `backend/migrations/{ts}_creator_application_status_transitions.sql` — Up: CREATE TABLE + INDEX. Down: DROP.
- `backend/migrations/{ts}_normalize_instagram_handles.sql` — Up: `UPDATE creator_application_socials SET handle = lower(trim(BOTH '@' FROM handle)) WHERE platform = 'instagram'`. Down: no-op (необратимо).
- `backend/internal/handler/webhook_sendpulse_test.go`, расширение `service/creator_application_test.go`, `repository/creator_application_status_transition_test.go`, расширения `repository/creator_application_test.go` и `repository/creator_application_social_test.go`, расширение `domain/creator_application_test.go` (table-driven для парсера, нормализации, переходов).
- `backend/e2e/webhooks/sendpulse_instagram_test.go` — e2e по матрице через apiclient + admin-detail (статус/verified) + admin `/audit-logs` (audit-row) ассерты. transitions через API не ассертим (нет ручки) — покрытие unit + локальный psql.
- `backend/e2e/testutil/sendpulse_webhook.go` — helper, инкапсулирующий bearer + POST.
- `frontend/tma/src/App.tsx` + `App.test.tsx` — плейсхолдер-текст и smoke.
- `_bmad-output/planning-artifacts/creator-onboarding-roadmap.md` — chunk 8 → `[~]` сейчас, при merge → `[x]`.

## Tasks & Acceptance

**Execution:**
- [ ] OpenAPI: path + схемы → `make generate-api`.
- [ ] Domain: парсер, нормализация handle, allowed transitions map, helper, sentinel, status-enum, transition-reason константы, audit-action const + table-driven unit-тесты.
- [ ] Migration 1: `creator_application_status_transitions` (CREATE TABLE + INDEX).
- [ ] Migration 2: `normalize_instagram_handles` (UPDATE для existing IG-socials).
- [ ] Repository: `TransitionRepo` (новый файл) + `UpdateStatus` + `GetByVerificationCodeAndStatus` на `CreatorApplicationRepo` + `UpdateVerification` на `CreatorApplicationSocialRepo`. Расширение `RepoFactory`. pgxmock-тесты на SQL.
- [ ] Config: новые env vars, обновление env-loader тестов.
- [ ] Middleware: `sendpulse_auth` + unit-тесты.
- [ ] Service: `applyTransition` + `VerifyInstagramByCode` (покрывает все ветки I/O matrix через mock'и). Нормализация handle в `Submit`.
- [ ] Telegram: `MessageVerificationApproved` + `SendVerificationNotification` helper.
- [ ] Handler: `webhook_sendpulse.go` + регистрация route + unit-тесты (captured-input — что username/lastMessage из payload корректно передаются в сервис).
- [ ] TMA: `App.tsx` + `App.test.tsx`.
- [ ] E2E: `webhooks/sendpulse_instagram_test.go` покрывает матрицу (audit-row через `/audit-logs`).
- [ ] Roadmap: `[~] → [x]` при merge.

**Acceptance Criteria:**
- Given заявка в `verification` с кодом `UGC-123456` и IG-социалкой handle="myhandle", when POST с body `{"username":"myhandle","lastMessage":"UGC-123456"}` и валидным bearer'ом, then 200 `{}`, в БД `social.verified=true`, `method='auto'`, `verified_at` непустой, `application.status='moderation'`, новый row в `creator_application_status_transitions` (`from='verification'`, `to='moderation'`, `actor_id=NULL`, `reason='instagram_auto'` через `TransitionReasonInstagramAuto`) — проверяется в unit'ах + psql; новый audit-row с action `creator_application.verification.auto` и метаданными — проверяется в e2e через `/audit-logs`. Telegram `SendMessage` вызван (mock на `Sender` в unit-тестах сервиса; в e2e недоступно).
- Given валидная заявка + IG-handle "old", when POST с username "new" и валидным кодом, then `social.handle='new'`, далее как happy. Audit с `handle_changed=true`.
- Given заявка с уже `social.verified=true`, when повторный POST, then 200 `{}`, debug-лог, БД без изменений, без Telegram.
- Given код не находит заявки в `verification` (или нет IG-социалки), when POST, then 200 `{}`, warn-лог, БД без изменений.
- Given текст без `UGC-NNNNNN`, when POST, then 200 `{}`, debug-лог, БД без изменений.
- Given неверный bearer-secret, when POST, then 401 `{}`.
- Given happy + `telegram_user_id=NULL` у заявки, when POST, then UPDATE+transition+audit как обычно, Telegram пропущен (warn-лог), 200 `{}`.
- Given существующие IG-социалки с handle `"@MyName"` / `"MyName"` / `"myname"`, after migration `normalize_instagram_handles`, then handle везде `"myname"`.
- `make build-backend lint-backend test-unit-backend-coverage test-e2e-backend lint-web build-web build-tma test-unit-tma generate-api` — все зелёные.

## Verification

**Commands:**
- `make generate-api` — codegen актуален.
- `make build-backend lint-backend test-unit-backend-coverage test-e2e-backend` — зелёные, per-method coverage ≥80%.
- `make migrate-up` — обе миграции применяются на dev.
- `make build-tma test-unit-tma lint-web` — frontend собирается.

**Manual smoke локально (перед PR):**
- `make compose-up && make migrate-up` — миграции применились без ошибок.
- `psql` в локальный контейнер: `SELECT id, platform, handle, verified, method, verified_at FROM creator_application_socials WHERE platform='instagram';` — после migration handle в lowercase без `@`.
- Подать тестовую заявку (через лендос или `make run-landing` форму) → дёрнуть `curl -X POST http://localhost:8082/webhooks/sendpulse/instagram -H "Authorization: Bearer $SENDPULSE_WEBHOOK_SECRET" -H "Content-Type: application/json" -d '{"username":"<handle-из-заявки>","lastMessage":"<verification_code-из-БД>"}'` — 200 `{}`, статус заявки `moderation`, social.verified=true.
- `psql`: `SELECT * FROM creator_application_status_transitions ORDER BY created_at DESC LIMIT 10;` — появился row `(verification → moderation)`.
- `psql`: `SELECT action, metadata, created_at FROM audit_logs WHERE action LIKE 'creator_application.verification%' ORDER BY created_at DESC LIMIT 10;` — появился audit-row.
