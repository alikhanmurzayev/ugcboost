---
title: 'Все статусы креаторов кампании показываются всегда'
type: feature
created: '2026-05-10'
status: done
baseline_commit: 0399daad64b7e9b73283c80ff6c3f559c8dee376
context:
  - docs/standards/frontend-components.md
  - docs/standards/frontend-testing-unit.md
  - docs/standards/frontend-testing-e2e.md
---

<frozen-after-approval reason="human-owned intent — do not modify unless human renegotiates">

## Intent

**Problem:** На странице кампании секции креаторов по статусам (`planned, invited, declined, agreed, signing, signed, signing_declined`) скрываются когда в них нет рядов — пользователь не видит, какие статусы вообще существуют. При полностью пустой кампании показывается единый текст «Креаторов в кампании пока нет» (`campaign-creators-empty-all`).

**Approach:** Всегда рендерить все 7 секций (`CAMPAIGN_CREATOR_GROUP_ORDER`) независимо от наличия рядов. Внутри пустой секции — empty-state текст «Нет креаторов» вместо таблицы. Состояние «empty-all» больше не нужно.

## Boundaries & Constraints

**Always:**
- Все 7 групп присутствуют в DOM при любом количестве креаторов (включая 0)
- Заголовок секции с counter `0` и Add-кнопка работают как раньше
- Result-блоки (success/validation/network/contract_template_required) сохраняют поведение
- Empty-state — простой `<p>` с `data-testid="campaign-creators-group-empty-{status}"` и текстом из i18n
- Loading/Error состояния не показывают группы (как сейчас)

**Never:**
- Изменять API/бэкенд/OpenAPI
- Менять состав статусов или порядок (`CAMPAIGN_CREATOR_GROUP_ORDER`)
- Менять заголовки групп / переводы статусов
- Менять поведение `isDeleted` (секция не рендерится)

## I/O & Edge-Case Matrix

| Сценарий | Состояние | Ожидаемое поведение |
|---|---|---|
| Кампания без креаторов | `total=0`, нет result | 7 групп видны, в каждой `empty-{status}`; `empty-all` отсутствует |
| Креаторы только в одном статусе | 3 в `invited`, 0 остальных | `invited` — таблица; 6 групп — empty-state |
| Группа опустела после успеха | `rows=[]`, есть `result` | заголовок + result-блок видны; таблица не рендерится; empty-state виден |
| `isLoading=true` | — | Spinner; ни одна группа не рендерится |
| `isError=true` | — | ErrorState с retry; ни одна группа не рендерится |

</frozen-after-approval>

## Code Map

- `frontend/web/src/features/campaigns/creators/CampaignCreatorsSection.tsx` -- удалить `emptyAll` branch и guard `return null` на пустые группы
- `frontend/web/src/features/campaigns/creators/CampaignCreatorGroupSection.tsx` -- добавить empty-state для `rows.length === 0`
- `frontend/web/src/shared/i18n/locales/ru/campaigns.json` -- удалить `emptyAll`, добавить `emptyGroup`
- `frontend/web/src/features/campaigns/creators/CampaignCreatorsSection.test.tsx` -- переписать 5 ассертов на `empty-all` под новое поведение (строки 152, 171, 179–185, 447, 463)
- `frontend/web/src/features/campaigns/creators/CampaignCreatorGroupSection.test.tsx` -- добавить кейс empty-state
- `frontend/e2e/web/admin-campaign-creators-read.spec.ts` -- заменить `empty-all` (строка 183) на проверку 7 пустых групп
- `frontend/e2e/web/admin-campaign-creators-mutations.spec.ts` -- то же (строки 112, 236)
- `frontend/e2e/web/admin-campaign-creators-large.spec.ts` -- то же (строка 138)
- `frontend/e2e/web/admin-campaign-creators-trustme.spec.ts` -- ассерты `signing toHaveCount(0)` после перехода в signed/signing_declined заменить на «row отсутствует внутри signing-группы + empty-signing виден» (две точки)

## Tasks & Acceptance

**Execution:**
- [x] `CampaignCreatorsSection.tsx` -- убрать `total === 0 && !hasAnyResult(...)` ветку (245–251) и guard `if (groupRows.length === 0 && !result) return null;` (258); рендерить `CAMPAIGN_CREATOR_GROUP_ORDER.map(...)` напрямую после `isLoading`/`isError`; удалить функцию `hasAnyResult`
- [x] `CampaignCreatorGroupSection.tsx` -- заменить `{rows.length > 0 && <CampaignCreatorsTable .../>}` на тернарник: при пустых рядах — `<p className="mt-4 text-sm text-gray-400" data-testid={`campaign-creators-group-empty-${status}`}>{t("campaignCreators.emptyGroup")}</p>`
- [x] `campaigns.json` (ru) -- удалить ключ `emptyAll`; добавить `"emptyGroup": "Нет креаторов"`
- [x] `CampaignCreatorsSection.test.tsx` -- удалить ассерты `empty-all` в тестах loading/error (152, 171); кейс «empty state when total=0» (179) переименовать и переписать: проверять что все 7 `campaign-creators-group-{status}` видны и содержат `campaign-creators-group-empty-{status}`, Add-кнопка enabled, counter не виден; в drawer-кейсах (447, 463) заменить ожидание `empty-all` на ожидание любой `campaign-creators-group-{status}`
- [x] `CampaignCreatorGroupSection.test.tsx` -- добавить `it("renders empty placeholder when rows is empty")`: `rows=[]`, проверка `campaign-creators-group-empty-{status}` присутствует, `CampaignCreatorsTable` не рендерится; учесть кейс с активным `result` отдельно (заголовок + result + empty-state)
- [x] `admin-campaign-creators-read.spec.ts` -- кейс пустой кампании: вместо `getByTestId("campaign-creators-empty-all")` проверить видимость всех 7 `campaign-creators-group-{status}` и `campaign-creators-group-empty-{status}` внутри
- [x] `admin-campaign-creators-mutations.spec.ts` -- те же замены для двух кейсов (после действия становится пусто)
- [x] `admin-campaign-creators-large.spec.ts` -- та же замена

**Acceptance Criteria:**
- Given кампания без креаторов, when рендерится `CampaignCreatorsSection`, then все 7 `campaign-creators-group-{status}` видны и каждый содержит `campaign-creators-group-empty-{status}`; `campaign-creators-empty-all` отсутствует в DOM
- Given креаторы только в `invited`, when рендерится секция, then `invited` показывает таблицу, остальные 6 показывают empty-state
- Given группа с активным `result` и пустыми рядами, when рендерится секция, then result-блок виден, таблица не рендерится, empty-state виден
- Given `isLoading=true` или `isError=true`, when рендерится секция, then ни одна группа не рендерится
- `make lint-web` зелёный (нет неиспользуемого `hasAnyResult`, `emptyAll` ключ удалён)
- `make test-unit-web` зелёный
- `make test-e2e-frontend` зелёный

## Verification

**Commands:**
- `cd frontend/web && npm test -- --run src/features/campaigns/creators/` -- expected: все тесты `CampaignCreators*` зелёные
- `cd frontend/web && npx tsc --noEmit` -- expected: 0 ошибок
- `cd frontend/web && npx eslint src/` -- expected: 0 ошибок
- `make test-e2e-frontend` -- expected: все 4 затронутых spec-файла проходят

**Manual checks:**
- Открыть `/campaigns/{id}` для кампании без креаторов → видны 7 секций с counter `0` и «Нет креаторов»
- Добавить 1 креатора → одна группа показывает таблицу, остальные 6 пустые

## Suggested Review Order

**Поведение секции**

- Главная развязка: после loading/error безусловно мапим все 7 статусов, без empty-all и без guard на пустые группы.
  `frontend/web/src/features/campaigns/creators/CampaignCreatorsSection.tsx:246`

- Внутри группы пустые `rows` теперь рендерят `<p>Нет креаторов</p>` с `data-testid=campaign-creators-group-empty-{status}` рядом с уже существующим result-блоком.
  `frontend/web/src/features/campaigns/creators/CampaignCreatorGroupSection.tsx:207`

**i18n**

- `emptyAll` удалён, добавлен `emptyGroup`.
  `frontend/web/src/shared/i18n/locales/ru/campaigns.json:78`

**Unit-тесты**

- Кейс пустой кампании: все 7 групп присутствуют, в каждой empty-placeholder; `empty-all` отсутствует, counter тоже.
  `frontend/web/src/features/campaigns/creators/CampaignCreatorsSection.test.tsx:198`

- Loading/error: ни одна группа не примонтирована, после retry группы появляются.
  `frontend/web/src/features/campaigns/creators/CampaignCreatorsSection.test.tsx:152`

- Частично заполненные группы: в `planned/agreed` — таблицы, в остальных 5 — empty-placeholder.
  `frontend/web/src/features/campaigns/creators/CampaignCreatorsSection.test.tsx:344`

- Новый describe «empty placeholder»: проверяет факт empty-state и его исчезновение при наличии row; кейс result+empty совмещён.
  `frontend/web/src/features/campaigns/creators/CampaignCreatorGroupSection.test.tsx:740`

**E2E**

- Read-spec: пустая кампания → все 7 групп видны и каждая показывает `Нет креаторов`.
  `frontend/e2e/web/admin-campaign-creators-read.spec.ts:197`

- Mutations-spec: после удаления последнего креатора — fallback на 7 empty-placeholders + counter исчезает.
  `frontend/e2e/web/admin-campaign-creators-mutations.spec.ts:248`

- Large-spec: стартовый ассерт пустой кампании теперь смотрит на `empty-planned`, не на `empty-all`.
  `frontend/e2e/web/admin-campaign-creators-large.spec.ts:138`

- TrustMe-spec: после перехода `signing → signed/declined` группа `signing` остаётся в DOM, но row исчез — assert через empty-signing.
  `frontend/e2e/web/admin-campaign-creators-trustme.spec.ts:163`
