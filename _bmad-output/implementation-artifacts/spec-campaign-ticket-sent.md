---
title: 'Чекбокс «Билет отправлен» в админской таблице креаторов кампании'
type: 'feature'
created: '2026-05-11'
status: 'done'
baseline_commit: '2e49517e7ba626186ac8815c126d921405493d9f'
context:
  - docs/standards/backend-architecture.md
  - docs/standards/backend-codegen.md
  - docs/standards/frontend-state.md
---

<frozen-after-approval reason="human-owned intent — do not modify unless human renegotiates">

## Intent

**Problem:** Админ на `/campaigns/{id}` (web) ведёт операционную работу — отправляет авиа/жд-билеты креаторам, подписавшим договор. Отметки «билет отправлен» в системе нет: оператор держит список в голове / в сторонней табличке.

**Approach:** В группе `signed` таблицы креаторов добавляется колонка с чекбоксом. Флип шлёт `PATCH /campaigns/{id}/creators/{creatorId}` `{ticketSent: bool}` → бэк пишет `campaign_creators.ticket_sent_at TIMESTAMPTZ` (`NOW()` / `NULL`) внутри `dbutil.WithTx` с audit-row. UI обновляется optimistic'ом.

## Boundaries & Constraints

**Always:**
- Только `admin` (через `internal/authz/campaign_creator.go`, `Can*` + `middleware.RoleFromContext`).
- Чекбокс рендерится **только** для группы `status === 'signed'`.
- Любая мутация — внутри `dbutil.WithTx` с audit-row в одной транзакции.
- Одно audit-action `AuditActionCampaignCreatorTicketSent` (`"campaign_creator.ticket_sent"`) + `oldValue`/`newValue` — не `.set` / `.unset`.
- No-op early return: если `(row.TicketSentAt != nil) == *patch.TicketSent` — возвращаем row без UPDATE и без audit.
- После правки `openapi.yaml` — `make generate-api`; правка `*.gen.go` / `schema.ts` руками запрещена.
- Логи без PII (только UUID, action) — `security.md`.

**Ask First:**
- Семантика поля (bool вместо timestamptz).
- Расширение на не-admin роли.
- Показ чекбокса в других статус-группах.

**Never:**
- DEFAULT/CHECK на колонке в миграции (это business state).
- `ticket_sent_at` в `INSERT`-теге Row-структуры.
- Локаторы по тексту во frontend e2e (только `data-testid`).

## I/O & Edge-Case Matrix

| Scenario | Input / State | Expected | Error |
|---|---|---|---|
| Set (admin, signed) | `{ticketSent: true}` | 200, `ticketSentAt ≈ NOW()`; audit old=null/new=ts | — |
| Unset (admin, signed) | `{ticketSent: false}` | 200, `ticketSentAt = null`; audit old=ts/new=null | — |
| No-op (значение не меняется) | `{ticketSent: true}` при `!= null` | 200, row без изменений; **audit не пишется** | — |
| status != signed | `{ticketSent: true}` для invited/signing/declined | — | 422, `CodeValidation`, «Билет можно отметить только подписавшему договор креатору» |
| Empty body | `PATCH` с `{}` | — | 422, `CodeValidation`, «Передайте хотя бы одно поле для обновления» (service не вызывается) |
| Unknown creator | `PATCH ...creators/{nonexistent}` | — | 404, `CodeNotFound` |
| Не-admin | brand_manager / creator | — | 403, `domain.ErrForbidden` |
| Без access token | — | — | 401 |

</frozen-after-approval>

## Tasks & Acceptance

**Execution:**

- [x] `backend/api/openapi.yaml` -- `PATCH /campaigns/{campaignId}/creators/{creatorId}` (образец — `PATCH /campaigns/{id}` ~стр. 1164); schema `CampaignCreatorPatchInput {ticketSent?: bool}`; добавить `ticketSentAt: string|null` в schema `CampaignCreator` (~стр. 3511). Затем `make generate-api`.
- [x] `backend/migrations/20260511182253_campaign_creators_ticket_sent_at.sql` -- Up: `ALTER TABLE campaign_creators ADD COLUMN ticket_sent_at TIMESTAMPTZ`; Down: `DROP COLUMN ticket_sent_at`. Без default, без CHECK.
- [x] `backend/internal/repository/campaign_creator.go` -- `TicketSentAt *time.Time` (`db:"ticket_sent_at"`, без `insert`); константа `CampaignCreatorColumnTicketSentAt`; метод `UpdateTicketSentAt(ctx, id string, sentAt *time.Time) (*CampaignCreatorRow, error)` — `UPDATE … SET ticket_sent_at=$1, updated_at=NOW() WHERE id=$2 RETURNING <selectColumns>`. Паттерн `ApplyInvite/ApplyRemind/ApplyDecision`. 0 rows → `sql.ErrNoRows`.
- [x] `backend/internal/repository/campaign_creator_test.go` -- `TestCampaignCreatorRepository_UpdateTicketSentAt`: set, unset, not-found, error propagation. Точные SQL-литералы + pgxmock.
- [x] `backend/internal/service/audit_constants.go` -- `AuditActionCampaignCreatorTicketSent = "campaign_creator.ticket_sent"`.
- [x] `backend/internal/service/campaign_creator.go` -- после `RemindSigning` добавить `PatchParticipation(ctx, campaignID, creatorID string, patch api.CampaignCreatorPatchInput) (*domain.CampaignCreator, error)`: Get → 404 mapping → status check (`signed` иначе 422) → no-op early return → `dbutil.WithTx`{ `UpdateTicketSentAt` + `writeAudit(ctx, auditRepo, AuditActionCampaignCreatorTicketSent, AuditEntityTypeCampaignCreator, row.ID, oldVal, newVal)` } → `logger.Info` после tx → `campaignCreatorRowToDomain(updated)` (стр. ~457).
- [x] `backend/internal/service/campaign_creator_test.go` -- `TestCampaignCreatorService_PatchParticipation`: not-found → 404, status!=signed → 422, no-op (без UPDATE, без audit), set (`nil→ts`), unset (`ts→nil`), ошибка repo прокинулась. Captured args для writeAudit old/new через `mock.Run`.
- [x] `backend/internal/authz/campaign_creator.go` -- `CanPatchCampaignCreator()` по паттерну `CanRemoveCampaignCreator`.
- [x] `backend/internal/handler/campaign_creator.go` -- `PatchCampaignCreator` (receiver `(s *Server)`): authz → если `body.TicketSent == nil` (нет полей) → 422 `CodeValidation` ещё до service → `service.PatchParticipation` → 200 c `domain.CampaignCreator` → респонс через сгенерированный strict-server type.
- [x] `backend/internal/handler/campaign_creator_test.go` -- `TestServer_PatchCampaignCreator`: empty body 422, forbidden 403, ValidationError из service 422, NotFound 404, success 200; captured admin-actor из middleware-context.
- [x] `backend/e2e/campaign_creator/campaign_creator_test.go` (фактический home для /campaigns/{id}/creators e2e — заменил `campaign_test.go` из плана) -- `TestPatchCampaignCreator` через `testutil.SetupCampaignWithSigningCreator` + `PostTrustMeWebhook(SignedPayload)`: PATCH true → 200 + audit; PATCH true повторно → no-op (audit count не растёт); PATCH false → 200 + второй audit с обратным diff; status!=signed → 422; unknown creator → 404; empty body → 422; non-admin → 403; raw HTTP unauthenticated → 401. Все `t.Run` с `t.Parallel()` и LIFO-cleanup.
- [x] `frontend/web/src/api/campaignCreators.ts` -- `patchCampaignCreator(apiUrl, campaignId, creatorId, body, token)` через openapi-fetch (паттерн `removeCampaignCreator`).
- [x] `frontend/web/src/features/campaigns/creators/hooks/usePatchCampaignCreator.ts` -- `useMutation`: `onMutate` (`cancelQueries(campaignCreatorKeys.list(campaignId))` → snapshot → `setQueryData` patch-in-place); `onError` (откат к snapshot + toast `campaignCreators.ticketSentSaveError`); `onSettled` (`invalidateQueries(campaignCreatorKeys.list(...))`).
- [x] `frontend/web/src/features/campaigns/creators/hooks/usePatchCampaignCreator.test.ts` -- success: cache обновлён; error: откат + toast вызван.
- [x] `frontend/web/src/features/campaigns/creators/CampaignCreatorsTable.tsx` + `CampaignCreatorGroupSection.tsx` -- колонка «Билет отправлен» под условием `status === 'signed'`; чекбокс с `data-testid="campaign-creator-ticket-sent-{creatorId}"`, `aria-label`, `disabled` при `mutation.isPending`. Текст через `t('campaignCreators.ticketSent')`.
- [x] `frontend/web/src/shared/i18n/locales/ru/campaigns.json` -- `campaignCreators.ticketSent` + `campaignCreators.ticketSentSaveError`.
- [~] `frontend/e2e/web/admin-campaign-creators-mutations.spec.ts` -- **DEFERRED** (см. Spec Change Log + `_bmad-output/implementation-artifacts/deferred-work.md`): требует composable helper для TrustMe webhook signed в `frontend/e2e/helpers/`, которого пока нет. Покрытие закрыто backend e2e + frontend unit.

**Acceptance Criteria:**

- Given admin в кампании со signed-креатором, when кликает чекбокс «Билет отправлен», then UI сразу показывает галку, `PATCH` → 200, в БД `ticket_sent_at = NOW()`, в `audit_logs` появилась строка `campaign_creator.ticket_sent` с old=null/new=ts.
- Given signed-креатор с уже выставленной отметкой, when admin кликает повторно с тем же значением, then 200, БД не тронута, новых audit-row нет.
- Given креатор со status invited/signing/declined, when admin шлёт PATCH ticketSent=true, then 422 `CodeValidation`, UI откатывает optimistic-изменение и показывает toast.
- Given не-admin (brand_manager / creator), when шлёт PATCH, then 403.

## Spec Change Log

- **Frontend e2e deferred (2026-05-11).** Под жёсткий дедлайн фичи UI-флип чекбокса не покрыт Playwright-тестом — для этого нужен composable helper, симулирующий TrustMe webhook signed на стороне frontend e2e (его сейчас нет в `frontend/e2e/helpers/`). Покрытие закрыто backend e2e (TrustMe webhook + PATCH), backend unit (repo+service+handler) и frontend unit (хук, optimistic/rollback/toast). Подробности — `_bmad-output/implementation-artifacts/deferred-work.md`. KEEP: трёхслойное покрытие без UI-флипа достаточно для приёмки фичи.
- **E2E файл-локация (2026-05-11).** Бэк-e2e дополнили `backend/e2e/campaign_creator/campaign_creator_test.go` (это реальный home для /campaigns/{id}/creators сценариев), а не `backend/e2e/campaign/campaign_test.go`, как было заявлено в спеке. Логически — тесты campaign_creator живут рядом.

## Design Notes

**Audit-payload форма.** `writeAudit(ctx, repo, action, entityType, entityID, oldValue, newValue)` принимает old/new. Конкретный type/shape — скопировать из вызова `writeAudit` в `service/campaign_creator.go` для decision/contract_signed (соседние audit-вызовы). Для нашего поля old/new — `*time.Time` либо обёртка `map[string]any{"ticket_sent_at": ...}` под существующий контракт `writeAudit`.

**Frontend optimistic.** `onMutate` шаги: `cancelQueries` → `getQueryData` snapshot → `setQueryData` — заменить конкретный `CampaignCreator` в массиве (по `creatorId`) с новым `ticketSentAt` → вернуть `{previous}` для отката.

## Verification

**Commands:**
- `make generate-api` -- регенерация после правки `openapi.yaml`.
- `make migrate-up` -- миграция применилась.
- `make lint-backend`, `make lint-web` -- 0 ошибок.
- `make test-unit-backend`, `make test-unit-backend-coverage` -- зелёные, 80%-gate пройден.
- `make test-unit-web` -- зелёный.
- `make test-e2e-backend`, `make test-e2e-frontend` -- зелёные.

## Suggested Review Order

**API контракт + миграция**

- PATCH-операция, schema CampaignCreatorPatchInput, поле ticketSentAt — точка входа архитектуры.
  [`openapi.yaml:1375`](../../backend/api/openapi.yaml#L1375)

- Forward + Down миграция без default/CHECK — ticket_sent_at NULLABLE для NULL=«не отправлено».
  [`20260511182253_campaign_creators_ticket_sent_at.sql:1`](../../backend/migrations/20260511182253_campaign_creators_ticket_sent_at.sql#L1)

**Backend: business logic + audit (главная развилка)**

- `PatchParticipation` — flow status check → no-op early return → WithTx{Update + writeAudit}.
  [`campaign_creator.go:313`](../../backend/internal/service/campaign_creator.go#L313)

- Audit action `campaign_creator.ticket_sent` (одно действие + old/new diff, не `.set`/`.unset`).
  [`audit_constants.go:35`](../../backend/internal/service/audit_constants.go#L35)

- `UpdateTicketSentAt` — `UPDATE … RETURNING` в паттерне `ApplyInvite/ApplyRemind/ApplyDecision`.
  [`campaign_creator.go:335`](../../backend/internal/repository/campaign_creator.go#L335)

- Domain input `PatchCampaignCreatorInput` + sentinel-ошибки (`ErrCampaignCreatorTicketSentBadStatus`, `ErrCampaignCreatorPatchEmpty`).
  [`campaign_creator.go:155`](../../backend/internal/domain/campaign_creator.go#L155)

**Backend: handler + authz boundary**

- `PatchCampaignCreator` — authz → empty body 422 (защита в handler, не дублируется в сервисе) → service.
  [`campaign_creator.go:71`](../../backend/internal/handler/campaign_creator.go#L71)

- `CanPatchCampaignCreator` — admin-only gate по паттерну `CanRemoveCampaignCreator`.
  [`campaign_creator.go:71`](../../backend/internal/authz/campaign_creator.go#L71)

**Frontend: data слой (мутация с optimistic update)**

- `usePatchCampaignCreator` — onMutate snapshot → setQueryData → onError rollback → onSettled invalidate.
  [`usePatchCampaignCreator.ts:30`](../../frontend/web/src/features/campaigns/creators/hooks/usePatchCampaignCreator.ts#L30)

- `patchCampaignCreator` API-обёртка через openapi-fetch (паттерн `removeCampaignCreator`).
  [`campaignCreators.ts:171`](../../frontend/web/src/api/campaignCreators.ts#L171)

**Frontend: UI binding (admin-флоу)**

- `CampaignCreatorsSection` — оркестрирует mutation, ticketSentPending Set, inline-alert на ошибке.
  [`CampaignCreatorsSection.tsx:80`](../../frontend/web/src/features/campaigns/creators/CampaignCreatorsSection.tsx#L80)

- `ticketSentColumn` — рендер чекбокса с data-testid/aria-label/disabled под `mutation.isPending`.
  [`CampaignCreatorsTable.tsx:381`](../../frontend/web/src/features/campaigns/creators/CampaignCreatorsTable.tsx#L381)

- i18n ключи (`columns.ticketSent`, `ticketSentToggleAria`, `ticketSentSaveError`).
  [`campaigns.json:109`](../../frontend/web/src/shared/i18n/locales/ru/campaigns.json#L109)

**Тесты (пирамида)**

- Backend e2e — happy через TrustMe webhook signed + no-op + status!=signed + 404/422/403/401.
  [`campaign_creator_test.go:580`](../../backend/e2e/campaign_creator/campaign_creator_test.go#L580)

- Service unit — captured args для audit OldValue/NewValue, no-op без UPDATE/audit.
  [`campaign_creator_test.go:1561`](../../backend/internal/service/campaign_creator_test.go#L1561)

- Frontend hook unit — success-cache update / rollback на ошибке / unset clears `ticketSentAt`.
  [`usePatchCampaignCreator.test.tsx:43`](../../frontend/web/src/features/campaigns/creators/hooks/usePatchCampaignCreator.test.tsx#L43)
