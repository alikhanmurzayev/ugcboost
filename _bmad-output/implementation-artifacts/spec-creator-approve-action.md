---
title: "Approve action для заявки креатора (chunk 18b)"
type: feature
created: "2026-05-05"
status: in-progress
baseline_commit: "18ed697"
context:
  - docs/standards/
  - _bmad-output/planning-artifacts/creator-onboarding-roadmap.md
  - _bmad-output/implementation-artifacts/spec-creator-foundation.md (18a — фундамент, должен быть смержен)
  - _bmad-output/implementation-artifacts/spec-creator-application-approve.md (старая большая спека сохранена как референс)
---

> Перед реализацией обязательно полностью загрузить все файлы `docs/standards/` — это hard rules, спека не дублирует.

<frozen-after-approval reason="human-owned intent — do not modify unless human renegotiates">

## Intent

**Problem:** После 18a в БД лежат пустые таблицы `creators` / `creator_socials` / `creator_categories` и есть готовые repo. Но нет ни action'а для admin-approve, ни заполнения этих таблиц данными — заявка не может перейти в `approved`, креатор как сущность не возникает в системе.

**Approach:** Один admin-action `POST /creators/applications/{id}/approve` без body, response `{ creatorId }`. В одной `dbutil.WithTx`: status-guard → telegram-link-guard → snapshot-read из заявки → `applyTransition(moderation → approved)` → `creatorRepo.Create` + bulk INSERT соцсетей и категорий → audit. После commit'а — fire-and-forget Telegram-уведомление через retry-обёртку из 18a (новый метод `NotifyApplicationApproved`). E2E `approve_test.go` покрывает all-paths матрицу + race; **полная structural-проверка агрегата откладывается на 18c** (там введём GET /creators и helper `AssertCreatorAggregateMatchesSetup`). В 18b happy-path E2E проверяет факт записи через прямые SQL-чтения из БД (по аналогии со smoke в 18a) + acceptance чек-лист для ручной проверки.

## Decisions

### Snapshot из заявки в момент approve

Все PII / соцсети / категории / Telegram-link заявки копируются в `creators` + связанные таблицы. Маппинг:

- `creators.{iin, last_name, first_name, middle_name, birth_date, phone, city_code, address, category_other_text}` ← `creator_applications.<тоже самое>` (1-в-1).
- `creators.{telegram_user_id, telegram_username, telegram_first_name, telegram_last_name}` ← `creator_application_telegram_links.<тоже самое>` (плоско).
- `creators.source_application_id` ← `creator_applications.id`.
- `creator_socials` rows ← `creator_application_socials` rows (все, включая невёрнутые; verified-поля копируются 1-в-1: `verified`, `method`, `verified_by_user_id`, `verified_at`).
- `creator_categories` rows ← `creator_application_categories` rows (`category_code`).

**Не копируется:** `verification_code`, consents, status_transitions, `linked_at` Telegram-link, оригинальные `created_at` / `updated_at` заявки (у креатора собственные).

### Domain-types для Creator вводятся здесь

В 18a sentinel'ы лежали без types. В 18b service оперирует доменными представлениями:

- `domain.Creator` — плоский объект, что service отдаёт repo на `Create`.
- `domain.CreatorSocial`, `domain.CreatorCategory` — слайсы, что service отдаёт на `InsertMany`.

Все три заводятся в `domain/creator.go` (файл создан в 18a с тремя sentinel'ами; здесь только дополняем types). User-facing codes для трёх sentinel'ов из 18a (`ErrCreatorAlreadyExists`, `ErrCreatorTelegramAlreadyTaken`, `ErrCreatorApplicationNotApprovable`) — вводятся в 18b: `Code* = "CREATOR_*"` + actionable-message в `handler/response.go`.

### Approve без Telegram-link → 422 с reuse существующего sentinel

Инвариант: одобренный креатор обязан иметь канал коммуникации. Если `creator_application_telegram_links` для заявки отсутствует — service возвращает 422 `CREATOR_APPLICATION_TELEGRAM_NOT_LINKED`. Переиспользуется существующий `domain.ErrCreatorApplicationTelegramNotLinked` (`creator_application.go:163`, используется в manual-verify chunk 10) и его маппинг в `handler/response.go:55`. Никакого нового sentinel'а / кода вводить не надо.

### Notify после commit, fire-and-forget с retry из 18a

`s.notifyApplicationApproved(ctx, applicationID)` дёргается ПОСЛЕ `WithTx` (вне callback'а), по образцу `notifyApplicationRejected` (`service/creator_application.go:1117`). Делает второй lookup link — это незначительный дополнительный запрос, но семантически чище (lookup отделён от транзакционных INSERT'ов). Captured-variable из callback'а тоже работал бы, но reject уже задал паттерн «второй lookup» — зеркалим.

Под капотом `notifier.NotifyApplicationApproved` идёт через перенесённую в 18a retry-обёртку — никаких дополнительных параметров здесь.

Текст сообщения — статический в `internal/telegram/messages.go` (как `applicationRejectedText`):

> Здравствуйте!
>
> Рады сообщить, что ваша заявка прошла модерацию 😍 Ваш профиль, визуальный стиль и контент соответствуют критериям отбора для участия в fashion-кампаниях платформы UGC boost 💫
>
> В ближайшее время мы отправим вам детали участия в EURASIAN FASHION WEEK и договор для подписания.
>
> Добро пожаловать на платформу UGC boost 💫
>
> После Недели моды мы планируем запустить приложение в App Store и добавить новые возможности для UGC-сотрудничества с брендами и партнерами EURASIAN FASHION WEEK.
>
> Оставайтесь с нами — впереди много масштабных проектов!

Plain text, без `parse_mode`, без inline-keyboard. Итерируется отдельным PR'ом заменой константы.

### Race на approve закрыт UNIQUE из 18a

Два concurrent `POST approve` на одну заявку: оба читают `moderation`, оба применяют `applyTransition` (UPDATE статуса + INSERT в `creator_application_status_transitions`), но второй упирается в `creators_source_application_id_unique` на `creatorRepo.Create`. Весь TX2 откатывается атомарно — включая INSERT в transitions. В БД остаётся ровно одна creator-row + одна transition-row (моя ошибка из аудита, что нужен `SELECT FOR UPDATE`, опровергнута: rollback атомарен). С `-race` тест проходит детерминированно.

### Полная сверка агрегата откладывается на 18c

В 18b нет GET /creators — значит нет API, через который E2E-тест мог бы пощупать все 30+ полей креатора. Поэтому E2E happy-path в 18b делает усечённую проверку:

- 200 + `creatorId` непустой и UUID-формат.
- Admin GET application detail → `status=approved`.
- `WaitForTelegramSent` ловит одно `applicationApprovedText`.
- Faktum записи в БД проверяется через **прямой SQL-запрос** в e2e-test через testapi (или новый testapi-endpoint `/test/creator/{id}` — либо без него, через `e2e/raw.go`-стиль raw-DB-clien, если он уже есть для других проверок).

Полная structural-проверка `AssertCreatorAggregateMatchesSetup` приедет в 18c вместе с GET-handler'ом и fixture-helper'ами.

## Boundaries & Constraints

**Always:**

- OpenAPI: `POST /creators/applications/{id}/approve`, security `bearerAuth`, без `requestBody`, responses 200 / 401 / 403 / 404 / 422 / default. Path `id` — uuid. Response 200 — schema `CreatorApprovalResult` с одним полем `creatorId: uuid`.
- 3 новых ErrorCode: `CREATOR_APPLICATION_NOT_APPROVABLE`, `CREATOR_ALREADY_EXISTS`, `CREATOR_TELEGRAM_ALREADY_TAKEN`. Все с actionable user-facing message.
- Сценарий «нет Telegram-link» — переиспользует существующий `CREATOR_APPLICATION_TELEGRAM_NOT_LINKED` + `ErrCreatorApplicationTelegramNotLinked` + готовый маппинг в `handler/response.go:55`. Никакого нового кода / sentinel'а.
- Authz: `AuthzService.CanApproveCreatorApplication(ctx)` admin-only. Шаблон зеркало `CanRejectCreatorApplication` (`authz/creator_application.go`).
- Service: `(s *CreatorApplicationService) ApproveApplication(ctx context.Context, applicationID, actorUserID string) (creatorID string, err error)`. В одной `dbutil.WithTx`:
  1. `appRepo.GetByID(tx, applicationID)` → `sql.ErrNoRows` ⇒ `ErrCreatorApplicationNotFound`; `app.Status != moderation` ⇒ `ErrCreatorApplicationNotApprovable`.
  2. `linkRepo.GetByApplicationID(tx, applicationID)` → `sql.ErrNoRows` ⇒ `ErrCreatorApplicationTelegramNotLinked` (существующий).
  3. `socialsRepo.ListByApplicationID(tx, applicationID)` + `categoriesRepo.ListByApplicationID(tx, applicationID)` — snapshot read.
  4. `s.applyTransition(ctx, tx, appRow, domain.CreatorApplicationStatusApproved, &actor, domain.TransitionReasonApprove)`.
  5. `creatorRepo.Create(tx, composedCreatorRow)` → 23505 транслируется через repo (уже сделано в 18a) в один из трёх sentinel'ов.
  6. `creatorSocialsRepo.InsertMany(tx, mappedRows)` + `creatorCategoriesRepo.InsertMany(tx, mappedRows)`.
  7. `writeAudit(auditCtx, auditRepo, AuditActionCreatorApplicationApprove, AuditEntityTypeCreatorApplication, appRow.ID, nil, metadata)`.
- После commit: `s.notifyApplicationApproved(ctx, applicationID)` — lookup link + вызов `notifier.NotifyApplicationApproved`. Зеркало `notifyApplicationRejected` (`creator_application.go:1117`).
- Расширение `creatorAppNotifier` интерфейса (`service/creator_application.go:60`) методом `NotifyApplicationApproved(ctx context.Context, chatID int64)`. После — `make generate-mocks`.
- Domain: 3 новых `CreatorApplicationStatusApproved` (если ещё не было после chunk 17 — оно в `domain.CreatorApplicationStatuses` уже после 17, sanity-check в Code Map). Константа `TransitionReasonApprove = "approve_admin"` в `domain/creator_application.go`. Расширение `creatorApplicationAllowedTransitions` парой `(moderation → approved)`.
- Domain types: `domain.Creator`, `domain.CreatorSocial`, `domain.CreatorCategory` в `domain/creator.go` (файл создан в 18a). Helper `domain.NewCreatorFromApplication(app *CreatorApplicationRow, link *CreatorApplicationTelegramLinkRow) *Creator` композирует доменный объект из двух источников.
- User-facing codes для трёх 18a-sentinel'ов: `CodeCreatorAlreadyExists`, `CodeCreatorTelegramAlreadyTaken`, `CodeCreatorApplicationNotApprovable`. Plus messages в `handler/response.go` маппинге.
- Audit: `AuditActionCreatorApplicationApprove = "creator_application_approve"` в `service/audit_constants.go`. Metadata: `{application_id, creator_id, from_status: moderation, to_status: approved}`.
- Handler: `ApproveCreatorApplication` (по openapi `operationId`). Actor через `middleware.UserIDFromContext`. Captured-input в handler-тестах. Маппинг 3 новых + reuse существующего `TelegramNotLinked` в `handler/response.go`.
- Telegram: `applicationApprovedText` в `messages.go`. Метод `(*Notifier) NotifyApplicationApproved(ctx, chatID)` рядом с reject — простой делегат к `n.fire(...)` из 18a (no parse_mode, no inline-keyboard).
- Service-side `CreatorApplicationRepoFactory` интерфейс в `service/creator_application.go` расширяется тремя методами: `NewCreatorRepo` / `NewCreatorSocialRepo` / `NewCreatorCategoryRepo`. После — `make generate-mocks`.
- testapi `CleanupEntityRequest.type` enum в `openapi-test.yaml` расширяется значением `creator`. Handler-case в `internal/testapi/...` делегирует в `creatorRepo.DeleteForTests` (готов с 18a). После — `make generate-api`.
- testutil `DeleteCreatorForTests(t *testing.T, creatorID string)` — новый файл `backend/e2e/testutil/creator.go`, зеркало `DeleteCreatorApplicationForTests`. Cleanup-stack регистрирует `DeleteCreatorForTests` ПОСЛЕ `DeleteCreatorApplicationForTests` (LIFO выполнит creator первым; FK `creators.source_application_id` без ON DELETE — RESTRICT по умолчанию).
- E2E `creator_applications/approve_test.go`: матрица сценариев (см. § I/O Matrix). Setup: `testutil.SetupAdmin` + ручной composable `seedApprovableApplication(t)` локальный к файлу (заявка → `verification` → manual-verify админом → `moderation` → возврат `applicationID` + actorID + chatID). `SetupCreatorApplicationInModeration` (переиспользуемый fixture-pipeline) и `AssertCreatorAggregateMatchesSetup` (структурное сравнение) — откладываются на 18c.
- Factual-чек happy в e2e (без structural-сравнения — оно в 18c): 200 + `creatorId` непустой UUID + admin GET application detail отдаёт `status=approved` + `WaitForTelegramSent` ловит `applicationApprovedText` + `DeleteCreatorForTests(t, creatorId)` в cleanup-стеке отрабатывает успешно (доказывает, что creator-row реально записан — cleanup получит `sql.ErrNoRows`, если row'а нет).

**Ask First (BLOCKING до Execute):**
- (нет — все вопросы зарезолвлены)

**Never:**
- GET /creators/{id} endpoint (это 18c).
- `CreatorAggregate` schema, `CreatorService` сервис, `GetByID` метод (это 18c).
- `CreatorApplicationFixture` / `SocialFixture` / `SetupCreatorApplicationInModeration` / `AssertCreatorAggregateMatchesSetup` testutil-helpers (это 18c).
- Approval-блок в admin GET application detail.
- Метрики / observability counter'ы.
- Бэкфил.
- Расширение `CreatorApplicationStatus` enum (он уже в state-machine после chunk 17).
- Fix-up'ы / правки notifier-retry (если в 18a что-то сломалось — отдельный hotfix-PR).
- Текст approve-сообщения с переменными / шаблонами (статическая константа).
- HTML / inline-keyboard в approve-сообщении.

## I/O & Edge-Case Matrix

| Сценарий | Состояние | Поведение |
|---|---|---|
| Happy approve | app `moderation`, telegram-link есть, заявка содержит `middle_name` + `address` + `category_other_text` + 3 соцсети (1 verified IG + 1 verified TT + 1 non-verified Threads) + 3 категории | 200 `{creatorId}`; `creator_applications.status=approved`; transition row `(moderation → approved, actor, reason=approve_admin)`; audit row `creator_application_approve` с metadata; новые rows: 1 в `creators`, 3 в `creator_socials` (verified-поля 1-в-1), 3 в `creator_categories`; `WaitForTelegramSent` ловит одно `applicationApprovedText` на link.chat_id |
| Application не существует | random UUID | 404 `CREATOR_APPLICATION_NOT_FOUND` |
| Wrong status — verification | app в `verification` | 422 `CREATOR_APPLICATION_NOT_APPROVABLE`, БД не изменилась |
| Wrong status — rejected / withdrawn / approved | соответственно | 422 `CREATOR_APPLICATION_NOT_APPROVABLE`, БД не изменилась |
| No telegram-link | app в `moderation`, link отсутствует | 422 `CREATOR_APPLICATION_TELEGRAM_NOT_LINKED`; в `creators` / `creator_socials` / `creator_categories` ничего не появилось; transition row не создалась (guard до applyTransition) |
| Concurrent approve | 2 параллельных POST на одну заявку | один 200, второй 422 `CREATOR_APPLICATION_NOT_APPROVABLE`; в БД ровно одна creator-row + одна transition-row (UNIQUE на `creators.source_application_id` ловит TX2 на Create, весь TX2 откатывается атомарно) |
| Дубликат IIN | в `creators` уже есть row с тем же IIN (теоретически из будущего re-application flow) | 422 `CREATOR_ALREADY_EXISTS` |
| Telegram занят | telegram_user_id уже у другого креатора | 422 `CREATOR_TELEGRAM_ALREADY_TAKEN` |
| Non-admin caller | brand_manager Bearer | 403 `FORBIDDEN` (до DB-вызова) |
| Unauthenticated | без Bearer | 401 |

## Local Smoke Acceptance

Автор PR обязан **лично** прогнать после реализации:

1. `make compose-up && make migrate-up && make start-backend`.
2. Через `curl` или `httpie` локально воспроизвести full pipeline: подача заявки → /start link → manual-verify → переход в moderation → approve.
3. После approve — `curl` POST /creators/applications/{id}/approve с admin Bearer → 200 `{creatorId}`.
4. Прямые SQL-запросы (psql или DBeaver):
   - ✅ `SELECT * FROM creator_applications WHERE id=<appID>` → `status='approved'`.
   - ✅ `SELECT * FROM creators WHERE source_application_id=<appID>` → ровно одна row, все поля 1-в-1 с заявкой (`iin`, ФИО, `birth_date`, `phone`, `city_code`, `address`, `category_other_text`, `telegram_*`).
   - ✅ `SELECT * FROM creator_socials WHERE creator_id=<creatorID> ORDER BY platform, handle` → все соцсети заявки, verified-поля 1-в-1.
   - ✅ `SELECT * FROM creator_categories WHERE creator_id=<creatorID> ORDER BY category_code` → все category_code заявки.
   - ✅ `SELECT * FROM creator_application_status_transitions WHERE application_id=<appID> AND to_status='approved'` → одна row, `from_status='moderation'`, `actor_id=<adminID>`, `reason='approve_admin'`.
   - ✅ `SELECT * FROM audit_logs WHERE action='creator_application_approve' AND entity_id=<appID>` → одна row, metadata содержит `creator_id`, `from_status='moderation'`, `to_status='approved'`.
5. ✅ В Telegram у тестового аккаунта (через локальный Telegram-test-token, см. `.env.example` `TELEGRAM_MOCK`) пришло одно сообщение `applicationApprovedText`.
6. ✅ Повторный POST approve той же заявки → 422 `CREATOR_APPLICATION_NOT_APPROVABLE`.
7. ✅ POST approve **той же заявки** с brand_manager Bearer → 403.
8. ✅ POST approve **рандомного UUID** → 404.
9. ✅ Создать новую заявку без link (без /start), довести её manually до `moderation` через прямые SQL-UPDATE для теста, POST approve → 422 `CREATOR_APPLICATION_TELEGRAM_NOT_LINKED`. SQL-проверка: в `creators` и связанных таблицах для этой заявки ничего нет.

Все 9 шагов должны успешно пройти подряд перед review-ready пометкой PR.

## Code Map

> Baseline — `18ed697` (merge commit PR #65, 18a).

- `backend/api/openapi.yaml` —
  - Новый path `POST /creators/applications/{id}/approve`.
  - Новая schema `CreatorApprovalResult` (`{creatorId: uuid}`) для response 200.
  - 3 новых ErrorCode (см. § Always). `TELEGRAM_NOT_LINKED` уже существует.
  - После — `make generate-api`.
- `backend/internal/domain/creator.go` —
  - Patch (файл создан в 18a с 3 sentinel'ами): добавить domain types `Creator`, `CreatorSocial`, `CreatorCategory` (плоские структуры, не Row).
  - Добавить user-facing codes для трёх sentinel'ов из 18a: `CodeCreatorApplicationNotApprovable`, `CodeCreatorAlreadyExists`, `CodeCreatorTelegramAlreadyTaken`.
  - Helper `NewCreatorFromApplication(app, link) *Creator` — композиция доменного объекта.
- `backend/internal/domain/creator_application.go` —
  - Patch: константа `TransitionReasonApprove = "approve_admin"`.
  - Patch: расширение `creatorApplicationAllowedTransitions` парой `(moderation → approved)`.
  - Patch: actionable user-facing messages для новых codes (если нужно через Code-системы; в существующем коде сейчас messages пишутся прямо в `handler/response.go`).
- `backend/internal/service/audit_constants.go` — patch: `AuditActionCreatorApplicationApprove`.
- `backend/internal/authz/creator_application.go` — patch: `CanApproveCreatorApplication`.
- `backend/internal/authz/creator_application_test.go` — patch: 3 новых `t.Run` для approve (admin / brand_manager / no-role).
- `backend/internal/service/creator_application.go` —
  - Patch: расширение `CreatorApplicationRepoFactory` интерфейса 3 методами (`NewCreatorRepo` и т.д.).
  - Patch: расширение `creatorAppNotifier` интерфейса методом `NotifyApplicationApproved`.
  - Patch: метод `ApproveApplication(ctx, applicationID, actorUserID) (creatorID string, err error)`. Использует `applyTransition`, `writeAudit` (existing helpers).
  - Patch: метод `notifyApplicationApproved(ctx, applicationID)` (private, зеркало `notifyApplicationRejected` строка 1117).
- `backend/internal/service/creator_application_test.go` —
  - Patch: `TestCreatorApplicationService_ApproveApplication` (см. § Test Plan).
- `backend/internal/handler/creator_application.go` — patch: `ApproveCreatorApplication` handler.
- `backend/internal/handler/creator_application_test.go` — patch: `TestCreatorApplicationHandler_ApproveCreatorApplication`.
- `backend/internal/handler/response.go` — patch: 3 новых case (NotApprovable / CreatorAlreadyExists / CreatorTelegramAlreadyTaken). Existing `TELEGRAM_NOT_LINKED` case (`response.go:55`) остаётся без правок — переиспользуется.
- `backend/internal/telegram/messages.go` — patch: `applicationApprovedText`.
- `backend/internal/telegram/notifier.go` — patch: метод `NotifyApplicationApproved(ctx, chatID)` (тривиальный — делегат к `n.fire(...)`).
- `backend/internal/telegram/notifier_test.go` — patch: 2 новых `t.Run` под approve (verbatim text + sender error logged).
- `backend/internal/service/mocks/...` — regenerate after notifier-interface + factory-interface changes (`make generate-mocks`).
- `backend/api/openapi-test.yaml` — patch: `CleanupEntityRequest.type` enum расширяется значением `creator`. После — `make generate-api`.
- `backend/internal/testapi/...` — patch: handler-case `creator` в `cleanupEntity`-обработчике, делегирует в `creatorRepo.DeleteForTests`.
- `backend/e2e/testutil/creator.go` — новый файл: `DeleteCreatorForTests(t *testing.T, creatorID string)` через testapi (`POST /test/cleanup-entity` с `{type: "creator", id: creatorID}`). Этот файл расширится в 18c новыми helper'ами (`AssertCreatorAggregateMatchesSetup` и т.д.), здесь только cleanup.
- `backend/e2e/creator_applications/approve_test.go` — новый файл. `TestApproveCreatorApplication`. Локальный composable `seedApprovableApplication(t)` (через testutil-existing `LinkTelegramToApplication` + `manualVerifyApplicationSocial` если есть; или прямой submit + link + admin manual-verify). Cleanup-стек регистрирует `DeleteCreatorForTests` (для creator-row) ПОСЛЕ `DeleteCreatorApplicationForTests` (для заявки) — LIFO порядок гарантирует, что creator уйдёт первым.
- `_bmad-output/planning-artifacts/creator-onboarding-roadmap.md` — chunk 18.5 (approve action) → `[~]` при старте, `[x]` при merge.

## Tasks & Acceptance

**Pre-execution gates:**
- [x] PR 18a (foundation) смержен в main; baseline_commit зафиксирован (`18ed697`).
- [x] `make migrate-up` локально работает — таблицы из 18a на месте.
- [x] `make test-unit-backend` зелёный на baseline.

**Execution:**
- [x] OpenAPI: 1 path + 1 response schema + 3 ErrorCode. `make generate-api`.
- [x] Domain: types Creator/CreatorSocial/CreatorCategory + helper NewCreatorFromApplication + user-facing codes для 3 sentinel'ов из 18a + TransitionReasonApprove + расширение transition map.
- [x] Audit: action-константа.
- [x] Authz: CanApproveCreatorApplication + unit-тесты.
- [x] Service: расширение CreatorApplicationRepoFactory + creatorAppNotifier интерфейсов; `make generate-mocks`. Метод ApproveApplication + notifyApplicationApproved + unit-тесты по матрице. (Дополнительно: `GetByIDForUpdate` на CreatorApplicationRepo для сериализации concurrent approve race — Postgres без явного row-lock'а возвращает `iin_unique` раньше `source_application_id_unique`, что разъезжается с § Decisions.)
- [x] Handler: ApproveCreatorApplication + 3 новых case в response.go + unit-тесты.
- [x] Telegram: applicationApprovedText + NotifyApplicationApproved + unit-тесты.
- [x] testapi: `CleanupEntityRequest.type=creator` enum + handler-case `creatorRepo.DeleteForTests`. `make generate-api`.
- [x] testutil: `DeleteCreatorForTests` + `RegisterCreatorCleanup` (новый файл `creator.go`).
- [x] E2E: approve_test.go по матрице. Happy: 200 + `creatorId` UUID + GET application status=approved + WaitForTelegramSent + успешный `DeleteCreatorForTests` в cleanup. Полная structural-проверка БД откладывается на 18c.
- [x] Локальный smoke (через изолированный e2e happy + прямой psql): creators / creator_socials / creator_categories / creator_application_status_transitions / audit_logs — все по 1+ row, поля 1-в-1.
- [x] Roadmap: chunk 18.5 → `[~]` старт; `[x]` ставится через /finalize после merge.

**Acceptance Criteria:**
- Given admin Bearer + app `moderation` + link, when POST approve, then 200 `{creatorId}`; admin GET application → `status=approved`; `WaitForTelegramSent` → 1 approve-message; `DeleteCreatorForTests(creatorId)` в cleanup'е успешен (доказывает запись).
- Given app `verification` / `rejected` / `withdrawn` / `approved`, when call, then 422 NOT_APPROVABLE.
- Given app `moderation` без link, when call, then 422 TELEGRAM_NOT_LINKED. (Отсутствие creator-row подтверждается unit-тестом сервиса: guard стоит ДО любых INSERT'ов; в e2e factual-чек придёт в 18c.)
- Given несуществующий applicationID, then 404 NOT_FOUND.
- Given brand_manager / unauthenticated, then 403 / 401 без БД-вызовов.
- Given concurrent approve, then ровно один 200 + ровно один 422 NOT_APPROVABLE; cleanup-стек чистит 1 creator + 1 заявку.
- Все 9 ручных smoke-шагов отработали.
- `make generate-api && make generate-mocks && make build-backend lint-backend test-unit-backend-coverage test-e2e-backend` — зелёные.

## Test Plan

### Validation rules

В handler ловится только path-uuid (через ServerInterfaceWrapper) — ручного парсинга нет. Body отсутствует.

### Security invariants

Сверить перед merge:
- PII в stdout-логах запрещена. В service / handler разрешены: `application_id`, `creator_id`, `actor_id`, `chat_id`, `from_status`, `to_status`. PII (имена / IIN / handle / phone / address) — нет.
- Нет PII в `error.Message` — все шаблонные.
- Нет PII в URL (uuid в path).
- Length bound — body отсутствует.
- Rate-limiting — admin-only, не публичный вектор. Не закладываем.

### Unit tests

#### `authz/creator_application_test.go` — `TestCanApproveCreatorApplication`

`t.Parallel()`. Шаблон зеркало `CanRejectCreatorApplication`. `t.Run`: admin → nil, brand_manager → ErrForbidden, no role → ErrForbidden.

#### `service/creator_application_test.go` — `TestCreatorApplicationService_ApproveApplication`

`t.Parallel()` на функции и каждом `t.Run`. Новый mock на каждый сценарий.

`creatorAppNotifier` mock внутри callback'а получает **0 EXPECT** — mockery упадёт, если service случайно отправит уведомление до commit'а. После commit — `EXPECT().NotifyApplicationApproved(...)` с captured `chat_id`.

`t.Run` в порядке исполнения:
- `application not found` — `appRepo.GetByID` → `sql.ErrNoRows` → `errors.Is(ErrCreatorApplicationNotFound)`.
- `not approvable` — table-driven для 4 не-moderation статусов: `verification`, `rejected`, `withdrawn`, `approved` → `errors.Is(ErrCreatorApplicationNotApprovable)`. Mock'и не вызывают applyTransition / Create / audit.
- `telegram link missing` — `linkRepo.GetByApplicationID` → `sql.ErrNoRows` → `errors.Is(ErrCreatorApplicationTelegramNotLinked)`. applyTransition / Create не вызваны.
- `applyTransition error propagated` — `appRepo.UpdateStatus` или `transitionRepo.Insert` возвращают ошибку → `ErrorContains` обёртки. Create не вызван.
- `creator already exists` — `creatorRepo.Create` → `ErrCreatorAlreadyExists` (из 23505 — repo сам мапит в 18a) → пробрасывается as-is.
- `telegram already taken` — `creatorRepo.Create` → `ErrCreatorTelegramAlreadyTaken`.
- `concurrent race` — `creatorRepo.Create` → `ErrCreatorApplicationNotApprovable` (23505 на source_application_id).
- `socials insert error propagated` — `creatorSocialsRepo.InsertMany` → ошибка → `ErrorContains` обёртки + audit НЕ вызван.
- `categories insert error propagated` — `creatorCategoriesRepo.InsertMany` → ошибка → `ErrorContains` обёртки + audit НЕ вызван.
- `audit error propagated` — все INSERT'ы ok, `writeAudit` → ошибка → `ErrorContains` обёртки.
- `happy path` — captured-input на каждый repo-call:
  - `applyTransition`: `app.Status=moderation`, `toStatus=approved`, `actor=&adminID`, `reason=TransitionReasonApprove`.
  - `creatorRepo.Create`: полный CreatorRow из app + link (поле-в-поле).
  - `creatorSocialsRepo.InsertMany`: snapshot всех соцсетей, verified-поля 1-в-1, `creator_id` подменён на новый creator.ID.
  - `creatorCategoriesRepo.InsertMany`: все category_code из заявки.
  - `writeAudit`: action / actor / entity / metadata через `JSONEq` на `{application_id, creator_id, from_status, to_status}`.
- После commit — `notifier.NotifyApplicationApproved` дёрнут с правильным chat_id.

Captured-input через `mock.EXPECT().X(...).Run(func(args mock.Arguments) {...})`.

#### `handler/creator_application_test.go` — `TestCreatorApplicationHandler_ApproveCreatorApplication`

`t.Parallel()`. Black-box через `httptest`. Body отсутствует.

`t.Run`:
- `unauthenticated → 401`.
- `forbidden non-admin → 403`.
- `application not found → 404 CREATOR_APPLICATION_NOT_FOUND`.
- `not approvable → 422 CREATOR_APPLICATION_NOT_APPROVABLE`.
- `telegram not linked → 422 CREATOR_APPLICATION_TELEGRAM_NOT_LINKED`.
- `creator already exists → 422 CREATOR_ALREADY_EXISTS`.
- `telegram already taken → 422 CREATOR_TELEGRAM_ALREADY_TAKEN`.
- `happy → 200 {creatorId}` + captured-input на service: `actor=UserIDFromContext`, `applicationID=path`. `creatorId` из service mock возвращается, проверяем что handler корректно сериализует.

#### `telegram/notifier_test.go` — patch для approve

- `TestNotifier_NotifyApplicationApproved`:
  - `posts exact text without parse mode or inline keyboard` — sender mock `EXPECT().SendMessage` с params, ассерт `params.Text == applicationApprovedText` + `ParseMode` пустой + `ReplyMarkup` пустой.
  - `sender error logged with chat id and op` — sender mock возвращает terminal-error (400), `log.Error` с `op=application_approved`. Retry уже из 18a, новых веток нет.

### E2E tests

#### `backend/e2e/creator_applications/approve_test.go`

Файл-комментарий на русском, нарратив (`backend-testing-e2e.md`).

Setup в test'е (без переиспользуемого testutil-helper — он будет в 18c):
- `testutil.SetupAdmin(t)`.
- Локальная функция `seedApprovableApplication(t)` (определена в этом же `_test.go`):
  - `testutil.SubmitCreatorApplication(t, ...)` (если есть; иначе `submitTestApplication`).
  - `testutil.LinkTelegramToApplication(t, appID)`.
  - `testutil.ManualVerifyApplicationSocial(t, adminToken, appID, socialID)` (если helper существует с chunk 16.5; иначе локальный wrapper).
  - Возвращает struct `{ApplicationID, AdminToken, ChatID, Categories, Socials, ...}` для assert'ов.

`t.Run`:
- `happy` — POST approve → 200 + непустой `creatorId` UUID-формата. Затем:
  - Admin GET application detail → `status=approved`.
  - `WaitForTelegramSent(t, fixture.ChatID, opts{ExpectCount: 1})` — 1 сообщение, текст совпадает с `applicationApprovedText`.
  - В cleanup-стеке `DeleteCreatorForTests(t, creatorId)` отрабатывает успешно (testapi вернёт 200/204) — это доказательство, что creator-row реально записан в БД (если бы row'а не было, `creatorRepo.DeleteForTests` вернул бы `sql.ErrNoRows` и testapi → 404/422).
  - **Полная structural-сверка** (поле-в-поле всех 30+ полей агрегата + verified-поля 1-в-1 в социалках + локализованные имена категорий) — **в 18c через `GET /creators/{id}` + `AssertCreatorAggregateMatchesSetup`**. В 18b мы сознательно ограничиваемся «факт записи + статус + telegram», структуру сверять нечем (нет API для чтения creator'а).
- `not_found` — random uuid → 404.
- `not_approvable_from_verification` — заявка ещё verification → 422. Cleanup в стеке только для заявки (creator не создавался).
- `not_approvable_repeat` — два POST подряд → второй 422. После теста — 1 creator в cleanup'е.
- `telegram_not_linked` — заявка без link, прокинутая SQL'ом в moderation → 422 TELEGRAM_NOT_LINKED. Cleanup в стеке только для заявки. (Дополнительная проверка отсутствия creator-row в БД формально требовала бы read-API; вместо этого полагаемся на тот факт, что service guard стоит ДО любых INSERT'ов в creators — это проверено unit-тестом на сервисном уровне.)
- `forbidden_brand_manager` → 403, без БД-изменений.
- `unauthenticated` → 401.
- `concurrent_approve_race` — два goroutine POST approve на одну заявку. Counter 200 == 1, 422 == 1. С `-race`. В cleanup — 1 creator (от выигравшего goroutine'а). Точный счёт transition-row откладывается на 18c (пока хватает функционального доказательства через counter ответов).

### Coverage gate

`make test-unit-backend-coverage` ≥ 80% per-method на новых функциях.

### Constants

Все codes / actions через exported константы. SQL-литералы только в repo-тестах (а repo здесь не правится).

### Race detector

`-race` обязателен. concurrent-test обязан проходить чисто.

## Verification

**Commands:**
- `make generate-api && make generate-mocks`
- `make build-backend lint-backend test-unit-backend-coverage test-e2e-backend`

**Manual smoke:** см. § Local Smoke Acceptance — 9 шагов.

## Spec Change Log

- **2026-05-05** — спека создана как 18b в декомпозиции chunk 18 (approve action). Status: `draft`. Зависит от 18a — pre-execution gate проставлен.
- **2026-05-05 (cleanup-pipeline)** — `CleanupEntityRequest.type=creator` enum extension + handler-case в testapi + `DeleteCreatorForTests` testutil-helper **переехали из 18c в 18b**. Причина: 18b всё равно нуждается в cleanup'е creator-row после happy-теста, а ad-hoc workaround (raw-SQL / temp testapi-endpoint) оставлял TBD-вопрос. Единый pipeline через `cleanup-entity` устраняет TBD и упрощает 18c (там эти примитивы только переиспользуются).
- **2026-05-05 (e2e factual-check)** — § Test Plan / E2E happy упрощён: вместо ветвистого «raw DB / temp testapi / new endpoint» factual-чек делается через **успешный `DeleteCreatorForTests(creatorId)` в cleanup-стеке** (cleanup получит `sql.ErrNoRows`, если row'а в БД нет — это автоматическое функциональное доказательство записи). Полная structural-сверка явно отложена на 18c.
- **2026-05-05 (telegram_not_linked clarification)** — для сценария `telegram_not_linked` отсутствие creator-row в БД формально не проверяется в e2e 18b (нет read-API), но гарантируется unit-тестом сервиса (guard `ErrCreatorApplicationTelegramNotLinked` стоит ДО любых INSERT'ов в `creators`). Дополнительный e2e-чек придёт в 18c.

</frozen-after-approval>
