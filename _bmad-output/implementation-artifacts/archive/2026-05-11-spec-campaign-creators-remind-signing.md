---
title: 'Ремайндер креаторам в статусе «подписывает договор»'
type: feature
created: '2026-05-11'
status: in-review
baseline_commit: 4d6e12ebdbdaef4b92c7dab6ad280926c9a53472
context:
  - docs/standards/backend-architecture.md
  - docs/standards/backend-codegen.md
  - docs/standards/backend-errors.md
  - docs/standards/backend-repository.md
  - docs/standards/backend-transactions.md
  - docs/standards/backend-testing-unit.md
  - docs/standards/backend-testing-e2e.md
  - docs/standards/frontend-api.md
  - docs/standards/frontend-components.md
  - docs/standards/frontend-testing-unit.md
  - docs/standards/frontend-testing-e2e.md
  - docs/standards/naming.md
  - docs/standards/security.md
---

<frozen-after-approval reason="human-owned intent — do not modify unless human renegotiates">

## Intent

**Problem:** Некоторые креаторы зависают в статусе `signing` — контракт через TrustMe отправлен, СМС-ссылка пришла, но подпись не приходит и креатор молчит. У админа нет рычага напомнить — кнопки-ремайндеры есть только для `planned`/`declined` (notify) и `invited` (remind-invitation). Группа `signing` на странице кампании показывается без действия.

**Approach:** Третий ремайндер по симметрии с `remind-invitation`. Новый эндпоинт `POST /campaigns/{id}/remind-signing` с новой `batchOpSpec` (`allowedStatuses={signing}`, `requireContractTemplate=false`) — переиспользует существующий пайплайн `dispatchBatch` / `applyDelivered`, инкрементирует `reminded_count` и `reminded_at` через тот же `repo.ApplyRemind` без смены статуса. Новый константный текст в `telegram` notifier (короткое напоминание про СМС-ссылку TrustMe с контактом `@aizerealzair`). Фронт: третья мутация в `useCampaignNotifyMutations`, новый case `SIGNING` в `actionForStatus`, i18n-ключи `remindSigningButton` / `remindSigningSubmitting`. Миграции БД не нужны.

## Boundaries & Constraints

**Always:**
- Новый path `/campaigns/{id}/remind-signing` использует existing schemas `CampaignCreatorBatchInput` и `CampaignNotifyResult` — без новых API-структур.
- Только статус `signing` — допустим. Любой другой статус (planned/invited/agreed/signed/signing_declined/declined) → 422 `CAMPAIGN_CREATOR_BATCH_INVALID` с `reason=wrong_status` и `current_status` заполнен.
- Статус строки **не меняется**. Меняются только `reminded_count` (+1) и `reminded_at` (now), `updated_at` (now). `invited_count` и `decided_at` остаются как есть.
- Audit-row пишется в той же per-creator транзакции с action `campaign_creator_remind_signing`.
- Telegram-доставка через тот же метод `notifier.SendCampaignInvite(ctx, chatID, text, tmaURL)` с **новым** текстом `CampaignRemindSigningText()`. WebApp-кнопка «Посмотреть» в TMA сохраняется.
- Partial-success семантика идентична `remind-invitation`: failed delivery → `undelivered[]` с `reason`, остальной батч продолжает; persist-failure после успешного send → `undelivered` reason `unknown` + error в log.
- Frontend кнопка disabled при `isPending` + external `isSubmitting` flag сбрасывается в `onSettled` (по `frontend-state.md`).
- `data-testid` для новой кнопки **уже** есть через существующий шаблон `CampaignCreatorGroupSection.tsx:153` — `campaign-creators-group-action-${status}`. Для signing — `campaign-creators-group-action-signing`. Ручного testid в новом коде не добавляем.

**Never:**
- НЕ менять `repo.ApplyRemind` (он уже подходит — partial UPDATE без status).
- НЕ менять `dispatchBatch` / `applyDelivered` — параметризация через `batchOpSpec` уже есть.
- НЕ добавлять миграции БД (все колонки и enum-значения уже есть).
- НЕ менять `requireContractTemplate` для остальных batchOpSpec.
- НЕ переименовывать `AuditActionCampaignCreatorRemind` (он остаётся за `remind-invitation`).
- НЕ создавать новый метод в `CampaignInviteNotifier` интерфейсе — переиспользуем `SendCampaignInvite`.
- НЕ менять `CampaignCreatorBatchInput` / `CampaignNotifyResult` / `CampaignCreatorBatchInvalidErrorResponse` — переиспользуем.
- НЕ выставлять кнопку для `agreed` / `signed` / `signing_declined`.
- НЕ редактировать существующие миграции in-place.

## I/O & Edge-Case Matrix

| Сценарий | Input / State | Expected | Error Handling |
|---|---|---|---|
| Happy path single | creatorId в `signing`, валидный chatID, бот доступен | 200, `undelivered=[]`, `reminded_count`+1, `reminded_at`=now, status=`signing`, audit `campaign_creator_remind_signing`, Telegram message с `CampaignRemindSigningText()` | — |
| Happy path batch | 5 creatorIds, все в `signing` | 200, `undelivered=[]`, 5 audit-rows, 5 send-вызовов, по +1 к каждому `reminded_count` | per-creator tx |
| Повторный ремайндер | тот же creator, `reminded_count`=N | 200, `reminded_count`=N+1, новый audit-row, новый `reminded_at` | — |
| Wrong status (planned/invited/agreed/signed/signing_declined/declined) | batch содержит creator не в `signing` | 422 `CAMPAIGN_CREATOR_BATCH_INVALID`, `details` с `reason=wrong_status` и `current_status` для каждого; **ничего** не отправлено, БД не тронута | strict-422 до send-loop |
| Not in campaign | creatorId не привязан к кампании | 422 с `reason=not_in_campaign` | strict-422 |
| Mixed: 2 valid + 1 wrong | один creator не в `signing` | 422 на весь батч, ни один не получает сообщение | whole-batch validation |
| Soft-deleted campaign | id кампании is_deleted=true | 404 `ErrCampaignNotFound` | `getActiveCampaign` гейт |
| Missing campaign | id кампании не существует | 404 `ErrCampaignNotFound` | `sql.ErrNoRows` → 404 |
| Bot blocked | Telegram возвращает `bot_blocked` | 200, `undelivered=[{creatorId, reason: bot_blocked}]`, `ApplyRemind` **не** вызван для этого creator | `MapTelegramErrorToReason` |
| Unknown telegram error | Telegram возвращает unexpected error | 200, `undelivered` с reason `unknown`, error в `logger.Warn` | — |
| Missing telegram_user_id | creator hard-deleted между validate и send | 200, `undelivered` с reason `unknown`, error в `logger.Error`, цикл продолжает | invariant breach branch |
| Send OK + persist fail | `ApplyRemind` падает после успешного send | 200, `undelivered` reason `unknown`, error в `logger.Error`, остальной батч продолжает | per-creator tx изолирована |
| Duplicate creatorIds в батче | `[id1, id1, id2]` | 422 ErrorResponse, `code=CAMPAIGN_CREATOR_IDS_DUPLICATES` | handler-level `validateCampaignCreatorBatch` (campaign_creator.go:135-153) |
| Пустой батч | `creatorIds=[]` | 422 ErrorResponse, `code=CAMPAIGN_CREATOR_IDS_REQUIRED` | handler-level `validateCampaignCreatorBatch` |
| > 200 creatorIds | `creatorIds.length > 200` | 422 ErrorResponse, `code=CAMPAIGN_CREATOR_IDS_TOO_MANY` | handler-level `validateCampaignCreatorBatch` |
| Unauthenticated | без access-token | 401 | bearer middleware |
| Non-admin | пользователь без admin-роли | 403 | AuthzService |
| Frontend: повторный клик | админ дважды кликает кнопку | second click no-op (disabled + isSubmitting flag) | UI-guard |

</frozen-after-approval>

## Code Map

### Backend

- `backend/api/openapi.yaml` — новый path `/campaigns/{id}/remind-signing` (POST, `operationId: remindCampaignCreatorsSigning`), копия структуры `/campaigns/{id}/remind-invitation`. Request `CampaignCreatorBatchInput`, responses 200 `CampaignNotifyResult` / 401 / 403 / 404 / 422 `oneOf(CampaignCreatorBatchInvalidErrorResponse, ErrorResponse)`. Description явно говорит про симметрию и что статус не меняется. Никаких новых components/schemas.
- `backend/internal/service/audit_constants.go:25` — добавить `AuditActionCampaignCreatorRemindSigning = "campaign_creator_remind_signing"` рядом с существующей `AuditActionCampaignCreatorRemind`.
- `backend/internal/telegram/notifier.go:95` — добавить константу `campaignRemindSigningText` внутри того же `const (...)` блока, где сейчас `campaignInviteText` / `campaignRemindInvitationText`; экспортируемый геттер `CampaignRemindSigningText() string` рядом с `CampaignContractSentText()` (notifier.go:164). **Не забывать про комментарий-инвариант** на notifier.go:91-94: литерал зеркалится в `backend/e2e/campaign_creator/campaign_notify_test.go:62-68`, и при добавлении нового надо обновить mirror в том же PR — иначе e2e `waitInviteSent` зависнет.
- `backend/internal/service/campaign_creator.go:195` — новая константа `batchOpRemindSigning batchOp = "remind_signing"` рядом с существующими `batchOpNotify` / `batchOpRemindInvitation`.
- `backend/internal/service/campaign_creator.go:210` — новая запись в `batchOpSpecs`: копия `batchOpRemindInvitation` с заменой `allowedStatuses` на `{domain.CampaignCreatorStatusSigning: true}`, `auditAction` на новую `AuditActionCampaignCreatorRemindSigning`, `text` на `telegram.CampaignRemindSigningText()`. `apply` — `r.ApplyRemind(ctx, id)` (переиспользуем существующий метод репозитория — он уже UPDATE'ит `reminded_count`+1, `reminded_at`=now, `updated_at`=now без смены статуса). `requireContractTemplate` опущено (false по zero-value).
- `backend/internal/service/campaign_creator.go:255` — новый публичный метод `RemindSigning(ctx context.Context, campaignID string, creatorIDs []string) ([]domain.NotifyFailure, error)` рядом с `RemindInvitation` — однострочный wrapper `s.dispatchBatch(ctx, campaignID, creatorIDs, batchOpRemindSigning)`. Godoc-комментарий на русском, описывает симметрию с `RemindInvitation` и отличие только в allowed source status и audit action.
- `backend/internal/handler/campaign_creator.go:114` — новый метод `RemindCampaignCreatorsSigning(ctx, request)`, копия `RemindCampaignCreatorsInvitation`. Тот же `validateCampaignCreatorBatch` хелпер (он же enforces empty/≤200/no-duplicates с тремя 422-кодами: `CAMPAIGN_CREATOR_IDS_REQUIRED` / `CAMPAIGN_CREATOR_IDS_DUPLICATES` / `CAMPAIGN_CREATOR_IDS_TOO_MANY`). Тот же `domainNotifyFailuresToAPI` для маппинга response. Делегат — `s.campaignCreatorService.RemindSigning(...)`. Authz-вызов — новый `s.authzService.CanRemindCampaignCreatorsSigning(ctx)` (см. ниже).
- `backend/internal/handler/server.go:122-128` — расширить интерфейс `CampaignCreatorService` новым методом `RemindSigning(ctx context.Context, campaignID string, creatorIDs []string) ([]domain.NotifyFailure, error)`. Обновить godoc интерфейса.
- `backend/internal/handler/server.go:66` — расширить интерфейс `AuthzService` новым методом `CanRemindCampaignCreatorsSigning(ctx context.Context) error`.
- `backend/internal/authz/campaign_creator.go:50` — добавить новый метод `CanRemindCampaignCreatorsSigning(ctx context.Context) error` рядом с `CanRemindCampaignCreators`. Реализация идентична: admin-only, otherwise `domain.ErrForbidden`. Комментарий — про gate `POST /campaigns/{id}/remind-signing`, симметрия с `CanRemindCampaignCreators`. **Не** переименовываем существующий — пусть остаётся за `remind-invitation`, поведение per-endpoint.
- `backend/internal/{service,handler,authz}/mocks/` — после правки интерфейсов запустить `make generate-mocks`.

### Frontend (web)

- `frontend/web/src/api/campaignCreators.ts:123` — новая функция `remindCampaignCreatorsSigning(campaignId, creatorIds)`, копия `remindCampaignCreatorsInvitation` с заменой path на `/campaigns/{id}/remind-signing` (через openapi-fetch — `client.POST("/campaigns/{id}/remind-signing", ...)`). Возвращает `CampaignNotifyResult` (тот же тип). Обработка ошибок через `extractErrorParts` + `ApiError` — как у соседних функций.
- `frontend/web/src/features/campaigns/creators/hooks/useCampaignNotifyMutations.ts` — расширить:
  - Интерфейс `CampaignNotifyMutations` (useCampaignNotifyMutations.ts:9-12) — добавить поле `remindSigning: UseMutationResult<CampaignNotifyResult, ApiError, string[]>`.
  - Тело хука (useCampaignNotifyMutations.ts:14-35) — добавить третью мутацию `remindSigning` рядом с `remind`, тот же `noopOnError`, `mutationFn` вызывает `remindCampaignCreatorsSigning(campaignId, creatorIds)`.
  - Импорт новой API-функции из `@/api/campaignCreators`.
- `frontend/web/src/features/campaigns/creators/CampaignCreatorsSection.tsx`:
  - Сигнатура `actionForStatus()` (CampaignCreatorsSection.tsx:301-311) — union `mutation?:` (строки 308-310) расширить: добавить `| CampaignNotifyMutations["remindSigning"]`.
  - Тело switch (CampaignCreatorsSection.tsx:312-335) — вынести `SIGNING` из общего case с `AGREED`/`SIGNED`/`SIGNING_DECLINED` в отдельный case:
    ```ts
    case CAMPAIGN_CREATOR_STATUS.SIGNING:
      return {
        actionLabel: t("campaignCreators.remindSigningButton"),
        actionSubmittingLabel: t("campaignCreators.remindSigningSubmitting"),
        mutation: mutations.remindSigning,
      };
    ```
  - Оставшийся case `AGREED`/`SIGNED`/`SIGNING_DECLINED` → `{}` без `mutation` (никакого ремайндера для них).
  - `handleGroupSubmit` (CampaignCreatorsSection.tsx:177-207) и render-цикл `CAMPAIGN_CREATOR_GROUP_ORDER.map(...)` (CampaignCreatorsSection.tsx:246) **трогать не нужно** — они уже работают через `actionForStatus` и поддерживают любую группу с `mutation`. Query-invalidation через `campaignCreatorKeys.list` уже на месте в `onSettled`.
- `frontend/web/src/shared/i18n/locales/ru/campaigns.json:97` — в секцию `campaignCreators` добавить, сразу после ключей `remindButton` / `remindSubmitting`:
  ```json
  "remindSigningButton": "Разослать ремайндер",
  "remindSigningSubmitting": "Отправка…",
  ```
  Ключи `groups.signing` (campaigns.json:103) и `currentStatus.signing` (campaigns.json:117) **уже есть** — переиспользуем без изменений.
- `frontend/web/src/api/generated/schema.ts` — регенерируется через `make generate-api`, **руками не редактировать**.

### Tests (backend)

- `backend/internal/service/campaign_creator_test.go` — новый `TestCampaignCreatorService_RemindSigning` (см. подробности в Tasks).
- `backend/internal/handler/campaign_creator_test.go` — новый `TestCampaignCreatorHandler_RemindSigning`.
- `backend/internal/authz/campaign_creator_test.go:102` — новый `TestAuthzService_CanRemindCampaignCreatorsSigning` рядом с `TestAuthzService_CanRemindCampaignCreators`. Те же три кейса: brand-manager / unauthenticated / admin.
- `backend/e2e/campaign_creator/campaign_notify_test.go` — расширить:
  - Литералы (campaign_notify_test.go:62-68) — добавить `chunk12RemindSigningText` рядом с `chunk12InviteText` / `chunk12RemindText` (точное зеркало `campaignRemindSigningText` из notifier).
  - Новый `TestRemindCampaignCreatorsSigning` (top-level `Test*` с `t.Parallel()`) рядом с existing `TestRemindCampaignCreatorsInvitation`. Setup через готовый `testutil.SetupCampaignWithSigningCreator` (testutil/contract_setup.go:92) — он доводит креатора до `signing` через A4 notify → TMA agree → outbox-worker TrustMe Phase 3.
  - Для partial-success теста нужны TWO creators в `signing` в одной кампании: текущий `SetupCampaignWithSigningCreator` создаёт **одного**. Опции (выбирает реализатор по простоте): (a) вызвать setup второй раз и кампании будут разные → разделить partial-success-тест на две кампании, либо (b) добавить второго creator вручную в test-body через `addCreatorToCampaign` + TMA agree + outbox-tick (по образцу existing helper). Не плодить universal `SetupCampaignWithSigningCreators` (множ.) без явной потребности больше чем тут.

### Tests (frontend)

- `frontend/web/src/features/campaigns/creators/hooks/useCampaignNotifyMutations.test.ts` — расширить кейсами `remindSigning`.
- `frontend/web/src/features/campaigns/creators/CampaignCreatorsSection.test.tsx` — добавить кейс для группы `signing`.
- `frontend/e2e/web/admin-campaign-creators-trustme.spec.ts` (или подходящий spec, охватывающий signing-flow) — добавить шаг с remind-signing.

## Tasks & Acceptance

**Execution:**

Backend контракт и кодоген:
- [x] OpenAPI: новый path `/campaigns/{id}/remind-signing` (POST). `operationId: remindCampaignCreatorsSigning`. Симметрия с `remind-invitation`: те же schemas, те же responses, новый description. `tags: [campaigns]`, `security: bearerAuth: []`.
- [x] `make generate-api` — обновить `backend/internal/api/server.gen.go`, `backend/e2e/apiclient/*.gen.go`, `frontend/web/src/api/generated/schema.ts`, `frontend/e2e/types/schema.ts`.

Backend код:
- [x] `service/audit_constants.go`: добавить `AuditActionCampaignCreatorRemindSigning = "campaign_creator_remind_signing"` рядом с `AuditActionCampaignCreatorRemind`.
- [x] `telegram/notifier.go`: внутри const-блока (~notifier.go:95-102) добавить `campaignRemindSigningText`:
  ```
  Напоминаем, что мы отправили вам соглашение на подпись по СМС на номер
  телефона, указанный при регистрации.

  Перейдите по ссылке из СМС и подпишите соглашение.

  Если есть вопросы, можете обратиться к @aizerealzair
  ```
- [x] `telegram/notifier.go`: экспортируемый геттер `CampaignRemindSigningText() string` рядом с `CampaignContractSentText()` (~notifier.go:164). Обновить header-комментарий const-блока (notifier.go:91-94), упомянуть, что зеркало в e2e теперь включает `chunk12RemindSigningText`.
- [x] `service/campaign_creator.go:195`: добавить `batchOpRemindSigning batchOp = "remind_signing"` в const-блок рядом с `batchOpNotify` / `batchOpRemindInvitation`.
- [x] `service/campaign_creator.go:210`: добавить запись `batchOpRemindSigning: { ... }` в `batchOpSpecs` map. Конфигурация: `allowedStatuses={domain.CampaignCreatorStatusSigning: true}`, `auditAction=AuditActionCampaignCreatorRemindSigning`, `text=telegram.CampaignRemindSigningText()`, `apply` использует `r.ApplyRemind(ctx, id)` (тот же что у `remind-invitation`). `requireContractTemplate` опущено (false по zero-value).
- [x] `service/campaign_creator.go:255`: новый публичный метод `RemindSigning(ctx, campaignID, creatorIDs)` — однострочный wrapper `dispatchBatch(ctx, campaignID, creatorIDs, batchOpRemindSigning)`. Godoc на русском.
- [x] `handler/campaign_creator.go:114`: новый метод `RemindCampaignCreatorsSigning` — копия `RemindCampaignCreatorsInvitation` с делегатом `s.campaignCreatorService.RemindSigning` и authz-вызовом `s.authzService.CanRemindCampaignCreatorsSigning`.
- [x] `authz/campaign_creator.go:50`: новый метод `CanRemindCampaignCreatorsSigning(ctx)` — admin-only, рядом с `CanRemindCampaignCreators`. Существующий **не** переименовывать.
- [x] `handler/server.go:122-128`: расширить `CampaignCreatorService` интерфейс новым методом `RemindSigning`. Обновить godoc-комментарий интерфейса (упомянуть третий remind-flow).
- [x] `handler/server.go:66`: расширить `AuthzService` интерфейс новым методом `CanRemindCampaignCreatorsSigning`.
- [x] `make generate-mocks` — обновить `MockCampaignCreatorService`, `MockAuthzService` и связанные.
- [x] `make build-backend` и `make lint-backend` — зелёные.

Backend тесты:
- [x] Authz unit `TestAuthzService_CanRemindCampaignCreatorsSigning` (`backend/internal/authz/campaign_creator_test.go:102` — рядом с `CanRemindCampaignCreators`). Три кейса по образцу: brand_manager → `ErrForbidden`, no role → `ErrForbidden`, admin → no error. `t.Parallel()`.
- [x] Unit service `TestCampaignCreatorService_RemindSigning` с `t.Parallel()` на функции и каждом t.Run; новый набор моков на каждом t.Run. Mocks: `MockCampaignCreatorRepoFactory`, `MockCampaignInviteNotifier`, `MockCampaignRepo`, `MockCampaignCreatorRepo`, `MockCreatorRepo`, `MockAuditRepo`, `MockPool`, capturing logger. Сценарии в порядке исполнения кода:
  - campaign not found / soft-deleted → `ErrCampaignNotFound` (require.ErrorIs).
  - not_in_campaign — 422 с правильным `reason`; notifier и ApplyRemind не вызваны (`AssertNotCalled`).
  - wrong_status table-driven для каждого из 6 «нецелевых» статусов (planned/invited/agreed/signed/signing_declined/declined) → 422 с `current_status`.
  - bot_blocked / unknown telegram error → undelivered с правильным reason, ApplyRemind не вызван, остальной батч продолжает (для другого creator вызван).
  - missing telegram_user_id (creator hard-deleted между validate и delivery) → undelivered reason `unknown`, error логгируется.
  - send OK + ApplyRemind error → undelivered reason `unknown`, error логгируется, остальной батч продолжает.
  - happy path 3 creators → пустой undelivered; ApplyRemind вызван с точным id (`mock.EXPECT().ApplyRemind(ctx, "exact-uuid")`); audit-row с `action=campaign_creator_remind_signing`, snapshot before (status=signing, reminded_count=N) и after (status=signing, reminded_count=N+1, reminded_at свежий через `require.WithinDuration` запас 1 мин).
  - **Captured-input на text**: через `mock.Run(func(args mock.Arguments) { ... })` достать `text` параметр `notifier.SendCampaignInvite` и проверить `require.Equal(t, telegram.CampaignRemindSigningText(), text)` — защита от copy-paste-бага, где batchOpSpec случайно ссылается на `CampaignRemindInvitationText`.
- [x] Unit handler `TestServer_RemindCampaignCreatorsSigning` (имя по соглашению `TestServer_*` — см. `TestServer_RemindCampaignCreatorsInvitation` на campaign_creator_test.go:612). Чёрный ящик через `httptest`. Mock'и: `authz.MockAuthzService`, `MockCampaignCreatorService`. Каждый `t.Run` с `t.Parallel()`, новый набор моков на сценарий. Кейсы:
  - **403 forbidden**: `authz.EXPECT().CanRemindCampaignCreatorsSigning(mock.Anything).Return(domain.ErrForbidden)` → 403 + ErrorResponse `FORBIDDEN`. `MockCampaignCreatorService` не вызван (`AssertNotCalled`).
  - **422 `CAMPAIGN_CREATOR_IDS_REQUIRED`** (domain.CodeCampaignCreatorIdsRequired): пустой `creatorIds`.
  - **422 `CAMPAIGN_CREATOR_IDS_DUPLICATES`** (domain.CodeCampaignCreatorIdsDuplicates): `[id1, id1]`.
  - **422 `CAMPAIGN_CREATOR_IDS_TOO_MANY`** (domain.CodeCampaignCreatorIdsTooMany): >200 ids.
  - **422 BATCH_INVALID**: service возвращает `CampaignCreatorBatchInvalidError` с `details` → handler возвращает 422 + code `CAMPAIGN_CREATOR_BATCH_INVALID` + полный массив details (типизированно через generated `CampaignCreatorBatchInvalidErrorResponse`).
  - **404 not found**: service возвращает `ErrCampaignNotFound` → 404.
  - **200 happy**: service возвращает пустой `undelivered` → 200 с пустым `Data.Undelivered`. Response через `json.Unmarshal` в generated `api.RemindCampaignCreatorsSigning200JSONResponse` (или эквивалент-имя). `require.Equal` целиком.
  - **200 partial**: service возвращает `[]domain.NotifyFailure{ {CreatorID, Reason} }` → 200 с одним элементом, проверка маппинга `domainNotifyFailuresToAPI`.
  - **Captured input** через `mock.Run(func(args mock.Arguments) { ... })` — `creatorIds` и `campaignID` дошли до сервиса без модификации (uuid сериализуется как string).
  - Сырой JSON в тестах **запрещён** — request body и response через generated `api.*` структуры.
- [x] E2E `backend/e2e/campaign_creator/campaign_notify_test.go` — новый `Test*` с `t.Parallel()`, header-doc обновить (нарратив на русском, дополнить новым `Test*`). Литерал `chunk12RemindSigningText` добавить рядом с `chunk12InviteText` / `chunk12RemindText` (campaign_notify_test.go:62-68) — точное зеркало `campaignRemindSigningText`. Setup через `testutil.SetupCampaignWithSigningCreator(t)` (testutil/contract_setup.go:92). Telegram-spy mechanics — переиспользуем helpers `waitInviteSent`, `WaitForTelegramSent`, `currentSpyForChat`, `RegisterTelegramSpyFailNext`, `AssertAuditEntry`, `findStatus`. Сценарии (через `t.Run` внутри одного `Test*` либо отдельные `Test*` — по образцу existing `TestRemindCampaignCreatorsInvitation`):
  - **403** brand_manager / **401** unauthenticated — короткие кейсы по образцу `TestNotifyCampaignCreators` (campaign_notify_test.go:151).
  - **422 empty batch** / **422 >200 ids** / **422 duplicates** — через `validateCampaignCreatorBatch` (handler-level).
  - **422 wrong_status**: remind-signing на creator в `invited` (привязан, но в неподходящем статусе) → `CAMPAIGN_CREATOR_BATCH_INVALID`, `current_status=invited`. GET creators — без изменений. Audit-row отсутствует. Telegram-spy: новых сообщений нет (`EnsureNoNewTelegramSent` или эквивалент после `NotifyBaselineSize`).
  - **422 not_in_campaign**: creatorId не привязан к этой кампании.
  - **404 soft-deleted campaign**.
  - **Happy path single**: после `SetupCampaignWithSigningCreator` → POST `/campaigns/{id}/remind-signing` с одним creatorId → 200, `undelivered=[]`. `findStatus` → `status=signing` (без изменений), `reminded_count=1`, `reminded_at` свежий (`require.NotNil` + `require.WithinDuration` с запасом 1 мин). `AssertAuditEntry` проверяет `action="campaign_creator_remind_signing"`. `waitInviteSent` с `expectedText=chunk12RemindSigningText` ассертит запись в spy с правильным `ChatId` и `WebAppUrl=tmaURL` (поведение `SendCampaignInvite` сохраняет WebApp-кнопку).
  - **Happy path repeat**: второй вызов на того же creator → `reminded_count=2`, новый audit-row, новая запись в spy.
  - **Partial success**: два creators в `signing` (см. Code Map → реализатор выбирает (a) две кампании / (b) helper-расширение). На одного из них зарегистрировать `RegisterTelegramSpyFailNext(t, creator2.TelegramUserID, "")` → POST с обоими ids → 200, `undelivered.len==1` с `reason="bot_blocked"`, у first creator `reminded_count=1` и audit-row есть, у failing creator `reminded_count=0` и audit-row отсутствует.

Frontend код:
- [x] `api/campaignCreators.ts:123`: `remindCampaignCreatorsSigning(campaignId, creatorIds)` через openapi-fetch на `/campaigns/{id}/remind-signing`. После `make generate-api` тип path известен openapi-fetch'у — литерал в `client.POST(...)` проходит typecheck.
- [x] `hooks/useCampaignNotifyMutations.ts`: расширить интерфейс `CampaignNotifyMutations` полем `remindSigning`; добавить третью мутацию `remindSigning` в теле хука; импорт новой API-функции. Тот же `noopOnError` (per `frontend-api.md` — обязателен `onError`).
- [x] `CampaignCreatorsSection.tsx`: расширить union `mutation?:` в сигнатуре `actionForStatus` (строки 308-310), вынести `SIGNING` в отдельный case с `mutation: mutations.remindSigning`.
- [x] `ru/campaigns.json:97`: добавить ключи `remindSigningButton` и `remindSigningSubmitting` (значения копируют `remindButton` / `remindSubmitting`).
- [x] `cd frontend/web && npx tsc --noEmit && npx eslint src/` — 0 ошибок.
- [x] `make lint-web` зелёный (нет unused imports, нет hardcoded литералов, exhaustive switch).

Frontend тесты:
- [x] `useCampaignNotifyMutations.test.ts`: успех (вызов правильной API-функции с правильными `campaignId`/`creatorIds`, корректный `CampaignNotifyResult`), ошибка (`onError` срабатывает). Мок самого API-клиента (не MSW).
- [x] `CampaignCreatorsSection.test.tsx`: для группы `signing` рендерится кнопка с testid `campaign-creators-group-action-signing` и текстом `t("campaignCreators.remindSigningButton")` через реальный `I18nextProvider` (i18n не мокать); loading state: при `isPending` кнопка disabled, текст «Отправка…»; клик с выбранными чекбоксами → мутация вызвана с правильными `creatorIds`; double-submit guard работает (повторный клик блокируется). Для AGREED/SIGNED/SIGNING_DECLINED ассертить, что testid `campaign-creators-group-action-${status}` **отсутствует** в DOM.
- [x] Frontend E2E `admin-campaign-creators-trustme.spec.ts` (или подходящий spec, ассертит TrustMe-driven signing-flow): после того как seed-helper приводит creator в `signing` (через API-хелперы + spy TrustMe — `helpers/api.ts` уже умеет это для existing trustme-spec'а), админ открывает страницу кампании → видит группу `campaign-creators-group-signing` → кликает testid `campaign-creators-group-action-signing` → ожидает success-блок `campaign-creators-group-{...}-success` с текстом «Доставлено N» → счётчик «получили N ремайндеров» / `data-testid="campaign-creators-reminded-count-{creatorId}"` (если такой есть; иначе — асерт через row-state) инкрементирован. Header — `/** ... */` на русском, нарратив. `E2E_CLEANUP=true` по умолчанию.
- [x] `make test-unit-web` и `make test-e2e-frontend` — зелёные.

**Acceptance Criteria:**
- Given кампания и креатор в статусе `signing`, when админ кликает «Разослать ремайндер» в группе «Подписывают договор», then ответ 200 с `undelivered=[]`, у креатора `reminded_count` инкрементирован, `reminded_at` свежий, `status` остаётся `signing`, audit-row `campaign_creator_remind_signing` создан, Telegram-сообщение с текстом `CampaignRemindSigningText()` доставлено.
- Given креатор в статусе `agreed` (или любом другом кроме `signing`), when админ пытается remind-signing, then 422 `CAMPAIGN_CREATOR_BATCH_INVALID` с `current_status` заполнен, БД не тронута, Telegram не вызван.
- Given креатор не привязан к кампании, when remind-signing, then 422 reason `not_in_campaign`.
- Given soft-deleted кампания, when remind-signing, then 404 `ErrCampaignNotFound`.
- Given бот заблокирован креатором (или unknown telegram error), when remind-signing, then 200 + `undelivered` содержит этого креатора с правильным `reason`, `reminded_count` для него **не** инкрементирован, остальная партия проходит.
- Given повторный клик ремайндера, when админ кликает дважды быстро, then только один POST уходит на бэкенд (UI-guard).
- Given группа `signing` пуста (rows=[]), when админ смотрит её, then секция отображается с empty-state «Нет креаторов» (из existing `always-show-groups`); кнопка «Разослать ремайндер» **отрендерена**, но `disabled` (size=0) — поведение наследовано от `CampaignCreatorGroupSection` (см. `submitDisabled = !hasAction || size === 0 || isBusy`).
- Given группы `agreed` / `signed` / `signing_declined`, when админ смотрит их, then кнопка действия **отсутствует в DOM** (testid `campaign-creators-group-action-${status}` не найден) — `actionForStatus` для этих статусов возвращает `{}`.
- Given группа `signing` с N креаторами, when админ выбирает M из них и жмёт ремайндер, then только M creatorIds уходят в POST.
- `make build-backend`, `make lint-backend`, `make test-unit-backend`, `make test-unit-backend-coverage`, `make test-e2e-backend` — зелёные.
- `make lint-web`, `make test-unit-web`, `make test-e2e-frontend` — зелёные.
- Coverage gate (≥ 80% per public/private method в покрываемых пакетах) не падает.

## Verification

**Commands:**
- `make generate-api` — регенерирует `server.gen.go`, `apiclient/*.gen.go`, `testclient/*.gen.go` и фронтовые `schema.ts`. После: `git status --short -- '*.gen.go' '*/schema.ts' '*/test-schema.ts'` — diff соответствует yaml-правке, ручных модификаций нет.
- `make generate-mocks` — регенерирует моки для `CampaignCreatorService` (+`RemindSigning`) и `AuthzService` (+`CanRemindCampaignCreatorsSigning`). Diff чистый.
- `cd backend && go build ./...` — 0 ошибок.
- `make lint-backend` — 0 ошибок.
- `make test-unit-backend` — все зелёные, `-race` включён.
- `make test-unit-backend-coverage` — gate зелёный (≥ 80% per identifier в покрываемых пакетах).
- `make test-e2e-backend` — все зелёные.
- `cd frontend/web && npx tsc --noEmit && npx eslint src/` — 0 ошибок.
- `make test-unit-web` — зелёные.
- `make test-e2e-frontend` — зелёные.

**Manual checks:**
- Подготовить (через staging seed-flow) одного креатора в статусе `signing`. Открыть `/campaigns/{id}` под админом → группа «Подписывают договор» содержит креатора и кнопку «Разослать ремайндер».
- Выбрать креатора → кликнуть кнопку → проверить, что Telegram-уведомление пришло с текстом из `CampaignRemindSigningText()`.
- Перезагрузить страницу — счётчики `reminded_count` обновились.
- Запросить `/campaigns/{id}/creators` через DevTools (или curl) → JSON содержит инкрементированный `reminded_count` и свежий `reminded_at`.
- Группы `agreed`, `signed`, `signing_declined` остаются **без кнопки** ремайндера.

## Suggested Review Order

**Контракт (OpenAPI + кодоген)**

- Точка входа: новый path-блок `/campaigns/{id}/remind-signing` рядом с `remind-invitation`; reuse `CampaignCreatorBatchInput` / `CampaignNotifyResult` / `CampaignCreatorBatchInvalidErrorResponse`; description с упоминанием симметрии и неизменности статуса.
  `backend/api/openapi.yaml`

- Generated server interface, openapi-fetch types, e2e-клиенты — diff только следствие yaml-правки.
  `backend/internal/api/server.gen.go`
  `backend/e2e/apiclient/client.gen.go`
  `frontend/web/src/api/generated/schema.ts`

**Backend: telegram + audit**

- Новый константный текст ремайндера + экспортируемый геттер по симметрии с `CampaignRemindInvitationText` (mirror в e2e обновляется в этом же PR).
  `backend/internal/telegram/notifier.go:95`
  `backend/internal/telegram/notifier.go:164`

- Новый AuditAction рядом с `AuditActionCampaignCreatorRemind`.
  `backend/internal/service/audit_constants.go:25`

**Backend: authz**

- Новый метод `CanRemindCampaignCreatorsSigning` (admin-only, симметрия с `CanRemindCampaignCreators`).
  `backend/internal/authz/campaign_creator.go:50`

- Расширение `AuthzService` интерфейса для handler — новый метод в списке.
  `backend/internal/handler/server.go:66`

**Backend: service**

- Новый `batchOpRemindSigning` константа.
  `backend/internal/service/campaign_creator.go:195`

- Новая запись в `batchOpSpecs`: `allowedStatuses={signing}`, audit=`RemindSigning`, text=`CampaignRemindSigningText()`, apply=`ApplyRemind`, без `requireContractTemplate`.
  `backend/internal/service/campaign_creator.go:210`

- Публичный метод `RemindSigning` — wrapper над `dispatchBatch`.
  `backend/internal/service/campaign_creator.go:255`

- Расширение `CampaignCreatorService` интерфейса для handler — новый метод в списке.
  `backend/internal/handler/server.go:122`

**Backend: handler**

- Новый метод `RemindCampaignCreatorsSigning` — копия `RemindCampaignCreatorsInvitation` с authz-вызовом `CanRemindCampaignCreatorsSigning` и делегатом в `s.campaignCreatorService.RemindSigning`.
  `backend/internal/handler/campaign_creator.go:114`

**Frontend: API + хук**

- Новая функция-клиент `remindCampaignCreatorsSigning` через openapi-fetch.
  `frontend/web/src/api/campaignCreators.ts:123`

- Третья мутация `remindSigning` в `useCampaignNotifyMutations`, `noopOnError` (frontend-api: onError обязателен). Интерфейс `CampaignNotifyMutations` расширен полем `remindSigning`.
  `frontend/web/src/features/campaigns/creators/hooks/useCampaignNotifyMutations.ts:9`

**Frontend: UI + i18n**

- Case `SIGNING` в `actionForStatus()` маппит на новую мутацию и i18n-ключи; union `mutation?:` расширен.
  `frontend/web/src/features/campaigns/creators/CampaignCreatorsSection.tsx:301`

- Ключи `remindSigningButton` / `remindSigningSubmitting` рядом с существующими `remindButton` / `remindSubmitting`.
  `frontend/web/src/shared/i18n/locales/ru/campaigns.json:97`

**Тесты — backend unit**

- Authz: три кейса (brand_manager forbidden / unauthenticated forbidden / admin ok). t.Parallel().
  `backend/internal/authz/campaign_creator_test.go:102`

- Service `RemindSigning`: t.Parallel(), новые моки на каждый t.Run; happy + wrong_status table-driven + not_in_campaign + bot_blocked + missing chat + send-ok-persist-fail; **captured-input** `text=CampaignRemindSigningText()` (защита от copy-paste-бага).
  `backend/internal/service/campaign_creator_test.go`

- Handler `RemindCampaignCreatorsSigning`: 403 forbidden + 422 (empty/duplicates/too_many/batch_invalid) + 404 + 200 happy/partial. Типизированный request/response через `api.*`. Captured-input на campaignID и creatorIds.
  `backend/internal/handler/campaign_creator_test.go`

**Тесты — backend e2e**

- Литералы `chunk12RemindSigningText` (зеркало `campaignRemindSigningText`) рядом с `chunk12InviteText` / `chunk12RemindText`.
  `backend/e2e/campaign_creator/campaign_notify_test.go:62`

- Новый `Test*` со сценариями: 403 / 401 / 422-shape (empty/>200/duplicates) / 422 wrong_status / 422 not_in_campaign / 404 soft-deleted / happy single / happy repeat / partial success bot_blocked. Setup через `testutil.SetupCampaignWithSigningCreator`; helpers `waitInviteSent` (с `chunk12RemindSigningText`), `RegisterTelegramSpyFailNext`, `AssertAuditEntry`, `findStatus`. Header на русском, нарратив, `t.Parallel()`.
  `backend/e2e/campaign_creator/campaign_notify_test.go`
  `backend/e2e/testutil/contract_setup.go:92`

**Тесты — frontend**

- Unit: вызов API-функции с правильными аргументами, `onError` срабатывает.
  `frontend/web/src/features/campaigns/creators/hooks/useCampaignNotifyMutations.test.ts`

- Unit компонента: для группы `signing` рендерится кнопка, loading state, клик → мутация, double-submit guard.
  `frontend/web/src/features/campaigns/creators/CampaignCreatorsSection.test.tsx`

- E2E: админ-flow remind-signing после доведения креатора до signing; assert на инкремент `reminded_count` в UI; `data-testid` на новой кнопке.
  `frontend/e2e/web/admin-campaign-creators-trustme.spec.ts`
