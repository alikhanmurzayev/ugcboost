---
title: "Approve action в drawer на moderation-экране (chunk 19)"
type: feature
created: "2026-05-05"
status: done
baseline_commit: a0222e3d0346bc9fbd64661acb5d81c780991ff1
context:
  - docs/standards/
  - _bmad-output/planning-artifacts/creator-onboarding-roadmap.md
  - _bmad-output/implementation-artifacts/archive/2026-05-04-spec-creator-application-reject-frontend.md
---

> Перед реализацией обязательно полностью загрузить все файлы `docs/standards/` — это hard rules, спека не дублирует.

<frozen-after-approval reason="human-owned intent — do not modify unless human renegotiates">

## Intent

**Problem:** Backend approve (chunk 18.5, PR #66) и Telegram-уведомление (chunk 20) уже в проде, но в admin-UI на moderation-drawer'е стоит `disabled`-placeholder с tooltip «Скоро» (`ApplicationActions.tsx:41-49`). Модератор не может одобрить заявку через интерфейс — приходится дёргать endpoint руками, что не работает в живой эксплуатации.

**Approach:** Зеркало `RejectApplicationDialog` под approve. Заменить disabled-placeholder в `ApplicationActions.tsx` (case `moderation`) на `<ApproveApplicationDialog />` — отдельный компонент с trigger-button (emerald) и confirm-modal (без telegramLink-conditional, потому что approve guard'ит наличие TG на бэке: 422 `TELEGRAM_NOT_LINKED` показывается через единый `apiError`-banner). Добавить `approveApplication(applicationId)` в `api/creatorApplications.ts` (зеркало `rejectApplication`). Сначала реализация + unit-тесты, потом ручной smoke через Playwright MCP, и только при успехе — отдельный e2e spec по образцу reject-action.

## Boundaries & Constraints

**Always:**

- Использовать generated `CreatorApprovalResult` (`schema.ts:1074`) и operation `approveCreatorApplication` через `client.POST("/creators/applications/{id}/approve", {params:{path:{id}}})`. Никаких ручных interface для request/response.
- Mutation pattern: `useMutation` + `onSuccess` (invalidate `creatorApplicationKeys.all()` + close dialog + close drawer) + `onError` + `onSettled` (сброс `isSubmitting`).
- Error handling — паттерн зеркало reject (`RejectApplicationDialog.tsx:36-55`): 4xx (status<500 && code !== INTERNAL_ERROR) → `onApiError(getErrorMessage(code))` + invalidate + close dialog; 404 дополнительно closes drawer; 5xx/network → inline error в диалоге, диалог остаётся.
- Double-submit guard: external `isSubmitting` boolean + `isPending` from mutation, сбрасывается в `onSettled`.
- Disabled при pending: submit, cancel, backdrop. Submit-text меняется на `actions.approving`.
- Escape closes dialog (если не pending) — обработчик с `e.stopImmediatePropagation()` чтобы не закрылся drawer.
- data-testid на каждом интерактиве: `approve-button`, `approve-confirm-dialog`, `approve-confirm-backdrop`, `approve-confirm-cancel`, `approve-confirm-submit`, `approve-dialog-error`.
- i18n через `react-i18next` (namespace `creatorApplications.approveDialog.*` и `creatorApplications.actions.{approve,approving}` + `common.cancel`); error codes — в `common.errors.*`.
- Все 4 approve-related ErrorCode'а покрываются переводами в `common.json`: `CREATOR_APPLICATION_NOT_APPROVABLE`, `CREATOR_ALREADY_EXISTS`, `CREATOR_TELEGRAM_ALREADY_TAKEN` (новые). `CREATOR_APPLICATION_TELEGRAM_NOT_LINKED` уже есть.
- Trigger-button — emerald-outlined (`border border-emerald-600 text-emerald-700`, hover `bg-emerald-50`); confirm submit — solid emerald (`bg-emerald-600 hover:bg-emerald-700 text-white`). Точные tailwind-классы возьмите по образцу reject (red → emerald), без новых palette-токенов.
- Manual smoke через Playwright MCP — обязательная gate перед e2e: happy / cancel / race (двойное окно одной заявки → 422). Без зелёного smoke'а e2e spec не создаётся.

**Ask First (BLOCKING до Execute):**

- (нет — все вопросы зарезолвлены: тексты i18n утверждены § "Тексты i18n", e2e spec — отдельный файл).

**Never:**

- Менять backend (chunk 18.5/20 уже в проде).
- Менять reject-flow, manual-verify-flow, list/filters.
- Conditional `hasTelegram` body в approve-dialog: на moderation TG практически всегда есть, а потеря link — corner case покрытый 422-баннером. Дополнительный текст создавал бы ложное впечатление, что approve без TG возможен.
- Comment / feedback-поле в approve (body endpoint'а пустой, изменения текста ноты — отдельный PR'ом константой).
- Навигация на `/creators/{creatorId}` после approve — это другой чанк (read-screen креатора).
- Кеш `creatorId` в Zustand / поверх React Query.
- Чужие компоненты-обёртки (`ConfirmDialog`-shared) — паттерн reject inline и оптимально читается, не оверинженерить.

## I/O & Edge-Case Matrix

| Сценарий | Состояние | Поведение |
|---|---|---|
| Trigger render | drawer открыт, app `moderation` | `approve-button` viewable, label `actions.approve`, dialog hidden |
| Open dialog | click `approve-button` | `approve-confirm-dialog` viewable, body `approveDialog.body`, submit/cancel/backdrop active |
| Cancel / backdrop / Escape | dialog open, not pending | dialog закрывается, drawer остаётся, server-state не трогается |
| Submit 200 | `mutation` returns `{data:{creatorId}}` | invalidate `creatorApplicationKeys.all()`, dialog closes, drawer closes; заявка уходит из moderation-листа на refetch |
| 422 NOT_APPROVABLE / TELEGRAM_NOT_LINKED / CREATOR_ALREADY_EXISTS / CREATOR_TELEGRAM_ALREADY_TAKEN | `ApiError(422, code)` | `onApiError(getErrorMessage(code))` + invalidate + close dialog; drawer остаётся, баннер `[drawer-api-error]` показывает текст |
| 403 FORBIDDEN | `ApiError(403, "FORBIDDEN")` | то же |
| 404 NOT_FOUND | `ApiError(404, ...)` | onApiError + invalidate + **close dialog AND drawer** |
| 500 INTERNAL_ERROR / network throw | rejection не ApiError или 5xx | inline `[approve-dialog-error]` (`approveDialog.retryError`); диалог открыт, drawer открыт; нет invalidate; submit активен заново |
| Pending | mutation in-flight | submit/cancel/backdrop disabled; submit text = `actions.approving`; повторные клики на submit игнорируются (counter в моке = 1) |

</frozen-after-approval>

## Code Map

- `frontend/web/src/api/creatorApplications.ts` — добавить `approveApplication(applicationId)` зеркалом `rejectApplication` (`api/creatorApplications.ts:60-71`). Сигнатура: `(id: string) => Promise<{data: {creatorId: string}}>`.
- `frontend/web/src/features/creatorApplications/components/ApproveApplicationDialog.tsx` — новый компонент. Зеркало `RejectApplicationDialog.tsx` без `hasTelegram` prop и conditional body. Props: `{applicationId, onApiError, onCloseDrawer}`.
- `frontend/web/src/features/creatorApplications/components/ApproveApplicationDialog.test.tsx` — новые юнит-тесты по структуре `RejectApplicationDialog.test.tsx`: trigger / open / cancel(+backdrop) / 200 / 422 NOT_APPROVABLE / 422 TELEGRAM_NOT_LINKED / 403 / 404 (closes drawer) / 5xx inline / network inline / pending disable / double-submit guard.
- `frontend/web/src/features/creatorApplications/components/ApplicationActions.tsx` — case `moderation`: заменить `<button disabled ...>` на `<ApproveApplicationDialog applicationId onApiError onCloseDrawer />`. Импорт + удаление tooltip-классов и `actions.approveDisabledHint`.
- `frontend/web/src/features/creatorApplications/components/ApplicationActions.test.tsx` — обновить кейс `renders reject + disabled approve placeholder for moderation status`: ожидать активный `approve-button`, click → `approve-confirm-dialog` visible.
- `frontend/web/src/features/creatorApplications/ModerationPage.test.tsx` — обновить кейс на `approve-button` enabled (без `toBeDisabled`).
- `frontend/web/src/shared/i18n/locales/ru/creatorApplications.json` — добавить `approveDialog: {title, body, submit, retryError}`, `actions.approving = "Одобрение..."`; **удалить** `actions.approveDisabledHint` (мёртвый ключ).
- `frontend/web/src/shared/i18n/locales/ru/common.json` — добавить 3 error code'а: `CREATOR_APPLICATION_NOT_APPROVABLE`, `CREATOR_ALREADY_EXISTS`, `CREATOR_TELEGRAM_ALREADY_TAKEN` (actionable user-facing messages, см. § Manual Smoke ниже для рекомендованных текстов).
- `frontend/e2e/web/admin-creator-applications-moderation-approve-action.spec.ts` — **создаётся только после успешного manual smoke**. Зеркало `admin-creator-applications-moderation-reject-action.spec.ts`. 3 сценария:
  - Happy с TG: setup IG-only → webhook → moderation → admin UI → approve → 200; admin GET application → status=approved; `collectTelegramSent` ловит ≥1 message в чат creator'а.
  - Cancel: open dialog → cancel; admin GET → всё ещё `moderation`.
  - 422 NOT_APPROVABLE: race-симуляция — две сессии (или прямой backend-approve через `request.post(...)` после открытия drawer'а), submit во второй → `[drawer-api-error]` с текстом `CREATOR_APPLICATION_NOT_APPROVABLE`.
  - Reuse `setupModerationViaIG` из reject-action — либо вынести его в `helpers/api.ts`, либо скопировать локально (текущий паттерн — локально). Default: **локально**, копипаст 40 строк проще, чем premature shared-helper.
- `_bmad-output/planning-artifacts/creator-onboarding-roadmap.md` — отметить chunks 18.5 и 19 как `[x]` в финальном коммите; убрать соответствующий `[~]`-маркер у 18.6 (он уже `[x]` в main).

## Tasks & Acceptance

**Pre-execution gates:**
- [x] Backend approve (chunk 18.5) в main — endpoint работает (`backend/api/openapi.yaml:785`).
- [x] Frontend types сгенерированы — `CreatorApprovalResult` есть (`schema.ts:1074`).
- [x] Branch `alikhan/creator-approve-action-frontend` создан от свежего main, working tree чистый.

**Execution (Phase 1 — реализация + unit):**
- [x] `frontend/web/src/api/creatorApplications.ts` — добавить `approveApplication`.
- [x] `frontend/web/src/features/creatorApplications/components/ApproveApplicationDialog.tsx` — новый компонент.
- [x] `frontend/web/src/features/creatorApplications/components/ApproveApplicationDialog.test.tsx` — юнит-тесты.
- [x] `frontend/web/src/features/creatorApplications/components/ApplicationActions.tsx` — replace placeholder.
- [x] `frontend/web/src/features/creatorApplications/components/ApplicationActions.test.tsx` — обновить кейс.
- [x] `frontend/web/src/features/creatorApplications/ModerationPage.test.tsx` — обновить кейс.
- [x] `frontend/web/src/shared/i18n/locales/ru/{creatorApplications,common}.json` — i18n keys.
- [x] `make build-web lint-web test-unit-web` зелёные.

**Execution (Phase 2 — manual smoke):**
- [x] `make start-backend && make start-web` (или `make compose-up && make migrate-up && make run-backend` + `make run-web`).
- [x] Через Playwright MCP-браузер: seed admin → seed app → linkTG → IG webhook → /creator-applications/moderation → drawer → approve → success.
- [x] Cancel-сценарий: open dialog → cancel → server-state не трогается.
- [x] 422 NOT_APPROVABLE race: два окна, approve в первом, потом submit во втором → красный баннер с локализованным текстом.
- [x] Зафиксировать скриншоты или текстовый лог результата в коммите (или в этой спеке как `Spec Change Log`).

**Execution (Phase 3 — e2e, только после зелёного smoke):**
- [x] `frontend/e2e/web/admin-creator-applications-moderation-approve-action.spec.ts` — создать.
- [x] Spec локально зелёный (3 passed) — `make test-e2e-frontend` ниже подтверждает на полном прогоне.

**Execution (Phase 4 — финализация):**
- [x] `_bmad-output/planning-artifacts/creator-onboarding-roadmap.md` — chunks 18.5, 18.6 и 19 → `[x]`.
- [x] `make build-web lint-web test-unit-web test-e2e-frontend` — все четыре зелёные (38/38 frontend e2e).
- [ ] Spec архивируется через `/finalize` после merge.

**Acceptance Criteria:**
- Given drawer открыт на app `moderation`, when click `approve-button`, then `approve-confirm-dialog` visible с body из i18n.
- Given dialog open, when cancel/backdrop/Escape, then dialog closes, drawer stays, server-state untouched.
- Given dialog open, when submit и backend 200, then invalidate `creatorApplicationKeys.all()`, dialog closes, drawer closes, list refetch не содержит app.
- Given backend 422 любым из 4 кодов / 403, when submit, then `[drawer-api-error]` показывает локализованный текст, dialog closes, drawer открыт; invalidate сделан.
- Given backend 404, when submit, then onApiError + invalidate + dialog closes + drawer closes.
- Given 5xx или network throw, then `[approve-dialog-error]` с `approveDialog.retryError`, диалог открыт, нет invalidate.
- Pending state: submit/cancel/backdrop disabled, double-click → ровно 1 mutation call.
- Manual smoke зелёный (3 сценария).
- E2E spec зелёный (3 сценария).
- `make build-web lint-web test-unit-web test-e2e-frontend` все зелёные.

## Тексты i18n (утверждены)

`common.errors.*` (3 новых кода):

- `CREATOR_APPLICATION_NOT_APPROVABLE`: «Заявку нельзя одобрить — статус изменился. Обновите страницу.»
- `CREATOR_ALREADY_EXISTS`: «Креатор с этим ИИН уже существует. Найдите его в реестре.»
- `CREATOR_TELEGRAM_ALREADY_TAKEN`: «Этот Telegram уже привязан к другому креатору.»

`creatorApplications.approveDialog.*`:

- `title`: «Одобрить заявку?»
- `body`: «Заявка перейдёт в статус «Одобрена». Креатор получит уведомление в Telegram-боте.»
- `submit`: «Одобрить»
- `retryError`: «Не удалось одобрить, попробуйте ещё раз»

`creatorApplications.actions.approving`: «Одобрение...»

`creatorApplications.actions.approveDisabledHint` — удалить (placeholder уходит вместе с disabled-кнопкой).

## Verification

**Commands:**
- `cd frontend/web && npx tsc --noEmit && npx eslint src/`
- `make test-unit-web`
- `make test-e2e-frontend` (Phase 3 only)
- `make build-web lint-web test-unit-web test-e2e-frontend` — финальный gate

**Manual checks (Phase 2):** — см. § Tasks Phase 2; через Playwright MCP-браузер на локальном `make start-web` стенде.

## Spec Change Log

**2026-05-05 — Manual smoke зелёный (Phase 2):**
- Стенд: `make start-web` пересобрал `ugcboost-web-1` с текущим бранчем (старый image был на 17 часов отставал — placeholder ещё disabled).
- Сценарий 1 (Happy): seed admin + 3 заявки IG-only → linkTG → IG webhook → все три в `moderation`. Открыл drawer на app1 → approve-button с emerald-классами и `disabled:false`, текст «Одобрить заявку». Click → диалог с правильными текстами (`title=Одобрить заявку?`, body отсылает к Telegram-боту, submit=`Одобрить`, cancel=`Отмена`). Submit → drawer + dialog закрылись, total Модерации 3→2, app1 на бэке `status=approved`.
- Сценарий 2 (Cancel): drawer на app2 → approve-button → диалог → cancel → диалог закрыт, drawer открыт, total=2 без изменений, app2 на бэке `status=moderation`.
- Сценарий 3 (Race 422): drawer на app3 → approve-button → диалог открыт; параллельно `POST /creators/applications/{app3}/approve` напрямую → 200 (status стал `approved`); submit в UI → диалог закрыт, drawer открыт, `[drawer-api-error]` показал «Заявку нельзя одобрить — статус изменился. Обновите страницу.» (точный текст для `CREATOR_APPLICATION_NOT_APPROVABLE`); invalidate отработал — app3 ушла из листа (теперь total=1).
- Cleanup частичный (app1/app3 approved → cleanup-entity 500 без cascade на creator; app2 + admin — 204). Локалка, не блокер для дальнейшего e2e.

**2026-05-05 — Review loop #1 (3 субагента: blind-hunter, edge-case-hunter, acceptance-auditor):**

Patches применены:
- e2e Happy: `Promise.all([waitForResponse, click])` вместо последовательного `click(); await promise` — устраняет потенциальный race на быстрых машинах.
- e2e Happy + Race: `expect(creatorId).toMatch(uuid-regex)` перед `cleanupCreator` push — защита от orphan creator'а если backend когда-нибудь вернёт пустой/null `data`.
- unit: добавлены два теста Escape-key — `closes dialog when not pending` и `ignores Escape while pending` — закрывает AC «cancel/backdrop/Escape» который был протестирован только для cancel/backdrop.

Defer'ы записаны в `deferred-work.md`: 8 пунктов (Escape global swallow, отсутствие ключей `common.errors.UNAUTHORIZED/429/408`, onSettled на unmount, backdrop a11y, ApiError class drift в моках, нет `I18nextProvider` в unit-тестах, `w-[420px]` min-width, стилистика точек в i18n). Все — pre-existing reject pattern или cross-cutting concerns, не in scope chunk 19.

Reject'ы (silent): `isSubmitting` дубликат `mutation.isPending` (спека требует), хрупкая логика `code !== "INTERNAL_ERROR"` (спека определяет), `handleSubmit` double guard (намеренная защита от programmatic dispatch).

Gate после patches: build/lint/unit/e2e — все четыре зелёные (38/38 frontend e2e).

## Suggested Review Order

**UI-integration**

- Entry point: где живой approve-button подменяет старый disabled-placeholder в moderation-кейсе.
  [`ApplicationActions.tsx:30-46`](../../frontend/web/src/features/creatorApplications/components/ApplicationActions.tsx#L30)

**Mutation core**

- Новый dialog зеркалит RejectApplicationDialog без `hasTelegram`-conditional; emerald, double-submit guard, Escape с `stopImmediatePropagation`.
  [`ApproveApplicationDialog.tsx:1`](../../frontend/web/src/features/creatorApplications/components/ApproveApplicationDialog.tsx#L1)

- Error-handling ветки: 4xx → `apiError`-banner + close, 404 — ещё close drawer, 5xx/network — inline retry; зеркало reject.
  [`ApproveApplicationDialog.tsx:36-55`](../../frontend/web/src/features/creatorApplications/components/ApproveApplicationDialog.tsx#L36)

**API surface**

- `approveApplication(id)` — тонкий wrapper над generated `paths`-вызовом, без ручных interface; зеркало `rejectApplication`.
  [`creatorApplications.ts:73-84`](../../frontend/web/src/api/creatorApplications.ts#L73)

**i18n**

- Новый `approveDialog.*` namespace + `actions.approving`; удалён мёртвый `actions.approveDisabledHint`.
  [`creatorApplications.json:81-110`](../../frontend/web/src/shared/i18n/locales/ru/creatorApplications.json#L81)

- Три новых `errors.*` кода (NOT_APPROVABLE, CREATOR_ALREADY_EXISTS, CREATOR_TELEGRAM_ALREADY_TAKEN) для admin-баннера.
  [`common.json:40-45`](../../frontend/web/src/shared/i18n/locales/ru/common.json#L40)

**Unit tests**

- Покрытие 9 сценариев матрицы из спеки (trigger / open / cancel / 200 / 4×422 / 403 / 404 / 5xx / network / pending / double-submit).
  [`ApproveApplicationDialog.test.tsx:1`](../../frontend/web/src/features/creatorApplications/components/ApproveApplicationDialog.test.tsx#L1)

- Escape-ветка после review-loop'а: closes when not pending / ignored when pending.
  [`ApproveApplicationDialog.test.tsx:227-249`](../../frontend/web/src/features/creatorApplications/components/ApproveApplicationDialog.test.tsx#L227)

- Кейс moderation в ApplicationActions перепроверяет активный approve и open-on-click.
  [`ApplicationActions.test.tsx:88-99`](../../frontend/web/src/features/creatorApplications/components/ApplicationActions.test.tsx#L88)

- ModerationPage перешёл с disabled на enabled approve-button.
  [`ModerationPage.test.tsx:195-209`](../../frontend/web/src/features/creatorApplications/ModerationPage.test.tsx#L195)

**Browser e2e**

- 3 сценария Happy/Cancel/Race-422; нарративный JSDoc по стандарту, локальный `setupModerationViaIG`, `Promise.all` вокруг submit для устойчивости race с `waitForResponse`.
  [`admin-creator-applications-moderation-approve-action.spec.ts:1`](../../frontend/e2e/web/admin-creator-applications-moderation-approve-action.spec.ts#L1)

- Cleanup creator перед application — приватный helper `cleanupCreator`, защищён `expect(creatorId).toMatch(uuid)`.
  [`admin-creator-applications-moderation-approve-action.spec.ts:387-405`](../../frontend/e2e/web/admin-creator-applications-moderation-approve-action.spec.ts#L387)

- Сосед: shape-spec moderation теперь проверяет enabled-approve, не placeholder.
  [`admin-creator-applications-moderation.spec.ts:287-292`](../../frontend/e2e/web/admin-creator-applications-moderation.spec.ts#L287)

**Roadmap / docs**

- Chunks 18.5, 18.6, 19 → `[x]`; chunk 19 пополнен описанием итогового PR'а.
  [`creator-onboarding-roadmap.md:88-90`](../planning-artifacts/creator-onboarding-roadmap.md#L88)

- Defer'ы из review (8 пунктов confirm-dialog pattern, унаследованных от reject).
  [`deferred-work.md:1`](deferred-work.md#L1)
