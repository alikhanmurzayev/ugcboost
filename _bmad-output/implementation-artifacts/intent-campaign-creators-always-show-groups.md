# Intent: показывать все статусы на странице креаторов кампании всегда

## Проблема

На странице креаторов кампании секции по статусам скрываются, если в них нет записей. Пользователю непонятно, какие статусы вообще существуют — секция просто исчезает.

## Решение

Всегда рендерить все 7 секций (`CAMPAIGN_CREATOR_GROUP_ORDER`). При пустом статусе показывать empty state внутри карточки секции.

---

## Изменения

### `frontend/web/src/features/campaigns/creators/CampaignCreatorsSection.tsx`

1. Убрать branch `total === 0 && !hasAnyResult(resultsByStatus)` — больше не нужен, все 7 групп покажутся пустыми.
2. Убрать guard на строке 258: `if (groupRows.length === 0 && !result) return null;`
3. Перенести `CAMPAIGN_CREATOR_GROUP_ORDER.map(...)` из else-ветки тернарника — рендерить всегда (только после loading/error состояний).
4. Удалить функцию `hasAnyResult` — dead code после п.1.

Итоговая структура рендера секции:
```tsx
{isLoading ? (
  <Spinner ... />
) : isError ? (
  <ErrorState ... />
) : (
  CAMPAIGN_CREATOR_GROUP_ORDER.map((status) => {
    const groupRows = groupedRows[status];
    const result = resultsByStatus[status] ?? null;
    const action = actionForStatus(status, notifyMutations, t);
    const isPending = action.mutation?.isPending ?? false;
    const isSubmitting = submittingByStatus[status] ?? false;
    return (
      <CampaignCreatorGroupSection
        key={status}
        ...
      />
    );
  })
)}
```

### `frontend/web/src/features/campaigns/creators/CampaignCreatorGroupSection.tsx`

Заменить блок (строки 189–204):
```tsx
{rows.length > 0 && (
  <CampaignCreatorsTable ... />
)}
```

На условный рендер:
```tsx
{rows.length > 0 ? (
  <CampaignCreatorsTable ... />
) : (
  <p
    className="mt-4 text-sm text-gray-400"
    data-testid={`campaign-creators-group-empty-${status}`}
  >
    {t("campaignCreators.emptyGroup")}
  </p>
)}
```

### `frontend/web/src/shared/i18n/locales/ru/campaigns.json`

- Удалить ключ `"emptyAll"` (больше не используется)
- Добавить ключ `"emptyGroup": "Нет креаторов"`

---

## Тесты

### `CampaignCreatorsSection.test.tsx`

Все кейсы, проверяющие `campaign-creators-empty-all`, переписать:
- При `total=0` — проверять что все 7 `campaign-creators-group-{status}` присутствуют в DOM
- Убрать проверки на `campaign-creators-empty-all` (элемент удалён)

### `CampaignCreatorGroupSection.test.tsx`

Добавить кейс: при `rows=[]` рендерится `campaign-creators-group-empty-{status}`, таблица не рендерится.

---

### E2E тесты (Playwright)

`campaign-creators-empty-all` используется в 3 spec-файлах — нужно обновить:

- `admin-campaign-creators-read.spec.ts:183` — кейс "кампания без креаторов": заменить проверку `empty-all` на проверку что все 7 групп `campaign-creators-group-{status}` видны + каждая показывает `campaign-creators-group-empty-{status}`
- `admin-campaign-creators-mutations.spec.ts:112,236` — после удаления всех креаторов: то же самое
- `admin-campaign-creators-large.spec.ts:138` — аналогично

---

## Не затрагивается

- OpenAPI / бэкенд
- Другие компоненты
