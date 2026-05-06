# Deferred work

Tech-debt и cross-cutting findings, обнаруженные параллельно реализации
конкретных feature'ов, но не относящиеся к их scope. Каждая запись —
кандидат в отдельный implementation chunk или в чеклист стандартов.

## 2026-05-06 — после ревью `spec-creators-list-frontend`

### Coverage gate (frontend)

- В `frontend/web/package.json` нет `@vitest/coverage-v8`/`coverage-istanbul`.
- В `vite.config.ts` секция `test` не задаёт `coverage.provider` или
  `coverage.thresholds`.
- `make test-unit-web` запускает `vitest --run` без `--coverage`.
- В `Makefile` нет фронтового аналога `test-unit-backend-coverage`.
- Spec `creators-list-frontend.md` AC «0 файлов <80%» формально не
  выполняется без инфраструктуры.

**Action:** отдельный chunk — установить provider, добавить per-file
80% threshold, исключить `types.ts`/`index.ts`/`generated/`,
завести `make test-unit-web-coverage`, прошить в CI.

### Window-level keyboard listener в drawer'ах

`features/creators/CreatorDrawer.tsx` и
`features/creatorApplications/components/ApplicationDrawer.tsx`
вешают `keydown` на `window` без проверки `e.target` /
`document.activeElement`. Когда drawer открыт И сфокусирован inputbg,
ArrowLeft/Right двигает текстовый курсор + триггерит prev/next, Escape
закрывает drawer вместо нативного UX. Pattern скопирован между фичами.

**Action:** либо scope listener на `dialogRef.current.addEventListener`,
либо в обработчике сначала `if (!dialogRef.current?.contains(e.target))
return;`. Пройти по обоим drawer'ам.

### parseIsoDate calendar validity

`features/creators/filters.ts:parseIsoDate` и одноимённый в
`creatorApplications/filters.ts` пропускают `2026-02-30`/`2026-04-31`:
`new Date(...)` тихо ролит в следующий месяц. Тесты покрывают только
`2026-13-99` (где month >12 даёт NaN).

**Action:** после `new Date()` сверять `getUTCMonth()/getUTCDate()` с
введёнными цифрами.

### Cross-field validation: ageFrom ≤ ageTo, dateFrom ≤ dateTo

UI допускает `ageFrom=80, ageTo=20` без warning'а — backend вернёт пустой
список и пользователь не понимает почему. То же для dateFrom > dateTo.

**Action:** UI-валидация на блюре поля ИЛИ swap при submit.

### Numeric URL params без upper-bound (общий чеклист-кандидат)

`parsePage` и `parsePositiveInt` теперь имеют clamp в creators/, но
аналогичный pattern в `creatorApplications/filters.ts` остался без
ограничений. `?page=1e10` отдаёт backend'у большой OFFSET.

**Action:** добавить clamp в `creatorApplications/filters.ts`. Завести
правило в `frontend-quality.md`: «URL-derived numeric pagination/filter
inputs обязаны иметь upper-bound на клиенте».

### Date filters TZ-correctness

`toListInput` строит `${dateFrom}T00:00:00.000Z` — UTC midnight. На
admin'е в Алматы (UTC+5) это 05:00 локально, а не «начало дня
пользователя». Креаторы одобренные между 19:00 предыдущего UTC-дня и
00:00 UTC выпадают из фильтра «с такого-то дня».

**Action:** или конвертировать `<date>T00:00:00` через локальный
Date(...).toISOString(), или согласовать с бэком явный TZ-параметр.

### Orphan drawer state edges

После filter/page change может остаться `?id=<old>` в URL, хотя строка
больше не в `items`. `idx === -1` → `prefill === undefined`,
`canPrev/canNext === false`, drawer показывает только `detail` если он
успел резолвиться. Reset/filter/changePage не делают `np.delete("id")`.

**Action:** в `changePage` и `reset` добавить `np.delete("id")` либо
показывать в drawer'е сообщение «креатор не входит в текущий фильтр».

### `as` type-assertions без guard'ов

`creators/sort.ts` и `creators/CreatorFilters.tsx` используют `as
ApiSortField` / `as Node`. Pattern скопирован из
`creatorApplications/sort.ts`. ESLint config не имеет
`@typescript-eslint/consistent-type-assertions: never` — стандарт
`frontend-quality.md` декларативный.

**Action:** включить ESLint правило, рефакторить на type guards в обоих
sort.ts.

### listQuery без `placeholderData: keepPreviousData`

`CreatorsListPage`, `ModerationPage`, `VerificationPage` — каждая смена
страницы/sort/filter обнуляет таблицу до спиннера. UX-регрессия для
любого list-screen'а.

**Action:** добавить `placeholderData: keepPreviousData` всем list
queries; завести правило в `frontend-state.md`.

### `navigator.clipboard` fallback на insecure context

Copy-кнопки в `CreatorDrawerBody` (и аналогичный bot-message-copy в
`ApplicationDrawer`) silent-catch'ат ошибки `writeText`. На HTTP staging
без TLS (или в Safari Private mode) кнопка молча не работает.

**Action:** при отсутствии `navigator.clipboard?.writeText` показывать
fallback (выделение текста / toast «не удалось»). Завести правило в
`frontend-components.md`.

### Coverage gaps и UX-инварианты для drawer'а

- `page > totalPages` — пользователь застрял на пустой странице, нет
  авто-clamp'а к totalPages после загрузки.
- `?sort=updated_at` валиден, но без UI колонки → нет подсветки.
- Whitespace-only `q=   ` в URL проходит, хотя toListInput его trim'ит.
- `<input type="number">` для ageFrom/ageTo не предупреждает о
  not-a-number.

**Action:** объединить в небольшой UX-cleanup chunk вместе с orphan
drawer.
