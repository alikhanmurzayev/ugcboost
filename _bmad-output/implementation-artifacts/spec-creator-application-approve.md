---
title: "Approve заявки креатора и создание сущности креатора (бэк)"
type: feature
created: "2026-05-05"
status: draft
baseline_commit: "TBD — фиксируется после merge PR chunk 17"
context:
  - docs/standards/
  - _bmad-output/planning-artifacts/creator-onboarding-roadmap.md
  - _bmad-output/planning-artifacts/creator-application-state-machine.md
  - _bmad-output/implementation-artifacts/archive/2026-05-04-spec-creator-application-reject.md
---

> Перед реализацией обязательно полностью загрузить все файлы `docs/standards/` — это hard rules, спека не дублирует.

<frozen-after-approval reason="human-owned intent — do not modify unless human renegotiates">

## Intent

**Problem:** State-machine v2 (chunk 17) добавила терминал `approved` в enum, но переход в него не подключён ни в `creatorApplicationAllowedTransitions`, ни как admin-action. Pipeline останавливается на `moderation` — заявку, прошедшую модерацию, нечем закрыть; и сущности «креатор», на которой повиснут campaigns / payouts / audience-метрики, в БД ещё нет.

**Approach:** Один PR закрывает три связанные вещи:

1. **Admin-action `POST /creators/applications/{id}/approve`** — переход `moderation → approved`, в одной `dbutil.WithTx`: state-guard → telegram-link-guard → snapshot-read → `applyTransition` → INSERT в новую сущность `creators` + snapshot socials + snapshot categories → audit. Body пустой, response — `{ creatorId }` (E2E дёргает `GET creator` и проверяет агрегат).
2. **Сущность креатора в БД** — три новые таблицы (`creators` + `creator_socials` + `creator_categories`). Snapshot всех данных заявки на момент approve. Без row в `users` (см. § Decisions). Admin-агрегат `GET /creators/{id}` отдаёт всё одним запросом.
3. **Notifier retry-обёртка** — `Notifier.fire()` поднимается на `cenkalti/backoff/v5`, retryable классификация ошибок применяется ко всем `Notify*` методам (approve / reject / verification_approved / application_linked получают надёжный канал бесплатно). Текст approve — короткая константа в стиле существующих.

## Decisions

### Сущность креатора — отдельная ветка, не `users.role=creator`

`users` остаётся таблицей внутренней команды (admin / brand_manager) с email + password_hash. У креатора login'а нет — единственный канал связи Telegram-бот. Класть creator'а в `users` сейчас = nullable-rot для `email/password_hash` и преждевременная связка двух разных domain-областей.

Будущая привязка к `users` (если/когда у креатора появится login) — отдельной миграцией `creators.user_id UUID NULL FK`, без переноса данных.

### Snapshot из заявки

В `creators` / `creator_socials` / `creator_categories` копируется **всё**, что заявка имеет ценного для бренда / кампании:

- Идентичность + PII: `iin`, ФИО, `birth_date`, `phone`, `address`, `city_code`, `category_other_text`.
- Telegram плоско в той же строке: `telegram_user_id` + 3 nullable метаданных. Без отдельной таблицы — у креатора один Telegram, своего жизненного цикла link не имеет.
- **Все** соцсети (включая невёрнутые) с verified-полями (`verified` / `method` / `verified_by_user_id` / `verified_at`) — по 3 платформам `instagram` / `tiktok` / `threads`.
- Все категории через `category_code`.
- `source_application_id` UNIQUE — race-protect и ссылка на исходную заявку.

**Не переносим:** `verification_code`, 4 `creator_application_consents`, `creator_application_status_transitions`, `status` заявки, `linked_at` Telegram-link, `created_at/updated_at` заявки. Всё это артефакты процесса подачи; у креатора им места нет.

### Approve без Telegram-link → 422

Инвариант: одобренный креатор обязан иметь канал коммуникации. Если `creator_application_telegram_links` для заявки нет — service отказывает 422 `CREATOR_APPLICATION_TELEGRAM_NOT_LINKED` до любых INSERT'ов. Переиспользуется существующий sentinel `domain.ErrCreatorApplicationTelegramNotLinked` (используется в manual-verify, chunk 10) и его user-facing message — отдельного кода для approve не вводим.

### Approve-сообщение

Короткая статическая константа `applicationApprovedText` в стиле `verificationApprovedText` / `applicationRejectedText`. MVP-текст:

> Поздравляем! Ваша заявка одобрена ✅
>
> Скоро вернёмся с предложениями по сотрудничеству 🖤

Без HTML, без inline-keyboard, без переменных. Итерируется отдельным PR'ом заменой константы (как reject).

### Retry в notifier — общий слой, не локальный

`Notifier.fire()` оборачивает `sender.SendMessage` в `backoff.Retry` (`cenkalti/backoff/v5` — реестр в `backend-libraries.md`). Все 4 `Notify*` метода получают одну и ту же стратегию.

- Per-attempt timeout: `telegramNotifyTimeout` (10s, как сейчас).
- Max attempts: 4 (1 initial + 3 retry). Initial interval 1s, multiplier 2.0, max interval 8s → backoff'ы 1s/2s/4s. Max elapsed time 30s — hard cap на полное ожидание.
- Backoff чтит `ctx.Done()` — closer-shutdown прерывает sleep.
- Retryable: 5xx, 429, network errors (timeout, DNS, connection refused).
- Non-retryable (terminal): 4xx кроме 429 (bad request, forbidden — бот забанен, chat not found), `ctx.Err()`.
- Промежуточные неудачи — `log.Warn` с `attempt=N/4`. Финальная неудача — `log.Error`. Успех — без отдельного лога.
- Никаких метрик / observability-counter'ов — отдельным чанком когда появится stack.

### Authz сейчас — admin-only

`CanApproveCreatorApplication(ctx)` и `CanViewCreator(ctx)` — admin-only. Расширение для brand_manager'а (когда появится campaign-flow) — отдельным чанком.

### Никакого approval-блока в admin GET application detail

Reject-блок в detail-ответе оправдан тем, что отказ нужно показать модератору на фронте; approve достаточно состояния `status=approved` + (если фронту понадобится) отдельного `GET /creators/{id}` через `creatorId` из approve-response. Хочется простоты — не делаем зеркало `rejection`-блока.

## Boundaries & Constraints

**Always:**
- OpenAPI: `POST /creators/applications/{id}/approve` без `requestBody`, security `bearerAuth`, responses 200 / 401 / 403 / 404 / 422 / default. Path `id` — uuid. Response 200 — объект с одним полем — UUID созданного креатора.
- OpenAPI: `GET /creators/{id}` — admin agregate. Response 200 — большой объект с идентичностью (id, iin, sourceApplicationId), плоскими PII-полями (ФИО / birthDate / phone / address? / cityCode + cityName / categoryOtherText?), telegram-блок (user_id + 3 nullable метаданных), массив socials (платформа + handle + 4 verified-поля), массив categories (code + name через словарь). Hydration словарей на handler-уровне — паттерн как у admin GET application detail.
- 4 новых ErrorCode: `CREATOR_APPLICATION_NOT_APPROVABLE`, `CREATOR_ALREADY_EXISTS`, `CREATOR_TELEGRAM_ALREADY_TAKEN`, `CREATOR_NOT_FOUND`. Сценарий «нет Telegram-link» переиспользует существующий `CREATOR_APPLICATION_TELEGRAM_NOT_LINKED` + sentinel `ErrCreatorApplicationTelegramNotLinked` (сейчас стоит в manual-verify, chunk 10). Все user-facing message'и actionable (см. `backend-errors.md`); существующий маппинг в `handler/response.go:55` для `TelegramNotLinked` накрывает и approve без правок.
- Authz: `AuthzService.CanApproveCreatorApplication(ctx)` и `CanViewCreator(ctx)` — admin-only, шаблон как `CanRejectCreatorApplication` / `CanViewCreatorApplication`.
- Service: `(s *CreatorApplicationService) ApproveApplication(ctx, applicationID, actorUserID string) (creatorID string, err error)`. В одной `dbutil.WithTx`:
  1. `appRepo.GetByID` → `sql.ErrNoRows` ⇒ `ErrCreatorApplicationNotFound`; `app.Status != moderation` ⇒ `ErrCreatorApplicationNotApprovable`.
  2. `linkRepo.GetByApplicationID` → `sql.ErrNoRows` ⇒ `ErrCreatorApplicationTelegramNotLinked` (существующий sentinel, переиспользуется).
  3. `socialsRepo.ListByApplicationID(tx, applicationID)` + `categoriesRepo.ListByApplicationID(tx, applicationID)` — snapshot read.
  4. `applyTransition(tx, app, moderation → approved, actorID=&actor, reason=TransitionReasonApprove)`.
  5. `creatorRepo.Create(tx, compose(app, link))` → 23505 транслируется по `pgErr.ConstraintName` (см. ниже).
  6. `creatorSocialsRepo.InsertMany(tx, snapshotSocials)` + `creatorCategoriesRepo.InsertMany(tx, snapshotCategories)`.
  7. `auditRepo.Create(tx, AuditActionCreatorApplicationApprove, actor, entity={type:creator_application, id:applicationID}, metadata={creator_id, from_status:moderation, to_status:approved})`.
- Service: `(s *CreatorService) GetByID(ctx, creatorID) (*CreatorAggregate, error)` — `creatorRepo.GetByID` + bulk `socialsRepo.ListByCreatorID` + `categoriesRepo.ListByCreatorID` + dictionary lookup для cityName / categoryNames. `sql.ErrNoRows` ⇒ `ErrCreatorNotFound`.
- Notifier: новый метод `NotifyApplicationApproved(ctx, chatID)` рядом с reject/verification_approved. Дёргается ПОСЛЕ commit'а из `ApproveApplication` (не из callback'а), fire-and-forget. Шаблон как chunk 8.
- Service-side `creatorAppNotifier` интерфейс (`service/creator_application.go:60`) расширяется методом `NotifyApplicationApproved(ctx, chatID int64)` — без этого сервис физически не вызовет новый метод. После расширения — `make generate-mocks` обновит `service/mocks`.
- Notifier `fire()` — обёртка `backoff.Retry` (см. § Decisions / Retry). Все 4 `Notify*` метода получают одну стратегию.
- Domain: новые sentinel'ы `ErrCreatorApplicationNotApprovable` / `ErrCreatorAlreadyExists` / `ErrCreatorTelegramAlreadyTaken` / `ErrCreatorNotFound`. Каждый со своим actionable user-facing code и message. Sentinel «нет Telegram-link» — переиспользуется существующий `domain.ErrCreatorApplicationTelegramNotLinked` (отдельный sentinel не плодим).
- Domain: константа `TransitionReasonApprove = "approve_admin"`. Расширение `creatorApplicationAllowedTransitions` парой `(moderation → approved)` (chunk 17 этого не делал).
- Audit: `AuditActionCreatorApplicationApprove = "creator_application_approve"` в `audit_constants.go`.
- Migration: одна forward-only goose-миграция создаёт 3 таблицы. Никаких regex / length CHECK; платформа CHECK = `instagram | tiktok | threads` (зеркало `creator_application_socials`). Constraint names — экспортированные константы в repo (зеркало `CreatorApplicationTelegramLinksPK`):
  - `creators_iin_unique` ⇒ `ErrCreatorAlreadyExists`.
  - `creators_telegram_user_id_unique` ⇒ `ErrCreatorTelegramAlreadyTaken`.
  - `creators_source_application_id_unique` ⇒ `ErrCreatorApplicationNotApprovable` (race на одну заявку).
- Constraints на M2M: `creator_socials (creator_id, platform, handle)` UNIQUE; `creator_categories (creator_id, category_code)` UNIQUE; FK `creator_id` ON DELETE CASCADE; FK `verified_by_user_id` ON DELETE SET NULL (зеркало `creator_application_socials`).
- Repo: `CreatorRepo` с минимумом — `Create`, `GetByID`, `DeleteForTests`. `CreatorSocialRepo` с `InsertMany` + `ListByCreatorID` (детерминированный `ORDER BY platform, handle` — зеркало `creator_application_socials.go`). `CreatorCategoryRepo` с `InsertMany` + `ListByCreatorID` (`ORDER BY category_code`). Все три добавляются в `RepoFactory`. Детерминированный порядок — основа для `require.Equal` на агрегате целиком в E2E (см. § Test Plan / E2E). Stom-теги, sortColumns, паттерн зеркало `creator_application_*.go`.
- Handler: `ApproveCreatorApplication` (по operationId) + `GetCreator`. Маппинг новых sentinel'ов в `respondError`. Actor UUID — `middleware.UserIDFromContext(ctx)`, captured-input в handler-тестах.

**Ask First (BLOCKING до Execute):**
- (вопросы зарезолвлены инкрементальным дизайном; см. чат-протокол → `_bmad-output/planning-artifacts/creator-onboarding-roadmap.md` chunk 18)

**Never:**
- Создание row в `users` с `role=creator`.
- Перенос в `creators` `verification_code` / consents / status_transitions / `linked_at` Telegram-link.
- `approval`-блок в admin GET application detail (без зеркала `rejection`).
- Метрики / Prometheus / observability counter'ы.
- Бэкфил существующих approved-заявок (на новой state-machine их 0).
- Мутирующие endpoint'ы / методы на `creators` (только Create + GetByID + DeleteForTests).
- Удаление creator-row при последующем reject заявки (forward-only; reject из `approved` запрещён state-machine).
- Wider auth для `GET /creators/{id}` (admin-only сейчас).
- Inline-keyboard / HTML в approve-сообщении.
- Per-method retry-стратегия в каждом сервисе (retry поднят в `Notifier.fire()` один раз).
- Обновление `creators`-row при повторной заявке тем же IIN (UPDATE-flow — отдельный чанк, когда появится первый кейс).
- Удаление link `creator_application_telegram_links` после approve (link сохраняется по тому же контракту что и для reject — нужен для будущего broadcast / audit-trace).

## I/O & Edge-Case Matrix

> POST approve без body. GET creator без body. Все mutate-операции — actor через middleware.

| Сценарий | Состояние | Поведение |
|---|---|---|
| Happy approve (full fixture) | app `moderation`, telegram-link есть; заявка содержит `middle_name` + `address` + `category_other_text` + 3 соцсети (IG auto-verified через webhook chunk 8, TT manual-verified через chunk 10, Threads non-verified) | 200 `{creatorId}`; app.status=`approved`; transition row `(moderation → approved, actor, reason=approve_admin)`; audit row `creator_application_approve`; `GET /creators/{creatorId}` отдаёт агрегат, проходящий `testutil.AssertCreatorAggregateMatchesSetup` против исходной fixture (включая 3 соцсети с правильными verified-полями + 3 категории + плоский Telegram-блок); `WaitForTelegramSent` на `chat_id` из link фиксирует одно `applicationApprovedText` |
| Happy approve (sparse fixture) | то же что full, но `middle_name=nil`, `address=nil`, `category_other_text=nil`, одна соцсеть (IG auto-verified) | 200 `{creatorId}`; агрегат проходит helper с `null` в nullable-полях (а не пустыми строками) и одной соцсетью; остальные инварианты как в full-варианте |
| Application не существует | random UUID | 404 `CREATOR_APPLICATION_NOT_FOUND` |
| Wrong status | app в `verification` / `rejected` / `withdrawn` / `approved` | 422 `CREATOR_APPLICATION_NOT_APPROVABLE`, БД не изменилась |
| No telegram-link | app `moderation`, link отсутствует | 422 `CREATOR_APPLICATION_TELEGRAM_NOT_LINKED` (переиспользование существующего кода), БД не изменилась (включая отсутствие creator-row) |
| Concurrent approve одной заявки | два параллельных POST | один 200, второй 422 `CREATOR_APPLICATION_NOT_APPROVABLE` (23505 на `source_application_id_unique`) |
| Дубликат IIN | в `creators` уже есть row с этим IIN (теоретически из будущего re-application flow) | 422 `CREATOR_ALREADY_EXISTS` |
| Telegram занят | `telegram_user_id` уже у другого креатора | 422 `CREATOR_TELEGRAM_ALREADY_TAKEN` |
| Non-admin caller | brand_manager Bearer | 403 `FORBIDDEN` (до любого DB-вызова) |
| Unauthenticated | без Bearer | 401 |
| GET creator happy | creator существует | 200 — полный агрегат с PII / telegram-block / socials[] / categories[] |
| GET creator не существует | random UUID | 404 `CREATOR_NOT_FOUND` |
| GET creator non-admin | brand_manager Bearer | 403 |
| GET creator unauthenticated | без Bearer | 401 |
| Notify transient error | sender возвращает 5xx / network timeout | retry успешен в пределах 4 попыток → один user-facing message; `log.Warn` на промежуточных |
| Notify terminal error | sender возвращает 400 (bad request) | один вызов, нет retry, `log.Error` финальный |
| Notify все retry исчерпаны | sender падает 4 раза подряд | `log.Error` финальный, креатор уже approved'ом — manual broadcast unblock'ает позже |
| Notify ctx cancelled (shutdown) | closer dropping | retry-loop выходит, in-flight goroutine завершается через WaitGroup |

</frozen-after-approval>

## Code Map

> Baseline — TBD (зафиксируется после merge PR chunk 17 — спека `_bmad-output/implementation-artifacts/spec-creator-application-state-machine-v2.md`). До этого момента ниже могут быть конфликты с pending-changes в `creator_application.go` / `domain/creator_application.go`.

- `backend/migrations/<timestamp>_creators.sql` — новая миграция: 3 таблицы (`creators`, `creator_socials`, `creator_categories`) + UNIQUE / FK / индексы / CHECK на platform. Down-миграция — DROP в обратном порядке (нет данных в проде, бэкфила не было).
- `backend/api/openapi.yaml` —
  - Новый path `POST /creators/applications/{id}/approve`.
  - Новый path `GET /creators/{id}`.
  - Новые ErrorCode: 4 штуки (см. § Always). `CREATOR_APPLICATION_TELEGRAM_NOT_LINKED` уже существует — не дублируем.
  - Новая schema `CreatorAggregate` (response 200 на GET).
  - Новая schema response для approve (один UUID-поле).
  - После — `make generate-api`.
- `backend/internal/domain/creator.go` — новый файл: domain-типы `Creator` / `CreatorSocial` / `CreatorCategory` / `CreatorAggregate` + 4 новых sentinel'а + actionable user-facing codes/messages. `ErrCreatorApplicationTelegramNotLinked` остаётся в `creator_application.go` (используется approve через `errors.Is`).
- `backend/internal/domain/creator_application.go` — расширение `creatorApplicationAllowedTransitions` парой `(moderation → approved)`; константа `TransitionReasonApprove = "approve_admin"`.
- `backend/internal/service/audit_constants.go` — `AuditActionCreatorApplicationApprove`.
- `backend/internal/authz/creator_application.go` — `CanApproveCreatorApplication`.
- `backend/internal/authz/creator.go` — новый файл: `CanViewCreator`.
- `backend/internal/authz/creator_application_test.go` / `creator_test.go` — admin / brand_manager / no-role.
- `backend/internal/repository/creator.go` — новый: `CreatorRow`, `CreatorRepo` (`Create`, `GetByID`, `DeleteForTests`), 23505-switch по 3 constraint'ам, экспортированные `Creators*Unique` константы.
- `backend/internal/repository/creator_social.go` — новый: `CreatorSocialRow`, `CreatorSocialRepo` (`InsertMany`, `ListByCreatorID`).
- `backend/internal/repository/creator_category.go` — новый: `CreatorCategoryRow`, `CreatorCategoryRepo` (`InsertMany`, `ListByCreatorID`).
- `backend/internal/repository/repo_factory.go` — добавить 3 конструктора + 3 интерфейса в `*RepoFactory` соответствующих сервисов.
- `backend/internal/repository/creator_test.go` / `creator_social_test.go` / `creator_category_test.go` — pgxmock-tests (см. `backend-testing-unit.md` § Repository): SQL-asserts + 23505-маппинг для CreatorRepo + happy / not-found / error propagation.
- `backend/internal/service/creator_application.go` — `ApproveApplication`. Notify дёргается ПОСЛЕ `WithTx` (вне callback'а).
- `backend/internal/service/creator.go` — новый: `CreatorService` с `GetByID`. Hydration словарей через те же dictionary-репо что и `creator_applications`-detail.
- `backend/internal/service/creator_application_test.go` — `TestCreatorApplicationService_ApproveApplication` по матрице (см. § Test Plan).
- `backend/internal/service/creator_test.go` — `TestCreatorService_GetByID` (happy, not-found).
- `backend/internal/handler/creator_application.go` — `ApproveCreatorApplication`. Маппинг 4 sentinel'ов.
- `backend/internal/handler/creator.go` — новый: `GetCreator`. Маппинг `ErrCreatorNotFound` + dictionary hydration в response.
- `backend/internal/handler/response.go` — расширить маппинг `respondError` 4 новыми sentinel'ами. Существующий case `ErrCreatorApplicationTelegramNotLinked` (`response.go:55`) уже отдаёт 422 + actionable message и накрывает approve без изменений.
- `backend/internal/handler/creator_application_test.go` / `creator_test.go` — happy + error-mapping + captured actor + 401/403.
- `backend/internal/telegram/messages.go` — `applicationApprovedText`.
- `backend/internal/telegram/notifier.go` — `NotifyApplicationApproved` + retry-обёртка в `fire()` через `cenkalti/backoff/v5`. Классификация retryable / non-retryable ошибок.
- `backend/internal/service/creator_application.go:60` — расширение интерфейса `creatorAppNotifier` методом `NotifyApplicationApproved(ctx context.Context, chatID int64)`. После — `make generate-mocks`.
- `backend/internal/telegram/notifier_test.go` — добавить retry-сценарии: transient-success / terminal-no-retry / retry-exhausted / ctx-cancelled-breaks-loop.
- `backend/e2e/testutil/creator_application.go` — добавить struct `CreatorApplicationFixture` (контролируемые входные данные: ИИН, ФИО, `middleName *string`, `birthDate`, `phone`, `address *string`, `cityCode`, `categoryCodes []string`, `categoryOtherText *string`, `socials []SocialFixture` где каждая = platform + handle + verification-mode `auto-ig` / `manual` / `none`). Helper `SetupCreatorApplicationInModeration(t, in CreatorApplicationFixture) CreatorApplicationFixture` — прокидывает submit → link Telegram → auto-verify (IG через chunk 8 webhook) и/или manual-verify (chunk 10) согласно списку соцсетей, регистрирует cleanup, возвращает обогащённую fixture (`applicationID`, `telegramUserID`, `verifiedByAdminID` + `verifiedAt` для соответствующих соцсетей). Уникальные значения (IIN, telegram_user_id) — через существующие `crypto/rand`-helpers.
- `backend/e2e/testutil/creator.go` — новый. `AssertCreatorAggregateMatchesSetup(t, fx CreatorApplicationFixture, creatorID string, aggregate apiclient.CreatorAggregate)` — two-stage assertion: Stage 1 — динамические поля (`Id`, `SourceApplicationId`, `CreatedAt` / `UpdatedAt`, `socials[].Id` / `verifiedAt`) через `NotEmpty` + `WithinDuration`; Stage 2 — substitute динамических полей из actual в ожидаемый и `require.Equal` на агрегате целиком. Сортирует ожидаемые `socials` по `(platform, handle)` и `categories` по `code` перед сборкой `expected`. Helper `DeleteCreatorForTests(t, creatorID)` для cleanup-stack.
- `backend/api/openapi-test.yaml` + `backend/internal/testapi/...` — расширить `CleanupEntityRequestType` новым значением `creator` и добавить case в test-API delete-handler. Cleanup-stack для creator должен идти ДО удаления связанной заявки (FK `creators.source_application_id` без ON DELETE — RESTRICT по умолчанию).
- `backend/e2e/creator_applications/approve_test.go` — новый: `TestApproveCreatorApplication` по матрице. Использует `SetupCreatorApplicationInModeration` + `AssertCreatorAggregateMatchesSetup`.
- `backend/e2e/creators/get_test.go` — новый: `TestGetCreator`. Переиспользует тот же `AssertCreatorAggregateMatchesSetup` — ноль дублирования diff-логики между двумя файлами.
- `_bmad-output/planning-artifacts/creator-onboarding-roadmap.md` — chunk 18 → `[~]` при старте, `[x]` при merge.

## Tasks & Acceptance

**Pre-execution gates:**
- [ ] PR chunk 17 смержен в main; `baseline_commit` обновлён в frontmatter спеки.

**Execution:**
- [ ] Migration `creators` + `creator_socials` + `creator_categories` (1 файл) + manual smoke `make migrate-up`.
- [ ] OpenAPI: 2 path'а + 4 ErrorCode (TelegramNotLinked переиспользуется) + `CreatorAggregate` + approve-response. `make generate-api`.
- [ ] Domain: 4 новых sentinel'а + actionable codes/messages + `TransitionReasonApprove` + расширение transition map. Approve использует существующий `ErrCreatorApplicationTelegramNotLinked` для guard'а.
- [ ] Audit: action-константа.
- [ ] Authz: 2 новых метода + unit-тесты.
- [ ] Repository: `CreatorRepo` (Create / GetByID / DeleteForTests с 23505-switch) + `CreatorSocialRepo` + `CreatorCategoryRepo` + расширение `RepoFactory` + unit-тесты на каждый.
- [ ] Service: `ApproveApplication` + `CreatorService.GetByID` + unit-тесты по матрице (captured-input на applyTransition / Create / InsertMany / audit; mock notifier 0 EXPECT внутри callback'а — notify ПОСЛЕ commit).
- [ ] Handler: `ApproveCreatorApplication` + `GetCreator` + маппинг sentinel'ов + unit-тесты (captured actor + applicationID; 4 ошибки → коды; 403/401).
- [ ] Telegram: `applicationApprovedText` + `NotifyApplicationApproved` + расширение `creatorAppNotifier` интерфейса в сервисе + retry-обёртка в `fire()` + unit-тесты (transient / terminal / exhausted / ctx-cancelled). `make generate-mocks` после изменений интерфейса.
- [ ] E2E testutil: `CreatorApplicationFixture` + `SetupCreatorApplicationInModeration(t, fx)` + `AssertCreatorAggregateMatchesSetup(t, fx, creatorID, aggregate)` + `DeleteCreatorForTests` + расширение `CleanupEntityRequestType` в `openapi-test.yaml` и testapi-handler.
- [ ] E2E `approve_test.go` по матрице: `happy_full` + `happy_sparse` + 7 негативных + race-сценарий. Notify через `WaitForTelegramSent`.
- [ ] E2E `get_test.go` по матрице (happy через тот же helper + 3 негативных).
- [ ] Roadmap: chunk 18 → `[~]` (start) → `[x]` (merge).

**Acceptance Criteria:**
- Given admin Bearer, app в `moderation` с telegram-link, when `POST /creators/applications/{id}/approve`, then 200 `{creatorId}`; admin GET `/creators/{creatorId}` возвращает агрегат, который проходит `testutil.AssertCreatorAggregateMatchesSetup` против исходной fixture (identity / все PII-поля / telegram-блок / socials с verified-полями / categories с локализованными именами — структурное равенство одной проверкой); transition row `(moderation → approved, actor, reason=approve_admin)`; audit row `creator_application_approve` с metadata `{creator_id, from_status, to_status}`; `WaitForTelegramSent` ловит одно сообщение `applicationApprovedText` на `chat_id` из link.
- Given app в `verification` / `rejected` / `withdrawn` / `approved`, when call approve, then 422 `CREATOR_APPLICATION_NOT_APPROVABLE`, БД не изменилась.
- Given app в `moderation` без telegram-link, when call, then 422 `CREATOR_APPLICATION_TELEGRAM_NOT_LINKED`, никаких новых rows в `creators` / `creator_socials` / `creator_categories`.
- Given несуществующий applicationID, when call, then 404 `CREATOR_APPLICATION_NOT_FOUND`.
- Given brand_manager Bearer, when call approve, then 403 без обращений к БД.
- Given неавторизованный, when call, then 401.
- Given concurrent approve одной заявки, then ровно один 200 + ровно один 422 `CREATOR_APPLICATION_NOT_APPROVABLE`; в БД ровно одна creator-row + один transition row.
- Given creator существует, when admin GET `/creators/{id}`, then 200 + полный агрегат (см. выше).
- Given несуществующий creatorID, when admin GET, then 404 `CREATOR_NOT_FOUND`.
- Given brand_manager / unauthenticated, when GET creator, then 403 / 401.
- Given notifier transient error на первой попытке, when fire, then retry успешен в пределах max attempts → один user-facing message приходит; `log.Warn` на промежуточной попытке.
- Given notifier terminal 4xx, when fire, then ровно один вызов sender'а, `log.Error` финальный, retry не состоялся.
- Given все retry исчерпаны, when fire, then `log.Error` финальный, никаких лишних логов / panic'ов.
- `make generate-api build-backend lint-backend test-unit-backend-coverage test-e2e-backend` — все зелёные, per-method coverage ≥ 80%.

## Test Plan

> Производное от `docs/standards/backend-testing-unit.md`, `backend-testing-e2e.md`, `backend-constants.md`, `security.md`, `backend-errors.md`, `backend-libraries.md`. Перед стартом Execute сверить с актуальной версией стандартов.

### Validation rules (handler-level)

- Body нет на approve / GET — нечего валидировать.
- Path `id` — openapi-валидация uuid через ServerInterfaceWrapper, ручной парсинг запрещён (`backend-codegen.md`).
- Error-message actionable (`backend-errors.md`): «Заявку нельзя одобрить в текущем статусе. Допустимый статус — `moderation`.» / «Креатор с этим ИИН уже существует.» / «Этот Telegram уже привязан к другому креатору.» / «Креатор не найден.» Для `TELEGRAM_NOT_LINKED` — существующее «Креатор не привязал Telegram-бота — попросите его открыть бот по deep-link и повторите» (`handler/response.go:57`).

### Security invariants

Сверить перед merge (`security.md` § PII, § Length bounds, § Rate-limiting):
- **Никаких PII в stdout-логах.** В service / handler / notifier / repo разрешены: `application_id`, `creator_id`, `actor_id`, `chat_id` (число), `from_status`, `to_status`, `attempt`, `op`. Имена / IIN / handle / phone / address / Telegram-username — запрещены.
- **Нет PII в `error.Message`** — текст ошибок шаблонный.
- **Нет PII в URL** — uuid в path-параметрах.
- **Length bound** — body отсутствует, риск гигантского body отсутствует.
- **Rate-limiting** — admin-only endpoint'ы, не публичный вектор. Не закладываем.

### Unit tests

#### `authz/creator_application_test.go` / `creator_test.go`

`TestCanApproveCreatorApplication` / `TestCanViewCreator` — `t.Parallel()`. Шаблон как `CanRejectCreatorApplication`. `t.Run`: admin → nil; brand_manager → ErrForbidden; no role → ErrForbidden.

#### `repository/creator_test.go`

Pgxmock-tests (`backend-testing-unit.md` § Repository).

`TestCreatorRepository_Create` — happy (точная SQL + аргументы через capture, маппинг row → struct), 23505 на `creators_iin_unique` ⇒ `ErrCreatorAlreadyExists`, 23505 на `creators_telegram_user_id_unique` ⇒ `ErrCreatorTelegramAlreadyTaken`, 23505 на `creators_source_application_id_unique` ⇒ `ErrCreatorApplicationNotApprovable`, прочая БД-ошибка пробрасывается с обёрткой.

`TestCreatorRepository_GetByID` — happy (маппинг колонок), `sql.ErrNoRows` пробрасывается wrapped.

`TestCreatorRepository_DeleteForTests` — n=1 success, n=0 → `sql.ErrNoRows`.

#### `repository/creator_social_test.go`, `creator_category_test.go`

`InsertMany` — happy (точная SQL multi-row INSERT), empty input → no-op, БД-ошибка обёрнута. `ListByCreatorID` — happy (маппинг + ORDER BY), empty result.

#### `service/creator_application_test.go` — `TestCreatorApplicationService_ApproveApplication`

`t.Parallel()` на функции и каждом `t.Run`. Новый mock на каждый `t.Run` (`backend-testing-unit.md`). Notifier mock получает **0 EXPECT внутри callback'а** — mockery упадёт, если service случайно отправит уведомление до commit'а. После commit'а в одном `t.Run` (happy) — `EXPECT().NotifyApplicationApproved(...)` с captured `chat_id`.

Порядок `t.Run` повторяет порядок исполнения кода:

- `application not found` — `appRepo.GetByID` → `sql.ErrNoRows` → `errors.Is(err, ErrCreatorApplicationNotFound)`.
- `not approvable from verification` / `from rejected` / `from withdrawn` / `from approved` — table-driven; ассерт `errors.Is(err, ErrCreatorApplicationNotApprovable)` + БД-вызовов после lookup нет.
- `telegram link missing` — `linkRepo.GetByApplicationID` → `sql.ErrNoRows` → `errors.Is(err, ErrCreatorApplicationTelegramNotLinked)` + ни `applyTransition`, ни `creatorRepo.Create` не вызваны.
- `applyTransition error propagated` — `applyTransition` → ошибка → `ErrorContains` обёртки + `creatorRepo.Create` НЕ вызван.
- `creator already exists (race на iin)` — `creatorRepo.Create` → `ErrCreatorAlreadyExists` → пробрасывается.
- `telegram already taken` — `creatorRepo.Create` → `ErrCreatorTelegramAlreadyTaken`.
- `concurrent approve race` — `creatorRepo.Create` → `ErrCreatorApplicationNotApprovable` (23505 на source_application_id).
- `socials insert error propagated` — `creatorSocialsRepo.InsertMany` → ошибка → обёртка + audit НЕ вызван.
- `categories insert error propagated` — `creatorCategoriesRepo.InsertMany` → ошибка → обёртка + audit НЕ вызван.
- `audit error propagated` — все INSERT'ы ok, `auditRepo.Create` → ошибка → `ErrorContains` обёртки.
- `happy path` — captured-input на каждый repo-call: `applyTransition`(`fromStatus=moderation`, `toStatus=approved`, `actor=&adminID`, `reason=TransitionReasonApprove`); `creatorRepo.Create` (полный CreatorRow со всеми полями из app+link); `creatorSocialsRepo.InsertMany` (snapshot всех соцсетей с verified-полями 1-в-1); `creatorCategoriesRepo.InsertMany` (все category_code из заявки); `auditRepo.Create` (action / actor / entity / metadata через JSONEq). После commit — `notifier.NotifyApplicationApproved` дёрнут с правильным `chat_id`.

Captured-input через `mock.EXPECT().X(...).Run(func(args mock.Arguments) { ... })`. JSONEq для metadata (`backend-testing-unit.md` § Assertions).

#### `service/creator_test.go` — `TestCreatorService_GetByID`

`happy` — `creatorRepo.GetByID` ok; `socialsRepo.ListByCreatorID` ok; `categoriesRepo.ListByCreatorID` ok; dictionary lookup hydrated для cityName / categoryNames; output совпадает 1-в-1 (UUID / time подменяются после `WithinDuration`).

`not found` — `creatorRepo.GetByID` → `sql.ErrNoRows` → `errors.Is(err, ErrCreatorNotFound)`; ни socials, ни categories не запрошены.

#### `handler/creator_application_test.go` — `TestCreatorApplicationHandler_ApproveCreatorApplication`

`t.Parallel()`. Black-box через HTTP с `httptest`. Body отсутствует.

`t.Run`:
- `unauthenticated → 401` (без Bearer).
- `forbidden non-admin → 403` (brand_manager).
- `application not found → 404 CREATOR_APPLICATION_NOT_FOUND`.
- `not approvable → 422 CREATOR_APPLICATION_NOT_APPROVABLE`.
- `telegram not linked → 422 CREATOR_APPLICATION_TELEGRAM_NOT_LINKED`.
- `creator already exists → 422 CREATOR_ALREADY_EXISTS`.
- `telegram already taken → 422 CREATOR_TELEGRAM_ALREADY_TAKEN`.
- `happy → 200 {creatorId}` + captured-input на service: `actor=UserIDFromContext`, `applicationID=path`. Захват через `mock.Run`, ассерт через `require.Equal`.

Middleware-derived поле — actor UUID из `middleware.UserIDFromContext`. Captured-input в test обязательно (`backend-testing-unit.md` § Handler).

#### `handler/creator_test.go` — `TestCreatorHandler_GetCreator`

`unauthenticated → 401`, `non-admin → 403`, `not found → 404 CREATOR_NOT_FOUND`, `happy → 200 + полный CreatorAggregate в response` с captured creatorID.

#### `telegram/notifier_test.go` — retry-сценарии

Поверх существующих happy-path'ов добавить:

- `transient_5xx_then_success` — sender mock возвращает 503 на первой попытке, 200 на второй; ровно 2 вызова `SendMessage`, `log.Warn` на attempt=1, без финального Error.
- `transient_429_then_success` — то же, но 429.
- `network_error_then_success` — `net.OpError` / dial timeout.
- `terminal_400_no_retry` — sender mock возвращает 400; ровно 1 вызов `SendMessage`, `log.Error` финальный.
- `terminal_403_no_retry` — sender mock возвращает 403 (бот забанен).
- `retry_exhausted` — sender mock возвращает 503 4 раза подряд; ровно 4 вызова, 3 Warn-лога + 1 финальный Error.
- `ctx_cancelled_breaks_retry` — после первой неудачи отменить ctx; retry-loop выходит, `log.Error` с `ctx canceled`.
- `panic_in_sender_recovered` — паника не должна падать процесс (existing test расширить retry-обёрткой).

Backoff-таймауты в тестах сжимаются до миллисекунд через DI (Notifier принимает `backoff.BackOff` или DI-тестируемый `clock` — паттерн как с `telegramNotifyTimeout`).

### E2E tests

#### `backend/e2e/creator_applications/approve_test.go`

Файл-комментарий на русском, нарратив (`backend-testing-e2e.md`). Заголовок: `// Package creator_applications — E2E тесты HTTP-поверхности /creators/applications/{id}/approve.` + по абзацу на каждый Test* + setup/cleanup абзац.

Сгенерированный клиент (`apiclient` + `testclient`), импорт `internal/` запрещён. `t.Parallel()` на `TestApproveCreatorApplication`. Cleanup через `testutil.Cleanup` + env `E2E_CLEANUP`.

Setup: `testutil.SetupAdmin` + `testutil.SetupCreatorApplicationInModeration(t, fixture)` (заявка с контролируемыми значениями + link Telegram + auto-/manual-verify для перечисленных соцсетей). Hardcoded дат не использовать (`birth_date` через `time.Now().AddDate(-25, 0, 0)`).

`t.Run` по матрице:
- `happy_full` — fixture с заполненным `middle_name` + `address` + `category_other_text` + 3 соцсети (IG auto / TT manual / Threads non-verified). POST approve → 200 `{creatorId}`. Admin GET application detail → `status=approved`. **`testutil.AssertCreatorAggregateMatchesSetup(t, fixture, creatorID, aggregate)`** против `GET /creators/{creatorId}` — структурное совпадение всех полей агрегата (PII + telegram-блок + 3 соцсети с verified-полями + 3 категории) одной проверкой; никаких поле-в-поле `require.Equal` на 30 строк. `testutil.WaitForTelegramSent(t, fixture.TelegramUserID, opts{ExpectCount: 1, Timeout: 10*time.Second})` ловит `applicationApprovedText`.
- `happy_sparse` — fixture с `middleName=nil`, `address=nil`, `categoryOtherText=nil`, одна IG-соцсеть (auto-verified). Тот же helper — ловит nullable как `nil`, не пустые строки. Защищает omitempty-инвариант openapi-схемы.
- `not_found` — random uuid → 404 `CREATOR_APPLICATION_NOT_FOUND`.
- `not_approvable_from_verification` — заявка ещё в `verification` → 422 `CREATOR_APPLICATION_NOT_APPROVABLE`. БД не изменилась (в `creators` пусто).
- `not_approvable_repeat` — два POST подряд → второй 422 `CREATOR_APPLICATION_NOT_APPROVABLE`, ровно одна creator-row.
- `telegram_not_linked` — fixture без `LinkTelegramToApplication` → 422 `CREATOR_APPLICATION_TELEGRAM_NOT_LINKED`, в `creators` / `creator_socials` / `creator_categories` ничего не появилось.
- `forbidden_brand_manager`, `unauthenticated` — 403 / 401 без обращений к БД.
- `concurrent_approve_race` — два goroutine-а одновременно POST approve на одну заявку → счётчик 200 == 1, счётчик 422 == 1, в БД ровно одна creator-row + одна transition-row (UNIQUE на `creators.source_application_id` ловит второго на `Create`, весь TX2 откатывается атомарно). С `-race`.

#### `backend/e2e/creators/get_test.go`

`TestGetCreator` — `t.Parallel()` + cleanup. Файл-комментарий на русском, нарратив. Setup: full-fixture через `SetupCreatorApplicationInModeration` + admin approve (полный pipeline `submit → link → verify → approve`).

`t.Run`:
- `happy` — `GET /creators/{creatorId}` → `AssertCreatorAggregateMatchesSetup` (тот же helper что в `approve_test.go`, ноль дублирования diff-логики).
- `not_found` — random uuid → 404 `CREATOR_NOT_FOUND`.
- `forbidden_brand_manager` — brand_manager Bearer → 403.
- `unauthenticated` — без Bearer → 401.

### Coverage gate

`make test-unit-backend-coverage` — per-method ≥ 80% на новых функциях (`backend-testing-unit.md` § Coverage). Покрываемые пакеты: `handler/`, `service/`, `repository/`, `authz/`, `telegram/` (если `internal/telegram/` уже включён в фильтр; иначе чанк не расширяет правила).

### Generated/codegen invariants

`*.gen.go` файлы меняются ТОЛЬКО через `make generate-api` после правки `openapi.yaml`. `mockery` с `all: true` после добавления интерфейсов — `make generate-mocks`.

### Constants

Все enum-значения через константы (`backend-constants.md`): `domain.TransitionReasonApprove`, `service.AuditActionCreatorApplicationApprove`, `repository.Creators*Unique`, `domain.SocialPlatformValues` (зеркало). Литералы запрещены, кроме SQL-литералов в repo-тестах.

### Race detector

`-race` обязателен в `make test-unit-backend` и `make test-e2e-backend` (Makefile уже включает). Concurrent approve race-тест и notifier retry-тесты обязаны проходить с `-race`.

## Verification

**Commands:**
- `make generate-api`
- `make generate-mocks`
- `make build-backend lint-backend test-unit-backend-coverage test-e2e-backend`

**Manual smoke (локально):**
- `make compose-up && make migrate-up`. Прогнать pipeline: подать заявку через лендос → привязать Telegram → auto-verify (через chunk 8) или manual verify (chunk 10) → `curl -X POST -H "Authorization: Bearer $ADMIN_TOKEN" http://localhost:8082/creators/applications/<id>/approve` → 200 `{creatorId}`. `curl -H "Authorization: Bearer $ADMIN_TOKEN" http://localhost:8082/creators/<creatorId>` → агрегат. В Telegram у тестового аккаунта приходит `applicationApprovedText`. В БД: `creator_applications.status=approved`, transition row есть, audit row есть, новые rows в `creators` / `creator_socials` / `creator_categories`.

## Spec Change Log

- **2026-05-05** — спека создана инкрементальным дизайном (картина мира v0 → v7 в чате). Status: `draft`. baseline_commit зафиксируется после merge PR chunk 17.
- **2026-05-05 (e2e hardening)** — четыре правки по запросу про «надёжные E2E с полной проверкой агрегата»:
  1. § Always: `ListByCreatorID` в `CreatorSocialRepo` / `CreatorCategoryRepo` получили детерминированный `ORDER BY (platform, handle)` / `ORDER BY category_code` — фундамент для structural-equal на агрегате.
  2. Code Map: новые testutil-структуры `CreatorApplicationFixture` + `SetupCreatorApplicationInModeration(t, fx)` + `AssertCreatorAggregateMatchesSetup(t, fx, creatorID, aggregate)` + `DeleteCreatorForTests` + расширение `CleanupEntityRequestType` (`openapi-test.yaml` + testapi). Two-stage assertion: `WithinDuration` / `NotEmpty` на динамических полях, потом `require.Equal` на агрегат целиком.
  3. § I/O matrix + Test Plan: `happy` разбит на `happy_full` (полная заявка с 3 соцсетями: IG auto / TT manual / Threads non-verified, всеми nullable заполненными) и `happy_sparse` (все nullable=nil, 1 соцсеть). Защищает omitempty + покрывает копирование verification-полей.
  4. `get_creator_test.go` переиспользует `AssertCreatorAggregateMatchesSetup` — ноль дублирования diff-логики между двумя файлами.
- **2026-05-05 (audit)** — две правки по результату аудита спеки против текущего кода:
  1. Sentinel/code для «нет Telegram-link» — убраны запланированные `ErrCreatorApplicationTelegramRequired` / `CREATOR_APPLICATION_TELEGRAM_REQUIRED`; используется существующий `domain.ErrCreatorApplicationTelegramNotLinked` / `CREATOR_APPLICATION_TELEGRAM_NOT_LINKED` (стоит в manual-verify, chunk 10) + готовый маппинг в `handler/response.go:55`. Минус один новый код, минус один sentinel.
  2. Code Map дополнен пунктом расширения интерфейса `creatorAppNotifier` в `service/creator_application.go:60` методом `NotifyApplicationApproved` — без этого сервис физически не вызовет нотификатор.

  Acceptance Criteria по race-сценарию **подтверждён как корректный**: UNIQUE на `creators.source_application_id` ловит второй concurrent TX на `Create`, весь TX откатывается атомарно (включая INSERT в `creator_application_status_transitions`), в БД остаётся ровно одна creator-row + одна transition-row. Зеркаление reject'овской снятой проверки оказалось ложным — в reject не было UNIQUE-protect'а, здесь есть.
