---
title: 'chunk 13 — фронт кнопок рассылки приглашений и ремайндеров'
type: feature
created: '2026-05-08'
status: in-review
baseline_commit: a3d9562
context:
  - docs/standards/frontend-api.md
  - docs/standards/frontend-state.md
  - docs/standards/frontend-types.md
---

## Преамбула

Перед реализацией агент полностью загружает `docs/standards/`. Каждое правило — hard rule.

<frozen-after-approval reason="human-owned intent — do not modify unless human renegotiates">

## Intent

**Проблема:** chunk 12 поднял ручки `POST /campaigns/{id}/notify` и
`POST /campaigns/{id}/remind-invitation`, но фронт не умеет их вызывать.
Админ не может разослать приглашения / ремайндеры из UI.

**Подход:** рефакторим `CampaignCreatorsSection` из плоской таблицы в 4 секции по
`status` (порядок: `planned → invited → declined → agreed`). В каждой секции —
заголовок с count + кнопка действия (для `agreed` кнопки нет), таблица с
чекбоксами строк и select-all (3 состояния), inline-блок результата под кнопкой.
Кнопка вызывает A4/A5; на 200 — inline-success + invalidate, на 422
`CAMPAIGN_CREATOR_BATCH_INVALID` — inline-validation-error + invalidate, на
network/5xx — generic inline-error. Selection и `isSubmitting` сбрасываются в
`onSettled` (всегда).

## Boundaries

**Always:**
- Типы — только из `api/generated/schema.ts`. Const-объект `CAMPAIGN_CREATOR_STATUS`
  для рантайм-итерации (`frontend-types.md`).
- `useMutation` имеет `onError`. Кнопка `disabled` пока selection пуст ИЛИ
  `isPending` ИЛИ внешний `isSubmitting` (double-submit guard, `frontend-state.md`).
- Selection / `isSubmitting` сбрасываются в `onSettled`, не `onSuccess`.
- Чекбокс-ячейка вызывает `e.stopPropagation()` на `onClick`/`onKeyDown` —
  паттерн `socials`-колонки в существующем `CampaignCreatorsTable`.
- UI-строки через `react-i18next`. `data-testid` фиксированы (см. Design Notes).
- `CampaignNotifyResult.data` имеет только `undelivered[]` —
  `delivered_count = creatorIds.length - undelivered.length` вычисляется на фронте.

**Ask First:**
- Если потребуется расширять общий `Table.tsx` (а не локализовать чекбокс
  колонкой) — HALT.

**Never:**
- Toast / global notifications — нет инфраструктуры.
- Сетевая логика в компоненте — только через хук `useCampaignNotifyMutations`.
- Изменения бэка / OpenAPI / миграций / audit.
- Status-колонка / counters / timestamps в таблице — это chunk 15.

## I/O & Edge-Case Matrix

| Сценарий | Состояние | Поведение |
|---|---|---|
| Loading / Error | `useCampaignCreators.isLoading/isError` | Spinner / ErrorState на уровне секции (как сейчас); 4 группы не рендерятся |
| Пустая кампания | `total === 0` | Один общий `campaign-creators-empty-all`; группы не рендерятся |
| Пустая группа | `rows[status].length === 0`, total > 0 | Секция группы не рендерится |
| Selection пуст | `size === 0` | Кнопка disabled |
| Selection частично | `0 < size < group.length` | select-all → `indeterminate`, кнопка enabled |
| Selection полный | `size === group.length` | select-all → `checked`, кнопка enabled |
| 200 без undelivered | `data.undelivered = []` | inline «Доставлено N»; invalidate; selection сброс |
| 200 partial | `undelivered.length > 0` | inline «Доставлено N. Не доставлено M:» + список (имя или fallback + reason); invalidate; selection сброс |
| 422 batch invalid | `code === "CAMPAIGN_CREATOR_BATCH_INVALID"` | inline «несовместимый статус. Список обновлён.»; invalidate; selection сброс |
| Network / 5xx | network error / status >= 500 | inline «Не удалось разослать. Попробуйте снова.»; selection сброс |
| Клик по чекбоксу | строка с `onRowClick` | Только toggle, drawer не открывается |

</frozen-after-approval>

## Code Map

- `api/generated/schema.ts` — источник типов: `CampaignCreatorStatus`,
  `CampaignNotifyResult`, `CampaignNotifyUndelivered`,
  `CampaignNotifyUndeliveredReason`, `CampaignCreatorBatchInvalidError`.
- `api/campaignCreators.ts` (+ `.test.ts`) — добавить две обёртки + ре-экспорт типов.
- `shared/constants/campaignCreatorStatus.ts` (+ `.test.ts`) — НОВЫЙ. Const-объект
  `CAMPAIGN_CREATOR_STATUS` + `CAMPAIGN_CREATOR_GROUP_ORDER`.
- `features/campaigns/creators/hooks/useCampaignNotifyMutations.ts`
  (+ `.test.tsx`) — НОВЫЙ. Один хук на обе ручки (одинаковый shape).
- `features/campaigns/creators/CampaignCreatorGroupSection.tsx`
  (+ `.test.tsx`) — НОВЫЙ. Один блок для одной status-группы.
- `features/campaigns/creators/CampaignCreatorsTable.tsx` (+ `.test.tsx`) —
  расширить опциональными пропсами для чекбоксов; обновить тесты.
- `features/campaigns/creators/CampaignCreatorsSection.tsx` (+ `.test.tsx`) —
  рефакторить из плоской в group-based; переписать тесты.
- `shared/i18n/locales/ru/campaigns.json` — добавить ключи (см. Design Notes).
- `frontend/e2e/web/campaign-notify.spec.ts` — НОВЫЙ Playwright spec.

## Tasks & Acceptance

**Execution:**

- [x] `shared/constants/campaignCreatorStatus.ts` — `CAMPAIGN_CREATOR_STATUS = {
  PLANNED: "planned", INVITED: "invited", DECLINED: "declined", AGREED: "agreed"
  } as const satisfies Record<string, CampaignCreatorStatus>`. Массив
  `CAMPAIGN_CREATOR_GROUP_ORDER` с тем же порядком.
- [x] `api/campaignCreators.ts` — `notifyCampaignCreators(campaignId, creatorIds):
  Promise<CampaignNotifyResult>` и `remindCampaignCreatorsInvitation(...)` через
  `client.POST(path, { params: { path: { id } }, body: { creatorIds } })`.
  Ошибки извлекать как в `addCampaignCreators` (через `extractErrorParts`),
  пробрасывать `ApiError` с `code` и `details` (нужно расширить `extractErrorParts`,
  чтобы вытаскивать `details` для 422).
- [x] `features/campaigns/creators/hooks/useCampaignNotifyMutations.ts` —
  `function useCampaignNotifyMutations(campaignId)` возвращает
  `{ notify: UseMutationResult<CampaignNotifyResult, ApiError, string[]>, remind: ... }`.
  `mutationFn` вызывает соответствующую обёртку. `onError` пустой — обработка
  результата живёт в `onSettled` компонента (нужно видеть `data` vs `error`).
- [x] `features/campaigns/creators/CampaignCreatorsTable.tsx` — добавить пропсы
  `checkedCreatorIds?: Set<string>`, `onToggleOne?: (id: string) => void`,
  `onToggleAll?: () => void`, `selectAllState?: "unchecked" | "indeterminate" | "checked"`.
  Если `checkedCreatorIds` не передан — колонка чекбоксов не рендерится.
  Header чекбокса — через `<input ref>` с `useEffect`-устанавливающим
  `el.indeterminate = state === "indeterminate"` (HTML-атрибут не через React-prop).
  Чекбокс ячейки — `<input type="checkbox">` обёрнутый в `<div onClick=stopPropagation>`.
- [x] `features/campaigns/creators/CampaignCreatorGroupSection.tsx` — props:
  `status, title, rows, actionLabel?, mutation?, onRemove,
  drawerSelectedCreatorId?, onRowClick`. Локально:
  `checkedCreatorIds`, `isSubmitting`, `result: { kind: "success" | "validation_error"
  | "network_error"; undelivered?; rows? } | null`. На submit: вычислить selection,
  set `isSubmitting=true`, `mutation.mutate(creatorIds, { onSettled: (data, error) =>
  { invalidateQueries; setCheckedCreatorIds(empty); setIsSubmitting(false);
  setResult(parseFromDataOrError) } })`. `result` ре-сетится при следующем submit
  (или при unmount).
- [x] `features/campaigns/creators/CampaignCreatorsSection.tsx` — `useMemo`
  группировка `rows` по `campaignCreator.status`. Если `total === 0` после
  загрузки — рендерить `campaign-creators-empty-all`. Иначе — итерировать
  `CAMPAIGN_CREATOR_GROUP_ORDER`, для каждого статуса с `rows.length > 0`
  рендерить `CampaignCreatorGroupSection`. `useCampaignNotifyMutations(campaignId)`
  вызывается здесь, мутации передаются вниз. Loading/Error на уровне секции — как сейчас.
- [x] `shared/i18n/locales/ru/campaigns.json` — расширить ветку `campaignCreators`
  ключами `groups.{planned,invited,declined,agreed}`, `notifyButton`, `remindButton`,
  `selectAll`, `emptyAll`, `result.{delivered,undelivered,validationError,networkError}`,
  `undeliveredReason.{bot_blocked,unknown}`.
- [x] **Все *.test.* файлы выше** — обновить / создать. Покрытие по сценариям из
  I/O Matrix + Acceptance Criteria. Для существующих `CampaignCreatorsSection.test.tsx`
  и `CampaignCreatorsTable.test.tsx` — переписать под новую структуру (group-based).
- [x] `frontend/e2e/web/campaign-notify.spec.ts` — Playwright spec, JSDoc-narrative
  на русском. beforeEach через бизнес-ручки: создать кампанию, approve 3-х
  креаторов, добавить их в кампанию (planned). Тесты:
  (1) кнопка disabled → отметить 2 → enabled → клик → inline-success + 2 строки
  переехали в группу `invited`;
  (2) partial-success через `POST /test/telegram/spy/fail-next` (есть в
  `openapi-test.yaml`) → inline-undelivered с именем;
  (3) race-422: изменить статус одного через бизнес-ручку → клик → inline-validation-error.

**Acceptance Criteria:**

- Given кампания `1×planned + 1×invited + 1×declined + 1×agreed`,
  when страница загружена, then рендерятся 4 группы в порядке
  `planned → invited → declined → agreed`; кнопка «Разослать приглашение» в
  `planned`/`declined`, «Разослать ремайндер» в `invited`, у `agreed` кнопки нет.
- Given группа с 3 строками, when админ ставит чекбокс на одной, then
  select-all = `indeterminate`, кнопка enabled.
- Given selection с 2-мя выбранными, when клик и API → 200 `undelivered=[]`,
  then под кнопкой «Доставлено 2», `campaignCreatorKeys.list(campaignId)`
  инвалидируется, selection сброшен, кнопка снова disabled.
- Given API → 422 `CAMPAIGN_CREATOR_BATCH_INVALID`, when `onSettled`, then
  inline-validation-error, invalidate, selection сброшен.
- Given кампания пустая (total=0), when страница загружена, then рендерится
  один `campaign-creators-empty-all`, 4 секции групп НЕ рендерятся.

## Design Notes

**i18n keys** (под `campaignCreators` в `campaigns.json`):
`groups.planned/invited/declined/agreed`, `notifyButton`, `remindButton`,
`selectAll`, `emptyAll`, `result.delivered/undelivered/validationError/networkError`,
`undeliveredReason.bot_blocked/unknown`.

**data-testid:**
- `campaign-creators-empty-all`
- `campaign-creators-group-{status}` (статус из `CAMPAIGN_CREATOR_STATUS` values)
- `campaign-creators-group-action-{status}`
- `campaign-creators-group-result-{status}`
- `campaign-creator-checkbox-{creatorId}`
- `campaign-creators-select-all-{status}`

**`onSettled` парсинг:** ApiError с `status === 422 && code === "CAMPAIGN_CREATOR_BATCH_INVALID"`
→ `validation_error`; `data` (200) → `success` с `undelivered`; всё остальное → `network_error`.

**Undelivered с именами:** `rows.find(r => r.campaignCreator.creatorId === id)`;
если `creator` есть — `lastName + " " + firstName`, иначе fallback из существующего
`campaignCreators.deletedPlaceholder`.

**`indeterminate`:** HTML-атрибут не через React-prop. `useEffect`/`ref-callback`
ставит `el.indeterminate` после mount/update.

## Verification

**Commands:**
- `cd frontend/web && npx tsc --noEmit` — no errors.
- `cd frontend/web && npx eslint src/` — no errors.
- `cd frontend/web && npm test -- --run` — all tests pass.
- `make test-e2e-frontend` — новый spec зелёный, существующие не сломаны.

## Suggested Review Order

**Status taxonomy & ordering**

- Const-объект для рантайма + tuple с compile-time exhaustiveness — единственный источник порядка групп.
  [`campaignCreatorStatus.ts:1`](../../frontend/web/src/shared/constants/campaignCreatorStatus.ts#L1)

**API contract: notify / remind + ApiError.details**

- Новые обёртки и реэкспорт типов — формы доставки и валидационные коды.
  [`campaignCreators.ts:106`](../../frontend/web/src/api/campaignCreators.ts#L106)

- `extractErrorParts` теперь поднимает `details` для 422 batch-invalid.
  [`campaignCreators.ts:22`](../../frontend/web/src/api/campaignCreators.ts#L22)

- `ApiError` расширен 4-м параметром `details` — обратно-совместимо.
  [`client.ts:15`](../../frontend/web/src/api/client.ts#L15)

**Mutations hook**

- Один хук, два mutation'а; `onError` пустой — обработка перенесена в onSettled компонента.
  [`useCampaignNotifyMutations.ts:14`](../../frontend/web/src/features/campaigns/creators/hooks/useCampaignNotifyMutations.ts#L14)

**Group section — главный новый компонент**

- Локальные selection / isSubmitting / result; `mutate(ids, { onSettled })` разруливает data vs error.
  [`CampaignCreatorGroupSection.tsx:38`](../../frontend/web/src/features/campaigns/creators/CampaignCreatorGroupSection.tsx#L38)

- `parseSettled` — единственная точка маппинга 200/422-batch/прочее в `result.kind`.
  [`CampaignCreatorGroupSection.tsx:128`](../../frontend/web/src/features/campaigns/creators/CampaignCreatorGroupSection.tsx#L128)

- `ResultBlock` рендерит inline-успех/ошибку; разделитель «:» и i18n fallback на `unknown`.
  [`CampaignCreatorGroupSection.tsx:163`](../../frontend/web/src/features/campaigns/creators/CampaignCreatorGroupSection.tsx#L163)

**Section refactor — рендер по статусам**

- `useMemo` группировка + итерация по `CAMPAIGN_CREATOR_GROUP_ORDER`; `total === 0` → `empty-all`.
  [`CampaignCreatorsSection.tsx:96`](../../frontend/web/src/features/campaigns/creators/CampaignCreatorsSection.tsx#L96)

- `actionForStatus` — таблица «status → label/mutation»; agreed без кнопки.
  [`CampaignCreatorsSection.tsx:228`](../../frontend/web/src/features/campaigns/creators/CampaignCreatorsSection.tsx#L228)

**Table — чекбокс-колонка**

- Опциональные пропсы; колонка не рендерится без `checkedCreatorIds`.
  [`CampaignCreatorsTable.tsx:82`](../../frontend/web/src/features/campaigns/creators/CampaignCreatorsTable.tsx#L82)

- `indeterminate` ставится через ref+useEffect — HTML property, не React prop.
  [`CampaignCreatorsTable.tsx:269`](../../frontend/web/src/features/campaigns/creators/CampaignCreatorsTable.tsx#L269)

- Чекбокс-ячейка обёрнута в div c stopPropagation на onClick/onKeyDown — паттерн socials-колонки.
  [`CampaignCreatorsTable.tsx:316`](../../frontend/web/src/features/campaigns/creators/CampaignCreatorsTable.tsx#L316)

**i18n keys**

- Ветка `campaignCreators` дополнена группами, кнопками, result-копией и причинами недоставки.
  [`campaigns.json:60`](../../frontend/web/src/shared/i18n/locales/ru/campaigns.json#L60)

**Tests — unit**

- Group section — selection/result/422/network/double-click guard/remind vs notify.
  [`CampaignCreatorGroupSection.test.tsx:1`](../../frontend/web/src/features/campaigns/creators/CampaignCreatorGroupSection.test.tsx#L1)

- Section integration — порядок групп + правильные label'ы.
  [`CampaignCreatorsSection.test.tsx:179`](../../frontend/web/src/features/campaigns/creators/CampaignCreatorsSection.test.tsx#L179)

- Table — header indeterminate, row stopPropagation, отсутствие колонки без props.
  [`CampaignCreatorsTable.test.tsx:251`](../../frontend/web/src/features/campaigns/creators/CampaignCreatorsTable.test.tsx#L251)

- Mutations hook — independent mutate, ошибки 422/500.
  [`useCampaignNotifyMutations.test.tsx:36`](../../frontend/web/src/features/campaigns/creators/hooks/useCampaignNotifyMutations.test.tsx#L36)

- API wrappers — happy / 404 / 422 with details / 5xx fallback.
  [`campaignCreators.test.ts:282`](../../frontend/web/src/api/campaignCreators.test.ts#L282)

**Tests — e2e**

- Playwright spec: happy + partial via fail-next + race-422.
  [`campaign-notify.spec.ts:1`](../../frontend/e2e/web/campaign-notify.spec.ts#L1)

- Существующие e2e обновлены под новый testid `campaign-creators-empty-all`.
  [`admin-campaign-creators-mutations.spec.ts:111`](../../frontend/e2e/web/admin-campaign-creators-mutations.spec.ts#L111)
  [`admin-campaign-creators-read.spec.ts:182`](../../frontend/e2e/web/admin-campaign-creators-read.spec.ts#L182)
  [`admin-campaign-creators-large.spec.ts:137`](../../frontend/e2e/web/admin-campaign-creators-large.spec.ts#L137)
