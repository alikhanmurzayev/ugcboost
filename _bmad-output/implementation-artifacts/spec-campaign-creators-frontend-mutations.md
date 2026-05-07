---
title: 'campaign_creators frontend — mutations Add/Remove (chunk 11, slice 2/2)'
type: feature
created: '2026-05-07'
status: done
baseline_commit: 13e2fa9d4aafd7aa1bbdecbed5dc785760729d30
context:
  - docs/standards/
  - _bmad-output/planning-artifacts/design-campaign-creator-flow.md
  - _bmad-output/implementation-artifacts/intent-campaign-creators-frontend.md
  - _bmad-output/implementation-artifacts/spec-campaign-creators-frontend-reference.md
---

<frozen-after-approval reason="human-owned intent — do not modify unless human renegotiates">

## Intent

**Problem:** После slice 1/2 (`spec-campaign-creators-frontend-read.md`) на `/campaigns/:id` есть read-only секция «Креаторы кампании», но admin **не может** управлять составом — кнопка Add disabled с tooltip, корзины нет. Без mutations chunk 11 не закрывает свою задачу из roadmap'а: «выбрать креаторов в кампанию, удалить из неё».

**Approach:** Vertical slice 2/2 chunk 11. Активируем Add-кнопку: открывает drawer справа (`@/shared/components/Drawer` с `widthClassName="w-[1100px] max-w-[90vw]"`) с полным `CreatorsListPage`-форматом (фильтры из `CreatorFilters`, пагинация per_page=50, чек-боксы первой колонкой, sort `created_at desc`) + selection state с persistence через filters/pagination + hard-cap 200 в UI + уже-добавленные disabled-row + badge «Добавлен». Submit-batch шлёт A1 `POST /campaigns/{id}/creators`. Remove: добавляем колонку «Действия» в существующую таблицу секции (из Spec B) с иконкой корзины → локальный inline `RemoveCreatorConfirm.tsx` (НЕ в shared/, rule of three: вынести при ≥3 use case'ах) → DELETE A2. Race-resilience: на 422 `CREATOR_ALREADY_IN_CAMPAIGN` — `invalidateQueries(campaignCreatorKeys.list)`, drawer **не закрывается**, alert; на 404 — закрывается, alert, parent invalidate.

## Boundaries & Constraints

**Always:**
- Полная загрузка `docs/standards/` перед кодом. Особенно `frontend-api.md`, `frontend-components.md`, `frontend-quality.md`, `frontend-state.md`, `frontend-types.md`, `frontend-testing-unit.md`, `frontend-testing-e2e.md`, `naming.md`, `security.md`, `review-checklist.md`.
- API-типы — только из `frontend/web/src/api/generated/schema.ts`. Ручные `interface`/`type` для request/response запрещены.
- `useMutation` обязан иметь `onError` handler. Кнопки `disabled` при `isPending` + external `isSubmitting` flag, сбрасываемый в `onSettled` (`frontend-state.md` § Кнопки мутаций, § Double-submit guard).
- Loading/Error состояния обработаны для drawer'а (listCreators paginated), submit'а (Add), confirm-dialog (Remove).
- `data-testid` на каждом интерактивном элементе и ключевом контейнере; `aria-label` на иконках без текста (корзина: `aria-label={t("campaigns:campaignCreators.removeAria")}`).
- Все строки UI — через `react-i18next`; namespace `campaigns`, ключи `campaignCreators.*` (расширяем существующий блок из Spec B).
- Selection state в drawer'е — local `useState<Set<string>>(new Set())`, persists across фильтры/пагинацию, сбрасывается при close.
- Hard-cap 200 в UI: `selectedCount === 200` → все unchecked-чек-боксы блокируются + amber-counter + hint.
- Уже добавленные в кампанию креаторы в drawer'е — disabled-row (`opacity-50`) + badge «Добавлен» в колонке ФИО + чек-бокс disabled.
- На 422 `CREATOR_ALREADY_IN_CAMPAIGN` (race) — `invalidateQueries(campaignCreatorKeys.list(campaignId))`, drawer **не закрывается**, alert; selection не сбрасывается.
- На 404 (кампания только что soft-deleted) — drawer/dialog закрывается + alert + invalidate `campaignKeys.detail(campaignId)` (parent campaign refetch'ится → isDeleted:true → секция перестаёт рендериться по правилу из Spec B).
- На 5xx/network — alert; drawer/dialog НЕ закрывается; selection не сбрасывается.
- Add-кнопка в `CampaignCreatorsSection` (из Spec B) теперь enabled (раньше disabled с tooltip).
- ConfirmDialog — **inline в `features/campaigns/creators/RemoveCreatorConfirm.tsx`**, НЕ в `shared/`. Rule of three: вынести в shared при появлении ≥3 use case'а (chunks 12/13 — рассылки + ремайндеры дадут 3-й; тогда extract).
- `e.stopPropagation()` на onClick корзины — иначе клик одновременно открыл бы detail drawer (existing onRowClick из Spec B).
- Existing detail-drawer flow (`?creatorId=...`) из Spec B сохраняется без изменений — клик по строке вне корзины открывает CreatorDrawer как раньше.
- Spec A (`creators-list-ids-filter`) и Spec B (`campaign-creators-frontend-read`) **должны быть в main** до старта имплементации этой spec'и.
- Coverage frontend ≥80%. `make build-web && make lint-web && make test-unit-web && make test-e2e-frontend` зелёные.

**Ask First:**
- Изменение column-set таблицы (отличающееся от текущего `CreatorsListPage` для drawer'а или Spec B для секции).
- Включение колонки «Статус» (это chunk 15).
- Любая попытка вынести `RemoveCreatorConfirm` в `shared/components/` (rule of three не срабатывает в этом PR).
- Любые отклонения от UX решений зафиксированных в `intent-campaign-creators-frontend.md` / `spec-campaign-creators-frontend-reference.md`.
- Изменения existing API hook'а `useCampaignCreators` помимо аддитивного экспорта `existingCreatorIds: Set<string>` (всё остальное API hook'а — заморожено из Spec B).
- Любые правки в backend (этот PR — только фронт; backend `ids` filter в Spec A, backend chunks 10/12/14 отдельно).

**Never:**
- ConfirmDialog в `shared/components/` сейчас.
- Toast-библиотека (нет в проекте; pattern — inline `role="alert"`).
- Колонки status/счётчики (chunk 15).
- Кнопки рассылок/ремайндеров (chunk 13).
- TMA-flow (chunk 14).
- Прототип Айданы (`frontend/web/src/_prototype/`) как функциональный референс — только визуальный язык.
- Хардкод query keys / route paths / ролей.
- `any` / `!` / `as` / `console.log` / `window.confirm` / `fireEvent` (в тестах).
- Прямой raw `fetch()` вне `api/client.ts`.
- 422-from-agreed e2e — нет business-flow для `agreed` в этом chunk'е (e2e covers только plain remove).
- Удаление любых файлов / экспортов из Spec B — только аддитивные правки (CampaignCreatorsTable получает новую колонку; CampaignCreatorsSection — Add активируется + интеграция drawer/confirm; useCampaignCreators — добавляет `existingCreatorIds`).

## I/O & Edge-Case Matrix

| Scenario | State / Action | Expected UI | Error Handling |
|---|---|---|---|
| Add drawer open | клик «Добавить креаторов» | Drawer справа `w-[1100px] max-w-[90vw]` + header «Добавить креаторов» + `CreatorFilters` + `Table` с чек-боксом первой колонкой + pagination per_page=50 + sort default `created_at desc` + footer counter «выбрано: N / 200» + кнопки Cancel/Submit | — |
| Drawer: уже добавленный | row.id ∈ existingCreatorIds | Чек-бокс disabled, строка `opacity-50`, badge «Добавлен» в колонке ФИО | — |
| Drawer: cap-200 | selectedCount === 200 | Все unchecked-чек-боксы disabled, counter amber, hint «Максимум 200 за одну операцию» | — |
| Drawer: filters/pagination | toggle filter / next page | Selection (`Set<UUID>`) сохраняется; counter виден всегда; уже-добавленные продолжают быть disabled на свежих страницах | — |
| Drawer: clear selection | клик Cancel | Drawer закрывается, selection cleared | — |
| Drawer: Esc/backdrop | Esc или клик по backdrop | Drawer закрывается, selection cleared | — |
| Add submit happy | A1 → 201 + items[] | Drawer закрывается, `invalidateQueries(campaignCreatorKeys.list(campaignId))`, новые строки появляются в Section через refetch (creator-profile подтягивается через `listCreators({ids})` из Spec B) | — |
| Add submit 422 (`CREATOR_ALREADY_IN_CAMPAIGN`) — race | concurrent admin добавил тех же | inline `role="alert"`: «Часть выбранных уже в кампании. Список обновлён, отметьте только новых и повторите.»; invalidate `campaignCreatorKeys.list`; drawer **не закрывается**; selection не сбрасывается; уже-добавленные становятся disabled при следующем render | message через `getErrorMessage(code)` если есть, иначе локальный i18n fallback |
| Add submit 422 другое (`CAMPAIGN_CREATOR_IDS_REQUIRED` / `CAMPAIGN_CREATOR_IDS_DUPLICATES` / `CREATOR_NOT_FOUND` — chunk 10 codes) | server-side validation error | inline alert с message из error response | — |
| Add submit 404 | кампания soft-deleted между fetch и submit | inline alert «Кампания удалена», drawer закрывается, `invalidateQueries(campaignKeys.detail(campaignId))` (parent refetch — isDeleted:true → секция исчезнет по правилу Spec B) | — |
| Add submit 5xx/network | | inline alert «Не удалось сохранить, попробуйте ещё раз»; drawer не закрывается; selection не теряется | retry допустим повторным submit |
| Remove icon click | клик корзины в строке секции | Открывается локальный `RemoveCreatorConfirm`: title «Удалить креатора?», message «{ФИО} будет удалён(а) из кампании. Это действие нельзя отменить.», 2 кнопки Cancel/Confirm | — |
| Remove confirm happy | A2 → 204 | `RemoveCreatorConfirm` закрывается, invalidate list, строка исчезает | — |
| Remove confirm 404 race | concurrent remove | invalidate list, RemoveCreatorConfirm закрывается, silent (или inline alert «Креатор уже удалён» — на usacy агента) | — |
| Remove confirm 422 status=agreed (future-state, chunk 14) | будет в chunk 14 | inline alert «Креатор согласился — удалить нельзя» (готово на будущее, e2e в этом chunk'е НЕ покрываем) | — |
| Remove confirm 5xx/network | | inline alert «Не удалось удалить, попробуйте ещё раз»; dialog не закрывается | retry |
| Remove cancel/Esc/backdrop | | Dialog закрывается без действия | — |
| Add-кнопка после Spec B | была disabled с tooltip | Теперь enabled, click открывает drawer | — |
| Click row (вне корзины) | существующий onRowClick из Spec B | URL `?creatorId=<uuid>` → CreatorDrawer (паттерн Spec B сохраняется) | — |
| Click корзина | onClick кнопки trash | `e.stopPropagation()` — RowClick не срабатывает, открывается RemoveCreatorConfirm | — |
| Auth/role | non-admin | RoleGuard на `/campaigns/:id` filter'ит на 403 (existing) | — |
| Soft-delete creator после add (rare race) | A3 returns ID, listCreators({ids}) не возвращает | Tooltip «Креатор удалён из системы», placeholder в ФИО (наследовано из Spec B); корзина активна (DELETE A2 по creator_id всё равно работает) | — |

</frozen-after-approval>

## Code Map

**New files:**
- `frontend/web/src/features/campaigns/creators/AddCreatorsDrawer.tsx` -- `<Drawer widthClassName="w-[1100px] max-w-[90vw]">` + state {selection: Set<string>, filters, page, sort} + footer counter + Cancel/Submit. `useMutation(addCampaignCreators)`. На 422 `CREATOR_ALREADY_IN_CAMPAIGN` — `queryClient.invalidateQueries({queryKey: campaignCreatorKeys.list(campaignId)})`, drawer открыт, alert; selection не сбрасывается. На 404 — `invalidateQueries(campaignKeys.detail(campaignId))` + alert + onClose. На 5xx — alert; drawer открыт.
- `frontend/web/src/features/campaigns/creators/AddCreatorsDrawerTable.tsx` -- shared `<Table>` с колонкой чек-бокса первой (custom render); колонки те же что в `CreatorsListPage` (повторно используемые из Spec B / `creators` feature); sticky header (`<thead className="sticky top-0 bg-white z-10">`); уже-добавленные `opacity-50` + badge «Добавлен» в колонке ФИО + чек-бокс disabled; чек-бокс disabled при `isMember || (capReached && !isSelected)`.
- `frontend/web/src/features/campaigns/creators/RemoveCreatorConfirm.tsx` -- local inline confirm dialog. Структура: overlay (button `aria-label="Закрыть"` + `bg-black/30`) + centered card (`rounded-card border bg-white max-w-md p-6 shadow-xl`), `role="dialog"` + `aria-modal="true"`. Props: `open`, `onClose`, `onConfirm`, `creatorName`, `isLoading`, `error`. На Esc/backdrop/cancel — `onClose`. Кнопки disabled при `isLoading`. Тексты — через `t('campaigns:campaignCreators.removeConfirm*')`. **НЕ в shared/components/** (rule of three).
- `frontend/web/src/features/campaigns/creators/hooks/useDrawerSelection.ts` -- `useState<Set<string>>(new Set())` + `toggle(id, isMember) / clear() / canSelect(id, isMember) / capReached / size`. Set даёт O(1) `has(id)`. Toggle игнорирует если `isMember` или `capReached && !selected.has(id)`.
- `frontend/web/src/features/campaigns/creators/AddCreatorsDrawer.test.tsx` -- vitest+RTL. Open/close (Esc/backdrop/cancel), selection persists across pages, cap-200 blocks unchecked, submit happy → onClose+invalidate, submit 422 race (alert + invalidate + drawer открыт + selection сохраняется), submit 404 (drawer закрыт + invalidate parent), submit 5xx (alert + drawer открыт), double-submit guard.
- `frontend/web/src/features/campaigns/creators/AddCreatorsDrawerTable.test.tsx` -- disabled-row для already-added (badge + opacity-50), checkbox disabled при cap, click checkbox → onToggle.
- `frontend/web/src/features/campaigns/creators/RemoveCreatorConfirm.test.tsx` -- open/close (Esc/backdrop/cancel), confirm/cancel handlers, isLoading state (кнопки disabled), error display.
- `frontend/web/src/features/campaigns/creators/hooks/useDrawerSelection.test.ts` -- toggle (add/remove), clear, cap-logic (size, canSelect), `isMember` блокирует toggle.
- `frontend/e2e/web/admin-campaign-creators-mutations.spec.ts` -- Playwright, **Russian narrative JSDoc header** (по `frontend-testing-e2e.md`). Full flow: setup admin → создать кампанию через UI → seed 3 creators (через approve flow или /test/seed-creator) → открыть `/campaigns/:id` → клик Add → drawer открыт → отметить 2 → submit → drawer closed → 2 строки в таблице с подтянутыми ФИО → reload → 2 строки persist → клик корзина одного → `RemoveCreatorConfirm` → confirm → 1 строка. Cleanup defer-stack.

**Modified files (additive only):**
- `frontend/web/src/api/campaignCreators.ts` -- добавить `addCampaignCreators(campaignId: string, creatorIds: string[]): Promise<CampaignCreator[]>` и `removeCampaignCreator(campaignId: string, creatorId: string): Promise<void>`. Через generated openapi-fetch client; на non-2xx — throw `ApiError`. Existing `listCampaignCreators` (из Spec B) не меняется.
- `frontend/web/src/features/campaigns/creators/CampaignCreatorsTable.tsx` -- добавить колонку «Действия» с кнопкой trash. Кнопка: `aria-label={t("campaigns:campaignCreators.removeAria")}`, `data-testid="campaign-creator-remove-{id}"`, `onClick={(e) => { e.stopPropagation(); onRemove(row); }}`. Existing onRowClick (URL `?creatorId=...`) — без изменений; срабатывает на клик ВНЕ корзины (благодаря stopPropagation).
- `frontend/web/src/features/campaigns/creators/CampaignCreatorsSection.tsx` -- активировать Add-кнопку (`disabled={false}`, убрать tooltip из Spec B). Добавить state `[isAddOpen, setIsAddOpen]` + `[removeTarget, setRemoveTarget]`. На клик Add → `setIsAddOpen(true)`. На корзина-callback → `setRemoveTarget(row)`. `useMutation(removeCampaignCreator)` с onSuccess → invalidate + setRemoveTarget(null), onError → отображение в RemoveCreatorConfirm.error. Рендерить `<AddCreatorsDrawer open={isAddOpen} onClose={() => setIsAddOpen(false)} campaignId={campaign.id} existingCreatorIds={existingCreatorIds} />` + `<RemoveCreatorConfirm open={!!removeTarget} ... />`.
- `frontend/web/src/features/campaigns/creators/hooks/useCampaignCreators.ts` -- **аддитивно** экспортировать `existingCreatorIds: Set<string>` (computed from A3 result `creator_ids`, memoize через `useMemo`). Используется в drawer'е для пометки disabled-row. Existing API hook'а из Spec B сохраняется.
- `frontend/web/src/features/campaigns/creators/CampaignCreatorsSection.test.tsx` -- расширить existing tests (из Spec B): add open/submit/remove flows. Сохранить existing scenarios (loading/error/empty/happy/soft-deleted).
- `frontend/web/src/features/campaigns/creators/CampaignCreatorsTable.test.tsx` -- расширить existing tests: новая колонка trash, click corzina → onRemove callback called, click row (не на корзине) → onRowClick callback (existing scenario).
- `frontend/web/src/features/campaigns/creators/hooks/useCampaignCreators.test.tsx` -- расширить: `existingCreatorIds` correctly memoized.
- `frontend/web/src/locales/ru/campaigns.json` -- расширить существующий блок `campaignCreators.*` (из Spec B) mutation-ключами: `addDrawerTitle` (= "Добавить креаторов"), `addSubmitButton` (= "Добавить ({{count}})"), `addedBadge` (= "Добавлен"), `capCounter_*` (с pluralization для 0/1/N), `capHint` (= "Максимум 200 за одну операцию"), `removeButton` (= "Удалить"), `removeAria` (= "Удалить креатора из кампании"), `removeConfirmTitle` (= "Удалить креатора?"), `removeConfirmMessage` (= "{{name}} будет удалён(а) из кампании. Это действие нельзя отменить."), `removeConfirmButton` (= "Удалить"), `cancelButton` (= "Отмена"), `errors.alreadyInCampaign` (= "Часть выбранных уже в кампании..."), `errors.campaignDeleted` (= "Кампания удалена"), `errors.removeFailed`, `errors.addFailed`, `errors.generic`. Add-кнопка теперь использует `addButton` (existing из Spec B) уже без `addDisabledTooltip`.

**Notes для feature-агента:**
- `CreatorFilters` (из existing `features/creators/CreatorFilters.tsx`) сейчас использует URL-binding state. Drawer не deep-linkable — потребуется выделить controlled-state-вариант. Решение: вынести state-логику из URL-bindings в отдельный hook, оставив `CreatorFilters` как presentational. Альтернатива: создать локальный `<DrawerCreatorFilters>` с тем же layout. Решает feature-агент при имплементации; **Ask First** если разнобой со standards.

## Tasks & Acceptance

**Execution (предполагается, что Spec A в main, Spec B в main):**
- [x] `frontend/web/src/api/campaignCreators.ts` -- добавить `addCampaignCreators(campaignId, creatorIds: string[]): Promise<CampaignCreator[]>` (POST A1) и `removeCampaignCreator(campaignId, creatorId: string): Promise<void>` (DELETE A2). Через generated openapi-fetch client; на non-2xx → throw `ApiError`.
- [x] `frontend/web/src/features/campaigns/creators/RemoveCreatorConfirm.tsx` -- local inline dialog (НЕ в shared/). `role="dialog"` + `aria-modal="true"`. Esc/backdrop/cancel → onClose. Кнопки disabled при isLoading.
- [x] `frontend/web/src/features/campaigns/creators/hooks/useDrawerSelection.ts` -- + unit-тест на toggle/clear/cap.
- [x] `frontend/web/src/features/campaigns/creators/AddCreatorsDrawerTable.tsx` -- shared `<Table>` с колонкой чек-бокса. disabled-row + badge для уже-добавленных. checkbox disabled при cap.
- [x] `frontend/web/src/features/campaigns/creators/AddCreatorsDrawer.tsx` -- composition: `<Drawer>` + `<CreatorFilters>` (или эквивалент с controlled state — см. Notes) + `<AddCreatorsDrawerTable>` + footer counter + Cancel/Submit. Submit calls `addCampaignCreators(campaignId, [...selection])`. Error handlers: 422 (`CREATOR_ALREADY_IN_CAMPAIGN`) → `invalidateQueries(campaignCreatorKeys.list)` + alert; drawer открыт; selection сохраняется. 404 → `invalidateQueries(campaignKeys.detail)` + alert + onClose. 5xx → alert; drawer открыт. Double-submit guard через external `isSubmitting` flag сбрасываемый в `onSettled`.
- [x] `frontend/web/src/features/campaigns/creators/CampaignCreatorsTable.tsx` -- добавить колонку «Действия». Кнопка trash: `aria-label`, `data-testid`, `onClick={e => { e.stopPropagation(); onRemove(row); }}`.
- [x] `frontend/web/src/features/campaigns/creators/CampaignCreatorsSection.tsx` -- активировать Add-кнопку (`disabled={false}`, убрать tooltip). State для drawer/removeTarget. `useMutation(removeCampaignCreator)` с onSuccess → invalidate + close + onError → передать в RemoveCreatorConfirm. Рендерить `<AddCreatorsDrawer>` + `<RemoveCreatorConfirm>`.
- [x] `frontend/web/src/features/campaigns/creators/hooks/useCampaignCreators.ts` -- **аддитивно** экспортировать `existingCreatorIds: Set<string>` (`useMemo`).
- [x] `frontend/web/src/locales/ru/campaigns.json` -- расширить блок `campaignCreators.*` mutation-ключами (см. Code Map).
- [x] Unit-тесты: `AddCreatorsDrawer.test.tsx`, `AddCreatorsDrawerTable.test.tsx`, `RemoveCreatorConfirm.test.tsx`, `hooks/useDrawerSelection.test.ts`. Расширить `CampaignCreatorsSection.test.tsx` (add open/submit/remove flow), `CampaignCreatorsTable.test.tsx` (колонка trash, stopPropagation), `hooks/useCampaignCreators.test.tsx` (`existingCreatorIds`). Coverage ≥80%.
- [x] E2E `frontend/e2e/web/admin-campaign-creators-mutations.spec.ts` -- Russian narrative header, full flow (см. Code Map). Cleanup defer-stack.

**Acceptance Criteria:**
- Given Spec B в main + 3 approved креатора в БД + admin auth, when admin открывает `/campaigns/:id` и кликает Add, then drawer открывается с полным `CreatorsListPage`-форматом (фильтры, чек-боксы первой колонкой, sort `created_at desc`, page 50).
- Given открытый Add drawer и 250 креаторов в системе, when admin отметил 200, then unchecked-чек-боксы заблокированы, counter становится amber, виден hint «Максимум 200 за одну операцию».
- Given выбраны 3 креатора в Add drawer, when admin переключает page и/или filters и возвращается, then 3 чек-бокса остаются отмеченными, counter «выбрано: 3».
- Given submit Add → 201, then drawer закрывается, в секции появляются N новых строк с подтянутыми ФИО / соцсетями / категориями (через `listCreators({ids})` refetch из Spec B).
- Given backend 422 `CREATOR_ALREADY_IN_CAMPAIGN` (race), when admin submit'ит add, then drawer не закрывается, виден inline alert «Часть выбранных уже в кампании. Список обновлён, отметьте только новых и повторите.», `campaignCreatorKeys.list` инвалидируется, при следующем render таблицы drawer'а ранее-добавленные становятся disabled, selection сохраняется.
- Given backend 404 (campaign soft-deleted между fetch и submit), then drawer закрывается с alert, `campaignKeys.detail` инвалидируется, parent campaign refetch'ится → isDeleted=true → секция перестаёт рендериться (правило Spec B).
- Given клик по корзине + confirm в `RemoveCreatorConfirm`, then DELETE A2 шлётся, dialog закрывается, строка исчезает, `campaignCreatorKeys.list` invalidate.
- Given remove 404 race, then invalidate + dialog close + (silent / короткий alert «Креатор уже удалён»).
- Given клик по корзине, when admin не подтверждает (Cancel/Esc/backdrop), then dialog закрывается, mutation не вызывается.
- Given клик по строке (не на корзине), then URL обновляется → `?creatorId=<uuid>` → CreatorDrawer открыт (правило Spec B сохраняется через `e.stopPropagation()` на корзине).
- Given `make build-web && make lint-web && make test-unit-web && make test-e2e-frontend`, then всё зелёное; coverage ≥80%.

## Verification

**Commands:**
- `make build-web && make lint-web && make test-unit-web` -- frontend tsc/eslint/vitest зелёные.
- `make test-e2e-frontend` -- mutations smoke + read smoke (из Spec B) зелёные.

**Self-check агента (без HALT, между unit и e2e):**
1. `make migrate-up && make start-backend && make run-web`.
2. Войти как admin → создать кампанию → `/campaigns/:id`.
3. Empty state виден; Add теперь enabled (после Spec B был disabled).
4. Click Add → drawer открыт; фильтры/пагинация работают; sort `created_at desc` по умолчанию.
5. Отметить 2 креаторов → submit → drawer закрылся, 2 строки в таблице с подтянутыми ФИО/соцсетями/категориями.
6. Reopen drawer → эти 2 — disabled + badge «Добавлен», чек-бокс выключен.
7. Cap-200: создать 250 креаторов в БД (или через approve flow); открыть drawer; выбрать 200 → counter amber, остальные unchecked-чек-боксы disabled, hint виден.
8. Click trash у одного → `RemoveCreatorConfirm` открыт; click Confirm → строка исчезла; reload → 1 строка persist.
9. `psql` через docker exec: `SELECT campaign_id, creator_id, status FROM campaign_creators WHERE campaign_id='<id>';` — 1 строка status='planned'. `audit_logs WHERE entity_type='campaign_creator';` — 2× add + 1× remove.
10. Race-проверка вручную: открыть 2 вкладки admin'а с одной и той же кампанией; в первой submit add — success; во второй submit add тех же creator_ids — alert «Часть выбранных уже...», invalidate, drawer открыт.
11. Click trash → click Cancel → dialog закрыт, mutation не вызывалась.
12. Click row (НЕ корзина) → URL `?creatorId=...` → CreatorDrawer открыт.
13. Расхождение со спекой = баг → агент сам фиксит, перезапускает self-check, переходит к e2e.

## Suggested Review Order

**Точка входа — оркестрация секции**

- Куда подключаются drawer и confirm-dialog; где живёт removeMutation с granular-422 веткой.
  [`CampaignCreatorsSection.tsx:42`](../../frontend/web/src/features/campaigns/creators/CampaignCreatorsSection.tsx#L42)

**API-слой (новые мутации)**

- Batch-add через openapi-fetch; ApiError для unwrap'а server codes.
  [`campaignCreators.ts:36`](../../frontend/web/src/api/campaignCreators.ts#L36)

- DELETE по compound path-params `(campaignId, creatorId)`.
  [`campaignCreators.ts:53`](../../frontend/web/src/api/campaignCreators.ts#L53)

**Drawer для добавления — error-flows и selection cap**

- useMutation: 422 race очищает selection, 404 silent close, 5xx alert + drawer открыт.
  [`AddCreatorsDrawer.tsx:104`](../../frontend/web/src/features/campaigns/creators/AddCreatorsDrawer.tsx#L104)

- Set-based selection с injectable cap для тестов и hard-cap-200 в проде.
  [`useDrawerSelection.ts:1`](../../frontend/web/src/features/campaigns/creators/hooks/useDrawerSelection.ts#L1)

- Checkbox column: disabled-row для members, capReached блокирует unchecked.
  [`AddCreatorsDrawerTable.tsx:81`](../../frontend/web/src/features/campaigns/creators/AddCreatorsDrawerTable.tsx#L81)

- Controlled-двойник CreatorFilters — изоляция blast radius (см. Notes для feature-агента).
  [`DrawerCreatorFilters.tsx:1`](../../frontend/web/src/features/campaigns/creators/DrawerCreatorFilters.tsx#L1)

**Removal — local inline confirm (rule of three)**

- Modal с role=dialog, Esc/backdrop/Cancel — disabled при isLoading.
  [`RemoveCreatorConfirm.tsx:1`](../../frontend/web/src/features/campaigns/creators/RemoveCreatorConfirm.tsx#L1)

- Trash-колонка с stopPropagation чтобы row-click не пересекался с remove.
  [`CampaignCreatorsTable.tsx:181`](../../frontend/web/src/features/campaigns/creators/CampaignCreatorsTable.tsx#L181)

**Аддитивная правка hook'а Spec B**

- existingCreatorIds: Set<UUID> — мемоизация для drawer'а disabled-row.
  [`useCampaignCreators.ts:80`](../../frontend/web/src/features/campaigns/creators/hooks/useCampaignCreators.ts#L80)

**i18n — расширение namespace campaigns.campaignCreators**

- Все строки UI mutations + selectAria/removeAria для screen-readers.
  [`campaigns.json:60`](../../frontend/web/src/shared/i18n/locales/ru/campaigns.json#L60)

**E2E — полный happy + cancel flow**

- Russian-narrative JSDoc; reload-проверка persistence; reopen → existing badges.
  [`admin-campaign-creators-mutations.spec.ts:1`](../../frontend/e2e/web/admin-campaign-creators-mutations.spec.ts#L1)

**Tests — peripheral (расширения после ревью)**

- 422 race с N>1 selection clear; cap=2 visual проверка hint+disabled; pagination persistence.
  [`AddCreatorsDrawer.test.tsx:1`](../../frontend/web/src/features/campaigns/creators/AddCreatorsDrawer.test.tsx#L1)

- 422 unknown code → fallback removeFailed; soft-deleted creator confirm placeholder.
  [`CampaignCreatorsSection.test.tsx:1`](../../frontend/web/src/features/campaigns/creators/CampaignCreatorsSection.test.tsx#L1)

- Cap-edge коробка: размер, canSelect, off-by-one на cap.
  [`useDrawerSelection.test.ts:1`](../../frontend/web/src/features/campaigns/creators/hooks/useDrawerSelection.test.ts#L1)

- Минимальный smoke на controlled-popover (open/escape/reset/age input).
  [`DrawerCreatorFilters.test.tsx:1`](../../frontend/web/src/features/campaigns/creators/DrawerCreatorFilters.test.tsx#L1)

- Trash column add/stopPropagation; api-client add/remove с 422/404/5xx.
  [`CampaignCreatorsTable.test.tsx:1`](../../frontend/web/src/features/campaigns/creators/CampaignCreatorsTable.test.tsx#L1)
  [`campaignCreators.test.ts:1`](../../frontend/web/src/api/campaignCreators.test.ts#L1)

