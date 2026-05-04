---
title: "Manual verify соцсети заявки админом"
type: feature
created: "2026-05-04"
status: done
baseline_commit: ab57c67d2d91ce486254fa33554bbef94ed67f89
context:
  - docs/standards/
  - _bmad-output/planning-artifacts/creator-onboarding-roadmap.md
  - _bmad-output/planning-artifacts/creator-verification-concept.md
---

> Перед реализацией обязательно полностью загрузить все файлы `docs/standards/` — это hard rules, спека не дублирует их.

<frozen-after-approval reason="human-owned intent — do not modify unless human renegotiates">

## Intent

**Problem:** Chunk 10 roadmap'а. Заявки без Instagram (только TikTok / Threads) и заявки, у которых auto-verify в SendPulse не сработал, сидят в `verification` навсегда — нет admin-action'а пометить соцсеть верифицированной «под ответственность админа», чтобы заявка ушла в `moderation`. Без этого админка-фронт чанка 11 не может появиться, и pipeline стоит.

**Approach:** Новый admin-only `POST /creators/applications/{id}/socials/{socialId}/verify` (пустой body). Сервис в одной `WithTx`: lookup заявки в `verification` → lookup социалки в этой заявке → проверка not-already-verified → проверка наличия `creator_application_telegram_link` → `UpdateVerification` (method=`manual`, `verified_by_user_id=actor`) → `applyTransition(verification → moderation, reason=manual_verify)` → audit. **Никакого Telegram-уведомления креатору** — он сам не доказывал владение соцсетью, push об этом был бы вводящим в заблуждение.

## Boundaries & Constraints

**Always:**
- Endpoint в `openapi.yaml`: `POST /creators/applications/{id}/socials/{socialId}/verify`, security: bearerAuth, body: пустой `application/json` (не required), responses 200/401/403/404/409/422/default. Path-параметры — оба uuid.
- Authz: новый `AuthzService.CanVerifyCreatorApplicationSocialManually(ctx)` — admin-only, тот же шаблон что `CanViewCreatorApplication` (forbidden до любых DB-вызовов).
- Сервис: новый метод `(s *CreatorApplicationService) VerifyApplicationSocialManually(ctx, applicationID, socialID, actorUserID string) error`. Внутри одной `dbutil.WithTx` строгий порядок: (1) `appRepo.GetByID` — `sql.ErrNoRows` → `ErrCreatorApplicationNotFound`; (2) если `app.Status != verification` → `ErrCreatorApplicationNotInVerification`; (3) `socialRepo.ListByApplicationID` + поиск по socialID — нет → `ErrCreatorApplicationSocialNotFound`; (4) `social.Verified` → `ErrCreatorApplicationSocialAlreadyVerified`; (5) `linkRepo.GetByApplicationID` — `sql.ErrNoRows` → `ErrCreatorApplicationTelegramNotLinked`; (6) `socialRepo.UpdateVerification` (method=`manual`, `verified_by_user_id`=actor, `verified_at`=now); (7) `applyTransition(verification → moderation, actorID=&actor, reason=TransitionReasonManualVerify)`; (8) audit-row. **Notifier не дёргается ни в одной ветке.**
- Domain: новые sentinel'ы `ErrCreatorApplicationNotFound`, `ErrCreatorApplicationNotInVerification`, `ErrCreatorApplicationSocialNotFound`, `ErrCreatorApplicationSocialAlreadyVerified`, `ErrCreatorApplicationTelegramNotLinked`. Новые user-facing коды `CodeCreatorApplicationNotInVerification` (422), `CodeCreatorApplicationSocialNotFound` (404), `CodeCreatorApplicationSocialAlreadyVerified` (409), `CodeCreatorApplicationTelegramNotLinked` (422). Сообщения actionable («Заявка уже не на этапе верификации», «Эта соцсеть уже верифицирована», «Креатор не привязал Telegram-бота — попросите его открыть бот по deep-link и повторите»).
- Audit: новая action-константа `creator_application_verification_manual` в `audit_constants.go`. Метаданные: `application_id`, `social_id`, `social_platform`, `from_status`, `to_status`. ActorID = верифицирующий админ.
- Transition reason: новая константа `domain.TransitionReasonManualVerify = "manual_verify"`.
- Handler: маппинг sentinel'ов на статусы делает `respondError`/wrapping: `ErrCreatorApplicationNotFound` → 404, `ErrCreatorApplicationSocialNotFound` → 404, `ErrCreatorApplicationSocialAlreadyVerified` → 409, остальные два → 422. Тело успеха — пустой JSON `{}`.
- Actor UUID берётся из `middleware.UserIDFromContext(ctx)` в handler'е и пробрасывается в сервис как параметр (captured-input в хендлер-тестах).
- E2E ждёт 5 секунд после happy-path call'а и через `GET /test/telegram/sent?since=before` проверяет, что для этого `chat_id` записей нет (используется `SpyOnlySender` из chunk 8 PR-fix'ов).
- Roadmap: chunk 10 → `[~]` сейчас, при merge → `[x]`.

**Ask First:**
- Применять ли action на статусах `moderation` (доверификация остальных соцсетей после auto-verify) — default из обсуждения: **только `verification`**, на остальных 422.
- Принимать ли `note` от админа в body для `transitions.reason` / audit metadata — default: **не принимаем**, оставляем зарезервированную константу `manual_verify`.

**Never:**
- Telegram-уведомление креатору (см. Approach — креатор сам не верифицировал).
- Reject / withdraw — отдельные чанки (12, 13, 14, 19).
- UI-фронт — chunk 11.
- Изменения state-machine map (verification → moderation уже декларирован в chunk 8).
- Бэкфил / миграция — этот чанк только новый код.
- Регенерация `verification_code` или его модификация.
- Любые UPDATE'ы на других social-полях (handle, url) — только verified-блок.
- Снятие manual-verify (un-verify) — pipeline forward-only.

## I/O & Edge-Case Matrix

| Сценарий | Состояние | Поведение |
|---|---|---|
| Happy path | app `verification`, social `verified=false`, link есть | 200 `{}`; в БД: social `verified=true / method=manual / verified_by_user_id=actor / verified_at≈now`; app.status=`moderation`; transition row (`verification→moderation`, actor, reason=`manual_verify`); audit row; **никакого SendMessage** |
| Application не существует | random UUID | 404 `CREATOR_APPLICATION_NOT_FOUND` |
| Wrong status | app в `moderation` / `awaiting_contract` / `signed` / `rejected` / `withdrawn` | 422 `CREATOR_APPLICATION_NOT_IN_VERIFICATION`, ничего не пишется |
| Social не в этой app | random socialID или socialID из другой app | 404 `CREATOR_APPLICATION_SOCIAL_NOT_FOUND` |
| Social уже верифицирована | social `verified=true` (любым методом) | 409 `CREATOR_APPLICATION_SOCIAL_ALREADY_VERIFIED`, ничего не пишется |
| Telegram не привязан | app `verification`, social ok, link отсутствует | 422 `CREATOR_APPLICATION_TELEGRAM_NOT_LINKED`, ничего не пишется |
| Non-admin caller | brand_manager Bearer | 403 `FORBIDDEN` (до любого DB-вызова) |
| Unauthenticated | без Bearer | 401 |

</frozen-after-approval>

## Code Map

- `backend/api/openapi.yaml` — новый path + responses + новые ErrorCode-значения в shared enum (если есть; иначе в текстах).
- `backend/internal/domain/creator_application.go` — 5 новых sentinel'ов, 4 новых `Code*`, `TransitionReasonManualVerify`. `ErrCreatorApplicationNotFound` — добавить, если ещё нет (ранее `sql.ErrNoRows` доходил до handler через `respondError`).
- `backend/internal/service/audit_constants.go` — `AuditActionCreatorApplicationVerificationManual = "creator_application_verification_manual"`.
- `backend/internal/authz/creator_application.go` — `CanVerifyCreatorApplicationSocialManually(ctx)`.
- `backend/internal/authz/creator_application_test.go` — три сценария по шаблону `CanViewCreatorApplication`.
- `backend/internal/service/creator_application.go` — `VerifyApplicationSocialManually` (метод `*CreatorApplicationService`). Использует существующие `appRepo.GetByID`, `socialRepo.ListByApplicationID`, `socialRepo.UpdateVerification`, `linkRepo.GetByApplicationID`, `applyTransition`, `writeAudit`.
- `backend/internal/service/creator_application_test.go` — 7 сценариев (см. план тестов).
- `backend/internal/handler/creator_application.go` — handler `VerifyCreatorApplicationSocial` (имя — по `operationId`). Тянет actor из `middleware.UserIDFromContext`, маппит sentinel'ы на коды.
- `backend/internal/handler/creator_application_test.go` — 7 сценариев (success-captured-input, 4 ошибки сервиса → коды, 403, 401).
- `backend/internal/handler/response.go` (или где маппится `respondError`) — расширить маппинг на новые sentinel'ы.
- `backend/e2e/creator_applications/manual_verify_test.go` — `TestVerifyCreatorApplicationSocialManually` с 7 `t.Run` по матрице.
- `_bmad-output/planning-artifacts/creator-onboarding-roadmap.md` — chunk 10 → `[~]` сейчас, `[x]` при merge.

## Tasks & Acceptance

**Execution:**
- [x] OpenAPI: путь + responses + коды. `make generate-api`.
- [x] Domain: sentinel'ы, коды, `TransitionReasonManualVerify`.
- [x] Audit: action-константа.
- [x] Authz: `CanVerifyCreatorApplicationSocialManually` + unit-тесты (admin / brand_manager / no-role).
- [x] Service: `VerifyApplicationSocialManually` + unit-тесты по 7 сценариям (captured-input на `UpdateVerification` / `applyTransition` / audit; ассерт «notifier не дёрнут» через mockery без EXPECT).
- [x] Handler: новый handler + маппинг sentinel'ов в `respondError` + unit-тесты (captured actor/applicationID/socialID; 4 ошибки → коды; 403/401).
- [x] E2E: `manual_verify_test.go` по матрице, в happy-path — `EnsureNoNewTelegramSent` (5 секунд) ассерт «no records for chatID».
- [x] Roadmap: chunk 10 → `[~]`, при merge → `[x]`.

**Acceptance Criteria:**
- Given admin Bearer, app в `verification` со связкой Telegram и одной не-верифицированной TikTok-социалкой, when `POST /creators/applications/{id}/socials/{socialId}/verify`, then 200 `{}`; `GET /creators/applications/{id}` показывает `status=moderation`, `socials[i].verified=true / method=manual / verifiedByUserId=adminID / verifiedAt` непусто; в `audit_logs` строка `action=creator_application_verification_manual` с метаданными; в `creator_application_status_transitions` строка `(verification → moderation, actor=adminID, reason=manual_verify)`; через `GET /test/telegram/sent?since=before` после `Sleep(5s)` записей с этим `chat_id` нет.
- Given app в любом статусе ≠ `verification`, when call, then 422 `CREATOR_APPLICATION_NOT_IN_VERIFICATION` и БД не изменилась.
- Given app в `verification` без `creator_application_telegram_link`, when call, then 422 `CREATOR_APPLICATION_TELEGRAM_NOT_LINKED` и БД не изменилась.
- Given social уже `verified=true`, when call, then 409 и БД не изменилась.
- Given несуществующий applicationID или socialID не из этой app, when call, then 404 с соответствующим кодом.
- Given brand_manager Bearer, when call, then 403 `FORBIDDEN` без обращений к БД.
- `make generate-api build-backend lint-backend test-unit-backend-coverage test-e2e-backend` — все зелёные, per-method coverage ≥80%.

## Verification

**Commands:**
- `make generate-api` — codegen актуален.
- `make build-backend lint-backend test-unit-backend-coverage test-e2e-backend` — зелёные.

**Manual smoke (локально):**
- `make compose-up && make migrate-up`. В psql: создать заявку через лендос, привязать Telegram через бот, дёрнуть `curl -X POST -H "Authorization: Bearer $ADMIN_TOKEN" http://localhost:8082/creators/applications/<id>/socials/<social-id>/verify` → 200; в БД `social.verified=true / method=manual / verified_by_user_id=<admin-uuid>`; `app.status=moderation`; row в `creator_application_status_transitions`; row в `audit_logs`. В Telegram-боте у тестового аккаунта **сообщение НЕ приходит** (это новое поведение vs chunk 8).

## Spec Change Log

- **2026-05-04 — миграция fix FK + поле `id` у social DTO считаются _инфра-фиксом_, а не бэкфилом данных.**
  Триггер: ревью acceptance-аудитора зацепил Never-секцию `Бэкфил / миграция`. Исследование показало два infra-пробела чужих чанков, без которых chunk 10 не работает корректно:
  1. **FK `verified_by_user_id`**: миграция chunk-7 (`20260502204252_creator_application_verification_storage.sql:28`) объявляет `ON DELETE SET NULL`, но в живой схеме clause отсутствует. Без фикса любой `DELETE FROM users` для админа, который вручную верифицировал хотя бы одну социалку, ловит 23503 — что блокирует e2e-cleanup и будущие admin-driven сценарии. Forward-only миграция `20260504010647_fix_socials_verified_by_user_fk_set_null.sql` восстанавливает SET NULL без миграции данных.
  2. **`id` у `CreatorApplicationDetailSocial`**: без uuid соцсети admin-фронт chunk 11 не сможет дёрнуть `POST /creators/applications/{id}/socials/{socialId}/verify` — это естественный пререквизит, который Acceptance Criteria подразумевает («`socials[i].verified=true`» → нужен `i`-индекс с уникальным id).

  KEEP: оба фикса остаются в этом PR, миграция бамp бизнес-семантики не несёт. Ограничение «не пишем data-migrations / backfill» сохраняется — фикс FK constraint данных не трогает.

## Suggested Review Order

**Контракт API (entry point)**

- Новый admin-only endpoint, пустой body, шесть response кодов — точка входа всего PR
  [`openapi.yaml:656`](../../backend/api/openapi.yaml#L656)
- Поле `id` у social DTO — пререквизит для admin-фронта chunk 11
  [`openapi.yaml:1283`](../../backend/api/openapi.yaml#L1283)

**Сервис: intent + строгий порядок проверок**

- Метод `VerifyApplicationSocialManually` в одной WithTx — порядок 8 шагов (app→status→social→verified→link→update→transition→audit), notifier отсутствует намеренно
  [`creator_application.go:875`](../../backend/internal/service/creator_application.go#L875)
- Каждый sentinel возвращается до того, как что-либо пишется — гарантирует «БД не меняется при отказе»
  [`creator_application.go:885`](../../backend/internal/service/creator_application.go#L885)

**Domain → handler error mapping**

- Пять новых sentinel'ов (`ErrCreatorApplicationNotFound`, `…NotInVerification`, `…SocialNotFound`, `…SocialAlreadyVerified`, `…TelegramNotLinked`) и четыре кода
  [`creator_application.go:135`](../../backend/internal/domain/creator_application.go#L135)
- Расширение `respondError` — пять веток для новых sentinel'ов с actionable русскими сообщениями
  [`response.go:44`](../../backend/internal/handler/response.go#L44)
- Handler 9 строк: actor через `UserIDFromContext`, маппинг через `respondError`, `EmptyResult{}` при успехе
  [`creator_application.go:289`](../../backend/internal/handler/creator_application.go#L289)

**Authz**

- `CanVerifyCreatorApplicationSocialManually` — admin-only до любых DB-вызовов
  [`creator_application.go:53`](../../backend/internal/authz/creator_application.go#L53)

**Audit**

- Новая action-константа `creator_application_verification_manual`
  [`audit_constants.go:17`](../../backend/internal/service/audit_constants.go#L17)
- `contextWithActor` перед `writeAudit` — admin uuid и роль попадают в `audit_logs.actor_*` даже при bare-context из юнит-тестов
  [`creator_application.go:929`](../../backend/internal/service/creator_application.go#L929)

**State machine**

- `TransitionReasonManualVerify` — единственная новая константа в state-history vocabulary
  [`creator_application.go:527`](../../backend/internal/domain/creator_application.go#L527)

**Infra-фикс (см. Spec Change Log)**

- Forward-only fix FK `verified_by_user_id → users(id) ON DELETE SET NULL` — без него admin-actor блокирует e2e cleanup и future hard-deletes
  [`20260504010647_fix_socials_verified_by_user_fk_set_null.sql`](../../backend/migrations/20260504010647_fix_socials_verified_by_user_fk_set_null.sql)

**Тесты**

- E2E `TestVerifyCreatorApplicationSocialManually` — 8 t.Run по матрице, в happy-path `EnsureNoNewTelegramSent` (5 секунд) ассертит отсутствие пуша
  [`manual_verify_test.go:51`](../../backend/e2e/creator_applications/manual_verify_test.go#L51)
- Service-юниты: 7 сценариев + ассерт «notifier mock без EXPECT»
  [`creator_application_test.go:1597`](../../backend/internal/service/creator_application_test.go#L1597)
- Handler-юниты: 7 сценариев — captured actor/IDs, маппинг ошибок, 401 через `respondError`
  [`creator_application_test.go:1193`](../../backend/internal/handler/creator_application_test.go#L1193)
- Authz-юниты: 3 шаблонных сценария (admin/manager/no-role)
  [`creator_application_test.go:80`](../../backend/internal/authz/creator_application_test.go#L80)
