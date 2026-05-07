# Deferred work

Записи копятся при review-loops в `bmad-quick-dev`. Каждая запись — incidentально вскрытая проблема, которая не относится к текущему changeset, но требует отдельного PR.

## 2026-05-07 — review chunk 8b (campaign detail+edit frontend)

### [blocker] Refresh-middleware: повторный fetch отправляет пустой body

**Где:** `frontend/web/src/api/client.ts:60-77`.

**Что:** При 401 на любом мутационном запросе (POST/PATCH/PUT) middleware делает `return fetch(request)` после refresh. У уже отправленного `Request` тело — одноразовый `ReadableStream`; повторный `fetch(request)` уйдёт с пустым body, бэк ответит 422 вместо желаемого retry.

**Импакт:** При истёкшем access-токене любая мутация (PATCH /campaigns/{id}, POST /campaigns, любая будущая) даст ложный validation-error пользователю вместо прозрачного retry. UX-bug + диагностический шум.

**Фикс:** склонировать запрос ДО первого fetch'а (`request.clone()`), повторно отправлять клон.

**Кандидат в стандарты:** правило «HTTP-клиент с retry-after-refresh обязан клонировать Request до первого fetch». Решить — добавлять ли в `frontend-api.md` (cross-cutting).

### [major] PATCH 404 race не закрывает edit-режим

**Где:** `frontend/web/src/features/campaigns/CampaignDetailPage.tsx` (Edit secion `onError`).

**Что:** Если пока admin редактирует кампанию, кампанию удалили (race-сценарий), PATCH вернёт 404 `CAMPAIGN_NOT_FOUND`. Текущая реализация (по спеке) показывает form-level error и оставляет пользователя в edit-режиме без шанса выйти иначе как через Cancel. Сама кампания при этом всё ещё readable (`isDeleted=true`), GET вернёт её — UI рассогласован: редактируем то, что уже не редактируется.

**Импакт:** Узкий corner case, но застрявший edit-режим без явной подсказки. Пользователь поймёт по тексту «Кампания не найдена», но кнопки выхода кроме Cancel нет.

**Фикс (предложение для нового PR):** при PATCH 404 не показывать form-level — invalidate detail (что приведёт к refetch и dedicated NotFoundState). Либо явный редирект на `/campaigns` с toast'ом.

**Спека-аспект:** текущая спека выбрала именно form-level error («422/404 на PATCH | form-level error через getErrorMessage»). Если меняем поведение — править I/O Matrix следующего chunk'а.

### [nitpick] Spec gap: переиспользование `labels.deletedBadge` не задокументировано в Code Map

**Где:** спека `_bmad-output/implementation-artifacts/spec-campaign-detail-edit-frontend.md` § Code Map.

**Что:** Спека перечисляет блок `detail.*` ключами без `deletedBadge`. Реализация переиспользует `campaigns:labels.deletedBadge` (уже существующий из chunk 9 admin-list). Это корректное DRY-решение, но нечитающий контекст разработчик легко добавил бы дубль `detail.deletedBadge`.

**Фикс:** уточнить в Code Map: «бейдж переиспользует `campaigns:labels.deletedBadge` из chunk 9». Решить при следующем редактировании спеки.
