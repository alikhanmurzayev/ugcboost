---
title: "Админ-фронт: список + drawer заявок на верификации"
type: feature
created: "2026-05-02"
status: in-progress
baseline_commit: "0cc7307"
context:
  - docs/standards/
  - _bmad-output/planning-artifacts/creator-onboarding-roadmap.md
  - _bmad-output/planning-artifacts/creator-application-state-machine.md
---

> Перед реализацией обязательно полностью загрузить все файлы `docs/standards/` — это hard rules, спека не дублирует их.

<frozen-after-approval reason="human-owned intent — do not modify unless human renegotiates">

## Intent

**Problem:** Chunk 6 roadmap'а. Backend готов (`POST /creators/applications/list`, `GET /counts`, `GET /{id}`), но реальной admin-страницы нет — только мок-прототип под `/prototype/*`.

**Approach:** Перенести `_prototype/features/creatorApplications/` (verification list + drawer + filters + sort + table) в `features/creatorApplications/`, переключить с моков на generated openapi-fetch (server-side filter/sort/pagination). Адаптировать `PrototypeLayout` в `AdminLayout` 1:1 (группы + бейджи) под `RoleGuard(ADMIN)`. Нерабочие nav-пункты — stub-страницы «Скоро».

## Boundaries & Constraints

**Always:**
- `RoleGuard(ADMIN)` на `/creator-applications/*` и `/creators`. Brand-страницы — без guard.
- Server-side фильтры/sort/pagination через `POST /list`. На verification-странице `statuses` хардкод = `["verification"]`. `perPage = 50` фиксирован, не в URL.
- List-state в URL: `q`, `dateFrom/To`, `cities`/`categories` (CSV), `ageFrom/To`, `telegramLinked`, `sort`, `order`, `page`. Drawer state в URL: `?id={uuid}`. Смена фильтра сбрасывает `page=1`.
- Counts (`GET /counts`) — в `AdminLayout`, `enabled: isAdminView`. Бейдж = `counts.find(p => p.status === "verification")?.count ?? 0`. Counts-error → бейдж скрыт, list работает.
- Drawer prev/next — только в пределах loaded page.
- Прототип `_prototype/` остаётся живым — не удаляем.
- `data-testid` по `frontend-components.md` — обязателен на page-wrapper, table, table row (`row-{uuid}`), filter input, sort header, pagination control, drawer container, drawer prev/next/close, nav-link, nav-badge. testid'ы — стабильный контракт для chunk 6.5 (Playwright e2e), не привязаны к копирайту/i18n.
- Unit-тесты по `frontend-testing-unit.md`: VerificationPage (loading/error/empty/data/drawer-toggle/filter URL roundtrip), AdminLayout (admin vs brand nav + бейдж + counts-error скрывает бейдж), filters/sort utils, api-клиент (правильный path и body). Реальный `I18nextProvider` без mock'а i18n. Coverage ≥80% per-method.
- Manual MCP smoke (Playwright MCP, не автотест): полный flow перед PR — на лендосе подать заявку с тестовыми данными → залогиниться admin'ом на web → открыть `/creator-applications/verification` → найти поданную заявку в таблице → кликнуть row → проверить данные в drawer'е → применить фильтр city → закрыть drawer. Этим закрываем chunk 6 без формальных Playwright-тестов; формализуется в chunk 6.5.

**Ask First:**
- Изменение `perPage = 50` или вынос в URL.
- Удаление файлов из `_prototype/`.
- Расширение колонок таблицы за пределы прототипа (№/ФИО/соцсети/категории/дата/часы в этапе).
- Добавление E2E Playwright (default — отдельной задачей).
- Решение по «Просмотр от лица» toggle в `AdminLayout` (default: убрать).

**Never:**
- Действия модерации (approve/reject/sendContract/internalNote) — chunk 10.
- Quality / metrics / contractStatus / rejectionComment / signedAt — chunks 9–11.
- Bulk-actions (multi-select).
- Изменения `backend/` или `openapi.yaml`.
- Ручной `interface` для API request/response (только generated + `Pick`/`Omit`).
- Автоматизированные Playwright-тесты — в этом chunk не пишем (это scope chunk 6.5). Только manual MCP smoke перед PR.

## I/O & Edge-Case Matrix

| Сценарий | State | Behavior |
|---|---|---|
| brand_manager → `/creator-applications/verification` | non-admin | RoleGuard редирект на `/` |
| Empty без фильтров / с фильтрами | `items: []` | «Заявок нет» / «Ничего не найдено» + «Сбросить» |
| Loading list | initial fetch | `<Spinner>` |
| List error | `isError` | `<ErrorState>` retry |
| Drawer на 404 | `?id=valid_uuid`, GET → 404 | inside-drawer error, list жив |
| Malformed URL filter | `?ageFrom=abc` | битое поле игнор, остальные применяются |
| Counts error | `counts.isError` | бейдж скрыт |

</frozen-after-approval>

## Code Map

- `frontend/web/src/api/creatorApplications.ts` — новый. `listCreatorApplications`, `getCreatorApplication`, `getCreatorApplicationsCounts` поверх generated client.
- `frontend/web/src/shared/constants/routes.ts` — добавить `CREATOR_APP_VERIFICATION/MODERATION/CONTRACTS/REJECTED`, `CREATORS`, `CAMPAIGNS_*`, `CAMPAIGN_NEW`, `CAMPAIGN_DETAIL_PATTERN` (без `/prototype` префикса).
- `frontend/web/src/shared/constants/queryKeys.ts` — `creatorApplicationKeys = { all, list(params), detail(id), counts() }`.
- `frontend/web/src/shared/components/Drawer.tsx` — перенос `_prototype/shared/components/Drawer.tsx`.
- `frontend/web/src/shared/layouts/AdminLayout.tsx` — адаптация `_prototype/PrototypeLayout.tsx`: real `logout()` из `api/auth.ts`, real `ROUTES`, counts через real API, без «Просмотр от лица» toggle.
- `frontend/web/src/shared/i18n/locales/ru/creatorApplications.json` — перенос ключей из прототипа без `prototype_*` префикса. Удалить ключи будущих chunks (`actions.*`, `qualityIndicator.*`, `metrics.*`, `drawer.{rejectionComment,internalNote,contractStatus,approvedAt,rejectedAt,signedAt}`, `columns.{contractStatus,rejectionComment}`). `stages.{moderation,contracts,rejected}.title` оставить — нужны для NavLink labels.
- `frontend/web/src/shared/i18n/config.ts` — добавить namespace `creatorApplications`.
- `frontend/web/src/features/creatorApplications/`:
  - `VerificationPage.tsx` — server-side query, drawer через `?id`, URL-state.
  - `components/{ApplicationsTable,ApplicationDrawer,ApplicationFilters,CategoryChip,HoursBadge,SocialLink}.tsx` — перенос с очисткой полей будущих chunks.
  - `filters.ts` — `parseFilters`/`serializeFilters`/`isFilterActive`/`toListInput`.
  - `sort.ts` — `parseSortFromUrl`/`serializeSort` + UI sort key ↔ API enum (`created_at|updated_at|full_name|birth_date|city_name`).
  - `hours.ts`, `types.ts` (aliases от generated `paths`).
- `frontend/web/src/features/creatorApplications/stubs/{Moderation,Contracts,Rejected,Creators}Page.tsx` + `frontend/web/src/features/campaigns/stubs/CampaignsStubPage.tsx` — заголовок «Скоро в разработке» из i18n.
- `frontend/web/src/App.tsx` — admin routes под `AdminLayout` + `RoleGuard(ADMIN)` для `/creator-applications/*` и `/creators`. Campaigns-stub под `AdminLayout` (для 1:1 копии прототипа).

## Tasks & Acceptance

**Execution:**
- [ ] `api/creatorApplications.ts` + расширения `routes.ts`/`queryKeys.ts`/`i18n/config.ts`.
- [ ] `shared/components/Drawer.tsx` (перенос).
- [ ] `shared/layouts/AdminLayout.tsx` (адаптация под real auth/counts).
- [ ] `shared/i18n/locales/ru/creatorApplications.json` (очищенный перенос).
- [ ] `features/creatorApplications/` — page + components + utils + types.
- [ ] Stub-страницы (`stubs/`) для других nav-пунктов.
- [ ] `App.tsx` — admin routes под RoleGuard.
- [ ] Unit-тесты (VerificationPage / AdminLayout / filters / sort / api-client). Coverage ≥80%.
- [ ] Roadmap: `[ ]` → `[~]` (в работе). После merge → `[x]`.

**Acceptance Criteria:**
- Given admin Bearer, when GET `/creator-applications/verification`, then page рендерит таблицу заявок `verification`-статуса.
- Given brand_manager Bearer, when GET `/creator-applications/verification`, then RoleGuard редиректит на `/`.
- Given URL `?dateFrom=2026-04-01&cities=ALA&q=Иван`, when page mounts, then `POST /creators/applications/list` вызывается с `{statuses:["verification"], dateFrom:"2026-04-01", cities:["ALA"], search:"Иван", sort:"created_at", order:"desc", page:1, perPage:50}`.
- Given click на row, when row clicked, then URL → `?id={uuid}`, `GET /creators/applications/{uuid}` вызывается, drawer показывает данные.
- Given counts API → `[{status:"verification",count:7}]`, when `AdminLayout` рендерится для admin, then NavLink «Верификация» имеет бейдж `7`.
- Given counts query → error, when `AdminLayout` рендерится, then бейджа нет, NavLink жив.
- `make build-web lint-web test-unit-web` — зелёное.

## Verification

**Commands:**
- `make build-web` / `make lint-web` / `make test-unit-web` — все зелёные, coverage ≥80%.

**Manual MCP smoke (перед PR):** запустить `make run-backend` + `make run-landing` + `make run-web`; через Playwright MCP пройти полный flow — лендос: подать заявку с тестовыми данными → web как admin: `/creator-applications/verification` → найти заявку в таблице → клик на row → drawer показал данные → применить фильтр city → закрыть drawer. Подтвердить отчётом.
