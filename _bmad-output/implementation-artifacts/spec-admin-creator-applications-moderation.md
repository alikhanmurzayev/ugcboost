---
title: 'Админка-фронт: экран списка заявок на модерации + drawer'
type: feature
created: '2026-05-04'
status: done
baseline_commit: '484f650'
context:
  - docs/standards/frontend-components.md
  - docs/standards/frontend-state.md
  - docs/standards/frontend-testing-unit.md
---

<frozen-after-approval>

## Intent

**Problem:** После auto/manual-верификации заявка переходит в `moderation` и пропадает с экрана `verification` (chunk 6). Сейчас в админке маршрут `/creator-applications/moderation` указан на stub — модератор не видит ни список, ни бейдж в sidebar, ни drawer для принятия решения. Backend полностью готов (chunks 4/5/7/12), фронт — нет.

**Approach:** Клонировать `VerificationPage.tsx` под `moderation`, переиспустив все существующие компоненты (`ApplicationsTable`, `ApplicationDrawer`, `ApplicationFilters`, `HoursBadge`, `SocialAdminRow`, `SocialLink`). Заменить stub в роутере на реальную страницу. Добавить бейдж модерации в `DashboardLayout` рядом с уже работающим бейджем верификации. Reject-кнопку в drawer переиспустить из chunk 14 по тому же паттерну, какой будет применён там для verification.

## Boundaries & Constraints

**Always:**
- Переиспустить существующие компоненты `ApplicationsTable` / `ApplicationDrawer` / `ApplicationFilters` / `SocialAdminRow` / `SocialLink` / `HoursBadge` / `CategoryChip` без модификации их публичного API.
- Дефолтный sort на ModerationPage = `updated_at asc` (старейшие сверху — кого дольше всех заставили ждать с момента входа в стейдж).
- Колонка `hoursInStage` на ModerationPage считается от `updatedAt` (на VerificationPage остаётся от `createdAt`).
- Бейдж в sidebar для moderation использует тот же counts-handler и тот же UI-паттерн, что и для verification.
- Production-код пишется **без объяснительных комментариев** — стандарт `naming.md` § Комментарии. Только WHY, и только когда неочевидно. Заголовок e2e-файла (chunk 16.5) — отдельная история, к этой спеке не относится.

**Ask First:**
- Если параллельный chunk 14 встроил reject-кнопку **внутрь** `ApplicationDrawer` (вариант A из обсуждения), а не через children/slot prop — HALT и спросить, оставлять ли так, или рефакторить под slot-паттерн ради симметрии с moderation.

**Never:**
- Добавлять колонки `city` / `age` / `qualityIndicator` в таблицу (прототип Айданы их содержал, мы решили нет — Q3).
- Textarea для комментария в reject-модалке (бэк chunk 12 принимает пустое body, текст для креатора захардкожен в chunk 13).
- Approve-кнопка в любом виде (откладывается в chunk 19).
- E2E Playwright-тесты (отдельный chunk 16.5 после стабилизации UI).
- Любые правки backend (chunk 15 закрыт как N/A).
- Удалять прототип в `frontend/web/src/_prototype/` — он остаётся как референс для будущих экранов.

## I/O & Edge-Case Matrix

| Scenario | Input / State | Expected Output / Behavior | Error Handling |
|----------|--------------|---------------------------|----------------|
| Happy: открытие страницы | admin аутентифицирован, есть заявки в `moderation` | Запрос `POST /creators/applications/list` с `statuses=["moderation"]`, `sort=updated_at`, `order=asc`. Таблица + total в заголовке. | N/A |
| URL deep-link на заявку | `?id=<uuid>` | Drawer открыт сразу с detail-данными, prev/next активны по позиции в текущей странице | Если detail вернул 404 — drawer показывает ErrorState |
| URL deep-link на sort | `?sort=full_name&order=asc` | Таблица отсортирована соответственно, дефолтные значения из URL не пишутся обратно | Невалидный sort/order → silently fallback на дефолт `updated_at asc` |
| Counts-handler вернул ошибку | `getCreatorApplicationsCounts` failed | Бейдж модерации скрыт (как уже сделано для verification — `verificationCount = undefined`) | N/A |
| Список пустой (нет фильтров) | `items.length === 0` и фильтры пустые | `t("empty")` | N/A |
| Список пустой (под фильтром) | `items.length === 0` и `isFilterActive(filters)` | `t("emptyFiltered")` | N/A |
| Не-admin зашёл по URL | role ≠ admin | RoleGuard уже редиректит на dashboard (как для verification — без изменений) | N/A |
| List API ошибка | network/500 | `ErrorState` с кнопкой retry, `listQuery.refetch` | N/A |

</frozen-after-approval>

## Code Map

- `frontend/web/src/features/creatorApplications/VerificationPage.tsx` — паттерн-исходник для клонирования (структура, query-keys, filter-logic, pagination, sort-state в URL).
- `frontend/web/src/features/creatorApplications/stubs/ModerationPage.tsx` — текущая заглушка, удаляется.
- `frontend/web/src/features/creatorApplications/sort.ts` — содержит глобальный `DEFAULT_SORT` и `COLUMN_TO_FIELD` (где `hoursInStage → created_at`). Требует параметризации, чтобы оба экрана могли иметь свои дефолты и column→field-маппинги, не ломая друг друга.
- `frontend/web/src/features/creatorApplications/components/SocialAdminRow.tsx` — уже рендерит per-social `method` (auto/manual) + `verifiedAt`. Используется drawer'ом обоих экранов без изменений.
- `frontend/web/src/App.tsx` — строка 13 (импорт stub) и строка 73 (route element). Меняется импорт.
- `frontend/web/src/shared/layouts/DashboardLayout.tsx` — строки 44–47 (вычисление `verificationCount`) и строка 70 (moderation menu item без бейджа). Добавляется `moderationCount` и пробрасывается в badge.
- `frontend/web/src/shared/i18n/locales/ru/creatorApplications.json` — `stages.moderation` имеет только `title`, нужно добавить `description`.
- `frontend/web/src/features/creatorApplications/VerificationPage.test.tsx` — паттерн для теста ModerationPage.
- `frontend/web/src/shared/layouts/DashboardLayout.test.tsx` — расширяется кейсом «бейдж модерации».

## Tasks & Acceptance

**Execution:**
- [x] `frontend/web/src/features/creatorApplications/sort.ts` -- параметризовать: `parseSortFromUrl(sp, defaults?)`, `serializeSort(sp, state, defaults?)`, `fieldForColumn(key, map?)`. При отсутствии параметров поведение не меняется. -- Чтобы ModerationPage мог иметь дефолт `updated_at asc` и `hoursInStage → updated_at`, не ломая VerificationPage.
- [x] `frontend/web/src/features/creatorApplications/ModerationPage.tsx` -- создать. Клон `VerificationPage.tsx`: `STATUSES = ["moderation"]`, локальный `DEFAULT_SORT = { sort: "updated_at", order: "asc" }`, локальный column→field map с `hoursInStage → updated_at`, `HoursBadge` принимает `hoursSince(row.updatedAt)`. Колонки идентичны VerificationPage. testid'ы: `creator-applications-moderation-page` (контейнер), `moderation-total` (счётчик в h1). i18n: `stages.moderation.title` / `description`. Drawer без особых children — reject-кнопку подключить тем же способом, каким она подключена в VerificationPage по итогу chunk 14. -- Основной деливерэбл.
- [x] `frontend/web/src/App.tsx` -- заменить импорт `ModerationPage` на `@/features/creatorApplications/ModerationPage` (без `/stubs/`). -- Активация реальной страницы.
- [x] `frontend/web/src/features/creatorApplications/stubs/ModerationPage.tsx` -- удалить файл. -- Stub больше не нужен, не оставляем мёртвый код.
- [x] `frontend/web/src/shared/layouts/DashboardLayout.tsx` -- по аналогии с `verificationCount` добавить `moderationCount = countsQuery.isError ? undefined : (counts?.find((c) => c.status === "moderation")?.count ?? 0)`. Передать как `badge: moderationCount` в moderation menu item. -- Бейдж в sidebar.
- [x] `frontend/web/src/shared/i18n/locales/ru/creatorApplications.json` -- добавить `stages.moderation.description` (формулировка по смыслу: «Модератор проверяет соцсети и принимает решение по заявке.»). -- i18n для подзаголовка страницы.
- [x] `frontend/web/src/features/creatorApplications/ModerationPage.test.tsx` -- создать по аналогии с `VerificationPage.test.tsx`: fixture со `status: "moderation"`, mock `listCreatorApplications` / `getCreatorApplication`, проверка передачи `statuses: ["moderation"]` и дефолтного sort в API, открытие/закрытие drawer через URL state, ErrorState на network-fail, разные empty-сообщения. -- Юнит-покрытие нового page.
- [x] `frontend/web/src/shared/layouts/DashboardLayout.test.tsx` -- расширить: моk counts с moderation-count > 0 → проверить `nav-badge-/admin/creator-applications/moderation` рендерится с правильным числом. -- Покрытие нового бейджа.
- [x] **In-scope addition:** `frontend/web/src/features/creatorApplications/components/ApplicationActions.tsx` -- расширить status-switch чтобы reject-кнопка рендерилась и для `moderation` (chunk 14 оставил switch только для `verification`). Без этой правки спека-задача «reject-кнопку подключить тем же способом» не выполнима — backend `RejectApplication` уже принимает оба статуса. Тест `ApplicationActions.test.tsx` обновлён симметрично.

**Acceptance Criteria:**
- Given admin зашёл на `/admin/creator-applications/moderation`, when страница загрузилась, then список содержит только заявки со `status=moderation`, отсортированные по `updated_at asc`, заголовок показывает total из ответа API.
- Given counts-handler вернул `{moderation: 7}`, when admin находится на любой странице админки, then в sidebar возле пункта «Модерация» отображается бейдж «7» с testid `nav-badge-/admin/creator-applications/moderation`.
- Given admin кликает по строке таблицы, when drawer открывается, then в URL появляется `?id=<uuid>`, prev/next работают, в теле drawer'а виден per-social verification-метод (auto/manual) через `SocialAdminRow`.
- Given пользователь обновляет страницу с `?id=<uuid>` в URL, when страница загрузилась, then drawer открыт сразу с этой заявкой.
- Given role brand_manager пытается открыть `/admin/creator-applications/moderation`, when страница рендерится, then RoleGuard редиректит на dashboard (поведение без изменений).
- Given chunk 14 предоставил reject-обработчик в `ApplicationDrawer`, when admin нажимает «Отклонить» на drawer'е moderation-экрана, then обработчик отрабатывает идентично verification-экрану и инвалидирует list+counts queries.

## Spec Change Log

## Design Notes

**`sort.ts` рефактор — минимальный, обратно-совместимый.** Сейчас `parseSortFromUrl`, `serializeSort`, `fieldForColumn` опираются на модульные константы `DEFAULT_SORT` и `COLUMN_TO_FIELD`. Чтобы не плодить дубли утилит, добавить опциональный параметр-перегрузку: при отсутствии — поведение прежнее (важно для VerificationPage и его теста). ModerationPage передаёт свой defaults и column-map.

**Структура страницы — клон, не абстракция.** Не выносим общую `<ApplicationsListPage>` обёртку: разница в STATUSES + sort defaults + column map + один-два testid + пара hours-вычислений. Клонирование короче и читабельнее, чем 5 props на абстрактный компонент. Если появится третий экран (creators?) — тогда решим.

**Reject в drawer — зависимость от chunk 14.** На момент написания спеки chunk 14 ещё в работе у параллельного агента. Имплементатор chunk 16 при старте обязан сначала прочитать VerificationPage + ApplicationDrawer в их финальном состоянии после chunk 14, и применить ровно тот же паттерн (drawer внутри / slot / отдельный компонент — без разницы, главное симметрия). Если паттерн вызывает вопросы — Ask First (см. Boundaries).

## Verification

**Commands:**
- `make lint-web` -- expected: tsc + eslint без ошибок.
- `make test-unit-web` -- expected: все тесты зелёные, включая новый `ModerationPage.test.tsx` и расширенный `DashboardLayout.test.tsx`.

**Manual checks:**
- `make start-web` → залогиниться админом → открыть `/admin/creator-applications/moderation`: ожидается список с заявками только в `moderation`, дефолтный sort (старейшие по updatedAt сверху), счётчик в заголовке.
- В sidebar возле «Модерация» — бейдж с числом, совпадающим с counts.moderation.
- Клик по строке → drawer с детализацией; в блоке соцсетей виден лейбл «Подтверждена автоматически/вручную» + дата.
- Reject-кнопка отрабатывает по тому же паттерну, что в verification (после интеграции с chunk 14).

## Suggested Review Order

**ModerationPage — основная страница**

- Конфигурация страницы: статусы, дефолтный sort `updated_at asc`, override колонок-в-поля.
  [`ModerationPage.tsx:33`](../../frontend/web/src/features/creatorApplications/ModerationPage.tsx#L33)

- `hoursInStage` рендерится от `updatedAt` (а не `createdAt`) и сделана sortable, чтобы клик возвращал к дефолтной сортировке.
  [`ModerationPage.tsx:291`](../../frontend/web/src/features/creatorApplications/ModerationPage.tsx#L291)

- Маппинг sort-поля → активная колонка в шапке, расширен `updated_at → hoursInStage` (стрелка ↑/↓ при дефолте).
  [`ModerationPage.tsx:349`](../../frontend/web/src/features/creatorApplications/ModerationPage.tsx#L349)

**Sort-инфраструктура — параметризация**

- Опциональные `defaults` и `map` — VerificationPage продолжает работать без аргументов.
  [`sort.ts:32`](../../frontend/web/src/features/creatorApplications/sort.ts#L32)

- `DEFAULT_COLUMN_TO_FIELD` теперь экспортирован — ModerationPage расширяет его spread'ом.
  [`sort.ts:15`](../../frontend/web/src/features/creatorApplications/sort.ts#L15)

**Reject — расширение action-switch'а**

- `verification` и `moderation` рендерят один и тот же `RejectApplicationDialog` (бэк уже принимает оба статуса).
  [`ApplicationActions.tsx:15`](../../frontend/web/src/features/creatorApplications/components/ApplicationActions.tsx#L15)

**Sidebar — бейдж модерации**

- `moderationCount` симметричен `verificationCount`, та же sparse-miss защита и hide-on-error.
  [`DashboardLayout.tsx:48`](../../frontend/web/src/shared/layouts/DashboardLayout.tsx#L48)

- Передача в menu item.
  [`DashboardLayout.tsx:74`](../../frontend/web/src/shared/layouts/DashboardLayout.tsx#L74)

**Активация страницы**

- Импорт переключён со stub'а на реальную ModerationPage.
  [`App.tsx:13`](../../frontend/web/src/App.tsx#L13)

- Подзаголовок страницы (description) добавлен в i18n.
  [`creatorApplications.json:11`](../../frontend/web/src/shared/i18n/locales/ru/creatorApplications.json#L11)

**Тесты**

- Юнит-покрытие новой страницы: states, drawer, reject-кнопка, sort-roundtrip + клик по hoursInStage.
  [`ModerationPage.test.tsx:1`](../../frontend/web/src/features/creatorApplications/ModerationPage.test.tsx#L1)

- Бейдж модерации, sparse-miss, hide-on-error.
  [`DashboardLayout.test.tsx:104`](../../frontend/web/src/shared/layouts/DashboardLayout.test.tsx#L104)

- ApplicationActions: `verification` + `moderation` через it.each.
  [`ApplicationActions.test.tsx:79`](../../frontend/web/src/features/creatorApplications/components/ApplicationActions.test.tsx#L79)

- Sort: новые блоки про `parseSortFromUrl`/`serializeSort` с custom defaults и `fieldForColumn` с custom map.
  [`sort.test.ts:69`](../../frontend/web/src/features/creatorApplications/sort.test.ts#L69)
