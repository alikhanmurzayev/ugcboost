---
title: "Chunk 15 — счётчики и timestamps попыток на странице кампании"
type: feature
created: "2026-05-08"
status: done
baseline_commit: d667fef
context:
  - docs/standards/frontend-components.md
  - docs/standards/frontend-testing-unit.md
  - docs/standards/frontend-testing-e2e.md
---

<frozen-after-approval reason="human-owned intent — do not modify unless human renegotiates">

## Intent

**Problem:** На `/campaigns/:id` админ видит креаторов в 4 статусных секциях, но не видит счётчики попыток приглашения / ремайндера и когда именно креатор был приглашён, отказался или согласился. Без этого нельзя понять, давно ли тишина после приглашения и пора ли слать ремайндер.

**Approach:** Расширить `CampaignCreatorsTable` контекстными колонками, набор зависит от пропса `status: CampaignCreatorStatus`. Все данные уже отдаёт `GET /campaigns/{id}/creators` (A3) — бэк-контракт не меняется. Отдельной колонки `status` не вводим — статус виден из заголовка секции.

## Boundaries & Constraints

**Always:**
- Раскладка колонок по `status` (composite — пары count+at в одной ячейке):
  - `planned` — никаких новых
  - `invited` — 2 ячейки: «Приглашение» (`invitedCount · invitedAt`), «Ремайндер» (`remindedCount · remindedAt`)
  - `declined` / `agreed` — 2 ячейки: «Приглашение» (`invitedCount · invitedAt`), «Решение» (`decidedAt` без count)
- Порядок в строке: ФИО → соцсети → категории → возраст → город → **новые** → `createdAt` → actions.
- Composite-ячейка рендерит `{count}` (число, включая `0`), middot-разделитель `·`, и `formatDateTimeShort(iso)` (`—` при `null`/невалидном ISO). Decided-ячейка — только timestamp без count.
- Формат timestamp: `6 мая, 14:30` — `toLocaleString("ru", { day:"numeric", month:"short", hour:"2-digit", minute:"2-digit" })`, без года.
- i18n: новые ключи в `campaignCreators.columns.{invited|reminded|decided}`. Литералы в JSX запрещены (стандарт `frontend-components.md`).
- data-testid на inner-span'ах внутри composite-ячейки: `campaign-creator-{kind}-{count|at}-{creatorId}`, `kind` ∈ `invited` / `reminded` / `decided`. Decided не имеет count-span'а.
- Стандарты `docs/standards/frontend-*` обязательны (TS strict, RTL `userEvent`, без `any`/`!`/`as`).

**Ask First:**
- Миграция existing `formatDateTime` из `CampaignDetailPage.tsx:264` на shared util — out of scope, HALT перед таким рефакторингом.

**Never:**
- Колонку `status` в таблицу не добавляем (дубль заголовка секции).
- Бэк-контракт / OpenAPI / Go-код не трогаем.
- e2e под `decidedAt` (`declined`/`agreed`) — не покрываем; уйдёт в spec chunk 14 после мержа TMA-flow.
- Локальный дубликат `formatDateTimeShort` в файле — нет; только из `shared/utils/`.
- Файлы chunk 14 / chunk 13a (intent/spec) и их ветку — не трогаем (параллельный агент).

## I/O & Edge-Case Matrix

| Scenario | Input | Expected |
|---|---|---|
| `planned` | любая запись | новых колонок в DOM нет |
| `invited`, после первого notify | `invitedCount=1, invitedAt=ISO_a, remindedCount=0, remindedAt=null` | 2 ячейки: «Приглашение» = `1 · {ISO_a formatted}`; «Ремайндер» = `0 · —` |
| `invited`, после remind | `invitedCount=1, invitedAt=ISO_a, remindedCount=2, remindedAt=ISO_b` | «Приглашение» = `1 · {ISO_a}`; «Ремайндер» = `2 · {ISO_b}` |
| `declined` / `agreed` | `invitedCount=2, invitedAt=ISO_a, decidedAt=ISO_c` | 2 ячейки: «Приглашение» = `2 · {ISO_a}`; «Решение» = formatted `ISO_c`; reminded-ячейки нет |
| Невалидный или null timestamp | `invitedAt="not-a-date"` или `null` | в composite-ячейке вместо timestamp `—`, без runtime-throws; count-часть рендерится как обычно |

</frozen-after-approval>

## Code Map

- `frontend/web/src/shared/utils/formatDateTime.ts` + `.test.ts` -- новая утилита `formatDateTimeShort`.
- `frontend/web/src/shared/i18n/locales/ru/campaigns.json` -- блок `campaignCreators.columns.*`.
- `frontend/web/src/features/campaigns/creators/CampaignCreatorsTable.{tsx,test.tsx}` -- prop `status`, `buildColumns` с контекстными колонками; тесты на 4 статуса.
- `frontend/web/src/features/campaigns/creators/CampaignCreatorGroupSection.{tsx,test.tsx}` -- проброс `status`; spot-тест.
- `frontend/e2e/web/campaign-notify.spec.ts` -- asserts на новые data-testid в существующих сценариях; абзац в header-JSDoc.

## Tasks & Acceptance

**Execution:**
- [x] `shared/utils/formatDateTime.ts` -- `formatDateTimeShort(iso)`; null/undefined/невалидный → `"—"`; иначе `toLocaleString("ru", {day,month:"short",hour:"2-digit",minute:"2-digit"})`.
- [x] `shared/utils/formatDateTime.test.ts` -- 3 кейса: валидный ISO (assert через regex/contains), `null`, `"not-a-date"`.
- [x] `shared/i18n/locales/ru/campaigns.json` -- `campaignCreators.columns`: `invitedCount:"Приглашений"`, `invitedAt:"Когда приглашён"`, `remindedCount:"Ремайндеров"`, `remindedAt:"Когда ремайндер"`, `decidedAt:"Решение от"`.
- [x] `CampaignCreatorsTable.tsx` -- required prop `status`; собрать `extraColumns` по `status` и вставить между `city` и `createdAt`. Counter-ячейка `<span>{count}</span>`, timestamp `formatDateTimeShort(iso)`. Каждая ячейка получает data-testid `campaign-creator-{kind}-{count|at}-{creatorId}`.
- [x] `CampaignCreatorsTable.test.tsx` -- параметризовать `makeCC` counter-полями; describe «counter columns» × 4 кейса по статусам, с `0`-remindedCount и null-remindedAt в `invited`.
- [x] `CampaignCreatorGroupSection.tsx` -- передать свой `status` в `<CampaignCreatorsTable>`.
- [x] `CampaignCreatorGroupSection.test.tsx` -- 1 spot-тест `status:"invited"` → counter-ячейки в DOM.
- [x] `e2e/web/campaign-notify.spec.ts` -- после notify assert `invited-count={creatorId}` text=`1` и непустой `invited-at`; после remind — `reminded-count=1` и непустой `reminded-at`. Header-JSDoc обновить (абзац про counter-инварианты).

**Acceptance Criteria:**
- Given `invited` с `(invitedCount=2, invitedAt, remindedCount=1, remindedAt)`, when таблица рендерится, then 4 новые колонки видны с корректными значениями.
- Given `planned`, when таблица рендерится, then counter/timestamp колонок в DOM нет.
- Given `declined`/`agreed` с `invitedCount, decidedAt`, when таблица рендерится, then ровно 2 новые колонки; reminded-колонок нет.
- `make lint-web` и `make test-unit-web` зелёные; `campaign-notify.spec.ts` зелёный после `make start-web`.

## Verification

**Commands:**
- `cd frontend/web && npx tsc --noEmit` -- expected: без ошибок.
- `make lint-web` -- expected: проходит.
- `cd frontend/web && npm test -- --run` -- expected: все тесты зелёные, новые describe-блоки покрывают раскладку по 4 статусам.
- `make start-web && cd frontend/web && CI=true BASE_URL=http://localhost:3001 API_URL=http://localhost:8082 npx playwright test campaign-notify` -- expected: расширенный `campaign-notify.spec.ts` зелёный.

**Manual checks:**
- Открыть `/campaigns/:id` живой кампании, проверить, что новые ячейки помещаются в строку без горизонтального скролла; что в composite-ячейке видно и count, и timestamp, разделённые `·`; в `planned` — отсутствуют.

## Spec Change Log

- **2026-05-09 (post-merge UX-feedback)** — пересмотрено решение «count и timestamp — каждое в своей колонке»: на живых данных таблица не помещалась в строку (4 новых колонки в `invited` пушили contents за viewport). Слиплены пары в composite-ячейку: «Приглашение» = `{invitedCount} · {invitedAt}`, «Ремайндер» = `{remindedCount} · {remindedAt}`, «Решение» = `{decidedAt}` (без count). data-testid'ы count и at сохранены на inner-span'ах — e2e и unit-asserts не ломаются по селекторам, обновляются только assertion'ы по headers и количеству DOM-ячеек. Decided/agreed теперь дополнительно показывают `invitedAt` (раньше был n/a в матрице).

## Suggested Review Order

**Counter column wiring**

- Дизайн-точка: status-проп управляет составом extra-колонок, вставленных между city и createdAt.
  [`CampaignCreatorsTable.tsx:256`](../../frontend/web/src/features/campaigns/creators/CampaignCreatorsTable.tsx#L256)

- Контекстная раскладка: planned пусто, invited — 4 колонки, declined/agreed — 2; никаких сюрпризов для unknown status.
  [`CampaignCreatorsTable.tsx:304`](../../frontend/web/src/features/campaigns/creators/CampaignCreatorsTable.tsx#L304)

- Counter-фабрика: число рендерится напрямую (включая `0`), data-testid привязан к creatorId.
  [`CampaignCreatorsTable.tsx:349`](../../frontend/web/src/features/campaigns/creators/CampaignCreatorsTable.tsx#L349)

- Timestamp-фабрика: делегирует formatDateTimeShort, em-dash для null/невалидного.
  [`CampaignCreatorsTable.tsx:370`](../../frontend/web/src/features/campaigns/creators/CampaignCreatorsTable.tsx#L370)

- Required prop status поднялся в публичный API — компонент больше не угадывает контекст.
  [`CampaignCreatorsTable.tsx:22`](../../frontend/web/src/features/campaigns/creators/CampaignCreatorsTable.tsx#L22)

**Shared util + i18n**

- formatDateTimeShort: `null|undefined|""|невалид → "—"`, иначе ru-локаль без года.
  [`formatDateTime.ts:1`](../../frontend/web/src/shared/utils/formatDateTime.ts#L1)

- Новые ключи в неймспейсе `campaignCreators.columns.*` — JSX без литералов сохранён.
  [`campaigns.json:88`](../../frontend/web/src/shared/i18n/locales/ru/campaigns.json#L88)

**Group-section forwarding**

- GroupSection пробрасывает свой status в Table — единственная точка координации UI и контракта.
  [`CampaignCreatorGroupSection.tsx:192`](../../frontend/web/src/features/campaigns/creators/CampaignCreatorGroupSection.tsx#L192)

**Tests (unit)**

- 5 кейсов: null/undefined/""/невалид/valid ISO для util.
  [`formatDateTime.test.ts:1`](../../frontend/web/src/shared/utils/formatDateTime.test.ts#L1)

- 4 раскладки по статусам + invalid ISO — фиксируем матрицу из спеки.
  [`CampaignCreatorsTable.test.tsx:492`](../../frontend/web/src/features/campaigns/creators/CampaignCreatorsTable.test.tsx#L492)

- Spot-тест: GroupSection реально форвардит status в Table.
  [`CampaignCreatorGroupSection.test.tsx:694`](../../frontend/web/src/features/campaigns/creators/CampaignCreatorGroupSection.test.tsx#L694)

**Tests (e2e)**

- После notify: invited-count=1, invited-at не em-dash; reminded стартует с 0/em-dash.
  [`campaign-notify.spec.ts:152`](../../frontend/e2e/web/campaign-notify.spec.ts#L152)

- Новый тест remind: API-pre-flip → UI-клик → reminded-* у A заполнились, у B остались нулями.
  [`campaign-notify.spec.ts:183`](../../frontend/e2e/web/campaign-notify.spec.ts#L183)

- Header-JSDoc описывает counter-инвариант нарративом, без нумерованных шагов.
  [`campaign-notify.spec.ts:15`](../../frontend/e2e/web/campaign-notify.spec.ts#L15)
