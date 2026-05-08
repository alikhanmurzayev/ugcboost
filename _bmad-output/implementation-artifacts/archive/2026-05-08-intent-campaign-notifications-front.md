---
title: "Intent: chunk 13 — фронт кнопок рассылки приглашений и ремайндеров"
type: intent
status: draft
created: "2026-05-08"
roadmap: _bmad-output/planning-artifacts/campaign-roadmap.md
related_design: _bmad-output/planning-artifacts/design-campaign-creator-flow.md
---

# Intent: chunk 13 — фронт кнопок рассылки приглашений и ремайндеров

## Преамбула — стандарты обязательны

Перед любой строкой кода агент обязан полностью загрузить все файлы `docs/standards/` (через `/standards`).
Применимы все. Особенно: `frontend-api.md`, `frontend-components.md`, `frontend-quality.md`,
`frontend-state.md`, `frontend-testing-e2e.md`, `frontend-testing-unit.md`, `frontend-types.md`,
`naming.md`, `security.md`. Каждое правило — hard rule.

## Тезис

Chunk 13 рефакторит `CampaignCreatorsSection` (плоская таблица → 4 grouped-секции по статусу),
добавляет чекбоксы выбора получателей и кнопки «Разослать приглашение» / «Разослать ремайндер»
с inline-отображением успеха и `undelivered[]`. Подключает бэк-ручки A4/A5 (chunk 12, уже в main).

## Бэкграунд

- **chunk 12** (в main) добавил бэк-ручки `POST /campaigns/{id}/notify` (A4) и
  `POST /campaigns/{id}/remind-invitation` (A5). Типы сгенерированы в
  `frontend/web/src/api/generated/schema.ts`: `CampaignCreatorStatus`, `CampaignNotifyResult`,
  `CampaignNotifyUndelivered`, `CampaignNotifyUndeliveredReason`,
  `CampaignNotifyBatchValidationError`.
- **chunk 11** (в main) построил `CampaignCreatorsSection` с плоской `CampaignCreatorsTable` +
  add/remove flow. Существующие unit-тесты (`CampaignCreatorsSection.test.tsx`,
  `CampaignCreatorsTable.test.tsx`) переписываются под новую структуру в этом chunk'е.
- **chunk 15** (будущий) добавит столбцы `invited_count`, `reminded_count`, timestamp'ы.
  Столбец статуса в chunk 13 не добавляем — статус имплицитен из группы-заголовка.

## User-facing flow

Секция «Участники кампании» переходит от одной плоской таблицы к 4 группам в фиксированном порядке
(по жизненному циклу состояния):

| Порядок | Группа           | Кнопка в заголовке              | Чекбоксы | Action API |
|---------|------------------|---------------------------------|----------|------------|
| 1       | Запланированы    | «Разослать приглашение»         | да       | A4 notify  |
| 2       | Приглашены       | «Разослать ремайндер»           | да       | A5 remind  |
| 3       | Отказались       | «Разослать приглашение»         | да       | A4 notify  |
| 4       | Согласились      | нет                             | нет      | —          |

**Поведение:**
- Чекбоксы: все сняты по умолчанию. Чекбокс «Выбрать всех» в шапке таблицы — три состояния:
  `unchecked` / `indeterminate` (часть выбрана) / `checked` (все).
- Чекбокс-ячейка имеет `e.stopPropagation()` на `onClick` — клик по чекбоксу не открывает
  CreatorDrawer (стандартный паттерн в проекте, см. `socials`-колонка в `CampaignCreatorsTable`).
- Кнопка действия: `disabled` пока selection пустой ИЛИ `isPending` ИЛИ `isSubmitting`.
- Double-submit guard: внешний `isSubmitting` boolean (см. `frontend-state.md`),
  сбрасывается в `onSettled`.
- Selection сбрасывается в `onSettled` (не `onSuccess`) — иначе после 422 selection
  остаётся stale на rows, которые могли поменять статус в БД параллельно.
- Loading/Error всей секции — на уровне `CampaignCreatorsSection` (как сейчас): пока
  `useCampaignCreators.isLoading || isError` — группы не рендерятся.
- Пустые группы (status-bucket с 0 строк) — секция группы не рендерится.
- Пустая кампания (все 4 группы пустые после успешной загрузки) — общий empty-state
  секции «Никого ещё не добавили в эту кампанию» (заменяет текущий empty-state таблицы).

**Результат рассылки (200):**
- `delivered_count > 0, undelivered = []` → inline-блок «Доставлено N креаторам» под кнопкой,
  invalidate `campaignCreatorKeys.list(campaignId)`.
- `delivered_count >= 0, undelivered.length > 0` → inline-блок «Доставлено N. Не доставлено M:»
  + список (имя/id + reason) под кнопкой, invalidate.
- Inline-блок исчезает при следующей попытке рассылки или при размонтировании группы.

**Ошибка 422 (race на статус):**
- API вернул `CampaignNotifyBatchValidationError` (статус одного из creator_id поменялся
  между загрузкой и кликом).
- Действие: invalidate `campaignCreatorKeys.list(campaignId)` (UI пересобирается, креатор
  переедет в правильную группу), inline-error под кнопкой:
  «Один или несколько креаторов в несовместимом статусе. Список обновлён.»
- Selection сбрасывается через `onSettled`.

**Прочие ошибки (network, 500):** inline-error «Не удалось разослать. Попробуйте снова.»

## Компонентная структура

```
CampaignCreatorsSection (refactored)
  ├── Шапка: "Участники кампании (N)" | [Добавить]
  ├── Если total === 0 после загрузки:
  │   └── Общий empty-state «Никого не добавили»
  ├── Иначе: 4 × CampaignCreatorGroupSection (рендерим только non-empty группы)
  │   ├── Шапка: "Запланированы (N)" | [Разослать приглашение / disabled]
  │   ├── Inline-результат / inline-error под кнопкой
  │   └── CampaignCreatorsTable (с пропсами checkboxes)
  ├── AddCreatorsDrawer (без изменений)
  └── RemoveCreatorConfirm (без изменений)
```

### Новые файлы

**`features/campaigns/creators/CampaignCreatorGroupSection.tsx`** — новый. Принимает:
```ts
interface Props {
  status: CampaignCreatorStatus;            // из generated schema
  title: string;
  rows: CampaignCreatorRow[];
  actionLabel?: string;                     // undefined → кнопки нет (Согласились)
  onAction?: (creatorIds: string[]) => Promise<CampaignNotifyResult>;
  onRemove: (row: CampaignCreatorRow) => void;
  drawerSelectedCreatorId?: string;
  onRowClick: (row: CampaignCreatorRow) => void;
}
```
Локальный state:
- `checkedCreatorIds: Set<string>` (useState — сбрасывается в `onSettled`)
- `isSubmitting: boolean`
- `result: { kind: "success" | "validation_error" | "network_error"; data?: CampaignNotifyResult; message?: string } | null`

**`features/campaigns/creators/hooks/useCampaignNotifyMutations.ts`** — новый. Один хук на
обе ручки (одинаковый shape: `(campaignId, creatorIds[]) → CampaignNotifyResult`, одинаковая
обработка ошибок). Возвращает `{ notify: UseMutationResult, remind: UseMutationResult }`.
`onError` каждой — без сайд-эффектов (вызовы `invalidateQueries` + установка inline-error
живут в `CampaignCreatorGroupSection.onSettled`, чтобы видеть `data` vs `error`).

### Изменяемые файлы

**`features/campaigns/creators/CampaignCreatorsTable.tsx`** — расширяется опциональными пропсами:
```ts
checkedCreatorIds?: Set<string>;
onToggleOne?: (creatorId: string) => void;
onToggleAll?: () => void;       // toggle между all/none; индикатор indeterminate в header
```
Если `checkedCreatorIds` не передан — колонка чекбоксов не рендерится (Согласились-группа,
обратная совместимость для chunk 15).

**`features/campaigns/creators/CampaignCreatorsSection.tsx`** — рефакторится:
- Группирует `rows` по `campaignCreator.status` через `useMemo`.
- Рендерит общий empty-state ИЛИ 4 `CampaignCreatorGroupSection`.
- Логика remove (target/error/confirm) и AddCreatorsDrawer — остаются здесь без изменений.
- Loading/Error — на уровне секции, как сейчас.

**`api/campaignCreators.ts`** — добавить:
```ts
export type CampaignNotifyResult = components["schemas"]["CampaignNotifyResult"];
export type CampaignNotifyUndelivered = components["schemas"]["CampaignNotifyUndelivered"];
export type CampaignNotifyUndeliveredReason =
  components["schemas"]["CampaignNotifyUndeliveredReason"];
export type CampaignCreatorStatus = components["schemas"]["CampaignCreatorStatus"];

export async function notifyCampaignCreators(
  campaignId: string, creatorIds: string[],
): Promise<CampaignNotifyResult>;

export async function remindCampaignCreatorsInvitation(
  campaignId: string, creatorIds: string[],
): Promise<CampaignNotifyResult>;
```

**`shared/constants/campaignCreatorStatus.ts`** — новый. Const-объект для рантайм-итерации
(требование `frontend-types.md`: «OpenAPI enum используется в рантайме без const-объекта =
finding `major`»):
```ts
export const CAMPAIGN_CREATOR_STATUS = {
  PLANNED:  "planned",
  INVITED:  "invited",
  DECLINED: "declined",
  AGREED:   "agreed",
} as const satisfies Record<string, CampaignCreatorStatus>;

// Порядок секций в UI:
export const CAMPAIGN_CREATOR_GROUP_ORDER: CampaignCreatorStatus[] = [
  CAMPAIGN_CREATOR_STATUS.PLANNED,
  CAMPAIGN_CREATOR_STATUS.INVITED,
  CAMPAIGN_CREATOR_STATUS.DECLINED,
  CAMPAIGN_CREATOR_STATUS.AGREED,
];
```

## i18n ключи (`locales/ru/campaigns.json`)

```json
"campaignCreators": {
  "groups": {
    "planned":  "Запланированы",
    "invited":  "Приглашены",
    "declined": "Отказались",
    "agreed":   "Согласились"
  },
  "notifyButton":  "Разослать приглашение",
  "remindButton":  "Разослать ремайндер",
  "selectAll":     "Выбрать всех",
  "emptyAll":      "Никого ещё не добавили в эту кампанию",
  "result": {
    "delivered":   "Доставлено {{count}}",
    "undelivered": "Не доставлено {{count}}:",
    "validationError":
      "Один или несколько креаторов в несовместимом статусе. Список обновлён.",
    "networkError": "Не удалось разослать. Попробуйте снова."
  },
  "undeliveredReason": {
    "bot_blocked": "бот заблокирован",
    "unknown":     "неизвестная ошибка"
  }
}
```

## data-testid (контракт с Playwright)

- `campaign-creators-empty-all` — empty-state всей кампании
- `campaign-creators-group-{status}` — секция группы
- `campaign-creators-group-action-{status}` — кнопка действия
- `campaign-creators-group-result-{status}` — inline-блок результата (success/error)
- `campaign-creator-checkbox-{creatorId}` — чекбокс строки
- `campaign-creators-select-all-{status}` — чекбокс «Выбрать всех»

## Тесты

**Unit (Vitest + RTL):**
- `CampaignCreatorGroupSection`:
  - disabled кнопка при пустом selection;
  - enabled после выбора одного;
  - selection и `isSubmitting` сбрасываются в `onSettled` (как при success, так и при error);
  - inline-success при `undelivered = []`;
  - inline-undelivered при `undelivered.length > 0`;
  - inline-validation-error при API 422;
  - inline-network-error при network failure;
  - чекбокс не открывает drawer (stopPropagation проверяется через `onRowClick` mock).
- `CampaignCreatorsTable` с чекбоксами: toggle one, toggle all (none → all), toggle all (all → none),
  indeterminate state при части выбранных, чекбоксы не рендерятся когда `checkedCreatorIds` undefined.
- `useCampaignNotifyMutations`: mock `notifyCampaignCreators` → правильный endpoint + `creatorIds`;
  то же для `remindCampaignCreatorsInvitation`.
- `CampaignCreatorsSection` (рефакторинг): группировка rows по статусам, empty-state всей кампании
  при `total = 0`, скрытие пустых групп при non-empty других, loading/error на уровне секции.
- `shared/constants/campaignCreatorStatus.ts`: проверка соответствия const-объекта типу.

**Playwright E2E (`frontend/e2e/web/campaign-notify.spec.ts`):**
- beforeAll через бизнес-ручки (стандарт `frontend-testing-e2e.md`): создание кампании +
  approve креаторов + добавление их в кампанию (3 в planned).
- Открыть страницу кампании → группа «Запланированы» видна, кнопка `disabled`.
- Поставить 2 чекбокса → кнопка активна, индикатор select-all = `indeterminate`.
- Кликнуть «Разослать приглашение» → inline-success «Доставлено 2».
- Перерендер: 2 строки переехали в группу «Приглашены».
- Partial-success: использовать `POST /test/telegram/spy/fail-next` (есть в `openapi-test.yaml`)
  → fail на одного → inline-блок с undelivered{name, reason}.
- Race-422: после открытия страницы изменить статус одного creator через бизнес-ручку,
  кликнуть рассылку → inline-validation-error + список обновлён через invalidate.

## Cohesion check

- Миграции: нет (frontend-only).
- Audit: нет (frontend-only).
- Бэкенд: не трогаем.
- chunk 15 совместимость: новые пропсы `CampaignCreatorsTable` опциональны (`undefined`
  default) — обратно совместимо. Chunk 15 расширит `buildColumns` для status-related данных
  (timestamps, counters); статус-колонку добавлять не будет — он остаётся имплицитным
  через группу.
- Remove-flow: остаётся в `CampaignCreatorsSection`, проброс `onRemove` вниз в группу.
- `agreed`-группа: чекбоксов нет; remove-кнопка рендерится но backend вернёт 422 как сейчас
  (`CAMPAIGN_CREATOR_AGREED`) → `setRemoveError` показывает inline-сообщение, как реализовано
  в текущем `CampaignCreatorsSection`.
- PII в логах: нет логирования (frontend) — `console.log` запрещён `frontend-quality.md`.
- Selection naming-разделение: `drawerSelectedCreatorId` (URL search-param для CreatorDrawer)
  vs `checkedCreatorIds` (локальный Set для рассылки) — не пересекаются.
