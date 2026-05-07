---
title: "Intent: chunk 11 — campaign_creators frontend"
type: intent
status: living
created: "2026-05-07"
chunk: 11
roadmap: _bmad-output/planning-artifacts/campaign-roadmap.md
design: _bmad-output/planning-artifacts/design-campaign-creator-flow.md
split:
  - _bmad-output/implementation-artifacts/spec-creators-list-ids-filter.md
  - _bmad-output/implementation-artifacts/spec-campaign-creators-frontend-read.md
  - _bmad-output/implementation-artifacts/spec-campaign-creators-frontend-mutations.md
reference: _bmad-output/implementation-artifacts/spec-campaign-creators-frontend-reference.md
---

# Intent: chunk 11 — frontend campaign-creators (секция на /campaigns/:id + drawer add/remove)

> **Split note (2026-05-07).** Полная единая spec для chunk 11 превышала token-cap (~6000 tokens, 4× target 1600). По решению — split на 3 vertical-slice'а с реализацией по очереди:
>
> 1. **`spec-creators-list-ids-filter.md`** — backend prereq (мини-PR): `ids[]` filter в `POST /creators/list`. Реализуется первым на отдельной ветке от main (после мержа chunk 10).
> 2. **`spec-campaign-creators-frontend-read.md`** — vertical slice 1/2: read-only секция «Креаторы кампании» + integration в `/campaigns/:id` + Add-кнопка disabled с tooltip. После мержа Spec A.
> 3. **`spec-campaign-creators-frontend-mutations.md`** — vertical slice 2/2: Add drawer (полный CreatorsListPage-формат + cap-200 + race-resilience) + per-row remove + inline `RemoveCreatorConfirm` + e2e flow. После мержа Spec B.
>
> Полная единая картина (на случай вопросов / потери контекста) сохранена в `spec-campaign-creators-frontend-reference.md`.
>
> Этот intent остаётся **living** и описывает chunk 11 как единое целое — split — это исключительно артефакт-разрез под bmad-quick-dev token-cap, а не пересмотр продуктового плана. Все UX-решения ниже распределены по 3 spec'ам без изменений.

## Преамбула — стандарты обязательны

Перед любой строкой production-кода агент обязан полностью загрузить все файлы `docs/standards/` (через `/standards`). Применимы все. Особенно: `frontend-api.md`, `frontend-components.md`, `frontend-quality.md`, `frontend-state.md`, `frontend-testing-e2e.md`, `frontend-testing-unit.md`, `frontend-types.md`, `naming.md`, `security.md`, `review-checklist.md`. Каждое правило — hard rule; отклонение = finding.

## Скоуп

Фронт-чанк 11 из `_bmad-output/planning-artifacts/campaign-roadmap.md`. Полная картина мира — `_bmad-output/planning-artifacts/design-campaign-creator-flow.md` (Группы 4–6).

В этом чанке — UI на `/campaigns/:id` для управления составом креаторов кампании, потребляющий A1/A2/A3 из chunk 10:

- Новая секция **«Креаторы кампании»** на странице кампании (`features/campaigns/CampaignDetailPage.tsx`) — таблица уже добавленных + кнопка «Добавить креаторов» + per-row remove.
- **Drawer справа** для add — широкий drawer (~70% экрана), внутри — полный `CreatorsListPage`-формат (все фильтры + все колонки таблицы + чек-бокс первой колонкой, пагинация). Submit drawer'а = batch POST.
- **Мини-бэк-расширение**: добавить фильтр `ids: []` в `CreatorsListRequest` (`POST /creators/list`) — нужен фронту для подтягивания creator-profile'ов добавленных креаторов (A3 возвращает только `creator_id`).

**Out of scope** (chunks 12/13/15): рассылки, ремайндеры, статусы и счётчики в UI, partial-success delivery, sidebar-навигация по секциям, любые TMA-flow.

## UX-решения (фиксированные)

| # | Решение | Reasoning |
|---|---|---|
| 1 | Add UX = **drawer справа на той же странице** | Не рвёт контекст кампании; pattern уже есть в `features/creators/CreatorDrawer.tsx` (детальный) — расширяем на «list-mode». |
| 2 | Drawer-content = **полный CreatorsListPage-формат** | Все фильтры (`CreatorFilters`) + все колонки таблицы + чек-бокс первой колонкой. Drawer широкий (~70% экрана) для размещения. |
| 3 | Уже добавленные в кампанию = **disabled-row + badge «Добавлен»** | Чек-бокс выключен, строка приглушена, видна как контекст; защита от «куда делся креатор?» при фильтрации. |
| 4 | Selection state = **persists across filters/pages** | Список выбранных UUID живёт в state drawer'а; счётчик «выбрано: N» виден всегда. |
| 5 | Cap = **hard-cap 200 в UI** | При selectedCount === 200 unchecked-чек-боксы блокируются; счётчик amber + hint «Максимум 200 за одну операцию». Совпадает с server `maxItems: 200` из chunk 10. |
| 6 | Creator-profile data = **расширить POST /creators/list фильтром `ids: []`** | Минимальное расширение `CreatorsListRequest` (одно поле + ветка в SQL `WHERE id = ANY(?)`). Frozen spec chunk 10 не трогаем. Реализуется в этом же chunk 11 PR. |
| 7 | Колонки таблицы — **из реального `CreatorsListPage`** | №, ФИО (sortable), Соцсети (`SocialLink` per row), Категории (`CategoryChips`), Возраст (`calcAge(birthDate)`, sortable), Город (sortable), Дата создания (sortable). Никаких followers/views/ER (это были метрики из прототипа Айданы — у нас их пока нет). Секция на странице кампании добавляет колонку «Действия» (иконка корзины → confirm-модалка → DELETE A2). Drawer добавляет колонку чек-боксов первой. |
| 8 | Клик по строке = **переиспользуем `CreatorDrawer` (detail)** | URL state `?creatorId=<uuid>` (отдельный query-param, чтобы не сломать роут кампании). Add drawer — local state (не deep-linkable). Add drawer и detail drawer не открыты одновременно — Add drawer закрывает контент сзади. |
| 9 | Soft-deleted кампания = **секция скрыта полностью** | Если `campaign.isDeleted === true` — секция не рендерится; A3 не вызывается (всё равно 404). Симметрично с уже задизейбленной кнопкой Edit на этой же странице. |

## API-расширение (мини-бэк в этом chunk'е)

`backend/api/openapi.yaml` — `CreatorsListRequest`:

```yaml
ids:
  type: array
  description: Match only creators whose id is in this list (admin-curated lookups, e.g. campaign-creator hydration). Combined with other filters via AND.
  items: { type: string, format: uuid }
  maxItems: 200
```

Backend (chunk 11 PR):
- `service/creator.go` (List) — пробрасывает `ids` в repo.
- `repository/creator.go` (List query) — добавляет `WHERE id = ANY(?)` если `ids` непуст. Squirrel: `squirrel.Eq{CreatorColumnID: ids}`.
- Unit (handler/service/repo) — новые сценарии: фильтр `ids` (1 элемент / N элементов / пустой массив = NO-OP).
- E2E `backend/e2e/creator/list_test.go` — кейс с `ids: [...]`, проверяем что вернулись только запрошенные креаторы; кейс с несуществующим id (просто не вернётся).

## UX-default'ы (без отдельной развилки)

- **Submit-feedback** — inline `<p role="alert">` (toast-инфраструктуры в проекте нет, паттерн совпадает с `CampaignEditSection`). Add drawer показывает ошибку под submit-кнопкой; ConfirmDialog для remove — внутри карточки.
- **Confirm-модалка для remove** — новый shared-компонент `shared/components/ConfirmDialog.tsx` (overlay + centered card + 2 кнопки). В проекте dialog/modal-компонента нет (только `Drawer.tsx`); нативный `window.confirm` запрещён по `frontend-components.md`. ConfirmDialog переиспользуем дальше (рассылки в chunks 12/13).
- **Loading/Error/Empty в секции** — `Spinner` + `ErrorState.onRetry` + текст «Креаторов пока нет. Нажмите “Добавить креаторов”, чтобы выбрать первых».
- **Pagination в drawer'е** — `PER_PAGE = 50` как в `/creators`; pagination state — local (drawer не deep-linkable). Selection — local state в drawer'е (Set<UUID>), сбрасывается при close, восстанавливается при следующем open пустым.
- **Sort/filter state в drawer'е** — local (URL-param'ы не используем — drawer не deep-linkable). `CreatorFilters` переиспользуем как-есть, но в режиме «controlled state», не URL-binding (потребует выноса state из текущего hook'а; решает feature-агент при имплементации).
- **A11y** — `role="dialog"` уже на `Drawer`; ConfirmDialog тоже `role="dialog"` + `aria-modal="true"`. Все интерактивные элементы — `data-testid`. Иконка-корзина — `aria-label="Удалить креатора из кампании"`.
- **Sort/initial state в drawer'е** — `sort: created_at`, `order: desc` (свежие сверху). Filters пустые. Page = 1.
- **Race на Add 422 (`CREATOR_ALREADY_IN_CAMPAIGN`)** — invalidate `campaignCreatorKeys.list(campaignId)` сразу при ошибке (refetch списка добавленных), drawer **не закрывается**, alert: «Часть выбранных уже в кампании. Список обновлён, отметьте только новых и повторите.». При следующем render таблицы drawer'а уже-добавленные станут disabled через свежий `existingCreatorIds`.
- **Race на Remove 404** — invalidate `campaignCreatorKeys.list(campaignId)`, ConfirmDialog закрывается, alert на странице (короткий toast-like inline или просто silent invalidate; решает feature-агент при имплементации — фронт-тест зеркально проверяет).
- **Creator soft-delete после add** — `POST /creators/list { ids }` потенциально вернёт меньше креаторов чем добавлено. Для отсутствующих в join — рендерим строку со значком `—` в колонках ФИО/Соцсети + tooltip «Креатор удалён из системы»; кнопка корзины активна (DELETE A2 по creator_id всё равно работает). Это minor edge — описывает feature-агент при имплементации, отдельно НЕ тестируем e2e (нет flow для soft-delete creator'а в этом chunk'е).

## Frontend: структура и файлы

```
frontend/web/src/
├── api/
│   └── campaignCreators.ts                  # NEW — addCampaignCreators / removeCampaignCreator / listCampaignCreators (обёртки над generated openapi-fetch)
├── features/campaigns/
│   ├── CampaignDetailPage.tsx               # +<CampaignCreatorsSection campaignId=... />
│   └── creators/                            # NEW — sub-feature внутри campaigns
│       ├── CampaignCreatorsSection.tsx
│       ├── CampaignCreatorsTable.tsx
│       ├── AddCreatorsDrawer.tsx
│       ├── AddCreatorsDrawerTable.tsx
│       ├── *.test.tsx                       # vitest unit per-component
│       └── hooks/
│           ├── useCampaignCreators.ts        # composes A3 + listCreators({ ids: [...] }) → join row
│           └── useDrawerSelection.ts         # selection set + cap-200 logic
├── shared/
│   ├── components/
│   │   └── ConfirmDialog.tsx                # NEW — переиспользуемая confirm-модалка
│   └── constants/queryKeys.ts               # +campaignCreatorKeys factory
└── locales/ru/campaigns.json                # +campaignCreators.* keys (расширяем существующий namespace)

frontend/e2e/web/
└── admin-campaign-creators.spec.ts          # NEW — Playwright smoke (Russian narrative header)
```

## API-обёртки

```ts
// frontend/web/src/api/campaignCreators.ts (новый файл)
import { client } from "./client";
import type { components, paths } from "./generated/schema";

export type CampaignCreator = components["schemas"]["CampaignCreator"];

export async function listCampaignCreators(campaignId: string): Promise<CampaignCreator[]> { ... }
export async function addCampaignCreators(campaignId: string, creatorIds: string[]): Promise<CampaignCreator[]> { ... }
export async function removeCampaignCreator(campaignId: string, creatorId: string): Promise<void> { ... }
```

`api/creators.ts` — расширяем `listCreators` чтобы пробрасывать новый `ids: string[]` filter из `CreatorsListRequest`.

## Query keys

```ts
// shared/constants/queryKeys.ts (расширяем)
export const campaignCreatorKeys = {
  all: () => ["campaignCreators"] as const,
  list: (campaignId: string) => ["campaignCreators", "list", campaignId] as const,
};
```

После Add/Remove инвалидируем `campaignCreatorKeys.list(campaignId)`. Detail-drawer креатора использует существующий `creatorKeys.detail(id)`.

## I/O & Edge-Case Matrix

| Scenario | Trigger / State | Expected UI |
|---|---|---|
| Initial load (живая кампания) | A3 → 200 пустой `items: []` | Секция «Креаторы кампании» с пустым state «Креаторов пока нет, добавьте первых» + кнопка Add |
| Initial load (живая, с креаторами) | A3 → 200, потом `POST /creators/list { ids: [...] }` | Таблица с полными колонками /creators + per-row корзина |
| A3 ошибка | 5xx / network | `<ErrorState onRetry={refetch}>` внутри секции |
| Soft-deleted кампания | `campaign.isDeleted === true` | Секция не рендерится |
| Add drawer: загрузка списка | listCreators → loading | Spinner внутри drawer'а |
| Add drawer: фильтры/пагинация | toggle filters / pagination | Selection state сохраняется (Set<UUID>) |
| Add drawer: уже добавленный | row.id ∈ existingCreatorIds | Чек-бокс disabled, badge «Добавлен», строка `opacity-50` |
| Add drawer: cap = 200 | selectedCount === 200 | Все unchecked-чек-боксы disabled, hint «Максимум 200 за одну операцию», счётчик amber |
| Add submit happy | A1 → 201 | Drawer закрывается, invalidate `campaignCreatorKeys.list`, таблица обновляется |
| Add submit 422 (creator уже добавлен — race) | concurrent admin добавил того же | inline `role="alert"`: «Креатор уже в кампании. Обновите список и повторите» (use `getErrorMessage(code)` из `shared/i18n/errors.ts` если есть; иначе локальный fallback) |
| Add submit 404 | кампания только что soft-deleted | inline alert «Кампания удалена» + закрыть drawer |
| Remove кнопка | clicked | Открывается ConfirmDialog: «Удалить {ФИО} из кампании? Это действие нельзя отменить.» |
| Remove submit happy | A2 → 204 | ConfirmDialog закрывается, invalidate `campaignCreatorKeys.list` |
| Remove submit 404 (gone) | concurrent remove | inline alert «Креатор уже удалён» + закрыть dialog + refetch |
| Remove submit 422 (status=agreed) | future-state из chunk 14 | inline alert «Креатор согласился — удалить нельзя» (готово на будущее, в chunk 11 e2e не тестируется — нет business-flow для agreed) |

## Tasks & Acceptance

**Execution:**
- [ ] `backend/api/openapi.yaml` — добавить `ids: array<string format=uuid> maxItems=200` в `CreatorsListRequest`. Описание про admin-curated lookups.
- [ ] `make generate-api` — server.gen.go, e2e clients, `frontend/web/src/api/generated/schema.ts`, `frontend/e2e/types/schema.ts`. Generated файлы коммитятся.
- [ ] `backend/internal/repository/creator.go` — `List` пробрасывает `ids` фильтр в squirrel: `Where(squirrel.Eq{CreatorColumnID: ids})` если `len(ids) > 0`.
- [ ] `backend/internal/service/creator.go` (`List`) — domain-input `ListCreatorsInput.IDs []string`; пробрасывается в repo.
- [ ] `backend/internal/handler/creator.go` (`ListCreators`) — `req.Body.Ids` → `[]uuid.UUID` → `[]string` → service input.
- [ ] Unit-тесты backend: handler/service/repo `List` — новые сценарии `ids: []` (empty = NO-OP), `ids: [a,b]` (returns matching), `ids: [non-existent]` (returns empty). Per-method coverage ≥80%.
- [ ] E2E `backend/e2e/creator/list_test.go` — добавить `t.Run("filter by ids", ...)`: создаём 3 креаторов, шлём filter `{ ids: [c1, c3] }`, ожидаем 2 строки.
- [ ] `frontend/web/src/shared/components/ConfirmDialog.tsx` — новый компонент: overlay (button `aria-label="Закрыть"` + bg-black/30) + centered card (rounded-card border bg-white max-w-md p-6), `role="dialog"` + `aria-modal="true"`, props: `open`, `onClose`, `onConfirm`, `title`, `message`, `confirmLabel`, `cancelLabel`, `isLoading`, `error`.
- [ ] `frontend/web/src/api/campaignCreators.ts` — listCampaignCreators / addCampaignCreators / removeCampaignCreator (обёртки над generated openapi-fetch).
- [ ] `frontend/web/src/api/creators.ts` — расширить `listCreators` для проброса `ids`.
- [ ] `frontend/web/src/shared/constants/queryKeys.ts` — добавить `campaignCreatorKeys`.
- [ ] `frontend/web/src/features/campaigns/creators/CampaignCreatorsSection.tsx` — секция с заголовком, счётчиком, кнопкой Add, таблицей, ConfirmDialog для remove.
- [ ] `…/CampaignCreatorsTable.tsx` — переиспользует `shared/components/Table.tsx` с колонками /creators + колонка действия (корзина); клик по строке → URL `?creatorId=...`; кнопка Add — primary; кнопка корзины — `aria-label`, `data-testid="campaign-creator-remove-{id}"`.
- [ ] `…/AddCreatorsDrawer.tsx` — `<Drawer widthClassName="w-[1100px] max-w-[90vw]">` + header «Добавить креаторов» + state {selection: Set<string>, filters, page, sort} + сабмит-кнопка `Добавить (N)` (primary). Перед submit — guard: `selectedCount > 0`.
- [ ] `…/AddCreatorsDrawerTable.tsx` — таблица с колонкой чек-бокса первой; колонки те же; уже добавленные `opacity-50` + badge; чек-боксы disabled при `isMember || (capReached && !isSelected)`; sticky header.
- [ ] `…/hooks/useCampaignCreators.ts` — `useQuery campaignCreatorKeys.list(campaignId)`; затем `useQuery creatorKeys.list({ ids: ... })` зависимый; mapped row = `{ campaignCreator, creator? }`.
- [ ] `…/hooks/useDrawerSelection.ts` — `useState<Set<string>>` + `toggle / clear / canSelect / capReached` логика.
- [ ] `frontend/web/src/features/campaigns/CampaignDetailPage.tsx` — `<CampaignCreatorsSection>` под `<CampaignSection>` (только если `!campaign.isDeleted`); ширина страницы — расширить с `max-w-2xl` до `max-w-7xl` (текущая узкая секция details переедет под этот же контейнер; внутри её ограничим `max-w-2xl` сами).
- [ ] `frontend/web/src/locales/ru/campaigns.json` — добавить блок `campaignCreators.*` (title, count, addButton, empty, addDrawerTitle, addSubmitButton, addedBadge, capHint, removeConfirmTitle, removeConfirmMessage, removeConfirmButton, errors.*).
- [ ] Unit-тесты (vitest + RTL): `CampaignCreatorsSection.test.tsx` (loading/error/empty/happy + soft-deleted hidden), `CampaignCreatorsTable.test.tsx` (per-row remove flow, click→URL change), `AddCreatorsDrawer.test.tsx` (open/close, selection persists across pages, cap-200 blocks unchecked, submit happy + error 422), `AddCreatorsDrawerTable.test.tsx` (disabled-row для already-added, badge), `ConfirmDialog.test.tsx` (open/close, confirm/cancel, loading, error). Покрытие ≥80%.
- [ ] E2E `frontend/e2e/web/admin-campaign-creators.spec.ts` — Russian narrative header. `test.describe('campaign creators flow')`: setup admin (`/test/seed-user`) → создать кампанию через UI или API helper → seed 3 креатора (`/test/seed-creator` если есть, иначе через approve flow) → открыть `/campaigns/:id` → empty state + Add button visible → клик Add → drawer открыт → отметить 2 → submit → drawer closed → 2 строки в таблице → remove одного через корзину + ConfirmDialog → 1 строка → reload page → 1 строка персистится. Cleanup defer-stack.

**Acceptance Criteria:**
- Given чистая БД + admin auth, when админ заходит на `/campaigns/:id` живой кампании без креаторов, then секция «Креаторы кампании» видна с empty-state и кнопкой Add.
- Given soft-deleted кампания (`isDeleted=true`), when админ заходит на `/campaigns/:id`, then секция не рендерится; остальная страница работает (badge «Удалено», disabled Edit).
- Given открытый Add drawer с 250 unchecked креаторами, when админ выбрал 200, then unchecked-чек-боксы заблокированы; счётчик amber; hint «Максимум 200 за одну операцию» виден.
- Given выбраны 3 креатора в Add drawer, when админ переключил page и вернулся, then 3 чек-бокса остаются отмечены.
- Given submit Add → 201, then drawer закрывается, в секции появляются 3 новые строки (через invalidate) с правильными ФИО/соцсетями/категориями (creator-profile подтягивается через `POST /creators/list { ids }`).
- Given клик по корзине, when админ подтверждает в ConfirmDialog, then DELETE A2 шлётся, строка исчезает из таблицы.
- Given бэкенд 422 (concurrent re-add), when admin submit'ит add того же creator, then drawer показывает inline alert с понятным текстом, drawer не закрывается.
- Given `make test-unit-web && make lint-web && make build-web`, then всё зелёное; coverage gate ≥80%.
- Given backend поднят локально + admin token, when запускается `make test-e2e-frontend`, then `admin-campaign-creators.spec.ts` зелёный.
- Given `make generate-api`, then generated файлы (server.gen.go + frontend schemas + e2e schemas) перегенерированы и коммитятся.
- Given `make lint-backend && make test-unit-backend && make test-unit-backend-coverage && make test-e2e-backend`, then всё зелёное (бэк-расширение `ids` фильтра).

## Verification

**Commands:**
- `make generate-api`
- `make build-backend && make lint-backend && make test-unit-backend && make test-unit-backend-coverage && make test-e2e-backend` (бэк-расширение `ids`)
- `make build-web && make lint-web && make test-unit-web && make test-e2e-frontend` (фронт)

**Self-check агента (между unit и e2e):**
1. `make migrate-up && make start-backend && make run-web`
2. Войти как admin → `/campaigns` → создать кампанию → перейти на `/campaigns/:id`
3. Empty-state «Креаторов пока нет» виден; клик «Добавить креаторов» → drawer открывается с полным списком из `/creators` (фильтры, пагинация, чек-боксы первой колонкой).
4. Отметить 2 креаторов → submit → drawer закрылся, в таблице 2 строки с ФИО/соцсетями/категориями.
5. Снова Add → видно что эти 2 — `disabled + badge «Добавлен»`; снять с них нельзя.
6. Клик корзина у одного → ConfirmDialog → подтвердить → строка исчезла; reload page → 1 строка осталась.
7. `psql` через docker exec: `SELECT campaign_id, creator_id, status FROM campaign_creators WHERE campaign_id = '<id>';` — 1 строка status='planned'. `audit_logs WHERE entity_type='campaign_creator';` — 2× add + 1× remove.
8. Расхождение со спекой = баг → агент сам фиксит, без HALT.

## Что не делаем в чанке 11

- Колонка статуса (planned/invited/declined/agreed) и счётчики — chunk 15.
- Кнопки рассылки/ремайндера + selection sub-list для рассылки — chunk 13.
- Sidebar-навигация по секциям campaign'а (как в прототипе Айданы) — не нужна, единственная секция помимо details/edit.
- Любые TMA-flow.

## Связанные документы

- Roadmap: `_bmad-output/planning-artifacts/campaign-roadmap.md`
- Design Групп 4–6: `_bmad-output/planning-artifacts/design-campaign-creator-flow.md`
- Backend chunk 10 spec: `_bmad-output/implementation-artifacts/spec-campaign-creators-backend.md` (frozen)
- Стандарты: `docs/standards/`
- Frontend эталоны UI: `features/creators/`, `features/campaigns/CampaignDetailPage.tsx`, `features/auth/`, `features/dashboard/`
- Прототипы Айданы (только визуальный референс, **не функциональный**): `frontend/web/src/_prototype/features/campaigns/`
