---
title: 'campaign_creators frontend (chunk 11) — REFERENCE (split into 3 specs)'
type: feature
created: '2026-05-07'
status: reference
context:
  - docs/standards/
  - _bmad-output/planning-artifacts/design-campaign-creator-flow.md
  - _bmad-output/implementation-artifacts/intent-campaign-creators-frontend.md
  - _bmad-output/implementation-artifacts/spec-creators-list-ids-filter.md
  - _bmad-output/implementation-artifacts/spec-campaign-creators-frontend-read.md
  - _bmad-output/implementation-artifacts/spec-campaign-creators-frontend-mutations.md
---

> **REFERENCE NOTE.** Эта spec — полная картина chunk 11 ДО split'а на 3 части (token cap превышен ~6000 tokens). По решению 2026-05-07 разбита на:
>
> - `spec-creators-list-ids-filter.md` — мини-PR backend prereq (`ids[]` filter в `POST /creators/list`).
> - `spec-campaign-creators-frontend-read.md` — vertical slice 1/2 (read-only секция + integration в page).
> - `spec-campaign-creators-frontend-mutations.md` — vertical slice 2/2 (Add drawer + Remove + inline ConfirmDialog + e2e).
>
> Файл сохраняется как **полный reference** на случай вопросов / потери контекста при имплементации частей. Не имплементировать напрямую — каждая из 3 spec'ов выше является источником правды для своего PR.

<frozen-after-approval reason="human-owned intent — do not modify unless human renegotiates">

## Intent

**Problem:** На `/campaigns/:id` админ не может управлять составом креаторов в кампании — нет UI для add/remove/list поверх ручек A1/A2/A3 chunk 10. Без этого зарелизенный backend бесполезен. Дополнительно: A3 возвращает только `creator_id`, а в `POST /creators/list` нет фильтра по `ids[]`, поэтому нельзя подтянуть profile-данные уже добавленных.

**Approach:** Фронт-чанк 11 из `campaign-roadmap.md`. Добавляем на `/campaigns/:id` секцию «Креаторы кампании»: read-таблица + кнопка Add, открывающая drawer справа с полным `CreatorsListPage`-форматом (фильтры, пагинация, чек-боксы первой колонкой, hard-cap 200) + per-row remove через ConfirmDialog. Параллельно расширяем `CreatorsListRequest` опциональным фильтром `ids: array<uuid> maxItems=200` (минимальное серверное расширение в этом же PR) — фронт join'ит profile через `POST /creators/list { ids }`. Без статусов/счётчиков (chunk 15), без рассылок (chunks 12/13), без TMA (chunk 14).

## Boundaries & Constraints

**Always:**
- Полная загрузка `docs/standards/` перед кодом — все правила hard rules. Особенно `frontend-api.md`, `frontend-components.md`, `frontend-quality.md`, `frontend-state.md`, `frontend-types.md`, `frontend-testing-unit.md`, `frontend-testing-e2e.md`, `naming.md`, `security.md`, `review-checklist.md`.
- Все API-типы — только из `frontend/web/src/api/generated/schema.ts` (после `make generate-api`). Ручные `interface`/`type` для request/response запрещены.
- `useMutation` обязан иметь `onError` handler (`frontend-api.md`); кнопки `disabled` при `isPending` + external `isSubmitting` flag, сбрасываемый в `onSettled` (`frontend-state.md`).
- Loading/Error/Empty состояния обработаны для всех queries.
- `data-testid` на каждом интерактивном элементе и ключевом контейнере; `aria-label` на иконках без текста; ошибки валидации — `role="alert"`.
- `localStorage`/`sessionStorage`/`useState`-выноса access token нет (auth уже инкапсулирован в существующем клиенте).
- Все строки UI — через `react-i18next` (`t("...")`); namespace `campaigns`, ключи `campaignCreators.*`. Hardcoded JSX-строки запрещены.
- Selection state в drawer'е — local Set<UUID>, persists across фильтры/пагинацию, сбрасывается при close.
- Hard-cap 200 в UI: при `selectedCount === 200` все unchecked-чек-боксы блокируются + amber-counter + hint.
- Уже добавленные в кампанию креаторы в drawer'е — disabled-row + badge «Добавлен».
- На `campaign.isDeleted === true` секция не рендерится (A3 не вызывается).
- На 422 `CREATOR_ALREADY_IN_CAMPAIGN` (race) — invalidate `campaignCreatorKeys.list`, drawer не закрывается, alert.
- Coverage frontend ≥80%, race-detector backend `-race`, per-method coverage ≥80% backend (`make test-unit-backend-coverage`).

**Ask First:**
- Любые отклонения от ux/api решений зафиксированных в `intent-campaign-creators-frontend.md`.
- Изменение column-set таблицы (отличается от `CreatorsListPage`) — спросить.
- Любая попытка сменить ветку до того, как backend chunk 10 замержен в `main` — спросить (по явной директиве пользователя пишем spec на текущей `-backend` ветке, имплементацию делаем на отдельной ветке после мержа chunk 10).

**Never:**
- Колонка статуса (planned/invited/declined/agreed) и счётчики попыток в UI — chunk 15.
- Кнопки рассылки/ремайндера + selection sub-list — chunk 13.
- Sidebar-навигация по секциям campaign'а — не нужна (единственная секция помимо details/edit).
- Любые TMA-flow, secret-token, agree/decline UI — chunk 14.
- 422-from-agreed e2e — нет business-flow для status=agreed в этом chunk'е.
- Toast-библиотека (в проекте нет, не добавляем; pattern — inline `role="alert"`).
- Хардкод query-keys / route paths / ролей — только через factory/константы.
- `any` / `!` / `as` / `console.log` / `window.confirm` / `fireEvent` (в тестах) — запрещены стандартами.
- Прямой raw `fetch()` вне `api/client.ts` — все запросы через generated openapi-fetch обёртки.
- Прототип `_prototype/features/campaigns/` — только визуальный референс, **не функциональный**.

## I/O & Edge-Case Matrix

| Scenario | Input / State | Expected Output / Behavior | Error Handling |
|---|---|---|---|
| Section initial happy | живая кампания, A3 → 200 + N креаторов | Таблица с колонками №/ФИО/Соцсети/Категории/Возраст/Город/Создан/Действия; счётчик «N в кампании» в заголовке | — |
| Section empty | A3 → 200 `items:[]` | Empty-state «Креаторов пока нет, добавьте через кнопку выше» + Add-кнопка | — |
| Section soft-deleted campaign | `campaign.isDeleted === true` | Секция не рендерится; A3 не вызывается | — |
| Section A3 error | 5xx / network | `<ErrorState onRetry>` внутри секции | retry на refetch |
| Add drawer open | клик на «Добавить креаторов» | Drawer справа `w-[1100px] max-w-[90vw]`, header «Добавить креаторов», `CreatorFilters` + Table + pagination per_page=50, sort по умолчанию `created_at desc` | — |
| Add drawer: уже добавленные | row.id ∈ existingCreatorIds | Чек-бокс disabled, строка `opacity-50`, badge «Добавлен» | — |
| Add drawer: cap-200 | selectedCount === 200 | Все unchecked-чек-боксы disabled, счётчик amber, hint «Максимум 200 за одну операцию» | — |
| Add drawer: filters/pagination | toggle filter / next page | Selection (Set<UUID>) сохраняется; счётчик «выбрано: N» виден всегда | — |
| Add submit happy | A1 → 201 | Drawer закрывается, invalidate `campaignCreatorKeys.list`, новые строки появляются | — |
| Add submit 422 race | concurrent add того же creator | inline `role="alert"`: «Часть выбранных уже в кампании. Список обновлён, отметьте только новых и повторите.»; invalidate list; drawer **не закрывается** | message через `getErrorMessage` |
| Add submit 404 | кампания только что soft-deleted | inline alert «Кампания удалена», drawer закрывается, refetch parent | — |
| Add submit 5xx/network | | inline alert «Не удалось сохранить, попробуйте ещё раз»; drawer не закрывается | retry допустим повторным submit |
| Row click in section | строка добавленного креатора | URL обновляется до `?creatorId=<uuid>`, открывается существующий `CreatorDrawer` (detail) | — |
| Remove icon click | клик на корзину | Открывается `ConfirmDialog`: title «Удалить креатора?», message «{ФИО} будет удалён(а) из кампании. Это действие нельзя отменить.», 2 кнопки | — |
| Remove confirm happy | A2 → 204 | ConfirmDialog закрывается, invalidate list, строка исчезает | — |
| Remove 404 race | concurrent remove | invalidate list, ConfirmDialog закрывается, silent (или короткий inline alert) | — |
| Remove 422 status=agreed | future-state из chunk 14 | inline alert «Креатор согласился — удалить нельзя» (готово на будущее, e2e не покрываем) | — |
| Auth/role | non-admin | RoleGuard на роуте `/campaigns/:id` уже фильтрует на 403 (как для остальных admin-pages) | существующий guard |
| Backend `ids` filter | `POST /creators/list { ids: [a,b] }` | Возвращаются только matching креаторы; пустой массив = NO-OP (filter не применяется) | unknown id просто не возвращается |

</frozen-after-approval>

## Code Map

**Backend (мини-расширение):**
- `backend/api/openapi.yaml` -- добавить `ids: array<uuid> maxItems=200` в `CreatorsListRequest`.
- `backend/internal/repository/creator.go` -- `List` пробрасывает `ids` в squirrel: `Where(squirrel.Eq{CreatorColumnID: ids})` если непуст.
- `backend/internal/service/creator.go` -- domain-input `ListCreatorsInput.IDs []string`; пробрасывается в repo.
- `backend/internal/handler/creator.go` -- `req.Body.Ids` (`[]openapi_types.UUID`) → `[]string` → service input.
- `backend/internal/repository/creator_test.go` -- сценарии `ids:nil` (NO-OP), `ids:[a,b]`, `ids:[unknown]`.
- `backend/internal/service/creator_test.go` -- проброс `ids` в repo (assert через `mock.AnythingOfType` / `mock.Run`).
- `backend/internal/handler/creator_test.go` -- handler парсит `Ids` в request body.
- `backend/e2e/creator/list_test.go` -- `t.Run("filter by ids", ...)`: 3 креатора, `ids:[c1,c3]` → 2 матча.

**Frontend новые файлы:**
- `frontend/web/src/api/campaignCreators.ts` -- обёртки: `listCampaignCreators`, `addCampaignCreators`, `removeCampaignCreator` через `client` (openapi-fetch).
- `frontend/web/src/shared/components/ConfirmDialog.tsx` -- shared overlay+centered card+2 кнопки; props `open/onClose/onConfirm/title/message/confirmLabel/cancelLabel/isLoading/error`. `role="dialog"` + `aria-modal="true"`.
- `frontend/web/src/shared/components/ConfirmDialog.test.tsx` -- open/close/confirm/cancel/loading/error.
- `frontend/web/src/features/campaigns/creators/CampaignCreatorsSection.tsx` -- секция с заголовком, счётчиком, кнопкой Add, таблицей, ConfirmDialog для remove. Пропс `campaign: Campaign`. Возвращает `null` если `campaign.isDeleted`.
- `frontend/web/src/features/campaigns/creators/CampaignCreatorsTable.tsx` -- `<Table>` с колонками /creators + «Действия» (корзина), клик по строке → URL `?creatorId=...`.
- `frontend/web/src/features/campaigns/creators/AddCreatorsDrawer.tsx` -- `<Drawer widthClassName="w-[1100px] max-w-[90vw]">`, state {selection, filters, page, sort}, submit-кнопка `Добавить (N)`. На 422 — `invalidateQueries(campaignCreatorKeys.list)`.
- `frontend/web/src/features/campaigns/creators/AddCreatorsDrawerTable.tsx` -- `<Table>` с колонкой чек-бокса первой; sticky header; уже-добавленные `opacity-50` + badge; чек-бокс `disabled` при `isMember || (capReached && !isSelected)`.
- `frontend/web/src/features/campaigns/creators/hooks/useCampaignCreators.ts` -- `useQuery campaignCreatorKeys.list(campaignId)` → `creatorIds[]`; `useQuery creatorKeys.list({ids,...})` зависимый; mapped row `{ campaignCreator, creator? }`.
- `frontend/web/src/features/campaigns/creators/hooks/useDrawerSelection.ts` -- `useState<Set<string>>` + `toggle/clear/canSelect/capReached`.
- `frontend/web/src/features/campaigns/creators/CampaignCreatorsSection.test.tsx` + `CampaignCreatorsTable.test.tsx` + `AddCreatorsDrawer.test.tsx` + `AddCreatorsDrawerTable.test.tsx` -- vitest + RTL unit тесты.
- `frontend/e2e/web/admin-campaign-creators.spec.ts` -- Playwright e2e, Russian narrative header, flow «add 2 → reload → remove 1».

**Frontend изменения:**
- `frontend/web/src/api/creators.ts` -- `listCreators` пробрасывает `ids: string[]` в request body.
- `frontend/web/src/shared/constants/queryKeys.ts` -- `campaignCreatorKeys = { all: () => [...], list: (id) => [...] }`.
- `frontend/web/src/features/campaigns/CampaignDetailPage.tsx` -- `<CampaignCreatorsSection campaign={campaign} />` под существующей секцией; контейнер `max-w-2xl` → `max-w-7xl` (existing details-секцию ограничить локально). Поддержка query-param `?creatorId=...` для открытия `CreatorDrawer`.
- `frontend/web/src/features/campaigns/CampaignDetailPage.test.tsx` -- расширить: section visibility on `isDeleted=false/true`, query-param drawer.
- `frontend/web/src/locales/ru/campaigns.json` -- `campaignCreators.{title, count, addButton, empty, addDrawerTitle, addSubmitButton, addedBadge, capHint, removeConfirmTitle, removeConfirmMessage, removeConfirmButton, errors.*}`.

## Tasks & Acceptance

**Execution:**
- [ ] `backend/api/openapi.yaml` -- `CreatorsListRequest.ids: array<uuid> maxItems=200` (optional). Description упоминает admin-curated lookup для campaign-creator hydration.
- [ ] `make generate-api` -- регенерит server.gen.go, e2e clients, frontend `schema.ts`. Generated файлы коммитятся в том же PR.
- [ ] `backend/internal/repository/creator.go` -- ветка `if len(ids) > 0 { qb = qb.Where(squirrel.Eq{CreatorColumnID: ids}) }`.
- [ ] `backend/internal/service/creator.go` -- `ListCreatorsInput.IDs`; проброс в repo.
- [ ] `backend/internal/handler/creator.go` -- `req.Body.Ids` (`*[]openapi_types.UUID` если pointer; иначе `[]openapi_types.UUID`) → `[]string` через `pointer.Get` + `id.String()` per element. Если `nil` или `len==0` — пустой слайс в input.
- [ ] Backend unit + e2e -- сценарии `ids` фильтра (см. Code Map). Per-method coverage ≥80%; race-detector включён. `make test-unit-backend && make test-unit-backend-coverage && make test-e2e-backend` зелёные.
- [ ] `frontend/web/src/shared/components/ConfirmDialog.tsx` -- новый shared компонент. После Esc/backdrop-click/cancel — `onClose`. На confirm — `onConfirm` (внешний async). Кнопки `disabled` при `isLoading`. Локализация через `t()` + props (default ru-копирайт).
- [ ] `frontend/web/src/shared/components/ConfirmDialog.test.tsx` -- open/close/confirm/cancel/loading/error states.
- [ ] `frontend/web/src/api/campaignCreators.ts` -- 3 обёртки. На non-2xx — `throw new ApiError(...)` (паттерн проектный, см. `api/client.ts`).
- [ ] `frontend/web/src/api/creators.ts` -- расширить `listCreators` для `ids?: string[]`.
- [ ] `frontend/web/src/shared/constants/queryKeys.ts` -- `campaignCreatorKeys`.
- [ ] `frontend/web/src/features/campaigns/creators/hooks/useDrawerSelection.ts` -- + unit-тест на toggle/clear/cap.
- [ ] `frontend/web/src/features/campaigns/creators/hooks/useCampaignCreators.ts` -- composes A3 + listCreators({ids}); возвращает `{rows, isLoading, isError, refetch, existingCreatorIds: Set<string>}`. + unit-тест.
- [ ] `frontend/web/src/features/campaigns/creators/AddCreatorsDrawerTable.tsx` -- структура: shared `<Table>` с колонкой чек-бокса (custom render), остальные — те же что в CreatorsListPage, плюс badge «Добавлен» в колонке ФИО для already-added rows.
- [ ] `frontend/web/src/features/campaigns/creators/AddCreatorsDrawer.tsx` -- открыт/закрыт через prop; внутри: `<CreatorFilters controlled={...} />` (потребует выноса state из existing hook — feature-агент решит) + `<AddCreatorsDrawerTable>` + footer с counter «выбрано: N / 200» + кнопки Cancel/Submit. На submit — `addCampaignCreators(campaignId, [...selection])`. На 422 — `invalidateQueries(campaignCreatorKeys.list(campaignId))` + alert; drawer не закрывается. На 404 — alert + `onClose()` + invalidate.
- [ ] `frontend/web/src/features/campaigns/creators/CampaignCreatorsTable.tsx` -- `<Table>` с колонками /creators + «Действия». Корзина: кнопка `aria-label={t("campaignCreators.removeAria")}`, `data-testid="campaign-creator-remove-{id}"`, onClick → setRemoveTarget. Click по строке (но не на корзине) — `onRowClick(row.id)` → URL `?creatorId=...`.
- [ ] `frontend/web/src/features/campaigns/creators/CampaignCreatorsSection.tsx` -- composition: `useCampaignCreators` hook + AddCreatorsDrawer + CampaignCreatorsTable + ConfirmDialog. Title «Креаторы кампании» + counter. Empty state с кнопкой Add. Loading/Error states. Hidden если `campaign.isDeleted`.
- [ ] `frontend/web/src/features/campaigns/CampaignDetailPage.tsx` -- расширить контейнер до `max-w-7xl`; details-секцию обернуть в `max-w-2xl`. Под details — `<CampaignCreatorsSection>`. Прочитать `searchParams.get("creatorId")` → открыть `CreatorDrawer` с этим id (как в `/creators` page).
- [ ] `frontend/web/src/locales/ru/campaigns.json` -- блок `campaignCreators.*`.
- [ ] Все unit-тесты выше; coverage ≥80%. `make test-unit-web && make lint-web && make build-web` зелёные.
- [ ] `frontend/e2e/web/admin-campaign-creators.spec.ts` -- Russian narrative JSDoc header. `test.describe('Управление составом креаторов кампании')`: setup admin (login через UI или test-helper) → создать кампанию через UI/API → seed 3 creators (`/test/seed-creator` или approve flow) → открыть `/campaigns/:id` → empty state → клик Add → drawer открыт → отметить 2 → submit → drawer closed → 2 строки → reload → 2 строки → корзина одного → ConfirmDialog → confirm → 1 строка. Cleanup defer-stack.

**Acceptance Criteria:**
- Given чистая БД + admin auth, when admin открывает `/campaigns/:id` живой кампании без креаторов, then секция «Креаторы кампании» видна с empty-state и кнопкой Add.
- Given soft-deleted кампания (`isDeleted=true`), when admin открывает `/campaigns/:id`, then секция не рендерится; existing badge «Удалено» + disabled-Edit работают.
- Given открытый Add drawer и 250 креаторов в системе, when admin отметил 200, then unchecked-чек-боксы заблокированы, counter становится amber, виден hint «Максимум 200 за одну операцию».
- Given выбраны 3 креатора в Add drawer, when admin переключает page и/или filters и возвращается, then 3 чек-бокса остаются отмеченными.
- Given submit Add → 201, then drawer закрывается, в секции появляются 3 новые строки с подтянутыми ФИО/соцсетями/категориями/возрастом/городом (через `POST /creators/list { ids }`).
- Given клик по корзине + confirm в ConfirmDialog, then DELETE A2 шлётся, ConfirmDialog закрывается, строка исчезает, list invalidate.
- Given backend 422 `CREATOR_ALREADY_IN_CAMPAIGN` (race), when admin submit'ит add, then drawer не закрывается, виден inline alert, list инвалидируется, ранее-добавленные становятся disabled при следующем render.
- Given backend `ids` фильтр в `POST /creators/list` с `[c1,c3]` из 3 креаторов, then ответ содержит ровно 2 row'а (c1, c3); пустой `ids` array — фильтр не применяется.
- Given `make build-backend && make lint-backend && make test-unit-backend && make test-unit-backend-coverage && make test-e2e-backend && make build-web && make lint-web && make test-unit-web && make test-e2e-frontend`, then всё зелёное; coverage gate ≥80% (бэк per-method, фронт project-wide).

## Design Notes

**Selection state.** Drawer держит `useState<Set<string>>(new Set())` через `useDrawerSelection` hook. Set позволяет O(1) проверки `has(id)`. `existingCreatorIds: Set<string>` приходит из `useCampaignCreators` — для пометки disabled-row. `capReached = selection.size >= 200`. Toggle-логика: `toggle(id)` смотрит `if (selection.has(id)) delete else if (existing.has(id)) ignore else if (capReached) ignore else add`.

**Resilience к chunk 10 race.** Между моментом, когда фронт загрузил список добавленных и моментом submit'а, другой админ может добавить тех же креаторов → 422 от A1. Тогда мы invalidate `campaignCreatorKeys.list` (refetch), оставляем drawer открытым с alert, и при следующем render таблицы drawer'а уже-добавленные пометятся disabled через свежий `existingCreatorIds`. Без drawer-close, чтобы пользователь не потерял свой не-конфликтный выбор.

**Контейнер CampaignDetailPage.** Текущая страница `max-w-2xl` (узкая) — таблица креаторов с 7 колонками туда не влезет. Расширяем до `max-w-7xl`. Existing details-блок ограничиваем локально `max-w-2xl` чтобы не разрастался. Секция креаторов использует полную ширину.

**Clickable row vs trash.** В `CampaignCreatorsTable` клик по строке → URL `?creatorId=...` → открывает существующий `CreatorDrawer`. Корзина в колонке «Действия» — `<button onClick={(e)=>{e.stopPropagation(); setRemoveTarget(row)}}>`. Без stopPropagation клик на корзине одновременно открыл бы detail drawer.

## Verification

**Commands:**
- `make generate-api` -- regenerated файлы коммитятся.
- `make build-backend && make lint-backend && make test-unit-backend && make test-unit-backend-coverage && make test-e2e-backend` -- ids-фильтр зелёный, coverage gate проходит.
- `make build-web && make lint-web && make test-unit-web` -- frontend tsc/eslint/vitest зелёные.
- `make test-e2e-frontend` -- `admin-campaign-creators.spec.ts` зелёный.

**Self-check агента (между unit и e2e — без HALT):**
1. `make migrate-up && make start-backend && make run-web`.
2. Войти как admin → `/campaigns` → создать кампанию → `/campaigns/:id`.
3. Empty state виден; клик Add → drawer открывается с полным `CreatorsListPage`-форматом (фильтры, чек-боксы первой колонкой).
4. Отметить 2 → submit → drawer закрылся, 2 строки в таблице с подтянутыми ФИО/соцсетями/категориями.
5. Reopen drawer → эти 2 — `disabled + badge «Добавлен»`.
6. Клик корзина одного → ConfirmDialog → confirm → строка исчезла; reload → 1 строка.
7. `docker exec ... psql ... 'SELECT campaign_id, creator_id, status FROM campaign_creators WHERE campaign_id = ?;'` — 1 строка status='planned'. `'SELECT action, entity_id FROM audit_logs WHERE entity_type = ?;' campaign_creator` — 2× add + 1× remove.
8. `curl -X POST .../creators/list -d '{"ids":["..."],"page":1,"perPage":50,"sort":"created_at","order":"desc"}'` — отвечает только запрошенными креаторами.
9. Расхождение со спекой = баг → агент сам фиксит, перезапускает self-check, переходит к e2e в той же сессии.
