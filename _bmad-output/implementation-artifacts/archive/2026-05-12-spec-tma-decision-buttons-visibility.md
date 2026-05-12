---
title: 'TMA: видимость accept/decline только при invited'
type: 'feature'
created: '2026-05-11'
status: 'done'
baseline_commit: b24b628bd20ffb75a2551cb485b81ec1cafa11d7
context:
  - docs/standards/backend-codegen.md
  - docs/standards/frontend-api.md
---

<frozen-after-approval reason="human-owned intent — do not modify unless human renegotiates">

## Intent

**Problem:** На странице ТЗ в TMA кнопки «Согласиться/Отказаться» (`CampaignBriefPage.tsx:198-215`) видны всегда после NDA. После подписания договора повторно присылаем тот же ТЗ «для архива» — креатор открывает, снова видит кнопки и путается.

**Approach:** Новая read-only ручка `GET /tma/campaigns/{secretToken}/participation` отдаёт `{status}`. TMA на mount вызывает её, блок кнопок рендерится только при `status === "invited"`.

## Boundaries & Constraints

**Always:**
- Backend: только OpenAPI + новый handler. `AuthzService.AuthorizeTMACampaignDecision` уже возвращает `CurrentStatus` (`authz/tma.go:60`) — handler делает regex-check, вызывает authz, возвращает status. Всё.
- OpenAPI: тот же стиль, что у `tmaAgree`/`tmaDecline` (security, params, 401/403/404, anti-fingerprint).
- TMA: `apiClient` (raw `fetch` запрещён), query key через фабрику.

**Never:**
- Не трогать service / repository / authz / audit.
- Не трогать NDA-gate, рендеринг брифа, `AcceptedView`/`DeclinedView`.
- Не добавлять поля помимо `status`; не вводить rate-limit; не делать кэш-инвалидацию.
- Не редактировать `*.gen.go` / `schema.ts` руками.

## I/O & Edge-Case Matrix

| Scenario | State | Expected |
|---|---|---|
| Бэк success | regex ok + creator в кампании | 200 `{status:<current>}` |
| Бэк auth/lookup ошибки | regex-fail / no initData / not creator / campaign missing / not in campaign | пробрасываем как у `tmaAgree`: 404/401/403 (пути ровно те же — regex-reject + `AuthorizeTMACampaignDecision`) |
| TMA: status=invited | data загружено | блок кнопок виден после NDA |
| TMA: status≠invited | любой другой статус | блок скрыт, бриф рендерится |
| TMA: loading / error | `isLoading\|isError` | блок скрыт |

</frozen-after-approval>

## Code Map

- `backend/api/openapi.yaml` -- новый GET endpoint + `TmaParticipationResult`.
- `backend/internal/handler/tma_campaign_creator.go` -- метод `TmaGetParticipation`.
- `backend/internal/handler/tma_campaign_creator_test.go` -- тесты handler.
- `backend/e2e/tma/<existing>_test.go` -- 1 success-сценарий (auth-пути уже покрыты тестами agree/decline).
- `frontend/tma/src/shared/api/queryKeys.ts` -- **новый**, фабрика keys.
- `frontend/tma/src/features/campaign/useParticipationStatus.ts` -- **новый** хук.
- `frontend/tma/src/features/campaign/useParticipationStatus.test.tsx` -- **новый** unit-тест.
- `frontend/tma/src/features/campaign/CampaignBriefPage.tsx` -- условный рендер блока `:188-216`.
- `frontend/tma/src/features/campaign/CampaignBriefPage.test.tsx` -- 2 теста: `invited` → кнопки видны, `signed` → нет.
- `frontend/e2e/tma/<existing>.spec.ts` -- 1 negative: creator в `signed` → бриф виден, кнопок нет.

## Tasks & Acceptance

**Execution:**

- [x] `backend/api/openapi.yaml` -- Добавить GET `/tma/campaigns/{secretToken}/participation` (operationId `tmaGetParticipation`, тег `tma`, security `tmaInitData`, `$ref TmaSecretTokenPathParam`) и схему `TmaParticipationResult{status: $ref CampaignCreatorStatus}`. Responses: 200, 401, 403, 404, default. Без 422 (read-only).
- [x] `make generate-api` -- регенерировать.
- [x] `backend/internal/handler/tma_campaign_creator.go` -- `TmaGetParticipation`: regex-fail → `ErrCampaignNotFound`; `auth, err := s.authzService.AuthorizeTMACampaignDecision(...)`; return 200 `{Status: api.CampaignCreatorStatus(auth.CurrentStatus)}`. Godoc — однострочник.
- [x] `backend/internal/handler/tma_campaign_creator_test.go` -- `TestServer_TmaGetParticipation` (`t.Parallel()`, новый mockery-мок на каждый t.Run): regex-fail (без вызова authz); authz error пробрасывается; success table-driven по всем 7 значениям `CampaignCreatorStatus`.
- [x] `backend/e2e/tma/<existing>_test.go` -- `TestTmaGetParticipation` (`t.Parallel()`): 1 success-сценарий `invited` → 200 `"invited"`. Auth/regex пути идентичны agree/decline и там уже покрыты — не дублируем.
- [x] `frontend/tma/src/shared/api/queryKeys.ts` -- `tmaQueryKeys.participation(secretToken) = ["tma","participation",secretToken] as const`.
- [x] `frontend/tma/src/features/campaign/useParticipationStatus.ts` -- `useQuery` + `apiClient.GET("/tma/campaigns/{secretToken}/participation",...)`. `enabled: !!secretToken && secretTokenFormat.test(secretToken)` (regex из `campaigns.ts:387`). Type-guard по аналогии с `isDecisionResult` (`useDecision.ts:20-28`).
- [x] `frontend/tma/src/features/campaign/useParticipationStatus.test.tsx` -- 2 теста: success → data.status; пустой/невалидный token → `enabled=false`, fetch не вызван. Мок `apiClient.GET` напрямую.
- [x] `frontend/tma/src/features/campaign/CampaignBriefPage.tsx` -- подключить хук, обернуть блок кнопок `:188-216` в `participation.data?.status === CAMPAIGN_CREATOR_STATUS.INVITED`. Поведение после клика — без изменений.
- [x] `frontend/tma/src/features/campaign/CampaignBriefPage.test.tsx` -- 2 теста в новом describe: `invited` → обе кнопки `getByTestId`; `signed` → обе `queryByTestId` === null. Мок `apiClient.GET` через `vi.mock`.
- [x] `frontend/e2e/tma/<existing>.spec.ts` -- 1 negative-сценарий: creator в `signed` (через `helpers/api.ts`) → бриф виден, кнопок нет.

**Acceptance Criteria:**

- Given creator в `invited`, when страница открыта и NDA пройден, then обе кнопки видны и работают как сейчас.
- Given creator в любом из {`planned`, `agreed`, `signing`, `signed`, `declined`, `signing_declined`}, when страница открыта, then кнопки не рендерятся; бриф виден.
- Given loading / network error / 4xx, when страница рендерится, then кнопки не видны (safe fallback).
- `make generate-api && make lint-backend && make lint-tma && make test-unit-backend && make test-unit-backend-coverage && make test-unit-tma && make test-e2e-backend && make test-e2e-frontend` — зелёные, в сгенерированных файлах нет git-diff после ручных правок.

## Spec Change Log

(empty)

## Design Notes

`AuthorizeTMACampaignDecision` уже отдаёт `CurrentStatus` — отдельный service/repo метод не нужен. Имя authz-метода чуть шире сути (он не только про «decision»), но gates те же; не переименовываем.

Это первый `useQuery` в TMA — потому `shared/api/queryKeys.ts` создаётся с нуля (по `frontend-api.md` литералы query keys в компонентах запрещены).

Intent с историей решений: `_bmad-output/implementation-artifacts/intent-tma-decision-buttons-visibility.md` (удалить после CHECKPOINT 1).

## Suggested Review Order

**Backend: новая read-only ручка**

- Точка входа: handler читает `auth.CurrentStatus` и возвращает 200 без UPDATE/audit
  [`tma_campaign_creator.go:55`](../../backend/internal/handler/tma_campaign_creator.go#L55)

- Контракт ручки в OpenAPI — те же anti-fingerprint responses что у `tmaAgree`/`tmaDecline`
  [`openapi.yaml:1799`](../../backend/api/openapi.yaml#L1799)

- Схема ответа — только `status`, ничего лишнего
  [`openapi.yaml:3775`](../../backend/api/openapi.yaml#L3775)

- Handler unit-тест: table-driven по всем 7 значениям `CampaignCreatorStatus`
  [`tma_campaign_creator_test.go:176`](../../backend/internal/handler/tma_campaign_creator_test.go#L176)

- Backend e2e: 1 success-сценарий через реальный invite flow
  [`tma_test.go:444`](../../backend/e2e/tma/tma_test.go#L444)

**Frontend: видимость кнопок**

- Точка входа фронта: подключение хука + `canDecide` условие
  [`CampaignBriefPage.tsx:33`](../../frontend/tma/src/features/campaign/CampaignBriefPage.tsx#L33)

- Условный рендер блока кнопок (обёртка `{canDecide && (...)}`)
  [`CampaignBriefPage.tsx:183`](../../frontend/tma/src/features/campaign/CampaignBriefPage.tsx#L183)

- React Query хук с NDA-gate и permissive type-guard на unknown status
  [`useParticipationStatus.ts:34`](../../frontend/tma/src/features/campaign/useParticipationStatus.ts#L34)

- Фабрика query keys (первый useQuery в TMA — обязательна по `frontend-api.md`)
  [`queryKeys.ts:5`](../../frontend/tma/src/shared/api/queryKeys.ts#L5)

**Тесты фронта**

- Hook-тесты: success / undefined / regex-fail / NDA gate
  [`useParticipationStatus.test.tsx:29`](../../frontend/tma/src/features/campaign/useParticipationStatus.test.tsx#L29)

- Page-тесты: `invited` → кнопки видны / `signed` → кнопок нет
  [`CampaignBriefPage.test.tsx:332`](../../frontend/tma/src/features/campaign/CampaignBriefPage.test.tsx#L332)

- E2E: после agree+reload бриф виден, кнопки скрыты (archived mode)
  [`decision.spec.ts:187`](../../frontend/e2e/tma/decision.spec.ts#L187)
