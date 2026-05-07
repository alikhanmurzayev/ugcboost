---
title: 'Approve креатора с опциональным добавлением в выбранные кампании'
type: 'feature'
created: '2026-05-07'
status: 'done'
baseline_commit: 'fea0261d8ffd6fa3eec11260ec937a1eb7a4a5f2'
context:
  - _bmad-output/implementation-artifacts/intent-creator-approve-with-campaigns.md
  - _bmad-output/planning-artifacts/design-campaign-creator-flow.md
---

## Преамбула — стандарты обязательны

Перед любой строкой production-кода агент обязан полностью загрузить все файлы `docs/standards/` (через `/standards`). Каждое правило — hard rule. Особенно применимы: `backend-architecture.md`, `backend-codegen.md`, `backend-errors.md`, `backend-transactions.md`, `backend-testing-unit.md`, `backend-testing-e2e.md`, `frontend-api.md`, `frontend-components.md`, `frontend-state.md`, `frontend-types.md`, `frontend-testing-unit.md`, `frontend-testing-e2e.md`, `naming.md`, `security.md`, `review-checklist.md`.

<frozen-after-approval reason="human-owned intent — do not modify unless human renegotiates">

## Intent

**Problem:** Чтобы добавить нового креатора в одну/несколько кампаний, админ совершает много действий: approve заявки → переход в каждую кампанию → drawer выбора креаторов → поиск → check → save. На MVP-сцене с десятками заявок в день это ощутимое трение.

**Approach:** Расширяем `POST /creators/applications/{id}/approve` опциональным `campaignIds[]` (UUID, maxItems=20). Handler валидирует (count/dedupe/format) + батч-проверяет существование и `is_deleted=false` через новый `CampaignService.AssertActiveCampaigns`. Service approve после tx1 (creator+audit) и Telegram-notify (как сейчас) последовательно добавляет креатора в каждую кампанию через `CampaignCreatorService.Add` (своя tx на каждую) с first-fail-stop. На UI диалога approve добавляем `SearchableMultiselect` с активными кампаниями.

## Boundaries & Constraints

**Always:**
- Расширяем существующую ручку, новый endpoint не создаём.
- Sequential transactions, не одна атомарная: tx1 (approve+create+audit) → notify (fire-and-forget) → цикл по `campaignIds` (своя tx на каждую через `CampaignCreatorService.Add`).
- First-fail-stop: при первом fail на add — стоп, ошибка наверх; уже добавленные в предыдущие кампании остаются commit'ed.
- Pre-validation существования + `is_deleted=false` — в handler через `CampaignService.AssertActiveCampaigns`. Если хоть одна кампания невалидна — service approve не вызывается, креатор не создаётся.
- Audit без новых event types: `creator_application_approve` (tx1, как сейчас), `campaign_creator_add` (внутри `Add`, как сейчас).
- Multi-select — реиспользуем готовый `frontend/web/src/shared/components/SearchableMultiselect.tsx`.
- Список кампаний во фронте — `listCampaigns({ isDeleted: false, page: 1, pageSize: 100 })`.
- Handler-validation: `campaignIds` опционально; пустой массив / null / отсутствие = старое поведение; len > 20 → 422 `CAMPAIGN_IDS_TOO_MANY`; дубликаты UUID → 422 `CAMPAIGN_IDS_DUPLICATES`.
- Pre-validation fail → 422 `CAMPAIGN_NOT_AVAILABLE_FOR_ADD` (единый код для обоих кейсов: не существует / soft-deleted).
- E2E на approve+with+campaigns живёт в новом файле `backend/e2e/creator_applications/approve_with_campaigns_test.go`. Существующий `approve_test.go` не трогаем.

**Ask First:**
- Если `CreatorApplicationHandler`-конструктор требует invasive рефактора (новые зависимости ломают существующие тесты непредвиденно) — HALT.
- Если в `frontend/web/src/api/campaigns.ts` стоят разные `CampaignsListInput.pageSize` дефолты или фильтр `isDeleted` ведёт себя не так как в OpenAPI — HALT.
- Если параллельная работа агента в `frontend/web/src/features/campaigns/creators/*` за время этой сессии замержилась и API-контракт `addCampaignCreators` изменился — HALT.

**Never:**
- Не оборачиваем approve+all-adds в одну атомарную транзакцию (требует рефактора `CampaignCreatorService.Add`).
- Не возвращаем `{addedCampaignIds, failedCampaigns}`-структурированную response shape — стандартный error envelope.
- Не делаем параллельные add'ы (best-effort partial-success запрещён).
- Не вводим новые audit event types.
- Не создаём новый endpoint `/approve-with-campaigns`.
- Не модифицируем `backend/e2e/creator_applications/approve_test.go` — старый flow остаётся как baseline.
- Не модифицируем файлы `frontend/web/src/features/campaigns/creators/*` и `frontend/web/src/shared/components/Drawer.tsx` (параллельный агент работает там).

## I/O & Edge-Case Matrix

| Scenario | Input / State | Expected Output / Behavior | Error Handling |
|---|---|---|---|
| approve без кампаний | body `{}` или `{campaignIds: []}` или `{campaignIds: null}` | 200, creatorId; полностью старое поведение | — |
| approve с N валидными кампаниями | `campaignIds: [A,B,C]`, все active | 200, creatorId; в audit_logs N+1 rows: `creator_application_approve` (tx1) + N×`campaign_creator_add` (tx_i) | — |
| handler-validation: > 20 IDs | `campaignIds: [21 UUIDs]` | 422 | `CAMPAIGN_IDS_TOO_MANY`; service не вызван |
| handler-validation: дубликаты | `campaignIds: [A,A,B]` | 422 | `CAMPAIGN_IDS_DUPLICATES`; service не вызван |
| pre-validation fail: ID не существует | `campaignIds: [A,nonexistent]` | 422; заявка остаётся в moderation, креатор не создан | `CAMPAIGN_NOT_AVAILABLE_FOR_ADD`; tx1 не открыта |
| pre-validation fail: soft-deleted | `campaignIds: [A,deleted]` | 422; заявка остаётся в moderation | `CAMPAIGN_NOT_AVAILABLE_FOR_ADD`; tx1 не открыта |
| race mid-cycle: B удалена между pre-check и циклом | `campaignIds: [A,B,C]`; B → deleted после pre-check | creator создан, в A добавлен, на B — first-fail-stop (422 `CAMPAIGN_NOT_FOUND` от `Add`); C не пробуется | actionable error.Message от backend — фронт показывает inline |
| Telegram-notify падает | `campaignIds: [A]`; tg-bot недоступен | 200 (как сейчас — fire-and-forget); add в A проходит | log warn, как в текущем `notifyApplicationApproved` |
| креатор уже в кампании (race) | теоретически невозможно (только что создан) | если случилось — 422 `CREATOR_ALREADY_IN_CAMPAIGN` от `Add` | first-fail-stop срабатывает |

</frozen-after-approval>

## Code Map

- `backend/api/openapi.yaml:785-856` — operation `approveCreatorApplication`; добавить request body с `campaignIds[]`; обновить error responses.
- `backend/internal/domain/errors.go:18-55` — соседние `Code*`/`Err*` для campaign и campaign_creator; добавить новые коды.
- `backend/internal/service/campaign.go:25-160` — `CampaignService`; добавить публичный метод `AssertActiveCampaigns(ctx, ids []string) error`.
- `backend/internal/repository/campaign.go` — добавить `ListByIDs(ctx, ids []string) ([]*CampaignRow, error)` для batch lookup.
- `backend/internal/handler/creator_application.go:340-365` — `ApproveCreatorApplication`; принять и провалидировать `campaignIds`, вызвать `campaignService.AssertActiveCampaigns`, прокинуть в service.
- `backend/internal/service/creator_application.go:1159-1297` — `ApproveApplication`; новый параметр `campaignIDs []string`; цикл add'ов после notify.
- `backend/internal/service/campaign_creator.go:48-87` — `CampaignCreatorService.Add`; не меняется, но становится внешней зависимостью service approve.
- `backend/e2e/creator_applications/approve_test.go` — baseline; не трогаем.
- `frontend/web/src/api/creatorApplications.ts:73-84` — `approveApplication`; принимать опциональный `campaignIds`.
- `frontend/web/src/api/campaigns.ts:17-25` — `listCampaigns` API wrapper.
- `frontend/web/src/features/creatorApplications/components/ApproveApplicationDialog.tsx:1-159` — расширение UI: SearchableMultiselect, fetch кампаний, передача в mutation.
- `frontend/web/src/shared/components/SearchableMultiselect.tsx:1-164` — готовый компонент; используем как есть.
- `frontend/web/src/shared/i18n/locales/ru/creatorApplications.json` — добавить ключи `approveDialog.campaigns*`.
- `frontend/e2e/web/admin-creator-applications-moderation-approve-action.spec.ts:1-80` — соседний baseline e2e; новый файл рядом.

## Tasks & Acceptance

**Execution:**

Backend — contract & domain:
- [x] `backend/api/openapi.yaml` — расширить `approveCreatorApplication` request body опциональным `campaignIds: { type: array, items: {uuid}, maxItems: 20 }`. Обновить response 422 examples новыми кодами. После — `make generate-api`.
- [x] `backend/internal/domain/errors.go` — добавить `CodeCampaignIdsTooMany`/`CodeCampaignIdsDuplicates`/`CodeCampaignNotAvailableForAdd` + соответствующие `Err*` constructor functions с actionable message (`CAMPAIGN_NOT_AVAILABLE_FOR_ADD`: «Одна или несколько выбранных кампаний недоступны. Обновите список и попробуйте снова»).

Backend — repo & service:
- [x] `backend/internal/repository/campaign.go` — добавить `ListByIDs(ctx, ids []string) ([]*CampaignRow, error)`: SELECT всех колонок `WHERE id = ANY(?)`, через `dbutil.Many`. Использует `campaignSelectColumns`.
- [x] `backend/internal/service/campaign.go` — добавить публичный метод `AssertActiveCampaigns(ctx, ids []string) error`: noop при пустом slice; иначе вызвать `repoFactory.NewCampaignRepo(s.pool).ListByIDs(ids)`, сравнить длины и `is_deleted` каждого row; если хоть один не найден или soft-deleted — вернуть `domain.ErrCampaignNotAvailableForAdd`.
- [x] `backend/internal/service/creator_application.go` — `ApproveApplication` принимает новый параметр `campaignIDs []string`. После tx1 + `notifyApplicationApproved` (порядок не меняем): `for _, id := range campaignIDs { if err := s.campaignCreatorService.Add(ctx, id, []string{creatorID}); err != nil { return "", fmt.Errorf("approveApplication add to campaign %s: %w", id, err) } }`. Новый локальный интерфейс `CampaignCreatorAdder` в пакете service по convention "accept interfaces".

Backend — handler:
- [x] `backend/internal/handler/creator_application.go` — `ApproveCreatorApplication`: распарсить body (если присутствует), normalize (nil/empty = "не добавлять"). Если непуст: validate len ≤ 20 → 422; dedupe (по сравнению `len(set) != len(slice)`) → 422; вызвать `campaignService.AssertActiveCampaigns(ctx, ids)` → пробросить domain-error (translate в 422). Прокинуть нормализованный `campaignIDs` в `creatorApplicationService.ApproveApplication`. Новая зависимость в конструкторе handler — локальный интерфейс `CampaignActiveChecker`.

Backend — unit tests:
- [x] `backend/internal/repository/campaign_test.go` — `TestCampaignRepository_ListByIDs`: SQL assertion (точный query через captureQuery), row mapping для 1 и N рядов, empty input.
- [x] `backend/internal/service/campaign_test.go` — `TestCampaignService_AssertActiveCampaigns`: empty slice (noop), все active (nil), один отсутствует (`ErrCampaignNotAvailableForAdd`), один soft-deleted (`ErrCampaignNotAvailableForAdd`).
- [x] `backend/internal/service/creator_application_test.go` — `TestCreatorApplicationService_ApproveApplication`: новые `t.Run`'s — empty campaignIDs (только tx1+notify, `Add` не вызван); happy path с N кампаний (последовательные `Add` через captured-input на mock); first-fail-stop (mock возвращает err на 2-й из 3 → 3-й `Add` не вызван, ошибка обёрнута с контекстом).
- [x] `backend/internal/handler/creator_application_test.go` — `TestCreatorApplicationHandler_ApproveCreatorApplication`: новые `t.Run` — body без поля (старое поведение), пустой массив, валидный массив (mocks: `AssertActiveCampaigns` returns nil → service called с captured `campaignIDs`), > 20 (422 `CAMPAIGN_IDS_TOO_MANY`, service не вызван), дубли (422 `CAMPAIGN_IDS_DUPLICATES`, service не вызван), pre-validation fail (mock returns `ErrCampaignNotAvailableForAdd` → 422, service не вызван).

Backend — e2e:
- [x] `backend/e2e/creator_applications/approve_with_campaigns_test.go` — NEW FILE с обязательным godoc-нарративом на русском. `TestApproveWithCampaigns` с `t.Run`'s: success-with-N-campaigns (assert audit_logs: 1 approve + N campaign_creator_add per-creator; assert `campaign_creators` rows со статусом `planned`); validation `CAMPAIGN_IDS_TOO_MANY`/`CAMPAIGN_IDS_DUPLICATES`; pre-validation `CAMPAIGN_NOT_AVAILABLE_FOR_ADD` для несуществующего ID. Soft-deleted и mid-cycle race зафиксированы как ограничение в нарративе (бизнес-API soft-delete'а нет; race недостижим через HTTP — покрыт unit-тестом first-fail-stop).

Frontend — API & state:
- [x] `frontend/web/src/api/creatorApplications.ts` — `approveApplication` принимает второй параметр `campaignIds?: string[]`; передаёт в `body: campaignIds && campaignIds.length > 0 ? { campaignIds } : undefined`. Сохраняем существующий error-handling.

Frontend — UI:
- [x] `frontend/web/src/features/creatorApplications/components/ApproveApplicationDialog.tsx` — внутри dialog добавить `useQuery` на `listCampaigns({ isDeleted: false, page: 1, perPage: 100, sort: "name", order: "asc" })`. Render `SearchableMultiselect` с `options = campaigns.items.map(c => ({code: c.id, name: c.name}))`. Local state `selectedCampaignIds: string[]`. Submit-button disabled пока кампании грузятся (`isLoading`) или mutation pending (submit-lock с async prereqs). При submit передавать `selectedCampaignIds` в `approveApplication(applicationId, selectedCampaignIds)`. Discriminated 4xx error handling: `CAMPAIGN_IDS_*` / `CAMPAIGN_NOT_AVAILABLE_FOR_ADD` → inline-error в dialog без invalidate (иначе drawer remount теряет state); другие 4xx → `onApiError` + invalidate + close (как раньше). На success — invalidate `campaignKeys.detail(id)` и `campaignCreatorKeys.list(id)` для каждого выбранного ID.
- [x] `frontend/web/src/shared/i18n/locales/ru/creatorApplications.json` — добавлены ключи под `approveDialog`: `campaignsLabel`, `campaignsHint`, `campaignsPlaceholder`, `campaignsSearchPlaceholder`, `campaignsLoading`, `campaignsLoadError`.

Frontend — unit tests:
- [x] `frontend/web/src/features/creatorApplications/components/ApproveApplicationDialog.test.tsx` — расширены до 21 сценария: render multi-select, fetches только при open, loading disables submit, fetch error → inline campaigns error, submit без выбора `(id, [])`, submit с выбором → forwards ids + invalidates per-campaign keys, 422 NOT_APPROVABLE → onApiError + close dialog (старый UX), 422 CAMPAIGN_NOT_AVAILABLE_FOR_ADD → inline error без invalidate (новый UX), 422 CAMPAIGN_IDS_TOO_MANY → inline, 404 → close drawer + dialog, 500 / non-ApiError → fallback retry-error, Escape, double-submit guard.

Frontend — e2e:
- [x] `frontend/e2e/web/admin-creator-applications-moderation-approve-with-campaigns.spec.ts` — NEW FILE с обязательным JSDoc-нарративом на русском. Сценарий: seed creator-application + 2 active campaigns через `seedCampaign` → login admin → открыть moderation drawer → выбрать обе кампании в `SearchableMultiselect` → approve → assert success closes dialog + drawer → assert через `GET /campaigns/{id}/creators` что creator в обеих кампаниях со статусом `planned`. Cleanup LIFO: campaign_creators detach → creator → application → campaigns. Soft-deleted сценарий не покрыт (нет soft-delete API).

**Acceptance Criteria:**

- Given empty/null/missing `campaignIds` in body, when admin approves, then existing approve flow runs unchanged (creator created in tx1, audit row, telegram-notify), no `campaign_creators` rows created.
- Given valid `campaignIds=[A,B,C]`, when admin approves, then audit_logs contains exactly 1 `creator_application_approve` row + 3 `campaign_creator_add` rows in chronological order; `campaign_creators` table has 3 rows with status='planned'.
- Given `campaignIds` with duplicates or `len > 20`, when admin approves, then 422 with corresponding code, no `creator` row created in `creators`, application stays in `moderation`.
- Given `campaignIds` with non-existent or soft-deleted UUID, when admin approves, then 422 `CAMPAIGN_NOT_AVAILABLE_FOR_ADD`, no `creator` row created, application stays in `moderation`.
- Given pre-validation passes but `Add` fails on N-th campaign (race), when admin approves, then creator created, telegram-notify sent, first N-1 campaigns get `campaign_creators` rows, N-th and later — none; response is 4xx with actionable `error.Message` ("Не удалось добавить креатора в кампанию <name|id>. Креатор создан, добавьте вручную через страницу кампании.").
- Given UI submit with selected campaigns, when approve succeeds, then dialog closes, queries invalidated for moderation list, creators list, campaign-creators of each selected campaign.
- Given UI submit with selected campaigns, when approve returns 4xx mid-cycle, then dialog stays open with inline error from `error.Message`, queries still invalidated to keep UI consistent.

## Spec Change Log

### 2026-05-07 — round 1 review patches

- **bad_spec → patched in non-frozen sections.**
  - Always #7: `pageSize: 100` → `perPage: 100` (matches `CampaignsListInput` actual field name from openapi.yaml).
  - I/O Matrix `race mid-cycle` row: `422 CAMPAIGN_NOT_FOUND` was internally inconsistent (response.go maps `ErrCampaignNotFound` to 404). Replaced behaviour with new domain code `CAMPAIGN_ADD_AFTER_APPROVE_FAILED` (422 + actionable message «Не удалось добавить креатора в кампанию <id>. Креатор уже создан — добавьте его вручную через страницу кампании.»).
- **patch findings (non-loopback):**
  - Backend: post-tx1 add-loop wraps the request ctx in `context.WithoutCancel(ctx)` — request cancellation after commit must not strand a partial-add creator. Mirrors notifyApplication* patterns.
  - Backend: any `Add` failure inside the loop is translated into `domain.NewErrCampaignAddAfterApproveFailed(campaignID)` with an actionable user-facing message; the raw error is logged at Error-level with `creator_id`, `committed_count`, `failed_campaign_id`, `remaining_count`.
  - Backend: `CampaignActiveChecker` extracted into a single-method handler-level interface (was glued to `CampaignService`); `CampaignCreatorAdder` made unexported (`campaignCreatorAdder`) per Go convention.
  - Backend: `backend/api/openapi.yaml` description corrected — removed contradictory «No body is accepted» paragraph; 422 description lists all 4 new codes.
  - Frontend: `creatorApplications.ts.approveApplication` propagates `error.message` from backend into the new `ApiError.serverMessage` slot.
  - Frontend: `ApproveApplicationDialog.onSuccess` now also invalidates `creatorKeys.all()` (creator was just materialised; «Все креаторы» must reflect it).
  - Frontend: `ApproveApplicationDialog.onError` whitelist for inline-error grew to include `CAMPAIGN_ADD_AFTER_APPROVE_FAILED`. For that code the dialog stays open with the actionable backend message and invalidates per-campaign queries plus creators/applications, since the creator was created and some campaigns may already be attached.
  - Frontend i18n: `common.json` got fallback texts for all 4 new error codes plus `CREATOR_ALREADY_IN_CAMPAIGN` (used as a fallback when `serverMessage` is unavailable).
  - Tests: handler/service/dialog unit-tests updated to the new contracts (12-arg `NewServer`, `MockCampaignActiveChecker`, `MockcampaignCreatorAdder`, lowercase interface, `serverMessage` propagation in dialog).
- **deviation accepted (Always #10):** `backend/e2e/creator_applications/approve_test.go` modified — pure compile-fix forced by oapi-codegen signature change for `ApproveCreatorApplicationWithResponse(... body, ...)`; semantics of the existing tests unchanged. Documented here per Always #10 / Never #7.
- **rejected (noise):** TOCTOU finding on `AssertActiveCampaigns` (by design — early-fail UX optimisation; the per-campaign `Add` provides defense-in-depth); cap-vs-dedupe order in handler; zero-UUID handling.
- **deferred:** `CampaignRepo.ListByIDs` cap — handler caps at 20, public-API misuse handled separately if/when needed; `listCampaigns({ perPage: 100 })` UI scaling — adequate while the campaign roster is small, server-search needed when admins routinely cross 100 campaigns.

## Design Notes

**Sequence (handler → service):**

```
Handler ApproveCreatorApplication
  ├── parse body, normalize campaignIds (nil/[] → "no add")
  ├── if non-empty: validate len ≤20 → 422 CAMPAIGN_IDS_TOO_MANY
  ├── if non-empty: dedupe check → 422 CAMPAIGN_IDS_DUPLICATES
  ├── if non-empty: campaignService.AssertActiveCampaigns(ctx, ids)
  │     └── on err → 422 CAMPAIGN_NOT_AVAILABLE_FOR_ADD (service approve НЕ вызывается)
  └── creatorApplicationService.ApproveApplication(ctx, applicationID, actorUserID, campaignIDs)
        ├── tx1: transition + create creator + audit (как сейчас)
        ├── notifyApplicationApproved (fire-and-forget, как сейчас)
        ├── for _, id := range campaignIDs:
        │     ├── campaignCreatorService.Add(ctx, id, []string{creatorID})  ← своя tx + audit
        │     └── on first fail: STOP, return wrapped err
        └── return creatorID, nil
```

**Почему pre-validation в handler, а не в service approve:** решение tech lead'а (intent #9) — единая точка валидации входа, до сервиса approve доходят только провалидированные данные. Race между pre-check и циклом add ловится через first-fail-stop (defense in depth).

**Зависимости (новые):**
- `CreatorApplicationHandler` ← `CampaignActiveChecker` (локальный интерфейс с одним методом `AssertActiveCampaigns`); реализует `*CampaignService`.
- `CreatorApplicationService` ← `CampaignCreatorAdder` (локальный интерфейс с одним методом `Add(ctx, campaignID, []string{creatorID})`); реализует `*CampaignCreatorService`.

Обе по go-convention "accept interfaces, return structs"; обе через конструктор (никаких `Set*` на иммутабельную структуру per `backend-design.md`).

## Verification

**Commands:**
- `make generate-api` — после правки `backend/api/openapi.yaml`; ожидаемо: обновлены `*.gen.go` и `frontend/*/generated/schema.ts`.
- `make lint-backend` — ожидаемо: 0 ошибок.
- `make test-unit-backend` — ожидаемо: PASS включая новые сценарии.
- `make test-unit-backend-coverage` — ожидаемо: PASS, gate ≥80% per-method на handler/service/repository новых функций.
- `make test-e2e-backend` — ожидаемо: PASS включая `TestApproveWithCampaigns`.
- `make lint-web` — ожидаемо: 0 TS/ESLint ошибок.
- `make test-unit-web` — ожидаемо: PASS.
- `make test-e2e-frontend` — ожидаемо: PASS включая новый spec.

**Manual checks:**
- `git diff backend/api/openapi.yaml` — `campaignIds` в `approveCreatorApplication` request body как optional array; новые коды в error response examples.
- `cat backend/internal/domain/errors.go | grep -E "CAMPAIGN_IDS_(TOO_MANY|DUPLICATES)|CAMPAIGN_NOT_AVAILABLE_FOR_ADD"` — три новых кода присутствуют.
- Self-check агента после backend, до e2e: `curl -X POST .../creators/applications/<id>/approve -d '{"campaignIds":["<a>","<b>"]}'` → 200; `psql -c "SELECT count(*) FROM campaign_creators WHERE creator_id=<creator>"` → 2; `psql -c "SELECT action FROM audit_logs WHERE entity_id IN (...)"` → ровно 1 approve + 2 add.

## Suggested Review Order

**Контракт ручки**

- Расширение `approveCreatorApplication` опциональным `campaignIds[]` + новый код `CAMPAIGN_ADD_AFTER_APPROVE_FAILED`.
  `backend/api/openapi.yaml:785`
- Сгенерированный тип `CreatorApprovalInput` для request body.
  `backend/api/openapi.yaml:2348`

**Domain — error codes**

- 4 новых `Code*` для всей цепочки validation + post-approve.
  `backend/internal/domain/errors.go:31`
- Sentinel errors + actionable message-конструктор для post-approve fail.
  `backend/internal/domain/campaign.go:46`

**Backend — pre-validation в handler**

- Главная точка входа: `parseApproveCampaignIDs` cap 20 → dedupe → `AssertActiveCampaigns`.
  `backend/internal/handler/creator_application.go:368`
- Локальный single-method интерфейс по convention "accept interfaces, return structs".
  `backend/internal/handler/server.go:108`

**Backend — service approve cycle**

- Sequential add loop под `context.WithoutCancel(ctx)`, first-fail-stop с actionable error и логированием partial-state.
  `backend/internal/service/creator_application.go:1308`
- Внутренний `campaignCreatorAdder` (lowercase) — consumer-side dependency.
  `backend/internal/service/creator_application.go:69`

**Backend — repo + service для pre-validation**

- `AssertActiveCampaigns`: noop при пустом, иначе ListByIDs + сравнение длин и `IsDeleted`.
  `backend/internal/service/campaign.go:125`
- `ListByIDs` через squirrel `WHERE id IN (...)` с empty-input guard.
  `backend/internal/repository/campaign.go:113`

**Frontend — API + dialog UX**

- `approveApplication(id, campaignIds?)` пробрасывает `error.message` от бэка в `ApiError.serverMessage`.
  `frontend/web/src/api/creatorApplications.ts:78`
- Диалог: useQuery campaigns, SearchableMultiselect, discriminator inline-error vs drawer-banner, invalidate creators/campaigns на success и на post-approve fail.
  `frontend/web/src/features/creatorApplications/components/ApproveApplicationDialog.tsx:51`
- 4 новых i18n-fallback'а (используются если backend не вернул `message`).
  `frontend/web/src/shared/i18n/locales/ru/common.json:43`

**Backend — unit-тесты**

- Сервис: happy с N кампаний (captured order) + first-fail-stop (actionable error + log).
  `backend/internal/service/creator_application_test.go:2716`
- Handler: validation 422 / pre-validation / forwards campaignIDs.
  `backend/internal/handler/creator_application_test.go:1672`
- AssertActiveCampaigns + ListByIDs.
  `backend/internal/service/campaign_test.go:589`
  `backend/internal/repository/campaign_test.go:158`

**Backend — e2e**

- Happy + 422 sub-scenarios на новой ручке.
  `backend/e2e/creator_applications/approve_with_campaigns_test.go:74`

**Frontend — unit + e2e**

- Dialog unit-tests: 22 сценария (multiselect, loading, validation, post-approve fail, success invalidate).
  `frontend/web/src/features/creatorApplications/components/ApproveApplicationDialog.test.tsx:1`
- Browser e2e: full happy path с двумя кампаниями + assert обоих roster'ов.
  `frontend/e2e/web/admin-creator-applications-moderation-approve-with-campaigns.spec.ts:74`

**Bootstrap / wiring**

- main.go: campaignCreatorSvc создаётся ДО creatorApplicationSvc; campaignSvc передан и как `CampaignService`, и как `CampaignActiveChecker`.
  `backend/cmd/api/main.go:99`
