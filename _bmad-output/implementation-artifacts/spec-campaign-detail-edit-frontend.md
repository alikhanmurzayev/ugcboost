---
title: 'Фронт-страница деталей и редактирования кампании (chunk 8b)'
type: 'feature'
created: '2026-05-07'
status: 'done'
baseline_commit: 'ff85537'
context:
  - 'docs/standards/'
  - '_bmad-output/planning-artifacts/campaign-roadmap.md'
  - '_bmad-output/implementation-artifacts/archive/2026-05-07-spec-campaign-create-frontend.md'
---

> **Перед реализацией:** агент обязан полностью загрузить `docs/standards/` и применять как hard rules.

<frozen-after-approval reason="human-owned intent — do not modify unless human renegotiates">

## Intent

**Problem:** `/campaigns/:campaignId` смонтирован под `RoleGuard(ADMIN)` как stub `<ComingSoonPage testid="campaign-detail-stub" />` (`App.tsx:88`). Бэк-ручки `GET /campaigns/{id}` (#4) и `PATCH /campaigns/{id}` (#5) — в main, но без UI: админ не видит детали и не может править `name` / `tmaUrl`.

**Approach:** `features/campaigns/CampaignDetailPage.tsx` — два режима на одном URL через локальный `isEditing`. **View** (read-only секция «О кампании»: h1=`name` + бейдж «Удалена» при `isDeleted=true`, `tmaUrl` как `<a target="_blank">`, `createdAt`, `updatedAt`, кнопка «Редактировать» — `disabled` при soft-deleted). **Edit** — форма по образу `CampaignCreatePage`, предзаполненная, через `useMutation → updateCampaign`; success (204) → invalidate `campaignKeys.detail(id)` + `all()` → возврат в view, остаёмся на странице. Layout — секциями, чтобы chunk 11 (выбор креаторов) добавил вторую секцию рядом без переписывания.

## Boundaries & Constraints

**Always:**
- AuthZ — существующий `RoleGuard(ADMIN)` в `App.tsx`, без изменений.
- API — `client.GET("/campaigns/{id}")` и `client.PATCH("/campaigns/{id}")` через `openapi-fetch`. Raw `fetch` запрещён.
- Типы — re-export `Campaign`, `GetCampaignResult` из `components["schemas"]`. `CampaignInput` уже re-exported после 8a.
- Query-key — `campaignKeys.detail(id)` (фабрика уже в `queryKeys.ts:28`); на success мутации — invalidate `detail(id)` и `all()`.
- Mutation — `disabled` при `isPending`; double-submit guard через external `isSubmitting`, сбрасываемый в `onSettled` (`frontend-state.md`).
- Loading/Error/Empty — `<Spinner>` на GET, `<ErrorState>` (общий) с retry на network/5xx, dedicated 404-state с back-link.
- i18n — namespace `campaigns` расширяется блоками `detail.*` + `edit.*`; ключи формы (`nameLabel`, `nameRequired` и т.д.) переиспользуются из `create.*`. В `common.json` добавляется `errors.CAMPAIGN_NOT_FOUND` («Кампания не найдена.» из `backend/internal/handler/response.go:75`).
- Form-level error через `getErrorMessage(err.code)` (паттерн 8a / `LoginPage`); поля при ошибке сохраняются.
- `data-testid` на каждом интерактивном элементе и ключевом контейнере.
- `useParams<{ campaignId: string }>()` — имя из `ROUTES.CAMPAIGN_DETAIL_PATTERN`.
- **Self-check без HALT.** После unit-тестов агент сам поднимает `make start-web`, через Playwright MCP проходит I/O Matrix, любые расхождения чинит сам и перепроверяет до сходимости. Затем — Playwright e2e spec в той же сессии. HALT — только при продуктовой развилке, не описанной здесь.

**Ask First:**
- Изменение состава полей view (помимо `name`, `tmaUrl`, `isDeleted`-бейдж, `createdAt`, `updatedAt`).
- Замена inline-toggle на отдельный `/edit`-route.
- Введение библиотек форм (RHF, Formik).
- Field-binding серверных ошибок (маппинг 409 под поле `name`).
- Снятие `disabled` с edit-кнопки для soft-deleted.

**Never:**
- Backend-изменения (контракты #4 и #5 — в main).
- Реализация выбора креаторов (chunk 11) или soft-delete UI (chunk 7) — отдельные PR.
- Импорт UI из `_prototype/`, `features/brands/`, `features/audit/` (legacy).
- HALT с просьбой «потыкай ручками».
- Hardcoded строки JSX-текста (только `t(...)`); `window.confirm/alert`.

## I/O & Edge-Case Matrix

| Сценарий | Состояние | Ожидаемое |
|---|---|---|
| brand_manager на `/campaigns/:id` | manager-токен | RoleGuard редиректит |
| Loading GET | первичный запрос | `<Spinner>` |
| 404 GET | несуществующий id | dedicated state «Кампания не найдена» + back-link; форма не рендерится |
| Network/5xx GET | сервер недоступен | `<ErrorState>` с retry |
| View live | admin, `isDeleted=false` | h1=`name`, карточка полей, кнопка «Редактировать» активна |
| View soft-deleted | admin, `isDeleted=true` | h1=`name` + бейдж «Удалена»; кнопка edit `disabled` + tooltip |
| Click «Редактировать» | view → клик | `isEditing=true`; форма с предзаполненными `name`/`tmaUrl` + кнопки «Сохранить»/«Отменить» |
| Click «Отменить» | edit → клик | `isEditing=false`; локальные изменения отброшены |
| Empty `name` (после trim) | submit | inline error под `name`; PATCH не уходит |
| Empty `tmaUrl` | submit | inline error под `tmaUrl`; PATCH не уходит |
| Submit во время `isPending` | повторный клик | кнопка disabled; double-submit guard блокирует |
| Happy save | непустые поля, имя свободно | 204 → invalidate `detail(id)` + `all()` → `isEditing=false`; view рендерит свежие значения после refetch |
| 409 `CAMPAIGN_NAME_TAKEN` | имя занято другой кампанией | form-level error из `common:errors.CAMPAIGN_NAME_TAKEN`; поля сохранены; пользователь в edit-режиме |
| 422/404 на PATCH | теоретический fallback / race-удаление | form-level error через `getErrorMessage(err.code)` |
| Network/unknown на PATCH | сервер недоступен / нет ключа | fallback `common:errors.unknown` |
| Click back-link | view → клик | `navigate("/campaigns")` |

</frozen-after-approval>

## Code Map

- `frontend/web/src/api/campaigns.ts` (+`.test.ts`) — добавить `getCampaign(id): Promise<GetCampaignResult>` и `updateCampaign(id, input): Promise<void>` (PATCH, 204). Re-export `GetCampaignResult`. Тесты по паттерну `listCampaigns`/`createCampaign`.
- `frontend/web/src/features/campaigns/CampaignDetailPage.tsx` (+`.test.tsx`) — новая страница. `useParams` (`campaignId`), `useQuery → getCampaign`, `useState` (`isEditing`, `isSubmitting`, поля формы, errors), `useMutation → updateCampaign`. Внутренние подкомпоненты в одном файле (`ViewSection`, `EditSection`) ради 150-line guideline.
- `frontend/web/src/App.tsx` — импорт `CampaignDetailPage`, заменить stub под `ROUTES.CAMPAIGN_DETAIL_PATTERN`.
- `frontend/web/src/shared/i18n/locales/ru/campaigns.json` — блоки `detail.*` (h1-helper, лейблы полей в view, бейдж, `editButton`, `editDisabledHint`, `backToList`, `notFoundTitle`, `notFoundMessage`, `loadError`) и `edit.*` (`title`, `submitButton`, `submittingButton`, `cancelButton`).
- `frontend/web/src/shared/i18n/locales/ru/common.json` — `errors.CAMPAIGN_NOT_FOUND`.
- `frontend/e2e/web/admin-campaign-detail.spec.ts` — Playwright spec; русский нарратив-header (`frontend-testing-e2e.md`); сценарии: happy view, happy edit+save (reload подтверждает persistence), validation-empty в edit, 409 (две кампании, попытка переименовать), back-link, brand_manager redirect, 404. Cleanup через `cleanupEntity` `type: "campaign"` (паттерн `seedCampaign`).

## Tasks & Acceptance

**Execution:**
- [x] `git fetch && git checkout main && git pull && git checkout -b alikhan/campaign-detail-edit-frontend`. Сверить, что 8a и chunk 9 в main.
- [x] `api/campaigns.ts` (+`.test.ts`) — обёртки + re-export.
- [x] `i18n/locales/ru/campaigns.json` — блоки `detail.*` + `edit.*`.
- [x] `i18n/locales/ru/common.json` — `errors.CAMPAIGN_NOT_FOUND`.
- [x] `CampaignDetailPage.tsx` — view + edit режимы, mutation, invalidation, double-submit guard, dedicated 404-state.
- [x] `CampaignDetailPage.test.tsx` — RTL по всему I/O Matrix; happy save с проверкой `expect(updateCampaign).toHaveBeenCalledWith(id, { name: trimmed, tmaUrl: trimmed })`, invalidate обоих keys, переключения в view; реальный `I18nextProvider`; per-method coverage ≥80%.
- [x] `App.tsx` — импорт + замена stub.
- [x] **Self-check через Playwright MCP — БЕЗ HALT.** `make start-web` → вход админом → создать кампанию через UI → детали → view → edit → изменить `name` → save → view с новым `name` → reload → persistence. Empty-edit → inline errors. 409: вторая кампания + попытка переименовать первую → form-level error. Back-link, brand_manager redirect, 404 на несуществующем UUID. Расхождения со спекой агент чинит сам и перепроверяет до сходимости. Только затем — следующий таск.
- [x] `frontend/e2e/web/admin-campaign-detail.spec.ts` — Playwright spec.
- [x] Финальный gate: `make build-web && make lint-web && make test-unit-web && make test-e2e-frontend` — всё зелёное.

**Acceptance Criteria:**
- Given admin, when открывает `/campaigns/{id}`, then рендерится `CampaignDetailPage` с h1 = `name`, секцией «О кампании» и кнопкой edit (активной для live, `disabled` для soft-deleted).
- Given непустые `name`+`tmaUrl`, when submit edit, then `PATCH /campaigns/{id}` уходит с `{name: trimmed, tmaUrl: trimmed}`; на 204 — invalidate `detail(id)`+`all()`, `isEditing=false`, view с свежими значениями после refetch.
- Given PATCH → 409, when обработана, then `<p role="alert">` с текстом `common:errors.CAMPAIGN_NAME_TAKEN`; пользователь в edit-режиме; поля сохранены.
- Given GET → 404, when страница загрузилась, then dedicated state «Кампания не найдена» + back-link; форма не рендерится.
- Given `git grep -n 'campaign-detail-stub' frontend/web/src/App.tsx`, when выполнен, then пусто.
- Given self-check Playwright MCP, when пройден, then I/O Matrix зелёный локально без вмешательства человека; e2e spec написан в той же сессии.
- Given `make build-web && lint-web && test-unit-web && test-e2e-frontend`, when запускаются, then все зелёные; coverage ≥80% на новых файлах.

## Spec Change Log

<!-- Append-only. Заполняется bmad-quick-dev step-04 при review-loops. Пусто до первого bad_spec loopback. -->

## Design Notes

**Inline-toggle вместо `/edit`-route.** Один URL, один cache-entry, никакой навигации. При двух полях overhead отдельного route'а не оправдан.

**Form-level error без field-binding'а.** Преемственно с 8a. 409 `CAMPAIGN_NAME_TAKEN` показывается form-level — текст уже actionable.

**Disabled edit при soft-deleted.** UX-ограничение поверх бэка (бэк PATCH разрешает). Снятие — отдельным feature-PR'ом, когда появится product-сценарий.

**Расширяемость для chunk 11.** Секционный layout — позже добавится «Креаторы» рядом, без переписывания. Архитектуру под несуществующее не закладываем.

**Self-check без HALT.** Стандарт `campaign-roadmap.md` § Тестирование. Дублируется в Boundaries и в execution, чтобы не повторилась прошлая ошибка с HALT-просьбами «потыкай ручками».

## Verification

**Commands:**
- `make build-web` — компиляция чистая.
- `make lint-web` — 0 ошибок.
- `make test-unit-web` — green; coverage ≥80% на новых файлах.
- `make test-e2e-frontend` — green; `admin-campaign-detail.spec.ts` среди прогнанных.

**Self-check (агент через Playwright MCP, без HALT):** view → edit → save → view с новым `name` после reload; empty → inline; 409 → form-level; back-link; brand_manager redirect; 404 dedicated state; `git grep` на stub пусто.

## Suggested Review Order

**Routing & data loading**

- Entry point. Оркестратор: useQuery с retry:false, ветки loading/404/error/happy.
  `frontend/web/src/features/campaigns/CampaignDetailPage.tsx:15`

- Dedicated 404 — отдельный testid внутри стандартного контейнера для консистентности селекторов.
  `frontend/web/src/features/campaigns/CampaignDetailPage.tsx:56`

- Замена stub'а на реальную страницу под существующий RoleGuard(ADMIN).
  `frontend/web/src/App.tsx:88`

**View rendering — defence-in-depth**

- safeHref scheme-whitelist (`http`/`https`/`tg`) — защита от javascript:/data: даже с admin-controlled tmaUrl.
  `frontend/web/src/features/campaigns/CampaignDetailPage.tsx:212`

- Невалидный ISO в `createdAt`/`updatedAt` → fallback `«—»`, не строка `"Invalid Date"`.
  `frontend/web/src/features/campaigns/CampaignDetailPage.tsx:221`

**Edit flow — mutation lifecycle**

- Вынос edit-формы в отдельный файл (150-line guideline).
  `frontend/web/src/features/campaigns/CampaignEditSection.tsx:22`

- Double-submit guard: external `isSubmitting` + `isPending`, сброс в `onSettled` (`frontend-state.md`).
  `frontend/web/src/features/campaigns/CampaignEditSection.tsx:35`

- Trim + non-empty + длинна (255 / 2048) до mutate — paste-input не уходит в backend 422.
  `frontend/web/src/features/campaigns/CampaignEditSection.tsx:71`

- Invalidate detail(id) + all() на success → refetch выводит свежие значения в view.
  `frontend/web/src/features/campaigns/CampaignEditSection.tsx:39`

**API layer**

- openapi-fetch обёртки с ApiError-маппингом ошибок: новые `getCampaign` (404 → ApiError) и `updateCampaign` (PATCH 204).
  `frontend/web/src/api/campaigns.ts:39`

**i18n**

- Блоки `detail.*` + `edit.*` (form-ключи переиспользуют `create.*`, бейдж reuse'ит `labels.deletedBadge` из chunk 9).
  `frontend/web/src/shared/i18n/locales/ru/campaigns.json:42`

- `errors.CAMPAIGN_NOT_FOUND` для form-level 404 race и dedicated 404-state.
  `frontend/web/src/shared/i18n/locales/ru/common.json:38`

**Tests**

- E2E нарратив-header + 7 user-flow тестов по I/O Matrix (happy view, edit+save+reload, validation, 409, back-link, 404, RoleGuard).
  `frontend/e2e/web/admin-campaign-detail.spec.ts:1`

- Unit-покрытие RTL: loading / 404 / error retry / view (live + soft-deleted) / safeHref / invalid-date / enter+cancel / validation / happy save / 409 / 404 race / 422 / unknown / submit guard.
  `frontend/web/src/features/campaigns/CampaignDetailPage.test.tsx:1`

- Расширение API-тестов: getCampaign (200/404/malformed) + updateCampaign (204/409/422/malformed).
  `frontend/web/src/api/campaigns.test.ts:201`

- Адаптация старых e2e под удалённый stub: `campaign-detail-stub` → `campaign-detail-page`.
  `frontend/e2e/web/admin-campaign-create.spec.ts:93`
  `frontend/e2e/web/admin-campaigns-list.spec.ts:230`
