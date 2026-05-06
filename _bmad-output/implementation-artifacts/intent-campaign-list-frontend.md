---
title: "Intent: фронт-страница списка кампаний (chunk 9 campaign-roadmap)"
type: intent
status: draft
chunk: "campaign-roadmap chunk 9"
created: "2026-05-06"
updated: "2026-05-06"
---

# Intent: фронт-страница списка кампаний (chunk 9)

> **Преамбула.** Перед стартом реализации агент обязан полностью загрузить
> `docs/standards/` (все файлы целиком) и применять их как hard rules. Этот
> документ — only intent, не PRD: фиксирует, ЧТО строим и ПОЧЕМУ так,
> а не пошаговую реализацию.

## Контекст

Девятый chunk roadmap'а кампаний (`_bmad-output/planning-artifacts/campaign-roadmap.md`).
Цель — заменить заглушку `features/campaigns/stubs/CampaignsStubPage.tsx`
полноценной admin-страницей `/campaigns` для CRUD-входа в кампании.

**Зависимости (бэк):**
- `GET /campaigns` (chunk 6) — спека готова, в работе у параллельного агента
  (`_bmad-output/implementation-artifacts/spec-campaign-list-backend.md`).
- `DELETE /campaigns/{id}` soft-delete (chunk 7) — **не нужен этому PR'у**:
  delete-кнопки в UI отображаются, но `disabled` с tooltip «появится
  позже». Активируются отдельным мини-PR'ом, когда chunk 7 в main.

**Реализация фронта стартует после мержа chunk 6 в main.**
Дизайн идёт сейчас параллельно, чтобы как только backend готов — фронт
шёл без задержки.

**Прототип Айданы:** `frontend/web/src/_prototype/features/campaigns/CampaignsPage.tsx`
— устарел: построен вокруг статусов кампаний (active/pending/draft/...),
которых roadmap явно не делает. Берём из прототипа только визуальный
ориентир, не модель данных.

**Эталон паттерна:** chunk 2 (`features/creators/CreatorsListPage.tsx`,
архивная спека `2026-05-06-spec-creators-list-frontend.md`) — RoleGuard(ADMIN),
таблица + filters + sort + URL-state, Vitest unit + Playwright e2e.

## Тезис

Admin-only страница `/campaigns` под `RoleGuard(ADMIN)`: таблица всех
кампаний с поиском по названию, сортировкой, пагинацией, фильтром
по `is_deleted`, soft-delete-действием в строке, переходом в детальную
(chunk 8 `/campaigns/:id`) и кнопкой «Создать кампанию» (`/campaigns/new`,
тоже chunk 8). Без статусов, без LiveDune-блоков, без drawer'а
(детальная — отдельная страница chunk 8, не sidesheet).

## Фактическая модель кампании

Бэк-контракт (`backend/api/openapi.yaml` schema `Campaign`,
миграция `20260506021843_campaigns.sql`):

```
{ id, name (unique among non-deleted), tmaUrl, isDeleted, createdAt, updatedAt }
```

`tmaUrl` — ссылка на TMA-страницу с ТЗ (секретный лендинг внутри
TMA-приложения). Подставляется в outbound-приглашения креатору.
Никаких `public_brief / private_brief / event_date` в БД нет.
Roadmap chunks 3 и 8 синхронизированы с фактической моделью
2026-05-06.

## Концепт-каркас (3 экрана)

1. **`/campaigns`** (chunk 9, list) — таблица с поиском по `name`,
   sort, pagination, фильтром `isDeleted`; колонки `index | name |
   tmaUrl | createdAt | actions(⋮)`. Action-меню: «Открыть» (= row click)
   и «Удалить» (`disabled` + tooltip «появится позже» — chunk 7 ещё
   не готов). Кнопка «Создать кампанию» в шапке → `/campaigns/new`.

2. **`/campaigns/new`** (chunk 8a, create) — отдельная страница, форма
   из 2 полей: `name` (input), `tmaUrl` (input). Submit → `POST /campaigns`
   → redirect на `/campaigns/:id`. Loading/Error states; submit `disabled`
   при `isPending` + double-submit guard.

3. **`/campaigns/:id`** (chunk 8b, detail+edit, foundation для chunk 11)
   — одна страница с двумя режимами view ↔ edit (тоггл «Редактировать»
   в header, без отдельного `/edit` под-роута). Секции:
   - **Основное** — `name` + `tmaUrl`. View — read-only с copy-кнопкой
     на ссылку. Edit — те же 2 input'а + Save/Cancel.
   - **Креаторы** (placeholder под chunk 11) — заглушка «Добавление
     креаторов появится в следующем chunk'е». Когда chunk 11 поднимется
     — мультиселект из approved через переиспользование `POST /creators/list`.
   - **Действия** — кнопка «Удалить» (`disabled` + tooltip «появится
     позже» в этом PR'е; активируется отдельно, когда chunk 7 готов).
     Soft-deleted → banner «Кампания удалена», все mutations disabled.

## Решения (interview 2026-05-06)

- **`isDeleted` UX** — чекбокс «Показать удалённые» в toolbar `/campaigns`.
  Off (default) → видно только живые (`isDeleted=false` в API-запросе).
  On → видно ВСЕ кампании, включая удалённые (параметр опускается из
  API-запроса). URL: `?showDeleted=true` когда чекбокс включён, иначе
  опускается. Удалённые ряды визуально помечены (приглушённый стиль /
  бейдж «Удалена») — точная вёрстка отдаётся bmad-quick-dev. Restore
  не делаем (как и решено в roadmap'е).
- **Routes cleanup** — этим же PR'ом дропаем все 5 status-based маршрутов
  (`CAMPAIGNS_ACTIVE/PENDING/REJECTED/DRAFT/COMPLETED`) и добавляем
  единственный `CAMPAIGNS = "campaigns"`. `CAMPAIGN_NEW`, `CAMPAIGN_DETAIL`,
  `CAMPAIGN_DETAIL_PATTERN` — оставляем (используются chunk 8). Сайдбар
  переподключаем на `CAMPAIGNS`. Стаб `CampaignsStubPage.tsx` удаляется.
- **Прототип `_prototype/features/campaigns/`** — оставляем как есть. Папка
  изолирована, в production-код не импортируется; будет полезна как
  визуальная отсылка при расширении формы (audience / content / payment
  секции из прототипа), не требует немедленной чистки.

## Связанные артефакты

- Roadmap: `_bmad-output/planning-artifacts/campaign-roadmap.md`
- Параллельная backend-спека (chunk 6): `_bmad-output/implementation-artifacts/spec-campaign-list-backend.md`
- Эталон фронт-list-страницы: архив `2026-05-06-spec-creators-list-frontend.md`
- Прототип (визуал, не модель): `frontend/web/src/_prototype/features/campaigns/CampaignsPage.tsx`
- Стандарты: `docs/standards/`
