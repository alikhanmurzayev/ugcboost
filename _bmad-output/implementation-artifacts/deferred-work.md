# Deferred Work

Findings из ревью, которые признаны pre-existing (не вызваны текущей story) или вне scope, и отложены в backlog.

## Из chunk 16 (admin moderation screen frontend, baseline 484f650)

### VerificationPage-наследие — общая applications-list-page инфра

Унаследовано в ModerationPage без правок (тот же код-путь существует и в VerificationPage):

- **Drawer-footer всегда рендерится на `isLoading`/`isError` детали** — `<footer>` показывает border+padding, пока `application=undefined`. Визуальный flicker. Файл: `components/ApplicationDrawer.tsx:124-131`.
- **`idx === -1` после refetch / при URL'е с чужим `?id=`** — drawer открыт, prev/next кнопки блокированы, навигация мёртвая.
- **`goPrev/goNext` со stale `idx`** — TOCTOU между refetch'ем list'а и кликом стрелки.
- **`idx`-навигация не пересекает границы страниц** — на 50-й заявке `→` блокируется, хотя на странице 2 есть следующая.
- **`parsePage` без upper bound** — `?page=99999999999` принимается, фронт уходит в пустую страницу.
- **`detailQuery.data?.status !== STATUSES`** — drawer открывается заявкой, которая уже сменила статус (race), без сигнала пользователю.

Решение: вынести общую `<ApplicationsListPage>` обёртку или hook, когда появится третий клон (Contracts/Rejected/Creators) — тогда есть смысл обобщать.

### Counts-бейдж не свежеется автоматически у других админов

- `countsQuery` в `DashboardLayout` обновляется только при invalidate из reject/verify-флоу того же сеанса. Действия других админов на других ноутах badge не отразит до перезагрузки страницы.

Решение: настроить `refetchInterval` (например, 30s) или `refetchOnWindowFocus: true` для `countsQuery`.

### `hoursInStage` от `updatedAt` — не равно «часов в этапе»

- На бэке `updated_at` пересчитывается на любом UPDATE, не только при смене статуса. SLA-индикатор `HoursBadge` будет занижать для заявок, у которых что-то правили в moderation (re-verify соцсети, привязка Telegram).

Решение: добавить на бэке `status_changed_at` или таблицу `application_status_history`, переключить frontend на это поле.

### `hoursSince(undefined)` → `NaN` → `"NaNд"`

- `hours.ts:1-4` не валидирует `Number.isFinite`. Если backend отдаст `updatedAt: null` — badge покажет `NaN`. Защита нужна и в `hoursSince`, и в `HoursBadge.formatHours`.

### URL не нормализуется при mount

- Если попасть на ModerationPage с URL `?sort=created_at&order=desc` (например, скопировано с VerificationPage), параметры применятся как есть — оператор не поймёт, что страница сортируется не по дефолту.

Решение: при mount нормализовать `searchParams` через `serializeSort` с дефолтом текущей страницы.

### testid префикс `/admin/` в спеке

- Спека `spec-admin-creator-applications-moderation.md` (frozen-секция AC-2) ссылается на testid `nav-badge-/admin/creator-applications/moderation`, но фактический паттерн URL'ов админки в `routes.ts` — без префикса `/admin`. Реальный testid `nav-badge-creator-applications/moderation` (консистентен с verification-бейджем).

Решение: либо пофиксить опечатку в спеке (next iteration), либо отрефакторить роутинг на `/admin/*` (отдельный chunk, scope заметный).
