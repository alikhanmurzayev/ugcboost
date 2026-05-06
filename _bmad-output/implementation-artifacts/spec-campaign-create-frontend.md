---
title: "Фронт-страница создания кампании (chunk 8a campaign-roadmap)"
type: "feature"
created: "2026-05-06"
status: "done"
baseline_commit: "2a12bdebea07bf6867671df10b89c4dcc378009e"
context:
  - "docs/standards/"
  - "_bmad-output/planning-artifacts/campaign-roadmap.md"
---

<frozen-after-approval reason="human-owned intent — do not modify unless human renegotiates">

## Intent

**Problem:** `/campaigns/new` смонтирован в `App.tsx` под `RoleGuard(ADMIN)` как inline-stub `<ComingSoonPage testid="campaign-new-stub" />` (заложен chunk 9'ом). CTA «Создать кампанию» на `/campaigns` ведёт в этот тупик — admin не может создать кампанию через UI, только через прямой `POST /campaigns`.

**Approach:** Полноценная страница `features/campaigns/CampaignCreatePage.tsx`: контролируемая форма (`name` + `tmaUrl`), `useMutation → POST /campaigns` через новую обёртку `createCampaign` в `api/campaigns.ts` (паттерн `listCampaigns`). Submit делает `trim + non-empty` per-field валидацию (inline error под пустым полем); длину доверяем `maxLength=255 / 2048` HTML-атрибутам (синхронно с бэк-валидаторами). На 201 — `invalidate(campaignKeys.all())` + `navigate("/campaigns/" + data.data.id)` (детальная — stub до chunk 8b). На 4xx/5xx — form-level error через `getErrorMessage(err.code)` (паттерн `LoginPage`); серверные коды добавляются ключами в `common.json`. App.tsx заменяет stub на новый компонент. Тем же PR'ом — unit + Playwright e2e.

## Boundaries & Constraints

**Always:**
- Все `docs/standards/` загружены целиком и применяются как hard rules.
- AuthZ — `RoleGuard(ADMIN)` через существующий wrapper в `App.tsx`.
- API — `client.POST("/campaigns", { body })` через `openapi-fetch`; raw `fetch` запрещён.
- Типы — re-export `CampaignInput` / `CampaignCreatedResult` из `components["schemas"]`; ручные дубли запрещены.
- i18n — namespace `campaigns` расширяется блоком `create.*`; серверные коды → `common:errors.{code}` через `getErrorMessage`.
- Submit `disabled` при `isPending`, текст «Создаётся…».
- Поля и интерактивные элементы получают `data-testid`.
- Реализация стартует **только после мержа chunk 9 в main**; ветка от свежего main.

**Ask First:**
- Изменение состава полей формы (что-то кроме `name` + `tmaUrl`).
- Замена redirect-target'а на success.
- Добавление client-side формат-валидации `tmaUrl` (бэк её не делает — admin-controlled).
- Внедрение toast или библиотек форм (RHF, Formik) — строим на `useState + useMutation`.
- Field-binding серверных ошибок (routing `CAMPAIGN_NAME_*` под `name`) — снято в spec-mode.

**Never:**
- Backend-изменения (контракт `POST /campaigns` зафиксирован chunk 3, в main).
- Реализация detail/edit `/campaigns/:id` — это chunk 8b, отдельный PR.
- Drawer / inline-форма внутри `CampaignsListPage` — 8a отдельная страница.
- Импорт UI из `_prototype/` или `features/brands/` (legacy UI, см. revisions roadmap'а 2026-05-06).
- Удаление stub'а `CAMPAIGN_DETAIL_PATTERN` (его заменит chunk 8b).

## I/O & Edge-Case Matrix

| Сценарий | Состояние | Ожидаемое |
|---|---|---|
| Не админ | brand_manager на `/campaigns/new` | `RoleGuard` редиректит, страница не рендерится |
| Empty `name` (после trim) | submit с пустым/whitespace `name` | inline error под `name` (`campaigns:create.nameRequired`); мутация не вызывается |
| Empty `tmaUrl` | submit с пустым `tmaUrl` | inline error под `tmaUrl`; мутация не вызывается |
| Оба пустые | submit с обоими пустыми | две inline errors; мутация не вызывается |
| Submit во время `isPending` | повторный клик | кнопка `disabled`, no-op |
| Happy submit | непустые поля, `name` уникален | 201 → `invalidate(campaignKeys.all())` + `navigate("/campaigns/" + data.data.id)` |
| 409 `CAMPAIGN_NAME_TAKEN` | другой админ занял `name` | form-level error из `common:errors.CAMPAIGN_NAME_TAKEN`; форма заполнена |
| 422 `*_TOO_LONG` | теоретический fallback (HTML `maxLength` блокирует ввод) | form-level error через `getErrorMessage` |
| Network / 5xx / unknown code | сервер недоступен или нет ключа в `errors.*` | fallback `common:errors.unknown` |
| Click back-link | пользователь на «← К списку» | `navigate("/campaigns")` |
| Заход через CTA chunk 9 | `/campaigns` → клик `campaigns-create-button` | `/campaigns/new` рендерит `CampaignCreatePage`, не stub |

</frozen-after-approval>

## Code Map

- `frontend/web/src/api/campaigns.ts` (+`.test.ts`) -- `createCampaign(input: CampaignInput): Promise<CampaignCreatedResult>` через `client.POST("/campaigns", { body: input })` + `extractErrorCode`. Re-export `CampaignInput` / `CampaignCreatedResult`.
- `frontend/web/src/features/campaigns/CampaignCreatePage.tsx` (+`.test.tsx`) -- новая страница. `useState` для полей и errors, `useMutation`, `useNavigate`, `useQueryClient`. Header h1 + description + back-link `<Link to={ROUTES.CAMPAIGNS}>`. Form: 2 input'а (label + input + per-field error), submit-кнопка, form-level `<p role="alert">` под формой.
- `frontend/web/src/App.tsx` -- импорт `CampaignCreatePage`; заменить `<ComingSoonPage testid="campaign-new-stub" />` под `ROUTES.CAMPAIGN_NEW` на `<CampaignCreatePage />`. `CAMPAIGN_DETAIL_PATTERN`-stub оставить.
- `frontend/web/src/shared/i18n/locales/ru/campaigns.json` -- расширить `create`: `title`, `description`, `backToList`, `nameLabel`, `namePlaceholder`, `tmaUrlLabel`, `tmaUrlPlaceholder`, `nameRequired`, `tmaUrlRequired`, `submitButton`, `submittingButton`.
- `frontend/web/src/shared/i18n/locales/ru/common.json` -- 5 ключей `errors.CAMPAIGN_*`. Источник текстов — `backend/internal/domain/campaign.go:46-89`.
- `frontend/e2e/web/admin-campaign-create.spec.ts` -- Playwright spec; русский header-нарратив (`frontend-testing-e2e.md`); сценарии: happy create через UI с uuid-маркером, validation-empty, 409 NAME_TAKEN (`seedCampaign` из `frontend/e2e/helpers/api.ts:649-680` + попытка дубликата через UI), back-link, brand_manager redirect.

## Tasks & Acceptance

**Execution (порядок):**

- [x] Pre-flight -- `git fetch && git checkout main && git pull && git checkout -b alikhan/campaign-create-frontend`. **HALT и сообщить**, если chunk 9 (`CampaignsListPage`, `seedCampaign`, namespace `campaigns`) ещё не в main.
- [x] `frontend/web/src/api/campaigns.ts` (+`.test.ts`) -- `createCampaign` + re-export типов; тесты по паттерну `listCampaigns` (happy, 409, 422, malformed body).
- [x] `frontend/web/src/shared/i18n/locales/ru/campaigns.json` -- блок `create.*`.
- [x] `frontend/web/src/shared/i18n/locales/ru/common.json` -- 5 `errors.CAMPAIGN_*` ключей.
- [x] `frontend/web/src/features/campaigns/CampaignCreatePage.tsx` -- компонент.
- [x] `frontend/web/src/features/campaigns/CampaignCreatePage.test.tsx` -- unit + RTL по всему I/O Matrix; happy submit с `expect(createCampaign).toHaveBeenCalledWith({ name, tmaUrl })`, проверка `invalidate(campaignKeys.all())` и navigate-mock на `/campaigns/{id}`; реальный `I18nextProvider` (`frontend-testing-unit.md`); per-method coverage ≥80%.
- [x] `frontend/web/src/App.tsx` -- импорт + замена `CAMPAIGN_NEW`-stub.
- [x] `frontend/e2e/web/admin-campaign-create.spec.ts` -- Playwright spec; cleanup кампаний через `cleanupEntity` `type: "campaign"`.
- [x] **Manual local sanity check (HALT)** -- `make start-web` + admin → `/campaigns` → CTA → форма → заполнить → submit → ожидать `/campaigns/{uuid}` (stub). Также: empty submit → inline errors; brand_manager на `/campaigns/new` → редирект; back-link → `/campaigns`. **HALT и сообщить результат.**

**Acceptance Criteria:**

- Given admin, when открывает `/campaigns/new`, then рендерится `CampaignCreatePage` с h1 «Новая кампания», back-link, `campaign-name-input`, `campaign-tma-url-input`, `create-campaign-submit`.
- Given непустые `name`+`tmaUrl`, when submit, then `POST /campaigns` уходит с `{ name: trimmed, tmaUrl: trimmed }`; на 201 — `invalidateQueries({ queryKey: campaignKeys.all() })` + `navigate("/campaigns/" + response.data.id)`.
- Given POST → 409 `CAMPAIGN_NAME_TAKEN`, when обработана, then `<p role="alert" data-testid="create-campaign-error">` с текстом из `common:errors.CAMPAIGN_NAME_TAKEN`; значения полей сохранены.
- Given `make build-web && make lint-web && make test-unit-web && make test-e2e-frontend`, when запускаются, then все зелёные; per-method coverage ≥80% на новых файлах; существующие spec'и не регрессируют.
- Given `git grep -n 'campaign-new-stub' frontend/web/src/App.tsx`, when выполнен, then пусто.

## Spec Change Log

<!-- Append-only. Populated by step-04 during review loops. Empty until the first bad_spec loopback. -->

- **2026-05-06 (step-04 patch, no loopback).** AC line «h1 «Создать кампанию»» → «h1 «Новая кампания»». Триггер: acceptance-auditor (minor) — реализация рендерит «Новая кампания» (i18n key `create.title`), что не дублирует CTA «Создать кампанию» на `/campaigns` и совпадает с шаблоном noun-form у других списков. Спека читалась как назначение копирайта, а не литерал; исправлено wording'ом, код не трогали.

## Design Notes

**`useState` + `useMutation` без библиотек форм.** Минимально достаточно для двух полей; RHF/Formik — scope creep, вернёмся когда форм с >5 полями станет много.

**Form-level error без field-binding'а.** `LoginPage`-паттерн. 409 `CAMPAIGN_NAME_TAKEN` показывается form-level — текст уже actionable («Кампания с таким названием уже есть. Выберите другое название…»).

**Redirect на `/campaigns/:id` (stub до 8b).** Когда 8b мерджится — детальная заработает без правок 8a. Альтернатива «list + toast» снята: toast-инфра отсутствует.

**`maxLength` HTML вместо client count.** Hard cap (255 / 2048) совпадает с `domain.ValidateCampaignName` / `ValidateCampaignTmaURL`. 422 `*_TOO_LONG` — fallback через `getErrorMessage`.

## Verification

**Commands:**
- `make build-web` -- expected: компиляция чистая.
- `make lint-web` -- expected: 0 ошибок.
- `make test-unit-web` -- expected: green; coverage ≥80% на новых файлах.
- `make test-e2e-frontend` -- expected: green; `admin-campaign-create.spec.ts` среди прогнанных.

**Manual checks:**
- `git grep -n 'ComingSoonPage testid="campaign-new-stub"' frontend/web/src/App.tsx` -- пусто.
- `make start-web` + admin → `/campaigns` → CTA → ручной happy / empty / brand_manager redirect / back-link.

## Suggested Review Order

**Дизайн-точка входа**

- Контракт компонента: form-state, mutation, redirect, ARIA.
  [`CampaignCreatePage.tsx:14`](../../frontend/web/src/features/campaigns/CampaignCreatePage.tsx#L14)

**API-обёртка**

- `createCampaign` повторяет паттерн `listCampaigns`, бросает `ApiError`.
  [`campaigns.ts:30`](../../frontend/web/src/api/campaigns.ts#L30)

**Подключение в роутер**

- Замена inline-stub'а chunk 9 на реальный компонент под `RoleGuard(ADMIN)`.
  [`App.tsx:82`](../../frontend/web/src/App.tsx#L82)

**i18n**

- Локализация формы: title, описание, поля, ошибки, submit.
  [`campaigns.json:25`](../../frontend/web/src/shared/i18n/locales/ru/campaigns.json#L25)

- Серверные коды CAMPAIGN_* (тексты совпадают с `domain/campaign.go`).
  [`common.json:36`](../../frontend/web/src/shared/i18n/locales/ru/common.json#L36)

**E2E coverage**

- Сценарии 8a: happy / empty / 409 / back-link / brand_manager redirect.
  [`admin-campaign-create.spec.ts:1`](../../frontend/e2e/web/admin-campaign-create.spec.ts#L1)

- Retarget chunk 9 CTA-теста с stub'а на реальную страницу 8a.
  [`admin-campaigns-list.spec.ts:185`](../../frontend/e2e/web/admin-campaigns-list.spec.ts#L185)

- Экспорт `cleanupCampaign` для UI-созданных кампаний в e2e.
  [`api.ts:627`](../../frontend/e2e/helpers/api.ts#L627)

**Unit coverage**

- I/O Matrix: render, validation, happy submit, server errors, submit guard.
  [`CampaignCreatePage.test.tsx:1`](../../frontend/web/src/features/campaigns/CampaignCreatePage.test.tsx#L1)

- Тесты `createCampaign` (happy, 409, 422, malformed).
  [`campaigns.test.ts:138`](../../frontend/web/src/api/campaigns.test.ts#L138)
