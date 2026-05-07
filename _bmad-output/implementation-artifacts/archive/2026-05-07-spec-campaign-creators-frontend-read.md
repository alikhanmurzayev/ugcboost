---
title: 'campaign_creators frontend — read-only секция (chunk 11, slice 1/2)'
type: feature
created: '2026-05-07'
status: done
baseline_commit: 3eb8df02dfee58aa1786db66c0a38255fbf315a9
context:
  - docs/standards/
  - _bmad-output/planning-artifacts/design-campaign-creator-flow.md
  - _bmad-output/implementation-artifacts/intent-campaign-creators-frontend.md
  - _bmad-output/implementation-artifacts/spec-campaign-creators-frontend-reference.md
---

<frozen-after-approval reason="human-owned intent — do not modify unless human renegotiates">

## Intent

**Problem:** На `/campaigns/:id` админ не видит, какие креаторы добавлены в кампанию. Backend chunk 10 уже хранит `campaign_creators`, но в UI ничего нет — visibility закрыта. Без визуальной обратной связи управлять составом через будущий drawer (Spec C) бессмысленно.

**Approach:** Vertical slice 1/2 chunk 11. Добавляем на `/campaigns/:id` секцию «Креаторы кампании» с read-only-таблицей: A3 (`GET /campaigns/{id}/creators`) → `creator_ids[]` → `POST /creators/list { ids }` (с фильтром из Spec A `creators-list-ids-filter`) → join row `{ campaignCreator, creator }`. Колонки таблицы — те же, что в `/creators`. Кнопка «Добавить креаторов» — disabled с tooltip «Появится в следующем PR» (mutations = slice 2/2). Клик по строке открывает существующий `CreatorDrawer` (detail) через URL `?creatorId=<uuid>`. Без drawer'а add, без кнопки remove, без ConfirmDialog — это всё в slice 2/2 (`spec-campaign-creators-frontend-mutations.md`).

## Boundaries & Constraints

**Always:**
- Полная загрузка `docs/standards/` перед кодом. Особенно `frontend-api.md`, `frontend-components.md`, `frontend-quality.md`, `frontend-state.md`, `frontend-types.md`, `frontend-testing-unit.md`, `frontend-testing-e2e.md`, `naming.md`, `security.md`, `review-checklist.md`.
- API-типы — только из `frontend/web/src/api/generated/schema.ts`. Ручные `interface`/`type` для request/response запрещены.
- Loading/Error/Empty состояния обработаны для обоих queries (A3 + listCreators({ids})).
- `data-testid` на каждом интерактивном элементе и ключевом контейнере; `aria-label` на иконках без текста.
- Все строки UI — через `react-i18next`; namespace `campaigns`, ключи `campaignCreators.*`. Hardcoded JSX-строки запрещены.
- На `campaign.isDeleted === true` секция полностью скрыта (return null); A3 не вызывается.
- Click по строке → URL `?creatorId=<uuid>` → открывает существующий `CreatorDrawer` (паттерн как в `CreatorsListPage`).
- Direct deep-link `/campaigns/:id?creatorId=<uuid>` открывает страницу с уже открытым CreatorDrawer.
- Coverage frontend ≥80%. `make build-web && make lint-web && make test-unit-web && make test-e2e-frontend` зелёные.
- Контейнер `CampaignDetailPage` расширяется `max-w-2xl` → `max-w-7xl`; existing details-секция (текущая узкая) ограничивается локально через свой `max-w-2xl` wrapper.
- Vertical-slicing: после мержа этого PR на staging admin **видит** существующих креаторов в кампании (если они есть в БД из ручных INSERT/curl — что естественно после chunk 10 в main). Add-кнопка disabled с tooltip — precedent есть (chunk 9 disable soft-delete до chunk 7).
- Spec A (`creators-list-ids-filter`) **должна быть в main** до старта имплементации этой spec'и (фронт зависит от расширения /creators/list).

**Ask First:**
- Изменение column-set таблицы (отличающееся от текущего `CreatorsListPage`).
- Любые отклонения от UX решений зафиксированных в `intent-campaign-creators-frontend.md` / `spec-campaign-creators-frontend-reference.md`.
- Включение колонки «Действия» (она появляется в slice 2/2, не сейчас).
- Любые правки в backend (этот PR — только фронт).

**Never:**
- Любые mutations (Add drawer, Remove, ConfirmDialog) — это slice 2/2 (`spec-campaign-creators-frontend-mutations.md`).
- ConfirmDialog как shared компонент (rule of three; в slice 2/2 — local inline).
- Toast библиотека (нет в проекте).
- Колонки status/счётчики (chunk 15).
- Кнопки рассылок/ремайндеров (chunk 13).
- TMA-flow (chunk 14).
- Прототип Айданы (`frontend/web/src/_prototype/`) как функциональный референс — только визуальный язык (карточки/чипсы/таблицы).
- Хардкод query keys / route paths / ролей.
- `any` / `!` / `as` / `console.log` / `window.confirm` / `fireEvent` (в тестах).
- Прямой raw `fetch()` вне `api/client.ts`.

## I/O & Edge-Case Matrix

| Scenario | State / Action | Expected UI | Error Handling |
|---|---|---|---|
| Live campaign + N добавленных | A3 → 200 + N items, listCreators({ids}) → 200 + N profile | Таблица с колонками №/ФИО/Соцсети/Категории/Возраст/Город/Создан; счётчик «N в кампании» в заголовке; Add disabled с tooltip | — |
| Live campaign + 0 добавленных | A3 → 200 [] | Empty state «Креаторов пока нет» + Add disabled с tooltip | — |
| Soft-deleted campaign | `campaign.isDeleted === true` | Секция не рендерится; A3 не вызывается; existing badge «Удалено» + disabled-Edit работают | — |
| A3 5xx/network | error response | `<ErrorState onRetry={refetch}>` внутри секции | refetch на retry |
| listCreators({ids}) 5xx | A3 ok, listCreators error | `<ErrorState onRetry>` (refetch перевычисляет hook полностью) | retry |
| Soft-deleted creator после add (rare race) | A3 returns ID, listCreators({ids}) не возвращает (creator deactivated) | Строка с placeholder `—` в ФИО/Соцсетях, tooltip «Креатор удалён из системы» | — |
| Click row | клик по строке (не на disabled-Add) | URL обновляется → `?creatorId=<uuid>` → открывается `CreatorDrawer` | — |
| URL `?creatorId=<uuid>` deep-link | прямой переход с creatorId | `CreatorDrawer` открывается на этой странице с подтянутым profile через `getCreator` (existing) | — |
| Auth/role | non-admin | RoleGuard на `/campaigns/:id` filter'ит на 403 (existing) | — |
| Initial load slow | A3 fetching | Spinner внутри секции | — |
| `campaign.isDeleted === false` после мержа | повторное открытие страницы | Секция рендерится; обычный flow | — |

</frozen-after-approval>

## Code Map

**New files:**
- `frontend/web/src/api/campaignCreators.ts` -- обёртка `listCampaignCreators(campaignId): Promise<CampaignCreator[]>` через generated openapi-fetch client; на non-2xx — throw `ApiError` (паттерн `api/client.ts`). **Только** этот wrapper в Spec B; `addCampaignCreators`/`removeCampaignCreator` появятся в Spec C.
- `frontend/web/src/features/campaigns/creators/CampaignCreatorsSection.tsx` -- секция: header (`t('campaigns:campaignCreators.title')` + counter), Add-кнопка `disabled` с `title={t(...)}`, таблица или empty state. Возвращает `null` при `campaign.isDeleted`. Loading → `<Spinner>`, Error → `<ErrorState onRetry>`.
- `frontend/web/src/features/campaigns/creators/CampaignCreatorsTable.tsx` -- shared `<Table>` (`@/shared/components/Table`) с колонками /creators (взято из реального `CreatorsListPage.buildColumns`): №, ФИО, Соцсети (`SocialLink` per row), Категории (`CategoryChips`), Возраст (`calcAge(birthDate)`), Город, Дата создания. **Без колонки «Действия»** (она появится в slice 2/2). `onRowClick` callback → URL `?creatorId=<uuid>`. `selectedKey` prop для highlight выбранной строки. Empty message пропускается как prop из Section.
- `frontend/web/src/features/campaigns/creators/hooks/useCampaignCreators.ts` -- composes 2 useQuery: A3 (`campaignCreatorKeys.list(campaignId)`) → `creator_ids[]` → `listCreators({ ids, page:1, perPage:200, sort:"created_at", order:"desc" })` (perPage=200 покрывает chunk-creator-cap из chunk 10). Возвращает `{ rows: { campaignCreator, creator? }[], isLoading, isError, refetch }`. `creator?` — undefined если соответствующий ID не вернулся из listCreators (deactivated edge case).
- `frontend/web/src/features/campaigns/creators/CampaignCreatorsSection.test.tsx` -- vitest+RTL. Сценарии: loading (Spinner), error (ErrorState onRetry), empty (text + disabled Add), happy (N rows + counter), soft-deleted (return null).
- `frontend/web/src/features/campaigns/creators/CampaignCreatorsTable.test.tsx` -- rendering rows + click → callback called with row.id; selected highlight.
- `frontend/web/src/features/campaigns/creators/hooks/useCampaignCreators.test.tsx` -- compose A3+listCreators (mock obvious — generated client), happy/loading/error, soft-deleted creator handling (id в A3, нет в listCreators → row.creator undefined).
- `frontend/e2e/web/admin-campaign-creators-read.spec.ts` -- Playwright, **Russian narrative JSDoc header** (по `frontend-testing-e2e.md`). Smoke flow: setup admin (login через UI или /test/seed-user) → создать кампанию через UI → seed 2 creators (через approve flow или /test/seed-creator если такой helper есть; иначе backend `curl POST /creator-applications/...` → approve через UI/API) → backend `curl POST /campaigns/{id}/creators -d '{"creatorIds":[uuid1,uuid2]}'` с admin token (чтобы добавить в campaign_creators до того как UI это поддерживает — это slice 2/2) → открыть `/campaigns/:id` → видеть таблицу с 2 строками с подтянутыми ФИО → клик строки → CreatorDrawer открыт. Cleanup defer-stack.

**Modified files:**
- `frontend/web/src/api/creators.ts` -- `listCreators` пробрасывает `ids?: string[]` в request body. Только если `ids && ids.length > 0`. Spec A (`creators-list-ids-filter`) предполагается уже в main — generated `schema.ts` уже содержит поле.
- `frontend/web/src/shared/constants/queryKeys.ts` -- `campaignCreatorKeys = { all: () => ["campaignCreators"] as const, list: (campaignId: string) => ["campaignCreators", "list", campaignId] as const }`.
- `frontend/web/src/features/campaigns/CampaignDetailPage.tsx` -- расширить контейнер `max-w-2xl` → `max-w-7xl`; existing details-section обернуть локально в `<div className="max-w-2xl">` (чтобы не разрасталась). Под details — `<CampaignCreatorsSection campaign={campaign} />` (компонент сам решает return null). Прочитать `searchParams.get("creatorId")` → открыть `CreatorDrawer` с этим id (паттерн как в `CreatorsListPage`).
- `frontend/web/src/features/campaigns/CampaignDetailPage.test.tsx` -- расширить existing tests: `CampaignCreatorsSection` присутствует на live, отсутствует на soft-deleted; query-param `?creatorId=...` открывает `CreatorDrawer`.
- `frontend/web/src/locales/ru/campaigns.json` -- блок `campaignCreators.*`: `title` (= "Креаторы кампании"), `count` (= "{{count}} в кампании", без плюрализации — все формы давали идентичный текст), `empty` (= "Креаторов пока нет"), `addButton` (= "Добавить креаторов"), `addDisabledTooltip` (= "Появится в следующем PR"), `loadError` (= "Не удалось загрузить креаторов"), `creatorDeleted` (tooltip = "Креатор удалён из системы"). Колонки таблицы — переиспользуем существующие `creators.columns.*` через `t('creators:columns.fullName')` и т.д. Добавлен `creators:columns.index` (= "№") поскольку header `№` теперь идёт через i18n.

## Tasks & Acceptance

**Execution (предполагается, что Spec A `creators-list-ids-filter` уже в main):**
- [x] `frontend/web/src/api/creators.ts` -- `listCreators` уже принимает `CreatorsListInput` где `ids?: string[]` присутствует от Spec A; передачу `ids` управляет вызывающий код (хук). Без code-change в `creators.ts`.
- [x] `frontend/web/src/api/campaignCreators.ts` -- новый файл с `listCampaignCreators(campaignId): Promise<CampaignCreator[]>`. Только этот wrapper; add/remove появятся в Spec C.
- [x] `frontend/web/src/shared/constants/queryKeys.ts` -- `campaignCreatorKeys` factory.
- [x] `frontend/web/src/features/campaigns/creators/hooks/useCampaignCreators.ts` -- hook composes 2 useQuery (A3 + listCreators({ids}) с perPage=200). Возвращает `{ rows, total, isLoading, isError, refetch }`. Soft-delete creator → `row.creator: undefined`.
- [x] `frontend/web/src/features/campaigns/creators/CampaignCreatorsTable.tsx` -- shared `<Table>` с колонками /creators (без «Действий»). `onRowClick` callback. `selectedKey` prop для highlight. Empty message — prop.
- [x] `frontend/web/src/features/campaigns/creators/CampaignCreatorsSection.tsx` -- composition: header + counter + disabled-Add + либо `<Table>` либо empty state. Если `campaign.isDeleted` — return null. Loading → Spinner. Error → ErrorState onRetry.
- [x] `frontend/web/src/features/campaigns/CampaignDetailPage.tsx` -- расширить контейнер до `max-w-7xl`; details-секцию обернуть в `max-w-2xl`; добавить `<CampaignCreatorsSection campaign={campaign} />`; читать searchParams `creatorId` для CreatorDrawer (паттерн копируется из `CreatorsListPage`).
- [x] `frontend/web/src/shared/i18n/locales/ru/campaigns.json` -- блок `campaignCreators.*` (см. Code Map).
- [x] Unit-тесты: `CampaignCreatorsSection.test.tsx`, `CampaignCreatorsTable.test.tsx`, `hooks/useCampaignCreators.test.tsx`, `api/campaignCreators.test.ts`, расширение `CampaignDetailPage.test.tsx`. 377/377 passing.
- [x] E2E `frontend/e2e/web/admin-campaign-creators-read.spec.ts` -- Russian narrative header (по `frontend-testing-e2e.md`). 3 кейса: 2 креатора (counter+rows+Add disabled), 0 креаторов (empty+disabled Add), click row → CreatorDrawer + close. `addCampaignCreators` + `removeCampaignCreator` helpers в `frontend/e2e/helpers/api.ts`.

**Acceptance Criteria:**
- Given чистая БД + admin auth + живая кампания без креаторов, when admin открывает `/campaigns/:id`, then секция «Креаторы кампании» видна с empty state «Креаторов пока нет» + Add disabled с tooltip «Появится в следующем PR».
- Given кампания с 2 добавленными `campaign_creators` (через backend curl A1), when admin открывает `/campaigns/:id`, then таблица показывает 2 строки с подтянутыми ФИО / соцсетями / категориями / возрастом / городом / датой создания (через `listCreators({ids})`); счётчик «2 в кампании» в заголовке; Add disabled.
- Given `campaign.isDeleted === true`, when admin открывает `/campaigns/:id`, then секция не рендерится (вообще не в DOM); existing badge «Удалено» + disabled-Edit работают.
- Given клик по строке креатора, when admin кликает, then URL обновляется до `?creatorId=<uuid>`, открывается existing `CreatorDrawer` с подтянутым profile через `getCreator`.
- Given direct deep-link на `/campaigns/:id?creatorId=<uuid>`, then страница открывается с уже открытым `CreatorDrawer`.
- Given A3 5xx, then `<ErrorState>` с retry.
- Given listCreators({ids}) не вернул один из ID (soft-delete creator после add), then соответствующая строка показывает placeholder `—` + tooltip «Креатор удалён из системы».
- Given `make build-web && make lint-web && make test-unit-web`, then всё зелёное; coverage ≥80%.
- Given backend поднят локально + admin token + Spec A в main, when запускается `make test-e2e-frontend`, then `admin-campaign-creators-read.spec.ts` зелёный.

## Verification

**Commands:**
- `make build-web && make lint-web && make test-unit-web` -- frontend tsc/eslint/vitest зелёные.
- `make test-e2e-frontend` -- read-only smoke зелёный.

**Self-check агента (без HALT, между unit и e2e):**
1. `make migrate-up && make start-backend && make run-web`.
2. Войти как admin → создать кампанию → `/campaigns/:id`.
3. Empty state виден; Add-кнопка disabled с tooltip; click — без эффекта.
4. Через `curl POST /campaigns/{id}/creators -d '{"creatorIds":[<uuid1>,<uuid2>]}'` (admin token, requires Spec A в main и chunk 10 в main) — добавить 2 креаторов.
5. Refresh страницы → 2 строки с ФИО / соцсетями / категориями / возрастом / городом.
6. Click по первой строке → URL `?creatorId=<uuid>` → CreatorDrawer открыт с profile.
7. Soft-deleted кампания (через DELETE /campaigns/{id} когда chunk 7 в main или ручной UPDATE в БД) → секция не видна.
8. Расхождение со спекой = баг → агент сам фиксит, перезапускает self-check, переходит к e2e.

## Suggested Review Order

**API surface**

- Тонкий wrapper над generated client: GET A3 → unwrap `items[]` или `ApiError`.
  `frontend/web/src/api/campaignCreators.ts:1`

- E2E helpers — `addCampaignCreators` (A1) и `removeCampaignCreator` (A2, для cleanup), под admin-токеном.
  `frontend/e2e/helpers/api.ts:684`

**Composition: hook + section + table**

- Hook composes A3 → ids → `listCreators({ids, perPage:200, sort:created_at desc})`. Soft-deleted creator → `row.creator=undefined`.
  `frontend/web/src/features/campaigns/creators/hooks/useCampaignCreators.ts:25`

- Section: header + counter + disabled-Add + Spinner/ErrorState/Table. `if (campaign.isDeleted) return null` после хука; click skip для row без creator.
  `frontend/web/src/features/campaigns/creators/CampaignCreatorsSection.tsx:17`

- Table: переиспользует `<Table>`, колонки те же что в `/creators`; placeholder `—` + tooltip для row без creator.
  `frontend/web/src/features/campaigns/creators/CampaignCreatorsTable.tsx:18`

**Page integration: drawer via URL**

- Контейнер `max-w-2xl` → `max-w-7xl`; details локально в `max-w-2xl`; секция креаторов добавлена; `?creatorId` → `CreatorDrawer` + `getCreator` с `retry:false`.
  `frontend/web/src/features/campaigns/CampaignDetailPage.tsx:81`

**Constants & i18n**

- `campaignCreatorKeys.profiles(...)` — отдельный namespace, чтобы invalidate `creatorKeys.all()` от `/creators` не цеплял profile-fetch для секции кампании.
  `frontend/web/src/shared/constants/queryKeys.ts:31`

- `SEARCH_PARAMS.CREATOR_ID` — единственная константа URL-контракта для пары Section/Page.
  `frontend/web/src/shared/constants/routes.ts:18`

- i18n блок `campaignCreators.*` — счётчик через `count` без плюрализации.
  `frontend/web/src/shared/i18n/locales/ru/campaigns.json:60`

- `<Table>` теперь сетит `data-selected` для highlighted row — тестируем поведение, не CSS-класс.
  `frontend/web/src/shared/components/Table.tsx:118`

**Tests**

- Unit hook: empty A3 skip listCreators, compose+merge, missing creator → row.creator undefined, error pathways, refetch.
  `frontend/web/src/features/campaigns/creators/hooks/useCampaignCreators.test.tsx:73`

- Unit section: visibility gate (isDeleted), loading/error/empty/happy, click → data-selected, soft-deleted skip.
  `frontend/web/src/features/campaigns/creators/CampaignCreatorsSection.test.tsx:90`

- Unit table: rowKey, click callback, data-selected highlight, deleted-creator placeholder.
  `frontend/web/src/features/campaigns/creators/CampaignCreatorsTable.test.tsx:42`

- Unit page: section presence/absence по isDeleted, drawer открывается на deep-link и на click, close.
  `frontend/web/src/features/campaigns/CampaignDetailPage.test.tsx:540`

- E2E: 2 креатора (counter+rows+disabled Add), 0 креаторов (empty), click→Drawer→close. Cleanup: detach перед campaign delete (FK).
  `frontend/e2e/web/admin-campaign-creators-read.spec.ts:51`

